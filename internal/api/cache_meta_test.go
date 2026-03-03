package api

import (
	"testing"
	"time"

	"eve-flipper/internal/esi"
)

func TestStationCacheMetaFromWindows_Empty(t *testing.T) {
	meta := stationCacheMetaFromWindows(3)
	if meta.Regions != 3 {
		t.Fatalf("expected regions=3, got %d", meta.Regions)
	}
	if meta.Entries != 0 {
		t.Fatalf("expected entries=0, got %d", meta.Entries)
	}
	if meta.Stale {
		t.Fatalf("expected stale=false")
	}
	if meta.NextExpiryAt != "" {
		t.Fatalf("expected empty next_expiry_at, got %q", meta.NextExpiryAt)
	}
}

func TestStationCacheMetaFromWindows_CombinesByMinExpiry(t *testing.T) {
	now := time.Now()
	minExpiry := now.Add(2 * time.Minute)
	maxExpiry := now.Add(7 * time.Minute)
	lastRefresh := now.Add(-30 * time.Second)

	meta := stationCacheMetaFromWindows(
		5,
		esi.OrderCacheWindow{
			Entries:       10,
			NextExpiryAt:  maxExpiry,
			LastRefreshAt: now.Add(-2 * time.Minute),
		},
		esi.OrderCacheWindow{
			Entries:       7,
			NextExpiryAt:  minExpiry,
			LastRefreshAt: lastRefresh,
		},
	)

	if meta.Regions != 5 {
		t.Fatalf("expected regions=5, got %d", meta.Regions)
	}
	if meta.Entries != 17 {
		t.Fatalf("expected entries=17, got %d", meta.Entries)
	}
	if meta.Stale {
		t.Fatalf("expected stale=false")
	}
	if meta.CurrentRevision != minExpiry.Unix() {
		t.Fatalf("expected current_revision=%d, got %d", minExpiry.Unix(), meta.CurrentRevision)
	}
	if meta.MinTTLSec <= 0 {
		t.Fatalf("expected min_ttl_sec > 0, got %d", meta.MinTTLSec)
	}
	if meta.MaxTTLSec <= meta.MinTTLSec {
		t.Fatalf("expected max_ttl_sec > min_ttl_sec, got min=%d max=%d", meta.MinTTLSec, meta.MaxTTLSec)
	}
	if meta.LastRefreshAt == "" {
		t.Fatalf("expected last_refresh_at to be set")
	}
	if meta.NextExpiryAt == "" {
		t.Fatalf("expected next_expiry_at to be set")
	}
}

func TestStationCacheMetaFromWindows_StaleWhenMinExpiryInPast(t *testing.T) {
	now := time.Now()
	meta := stationCacheMetaFromWindows(
		2,
		esi.OrderCacheWindow{
			Entries:       4,
			NextExpiryAt:  now.Add(-15 * time.Second),
			LastRefreshAt: now.Add(-2 * time.Minute),
		},
		esi.OrderCacheWindow{
			Entries:       9,
			NextExpiryAt:  now.Add(3 * time.Minute),
			LastRefreshAt: now.Add(-30 * time.Second),
		},
	)

	if !meta.Stale {
		t.Fatalf("expected stale=true")
	}
	if meta.MinTTLSec != 0 {
		t.Fatalf("expected min_ttl_sec=0 for stale window, got %d", meta.MinTTLSec)
	}
	if meta.MaxTTLSec <= 0 {
		t.Fatalf("expected max_ttl_sec > 0, got %d", meta.MaxTTLSec)
	}
}
