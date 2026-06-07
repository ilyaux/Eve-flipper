package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"eve-flipper/internal/auth"
	"eve-flipper/internal/config"
	"eve-flipper/internal/esi"

	_ "modernc.org/sqlite"
)

// GET /api/status is not tested here because it calls esi.Client.HealthCheck() which performs a real HTTP request.

func TestHandleGetConfig_ReturnsConfig(t *testing.T) {
	cfg := &config.Config{SystemName: "Jita", CargoCapacity: 10000}
	srv := NewServer(cfg, &esi.Client{}, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /api/config status = %d, want 200", rec.Code)
	}
	var out config.Config
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if out.SystemName != "Jita" || out.CargoCapacity != 10000 {
		t.Errorf("config = %+v", out)
	}
}

func newSessionStoreForAPITest(t *testing.T) *auth.SessionStore {
	t.Helper()

	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	_, err = sqlDB.Exec(`
		CREATE TABLE auth_session (
			user_id         TEXT NOT NULL,
			character_id    INTEGER NOT NULL,
			character_name  TEXT NOT NULL,
			access_token    TEXT NOT NULL,
			refresh_token   TEXT NOT NULL,
			expires_at      INTEGER NOT NULL,
			is_active       INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (user_id, character_id)
		)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	return auth.NewSessionStore(sqlDB)
}

func newVaultSessionStoreForAPITest(t *testing.T) *auth.SessionStore {
	t.Helper()

	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open vault db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	_, err = sqlDB.Exec(`
		CREATE TABLE auth_session (
			user_id         TEXT NOT NULL,
			character_id    INTEGER NOT NULL,
			character_name  TEXT NOT NULL,
			access_token    TEXT NOT NULL,
			refresh_token   TEXT NOT NULL,
			expires_at      INTEGER NOT NULL,
			is_active       INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (user_id, character_id)
		);
		CREATE TABLE vault_state (
			user_id              TEXT PRIMARY KEY,
			mode                 TEXT NOT NULL,
			status               TEXT NOT NULL,
			schema_version       INTEGER NOT NULL DEFAULT 1,
			checkpoint_version   INTEGER NOT NULL DEFAULT 1,
			kdf_alg              TEXT NOT NULL DEFAULT '',
			kdf_salt             TEXT NOT NULL DEFAULT '',
			wrapped_key          TEXT NOT NULL,
			key_check            TEXT NOT NULL,
			plaintext_purged_at  TEXT NOT NULL,
			created_at           TEXT NOT NULL,
			updated_at           TEXT NOT NULL
		);
		CREATE TABLE security_events (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id     TEXT NOT NULL,
			event_type  TEXT NOT NULL DEFAULT '',
			detail      TEXT NOT NULL DEFAULT '',
			created_at  TEXT NOT NULL
		);`)
	if err != nil {
		t.Fatalf("create vault tables: %v", err)
	}

	return auth.NewSessionStore(sqlDB)
}

func addSignedUserCookie(req *http.Request, srv *Server, userID string) {
	req.AddCookie(&http.Cookie{
		Name:  userIDCookieName,
		Value: srv.signedUserIDCookieValue(userID),
		Path:  "/",
	})
}

func TestHandleAuthStructures_UsesRequestedCharacterScope(t *testing.T) {
	store := newSessionStoreForAPITest(t)

	err := store.SaveAndActivateForUser("u1", &auth.Session{
		CharacterID:   1001,
		CharacterName: "Expired Active",
		AccessToken:   "expired-token",
		RefreshToken:  "expired-refresh",
		ExpiresAt:     time.Now().Add(-2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("SaveAndActivateForUser(active): %v", err)
	}

	err = store.SaveForUser("u1", &auth.Session{
		CharacterID:   2002,
		CharacterName: "Scoped Pilot",
		AccessToken:   "scoped-token",
		RefreshToken:  "scoped-refresh",
		ExpiresAt:     time.Now().Add(15 * time.Minute),
	})
	if err != nil {
		t.Fatalf("SaveForUser(scoped): %v", err)
	}

	srv := NewServer(config.Default(), &esi.Client{}, nil, nil, nil)
	srv.sessions = store

	req := httptest.NewRequest(http.MethodGet, "/api/auth/structures?character_id=2002", nil)
	addSignedUserCookie(req, srv, "u1")
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/auth/structures status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var out []any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode structures response: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("structures response length = %d, want 0", len(out))
	}
}

func TestReadBodyWithLimit(t *testing.T) {
	t.Parallel()

	body, err := readBodyWithLimit(strings.NewReader("abc"), 3)
	if err != nil {
		t.Fatalf("readBodyWithLimit exact size failed: %v", err)
	}
	if string(body) != "abc" {
		t.Fatalf("body = %q, want %q", string(body), "abc")
	}

	_, err = readBodyWithLimit(strings.NewReader("abcd"), 3)
	if err == nil {
		t.Fatalf("expected size-limit error for oversized body")
	}
}

func TestWalletTxnCache_IsolatedByCharacterAndClearable(t *testing.T) {
	srv := &Server{}
	txns := []esi.WalletTransaction{
		{TransactionID: 1, TypeID: 34, Quantity: 10},
	}

	srv.setWalletTxnCache(1001, txns)

	if got, ok := srv.getWalletTxnCache(1001); !ok || len(got) != 1 || got[0].TransactionID != 1 {
		t.Fatalf("expected cache hit for same character, got ok=%v txns=%v", ok, got)
	}

	if _, ok := srv.getWalletTxnCache(2002); ok {
		t.Fatalf("expected cache miss for different character")
	}

	srv.clearWalletTxnCache()
	if _, ok := srv.getWalletTxnCache(1001); ok {
		t.Fatalf("expected cache miss after clear")
	}
}

func TestWalletTxnCache_ExpiresByTTL(t *testing.T) {
	srv := &Server{}
	srv.setWalletTxnCache(1001, []esi.WalletTransaction{{TransactionID: 42}})

	// Simulate stale cache entry.
	srv.txnCacheMu.Lock()
	srv.txnCacheTime = time.Now().Add(-walletTxnCacheTTL - time.Second)
	srv.txnCacheMu.Unlock()

	if _, ok := srv.getWalletTxnCache(1001); ok {
		t.Fatalf("expected cache miss for stale entry")
	}
}

func TestHandleAuthPortfolio_AllowsEmptyTransactions(t *testing.T) {
	store := newSessionStoreForAPITest(t)
	userID := "u-empty-portfolio"
	characterID := int64(1001)

	err := store.SaveAndActivateForUser(userID, &auth.Session{
		CharacterID:   characterID,
		CharacterName: "Empty Trader",
		AccessToken:   "cached-token",
		RefreshToken:  "cached-refresh",
		ExpiresAt:     time.Now().Add(15 * time.Minute),
	})
	if err != nil {
		t.Fatalf("SaveAndActivateForUser: %v", err)
	}

	srv := NewServer(config.Default(), &esi.Client{}, nil, nil, store)
	srv.setWalletTxnCache(characterID, []esi.WalletTransaction{})

	req := httptest.NewRequest(http.MethodGet, "/api/auth/portfolio?character_id=1001", nil)
	addSignedUserCookie(req, srv, userID)
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/auth/portfolio status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var out struct {
		DailyPnL []any `json:"daily_pnl"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.DailyPnL == nil || len(out.DailyPnL) != 0 {
		t.Fatalf("daily_pnl = %#v, want empty array", out.DailyPnL)
	}
}

func TestHandleAuthPortfolio_UsesArchiveWhenLiveTransactionsFail(t *testing.T) {
	database := openAPITestDB(t)
	userID := "u-archived-portfolio"
	characterID := int64(1001)
	store := auth.NewSessionStore(database.SqlDB())
	setupAPITestVault(t, store, userID)
	if err := store.SaveAndActivateForUser(userID, &auth.Session{
		CharacterID:   characterID,
		CharacterName: "Archived Trader",
		AccessToken:   "expired-token",
		RefreshToken:  "expired-refresh",
		ExpiresAt:     time.Now().Add(-15 * time.Minute),
	}); err != nil {
		t.Fatalf("SaveAndActivateForUser: %v", err)
	}

	txns := []esi.WalletTransaction{
		{
			TransactionID: 1,
			Date:          time.Now().AddDate(0, 0, -2).UTC().Format(time.RFC3339),
			TypeID:        34,
			LocationID:    60003760,
			UnitPrice:     100,
			Quantity:      2,
			IsBuy:         true,
		},
		{
			TransactionID: 2,
			Date:          time.Now().AddDate(0, 0, -1).UTC().Format(time.RFC3339),
			TypeID:        34,
			LocationID:    60003760,
			UnitPrice:     150,
			Quantity:      1,
			IsBuy:         false,
		},
	}
	if _, err := database.UpsertWalletTransactionsForUser(userID, characterID, txns); err != nil {
		t.Fatalf("UpsertWalletTransactionsForUser: %v", err)
	}

	srv := NewServer(config.Default(), &esi.Client{}, database, nil, store)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/portfolio?character_id=1001", nil)
	addSignedUserCookie(req, srv, userID)
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/auth/portfolio status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Ledger []any `json:"ledger"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(out.Ledger) == 0 {
		t.Fatalf("expected archived transactions to produce portfolio ledger, got none")
	}
}

func TestEnsureRequestUserID_SignedCookieRoundTrip(t *testing.T) {
	srv := NewServer(config.Default(), &esi.Client{}, nil, nil, nil)

	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec1 := httptest.NewRecorder()
	userID1 := srv.ensureRequestUserID(rec1, req1)
	if !isValidUserID(userID1) {
		t.Fatalf("ensureRequestUserID returned invalid user id: %q", userID1)
	}

	cookies := rec1.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected Set-Cookie on first request")
	}
	cookie := cookies[0]
	if cookie.Name != userIDCookieName {
		t.Fatalf("cookie name = %q, want %q", cookie.Name, userIDCookieName)
	}
	if parsed, ok := srv.parseSignedUserIDCookieValue(cookie.Value); !ok || parsed != userID1 {
		t.Fatalf("cookie value is not a valid signed user id: value=%q parsed=%q ok=%v", cookie.Value, parsed, ok)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.AddCookie(cookie)
	rec2 := httptest.NewRecorder()
	userID2 := srv.ensureRequestUserID(rec2, req2)
	if userID2 != userID1 {
		t.Fatalf("user id mismatch on valid signed cookie: got %q, want %q", userID2, userID1)
	}
	if len(rec2.Result().Cookies()) != 0 {
		t.Fatalf("did not expect Set-Cookie for valid signed cookie, got %d", len(rec2.Result().Cookies()))
	}
}

func TestEnsureRequestUserID_RotatesTamperedCookie(t *testing.T) {
	srv := NewServer(config.Default(), &esi.Client{}, nil, nil, nil)

	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec1 := httptest.NewRecorder()
	userID1 := srv.ensureRequestUserID(rec1, req1)
	origCookies := rec1.Result().Cookies()
	if len(origCookies) == 0 {
		t.Fatal("expected Set-Cookie on first request")
	}
	original := origCookies[0]

	tampered := *original
	tampered.Value = userID1 + ".tampered-signature"

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.AddCookie(&tampered)
	rec2 := httptest.NewRecorder()
	userID2 := srv.ensureRequestUserID(rec2, req2)
	if userID2 == "" {
		t.Fatal("expected non-empty user id after tampered cookie")
	}

	newCookies := rec2.Result().Cookies()
	if len(newCookies) == 0 {
		t.Fatal("expected Set-Cookie after tampered cookie")
	}
	if parsed, ok := srv.parseSignedUserIDCookieValue(newCookies[0].Value); !ok || parsed != userID2 {
		t.Fatalf("new cookie is not a valid signed user id: value=%q parsed=%q ok=%v", newCookies[0].Value, parsed, ok)
	}
}

func TestEnsureRequestUserID_IgnoresUnsignedHeaderInWebFlavor(t *testing.T) {
	srv := NewServer(config.Default(), &esi.Client{}, nil, nil, nil)
	srv.SetAppFlavor("web")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(userIDHeaderName, "attacker-supplied-user")
	rec := httptest.NewRecorder()

	got := srv.ensureRequestUserID(rec, req)
	if got == "attacker-supplied-user" {
		t.Fatalf("web flavor accepted unsigned %s header", userIDHeaderName)
	}
	if !isValidUserID(got) {
		t.Fatalf("generated user id invalid: %q", got)
	}
}

func TestEnsureRequestUserID_AllowsHeaderOnlyInDesktopFlavor(t *testing.T) {
	srv := NewServer(config.Default(), &esi.Client{}, nil, nil, nil)
	srv.SetAppFlavor("desktop")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(userIDHeaderName, "eveflipper_desktop")
	rec := httptest.NewRecorder()

	got := srv.ensureRequestUserID(rec, req)
	if got != "eveflipper_desktop" {
		t.Fatalf("desktop flavor user id = %q, want eveflipper_desktop", got)
	}
}

func TestCORSOriginIsPortAware(t *testing.T) {
	if !isAllowedCORSOrigin("http://127.0.0.1:5173", "127.0.0.1:13370") {
		t.Fatalf("expected Vite dev frontend origin to be allowed")
	}
	if !isAllowedCORSOrigin("http://localhost:1420", "127.0.0.1:13370") {
		t.Fatalf("expected Wails/dev frontend origin to be allowed")
	}
	if isAllowedCORSOrigin("http://127.0.0.1:9999", "127.0.0.1:13370") {
		t.Fatalf("unexpected arbitrary loopback port allowed")
	}
	if isAllowedCORSOrigin("http://evil.example", "127.0.0.1:13370") {
		t.Fatalf("unexpected external origin allowed")
	}
}

func TestOriginGuardBlocksStateChangingBadOrigin(t *testing.T) {
	srv := NewServer(config.Default(), &esi.Client{}, nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/security/vault/setup", strings.NewReader(`{"mode":"standard"}`))
	req.Host = "127.0.0.1:13370"
	req.Header.Set("Origin", "http://127.0.0.1:9999")
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
}

func TestDesktopOriginGuardAllowsWailsOriginsOnlyInDesktopFlavor(t *testing.T) {
	origins := []string{"http://wails.localhost", "wails://wails"}

	for _, origin := range origins {
		t.Run("desktop "+origin, func(t *testing.T) {
			srv := NewServer(config.Default(), &esi.Client{}, nil, nil, nil)
			srv.SetAppFlavor("desktop")
			handler := srv.originGuardMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))

			req := httptest.NewRequest(http.MethodPost, "/api/security/vault/setup", strings.NewReader(`{"mode":"standard"}`))
			req.Host = "127.0.0.1:13370"
			req.Header.Set("Origin", origin)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusNoContent {
				t.Fatalf("desktop origin guard status = %d, want 204; body=%s", rec.Code, rec.Body.String())
			}
		})

		t.Run("web "+origin, func(t *testing.T) {
			srv := NewServer(config.Default(), &esi.Client{}, nil, nil, nil)
			srv.SetAppFlavor("web")
			handler := srv.originGuardMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))

			req := httptest.NewRequest(http.MethodPost, "/api/security/vault/setup", strings.NewReader(`{"mode":"standard"}`))
			req.Host = "127.0.0.1:13370"
			req.Header.Set("Origin", origin)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("web origin guard status = %d, want 403; body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestDesktopCORSMiddlewareAllowsWailsOrigin(t *testing.T) {
	srv := NewServer(config.Default(), &esi.Client{}, nil, nil, nil)
	srv.SetAppFlavor("desktop")
	handler := srv.corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/security/vault/setup", nil)
	req.Host = "127.0.0.1:13370"
	req.Header.Set("Origin", "http://wails.localhost")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://wails.localhost" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want Wails origin", got)
	}
}

func TestDesktopVaultSetupAcceptsWailsOrigin(t *testing.T) {
	store := newVaultSessionStoreForAPITest(t)
	srv := NewServer(config.Default(), &esi.Client{}, nil, nil, store)
	srv.SetAppFlavor("desktop")

	req := httptest.NewRequest(http.MethodPost, "/api/security/vault/setup", strings.NewReader(`{"mode":"standard"}`))
	req.Host = "127.0.0.1:13370"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://wails.localhost")
	req.Header.Set(userIDHeaderName, "eveflipper_desktop")
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://wails.localhost" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want Wails origin", got)
	}
}

func TestSecurityVaultPrivatePassphraseAPIFlow(t *testing.T) {
	store := newVaultSessionStoreForAPITest(t)
	srv := NewServer(config.Default(), &esi.Client{}, nil, nil, store)
	userID := "api-private-vault-user"
	passphrase := "correct horse battery staple"

	doJSON := func(method, target, body string) (int, map[string]interface{}) {
		t.Helper()
		req := httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		addSignedUserCookie(req, srv, userID)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		var payload map[string]interface{}
		if strings.TrimSpace(rec.Body.String()) != "" {
			if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
				t.Fatalf("decode %s %s response %q: %v", method, target, rec.Body.String(), err)
			}
		}
		return rec.Code, payload
	}

	code, payload := doJSON(http.MethodPost, "/api/security/vault/setup", `{"mode":"private","passphrase":"`+passphrase+`"}`)
	if code != http.StatusOK {
		t.Fatalf("private setup status=%d payload=%v", code, payload)
	}
	if payload["configured"] != true || payload["mode"] != "private" || payload["locked"] != false {
		t.Fatalf("private setup payload=%v", payload)
	}
	if payload["zero_knowledge_local_storage"] != true || payload["field_encryption_active"] != true {
		t.Fatalf("private setup flags payload=%v", payload)
	}

	code, payload = doJSON(http.MethodPost, "/api/security/vault/lock", `{}`)
	if code != http.StatusOK {
		t.Fatalf("private lock status=%d payload=%v", code, payload)
	}
	if payload["locked"] != true || payload["private_unlock_required"] != true || payload["field_encryption_active"] != false {
		t.Fatalf("private lock payload=%v", payload)
	}

	code, _ = doJSON(http.MethodPost, "/api/security/vault/unlock", `{"passphrase":"wrong passphrase"}`)
	if code != http.StatusUnauthorized {
		t.Fatalf("wrong passphrase unlock status=%d, want 401", code)
	}

	code, payload = doJSON(http.MethodPost, "/api/security/vault/unlock", `{"passphrase":"`+passphrase+`"}`)
	if code != http.StatusOK {
		t.Fatalf("correct passphrase unlock status=%d payload=%v", code, payload)
	}
	if payload["locked"] != false || payload["private_unlock_required"] != false || payload["field_encryption_active"] != true {
		t.Fatalf("private unlock payload=%v", payload)
	}
}

func TestAuthRevisionBumpAndStatusPayload(t *testing.T) {
	srv := NewServer(config.Default(), &esi.Client{}, nil, nil, nil)

	if got := srv.authRevisionForUser("u1"); got != 0 {
		t.Fatalf("initial auth revision = %d, want 0", got)
	}
	if got := srv.bumpAuthRevision("u1"); got != 1 {
		t.Fatalf("auth revision after first bump = %d, want 1", got)
	}
	if got := srv.bumpAuthRevision("u1"); got != 2 {
		t.Fatalf("auth revision after second bump = %d, want 2", got)
	}
	if got := srv.authRevisionForUser("u1"); got != 2 {
		t.Fatalf("stored auth revision = %d, want 2", got)
	}

	payload := srv.authStatusPayload("u1")
	revision, ok := payload["auth_revision"].(int64)
	if !ok {
		t.Fatalf("auth_revision type = %T, want int64", payload["auth_revision"])
	}
	if revision != 2 {
		t.Fatalf("payload auth_revision = %d, want 2", revision)
	}
}
