package engine

import (
	"math"
	"sort"

	"eve-flipper/internal/esi"
)

const defaultBacktestWindowDays = 30

type historicalFillBacktest struct {
	Days        int
	FillRate    float64
	MedianVol   int64
	AvgDailyVol float64
}

func estimateFillTimeDaysFromFlow(units int64, dailyFlow float64) float64 {
	if units <= 0 || dailyFlow <= 0 {
		return 0
	}
	return sanitizeFloat(float64(units) / dailyFlow)
}

func estimateCycleFillTimeDays(units int32, s2bPerDay, bfsPerDay float64) float64 {
	if units <= 0 || s2bPerDay <= 0 || bfsPerDay <= 0 {
		return 0
	}
	return estimateFillTimeDaysFromFlow(int64(units), math.Min(s2bPerDay, bfsPerDay))
}

func liquidityScoreFromFillTime(fillTimeDays float64, historyAvailable bool) (float64, string) {
	if fillTimeDays <= 0 || !historyAvailable {
		return 0, "unknown"
	}

	score := 0.0
	switch {
	case fillTimeDays <= 1:
		score = 100
	case fillTimeDays <= 2:
		score = 90
	case fillTimeDays <= 3:
		score = 80
	case fillTimeDays <= 7:
		score = 65
	case fillTimeDays <= 14:
		score = 45
	case fillTimeDays <= 30:
		score = 25
	default:
		score = 10
	}

	label := "thin"
	switch {
	case score >= 75:
		label = "high"
	case score >= 45:
		label = "medium"
	case score >= 20:
		label = "low"
	}
	return sanitizeFloat(score), label
}

func computeHistoricalFillBacktest(entries []esi.HistoryEntry, units int32) historicalFillBacktest {
	if len(entries) == 0 || units <= 0 {
		return historicalFillBacktest{}
	}

	sorted := make([]esi.HistoryEntry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Date < sorted[j].Date
	})
	if len(sorted) > defaultBacktestWindowDays {
		sorted = sorted[len(sorted)-defaultBacktestWindowDays:]
	}

	volumes := make([]int64, 0, len(sorted))
	enoughDays := 0
	totalVol := int64(0)
	for _, e := range sorted {
		if e.Volume < 0 {
			continue
		}
		volumes = append(volumes, e.Volume)
		totalVol += e.Volume
		if e.Volume >= int64(units) {
			enoughDays++
		}
	}
	if len(volumes) == 0 {
		return historicalFillBacktest{}
	}

	sort.Slice(volumes, func(i, j int) bool { return volumes[i] < volumes[j] })
	mid := len(volumes) / 2
	median := volumes[mid]
	if len(volumes)%2 == 0 {
		median = int64(math.Round(float64(volumes[mid-1]+volumes[mid]) / 2))
	}

	return historicalFillBacktest{
		Days:        len(volumes),
		FillRate:    sanitizeFloat(float64(enoughDays) / float64(len(volumes)) * 100),
		MedianVol:   median,
		AvgDailyVol: sanitizeFloat(float64(totalVol) / float64(len(volumes))),
	}
}

func aggregateLiquidity(hops []RouteHop) (float64, float64, string) {
	if len(hops) == 0 {
		return 0, 0, ""
	}
	worstDays := 0.0
	minScore := math.MaxFloat64
	known := 0
	for _, hop := range hops {
		if hop.FillTimeDays > worstDays {
			worstDays = hop.FillTimeDays
		}
		if hop.LiquidityScore > 0 {
			known++
			if hop.LiquidityScore < minScore {
				minScore = hop.LiquidityScore
			}
		}
	}
	if known == 0 || minScore == math.MaxFloat64 {
		return sanitizeFloat(worstDays), 0, "unknown"
	}
	label := "thin"
	switch {
	case minScore >= 75:
		label = "high"
	case minScore >= 45:
		label = "medium"
	case minScore >= 20:
		label = "low"
	}
	return sanitizeFloat(worstDays), sanitizeFloat(minScore), label
}

func EnrichFlipResultsWithInventory(results []FlipResult, inventory *RegionalInventorySnapshot) {
	if inventory == nil || len(results) == 0 {
		return
	}
	for i := range results {
		typeID := results[i].TypeID
		if inventory.AssetsByType != nil {
			results[i].CharacterAssets = inventory.AssetsByType[typeID]
		}
		if inventory.ActiveBuyByType != nil {
			results[i].CharacterBuyOrders = inventory.ActiveBuyByType[typeID]
		}
		if inventory.ActiveSellByType != nil {
			results[i].CharacterSellOrders = inventory.ActiveSellByType[typeID]
		}
	}
}

func EnrichStationTradesWithInventory(results []StationTrade, inventory *RegionalInventorySnapshot) {
	if inventory == nil || len(results) == 0 {
		return
	}
	for i := range results {
		typeID := results[i].TypeID
		if inventory.AssetsByType != nil {
			results[i].CharacterAssets = inventory.AssetsByType[typeID]
		}
		if inventory.ActiveBuyByType != nil {
			results[i].CharacterBuyOrders = inventory.ActiveBuyByType[typeID]
		}
		if inventory.ActiveSellByType != nil {
			results[i].CharacterSellOrders = inventory.ActiveSellByType[typeID]
		}
	}
}
