package engine

import (
	"math"
	"testing"
)

func TestTradeFeeMultipliers_LegacyFallback(t *testing.T) {
	buyMult, sellMult := tradeFeeMultipliers(tradeFeeInputs{
		SplitTradeFees:   false,
		BrokerFeePercent: 3,
		SalesTaxPercent:  8,
	})

	// Legacy: buy side broker only, sell side broker + tax.
	if math.Abs(buyMult-1.03) > 1e-9 {
		t.Fatalf("buyMult = %v, want 1.03", buyMult)
	}
	if math.Abs(sellMult-0.89) > 1e-9 {
		t.Fatalf("sellMult = %v, want 0.89", sellMult)
	}
}

func TestTradeFeeMultipliers_SplitMode(t *testing.T) {
	buyMult, sellMult := tradeFeeMultipliers(tradeFeeInputs{
		SplitTradeFees:       true,
		BuyBrokerFeePercent:  0.5,
		SellBrokerFeePercent: 0.2,
		BuySalesTaxPercent:   0.1,
		SellSalesTaxPercent:  3.6,
	})

	if math.Abs(buyMult-1.006) > 1e-9 {
		t.Fatalf("buyMult = %v, want 1.006", buyMult)
	}
	if math.Abs(sellMult-0.962) > 1e-9 {
		t.Fatalf("sellMult = %v, want 0.962", sellMult)
	}
}

func TestTradeFeeMultipliers_Clamp(t *testing.T) {
	buyMult, sellMult := tradeFeeMultipliers(tradeFeeInputs{
		SplitTradeFees:       true,
		BuyBrokerFeePercent:  -5,
		SellBrokerFeePercent: 200,
		BuySalesTaxPercent:   -1,
		SellSalesTaxPercent:  50,
	})

	if math.Abs(buyMult-1.0) > 1e-9 {
		t.Fatalf("buyMult = %v, want 1.0", buyMult)
	}
	if sellMult != 0 {
		t.Fatalf("sellMult = %v, want 0", sellMult)
	}
}
