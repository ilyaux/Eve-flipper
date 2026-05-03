package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"eve-flipper/internal/db"
)

func parseOptionalInt32Query(r *http.Request, key string) (int32, bool, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return 0, false, nil
	}
	v, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || v < 0 {
		return 0, false, err
	}
	return int32(v), true, nil
}

func parseOptionalInt64Query(r *http.Request, key string) (int64, bool, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return 0, false, nil
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v < 0 {
		return 0, false, err
	}
	return v, true, nil
}

func parseOptionalLimitQuery(r *http.Request, defaultLimit, maxLimit int) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return defaultLimit, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return 0, err
	}
	if v > maxLimit {
		v = maxLimit
	}
	return v, nil
}

func (s *Server) handleOrderBookSnapshots(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		writeJSON(w, map[string]interface{}{
			"snapshots": []db.OrderBookSnapshotMeta{},
			"count":     0,
		})
		return
	}

	regionID, _, err := parseOptionalInt32Query(r, "region_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid region_id")
		return
	}
	typeID, _, err := parseOptionalInt32Query(r, "type_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid type_id")
		return
	}
	locationID, _, err := parseOptionalInt64Query(r, "location_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid location_id")
		return
	}
	limit, err := parseOptionalLimitQuery(r, 100, 1000)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid limit")
		return
	}

	snapshots, err := s.db.ListOrderBookSnapshots(db.OrderBookSnapshotFilter{
		Source:     r.URL.Query().Get("source"),
		RegionID:   regionID,
		OrderType:  r.URL.Query().Get("order_type"),
		TypeID:     typeID,
		LocationID: locationID,
		Limit:      limit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list orderbook snapshots")
		return
	}
	if snapshots == nil {
		snapshots = []db.OrderBookSnapshotMeta{}
	}
	writeJSON(w, map[string]interface{}{
		"snapshots": snapshots,
		"count":     len(snapshots),
	})
}

func (s *Server) handleOrderBookLevels(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		writeJSON(w, map[string]interface{}{
			"snapshot": nil,
			"levels":   []db.OrderBookLevel{},
			"count":    0,
		})
		return
	}
	snapshotID, err := strconv.ParseInt(strings.TrimSpace(r.PathValue("snapshotID")), 10, 64)
	if err != nil || snapshotID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid snapshot id")
		return
	}

	typeID, _, err := parseOptionalInt32Query(r, "type_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid type_id")
		return
	}
	locationID, _, err := parseOptionalInt64Query(r, "location_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid location_id")
		return
	}
	limit, err := parseOptionalLimitQuery(r, 5000, 50000)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid limit")
		return
	}

	snapshot, err := s.db.GetOrderBookSnapshot(snapshotID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "orderbook snapshot not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load orderbook snapshot")
		return
	}
	levels, err := s.db.GetOrderBookLevels(snapshotID, db.OrderBookLevelFilter{
		TypeID:     typeID,
		LocationID: locationID,
		Side:       r.URL.Query().Get("side"),
		Limit:      limit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load orderbook levels")
		return
	}
	if levels == nil {
		levels = []db.OrderBookLevel{}
	}
	writeJSON(w, map[string]interface{}{
		"snapshot": snapshot,
		"levels":   levels,
		"count":    len(levels),
	})
}

func (s *Server) handleOrderBookStats(w http.ResponseWriter, r *http.Request) {
	limit, err := parseOptionalLimitQuery(r, 10, 50)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid limit")
		return
	}
	if s.db == nil {
		writeJSON(w, db.OrderBookStats{
			TopTypes:     []db.OrderBookStatsType{},
			TopLocations: []db.OrderBookStatsLocation{},
		})
		return
	}
	stats, err := s.db.GetOrderBookStats(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load orderbook stats")
		return
	}
	if stats.TopTypes == nil {
		stats.TopTypes = []db.OrderBookStatsType{}
	}
	if stats.TopLocations == nil {
		stats.TopLocations = []db.OrderBookStatsLocation{}
	}
	writeJSON(w, stats)
}

type orderBookCleanupRequest struct {
	KeepDays int  `json:"keep_days"`
	DryRun   bool `json:"dry_run"`
	Vacuum   bool `json:"vacuum"`
}

func (s *Server) handleOrderBookCleanup(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		writeError(w, http.StatusServiceUnavailable, "orderbook database not ready")
		return
	}
	var req orderBookCleanupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.KeepDays <= 0 {
		writeError(w, http.StatusBadRequest, "keep_days must be positive")
		return
	}
	plan, err := s.db.CleanupOrderBookSnapshots(req.KeepDays, req.DryRun, req.Vacuum)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, plan)
}
