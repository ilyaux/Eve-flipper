package engine

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strings"

	"eve-flipper/internal/esi"
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
	BuyVolume      int32   `json:"BuyVolume"`  // total volume of buy orders
	SellVolume     int32   `json:"SellVolume"` // total volume of sell orders
	TotalProfit    float64 `json:"TotalProfit"`
	DailyProfit    float64 `json:"DailyProfit"` // estimated executable daily profit
	ROI            float64 `json:"ROI"`         // profit / investment * 100
	StationName    string  `json:"StationName"`
	StationID      int64   `json:"StationID"`

	// --- EVE Guru style metrics ---
	CapitalRequired float64 `json:"CapitalRequired"` // Sum of all buy orders ISK
	NowROI          float64 `json:"NowROI"`          // ProfitPerUnit / CapitalPerUnit * 100
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

// estimateSellUnitsPerDay derives supply-side daily throughput from recent traded
// volume and current book imbalance. This keeps BvS mathematically symmetric:
// BvS = BuyUnitsPerDay / SellUnitsPerDay ~= BuyVolume / SellVolume.
func estimateSellUnitsPerDay(dailyVolume float64, buyVolume, sellVolume int32) float64 {
	if dailyVolume <= 0 || buyVolume <= 0 || sellVolume <= 0 {
		return 0
	}
	return dailyVolume * float64(sellVolume) / float64(buyVolume)
}

// stationExecutionDesiredQty picks the quantity for execution simulation.
// If daily share is known, we target that share capped by current depth.
// Otherwise we fall back to a bounded probe size to avoid over-weighting huge books.
func stationExecutionDesiredQty(dailyShare int64, buyVolume, sellVolume int32) int32 {
	depthCap := minInt32(buyVolume, sellVolume)
	if depthCap <= 0 {
		return 0
	}
	if dailyShare > 0 {
		if dailyShare > int64(depthCap) {
			return depthCap
		}
		return int32(dailyShare)
	}
	const fallbackQty int32 = 1000
	if depthCap < fallbackQty {
		return depthCap
	}
	return fallbackQty
}

// StationTradeParams holds input parameters for station trading scan.
type StationTradeParams struct {
	StationIDs      map[int64]bool // nil or empty = all stations in region
	RegionID        int32
	MinMargin       float64
	SalesTaxPercent float64
	BrokerFee       float64 // percent
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

		// Total volumes and capital required
		var totalBuyVol, totalSellVol int32
		var capitalRequired float64
		for _, o := range g.buyOrders {
			totalBuyVol += o.VolumeRemain
			capitalRequired += o.Price * float64(o.VolumeRemain)
		}
		for _, o := range g.sellOrders {
			totalSellVol += o.VolumeRemain
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
		tradableUnits := minInt32(totalBuyVol, totalSellVol)
		obdsCapital := effectiveBuy * float64(tradableUnits)
		if obdsCapital <= 0 {
			obdsCapital = capitalRequired
		}
		ci := CalcCI(append(g.buyOrders, g.sellOrders...))
		obds := CalcOBDS(g.buyOrders, g.sellOrders, obdsCapital)

		// NowROI = profit per unit / effective cost per unit (including broker fee) * 100
		nowROI := 0.0
		if effectiveBuy > 0 {
			nowROI = profitPerUnit / effectiveBuy * 100
		}

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
			NowROI:          sanitizeFloat(nowROI),
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
		results = results[:MaxUnlimitedResults]
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
	s.enrichStationWithHistory(results, params.RegionID, orderGroups, params, progress)

	// Apply post-history filters
	results = applyStationTradeFilters(results, params)

	log.Printf("[DEBUG] StationTrades: %d after all filters", len(results))

	// Final sort by CTS (Composite Trading Score) descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].CTS > results[j].CTS
	})

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

	// Debug counters
	var dropVol, dropS2B, dropBfS, dropROI, dropBvS, dropPVI, dropSDS, dropPrice int

	for _, r := range results {
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
		log.Printf("[DEBUG] StationFilter drops: vol=%d s2b=%d bfs=%d roi=%d bvs=%d pvi=%d sds=%d price=%d",
			dropVol, dropS2B, dropBfS, dropROI, dropBvS, dropPVI, dropSDS, dropPrice)
	}

	return filtered
}

// enrichStationWithHistory fetches market history and calculates advanced metrics.
func (s *Scanner) enrichStationWithHistory(results []StationTrade, regionID int32, orderGroups map[stationTypeKey]*orderGroup, params StationTradeParams, progress func(string)) {
	if s.History == nil || len(results) == 0 {
		return
	}

	progress("Fetching market history...")

	// Determine period for calculations (default 90 days)
	avgPeriod := params.AvgPricePeriod
	if avgPeriod <= 0 {
		avgPeriod = 90
	}
	buyCostMult, sellRevenueMult := tradeFeeMultipliers(tradeFeeInputs{
		SplitTradeFees:       params.SplitTradeFees,
		BrokerFeePercent:     params.BrokerFee,
		SalesTaxPercent:      params.SalesTaxPercent,
		BuyBrokerFeePercent:  params.BuyBrokerFeePercent,
		SellBrokerFeePercent: params.SellBrokerFeePercent,
		BuySalesTaxPercent:   params.BuySalesTaxPercent,
		SellSalesTaxPercent:  params.SellSalesTaxPercent,
	})

	type histResult struct {
		idx     int
		entries []esi.HistoryEntry
		stats   esi.MarketStats
	}
	ch := make(chan histResult, len(results))
	sem := make(chan struct{}, 20)

	for i := range results {
		sem <- struct{}{}
		go func(idx int) {
			defer func() { <-sem }()

			entries, ok := s.History.GetMarketHistory(regionID, results[idx].TypeID)
			if !ok {
				var err error
				entries, err = s.ESI.FetchMarketHistory(regionID, results[idx].TypeID)
				if err != nil {
					ch <- histResult{idx, nil, esi.MarketStats{}}
					return
				}
				s.History.SetMarketHistory(regionID, results[idx].TypeID, entries)
			}

			totalListed := results[idx].BuyVolume + results[idx].SellVolume
			stats := esi.ComputeMarketStats(entries, totalListed)
			ch <- histResult{idx, entries, stats}
		}(i)
	}

	progress("Calculating advanced metrics...")

	for range results {
		r := <-ch
		idx := r.idx

		// Basic history stats
		results[idx].DailyVolume = r.stats.DailyVolume
		// Estimate daily share using harmonic distribution (top-of-book fills faster).
		competitors := results[idx].BuyOrderCount + results[idx].SellOrderCount
		dailyShare := harmonicDailyShare(r.stats.DailyVolume, competitors)
		baselineDailyProfit := sanitizeFloat(results[idx].ProfitPerUnit * float64(dailyShare))
		results[idx].DailyProfit = baselineDailyProfit
		results[idx].TotalProfit = baselineDailyProfit

		// Recompute execution-aware daily profit with an economically relevant qty.
		// This fixes fixed-qty distortion from early pre-history enrichment.
		execKey := stationTypeKey{results[idx].StationID, results[idx].TypeID}
		if g, ok := orderGroups[execKey]; ok {
			desiredQty := stationExecutionDesiredQty(dailyShare, results[idx].BuyVolume, results[idx].SellVolume)
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
					results[idx].RealProfit = sanitizeFloat(expectedProfit)
					results[idx].DailyProfit = sanitizeFloat(expectedProfit)
					results[idx].TotalProfit = results[idx].DailyProfit // compatibility
					results[idx].ExpectedProfit = sanitizeFloat(expectedProfit / float64(safeQty))
				}
			}
		}

		if len(r.entries) == 0 {
			continue
		}

		// Calculate VWAP (30 days)
		results[idx].VWAP = sanitizeFloat(CalcVWAP(r.entries, 30))

		// Calculate DRVI (30 days)
		results[idx].PVI = sanitizeFloat(CalcDRVI(r.entries, 30))

		// Calculate spread ROI (typical buy-sell spread over the period)
		results[idx].PeriodROI = sanitizeFloat(CalcSpreadROI(r.entries, avgPeriod))

		// Calculate price stats
		avg, high, low := CalcAvgPriceStats(r.entries, avgPeriod)
		results[idx].AvgPrice = sanitizeFloat(avg)
		results[idx].PriceHigh = sanitizeFloat(high)
		results[idx].PriceLow = sanitizeFloat(low)

		// Calculate Buy/Sell Units per Day from market history
		dailyVol := avgDailyVolume(r.entries, 7)
		results[idx].BuyUnitsPerDay = dailyVol

		// SellUnitsPerDay is derived symmetrically from book imbalance.
		results[idx].SellUnitsPerDay = estimateSellUnitsPerDay(
			dailyVol,
			results[idx].BuyVolume,
			results[idx].SellVolume,
		)

		// B v S Ratio
		if results[idx].SellUnitsPerDay > 0 {
			results[idx].BvSRatio = sanitizeFloat(results[idx].BuyUnitsPerDay / results[idx].SellUnitsPerDay)
		}
		// A4E-style aliases with mass-balance: S2B + BfS = traded flow.
		s2b, bfs := estimateSideFlowsPerDay(
			dailyVol,
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
			results[idx].SDS = CalcSDS(g.buyOrders, r.entries, results[idx].VWAP)
		}

		// Set risk flags
		results[idx].IsHighRiskFlag = results[idx].SDS >= 50
		if params.FlagExtremePrices && results[idx].VWAP > 0 {
			results[idx].IsExtremePriceFlag = IsExtremePrice(results[idx].BuyPrice, results[idx].VWAP, 50)
		}

		// Calculate CTS (Composite Trading Score)
		results[idx].CTS = sanitizeFloat(CalcCTS(
			results[idx].PeriodROI,
			results[idx].OBDS,
			results[idx].PVI,
			results[idx].CI,
			results[idx].SDS,
			results[idx].BuyUnitsPerDay,
		))

	}
}
