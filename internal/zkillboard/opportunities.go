package zkillboard

import (
	"sort"
	"sync"

	"eve-flipper/internal/esi"
)

// TradeOpportunity represents a potential trade based on war demand.
type TradeOpportunity struct {
	TypeID        int32   `json:"type_id"`
	TypeName      string  `json:"type_name"`
	Category      string  `json:"category"` // "ship", "module", "ammo", "drone"
	KillsPerDay   int     `json:"kills_per_day"`
	JitaPrice     float64 `json:"jita_price"`      // Best sell price in Jita
	RegionPrice   float64 `json:"region_price"`    // Best sell price in target region
	ProfitPerUnit float64 `json:"profit_per_unit"` // RegionPrice - JitaPrice
	ProfitPercent float64 `json:"profit_percent"`  // Profit margin %
	DailyVolume   int     `json:"daily_volume"`    // Estimated daily demand
	DailyProfit   float64 `json:"daily_profit"`    // Potential daily profit
	JitaVolume    int32   `json:"jita_volume"`     // Available volume in Jita
	RegionVolume  int32   `json:"region_volume"`   // Available volume in region
}

// RegionOpportunities contains all trade opportunities for a region.
type RegionOpportunities struct {
	RegionID       int32              `json:"region_id"`
	RegionName     string             `json:"region_name"`
	Status         string             `json:"status"`
	HotScore       float64            `json:"hot_score"`
	SecurityClass  string             `json:"security_class"`  // "highsec", "lowsec", "nullsec"
	SecurityBlocks []string           `json:"security_blocks"` // ["high", "low", "null"] for display
	JumpsFromJita  int                `json:"jumps_from_jita"` // Distance from Jita
	MainSystem     string             `json:"main_system"`     // Main hub/system name
	Ships          []TradeOpportunity `json:"ships"`
	Modules        []TradeOpportunity `json:"modules"`
	Ammo           []TradeOpportunity `json:"ammo"`
	TotalPotential float64            `json:"total_potential"` // Sum of daily profits
}

// Common PvP modules that are always in demand during conflicts
var commonPvPModules = []struct {
	TypeID   int32
	Name     string
	Category string
}{
	// Shield modules
	{3841, "Large Shield Extender II", "module"},
	{3831, "Medium Shield Extender II", "module"},
	{2281, "Damage Control II", "module"},
	{2048, "Adaptive Invulnerability Field II", "module"},

	// Armor modules
	{11269, "1600mm Steel Plates II", "module"},
	{11295, "Energized Adaptive Nano Membrane II", "module"},
	{20353, "Damage Control II", "module"},

	// Tackle
	{5443, "Warp Disruptor II", "module"},
	{5439, "Warp Scrambler II", "module"},
	{4405, "Stasis Webifier II", "module"},

	// Propulsion
	{12076, "50MN Microwarpdrive II", "module"},
	{12084, "500MN Microwarpdrive II", "module"},
	{12068, "10MN Afterburner II", "module"},

	// Weapon upgrades
	{22291, "Gyrostabilizer II", "module"},
	{10190, "Ballistic Control System II", "module"},
	{22919, "Heat Sink II", "module"},
	{4405, "Magnetic Field Stabilizer II", "module"},
}

// Common ammo types in demand
var commonAmmo = []struct {
	TypeID   int32
	Name     string
	Category string
}{
	// Hybrid charges
	{233, "Antimatter Charge L", "ammo"},
	{237, "Antimatter Charge M", "ammo"},
	{229, "Antimatter Charge S", "ammo"},
	{244, "Void L", "ammo"},
	{248, "Void M", "ammo"},
	{240, "Void S", "ammo"},
	{243, "Null L", "ammo"},
	{247, "Null M", "ammo"},
	{239, "Null S", "ammo"},

	// Projectile ammo
	{2203, "EMP L", "ammo"},
	{2205, "EMP M", "ammo"},
	{2201, "EMP S", "ammo"},
	{12761, "Hail L", "ammo"},
	{12763, "Hail M", "ammo"},
	{12759, "Hail S", "ammo"},
	{12774, "Barrage L", "ammo"},
	{12776, "Barrage M", "ammo"},
	{12772, "Barrage S", "ammo"},

	// Laser crystals
	{12820, "Scorch L", "ammo"},
	{12822, "Scorch M", "ammo"},
	{12818, "Scorch S", "ammo"},
	{12826, "Conflagration L", "ammo"},
	{12828, "Conflagration M", "ammo"},
	{12824, "Conflagration S", "ammo"},

	// Missiles
	{24513, "Caldari Navy Scourge Heavy Missile", "ammo"},
	{24519, "Caldari Navy Mjolnir Heavy Missile", "ammo"},
	{27361, "Fury Heavy Missile", "ammo"},
	{2629, "Nova Rage Torpedo", "ammo"},

	// Drones
	{2456, "Hobgoblin II", "ammo"},
	{2454, "Hammerhead II", "ammo"},
	{2446, "Ogre II", "ammo"},
	{28209, "Warrior II", "ammo"},
	{28211, "Valkyrie II", "ammo"},
	{28213, "Berserker II", "ammo"},

	// Nanite paste
	{28668, "Nanite Repair Paste", "ammo"},

	// Cap boosters
	{11283, "Cap Booster 800", "ammo"},
	{263, "Cap Booster 400", "ammo"},
}

const jitaRegionID = int32(10000002) // The Forge

// GetRegionOpportunities analyzes trade opportunities for a war region.
func (d *DemandAnalyzer) GetRegionOpportunities(regionID int32, esiClient *esi.Client) (*RegionOpportunities, error) {
	// Get region stats from zkillboard
	stats, err := d.client.GetRegionStats(regionID)
	if err != nil {
		return nil, err
	}

	zone := d.analyzeRegion(regionID, stats)
	if zone == nil {
		return nil, nil
	}

	result := &RegionOpportunities{
		RegionID:   regionID,
		RegionName: zone.RegionName,
		Status:     zone.Status,
		HotScore:   zone.HotScore,
	}

	// Collect all type IDs we need prices for
	typeIDs := make(map[int32]struct{})
	shipKills := make(map[int32]int) // typeID -> kills

	// Get top ships from zkillboard stats
	for _, list := range stats.TopLists {
		if list.Type == "shipType" {
			for _, v := range list.Values {
				if v.ShipTypeID > 0 {
					typeIDs[v.ShipTypeID] = struct{}{}
					shipKills[v.ShipTypeID] = v.Kills
				}
			}
		}
	}

	// Add common modules and ammo
	for _, m := range commonPvPModules {
		typeIDs[m.TypeID] = struct{}{}
	}
	for _, a := range commonAmmo {
		typeIDs[a.TypeID] = struct{}{}
	}

	// Convert to slice
	typeIDSlice := make([]int32, 0, len(typeIDs))
	for id := range typeIDs {
		typeIDSlice = append(typeIDSlice, id)
	}

	// Fetch prices in parallel
	jitaPrices := make(map[int32]priceInfo)
	regionPrices := make(map[int32]priceInfo)
	var wg sync.WaitGroup
	var mu sync.Mutex

	// Fetch Jita prices
	wg.Add(1)
	go func() {
		defer wg.Done()
		prices := fetchBestPrices(esiClient, jitaRegionID, typeIDSlice)
		mu.Lock()
		jitaPrices = prices
		mu.Unlock()
	}()

	// Fetch target region prices (only if different from Jita)
	if regionID != jitaRegionID {
		wg.Add(1)
		go func() {
			defer wg.Done()
			prices := fetchBestPrices(esiClient, regionID, typeIDSlice)
			mu.Lock()
			regionPrices = prices
			mu.Unlock()
		}()
	}

	wg.Wait()

	// Build opportunities
	// Ships
	for typeID, kills := range shipKills {
		jita := jitaPrices[typeID]
		region := regionPrices[typeID]

		if jita.sellPrice > 0 {
			opp := buildOpportunity(typeID, "", "ship", kills, jita, region)
			if opp != nil && opp.ProfitPercent > 5 { // Only show if >5% margin
				result.Ships = append(result.Ships, *opp)
			}
		}
	}

	// Modules
	for _, m := range commonPvPModules {
		jita := jitaPrices[m.TypeID]
		region := regionPrices[m.TypeID]

		if jita.sellPrice > 0 {
			// Estimate demand based on region activity
			estimatedKills := int(float64(zone.KillsToday) * 0.5) // ~50% of kills need these
			opp := buildOpportunity(m.TypeID, m.Name, m.Category, estimatedKills, jita, region)
			if opp != nil && opp.ProfitPercent > 10 { // Higher threshold for modules
				result.Modules = append(result.Modules, *opp)
			}
		}
	}

	// Ammo
	for _, a := range commonAmmo {
		jita := jitaPrices[a.TypeID]
		region := regionPrices[a.TypeID]

		if jita.sellPrice > 0 {
			// Ammo is consumed heavily
			estimatedKills := int(float64(zone.KillsToday) * 100) // ~100 units per kill
			opp := buildOpportunity(a.TypeID, a.Name, a.Category, estimatedKills, jita, region)
			if opp != nil && opp.ProfitPercent > 15 { // Even higher for ammo due to low margins
				result.Ammo = append(result.Ammo, *opp)
			}
		}
	}

	// Sort by daily profit
	sort.Slice(result.Ships, func(i, j int) bool {
		return result.Ships[i].DailyProfit > result.Ships[j].DailyProfit
	})
	sort.Slice(result.Modules, func(i, j int) bool {
		return result.Modules[i].DailyProfit > result.Modules[j].DailyProfit
	})
	sort.Slice(result.Ammo, func(i, j int) bool {
		return result.Ammo[i].DailyProfit > result.Ammo[j].DailyProfit
	})

	// Limit results
	if len(result.Ships) > 10 {
		result.Ships = result.Ships[:10]
	}
	if len(result.Modules) > 10 {
		result.Modules = result.Modules[:10]
	}
	if len(result.Ammo) > 10 {
		result.Ammo = result.Ammo[:10]
	}

	// Calculate total potential
	for _, s := range result.Ships {
		result.TotalPotential += s.DailyProfit
	}
	for _, m := range result.Modules {
		result.TotalPotential += m.DailyProfit
	}
	for _, a := range result.Ammo {
		result.TotalPotential += a.DailyProfit
	}

	return result, nil
}

type priceInfo struct {
	sellPrice  float64
	sellVolume int32
}

func fetchBestPrices(esiClient *esi.Client, regionID int32, typeIDs []int32) map[int32]priceInfo {
	prices := make(map[int32]priceInfo)

	orders, err := esiClient.FetchRegionOrders(regionID, "sell")
	if err != nil {
		return prices
	}

	// Find best (lowest) sell price for each type
	for _, order := range orders {
		existing, ok := prices[order.TypeID]
		if !ok || order.Price < existing.sellPrice {
			prices[order.TypeID] = priceInfo{
				sellPrice:  order.Price,
				sellVolume: order.VolumeRemain,
			}
		} else if order.Price == existing.sellPrice {
			// Same price, add volume
			existing.sellVolume += order.VolumeRemain
			prices[order.TypeID] = existing
		}
	}

	return prices
}

func buildOpportunity(typeID int32, name string, category string, kills int, jita, region priceInfo) *TradeOpportunity {
	if jita.sellPrice <= 0 {
		return nil
	}

	profitPerUnit := region.sellPrice - jita.sellPrice
	if profitPerUnit <= 0 {
		// No profit if region price is lower or equal
		// But might be opportunity if no supply in region
		if region.sellVolume == 0 && jita.sellPrice > 0 {
			// No supply in region - estimate 20% markup
			profitPerUnit = jita.sellPrice * 0.2
		} else {
			return nil
		}
	}

	profitPercent := (profitPerUnit / jita.sellPrice) * 100
	dailyVolume := kills
	if dailyVolume < 1 {
		dailyVolume = 1
	}

	return &TradeOpportunity{
		TypeID:        typeID,
		TypeName:      name,
		Category:      category,
		KillsPerDay:   kills,
		JitaPrice:     jita.sellPrice,
		RegionPrice:   region.sellPrice,
		ProfitPerUnit: profitPerUnit,
		ProfitPercent: profitPercent,
		DailyVolume:   dailyVolume,
		DailyProfit:   profitPerUnit * float64(dailyVolume),
		JitaVolume:    jita.sellVolume,
		RegionVolume:  region.sellVolume,
	}
}
