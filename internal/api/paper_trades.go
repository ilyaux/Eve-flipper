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

func (s *Server) handleAuthListPaperTrades(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	if s.db == nil {
		writeJSON(w, map[string]interface{}{
			"trades": []db.PaperTrade{},
			"count":  0,
		})
		return
	}

	status := strings.TrimSpace(r.URL.Query().Get("status"))
	limit := 200
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
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
	if trades == nil {
		trades = []db.PaperTrade{}
	}
	writeJSON(w, map[string]interface{}{
		"trades": trades,
		"count":  len(trades),
	})
}

func (s *Server) handleAuthCreatePaperTrade(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	if s.db == nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}

	var req db.PaperTradeCreateInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	trade, err := s.db.CreatePaperTradeForUser(userID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSONStatus(w, http.StatusCreated, map[string]interface{}{
		"ok":    true,
		"trade": trade,
	})
}

func (s *Server) handleAuthUpdatePaperTrade(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	if s.db == nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	id, err := strconv.ParseInt(strings.TrimSpace(r.PathValue("tradeID")), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid trade id")
		return
	}

	var req db.PaperTradeUpdateInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	trade, err := s.db.UpdatePaperTradeForUser(userID, id, req)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "paper trade not found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{
		"ok":    true,
		"trade": trade,
	})
}

func (s *Server) handleAuthDeletePaperTrade(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	if s.db == nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	id, err := strconv.ParseInt(strings.TrimSpace(r.PathValue("tradeID")), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid trade id")
		return
	}
	deleted, err := s.db.DeletePaperTradeForUser(userID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete paper trade")
		return
	}
	writeJSON(w, map[string]interface{}{
		"ok":      true,
		"deleted": deleted,
	})
}
