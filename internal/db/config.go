package db

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"eve-flipper/internal/config"
)

// LoadConfig reads config from SQLite. If empty, returns defaults.
func (d *DB) LoadConfig() *config.Config {
	cfg := config.Default()

	rows, err := d.sql.Query("SELECT key, value FROM config")
	if err != nil {
		return cfg
	}
	defer rows.Close()

	m := make(map[string]string)
	for rows.Next() {
		var k, v string
		rows.Scan(&k, &v)
		m[k] = v
	}

	if len(m) == 0 {
		return cfg
	}

	if v, ok := m["system_name"]; ok {
		cfg.SystemName = v
	}
	if v, ok := m["cargo_capacity"]; ok {
		cfg.CargoCapacity, _ = strconv.ParseFloat(v, 64)
	}
	if v, ok := m["buy_radius"]; ok {
		cfg.BuyRadius, _ = strconv.Atoi(v)
	}
	if v, ok := m["sell_radius"]; ok {
		cfg.SellRadius, _ = strconv.Atoi(v)
	}
	if v, ok := m["min_margin"]; ok {
		cfg.MinMargin, _ = strconv.ParseFloat(v, 64)
	}
	if v, ok := m["sales_tax_percent"]; ok {
		cfg.SalesTaxPercent, _ = strconv.ParseFloat(v, 64)
	}
	if v, ok := m["opacity"]; ok {
		cfg.Opacity, _ = strconv.Atoi(v)
	}
	if v, ok := m["window_x"]; ok {
		cfg.WindowX, _ = strconv.Atoi(v)
	}
	if v, ok := m["window_y"]; ok {
		cfg.WindowY, _ = strconv.Atoi(v)
	}
	if v, ok := m["window_w"]; ok {
		cfg.WindowW, _ = strconv.Atoi(v)
	}
	if v, ok := m["window_h"]; ok {
		cfg.WindowH, _ = strconv.Atoi(v)
	}

	return cfg
}

// SaveConfig writes config to SQLite (upsert all fields).
func (d *DB) SaveConfig(cfg *config.Config) error {
	pairs := map[string]string{
		"system_name":       cfg.SystemName,
		"cargo_capacity":    fmt.Sprintf("%g", cfg.CargoCapacity),
		"buy_radius":        strconv.Itoa(cfg.BuyRadius),
		"sell_radius":       strconv.Itoa(cfg.SellRadius),
		"min_margin":        fmt.Sprintf("%g", cfg.MinMargin),
		"sales_tax_percent": fmt.Sprintf("%g", cfg.SalesTaxPercent),
		"opacity":           strconv.Itoa(cfg.Opacity),
		"window_x":          strconv.Itoa(cfg.WindowX),
		"window_y":          strconv.Itoa(cfg.WindowY),
		"window_w":          strconv.Itoa(cfg.WindowW),
		"window_h":          strconv.Itoa(cfg.WindowH),
	}

	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare("INSERT OR REPLACE INTO config (key, value) VALUES (?, ?)")
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	for k, v := range pairs {
		if _, err := stmt.Exec(k, v); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// MigrateFromJSON checks for config.json and imports it into SQLite.
func (d *DB) MigrateFromJSON() {
	wd, _ := os.Getwd()
	jsonPath := filepath.Join(wd, "config.json")

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return // no config.json, nothing to migrate
	}

	// Check if config table already has data
	var count int
	d.sql.QueryRow("SELECT COUNT(*) FROM config").Scan(&count)
	if count > 0 {
		// Already migrated, just rename the file
		os.Rename(jsonPath, jsonPath+".bak")
		return
	}

	log.Println("[DB] Migrating config.json → SQLite...")

	// Parse the old config
	var old struct {
		SystemName      string                 `json:"system_name"`
		CargoCapacity   float64                `json:"cargo_capacity"`
		BuyRadius       int                    `json:"buy_radius"`
		SellRadius      int                    `json:"sell_radius"`
		MinMargin       float64                `json:"min_margin"`
		SalesTaxPercent float64                `json:"sales_tax_percent"`
		Opacity         int                    `json:"opacity"`
		WindowX         int                    `json:"window_x"`
		WindowY         int                    `json:"window_y"`
		WindowW         int                    `json:"window_w"`
		WindowH         int                    `json:"window_h"`
		Watchlist       []config.WatchlistItem `json:"watchlist"`
	}
	if err := json.Unmarshal(data, &old); err != nil {
		log.Printf("[DB] Failed to parse config.json: %v", err)
		return
	}

	// Save config
	cfg := config.Default()
	cfg.SystemName = old.SystemName
	cfg.CargoCapacity = old.CargoCapacity
	cfg.BuyRadius = old.BuyRadius
	cfg.SellRadius = old.SellRadius
	cfg.MinMargin = old.MinMargin
	cfg.SalesTaxPercent = old.SalesTaxPercent
	cfg.Opacity = old.Opacity
	cfg.WindowX = old.WindowX
	cfg.WindowY = old.WindowY
	cfg.WindowW = old.WindowW
	cfg.WindowH = old.WindowH
	d.SaveConfig(cfg)

	// Migrate watchlist
	for _, item := range old.Watchlist {
		d.AddWatchlistItem(item)
	}

	// Rename old file
	os.Rename(jsonPath, jsonPath+".bak")
	log.Printf("[DB] Migrated config.json → SQLite (%d watchlist items)", len(old.Watchlist))
}
