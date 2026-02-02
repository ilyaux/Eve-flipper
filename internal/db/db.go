package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB wraps a SQLite database connection.
type DB struct {
	sql *sql.DB
}

func dbPath() string {
	// Prefer working directory so the DB is stable across go run / go build.
	// Fall back to executable directory for deployed builds.
	if wd, err := os.Getwd(); err == nil {
		return filepath.Join(wd, "flipper.db")
	}
	exe, _ := os.Executable()
	return filepath.Join(filepath.Dir(exe), "flipper.db")
}

// Open opens (or creates) the SQLite database and runs migrations.
func Open() (*DB, error) {
	path := dbPath()
	sqlDB, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	d := &DB{sql: sqlDB}
	if err := d.migrate(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("migrate db: %w", err)
	}
	log.Printf("[DB] Opened %s", path)
	return d, nil
}

// Close closes the database connection.
func (d *DB) Close() error {
	return d.sql.Close()
}

func (d *DB) migrate() error {
	version := 0
	// Try to read current version
	d.sql.QueryRow("SELECT version FROM schema_version ORDER BY version DESC LIMIT 1").Scan(&version)

	if version < 1 {
		_, err := d.sql.Exec(`
			CREATE TABLE IF NOT EXISTS schema_version (version INTEGER PRIMARY KEY);

			CREATE TABLE IF NOT EXISTS config (
				key   TEXT PRIMARY KEY,
				value TEXT NOT NULL
			);

			CREATE TABLE IF NOT EXISTS watchlist (
				type_id          INTEGER PRIMARY KEY,
				type_name        TEXT NOT NULL,
				added_at         TEXT NOT NULL,
				alert_min_margin REAL NOT NULL DEFAULT 0
			);

			CREATE TABLE IF NOT EXISTS scan_history (
				id         INTEGER PRIMARY KEY AUTOINCREMENT,
				timestamp  TEXT NOT NULL,
				tab        TEXT NOT NULL,
				system     TEXT NOT NULL,
				count      INTEGER NOT NULL,
				top_profit REAL NOT NULL
			);
			CREATE INDEX IF NOT EXISTS idx_scan_history_ts ON scan_history(timestamp);

			CREATE TABLE IF NOT EXISTS flip_results (
				id               INTEGER PRIMARY KEY AUTOINCREMENT,
				scan_id          INTEGER NOT NULL REFERENCES scan_history(id),
				type_id          INTEGER,
				type_name        TEXT,
				volume           REAL,
				buy_price        REAL,
				buy_station      TEXT,
				buy_system_name  TEXT,
				buy_system_id    INTEGER,
				sell_price       REAL,
				sell_station     TEXT,
				sell_system_name TEXT,
				sell_system_id   INTEGER,
				profit_per_unit  REAL,
				margin_percent   REAL,
				units_to_buy     INTEGER,
				buy_order_remain INTEGER,
				sell_order_remain INTEGER,
				total_profit     REAL,
				profit_per_jump  REAL,
				buy_jumps        INTEGER,
				sell_jumps       INTEGER,
				total_jumps      INTEGER
			);
			CREATE INDEX IF NOT EXISTS idx_flip_scan ON flip_results(scan_id);
			CREATE INDEX IF NOT EXISTS idx_flip_type ON flip_results(type_id);

			CREATE TABLE IF NOT EXISTS contract_results (
				id              INTEGER PRIMARY KEY AUTOINCREMENT,
				scan_id         INTEGER NOT NULL REFERENCES scan_history(id),
				contract_id     INTEGER,
				title           TEXT,
				price           REAL,
				market_value    REAL,
				profit          REAL,
				margin_percent  REAL,
				volume          REAL,
				station_name    TEXT,
				item_count      INTEGER,
				jumps           INTEGER,
				profit_per_jump REAL
			);
			CREATE INDEX IF NOT EXISTS idx_contract_scan ON contract_results(scan_id);

			CREATE TABLE IF NOT EXISTS station_cache (
				location_id INTEGER PRIMARY KEY,
				name        TEXT NOT NULL
			);

			INSERT OR IGNORE INTO schema_version (version) VALUES (1);
		`)
		if err != nil {
			return fmt.Errorf("migration v1: %w", err)
		}
		log.Println("[DB] Applied migration v1")
	}

	return nil
}
