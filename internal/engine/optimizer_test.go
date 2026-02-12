package engine

import (
	"math"
	"testing"
	"time"

	"eve-flipper/internal/esi"
)

// --- Linear algebra helper tests ---

func TestDotProduct(t *testing.T) {
	a := []float64{1, 2, 3}
	b := []float64{4, 5, 6}
	got := dotProduct(a, b)
	want := 1*4.0 + 2*5.0 + 3*6.0 // 32
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("dotProduct = %v, want %v", got, want)
	}
}

func TestMatVecMul(t *testing.T) {
	// [[1,2],[3,4]] * [5,6] = [17, 39]
	m := [][]float64{{1, 2}, {3, 4}}
	v := []float64{5, 6}
	got := matVecMul(m, v)
	want := []float64{17, 39}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-9 {
			t.Errorf("matVecMul[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestIdentityMatrix(t *testing.T) {
	m := identityMatrix(3)
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			want := 0.0
			if i == j {
				want = 1.0
			}
			if m[i][j] != want {
				t.Errorf("identityMatrix[%d][%d] = %v, want %v", i, j, m[i][j], want)
			}
		}
	}
}

func TestInvertMatrix_2x2(t *testing.T) {
	// [[4,7],[2,6]] -> inv = 1/10 * [[6,-7],[-2,4]]
	m := [][]float64{{4, 7}, {2, 6}}
	inv := invertMatrix(m)
	if inv == nil {
		t.Fatal("expected non-nil inverse")
	}
	want := [][]float64{{0.6, -0.7}, {-0.2, 0.4}}
	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			if math.Abs(inv[i][j]-want[i][j]) > 1e-9 {
				t.Errorf("inv[%d][%d] = %v, want %v", i, j, inv[i][j], want[i][j])
			}
		}
	}
}

func TestInvertMatrix_Singular(t *testing.T) {
	// Singular matrix: rows are multiples.
	m := [][]float64{{1, 2}, {2, 4}}
	inv := invertMatrix(m)
	if inv != nil {
		t.Errorf("expected nil for singular matrix, got %v", inv)
	}
}

func TestInvertMatrix_Identity(t *testing.T) {
	m := identityMatrix(3)
	inv := invertMatrix(m)
	if inv == nil {
		t.Fatal("expected non-nil")
	}
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			want := 0.0
			if i == j {
				want = 1.0
			}
			if math.Abs(inv[i][j]-want) > 1e-9 {
				t.Errorf("inv[%d][%d] = %v, want %v", i, j, inv[i][j], want)
			}
		}
	}
}

func TestInvertMatrix_Empty(t *testing.T) {
	inv := invertMatrix(nil)
	if inv != nil {
		t.Errorf("expected nil for empty matrix")
	}
}

// --- projectOntoSimplex tests ---

func TestProjectOntoSimplex_AlreadyOnSimplex(t *testing.T) {
	w := []float64{0.3, 0.5, 0.2}
	projectOntoSimplex(w)
	sum := 0.0
	for _, v := range w {
		sum += v
		if v < 0 {
			t.Errorf("weight = %v, expected non-negative", v)
		}
	}
	if math.Abs(sum-1.0) > 1e-9 {
		t.Errorf("sum = %v, want 1", sum)
	}
	// Should be unchanged (already a valid simplex point).
	if math.Abs(w[0]-0.3) > 1e-9 || math.Abs(w[1]-0.5) > 1e-9 || math.Abs(w[2]-0.2) > 1e-9 {
		t.Errorf("already-on-simplex vector changed: got %v", w)
	}
}

func TestProjectOntoSimplex_WithNegatives(t *testing.T) {
	w := []float64{0.6, -0.1, 0.5}
	projectOntoSimplex(w)
	// The Euclidean projection of [0.6, -0.1, 0.5] onto the simplex:
	// Sort desc: [0.6, 0.5, -0.1], cumsum: 0.6, 1.1, 1.0
	// rho=1, theta=(1.1-1)/2=0.05
	// Result: [0.55, 0, 0.45]
	if w[1] != 0 {
		t.Errorf("negative component should project to 0, got %v", w[1])
	}
	sum := 0.0
	for _, v := range w {
		sum += v
	}
	if math.Abs(sum-1.0) > 1e-9 {
		t.Errorf("sum = %v, want 1 after projection", sum)
	}
	if math.Abs(w[0]-0.55) > 1e-9 {
		t.Errorf("w[0] = %v, want 0.55", w[0])
	}
	if math.Abs(w[2]-0.45) > 1e-9 {
		t.Errorf("w[2] = %v, want 0.45", w[2])
	}
}

func TestProjectOntoSimplex_AllNegative(t *testing.T) {
	// All negative: Euclidean projection distributes weight proportionally.
	// [-0.3, -0.5, -0.2] -> sort desc: [-0.2, -0.3, -0.5]
	// rho=2, theta=(-1.0-1)/3=-0.6667
	// Result: [0.3667, 0.1667, 0.4667]
	w := []float64{-0.3, -0.5, -0.2}
	projectOntoSimplex(w)
	sum := 0.0
	for _, v := range w {
		sum += v
		if v < -1e-12 {
			t.Errorf("weight = %v, expected non-negative", v)
		}
	}
	if math.Abs(sum-1.0) > 1e-9 {
		t.Errorf("sum = %v, want 1", sum)
	}
}

func TestSolveLongOnlyMinVar(t *testing.T) {
	// Simple 2-asset case: asset 1 has lower variance.
	cov := [][]float64{{1, 0.5}, {0.5, 4}}
	w := solveLongOnlyMinVar(cov)
	if w == nil {
		t.Fatal("expected non-nil weights")
	}
	sum := w[0] + w[1]
	if math.Abs(sum-1.0) > 1e-6 {
		t.Errorf("weights sum = %v, want 1", sum)
	}
	// Lower-variance asset should get higher weight.
	if w[0] < w[1] {
		t.Errorf("expected w[0] > w[1] since asset 0 has lower variance, got %v vs %v", w[0], w[1])
	}
	// Both weights should be non-negative.
	if w[0] < -1e-9 || w[1] < -1e-9 {
		t.Errorf("weights should be non-negative: %v", w)
	}
}

func TestSolveLongOnlyMaxSharpe(t *testing.T) {
	// Asset 0: low return, low variance. Asset 1: high return, higher variance.
	means := []float64{1, 5}
	cov := [][]float64{{1, 0}, {0, 4}}
	w := solveLongOnlyMaxSharpe(means, cov)
	if w == nil {
		t.Fatal("expected non-nil weights")
	}
	sum := w[0] + w[1]
	if math.Abs(sum-1.0) > 1e-6 {
		t.Errorf("weights sum = %v, want 1", sum)
	}
	// Higher Sharpe asset (asset 1: 5/2=2.5 vs asset 0: 1/1=1) should get more weight.
	if w[1] < w[0] {
		t.Errorf("expected w[1] > w[0] since asset 1 has better Sharpe, got %v vs %v", w[1], w[0])
	}
}

// --- Portfolio calculation tests ---

func TestPortfolioVariance(t *testing.T) {
	// Simple 2-asset portfolio:
	// cov = [[4, 1], [1, 9]], w = [0.5, 0.5]
	// var = 0.25*4 + 2*0.25*1 + 0.25*9 = 1 + 0.5 + 2.25 = 3.75
	cov := [][]float64{{4, 1}, {1, 9}}
	w := []float64{0.5, 0.5}
	got := portfolioVariance(w, cov)
	want := 3.75
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("portfolioVariance = %v, want %v", got, want)
	}
}

func TestPortfolioReturn(t *testing.T) {
	w := []float64{0.6, 0.4}
	mu := []float64{10, 5}
	got := portfolioReturn(w, mu)
	want := 0.6*10 + 0.4*5 // 8
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("portfolioReturn = %v, want %v", got, want)
	}
}

func TestPortfolioSharpe(t *testing.T) {
	// cov = [[1,0],[0,1]], w=[0.5,0.5], mu=[2,4]
	// return = 0.5*2 + 0.5*4 = 3
	// var = 0.25*1 + 0.25*1 = 0.5
	// vol = sqrt(0.5) ≈ 0.7071
	// Sharpe = (3 / 0.7071) * sqrt(365) ≈ 4.2426 * 19.105 = 81.05
	cov := [][]float64{{1, 0}, {0, 1}}
	w := []float64{0.5, 0.5}
	mu := []float64{2, 4}
	got := portfolioSharpe(w, mu, cov)
	ret := 3.0
	vol := math.Sqrt(0.5)
	want := (ret / vol) * math.Sqrt(365)
	if math.Abs(got-want) > 0.01 {
		t.Errorf("portfolioSharpe = %v, want %v", got, want)
	}
}

func TestPortfolioSharpe_ZeroVolatility(t *testing.T) {
	// All zeros in covariance -> vol = 0 -> Sharpe should be 0.
	cov := [][]float64{{0, 0}, {0, 0}}
	w := []float64{0.5, 0.5}
	mu := []float64{2, 4}
	got := portfolioSharpe(w, mu, cov)
	if got != 0 {
		t.Errorf("portfolioSharpe with zero vol = %v, want 0", got)
	}
}

// --- Ledoit-Wolf covariance test ---

func TestLedoitWolfCov_Properties(t *testing.T) {
	// Test that Ledoit-Wolf shrinkage produces a valid covariance matrix:
	// symmetric, positive diagonal, and shrinkage moves off-diagonal toward zero.
	n := 3
	T := 200
	returns := make([][]float64, n)
	for i := 0; i < n; i++ {
		returns[i] = make([]float64, T)
	}
	// Create 3 assets with comparable variance using simple deterministic patterns.
	for t := 0; t < T; t++ {
		returns[0][t] = float64(t%7) * 10        // 0,10,20,...,60,0,10,...
		returns[1][t] = float64((t+3)%11) * 8    // offset pattern
		returns[2][t] = float64((t*7+13)%13) * 6 // different pattern
	}

	means := make([]float64, n)
	for i := 0; i < n; i++ {
		sum := 0.0
		for t := 0; t < T; t++ {
			sum += returns[i][t]
		}
		means[i] = sum / float64(T)
	}

	cov := ledoitWolfCov(returns, means, T)
	if len(cov) != n {
		t.Fatalf("cov size = %d, want %d", len(cov), n)
	}

	// 1. Diagonal should be positive.
	for i := 0; i < n; i++ {
		if cov[i][i] <= 0 {
			t.Errorf("cov[%d][%d] = %v, want positive", i, i, cov[i][i])
		}
	}

	// 2. Matrix should be symmetric.
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if math.Abs(cov[i][j]-cov[j][i]) > 1e-12 {
				t.Errorf("cov not symmetric: [%d][%d]=%v vs [%d][%d]=%v",
					i, j, cov[i][j], j, i, cov[j][i])
			}
		}
	}

	// 3. Off-diagonal elements should be shrunk toward zero relative to
	//    the sample covariance (data-driven optimal α from LW 2004).
	sampleCov01 := 0.0
	for t := 0; t < T; t++ {
		sampleCov01 += (returns[0][t] - means[0]) * (returns[1][t] - means[1])
	}
	sampleCov01 /= float64(T - 1)
	// Shrunk off-diagonal magnitude should be ≤ sample (shrinkage toward zero).
	if math.Abs(cov[0][1]) > math.Abs(sampleCov01)+1e-6 {
		t.Errorf("shrunk |cov[0][1]| = %v > |sample| = %v, expected shrinkage toward zero",
			math.Abs(cov[0][1]), math.Abs(sampleCov01))
	}

	// 4. Matrix should be invertible (non-singular after shrinkage).
	inv := invertMatrix(cov)
	if inv == nil {
		t.Error("shrunk covariance matrix should be invertible")
	}
}

// --- Long-only efficient frontier test ---

func TestComputeLongOnlyFrontier(t *testing.T) {
	// Simple 2-asset case with known covariance.
	mu := []float64{2, 5}
	cov := [][]float64{{4, 0.5}, {0.5, 9}}

	frontier := computeLongOnlyFrontier(mu, cov, 10)
	if len(frontier) == 0 {
		t.Fatal("expected non-empty frontier")
	}

	// Frontier should be monotonically increasing in both risk and return.
	for i := 1; i < len(frontier); i++ {
		if frontier[i].Return < frontier[i-1].Return-1e-9 {
			t.Errorf("frontier return not increasing: [%d]=%v > [%d]=%v",
				i-1, frontier[i-1].Return, i, frontier[i].Return)
		}
		if frontier[i].Risk < frontier[i-1].Risk-1e-9 {
			t.Errorf("frontier risk not increasing: [%d]=%v > [%d]=%v",
				i-1, frontier[i-1].Risk, i, frontier[i].Risk)
		}
	}

	// All risk values should be non-negative.
	for i, pt := range frontier {
		if pt.Risk < 0 {
			t.Errorf("frontier[%d].Risk = %v, want >= 0", i, pt.Risk)
		}
	}
}

func TestComputeLongOnlyFrontier_Empty(t *testing.T) {
	// Empty means → should return nil.
	frontier := computeLongOnlyFrontier(nil, nil, 10)
	if frontier != nil {
		t.Errorf("expected nil frontier for empty inputs, got %d points", len(frontier))
	}
}

// --- generateSuggestions tests ---

func TestGenerateSuggestions_IncreaseDecrease(t *testing.T) {
	assets := []AssetStats{
		{TypeID: 1, TypeName: "Asset A", SharpeRatio: 2.0, Volatility: 100, AvgDailyPnL: 50},
		{TypeID: 2, TypeName: "Asset B", SharpeRatio: -0.5, Volatility: 200, AvgDailyPnL: -10},
		{TypeID: 3, TypeName: "Asset C", SharpeRatio: 0.5, Volatility: 50, AvgDailyPnL: 5},
	}
	current := []float64{0.2, 0.6, 0.2}
	optimal := []float64{0.5, 0.1, 0.4}

	suggestions := generateSuggestions(assets, current, optimal)
	if len(suggestions) != 3 {
		t.Fatalf("expected 3 suggestions, got %d", len(suggestions))
	}

	// Should be sorted: decreases first, then increases.
	foundDecreaseFirst := false
	for _, s := range suggestions {
		if s.Action == "decrease" {
			foundDecreaseFirst = true
			break
		}
		if s.Action == "increase" {
			break
		}
	}
	if !foundDecreaseFirst {
		t.Error("expected decrease suggestions before increase suggestions")
	}

	// Asset B has delta = (10-60) = -50% and negative sharpe -> reason = "negative_returns"
	for _, s := range suggestions {
		if s.TypeID == 2 {
			if s.Action != "decrease" {
				t.Errorf("Asset B action = %q, want decrease", s.Action)
			}
			if s.Reason != "negative_returns" {
				t.Errorf("Asset B reason = %q, want negative_returns", s.Reason)
			}
		}
		if s.TypeID == 1 {
			if s.Action != "increase" {
				t.Errorf("Asset A action = %q, want increase", s.Action)
			}
			if s.Reason != "high_sharpe" {
				t.Errorf("Asset A reason = %q, want high_sharpe", s.Reason)
			}
		}
	}
}

func TestGenerateSuggestions_Hold(t *testing.T) {
	assets := []AssetStats{
		{TypeID: 1, TypeName: "A", SharpeRatio: 1.0, Volatility: 100, AvgDailyPnL: 10},
	}
	current := []float64{0.5}
	optimal := []float64{0.52} // delta = 2% < 3% threshold -> hold

	suggestions := generateSuggestions(assets, current, optimal)
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
	}
	if suggestions[0].Action != "hold" {
		t.Errorf("action = %q, want hold for small delta", suggestions[0].Action)
	}
}

// --- Full ComputePortfolioOptimization integration tests ---

func TestComputePortfolioOptimization_Empty(t *testing.T) {
	got, diag := ComputePortfolioOptimization(nil, 90)
	if got != nil {
		t.Error("expected nil for empty transactions")
	}
	if diag == nil {
		t.Fatal("expected diagnostic for empty input")
	}
	if diag.MinDaysRequired != minOptimizerDays {
		t.Errorf("diag.MinDaysRequired = %d, want %d", diag.MinDaysRequired, minOptimizerDays)
	}
}

func TestComputePortfolioOptimization_TooFewItems(t *testing.T) {
	// Only 1 item -> need at least 2 for optimization.
	base := time.Now().UTC()
	var txns []esi.WalletTransaction
	for i := 0; i < 10; i++ {
		d := base.AddDate(0, 0, -i-1)
		txns = append(txns, esi.WalletTransaction{
			Date:      d.Format(time.RFC3339),
			TypeID:    34,
			TypeName:  "Tritanium",
			UnitPrice: 100,
			Quantity:  10,
			IsBuy:     (i%2 == 0),
		})
	}
	got, diag := ComputePortfolioOptimization(txns, 90)
	if got != nil {
		t.Error("expected nil for single item portfolio")
	}
	if diag == nil {
		t.Fatal("expected diagnostic")
	}
	if diag.UniqueItems != 1 {
		t.Errorf("diag.UniqueItems = %d, want 1", diag.UniqueItems)
	}
	if diag.QualifiedItems != 1 {
		t.Errorf("diag.QualifiedItems = %d, want 1 (single item qualifies but need 2)", diag.QualifiedItems)
	}
	if len(diag.TopItems) != 1 {
		t.Errorf("diag.TopItems length = %d, want 1", len(diag.TopItems))
	}
}

func TestComputePortfolioOptimization_TooFewDaysPerItem(t *testing.T) {
	// 2 items but each with only 2 trading days (< minOptimizerDays=3).
	base := time.Now().UTC()
	var txns []esi.WalletTransaction
	for i := 0; i < 2; i++ {
		d := base.AddDate(0, 0, -i-1)
		txns = append(txns,
			esi.WalletTransaction{
				Date: d.Format(time.RFC3339), TypeID: 34, TypeName: "Trit",
				UnitPrice: 100, Quantity: 10, IsBuy: true,
			},
			esi.WalletTransaction{
				Date: d.Format(time.RFC3339), TypeID: 35, TypeName: "Pyerite",
				UnitPrice: 50, Quantity: 20, IsBuy: true,
			},
		)
	}
	got, diag := ComputePortfolioOptimization(txns, 90)
	if got != nil {
		t.Error("expected nil when items have too few trading days")
	}
	if diag == nil {
		t.Fatal("expected diagnostic")
	}
	if diag.UniqueDays != 2 {
		t.Errorf("diag.UniqueDays = %d, want 2", diag.UniqueDays)
	}
	// All items have 2 days < 3 minimum, so 0 qualified.
	if diag.QualifiedItems != 0 {
		t.Errorf("diag.QualifiedItems = %d, want 0", diag.QualifiedItems)
	}
}

func TestComputePortfolioOptimization_BasicIntegration(t *testing.T) {
	// Create a portfolio with 3 items over 10 days, each with enough trading days.
	base := time.Now().UTC()
	var txns []esi.WalletTransaction
	for i := 0; i < 10; i++ {
		d := base.AddDate(0, 0, -i-1)
		ds := d.Format(time.RFC3339)

		// Item 1: Tritanium - buys and sells
		txns = append(txns, esi.WalletTransaction{
			Date: ds, TypeID: 34, TypeName: "Tritanium",
			UnitPrice: 100 + float64(i), Quantity: 10, IsBuy: (i%2 == 0),
		})
		// Item 2: Pyerite - buys and sells
		txns = append(txns, esi.WalletTransaction{
			Date: ds, TypeID: 35, TypeName: "Pyerite",
			UnitPrice: 50 + float64(i)*2, Quantity: 20, IsBuy: (i%2 == 1),
		})
		// Item 3: Mexallon - buys and sells
		txns = append(txns, esi.WalletTransaction{
			Date: ds, TypeID: 36, TypeName: "Mexallon",
			UnitPrice: 200 - float64(i)*5, Quantity: 5, IsBuy: (i%3 == 0),
		})
	}

	got, diag := ComputePortfolioOptimization(txns, 90)
	if got == nil {
		t.Fatalf("expected non-nil optimization result, diagnostic: %+v", diag)
	}

	// Should have 3 assets.
	if len(got.Assets) != 3 {
		t.Errorf("expected 3 assets, got %d", len(got.Assets))
	}

	// Correlation matrix should be n×n.
	n := len(got.Assets)
	if len(got.CorrelationMatrix) != n {
		t.Fatalf("correlation matrix rows = %d, want %d", len(got.CorrelationMatrix), n)
	}
	for i, row := range got.CorrelationMatrix {
		if len(row) != n {
			t.Errorf("correlation matrix row %d length = %d, want %d", i, len(row), n)
		}
	}

	// Diagonal of correlation matrix should be 1.
	for i := 0; i < n; i++ {
		if math.Abs(got.CorrelationMatrix[i][i]-1.0) > 0.01 {
			t.Errorf("corr[%d][%d] = %v, want 1", i, i, got.CorrelationMatrix[i][i])
		}
	}

	// Correlation values should be in [-1, 1].
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			v := got.CorrelationMatrix[i][j]
			if v < -1.01 || v > 1.01 {
				t.Errorf("corr[%d][%d] = %v, outside [-1,1]", i, j, v)
			}
		}
	}

	// Weights should sum to 1.
	sumCurrent := 0.0
	sumOptimal := 0.0
	sumMinVar := 0.0
	for i := 0; i < n; i++ {
		sumCurrent += got.CurrentWeights[i]
		sumOptimal += got.OptimalWeights[i]
		sumMinVar += got.MinVarWeights[i]
	}
	if math.Abs(sumCurrent-1.0) > 1e-6 {
		t.Errorf("current weights sum = %v, want 1", sumCurrent)
	}
	if math.Abs(sumOptimal-1.0) > 1e-6 {
		t.Errorf("optimal weights sum = %v, want 1", sumOptimal)
	}
	if math.Abs(sumMinVar-1.0) > 1e-6 {
		t.Errorf("min-var weights sum = %v, want 1", sumMinVar)
	}

	// All weights should be non-negative (long-only).
	for i := 0; i < n; i++ {
		if got.OptimalWeights[i] < -1e-9 {
			t.Errorf("optimal weight[%d] = %v, expected non-negative", i, got.OptimalWeights[i])
		}
		if got.MinVarWeights[i] < -1e-9 {
			t.Errorf("min-var weight[%d] = %v, expected non-negative", i, got.MinVarWeights[i])
		}
	}

	// HHI should be in [1/n, 1].
	if got.HHI < 1.0/float64(n)-1e-6 || got.HHI > 1.0+1e-6 {
		t.Errorf("HHI = %v, want in [%v, 1]", got.HHI, 1.0/float64(n))
	}

	// Efficient frontier should have points.
	if len(got.EfficientFrontier) == 0 {
		t.Error("expected non-empty efficient frontier")
	}

	// Suggestions should match the number of assets.
	if len(got.Suggestions) != n {
		t.Errorf("suggestions count = %d, want %d", len(got.Suggestions), n)
	}

	// Each suggestion should have a valid action.
	for _, s := range got.Suggestions {
		if s.Action != "increase" && s.Action != "decrease" && s.Action != "hold" {
			t.Errorf("invalid action %q for %s", s.Action, s.TypeName)
		}
	}
}

func TestComputePortfolioOptimization_WeightsAreByCapital(t *testing.T) {
	// Create 2 items where item 1 has 3x the capital of item 2.
	base := time.Now().UTC()
	var txns []esi.WalletTransaction
	for i := 0; i < 8; i++ {
		d := base.AddDate(0, 0, -i-1)
		ds := d.Format(time.RFC3339)
		// Tritanium: buy 300 ISK per day
		txns = append(txns, esi.WalletTransaction{
			Date: ds, TypeID: 34, TypeName: "Tritanium",
			UnitPrice: 300, Quantity: 1, IsBuy: true,
		})
		// Pyerite: buy 100 ISK per day
		txns = append(txns, esi.WalletTransaction{
			Date: ds, TypeID: 35, TypeName: "Pyerite",
			UnitPrice: 100, Quantity: 1, IsBuy: true,
		})
	}
	got, _ := ComputePortfolioOptimization(txns, 90)
	if got == nil {
		t.Fatal("expected non-nil")
	}
	if len(got.Assets) != 2 {
		t.Fatalf("expected 2 assets, got %d", len(got.Assets))
	}

	// Current weights should reflect capital ratio (3:1 = 0.75:0.25).
	var tritW, pyeW float64
	for i, a := range got.Assets {
		if a.TypeID == 34 {
			tritW = got.CurrentWeights[i]
		}
		if a.TypeID == 35 {
			pyeW = got.CurrentWeights[i]
		}
	}
	if math.Abs(tritW-0.75) > 1e-6 {
		t.Errorf("Tritanium current weight = %v, want 0.75", tritW)
	}
	if math.Abs(pyeW-0.25) > 1e-6 {
		t.Errorf("Pyerite current weight = %v, want 0.25", pyeW)
	}
}

func TestComputePortfolioOptimization_DiversificationRatio(t *testing.T) {
	// For a portfolio with uncorrelated assets, diversification ratio > 1.
	base := time.Now().UTC()
	var txns []esi.WalletTransaction
	for i := 0; i < 10; i++ {
		d := base.AddDate(0, 0, -i-1)
		ds := d.Format(time.RFC3339)
		// Alternate buy/sell to create varying daily PnL.
		txns = append(txns, esi.WalletTransaction{
			Date: ds, TypeID: 34, TypeName: "Tritanium",
			UnitPrice: 100 + float64(i%3)*50, Quantity: 10,
			IsBuy: (i%2 == 0),
		})
		txns = append(txns, esi.WalletTransaction{
			Date: ds, TypeID: 35, TypeName: "Pyerite",
			UnitPrice: 200 - float64(i%4)*30, Quantity: 5,
			IsBuy: (i%2 == 1),
		})
	}
	got, _ := ComputePortfolioOptimization(txns, 90)
	if got == nil {
		t.Fatal("expected non-nil")
	}

	// Diversification ratio should be non-negative.
	if got.DiversificationRatio < 0 {
		t.Errorf("DiversificationRatio = %v, want >= 0", got.DiversificationRatio)
	}
}

func TestComputePortfolioOptimization_LookbackFilter(t *testing.T) {
	// Transactions outside the lookback window should be ignored.
	base := time.Now().UTC()
	var txns []esi.WalletTransaction
	// Old transactions (200 days ago) - should be excluded for lookback=90.
	for i := 0; i < 10; i++ {
		d := base.AddDate(0, 0, -200-i)
		txns = append(txns,
			esi.WalletTransaction{
				Date: d.Format(time.RFC3339), TypeID: 34, TypeName: "Trit",
				UnitPrice: 1000000, Quantity: 100, IsBuy: true,
			},
			esi.WalletTransaction{
				Date: d.Format(time.RFC3339), TypeID: 35, TypeName: "Pye",
				UnitPrice: 500000, Quantity: 100, IsBuy: true,
			},
		)
	}
	// Recent transactions within lookback (only 2 days for each, less than minOptimizerDays=3).
	for i := 0; i < 2; i++ {
		d := base.AddDate(0, 0, -i-1)
		txns = append(txns,
			esi.WalletTransaction{
				Date: d.Format(time.RFC3339), TypeID: 34, TypeName: "Trit",
				UnitPrice: 100, Quantity: 10, IsBuy: true,
			},
			esi.WalletTransaction{
				Date: d.Format(time.RFC3339), TypeID: 35, TypeName: "Pye",
				UnitPrice: 50, Quantity: 20, IsBuy: true,
			},
		)
	}
	// With lookback=90, only recent 2 days are included per item (< minOptimizerDays=3) -> nil.
	got, diag := ComputePortfolioOptimization(txns, 90)
	if got != nil {
		t.Error("expected nil when items within lookback have too few trading days")
	}
	if diag == nil {
		t.Fatal("expected diagnostic")
	}
	// 20 old txns + 4 recent = 24 total, but only 4 within lookback.
	if diag.WithinLookback != 4 {
		t.Errorf("diag.WithinLookback = %d, want 4", diag.WithinLookback)
	}
	if diag.UniqueDays != 2 {
		t.Errorf("diag.UniqueDays = %d, want 2", diag.UniqueDays)
	}
}

func TestComputePortfolioOptimization_InvariantToNotionalScaling(t *testing.T) {
	base := time.Now().UTC()

	buildTxns := func(scaleType35 float64) []esi.WalletTransaction {
		cashflows34 := []float64{-100, 125, -90, 120, -95, 130}
		cashflows35 := []float64{-80, 95, -70, 90, -75, 100}
		txns := make([]esi.WalletTransaction, 0, 12)

		appendTxn := func(day time.Time, typeID int32, typeName string, cashflow float64) {
			amount := math.Abs(cashflow)
			isBuy := cashflow < 0
			txns = append(txns, esi.WalletTransaction{
				Date:      day.Format(time.RFC3339),
				TypeID:    typeID,
				TypeName:  typeName,
				UnitPrice: amount,
				Quantity:  1,
				IsBuy:     isBuy,
			})
		}

		for i := 0; i < len(cashflows34); i++ {
			day := base.AddDate(0, 0, -i-1)
			appendTxn(day, 34, "Tritanium", cashflows34[i])
			appendTxn(day, 35, "Pyerite", cashflows35[i]*scaleType35)
		}
		return txns
	}

	plain, diagPlain := ComputePortfolioOptimization(buildTxns(1), 90)
	scaled, diagScaled := ComputePortfolioOptimization(buildTxns(50), 90)
	if plain == nil {
		t.Fatalf("expected non-nil base optimization, diagnostic: %+v", diagPlain)
	}
	if scaled == nil {
		t.Fatalf("expected non-nil scaled optimization, diagnostic: %+v", diagScaled)
	}

	weightByType := func(out *PortfolioOptimization, typeID int32) float64 {
		for i, a := range out.Assets {
			if a.TypeID == typeID {
				return out.OptimalWeights[i]
			}
		}
		return -1
	}

	w34Plain := weightByType(plain, 34)
	w35Plain := weightByType(plain, 35)
	w34Scaled := weightByType(scaled, 34)
	w35Scaled := weightByType(scaled, 35)

	if w34Plain < 0 || w35Plain < 0 || w34Scaled < 0 || w35Scaled < 0 {
		t.Fatalf("failed to find expected assets in optimization output")
	}

	if math.Abs(w34Plain-w34Scaled) > 0.05 {
		t.Errorf("type 34 optimal weight changed after pure notional scaling: base=%v scaled=%v", w34Plain, w34Scaled)
	}
	if math.Abs(w35Plain-w35Scaled) > 0.05 {
		t.Errorf("type 35 optimal weight changed after pure notional scaling: base=%v scaled=%v", w35Plain, w35Scaled)
	}
}
