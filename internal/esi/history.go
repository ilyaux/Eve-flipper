package esi

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// HistoryEntry represents a single day of market history for an item in a region.
type HistoryEntry struct {
	Date       string  `json:"date"`
	Average    float64 `json:"average"`
	Highest    float64 `json:"highest"`
	Lowest     float64 `json:"lowest"`
	Volume     int64   `json:"volume"`
	OrderCount int64   `json:"order_count"`
}

// MarketStats holds computed statistics from market history.
type MarketStats struct {
	DailyVolume int64   // average daily volume over last 7 days
	Velocity    float64 // daily_volume / total_listed_quantity
	PriceTrend  float64 // % change over last 7 days (Theil-Sen slope)
}

// FetchMarketHistory fetches market history for a type in a region from ESI.
func (c *Client) FetchMarketHistory(regionID, typeID int32) ([]HistoryEntry, error) {
	url := fmt.Sprintf("%s/markets/%d/history/?datasource=tranquility&type_id=%d",
		baseURL, regionID, typeID)

	var entries []HistoryEntry
	if err := c.GetJSON(url, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// ComputeMarketStats computes trading statistics from history entries.
func ComputeMarketStats(entries []HistoryEntry, totalListed int32) MarketStats {
	if len(entries) == 0 {
		return MarketStats{}
	}

	// Sort entries by date to ensure correct chronological order.
	// ESI does not guarantee chronological order.
	sorted := make([]HistoryEntry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Date < sorted[j].Date
	})

	now := time.Now().UTC()
	cutoff7 := now.AddDate(0, 0, -7).Format("2006-01-02")

	var vol7 int64
	var count7 int
	// Collect prices with day-index for Theil-Sen regression.
	var prices []float64
	var dayIndices []float64

	for _, e := range sorted {
		if e.Date >= cutoff7 {
			vol7 += e.Volume
			count7++
			if e.Average > 0 {
				prices = append(prices, e.Average)
				dayIndices = append(dayIndices, float64(len(prices)-1))
			}
		}
	}

	// Use float division to avoid rounding down small volumes.
	// e.g. vol7=5, count7=3 → 1.67 → rounds to 2 instead of 1.
	dailyVol := int64(0)
	if count7 > 0 {
		dailyVol = int64(math.Round(float64(vol7) / float64(count7)))
	}

	velocity := 0.0
	if totalListed > 0 {
		velocity = float64(dailyVol) / float64(totalListed)
	}

	// Price trend: Theil-Sen median slope over 7-day window, expressed as % change.
	// Theil-Sen is robust to outliers (up to ~29% breakdown point), unlike OLS
	// which can be heavily influenced by a single spike or crash day.
	// slope = median of all pairwise slopes (y_j - y_i) / (x_j - x_i), i < j
	// trend% = slope * (N-1) / midPrice * 100
	trend := 0.0
	if len(prices) >= 2 {
		n := len(prices)

		// Compute all pairwise slopes.
		slopes := make([]float64, 0, n*(n-1)/2)
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				dx := dayIndices[j] - dayIndices[i]
				if dx > 0 {
					slopes = append(slopes, (prices[j]-prices[i])/dx)
				}
			}
		}

		if len(slopes) > 0 {
			sort.Float64s(slopes)
			slope := medianSorted(slopes)

			// Mid-price as mean of prices for normalization.
			var sumP float64
			for _, p := range prices {
				sumP += p
			}
			midPrice := sumP / float64(n)

			if midPrice > 0 {
				// Total % change over the window: slope * (N-1) days / average price * 100
				trend = slope * float64(n-1) / midPrice * 100
			}
		}
	}

	return MarketStats{
		DailyVolume: dailyVol,
		Velocity:    velocity,
		PriceTrend:  trend,
	}
}

// medianSorted returns the median of a pre-sorted slice.
func medianSorted(s []float64) float64 {
	n := len(s)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return s[n/2]
	}
	return 0.5 * (s[n/2-1] + s[n/2])
}
