package db

import (
	"eve-flipper/internal/esi"
	"time"
)

// GetHistory retrieves cached market history for a region/type pair.
// Returns nil, false if not cached or if cache is older than 24 hours.
func (d *DB) GetMarketHistory(regionID int32, typeID int32) ([]esi.HistoryEntry, bool) {
	var updatedAt string
	err := d.sql.QueryRow(
		"SELECT updated_at FROM market_history_meta WHERE region_id=? AND type_id=?",
		regionID, typeID,
	).Scan(&updatedAt)
	if err != nil {
		return nil, false
	}

	// Check if cache is fresh (< 24 hours)
	t, err := time.Parse(time.RFC3339, updatedAt)
	if err != nil || time.Since(t) > 24*time.Hour {
		return nil, false
	}

	rows, err := d.sql.Query(
		"SELECT date, average, highest, lowest, volume, order_count FROM market_history WHERE region_id=? AND type_id=? ORDER BY date",
		regionID, typeID,
	)
	if err != nil {
		return nil, false
	}
	defer rows.Close()

	var entries []esi.HistoryEntry
	for rows.Next() {
		var e esi.HistoryEntry
		if err := rows.Scan(&e.Date, &e.Average, &e.Highest, &e.Lowest, &e.Volume, &e.OrderCount); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	if len(entries) == 0 {
		return nil, false
	}
	return entries, true
}

// SetMarketHistory stores market history entries in the cache.
func (d *DB) SetMarketHistory(regionID int32, typeID int32, entries []esi.HistoryEntry) {
	tx, err := d.sql.Begin()
	if err != nil {
		return
	}
	defer tx.Rollback()

	// Delete old entries
	tx.Exec("DELETE FROM market_history WHERE region_id=? AND type_id=?", regionID, typeID)

	stmt, err := tx.Prepare("INSERT INTO market_history (region_id, type_id, date, average, highest, lowest, volume, order_count) VALUES (?,?,?,?,?,?,?,?)")
	if err != nil {
		return
	}
	defer stmt.Close()

	// Only keep last 90 days
	for _, e := range entries {
		stmt.Exec(regionID, typeID, e.Date, e.Average, e.Highest, e.Lowest, e.Volume, e.OrderCount)
	}

	// Update meta
	tx.Exec(
		"INSERT OR REPLACE INTO market_history_meta (region_id, type_id, updated_at) VALUES (?,?,?)",
		regionID, typeID, time.Now().UTC().Format(time.RFC3339),
	)

	tx.Commit()
}
