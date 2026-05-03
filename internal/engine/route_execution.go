package engine

import (
	"math"
	"strings"
)

const (
	defaultRouteMinutesPerJump = 2.0
	defaultRouteDockMinutes    = 4.0
)

type RouteExecutionProfile struct {
	ShipProfile        string
	CargoCapacity      float64
	MinutesPerJump     float64
	DockMinutes        float64
	SafetyDelayPercent float64
}

func RouteExecutionProfileFromParams(params RouteParams) RouteExecutionProfile {
	return normalizeRouteExecutionProfile(RouteExecutionProfile{
		ShipProfile:        params.RouteShipProfile,
		CargoCapacity:      params.EffectiveRouteCargoCapacity(),
		MinutesPerJump:     params.RouteMinutesPerJump,
		DockMinutes:        params.RouteDockMinutes,
		SafetyDelayPercent: params.RouteSafetyDelayPercent,
	})
}

func (params RouteParams) EffectiveRouteCargoCapacity() float64 {
	if isPositiveFinite(params.RouteCargoCapacity) {
		return params.RouteCargoCapacity
	}
	profile := routeShipProfileDefaults(params.RouteShipProfile)
	if profile.ShipProfile != "custom" && isPositiveFinite(profile.CargoCapacity) {
		return profile.CargoCapacity
	}
	if isPositiveFinite(params.CargoCapacity) {
		return params.CargoCapacity
	}
	return profile.CargoCapacity
}

// EnrichRouteExecutionEstimates adds a practical hauling-time layer to route
// results. It is intentionally deterministic and conservative: cargo capacity
// drives multi-trip hauling, every extra trip includes a return leg, and gank
// risk can stretch the time estimate through HaulingSafetyMultiplier.
func EnrichRouteExecutionEstimates(routes []RouteResult, cargoCapacity float64) {
	EnrichRouteExecutionEstimatesWithProfile(routes, RouteExecutionProfile{CargoCapacity: cargoCapacity})
}

func EnrichRouteExecutionEstimatesWithProfile(routes []RouteResult, profile RouteExecutionProfile) {
	profile = normalizeRouteExecutionProfile(profile)
	for i := range routes {
		enrichRouteExecutionEstimate(&routes[i], profile)
	}
}

func enrichRouteExecutionEstimate(route *RouteResult, profile RouteExecutionProfile) {
	if route == nil {
		return
	}
	safetyMult := route.HaulingSafetyMultiplier
	if safetyMult <= 0 || math.IsNaN(safetyMult) || math.IsInf(safetyMult, 0) {
		safetyMult = 1
	}
	safetyMult *= 1 + profile.SafetyDelayPercent/100
	if safetyMult > 10 {
		safetyMult = 10
	}

	var cargoM3 float64
	var cargoValueISK float64
	var cargoTrips int
	var baseMinutes float64
	for i := range route.Hops {
		hop := &route.Hops[i]
		hop.CargoM3 = sanitizeFloat(float64(hop.Units) * hop.VolumeM3)
		cargoValueISK += float64(hop.Units) * hop.BuyPrice
		hop.CargoTrips = routeCargoTrips(hop.CargoM3, profile.CargoCapacity)
		if hop.CargoTrips > cargoTrips {
			cargoTrips = hop.CargoTrips
		}
		cargoM3 += hop.CargoM3

		jumps := hop.EmptyJumps
		if hop.Jumps > 0 {
			jumps += (2*hop.CargoTrips - 1) * hop.Jumps
		}
		if jumps < 0 {
			jumps = 0
		}
		dockStops := hop.CargoTrips * 2
		hopMinutes := float64(jumps)*profile.MinutesPerJump + float64(dockStops)*profile.DockMinutes
		hop.ExecutionMinutes = sanitizeFloat(hopMinutes * safetyMult)
		if hop.ExecutionMinutes > 0 {
			hop.ProfitPerHour = sanitizeFloat(hop.Profit / (hop.ExecutionMinutes / 60))
		} else {
			hop.ProfitPerHour = 0
		}
		baseMinutes += hopMinutes
	}

	targetMinutes := float64(max(0, route.TargetJumps)) * profile.MinutesPerJump
	totalMinutes := (baseMinutes + targetMinutes) * safetyMult
	route.CargoM3 = sanitizeFloat(cargoM3)
	route.CargoTrips = cargoTrips
	route.ExecutionMinutes = sanitizeFloat(totalMinutes)
	if route.ExecutionMinutes > 0 {
		route.ProfitPerHour = sanitizeFloat(route.TotalProfit / (route.ExecutionMinutes / 60))
	} else {
		route.ProfitPerHour = 0
	}
	if route.HaulingSafetyMultiplier <= 0 && safetyMult > 1 {
		route.HaulingSafetyMultiplier = safetyMult
	}
	enrichRouteCourierCollateral(route, cargoValueISK)
}

func enrichRouteCourierCollateral(route *RouteResult, cargoValueISK float64) {
	if route == nil {
		return
	}
	route.CargoValueISK = sanitizeFloat(cargoValueISK)
	if cargoValueISK <= 0 {
		route.CourierCollateralISK = 0
		route.CourierRewardFloorISK = 0
		route.CourierRewardPerJumpISK = 0
		route.CourierProfitAfterRewardISK = sanitizeFloat(route.TotalProfit)
		route.CourierRiskPremiumPercent = 0
		route.CourierViable = route.TotalProfit > 0
		return
	}

	riskPremiumPct := routeCourierRiskPremiumPercent(route)
	rewardFloor := math.Max(1_000_000, cargoValueISK*riskPremiumPct/100)
	if route.TotalJumps > 0 {
		rewardFloor = math.Max(rewardFloor, float64(route.TotalJumps)*1_000_000)
	}
	if route.CargoTrips > 1 {
		rewardFloor *= 1 + math.Min(float64(route.CargoTrips-1)*0.2, 1.0)
	}

	route.CourierRiskPremiumPercent = sanitizeFloat(riskPremiumPct)
	route.CourierCollateralISK = sanitizeFloat(cargoValueISK * 1.10)
	route.CourierRewardFloorISK = sanitizeFloat(rewardFloor)
	if route.TotalJumps > 0 {
		route.CourierRewardPerJumpISK = sanitizeFloat(rewardFloor / float64(route.TotalJumps))
	} else {
		route.CourierRewardPerJumpISK = sanitizeFloat(rewardFloor)
	}
	route.CourierProfitAfterRewardISK = sanitizeFloat(route.TotalProfit - rewardFloor)
	route.CourierViable = route.CourierProfitAfterRewardISK > 0
	if strings.EqualFold(route.HaulingDanger, "red") && route.CourierProfitAfterRewardISK < rewardFloor {
		route.CourierViable = false
	}
}

func routeCourierRiskPremiumPercent(route *RouteResult) float64 {
	pct := 1.0
	switch strings.ToLower(strings.TrimSpace(route.HaulingDanger)) {
	case "yellow":
		pct = 3.0
	case "red":
		pct = 8.0
	}
	if route.HaulingRiskKnown && route.HaulingRiskScore > 0 {
		pct += math.Min(route.HaulingRiskScore/100*2, 4)
	}
	if route.CargoTrips > 1 {
		pct += math.Min(float64(route.CargoTrips-1)*0.25, 1.5)
	}
	if pct < 0.5 {
		return 0.5
	}
	if pct > 12 {
		return 12
	}
	return pct
}

func normalizeRouteExecutionProfile(profile RouteExecutionProfile) RouteExecutionProfile {
	defaults := routeShipProfileDefaults(profile.ShipProfile)
	profile.ShipProfile = normalizeRouteShipProfile(profile.ShipProfile)
	if !isPositiveFinite(profile.CargoCapacity) {
		profile.CargoCapacity = defaults.CargoCapacity
	}
	if !isPositiveFinite(profile.MinutesPerJump) {
		profile.MinutesPerJump = defaults.MinutesPerJump
	}
	if !isPositiveFinite(profile.DockMinutes) {
		profile.DockMinutes = defaults.DockMinutes
	}
	if math.IsNaN(profile.SafetyDelayPercent) || math.IsInf(profile.SafetyDelayPercent, 0) || profile.SafetyDelayPercent <= 0 {
		profile.SafetyDelayPercent = defaults.SafetyDelayPercent
	}
	if profile.SafetyDelayPercent > 500 {
		profile.SafetyDelayPercent = 500
	}
	if !isPositiveFinite(profile.MinutesPerJump) {
		profile.MinutesPerJump = defaultRouteMinutesPerJump
	}
	if !isPositiveFinite(profile.DockMinutes) {
		profile.DockMinutes = defaultRouteDockMinutes
	}
	return profile
}

func routeShipProfileDefaults(profile string) RouteExecutionProfile {
	switch normalizeRouteShipProfile(profile) {
	case "fast_frigate":
		return RouteExecutionProfile{ShipProfile: "fast_frigate", CargoCapacity: 400, MinutesPerJump: 1.2, DockMinutes: 2.5}
	case "sunesis":
		return RouteExecutionProfile{ShipProfile: "sunesis", CargoCapacity: 1500, MinutesPerJump: 1.4, DockMinutes: 3}
	case "blockade_runner":
		return RouteExecutionProfile{ShipProfile: "blockade_runner", CargoCapacity: 10000, MinutesPerJump: 1.6, DockMinutes: 3.5, SafetyDelayPercent: 5}
	case "deep_space_transport":
		return RouteExecutionProfile{ShipProfile: "deep_space_transport", CargoCapacity: 60000, MinutesPerJump: 2.1, DockMinutes: 4.5, SafetyDelayPercent: 10}
	case "freighter":
		return RouteExecutionProfile{ShipProfile: "freighter", CargoCapacity: 850000, MinutesPerJump: 3.6, DockMinutes: 7, SafetyDelayPercent: 20}
	default:
		return RouteExecutionProfile{ShipProfile: "custom", CargoCapacity: 0, MinutesPerJump: defaultRouteMinutesPerJump, DockMinutes: defaultRouteDockMinutes}
	}
}

func normalizeRouteShipProfile(profile string) string {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "fast_frigate", "sunesis", "blockade_runner", "deep_space_transport", "freighter":
		return strings.ToLower(strings.TrimSpace(profile))
	default:
		return "custom"
	}
}

func isPositiveFinite(v float64) bool {
	return v > 0 && !math.IsNaN(v) && !math.IsInf(v, 0)
}

func routeCargoTrips(cargoM3, cargoCapacity float64) int {
	if cargoM3 <= 0 || cargoCapacity <= 0 || math.IsNaN(cargoM3) || math.IsNaN(cargoCapacity) || math.IsInf(cargoM3, 0) || math.IsInf(cargoCapacity, 0) {
		return 1
	}
	trips := int(math.Ceil(cargoM3 / cargoCapacity))
	if trips < 1 {
		return 1
	}
	return trips
}
