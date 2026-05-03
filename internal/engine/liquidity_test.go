package engine

import (
	"testing"

	"eve-flipper/internal/esi"
)

func TestEstimateCycleFillTimeDaysRequiresBothFlows(t *testing.T) {
	if got := estimateCycleFillTimeDays(100, 50, 25); got != 4 {
		t.Fatalf("fill days = %v, want 4", got)
	}
	if got := estimateCycleFillTimeDays(100, 0, 25); got != 0 {
		t.Fatalf("missing source flow should be unknown, got %v", got)
	}
	if got := estimateCycleFillTimeDays(100, 50, 0); got != 0 {
		t.Fatalf("missing liquidation flow should be unknown, got %v", got)
	}
}

func TestLiquidityScoreFromFillTime(t *testing.T) {
	score, label := liquidityScoreFromFillTime(2.5, true)
	if score != 80 || label != "high" {
		t.Fatalf("score/label = %v/%s, want 80/high", score, label)
	}
	score, label = liquidityScoreFromFillTime(20, true)
	if score != 25 || label != "low" {
		t.Fatalf("score/label = %v/%s, want 25/low", score, label)
	}
	score, label = liquidityScoreFromFillTime(2, false)
	if score != 0 || label != "unknown" {
		t.Fatalf("no history score/label = %v/%s, want 0/unknown", score, label)
	}
}

func TestComputeHistoricalFillBacktest(t *testing.T) {
	entries := []esi.HistoryEntry{
		{Date: "2026-01-01", Volume: 5},
		{Date: "2026-01-02", Volume: 10},
		{Date: "2026-01-03", Volume: 20},
		{Date: "2026-01-04", Volume: 30},
	}
	got := computeHistoricalFillBacktest(entries, 20)
	if got.Days != 4 {
		t.Fatalf("days = %d, want 4", got.Days)
	}
	if got.FillRate != 50 {
		t.Fatalf("fill rate = %v, want 50", got.FillRate)
	}
	if got.MedianVol != 15 {
		t.Fatalf("median volume = %d, want 15", got.MedianVol)
	}
}

func TestEnrichFlipResultsWithInventory(t *testing.T) {
	rows := []FlipResult{{TypeID: 34}, {TypeID: 35}}
	inv := &RegionalInventorySnapshot{
		AssetsByType:     map[int32]int64{34: 10},
		ActiveBuyByType:  map[int32]int64{34: 3},
		ActiveSellByType: map[int32]int64{35: 7},
	}
	EnrichFlipResultsWithInventory(rows, inv)
	if rows[0].CharacterAssets != 10 || rows[0].CharacterBuyOrders != 3 {
		t.Fatalf("type 34 inventory = assets %d buy %d", rows[0].CharacterAssets, rows[0].CharacterBuyOrders)
	}
	if rows[1].CharacterSellOrders != 7 {
		t.Fatalf("type 35 sell orders = %d, want 7", rows[1].CharacterSellOrders)
	}
}

func TestEnrichStationTradesWithInventory(t *testing.T) {
	rows := []StationTrade{{TypeID: 34}, {TypeID: 35}}
	inv := &RegionalInventorySnapshot{
		AssetsByType:     map[int32]int64{34: 11},
		ActiveBuyByType:  map[int32]int64{35: 4},
		ActiveSellByType: map[int32]int64{34: 6},
	}
	EnrichStationTradesWithInventory(rows, inv)
	if rows[0].CharacterAssets != 11 || rows[0].CharacterSellOrders != 6 {
		t.Fatalf("type 34 station inventory = assets %d sell %d", rows[0].CharacterAssets, rows[0].CharacterSellOrders)
	}
	if rows[1].CharacterBuyOrders != 4 {
		t.Fatalf("type 35 station buy orders = %d, want 4", rows[1].CharacterBuyOrders)
	}
}
