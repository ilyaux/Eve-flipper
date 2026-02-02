package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"eve-flipper/internal/config"
	"eve-flipper/internal/db"
	"eve-flipper/internal/engine"
	"eve-flipper/internal/esi"
	"eve-flipper/internal/sde"
)

// Server is the HTTP API server that connects the ESI client, scanner engine, and database.
type Server struct {
	cfg     *config.Config
	sdeData *sde.Data
	scanner *engine.Scanner
	esi     *esi.Client
	db      *db.DB
	mu      sync.RWMutex
	ready   bool
}

// NewServer creates a Server with the given config, ESI client, and database.
func NewServer(cfg *config.Config, esiClient *esi.Client, database *db.DB) *Server {
	return &Server{
		cfg: cfg,
		esi: esiClient,
		db:  database,
	}
}

// SetSDE is called when SDE data finishes loading.
func (s *Server) SetSDE(data *sde.Data) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sdeData = data
	s.scanner = engine.NewScanner(data, s.esi)
	s.ready = true
}

func (s *Server) isReady() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ready
}

// Handler returns the HTTP handler with all API routes and CORS middleware.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /api/config", s.handleGetConfig)
	mux.HandleFunc("POST /api/config", s.handleSetConfig)
	mux.HandleFunc("GET /api/systems/autocomplete", s.handleAutocomplete)
	mux.HandleFunc("POST /api/scan", s.handleScan)
	mux.HandleFunc("POST /api/scan/multi-region", s.handleScanMultiRegion)
	mux.HandleFunc("POST /api/scan/contracts", s.handleScanContracts)
	mux.HandleFunc("POST /api/route/find", s.handleRouteFind)
	mux.HandleFunc("GET /api/watchlist", s.handleGetWatchlist)
	mux.HandleFunc("POST /api/watchlist", s.handleAddWatchlist)
	mux.HandleFunc("DELETE /api/watchlist/{typeID}", s.handleDeleteWatchlist)
	mux.HandleFunc("PUT /api/watchlist/{typeID}", s.handleUpdateWatchlist)
	mux.HandleFunc("GET /api/scan/history", s.handleGetHistory)
	return corsMiddleware(mux)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// --- Handlers ---

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	sdeLoaded := s.ready
	var systemCount, typeCount int
	if s.sdeData != nil {
		systemCount = len(s.sdeData.Systems)
		typeCount = len(s.sdeData.Types)
	}
	s.mu.RUnlock()

	esiOK := s.esi.HealthCheck()

	writeJSON(w, map[string]interface{}{
		"sde_loaded":  sdeLoaded,
		"sde_systems": systemCount,
		"sde_types":   typeCount,
		"esi_ok":      esiOK,
	})
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.cfg)
}

func (s *Server) handleSetConfig(w http.ResponseWriter, r *http.Request) {
	var patch map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, 400, "invalid json")
		return
	}

	if v, ok := patch["system_name"]; ok {
		json.Unmarshal(v, &s.cfg.SystemName)
	}
	if v, ok := patch["cargo_capacity"]; ok {
		json.Unmarshal(v, &s.cfg.CargoCapacity)
	}
	if v, ok := patch["buy_radius"]; ok {
		json.Unmarshal(v, &s.cfg.BuyRadius)
	}
	if v, ok := patch["sell_radius"]; ok {
		json.Unmarshal(v, &s.cfg.SellRadius)
	}
	if v, ok := patch["min_margin"]; ok {
		json.Unmarshal(v, &s.cfg.MinMargin)
	}
	if v, ok := patch["sales_tax_percent"]; ok {
		json.Unmarshal(v, &s.cfg.SalesTaxPercent)
	}
	if v, ok := patch["opacity"]; ok {
		json.Unmarshal(v, &s.cfg.Opacity)
	}

	s.db.SaveConfig(s.cfg)
	writeJSON(w, s.cfg)
}

func (s *Server) handleAutocomplete(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	if q == "" || !s.isReady() {
		writeJSON(w, map[string][]string{"systems": {}})
		return
	}

	s.mu.RLock()
	names := s.sdeData.SystemNames
	s.mu.RUnlock()

	var prefix, contains []string
	for _, name := range names {
		lower := strings.ToLower(name)
		if strings.HasPrefix(lower, q) {
			prefix = append(prefix, name)
		} else if strings.Contains(lower, q) {
			contains = append(contains, name)
		}
	}

	result := append(prefix, contains...)
	if len(result) > 15 {
		result = result[:15]
	}

	writeJSON(w, map[string][]string{"systems": result})
}

type scanRequest struct {
	SystemName      string  `json:"system_name"`
	CargoCapacity   float64 `json:"cargo_capacity"`
	BuyRadius       int     `json:"buy_radius"`
	SellRadius      int     `json:"sell_radius"`
	MinMargin       float64 `json:"min_margin"`
	SalesTaxPercent float64 `json:"sales_tax_percent"`
}

func (s *Server) parseScanParams(req scanRequest) (engine.ScanParams, error) {
	if !s.isReady() {
		return engine.ScanParams{}, fmt.Errorf("SDE not loaded yet")
	}

	s.mu.RLock()
	systemID, ok := s.sdeData.SystemByName[strings.ToLower(req.SystemName)]
	s.mu.RUnlock()
	if !ok {
		return engine.ScanParams{}, fmt.Errorf("system not found: %s", req.SystemName)
	}

	return engine.ScanParams{
		CurrentSystemID: systemID,
		CargoCapacity:   req.CargoCapacity,
		BuyRadius:       req.BuyRadius,
		SellRadius:      req.SellRadius,
		MinMargin:       req.MinMargin,
		SalesTaxPercent: req.SalesTaxPercent,
	}, nil
}

func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	var req scanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json")
		return
	}

	params, err := s.parseScanParams(req)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "streaming not supported")
		return
	}

	s.mu.RLock()
	scanner := s.scanner
	s.mu.RUnlock()

	log.Printf("[API] Scan starting: system=%d, cargo=%.0f, buyR=%d, sellR=%d, margin=%.1f, tax=%.1f",
		params.CurrentSystemID, params.CargoCapacity, params.BuyRadius, params.SellRadius, params.MinMargin, params.SalesTaxPercent)

	results, err := scanner.Scan(params, func(msg string) {
		line, _ := json.Marshal(map[string]string{"type": "progress", "message": msg})
		fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()
	})
	if err != nil {
		log.Printf("[API] Scan error: %v", err)
		line, _ := json.Marshal(map[string]string{"type": "error", "message": err.Error()})
		fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()
		return
	}

	log.Printf("[API] Scan complete: %d results", len(results))
	topProfit := 0.0
	for _, r := range results {
		if r.TotalProfit > topProfit {
			topProfit = r.TotalProfit
		}
	}
	scanID := s.db.InsertHistory("radius", req.SystemName, len(results), topProfit)
	go s.db.InsertFlipResults(scanID, results)

	line, marshalErr := json.Marshal(map[string]interface{}{"type": "result", "data": results, "count": len(results)})
	if marshalErr != nil {
		log.Printf("[API] Scan JSON marshal error: %v", marshalErr)
		errLine, _ := json.Marshal(map[string]string{"type": "error", "message": "JSON: " + marshalErr.Error()})
		fmt.Fprintf(w, "%s\n", errLine)
		flusher.Flush()
		return
	}
	fmt.Fprintf(w, "%s\n", line)
	flusher.Flush()
}

func (s *Server) handleScanMultiRegion(w http.ResponseWriter, r *http.Request) {
	var req scanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json")
		return
	}

	params, err := s.parseScanParams(req)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "streaming not supported")
		return
	}

	s.mu.RLock()
	scanner := s.scanner
	s.mu.RUnlock()

	log.Printf("[API] ScanMultiRegion starting: system=%d, cargo=%.0f, buyR=%d, sellR=%d",
		params.CurrentSystemID, params.CargoCapacity, params.BuyRadius, params.SellRadius)

	results, err := scanner.ScanMultiRegion(params, func(msg string) {
		line, _ := json.Marshal(map[string]string{"type": "progress", "message": msg})
		fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()
	})
	if err != nil {
		log.Printf("[API] ScanMultiRegion error: %v", err)
		line, _ := json.Marshal(map[string]string{"type": "error", "message": err.Error()})
		fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()
		return
	}

	log.Printf("[API] ScanMultiRegion complete: %d results", len(results))
	tp := 0.0
	for _, r := range results {
		if r.TotalProfit > tp {
			tp = r.TotalProfit
		}
	}
	scanID := s.db.InsertHistory("region", req.SystemName, len(results), tp)
	go s.db.InsertFlipResults(scanID, results)

	line, marshalErr := json.Marshal(map[string]interface{}{"type": "result", "data": results, "count": len(results)})
	if marshalErr != nil {
		log.Printf("[API] ScanMultiRegion JSON marshal error: %v", marshalErr)
		errLine, _ := json.Marshal(map[string]string{"type": "error", "message": "JSON: " + marshalErr.Error()})
		fmt.Fprintf(w, "%s\n", errLine)
		flusher.Flush()
		return
	}
	fmt.Fprintf(w, "%s\n", line)
	flusher.Flush()
}

func (s *Server) handleScanContracts(w http.ResponseWriter, r *http.Request) {
	var req scanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json")
		return
	}

	params, err := s.parseScanParams(req)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "streaming not supported")
		return
	}

	s.mu.RLock()
	scanner := s.scanner
	s.mu.RUnlock()

	log.Printf("[API] ScanContracts starting: system=%d, buyR=%d, margin=%.1f, tax=%.1f",
		params.CurrentSystemID, params.BuyRadius, params.MinMargin, params.SalesTaxPercent)

	results, err := scanner.ScanContracts(params, func(msg string) {
		line, _ := json.Marshal(map[string]string{"type": "progress", "message": msg})
		fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()
	})
	if err != nil {
		log.Printf("[API] ScanContracts error: %v", err)
		line, _ := json.Marshal(map[string]string{"type": "error", "message": err.Error()})
		fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()
		return
	}

	log.Printf("[API] ScanContracts complete: %d results", len(results))
	tp := 0.0
	for _, r := range results {
		if r.Profit > tp {
			tp = r.Profit
		}
	}
	scanID := s.db.InsertHistory("contracts", req.SystemName, len(results), tp)
	go s.db.InsertContractResults(scanID, results)

	line, marshalErr := json.Marshal(map[string]interface{}{"type": "result", "data": results, "count": len(results)})
	if marshalErr != nil {
		log.Printf("[API] ScanContracts JSON marshal error: %v", marshalErr)
		errLine, _ := json.Marshal(map[string]string{"type": "error", "message": "JSON: " + marshalErr.Error()})
		fmt.Fprintf(w, "%s\n", errLine)
		flusher.Flush()
		return
	}
	fmt.Fprintf(w, "%s\n", line)
	flusher.Flush()
}

func (s *Server) handleRouteFind(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SystemName      string  `json:"system_name"`
		CargoCapacity   float64 `json:"cargo_capacity"`
		MinMargin       float64 `json:"min_margin"`
		SalesTaxPercent float64 `json:"sales_tax_percent"`
		MinHops         int     `json:"min_hops"`
		MaxHops         int     `json:"max_hops"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json")
		return
	}
	if !s.isReady() {
		writeError(w, 503, "SDE not loaded yet")
		return
	}
	if req.MinHops < 1 {
		req.MinHops = 2
	}
	if req.MaxHops < req.MinHops {
		req.MaxHops = req.MinHops + 2
	}
	if req.MaxHops > 10 {
		req.MaxHops = 10
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "streaming not supported")
		return
	}

	s.mu.RLock()
	scanner := s.scanner
	s.mu.RUnlock()

	params := engine.RouteParams{
		SystemName:      req.SystemName,
		CargoCapacity:   req.CargoCapacity,
		MinMargin:       req.MinMargin,
		SalesTaxPercent: req.SalesTaxPercent,
		MinHops:         req.MinHops,
		MaxHops:         req.MaxHops,
	}

	log.Printf("[API] RouteFind: system=%s, cargo=%.0f, margin=%.1f, hops=%d-%d",
		req.SystemName, req.CargoCapacity, req.MinMargin, req.MinHops, req.MaxHops)

	results, err := scanner.FindRoutes(params, func(msg string) {
		line, _ := json.Marshal(map[string]string{"type": "progress", "message": msg})
		fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()
	})
	if err != nil {
		log.Printf("[API] RouteFind error: %v", err)
		line, _ := json.Marshal(map[string]string{"type": "error", "message": err.Error()})
		fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()
		return
	}

	log.Printf("[API] RouteFind complete: %d routes", len(results))
	tp := 0.0
	for _, r := range results {
		if r.TotalProfit > tp {
			tp = r.TotalProfit
		}
	}
	s.db.InsertHistory("route", req.SystemName, len(results), tp)

	line, marshalErr := json.Marshal(map[string]interface{}{"type": "result", "data": results, "count": len(results)})
	if marshalErr != nil {
		log.Printf("[API] RouteFind JSON marshal error: %v", marshalErr)
		errLine, _ := json.Marshal(map[string]string{"type": "error", "message": "JSON: " + marshalErr.Error()})
		fmt.Fprintf(w, "%s\n", errLine)
		flusher.Flush()
		return
	}
	fmt.Fprintf(w, "%s\n", line)
	flusher.Flush()
}

// --- Watchlist ---

func (s *Server) handleGetWatchlist(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.db.GetWatchlist())
}

func (s *Server) handleAddWatchlist(w http.ResponseWriter, r *http.Request) {
	var item config.WatchlistItem
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		writeError(w, 400, "invalid json")
		return
	}
	item.AddedAt = time.Now().Format(time.RFC3339)
	s.db.AddWatchlistItem(item)
	writeJSON(w, s.db.GetWatchlist())
}

func (s *Server) handleDeleteWatchlist(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("typeID")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, 400, "invalid type_id")
		return
	}
	s.db.DeleteWatchlistItem(int32(id))
	writeJSON(w, s.db.GetWatchlist())
}

func (s *Server) handleUpdateWatchlist(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("typeID")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, 400, "invalid type_id")
		return
	}
	var body struct {
		AlertMinMargin float64 `json:"alert_min_margin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, 400, "invalid json")
		return
	}
	s.db.UpdateWatchlistItem(int32(id), body.AlertMinMargin)
	writeJSON(w, s.db.GetWatchlist())
}

// --- Scan History ---

func (s *Server) handleGetHistory(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.db.GetHistory(50))
}
