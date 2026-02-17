package engine

import (
	"math"
	"sort"
	"time"

	"eve-flipper/internal/esi"
)

// filterLastNDays returns history entries from the last N days.
func filterLastNDays(history []esi.HistoryEntry, days int) []esi.HistoryEntry {
	if len(history) == 0 || days <= 0 {
		return nil
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	var filtered []esi.HistoryEntry
	for _, h := range history {
		t, err := time.Parse("2006-01-02", h.Date)
		if err != nil {
			continue
		}
		if t.After(cutoff) || t.Equal(cutoff) {
			filtered = append(filtered, h)
		}
	}
	return filtered
}

// CalcVWAP calculates Volume-Weighted Average Price over N days.
func CalcVWAP(history []esi.HistoryEntry, days int) float64 {
	entries := filterLastNDays(history, days)
	if len(entries) == 0 {
		return 0
	}

	var sumPriceVol, sumVol float64
	for _, h := range entries {
		sumPriceVol += h.Average * float64(h.Volume)
		sumVol += float64(h.Volume)
	}
	if sumVol == 0 {
		return 0
	}
	return sumPriceVol / sumVol
}

// CalcDRVI calculates the Daily Range Volatility Index (StdDev of daily range %).
// This measures intraday price volatility as the standard deviation of
// (Highest − Lowest) / Average across recent days.
//
// NOTE: Previously named CalcPVI. Renamed to avoid confusion with the classic
// "Positive Volume Index" (Norman Fosback, 1976) which tracks price changes on
// days when volume increases — an entirely different concept.
func CalcDRVI(history []esi.HistoryEntry, days int) float64 {
	entries := filterLastNDays(history, days)
	if len(entries) < 2 {
		return 0
	}

	var ranges []float64
	for _, h := range entries {
		if h.Average > 0 {
			dailyRange := (h.Highest - h.Lowest) / h.Average * 100
			ranges = append(ranges, dailyRange)
		}
	}

	if len(ranges) < 2 {
		return 0
	}

	return stdDev(ranges)
}

// stdDev calculates sample standard deviation (Bessel's correction: N-1).
func stdDev(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}

	var sum float64
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(len(values))

	var variance float64
	for _, v := range values {
		diff := v - mean
		variance += diff * diff
	}
	variance /= float64(len(values) - 1) // Bessel's correction: unbiased sample variance

	return math.Sqrt(variance)
}

// CalcOBDS calculates Order Book Depth Score.
// Measures liquidity within ±5% of best price.
func CalcOBDS(buyOrders, sellOrders []esi.MarketOrder, capitalRequired float64) float64 {
	if capitalRequired <= 0 || len(buyOrders) == 0 || len(sellOrders) == 0 {
		return 0
	}

	bestBuy := maxBuyPrice(buyOrders)
	bestSell := minSellPrice(sellOrders)

	if bestBuy <= 0 || bestSell <= 0 {
		return 0
	}

	buyDepth := sumVolumeWithinPercent(buyOrders, bestBuy, 5.0, true)
	sellDepth := sumVolumeWithinPercent(sellOrders, bestSell, 5.0, false)

	minDepth := math.Min(buyDepth, sellDepth)
	return minDepth / capitalRequired
}

// maxBuyPrice finds the highest buy order price.
func maxBuyPrice(orders []esi.MarketOrder) float64 {
	var max float64
	for _, o := range orders {
		if o.Price > max {
			max = o.Price
		}
	}
	return max
}

// minSellPrice finds the lowest sell order price.
func minSellPrice(orders []esi.MarketOrder) float64 {
	min := math.MaxFloat64
	for _, o := range orders {
		if o.Price < min {
			min = o.Price
		}
	}
	if min == math.MaxFloat64 {
		return 0
	}
	return min
}

// sumVolumeWithinPercent sums ISK value of orders within X% of reference price.
func sumVolumeWithinPercent(orders []esi.MarketOrder, refPrice, pct float64, isBuy bool) float64 {
	var total float64
	for _, o := range orders {
		var priceDiff float64
		if isBuy {
			// For buy orders, we count those within X% below the best buy
			priceDiff = (refPrice - o.Price) / refPrice * 100
		} else {
			// For sell orders, we count those within X% above the best sell
			priceDiff = (o.Price - refPrice) / refPrice * 100
		}
		if priceDiff >= 0 && priceDiff <= pct {
			total += o.Price * float64(o.VolumeRemain)
		}
	}
	return total
}

// CalcSDS calculates Scam Detection Score (0-100).
func CalcSDS(buyOrders []esi.MarketOrder, history []esi.HistoryEntry, vwap float64) int {
	score := 0
	if len(buyOrders) == 0 {
		return 100 // No buy orders = suspicious
	}

	bestBuy := maxBuyPrice(buyOrders)

	// +30: Best buy < 50% of VWAP (price deviation)
	if vwap > 0 && bestBuy < vwap*0.5 {
		score += 30
	}

	// +25: Order volume >> daily volume * 10 (volume mismatch)
	dailyVol := avgDailyVolume(history, 7)
	totalOrderVol := sumOrderVolume(buyOrders)
	if dailyVol > 0 && float64(totalOrderVol) > dailyVol*10 {
		score += 25
	}

	// +25: Single order dominates >90% volume
	if singleOrderDominance(buyOrders) > 0.9 {
		score += 25
	}

	// +20: No trades in last 7 days
	if noRecentTrades(history, 7) {
		score += 20
	}

	return score
}

// avgDailyVolume calculates average daily volume from history.
func avgDailyVolume(history []esi.HistoryEntry, days int) float64 {
	entries := filterLastNDays(history, days)
	if len(entries) == 0 {
		return 0
	}
	var total int64
	for _, h := range entries {
		total += h.Volume
	}
	return float64(total) / float64(len(entries))
}

// sumOrderVolume sums total volume of orders.
func sumOrderVolume(orders []esi.MarketOrder) int64 {
	var total int64
	for _, o := range orders {
		total += int64(o.VolumeRemain)
	}
	return total
}

// singleOrderDominance returns ratio of largest order to total volume.
func singleOrderDominance(orders []esi.MarketOrder) float64 {
	if len(orders) == 0 {
		return 0
	}
	var maxVol int32
	var total int32
	for _, o := range orders {
		total += o.VolumeRemain
		if o.VolumeRemain > maxVol {
			maxVol = o.VolumeRemain
		}
	}
	if total == 0 {
		return 0
	}
	return float64(maxVol) / float64(total)
}

// noRecentTrades checks if there were no trades in the last N days.
func noRecentTrades(history []esi.HistoryEntry, days int) bool {
	entries := filterLastNDays(history, days)
	if len(entries) == 0 {
		return true
	}
	for _, h := range entries {
		if h.Volume > 0 {
			return false
		}
	}
	return true
}

// CalcCI calculates Competition Index.
func CalcCI(orders []esi.MarketOrder) int {
	if len(orders) == 0 {
		return 0
	}

	// Base score: number of unique orders
	score := len(orders)

	// Count "0.01 ISK wars" (orders with very tight relative spreads)
	tightSpreadCount := countTightSpreadOrders(orders)
	score += tightSpreadCount * 2

	return score
}

// countTightSpreadOrders counts orders within 0.01% of each other's price.
// Uses relative threshold to work correctly for both cheap (< 1 ISK) and expensive (> 1B ISK) items.
func countTightSpreadOrders(orders []esi.MarketOrder) int {
	if len(orders) < 2 {
		return 0
	}

	// Sort by price
	prices := make([]float64, len(orders))
	for i, o := range orders {
		prices[i] = o.Price
	}
	sort.Float64s(prices)

	count := 0
	for i := 1; i < len(prices); i++ {
		if prices[i] <= 0 {
			continue
		}
		// Relative threshold: 0.01% of the price (e.g., 0.01 ISK for a 100 ISK item,
		// 100,000 ISK for a 1B ISK item)
		relativeThreshold := prices[i] * 0.0001
		// Floor at 0.01 ISK (EVE minimum price increment)
		if relativeThreshold < 0.01 {
			relativeThreshold = 0.01
		}
		if prices[i]-prices[i-1] <= relativeThreshold {
			count++
		}
	}
	return count
}

// CalcCTS calculates Composite Trading Score (0-100). Higher is better.
//
// Weight rationale (sum = 1.0):
//   - SpreadROI  (25%): Primary profit driver — a wide, stable spread is the core
//     value proposition of station trading.
//   - SDS        (20%): Scam/manipulation detection is critical in EVE; even a
//     profitable-looking item is worthless if the book is manipulated.
//   - OBDS       (15%): Order book depth ensures fills can actually happen at the
//     quoted spread without excessive slippage.
//   - DRVI       (15%): Lower daily range volatility means more predictable margins;
//     high DRVI items can swing against the trader between order placement and fill.
//   - Volume     (15%): Higher turnover means faster capital cycling and lower
//     opportunity cost of locked-up ISK.
//   - CI         (10%): Competition matters but is partially captured by spread
//     compression already; kept at lower weight to avoid double-counting.
type CTSWeights struct {
	SpreadROI float64
	OBDS      float64
	DRVI      float64
	CI        float64
	SDS       float64
	Volume    float64
}

var DefaultCTSWeights = CTSWeights{
	SpreadROI: 0.25,
	OBDS:      0.15,
	DRVI:      0.15,
	CI:        0.10,
	SDS:       0.20,
	Volume:    0.15,
}

func normalizeCTSWeights(weights CTSWeights) CTSWeights {
	// Guard against invalid negative weights.
	if weights.SpreadROI < 0 {
		weights.SpreadROI = 0
	}
	if weights.OBDS < 0 {
		weights.OBDS = 0
	}
	if weights.DRVI < 0 {
		weights.DRVI = 0
	}
	if weights.CI < 0 {
		weights.CI = 0
	}
	if weights.SDS < 0 {
		weights.SDS = 0
	}
	if weights.Volume < 0 {
		weights.Volume = 0
	}

	total := weights.SpreadROI + weights.OBDS + weights.DRVI + weights.CI + weights.SDS + weights.Volume
	if total <= 0 {
		return DefaultCTSWeights
	}
	return CTSWeights{
		SpreadROI: weights.SpreadROI / total,
		OBDS:      weights.OBDS / total,
		DRVI:      weights.DRVI / total,
		CI:        weights.CI / total,
		SDS:       weights.SDS / total,
		Volume:    weights.Volume / total,
	}
}

func CalcCTSWithWeights(spreadROI, obds, drvi float64, ci, sds int, dailyVolume float64, weights CTSWeights) float64 {
	weights = normalizeCTSWeights(weights)

	// Normalize each component to 0-100 scale
	roiScore := normalize(spreadROI, 0, 300) * 100        // Higher spread ROI = better (300% cap for lowsec/null)
	obdsScore := normalize(obds, 0, 2) * 100              // Higher depth = better
	pviScore := 100 - normalize(drvi, 0, 50)*100          // Lower volatility = better
	ciScore := 100 - normalize(float64(ci), 0, 100)*100   // Lower competition = better
	sdsScore := 100 - normalize(float64(sds), 0, 100)*100 // Lower scam score = better

	// Volume score: use log scale so both low-volume (10/day) and high-volume (10000/day)
	// items are fairly represented. log10(10)=1, log10(100)=2, log10(1000)=3, log10(10000)=4
	var volScore float64
	if dailyVolume > 1 {
		volScore = normalize(math.Log10(dailyVolume), 0, 4) * 100 // 0..10000 units/day mapped to 0..100
	}

	return roiScore*weights.SpreadROI +
		obdsScore*weights.OBDS +
		pviScore*weights.DRVI +
		ciScore*weights.CI +
		sdsScore*weights.SDS +
		volScore*weights.Volume
}

func CalcCTS(spreadROI, obds, drvi float64, ci, sds int, dailyVolume float64) float64 {
	return CalcCTSWithWeights(spreadROI, obds, drvi, ci, sds, dailyVolume, DefaultCTSWeights)
}

// normalize clamps value to [0, 1] range based on min/max.
func normalize(value, minVal, maxVal float64) float64 {
	if maxVal <= minVal {
		return 0
	}
	normalized := (value - minVal) / (maxVal - minVal)
	if normalized < 0 {
		return 0
	}
	if normalized > 1 {
		return 1
	}
	return normalized
}

// CalcSpreadROI estimates the typical buy-sell spread as a percentage, using
// 10th percentile of daily lows (typical buy entry) and 90th percentile of
// daily highs (typical sell exit) over N days. This measures the realistic
// trading spread available to a station trader, not buy-and-hold ROI.
// Outlier spikes and crashes are filtered by the percentile approach.
//
// The result populates the JSON field "PeriodROI" for backward compatibility.
func CalcSpreadROI(history []esi.HistoryEntry, days int) float64 {
	entries := filterLastNDays(history, days)
	if len(entries) < 2 {
		return 0
	}

	// Collect all daily low and high prices
	lows := make([]float64, 0, len(entries))
	highs := make([]float64, 0, len(entries))
	for _, h := range entries {
		if h.Lowest > 0 {
			lows = append(lows, h.Lowest)
		}
		if h.Highest > 0 {
			highs = append(highs, h.Highest)
		}
	}
	if len(lows) < 2 || len(highs) < 2 {
		return 0
	}

	sort.Float64s(lows)
	sort.Float64s(highs)

	// Use 10th percentile of lows (typical buy) and 90th percentile of highs (typical sell)
	// to filter out outlier spikes / crashes.
	// Using pure percentiles avoids VWAP-blending which compresses the spread
	// and systematically underestimates ROI.
	typicalBuy := percentile(lows, 10)
	typicalSell := percentile(highs, 90)

	if typicalBuy <= 0 {
		return 0
	}

	return (typicalSell - typicalBuy) / typicalBuy * 100
}

// percentile returns the p-th percentile from a sorted slice (p in 0..100).
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	idx := p / 100 * float64(len(sorted)-1)
	lower := int(math.Floor(idx))
	upper := int(math.Ceil(idx))
	if lower == upper || upper >= len(sorted) {
		return sorted[lower]
	}
	frac := idx - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}

// CalcAvgPriceStats returns average (VWAP), high, and low prices over N days.
// Computes VWAP inline from the same filtered entries to avoid a redundant
// filterLastNDays pass that CalcVWAP would perform.
func CalcAvgPriceStats(history []esi.HistoryEntry, days int) (avg, high, low float64) {
	entries := filterLastNDays(history, days)
	if len(entries) == 0 {
		return 0, 0, 0
	}

	// VWAP inline (avoids double filterLastNDays)
	var sumPriceVol, sumVol float64
	low = math.MaxFloat64
	for _, h := range entries {
		sumPriceVol += h.Average * float64(h.Volume)
		sumVol += float64(h.Volume)
		if h.Highest > high {
			high = h.Highest
		}
		if h.Lowest < low && h.Lowest > 0 {
			low = h.Lowest
		}
	}
	if sumVol > 0 {
		avg = sumPriceVol / sumVol
	}
	if low == math.MaxFloat64 {
		low = 0
	}
	return avg, high, low
}

// IsExtremePrice checks if current price deviates significantly from historical average.
func IsExtremePrice(currentPrice, avgPrice float64, thresholdPct float64) bool {
	if avgPrice <= 0 {
		return false
	}
	deviation := math.Abs(currentPrice-avgPrice) / avgPrice * 100
	return deviation > thresholdPct
}
