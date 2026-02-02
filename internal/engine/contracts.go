package engine

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"

	"eve-flipper/internal/esi"
)

const (
	// MinContractPrice filters out scam/bait contracts below this ISK threshold.
	MinContractPrice = 1_000_000
	// MaxContractMargin filters out scam contracts with unrealistically high margins (%).
	MaxContractMargin = 200
	// MinPricedRatio is the minimum fraction of item types that must have a market price.
	MinPricedRatio = 0.7
)

// ScanContracts finds profitable public contracts by comparing contract price to market value.
func (s *Scanner) ScanContracts(params ScanParams, progress func(string)) ([]ContractResult, error) {
	progress("Finding systems within radius...")
	buySystems := s.SDE.Universe.SystemsWithinRadius(params.CurrentSystemID, params.BuyRadius)
	buyRegions := s.SDE.Universe.RegionsInSet(buySystems)

	log.Printf("[DEBUG] ScanContracts: buySystems=%d, buyRegions=%d", len(buySystems), len(buyRegions))

	// Fetch market orders and contracts in parallel
	var sellOrders []esi.MarketOrder
	var allContracts []esi.PublicContract
	var wg sync.WaitGroup

	progress(fmt.Sprintf("Fetching market orders + contracts from %d regions...", len(buyRegions)))

	wg.Add(2)
	go func() {
		defer wg.Done()
		sellOrders = s.fetchOrders(buyRegions, "sell", buySystems)
	}()
	go func() {
		defer wg.Done()
		for rid := range buyRegions {
			contracts, err := s.ESI.FetchRegionContracts(rid)
			if err != nil {
				log.Printf("[DEBUG] failed to fetch contracts for region %d: %v", rid, err)
				continue
			}
			allContracts = append(allContracts, contracts...)
		}
	}()
	wg.Wait()

	log.Printf("[DEBUG] ScanContracts: %d sell orders, %d contracts total", len(sellOrders), len(allContracts))

	// Build cheapest sell price map: typeID -> price
	cheapestSell := make(map[int32]float64)
	for _, o := range sellOrders {
		if cur, ok := cheapestSell[o.TypeID]; !ok || o.Price < cur {
			cheapestSell[o.TypeID] = o.Price
		}
	}

	// Filter contracts: only item_exchange, not expired, price > 0
	// Also skip obvious scams: price < 10000 ISK (nobody sells real items for < 10k)
	var candidates []esi.PublicContract
	for _, c := range allContracts {
		if c.Type != "item_exchange" {
			continue
		}
		if c.IsExpired() {
			continue
		}
		if c.Price < MinContractPrice {
			continue // skip scam/bait contracts with very low prices (<1M ISK)
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
		progress(fmt.Sprintf("Evaluating contracts %d/%d...", done, total))
	})

	log.Printf("[DEBUG] ScanContracts: fetched items for %d contracts", len(contractItems))

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
		var pricedCount int   // how many item types we could price
		var totalTypes int    // total included item types (non-BPC)
		var topItems []string // for generating title

		hasBPO := false
		for _, item := range items {
			if !item.IsIncluded {
				continue // items the buyer must provide
			}
			if item.IsBlueprintCopy {
				continue // BPCs have no reliable market price
			}
			// Detect BPOs by name — their market price is unreliable
			if typeName, ok := s.SDE.Types[item.TypeID]; ok {
				if strings.Contains(strings.ToLower(typeName.Name), "blueprint") {
					hasBPO = true
					continue
				}
			}
			totalTypes++

			price, ok := cheapestSell[item.TypeID]
			if !ok {
				continue // can't price this item
			}
			pricedCount++
			marketValue += price * float64(item.Quantity)
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

		// Skip if we couldn't price most items (>30% unknown = unreliable)
		if totalTypes == 0 || pricedCount == 0 {
			continue
		}
		if float64(pricedCount)/float64(totalTypes) < MinPricedRatio {
			continue
		}

		if marketValue <= 0 {
			continue
		}

		// Skip scam: if margin > 1000% it's almost certainly a scam/trap
		effectiveValue := marketValue * taxMult
		profit := effectiveValue - contract.Price
		if profit <= 0 {
			continue
		}

		margin := profit / contract.Price * 100
		if margin < params.MinMargin {
			continue
		}
		if margin > MaxContractMargin {
			continue // margin >200% is almost certainly a scam or bait contract
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
				jumps = s.jumpsBetween(params.CurrentSystemID, sysID)
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
	if len(results) > MaxResults {
		results = results[:MaxResults]
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
