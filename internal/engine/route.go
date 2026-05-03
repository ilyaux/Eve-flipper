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
	// Allow at most this many deadhead jumps before a profitable hop.
	maxEmptyHopJumps = MaxTradeJumps
	// Limit candidate source systems when empty hops are enabled.
	maxEmptyHopSources = 12
)

// orderIndex is a pre-built index of best sell/buy prices per system per type.
type orderIndex struct {
	// cheapestSell[systemID][typeID] = best sell order info
	cheapestSell map[int32]map[int32]orderEntry
	// highestBuy[systemID][typeID] = best buy orders sorted by price desc
	highestBuy map[int32]map[int32][]orderEntry
	// Full books by concrete system/type/location for slippage-aware route hops.
	sellOrdersByLocation map[routeBookKey][]esi.MarketOrder
	buyOrdersByLocation  map[routeBookKey][]esi.MarketOrder
}

type orderEntry struct {
	Price        float64
	VolumeRemain int32
	MinVolume    int32
	LocationID   int64
}

type routeBookKey struct {
	systemID   int32
	typeID     int32
	locationID int64
}

type regionDistance struct {
	regionID int32
	dist     int
}

type sourceSystemCandidate struct {
	systemID   int32
	emptyJumps int
}

func routeMinISKPerJumpPass(
	minISKPerJump float64,
	profit float64,
	totalJumps int,
	allowByTargetProgress bool,
) bool {
	if totalJumps <= 0 {
		return false
	}
	if minISKPerJump <= 0 {
		return true
	}
	if (profit / float64(totalJumps)) >= minISKPerJump {
		return true
	}
	return allowByTargetProgress
}

func routeFilterJumpCountForTarget(totalTradeJumps, targetJumps int, hasTarget bool) int {
	if hasTarget {
		return max(1, totalTradeJumps)
	}
	return max(1, totalTradeJumps+targetJumps)
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
// This legacy helper keeps historical behavior (structures included).
func buildOrderIndex(sellOrders, buyOrders []esi.MarketOrder) *orderIndex {
	return buildOrderIndexWithFilters(sellOrders, buyOrders, true)
}

// buildOrderIndexWithFilters builds per-system order maps and applies route-level order filters.
func buildOrderIndexWithFilters(sellOrders, buyOrders []esi.MarketOrder, includeStructures bool) *orderIndex {
	idx := &orderIndex{
		cheapestSell:         make(map[int32]map[int32]orderEntry),
		highestBuy:           make(map[int32]map[int32][]orderEntry),
		sellOrdersByLocation: make(map[routeBookKey][]esi.MarketOrder),
		buyOrdersByLocation:  make(map[routeBookKey][]esi.MarketOrder),
	}

	for _, o := range sellOrders {
		if isMarketDisabledType(o.TypeID) {
			continue
		}
		if !includeStructures && isPlayerStructureLocationID(o.LocationID) {
			continue
		}
		o.IsBuyOrder = false
		key := routeBookKey{systemID: o.SystemID, typeID: o.TypeID, locationID: o.LocationID}
		idx.sellOrdersByLocation[key] = append(idx.sellOrdersByLocation[key], o)
		byType, ok := idx.cheapestSell[o.SystemID]
		if !ok {
			byType = make(map[int32]orderEntry)
			idx.cheapestSell[o.SystemID] = byType
		}
		if cur, ok := byType[o.TypeID]; !ok || o.Price < cur.Price {
			byType[o.TypeID] = orderEntry{
				Price:        o.Price,
				VolumeRemain: o.VolumeRemain,
				MinVolume:    o.MinVolume,
				LocationID:   o.LocationID,
			}
		}
	}

	for _, o := range buyOrders {
		if isMarketDisabledType(o.TypeID) {
			continue
		}
		if !includeStructures && isPlayerStructureLocationID(o.LocationID) {
			continue
		}
		o.IsBuyOrder = true
		key := routeBookKey{systemID: o.SystemID, typeID: o.TypeID, locationID: o.LocationID}
		idx.buyOrdersByLocation[key] = append(idx.buyOrdersByLocation[key], o)
		byType, ok := idx.highestBuy[o.SystemID]
		if !ok {
			byType = make(map[int32][]orderEntry)
			idx.highestBuy[o.SystemID] = byType
		}
		byType[o.TypeID] = append(byType[o.TypeID], orderEntry{
			Price:        o.Price,
			VolumeRemain: o.VolumeRemain,
			MinVolume:    o.MinVolume,
			LocationID:   o.LocationID,
		})
	}

	// Normalize buy ladders: highest price first, then lowest min volume.
	// Keep only the top levels to bound memory and evaluation cost.
	const maxBuyLevelsPerSystemType = 8
	for _, byType := range idx.highestBuy {
		for typeID, entries := range byType {
			sort.Slice(entries, func(i, j int) bool {
				if entries[i].Price == entries[j].Price {
					if entries[i].MinVolume == entries[j].MinVolume {
						return entries[i].VolumeRemain > entries[j].VolumeRemain
					}
					return entries[i].MinVolume < entries[j].MinVolume
				}
				return entries[i].Price > entries[j].Price
			})
			if len(entries) > maxBuyLevelsPerSystemType {
				entries = entries[:maxBuyLevelsPerSystemType]
			}
			byType[typeID] = entries
		}
	}

	return idx
}

func (s *Scanner) nearestTradeSourceSystems(
	idx *orderIndex,
	fromSystemID int32,
	params RouteParams,
) []sourceSystemCandidate {
	ignoredSystems := ignoredSystemSetFromIDs(params.IgnoredSystemIDs)
	if len(ignoredSystems) > 0 && ignoredSystems[fromSystemID] {
		return nil
	}
	sources := []sourceSystemCandidate{{systemID: fromSystemID, emptyJumps: 0}}
	if !params.AllowEmptyHops {
		return sources
	}

	var reachable map[int32]int
	if params.MinRouteSecurity > 0 {
		reachable = s.SDE.Universe.SystemsWithinRadiusMinSecurity(fromSystemID, maxEmptyHopJumps, params.MinRouteSecurity)
	} else {
		reachable = s.SDE.Universe.SystemsWithinRadius(fromSystemID, maxEmptyHopJumps)
	}
	reachable = filterSystemDistanceMap(reachable, ignoredSystems)
	if len(reachable) == 0 {
		return sources
	}

	extra := make([]sourceSystemCandidate, 0, len(reachable))
	for systemID, jumps := range reachable {
		if systemID == fromSystemID || jumps <= 0 {
			continue
		}
		if _, hasSell := idx.cheapestSell[systemID]; !hasSell {
			continue
		}
		extra = append(extra, sourceSystemCandidate{systemID: systemID, emptyJumps: jumps})
	}
	sort.Slice(extra, func(i, j int) bool {
		if extra[i].emptyJumps == extra[j].emptyJumps {
			return extra[i].systemID < extra[j].systemID
		}
		return extra[i].emptyJumps < extra[j].emptyJumps
	})
	if len(extra) > maxEmptyHopSources {
		extra = extra[:maxEmptyHopSources]
	}
	return append(sources, extra...)
}

// findBestTrades finds the best N trades: buy at fromSystem (or nearby with empty hops), sell at any other system.
func (s *Scanner) findBestTrades(idx *orderIndex, fromSystemID int32, params RouteParams, topN int) []RouteHop {
	sources := s.nearestTradeSourceSystems(idx, fromSystemID, params)
	return s.findBestTradesFromSources(idx, sources, params, topN)
}

func (s *Scanner) findBestTradesFromSources(
	idx *orderIndex,
	sources []sourceSystemCandidate,
	params RouteParams,
	topN int,
) []RouteHop {
	buyCostMult, sellRevenueMult := tradeFeeMultipliers(tradeFeeInputs{
		SplitTradeFees:       params.SplitTradeFees,
		BrokerFeePercent:     params.BrokerFeePercent,
		SalesTaxPercent:      params.SalesTaxPercent,
		BuyBrokerFeePercent:  params.BuyBrokerFeePercent,
		SellBrokerFeePercent: params.SellBrokerFeePercent,
		BuySalesTaxPercent:   params.BuySalesTaxPercent,
		SellSalesTaxPercent:  params.SellSalesTaxPercent,
	})

	type candidate struct {
		hop   RouteHop
		score float64 // total profit for consistent beam objective
	}

	var candidates []candidate
	ignoredSystems := ignoredSystemSetFromIDs(params.IgnoredSystemIDs)
	for _, source := range sources {
		if len(ignoredSystems) > 0 && ignoredSystems[source.systemID] {
			continue
		}
		sellsHere, ok := idx.cheapestSell[source.systemID]
		if !ok {
			continue
		}

		for typeID, sell := range sellsHere {
			if isMarketDisabledType(typeID) {
				continue
			}
			itemType, ok := s.SDE.Types[typeID]
			if !ok || itemType.Volume <= 0 {
				continue
			}

			routeCargoCapacity := params.EffectiveRouteCargoCapacity()
			units := int32(math.MaxInt32)
			if routeCargoCapacity > 0 {
				unitsF := math.Floor(routeCargoCapacity / itemType.Volume)
				if unitsF > math.MaxInt32 {
					unitsF = math.MaxInt32
				}
				units = int32(unitsF)
				if units <= 0 {
					continue
				}
			}
			askBook := idx.sellOrdersByLocation[routeBookKey{
				systemID:   source.systemID,
				typeID:     typeID,
				locationID: sell.LocationID,
			}]
			if len(askBook) == 0 {
				continue
			}

			// Find executable buy orders for this type across all destination systems.
			for buySystemID, buysByType := range idx.highestBuy {
				if len(ignoredSystems) > 0 && ignoredSystems[buySystemID] {
					continue
				}
				if buySystemID == source.systemID {
					continue
				}
				buyEntries, ok := buysByType[typeID]
				if !ok {
					continue
				}

				seenDestLocations := make(map[int64]bool, len(buyEntries))
				for _, buy := range buyEntries {
					if seenDestLocations[buy.LocationID] {
						continue
					}
					seenDestLocations[buy.LocationID] = true

					bidBook := idx.buyOrdersByLocation[routeBookKey{
						systemID:   buySystemID,
						typeID:     typeID,
						locationID: buy.LocationID,
					}]
					if len(bidBook) == 0 {
						continue
					}

					safeQty, planBuy, planSell, expectedProfit := findSafeExecutionQuantity(
						askBook,
						bidBook,
						units,
						buyCostMult,
						sellRevenueMult,
					)
					if safeQty <= 0 || expectedProfit <= 0 {
						continue
					}
					// ESI buy orders may require a minimum fill size.
					if buy.MinVolume > 0 && safeQty < buy.MinVolume {
						continue
					}

					effectiveBuy := planBuy.ExpectedPrice * buyCostMult
					if effectiveBuy <= 0 {
						continue
					}
					profitPerUnit := expectedProfit / float64(safeQty)
					margin := profitPerUnit / effectiveBuy * 100
					if margin < params.MinMargin {
						continue
					}

					tradeJumps := s.jumpsBetweenWithSecurity(source.systemID, buySystemID, params.MinRouteSecurity)
					if tradeJumps <= 0 || tradeJumps > MaxTradeJumps {
						continue
					}
					totalHopJumps := tradeJumps + source.emptyJumps
					if totalHopJumps <= 0 {
						continue
					}
					if params.MinISKPerJump > 0 && (expectedProfit/float64(totalHopJumps)) < params.MinISKPerJump {
						continue
					}

					regionID := int32(0)
					if sys, ok := s.SDE.Systems[source.systemID]; ok {
						regionID = sys.RegionID
					}
					candidates = append(candidates, candidate{
						hop: RouteHop{
							SystemName:     s.systemName(source.systemID),
							SystemID:       source.systemID,
							RegionID:       regionID,
							LocationID:     sell.LocationID,
							EmptyJumps:     source.emptyJumps,
							DestSystemID:   buySystemID,
							DestSystemName: s.systemName(buySystemID),
							DestLocationID: buy.LocationID,
							TypeName:       itemType.Name,
							TypeID:         typeID,
							BuyPrice:       sanitizeFloat(planBuy.ExpectedPrice),
							SellPrice:      sanitizeFloat(planSell.ExpectedPrice),
							Units:          safeQty,
							Profit:         sanitizeFloat(expectedProfit),
							Jumps:          tradeJumps,
							VolumeM3:       sanitizeFloat(itemType.Volume),
						},
						score: routeSearchScore(expectedProfit, totalHopJumps, params.RouteMode),
					})
				}
			}
		}
	}

	// Sort by route-mode search score, tie-break by ISK/jump.
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score == candidates[j].score {
			leftJumps := candidates[i].hop.Jumps + candidates[i].hop.EmptyJumps
			rightJumps := candidates[j].hop.Jumps + candidates[j].hop.EmptyJumps
			leftPPJ := candidates[i].hop.Profit / float64(max(1, leftJumps))
			rightPPJ := candidates[j].hop.Profit / float64(max(1, rightJumps))
			return leftPPJ > rightPPJ
		}
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
	startName := strings.TrimSpace(params.SystemName)
	systemID, ok := s.SDE.SystemByName[strings.ToLower(startName)]
	if !ok {
		return nil, fmt.Errorf("system not found: %s", params.SystemName)
	}
	ignoredSystems := ignoredSystemSetFromIDs(params.IgnoredSystemIDs)
	if len(ignoredSystems) > 0 && ignoredSystems[systemID] {
		return nil, fmt.Errorf("start system is in ignored systems list: %s", startName)
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
	reachableSystems = filterSystemDistanceMap(reachableSystems, ignoredSystems)
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

	targetSystemID := int32(0)
	targetSystemName := ""
	if strings.TrimSpace(params.TargetSystemName) != "" {
		targetID, found := s.SDE.SystemByName[strings.ToLower(strings.TrimSpace(params.TargetSystemName))]
		if !found {
			return nil, fmt.Errorf("target system not found: %s", params.TargetSystemName)
		}
		if len(ignoredSystems) > 0 && ignoredSystems[targetID] {
			return nil, fmt.Errorf("target system is in ignored systems list: %s", params.TargetSystemName)
		}
		targetSystemID = targetID
		targetSystemName = s.systemName(targetID)
	}
	targetDistanceBySystem := make(map[int32]int)
	distanceToTarget := func(systemID int32) int {
		if targetSystemID == 0 {
			return 0
		}
		if dist, ok := targetDistanceBySystem[systemID]; ok {
			return dist
		}
		dist := s.jumpsBetweenWithSecurity(systemID, targetSystemID, params.MinRouteSecurity)
		targetDistanceBySystem[systemID] = dist
		return dist
	}
	hopAdvancesTowardTarget := func(fromSystemID, destSystemID int32) bool {
		if targetSystemID == 0 {
			return false
		}
		fromDist := distanceToTarget(fromSystemID)
		destDist := distanceToTarget(destSystemID)
		if fromDist == UnreachableJumps || destDist == UnreachableJumps {
			return false
		}
		return destDist < fromDist
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
	idx := buildOrderIndexWithFilters(sellOrders, buyOrders, params.IncludeStructures)
	log.Printf(
		"[Route] Search params: start=%s target=%s mode=%s hops=%d-%d minMargin=%.2f minISK/jump=%.2f allowEmpty=%t",
		startName,
		targetSystemName,
		NormalizeRouteMode(params.RouteMode),
		params.MinHops,
		params.MaxHops,
		params.MinMargin,
		params.MinISKPerJump,
		params.AllowEmptyHops,
	)

	// Scale beam search parameters based on requested depth
	beamWidth := 50    // keep top N partial routes per level
	branchFactor := 10 // explore top N trades per system
	cachedTradesPerSource := branchFactor * 4

	type partialRoute struct {
		hops        []RouteHop
		totalProfit float64
		totalJumps  int
		lastSystem  int32
	}

	sourceCandidatesBySystem := make(map[int32][]sourceSystemCandidate)
	tradesBySourceSystem := make(map[int32][]RouteHop)

	baseTradeParams := params
	baseTradeParams.AllowEmptyHops = false
	baseTradeParams.MinISKPerJump = 0

	getSourceCandidates := func(fromSystemID int32) []sourceSystemCandidate {
		if !params.AllowEmptyHops {
			return []sourceSystemCandidate{{systemID: fromSystemID, emptyJumps: 0}}
		}
		if cached, ok := sourceCandidatesBySystem[fromSystemID]; ok {
			return cached
		}
		candidates := s.nearestTradeSourceSystems(idx, fromSystemID, params)
		sourceCandidatesBySystem[fromSystemID] = candidates
		return candidates
	}

	getTradesFromSource := func(sourceSystemID int32, topN int) []RouteHop {
		if topN <= 0 {
			return nil
		}
		if cached, ok := tradesBySourceSystem[sourceSystemID]; ok {
			if len(cached) > topN {
				return cached[:topN]
			}
			return cached
		}
		cached := s.findBestTradesFromSources(
			idx,
			[]sourceSystemCandidate{{systemID: sourceSystemID, emptyJumps: 0}},
			baseTradeParams,
			cachedTradesPerSource,
		)
		tradesBySourceSystem[sourceSystemID] = cached
		if len(cached) > topN {
			return cached[:topN]
		}
		return cached
	}

	selectBestHopCandidates := func(fromSystemID int32, topN int) []RouteHop {
		type hopCandidate struct {
			hop RouteHop
		}
		raw := make([]hopCandidate, 0, topN*2)
		for _, source := range getSourceCandidates(fromSystemID) {
			perSourceTopN := topN
			if source.emptyJumps > 0 {
				perSourceTopN = max(2, topN/2)
			}
			for _, trade := range getTradesFromSource(source.systemID, perSourceTopN) {
				hop := trade
				hop.EmptyJumps = source.emptyJumps
				totalHopJumps := hop.Jumps + hop.EmptyJumps
				allowBelowThreshold := hopAdvancesTowardTarget(fromSystemID, hop.DestSystemID)
				if !routeMinISKPerJumpPass(
					params.MinISKPerJump,
					hop.Profit,
					totalHopJumps,
					allowBelowThreshold,
				) {
					continue
				}
				raw = append(raw, hopCandidate{hop: hop})
			}
		}
		sort.Slice(raw, func(i, j int) bool {
			leftJumps := max(1, raw[i].hop.Jumps+raw[i].hop.EmptyJumps)
			rightJumps := max(1, raw[j].hop.Jumps+raw[j].hop.EmptyJumps)
			leftScore := routeSearchScore(raw[i].hop.Profit, leftJumps, params.RouteMode)
			rightScore := routeSearchScore(raw[j].hop.Profit, rightJumps, params.RouteMode)
			if !floatNearlyEqual(leftScore, rightScore) {
				return leftScore > rightScore
			}
			return raw[i].hop.Profit > raw[j].hop.Profit
		})
		if len(raw) > topN {
			raw = raw[:topN]
		}

		out := make([]RouteHop, len(raw))
		for i, c := range raw {
			out[i] = c.hop
		}
		return out
	}

	finalizeRoute := func(pr partialRoute) (RouteResult, bool) {
		totalJumps := pr.totalJumps
		targetJumps := 0
		if targetSystemID != 0 {
			targetJumps = s.jumpsBetweenWithSecurity(pr.lastSystem, targetSystemID, params.MinRouteSecurity)
			if targetJumps == UnreachableJumps {
				return RouteResult{}, false
			}
			totalJumps += targetJumps
		}
		if totalJumps <= 0 {
			return RouteResult{}, false
		}

		profitPerJump := sanitizeFloat(pr.totalProfit / float64(totalJumps))
		filterJumps := routeFilterJumpCountForTarget(pr.totalJumps, targetJumps, targetSystemID != 0)
		if params.MinISKPerJump > 0 {
			filterProfitPerJump := sanitizeFloat(pr.totalProfit / float64(filterJumps))
			if filterProfitPerJump < params.MinISKPerJump {
				return RouteResult{}, false
			}
		}

		return RouteResult{
			Hops:             copyHops(pr.hops),
			TotalProfit:      pr.totalProfit,
			TotalJumps:       totalJumps,
			ProfitPerJump:    profitPerJump,
			HopCount:         len(pr.hops),
			TargetSystemName: targetSystemName,
			TargetJumps:      targetJumps,
		}, true
	}

	// Seed with initial trades from start system
	progress("Exploring routes...")
	initialTrades := selectBestHopCandidates(systemID, branchFactor*3) // wider initial search
	if len(initialTrades) == 0 {
		progress("No profitable trades found")
		return []RouteResult{}, nil
	}

	// Initialize beam with hop-1 routes
	beam := make([]partialRoute, 0, len(initialTrades))
	for _, hop := range initialTrades {
		hopTravelJumps := hop.Jumps + hop.EmptyJumps
		beam = append(beam, partialRoute{
			hops:        []RouteHop{hop},
			totalProfit: hop.Profit,
			totalJumps:  hopTravelJumps,
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
				if result, ok := finalizeRoute(pr); ok {
					completedRoutes = append(completedRoutes, result)
				}
			}
		}

		var nextBeam []partialRoute
		for _, pr := range beam {
			trades := selectBestHopCandidates(pr.lastSystem, branchFactor)
			for _, hop := range trades {
				// Avoid revisiting source/destination systems (except current source system).
				if hop.SystemID != pr.lastSystem && routeVisitsSystem(pr.hops, hop.SystemID) {
					continue
				}
				if routeVisitsSystem(pr.hops, hop.DestSystemID) {
					continue
				}

				newHops := make([]RouteHop, len(pr.hops)+1)
				copy(newHops, pr.hops)
				newHops[len(pr.hops)] = hop
				hopTravelJumps := hop.Jumps + hop.EmptyJumps

				nextBeam = append(nextBeam, partialRoute{
					hops:        newHops,
					totalProfit: pr.totalProfit + hop.Profit,
					totalJumps:  pr.totalJumps + hopTravelJumps,
					lastSystem:  hop.DestSystemID,
				})
				explored++
			}
		}

		// Prune to beamWidth
		sort.Slice(nextBeam, func(i, j int) bool {
			leftScore := routeSearchScore(nextBeam[i].totalProfit, nextBeam[i].totalJumps, params.RouteMode)
			rightScore := routeSearchScore(nextBeam[j].totalProfit, nextBeam[j].totalJumps, params.RouteMode)
			if !floatNearlyEqual(leftScore, rightScore) {
				return leftScore > rightScore
			}
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
			if result, ok := finalizeRoute(pr); ok {
				completedRoutes = append(completedRoutes, result)
			}
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

	s.enrichRoutesWithLiquidity(completedRoutes, progress)
	EnrichRouteExecutionEstimatesWithProfile(completedRoutes, RouteExecutionProfileFromParams(params))
	SortRouteResultsByMode(completedRoutes, params.RouteMode)

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

func routeVisitsSystem(hops []RouteHop, systemID int32) bool {
	for _, h := range hops {
		if h.SystemID == systemID || h.DestSystemID == systemID {
			return true
		}
	}
	return false
}

// routeKey generates a unique string key for a route based on the sequence of systems and items.
func routeKey(r RouteResult) string {
	parts := make([]string, len(r.Hops))
	for i, h := range r.Hops {
		parts[i] = fmt.Sprintf("%d>%d:%d", h.SystemID, h.DestSystemID, h.TypeID)
	}
	return strings.Join(parts, "|")
}
