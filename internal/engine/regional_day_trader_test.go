package engine

import (
	"math"
	"testing"
	"time"

	"eve-flipper/internal/esi"
	"eve-flipper/internal/sde"
)

func TestBuildRegionalDayTrader_GroupsAndCalculates(t *testing.T) {
	now := time.Now().UTC()
	sourceHistory := []esi.HistoryEntry{
		{Date: now.AddDate(0, 0, -1).Format("2006-01-02"), Average: 100, Volume: 100},
		{Date: now.Format("2006-01-02"), Average: 100, Volume: 100},
	}
	targetHistory := []esi.HistoryEntry{
		{Date: now.AddDate(0, 0, -1).Format("2006-01-02"), Average: 200, Volume: 70},
		{Date: now.Format("2006-01-02"), Average: 200, Volume: 70},
	}

	hp := &testHistoryProvider{
		store: map[string][]esi.HistoryEntry{
			"1:34": sourceHistory,
			"2:34": targetHistory,
		},
	}

	scanner := &Scanner{
		SDE: &sde.Data{
			Systems: map[int32]*sde.SolarSystem{
				30002187: {ID: 30002187, Name: "Amarr", RegionID: 1, Security: 0.87},
				30000142: {ID: 30000142, Name: "Jita", RegionID: 2, Security: 0.95},
			},
			Regions: map[int32]*sde.Region{
				1: {ID: 1, Name: "Domain"},
				2: {ID: 2, Name: "The Forge"},
			},
		},
		History: hp,
	}

	flips := []FlipResult{
		{
			TypeID:          34,
			TypeName:        "Tritanium",
			Volume:          0.01,
			BuyPrice:        100,
			SellPrice:       205,
			BuySystemID:     30002187,
			BuySystemName:   "Amarr",
			BuyRegionID:     1,
			BuyRegionName:   "Domain",
			SellSystemID:    30000142,
			SellSystemName:  "Jita",
			SellRegionID:    2,
			SellRegionName:  "The Forge",
			UnitsToBuy:      20,
			SellOrderRemain: 500,
			BuyOrderRemain:  120,
			SellJumps:       10,
		},
	}

	hubs, totalItems, targetRegion, periodDays := scanner.BuildRegionalDayTrader(
		ScanParams{
			AvgPricePeriod: 14,
		},
		flips,
		nil,
		nil,
	)

	if len(hubs) != 1 {
		t.Fatalf("hub count = %d, want 1", len(hubs))
	}
	if totalItems != 1 {
		t.Fatalf("totalItems = %d, want 1", totalItems)
	}
	if targetRegion != "The Forge" {
		t.Fatalf("targetRegion = %q, want The Forge", targetRegion)
	}
	if periodDays != 14 {
		t.Fatalf("periodDays = %d, want 14", periodDays)
	}

	hub := hubs[0]
	if hub.ItemCount != 1 {
		t.Fatalf("hub item_count = %d, want 1", hub.ItemCount)
	}
	if len(hub.Items) != 1 {
		t.Fatalf("hub items len = %d, want 1", len(hub.Items))
	}
	item := hub.Items[0]

	// avgDailyVolume = (70+70)/14 = 10, so purchase should be capped from 20 to 10.
	if item.PurchaseUnits != 10 {
		t.Fatalf("purchase_units = %d, want 10", item.PurchaseUnits)
	}
	if math.Abs(item.SourceAvgPrice-100) > 1e-6 {
		t.Fatalf("source_avg_price = %v, want 100", item.SourceAvgPrice)
	}
	if math.Abs(item.TargetPeriodPrice-200) > 1e-6 {
		t.Fatalf("target_period_price = %v, want 200", item.TargetPeriodPrice)
	}
	if math.Abs(item.TargetDemandPerDay-10) > 1e-6 {
		t.Fatalf("target_demand_per_day = %v, want 10", item.TargetDemandPerDay)
	}
	if math.Abs(item.TargetPeriodProfit-1000) > 1e-6 {
		t.Fatalf("target_period_profit = %v, want 1000", item.TargetPeriodProfit)
	}
}

func TestBuildRegionalDayTrader_PurchaseUnitsBehaviorByRevenueMode(t *testing.T) {
	now := time.Now().UTC()
	sourceHistory := []esi.HistoryEntry{
		{Date: now.AddDate(0, 0, -1).Format("2006-01-02"), Average: 100, Volume: 300},
		{Date: now.Format("2006-01-02"), Average: 100, Volume: 300},
	}
	targetHistory := []esi.HistoryEntry{
		// avgDailyVolume over 14 days = (525+525)/14 = 75
		{Date: now.AddDate(0, 0, -1).Format("2006-01-02"), Average: 200, Volume: 525},
		{Date: now.Format("2006-01-02"), Average: 200, Volume: 525},
	}

	hp := &testHistoryProvider{
		store: map[string][]esi.HistoryEntry{
			"1:34": sourceHistory,
			"2:34": targetHistory,
		},
	}
	scanner := &Scanner{
		SDE: &sde.Data{
			Systems: map[int32]*sde.SolarSystem{
				1: {ID: 1, Name: "Src", RegionID: 1, Security: 0.9},
				2: {ID: 2, Name: "Dst", RegionID: 2, Security: 0.9},
			},
			Regions: map[int32]*sde.Region{
				1: {ID: 1, Name: "Source"},
				2: {ID: 2, Name: "Target"},
			},
		},
		History: hp,
	}

	flips := []FlipResult{
		{
			TypeID:          34,
			TypeName:        "Sigil",
			Volume:          5000,
			BuyPrice:        100,
			SellPrice:       220,
			BuySystemID:     1,
			BuySystemName:   "Src",
			BuyRegionID:     1,
			BuyRegionName:   "Source",
			SellSystemID:    2,
			SellSystemName:  "Dst",
			SellRegionID:    2,
			SellRegionName:  "Target",
			UnitsToBuy:      4,
			SellOrderRemain: 55,
			BuyOrderRemain:  4,
			SellJumps:       5,
		},
	}

	t.Run("instant_mode_keeps_units_to_buy_bound", func(t *testing.T) {
		hubs, totalItems, _, _ := scanner.BuildRegionalDayTrader(
			ScanParams{
				AvgPricePeriod:     14,
				PurchaseDemandDays: 0.5,
				SellOrderMode:      false,
			},
			flips,
			nil,
			nil,
		)
		if len(hubs) != 1 || totalItems != 1 {
			t.Fatalf("unexpected shape: hubs=%d items=%d", len(hubs), totalItems)
		}
		if got := hubs[0].Items[0].PurchaseUnits; got != 4 {
			t.Fatalf("purchase_units = %d, want 4 in instant mode", got)
		}
	})

	t.Run("sell_order_mode_uses_source_liquidity_and_demand_window", func(t *testing.T) {
		hubs, totalItems, _, _ := scanner.BuildRegionalDayTrader(
			ScanParams{
				AvgPricePeriod:     14,
				PurchaseDemandDays: 0.5,
				SellOrderMode:      true,
			},
			flips,
			nil,
			nil,
		)
		if len(hubs) != 1 || totalItems != 1 {
			t.Fatalf("unexpected shape: hubs=%d items=%d", len(hubs), totalItems)
		}
		// demand/day = 75, purchase window = 0.5 day => ceil(37.5) = 38
		// source liquidity is 55, so expected purchase is 38.
		if got := hubs[0].Items[0].PurchaseUnits; got != 38 {
			t.Fatalf("purchase_units = %d, want 38 in sell-order mode", got)
		}
	})

	t.Run("sell_order_mode_respects_cargo_capacity", func(t *testing.T) {
		hubs, totalItems, _, _ := scanner.BuildRegionalDayTrader(
			ScanParams{
				AvgPricePeriod:     14,
				PurchaseDemandDays: 0.5,
				SellOrderMode:      true,
				CargoCapacity:      50_000, // volume=5000 => max 10 units
			},
			flips,
			nil,
			nil,
		)
		if len(hubs) != 1 || totalItems != 1 {
			t.Fatalf("unexpected shape: hubs=%d items=%d", len(hubs), totalItems)
		}
		if got := hubs[0].Items[0].PurchaseUnits; got != 10 {
			t.Fatalf("purchase_units = %d, want 10 with cargo cap in sell-order mode", got)
		}
	})
}

func TestBuildRegionalDayTrader_MinItemProfitFiltersRows(t *testing.T) {
	scanner := &Scanner{
		SDE: &sde.Data{
			Systems: map[int32]*sde.SolarSystem{
				1: {ID: 1, Name: "Src", RegionID: 10000001, Security: 0.9},
				2: {ID: 2, Name: "Dst", RegionID: 10000002, Security: 0.9},
			},
			Regions: map[int32]*sde.Region{
				10000001: {ID: 10000001, Name: "Src Region"},
				10000002: {ID: 10000002, Name: "Dst Region"},
			},
		},
	}

	flips := []FlipResult{
		{
			TypeID:          1,
			TypeName:        "Test Item",
			Volume:          1,
			BuyPrice:        100,
			SellPrice:       101,
			BuySystemID:     1,
			BuySystemName:   "Src",
			BuyRegionID:     10000001,
			BuyRegionName:   "Src Region",
			SellSystemID:    2,
			SellSystemName:  "Dst",
			SellRegionID:    10000002,
			SellRegionName:  "Dst Region",
			UnitsToBuy:      10,
			SellOrderRemain: 1000,
			BuyOrderRemain:  1000,
			SellJumps:       5,
		},
	}

	hubs, totalItems, _, _ := scanner.BuildRegionalDayTrader(
		ScanParams{
			MinItemProfit: 1000,
		},
		flips,
		nil,
		nil,
	)

	if len(hubs) != 0 {
		t.Fatalf("hub count = %d, want 0", len(hubs))
	}
	if totalItems != 0 {
		t.Fatalf("totalItems = %d, want 0", totalItems)
	}
}

func TestBuildRegionalDayTrader_MinMarginUsesNowMarginInInstantMode(t *testing.T) {
	now := time.Now().UTC()
	sourceHistory := []esi.HistoryEntry{
		{Date: now.AddDate(0, 0, -1).Format("2006-01-02"), Average: 100, Volume: 200},
		{Date: now.Format("2006-01-02"), Average: 100, Volume: 200},
	}
	targetHistory := []esi.HistoryEntry{
		{Date: now.AddDate(0, 0, -1).Format("2006-01-02"), Average: 200, Volume: 200},
		{Date: now.Format("2006-01-02"), Average: 200, Volume: 200},
	}

	hp := &testHistoryProvider{
		store: map[string][]esi.HistoryEntry{
			"10:9001": sourceHistory,
			"20:9001": targetHistory,
		},
	}

	scanner := &Scanner{
		SDE: &sde.Data{
			Systems: map[int32]*sde.SolarSystem{
				1: {ID: 1, Name: "Src", RegionID: 10, Security: 0.9},
				2: {ID: 2, Name: "Dst", RegionID: 20, Security: 0.9},
			},
			Regions: map[int32]*sde.Region{
				10: {ID: 10, Name: "Source"},
				20: {ID: 20, Name: "Target"},
			},
		},
		History: hp,
	}

	// Now margin is negative (80 vs 100), but period margin is positive (history ~200).
	// MinMargin should filter this row out in instant mode.
	flips := []FlipResult{
		{
			TypeID:           9001,
			TypeName:         "Margin Leakage Probe",
			Volume:           1,
			BuyPrice:         100,
			ExpectedBuyPrice: 100,
			SellPrice:        80,
			BuySystemID:      1,
			BuySystemName:    "Src",
			BuyRegionID:      10,
			BuyRegionName:    "Source",
			SellSystemID:     2,
			SellSystemName:   "Dst",
			SellRegionID:     20,
			SellRegionName:   "Target",
			UnitsToBuy:       10,
			SellOrderRemain:  100,
			TargetSellSupply: 100,
			S2BPerDay:        10,
			SellJumps:        1,
		},
	}

	hubs, totalItems, _, _ := scanner.BuildRegionalDayTrader(
		ScanParams{
			MinMargin:      15,
			AvgPricePeriod: 14,
			SellOrderMode:  false,
		},
		flips,
		nil,
		nil,
	)
	if len(hubs) != 0 || totalItems != 0 {
		t.Fatalf("expected row to be filtered by now-margin, got hubs=%d items=%d", len(hubs), totalItems)
	}
}

func TestBuildRegionalDayTrader_InventoryCoverageReducesPurchase(t *testing.T) {
	scanner := &Scanner{
		SDE: &sde.Data{
			Systems: map[int32]*sde.SolarSystem{
				1: {ID: 1, Name: "Src", RegionID: 10, Security: 0.8},
				2: {ID: 2, Name: "Dst", RegionID: 20, Security: 0.9},
			},
			Regions: map[int32]*sde.Region{
				10: {ID: 10, Name: "Source"},
				20: {ID: 20, Name: "Target"},
			},
		},
	}

	flips := []FlipResult{
		{
			TypeID:          99,
			TypeName:        "Coverage Item",
			Volume:          1,
			BuyPrice:        100,
			SellPrice:       200,
			BuySystemID:     1,
			BuySystemName:   "Src",
			BuyRegionID:     10,
			BuyRegionName:   "Source",
			SellSystemID:    2,
			SellSystemName:  "Dst",
			SellRegionID:    20,
			SellRegionName:  "Target",
			UnitsToBuy:      30,
			SellOrderRemain: 1000,
			BuyOrderRemain:  1000,
			SellJumps:       2,
		},
	}

	inv := &RegionalInventorySnapshot{
		AssetsByType: map[int32]int64{
			99: 12,
		},
		ActiveSellByType: map[int32]int64{
			99: 8,
		},
	}

	hubs, totalItems, _, _ := scanner.BuildRegionalDayTrader(
		ScanParams{},
		flips,
		inv,
		nil,
	)
	if len(hubs) != 1 || totalItems != 1 {
		t.Fatalf("unexpected result shape: hubs=%d items=%d", len(hubs), totalItems)
	}
	item := hubs[0].Items[0]
	if item.PurchaseUnits != 10 {
		t.Fatalf("purchase units = %d, want 10", item.PurchaseUnits)
	}
	if item.Assets != 12 {
		t.Fatalf("assets coverage = %d, want 12", item.Assets)
	}
	if item.ActiveOrders != 8 {
		t.Fatalf("active order coverage = %d, want 8", item.ActiveOrders)
	}
}

// ── computeTradeScore ─────────────────────────────────────────────────────────

func TestComputeTradeScore_HighQualityItem(t *testing.T) {
	score := computeTradeScore(regionalTradeScoreInput{
		ROIPeriod:           35,
		DemandPerDay:        60,
		DOS:                 6,
		MarginPeriod:        24,
		HistoryEntries:      20,
		PeriodDays:          14,
		VolatilityDRVI:      7,
		PriceDislocationPct: 3,
		FlowBalanceScore:    0.92,
	})
	if score < 80 {
		t.Fatalf("high-quality score = %.2f, want >= 80", score)
	}
}

func TestComputeTradeScore_NoHistoryPenalty(t *testing.T) {
	withHistory := computeTradeScore(regionalTradeScoreInput{
		ROIPeriod:           25,
		DemandPerDay:        20,
		DOS:                 12,
		MarginPeriod:        18,
		HistoryEntries:      14,
		PeriodDays:          14,
		VolatilityDRVI:      10,
		PriceDislocationPct: 5,
		FlowBalanceScore:    0.8,
	})
	noHistory := computeTradeScore(regionalTradeScoreInput{
		ROIPeriod:           25,
		DemandPerDay:        20,
		DOS:                 12,
		MarginPeriod:        18,
		HistoryEntries:      0,
		PeriodDays:          14,
		VolatilityDRVI:      0,
		PriceDislocationPct: 5,
		FlowBalanceScore:    0.8,
	})
	if noHistory >= withHistory {
		t.Fatalf("no-history score = %.2f should be below with-history %.2f", noHistory, withHistory)
	}
}

func TestComputeTradeScore_HighDOSPenalty(t *testing.T) {
	base := computeTradeScore(regionalTradeScoreInput{
		ROIPeriod:           20,
		DemandPerDay:        15,
		DOS:                 10,
		MarginPeriod:        15,
		HistoryEntries:      14,
		PeriodDays:          14,
		VolatilityDRVI:      8,
		PriceDislocationPct: 4,
		FlowBalanceScore:    0.75,
	})
	highDOS := computeTradeScore(regionalTradeScoreInput{
		ROIPeriod:           20,
		DemandPerDay:        15,
		DOS:                 120,
		MarginPeriod:        15,
		HistoryEntries:      14,
		PeriodDays:          14,
		VolatilityDRVI:      8,
		PriceDislocationPct: 4,
		FlowBalanceScore:    0.75,
	})
	if highDOS >= base {
		t.Fatalf("high DOS score = %.2f should be below base %.2f", highDOS, base)
	}
}

func TestComputeTradeScore_VolatilityPenalty(t *testing.T) {
	lowVol := computeTradeScore(regionalTradeScoreInput{
		ROIPeriod:           22,
		DemandPerDay:        18,
		DOS:                 14,
		MarginPeriod:        16,
		HistoryEntries:      14,
		PeriodDays:          14,
		VolatilityDRVI:      8,
		PriceDislocationPct: 6,
		FlowBalanceScore:    0.7,
	})
	highVol := computeTradeScore(regionalTradeScoreInput{
		ROIPeriod:           22,
		DemandPerDay:        18,
		DOS:                 14,
		MarginPeriod:        16,
		HistoryEntries:      14,
		PeriodDays:          14,
		VolatilityDRVI:      70,
		PriceDislocationPct: 6,
		FlowBalanceScore:    0.7,
	})
	if highVol >= lowVol {
		t.Fatalf("high-volatility score = %.2f should be below low-volatility %.2f", highVol, lowVol)
	}
}

func TestComputeTradeScore_ClampedRange(t *testing.T) {
	maxLike := computeTradeScore(regionalTradeScoreInput{
		ROIPeriod:           1000,
		DemandPerDay:        10000,
		DOS:                 0,
		MarginPeriod:        1000,
		HistoryEntries:      365,
		PeriodDays:          14,
		VolatilityDRVI:      0,
		PriceDislocationPct: 0,
		FlowBalanceScore:    1,
	})
	minLike := computeTradeScore(regionalTradeScoreInput{
		ROIPeriod:           -1000,
		DemandPerDay:        0,
		DOS:                 1000,
		MarginPeriod:        -1000,
		HistoryEntries:      0,
		PeriodDays:          14,
		VolatilityDRVI:      1000,
		PriceDislocationPct: 1000,
		FlowBalanceScore:    0,
	})
	if maxLike > 100 {
		t.Fatalf("max-like score = %.2f exceeds 100", maxLike)
	}
	if minLike < 0 {
		t.Fatalf("min-like score = %.2f below 0", minLike)
	}
}

func TestBlendedRegionalDemandPerDay_UsesGeometricOnDivergence(t *testing.T) {
	row := FlipResult{
		S2BPerDay:   100,
		DailyVolume: 0,
	}
	stats := regionalHistoryStats{
		demandPerDay:  1,
		windowEntries: 14,
	}
	got := blendedRegionalDemandPerDay(row, stats, 14)
	// 0.7*sqrt(100*1) + 0.3*1 = 7.3
	if math.Abs(got-7.3) > 0.05 {
		t.Fatalf("blended demand = %.2f, want ~7.3", got)
	}
}

func TestBlendedRegionalDemandPerDay_CappedByHistoryFlow(t *testing.T) {
	row := FlipResult{
		S2BPerDay:   500,
		DailyVolume: 50,
	}
	stats := regionalHistoryStats{
		demandPerDay:  200,
		windowEntries: 14,
	}
	got := blendedRegionalDemandPerDay(row, stats, 14)
	if math.Abs(got-150) > 0.01 {
		t.Fatalf("blended demand cap = %.2f, want 150", got)
	}
}

func TestRobustRegionalHistoryPrice_SuppressesSingleSpike(t *testing.T) {
	now := time.Now().UTC()
	entries := make([]esi.HistoryEntry, 0, 14)
	for i := 0; i < 13; i++ {
		entries = append(entries, esi.HistoryEntry{
			Date:    now.AddDate(0, 0, -(13 - i)).Format("2006-01-02"),
			Average: 2000 + float64(i%3)*25,
			Volume:  100,
		})
	}
	entries = append(entries, esi.HistoryEntry{
		Date:    now.Format("2006-01-02"),
		Average: 9_800_000,
		Volume:  1,
	})

	stats := regionalHistoryStats{
		avgPrice:      640_000, // skewed raw VWAP-like anchor
		windowEntries: 14,
		entries:       entries,
	}

	got := robustRegionalHistoryPrice(stats, 14)
	if got < 1800 || got > 2600 {
		t.Fatalf("robust price = %.2f, expected near normal band (1800..2600)", got)
	}
}

func TestStabilizedSourceBuyPrice_AppliesHistoryFloorOnExtremeDislocation(t *testing.T) {
	now := time.Now().UTC()
	entries := make([]esi.HistoryEntry, 0, 14)
	for i := 0; i < 14; i++ {
		entries = append(entries, esi.HistoryEntry{
			Date:    now.AddDate(0, 0, -(14 - i)).Format("2006-01-02"),
			Average: 1000 + float64(i%2)*20,
			Volume:  50,
		})
	}
	stats := regionalHistoryStats{
		avgPrice:      1010,
		windowEntries: 14,
		entries:       entries,
	}

	got := stabilizedSourceBuyPrice(1, 1, stats, 14)
	if got < 200 {
		t.Fatalf("stabilized source price = %.2f, expected history floor >= 200", got)
	}
}

func TestStabilizedTargetPeriodPrice_InstantModeCapsAgainstLiveContext(t *testing.T) {
	now := time.Now().UTC()
	entries := make([]esi.HistoryEntry, 0, 14)
	for i := 0; i < 14; i++ {
		entries = append(entries, esi.HistoryEntry{
			Date:    now.AddDate(0, 0, -(14 - i)).Format("2006-01-02"),
			Average: 100_000,
			Volume:  25,
		})
	}
	stats := regionalHistoryStats{
		avgPrice:      100_000,
		windowEntries: 14,
		entries:       entries,
	}

	got := stabilizedTargetPeriodPrice(stats, 1000, 1200, 14, false)
	// In instant mode this is capped by ask-side context (1.25 * 1200).
	if math.Abs(got-1500) > 1e-6 {
		t.Fatalf("stabilized target period price = %.2f, want 1500", got)
	}
}

func TestBuildRegionalDayTrader_RobustHistoryPreventsExplosivePeriodROI(t *testing.T) {
	now := time.Now().UTC()
	sourceHistory := make([]esi.HistoryEntry, 0, 14)
	targetHistory := make([]esi.HistoryEntry, 0, 14)
	for i := 0; i < 13; i++ {
		sourceHistory = append(sourceHistory, esi.HistoryEntry{
			Date:    now.AddDate(0, 0, -(13 - i)).Format("2006-01-02"),
			Average: 1000,
			Volume:  100,
		})
		targetHistory = append(targetHistory, esi.HistoryEntry{
			Date:    now.AddDate(0, 0, -(13 - i)).Format("2006-01-02"),
			Average: 2000,
			Volume:  100,
		})
	}
	targetHistory = append(targetHistory, esi.HistoryEntry{
		Date:    now.Format("2006-01-02"),
		Average: 9_000_000,
		Volume:  1,
	})

	hp := &testHistoryProvider{
		store: map[string][]esi.HistoryEntry{
			"10:5001": sourceHistory,
			"20:5001": targetHistory,
		},
	}
	scanner := &Scanner{
		SDE: &sde.Data{
			Systems: map[int32]*sde.SolarSystem{
				1: {ID: 1, Name: "Src", RegionID: 10, Security: 0.8},
				2: {ID: 2, Name: "Dst", RegionID: 20, Security: 0.9},
			},
			Regions: map[int32]*sde.Region{
				10: {ID: 10, Name: "Source"},
				20: {ID: 20, Name: "Target"},
			},
		},
		History: hp,
	}

	flips := []FlipResult{
		{
			TypeID:          5001,
			TypeName:        "Outlier Guard Item",
			Volume:          1,
			BuyPrice:        1000,
			SellPrice:       1800,
			BuySystemID:     1,
			BuySystemName:   "Src",
			BuyRegionID:     10,
			BuyRegionName:   "Source",
			SellSystemID:    2,
			SellSystemName:  "Dst",
			SellRegionID:    20,
			SellRegionName:  "Target",
			UnitsToBuy:      1,
			SellOrderRemain: 100,
			BuyOrderRemain:  100,
			SellJumps:       1,
			S2BPerDay:       10,
		},
	}

	hubs, totalItems, _, _ := scanner.BuildRegionalDayTrader(ScanParams{}, flips, nil, nil)
	if len(hubs) != 1 || totalItems != 1 {
		t.Fatalf("unexpected result shape: hubs=%d items=%d", len(hubs), totalItems)
	}
	item := hubs[0].Items[0]
	if item.ROIPeriod > 500 {
		t.Fatalf("period ROI too high after robust guard: %.2f", item.ROIPeriod)
	}
}

func TestBuildRegionalDayTrader_ROIUsesMinimumExecutableCapital(t *testing.T) {
	scanner := &Scanner{
		SDE: &sde.Data{
			Systems: map[int32]*sde.SolarSystem{
				1: {ID: 1, Name: "Src", RegionID: 10, Security: 0.8},
				2: {ID: 2, Name: "Dst", RegionID: 20, Security: 0.9},
			},
			Regions: map[int32]*sde.Region{
				10: {ID: 10, Name: "Source"},
				20: {ID: 20, Name: "Target"},
			},
		},
	}

	flips := []FlipResult{
		{
			TypeID:          7001,
			TypeName:        "Tiny Capital Item",
			Volume:          0.1,
			BuyPrice:        1,
			SellPrice:       1000,
			BuySystemID:     1,
			BuySystemName:   "Src",
			BuyRegionID:     10,
			BuyRegionName:   "Source",
			SellSystemID:    2,
			SellSystemName:  "Dst",
			SellRegionID:    20,
			SellRegionName:  "Target",
			UnitsToBuy:      1,
			SellOrderRemain: 10,
			BuyOrderRemain:  10,
			SellJumps:       1,
		},
	}

	hubs, totalItems, _, _ := scanner.BuildRegionalDayTrader(ScanParams{}, flips, nil, nil)
	if len(hubs) != 1 || totalItems != 1 {
		t.Fatalf("unexpected result shape: hubs=%d items=%d", len(hubs), totalItems)
	}
	item := hubs[0].Items[0]
	if item.ROIPeriod > 2 {
		t.Fatalf("period ROI = %.2f, expected ROI damped by minimum executable capital", item.ROIPeriod)
	}
}

func TestBuildRegionalDayTrader_ROIUsesStricterFloorInSellOrderMode(t *testing.T) {
	scanner := &Scanner{
		SDE: &sde.Data{
			Systems: map[int32]*sde.SolarSystem{
				1: {ID: 1, Name: "Src", RegionID: 10, Security: 0.8},
				2: {ID: 2, Name: "Dst", RegionID: 20, Security: 0.9},
			},
			Regions: map[int32]*sde.Region{
				10: {ID: 10, Name: "Source"},
				20: {ID: 20, Name: "Target"},
			},
		},
	}

	flips := []FlipResult{
		{
			TypeID:           7002,
			TypeName:         "Tiny Capital Sell Mode",
			Volume:           0.1,
			BuyPrice:         1,
			SellPrice:        1000,
			TargetLowestSell: 1000,
			BuySystemID:      1,
			BuySystemName:    "Src",
			BuyRegionID:      10,
			BuyRegionName:    "Source",
			SellSystemID:     2,
			SellSystemName:   "Dst",
			SellRegionID:     20,
			SellRegionName:   "Target",
			UnitsToBuy:       1,
			SellOrderRemain:  1,
			BuyOrderRemain:   10,
			SellJumps:        1,
		},
	}

	hubs, totalItems, _, _ := scanner.BuildRegionalDayTrader(
		ScanParams{SellOrderMode: true},
		flips,
		nil,
		nil,
	)
	if len(hubs) != 1 || totalItems != 1 {
		t.Fatalf("unexpected result shape: hubs=%d items=%d", len(hubs), totalItems)
	}
	item := hubs[0].Items[0]
	if item.ROIPeriod > 0.2 {
		t.Fatalf("sell-order period ROI = %.4f, expected strict floor dampening", item.ROIPeriod)
	}
}

func TestBuildRegionalDayTrader_RejectsUntrustedMicroSourceWithoutHistory(t *testing.T) {
	scanner := &Scanner{
		SDE: &sde.Data{
			Systems: map[int32]*sde.SolarSystem{
				1: {ID: 1, Name: "Src", RegionID: 10, Security: 0.8},
				2: {ID: 2, Name: "Dst", RegionID: 20, Security: 0.9},
			},
			Regions: map[int32]*sde.Region{
				10: {ID: 10, Name: "Source"},
				20: {ID: 20, Name: "Target"},
			},
		},
	}

	flips := []FlipResult{
		{
			TypeID:           7003,
			TypeName:         "Outlier Micro Source",
			Volume:           0.1,
			BuyPrice:         0.99,
			ExpectedBuyPrice: 0.99,
			SellPrice:        150400,
			TargetLowestSell: 3000000,
			BuySystemID:      1,
			BuySystemName:    "Src",
			BuyRegionID:      10,
			BuyRegionName:    "Source",
			SellSystemID:     2,
			SellSystemName:   "Dst",
			SellRegionID:     20,
			SellRegionName:   "Target",
			UnitsToBuy:       1,
			SellOrderRemain:  10,
			BuyOrderRemain:   10,
			SellJumps:        1,
		},
	}

	hubs, totalItems, _, _ := scanner.BuildRegionalDayTrader(ScanParams{}, flips, nil, nil)
	if len(hubs) != 0 || totalItems != 0 {
		t.Fatalf("expected micro-source outlier to be rejected, got hubs=%d items=%d", len(hubs), totalItems)
	}
}

func TestBuildRegionalDayTrader_RejectsExtremeMicroSourceEvenWithHistory(t *testing.T) {
	now := time.Now().UTC()
	sourceHistory := make([]esi.HistoryEntry, 0, 14)
	targetHistory := make([]esi.HistoryEntry, 0, 14)
	for i := 0; i < 14; i++ {
		sourceHistory = append(sourceHistory, esi.HistoryEntry{
			Date:    now.AddDate(0, 0, -(14 - i)).Format("2006-01-02"),
			Average: 1.1,
			Volume:  50,
		})
		targetHistory = append(targetHistory, esi.HistoryEntry{
			Date:    now.AddDate(0, 0, -(14 - i)).Format("2006-01-02"),
			Average: 150000,
			Volume:  20,
		})
	}

	hp := &testHistoryProvider{
		store: map[string][]esi.HistoryEntry{
			"10:7004": sourceHistory,
			"20:7004": targetHistory,
		},
	}
	scanner := &Scanner{
		SDE: &sde.Data{
			Systems: map[int32]*sde.SolarSystem{
				1: {ID: 1, Name: "Src", RegionID: 10, Security: 0.8},
				2: {ID: 2, Name: "Dst", RegionID: 20, Security: 0.9},
			},
			Regions: map[int32]*sde.Region{
				10: {ID: 10, Name: "Source"},
				20: {ID: 20, Name: "Target"},
			},
		},
		History: hp,
	}

	flips := []FlipResult{
		{
			TypeID:           7004,
			TypeName:         "Extreme Micro Source With History",
			Volume:           0.1,
			BuyPrice:         0.99,
			ExpectedBuyPrice: 0.99,
			SellPrice:        150400,
			TargetLowestSell: 3000000,
			BuySystemID:      1,
			BuySystemName:    "Src",
			BuyRegionID:      10,
			BuyRegionName:    "Source",
			SellSystemID:     2,
			SellSystemName:   "Dst",
			SellRegionID:     20,
			SellRegionName:   "Target",
			UnitsToBuy:       1,
			SellOrderRemain:  10,
			BuyOrderRemain:   10,
			SellJumps:        1,
		},
	}

	hubs, totalItems, _, _ := scanner.BuildRegionalDayTrader(ScanParams{}, flips, nil, nil)
	if len(hubs) != 0 || totalItems != 0 {
		t.Fatalf("expected extreme micro-source to be rejected even with history, got hubs=%d items=%d", len(hubs), totalItems)
	}
}

func TestBuildRegionalDayTrader_MaxDOSBehavior(t *testing.T) {
	scanner := &Scanner{
		SDE: &sde.Data{
			Systems: map[int32]*sde.SolarSystem{
				1: {ID: 1, Name: "Src", RegionID: 10, Security: 0.8},
				2: {ID: 2, Name: "Dst", RegionID: 20, Security: 0.9},
			},
			Regions: map[int32]*sde.Region{
				10: {ID: 10, Name: "Source"},
				20: {ID: 20, Name: "Target"},
			},
		},
	}

	flips := []FlipResult{
		{
			TypeID:           7005,
			TypeName:         "Very Slow Item",
			BuyPrice:         100,
			SellPrice:        160,
			BuySystemID:      1,
			BuySystemName:    "Src",
			BuyRegionID:      10,
			BuyRegionName:    "Source",
			SellSystemID:     2,
			SellSystemName:   "Dst",
			SellRegionID:     20,
			SellRegionName:   "Target",
			UnitsToBuy:       1,
			SellOrderRemain:  1000,
			S2BPerDay:        1,
			TargetSellSupply: 1000, // DOS 1000
			SellJumps:        1,
		},
	}

	t.Run("max_dos_zero_disables_filter", func(t *testing.T) {
		hubs, totalItems, _, _ := scanner.BuildRegionalDayTrader(ScanParams{}, flips, nil, nil)
		if len(hubs) != 1 || totalItems != 1 {
			t.Fatalf("expected row to pass when max_dos=0, got hubs=%d items=%d", len(hubs), totalItems)
		}
	})

	t.Run("positive_max_dos_filters_high_dos", func(t *testing.T) {
		hubs, totalItems, _, _ := scanner.BuildRegionalDayTrader(
			ScanParams{MaxDOS: 365},
			flips,
			nil,
			nil,
		)
		if len(hubs) != 0 || totalItems != 0 {
			t.Fatalf("expected row to be filtered by max_dos=365, got hubs=%d items=%d", len(hubs), totalItems)
		}
	})
}

// ── extractLastNAvgPrices ────────────────────────────────────────────────────

func TestExtractLastNAvgPrices_Empty(t *testing.T) {
	result := extractLastNAvgPrices(nil, 14)
	if result != nil {
		t.Fatalf("expected nil for empty entries, got %v", result)
	}
}

func TestExtractLastNAvgPrices_ZeroN(t *testing.T) {
	entries := []esi.HistoryEntry{{Date: "2025-01-01", Average: 100}}
	result := extractLastNAvgPrices(entries, 0)
	if result != nil {
		t.Fatalf("expected nil for n=0, got %v", result)
	}
}

func TestExtractLastNAvgPrices_FewerThanN(t *testing.T) {
	entries := []esi.HistoryEntry{
		{Date: "2025-01-01", Average: 100},
		{Date: "2025-01-03", Average: 300},
		{Date: "2025-01-02", Average: 200},
	}
	result := extractLastNAvgPrices(entries, 14)
	// All 3 entries returned, sorted chronologically
	if len(result) != 3 {
		t.Fatalf("len = %d, want 3", len(result))
	}
	if result[0] != 100 || result[1] != 200 || result[2] != 300 {
		t.Fatalf("wrong order: %v", result)
	}
}

func TestExtractLastNAvgPrices_MoreThanN(t *testing.T) {
	entries := make([]esi.HistoryEntry, 20)
	for i := 0; i < 20; i++ {
		entries[i] = esi.HistoryEntry{
			Date:    time.Now().AddDate(0, 0, -(20 - i)).Format("2006-01-02"),
			Average: float64(i + 1),
		}
	}
	result := extractLastNAvgPrices(entries, 5)
	if len(result) != 5 {
		t.Fatalf("len = %d, want 5", len(result))
	}
	// Last 5 entries: values 16,17,18,19,20
	if result[0] != 16 || result[4] != 20 {
		t.Fatalf("wrong values: %v", result)
	}
}

func TestBuildRegionalDayTrader_UsesWeightedHubDOS(t *testing.T) {
	scanner := &Scanner{
		SDE: &sde.Data{
			Systems: map[int32]*sde.SolarSystem{
				1: {ID: 1, Name: "Hub", RegionID: 10, Security: 0.8},
				2: {ID: 2, Name: "Dst", RegionID: 20, Security: 0.9},
			},
			Regions: map[int32]*sde.Region{
				10: {ID: 10, Name: "Source"},
				20: {ID: 20, Name: "Target"},
			},
		},
	}

	flips := []FlipResult{
		{
			TypeID:           101,
			TypeName:         "Low-capital high DOS",
			BuyPrice:         10,
			SellPrice:        12,
			BuySystemID:      1,
			BuySystemName:    "Hub",
			BuyRegionID:      10,
			BuyRegionName:    "Source",
			SellSystemID:     2,
			SellSystemName:   "Dst",
			SellRegionID:     20,
			SellRegionName:   "Target",
			UnitsToBuy:       1,
			SellOrderRemain:  100,
			S2BPerDay:        1,
			TargetSellSupply: 100, // DOS 100
			SellJumps:        1,
		},
		{
			TypeID:           102,
			TypeName:         "High-capital low DOS",
			BuyPrice:         100,
			SellPrice:        120,
			BuySystemID:      1,
			BuySystemName:    "Hub",
			BuyRegionID:      10,
			BuyRegionName:    "Source",
			SellSystemID:     2,
			SellSystemName:   "Dst",
			SellRegionID:     20,
			SellRegionName:   "Target",
			UnitsToBuy:       50,
			SellOrderRemain:  50,
			S2BPerDay:        50,
			TargetSellSupply: 50, // DOS 1
			SellJumps:        1,
		},
	}

	hubs, totalItems, _, _ := scanner.BuildRegionalDayTrader(ScanParams{}, flips, nil, nil)
	if len(hubs) != 1 || totalItems != 2 {
		t.Fatalf("unexpected result shape: hubs=%d items=%d", len(hubs), totalItems)
	}

	weightedExpected := ((100.0 * 10.0) + (1.0 * 5000.0)) / (10.0 + 5000.0)
	if math.Abs(hubs[0].TargetDOS-weightedExpected) > 1e-6 {
		t.Fatalf("hub target_dos = %.6f, want weighted %.6f", hubs[0].TargetDOS, weightedExpected)
	}
}

func TestFlattenRegionalDayHubs_MapsToFlipRows(t *testing.T) {
	hubs := []RegionalDayTradeHub{
		{
			SourceSystemID:   30000142,
			SourceSystemName: "Jita",
			SourceRegionID:   10000002,
			SourceRegionName: "The Forge",
			Security:         0.9,
			Items: []RegionalDayTradeItem{
				{
					TypeID:             34,
					TypeName:           "Tritanium",
					SourceSystemID:     30000142,
					SourceSystemName:   "Jita",
					SourceStationName:  "Jita 4-4",
					SourceLocationID:   60003760,
					SourceRegionID:     10000002,
					SourceRegionName:   "The Forge",
					TargetSystemID:     30002187,
					TargetSystemName:   "Amarr",
					TargetStationName:  "Amarr VIII",
					TargetLocationID:   60008494,
					TargetRegionID:     10000043,
					TargetRegionName:   "Domain",
					PurchaseUnits:      10,
					SourceUnits:        42,
					TargetDemandPerDay: 5,
					TargetSupplyUnits:  25,
					TargetDOS:          5,
					Assets:             2,
					ActiveOrders:       3,
					SourceAvgPrice:     100,
					TargetNowPrice:     120,
					TargetPeriodPrice:  118,
					TargetNowProfit:    180,
					TargetPeriodProfit: 160,
					ROINow:             18,
					ROIPeriod:          16,
					CapitalRequired:    1000,
					ItemVolume:         0.01,
					ShippingCost:       20,
					Jumps:              4,
					MarginNow:          20,
					MarginPeriod:       18,
					CategoryID:         8,
					GroupID:            12,
					GroupName:          "Charges",
					TradeScore:         77,
					TargetPriceHistory: []float64{100, 101, 102},
					TargetLowestSell:   119,
				},
			},
		},
	}

	rows := FlattenRegionalDayHubs(hubs)
	if len(rows) != 1 {
		t.Fatalf("rows len = %d, want 1", len(rows))
	}

	row := rows[0]
	if row.TypeID != 34 || row.TypeName != "Tritanium" {
		t.Fatalf("row type mismatch: %+v", row)
	}
	if row.BuySystemName != "Jita" || row.SellSystemName != "Amarr" {
		t.Fatalf("row system mapping mismatch: buy=%s sell=%s", row.BuySystemName, row.SellSystemName)
	}
	if row.UnitsToBuy != 10 || row.TotalProfit != 180 || row.RealProfit != 160 {
		t.Fatalf("row profit/units mismatch: units=%d now=%.2f period=%.2f", row.UnitsToBuy, row.TotalProfit, row.RealProfit)
	}
	if row.DayTargetDOS != 5 || row.DayTradeScore != 77 {
		t.Fatalf("row day metrics mismatch: dos=%.2f score=%.2f", row.DayTargetDOS, row.DayTradeScore)
	}
	if len(row.DayPriceHistory) != 3 {
		t.Fatalf("row history len = %d, want 3", len(row.DayPriceHistory))
	}
}

func TestExtractLastNAvgPrices_UnsortedInput(t *testing.T) {
	entries := []esi.HistoryEntry{
		{Date: "2025-03-01", Average: 300},
		{Date: "2025-01-01", Average: 100},
		{Date: "2025-02-01", Average: 200},
	}
	result := extractLastNAvgPrices(entries, 2)
	// Last 2 sorted: 2025-02-01=200 and 2025-03-01=300
	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}
	if result[0] != 200 || result[1] != 300 {
		t.Fatalf("wrong values: %v (expected [200 300])", result)
	}
}

func TestExtractLastNAvgPrices_SanitizesNaN(t *testing.T) {
	entries := []esi.HistoryEntry{
		{Date: "2025-01-01", Average: math.NaN()},
		{Date: "2025-01-02", Average: 500},
	}
	result := extractLastNAvgPrices(entries, 14)
	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}
	if math.IsNaN(result[0]) {
		t.Fatalf("NaN not sanitized in result[0]")
	}
}

// ── CategoryIDs filter ───────────────────────────────────────────────────────

func TestBuildRegionalDayTrader_CategoryFilterExcludes(t *testing.T) {
	scanner := &Scanner{
		SDE: &sde.Data{
			Systems: map[int32]*sde.SolarSystem{
				1: {ID: 1, Name: "Src", RegionID: 10, Security: 0.8},
				2: {ID: 2, Name: "Dst", RegionID: 20, Security: 0.9},
			},
			Regions: map[int32]*sde.Region{
				10: {ID: 10, Name: "Source"},
				20: {ID: 20, Name: "Target"},
			},
			Types: map[int32]*sde.ItemType{
				// TypeID 1 = Ships (cat 6), TypeID 2 = Modules (cat 7)
				1: {ID: 1, Name: "Rifter", GroupID: 25, CategoryID: 6},
				2: {ID: 2, Name: "Shield Extender", GroupID: 55, CategoryID: 7},
			},
			Groups: map[int32]*sde.ItemGroup{
				25: {ID: 25, Name: "Frigate", CategoryID: 6},
				55: {ID: 55, Name: "Shield", CategoryID: 7},
			},
		},
	}

	flips := []FlipResult{
		{TypeID: 1, TypeName: "Rifter", Volume: 50, BuyPrice: 1000, SellPrice: 2000,
			BuySystemID: 1, BuySystemName: "Src", BuyRegionID: 10, BuyRegionName: "Source",
			SellSystemID: 2, SellSystemName: "Dst", SellRegionID: 20, SellRegionName: "Target",
			UnitsToBuy: 5, SellOrderRemain: 100, SellJumps: 3},
		{TypeID: 2, TypeName: "Shield Extender", Volume: 1, BuyPrice: 500, SellPrice: 1200,
			BuySystemID: 1, BuySystemName: "Src", BuyRegionID: 10, BuyRegionName: "Source",
			SellSystemID: 2, SellSystemName: "Dst", SellRegionID: 20, SellRegionName: "Target",
			UnitsToBuy: 10, SellOrderRemain: 100, SellJumps: 3},
	}

	// Filter: only Modules (cat 7) — should exclude Ships (cat 6)
	hubs, totalItems, _, _ := scanner.BuildRegionalDayTrader(
		ScanParams{CategoryIDs: []int32{7}},
		flips, nil, nil,
	)
	if totalItems != 1 {
		t.Fatalf("totalItems = %d, want 1 (only modules)", totalItems)
	}
	if hubs[0].Items[0].TypeID != 2 {
		t.Fatalf("expected TypeID 2 (Shield Extender), got %d", hubs[0].Items[0].TypeID)
	}
}

func TestBuildRegionalDayTrader_CategoryFilterEmpty_AllowsAll(t *testing.T) {
	scanner := &Scanner{
		SDE: &sde.Data{
			Systems: map[int32]*sde.SolarSystem{
				1: {ID: 1, Name: "Src", RegionID: 10, Security: 0.8},
				2: {ID: 2, Name: "Dst", RegionID: 20, Security: 0.9},
			},
			Regions: map[int32]*sde.Region{
				10: {ID: 10, Name: "Source"},
				20: {ID: 20, Name: "Target"},
			},
			Types: map[int32]*sde.ItemType{
				1: {ID: 1, Name: "Rifter", GroupID: 25, CategoryID: 6},
				2: {ID: 2, Name: "Shield Extender", GroupID: 55, CategoryID: 7},
			},
			Groups: map[int32]*sde.ItemGroup{
				25: {ID: 25, Name: "Frigate", CategoryID: 6},
				55: {ID: 55, Name: "Shield", CategoryID: 7},
			},
		},
	}

	flips := []FlipResult{
		{TypeID: 1, TypeName: "Rifter", Volume: 50, BuyPrice: 1000, SellPrice: 2000,
			BuySystemID: 1, BuySystemName: "Src", BuyRegionID: 10, BuyRegionName: "Source",
			SellSystemID: 2, SellSystemName: "Dst", SellRegionID: 20, SellRegionName: "Target",
			UnitsToBuy: 5, SellOrderRemain: 100, SellJumps: 3},
		{TypeID: 2, TypeName: "Shield Extender", Volume: 1, BuyPrice: 500, SellPrice: 1200,
			BuySystemID: 1, BuySystemName: "Src", BuyRegionID: 10, BuyRegionName: "Source",
			SellSystemID: 2, SellSystemName: "Dst", SellRegionID: 20, SellRegionName: "Target",
			UnitsToBuy: 10, SellOrderRemain: 100, SellJumps: 3},
	}

	// No category filter: all items pass
	_, totalItems, _, _ := scanner.BuildRegionalDayTrader(
		ScanParams{},
		flips, nil, nil,
	)
	if totalItems != 2 {
		t.Fatalf("totalItems = %d, want 2 (no filter)", totalItems)
	}
}
