package esi

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// orderCacheKey identifies a cached set of region orders.
type orderCacheKey struct {
	RegionID  int32
	OrderType string // "sell" or "buy"
}

// orderCacheEntry holds cached orders together with HTTP caching metadata.
type orderCacheEntry struct {
	orders  []MarketOrder
	etag    string    // ETag from ESI response (page 1)
	expires time.Time // parsed Expires header
	updated time.Time // when entry was last refreshed (MISS or 304)
}

// OrderCache is a thread-safe in-memory cache for region market orders.
// It uses ETag/Expires headers from ESI to avoid re-downloading unchanged data.
// A singleflight.Group prevents duplicate in-flight fetches for the same key.
type OrderCache struct {
	mu      sync.RWMutex
	groupMu sync.RWMutex
	entries map[orderCacheKey]*orderCacheEntry
	group   *singleflight.Group
}

// OrderCacheWindow describes freshness bounds for a set of region cache entries.
type OrderCacheWindow struct {
	CurrentRevision int64
	LastRefreshAt   time.Time
	NextExpiryAt    time.Time
	MinTTLSeconds   int64
	MaxTTLSeconds   int64
	Regions         int
	Entries         int
	Stale           bool
}

// NewOrderCache creates an empty order cache.
func NewOrderCache() *OrderCache {
	return &OrderCache{
		entries: make(map[orderCacheKey]*orderCacheEntry),
		group:   &singleflight.Group{},
	}
}

// Clear removes all cached region order entries.
// Returns number of entries removed.
func (oc *OrderCache) Clear() int {
	if oc == nil {
		return 0
	}
	oc.mu.Lock()
	n := len(oc.entries)
	oc.entries = make(map[orderCacheKey]*orderCacheEntry)
	oc.mu.Unlock()

	oc.groupMu.Lock()
	oc.group = &singleflight.Group{}
	oc.groupMu.Unlock()
	return n
}

func (oc *OrderCache) Do(key string, fn func() (interface{}, error)) (interface{}, error, bool) {
	if oc == nil {
		v, err := fn()
		return v, err, false
	}
	oc.groupMu.RLock()
	group := oc.group
	oc.groupMu.RUnlock()
	if group == nil {
		group = &singleflight.Group{}
	}
	return group.Do(key, fn)
}

// EvictExpired removes all cache entries whose Expires time has passed.
// This prevents unbounded memory growth when scanning many different regions
// over time — stale entries for regions no longer being scanned are freed.
func (oc *OrderCache) EvictExpired() int {
	oc.mu.Lock()
	defer oc.mu.Unlock()

	now := time.Now()
	evicted := 0
	for key, entry := range oc.entries {
		// Keep entries that expired recently (within 30 min) for ETag revalidation.
		// Only evict entries that have been expired for a long time.
		if now.Sub(entry.expires) > 30*time.Minute {
			delete(oc.entries, key)
			evicted++
		}
	}
	return evicted
}

// Get returns cached orders if they exist and have not expired.
// Returns (orders, etag, hit).
func (oc *OrderCache) Get(regionID int32, orderType string) ([]MarketOrder, string, bool) {
	oc.mu.RLock()
	defer oc.mu.RUnlock()

	e, ok := oc.entries[orderCacheKey{regionID, orderType}]
	if !ok {
		return nil, "", false
	}
	if time.Now().After(e.expires) {
		// Expired — return etag for conditional request, but signal miss.
		return nil, e.etag, false
	}
	return e.orders, e.etag, true
}

// Put stores orders in the cache with the given etag and expiry.
// Periodically evicts long-expired entries to bound memory usage.
func (oc *OrderCache) Put(regionID int32, orderType string, orders []MarketOrder, etag string, expires time.Time) {
	oc.mu.Lock()
	defer oc.mu.Unlock()

	// Periodic eviction: when cache grows beyond 50 region+type pairs,
	// sweep out entries that expired >30 min ago.
	if len(oc.entries) > 50 {
		now := time.Now()
		for key, entry := range oc.entries {
			if now.Sub(entry.expires) > 30*time.Minute {
				delete(oc.entries, key)
			}
		}
	}

	oc.entries[orderCacheKey{regionID, orderType}] = &orderCacheEntry{
		orders:  orders,
		etag:    etag,
		expires: expires,
		updated: time.Now().UTC(),
	}
}

// Touch updates the expiry of an existing cache entry (used on 304 Not Modified).
func (oc *OrderCache) Touch(regionID int32, orderType string, expires time.Time) {
	oc.mu.Lock()
	defer oc.mu.Unlock()

	key := orderCacheKey{regionID, orderType}
	if e, ok := oc.entries[key]; ok {
		e.expires = expires
		e.updated = time.Now().UTC()
	}
}

// WindowForRegions returns cache freshness bounds for the provided region IDs.
func (oc *OrderCache) WindowForRegions(regionIDs []int32, orderType string) OrderCacheWindow {
	window := OrderCacheWindow{
		Regions: len(regionIDs),
	}
	if len(regionIDs) == 0 {
		return window
	}
	orderType = strings.ToLower(strings.TrimSpace(orderType))
	orderTypes := []string{orderType}
	if orderType == "" || orderType == "all" {
		orderTypes = []string{"sell", "buy"}
	}

	now := time.Now()
	oc.mu.RLock()
	defer oc.mu.RUnlock()

	seen := make(map[int32]bool, len(regionIDs))
	found := false
	var maxExpiry time.Time
	for _, regionID := range regionIDs {
		if seen[regionID] {
			continue
		}
		seen[regionID] = true
		for _, ot := range orderTypes {
			entry, ok := oc.entries[orderCacheKey{RegionID: regionID, OrderType: ot}]
			if !ok || entry == nil {
				continue
			}
			window.Entries++
			if !found {
				found = true
				window.NextExpiryAt = entry.expires
				window.LastRefreshAt = entry.updated
				maxExpiry = entry.expires
				continue
			}
			if entry.expires.Before(window.NextExpiryAt) {
				window.NextExpiryAt = entry.expires
			}
			if entry.expires.After(maxExpiry) {
				maxExpiry = entry.expires
			}
			if entry.updated.After(window.LastRefreshAt) {
				window.LastRefreshAt = entry.updated
			}
		}
	}

	if !found || window.NextExpiryAt.IsZero() {
		return window
	}
	if maxExpiry.IsZero() {
		maxExpiry = window.NextExpiryAt
	}

	minTTL := int64(time.Until(window.NextExpiryAt).Seconds())
	maxTTL := int64(time.Until(maxExpiry).Seconds())
	if minTTL < 0 {
		minTTL = 0
	}
	if maxTTL < 0 {
		maxTTL = 0
	}

	window.MinTTLSeconds = minTTL
	window.MaxTTLSeconds = maxTTL
	window.CurrentRevision = window.NextExpiryAt.Unix()
	window.Stale = now.After(window.NextExpiryAt)
	return window
}

// OrderCacheWindow returns cache freshness bounds for regions/order type.
func (c *Client) OrderCacheWindow(regionIDs []int32, orderType string) OrderCacheWindow {
	if c == nil || c.orderCache == nil {
		return OrderCacheWindow{Regions: len(regionIDs)}
	}
	return c.orderCache.WindowForRegions(regionIDs, orderType)
}

// ClearOrderCache clears all region order cache entries.
// Returns number of entries removed.
func (c *Client) ClearOrderCache() int {
	if c == nil || c.orderCache == nil {
		return 0
	}
	return c.orderCache.Clear()
}

// FetchRegionOrdersCached fetches region orders with full caching support:
//  1. If orders are in cache and not expired → instant return
//  2. If orders expired but we have an ETag → conditional request (If-None-Match)
//     - 304: touch expiry, return cached data (no body transfer)
//     - 200: full re-fetch, update cache
//  3. Cache miss → full fetch, populate cache
//
// Uses singleflight to coalesce concurrent requests for the same region+orderType.
func (c *Client) FetchRegionOrdersCached(regionID int32, orderType string) ([]MarketOrder, error) {
	sfKey := fmt.Sprintf("%d:%s", regionID, orderType)

	result, err, _ := c.orderCache.Do(sfKey, func() (interface{}, error) {
		return c.fetchRegionOrdersWithCache(regionID, orderType)
	})
	if err != nil {
		return nil, err
	}
	return result.([]MarketOrder), nil
}

// fetchRegionOrdersWithCache is the actual implementation behind singleflight.
func (c *Client) fetchRegionOrdersWithCache(regionID int32, orderType string) ([]MarketOrder, error) {
	// 1. Check cache
	orders, etag, hit := c.orderCache.Get(regionID, orderType)
	if hit {
		log.Printf("[ESI] OrderCache HIT region=%d type=%s (%d orders)", regionID, orderType, len(orders))
		return orders, nil
	}

	url := fmt.Sprintf("%s/markets/%d/orders/?datasource=tranquility&order_type=%s",
		baseURL, regionID, orderType)

	// 2. If we have an ETag, try conditional request on page 1
	if etag != "" {
		notModified, newExpires, err := c.conditionalCheck(url+"&page=1", etag)
		if err == nil && notModified {
			// 304 — data unchanged, refresh expiry
			c.orderCache.Touch(regionID, orderType, newExpires)
			cached, _, _ := c.orderCache.Get(regionID, orderType)
			if cached != nil {
				log.Printf("[ESI] OrderCache 304 region=%d type=%s (ETag match)", regionID, orderType)
				return cached, nil
			}
		}
		// ETag miss or error — fall through to full fetch
	}

	// 3. Full fetch
	allOrders, respEtag, respExpires, err := c.getPaginatedDirectWithHeaders(url, regionID)
	if err != nil {
		return nil, err
	}

	// Store in cache
	c.orderCache.Put(regionID, orderType, allOrders, respEtag, respExpires)
	c.recordMarketOrderSnapshot(MarketOrderSnapshot{
		RegionID:   regionID,
		OrderType:  orderType,
		Source:     "region",
		ETag:       respEtag,
		ExpiresAt:  respExpires,
		CapturedAt: time.Now().UTC(),
		Orders:     allOrders,
	})
	log.Printf("[ESI] OrderCache MISS region=%d type=%s (%d orders, expires=%s)",
		regionID, orderType, len(allOrders), respExpires.Format("15:04:05"))

	return allOrders, nil
}

// conditionalCheck sends a HEAD-like GET with If-None-Match.
// Returns (notModified, newExpires, error).
func (c *Client) conditionalCheck(pageURL, etag string) (bool, time.Time, error) {
	c.scanSem <- struct{}{}
	defer func() { <-c.scanSem }()

	req, err := newESIRequest(pageURL)
	if err != nil {
		return false, time.Time{}, err
	}
	req.Header.Set("If-None-Match", etag)

	resp, err := c.http.Do(req)
	if err != nil {
		return false, time.Time{}, err
	}
	resp.Body.Close()

	expires := parseExpires(resp)

	if resp.StatusCode == 304 {
		return true, expires, nil
	}
	return false, expires, nil
}

// newESIRequest creates a standard ESI GET request with common headers.
func newESIRequest(url string) (*http.Request, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "eve-flipper/1.0 (github.com)")
	req.Header.Set("Accept", "application/json")
	return req, nil
}

// parseExpires reads the Expires header from an ESI response.
// Falls back to 5-minute TTL if header is missing or unparseable.
func parseExpires(resp *http.Response) time.Time {
	if exp := resp.Header.Get("Expires"); exp != "" {
		if t, err := time.Parse(time.RFC1123, exp); err == nil {
			return t
		}
	}
	// Fallback: ESI market orders typically refresh every 5 minutes.
	return time.Now().Add(5 * time.Minute)
}
