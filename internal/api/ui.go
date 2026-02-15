package api

import (
	"encoding/json"
	"log"
	"net/http"
)

// handleUIOpenMarket opens a market window in the EVE client for the given type_id.
// POST /api/ui/open-market
// Body: {"type_id": 34}
func (s *Server) handleUIOpenMarket(w http.ResponseWriter, r *http.Request) {
	sess := s.sessions.Get()
	if sess == nil || sess.AccessToken == "" {
		http.Error(w, `{"error":"not_logged_in"}`, http.StatusUnauthorized)
		return
	}

	var req struct {
		TypeID int64 `json:"type_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
		return
	}

	if req.TypeID <= 0 {
		http.Error(w, `{"error":"invalid_type_id"}`, http.StatusBadRequest)
		return
	}

	log.Printf("[API] OpenMarketWindow: type_id=%d, character_id=%d", req.TypeID, sess.CharacterID)
	if err := s.esi.OpenMarketWindow(req.TypeID, sess.AccessToken); err != nil {
		log.Printf("[API] OpenMarketWindow error: type_id=%d, err=%v", req.TypeID, err)
		http.Error(w, `{"error":"esi_error"}`, http.StatusInternalServerError)
		return
	}
	log.Printf("[API] OpenMarketWindow success: type_id=%d", req.TypeID)

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"success":true}`))
}

// handleUISetWaypoint sets a waypoint in the EVE client.
// POST /api/ui/set-waypoint
// Body: {"solar_system_id": 30000142, "clear_other_waypoints": true, "add_to_beginning": false}
func (s *Server) handleUISetWaypoint(w http.ResponseWriter, r *http.Request) {
	sess := s.sessions.Get()
	if sess == nil || sess.AccessToken == "" {
		http.Error(w, `{"error":"not_logged_in"}`, http.StatusUnauthorized)
		return
	}

	var req struct {
		SolarSystemID       int64 `json:"solar_system_id"`
		ClearOtherWaypoints bool  `json:"clear_other_waypoints"`
		AddToBeginning      bool  `json:"add_to_beginning"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
		return
	}

	if req.SolarSystemID <= 0 {
		http.Error(w, `{"error":"invalid_solar_system_id"}`, http.StatusBadRequest)
		return
	}

	log.Printf("[API] SetWaypoint: solar_system_id=%d, clear=%t, add_to_beginning=%t, character_id=%d",
		req.SolarSystemID, req.ClearOtherWaypoints, req.AddToBeginning, sess.CharacterID)
	if err := s.esi.SetWaypoint(req.SolarSystemID, req.ClearOtherWaypoints, req.AddToBeginning, sess.AccessToken); err != nil {
		log.Printf("[API] SetWaypoint error: solar_system_id=%d, err=%v", req.SolarSystemID, err)
		http.Error(w, `{"error":"esi_error"}`, http.StatusInternalServerError)
		return
	}
	log.Printf("[API] SetWaypoint success: solar_system_id=%d", req.SolarSystemID)

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"success":true}`))
}

// handleUIOpenContract opens a contract window in the EVE client.
// POST /api/ui/open-contract
// Body: {"contract_id": 123456789}
func (s *Server) handleUIOpenContract(w http.ResponseWriter, r *http.Request) {
	sess := s.sessions.Get()
	if sess == nil || sess.AccessToken == "" {
		http.Error(w, `{"error":"not_logged_in"}`, http.StatusUnauthorized)
		return
	}

	var req struct {
		ContractID int64 `json:"contract_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
		return
	}

	if req.ContractID <= 0 {
		http.Error(w, `{"error":"invalid_contract_id"}`, http.StatusBadRequest)
		return
	}

	log.Printf("[API] OpenContractWindow: contract_id=%d, character_id=%d", req.ContractID, sess.CharacterID)
	if err := s.esi.OpenContractWindow(req.ContractID, sess.AccessToken); err != nil {
		log.Printf("[API] OpenContractWindow error: contract_id=%d, err=%v", req.ContractID, err)
		http.Error(w, `{"error":"esi_error"}`, http.StatusInternalServerError)
		return
	}
	log.Printf("[API] OpenContractWindow success: contract_id=%d", req.ContractID)

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"success":true}`))
}
