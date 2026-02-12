package engine

import (
	"math"
	"sort"
	"time"

	"eve-flipper/internal/esi"
)

// PortfolioOptimization is the response for the portfolio optimizer tab.
type PortfolioOptimization struct {
	Assets               []AssetStats           `json:"assets"`
	CorrelationMatrix    [][]float64            `json:"correlation_matrix"`
	CurrentWeights       []float64              `json:"current_weights"`
	OptimalWeights       []float64              `json:"optimal_weights"`
	MinVarWeights        []float64              `json:"min_var_weights"`
	EfficientFrontier    []FrontierPoint        `json:"efficient_frontier"`
	DiversificationRatio float64                `json:"diversification_ratio"`
	CurrentSharpe        float64                `json:"current_sharpe"`
	OptimalSharpe        float64                `json:"optimal_sharpe"`
	MinVarSharpe         float64                `json:"min_var_sharpe"`
	HHI                  float64                `json:"hhi"` // Herfindahl-Hirschman Index (0-1)
	Suggestions          []AllocationSuggestion `json:"suggestions"`
}

// OptimizerDiagnostic is returned when optimization fails to help users understand why.
type OptimizerDiagnostic struct {
	TotalTransactions int              `json:"total_transactions"` // how many txns were in the input
	WithinLookback    int              `json:"within_lookback"`    // how many passed the date filter
	UniqueDays        int              `json:"unique_days"`        // how many distinct calendar days
	UniqueItems       int              `json:"unique_items"`       // how many distinct items
	QualifiedItems    int              `json:"qualified_items"`    // items with >= minOptimizerDays
	MinDaysRequired   int              `json:"min_days_required"`  // current threshold
	TopItems          []DiagnosticItem `json:"top_items"`          // top items by trading days
}

// DiagnosticItem shows a single item's stats for the diagnostic view.
type DiagnosticItem struct {
	TypeID       int32  `json:"type_id"`
	TypeName     string `json:"type_name"`
	TradingDays  int    `json:"trading_days"`
	Transactions int    `json:"transactions"`
}

// AssetStats describes a single tradeable item in the portfolio.
type AssetStats struct {
	TypeID        int32   `json:"type_id"`
	TypeName      string  `json:"type_name"`
	AvgDailyPnL   float64 `json:"avg_daily_pnl"`  // mean daily P&L in ISK
	Volatility    float64 `json:"volatility"`     // daily std dev of P&L
	SharpeRatio   float64 `json:"sharpe_ratio"`   // annualized
	CurrentWeight float64 `json:"current_weight"` // fraction of total capital
	TotalInvested float64 `json:"total_invested"`
	TotalPnL      float64 `json:"total_pnl"`
	TradingDays   int     `json:"trading_days"`
}

// FrontierPoint is a point on the efficient frontier.
type FrontierPoint struct {
	Risk   float64 `json:"risk"`   // portfolio std dev (daily)
	Return float64 `json:"return"` // portfolio expected daily return
}

// AllocationSuggestion recommends increasing or decreasing allocation to an item.
type AllocationSuggestion struct {
	TypeID     int32   `json:"type_id"`
	TypeName   string  `json:"type_name"`
	Action     string  `json:"action"` // "increase", "decrease", "hold"
	CurrentPct float64 `json:"current_pct"`
	OptimalPct float64 `json:"optimal_pct"`
	DeltaPct   float64 `json:"delta_pct"` // optimal - current
	Reason     string  `json:"reason"`
}

const (
	// minOptimizerDays is the minimum number of days an item must have traded
	// to be included in the optimization. Lowered to 3 because the ESI wallet
	// transactions endpoint only returns a single page (~1000 recent entries)
	// which may span just a few calendar days for active traders.
	minOptimizerDays = 3
	// maxOptimizerAssets limits the optimization to the top N items by capital.
	maxOptimizerAssets = 20
	// frontierPoints is how many points to sample on the efficient frontier.
	frontierPoints = 30
)

// ComputePortfolioOptimization runs Markowitz mean-variance optimization
// on the player's trading portfolio derived from wallet transactions.
// Returns (result, nil) on success, or (nil, diagnostic) when there isn't enough data.
func ComputePortfolioOptimization(txns []esi.WalletTransaction, lookbackDays int) (*PortfolioOptimization, *OptimizerDiagnostic) {
	if len(txns) == 0 {
		return nil, &OptimizerDiagnostic{
			MinDaysRequired: minOptimizerDays,
		}
	}

	now := time.Now().UTC()
	cutoff := now.AddDate(0, 0, -lookbackDays)

	// Phase 1: Build per-item daily P&L series.
	// Key = typeID, value = map[dayString]pnl.
	type itemDayPnL struct {
		pnlByDay     map[string]float64
		totalBought  float64
		totalSold    float64
		typeName     string
		transactions int
	}
	items := make(map[int32]*itemDayPnL)
	allDays := make(map[string]bool)
	withinLookback := 0

	for _, tx := range txns {
		t, err := time.Parse(time.RFC3339, tx.Date)
		if err != nil || t.Before(cutoff) {
			continue
		}
		withinLookback++
		day := t.Format("2006-01-02")
		allDays[day] = true

		item, ok := items[tx.TypeID]
		if !ok {
			item = &itemDayPnL{
				pnlByDay: make(map[string]float64),
				typeName: tx.TypeName,
			}
			items[tx.TypeID] = item
		}

		item.transactions++
		amount := tx.UnitPrice * float64(tx.Quantity)
		if tx.IsBuy {
			item.pnlByDay[day] -= amount
			item.totalBought += amount
		} else {
			item.pnlByDay[day] += amount
			item.totalSold += amount
		}
	}

	// Sort all days.
	sortedDays := make([]string, 0, len(allDays))
	for d := range allDays {
		sortedDays = append(sortedDays, d)
	}
	sort.Strings(sortedDays)

	// Phase 2: Filter to items with enough trading days and select top N by capital.
	type assetCandidate struct {
		typeID       int32
		typeName     string
		totalBought  float64
		tradingDays  int
		dailyPnL     []float64
		dailyReturns []float64
	}
	var candidates []assetCandidate

	for typeID, item := range items {
		tradingDays := len(item.pnlByDay)
		if tradingDays < minOptimizerDays {
			continue
		}

		// Normalize daily cashflows by item capital scale so covariance and weights
		// are not dominated by absolute ISK size differences between items.
		capitalScale := math.Max(item.totalBought, item.totalSold)
		if capitalScale <= 0 {
			continue
		}

		// Build aligned daily series (using all days, 0 for days with no activity).
		dailyPnL := make([]float64, len(sortedDays))
		returns := make([]float64, len(sortedDays))
		for i, day := range sortedDays {
			if pnl, ok := item.pnlByDay[day]; ok {
				dailyPnL[i] = pnl
				returns[i] = pnl / capitalScale
			}
		}

		candidates = append(candidates, assetCandidate{
			typeID:       typeID,
			typeName:     item.typeName,
			totalBought:  item.totalBought,
			tradingDays:  tradingDays,
			dailyPnL:     dailyPnL,
			dailyReturns: returns,
		})
	}

	if len(candidates) < 2 {
		// Build diagnostic: show top items by trading days so the user understands what's happening.
		diag := &OptimizerDiagnostic{
			TotalTransactions: len(txns),
			WithinLookback:    withinLookback,
			UniqueDays:        len(allDays),
			UniqueItems:       len(items),
			QualifiedItems:    len(candidates),
			MinDaysRequired:   minOptimizerDays,
		}
		// Collect all items sorted by trading days (desc).
		type itemInfo struct {
			typeID       int32
			typeName     string
			tradingDays  int
			transactions int
		}
		var allItems []itemInfo
		for tid, item := range items {
			allItems = append(allItems, itemInfo{
				typeID:       tid,
				typeName:     item.typeName,
				tradingDays:  len(item.pnlByDay),
				transactions: item.transactions,
			})
		}
		sort.Slice(allItems, func(i, j int) bool {
			return allItems[i].tradingDays > allItems[j].tradingDays
		})
		limit := 10
		if len(allItems) < limit {
			limit = len(allItems)
		}
		for _, it := range allItems[:limit] {
			diag.TopItems = append(diag.TopItems, DiagnosticItem{
				TypeID:       it.typeID,
				TypeName:     it.typeName,
				TradingDays:  it.tradingDays,
				Transactions: it.transactions,
			})
		}
		return nil, diag
	}

	// Sort by total invested (descending) and take top N.
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].totalBought > candidates[j].totalBought
	})
	if len(candidates) > maxOptimizerAssets {
		candidates = candidates[:maxOptimizerAssets]
	}

	n := len(candidates)
	T := len(sortedDays)

	// Phase 3: Compute mean returns and covariance matrix.
	means := make([]float64, n)
	for i, c := range candidates {
		means[i] = mean(c.dailyReturns)
	}

	// Build returns matrix: n assets x T days.
	returnsMatrix := make([][]float64, n)
	for i, c := range candidates {
		returnsMatrix[i] = c.dailyReturns
	}

	// Covariance matrix with Ledoit-Wolf shrinkage for stability.
	covMatrix := ledoitWolfCov(returnsMatrix, means, T)

	// Correlation matrix for display.
	corrMatrix := make([][]float64, n)
	for i := 0; i < n; i++ {
		corrMatrix[i] = make([]float64, n)
		for j := 0; j < n; j++ {
			si := math.Sqrt(covMatrix[i][i])
			sj := math.Sqrt(covMatrix[j][j])
			if si > 0 && sj > 0 {
				corrMatrix[i][j] = covMatrix[i][j] / (si * sj)
			} else {
				corrMatrix[i][j] = 0
			}
			// Clamp to [-1, 1].
			if corrMatrix[i][j] > 1 {
				corrMatrix[i][j] = 1
			}
			if corrMatrix[i][j] < -1 {
				corrMatrix[i][j] = -1
			}
		}
	}

	// Phase 4: Current weights (by capital invested).
	totalCapital := 0.0
	for _, c := range candidates {
		totalCapital += c.totalBought
	}
	currentWeights := make([]float64, n)
	for i, c := range candidates {
		if totalCapital > 0 {
			currentWeights[i] = c.totalBought / totalCapital
		}
	}

	// Phase 5: Long-only optimization via projected gradient descent.
	// Properly solves the constrained QP: min w'Σw s.t. w >= 0, 1'w = 1
	// instead of the naive approach of solving unconstrained and clamping negatives.
	// The naive clamp-and-renormalize is not a valid QP projection and produces
	// suboptimal weights that may not lie on the efficient frontier.
	minVarWeights := solveLongOnlyMinVar(covMatrix)
	optimalWeights := solveLongOnlyMaxSharpe(means, covMatrix)

	// Phase 6: Compute portfolio metrics for each allocation.
	currentSharpe := portfolioSharpe(currentWeights, means, covMatrix)
	optimalSharpe := portfolioSharpe(optimalWeights, means, covMatrix)
	minVarSharpe := portfolioSharpe(minVarWeights, means, covMatrix)

	// Diversification ratio: weighted avg vol / portfolio vol.
	divRatio := 0.0
	portVar := portfolioVariance(currentWeights, covMatrix)
	if portVar > 0 {
		weightedAvgVol := 0.0
		for i := 0; i < n; i++ {
			weightedAvgVol += currentWeights[i] * math.Sqrt(covMatrix[i][i])
		}
		divRatio = weightedAvgVol / math.Sqrt(portVar)
	}

	// HHI (Herfindahl-Hirschman Index): sum of squared weights. 1/n = perfectly diversified.
	hhi := 0.0
	for _, w := range currentWeights {
		hhi += w * w
	}

	// Scale return-space risk/return metrics to ISK/day using current deployed capital.
	// This keeps frontend charts in familiar units while optimization itself remains
	// based on normalized returns.
	referenceCapital := totalCapital
	if referenceCapital <= 0 {
		referenceCapital = 1
	}

	// Phase 7: Long-only efficient frontier.
	// Computed by solving the constrained QP at each target return level,
	// ensuring all plotted points are achievable with long-only portfolios.
	frontier := computeLongOnlyFrontier(means, covMatrix, frontierPoints)
	for i := range frontier {
		frontier[i].Risk *= referenceCapital
		frontier[i].Return *= referenceCapital
	}

	// Phase 8: Build asset stats.
	assetStats := make([]AssetStats, n)
	for i, c := range candidates {
		volRet := math.Sqrt(variance(c.dailyReturns))
		sr := 0.0
		if volRet > 0 {
			sr = (means[i] / volRet) * math.Sqrt(365)
		}
		avgDailyPnL := means[i] * referenceCapital
		vol := volRet * referenceCapital
		totalPnL := 0.0
		for _, pnl := range c.dailyPnL {
			totalPnL += pnl
		}
		assetStats[i] = AssetStats{
			TypeID:        c.typeID,
			TypeName:      c.typeName,
			AvgDailyPnL:   avgDailyPnL,
			Volatility:    vol,
			SharpeRatio:   sr,
			CurrentWeight: currentWeights[i],
			TotalInvested: c.totalBought,
			TotalPnL:      totalPnL,
			TradingDays:   c.tradingDays,
		}
	}

	// Phase 9: Generate suggestions.
	suggestions := generateSuggestions(assetStats, currentWeights, optimalWeights)

	return &PortfolioOptimization{
		Assets:               assetStats,
		CorrelationMatrix:    corrMatrix,
		CurrentWeights:       currentWeights,
		OptimalWeights:       optimalWeights,
		MinVarWeights:        minVarWeights,
		EfficientFrontier:    frontier,
		DiversificationRatio: divRatio,
		CurrentSharpe:        currentSharpe,
		OptimalSharpe:        optimalSharpe,
		MinVarSharpe:         minVarSharpe,
		HHI:                  hhi,
		Suggestions:          suggestions,
	}, nil
}

// --- Linear algebra utilities for small matrices ---

// ledoitWolfCov computes a shrinkage covariance matrix using the oracle
// approximating shrinkage estimator from Ledoit & Wolf (2004).
// Target: μ̄·I (scaled identity with average variance on the diagonal).
// Shrinkage intensity: α* = min(β̂²/δ̂², 1), the data-driven optimal.
//
// Reference: O. Ledoit, M. Wolf, "A well-conditioned estimator for
// large-dimensional covariance matrices", J. Multivariate Analysis (2004).
func ledoitWolfCov(returns [][]float64, means []float64, T int) [][]float64 {
	n := len(returns)

	// Step 1: Sample covariance matrix S (unbiased, Bessel's correction).
	sample := make([][]float64, n)
	for i := 0; i < n; i++ {
		sample[i] = make([]float64, n)
		for j := 0; j <= i; j++ {
			cov := 0.0
			for t := 0; t < T; t++ {
				cov += (returns[i][t] - means[i]) * (returns[j][t] - means[j])
			}
			if T > 1 {
				cov /= float64(T - 1)
			}
			sample[i][j] = cov
			sample[j][i] = cov
		}
	}

	// Step 2: Shrinkage target F = μ̄·I (average variance on diagonal).
	avgVar := 0.0
	for i := 0; i < n; i++ {
		avgVar += sample[i][i]
	}
	avgVar /= float64(n)

	// Step 3: Compute optimal shrinkage intensity (Ledoit-Wolf 2004).
	// δ² = ||S − F||²_F  (squared Frobenius distance from sample to target)
	dSq := 0.0
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			targetIJ := 0.0
			if i == j {
				targetIJ = avgVar
			}
			diff := sample[i][j] - targetIJ
			dSq += diff * diff
		}
	}

	// β̂² = (1/T²) Σ_k ||z_k z_k' − S||²_F
	// where z_k = centered observation vector for period k.
	// This estimates the total squared estimation error of S.
	bSq := 0.0
	for k := 0; k < T; k++ {
		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				diff := (returns[i][k]-means[i])*(returns[j][k]-means[j]) - sample[i][j]
				bSq += diff * diff
			}
		}
	}
	bSq /= float64(T) * float64(T)

	// α* = min(β̂²/δ², 1)
	alpha := 0.0
	if dSq > 1e-15 {
		alpha = bSq / dSq
	}
	if alpha > 1 {
		alpha = 1
	}

	// Step 4: Shrunk covariance Σ̂ = (1−α)·S + α·F
	shrunk := make([][]float64, n)
	for i := 0; i < n; i++ {
		shrunk[i] = make([]float64, n)
		for j := 0; j < n; j++ {
			shrunk[i][j] = (1 - alpha) * sample[i][j]
			if i == j {
				shrunk[i][j] += alpha * avgVar
			}
		}
	}

	return shrunk
}

// invertMatrix inverts a square matrix using Gauss-Jordan elimination.
// Returns nil if the matrix is singular.
func invertMatrix(m [][]float64) [][]float64 {
	n := len(m)
	if n == 0 {
		return nil
	}

	// Augmented matrix [M | I].
	aug := make([][]float64, n)
	for i := 0; i < n; i++ {
		aug[i] = make([]float64, 2*n)
		for j := 0; j < n; j++ {
			aug[i][j] = m[i][j]
		}
		aug[i][n+i] = 1
	}

	// Forward elimination with partial pivoting.
	for col := 0; col < n; col++ {
		// Find pivot.
		maxVal := math.Abs(aug[col][col])
		maxRow := col
		for row := col + 1; row < n; row++ {
			if math.Abs(aug[row][col]) > maxVal {
				maxVal = math.Abs(aug[row][col])
				maxRow = row
			}
		}
		if maxVal < 1e-12 {
			return nil // singular
		}
		// Swap rows.
		aug[col], aug[maxRow] = aug[maxRow], aug[col]

		// Scale pivot row.
		scale := aug[col][col]
		for j := 0; j < 2*n; j++ {
			aug[col][j] /= scale
		}

		// Eliminate column.
		for row := 0; row < n; row++ {
			if row == col {
				continue
			}
			factor := aug[row][col]
			for j := 0; j < 2*n; j++ {
				aug[row][j] -= factor * aug[col][j]
			}
		}
	}

	// Extract inverse.
	inv := make([][]float64, n)
	for i := 0; i < n; i++ {
		inv[i] = make([]float64, n)
		for j := 0; j < n; j++ {
			inv[i][j] = aug[i][n+j]
		}
	}
	return inv
}

func identityMatrix(n int) [][]float64 {
	m := make([][]float64, n)
	for i := 0; i < n; i++ {
		m[i] = make([]float64, n)
		m[i][i] = 1
	}
	return m
}

func matVecMul(m [][]float64, v []float64) []float64 {
	n := len(m)
	result := make([]float64, n)
	for i := 0; i < n; i++ {
		for j := 0; j < len(v); j++ {
			result[i] += m[i][j] * v[j]
		}
	}
	return result
}

func dotProduct(a, b []float64) float64 {
	sum := 0.0
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}

// projectOntoSimplex projects vector v onto the probability simplex
// Δ = {x ∈ ℝⁿ : x ≥ 0, Σxᵢ = 1} using the exact O(n log n) algorithm
// from Duchi et al. (2008), "Efficient projections onto the l1-ball".
// Modifies v in place.
func projectOntoSimplex(v []float64) {
	n := len(v)
	if n == 0 {
		return
	}

	// Sort a copy in descending order.
	u := make([]float64, n)
	copy(u, v)
	sort.Float64s(u)
	// Reverse to descending.
	for i, j := 0, n-1; i < j; i, j = i+1, j-1 {
		u[i], u[j] = u[j], u[i]
	}

	// Find ρ: largest index j (1-based) such that u[j] - (Σ_{i=1..j} u[i] - 1)/j > 0.
	cumSum := 0.0
	rho := 0
	for j := 0; j < n; j++ {
		cumSum += u[j]
		if u[j]-(cumSum-1)/float64(j+1) > 0 {
			rho = j
		}
	}

	// Threshold θ.
	cumSum = 0
	for j := 0; j <= rho; j++ {
		cumSum += u[j]
	}
	theta := (cumSum - 1) / float64(rho+1)

	// Project.
	for i := range v {
		v[i] -= theta
		if v[i] < 0 {
			v[i] = 0
		}
	}
}

// solveLongOnlyMinVar finds the minimum-variance portfolio with long-only constraints
// by solving: min w'Σw  s.t. w ≥ 0, 1'w = 1
// using projected gradient descent onto the probability simplex.
// For n ≤ 20 (maxOptimizerAssets), this converges in well under 1 ms.
func solveLongOnlyMinVar(cov [][]float64) []float64 {
	n := len(cov)
	if n == 0 {
		return nil
	}

	// Initial weights: equal (feasible point on simplex).
	w := make([]float64, n)
	for i := range w {
		w[i] = 1.0 / float64(n)
	}

	// Step size: 1 / (2·trace(Σ)). Conservative upper bound since
	// the Lipschitz constant of ∇(w'Σw) = 2Σw is L = 2·λ_max(Σ) ≤ 2·trace(Σ).
	trace := 0.0
	for i := 0; i < n; i++ {
		trace += cov[i][i]
	}
	if trace <= 0 {
		return w
	}
	stepSize := 1.0 / (2 * trace)

	const maxIter = 1000
	const tol = 1e-10

	for iter := 0; iter < maxIter; iter++ {
		// Gradient of w'Σw is 2·Σ·w.
		grad := matVecMul(cov, w)

		prevW := make([]float64, n)
		copy(prevW, w)
		for i := range w {
			w[i] -= stepSize * 2 * grad[i]
		}

		// Project onto probability simplex.
		projectOntoSimplex(w)

		// Check convergence: max|w_new - w_old|.
		maxDiff := 0.0
		for i := range w {
			d := math.Abs(w[i] - prevW[i])
			if d > maxDiff {
				maxDiff = d
			}
		}
		if maxDiff < tol {
			break
		}
	}

	return w
}

// solveLongOnlyMaxSharpe finds the maximum Sharpe ratio (tangency) portfolio
// with long-only constraints. Scans the risk-aversion parameter λ ≥ 0 and
// for each solves: min w'Σw − λ·μ'w  s.t. w ≥ 0, 1'w = 1, then picks the
// solution with the highest Sharpe ratio. This is a robust approach for small n.
func solveLongOnlyMaxSharpe(means []float64, cov [][]float64) []float64 {
	n := len(means)
	if n == 0 {
		return nil
	}

	bestSharpe := -math.MaxFloat64
	var bestW []float64

	// Scan λ from 0 (min-variance) to large (max-return emphasis).
	// Logarithmic spacing gives good coverage of the efficient frontier.
	const numScans = 50
	for k := 0; k <= numScans; k++ {
		var lambda float64
		if k == 0 {
			lambda = 0
		} else {
			t := float64(k) / float64(numScans)
			lambda = 0.001 * math.Pow(100000, t) // 0.001 to 100
		}

		w := solveLongOnlyQP(means, cov, lambda)
		sr := portfolioSharpe(w, means, cov)
		if sr > bestSharpe {
			bestSharpe = sr
			bestW = w
		}
	}

	if bestW == nil {
		bestW = make([]float64, n)
		for i := range bestW {
			bestW[i] = 1.0 / float64(n)
		}
	}

	return bestW
}

// solveLongOnlyQP solves: min w'Σw − λ·μ'w  s.t. w ≥ 0, 1'w = 1
// via projected gradient descent onto the simplex. The parameter λ controls
// the tradeoff between variance minimization and return maximization.
func solveLongOnlyQP(means []float64, cov [][]float64, lambda float64) []float64 {
	n := len(cov)
	if n == 0 {
		return nil
	}

	w := make([]float64, n)
	for i := range w {
		w[i] = 1.0 / float64(n)
	}

	// Lipschitz constant of ∇f = 2Σw − λμ is 2·λ_max(Σ) ≤ 2·trace(Σ).
	// The linear term −λμ has zero Hessian, so the Lipschitz constant is unchanged.
	trace := 0.0
	for i := 0; i < n; i++ {
		trace += cov[i][i]
	}
	if trace <= 0 {
		return w
	}
	stepSize := 1.0 / (2 * trace)

	const maxIter = 1000
	const tol = 1e-10

	for iter := 0; iter < maxIter; iter++ {
		grad := matVecMul(cov, w)

		prevW := make([]float64, n)
		copy(prevW, w)
		for i := range w {
			w[i] -= stepSize * (2*grad[i] - lambda*means[i])
		}

		projectOntoSimplex(w)

		maxDiff := 0.0
		for i := range w {
			d := math.Abs(w[i] - prevW[i])
			if d > maxDiff {
				maxDiff = d
			}
		}
		if maxDiff < tol {
			break
		}
	}

	return w
}

func portfolioVariance(w []float64, cov [][]float64) float64 {
	n := len(w)
	v := 0.0
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			v += w[i] * w[j] * cov[i][j]
		}
	}
	return v
}

func portfolioReturn(w, means []float64) float64 {
	r := 0.0
	for i := range w {
		r += w[i] * means[i]
	}
	return r
}

func portfolioSharpe(w, means []float64, cov [][]float64) float64 {
	ret := portfolioReturn(w, means)
	vol := math.Sqrt(portfolioVariance(w, cov))
	if vol <= 0 {
		return 0
	}
	return (ret / vol) * math.Sqrt(365)
}

// computeLongOnlyFrontier traces the efficient frontier for long-only portfolios
// by scanning the risk-aversion parameter λ and solving the constrained QP
// at each level. This ensures every plotted point is achievable without shorting.
func computeLongOnlyFrontier(means []float64, cov [][]float64, numPoints int) []FrontierPoint {
	if len(means) == 0 || numPoints < 2 {
		return nil
	}

	// Scan λ from 0 (min-variance) to large (max-return).
	// Use logarithmic spacing for good coverage across the frontier.
	type rawPoint struct {
		risk, ret float64
	}
	var raw []rawPoint

	for k := 0; k < numPoints*2; k++ {
		var lambda float64
		if k == 0 {
			lambda = 0
		} else {
			t := float64(k) / float64(numPoints*2-1)
			lambda = 0.001 * math.Pow(1000000, t) // 0.001 to 1000
		}

		w := solveLongOnlyQP(means, cov, lambda)
		risk := math.Sqrt(portfolioVariance(w, cov))
		ret := portfolioReturn(w, means)
		raw = append(raw, rawPoint{risk: risk, ret: ret})
	}

	// Sort by risk ascending.
	sort.Slice(raw, func(i, j int) bool {
		return raw[i].risk < raw[j].risk
	})

	// Remove dominated points: keep only those with monotonically increasing return.
	var clean []rawPoint
	maxRet := -math.MaxFloat64
	for _, p := range raw {
		if p.ret > maxRet {
			clean = append(clean, p)
			maxRet = p.ret
		}
	}

	// Deduplicate points that are too close (within 0.1% of risk range).
	if len(clean) == 0 {
		return nil
	}
	riskRange := clean[len(clean)-1].risk - clean[0].risk
	minGap := riskRange * 0.001
	if minGap < 1e-12 {
		minGap = 1e-12
	}

	frontier := []FrontierPoint{{Risk: clean[0].risk, Return: clean[0].ret}}
	for _, p := range clean[1:] {
		last := frontier[len(frontier)-1]
		if p.risk-last.Risk >= minGap {
			frontier = append(frontier, FrontierPoint{Risk: p.risk, Return: p.ret})
		}
	}

	// Downsample to requested number of points if we have too many.
	if len(frontier) > numPoints {
		sampled := make([]FrontierPoint, numPoints)
		for i := 0; i < numPoints; i++ {
			idx := i * (len(frontier) - 1) / (numPoints - 1)
			sampled[i] = frontier[idx]
		}
		frontier = sampled
	}

	return frontier
}

func generateSuggestions(assets []AssetStats, current, optimal []float64) []AllocationSuggestion {
	var suggestions []AllocationSuggestion
	for i, a := range assets {
		curPct := current[i] * 100
		optPct := optimal[i] * 100
		delta := optPct - curPct

		action := "hold"
		reason := ""

		if delta > 3 {
			action = "increase"
			if a.SharpeRatio > 1 {
				reason = "high_sharpe"
			} else {
				reason = "diversification"
			}
		} else if delta < -3 {
			action = "decrease"
			if a.SharpeRatio < 0 {
				reason = "negative_returns"
			} else if a.Volatility > 0 && a.AvgDailyPnL/a.Volatility < 0.1 {
				reason = "poor_risk_adjusted"
			} else {
				reason = "overweight"
			}
		}

		suggestions = append(suggestions, AllocationSuggestion{
			TypeID:     a.TypeID,
			TypeName:   a.TypeName,
			Action:     action,
			CurrentPct: curPct,
			OptimalPct: optPct,
			DeltaPct:   delta,
			Reason:     reason,
		})
	}

	// Sort: decreases first, then increases.
	sort.Slice(suggestions, func(i, j int) bool {
		if suggestions[i].Action != suggestions[j].Action {
			// decrease < hold < increase (show decreases first)
			order := map[string]int{"decrease": 0, "increase": 1, "hold": 2}
			return order[suggestions[i].Action] < order[suggestions[j].Action]
		}
		return math.Abs(suggestions[i].DeltaPct) > math.Abs(suggestions[j].DeltaPct)
	})

	return suggestions
}
