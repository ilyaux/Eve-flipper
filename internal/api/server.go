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
	"eve-flipper/internal/corp"
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

	// SSO state: map of CSRF state tokens → (expiry, desktop flag).
	// Supports concurrent login flows from multiple tabs.
	ssoStatesMu sync.Mutex
	ssoStates   map[string]ssoStateEntry

	// Wallet transaction cache for P&L tab (TTL 2 min).
	txnCacheMu   sync.RWMutex
	txnCache     []esi.WalletTransaction
	txnCacheTime time.Time

	// PLEX dashboard cache (TTL 5 min) to avoid hammering ESI with 5 concurrent requests per click.
	plexCacheMu   sync.RWMutex
	plexCache     *engine.PLEXDashboard
	plexCacheTime time.Time
	plexCacheKey  string // "salesTax_brokerFee_nesE_nesM_nesO"

	// Corporation demo provider (initialized on SDE load).
	demoCorpProvider *corp.DemoCorpProvider
}

// ssoStateEntry holds metadata for a pending SSO login flow.
type ssoStateEntry struct {
	ExpiresAt time.Time
	Desktop   bool
}

// NewServer creates a Server with the given config, ESI client, and database.
func NewServer(cfg *config.Config, esiClient *esi.Client, database *db.DB, ssoConfig *auth.SSOConfig, sessions *auth.SessionStore) *Server {
	return &Server{
		cfg:       cfg,
		esi:       esiClient,
		db:        database,
		sso:       ssoConfig,
		sessions:  sessions,
		ssoStates: make(map[string]ssoStateEntry),
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

	// Initialize corporation demo provider
	s.demoCorpProvider = corp.NewDemoCorpProvider()

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
	mux.HandleFunc("GET /api/auth/undercuts", s.handleAuthUndercuts)
	mux.HandleFunc("GET /api/auth/portfolio", s.handleAuthPortfolio)
	mux.HandleFunc("GET /api/auth/portfolio/optimize", s.handleAuthPortfolioOptimize)
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
	mux.HandleFunc("GET /api/demand/fittings/{regionID}", s.handleDemandFittings)
	mux.HandleFunc("POST /api/demand/refresh", s.handleDemandRefresh)
	// PLEX+
	mux.HandleFunc("GET /api/plex/dashboard", s.handlePLEXDashboard)
	// Corporation
	mux.HandleFunc("GET /api/auth/roles", s.handleAuthRoles)
	mux.HandleFunc("GET /api/corp/dashboard", s.handleCorpDashboard)
	mux.HandleFunc("GET /api/corp/members", s.handleCorpMembers)
	mux.HandleFunc("GET /api/corp/wallets", s.handleCorpWallets)
	mux.HandleFunc("GET /api/corp/journal", s.handleCorpJournal)
	mux.HandleFunc("GET /api/corp/orders", s.handleCorpOrders)
	mux.HandleFunc("GET /api/corp/industry", s.handleCorpIndustry)
	mux.HandleFunc("GET /api/corp/mining", s.handleCorpMining)
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

	// Validate bounds
	if s.cfg.CargoCapacity < 0 {
		s.cfg.CargoCapacity = 0
	}
	if s.cfg.BuyRadius < 0 {
		s.cfg.BuyRadius = 0
	} else if s.cfg.BuyRadius > 50 {
		s.cfg.BuyRadius = 50
	}
	if s.cfg.SellRadius < 0 {
		s.cfg.SellRadius = 0
	} else if s.cfg.SellRadius > 50 {
		s.cfg.SellRadius = 50
	}
	if s.cfg.MinMargin < 0 {
		s.cfg.MinMargin = 0
	} else if s.cfg.MinMargin > 100 {
		s.cfg.MinMargin = 100
	}
	if s.cfg.SalesTaxPercent < 0 {
		s.cfg.SalesTaxPercent = 0
	} else if s.cfg.SalesTaxPercent > 100 {
		s.cfg.SalesTaxPercent = 100
	}
	if s.cfg.Opacity < 0 {
		s.cfg.Opacity = 0
	} else if s.cfg.Opacity > 100 {
		s.cfg.Opacity = 100
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
	systems := s.sdeData.Systems
	s.mu.RUnlock()

	seen := map[string]bool{}
	var prefix, contains, bySystem []string
	for _, region := range regions {
		lower := strings.ToLower(region.Name)
		if strings.HasPrefix(lower, q) {
			prefix = append(prefix, region.Name)
			seen[region.Name] = true
		} else if strings.Contains(lower, q) {
			contains = append(contains, region.Name)
			seen[region.Name] = true
		}
	}

	// Also match by system name → suggest the region that system belongs to
	for _, sys := range systems {
		if strings.HasPrefix(strings.ToLower(sys.Name), q) {
			if r, ok := regions[sys.RegionID]; ok && !seen[r.Name] {
				bySystem = append(bySystem, r.Name+" ("+sys.Name+")")
				seen[r.Name] = true
			}
		}
	}

	result := append(prefix, contains...)
	result = append(result, bySystem...)
	if len(result) > 15 {
		result = result[:15]
	}

	writeJSON(w, map[string][]string{"regions": result})
}

type scanRequest struct {
	SystemName       string  `json:"system_name"`
	CargoCapacity    float64 `json:"cargo_capacity"`
	BuyRadius        int     `json:"buy_radius"`
	SellRadius       int     `json:"sell_radius"`
	MinMargin        float64 `json:"min_margin"`
	SalesTaxPercent  float64 `json:"sales_tax_percent"`
	BrokerFeePercent float64 `json:"broker_fee_percent"` // 0 = instant trades (no broker fee); >0 = applied to both sides
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
		rid, regionOK := s.sdeData.RegionByName[strings.ToLower(req.TargetRegion)]
		if regionOK {
			targetRegionID = rid
		} else {
			s.mu.RUnlock()
			return engine.ScanParams{}, fmt.Errorf("region not found: %s", req.TargetRegion)
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
		BrokerFeePercent:  req.BrokerFeePercent,
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
		BrokerFeePercent float64 `json:"broker_fee_percent"`
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
	if req.MaxHops > 25 {
		req.MaxHops = 25
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
		BrokerFeePercent: req.BrokerFeePercent,
		MinHops:          req.MinHops,
		MaxHops:          req.MaxHops,
		MaxResults:       req.MaxResults,
		MinRouteSecurity: req.MinRouteSecurity,
	}

	log.Printf("[API] RouteFind: system=%s, cargo=%.0f, margin=%.1f, hops=%d-%d",
		req.SystemName, req.CargoCapacity, req.MinMargin, req.MinHops, req.MaxHops)

	startTime := time.Now()
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

	durationMs := time.Since(startTime).Milliseconds()
	log.Printf("[API] RouteFind complete: %d routes in %dms", len(results), durationMs)

	var topProfit, totalProfit float64
	for _, r := range results {
		if r.TotalProfit > topProfit {
			topProfit = r.TotalProfit
		}
		totalProfit += r.TotalProfit
	}

	scanID := s.db.InsertHistoryFull("route", req.SystemName, len(results), topProfit, totalProfit, durationMs, req)
	go s.db.InsertRouteResults(scanID, results)

	line, marshalErr := json.Marshal(map[string]interface{}{"type": "result", "data": results, "count": len(results), "scan_id": scanID})
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

	// Validate type_id against SDE
	s.mu.RLock()
	sdeData := s.sdeData
	s.mu.RUnlock()
	if sdeData != nil {
		if _, ok := sdeData.Types[item.TypeID]; !ok {
			writeError(w, 400, fmt.Sprintf("unknown type_id %d", item.TypeID))
			return
		}
		// Use canonical SDE name if client didn't provide one
		if item.TypeName == "" {
			item.TypeName = sdeData.Types[item.TypeID].Name
		}
	}

	item.AddedAt = time.Now().Format(time.RFC3339)
	inserted := s.db.AddWatchlistItem(item)

	type addResponse struct {
		Items    []config.WatchlistItem `json:"items"`
		Inserted bool                   `json:"inserted"`
	}
	writeJSON(w, addResponse{
		Items:    s.db.GetWatchlist(),
		Inserted: inserted,
	})
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
		StationID       int64   `json:"station_id"`  // 0 = all stations
		RegionID        int32   `json:"region_id"`   // required
		SystemName      string  `json:"system_name"` // for radius-based scan
		Radius          int     `json:"radius"`      // 0 = single system
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
		go s.db.InsertStationResults(scanID, allResults)
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

	// When market history is available, add impact calibration (Amihud, σ, TWAP slices)
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
				// Use best price from execution plan as reference for ISK conversion
				refPrice := result.BestPrice
				est := engine.EstimateImpact(params, float64(req.Quantity), refPrice)
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
	case "route":
		results = s.db.GetRouteResults(id)
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
	desktop := r.URL.Query().Get("desktop") == "1"

	s.ssoStatesMu.Lock()
	// Purge expired states
	now := time.Now()
	for k, v := range s.ssoStates {
		if now.After(v.ExpiresAt) {
			delete(s.ssoStates, k)
		}
	}
	s.ssoStates[state] = ssoStateEntry{
		ExpiresAt: now.Add(10 * time.Minute),
		Desktop:   desktop,
	}
	s.ssoStatesMu.Unlock()

	http.Redirect(w, r, s.sso.BuildAuthURL(state), http.StatusTemporaryRedirect)
}

func (s *Server) handleAuthCallback(w http.ResponseWriter, r *http.Request) {
	if s.sso == nil {
		writeError(w, 500, "SSO not configured")
		return
	}

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	s.ssoStatesMu.Lock()
	entry, ok := s.ssoStates[state]
	if ok {
		delete(s.ssoStates, state) // consume: one-time use
	}
	s.ssoStatesMu.Unlock()

	if state == "" || !ok || time.Now().After(entry.ExpiresAt) {
		writeError(w, 400, "invalid or expired state parameter")
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

	// Check whether the login was initiated from the desktop (Tauri) app.
	if !entry.Desktop {
		// Web browser: redirect back to the frontend (original behaviour).
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	// Desktop / Tauri: show a styled success page in the system browser.
	// The Tauri app detects login via polling /api/auth/status.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><title>EVE Flipper - Login</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{background:#0d1117;color:#c9d1d9;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Helvetica,Arial,sans-serif;
display:flex;align-items:center;justify-content:center;min-height:100vh}
.card{text-align:center;padding:3rem 4rem;border:1px solid #30363d;border-radius:12px;background:#161b22}
.avatar{width:64px;height:64px;border-radius:8px;margin-bottom:1rem}
h1{font-size:1.5rem;color:#58a6ff;margin-bottom:.5rem}
p{color:#8b949e;margin-bottom:.25rem}
.hint{margin-top:1.5rem;font-size:.85rem;color:#484f58}
</style></head>
<body><div class="card">
<img class="avatar" src="https://images.evetech.net/characters/%d/portrait?size=128" alt="">
<h1>%s</h1>
<p>Login successful!</p>
<p class="hint">You can close this tab and return to EVE Flipper.</p>
</div>
<script>setTimeout(function(){window.close()},4000)</script>
</body></html>`, info.CharacterID, info.CharacterName)
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

	// Fetch all character data in parallel for faster popup loading.
	var wgChar sync.WaitGroup
	var muChar sync.Mutex

	wgChar.Add(5)

	go func() {
		defer wgChar.Done()
		if balance, err := s.esi.GetWalletBalance(sess.CharacterID, token); err == nil {
			muChar.Lock()
			result.Wallet = balance
			muChar.Unlock()
		} else {
			log.Printf("[AUTH] Wallet error: %v", err)
		}
	}()

	go func() {
		defer wgChar.Done()
		if orders, err := s.esi.GetCharacterOrders(sess.CharacterID, token); err == nil {
			muChar.Lock()
			result.Orders = orders
			muChar.Unlock()
		} else {
			log.Printf("[AUTH] Orders error: %v", err)
		}
	}()

	go func() {
		defer wgChar.Done()
		if history, err := s.esi.GetOrderHistory(sess.CharacterID, token); err == nil {
			muChar.Lock()
			result.OrderHistory = history
			muChar.Unlock()
		} else {
			log.Printf("[AUTH] Order history error: %v", err)
		}
	}()

	go func() {
		defer wgChar.Done()
		if txns, err := s.esi.GetWalletTransactions(sess.CharacterID, token); err == nil {
			muChar.Lock()
			result.Transactions = txns
			muChar.Unlock()
		} else {
			log.Printf("[AUTH] Transactions error: %v", err)
		}
	}()

	go func() {
		defer wgChar.Done()
		if skills, err := s.esi.GetSkills(sess.CharacterID, token); err == nil {
			muChar.Lock()
			result.Skills = skills
			muChar.Unlock()
		} else {
			log.Printf("[AUTH] Skills error: %v", err)
		}
	}()

	wgChar.Wait()

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

	loc, err := s.esi.GetCharacterLocation(sess.CharacterID, token)
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

func (s *Server) handleAuthUndercuts(w http.ResponseWriter, r *http.Request) {
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

	// Fetch active orders using the shared ESI client.
	orders, err := s.esi.GetCharacterOrders(sess.CharacterID, token)
	if err != nil {
		writeError(w, 500, "failed to fetch orders: "+err.Error())
		return
	}

	if len(orders) == 0 {
		writeJSON(w, []engine.UndercutStatus{})
		return
	}

	// Collect unique (region, type) pairs.
	type regionType struct {
		regionID int32
		typeID   int32
	}
	pairs := make(map[regionType]bool)
	for _, o := range orders {
		pairs[regionType{o.RegionID, o.TypeID}] = true
	}

	// Fetch regional orders for each unique type (concurrently, with semaphore).
	// Limit concurrency to 10 to avoid ESI rate-limit issues.
	type fetchResult struct {
		orders []esi.MarketOrder
		err    error
	}
	results := make(map[regionType]fetchResult)
	var mu sync.Mutex
	var wg sync.WaitGroup
	undercutSem := make(chan struct{}, 10) // limit to 10 concurrent ESI requests

	for pair := range pairs {
		wg.Add(1)
		go func(rt regionType) {
			defer wg.Done()
			undercutSem <- struct{}{}
			ro, fetchErr := s.esi.FetchRegionOrdersByType(rt.regionID, rt.typeID)
			<-undercutSem
			mu.Lock()
			results[rt] = fetchResult{ro, fetchErr}
			mu.Unlock()
		}(pair)
	}
	wg.Wait()

	// Flatten all regional orders into one slice.
	var allRegional []esi.MarketOrder
	for _, fr := range results {
		if fr.err == nil {
			allRegional = append(allRegional, fr.orders...)
		}
	}

	undercuts := engine.AnalyzeUndercuts(orders, allRegional)
	writeJSON(w, undercuts)
}

func (s *Server) handleAuthPortfolio(w http.ResponseWriter, r *http.Request) {
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

	daysStr := r.URL.Query().Get("days")
	days := 30
	if daysStr != "" {
		if d, err := strconv.Atoi(daysStr); err == nil && d > 0 && d <= 365 {
			days = d
		}
	}

	// Use cached wallet transactions (TTL 2 min) to avoid hammering ESI
	// when the user switches between P&L periods.
	s.txnCacheMu.RLock()
	cached := s.txnCache
	cacheAge := time.Since(s.txnCacheTime)
	s.txnCacheMu.RUnlock()

	var txns []esi.WalletTransaction
	if cached != nil && cacheAge < 2*time.Minute {
		txns = cached
	} else {
		freshTxns, fetchErr := s.esi.GetWalletTransactions(sess.CharacterID, token)
		if fetchErr != nil {
			writeError(w, 500, "failed to fetch transactions: "+fetchErr.Error())
			return
		}

		// Enrich type names from SDE
		s.mu.RLock()
		sdeData := s.sdeData
		s.mu.RUnlock()

		if sdeData != nil {
			for i := range freshTxns {
				if t, ok := sdeData.Types[freshTxns[i].TypeID]; ok {
					freshTxns[i].TypeName = t.Name
				}
			}
		}

		// Store in cache
		s.txnCacheMu.Lock()
		s.txnCache = freshTxns
		s.txnCacheTime = time.Now()
		s.txnCacheMu.Unlock()

		txns = freshTxns
	}

	result := engine.ComputePortfolioPnL(txns, days)
	writeJSON(w, result)
}

func (s *Server) handleAuthPortfolioOptimize(w http.ResponseWriter, r *http.Request) {
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

	daysStr := r.URL.Query().Get("days")
	days := 90
	if daysStr != "" {
		if d, err := strconv.Atoi(daysStr); err == nil && d > 0 && d <= 365 {
			days = d
		}
	}

	// Use cached wallet transactions (TTL 2 min) to avoid hammering ESI.
	s.txnCacheMu.RLock()
	cached := s.txnCache
	cacheAge := time.Since(s.txnCacheTime)
	s.txnCacheMu.RUnlock()

	var txns []esi.WalletTransaction
	if cached != nil && cacheAge < 2*time.Minute {
		txns = cached
	} else {
		freshTxns, fetchErr := s.esi.GetWalletTransactions(sess.CharacterID, token)
		if fetchErr != nil {
			writeError(w, 500, "failed to fetch transactions: "+fetchErr.Error())
			return
		}

		// Enrich type names from SDE
		s.mu.RLock()
		sdeData := s.sdeData
		s.mu.RUnlock()

		if sdeData != nil {
			for i := range freshTxns {
				if t, ok := sdeData.Types[freshTxns[i].TypeID]; ok {
					freshTxns[i].TypeName = t.Name
				}
			}
		}

		// Store in cache
		s.txnCacheMu.Lock()
		s.txnCache = freshTxns
		s.txnCacheTime = time.Now()
		s.txnCacheMu.Unlock()

		txns = freshTxns
	}

	result, diag := engine.ComputePortfolioOptimization(txns, days)
	if result == nil {
		// Return diagnostic info as JSON so the frontend can show details.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]any{
			"error":      "not enough trading data for optimization",
			"diagnostic": diag,
		})
		return
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
		"regions":           regions,
		"count":             len(regions),
		"cache_age_minutes": cacheAge,
		"stale":             len(regions) == 0 || !s.db.IsDemandCacheFresh(60*time.Minute),
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
			"hot_zones":  zones,
			"count":      len(zones),
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
		"hot_zones":  zones,
		"count":      len(zones),
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

	// Try to load fitting profile from cache (TTL 2 hours)
	var fittingProfile *zkillboard.RegionDemandProfile
	if s.db.IsFittingProfileFresh(regionID, 2*time.Hour) {
		items, err := s.db.GetFittingDemandProfile(regionID)
		if err == nil && len(items) > 0 {
			fittingProfile = &zkillboard.RegionDemandProfile{
				RegionID: regionID,
				Items:    make(map[int32]*zkillboard.ItemDemandProfile),
			}
			for _, item := range items {
				fittingProfile.Items[item.TypeID] = &zkillboard.ItemDemandProfile{
					TypeID:         item.TypeID,
					TypeName:       item.TypeName,
					Category:       item.Category,
					TotalDestroyed: item.TotalDestroyed,
					KillmailCount:  item.KillmailCount,
					AvgPerKillmail: item.AvgPerKillmail,
					EstDailyDemand: item.EstDailyDemand,
				}
			}
		}
	}

	// Get opportunities (with fitting profile if available)
	opportunities, err := analyzer.GetRegionOpportunities(regionID, esiClient, fittingProfile)
	if err != nil {
		writeError(w, 500, fmt.Sprintf("failed to get opportunities: %v", err))
		return
	}

	if opportunities == nil {
		writeError(w, 404, "region not found or no data")
		return
	}

	// Resolve type names + volumes from SDE
	if sdeData != nil {
		resolveTypeInfo := func(opps []zkillboard.TradeOpportunity) {
			for i := range opps {
				if t, ok := sdeData.Types[opps[i].TypeID]; ok {
					if opps[i].TypeName == "" {
						opps[i].TypeName = t.Name
					}
					// FIX #6: Populate Volume (m³) from SDE
					opps[i].Volume = t.Volume
				}
			}
		}
		resolveTypeInfo(opportunities.Ships)
		resolveTypeInfo(opportunities.Modules)
		resolveTypeInfo(opportunities.Ammo)

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

// handleDemandFittings returns raw fitting demand data for a region.
func (s *Server) handleDemandFittings(w http.ResponseWriter, r *http.Request) {
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

	items, err := s.db.GetFittingDemandProfile(regionID)
	if err != nil {
		writeError(w, 500, fmt.Sprintf("failed to get fitting data: %v", err))
		return
	}

	fresh := s.db.IsFittingProfileFresh(regionID, 2*time.Hour)

	writeJSON(w, map[string]interface{}{
		"region_id":  regionID,
		"items":      items,
		"count":      len(items),
		"from_cache": fresh,
	})
}

// handleDemandRefresh forces a refresh of demand data for all regions.
// Uses NDJSON streaming so the frontend can track progress in real time.
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

	s.mu.RLock()
	esiClient := s.esi
	sdeData := s.sdeData
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "streaming not supported")
		return
	}

	sendProgress := func(msg string) {
		line, _ := json.Marshal(map[string]string{"type": "progress", "message": msg})
		fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()
	}

	sendProgress("Clearing cache...")
	analyzer.ClearCache()
	log.Printf("[Demand] Cache cleared, starting refresh...")

	sendProgress("Fetching region kill data from zKillboard...")
	zones, err := analyzer.GetHotZones(0)
	if err != nil {
		log.Printf("[Demand] Refresh failed: %v", err)
		line, _ := json.Marshal(map[string]string{"type": "error", "message": err.Error()})
		fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()
		return
	}

	sendProgress(fmt.Sprintf("Saving %d regions...", len(zones)))
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
	log.Printf("[Demand] Region refresh completed: %d regions", len(zones))

	// Analyze fittings for hot regions (elevated+)
	var hotRegions []zkillboard.RegionHotZone
	for _, z := range zones {
		if z.HotScore >= 1.15 {
			hotRegions = append(hotRegions, z)
		}
	}
	if len(hotRegions) > 0 && esiClient != nil && sdeData != nil {
		sendProgress(fmt.Sprintf("Analyzing killmail fittings for %d hot regions...", len(hotRegions)))
		for i, z := range hotRegions {
			sendProgress(fmt.Sprintf("Analyzing fittings: %s (%d/%d)...", z.RegionName, i+1, len(hotRegions)))
			profile, err := analyzer.AnalyzeRegionFittings(z.RegionID, esiClient, sdeData, 100)
			if err != nil {
				log.Printf("[Demand] Fitting analysis failed for region %d: %v", z.RegionID, err)
				continue
			}
			var dbItems []db.FittingDemandItem
			for _, item := range profile.Items {
				dbItems = append(dbItems, db.FittingDemandItem{
					RegionID:       z.RegionID,
					TypeID:         item.TypeID,
					TypeName:       item.TypeName,
					Category:       item.Category,
					TotalDestroyed: item.TotalDestroyed,
					KillmailCount:  item.KillmailCount,
					AvgPerKillmail: item.AvgPerKillmail,
					EstDailyDemand: item.EstDailyDemand,
					SampledKills:   profile.SampledKills,
					TotalKills24h:  profile.TotalKills24h,
				})
			}
			if err := s.db.SaveFittingDemandProfile(z.RegionID, dbItems); err != nil {
				log.Printf("[Demand] Failed to save fitting profile for region %d: %v", z.RegionID, err)
			}
		}
		log.Printf("[Demand] Fitting analysis completed for %d regions", len(hotRegions))
	}

	line, _ := json.Marshal(map[string]interface{}{
		"type":    "result",
		"status":  "completed",
		"regions": len(zones),
	})
	fmt.Fprintf(w, "%s\n", line)
	flusher.Flush()
}

// --- PLEX+ ---

func (s *Server) handlePLEXDashboard(w http.ResponseWriter, r *http.Request) {
	if !s.isReady() {
		writeError(w, 503, "SDE not loaded yet")
		return
	}

	q := r.URL.Query()
	salesTax := 3.6
	brokerFee := 1.0
	if v, err := strconv.ParseFloat(q.Get("sales_tax"), 64); err == nil && v >= 0 && v <= 100 {
		salesTax = v
	}
	if v, err := strconv.ParseFloat(q.Get("broker_fee"), 64); err == nil && v >= 0 && v <= 100 {
		brokerFee = v
	}

	// NES PLEX prices — user-overridable, 0 = use default
	var nes engine.NESPrices
	if v, err := strconv.Atoi(q.Get("nes_extractor")); err == nil && v > 0 {
		nes.ExtractorPLEX = v
	}
	if v, err := strconv.Atoi(q.Get("nes_mptc")); err == nil && v > 0 {
		nes.MPTCPLEX = v
	}
	if v, err := strconv.Atoi(q.Get("nes_omega")); err == nil && v > 0 {
		nes.OmegaPLEX = v
	}

	log.Printf("[API] PLEX Dashboard: salesTax=%.1f, brokerFee=%.1f, nes=%+v", salesTax, brokerFee, nes)

	// Check cache (5 min TTL, keyed by user params)
	cacheKey := fmt.Sprintf("%.2f_%.2f_%d_%d_%d", salesTax, brokerFee, nes.ExtractorPLEX, nes.MPTCPLEX, nes.OmegaPLEX)
	s.plexCacheMu.RLock()
	if s.plexCache != nil && s.plexCacheKey == cacheKey && time.Since(s.plexCacheTime) < 5*time.Minute {
		cached := s.plexCache
		s.plexCacheMu.RUnlock()
		log.Printf("[PLEX] Serving from cache (age: %v)", time.Since(s.plexCacheTime).Round(time.Second))
		writeJSON(w, cached)
		return
	}
	s.plexCacheMu.RUnlock()

	// All fetches run in parallel:
	// 1. PLEX orders from Global PLEX Market (region 19000001)
	// 2. Related items (Extractor, Injector, MPTC) from Jita
	// 3. PLEX price history from Global PLEX Market

	type ordersResult struct {
		orders []esi.MarketOrder
		err    error
	}
	plexCh := make(chan ordersResult, 1)
	go func() {
		orders, err := s.esi.FetchRegionOrdersByType(engine.GlobalPLEXRegionID, engine.PLEXTypeID)
		plexCh <- ordersResult{orders, err}
	}()

	relatedTypes := []int32{engine.SkillExtractorTypeID, engine.LargeSkillInjTypeID, engine.MPTCTypeID}
	type typeResult struct {
		typeID int32
		orders []esi.MarketOrder
	}
	relCh := make(chan typeResult, len(relatedTypes))
	for _, tid := range relatedTypes {
		go func(t int32) {
			orders, err := s.esi.FetchRegionOrdersByType(engine.JitaRegionID, t)
			if err != nil {
				log.Printf("[PLEX] Failed to fetch type %d orders: %v", t, err)
				relCh <- typeResult{t, nil}
				return
			}
			relCh <- typeResult{t, orders}
		}(tid)
	}

	type histResult struct {
		entries []esi.HistoryEntry
		err     error
	}
	histCh := make(chan histResult, 1)
	go func() {
		entries, err := s.esi.FetchMarketHistory(engine.GlobalPLEXRegionID, engine.PLEXTypeID)
		histCh <- histResult{entries, err}
	}()

	// Collect results
	plexRes := <-plexCh
	if plexRes.err != nil {
		log.Printf("[PLEX] Failed to fetch global PLEX orders: %v", plexRes.err)
	}

	relatedOrders := make(map[int32][]esi.MarketOrder)
	for range relatedTypes {
		res := <-relCh
		if res.orders != nil {
			relatedOrders[res.typeID] = res.orders
		}
	}

	histRes := <-histCh
	history := histRes.entries
	if histRes.err != nil {
		log.Printf("[PLEX] Failed to fetch history: %v", histRes.err)
	}

	log.Printf("[PLEX] Global orders: %d, history: %d, related types: %d",
		len(plexRes.orders), len(history), len(relatedOrders))

	dashboard := engine.ComputePLEXDashboard(plexRes.orders, relatedOrders, history, salesTax, brokerFee, nes)

	// Store in cache
	s.plexCacheMu.Lock()
	s.plexCache = &dashboard
	s.plexCacheTime = time.Now()
	s.plexCacheKey = cacheKey
	s.plexCacheMu.Unlock()

	writeJSON(w, dashboard)
}

// ============================================================
// Corporation Handlers
// ============================================================

// handleAuthRoles returns the character's corporation roles and director status.
func (s *Server) handleAuthRoles(w http.ResponseWriter, r *http.Request) {
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

	// Fetch roles and corp ID in parallel
	var roles *esi.CharacterRolesResponse
	var corpID int32
	var rolesErr, corpErr error
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		roles, rolesErr = s.esi.GetCharacterRoles(sess.CharacterID, token)
	}()
	go func() {
		defer wg.Done()
		corpID, corpErr = s.esi.GetCharacterCorporationID(sess.CharacterID)
	}()
	wg.Wait()

	result := corp.CharacterRoles{
		CorporationID: corpID,
	}

	if rolesErr == nil && roles != nil {
		result.Roles = roles.Roles
		for _, role := range roles.Roles {
			if role == "Director" || role == "CEO" {
				result.IsDirector = true
				break
			}
		}
	}
	if corpErr != nil {
		log.Printf("[CORP] Failed to fetch corp ID: %v", corpErr)
	}

	writeJSON(w, result)
}

// corpProvider returns the appropriate CorpDataProvider based on the ?mode= query param.
func (s *Server) corpProvider(r *http.Request) (corp.CorpDataProvider, error) {
	mode := r.URL.Query().Get("mode")
	if mode == "live" {
		token, err := s.sessions.EnsureValidToken(s.sso)
		if err != nil {
			return nil, fmt.Errorf("not logged in: %w", err)
		}
		sess := s.sessions.Get()
		if sess == nil {
			return nil, fmt.Errorf("not logged in")
		}
		corpID, err := s.esi.GetCharacterCorporationID(sess.CharacterID)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve corporation: %w", err)
		}
		s.mu.RLock()
		sdeData := s.sdeData
		s.mu.RUnlock()
		return corp.NewESICorpProvider(s.esi, sdeData, token, corpID, sess.CharacterID), nil
	}
	// Default: demo mode
	if s.demoCorpProvider == nil {
		return nil, fmt.Errorf("demo data not ready (SDE still loading)")
	}
	return s.demoCorpProvider, nil
}

func (s *Server) handleCorpDashboard(w http.ResponseWriter, r *http.Request) {
	provider, err := s.corpProvider(r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}

	// Fetch adjusted prices for ISK estimation (mining ores, industry products).
	// Non-blocking: if prices fail, dashboard still works with zero ISK estimates.
	var prices corp.PriceMap
	if provider.IsDemo() && s.demoCorpProvider != nil {
		prices = s.demoCorpProvider.DemoPrices()
	} else {
		s.mu.RLock()
		ia := s.industryAnalyzer
		s.mu.RUnlock()
		if ia != nil {
			if adjusted, err := s.esi.GetAllAdjustedPrices(ia.IndustryCache); err == nil {
				prices = make(corp.PriceMap, len(adjusted))
				for k, v := range adjusted {
					prices[k] = v
				}
			} else {
				log.Printf("[CORP] Failed to fetch adjusted prices: %v (ISK estimates will be zero)", err)
			}
		}
	}

	dashboard, err := corp.BuildDashboard(provider, prices)
	if err != nil {
		writeError(w, 500, fmt.Sprintf("dashboard build failed: %v", err))
		return
	}

	writeJSON(w, dashboard)
}

func (s *Server) handleCorpMembers(w http.ResponseWriter, r *http.Request) {
	provider, err := s.corpProvider(r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}

	members, err := provider.GetMembers()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}

	writeJSON(w, members)
}

func (s *Server) handleCorpWallets(w http.ResponseWriter, r *http.Request) {
	provider, err := s.corpProvider(r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}

	wallets, err := provider.GetWallets()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}

	writeJSON(w, wallets)
}

func (s *Server) handleCorpJournal(w http.ResponseWriter, r *http.Request) {
	provider, err := s.corpProvider(r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}

	division := 1
	if d := r.URL.Query().Get("division"); d != "" {
		if v, err := strconv.Atoi(d); err == nil && v >= 1 && v <= 7 {
			division = v
		}
	}
	days := 90
	if d := r.URL.Query().Get("days"); d != "" {
		if v, err := strconv.Atoi(d); err == nil && v > 0 {
			days = v
		}
	}

	journal, err := provider.GetJournal(division, days)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}

	writeJSON(w, journal)
}

func (s *Server) handleCorpOrders(w http.ResponseWriter, r *http.Request) {
	provider, err := s.corpProvider(r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}

	orders, err := provider.GetOrders()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}

	writeJSON(w, orders)
}

func (s *Server) handleCorpIndustry(w http.ResponseWriter, r *http.Request) {
	provider, err := s.corpProvider(r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}

	jobs, err := provider.GetIndustryJobs()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}

	writeJSON(w, jobs)
}

func (s *Server) handleCorpMining(w http.ResponseWriter, r *http.Request) {
	provider, err := s.corpProvider(r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}

	entries, err := provider.GetMiningLedger()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}

	writeJSON(w, entries)
}
