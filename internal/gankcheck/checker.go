package gankcheck

import (
	"fmt"
	"sort"
	"sync"

	"eve-flipper/internal/esi"
	"eve-flipper/internal/graph"
	"eve-flipper/internal/sde"
	"eve-flipper/internal/zkillboard"
)

// EVE ship group IDs for threat detection
const (
	groupInterdictor      = int32(541) // T2 destroyer — bubble launcher
	groupHeavyInterdictor = int32(894) // T2 cruiser — HIC
	groupSmartbomb        = int32(135) // Energy Smartbomb modules
)

type KillSummary struct {
	KillmailID    int64    `json:"KillmailID"`
	VictimShip    string   `json:"VictimShip"`
	AttackerShips []string `json:"AttackerShips"`
	Corporations  []string `json:"Corporations"`
	AttackerCount int      `json:"AttackerCount"`
	ISKValue      float64  `json:"ISKValue"`
	KillTime      string   `json:"KillTime"`
	ZKBLink       string   `json:"ZKBLink"`
	IsSmartbomb   bool     `json:"IsSmartbomb"`
	IsInterdictor bool     `json:"IsInterdictor"`
}

type SystemDanger struct {
	SystemID      int32   `json:"SystemID"`
	SystemName    string  `json:"SystemName"`
	Security      float64 `json:"Security"`
	KillsTotal    int     `json:"KillsTotal"`
	DangerLevel   string  `json:"DangerLevel"` // "green" | "yellow" | "red"
	IsSmartbomb   bool    `json:"IsSmartbomb"`
	IsInterdictor bool    `json:"IsInterdictor"`
	TotalISK      float64 `json:"TotalISK"`
}

// CamperProfile tracks suspected player killers in a system.
type CamperProfile struct {
	SystemID    int32    `json:"SystemID"`
	SystemName  string   `json:"SystemName"`
	CamperCorps []string `json:"CamperCorps"`
	KillCount   int      `json:"KillCount"`
}

type Checker struct {
	zkb        *zkillboard.Client
	esiCl      *esi.Client
	sde        *sde.Data
	universe   *graph.Universe
	cache      *systemDangerCache
	typeNameMu sync.RWMutex
	typeNames  map[int32]string // fallback cache for non-market types
}

func NewChecker(zkb *zkillboard.Client, esiCl *esi.Client, sdeData *sde.Data, universe *graph.Universe) *Checker {
	return &Checker{
		zkb:       zkb,
		esiCl:     esiCl,
		sde:       sdeData,
		universe:  universe,
		cache:     newSystemDangerCache(),
		typeNames: make(map[int32]string),
	}
}

// typeName resolves a type ID to its name, falling back to ESI if not in SDE.
func (c *Checker) typeName(typeID int32) string {
	if typeID <= 0 {
		return ""
	}
	if t, ok := c.sde.Types[typeID]; ok {
		return t.Name
	}
	// Check fallback cache
	c.typeNameMu.RLock()
	if name, ok := c.typeNames[typeID]; ok {
		c.typeNameMu.RUnlock()
		return name
	}
	c.typeNameMu.RUnlock()

	// Fetch from ESI
	var resp struct {
		Name string `json:"name"`
	}
	url := fmt.Sprintf("https://esi.evetech.net/latest/universe/types/%d/?datasource=tranquility", typeID)
	name := ""
	if err := c.esiCl.GetJSON(url, &resp); err == nil && resp.Name != "" {
		name = resp.Name
	}

	c.typeNameMu.Lock()
	c.typeNames[typeID] = name
	c.typeNameMu.Unlock()
	return name
}

func dangerLevel(kills int) string {
	switch {
	case kills == 0:
		return "green"
	case kills <= 2:
		return "yellow"
	default:
		return "red"
	}
}

// CheckRoute returns danger info for every system along the route from->to.
func (c *Checker) CheckRoute(from, to int32, minSec float64) ([]SystemDanger, error) {
	path := c.universe.GetPath(from, to, minSec)
	if path == nil {
		path = c.universe.GetPath(from, to, 0)
	}
	if path == nil {
		return nil, fmt.Errorf("no path from %d to %d", from, to)
	}

	result := make([]SystemDanger, len(path))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i, sysID := range path {
		wg.Add(1)
		go func(idx int, systemID int32) {
			defer wg.Done()
			sd, err := c.checkSystem(systemID)
			mu.Lock()
			defer mu.Unlock()
			if err != nil || sd == nil {
				name := ""
				security := 0.0
				if s, ok := c.sde.Systems[systemID]; ok {
					name = s.Name
					security = s.Security
				}
				result[idx] = SystemDanger{
					SystemID:    systemID,
					SystemName:  name,
					Security:    security,
					DangerLevel: "green",
				}
				return
			}
			result[idx] = *sd
		}(i, sysID)
	}
	wg.Wait()
	return result, nil
}

// checkSystem fetches danger info for a single system, using cache.
func (c *Checker) checkSystem(systemID int32) (*SystemDanger, error) {
	if cached := c.cache.get(systemID); cached != nil {
		return cached, nil
	}

	name := ""
	security := 0.0
	if s, ok := c.sde.Systems[systemID]; ok {
		name = s.Name
		security = s.Security
	}

	kills, err := c.zkb.GetSystemKills(systemID, 3600)
	if err != nil {
		return &SystemDanger{
			SystemID:    systemID,
			SystemName:  name,
			Security:    security,
			KillsTotal:  0,
			DangerLevel: "green",
		}, nil
	}

	// Extract basic stats from zkb response (ISK, ship threat flags)
	totalISK := 0.0
	isSmartbomb := false
	isInterdictor := false

	for _, km := range kills {
		zkbRaw, ok := km["zkb"].(map[string]interface{})
		if !ok {
			continue
		}
		if v, ok := zkbRaw["totalValue"].(float64); ok {
			totalISK += v
		}
		// Check attacker ship types from zkb data if present
		if attackers, ok := km["attackers"].([]interface{}); ok {
			for _, a := range attackers {
				atk, ok := a.(map[string]interface{})
				if !ok {
					continue
				}
				if shipTypeID, ok := atk["ship_type_id"].(float64); ok {
					gid := c.shipGroupID(int32(shipTypeID))
					if gid == groupInterdictor || gid == groupHeavyInterdictor {
						isInterdictor = true
					}
				}
				if weaponTypeID, ok := atk["weapon_type_id"].(float64); ok {
					gid := c.shipGroupID(int32(weaponTypeID))
					if gid == groupSmartbomb {
						isSmartbomb = true
					}
				}
			}
		}
	}

	sd := &SystemDanger{
		SystemID:      systemID,
		SystemName:    name,
		Security:      security,
		KillsTotal:    len(kills),
		DangerLevel:   dangerLevel(len(kills)),
		IsSmartbomb:   isSmartbomb,
		IsInterdictor: isInterdictor,
		TotalISK:      totalISK,
	}

	c.cache.set(systemID, sd)
	return sd, nil
}

// shipGroupID returns the SDE group ID for a given type ID.
func (c *Checker) shipGroupID(typeID int32) int32 {
	if t, ok := c.sde.Types[typeID]; ok {
		return t.GroupID
	}
	return 0
}

// CheckSystemDetail fetches full killmail details for a single system (lazy, for modal).
func (c *Checker) CheckSystemDetail(systemID int32) ([]KillSummary, error) {
	kills, err := c.zkb.GetSystemKills(systemID, 3600)
	if err != nil {
		return nil, err
	}

	type killRef struct {
		id       int64
		hash     string
		iskValue float64
	}
	var refs []killRef
	for _, km := range kills {
		idFloat, ok := km["killmail_id"].(float64)
		if !ok {
			continue
		}
		zkbRaw, ok := km["zkb"].(map[string]interface{})
		if !ok {
			continue
		}
		hash, ok := zkbRaw["hash"].(string)
		if !ok {
			continue
		}
		iskValue := 0.0
		if v, ok := zkbRaw["totalValue"].(float64); ok {
			iskValue = v
		}
		refs = append(refs, killRef{id: int64(idFloat), hash: hash, iskValue: iskValue})
	}

	type esiKM struct {
		KillmailID   int64  `json:"killmail_id"`
		KillmailTime string `json:"killmail_time"`
		Victim       struct {
			ShipTypeID    int32 `json:"ship_type_id"`
			CharacterID   int32 `json:"character_id"`
			CorporationID int32 `json:"corporation_id"`
		} `json:"victim"`
		Attackers []struct {
			ShipTypeID    int32   `json:"ship_type_id"`
			WeaponTypeID  int32   `json:"weapon_type_id"`
			CorporationID int32   `json:"corporation_id"`
			CharacterID   int32   `json:"character_id"`
			FinalBlow     bool    `json:"final_blow"`
			DamageDone    float64 `json:"damage_done"`
		} `json:"attackers"`
	}

	type kmResult struct {
		km       *esiKM
		iskValue float64
		err      error
	}

	results := make(chan kmResult, len(refs))
	sem := make(chan struct{}, 10)

	var wg sync.WaitGroup
	for _, ref := range refs {
		wg.Add(1)
		go func(r killRef) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			var km esiKM
			url := fmt.Sprintf("https://esi.evetech.net/latest/killmails/%d/%s/?datasource=tranquility", r.id, r.hash)
			if err := c.esiCl.GetJSON(url, &km); err != nil {
				results <- kmResult{err: err}
				return
			}
			results <- kmResult{km: &km, iskValue: r.iskValue}
		}(ref)
	}
	wg.Wait()
	close(results)

	var summaries []KillSummary
	for r := range results {
		if r.err != nil || r.km == nil {
			continue
		}
		km := r.km

		victimShip := c.typeName(km.Victim.ShipTypeID)

		attackerShips := make([]string, 0)
		corps := make([]string, 0)
		seenShip := make(map[int32]bool)
		corpSeen := make(map[int32]bool)
		isSmartbomb := false
		isInterdictor := false

		for _, a := range km.Attackers {
			if a.ShipTypeID > 0 && !seenShip[a.ShipTypeID] {
				seenShip[a.ShipTypeID] = true
				gid := c.shipGroupID(a.ShipTypeID)
				if gid == groupInterdictor || gid == groupHeavyInterdictor {
					isInterdictor = true
				}
				if name := c.typeName(a.ShipTypeID); name != "" {
					attackerShips = append(attackerShips, name)
				}
			}
			if a.WeaponTypeID > 0 {
				gid := c.shipGroupID(a.WeaponTypeID)
				if gid == groupSmartbomb {
					isSmartbomb = true
				}
			}
			if a.CorporationID > 0 && !corpSeen[a.CorporationID] {
				corpSeen[a.CorporationID] = true
				corps = append(corps, fmt.Sprintf("%d", a.CorporationID))
			}
		}

		summaries = append(summaries, KillSummary{
			KillmailID:    km.KillmailID,
			VictimShip:    victimShip,
			AttackerShips: attackerShips,
			Corporations:  corps,
			AttackerCount: len(km.Attackers),
			ISKValue:      r.iskValue,
			KillTime:      km.KillmailTime,
			ZKBLink:       fmt.Sprintf("https://zkillboard.com/kill/%d/", km.KillmailID),
			IsSmartbomb:   isSmartbomb,
			IsInterdictor: isInterdictor,
		})
	}

	// Sort by ISK value descending (most valuable kills first)
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].ISKValue > summaries[j].ISKValue
	})

	return summaries, nil
}

// GetCamperProfile analyzes recent kills to identify suspected camping corporations.
func (c *Checker) GetCamperProfile(systemID int32) CamperProfile {
	// Get kills from the last hour
	kills, err := c.zkb.GetSystemKills(systemID, 1)
	if err != nil || len(kills) == 0 {
		return CamperProfile{
			SystemID:  systemID,
			KillCount: 0,
		}
	}

	// Count corporations appearing in recent kills
	corpCounts := make(map[string]int)
	for _, km := range kills {
		attackers, ok := km["attackers"].([]interface{})
		if !ok {
			continue
		}
		corpSeen := make(map[int32]bool)
		for _, a := range attackers {
			atk, ok := a.(map[string]interface{})
			if !ok {
				continue
			}
			if corpID, ok := atk["corporation_id"].(float64); ok {
				corpIDInt := int32(corpID)
				if !corpSeen[corpIDInt] {
					corpSeen[corpIDInt] = true
					corpCounts[fmt.Sprintf("%d", corpIDInt)]++
				}
			}
		}
	}

	// Identify corps appearing in 3+ recent kills as suspected campers
	var campers []string
	for corp, count := range corpCounts {
		if count >= 3 {
			campers = append(campers, corp)
		}
	}

	return CamperProfile{
		SystemID:    systemID,
		CamperCorps: campers,
		KillCount:   len(kills),
	}
}
