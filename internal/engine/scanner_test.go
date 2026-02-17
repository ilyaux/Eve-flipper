package engine

import (
	"fmt"
	"math"
	"math/rand"
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

func TestHarmonicDailyShare_MonotoneAndBounded(t *testing.T) {
	const daily = int64(10_000)
	if got := harmonicDailyShare(0, 5); got != 0 {
		t.Fatalf("harmonicDailyShare(0, 5) = %d, want 0", got)
	}

	prev := harmonicDailyShare(daily, 0)
	if prev != daily {
		t.Fatalf("harmonicDailyShare(%d,0) = %d, want %d", daily, prev, daily)
	}
	for competitors := 1; competitors <= 200; competitors++ {
		got := harmonicDailyShare(daily, competitors)
		if got <= 0 || got > daily {
			t.Fatalf("competitors=%d: share=%d out of bounds [1,%d]", competitors, got, daily)
		}
		if got > prev {
			t.Fatalf("non-monotone share: competitors=%d share=%d > previous=%d", competitors, got, prev)
		}
		prev = got
	}
}

func TestEstimateSideFlowsPerDay_MonotoneByBuyDepth(t *testing.T) {
	const (
		total    = 1_000.0
		sellBook = int64(1_000)
		eps      = 1e-9
	)
	prevS2B := -1.0
	prevBfS := total + 1

	for buyDepth := int64(0); buyDepth <= 5_000; buyDepth += 100 {
		s2b, bfs := estimateSideFlowsPerDay(total, buyDepth, sellBook)
		if math.Abs((s2b+bfs)-total) > eps {
			t.Fatalf("mass-balance broken for buyDepth=%d: s2b+bfs=%f, want %f", buyDepth, s2b+bfs, total)
		}
		if s2b < prevS2B-eps {
			t.Fatalf("S2B decreased with higher buy depth: prev=%f, cur=%f", prevS2B, s2b)
		}
		if bfs > prevBfS+eps {
			t.Fatalf("BfS increased with higher buy depth: prev=%f, cur=%f", prevBfS, bfs)
		}
		prevS2B = s2b
		prevBfS = bfs
	}
}

func TestExpectedProfitForPlans_LinearAndFeeSensitive(t *testing.T) {
	planBuy := ExecutionPlanResult{ExpectedPrice: 100}
	planSell := ExecutionPlanResult{ExpectedPrice: 120}

	p10 := expectedProfitForPlans(planBuy, planSell, 10, 1.01, 0.98)
	p20 := expectedProfitForPlans(planBuy, planSell, 20, 1.01, 0.98)
	if math.Abs(p20-2*p10) > 1e-9 {
		t.Fatalf("linearity broken: p20=%f, want %f", p20, 2*p10)
	}

	base := expectedProfitForPlans(planBuy, planSell, 10, 1.0, 1.0)
	higherBuyFees := expectedProfitForPlans(planBuy, planSell, 10, 1.05, 1.0)
	lowerSellRevenue := expectedProfitForPlans(planBuy, planSell, 10, 1.0, 0.95)
	if higherBuyFees >= base {
		t.Fatalf("profit should decrease with higher buy fees: base=%f highBuyFee=%f", base, higherBuyFees)
	}
	if lowerSellRevenue >= base {
		t.Fatalf("profit should decrease with lower sell revenue: base=%f lowSellRev=%f", base, lowerSellRevenue)
	}
}

func TestEstimateFlipDailyExecutableUnitsPerDay_CycleBounded(t *testing.T) {
	if got := estimateFlipDailyExecutableUnitsPerDay(1_000, 600, 200); got != 200 {
		t.Fatalf("cycle bound should be min(S2B,BfS): got=%d want=200", got)
	}
	if got := estimateFlipDailyExecutableUnitsPerDay(150, 600, 200); got != 150 {
		t.Fatalf("units cap should apply: got=%d want=150", got)
	}
	if got := estimateFlipDailyExecutableUnitsPerDay(1_000, 0, 200); got != 0 {
		t.Fatalf("zero side-flow should yield zero executable units, got=%d", got)
	}
	if got := estimateFlipDailyExecutableUnitsPerDay(1_000, -5, 200); got != 0 {
		t.Fatalf("negative side-flow should yield zero executable units, got=%d", got)
	}
}

func TestFindSafeExecutionQuantity_MatchesExhaustiveLargestProfitableQty(t *testing.T) {
	rng := rand.New(rand.NewSource(42))

	for tc := 0; tc < 250; tc++ {
		askLevels := 1 + rng.Intn(5)
		bidLevels := 1 + rng.Intn(5)

		asks := make([]esi.MarketOrder, 0, askLevels)
		bids := make([]esi.MarketOrder, 0, bidLevels)

		askBase := 90.0 + rng.Float64()*20.0  // 90..110
		bidBase := 100.0 + rng.Float64()*20.0 // 100..120
		for i := 0; i < askLevels; i++ {
			asks = append(asks, esi.MarketOrder{
				Price:        askBase + float64(i)*rng.Float64()*3.0,
				VolumeRemain: int32(10 + rng.Intn(80)),
			})
		}
		for i := 0; i < bidLevels; i++ {
			bids = append(bids, esi.MarketOrder{
				Price:        bidBase - float64(i)*rng.Float64()*3.0,
				VolumeRemain: int32(10 + rng.Intn(80)),
			})
		}

		desired := int32(1 + rng.Intn(150))

		var bruteQty int32
		for q := int32(1); q <= desired; q++ {
			pb := ComputeExecutionPlan(asks, q, true)
			ps := ComputeExecutionPlan(bids, q, false)
			if !pb.CanFill || !ps.CanFill {
				continue
			}
			if expectedProfitForPlans(pb, ps, q, 1.0, 1.0) > 0 {
				bruteQty = q
			}
		}

		gotQty, _, _, _ := findSafeExecutionQuantity(asks, bids, desired, 1.0, 1.0)
		if gotQty != bruteQty {
			t.Fatalf("tc=%d desired=%d gotQty=%d bruteQty=%d", tc, desired, gotQty, bruteQty)
		}
	}
}
