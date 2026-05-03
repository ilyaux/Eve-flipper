package esi

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// FetchStructureOrders fetches all market orders for a specific Upwell structure.
// Requires an authenticated access token with structure-market scope.
func (c *Client) FetchStructureOrders(structureID int64, accessToken string) ([]MarketOrder, error) {
	if structureID <= 0 {
		return nil, fmt.Errorf("invalid structure id: %d", structureID)
	}
	if strings.TrimSpace(accessToken) == "" {
		return nil, fmt.Errorf("access token required for structure market")
	}

	url := fmt.Sprintf("%s/markets/structures/%d/?datasource=tranquility", baseURL, structureID)
	raw, err := c.AuthGetPaginated(url, accessToken)
	if err != nil {
		return nil, err
	}

	orders := make([]MarketOrder, 0, len(raw))
	for _, msg := range raw {
		var o MarketOrder
		if err := json.Unmarshal(msg, &o); err != nil {
			continue
		}
		if o.LocationID == 0 {
			o.LocationID = structureID
		}
		orders = append(orders, o)
	}
	c.recordMarketOrderSnapshot(MarketOrderSnapshot{
		OrderType:  "all",
		Source:     "structure",
		LocationID: structureID,
		CapturedAt: time.Now().UTC(),
		Orders:     orders,
	})
	return orders, nil
}
