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

	"eve-flipper/internal/auth"
	"eve-flipper/internal/config"
	"eve-flipper/internal/db"
	"eve-flipper/internal/engine"
	"eve-flipper/internal/esi"
	"eve-flipper/internal/sde"
	"eve-flipper/internal/zkillboard"
)

// Server is the HTTP API server that connects the ESI client, scanner engine, and database.
type Server struct {
	cfg              *config.Config
	sdeData          *sde.Data
	scanner          *engine.Scanner
	industryAnalyzer *engine.IndustryAnalyzer
	demandAnalyzer   *zkillboard.DemandAnalyzer
	esi              *esi.Client
	db               *db.DB
	sso              *auth.SSOConfig
	sessions         *auth.SessionStore
	mu               sync.RWMutex
	ready            bool
	ssoState         string // CSRF state for current login flow
}

// NewServer creates a Server with the given config, ESI client, and database.
func NewServer(cfg *config.Config, esiClient *esi.Client, database *db.DB, ssoConfig *auth.SSOConfig, sessions *auth.SessionStore) *Server {
	return &Server{
		cfg:      cfg,
		esi:      esiClient,
		db:       database,
		sso:      ssoConfig,
		sessions: sessions,
	}
}

// SetSDE is called when SDE data finishes loading.
func (s *Server) SetSDE(data *sde.Data) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sdeData = data
	scanner := engine.NewScanner(data, s.esi)
	scanner.History = s.db
	s.scanner = scanner
	s.industryAnalyzer = engine.NewIndustryAnalyzer(data, s.esi)
	
	// Initialize demand analyzer with region names from SDE
	s.demandAnalyzer = zkillboard.NewDemandAnalyzer(data.RegionNames())
	
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
	mux.HandleFunc("GET /api/regions/autocomplete", s.handleRegionAutocomplete)
	mux.HandleFunc("POST /api/scan", s.handleScan)
	mux.HandleFunc("POST /api/scan/multi-region", s.handleScanMultiRegion)
	mux.HandleFunc("POST /api/scan/contracts", s.handleScanContracts)
	mux.HandleFunc("POST /api/route/find", s.handleRouteFind)
	mux.HandleFunc("GET /api/watchlist", s.handleGetWatchlist)
	mux.HandleFunc("POST /api/watchlist", s.handleAddWatchlist)
	mux.HandleFunc("DELETE /api/watchlist/{typeID}", s.handleDeleteWatchlist)
	mux.HandleFunc("PUT /api/watchlist/{typeID}", s.handleUpdateWatchlist)
	mux.HandleFunc("POST /api/scan/station", s.handleScanStation)
	mux.HandleFunc("GET /api/stations", s.handleGetStations)
	mux.HandleFunc("GET /api/scan/history", s.handleGetHistory)
	mux.HandleFunc("GET /api/scan/history/{id}", s.handleGetHistoryByID)
	mux.HandleFunc("GET /api/scan/history/{id}/results", s.handleGetHistoryResults)
	mux.HandleFunc("DELETE /api/scan/history/{id}", s.handleDeleteHistory)
	mux.HandleFunc("POST /api/scan/history/clear", s.handleClearHistory)
	// Auth
	mux.HandleFunc("GET /api/auth/login", s.handleAuthLogin)
	mux.HandleFunc("GET /api/auth/callback", s.handleAuthCallback)
	mux.HandleFunc("GET /api/auth/status", s.handleAuthStatus)
	mux.HandleFunc("POST /api/auth/logout", s.handleAuthLogout)
	mux.HandleFunc("GET /api/auth/character", s.handleAuthCharacter)
	mux.HandleFunc("GET /api/auth/location", s.handleAuthLocation)
	// Industry
	mux.HandleFunc("POST /api/industry/analyze", s.handleIndustryAnalyze)
	mux.HandleFunc("GET /api/industry/search", s.handleIndustrySearch)
	mux.HandleFunc("GET /api/industry/systems", s.handleIndustrySystems)
	mux.HandleFunc("GET /api/industry/status", s.handleIndustryStatus)
	mux.HandleFunc("POST /api/execution/plan", s.handleExecutionPlan)
	// Demand / War Tracker
	mux.HandleFunc("GET /api/demand/regions", s.handleDemandRegions)
	mux.HandleFunc("GET /api/demand/hotzones", s.handleDemandHotZones)
	mux.HandleFunc("GET /api/demand/region/{regionID}", s.handleDemandRegion)
	mux.HandleFunc("GET /api/demand/opportunities/{regionID}", s.handleDemandOpportunities)
	mux.HandleFunc("POST /api/demand/refresh", s.handleDemandRefresh)
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
	_, lastOK := s.esi.HealthStatus()

	result := map[string]interface{}{
		"sde_loaded":  sdeLoaded,
		"sde_systems": systemCount,
		"sde_types":   typeCount,
		"esi_ok":      esiOK,
	}

	// Add last successful ESI connection time if available
	if !lastOK.IsZero() {
		result["esi_last_ok"] = lastOK.Unix()
	}

	writeJSON(w, result)
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

func (s *Server) handleRegionAutocomplete(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	if q == "" || !s.isReady() {
		writeJSON(w, map[string][]string{"regions": {}})
		return
	}

	s.mu.RLock()
	regions := s.sdeData.Regions
	s.mu.RUnlock()

	var prefix, contains []string
	for _, region := range regions {
		lower := strings.ToLower(region.Name)
		if strings.HasPrefix(lower, q) {
			prefix = append(prefix, region.Name)
		} else if strings.Contains(lower, q) {
			contains = append(contains, region.Name)
		}
	}

	result := append(prefix, contains...)
	if len(result) > 15 {
		result = result[:15]
	}

	writeJSON(w, map[string][]string{"regions": result})
}

type scanRequest struct {
	SystemName      string  `json:"system_name"`
	CargoCapacity   float64 `json:"cargo_capacity"`
	BuyRadius       int     `json:"buy_radius"`
	SellRadius      int     `json:"sell_radius"`
	MinMargin       float64 `json:"min_margin"`
	SalesTaxPercent float64 `json:"sales_tax_percent"`
	// Advanced filters
	MinDailyVolume   int64   `json:"min_daily_volume"`
	MaxInvestment    float64 `json:"max_investment"`
	MaxResults       int     `json:"max_results"`
	MinRouteSecurity float64 `json:"min_route_security"` // 0 = all; 0.45 = highsec only; 0.7 = min 0.7
	TargetRegion     string  `json:"target_region"`      // Empty = search all by radius; region name = search only in that region
	// Contract-specific filters
	MinContractPrice  float64 `json:"min_contract_price"`
	MaxContractMargin float64 `json:"max_contract_margin"`
	MinPricedRatio    float64 `json:"min_priced_ratio"`
	RequireHistory    bool    `json:"require_history"`
}

func (s *Server) parseScanParams(req scanRequest) (engine.ScanParams, error) {
	if !s.isReady() {
		return engine.ScanParams{}, fmt.Errorf("SDE not loaded yet")
	}

	s.mu.RLock()
	systemID, ok := s.sdeData.SystemByName[strings.ToLower(req.SystemName)]
	
	// Parse target region if specified
	var targetRegionID int32
	if req.TargetRegion != "" {
		if regionID, ok := s.sdeData.RegionByName[strings.ToLower(req.TargetRegion)]; ok {
			targetRegionID = regionID
		}
	}
	s.mu.RUnlock()
	
	if !ok {
		return engine.ScanParams{}, fmt.Errorf("system not found: %s", req.SystemName)
	}

	return engine.ScanParams{
		CurrentSystemID:   systemID,
		CargoCapacity:     req.CargoCapacity,
		BuyRadius:         req.BuyRadius,
		SellRadius:        req.SellRadius,
		MinMargin:         req.MinMargin,
		SalesTaxPercent:   req.SalesTaxPercent,
		MinDailyVolume:    req.MinDailyVolume,
		MaxInvestment:     req.MaxInvestment,
		MinRouteSecurity:  req.MinRouteSecurity,
		MaxResults:        req.MaxResults,
		TargetRegionID:    targetRegionID,
		MinContractPrice:  req.MinContractPrice,
		MaxContractMargin: req.MaxContractMargin,
		MinPricedRatio:    req.MinPricedRatio,
		RequireHistory:    req.RequireHistory,
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

	startTime := time.Now()

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

	durationMs := time.Since(startTime).Milliseconds()
	log.Printf("[API] Scan complete: %d results in %dms", len(results), durationMs)

	topProfit := 0.0
	totalProfit := 0.0
	for _, r := range results {
		if r.TotalProfit > topProfit {
			topProfit = r.TotalProfit
		}
		totalProfit += r.TotalProfit
	}
	scanID := s.db.InsertHistoryFull("radius", req.SystemName, len(results), topProfit, totalProfit, durationMs, req)
	go s.db.InsertFlipResults(scanID, results)

	line, marshalErr := json.Marshal(map[string]interface{}{"type": "result", "data": results, "count": len(results), "scan_id": scanID})
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

	startTime := time.Now()

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

	durationMs := time.Since(startTime).Milliseconds()
	log.Printf("[API] ScanMultiRegion complete: %d results in %dms", len(results), durationMs)

	topProfit := 0.0
	totalProfit := 0.0
	for _, r := range results {
		if r.TotalProfit > topProfit {
			topProfit = r.TotalProfit
		}
		totalProfit += r.TotalProfit
	}
	scanID := s.db.InsertHistoryFull("region", req.SystemName, len(results), topProfit, totalProfit, durationMs, req)
	go s.db.InsertFlipResults(scanID, results)

	line, marshalErr := json.Marshal(map[string]interface{}{"type": "result", "data": results, "count": len(results), "scan_id": scanID})
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

	startTime := time.Now()

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

	durationMs := time.Since(startTime).Milliseconds()
	log.Printf("[API] ScanContracts complete: %d results in %dms", len(results), durationMs)

	topProfit := 0.0
	totalProfit := 0.0
	for _, r := range results {
		if r.Profit > topProfit {
			topProfit = r.Profit
		}
		totalProfit += r.Profit
	}
	scanID := s.db.InsertHistoryFull("contracts", req.SystemName, len(results), topProfit, totalProfit, durationMs, req)
	go s.db.InsertContractResults(scanID, results)

	line, marshalErr := json.Marshal(map[string]interface{}{"type": "result", "data": results, "count": len(results), "scan_id": scanID})
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
		SystemName       string  `json:"system_name"`
		CargoCapacity    float64 `json:"cargo_capacity"`
		MinMargin        float64 `json:"min_margin"`
		SalesTaxPercent  float64 `json:"sales_tax_percent"`
		MinHops          int     `json:"min_hops"`
		MaxHops          int     `json:"max_hops"`
		MaxResults       int     `json:"max_results"`
		MinRouteSecurity float64 `json:"min_route_security"` // 0 = all; 0.45 = highsec only; 0.7 = min 0.7
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
		SystemName:       req.SystemName,
		CargoCapacity:    req.CargoCapacity,
		MinMargin:        req.MinMargin,
		SalesTaxPercent:  req.SalesTaxPercent,
		MinHops:          req.MinHops,
		MaxHops:          req.MaxHops,
		MaxResults:       req.MaxResults,
		MinRouteSecurity: req.MinRouteSecurity,
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

// --- Station Trading ---

func (s *Server) handleScanStation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StationID       int64   `json:"station_id"`       // 0 = all stations
		RegionID        int32   `json:"region_id"`         // required
		SystemName      string  `json:"system_name"`       // for radius-based scan
		Radius          int     `json:"radius"`            // 0 = single system
		MinMargin       float64 `json:"min_margin"`
		SalesTaxPercent float64 `json:"sales_tax_percent"`
		BrokerFee       float64 `json:"broker_fee"`
		MinDailyVolume  int64   `json:"min_daily_volume"`
		MaxResults      int     `json:"max_results"`
		// EVE Guru Profit Filters
		MinItemProfit   float64 `json:"min_item_profit"`
		MinDemandPerDay float64 `json:"min_demand_per_day"`
		// Risk Profile
		AvgPricePeriod     int     `json:"avg_price_period"`
		MinPeriodROI       float64 `json:"min_period_roi"`
		BvSRatioMin        float64 `json:"bvs_ratio_min"`
		BvSRatioMax        float64 `json:"bvs_ratio_max"`
		MaxPVI             float64 `json:"max_pvi"`
		MaxSDS             int     `json:"max_sds"`
		LimitBuyToPriceLow bool    `json:"limit_buy_to_price_low"`
		FlagExtremePrices  bool    `json:"flag_extreme_prices"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json")
		return
	}
	if !s.isReady() {
		writeError(w, 503, "SDE not loaded yet")
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

	progressFn := func(msg string) {
		line, _ := json.Marshal(map[string]string{"type": "progress", "message": msg})
		fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()
	}

	// Build StationIDs and RegionIDs based on request params
	s.mu.RLock()
	sdeData := s.sdeData
	s.mu.RUnlock()

	stationIDs := make(map[int64]bool)
	regionIDs := make(map[int32]bool)
	historyLabel := ""

	if req.Radius > 0 && req.SystemName != "" {
		// Radius-based scan: find all systems within radius, collect their stations
		systemID, ok := sdeData.SystemByName[strings.ToLower(req.SystemName)]
		if !ok {
			writeError(w, 400, "unknown system")
			return
		}
		systems := sdeData.Universe.SystemsWithinRadius(systemID, req.Radius)
		for _, st := range sdeData.Stations {
			if _, inRange := systems[st.SystemID]; inRange {
				stationIDs[st.ID] = true
			}
		}
		for sysID := range systems {
			if sys, ok2 := sdeData.Systems[sysID]; ok2 {
				regionIDs[sys.RegionID] = true
			}
		}
		historyLabel = fmt.Sprintf("%s +%d jumps", req.SystemName, req.Radius)
	} else if req.StationID > 0 {
		// Single station
		stationIDs[req.StationID] = true
		regionIDs[req.RegionID] = true
		historyLabel = fmt.Sprintf("Station %d", req.StationID)
	} else {
		// All stations in region
		regionIDs[req.RegionID] = true
		historyLabel = fmt.Sprintf("Region %d (all)", req.RegionID)
	}

	log.Printf("[API] ScanStation starting: stations=%d, regions=%d, margin=%.1f, tax=%.1f, broker=%.1f",
		len(stationIDs), len(regionIDs), req.MinMargin, req.SalesTaxPercent, req.BrokerFee)

	startTime := time.Now()

	// Scan each region and merge results
	var allResults []engine.StationTrade
	for regionID := range regionIDs {
		params := engine.StationTradeParams{
			StationIDs:         stationIDs,
			RegionID:           regionID,
			MinMargin:          req.MinMargin,
			SalesTaxPercent:    req.SalesTaxPercent,
			BrokerFee:          req.BrokerFee,
			MinDailyVolume:     req.MinDailyVolume,
			MaxResults:         req.MaxResults,
			MinItemProfit:      req.MinItemProfit,
			MinDemandPerDay:    req.MinDemandPerDay,
			AvgPricePeriod:     req.AvgPricePeriod,
			MinPeriodROI:       req.MinPeriodROI,
			BvSRatioMin:        req.BvSRatioMin,
			BvSRatioMax:        req.BvSRatioMax,
			MaxPVI:             req.MaxPVI,
			MaxSDS:             req.MaxSDS,
			LimitBuyToPriceLow: req.LimitBuyToPriceLow,
			FlagExtremePrices:  req.FlagExtremePrices,
		}
		// For "all stations in region" mode, pass nil StationIDs
		if req.StationID == 0 && req.Radius == 0 {
			params.StationIDs = nil
		}

		results, err := scanner.ScanStationTrades(params, progressFn)
		if err != nil {
			log.Printf("[API] ScanStation error (region %d): %v", regionID, err)
			line, _ := json.Marshal(map[string]string{"type": "error", "message": err.Error()})
			fmt.Fprintf(w, "%s\n", line)
			flusher.Flush()
			return
		}
		allResults = append(allResults, results...)
	}

	durationMs := time.Since(startTime).Milliseconds()
	log.Printf("[API] ScanStation complete: %d results in %dms", len(allResults), durationMs)

	// Calculate totals
	topProfit := 0.0
	totalProfit := 0.0
	for _, r := range allResults {
		if r.TotalProfit > topProfit {
			topProfit = r.TotalProfit
		}
		totalProfit += r.TotalProfit
	}

	// Save to history with full params
	scanID := s.db.InsertHistoryFull("station", historyLabel, len(allResults), topProfit, totalProfit, durationMs, req)
	if scanID > 0 {
		s.db.InsertStationResults(scanID, allResults)
	}

	line, marshalErr := json.Marshal(map[string]interface{}{"type": "result", "data": allResults, "count": len(allResults), "scan_id": scanID})
	if marshalErr != nil {
		log.Printf("[API] ScanStation JSON marshal error: %v", marshalErr)
		errLine, _ := json.Marshal(map[string]string{"type": "error", "message": "JSON: " + marshalErr.Error()})
		fmt.Fprintf(w, "%s\n", errLine)
		flusher.Flush()
		return
	}
	fmt.Fprintf(w, "%s\n", line)
	flusher.Flush()
}

func (s *Server) handleGetStations(w http.ResponseWriter, r *http.Request) {
	systemName := strings.TrimSpace(r.URL.Query().Get("system"))
	if systemName == "" || !s.isReady() {
		writeJSON(w, []interface{}{})
		return
	}

	s.mu.RLock()
	systemID, ok := s.sdeData.SystemByName[strings.ToLower(systemName)]
	stations := s.sdeData.Stations
	s.mu.RUnlock()

	if !ok {
		writeJSON(w, []interface{}{})
		return
	}

	type stationInfo struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		SystemID int32  `json:"system_id"`
		RegionID int32  `json:"region_id"`
	}

	// Collect station IDs for this system
	var stationIDs []int64
	for _, st := range stations {
		if st.SystemID == systemID {
			stationIDs = append(stationIDs, st.ID)
		}
	}

	// Prefetch station names from ESI (uses cache)
	idMap := make(map[int64]bool, len(stationIDs))
	for _, id := range stationIDs {
		idMap[id] = true
	}
	s.esi.PrefetchStationNames(idMap)

	regionID := int32(0)
	if sys, ok2 := s.sdeData.Systems[systemID]; ok2 {
		regionID = sys.RegionID
	}

	var result []stationInfo
	for _, id := range stationIDs {
		result = append(result, stationInfo{
			ID:       id,
			Name:     s.esi.StationName(id),
			SystemID: systemID,
			RegionID: regionID,
		})
	}

	writeJSON(w, result)
}

func (s *Server) handleExecutionPlan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TypeID     int32 `json:"type_id"`
		RegionID   int32 `json:"region_id"`
		LocationID int64 `json:"location_id"` // 0 = whole region
		Quantity   int32 `json:"quantity"`
		IsBuy      bool  `json:"is_buy"`
		ImpactDays int   `json:"impact_days"` // 0 = use engine default (e.g. 30); from station trading "Period (days)"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json")
		return
	}
	if req.RegionID == 0 || req.TypeID == 0 || req.Quantity <= 0 {
		writeError(w, 400, "region_id, type_id and positive quantity required")
		return
	}

	// For buy we need sell orders (we walk the ask side); for sell we need buy orders (bid side)
	orderType := "sell"
	if !req.IsBuy {
		orderType = "buy"
	}
	orders, err := s.esi.FetchRegionOrders(req.RegionID, orderType)
	if err != nil {
		log.Printf("[API] execution/plan FetchRegionOrders: %v", err)
		writeError(w, 502, "failed to fetch market orders")
		return
	}

	// Filter by type and optional location
	var filtered []esi.MarketOrder
	for _, o := range orders {
		if o.TypeID != req.TypeID {
			continue
		}
		if req.LocationID != 0 && o.LocationID != req.LocationID {
			continue
		}
		filtered = append(filtered, o)
	}

	result := engine.ComputeExecutionPlan(filtered, req.Quantity, req.IsBuy)

	// When market history is available, add impact calibration (Kyle's λ, √V, TWAP n*)
	if s.db != nil {
		history, ok := s.db.GetMarketHistory(req.RegionID, req.TypeID)
		if !ok {
			entries, err := s.esi.FetchMarketHistory(req.RegionID, req.TypeID)
			if err == nil && len(entries) > 0 {
				s.db.SetMarketHistory(req.RegionID, req.TypeID, entries)
				history = entries
			}
		}
		if len(history) >= 5 {
			impactDays := req.ImpactDays
			if impactDays <= 0 {
				impactDays = engine.DefaultImpactDays
			}
			if impactDays > 365 {
				impactDays = 365
			}
			params := engine.CalibrateImpact(history, impactDays)
			if params.Valid {
				est := engine.EstimateImpact(params, float64(req.Quantity), engine.DefaultTWAPHorizonDays)
				result.Impact = &est
			}
		}
	}

	writeJSON(w, result)
}

// --- Scan History ---

func (s *Server) handleGetHistory(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	writeJSON(w, s.db.GetHistory(limit))
}

func (s *Server) handleGetHistoryByID(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}
	record := s.db.GetHistoryByID(id)
	if record == nil {
		writeError(w, 404, "not found")
		return
	}
	writeJSON(w, record)
}

func (s *Server) handleGetHistoryResults(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}

	record := s.db.GetHistoryByID(id)
	if record == nil {
		writeError(w, 404, "not found")
		return
	}

	var results interface{}
	switch record.Tab {
	case "station":
		results = s.db.GetStationResults(id)
	case "contracts":
		results = s.db.GetContractResults(id)
	default:
		results = s.db.GetFlipResults(id)
	}

	writeJSON(w, map[string]interface{}{
		"scan":    record,
		"results": results,
	})
}

func (s *Server) handleDeleteHistory(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}
	if err := s.db.DeleteHistory(id); err != nil {
		writeError(w, 500, "delete failed: "+err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "deleted"})
}

func (s *Server) handleClearHistory(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OlderThanDays int `json:"older_than_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.OlderThanDays = 7 // default: clear older than 7 days
	}
	if req.OlderThanDays < 1 {
		req.OlderThanDays = 7
	}
	count, err := s.db.ClearHistory(req.OlderThanDays)
	if err != nil {
		writeError(w, 500, "clear failed: "+err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"status": "cleared", "deleted": count})
}

// --- Auth ---

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if s.sso == nil {
		writeError(w, 500, "SSO not configured")
		return
	}
	state := auth.GenerateState()
	s.mu.Lock()
	s.ssoState = state
	s.mu.Unlock()
	http.Redirect(w, r, s.sso.BuildAuthURL(state), http.StatusTemporaryRedirect)
}

func (s *Server) handleAuthCallback(w http.ResponseWriter, r *http.Request) {
	if s.sso == nil {
		writeError(w, 500, "SSO not configured")
		return
	}

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	s.mu.RLock()
	expectedState := s.ssoState
	s.mu.RUnlock()

	if state == "" || state != expectedState {
		writeError(w, 400, "invalid state parameter")
		return
	}

	// Exchange code for tokens
	tok, err := s.sso.ExchangeCode(code)
	if err != nil {
		log.Printf("[AUTH] Exchange error: %v", err)
		writeError(w, 500, "token exchange failed: "+err.Error())
		return
	}

	// Verify token to get character info
	info, err := auth.VerifyToken(tok.AccessToken)
	if err != nil {
		log.Printf("[AUTH] Verify error: %v", err)
		writeError(w, 500, "token verify failed: "+err.Error())
		return
	}

	// Save session
	sess := &auth.Session{
		CharacterID:   info.CharacterID,
		CharacterName: info.CharacterName,
		AccessToken:   tok.AccessToken,
		RefreshToken:  tok.RefreshToken,
		ExpiresAt:     time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second),
	}
	if err := s.sessions.Save(sess); err != nil {
		log.Printf("[AUTH] Save session error: %v", err)
		writeError(w, 500, "save session failed")
		return
	}

	log.Printf("[AUTH] Logged in as %s (ID: %d)", info.CharacterName, info.CharacterID)

	// Redirect back to frontend
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	sess := s.sessions.Get()
	if sess == nil {
		writeJSON(w, map[string]interface{}{"logged_in": false})
		return
	}
	writeJSON(w, map[string]interface{}{
		"logged_in":      true,
		"character_id":   sess.CharacterID,
		"character_name": sess.CharacterName,
	})
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	s.sessions.Delete()
	log.Println("[AUTH] Logged out")
	writeJSON(w, map[string]interface{}{"logged_in": false})
}

func (s *Server) handleAuthCharacter(w http.ResponseWriter, r *http.Request) {
	token, err := s.sessions.EnsureValidToken(s.sso)
	if err != nil {
		writeError(w, 401, err.Error())
		return
	}
	sess := s.sessions.Get()
	if sess == nil {
		writeError(w, 401, "not logged in")
		return
	}

	type charInfo struct {
		CharacterID   int64                        `json:"character_id"`
		CharacterName string                       `json:"character_name"`
		Wallet        float64                      `json:"wallet"`
		Orders        []esi.CharacterOrder         `json:"orders"`
		OrderHistory  []esi.HistoricalOrder        `json:"order_history"`
		Transactions  []esi.WalletTransaction      `json:"transactions"`
		Skills        *esi.SkillSheet              `json:"skills"`
		Risk          *engine.PortfolioRiskSummary `json:"risk,omitempty"`
	}

	result := charInfo{
		CharacterID:   sess.CharacterID,
		CharacterName: sess.CharacterName,
	}

	// Fetch wallet
	if balance, err := esi.GetWalletBalance(sess.CharacterID, token); err == nil {
		result.Wallet = balance
	} else {
		log.Printf("[AUTH] Wallet error: %v", err)
	}

	// Fetch active orders
	if orders, err := esi.GetCharacterOrders(sess.CharacterID, token); err == nil {
		result.Orders = orders
	} else {
		log.Printf("[AUTH] Orders error: %v", err)
	}

	// Fetch order history
	if history, err := esi.GetOrderHistory(sess.CharacterID, token); err == nil {
		result.OrderHistory = history
	} else {
		log.Printf("[AUTH] Order history error: %v", err)
	}

	// Fetch wallet transactions
	if txns, err := esi.GetWalletTransactions(sess.CharacterID, token); err == nil {
		result.Transactions = txns
	} else {
		log.Printf("[AUTH] Transactions error: %v", err)
	}

	// Fetch skills
	if skills, err := esi.GetSkills(sess.CharacterID, token); err == nil {
		result.Skills = skills
	} else {
		log.Printf("[AUTH] Skills error: %v", err)
	}

	// Enrich orders with type/location names
	s.mu.RLock()
	sdeData := s.sdeData
	s.mu.RUnlock()

	if sdeData != nil {
		// Collect all location IDs for prefetch
		locationIDs := make(map[int64]bool)
		for _, o := range result.Orders {
			locationIDs[o.LocationID] = true
		}
		for _, o := range result.OrderHistory {
			locationIDs[o.LocationID] = true
		}
		for _, t := range result.Transactions {
			locationIDs[t.LocationID] = true
		}
		s.esi.PrefetchStationNames(locationIDs)

		// Enrich active orders
		for i := range result.Orders {
			if t, ok := sdeData.Types[result.Orders[i].TypeID]; ok {
				result.Orders[i].TypeName = t.Name
			}
			result.Orders[i].LocationName = s.esi.StationName(result.Orders[i].LocationID)
		}

		// Enrich order history
		for i := range result.OrderHistory {
			if t, ok := sdeData.Types[result.OrderHistory[i].TypeID]; ok {
				result.OrderHistory[i].TypeName = t.Name
			}
			result.OrderHistory[i].LocationName = s.esi.StationName(result.OrderHistory[i].LocationID)
		}

		// Enrich transactions
		for i := range result.Transactions {
			if t, ok := sdeData.Types[result.Transactions[i].TypeID]; ok {
				result.Transactions[i].TypeName = t.Name
			}
			result.Transactions[i].LocationName = s.esi.StationName(result.Transactions[i].LocationID)
		}
	}

	// Compute portfolio risk summary from recent wallet transactions.
	if len(result.Transactions) > 0 {
		if risk := engine.ComputePortfolioRiskFromTransactions(result.Transactions); risk != nil {
			result.Risk = risk
		}
	}

	writeJSON(w, result)
}

func (s *Server) handleAuthLocation(w http.ResponseWriter, r *http.Request) {
	token, err := s.sessions.EnsureValidToken(s.sso)
	if err != nil {
		writeError(w, 401, err.Error())
		return
	}
	sess := s.sessions.Get()
	if sess == nil {
		writeError(w, 401, "not logged in")
		return
	}

	loc, err := esi.GetCharacterLocation(sess.CharacterID, token)
	if err != nil {
		writeError(w, 500, "failed to get location: "+err.Error())
		return
	}

	// Resolve system name from SDE
	s.mu.RLock()
	sdeData := s.sdeData
	s.mu.RUnlock()

	result := struct {
		SolarSystemID   int32  `json:"solar_system_id"`
		SolarSystemName string `json:"solar_system_name"`
		StationID       int64  `json:"station_id,omitempty"`
		StationName     string `json:"station_name,omitempty"`
	}{
		SolarSystemID: loc.SolarSystemID,
	}

	if sdeData != nil {
		if sys, ok := sdeData.Systems[loc.SolarSystemID]; ok {
			result.SolarSystemName = sys.Name
		}
	}

	// Get station name if docked
	if loc.StationID != 0 {
		result.StationID = loc.StationID
		result.StationName = s.esi.StationName(loc.StationID)
	} else if loc.StructureID != 0 {
		result.StationID = loc.StructureID
		result.StationName = s.esi.StationName(loc.StructureID)
	}

	writeJSON(w, result)
}

// --- Industry Handlers ---

func (s *Server) handleIndustryAnalyze(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TypeID             int32   `json:"type_id"`
		Runs               int32   `json:"runs"`
		MaterialEfficiency int32   `json:"me"`
		TimeEfficiency     int32   `json:"te"`
		SystemName         string  `json:"system_name"`
		FacilityTax        float64 `json:"facility_tax"`
		StructureBonus     float64 `json:"structure_bonus"`
		BrokerFee          float64 `json:"broker_fee"`
		SalesTaxPercent    float64 `json:"sales_tax_percent"`
		MaxDepth           int     `json:"max_depth"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json")
		return
	}

	if !s.isReady() {
		writeError(w, 503, "SDE not loaded yet")
		return
	}

	if req.TypeID == 0 {
		writeError(w, 400, "type_id is required")
		return
	}

	// Resolve system ID
	var systemID int32
	if req.SystemName != "" {
		s.mu.RLock()
		systemID = s.sdeData.SystemByName[strings.ToLower(req.SystemName)]
		s.mu.RUnlock()
	}

	params := engine.IndustryParams{
		TypeID:             req.TypeID,
		Runs:               req.Runs,
		MaterialEfficiency: req.MaterialEfficiency,
		TimeEfficiency:     req.TimeEfficiency,
		SystemID:           systemID,
		FacilityTax:        req.FacilityTax,
		StructureBonus:     req.StructureBonus,
		BrokerFee:          req.BrokerFee,
		SalesTaxPercent:    req.SalesTaxPercent,
		MaxDepth:           req.MaxDepth,
	}

	// Use NDJSON streaming for progress
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "streaming not supported")
		return
	}

	s.mu.RLock()
	analyzer := s.industryAnalyzer
	s.mu.RUnlock()

	log.Printf("[API] IndustryAnalyze: typeID=%d, runs=%d, ME=%d, TE=%d, system=%s",
		req.TypeID, req.Runs, req.MaterialEfficiency, req.TimeEfficiency, req.SystemName)

	startTime := time.Now()

	result, err := analyzer.Analyze(params, func(msg string) {
		line, _ := json.Marshal(map[string]string{"type": "progress", "message": msg})
		fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()
	})

	if err != nil {
		log.Printf("[API] IndustryAnalyze error: %v", err)
		line, _ := json.Marshal(map[string]string{"type": "error", "message": err.Error()})
		fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()
		return
	}

	durationMs := time.Since(startTime).Milliseconds()
	log.Printf("[API] IndustryAnalyze complete in %dms", durationMs)

	line, _ := json.Marshal(map[string]interface{}{"type": "result", "data": result})
	fmt.Fprintf(w, "%s\n", line)
	flusher.Flush()
}

func (s *Server) handleIndustrySearch(w http.ResponseWriter, r *http.Request) {
	if !s.isReady() {
		writeError(w, 503, "SDE not loaded yet")
		return
	}

	query := r.URL.Query().Get("q")
	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	s.mu.RLock()
	analyzer := s.industryAnalyzer
	s.mu.RUnlock()

	if analyzer == nil {
		log.Printf("[API] IndustrySearch: analyzer is nil!")
		writeJSON(w, []struct{}{})
		return
	}

	results := analyzer.SearchBuildableItems(query, limit)
	log.Printf("[API] IndustrySearch: query=%q, found %d results", query, len(results))
	writeJSON(w, results)
}

func (s *Server) handleIndustrySystems(w http.ResponseWriter, r *http.Request) {
	if !s.isReady() {
		writeError(w, 503, "SDE not loaded yet")
		return
	}

	// Return list of systems with cost indices
	systems, err := s.esi.FetchIndustrySystems()
	if err != nil {
		writeError(w, 500, "failed to fetch industry systems: "+err.Error())
		return
	}

	// Enrich with system names
	s.mu.RLock()
	sdeData := s.sdeData
	s.mu.RUnlock()

	type SystemWithName struct {
		SolarSystemID   int32   `json:"solar_system_id"`
		SolarSystemName string  `json:"solar_system_name"`
		Manufacturing   float64 `json:"manufacturing"`
		Reaction        float64 `json:"reaction"`
		Copying         float64 `json:"copying"`
		Invention       float64 `json:"invention"`
	}

	result := make([]SystemWithName, 0, len(systems))
	for _, sys := range systems {
		name := ""
		if s, ok := sdeData.Systems[sys.SolarSystemID]; ok {
			name = s.Name
		}

		swn := SystemWithName{
			SolarSystemID:   sys.SolarSystemID,
			SolarSystemName: name,
		}

		for _, ci := range sys.CostIndices {
			switch ci.Activity {
			case "manufacturing":
				swn.Manufacturing = ci.CostIndex
			case "reaction":
				swn.Reaction = ci.CostIndex
			case "copying":
				swn.Copying = ci.CostIndex
			case "invention":
				swn.Invention = ci.CostIndex
			}
		}

		result = append(result, swn)
	}

	writeJSON(w, result)
}

func (s *Server) handleIndustryStatus(w http.ResponseWriter, r *http.Request) {
	if !s.isReady() {
		writeError(w, 503, "SDE not loaded yet")
		return
	}

	s.mu.RLock()
	sdeData := s.sdeData
	s.mu.RUnlock()

	blueprintCount := 0
	productCount := 0
	if sdeData.Industry != nil {
		blueprintCount = len(sdeData.Industry.Blueprints)
		productCount = len(sdeData.Industry.ProductToBlueprint)
	}

	writeJSON(w, map[string]interface{}{
		"blueprints_loaded":   blueprintCount,
		"products_with_bp":    productCount,
		"total_types":         len(sdeData.Types),
		"industry_data_ready": sdeData.Industry != nil,
	})
}

// --- Demand / War Tracker Handlers ---

// handleDemandRegions returns cached demand data for all regions.
func (s *Server) handleDemandRegions(w http.ResponseWriter, r *http.Request) {
	if !s.isReady() {
		writeError(w, 503, "SDE not loaded yet")
		return
	}

	// Try to get from cache first
	regions, err := s.db.GetDemandRegions()
	if err != nil {
		writeError(w, 500, "failed to get demand regions: "+err.Error())
		return
	}

	// If cache is empty or stale, return what we have but suggest refresh
	cacheAge := 0
	if len(regions) > 0 {
		cacheAge = int(time.Since(regions[0].UpdatedAt).Minutes())
	}

	writeJSON(w, map[string]interface{}{
		"regions":    regions,
		"count":      len(regions),
		"cache_age_minutes": cacheAge,
		"stale":      len(regions) == 0 || !s.db.IsDemandCacheFresh(60*time.Minute),
	})
}

// handleDemandHotZones returns regions with elevated kill activity.
func (s *Server) handleDemandHotZones(w http.ResponseWriter, r *http.Request) {
	if !s.isReady() {
		writeError(w, 503, "SDE not loaded yet")
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	// Check if cache is fresh (less than 1 hour old)
	if s.db.IsDemandCacheFresh(60 * time.Minute) {
		// Return from cache
		zones, err := s.db.GetHotZones(limit)
		if err != nil {
			writeError(w, 500, "failed to get hot zones: "+err.Error())
			return
		}
		writeJSON(w, map[string]interface{}{
			"hot_zones": zones,
			"count":     len(zones),
			"from_cache": true,
		})
		return
	}

	// Cache is stale - fetch fresh data
	s.mu.RLock()
	analyzer := s.demandAnalyzer
	s.mu.RUnlock()

	if analyzer == nil {
		writeError(w, 503, "demand analyzer not ready")
		return
	}

	zones, err := analyzer.GetHotZones(limit)
	if err != nil {
		writeError(w, 500, "failed to analyze hot zones: "+err.Error())
		return
	}

	// Cache the results
	for _, z := range zones {
		s.db.SaveDemandRegion(&db.DemandRegion{
			RegionID:      z.RegionID,
			RegionName:    z.RegionName,
			HotScore:      z.HotScore,
			Status:        z.Status,
			KillsToday:    z.KillsToday,
			KillsBaseline: z.KillsBaseline,
			ISKDestroyed:  z.ISKDestroyed,
			ActivePlayers: z.ActivePlayers,
			TopShips:      z.TopShips,
		})
	}

	writeJSON(w, map[string]interface{}{
		"hot_zones": zones,
		"count":     len(zones),
		"from_cache": false,
	})
}

// handleDemandRegion returns detailed demand data for a single region.
func (s *Server) handleDemandRegion(w http.ResponseWriter, r *http.Request) {
	if !s.isReady() {
		writeError(w, 503, "SDE not loaded yet")
		return
	}

	regionIDStr := r.PathValue("regionID")
	regionID, err := strconv.ParseInt(regionIDStr, 10, 32)
	if err != nil {
		writeError(w, 400, "invalid region ID")
		return
	}

	// Try cache first
	cached, err := s.db.GetDemandRegion(int32(regionID))
	if err != nil {
		writeError(w, 500, "failed to get region: "+err.Error())
		return
	}

	if cached != nil && time.Since(cached.UpdatedAt) < 60*time.Minute {
		writeJSON(w, map[string]interface{}{
			"region":     cached,
			"from_cache": true,
		})
		return
	}

	// Fetch fresh data
	s.mu.RLock()
	analyzer := s.demandAnalyzer
	s.mu.RUnlock()

	if analyzer == nil {
		writeError(w, 503, "demand analyzer not ready")
		return
	}

	zone, err := analyzer.GetSingleRegionStats(int32(regionID))
	if err != nil {
		writeError(w, 500, "failed to get region stats: "+err.Error())
		return
	}

	if zone == nil {
		writeError(w, 404, "region not found")
		return
	}

	// Cache the result
	s.db.SaveDemandRegion(&db.DemandRegion{
		RegionID:      zone.RegionID,
		RegionName:    zone.RegionName,
		HotScore:      zone.HotScore,
		Status:        zone.Status,
		KillsToday:    zone.KillsToday,
		KillsBaseline: zone.KillsBaseline,
		ISKDestroyed:  zone.ISKDestroyed,
		ActivePlayers: zone.ActivePlayers,
		TopShips:      zone.TopShips,
	})

	writeJSON(w, map[string]interface{}{
		"region":     zone,
		"from_cache": false,
	})
}

// handleDemandOpportunities returns trade opportunities for a specific region.
func (s *Server) handleDemandOpportunities(w http.ResponseWriter, r *http.Request) {
	if !s.isReady() {
		writeError(w, 503, "SDE not loaded yet")
		return
	}

	regionIDStr := r.PathValue("regionID")
	regionIDInt, err := strconv.Atoi(regionIDStr)
	if err != nil {
		writeError(w, 400, "invalid region ID")
		return
	}
	regionID := int32(regionIDInt)

	s.mu.RLock()
	analyzer := s.demandAnalyzer
	esiClient := s.esi
	sdeData := s.sdeData
	s.mu.RUnlock()

	if analyzer == nil {
		writeError(w, 503, "demand analyzer not ready")
		return
	}

	// Get opportunities
	opportunities, err := analyzer.GetRegionOpportunities(regionID, esiClient)
	if err != nil {
		writeError(w, 500, fmt.Sprintf("failed to get opportunities: %v", err))
		return
	}

	if opportunities == nil {
		writeError(w, 404, "region not found or no data")
		return
	}

	// Resolve type names from SDE
	if sdeData != nil {
		for i := range opportunities.Ships {
			if opportunities.Ships[i].TypeName == "" {
				if t, ok := sdeData.Types[opportunities.Ships[i].TypeID]; ok {
					opportunities.Ships[i].TypeName = t.Name
				}
			}
		}
		
		// Calculate security class and jumps from Jita
		const jitaSystemID = int32(30000142)
		
		// Find systems in this region and calculate security distribution
		var highCount, lowCount, nullCount int
		var closestDistance = 999999
		var mainSystemName string
		
		for _, sys := range sdeData.Systems {
			if sys.RegionID == regionID {
				// Count security types
				if sys.Security >= 0.45 {
					highCount++
				} else if sys.Security > 0.0 {
					lowCount++
				} else {
					nullCount++
				}
				
				// Find closest system to Jita (using graph if available)
				if sdeData.Universe != nil {
					dist := sdeData.Universe.ShortestPath(jitaSystemID, sys.ID)
					if dist >= 0 && dist < closestDistance {
						closestDistance = dist
						mainSystemName = sys.Name
					}
				}
			}
		}
		
		// Determine dominant security class
		total := highCount + lowCount + nullCount
		if total > 0 {
			// Build security blocks array
			var blocks []string
			if highCount > 0 {
				blocks = append(blocks, "high")
			}
			if lowCount > 0 {
				blocks = append(blocks, "low")
			}
			if nullCount > 0 {
				blocks = append(blocks, "null")
			}
			opportunities.SecurityBlocks = blocks
			
			// Dominant class
			if nullCount > highCount && nullCount > lowCount {
				opportunities.SecurityClass = "nullsec"
			} else if lowCount > highCount {
				opportunities.SecurityClass = "lowsec"
			} else if highCount > 0 {
				opportunities.SecurityClass = "highsec"
			} else {
				opportunities.SecurityClass = "nullsec"
			}
		}
		
		// Set jumps from Jita
		if closestDistance < 999999 {
			opportunities.JumpsFromJita = closestDistance
			opportunities.MainSystem = mainSystemName
		}
	}

	writeJSON(w, opportunities)
}

// handleDemandRefresh forces a refresh of demand data for all regions.
// The refresh now runs in the background so that the HTTP request returns
// quickly and does not block other scans or the UI. Results are written
// to the demand cache as they arrive, and subsequent /hotzones requests
// will start using fresh data once available.
func (s *Server) handleDemandRefresh(w http.ResponseWriter, r *http.Request) {
	if !s.isReady() {
		writeError(w, 503, "SDE not loaded yet")
		return
	}

	s.mu.RLock()
	analyzer := s.demandAnalyzer
	s.mu.RUnlock()

	if analyzer == nil {
		writeError(w, 503, "demand analyzer not ready")
		return
	}

	// Kick off background refresh
	go func() {
		// Clear in-memory cache first to force fresh API calls
		analyzer.ClearCache()
		log.Printf("[Demand] Cache cleared, starting background refresh...")

		zones, err := analyzer.GetHotZones(0) // 0 = no limit
		if err != nil {
			log.Printf("[Demand] Background refresh failed: %v", err)
			return
		}

		// Cache all results to database
		for _, z := range zones {
			if err := s.db.SaveDemandRegion(&db.DemandRegion{
				RegionID:      z.RegionID,
				RegionName:    z.RegionName,
				HotScore:      z.HotScore,
				Status:        z.Status,
				KillsToday:    z.KillsToday,
				KillsBaseline: z.KillsBaseline,
				ISKDestroyed:  z.ISKDestroyed,
				ActivePlayers: z.ActivePlayers,
				TopShips:      z.TopShips,
			}); err != nil {
				log.Printf("[Demand] Failed to save region %d: %v", z.RegionID, err)
			}
		}
		log.Printf("[Demand] Background refresh completed: %d regions", len(zones))
	}()

	writeJSON(w, map[string]interface{}{
		"status":  "started",
		"message": "Demand refresh started in background",
	})
}
