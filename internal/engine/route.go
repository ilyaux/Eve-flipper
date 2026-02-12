package engine

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"sync"

	"eve-flipper/internal/esi"
)

const (
	// MaxTradeJumps is the maximum jump distance for a single trade hop.
	MaxTradeJumps = 50
	// MaxRouteSearchRegions limits how many nearest regions we load market orders from
	// for route search. Keeps inter-region support while bounding ESI load.
	MaxRouteSearchRegions = 12
)

// orderIndex is a pre-built index of best sell/buy prices per system per type.
type orderIndex struct {
	// cheapestSell[systemID][typeID] = best sell order info
	cheapestSell map[int32]map[int32]orderEntry
	// highestBuy[systemID][typeID] = best buy order info
	highestBuy map[int32]map[int32]orderEntry
}

type orderEntry struct {
	Price        float64
	VolumeRemain int32
	LocationID   int64
}

type regionDistance struct {
	regionID int32
	dist     int
}

func selectClosestRouteRegions(systemRegion map[int32]int32, systems map[int32]int, maxRegions int) map[int32]bool {
	minDistByRegion := make(map[int32]int)
	for systemID, dist := range systems {
		regionID, ok := systemRegion[systemID]
		if !ok || regionID == 0 {
			continue
		}
		cur, exists := minDistByRegion[regionID]
		if !exists || dist < cur {
			minDistByRegion[regionID] = dist
		}
	}

	ordered := make([]regionDistance, 0, len(minDistByRegion))
	for regionID, dist := range minDistByRegion {
		ordered = append(ordered, regionDistance{regionID: regionID, dist: dist})
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].dist == ordered[j].dist {
			return ordered[i].regionID < ordered[j].regionID
		}
		return ordered[i].dist < ordered[j].dist
	})

	if maxRegions > 0 && len(ordered) > maxRegions {
		ordered = ordered[:maxRegions]
	}

	out := make(map[int32]bool, len(ordered))
	for _, r := range ordered {
		out[r.regionID] = true
	}
	return out
}

// buildOrderIndex builds per-system order maps from raw orders.
func buildOrderIndex(sellOrders, buyOrders []esi.MarketOrder) *orderIndex {
	idx := &orderIndex{
		cheapestSell: make(map[int32]map[int32]orderEntry),
		highestBuy:   make(map[int32]map[int32]orderEntry),
	}

	for _, o := range sellOrders {
		byType, ok := idx.cheapestSell[o.SystemID]
		if !ok {
			byType = make(map[int32]orderEntry)
			idx.cheapestSell[o.SystemID] = byType
		}
		if cur, ok := byType[o.TypeID]; !ok || o.Price < cur.Price {
			byType[o.TypeID] = orderEntry{o.Price, o.VolumeRemain, o.LocationID}
		}
	}

	for _, o := range buyOrders {
		byType, ok := idx.highestBuy[o.SystemID]
		if !ok {
			byType = make(map[int32]orderEntry)
			idx.highestBuy[o.SystemID] = byType
		}
		if cur, ok := byType[o.TypeID]; !ok || o.Price > cur.Price {
			byType[o.TypeID] = orderEntry{o.Price, o.VolumeRemain, o.LocationID}
		}
	}

	return idx
}

// findBestTrades finds the best N trades: buy at fromSystem, sell at any other system.
func (s *Scanner) findBestTrades(idx *orderIndex, fromSystemID int32, params RouteParams, topN int) []RouteHop {
	sellsHere, ok := idx.cheapestSell[fromSystemID]
	if !ok {
		return nil
	}

	taxMult := 1.0 - params.SalesTaxPercent/100
	if taxMult < 0 {
		taxMult = 0
	}
	brokerFeeRate := params.BrokerFeePercent / 100 // fraction e.g. 0.03 for 3%

	type candidate struct {
		hop   RouteHop
		score float64 // profit per jump for ranking
	}

	var candidates []candidate

	for typeID, sell := range sellsHere {
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

		// Find the best buy order for this type across all systems
		for buySystemID, buysByType := range idx.highestBuy {
			if buySystemID == fromSystemID {
				continue
			}
			buy, ok := buysByType[typeID]
			if !ok {
				continue
			}

			effectiveBuy := sell.Price * (1 + brokerFeeRate)
			effectiveSell := buy.Price * taxMult * (1 - brokerFeeRate)
			profitPerUnit := effectiveSell - effectiveBuy
			if profitPerUnit <= 0 {
				continue
			}
			margin := profitPerUnit / effectiveBuy * 100
			if margin < params.MinMargin {
				continue
			}

			actualUnits := units
			if buy.VolumeRemain < actualUnits {
				actualUnits = buy.VolumeRemain
			}
			if actualUnits <= 0 {
				continue
			}

			profit := profitPerUnit * float64(actualUnits)
			jumps := s.jumpsBetweenWithSecurity(fromSystemID, buySystemID, params.MinRouteSecurity)
			if jumps <= 0 || jumps > MaxTradeJumps {
				continue
			}

			ppj := profit / float64(jumps)

			regionID := int32(0)
			if sys, ok := s.SDE.Systems[fromSystemID]; ok {
				regionID = sys.RegionID
			}
			candidates = append(candidates, candidate{
				hop: RouteHop{
					SystemName:     s.systemName(fromSystemID),
					SystemID:       fromSystemID,
					RegionID:       regionID,
					LocationID:     sell.LocationID,
					DestSystemID:   buySystemID,
					DestSystemName: s.systemName(buySystemID),
					DestLocationID: buy.LocationID,
					TypeName:       itemType.Name,
					TypeID:         typeID,
					BuyPrice:       sell.Price,
					SellPrice:      buy.Price,
					Units:          actualUnits,
					Profit:         profit,
					Jumps:          jumps,
				},
				score: ppj,
			})
		}
	}

	// Sort by profit per jump, take top N
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})
	if len(candidates) > topN {
		candidates = candidates[:topN]
	}

	hops := make([]RouteHop, len(candidates))
	for i, c := range candidates {
		hops[i] = c.hop
	}
	return hops
}

// FindRoutes finds the most profitable multi-hop trade routes using beam search.
func (s *Scanner) FindRoutes(params RouteParams, progress func(string)) ([]RouteResult, error) {
	systemID, ok := s.SDE.SystemByName[strings.ToLower(params.SystemName)]
	if !ok {
		return nil, fmt.Errorf("system not found: %s", params.SystemName)
	}

	searchRadius := params.MaxHops * MaxTradeJumps
	if searchRadius < MaxTradeJumps {
		searchRadius = MaxTradeJumps
	}
	var reachableSystems map[int32]int
	if params.MinRouteSecurity > 0 {
		reachableSystems = s.SDE.Universe.SystemsWithinRadiusMinSecurity(systemID, searchRadius, params.MinRouteSecurity)
	} else {
		reachableSystems = s.SDE.Universe.SystemsWithinRadius(systemID, searchRadius)
	}
	if len(reachableSystems) == 0 {
		return nil, fmt.Errorf("no reachable systems from system %d", systemID)
	}

	// Select the nearest reachable regions (inter-region support with bounded load).
	regions := selectClosestRouteRegions(s.SDE.Universe.SystemRegion, reachableSystems, MaxRouteSearchRegions)
	if len(regions) == 0 {
		return nil, fmt.Errorf("no reachable regions from system %d", systemID)
	}
	searchSystems := make(map[int32]int, len(reachableSystems))
	for sysID, dist := range reachableSystems {
		if regionID, ok := s.SDE.Universe.SystemRegion[sysID]; ok && regions[regionID] {
			searchSystems[sysID] = dist
		}
	}

	progress(fmt.Sprintf("Fetching market orders from %d regions...", len(regions)))

	var sellOrders, buyOrders []esi.MarketOrder
	fetchBySide := func(orderType string) []esi.MarketOrder {
		var out []esi.MarketOrder
		var outMu sync.Mutex
		var wg sync.WaitGroup
		sem := make(chan struct{}, 4)

		for regionID := range regions {
			wg.Add(1)
			go func(rid int32) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				orders, err := s.ESI.FetchRegionOrders(rid, orderType)
				if err != nil {
					log.Printf("[Route] FetchRegionOrders(%d,%s) error: %v", rid, orderType, err)
					return
				}

				filtered := make([]esi.MarketOrder, 0, len(orders))
				for _, o := range orders {
					if _, ok := searchSystems[o.SystemID]; ok {
						filtered = append(filtered, o)
					}
				}
				if len(filtered) == 0 {
					return
				}

				outMu.Lock()
				out = append(out, filtered...)
				outMu.Unlock()
			}(regionID)
		}
		wg.Wait()
		return out
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		sellOrders = fetchBySide("sell")
	}()
	go func() {
		defer wg.Done()
		buyOrders = fetchBySide("buy")
	}()
	wg.Wait()

	log.Printf("[Route] Fetched %d sell, %d buy orders across %d regions (%d systems in envelope)",
		len(sellOrders), len(buyOrders), len(regions), len(searchSystems))
	progress("Building order index...")
	idx := buildOrderIndex(sellOrders, buyOrders)

	// Scale beam search parameters based on requested depth
	beamWidth := 50    // keep top N partial routes per level
	branchFactor := 10 // explore top N trades per system

	type partialRoute struct {
		hops        []RouteHop
		totalProfit float64
		totalJumps  int
		lastSystem  int32
	}

	// Seed with initial trades from start system
	progress("Exploring routes...")
	initialTrades := s.findBestTrades(idx, systemID, params, branchFactor*3) // wider initial search
	if len(initialTrades) == 0 {
		progress("No profitable trades found")
		return []RouteResult{}, nil
	}

	// Initialize beam with hop-1 routes
	beam := make([]partialRoute, 0, len(initialTrades))
	for _, hop := range initialTrades {
		beam = append(beam, partialRoute{
			hops:        []RouteHop{hop},
			totalProfit: hop.Profit,
			totalJumps:  hop.Jumps,
			lastSystem:  hop.DestSystemID,
		})
	}

	var completedRoutes []RouteResult
	explored := 0

	// Expand beam level by level
	for depth := 1; depth < params.MaxHops; depth++ {
		if len(beam) == 0 {
			break
		}

		// Collect completed routes (if we've reached min depth)
		if depth >= params.MinHops {
			for _, pr := range beam {
				ppj := sanitizeFloat(pr.totalProfit / float64(pr.totalJumps))
				completedRoutes = append(completedRoutes, RouteResult{
					Hops:          copyHops(pr.hops),
					TotalProfit:   pr.totalProfit,
					TotalJumps:    pr.totalJumps,
					ProfitPerJump: ppj,
					HopCount:      len(pr.hops),
				})
			}
		}

		var nextBeam []partialRoute
		for _, pr := range beam {
			trades := s.findBestTrades(idx, pr.lastSystem, params, branchFactor)
			for _, hop := range trades {
				// Avoid revisiting systems
				revisit := false
				for _, h := range pr.hops {
					if h.SystemID == hop.DestSystemID {
						revisit = true
						break
					}
				}
				if revisit {
					continue
				}

				newHops := make([]RouteHop, len(pr.hops)+1)
				copy(newHops, pr.hops)
				newHops[len(pr.hops)] = hop

				nextBeam = append(nextBeam, partialRoute{
					hops:        newHops,
					totalProfit: pr.totalProfit + hop.Profit,
					totalJumps:  pr.totalJumps + hop.Jumps,
					lastSystem:  hop.DestSystemID,
				})
				explored++
			}
		}

		// Prune to beamWidth
		sort.Slice(nextBeam, func(i, j int) bool {
			return nextBeam[i].totalProfit > nextBeam[j].totalProfit
		})
		if len(nextBeam) > beamWidth {
			nextBeam = nextBeam[:beamWidth]
		}
		beam = nextBeam

		progress(fmt.Sprintf("Exploring routes: depth %d/%d, %d candidates...", depth+1, params.MaxHops, explored))
	}

	// Add final beam as completed routes
	for _, pr := range beam {
		if len(pr.hops) >= params.MinHops {
			ppj := sanitizeFloat(pr.totalProfit / float64(pr.totalJumps))
			completedRoutes = append(completedRoutes, RouteResult{
				Hops:          pr.hops,
				TotalProfit:   pr.totalProfit,
				TotalJumps:    pr.totalJumps,
				ProfitPerJump: ppj,
				HopCount:      len(pr.hops),
			})
		}
	}

	// Deduplicate: keep highest-profit version of routes with same hop sequence
	seen := make(map[string]bool)
	var unique []RouteResult
	// Sort by profit desc first so we keep the best version
	sort.Slice(completedRoutes, func(i, j int) bool {
		return completedRoutes[i].TotalProfit > completedRoutes[j].TotalProfit
	})
	for _, r := range completedRoutes {
		key := routeKey(r)
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, r)
	}
	completedRoutes = unique

	// Cap to prevent server overload on route results
	if len(completedRoutes) > MaxUnlimitedResults {
		completedRoutes = completedRoutes[:MaxUnlimitedResults]
	}

	// Prefetch station names for all hops (buy and sell stations)
	if len(completedRoutes) > 0 {
		progress("Fetching station names...")
		stations := make(map[int64]bool)
		for _, route := range completedRoutes {
			for _, hop := range route.Hops {
				stations[hop.LocationID] = true
				if hop.DestLocationID != 0 {
					stations[hop.DestLocationID] = true
				}
			}
		}
		s.ESI.PrefetchStationNames(stations)
		for i := range completedRoutes {
			for j := range completedRoutes[i].Hops {
				completedRoutes[i].Hops[j].StationName = s.ESI.StationName(completedRoutes[i].Hops[j].LocationID)
				if completedRoutes[i].Hops[j].DestLocationID != 0 {
					completedRoutes[i].Hops[j].DestStationName = s.ESI.StationName(completedRoutes[i].Hops[j].DestLocationID)
				}
			}
		}
	}

	progress(fmt.Sprintf("Found %d routes", len(completedRoutes)))
	log.Printf("[Route] Found %d routes, explored %d candidates", len(completedRoutes), explored)
	return completedRoutes, nil
}

func copyHops(hops []RouteHop) []RouteHop {
	c := make([]RouteHop, len(hops))
	copy(c, hops)
	return c
}

// routeKey generates a unique string key for a route based on the sequence of systems and items.
func routeKey(r RouteResult) string {
	parts := make([]string, len(r.Hops))
	for i, h := range r.Hops {
		parts[i] = fmt.Sprintf("%d>%d:%d", h.SystemID, h.DestSystemID, h.TypeID)
	}
	return strings.Join(parts, "|")
}
