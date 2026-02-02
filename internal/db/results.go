package db

import (
	"eve-flipper/internal/engine"
	"log"
)

// InsertFlipResults bulk-inserts flip results linked to a scan history record.
func (d *DB) InsertFlipResults(scanID int64, results []engine.FlipResult) {
	if scanID == 0 || len(results) == 0 {
		return
	}

	tx, err := d.sql.Begin()
	if err != nil {
		log.Printf("[DB] InsertFlipResults begin tx: %v", err)
		return
	}

	stmt, err := tx.Prepare(`INSERT INTO flip_results (
		scan_id, type_id, type_name, volume,
		buy_price, buy_station, buy_system_name, buy_system_id,
		sell_price, sell_station, sell_system_name, sell_system_id,
		profit_per_unit, margin_percent, units_to_buy,
		buy_order_remain, sell_order_remain,
		total_profit, profit_per_jump, buy_jumps, sell_jumps, total_jumps
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		tx.Rollback()
		log.Printf("[DB] InsertFlipResults prepare: %v", err)
		return
	}
	defer stmt.Close()

	for _, r := range results {
		stmt.Exec(
			scanID, r.TypeID, r.TypeName, r.Volume,
			r.BuyPrice, r.BuyStation, r.BuySystemName, r.BuySystemID,
			r.SellPrice, r.SellStation, r.SellSystemName, r.SellSystemID,
			r.ProfitPerUnit, r.MarginPercent, r.UnitsToBuy,
			r.BuyOrderRemain, r.SellOrderRemain,
			r.TotalProfit, r.ProfitPerJump, r.BuyJumps, r.SellJumps, r.TotalJumps,
		)
	}

	if err := tx.Commit(); err != nil {
		log.Printf("[DB] InsertFlipResults commit: %v", err)
	}
}

// InsertContractResults bulk-inserts contract results linked to a scan history record.
func (d *DB) InsertContractResults(scanID int64, results []engine.ContractResult) {
	if scanID == 0 || len(results) == 0 {
		return
	}

	tx, err := d.sql.Begin()
	if err != nil {
		log.Printf("[DB] InsertContractResults begin tx: %v", err)
		return
	}

	stmt, err := tx.Prepare(`INSERT INTO contract_results (
		scan_id, contract_id, title, price, market_value,
		profit, margin_percent, volume, station_name,
		item_count, jumps, profit_per_jump
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		tx.Rollback()
		log.Printf("[DB] InsertContractResults prepare: %v", err)
		return
	}
	defer stmt.Close()

	for _, r := range results {
		stmt.Exec(
			scanID, r.ContractID, r.Title, r.Price, r.MarketValue,
			r.Profit, r.MarginPercent, r.Volume, r.StationName,
			r.ItemCount, r.Jumps, r.ProfitPerJump,
		)
	}

	if err := tx.Commit(); err != nil {
		log.Printf("[DB] InsertContractResults commit: %v", err)
	}
}
