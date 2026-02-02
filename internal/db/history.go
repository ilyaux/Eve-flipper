package db

import "time"

// ScanRecord represents a scan history entry.
type ScanRecord struct {
	ID        int64   `json:"id"`
	Timestamp string  `json:"timestamp"`
	Tab       string  `json:"tab"`
	System    string  `json:"system"`
	Count     int     `json:"count"`
	TopProfit float64 `json:"top_profit"`
}

// InsertHistory inserts a scan history record and returns its ID.
func (d *DB) InsertHistory(tab, system string, count int, topProfit float64) int64 {
	result, err := d.sql.Exec(
		"INSERT INTO scan_history (timestamp, tab, system, count, top_profit) VALUES (?, ?, ?, ?, ?)",
		time.Now().Format(time.RFC3339), tab, system, count, topProfit,
	)
	if err != nil {
		return 0
	}
	id, _ := result.LastInsertId()
	return id
}

// GetHistory returns the last N scan history records (newest first).
func (d *DB) GetHistory(limit int) []ScanRecord {
	if limit <= 0 {
		limit = 50
	}
	rows, err := d.sql.Query(
		"SELECT id, timestamp, tab, system, count, top_profit FROM scan_history ORDER BY id DESC LIMIT ?",
		limit,
	)
	if err != nil {
		return []ScanRecord{}
	}
	defer rows.Close()

	var records []ScanRecord
	for rows.Next() {
		var r ScanRecord
		rows.Scan(&r.ID, &r.Timestamp, &r.Tab, &r.System, &r.Count, &r.TopProfit)
		records = append(records, r)
	}
	if records == nil {
		return []ScanRecord{}
	}
	return records
}
