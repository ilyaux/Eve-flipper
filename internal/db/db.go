package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	sqlDB, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
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
				expected_profit REAL NOT NULL DEFAULT 0,
				expected_margin_percent REAL NOT NULL DEFAULT 0,
				sell_confidence REAL NOT NULL DEFAULT 0,
				est_liquidation_days REAL NOT NULL DEFAULT 0,
				conservative_value REAL NOT NULL DEFAULT 0,
				carry_cost REAL NOT NULL DEFAULT 0,
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

	if version < 7 {
		_, err := d.sql.Exec(`
			CREATE TABLE IF NOT EXISTS demand_fitting_cache (
				region_id        INTEGER NOT NULL,
				type_id          INTEGER NOT NULL,
				type_name        TEXT,
				category         TEXT NOT NULL,
				total_destroyed  INTEGER NOT NULL DEFAULT 0,
				killmail_count   INTEGER NOT NULL DEFAULT 0,
				avg_per_killmail REAL NOT NULL DEFAULT 0,
				est_daily_demand REAL NOT NULL DEFAULT 0,
				sampled_kills    INTEGER NOT NULL DEFAULT 0,
				total_kills_24h  INTEGER NOT NULL DEFAULT 0,
				updated_at       TEXT NOT NULL,
				PRIMARY KEY (region_id, type_id)
			);
			CREATE INDEX IF NOT EXISTS idx_demand_fitting_region ON demand_fitting_cache(region_id);
			CREATE INDEX IF NOT EXISTS idx_demand_fitting_demand ON demand_fitting_cache(est_daily_demand DESC);

			INSERT OR IGNORE INTO schema_version (version) VALUES (7);
		`)
		if err != nil {
			return fmt.Errorf("migration v7: %w", err)
		}
		logger.Info("DB", "Applied migration v7 (demand fitting cache)")
	}

	if version < 8 {
		_, err := d.sql.Exec(`
			CREATE TABLE IF NOT EXISTS demand_history (
				region_id      INTEGER NOT NULL,
				snapshot_at    TEXT NOT NULL,
				hot_score      REAL NOT NULL,
				status         TEXT NOT NULL,
				kills_today    INTEGER NOT NULL,
				active_players INTEGER NOT NULL,
				PRIMARY KEY (region_id, snapshot_at)
			);
			CREATE INDEX IF NOT EXISTS idx_demand_history_region ON demand_history(region_id, snapshot_at);

			INSERT OR IGNORE INTO schema_version (version) VALUES (8);
		`)
		if err != nil {
			return fmt.Errorf("migration v8: %w", err)
		}
		logger.Info("DB", "Applied migration v8 (demand history)")
	}

	if version < 10 {
		_, err := d.sql.Exec(`
			CREATE TABLE IF NOT EXISTS route_results (
				id               INTEGER PRIMARY KEY AUTOINCREMENT,
				scan_id          INTEGER NOT NULL REFERENCES scan_history(id),
				route_index      INTEGER NOT NULL DEFAULT 0,
				hop_index        INTEGER NOT NULL DEFAULT 0,
				system_name      TEXT,
				station_name     TEXT,
				dest_system_name TEXT,
				dest_station_name TEXT,
				type_name        TEXT,
				type_id          INTEGER,
				buy_price        REAL,
				sell_price       REAL,
				units            INTEGER,
				profit           REAL,
				jumps            INTEGER,
				total_profit     REAL,
				total_jumps      INTEGER,
				profit_per_jump  REAL,
				hop_count        INTEGER
			);
			CREATE INDEX IF NOT EXISTS idx_route_scan ON route_results(scan_id);

			INSERT OR IGNORE INTO schema_version (version) VALUES (10);
		`)
		if err != nil {
			return fmt.Errorf("migration v10: %w", err)
		}
		logger.Info("DB", "Applied migration v10 (route results)")
	}

	if version < 11 {
		// Extend station_results with execution-aware and daily-profit fields.
		// This keeps scan history consistent with current Station Trading model.
		stationCols := []struct {
			name string
			def  string
		}{
			{name: "daily_profit", def: "REAL NOT NULL DEFAULT 0"},
			{name: "real_profit", def: "REAL NOT NULL DEFAULT 0"},
			{name: "filled_qty", def: "INTEGER NOT NULL DEFAULT 0"},
			{name: "can_fill", def: "INTEGER NOT NULL DEFAULT 0"},
			{name: "expected_profit", def: "REAL NOT NULL DEFAULT 0"},
			{name: "expected_buy_price", def: "REAL NOT NULL DEFAULT 0"},
			{name: "expected_sell_price", def: "REAL NOT NULL DEFAULT 0"},
			{name: "slippage_buy_pct", def: "REAL NOT NULL DEFAULT 0"},
			{name: "slippage_sell_pct", def: "REAL NOT NULL DEFAULT 0"},
		}
		for _, c := range stationCols {
			if err := d.ensureTableColumn("station_results", c.name, c.def); err != nil {
				return fmt.Errorf("migration v11 add station_results.%s: %w", c.name, err)
			}
		}
		if _, err := d.sql.Exec(`INSERT OR IGNORE INTO schema_version (version) VALUES (11);`); err != nil {
			return fmt.Errorf("migration v11: %w", err)
		}
		logger.Info("DB", "Applied migration v11 (station_results execution fields)")
	}

	if version < 12 {
		contractCols := []struct {
			name string
			def  string
		}{
			{name: "expected_profit", def: "REAL NOT NULL DEFAULT 0"},
			{name: "expected_margin_percent", def: "REAL NOT NULL DEFAULT 0"},
			{name: "sell_confidence", def: "REAL NOT NULL DEFAULT 0"},
			{name: "est_liquidation_days", def: "REAL NOT NULL DEFAULT 0"},
			{name: "conservative_value", def: "REAL NOT NULL DEFAULT 0"},
			{name: "carry_cost", def: "REAL NOT NULL DEFAULT 0"},
		}
		for _, c := range contractCols {
			if err := d.ensureTableColumn("contract_results", c.name, c.def); err != nil {
				return fmt.Errorf("migration v12 add contract_results.%s: %w", c.name, err)
			}
		}
		if _, err := d.sql.Exec(`INSERT OR IGNORE INTO schema_version (version) VALUES (12);`); err != nil {
			return fmt.Errorf("migration v12: %w", err)
		}
		logger.Info("DB", "Applied migration v12 (contract_results long-horizon fields)")
	}

	if version < 13 {
		watchlistCols := []struct {
			name string
			def  string
		}{
			{name: "alert_enabled", def: "INTEGER NOT NULL DEFAULT 0"},
			{name: "alert_metric", def: "TEXT NOT NULL DEFAULT 'margin_percent'"},
			{name: "alert_threshold", def: "REAL NOT NULL DEFAULT 0"},
		}
		for _, c := range watchlistCols {
			if err := d.ensureTableColumn("watchlist", c.name, c.def); err != nil {
				return fmt.Errorf("migration v13 add watchlist.%s: %w", c.name, err)
			}
		}
		// Backfill legacy threshold into the new alert model.
		if _, err := d.sql.Exec(`
			UPDATE watchlist
			   SET alert_enabled = CASE WHEN alert_min_margin > 0 THEN 1 ELSE 0 END,
			       alert_metric = 'margin_percent',
			       alert_threshold = CASE WHEN alert_min_margin > 0 THEN alert_min_margin ELSE 0 END
			 WHERE alert_threshold <= 0;
		`); err != nil {
			return fmt.Errorf("migration v13 backfill watchlist alerts: %w", err)
		}
		if _, err := d.sql.Exec(`INSERT OR IGNORE INTO schema_version (version) VALUES (13);`); err != nil {
			return fmt.Errorf("migration v13: %w", err)
		}
		logger.Info("DB", "Applied migration v13 (watchlist alert model)")
	}

	if version < 14 {
		_, err := d.sql.Exec(`
			CREATE TABLE IF NOT EXISTS alert_history (
				id                  INTEGER PRIMARY KEY AUTOINCREMENT,
				watchlist_type_id   INTEGER NOT NULL,
				type_name           TEXT NOT NULL,
				alert_metric        TEXT NOT NULL,
				alert_threshold     REAL NOT NULL,
				current_value       REAL NOT NULL,
				message             TEXT NOT NULL,
				channels_sent       TEXT NOT NULL,
				channels_failed     TEXT,
				sent_at             TEXT NOT NULL,
				scan_id             INTEGER,
				FOREIGN KEY (watchlist_type_id) REFERENCES watchlist(type_id) ON DELETE CASCADE
			);
			CREATE INDEX IF NOT EXISTS idx_alert_history_type ON alert_history(watchlist_type_id, sent_at DESC);
			CREATE INDEX IF NOT EXISTS idx_alert_history_time ON alert_history(sent_at DESC);
			CREATE INDEX IF NOT EXISTS idx_alert_history_scan ON alert_history(scan_id);

			INSERT OR IGNORE INTO schema_version (version) VALUES (14);
		`)
		if err != nil {
			return fmt.Errorf("migration v14: %w", err)
		}
		logger.Info("DB", "Applied migration v14 (alert history)")
	}

	if version < 15 {
		_, err := d.sql.Exec(`
			CREATE TABLE IF NOT EXISTS auth_session_new (
				character_id    INTEGER PRIMARY KEY,
				character_name  TEXT NOT NULL,
				access_token    TEXT NOT NULL,
				refresh_token   TEXT NOT NULL,
				expires_at      INTEGER NOT NULL,
				is_active       INTEGER NOT NULL DEFAULT 0
			);

			INSERT OR REPLACE INTO auth_session_new (character_id, character_name, access_token, refresh_token, expires_at, is_active)
			SELECT
				character_id,
				character_name,
				access_token,
				refresh_token,
				expires_at,
				CASE WHEN id = 1 THEN 1 ELSE 0 END
			FROM auth_session;

			DROP TABLE auth_session;
			ALTER TABLE auth_session_new RENAME TO auth_session;

			-- Exactly one active character is preferred; ensure at least one.
			UPDATE auth_session
			   SET is_active = 1
			 WHERE character_id = (
				SELECT character_id
				  FROM auth_session
				 ORDER BY is_active DESC, character_id ASC
				 LIMIT 1
			 )
			   AND NOT EXISTS (
				SELECT 1 FROM auth_session WHERE is_active = 1
			 );

			CREATE UNIQUE INDEX IF NOT EXISTS idx_auth_session_active ON auth_session(is_active) WHERE is_active = 1;

			INSERT OR IGNORE INTO schema_version (version) VALUES (15);
		`)
		if err != nil {
			return fmt.Errorf("migration v15: %w", err)
		}
		logger.Info("DB", "Applied migration v15 (multi-character auth sessions)")
	}

	if version < 16 {
		_, err := d.sql.Exec(`
			CREATE TABLE IF NOT EXISTS config_new (
				user_id TEXT NOT NULL,
				key     TEXT NOT NULL,
				value   TEXT NOT NULL,
				PRIMARY KEY (user_id, key)
			);

			INSERT OR IGNORE INTO config_new (user_id, key, value)
			SELECT 'default', key, value FROM config;

			DROP TABLE config;
			ALTER TABLE config_new RENAME TO config;

			ALTER TABLE watchlist RENAME TO watchlist_legacy;

			CREATE TABLE IF NOT EXISTS watchlist (
				user_id          TEXT NOT NULL,
				type_id          INTEGER NOT NULL,
				type_name        TEXT NOT NULL,
				added_at         TEXT NOT NULL,
				alert_min_margin REAL NOT NULL DEFAULT 0,
				alert_enabled    INTEGER NOT NULL DEFAULT 0,
				alert_metric     TEXT NOT NULL DEFAULT 'margin_percent',
				alert_threshold  REAL NOT NULL DEFAULT 0,
				PRIMARY KEY (user_id, type_id)
			);

			INSERT OR IGNORE INTO watchlist (
				user_id, type_id, type_name, added_at,
				alert_min_margin, alert_enabled, alert_metric, alert_threshold
			)
			SELECT
				'default', type_id, type_name, added_at,
				alert_min_margin, alert_enabled, alert_metric, alert_threshold
			FROM watchlist_legacy;

			CREATE INDEX IF NOT EXISTS idx_watchlist_user_added ON watchlist(user_id, added_at DESC);

			CREATE TABLE IF NOT EXISTS alert_history_new (
				id                  INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id             TEXT NOT NULL,
				watchlist_type_id   INTEGER NOT NULL,
				type_name           TEXT NOT NULL,
				alert_metric        TEXT NOT NULL,
				alert_threshold     REAL NOT NULL,
				current_value       REAL NOT NULL,
				message             TEXT NOT NULL,
				channels_sent       TEXT NOT NULL,
				channels_failed     TEXT,
				sent_at             TEXT NOT NULL,
				scan_id             INTEGER,
				FOREIGN KEY (user_id, watchlist_type_id) REFERENCES watchlist(user_id, type_id) ON DELETE CASCADE
			);

			INSERT OR IGNORE INTO alert_history_new (
				user_id, watchlist_type_id, type_name, alert_metric, alert_threshold,
				current_value, message, channels_sent, channels_failed, sent_at, scan_id
			)
			SELECT
				'default', ah.watchlist_type_id, ah.type_name, ah.alert_metric, ah.alert_threshold,
				ah.current_value, ah.message, ah.channels_sent, ah.channels_failed, ah.sent_at, ah.scan_id
			FROM alert_history ah
			INNER JOIN watchlist_legacy w
				ON w.type_id = ah.watchlist_type_id;

			DROP TABLE alert_history;
			ALTER TABLE alert_history_new RENAME TO alert_history;
			CREATE INDEX IF NOT EXISTS idx_alert_history_user_type ON alert_history(user_id, watchlist_type_id, sent_at DESC);
			CREATE INDEX IF NOT EXISTS idx_alert_history_user_time ON alert_history(user_id, sent_at DESC);
			CREATE INDEX IF NOT EXISTS idx_alert_history_scan ON alert_history(scan_id);

			DROP TABLE watchlist_legacy;

			CREATE TABLE IF NOT EXISTS auth_session_new (
				user_id         TEXT NOT NULL,
				character_id    INTEGER NOT NULL,
				character_name  TEXT NOT NULL,
				access_token    TEXT NOT NULL,
				refresh_token   TEXT NOT NULL,
				expires_at      INTEGER NOT NULL,
				is_active       INTEGER NOT NULL DEFAULT 0,
				PRIMARY KEY (user_id, character_id)
			);

			INSERT OR REPLACE INTO auth_session_new (
				user_id, character_id, character_name, access_token, refresh_token, expires_at, is_active
			)
			SELECT
				'default', character_id, character_name, access_token, refresh_token, expires_at, is_active
			FROM auth_session;

			DROP TABLE auth_session;
			ALTER TABLE auth_session_new RENAME TO auth_session;
			CREATE UNIQUE INDEX IF NOT EXISTS idx_auth_session_active ON auth_session(user_id) WHERE is_active = 1;
			CREATE INDEX IF NOT EXISTS idx_auth_session_user ON auth_session(user_id, character_name, character_id);

			INSERT OR IGNORE INTO schema_version (version) VALUES (16);
		`)
		if err != nil {
			return fmt.Errorf("migration v16: %w", err)
		}
		logger.Info("DB", "Applied migration v16 (user-scoped auth/config/watchlist/alerts)")
	}

	if version < 17 {
		flipCols := []struct {
			name string
			def  string
		}{
			{name: "daily_volume", def: "INTEGER NOT NULL DEFAULT 0"},
			{name: "velocity", def: "REAL NOT NULL DEFAULT 0"},
			{name: "price_trend", def: "REAL NOT NULL DEFAULT 0"},
			{name: "s2b_per_day", def: "REAL NOT NULL DEFAULT 0"},
			{name: "bfs_per_day", def: "REAL NOT NULL DEFAULT 0"},
			{name: "s2b_bfs_ratio", def: "REAL NOT NULL DEFAULT 0"},
			{name: "daily_profit", def: "REAL NOT NULL DEFAULT 0"},
			{name: "real_profit", def: "REAL NOT NULL DEFAULT 0"},
			{name: "real_margin_percent", def: "REAL NOT NULL DEFAULT 0"},
			{name: "filled_qty", def: "INTEGER NOT NULL DEFAULT 0"},
			{name: "can_fill", def: "INTEGER NOT NULL DEFAULT 0"},
			{name: "expected_profit", def: "REAL NOT NULL DEFAULT 0"},
			{name: "expected_buy_price", def: "REAL NOT NULL DEFAULT 0"},
			{name: "expected_sell_price", def: "REAL NOT NULL DEFAULT 0"},
			{name: "slippage_buy_pct", def: "REAL NOT NULL DEFAULT 0"},
			{name: "slippage_sell_pct", def: "REAL NOT NULL DEFAULT 0"},
			{name: "history_available", def: "INTEGER NOT NULL DEFAULT 0"},
		}
		flipResultsExists, err := d.tableExists("flip_results")
		if err != nil {
			return fmt.Errorf("migration v17 check flip_results exists: %w", err)
		}
		if flipResultsExists {
			for _, c := range flipCols {
				if err := d.ensureTableColumn("flip_results", c.name, c.def); err != nil {
					return fmt.Errorf("migration v17 add flip_results.%s: %w", c.name, err)
				}
			}
		}

		stationCols := []struct {
			name string
			def  string
		}{
			{name: "s2b_per_day", def: "REAL NOT NULL DEFAULT 0"},
			{name: "bfs_per_day", def: "REAL NOT NULL DEFAULT 0"},
			{name: "s2b_bfs_ratio", def: "REAL NOT NULL DEFAULT 0"},
			{name: "real_margin_percent", def: "REAL NOT NULL DEFAULT 0"},
			{name: "history_available", def: "INTEGER NOT NULL DEFAULT 0"},
		}
		stationResultsExists, err := d.tableExists("station_results")
		if err != nil {
			return fmt.Errorf("migration v17 check station_results exists: %w", err)
		}
		if stationResultsExists {
			for _, c := range stationCols {
				if err := d.ensureTableColumn("station_results", c.name, c.def); err != nil {
					return fmt.Errorf("migration v17 add station_results.%s: %w", c.name, err)
				}
			}
		}

		if _, err := d.sql.Exec(`INSERT OR IGNORE INTO schema_version (version) VALUES (17);`); err != nil {
			return fmt.Errorf("migration v17: %w", err)
		}
		logger.Info("DB", "Applied migration v17 (scan history liquidity + execution fields)")
	}

	if version < 18 {
		// Extend station_results with full metric fields that were previously
		// computed at scan time but not persisted to the database.
		stationCols := []struct {
			name string
			def  string
		}{
			{name: "profit_per_unit", def: "REAL NOT NULL DEFAULT 0"},
			{name: "total_profit", def: "REAL NOT NULL DEFAULT 0"},
			{name: "roi", def: "REAL NOT NULL DEFAULT 0"},
			{name: "now_roi", def: "REAL NOT NULL DEFAULT 0"},
			{name: "capital_required", def: "REAL NOT NULL DEFAULT 0"},
			{name: "ci", def: "INTEGER NOT NULL DEFAULT 0"},
			{name: "buy_order_count", def: "INTEGER NOT NULL DEFAULT 0"},
			{name: "sell_order_count", def: "INTEGER NOT NULL DEFAULT 0"},
			{name: "buy_units_per_day", def: "REAL NOT NULL DEFAULT 0"},
			{name: "sell_units_per_day", def: "REAL NOT NULL DEFAULT 0"},
			{name: "avg_price", def: "REAL NOT NULL DEFAULT 0"},
			{name: "price_high", def: "REAL NOT NULL DEFAULT 0"},
			{name: "price_low", def: "REAL NOT NULL DEFAULT 0"},
			{name: "confidence_score", def: "REAL NOT NULL DEFAULT 0"},
			{name: "confidence_label", def: "TEXT NOT NULL DEFAULT ''"},
			{name: "has_execution_evidence", def: "INTEGER NOT NULL DEFAULT 0"},
			{name: "is_extreme_price", def: "INTEGER NOT NULL DEFAULT 0"},
			{name: "is_high_risk", def: "INTEGER NOT NULL DEFAULT 0"},
		}
		stationResultsExists, err := d.tableExists("station_results")
		if err != nil {
			return fmt.Errorf("migration v18 check station_results exists: %w", err)
		}
		if stationResultsExists {
			for _, c := range stationCols {
				if err := d.ensureTableColumn("station_results", c.name, c.def); err != nil {
					return fmt.Errorf("migration v18 add station_results.%s: %w", c.name, err)
				}
			}
		}
		if _, err := d.sql.Exec(`INSERT OR IGNORE INTO schema_version (version) VALUES (18);`); err != nil {
			return fmt.Errorf("migration v18: %w", err)
		}
		logger.Info("DB", "Applied migration v18 (station_results full metric persistence)")
	}

	return nil
}

func (d *DB) tableExists(tableName string) (bool, error) {
	var name string
	err := d.sql.QueryRow(
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ? LIMIT 1`,
		tableName,
	).Scan(&name)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (d *DB) ensureTableColumn(tableName, columnName, columnDef string) error {
	rows, err := d.sql.Query("PRAGMA table_info(" + tableName + ")")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, typ string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			return err
		}
		if strings.EqualFold(name, columnName) {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = d.sql.Exec("ALTER TABLE " + tableName + " ADD COLUMN " + columnName + " " + columnDef)
	return err
}

// SqlDB returns the underlying *sql.DB for use by other packages (e.g. auth store).
func (d *DB) SqlDB() *sql.DB {
	return d.sql
}
