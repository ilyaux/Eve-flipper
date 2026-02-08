package engine

import (
	"math"
	"testing"

	"eve-flipper/internal/esi"
)

// ===================================================================
// Helper: generate synthetic price history
// ===================================================================

// generateHistory creates n days of synthetic PLEX history starting at basePrice
// with a small daily drift and fixed volume.
func generateHistory(n int, basePrice float64, dailyDrift float64, volume int64) []esi.HistoryEntry {
	h := make([]esi.HistoryEntry, n)
	price := basePrice
	for i := 0; i < n; i++ {
		h[i] = esi.HistoryEntry{
			Date:    dayStr(i),
			Average: price,
			Highest: price * 1.02,
			Lowest:  price * 0.98,
			Volume:  volume,
		}
		price += dailyDrift
	}
	return h
}

func dayStr(i int) string {
	// 2025-01-01 + i days, simplified
	d := i + 1
	m := 1
	for d > 28 {
		d -= 28
		m++
	}
	return "2025-" + pad(m) + "-" + pad(d)
}

func pad(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

func almostEqual(a, b, tolerance float64) bool {
	return math.Abs(a-b) <= tolerance
}

// ===================================================================
// Tests: sortHistory
// ===================================================================

func TestSortHistory(t *testing.T) {
	unsorted := []esi.HistoryEntry{
		{Date: "2025-01-03", Average: 3},
		{Date: "2025-01-01", Average: 1},
		{Date: "2025-01-02", Average: 2},
	}
	sorted := sortHistory(unsorted)
	if len(sorted) != 3 {
		t.Fatalf("expected 3, got %d", len(sorted))
	}
	if sorted[0].Date != "2025-01-01" || sorted[1].Date != "2025-01-02" || sorted[2].Date != "2025-01-03" {
		t.Errorf("unexpected order: %v", sorted)
	}
	// Ensure original is not modified
	if unsorted[0].Date != "2025-01-03" {
		t.Error("original slice was modified")
	}
}

func TestSortHistoryEmpty(t *testing.T) {
	sorted := sortHistory(nil)
	if sorted != nil {
		t.Errorf("expected nil, got %v", sorted)
	}
}

// ===================================================================
// Tests: bestSellPrice / bestBuyPrice
// ===================================================================

func TestBestSellPrice(t *testing.T) {
	orders := []esi.MarketOrder{
		{Price: 5_000_000, IsBuyOrder: false},
		{Price: 4_500_000, IsBuyOrder: false},
		{Price: 4_800_000, IsBuyOrder: false},
		{Price: 4_000_000, IsBuyOrder: true}, // buy order, should be ignored
	}
	if got := bestSellPrice(orders); got != 4_500_000 {
		t.Errorf("expected 4500000, got %f", got)
	}
}

func TestBestSellPriceEmpty(t *testing.T) {
	if got := bestSellPrice(nil); got != 0 {
		t.Errorf("expected 0, got %f", got)
	}
}

func TestBestBuyPrice(t *testing.T) {
	orders := []esi.MarketOrder{
		{Price: 4_000_000, IsBuyOrder: true},
		{Price: 4_200_000, IsBuyOrder: true},
		{Price: 5_000_000, IsBuyOrder: false},
	}
	if got := bestBuyPrice(orders); got != 4_200_000 {
		t.Errorf("expected 4200000, got %f", got)
	}
}

func TestBestBuyPriceEmpty(t *testing.T) {
	if got := bestBuyPrice(nil); got != 0 {
		t.Errorf("expected 0, got %f", got)
	}
}

// ===================================================================
// Tests: safeDiv
// ===================================================================

func TestSafeDiv(t *testing.T) {
	if v := safeDiv(10, 2); v != 5 {
		t.Errorf("10/2 = %f, want 5", v)
	}
	if v := safeDiv(10, 0); v != 0 {
		t.Errorf("10/0 = %f, want 0", v)
	}
	if v := safeDiv(0, 0); v != 0 {
		t.Errorf("0/0 = %f, want 0", v)
	}
}

// ===================================================================
// Tests: convertHistory
// ===================================================================

func TestConvertHistoryTruncation(t *testing.T) {
	// Generate more than MaxHistoryPoints entries
	h := generateHistory(MaxHistoryPoints+20, 5_000_000, 1000, 50000)
	sorted := sortHistory(h)
	pts := convertHistory(sorted)

	if len(pts) != MaxHistoryPoints {
		t.Errorf("expected %d points, got %d", MaxHistoryPoints, len(pts))
	}
	// First point should correspond to the 21st entry (0-indexed: 20)
	if pts[0].Date != sorted[20].Date {
		t.Errorf("expected first date %s, got %s", sorted[20].Date, pts[0].Date)
	}
}

func TestConvertHistoryEmpty(t *testing.T) {
	if pts := convertHistory(nil); pts != nil {
		t.Errorf("expected nil, got %v", pts)
	}
}

// ===================================================================
// Tests: computeGlobalPLEXPrice
// ===================================================================

func TestComputeGlobalPLEXPrice(t *testing.T) {
	orders := []esi.MarketOrder{
		{Price: 4_500_000, IsBuyOrder: true},
		{Price: 4_600_000, IsBuyOrder: true},
		{Price: 5_000_000, IsBuyOrder: false},
		{Price: 4_900_000, IsBuyOrder: false},
	}
	history := generateHistory(5, 4_800_000, 0, 100_000)

	gp := computeGlobalPLEXPrice(orders, sortHistory(history))

	if gp.BuyPrice != 4_600_000 {
		t.Errorf("BuyPrice = %f, want 4600000", gp.BuyPrice)
	}
	if gp.SellPrice != 4_900_000 {
		t.Errorf("SellPrice = %f, want 4900000", gp.SellPrice)
	}
	if gp.BuyOrders != 2 {
		t.Errorf("BuyOrders = %d, want 2", gp.BuyOrders)
	}
	if gp.SellOrders != 2 {
		t.Errorf("SellOrders = %d, want 2", gp.SellOrders)
	}
	expectedSpread := 4_900_000.0 - 4_600_000.0
	if gp.Spread != expectedSpread {
		t.Errorf("Spread = %f, want %f", gp.Spread, expectedSpread)
	}
	if gp.Volume24h != 100_000 {
		t.Errorf("Volume24h = %d, want 100000", gp.Volume24h)
	}
}

func TestComputeGlobalPLEXPriceEmpty(t *testing.T) {
	gp := computeGlobalPLEXPrice(nil, nil)
	if gp.BuyPrice != 0 || gp.SellPrice != 0 {
		t.Errorf("expected zero prices for empty orders, got buy=%f sell=%f", gp.BuyPrice, gp.SellPrice)
	}
}

// ===================================================================
// Tests: computeIndicators
// ===================================================================

func TestComputeIndicatorsNil(t *testing.T) {
	// Less than 7 data points → nil
	h := generateHistory(5, 5_000_000, 0, 50000)
	ind := computeIndicators(sortHistory(h))
	if ind != nil {
		t.Errorf("expected nil indicators for 5 entries, got %+v", ind)
	}
}

func TestComputeIndicatorsSMA7(t *testing.T) {
	// 7 constant-price entries → SMA7 == price
	h := generateHistory(7, 5_000_000, 0, 50000)
	ind := computeIndicators(sortHistory(h))
	if ind == nil {
		t.Fatal("expected non-nil indicators")
	}
	if !almostEqual(ind.SMA7, 5_000_000, 1) {
		t.Errorf("SMA7 = %f, want 5000000", ind.SMA7)
	}
	// SMA30 should be 0 (not enough data)
	if ind.SMA30 != 0 {
		t.Errorf("SMA30 should be 0 with only 7 entries, got %f", ind.SMA30)
	}
}

func TestComputeIndicatorsSMA30(t *testing.T) {
	h := generateHistory(30, 5_000_000, 0, 50000)
	ind := computeIndicators(sortHistory(h))
	if ind == nil {
		t.Fatal("expected non-nil indicators")
	}
	if !almostEqual(ind.SMA30, 5_000_000, 1) {
		t.Errorf("SMA30 = %f, want 5000000", ind.SMA30)
	}
}

func TestComputeIndicatorsRSI_Neutral(t *testing.T) {
	// Flat price → RSI should be 50 (no movement)
	h := generateHistory(30, 5_000_000, 0, 50000)
	ind := computeIndicators(sortHistory(h))
	if ind == nil {
		t.Fatal("expected non-nil indicators")
	}
	if !almostEqual(ind.RSI, 50, 0.1) {
		t.Errorf("RSI = %f, want 50 (flat)", ind.RSI)
	}
}

func TestComputeIndicatorsRSI_AllUp(t *testing.T) {
	// Strictly increasing prices → RSI should be 100
	h := generateHistory(30, 5_000_000, 10_000, 50000)
	ind := computeIndicators(sortHistory(h))
	if ind == nil {
		t.Fatal("expected non-nil indicators")
	}
	if ind.RSI != 100 {
		t.Errorf("RSI = %f, want 100 (all up)", ind.RSI)
	}
}

func TestComputeIndicatorsRSI_AllDown(t *testing.T) {
	// Strictly decreasing prices → RSI should be 0
	h := generateHistory(30, 5_000_000, -10_000, 50000)
	ind := computeIndicators(sortHistory(h))
	if ind == nil {
		t.Fatal("expected non-nil indicators")
	}
	if !almostEqual(ind.RSI, 0, 0.1) {
		t.Errorf("RSI = %f, want ~0 (all down)", ind.RSI)
	}
}

func TestComputeIndicatorsBollingerBands(t *testing.T) {
	// Constant price → BB upper == lower == middle == price
	h := generateHistory(25, 5_000_000, 0, 50000)
	ind := computeIndicators(sortHistory(h))
	if ind == nil {
		t.Fatal("expected non-nil indicators")
	}
	if !almostEqual(ind.BollingerMiddle, 5_000_000, 1) {
		t.Errorf("BB Middle = %f, want 5000000", ind.BollingerMiddle)
	}
	// With no variance, upper == lower == middle
	if !almostEqual(ind.BollingerUpper, ind.BollingerMiddle, 1) {
		t.Errorf("BB Upper = %f, should equal middle %f (no variance)", ind.BollingerUpper, ind.BollingerMiddle)
	}
	if !almostEqual(ind.BollingerLower, ind.BollingerMiddle, 1) {
		t.Errorf("BB Lower = %f, should equal middle %f (no variance)", ind.BollingerLower, ind.BollingerMiddle)
	}
}

func TestComputeIndicatorsVolumeAnomaly(t *testing.T) {
	// Normal volumes for 29 days, then a massive spike
	// Use a large enough price drop so Change24h < -3%
	// price[28] = 5_000_000 - 28*200_000 = -600_000 ... need to keep positive.
	// Better: start at 5M, drop steeply only on last day
	h := generateHistory(30, 5_000_000, 0, 10_000)
	h[29].Volume = 1_000_000             // 100x normal volume
	h[29].Average = h[28].Average * 0.93 // -7% drop on last day (well below -3%)
	ind := computeIndicators(sortHistory(h))
	if ind == nil {
		t.Fatal("expected non-nil indicators")
	}
	if ind.VolumeSigma <= 2 {
		t.Errorf("VolumeSigma = %f, expected > 2 for 100x volume spike", ind.VolumeSigma)
	}
	if ind.Change24h >= -3 {
		t.Errorf("Change24h = %f, expected < -3%% for CCP sale detection", ind.Change24h)
	}
	// With price dropping and volume spike, CCP sale should be detected
	if !ind.CCPSaleSignal {
		t.Error("expected CCPSaleSignal to be true")
	}
}

func TestComputeIndicatorsPriceChanges(t *testing.T) {
	// Generate 30 days, price goes from 5M to 5.29M (+10k/day)
	h := generateHistory(30, 5_000_000, 10_000, 50000)
	ind := computeIndicators(sortHistory(h))
	if ind == nil {
		t.Fatal("expected non-nil indicators")
	}
	// Change24h: (price[29] - price[28]) / price[28] * 100
	// price[28] = 5_000_000 + 28*10_000 = 5_280_000
	// price[29] = 5_290_000
	expected24h := (5_290_000.0 - 5_280_000.0) / 5_280_000.0 * 100
	if !almostEqual(ind.Change24h, expected24h, 0.01) {
		t.Errorf("Change24h = %f, want %f", ind.Change24h, expected24h)
	}
}

// ===================================================================
// Tests: computeSignal
// ===================================================================

func TestComputeSignalNilIndicators(t *testing.T) {
	sig := computeSignal(nil, PLEXGlobalPrice{})
	if sig.Action != "HOLD" {
		t.Errorf("action = %s, want HOLD", sig.Action)
	}
}

func TestComputeSignalBuy_RSIOversold(t *testing.T) {
	ind := &PLEXIndicators{RSI: 25}
	gp := PLEXGlobalPrice{SellPrice: 5_000_000}
	sig := computeSignal(ind, gp)
	if sig.Action != "BUY" {
		t.Errorf("action = %s, want BUY (RSI oversold)", sig.Action)
	}
}

func TestComputeSignalSell_RSIOverbought(t *testing.T) {
	ind := &PLEXIndicators{RSI: 75}
	gp := PLEXGlobalPrice{SellPrice: 5_000_000}
	sig := computeSignal(ind, gp)
	if sig.Action != "SELL" {
		t.Errorf("action = %s, want SELL (RSI overbought)", sig.Action)
	}
}

func TestComputeSignalBuy_CCPSale(t *testing.T) {
	ind := &PLEXIndicators{RSI: 50, CCPSaleSignal: true}
	gp := PLEXGlobalPrice{SellPrice: 5_000_000}
	sig := computeSignal(ind, gp)
	if sig.Action != "BUY" {
		t.Errorf("action = %s, want BUY (CCP sale)", sig.Action)
	}
	if sig.Confidence <= 0 {
		t.Error("expected positive confidence for CCP sale signal")
	}
}

func TestComputeSignalConfidenceCap(t *testing.T) {
	// Stack all buy signals
	ind := &PLEXIndicators{
		RSI:            20,
		SMA7:           4_000_000,
		SMA30:          5_000_000,
		BollingerLower: 5_100_000,
		BollingerUpper: 6_000_000,
		Change7d:       -10,
		CCPSaleSignal:  true,
	}
	gp := PLEXGlobalPrice{SellPrice: 5_000_000}
	sig := computeSignal(ind, gp)
	if sig.Confidence > 95 {
		t.Errorf("confidence = %f, should be capped at 95", sig.Confidence)
	}
}

// ===================================================================
// Tests: computeSPFarm
// ===================================================================

func TestComputeSPFarm_Basic(t *testing.T) {
	// PLEX price = 5M, extractor sell = 300M, injector sell = 900M
	plexPrice := 5_000_000.0
	extractorSell := 300_000_000.0
	injectorSell := 900_000_000.0
	injectorBuy := 850_000_000.0
	netMult := 0.954      // 3.6% tax + 1% broker = 4.6%
	salesTaxOnly := 0.964 // 3.6% tax only
	nesExtractor := 293
	nesOmega := 500
	nesMPTC := 485

	result := computeSPFarm(plexPrice, extractorSell, injectorSell, injectorBuy, netMult, salesTaxOnly, nesExtractor, nesOmega, nesMPTC)

	// Omega cost: 500 * 5M = 2.5B
	expectedOmegaCost := 500 * 5_000_000.0
	if !almostEqual(result.OmegaCostISK, expectedOmegaCost, 1) {
		t.Errorf("OmegaCostISK = %f, want %f", result.OmegaCostISK, expectedOmegaCost)
	}

	// Extractors per month: 2250 * 720 / 500_000 = 3.24
	expectedExtractors := BaseSPPerHour * HoursPerMonth / float64(SPPerExtractor)
	if !almostEqual(result.ExtractorsPerMonth, expectedExtractors, 0.01) {
		t.Errorf("ExtractorsPerMonth = %f, want %f", result.ExtractorsPerMonth, expectedExtractors)
	}

	// StartupSP should be MinSPForExtraction
	if result.StartupSP != MinSPForExtraction {
		t.Errorf("StartupSP = %d, want %d", result.StartupSP, MinSPForExtraction)
	}

	// Startup train days: (5_000_000 + 500_000) / 2250 / 24
	expectedStartupDays := (float64(MinSPForExtraction) + float64(SPPerExtractor)) / BaseSPPerHour / 24
	if !almostEqual(result.StartupTrainDays, expectedStartupDays, 0.1) {
		t.Errorf("StartupTrainDays = %f, want %f", result.StartupTrainDays, expectedStartupDays)
	}

	// MPTC cost
	expectedMPTCCostISK := float64(nesMPTC) * plexPrice
	if !almostEqual(result.MPTCCostISK, expectedMPTCCostISK, 1) {
		t.Errorf("MPTCCostISK = %f, want %f", result.MPTCCostISK, expectedMPTCCostISK)
	}

	// Omega ISK value
	if !almostEqual(result.OmegaISKValue, expectedOmegaCost, 1) {
		t.Errorf("OmegaISKValue = %f, want %f", result.OmegaISKValue, expectedOmegaCost)
	}

	// PLEX unit price
	if result.PLEXUnitPrice != plexPrice {
		t.Errorf("PLEXUnitPrice = %f, want %f", result.PLEXUnitPrice, plexPrice)
	}

	// Instant sell should have values
	if result.InstantSellRevenueISK <= 0 {
		t.Error("InstantSellRevenueISK should be > 0")
	}
	if result.InstantSellROI == 0 {
		t.Error("InstantSellROI should be non-zero")
	}
}

func TestComputeSPFarm_PaybackZeroWhenNotViable(t *testing.T) {
	// Set injector price extremely low so profit is negative
	result := computeSPFarm(5_000_000, 300_000_000, 100_000_000, 90_000_000, 0.954, 0.964, 293, 500, 485)
	if result.Viable {
		t.Error("should not be viable with 100M injector price")
	}
	if result.PaybackDays != 0 {
		t.Errorf("PaybackDays = %f, want 0 when not viable", result.PaybackDays)
	}
}

// ===================================================================
// Tests: computeChartOverlays
// ===================================================================

func TestComputeChartOverlaysNil(t *testing.T) {
	// < 7 entries → nil
	h := generateHistory(5, 5_000_000, 0, 50000)
	ov := computeChartOverlays(sortHistory(h))
	if ov != nil {
		t.Error("expected nil overlays for < 7 entries")
	}
}

func TestComputeChartOverlaysSMA7(t *testing.T) {
	h := generateHistory(10, 5_000_000, 0, 50000)
	sorted := sortHistory(h)
	ov := computeChartOverlays(sorted)
	if ov == nil {
		t.Fatal("expected non-nil overlays")
	}
	// SMA7 should have 10-7+1 = 4 points
	if len(ov.SMA7) != 4 {
		t.Errorf("SMA7 has %d points, want 4", len(ov.SMA7))
	}
	// All constant price → SMA should equal price
	for _, p := range ov.SMA7 {
		if !almostEqual(p.Value, 5_000_000, 1) {
			t.Errorf("SMA7 point value = %f, want 5000000", p.Value)
		}
	}
}

func TestComputeChartOverlaysBB(t *testing.T) {
	h := generateHistory(25, 5_000_000, 0, 50000)
	sorted := sortHistory(h)
	ov := computeChartOverlays(sorted)
	if ov == nil {
		t.Fatal("expected non-nil overlays")
	}
	// BB should have 25-20+1 = 6 points
	if len(ov.BollingerUp) != 6 {
		t.Errorf("BollingerUp has %d points, want 6", len(ov.BollingerUp))
	}
	// Constant price → upper == lower == price
	for _, p := range ov.BollingerUp {
		if !almostEqual(p.Value, 5_000_000, 1) {
			t.Errorf("BB Upper = %f, want 5000000 (no variance)", p.Value)
		}
	}
}

// ===================================================================
// Tests: ComputePLEXDashboard (integration)
// ===================================================================

func TestComputePLEXDashboard_Integration(t *testing.T) {
	plexOrders := []esi.MarketOrder{
		{Price: 4_500_000, IsBuyOrder: true, TypeID: PLEXTypeID},
		{Price: 5_000_000, IsBuyOrder: false, TypeID: PLEXTypeID},
	}
	related := map[int32][]esi.MarketOrder{
		SkillExtractorTypeID: {
			{Price: 300_000_000, IsBuyOrder: false},
			{Price: 280_000_000, IsBuyOrder: true},
		},
		LargeSkillInjTypeID: {
			{Price: 900_000_000, IsBuyOrder: false},
			{Price: 850_000_000, IsBuyOrder: true},
		},
		MPTCTypeID: {
			{Price: 600_000_000, IsBuyOrder: false},
			{Price: 550_000_000, IsBuyOrder: true},
		},
	}
	history := generateHistory(90, 4_800_000, 2000, 80_000)
	nes := NESPrices{ExtractorPLEX: 293, MPTCPLEX: 485, OmegaPLEX: 500}

	dash := ComputePLEXDashboard(plexOrders, related, history, nil, 3.6, 1.0, nes, 0, nil)

	// PLEX price
	if dash.PLEXPrice.SellPrice != 5_000_000 {
		t.Errorf("SellPrice = %f, want 5000000", dash.PLEXPrice.SellPrice)
	}

	// Should have 8 arbitrage paths (4 NES/market + 4 spread)
	if len(dash.Arbitrage) != 8 {
		t.Errorf("expected 8 arbitrage paths, got %d", len(dash.Arbitrage))
	}

	// All arbitrage paths should have data
	for _, arb := range dash.Arbitrage {
		if arb.NoData {
			t.Errorf("arb path %q should have data", arb.Name)
		}
	}

	// SP Farm should be computed
	if dash.SPFarm.ExtractorsPerMonth <= 0 {
		t.Error("SP farm extractors should be > 0")
	}

	// Indicators should be non-nil (90 data points)
	if dash.Indicators == nil {
		t.Error("expected non-nil indicators with 90 history entries")
	}

	// Chart overlays should be non-nil
	if dash.ChartOverlays == nil {
		t.Error("expected non-nil chart overlays with 90 history entries")
	}

	// Signal should have an action
	if dash.Signal.Action != "BUY" && dash.Signal.Action != "SELL" && dash.Signal.Action != "HOLD" {
		t.Errorf("unexpected signal action: %s", dash.Signal.Action)
	}

	// History should be truncated to MaxHistoryPoints
	if len(dash.History) != MaxHistoryPoints {
		t.Errorf("expected %d history points, got %d", MaxHistoryPoints, len(dash.History))
	}

	// SP Farm new fields
	if dash.SPFarm.StartupSP != MinSPForExtraction {
		t.Errorf("StartupSP = %d, want %d", dash.SPFarm.StartupSP, MinSPForExtraction)
	}
	if dash.SPFarm.StartupTrainDays <= 0 {
		t.Error("StartupTrainDays should be > 0")
	}
	if dash.SPFarm.MPTCCostISK <= 0 {
		t.Error("MPTCCostISK should be > 0")
	}
	if dash.SPFarm.OmegaISKValue <= 0 {
		t.Error("OmegaISKValue should be > 0")
	}
}

func TestComputePLEXDashboard_NoDataPaths(t *testing.T) {
	plexOrders := []esi.MarketOrder{
		{Price: 5_000_000, IsBuyOrder: false, TypeID: PLEXTypeID},
	}
	// No related orders at all
	related := map[int32][]esi.MarketOrder{}
	history := generateHistory(30, 5_000_000, 0, 50000)
	nes := NESPrices{}

	dash := ComputePLEXDashboard(plexOrders, related, history, nil, 0, 0, nes, 0, nil)

	for _, arb := range dash.Arbitrage {
		if !arb.NoData {
			t.Errorf("arb path %q should have NoData=true when no related orders", arb.Name)
		}
		if arb.Viable {
			t.Errorf("arb path %q should not be viable with no data", arb.Name)
		}
	}
}

func TestComputePLEXDashboard_DefaultTaxes(t *testing.T) {
	plexOrders := []esi.MarketOrder{
		{Price: 5_000_000, IsBuyOrder: false, TypeID: PLEXTypeID},
	}
	related := map[int32][]esi.MarketOrder{}
	history := generateHistory(10, 5_000_000, 0, 50000)
	nes := NESPrices{}

	// Pass 0 for tax/broker → should use defaults (3.6 / 1.0)
	dash := ComputePLEXDashboard(plexOrders, related, history, nil, 0, 0, nes, 0, nil)

	// The SP farm omega cost should reflect default nesOmega (500)
	expectedOmega := 500 * 5_000_000.0
	if !almostEqual(dash.SPFarm.OmegaCostISK, expectedOmega, 1) {
		t.Errorf("OmegaCostISK = %f, want %f (default Omega PLEX)", dash.SPFarm.OmegaCostISK, expectedOmega)
	}
}

// ===================================================================
// Tests: NESPrices.Resolve
// ===================================================================

func TestNESPricesResolve_Defaults(t *testing.T) {
	nes := NESPrices{}
	ext, mptc, omega := nes.Resolve()
	if ext != DefaultNESExtractorPLEX {
		t.Errorf("ext = %d, want %d", ext, DefaultNESExtractorPLEX)
	}
	if mptc != DefaultNESMPTCPLEX {
		t.Errorf("mptc = %d, want %d", mptc, DefaultNESMPTCPLEX)
	}
	if omega != DefaultNESOmegaPLEX {
		t.Errorf("omega = %d, want %d", omega, DefaultNESOmegaPLEX)
	}
}

func TestNESPricesResolve_Custom(t *testing.T) {
	nes := NESPrices{ExtractorPLEX: 250, MPTCPLEX: 400, OmegaPLEX: 450}
	ext, mptc, omega := nes.Resolve()
	if ext != 250 || mptc != 400 || omega != 450 {
		t.Errorf("unexpected resolved values: %d, %d, %d", ext, mptc, omega)
	}
}
