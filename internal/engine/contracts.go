package engine

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"sync"

	"eve-flipper/internal/esi"
)

const (
	// DefaultMinContractPrice filters out scam/bait contracts below this ISK threshold.
	DefaultMinContractPrice = 10_000_000 // 10M ISK
	// DefaultMaxContractMargin filters out scam contracts with unrealistically high margins (%).
	DefaultMaxContractMargin = 100 // margins >100% are almost always scams
	// DefaultMinPricedRatio is the minimum fraction of item types that must have a market price.
	DefaultMinPricedRatio = 0.8
	// MinSellOrderVolume is the minimum total sell volume to trust the price.
	MinSellOrderVolume = 5
	// MaxVWAPDeviation is the maximum % deviation from VWAP to consider contract valid.
	MaxVWAPDeviation = 30.0
	// MinDailyVolumeForContract filters out items with no recent trading activity.
	MinDailyVolumeForContract = 1
)

// getContractFilters returns effective filter values, using defaults if params are 0.
func getContractFilters(params ScanParams) (minPrice, maxMargin, minPricedRatio float64) {
	minPrice = params.MinContractPrice
	if minPrice <= 0 {
		minPrice = DefaultMinContractPrice
	}
	maxMargin = params.MaxContractMargin
	if maxMargin <= 0 {
		maxMargin = DefaultMaxContractMargin
	}
	minPricedRatio = params.MinPricedRatio
	if minPricedRatio <= 0 {
		minPricedRatio = DefaultMinPricedRatio
	}
	return
}

// itemPriceData holds market data for an item type.
type itemPriceData struct {
	MinSellPrice float64 // Cheapest sell order price
	TotalSellVol int32   // Total volume of sell orders
	SellOrderCnt int     // Number of sell orders
	VWAP         float64 // Volume-weighted average price from history (0 if no history)
	DailyVolume  float64 // Average daily trading volume
	HasHistory   bool    // Whether we have reliable history data
}

// ScanContracts finds profitable public contracts by comparing contract price to market value.
func (s *Scanner) ScanContracts(params ScanParams, progress func(string)) ([]ContractResult, error) {
	// Get effective filter values
	minContractPrice, maxContractMargin, minPricedRatio := getContractFilters(params)

	progress("Finding systems within radius...")
	var buySystems map[int32]int
	if params.MinRouteSecurity > 0 {
		buySystems = s.SDE.Universe.SystemsWithinRadiusMinSecurity(params.CurrentSystemID, params.BuyRadius, params.MinRouteSecurity)
	} else {
		buySystems = s.SDE.Universe.SystemsWithinRadius(params.CurrentSystemID, params.BuyRadius)
	}
	buyRegions := s.SDE.Universe.RegionsInSet(buySystems)

	log.Printf("[DEBUG] ScanContracts: buySystems=%d, buyRegions=%d, minPrice=%.0f, maxMargin=%.1f",
		len(buySystems), len(buyRegions), minContractPrice, maxContractMargin)

	// Fetch market orders and contracts in parallel
	var sellOrders []esi.MarketOrder
	var allContracts []esi.PublicContract
	var contractsMu sync.Mutex
	var wg sync.WaitGroup

	progress(fmt.Sprintf("Fetching market orders + contracts from %d regions...", len(buyRegions)))

	wg.Add(2)
	go func() {
		defer wg.Done()
		sellOrders = s.fetchOrders(buyRegions, "sell", buySystems)
	}()
	go func() {
		defer wg.Done()
		// Fetch contracts from ALL regions in PARALLEL (with caching)
		var contractsWg sync.WaitGroup
		for rid := range buyRegions {
			contractsWg.Add(1)
			go func(regionID int32) {
				defer contractsWg.Done()
				contracts, err := s.ESI.FetchRegionContractsCached(s.ContractsCache, regionID)
				if err != nil {
					log.Printf("[DEBUG] failed to fetch contracts for region %d: %v", regionID, err)
					return
				}
				contractsMu.Lock()
				allContracts = append(allContracts, contracts...)
				contractsMu.Unlock()
			}(rid)
		}
		contractsWg.Wait()
	}()
	wg.Wait()

	log.Printf("[DEBUG] ScanContracts: %d sell orders, %d contracts total", len(sellOrders), len(allContracts))

	// Build price data map: typeID -> itemPriceData
	// Track min price, total volume, and order count per type
	priceData := make(map[int32]*itemPriceData)
	for _, o := range sellOrders {
		pd, ok := priceData[o.TypeID]
		if !ok {
			pd = &itemPriceData{MinSellPrice: math.MaxFloat64}
			priceData[o.TypeID] = pd
		}
		if o.Price < pd.MinSellPrice {
			pd.MinSellPrice = o.Price
		}
		pd.TotalSellVol += o.VolumeRemain
		pd.SellOrderCnt++
	}

	// Clean up items with insufficient market data
	for typeID, pd := range priceData {
		if pd.MinSellPrice == math.MaxFloat64 {
			delete(priceData, typeID)
			continue
		}
		// Require minimum sell volume to trust the price
		if pd.TotalSellVol < MinSellOrderVolume {
			pd.MinSellPrice = pd.MinSellPrice * 1.5 // Penalize low-volume items
		}
	}

	// Filter contracts: only item_exchange, not expired, price > threshold
	var candidates []esi.PublicContract
	for _, c := range allContracts {
		if c.Type != "item_exchange" {
			continue
		}
		if c.IsExpired() {
			continue
		}
		if c.Price < minContractPrice {
			continue // skip scam/bait contracts with very low prices
		}
		candidates = append(candidates, c)
	}

	log.Printf("[DEBUG] ScanContracts: %d item_exchange candidates after filtering", len(candidates))
	progress(fmt.Sprintf("Evaluating %d contracts...", len(candidates)))

	if len(candidates) == 0 {
		return nil, nil
	}

	// Fetch items for all candidates
	contractIDs := make([]int32, len(candidates))
	for i, c := range candidates {
		contractIDs[i] = c.ContractID
	}

	contractItems := s.ESI.FetchContractItemsBatch(contractIDs, func(done, total int) {
		progress(fmt.Sprintf("Fetching contract items %d/%d...", done, total))
	})

	log.Printf("[DEBUG] ScanContracts: fetched items for %d contracts", len(contractItems))

	// Collect unique type IDs that need history lookup
	typeIDsNeedHistory := make(map[int32]bool)
	for _, items := range contractItems {
		for _, item := range items {
			if item.IsIncluded && !item.IsBlueprintCopy {
				if _, ok := priceData[item.TypeID]; ok {
					typeIDsNeedHistory[item.TypeID] = true
				}
			}
		}
	}

	// Fetch market history for pricing validation (use first region for VWAP)
	var primaryRegion int32
	for rid := range buyRegions {
		primaryRegion = rid
		break
	}

	if s.History != nil && len(typeIDsNeedHistory) > 0 {
		progress(fmt.Sprintf("Fetching market history for %d item types...", len(typeIDsNeedHistory)))
		s.fetchContractItemsHistory(typeIDsNeedHistory, priceData, primaryRegion)
	}

	log.Printf("[DEBUG] ScanContracts: enriched %d types with history", len(typeIDsNeedHistory))

	// Calculate profit for each contract
	taxMult := 1.0 - params.SalesTaxPercent/100
	if taxMult < 0 {
		taxMult = 0
	}

	var results []ContractResult

	for _, contract := range candidates {
		items, ok := contractItems[contract.ContractID]
		if !ok || len(items) == 0 {
			continue
		}

		var marketValue float64
		var itemCount int32
		var pricedCount int        // how many item types we could price
		var totalTypes int         // total included item types (non-BPC)
		var topItems []string      // for generating title
		var lowVolumeItems int     // items with suspicious low trading volume
		var highDeviationItems int // items where sell price deviates significantly from VWAP

		hasBPO := false
		for _, item := range items {
			if !item.IsIncluded {
				continue // items the buyer must provide
			}
			if item.IsBlueprintCopy {
				continue // BPCs have no reliable market price
			}

			// Detect BPOs/BPCs by checking SDE category or name
			if typeName, ok := s.SDE.Types[item.TypeID]; ok {
				nameLower := strings.ToLower(typeName.Name)
				if strings.Contains(nameLower, "blueprint") {
					hasBPO = true
					continue
				}
			}
			totalTypes++

			pd, ok := priceData[item.TypeID]
			if !ok || pd.MinSellPrice == 0 || pd.MinSellPrice == math.MaxFloat64 {
				continue // can't price this item
			}

			// Determine the best price to use: prefer VWAP if available and reliable
			var usePrice float64
			if pd.HasHistory && pd.VWAP > 0 {
				// Use VWAP as primary, but cap at min sell price (can't sell above market)
				usePrice = math.Min(pd.VWAP, pd.MinSellPrice)

				// Check if min sell deviates too much from VWAP (potential bait order)
				if pd.MinSellPrice < pd.VWAP*0.5 {
					// Sell price is <50% of VWAP — likely a bait order, use VWAP instead
					usePrice = pd.VWAP * 0.9 // Conservative estimate
					highDeviationItems++
				}
			} else {
				// No history — if RequireHistory is set, skip this item entirely
				if params.RequireHistory {
					continue
				}
				// No history — use min sell but be conservative
				usePrice = pd.MinSellPrice
			}

			// Track items with low daily volume (unreliable pricing)
			if pd.DailyVolume < MinDailyVolumeForContract {
				lowVolumeItems++
			}

			pricedCount++
			marketValue += usePrice * float64(item.Quantity)
			itemCount += item.Quantity

			// Build item name for title generation
			if typeName, ok := s.SDE.Types[item.TypeID]; ok {
				if item.Quantity > 1 {
					topItems = append(topItems, fmt.Sprintf("%dx %s", item.Quantity, typeName.Name))
				} else {
					topItems = append(topItems, typeName.Name)
				}
			}
		}

		// Skip contracts that are purely BPOs — unreliable market pricing
		if hasBPO && totalTypes == 0 {
			continue
		}

		// Skip if we couldn't price most items
		if totalTypes == 0 || pricedCount == 0 {
			continue
		}
		if float64(pricedCount)/float64(totalTypes) < minPricedRatio {
			continue
		}

		// Skip if too many items have low volume (unreliable pricing)
		if pricedCount > 0 && float64(lowVolumeItems)/float64(pricedCount) > 0.5 {
			continue // >50% items have no recent trading — unreliable
		}

		// Skip if too many items have suspicious price deviations
		if pricedCount > 0 && float64(highDeviationItems)/float64(pricedCount) > 0.3 {
			continue // >30% items have bait-like pricing — suspicious
		}

		if marketValue <= 0 {
			continue
		}

		// Calculate profit
		effectiveValue := marketValue * taxMult
		profit := effectiveValue - contract.Price
		if profit <= 0 {
			continue
		}

		margin := profit / contract.Price * 100
		if margin < params.MinMargin {
			continue
		}
		if margin > maxContractMargin {
			continue // margin too high — likely a scam or pricing error
		}

		// Generate title from items if contract title is empty
		title := strings.TrimSpace(contract.Title)
		if title == "" {
			if len(topItems) == 1 {
				title = topItems[0]
			} else if len(topItems) <= 3 {
				title = strings.Join(topItems, ", ")
			} else {
				title = fmt.Sprintf("%s + %d more", strings.Join(topItems[:2], ", "), len(topItems)-2)
			}
		}

		stationName := s.ESI.StationName(contract.StartLocationID)

		// Calculate jumps from current system to contract station
		jumps := 0
		sysID := s.locationToSystem(contract.StartLocationID)
		if sysID != 0 {
			if d, ok := buySystems[sysID]; ok {
				jumps = d
			} else {
				jumps = s.jumpsBetweenWithSecurity(params.CurrentSystemID, sysID, params.MinRouteSecurity)
			}
		}

		var profitPerJump float64
		if jumps > 0 {
			profitPerJump = profit / float64(jumps)
		}

		results = append(results, ContractResult{
			ContractID:    contract.ContractID,
			Title:         title,
			Price:         contract.Price,
			MarketValue:   marketValue,
			Profit:        sanitizeFloat(profit),
			MarginPercent: sanitizeFloat(margin),
			Volume:        contract.Volume,
			StationName:   stationName,
			ItemCount:     itemCount,
			Jumps:         jumps,
			ProfitPerJump: sanitizeFloat(profitPerJump),
		})
	}

	log.Printf("[DEBUG] ScanContracts: %d profitable results", len(results))

	// Sort by profit descending, keep top 100
	sort.Slice(results, func(i, j int) bool {
		return results[i].Profit > results[j].Profit
	})
	limit := EffectiveMaxResults(params.MaxResults, DefaultMaxResults)
	if len(results) > limit {
		results = results[:limit]
	}

	progress(fmt.Sprintf("Found %d profitable contracts", len(results)))
	return results, nil
}

// locationToSystem maps a station/structure ID to its solar system ID.
func (s *Scanner) locationToSystem(locationID int64) int32 {
	if station, ok := s.SDE.Stations[locationID]; ok {
		return station.SystemID
	}
	return 0
}

// fetchContractItemsHistory fetches market history for contract items and calculates VWAP.
func (s *Scanner) fetchContractItemsHistory(typeIDs map[int32]bool, priceData map[int32]*itemPriceData, regionID int32) {
	if s.History == nil || len(typeIDs) == 0 {
		return
	}

	// Use semaphore to limit concurrent requests (increased from 10 to 30)
	sem := make(chan struct{}, 30)
	var wg sync.WaitGroup

	for typeID := range typeIDs {
		pd, ok := priceData[typeID]
		if !ok {
			continue
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(tid int32, pdata *itemPriceData) {
			defer wg.Done()
			defer func() { <-sem }()

			// Try cache first
			entries, ok := s.History.GetMarketHistory(regionID, tid)
			if !ok {
				// Fetch from ESI
				var err error
				entries, err = s.ESI.FetchMarketHistory(regionID, tid)
				if err != nil {
					return
				}
				s.History.SetMarketHistory(regionID, tid, entries)
			}

			if len(entries) == 0 {
				return
			}

			// Calculate VWAP (30 days)
			pdata.VWAP = CalcVWAP(entries, 30)
			pdata.DailyVolume = avgDailyVolume(entries, 7)
			pdata.HasHistory = true
		}(typeID, pd)
	}

	wg.Wait()
}
