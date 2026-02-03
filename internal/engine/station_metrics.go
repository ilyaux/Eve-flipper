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

// CalcPVI calculates Price Volatility Index (StdDev of daily range %).
func CalcPVI(history []esi.HistoryEntry, days int) float64 {
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

// stdDev calculates standard deviation.
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
	variance /= float64(len(values))

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
	return len(entries) == 0
}

// CalcCI calculates Competition Index.
func CalcCI(orders []esi.MarketOrder) int {
	if len(orders) == 0 {
		return 0
	}

	// Base score: number of unique orders
	score := len(orders)

	// Count "0.01 ISK wars" (orders with very tight spreads)
	tightSpreadCount := countTightSpreadOrders(orders, 0.01)
	score += tightSpreadCount * 2

	return score
}

// countTightSpreadOrders counts orders within threshold ISK of each other.
func countTightSpreadOrders(orders []esi.MarketOrder, threshold float64) int {
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
		if prices[i]-prices[i-1] <= threshold {
			count++
		}
	}
	return count
}

// CalcCTS calculates Composite Trading Score (0-100).
// Higher is better.
func CalcCTS(periodROI, obds, pvi float64, ci, sds int, dailyVolume float64) float64 {
	// Normalize each component to 0-100 scale
	roiScore := normalize(periodROI, 0, 100)       // Higher ROI = better
	obdsScore := normalize(obds, 0, 2) * 100       // Higher depth = better
	pviScore := 100 - normalize(pvi, 0, 50)*100    // Lower volatility = better
	ciScore := 100 - normalize(float64(ci), 0, 100)*100  // Lower competition = better
	sdsScore := 100 - normalize(float64(sds), 0, 100)*100 // Lower scam score = better
	volScore := normalize(dailyVolume, 0, 100)     // Higher volume = better (capped at 100)

	// Weighted sum (weights should sum to 1.0)
	return roiScore*0.25 +
		obdsScore*0.15 +
		pviScore*0.15 +
		ciScore*0.10 +
		sdsScore*0.20 +
		volScore*0.15
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

// CalcPeriodROI calculates ROI based on historical average prices.
func CalcPeriodROI(history []esi.HistoryEntry, days int) float64 {
	entries := filterLastNDays(history, days)
	if len(entries) < 2 {
		return 0
	}

	// Use VWAP as average price
	vwap := CalcVWAP(history, days)
	if vwap <= 0 {
		return 0
	}

	// Find min and max for potential spread
	var minPrice float64 = math.MaxFloat64
	var maxPrice float64
	for _, h := range entries {
		if h.Lowest < minPrice {
			minPrice = h.Lowest
		}
		if h.Highest > maxPrice {
			maxPrice = h.Highest
		}
	}

	if minPrice >= math.MaxFloat64 || minPrice <= 0 {
		return 0
	}

	// Period ROI approximation: (typical sell - typical buy) / typical buy
	// Using avg between high-low range as typical prices
	typicalBuy := (minPrice + vwap) / 2
	typicalSell := (maxPrice + vwap) / 2

	return (typicalSell - typicalBuy) / typicalBuy * 100
}

// CalcAvgPriceStats returns average, high, and low prices over N days.
func CalcAvgPriceStats(history []esi.HistoryEntry, days int) (avg, high, low float64) {
	entries := filterLastNDays(history, days)
	if len(entries) == 0 {
		return 0, 0, 0
	}

	avg = CalcVWAP(history, days)
	low = math.MaxFloat64
	for _, h := range entries {
		if h.Highest > high {
			high = h.Highest
		}
		if h.Lowest < low && h.Lowest > 0 {
			low = h.Lowest
		}
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
