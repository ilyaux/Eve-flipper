package engine

import (
	"math"
	"sort"
	"strings"
)

const (
	RouteModeBalanced = "balanced"
	RouteModeFastest  = "fastest"
	RouteModeSafest   = "safest"
)

func NormalizeRouteMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case RouteModeFastest:
		return RouteModeFastest
	case RouteModeSafest:
		return RouteModeSafest
	default:
		return RouteModeBalanced
	}
}

func routeSearchScore(profit float64, jumps int, mode string) float64 {
	if profit <= 0 {
		return profit
	}
	jumpsF := float64(max(1, jumps))
	switch NormalizeRouteMode(mode) {
	case RouteModeFastest:
		return profit / math.Pow(jumpsF, 1.35)
	case RouteModeSafest:
		return profit / math.Pow(jumpsF, 1.15)
	default:
		return profit
	}
}

func SortRouteResultsByMode(routes []RouteResult, mode string) {
	mode = NormalizeRouteMode(mode)
	sort.SliceStable(routes, func(i, j int) bool {
		left := routes[i]
		right := routes[j]
		switch mode {
		case RouteModeFastest:
			leftMinutes := normalizedRouteMinutes(left)
			rightMinutes := normalizedRouteMinutes(right)
			if !floatNearlyEqual(leftMinutes, rightMinutes) {
				return leftMinutes < rightMinutes
			}
			if !floatNearlyEqual(left.ProfitPerHour, right.ProfitPerHour) {
				return left.ProfitPerHour > right.ProfitPerHour
			}
			return routeProfitTieBreak(left, right)
		case RouteModeSafest:
			leftRisk := routeRiskSortScore(left, true)
			rightRisk := routeRiskSortScore(right, true)
			if !floatNearlyEqual(leftRisk, rightRisk) {
				return leftRisk < rightRisk
			}
			if !floatNearlyEqual(left.ProfitPerHour, right.ProfitPerHour) {
				return left.ProfitPerHour > right.ProfitPerHour
			}
			return routeProfitTieBreak(left, right)
		default:
			leftScore := routeBalancedSortScore(left)
			rightScore := routeBalancedSortScore(right)
			if !floatNearlyEqual(leftScore, rightScore) {
				return leftScore > rightScore
			}
			return routeProfitTieBreak(left, right)
		}
	})
}

func routeBalancedSortScore(route RouteResult) float64 {
	base := route.ProfitPerHour
	if base <= 0 || math.IsNaN(base) || math.IsInf(base, 0) {
		base = route.TotalProfit / float64(max(1, route.TotalJumps))
	}
	liquidityMult := 1.0
	if route.LiquidityScore > 0 {
		liquidityMult = 0.75 + clampRouteFloat(route.LiquidityScore, 0, 100)/100*0.5
	}
	riskPenalty := 1 + routeRiskSortScore(route, false)/100
	if riskPenalty <= 0 {
		riskPenalty = 1
	}
	return base * liquidityMult / riskPenalty
}

func routeRiskSortScore(route RouteResult, unknownIsRisky bool) float64 {
	if !route.HaulingRiskKnown {
		if unknownIsRisky {
			return 75
		}
		return 35
	}
	score := clampRouteFloat(route.HaulingRiskScore, 0, 100)
	switch route.HaulingDanger {
	case "red":
		score += 35
	case "yellow":
		score += 15
	}
	if route.HaulingSafetyMultiplier > 1 {
		score += (route.HaulingSafetyMultiplier - 1) * 20
	}
	return score
}

func normalizedRouteMinutes(route RouteResult) float64 {
	minutes := route.ExecutionMinutes
	if minutes <= 0 || math.IsNaN(minutes) || math.IsInf(minutes, 0) {
		minutes = float64(max(1, route.TotalJumps)) * defaultRouteMinutesPerJump
	}
	return minutes
}

func routeProfitTieBreak(left, right RouteResult) bool {
	if !floatNearlyEqual(left.TotalProfit, right.TotalProfit) {
		return left.TotalProfit > right.TotalProfit
	}
	if !floatNearlyEqual(left.ProfitPerJump, right.ProfitPerJump) {
		return left.ProfitPerJump > right.ProfitPerJump
	}
	return left.TotalJumps < right.TotalJumps
}

func floatNearlyEqual(a, b float64) bool {
	return math.Abs(a-b) < 0.000001
}

func clampRouteFloat(v, lo, hi float64) float64 {
	if math.IsNaN(v) {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
