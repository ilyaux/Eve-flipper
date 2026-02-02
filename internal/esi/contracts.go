package esi

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

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
	RecordID         int64 `json:"record_id"`
	TypeID           int32 `json:"type_id"`
	Quantity         int32 `json:"quantity"`
	IsIncluded       bool  `json:"is_included"`
	IsBlueprintCopy  bool  `json:"is_blueprint_copy"`
	ItemID           int64 `json:"item_id"`
	MaterialEfficiency int  `json:"material_efficiency"`
	TimeEfficiency   int   `json:"time_efficiency"`
	Runs             int   `json:"runs"`
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

	totalPages := 1
	if p := resp.Header.Get("X-Pages"); p != "" {
		totalPages, _ = strconv.Atoi(p)
	}

	var page1 []PublicContract
	json.NewDecoder(resp.Body).Decode(&page1)
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

// FetchContractItemsBatch fetches items for multiple contracts using a worker pool.
// 50 parallel workers to maximize throughput without spawning thousands of goroutines.
// Returns a map of contractID -> []ContractItem. Failed fetches are silently skipped.
func (c *Client) FetchContractItemsBatch(contractIDs []int32, progress func(done, total int)) map[int32][]ContractItem {
	total := len(contractIDs)
	if total == 0 {
		return nil
	}

	const workers = 50

	type result struct {
		id    int32
		items []ContractItem
	}

	jobs := make(chan int32, workers*2)
	results := make(chan result, workers*2)

	// Start workers
	var wg sync.WaitGroup
	numWorkers := workers
	if total < numWorkers {
		numWorkers = total
	}
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				items, err := c.FetchContractItems(id)
				if err == nil && len(items) > 0 {
					results <- result{id: id, items: items}
				} else {
					results <- result{id: id, items: nil}
				}
			}
		}()
	}

	// Feed jobs
	go func() {
		for _, id := range contractIDs {
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
	out := make(map[int32][]ContractItem)
	done := 0
	for r := range results {
		if r.items != nil {
			out[r.id] = r.items
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
