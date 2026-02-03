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

			effectiveSell := buy.Price * taxMult
			profitPerUnit := effectiveSell - sell.Price
			if profitPerUnit <= 0 {
				continue
			}
			margin := profitPerUnit / sell.Price * 100
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
			jumps := s.jumpsBetween(fromSystemID, buySystemID)
			if jumps <= 0 || jumps > MaxTradeJumps {
				continue
			}

			ppj := profit / float64(jumps)

			candidates = append(candidates, candidate{
				hop: RouteHop{
					SystemName:     s.systemName(fromSystemID),
					SystemID:       fromSystemID,
					LocationID:     sell.LocationID,
					DestSystemID:   buySystemID,
					DestSystemName: s.systemName(buySystemID),
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

	regionID, ok := s.SDE.Universe.SystemRegion[systemID]
	if !ok || regionID == 0 {
		return nil, fmt.Errorf("region not found for system %d", systemID)
	}

	// Fetch all orders for the region once
	progress("Fetching market orders...")
	regions := map[int32]bool{regionID: true}
	allSystems := s.SDE.Universe.SystemsInRegions(regions)

	var sellOrders, buyOrders []esi.MarketOrder
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		orders, err := s.ESI.FetchRegionOrders(regionID, "sell")
		if err != nil {
			return
		}
		for _, o := range orders {
			if _, ok := allSystems[o.SystemID]; ok {
				sellOrders = append(sellOrders, o)
			}
		}
	}()
	go func() {
		defer wg.Done()
		orders, err := s.ESI.FetchRegionOrders(regionID, "buy")
		if err != nil {
			return
		}
		for _, o := range orders {
			if _, ok := allSystems[o.SystemID]; ok {
				buyOrders = append(buyOrders, o)
			}
		}
	}()
	wg.Wait()

	log.Printf("[Route] Fetched %d sell, %d buy orders", len(sellOrders), len(buyOrders))
	progress("Building order index...")
	idx := buildOrderIndex(sellOrders, buyOrders)

	const (
		beamWidth    = 20 // keep top N partial routes per level
		branchFactor = 5  // explore top N trades per system
		maxRoutes    = 50 // max completed routes to return
	)

	type partialRoute struct {
		hops        []RouteHop
		totalProfit float64
		totalJumps  int
		lastSystem  int32
	}

	// Seed with initial trades from start system
	progress("Exploring routes...")
	initialTrades := s.findBestTrades(idx, systemID, params, branchFactor*4) // wider initial search
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

	// Deduplicate and sort
	sort.Slice(completedRoutes, func(i, j int) bool {
		return completedRoutes[i].TotalProfit > completedRoutes[j].TotalProfit
	})
	routeLimit := EffectiveMaxResults(params.MaxResults, maxRoutes)
	if len(completedRoutes) > routeLimit {
		completedRoutes = completedRoutes[:routeLimit]
	}

	// Prefetch station names for all hops
	if len(completedRoutes) > 0 {
		progress("Fetching station names...")
		stations := make(map[int64]bool)
		for _, route := range completedRoutes {
			for _, hop := range route.Hops {
				stations[hop.LocationID] = true
			}
		}
		s.ESI.PrefetchStationNames(stations)
		for i := range completedRoutes {
			for j := range completedRoutes[i].Hops {
				completedRoutes[i].Hops[j].StationName = s.ESI.StationName(completedRoutes[i].Hops[j].LocationID)
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
