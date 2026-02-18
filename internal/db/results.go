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
		total_profit, profit_per_jump, buy_jumps, sell_jumps, total_jumps,
		daily_volume, velocity, price_trend,
		s2b_per_day, bfs_per_day, s2b_bfs_ratio,
		daily_profit, real_profit, real_margin_percent, filled_qty, can_fill,
		expected_profit, expected_buy_price, expected_sell_price,
		slippage_buy_pct, slippage_sell_pct, history_available
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		tx.Rollback()
		log.Printf("[DB] InsertFlipResults prepare: %v", err)
		return
	}
	defer stmt.Close()

	for _, r := range results {
		canFill := 0
		if r.CanFill {
			canFill = 1
		}
		historyAvailable := 0
		if r.HistoryAvailable {
			historyAvailable = 1
		}
		if _, err := stmt.Exec(
			scanID, r.TypeID, r.TypeName, r.Volume,
			r.BuyPrice, r.BuyStation, r.BuySystemName, r.BuySystemID,
			r.SellPrice, r.SellStation, r.SellSystemName, r.SellSystemID,
			r.ProfitPerUnit, r.MarginPercent, r.UnitsToBuy,
			r.BuyOrderRemain, r.SellOrderRemain,
			r.TotalProfit, r.ProfitPerJump, r.BuyJumps, r.SellJumps, r.TotalJumps,
			r.DailyVolume, r.Velocity, r.PriceTrend,
			r.S2BPerDay, r.BfSPerDay, r.S2BBfSRatio,
			r.DailyProfit, r.RealProfit, r.RealMarginPercent, r.FilledQty, canFill,
			r.ExpectedProfit, r.ExpectedBuyPrice, r.ExpectedSellPrice,
			r.SlippageBuyPct, r.SlippageSellPct, historyAvailable,
		); err != nil {
			tx.Rollback()
			log.Printf("[DB] InsertFlipResults exec row type_id=%d: %v", r.TypeID, err)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		log.Printf("[DB] InsertFlipResults commit: %v", err)
	}
}

// GetFlipResults retrieves flip results for a scan.
func (d *DB) GetFlipResults(scanID int64) []engine.FlipResult {
	rows, err := d.sql.Query(`
		SELECT type_id, type_name, volume,
			buy_price, buy_station, buy_system_name, buy_system_id,
			sell_price, sell_station, sell_system_name, sell_system_id,
			profit_per_unit, margin_percent, units_to_buy,
			buy_order_remain, sell_order_remain,
			total_profit, profit_per_jump, buy_jumps, sell_jumps, total_jumps,
			daily_volume, velocity, price_trend,
			s2b_per_day, bfs_per_day, s2b_bfs_ratio,
			daily_profit, real_profit, real_margin_percent, filled_qty, can_fill,
			expected_profit, expected_buy_price, expected_sell_price,
			slippage_buy_pct, slippage_sell_pct, history_available
		FROM flip_results WHERE scan_id = ?
	`, scanID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var results []engine.FlipResult
	for rows.Next() {
		var r engine.FlipResult
		var canFill int
		var historyAvailable int
		if err := rows.Scan(
			&r.TypeID, &r.TypeName, &r.Volume,
			&r.BuyPrice, &r.BuyStation, &r.BuySystemName, &r.BuySystemID,
			&r.SellPrice, &r.SellStation, &r.SellSystemName, &r.SellSystemID,
			&r.ProfitPerUnit, &r.MarginPercent, &r.UnitsToBuy,
			&r.BuyOrderRemain, &r.SellOrderRemain,
			&r.TotalProfit, &r.ProfitPerJump, &r.BuyJumps, &r.SellJumps, &r.TotalJumps,
			&r.DailyVolume, &r.Velocity, &r.PriceTrend,
			&r.S2BPerDay, &r.BfSPerDay, &r.S2BBfSRatio,
			&r.DailyProfit, &r.RealProfit, &r.RealMarginPercent, &r.FilledQty, &canFill,
			&r.ExpectedProfit, &r.ExpectedBuyPrice, &r.ExpectedSellPrice,
			&r.SlippageBuyPct, &r.SlippageSellPct, &historyAvailable,
		); err != nil {
			log.Printf("[DB] GetFlipResults scan row: %v", err)
			continue
		}
		r.CanFill = canFill != 0
		r.HistoryAvailable = historyAvailable != 0
		results = append(results, r)
	}
	return results
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
		profit, margin_percent, expected_profit, expected_margin_percent,
		sell_confidence, est_liquidation_days, conservative_value, carry_cost,
		volume, station_name,
		item_count, jumps, profit_per_jump
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		tx.Rollback()
		log.Printf("[DB] InsertContractResults prepare: %v", err)
		return
	}
	defer stmt.Close()

	for _, r := range results {
		if _, err := stmt.Exec(
			scanID, r.ContractID, r.Title, r.Price, r.MarketValue,
			r.Profit, r.MarginPercent, r.ExpectedProfit, r.ExpectedMarginPercent,
			r.SellConfidence, r.EstLiquidationDays, r.ConservativeValue, r.CarryCost,
			r.Volume, r.StationName,
			r.ItemCount, r.Jumps, r.ProfitPerJump,
		); err != nil {
			tx.Rollback()
			log.Printf("[DB] InsertContractResults exec contract_id=%d: %v", r.ContractID, err)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		log.Printf("[DB] InsertContractResults commit: %v", err)
	}
}

// GetContractResults retrieves contract results for a scan.
func (d *DB) GetContractResults(scanID int64) []engine.ContractResult {
	rows, err := d.sql.Query(`
		SELECT contract_id, title, price, market_value,
			profit, margin_percent, expected_profit, expected_margin_percent,
			sell_confidence, est_liquidation_days, conservative_value, carry_cost,
			volume, station_name,
			item_count, jumps, profit_per_jump
		FROM contract_results WHERE scan_id = ?
	`, scanID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var results []engine.ContractResult
	for rows.Next() {
		var r engine.ContractResult
		if err := rows.Scan(
			&r.ContractID, &r.Title, &r.Price, &r.MarketValue,
			&r.Profit, &r.MarginPercent, &r.ExpectedProfit, &r.ExpectedMarginPercent,
			&r.SellConfidence, &r.EstLiquidationDays, &r.ConservativeValue, &r.CarryCost,
			&r.Volume, &r.StationName,
			&r.ItemCount, &r.Jumps, &r.ProfitPerJump,
		); err != nil {
			log.Printf("[DB] GetContractResults scan row: %v", err)
			continue
		}
		results = append(results, r)
	}
	return results
}

// InsertStationResults bulk-inserts station trading results.
func (d *DB) InsertStationResults(scanID int64, results []engine.StationTrade) {
	if scanID == 0 || len(results) == 0 {
		return
	}

	tx, err := d.sql.Begin()
	if err != nil {
		log.Printf("[DB] InsertStationResults begin tx: %v", err)
		return
	}

	stmt, err := tx.Prepare(`INSERT INTO station_results (
		scan_id, type_id, type_name, buy_price, sell_price,
		margin, margin_pct, volume, buy_volume, sell_volume,
		station_id, station_name, cts, sds, period_roi,
		vwap, pvi, obds, bvs_ratio, dos,
		s2b_per_day, bfs_per_day, s2b_bfs_ratio, real_margin_percent, history_available,
		daily_profit, real_profit, filled_qty, can_fill,
		expected_profit, expected_buy_price, expected_sell_price,
		slippage_buy_pct, slippage_sell_pct,
		profit_per_unit, total_profit, roi, now_roi, capital_required,
		ci, buy_order_count, sell_order_count,
		buy_units_per_day, sell_units_per_day,
		avg_price, price_high, price_low,
		confidence_score, confidence_label, has_execution_evidence,
		is_extreme_price, is_high_risk
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		tx.Rollback()
		log.Printf("[DB] InsertStationResults prepare: %v", err)
		return
	}
	defer stmt.Close()

	for _, r := range results {
		canFill := 0
		if r.CanFill {
			canFill = 1
		}
		historyAvailable := 0
		if r.HistoryAvailable {
			historyAvailable = 1
		}
		hasExecEvidence := 0
		if r.HasExecutionEvidence {
			hasExecEvidence = 1
		}
		isExtremePrice := 0
		if r.IsExtremePriceFlag {
			isExtremePrice = 1
		}
		isHighRisk := 0
		if r.IsHighRiskFlag {
			isHighRisk = 1
		}
		if _, err := stmt.Exec(
			scanID, r.TypeID, r.TypeName, r.BuyPrice, r.SellPrice,
			r.Spread, r.MarginPercent, r.DailyVolume, r.BuyVolume, r.SellVolume,
			r.StationID, r.StationName, r.CTS, r.SDS, r.PeriodROI,
			r.VWAP, r.PVI, r.OBDS, r.BvSRatio, r.DOS,
			r.S2BPerDay, r.BfSPerDay, r.S2BBfSRatio, r.RealMarginPercent, historyAvailable,
			r.DailyProfit, r.RealProfit, r.FilledQty, canFill,
			r.ExpectedProfit, r.ExpectedBuyPrice, r.ExpectedSellPrice,
			r.SlippageBuyPct, r.SlippageSellPct,
			r.ProfitPerUnit, r.TotalProfit, r.ROI, r.NowROI, r.CapitalRequired,
			r.CI, r.BuyOrderCount, r.SellOrderCount,
			r.BuyUnitsPerDay, r.SellUnitsPerDay,
			r.AvgPrice, r.PriceHigh, r.PriceLow,
			r.ConfidenceScore, r.ConfidenceLabel, hasExecEvidence,
			isExtremePrice, isHighRisk,
		); err != nil {
			tx.Rollback()
			log.Printf("[DB] InsertStationResults exec type_id=%d: %v", r.TypeID, err)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		log.Printf("[DB] InsertStationResults commit: %v", err)
	}
}

// GetStationResults retrieves station trading results for a scan.
func (d *DB) GetStationResults(scanID int64) []engine.StationTrade {
	rows, err := d.sql.Query(`
		SELECT type_id, type_name, buy_price, sell_price,
			margin, margin_pct, volume, buy_volume, sell_volume,
			station_id, station_name, cts, sds, period_roi,
			vwap, pvi, obds, bvs_ratio, dos,
			s2b_per_day, bfs_per_day, s2b_bfs_ratio, real_margin_percent, history_available,
			daily_profit, real_profit, filled_qty, can_fill,
			expected_profit, expected_buy_price, expected_sell_price,
			slippage_buy_pct, slippage_sell_pct,
			COALESCE(profit_per_unit, 0), COALESCE(total_profit, 0),
			COALESCE(roi, 0), COALESCE(now_roi, 0), COALESCE(capital_required, 0),
			COALESCE(ci, 0), COALESCE(buy_order_count, 0), COALESCE(sell_order_count, 0),
			COALESCE(buy_units_per_day, 0), COALESCE(sell_units_per_day, 0),
			COALESCE(avg_price, 0), COALESCE(price_high, 0), COALESCE(price_low, 0),
			COALESCE(confidence_score, 0), COALESCE(confidence_label, ''),
			COALESCE(has_execution_evidence, 0),
			COALESCE(is_extreme_price, 0), COALESCE(is_high_risk, 0)
		FROM station_results WHERE scan_id = ?
	`, scanID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var results []engine.StationTrade
	for rows.Next() {
		var r engine.StationTrade
		var canFill, historyAvailable, hasExecEvidence, isExtremePrice, isHighRisk int
		if err := rows.Scan(
			&r.TypeID, &r.TypeName, &r.BuyPrice, &r.SellPrice,
			&r.Spread, &r.MarginPercent, &r.DailyVolume, &r.BuyVolume, &r.SellVolume,
			&r.StationID, &r.StationName, &r.CTS, &r.SDS, &r.PeriodROI,
			&r.VWAP, &r.PVI, &r.OBDS, &r.BvSRatio, &r.DOS,
			&r.S2BPerDay, &r.BfSPerDay, &r.S2BBfSRatio, &r.RealMarginPercent, &historyAvailable,
			&r.DailyProfit, &r.RealProfit, &r.FilledQty, &canFill,
			&r.ExpectedProfit, &r.ExpectedBuyPrice, &r.ExpectedSellPrice,
			&r.SlippageBuyPct, &r.SlippageSellPct,
			&r.ProfitPerUnit, &r.TotalProfit, &r.ROI, &r.NowROI, &r.CapitalRequired,
			&r.CI, &r.BuyOrderCount, &r.SellOrderCount,
			&r.BuyUnitsPerDay, &r.SellUnitsPerDay,
			&r.AvgPrice, &r.PriceHigh, &r.PriceLow,
			&r.ConfidenceScore, &r.ConfidenceLabel,
			&hasExecEvidence,
			&isExtremePrice, &isHighRisk,
		); err != nil {
			log.Printf("[DB] GetStationResults scan row: %v", err)
			continue
		}
		r.CanFill = canFill != 0
		r.HistoryAvailable = historyAvailable != 0
		r.HasExecutionEvidence = hasExecEvidence != 0
		r.IsExtremePriceFlag = isExtremePrice != 0
		r.IsHighRiskFlag = isHighRisk != 0
		results = append(results, r)
	}
	return results
}

// InsertRouteResults bulk-inserts route results linked to a scan history record.
func (d *DB) InsertRouteResults(scanID int64, routes []engine.RouteResult) {
	if scanID == 0 || len(routes) == 0 {
		return
	}

	tx, err := d.sql.Begin()
	if err != nil {
		log.Printf("[DB] InsertRouteResults begin tx: %v", err)
		return
	}

	stmt, err := tx.Prepare(`INSERT INTO route_results (
		scan_id, route_index, hop_index,
		system_name, station_name, dest_system_name, dest_station_name,
		type_name, type_id, buy_price, sell_price, units, profit, jumps,
		total_profit, total_jumps, profit_per_jump, hop_count
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		tx.Rollback()
		log.Printf("[DB] InsertRouteResults prepare: %v", err)
		return
	}
	defer stmt.Close()

	for ri, route := range routes {
		for hi, hop := range route.Hops {
			if _, err := stmt.Exec(
				scanID, ri, hi,
				hop.SystemName, hop.StationName, hop.DestSystemName, hop.DestStationName,
				hop.TypeName, hop.TypeID, hop.BuyPrice, hop.SellPrice, hop.Units, hop.Profit, hop.Jumps,
				route.TotalProfit, route.TotalJumps, route.ProfitPerJump, route.HopCount,
			); err != nil {
				tx.Rollback()
				log.Printf("[DB] InsertRouteResults exec route=%d hop=%d: %v", ri, hi, err)
				return
			}
		}
	}

	if err := tx.Commit(); err != nil {
		log.Printf("[DB] InsertRouteResults commit: %v", err)
	}
}

// GetRouteResults retrieves route results for a scan and reconstructs RouteResult slices.
func (d *DB) GetRouteResults(scanID int64) []engine.RouteResult {
	rows, err := d.sql.Query(`
		SELECT route_index, hop_index,
			system_name, station_name, dest_system_name, dest_station_name,
			type_name, type_id, buy_price, sell_price, units, profit, jumps,
			total_profit, total_jumps, profit_per_jump, hop_count
		FROM route_results WHERE scan_id = ? ORDER BY route_index, hop_index
	`, scanID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	routeMap := make(map[int]*engine.RouteResult)
	var maxIdx int
	for rows.Next() {
		var ri, hi int
		var hop engine.RouteHop
		var totalProfit, profitPerJump float64
		var totalJumps, hopCount int
		if err := rows.Scan(
			&ri, &hi,
			&hop.SystemName, &hop.StationName, &hop.DestSystemName, &hop.DestStationName,
			&hop.TypeName, &hop.TypeID, &hop.BuyPrice, &hop.SellPrice, &hop.Units, &hop.Profit, &hop.Jumps,
			&totalProfit, &totalJumps, &profitPerJump, &hopCount,
		); err != nil {
			log.Printf("[DB] GetRouteResults scan row: %v", err)
			continue
		}
		if _, ok := routeMap[ri]; !ok {
			routeMap[ri] = &engine.RouteResult{
				TotalProfit:   totalProfit,
				TotalJumps:    totalJumps,
				ProfitPerJump: profitPerJump,
				HopCount:      hopCount,
			}
		}
		routeMap[ri].Hops = append(routeMap[ri].Hops, hop)
		if ri > maxIdx {
			maxIdx = ri
		}
	}

	results := make([]engine.RouteResult, 0, len(routeMap))
	for i := 0; i <= maxIdx; i++ {
		if r, ok := routeMap[i]; ok {
			results = append(results, *r)
		}
	}
	return results
}
