package engine

import (
	"fmt"
	"math"
	"testing"
	"time"

	"eve-flipper/internal/esi"
	"eve-flipper/internal/graph"
	"eve-flipper/internal/sde"
)

func TestSanitizeFloat_Normal(t *testing.T) {
	if v := sanitizeFloat(42.5); v != 42.5 {
		t.Errorf("sanitizeFloat(42.5) = %v, want 42.5", v)
	}
}

func TestSanitizeFloat_Zero(t *testing.T) {
	if v := sanitizeFloat(0); v != 0 {
		t.Errorf("sanitizeFloat(0) = %v, want 0", v)
	}
}

func TestSanitizeFloat_NaN(t *testing.T) {
	if v := sanitizeFloat(math.NaN()); v != 0 {
		t.Errorf("sanitizeFloat(NaN) = %v, want 0", v)
	}
}

func TestSanitizeFloat_PosInf(t *testing.T) {
	if v := sanitizeFloat(math.Inf(1)); v != 0 {
		t.Errorf("sanitizeFloat(+Inf) = %v, want 0", v)
	}
}

func TestSanitizeFloat_NegInf(t *testing.T) {
	if v := sanitizeFloat(math.Inf(-1)); v != 0 {
		t.Errorf("sanitizeFloat(-Inf) = %v, want 0", v)
	}
}

func TestSanitizeFloat_Negative(t *testing.T) {
	if v := sanitizeFloat(-100.5); v != -100.5 {
		t.Errorf("sanitizeFloat(-100.5) = %v, want -100.5", v)
	}
}

func TestProfitCalculation(t *testing.T) {
	// Simulate the core profit formula from calculateResults
	salesTaxPercent := 8.0
	taxMult := 1.0 - salesTaxPercent/100 // 0.92

	sellPrice := 100.0 // cheapest sell order (we buy here)
	buyPrice := 200.0  // highest buy order (we sell here)
	cargoCapacity := 500.0
	itemVolume := 10.0

	effectiveSellPrice := buyPrice * taxMult        // 184
	profitPerUnit := effectiveSellPrice - sellPrice // 84
	margin := profitPerUnit / sellPrice * 100       // 84%

	units := int32(math.Floor(cargoCapacity / itemVolume)) // 50
	totalProfit := profitPerUnit * float64(units)          // 4200

	if math.Abs(taxMult-0.92) > 1e-9 {
		t.Errorf("taxMult = %v, want 0.92", taxMult)
	}
	if math.Abs(effectiveSellPrice-184) > 1e-9 {
		t.Errorf("effectiveSellPrice = %v, want 184", effectiveSellPrice)
	}
	if math.Abs(profitPerUnit-84) > 1e-9 {
		t.Errorf("profitPerUnit = %v, want 84", profitPerUnit)
	}
	if math.Abs(margin-84) > 1e-9 {
		t.Errorf("margin = %v%%, want 84%%", margin)
	}
	if units != 50 {
		t.Errorf("units = %d, want 50", units)
	}
	if math.Abs(totalProfit-4200) > 1e-9 {
		t.Errorf("totalProfit = %v, want 4200", totalProfit)
	}
}

func TestProfitCalculation_ZeroTax(t *testing.T) {
	taxMult := 1.0 - 0.0/100
	buyPrice := 150.0
	sellPrice := 100.0
	effective := buyPrice * taxMult
	profit := effective - sellPrice

	if math.Abs(profit-50) > 1e-9 {
		t.Errorf("profit with 0%% tax = %v, want 50", profit)
	}
}

func TestProfitCalculation_HighTax(t *testing.T) {
	taxMult := 1.0 - 100.0/100 // 0
	buyPrice := 150.0
	sellPrice := 100.0
	effective := buyPrice * taxMult
	profit := effective - sellPrice

	if math.Abs(profit-(-100)) > 1e-9 {
		t.Errorf("profit with 100%% tax = %v, want -100", profit)
	}
}

type testHistoryProvider struct {
	store map[string][]esi.HistoryEntry
}

func (h *testHistoryProvider) key(regionID, typeID int32) string {
	return fmt.Sprintf("%d:%d", regionID, typeID)
}

func (h *testHistoryProvider) GetMarketHistory(regionID int32, typeID int32) ([]esi.HistoryEntry, bool) {
	entries, ok := h.store[h.key(regionID, typeID)]
	return entries, ok
}

func (h *testHistoryProvider) SetMarketHistory(regionID int32, typeID int32, entries []esi.HistoryEntry) {
	h.store[h.key(regionID, typeID)] = entries
}

func TestEnrichWithHistory_AppliesToAllDuplicateRegionTypeResults(t *testing.T) {
	u := graph.NewUniverse()
	u.SetRegion(30000142, 10000002)

	now := time.Now().UTC()
	history := []esi.HistoryEntry{
		{Date: now.AddDate(0, 0, -2).Format("2006-01-02"), Average: 100, Volume: 100},
		{Date: now.AddDate(0, 0, -1).Format("2006-01-02"), Average: 110, Volume: 200},
		{Date: now.Format("2006-01-02"), Average: 120, Volume: 300},
	}

	hp := &testHistoryProvider{
		store: map[string][]esi.HistoryEntry{
			"10000002:34": history,
		},
	}

	s := &Scanner{
		SDE: &sde.Data{
			Universe: u,
		},
		History: hp,
	}

	results := []FlipResult{
		{
			TypeID:          34,
			SellSystemID:    30000142,
			SellOrderRemain: 150,
			BuyOrderRemain:  50, // totalListed = 200
		},
		{
			TypeID:          34,
			SellSystemID:    30000142,
			SellOrderRemain: 15,
			BuyOrderRemain:  5, // totalListed = 20
		},
	}

	s.enrichWithHistory(results, func(string) {})

	if results[0].DailyVolume <= 0 || results[1].DailyVolume <= 0 {
		t.Fatalf("expected both results to have non-zero DailyVolume, got %d and %d", results[0].DailyVolume, results[1].DailyVolume)
	}
	if results[0].PriceTrend == 0 || results[1].PriceTrend == 0 {
		t.Fatalf("expected both results to have non-zero PriceTrend, got %f and %f", results[0].PriceTrend, results[1].PriceTrend)
	}
	if results[0].Velocity == 0 || results[1].Velocity == 0 {
		t.Fatalf("expected both results to have non-zero Velocity, got %f and %f", results[0].Velocity, results[1].Velocity)
	}
	if math.Abs(results[0].Velocity-results[1].Velocity) < 1e-9 {
		t.Fatalf("expected different Velocity values because totalListed differs, got %f and %f", results[0].Velocity, results[1].Velocity)
	}
}

func TestFindSafeExecutionQuantity_CapsToFillableDepth(t *testing.T) {
	asks := []esi.MarketOrder{
		{Price: 10, VolumeRemain: 100},
	}
	bids := []esi.MarketOrder{
		{Price: 15, VolumeRemain: 60},
	}

	qty, planBuy, planSell, expected := findSafeExecutionQuantity(asks, bids, 80, 1.0, 1.0)

	if qty != 60 {
		t.Fatalf("qty = %d, want 60", qty)
	}
	if !planBuy.CanFill || !planSell.CanFill {
		t.Fatalf("expected both plans to be fillable at safe qty")
	}
	if expected <= 0 {
		t.Fatalf("expected profit must be positive, got %f", expected)
	}
}

func TestFindSafeExecutionQuantity_ReducesToProfitableQty(t *testing.T) {
	asks := []esi.MarketOrder{
		{Price: 10, VolumeRemain: 50},
		{Price: 20, VolumeRemain: 100},
	}
	bids := []esi.MarketOrder{
		{Price: 15, VolumeRemain: 200},
	}

	qty, _, _, expected := findSafeExecutionQuantity(asks, bids, 100, 1.0, 1.0)

	if qty != 99 {
		t.Fatalf("qty = %d, want 99", qty)
	}
	if expected <= 0 {
		t.Fatalf("expected profit must stay positive, got %f", expected)
	}
}

func TestFindSafeExecutionQuantity_NoProfitableQty(t *testing.T) {
	asks := []esi.MarketOrder{
		{Price: 20, VolumeRemain: 100},
	}
	bids := []esi.MarketOrder{
		{Price: 10, VolumeRemain: 100},
	}

	qty, _, _, expected := findSafeExecutionQuantity(asks, bids, 50, 1.0, 1.0)

	if qty != 0 {
		t.Fatalf("qty = %d, want 0", qty)
	}
	if expected != 0 {
		t.Fatalf("expected profit = %f, want 0", expected)
	}
}
