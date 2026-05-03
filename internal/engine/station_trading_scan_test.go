package engine

import (
	"fmt"
	"testing"

	"eve-flipper/internal/esi"
	"eve-flipper/internal/sde"
)

func TestScanStationTrades_UsesFullRegionDepthWhenStationFiltered(t *testing.T) {
	const (
		regionID       = int32(10000002)
		typeID         = int32(34)
		targetStation  = int64(60003760)
		otherStation   = int64(60008494)
		targetSystemID = int32(30000142)
		otherSystemID  = int32(30002510)
	)

	origFetchOrders := stationFetchRegionOrders
	origPrefetchNPC := stationPrefetchNPCNames
	origPrefetchStr := stationPrefetchStructureNames
	origResolveName := stationResolveName
	origFetchHistory := stationFetchMarketHistory
	defer func() {
		stationFetchRegionOrders = origFetchOrders
		stationPrefetchNPCNames = origPrefetchNPC
		stationPrefetchStructureNames = origPrefetchStr
		stationResolveName = origResolveName
		stationFetchMarketHistory = origFetchHistory
	}()

	orders := []esi.MarketOrder{
		// Target station: profitable spread, depth = 100.
		{TypeID: typeID, LocationID: targetStation, SystemID: targetSystemID, Price: 90, VolumeRemain: 50, IsBuyOrder: true},
		{TypeID: typeID, LocationID: targetStation, SystemID: targetSystemID, Price: 100, VolumeRemain: 50, IsBuyOrder: false},
		// Another station in same region/type: extra depth = 900.
		{TypeID: typeID, LocationID: otherStation, SystemID: otherSystemID, Price: 89, VolumeRemain: 450, IsBuyOrder: true},
		{TypeID: typeID, LocationID: otherStation, SystemID: otherSystemID, Price: 101, VolumeRemain: 450, IsBuyOrder: false},
	}

	stationFetchRegionOrders = func(_ *esi.Client, rid int32, orderType string) ([]esi.MarketOrder, error) {
		if rid != regionID || orderType != "all" {
			return nil, fmt.Errorf("unexpected region/orderType: %d/%s", rid, orderType)
		}
		return orders, nil
	}
	stationPrefetchNPCNames = func(_ *esi.Client, _ map[int64]bool) {}
	stationPrefetchStructureNames = func(_ *esi.Client, _ map[int64]bool, _ string) {}
	stationResolveName = func(_ *esi.Client, stationID int64) string {
		if stationID == targetStation {
			return "Jita IV - Moon 4 - Caldari Navy Assembly Plant"
		}
		return "Other Station"
	}
	stationFetchMarketHistory = func(_ *esi.Client, rid int32, tid int32) ([]esi.HistoryEntry, error) {
		if rid != regionID || tid != typeID {
			return nil, fmt.Errorf("unexpected history key: %d/%d", rid, tid)
		}
		return testHistoryFixedDailyVolume(100), nil // regionFlowPerDay = 100
	}

	scanner := &Scanner{
		SDE: &sde.Data{
			Types: map[int32]*sde.ItemType{
				typeID: {ID: typeID, Name: "Tritanium", Volume: 0.01},
			},
		},
		History: &testHistoryProvider{
			store: map[string][]esi.HistoryEntry{
				fmt.Sprintf("%d:%d", regionID, typeID): testHistoryFixedDailyVolume(100),
			},
		},
	}

	results, err := scanner.ScanStationTrades(StationTradeParams{
		StationIDs: map[int64]bool{
			targetStation: true,
		},
		RegionID:  regionID,
		MinMargin: 0.1,
	}, func(string) {})
	if err != nil {
		t.Fatalf("ScanStationTrades returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}

	row := results[0]
	if row.StationID != targetStation {
		t.Fatalf("StationID = %d, want %d", row.StationID, targetStation)
	}
	// Full region depth is 1000, target depth is 100 => share 0.1.
	// regionFlowPerDay=100 => station flow = 10/day.
	if row.DailyVolume != 10 {
		t.Fatalf("DailyVolume = %d, want 10", row.DailyVolume)
	}
	if row.RegionID != regionID || row.SystemID != targetSystemID {
		t.Fatalf("RegionID/SystemID = %d/%d, want %d/%d", row.RegionID, row.SystemID, regionID, targetSystemID)
	}
}
