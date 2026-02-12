package esi

import (
	"fmt"
	"sync"
	"time"
)

// IndustryCostIndex represents the cost index for a solar system.
type IndustryCostIndex struct {
	SolarSystemID int32 `json:"solar_system_id"`
	CostIndices   []struct {
		Activity  string  `json:"activity"`
		CostIndex float64 `json:"cost_index"`
	} `json:"cost_indices"`
}

// SystemCostIndices holds cost indices by activity type for a system.
type SystemCostIndices struct {
	Manufacturing float64
	Copying       float64
	Invention     float64
	Reaction      float64
	MEResearch    float64
	TEResearch    float64
}

// IndustryPrices holds adjusted_price and average_price for all types.
type IndustryPrice struct {
	TypeID        int32   `json:"type_id"`
	AdjustedPrice float64 `json:"adjusted_price"`
	AveragePrice  float64 `json:"average_price"`
}

// IndustryCache caches industry-related ESI data.
type IndustryCache struct {
	mu              sync.RWMutex
	costIndices     map[int32]*SystemCostIndices // systemID -> costs
	costIndicesTime time.Time
	prices          map[int32]*IndustryPrice // typeID -> prices
	pricesTime      time.Time

	// Market prices cache (sell order minimums), keyed by region
	marketPricesMu       sync.RWMutex
	marketPrices         map[int32]float64 // typeID -> min sell price
	marketPricesRegionID int32             // which region these prices belong to
	marketPricesTime     time.Time

	// Station/structure specific market prices cache (sell order minimums),
	// keyed by "regionID:locationID".
	locationMarketPrices map[string]locationMarketPricesEntry
}

type locationMarketPricesEntry struct {
	prices    map[int32]float64
	fetchedAt time.Time
}

// NewIndustryCache creates a new industry cache.
func NewIndustryCache() *IndustryCache {
	return &IndustryCache{
		costIndices:          make(map[int32]*SystemCostIndices),
		prices:               make(map[int32]*IndustryPrice),
		marketPrices:         make(map[int32]float64),
		locationMarketPrices: make(map[string]locationMarketPricesEntry),
	}
}

// MarketPricesCacheTTL is how long market prices are cached.
const MarketPricesCacheTTL = 10 * time.Minute

// FetchIndustrySystems fetches cost indices for all systems.
func (c *Client) FetchIndustrySystems() ([]IndustryCostIndex, error) {
	url := fmt.Sprintf("%s/industry/systems/?datasource=tranquility", baseURL)
	var result []IndustryCostIndex
	if err := c.GetJSON(url, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// FetchMarketPrices fetches adjusted and average prices for all types.
func (c *Client) FetchMarketPrices() ([]IndustryPrice, error) {
	url := fmt.Sprintf("%s/markets/prices/?datasource=tranquility", baseURL)
	var result []IndustryPrice
	if err := c.GetJSON(url, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetSystemCostIndex returns cached cost index for a system, fetching if needed.
func (c *Client) GetSystemCostIndex(cache *IndustryCache, systemID int32) (*SystemCostIndices, error) {
	cache.mu.RLock()
	if time.Since(cache.costIndicesTime) < time.Hour {
		if idx, ok := cache.costIndices[systemID]; ok {
			cache.mu.RUnlock()
			return idx, nil
		}
	}
	cache.mu.RUnlock()

	// Fetch fresh data
	cache.mu.Lock()
	defer cache.mu.Unlock()

	// Double-check after acquiring write lock
	if time.Since(cache.costIndicesTime) < time.Hour {
		if idx, ok := cache.costIndices[systemID]; ok {
			return idx, nil
		}
	}

	// Fetch all systems
	systems, err := c.FetchIndustrySystems()
	if err != nil {
		return nil, err
	}

	// Update cache
	cache.costIndices = make(map[int32]*SystemCostIndices)
	for _, sys := range systems {
		idx := &SystemCostIndices{}
		for _, ci := range sys.CostIndices {
			switch ci.Activity {
			case "manufacturing":
				idx.Manufacturing = ci.CostIndex
			case "copying":
				idx.Copying = ci.CostIndex
			case "invention":
				idx.Invention = ci.CostIndex
			case "reaction":
				idx.Reaction = ci.CostIndex
			case "researching_material_efficiency":
				idx.MEResearch = ci.CostIndex
			case "researching_time_efficiency":
				idx.TEResearch = ci.CostIndex
			}
		}
		cache.costIndices[sys.SolarSystemID] = idx
	}
	cache.costIndicesTime = time.Now()

	if idx, ok := cache.costIndices[systemID]; ok {
		return idx, nil
	}
	// System not found in industry data, return zeros
	return &SystemCostIndices{}, nil
}

// GetAdjustedPrice returns the adjusted price for a type, fetching if needed.
func (c *Client) GetAdjustedPrice(cache *IndustryCache, typeID int32) (float64, error) {
	cache.mu.RLock()
	if time.Since(cache.pricesTime) < 30*time.Minute {
		if p, ok := cache.prices[typeID]; ok {
			cache.mu.RUnlock()
			return p.AdjustedPrice, nil
		}
	}
	cache.mu.RUnlock()

	// Fetch fresh data
	cache.mu.Lock()
	defer cache.mu.Unlock()

	// Double-check
	if time.Since(cache.pricesTime) < 30*time.Minute {
		if p, ok := cache.prices[typeID]; ok {
			return p.AdjustedPrice, nil
		}
	}

	// Fetch all prices
	prices, err := c.FetchMarketPrices()
	if err != nil {
		return 0, err
	}

	// Update cache
	cache.prices = make(map[int32]*IndustryPrice)
	for i := range prices {
		cache.prices[prices[i].TypeID] = &prices[i]
	}
	cache.pricesTime = time.Now()

	if p, ok := cache.prices[typeID]; ok {
		return p.AdjustedPrice, nil
	}
	return 0, nil
}

// GetAllAdjustedPrices returns all cached adjusted prices after ensuring cache is fresh.
func (c *Client) GetAllAdjustedPrices(cache *IndustryCache) (map[int32]float64, error) {
	cache.mu.RLock()
	if time.Since(cache.pricesTime) < 30*time.Minute && len(cache.prices) > 0 {
		result := make(map[int32]float64, len(cache.prices))
		for id, p := range cache.prices {
			result[id] = p.AdjustedPrice
		}
		cache.mu.RUnlock()
		return result, nil
	}
	cache.mu.RUnlock()

	// Fetch fresh data
	cache.mu.Lock()
	defer cache.mu.Unlock()

	prices, err := c.FetchMarketPrices()
	if err != nil {
		return nil, err
	}

	cache.prices = make(map[int32]*IndustryPrice)
	result := make(map[int32]float64, len(prices))
	for i := range prices {
		cache.prices[prices[i].TypeID] = &prices[i]
		result[prices[i].TypeID] = prices[i].AdjustedPrice
	}
	cache.pricesTime = time.Now()

	return result, nil
}

// GetCachedMarketPrices returns cached market prices or fetches fresh ones.
// Uses 10-minute cache for sell order minimums, keyed by region.
func (c *Client) GetCachedMarketPrices(cache *IndustryCache, regionID int32) (map[int32]float64, error) {
	cache.marketPricesMu.RLock()
	if cache.marketPricesRegionID == regionID &&
		time.Since(cache.marketPricesTime) < MarketPricesCacheTTL &&
		len(cache.marketPrices) > 0 {
		// Return cached prices for the same region
		result := make(map[int32]float64, len(cache.marketPrices))
		for k, v := range cache.marketPrices {
			result[k] = v
		}
		cache.marketPricesMu.RUnlock()
		return result, nil
	}
	cache.marketPricesMu.RUnlock()

	// Fetch fresh data
	cache.marketPricesMu.Lock()
	defer cache.marketPricesMu.Unlock()

	// Double-check after acquiring write lock
	if cache.marketPricesRegionID == regionID &&
		time.Since(cache.marketPricesTime) < MarketPricesCacheTTL &&
		len(cache.marketPrices) > 0 {
		result := make(map[int32]float64, len(cache.marketPrices))
		for k, v := range cache.marketPrices {
			result[k] = v
		}
		return result, nil
	}

	// Fetch all sell orders for region
	orders, err := c.FetchRegionOrders(regionID, "sell")
	if err != nil {
		return nil, err
	}

	// Build min prices map
	prices := make(map[int32]float64)
	for _, o := range orders {
		if existing, ok := prices[o.TypeID]; !ok || o.Price < existing {
			prices[o.TypeID] = o.Price
		}
	}

	// Update cache with region tag
	cache.marketPrices = prices
	cache.marketPricesRegionID = regionID
	cache.marketPricesTime = time.Now()

	return prices, nil
}

// GetCachedMarketPricesByLocation returns min sell prices scoped to a specific
// station/structure (location ID), cached for 10 minutes.
func (c *Client) GetCachedMarketPricesByLocation(cache *IndustryCache, regionID int32, locationID int64) (map[int32]float64, error) {
	if locationID == 0 {
		return c.GetCachedMarketPrices(cache, regionID)
	}

	key := fmt.Sprintf("%d:%d", regionID, locationID)

	cache.marketPricesMu.RLock()
	if entry, ok := cache.locationMarketPrices[key]; ok &&
		time.Since(entry.fetchedAt) < MarketPricesCacheTTL &&
		len(entry.prices) > 0 {
		result := make(map[int32]float64, len(entry.prices))
		for typeID, price := range entry.prices {
			result[typeID] = price
		}
		cache.marketPricesMu.RUnlock()
		return result, nil
	}
	cache.marketPricesMu.RUnlock()

	orders, err := c.FetchRegionOrders(regionID, "sell")
	if err != nil {
		return nil, err
	}

	prices := make(map[int32]float64)
	for _, o := range orders {
		if o.LocationID != locationID {
			continue
		}
		if existing, ok := prices[o.TypeID]; !ok || o.Price < existing {
			prices[o.TypeID] = o.Price
		}
	}

	cache.marketPricesMu.Lock()
	cache.locationMarketPrices[key] = locationMarketPricesEntry{
		prices:    prices,
		fetchedAt: time.Now(),
	}
	cache.marketPricesMu.Unlock()

	result := make(map[int32]float64, len(prices))
	for typeID, price := range prices {
		result[typeID] = price
	}
	return result, nil
}
