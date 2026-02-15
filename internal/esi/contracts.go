package esi

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// ContractsCache caches public contracts by region.
type ContractsCache struct {
	mu        sync.RWMutex
	contracts map[int32][]PublicContract // regionID -> contracts
	times     map[int32]time.Time        // regionID -> fetch time
}

// ContractsCacheTTL is how long contracts are cached (5 minutes).
const ContractsCacheTTL = 5 * time.Minute

// NewContractsCache creates a new contracts cache.
func NewContractsCache() *ContractsCache {
	return &ContractsCache{
		contracts: make(map[int32][]PublicContract),
		times:     make(map[int32]time.Time),
	}
}

// PublicContract represents a public contract from ESI.
type PublicContract struct {
	ContractID          int32   `json:"contract_id"`
	Type                string  `json:"type"`
	Price               float64 `json:"price"`
	Buyout              float64 `json:"buyout"`
	Reward              float64 `json:"reward"`
	Collateral          float64 `json:"collateral"`
	Volume              float64 `json:"volume"`
	StartLocationID     int64   `json:"start_location_id"`
	EndLocationID       int64   `json:"end_location_id"`
	IssuerID            int32   `json:"issuer_id"`
	IssuerCorporationID int32   `json:"issuer_corporation_id"`
	DateIssued          string  `json:"date_issued"`
	DateExpired         string  `json:"date_expired"`
	DaysToComplete      int     `json:"days_to_complete"`
	ForCorporation      bool    `json:"for_corporation"`
	Title               string  `json:"title"`
}

// ContractItem represents an item inside a public contract.
type ContractItem struct {
	RecordID           int64   `json:"record_id"`
	TypeID             int32   `json:"type_id"`
	Quantity           int32   `json:"quantity"`
	IsIncluded         bool    `json:"is_included"`
	IsBlueprintCopy    bool    `json:"is_blueprint_copy"`
	ItemID             int64   `json:"item_id"`
	MaterialEfficiency int     `json:"material_efficiency"`
	TimeEfficiency     int     `json:"time_efficiency"`
	Runs               int     `json:"runs"`
	Flag               int     `json:"flag"`      // Item location flag (46-53 = Rig Slots, 0 = Cargo, etc.)
	Singleton          bool    `json:"singleton"` // True for fitted items
	Damage             float64 `json:"damage"`    // Damage level 0.0-1.0 (0.1 = 10% damaged)
}

// FetchRegionContracts fetches all public contracts for a region (paginated).
func (c *Client) FetchRegionContracts(regionID int32) ([]PublicContract, error) {
	contractsURL := fmt.Sprintf("%s/contracts/public/%d/?datasource=tranquility", baseURL, regionID)

	// Fetch page 1 to get total pages
	c.sem <- struct{}{}
	req, _ := http.NewRequest("GET", contractsURL+"&page=1", nil)
	req.Header.Set("User-Agent", "eve-flipper/1.0 (github.com)")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		<-c.sem
		return nil, err
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		<-c.sem
		return nil, fmt.Errorf("ESI contracts %d: %s", resp.StatusCode, string(body))
	}

	totalPages := 1
	if p := resp.Header.Get("X-Pages"); p != "" {
		totalPages, _ = strconv.Atoi(p)
	}

	var page1 []PublicContract
	if err := json.NewDecoder(resp.Body).Decode(&page1); err != nil {
		resp.Body.Close()
		<-c.sem
		return nil, fmt.Errorf("decode contracts page 1: %w", err)
	}
	resp.Body.Close()
	<-c.sem

	if totalPages == 1 {
		return page1, nil
	}

	log.Printf("[DEBUG] contracts region %d: %d pages", regionID, totalPages)

	type pageResult struct {
		data []PublicContract
		err  error
	}

	results := make(chan pageResult, totalPages-1)
	for p := 2; p <= totalPages; p++ {
		go func(pageNum int) {
			var data []PublicContract
			pageURL := fmt.Sprintf("%s&page=%d", contractsURL, pageNum)
			err := c.GetJSON(pageURL, &data)
			results <- pageResult{data: data, err: err}
		}(p)
	}

	all := make([]PublicContract, 0, len(page1)*totalPages)
	all = append(all, page1...)
	for i := 0; i < totalPages-1; i++ {
		r := <-results
		if r.err != nil {
			continue
		}
		all = append(all, r.data...)
	}
	return all, nil
}

// FetchContractItems fetches items of a single public contract.
func (c *Client) FetchContractItems(contractID int32) ([]ContractItem, error) {
	url := fmt.Sprintf("%s/contracts/public/items/%d/?datasource=tranquility", baseURL, contractID)
	var items []ContractItem
	err := c.GetJSON(url, &items)
	return items, err
}

// ContractItemsCacheMaxSize is the maximum number of contracts to cache.
// Once exceeded, oldest entries are evicted to prevent unbounded memory growth.
const ContractItemsCacheMaxSize = 10_000

// ContractItemsCache caches fetched contract items (they never change once created).
// Bounded to ContractItemsCacheMaxSize entries; oldest are evicted when full.
type ContractItemsCache struct {
	mu    sync.RWMutex
	items map[int32][]ContractItem // contractID -> items (nil = fetched but empty/error)
	order []int32                  // insertion order for FIFO eviction
}

// NewContractItemsCache creates a new contract items cache.
func NewContractItemsCache() *ContractItemsCache {
	return &ContractItemsCache{
		items: make(map[int32][]ContractItem),
		order: make([]int32, 0, ContractItemsCacheMaxSize),
	}
}

// evictOldest removes the oldest entries until the cache is within MaxSize.
// Must be called with mu held.
func (cc *ContractItemsCache) evictOldest() {
	for len(cc.items) >= ContractItemsCacheMaxSize && len(cc.order) > 0 {
		oldest := cc.order[0]
		cc.order = cc.order[1:]
		delete(cc.items, oldest)
	}
}

// FetchContractItemsBatch fetches items for multiple contracts using a worker pool.
// Uses cache to skip already-fetched contracts. 50 parallel workers for throughput.
// Returns a map of contractID -> []ContractItem. Failed fetches are silently skipped.
func (c *Client) FetchContractItemsBatch(contractIDs []int32, cache *ContractItemsCache, progress func(done, total int)) map[int32][]ContractItem {
	total := len(contractIDs)
	if total == 0 {
		return nil
	}

	out := make(map[int32][]ContractItem)

	// Separate cached vs uncached
	var uncached []int32
	if cache != nil {
		cache.mu.RLock()
		for _, id := range contractIDs {
			if items, ok := cache.items[id]; ok {
				if items != nil {
					out[id] = items
				}
			} else {
				uncached = append(uncached, id)
			}
		}
		cache.mu.RUnlock()
	} else {
		uncached = contractIDs
	}

	cachedCount := total - len(uncached)
	if cachedCount > 0 {
		log.Printf("[DEBUG] ContractItemsBatch: %d cached, %d to fetch", cachedCount, len(uncached))
	}

	if len(uncached) == 0 {
		progress(total, total)
		return out
	}

	const workers = 50

	type result struct {
		id    int32
		items []ContractItem
		err   error
	}

	jobs := make(chan int32, workers*2)
	results := make(chan result, workers*2)

	// Start workers
	var wg sync.WaitGroup
	numWorkers := workers
	if len(uncached) < numWorkers {
		numWorkers = len(uncached)
	}
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				items, err := c.FetchContractItems(id)
				if err == nil && len(items) > 0 {
					results <- result{id: id, items: items, err: nil}
				} else {
					results <- result{id: id, items: nil, err: err}
				}
			}
		}()
	}

	// Feed jobs
	go func() {
		for _, id := range uncached {
			jobs <- id
		}
		close(jobs)
	}()

	// Close results when all workers done
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	done := cachedCount
	for r := range results {
		if r.items != nil {
			out[r.id] = r.items
		}
		// Cache successful results (including empty lists), but do not cache
		// transient fetch errors to avoid sticky false-negatives.
		if cache != nil {
			if r.err == nil {
				cache.mu.Lock()
				if _, exists := cache.items[r.id]; !exists {
					cache.evictOldest()
					cache.order = append(cache.order, r.id)
				}
				cache.items[r.id] = r.items
				cache.mu.Unlock()
			} else {
				log.Printf("[DEBUG] ContractItemsBatch: fetch failed for %d: %v", r.id, r.err)
			}
		}
		done++
		if done%50 == 0 || done == total {
			progress(done, total)
		}
	}
	return out
}

// IsExpired checks if a contract's expiration date has passed.
func (c PublicContract) IsExpired() bool {
	t, err := time.Parse(time.RFC3339, c.DateExpired)
	if err != nil {
		return true
	}
	return time.Now().After(t)
}

// FetchRegionContractsCached fetches contracts with caching.
func (c *Client) FetchRegionContractsCached(cache *ContractsCache, regionID int32) ([]PublicContract, error) {
	// Check cache
	cache.mu.RLock()
	if contracts, ok := cache.contracts[regionID]; ok {
		if time.Since(cache.times[regionID]) < ContractsCacheTTL {
			cache.mu.RUnlock()
			return contracts, nil
		}
	}
	cache.mu.RUnlock()

	// Fetch fresh
	contracts, err := c.FetchRegionContracts(regionID)
	if err != nil {
		return nil, err
	}

	// Update cache
	cache.mu.Lock()
	cache.contracts[regionID] = contracts
	cache.times[regionID] = time.Now()
	cache.mu.Unlock()

	return contracts, nil
}
