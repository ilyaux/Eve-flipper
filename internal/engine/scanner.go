package engine

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"sync"

	"eve-flipper/internal/esi"
	"eve-flipper/internal/sde"
)

const (
	// MaxUnlimitedResults caps the working set to prevent server overload
	// (sorting, history enrichment, and JSON serialization of very large result sets).
	MaxUnlimitedResults = 5000
	// UnreachableJumps is the fallback jump count when no path exists.
	UnreachableJumps = 999
)

// HistoryProvider is an interface for fetching and caching market history.
type HistoryProvider interface {
	GetMarketHistory(regionID int32, typeID int32) ([]esi.HistoryEntry, bool)
	SetMarketHistory(regionID int32, typeID int32, entries []esi.HistoryEntry)
}

// Scanner orchestrates market scans using SDE data and the ESI client.
type Scanner struct {
	SDE                *sde.Data
	ESI                *esi.Client
	History            HistoryProvider
	ContractsCache     *esi.ContractsCache     // Cache for contracts (5 min TTL)
	ContractItemsCache *esi.ContractItemsCache // Cache for contract items (immutable)
}

// NewScanner creates a Scanner with the given static data and ESI client.
func NewScanner(data *sde.Data, client *esi.Client) *Scanner {
	return &Scanner{
		SDE:                data,
		ESI:                client,
		ContractsCache:     esi.NewContractsCache(),
		ContractItemsCache: esi.NewContractItemsCache(),
	}
}

// Scan finds profitable flip opportunities based on the given parameters.
func (s *Scanner) Scan(params ScanParams, progress func(string)) ([]FlipResult, error) {
	progress("Finding systems within radius...")
	var buySystems, sellSystems map[int32]int
	var wg sync.WaitGroup
	wg.Add(2)
	minSec := params.MinRouteSecurity
	go func() {
		defer wg.Done()
		if minSec > 0 {
			buySystems = s.SDE.Universe.SystemsWithinRadiusMinSecurity(params.CurrentSystemID, params.BuyRadius, minSec)
		} else {
			buySystems = s.SDE.Universe.SystemsWithinRadius(params.CurrentSystemID, params.BuyRadius)
		}
	}()
	go func() {
		defer wg.Done()
		if minSec > 0 {
			sellSystems = s.SDE.Universe.SystemsWithinRadiusMinSecurity(params.CurrentSystemID, params.SellRadius, minSec)
		} else {
			sellSystems = s.SDE.Universe.SystemsWithinRadius(params.CurrentSystemID, params.SellRadius)
		}
	}()
	wg.Wait()

	buyRegions := s.SDE.Universe.RegionsInSet(buySystems)
	sellRegions := s.SDE.Universe.RegionsInSet(sellSystems)

	log.Printf("[DEBUG] Scan: buySystems=%d, sellSystems=%d, buyRegions=%d, sellRegions=%d",
		len(buySystems), len(sellSystems), len(buyRegions), len(sellRegions))

	progress(fmt.Sprintf("Fetching orders from %d+%d regions...", len(buyRegions), len(sellRegions)))
	idx := s.fetchAndIndex(buyRegions, buySystems, sellRegions, sellSystems)
	return s.calculateResults(params, idx, buySystems, progress)
}

// ScanMultiRegion finds profitable flip opportunities across whole regions.
func (s *Scanner) ScanMultiRegion(params ScanParams, progress func(string)) ([]FlipResult, error) {
	minSec := params.MinRouteSecurity

	// If target region is specified, use it directly instead of radius search
	if params.TargetRegionID > 0 {
		progress(fmt.Sprintf("Searching target region %d...", params.TargetRegionID))

		var buySystemsRadius map[int32]int
		if minSec > 0 {
			buySystemsRadius = s.SDE.Universe.SystemsWithinRadiusMinSecurity(params.CurrentSystemID, params.BuyRadius, minSec)
		} else {
			buySystemsRadius = s.SDE.Universe.SystemsWithinRadius(params.CurrentSystemID, params.BuyRadius)
		}
		buyRegions := s.SDE.Universe.RegionsInSet(buySystemsRadius)
		buySystems := s.SDE.Universe.SystemsInRegions(buyRegions)

		sellRegions := map[int32]bool{params.TargetRegionID: true}
		sellSystems := s.SDE.Universe.SystemsInRegions(sellRegions)

		progress(fmt.Sprintf("Fetching orders: buy from %d regions, sell to target region...", len(buyRegions)))
		idx := s.fetchAndIndex(buyRegions, buySystems, sellRegions, sellSystems)
		return s.calculateResults(params, idx, buySystemsRadius, progress)
	}

	// Default behavior: search by radius
	progress("Finding regions by radius...")
	var buySystemsRadius, sellSystemsRadius map[int32]int
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if minSec > 0 {
			buySystemsRadius = s.SDE.Universe.SystemsWithinRadiusMinSecurity(params.CurrentSystemID, params.BuyRadius, minSec)
		} else {
			buySystemsRadius = s.SDE.Universe.SystemsWithinRadius(params.CurrentSystemID, params.BuyRadius)
		}
	}()
	go func() {
		defer wg.Done()
		if minSec > 0 {
			sellSystemsRadius = s.SDE.Universe.SystemsWithinRadiusMinSecurity(params.CurrentSystemID, params.SellRadius, minSec)
		} else {
			sellSystemsRadius = s.SDE.Universe.SystemsWithinRadius(params.CurrentSystemID, params.SellRadius)
		}
	}()
	wg.Wait()

	buyRegions := s.SDE.Universe.RegionsInSet(buySystemsRadius)
	sellRegions := s.SDE.Universe.RegionsInSet(sellSystemsRadius)
	buySystems := s.SDE.Universe.SystemsInRegions(buyRegions)
	sellSystems := s.SDE.Universe.SystemsInRegions(sellRegions)

	progress(fmt.Sprintf("Fetching orders from %d+%d regions...", len(buyRegions), len(sellRegions)))
	idx := s.fetchAndIndex(buyRegions, buySystems, sellRegions, sellSystems)
	return s.calculateResults(params, idx, buySystemsRadius, progress)
}

// --- Streaming order index types ---

type sellInfo struct {
	Price        float64
	VolumeRemain int32
	LocationID   int64
	SystemID     int32
	OrderCount   int
}

type buyInfo struct {
	Price        float64
	VolumeRemain int32
	LocationID   int64
	SystemID     int32
	OrderCount   int
}

type locKey struct {
	typeID     int32
	locationID int64
}

// scanIndex holds pre-built maps from the streaming fetch phase.
// Built concurrently while orders are still arriving from ESI.
type scanIndex struct {
	sellByType map[int32][]sellInfo // all sell orders grouped by typeID
	sellCounts map[locKey]int
	buyByType  map[int32][]buyInfo // all buy orders grouped by typeID
	buyCounts  map[locKey]int
	// Raw orders kept for execution plan (indexed by location+type).
	sellOrders []esi.MarketOrder
	buyOrders  []esi.MarketOrder
}

// hubRegionPriority maps known high-traffic region IDs to priority (lower = first).
// These regions have the most orders and should be fetched earliest for pipeline benefit.
var hubRegionPriority = map[int32]int{
	10000002: 0, // The Forge (Jita)
	10000043: 1, // Domain (Amarr)
	10000032: 2, // Sinq Laison (Dodixie)
	10000042: 3, // Metropolis (Hek)
	10000030: 4, // Heimatar (Rens)
}

// fetchOrdersStream starts fetching orders for all regions concurrently and
// streams batches of filtered orders through the returned channel.
// Hub regions are launched first so the pipeline starts building maps from
// the largest data sets sooner.
func (s *Scanner) fetchOrdersStream(
	regions map[int32]bool,
	orderType string,
	validSystems map[int32]int,
) <-chan []esi.MarketOrder {
	ch := make(chan []esi.MarketOrder, len(regions))

	// Sort regions: hubs first, then the rest.
	sorted := make([]int32, 0, len(regions))
	for rid := range regions {
		sorted = append(sorted, rid)
	}
	sort.Slice(sorted, func(i, j int) bool {
		pi, oki := hubRegionPriority[sorted[i]]
		pj, okj := hubRegionPriority[sorted[j]]
		if oki && okj {
			return pi < pj
		}
		if oki {
			return true
		}
		if okj {
			return false
		}
		return sorted[i] < sorted[j]
	})

	var wg sync.WaitGroup
	for _, regionID := range sorted {
		wg.Add(1)
		go func(rid int32) {
			defer wg.Done()
			orders, err := s.ESI.FetchRegionOrders(rid, orderType)
			if err != nil {
				return
			}
			// Filter to valid systems
			filtered := make([]esi.MarketOrder, 0, len(orders)/2)
			for _, o := range orders {
				if _, ok := validSystems[o.SystemID]; ok {
					filtered = append(filtered, o)
				}
			}
			if len(filtered) > 0 {
				ch <- filtered
			}
		}(regionID)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	return ch
}

// fetchAndIndex launches parallel streaming fetches for sell and buy orders,
// building the scanIndex incrementally as regions complete.
func (s *Scanner) fetchAndIndex(
	buyRegions map[int32]bool, buySystems map[int32]int,
	sellRegions map[int32]bool, sellSystems map[int32]int,
) *scanIndex {
	sellCh := s.fetchOrdersStream(buyRegions, "sell", buySystems)
	buyCh := s.fetchOrdersStream(sellRegions, "buy", sellSystems)

	idx := &scanIndex{
		sellByType: make(map[int32][]sellInfo),
		sellCounts: make(map[locKey]int),
		buyByType:  make(map[int32][]buyInfo),
		buyCounts:  make(map[locKey]int),
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// Consumer 1: collect all sell orders grouped by type
	go func() {
		defer wg.Done()
		for batch := range sellCh {
			idx.sellOrders = append(idx.sellOrders, batch...)
			for _, o := range batch {
				idx.sellCounts[locKey{o.TypeID, o.LocationID}]++
				idx.sellByType[o.TypeID] = append(idx.sellByType[o.TypeID], sellInfo{
					Price: o.Price, VolumeRemain: o.VolumeRemain,
					LocationID: o.LocationID, SystemID: o.SystemID,
				})
			}
		}
		// Fill order counts per location
		for tid, sells := range idx.sellByType {
			for i := range sells {
				sells[i].OrderCount = idx.sellCounts[locKey{tid, sells[i].LocationID}]
			}
		}
	}()

	// Consumer 2: collect all buy orders grouped by type
	go func() {
		defer wg.Done()
		for batch := range buyCh {
			idx.buyOrders = append(idx.buyOrders, batch...)
			for _, o := range batch {
				idx.buyCounts[locKey{o.TypeID, o.LocationID}]++
				idx.buyByType[o.TypeID] = append(idx.buyByType[o.TypeID], buyInfo{
					Price: o.Price, VolumeRemain: o.VolumeRemain,
					LocationID: o.LocationID, SystemID: o.SystemID,
				})
			}
		}
		for tid, buys := range idx.buyByType {
			for i := range buys {
				buys[i].OrderCount = idx.buyCounts[locKey{tid, buys[i].LocationID}]
			}
		}
	}()

	wg.Wait()

	log.Printf("[DEBUG] fetchAndIndex: %d sell orders, %d buy orders", len(idx.sellOrders), len(idx.buyOrders))
	log.Printf("[DEBUG] sellByType: %d types, buyByType: %d types", len(idx.sellByType), len(idx.buyByType))
	return idx
}

// calculateResults is the shared profit calculation logic.
// bfsDistances = pre-computed distances from origin (used for buyJumps lookup).
func (s *Scanner) calculateResults(
	params ScanParams,
	idx *scanIndex,
	bfsDistances map[int32]int,
	progress func(string),
) ([]FlipResult, error) {
	sellOrders := idx.sellOrders
	buyOrders := idx.buyOrders

	progress("Calculating profits...")
	buyCostMult, sellRevenueMult := tradeFeeMultipliers(tradeFeeInputs{
		SplitTradeFees:       params.SplitTradeFees,
		BrokerFeePercent:     params.BrokerFeePercent,
		SalesTaxPercent:      params.SalesTaxPercent,
		BuyBrokerFeePercent:  params.BuyBrokerFeePercent,
		SellBrokerFeePercent: params.SellBrokerFeePercent,
		BuySalesTaxPercent:   params.BuySalesTaxPercent,
		SellSalesTaxPercent:  params.SellSalesTaxPercent,
	})

	// For each (typeID, sellLocationID, buyLocationID) keep only the best-profit pair.
	// This deduplicates multiple orders at the same location while preserving
	// different location combinations (e.g. Amarr→Rens AND Jita→Rens).
	type pairKey struct {
		typeID    int32
		sellLocID int64 // where we BUY (from sell orders)
		buyLocID  int64 // where we SELL (to buy orders)
	}
	bestPairs := make(map[pairKey]*FlipResult)

	minSec := params.MinRouteSecurity

	// Pre-filter: for each type, keep only the cheapest sell per location
	// and the most expensive buy per location to reduce cross-join iterations.
	// This collapses e.g. 500 sell orders at Jita into 1 best-price entry.
	type sellLocBest struct {
		sellInfo
	}
	type buyLocBest struct {
		buyInfo
	}

	for typeID, sells := range idx.sellByType {
		buys := idx.buyByType[typeID]
		if len(buys) == 0 {
			continue
		}

		itemType, ok := s.SDE.Types[typeID]
		if !ok || itemType.Volume <= 0 {
			continue
		}

		maxUnitsF := math.Floor(params.CargoCapacity / itemType.Volume)
		if maxUnitsF > math.MaxInt32 {
			maxUnitsF = math.MaxInt32
		}
		maxUnits := int32(maxUnitsF)
		if maxUnits <= 0 {
			continue
		}

		// Deduplicate sells: keep cheapest per location (with total volume)
		bestSellByLoc := make(map[int64]*sellLocBest)
		for _, sell := range sells {
			if existing, ok := bestSellByLoc[sell.LocationID]; ok {
				// Accumulate volume, keep cheapest price
				existing.VolumeRemain += sell.VolumeRemain
				if sell.Price < existing.Price {
					existing.Price = sell.Price
					existing.SystemID = sell.SystemID
					existing.OrderCount = sell.OrderCount
				}
			} else {
				cp := sell
				bestSellByLoc[sell.LocationID] = &sellLocBest{cp}
			}
		}

		// Deduplicate buys: keep most expensive per location (with total volume)
		bestBuyByLoc := make(map[int64]*buyLocBest)
		for _, buy := range buys {
			if existing, ok := bestBuyByLoc[buy.LocationID]; ok {
				// Accumulate volume, keep highest price
				existing.VolumeRemain += buy.VolumeRemain
				if buy.Price > existing.Price {
					existing.Price = buy.Price
					existing.SystemID = buy.SystemID
					existing.OrderCount = buy.OrderCount
				}
			} else {
				cp := buy
				bestBuyByLoc[buy.LocationID] = &buyLocBest{cp}
			}
		}

		// Quick check: can the best possible pair for this type be profitable?
		cheapestSell := math.MaxFloat64
		for _, sell := range bestSellByLoc {
			if sell.Price < cheapestSell {
				cheapestSell = sell.Price
			}
		}
		expensiveBuy := 0.0
		for _, buy := range bestBuyByLoc {
			if buy.Price > expensiveBuy {
				expensiveBuy = buy.Price
			}
		}
		bestEffBuy := cheapestSell * buyCostMult
		bestEffSell := expensiveBuy * sellRevenueMult
		if bestEffSell <= bestEffBuy {
			continue
		}
		bestMargin := (bestEffSell - bestEffBuy) / bestEffBuy * 100
		if bestMargin < params.MinMargin {
			continue
		}

		// Cross-join deduplicated locations (much smaller than raw order count)
		for sellLocID, sell := range bestSellByLoc {
			for buyLocID, buy := range bestBuyByLoc {
				if buy.Price <= sell.Price {
					continue
				}
				if sellLocID == buyLocID {
					continue
				}

				effectiveBuyPrice := sell.Price * buyCostMult
				effectiveSellPrice := buy.Price * sellRevenueMult
				profitPerUnit := effectiveSellPrice - effectiveBuyPrice
				if profitPerUnit <= 0 {
					continue
				}
				margin := profitPerUnit / effectiveBuyPrice * 100
				if margin < params.MinMargin {
					continue
				}

				units := maxUnits
				if sell.VolumeRemain < units {
					units = sell.VolumeRemain
				}
				if buy.VolumeRemain < units {
					units = buy.VolumeRemain
				}

				// MaxInvestment filter
				if params.MaxInvestment > 0 {
					maxAfford := int32(params.MaxInvestment / effectiveBuyPrice)
					if maxAfford <= 0 {
						continue
					}
					if units > maxAfford {
						units = maxAfford
					}
				}

				totalProfit := profitPerUnit * float64(units)

				// Dedup: keep only the best profit for this location pair + type
				pk := pairKey{typeID, sellLocID, buyLocID}
				if existing, ok := bestPairs[pk]; ok {
					if totalProfit <= existing.TotalProfit {
						continue
					}
				}

				// Route check (BFS)
				buyJumps := s.jumpsBetweenWithBFS(params.CurrentSystemID, sell.SystemID, bfsDistances, minSec)
				sellJumps := s.jumpsBetweenWithSecurity(sell.SystemID, buy.SystemID, minSec)
				if buyJumps >= UnreachableJumps || sellJumps >= UnreachableJumps {
					continue
				}

				totalJumps := buyJumps + sellJumps
				var profitPerJump float64
				if totalJumps > 0 {
					profitPerJump = totalProfit / float64(totalJumps)
				}

				buyRegionID := int32(0)
				if sys, ok := s.SDE.Systems[sell.SystemID]; ok {
					buyRegionID = sys.RegionID
				}
				sellRegionID := int32(0)
				if sys, ok := s.SDE.Systems[buy.SystemID]; ok {
					sellRegionID = sys.RegionID
				}

				result := FlipResult{
					TypeID:          typeID,
					TypeName:        itemType.Name,
					Volume:          itemType.Volume,
					BuyPrice:        sell.Price,
					BuyStation:      "",
					BuySystemName:   s.systemName(sell.SystemID),
					BuySystemID:     sell.SystemID,
					BuyRegionID:     buyRegionID,
					BuyRegionName:   s.regionName(buyRegionID),
					BuyLocationID:   sellLocID,
					SellPrice:       buy.Price,
					SellStation:     "",
					SellSystemName:  s.systemName(buy.SystemID),
					SellSystemID:    buy.SystemID,
					SellRegionID:    sellRegionID,
					SellRegionName:  s.regionName(sellRegionID),
					SellLocationID:  buyLocID,
					ProfitPerUnit:   profitPerUnit,
					MarginPercent:   margin,
					UnitsToBuy:      units,
					BuyOrderRemain:  buy.VolumeRemain,
					SellOrderRemain: sell.VolumeRemain,
					TotalProfit:     totalProfit,
					ProfitPerJump:   sanitizeFloat(profitPerJump),
					BuyJumps:        buyJumps,
					SellJumps:       sellJumps,
					TotalJumps:      totalJumps,
					BuyCompetitors:  sell.OrderCount,
					SellCompetitors: buy.OrderCount,
				}
				bestPairs[pk] = &result
			}
		}
	}

	// Flatten deduped results
	results := make([]FlipResult, 0, len(bestPairs))
	for _, r := range bestPairs {
		results = append(results, *r)
	}
	log.Printf("[DEBUG] found %d results before sort/trim", len(results))

	// Sort by profit descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].TotalProfit > results[j].TotalProfit
	})

	// Cap internal working set for history enrichment to prevent server overload
	// on extremely large result sets (e.g. multi-region with 200k+ results).
	if len(results) > MaxUnlimitedResults {
		results = results[:MaxUnlimitedResults]
	}

	// Enrich with execution-plan expected prices (same order book, no extra ESI).
	// Filter orders by location_id for more accurate slippage estimates.
	if len(results) > 0 {
		progress("Expected fill prices...")
		type locTypeKey struct {
			locationID int64
			typeID     int32
		}
		// Index sell orders by location+type (for buy-side execution plan at specific station)
		sellByLT := make(map[locTypeKey][]esi.MarketOrder)
		for _, o := range sellOrders {
			k := locTypeKey{o.LocationID, o.TypeID}
			sellByLT[k] = append(sellByLT[k], o)
		}
		// Index buy orders by location+type (for sell-side execution plan at specific station)
		buyByLT := make(map[locTypeKey][]esi.MarketOrder)
		for _, o := range buyOrders {
			k := locTypeKey{o.LocationID, o.TypeID}
			buyByLT[k] = append(buyByLT[k], o)
		}
		filtered := make([]FlipResult, 0, len(results))
		for i := range results {
			r := &results[i]
			requestedQty := r.UnitsToBuy
			safeQty, planBuy, planSell, expectedProfit := findSafeExecutionQuantity(
				sellByLT[locTypeKey{r.BuyLocationID, r.TypeID}],
				buyByLT[locTypeKey{r.SellLocationID, r.TypeID}],
				requestedQty,
				buyCostMult,
				sellRevenueMult,
			)
			if safeQty <= 0 {
				continue
			}

			r.FilledQty = safeQty
			r.CanFill = safeQty >= requestedQty

			if safeQty != requestedQty {
				r.UnitsToBuy = safeQty
				r.TotalProfit = r.ProfitPerUnit * float64(safeQty)
				if r.TotalJumps > 0 {
					r.ProfitPerJump = sanitizeFloat(r.TotalProfit / float64(r.TotalJumps))
				} else {
					r.ProfitPerJump = 0
				}
			}
			r.ExpectedBuyPrice = planBuy.ExpectedPrice
			r.ExpectedSellPrice = planSell.ExpectedPrice
			r.SlippageBuyPct = planBuy.SlippagePercent
			r.SlippageSellPct = planSell.SlippagePercent
			r.ExpectedProfit = expectedProfit
			r.RealProfit = expectedProfit
			filtered = append(filtered, *r)
		}
		results = filtered

		// Re-sort by real profit (depth/slippage-aware KPI).
		sort.Slice(results, func(i, j int) bool {
			if results[i].RealProfit == results[j].RealProfit {
				return results[i].TotalProfit > results[j].TotalProfit
			}
			return results[i].RealProfit > results[j].RealProfit
		})
	}

	// OPT: prefetch station names in parallel (only for top N)
	if len(results) > 0 {
		progress("Fetching station names...")
		topStations := make(map[int64]bool)
		for i := range results {
			topStations[results[i].BuyLocationID] = true
			topStations[results[i].SellLocationID] = true
		}
		s.ESI.PrefetchStationNames(topStations)

		// Fill station names from cache (instant, all prefetched)
		// For citadels (player structures), fallback to system name
		for i := range results {
			results[i].BuyStation = s.ESI.StationName(results[i].BuyLocationID)
			results[i].SellStation = s.ESI.StationName(results[i].SellLocationID)

			// If sell station is unresolved citadel, show system name instead
			if strings.HasPrefix(results[i].SellStation, "Location ") {
				if sys, ok := s.SDE.Systems[results[i].SellSystemID]; ok {
					results[i].SellStation = fmt.Sprintf("Structure @ %s", sys.Name)
				}
			}
			// Same for buy station
			if strings.HasPrefix(results[i].BuyStation, "Location ") {
				if sys, ok := s.SDE.Systems[results[i].BuySystemID]; ok {
					results[i].BuyStation = fmt.Sprintf("Structure @ %s", sys.Name)
				}
			}
		}
	}

	// Enrich with market history (volume, velocity, trend)
	s.enrichWithHistory(results, progress)

	// Derive A4E-style tradability proxies from daily traded flow and current
	// book imbalance for this specific buy->sell pair.
	for i := range results {
		s2b := float64(results[i].DailyVolume)
		bfs := estimateBfSPerDay(s2b, results[i].BuyOrderRemain, results[i].SellOrderRemain)
		results[i].S2BPerDay = sanitizeFloat(s2b)
		results[i].BfSPerDay = sanitizeFloat(bfs)
		if results[i].BfSPerDay > 0 {
			results[i].S2BBfSRatio = sanitizeFloat(results[i].S2BPerDay / results[i].BfSPerDay)
		}
	}

	// Compute DailyProfit = ProfitPerUnit * min(UnitsToBuy, estimatedDailyShare).
	// Uses harmonic distribution: top-of-book gets more volume than deeper positions.
	for i := range results {
		sellablePerDay := int64(results[i].UnitsToBuy)
		if results[i].DailyVolume > 0 {
			dailyShare := harmonicDailyShare(results[i].DailyVolume, results[i].SellCompetitors)
			if dailyShare < sellablePerDay {
				sellablePerDay = dailyShare
			}
		}
		profitPerUnit := results[i].ProfitPerUnit
		if results[i].FilledQty > 0 && results[i].RealProfit > 0 {
			profitPerUnit = results[i].RealProfit / float64(results[i].FilledQty)
		}
		results[i].DailyProfit = profitPerUnit * float64(sellablePerDay)
	}

	// Post-filter: min daily volume
	if params.MinDailyVolume > 0 {
		filtered := make([]FlipResult, 0, len(results))
		for _, r := range results {
			if r.DailyVolume >= params.MinDailyVolume {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}
	if params.MinS2BPerDay > 0 {
		filtered := make([]FlipResult, 0, len(results))
		for _, r := range results {
			if r.S2BPerDay >= params.MinS2BPerDay {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}
	if params.MinBfSPerDay > 0 {
		filtered := make([]FlipResult, 0, len(results))
		for _, r := range results {
			if r.BfSPerDay >= params.MinBfSPerDay {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}
	if params.MinS2BBfSRatio > 0 {
		filtered := make([]FlipResult, 0, len(results))
		for _, r := range results {
			if r.S2BBfSRatio >= params.MinS2BBfSRatio {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}
	if params.MaxS2BBfSRatio > 0 {
		filtered := make([]FlipResult, 0, len(results))
		for _, r := range results {
			if r.S2BBfSRatio <= params.MaxS2BBfSRatio {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}

	progress(fmt.Sprintf("Found %d profitable trades", len(results)))
	return results, nil
}

// fetchOrders is the legacy blocking version, kept for non-scan callers.
func (s *Scanner) fetchOrders(regions map[int32]bool, orderType string, validSystems map[int32]int) []esi.MarketOrder {
	ch := s.fetchOrdersStream(regions, orderType, validSystems)
	var all []esi.MarketOrder
	for batch := range ch {
		all = append(all, batch...)
	}
	log.Printf("[DEBUG] fetchOrders(%s): %d orders after filtering", orderType, len(all))
	return all
}

func (s *Scanner) jumpsBetween(from, to int32) int {
	return s.jumpsBetweenWithSecurity(from, to, 0)
}

// jumpsBetweenWithSecurity returns jump count using only systems with security >= minSecurity (0 = no filter).
func (s *Scanner) jumpsBetweenWithSecurity(from, to int32, minSecurity float64) int {
	var d int
	if minSecurity > 0 {
		d = s.SDE.Universe.ShortestPathMinSecurity(from, to, minSecurity)
	} else {
		d = s.SDE.Universe.ShortestPath(from, to)
	}
	if d < 0 {
		return UnreachableJumps
	}
	return d
}

// jumpsBetweenWithBFS uses pre-computed BFS distances when 'from' is the origin.
func (s *Scanner) jumpsBetweenWithBFS(from, to int32, bfsDistances map[int32]int, minRouteSecurity float64) int {
	if d, ok := bfsDistances[to]; ok {
		return d
	}
	return s.jumpsBetweenWithSecurity(from, to, minRouteSecurity)
}

// harmonicDailyShare estimates a player's share of daily volume using a harmonic
// distribution model. In real markets, top-of-book orders fill disproportionately
// faster than deeper positions. The harmonic model assigns share proportional to
// 1/position: position 1 gets 1/H(n), position 2 gets (1/2)/H(n), etc.
// where H(n) = 1 + 1/2 + ... + 1/n is the n-th harmonic number.
//
// A new player entering the market is conservatively placed at the median position
// ceil(n/2). This gives a more realistic (and usually more conservative) estimate
// than the naïve uniform model dailyVolume/(competitors+1).
func harmonicDailyShare(dailyVolume int64, competitors int) int64 {
	if dailyVolume <= 0 {
		return 0
	}
	if competitors <= 0 {
		return dailyVolume
	}
	n := competitors + 1 // total participants including the player
	// Harmonic number H(n) = Σ(1/k) for k=1..n
	hn := 0.0
	for k := 1; k <= n; k++ {
		hn += 1.0 / float64(k)
	}
	// Player at median position
	position := (n + 1) / 2 // ceil(n/2) via integer division
	share := float64(dailyVolume) * (1.0 / float64(position)) / hn
	result := int64(math.Round(share))
	if result < 1 {
		result = 1
	}
	return result
}

// estimateBfSPerDay approximates "buys from sell orders" flow for a specific
// arbitrage pair using observed S2B flow and current order-book imbalance.
// S2B/BfS ~= buyDepth/sellDepth to keep the ratio symmetric with book depth.
func estimateBfSPerDay(s2bPerDay float64, buyDepth, sellDepth int32) float64 {
	if s2bPerDay <= 0 || buyDepth <= 0 || sellDepth <= 0 {
		return 0
	}
	return s2bPerDay * float64(sellDepth) / float64(buyDepth)
}

// findSafeExecutionQuantity returns the largest executable and profitable quantity
// up to desiredQty based on order-book depth and expected fill prices.
// Predicate assumes profitability does not improve on larger quantities.
func findSafeExecutionQuantity(
	askOrdersAtBuy []esi.MarketOrder, // sell orders we buy from
	bidOrdersAtSell []esi.MarketOrder, // buy orders we sell into
	desiredQty int32,
	buyCostMult float64,
	sellRevenueMult float64,
) (int32, ExecutionPlanResult, ExecutionPlanResult, float64) {
	var zeroBuy ExecutionPlanResult
	var zeroSell ExecutionPlanResult
	if desiredQty <= 0 || len(askOrdersAtBuy) == 0 || len(bidOrdersAtSell) == 0 {
		return 0, zeroBuy, zeroSell, 0
	}

	eval := func(q int32) (bool, ExecutionPlanResult, ExecutionPlanResult, float64) {
		if q <= 0 {
			return false, zeroBuy, zeroSell, 0
		}
		planBuy := ComputeExecutionPlan(askOrdersAtBuy, q, true)
		planSell := ComputeExecutionPlan(bidOrdersAtSell, q, false)
		expectedProfit := expectedProfitForPlans(planBuy, planSell, q, buyCostMult, sellRevenueMult)
		ok := planBuy.CanFill && planSell.CanFill && expectedProfit > 0
		return ok, planBuy, planSell, expectedProfit
	}

	ok, planBuy, planSell, expectedProfit := eval(desiredQty)
	if ok {
		return desiredQty, planBuy, planSell, expectedProfit
	}

	maxFill := desiredQty
	if planBuy.TotalDepth < maxFill {
		maxFill = planBuy.TotalDepth
	}
	if planSell.TotalDepth < maxFill {
		maxFill = planSell.TotalDepth
	}
	if maxFill <= 0 {
		return 0, planBuy, planSell, 0
	}

	okOne, planBuyOne, planSellOne, expectedOne := eval(1)
	if !okOne {
		return 0, planBuyOne, planSellOne, 0
	}

	low := int32(1)
	bestBuy := planBuyOne
	bestSell := planSellOne
	bestExpected := expectedOne
	high := maxFill

	for low+1 < high {
		mid := low + (high-low)/2
		okMid, planBuyMid, planSellMid, expectedMid := eval(mid)
		if okMid {
			low = mid
			bestBuy = planBuyMid
			bestSell = planSellMid
			bestExpected = expectedMid
		} else {
			high = mid
		}
	}

	okHigh, planBuyHigh, planSellHigh, expectedHigh := eval(high)
	if okHigh {
		return high, planBuyHigh, planSellHigh, expectedHigh
	}
	return low, bestBuy, bestSell, bestExpected
}

func expectedProfitForPlans(
	planBuy ExecutionPlanResult,
	planSell ExecutionPlanResult,
	qty int32,
	buyCostMult float64,
	sellRevenueMult float64,
) float64 {
	if qty <= 0 || planBuy.ExpectedPrice <= 0 || planSell.ExpectedPrice <= 0 {
		return 0
	}
	effBuy := planBuy.ExpectedPrice * buyCostMult
	effSell := planSell.ExpectedPrice * sellRevenueMult
	return (effSell - effBuy) * float64(qty)
}

// sanitizeFloat replaces NaN/Inf with 0 to prevent JSON marshal errors.
func sanitizeFloat(f float64) float64 {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return f
}

func (s *Scanner) systemName(systemID int32) string {
	if sys, ok := s.SDE.Systems[systemID]; ok {
		return sys.Name
	}
	return fmt.Sprintf("System %d", systemID)
}

func (s *Scanner) regionName(regionID int32) string {
	if r, ok := s.SDE.Regions[regionID]; ok {
		return r.Name
	}
	return fmt.Sprintf("Region %d", regionID)
}

// enrichWithHistory fetches market history for top results and fills DailyVolume/Velocity/PriceTrend.
// regionID is the sell region (where we care about volume).
func (s *Scanner) enrichWithHistory(results []FlipResult, progress func(string)) {
	if s.History == nil || len(results) == 0 {
		return
	}

	progress("Fetching market history...")

	// Determine region for each result (sell side)
	type historyKey struct {
		regionID int32
		typeID   int32
	}
	type historyNeed struct {
		idx         int
		totalListed int32
	}
	needed := make(map[historyKey][]historyNeed) // key -> all result indices with total listed quantity
	totalNeeds := 0
	for i := range results {
		regionID := s.SDE.Universe.SystemRegion[results[i].SellSystemID]
		if regionID == 0 {
			continue
		}
		key := historyKey{regionID, results[i].TypeID}
		totalListed := results[i].SellOrderRemain + results[i].BuyOrderRemain
		needed[key] = append(needed[key], historyNeed{
			idx:         i,
			totalListed: totalListed,
		})
		totalNeeds++
	}

	// Fetch history concurrently (limited)
	type histResult struct {
		idx   int
		stats esi.MarketStats
	}
	ch := make(chan histResult, totalNeeds)
	sem := make(chan struct{}, 10) // limit concurrent history requests

	for key, needs := range needed {
		sem <- struct{}{}
		go func(k historyKey, ns []historyNeed) {
			defer func() { <-sem }()

			// Try cache first
			entries, ok := s.History.GetMarketHistory(k.regionID, k.typeID)
			if !ok {
				var err error
				entries, err = s.ESI.FetchMarketHistory(k.regionID, k.typeID)
				if err != nil {
					for _, n := range ns {
						ch <- histResult{n.idx, esi.MarketStats{}}
					}
					return
				}
				s.History.SetMarketHistory(k.regionID, k.typeID, entries)
			}

			for _, n := range ns {
				stats := esi.ComputeMarketStats(entries, n.totalListed)
				ch <- histResult{n.idx, stats}
			}
		}(key, needs)
	}

	for i := 0; i < totalNeeds; i++ {
		r := <-ch
		results[r.idx].DailyVolume = r.stats.DailyVolume
		results[r.idx].Velocity = sanitizeFloat(r.stats.Velocity)
		results[r.idx].PriceTrend = sanitizeFloat(r.stats.PriceTrend)
	}
}
