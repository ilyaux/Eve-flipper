package engine

import (
	"sync"

	"eve-flipper/internal/esi"
)

type routeLiquidityKey struct {
	regionID int32
	typeID   int32
}

type routeLiquidityStats struct {
	dailyVolume int64
	available   bool
}

func (s *Scanner) enrichRoutesWithLiquidity(routes []RouteResult, progress func(string)) {
	if s == nil || s.History == nil || len(routes) == 0 {
		return
	}
	if progress != nil {
		progress("Scoring route liquidity...")
	}

	needed := make(map[routeLiquidityKey]bool)
	for _, route := range routes {
		for _, hop := range route.Hops {
			regionID := int32(0)
			if s.SDE != nil && s.SDE.Universe != nil {
				regionID = s.SDE.Universe.SystemRegion[hop.DestSystemID]
			}
			if regionID <= 0 || hop.TypeID <= 0 {
				continue
			}
			needed[routeLiquidityKey{regionID: regionID, typeID: hop.TypeID}] = true
		}
	}
	if len(needed) == 0 {
		return
	}

	statsByKey := make(map[routeLiquidityKey]routeLiquidityStats, len(needed))
	type result struct {
		key   routeLiquidityKey
		stats routeLiquidityStats
	}
	outCh := make(chan result, len(needed))
	sem := make(chan struct{}, 10)
	var wg sync.WaitGroup
	for key := range needed {
		wg.Add(1)
		go func(k routeLiquidityKey) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			entries := s.historyEntries(k.regionID, k.typeID)
			if len(entries) == 0 {
				outCh <- result{key: k}
				return
			}
			stats := esi.ComputeMarketStats(entries, 0)
			outCh <- result{
				key: k,
				stats: routeLiquidityStats{
					dailyVolume: stats.DailyVolume,
					available:   len(entries) > 0,
				},
			}
		}(key)
	}
	wg.Wait()
	close(outCh)
	for r := range outCh {
		statsByKey[r.key] = r.stats
	}

	for i := range routes {
		for j := range routes[i].Hops {
			hop := &routes[i].Hops[j]
			regionID := int32(0)
			if s.SDE != nil && s.SDE.Universe != nil {
				regionID = s.SDE.Universe.SystemRegion[hop.DestSystemID]
			}
			stats := statsByKey[routeLiquidityKey{regionID: regionID, typeID: hop.TypeID}]
			hop.DailyVolume = stats.dailyVolume
			hop.FillTimeDays = estimateFillTimeDaysFromFlow(int64(hop.Units), float64(stats.dailyVolume))
			hop.LiquidityScore, hop.LiquidityLabel = liquidityScoreFromFillTime(hop.FillTimeDays, stats.available)
		}
		routes[i].FillTimeDays, routes[i].LiquidityScore, routes[i].LiquidityLabel = aggregateLiquidity(routes[i].Hops)
	}
}
