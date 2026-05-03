package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"eve-flipper/internal/db"
	"eve-flipper/internal/engine"
	"eve-flipper/internal/esi"
)

func TestOrderBookSnapshotHandlers(t *testing.T) {
	database := openAPITestDB(t)
	if err := database.RecordMarketOrderSnapshot(esi.MarketOrderSnapshot{
		RegionID:   10000002,
		OrderType:  "all",
		Source:     "region_type",
		TypeID:     34,
		CapturedAt: time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
		Orders: []esi.MarketOrder{
			{TypeID: 34, LocationID: 60003760, SystemID: 30000142, Price: 5.0, VolumeRemain: 100, IsBuyOrder: false},
			{TypeID: 34, LocationID: 60003760, SystemID: 30000142, Price: 4.8, VolumeRemain: 50, IsBuyOrder: true},
		},
	}); err != nil {
		t.Fatalf("record orderbook snapshot: %v", err)
	}

	srv := &Server{db: database}
	handler := srv.Handler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/orderbook/snapshots?type_id=34", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("snapshots status=%d body=%s", rec.Code, rec.Body.String())
	}
	var snapsOut struct {
		Snapshots []db.OrderBookSnapshotMeta `json:"snapshots"`
		Count     int                        `json:"count"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&snapsOut); err != nil {
		t.Fatalf("decode snapshots: %v", err)
	}
	if snapsOut.Count != 1 || len(snapsOut.Snapshots) != 1 {
		t.Fatalf("snapshot count=%d len=%d", snapsOut.Count, len(snapsOut.Snapshots))
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/orderbook/snapshots/"+strconv.FormatInt(snapsOut.Snapshots[0].ID, 10)+"/levels?side=sell", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("levels status=%d body=%s", rec.Code, rec.Body.String())
	}
	var levelsOut struct {
		Levels []db.OrderBookLevel `json:"levels"`
		Count  int                 `json:"count"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&levelsOut); err != nil {
		t.Fatalf("decode levels: %v", err)
	}
	if levelsOut.Count != 1 || levelsOut.Levels[0].Side != "sell" {
		t.Fatalf("levels response = %#v", levelsOut)
	}
}

func TestBacktestFlipsRecordedOrderbook(t *testing.T) {
	database := openAPITestDB(t)
	now := time.Now().UTC().Add(-time.Hour)
	if err := database.RecordMarketOrderSnapshot(esi.MarketOrderSnapshot{
		RegionID:   1,
		OrderType:  "sell",
		Source:     "region",
		CapturedAt: now,
		Orders: []esi.MarketOrder{
			{TypeID: 34, LocationID: 100, SystemID: 10, Price: 5, VolumeRemain: 5, IsBuyOrder: false},
			{TypeID: 34, LocationID: 100, SystemID: 10, Price: 6, VolumeRemain: 10, IsBuyOrder: false},
		},
	}); err != nil {
		t.Fatalf("record source snapshot: %v", err)
	}
	if err := database.RecordMarketOrderSnapshot(esi.MarketOrderSnapshot{
		RegionID:   2,
		OrderType:  "buy",
		Source:     "region",
		CapturedAt: now.Add(time.Minute),
		Orders: []esi.MarketOrder{
			{TypeID: 34, LocationID: 200, SystemID: 20, Price: 8, VolumeRemain: 10, IsBuyOrder: true},
		},
	}); err != nil {
		t.Fatalf("record target snapshot: %v", err)
	}

	body, err := json.Marshal(map[string]any{
		"strategy_mode":              "instant_flip",
		"instant_price_mode":         "recorded_orderbook",
		"window_days":                1,
		"max_rows":                   10,
		"quantity_mode":              "scan",
		"orderbook_max_age_minutes":  5,
		"orderbook_cooldown_minutes": 1,
		"rows": []engine.FlipResult{{
			TypeID:         34,
			TypeName:       "Tritanium",
			BuyRegionID:    1,
			SellRegionID:   2,
			BuyLocationID:  100,
			SellLocationID: 200,
			BuyPrice:       5,
			SellPrice:      8,
			FilledQty:      10,
		}},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	srv := &Server{db: database}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/backtest/flips", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("recorded backtest status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out engine.FlipBacktestResult
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode backtest: %v", err)
	}
	if out.Summary.Trades != 1 || out.Ledger[0].PnL != 25 {
		t.Fatalf("backtest result = %#v", out)
	}
}

func TestOrderBookCoverageHandler(t *testing.T) {
	database := openAPITestDB(t)
	now := time.Now().UTC().Add(-time.Hour)
	if err := database.RecordMarketOrderSnapshot(esi.MarketOrderSnapshot{
		RegionID:   1,
		OrderType:  "sell",
		Source:     "region",
		CapturedAt: now,
		Orders: []esi.MarketOrder{
			{TypeID: 34, LocationID: 100, SystemID: 10, Price: 5, VolumeRemain: 5, IsBuyOrder: false},
		},
	}); err != nil {
		t.Fatalf("record source snapshot: %v", err)
	}
	if err := database.RecordMarketOrderSnapshot(esi.MarketOrderSnapshot{
		RegionID:   2,
		OrderType:  "buy",
		Source:     "region",
		CapturedAt: now.Add(time.Minute),
		Orders: []esi.MarketOrder{
			{TypeID: 34, LocationID: 200, SystemID: 20, Price: 8, VolumeRemain: 5, IsBuyOrder: true},
		},
	}); err != nil {
		t.Fatalf("record target snapshot: %v", err)
	}

	body, err := json.Marshal(map[string]any{
		"window_days":                1,
		"max_rows":                   10,
		"orderbook_max_age_minutes":  5,
		"orderbook_cooldown_minutes": 1,
		"rows": []engine.FlipResult{{
			TypeID:         34,
			TypeName:       "Tritanium",
			BuyRegionID:    1,
			SellRegionID:   2,
			BuyLocationID:  100,
			SellLocationID: 200,
		}},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	srv := &Server{db: database}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/orderbook/coverage", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("coverage status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out engine.OrderBookReplayCoverageResult
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode coverage: %v", err)
	}
	if out.Summary.RowsReady != 1 || out.Summary.PairedBooks != 1 || out.Rows[0].Status != "ready" {
		t.Fatalf("coverage result = %#v", out)
	}
}

func TestOrderBookMaintenanceHandlers(t *testing.T) {
	database := openAPITestDB(t)
	now := time.Now().UTC()
	if err := database.RecordMarketOrderSnapshot(esi.MarketOrderSnapshot{
		RegionID:   10000002,
		OrderType:  "sell",
		Source:     "region",
		CapturedAt: now.AddDate(0, 0, -90),
		Orders: []esi.MarketOrder{
			{TypeID: 34, LocationID: 60003760, SystemID: 30000142, Price: 5.0, VolumeRemain: 100, IsBuyOrder: false},
		},
	}); err != nil {
		t.Fatalf("record old snapshot: %v", err)
	}
	if err := database.RecordMarketOrderSnapshot(esi.MarketOrderSnapshot{
		RegionID:   10000002,
		OrderType:  "buy",
		Source:     "region",
		CapturedAt: now.Add(-time.Hour),
		Orders: []esi.MarketOrder{
			{TypeID: 35, LocationID: 60008494, SystemID: 30000144, Price: 8.0, VolumeRemain: 50, IsBuyOrder: true},
		},
	}); err != nil {
		t.Fatalf("record new snapshot: %v", err)
	}

	handler := (&Server{db: database}).Handler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/orderbook/stats?limit=2", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("stats status=%d body=%s", rec.Code, rec.Body.String())
	}
	var stats db.OrderBookStats
	if err := json.NewDecoder(rec.Body).Decode(&stats); err != nil {
		t.Fatalf("decode stats: %v", err)
	}
	if stats.SnapshotCount != 2 || stats.LevelCount != 2 || len(stats.TopTypes) != 2 {
		t.Fatalf("stats = %#v", stats)
	}

	body, err := json.Marshal(map[string]any{
		"keep_days": 30,
		"dry_run":   true,
	})
	if err != nil {
		t.Fatalf("marshal preview: %v", err)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/orderbook/cleanup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", rec.Code, rec.Body.String())
	}
	var preview db.OrderBookCleanupPlan
	if err := json.NewDecoder(rec.Body).Decode(&preview); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if !preview.DryRun || preview.SnapshotsDeleted != 1 || preview.LevelsDeleted != 1 {
		t.Fatalf("preview = %#v", preview)
	}

	body, err = json.Marshal(map[string]any{
		"keep_days": 30,
		"dry_run":   false,
	})
	if err != nil {
		t.Fatalf("marshal cleanup: %v", err)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/orderbook/cleanup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("cleanup status=%d body=%s", rec.Code, rec.Body.String())
	}
	var cleanup db.OrderBookCleanupPlan
	if err := json.NewDecoder(rec.Body).Decode(&cleanup); err != nil {
		t.Fatalf("decode cleanup: %v", err)
	}
	if cleanup.DryRun || cleanup.SnapshotsDeleted != 1 || cleanup.LevelsDeleted != 1 {
		t.Fatalf("cleanup = %#v", cleanup)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/orderbook/stats", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("stats after cleanup status=%d body=%s", rec.Code, rec.Body.String())
	}
	stats = db.OrderBookStats{}
	if err := json.NewDecoder(rec.Body).Decode(&stats); err != nil {
		t.Fatalf("decode stats after cleanup: %v", err)
	}
	if stats.SnapshotCount != 1 || stats.LevelCount != 1 || stats.TopTypes[0].TypeID != 35 {
		t.Fatalf("stats after cleanup = %#v", stats)
	}
}
