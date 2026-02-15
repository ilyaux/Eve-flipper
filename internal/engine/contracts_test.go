package engine

import (
	"testing"
)

// getContractFilters returns effective minPrice, maxMargin, minPricedRatio (defaults when params are 0).

func TestGetContractFilters_Defaults(t *testing.T) {
	var params ScanParams
	minPrice, maxMargin, minPricedRatio := getContractFilters(params)
	if minPrice != DefaultMinContractPrice {
		t.Errorf("minPrice = %v, want DefaultMinContractPrice %v", minPrice, DefaultMinContractPrice)
	}
	if maxMargin != DefaultMaxContractMargin {
		t.Errorf("maxMargin = %v, want DefaultMaxContractMargin %v", maxMargin, DefaultMaxContractMargin)
	}
	if minPricedRatio != DefaultMinPricedRatio {
		t.Errorf("minPricedRatio = %v, want DefaultMinPricedRatio %v", minPricedRatio, DefaultMinPricedRatio)
	}
}

func TestGetContractFilters_Explicit(t *testing.T) {
	params := ScanParams{
		MinContractPrice:  50_000_000,
		MaxContractMargin: 80,
		MinPricedRatio:    0.9,
	}
	minPrice, maxMargin, minPricedRatio := getContractFilters(params)
	if minPrice != 50_000_000 {
		t.Errorf("minPrice = %v, want 50_000_000", minPrice)
	}
	if maxMargin != 80 {
		t.Errorf("maxMargin = %v, want 80", maxMargin)
	}
	if minPricedRatio != 0.9 {
		t.Errorf("minPricedRatio = %v, want 0.9", minPricedRatio)
	}
}

func TestGetContractFilters_PartialDefaults(t *testing.T) {
	// Only MinContractPrice set; others use defaults
	params := ScanParams{MinContractPrice: 1_000_000}
	minPrice, maxMargin, minPricedRatio := getContractFilters(params)
	if minPrice != 1_000_000 {
		t.Errorf("minPrice = %v, want 1_000_000", minPrice)
	}
	if maxMargin != DefaultMaxContractMargin {
		t.Errorf("maxMargin = %v, want default %v", maxMargin, DefaultMaxContractMargin)
	}
	if minPricedRatio != DefaultMinPricedRatio {
		t.Errorf("minPricedRatio = %v, want default %v", minPricedRatio, DefaultMinPricedRatio)
	}
}

func TestGetContractFilters_ZeroMaxMarginUsesDefault(t *testing.T) {
	params := ScanParams{MaxContractMargin: 0}
	_, maxMargin, _ := getContractFilters(params)
	if maxMargin != DefaultMaxContractMargin {
		t.Errorf("maxMargin when 0 = %v, want default %v", maxMargin, DefaultMaxContractMargin)
	}
}

func TestGetContractFilters_MinPricedRatioPercentAndClamp(t *testing.T) {
	params := ScanParams{MinPricedRatio: 80} // accidental percent input from API client
	_, _, minPricedRatio := getContractFilters(params)
	if minPricedRatio != 0.8 {
		t.Errorf("minPricedRatio(80) = %v, want 0.8", minPricedRatio)
	}

	params = ScanParams{MinPricedRatio: 0.01}
	_, _, minPricedRatio = getContractFilters(params)
	if minPricedRatio != 0.1 {
		t.Errorf("minPricedRatio lower clamp = %v, want 0.1", minPricedRatio)
	}
}

func TestContractSellValueMultiplier_InstantLiquidation_IgnoresBroker(t *testing.T) {
	params := ScanParams{
		SalesTaxPercent:            8,
		BrokerFeePercent:           3,
		ContractInstantLiquidation: true,
	}
	got := contractSellValueMultiplier(params)
	want := 0.92 // 1 - 8%
	if got != want {
		t.Errorf("contractSellValueMultiplier instant = %v, want %v", got, want)
	}
}

func TestContractSellValueMultiplier_Estimate_IncludesBroker(t *testing.T) {
	params := ScanParams{
		SalesTaxPercent:            8,
		BrokerFeePercent:           3,
		ContractInstantLiquidation: false,
	}
	got := contractSellValueMultiplier(params)
	want := 0.89 // 1 - (8% + 3%)
	if got != want {
		t.Errorf("contractSellValueMultiplier estimate = %v, want %v", got, want)
	}
}

func TestContractHoldDays_DefaultAndClamp(t *testing.T) {
	if got := contractHoldDays(ScanParams{}); got != DefaultContractHoldDays {
		t.Errorf("contractHoldDays default = %d, want %d", got, DefaultContractHoldDays)
	}
	if got := contractHoldDays(ScanParams{ContractHoldDays: 365}); got != 180 {
		t.Errorf("contractHoldDays clamp = %d, want 180", got)
	}
}

func TestContractTargetConfidence_DefaultAndClamp(t *testing.T) {
	if got := contractTargetConfidence(ScanParams{}); got != DefaultContractTargetConfidence {
		t.Errorf("contractTargetConfidence default = %v, want %v", got, DefaultContractTargetConfidence)
	}
	if got := contractTargetConfidence(ScanParams{ContractTargetConfidence: 140}); got != 100 {
		t.Errorf("contractTargetConfidence clamp = %v, want 100", got)
	}
}

func TestEstimateFillDaysAndProbability(t *testing.T) {
	fillDays := estimateFillDays(350, 100) // effective/day = 35
	if fillDays != 10 {
		t.Errorf("estimateFillDays = %v, want 10", fillDays)
	}
	p := fillProbabilityWithinDays(fillDays, 7)
	if p <= 0 || p >= 1 {
		t.Errorf("fillProbabilityWithinDays = %v, want (0,1)", p)
	}
	// No volume => impossible fill in model.
	if p0 := fillProbabilityWithinDays(estimateFillDays(10, 0), 7); p0 != 0 {
		t.Errorf("fillProbabilityWithinDays(no volume) = %v, want 0", p0)
	}
}

func TestIsRig(t *testing.T) {
	tests := []struct {
		name    string
		groupID int32
		want    bool
	}{
		{"Small rig group", 28, true},
		{"Medium rig group", 54, true},
		{"Large rig group", 80, true},
		{"Capital rig group", 106, true},
		{"Armor rig group", 132, true},
		{"Shield rig group", 158, true},
		{"Astronautic rig group", 184, true},
		{"Projectile weapon rig group", 210, true},
		{"Drone rig group", 236, true},
		{"Launcher rig group", 262, true},
		{"Energy weapon rig group", 289, true},
		{"Hybrid weapon rig group", 290, true},
		{"Electronic superiority rig group", 291, true},
		{"Ship group (not a rig)", 25, false},       // Frigate
		{"Module group (not a rig)", 53, false},     // Energy Weapon
		{"Ammunition group (not a rig)", 83, false}, // Hybrid Charge
		{"Unknown group", 99999, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRig(tt.groupID); got != tt.want {
				t.Errorf("isRig(groupID=%d) = %v, want %v", tt.groupID, got, tt.want)
			}
		})
	}
}
