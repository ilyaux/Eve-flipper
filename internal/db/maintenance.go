package db

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultOrderBookSnapshotRetentionDays = 14
	DefaultOrderBookCleanupBatchSnapshots = 100
	DefaultOrderBookCleanupMaxSeconds     = 20
	DefaultScanHistoryRetentionDays       = 30
	DefaultCacheCleanupInterval           = 6 * time.Hour
)

func (d *DB) CleanupStartupCachesAsync(delay time.Duration) {
	if d == nil || d.sql == nil {
		return
	}
	go func() {
		if delay > 0 {
			time.Sleep(delay)
		}
		for {
			if err := d.sql.Ping(); err != nil {
				log.Printf("[DB] CleanupStartupCaches: skipped, database unavailable: %v", err)
			} else {
				d.CleanupStartupCaches()
			}
			time.Sleep(DefaultCacheCleanupInterval)
		}
	}()
}

// CleanupStartupCaches bounds the largest local cache tables on startup and
// during periodic maintenance for long-running instances.
// It avoids VACUUM because compacting multi-GB SQLite files can block the app
// for a long time; manual cleanup can still request VACUUM from the UI/API.
func (d *DB) CleanupStartupCaches() {
	if d == nil || d.sql == nil {
		return
	}

	d.CleanupOldHistory()

	orderbookDays := retentionDaysFromEnv("EVE_FLIPPER_ORDERBOOK_RETENTION_DAYS", DefaultOrderBookSnapshotRetentionDays)
	if orderbookDays > 0 {
		batchSize := retentionDaysFromEnv("EVE_FLIPPER_ORDERBOOK_CLEANUP_BATCH_SNAPSHOTS", DefaultOrderBookCleanupBatchSnapshots)
		maxDuration := durationSecondsFromEnv("EVE_FLIPPER_ORDERBOOK_CLEANUP_MAX_SECONDS", DefaultOrderBookCleanupMaxSeconds)
		plan, err := d.CleanupOrderBookSnapshotsBatches(orderbookDays, batchSize, maxDuration)
		if err != nil {
			log.Printf("[DB] CleanupStartupCaches: orderbook cleanup error: %v", err)
		} else if plan.SnapshotsDeleted > 0 || plan.LevelsDeleted > 0 {
			log.Printf("[DB] CleanupStartupCaches: kept %d days of orderbook snapshots, removed %d snapshots and %d levels", orderbookDays, plan.SnapshotsDeleted, plan.LevelsDeleted)
		}
	}

	scanDays := retentionDaysFromEnv("EVE_FLIPPER_SCAN_HISTORY_RETENTION_DAYS", DefaultScanHistoryRetentionDays)
	if scanDays > 0 {
		removed, err := d.ClearHistory(scanDays)
		if err != nil {
			log.Printf("[DB] CleanupStartupCaches: scan history cleanup error: %v", err)
		} else if removed > 0 {
			log.Printf("[DB] CleanupStartupCaches: kept %d days of scan history, removed %d scans and result sets", scanDays, removed)
		}
	}

	if _, err := d.sql.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		log.Printf("[DB] CleanupStartupCaches: wal checkpoint error: %v", err)
	}
}

func (d *DB) CleanupOrderBookSnapshotsBatches(keepDays int, batchSize int, maxDuration time.Duration) (OrderBookCleanupPlan, error) {
	if batchSize <= 0 {
		batchSize = DefaultOrderBookCleanupBatchSnapshots
	}
	if maxDuration <= 0 {
		maxDuration = time.Duration(DefaultOrderBookCleanupMaxSeconds) * time.Second
	}
	deadline := time.Now().Add(maxDuration)
	total := OrderBookCleanupPlan{KeepDays: keepDays}
	for {
		plan, err := d.CleanupOrderBookSnapshotsBatch(keepDays, batchSize)
		if err != nil {
			return total, err
		}
		total.KeepDays = plan.KeepDays
		total.Cutoff = plan.Cutoff
		total.OldestRemaining = plan.OldestRemaining
		total.NewestRemaining = plan.NewestRemaining
		total.SnapshotsDeleted += plan.SnapshotsDeleted
		total.LevelsDeleted += plan.LevelsDeleted
		if plan.SnapshotsDeleted == 0 || time.Now().After(deadline) {
			return total, nil
		}
	}
}

func (d *DB) CleanupOrderBookSnapshotsBatch(keepDays int, maxSnapshots int) (OrderBookCleanupPlan, error) {
	if maxSnapshots <= 0 {
		maxSnapshots = DefaultOrderBookCleanupBatchSnapshots
	}
	if maxSnapshots > 200 {
		maxSnapshots = 200
	}
	if keepDays <= 0 {
		return OrderBookCleanupPlan{}, fmt.Errorf("keep_days must be positive")
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -keepDays)
	plan := OrderBookCleanupPlan{
		KeepDays: keepDays,
		Cutoff:   utcRFC3339(cutoff),
	}
	if d == nil || d.sql == nil {
		return plan, nil
	}

	rows, err := d.sql.Query(`
		SELECT id
		  FROM orderbook_snapshots
		 WHERE captured_at < ?
		 ORDER BY captured_at ASC
		 LIMIT ?
	`, plan.Cutoff, maxSnapshots)
	if err != nil {
		return plan, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return plan, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return plan, err
	}
	if err := rows.Close(); err != nil {
		return plan, err
	}
	if len(ids) == 0 {
		if err := d.scanOrderBookRemainingRange(&plan); err != nil {
			return plan, err
		}
		return plan, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	inClause := strings.Join(placeholders, ",")
	if err := d.sql.QueryRow(`SELECT COUNT(*) FROM orderbook_levels WHERE snapshot_id IN (`+inClause+`)`, args...).Scan(&plan.LevelsDeleted); err != nil {
		return plan, err
	}
	plan.SnapshotsDeleted = int64(len(ids))

	tx, err := d.sql.Begin()
	if err != nil {
		return plan, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM orderbook_levels WHERE snapshot_id IN (`+inClause+`)`, args...); err != nil {
		return plan, err
	}
	if _, err := tx.Exec(`DELETE FROM orderbook_snapshots WHERE id IN (`+inClause+`)`, args...); err != nil {
		return plan, err
	}
	if err := tx.Commit(); err != nil {
		return plan, err
	}
	d.invalidateOrderBookStatsCache()
	if err := d.scanOrderBookRemainingRange(&plan); err != nil {
		return plan, err
	}
	return plan, nil
}

func durationSecondsFromEnv(name string, fallback int) time.Duration {
	seconds := retentionDaysFromEnv(name, fallback)
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func retentionDaysFromEnv(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	days, err := strconv.Atoi(raw)
	if err != nil {
		log.Printf("[DB] invalid %s=%q, using %d", name, raw, fallback)
		return fallback
	}
	if days < 0 {
		log.Printf("[DB] invalid %s=%q, using %d", name, raw, fallback)
		return fallback
	}
	return days
}
