package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"eve-flipper/internal/db"
	"eve-flipper/internal/telemetry"
)

const hostedEntitlementsURLEnv = "EVEFLIPPER_ENTITLEMENTS_URL"
const hostedEntitlementsKeyEnv = "EVEFLIPPER_ENTITLEMENTS_KEY"
const hostedPaymentsURLEnv = "EVEFLIPPER_PAYMENTS_URL"
const hostedUsageURLEnv = "EVEFLIPPER_USAGE_URL"

type hostedAccessPlan struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ExpiresAt string `json:"expires_at,omitempty"`
	RenewsAt  string `json:"renews_at,omitempty"`
}

type hostedAccessUsage struct {
	Used      int    `json:"used"`
	Limit     *int   `json:"limit"`
	Remaining *int   `json:"remaining"`
	Window    string `json:"window"`
	ResetsAt  string `json:"resets_at,omitempty"`
}

type hostedAccessPayment struct {
	ReceiverName          string `json:"receiver_name,omitempty"`
	ReceiverCharacterID   *int64 `json:"receiver_character_id,omitempty"`
	ReceiverCorporationID *int64 `json:"receiver_corporation_id,omitempty"`
	AmountISK             int64  `json:"amount_isk"`
	ReasonCode            string `json:"reason_code"`
	ExpiresAt             string `json:"expires_at,omitempty"`
	SentMarkedAt          string `json:"sent_marked_at,omitempty"`
}

type hostedAccessPaymentHistoryItem struct {
	Code             string `json:"code"`
	PlanID           string `json:"plan_id"`
	AmountISK        int64  `json:"amount_isk"`
	Status           string `json:"status"`
	CreatedAt        string `json:"created_at"`
	ExpiresAt        string `json:"expires_at"`
	MatchedAt        string `json:"matched_at,omitempty"`
	MatchedAmountISK int64  `json:"matched_amount_isk,omitempty"`
	SentMarkedAt     string `json:"sent_marked_at,omitempty"`
	Note             string `json:"note,omitempty"`
}

type hostedAccessPlanOffer struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	PriceISK             int64    `json:"price_isk"`
	PeriodDays           int      `json:"period_days"`
	ScanLimitPerDay      *int     `json:"scan_limit_per_day,omitempty"`
	StationAILimitPerDay *int     `json:"station_ai_limit_per_day,omitempty"`
	Features             []string `json:"features,omitempty"`
}

type hostedAccessResponse struct {
	Hosted         bool                             `json:"hosted"`
	Plan           hostedAccessPlan                 `json:"plan"`
	Status         string                           `json:"status"`
	Features       map[string]bool                  `json:"features"`
	Usage          map[string]hostedAccessUsage     `json:"usage"`
	Payment        *hostedAccessPayment             `json:"payment,omitempty"`
	PaymentHistory []hostedAccessPaymentHistoryItem `json:"payment_history,omitempty"`
	AvailablePlans []hostedAccessPlanOffer          `json:"available_plans,omitempty"`
	UpgradeURL     string                           `json:"upgrade_url,omitempty"`
	Message        string                           `json:"message,omitempty"`
}

type hostedPaymentRequest struct {
	PlanID      string `json:"plan_id"`
	CharacterID string `json:"character_id,omitempty"`
}

type hostedPaymentCancelRequest struct {
	CharacterID string `json:"character_id,omitempty"`
}

type hostedPaymentMarkSentRequest struct {
	CharacterID string `json:"character_id,omitempty"`
	Code        string `json:"code"`
}

type hostedUsageDecision struct {
	Allowed     bool   `json:"allowed"`
	UserID      string `json:"user_id"`
	CharacterID string `json:"character_id,omitempty"`
	Feature     string `json:"feature"`
	Used        int    `json:"used"`
	Limit       *int   `json:"limit"`
	Remaining   *int   `json:"remaining"`
	Window      string `json:"window"`
	WindowStart string `json:"window_start"`
	WindowEnd   string `json:"window_end"`
	Reason      string `json:"reason,omitempty"`
}

func (s *Server) handleHostedAccess(w http.ResponseWriter, r *http.Request) {
	if s == nil || !s.isHostedDeployment() {
		writeJSON(w, defaultHostedAccess())
		return
	}
	characterID := strings.TrimSpace(r.URL.Query().Get("character_id"))
	identity, identityOK := s.hostedBillingIdentityFromRequest(r, characterID, false)
	if !identityOK && characterID != "" {
		writeError(w, http.StatusUnauthorized, "verified EVE character identity is required")
		return
	}
	billingUserID := ""
	verifiedCharacterID := ""
	if identityOK {
		billingUserID = identity.UserID
		verifiedCharacterID = identity.CharacterID
	}
	access, err := s.fetchHostedAccess(r.Context(), billingUserID, verifiedCharacterID)
	if err != nil {
		access = defaultHostedAccess()
		access.Hosted = true
		access.Status = "free"
		access.Features["billing_unavailable"] = true
		access.Message = "Hosted billing service is temporarily unavailable."
		s.trackTelemetryEvent(r, telemetryEvent("entitlement_checked", "hosted", map[string]interface{}{
			"entitlement_result": "fallback",
			"error":              err.Error(),
			"character_id":       verifiedCharacterID,
		}))
	} else {
		s.trackTelemetryEvent(r, telemetryEvent("entitlement_checked", "hosted", map[string]interface{}{
			"entitlement_result":  "ok",
			"plan":                access.Plan.ID,
			"subscription_status": access.Status,
			"character_id":        verifiedCharacterID,
		}))
	}
	writeJSON(w, access)
}

func (s *Server) handleHostedPaymentRequest(w http.ResponseWriter, r *http.Request) {
	if s == nil || !s.isHostedDeployment() {
		writeError(w, http.StatusNotFound, "hosted payments are not configured")
		return
	}
	paymentURL := hostedPaymentURL()
	if paymentURL == "" {
		writeError(w, http.StatusNotFound, "hosted payments are not configured")
		return
	}
	var req hostedPaymentRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.PlanID = strings.TrimSpace(req.PlanID)
	req.CharacterID = strings.TrimSpace(req.CharacterID)
	if req.PlanID == "" {
		writeError(w, http.StatusBadRequest, "plan_id is required")
		return
	}
	identity, ok := s.hostedBillingIdentityFromRequest(r, req.CharacterID, true)
	if !ok {
		writeError(w, http.StatusUnauthorized, "verified EVE character identity is required")
		return
	}
	internalKey := hostedBillingInternalKey()
	if internalKey == "" {
		writeError(w, http.StatusServiceUnavailable, "hosted billing internal key is not configured")
		return
	}

	payload := map[string]string{
		"user_id":      identity.UserID,
		"plan_id":      req.PlanID,
		"character_id": identity.CharacterID,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "payment request failed")
		return
	}

	outReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, paymentURL, bytes.NewReader(body))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "payment request failed")
		return
	}
	outReq.Header.Set("Content-Type", "application/json")
	outReq.Header.Set("Accept", "application/json")
	outReq.Header.Set("X-Entitlements-Key", internalKey)

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(outReq)
	if err != nil {
		writeError(w, http.StatusBadGateway, "billing service is unavailable")
		return
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 128*1024)).Decode(&result); err != nil {
		writeError(w, http.StatusBadGateway, "billing service returned invalid json")
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := result["error"].(string)
		if strings.TrimSpace(message) == "" {
			message = fmt.Sprintf("billing service returned HTTP %d", resp.StatusCode)
		}
		writeError(w, resp.StatusCode, message)
		return
	}

	s.trackTelemetryEvent(r, telemetryEvent("plan_selected", "hosted", map[string]interface{}{
		"plan":         req.PlanID,
		"character_id": identity.CharacterID,
	}))
	writeJSON(w, result)
}

func (s *Server) handleHostedPaymentCancel(w http.ResponseWriter, r *http.Request) {
	if s == nil || !s.isHostedDeployment() {
		writeError(w, http.StatusNotFound, "hosted payments are not configured")
		return
	}
	cancelURL := hostedPaymentCancelURL()
	if cancelURL == "" {
		writeError(w, http.StatusNotFound, "hosted payments are not configured")
		return
	}
	var req hostedPaymentCancelRequest
	if r.Body != nil {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
	}
	req.CharacterID = strings.TrimSpace(req.CharacterID)
	identity, ok := s.hostedBillingIdentityFromRequest(r, req.CharacterID, true)
	if !ok {
		writeError(w, http.StatusUnauthorized, "verified EVE character identity is required")
		return
	}
	internalKey := hostedBillingInternalKey()
	if internalKey == "" {
		writeError(w, http.StatusServiceUnavailable, "hosted billing internal key is not configured")
		return
	}

	payload := map[string]string{
		"user_id":      identity.UserID,
		"character_id": identity.CharacterID,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "payment cancel failed")
		return
	}

	outReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, cancelURL, bytes.NewReader(body))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "payment cancel failed")
		return
	}
	outReq.Header.Set("Content-Type", "application/json")
	outReq.Header.Set("Accept", "application/json")
	outReq.Header.Set("X-Entitlements-Key", internalKey)

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(outReq)
	if err != nil {
		writeError(w, http.StatusBadGateway, "billing service is unavailable")
		return
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 128*1024)).Decode(&result); err != nil {
		writeError(w, http.StatusBadGateway, "billing service returned invalid json")
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := result["error"].(string)
		if strings.TrimSpace(message) == "" {
			message = fmt.Sprintf("billing service returned HTTP %d", resp.StatusCode)
		}
		writeError(w, resp.StatusCode, message)
		return
	}

	s.trackTelemetryEvent(r, telemetryEvent("payment_request_cancelled", "hosted", map[string]interface{}{
		"character_id": identity.CharacterID,
		"cancelled":    result["cancelled"],
	}))
	writeJSON(w, result)
}

func (s *Server) handleHostedPaymentMarkSent(w http.ResponseWriter, r *http.Request) {
	if s == nil || !s.isHostedDeployment() {
		writeError(w, http.StatusNotFound, "hosted payments are not configured")
		return
	}
	markURL := hostedPaymentMarkSentURL()
	if markURL == "" {
		writeError(w, http.StatusNotFound, "hosted payments are not configured")
		return
	}
	var req hostedPaymentMarkSentRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.CharacterID = strings.TrimSpace(req.CharacterID)
	req.Code = strings.ToUpper(strings.TrimSpace(req.Code))
	if req.Code == "" {
		writeError(w, http.StatusBadRequest, "payment code is required")
		return
	}
	identity, ok := s.hostedBillingIdentityFromRequest(r, req.CharacterID, true)
	if !ok {
		writeError(w, http.StatusUnauthorized, "verified EVE character identity is required")
		return
	}
	internalKey := hostedBillingInternalKey()
	if internalKey == "" {
		writeError(w, http.StatusServiceUnavailable, "hosted billing internal key is not configured")
		return
	}

	payload := map[string]string{
		"user_id":      identity.UserID,
		"character_id": identity.CharacterID,
		"code":         req.Code,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "payment mark sent failed")
		return
	}

	outReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, markURL, bytes.NewReader(body))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "payment mark sent failed")
		return
	}
	outReq.Header.Set("Content-Type", "application/json")
	outReq.Header.Set("Accept", "application/json")
	outReq.Header.Set("X-Entitlements-Key", internalKey)

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(outReq)
	if err != nil {
		writeError(w, http.StatusBadGateway, "billing service is unavailable")
		return
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 128*1024)).Decode(&result); err != nil {
		writeError(w, http.StatusBadGateway, "billing service returned invalid json")
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := result["error"].(string)
		if strings.TrimSpace(message) == "" {
			message = fmt.Sprintf("billing service returned HTTP %d", resp.StatusCode)
		}
		writeError(w, resp.StatusCode, message)
		return
	}

	s.trackTelemetryEvent(r, telemetryEvent("payment_marked_sent", "hosted", map[string]interface{}{
		"character_id": identity.CharacterID,
	}))
	writeJSON(w, result)
}

func (s *Server) fetchHostedAccess(ctx context.Context, userID string, characterID string) (hostedAccessResponse, error) {
	if s == nil || !s.isHostedDeployment() {
		return defaultHostedAccess(), nil
	}
	entitlementURL := strings.TrimSpace(os.Getenv(hostedEntitlementsURLEnv))
	if entitlementURL == "" {
		return defaultHostedAccess(), nil
	}

	endpoint, err := url.Parse(entitlementURL)
	if err != nil {
		return hostedAccessResponse{}, err
	}
	query := endpoint.Query()
	if strings.TrimSpace(userID) != "" {
		query.Set("user_id", strings.TrimSpace(userID))
	}
	if characterID != "" {
		query.Set("character_id", characterID)
	}
	endpoint.RawQuery = query.Encode()

	reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return hostedAccessResponse{}, err
	}
	if key := hostedBillingInternalKey(); key != "" {
		req.Header.Set("X-Entitlements-Key", key)
	} else if strings.TrimSpace(entitlementURL) != "" {
		return hostedAccessResponse{}, fmt.Errorf("hosted billing internal key is not configured")
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 2500 * time.Millisecond}
	resp, err := client.Do(req)
	if err != nil {
		return hostedAccessResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return hostedAccessResponse{}, fmt.Errorf("entitlement service returned HTTP %d", resp.StatusCode)
	}

	var access hostedAccessResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 128*1024)).Decode(&access); err != nil {
		return hostedAccessResponse{}, err
	}
	if access.Features == nil {
		access.Features = map[string]bool{}
	}
	if access.Usage == nil {
		access.Usage = map[string]hostedAccessUsage{}
	}
	if access.Plan.ID == "" {
		access.Plan.ID = "free"
	}
	if access.Plan.Name == "" {
		access.Plan.Name = access.Plan.ID
	}
	if access.Status == "" {
		access.Status = "free"
	}
	access.Hosted = true
	return access, nil
}

func hostedPaymentURL() string {
	if explicit := strings.TrimSpace(os.Getenv(hostedPaymentsURLEnv)); explicit != "" {
		return explicit
	}
	entitlementURL := strings.TrimSpace(os.Getenv(hostedEntitlementsURLEnv))
	if entitlementURL == "" {
		return ""
	}
	if strings.HasSuffix(entitlementURL, "/v1/entitlements/current") {
		return strings.TrimSuffix(entitlementURL, "/v1/entitlements/current") + "/v1/payments/request"
	}
	return ""
}

func hostedPaymentCancelURL() string {
	if explicit := strings.TrimSpace(os.Getenv(hostedPaymentsURLEnv)); explicit != "" {
		if strings.HasSuffix(explicit, "/v1/payments/request") {
			return strings.TrimSuffix(explicit, "/v1/payments/request") + "/v1/payments/cancel"
		}
		if strings.HasSuffix(explicit, "/request") {
			return strings.TrimSuffix(explicit, "/request") + "/cancel"
		}
	}
	entitlementURL := strings.TrimSpace(os.Getenv(hostedEntitlementsURLEnv))
	if entitlementURL == "" {
		return ""
	}
	if strings.HasSuffix(entitlementURL, "/v1/entitlements/current") {
		return strings.TrimSuffix(entitlementURL, "/v1/entitlements/current") + "/v1/payments/cancel"
	}
	return ""
}

func hostedPaymentMarkSentURL() string {
	if explicit := strings.TrimSpace(os.Getenv(hostedPaymentsURLEnv)); explicit != "" {
		if strings.HasSuffix(explicit, "/v1/payments/request") {
			return strings.TrimSuffix(explicit, "/v1/payments/request") + "/v1/payments/mark-sent"
		}
		if strings.HasSuffix(explicit, "/request") {
			return strings.TrimSuffix(explicit, "/request") + "/mark-sent"
		}
	}
	entitlementURL := strings.TrimSpace(os.Getenv(hostedEntitlementsURLEnv))
	if entitlementURL == "" {
		return ""
	}
	if strings.HasSuffix(entitlementURL, "/v1/entitlements/current") {
		return strings.TrimSuffix(entitlementURL, "/v1/entitlements/current") + "/v1/payments/mark-sent"
	}
	return ""
}

func hostedUsageURL() string {
	if explicit := strings.TrimSpace(os.Getenv(hostedUsageURLEnv)); explicit != "" {
		return explicit
	}
	entitlementURL := strings.TrimSpace(os.Getenv(hostedEntitlementsURLEnv))
	if entitlementURL == "" {
		return ""
	}
	if strings.HasSuffix(entitlementURL, "/v1/entitlements/current") {
		return strings.TrimSuffix(entitlementURL, "/v1/entitlements/current") + "/v1/usage/consume"
	}
	return ""
}

func (s *Server) hostedQuotaMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		feature, ok := hostedQuotaFeatureForRequest(r)
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		if !s.hostedBillingConfigured() {
			next.ServeHTTP(w, r)
			return
		}
		if hostedUsageURL() == "" {
			writeError(w, http.StatusServiceUnavailable, "hosted usage enforcement is not configured")
			s.trackTelemetryEvent(r, telemetryEvent("quota_enforcement_unconfigured", "hosted", map[string]interface{}{
				"feature": feature,
			}))
			return
		}
		decision, err := s.consumeHostedUsage(r, feature)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "billing service is unavailable")
			return
		}
		if !decision.Allowed {
			status := http.StatusForbidden
			switch decision.Reason {
			case "hosted_identity_required":
				status = http.StatusUnauthorized
			case "quota_exhausted":
				status = http.StatusTooManyRequests
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error":    firstNonEmpty(decision.Reason, "feature_denied"),
				"decision": decision,
			})
			s.trackTelemetryEvent(r, telemetryEvent(firstNonEmpty(decision.Reason, "feature_denied"), "hosted", map[string]interface{}{
				"feature":         feature,
				"quota_used":      decision.Used,
				"quota_limit":     decision.Limit,
				"quota_remaining": decision.Remaining,
				"usage_window":    decision.Window,
			}))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) consumeHostedUsage(r *http.Request, feature string) (hostedUsageDecision, error) {
	usageURL := hostedUsageURL()
	if usageURL == "" {
		return hostedUsageDecision{Allowed: true, Feature: feature}, nil
	}
	internalKey := hostedBillingInternalKey()
	if internalKey == "" && s.hostedBillingConfigured() {
		return hostedUsageDecision{}, fmt.Errorf("hosted billing internal key is not configured")
	}
	identity, ok := s.hostedBillingIdentityFromRequest(r, "", true)
	if !ok {
		return hostedUsageDecision{
			Allowed: false,
			Feature: feature,
			Reason:  "hosted_identity_required",
		}, nil
	}
	payload := map[string]interface{}{
		"user_id":      identity.UserID,
		"character_id": identity.CharacterID,
		"feature":      feature,
		"units":        1,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return hostedUsageDecision{}, err
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, usageURL, bytes.NewReader(body))
	if err != nil {
		return hostedUsageDecision{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if internalKey != "" {
		req.Header.Set("X-Entitlements-Key", internalKey)
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return hostedUsageDecision{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return hostedUsageDecision{}, fmt.Errorf("usage service returned HTTP %d", resp.StatusCode)
	}
	var decision hostedUsageDecision
	if err := json.NewDecoder(io.LimitReader(resp.Body, 128*1024)).Decode(&decision); err != nil {
		return hostedUsageDecision{}, err
	}
	return decision, nil
}

func hostedBillingUserIDFromRequest(r *http.Request) (string, bool) {
	userID := strings.TrimSpace(userIDFromRequest(r))
	if !isValidUserID(userID) || userID == db.DefaultUserID {
		return "", false
	}
	return userID, true
}

type hostedBillingIdentity struct {
	UserID        string
	BrowserUserID string
	CharacterID   string
	CharacterName string
}

func (s *Server) hostedBillingIdentityFromRequest(r *http.Request, requestedCharacterID string, requireCharacter bool) (hostedBillingIdentity, bool) {
	browserUserID, ok := hostedBillingUserIDFromRequest(r)
	if !ok {
		return hostedBillingIdentity{}, false
	}
	requestedCharacterID = strings.TrimSpace(requestedCharacterID)
	if s != nil && s.sessions != nil {
		if requestedCharacterID != "" {
			characterID, err := strconv.ParseInt(requestedCharacterID, 10, 64)
			if err != nil || characterID <= 0 {
				return hostedBillingIdentity{}, false
			}
			sess := s.sessions.GetByCharacterIDForUser(browserUserID, characterID)
			if sess == nil || sess.CharacterID <= 0 {
				return hostedBillingIdentity{}, false
			}
			return hostedBillingIdentityForSession(browserUserID, sess.CharacterID, sess.CharacterName), true
		}
		if sess := s.sessions.GetForUser(browserUserID); sess != nil && sess.CharacterID > 0 {
			return hostedBillingIdentityForSession(browserUserID, sess.CharacterID, sess.CharacterName), true
		}
	}
	if requireCharacter {
		return hostedBillingIdentity{}, false
	}
	return hostedBillingIdentity{
		UserID:        browserUserID,
		BrowserUserID: browserUserID,
	}, true
}

func hostedBillingIdentityForSession(browserUserID string, characterID int64, characterName string) hostedBillingIdentity {
	return hostedBillingIdentity{
		UserID:        hostedBillingUserIDForCharacter(characterID),
		BrowserUserID: browserUserID,
		CharacterID:   fmt.Sprintf("%d", characterID),
		CharacterName: strings.TrimSpace(characterName),
	}
}

func hostedBillingUserIDForCharacter(characterID int64) string {
	if characterID <= 0 {
		return ""
	}
	return fmt.Sprintf("eve_character_%d", characterID)
}

func (s *Server) hostedBillingConfigured() bool {
	return s != nil && s.isHostedDeployment()
}

func hostedBillingInternalKey() string {
	return strings.TrimSpace(os.Getenv(hostedEntitlementsKeyEnv))
}

func hostedQuotaFeatureForRequest(r *http.Request) (string, bool) {
	if r == nil || r.Method != http.MethodPost {
		return "", false
	}
	path := r.URL.Path
	switch {
	case path == "/api/scan",
		path == "/api/scan/multi-region",
		path == "/api/scan/regional-day",
		path == "/api/scan/contracts",
		path == "/api/scan/station",
		path == "/api/backtest/flips",
		path == "/api/orderbook/coverage",
		path == "/api/route/find",
		path == "/api/industry/analyze",
		path == "/api/execution/plan",
		path == "/api/demand/refresh",
		path == "/api/auth/station/cache/reboot",
		path == "/api/auth/station/command",
		path == "/api/auth/industry/coverage",
		isHostedQuotaIndustryProjectComputePath(path):
		return "scans", true
	case path == "/api/auth/station/ai/chat",
		path == "/api/auth/station/ai/chat/stream":
		return "station_ai", true
	default:
		return "", false
	}
}

func isHostedQuotaIndustryProjectComputePath(path string) bool {
	if !strings.HasPrefix(path, "/api/auth/industry/projects/") {
		return false
	}
	return strings.HasSuffix(path, "/plan/preview") ||
		strings.HasSuffix(path, "/plan") ||
		strings.HasSuffix(path, "/materials/rebalance") ||
		strings.HasSuffix(path, "/blueprints/sync")
}

func defaultHostedAccess() hostedAccessResponse {
	limit := 25
	remaining := 25
	return hostedAccessResponse{
		Hosted: false,
		Plan: hostedAccessPlan{
			ID:   "local",
			Name: "Local",
		},
		Status: "local",
		Features: map[string]bool{
			"basic_scans":     true,
			"local_vault":     true,
			"multibuy_export": true,
		},
		Usage: map[string]hostedAccessUsage{
			"scans": {
				Used:      0,
				Limit:     &limit,
				Remaining: &remaining,
				Window:    "day",
			},
		},
		Message: "Local build. Hosted plans are available on app.eveflipper.com.",
	}
}

func telemetryEvent(eventType string, module string, props map[string]interface{}) telemetry.Event {
	return telemetry.Event{
		EventType:  eventType,
		Source:     "backend",
		Module:     module,
		Properties: props,
	}
}
