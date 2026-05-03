package engine

import (
	"testing"
	"time"

	"eve-flipper/internal/esi"
)

func TestBuildFlipBacktest_ClosedAndOpenLedger(t *testing.T) {
	row := FlipResult{
		TypeID:           34,
		TypeName:         "Tritanium",
		BuyRegionID:      1,
		SellRegionID:     2,
		BuyPrice:         10,
		SellPrice:        12,
		ExpectedBuyPrice: 10,
		FilledQty:        100,
	}
	history := map[int32][]esi.HistoryEntry{
		1: {
			{Date: "2026-01-01", Average: 10, Volume: 1000},
			{Date: "2026-01-02", Average: 11, Volume: 1000},
			{Date: "2026-01-03", Average: 12, Volume: 1000},
		},
		2: {
			{Date: "2026-01-01", Average: 12, Volume: 1000},
			{Date: "2026-01-02", Average: 14, Volume: 50},
			{Date: "2026-01-03", Average: 16, Volume: 1000},
		},
	}

	result := BuildFlipBacktest(
		[]FlipResult{row},
		FlipBacktestParams{HoldDays: 1, WindowDays: 3, MaxRows: 10},
		func(regionID int32, typeID int32) []esi.HistoryEntry {
			return history[regionID]
		},
	)

	if result.Summary.RowsTested != 1 {
		t.Fatalf("rows tested = %d, want 1", result.Summary.RowsTested)
	}
	if result.Summary.ClosedTrades != 2 || result.Summary.OpenTrades != 1 {
		t.Fatalf("closed/open = %d/%d, want 2/1", result.Summary.ClosedTrades, result.Summary.OpenTrades)
	}
	if result.Summary.RealizedPnL != 900 {
		t.Fatalf("realized pnl = %v, want 900", result.Summary.RealizedPnL)
	}
	if result.Summary.MTMPnL != 400 {
		t.Fatalf("mtm pnl = %v, want 400", result.Summary.MTMPnL)
	}
	if len(result.Items) != 1 || result.Items[0].FillRate < 66 || result.Items[0].FillRate > 67 {
		t.Fatalf("item fill rate = %#v, want one item near 66.7%%", result.Items)
	}
}

func TestBuildFlipBacktest_TruncatesRows(t *testing.T) {
	rows := []FlipResult{{TypeID: 34, SellRegionID: 2, BuyPrice: 10, FilledQty: 1}, {TypeID: 35, SellRegionID: 2, BuyPrice: 10, FilledQty: 1}}
	result := BuildFlipBacktest(rows, FlipBacktestParams{MaxRows: 1}, func(regionID int32, typeID int32) []esi.HistoryEntry {
		return []esi.HistoryEntry{
			{Date: "2026-01-01", Average: 10, Volume: 10},
			{Date: "2026-01-02", Average: 11, Volume: 10},
		}
	})
	if len(result.Warnings) == 0 {
		t.Fatal("expected truncation warning")
	}
	if result.Summary.RowsTested != 1 {
		t.Fatalf("rows tested = %d, want 1", result.Summary.RowsTested)
	}
}

func TestBuildFlipBacktest_AdvancedParamsAndEquityCurve(t *testing.T) {
	row := FlipResult{
		TypeID:           34,
		TypeName:         "Tritanium",
		BuyRegionID:      1,
		SellRegionID:     2,
		BuyPrice:         10,
		SellPrice:        20,
		ExpectedBuyPrice: 10,
		FilledQty:        100,
	}
	history := map[int32][]esi.HistoryEntry{
		1: {
			{Date: "2026-01-01", Average: 99, Volume: 1000},
			{Date: "2026-01-02", Average: 99, Volume: 1000},
			{Date: "2026-01-03", Average: 99, Volume: 1000},
		},
		2: {
			{Date: "2026-01-01", Average: 20, Volume: 100},
			{Date: "2026-01-02", Average: 30, Volume: 100},
			{Date: "2026-01-03", Average: 40, Volume: 100},
		},
	}

	result := BuildFlipBacktest(
		[]FlipResult{row},
		FlipBacktestParams{
			HoldDays:            1,
			WindowDays:          3,
			MaxRows:             10,
			EntrySpacingDays:    2,
			QuantityMode:        "fixed",
			FixedQuantity:       20,
			BuyPriceSource:      "scan",
			BuyPriceMarkupPct:   10,
			SellPriceHaircutPct: 10,
			VolumeFillFraction:  50,
			SkipUnfillable:      true,
			ExcludeOpenTrades:   true,
		},
		func(regionID int32, typeID int32) []esi.HistoryEntry {
			return history[regionID]
		},
	)

	if result.Summary.Trades != 1 || result.Summary.OpenTrades != 0 {
		t.Fatalf("trades/open = %d/%d, want 1/0", result.Summary.Trades, result.Summary.OpenTrades)
	}
	if result.Ledger[0].Quantity != 20 {
		t.Fatalf("quantity = %d, want 20", result.Ledger[0].Quantity)
	}
	if result.Ledger[0].BuyPrice != 11 {
		t.Fatalf("buy price = %v, want 11", result.Ledger[0].BuyPrice)
	}
	if result.Ledger[0].SellPrice != 27 {
		t.Fatalf("sell price = %v, want 27", result.Ledger[0].SellPrice)
	}
	if result.Summary.RealizedPnL != 320 {
		t.Fatalf("realized pnl = %v, want 320", result.Summary.RealizedPnL)
	}
	if len(result.Equity) != 1 || result.Equity[0].Equity != 320 || result.Equity[0].Realized != 320 {
		t.Fatalf("equity = %#v, want one 320/320 point", result.Equity)
	}
}

func TestBuildFlipBacktest_NonOverlappingEntries(t *testing.T) {
	row := FlipResult{
		TypeID:           34,
		TypeName:         "Tritanium",
		BuyRegionID:      1,
		SellRegionID:     2,
		BuyPrice:         10,
		SellPrice:        12,
		ExpectedBuyPrice: 10,
		FilledQty:        1,
	}
	history := []esi.HistoryEntry{
		{Date: "2026-01-01", Average: 10, Volume: 100},
		{Date: "2026-01-02", Average: 11, Volume: 100},
		{Date: "2026-01-03", Average: 12, Volume: 100},
		{Date: "2026-01-04", Average: 13, Volume: 100},
		{Date: "2026-01-05", Average: 14, Volume: 100},
	}

	result := BuildFlipBacktest(
		[]FlipResult{row},
		FlipBacktestParams{
			HoldDays:          2,
			WindowDays:        5,
			EntrySpacingDays:  1,
			NonOverlapping:    true,
			ExcludeOpenTrades: true,
		},
		func(regionID int32, typeID int32) []esi.HistoryEntry {
			return history
		},
	)

	if result.Summary.Trades != 2 {
		t.Fatalf("trades = %d, want 2 non-overlapping entries", result.Summary.Trades)
	}
	if result.Ledger[0].EntryDate != "2026-01-01" || result.Ledger[1].EntryDate != "2026-01-03" {
		t.Fatalf("entry dates = %s/%s, want 2026-01-01/2026-01-03", result.Ledger[0].EntryDate, result.Ledger[1].EntryDate)
	}
}

func TestBuildFlipBacktest_InstantFlipMode(t *testing.T) {
	row := FlipResult{
		TypeID:           34,
		TypeName:         "Tritanium",
		BuyRegionID:      1,
		SellRegionID:     2,
		BuyPrice:         10,
		SellPrice:        12,
		ExpectedBuyPrice: 10,
		FilledQty:        10,
	}
	history := map[int32][]esi.HistoryEntry{
		1: {
			{Date: "2026-01-01", Average: 10, Volume: 100},
			{Date: "2026-01-02", Average: 10, Volume: 100},
			{Date: "2026-01-03", Average: 10, Volume: 100},
			{Date: "2026-01-04", Average: 10, Volume: 100},
		},
		2: {
			{Date: "2026-01-01", Average: 12, Volume: 100},
			{Date: "2026-01-02", Average: 9, Volume: 100},
			{Date: "2026-01-03", Average: 13, Volume: 100},
			{Date: "2026-01-04", Average: 14, Volume: 100},
		},
	}

	result := BuildFlipBacktest(
		[]FlipResult{row},
		FlipBacktestParams{
			StrategyMode:       "instant_flip",
			InstantPriceMode:   "history_pair",
			WindowDays:         4,
			EntrySpacingDays:   1,
			TravelCooldownDays: 2,
			MinROIPercent:      0,
		},
		func(regionID int32, typeID int32) []esi.HistoryEntry {
			return history[regionID]
		},
	)

	if result.Summary.StrategyMode != "instant_flip" {
		t.Fatalf("strategy = %q, want instant_flip", result.Summary.StrategyMode)
	}
	if result.Summary.Trades != 2 || result.Summary.OpenTrades != 0 {
		t.Fatalf("trades/open = %d/%d, want 2/0", result.Summary.Trades, result.Summary.OpenTrades)
	}
	if result.Ledger[0].EntryDate != "2026-01-01" || result.Ledger[0].ExitDate != "2026-01-01" {
		t.Fatalf("first trade dates = %s/%s, want same-day 2026-01-01", result.Ledger[0].EntryDate, result.Ledger[0].ExitDate)
	}
	if result.Ledger[1].EntryDate != "2026-01-03" || result.Ledger[1].ExitDate != "2026-01-03" {
		t.Fatalf("second trade dates = %s/%s, want same-day 2026-01-03", result.Ledger[1].EntryDate, result.Ledger[1].ExitDate)
	}
	if result.Summary.RealizedPnL != 50 {
		t.Fatalf("realized pnl = %v, want 50", result.Summary.RealizedPnL)
	}
}

func TestBuildFlipBacktest_InstantFlipScanSpreadMode(t *testing.T) {
	row := FlipResult{
		TypeID:           34,
		TypeName:         "Tritanium",
		BuyRegionID:      1,
		SellRegionID:     2,
		BuyPrice:         10,
		SellPrice:        15,
		ExpectedBuyPrice: 10,
		FilledQty:        10,
	}
	targetHistory := []esi.HistoryEntry{
		{Date: "2026-01-01", Average: 9, Volume: 100},
		{Date: "2026-01-02", Average: 9, Volume: 100},
		{Date: "2026-01-03", Average: 9, Volume: 100},
	}

	result := BuildFlipBacktest(
		[]FlipResult{row},
		FlipBacktestParams{
			StrategyMode:       "instant_flip",
			InstantPriceMode:   "scan_spread",
			WindowDays:         3,
			EntrySpacingDays:   1,
			TravelCooldownDays: 1,
			MinROIPercent:      0,
		},
		func(regionID int32, typeID int32) []esi.HistoryEntry {
			if regionID == 2 {
				return targetHistory
			}
			return nil
		},
	)

	if result.Summary.Trades != 3 {
		t.Fatalf("trades = %d, want 3 scan-spread flips", result.Summary.Trades)
	}
	if result.Summary.RealizedPnL != 150 {
		t.Fatalf("realized pnl = %v, want 150", result.Summary.RealizedPnL)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", result.Warnings)
	}
}

func TestBuildOrderBookReplayBacktest_UsesRecordedVWAPDepth(t *testing.T) {
	now := time.Now().UTC().Add(-time.Hour)
	row := FlipResult{
		TypeID:         34,
		TypeName:       "Tritanium",
		BuyRegionID:    1,
		SellRegionID:   2,
		BuyLocationID:  100,
		SellLocationID: 200,
		BuyPrice:       5,
		SellPrice:      8,
		FilledQty:      10,
	}

	result := BuildOrderBookReplayBacktest(
		[]FlipResult{row},
		FlipBacktestParams{
			WindowDays:           1,
			MaxRows:              10,
			QuantityMode:         "scan",
			VolumeFillFraction:   100,
			OrderBookMaxAgeMin:   5,
			OrderBookCooldownMin: 1,
		},
		func(filter OrderBookReplayFilter) ([]OrderBookReplayBook, error) {
			switch {
			case filter.RegionID == 1 && filter.Side == "sell":
				return []OrderBookReplayBook{{
					SnapshotID: 1,
					CapturedAt: now,
					Levels: []OrderBookReplayLevel{
						{Price: 5, VolumeRemain: 5},
						{Price: 6, VolumeRemain: 10},
					},
				}}, nil
			case filter.RegionID == 2 && filter.Side == "buy":
				return []OrderBookReplayBook{{
					SnapshotID: 2,
					CapturedAt: now.Add(time.Minute),
					Levels: []OrderBookReplayLevel{
						{Price: 8, VolumeRemain: 10},
					},
				}}, nil
			default:
				return nil, nil
			}
		},
	)

	if result.Summary.Trades != 1 {
		t.Fatalf("trades = %d, want 1; warnings=%v", result.Summary.Trades, result.Warnings)
	}
	trade := result.Ledger[0]
	if trade.BuyPrice != 5.5 || trade.SellPrice != 8 {
		t.Fatalf("prices = buy %v sell %v, want 5.5/8", trade.BuyPrice, trade.SellPrice)
	}
	if trade.PnL != 25 {
		t.Fatalf("pnl = %v, want 25", trade.PnL)
	}
	if result.Summary.DataSource != "recorded_orderbook" {
		t.Fatalf("data source = %q, want recorded_orderbook", result.Summary.DataSource)
	}
}

func TestBuildOrderBookReplayBacktest_RouteTimeCooldownSkipsEarlyReentry(t *testing.T) {
	now := time.Now().UTC().Add(-2 * time.Hour)
	row := FlipResult{
		TypeID:         34,
		TypeName:       "Tritanium",
		Volume:         10,
		BuyRegionID:    1,
		SellRegionID:   2,
		BuyLocationID:  100,
		SellLocationID: 200,
		SellJumps:      5,
		FilledQty:      10,
	}

	result := BuildOrderBookReplayBacktest(
		[]FlipResult{row},
		FlipBacktestParams{
			WindowDays:          1,
			MaxRows:             10,
			QuantityMode:        "scan",
			VolumeFillFraction:  100,
			OrderBookMaxAgeMin:  5,
			CooldownMode:        "route_time",
			CargoCapacity:       50,
			RouteMinutesPerJump: 5,
			RouteDockMinutes:    0,
			RouteSafetyMult:     1,
		},
		func(filter OrderBookReplayFilter) ([]OrderBookReplayBook, error) {
			switch filter.Side {
			case "sell":
				return []OrderBookReplayBook{
					{SnapshotID: 1, CapturedAt: now, Levels: []OrderBookReplayLevel{{Price: 5, VolumeRemain: 10}}},
					{SnapshotID: 3, CapturedAt: now.Add(30 * time.Minute), Levels: []OrderBookReplayLevel{{Price: 5, VolumeRemain: 10}}},
				}, nil
			case "buy":
				return []OrderBookReplayBook{
					{SnapshotID: 2, CapturedAt: now.Add(time.Minute), Levels: []OrderBookReplayLevel{{Price: 8, VolumeRemain: 10}}},
					{SnapshotID: 4, CapturedAt: now.Add(31 * time.Minute), Levels: []OrderBookReplayLevel{{Price: 8, VolumeRemain: 10}}},
				}, nil
			default:
				return nil, nil
			}
		},
	)

	if result.Summary.Trades != 1 {
		t.Fatalf("trades = %d, want 1 after route-time cooldown", result.Summary.Trades)
	}
	if result.Ledger[0].RouteTimeMin != 75 || result.Ledger[0].CargoTrips != 2 || result.Ledger[0].RouteJumps != 15 {
		t.Fatalf("route fields = minutes %v trips %d jumps %d", result.Ledger[0].RouteTimeMin, result.Ledger[0].CargoTrips, result.Ledger[0].RouteJumps)
	}
}

func TestBuildOrderBookReplayCoverageReportsPairs(t *testing.T) {
	now := time.Now().UTC().Add(-time.Hour)
	rows := []FlipResult{
		{TypeID: 34, TypeName: "Tritanium", BuyRegionID: 1, SellRegionID: 2, BuyLocationID: 100, SellLocationID: 200},
		{TypeID: 35, TypeName: "Pyerite", BuyRegionID: 1, SellRegionID: 2, BuyLocationID: 100, SellLocationID: 200},
	}

	result := BuildOrderBookReplayCoverage(rows, FlipBacktestParams{
		WindowDays:         1,
		MaxRows:            10,
		OrderBookMaxAgeMin: 5,
	}, func(filter OrderBookReplayFilter) ([]OrderBookReplayBook, error) {
		if filter.TypeID != 34 {
			return nil, nil
		}
		if filter.Side == "sell" {
			return []OrderBookReplayBook{{SnapshotID: 1, CapturedAt: now, Levels: []OrderBookReplayLevel{{Price: 5, VolumeRemain: 10}}}}, nil
		}
		return []OrderBookReplayBook{{SnapshotID: 2, CapturedAt: now.Add(time.Minute), Levels: []OrderBookReplayLevel{{Price: 8, VolumeRemain: 10}}}}, nil
	})

	if result.Summary.RowsTested != 2 || result.Summary.RowsReady != 1 || result.Summary.RowsMissingSource != 1 {
		t.Fatalf("coverage summary = %#v", result.Summary)
	}
	if result.Rows[0].Status != "ready" || result.Rows[1].Status != "missing_source" {
		t.Fatalf("coverage rows = %#v", result.Rows)
	}
}
