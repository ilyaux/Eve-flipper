package engine

import "testing"

func TestSortRouteResultsByMode_FastestPrioritizesExecutionTime(t *testing.T) {
	routes := []RouteResult{
		{TotalProfit: 1000, TotalJumps: 5, ExecutionMinutes: 60, ProfitPerHour: 1000},
		{TotalProfit: 700, TotalJumps: 3, ExecutionMinutes: 20, ProfitPerHour: 2100},
	}

	SortRouteResultsByMode(routes, RouteModeFastest)

	if routes[0].ExecutionMinutes != 20 {
		t.Fatalf("first route minutes = %v, want 20", routes[0].ExecutionMinutes)
	}
}

func TestSortRouteResultsByMode_SafestPrioritizesRisk(t *testing.T) {
	routes := []RouteResult{
		{
			TotalProfit:             5000,
			TotalJumps:              4,
			ExecutionMinutes:        30,
			ProfitPerHour:           10000,
			HaulingRiskKnown:        true,
			HaulingDanger:           "red",
			HaulingRiskScore:        80,
			HaulingSafetyMultiplier: 2,
		},
		{
			TotalProfit:             1000,
			TotalJumps:              6,
			ExecutionMinutes:        90,
			ProfitPerHour:           700,
			HaulingRiskKnown:        true,
			HaulingDanger:           "green",
			HaulingRiskScore:        2,
			HaulingSafetyMultiplier: 1,
		},
	}

	SortRouteResultsByMode(routes, RouteModeSafest)

	if routes[0].HaulingDanger != "green" {
		t.Fatalf("first route danger = %q, want green", routes[0].HaulingDanger)
	}
}

func TestSortRouteResultsByMode_BalancedUsesRiskAdjustedISKHour(t *testing.T) {
	routes := []RouteResult{
		{
			TotalProfit:             3000,
			TotalJumps:              4,
			ProfitPerHour:           2000,
			LiquidityScore:          60,
			HaulingRiskKnown:        true,
			HaulingDanger:           "red",
			HaulingRiskScore:        90,
			HaulingSafetyMultiplier: 2.5,
		},
		{
			TotalProfit:             2000,
			TotalJumps:              5,
			ProfitPerHour:           1500,
			LiquidityScore:          80,
			HaulingRiskKnown:        true,
			HaulingDanger:           "green",
			HaulingRiskScore:        5,
			HaulingSafetyMultiplier: 1,
		},
	}

	SortRouteResultsByMode(routes, RouteModeBalanced)

	if routes[0].HaulingDanger != "green" {
		t.Fatalf("first route danger = %q, want balanced to prefer safer adjusted route", routes[0].HaulingDanger)
	}
}
