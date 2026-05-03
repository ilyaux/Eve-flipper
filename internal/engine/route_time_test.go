package engine

import "testing"

func TestEstimateBacktestRouteTime_MultiTripRoundRoute(t *testing.T) {
	row := FlipResult{
		Volume:     10,
		BuyJumps:   3,
		SellJumps:  5,
		TotalJumps: 8,
	}
	params := normalizeFlipBacktestParams(FlipBacktestParams{
		CargoCapacity:       50,
		RouteMinutesPerJump: 2,
		RouteDockMinutes:    4,
		RouteSafetyMult:     1.5,
	})

	got := estimateBacktestRouteTime(row, params, 10)
	if got.Trips != 2 {
		t.Fatalf("trips = %d, want 2", got.Trips)
	}
	if got.Jumps != 18 {
		t.Fatalf("jumps = %d, want 18", got.Jumps)
	}
	// jumps 18 * 2 min + 4 dock stops * 4 min = 52; safety x1.5 = 78.
	if got.Minutes != 78 {
		t.Fatalf("minutes = %v, want 78", got.Minutes)
	}
}

func TestEstimateBacktestRouteTime_MinCooldownFloor(t *testing.T) {
	row := FlipResult{Volume: 1, BuyJumps: 0, SellJumps: 1}
	params := normalizeFlipBacktestParams(FlipBacktestParams{
		CargoCapacity:       100,
		RouteMinutesPerJump: 1,
		RouteDockMinutes:    0,
		RouteSafetyMult:     1,
		RouteMinCooldownMin: 15,
	})

	got := estimateBacktestRouteTime(row, params, 1)
	if got.Minutes != 15 {
		t.Fatalf("minutes = %v, want floor 15", got.Minutes)
	}
}

func TestEstimateBacktestRouteTime_AutoSafetyMultiplier(t *testing.T) {
	row := FlipResult{
		Volume:                1,
		SellJumps:             10,
		RouteSafetyMultiplier: 2,
		RouteSafetyDanger:     "red",
		RouteSafetyKills:      4,
	}
	params := normalizeFlipBacktestParams(FlipBacktestParams{
		CooldownMode:        "route_time",
		RouteSafetyMode:     "auto",
		RouteMinutesPerJump: 1,
		RouteDockMinutes:    0,
	})

	got := estimateBacktestRouteTime(row, params, 1)
	if got.Minutes != 20 {
		t.Fatalf("minutes = %v, want 20", got.Minutes)
	}
	if got.SafetyMult != 2 || got.Danger != "red" || got.Kills != 4 {
		t.Fatalf("risk fields = mult %v danger %q kills %d", got.SafetyMult, got.Danger, got.Kills)
	}
}
