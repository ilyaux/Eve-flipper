package engine

import "math"

type RouteTimeEstimate struct {
	Minutes    float64
	Jumps      int
	Trips      int
	CargoM3    float64
	SafetyMult float64
	Danger     string
	Kills      int
}

func estimateBacktestRouteTime(row FlipResult, params FlipBacktestParams, qty int32) RouteTimeEstimate {
	trips := estimateCargoTrips(row, params, qty)
	buyJumps := row.BuyJumps
	if buyJumps < 0 {
		buyJumps = 0
	}
	sellJumps := row.SellJumps
	if sellJumps < 0 {
		sellJumps = 0
	}

	jumps := 0
	if sellJumps > 0 {
		// For multi-trip hauling, every extra load requires returning from target
		// to source before hauling the next batch.
		jumps = buyJumps + (2*trips-1)*sellJumps
	} else if row.TotalJumps > 0 {
		jumps = row.TotalJumps * trips
	} else {
		jumps = buyJumps * trips
	}
	if jumps < 0 {
		jumps = 0
	}

	dockStops := trips * 2
	minutes := (float64(jumps) * params.RouteMinutesPerJump) + (float64(dockStops) * params.RouteDockMinutes)
	safetyMult := routeTimeSafetyMultiplier(row, params)
	minutes *= safetyMult
	if minutes < float64(params.RouteMinCooldownMin) {
		minutes = float64(params.RouteMinCooldownMin)
	}
	return RouteTimeEstimate{
		Minutes:    sanitizeFloat(minutes),
		Jumps:      jumps,
		Trips:      trips,
		CargoM3:    sanitizeFloat(float64(qty) * row.Volume),
		SafetyMult: sanitizeFloat(safetyMult),
		Danger:     row.RouteSafetyDanger,
		Kills:      row.RouteSafetyKills,
	}
}

func routeTimeSafetyMultiplier(row FlipResult, params FlipBacktestParams) float64 {
	if params.RouteSafetyMode == "auto" {
		if row.RouteSafetyMultiplier > 0 && !math.IsNaN(row.RouteSafetyMultiplier) && !math.IsInf(row.RouteSafetyMultiplier, 0) {
			if row.RouteSafetyMultiplier > 10 {
				return 10
			}
			return row.RouteSafetyMultiplier
		}
		return 1
	}
	return params.RouteSafetyMult
}

func estimateCargoTrips(row FlipResult, params FlipBacktestParams, qty int32) int {
	if qty <= 0 || row.Volume <= 0 || params.CargoCapacity <= 0 {
		return 1
	}
	cargoM3 := float64(qty) * row.Volume
	if cargoM3 <= 0 || math.IsNaN(cargoM3) || math.IsInf(cargoM3, 0) {
		return 1
	}
	trips := int(math.Ceil(cargoM3 / params.CargoCapacity))
	if trips < 1 {
		return 1
	}
	return trips
}
