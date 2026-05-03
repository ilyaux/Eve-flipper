package db

import (
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	PaperTradeStatusPlanned   = "planned"
	PaperTradeStatusBought    = "bought"
	PaperTradeStatusHauled    = "hauled"
	PaperTradeStatusSold      = "sold"
	PaperTradeStatusCancelled = "cancelled"
	PaperTradeStatusActive    = "active"
)

type PaperTrade struct {
	ID                int64   `json:"id"`
	UserID            string  `json:"user_id"`
	Status            string  `json:"status"`
	TypeID            int32   `json:"type_id"`
	TypeName          string  `json:"type_name"`
	PlannedQuantity   int64   `json:"planned_quantity"`
	ActualQuantity    int64   `json:"actual_quantity"`
	PlannedBuyPrice   float64 `json:"planned_buy_price"`
	PlannedSellPrice  float64 `json:"planned_sell_price"`
	ActualBuyPrice    float64 `json:"actual_buy_price"`
	ActualSellPrice   float64 `json:"actual_sell_price"`
	PlannedProfitISK  float64 `json:"planned_profit_isk"`
	PlannedROIPercent float64 `json:"planned_roi_percent"`
	FeesISK           float64 `json:"fees_isk"`
	HaulingCostISK    float64 `json:"hauling_cost_isk"`
	BuyStation        string  `json:"buy_station"`
	SellStation       string  `json:"sell_station"`
	BuySystemName     string  `json:"buy_system_name"`
	SellSystemName    string  `json:"sell_system_name"`
	BuySystemID       int32   `json:"buy_system_id"`
	SellSystemID      int32   `json:"sell_system_id"`
	BuyRegionID       int32   `json:"buy_region_id"`
	SellRegionID      int32   `json:"sell_region_id"`
	BuyLocationID     int64   `json:"buy_location_id"`
	SellLocationID    int64   `json:"sell_location_id"`
	VolumeM3          float64 `json:"volume_m3"`
	Notes             string  `json:"notes"`
	Source            string  `json:"source"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
	ClosedAt          string  `json:"closed_at"`

	ExpectedProfitISK float64 `json:"expected_profit_isk"`
	RealizedProfitISK float64 `json:"realized_profit_isk"`
	CapitalISK        float64 `json:"capital_isk"`
	ROIPercent        float64 `json:"roi_percent"`
}

type PaperTradeCreateInput struct {
	Status            string  `json:"status"`
	TypeID            int32   `json:"type_id"`
	TypeName          string  `json:"type_name"`
	PlannedQuantity   int64   `json:"planned_quantity"`
	ActualQuantity    int64   `json:"actual_quantity"`
	PlannedBuyPrice   float64 `json:"planned_buy_price"`
	PlannedSellPrice  float64 `json:"planned_sell_price"`
	ActualBuyPrice    float64 `json:"actual_buy_price"`
	ActualSellPrice   float64 `json:"actual_sell_price"`
	PlannedProfitISK  float64 `json:"planned_profit_isk"`
	PlannedROIPercent float64 `json:"planned_roi_percent"`
	FeesISK           float64 `json:"fees_isk"`
	HaulingCostISK    float64 `json:"hauling_cost_isk"`
	BuyStation        string  `json:"buy_station"`
	SellStation       string  `json:"sell_station"`
	BuySystemName     string  `json:"buy_system_name"`
	SellSystemName    string  `json:"sell_system_name"`
	BuySystemID       int32   `json:"buy_system_id"`
	SellSystemID      int32   `json:"sell_system_id"`
	BuyRegionID       int32   `json:"buy_region_id"`
	SellRegionID      int32   `json:"sell_region_id"`
	BuyLocationID     int64   `json:"buy_location_id"`
	SellLocationID    int64   `json:"sell_location_id"`
	VolumeM3          float64 `json:"volume_m3"`
	Notes             string  `json:"notes"`
	Source            string  `json:"source"`
}

type PaperTradeUpdateInput struct {
	Status            *string  `json:"status"`
	PlannedQuantity   *int64   `json:"planned_quantity"`
	ActualQuantity    *int64   `json:"actual_quantity"`
	PlannedBuyPrice   *float64 `json:"planned_buy_price"`
	PlannedSellPrice  *float64 `json:"planned_sell_price"`
	ActualBuyPrice    *float64 `json:"actual_buy_price"`
	ActualSellPrice   *float64 `json:"actual_sell_price"`
	PlannedProfitISK  *float64 `json:"planned_profit_isk"`
	PlannedROIPercent *float64 `json:"planned_roi_percent"`
	FeesISK           *float64 `json:"fees_isk"`
	HaulingCostISK    *float64 `json:"hauling_cost_isk"`
	Notes             *string  `json:"notes"`
}

func normalizePaperTradeStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", PaperTradeStatusPlanned:
		return PaperTradeStatusPlanned
	case PaperTradeStatusBought:
		return PaperTradeStatusBought
	case PaperTradeStatusHauled:
		return PaperTradeStatusHauled
	case PaperTradeStatusSold:
		return PaperTradeStatusSold
	case PaperTradeStatusCancelled:
		return PaperTradeStatusCancelled
	default:
		return ""
	}
}

func cleanPaperFloat(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

func cleanPaperText(v string, maxLen int) string {
	v = strings.TrimSpace(v)
	if maxLen > 0 && len(v) > maxLen {
		return v[:maxLen]
	}
	return v
}

func (t *PaperTrade) finalizeComputedFields() {
	if t.PlannedQuantity < 0 {
		t.PlannedQuantity = 0
	}
	if t.ActualQuantity < 0 {
		t.ActualQuantity = 0
	}

	plannedQty := float64(t.PlannedQuantity)
	t.CapitalISK = cleanPaperFloat(t.PlannedBuyPrice) * plannedQty
	t.ExpectedProfitISK = cleanPaperFloat(t.PlannedProfitISK)
	if t.ExpectedProfitISK == 0 && t.PlannedQuantity > 0 {
		t.ExpectedProfitISK = (cleanPaperFloat(t.PlannedSellPrice) - cleanPaperFloat(t.PlannedBuyPrice)) * plannedQty
	}

	if t.Status == PaperTradeStatusSold {
		actualQty := t.ActualQuantity
		if actualQty <= 0 {
			actualQty = t.PlannedQuantity
		}
		buyPrice := cleanPaperFloat(t.ActualBuyPrice)
		if buyPrice <= 0 {
			buyPrice = cleanPaperFloat(t.PlannedBuyPrice)
		}
		sellPrice := cleanPaperFloat(t.ActualSellPrice)
		if sellPrice <= 0 {
			sellPrice = cleanPaperFloat(t.PlannedSellPrice)
		}
		capital := buyPrice * float64(actualQty)
		t.CapitalISK = capital
		t.RealizedProfitISK = (sellPrice-buyPrice)*float64(actualQty) - cleanPaperFloat(t.FeesISK) - cleanPaperFloat(t.HaulingCostISK)
		if capital > 0 {
			t.ROIPercent = t.RealizedProfitISK / capital * 100
		}
		return
	}

	if t.CapitalISK > 0 {
		if t.PlannedROIPercent != 0 {
			t.ROIPercent = t.PlannedROIPercent
		} else {
			t.ROIPercent = t.ExpectedProfitISK / t.CapitalISK * 100
		}
	}
}

func validatePaperTrade(t PaperTrade) error {
	if normalizePaperTradeStatus(t.Status) == "" {
		return fmt.Errorf("invalid status")
	}
	if t.TypeID <= 0 {
		return fmt.Errorf("type_id is required")
	}
	if strings.TrimSpace(t.TypeName) == "" {
		return fmt.Errorf("type_name is required")
	}
	if t.PlannedQuantity <= 0 {
		return fmt.Errorf("planned_quantity must be positive")
	}
	if t.PlannedBuyPrice < 0 || t.PlannedSellPrice < 0 {
		return fmt.Errorf("planned prices must be non-negative")
	}
	if t.ActualQuantity < 0 {
		return fmt.Errorf("actual_quantity must be non-negative")
	}
	if t.ActualBuyPrice < 0 || t.ActualSellPrice < 0 || t.FeesISK < 0 || t.HaulingCostISK < 0 {
		return fmt.Errorf("actual prices and costs must be non-negative")
	}
	return nil
}

func paperTradeFromCreateInput(userID string, in PaperTradeCreateInput, now string) (PaperTrade, error) {
	status := normalizePaperTradeStatus(in.Status)
	if status == "" {
		return PaperTrade{}, fmt.Errorf("invalid status")
	}
	t := PaperTrade{
		UserID:            normalizeUserID(userID),
		Status:            status,
		TypeID:            in.TypeID,
		TypeName:          cleanPaperText(in.TypeName, 256),
		PlannedQuantity:   in.PlannedQuantity,
		ActualQuantity:    in.ActualQuantity,
		PlannedBuyPrice:   cleanPaperFloat(in.PlannedBuyPrice),
		PlannedSellPrice:  cleanPaperFloat(in.PlannedSellPrice),
		ActualBuyPrice:    cleanPaperFloat(in.ActualBuyPrice),
		ActualSellPrice:   cleanPaperFloat(in.ActualSellPrice),
		PlannedProfitISK:  cleanPaperFloat(in.PlannedProfitISK),
		PlannedROIPercent: cleanPaperFloat(in.PlannedROIPercent),
		FeesISK:           cleanPaperFloat(in.FeesISK),
		HaulingCostISK:    cleanPaperFloat(in.HaulingCostISK),
		BuyStation:        cleanPaperText(in.BuyStation, 256),
		SellStation:       cleanPaperText(in.SellStation, 256),
		BuySystemName:     cleanPaperText(in.BuySystemName, 128),
		SellSystemName:    cleanPaperText(in.SellSystemName, 128),
		BuySystemID:       in.BuySystemID,
		SellSystemID:      in.SellSystemID,
		BuyRegionID:       in.BuyRegionID,
		SellRegionID:      in.SellRegionID,
		BuyLocationID:     in.BuyLocationID,
		SellLocationID:    in.SellLocationID,
		VolumeM3:          cleanPaperFloat(in.VolumeM3),
		Notes:             cleanPaperText(in.Notes, 2048),
		Source:            cleanPaperText(in.Source, 64),
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if t.ActualQuantity <= 0 && (status == PaperTradeStatusBought || status == PaperTradeStatusHauled || status == PaperTradeStatusSold) {
		t.ActualQuantity = t.PlannedQuantity
	}
	if t.ActualBuyPrice <= 0 && (status == PaperTradeStatusBought || status == PaperTradeStatusHauled || status == PaperTradeStatusSold) {
		t.ActualBuyPrice = t.PlannedBuyPrice
	}
	if t.ActualSellPrice <= 0 && status == PaperTradeStatusSold {
		t.ActualSellPrice = t.PlannedSellPrice
	}
	if status == PaperTradeStatusSold || status == PaperTradeStatusCancelled {
		t.ClosedAt = now
	}
	if err := validatePaperTrade(t); err != nil {
		return PaperTrade{}, err
	}
	t.finalizeComputedFields()
	return t, nil
}

func (d *DB) CreatePaperTradeForUser(userID string, in PaperTradeCreateInput) (PaperTrade, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	trade, err := paperTradeFromCreateInput(userID, in, now)
	if err != nil {
		return PaperTrade{}, err
	}

	res, err := d.sql.Exec(`
		INSERT INTO paper_trades (
			user_id, status, type_id, type_name,
			planned_quantity, actual_quantity,
			planned_buy_price, planned_sell_price, actual_buy_price, actual_sell_price,
			planned_profit_isk, planned_roi_percent, fees_isk, hauling_cost_isk,
			buy_station, sell_station, buy_system_name, sell_system_name,
			buy_system_id, sell_system_id, buy_region_id, sell_region_id,
			buy_location_id, sell_location_id, volume_m3, notes, source,
			created_at, updated_at, closed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		trade.UserID, trade.Status, trade.TypeID, trade.TypeName,
		trade.PlannedQuantity, trade.ActualQuantity,
		trade.PlannedBuyPrice, trade.PlannedSellPrice, trade.ActualBuyPrice, trade.ActualSellPrice,
		trade.PlannedProfitISK, trade.PlannedROIPercent, trade.FeesISK, trade.HaulingCostISK,
		trade.BuyStation, trade.SellStation, trade.BuySystemName, trade.SellSystemName,
		trade.BuySystemID, trade.SellSystemID, trade.BuyRegionID, trade.SellRegionID,
		trade.BuyLocationID, trade.SellLocationID, trade.VolumeM3, trade.Notes, trade.Source,
		trade.CreatedAt, trade.UpdatedAt, trade.ClosedAt,
	)
	if err != nil {
		return PaperTrade{}, err
	}
	trade.ID, _ = res.LastInsertId()
	return trade, nil
}

type paperTradeScanner interface {
	Scan(dest ...any) error
}

func scanPaperTrade(scanner paperTradeScanner) (PaperTrade, error) {
	var t PaperTrade
	err := scanner.Scan(
		&t.ID, &t.UserID, &t.Status, &t.TypeID, &t.TypeName,
		&t.PlannedQuantity, &t.ActualQuantity,
		&t.PlannedBuyPrice, &t.PlannedSellPrice, &t.ActualBuyPrice, &t.ActualSellPrice,
		&t.PlannedProfitISK, &t.PlannedROIPercent, &t.FeesISK, &t.HaulingCostISK,
		&t.BuyStation, &t.SellStation, &t.BuySystemName, &t.SellSystemName,
		&t.BuySystemID, &t.SellSystemID, &t.BuyRegionID, &t.SellRegionID,
		&t.BuyLocationID, &t.SellLocationID, &t.VolumeM3, &t.Notes, &t.Source,
		&t.CreatedAt, &t.UpdatedAt, &t.ClosedAt,
	)
	if err != nil {
		return PaperTrade{}, err
	}
	t.finalizeComputedFields()
	return t, nil
}

const paperTradeSelectColumns = `
	id, user_id, status, type_id, type_name,
	planned_quantity, actual_quantity,
	planned_buy_price, planned_sell_price, actual_buy_price, actual_sell_price,
	planned_profit_isk, planned_roi_percent, fees_isk, hauling_cost_isk,
	buy_station, sell_station, buy_system_name, sell_system_name,
	buy_system_id, sell_system_id, buy_region_id, sell_region_id,
	buy_location_id, sell_location_id, volume_m3, notes, source,
	created_at, updated_at, closed_at
`

func (d *DB) GetPaperTradeForUser(userID string, id int64) (PaperTrade, error) {
	userID = normalizeUserID(userID)
	if id <= 0 {
		return PaperTrade{}, sql.ErrNoRows
	}
	return scanPaperTrade(d.sql.QueryRow(`
		SELECT `+paperTradeSelectColumns+`
		  FROM paper_trades
		 WHERE user_id = ? AND id = ?
		 LIMIT 1
	`, userID, id))
}

func (d *DB) ListPaperTradesForUser(userID, status string, limit int) ([]PaperTrade, error) {
	userID = normalizeUserID(userID)
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}

	status = strings.ToLower(strings.TrimSpace(status))
	var (
		rows *sql.Rows
		err  error
	)
	switch status {
	case "", "all":
		rows, err = d.sql.Query(`
			SELECT `+paperTradeSelectColumns+`
			  FROM paper_trades
			 WHERE user_id = ?
			 ORDER BY updated_at DESC, id DESC
			 LIMIT ?
		`, userID, limit)
	case PaperTradeStatusActive:
		rows, err = d.sql.Query(`
			SELECT `+paperTradeSelectColumns+`
			  FROM paper_trades
			 WHERE user_id = ?
			   AND status IN (?, ?, ?)
			 ORDER BY updated_at DESC, id DESC
			 LIMIT ?
		`, userID, PaperTradeStatusPlanned, PaperTradeStatusBought, PaperTradeStatusHauled, limit)
	default:
		normalized := normalizePaperTradeStatus(status)
		if normalized == "" {
			return nil, fmt.Errorf("invalid status")
		}
		rows, err = d.sql.Query(`
			SELECT `+paperTradeSelectColumns+`
			  FROM paper_trades
			 WHERE user_id = ? AND status = ?
			 ORDER BY updated_at DESC, id DESC
			 LIMIT ?
		`, userID, normalized, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]PaperTrade, 0)
	for rows.Next() {
		trade, err := scanPaperTrade(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, trade)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func applyPaperTradePatch(t *PaperTrade, patch PaperTradeUpdateInput, now string) error {
	if patch.Status != nil {
		status := normalizePaperTradeStatus(*patch.Status)
		if status == "" {
			return fmt.Errorf("invalid status")
		}
		t.Status = status
	}
	if patch.PlannedQuantity != nil {
		t.PlannedQuantity = *patch.PlannedQuantity
	}
	if patch.ActualQuantity != nil {
		t.ActualQuantity = *patch.ActualQuantity
	}
	if patch.PlannedBuyPrice != nil {
		t.PlannedBuyPrice = cleanPaperFloat(*patch.PlannedBuyPrice)
	}
	if patch.PlannedSellPrice != nil {
		t.PlannedSellPrice = cleanPaperFloat(*patch.PlannedSellPrice)
	}
	if patch.ActualBuyPrice != nil {
		t.ActualBuyPrice = cleanPaperFloat(*patch.ActualBuyPrice)
	}
	if patch.ActualSellPrice != nil {
		t.ActualSellPrice = cleanPaperFloat(*patch.ActualSellPrice)
	}
	if patch.PlannedProfitISK != nil {
		t.PlannedProfitISK = cleanPaperFloat(*patch.PlannedProfitISK)
	}
	if patch.PlannedROIPercent != nil {
		t.PlannedROIPercent = cleanPaperFloat(*patch.PlannedROIPercent)
	}
	if patch.FeesISK != nil {
		t.FeesISK = cleanPaperFloat(*patch.FeesISK)
	}
	if patch.HaulingCostISK != nil {
		t.HaulingCostISK = cleanPaperFloat(*patch.HaulingCostISK)
	}
	if patch.Notes != nil {
		t.Notes = cleanPaperText(*patch.Notes, 2048)
	}

	if t.ActualQuantity <= 0 && (t.Status == PaperTradeStatusBought || t.Status == PaperTradeStatusHauled || t.Status == PaperTradeStatusSold) {
		t.ActualQuantity = t.PlannedQuantity
	}
	if t.ActualBuyPrice <= 0 && (t.Status == PaperTradeStatusBought || t.Status == PaperTradeStatusHauled || t.Status == PaperTradeStatusSold) {
		t.ActualBuyPrice = t.PlannedBuyPrice
	}
	if t.ActualSellPrice <= 0 && t.Status == PaperTradeStatusSold {
		t.ActualSellPrice = t.PlannedSellPrice
	}
	if t.Status == PaperTradeStatusSold || t.Status == PaperTradeStatusCancelled {
		if strings.TrimSpace(t.ClosedAt) == "" {
			t.ClosedAt = now
		}
	} else {
		t.ClosedAt = ""
	}
	t.UpdatedAt = now

	if err := validatePaperTrade(*t); err != nil {
		return err
	}
	t.finalizeComputedFields()
	return nil
}

func (d *DB) UpdatePaperTradeForUser(userID string, id int64, patch PaperTradeUpdateInput) (PaperTrade, error) {
	trade, err := d.GetPaperTradeForUser(userID, id)
	if err != nil {
		return PaperTrade{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if err := applyPaperTradePatch(&trade, patch, now); err != nil {
		return PaperTrade{}, err
	}

	res, err := d.sql.Exec(`
		UPDATE paper_trades
		   SET status = ?,
		       planned_quantity = ?,
		       actual_quantity = ?,
		       planned_buy_price = ?,
		       planned_sell_price = ?,
		       actual_buy_price = ?,
		       actual_sell_price = ?,
		       planned_profit_isk = ?,
		       planned_roi_percent = ?,
		       fees_isk = ?,
		       hauling_cost_isk = ?,
		       notes = ?,
		       updated_at = ?,
		       closed_at = ?
		 WHERE user_id = ? AND id = ?
	`,
		trade.Status,
		trade.PlannedQuantity,
		trade.ActualQuantity,
		trade.PlannedBuyPrice,
		trade.PlannedSellPrice,
		trade.ActualBuyPrice,
		trade.ActualSellPrice,
		trade.PlannedProfitISK,
		trade.PlannedROIPercent,
		trade.FeesISK,
		trade.HaulingCostISK,
		trade.Notes,
		trade.UpdatedAt,
		trade.ClosedAt,
		trade.UserID,
		trade.ID,
	)
	if err != nil {
		return PaperTrade{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return PaperTrade{}, err
	}
	if affected == 0 {
		return PaperTrade{}, sql.ErrNoRows
	}
	return trade, nil
}

func (d *DB) DeletePaperTradeForUser(userID string, id int64) (int64, error) {
	userID = normalizeUserID(userID)
	res, err := d.sql.Exec(`DELETE FROM paper_trades WHERE user_id = ? AND id = ?`, userID, id)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
