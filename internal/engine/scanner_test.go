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
		{Price: 15, VolumeRemain: 60, IsBuyOrder: true},
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
		{Price: 15, VolumeRemain: 200, IsBuyOrder: true},
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
		{Price: 10, VolumeRemain: 100, IsBuyOrder: true},
	}

	qty, _, _, expected := findSafeExecutionQuantity(asks, bids, 50, 1.0, 1.0)

	if qty != 0 {
		t.Fatalf("qty = %d, want 0", qty)
	}
	if expected != 0 {
		t.Fatalf("expected profit = %f, want 0", expected)
	}
}

func TestCalculateResults_TracksBestLevelPriceAndQty(t *testing.T) {
	u := graph.NewUniverse()
	u.SetRegion(1, 10000002)
	u.SetRegion(2, 10000002)
	u.SetSecurity(1, 0.9)
	u.SetSecurity(2, 0.9)
	u.AddGate(1, 2)
	u.AddGate(2, 1)

	scanner := &Scanner{
		SDE: &sde.Data{
			Universe: u,
			Systems: map[int32]*sde.SolarSystem{
				1: {ID: 1, Name: "Alpha", RegionID: 10000002},
				2: {ID: 2, Name: "Beta", RegionID: 10000002},
			},
			Types: map[int32]*sde.ItemType{
				34: {ID: 34, Name: "Tritanium", Volume: 0.01},
			},
		},
		ESI: esi.NewClient(nil),
	}

	const (
		typeID       = int32(34)
		buyLocID     = int64(100000000001)
		sellLocID    = int64(100000000002)
		currentSys   = int32(1)
		buySystemID  = int32(1)
		sellSystemID = int32(2)
	)

	asks := []esi.MarketOrder{
		{TypeID: typeID, LocationID: buyLocID, SystemID: buySystemID, Price: 10, VolumeRemain: 5},
		{TypeID: typeID, LocationID: buyLocID, SystemID: buySystemID, Price: 10, VolumeRemain: 7},
		{TypeID: typeID, LocationID: buyLocID, SystemID: buySystemID, Price: 11, VolumeRemain: 20},
	}
	bids := []esi.MarketOrder{
		{TypeID: typeID, LocationID: sellLocID, SystemID: sellSystemID, Price: 15, VolumeRemain: 4, IsBuyOrder: true},
		{TypeID: typeID, LocationID: sellLocID, SystemID: sellSystemID, Price: 15, VolumeRemain: 6, IsBuyOrder: true},
		{TypeID: typeID, LocationID: sellLocID, SystemID: sellSystemID, Price: 14, VolumeRemain: 50, IsBuyOrder: true},
	}

	idx := &scanIndex{
		sellByType: map[int32][]sellInfo{
			typeID: {
				{Price: 10, VolumeRemain: 5, LocationID: buyLocID, SystemID: buySystemID},
				{Price: 10, VolumeRemain: 7, LocationID: buyLocID, SystemID: buySystemID},
				{Price: 11, VolumeRemain: 20, LocationID: buyLocID, SystemID: buySystemID},
			},
		},
		buyByType: map[int32][]buyInfo{
			typeID: {
				{Price: 15, VolumeRemain: 4, LocationID: sellLocID, SystemID: sellSystemID},
				{Price: 15, VolumeRemain: 6, LocationID: sellLocID, SystemID: sellSystemID},
				{Price: 14, VolumeRemain: 50, LocationID: sellLocID, SystemID: sellSystemID},
			},
		},
		sellOrders: asks,
		buyOrders:  bids,
		sellSideBuyDepthByType: map[int32]int64{
			typeID: 60,
		},
		sellSideSellDepthByType: map[int32]int64{
			typeID: 32,
		},
	}

	params := ScanParams{
		CurrentSystemID: currentSys,
		CargoCapacity:   1_000_000,
		MinMargin:       0.1,
	}
	bfs := map[int32]int{
		currentSys: 0,
	}

	results, err := scanner.calculateResults(params, idx, bfs, func(string) {})
	if err != nil {
		t.Fatalf("calculateResults error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}

	r := results[0]
	if r.BuyPrice != 10 || r.BestAskPrice != 10 {
		t.Fatalf("BuyPrice/BestAskPrice = %v/%v, want 10/10", r.BuyPrice, r.BestAskPrice)
	}
	if r.SellPrice != 15 || r.BestBidPrice != 15 {
		t.Fatalf("SellPrice/BestBidPrice = %v/%v, want 15/15", r.SellPrice, r.BestBidPrice)
	}
	if r.BestAskQty != 12 {
		t.Fatalf("BestAskQty = %d, want 12", r.BestAskQty)
	}
	if r.BestBidQty != 10 {
		t.Fatalf("BestBidQty = %d, want 10", r.BestBidQty)
	}
	if r.FilledQty <= 0 || r.RealProfit <= 0 {
		t.Fatalf("expected depth-aware execution fields to be populated, got FilledQty=%d RealProfit=%f", r.FilledQty, r.RealProfit)
	}
}

func TestCalculateResults_CargoCapacityZeroMeansUnlimited(t *testing.T) {
	u := graph.NewUniverse()
	u.SetRegion(1, 10000002)
	u.SetRegion(2, 10000002)
	u.SetSecurity(1, 0.9)
	u.SetSecurity(2, 0.9)
	u.AddGate(1, 2)
	u.AddGate(2, 1)

	const (
		typeID       = int32(55555)
		buyLocID     = int64(200000000001)
		sellLocID    = int64(200000000002)
		currentSys   = int32(1)
		buySystemID  = int32(1)
		sellSystemID = int32(2)
	)

	scanner := &Scanner{
		SDE: &sde.Data{
			Universe: u,
			Systems: map[int32]*sde.SolarSystem{
				1: {ID: 1, Name: "Alpha", RegionID: 10000002},
				2: {ID: 2, Name: "Beta", RegionID: 10000002},
			},
			Types: map[int32]*sde.ItemType{
				typeID: {ID: typeID, Name: "Bulky Item", Volume: 2_000_000},
			},
		},
		ESI: esi.NewClient(nil),
	}

	asks := []esi.MarketOrder{
		{TypeID: typeID, LocationID: buyLocID, SystemID: buySystemID, Price: 100, VolumeRemain: 1},
	}
	bids := []esi.MarketOrder{
		{TypeID: typeID, LocationID: sellLocID, SystemID: sellSystemID, Price: 150, VolumeRemain: 1, IsBuyOrder: true},
	}

	idx := &scanIndex{
		sellByType: map[int32][]sellInfo{
			typeID: {
				{Price: 100, VolumeRemain: 1, LocationID: buyLocID, SystemID: buySystemID},
			},
		},
		buyByType: map[int32][]buyInfo{
			typeID: {
				{Price: 150, VolumeRemain: 1, LocationID: sellLocID, SystemID: sellSystemID},
			},
		},
		sellOrders: asks,
		buyOrders:  bids,
		sellSideBuyDepthByType: map[int32]int64{
			typeID: 1,
		},
		sellSideSellDepthByType: map[int32]int64{
			typeID: 1,
		},
	}

	bfs := map[int32]int{currentSys: 0}

	limitedParams := ScanParams{
		CurrentSystemID: currentSys,
		CargoCapacity:   1_000_000,
		MinMargin:       0.1,
	}
	limitedResults, err := scanner.calculateResults(limitedParams, idx, bfs, func(string) {})
	if err != nil {
		t.Fatalf("calculateResults(limited) error: %v", err)
	}
	if len(limitedResults) != 0 {
		t.Fatalf("limited cargo should filter bulky item, got %d rows", len(limitedResults))
	}

	unlimitedParams := limitedParams
	unlimitedParams.CargoCapacity = 0
	unlimitedResults, err := scanner.calculateResults(unlimitedParams, idx, bfs, func(string) {})
	if err != nil {
		t.Fatalf("calculateResults(unlimited) error: %v", err)
	}
	if len(unlimitedResults) != 1 {
		t.Fatalf("cargo=0 should disable cargo cap, got %d rows", len(unlimitedResults))
	}
	if unlimitedResults[0].UnitsToBuy != 1 {
		t.Fatalf("unlimited units_to_buy = %d, want 1", unlimitedResults[0].UnitsToBuy)
	}
}

func TestCalculateResults_CargoCapacityClampsQuantityWithoutDroppingRow(t *testing.T) {
	u := graph.NewUniverse()
	u.SetRegion(1, 10000002)
	u.SetRegion(2, 10000002)
	u.SetSecurity(1, 0.9)
	u.SetSecurity(2, 0.9)
	u.AddGate(1, 2)
	u.AddGate(2, 1)

	const (
		typeID       = int32(77777)
		buyLocID     = int64(300000000001)
		sellLocID    = int64(300000000002)
		currentSys   = int32(1)
		buySystemID  = int32(1)
		sellSystemID = int32(2)
	)

	scanner := &Scanner{
		SDE: &sde.Data{
			Universe: u,
			Systems: map[int32]*sde.SolarSystem{
				1: {ID: 1, Name: "Alpha", RegionID: 10000002},
				2: {ID: 2, Name: "Beta", RegionID: 10000002},
			},
			Types: map[int32]*sde.ItemType{
				typeID: {ID: typeID, Name: "Cargo-Limited Item", Volume: 100},
			},
		},
		ESI: esi.NewClient(nil),
	}

	asks := []esi.MarketOrder{
		{TypeID: typeID, LocationID: buyLocID, SystemID: buySystemID, Price: 100, VolumeRemain: 10},
	}
	bids := []esi.MarketOrder{
		{TypeID: typeID, LocationID: sellLocID, SystemID: sellSystemID, Price: 150, VolumeRemain: 10, IsBuyOrder: true},
	}

	idx := &scanIndex{
		sellByType: map[int32][]sellInfo{
			typeID: {
				{Price: 100, VolumeRemain: 10, LocationID: buyLocID, SystemID: buySystemID},
			},
		},
		buyByType: map[int32][]buyInfo{
			typeID: {
				{Price: 150, VolumeRemain: 10, LocationID: sellLocID, SystemID: sellSystemID},
			},
		},
		sellOrders: asks,
		buyOrders:  bids,
		sellSideBuyDepthByType: map[int32]int64{
			typeID: 10,
		},
		sellSideSellDepthByType: map[int32]int64{
			typeID: 10,
		},
	}

	params := ScanParams{
		CurrentSystemID: currentSys,
		CargoCapacity:   250, // 2 units max for volume=100
		MinMargin:       0.1,
	}
	bfs := map[int32]int{currentSys: 0}

	results, err := scanner.calculateResults(params, idx, bfs, func(string) {})
	if err != nil {
		t.Fatalf("calculateResults error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected row to survive cargo clamp, got %d rows", len(results))
	}
	if results[0].UnitsToBuy != 2 {
		t.Fatalf("units_to_buy = %d, want 2", results[0].UnitsToBuy)
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
		if got < 0 || got > daily {
			t.Fatalf("competitors=%d: share=%d out of bounds [0,%d]", competitors, got, daily)
		}
		if got > prev {
			t.Fatalf("non-monotone share: competitors=%d share=%d > previous=%d", competitors, got, prev)
		}
		prev = got
	}
}

func TestHarmonicDailyShare_ThinLiquidityCanRoundToZero(t *testing.T) {
	got := harmonicDailyShare(1, 200)
	if got != 0 {
		t.Fatalf("harmonicDailyShare(1, 200) = %d, want 0", got)
	}
}

func TestHarmonicDailyShare_SumOfSharesLEVolume(t *testing.T) {
	// Property: rounded harmonic shares across all positions should conserve
	// daily volume up to bounded rounding error (<= number of positions).
	const daily = int64(1_000)
	for n := 1; n <= 50; n++ {
		hn := 0.0
		for k := 1; k <= n; k++ {
			hn += 1.0 / float64(k)
		}

		var roundedTotal int64
		for pos := 1; pos <= n; pos++ {
			share := float64(daily) * (1.0 / float64(pos)) / hn
			rounded := int64(math.Round(share))
			if rounded < 0 {
				rounded = 0
			}
			roundedTotal += rounded
		}

		diff := roundedTotal - daily
		if diff < 0 {
			diff = -diff
		}
		if diff > int64(n) {
			t.Fatalf("n=%d: rounded total=%d daily=%d diff=%d (too large)", n, roundedTotal, daily, diff)
		}

		// Player share should match the median-position harmonic share model.
		got := harmonicDailyShare(daily, n-1)
		medianPos := (n + 1) / 2
		expected := int64(math.Round(float64(daily) * (1.0 / float64(medianPos)) / hn))
		if expected < 0 {
			expected = 0
		}
		if got != expected {
			t.Fatalf("n=%d: harmonicDailyShare=%d, expected median share=%d", n, got, expected)
		}
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

func TestEstimateSideFlowsPerDay_NoBookDataFallsBackNeutral(t *testing.T) {
	const total = 1_000.0
	s2b, bfs := estimateSideFlowsPerDay(total, 0, 0)
	if math.Abs((s2b+bfs)-total) > 1e-9 {
		t.Fatalf("mass-balance broken: s2b+bfs=%f, want=%f", s2b+bfs, total)
	}
	if math.Abs(s2b-total*0.5) > 1e-9 {
		t.Fatalf("S2B fallback=%f, want=%f", s2b, total*0.5)
	}
	if math.Abs(bfs-total*0.5) > 1e-9 {
		t.Fatalf("BfS fallback=%f, want=%f", bfs, total*0.5)
	}
}

func TestEstimateSideFlowsPerDay_OneSidedDepthUsesTailShare(t *testing.T) {
	const total = 1_000.0
	s2bBuyOnly, bfsBuyOnly := estimateSideFlowsPerDay(total, 500, 0)
	s2bSellOnly, bfsSellOnly := estimateSideFlowsPerDay(total, 0, 500)

	if math.Abs((s2bBuyOnly+bfsBuyOnly)-total) > 1e-9 || math.Abs((s2bSellOnly+bfsSellOnly)-total) > 1e-9 {
		t.Fatalf("mass-balance broken for one-sided depth")
	}
	if math.Abs(s2bBuyOnly-total*(1-sideFlowMinShare)) > 1e-9 {
		t.Fatalf("buy-only S2B=%f, want=%f", s2bBuyOnly, total*(1-sideFlowMinShare))
	}
	if math.Abs(s2bSellOnly-total*sideFlowMinShare) > 1e-9 {
		t.Fatalf("sell-only S2B=%f, want=%f", s2bSellOnly, total*sideFlowMinShare)
	}
}

func TestEstimateSideFlowsPerDay_HighCoverageUsesSplitSignal(t *testing.T) {
	const total = 1_000.0
	// Same 9:1 imbalance, different absolute depth.
	lowDepthS2B, _ := estimateSideFlowsPerDay(total, 90, 10)
	highDepthS2B, _ := estimateSideFlowsPerDay(total, 9_000, 1_000)

	if highDepthS2B <= lowDepthS2B {
		t.Fatalf("expected stronger split with higher coverage: low=%f high=%f", lowDepthS2B, highDepthS2B)
	}
	// Low coverage should stay close to neutral (0.5 * total).
	if math.Abs(lowDepthS2B-total*0.5) > total*0.08 {
		t.Fatalf("low-coverage split too far from neutral: got=%f neutral=%f", lowDepthS2B, total*0.5)
	}
}

func TestReconcileSideFlowShare_ClampsExtremeShares(t *testing.T) {
	high := reconcileSideFlowShare(0.5, 1.0, 1.0)
	low := reconcileSideFlowShare(0.5, 0.0, 1.0)

	if math.Abs(high-(1-sideFlowMinShare)) > 1e-9 {
		t.Fatalf("high share clamp=%f, want=%f", high, 1-sideFlowMinShare)
	}
	if math.Abs(low-sideFlowMinShare) > 1e-9 {
		t.Fatalf("low share clamp=%f, want=%f", low, sideFlowMinShare)
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
