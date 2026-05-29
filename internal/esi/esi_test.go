package esi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestMarketOrder_UnmarshalJSON(t *testing.T) {
	raw := `{"order_id":1,"type_id":34,"location_id":60003760,"system_id":30000142,"price":4.5,"volume_remain":100000,"is_buy_order":false}`
	var o MarketOrder
	if err := json.Unmarshal([]byte(raw), &o); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if o.OrderID != 1 || o.TypeID != 34 || o.LocationID != 60003760 || o.SystemID != 30000142 {
		t.Errorf("MarketOrder = %+v", o)
	}
	if o.Price != 4.5 || o.VolumeRemain != 100000 {
		t.Errorf("Price/VolumeRemain = %v/%v", o.Price, o.VolumeRemain)
	}
	if o.IsBuyOrder != false {
		t.Error("IsBuyOrder want false")
	}
}

func TestHistoryEntry_UnmarshalJSON(t *testing.T) {
	raw := `{"date":"2025-01-15","average":100.5,"highest":105,"lowest":98,"volume":50000,"order_count":12}`
	var h HistoryEntry
	if err := json.Unmarshal([]byte(raw), &h); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if h.Date != "2025-01-15" || h.Average != 100.5 || h.Highest != 105 || h.Lowest != 98 {
		t.Errorf("HistoryEntry = %+v", h)
	}
	if h.Volume != 50000 || h.OrderCount != 12 {
		t.Errorf("Volume/OrderCount = %v/%v", h.Volume, h.OrderCount)
	}
}

func TestNewClient_NonNil(t *testing.T) {
	c := NewClient(nil)
	if c == nil {
		t.Fatal("NewClient(nil) returned nil")
	}
}

func TestClassifyStructureNameFailure(t *testing.T) {
	tests := []struct {
		errText string
		reason  string
		minTTL  time.Duration
	}{
		{`ESI 403: {"error":"Forbidden"}`, "inaccessible structure", 23 * time.Hour},
		{`ESI 420: {"error":"Rate limit exceeded"}`, "ESI rate limit", 14 * time.Minute},
		{`ESI 503: unavailable`, "transient ESI failure", time.Minute},
	}
	for _, tt := range tests {
		reason, ttl := classifyStructureNameFailure(errors.New(tt.errText))
		if reason != tt.reason {
			t.Fatalf("reason for %q = %q, want %q", tt.errText, reason, tt.reason)
		}
		if ttl < tt.minTTL {
			t.Fatalf("ttl for %q = %s, want >= %s", tt.errText, ttl, tt.minTTL)
		}
	}
}

func TestStructureNameFailureSuppression(t *testing.T) {
	c := NewClient(nil)
	const structureID int64 = 100000001

	c.rememberStructureNameFailure(structureID, errors.New(`ESI 403: {"error":"Forbidden"}`))
	if !c.structureNameLookupBlocked(structureID) {
		t.Fatal("structure lookup should be blocked after 403")
	}

	otherID := structureID + 1
	if c.structureNameLookupBlocked(otherID) {
		t.Fatal("403 for one structure should not globally block unrelated structures")
	}
}

func TestStructureNameRateLimitCreatesGlobalSuppression(t *testing.T) {
	c := NewClient(nil)
	const structureID int64 = 100000001
	const otherID int64 = 100000002

	c.rememberStructureNameFailure(structureID, errors.New(`ESI 420: {"error":"Rate limit exceeded"}`))
	if !c.structureNameLookupBlocked(structureID) {
		t.Fatal("rate-limited structure should be blocked")
	}
	if !c.structureNameLookupBlocked(otherID) {
		t.Fatal("ESI rate-limit should temporarily block new structure-name lookups globally")
	}
}

func TestStructureNameUsesEVERefBeforeSuppression(t *testing.T) {
	c := NewClient(nil)
	const structureID int64 = 100000001
	const structureName = "Known Player Structure"

	c.everefNames.Store(structureID, structureName)
	c.rememberStructureNameFailure(structureID, errors.New(`ESI 403: {"error":"Forbidden"}`))

	if got := c.StructureName(structureID, ""); got != structureName {
		t.Fatalf("StructureName() = %q, want EVERef name %q", got, structureName)
	}
	if c.structureNameLookupBlocked(structureID) {
		t.Fatal("successful EVERef resolution should clear the negative cache")
	}
}

func TestGetPaginatedDirectWithHeaders_RetriesDecodeError(t *testing.T) {
	var page2Attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "1":
			w.Header().Set("X-Pages", "2")
			_, _ = w.Write([]byte(`[{"order_id":1,"type_id":34,"location_id":60003760,"system_id":30000142,"price":4.5,"volume_remain":10}]`))
		case "2":
			attempt := atomic.AddInt32(&page2Attempts, 1)
			if attempt == 1 {
				_, _ = w.Write([]byte(`{bad json`))
				return
			}
			_, _ = w.Write([]byte(`[{"order_id":2,"type_id":34,"location_id":60008494,"system_id":30002187,"price":5.0,"volume_remain":20}]`))
		default:
			t.Fatalf("unexpected page query: %q", r.URL.RawQuery)
		}
	}))
	defer srv.Close()

	c := NewClient(nil)
	c.http = srv.Client()

	orders, _, _, err := c.getPaginatedDirectWithHeaders(srv.URL+"/orders?datasource=tranquility&order_type=all", 10000002)
	if err != nil {
		t.Fatalf("getPaginatedDirectWithHeaders error: %v", err)
	}
	if len(orders) != 2 {
		t.Fatalf("len(orders) = %d, want 2 after retrying decode error", len(orders))
	}
	if got := atomic.LoadInt32(&page2Attempts); got != 2 {
		t.Fatalf("page2 attempts = %d, want 2", got)
	}
	if orders[1].RegionID != 10000002 {
		t.Fatalf("orders[1].RegionID = %d, want 10000002", orders[1].RegionID)
	}
}
