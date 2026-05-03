package engine

import (
	"fmt"
	"math"
	"sort"
	"time"
)

type OrderBookReplayFilter struct {
	RegionID   int32
	TypeID     int32
	LocationID int64
	Side       string
	From       time.Time
	To         time.Time
	Limit      int
}

type OrderBookReplayLevel struct {
	Price        float64
	VolumeRemain int64
}

type OrderBookReplayBook struct {
	SnapshotID int64
	CapturedAt time.Time
	Levels     []OrderBookReplayLevel
}

type OrderBookReplayGetter func(filter OrderBookReplayFilter) ([]OrderBookReplayBook, error)

type OrderBookReplayCoverageRow struct {
	TypeID        int32  `json:"type_id"`
	TypeName      string `json:"type_name"`
	Status        string `json:"status"`
	Reason        string `json:"reason"`
	SourceBooks   int    `json:"source_books"`
	TargetBooks   int    `json:"target_books"`
	PairedBooks   int    `json:"paired_books"`
	SourceDepth   int64  `json:"source_depth"`
	TargetDepth   int64  `json:"target_depth"`
	SourceLevels  int    `json:"source_levels"`
	TargetLevels  int    `json:"target_levels"`
	OldestCapture string `json:"oldest_capture"`
	NewestCapture string `json:"newest_capture"`
}

type OrderBookReplayCoverageSummary struct {
	RowsTested        int     `json:"rows_tested"`
	RowsReady         int     `json:"rows_ready"`
	RowsMissingSource int     `json:"rows_missing_source"`
	RowsMissingTarget int     `json:"rows_missing_target"`
	RowsNoPairs       int     `json:"rows_no_pairs"`
	RowsInvalidScope  int     `json:"rows_invalid_scope"`
	SourceBooks       int     `json:"source_books"`
	TargetBooks       int     `json:"target_books"`
	PairedBooks       int     `json:"paired_books"`
	SourceDepth       int64   `json:"source_depth"`
	TargetDepth       int64   `json:"target_depth"`
	ReadyPercent      float64 `json:"ready_percent"`
	OldestCapture     string  `json:"oldest_capture"`
	NewestCapture     string  `json:"newest_capture"`
	BacktestDays      int     `json:"backtest_days"`
	MaxAgeMinutes     int     `json:"max_age_minutes"`
}

type OrderBookReplayCoverageResult struct {
	Summary  OrderBookReplayCoverageSummary `json:"summary"`
	Rows     []OrderBookReplayCoverageRow   `json:"rows"`
	Warnings []string                       `json:"warnings,omitempty"`
}

type orderBookReplayDiag struct {
	SourceBooks int
	TargetBooks int
	Pairs       int
	NoPair      int
	NoQuantity  int
	Unfillable  int
	BelowROI    int
	Errors      int
	BestROI     float64
	HasBestROI  bool
}

func (d *orderBookReplayDiag) add(other orderBookReplayDiag) {
	d.SourceBooks += other.SourceBooks
	d.TargetBooks += other.TargetBooks
	d.Pairs += other.Pairs
	d.NoPair += other.NoPair
	d.NoQuantity += other.NoQuantity
	d.Unfillable += other.Unfillable
	d.BelowROI += other.BelowROI
	d.Errors += other.Errors
	if other.HasBestROI && (!d.HasBestROI || other.BestROI > d.BestROI) {
		d.BestROI = other.BestROI
		d.HasBestROI = true
	}
}

func (d *orderBookReplayDiag) observeROI(roi float64) {
	if !d.HasBestROI || roi > d.BestROI {
		d.BestROI = roi
		d.HasBestROI = true
	}
}

func (d orderBookReplayDiag) warning() string {
	best := "n/a"
	if d.HasBestROI {
		best = fmt.Sprintf("%.1f%%", sanitizeFloat(d.BestROI))
	}
	return fmt.Sprintf(
		"recorded orderbook found no trades: source_books=%d target_books=%d paired_books=%d no_pair=%d unfillable=%d below_min_roi=%d no_quantity=%d errors=%d best_roi=%s",
		d.SourceBooks,
		d.TargetBooks,
		d.Pairs,
		d.NoPair,
		d.Unfillable,
		d.BelowROI,
		d.NoQuantity,
		d.Errors,
		best,
	)
}

func BuildOrderBookReplayBacktest(rows []FlipResult, params FlipBacktestParams, getBooks OrderBookReplayGetter) FlipBacktestResult {
	params.StrategyMode = "instant_flip"
	params.InstantPriceMode = "recorded_orderbook"
	params = normalizeFlipBacktestParams(params)

	result := FlipBacktestResult{}
	if len(rows) == 0 || getBooks == nil {
		result.Summary = summarizeFlipBacktest(rows, nil, params)
		result.Warnings = append(result.Warnings, "recorded orderbook replay needs stored orderbook snapshots")
		return result
	}
	if len(rows) > params.MaxRows {
		rows = rows[:params.MaxRows]
		result.Warnings = append(result.Warnings, "rows truncated to max_rows")
	}

	buyCostMult, sellRevenueMult := tradeFeeMultipliers(tradeFeeInputs{
		SplitTradeFees:       params.SplitTradeFees,
		BrokerFeePercent:     params.BrokerFeePercent,
		SalesTaxPercent:      params.SalesTaxPercent,
		BuyBrokerFeePercent:  params.BuyBrokerFeePercent,
		SellBrokerFeePercent: params.SellBrokerFeePercent,
		BuySalesTaxPercent:   params.BuySalesTaxPercent,
		SellSalesTaxPercent:  params.SellSalesTaxPercent,
	})

	var diag orderBookReplayDiag
	for _, row := range rows {
		trades, rowDiag := backtestOrderBookReplayRow(row, params, buyCostMult, sellRevenueMult, getBooks)
		diag.add(rowDiag)
		if len(trades) == 0 {
			continue
		}
		result.Ledger = append(result.Ledger, trades...)
	}

	sort.Slice(result.Ledger, func(i, j int) bool {
		if result.Ledger[i].ExitDate == result.Ledger[j].ExitDate {
			if result.Ledger[i].EntryDate == result.Ledger[j].EntryDate {
				return result.Ledger[i].TypeID < result.Ledger[j].TypeID
			}
			return result.Ledger[i].EntryDate < result.Ledger[j].EntryDate
		}
		return result.Ledger[i].ExitDate < result.Ledger[j].ExitDate
	})

	result.Summary = summarizeFlipBacktest(rows, result.Ledger, params)
	result.Items = summarizeFlipBacktestItems(result.Ledger)
	result.Equity = buildFlipBacktestEquityCurve(result.Ledger)
	if len(result.Ledger) == 0 {
		if diag.SourceBooks == 0 && diag.TargetBooks == 0 {
			result.Warnings = append(result.Warnings, "recorded orderbook has no stored snapshots for these rows yet; ESI cannot backfill old order books, so run live scans first")
		}
		result.Warnings = append(result.Warnings, diag.warning())
	}
	return result
}

func BuildOrderBookReplayCoverage(rows []FlipResult, params FlipBacktestParams, getBooks OrderBookReplayGetter) OrderBookReplayCoverageResult {
	params.StrategyMode = "instant_flip"
	params.InstantPriceMode = "recorded_orderbook"
	params = normalizeFlipBacktestParams(params)

	result := OrderBookReplayCoverageResult{}
	if len(rows) == 0 || getBooks == nil {
		result.Summary.BacktestDays = params.WindowDays
		result.Summary.MaxAgeMinutes = params.OrderBookMaxAgeMin
		result.Warnings = append(result.Warnings, "recorded orderbook coverage needs rows and stored orderbook snapshots")
		return result
	}
	if len(rows) > params.MaxRows {
		rows = rows[:params.MaxRows]
		result.Warnings = append(result.Warnings, "rows truncated to max_rows")
	}

	now := time.Now().UTC()
	from := now.Add(-time.Duration(params.WindowDays) * 24 * time.Hour)
	maxAge := time.Duration(params.OrderBookMaxAgeMin) * time.Minute
	result.Rows = make([]OrderBookReplayCoverageRow, 0, len(rows))
	var oldest, newest time.Time

	for _, row := range rows {
		coverage := OrderBookReplayCoverageRow{
			TypeID:   row.TypeID,
			TypeName: row.TypeName,
			Status:   "ready",
		}
		if row.TypeID <= 0 || (row.BuyRegionID <= 0 && row.BuyLocationID <= 0) || (row.SellRegionID <= 0 && row.SellLocationID <= 0) {
			coverage.Status = "invalid_scope"
			coverage.Reason = "missing type, source, or target scope"
			result.Summary.RowsInvalidScope++
			result.Rows = append(result.Rows, coverage)
			continue
		}

		sourceBooks, sourceErr := getBooks(OrderBookReplayFilter{
			RegionID:   row.BuyRegionID,
			TypeID:     row.TypeID,
			LocationID: row.BuyLocationID,
			Side:       "sell",
			From:       from,
			To:         now,
			Limit:      2000,
		})
		targetBooks, targetErr := getBooks(OrderBookReplayFilter{
			RegionID:   row.SellRegionID,
			TypeID:     row.TypeID,
			LocationID: row.SellLocationID,
			Side:       "buy",
			From:       from,
			To:         now,
			Limit:      2000,
		})
		if sourceErr != nil || targetErr != nil {
			coverage.Status = "query_error"
			coverage.Reason = "failed to query stored orderbook snapshots"
			result.Rows = append(result.Rows, coverage)
			continue
		}

		sourceBooks = normalizeReplayBooks(sourceBooks, "sell")
		targetBooks = normalizeReplayBooks(targetBooks, "buy")
		coverage.SourceBooks = len(sourceBooks)
		coverage.TargetBooks = len(targetBooks)
		coverage.SourceDepth, coverage.SourceLevels = replayBooksDepth(sourceBooks)
		coverage.TargetDepth, coverage.TargetLevels = replayBooksDepth(targetBooks)
		coverage.PairedBooks = countReplayBookPairs(sourceBooks, targetBooks, maxAge)
		rowOldest, rowNewest := replayBooksTimeRange(sourceBooks, targetBooks)
		coverage.OldestCapture = replayTimeString(rowOldest)
		coverage.NewestCapture = replayTimeString(rowNewest)
		oldest, newest = mergeReplayTimeRange(oldest, newest, rowOldest, rowNewest)

		switch {
		case coverage.SourceBooks == 0:
			coverage.Status = "missing_source"
			coverage.Reason = "no recorded source sell book"
			result.Summary.RowsMissingSource++
		case coverage.TargetBooks == 0:
			coverage.Status = "missing_target"
			coverage.Reason = "no recorded target buy book"
			result.Summary.RowsMissingTarget++
		case coverage.PairedBooks == 0:
			coverage.Status = "no_pairs"
			coverage.Reason = "source and target books are outside max age"
			result.Summary.RowsNoPairs++
		default:
			result.Summary.RowsReady++
		}

		result.Summary.SourceBooks += coverage.SourceBooks
		result.Summary.TargetBooks += coverage.TargetBooks
		result.Summary.PairedBooks += coverage.PairedBooks
		result.Summary.SourceDepth += coverage.SourceDepth
		result.Summary.TargetDepth += coverage.TargetDepth
		result.Rows = append(result.Rows, coverage)
	}

	result.Summary.RowsTested = len(rows)
	result.Summary.BacktestDays = params.WindowDays
	result.Summary.MaxAgeMinutes = params.OrderBookMaxAgeMin
	result.Summary.OldestCapture = replayTimeString(oldest)
	result.Summary.NewestCapture = replayTimeString(newest)
	if result.Summary.RowsTested > 0 {
		result.Summary.ReadyPercent = sanitizeFloat(float64(result.Summary.RowsReady) / float64(result.Summary.RowsTested) * 100)
	}
	if result.Summary.SourceBooks == 0 && result.Summary.TargetBooks == 0 {
		result.Warnings = append(result.Warnings, "no recorded orderbook snapshots found for selected rows")
	} else if result.Summary.RowsReady == 0 {
		result.Warnings = append(result.Warnings, "recorded orderbook snapshots exist, but no source/target pair is close enough for replay")
	}
	return result
}

func backtestOrderBookReplayRow(
	row FlipResult,
	params FlipBacktestParams,
	buyCostMult float64,
	sellRevenueMult float64,
	getBooks OrderBookReplayGetter,
) ([]FlipBacktestTrade, orderBookReplayDiag) {
	var diag orderBookReplayDiag
	if row.TypeID <= 0 {
		return nil, diag
	}
	if (row.BuyRegionID <= 0 && row.BuyLocationID <= 0) || (row.SellRegionID <= 0 && row.SellLocationID <= 0) {
		diag.Errors++
		return nil, diag
	}

	now := time.Now().UTC()
	from := now.Add(-time.Duration(params.WindowDays) * 24 * time.Hour)
	sourceBooks, err := getBooks(OrderBookReplayFilter{
		RegionID:   row.BuyRegionID,
		TypeID:     row.TypeID,
		LocationID: row.BuyLocationID,
		Side:       "sell",
		From:       from,
		To:         now,
		Limit:      2000,
	})
	if err != nil {
		diag.Errors++
		return nil, diag
	}
	targetBooks, err := getBooks(OrderBookReplayFilter{
		RegionID:   row.SellRegionID,
		TypeID:     row.TypeID,
		LocationID: row.SellLocationID,
		Side:       "buy",
		From:       from,
		To:         now,
		Limit:      2000,
	})
	if err != nil {
		diag.Errors++
		return nil, diag
	}
	sourceBooks = normalizeReplayBooks(sourceBooks, "sell")
	targetBooks = normalizeReplayBooks(targetBooks, "buy")
	diag.SourceBooks = len(sourceBooks)
	diag.TargetBooks = len(targetBooks)
	if len(sourceBooks) == 0 || len(targetBooks) == 0 {
		return nil, diag
	}

	maxAge := time.Duration(params.OrderBookMaxAgeMin) * time.Minute
	manualCooldown := time.Duration(params.OrderBookCooldownMin) * time.Minute
	var out []FlipBacktestTrade
	var nextAllowed time.Time
	for _, sourceBook := range sourceBooks {
		if !nextAllowed.IsZero() && sourceBook.CapturedAt.Before(nextAllowed) {
			continue
		}
		targetBook, ok := nearestReplayBook(sourceBook.CapturedAt, targetBooks, maxAge)
		if !ok {
			diag.NoPair++
			continue
		}
		diag.Pairs++

		buyPriceMult := 1 + params.BuyPriceMarkupPct/100
		sellPriceMult := 1 - params.SellPriceHaircutPct/100
		if sellPriceMult < 0 {
			sellPriceMult = 0
		}
		firstAsk := firstReplayPrice(sourceBook.Levels, buyPriceMult)
		requestedQty := backtestTradeQuantity(row, params, firstAsk, buyCostMult)
		if requestedQty <= 0 {
			diag.NoQuantity++
			continue
		}

		buyFilled, _, sourceDepth := fillReplayLevels(sourceBook.Levels, requestedQty, buyPriceMult, params.VolumeFillFraction)
		sellFilled, _, targetDepth := fillReplayLevels(targetBook.Levels, requestedQty, sellPriceMult, params.VolumeFillFraction)
		actualQty := minInt32(requestedQty, minInt32(buyFilled, sellFilled))
		if params.QuantityMode == "budget" && params.BudgetISK > 0 && actualQty > 0 {
			budgetQty := maxAffordableReplayQty(sourceBook.Levels, actualQty, buyPriceMult, buyCostMult, params.VolumeFillFraction, params.BudgetISK)
			actualQty = minInt32(actualQty, budgetQty)
		}
		if actualQty <= 0 {
			diag.Unfillable++
			continue
		}

		fillable := actualQty == requestedQty
		if params.SkipUnfillable && !fillable {
			diag.Unfillable++
			continue
		}
		var buyGross, sellGross float64
		buyFilled, buyGross, sourceDepth = fillReplayLevels(sourceBook.Levels, actualQty, buyPriceMult, params.VolumeFillFraction)
		sellFilled, sellGross, targetDepth = fillReplayLevels(targetBook.Levels, actualQty, sellPriceMult, params.VolumeFillFraction)
		if buyFilled < actualQty || sellFilled < actualQty {
			diag.Unfillable++
			continue
		}

		buyCost := buyGross * buyCostMult
		sellRevenue := sellGross * sellRevenueMult
		pnl := sellRevenue - buyCost
		roi := 0.0
		if buyCost > 0 {
			roi = pnl / buyCost * 100
		}
		diag.observeROI(roi)
		if roi < params.MinROIPercent {
			diag.BelowROI++
			continue
		}

		exitAt := sourceBook.CapturedAt
		if targetBook.CapturedAt.After(exitAt) {
			exitAt = targetBook.CapturedAt
		}
		routeTime := RouteTimeEstimate{}
		cooldown := manualCooldown
		if params.CooldownMode == "route_time" {
			routeTime = estimateBacktestRouteTime(row, params, actualQty)
			cooldown = time.Duration(math.Ceil(routeTime.Minutes)) * time.Minute
		}
		tradeDate := exitAt.UTC().Format("2006-01-02")
		out = append(out, FlipBacktestTrade{
			TypeID:          row.TypeID,
			TypeName:        row.TypeName,
			EntryDate:       sourceBook.CapturedAt.UTC().Format("2006-01-02"),
			ExitDate:        tradeDate,
			Status:          "closed",
			Quantity:        actualQty,
			BuyPrice:        sanitizeFloat(buyGross / float64(actualQty)),
			SellPrice:       sanitizeFloat(sellGross / float64(actualQty)),
			BuyCost:         sanitizeFloat(buyCost),
			SellRevenue:     sanitizeFloat(sellRevenue),
			PnL:             sanitizeFloat(pnl),
			ROIPercent:      sanitizeFloat(roi),
			Fillable:        fillable,
			TargetVolume:    minInt64(sourceDepth, targetDepth),
			RouteTimeMin:    routeTime.Minutes,
			RouteJumps:      routeTime.Jumps,
			CargoTrips:      routeTime.Trips,
			RouteSafetyMult: routeTime.SafetyMult,
			RouteDanger:     routeTime.Danger,
			RouteKills:      routeTime.Kills,
		})
		nextAllowed = sourceBook.CapturedAt.Add(cooldown)
	}
	return out, diag
}

func normalizeReplayBooks(books []OrderBookReplayBook, side string) []OrderBookReplayBook {
	out := make([]OrderBookReplayBook, 0, len(books))
	for _, book := range books {
		if book.CapturedAt.IsZero() {
			continue
		}
		levels := normalizeReplayLevels(book.Levels, side)
		if len(levels) == 0 {
			continue
		}
		book.Levels = levels
		out = append(out, book)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CapturedAt.Equal(out[j].CapturedAt) {
			return out[i].SnapshotID < out[j].SnapshotID
		}
		return out[i].CapturedAt.Before(out[j].CapturedAt)
	})
	return out
}

func normalizeReplayLevels(levels []OrderBookReplayLevel, side string) []OrderBookReplayLevel {
	out := make([]OrderBookReplayLevel, 0, len(levels))
	for _, level := range levels {
		if level.Price <= 0 || math.IsNaN(level.Price) || math.IsInf(level.Price, 0) || level.VolumeRemain <= 0 {
			continue
		}
		out = append(out, level)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Price == out[j].Price {
			return out[i].VolumeRemain > out[j].VolumeRemain
		}
		if side == "buy" {
			return out[i].Price > out[j].Price
		}
		return out[i].Price < out[j].Price
	})
	return out
}

func nearestReplayBook(at time.Time, books []OrderBookReplayBook, maxAge time.Duration) (OrderBookReplayBook, bool) {
	var best OrderBookReplayBook
	bestDelta := time.Duration(1<<63 - 1)
	for _, book := range books {
		delta := book.CapturedAt.Sub(at)
		if delta < 0 {
			delta = -delta
		}
		if delta <= maxAge && delta < bestDelta {
			best = book
			bestDelta = delta
		}
	}
	return best, bestDelta != time.Duration(1<<63-1)
}

func firstReplayPrice(levels []OrderBookReplayLevel, priceMult float64) float64 {
	if len(levels) == 0 {
		return 0
	}
	return levels[0].Price * priceMult
}

func fillReplayLevels(levels []OrderBookReplayLevel, qty int32, priceMult float64, volumePct float64) (int32, float64, int64) {
	if qty <= 0 {
		return 0, 0, 0
	}
	remaining := int64(qty)
	var filled int64
	total := 0.0
	var available int64
	for _, level := range levels {
		levelVolume := replayLevelVolume(level.VolumeRemain, volumePct)
		if levelVolume <= 0 {
			continue
		}
		available += levelVolume
		if remaining <= 0 {
			continue
		}
		take := minInt64(remaining, levelVolume)
		total += level.Price * priceMult * float64(take)
		filled += take
		remaining -= take
	}
	return clampInt64ToInt32(filled), total, available
}

func replayLevelVolume(volume int64, pct float64) int64 {
	if volume <= 0 || pct <= 0 {
		return 0
	}
	if pct >= 100 {
		return volume
	}
	scaled := int64(math.Floor(float64(volume) * pct / 100))
	if scaled <= 0 {
		return 1
	}
	return scaled
}

func maxAffordableReplayQty(levels []OrderBookReplayLevel, maxQty int32, priceMult, buyCostMult, volumePct, budget float64) int32 {
	if maxQty <= 0 || budget <= 0 || buyCostMult <= 0 {
		return 0
	}
	lo, hi := int32(0), maxQty
	for lo < hi {
		mid := lo + (hi-lo+1)/2
		filled, gross, _ := fillReplayLevels(levels, mid, priceMult, volumePct)
		if filled == mid && gross*buyCostMult <= budget {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
}

func replayBooksDepth(books []OrderBookReplayBook) (int64, int) {
	var depth int64
	var levels int
	for _, book := range books {
		levels += len(book.Levels)
		for _, level := range book.Levels {
			if level.VolumeRemain > 0 {
				depth += level.VolumeRemain
			}
		}
	}
	return depth, levels
}

func countReplayBookPairs(sourceBooks, targetBooks []OrderBookReplayBook, maxAge time.Duration) int {
	pairs := 0
	for _, sourceBook := range sourceBooks {
		if _, ok := nearestReplayBook(sourceBook.CapturedAt, targetBooks, maxAge); ok {
			pairs++
		}
	}
	return pairs
}

func replayBooksTimeRange(sourceBooks, targetBooks []OrderBookReplayBook) (time.Time, time.Time) {
	var oldest, newest time.Time
	for _, book := range sourceBooks {
		oldest, newest = mergeReplayTimeRange(oldest, newest, book.CapturedAt, book.CapturedAt)
	}
	for _, book := range targetBooks {
		oldest, newest = mergeReplayTimeRange(oldest, newest, book.CapturedAt, book.CapturedAt)
	}
	return oldest, newest
}

func mergeReplayTimeRange(oldest, newest, rowOldest, rowNewest time.Time) (time.Time, time.Time) {
	if !rowOldest.IsZero() && (oldest.IsZero() || rowOldest.Before(oldest)) {
		oldest = rowOldest
	}
	if !rowNewest.IsZero() && (newest.IsZero() || rowNewest.After(newest)) {
		newest = rowNewest
	}
	return oldest, newest
}

func replayTimeString(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
