package engine

import (
	"testing"

	"eve-flipper/internal/esi"
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
