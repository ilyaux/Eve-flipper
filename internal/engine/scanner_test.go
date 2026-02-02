package engine

import (
	"math"
	"testing"
)

func TestSanitizeFloat_Normal(t *testing.T) {
	if v := sanitizeFloat(42.5); v != 42.5 {
		t.Errorf("sanitizeFloat(42.5) = %v, want 42.5", v)
	}
}

func TestSanitizeFloat_Zero(t *testing.T) {
	if v := sanitizeFloat(0); v != 0 {
		t.Errorf("sanitizeFloat(0) = %v, want 0", v)
	}
}

func TestSanitizeFloat_NaN(t *testing.T) {
	if v := sanitizeFloat(math.NaN()); v != 0 {
		t.Errorf("sanitizeFloat(NaN) = %v, want 0", v)
	}
}

func TestSanitizeFloat_PosInf(t *testing.T) {
	if v := sanitizeFloat(math.Inf(1)); v != 0 {
		t.Errorf("sanitizeFloat(+Inf) = %v, want 0", v)
	}
}

func TestSanitizeFloat_NegInf(t *testing.T) {
	if v := sanitizeFloat(math.Inf(-1)); v != 0 {
		t.Errorf("sanitizeFloat(-Inf) = %v, want 0", v)
	}
}

func TestSanitizeFloat_Negative(t *testing.T) {
	if v := sanitizeFloat(-100.5); v != -100.5 {
		t.Errorf("sanitizeFloat(-100.5) = %v, want -100.5", v)
	}
}

func TestProfitCalculation(t *testing.T) {
	// Simulate the core profit formula from calculateResults
	salesTaxPercent := 8.0
	taxMult := 1.0 - salesTaxPercent/100 // 0.92

	sellPrice := 100.0   // cheapest sell order (we buy here)
	buyPrice := 200.0    // highest buy order (we sell here)
	cargoCapacity := 500.0
	itemVolume := 10.0

	effectiveSellPrice := buyPrice * taxMult // 184
	profitPerUnit := effectiveSellPrice - sellPrice // 84
	margin := profitPerUnit / sellPrice * 100 // 84%

	units := int32(math.Floor(cargoCapacity / itemVolume)) // 50
	totalProfit := profitPerUnit * float64(units) // 4200

	if math.Abs(taxMult-0.92) > 1e-9 {
		t.Errorf("taxMult = %v, want 0.92", taxMult)
	}
	if math.Abs(effectiveSellPrice-184) > 1e-9 {
		t.Errorf("effectiveSellPrice = %v, want 184", effectiveSellPrice)
	}
	if math.Abs(profitPerUnit-84) > 1e-9 {
		t.Errorf("profitPerUnit = %v, want 84", profitPerUnit)
	}
	if math.Abs(margin-84) > 1e-9 {
		t.Errorf("margin = %v%%, want 84%%", margin)
	}
	if units != 50 {
		t.Errorf("units = %d, want 50", units)
	}
	if math.Abs(totalProfit-4200) > 1e-9 {
		t.Errorf("totalProfit = %v, want 4200", totalProfit)
	}
}

func TestProfitCalculation_ZeroTax(t *testing.T) {
	taxMult := 1.0 - 0.0/100
	buyPrice := 150.0
	sellPrice := 100.0
	effective := buyPrice * taxMult
	profit := effective - sellPrice

	if math.Abs(profit-50) > 1e-9 {
		t.Errorf("profit with 0%% tax = %v, want 50", profit)
	}
}

func TestProfitCalculation_HighTax(t *testing.T) {
	taxMult := 1.0 - 100.0/100 // 0
	buyPrice := 150.0
	sellPrice := 100.0
	effective := buyPrice * taxMult
	profit := effective - sellPrice

	if math.Abs(profit-(-100)) > 1e-9 {
		t.Errorf("profit with 100%% tax = %v, want -100", profit)
	}
}
