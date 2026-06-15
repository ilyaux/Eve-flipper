package api

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"eve-flipper/internal/telemetry"
)

type telemetrySink interface {
	Enabled() bool
	Track(telemetry.Event)
}

func (s *Server) SetTelemetry(client telemetrySink) {
	s.telemetry = client
}

func (s *Server) telemetryEnabled() bool {
	return s != nil && s.telemetry != nil && s.telemetry.Enabled()
}

type telemetryResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *telemetryResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *telemetryResponseWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func (w *telemetryResponseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (s *Server) telemetryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.telemetryEnabled() || !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &telemetryResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		duration := time.Since(start)
		path := strings.TrimSpace(r.Pattern)
		if path == "" {
			path = normalizedTelemetryPath(r.URL.Path)
		}
		userID := userIDFromRequest(r)
		var characterID *int64
		if s.sessions != nil {
			if sess := s.sessions.GetForUser(userID); sess != nil && sess.CharacterID > 0 {
				id := sess.CharacterID
				characterID = &id
			}
		}
		s.telemetry.Track(telemetry.Event{
			EventType:   "api_request",
			Source:      "backend",
			Module:      telemetryModuleFromPath(path),
			UserID:      userID,
			CharacterID: characterID,
			Path:        path,
			Method:      r.Method,
			Status:      rec.status,
			DurationMS:  float64(duration.Microseconds()) / 1000,
			ErrorCode:   telemetryErrorCode(rec.status),
			IP:          telemetryClientIP(r),
			UserAgent:   r.UserAgent(),
		})
	})
}

type clientTelemetryRequest struct {
	EventType   string                 `json:"event_type"`
	SessionID   string                 `json:"session_id"`
	Module      string                 `json:"module"`
	CharacterID *int64                 `json:"character_id,omitempty"`
	Properties  map[string]interface{} `json:"properties"`
}

func (s *Server) handleTelemetryClient(w http.ResponseWriter, r *http.Request) {
	if !s.telemetryEnabled() {
		writeJSON(w, map[string]bool{"ok": true})
		return
	}
	var req clientTelemetryRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.EventType = strings.TrimSpace(req.EventType)
	if !telemetry.ClientEventAllowed(req.EventType) {
		writeError(w, http.StatusBadRequest, "telemetry event type is not allowed")
		return
	}
	userID := userIDFromRequest(r)
	s.telemetry.Track(telemetry.Event{
		EventType:   req.EventType,
		Source:      "frontend",
		Module:      strings.TrimSpace(req.Module),
		UserID:      userID,
		SessionID:   strings.TrimSpace(req.SessionID),
		CharacterID: req.CharacterID,
		IP:          telemetryClientIP(r),
		UserAgent:   r.UserAgent(),
		Properties:  req.Properties,
	})
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) trackTelemetryEvent(r *http.Request, event telemetry.Event) {
	if !s.telemetryEnabled() {
		return
	}
	if event.UserID == "" && r != nil {
		event.UserID = userIDFromRequest(r)
		event.IP = firstNonEmpty(event.IP, telemetryClientIP(r))
		event.UserAgent = firstNonEmpty(event.UserAgent, r.UserAgent())
	}
	s.telemetry.Track(event)
}

func (s *Server) trackScanStarted(r *http.Request, module string, props map[string]interface{}) {
	s.trackTelemetryEvent(r, telemetry.Event{
		EventType:  "scan_started",
		Source:     "backend",
		Module:     module,
		Properties: props,
	})
}

func (s *Server) trackScanFinished(r *http.Request, module string, resultCount int, durationMs int64, props map[string]interface{}) {
	if props == nil {
		props = map[string]interface{}{}
	}
	props["result_count"] = resultCount
	props["duration_ms"] = durationMs
	s.trackTelemetryEvent(r, telemetry.Event{
		EventType:  "scan_finished",
		Source:     "backend",
		Module:     module,
		DurationMS: float64(durationMs),
		Properties: props,
	})
}

func (s *Server) trackScanFailed(r *http.Request, module string, err error, props map[string]interface{}) {
	if props == nil {
		props = map[string]interface{}{}
	}
	if err != nil {
		props["error"] = err.Error()
	}
	s.trackTelemetryEvent(r, telemetry.Event{
		EventType:  "scan_failed",
		Source:     "backend",
		Module:     module,
		ErrorCode:  "scan_failed",
		Properties: props,
	})
}

func (s *Server) trackAuthEvent(r *http.Request, eventType string, characterID *int64, errorCode string, props map[string]interface{}) {
	s.trackAuthEventForUser(r, "", eventType, characterID, errorCode, props)
}

func (s *Server) trackAuthEventForUser(r *http.Request, userID string, eventType string, characterID *int64, errorCode string, props map[string]interface{}) {
	s.trackTelemetryEvent(r, telemetry.Event{
		EventType:   eventType,
		Source:      "backend",
		Module:      "auth",
		UserID:      strings.TrimSpace(userID),
		CharacterID: characterID,
		ErrorCode:   errorCode,
		Properties:  props,
	})
}

func (s *Server) trackUserSnapshot(r *http.Request, snapshotType string, characterID *int64, payload map[string]interface{}) {
	s.trackUserSnapshotForUser(r, "", snapshotType, characterID, payload)
}

func (s *Server) trackUserSnapshotForUser(r *http.Request, userID string, snapshotType string, characterID *int64, payload map[string]interface{}) {
	s.trackTelemetryEvent(r, telemetry.Event{
		EventType:       "user_snapshot",
		Source:          "backend",
		Module:          "character",
		UserID:          strings.TrimSpace(userID),
		CharacterID:     characterID,
		Private:         true,
		SnapshotType:    snapshotType,
		SnapshotPayload: payload,
		Properties:      map[string]interface{}{"snapshot_type": snapshotType},
	})
}

func scanRequestTelemetryProps(req scanRequest) map[string]interface{} {
	return map[string]interface{}{
		"system_name":                     req.SystemName,
		"buy_radius":                      req.BuyRadius,
		"sell_radius":                     req.SellRadius,
		"cargo_capacity":                  req.CargoCapacity,
		"min_margin":                      req.MinMargin,
		"min_daily_volume":                req.MinDailyVolume,
		"max_investment":                  req.MaxInvestment,
		"min_item_profit":                 req.MinItemProfit,
		"avg_price_period":                req.AvgPricePeriod,
		"min_period_roi":                  req.MinPeriodROI,
		"max_dos":                         req.MaxDOS,
		"min_demand_per_day":              req.MinDemandPerDay,
		"purchase_demand_days":            req.PurchaseDemandDays,
		"min_route_security":              req.MinRouteSecurity,
		"source_regions":                  req.SourceRegions,
		"target_region":                   req.TargetRegion,
		"target_market_system":            req.TargetMarketSystem,
		"target_market_location_id":       req.TargetMarketLocationID,
		"category_ids":                    req.CategoryIDs,
		"sell_order_mode":                 req.SellOrderMode,
		"regional_diagnostic_mode":        req.RegionalDiagnosticMode,
		"include_structures":              req.IncludeStructures,
		"contract_instant_liquidation":    req.ContractInstantLiquidation,
		"contract_hold_days":              req.ContractHoldDays,
		"contract_target_confidence":      req.ContractTargetConfidence,
		"contract_require_history":        req.RequireHistory,
		"contract_exclude_rigs_with_ship": req.ExcludeRigsWithShip,
	}
}

func stationScanTelemetryProps(req interface{}, scope string) map[string]interface{} {
	return map[string]interface{}{
		"scan_module": "station",
		"scope":       scope,
		"filters":     req,
	}
}

func telemetryClientIP(r *http.Request) string {
	for _, header := range []string{"CF-Connecting-IP", "X-Forwarded-For", "X-Real-IP"} {
		value := strings.TrimSpace(r.Header.Get(header))
		if value == "" {
			continue
		}
		if header == "X-Forwarded-For" {
			value = strings.TrimSpace(strings.Split(value, ",")[0])
		}
		if net.ParseIP(value) != nil {
			return value
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && net.ParseIP(host) != nil {
		return host
	}
	return ""
}

func normalizedTelemetryPath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if _, err := strconv.ParseInt(part, 10, 64); err == nil && part != "" {
			parts[i] = "{id}"
		}
	}
	return strings.Join(parts, "/")
}

func telemetryModuleFromPath(path string) string {
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) >= 2 && parts[0] == "api" {
		return parts[1]
	}
	return ""
}

func telemetryErrorCode(status int) string {
	if status >= 500 {
		return "server_error"
	}
	if status >= 400 {
		return "client_error"
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
