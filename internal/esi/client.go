package esi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const baseURL = "https://esi.evetech.net/latest"

// StationStore is a persistent L2 cache for station names.
type StationStore interface {
	GetStation(locationID int64) (string, bool)
	SetStation(locationID int64, name string)
}

// Client is a rate-limited ESI HTTP client.
type Client struct {
	http         *http.Client
	sem          chan struct{}
	mu           sync.Mutex
	stationCache sync.Map     // int64 -> string (L1 in-memory)
	stationStore StationStore // L2 persistent cache (SQLite)
}

// NewClient creates an ESI client with rate limiting and the given station cache store.
func NewClient(store StationStore) *Client {
	return &Client{
		http:         &http.Client{Timeout: 30 * time.Second},
		sem:          make(chan struct{}, 20),
		stationStore: store,
	}
}

// HealthCheck pings ESI to verify connectivity.
func (c *Client) HealthCheck() bool {
	req, err := http.NewRequest("GET", baseURL+"/status/?datasource=tranquility", nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", "eve-flipper/1.0 (github.com)")
	resp, err := c.http.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

// StationName fetches and caches a station name by ID.
// For NPC stations (< 1B), uses /universe/stations/{id}/
// For player structures (>= 1B), returns "Structure {id}" (requires auth).
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
	// L3: ESI API
	name := fmt.Sprintf("Location %d", locationID)
	if locationID >= 60000000 && locationID < 64000000 {
		var info struct {
			Name string `json:"name"`
		}
		url := fmt.Sprintf("%s/universe/stations/%d/?datasource=tranquility", baseURL, locationID)
		if err := c.GetJSON(url, &info); err == nil && info.Name != "" {
			name = info.Name
		}
	}
	c.stationCache.Store(locationID, name)
	if c.stationStore != nil {
		c.stationStore.SetStation(locationID, name)
	}
	return name
}

// GetJSON fetches a URL and decodes JSON into dst.
func (c *Client) GetJSON(url string, dst interface{}) error {
	c.sem <- struct{}{}
	defer func() { <-c.sem }()

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "eve-flipper/1.0 (github.com)")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ESI %d: %s", resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(dst)
}

// GetPaginated fetches all pages from a paginated ESI endpoint.
func (c *Client) GetPaginated(url string) ([]json.RawMessage, error) {
	c.sem <- struct{}{}

	req, err := http.NewRequest("GET", url+"&page=1", nil)
	if err != nil {
		<-c.sem
		return nil, err
	}
	req.Header.Set("User-Agent", "eve-flipper/1.0 (github.com)")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		<-c.sem
		return nil, err
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
			var data []json.RawMessage
			pageURL := fmt.Sprintf("%s&page=%d", url, pageNum)
			err := c.GetJSON(pageURL, &data)
			results <- pageResult{page: pageNum, data: data, err: err}
		}(p)
	}

	all := make([]json.RawMessage, 0, len(page1)*totalPages)
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

// GetPaginatedDirect fetches all pages and decodes directly into MarketOrder slice.
// Avoids double unmarshal (RawMessage -> MarketOrder).
func (c *Client) GetPaginatedDirect(url string, regionID int32) ([]MarketOrder, error) {
	c.sem <- struct{}{}

	req, err := http.NewRequest("GET", url+"&page=1", nil)
	if err != nil {
		<-c.sem
		return nil, err
	}
	req.Header.Set("User-Agent", "eve-flipper/1.0 (github.com)")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		<-c.sem
		return nil, err
	}

	totalPages := 1
	if p := resp.Header.Get("X-Pages"); p != "" {
		totalPages, _ = strconv.Atoi(p)
	}

	var page1 []MarketOrder
	json.NewDecoder(resp.Body).Decode(&page1)
	resp.Body.Close()
	<-c.sem

	for i := range page1 {
		page1[i].RegionID = regionID
	}

	if totalPages == 1 {
		return page1, nil
	}

	type pageResult struct {
		data []MarketOrder
		err  error
	}

	results := make(chan pageResult, totalPages-1)
	for p := 2; p <= totalPages; p++ {
		go func(pageNum int) {
			c.sem <- struct{}{}
			defer func() { <-c.sem }()

			pageReq, err := http.NewRequest("GET", fmt.Sprintf("%s&page=%d", url, pageNum), nil)
			if err != nil {
				results <- pageResult{err: err}
				return
			}
			pageReq.Header.Set("User-Agent", "eve-flipper/1.0 (github.com)")
			pageReq.Header.Set("Accept", "application/json")

			pageResp, err := c.http.Do(pageReq)
			if err != nil {
				results <- pageResult{err: err}
				return
			}
			defer pageResp.Body.Close()

			if pageResp.StatusCode != 200 {
				results <- pageResult{err: fmt.Errorf("ESI %d", pageResp.StatusCode)}
				return
			}

			var data []MarketOrder
			json.NewDecoder(pageResp.Body).Decode(&data)
			for i := range data {
				data[i].RegionID = regionID
			}
			results <- pageResult{data: data}
		}(p)
	}

	all := make([]MarketOrder, 0, len(page1)*totalPages)
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
