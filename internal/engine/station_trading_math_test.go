package engine

import "testing"

func TestEstimateSellUnitsPerDay_AllowsBvSBelowOne(t *testing.T) {
	daily := 100.0
	buyVol := int64(500)
	sellVol := int64(1000)

	sellPerDay := estimateSellUnitsPerDay(daily, buyVol, sellVol)
	if sellPerDay <= daily {
		t.Fatalf("sellPerDay = %v, want > %v", sellPerDay, daily)
	}

	bvs := daily / sellPerDay
	if bvs >= 1 {
		t.Fatalf("BvS = %v, want < 1", bvs)
	}
}

func TestEstimateSellUnitsPerDay_AllowsBvSAboveOne(t *testing.T) {
	daily := 100.0
	buyVol := int64(1000)
	sellVol := int64(500)

	sellPerDay := estimateSellUnitsPerDay(daily, buyVol, sellVol)
	if sellPerDay >= daily {
		t.Fatalf("sellPerDay = %v, want < %v", sellPerDay, daily)
	}

	bvs := daily / sellPerDay
	if bvs <= 1 {
		t.Fatalf("BvS = %v, want > 1", bvs)
	}
}

func TestStationExecutionDesiredQty(t *testing.T) {
	if got := stationExecutionDesiredQty(400, 1000, 300); got != 300 {
		t.Fatalf("desired qty with dailyShare cap = %d, want 300", got)
	}
	if got := stationExecutionDesiredQty(50, 40, 100); got != 40 {
		t.Fatalf("desired qty with depth cap = %d, want 40", got)
	}
	if got := stationExecutionDesiredQty(0, 5000, 8000); got != 1000 {
		t.Fatalf("fallback desired qty = %d, want 1000", got)
	}
	if got := stationExecutionDesiredQty(0, 0, 10); got != 0 {
		t.Fatalf("zero depth desired qty = %d, want 0", got)
	}
}

func TestStationExecutionDesiredQtyFromDailyShare(t *testing.T) {
	if got := stationExecutionDesiredQtyFromDailyShare(0, 5000, 8000); got != 0 {
		t.Fatalf("strict daily-share qty with unknown share = %d, want 0", got)
	}
	if got := stationExecutionDesiredQtyFromDailyShare(120, 1000, 90); got != 90 {
		t.Fatalf("strict daily-share qty with depth cap = %d, want 90", got)
	}
}

func TestResetExecutionDerivedFields(t *testing.T) {
	row := StationTrade{
		MarginPercent:        12.5,
		NowROI:               99,
		ExpectedBuyPrice:     10,
		ExpectedSellPrice:    20,
		ExpectedProfit:       3,
		RealProfit:           300,
		FilledQty:            100,
		CanFill:              true,
		SlippageBuyPct:       1.1,
		SlippageSellPct:      2.2,
		RealMarginPercent:    9.9,
		HasExecutionEvidence: true,
	}

	resetExecutionDerivedFields(&row)

	if row.ExpectedBuyPrice != 0 || row.ExpectedSellPrice != 0 {
		t.Fatalf("expected buy/sell prices to reset, got buy=%v sell=%v", row.ExpectedBuyPrice, row.ExpectedSellPrice)
	}
	if row.ExpectedProfit != 0 || row.RealProfit != 0 {
		t.Fatalf("expected profits to reset, got expected=%v real=%v", row.ExpectedProfit, row.RealProfit)
	}
	if row.FilledQty != 0 || row.CanFill {
		t.Fatalf("expected fill state reset, got qty=%d canFill=%v", row.FilledQty, row.CanFill)
	}
	if row.SlippageBuyPct != 0 || row.SlippageSellPct != 0 {
		t.Fatalf("expected slippage reset, got buy=%v sell=%v", row.SlippageBuyPct, row.SlippageSellPct)
	}
	if row.RealMarginPercent != 0 || row.HasExecutionEvidence {
		t.Fatalf("expected execution evidence reset, got margin=%v hasEvidence=%v", row.RealMarginPercent, row.HasExecutionEvidence)
	}
	if row.NowROI != row.MarginPercent {
		t.Fatalf("expected NowROI fallback to MarginPercent, got now=%v margin=%v", row.NowROI, row.MarginPercent)
	}
}

func TestEstimateSideFlowsPerDay_MassBalance(t *testing.T) {
	total := 100.0
	s2b, bfs := estimateSideFlowsPerDay(total, 600, 400)
	if s2b <= 0 || bfs <= 0 {
		t.Fatalf("flows should be positive, got s2b=%v bfs=%v", s2b, bfs)
	}
	gotTotal := s2b + bfs
	if gotTotal != total {
		t.Fatalf("mass-balance violated: s2b+bfs=%v, want %v", gotTotal, total)
	}

	halfS2B, halfBfS := estimateSideFlowsPerDay(total, 0, 0)
	if halfS2B != 50 || halfBfS != 50 {
		t.Fatalf("zero-depth split = %v/%v, want 50/50", halfS2B, halfBfS)
	}
}

func TestApplyStationTradeFilters_UsesExecutionAwareMarginsAndHistory(t *testing.T) {
	rows := []StationTrade{
		{
			TypeID:            1,
			MarginPercent:     20,
			RealMarginPercent: 2,
			ProfitPerUnit:     10_000,
			RealProfit:        1000,
			FilledQty:         1000,
			DailyVolume:       200,
			S2BPerDay:         100,
			BfSPerDay:         100,
			S2BBfSRatio:       1,
			HistoryAvailable:  true,
		},
	}
	params := StationTradeParams{
		MinMargin:      5,
		MinDailyVolume: 10,
	}
	out := applyStationTradeFilters(rows, params)
	if len(out) != 0 {
		t.Fatalf("expected row to be dropped by RealMarginPercent, got %d rows", len(out))
	}

	rows[0].RealMarginPercent = 8
	rows[0].HistoryAvailable = false
	out = applyStationTradeFilters(rows, params)
	if len(out) != 0 {
		t.Fatalf("expected row to be dropped by missing history, got %d rows", len(out))
	}

	rows[0].HistoryAvailable = true
	rows[0].RealMarginPercent = -1
	rows[0].FilledQty = 100
	rows[0].RealProfit = -500
	params.MinItemProfit = 1
	out = applyStationTradeFilters(rows, params)
	if len(out) != 0 {
		t.Fatalf("expected row to be dropped by execution-aware negative margin/profit, got %d rows", len(out))
	}

	rows[0].RealMarginPercent = 8
	rows[0].RealProfit = 100
	rows[0].FilledQty = 0
	rows[0].DailyVolume = 50
	out = applyStationTradeFilters(rows, StationTradeParams{})
	if len(out) != 1 {
		t.Fatalf("expected row to fallback to baseline maker economics, got %d rows", len(out))
	}
}

func TestStationMakerFallbackRealizationFactor_BoundsAndMonotone(t *testing.T) {
	lowConfHighComp := stationMakerFallbackRealizationFactor(10, 40)
	highConfLowComp := stationMakerFallbackRealizationFactor(90, 1)

	if lowConfHighComp < 0.2 || lowConfHighComp > 0.9 {
		t.Fatalf("lowConfHighComp out of bounds: %v", lowConfHighComp)
	}
	if highConfLowComp < 0.2 || highConfLowComp > 0.9 {
		t.Fatalf("highConfLowComp out of bounds: %v", highConfLowComp)
	}
	if highConfLowComp <= lowConfHighComp {
		t.Fatalf("expected high confidence + low competition to realize more: low=%v high=%v", lowConfHighComp, highConfLowComp)
	}

	sameConfLowComp := stationMakerFallbackRealizationFactor(60, 1)
	sameConfHighComp := stationMakerFallbackRealizationFactor(60, 60)
	if sameConfLowComp <= sameConfHighComp {
		t.Fatalf("expected queue penalty from higher competition: lowComp=%v highComp=%v", sameConfLowComp, sameConfHighComp)
	}
}

func TestStationConfidenceScore_SignalsAndPenalties(t *testing.T) {
	row := &StationTrade{
		HistoryAvailable: true,
		OBDS:             0.8,
		SDS:              10,
		PVI:              8,
		S2BPerDay:        600,
		BfSPerDay:        500,
	}
	base := stationConfidenceScore(row, 550, false)
	withExecution := stationConfidenceScore(row, 550, true)
	if withExecution <= base {
		t.Fatalf("execution evidence should increase confidence: base=%v withExecution=%v", base, withExecution)
	}

	row.IsExtremePriceFlag = true
	row.IsHighRiskFlag = true
	risky := stationConfidenceScore(row, 550, true)
	if risky >= withExecution {
		t.Fatalf("risk flags should decrease confidence: risky=%v withExecution=%v", risky, withExecution)
	}
}

func TestStationConfidenceLabelBuckets(t *testing.T) {
	if got := stationConfidenceLabel(10); got != "low" {
		t.Fatalf("low bucket = %q, want low", got)
	}
	if got := stationConfidenceLabel(55); got != "medium" {
		t.Fatalf("medium bucket = %q, want medium", got)
	}
	if got := stationConfidenceLabel(90); got != "high" {
		t.Fatalf("high bucket = %q, want high", got)
	}
}
