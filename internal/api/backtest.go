package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"eve-flipper/internal/db"
	"eve-flipper/internal/engine"
)

type backtestFlipsRequest struct {
	Rows                 []engine.FlipResult `json:"rows"`
	StrategyMode         string              `json:"strategy_mode"`
	InstantPriceMode     string              `json:"instant_price_mode"`
	HoldDays             int                 `json:"hold_days"`
	WindowDays           int                 `json:"window_days"`
	MaxRows              int                 `json:"max_rows"`
	EntrySpacingDays     int                 `json:"entry_spacing_days"`
	TravelCooldownDays   int                 `json:"travel_cooldown_days"`
	NonOverlapping       bool                `json:"non_overlapping"`
	QuantityMode         string              `json:"quantity_mode"`
	FixedQuantity        int32               `json:"fixed_quantity"`
	BudgetISK            float64             `json:"budget_isk"`
	BuyPriceSource       string              `json:"buy_price_source"`
	VolumeFillFraction   float64             `json:"volume_fill_fraction"`
	SkipUnfillable       bool                `json:"skip_unfillable"`
	BuyPriceMarkupPct    float64             `json:"buy_price_markup_percent"`
	SellPriceHaircutPct  float64             `json:"sell_price_haircut_percent"`
	MinROIPercent        float64             `json:"min_roi_percent"`
	ExcludeOpenTrades    bool                `json:"exclude_open_trades"`
	SalesTaxPercent      float64             `json:"sales_tax_percent"`
	BrokerFeePercent     float64             `json:"broker_fee_percent"`
	SplitTradeFees       bool                `json:"split_trade_fees"`
	BuyBrokerFeePercent  float64             `json:"buy_broker_fee_percent"`
	SellBrokerFeePercent float64             `json:"sell_broker_fee_percent"`
	BuySalesTaxPercent   float64             `json:"buy_sales_tax_percent"`
	SellSalesTaxPercent  float64             `json:"sell_sales_tax_percent"`
	OrderBookMaxAgeMin   int                 `json:"orderbook_max_age_minutes"`
	OrderBookCooldownMin int                 `json:"orderbook_cooldown_minutes"`
	CooldownMode         string              `json:"cooldown_mode"`
	CargoCapacity        float64             `json:"cargo_capacity"`
	RouteMinutesPerJump  float64             `json:"route_minutes_per_jump"`
	RouteDockMinutes     float64             `json:"route_dock_minutes"`
	RouteSafetyMult      float64             `json:"route_safety_multiplier"`
	RouteSafetyMode      string              `json:"route_safety_mode"`
	RouteMinSecurity     float64             `json:"route_min_security"`
	RouteMinCooldownMin  int                 `json:"route_min_cooldown_minutes"`
}

func backtestParamsFromRequest(req backtestFlipsRequest) engine.FlipBacktestParams {
	return engine.FlipBacktestParams{
		StrategyMode:         req.StrategyMode,
		InstantPriceMode:     req.InstantPriceMode,
		HoldDays:             req.HoldDays,
		WindowDays:           req.WindowDays,
		MaxRows:              req.MaxRows,
		EntrySpacingDays:     req.EntrySpacingDays,
		TravelCooldownDays:   req.TravelCooldownDays,
		NonOverlapping:       req.NonOverlapping,
		QuantityMode:         req.QuantityMode,
		FixedQuantity:        req.FixedQuantity,
		BudgetISK:            req.BudgetISK,
		BuyPriceSource:       req.BuyPriceSource,
		VolumeFillFraction:   req.VolumeFillFraction,
		SkipUnfillable:       req.SkipUnfillable,
		BuyPriceMarkupPct:    req.BuyPriceMarkupPct,
		SellPriceHaircutPct:  req.SellPriceHaircutPct,
		MinROIPercent:        req.MinROIPercent,
		ExcludeOpenTrades:    req.ExcludeOpenTrades,
		SalesTaxPercent:      req.SalesTaxPercent,
		BrokerFeePercent:     req.BrokerFeePercent,
		SplitTradeFees:       req.SplitTradeFees,
		BuyBrokerFeePercent:  req.BuyBrokerFeePercent,
		SellBrokerFeePercent: req.SellBrokerFeePercent,
		BuySalesTaxPercent:   req.BuySalesTaxPercent,
		SellSalesTaxPercent:  req.SellSalesTaxPercent,
		OrderBookMaxAgeMin:   req.OrderBookMaxAgeMin,
		OrderBookCooldownMin: req.OrderBookCooldownMin,
		CooldownMode:         req.CooldownMode,
		CargoCapacity:        req.CargoCapacity,
		RouteMinutesPerJump:  req.RouteMinutesPerJump,
		RouteDockMinutes:     req.RouteDockMinutes,
		RouteSafetyMult:      req.RouteSafetyMult,
		RouteSafetyMode:      req.RouteSafetyMode,
		RouteMinCooldownMin:  req.RouteMinCooldownMin,
	}
}

func (s *Server) rowsWithBacktestRouteRisk(rows []engine.FlipResult, minSec float64) []engine.FlipResult {
	if len(rows) == 0 || s.ganker == nil {
		return rows
	}
	out := make([]engine.FlipResult, len(rows))
	copy(out, rows)
	cache := make(map[string]routeHaulingRiskSummary)
	for i := range out {
		row := &out[i]
		if row.BuySystemID <= 0 || row.SellSystemID <= 0 || row.BuySystemID == row.SellSystemID {
			row.RouteSafetyMultiplier = 1
			row.RouteSafetyDanger = "green"
			continue
		}
		key := fmt.Sprintf("%d:%d:%.2f", row.BuySystemID, row.SellSystemID, minSec)
		summary, ok := cache[key]
		if !ok {
			summary.add(s.routeDangerSystems(row.BuySystemID, row.SellSystemID, minSec))
			cache[key] = summary
		}
		mult := routeSafetyMultiplierFromSummary(summary)
		row.RouteSafetyMultiplier = mult
		row.RouteSafetyDanger = summary.danger
		if row.RouteSafetyDanger == "" {
			row.RouteSafetyDanger = "green"
		}
		row.RouteSafetyKills = summary.kills
		row.RouteSafetyISK = summary.totalISK
	}
	return out
}

func (s *Server) orderBookReplayGetter() engine.OrderBookReplayGetter {
	return func(filter engine.OrderBookReplayFilter) ([]engine.OrderBookReplayBook, error) {
		books, err := s.db.ListOrderBookReplayBooks(db.OrderBookReplayFilter{
			RegionID:       filter.RegionID,
			TypeID:         filter.TypeID,
			LocationID:     filter.LocationID,
			Side:           filter.Side,
			FromCapturedAt: filter.From,
			ToCapturedAt:   filter.To,
			Limit:          filter.Limit,
			LevelLimit:     2000,
		})
		if err != nil {
			return nil, err
		}
		out := make([]engine.OrderBookReplayBook, 0, len(books))
		for _, book := range books {
			capturedAt, err := time.Parse(time.RFC3339, book.Snapshot.CapturedAt)
			if err != nil || capturedAt.IsZero() {
				continue
			}
			levels := make([]engine.OrderBookReplayLevel, 0, len(book.Levels))
			for _, level := range book.Levels {
				levels = append(levels, engine.OrderBookReplayLevel{
					Price:        level.Price,
					VolumeRemain: level.VolumeRemain,
				})
			}
			out = append(out, engine.OrderBookReplayBook{
				SnapshotID: book.Snapshot.ID,
				CapturedAt: capturedAt,
				Levels:     levels,
			})
		}
		return out, nil
	}
}

func (s *Server) handleBacktestFlips(w http.ResponseWriter, r *http.Request) {
	var req backtestFlipsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if len(req.Rows) == 0 {
		writeError(w, http.StatusBadRequest, "rows are required")
		return
	}

	params := backtestParamsFromRequest(req)

	if req.StrategyMode == "instant_flip" && req.InstantPriceMode == "recorded_orderbook" {
		if s.db == nil {
			writeError(w, http.StatusServiceUnavailable, "orderbook database not ready")
			return
		}
		rows := req.Rows
		if params.CooldownMode == "route_time" && params.RouteSafetyMode == "auto" {
			rows = s.rowsWithBacktestRouteRisk(req.Rows, req.RouteMinSecurity)
		}
		result := engine.BuildOrderBookReplayBacktest(rows, params, s.orderBookReplayGetter())
		writeJSON(w, result)
		return
	}

	if !s.isReady() {
		writeError(w, http.StatusServiceUnavailable, "SDE not loaded yet")
		return
	}

	s.mu.RLock()
	scanner := s.scanner
	s.mu.RUnlock()
	if scanner == nil {
		writeError(w, http.StatusServiceUnavailable, "scanner not ready")
		return
	}

	result := scanner.BacktestFlips(req.Rows, params)
	writeJSON(w, result)
}

func (s *Server) handleOrderBookCoverage(w http.ResponseWriter, r *http.Request) {
	var req backtestFlipsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if len(req.Rows) == 0 {
		writeError(w, http.StatusBadRequest, "rows are required")
		return
	}
	if s.db == nil {
		writeError(w, http.StatusServiceUnavailable, "orderbook database not ready")
		return
	}
	result := engine.BuildOrderBookReplayCoverage(req.Rows, backtestParamsFromRequest(req), s.orderBookReplayGetter())
	writeJSON(w, result)
}
