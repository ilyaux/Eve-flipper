package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"eve-flipper/internal/config"
	"eve-flipper/internal/engine"
	"eve-flipper/internal/esi"
)

func TestFilterExecutionPlanOrders_StructureScopedLocation(t *testing.T) {
	const (
		typeID      = int32(40520)
		structureID = int64(1_024_000_000_001)
		otherLocID  = int64(1_024_000_000_002)
	)

	orders := []esi.MarketOrder{
		{TypeID: typeID, LocationID: structureID, SystemID: 30000142, Price: 700},
		{TypeID: typeID, LocationID: otherLocID, SystemID: 30000142, Price: 710},
		{TypeID: 34, LocationID: structureID, SystemID: 30000142, Price: 1},
	}

	got := filterExecutionPlanOrders(orders, typeID, 0, structureID)

	if len(got) != 1 {
		t.Fatalf("filtered length = %d, want 1", len(got))
	}
	if got[0].LocationID != structureID {
		t.Fatalf("location_id = %d, want %d", got[0].LocationID, structureID)
	}
}

func TestFilterExecutionPlanOrders_SystemScopeUsesRequestedSystem(t *testing.T) {
	const typeID = int32(40520)

	orders := []esi.MarketOrder{
		{TypeID: typeID, SystemID: 30000142, LocationID: 60003760, Price: 700},
		{TypeID: typeID, SystemID: 30002187, LocationID: 60008494, Price: 710},
		{TypeID: 34, SystemID: 30000142, LocationID: 60003760, Price: 1},
	}

	got := filterExecutionPlanOrders(orders, typeID, 30000142, 0)

	if len(got) != 1 {
		t.Fatalf("filtered length = %d, want 1", len(got))
	}
	if got[0].SystemID != 30000142 {
		t.Fatalf("system_id = %d, want 30000142", got[0].SystemID)
	}
}

func TestFilterExecutionPlanOrders_RegionScopeIncludesAllSystems(t *testing.T) {
	const typeID = int32(40520)

	orders := []esi.MarketOrder{
		{TypeID: typeID, SystemID: 30000142, LocationID: 60003760, Price: 700},
		{TypeID: typeID, SystemID: 30002187, LocationID: 60008494, Price: 710},
		{TypeID: 34, SystemID: 30000142, LocationID: 60003760, Price: 1},
	}

	got := filterExecutionPlanOrders(orders, typeID, 0, 0)

	if len(got) != 2 {
		t.Fatalf("filtered length = %d, want 2", len(got))
	}
}

func TestHandleExecutionPlanQuoteTrueReturnsCanonicalQuoteFields(t *testing.T) {
	const (
		typeID       = int32(34)
		buyRegionID  = int32(10000002)
		sellRegionID = int32(10000043)
		buySystemID  = int32(30000142)
		sellSystemID = int32(30002187)
		buyLocation  = int64(60003760)
		sellLocation = int64(60008494)
	)

	now := time.Now()
	ordersByFetch := map[string][]esi.MarketOrder{
		fmt.Sprintf("%d:%s", buyRegionID, "sell"): {
			{TypeID: typeID, SystemID: buySystemID, LocationID: buyLocation, Price: 100, VolumeRemain: 100},
			{TypeID: typeID, SystemID: buySystemID, LocationID: buyLocation, Price: 110, VolumeRemain: 100},
		},
		fmt.Sprintf("%d:%s", sellRegionID, "buy"): {
			{TypeID: typeID, SystemID: sellSystemID, LocationID: sellLocation, Price: 160, VolumeRemain: 100, IsBuyOrder: true},
			{TypeID: typeID, SystemID: sellSystemID, LocationID: sellLocation, Price: 150, VolumeRemain: 100, IsBuyOrder: true},
		},
	}
	windowsByFetch := map[string]esi.OrderCacheWindow{
		fmt.Sprintf("%d:%s", buyRegionID, "sell"): {
			LastRefreshAt: now.Add(-2 * time.Minute),
			NextExpiryAt:  now.Add(3 * time.Minute),
			MinTTLSeconds: 180,
			Entries:       1,
		},
		fmt.Sprintf("%d:%s", sellRegionID, "buy"): {
			LastRefreshAt: now.Add(-3 * time.Minute),
			NextExpiryAt:  now.Add(2 * time.Minute),
			MinTTLSeconds: 120,
			Entries:       1,
		},
	}

	origFetchRegionOrders := executionFetchRegionOrders
	origFetchStructureOrders := executionFetchStructureOrders
	origOrderCacheWindow := executionOrderCacheWindow
	executionFetchRegionOrders = func(_ *esi.Client, regionID int32, orderType string) ([]esi.MarketOrder, error) {
		key := fmt.Sprintf("%d:%s", regionID, orderType)
		orders, ok := ordersByFetch[key]
		if !ok {
			return nil, fmt.Errorf("unexpected order fetch %s", key)
		}
		return append([]esi.MarketOrder(nil), orders...), nil
	}
	executionFetchStructureOrders = func(_ context.Context, _ *esi.Client, structureID int64, _ string) ([]esi.MarketOrder, error) {
		return nil, fmt.Errorf("unexpected structure order fetch %d", structureID)
	}
	executionOrderCacheWindow = func(_ *esi.Client, regionIDs []int32, orderType string) esi.OrderCacheWindow {
		if len(regionIDs) != 1 {
			return esi.OrderCacheWindow{Regions: len(regionIDs)}
		}
		key := fmt.Sprintf("%d:%s", regionIDs[0], orderType)
		window := windowsByFetch[key]
		window.Regions = len(regionIDs)
		return window
	}
	t.Cleanup(func() {
		executionFetchRegionOrders = origFetchRegionOrders
		executionFetchStructureOrders = origFetchStructureOrders
		executionOrderCacheWindow = origOrderCacheWindow
	})

	srv := NewServer(config.Default(), &esi.Client{}, nil, nil, nil)
	body := []byte(`{
		"type_id": 34,
		"region_id": 10000002,
		"system_id": 30000142,
		"location_id": 60003760,
		"quantity": 150,
		"is_buy": true,
		"buy_region_id": 10000002,
		"buy_system_id": 30000142,
		"buy_location_id": 60003760,
		"sell_region_id": 10000043,
		"sell_system_id": 30002187,
		"sell_location_id": 60008494,
		"packaged_volume_m3": 2,
		"shipping_cost_per_m3_jump": 3,
		"shipping_jumps": 4,
		"broker_fee_percent": 1,
		"sales_tax_percent": 2
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/execution/plan?quote=true", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got engine.ExecutionPlanResult
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Quote == nil {
		t.Fatalf("quote missing from execution plan response")
	}
	quote := got.Quote
	if quote.Decision != "SAFE" {
		t.Fatalf("quote decision = %s, want SAFE (%v)", quote.Decision, quote.Warnings)
	}
	if quote.FillQty != 150 || quote.Buy.FilledQty != 150 || quote.Sell.FilledQty != 150 {
		t.Fatalf("filled qty = top %d buy %d sell %d, want 150", quote.FillQty, quote.Buy.FilledQty, quote.Sell.FilledQty)
	}
	if quote.PackagedVolumeM3 != 2 || quote.FilledVolumeM3 != 300 {
		t.Fatalf("volume fields = packaged %v filled %v, want 2/300", quote.PackagedVolumeM3, quote.FilledVolumeM3)
	}
	if quote.ShippingCost != 3600 || quote.ShippingJumps != 4 || quote.ShippingCostPerM3Jump != 3 {
		t.Fatalf("shipping fields = cost %v jumps %d rate %v, want 3600/4/3", quote.ShippingCost, quote.ShippingJumps, quote.ShippingCostPerM3Jump)
	}
	if quote.Cache == nil {
		t.Fatalf("cache metadata missing")
	}
	if quote.Cache.BuyTTLSeconds != 180 || quote.Cache.SellTTLSeconds != 120 {
		t.Fatalf("cache ttl = buy %d sell %d, want 180/120", quote.Cache.BuyTTLSeconds, quote.Cache.SellTTLSeconds)
	}
	if quote.Cache.BuyAgeSeconds < 110 || quote.Cache.BuyAgeSeconds > 130 {
		t.Fatalf("buy cache age = %d, want about 120", quote.Cache.BuyAgeSeconds)
	}
	if quote.Cache.SellAgeSeconds < 170 || quote.Cache.SellAgeSeconds > 190 {
		t.Fatalf("sell cache age = %d, want about 180", quote.Cache.SellAgeSeconds)
	}
}
