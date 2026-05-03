package db

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"eve-flipper/internal/esi"
)

var orderbookRecordMu sync.Mutex

type OrderBookSnapshotMeta struct {
	ID                  int64  `json:"id"`
	Source              string `json:"source"`
	RegionID            int32  `json:"region_id"`
	OrderType           string `json:"order_type"`
	TypeID              int32  `json:"type_id"`
	LocationID          int64  `json:"location_id"`
	ETag                string `json:"etag"`
	SnapshotHash        string `json:"snapshot_hash"`
	CapturedAt          string `json:"captured_at"`
	LastSeenAt          string `json:"last_seen_at"`
	ExpiresAt           string `json:"expires_at"`
	OrderCount          int    `json:"order_count"`
	LevelCount          int    `json:"level_count"`
	UniqueTypeCount     int    `json:"unique_type_count"`
	UniqueLocationCount int    `json:"unique_location_count"`
}

type OrderBookLevel struct {
	SnapshotID   int64   `json:"snapshot_id"`
	RegionID     int32   `json:"region_id"`
	TypeID       int32   `json:"type_id"`
	LocationID   int64   `json:"location_id"`
	SystemID     int32   `json:"system_id"`
	Side         string  `json:"side"`
	Price        float64 `json:"price"`
	VolumeRemain int64   `json:"volume_remain"`
	OrderCount   int     `json:"order_count"`
}

type OrderBookSnapshotFilter struct {
	Source     string
	RegionID   int32
	OrderType  string
	TypeID     int32
	LocationID int64
	Limit      int
}

type OrderBookLevelFilter struct {
	TypeID     int32
	LocationID int64
	Side       string
	Limit      int
}

type OrderBookReplayFilter struct {
	RegionID       int32
	TypeID         int32
	LocationID     int64
	Side           string
	FromCapturedAt time.Time
	ToCapturedAt   time.Time
	Limit          int
	LevelLimit     int
}

type OrderBookReplayBook struct {
	Snapshot OrderBookSnapshotMeta `json:"snapshot"`
	Levels   []OrderBookLevel      `json:"levels"`
}

type OrderBookStatsType struct {
	TypeID        int32 `json:"type_id"`
	SnapshotCount int64 `json:"snapshot_count"`
	LevelCount    int64 `json:"level_count"`
	VolumeRemain  int64 `json:"volume_remain"`
}

type OrderBookStatsLocation struct {
	LocationID    int64 `json:"location_id"`
	SnapshotCount int64 `json:"snapshot_count"`
	LevelCount    int64 `json:"level_count"`
	VolumeRemain  int64 `json:"volume_remain"`
}

type OrderBookStats struct {
	SnapshotCount       int64                    `json:"snapshot_count"`
	LevelCount          int64                    `json:"level_count"`
	UniqueTypeCount     int64                    `json:"unique_type_count"`
	UniqueLocationCount int64                    `json:"unique_location_count"`
	TotalVolumeRemain   int64                    `json:"total_volume_remain"`
	ApproxBytes         int64                    `json:"approx_bytes"`
	OldestCapturedAt    string                   `json:"oldest_captured_at"`
	NewestCapturedAt    string                   `json:"newest_captured_at"`
	TopTypes            []OrderBookStatsType     `json:"top_types"`
	TopLocations        []OrderBookStatsLocation `json:"top_locations"`
}

type OrderBookCleanupPlan struct {
	KeepDays         int    `json:"keep_days"`
	Cutoff           string `json:"cutoff"`
	DryRun           bool   `json:"dry_run"`
	Vacuum           bool   `json:"vacuum"`
	SnapshotsDeleted int64  `json:"snapshots_deleted"`
	LevelsDeleted    int64  `json:"levels_deleted"`
	OldestRemaining  string `json:"oldest_remaining"`
	NewestRemaining  string `json:"newest_remaining"`
}

type orderbookLevelKey struct {
	side       string
	typeID     int32
	locationID int64
	systemID   int32
	price      float64
}

type orderbookLevelAgg struct {
	key          orderbookLevelKey
	volumeRemain int64
	orderCount   int
}

func normalizeOrderBookSource(source string) string {
	source = strings.ToLower(strings.TrimSpace(source))
	switch source {
	case "", "region":
		return "region"
	case "region_type":
		return "region_type"
	case "structure":
		return "structure"
	default:
		return source
	}
}

func normalizeOrderBookOrderType(orderType string) string {
	orderType = strings.ToLower(strings.TrimSpace(orderType))
	switch orderType {
	case "buy", "sell", "all":
		return orderType
	case "":
		return "all"
	default:
		return orderType
	}
}

func normalizeOrderBookSide(side string) string {
	side = strings.ToLower(strings.TrimSpace(side))
	switch side {
	case "buy", "sell":
		return side
	default:
		return ""
	}
}

func utcRFC3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func buildOrderBookLevels(snapshot esi.MarketOrderSnapshot) ([]orderbookLevelAgg, int, int) {
	levelsByKey := make(map[orderbookLevelKey]*orderbookLevelAgg)
	typeSet := make(map[int32]bool)
	locationSet := make(map[int64]bool)

	for _, order := range snapshot.Orders {
		if order.TypeID <= 0 || order.VolumeRemain <= 0 || order.Price <= 0 || math.IsNaN(order.Price) || math.IsInf(order.Price, 0) {
			continue
		}
		side := "sell"
		if order.IsBuyOrder {
			side = "buy"
		}
		locationID := order.LocationID
		if locationID <= 0 && snapshot.LocationID > 0 {
			locationID = snapshot.LocationID
		}
		key := orderbookLevelKey{
			side:       side,
			typeID:     order.TypeID,
			locationID: locationID,
			systemID:   order.SystemID,
			price:      order.Price,
		}
		level := levelsByKey[key]
		if level == nil {
			level = &orderbookLevelAgg{key: key}
			levelsByKey[key] = level
		}
		level.volumeRemain += int64(order.VolumeRemain)
		level.orderCount++
		typeSet[order.TypeID] = true
		if locationID > 0 {
			locationSet[locationID] = true
		}
	}

	levels := make([]orderbookLevelAgg, 0, len(levelsByKey))
	for _, level := range levelsByKey {
		if level.volumeRemain > 0 && level.orderCount > 0 {
			levels = append(levels, *level)
		}
	}
	sort.Slice(levels, func(i, j int) bool {
		a, b := levels[i].key, levels[j].key
		if a.typeID != b.typeID {
			return a.typeID < b.typeID
		}
		if a.locationID != b.locationID {
			return a.locationID < b.locationID
		}
		if a.systemID != b.systemID {
			return a.systemID < b.systemID
		}
		if a.side != b.side {
			return a.side < b.side
		}
		return a.price < b.price
	})

	return levels, len(typeSet), len(locationSet)
}

func hashOrderBookLevels(snapshot esi.MarketOrderSnapshot, levels []orderbookLevelAgg) string {
	hasher := sha256.New()
	fmt.Fprintf(hasher, "source=%s|region=%d|order_type=%s|type=%d|location=%d|orders=%d\n",
		normalizeOrderBookSource(snapshot.Source),
		snapshot.RegionID,
		normalizeOrderBookOrderType(snapshot.OrderType),
		snapshot.TypeID,
		snapshot.LocationID,
		len(snapshot.Orders),
	)
	for _, level := range levels {
		key := level.key
		fmt.Fprintf(hasher, "%s|%d|%d|%d|%.8f|%d|%d\n",
			key.side,
			key.typeID,
			key.locationID,
			key.systemID,
			key.price,
			level.volumeRemain,
			level.orderCount,
		)
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

// RecordMarketOrderSnapshot stores aggregated orderbook levels for live ESI orders.
// Duplicate unchanged books are de-duped by hash and only update last_seen_at.
func (d *DB) RecordMarketOrderSnapshot(snapshot esi.MarketOrderSnapshot) error {
	if d == nil || d.sql == nil || len(snapshot.Orders) == 0 {
		return nil
	}
	source := normalizeOrderBookSource(snapshot.Source)
	orderType := normalizeOrderBookOrderType(snapshot.OrderType)
	capturedAt := snapshot.CapturedAt
	if capturedAt.IsZero() {
		capturedAt = time.Now().UTC()
	}
	capturedAt = capturedAt.UTC()
	capturedAtStr := utcRFC3339(capturedAt)
	expiresAtStr := utcRFC3339(snapshot.ExpiresAt)
	etag := strings.TrimSpace(snapshot.ETag)

	levels, uniqueTypes, uniqueLocations := buildOrderBookLevels(snapshot)
	if len(levels) == 0 {
		return nil
	}
	hash := hashOrderBookLevels(snapshot, levels)

	orderbookRecordMu.Lock()
	defer orderbookRecordMu.Unlock()

	var existingID int64
	err := d.sql.QueryRow(`
		SELECT id
		  FROM orderbook_snapshots
		 WHERE source = ?
		   AND region_id = ?
		   AND order_type = ?
		   AND type_id = ?
		   AND location_id = ?
		   AND snapshot_hash = ?
		 LIMIT 1
	`, source, snapshot.RegionID, orderType, snapshot.TypeID, snapshot.LocationID, hash).Scan(&existingID)
	switch {
	case err == nil && existingID > 0:
		_, err = d.sql.Exec(`
			UPDATE orderbook_snapshots
			   SET last_seen_at = ?,
			       expires_at = ?,
			       etag = CASE WHEN ? != '' THEN ? ELSE etag END
			 WHERE id = ?
		`, capturedAtStr, expiresAtStr, etag, etag, existingID)
		return err
	case err != nil && err != sql.ErrNoRows:
		return err
	}

	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`
		INSERT INTO orderbook_snapshots (
			source, region_id, order_type, type_id, location_id,
			etag, snapshot_hash, captured_at, last_seen_at, expires_at,
			order_count, level_count, unique_type_count, unique_location_count
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		source, snapshot.RegionID, orderType, snapshot.TypeID, snapshot.LocationID,
		etag, hash, capturedAtStr, capturedAtStr, expiresAtStr,
		len(snapshot.Orders), len(levels), uniqueTypes, uniqueLocations,
	)
	if err != nil {
		return err
	}
	snapshotID, err := res.LastInsertId()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`
		INSERT INTO orderbook_levels (
			snapshot_id, region_id, type_id, location_id, system_id, side,
			price, volume_remain, order_count
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, level := range levels {
		key := level.key
		if _, err := stmt.Exec(
			snapshotID,
			snapshot.RegionID,
			key.typeID,
			key.locationID,
			key.systemID,
			key.side,
			key.price,
			level.volumeRemain,
			level.orderCount,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func scanOrderBookSnapshot(scanner interface{ Scan(dest ...any) error }) (OrderBookSnapshotMeta, error) {
	var snap OrderBookSnapshotMeta
	err := scanner.Scan(
		&snap.ID,
		&snap.Source,
		&snap.RegionID,
		&snap.OrderType,
		&snap.TypeID,
		&snap.LocationID,
		&snap.ETag,
		&snap.SnapshotHash,
		&snap.CapturedAt,
		&snap.LastSeenAt,
		&snap.ExpiresAt,
		&snap.OrderCount,
		&snap.LevelCount,
		&snap.UniqueTypeCount,
		&snap.UniqueLocationCount,
	)
	return snap, err
}

func (d *DB) ListOrderBookSnapshots(filter OrderBookSnapshotFilter) ([]OrderBookSnapshotMeta, error) {
	if d == nil || d.sql == nil {
		return nil, nil
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	clauses := []string{"1=1"}
	args := make([]any, 0)
	if source := normalizeOrderBookSource(filter.Source); strings.TrimSpace(filter.Source) != "" {
		clauses = append(clauses, "source = ?")
		args = append(args, source)
	}
	if filter.RegionID > 0 {
		clauses = append(clauses, "region_id = ?")
		args = append(args, filter.RegionID)
	}
	if orderType := normalizeOrderBookOrderType(filter.OrderType); strings.TrimSpace(filter.OrderType) != "" {
		clauses = append(clauses, "order_type = ?")
		args = append(args, orderType)
	}
	if filter.TypeID > 0 {
		clauses = append(clauses, "(type_id = ? OR EXISTS (SELECT 1 FROM orderbook_levels l WHERE l.snapshot_id = orderbook_snapshots.id AND l.type_id = ?))")
		args = append(args, filter.TypeID, filter.TypeID)
	}
	if filter.LocationID > 0 {
		clauses = append(clauses, "(location_id = ? OR EXISTS (SELECT 1 FROM orderbook_levels l WHERE l.snapshot_id = orderbook_snapshots.id AND l.location_id = ?))")
		args = append(args, filter.LocationID, filter.LocationID)
	}
	args = append(args, limit)

	rows, err := d.sql.Query(`
		SELECT id, source, region_id, order_type, type_id, location_id,
		       etag, snapshot_hash, captured_at, last_seen_at, expires_at,
		       order_count, level_count, unique_type_count, unique_location_count
		  FROM orderbook_snapshots
		 WHERE `+strings.Join(clauses, " AND ")+`
		 ORDER BY captured_at DESC, id DESC
		 LIMIT ?
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []OrderBookSnapshotMeta
	for rows.Next() {
		snap, err := scanOrderBookSnapshot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, snap)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (d *DB) ListOrderBookReplayBooks(filter OrderBookReplayFilter) ([]OrderBookReplayBook, error) {
	if d == nil || d.sql == nil || filter.TypeID <= 0 {
		return nil, nil
	}
	side := normalizeOrderBookSide(filter.Side)
	if side == "" {
		return nil, nil
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 1000
	}
	if limit > 5000 {
		limit = 5000
	}
	levelLimit := filter.LevelLimit
	if levelLimit <= 0 {
		levelLimit = 2000
	}
	if levelLimit > 20000 {
		levelLimit = 20000
	}

	orderType := side
	clauses := []string{"l.type_id = ?", "l.side = ?", "(s.order_type = 'all' OR s.order_type = ?)"}
	args := []any{filter.TypeID, side, orderType}
	if filter.LocationID > 0 {
		clauses = append(clauses, "(s.location_id = ? OR l.location_id = ?)")
		args = append(args, filter.LocationID, filter.LocationID)
	} else if filter.RegionID > 0 {
		clauses = append(clauses, "(s.region_id = ? OR l.region_id = ?)")
		args = append(args, filter.RegionID, filter.RegionID)
	}
	if !filter.FromCapturedAt.IsZero() {
		clauses = append(clauses, "s.captured_at >= ?")
		args = append(args, utcRFC3339(filter.FromCapturedAt))
	}
	if !filter.ToCapturedAt.IsZero() {
		clauses = append(clauses, "s.captured_at <= ?")
		args = append(args, utcRFC3339(filter.ToCapturedAt))
	}
	args = append(args, limit)

	rows, err := d.sql.Query(`
		SELECT DISTINCT s.id, s.source, s.region_id, s.order_type, s.type_id, s.location_id,
		       s.etag, s.snapshot_hash, s.captured_at, s.last_seen_at, s.expires_at,
		       s.order_count, s.level_count, s.unique_type_count, s.unique_location_count
		  FROM orderbook_snapshots s
		  JOIN orderbook_levels l ON l.snapshot_id = s.id
		 WHERE `+strings.Join(clauses, " AND ")+`
		 ORDER BY s.captured_at ASC, s.id ASC
		 LIMIT ?
	`, args...)
	if err != nil {
		return nil, err
	}

	snapshots := make([]OrderBookSnapshotMeta, 0)
	for rows.Next() {
		snap, err := scanOrderBookSnapshot(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		snapshots = append(snapshots, snap)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	out := make([]OrderBookReplayBook, 0, len(snapshots))
	for _, snap := range snapshots {
		levels, err := d.GetOrderBookLevels(snap.ID, OrderBookLevelFilter{
			TypeID:     filter.TypeID,
			LocationID: filter.LocationID,
			Side:       side,
			Limit:      levelLimit,
		})
		if err != nil {
			return nil, err
		}
		if len(levels) == 0 {
			continue
		}
		out = append(out, OrderBookReplayBook{Snapshot: snap, Levels: levels})
	}
	return out, nil
}

func nullableStringValue(v sql.NullString) string {
	if v.Valid {
		return v.String
	}
	return ""
}

func (d *DB) GetOrderBookStats(limit int) (OrderBookStats, error) {
	if d == nil || d.sql == nil {
		return OrderBookStats{
			TopTypes:     []OrderBookStatsType{},
			TopLocations: []OrderBookStatsLocation{},
		}, nil
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	var stats OrderBookStats
	var oldest, newest sql.NullString
	if err := d.sql.QueryRow(`
		SELECT COUNT(*), MIN(captured_at), MAX(captured_at)
		  FROM orderbook_snapshots
	`).Scan(&stats.SnapshotCount, &oldest, &newest); err != nil {
		return stats, err
	}
	stats.OldestCapturedAt = nullableStringValue(oldest)
	stats.NewestCapturedAt = nullableStringValue(newest)

	if err := d.sql.QueryRow(`
		SELECT COUNT(*),
		       COUNT(DISTINCT CASE WHEN type_id > 0 THEN type_id END),
		       COUNT(DISTINCT CASE WHEN location_id > 0 THEN location_id END),
		       COALESCE(SUM(volume_remain), 0)
		  FROM orderbook_levels
	`).Scan(&stats.LevelCount, &stats.UniqueTypeCount, &stats.UniqueLocationCount, &stats.TotalVolumeRemain); err != nil {
		return stats, err
	}
	stats.ApproxBytes = stats.SnapshotCount*320 + stats.LevelCount*96

	typeRows, err := d.sql.Query(`
		SELECT type_id,
		       COUNT(DISTINCT snapshot_id) AS snapshot_count,
		       COUNT(*) AS level_count,
		       COALESCE(SUM(volume_remain), 0) AS volume_remain
		  FROM orderbook_levels
		 WHERE type_id > 0
		 GROUP BY type_id
		 ORDER BY snapshot_count DESC, level_count DESC, type_id ASC
		 LIMIT ?
	`, limit)
	if err != nil {
		return stats, err
	}
	stats.TopTypes = make([]OrderBookStatsType, 0)
	for typeRows.Next() {
		var row OrderBookStatsType
		if err := typeRows.Scan(&row.TypeID, &row.SnapshotCount, &row.LevelCount, &row.VolumeRemain); err != nil {
			typeRows.Close()
			return stats, err
		}
		stats.TopTypes = append(stats.TopTypes, row)
	}
	if err := typeRows.Err(); err != nil {
		typeRows.Close()
		return stats, err
	}
	if err := typeRows.Close(); err != nil {
		return stats, err
	}

	locationRows, err := d.sql.Query(`
		SELECT location_id,
		       COUNT(DISTINCT snapshot_id) AS snapshot_count,
		       COUNT(*) AS level_count,
		       COALESCE(SUM(volume_remain), 0) AS volume_remain
		  FROM orderbook_levels
		 WHERE location_id > 0
		 GROUP BY location_id
		 ORDER BY snapshot_count DESC, level_count DESC, location_id ASC
		 LIMIT ?
	`, limit)
	if err != nil {
		return stats, err
	}
	stats.TopLocations = make([]OrderBookStatsLocation, 0)
	for locationRows.Next() {
		var row OrderBookStatsLocation
		if err := locationRows.Scan(&row.LocationID, &row.SnapshotCount, &row.LevelCount, &row.VolumeRemain); err != nil {
			locationRows.Close()
			return stats, err
		}
		stats.TopLocations = append(stats.TopLocations, row)
	}
	if err := locationRows.Err(); err != nil {
		locationRows.Close()
		return stats, err
	}
	if err := locationRows.Close(); err != nil {
		return stats, err
	}

	return stats, nil
}

func (d *DB) CleanupOrderBookSnapshots(keepDays int, dryRun bool, vacuum bool) (OrderBookCleanupPlan, error) {
	if keepDays <= 0 {
		return OrderBookCleanupPlan{}, fmt.Errorf("keep_days must be positive")
	}
	if keepDays > 3650 {
		return OrderBookCleanupPlan{}, fmt.Errorf("keep_days must be <= 3650")
	}

	cutoff := time.Now().UTC().AddDate(0, 0, -keepDays)
	plan := OrderBookCleanupPlan{
		KeepDays: keepDays,
		Cutoff:   utcRFC3339(cutoff),
		DryRun:   dryRun,
		Vacuum:   vacuum && !dryRun,
	}
	if d == nil || d.sql == nil {
		return plan, nil
	}

	if err := d.sql.QueryRow(`
		SELECT COUNT(*)
		  FROM orderbook_snapshots
		 WHERE captured_at < ?
	`, plan.Cutoff).Scan(&plan.SnapshotsDeleted); err != nil {
		return plan, err
	}
	if err := d.sql.QueryRow(`
		SELECT COUNT(*)
		  FROM orderbook_levels
		 WHERE snapshot_id IN (
		       SELECT id
		         FROM orderbook_snapshots
		        WHERE captured_at < ?
		 )
	`, plan.Cutoff).Scan(&plan.LevelsDeleted); err != nil {
		return plan, err
	}
	if dryRun || plan.SnapshotsDeleted == 0 {
		if err := d.scanOrderBookRemainingRange(&plan); err != nil {
			return plan, err
		}
		return plan, nil
	}

	tx, err := d.sql.Begin()
	if err != nil {
		return plan, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		DELETE FROM orderbook_levels
		 WHERE snapshot_id IN (
		       SELECT id
		         FROM orderbook_snapshots
		        WHERE captured_at < ?
		 )
	`, plan.Cutoff); err != nil {
		return plan, err
	}
	if _, err := tx.Exec(`
		DELETE FROM orderbook_snapshots
		 WHERE captured_at < ?
	`, plan.Cutoff); err != nil {
		return plan, err
	}
	if err := tx.Commit(); err != nil {
		return plan, err
	}
	if plan.Vacuum {
		if _, err := d.sql.Exec(`VACUUM`); err != nil {
			return plan, err
		}
	}
	if err := d.scanOrderBookRemainingRange(&plan); err != nil {
		return plan, err
	}
	return plan, nil
}

func (d *DB) scanOrderBookRemainingRange(plan *OrderBookCleanupPlan) error {
	if d == nil || d.sql == nil || plan == nil {
		return nil
	}
	var oldest, newest sql.NullString
	if err := d.sql.QueryRow(`
		SELECT MIN(captured_at), MAX(captured_at)
		  FROM orderbook_snapshots
		 WHERE captured_at >= ?
	`, plan.Cutoff).Scan(&oldest, &newest); err != nil {
		return err
	}
	plan.OldestRemaining = nullableStringValue(oldest)
	plan.NewestRemaining = nullableStringValue(newest)
	return nil
}

func (d *DB) GetOrderBookSnapshot(id int64) (OrderBookSnapshotMeta, error) {
	if d == nil || d.sql == nil || id <= 0 {
		return OrderBookSnapshotMeta{}, sql.ErrNoRows
	}
	return scanOrderBookSnapshot(d.sql.QueryRow(`
		SELECT id, source, region_id, order_type, type_id, location_id,
		       etag, snapshot_hash, captured_at, last_seen_at, expires_at,
		       order_count, level_count, unique_type_count, unique_location_count
		  FROM orderbook_snapshots
		 WHERE id = ?
		 LIMIT 1
	`, id))
}

func (d *DB) GetOrderBookLevels(snapshotID int64, filter OrderBookLevelFilter) ([]OrderBookLevel, error) {
	if d == nil || d.sql == nil || snapshotID <= 0 {
		return nil, nil
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 5000
	}
	if limit > 50000 {
		limit = 50000
	}

	clauses := []string{"snapshot_id = ?"}
	args := []any{snapshotID}
	if filter.TypeID > 0 {
		clauses = append(clauses, "type_id = ?")
		args = append(args, filter.TypeID)
	}
	if filter.LocationID > 0 {
		clauses = append(clauses, "location_id = ?")
		args = append(args, filter.LocationID)
	}
	if side := normalizeOrderBookSide(filter.Side); side != "" {
		clauses = append(clauses, "side = ?")
		args = append(args, side)
	}
	args = append(args, limit)

	rows, err := d.sql.Query(`
		SELECT snapshot_id, region_id, type_id, location_id, system_id, side,
		       price, volume_remain, order_count
		  FROM orderbook_levels
		 WHERE `+strings.Join(clauses, " AND ")+`
		 ORDER BY type_id ASC, location_id ASC, side ASC,
		          CASE WHEN side = 'buy' THEN -price ELSE price END ASC
		 LIMIT ?
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []OrderBookLevel
	for rows.Next() {
		var level OrderBookLevel
		if err := rows.Scan(
			&level.SnapshotID,
			&level.RegionID,
			&level.TypeID,
			&level.LocationID,
			&level.SystemID,
			&level.Side,
			&level.Price,
			&level.VolumeRemain,
			&level.OrderCount,
		); err != nil {
			return nil, err
		}
		out = append(out, level)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
