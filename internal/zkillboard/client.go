package zkillboard

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"eve-flipper/internal/logger"
)

const baseURL = "https://zkillboard.com/api"

// Client is a rate-limited Zkillboard API client.
// Zkillboard has strict rate limits: 10 requests per second max.
type Client struct {
	http    *http.Client
	sem     chan struct{}
	mu      sync.Mutex
	lastReq time.Time
}

// NewClient creates a Zkillboard client with rate limiting.
func NewClient() *Client {
	return &Client{
		http: &http.Client{Timeout: 60 * time.Second}, // Zkillboard can be slow
		sem:  make(chan struct{}, 5),                  // Max 5 concurrent requests
	}
}

// RegionStats contains kill statistics for a region.
type RegionStats struct {
	ID             int32                  `json:"id"`
	Type           string                 `json:"type"`
	ShipsDestroyed int64                  `json:"shipsDestroyed"`
	ISKDestroyed   float64                `json:"iskDestroyed"`
	Months         map[string]*MonthStats `json:"months"`
	ActivePVP      *ActivePVP             `json:"activepvp"`
	TopLists       []TopList              `json:"topLists"`
	Groups         map[string]*GroupStats `json:"groups"`
	Info           *RegionInfo            `json:"info"`
}

// MonthStats contains monthly kill statistics.
type MonthStats struct {
	Year           int     `json:"year"`
	Month          int     `json:"month"`
	ShipsDestroyed int64   `json:"shipsDestroyed"`
	ISKDestroyed   float64 `json:"iskDestroyed"`
}

// ActivePVP contains active PVP statistics.
type ActivePVP struct {
	Characters   *CountStat `json:"characters"`
	Corporations *CountStat `json:"corporations"`
	Alliances    *CountStat `json:"alliances"`
	Ships        *CountStat `json:"ships"`
	Systems      *CountStat `json:"systems"`
	Kills        *CountStat `json:"kills"`
}

// CountStat is a simple count statistic.
type CountStat struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// TopList contains top items of a specific type.
type TopList struct {
	Type   string     `json:"type"`
	Title  string     `json:"title"`
	Values []TopValue `json:"values"`
}

// TopValue is a single entry in a top list.
type TopValue struct {
	Kills      int    `json:"kills"`
	ID         int32  `json:"id"`
	Name       string `json:"name"`
	ShipTypeID int32  `json:"shipTypeID,omitempty"`
	ShipName   string `json:"shipName,omitempty"`
}

// GroupStats contains statistics for a ship group.
type GroupStats struct {
	GroupID        int32   `json:"groupID"`
	ShipsDestroyed int64   `json:"shipsDestroyed"`
	ISKDestroyed   float64 `json:"iskDestroyed"`
}

// RegionInfo contains basic region info.
type RegionInfo struct {
	ID       int32  `json:"id"`
	Name     string `json:"name"`
	RegionID int32  `json:"region_id"`
}

// Killmail represents a single killmail from Zkillboard.
type Killmail struct {
	KillmailID   int64    `json:"killmail_id"`
	KillmailHash string   `json:"zkb.hash"`
	ZKB          *ZKBInfo `json:"zkb"`
}

// ZKBInfo contains Zkillboard-specific killmail info.
type ZKBInfo struct {
	LocationID     int64   `json:"locationID"`
	Hash           string  `json:"hash"`
	FittedValue    float64 `json:"fittedValue"`
	DroppedValue   float64 `json:"droppedValue"`
	DestroyedValue float64 `json:"destroyedValue"`
	TotalValue     float64 `json:"totalValue"`
	Points         int     `json:"points"`
	NPC            bool    `json:"npc"`
	Solo           bool    `json:"solo"`
	Awox           bool    `json:"awox"`
}

// GetRegionStats fetches kill statistics for a region.
func (c *Client) GetRegionStats(regionID int32) (*RegionStats, error) {
	url := fmt.Sprintf("%s/stats/regionID/%d/", baseURL, regionID)

	var stats RegionStats
	if err := c.getJSON(url, &stats); err != nil {
		return nil, fmt.Errorf("get region stats %d: %w", regionID, err)
	}

	return &stats, nil
}

// GetRecentKills fetches recent killmails for a region.
func (c *Client) GetRecentKills(regionID int32, pastSeconds int) ([]map[string]interface{}, error) {
	url := fmt.Sprintf("%s/regionID/%d/pastSeconds/%d/", baseURL, regionID, pastSeconds)

	var kills []map[string]interface{}
	if err := c.getJSON(url, &kills); err != nil {
		return nil, fmt.Errorf("get recent kills for region %d: %w", regionID, err)
	}

	return kills, nil
}

// getJSON fetches a URL and decodes JSON with rate limiting.
func (c *Client) getJSON(url string, dst interface{}) error {
	c.sem <- struct{}{}
	defer func() { <-c.sem }()

	// Rate limit: minimum 200ms between requests
	c.mu.Lock()
	elapsed := time.Since(c.lastReq)
	if elapsed < 200*time.Millisecond {
		time.Sleep(200*time.Millisecond - elapsed)
	}
	c.lastReq = time.Now()
	c.mu.Unlock()

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "eve-flipper/1.0 (https://github.com/user/eve-flipper)")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		// Rate limited - wait and retry
		logger.Warn("Zkillboard", "Rate limited, waiting 10 seconds...")
		time.Sleep(10 * time.Second)
		return c.getJSON(url, dst)
	}

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("zkillboard %d: %s", resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(dst)
}

// HealthCheck pings Zkillboard to verify connectivity.
func (c *Client) HealthCheck() bool {
	url := baseURL + "/stats/regionID/10000002/" // The Forge - always has data

	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", "eve-flipper/1.0")

	resp, err := c.http.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()

	return resp.StatusCode == 200
}
