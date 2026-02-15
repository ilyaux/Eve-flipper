package esi

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxRetries    = 3
	retryBaseWait = 500 * time.Millisecond
)

const baseURL = "https://esi.evetech.net/latest"

// StationStore is a persistent L2 cache for station names.
type StationStore interface {
	GetStation(locationID int64) (string, bool)
	SetStation(locationID int64, name string)
}

// Client is a rate-limited ESI HTTP client.
// Uses two separate semaphores so that bulk scan operations
// (thousands of market-order pages) never starve lightweight
// API calls (profile, station names, history, auth).
type Client struct {
	http         *http.Client
	sem          chan struct{} // lightweight / individual API calls
	scanSem      chan struct{} // bulk scan page fetches (GetPaginatedDirect)
	mu           sync.Mutex
	stationCache sync.Map     // int64 -> string (L1 in-memory)
	stationStore StationStore // L2 persistent cache (SQLite)
	orderCache   *OrderCache  // region order cache with ETag/Expires

	// EVERef structure name fallback (loaded at startup)
	everefNames sync.Map // int64 -> string

	// Health check cache
	healthMu      sync.RWMutex
	healthOK      bool
	healthChecked time.Time
	healthLastOK  time.Time
}

// NewClient creates an ESI client with rate limiting and the given station cache store.
// Configures HTTP transport for high-concurrency connection reuse to ESI.
func NewClient(store StationStore) *Client {
	transport := &http.Transport{
		// NOTE: HTTP/2 is intentionally NOT enabled. For bulk market-order fetching
		// (300+ pages per region), HTTP/1.1 with a large connection pool is faster
		// than HTTP/2 multiplexing through a single TCP connection.
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout: 10 * time.Second,
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 100, // reuse connections to ESI instead of re-handshaking TLS
		MaxConnsPerHost:     0,   // unlimited
		IdleConnTimeout:     120 * time.Second,
	}
	return &Client{
		http:         &http.Client{Timeout: 30 * time.Second, Transport: transport},
		sem:          make(chan struct{}, 50), // for GetJSON (history, stations, auth)
		scanSem:      make(chan struct{}, 50), // for GetPaginatedDirect (market order pages)
		stationStore: store,
		orderCache:   NewOrderCache(),
	}
}

const everefStructuresURL = "https://data.everef.net/structures/structures-latest.v2.json"

// LoadEVERefStructures fetches the public structure names from EVERef as a fallback
// for when ESI returns 403 on /universe/structures/{id}/. Runs in the background.
func (c *Client) LoadEVERefStructures() {
	go func() {
		start := time.Now()
		req, err := http.NewRequest("GET", everefStructuresURL, nil)
		if err != nil {
			log.Printf("[ESI] EVERef structures: request error: %v", err)
			return
		}
		req.Header.Set("User-Agent", "eve-flipper/1.0 (github.com)")
		req.Header.Set("Accept", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			log.Printf("[ESI] EVERef structures: fetch error: %v", err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			log.Printf("[ESI] EVERef structures: HTTP %d", resp.StatusCode)
			return
		}

		// Parse JSON: map of structure_id (string key) -> object with "name" field
		var raw map[string]struct {
			Name     string `json:"name"`
			SystemID int32  `json:"solar_system_id"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
			log.Printf("[ESI] EVERef structures: decode error: %v", err)
			return
		}

		count := 0
		for idStr, s := range raw {
			id, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil || s.Name == "" {
				continue
			}
			c.everefNames.Store(id, s.Name)
			count++
		}
		log.Printf("[ESI] EVERef structures loaded: %d names in %dms", count, time.Since(start).Milliseconds())
	}()
}

// EVERefStructureName returns the structure name from the EVERef dataset, or empty string if not found.
func (c *Client) EVERefStructureName(structureID int64) string {
	if v, ok := c.everefNames.Load(structureID); ok {
		return v.(string)
	}
	return ""
}

// HealthCheck pings ESI to verify connectivity.
// Results are cached for 10 seconds to avoid spamming ESI.
func (c *Client) HealthCheck() bool {
	c.healthMu.RLock()
	if time.Since(c.healthChecked) < 10*time.Second {
		ok := c.healthOK
		c.healthMu.RUnlock()
		return ok
	}
	c.healthMu.RUnlock()

	// Perform actual check
	c.healthMu.Lock()
	defer c.healthMu.Unlock()

	// Double-check after acquiring write lock
	if time.Since(c.healthChecked) < 10*time.Second {
		return c.healthOK
	}

	req, err := http.NewRequest("GET", baseURL+"/status/?datasource=tranquility", nil)
	if err != nil {
		c.healthOK = false
		c.healthChecked = time.Now()
		return false
	}
	req.Header.Set("User-Agent", "eve-flipper/1.0 (github.com)")
	resp, err := c.http.Do(req)
	if err != nil {
		c.healthOK = false
		c.healthChecked = time.Now()
		return false
	}
	resp.Body.Close()

	c.healthOK = resp.StatusCode == 200
	c.healthChecked = time.Now()
	if c.healthOK {
		c.healthLastOK = time.Now()
	}
	return c.healthOK
}

// HealthStatus returns detailed health information.
func (c *Client) HealthStatus() (ok bool, lastOK time.Time) {
	c.healthMu.RLock()
	defer c.healthMu.RUnlock()
	return c.healthOK, c.healthLastOK
}

// isNPCStation returns true if the location ID is in the NPC station range.
func isNPCStation(locationID int64) bool {
	return locationID >= 60000000 && locationID < 64000000
}

// isPlayerStructure returns true if the location ID looks like a player-owned structure.
func isPlayerStructure(locationID int64) bool {
	return locationID >= 100000000 && !isNPCStation(locationID)
}

// StationName fetches and caches a station name by ID.
// For NPC stations (60M–64M), uses /universe/stations/{id}/ (unauthenticated).
// For player structures, returns cached name or "Structure {id}" (use StructureName for auth-based resolution).
func (c *Client) StationName(locationID int64) string {
	// L1: in-memory cache
	if v, ok := c.stationCache.Load(locationID); ok {
		return v.(string)
	}
	// L2: persistent DB cache
	if c.stationStore != nil {
		if name, ok := c.stationStore.GetStation(locationID); ok {
			c.stationCache.Store(locationID, name)
			return name
		}
	}
	// L3: ESI API (only for NPC stations — structures require auth)
	name := fmt.Sprintf("Location %d", locationID)
	if isNPCStation(locationID) {
		var info struct {
			Name string `json:"name"`
		}
		url := fmt.Sprintf("%s/universe/stations/%d/?datasource=tranquility", baseURL, locationID)
		if err := c.GetJSON(url, &info); err == nil && info.Name != "" {
			name = info.Name
		}
	} else if isPlayerStructure(locationID) {
		// Only cache placeholder in L1 — StructureName() with auth can resolve later
		name = fmt.Sprintf("Structure %d", locationID)
		c.stationCache.Store(locationID, name)
		return name
	}
	c.stationCache.Store(locationID, name)
	if c.stationStore != nil {
		c.stationStore.SetStation(locationID, name)
	}
	return name
}

// StructureName fetches and caches a player structure name using an authenticated ESI call.
// Uses GET /universe/structures/{structure_id}/ with Bearer token.
// Falls back to cache or "Structure {id}" on error.
func (c *Client) StructureName(structureID int64, accessToken string) string {
	// L1: in-memory cache (skip "Structure NNN" / "Location NNN" placeholders)
	if v, ok := c.stationCache.Load(structureID); ok {
		name := v.(string)
		if !strings.HasPrefix(name, "Structure ") && !strings.HasPrefix(name, "Location ") {
			return name
		}
	}
	// L2: persistent DB cache (skip placeholders)
	if c.stationStore != nil {
		if name, ok := c.stationStore.GetStation(structureID); ok {
			if !strings.HasPrefix(name, "Structure ") && !strings.HasPrefix(name, "Location ") {
				c.stationCache.Store(structureID, name)
				return name
			}
		}
	}
	// L3: Authenticated ESI call
	var info struct {
		Name          string `json:"name"`
		SolarSystemID int32  `json:"solar_system_id"`
	}
	url := fmt.Sprintf("%s/universe/structures/%d/?datasource=tranquility", baseURL, structureID)
	if err := c.AuthGetJSON(url, accessToken, &info); err == nil && info.Name != "" {
		log.Printf("[ESI] Resolved structure %d → %q", structureID, info.Name)
		c.stationCache.Store(structureID, info.Name)
		if c.stationStore != nil {
			c.stationStore.SetStation(structureID, info.Name)
		}
		return info.Name
	} else if err != nil {
		log.Printf("[ESI] StructureName(%d) failed: %v", structureID, err)
	}
	// L4: EVERef fallback — public structure dataset
	if eveName := c.EVERefStructureName(structureID); eveName != "" {
		log.Printf("[ESI] Resolved structure %d via EVERef → %q", structureID, eveName)
		c.stationCache.Store(structureID, eveName)
		if c.stationStore != nil {
			c.stationStore.SetStation(structureID, eveName)
		}
		return eveName
	}
	// Fallback — DON'T cache placeholder so retries can resolve it later
	// (e.g., when token is refreshed or structure becomes accessible)
	name := fmt.Sprintf("Structure %d", structureID)
	return name
}

// StructureInfo holds details about a player-owned structure.
type StructureInfo struct {
	ID       int64
	Name     string
	SystemID int32
	RegionID int32
}

// FetchSystemStructures discovers player-owned structures with active markets
// in a given system by scanning region orders for non-NPC location IDs.
// Requires an authenticated access token for structure name resolution.
func (c *Client) FetchSystemStructures(systemID int32, regionID int32, accessToken string) ([]StructureInfo, error) {
	// Fetch all region orders (uses cache)
	orders, err := c.FetchRegionOrders(regionID, "all")
	if err != nil {
		return nil, fmt.Errorf("fetch region orders: %w", err)
	}

	// Collect unique structure location IDs in the target system
	seen := make(map[int64]bool)
	for _, o := range orders {
		if o.SystemID == systemID && isPlayerStructure(o.LocationID) {
			seen[o.LocationID] = true
		}
	}

	if len(seen) == 0 {
		return nil, nil
	}

	// Resolve structure names concurrently
	type result struct {
		id   int64
		name string
	}
	results := make(chan result, len(seen))
	var wg sync.WaitGroup
	for id := range seen {
		wg.Add(1)
		go func(sid int64) {
			defer wg.Done()
			name := c.StructureName(sid, accessToken)
			results <- result{id: sid, name: name}
		}(id)
	}
	wg.Wait()
	close(results)

	var structures []StructureInfo
	for r := range results {
		structures = append(structures, StructureInfo{
			ID:       r.id,
			Name:     r.name,
			SystemID: systemID,
			RegionID: regionID,
		})
	}
	return structures, nil
}

// PrefetchStructureNames fetches structure names concurrently for a set of location IDs
// that are player structures. Requires an access token.
func (c *Client) PrefetchStructureNames(locationIDs map[int64]bool, accessToken string) {
	var toFetch []int64
	for id := range locationIDs {
		if !isPlayerStructure(id) {
			continue
		}
		if v, ok := c.stationCache.Load(id); ok {
			name := v.(string)
			if !strings.HasPrefix(name, "Structure ") && !strings.HasPrefix(name, "Location ") {
				continue // already resolved
			}
		}
		toFetch = append(toFetch, id)
	}
	if len(toFetch) == 0 {
		return
	}
	var wg sync.WaitGroup
	for _, id := range toFetch {
		wg.Add(1)
		go func(sid int64) {
			defer wg.Done()
			c.StructureName(sid, accessToken)
		}(id)
	}
	wg.Wait()
}

// isRetryable returns true if the HTTP status code indicates a transient error worth retrying.
func isRetryable(statusCode int) bool {
	return statusCode == 420 || statusCode == 429 || statusCode == 502 || statusCode == 503 || statusCode == 504 || statusCode == 520
}

// PostJSON sends a POST request with a JSON body and decodes the response into dst.
// Uses the lightweight semaphore and retries transient errors like GetJSON.
func (c *Client) PostJSON(url string, body interface{}, dst interface{}) error {
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal POST body: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			wait := retryBaseWait * time.Duration(1<<(attempt-1))
			time.Sleep(wait)
		}

		c.sem <- struct{}{}

		req, err := http.NewRequest("POST", url, nil)
		if err != nil {
			<-c.sem
			return err
		}
		req.Body = io.NopCloser(&bytesReader{data: bodyBytes})
		req.ContentLength = int64(len(bodyBytes))
		req.Header.Set("User-Agent", "eve-flipper/1.0 (github.com)")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			<-c.sem
			lastErr = err
			log.Printf("[ESI] POST failed (attempt %d/%d): %v", attempt+1, maxRetries+1, err)
			continue
		}

		if resp.StatusCode == 200 {
			decErr := json.NewDecoder(resp.Body).Decode(dst)
			resp.Body.Close()
			<-c.sem
			return decErr
		}

		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		<-c.sem
		lastErr = fmt.Errorf("ESI POST %d: %s", resp.StatusCode, string(respBody))

		if !isRetryable(resp.StatusCode) {
			return lastErr
		}
		log.Printf("[ESI] POST retryable %d (attempt %d/%d): %s", resp.StatusCode, attempt+1, maxRetries+1, url)
	}

	return lastErr
}

// bytesReader is a simple io.Reader over a byte slice (avoids importing bytes package).
type bytesReader struct {
	data []byte
	pos  int
}

func (r *bytesReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

// GetJSON fetches a URL and decodes JSON into dst.
// Retries up to maxRetries times on transient ESI errors (502/503/504) with exponential backoff.
// Semaphore is released before sleeping so other requests can proceed.
func (c *Client) GetJSON(url string, dst interface{}) error {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			wait := retryBaseWait * time.Duration(1<<(attempt-1)) // 500ms, 1s, 2s
			time.Sleep(wait)
		}

		c.sem <- struct{}{} // acquire only for the actual request

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			<-c.sem
			return err
		}
		req.Header.Set("User-Agent", "eve-flipper/1.0 (github.com)")
		req.Header.Set("Accept", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			<-c.sem
			lastErr = err
			log.Printf("[ESI] Request failed (attempt %d/%d): %v", attempt+1, maxRetries+1, err)
			continue
		}

		if resp.StatusCode == 200 {
			decErr := json.NewDecoder(resp.Body).Decode(dst)
			resp.Body.Close()
			<-c.sem
			return decErr
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		<-c.sem // release before potential retry sleep
		lastErr = fmt.Errorf("ESI %d: %s", resp.StatusCode, string(body))

		if !isRetryable(resp.StatusCode) {
			return lastErr
		}
		log.Printf("[ESI] Retryable error %d (attempt %d/%d): %s", resp.StatusCode, attempt+1, maxRetries+1, url)
	}

	return lastErr
}

// GetPaginated fetches all pages from a paginated ESI endpoint (unauthenticated).
func (c *Client) GetPaginated(url string) ([]json.RawMessage, error) {
	return c.getPaginatedInternal(url, "")
}

// AuthGetPaginated fetches all pages from a paginated ESI endpoint with an access token.
// Required for authenticated endpoints like corp journal and corp orders.
func (c *Client) AuthGetPaginated(url, accessToken string) ([]json.RawMessage, error) {
	return c.getPaginatedInternal(url, accessToken)
}

// getPaginatedInternal is the shared implementation for paginated fetches.
// If accessToken is non-empty, it is sent as a Bearer token.
func (c *Client) getPaginatedInternal(url, accessToken string) ([]json.RawMessage, error) {
	c.sem <- struct{}{}

	sep := "&"
	if !strings.Contains(url, "?") {
		sep = "?"
	}
	req, err := http.NewRequest("GET", url+sep+"page=1", nil)
	if err != nil {
		<-c.sem
		return nil, err
	}
	req.Header.Set("User-Agent", "eve-flipper/1.0 (github.com)")
	req.Header.Set("Accept", "application/json")
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		<-c.sem
		return nil, err
	}

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		<-c.sem
		return nil, fmt.Errorf("ESI paginated %d: %s", resp.StatusCode, string(body))
	}

	totalPages := 1
	if p := resp.Header.Get("X-Pages"); p != "" {
		totalPages, _ = strconv.Atoi(p)
	}

	var page1 []json.RawMessage
	json.NewDecoder(resp.Body).Decode(&page1)
	resp.Body.Close()
	<-c.sem

	if totalPages == 1 {
		return page1, nil
	}

	// Fetch remaining pages concurrently
	type pageResult struct {
		page int
		data []json.RawMessage
		err  error
	}

	results := make(chan pageResult, totalPages-1)
	for p := 2; p <= totalPages; p++ {
		go func(pageNum int) {
			pageURL := fmt.Sprintf("%s%spage=%d", url, sep, pageNum)
			var data []json.RawMessage
			var fetchErr error
			if accessToken != "" {
				fetchErr = c.AuthGetJSON(pageURL, accessToken, &data)
			} else {
				fetchErr = c.GetJSON(pageURL, &data)
			}
			results <- pageResult{page: pageNum, data: data, err: fetchErr}
		}(p)
	}

	all := make([]json.RawMessage, 0, len(page1)*totalPages)
	all = append(all, page1...)
	for i := 0; i < totalPages-1; i++ {
		r := <-results
		if r.err != nil {
			log.Printf("[ESI] Paginated page %d failed: %v", r.page, r.err)
			continue
		}
		all = append(all, r.data...)
	}
	return all, nil
}

// GetPaginatedDirect fetches all pages and decodes directly into MarketOrder slice.
func (c *Client) GetPaginatedDirect(url string, regionID int32) ([]MarketOrder, error) {
	orders, _, _, err := c.getPaginatedDirectWithHeaders(url, regionID)
	return orders, err
}

// getPaginatedDirectWithHeaders fetches all pages, returning ETag and Expires from page 1.
// Uses scanSem so bulk page fetches never starve regular API calls.
// Retries transient ESI errors with exponential backoff; semaphore released during sleep.
func (c *Client) getPaginatedDirectWithHeaders(url string, regionID int32) ([]MarketOrder, string, time.Time, error) {
	// Fetch page 1 with retry
	var page1 []MarketOrder
	var totalPages int
	var respEtag string
	var respExpires time.Time
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(retryBaseWait * time.Duration(1<<(attempt-1)))
		}

		c.scanSem <- struct{}{}

		req, err := newESIRequest(url + "&page=1")
		if err != nil {
			<-c.scanSem
			return nil, "", time.Time{}, err
		}

		resp, err := c.http.Do(req)
		if err != nil {
			<-c.scanSem
			lastErr = err
			log.Printf("[ESI] Page 1 failed (attempt %d/%d): %v", attempt+1, maxRetries+1, err)
			continue
		}

		if resp.StatusCode != 200 {
			resp.Body.Close()
			<-c.scanSem
			lastErr = fmt.Errorf("ESI %d on page 1", resp.StatusCode)
			if !isRetryable(resp.StatusCode) {
				return nil, "", time.Time{}, lastErr
			}
			log.Printf("[ESI] Page 1 retryable %d (attempt %d/%d)", resp.StatusCode, attempt+1, maxRetries+1)
			continue
		}

		totalPages = 1
		if p := resp.Header.Get("X-Pages"); p != "" {
			totalPages, _ = strconv.Atoi(p)
		}
		respEtag = resp.Header.Get("Etag")
		respExpires = parseExpires(resp)

		json.NewDecoder(resp.Body).Decode(&page1)
		resp.Body.Close()
		<-c.scanSem
		lastErr = nil
		break
	}

	if lastErr != nil {
		return nil, "", time.Time{}, lastErr
	}

	for i := range page1 {
		page1[i].RegionID = regionID
	}

	if totalPages <= 1 {
		return page1, respEtag, respExpires, nil
	}

	type pageResult struct {
		data []MarketOrder
		err  error
	}

	results := make(chan pageResult, totalPages-1)
	for p := 2; p <= totalPages; p++ {
		go func(pageNum int) {
			var data []MarketOrder
			pageURL := fmt.Sprintf("%s&page=%d", url, pageNum)

			for attempt := 0; attempt <= maxRetries; attempt++ {
				if attempt > 0 {
					time.Sleep(retryBaseWait * time.Duration(1<<(attempt-1)))
				}

				c.scanSem <- struct{}{}

				pageReq, err := newESIRequest(pageURL)
				if err != nil {
					<-c.scanSem
					results <- pageResult{err: err}
					return
				}

				pageResp, err := c.http.Do(pageReq)
				if err != nil {
					<-c.scanSem
					if attempt == maxRetries {
						log.Printf("[ESI] Page %d failed after %d attempts: %v", pageNum, maxRetries+1, err)
						results <- pageResult{err: err}
						return
					}
					continue
				}

				if pageResp.StatusCode != 200 {
					pageResp.Body.Close()
					<-c.scanSem
					if !isRetryable(pageResp.StatusCode) || attempt == maxRetries {
						log.Printf("[ESI] Page %d error %d after %d attempts", pageNum, pageResp.StatusCode, attempt+1)
						results <- pageResult{err: fmt.Errorf("ESI %d", pageResp.StatusCode)}
						return
					}
					continue
				}

				json.NewDecoder(pageResp.Body).Decode(&data)
				pageResp.Body.Close()
				<-c.scanSem
				for i := range data {
					data[i].RegionID = regionID
				}
				results <- pageResult{data: data}
				return
			}

			results <- pageResult{err: fmt.Errorf("ESI page %d: exhausted retries", pageNum)}
		}(p)
	}

	all := make([]MarketOrder, 0, len(page1)*totalPages)
	all = append(all, page1...)
	for i := 0; i < totalPages-1; i++ {
		r := <-results
		if r.err != nil {
			log.Printf("[ESI] Skipping failed page: %v", r.err)
			continue
		}
		all = append(all, r.data...)
	}
	return all, respEtag, respExpires, nil
}

// PrefetchStationNames fetches station names concurrently for a set of location IDs.
func (c *Client) PrefetchStationNames(locationIDs map[int64]bool) {
	var toFetch []int64
	for id := range locationIDs {
		if _, ok := c.stationCache.Load(id); ok {
			continue
		}
		if id >= 60000000 && id < 64000000 {
			toFetch = append(toFetch, id)
		} else {
			c.stationCache.Store(id, fmt.Sprintf("Location %d", id))
		}
	}
	if len(toFetch) == 0 {
		return
	}

	var wg sync.WaitGroup
	for _, id := range toFetch {
		wg.Add(1)
		go func(lid int64) {
			defer wg.Done()
			c.StationName(lid)
		}(id)
	}
	wg.Wait()
}
