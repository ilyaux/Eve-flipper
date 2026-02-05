package zkillboard

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"eve-flipper/internal/logger"
)

// DemandAnalyzer analyzes killmail data to predict market demand.
type DemandAnalyzer struct {
	client      *Client
	cache       sync.Map // regionID -> *CachedRegionStats
	cacheTTL    time.Duration
	regionNames map[int32]string
}

// CachedRegionStats holds cached region statistics.
type CachedRegionStats struct {
	Stats     *RegionStats
	HotScore  float64
	UpdatedAt time.Time
}

// RegionHotZone represents a region with elevated kill activity.
type RegionHotZone struct {
	RegionID      int32    `json:"region_id"`
	RegionName    string   `json:"region_name"`
	HotScore      float64  `json:"hot_score"`      // Current vs baseline ratio
	Status        string   `json:"status"`         // "war", "conflict", "elevated", "normal"
	KillsToday    int64    `json:"kills_today"`    // Estimated kills in last 24h
	KillsBaseline int64    `json:"kills_baseline"` // Average daily kills (baseline)
	ISKDestroyed  float64  `json:"isk_destroyed"`  // Total ISK destroyed (all time)
	ActivePlayers int      `json:"active_players"` // Currently active PVP players
	TopShips      []string `json:"top_ships"`      // Most destroyed ship types
}

// DemandItem represents an item with high demand due to kills.
type DemandItem struct {
	TypeID       int32   `json:"type_id"`
	TypeName     string  `json:"type_name"`
	GroupID      int32   `json:"group_id"`
	GroupName    string  `json:"group_name"`
	LossesPerDay int64   `json:"losses_per_day"` // Estimated daily losses
	DemandScore  float64 `json:"demand_score"`   // Higher = more demand
	RegionID     int32   `json:"region_id"`
	RegionName   string  `json:"region_name"`
}

// NewDemandAnalyzer creates a new demand analyzer.
func NewDemandAnalyzer(regionNames map[int32]string) *DemandAnalyzer {
	return &DemandAnalyzer{
		client:      NewClient(),
		cacheTTL:    30 * time.Minute,
		regionNames: regionNames,
	}
}

// SetRegionNames updates the region name mapping.
func (d *DemandAnalyzer) SetRegionNames(names map[int32]string) {
	d.regionNames = names
}

// ClearCache removes all cached region stats, forcing fresh API calls.
func (d *DemandAnalyzer) ClearCache() {
	d.cache.Range(func(key, _ interface{}) bool {
		d.cache.Delete(key)
		return true
	})
}

// KnownSpaceRegions returns IDs of all known-space regions (excluding wormholes).
// Known space regions have IDs from 10000001 to 10000069.
func KnownSpaceRegions() []int32 {
	regions := make([]int32, 0, 70)
	for id := int32(10000001); id <= int32(10000069); id++ {
		// Skip special regions
		if id == 10000004 || id == 10000017 || id == 10000019 { // UUA-F4, J7HZ-F, A821-A
			continue
		}
		regions = append(regions, id)
	}
	return regions
}

// GetHotZones fetches and analyzes all regions to find war zones.
func (d *DemandAnalyzer) GetHotZones(limit int) ([]RegionHotZone, error) {
	regions := KnownSpaceRegions()

	logger.Info("Demand", fmt.Sprintf("Analyzing %d regions for hot zones...", len(regions)))

	// Fetch stats for all regions concurrently (with rate limiting in client)
	type result struct {
		regionID int32
		stats    *RegionStats
		err      error
	}

	results := make(chan result, len(regions))
	var wg sync.WaitGroup

	// Process in batches to avoid overwhelming the API.
	// Batch size is dynamic based on total regions, but capped conservatively
	// to avoid hitting zKillboard rate limits.
	batchSize := 10
	if len(regions) > 40 {
		batchSize = len(regions) / 4 // ~25% of regions in parallel
		if batchSize < 10 {
			batchSize = 10
		}
		if batchSize > 20 {
			batchSize = 20
		}
	}
	for i := 0; i < len(regions); i += batchSize {
		end := i + batchSize
		if end > len(regions) {
			end = len(regions)
		}

		for _, regionID := range regions[i:end] {
			wg.Add(1)
			go func(rid int32) {
				defer wg.Done()

				// Check cache first
				if cached, ok := d.cache.Load(rid); ok {
					c := cached.(*CachedRegionStats)
					if time.Since(c.UpdatedAt) < d.cacheTTL {
						results <- result{regionID: rid, stats: c.Stats}
						return
					}
				}

				stats, err := d.client.GetRegionStats(rid)
				if err != nil {
					results <- result{regionID: rid, err: err}
					return
				}

				// Cache the result
				d.cache.Store(rid, &CachedRegionStats{
					Stats:     stats,
					UpdatedAt: time.Now(),
				})

				results <- result{regionID: rid, stats: stats}
			}(regionID)
		}

		// Small pause between batches to play nice with the API
		if i+batchSize < len(regions) {
			time.Sleep(250 * time.Millisecond)
		}
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect and analyze results
	var hotZones []RegionHotZone
	for r := range results {
		if r.err != nil {
			logger.Warn("Demand", fmt.Sprintf("Failed to get stats for region %d: %v", r.regionID, r.err))
			continue
		}

		zone := d.analyzeRegion(r.regionID, r.stats)
		if zone != nil {
			hotZones = append(hotZones, *zone)
		}
	}

	// Sort by hot score (descending)
	sort.Slice(hotZones, func(i, j int) bool {
		return hotZones[i].HotScore > hotZones[j].HotScore
	})

	// Limit results
	if limit > 0 && len(hotZones) > limit {
		hotZones = hotZones[:limit]
	}

	logger.Success("Demand", fmt.Sprintf("Found %d hot zones", len(hotZones)))
	return hotZones, nil
}

// GetSingleRegionStats fetches stats for a single region.
func (d *DemandAnalyzer) GetSingleRegionStats(regionID int32) (*RegionHotZone, error) {
	// Check cache first
	if cached, ok := d.cache.Load(regionID); ok {
		c := cached.(*CachedRegionStats)
		if time.Since(c.UpdatedAt) < d.cacheTTL {
			return d.analyzeRegion(regionID, c.Stats), nil
		}
	}

	stats, err := d.client.GetRegionStats(regionID)
	if err != nil {
		return nil, err
	}

	// Cache the result
	d.cache.Store(regionID, &CachedRegionStats{
		Stats:     stats,
		UpdatedAt: time.Now(),
	})

	return d.analyzeRegion(regionID, stats), nil
}

// analyzeRegion calculates hot zone metrics for a region.
func (d *DemandAnalyzer) analyzeRegion(regionID int32, stats *RegionStats) *RegionHotZone {
	if stats == nil || stats.Months == nil {
		return nil
	}

	// Get region name
	regionName := d.regionNames[regionID]
	if regionName == "" {
		if stats.Info != nil && stats.Info.Name != "" {
			regionName = stats.Info.Name
		} else {
			regionName = fmt.Sprintf("Region %d", regionID)
		}
	}

	// Calculate baseline (average of last 6 months, excluding current)
	now := time.Now()
	currentKey := fmt.Sprintf("%d%02d", now.Year(), now.Month())

	var baselineTotal int64
	var baselineMonths int

	for key, month := range stats.Months {
		if key == currentKey {
			continue // Skip current month
		}

		// Only use last 6 months
		monthTime := time.Date(month.Year, time.Month(month.Month), 1, 0, 0, 0, 0, time.UTC)
		if now.Sub(monthTime) > 6*30*24*time.Hour {
			continue
		}

		baselineTotal += month.ShipsDestroyed
		baselineMonths++
	}

	if baselineMonths == 0 {
		baselineMonths = 1
	}

	baselineAvg := float64(baselineTotal) / float64(baselineMonths)
	baselineDaily := baselineAvg / 30.0 // Average daily kills

	// Get current month stats
	var currentMonthKills int64
	var daysPassed int
	if currentMonth, ok := stats.Months[currentKey]; ok {
		currentMonthKills = currentMonth.ShipsDestroyed
		daysPassed = now.Day()
	}

	if daysPassed == 0 {
		daysPassed = 1
	}

	// Estimate current daily rate
	currentDaily := float64(currentMonthKills) / float64(daysPassed)

	// Calculate hot score (current vs baseline ratio)
	var hotScore float64
	if baselineDaily > 0 {
		hotScore = currentDaily / baselineDaily
	} else if currentDaily > 0 {
		hotScore = 2.0 // No baseline but has activity
	}

	// Determine status based on activity ratio
	// Thresholds adjusted for EVE Online patterns:
	// - 1.8x+ indicates significant conflict (war)
	// - 1.4x+ indicates ongoing conflict
	// - 1.15x+ indicates elevated activity
	status := "normal"
	if hotScore >= 1.8 {
		status = "war"
	} else if hotScore >= 1.4 {
		status = "conflict"
	} else if hotScore >= 1.15 {
		status = "elevated"
	}

	// Get active players
	activePlayers := 0
	if stats.ActivePVP != nil && stats.ActivePVP.Characters != nil {
		activePlayers = stats.ActivePVP.Characters.Count
	}

	// Get top ships
	var topShips []string
	for _, list := range stats.TopLists {
		if list.Type == "shipType" {
			for i, v := range list.Values {
				if i >= 5 {
					break
				}
				if v.ShipName != "" {
					topShips = append(topShips, v.ShipName)
				}
			}
			break
		}
	}

	return &RegionHotZone{
		RegionID:      regionID,
		RegionName:    regionName,
		HotScore:      hotScore,
		Status:        status,
		KillsToday:    int64(currentDaily),
		KillsBaseline: int64(baselineDaily),
		ISKDestroyed:  stats.ISKDestroyed,
		ActivePlayers: activePlayers,
		TopShips:      topShips,
	}
}

// GetTopDestroyedShipGroups returns the most destroyed ship groups in a region.
func (d *DemandAnalyzer) GetTopDestroyedShipGroups(regionID int32, limit int) ([]DemandItem, error) {
	stats, err := d.client.GetRegionStats(regionID)
	if err != nil {
		return nil, err
	}

	if stats.Groups == nil {
		return nil, nil
	}

	// Convert groups to slice and sort by ships destroyed
	type groupStat struct {
		groupID        int32
		shipsDestroyed int64
		iskDestroyed   float64
	}

	var groups []groupStat
	for _, g := range stats.Groups {
		groups = append(groups, groupStat{
			groupID:        g.GroupID,
			shipsDestroyed: g.ShipsDestroyed,
			iskDestroyed:   g.ISKDestroyed,
		})
	}

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].shipsDestroyed > groups[j].shipsDestroyed
	})

	if limit > 0 && len(groups) > limit {
		groups = groups[:limit]
	}

	// Convert to DemandItem
	regionName := d.regionNames[regionID]
	items := make([]DemandItem, len(groups))
	for i, g := range groups {
		// Estimate daily losses (last month / 30)
		dailyLosses := g.shipsDestroyed / 30 // Rough estimate

		items[i] = DemandItem{
			GroupID:      g.groupID,
			LossesPerDay: dailyLosses,
			DemandScore:  float64(g.shipsDestroyed) / 1000.0, // Normalize
			RegionID:     regionID,
			RegionName:   regionName,
		}
	}

	return items, nil
}
