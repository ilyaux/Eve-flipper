package esi

import (
	"fmt"
	"time"
)

// MarketOrder mirrors the ESI market order response.
type MarketOrder struct {
	OrderID      int64   `json:"order_id"`
	TypeID       int32   `json:"type_id"`
	LocationID   int64   `json:"location_id"`
	SystemID     int32   `json:"system_id"`
	Price        float64 `json:"price"`
	VolumeRemain int32   `json:"volume_remain"`
	MinVolume    int32   `json:"min_volume"`
	IsBuyOrder   bool    `json:"is_buy_order"`
	RegionID     int32   `json:"-"` // set by us
}

// MarketOrderSnapshot is a point-in-time capture of live ESI market orders.
// ESI does not expose historical order books, so callers can persist these
// snapshots as they are fetched and replay them later for orderbook backtests.
type MarketOrderSnapshot struct {
	RegionID   int32
	OrderType  string
	Source     string
	TypeID     int32
	LocationID int64
	ETag       string
	ExpiresAt  time.Time
	CapturedAt time.Time
	Orders     []MarketOrder
}

// MarketOrderRecorder persists live market order snapshots outside the ESI client.
type MarketOrderRecorder interface {
	RecordMarketOrderSnapshot(snapshot MarketOrderSnapshot) error
}

// FetchRegionOrders fetches all market orders for a region.
// Uses in-memory cache with ETag/Expires — repeated calls within the ESI refresh
// window (typically 5 min) return instantly without any network I/O.
func (c *Client) FetchRegionOrders(regionID int32, orderType string) ([]MarketOrder, error) {
	return c.FetchRegionOrdersCached(regionID, orderType)
}

// FetchRegionOrdersByType fetches all market orders for a specific type in a region.
func (c *Client) FetchRegionOrdersByType(regionID int32, typeID int32) ([]MarketOrder, error) {
	url := fmt.Sprintf("%s/markets/%d/orders/?datasource=tranquility&order_type=all&type_id=%d",
		baseURL, regionID, typeID)

	orders, err := c.GetPaginatedDirect(url, regionID)
	if err != nil {
		return nil, err
	}
	c.recordMarketOrderSnapshot(MarketOrderSnapshot{
		RegionID:   regionID,
		OrderType:  "all",
		Source:     "region_type",
		TypeID:     typeID,
		CapturedAt: time.Now().UTC(),
		Orders:     orders,
	})
	return orders, nil
}
