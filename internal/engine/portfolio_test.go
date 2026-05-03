package engine

import (
	"math"
	"testing"
	"time"

	"eve-flipper/internal/esi"
)

var testTxnID int64

// helper: build a wallet transaction for a given day offset (negative = past).
func txn(dayOffset int, typeID int32, typeName string, locationID int64, locationName string, isBuy bool, price float64, qty int32) esi.WalletTransaction {
	d := time.Now().UTC().AddDate(0, 0, dayOffset)
	testTxnID++
	return esi.WalletTransaction{
		TransactionID: testTxnID,
		Date:          d.Format(time.RFC3339),
		TypeID:        typeID,
		TypeName:      typeName,
		LocationID:    locationID,
		LocationName:  locationName,
		IsBuy:         isBuy,
		UnitPrice:     price,
		Quantity:      qty,
	}
}

func TestComputePortfolioPnL_Empty(t *testing.T) {
	result := ComputePortfolioPnL(nil, 30)
	if result == nil {
		t.Fatal("expected non-nil for empty input")
	}
	if len(result.DailyPnL) != 0 {
		t.Errorf("expected 0 daily entries, got %d", len(result.DailyPnL))
	}
	if len(result.TopItems) != 0 {
		t.Errorf("expected 0 items, got %d", len(result.TopItems))
	}
	if len(result.TopStations) != 0 {
		t.Errorf("expected 0 stations, got %d", len(result.TopStations))
	}
}

func TestComputePortfolioPnL_SingleDay(t *testing.T) {
	txns := []esi.WalletTransaction{
		txn(-1, 34, "Tritanium", 60003760, "Jita", true, 100, 10),
		txn(-1, 34, "Tritanium", 60003760, "Jita", false, 150, 10),
	}
	result := ComputePortfolioPnL(txns, 30)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if len(result.DailyPnL) != 1 {
		t.Fatalf("expected 1 day, got %d", len(result.DailyPnL))
	}

	day := result.DailyPnL[0]
	// Buy: 100*10 = 1000, Sell: 150*10 = 1500
	if math.Abs(day.BuyTotal-1000) > 1e-6 {
		t.Errorf("BuyTotal = %v, want 1000", day.BuyTotal)
	}
	if math.Abs(day.SellTotal-1500) > 1e-6 {
		t.Errorf("SellTotal = %v, want 1500", day.SellTotal)
	}
	if math.Abs(day.NetPnL-500) > 1e-6 {
		t.Errorf("NetPnL = %v, want 500", day.NetPnL)
	}
	if day.Transactions != 1 {
		t.Errorf("Transactions = %d, want 1", day.Transactions)
	}
	if math.Abs(day.CumulativePnL-500) > 1e-6 {
		t.Errorf("CumulativePnL = %v, want 500", day.CumulativePnL)
	}

	s := result.Summary
	if math.Abs(s.TotalPnL-500) > 1e-6 {
		t.Errorf("TotalPnL = %v, want 500", s.TotalPnL)
	}
	if s.ProfitableDays != 1 {
		t.Errorf("ProfitableDays = %d, want 1", s.ProfitableDays)
	}
	if s.LosingDays != 0 {
		t.Errorf("LosingDays = %d, want 0", s.LosingDays)
	}
	if s.TotalDays != 1 {
		t.Errorf("TotalDays = %d, want 1", s.TotalDays)
	}
	if math.Abs(s.WinRate-100) > 1e-6 {
		t.Errorf("WinRate = %v, want 100", s.WinRate)
	}
}

func TestComputePortfolioPnL_SharpeRatio(t *testing.T) {
	// Create 5 days of transactions with known daily PnL:
	// Day 1: sell 1500, buy 1000 => +500
	// Day 2: sell 1200, buy 1000 => +200
	// Day 3: sell 1000, buy 1300 => -300
	// Day 4: sell 2000, buy 1000 => +1000
	// Day 5: sell 1100, buy 1000 => +100
	txns := []esi.WalletTransaction{
		txn(-5, 34, "Tritanium", 60003760, "Jita", true, 100, 10),
		txn(-5, 34, "Tritanium", 60003760, "Jita", false, 150, 10),
		txn(-4, 34, "Tritanium", 60003760, "Jita", true, 100, 10),
		txn(-4, 34, "Tritanium", 60003760, "Jita", false, 120, 10),
		txn(-3, 34, "Tritanium", 60003760, "Jita", true, 130, 10),
		txn(-3, 34, "Tritanium", 60003760, "Jita", false, 100, 10),
		txn(-2, 34, "Tritanium", 60003760, "Jita", true, 100, 10),
		txn(-2, 34, "Tritanium", 60003760, "Jita", false, 200, 10),
		txn(-1, 34, "Tritanium", 60003760, "Jita", true, 100, 10),
		txn(-1, 34, "Tritanium", 60003760, "Jita", false, 110, 10),
	}
	result := ComputePortfolioPnL(txns, 30)
	if result == nil {
		t.Fatal("expected non-nil")
	}

	// Daily PnLs: [500, 200, -300, 1000, 100]
	// mean = (500+200-300+1000+100)/5 = 1500/5 = 300
	// Sample variance = ((500-300)^2 + (200-300)^2 + (-300-300)^2 + (1000-300)^2 + (100-300)^2) / (5-1)
	//                 = (40000 + 10000 + 360000 + 490000 + 40000) / 4 = 940000 / 4 = 235000
	// sigma = sqrt(235000) ≈ 484.768
	// Sharpe = (300 / 484.768) * sqrt(365)
	dailyPnLs := []float64{500, 200, -300, 1000, 100}
	mu := mean(dailyPnLs)
	sigma := math.Sqrt(variance(dailyPnLs))
	wantSharpe := (mu / sigma) * math.Sqrt(365)

	s := result.Summary
	if math.Abs(s.SharpeRatio-wantSharpe) > 0.01 {
		t.Errorf("SharpeRatio = %v, want %v", s.SharpeRatio, wantSharpe)
	}
}

func TestComputePortfolioPnL_DrawdownAndMaxDrawdown(t *testing.T) {
	// Realized daily PnL:
	// Day 1: +1000 (buy 1000 -> sell 2000)
	// Day 2: +500  (buy 500  -> sell 1000)
	// Day 3: -800  (buy 1300 -> sell 500)
	// Day 4: -300  (buy 600  -> sell 300)
	// Day 5: +200  (buy 100  -> sell 300)
	// Cumulative: 1000,1500,700,400,600
	txns := []esi.WalletTransaction{
		txn(-5, 34, "Trit", 1, "Jita", true, 1000, 1),
		txn(-5, 34, "Trit", 1, "Jita", false, 2000, 1),
		txn(-4, 34, "Trit", 1, "Jita", true, 500, 1),
		txn(-4, 34, "Trit", 1, "Jita", false, 1000, 1),
		txn(-3, 34, "Trit", 1, "Jita", true, 1300, 1),
		txn(-3, 34, "Trit", 1, "Jita", false, 500, 1),
		txn(-2, 34, "Trit", 1, "Jita", true, 600, 1),
		txn(-2, 34, "Trit", 1, "Jita", false, 300, 1),
		txn(-1, 34, "Trit", 1, "Jita", true, 100, 1),
		txn(-1, 34, "Trit", 1, "Jita", false, 300, 1),
	}
	result := ComputePortfolioPnL(txns, 30)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if len(result.DailyPnL) != 5 {
		t.Fatalf("expected 5 days, got %d", len(result.DailyPnL))
	}

	// Check cumulative PnL at day 4 (index 3): should be 400
	if math.Abs(result.DailyPnL[3].CumulativePnL-400) > 1e-6 {
		t.Errorf("CumulativePnL[3] = %v, want 400", result.DailyPnL[3].CumulativePnL)
	}

	// Max drawdown should be at day 4: 400 - 1500 = -1100 ISK, pct = 1100/1500*100 ≈ 73.33%
	s := result.Summary
	if math.Abs(s.MaxDrawdownISK-1100) > 1e-6 {
		t.Errorf("MaxDrawdownISK = %v, want 1100", s.MaxDrawdownISK)
	}
	wantPct := 1100.0 / 1500.0 * 100
	if math.Abs(s.MaxDrawdownPct-wantPct) > 0.1 {
		t.Errorf("MaxDrawdownPct = %v, want ~%v", s.MaxDrawdownPct, wantPct)
	}

	// DrawdownPct at day 3 (index 2): (700-1500)/1500 * 100 = -53.33%
	if math.Abs(result.DailyPnL[2].DrawdownPct-(-53.33)) > 0.1 {
		t.Errorf("DrawdownPct[2] = %v, want ~-53.33", result.DailyPnL[2].DrawdownPct)
	}

	// MaxDrawdownDays: from peak index 1 to trough index 3 = 2
	if s.MaxDrawdownDays != 2 {
		t.Errorf("MaxDrawdownDays = %d, want 2", s.MaxDrawdownDays)
	}
}

func TestComputePortfolioPnL_ProfitFactor(t *testing.T) {
	// Day 1: +500, Day 2: -200, Day 3: +300, Day 4: -100
	// Gross profit = 500+300 = 800, Gross loss = 200+100 = 300
	// Profit factor = 800/300 ≈ 2.667
	txns := []esi.WalletTransaction{
		txn(-4, 34, "T", 1, "J", true, 100, 1),
		txn(-4, 34, "T", 1, "J", false, 600, 1), // +500
		txn(-3, 34, "T", 1, "J", true, 500, 1),
		txn(-3, 34, "T", 1, "J", false, 300, 1), // -200
		txn(-2, 34, "T", 1, "J", true, 100, 1),
		txn(-2, 34, "T", 1, "J", false, 400, 1), // +300
		txn(-1, 34, "T", 1, "J", true, 250, 1),
		txn(-1, 34, "T", 1, "J", false, 150, 1), // -100
	}
	result := ComputePortfolioPnL(txns, 30)
	s := result.Summary
	wantPF := 800.0 / 300.0
	if math.Abs(s.ProfitFactor-wantPF) > 0.01 {
		t.Errorf("ProfitFactor = %v, want %v", s.ProfitFactor, wantPF)
	}
}

func TestComputePortfolioPnL_AvgWinLossAndExpectancy(t *testing.T) {
	// Day 1: +500, Day 2: -200, Day 3: +300, Day 4: -100
	// AvgWin = (500+300)/2 = 400
	// AvgLoss = (200+100)/2 = 150
	// WinRate = 2/4 = 0.5, LossRate = 2/4 = 0.5
	// Expectancy = 0.5*400 - 0.5*150 = 200-75 = 125
	txns := []esi.WalletTransaction{
		txn(-4, 34, "T", 1, "J", true, 100, 1),
		txn(-4, 34, "T", 1, "J", false, 600, 1), // +500
		txn(-3, 34, "T", 1, "J", true, 500, 1),
		txn(-3, 34, "T", 1, "J", false, 300, 1), // -200
		txn(-2, 34, "T", 1, "J", true, 100, 1),
		txn(-2, 34, "T", 1, "J", false, 400, 1), // +300
		txn(-1, 34, "T", 1, "J", true, 250, 1),
		txn(-1, 34, "T", 1, "J", false, 150, 1), // -100
	}
	result := ComputePortfolioPnL(txns, 30)
	s := result.Summary
	if math.Abs(s.AvgWin-400) > 1e-6 {
		t.Errorf("AvgWin = %v, want 400", s.AvgWin)
	}
	if math.Abs(s.AvgLoss-150) > 1e-6 {
		t.Errorf("AvgLoss = %v, want 150", s.AvgLoss)
	}
	if math.Abs(s.ExpectancyPerTrade-125) > 1e-6 {
		t.Errorf("ExpectancyPerTrade = %v, want 125", s.ExpectancyPerTrade)
	}
}

func TestComputePortfolioPnL_CalmarRatio(t *testing.T) {
	// Day 1: +1000, Day 2: -500, Day 3: +200
	// Cumulative: 1000, 500, 700. Peak = 1000, trough = 500.
	// MaxDrawdownISK = 500. TotalPnL = 700.
	// Annualized return = 700 * 365 / 3
	// Calmar = annualized return / 500
	txns := []esi.WalletTransaction{
		txn(-3, 34, "T", 1, "J", true, 100, 1),
		txn(-3, 34, "T", 1, "J", false, 1100, 1), // +1000
		txn(-2, 34, "T", 1, "J", true, 800, 1),
		txn(-2, 34, "T", 1, "J", false, 300, 1), // -500
		txn(-1, 34, "T", 1, "J", true, 100, 1),
		txn(-1, 34, "T", 1, "J", false, 300, 1), // +200
	}
	result := ComputePortfolioPnL(txns, 30)
	s := result.Summary
	annualReturn := 700.0 * 365 / 3
	wantCalmar := annualReturn / 500
	if math.Abs(s.CalmarRatio-wantCalmar) > 0.01 {
		t.Errorf("CalmarRatio = %v, want %v", s.CalmarRatio, wantCalmar)
	}
}

func TestComputePortfolioPnL_StationBreakdown(t *testing.T) {
	txns := []esi.WalletTransaction{
		txn(-2, 34, "Trit", 60003760, "Jita", true, 100, 10),
		txn(-2, 34, "Trit", 60003760, "Jita", false, 150, 10),
		txn(-1, 35, "Pyerite", 60008494, "Amarr", true, 50, 20),
		txn(-1, 35, "Pyerite", 60008494, "Amarr", false, 80, 20),
	}
	result := ComputePortfolioPnL(txns, 30)
	if len(result.TopStations) != 2 {
		t.Fatalf("expected 2 stations, got %d", len(result.TopStations))
	}

	// Find Jita station entry
	var jita, amarr *StationPnL
	for i := range result.TopStations {
		if result.TopStations[i].LocationID == 60003760 {
			jita = &result.TopStations[i]
		}
		if result.TopStations[i].LocationID == 60008494 {
			amarr = &result.TopStations[i]
		}
	}
	if jita == nil || amarr == nil {
		t.Fatal("expected both Jita and Amarr stations")
	}

	// Jita: buy 100*10=1000, sell 150*10=1500, net=500
	if math.Abs(jita.TotalBought-1000) > 1e-6 {
		t.Errorf("Jita TotalBought = %v, want 1000", jita.TotalBought)
	}
	if math.Abs(jita.TotalSold-1500) > 1e-6 {
		t.Errorf("Jita TotalSold = %v, want 1500", jita.TotalSold)
	}
	if math.Abs(jita.NetPnL-500) > 1e-6 {
		t.Errorf("Jita NetPnL = %v, want 500", jita.NetPnL)
	}
	if jita.Transactions != 2 {
		t.Errorf("Jita Transactions = %d, want 2", jita.Transactions)
	}

	// Amarr: buy 50*20=1000, sell 80*20=1600, net=600
	if math.Abs(amarr.TotalBought-1000) > 1e-6 {
		t.Errorf("Amarr TotalBought = %v, want 1000", amarr.TotalBought)
	}
	if math.Abs(amarr.NetPnL-600) > 1e-6 {
		t.Errorf("Amarr NetPnL = %v, want 600", amarr.NetPnL)
	}
}

func TestComputePortfolioPnL_ItemBreakdown(t *testing.T) {
	txns := []esi.WalletTransaction{
		txn(-2, 34, "Tritanium", 1, "Jita", true, 100, 10),
		txn(-2, 34, "Tritanium", 1, "Jita", false, 150, 10),
		txn(-1, 35, "Pyerite", 1, "Jita", true, 50, 20),
		txn(-1, 35, "Pyerite", 1, "Jita", false, 80, 20),
	}
	result := ComputePortfolioPnL(txns, 30)
	if len(result.TopItems) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result.TopItems))
	}

	// Items are sorted by |NetPnL| desc. Pyerite: 600, Tritanium: 500
	if result.TopItems[0].TypeID != 35 {
		t.Errorf("Top item should be Pyerite (35), got %d", result.TopItems[0].TypeID)
	}

	pyerite := result.TopItems[0]
	if math.Abs(pyerite.AvgBuyPrice-50) > 1e-6 {
		t.Errorf("Pyerite AvgBuyPrice = %v, want 50", pyerite.AvgBuyPrice)
	}
	if math.Abs(pyerite.AvgSellPrice-80) > 1e-6 {
		t.Errorf("Pyerite AvgSellPrice = %v, want 80", pyerite.AvgSellPrice)
	}
	// Margin: (80-50)/50 * 100 = 60%
	if math.Abs(pyerite.MarginPercent-60) > 1e-6 {
		t.Errorf("Pyerite MarginPercent = %v, want 60", pyerite.MarginPercent)
	}
}

func TestComputePortfolioPnL_LookbackFilter(t *testing.T) {
	// Transactions from 60 days ago should be excluded with lookback of 30.
	txns := []esi.WalletTransaction{
		txn(-60, 34, "Trit", 1, "Jita", false, 1000, 1), // outside lookback
		txn(-2, 34, "Trit", 1, "Jita", true, 100, 1),    // inside lookback
		txn(-1, 34, "Trit", 1, "Jita", false, 600, 1),   // inside lookback
	}
	result := ComputePortfolioPnL(txns, 30)
	if len(result.DailyPnL) != 1 {
		t.Errorf("expected 1 day within lookback, got %d", len(result.DailyPnL))
	}
	if math.Abs(result.Summary.TotalPnL-500) > 1e-6 {
		t.Errorf("TotalPnL = %v, want 500 (60-day txn should be excluded)", result.Summary.TotalPnL)
	}
}

func TestComputePortfolioPnL_ROI(t *testing.T) {
	// Buy 1000, Sell 1500. ROI = (1500-1000)/1000 * 100 = 50%
	txns := []esi.WalletTransaction{
		txn(-1, 34, "T", 1, "J", true, 100, 10),
		txn(-1, 34, "T", 1, "J", false, 150, 10),
	}
	result := ComputePortfolioPnL(txns, 30)
	if math.Abs(result.Summary.ROIPercent-50) > 1e-6 {
		t.Errorf("ROIPercent = %v, want 50", result.Summary.ROIPercent)
	}
}

func TestComputePortfolioPnL_AllLosingDays(t *testing.T) {
	// No profitable days: ProfitFactor should be 0, AvgWin = 0.
	txns := []esi.WalletTransaction{
		txn(-3, 34, "T", 1, "J", true, 100, 1),
		txn(-2, 34, "T", 1, "J", true, 200, 1),
		txn(-1, 34, "T", 1, "J", true, 300, 1),
	}
	result := ComputePortfolioPnL(txns, 30)
	s := result.Summary
	if s.ProfitableDays != 0 {
		t.Errorf("ProfitableDays = %d, want 0", s.ProfitableDays)
	}
	if s.AvgWin != 0 {
		t.Errorf("AvgWin = %v, want 0", s.AvgWin)
	}
	if s.ProfitFactor != 0 {
		t.Errorf("ProfitFactor = %v, want 0 (no gross profit)", s.ProfitFactor)
	}
}

func TestComputePortfolioPnLWithOptions_StrictUnmatchedExcluded(t *testing.T) {
	txns := []esi.WalletTransaction{
		// Sell without known buy in lookback.
		txn(-1, 34, "Tritanium", 60003760, "Jita", false, 120, 10),
	}
	got := ComputePortfolioPnLWithOptions(txns, PortfolioPnLOptions{
		LookbackDays:         30,
		SalesTaxPercent:      0,
		BrokerFeePercent:     0,
		LedgerLimit:          100,
		IncludeUnmatchedSell: false,
	})
	if got == nil {
		t.Fatal("expected non-nil")
	}
	if len(got.DailyPnL) != 0 {
		t.Fatalf("strict mode should exclude unmatched sells, got daily=%d", len(got.DailyPnL))
	}
	if got.Coverage.UnmatchedSellQty != 10 {
		t.Fatalf("unmatched qty = %d, want 10", got.Coverage.UnmatchedSellQty)
	}
	if got.Coverage.MatchedSellQty != 0 {
		t.Fatalf("matched qty = %d, want 0", got.Coverage.MatchedSellQty)
	}
}

func TestComputePortfolioPnLWithOptions_OpenPositions(t *testing.T) {
	txns := []esi.WalletTransaction{
		txn(-3, 34, "Tritanium", 60003760, "Jita", true, 100, 10),
		txn(-2, 34, "Tritanium", 60003760, "Jita", false, 120, 4),
	}
	got := ComputePortfolioPnLWithOptions(txns, PortfolioPnLOptions{
		LookbackDays:         30,
		SalesTaxPercent:      0,
		BrokerFeePercent:     0,
		LedgerLimit:          100,
		IncludeUnmatchedSell: false,
	})
	if got == nil {
		t.Fatal("expected non-nil")
	}
	if len(got.OpenPositions) != 1 {
		t.Fatalf("open positions len = %d, want 1", len(got.OpenPositions))
	}
	if got.OpenPositions[0].Quantity != 6 {
		t.Fatalf("open quantity = %d, want 6", got.OpenPositions[0].Quantity)
	}
	if math.Abs(got.OpenPositions[0].CostBasis-600) > 1e-6 {
		t.Fatalf("open cost basis = %v, want 600", got.OpenPositions[0].CostBasis)
	}
}

func TestComputePortfolioPnLWithOptions_LedgerLimitKeepsNewest(t *testing.T) {
	txns := []esi.WalletTransaction{
		txn(-3, 34, "Tritanium", 60003760, "Jita", true, 100, 1),
		txn(-3, 34, "Tritanium", 60003760, "Jita", false, 110, 1),
		txn(-2, 34, "Tritanium", 60003760, "Jita", true, 100, 1),
		txn(-2, 34, "Tritanium", 60003760, "Jita", false, 120, 1),
		txn(-1, 34, "Tritanium", 60003760, "Jita", true, 100, 1),
		txn(-1, 34, "Tritanium", 60003760, "Jita", false, 130, 1),
	}
	got := ComputePortfolioPnLWithOptions(txns, PortfolioPnLOptions{
		LookbackDays:         30,
		SalesTaxPercent:      0,
		BrokerFeePercent:     0,
		LedgerLimit:          2,
		IncludeUnmatchedSell: false,
	})
	if got == nil {
		t.Fatal("expected non-nil")
	}
	if len(got.Ledger) != 2 {
		t.Fatalf("ledger len = %d, want 2", len(got.Ledger))
	}
	if math.Abs(got.Ledger[0].SellUnitPrice-130) > 1e-6 {
		t.Fatalf("ledger[0] sell price = %v, want 130", got.Ledger[0].SellUnitPrice)
	}
	if math.Abs(got.Ledger[1].SellUnitPrice-120) > 1e-6 {
		t.Fatalf("ledger[1] sell price = %v, want 120", got.Ledger[1].SellUnitPrice)
	}
}

func TestComputePortfolioPnLWithOptions_OpenPositionsGroupedByLocation(t *testing.T) {
	txns := []esi.WalletTransaction{
		txn(-2, 34, "Tritanium", 60003760, "Jita", true, 100, 5),
		txn(-1, 34, "Tritanium", 60008494, "Amarr", true, 120, 7),
	}
	got := ComputePortfolioPnLWithOptions(txns, PortfolioPnLOptions{
		LookbackDays:         30,
		SalesTaxPercent:      0,
		BrokerFeePercent:     0,
		LedgerLimit:          100,
		IncludeUnmatchedSell: false,
	})
	if got == nil {
		t.Fatal("expected non-nil")
	}
	if len(got.OpenPositions) != 2 {
		t.Fatalf("open positions len = %d, want 2", len(got.OpenPositions))
	}
	byLocation := make(map[int64]OpenPosition, len(got.OpenPositions))
	for _, p := range got.OpenPositions {
		byLocation[p.LocationID] = p
	}
	jita, okJ := byLocation[60003760]
	amarr, okA := byLocation[60008494]
	if !okJ || !okA {
		t.Fatalf("expected both Jita and Amarr positions, got %+v", byLocation)
	}
	if jita.Quantity != 5 {
		t.Fatalf("jita qty = %d, want 5", jita.Quantity)
	}
	if amarr.Quantity != 7 {
		t.Fatalf("amarr qty = %d, want 7", amarr.Quantity)
	}
	if math.Abs(jita.CostBasis-500) > 1e-6 {
		t.Fatalf("jita cost basis = %v, want 500", jita.CostBasis)
	}
	if math.Abs(amarr.CostBasis-840) > 1e-6 {
		t.Fatalf("amarr cost basis = %v, want 840", amarr.CostBasis)
	}
}

func TestComputePortfolioPnLWithOptions_OpenPositionsSummaryNotTruncated(t *testing.T) {
	txns := make([]esi.WalletTransaction, 0, 55)
	for i := 0; i < 55; i++ {
		txns = append(txns, txn(-1, int32(10000+i), "Type", 60003760, "Jita", true, 10, 1))
	}
	got := ComputePortfolioPnLWithOptions(txns, PortfolioPnLOptions{
		LookbackDays:         30,
		SalesTaxPercent:      0,
		BrokerFeePercent:     0,
		LedgerLimit:          100,
		IncludeUnmatchedSell: false,
	})
	if got == nil {
		t.Fatal("expected non-nil")
	}
	if got.Summary.OpenPositions != 55 {
		t.Fatalf("summary open positions = %d, want 55", got.Summary.OpenPositions)
	}
	if len(got.OpenPositions) != 50 {
		t.Fatalf("returned open positions len = %d, want 50 (UI cap)", len(got.OpenPositions))
	}
}
