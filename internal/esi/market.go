package esi

import (
	"fmt"
)

// MarketOrder mirrors the ESI market order response.
type MarketOrder struct {
	OrderID      int64   `json:"order_id"`
	TypeID       int32   `json:"type_id"`
	LocationID   int64   `json:"location_id"`
	SystemID     int32   `json:"system_id"`
	Price        float64 `json:"price"`
	VolumeRemain int32   `json:"volume_remain"`
	IsBuyOrder   bool    `json:"is_buy_order"`
	RegionID     int32   `json:"-"` // set by us
}

// FetchRegionOrders fetches all market orders for a region (direct decode, no double unmarshal).
func (c *Client) FetchRegionOrders(regionID int32, orderType string) ([]MarketOrder, error) {
	url := fmt.Sprintf("%s/markets/%d/orders/?datasource=tranquility&order_type=%s",
		baseURL, regionID, orderType)

	return c.GetPaginatedDirect(url, regionID)
}
