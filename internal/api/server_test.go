package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"eve-flipper/internal/config"
	"eve-flipper/internal/esi"
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
