package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"eve-flipper/internal/auth"
	"eve-flipper/internal/config"
)

const hostedTestBrowserUserID = "hosted-user"
const hostedTestCharacterID int64 = 123
const hostedTestBillingUserID = "eve_character_123"

func enableHostedMode(t *testing.T) {
	t.Helper()
	t.Setenv("EVEFLIPPER_HOSTED", "true")
	t.Setenv(hostedEntitlementsKeyEnv, "test-hosted-key")
}

func newHostedBillingTestServer(t *testing.T, userID string, characterID int64) *Server {
	t.Helper()
	store := newSessionStoreForAPITest(t)
	if err := store.SaveAndActivateForUser(userID, &auth.Session{
		CharacterID:   characterID,
		CharacterName: "Billing Test",
		AccessToken:   "access-token",
		RefreshToken:  "refresh-token",
		ExpiresAt:     time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("SaveAndActivateForUser: %v", err)
	}
	return NewServer(config.Default(), nil, nil, nil, store)
}

func withHostedUser(req *http.Request) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), userIDContextKey, hostedTestBrowserUserID))
}

func TestHostedAccessDefaultsToLocal(t *testing.T) {
	t.Setenv(hostedEntitlementsURLEnv, "")
	server := NewServer(config.Default(), nil, nil, nil, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/hosted/access", nil)
	server.handleHostedAccess(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var got hostedAccessResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Hosted {
		t.Fatalf("hosted = true, want false")
	}
	if got.Plan.ID != "local" || got.Status != "local" {
		t.Fatalf("unexpected default access: %#v", got)
	}
}

func TestHostedAccessIgnoresBillingEnvOutsideHostedMode(t *testing.T) {
	called := false
	entitlements := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer entitlements.Close()
	t.Setenv("EVEFLIPPER_HOSTED", "")
	t.Setenv("TELEMETRY_ENABLED", "true")
	t.Setenv("TELEMETRY_ENV", "hosted")
	t.Setenv(hostedEntitlementsURLEnv, entitlements.URL)
	t.Setenv(hostedEntitlementsKeyEnv, "secret-key")
	server := NewServer(config.Default(), nil, nil, nil, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/hosted/access", nil)
	server.handleHostedAccess(rec, req)

	if called {
		t.Fatal("entitlement service must not be called outside hosted deployment")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var got hostedAccessResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Hosted || got.Plan.ID != "local" || got.Status != "local" {
		t.Fatalf("ordinary build must stay local, got %#v", got)
	}
}

func TestHostedPaymentRequestProxiesBillingService(t *testing.T) {
	const apiKey = "payment-key"
	enableHostedMode(t)
	billing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/payments/request" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("X-Entitlements-Key"); got != apiKey {
			t.Fatalf("X-Entitlements-Key = %q, want %q", got, apiKey)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["user_id"] != hostedTestBillingUserID || body["plan_id"] != "trader" || body["character_id"] != "123" {
			t.Fatalf("unexpected body: %#v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok": true,
			"payment": map[string]interface{}{
				"amount_isk":  300000000,
				"reason_code": "EFLIP-TEST",
			},
		})
	}))
	defer billing.Close()

	t.Setenv(hostedPaymentsURLEnv, billing.URL+"/v1/payments/request")
	t.Setenv(hostedEntitlementsKeyEnv, apiKey)
	server := newHostedBillingTestServer(t, hostedTestBrowserUserID, hostedTestCharacterID)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/hosted/payments/request", bytes.NewBufferString(`{"plan_id":"trader","character_id":"123"}`))
	req = withHostedUser(req)
	server.handleHostedPaymentRequest(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var got map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["ok"] != true {
		t.Fatalf("unexpected response: %#v", got)
	}
}

func TestHostedPaymentCancelProxiesBillingService(t *testing.T) {
	const apiKey = "payment-key"
	enableHostedMode(t)
	billing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/payments/cancel" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("X-Entitlements-Key"); got != apiKey {
			t.Fatalf("X-Entitlements-Key = %q, want %q", got, apiKey)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["user_id"] != hostedTestBillingUserID || body["character_id"] != "123" {
			t.Fatalf("unexpected body: %#v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":        true,
			"cancelled": 2,
		})
	}))
	defer billing.Close()

	t.Setenv(hostedPaymentsURLEnv, billing.URL+"/v1/payments/request")
	t.Setenv(hostedEntitlementsKeyEnv, apiKey)
	server := newHostedBillingTestServer(t, hostedTestBrowserUserID, hostedTestCharacterID)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/hosted/payments/cancel", bytes.NewBufferString(`{"character_id":"123"}`))
	req = withHostedUser(req)
	server.handleHostedPaymentCancel(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var got map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["ok"] != true || got["cancelled"].(float64) != 2 {
		t.Fatalf("unexpected response: %#v", got)
	}
}

func TestHostedPaymentMarkSentProxiesBillingService(t *testing.T) {
	const apiKey = "payment-key"
	enableHostedMode(t)
	billing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/payments/mark-sent" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("X-Entitlements-Key"); got != apiKey {
			t.Fatalf("X-Entitlements-Key = %q, want %q", got, apiKey)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["user_id"] != hostedTestBillingUserID || body["character_id"] != "123" || body["code"] != "EFLIP-A1B2C3D4E5F6" {
			t.Fatalf("unexpected body: %#v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok": true,
			"payment": map[string]interface{}{
				"amount_isk":     300000000,
				"reason_code":    "EFLIP-A1B2C3D4E5F6",
				"sent_marked_at": "2026-06-20T00:00:00Z",
			},
		})
	}))
	defer billing.Close()

	t.Setenv(hostedPaymentsURLEnv, billing.URL+"/v1/payments/request")
	t.Setenv(hostedEntitlementsKeyEnv, apiKey)
	server := newHostedBillingTestServer(t, hostedTestBrowserUserID, hostedTestCharacterID)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/hosted/payments/mark-sent", bytes.NewBufferString(`{"character_id":"123","code":"eflip-a1b2c3d4e5f6"}`))
	req = withHostedUser(req)
	server.handleHostedPaymentMarkSent(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var got map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["ok"] != true {
		t.Fatalf("unexpected response: %#v", got)
	}
}

func TestHostedAccessPreservesCorporationReceiverInstructions(t *testing.T) {
	const apiKey = "entitlement-key"
	enableHostedMode(t)
	entitlements := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Entitlements-Key"); got != apiKey {
			t.Fatalf("X-Entitlements-Key = %q, want %q", got, apiKey)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"hosted": true,
			"plan": map[string]interface{}{
				"id":   "free",
				"name": "Free",
			},
			"status":   "free",
			"features": map[string]bool{"basic_scans": true},
			"usage":    map[string]interface{}{},
			"payment": map[string]interface{}{
				"receiver_name":           "EVE Flipper Corp Wallet",
				"receiver_corporation_id": int64(987654321),
				"amount_isk":              int64(300000000),
				"reason_code":             "EFLIP-CORPTEST",
			},
		})
	}))
	defer entitlements.Close()

	t.Setenv(hostedEntitlementsURLEnv, entitlements.URL)
	t.Setenv(hostedEntitlementsKeyEnv, apiKey)
	server := NewServer(config.Default(), nil, nil, nil, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/hosted/access", nil)
	req = withHostedUser(req)
	server.handleHostedAccess(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var got hostedAccessResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Payment == nil {
		t.Fatalf("payment missing: %#v", got)
	}
	if got.Payment.ReceiverCorporationID == nil || *got.Payment.ReceiverCorporationID != 987654321 {
		t.Fatalf("receiver corporation id = %#v, want 987654321", got.Payment.ReceiverCorporationID)
	}
}

func TestHostedPaymentRequestDisabledOutsideHostedMode(t *testing.T) {
	called := false
	billing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer billing.Close()
	t.Setenv("EVEFLIPPER_HOSTED", "")
	t.Setenv(hostedPaymentsURLEnv, billing.URL+"/v1/payments/request")
	t.Setenv(hostedEntitlementsKeyEnv, "secret-key")
	server := NewServer(config.Default(), nil, nil, nil, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/hosted/payments/request", bytes.NewBufferString(`{"plan_id":"trader"}`))
	req = withHostedUser(req)
	server.handleHostedPaymentRequest(rec, req)

	if called {
		t.Fatal("billing service must not be called outside hosted deployment")
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rec.Code, rec.Body.String())
	}
}

func TestHostedPaymentRequestRequiresUserIdentity(t *testing.T) {
	enableHostedMode(t)
	called := false
	billing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer billing.Close()

	t.Setenv(hostedPaymentsURLEnv, billing.URL+"/v1/payments/request")
	server := NewServer(config.Default(), nil, nil, nil, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/hosted/payments/request", bytes.NewBufferString(`{"plan_id":"trader"}`))
	server.handleHostedPaymentRequest(rec, req)

	if called {
		t.Fatal("billing service must not be called without hosted user identity")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", rec.Code, rec.Body.String())
	}
}

func TestHostedPaymentRequestRejectsUnownedCharacter(t *testing.T) {
	enableHostedMode(t)
	called := false
	billing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer billing.Close()

	t.Setenv(hostedPaymentsURLEnv, billing.URL+"/v1/payments/request")
	server := newHostedBillingTestServer(t, hostedTestBrowserUserID, hostedTestCharacterID)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/hosted/payments/request", bytes.NewBufferString(`{"plan_id":"trader","character_id":"999"}`))
	req = withHostedUser(req)
	server.handleHostedPaymentRequest(rec, req)

	if called {
		t.Fatal("billing service must not be called for unowned character_id")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", rec.Code, rec.Body.String())
	}
}

func TestHostedPaymentRequestRequiresInternalKey(t *testing.T) {
	enableHostedMode(t)
	called := false
	billing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer billing.Close()

	t.Setenv(hostedPaymentsURLEnv, billing.URL+"/v1/payments/request")
	t.Setenv(hostedEntitlementsKeyEnv, "")
	server := newHostedBillingTestServer(t, hostedTestBrowserUserID, hostedTestCharacterID)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/hosted/payments/request", bytes.NewBufferString(`{"plan_id":"trader"}`))
	req = withHostedUser(req)
	server.handleHostedPaymentRequest(rec, req)

	if called {
		t.Fatal("billing service must not be called without hosted internal key")
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", rec.Code, rec.Body.String())
	}
}

func TestHostedQuotaMiddlewareBlocksExhaustedQuota(t *testing.T) {
	enableHostedMode(t)
	usage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["user_id"] != hostedTestBillingUserID || body["character_id"] != "123" {
			t.Fatalf("unexpected usage body: %#v", body)
		}
		_ = json.NewEncoder(w).Encode(hostedUsageDecision{
			Allowed: false,
			Feature: "scans",
			Used:    25,
			Reason:  "quota_exhausted",
		})
	}))
	defer usage.Close()
	t.Setenv(hostedUsageURLEnv, usage.URL)
	server := newHostedBillingTestServer(t, hostedTestBrowserUserID, hostedTestCharacterID)

	nextCalled := false
	handler := server.hostedQuotaMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/scan", nil)
	req = withHostedUser(req)
	handler.ServeHTTP(rec, req)

	if nextCalled {
		t.Fatal("next handler must not run when quota is exhausted")
	}
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429; body=%s", rec.Code, rec.Body.String())
	}
}

func TestHostedQuotaMiddlewareRequiresUserIdentity(t *testing.T) {
	enableHostedMode(t)
	called := false
	usage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer usage.Close()
	t.Setenv(hostedUsageURLEnv, usage.URL)
	server := NewServer(config.Default(), nil, nil, nil, nil)

	nextCalled := false
	handler := server.hostedQuotaMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/scan", nil)
	handler.ServeHTTP(rec, req)

	if called {
		t.Fatal("usage service must not be called without hosted user identity")
	}
	if nextCalled {
		t.Fatal("next handler must not run without hosted user identity")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", rec.Code, rec.Body.String())
	}
}

func TestHostedQuotaMiddlewareAllowsUsage(t *testing.T) {
	enableHostedMode(t)
	usage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["feature"] != "station_ai" {
			t.Fatalf("feature = %#v, want station_ai", body["feature"])
		}
		if body["user_id"] != hostedTestBillingUserID || body["character_id"] != "123" {
			t.Fatalf("unexpected usage body: %#v", body)
		}
		_ = json.NewEncoder(w).Encode(hostedUsageDecision{Allowed: true, Feature: "station_ai", Used: 1})
	}))
	defer usage.Close()
	t.Setenv(hostedUsageURLEnv, usage.URL)
	server := newHostedBillingTestServer(t, hostedTestBrowserUserID, hostedTestCharacterID)

	nextCalled := false
	handler := server.hostedQuotaMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusNoContent)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/station/ai/chat", nil)
	req = withHostedUser(req)
	handler.ServeHTTP(rec, req)

	if !nextCalled {
		t.Fatal("next handler should run when usage is allowed")
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

func TestHostedQuotaMiddlewareAllowsLocalModeWithoutUsageService(t *testing.T) {
	t.Setenv(hostedUsageURLEnv, "")
	t.Setenv(hostedEntitlementsURLEnv, "")
	t.Setenv(hostedPaymentsURLEnv, "")
	t.Setenv("EVEFLIPPER_HOSTED", "")
	server := NewServer(config.Default(), nil, nil, nil, nil)

	nextCalled := false
	handler := server.hostedQuotaMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusNoContent)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/scan", nil)
	handler.ServeHTTP(rec, req)

	if !nextCalled {
		t.Fatal("local mode should allow heavy endpoints without hosted usage service")
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

func TestHostedQuotaMiddlewareAllowsLocalModeWithStrayUsageService(t *testing.T) {
	called := false
	usage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer usage.Close()
	t.Setenv("EVEFLIPPER_HOSTED", "")
	t.Setenv(hostedUsageURLEnv, usage.URL)
	t.Setenv(hostedEntitlementsKeyEnv, "secret-key")
	server := NewServer(config.Default(), nil, nil, nil, nil)

	nextCalled := false
	handler := server.hostedQuotaMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusNoContent)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/scan", nil)
	req = withHostedUser(req)
	handler.ServeHTTP(rec, req)

	if called {
		t.Fatal("usage service must not be called outside hosted deployment")
	}
	if !nextCalled {
		t.Fatal("local heavy endpoint should run outside hosted deployment")
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

func TestHostedQuotaMiddlewareFailsClosedWhenUsageServiceMissing(t *testing.T) {
	enableHostedMode(t)
	t.Setenv(hostedUsageURLEnv, "")
	t.Setenv(hostedEntitlementsURLEnv, "http://billing.test/v1/entitlements/current")
	server := newHostedBillingTestServer(t, hostedTestBrowserUserID, hostedTestCharacterID)

	nextCalled := false
	handler := server.hostedQuotaMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusNoContent)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/scan", nil)
	req = withHostedUser(req)
	handler.ServeHTTP(rec, req)

	if nextCalled {
		t.Fatal("hosted heavy endpoint must not run without usage enforcement")
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", rec.Code, rec.Body.String())
	}
}

func TestHostedQuotaMiddlewareFailsClosedWhenInternalKeyMissing(t *testing.T) {
	enableHostedMode(t)
	called := false
	usage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer usage.Close()
	t.Setenv(hostedUsageURLEnv, usage.URL)
	t.Setenv(hostedEntitlementsKeyEnv, "")
	server := NewServer(config.Default(), nil, nil, nil, nil)

	nextCalled := false
	handler := server.hostedQuotaMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusNoContent)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/scan", nil)
	req = withHostedUser(req)
	handler.ServeHTTP(rec, req)

	if called {
		t.Fatal("usage service must not be called without hosted internal key")
	}
	if nextCalled {
		t.Fatal("hosted heavy endpoint must not run without hosted internal key")
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", rec.Code, rec.Body.String())
	}
}

func TestHostedQuotaFeatureMappingCoversHeavyHostedPosts(t *testing.T) {
	tests := []struct {
		method  string
		path    string
		feature string
	}{
		{http.MethodPost, "/api/scan", "scans"},
		{http.MethodPost, "/api/scan/multi-region", "scans"},
		{http.MethodPost, "/api/scan/regional-day", "scans"},
		{http.MethodPost, "/api/scan/contracts", "scans"},
		{http.MethodPost, "/api/scan/station", "scans"},
		{http.MethodPost, "/api/backtest/flips", "scans"},
		{http.MethodPost, "/api/orderbook/coverage", "scans"},
		{http.MethodPost, "/api/route/find", "scans"},
		{http.MethodPost, "/api/industry/analyze", "scans"},
		{http.MethodPost, "/api/execution/plan", "scans"},
		{http.MethodPost, "/api/demand/refresh", "scans"},
		{http.MethodPost, "/api/auth/station/cache/reboot", "scans"},
		{http.MethodPost, "/api/auth/station/command", "scans"},
		{http.MethodPost, "/api/auth/industry/coverage", "scans"},
		{http.MethodPost, "/api/auth/industry/projects/42/plan/preview", "scans"},
		{http.MethodPost, "/api/auth/industry/projects/42/plan", "scans"},
		{http.MethodPost, "/api/auth/industry/projects/42/materials/rebalance", "scans"},
		{http.MethodPost, "/api/auth/industry/projects/42/blueprints/sync", "scans"},
		{http.MethodPost, "/api/auth/station/ai/chat", "station_ai"},
		{http.MethodPost, "/api/auth/station/ai/chat/stream", "station_ai"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			feature, ok := hostedQuotaFeatureForRequest(req)
			if !ok || feature != tt.feature {
				t.Fatalf("feature = %q ok=%v, want %q true", feature, ok, tt.feature)
			}
		})
	}
}

func TestHostedQuotaFeatureMappingClassifiesAllPostAPIRoutes(t *testing.T) {
	source, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`mux\.HandleFunc\("POST (/api/[^"]+)"`)
	matches := re.FindAllStringSubmatch(string(source), -1)
	if len(matches) == 0 {
		t.Fatal("no POST /api routes found in server.go")
	}
	intentionallyUnmetered := map[string]string{
		"/api/update/skip":                           "desktop update preference",
		"/api/update/apply":                          "desktop update action",
		"/api/internal/wiki/gollum":                  "internal webhook",
		"/api/telemetry/client":                      "telemetry ingest",
		"/api/hosted/payments/request":               "billing request has dedicated payment limits",
		"/api/hosted/payments/mark-sent":             "billing sent marker has dedicated payment limits",
		"/api/hosted/payments/cancel":                "billing cancel has dedicated payment limits",
		"/api/config":                                "local config write",
		"/api/cockpit/loadouts":                      "cockpit CRUD",
		"/api/cockpit/loadouts/{loadoutID}/activate": "cockpit CRUD",
		"/api/alerts/test":                           "local notification test",
		"/api/orderbook/cleanup":                     "hosted maintenance endpoint",
		"/api/watchlist":                             "watchlist CRUD",
		"/api/scan/history/clear":                    "history cleanup",
		"/api/auth/logout":                           "auth session action",
		"/api/auth/character/select":                 "auth session action",
		"/api/security/vault/setup":                  "local vault action",
		"/api/security/vault/unlock":                 "local vault action",
		"/api/security/vault/lock":                   "local vault action",
		"/api/security/vault/reset":                  "local vault action",
		"/api/auth/station/trade-states/set":         "trade-state CRUD",
		"/api/auth/station/trade-states/delete":      "trade-state CRUD",
		"/api/auth/station/trade-states/clear":       "trade-state CRUD",
		"/api/auth/paper-trades":                     "paper-trade CRUD",
		"/api/auth/paper-trades/reconcile":           "paper-trade CRUD",
		"/api/auth/achievements/seen":                "achievement state",
		"/api/auth/industry/projects":                "industry project CRUD",
		"/api/ui/open-market":                        "ESI UI action",
		"/api/ui/set-waypoint":                       "ESI UI action",
		"/api/ui/open-contract":                      "ESI UI action",
	}
	var unclassified []string
	for _, match := range matches {
		path := match[1]
		samplePath := strings.ReplaceAll(path, "{projectID}", "42")
		samplePath = strings.ReplaceAll(samplePath, "{loadoutID}", "default")
		req := httptest.NewRequest(http.MethodPost, samplePath, nil)
		if _, ok := hostedQuotaFeatureForRequest(req); ok {
			continue
		}
		if _, ok := intentionallyUnmetered[path]; ok {
			continue
		}
		unclassified = append(unclassified, path)
	}
	if len(unclassified) > 0 {
		t.Fatalf("POST /api routes must be metered or explicitly classified as unmetered: %v", unclassified)
	}
}

func TestHostedQuotaFeatureMappingSkipsCheapOrLocalRequests(t *testing.T) {
	for _, tt := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/auth/portfolio"},
		{http.MethodPost, "/api/watchlist"},
		{http.MethodPost, "/api/auth/industry/projects"},
		{http.MethodPatch, "/api/auth/industry/tasks/status"},
	} {
		req := httptest.NewRequest(tt.method, tt.path, nil)
		if feature, ok := hostedQuotaFeatureForRequest(req); ok {
			t.Fatalf("%s %s unexpectedly mapped to %q", tt.method, tt.path, feature)
		}
	}
}

func TestHostedAccessProxiesEntitlementService(t *testing.T) {
	const apiKey = "secret-key"
	enableHostedMode(t)
	entitlements := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Entitlements-Key"); got != apiKey {
			t.Fatalf("X-Entitlements-Key = %q, want %q", got, apiKey)
		}
		if got := r.URL.Query().Get("character_id"); got != "123" {
			t.Fatalf("character_id = %q, want 123", got)
		}
		if got := r.URL.Query().Get("user_id"); got != hostedTestBillingUserID {
			t.Fatalf("user_id = %q, want %q", got, hostedTestBillingUserID)
		}
		_ = json.NewEncoder(w).Encode(hostedAccessResponse{
			Plan:   hostedAccessPlan{ID: "pro", Name: "Pro"},
			Status: "active",
			Features: map[string]bool{
				"advanced_routes": true,
			},
			Usage: map[string]hostedAccessUsage{},
		})
	}))
	defer entitlements.Close()

	t.Setenv(hostedEntitlementsURLEnv, entitlements.URL)
	t.Setenv(hostedEntitlementsKeyEnv, apiKey)
	server := newHostedBillingTestServer(t, hostedTestBrowserUserID, hostedTestCharacterID)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/hosted/access?character_id=123", nil)
	req = withHostedUser(req)
	server.handleHostedAccess(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var got hostedAccessResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Hosted || got.Plan.ID != "pro" || got.Status != "active" {
		t.Fatalf("unexpected proxied access: %#v", got)
	}
	if !got.Features["advanced_routes"] {
		t.Fatalf("expected advanced_routes feature: %#v", got.Features)
	}
}
