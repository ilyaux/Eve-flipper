package engine

import (
	"fmt"
	"log"
	"math"
	"sort"
	"sync"

	"eve-flipper/internal/esi"
	"eve-flipper/internal/sde"
)

const (
	// DefaultMaxResults is the default number of results returned when not specified.
	DefaultMaxResults = 100
	// UnreachableJumps is the fallback jump count when no path exists.
	UnreachableJumps = 999
)

// EffectiveMaxResults returns the max results limit, using DefaultMaxResults if v <= 0.
func EffectiveMaxResults(v int, defaultVal int) int {
	if v <= 0 {
		return defaultVal
	}
	return v
}

// HistoryProvider is an interface for fetching and caching market history.
type HistoryProvider interface {
	GetMarketHistory(regionID int32, typeID int32) ([]esi.HistoryEntry, bool)
	SetMarketHistory(regionID int32, typeID int32, entries []esi.HistoryEntry)
}

// Scanner orchestrates market scans using SDE data and the ESI client.
type Scanner struct {
	SDE     *sde.Data
	ESI     *esi.Client
	History HistoryProvider
}

// NewScanner creates a Scanner with the given static data and ESI client.
func NewScanner(data *sde.Data, client *esi.Client) *Scanner {
	return &Scanner{SDE: data, ESI: client}
}

// Scan finds profitable flip opportunities based on the given parameters.
func (s *Scanner) Scan(params ScanParams, progress func(string)) ([]FlipResult, error) {
	progress("Finding systems within radius...")
	// OPT: compute both BFS in parallel
	var buySystems, sellSystems map[int32]int
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		buySystems = s.SDE.Universe.SystemsWithinRadius(params.CurrentSystemID, params.BuyRadius)
	}()
	go func() {
		defer wg.Done()
		sellSystems = s.SDE.Universe.SystemsWithinRadius(params.CurrentSystemID, params.SellRadius)
	}()
	wg.Wait()

	buyRegions := s.SDE.Universe.RegionsInSet(buySystems)
	sellRegions := s.SDE.Universe.RegionsInSet(sellSystems)

	log.Printf("[DEBUG] Scan: buySystems=%d, sellSystems=%d, buyRegions=%d, sellRegions=%d",
		len(buySystems), len(sellSystems), len(buyRegions), len(sellRegions))

	// OPT: fetch buy and sell orders in parallel
	progress(fmt.Sprintf("Fetching orders from %d+%d regions...", len(buyRegions), len(sellRegions)))
	var sellOrders, buyOrders []esi.MarketOrder
	wg.Add(2)
	go func() {
		defer wg.Done()
		sellOrders = s.fetchOrders(buyRegions, "sell", buySystems)
	}()
	go func() {
		defer wg.Done()
		buyOrders = s.fetchOrders(sellRegions, "buy", sellSystems)
	}()
	wg.Wait()

	return s.calculateResults(params, sellOrders, buyOrders, buySystems, progress)
}

// ScanMultiRegion finds profitable flip opportunities across whole regions.
func (s *Scanner) ScanMultiRegion(params ScanParams, progress func(string)) ([]FlipResult, error) {
	progress("Finding regions by radius...")
	var buySystemsRadius, sellSystemsRadius map[int32]int
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		buySystemsRadius = s.SDE.Universe.SystemsWithinRadius(params.CurrentSystemID, params.BuyRadius)
	}()
	go func() {
		defer wg.Done()
		sellSystemsRadius = s.SDE.Universe.SystemsWithinRadius(params.CurrentSystemID, params.SellRadius)
	}()
	wg.Wait()

	buyRegions := s.SDE.Universe.RegionsInSet(buySystemsRadius)
	sellRegions := s.SDE.Universe.RegionsInSet(sellSystemsRadius)
	buySystems := s.SDE.Universe.SystemsInRegions(buyRegions)
	sellSystems := s.SDE.Universe.SystemsInRegions(sellRegions)

	progress(fmt.Sprintf("Fetching orders from %d+%d regions...", len(buyRegions), len(sellRegions)))
	var sellOrders, buyOrders []esi.MarketOrder
	wg.Add(2)
	go func() {
		defer wg.Done()
		sellOrders = s.fetchOrders(buyRegions, "sell", buySystems)
	}()
	go func() {
		defer wg.Done()
		buyOrders = s.fetchOrders(sellRegions, "buy", sellSystems)
	}()
	wg.Wait()

	// For multi-region, use buySystemsRadius for BFS distances (from origin)
	return s.calculateResults(params, sellOrders, buyOrders, buySystemsRadius, progress)
}

// calculateResults is the shared profit calculation logic.
// bfsDistances = pre-computed distances from origin (used for buyJumps lookup).
func (s *Scanner) calculateResults(
	params ScanParams,
	sellOrders, buyOrders []esi.MarketOrder,
	bfsDistances map[int32]int,
	progress func(string),
) ([]FlipResult, error) {
	log.Printf("[DEBUG] calculateResults: %d sell orders, %d buy orders", len(sellOrders), len(buyOrders))

	// OPT: build type-grouped maps with only min-sell and max-buy per type
	// This avoids storing all orders and does a single pass
	type sellInfo struct {
		Price        float64
		VolumeRemain int32
		LocationID   int64
		SystemID     int32
		OrderCount   int // number of sell orders at this location for this type
	}
	type buyInfo struct {
		Price        float64
		VolumeRemain int32
		LocationID   int64
		SystemID     int32
		OrderCount   int // number of buy orders at this location for this type
	}

	// Single pass: find cheapest sell per type
	cheapestSell := make(map[int32]sellInfo)
	// Count sell orders per type+location
	type locKey struct {
		typeID     int32
		locationID int64
	}
	sellCounts := make(map[locKey]int)
	for _, o := range sellOrders {
		sellCounts[locKey{o.TypeID, o.LocationID}]++
		if cur, ok := cheapestSell[o.TypeID]; !ok || o.Price < cur.Price {
			cheapestSell[o.TypeID] = sellInfo{o.Price, o.VolumeRemain, o.LocationID, o.SystemID, 0}
		}
	}
	// Fill order counts for cheapest sell locations
	for tid, info := range cheapestSell {
		info.OrderCount = sellCounts[locKey{tid, info.LocationID}]
		cheapestSell[tid] = info
	}

	// Single pass: find highest buy per type
	highestBuy := make(map[int32]buyInfo)
	buyCounts := make(map[locKey]int)
	for _, o := range buyOrders {
		buyCounts[locKey{o.TypeID, o.LocationID}]++
		if cur, ok := highestBuy[o.TypeID]; !ok || o.Price > cur.Price {
			highestBuy[o.TypeID] = buyInfo{o.Price, o.VolumeRemain, o.LocationID, o.SystemID, 0}
		}
	}
	for tid, info := range highestBuy {
		info.OrderCount = buyCounts[locKey{tid, info.LocationID}]
		highestBuy[tid] = info
	}

	log.Printf("[DEBUG] cheapestSell: %d types, highestBuy: %d types", len(cheapestSell), len(highestBuy))

	progress("Calculating profits...")
	taxMult := 1.0 - params.SalesTaxPercent/100
	if taxMult < 0 {
		taxMult = 0
	}

	var results []FlipResult

	for typeID, sell := range cheapestSell {
		buy, ok := highestBuy[typeID]
		if !ok || buy.Price <= sell.Price {
			continue
		}

		// OPT: early margin check before item lookup
		effectiveSellPrice := buy.Price * taxMult
		profitPerUnit := effectiveSellPrice - sell.Price
		if profitPerUnit <= 0 {
			continue
		}
		margin := profitPerUnit / sell.Price * 100
		if margin < params.MinMargin {
			continue
		}

		itemType, ok := s.SDE.Types[typeID]
		if !ok || itemType.Volume <= 0 {
			continue
		}

		unitsF := math.Floor(params.CargoCapacity / itemType.Volume)
		if unitsF > math.MaxInt32 {
			unitsF = math.MaxInt32
		}
		units := int32(unitsF)
		if units <= 0 {
			continue
		}
		if sell.VolumeRemain < units {
			units = sell.VolumeRemain
		}
		if buy.VolumeRemain < units {
			units = buy.VolumeRemain
		}

		// MaxInvestment filter
		investment := sell.Price * float64(units)
		if params.MaxInvestment > 0 && investment > params.MaxInvestment {
			units = int32(params.MaxInvestment / sell.Price)
			if units <= 0 {
				continue
			}
		}

		totalProfit := profitPerUnit * float64(units)

		// OPT: use BFS distances when available, fallback to Dijkstra
		buyJumps := s.jumpsBetweenWithBFS(params.CurrentSystemID, sell.SystemID, bfsDistances)
		sellJumps := s.jumpsBetween(sell.SystemID, buy.SystemID)
		totalJumps := buyJumps + sellJumps

		var profitPerJump float64
		if totalJumps > 0 {
			profitPerJump = totalProfit / float64(totalJumps)
		}

		results = append(results, FlipResult{
			TypeID:          typeID,
			TypeName:        itemType.Name,
			Volume:          itemType.Volume,
			BuyPrice:        sell.Price,
			BuyStation:      "",
			BuySystemName:   s.systemName(sell.SystemID),
			BuySystemID:     sell.SystemID,
			BuyLocationID:   sell.LocationID,
			SellPrice:       buy.Price,
			SellStation:     "",
			SellSystemName:  s.systemName(buy.SystemID),
			SellSystemID:    buy.SystemID,
			SellLocationID:  buy.LocationID,
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
			BuyCompetitors:  sell.OrderCount,  // sell orders at buy station (we compete to buy)
			SellCompetitors: buy.OrderCount,   // buy orders at sell station (we compete to sell)
		})
	}

	log.Printf("[DEBUG] found %d results before sort/trim", len(results))

	// Sort by profit, keep top 100
	sort.Slice(results, func(i, j int) bool {
		return results[i].TotalProfit > results[j].TotalProfit
	})
	limit := EffectiveMaxResults(params.MaxResults, DefaultMaxResults)
	if len(results) > limit {
		results = results[:limit]
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
		for i := range results {
			results[i].BuyStation = s.ESI.StationName(results[i].BuyLocationID)
			results[i].SellStation = s.ESI.StationName(results[i].SellLocationID)
		}
	}

	// Enrich with market history (volume, velocity, trend)
	s.enrichWithHistory(results, progress)

	// Post-filter: min daily volume
	if params.MinDailyVolume > 0 {
		filtered := results[:0]
		for _, r := range results {
			if r.DailyVolume >= params.MinDailyVolume {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}

	progress(fmt.Sprintf("Found %d profitable trades", len(results)))
	return results, nil
}

func (s *Scanner) fetchOrders(regions map[int32]bool, orderType string, validSystems map[int32]int) []esi.MarketOrder {
	var mu sync.Mutex
	var all []esi.MarketOrder
	var wg sync.WaitGroup

	for regionID := range regions {
		wg.Add(1)
		go func(rid int32) {
			defer wg.Done()
			orders, err := s.ESI.FetchRegionOrders(rid, orderType)
			if err != nil {
				return
			}
			var filtered []esi.MarketOrder
			for _, o := range orders {
				if _, ok := validSystems[o.SystemID]; ok {
					filtered = append(filtered, o)
				}
			}
			mu.Lock()
			all = append(all, filtered...)
			mu.Unlock()
		}(regionID)
	}
	wg.Wait()
	log.Printf("[DEBUG] fetchOrders(%s): %d orders after filtering", orderType, len(all))
	return all
}

func (s *Scanner) jumpsBetween(from, to int32) int {
	d := s.SDE.Universe.ShortestPath(from, to)
	if d < 0 {
		return UnreachableJumps
	}
	return d
}

// jumpsBetweenWithBFS uses pre-computed BFS distances when 'from' is the origin.
func (s *Scanner) jumpsBetweenWithBFS(from, to int32, bfsDistances map[int32]int) int {
	if d, ok := bfsDistances[to]; ok {
		return d
	}
	return s.jumpsBetween(from, to)
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
	needed := make(map[historyKey]int) // key -> index in results
	for i := range results {
		regionID := s.SDE.Universe.SystemRegion[results[i].SellSystemID]
		if regionID == 0 {
			continue
		}
		key := historyKey{regionID, results[i].TypeID}
		needed[key] = i
	}

	// Fetch history concurrently (limited)
	type histResult struct {
		idx     int
		stats   esi.MarketStats
	}
	ch := make(chan histResult, len(needed))
	sem := make(chan struct{}, 10) // limit concurrent history requests

	for key, idx := range needed {
		sem <- struct{}{}
		go func(k historyKey, i int) {
			defer func() { <-sem }()

			// Try cache first
			entries, ok := s.History.GetMarketHistory(k.regionID, k.typeID)
			if !ok {
				var err error
				entries, err = s.ESI.FetchMarketHistory(k.regionID, k.typeID)
				if err != nil {
					ch <- histResult{i, esi.MarketStats{}}
					return
				}
				s.History.SetMarketHistory(k.regionID, k.typeID, entries)
			}

			totalListed := results[i].SellOrderRemain + results[i].BuyOrderRemain
			stats := esi.ComputeMarketStats(entries, totalListed)
			ch <- histResult{i, stats}
		}(key, idx)
	}

	for range needed {
		r := <-ch
		results[r.idx].DailyVolume = r.stats.DailyVolume
		results[r.idx].Velocity = sanitizeFloat(r.stats.Velocity)
		results[r.idx].PriceTrend = sanitizeFloat(r.stats.PriceTrend)
	}
}
