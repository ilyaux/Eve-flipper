package api

import (
	"testing"

	"eve-flipper/internal/engine"
	"eve-flipper/internal/gankcheck"
)

func TestRouteHaulingRiskSummary(t *testing.T) {
	var summary routeHaulingRiskSummary
	summary.add([]gankcheck.SystemDanger{
		{SystemID: 1, DangerLevel: "yellow", KillsTotal: 1, TotalISK: 500_000_000, Security: 0.6},
		{SystemID: 2, DangerLevel: "red", KillsTotal: 3, TotalISK: 2_000_000_000, IsSmartbomb: true, Security: 0.4},
		{SystemID: 2, DangerLevel: "red", KillsTotal: 3, TotalISK: 2_000_000_000, IsSmartbomb: true, Security: 0.4},
	})

	var route engine.RouteResult
	summary.applyTo(&route)

	if !route.HaulingRiskKnown {
		t.Fatal("expected risk to be known")
	}
	if route.HaulingDanger != "red" {
		t.Fatalf("danger = %s, want red", route.HaulingDanger)
	}
	if route.HaulingKills != 4 {
		t.Fatalf("kills = %d, want unique-system total 4", route.HaulingKills)
	}
	if route.HaulingRiskScore <= 0 || route.HaulingRiskScore > 100 {
		t.Fatalf("risk score out of range: %v", route.HaulingRiskScore)
	}
}

func TestRouteSafetyMultiplierFromSummary(t *testing.T) {
	var yellow routeHaulingRiskSummary
	yellow.add([]gankcheck.SystemDanger{{SystemID: 1, DangerLevel: "yellow", KillsTotal: 1, Security: 0.6}})
	if got := routeSafetyMultiplierFromSummary(yellow); got < 1.25 {
		t.Fatalf("yellow multiplier = %v, want at least 1.25", got)
	}

	var red routeHaulingRiskSummary
	red.add([]gankcheck.SystemDanger{{SystemID: 2, DangerLevel: "red", KillsTotal: 5, TotalISK: 5_000_000_000, IsSmartbomb: true, Security: 0.4}})
	if got := routeSafetyMultiplierFromSummary(red); got < 1.75 || got > 3 {
		t.Fatalf("red multiplier = %v, want within [1.75, 3]", got)
	}
}
