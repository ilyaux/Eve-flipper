package engine

import (
	"math"
	"testing"
	"time"

	"eve-flipper/internal/esi"
)

// --- Pure math helpers: exact expected values ---

func TestMean(t *testing.T) {
	tests := []struct {
		name string
		x    []float64
		want float64
	}{
		{"empty", nil, 0},
		{"single", []float64{42}, 42},
		{"five", []float64{1, 2, 3, 4, 5}, 3},
		{"negative", []float64{-10, -20, -30}, -20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mean(tt.x)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("mean(%v) = %v, want %v", tt.x, got, tt.want)
			}
		})
	}
}

func TestMinFloat64(t *testing.T) {
	tests := []struct {
		name string
		x    []float64
		want float64
	}{
		{"empty", nil, 0},
		{"single", []float64{7}, 7},
		{"positive", []float64{3, 1, 2}, 1},
		{"negative", []float64{-100, -50, -200}, -200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := minFloat64(tt.x)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("minFloat64(%v) = %v, want %v", tt.x, got, tt.want)
			}
		})
	}
}

func TestRobustScale(t *testing.T) {
	tests := []struct {
		name string
		x    []float64
		want float64
	}{
		{"empty", nil, 0},
		{"single", []float64{10}, 10},
		{"odd median of abs", []float64{1, 2, 3, 4, 5}, 3},       // abs 1,2,3,4,5 -> median 3
		{"even median of abs", []float64{1, 2, 3, 4, 5, 6}, 3.5}, // abs 1..6 -> (3+4)/2
		{"negative values", []float64{-5, -4, -3, -2, -1}, 3},    // abs 1..5 -> median 3
		{"mixed", []float64{-100, 50, 80, -20, 40}, 50},          // abs 20,40,50,80,100 -> median 50
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := robustScale(tt.x)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("robustScale(%v) = %v, want %v", tt.x, got, tt.want)
			}
		})
	}
}

func TestPortfolioVarEs(t *testing.T) {
	// n=10 (< 20): uses Cornish-Fisher expansion to adjust for skewness/kurtosis.
	// pnls = {-100, -90, ..., -10}: symmetric, so skew=0, exKurt ≈ -1.2 (platykurtic).
	// CF correction adjusts the normal quantile to account for the light tails.
	pnls := []float64{-100, -90, -80, -70, -60, -50, -40, -30, -20, -10}
	mu := -55.0
	sigma := math.Sqrt(variance(pnls))
	skew := sampleSkewness(pnls)
	exKurt := sampleExcessKurtosis(pnls)

	// Verify that skewness is ~0 for symmetric data.
	if math.Abs(skew) > 1e-9 {
		t.Errorf("skewness of symmetric data = %v, want ~0", skew)
	}

	// Cornish-Fisher adjusted quantiles.
	cf95 := cornishFisherQuantile(-1.6449, skew, exKurt)
	cf99 := cornishFisherQuantile(-2.3263, skew, exKurt)

	wantVar95 := mu + cf95*sigma
	wantVar99 := mu + cf99*sigma
	wantES95 := mu - sigma*normalPDF(cf95)/0.05
	wantES99 := mu - sigma*normalPDF(cf99)/0.01

	var95, var99, es95, es99 := portfolioVarEs(pnls)

	if math.Abs(var95-wantVar95) > 0.01 {
		t.Errorf("var95 = %v, want %v", var95, wantVar95)
	}
	if math.Abs(var99-wantVar99) > 0.01 {
		t.Errorf("var99 = %v, want %v", var99, wantVar99)
	}
	if math.Abs(es95-wantES95) > 0.01 {
		t.Errorf("es95 = %v, want %v", es95, wantES95)
	}
	if math.Abs(es99-wantES99) > 0.01 {
		t.Errorf("es99 = %v, want %v", es99, wantES99)
	}

	// Structural checks: VaR95 should be less extreme than VaR99.
	if var95 < var99 {
		t.Errorf("VaR95 = %v should be > VaR99 = %v (less extreme)", var95, var99)
	}
	// ES should be more extreme than corresponding VaR.
	if es95 > var95 {
		t.Errorf("ES95 = %v should be <= VaR95 = %v", es95, var95)
	}

	// n=20 (>= 20): still uses empirical quantiles (unchanged).
	// idx95 = floor(0.05*20)=1, idx99=0. So var95 = sorted[1], var99 = sorted[0].
	// ES95 = mean(sorted[:2]), ES99 = mean(sorted[:1]).
	pnls2 := make([]float64, 20)
	for i := range pnls2 {
		pnls2[i] = -100 + float64(i)*5 // -100,-95,-90,...,-5
	}
	var95, var99, es95, es99 = portfolioVarEs(pnls2)
	if math.Abs(var95-(-95)) > 1e-9 {
		t.Errorf("var95 (n=20) = %v, want -95", var95)
	}
	if math.Abs(var99-(-100)) > 1e-9 {
		t.Errorf("var99 (n=20) = %v, want -100", var99)
	}
	wantEmpES95 := (-100.0 + -95.0) / 2.0
	if math.Abs(es95-wantEmpES95) > 1e-9 {
		t.Errorf("es95 (n=20) = %v, want %v", es95, wantEmpES95)
	}
	if math.Abs(es99-(-100)) > 1e-9 {
		t.Errorf("es99 (n=20) = %v, want -100", es99)
	}
}

func TestCornishFisherQuantile_Normal(t *testing.T) {
	// With zero skewness and zero excess kurtosis, CF should return the original z.
	z := -1.6449
	cf := cornishFisherQuantile(z, 0, 0)
	if math.Abs(cf-z) > 1e-12 {
		t.Errorf("CF(z, 0, 0) = %v, want %v", cf, z)
	}
}

func TestCornishFisherQuantile_PositiveSkew(t *testing.T) {
	// Positive skewness should push the left-tail quantile further left.
	z := -1.6449
	cf := cornishFisherQuantile(z, 1.0, 0)
	// (z²-1)*skew/6 = (1.705-1)*1/6 ≈ +0.117 → shifts quantile right (less extreme)
	// Wait, with positive skewness the left tail becomes thinner (right tail fatter),
	// so VaR should be less extreme. The CF-adjusted quantile should be > z.
	if cf <= z {
		t.Errorf("CF with positive skew should be > z, got %v <= %v", cf, z)
	}
}

func TestSampleSkewness_Symmetric(t *testing.T) {
	// Symmetric data should have skewness ≈ 0.
	x := []float64{-2, -1, 0, 1, 2}
	skew := sampleSkewness(x)
	if math.Abs(skew) > 1e-9 {
		t.Errorf("sampleSkewness of symmetric data = %v, want 0", skew)
	}
}

func TestSampleExcessKurtosis_Uniform(t *testing.T) {
	// For a uniform-like distribution, excess kurtosis should be negative (platykurtic).
	x := make([]float64, 100)
	for i := range x {
		x[i] = float64(i)
	}
	ek := sampleExcessKurtosis(x)
	if ek >= 0 {
		t.Errorf("excess kurtosis of uniform-like data = %v, want negative", ek)
	}
}

func TestPortfolioVarEs_EmptyAndSmall(t *testing.T) {
	var95, var99, es95, es99 := portfolioVarEs(nil)
	if var95 != 0 || var99 != 0 || es95 != 0 || es99 != 0 {
		t.Errorf("portfolioVarEs(nil) should return zeros, got var95=%v var99=%v es95=%v es99=%v", var95, var99, es95, es99)
	}

	// Single element: idx95=0, idx99=0, cut95=1, cut99=1
	var95, var99, es95, es99 = portfolioVarEs([]float64{-50})
	if math.Abs(var95-(-50)) > 1e-9 || math.Abs(es95-(-50)) > 1e-9 {
		t.Errorf("portfolioVarEs([-50]) = var95 %v es95 %v, want -50", var95, es95)
	}
}

func TestComputePortfolioRiskFromTransactions_EmptyAndTooFewDays(t *testing.T) {
	if got := ComputePortfolioRiskFromTransactions(nil); got != nil {
		t.Errorf("ComputePortfolioRiskFromTransactions(nil) want nil, got %+v", got)
	}

	// One day only -> less than minRiskSampleDays (5)
	now := time.Now().UTC()
	txns := []esi.WalletTransaction{
		{Date: now.AddDate(0, 0, -1).Format(time.RFC3339), UnitPrice: 100, Quantity: 1, IsBuy: false},
	}
	if got := ComputePortfolioRiskFromTransactions(txns); got != nil {
		t.Errorf("ComputePortfolioRiskFromTransactions(1 day) want nil, got %+v", got)
	}
}

func TestComputePortfolioRiskFromTransactions_FIFOMatching(t *testing.T) {
	// Test FIFO matching: buy 10 units at 100 ISK, then sell 10 units at 150 ISK the next day.
	// Expected daily realized P&L:
	//   Day 1 (buy): no realized PnL (only inventory)
	//   Day 2 (sell): (150 - 100) * 10 = 500 ISK profit
	// We need at least 5 distinct days for the function to return non-nil,
	// so we repeat this pattern across 10 days.
	base := time.Now().UTC().AddDate(0, 0, -20)
	var txns []esi.WalletTransaction

	// 5 buy-sell cycles spread across 10 separate days.
	buyPrices := []float64{100, 200, 150, 80, 120}
	sellPrices := []float64{150, 250, 180, 100, 160}

	for i := 0; i < 5; i++ {
		buyDay := base.AddDate(0, 0, i*2)
		sellDay := base.AddDate(0, 0, i*2+1)
		txns = append(txns, esi.WalletTransaction{
			Date:      buyDay.Format(time.RFC3339),
			UnitPrice: buyPrices[i],
			Quantity:  10,
			IsBuy:     true,
			TypeID:    34, // Tritanium
		})
		txns = append(txns, esi.WalletTransaction{
			Date:      sellDay.Format(time.RFC3339),
			UnitPrice: sellPrices[i],
			Quantity:  10,
			IsBuy:     false,
			TypeID:    34,
		})
	}

	out := ComputePortfolioRiskFromTransactions(txns)
	if out == nil {
		t.Fatal("ComputePortfolioRiskFromTransactions: expected non-nil with 5 FIFO cycles")
	}

	// We should have exactly 5 sell days with realized PnL (buy days have no realized PnL).
	// Realized PnLs per sell day: (150-100)*10=500, (250-200)*10=500, (180-150)*10=300, (100-80)*10=200, (160-120)*10=400
	// Total realized = 500+500+300+200+400 = 1900
	if out.SampleDays != 5 {
		t.Errorf("SampleDays = %d, want 5", out.SampleDays)
	}

	// All realized PnLs are positive, so worst day loss should be 0 (least profitable day reported as positive).
	// Smallest PnL is 200, so worst loss = -min(pnls) = -200, but 200 > 0, so WorstDayLoss = -200 which is negative.
	// Actually: min(pnls) = 200 (positive), so -min(pnls) = -200 which means a gain, not a loss.
	// WorstDayLoss here represents the worst daily performance, which is 200 ISK (still a gain).
	// Since all days are profitable, WorstDayLoss = -min(200, 300, 400, 500, 500) = -200.
	// That's correct — the "worst day" was still profitable at 200 ISK.

	// Verify basic invariants.
	if out.RiskScore < 0 || out.RiskScore > 100 {
		t.Errorf("RiskScore = %v, want in [0,100]", out.RiskScore)
	}
	if out.RiskLevel != "safe" && out.RiskLevel != "balanced" && out.RiskLevel != "high" {
		t.Errorf("RiskLevel = %q, want safe|balanced|high", out.RiskLevel)
	}
	if out.WindowDays != portfolioLookbackDays {
		t.Errorf("WindowDays = %d, want %d", out.WindowDays, portfolioLookbackDays)
	}
	// With only 5 days, Var99Reliable should be false.
	if out.Var99Reliable {
		t.Errorf("Var99Reliable = true, want false for SampleDays=%d (<%d)", out.SampleDays, minVaR99Days)
	}
}

func TestComputePortfolioRiskFromTransactions_Var99ReliableFlag(t *testing.T) {
	// Build 35 days of transactions within lookback (180 days) to test Var99Reliable = true.
	// Each day: buy 1 unit at 100 ISK, sell 1 unit at random markup.
	base := time.Now().UTC().AddDate(0, 0, -70)
	var txns []esi.WalletTransaction

	for i := 0; i < 35; i++ {
		buyDay := base.AddDate(0, 0, i*2)
		sellDay := base.AddDate(0, 0, i*2+1)
		buyPrice := 100.0
		sellPrice := 100.0 + float64(i%7)*10 // 100..160
		txns = append(txns, esi.WalletTransaction{
			Date:      buyDay.Format(time.RFC3339),
			UnitPrice: buyPrice,
			Quantity:  1,
			IsBuy:     true,
			TypeID:    35,
		})
		txns = append(txns, esi.WalletTransaction{
			Date:      sellDay.Format(time.RFC3339),
			UnitPrice: sellPrice,
			Quantity:  1,
			IsBuy:     false,
			TypeID:    35,
		})
	}

	out := ComputePortfolioRiskFromTransactions(txns)
	if out == nil {
		t.Fatal("ComputePortfolioRiskFromTransactions: expected non-nil with 35 days")
	}
	if !out.Var99Reliable {
		t.Errorf("Var99Reliable = false, want true for SampleDays=%d (>=%d)", out.SampleDays, minVaR99Days)
	}
}

func TestComputePortfolioRiskFromTransactions_UnmatchedSells(t *testing.T) {
	// Sells without matching buys have unknown cost basis and must not be treated as profit.
	base := time.Now().UTC().AddDate(0, 0, -10)
	var txns []esi.WalletTransaction

	// 6 sell-only days at different prices (no buys in lookback).
	for i := 0; i < 6; i++ {
		d := base.AddDate(0, 0, i)
		txns = append(txns, esi.WalletTransaction{
			Date:      d.Format(time.RFC3339),
			UnitPrice: 100 + float64(i)*50,
			Quantity:  1,
			IsBuy:     false,
			TypeID:    36,
		})
	}

	out := ComputePortfolioRiskFromTransactions(txns)
	if out != nil {
		t.Fatalf("ComputePortfolioRiskFromTransactions: expected nil when only unmatched sells are present, got %+v", out)
	}
}
