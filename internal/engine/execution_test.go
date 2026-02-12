package engine

import (
	"math"
	"testing"

	"eve-flipper/internal/esi"
)

// ComputeExecutionPlan: buy = walk sell orders from lowest; sell = walk buy orders from highest.
// Aggregates by price level, then fills quantity and computes expected price and slippage.

func TestComputeExecutionPlan_Buy_Exact(t *testing.T) {
	// Sell orders: 100@100, 110@200. Buy 150 units: fill 100@100 + 50@110. Cost = 10000+5500=15500, expected = 15500/150 = 103.333...
	sellOrders := []esi.MarketOrder{
		{Price: 100, VolumeRemain: 100},
		{Price: 110, VolumeRemain: 200},
	}
	got := ComputeExecutionPlan(sellOrders, 150, true)
	if math.Abs(got.BestPrice-100) > 1e-9 {
		t.Errorf("BestPrice = %v, want 100", got.BestPrice)
	}
	wantExpected := 15500.0 / 150
	if math.Abs(got.ExpectedPrice-wantExpected) > 1e-6 {
		t.Errorf("ExpectedPrice = %v, want %v", got.ExpectedPrice, wantExpected)
	}
	if math.Abs(got.TotalCost-wantExpected*150) > 1e-3 {
		t.Errorf("TotalCost = %v, want %v", got.TotalCost, wantExpected*150)
	}
	wantSlippage := (wantExpected - 100) / 100 * 100
	if math.Abs(got.SlippagePercent-wantSlippage) > 1e-3 {
		t.Errorf("SlippagePercent = %v, want %v", got.SlippagePercent, wantSlippage)
	}
	if !got.CanFill {
		t.Error("CanFill want true")
	}
	if got.TotalDepth != 300 {
		t.Errorf("TotalDepth = %v, want 300", got.TotalDepth)
	}
}

func TestComputeExecutionPlan_Sell_Exact(t *testing.T) {
	// Buy orders: 90@100, 85@200. Sell 150: fill 100@90 + 50@85. Revenue = 9000+4250=13250, expected = 13250/150 = 88.333...
	buyOrders := []esi.MarketOrder{
		{Price: 90, VolumeRemain: 100},
		{Price: 85, VolumeRemain: 200},
	}
	got := ComputeExecutionPlan(buyOrders, 150, false)
	if math.Abs(got.BestPrice-90) > 1e-9 {
		t.Errorf("BestPrice (sell) = %v, want 90", got.BestPrice)
	}
	wantExpected := (100.0*90 + 50*85) / 150
	if math.Abs(got.ExpectedPrice-wantExpected) > 1e-6 {
		t.Errorf("ExpectedPrice = %v, want %v", got.ExpectedPrice, wantExpected)
	}
	if !got.CanFill {
		t.Error("CanFill want true")
	}
}

func TestComputeExecutionPlan_CannotFill(t *testing.T) {
	orders := []esi.MarketOrder{{Price: 100, VolumeRemain: 50}}
	got := ComputeExecutionPlan(orders, 100, true)
	if got.CanFill {
		t.Error("CanFill want false when depth < quantity")
	}
	if got.TotalDepth != 50 {
		t.Errorf("TotalDepth = %v, want 50", got.TotalDepth)
	}
	if math.Abs(got.ExpectedPrice-100) > 1e-9 {
		t.Errorf("ExpectedPrice = %v, want 100", got.ExpectedPrice)
	}
	// Must be based on actually filled volume (50), not requested quantity (100).
	if math.Abs(got.TotalCost-5000) > 1e-9 {
		t.Errorf("TotalCost = %v, want 5000", got.TotalCost)
	}
}

func TestComputeExecutionPlan_CannotFill_SellTotalCostUsesFilledQuantity(t *testing.T) {
	orders := []esi.MarketOrder{{Price: 80, VolumeRemain: 40}}
	got := ComputeExecutionPlan(orders, 100, false)
	if got.CanFill {
		t.Error("CanFill want false when depth < quantity")
	}
	if math.Abs(got.ExpectedPrice-80) > 1e-9 {
		t.Errorf("ExpectedPrice = %v, want 80", got.ExpectedPrice)
	}
	// For sell side TotalCost represents expected revenue for the executable part.
	if math.Abs(got.TotalCost-3200) > 1e-9 {
		t.Errorf("TotalCost = %v, want 3200", got.TotalCost)
	}
}

func TestComputeExecutionPlan_EmptyOrZeroQty(t *testing.T) {
	orders := []esi.MarketOrder{{Price: 100, VolumeRemain: 10}}
	if got := ComputeExecutionPlan(nil, 5, true); got.BestPrice != 0 || got.CanFill {
		t.Errorf("ComputeExecutionPlan(nil) should return zero result")
	}
	if got := ComputeExecutionPlan(orders, 0, true); got.BestPrice != 0 {
		t.Errorf("ComputeExecutionPlan(qty=0) should return zero result")
	}
}

func TestComputeExecutionPlan_SamePriceAggregated(t *testing.T) {
	// Two orders at same price: volume should be summed
	orders := []esi.MarketOrder{
		{Price: 100, VolumeRemain: 30},
		{Price: 100, VolumeRemain: 70},
	}
	got := ComputeExecutionPlan(orders, 50, true)
	if math.Abs(got.BestPrice-100) > 1e-9 {
		t.Errorf("BestPrice = %v, want 100", got.BestPrice)
	}
	if got.ExpectedPrice != 100 {
		t.Errorf("ExpectedPrice = %v, want 100 (single level)", got.ExpectedPrice)
	}
	if got.TotalDepth != 100 {
		t.Errorf("TotalDepth = %v, want 100", got.TotalDepth)
	}
}
