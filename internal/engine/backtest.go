package engine

import (
	"fmt"
	"math"
	"sort"

	"eve-flipper/internal/esi"
)

type FlipBacktestParams struct {
	StrategyMode         string
	InstantPriceMode     string
	HoldDays             int
	WindowDays           int
	MaxRows              int
	EntrySpacingDays     int
	TravelCooldownDays   int
	NonOverlapping       bool
	QuantityMode         string
	FixedQuantity        int32
	BudgetISK            float64
	BuyPriceSource       string
	VolumeFillFraction   float64
	SkipUnfillable       bool
	BuyPriceMarkupPct    float64
	SellPriceHaircutPct  float64
	MinROIPercent        float64
	ExcludeOpenTrades    bool
	SalesTaxPercent      float64
	BrokerFeePercent     float64
	SplitTradeFees       bool
	BuyBrokerFeePercent  float64
	SellBrokerFeePercent float64
	BuySalesTaxPercent   float64
	SellSalesTaxPercent  float64
	OrderBookMaxAgeMin   int
	OrderBookCooldownMin int
	CooldownMode         string
	CargoCapacity        float64
	RouteMinutesPerJump  float64
	RouteDockMinutes     float64
	RouteSafetyMult      float64
	RouteSafetyMode      string
	RouteMinCooldownMin  int
}

type FlipBacktestTrade struct {
	TypeID          int32   `json:"type_id"`
	TypeName        string  `json:"type_name"`
	EntryDate       string  `json:"entry_date"`
	ExitDate        string  `json:"exit_date"`
	Status          string  `json:"status"` // closed | open
	Quantity        int32   `json:"quantity"`
	BuyPrice        float64 `json:"buy_price"`
	SellPrice       float64 `json:"sell_price"`
	BuyCost         float64 `json:"buy_cost"`
	SellRevenue     float64 `json:"sell_revenue"`
	PnL             float64 `json:"pnl"`
	ROIPercent      float64 `json:"roi_percent"`
	Fillable        bool    `json:"fillable"`
	TargetVolume    int64   `json:"target_volume"`
	RouteTimeMin    float64 `json:"route_time_minutes,omitempty"`
	RouteJumps      int     `json:"route_jumps,omitempty"`
	CargoTrips      int     `json:"cargo_trips,omitempty"`
	RouteSafetyMult float64 `json:"route_safety_multiplier,omitempty"`
	RouteDanger     string  `json:"route_danger,omitempty"`
	RouteKills      int     `json:"route_kills,omitempty"`
}

type FlipBacktestItemSummary struct {
	TypeID       int32   `json:"type_id"`
	TypeName     string  `json:"type_name"`
	Trades       int     `json:"trades"`
	ClosedTrades int     `json:"closed_trades"`
	OpenTrades   int     `json:"open_trades"`
	TotalPnL     float64 `json:"total_pnl"`
	RealizedPnL  float64 `json:"realized_pnl"`
	MTMPnL       float64 `json:"mtm_pnl"`
	WinRate      float64 `json:"win_rate"`
	AvgROI       float64 `json:"avg_roi"`
	FillRate     float64 `json:"fill_rate"`
}

type FlipBacktestEquityPoint struct {
	Date        string  `json:"date"`
	Equity      float64 `json:"equity"`
	Realized    float64 `json:"realized"`
	Drawdown    float64 `json:"drawdown"`
	DayPnL      float64 `json:"day_pnl"`
	DayTrades   int     `json:"day_trades"`
	TotalTrades int     `json:"total_trades"`
}

type FlipBacktestSummary struct {
	RowsTested         int     `json:"rows_tested"`
	Trades             int     `json:"trades"`
	ClosedTrades       int     `json:"closed_trades"`
	OpenTrades         int     `json:"open_trades"`
	TotalPnL           float64 `json:"total_pnl"`
	RealizedPnL        float64 `json:"realized_pnl"`
	MTMPnL             float64 `json:"mtm_pnl"`
	WinRate            float64 `json:"win_rate"`
	AvgROI             float64 `json:"avg_roi"`
	MaxDrawdownISK     float64 `json:"max_drawdown_isk"`
	MaxDrawdownPct     float64 `json:"max_drawdown_pct"`
	BestTradePnL       float64 `json:"best_trade_pnl"`
	WorstTradePnL      float64 `json:"worst_trade_pnl"`
	BacktestDays       int     `json:"backtest_days"`
	HoldDays           int     `json:"hold_days"`
	StrategyMode       string  `json:"strategy_mode"`
	TravelCooldown     int     `json:"travel_cooldown_days"`
	CooldownMinutes    int     `json:"cooldown_minutes,omitempty"`
	CooldownMode       string  `json:"cooldown_mode,omitempty"`
	DataSource         string  `json:"data_source,omitempty"`
	OrderBookMaxAge    int     `json:"orderbook_max_age_minutes,omitempty"`
	AvgRouteTimeMin    float64 `json:"avg_route_time_minutes,omitempty"`
	MaxRouteTimeMin    float64 `json:"max_route_time_minutes,omitempty"`
	RouteSafetyMode    string  `json:"route_safety_mode,omitempty"`
	AvgRouteSafetyMult float64 `json:"avg_route_safety_multiplier,omitempty"`
	MaxRouteSafetyMult float64 `json:"max_route_safety_multiplier,omitempty"`
}

type FlipBacktestResult struct {
	Summary  FlipBacktestSummary       `json:"summary"`
	Items    []FlipBacktestItemSummary `json:"items"`
	Ledger   []FlipBacktestTrade       `json:"ledger"`
	Equity   []FlipBacktestEquityPoint `json:"equity"`
	Warnings []string                  `json:"warnings,omitempty"`
}

type historyGetter func(regionID int32, typeID int32) []esi.HistoryEntry

type instantBacktestDiag struct {
	Candidates   int
	MissingPrice int
	NoQuantity   int
	Unfillable   int
	BelowROI     int
	BestROI      float64
	HasBestROI   bool
}

func (d *instantBacktestDiag) add(other instantBacktestDiag) {
	d.Candidates += other.Candidates
	d.MissingPrice += other.MissingPrice
	d.NoQuantity += other.NoQuantity
	d.Unfillable += other.Unfillable
	d.BelowROI += other.BelowROI
	if other.HasBestROI && (!d.HasBestROI || other.BestROI > d.BestROI) {
		d.BestROI = other.BestROI
		d.HasBestROI = true
	}
}

func (d *instantBacktestDiag) observeROI(roi float64) {
	if !d.HasBestROI || roi > d.BestROI {
		d.BestROI = roi
		d.HasBestROI = true
	}
}

func (d instantBacktestDiag) warning() string {
	best := "n/a"
	if d.HasBestROI {
		best = fmt.Sprintf("%.1f%%", sanitizeFloat(d.BestROI))
	}
	return fmt.Sprintf(
		"instant flip found no trades: candidates=%d below_min_roi=%d unfillable=%d missing_price=%d no_quantity=%d best_roi=%s",
		d.Candidates,
		d.BelowROI,
		d.Unfillable,
		d.MissingPrice,
		d.NoQuantity,
		best,
	)
}

func (s *Scanner) BacktestFlips(rows []FlipResult, params FlipBacktestParams) FlipBacktestResult {
	return BuildFlipBacktest(rows, params, func(regionID int32, typeID int32) []esi.HistoryEntry {
		return s.historyEntries(regionID, typeID)
	})
}

func BuildFlipBacktest(rows []FlipResult, params FlipBacktestParams, getHistory historyGetter) FlipBacktestResult {
	params = normalizeFlipBacktestParams(params)
	result := FlipBacktestResult{}
	if len(rows) == 0 || getHistory == nil {
		result.Summary.HoldDays = params.HoldDays
		result.Summary.BacktestDays = params.WindowDays
		result.Summary.StrategyMode = params.StrategyMode
		result.Summary.TravelCooldown = params.TravelCooldownDays
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

	var instantDiag instantBacktestDiag
	for _, row := range rows {
		var trades []FlipBacktestTrade
		if params.StrategyMode == "instant_flip" {
			var diag instantBacktestDiag
			trades, diag = backtestInstantFlipRow(row, params, buyCostMult, sellRevenueMult, getHistory)
			instantDiag.add(diag)
		} else {
			trades = backtestHoldCycleRow(row, params, buyCostMult, sellRevenueMult, getHistory)
		}
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
	if params.StrategyMode == "instant_flip" && len(result.Ledger) == 0 {
		result.Warnings = append(result.Warnings, instantDiag.warning())
	}
	return result
}

func normalizeFlipBacktestParams(params FlipBacktestParams) FlipBacktestParams {
	if params.StrategyMode != "instant_flip" {
		params.StrategyMode = "hold"
	}
	if params.InstantPriceMode != "history_pair" {
		if params.InstantPriceMode != "recorded_orderbook" {
			params.InstantPriceMode = "scan_spread"
		}
	}
	if params.HoldDays <= 0 {
		params.HoldDays = 7
	}
	if params.HoldDays > 90 {
		params.HoldDays = 90
	}
	if params.WindowDays <= 0 {
		params.WindowDays = 90
	}
	if params.WindowDays < params.HoldDays+2 {
		params.WindowDays = params.HoldDays + 2
	}
	if params.WindowDays > 365 {
		params.WindowDays = 365
	}
	if params.MaxRows <= 0 {
		params.MaxRows = 100
	}
	if params.MaxRows > 500 {
		params.MaxRows = 500
	}
	if params.EntrySpacingDays <= 0 {
		params.EntrySpacingDays = 1
	}
	if params.EntrySpacingDays > 30 {
		params.EntrySpacingDays = 30
	}
	if params.TravelCooldownDays <= 0 {
		params.TravelCooldownDays = 1
	}
	if params.TravelCooldownDays > 30 {
		params.TravelCooldownDays = 30
	}
	if params.OrderBookMaxAgeMin <= 0 {
		params.OrderBookMaxAgeMin = 15
	}
	if params.OrderBookMaxAgeMin > 24*60 {
		params.OrderBookMaxAgeMin = 24 * 60
	}
	if params.OrderBookCooldownMin <= 0 {
		params.OrderBookCooldownMin = 60
	}
	if params.OrderBookCooldownMin > 7*24*60 {
		params.OrderBookCooldownMin = 7 * 24 * 60
	}
	if params.CooldownMode != "route_time" {
		params.CooldownMode = "manual"
	}
	if params.CargoCapacity < 0 || math.IsNaN(params.CargoCapacity) || math.IsInf(params.CargoCapacity, 0) {
		params.CargoCapacity = 0
	}
	if params.RouteMinutesPerJump <= 0 || math.IsNaN(params.RouteMinutesPerJump) || math.IsInf(params.RouteMinutesPerJump, 0) {
		params.RouteMinutesPerJump = 2
	}
	if params.RouteMinutesPerJump > 60 {
		params.RouteMinutesPerJump = 60
	}
	if params.RouteDockMinutes < 0 || math.IsNaN(params.RouteDockMinutes) || math.IsInf(params.RouteDockMinutes, 0) {
		params.RouteDockMinutes = 0
	}
	if params.RouteDockMinutes > 120 {
		params.RouteDockMinutes = 120
	}
	if params.RouteSafetyMult <= 0 || math.IsNaN(params.RouteSafetyMult) || math.IsInf(params.RouteSafetyMult, 0) {
		params.RouteSafetyMult = 1
	}
	if params.RouteSafetyMult > 10 {
		params.RouteSafetyMult = 10
	}
	if params.RouteSafetyMode != "auto" {
		params.RouteSafetyMode = "manual"
	}
	if params.RouteMinCooldownMin < 0 {
		params.RouteMinCooldownMin = 0
	}
	if params.RouteMinCooldownMin > 7*24*60 {
		params.RouteMinCooldownMin = 7 * 24 * 60
	}
	switch params.QuantityMode {
	case "fixed", "budget":
	default:
		params.QuantityMode = "scan"
	}
	if params.FixedQuantity < 0 {
		params.FixedQuantity = 0
	}
	if params.BudgetISK < 0 || math.IsNaN(params.BudgetISK) || math.IsInf(params.BudgetISK, 0) {
		params.BudgetISK = 0
	}
	if params.BuyPriceSource != "scan" {
		params.BuyPriceSource = "history"
	}
	if params.VolumeFillFraction <= 0 || math.IsNaN(params.VolumeFillFraction) || math.IsInf(params.VolumeFillFraction, 0) {
		params.VolumeFillFraction = 100
	}
	if params.VolumeFillFraction > 100 {
		params.VolumeFillFraction = 100
	}
	params.BuyPriceMarkupPct = clampPercent(params.BuyPriceMarkupPct)
	params.SellPriceHaircutPct = clampPercent(params.SellPriceHaircutPct)
	if math.IsNaN(params.MinROIPercent) || math.IsInf(params.MinROIPercent, 0) {
		params.MinROIPercent = 0
	}
	return params
}

func backtestHoldCycleRow(
	row FlipResult,
	params FlipBacktestParams,
	buyCostMult float64,
	sellRevenueMult float64,
	getHistory historyGetter,
) []FlipBacktestTrade {
	if row.TypeID <= 0 || row.SellRegionID <= 0 {
		return nil
	}

	target := sortedHistory(getHistory(row.SellRegionID, row.TypeID))
	if len(target) < 2 {
		return nil
	}
	sourceByDate := historyByDate(getHistory(row.BuyRegionID, row.TypeID))
	fallbackBuy := row.ExpectedBuyPrice
	if fallbackBuy <= 0 {
		fallbackBuy = row.BuyPrice
	}
	fallbackSell := row.ExpectedSellPrice
	if fallbackSell <= 0 {
		fallbackSell = row.SellPrice
	}
	if fallbackBuy <= 0 || fallbackSell <= 0 {
		return nil
	}

	start := len(target) - params.WindowDays
	if start < 0 {
		start = 0
	}

	var out []FlipBacktestTrade
	lastIdx := len(target) - 1
	effectiveEntrySpacing := params.EntrySpacingDays
	if params.NonOverlapping && effectiveEntrySpacing < params.HoldDays {
		effectiveEntrySpacing = params.HoldDays
	}
	for i := start; i < len(target); i++ {
		if (i-start)%effectiveEntrySpacing != 0 {
			continue
		}
		entry := target[i]
		if entry.Average <= 0 {
			continue
		}
		exitIdx := i + params.HoldDays
		status := "closed"
		if exitIdx > lastIdx {
			exitIdx = lastIdx
			status = "open"
		}
		if exitIdx <= i && status == "closed" {
			continue
		}
		exit := target[exitIdx]
		if exit.Average <= 0 {
			continue
		}

		buyPrice := fallbackBuy
		if params.BuyPriceSource == "history" {
			if src, ok := sourceByDate[entry.Date]; ok && src.Average > 0 {
				buyPrice = src.Average
			}
		}
		if params.BuyPriceMarkupPct > 0 {
			buyPrice *= 1 + params.BuyPriceMarkupPct/100
		}
		sellPrice := exit.Average
		if sellPrice <= 0 {
			sellPrice = fallbackSell
		}
		if params.SellPriceHaircutPct > 0 {
			sellPrice *= 1 - params.SellPriceHaircutPct/100
		}
		if buyPrice <= 0 || sellPrice <= 0 {
			continue
		}

		qty := backtestTradeQuantity(row, params, buyPrice, buyCostMult)
		if qty <= 0 {
			continue
		}
		fillable := float64(entry.Volume)*params.VolumeFillFraction/100 >= float64(qty)
		if params.SkipUnfillable && !fillable {
			continue
		}

		buyCost := buyPrice * buyCostMult * float64(qty)
		sellRevenue := sellPrice * sellRevenueMult * float64(qty)
		pnl := sellRevenue - buyCost
		roi := 0.0
		if buyCost > 0 {
			roi = pnl / buyCost * 100
		}
		if params.MinROIPercent != 0 && roi < params.MinROIPercent {
			continue
		}
		if params.ExcludeOpenTrades && status == "open" {
			continue
		}

		out = append(out, FlipBacktestTrade{
			TypeID:       row.TypeID,
			TypeName:     row.TypeName,
			EntryDate:    entry.Date,
			ExitDate:     exit.Date,
			Status:       status,
			Quantity:     qty,
			BuyPrice:     sanitizeFloat(buyPrice),
			SellPrice:    sanitizeFloat(sellPrice),
			BuyCost:      sanitizeFloat(buyCost),
			SellRevenue:  sanitizeFloat(sellRevenue),
			PnL:          sanitizeFloat(pnl),
			ROIPercent:   sanitizeFloat(roi),
			Fillable:     fillable,
			TargetVolume: entry.Volume,
		})
	}
	return out
}

func backtestInstantFlipRow(
	row FlipResult,
	params FlipBacktestParams,
	buyCostMult float64,
	sellRevenueMult float64,
	getHistory historyGetter,
) ([]FlipBacktestTrade, instantBacktestDiag) {
	var diag instantBacktestDiag
	if row.TypeID <= 0 || row.SellRegionID <= 0 {
		return nil, diag
	}

	target := sortedHistory(getHistory(row.SellRegionID, row.TypeID))
	if len(target) == 0 {
		return nil, diag
	}
	sourceByDate := historyByDate(getHistory(row.BuyRegionID, row.TypeID))
	fallbackBuy := row.ExpectedBuyPrice
	if fallbackBuy <= 0 {
		fallbackBuy = row.BuyPrice
	}
	fallbackSell := row.ExpectedSellPrice
	if fallbackSell <= 0 {
		fallbackSell = row.SellPrice
	}
	if fallbackBuy <= 0 || fallbackSell <= 0 {
		return nil, diag
	}

	start := len(target) - params.WindowDays
	if start < 0 {
		start = 0
	}

	var out []FlipBacktestTrade
	nextAllowedIdx := start
	for i := start; i < len(target); i++ {
		if i < nextAllowedIdx || (i-start)%params.EntrySpacingDays != 0 {
			continue
		}
		entry := target[i]
		if entry.Average <= 0 {
			continue
		}
		diag.Candidates++

		buyPrice := fallbackBuy
		sellPrice := fallbackSell
		sourceVolume := int64(0)
		if src, ok := sourceByDate[entry.Date]; ok {
			sourceVolume = src.Volume
			if params.InstantPriceMode == "history_pair" && params.BuyPriceSource == "history" && src.Average > 0 {
				buyPrice = src.Average
			}
		}
		if params.InstantPriceMode == "history_pair" {
			sellPrice = entry.Average
		}
		if params.BuyPriceMarkupPct > 0 {
			buyPrice *= 1 + params.BuyPriceMarkupPct/100
		}
		if sellPrice <= 0 {
			sellPrice = fallbackSell
		}
		if params.SellPriceHaircutPct > 0 {
			sellPrice *= 1 - params.SellPriceHaircutPct/100
		}
		if buyPrice <= 0 || sellPrice <= 0 {
			diag.MissingPrice++
			continue
		}

		qty := backtestTradeQuantity(row, params, buyPrice, buyCostMult)
		if qty <= 0 {
			diag.NoQuantity++
			continue
		}
		fillVolume := entry.Volume
		if sourceVolume > 0 && sourceVolume < fillVolume {
			fillVolume = sourceVolume
		}
		fillable := float64(fillVolume)*params.VolumeFillFraction/100 >= float64(qty)
		if params.SkipUnfillable && !fillable {
			diag.Unfillable++
			continue
		}

		buyCost := buyPrice * buyCostMult * float64(qty)
		sellRevenue := sellPrice * sellRevenueMult * float64(qty)
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

		out = append(out, FlipBacktestTrade{
			TypeID:       row.TypeID,
			TypeName:     row.TypeName,
			EntryDate:    entry.Date,
			ExitDate:     entry.Date,
			Status:       "closed",
			Quantity:     qty,
			BuyPrice:     sanitizeFloat(buyPrice),
			SellPrice:    sanitizeFloat(sellPrice),
			BuyCost:      sanitizeFloat(buyCost),
			SellRevenue:  sanitizeFloat(sellRevenue),
			PnL:          sanitizeFloat(pnl),
			ROIPercent:   sanitizeFloat(roi),
			Fillable:     fillable,
			TargetVolume: entry.Volume,
		})
		nextAllowedIdx = i + params.TravelCooldownDays
	}
	return out, diag
}

func backtestTradeQuantity(row FlipResult, params FlipBacktestParams, buyPrice float64, buyCostMult float64) int32 {
	switch params.QuantityMode {
	case "fixed":
		return params.FixedQuantity
	case "budget":
		if params.BudgetISK <= 0 || buyPrice <= 0 || buyCostMult <= 0 {
			return 0
		}
		qty := math.Floor(params.BudgetISK / (buyPrice * buyCostMult))
		const maxInt32 = int32(1<<31 - 1)
		if qty > float64(maxInt32) {
			return maxInt32
		}
		return int32(qty)
	default:
		qty := row.FilledQty
		if qty <= 0 {
			qty = row.UnitsToBuy
		}
		return qty
	}
}

func sortedHistory(entries []esi.HistoryEntry) []esi.HistoryEntry {
	if len(entries) == 0 {
		return nil
	}
	out := make([]esi.HistoryEntry, len(entries))
	copy(out, entries)
	sort.Slice(out, func(i, j int) bool {
		return out[i].Date < out[j].Date
	})
	return out
}

func historyByDate(entries []esi.HistoryEntry) map[string]esi.HistoryEntry {
	out := make(map[string]esi.HistoryEntry, len(entries))
	for _, e := range entries {
		if e.Date != "" {
			out[e.Date] = e
		}
	}
	return out
}

func summarizeFlipBacktest(rows []FlipResult, ledger []FlipBacktestTrade, params FlipBacktestParams) FlipBacktestSummary {
	s := FlipBacktestSummary{
		RowsTested:      len(rows),
		Trades:          len(ledger),
		BacktestDays:    params.WindowDays,
		HoldDays:        params.HoldDays,
		StrategyMode:    params.StrategyMode,
		TravelCooldown:  params.TravelCooldownDays,
		CooldownMinutes: params.OrderBookCooldownMin,
		CooldownMode:    params.CooldownMode,
		OrderBookMaxAge: params.OrderBookMaxAgeMin,
		RouteSafetyMode: params.RouteSafetyMode,
	}
	if params.InstantPriceMode == "recorded_orderbook" {
		s.DataSource = "recorded_orderbook"
	}
	if len(ledger) == 0 {
		return s
	}

	bestSet := false
	roiTotal := 0.0
	wins := 0
	roiCount := 0
	routeTimeTotal := 0.0
	routeTimeCount := 0
	routeSafetyTotal := 0.0
	routeSafetyCount := 0
	cum := 0.0
	peak := 0.0
	for _, tr := range ledger {
		s.TotalPnL += tr.PnL
		if tr.Status == "closed" {
			s.ClosedTrades++
			s.RealizedPnL += tr.PnL
			if tr.PnL > 0 {
				wins++
			}
			roiTotal += tr.ROIPercent
			roiCount++
			cum += tr.PnL
			if cum > peak {
				peak = cum
			}
			drawdown := peak - cum
			if drawdown > s.MaxDrawdownISK {
				s.MaxDrawdownISK = drawdown
				if peak > 0 {
					s.MaxDrawdownPct = sanitizeFloat(drawdown / peak * 100)
				}
			}
		} else {
			s.OpenTrades++
			s.MTMPnL += tr.PnL
		}
		if !bestSet {
			s.BestTradePnL = tr.PnL
			s.WorstTradePnL = tr.PnL
			bestSet = true
		} else {
			if tr.PnL > s.BestTradePnL {
				s.BestTradePnL = tr.PnL
			}
			if tr.PnL < s.WorstTradePnL {
				s.WorstTradePnL = tr.PnL
			}
		}
		if tr.RouteTimeMin > 0 {
			routeTimeTotal += tr.RouteTimeMin
			routeTimeCount++
			if tr.RouteTimeMin > s.MaxRouteTimeMin {
				s.MaxRouteTimeMin = tr.RouteTimeMin
			}
		}
		if tr.RouteSafetyMult > 0 {
			routeSafetyTotal += tr.RouteSafetyMult
			routeSafetyCount++
			if tr.RouteSafetyMult > s.MaxRouteSafetyMult {
				s.MaxRouteSafetyMult = tr.RouteSafetyMult
			}
		}
	}
	if s.ClosedTrades > 0 {
		s.WinRate = sanitizeFloat(float64(wins) / float64(s.ClosedTrades) * 100)
	}
	if roiCount > 0 {
		s.AvgROI = sanitizeFloat(roiTotal / float64(roiCount))
	}
	if routeTimeCount > 0 {
		s.AvgRouteTimeMin = sanitizeFloat(routeTimeTotal / float64(routeTimeCount))
		s.MaxRouteTimeMin = sanitizeFloat(s.MaxRouteTimeMin)
	}
	if routeSafetyCount > 0 {
		s.AvgRouteSafetyMult = sanitizeFloat(routeSafetyTotal / float64(routeSafetyCount))
		s.MaxRouteSafetyMult = sanitizeFloat(s.MaxRouteSafetyMult)
	}
	s.TotalPnL = sanitizeFloat(s.TotalPnL)
	s.RealizedPnL = sanitizeFloat(s.RealizedPnL)
	s.MTMPnL = sanitizeFloat(s.MTMPnL)
	s.MaxDrawdownISK = sanitizeFloat(s.MaxDrawdownISK)
	return s
}

func summarizeFlipBacktestItems(ledger []FlipBacktestTrade) []FlipBacktestItemSummary {
	byType := make(map[int32]*FlipBacktestItemSummary)
	wins := make(map[int32]int)
	roiTotal := make(map[int32]float64)
	roiCount := make(map[int32]int)
	fillable := make(map[int32]int)
	for _, tr := range ledger {
		item := byType[tr.TypeID]
		if item == nil {
			item = &FlipBacktestItemSummary{TypeID: tr.TypeID, TypeName: tr.TypeName}
			byType[tr.TypeID] = item
		}
		item.Trades++
		item.TotalPnL += tr.PnL
		if tr.Fillable {
			fillable[tr.TypeID]++
		}
		if tr.Status == "closed" {
			item.ClosedTrades++
			item.RealizedPnL += tr.PnL
			if tr.PnL > 0 {
				wins[tr.TypeID]++
			}
			roiTotal[tr.TypeID] += tr.ROIPercent
			roiCount[tr.TypeID]++
		} else {
			item.OpenTrades++
			item.MTMPnL += tr.PnL
		}
	}

	out := make([]FlipBacktestItemSummary, 0, len(byType))
	for typeID, item := range byType {
		if item.ClosedTrades > 0 {
			item.WinRate = sanitizeFloat(float64(wins[typeID]) / float64(item.ClosedTrades) * 100)
		}
		if roiCount[typeID] > 0 {
			item.AvgROI = sanitizeFloat(roiTotal[typeID] / float64(roiCount[typeID]))
		}
		if item.Trades > 0 {
			item.FillRate = sanitizeFloat(float64(fillable[typeID]) / float64(item.Trades) * 100)
		}
		item.TotalPnL = sanitizeFloat(item.TotalPnL)
		item.RealizedPnL = sanitizeFloat(item.RealizedPnL)
		item.MTMPnL = sanitizeFloat(item.MTMPnL)
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TotalPnL == out[j].TotalPnL {
			return out[i].TypeID < out[j].TypeID
		}
		return out[i].TotalPnL > out[j].TotalPnL
	})
	return out
}

func buildFlipBacktestEquityCurve(ledger []FlipBacktestTrade) []FlipBacktestEquityPoint {
	if len(ledger) == 0 {
		return nil
	}
	points := make([]FlipBacktestEquityPoint, 0, len(ledger))
	equity := 0.0
	realized := 0.0
	peak := 0.0
	trades := 0

	currentDate := ""
	dayPnL := 0.0
	dayTrades := 0
	flush := func() {
		if currentDate == "" {
			return
		}
		drawdown := peak - equity
		if drawdown < 0 {
			drawdown = 0
		}
		points = append(points, FlipBacktestEquityPoint{
			Date:        currentDate,
			Equity:      sanitizeFloat(equity),
			Realized:    sanitizeFloat(realized),
			Drawdown:    sanitizeFloat(drawdown),
			DayPnL:      sanitizeFloat(dayPnL),
			DayTrades:   dayTrades,
			TotalTrades: trades,
		})
	}

	for _, tr := range ledger {
		if currentDate != "" && tr.ExitDate != currentDate {
			flush()
			dayPnL = 0
			dayTrades = 0
		}
		currentDate = tr.ExitDate
		equity += tr.PnL
		dayPnL += tr.PnL
		dayTrades++
		trades++
		if tr.Status == "closed" {
			realized += tr.PnL
		}
		if equity > peak {
			peak = equity
		}
	}
	flush()
	return points
}
