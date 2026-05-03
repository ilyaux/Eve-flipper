package engine

import (
	"math"
	"testing"

	"eve-flipper/internal/esi"
	"eve-flipper/internal/graph"
	"eve-flipper/internal/sde"
)

// buildOrderIndex: cheapest sell per (system, type), highest buy per (system, type).

func TestBuildOrderIndex_CheapestSellWins(t *testing.T) {
	sellOrders := []esi.MarketOrder{
		{SystemID: 1, TypeID: 100, Price: 200, VolumeRemain: 50, LocationID: 1001},
		{SystemID: 1, TypeID: 100, Price: 150, VolumeRemain: 30, LocationID: 1002},
		{SystemID: 1, TypeID: 100, Price: 180, VolumeRemain: 20, LocationID: 1003},
	}
	idx := buildOrderIndex(sellOrders, nil)
	byType := idx.cheapestSell[1]
	if byType == nil {
		t.Fatal("cheapestSell[1] is nil")
	}
	entry := byType[100]
	if entry.Price != 150 {
		t.Errorf("cheapest sell for (sys=1, type=100) = %v, want 150", entry.Price)
	}
	if entry.VolumeRemain != 30 {
		t.Errorf("VolumeRemain = %v, want 30", entry.VolumeRemain)
	}
	if entry.LocationID != 1002 {
		t.Errorf("LocationID = %v, want 1002", entry.LocationID)
	}
}

func TestBuildOrderIndex_HighestBuyWins(t *testing.T) {
	buyOrders := []esi.MarketOrder{
		{SystemID: 2, TypeID: 200, Price: 90, VolumeRemain: 100, LocationID: 2001},
		{SystemID: 2, TypeID: 200, Price: 95, VolumeRemain: 80, LocationID: 2002},
		{SystemID: 2, TypeID: 200, Price: 92, VolumeRemain: 60, LocationID: 2003},
	}
	idx := buildOrderIndex(nil, buyOrders)
	byType := idx.highestBuy[2]
	if byType == nil {
		t.Fatal("highestBuy[2] is nil")
	}
	entries := byType[200]
	if len(entries) == 0 {
		t.Fatal("highestBuy[2][200] is empty")
	}
	entry := entries[0]
	if entry.Price != 95 {
		t.Errorf("highest buy for (sys=2, type=200) = %v, want 95", entry.Price)
	}
	if entry.LocationID != 2002 {
		t.Errorf("LocationID = %v, want 2002", entry.LocationID)
	}
}

func TestBuildOrderIndex_MultipleSystemsAndTypes(t *testing.T) {
	sellOrders := []esi.MarketOrder{
		{SystemID: 10, TypeID: 1, Price: 100, VolumeRemain: 1, LocationID: 1},
		{SystemID: 10, TypeID: 2, Price: 200, VolumeRemain: 1, LocationID: 1},
		{SystemID: 20, TypeID: 1, Price: 99, VolumeRemain: 1, LocationID: 2},
	}
	buyOrders := []esi.MarketOrder{
		{SystemID: 10, TypeID: 1, Price: 105, VolumeRemain: 1, LocationID: 1},
		{SystemID: 20, TypeID: 1, Price: 108, VolumeRemain: 1, LocationID: 2},
	}
	idx := buildOrderIndex(sellOrders, buyOrders)

	// Cheapest sell: sys 10 type 1 = 100, sys 10 type 2 = 200, sys 20 type 1 = 99
	if idx.cheapestSell[10][1].Price != 100 {
		t.Errorf("cheapestSell[10][1] = %v, want 100", idx.cheapestSell[10][1].Price)
	}
	if idx.cheapestSell[10][2].Price != 200 {
		t.Errorf("cheapestSell[10][2] = %v, want 200", idx.cheapestSell[10][2].Price)
	}
	if idx.cheapestSell[20][1].Price != 99 {
		t.Errorf("cheapestSell[20][1] = %v, want 99", idx.cheapestSell[20][1].Price)
	}

	// Highest buy: sys 10 type 1 = 105, sys 20 type 1 = 108
	if idx.highestBuy[10][1][0].Price != 105 {
		t.Errorf("highestBuy[10][1] = %v, want 105", idx.highestBuy[10][1][0].Price)
	}
	if idx.highestBuy[20][1][0].Price != 108 {
		t.Errorf("highestBuy[20][1] = %v, want 108", idx.highestBuy[20][1][0].Price)
	}
}

func TestBuildOrderIndex_Empty(t *testing.T) {
	idx := buildOrderIndex(nil, nil)
	if idx == nil {
		t.Fatal("buildOrderIndex(nil,nil) returned nil")
	}
	if len(idx.cheapestSell) != 0 {
		t.Errorf("cheapestSell should be empty, got %d systems", len(idx.cheapestSell))
	}
	if len(idx.highestBuy) != 0 {
		t.Errorf("highestBuy should be empty, got %d systems", len(idx.highestBuy))
	}
}

func TestBuildOrderIndexWithFilters_ExcludeStructures(t *testing.T) {
	sellOrders := []esi.MarketOrder{
		{SystemID: 1, TypeID: 100, Price: 10, VolumeRemain: 50, LocationID: 1_000_000_000_123}, // structure
		{SystemID: 1, TypeID: 100, Price: 15, VolumeRemain: 50, LocationID: 60003760},          // NPC station
	}
	buyOrders := []esi.MarketOrder{
		{SystemID: 2, TypeID: 100, Price: 30, VolumeRemain: 50, LocationID: 1_000_000_000_456}, // structure
		{SystemID: 2, TypeID: 100, Price: 25, VolumeRemain: 50, LocationID: 60008494},          // NPC station
	}

	idx := buildOrderIndexWithFilters(sellOrders, buyOrders, false)
	if got := idx.cheapestSell[1][100].LocationID; got != 60003760 {
		t.Fatalf("cheapestSell location = %d, want NPC station 60003760", got)
	}
	if got := idx.highestBuy[2][100][0].LocationID; got != 60008494 {
		t.Fatalf("highestBuy location = %d, want NPC station 60008494", got)
	}
}

func TestBuildOrderIndexWithFilters_IncludeStructures(t *testing.T) {
	sellOrders := []esi.MarketOrder{
		{SystemID: 1, TypeID: 100, Price: 10, VolumeRemain: 50, LocationID: 1_000_000_000_123}, // structure, best price
		{SystemID: 1, TypeID: 100, Price: 15, VolumeRemain: 50, LocationID: 60003760},          // NPC station
	}
	buyOrders := []esi.MarketOrder{
		{SystemID: 2, TypeID: 100, Price: 30, VolumeRemain: 50, LocationID: 1_000_000_000_456}, // structure, best price
		{SystemID: 2, TypeID: 100, Price: 25, VolumeRemain: 50, LocationID: 60008494},          // NPC station
	}

	idx := buildOrderIndexWithFilters(sellOrders, buyOrders, true)
	if got := idx.cheapestSell[1][100].LocationID; got != 1_000_000_000_123 {
		t.Fatalf("cheapestSell location = %d, want structure 1000000000123", got)
	}
	if got := idx.highestBuy[2][100][0].LocationID; got != 1_000_000_000_456 {
		t.Fatalf("highestBuy location = %d, want structure 1000000000456", got)
	}
}

func TestFindBestTrades_RespectsBuyMinVolume(t *testing.T) {
	u := graph.NewUniverse()
	u.AddGate(1, 2)
	u.AddGate(2, 1)
	u.SetRegion(1, 100)
	u.SetRegion(2, 200)
	u.SetSecurity(1, 1.0)
	u.SetSecurity(2, 1.0)

	s := &Scanner{
		SDE: &sde.Data{
			Universe: u,
			Types: map[int32]*sde.ItemType{
				34: {ID: 34, Name: "Tritanium", Volume: 1},
			},
			Systems: map[int32]*sde.SolarSystem{
				1: {ID: 1, Name: "Alpha", RegionID: 100, Security: 1.0},
				2: {ID: 2, Name: "Beta", RegionID: 200, Security: 1.0},
			},
		},
	}

	sellOrders := []esi.MarketOrder{
		{SystemID: 1, TypeID: 34, Price: 10, VolumeRemain: 100, LocationID: 1001},
	}
	buyOrders := []esi.MarketOrder{
		{SystemID: 2, TypeID: 34, Price: 15, VolumeRemain: 100, MinVolume: 60, LocationID: 2001},
		{SystemID: 2, TypeID: 34, Price: 14, VolumeRemain: 100, MinVolume: 1, LocationID: 2002},
	}
	idx := buildOrderIndex(sellOrders, buyOrders)

	params := RouteParams{
		CargoCapacity:    50, // below top order min_volume=60
		MinMargin:        0,
		SalesTaxPercent:  0,
		BrokerFeePercent: 0,
	}

	hops := s.findBestTrades(idx, 1, params, 10)
	if len(hops) == 0 {
		t.Fatalf("expected fallback to a lower-price executable buy order")
	}
	if hops[0].SellPrice != 14 {
		t.Fatalf("SellPrice = %f, want 14 (executable min_volume)", hops[0].SellPrice)
	}
}

func TestSelectClosestRouteRegions(t *testing.T) {
	systemRegion := map[int32]int32{
		1: 10, // dist 0
		2: 20, // dist 5
		3: 30, // dist 2
		4: 20, // dist 3 (improves region 20 min dist to 3)
		5: 40, // dist 9
	}
	systems := map[int32]int{
		1: 0,
		2: 5,
		3: 2,
		4: 3,
		5: 9,
	}

	got := selectClosestRouteRegions(systemRegion, systems, 2)

	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if !got[10] {
		t.Fatalf("expected region 10 (closest) to be selected")
	}
	if !got[30] {
		t.Fatalf("expected region 30 (second closest) to be selected")
	}
	if got[20] || got[40] {
		t.Fatalf("unexpected farther regions selected: %+v", got)
	}
}

func TestSelectClosestRouteRegions_TieBreakByRegionID(t *testing.T) {
	systemRegion := map[int32]int32{
		1: 20,
		2: 10,
	}
	systems := map[int32]int{
		1: 3,
		2: 3,
	}

	got := selectClosestRouteRegions(systemRegion, systems, 1)
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if !got[10] {
		t.Fatalf("expected lower regionID 10 to win tie-break, got %+v", got)
	}
}

func TestFindBestTrades_MathInvariants(t *testing.T) {
	u := graph.NewUniverse()
	u.AddGate(1, 2)
	u.AddGate(2, 1)
	u.SetRegion(1, 100)
	u.SetRegion(2, 200)
	u.SetSecurity(1, 1.0)
	u.SetSecurity(2, 1.0)

	s := &Scanner{
		SDE: &sde.Data{
			Universe: u,
			Types: map[int32]*sde.ItemType{
				34: {ID: 34, Name: "Tritanium", Volume: 1},
				35: {ID: 35, Name: "Pyerite", Volume: 1},
			},
			Systems: map[int32]*sde.SolarSystem{
				1: {ID: 1, Name: "Alpha", RegionID: 100, Security: 1.0},
				2: {ID: 2, Name: "Beta", RegionID: 200, Security: 1.0},
			},
		},
	}

	sellOrders := []esi.MarketOrder{
		{SystemID: 1, TypeID: 34, Price: 10, VolumeRemain: 100, LocationID: 1001},
		{SystemID: 1, TypeID: 35, Price: 10, VolumeRemain: 100, LocationID: 1002},
	}
	buyOrders := []esi.MarketOrder{
		{SystemID: 2, TypeID: 34, Price: 15, VolumeRemain: 40, LocationID: 2001},
		{SystemID: 2, TypeID: 35, Price: 9, VolumeRemain: 100, LocationID: 2002}, // not profitable
	}
	idx := buildOrderIndex(sellOrders, buyOrders)

	params := RouteParams{
		CargoCapacity:    30, // 30 units for volume=1
		MinMargin:        0,
		SalesTaxPercent:  0,
		BrokerFeePercent: 0,
	}

	hops := s.findBestTrades(idx, 1, params, 10)
	if len(hops) != 1 {
		t.Fatalf("len(hops) = %d, want 1", len(hops))
	}
	h := hops[0]
	if h.Profit <= 0 {
		t.Fatalf("hop profit = %f, want > 0", h.Profit)
	}
	if h.Jumps <= 0 || h.Jumps > MaxTradeJumps {
		t.Fatalf("hop jumps = %d, want in [1,%d]", h.Jumps, MaxTradeJumps)
	}
	if h.Units != 30 {
		t.Fatalf("hop units = %d, want 30", h.Units)
	}
	if h.Profit != 150 {
		t.Fatalf("hop profit = %f, want 150", h.Profit)
	}
}

func TestFindBestTrades_UsesDepthAwareProfit(t *testing.T) {
	u := graph.NewUniverse()
	u.AddGate(1, 2)
	u.AddGate(2, 1)
	u.SetRegion(1, 100)
	u.SetRegion(2, 200)
	u.SetSecurity(1, 1.0)
	u.SetSecurity(2, 1.0)

	s := &Scanner{
		SDE: &sde.Data{
			Universe: u,
			Types: map[int32]*sde.ItemType{
				34: {ID: 34, Name: "Tritanium", Volume: 1},
			},
			Systems: map[int32]*sde.SolarSystem{
				1: {ID: 1, Name: "Alpha", RegionID: 100, Security: 1.0},
				2: {ID: 2, Name: "Beta", RegionID: 200, Security: 1.0},
			},
		},
	}

	sellOrders := []esi.MarketOrder{
		{SystemID: 1, TypeID: 34, Price: 10, VolumeRemain: 1, LocationID: 1001},
		{SystemID: 1, TypeID: 34, Price: 100, VolumeRemain: 99, LocationID: 1001},
	}
	buyOrders := []esi.MarketOrder{
		{SystemID: 2, TypeID: 34, Price: 110, VolumeRemain: 100, LocationID: 2001},
	}
	idx := buildOrderIndex(sellOrders, buyOrders)

	hops := s.findBestTrades(idx, 1, RouteParams{
		CargoCapacity:    100,
		MinMargin:        0,
		SalesTaxPercent:  0,
		BrokerFeePercent: 0,
	}, 10)
	if len(hops) != 1 {
		t.Fatalf("len(hops) = %d, want 1", len(hops))
	}

	h := hops[0]
	if h.Units != 100 {
		t.Fatalf("Units = %d, want 100", h.Units)
	}
	const wantProfit = 1090.0
	if math.Abs(h.Profit-wantProfit) > 1e-9 {
		t.Fatalf("Profit = %f, want depth-aware %f", h.Profit, wantProfit)
	}
	if math.Abs(h.BuyPrice-99.1) > 1e-9 {
		t.Fatalf("BuyPrice = %f, want VWAP 99.1", h.BuyPrice)
	}
	if h.Profit >= 10_000 {
		t.Fatalf("Profit still uses top-book fantasy value: %f", h.Profit)
	}
}

func TestFindBestTrades_RanksByTotalProfit(t *testing.T) {
	u := graph.NewUniverse()
	// 1->2 (1 jump), 1->3 (2 jumps)
	u.AddGate(1, 2)
	u.AddGate(2, 1)
	u.AddGate(1, 4)
	u.AddGate(4, 1)
	u.AddGate(4, 3)
	u.AddGate(3, 4)
	u.SetRegion(1, 100)
	u.SetRegion(2, 200)
	u.SetRegion(3, 300)
	u.SetRegion(4, 400)
	u.SetSecurity(1, 1.0)
	u.SetSecurity(2, 1.0)
	u.SetSecurity(3, 1.0)
	u.SetSecurity(4, 1.0)

	s := &Scanner{
		SDE: &sde.Data{
			Universe: u,
			Types: map[int32]*sde.ItemType{
				34: {ID: 34, Name: "Tritanium", Volume: 1},
			},
			Systems: map[int32]*sde.SolarSystem{
				1: {ID: 1, Name: "Alpha", RegionID: 100, Security: 1.0},
				2: {ID: 2, Name: "Beta", RegionID: 200, Security: 1.0},
				3: {ID: 3, Name: "Gamma", RegionID: 300, Security: 1.0},
				4: {ID: 4, Name: "Delta", RegionID: 400, Security: 1.0},
			},
		},
	}

	// Buy source at system 1:
	sellOrders := []esi.MarketOrder{
		{SystemID: 1, TypeID: 34, Price: 10, VolumeRemain: 100, LocationID: 1001},
	}
	// Two destinations:
	// system 2: profit/unit=3, jumps=1 => total=300, ppj=300
	// system 3: profit/unit=5, jumps=2 => total=500, ppj=250
	// Ranking must prefer higher total profit (system 3), not higher ppj.
	buyOrders := []esi.MarketOrder{
		{SystemID: 2, TypeID: 34, Price: 13, VolumeRemain: 100, LocationID: 2001},
		{SystemID: 3, TypeID: 34, Price: 15, VolumeRemain: 100, LocationID: 3001},
	}
	idx := buildOrderIndex(sellOrders, buyOrders)

	params := RouteParams{
		CargoCapacity:    100,
		MinMargin:        0,
		SalesTaxPercent:  0,
		BrokerFeePercent: 0,
	}

	hops := s.findBestTrades(idx, 1, params, 1)
	if len(hops) != 1 {
		t.Fatalf("len(hops) = %d, want 1", len(hops))
	}
	if hops[0].DestSystemID != 3 {
		t.Fatalf("top hop should maximize total profit (dest 3), got dest %d", hops[0].DestSystemID)
	}
	if hops[0].Profit != 500 {
		t.Fatalf("top hop profit = %f, want 500", hops[0].Profit)
	}
}

func TestFindBestTrades_AllowEmptyHops(t *testing.T) {
	u := graph.NewUniverse()
	u.AddGate(1, 2)
	u.AddGate(2, 1)
	u.AddGate(2, 3)
	u.AddGate(3, 2)
	u.SetRegion(1, 100)
	u.SetRegion(2, 100)
	u.SetRegion(3, 100)
	u.SetSecurity(1, 1.0)
	u.SetSecurity(2, 1.0)
	u.SetSecurity(3, 1.0)

	s := &Scanner{
		SDE: &sde.Data{
			Universe: u,
			Types: map[int32]*sde.ItemType{
				34: {ID: 34, Name: "Tritanium", Volume: 1},
			},
			Systems: map[int32]*sde.SolarSystem{
				1: {ID: 1, Name: "Alpha", RegionID: 100, Security: 1.0},
				2: {ID: 2, Name: "Beta", RegionID: 100, Security: 1.0},
				3: {ID: 3, Name: "Gamma", RegionID: 100, Security: 1.0},
			},
		},
	}

	sellOrders := []esi.MarketOrder{
		{SystemID: 2, TypeID: 34, Price: 10, VolumeRemain: 100, LocationID: 2001},
	}
	buyOrders := []esi.MarketOrder{
		{SystemID: 3, TypeID: 34, Price: 20, VolumeRemain: 100, LocationID: 3001},
	}
	idx := buildOrderIndex(sellOrders, buyOrders)

	params := RouteParams{
		CargoCapacity:    10,
		MinMargin:        0,
		SalesTaxPercent:  0,
		BrokerFeePercent: 0,
		AllowEmptyHops:   true,
		MinISKPerJump:    40, // Profit=100 over 2 jumps (1 empty + 1 trade) => 50 ISK/jump
	}

	hops := s.findBestTrades(idx, 1, params, 10)
	if len(hops) == 0 {
		t.Fatalf("expected trade via empty hop source system")
	}
	top := hops[0]
	if top.SystemID != 2 {
		t.Fatalf("source system = %d, want 2", top.SystemID)
	}
	if top.EmptyJumps != 1 {
		t.Fatalf("empty jumps = %d, want 1", top.EmptyJumps)
	}
	if top.Jumps != 1 {
		t.Fatalf("trade jumps = %d, want 1", top.Jumps)
	}
}

func TestFindBestTrades_MinISKPerJumpFiltersEmptyHopCandidate(t *testing.T) {
	u := graph.NewUniverse()
	u.AddGate(1, 2)
	u.AddGate(2, 1)
	u.AddGate(2, 3)
	u.AddGate(3, 2)
	u.SetRegion(1, 100)
	u.SetRegion(2, 100)
	u.SetRegion(3, 100)
	u.SetSecurity(1, 1.0)
	u.SetSecurity(2, 1.0)
	u.SetSecurity(3, 1.0)

	s := &Scanner{
		SDE: &sde.Data{
			Universe: u,
			Types: map[int32]*sde.ItemType{
				34: {ID: 34, Name: "Tritanium", Volume: 1},
			},
			Systems: map[int32]*sde.SolarSystem{
				1: {ID: 1, Name: "Alpha", RegionID: 100, Security: 1.0},
				2: {ID: 2, Name: "Beta", RegionID: 100, Security: 1.0},
				3: {ID: 3, Name: "Gamma", RegionID: 100, Security: 1.0},
			},
		},
	}

	sellOrders := []esi.MarketOrder{
		{SystemID: 2, TypeID: 34, Price: 10, VolumeRemain: 100, LocationID: 2001},
	}
	buyOrders := []esi.MarketOrder{
		{SystemID: 3, TypeID: 34, Price: 20, VolumeRemain: 100, LocationID: 3001},
	}
	idx := buildOrderIndex(sellOrders, buyOrders)

	params := RouteParams{
		CargoCapacity:    10,
		MinMargin:        0,
		SalesTaxPercent:  0,
		BrokerFeePercent: 0,
		AllowEmptyHops:   true,
		MinISKPerJump:    80, // Candidate is 50 ISK/jump and should be filtered.
	}

	hops := s.findBestTrades(idx, 1, params, 10)
	if len(hops) != 0 {
		t.Fatalf("expected no hops, got %d", len(hops))
	}
}

func TestRouteMinISKPerJumpPass(t *testing.T) {
	if !routeMinISKPerJumpPass(0, 10, 1, false) {
		t.Fatalf("expected pass when threshold disabled")
	}
	if routeMinISKPerJumpPass(100, 50, 1, false) {
		t.Fatalf("expected fail below threshold without target-progress bypass")
	}
	if !routeMinISKPerJumpPass(100, 50, 1, true) {
		t.Fatalf("expected pass below threshold when target-progress bypass is allowed")
	}
}

func TestRouteFilterJumpCountForTarget(t *testing.T) {
	if got := routeFilterJumpCountForTarget(7, 4, false); got != 11 {
		t.Fatalf("without target: got %d jumps, want 11", got)
	}
	if got := routeFilterJumpCountForTarget(7, 4, true); got != 7 {
		t.Fatalf("with target: got %d jumps, want 7 (exclude tail deadhead)", got)
	}
	if got := routeFilterJumpCountForTarget(0, 3, true); got != 1 {
		t.Fatalf("with target and zero trade jumps: got %d, want floor 1", got)
	}
}

func TestNearestTradeSourceSystems_RespectsIgnoredSystems(t *testing.T) {
	u := graph.NewUniverse()
	u.AddGate(1, 2)
	u.AddGate(2, 1)
	u.AddGate(2, 3)
	u.AddGate(3, 2)
	u.SetSecurity(1, 1.0)
	u.SetSecurity(2, 1.0)
	u.SetSecurity(3, 1.0)

	s := &Scanner{
		SDE: &sde.Data{
			Universe: u,
		},
	}

	idx := &orderIndex{
		cheapestSell: map[int32]map[int32]orderEntry{
			2: {34: {Price: 10, VolumeRemain: 100}},
			3: {34: {Price: 11, VolumeRemain: 100}},
		},
		highestBuy: map[int32]map[int32][]orderEntry{},
	}

	params := RouteParams{
		AllowEmptyHops:   true,
		MinRouteSecurity: 0,
		IgnoredSystemIDs: []int32{3},
	}

	sources := s.nearestTradeSourceSystems(idx, 1, params)
	if len(sources) != 2 {
		t.Fatalf("len(sources) = %d, want 2 (start + system 2)", len(sources))
	}
	if sources[0].systemID != 1 || sources[0].emptyJumps != 0 {
		t.Fatalf("first source = %+v, want start system with 0 empty jumps", sources[0])
	}
	if sources[1].systemID != 2 || sources[1].emptyJumps != 1 {
		t.Fatalf("second source = %+v, want system 2 with 1 empty jump", sources[1])
	}
}

func TestNearestTradeSourceSystems_StartIgnoredReturnsNil(t *testing.T) {
	u := graph.NewUniverse()
	u.SetSecurity(1, 1.0)

	s := &Scanner{
		SDE: &sde.Data{
			Universe: u,
		},
	}

	idx := &orderIndex{
		cheapestSell: map[int32]map[int32]orderEntry{
			1: {34: {Price: 10, VolumeRemain: 100}},
		},
		highestBuy: map[int32]map[int32][]orderEntry{},
	}

	params := RouteParams{
		AllowEmptyHops:   true,
		IgnoredSystemIDs: []int32{1},
	}

	if got := s.nearestTradeSourceSystems(idx, 1, params); got != nil {
		t.Fatalf("expected nil sources when start system is ignored, got %+v", got)
	}
}

func TestRouteHelpers_CopyVisitAndKey(t *testing.T) {
	orig := []RouteHop{
		{SystemID: 1, DestSystemID: 2, TypeID: 34},
		{SystemID: 2, DestSystemID: 3, TypeID: 35},
	}
	copied := copyHops(orig)

	if len(copied) != len(orig) {
		t.Fatalf("len(copied) = %d, want %d", len(copied), len(orig))
	}
	copied[0].SystemID = 999
	if orig[0].SystemID != 1 {
		t.Fatalf("copyHops must return independent slice copy")
	}

	if !routeVisitsSystem(orig, 1) {
		t.Fatalf("expected route to visit system 1")
	}
	if !routeVisitsSystem(orig, 3) {
		t.Fatalf("expected route to visit system 3")
	}
	if routeVisitsSystem(orig, 42) {
		t.Fatalf("did not expect route to visit system 42")
	}

	key := routeKey(RouteResult{Hops: orig})
	if key != "1>2:34|2>3:35" {
		t.Fatalf("routeKey = %q, want %q", key, "1>2:34|2>3:35")
	}
}
