package engine

import (
	"fmt"
	"testing"
	"time"

	"eve-flipper/internal/esi"
)

func testHistoryFixedDailyVolume(volume int64) []esi.HistoryEntry {
	entries := make([]esi.HistoryEntry, 0, stationFlowWindowDays)
	for d := stationFlowWindowDays - 1; d >= 0; d-- {
		entries = append(entries, esi.HistoryEntry{
			Date:   time.Now().UTC().AddDate(0, 0, -d).Format("2006-01-02"),
			Volume: volume,
			// Keep OHLC sane so ancillary metrics remain defined.
			Average: 10,
			Highest: 11,
			Lowest:  9,
		})
	}
	return entries
}

func scannerWithHistory(regionID, typeID int32, entries []esi.HistoryEntry) *Scanner {
	return &Scanner{
		History: &testHistoryProvider{
			store: map[string][]esi.HistoryEntry{
				fmt.Sprintf("%d:%d", regionID, typeID): entries,
			},
		},
	}
}

func TestEnrichStationWithHistory_UsesFullRegionDepthDenominator_Single(t *testing.T) {
	const (
		regionID = int32(10000002)
		typeID   = int32(34)
	)

	s := scannerWithHistory(regionID, typeID, testHistoryFixedDailyVolume(100)) // regionFlowPerDay = 100
	results := []StationTrade{
		{
			TypeID:         typeID,
			StationID:      1,
			BuyVolume:      60,
			SellVolume:     40, // stationDepth = 100
			BuyOrderCount:  0,
			SellOrderCount: 0,
			ProfitPerUnit:  10,
		},
	}
	fullDepthByType := map[int32]int64{typeID: 1000}

	s.enrichStationWithHistory(results, regionID, map[stationTypeKey]*orderGroup{}, StationTradeParams{}, fullDepthByType, func(string) {})

	if got := results[0].DailyVolume; got != 10 {
		t.Fatalf("DailyVolume = %d, want 10 (100 * 100/1000)", got)
	}
	// With Buy/Sell depth 60/40, side-flow split of 10/day gives S2B=6, BfS=4.
	// competitors=0 => harmonic share is identity, so dailyShare=min(6,4)=4.
	if got := results[0].TheoreticalDailyProfit; got != 40 {
		t.Fatalf("TheoreticalDailyProfit = %v, want 40", got)
	}
}

func TestEnrichStationWithHistory_UsesFullRegionDepthDenominator_Radius(t *testing.T) {
	const (
		regionID = int32(10000002)
		typeID   = int32(35)
	)

	s := scannerWithHistory(regionID, typeID, testHistoryFixedDailyVolume(100)) // regionFlowPerDay = 100
	results := []StationTrade{
		{
			TypeID:         typeID,
			StationID:      10,
			BuyVolume:      60,
			SellVolume:     40, // depth 100 => share 0.1 => 10/day
			BuyOrderCount:  0,
			SellOrderCount: 0,
			ProfitPerUnit:  1,
		},
		{
			TypeID:         typeID,
			StationID:      11,
			BuyVolume:      180,
			SellVolume:     120, // depth 300 => share 0.3 => 30/day
			BuyOrderCount:  0,
			SellOrderCount: 0,
			ProfitPerUnit:  1,
		},
	}
	fullDepthByType := map[int32]int64{typeID: 1000}

	s.enrichStationWithHistory(results, regionID, map[stationTypeKey]*orderGroup{}, StationTradeParams{}, fullDepthByType, func(string) {})

	if results[0].DailyVolume != 10 {
		t.Fatalf("row0 DailyVolume = %d, want 10", results[0].DailyVolume)
	}
	if results[1].DailyVolume != 30 {
		t.Fatalf("row1 DailyVolume = %d, want 30", results[1].DailyVolume)
	}
}

func TestEnrichStationWithHistory_UsesFullRegionDepthDenominator_AllStations(t *testing.T) {
	const (
		regionID = int32(10000002)
		typeID   = int32(36)
	)

	s := scannerWithHistory(regionID, typeID, testHistoryFixedDailyVolume(100)) // regionFlowPerDay = 100
	results := []StationTrade{
		{TypeID: typeID, StationID: 101, BuyVolume: 120, SellVolume: 80},  // depth 200
		{TypeID: typeID, StationID: 102, BuyVolume: 180, SellVolume: 120}, // depth 300
		{TypeID: typeID, StationID: 103, BuyVolume: 300, SellVolume: 200}, // depth 500
	}
	fullDepthByType := map[int32]int64{typeID: 1000}

	s.enrichStationWithHistory(results, regionID, map[stationTypeKey]*orderGroup{}, StationTradeParams{}, fullDepthByType, func(string) {})

	if results[0].DailyVolume != 20 || results[1].DailyVolume != 30 || results[2].DailyVolume != 50 {
		t.Fatalf("DailyVolume split = [%d,%d,%d], want [20,30,50]",
			results[0].DailyVolume, results[1].DailyVolume, results[2].DailyVolume)
	}
	sum := results[0].DailyVolume + results[1].DailyVolume + results[2].DailyVolume
	if sum != 100 {
		t.Fatalf("sum DailyVolume = %d, want 100", sum)
	}
}
