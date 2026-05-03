package engine

import (
	"math"
	"sort"
	"time"

	"eve-flipper/internal/esi"
)

// PortfolioPnL is the full P&L analytics response for the character popup.
// It now includes a realized FIFO ledger and open inventory positions.
type PortfolioPnL struct {
	DailyPnL      []DailyPnLEntry   `json:"daily_pnl"`
	Summary       PortfolioPnLStats `json:"summary"`
	TopItems      []ItemPnL         `json:"top_items"`
	TopStations   []StationPnL      `json:"top_stations"`
	Ledger        []RealizedTrade   `json:"ledger"`
	OpenPositions []OpenPosition    `json:"open_positions"`
	Coverage      MatchingCoverage  `json:"coverage"`
	Settings      PortfolioSettings `json:"settings"`
}

// PortfolioPnLOptions controls realized P&L matching behavior.
type PortfolioPnLOptions struct {
	LookbackDays         int
	SalesTaxPercent      float64
	BrokerFeePercent     float64
	LedgerLimit          int
	IncludeUnmatchedSell bool // legacy mode: treat unmatched sells as zero-cost proceeds
}

// PortfolioSettings is echoed back in API responses for traceability.
type PortfolioSettings struct {
	LookbackDays         int     `json:"lookback_days"`
	SalesTaxPercent      float64 `json:"sales_tax_percent"`
	BrokerFeePercent     float64 `json:"broker_fee_percent"`
	LedgerLimit          int     `json:"ledger_limit"`
	IncludeUnmatchedSell bool    `json:"include_unmatched_sell"`
}

// MatchingCoverage describes how much sell flow had known cost basis.
type MatchingCoverage struct {
	TotalSellQty       int64   `json:"total_sell_qty"`
	MatchedSellQty     int64   `json:"matched_sell_qty"`
	UnmatchedSellQty   int64   `json:"unmatched_sell_qty"`
	TotalSellValue     float64 `json:"total_sell_value"`
	MatchedSellValue   float64 `json:"matched_sell_value"`
	UnmatchedSellValue float64 `json:"unmatched_sell_value"`
	MatchRateQtyPct    float64 `json:"match_rate_qty_pct"`
	MatchRateValuePct  float64 `json:"match_rate_value_pct"`
}

// RealizedTrade is one FIFO-matched realized trade leg.
type RealizedTrade struct {
	TypeID            int32   `json:"type_id"`
	TypeName          string  `json:"type_name"`
	Quantity          int32   `json:"quantity"`
	BuyTransactionID  int64   `json:"buy_transaction_id"`
	SellTransactionID int64   `json:"sell_transaction_id"`
	BuyDate           string  `json:"buy_date"`
	SellDate          string  `json:"sell_date"`
	HoldingDays       int     `json:"holding_days"`
	BuyLocationID     int64   `json:"buy_location_id"`
	BuyLocationName   string  `json:"buy_location_name"`
	SellLocationID    int64   `json:"sell_location_id"`
	SellLocationName  string  `json:"sell_location_name"`
	BuyUnitPrice      float64 `json:"buy_unit_price"`
	SellUnitPrice     float64 `json:"sell_unit_price"`
	BuyGross          float64 `json:"buy_gross"`
	SellGross         float64 `json:"sell_gross"`
	BuyFee            float64 `json:"buy_fee"`
	SellBrokerFee     float64 `json:"sell_broker_fee"`
	SellTax           float64 `json:"sell_tax"`
	BuyTotal          float64 `json:"buy_total"`
	SellTotal         float64 `json:"sell_total"`
	RealizedPnL       float64 `json:"realized_pnl"`
	MarginPercent     float64 `json:"margin_percent"`
	Unmatched         bool    `json:"unmatched,omitempty"`
}

// OpenPosition is remaining unmatched inventory from FIFO queues.
type OpenPosition struct {
	TypeID        int32   `json:"type_id"`
	TypeName      string  `json:"type_name"`
	LocationID    int64   `json:"location_id"`
	LocationName  string  `json:"location_name"`
	Quantity      int64   `json:"quantity"`
	AvgCost       float64 `json:"avg_cost"`
	CostBasis     float64 `json:"cost_basis"`
	OldestLotDate string  `json:"oldest_lot_date"`
}

// DailyPnLEntry represents one day's realized trading activity.
type DailyPnLEntry struct {
	Date          string  `json:"date"` // YYYY-MM-DD
	BuyTotal      float64 `json:"buy_total"`
	SellTotal     float64 `json:"sell_total"`
	NetPnL        float64 `json:"net_pnl"`
	CumulativePnL float64 `json:"cumulative_pnl"`
	DrawdownPct   float64 `json:"drawdown_pct"` // drawdown from cumulative peak (0 to -100)
	Transactions  int     `json:"transactions"`
}

// PortfolioPnLStats is the aggregated summary across the period.
type PortfolioPnLStats struct {
	TotalPnL       float64 `json:"total_pnl"`
	AvgDailyPnL    float64 `json:"avg_daily_pnl"`
	BestDayPnL     float64 `json:"best_day_pnl"`
	BestDayDate    string  `json:"best_day_date"`
	WorstDayPnL    float64 `json:"worst_day_pnl"`
	WorstDayDate   string  `json:"worst_day_date"`
	ProfitableDays int     `json:"profitable_days"`
	LosingDays     int     `json:"losing_days"`
	TotalDays      int     `json:"total_days"`
	WinRate        float64 `json:"win_rate"` // 0-100%
	TotalBought    float64 `json:"total_bought"`
	TotalSold      float64 `json:"total_sold"`
	ROIPercent     float64 `json:"roi_percent"`

	// Enhanced analytics
	SharpeRatio        float64 `json:"sharpe_ratio"`         // annualized: mean/std * sqrt(365)
	MaxDrawdownPct     float64 `json:"max_drawdown_pct"`     // deepest cumulative drawdown %
	MaxDrawdownISK     float64 `json:"max_drawdown_isk"`     // deepest drawdown in ISK
	MaxDrawdownDays    int     `json:"max_drawdown_days"`    // duration from peak to trough
	CalmarRatio        float64 `json:"calmar_ratio"`         // annualized return / max drawdown
	ProfitFactor       float64 `json:"profit_factor"`        // gross profit / gross loss
	AvgWin             float64 `json:"avg_win"`              // average winning day ISK
	AvgLoss            float64 `json:"avg_loss"`             // average losing day ISK
	ExpectancyPerTrade float64 `json:"expectancy_per_trade"` // (win_rate * avg_win) - (loss_rate * avg_loss)

	// Ledger quality / inventory stats
	RealizedTrades   int     `json:"realized_trades"`
	RealizedQuantity int64   `json:"realized_quantity"`
	OpenPositions    int     `json:"open_positions"`
	OpenCostBasis    float64 `json:"open_cost_basis"`
	TotalFees        float64 `json:"total_fees"`
	TotalTaxes       float64 `json:"total_taxes"`
}

// StationPnL is a per-station breakdown of trading activity.
type StationPnL struct {
	LocationID   int64   `json:"location_id"`
	LocationName string  `json:"location_name"`
	TotalBought  float64 `json:"total_bought"`
	TotalSold    float64 `json:"total_sold"`
	NetPnL       float64 `json:"net_pnl"`
	Transactions int     `json:"transactions"`
}

// ItemPnL is the per-item breakdown of trading activity.
type ItemPnL struct {
	TypeID        int32   `json:"type_id"`
	TypeName      string  `json:"type_name"`
	TotalBought   float64 `json:"total_bought"`
	TotalSold     float64 `json:"total_sold"`
	NetPnL        float64 `json:"net_pnl"`
	QtyBought     int64   `json:"qty_bought"`
	QtySold       int64   `json:"qty_sold"`
	AvgBuyPrice   float64 `json:"avg_buy_price"`
	AvgSellPrice  float64 `json:"avg_sell_price"`
	MarginPercent float64 `json:"margin_percent"`
	Transactions  int     `json:"transactions"`
}

type portfolioTx struct {
	tx esi.WalletTransaction
	t  time.Time
}

type portfolioBuyLot struct {
	TransactionID int64
	Date          time.Time
	TypeID        int32
	TypeName      string
	LocationID    int64
	LocationName  string
	UnitPrice     float64
	Remaining     int32
}

func normalizePortfolioOptions(opt PortfolioPnLOptions) PortfolioPnLOptions {
	if opt.LookbackDays <= 0 {
		opt.LookbackDays = 30
	}
	if opt.LookbackDays > 365 {
		opt.LookbackDays = 365
	}
	if opt.SalesTaxPercent < 0 {
		opt.SalesTaxPercent = 0
	}
	if opt.SalesTaxPercent > 100 {
		opt.SalesTaxPercent = 100
	}
	if opt.BrokerFeePercent < 0 {
		opt.BrokerFeePercent = 0
	}
	if opt.BrokerFeePercent > 100 {
		opt.BrokerFeePercent = 100
	}
	if opt.LedgerLimit == 0 {
		opt.LedgerLimit = 500
	}
	if opt.LedgerLimit < 0 {
		opt.LedgerLimit = 0 // unlimited
	}
	return opt
}

// ComputePortfolioPnL keeps legacy signature for callers/tests.
// It defaults to zero fees and strict realized matching. Sells without known
// cost basis are reported in coverage but excluded from P&L.
func ComputePortfolioPnL(txns []esi.WalletTransaction, lookbackDays int) *PortfolioPnL {
	return ComputePortfolioPnLWithOptions(txns, PortfolioPnLOptions{
		LookbackDays:         lookbackDays,
		SalesTaxPercent:      0,
		BrokerFeePercent:     0,
		LedgerLimit:          500,
		IncludeUnmatchedSell: false,
	})
}

// ComputePortfolioPnLWithOptions computes realized P&L using FIFO matching.
func ComputePortfolioPnLWithOptions(txns []esi.WalletTransaction, opt PortfolioPnLOptions) *PortfolioPnL {
	opt = normalizePortfolioOptions(opt)
	out := &PortfolioPnL{
		DailyPnL:      []DailyPnLEntry{},
		TopItems:      []ItemPnL{},
		TopStations:   []StationPnL{},
		Ledger:        []RealizedTrade{},
		OpenPositions: []OpenPosition{},
		Settings: PortfolioSettings{
			LookbackDays:         opt.LookbackDays,
			SalesTaxPercent:      opt.SalesTaxPercent,
			BrokerFeePercent:     opt.BrokerFeePercent,
			LedgerLimit:          opt.LedgerLimit,
			IncludeUnmatchedSell: opt.IncludeUnmatchedSell,
		},
	}
	if len(txns) == 0 {
		return out
	}

	now := time.Now().UTC()
	cutoff := now.AddDate(0, 0, -opt.LookbackDays)

	parsed := make([]portfolioTx, 0, len(txns))
	for _, tx := range txns {
		t, err := time.Parse(time.RFC3339, tx.Date)
		if err != nil {
			continue
		}
		parsed = append(parsed, portfolioTx{tx: tx, t: t})
	}
	if len(parsed) == 0 {
		return out
	}

	sort.Slice(parsed, func(i, j int) bool {
		if parsed[i].t.Equal(parsed[j].t) {
			return parsed[i].tx.TransactionID < parsed[j].tx.TransactionID
		}
		return parsed[i].t.Before(parsed[j].t)
	})

	type dayKey string
	dayMap := make(map[dayKey]*DailyPnLEntry)
	itemMap := make(map[int32]*ItemPnL)
	stationMap := make(map[int64]*StationPnL)
	buyQueues := make(map[int32][]portfolioBuyLot)
	ledgerCap := len(parsed)
	if opt.LedgerLimit > 0 {
		ledgerCap = minInt(len(parsed), opt.LedgerLimit)
	}
	ledger := make([]RealizedTrade, 0, ledgerCap)
	coverage := MatchingCoverage{}
	summary := PortfolioPnLStats{}

	addDay := func(date string) *DailyPnLEntry {
		dk := dayKey(date)
		entry, ok := dayMap[dk]
		if !ok {
			entry = &DailyPnLEntry{Date: date}
			dayMap[dk] = entry
		}
		return entry
	}
	addItem := func(typeID int32, typeName string) *ItemPnL {
		item, ok := itemMap[typeID]
		if !ok {
			item = &ItemPnL{TypeID: typeID, TypeName: typeName}
			itemMap[typeID] = item
		}
		return item
	}
	addStation := func(locationID int64, name string) *StationPnL {
		st, ok := stationMap[locationID]
		if !ok {
			st = &StationPnL{
				LocationID:   locationID,
				LocationName: name,
			}
			stationMap[locationID] = st
		}
		if st.LocationName == "" && name != "" {
			st.LocationName = name
		}
		return st
	}

	for _, rec := range parsed {
		tx := rec.tx
		inLookback := !rec.t.Before(cutoff)

		if tx.IsBuy {
			buyQueues[tx.TypeID] = append(buyQueues[tx.TypeID], portfolioBuyLot{
				TransactionID: tx.TransactionID,
				Date:          rec.t,
				TypeID:        tx.TypeID,
				TypeName:      tx.TypeName,
				LocationID:    tx.LocationID,
				LocationName:  tx.LocationName,
				UnitPrice:     tx.UnitPrice,
				Remaining:     tx.Quantity,
			})
			continue
		}

		queue := buyQueues[tx.TypeID]
		remaining := tx.Quantity
		if inLookback {
			sellGrossTotal := tx.UnitPrice * float64(tx.Quantity)
			coverage.TotalSellQty += int64(tx.Quantity)
			coverage.TotalSellValue += sellGrossTotal
		}

		for remaining > 0 && len(queue) > 0 {
			lot := &queue[0]
			matched := lot.Remaining
			if matched > remaining {
				matched = remaining
			}

			lot.Remaining -= matched
			remaining -= matched
			if lot.Remaining <= 0 {
				queue = queue[1:]
			}

			if !inLookback {
				continue
			}

			buyGross := lot.UnitPrice * float64(matched)
			buyFee := buyGross * opt.BrokerFeePercent / 100.0
			buyTotal := buyGross + buyFee

			sellGross := tx.UnitPrice * float64(matched)
			sellBrokerFee := sellGross * opt.BrokerFeePercent / 100.0
			sellTax := sellGross * opt.SalesTaxPercent / 100.0
			sellTotal := sellGross - sellBrokerFee - sellTax

			pnl := sellTotal - buyTotal
			margin := 0.0
			if buyTotal > 0 {
				margin = pnl / buyTotal * 100
			}

			holdingDays := int(rec.t.Sub(lot.Date).Hours() / 24)
			if holdingDays < 0 {
				holdingDays = 0
			}

			day := addDay(rec.t.Format("2006-01-02"))
			day.BuyTotal += buyTotal
			day.SellTotal += sellTotal
			day.Transactions++

			item := addItem(tx.TypeID, tx.TypeName)
			item.TotalBought += buyTotal
			item.TotalSold += sellTotal
			item.QtyBought += int64(matched)
			item.QtySold += int64(matched)
			item.Transactions++

			buySt := addStation(lot.LocationID, lot.LocationName)
			buySt.TotalBought += buyTotal
			buySt.Transactions++
			sellSt := addStation(tx.LocationID, tx.LocationName)
			sellSt.TotalSold += sellTotal
			sellSt.Transactions++

			coverage.MatchedSellQty += int64(matched)
			coverage.MatchedSellValue += sellGross

			summary.RealizedTrades++
			summary.RealizedQuantity += int64(matched)
			summary.TotalFees += buyFee + sellBrokerFee
			summary.TotalTaxes += sellTax

			ledger = append(ledger, RealizedTrade{
				TypeID:            tx.TypeID,
				TypeName:          tx.TypeName,
				Quantity:          matched,
				BuyTransactionID:  lot.TransactionID,
				SellTransactionID: tx.TransactionID,
				BuyDate:           lot.Date.Format(time.RFC3339),
				SellDate:          tx.Date,
				HoldingDays:       holdingDays,
				BuyLocationID:     lot.LocationID,
				BuyLocationName:   lot.LocationName,
				SellLocationID:    tx.LocationID,
				SellLocationName:  tx.LocationName,
				BuyUnitPrice:      lot.UnitPrice,
				SellUnitPrice:     tx.UnitPrice,
				BuyGross:          buyGross,
				SellGross:         sellGross,
				BuyFee:            buyFee,
				SellBrokerFee:     sellBrokerFee,
				SellTax:           sellTax,
				BuyTotal:          buyTotal,
				SellTotal:         sellTotal,
				RealizedPnL:       pnl,
				MarginPercent:     margin,
			})
		}

		if inLookback && remaining > 0 {
			unmatchedGross := tx.UnitPrice * float64(remaining)
			coverage.UnmatchedSellQty += int64(remaining)
			coverage.UnmatchedSellValue += unmatchedGross

			if opt.IncludeUnmatchedSell {
				// Legacy behavior for backward compatibility in tests/callers.
				// Treat unmatched sells as zero-cost proceeds.
				sellBrokerFee := unmatchedGross * opt.BrokerFeePercent / 100.0
				sellTax := unmatchedGross * opt.SalesTaxPercent / 100.0
				sellTotal := unmatchedGross - sellBrokerFee - sellTax

				day := addDay(rec.t.Format("2006-01-02"))
				day.SellTotal += sellTotal
				day.Transactions++

				item := addItem(tx.TypeID, tx.TypeName)
				item.TotalSold += sellTotal
				item.QtySold += int64(remaining)
				item.Transactions++

				sellSt := addStation(tx.LocationID, tx.LocationName)
				sellSt.TotalSold += sellTotal
				sellSt.Transactions++

				summary.RealizedTrades++
				summary.RealizedQuantity += int64(remaining)
				summary.TotalFees += sellBrokerFee
				summary.TotalTaxes += sellTax

				ledger = append(ledger, RealizedTrade{
					TypeID:            tx.TypeID,
					TypeName:          tx.TypeName,
					Quantity:          remaining,
					BuyTransactionID:  0,
					SellTransactionID: tx.TransactionID,
					BuyDate:           "",
					SellDate:          tx.Date,
					HoldingDays:       0,
					BuyLocationID:     0,
					BuyLocationName:   "",
					SellLocationID:    tx.LocationID,
					SellLocationName:  tx.LocationName,
					BuyUnitPrice:      0,
					SellUnitPrice:     tx.UnitPrice,
					BuyGross:          0,
					SellGross:         unmatchedGross,
					BuyFee:            0,
					SellBrokerFee:     sellBrokerFee,
					SellTax:           sellTax,
					BuyTotal:          0,
					SellTotal:         sellTotal,
					RealizedPnL:       sellTotal,
					MarginPercent:     0,
					Unmatched:         true,
				})
			}
		}

		buyQueues[tx.TypeID] = queue
	}

	if coverage.TotalSellQty > 0 {
		coverage.MatchRateQtyPct = float64(coverage.MatchedSellQty) / float64(coverage.TotalSellQty) * 100
	}
	if coverage.TotalSellValue > 0 {
		coverage.MatchRateValuePct = coverage.MatchedSellValue / coverage.TotalSellValue * 100
	}
	out.Coverage = coverage

	// Build daily series
	days := make([]DailyPnLEntry, 0, len(dayMap))
	for _, entry := range dayMap {
		entry.NetPnL = entry.SellTotal - entry.BuyTotal
		days = append(days, *entry)
	}
	sort.Slice(days, func(i, j int) bool {
		return days[i].Date < days[j].Date
	})

	// Cumulative and drawdown.
	cumulative := 0.0
	cumulativePeak := 0.0
	maxDrawdownISK := 0.0
	maxDrawdownPeakIdx := 0
	maxDrawdownTroughIdx := 0
	currentPeakIdx := 0

	for i := range days {
		cumulative += days[i].NetPnL
		days[i].CumulativePnL = cumulative

		if cumulative > cumulativePeak {
			cumulativePeak = cumulative
			currentPeakIdx = i
		}

		drawdownISK := cumulative - cumulativePeak
		if cumulativePeak > 0 {
			days[i].DrawdownPct = drawdownISK / cumulativePeak * 100
		}

		if drawdownISK < maxDrawdownISK {
			maxDrawdownISK = drawdownISK
			maxDrawdownPeakIdx = currentPeakIdx
			maxDrawdownTroughIdx = i
		}
	}

	summary.TotalDays = len(days)
	if len(days) > 0 {
		summary.BestDayPnL = days[0].NetPnL
		summary.BestDayDate = days[0].Date
		summary.WorstDayPnL = days[0].NetPnL
		summary.WorstDayDate = days[0].Date
	}

	var grossProfit, grossLoss float64
	var totalWinISK, totalLossISK float64

	for _, d := range days {
		summary.TotalPnL += d.NetPnL
		summary.TotalBought += d.BuyTotal
		summary.TotalSold += d.SellTotal

		if d.NetPnL > 0 {
			summary.ProfitableDays++
			grossProfit += d.NetPnL
			totalWinISK += d.NetPnL
		} else if d.NetPnL < 0 {
			summary.LosingDays++
			grossLoss += -d.NetPnL
			totalLossISK += -d.NetPnL
		}

		if d.NetPnL > summary.BestDayPnL {
			summary.BestDayPnL = d.NetPnL
			summary.BestDayDate = d.Date
		}
		if d.NetPnL < summary.WorstDayPnL {
			summary.WorstDayPnL = d.NetPnL
			summary.WorstDayDate = d.Date
		}
	}

	if summary.TotalDays > 0 {
		summary.AvgDailyPnL = summary.TotalPnL / float64(summary.TotalDays)
		summary.WinRate = float64(summary.ProfitableDays) / float64(summary.TotalDays) * 100
	}

	// ROI: time-weighted average deployed capital.
	if len(days) > 0 {
		var cumBuy, cumSell, capitalSum float64
		for _, d := range days {
			cumBuy += d.BuyTotal
			cumSell += d.SellTotal
			deployed := cumBuy - cumSell
			if deployed > 0 {
				capitalSum += deployed
			}
		}
		avgCapital := capitalSum / float64(len(days))
		if avgCapital > 0 {
			summary.ROIPercent = summary.TotalPnL / avgCapital * 100
		} else if summary.TotalBought > 0 {
			summary.ROIPercent = summary.TotalPnL / summary.TotalBought * 100
		}
	}

	if summary.TotalDays >= 2 {
		dailyPnLs := make([]float64, len(days))
		for i, d := range days {
			dailyPnLs[i] = d.NetPnL
		}
		mu := mean(dailyPnLs)
		sigma := math.Sqrt(variance(dailyPnLs))
		if sigma > 0 {
			summary.SharpeRatio = (mu / sigma) * math.Sqrt(365)
		}
	}

	summary.MaxDrawdownISK = -maxDrawdownISK
	if cumulativePeak > 0 {
		summary.MaxDrawdownPct = -maxDrawdownISK / cumulativePeak * 100
	}
	if maxDrawdownTroughIdx > maxDrawdownPeakIdx {
		peakDate, errP := time.Parse("2006-01-02", days[maxDrawdownPeakIdx].Date)
		troughDate, errT := time.Parse("2006-01-02", days[maxDrawdownTroughIdx].Date)
		if errP == nil && errT == nil {
			summary.MaxDrawdownDays = int(troughDate.Sub(peakDate).Hours() / 24)
		} else {
			summary.MaxDrawdownDays = maxDrawdownTroughIdx - maxDrawdownPeakIdx
		}
	}

	if summary.MaxDrawdownISK > 0 && summary.TotalDays > 0 {
		annualizedReturn := summary.TotalPnL * 365 / float64(summary.TotalDays)
		summary.CalmarRatio = annualizedReturn / summary.MaxDrawdownISK
	}
	if grossLoss > 0 {
		summary.ProfitFactor = grossProfit / grossLoss
	}
	if summary.ProfitableDays > 0 {
		summary.AvgWin = totalWinISK / float64(summary.ProfitableDays)
	}
	if summary.LosingDays > 0 {
		summary.AvgLoss = totalLossISK / float64(summary.LosingDays)
	}
	if summary.TotalDays > 0 {
		winRate := float64(summary.ProfitableDays) / float64(summary.TotalDays)
		lossRate := float64(summary.LosingDays) / float64(summary.TotalDays)
		summary.ExpectancyPerTrade = winRate*summary.AvgWin - lossRate*summary.AvgLoss
	}

	// Per-item stats.
	items := make([]ItemPnL, 0, len(itemMap))
	for _, item := range itemMap {
		item.NetPnL = item.TotalSold - item.TotalBought
		if item.QtyBought > 0 {
			item.AvgBuyPrice = item.TotalBought / float64(item.QtyBought)
		}
		if item.QtySold > 0 {
			item.AvgSellPrice = item.TotalSold / float64(item.QtySold)
		}
		if item.AvgBuyPrice > 0 && item.AvgSellPrice > 0 {
			item.MarginPercent = (item.AvgSellPrice - item.AvgBuyPrice) / item.AvgBuyPrice * 100
		}
		items = append(items, *item)
	}
	sort.Slice(items, func(i, j int) bool {
		absI := items[i].NetPnL
		if absI < 0 {
			absI = -absI
		}
		absJ := items[j].NetPnL
		if absJ < 0 {
			absJ = -absJ
		}
		return absI > absJ
	})
	if len(items) > 50 {
		items = items[:50]
	}

	// Per-station stats.
	stations := make([]StationPnL, 0, len(stationMap))
	for _, st := range stationMap {
		st.NetPnL = st.TotalSold - st.TotalBought
		stations = append(stations, *st)
	}
	sort.Slice(stations, func(i, j int) bool {
		absI := stations[i].NetPnL
		if absI < 0 {
			absI = -absI
		}
		absJ := stations[j].NetPnL
		if absJ < 0 {
			absJ = -absJ
		}
		return absI > absJ
	})
	if len(stations) > 20 {
		stations = stations[:20]
	}

	// Open positions (grouped by type+location to preserve where inventory sits).
	type openKey struct {
		typeID     int32
		locationID int64
	}
	type openAgg struct {
		typeID       int32
		typeName     string
		locationID   int64
		locationName string
		quantity     int64
		costBasis    float64
		oldest       time.Time
	}
	openMap := make(map[openKey]*openAgg)
	totalOpenCost := 0.0
	for _, queue := range buyQueues {
		for _, lot := range queue {
			if lot.Remaining <= 0 {
				continue
			}
			key := openKey{typeID: lot.TypeID, locationID: lot.LocationID}
			a := openMap[key]
			if a == nil {
				a = &openAgg{
					typeID:       lot.TypeID,
					typeName:     lot.TypeName,
					locationID:   lot.LocationID,
					locationName: lot.LocationName,
					oldest:       lot.Date,
				}
				openMap[key] = a
			} else if a.locationName == "" && lot.LocationName != "" {
				a.locationName = lot.LocationName
			}
			q := int64(lot.Remaining)
			gross := lot.UnitPrice * float64(lot.Remaining)
			buyFee := gross * opt.BrokerFeePercent / 100.0
			a.quantity += q
			a.costBasis += gross + buyFee
			if lot.Date.Before(a.oldest) {
				a.oldest = lot.Date
			}
		}
	}

	openPositions := make([]OpenPosition, 0, len(openMap))
	for _, a := range openMap {
		if a == nil || a.quantity <= 0 {
			continue
		}
		avgCost := 0.0
		if a.quantity > 0 {
			avgCost = a.costBasis / float64(a.quantity)
		}
		openPositions = append(openPositions, OpenPosition{
			TypeID:        a.typeID,
			TypeName:      a.typeName,
			LocationID:    a.locationID,
			LocationName:  a.locationName,
			Quantity:      a.quantity,
			AvgCost:       avgCost,
			CostBasis:     a.costBasis,
			OldestLotDate: a.oldest.Format("2006-01-02"),
		})
		totalOpenCost += a.costBasis
	}
	sort.Slice(openPositions, func(i, j int) bool {
		return openPositions[i].CostBasis > openPositions[j].CostBasis
	})
	summary.OpenPositions = len(openPositions)
	summary.OpenCostBasis = totalOpenCost

	// Ledger newest first.
	sort.Slice(ledger, func(i, j int) bool {
		if ledger[i].SellDate == ledger[j].SellDate {
			if ledger[i].SellTransactionID == ledger[j].SellTransactionID {
				return ledger[i].BuyTransactionID > ledger[j].BuyTransactionID
			}
			return ledger[i].SellTransactionID > ledger[j].SellTransactionID
		}
		return ledger[i].SellDate > ledger[j].SellDate
	})
	if opt.LedgerLimit > 0 && len(ledger) > opt.LedgerLimit {
		ledger = ledger[:opt.LedgerLimit]
	}
	if len(openPositions) > 50 {
		openPositions = openPositions[:50]
	}

	out.DailyPnL = days
	out.Summary = summary
	out.TopItems = items
	out.TopStations = stations
	out.Ledger = ledger
	out.OpenPositions = openPositions
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
