package api

import (
	"fmt"
	"math"
	"strings"

	"eve-flipper/internal/engine"
	"eve-flipper/internal/gankcheck"
)

const maxRouteHaulingRiskEnrich = 200

func (s *Server) enrichRouteHaulingRisk(
	routes []engine.RouteResult,
	startSystemName string,
	targetSystemName string,
	minSec float64,
	progress func(string),
) []engine.RouteResult {
	if len(routes) == 0 || s.ganker == nil {
		return routes
	}

	startSystemID := s.systemIDByName(startSystemName)
	if startSystemID == 0 {
		return routes
	}
	targetSystemID := s.systemIDByName(targetSystemName)

	limit := len(routes)
	if limit > maxRouteHaulingRiskEnrich {
		limit = maxRouteHaulingRiskEnrich
	}
	if progress != nil {
		msg := "Scoring hauling gank risk..."
		if limit < len(routes) {
			msg = fmt.Sprintf("Scoring hauling gank risk for top %d routes...", limit)
		}
		progress(msg)
	}

	for i := 0; i < limit; i++ {
		summary := routeHaulingRiskSummary{}
		prevSystemID := startSystemID
		for _, hop := range routes[i].Hops {
			if hop.SystemID > 0 && prevSystemID > 0 && hop.SystemID != prevSystemID {
				summary.add(s.routeDangerSystems(prevSystemID, hop.SystemID, minSec))
			}
			if hop.SystemID > 0 && hop.DestSystemID > 0 && hop.SystemID != hop.DestSystemID {
				summary.add(s.routeDangerSystems(hop.SystemID, hop.DestSystemID, minSec))
			}
			if hop.DestSystemID > 0 {
				prevSystemID = hop.DestSystemID
			}
		}
		if targetSystemID > 0 && prevSystemID > 0 && targetSystemID != prevSystemID {
			summary.add(s.routeDangerSystems(prevSystemID, targetSystemID, minSec))
		}
		summary.applyTo(&routes[i])
	}
	return routes
}

func (s *Server) systemIDByName(name string) int32 {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.sdeData == nil {
		return 0
	}
	return s.sdeData.SystemByName[name]
}

func (s *Server) routeDangerSystems(from, to int32, minSec float64) []gankcheck.SystemDanger {
	systems, err := s.ganker.CheckRoute(from, to, minSec)
	if err != nil {
		return nil
	}
	return systems
}

type routeHaulingRiskSummary struct {
	seen      map[int32]bool
	kills     int
	totalISK  float64
	score     float64
	danger    string
	haveRoute bool
}

func (s *routeHaulingRiskSummary) add(systems []gankcheck.SystemDanger) {
	if len(systems) == 0 {
		return
	}
	if s.seen == nil {
		s.seen = make(map[int32]bool, len(systems))
	}
	s.haveRoute = true
	for _, sys := range systems {
		if sys.SystemID != 0 {
			if s.seen[sys.SystemID] {
				continue
			}
			s.seen[sys.SystemID] = true
		}
		s.kills += sys.KillsTotal
		s.totalISK += sys.TotalISK
		switch sys.DangerLevel {
		case "red":
			s.danger = "red"
		case "yellow":
			if s.danger == "" || s.danger == "green" {
				s.danger = "yellow"
			}
		default:
			if s.danger == "" {
				s.danger = "green"
			}
		}
		s.score += float64(sys.KillsTotal) * 12
		s.score += math.Min(sys.TotalISK/1_000_000_000*4, 25)
		if sys.IsSmartbomb {
			s.score += 12
		}
		if sys.IsInterdictor {
			s.score += 18
		}
		if sys.Security > 0 && sys.Security < 0.45 {
			s.score += 8
		}
		if sys.Security <= 0 {
			s.score += 15
		}
	}
}

func (s routeHaulingRiskSummary) applyTo(route *engine.RouteResult) {
	if route == nil || !s.haveRoute {
		return
	}
	route.HaulingRiskKnown = true
	if s.danger == "" {
		s.danger = "green"
	}
	route.HaulingDanger = s.danger
	route.HaulingKills = s.kills
	route.HaulingISK = s.totalISK
	if s.score > 100 {
		s.score = 100
	}
	if s.score < 0 {
		s.score = 0
	}
	route.HaulingRiskScore = s.score
	route.HaulingSafetyMultiplier = routeSafetyMultiplierFromSummary(s)
}

func routeSafetyMultiplierFromSummary(s routeHaulingRiskSummary) float64 {
	if !s.haveRoute {
		return 1
	}
	score := s.score
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	mult := 1 + score/100
	switch s.danger {
	case "red":
		if mult < 1.75 {
			mult = 1.75
		}
	case "yellow":
		if mult < 1.25 {
			mult = 1.25
		}
	}
	if mult > 3 {
		mult = 3
	}
	return math.Round(mult*100) / 100
}
