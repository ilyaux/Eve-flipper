package engine

import (
	"math"
	"sort"

	"eve-flipper/internal/esi"
)

// ----- Well-known EVE type IDs -----

const (
	PLEXTypeID           int32 = 44992
	SkillExtractorTypeID int32 = 40519
	LargeSkillInjTypeID  int32 = 40520
	MPTCTypeID           int32 = 34133 // Multiple Pilot Training Certificate
)

// Global PLEX Market region (introduced 7 July 2025 by CCP).
// PLEX orders are no longer found in per-region markets; use this region ID.
const GlobalPLEXRegionID int32 = 19000001

// NES default PLEX prices set by CCP. Used as fallbacks when user doesn't override.
const (
	DefaultNESExtractorPLEX int = 293 // Skill Extractor cost in NES
	DefaultNESMPTCPLEX      int = 485 // Multiple Pilot Training Certificate
	DefaultNESOmegaPLEX     int = 500 // 30 days Omega
)

// NESPrices holds user-configurable NES PLEX prices.
// Zero values fall back to defaults above.
type NESPrices struct {
	ExtractorPLEX int `json:"extractor_plex"`
	MPTCPLEX      int `json:"mptc_plex"`
	OmegaPLEX     int `json:"omega_plex"`
}

// Resolve returns actual prices with defaults applied for zero values.
func (n NESPrices) Resolve() (extractor, mptc, omega int) {
	extractor = n.ExtractorPLEX
	if extractor <= 0 {
		extractor = DefaultNESExtractorPLEX
	}
	mptc = n.MPTCPLEX
	if mptc <= 0 {
		mptc = DefaultNESMPTCPLEX
	}
	omega = n.OmegaPLEX
	if omega <= 0 {
		omega = DefaultNESOmegaPLEX
	}
	return
}

// Jita region for related item lookups.
const JitaRegionID int32 = 10000002

// SP training constants
// Formula: SP/min = primary_attribute + secondary_attribute/2
// Optimal remap (27 primary / 21 secondary): 37.5 SP/min = 2250 SP/hr
// With +5 implants (32/26): 45 SP/min = 2700 SP/hr
const (
	BaseSPPerHour      float64 = 2250 // optimal remap (27/21), no implants, Omega
	SPPerHourPlus5     float64 = 2700 // optimal remap (32/26), +5 implants, Omega
	HoursPerMonth      float64 = 24 * 30
	SPPerExtractor     int     = 500000    // SP extracted per Skill Extractor
	MinSPForExtraction int     = 5_000_000 // cannot extract below this threshold
)

// MaxHistoryPoints is the number of price history entries kept for the chart.
const MaxHistoryPoints = 90

// ===================================================================
// Data structures returned to the frontend
// ===================================================================

// PLEXDashboard is the top-level response for GET /api/plex/dashboard.
type PLEXDashboard struct {
	// Global PLEX market price (since July 2025, PLEX is a global market)
	PLEXPrice PLEXGlobalPrice `json:"plex_price"`

	// NES ↔ Market arbitrage paths
	Arbitrage []ArbitragePath `json:"arbitrage"`

	// SP Farm profitability
	SPFarm SPFarmResult `json:"sp_farm"`

	// Technical indicators
	Indicators *PLEXIndicators `json:"indicators,omitempty"`

	// Chart overlay data computed on backend (eliminates frontend duplication)
	ChartOverlays *ChartOverlays `json:"chart_overlays,omitempty"`

	// Historical arbitrage profitability (retroactive computation from PLEX price history)
	ArbHistory *ArbHistoryData `json:"arb_history,omitempty"`

	// Trading signal
	Signal PLEXSignal `json:"signal"`

	// Price history for chart
	History []PricePoint `json:"history"`

	// Market depth info for PLEX arbitrage execution
	MarketDepth *MarketDepthInfo `json:"market_depth,omitempty"`

	// SP injection diminishing returns (ISK/SP for each SP tier)
	InjectionTiers []InjectionTier `json:"injection_tiers,omitempty"`

	// PLEX → Omega cost comparison
	OmegaComparison *OmegaComparison `json:"omega_comparison,omitempty"`

	// Cross-hub arbitrage opportunities
	CrossHub []CrossHubArbitrage `json:"cross_hub,omitempty"`
}

// OmegaComparison compares PLEX-based Omega cost vs real-money cost.
type OmegaComparison struct {
	PLEXNeeded int     `json:"plex_needed"` // 500
	TotalISK   float64 `json:"total_isk"`   // 500 * plex sell price
	RealMoneyUSD float64 `json:"real_money_usd"` // user-provided $ price
	ISKPerUSD    float64 `json:"isk_per_usd"`    // total_isk / real_money_usd
}

// CrossHubArbitrage shows price differences for SP-related items across major trade hubs.
type CrossHubArbitrage struct {
	ItemName  string  `json:"item_name"`
	TypeID    int32   `json:"type_id"`
	BestHub   string  `json:"best_hub"`   // hub with cheapest sell price
	BestPrice float64 `json:"best_price"` // best sell price
	JitaPrice float64 `json:"jita_price"` // Jita sell price
	DiffPct   float64 `json:"diff_pct"`   // (jita - best) / best * 100
	ProfitISK float64 `json:"profit_isk"` // per unit after fees
	Viable    bool    `json:"viable"`
}

// InjectionTier shows how many SP a buyer receives per injector at different SP levels.
type InjectionTier struct {
	Label      string  `json:"label"`       // e.g., "< 5M SP"
	SPReceived int     `json:"sp_received"` // 500000, 400000, 300000, 150000
	ISKPerSP   float64 `json:"isk_per_sp"`  // injector price / SP received
	Efficiency float64 `json:"efficiency"`  // SP received / 500000 * 100
}

// ArbHistoryData provides retroactive arbitrage profitability computed from
// historical PLEX prices + current related item prices (approximation).
type ArbHistoryData struct {
	ExtractorNES []ArbHistoryPoint `json:"extractor_nes,omitempty"`  // NES Extractor → Sell
	SPChainNES   []ArbHistoryPoint `json:"sp_chain_nes,omitempty"`   // NES Extractor → Injector
	MPTCNES      []ArbHistoryPoint `json:"mptc_nes,omitempty"`       // NES MPTC → Sell
	SPFarmProfit []ArbHistoryPoint `json:"sp_farm_profit,omitempty"` // SP Farm monthly profit
}

// ArbHistoryPoint is a single date + profit ISK for historical arb tracking.
type ArbHistoryPoint struct {
	Date      string  `json:"date"`
	ProfitISK float64 `json:"profit_isk"`
	ROI       float64 `json:"roi"`
}

// MarketDepthInfo holds order-book depth for key items relevant to PLEX arbitrage.
type MarketDepthInfo struct {
	PLEXSellDepth5   DepthSummary `json:"plex_sell_depth_5"`  // top 5 PLEX sell levels
	ExtractorSellQty int64        `json:"extractor_sell_qty"` // total extractor sell volume near best
	ExtractorBuyQty  int64        `json:"extractor_buy_qty"`  // total extractor buy volume near best
	InjectorSellQty  int64        `json:"injector_sell_qty"`  // total injector sell volume near best
	InjectorBuyQty   int64        `json:"injector_buy_qty"`   // total injector buy volume near best
	MPTCSellQty      int64        `json:"mptc_sell_qty"`      // total MPTC sell volume near best
	MPTCBuyQty       int64        `json:"mptc_buy_qty"`       // total MPTC buy volume near best
	// Time-to-fill estimates (hours to sell 1 unit, based on daily volume)
	ExtractorFillHours float64 `json:"extractor_fill_hours"`
	InjectorFillHours  float64 `json:"injector_fill_hours"`
	MPTCFillHours      float64 `json:"mptc_fill_hours"`
	PLEXFillHours      float64 `json:"plex_fill_hours"` // hours to sell 100 PLEX
}

// DepthSummary shows total volume available within a price range.
type DepthSummary struct {
	TotalVolume int64   `json:"total_volume"`
	BestPrice   float64 `json:"best_price"`
	WorstPrice  float64 `json:"worst_price"` // worst price in the depth window
	Levels      int     `json:"levels"`      // distinct price levels
}

// ChartOverlays provides pre-computed chart series so the frontend doesn't
// need to duplicate SMA/BB math.
type ChartOverlays struct {
	SMA7          []OverlayPoint `json:"sma7,omitempty"`
	SMA30         []OverlayPoint `json:"sma30,omitempty"`
	BollingerUp   []OverlayPoint `json:"bollinger_upper,omitempty"`
	BollingerDown []OverlayPoint `json:"bollinger_lower,omitempty"`
}

// OverlayPoint is a single date+value pair for chart overlays.
type OverlayPoint struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

// PLEXGlobalPrice holds pricing from the Global PLEX Market (region 19000001).
type PLEXGlobalPrice struct {
	BuyPrice      float64 `json:"buy_price"`      // highest buy order
	SellPrice     float64 `json:"sell_price"`      // lowest sell order
	Spread        float64 `json:"spread"`          // sell - buy
	SpreadPct     float64 `json:"spread_pct"`      // (sell - buy) / buy * 100
	Volume24h     int64   `json:"volume_24h"`
	BuyOrders     int     `json:"buy_orders"`      // total buy order count
	SellOrders    int     `json:"sell_orders"`      // total sell order count
	Percentile90d float64 `json:"percentile_90d"` // what % of 90d prices are below current
}

// ArbitragePath represents one NES→Market or cross-hub arbitrage route.
type ArbitragePath struct {
	Name         string  `json:"name"`
	Type         string  `json:"type"`          // "nes_sell", "nes_process", "market_process", "spread"
	PLEXCost     int     `json:"plex_cost"`     // PLEX spent (for NES paths; 0 for market paths)
	CostISK      float64 `json:"cost_isk"`      // total ISK cost
	RevenueGross float64 `json:"revenue_gross"` // ISK before taxes/fees
	RevenueISK   float64 `json:"revenue_isk"`   // ISK received after taxes
	ProfitISK    float64 `json:"profit_isk"`
	ROI          float64 `json:"roi"`     // profit / cost * 100
	Viable       bool    `json:"viable"`  // profit > 0 AND data available
	NoData       bool    `json:"no_data"` // true when market data was unavailable
	Detail       string  `json:"detail"`  // human-readable explanation
	// Break-even: PLEX price at which this path has zero profit (NES paths only; 0 for market/spread)
	BreakEvenPLEX float64 `json:"break_even_plex"`
	// ISK/hour estimate (0 for passive spread paths)
	EstMinutes int     `json:"est_minutes"`
	ISKPerHour float64 `json:"isk_per_hour"`
	// Execution slippage for 1 unit (from order book walk)
	SlippagePct       float64 `json:"slippage_pct"`        // worst-side slippage %
	AdjustedProfitISK float64 `json:"adjusted_profit_isk"` // profit after slippage
}

// SPFarmResult holds SP farming profitability per character per month.
type SPFarmResult struct {
	OmegaCostPLEX      int     `json:"omega_cost_plex"`
	OmegaCostISK       float64 `json:"omega_cost_isk"`
	ExtractorsPerMonth float64 `json:"extractors_per_month"`
	ExtractorCostPLEX  int     `json:"extractor_cost_plex"`
	ExtractorCostISK   float64 `json:"extractor_cost_isk"`
	TotalCostISK       float64 `json:"total_cost_isk"`
	InjectorsProduced  float64 `json:"injectors_produced"`
	InjectorSellPrice  float64 `json:"injector_sell_price"`
	RevenueISK         float64 `json:"revenue_isk"`
	ProfitISK          float64 `json:"profit_isk"`
	ProfitPerDay       float64 `json:"profit_per_day"`
	ROI                float64 `json:"roi"`
	Viable             bool    `json:"viable"`
	// With +5 implants
	ExtractorsPlus5   float64 `json:"extractors_plus5"`
	ProfitPlus5       float64 `json:"profit_plus5"`
	ProfitPerDayPlus5 float64 `json:"profit_per_day_plus5"`
	ROIPlus5          float64 `json:"roi_plus5"`
	// Startup & multi-char scaling
	StartupSP        int     `json:"startup_sp"`         // min SP needed (5,000,000)
	StartupTrainDays float64 `json:"startup_train_days"` // days to reach 5.5M SP from scratch
	StartupCostISK   float64 `json:"startup_cost_isk"`   // omega cost during ramp-up
	PaybackDays      float64 `json:"payback_days"`       // days to recoup startup from profit
	// Multi-character: profit scaled by N characters
	// (frontend supplies numChars; backend computes per-char, frontend multiplies)
	MPTCCostPLEX int     `json:"mptc_cost_plex"` // PLEX cost of 1 MPTC (for extra queue on same account)
	MPTCCostISK  float64 `json:"mptc_cost_isk"`
	// Omega ISK value (500 PLEX → how much ISK)
	OmegaISKValue float64 `json:"omega_isk_value"` // 500 PLEX * market price
	PLEXUnitPrice float64 `json:"plex_unit_price"` // single PLEX market price
	// Instant sell alternative (revenue if selling to buy orders)
	InstantSellRevenueISK float64 `json:"instant_sell_revenue_isk"`
	InstantSellProfitISK  float64 `json:"instant_sell_profit_isk"`
	InstantSellROI        float64 `json:"instant_sell_roi"`
	// Instant sell with +5 implants
	InstantSellProfitPlus5 float64 `json:"instant_sell_profit_plus5"`
	InstantSellROIPlus5    float64 `json:"instant_sell_roi_plus5"`
	// Break-even PLEX price where SP farming profit = 0
	BreakEvenPLEX float64 `json:"break_even_plex"`
}

// PLEXIndicators holds technical analysis indicators computed from price history.
type PLEXIndicators struct {
	// Moving averages
	SMA7  float64 `json:"sma7"`
	SMA30 float64 `json:"sma30"`

	// Bollinger Bands (20, 2)
	BollingerUpper  float64 `json:"bollinger_upper"`
	BollingerMiddle float64 `json:"bollinger_middle"`
	BollingerLower  float64 `json:"bollinger_lower"`

	// RSI (14)
	RSI float64 `json:"rsi"`

	// Price change percentages
	Change24h float64 `json:"change_24h"`
	Change7d  float64 `json:"change_7d"`
	Change30d float64 `json:"change_30d"`

	// Volume anomaly (for CCP sale detection)
	AvgVolume30d  float64 `json:"avg_volume_30d"`
	VolumeToday   int64   `json:"volume_today"`
	VolumeSigma   float64 `json:"volume_sigma"`    // std deviations above mean
	CCPSaleSignal bool    `json:"ccp_sale_signal"` // volume > mean+2σ AND price drop > 3%

	// Volatility index (20-day annualized)
	Volatility20d float64 `json:"volatility_20d"` // annualized 20-day log-return vol
	VolRegime     string  `json:"vol_regime"`      // "low", "medium", "high"
}

// PLEXSignal is the BUY/SELL/HOLD recommendation.
type PLEXSignal struct {
	Action     string   `json:"action"`     // "BUY", "SELL", "HOLD"
	Confidence float64  `json:"confidence"` // 0-100
	Reasons    []string `json:"reasons"`
}

// PricePoint is a single day of PLEX price history for charting.
type PricePoint struct {
	Date    string  `json:"date"`
	Average float64 `json:"average"`
	High    float64 `json:"high"`
	Low     float64 `json:"low"`
	Volume  int64   `json:"volume"`
}

// ===================================================================
// Core analytics functions
// ===================================================================

// ComputePLEXDashboard builds the full PLEX+ dashboard from raw market data.
func ComputePLEXDashboard(
	plexOrders []esi.MarketOrder, // Global PLEX Market orders (region 19000001)
	relatedOrders map[int32][]esi.MarketOrder, // typeID → Jita orders (extractor, injector, MPTC)
	history []esi.HistoryEntry, // PLEX price history
	relatedHistory map[int32][]esi.HistoryEntry, // typeID → price history (for fill-time estimation)
	salesTaxPct float64, // user's sales tax %
	brokerFeePct float64, // user's broker fee %
	nes NESPrices, // user-configurable NES PLEX prices
	omegaUSD float64, // real-money Omega price (USD, 0 = skip)
	crossHubOrders map[int32]map[int32][]esi.MarketOrder, // typeID → regionID → orders (for cross-hub arb)
) PLEXDashboard {
	if salesTaxPct <= 0 {
		salesTaxPct = 3.6 // Accounting V
	}
	if brokerFeePct <= 0 {
		brokerFeePct = 1.0 // Broker Relations V
	}
	nesExtractor, nesMPTC, nesOmega := nes.Resolve()

	// Sort history chronologically once; pass sorted slice to all sub-functions.
	sortedHist := sortHistory(history)

	// ---- Global PLEX price ----
	globalPrice := computeGlobalPLEXPrice(plexOrders, sortedHist)

	plexPrice := globalPrice.SellPrice // use sell price as reference (instant buy from market)
	if plexPrice <= 0 {
		plexPrice = globalPrice.BuyPrice
	}

	// ---- Related item prices (Jita) ----
	extractorSell := bestSellPrice(relatedOrders[SkillExtractorTypeID])
	extractorBuy := bestBuyPrice(relatedOrders[SkillExtractorTypeID])
	injectorSell := bestSellPrice(relatedOrders[LargeSkillInjTypeID])
	injectorBuy := bestBuyPrice(relatedOrders[LargeSkillInjTypeID])
	mptcSell := bestSellPrice(relatedOrders[MPTCTypeID])
	mptcBuy := bestBuyPrice(relatedOrders[MPTCTypeID])

	// Net revenue multiplier: in EVE, broker fee and sales tax are both
	// percentage deductions from the order price.
	netMult := 1.0 - salesTaxPct/100 - brokerFeePct/100
	// For instant sell (selling to buy orders), only sales tax applies; no broker fee
	salesTaxOnly := 1.0 - salesTaxPct/100

	// ---- NES Arbitrage ----
	var arbitrage []ArbitragePath

	// Path 1: NES Extractor → Sell on market
	{
		cost := float64(nesExtractor) * plexPrice
		noData := extractorSell <= 0
		revenue := extractorSell * netMult
		profit := revenue - cost
		// Break-even: revenue / nesPlexCost = PLEX price at zero profit
		be := safeDiv(revenue, float64(nesExtractor))
		arbitrage = append(arbitrage, ArbitragePath{
			Name:          "Skill Extractor (NES → Sell)",
			Type:          "nes_sell",
			PLEXCost:      nesExtractor,
			CostISK:       cost,
			RevenueGross:  extractorSell,
			RevenueISK:    revenue,
			ProfitISK:     profit,
			ROI:           safeDiv(profit, cost) * 100,
			Viable:        profit > 0 && !noData,
			NoData:        noData,
			Detail:        "Buy Skill Extractor from NES, sell on market",
			BreakEvenPLEX: be,
			EstMinutes:    5,
			ISKPerHour:    safeDiv(profit, 5.0/60),
		})
	}

	// Path 2: NES Extractor → Extract SP → Sell Injector
	{
		cost := float64(nesExtractor) * plexPrice
		noData := injectorSell <= 0
		revenue := injectorSell * netMult
		profit := revenue - cost
		be := safeDiv(revenue, float64(nesExtractor))
		arbitrage = append(arbitrage, ArbitragePath{
			Name:          "SP Chain (NES Extractor → Injector)",
			Type:          "nes_process",
			PLEXCost:      nesExtractor,
			CostISK:       cost,
			RevenueGross:  injectorSell,
			RevenueISK:    revenue,
			ProfitISK:     profit,
			ROI:           safeDiv(profit, cost) * 100,
			Viable:        profit > 0 && !noData,
			NoData:        noData,
			Detail:        "Buy Extractor from NES, extract SP, sell Injector",
			BreakEvenPLEX: be,
			EstMinutes:    10,
			ISKPerHour:    safeDiv(profit, 10.0/60),
		})
	}

	// Path 3: NES MPTC → Sell
	{
		cost := float64(nesMPTC) * plexPrice
		noData := mptcSell <= 0
		revenue := mptcSell * netMult
		profit := revenue - cost
		be := safeDiv(revenue, float64(nesMPTC))
		arbitrage = append(arbitrage, ArbitragePath{
			Name:          "MPTC (NES → Sell)",
			Type:          "nes_sell",
			PLEXCost:      nesMPTC,
			CostISK:       cost,
			RevenueGross:  mptcSell,
			RevenueISK:    revenue,
			ProfitISK:     profit,
			ROI:           safeDiv(profit, cost) * 100,
			Viable:        profit > 0 && !noData,
			NoData:        noData,
			Detail:        "Buy MPTC from NES, sell on market",
			BreakEvenPLEX: be,
			EstMinutes:    5,
			ISKPerHour:    safeDiv(profit, 5.0/60),
		})
	}

	// Path 4: Market Extractor → Extract SP → Sell Injector
	// Compare buying extractor directly from market vs NES
	{
		cost := extractorSell // buy extractor from market (lowest sell order)
		noData := extractorSell <= 0 || injectorSell <= 0
		revenue := injectorSell * netMult
		profit := revenue - cost
		arbitrage = append(arbitrage, ArbitragePath{
			Name:         "SP Chain (Market Extractor → Injector)",
			Type:         "market_process",
			PLEXCost:     0,
			CostISK:      cost,
			RevenueGross: injectorSell,
			RevenueISK:   revenue,
			ProfitISK:    profit,
			ROI:          safeDiv(profit, cost) * 100,
			Viable:       profit > 0 && !noData,
			NoData:       noData,
			Detail:       "Buy Extractor from market, extract SP, sell Injector",
			EstMinutes:   10,
			ISKPerHour:   safeDiv(profit, 10.0/60),
		})
	}

	// ---- Spread Trading (Market Making) ----
	// These paths don't use PLEX/NES; they're pure buy-low-sell-high on the items themselves.
	// Cost = buy order (you place buy order) + broker fee on buy side
	// Revenue = sell order (you place sell order) - tax - broker fee
	// For limit orders: buyer pays broker fee, seller pays broker fee + sales tax.
	buyBrokerMult := 1.0 + brokerFeePct/100 // you pay more when placing buy order

	// Path 5: PLEX Spread (Market Making)
	{
		cost := globalPrice.BuyPrice * buyBrokerMult // place buy order + broker
		noData := globalPrice.BuyPrice <= 0 || globalPrice.SellPrice <= 0
		grossRev := globalPrice.SellPrice
		revenue := grossRev * netMult // sell order - tax - broker
		profit := revenue - cost
		arbitrage = append(arbitrage, ArbitragePath{
			Name:         "PLEX Spread (Market Make)",
			Type:         "spread",
			PLEXCost:     0,
			CostISK:      cost,
			RevenueGross: grossRev,
			RevenueISK:   revenue,
			ProfitISK:    profit,
			ROI:          safeDiv(profit, cost) * 100,
			Viable:       profit > 0 && !noData,
			NoData:       noData,
			Detail:       "Place buy order for PLEX, sell via sell order (spread profit)",
		})
	}

	// Path 6: Extractor Spread
	{
		cost := extractorBuy * buyBrokerMult
		noData := extractorBuy <= 0 || extractorSell <= 0
		revenue := extractorSell * netMult
		profit := revenue - cost
		arbitrage = append(arbitrage, ArbitragePath{
			Name:         "Extractor Spread (Market Make)",
			Type:         "spread",
			PLEXCost:     0,
			CostISK:      cost,
			RevenueGross: extractorSell,
			RevenueISK:   revenue,
			ProfitISK:    profit,
			ROI:          safeDiv(profit, cost) * 100,
			Viable:       profit > 0 && !noData,
			NoData:       noData,
			Detail:       "Place buy order for Extractor, sell via sell order",
		})
	}

	// Path 7: Injector Spread
	{
		cost := injectorBuy * buyBrokerMult
		noData := injectorBuy <= 0 || injectorSell <= 0
		revenue := injectorSell * netMult
		profit := revenue - cost
		arbitrage = append(arbitrage, ArbitragePath{
			Name:         "Injector Spread (Market Make)",
			Type:         "spread",
			PLEXCost:     0,
			CostISK:      cost,
			RevenueGross: injectorSell,
			RevenueISK:   revenue,
			ProfitISK:    profit,
			ROI:          safeDiv(profit, cost) * 100,
			Viable:       profit > 0 && !noData,
			NoData:       noData,
			Detail:       "Place buy order for Injector, sell via sell order",
		})
	}

	// Path 8: MPTC Spread
	{
		cost := mptcBuy * buyBrokerMult
		noData := mptcBuy <= 0 || mptcSell <= 0
		revenue := mptcSell * netMult
		profit := revenue - cost
		arbitrage = append(arbitrage, ArbitragePath{
			Name:         "MPTC Spread (Market Make)",
			Type:         "spread",
			PLEXCost:     0,
			CostISK:      cost,
			RevenueGross: mptcSell,
			RevenueISK:   revenue,
			ProfitISK:    profit,
			ROI:          safeDiv(profit, cost) * 100,
			Viable:       profit > 0 && !noData,
			NoData:       noData,
			Detail:       "Place buy order for MPTC, sell via sell order",
		})
	}

	// ---- Execution Slippage per arb path ----
	// For NES paths: slippage on sell side only (you sell item on market)
	// For spread paths: slippage on both buy and sell sides
	for i := range arbitrage {
		arb := &arbitrage[i]
		if arb.NoData {
			arb.AdjustedProfitISK = arb.ProfitISK
			continue
		}
		switch {
		case arb.Type == "nes_sell" && arb.Name == "Skill Extractor (NES → Sell)":
			// Sell 1 extractor: walk buy orders
			plan := ComputeExecutionPlan(relatedOrders[SkillExtractorTypeID], 1, false)
			arb.SlippagePct = plan.SlippagePercent
			adjRev := plan.ExpectedPrice * netMult
			if adjRev <= 0 {
				adjRev = arb.RevenueISK
			}
			arb.AdjustedProfitISK = adjRev - arb.CostISK
		case arb.Type == "nes_process":
			// Sell 1 injector: walk buy orders
			plan := ComputeExecutionPlan(relatedOrders[LargeSkillInjTypeID], 1, false)
			arb.SlippagePct = plan.SlippagePercent
			adjRev := plan.ExpectedPrice * netMult
			if adjRev <= 0 {
				adjRev = arb.RevenueISK
			}
			arb.AdjustedProfitISK = adjRev - arb.CostISK
		case arb.Type == "nes_sell" && arb.Name == "MPTC (NES → Sell)":
			// Sell 1 MPTC: walk buy orders
			plan := ComputeExecutionPlan(relatedOrders[MPTCTypeID], 1, false)
			arb.SlippagePct = plan.SlippagePercent
			adjRev := plan.ExpectedPrice * netMult
			if adjRev <= 0 {
				adjRev = arb.RevenueISK
			}
			arb.AdjustedProfitISK = adjRev - arb.CostISK
		case arb.Type == "market_process":
			// Buy 1 extractor from market (walk sell orders) + sell 1 injector (walk buy orders)
			buyPlan := ComputeExecutionPlan(relatedOrders[SkillExtractorTypeID], 1, true)
			sellPlan := ComputeExecutionPlan(relatedOrders[LargeSkillInjTypeID], 1, false)
			// Combined slippage: buy side + sell side
			arb.SlippagePct = buyPlan.SlippagePercent + sellPlan.SlippagePercent
			adjCost := buyPlan.ExpectedPrice
			adjRev := sellPlan.ExpectedPrice * netMult
			if adjCost <= 0 {
				adjCost = arb.CostISK
			}
			if adjRev <= 0 {
				adjRev = arb.RevenueISK
			}
			arb.AdjustedProfitISK = adjRev - adjCost
		case arb.Type == "spread":
			// Spread paths: for 1 unit, slippage is typically 0 (you place limit orders)
			// But if you wanted to market-take, we can compute it
			arb.SlippagePct = 0
			arb.AdjustedProfitISK = arb.ProfitISK
		default:
			arb.AdjustedProfitISK = arb.ProfitISK
		}
	}

	// ---- SP Farm Calculator ----
	spFarm := computeSPFarm(plexPrice, extractorSell, injectorSell, injectorBuy, netMult, salesTaxOnly, nesExtractor, nesOmega, nesMPTC)

	// ---- Price History ----
	priceHistory := convertHistory(sortedHist)

	// ---- Technical Indicators (uses pre-sorted history) ----
	indicators := computeIndicators(sortedHist)

	// ---- Chart Overlays (pre-computed SMA/BB for frontend) ----
	chartOverlays := computeChartOverlays(sortedHist)

	// ---- Historical Arbitrage Profitability ----
	arbHistory := computeArbHistory(sortedHist, extractorSell, injectorSell, mptcSell, netMult, nesExtractor, nesMPTC, nesOmega)

	// ---- Market Depth + Fill Times ----
	marketDepth := computeMarketDepth(plexOrders, relatedOrders, history, relatedHistory)

	// ---- Signal ----
	signal := computeSignal(indicators, globalPrice)

	// ---- Injection efficiency tiers ----
	// Shows ISK/SP at each diminishing returns bracket for injector buyers.
	var injectionTiers []InjectionTier
	if injectorSell > 0 {
		tiers := []struct {
			label string
			sp    int
		}{
			{"< 5M SP", 500000},
			{"5–50M SP", 400000},
			{"50–80M SP", 300000},
			{"> 80M SP", 150000},
		}
		injectionTiers = make([]InjectionTier, len(tiers))
		for i, t := range tiers {
			injectionTiers[i] = InjectionTier{
				Label:      t.label,
				SPReceived: t.sp,
				ISKPerSP:   injectorSell / float64(t.sp),
				Efficiency: float64(t.sp) / 500000 * 100,
			}
		}
	}

	// ---- Omega Comparison (PLEX vs real money) ----
	var omegaComp *OmegaComparison
	if omegaUSD > 0 && plexPrice > 0 {
		totalISK := float64(nesOmega) * plexPrice
		omegaComp = &OmegaComparison{
			PLEXNeeded:   nesOmega,
			TotalISK:     totalISK,
			RealMoneyUSD: omegaUSD,
			ISKPerUSD:    safeDiv(totalISK, omegaUSD),
		}
	}

	// ---- Cross-Hub Arbitrage ----
	var crossHub []CrossHubArbitrage
	if len(crossHubOrders) > 0 {
		crossHub = computeCrossHubArbitrage(crossHubOrders, netMult)
	}

	return PLEXDashboard{
		PLEXPrice:       globalPrice,
		Arbitrage:       arbitrage,
		SPFarm:          spFarm,
		Indicators:      indicators,
		ChartOverlays:   chartOverlays,
		ArbHistory:      arbHistory,
		MarketDepth:     marketDepth,
		Signal:          signal,
		History:         priceHistory,
		InjectionTiers:  injectionTiers,
		OmegaComparison: omegaComp,
		CrossHub:        crossHub,
	}
}

// computeGlobalPLEXPrice calculates buy/sell/spread from the Global PLEX Market.
// history must be sorted chronologically (ascending by date).
func computeGlobalPLEXPrice(orders []esi.MarketOrder, history []esi.HistoryEntry) PLEXGlobalPrice {
	if len(orders) == 0 {
		return PLEXGlobalPrice{}
	}

	bestBuy := 0.0
	bestSell := math.MaxFloat64
	buyCount := 0
	sellCount := 0
	for _, o := range orders {
		if o.IsBuyOrder {
			buyCount++
			if o.Price > bestBuy {
				bestBuy = o.Price
			}
		} else {
			sellCount++
			if o.Price < bestSell {
				bestSell = o.Price
			}
		}
	}
	if bestSell == math.MaxFloat64 {
		bestSell = 0
	}

	spread := bestSell - bestBuy
	spreadPct := 0.0
	if bestBuy > 0 {
		spreadPct = spread / bestBuy * 100
	}

	// Get latest daily volume from history
	vol24h := int64(0)
	if len(history) > 0 {
		// Take the most recent entry
		vol24h = history[len(history)-1].Volume
	}

	// 90-day price percentile: what % of history prices are below current sell price
	percentile := 0.0
	if bestSell > 0 && len(history) > 0 {
		below := 0
		for _, e := range history {
			if e.Average < bestSell {
				below++
			}
		}
		percentile = float64(below) / float64(len(history)) * 100
	}

	return PLEXGlobalPrice{
		BuyPrice:      bestBuy,
		SellPrice:     bestSell,
		Spread:        spread,
		SpreadPct:     sanitizeFloat(spreadPct),
		Volume24h:     vol24h,
		BuyOrders:     buyCount,
		SellOrders:    sellCount,
		Percentile90d: percentile,
	}
}

// computeSPFarm calculates monthly SP farming profitability.
// netMult = 1 - salesTax/100 - brokerFee/100.
// salesTaxOnly = 1 - salesTax/100 (for instant sell, no broker fee on seller side).
func computeSPFarm(plexPrice, extractorMarketSell, injectorMarketSell, injectorBuyPrice, netMult, salesTaxOnly float64, nesExtractor, nesOmega, nesMPTC int) SPFarmResult {
	omegaCostISK := float64(nesOmega) * plexPrice

	// Base attributes (no implants)
	spPerMonth := BaseSPPerHour * HoursPerMonth
	extractorsBase := spPerMonth / float64(SPPerExtractor)

	// NES-based extractor cost
	extractorCostISK := float64(nesExtractor) * plexPrice

	totalCostBase := omegaCostISK + extractorsBase*extractorCostISK
	revenueBase := extractorsBase * injectorMarketSell * netMult
	profitBase := revenueBase - totalCostBase

	// +5 implants
	spPerMonthPlus5 := SPPerHourPlus5 * HoursPerMonth
	extractorsPlus5 := spPerMonthPlus5 / float64(SPPerExtractor)
	totalCostPlus5 := omegaCostISK + extractorsPlus5*extractorCostISK
	revenuePlus5 := extractorsPlus5 * injectorMarketSell * netMult
	profitPlus5 := revenuePlus5 - totalCostPlus5

	// Startup cost: new char needs ~5.5M SP (5M floor + 500K buffer)
	// At BaseSPPerHour, time to 5.5M SP:
	startupSP := float64(MinSPForExtraction) + float64(SPPerExtractor) // 5.5M
	startupHours := startupSP / BaseSPPerHour
	startupDays := startupHours / 24
	// Omega cost during ramp-up (ceil to months)
	startupMonths := math.Ceil(startupDays / 30)
	startupCostISK := startupMonths * omegaCostISK

	// Payback: how many days of profit to recoup startup
	paybackDays := 0.0
	if profitBase > 0 {
		paybackDays = startupCostISK / (profitBase / 30)
	}

	// MPTC cost (for extra training queue on same account)
	mptcCostISK := float64(nesMPTC) * plexPrice

	// Omega ISK value (500 PLEX at market price)
	omegaISKValue := float64(nesOmega) * plexPrice

	// Instant sell: sell injectors to buy orders (no broker fee on seller side)
	instantRevenueBase := 0.0
	instantProfitBase := 0.0
	instantROI := 0.0
	instantProfitPlus5 := 0.0
	instantROIPlus5 := 0.0
	if injectorBuyPrice > 0 {
		instantRevenueBase = extractorsBase * injectorBuyPrice * salesTaxOnly
		instantProfitBase = instantRevenueBase - totalCostBase
		instantROI = safeDiv(instantProfitBase, totalCostBase) * 100
		// +5 implants instant sell
		instantRevenuePlus5 := extractorsPlus5 * injectorBuyPrice * salesTaxOnly
		instantProfitPlus5 = instantRevenuePlus5 - totalCostPlus5
		instantROIPlus5 = safeDiv(instantProfitPlus5, totalCostPlus5) * 100
	}

	// Break-even PLEX price: solve profit=0 for plexPrice
	// profit = extractorsBase * injectorSell * netMult - plexPrice * (omega + extractorsBase * nesExtractor)
	// plexPrice_be = extractorsBase * injectorSell * netMult / (omega + extractorsBase * nesExtractor)
	plexDenominator := float64(nesOmega) + extractorsBase*float64(nesExtractor)
	spFarmBreakEven := safeDiv(extractorsBase*injectorMarketSell*netMult, plexDenominator)

	return SPFarmResult{
		OmegaCostPLEX:      nesOmega,
		OmegaCostISK:       omegaCostISK,
		ExtractorsPerMonth: extractorsBase,
		ExtractorCostPLEX:  nesExtractor,
		ExtractorCostISK:   extractorCostISK,
		TotalCostISK:       totalCostBase,
		InjectorsProduced:  extractorsBase,
		InjectorSellPrice:  injectorMarketSell,
		RevenueISK:         revenueBase,
		ProfitISK:          profitBase,
		ProfitPerDay:       profitBase / 30,
		ROI:                safeDiv(profitBase, totalCostBase) * 100,
		Viable:             profitBase > 0,

		ExtractorsPlus5:   extractorsPlus5,
		ProfitPlus5:       profitPlus5,
		ProfitPerDayPlus5: profitPlus5 / 30,
		ROIPlus5:          safeDiv(profitPlus5, totalCostPlus5) * 100,

		StartupSP:        MinSPForExtraction,
		StartupTrainDays: startupDays,
		StartupCostISK:   startupCostISK,
		PaybackDays:      paybackDays,

		MPTCCostPLEX: nesMPTC,
		MPTCCostISK:  mptcCostISK,

		OmegaISKValue: omegaISKValue,
		PLEXUnitPrice: plexPrice,

		InstantSellRevenueISK: instantRevenueBase,
		InstantSellProfitISK:  instantProfitBase,
		InstantSellROI:        instantROI,

		InstantSellProfitPlus5: instantProfitPlus5,
		InstantSellROIPlus5:    instantROIPlus5,

		BreakEvenPLEX: spFarmBreakEven,
	}
}

// computeIndicators computes technical analysis indicators from PLEX history.
// history must be sorted chronologically (ascending by date).
func computeIndicators(history []esi.HistoryEntry) *PLEXIndicators {
	if len(history) < 7 {
		return nil
	}

	n := len(history)
	prices := make([]float64, n)
	volumes := make([]int64, n)
	for i, e := range history {
		prices[i] = e.Average
		volumes[i] = e.Volume
	}

	ind := &PLEXIndicators{}

	// SMA(7)
	if n >= 7 {
		sum := 0.0
		for i := n - 7; i < n; i++ {
			sum += prices[i]
		}
		ind.SMA7 = sum / 7
	}

	// SMA(30) — require full 30-day window (no partial windows that mislead)
	if n >= 30 {
		sum := 0.0
		for i := n - 30; i < n; i++ {
			sum += prices[i]
		}
		ind.SMA30 = sum / 30
	}

	// Bollinger Bands (20, 2) — two-pass algorithm, population variance (N)
	// Uses population std dev to match TradingView/Bloomberg standard for BB.
	if n >= 20 {
		const bbWindow = 20
		// Pass 1: compute mean
		var bbSum float64
		for i := n - bbWindow; i < n; i++ {
			bbSum += prices[i]
		}
		bbMean := bbSum / float64(bbWindow)
		// Pass 2: sum of squared deviations
		var devSum float64
		for i := n - bbWindow; i < n; i++ {
			d := prices[i] - bbMean
			devSum += d * d
		}
		// Population standard deviation (N) — industry standard for Bollinger Bands
		bbStd := math.Sqrt(devSum / float64(bbWindow))
		ind.BollingerMiddle = bbMean
		ind.BollingerUpper = bbMean + 2*bbStd
		ind.BollingerLower = bbMean - 2*bbStd
	}

	// RSI(14) — Wilder's smoothed RSI (industry standard, used by TradingView/Bloomberg)
	// Requires at least 15 data points (14 for initial seed + 1 for smoothing)
	if n >= 15 {
		const rsiPeriod = 14
		// Step 1: simple average over first 'period' changes (seed)
		var firstGain, firstLoss float64
		for i := 1; i <= rsiPeriod; i++ {
			change := prices[i] - prices[i-1]
			if change > 0 {
				firstGain += change
			} else {
				firstLoss -= change
			}
		}
		avgGain := firstGain / float64(rsiPeriod)
		avgLoss := firstLoss / float64(rsiPeriod)
		// Step 2: Wilder's exponential smoothing for remaining data
		for i := rsiPeriod + 1; i < n; i++ {
			change := prices[i] - prices[i-1]
			gain, loss := 0.0, 0.0
			if change > 0 {
				gain = change
			} else {
				loss = -change
			}
			avgGain = (avgGain*float64(rsiPeriod-1) + gain) / float64(rsiPeriod)
			avgLoss = (avgLoss*float64(rsiPeriod-1) + loss) / float64(rsiPeriod)
		}
		if avgLoss > 0 {
			rs := avgGain / avgLoss
			ind.RSI = 100 - 100/(1+rs)
		} else if avgGain > 0 {
			ind.RSI = 100
		} else {
			ind.RSI = 50 // no movement
		}
	} else {
		ind.RSI = 50 // neutral when insufficient data for proper RSI(14)
	}

	// Price changes
	current := prices[n-1]
	if n >= 2 && prices[n-2] > 0 {
		ind.Change24h = (current - prices[n-2]) / prices[n-2] * 100
	}
	if n >= 7 && prices[n-7] > 0 {
		ind.Change7d = (current - prices[n-7]) / prices[n-7] * 100
	}
	if n >= 30 && prices[n-30] > 0 {
		ind.Change30d = (current - prices[n-30]) / prices[n-30] * 100
	}

	// Volume anomaly (CCP sale detection) — log-normal model
	// Market volume is log-normally distributed; z-scores on raw volume are unreliable.
	// We compute σ in log-space for proper anomaly detection.
	volWindow := min(30, n)
	if volWindow >= 7 {
		logVols := make([]float64, volWindow)
		var logSum, rawSum float64
		for i := 0; i < volWindow; i++ {
			v := float64(volumes[n-volWindow+i])
			rawSum += v
			if v < 1 {
				v = 1 // avoid log(0)
			}
			logVols[i] = math.Log(v)
			logSum += logVols[i]
		}
		logMean := logSum / float64(volWindow)
		ind.AvgVolume30d = rawSum / float64(volWindow)
		ind.VolumeToday = volumes[n-1]

		// Sample standard deviation in log space (N-1)
		var logVarSum float64
		for _, lv := range logVols {
			d := lv - logMean
			logVarSum += d * d
		}
		logStd := math.Sqrt(logVarSum / float64(volWindow-1))

		if logStd > 0 {
			todayLogVol := math.Log(math.Max(float64(volumes[n-1]), 1))
			ind.VolumeSigma = (todayLogVol - logMean) / logStd
		}

		// CCP Sale = volume spike (>2σ in log space) + price drop (>3%)
		ind.CCPSaleSignal = ind.VolumeSigma > 2 && ind.Change24h < -3
	}

	// 20-day annualized volatility (log-return std dev × √365)
	if n >= 21 {
		const volWindow = 20
		logReturns := make([]float64, 0, volWindow)
		for i := n - volWindow; i < n; i++ {
			if prices[i-1] > 0 && prices[i] > 0 {
				logReturns = append(logReturns, math.Log(prices[i]/prices[i-1]))
			}
		}
		if len(logReturns) >= 10 {
			var lrSum float64
			for _, lr := range logReturns {
				lrSum += lr
			}
			lrMean := lrSum / float64(len(logReturns))
			var lrVarSum float64
			for _, lr := range logReturns {
				d := lr - lrMean
				lrVarSum += d * d
			}
			dailyVol := math.Sqrt(lrVarSum / float64(len(logReturns)))
			annualizedVol := dailyVol * math.Sqrt(365) * 100 // percentage
			ind.Volatility20d = annualizedVol
			if annualizedVol < 20 {
				ind.VolRegime = "low"
			} else if annualizedVol < 40 {
				ind.VolRegime = "medium"
			} else {
				ind.VolRegime = "high"
			}
		}
	}

	return ind
}

// computeSignal generates BUY/SELL/HOLD recommendation.
func computeSignal(ind *PLEXIndicators, gp PLEXGlobalPrice) PLEXSignal {
	if ind == nil {
		return PLEXSignal{Action: "HOLD", Confidence: 0, Reasons: []string{"Insufficient data"}}
	}

	buyScore := 0.0
	sellScore := 0.0
	var reasons []string

	// RSI signals
	if ind.RSI < 30 {
		buyScore += 30
		reasons = append(reasons, "RSI oversold (<30)")
	} else if ind.RSI < 40 {
		buyScore += 15
		reasons = append(reasons, "RSI approaching oversold")
	} else if ind.RSI > 70 {
		sellScore += 30
		reasons = append(reasons, "RSI overbought (>70)")
	} else if ind.RSI > 60 {
		sellScore += 15
		reasons = append(reasons, "RSI approaching overbought")
	}

	// Bollinger Band signals
	if ind.BollingerLower > 0 && gp.SellPrice > 0 {
		if gp.SellPrice <= ind.BollingerLower {
			buyScore += 25
			reasons = append(reasons, "Price at lower Bollinger Band")
		} else if gp.SellPrice >= ind.BollingerUpper {
			sellScore += 25
			reasons = append(reasons, "Price at upper Bollinger Band")
		}
	}

	// SMA crossover — PLEX is mean-reverting, so crossovers are strong signals
	if ind.SMA7 > 0 && ind.SMA30 > 0 {
		if ind.SMA7 > ind.SMA30*1.01 {
			sellScore += 20
			reasons = append(reasons, "SMA7 > SMA30 (uptrend)")
		} else if ind.SMA7 < ind.SMA30*0.99 {
			buyScore += 20
			reasons = append(reasons, "SMA7 < SMA30 (downtrend — buy opportunity)")
		}
	}

	// CCP Sale detection (strongest buy signal)
	if ind.CCPSaleSignal {
		buyScore += 40
		reasons = append(reasons, "CCP SALE DETECTED — volume spike + price drop")
	}

	// 7-day momentum
	if ind.Change7d < -5 {
		buyScore += 15
		reasons = append(reasons, "Price dropped >5% in 7 days")
	} else if ind.Change7d > 5 {
		sellScore += 15
		reasons = append(reasons, "Price rose >5% in 7 days")
	}

	// Volatility regime context (informational, not a score contributor)
	switch ind.VolRegime {
	case "low":
		reasons = append(reasons, "Low volatility — NES arbitrage margins stable")
	case "high":
		reasons = append(reasons, "High volatility — spread trading more profitable")
	}

	// Generate signal
	signal := PLEXSignal{}
	totalScore := buyScore + sellScore
	if totalScore == 0 {
		signal.Action = "HOLD"
		signal.Confidence = 50
		signal.Reasons = []string{"No strong signals detected"}
		return signal
	}

	// Normalize confidence: max possible one-side score = 130 (CCP40+RSI30+BB25+SMA20+Mom15)
	const maxScore = 130.0
	if buyScore > sellScore {
		signal.Action = "BUY"
		signal.Confidence = math.Min(buyScore/maxScore*100, 95)
	} else if sellScore > buyScore {
		signal.Action = "SELL"
		signal.Confidence = math.Min(sellScore/maxScore*100, 95)
	} else {
		signal.Action = "HOLD"
		signal.Confidence = 50
	}
	signal.Reasons = reasons
	return signal
}

// computeArbHistory retroactively computes arbitrage profitability from PLEX
// price history. Uses current related item prices as constants (approximation —
// injector/extractor prices also fluctuate, but PLEX is the dominant variable).
func computeArbHistory(
	history []esi.HistoryEntry,
	extractorSell, injectorSell, mptcSell, netMult float64,
	nesExtractor, nesMPTC, nesOmega int,
) *ArbHistoryData {
	if len(history) < 7 || extractorSell <= 0 {
		return nil
	}

	// Keep only last MaxHistoryPoints entries
	start := 0
	if len(history) > MaxHistoryPoints {
		start = len(history) - MaxHistoryPoints
	}

	extNES := make([]ArbHistoryPoint, 0, len(history)-start)
	spNES := make([]ArbHistoryPoint, 0, len(history)-start)
	mptcArr := make([]ArbHistoryPoint, 0, len(history)-start)
	spFarm := make([]ArbHistoryPoint, 0, len(history)-start)

	extRevenue := extractorSell * netMult
	injRevenue := injectorSell * netMult
	mptcRevenue := mptcSell * netMult

	spPerMonth := BaseSPPerHour * HoursPerMonth
	extractorsBase := spPerMonth / float64(SPPerExtractor)

	for i := start; i < len(history); i++ {
		plexPrice := history[i].Average
		if plexPrice <= 0 {
			continue
		}
		date := history[i].Date

		// NES Extractor → Sell
		extCost := float64(nesExtractor) * plexPrice
		extProfit := extRevenue - extCost
		extNES = append(extNES, ArbHistoryPoint{
			Date:      date,
			ProfitISK: extProfit,
			ROI:       safeDiv(extProfit, extCost) * 100,
		})

		// SP Chain (NES Extractor → Injector)
		if injectorSell > 0 {
			spProfit := injRevenue - extCost
			spNES = append(spNES, ArbHistoryPoint{
				Date:      date,
				ProfitISK: spProfit,
				ROI:       safeDiv(spProfit, extCost) * 100,
			})
		}

		// MPTC
		if mptcSell > 0 {
			mptcCost := float64(nesMPTC) * plexPrice
			mptcProfit := mptcRevenue - mptcCost
			mptcArr = append(mptcArr, ArbHistoryPoint{
				Date:      date,
				ProfitISK: mptcProfit,
				ROI:       safeDiv(mptcProfit, mptcCost) * 100,
			})
		}

		// SP Farm monthly profit
		omegaCostISK := float64(nesOmega) * plexPrice
		extractorCostISK := float64(nesExtractor) * plexPrice
		totalCost := omegaCostISK + extractorsBase*extractorCostISK
		revenue := extractorsBase * injectorSell * netMult
		farmProfit := revenue - totalCost
		spFarm = append(spFarm, ArbHistoryPoint{
			Date:      date,
			ProfitISK: farmProfit,
			ROI:       safeDiv(farmProfit, totalCost) * 100,
		})
	}

	return &ArbHistoryData{
		ExtractorNES: extNES,
		SPChainNES:   spNES,
		MPTCNES:      mptcArr,
		SPFarmProfit: spFarm,
	}
}

// computeMarketDepth summarizes order-book depth for items relevant to PLEX arb.
func computeMarketDepth(
	plexOrders []esi.MarketOrder,
	relatedOrders map[int32][]esi.MarketOrder,
	plexHistory []esi.HistoryEntry,
	relatedHistory map[int32][]esi.HistoryEntry,
) *MarketDepthInfo {
	md := &MarketDepthInfo{}

	// PLEX sell depth (top 5 levels)
	md.PLEXSellDepth5 = sellDepthSummary(plexOrders, 5)

	// Extractor
	md.ExtractorSellQty = totalSellVolume(relatedOrders[SkillExtractorTypeID])
	md.ExtractorBuyQty = totalBuyVolume(relatedOrders[SkillExtractorTypeID])

	// Injector
	md.InjectorSellQty = totalSellVolume(relatedOrders[LargeSkillInjTypeID])
	md.InjectorBuyQty = totalBuyVolume(relatedOrders[LargeSkillInjTypeID])

	// MPTC
	md.MPTCSellQty = totalSellVolume(relatedOrders[MPTCTypeID])
	md.MPTCBuyQty = totalBuyVolume(relatedOrders[MPTCTypeID])

	// Time-to-fill estimates (hours to sell quantity based on avg daily volume)
	md.ExtractorFillHours = estimateFillHours(relatedHistory[SkillExtractorTypeID], 1)
	md.InjectorFillHours = estimateFillHours(relatedHistory[LargeSkillInjTypeID], 1)
	md.MPTCFillHours = estimateFillHours(relatedHistory[MPTCTypeID], 1)
	md.PLEXFillHours = estimateFillHours(plexHistory, 100)

	return md
}

// estimateFillHours estimates hours to fill a given quantity based on recent daily volume.
// Uses 7-day average daily volume: fillHours = (quantity / dailyVolume) * 24.
func estimateFillHours(history []esi.HistoryEntry, quantity int) float64 {
	if len(history) == 0 || quantity <= 0 {
		return 0
	}
	// Use last 7 days of history
	n := len(history)
	start := n - 7
	if start < 0 {
		start = 0
	}
	var totalVol int64
	days := 0
	for i := start; i < n; i++ {
		totalVol += history[i].Volume
		days++
	}
	if days == 0 || totalVol == 0 {
		return 0
	}
	dailyVol := float64(totalVol) / float64(days)
	return float64(quantity) / dailyVol * 24
}

func sellDepthSummary(orders []esi.MarketOrder, maxLevels int) DepthSummary {
	// Collect sell orders sorted by price ascending
	type priceVol struct {
		price float64
		vol   int32
	}
	var sells []priceVol
	for _, o := range orders {
		if !o.IsBuyOrder {
			sells = append(sells, priceVol{o.Price, o.VolumeRemain})
		}
	}
	if len(sells) == 0 {
		return DepthSummary{}
	}
	sort.Slice(sells, func(i, j int) bool { return sells[i].price < sells[j].price })

	// Aggregate by distinct price levels
	var totalVol int64
	levels := 0
	bestPrice := sells[0].price
	worstPrice := sells[0].price
	seen := map[float64]bool{}
	for _, s := range sells {
		if !seen[s.price] {
			seen[s.price] = true
			levels++
			if levels > maxLevels {
				break
			}
			worstPrice = s.price
		}
		totalVol += int64(s.vol)
	}

	return DepthSummary{
		TotalVolume: totalVol,
		BestPrice:   bestPrice,
		WorstPrice:  worstPrice,
		Levels:      levels,
	}
}

func totalSellVolume(orders []esi.MarketOrder) int64 {
	var vol int64
	for _, o := range orders {
		if !o.IsBuyOrder {
			vol += int64(o.VolumeRemain)
		}
	}
	return vol
}

func totalBuyVolume(orders []esi.MarketOrder) int64 {
	var vol int64
	for _, o := range orders {
		if o.IsBuyOrder {
			vol += int64(o.VolumeRemain)
		}
	}
	return vol
}

// computeChartOverlays produces pre-computed SMA and Bollinger Band series
// so the frontend doesn't duplicate this math.
func computeChartOverlays(history []esi.HistoryEntry) *ChartOverlays {
	n := len(history)
	if n < 7 {
		return nil
	}

	overlays := &ChartOverlays{}

	// SMA(7)
	if n >= 7 {
		sma := make([]OverlayPoint, 0, n-6)
		var window float64
		for i := 0; i < 7; i++ {
			window += history[i].Average
		}
		sma = append(sma, OverlayPoint{Date: history[6].Date, Value: window / 7})
		for i := 7; i < n; i++ {
			window += history[i].Average - history[i-7].Average
			sma = append(sma, OverlayPoint{Date: history[i].Date, Value: window / 7})
		}
		overlays.SMA7 = sma
	}

	// SMA(30)
	if n >= 30 {
		sma := make([]OverlayPoint, 0, n-29)
		var window float64
		for i := 0; i < 30; i++ {
			window += history[i].Average
		}
		sma = append(sma, OverlayPoint{Date: history[29].Date, Value: window / 30})
		for i := 30; i < n; i++ {
			window += history[i].Average - history[i-30].Average
			sma = append(sma, OverlayPoint{Date: history[i].Date, Value: window / 30})
		}
		overlays.SMA30 = sma
	}

	// Bollinger Bands (20, 2)
	if n >= 20 {
		const bbWindow = 20
		up := make([]OverlayPoint, 0, n-bbWindow+1)
		dn := make([]OverlayPoint, 0, n-bbWindow+1)
		for i := bbWindow - 1; i < n; i++ {
			var sum float64
			for j := i - bbWindow + 1; j <= i; j++ {
				sum += history[j].Average
			}
			mean := sum / float64(bbWindow)
			var devSum float64
			for j := i - bbWindow + 1; j <= i; j++ {
				d := history[j].Average - mean
				devSum += d * d
			}
			std := math.Sqrt(devSum / float64(bbWindow))
			up = append(up, OverlayPoint{Date: history[i].Date, Value: mean + 2*std})
			dn = append(dn, OverlayPoint{Date: history[i].Date, Value: mean - 2*std})
		}
		overlays.BollingerUp = up
		overlays.BollingerDown = dn
	}

	return overlays
}

// ===================================================================
// Helpers
// ===================================================================

func bestSellPrice(orders []esi.MarketOrder) float64 {
	best := math.MaxFloat64
	for _, o := range orders {
		if !o.IsBuyOrder && o.Price < best {
			best = o.Price
		}
	}
	if best == math.MaxFloat64 {
		return 0
	}
	return best
}

func bestBuyPrice(orders []esi.MarketOrder) float64 {
	best := 0.0
	for _, o := range orders {
		if o.IsBuyOrder && o.Price > best {
			best = o.Price
		}
	}
	return best
}

// convertHistory converts sorted HistoryEntry slice to PricePoint slice,
// keeping only the last MaxHistoryPoints entries.
// history must be sorted chronologically (ascending by date).
func convertHistory(history []esi.HistoryEntry) []PricePoint {
	if len(history) == 0 {
		return nil
	}

	start := 0
	if len(history) > MaxHistoryPoints {
		start = len(history) - MaxHistoryPoints
	}

	result := make([]PricePoint, 0, len(history)-start)
	for i := start; i < len(history); i++ {
		e := history[i]
		result = append(result, PricePoint{
			Date:    e.Date,
			Average: e.Average,
			High:    e.Highest,
			Low:     e.Lowest,
			Volume:  e.Volume,
		})
	}
	return result
}

// sortHistory returns a copy of history sorted chronologically (ascending date).
func sortHistory(entries []esi.HistoryEntry) []esi.HistoryEntry {
	if len(entries) == 0 {
		return nil
	}
	sorted := make([]esi.HistoryEntry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Date < sorted[j].Date
	})
	return sorted
}

func safeDiv(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	v := a / b
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

// Cross-hub region IDs and names for arbitrage comparison.
var crossHubRegions = []struct {
	RegionID int32
	Name     string
}{
	{10000002, "Jita"},
	{10000043, "Amarr"},
	{10000032, "Dodixie"},
	{10000030, "Rens"},
}

// crossHubItems maps type IDs to item names for cross-hub comparison.
var crossHubItems = []struct {
	TypeID int32
	Name   string
}{
	{SkillExtractorTypeID, "Skill Extractor"},
	{LargeSkillInjTypeID, "Large Skill Injector"},
	{MPTCTypeID, "MPTC"},
}

// computeCrossHubArbitrage compares sell prices across 4 hubs for SP-related items.
// crossHubOrders: typeID → regionID → orders
func computeCrossHubArbitrage(crossHubOrders map[int32]map[int32][]esi.MarketOrder, netMult float64) []CrossHubArbitrage {
	var results []CrossHubArbitrage

	for _, item := range crossHubItems {
		regionOrders, ok := crossHubOrders[item.TypeID]
		if !ok || len(regionOrders) == 0 {
			continue
		}

		// Find best sell price per hub
		jitaPrice := bestSellPrice(regionOrders[JitaRegionID])
		if jitaPrice <= 0 {
			continue
		}

		bestPrice := jitaPrice
		bestHub := "Jita"
		for _, hub := range crossHubRegions {
			sell := bestSellPrice(regionOrders[hub.RegionID])
			if sell > 0 && sell < bestPrice {
				bestPrice = sell
				bestHub = hub.Name
			}
		}

		diffPct := safeDiv(jitaPrice-bestPrice, bestPrice) * 100
		profitISK := (jitaPrice*netMult - bestPrice) // buy cheap, sell in Jita after fees
		viable := diffPct > 1.0 && profitISK > 0

		results = append(results, CrossHubArbitrage{
			ItemName:  item.Name,
			TypeID:    item.TypeID,
			BestHub:   bestHub,
			BestPrice: bestPrice,
			JitaPrice: jitaPrice,
			DiffPct:   diffPct,
			ProfitISK: profitISK,
			Viable:    viable,
		})
	}

	return results
}
