package esi

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// FetchStructureOrders fetches all market orders for a specific Upwell structure.
// Requires an authenticated access token with structure-market scope.
func (c *Client) FetchStructureOrders(structureID int64, accessToken string) ([]MarketOrder, error) {
	return c.FetchStructureOrdersContext(context.Background(), structureID, accessToken)
}

func (c *Client) FetchStructureOrdersContext(ctx context.Context, structureID int64, accessToken string) ([]MarketOrder, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if structureID <= 0 {
		return nil, fmt.Errorf("invalid structure id: %d", structureID)
	}
	if strings.TrimSpace(accessToken) == "" {
		return nil, fmt.Errorf("access token required for structure market")
	}
	cache := c.ensureOrderCache()
	if cache == nil {
		return nil, fmt.Errorf("esi client is nil")
	}

	url := fmt.Sprintf("%s/markets/structures/%d/?datasource=tranquility", baseURL, structureID)
	tokenHash := structureMarketTokenHash(accessToken)
	cacheKey := orderCacheKey{Scope: "structure", LocationID: structureID, OrderType: "all", TokenHash: tokenHash}
	sfKey := fmt.Sprintf("structure:%d:%s", structureID, tokenHash)
	result, err, _ := cache.Do(sfKey, func() (interface{}, error) {
		if cached, _, hit := cache.GetScoped(cacheKey); hit {
			log.Printf("[ESI] StructureOrderCache HIT structure=%d (%d orders)", structureID, len(cached))
			return cached, nil
		}

		raw, err := c.AuthGetPaginatedContext(ctx, url, accessToken)
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
		expires := time.Now().Add(5 * time.Minute)
		cache.PutScoped(cacheKey, orders, "", expires)
		c.recordMarketOrderSnapshot(MarketOrderSnapshot{
			OrderType:  "all",
			Source:     "structure",
			LocationID: structureID,
			ExpiresAt:  expires,
			CapturedAt: time.Now().UTC(),
			Orders:     orders,
		})
		log.Printf("[ESI] StructureOrderCache MISS structure=%d (%d orders)", structureID, len(orders))
		return orders, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]MarketOrder), nil
}

func structureMarketTokenHash(accessToken string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(accessToken)))
	return fmt.Sprintf("%x", sum[:8])
}
