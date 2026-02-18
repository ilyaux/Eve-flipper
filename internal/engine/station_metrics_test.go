package engine

import (
	"math"
	"testing"
	"time"

	"eve-flipper/internal/esi"
)

// --- CalcVWAP: VWAP = sum(avg*vol) / sum(vol) ---

func TestCalcVWAP_Exact(t *testing.T) {
	base := time.Now().AddDate(0, 0, -3).Format("2006-01-02")
	d1 := time.Now().AddDate(0, 0, -2).Format("2006-01-02")
	d2 := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	history := []esi.HistoryEntry{
		{Date: base, Average: 100, Highest: 105, Lowest: 95, Volume: 1000, OrderCount: 10},
		{Date: d1, Average: 200, Highest: 210, Lowest: 190, Volume: 2000, OrderCount: 20},
		{Date: d2, Average: 150, Highest: 155, Lowest: 145, Volume: 500, OrderCount: 5},
	}
	// VWAP = (100*1000 + 200*2000 + 150*500) / (1000+2000+500) = (100000+400000+75000)/3500 = 575000/3500 = 164.285714...
	got := CalcVWAP(history, 30)
	want := 100.0*1000 + 200.0*2000 + 150.0*500
	want /= 1000 + 2000 + 500
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("CalcVWAP = %v, want %v", got, want)
	}
}

func TestCalcVWAP_EmptyAndZeroVolume(t *testing.T) {
	if got := CalcVWAP(nil, 7); got != 0 {
		t.Errorf("CalcVWAP(nil) = %v, want 0", got)
	}
	history := []esi.HistoryEntry{{Date: time.Now().Format("2006-01-02"), Average: 100, Volume: 0}}
	if got := CalcVWAP(history, 7); got != 0 {
		t.Errorf("CalcVWAP(zero volume) = %v, want 0", got)
	}
}

// --- CalcDRVI: stdDev of daily range % = (Highest-Lowest)/Average*100 ---
// (Renamed from CalcPVI to avoid confusion with classic Positive Volume Index)

func TestCalcDRVI_Exact(t *testing.T) {
	base := time.Now().AddDate(0, 0, -2).Format("2006-01-02")
	d1 := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	// Day1: range% = (110-90)/100*100 = 20. Day2: (120-80)/100*100 = 40.
	// Mean=30, sample variance = ((20-30)^2+(40-30)^2)/(2-1) = 200, sample std = sqrt(200) ≈ 14.1421
	history := []esi.HistoryEntry{
		{Date: base, Average: 100, Highest: 110, Lowest: 90, Volume: 1000},
		{Date: d1, Average: 100, Highest: 120, Lowest: 80, Volume: 1000},
	}
	got := CalcDRVI(history, 30)
	want := math.Sqrt(200.0) // sample std of [20, 40] with Bessel's correction
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("CalcDRVI = %v, want %v", got, want)
	}
}

func TestCalcDRVI_LessThanTwoDays(t *testing.T) {
	history := []esi.HistoryEntry{{Date: time.Now().Format("2006-01-02"), Average: 100, Highest: 105, Lowest: 95, Volume: 100}}
	if got := CalcDRVI(history, 7); got != 0 {
		t.Errorf("CalcDRVI(1 day) = %v, want 0", got)
	}
}

// --- CalcOBDS: min(buyDepth, sellDepth) / capitalRequired, where depth is volume within ±5% of best ---

func TestCalcOBDS_Exact(t *testing.T) {
	// Best buy = 100, best sell = 110. Buy orders within 5% of 100: 100 to 100 (only 100). Sell within 5% of 110: 110 to 115.5.
	// sumVolumeWithinPercent: for buy orders, priceDiff = (refPrice - o.Price)/refPrice*100, count if 0<=priceDiff<=5.
	// So buy at 100: (100-100)/100*100=0 -> count. Buy at 98: (100-98)/100*100=2 -> count. Buy at 94: 6 -> no.
	// For sell: priceDiff = (o.Price - refPrice)/refPrice*100. Sell at 110: 0. Sell at 112: 1.82. Sell at 115: 4.54. All within 5%.
	buyOrders := []esi.MarketOrder{
		{Price: 100, VolumeRemain: 100},
		{Price: 98, VolumeRemain: 50},
	}
	sellOrders := []esi.MarketOrder{
		{Price: 110, VolumeRemain: 200},
		{Price: 112, VolumeRemain: 100},
	}
	// Best buy = 100, best sell = 110.
	// Buy depth within 5%: 100@100 (0%), 98@50 (2%) -> 100*100 + 50*98 = 10000+4900 = 14900.
	// Sell depth within 5%: 110@200 (0%), 112@100 (1.82%) -> 200*110 + 100*112 = 22000+11200 = 33200.
	// minDepth = 14900, capitalRequired = 10000 -> OBDS = 14900/10000 = 1.49
	got := CalcOBDS(buyOrders, sellOrders, 10000)
	want := 14900.0 / 10000.0
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("CalcOBDS = %v, want %v", got, want)
	}
}

func TestCalcOBDS_ZeroCapitalOrEmpty(t *testing.T) {
	orders := []esi.MarketOrder{{Price: 100, VolumeRemain: 10}}
	if got := CalcOBDS(orders, nil, 1000); got != 0 {
		t.Errorf("CalcOBDS(nil sell) = %v, want 0", got)
	}
	if got := CalcOBDS(nil, orders, 1000); got != 0 {
		t.Errorf("CalcOBDS(nil buy) = %v, want 0", got)
	}
	if got := CalcOBDS(orders, orders, 0); got != 0 {
		t.Errorf("CalcOBDS(capital 0) = %v, want 0", got)
	}
}

// --- CalcSDS: scam score 0-100 (best buy < 50% VWAP +30, volume mismatch +25, single order dominance +25, no recent trades +20) ---

func TestCalcSDS_NoOrders(t *testing.T) {
	got := CalcSDS(nil, nil, nil, 100)
	if got != 100 {
		t.Errorf("CalcSDS(no buy orders) = %v, want 100 (suspicious)", got)
	}
}

func TestCalcSDS_BestBuyBelowHalfVWAP(t *testing.T) {
	// VWAP=100, best buy 40 -> 40 < 50% VWAP -> +30 (plus possible +15 dominance, +20 no recent trades if history nil)
	orders := []esi.MarketOrder{{Price: 40, VolumeRemain: 100}}
	got := CalcSDS(orders, nil, nil, 100)
	if got < 30 {
		t.Errorf("CalcSDS(best buy 40, VWAP 100) = %v, want >= 30", got)
	}
}

func TestCalcSDS_BestBuyAboveHalfVWAP_NoOtherTriggers(t *testing.T) {
	// Best buy 60 >= 50% VWAP; use recent history and two orders so no +15 dominance, no +20 no recent trades
	today := time.Now().Format("2006-01-02")
	history := []esi.HistoryEntry{{Date: today, Average: 100, Volume: 1000}}
	buyOrders := []esi.MarketOrder{{Price: 60, VolumeRemain: 50}, {Price: 58, VolumeRemain: 50}}
	sellOrders := []esi.MarketOrder{{Price: 70, VolumeRemain: 50}, {Price: 72, VolumeRemain: 50}}
	got := CalcSDS(buyOrders, sellOrders, history, 100)
	if got != 0 {
		t.Errorf("CalcSDS(best buy 60, recent history, two orders) = %v, want 0", got)
	}
}

func TestCalcSDS_ZeroVolumeHistoryStillCountsAsNoRecentTrades(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	history := []esi.HistoryEntry{
		{Date: today, Average: 100, Volume: 0},
	}
	orders := []esi.MarketOrder{
		{Price: 60, VolumeRemain: 50},
		{Price: 59, VolumeRemain: 50},
	}
	got := CalcSDS(orders, nil, history, 100)
	if got < 20 {
		t.Errorf("CalcSDS with zero-volume history = %v, want >= 20 (no recent trades trigger)", got)
	}
}

func TestCalcSDS_SellSideManipulation(t *testing.T) {
	// Sell orders at 300 ISK with VWAP 100 -> 300 > 200% VWAP -> +15
	buyOrders := []esi.MarketOrder{{Price: 60, VolumeRemain: 50}, {Price: 58, VolumeRemain: 50}}
	sellOrders := []esi.MarketOrder{{Price: 300, VolumeRemain: 100}}
	today := time.Now().Format("2006-01-02")
	history := []esi.HistoryEntry{{Date: today, Average: 100, Volume: 1000}}
	got := CalcSDS(buyOrders, sellOrders, history, 100)
	if got < 15 {
		t.Errorf("CalcSDS(sell at 3x VWAP) = %v, want >= 15 (sell-side deviation)", got)
	}
}

// --- normalize and CalcCTS (composite) ---

func TestNormalize(t *testing.T) {
	// normalize(value, minVal, maxVal) -> clamp (value-min)/(max-min) to [0,1]
	if got := normalize(50, 0, 100); math.Abs(got-0.5) > 1e-9 {
		t.Errorf("normalize(50,0,100) = %v, want 0.5", got)
	}
	if got := normalize(-10, 0, 100); got != 0 {
		t.Errorf("normalize(-10,0,100) = %v, want 0", got)
	}
	if got := normalize(150, 0, 100); got != 1 {
		t.Errorf("normalize(150,0,100) = %v, want 1", got)
	}
}

func TestCalcCTS_Bounds(t *testing.T) {
	// CTS is weighted sum of normalized components; result should be in reasonable range (not strictly 0-100 due to formula)
	got := CalcCTS(50, 1, 10, 5, 10, 50)
	if got < 0 || got > 100 {
		t.Errorf("CalcCTS should be in [0,100] range, got %v", got)
	}
}

func TestCalcCTS_MonotoneCoreFactors(t *testing.T) {
	base := CalcCTS(20, 0.5, 20, 20, 20, 100)
	higherROI := CalcCTS(60, 0.5, 20, 20, 20, 100)
	if higherROI <= base {
		t.Errorf("CTS should increase with spread ROI: base=%v higherROI=%v", base, higherROI)
	}

	lowerRisk := CalcCTS(20, 0.5, 20, 20, 10, 100)
	if lowerRisk <= base {
		t.Errorf("CTS should increase when SDS decreases: base=%v lowerRisk=%v", base, lowerRisk)
	}

	higherVolume := CalcCTS(20, 0.5, 20, 20, 20, 2000)
	if higherVolume <= base {
		t.Errorf("CTS should increase with daily volume: base=%v higherVolume=%v", base, higherVolume)
	}
}

func TestNormalizeCTSWeights(t *testing.T) {
	got := normalizeCTSWeights(CTSWeights{
		SpreadROI: 2,
		OBDS:      1,
		DRVI:      -1, // should clamp to 0
		CI:        1,
		SDS:       0,
		Volume:    0,
	})
	sum := got.SpreadROI + got.OBDS + got.DRVI + got.CI + got.SDS + got.Volume
	if math.Abs(sum-1) > 1e-12 {
		t.Fatalf("normalized weight sum = %v, want 1", sum)
	}
	if got.DRVI != 0 {
		t.Fatalf("DRVI weight should be clamped to 0, got %v", got.DRVI)
	}

	fallback := normalizeCTSWeights(CTSWeights{})
	if math.Abs(fallback.SpreadROI-DefaultCTSWeights.SpreadROI) > 1e-12 {
		t.Fatalf("zero weights should fallback to default, got %+v", fallback)
	}
}

func TestCalcCTSWithWeights_DefaultParity(t *testing.T) {
	cases := []struct {
		spreadROI float64
		obds      float64
		drvi      float64
		ci        int
		sds       int
		dailyVol  float64
	}{
		{20, 0.5, 20, 20, 20, 100},
		{80, 1.2, 10, 5, 5, 2000},
		{5, 0.1, 40, 80, 70, 20},
	}
	for _, tc := range cases {
		base := CalcCTS(tc.spreadROI, tc.obds, tc.drvi, tc.ci, tc.sds, tc.dailyVol)
		custom := CalcCTSWithWeights(tc.spreadROI, tc.obds, tc.drvi, tc.ci, tc.sds, tc.dailyVol, DefaultCTSWeights)
		if math.Abs(base-custom) > 1e-12 {
			t.Fatalf("default parity mismatch: base=%v custom=%v", base, custom)
		}
	}
}

func TestCalcCTSSensitivityHarness_MonotoneUnderWeightPerturbation(t *testing.T) {
	weights := []CTSWeights{
		DefaultCTSWeights,
		// ROI-heavy profile
		{SpreadROI: 0.40, OBDS: 0.10, DRVI: 0.10, CI: 0.10, SDS: 0.15, Volume: 0.15},
		// Risk-heavy profile
		{SpreadROI: 0.20, OBDS: 0.10, DRVI: 0.20, CI: 0.10, SDS: 0.30, Volume: 0.10},
		// Liquidity-heavy profile
		{SpreadROI: 0.20, OBDS: 0.20, DRVI: 0.10, CI: 0.10, SDS: 0.15, Volume: 0.25},
	}

	for i, w := range weights {
		base := CalcCTSWithWeights(20, 0.5, 20, 20, 20, 100, w)
		higherROI := CalcCTSWithWeights(60, 0.5, 20, 20, 20, 100, w)
		if higherROI <= base {
			t.Fatalf("profile %d: higher ROI should raise CTS (base=%v highROI=%v)", i, base, higherROI)
		}
		lowerSDS := CalcCTSWithWeights(20, 0.5, 20, 20, 10, 100, w)
		if lowerSDS <= base {
			t.Fatalf("profile %d: lower SDS should raise CTS (base=%v lowSDS=%v)", i, base, lowerSDS)
		}
		higherVolume := CalcCTSWithWeights(20, 0.5, 20, 20, 20, 2000, w)
		if higherVolume <= base {
			t.Fatalf("profile %d: higher volume should raise CTS (base=%v highVol=%v)", i, base, higherVolume)
		}
	}
}

func TestNormalizeCTSProfile(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", CTSProfileBalanced},
		{"balanced", CTSProfileBalanced},
		{"BALANCED", CTSProfileBalanced},
		{" aggressive ", CTSProfileAggressive},
		{"defensive", CTSProfileDefensive},
		{"unknown", CTSProfileBalanced},
	}
	for _, tc := range tests {
		got := normalizeCTSProfile(tc.in)
		if got != tc.want {
			t.Fatalf("normalizeCTSProfile(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCTSWeightsForProfile(t *testing.T) {
	if got := CTSWeightsForProfile(""); got != DefaultCTSWeights {
		t.Fatalf("empty profile should fallback to default: got %+v", got)
	}
	if got := CTSWeightsForProfile("balanced"); got != DefaultCTSWeights {
		t.Fatalf("balanced profile should equal default: got %+v", got)
	}

	aggr := CTSWeightsForProfile("aggressive")
	def := CTSWeightsForProfile("defensive")
	if aggr == DefaultCTSWeights {
		t.Fatalf("aggressive profile should differ from default")
	}
	if def == DefaultCTSWeights {
		t.Fatalf("defensive profile should differ from default")
	}
}

func TestCTSProfilesBias(t *testing.T) {
	aggrW := CTSWeightsForProfile("aggressive")
	defW := CTSWeightsForProfile("defensive")

	// High spread but risky/volatile item should rank better under aggressive profile.
	riskyAgg := CalcCTSWithWeights(120, 0.2, 35, 70, 70, 150, aggrW)
	riskyDef := CalcCTSWithWeights(120, 0.2, 35, 70, 70, 150, defW)
	if riskyAgg <= riskyDef {
		t.Fatalf("aggressive profile should prefer high-spread risky items: aggr=%v def=%v", riskyAgg, riskyDef)
	}

	// Liquid, low-risk item should rank better under defensive profile.
	stableAgg := CalcCTSWithWeights(40, 1.2, 8, 20, 10, 400, aggrW)
	stableDef := CalcCTSWithWeights(40, 1.2, 8, 20, 10, 400, defW)
	if stableDef <= stableAgg {
		t.Fatalf("defensive profile should prefer stable/liquid items: aggr=%v def=%v", stableAgg, stableDef)
	}
}

func TestAvgDailyVolume_UsesWindow(t *testing.T) {
	old := time.Now().AddDate(0, 0, -20).Format("2006-01-02")
	d1 := time.Now().AddDate(0, 0, -2).Format("2006-01-02")
	d2 := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	history := []esi.HistoryEntry{
		{Date: old, Volume: 10_000},
		{Date: d1, Volume: 100},
		{Date: d2, Volume: 300},
	}
	got := avgDailyVolume(history, 7)
	// Divides by window (7), not by number of entries (2):
	// total in-window = 100+300 = 400, avg = 400/7 ≈ 57.14
	want := (100.0 + 300.0) / 7.0
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("avgDailyVolume(7d) = %v, want %v", got, want)
	}
}

func TestFilterLastNDays_IncludesBoundaryDay(t *testing.T) {
	// An entry dated exactly N days ago must always be included,
	// regardless of time-of-day when the test runs. This verifies
	// the UTC truncation fix in filterLastNDays.
	exactlyNDaysAgo := time.Now().UTC().Truncate(24 * time.Hour).AddDate(0, 0, -7).Format("2006-01-02")
	history := []esi.HistoryEntry{
		{Date: exactlyNDaysAgo, Volume: 42},
	}
	entries := filterLastNDays(history, 7)
	if len(entries) != 1 {
		t.Fatalf("entry dated exactly 7 days ago should be included, got %d entries", len(entries))
	}
}

func TestAvgDailyVolume_ZeroDays(t *testing.T) {
	history := []esi.HistoryEntry{{Date: time.Now().Format("2006-01-02"), Volume: 100}}
	if got := avgDailyVolume(history, 0); got != 0 {
		t.Errorf("avgDailyVolume(0 days) = %v, want 0", got)
	}
}
