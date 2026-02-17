package engine

import (
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
	entry := byType[200]
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
	if idx.highestBuy[10][1].Price != 105 {
		t.Errorf("highestBuy[10][1] = %v, want 105", idx.highestBuy[10][1].Price)
	}
	if idx.highestBuy[20][1].Price != 108 {
		t.Errorf("highestBuy[20][1] = %v, want 108", idx.highestBuy[20][1].Price)
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
