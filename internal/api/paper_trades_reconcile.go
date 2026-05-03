package api

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"eve-flipper/internal/db"
	"eve-flipper/internal/esi"
)

type paperTradeReconcilePatch struct {
	Status          string  `json:"status,omitempty"`
	ActualQuantity  int64   `json:"actual_quantity,omitempty"`
	ActualBuyPrice  float64 `json:"actual_buy_price,omitempty"`
	ActualSellPrice float64 `json:"actual_sell_price,omitempty"`
}

type paperTradeReconcileRow struct {
	TradeID              int64                     `json:"trade_id"`
	SuggestedStatus      string                    `json:"suggested_status"`
	Confidence           string                    `json:"confidence"`
	Reason               string                    `json:"reason"`
	MatchedBuyQty        int64                     `json:"matched_buy_qty"`
	MatchedSellQty       int64                     `json:"matched_sell_qty"`
	AvgBuyPrice          float64                   `json:"avg_buy_price"`
	AvgSellPrice         float64                   `json:"avg_sell_price"`
	OpenBuyQty           int64                     `json:"open_buy_qty"`
	OpenSellQty          int64                     `json:"open_sell_qty"`
	AssetQty             int64                     `json:"asset_qty"`
	BuyLocationAssetQty  int64                     `json:"buy_location_asset_qty"`
	SellLocationAssetQty int64                     `json:"sell_location_asset_qty"`
	SuggestedPatch       *paperTradeReconcilePatch `json:"suggested_patch,omitempty"`
}

type paperTradeReconcileSummary struct {
	TradesChecked    int `json:"trades_checked"`
	Matched          int `json:"matched"`
	HighConfidence   int `json:"high_confidence"`
	MediumConfidence int `json:"medium_confidence"`
	LowConfidence    int `json:"low_confidence"`
	Characters       int `json:"characters"`
	Transactions     int `json:"transactions"`
	Orders           int `json:"orders"`
	Assets           int `json:"assets"`
}

type paperTradeReconcileResponse struct {
	OK       bool                       `json:"ok"`
	Summary  paperTradeReconcileSummary `json:"summary"`
	Rows     []paperTradeReconcileRow   `json:"rows"`
	Warnings []string                   `json:"warnings,omitempty"`
}

type paperTradeRuntime struct {
	Transactions []esi.WalletTransaction
	Orders       []esi.CharacterOrder
	Assets       []esi.CharacterAsset
}

func (s *Server) handleAuthReconcilePaperTrades(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	if s.db == nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	if s.esi == nil || s.sessions == nil {
		writeError(w, http.StatusUnauthorized, "not logged in")
		return
	}

	characterID, allScope, err := parseAuthScope(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sessions, err := s.authSessionsForScope(userID, characterID, allScope, true)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status == "" {
		status = db.PaperTradeStatusActive
	}
	limit := 300
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil || parsed < 1 {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = parsed
	}
	trades, err := s.db.ListPaperTradesForUser(userID, status, limit)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	runtime := paperTradeRuntime{}
	warnings := make([]string, 0)
	charactersUsed := 0
	for _, sess := range sessions {
		if sess == nil {
			continue
		}
		token, tokenErr := s.sessions.EnsureValidTokenForUserCharacter(s.sso, userID, sess.CharacterID)
		if tokenErr != nil {
			warnings = append(warnings, fmt.Sprintf("%s: token unavailable", sess.CharacterName))
			continue
		}
		charactersUsed++

		if txns, fetchErr := s.esi.GetWalletTransactions(sess.CharacterID, token); fetchErr == nil {
			runtime.Transactions = append(runtime.Transactions, txns...)
		} else {
			warnings = append(warnings, fmt.Sprintf("%s: wallet transactions unavailable", sess.CharacterName))
		}
		if orders, fetchErr := s.esi.GetCharacterOrders(sess.CharacterID, token); fetchErr == nil {
			runtime.Orders = append(runtime.Orders, orders...)
		} else {
			warnings = append(warnings, fmt.Sprintf("%s: active orders unavailable", sess.CharacterName))
		}
		if assets, fetchErr := s.esi.GetCharacterAssets(sess.CharacterID, token); fetchErr == nil {
			runtime.Assets = append(runtime.Assets, assets...)
		} else {
			warnings = append(warnings, fmt.Sprintf("%s: assets unavailable", sess.CharacterName))
		}
	}

	rows := reconcilePaperTradesWithRuntime(trades, runtime)
	summary := paperTradeReconcileSummary{
		TradesChecked: len(trades),
		Characters:    charactersUsed,
		Transactions:  len(runtime.Transactions),
		Orders:        len(runtime.Orders),
		Assets:        len(runtime.Assets),
	}
	for _, row := range rows {
		if row.Confidence != "none" {
			summary.Matched++
		}
		switch row.Confidence {
		case "high":
			summary.HighConfidence++
		case "medium":
			summary.MediumConfidence++
		case "low":
			summary.LowConfidence++
		}
	}

	writeJSON(w, paperTradeReconcileResponse{
		OK:       true,
		Summary:  summary,
		Rows:     rows,
		Warnings: warnings,
	})
}

func reconcilePaperTradesWithRuntime(trades []db.PaperTrade, runtime paperTradeRuntime) []paperTradeReconcileRow {
	rows := make([]paperTradeReconcileRow, 0, len(trades))
	for _, trade := range trades {
		rows = append(rows, reconcilePaperTradeWithRuntime(trade, runtime))
	}
	return rows
}

func reconcilePaperTradeWithRuntime(trade db.PaperTrade, runtime paperTradeRuntime) paperTradeReconcileRow {
	row := paperTradeReconcileRow{
		TradeID:         trade.ID,
		SuggestedStatus: trade.Status,
		Confidence:      "none",
		Reason:          "No matching live transactions, orders or assets found",
	}
	createdAt, haveCreatedAt := parsePaperTradeTime(trade.CreatedAt)

	var buyCost, sellRevenue float64
	for _, tx := range runtime.Transactions {
		if tx.TypeID != trade.TypeID || tx.Quantity <= 0 {
			continue
		}
		if haveCreatedAt {
			txAt, ok := parsePaperTradeTime(tx.Date)
			if ok && txAt.Before(createdAt.Add(-time.Minute)) {
				continue
			}
		}
		qty := int64(tx.Quantity)
		if tx.IsBuy {
			if trade.BuyLocationID > 0 && tx.LocationID != trade.BuyLocationID {
				continue
			}
			row.MatchedBuyQty += qty
			buyCost += cleanAPIFloat(tx.UnitPrice) * float64(qty)
			continue
		}
		if trade.SellLocationID > 0 && tx.LocationID != trade.SellLocationID {
			continue
		}
		row.MatchedSellQty += qty
		sellRevenue += cleanAPIFloat(tx.UnitPrice) * float64(qty)
	}
	if row.MatchedBuyQty > 0 {
		row.AvgBuyPrice = buyCost / float64(row.MatchedBuyQty)
	}
	if row.MatchedSellQty > 0 {
		row.AvgSellPrice = sellRevenue / float64(row.MatchedSellQty)
	}

	for _, order := range runtime.Orders {
		if order.TypeID != trade.TypeID || order.VolumeRemain <= 0 {
			continue
		}
		if order.IsBuyOrder {
			if trade.BuyLocationID > 0 && order.LocationID != trade.BuyLocationID {
				continue
			}
			row.OpenBuyQty += int64(order.VolumeRemain)
			continue
		}
		if trade.SellLocationID > 0 && order.LocationID != trade.SellLocationID {
			continue
		}
		row.OpenSellQty += int64(order.VolumeRemain)
	}

	for _, asset := range runtime.Assets {
		if asset.TypeID != trade.TypeID || asset.Quantity <= 0 {
			continue
		}
		row.AssetQty += asset.Quantity
		if trade.BuyLocationID > 0 && asset.LocationID == trade.BuyLocationID {
			row.BuyLocationAssetQty += asset.Quantity
		}
		if trade.SellLocationID > 0 && asset.LocationID == trade.SellLocationID {
			row.SellLocationAssetQty += asset.Quantity
		}
	}

	row.SuggestedStatus, row.Confidence, row.Reason = suggestPaperTradeLiveStatus(trade, row)
	row.SuggestedPatch = buildPaperTradeReconcilePatch(trade, row)
	return row
}

func suggestPaperTradeLiveStatus(trade db.PaperTrade, row paperTradeReconcileRow) (status string, confidence string, reason string) {
	plannedQty := trade.PlannedQuantity
	if plannedQty <= 0 {
		plannedQty = trade.ActualQuantity
	}
	if plannedQty <= 0 {
		plannedQty = 1
	}

	if row.MatchedSellQty >= plannedQty && row.MatchedBuyQty > 0 {
		return db.PaperTradeStatusSold, "high", "Matched completed buy and sell wallet transactions"
	}
	if row.MatchedSellQty > 0 && row.MatchedBuyQty > 0 {
		return db.PaperTradeStatusHauled, "medium", "Matched partial sell wallet transactions"
	}
	if row.OpenSellQty > 0 {
		return db.PaperTradeStatusHauled, "high", "Matching active sell order is live"
	}
	if row.SellLocationAssetQty > 0 {
		return db.PaperTradeStatusHauled, "medium", "Matching inventory is already at the sell location"
	}
	if row.MatchedBuyQty >= plannedQty {
		return db.PaperTradeStatusBought, "high", "Matched completed buy wallet transactions"
	}
	if row.MatchedBuyQty > 0 {
		return db.PaperTradeStatusBought, "medium", "Matched partial buy wallet transactions"
	}
	if row.BuyLocationAssetQty > 0 || row.AssetQty > 0 {
		return db.PaperTradeStatusBought, "medium", "Matching inventory is still visible in assets"
	}
	if row.OpenBuyQty > 0 {
		return db.PaperTradeStatusPlanned, "medium", "Matching active buy order is live"
	}
	return trade.Status, "none", "No matching live transactions, orders or assets found"
}

func buildPaperTradeReconcilePatch(trade db.PaperTrade, row paperTradeReconcileRow) *paperTradeReconcilePatch {
	patch := &paperTradeReconcilePatch{}
	changed := false

	if row.SuggestedStatus != "" && row.SuggestedStatus != trade.Status {
		patch.Status = row.SuggestedStatus
		changed = true
	}

	actualQty := row.MatchedSellQty
	if actualQty <= 0 {
		actualQty = row.MatchedBuyQty
	}
	if actualQty > 0 && actualQty != trade.ActualQuantity {
		patch.ActualQuantity = actualQty
		changed = true
	}
	if row.AvgBuyPrice > 0 && math.Abs(row.AvgBuyPrice-trade.ActualBuyPrice) > 0.0001 {
		patch.ActualBuyPrice = row.AvgBuyPrice
		changed = true
	}
	if row.AvgSellPrice > 0 && math.Abs(row.AvgSellPrice-trade.ActualSellPrice) > 0.0001 {
		patch.ActualSellPrice = row.AvgSellPrice
		changed = true
	}

	if !changed {
		return nil
	}
	return patch
}

func parsePaperTradeTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	if ts, err := time.Parse(time.RFC3339, value); err == nil {
		return ts, true
	}
	if ts, err := time.Parse("2006-01-02T15:04:05Z", value); err == nil {
		return ts, true
	}
	if ts, err := time.Parse("2006-01-02", value); err == nil {
		return ts, true
	}
	return time.Time{}, false
}

func cleanAPIFloat(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}
