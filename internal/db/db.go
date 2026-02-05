package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"eve-flipper/internal/logger"

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
	logger.Success("DB", fmt.Sprintf("Opened %s", path))
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
		logger.Info("DB", "Applied migration v1")
	}

	if version < 2 {
		_, err := d.sql.Exec(`
			CREATE TABLE IF NOT EXISTS market_history (
				region_id   INTEGER NOT NULL,
				type_id     INTEGER NOT NULL,
				date        TEXT NOT NULL,
				average     REAL,
				highest     REAL,
				lowest      REAL,
				volume      INTEGER,
				order_count INTEGER,
				PRIMARY KEY (region_id, type_id, date)
			);

			CREATE TABLE IF NOT EXISTS market_history_meta (
				region_id  INTEGER NOT NULL,
				type_id    INTEGER NOT NULL,
				updated_at TEXT NOT NULL,
				PRIMARY KEY (region_id, type_id)
			);

			INSERT OR IGNORE INTO schema_version (version) VALUES (2);
		`)
		if err != nil {
			return fmt.Errorf("migration v2: %w", err)
		}
		logger.Info("DB", "Applied migration v2 (market history)")
	}

	if version < 3 {
		_, err := d.sql.Exec(`
			CREATE TABLE IF NOT EXISTS auth_session (
				id              INTEGER PRIMARY KEY DEFAULT 1,
				character_id    INTEGER NOT NULL,
				character_name  TEXT NOT NULL,
				access_token    TEXT NOT NULL,
				refresh_token   TEXT NOT NULL,
				expires_at      INTEGER NOT NULL
			);

			INSERT OR IGNORE INTO schema_version (version) VALUES (3);
		`)
		if err != nil {
			return fmt.Errorf("migration v3: %w", err)
		}
		logger.Info("DB", "Applied migration v3 (auth session)")
	}

	if version < 4 {
		_, err := d.sql.Exec(`
			ALTER TABLE scan_history ADD COLUMN params_json TEXT DEFAULT '{}';
			ALTER TABLE scan_history ADD COLUMN total_profit REAL DEFAULT 0;
			ALTER TABLE scan_history ADD COLUMN duration_ms INTEGER DEFAULT 0;

			CREATE TABLE IF NOT EXISTS station_results (
				id           INTEGER PRIMARY KEY AUTOINCREMENT,
				scan_id      INTEGER NOT NULL REFERENCES scan_history(id),
				type_id      INTEGER,
				type_name    TEXT,
				buy_price    REAL,
				sell_price   REAL,
				margin       REAL,
				margin_pct   REAL,
				volume       REAL,
				buy_volume   REAL,
				sell_volume  REAL,
				station_id   INTEGER,
				station_name TEXT,
				cts          REAL,
				sds          INTEGER,
				period_roi   REAL,
				vwap         REAL,
				pvi          REAL,
				obds         REAL,
				bvs_ratio    REAL,
				dos          REAL
			);
			CREATE INDEX IF NOT EXISTS idx_station_scan ON station_results(scan_id);

			INSERT OR IGNORE INTO schema_version (version) VALUES (4);
		`)
		if err != nil {
			return fmt.Errorf("migration v4: %w", err)
		}
		logger.Info("DB", "Applied migration v4 (scan history)")
	}

	if version < 5 {
		_, err := d.sql.Exec(`
			CREATE TABLE IF NOT EXISTS demand_region_cache (
				region_id      INTEGER PRIMARY KEY,
				region_name    TEXT NOT NULL,
				hot_score      REAL NOT NULL DEFAULT 0,
				status         TEXT NOT NULL DEFAULT 'normal',
				kills_today    INTEGER NOT NULL DEFAULT 0,
				kills_baseline INTEGER NOT NULL DEFAULT 0,
				isk_destroyed  REAL NOT NULL DEFAULT 0,
				active_players INTEGER NOT NULL DEFAULT 0,
				top_ships      TEXT DEFAULT '[]',
				stats_json     TEXT DEFAULT '{}',
				updated_at     TEXT NOT NULL
			);

			CREATE TABLE IF NOT EXISTS demand_item_cache (
				id             INTEGER PRIMARY KEY AUTOINCREMENT,
				region_id      INTEGER NOT NULL,
				type_id        INTEGER NOT NULL,
				type_name      TEXT,
				group_id       INTEGER,
				group_name     TEXT,
				losses_per_day INTEGER NOT NULL DEFAULT 0,
				demand_score   REAL NOT NULL DEFAULT 0,
				updated_at     TEXT NOT NULL,
				UNIQUE(region_id, type_id)
			);
			CREATE INDEX IF NOT EXISTS idx_demand_item_region ON demand_item_cache(region_id);
			CREATE INDEX IF NOT EXISTS idx_demand_item_score ON demand_item_cache(demand_score DESC);

			INSERT OR IGNORE INTO schema_version (version) VALUES (5);
		`)
		if err != nil {
			return fmt.Errorf("migration v5: %w", err)
		}
		logger.Info("DB", "Applied migration v5 (demand cache)")
	}

	// v6: ensure station_results exists (v4 may have failed after ALTER TABLE if columns already existed)
	if version < 6 {
		_, err := d.sql.Exec(`
			CREATE TABLE IF NOT EXISTS station_results (
				id           INTEGER PRIMARY KEY AUTOINCREMENT,
				scan_id      INTEGER NOT NULL REFERENCES scan_history(id),
				type_id      INTEGER,
				type_name    TEXT,
				buy_price    REAL,
				sell_price   REAL,
				margin       REAL,
				margin_pct   REAL,
				volume       REAL,
				buy_volume   REAL,
				sell_volume  REAL,
				station_id   INTEGER,
				station_name TEXT,
				cts          REAL,
				sds          INTEGER,
				period_roi   REAL,
				vwap         REAL,
				pvi          REAL,
				obds         REAL,
				bvs_ratio    REAL,
				dos          REAL
			);
			CREATE INDEX IF NOT EXISTS idx_station_scan ON station_results(scan_id);
			INSERT OR IGNORE INTO schema_version (version) VALUES (6);
		`)
		if err != nil {
			return fmt.Errorf("migration v6: %w", err)
		}
		logger.Info("DB", "Applied migration v6 (station_results ensure)")
	}

	return nil
}

// SqlDB returns the underlying *sql.DB for use by other packages (e.g. auth store).
func (d *DB) SqlDB() *sql.DB {
	return d.sql
}
