package engine

import (
	"math"
	"sort"
	"time"

	"eve-flipper/internal/esi"
)

// PortfolioRiskSummary is a user-friendly snapshot of portfolio risk for a character.
// All fields are in ISK or simple scores; no financial jargon is exposed to the UI.
type PortfolioRiskSummary struct {
	// RiskScore is 0-100 (higher = more risk).
	RiskScore float64 `json:"risk_score"`
	// RiskLevel is a coarse label: "safe", "balanced", "high".
	RiskLevel string `json:"risk_level"`

	// One-day worst loss levels (historical, 95/99%).
	Var95 float64 `json:"var_95"`
	Var99 float64 `json:"var_99"`
	// Expected shortfall (average of worst X% days).
	ES95 float64 `json:"es_95"`
	ES99 float64 `json:"es_99"`

	// TypicalDailyPnl is a robust scale of recent daily PnL.
	TypicalDailyPnl float64 `json:"typical_daily_pnl"`
	// WorstDayLoss is the worst historical daily loss in ISK.
	WorstDayLoss float64 `json:"worst_day_loss"`

	// SampleDays is the number of distinct days used.
	SampleDays int `json:"sample_days"`
	// WindowDays is the lookback window in calendar days.
	WindowDays int `json:"window_days"`

	// CapacityMultiplier is a rough estimate of how much more
	// notional the player could hold at similar risk (e.g. 1.8x).
	CapacityMultiplier float64 `json:"capacity_multiplier"`

	// LowSample is true when VaR/ES are unreliable due to small sample size (<20 days).
	LowSample bool `json:"low_sample"`
	// Var99Reliable is false when N < 30 (too few data points for 1% quantile).
	Var99Reliable bool `json:"var_99_reliable"`
}

const (
	// portfolioLookbackDays defines how far back we look for wallet transactions.
	portfolioLookbackDays = 180
	// minRiskSampleDays is the minimal number of distinct days required.
	// Keep this low so that new characters still see a rough estimate.
	minRiskSampleDays = 5
	// minVaR99Days is the minimum number of days for VaR99/ES99 to be meaningful.
	minVaR99Days = 30
)

// ComputePortfolioRiskFromTransactions builds a daily realized P&L series from wallet
// transactions using FIFO matching (buys matched to sells per item type) and estimates
// risk metrics. Unmatched buys (inventory accumulation) and unmatched sells
// (unknown cost basis) are excluded from P&L to avoid treating cashflow as
// realized trading performance.
func ComputePortfolioRiskFromTransactions(txns []esi.WalletTransaction) *PortfolioRiskSummary {
	if len(txns) == 0 {
		return nil
	}

	now := time.Now().UTC()
	cutoff := now.AddDate(0, 0, -portfolioLookbackDays)

	// Phase 1: FIFO matching per item type.
	// For each type, maintain a queue of buy lots. When a sell occurs,
	// match against oldest buy and compute realized P&L.
	type buyLot struct {
		unitPrice float64
		remaining int32
	}
	buyQueues := make(map[int32][]buyLot) // typeID -> FIFO queue
	dailyPnL := make(map[time.Time]float64)

	// Sort transactions chronologically for correct FIFO ordering.
	sorted := make([]esi.WalletTransaction, len(txns))
	copy(sorted, txns)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Date < sorted[j].Date
	})

	for _, tx := range sorted {
		t, err := time.Parse(time.RFC3339, tx.Date)
		if err != nil {
			continue
		}
		if t.Before(cutoff) {
			// Still process for FIFO queue building but don't record PnL.
			if tx.IsBuy {
				buyQueues[tx.TypeID] = append(buyQueues[tx.TypeID], buyLot{
					unitPrice: tx.UnitPrice,
					remaining: tx.Quantity,
				})
			}
			continue
		}

		day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)

		if tx.IsBuy {
			// Add to FIFO queue.
			buyQueues[tx.TypeID] = append(buyQueues[tx.TypeID], buyLot{
				unitPrice: tx.UnitPrice,
				remaining: tx.Quantity,
			})
		} else {
			// Sell: match against FIFO buy queue.
			sellQty := tx.Quantity
			sellPrice := tx.UnitPrice
			queue := buyQueues[tx.TypeID]

			for sellQty > 0 && len(queue) > 0 {
				lot := &queue[0]
				matched := lot.remaining
				if matched > sellQty {
					matched = sellQty
				}
				// Realized P&L for this match: (sell price - buy price) × matched quantity
				pnl := (sellPrice - lot.unitPrice) * float64(matched)
				dailyPnL[day] += pnl

				lot.remaining -= matched
				sellQty -= matched
				if lot.remaining <= 0 {
					queue = queue[1:]
				}
			}
			buyQueues[tx.TypeID] = queue

			// Do not count any remaining quantity because the cost basis is unknown.
		}
	}

	if len(dailyPnL) < minRiskSampleDays {
		// Not enough data for a meaningful estimate.
		return nil
	}

	// Turn map into sorted slice.
	type dayPnl struct {
		day time.Time
		pnl float64
	}
	var series []dayPnl
	for d, pnl := range dailyPnL {
		series = append(series, dayPnl{day: d, pnl: pnl})
	}
	sort.Slice(series, func(i, j int) bool {
		return series[i].day.Before(series[j].day)
	})

	// Extract PnL values.
	pnls := make([]float64, len(series))
	for i, dp := range series {
		pnls[i] = dp.pnl
	}

	typical := robustScale(pnls)
	if typical <= 0 {
		return nil
	}

	// Convert to "returns" relative to typical daily scale.
	returns := make([]float64, len(pnls))
	for i, v := range pnls {
		returns[i] = v / typical
	}

	n := len(pnls)

	// Compute empirical VaR/ES on the PnL distribution directly.
	var95, var99, es95, es99 := portfolioVarEs(pnls)
	worstLoss := minFloat64(pnls)

	// Volatility: use EWMA (λ=0.94, RiskMetrics convention) to weight recent
	// observations more heavily. This makes the risk score responsive to regime
	// changes (war declarations, patch days) rather than averaging over 180 days.
	ewmaStd := ewmaVolatility(returns, 0.94)

	// Risk score: map EWMA std to 0-100 with soft cap.
	// For EVE daily flows, std ~1 is "balanced", ~2+ is "high".
	score := ewmaStd * 40 // std=1 -> 40, std=2 -> 80
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	level := "balanced"
	switch {
	case score < 30:
		level = "safe"
	case score > 70:
		level = "high"
	}

	// Capacity multiplier: use bias-corrected Sharpe ratio.
	// For small N, sample Sharpe has standard error ≈ √(1/N + SR²/2N),
	// so we apply a conservative discount factor of √((N-3)/N) (Opdyke 2007).
	meanRet := mean(returns)
	capacity := 1.0
	if score < 70 && ewmaStd > 0 {
		sharpeLike := meanRet / ewmaStd
		// Bias correction: shrink Sharpe toward zero for small samples.
		if n > 3 {
			sharpeLike *= math.Sqrt(float64(n-3) / float64(n))
		}
		switch {
		case sharpeLike > 1.0:
			capacity = 2.0
		case sharpeLike > 0.5:
			capacity = 1.5
		default:
			capacity = 1.2
		}
	}

	var99Reliable := n >= minVaR99Days

	return &PortfolioRiskSummary{
		RiskScore:          score,
		RiskLevel:          level,
		Var95:              -var95, // report as positive loss
		Var99:              -var99,
		ES95:               -es95,
		ES99:               -es99,
		TypicalDailyPnl:    typical,
		WorstDayLoss:       -worstLoss,
		SampleDays:         n,
		WindowDays:         portfolioLookbackDays,
		CapacityMultiplier: capacity,
		LowSample:          n < 20,
		Var99Reliable:      var99Reliable,
	}
}

func robustScale(x []float64) float64 {
	if len(x) == 0 {
		return 0
	}
	absVals := make([]float64, 0, len(x))
	for _, v := range x {
		absVals = append(absVals, math.Abs(v))
	}
	sort.Float64s(absVals)
	// Use median of |PnL| as typical daily scale.
	n := len(absVals)
	if n%2 == 1 {
		return absVals[n/2]
	}
	return 0.5 * (absVals[n/2-1] + absVals[n/2])
}

func portfolioVarEs(pnls []float64) (var95, var99, es95, es99 float64) {
	if len(pnls) == 0 {
		return
	}

	n := len(pnls)

	// For small samples (N < 20), empirical quantiles degenerate:
	// floor(0.05*10)=0, so VaR95=VaR99=ES95=ES99=worst day.
	// Use Cornish-Fisher expansion to account for heavy tails (skewness,
	// kurtosis) which dominate in small EVE trading samples. This is a
	// 4th-order correction to the normal quantile that captures asymmetry
	// and fat tails without requiring a full distributional fit.
	if n < 20 {
		mu := mean(pnls)
		sigma := math.Sqrt(variance(pnls))
		if sigma <= 0 {
			// All PnLs identical — no risk spread.
			var95 = mu
			var99 = mu
			es95 = mu
			es99 = mu
			return
		}
		skew := sampleSkewness(pnls)
		exKurt := sampleExcessKurtosis(pnls)

		// Cornish-Fisher adjusted quantiles (left-tail).
		const (
			z95 = -1.6449 // Φ⁻¹(0.05) — left tail
			z99 = -2.3263 // Φ⁻¹(0.01) — left tail
		)
		cf95 := cornishFisherQuantile(z95, skew, exKurt)
		cf99 := cornishFisherQuantile(z99, skew, exKurt)

		// VaR_α = μ + cf_α × σ
		var95 = mu + cf95*sigma
		var99 = mu + cf99*sigma

		// ES_α (Cornish-Fisher) = μ − σ × φ(cf_α) / α
		// This extends the normal ES formula with the CF-adjusted quantile.
		es95 = mu - sigma*normalPDF(cf95)/0.05
		es99 = mu - sigma*normalPDF(cf99)/0.01
		return
	}

	// For sufficient samples, use empirical (historical simulation) quantiles.
	sorted := make([]float64, n)
	copy(sorted, pnls)
	sort.Float64s(sorted) // ascending: biggest loss first (most negative)

	idx95 := int(math.Floor(0.05 * float64(n)))
	idx99 := int(math.Floor(0.01 * float64(n)))
	if idx95 < 0 {
		idx95 = 0
	}
	if idx95 >= n {
		idx95 = n - 1
	}
	if idx99 < 0 {
		idx99 = 0
	}
	if idx99 >= n {
		idx99 = n - 1
	}

	var95 = sorted[idx95]
	var99 = sorted[idx99]

	// ES: average of worst X% days (most negative values).
	cut95 := idx95 + 1
	if cut95 < 1 {
		cut95 = 1
	}
	es95 = mean(sorted[:cut95])

	cut99 := idx99 + 1
	if cut99 < 1 {
		cut99 = 1
	}
	es99 = mean(sorted[:cut99])
	return
}

func minFloat64(x []float64) float64 {
	if len(x) == 0 {
		return 0
	}
	m := x[0]
	for _, v := range x[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

func mean(x []float64) float64 {
	if len(x) == 0 {
		return 0
	}
	var sum float64
	for _, v := range x {
		sum += v
	}
	return sum / float64(len(x))
}

// ewmaVolatility computes exponentially weighted moving average volatility.
// λ (lambda) is the decay factor: 0.94 is the RiskMetrics convention.
// Recent observations receive exponentially more weight, making the estimate
// responsive to regime changes (e.g., war declarations, patch-day spikes).
// Formula: σ²_t = λ × σ²_{t-1} + (1-λ) × (r_t - μ)²
func ewmaVolatility(returns []float64, lambda float64) float64 {
	n := len(returns)
	if n < 2 {
		return 0
	}
	mu := mean(returns)

	// Initialize with sample variance instead of a single squared deviation.
	// Single-observation init introduces significant noise for small N because
	// the EWMA has no time to "warm up". Sample variance gives a stable
	// unbiased starting point that the recursive update then adapts.
	sampleVar := 0.0
	for _, r := range returns {
		d := r - mu
		sampleVar += d * d
	}
	sampleVar /= float64(n)

	ewmaVar := sampleVar
	for i := 0; i < n; i++ {
		dev := returns[i] - mu
		ewmaVar = lambda*ewmaVar + (1-lambda)*dev*dev
	}
	return math.Sqrt(ewmaVar)
}

// sampleSkewness computes the adjusted Fisher-Pearson standardized moment
// coefficient (G1). Returns 0 for n < 3.
func sampleSkewness(x []float64) float64 {
	n := len(x)
	if n < 3 {
		return 0
	}
	mu := mean(x)
	s := math.Sqrt(variance(x))
	if s <= 0 {
		return 0
	}

	m3 := 0.0
	for _, v := range x {
		d := (v - mu) / s
		m3 += d * d * d
	}
	// Adjusted sample skewness: n / ((n-1)(n-2)) * Σ(zᵢ³)
	return float64(n) / (float64(n-1) * float64(n-2)) * m3
}

// sampleExcessKurtosis computes the adjusted excess kurtosis (G2).
// Returns 0 for n < 4.
func sampleExcessKurtosis(x []float64) float64 {
	n := len(x)
	if n < 4 {
		return 0
	}
	mu := mean(x)
	s := math.Sqrt(variance(x))
	if s <= 0 {
		return 0
	}

	m4 := 0.0
	for _, v := range x {
		d := (v - mu) / s
		m4 += d * d * d * d
	}
	n1 := float64(n)
	// Adjusted excess kurtosis formula.
	return (n1*(n1+1)/((n1-1)*(n1-2)*(n1-3)))*m4 - 3*(n1-1)*(n1-1)/((n1-2)*(n1-3))
}

// cornishFisherQuantile adjusts a normal quantile z for skewness and excess
// kurtosis using the Cornish-Fisher expansion (4th-order).
//
//	z_cf = z + (z²−1)·γ₁/6 + (z³−3z)·γ₂/24 − (2z³−5z)·γ₁²/36
//
// where γ₁ = skewness, γ₂ = excess kurtosis.
func cornishFisherQuantile(z, skew, excessKurt float64) float64 {
	z2 := z * z
	z3 := z2 * z
	cf := z +
		(z2-1)*skew/6 +
		(z3-3*z)*excessKurt/24 -
		(2*z3-5*z)*skew*skew/36
	return cf
}

// normalPDF returns the standard normal probability density function φ(x).
func normalPDF(x float64) float64 {
	return math.Exp(-0.5*x*x) / math.Sqrt(2*math.Pi)
}
