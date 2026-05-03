package engine

import "testing"

func TestEnrichRouteExecutionEstimates_MultiTripAndRiskAdjusted(t *testing.T) {
	routes := []RouteResult{
		{
			Hops: []RouteHop{
				{
					Units:      25,
					VolumeM3:   10,
					Profit:     600,
					EmptyJumps: 2,
					Jumps:      5,
				},
			},
			TotalProfit:             600,
			TotalJumps:              7,
			HaulingSafetyMultiplier: 2,
		},
	}

	EnrichRouteExecutionEstimates(routes, 100)
	got := routes[0]
	if got.CargoM3 != 250 {
		t.Fatalf("route cargo = %v, want 250", got.CargoM3)
	}
	if got.CargoTrips != 3 {
		t.Fatalf("route trips = %d, want 3", got.CargoTrips)
	}
	if got.ExecutionMinutes != 156 {
		t.Fatalf("route minutes = %v, want 156", got.ExecutionMinutes)
	}
	if got.ProfitPerHour < 230.7 || got.ProfitPerHour > 230.8 {
		t.Fatalf("route isk/h = %v, want about 230.77", got.ProfitPerHour)
	}
	hop := got.Hops[0]
	if hop.CargoM3 != 250 || hop.CargoTrips != 3 || hop.ExecutionMinutes != 156 {
		t.Fatalf("hop execution = cargo %v trips %d minutes %v, want 250/3/156", hop.CargoM3, hop.CargoTrips, hop.ExecutionMinutes)
	}
}

func TestRouteExecutionProfileFromParams_UsesRouteCargoAndShipTiming(t *testing.T) {
	params := RouteParams{
		CargoCapacity:           5000,
		RouteCargoCapacity:      100,
		RouteShipProfile:        "custom",
		RouteMinutesPerJump:     1,
		RouteDockMinutes:        2,
		RouteSafetyDelayPercent: 50,
	}
	routes := []RouteResult{
		{
			Hops: []RouteHop{
				{
					Units:      25,
					VolumeM3:   10,
					Profit:     600,
					EmptyJumps: 2,
					Jumps:      5,
				},
			},
			TotalProfit:             600,
			HaulingSafetyMultiplier: 2,
		},
	}

	EnrichRouteExecutionEstimatesWithProfile(routes, RouteExecutionProfileFromParams(params))

	got := routes[0]
	if got.CargoTrips != 3 {
		t.Fatalf("route trips = %d, want 3", got.CargoTrips)
	}
	if got.ExecutionMinutes != 117 {
		t.Fatalf("route minutes = %v, want 117", got.ExecutionMinutes)
	}
	if got.ProfitPerHour < 307.6 || got.ProfitPerHour > 307.7 {
		t.Fatalf("route isk/h = %v, want about 307.69", got.ProfitPerHour)
	}
}

func TestRouteExecutionProfileFromParams_UsesBuiltInShipCargo(t *testing.T) {
	params := RouteParams{
		CargoCapacity:    5000,
		RouteShipProfile: "freighter",
	}
	profile := RouteExecutionProfileFromParams(params)
	if profile.CargoCapacity != 850000 {
		t.Fatalf("cargo = %v, want 850000", profile.CargoCapacity)
	}
	if profile.MinutesPerJump != 3.6 {
		t.Fatalf("minutes per jump = %v, want 3.6", profile.MinutesPerJump)
	}
	if profile.SafetyDelayPercent != 20 {
		t.Fatalf("safety delay = %v, want 20", profile.SafetyDelayPercent)
	}
}

func TestEnrichRouteExecutionEstimates_AddsCourierCollateralAndReward(t *testing.T) {
	routes := []RouteResult{
		{
			Hops: []RouteHop{
				{
					Units:    10,
					VolumeM3: 20,
					BuyPrice: 50_000_000,
					Profit:   80_000_000,
					Jumps:    4,
				},
			},
			TotalProfit:      80_000_000,
			TotalJumps:       4,
			HaulingRiskKnown: true,
			HaulingDanger:    "yellow",
			HaulingRiskScore: 50,
		},
	}

	EnrichRouteExecutionEstimates(routes, 1_000)

	got := routes[0]
	if got.CargoValueISK != 500_000_000 {
		t.Fatalf("cargo value = %v, want 500m", got.CargoValueISK)
	}
	if got.CourierCollateralISK != 550_000_000 {
		t.Fatalf("collateral = %v, want 550m", got.CourierCollateralISK)
	}
	if got.CourierRiskPremiumPercent != 4 {
		t.Fatalf("premium = %v, want 4", got.CourierRiskPremiumPercent)
	}
	if got.CourierRewardFloorISK != 20_000_000 {
		t.Fatalf("reward floor = %v, want 20m", got.CourierRewardFloorISK)
	}
	if got.CourierProfitAfterRewardISK != 60_000_000 {
		t.Fatalf("profit after reward = %v, want 60m", got.CourierProfitAfterRewardISK)
	}
	if !got.CourierViable {
		t.Fatal("courier should be viable")
	}
}
