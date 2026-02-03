package esi

import (
	"fmt"
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
	PriceTrend  float64 // % change over last 7 days
}

// HistoryCache is a persistent cache for market history data.
type HistoryCache interface {
	GetHistory(regionID int32, typeID int32) ([]HistoryEntry, bool)
	SetHistory(regionID int32, typeID int32, entries []HistoryEntry)
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

	now := time.Now().UTC()
	cutoff7 := now.AddDate(0, 0, -7).Format("2006-01-02")

	var vol7 int64
	var count7 int
	var firstPrice, lastPrice float64

	for _, e := range entries {
		if e.Date >= cutoff7 {
			vol7 += e.Volume
			count7++
			if firstPrice == 0 {
				firstPrice = e.Average
			}
			lastPrice = e.Average
		}
	}

	dailyVol := int64(0)
	if count7 > 0 {
		dailyVol = vol7 / int64(count7)
	}

	velocity := 0.0
	if totalListed > 0 {
		velocity = float64(dailyVol) / float64(totalListed)
	}

	trend := 0.0
	if firstPrice > 0 {
		trend = (lastPrice - firstPrice) / firstPrice * 100
	}

	return MarketStats{
		DailyVolume: dailyVol,
		Velocity:    velocity,
		PriceTrend:  trend,
	}
}
