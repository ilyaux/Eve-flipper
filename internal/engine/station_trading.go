package engine

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"sync/atomic"

	"eve-flipper/internal/esi"
)

const (
	stationFlowWindowDays       = 7
	stationVWAPWindowDays       = 30
	stationVolatilityWindowDays = 30
	// Cap returned station rows to keep hosted UI responsive on large hubs.
	maxStationReturnedResults = 1500
)

// StationTrade represents a same-station flip opportunity (buy via buy order, sell via sell order).
type StationTrade struct {
	TypeID         int32   `json:"TypeID"`
	TypeName       string  `json:"TypeName"`
	Volume         float64 `json:"Volume"`
	BuyPrice       float64 `json:"BuyPrice"`  // highest buy order price (we sell to this)
	SellPrice      float64 `json:"SellPrice"` // lowest sell order price (we buy from this)
	Spread         float64 `json:"Spread"`    // SellPrice - BuyPrice
	MarginPercent  float64 `json:"MarginPercent"`
	ProfitPerUnit  float64 `json:"ProfitPerUnit"`
	DailyVolume    int64   `json:"DailyVolume"`
	BuyOrderCount  int     `json:"BuyOrderCount"`
	SellOrderCount int     `json:"SellOrderCount"`
	BuyVolume      int64   `json:"BuyVolume"`  // total volume of buy orders
	SellVolume     int64   `json:"SellVolume"` // total volume of sell orders
	TotalProfit    float64 `json:"TotalProfit"`
	DailyProfit    float64 `json:"DailyProfit"` // estimated executable daily profit
	// TheoreticalDailyProfit is spread-only maker estimate (before execution realism).
	TheoreticalDailyProfit float64 `json:"TheoreticalDailyProfit,omitempty"`
	// RealizableDailyProfit is conservative realizable estimate used for KPI.
	RealizableDailyProfit float64 `json:"RealizableDailyProfit,omitempty"`
	// Confidence score for the station-trading signal quality (0-100).
	ConfidenceScore float64 `json:"ConfidenceScore,omitempty"`
	// ConfidenceLabel buckets confidence score into low|medium|high.
	ConfidenceLabel string `json:"ConfidenceLabel,omitempty"`
	// True when depth-based execution model found positive executable quantity.
	HasExecutionEvidence bool `json:"HasExecutionEvidence,omitempty"`
	// Execution-aware effective margin after slippage and fees.
	RealMarginPercent float64 `json:"RealMarginPercent,omitempty"`
	// True when market history for this type/region was fetched successfully.
	HistoryAvailable bool    `json:"HistoryAvailable"`
	ROI              float64 `json:"ROI"` // profit / investment * 100
	StationName      string  `json:"StationName"`
	StationID        int64   `json:"StationID"`

	// --- EVE Guru style metrics ---
	CapitalRequired float64 `json:"CapitalRequired"` // Cycle capital: effectiveBuy * tradableUnits
	NowROI          float64 `json:"NowROI"`          // Execution-aware ROI (slippage-aware when available; falls back to margin)
	PeriodROI       float64 `json:"PeriodROI"`       // (AvgSell - AvgBuy) / AvgBuy * 100

	// Volume/Liquidity metrics
	BuyUnitsPerDay  float64 `json:"BuyUnitsPerDay"`  // History volume / days
	SellUnitsPerDay float64 `json:"SellUnitsPerDay"` // Estimated from order counts
	BvSRatio        float64 `json:"BvSRatio"`        // BuyUnitsPerDay / SellUnitsPerDay
	DOS             float64 `json:"DOS"`             // Days of Supply = SellVolume / BuyUnitsPerDay
	S2BPerDay       float64 `json:"S2BPerDay"`       // Alias: sells to buy orders per day
	BfSPerDay       float64 `json:"BfSPerDay"`       // Alias: buys from sell orders per day
	S2BBfSRatio     float64 `json:"S2BBfSRatio"`     // Alias: S2BPerDay / BfSPerDay

	// Advanced risk metrics
	VWAP float64 `json:"VWAP"` // Volume-Weighted Average Price (30 days)
	PVI  float64 `json:"PVI"`  // DRVI: Daily Range Volatility Index (StdDev of daily range %). JSON tag kept as "PVI" for backward compat.
	OBDS float64 `json:"OBDS"` // Order Book Depth Score
	SDS  int     `json:"SDS"`  // Scam Detection Score (0-100)
	CI   int     `json:"CI"`   // Competition Index
	CTS  float64 `json:"CTS"`  // Composite Trading Score (final rating 0-100)

	// Price history
	AvgPrice  float64 `json:"AvgPrice"`  // Average price over period
	PriceHigh float64 `json:"PriceHigh"` // Max price over period
	PriceLow  float64 `json:"PriceLow"`  // Min price over period

	// Risk flags
	IsExtremePriceFlag bool `json:"IsExtremePriceFlag"` // Anomalous price detected
	IsHighRiskFlag     bool `json:"IsHighRiskFlag"`     // SDS >= 50

	// Execution-plan derived (expected fill prices from order book depth)
	ExpectedBuyPrice  float64 `json:"ExpectedBuyPrice,omitempty"`
	ExpectedSellPrice float64 `json:"ExpectedSellPrice,omitempty"`
	ExpectedProfit    float64 `json:"ExpectedProfit,omitempty"` // expected net profit per unit
	RealProfit        float64 `json:"RealProfit,omitempty"`     // expected net profit for target quantity
	FilledQty         int32   `json:"FilledQty,omitempty"`      // executable profitable quantity
	CanFill           bool    `json:"CanFill"`                  // whether target quantity is fully fillable
	SlippageBuyPct    float64 `json:"SlippageBuyPct,omitempty"`
	SlippageSellPct   float64 `json:"SlippageSellPct,omitempty"`
}

// stationSortProxy returns a pre-history ranking score for a StationTrade.
// It penalises extreme margins (likely scam / junk) and rewards items that
// have many competing orders (indicating a real market).
func stationSortProxy(r *StationTrade) float64 {
	// Cap margin contribution at 50% so extreme-margin items don't dominate.
	cappedMargin := r.MarginPercent
	if cappedMargin > 50 {
		cappedMargin = 50
	}
	// Volume proxy: minimum of buy/sell volume avoids one-sided scam books.
	minVol := float64(r.BuyVolume)
	if float64(r.SellVolume) < minVol {
		minVol = float64(r.SellVolume)
	}
	// Order count bonus: more competing orders → more likely a real market.
	orderBonus := math.Log2(float64(r.BuyOrderCount+r.SellOrderCount) + 1)
	return cappedMargin * minVol * orderBonus
}

// stationTypeKey uniquely identifies a station+type combination for order grouping.
type stationTypeKey struct {
	locationID int64
	typeID     int32
}

// orderGroup holds buy and sell orders for a single station+type combination.
type orderGroup struct {
	buyOrders  []esi.MarketOrder
	sellOrders []esi.MarketOrder
}

func minInt32(a, b int32) int32 {
	if a < b {
		return a
	}
	return b
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func stationConfidenceLabel(score float64) string {
	switch {
	case score >= 75:
		return "high"
	case score >= 50:
		return "medium"
	default:
		return "low"
	}
}

// stationConfidenceScore computes confidence of a station-trading signal.
// It is intentionally orthogonal to profitability: a trade may be profitable
// but still low-confidence due to thin book/manipulation/volatility cues.
func stationConfidenceScore(row *StationTrade, flowPerDay float64, hasExecutionEvidence bool) float64 {
	if row == nil {
		return 0
	}

	score := 0.0
	if row.HistoryAvailable {
		score += 20
	}

	// Liquidity / depth quality.
	score += 20 * normalize(row.OBDS, 0, 1)
	// Lower manipulation risk is better.
	score += 20 * (1 - normalize(float64(row.SDS), 0, 100))
	// Lower volatility is better for maker fills.
	score += 15 * (1 - normalize(row.PVI, 0, 50))

	// Throughput quality (log-scaled).
	if flowPerDay > 1 {
		score += 10 * normalize(math.Log10(flowPerDay), 0, 4)
	}

	// Balanced two-sided flow is more reliable than one-sided pressure.
	balanceScore := 0.25 // weak prior if side flow unavailable
	if row.S2BPerDay > 0 && row.BfSPerDay > 0 {
		ratio := row.S2BPerDay / row.BfSPerDay
		if ratio > 0 {
			balanceScore = 1 - normalize(math.Abs(math.Log(ratio)), 0, 1.5)
		}
	}
	score += 10 * clamp01(balanceScore)

	if hasExecutionEvidence {
		score += 5
	}
	if row.IsExtremePriceFlag {
		score -= 10
	}
	if row.IsHighRiskFlag {
		score -= 10
	}

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return sanitizeFloat(score)
}

// stationMakerFallbackRealizationFactor maps confidence + competition into a
// conservative realizability factor for maker-mode daily PnL when direct
// execution evidence is not available.
func stationMakerFallbackRealizationFactor(confidenceScore float64, competitionIndex int) float64 {
	conf := normalize(confidenceScore, 0, 100) // 0..1
	base := 0.45 + 0.45*conf                   // 0.45..0.90

	ci := maxInt(0, competitionIndex)
	queuePenalty := 1.0 / math.Sqrt(float64(ci+1))
	if queuePenalty < 0.40 {
		queuePenalty = 0.40
	}
	if queuePenalty > 1.0 {
		queuePenalty = 1.0
	}

	factor := base * queuePenalty
	if factor < 0.20 {
		factor = 0.20
	}
	if factor > 0.90 {
		factor = 0.90
	}
	return sanitizeFloat(factor)
}

// estimateSellUnitsPerDay derives supply-side daily throughput from recent traded
// volume and current book imbalance. This keeps BvS mathematically symmetric:
// BvS = BuyUnitsPerDay / SellUnitsPerDay ~= BuyVolume / SellVolume.
func estimateSellUnitsPerDay(dailyVolume float64, buyVolume, sellVolume int64) float64 {
	if dailyVolume <= 0 || buyVolume <= 0 || sellVolume <= 0 {
		return 0
	}
	return dailyVolume * float64(sellVolume) / float64(buyVolume)
}

// stationExecutionDesiredQty picks the quantity for execution simulation.
// If daily share is known, we target that share capped by current depth.
// Otherwise we fall back to a bounded probe size to avoid over-weighting huge books.
func stationExecutionDesiredQty(dailyShare int64, buyVolume, sellVolume int64) int32 {
	depthCap := minInt64(buyVolume, sellVolume)
	if depthCap <= 0 {
		return 0
	}
	if dailyShare > 0 {
		if dailyShare > depthCap {
			if depthCap > math.MaxInt32 {
				return math.MaxInt32
			}
			return int32(depthCap)
		}
		if dailyShare > math.MaxInt32 {
			return math.MaxInt32
		}
		return int32(dailyShare)
	}
	const fallbackQty int64 = 1000
	if depthCap < fallbackQty {
		if depthCap > math.MaxInt32 {
			return math.MaxInt32
		}
		return int32(depthCap)
	}
	return int32(fallbackQty)
}

// stationExecutionDesiredQtyFromDailyShare is strict daily-throughput mode:
// if daily share is unknown/non-positive, we do not assume synthetic fallback qty.
func stationExecutionDesiredQtyFromDailyShare(dailyShare int64, buyVolume, sellVolume int64) int32 {
	if dailyShare <= 0 {
		return 0
	}
	return stationExecutionDesiredQty(dailyShare, buyVolume, sellVolume)
}

func stationFlowPerDay(entries []esi.HistoryEntry) float64 {
	return avgDailyVolume(entries, stationFlowWindowDays)
}

// resetExecutionDerivedFields clears depth-simulation outputs and keeps maker
// fallback ROI consistent with baseline spread economics.
func resetExecutionDerivedFields(r *StationTrade) {
	r.ExpectedBuyPrice = 0
	r.ExpectedSellPrice = 0
	r.ExpectedProfit = 0
	r.RealProfit = 0
	r.FilledQty = 0
	r.CanFill = false
	r.SlippageBuyPct = 0
	r.SlippageSellPct = 0
	r.RealMarginPercent = 0
	r.HasExecutionEvidence = false
	r.NowROI = sanitizeFloat(r.MarginPercent)
}

// StationTradeParams holds input parameters for station trading scan.
type StationTradeParams struct {
	StationIDs      map[int64]bool // nil or empty = all stations in region
	RegionID        int32
	MinMargin       float64
	SalesTaxPercent float64
	BrokerFee       float64 // percent
	CTSProfile      string  // balanced|aggressive|defensive
	// SplitTradeFees enables side-specific fee model.
	// When false, legacy fields above are used.
	SplitTradeFees       bool
	BuyBrokerFeePercent  float64
	SellBrokerFeePercent float64
	BuySalesTaxPercent   float64
	SellSalesTaxPercent  float64
	MinDailyVolume       int64 // 0 = no filter

	// --- EVE Guru Profit Filters ---
	MinItemProfit   float64 // Min profit per unit ISK (e.g. 1,000,000)
	MinDemandPerDay float64 // Legacy alias for MinS2BPerDay
	MinS2BPerDay    float64 // Min daily S2B flow
	MinBfSPerDay    float64 // Min daily BfS flow

	// --- Risk Profile ---
	AvgPricePeriod int     // Days for Period ROI calc (default 90)
	MinPeriodROI   float64 // Min Period ROI % (e.g. 20%)
	BvSRatioMin    float64 // Min B v S Ratio (e.g. 0.5)
	BvSRatioMax    float64 // Max B v S Ratio (e.g. 2.0)
	MaxPVI         float64 // Max volatility % (e.g. 25%)
	MaxSDS         int     // Max scam score (e.g. 40)

	// --- Price Limits ---
	LimitBuyToPriceLow bool // Don't buy above P.Low + 10%
	FlagExtremePrices  bool // Flag anomalous prices

	// --- Authentication ---
	AccessToken string // For resolving player structure names (optional)
}

// ScanStationTrades finds profitable same-station trading opportunities.
// isPlayerStructureID checks if a location ID belongs to a player-owned structure.
// NPC stations: 60,000,000 – 64,000,000. Player structures (Upwell): > 1,000,000,000,000.
func isPlayerStructureID(id int64) bool {
	return id > 1_000_000_000_000
}

func (s *Scanner) ScanStationTrades(params StationTradeParams, progress func(string)) ([]StationTrade, error) {
	progress("Fetching all region orders...")

	// Fetch all orders for the region
	allOrders, err := s.ESI.FetchRegionOrders(params.RegionID, "all")
	if err != nil {
		return nil, fmt.Errorf("fetch orders: %w", err)
	}

	progress(fmt.Sprintf("Processing %d orders...", len(allOrders)))

	// Group orders by (locationID, typeID) — supports multi-station scan
	groups := make(map[stationTypeKey]*orderGroup)
	// Depth denominator for station-share scaling, scoped to the same
	// order universe we actually scan (respects StationIDs/structure mode).
	regionDepthByType := make(map[int32]int64)

	filterStations := len(params.StationIDs) > 0

	for _, o := range allOrders {
		// Filter to allowed stations (if specified)
		if filterStations {
			if _, ok := params.StationIDs[o.LocationID]; !ok {
				continue
			}
		} else {
			// In "all stations in region" mode, skip player structures
			// unless they were explicitly included via StationIDs.
			// This prevents processing hundreds of thousands of structure
			// orders that would be filtered out later anyway.
			if isPlayerStructureID(o.LocationID) {
				continue
			}
		}
		regionDepthByType[o.TypeID] += int64(o.VolumeRemain)
		key := stationTypeKey{o.LocationID, o.TypeID}
		g, ok := groups[key]
		if !ok {
			g = &orderGroup{}
			groups[key] = g
		}
		if o.IsBuyOrder {
			g.buyOrders = append(g.buyOrders, o)
		} else {
			g.sellOrders = append(g.sellOrders, o)
		}
	}

	log.Printf("[DEBUG] StationTrades: %d type+station groups", len(groups))

	progress(fmt.Sprintf("Analyzing %d items...", len(groups)))

	buyCostMult, sellRevenueMult := tradeFeeMultipliers(tradeFeeInputs{
		SplitTradeFees:       params.SplitTradeFees,
		BrokerFeePercent:     params.BrokerFee,
		SalesTaxPercent:      params.SalesTaxPercent,
		BuyBrokerFeePercent:  params.BuyBrokerFeePercent,
		SellBrokerFeePercent: params.SellBrokerFeePercent,
		BuySalesTaxPercent:   params.BuySalesTaxPercent,
		SellSalesTaxPercent:  params.SellSalesTaxPercent,
	})

	var results []StationTrade
	// Store order groups for advanced metrics calculation
	orderGroups := make(map[stationTypeKey]*orderGroup)

	for key, g := range groups {
		typeID := key.typeID
		if len(g.buyOrders) == 0 || len(g.sellOrders) == 0 {
			continue
		}

		// Find highest buy and lowest sell
		var highestBuy esi.MarketOrder
		for _, o := range g.buyOrders {
			if o.Price > highestBuy.Price {
				highestBuy = o
			}
		}

		var lowestSell esi.MarketOrder
		lowestSell.Price = math.MaxFloat64
		for _, o := range g.sellOrders {
			if o.Price < lowestSell.Price {
				lowestSell = o
			}
		}

		if highestBuy.Price <= 0.01 || lowestSell.Price >= math.MaxFloat64 {
			continue
		}

		// Skip absurd spreads — if bid is less than 1% of ask, junk
		if highestBuy.Price < lowestSell.Price*0.01 {
			continue
		}

		// Station trading = market making: we PLACE a buy order (at bid) and a sell order (at ask).
		// When our buy is hit we pay the bid; when our sell is hit we receive the ask.
		// Profit = spread (ask - bid) minus fees. We need ask > bid (always true) and spread > fees.
		costToBuy := highestBuy.Price       // we place our buy at bid; when filled we pay this
		revenueFromSell := lowestSell.Price // we place our sell at ask; when filled we receive this
		if revenueFromSell <= costToBuy {
			continue // no spread
		}
		effectiveBuy := costToBuy * buyCostMult
		effectiveSell := revenueFromSell * sellRevenueMult
		profitPerUnit := effectiveSell - effectiveBuy

		if profitPerUnit <= 0 {
			continue
		}

		margin := profitPerUnit / effectiveBuy * 100
		if margin < params.MinMargin {
			continue
		}

		itemType, ok := s.SDE.Types[typeID]
		if !ok {
			continue
		}

		// Total volumes (int64 to avoid overflow on high-liquidity items)
		var totalBuyVol, totalSellVol int64
		for _, o := range g.buyOrders {
			totalBuyVol += int64(o.VolumeRemain)
		}
		for _, o := range g.sellOrders {
			totalSellVol += int64(o.VolumeRemain)
		}

		if totalBuyVol <= 0 || totalSellVol <= 0 {
			continue
		}

		// Pre-filter by MinItemProfit
		if params.MinItemProfit > 0 && profitPerUnit < params.MinItemProfit {
			continue
		}

		// Calculate order book metrics
		// OBDS denominator should reflect actionable cycle capital, not full
		// long-tail book not touched by this strategy.
		tradableUnits := minInt64(totalBuyVol, totalSellVol)
		// Cycle capital: ISK required to place the buy side of the trade
		// for all tradable units (minimum of buy/sell depth).
		capitalRequired := effectiveBuy * float64(tradableUnits)
		// Keep OBDS denominator in raw order-book ISK units (same unit as depth).
		obdsCapital := costToBuy * float64(tradableUnits)
		if obdsCapital <= 0 {
			obdsCapital = effectiveBuy // minimal non-zero fallback
		}
		ci := CalcCI(append(g.buyOrders, g.sellOrders...))
		obds := CalcOBDS(g.buyOrders, g.sellOrders, obdsCapital)

		results = append(results, StationTrade{
			TypeID:          typeID,
			TypeName:        itemType.Name,
			Volume:          itemType.Volume,
			BuyPrice:        costToBuy,                   // highest buy (we place our buy here; when filled we pay bid)
			SellPrice:       revenueFromSell,             // lowest sell (we place our sell here; when filled we receive ask)
			Spread:          revenueFromSell - costToBuy, // ask - bid
			MarginPercent:   sanitizeFloat(margin),
			ProfitPerUnit:   sanitizeFloat(profitPerUnit),
			BuyOrderCount:   len(g.buyOrders),
			SellOrderCount:  len(g.sellOrders),
			BuyVolume:       totalBuyVol,
			SellVolume:      totalSellVol,
			ROI:             sanitizeFloat(margin),
			StationID:       key.locationID,
			CapitalRequired: sanitizeFloat(capitalRequired),
			NowROI:          sanitizeFloat(margin), // initial fallback; refined from execution plans below
			CI:              ci,
			OBDS:            sanitizeFloat(obds),
			// History-dependent fields will be calculated in enrichStationWithHistory
		})

		// Store order groups for advanced metrics (needed for SDS calculation)
		orderGroups[key] = g
	}

	log.Printf("[DEBUG] StationTrades: %d profitable items", len(results))

	// Sort by a proxy that balances profit potential with legitimacy.
	// Pure "ProfitPerUnit * OrderVolume" heavily favours scam/junk items with
	// 1000%+ margins and large idle order volumes but zero actual trades.
	// We cap the margin contribution and add an order-count bonus so items
	// with many competing orders (= real market) are ranked higher.
	sort.Slice(results, func(i, j int) bool {
		return stationSortProxy(&results[i]) > stationSortProxy(&results[j])
	})

	// Cap internal working set for history enrichment to prevent server overload
	if len(results) > MaxUnlimitedResults {
		excluded := len(results) - MaxUnlimitedResults
		results = results[:MaxUnlimitedResults]
		progress(fmt.Sprintf("Capped to %d items for enrichment (%d excluded by proxy rank)", MaxUnlimitedResults, excluded))
	}

	// Initial expected fill prices from execution plan — per-unit signal.
	// Final daily executable PnL is recalculated after history enrichment using
	// stationExecutionDesiredQty(dailyShare, ...).
	for i := range results {
		r := &results[i]
		key := stationTypeKey{r.StationID, r.TypeID}
		if g, ok := orderGroups[key]; ok {
			qty := stationExecutionDesiredQty(0, r.BuyVolume, r.SellVolume)
			if qty > 0 {
				planBuy := ComputeExecutionPlan(g.sellOrders, qty, true)
				planSell := ComputeExecutionPlan(g.buyOrders, qty, false)
				r.ExpectedBuyPrice = planBuy.ExpectedPrice
				r.ExpectedSellPrice = planSell.ExpectedPrice
				r.SlippageBuyPct = planBuy.SlippagePercent
				r.SlippageSellPct = planSell.SlippagePercent
				if r.ExpectedBuyPrice > 0 && r.ExpectedSellPrice > 0 {
					// Account for configured buy/sell-side fees.
					effectiveBuy := r.ExpectedBuyPrice * buyCostMult
					effectiveSell := r.ExpectedSellPrice * sellRevenueMult
					r.ExpectedProfit = effectiveSell - effectiveBuy // per unit, net of fees
					if effectiveBuy > 0 {
						// NowROI is execution-aware current ROI from live depth.
						r.NowROI = sanitizeFloat((r.ExpectedProfit / effectiveBuy) * 100)
					}
				}
			}
		}
	}

	// Fill station names (prefetch NPC stations and player structures separately)
	if len(results) > 0 {
		progress("Fetching station names...")
		npcStationIDs := make(map[int64]bool)
		structureIDs := make(map[int64]bool)

		// Separate NPC stations from player structures
		for _, r := range results {
			if isPlayerStructureID(r.StationID) {
				structureIDs[r.StationID] = true
			} else {
				npcStationIDs[r.StationID] = true
			}
		}

		// Prefetch NPC station names
		if len(npcStationIDs) > 0 {
			s.ESI.PrefetchStationNames(npcStationIDs)
		}

		// Prefetch player structure names (requires auth token)
		if len(structureIDs) > 0 && params.AccessToken != "" {
			s.ESI.PrefetchStructureNames(structureIDs, params.AccessToken)
		}

		// Resolve all station names and filter out inaccessible structures
		filtered := make([]StationTrade, 0, len(results))
		skippedCount := 0
		for i := range results {
			results[i].StationName = s.ESI.StationName(results[i].StationID)
			// Skip player structures that couldn't be resolved (no access + not in EVERef)
			if isPlayerStructureID(results[i].StationID) &&
				(results[i].StationName == "" ||
					strings.HasPrefix(results[i].StationName, "Structure ") ||
					strings.HasPrefix(results[i].StationName, "Location ")) {
				skippedCount++
				continue
			}
			filtered = append(filtered, results[i])
		}
		results = filtered
		if skippedCount > 0 {
			log.Printf("[DEBUG] Skipped %d inaccessible player structures", skippedCount)
			progress(fmt.Sprintf("⚠️ Skipped %d private/inaccessible structures", skippedCount))
		}
	}

	// Enrich with market history and calculate advanced metrics
	s.enrichStationWithHistory(results, params.RegionID, orderGroups, params, regionDepthByType, progress)

	// Apply post-history filters
	results = applyStationTradeFilters(results, params)

	log.Printf("[DEBUG] StationTrades: %d after all filters", len(results))

	// Final sort by CTS (Composite Trading Score) descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].CTS > results[j].CTS
	})
	if len(results) > maxStationReturnedResults {
		results = results[:maxStationReturnedResults]
		progress(fmt.Sprintf("Capped station results to top %d by CTS", maxStationReturnedResults))
	}

	if replaced := atomic.SwapInt64(&sanitizeFloatCount, 0); replaced > 0 {
		log.Printf("[WARN] sanitizeFloat replaced %d NaN/Inf values during station scan", replaced)
	}

	progress(fmt.Sprintf("Found %d station trading opportunities", len(results)))
	return results, nil
}

// applyStationTradeFilters applies post-history filters based on params.
func applyStationTradeFilters(results []StationTrade, params StationTradeParams) []StationTrade {
	filtered := make([]StationTrade, 0, len(results))
	minS2B := params.MinS2BPerDay
	if params.MinDemandPerDay > minS2B {
		minS2B = params.MinDemandPerDay
	}
	needsHistory := params.MinDailyVolume > 0 ||
		minS2B > 0 ||
		params.MinBfSPerDay > 0 ||
		params.MinPeriodROI > 0 ||
		params.BvSRatioMin > 0 ||
		params.BvSRatioMax > 0 ||
		params.MaxPVI > 0 ||
		params.MaxSDS > 0 ||
		params.LimitBuyToPriceLow

	// Debug counters
	var dropExecution, dropHistory, dropMargin, dropItemProfit, dropVol, dropS2B, dropBfS, dropROI, dropBvS, dropPVI, dropSDS, dropPrice int

	for _, r := range results {
		// Station trading is a maker strategy (buy at bid, sell at ask). If
		// taker-style depth simulation yields zero safe qty, keep baseline maker
		// economics instead of dropping the opportunity outright.
		// Defensive guard against inconsistent execution payloads.
		if r.FilledQty > 0 && r.RealProfit <= 0 {
			dropExecution++
			continue
		}
		if needsHistory && !r.HistoryAvailable {
			dropHistory++
			continue
		}
		// Enforce execution-aware margin threshold.
		effectiveMargin := r.MarginPercent
		if r.FilledQty > 0 {
			effectiveMargin = r.RealMarginPercent
		}
		if params.MinMargin > 0 && effectiveMargin < params.MinMargin {
			dropMargin++
			continue
		}
		// Re-validate min item profit on execution-aware economics.
		if params.MinItemProfit > 0 {
			profitPerUnit := r.ProfitPerUnit
			if r.FilledQty > 0 {
				profitPerUnit = r.RealProfit / float64(r.FilledQty)
			}
			if profitPerUnit < params.MinItemProfit {
				dropItemProfit++
				continue
			}
		}
		// Min daily volume
		if params.MinDailyVolume > 0 && r.DailyVolume < params.MinDailyVolume {
			dropVol++
			continue
		}
		// Min S2B/day (legacy: MinDemandPerDay)
		if minS2B > 0 && r.S2BPerDay < minS2B {
			dropS2B++
			continue
		}
		// Min BfS/day
		if params.MinBfSPerDay > 0 && r.BfSPerDay < params.MinBfSPerDay {
			dropBfS++
			continue
		}
		// Min Period ROI
		if params.MinPeriodROI > 0 && r.PeriodROI < params.MinPeriodROI {
			dropROI++
			continue
		}
		// S2B/BfS ratio range
		if params.BvSRatioMin > 0 && r.S2BBfSRatio < params.BvSRatioMin {
			dropBvS++
			continue
		}
		if params.BvSRatioMax > 0 && r.S2BBfSRatio > params.BvSRatioMax {
			dropBvS++
			continue
		}
		// Max PVI (volatility)
		if params.MaxPVI > 0 && r.PVI > params.MaxPVI {
			dropPVI++
			continue
		}
		// Max SDS (scam score)
		if params.MaxSDS > 0 && r.SDS > params.MaxSDS {
			dropSDS++
			continue
		}
		// Price limit filter: don't place buy order above historical low + 10%
		if params.LimitBuyToPriceLow && r.PriceLow > 0 {
			maxBuyPrice := r.PriceLow * 1.1
			if r.BuyPrice > maxBuyPrice {
				dropPrice++
				continue
			}
		}
		filtered = append(filtered, r)
	}

	if len(results) != len(filtered) {
		log.Printf("[DEBUG] StationFilter drops: execution=%d history=%d margin=%d item_profit=%d vol=%d s2b=%d bfs=%d roi=%d bvs=%d pvi=%d sds=%d price=%d",
			dropExecution, dropHistory, dropMargin, dropItemProfit, dropVol, dropS2B, dropBfS, dropROI, dropBvS, dropPVI, dropSDS, dropPrice)
	}

	return filtered
}

// enrichStationWithHistory fetches market history and calculates advanced metrics.
// regionDepthByType holds region-wide order depth per typeID for station share estimation.
func (s *Scanner) enrichStationWithHistory(results []StationTrade, regionID int32, orderGroups map[stationTypeKey]*orderGroup, params StationTradeParams, regionDepthByType map[int32]int64, progress func(string)) {
	if s.History == nil || len(results) == 0 {
		return
	}

	progress("Fetching market history...")

	// Determine period for calculations (default 90 days)
	avgPeriod := params.AvgPricePeriod
	if avgPeriod <= 0 {
		avgPeriod = 90
	}
	ctsWeights := CTSWeightsForProfile(params.CTSProfile)
	buyCostMult, sellRevenueMult := tradeFeeMultipliers(tradeFeeInputs{
		SplitTradeFees:       params.SplitTradeFees,
		BrokerFeePercent:     params.BrokerFee,
		SalesTaxPercent:      params.SalesTaxPercent,
		BuyBrokerFeePercent:  params.BuyBrokerFeePercent,
		SellBrokerFeePercent: params.SellBrokerFeePercent,
		BuySalesTaxPercent:   params.BuySalesTaxPercent,
		SellSalesTaxPercent:  params.SellSalesTaxPercent,
	})

	// Deduplicate history fetches by typeID (all results share the same regionID).
	// This prevents N+1 / thundering-herd when multiple station+type rows map
	// to the same (region, type) history key.
	type historyData struct {
		entries          []esi.HistoryEntry
		historyAvailable bool
	}
	histByType := make(map[int32]*historyData)
	{
		// Collect unique typeIDs
		uniqueTypes := make(map[int32]bool, len(results))
		for i := range results {
			uniqueTypes[results[i].TypeID] = true
		}
		type fetchResult struct {
			typeID int32
			data   historyData
		}
		fetchCh := make(chan fetchResult, len(uniqueTypes))
		sem := make(chan struct{}, 20)
		for tid := range uniqueTypes {
			sem <- struct{}{}
			go func(typeID int32) {
				defer func() { <-sem }()
				entries, ok := s.History.GetMarketHistory(regionID, typeID)
				if !ok {
					var err error
					entries, err = s.ESI.FetchMarketHistory(regionID, typeID)
					if err != nil {
						fetchCh <- fetchResult{typeID, historyData{}}
						return
					}
					s.History.SetMarketHistory(regionID, typeID, entries)
				}
				fetchCh <- fetchResult{typeID, historyData{entries: entries, historyAvailable: len(entries) > 0}}
			}(tid)
		}
		for range uniqueTypes {
			r := <-fetchCh
			histByType[r.typeID] = &r.data
		}
	}

	progress("Calculating advanced metrics...")

	for idx := range results {
		hd := histByType[results[idx].TypeID]
		if hd == nil {
			hd = &historyData{}
		}

		results[idx].HistoryAvailable = hd.historyAvailable
		resetExecutionDerivedFields(&results[idx])
		if len(hd.entries) == 0 {
			results[idx].DailyVolume = 0
			results[idx].TheoreticalDailyProfit = 0
			results[idx].RealizableDailyProfit = 0
			results[idx].DailyProfit = 0
			results[idx].TotalProfit = 0
			results[idx].ConfidenceScore = 0
			results[idx].ConfidenceLabel = stationConfidenceLabel(0)
			continue
		}

		// Use one consistent flow window for all throughput-dependent metrics.
		regionFlowPerDay := stationFlowPerDay(hd.entries)

		// Scale region-level flow to station-level using order depth share.
		// ESI history is region-wide; without this adjustment, non-hub stations
		// get inflated volume/profit estimates.
		stationShare := 1.0
		stationDepth := results[idx].BuyVolume + results[idx].SellVolume
		if rd := regionDepthByType[results[idx].TypeID]; rd > 0 && stationDepth > 0 {
			stationShare = float64(stationDepth) / float64(rd)
			if stationShare > 1.0 {
				stationShare = 1.0
			}
		}
		flowPerDay := regionFlowPerDay * stationShare
		results[idx].DailyVolume = int64(math.Round(flowPerDay))
		// Estimate cycle-constrained daily share from both sides:
		// buy-order fills from S2B flow and sell-order fills from BfS flow.
		s2bForShare, bfsForShare := estimateSideFlowsPerDay(
			flowPerDay,
			results[idx].BuyVolume,
			results[idx].SellVolume,
		)
		buySideShare := harmonicDailyShare(int64(math.Round(s2bForShare)), results[idx].BuyOrderCount)
		sellSideShare := harmonicDailyShare(int64(math.Round(bfsForShare)), results[idx].SellOrderCount)
		dailyShare := minInt64(buySideShare, sellSideShare)
		baselineDailyProfit := sanitizeFloat(results[idx].ProfitPerUnit * float64(dailyShare))
		results[idx].TheoreticalDailyProfit = baselineDailyProfit
		results[idx].RealizableDailyProfit = baselineDailyProfit
		results[idx].DailyProfit = baselineDailyProfit
		// TotalProfit will be set at the end of the loop as full book spread profit.

		// Recompute execution-aware daily profit with an economically relevant qty.
		// This fixes fixed-qty distortion from early pre-history enrichment.
		execKey := stationTypeKey{results[idx].StationID, results[idx].TypeID}
		if g, ok := orderGroups[execKey]; ok {
			desiredQty := stationExecutionDesiredQtyFromDailyShare(dailyShare, results[idx].BuyVolume, results[idx].SellVolume)
			if desiredQty > 0 {
				safeQty, planBuy, planSell, expectedProfit := findSafeExecutionQuantity(
					g.sellOrders, // asks we buy from
					g.buyOrders,  // bids we sell into
					desiredQty,
					buyCostMult,
					sellRevenueMult,
				)
				results[idx].CanFill = safeQty >= desiredQty && safeQty > 0
				if safeQty > 0 {
					results[idx].FilledQty = safeQty
					results[idx].ExpectedBuyPrice = planBuy.ExpectedPrice
					results[idx].ExpectedSellPrice = planSell.ExpectedPrice
					results[idx].SlippageBuyPct = planBuy.SlippagePercent
					results[idx].SlippageSellPct = planSell.SlippagePercent
					realizable := sanitizeFloat(expectedProfit)
					results[idx].RealProfit = realizable
					results[idx].ExpectedProfit = sanitizeFloat(expectedProfit / float64(safeQty))
					effectiveBuyPerUnit := planBuy.ExpectedPrice * buyCostMult
					if effectiveBuyPerUnit > 0 {
						results[idx].RealMarginPercent = sanitizeFloat(
							(results[idx].ExpectedProfit / effectiveBuyPerUnit) * 100,
						)
					}
					if realizable > 0 {
						results[idx].RealizableDailyProfit = realizable
						results[idx].HasExecutionEvidence = true
						if results[idx].RealMarginPercent != 0 {
							results[idx].NowROI = results[idx].RealMarginPercent
						}
					}
				}
			}
		}

		// Calculate VWAP (30 days)
		results[idx].VWAP = sanitizeFloat(CalcVWAP(hd.entries, stationVWAPWindowDays))

		// Calculate DRVI (30 days)
		results[idx].PVI = sanitizeFloat(CalcDRVI(hd.entries, stationVolatilityWindowDays))

		// Calculate spread ROI (typical buy-sell spread over the period)
		results[idx].PeriodROI = sanitizeFloat(CalcSpreadROI(hd.entries, avgPeriod))

		// Calculate price stats
		avg, high, low := CalcAvgPriceStats(hd.entries, avgPeriod)
		results[idx].AvgPrice = sanitizeFloat(avg)
		results[idx].PriceHigh = sanitizeFloat(high)
		results[idx].PriceLow = sanitizeFloat(low)

		// Calculate Buy/Sell Units per Day from market history
		results[idx].BuyUnitsPerDay = flowPerDay

		// SellUnitsPerDay is derived symmetrically from book imbalance.
		results[idx].SellUnitsPerDay = estimateSellUnitsPerDay(
			flowPerDay,
			results[idx].BuyVolume,
			results[idx].SellVolume,
		)

		// B v S Ratio
		if results[idx].SellUnitsPerDay > 0 {
			results[idx].BvSRatio = sanitizeFloat(results[idx].BuyUnitsPerDay / results[idx].SellUnitsPerDay)
		}
		// A4E-style aliases with mass-balance: S2B + BfS = traded flow.
		s2b, bfs := estimateSideFlowsPerDay(
			flowPerDay,
			results[idx].BuyVolume,
			results[idx].SellVolume,
		)
		results[idx].S2BPerDay = sanitizeFloat(s2b)
		results[idx].BfSPerDay = sanitizeFloat(bfs)
		if results[idx].BfSPerDay > 0 {
			results[idx].S2BBfSRatio = sanitizeFloat(results[idx].S2BPerDay / results[idx].BfSPerDay)
		}

		// Days of Supply
		if results[idx].BuyUnitsPerDay > 0 {
			results[idx].DOS = sanitizeFloat(float64(results[idx].SellVolume) / results[idx].BuyUnitsPerDay)
		}

		// Calculate SDS (Scam Detection Score)
		key := stationTypeKey{results[idx].StationID, results[idx].TypeID}
		if g, ok := orderGroups[key]; ok {
			results[idx].SDS = CalcSDS(g.buyOrders, g.sellOrders, hd.entries, results[idx].VWAP)
		}

		// Set risk flags
		results[idx].IsHighRiskFlag = results[idx].SDS >= 50
		if params.FlagExtremePrices && results[idx].VWAP > 0 {
			results[idx].IsExtremePriceFlag = IsExtremePrice(results[idx].BuyPrice, results[idx].VWAP, 50)
		}

		confidenceScore := stationConfidenceScore(&results[idx], flowPerDay, results[idx].HasExecutionEvidence)
		results[idx].ConfidenceScore = confidenceScore
		results[idx].ConfidenceLabel = stationConfidenceLabel(confidenceScore)

		if !results[idx].HasExecutionEvidence {
			factor := stationMakerFallbackRealizationFactor(confidenceScore, results[idx].CI)
			results[idx].RealizableDailyProfit = sanitizeFloat(results[idx].TheoreticalDailyProfit * factor)
		}
		// Keep realizable estimate conservative: never exceed pure spread-maker baseline.
		if results[idx].TheoreticalDailyProfit > 0 &&
			results[idx].RealizableDailyProfit > results[idx].TheoreticalDailyProfit {
			results[idx].RealizableDailyProfit = results[idx].TheoreticalDailyProfit
		}
		if results[idx].RealizableDailyProfit < 0 {
			results[idx].RealizableDailyProfit = 0
		}
		results[idx].DailyProfit = sanitizeFloat(results[idx].RealizableDailyProfit)
		// TotalProfit: full book spread profit (not daily). Gives the user a
		// sense of total addressable opportunity on this item/station.
		tradableUnits := float64(minInt64(results[idx].BuyVolume, results[idx].SellVolume))
		results[idx].TotalProfit = sanitizeFloat(results[idx].ProfitPerUnit * tradableUnits)

		// Calculate CTS (Composite Trading Score)
		results[idx].CTS = sanitizeFloat(CalcCTSWithWeights(
			results[idx].PeriodROI,
			results[idx].OBDS,
			results[idx].PVI,
			results[idx].CI,
			results[idx].SDS,
			flowPerDay,
			ctsWeights,
		))

	}
}
