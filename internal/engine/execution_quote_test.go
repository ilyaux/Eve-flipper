package engine

import (
	"math"
	"testing"

	"eve-flipper/internal/esi"
)

func assertClose(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}

func TestComputeExecutionQuoteFullFillFeesAndShipping(t *testing.T) {
	quote := ComputeExecutionQuote(ExecutionQuoteInput{
		TypeID:       34,
		RequestedQty: 150,
		BuyOrders: []esi.MarketOrder{
			{Price: 100, VolumeRemain: 100},
			{Price: 110, VolumeRemain: 100},
		},
		SellOrders: []esi.MarketOrder{
			{Price: 160, VolumeRemain: 100, IsBuyOrder: true},
			{Price: 150, VolumeRemain: 100, IsBuyOrder: true},
		},
		PackagedVolumeM3:      2,
		ShippingCostPerM3Jump: 3,
		ShippingJumps:         4,
		Fees: ExecutionQuoteFeeInputs{
			BrokerFeePercent: 1,
			SalesTaxPercent:  2,
		},
	})

	if quote.Decision != "SAFE" {
		t.Fatalf("Decision = %s, want SAFE (%v)", quote.Decision, quote.Warnings)
	}
	if quote.FillQty != 150 {
		t.Fatalf("FillQty = %d, want 150", quote.FillQty)
	}
	assertClose(t, "BuyVWAP", quote.BuyVWAP, 15500.0/150.0)
	assertClose(t, "SellVWAP", quote.SellVWAP, 23500.0/150.0)
	assertClose(t, "BuyFees", quote.BuyFees, 155)
	assertClose(t, "SellFees", quote.SellFees, 705)
	assertClose(t, "ShippingCost", quote.ShippingCost, 3600)
	assertClose(t, "NetProfit", quote.NetProfit, 3540)
	assertClose(t, "FilledVolumeM3", quote.FilledVolumeM3, 300)
}

func TestComputeExecutionQuotePartialDepthRecomputesVWAP(t *testing.T) {
	quote := ComputeExecutionQuote(ExecutionQuoteInput{
		TypeID:       34,
		RequestedQty: 100,
		BuyOrders: []esi.MarketOrder{
			{Price: 100, VolumeRemain: 100},
		},
		SellOrders: []esi.MarketOrder{
			{Price: 150, VolumeRemain: 40, IsBuyOrder: true},
		},
		PackagedVolumeM3: 1,
	})

	if quote.Decision != "CHANGED" {
		t.Fatalf("Decision = %s, want CHANGED", quote.Decision)
	}
	if quote.FillQty != 40 {
		t.Fatalf("FillQty = %d, want 40", quote.FillQty)
	}
	if quote.PartialReason != "sell_depth" {
		t.Fatalf("PartialReason = %q, want sell_depth", quote.PartialReason)
	}
	assertClose(t, "BuyVWAP", quote.BuyVWAP, 100)
	assertClose(t, "SellVWAP", quote.SellVWAP, 150)
	assertClose(t, "NetProfit", quote.NetProfit, 2000)
	if quote.Buy.FilledQty != 40 || quote.Sell.FilledQty != 40 {
		t.Fatalf("side filled qty = buy %d sell %d, want 40/40", quote.Buy.FilledQty, quote.Sell.FilledQty)
	}
}

func TestComputeExecutionQuoteUnprofitableAfterCosts(t *testing.T) {
	quote := ComputeExecutionQuote(ExecutionQuoteInput{
		TypeID:       34,
		RequestedQty: 10,
		BuyOrders: []esi.MarketOrder{
			{Price: 100, VolumeRemain: 10},
		},
		SellOrders: []esi.MarketOrder{
			{Price: 101, VolumeRemain: 10, IsBuyOrder: true},
		},
		PackagedVolumeM3:      1,
		ShippingCostPerM3Jump: 10,
		ShippingJumps:         1,
	})

	if quote.Decision != "DANGER" {
		t.Fatalf("Decision = %s, want DANGER", quote.Decision)
	}
	if quote.PartialReason != "unprofitable_after_fees" {
		t.Fatalf("PartialReason = %q, want unprofitable_after_fees", quote.PartialReason)
	}
	if quote.NetProfit >= 0 {
		t.Fatalf("NetProfit = %v, want negative", quote.NetProfit)
	}
}
