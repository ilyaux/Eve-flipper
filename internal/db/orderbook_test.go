package db

import (
	"testing"
	"time"

	"eve-flipper/internal/esi"
)

func TestRecordMarketOrderSnapshotAggregatesAndDedupes(t *testing.T) {
	d := openTestDB(t)
	defer d.Close()

	snapshot := esi.MarketOrderSnapshot{
		RegionID:   10000002,
		OrderType:  "all",
		Source:     "region_type",
		TypeID:     34,
		CapturedAt: time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
		ExpiresAt:  time.Date(2026, 4, 30, 12, 5, 0, 0, time.UTC),
		Orders: []esi.MarketOrder{
			{OrderID: 1, TypeID: 34, LocationID: 60003760, SystemID: 30000142, Price: 5.0, VolumeRemain: 100, IsBuyOrder: false, RegionID: 10000002},
			{OrderID: 2, TypeID: 34, LocationID: 60003760, SystemID: 30000142, Price: 5.0, VolumeRemain: 50, IsBuyOrder: false, RegionID: 10000002},
			{OrderID: 3, TypeID: 34, LocationID: 60003760, SystemID: 30000142, Price: 4.8, VolumeRemain: 70, IsBuyOrder: true, RegionID: 10000002},
		},
	}
	if err := d.RecordMarketOrderSnapshot(snapshot); err != nil {
		t.Fatalf("record snapshot: %v", err)
	}

	snaps, err := d.ListOrderBookSnapshots(OrderBookSnapshotFilter{TypeID: 34, Limit: 10})
	if err != nil {
		t.Fatalf("list snapshots: %v", err)
	}
	if len(snaps) != 1 {
		t.Fatalf("snapshots len=%d, want 1", len(snaps))
	}
	if snaps[0].OrderCount != 3 || snaps[0].LevelCount != 2 || snaps[0].UniqueLocationCount != 1 {
		t.Fatalf("snapshot counts = orders %d levels %d locations %d", snaps[0].OrderCount, snaps[0].LevelCount, snaps[0].UniqueLocationCount)
	}

	levels, err := d.GetOrderBookLevels(snaps[0].ID, OrderBookLevelFilter{TypeID: 34, Side: "sell"})
	if err != nil {
		t.Fatalf("get sell levels: %v", err)
	}
	if len(levels) != 1 {
		t.Fatalf("sell levels len=%d, want 1", len(levels))
	}
	if levels[0].VolumeRemain != 150 || levels[0].OrderCount != 2 {
		t.Fatalf("aggregated sell level = volume %d orders %d", levels[0].VolumeRemain, levels[0].OrderCount)
	}

	snapshot.CapturedAt = snapshot.CapturedAt.Add(5 * time.Minute)
	if err := d.RecordMarketOrderSnapshot(snapshot); err != nil {
		t.Fatalf("record duplicate snapshot: %v", err)
	}
	snaps, err = d.ListOrderBookSnapshots(OrderBookSnapshotFilter{TypeID: 34, Limit: 10})
	if err != nil {
		t.Fatalf("list after duplicate: %v", err)
	}
	if len(snaps) != 1 {
		t.Fatalf("duplicate should update last_seen, got %d snapshots", len(snaps))
	}
	if snaps[0].CapturedAt == snaps[0].LastSeenAt {
		t.Fatalf("duplicate did not update last_seen_at")
	}
}

func TestRecordMarketOrderSnapshotStoresChangedBook(t *testing.T) {
	d := openTestDB(t)
	defer d.Close()

	base := esi.MarketOrderSnapshot{
		RegionID:   10000002,
		OrderType:  "sell",
		Source:     "region",
		CapturedAt: time.Now().UTC(),
		Orders: []esi.MarketOrder{
			{OrderID: 1, TypeID: 35, LocationID: 60003760, SystemID: 30000142, Price: 10.0, VolumeRemain: 10, IsBuyOrder: false},
		},
	}
	if err := d.RecordMarketOrderSnapshot(base); err != nil {
		t.Fatalf("record base: %v", err)
	}
	base.Orders[0].VolumeRemain = 11
	base.CapturedAt = base.CapturedAt.Add(time.Minute)
	if err := d.RecordMarketOrderSnapshot(base); err != nil {
		t.Fatalf("record changed: %v", err)
	}

	snaps, err := d.ListOrderBookSnapshots(OrderBookSnapshotFilter{RegionID: 10000002, OrderType: "sell", Limit: 10})
	if err != nil {
		t.Fatalf("list changed snapshots: %v", err)
	}
	if len(snaps) != 2 {
		t.Fatalf("changed book snapshots len=%d, want 2", len(snaps))
	}
}

func TestListOrderBookReplayBooksFindsRegionWideSnapshotsByLevelType(t *testing.T) {
	d := openTestDB(t)
	defer d.Close()

	capturedAt := time.Now().UTC().Add(-time.Minute)
	if err := d.RecordMarketOrderSnapshot(esi.MarketOrderSnapshot{
		RegionID:   10000002,
		OrderType:  "sell",
		Source:     "region",
		CapturedAt: capturedAt,
		Orders: []esi.MarketOrder{
			{OrderID: 1, TypeID: 35, LocationID: 60003760, SystemID: 30000142, Price: 10.0, VolumeRemain: 10, IsBuyOrder: false},
			{OrderID: 2, TypeID: 36, LocationID: 60003760, SystemID: 30000142, Price: 20.0, VolumeRemain: 10, IsBuyOrder: false},
		},
	}); err != nil {
		t.Fatalf("record region-wide snapshot: %v", err)
	}

	snaps, err := d.ListOrderBookSnapshots(OrderBookSnapshotFilter{TypeID: 35, Limit: 10})
	if err != nil {
		t.Fatalf("list snapshots by level type: %v", err)
	}
	if len(snaps) != 1 {
		t.Fatalf("snapshots len=%d, want 1", len(snaps))
	}
	if snaps[0].TypeID != 0 {
		t.Fatalf("region-wide snapshot type_id=%d, want 0", snaps[0].TypeID)
	}

	books, err := d.ListOrderBookReplayBooks(OrderBookReplayFilter{
		RegionID:       10000002,
		TypeID:         35,
		LocationID:     60003760,
		Side:           "sell",
		FromCapturedAt: capturedAt.Add(-time.Minute),
		ToCapturedAt:   capturedAt.Add(time.Minute),
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("list replay books: %v", err)
	}
	if len(books) != 1 || len(books[0].Levels) != 1 {
		t.Fatalf("books = %#v, want one book with one level", books)
	}
	if books[0].Levels[0].TypeID != 35 || books[0].Levels[0].Price != 10 {
		t.Fatalf("level = %#v, want type 35 price 10", books[0].Levels[0])
	}
}

func TestOrderBookStatsAndCleanup(t *testing.T) {
	d := openTestDB(t)
	defer d.Close()

	now := time.Now().UTC()
	if err := d.RecordMarketOrderSnapshot(esi.MarketOrderSnapshot{
		RegionID:   10000002,
		OrderType:  "sell",
		Source:     "region",
		CapturedAt: now.AddDate(0, 0, -120),
		Orders: []esi.MarketOrder{
			{OrderID: 1, TypeID: 34, LocationID: 60003760, SystemID: 30000142, Price: 5.0, VolumeRemain: 100, IsBuyOrder: false},
		},
	}); err != nil {
		t.Fatalf("record old snapshot: %v", err)
	}
	if err := d.RecordMarketOrderSnapshot(esi.MarketOrderSnapshot{
		RegionID:   10000002,
		OrderType:  "buy",
		Source:     "region",
		CapturedAt: now.Add(-time.Hour),
		Orders: []esi.MarketOrder{
			{OrderID: 2, TypeID: 35, LocationID: 60008494, SystemID: 30000144, Price: 8.0, VolumeRemain: 50, IsBuyOrder: true},
		},
	}); err != nil {
		t.Fatalf("record new snapshot: %v", err)
	}

	stats, err := d.GetOrderBookStats(5)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.SnapshotCount != 2 || stats.LevelCount != 2 || stats.UniqueTypeCount != 2 || stats.UniqueLocationCount != 2 {
		t.Fatalf("stats counts = %#v", stats)
	}
	if stats.TotalVolumeRemain != 150 || stats.ApproxBytes <= 0 || len(stats.TopTypes) != 2 || len(stats.TopLocations) != 2 {
		t.Fatalf("stats detail = %#v", stats)
	}

	if err := d.RecordMarketOrderSnapshot(esi.MarketOrderSnapshot{
		RegionID:   10000002,
		OrderType:  "sell",
		Source:     "region",
		CapturedAt: now.Add(-30 * time.Minute),
		Orders: []esi.MarketOrder{
			{OrderID: 3, TypeID: 36, LocationID: 60008494, SystemID: 30000144, Price: 9.0, VolumeRemain: 25, IsBuyOrder: false},
		},
	}); err != nil {
		t.Fatalf("record cache invalidating snapshot: %v", err)
	}
	stats, err = d.GetOrderBookStats(5)
	if err != nil {
		t.Fatalf("stats after cache invalidating snapshot: %v", err)
	}
	if stats.SnapshotCount != 3 || stats.LevelCount != 3 || stats.TotalVolumeRemain != 175 {
		t.Fatalf("stats after cache invalidating snapshot = %#v", stats)
	}

	preview, err := d.CleanupOrderBookSnapshots(30, true, false)
	if err != nil {
		t.Fatalf("preview cleanup: %v", err)
	}
	if !preview.DryRun || preview.SnapshotsDeleted != 1 || preview.LevelsDeleted != 1 {
		t.Fatalf("preview = %#v, want one old snapshot and level", preview)
	}
	snaps, err := d.ListOrderBookSnapshots(OrderBookSnapshotFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list after preview: %v", err)
	}
	if len(snaps) != 3 {
		t.Fatalf("dry-run deleted snapshots, got %d", len(snaps))
	}

	removed, err := d.CleanupOrderBookSnapshots(30, false, false)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if removed.DryRun || removed.SnapshotsDeleted != 1 || removed.LevelsDeleted != 1 {
		t.Fatalf("cleanup = %#v, want one removed snapshot and level", removed)
	}
	stats, err = d.GetOrderBookStats(5)
	if err != nil {
		t.Fatalf("stats after cleanup: %v", err)
	}
	if stats.SnapshotCount != 2 || stats.LevelCount != 2 || stats.UniqueTypeCount != 2 || stats.TopTypes[0].TypeID != 35 {
		t.Fatalf("stats after cleanup = %#v", stats)
	}
}

func TestCleanupOrderBookSnapshotsBatch(t *testing.T) {
	d := openTestDB(t)
	defer d.Close()

	old := time.Now().UTC().AddDate(0, 0, -60)
	for i := 0; i < 3; i++ {
		if err := d.RecordMarketOrderSnapshot(esi.MarketOrderSnapshot{
			RegionID:   10000002,
			OrderType:  "sell",
			Source:     "region",
			CapturedAt: old.Add(time.Duration(i) * time.Hour),
			Orders: []esi.MarketOrder{
				{OrderID: int64(100 + i), TypeID: int32(35 + i), LocationID: 60008494, SystemID: 30000142, Price: float64(10 + i), VolumeRemain: 10, IsBuyOrder: false},
			},
		}); err != nil {
			t.Fatalf("record old snapshot %d: %v", i, err)
		}
	}
	if err := d.RecordMarketOrderSnapshot(esi.MarketOrderSnapshot{
		RegionID:   10000002,
		OrderType:  "sell",
		Source:     "region",
		CapturedAt: time.Now().UTC(),
		Orders: []esi.MarketOrder{
			{OrderID: 200, TypeID: 40, LocationID: 60008494, SystemID: 30000142, Price: 99, VolumeRemain: 10, IsBuyOrder: false},
		},
	}); err != nil {
		t.Fatalf("record fresh snapshot: %v", err)
	}

	plan, err := d.CleanupOrderBookSnapshotsBatch(30, 2)
	if err != nil {
		t.Fatalf("batch cleanup: %v", err)
	}
	if plan.SnapshotsDeleted != 2 || plan.LevelsDeleted != 2 {
		t.Fatalf("first batch = %#v, want 2 old snapshots and levels", plan)
	}
	stats, err := d.GetOrderBookStats(5)
	if err != nil {
		t.Fatalf("stats after first batch: %v", err)
	}
	if stats.SnapshotCount != 2 || stats.LevelCount != 2 {
		t.Fatalf("stats after first batch = %#v, want 2 remaining", stats)
	}

	plan, err = d.CleanupOrderBookSnapshotsBatch(30, 2)
	if err != nil {
		t.Fatalf("second batch cleanup: %v", err)
	}
	if plan.SnapshotsDeleted != 1 || plan.LevelsDeleted != 1 {
		t.Fatalf("second batch = %#v, want final old snapshot and level", plan)
	}
	stats, err = d.GetOrderBookStats(5)
	if err != nil {
		t.Fatalf("stats after second batch: %v", err)
	}
	if stats.SnapshotCount != 1 || stats.LevelCount != 1 || stats.TopTypes[0].TypeID != 40 {
		t.Fatalf("stats after second batch = %#v, want only fresh snapshot", stats)
	}
}

func TestCleanupOrderBookSnapshotsBatches(t *testing.T) {
	d := openTestDB(t)
	defer d.Close()

	old := time.Now().UTC().AddDate(0, 0, -60)
	for i := 0; i < 5; i++ {
		if err := d.RecordMarketOrderSnapshot(esi.MarketOrderSnapshot{
			RegionID:   10000002,
			OrderType:  "sell",
			Source:     "region",
			CapturedAt: old.Add(time.Duration(i) * time.Hour),
			Orders: []esi.MarketOrder{
				{OrderID: int64(300 + i), TypeID: int32(50 + i), LocationID: 60008494, SystemID: 30000142, Price: float64(10 + i), VolumeRemain: 10, IsBuyOrder: false},
			},
		}); err != nil {
			t.Fatalf("record old snapshot %d: %v", i, err)
		}
	}
	if err := d.RecordMarketOrderSnapshot(esi.MarketOrderSnapshot{
		RegionID:   10000002,
		OrderType:  "sell",
		Source:     "region",
		CapturedAt: time.Now().UTC(),
		Orders: []esi.MarketOrder{
			{OrderID: 400, TypeID: 60, LocationID: 60008494, SystemID: 30000142, Price: 99, VolumeRemain: 10, IsBuyOrder: false},
		},
	}); err != nil {
		t.Fatalf("record fresh snapshot: %v", err)
	}

	plan, err := d.CleanupOrderBookSnapshotsBatches(30, 2, 5*time.Second)
	if err != nil {
		t.Fatalf("cleanup batches: %v", err)
	}
	if plan.SnapshotsDeleted != 5 || plan.LevelsDeleted != 5 {
		t.Fatalf("cleanup batches = %#v, want all 5 old snapshots and levels", plan)
	}
	stats, err := d.GetOrderBookStats(5)
	if err != nil {
		t.Fatalf("stats after cleanup batches: %v", err)
	}
	if stats.SnapshotCount != 1 || stats.LevelCount != 1 || stats.TopTypes[0].TypeID != 60 {
		t.Fatalf("stats after cleanup batches = %#v, want only fresh snapshot", stats)
	}
}
