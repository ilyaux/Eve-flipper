package engine

import (
	"math"
	"sort"
	"sync"

	"eve-flipper/internal/esi"
)

const (
	defaultRegionalPeriodDays = 14
)

type RegionalDayTradeItem struct {
	TypeID             int32     `json:"type_id"`
	TypeName           string    `json:"type_name"`
	SourceSystemID     int32     `json:"source_system_id"`
	SourceSystemName   string    `json:"source_system_name"`
	SourceStationName  string    `json:"source_station_name"`
	SourceLocationID   int64     `json:"source_location_id"`
	SourceRegionID     int32     `json:"source_region_id"`
	SourceRegionName   string    `json:"source_region_name"`
	TargetSystemID     int32     `json:"target_system_id"`
	TargetSystemName   string    `json:"target_system_name"`
	TargetStationName  string    `json:"target_station_name"`
	TargetLocationID   int64     `json:"target_location_id"`
	TargetRegionID     int32     `json:"target_region_id"`
	TargetRegionName   string    `json:"target_region_name"`
	PurchaseUnits      int32     `json:"purchase_units"`
	SourceUnits        int32     `json:"source_units"`
	TargetDemandPerDay float64   `json:"target_demand_per_day"`
	TargetSupplyUnits  int64     `json:"target_supply_units"`
	TargetDOS          float64   `json:"target_dos"`
	Assets             int64     `json:"assets"`
	ActiveOrders       int64     `json:"active_orders"`
	SourceAvgPrice     float64   `json:"source_avg_price"`
	TargetNowPrice     float64   `json:"target_now_price"`
	TargetPeriodPrice  float64   `json:"target_period_price"`
	TargetNowProfit    float64   `json:"target_now_profit"`
	TargetPeriodProfit float64   `json:"target_period_profit"`
	ROINow             float64   `json:"roi_now"`
	ROIPeriod          float64   `json:"roi_period"`
	CapitalRequired    float64   `json:"capital_required"`
	ItemVolume         float64   `json:"item_volume"`
	ShippingCost       float64   `json:"shipping_cost"`
	Jumps              int       `json:"jumps"`
	MarginNow          float64   `json:"margin_now"`
	MarginPeriod       float64   `json:"margin_period"`
	CategoryID         int32     `json:"category_id"`
	GroupID            int32     `json:"group_id"`
	GroupName          string    `json:"group_name"`
	TradeScore         float64   `json:"trade_score"`
	TargetPriceHistory []float64 `json:"target_price_history"`
	TargetLowestSell   float64   `json:"target_lowest_sell"`
}

type RegionalDayTradeHub struct {
	SourceSystemID     int32                  `json:"source_system_id"`
	SourceSystemName   string                 `json:"source_system_name"`
	SourceRegionID     int32                  `json:"source_region_id"`
	SourceRegionName   string                 `json:"source_region_name"`
	Security           float64                `json:"security"`
	PurchaseUnits      int64                  `json:"purchase_units"`
	SourceUnits        int64                  `json:"source_units"`
	TargetDemandPerDay float64                `json:"target_demand_per_day"`
	TargetSupplyUnits  int64                  `json:"target_supply_units"`
	TargetDOS          float64                `json:"target_dos"`
	Assets             int64                  `json:"assets"`
	ActiveOrders       int64                  `json:"active_orders"`
	TargetNowProfit    float64                `json:"target_now_profit"`
	TargetPeriodProfit float64                `json:"target_period_profit"`
	CapitalRequired    float64                `json:"capital_required"`
	ShippingCost       float64                `json:"shipping_cost"`
	ItemCount          int                    `json:"item_count"`
	Items              []RegionalDayTradeItem `json:"items"`
}

type RegionalInventorySnapshot struct {
	AssetsByType     map[int32]int64
	ActiveSellByType map[int32]int64
}

type regionalHistoryStats struct {
	avgPrice      float64
	demandPerDay  float64
	drvi          float64
	windowEntries int
	entries       []esi.HistoryEntry
}

type regionalHistoryKey struct {
	regionID int32
	typeID   int32
}

func regionalPeriodDays(params ScanParams) int {
	if params.AvgPricePeriod > 0 {
		return params.AvgPricePeriod
	}
	return defaultRegionalPeriodDays
}

func regionalPurchaseDemandDays(params ScanParams) float64 {
	if params.PurchaseDemandDays > 0 {
		return params.PurchaseDemandDays
	}
	// Sell-order mode is less immediate and usually sized below 1 full demand day.
	// Align default behavior with common "0.5 DoD" workflow.
	if params.SellOrderMode {
		return 0.5
	}
	return 1.0
}

func (s *Scanner) historyEntries(regionID int32, typeID int32) []esi.HistoryEntry {
	if regionID <= 0 || typeID <= 0 {
		return nil
	}
	if s.History != nil {
		if entries, ok := s.History.GetMarketHistory(regionID, typeID); ok {
			return entries
		}
	}
	if s.ESI == nil {
		return nil
	}
	entries, err := s.ESI.FetchMarketHistory(regionID, typeID)
	if err != nil {
		return nil
	}
	if s.History != nil {
		s.History.SetMarketHistory(regionID, typeID, entries)
	}
	return entries
}

func (s *Scanner) BuildRegionalDayTrader(
	params ScanParams,
	flips []FlipResult,
	inventory *RegionalInventorySnapshot,
	progress func(string),
) ([]RegionalDayTradeHub, int, string, int) {
	if len(flips) == 0 {
		return nil, 0, "", regionalPeriodDays(params)
	}

	periodDays := regionalPeriodDays(params)
	if progress != nil {
		progress("Building regional day-trader metrics...")
	}

	targetRegionID := params.TargetRegionID
	if targetRegionID <= 0 {
		targetRegionID = flips[0].SellRegionID
	}
	targetRegionName := ""
	if s.SDE != nil {
		targetRegionName = s.regionName(targetRegionID)
	}

	buyCostMult, sellRevenueMult := tradeFeeMultipliers(tradeFeeInputs{
		SplitTradeFees:       params.SplitTradeFees,
		BrokerFeePercent:     params.BrokerFeePercent,
		SalesTaxPercent:      params.SalesTaxPercent,
		BuyBrokerFeePercent:  params.BuyBrokerFeePercent,
		SellBrokerFeePercent: params.SellBrokerFeePercent,
		BuySalesTaxPercent:   params.BuySalesTaxPercent,
		SellSalesTaxPercent:  params.SellSalesTaxPercent,
	})

	needed := make(map[regionalHistoryKey]bool)
	for _, row := range flips {
		if row.BuyRegionID > 0 {
			needed[regionalHistoryKey{regionID: row.BuyRegionID, typeID: row.TypeID}] = true
		}
		if row.SellRegionID > 0 {
			needed[regionalHistoryKey{regionID: row.SellRegionID, typeID: row.TypeID}] = true
		}
	}

	statsByKey := make(map[regionalHistoryKey]regionalHistoryStats, len(needed))
	if len(needed) > 0 {
		if progress != nil {
			progress("Loading period price and demand metrics...")
		}
		sem := make(chan struct{}, 12)
		type result struct {
			key   regionalHistoryKey
			stats regionalHistoryStats
		}
		outCh := make(chan result, len(needed))
		var wg sync.WaitGroup
		for key := range needed {
			wg.Add(1)
			go func(k regionalHistoryKey) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				entries := s.historyEntries(k.regionID, k.typeID)
				if len(entries) == 0 {
					outCh <- result{key: k}
					return
				}
				avg, _, _ := CalcAvgPriceStats(entries, periodDays)
				volWindowDays := periodDays
				if volWindowDays < 14 {
					volWindowDays = 14
				}
				if volWindowDays > 30 {
					volWindowDays = 30
				}
				outCh <- result{
					key: k,
					stats: regionalHistoryStats{
						avgPrice:      sanitizeFloat(avg),
						demandPerDay:  sanitizeFloat(avgDailyVolume(entries, periodDays)),
						drvi:          sanitizeFloat(CalcDRVI(entries, volWindowDays)),
						windowEntries: len(filterLastNDays(entries, periodDays)),
						entries:       entries,
					},
				}
			}(key)
		}
		wg.Wait()
		close(outCh)
		for item := range outCh {
			statsByKey[item.key] = item.stats
		}
	}

	shippingRate := params.ShippingCostPerM3Jump
	if shippingRate < 0 {
		shippingRate = 0
	}

	remainingAssets := make(map[int32]int64)
	remainingActive := make(map[int32]int64)
	if inventory != nil {
		for typeID, qty := range inventory.AssetsByType {
			if qty > 0 {
				remainingAssets[typeID] = qty
			}
		}
		for typeID, qty := range inventory.ActiveSellByType {
			if qty > 0 {
				remainingActive[typeID] = qty
			}
		}
	}

	hubMap := make(map[int32]*RegionalDayTradeHub)
	// Weighted hub DOS accumulator: use ISK-at-risk (capital required) as weight
	// so heterogeneous item units do not distort aggregate DOS.
	hubDOSWeighted := make(map[int32]float64)
	hubDOSWeight := make(map[int32]float64)
	totalItems := 0

	for _, row := range flips {
		if row.TypeID <= 0 || row.UnitsToBuy <= 0 {
			continue
		}

		// Category filter: early exit before any costly calculations.
		if len(params.CategoryIDs) > 0 && s.SDE != nil {
			if typeInfo, ok := s.SDE.Types[row.TypeID]; ok {
				if !containsInt32(params.CategoryIDs, typeInfo.CategoryID) {
					continue
				}
			}
		}

		sourceStats := statsByKey[regionalHistoryKey{
			regionID: row.BuyRegionID,
			typeID:   row.TypeID,
		}]
		targetStats := statsByKey[regionalHistoryKey{
			regionID: row.SellRegionID,
			typeID:   row.TypeID,
		}]

		// Source buy cost:
		// 1) live executable price first,
		// 2) L1 ask fallback,
		// 3) historical fallback,
		// then stabilize extreme live-vs-history dislocations.
		sourceAvgPrice := stabilizedSourceBuyPrice(
			row.ExpectedBuyPrice,
			row.BuyPrice,
			sourceStats,
			periodDays,
		)
		// targetNowPrice: revenue side for the "Now" profit column.
		// Instant mode  → highest buy order at destination (immediate liquidity).
		// Sell-order mode → lowest sell order at destination (you compete as a seller).
		//                   Falls back to VWAP when no live sell data is available.
		targetNowPrice := row.SellPrice
		if row.ExpectedSellPrice > 0 {
			targetNowPrice = row.ExpectedSellPrice
		}
		if params.SellOrderMode {
			if row.TargetLowestSell > 0 {
				targetNowPrice = row.TargetLowestSell
			} else if targetStats.avgPrice > 0 {
				// No live sell orders at target — VWAP is the best available proxy.
				targetNowPrice = targetStats.avgPrice
			}
		}
		targetPeriodPrice := stabilizedTargetPeriodPrice(
			targetStats,
			targetNowPrice,
			row.TargetLowestSell,
			periodDays,
			params.SellOrderMode,
		)
		referenceSellPrice := minPositivePrice(targetNowPrice, targetPeriodPrice)
		referenceSellPrice = minPositivePrice(referenceSellPrice, row.TargetLowestSell)
		if rejectUntrustedMicroSourcePrice(sourceAvgPrice, referenceSellPrice, sourceStats, periodDays) {
			continue
		}

		targetDemandPerDay := blendedRegionalDemandPerDay(row, targetStats, periodDays)

		purchaseUnits := row.UnitsToBuy
		if params.SellOrderMode && row.SellOrderRemain > 0 {
			// In sell-order mode we are not constrained by destination buy-book L1 depth.
			// Base size on source-side executable availability.
			purchaseUnits = row.SellOrderRemain
		}
		if targetDemandPerDay > 0 {
			demandDays := regionalPurchaseDemandDays(params)
			demandCap := int32(math.Ceil(targetDemandPerDay * demandDays))
			// Keep tiny but non-zero demand executable.
			if demandCap <= 0 {
				demandCap = 1
			}
			if demandCap < purchaseUnits {
				purchaseUnits = demandCap
			}
		}
		// Cargo cap must apply in all revenue modes, including sell-order mode.
		if params.CargoCapacity > 0 && row.Volume > 0 {
			maxByCargoF := math.Floor(params.CargoCapacity / row.Volume)
			if maxByCargoF <= 0 {
				continue
			}
			if maxByCargoF > float64(math.MaxInt32) {
				maxByCargoF = float64(math.MaxInt32)
			}
			maxByCargo := int32(maxByCargoF)
			if purchaseUnits > maxByCargo {
				purchaseUnits = maxByCargo
			}
		}
		if params.MaxInvestment > 0 {
			effectiveUnitCost := sourceAvgPrice * buyCostMult
			if effectiveUnitCost > 0 {
				maxByCapital := int32(params.MaxInvestment / effectiveUnitCost)
				if maxByCapital <= 0 {
					continue
				}
				if purchaseUnits > maxByCapital {
					purchaseUnits = maxByCapital
				}
			}
		}
		if purchaseUnits <= 0 {
			continue
		}

		coveredByAssets := int64(0)
		coveredByOrders := int64(0)
		if have := remainingAssets[row.TypeID]; have > 0 {
			cover := int64(purchaseUnits)
			if cover > have {
				cover = have
			}
			coveredByAssets = cover
			remainingAssets[row.TypeID] = have - cover
			purchaseUnits -= int32(cover)
		}
		if purchaseUnits > 0 {
			if have := remainingActive[row.TypeID]; have > 0 {
				cover := int64(purchaseUnits)
				if cover > have {
					cover = have
				}
				coveredByOrders = cover
				remainingActive[row.TypeID] = have - cover
				purchaseUnits -= int32(cover)
			}
		}
		if purchaseUnits <= 0 {
			continue
		}

		jumps := row.SellJumps
		if jumps <= 0 {
			jumps = row.TotalJumps - row.BuyJumps
		}
		if jumps <= 0 {
			jumps = 1
		}

		shippingCost := shippingRate * row.Volume * float64(purchaseUnits) * float64(jumps)
		unitNowProfit := targetNowPrice*sellRevenueMult - sourceAvgPrice*buyCostMult
		unitPeriodProfit := targetPeriodPrice*sellRevenueMult - sourceAvgPrice*buyCostMult
		nowProfit := unitNowProfit*float64(purchaseUnits) - shippingCost
		periodProfit := unitPeriodProfit*float64(purchaseUnits) - shippingCost

		if params.MinItemProfit > 0 && nowProfit < params.MinItemProfit && periodProfit < params.MinItemProfit {
			continue
		}

		capitalRequired := sourceAvgPrice * buyCostMult * float64(purchaseUnits)
		marginNow := 0.0
		marginPeriod := 0.0
		roiNow := 0.0
		roiPeriod := 0.0
		if capitalRequired > 0 {
			effectiveUnitCost := sourceAvgPrice * buyCostMult
			marginNow = sanitizeFloat((unitNowProfit / effectiveUnitCost) * 100)
			marginPeriod = sanitizeFloat((unitPeriodProfit / effectiveUnitCost) * 100)
			// ROI denominator includes shipping so numerator and denominator are consistent:
			// ROI = net_profit / total_deployed_capital (buy cost + shipping)
			totalDeployed := capitalRequired + shippingCost
			// For micro-cap positions ROI can explode while absolute alpha is tiny.
			// Use a minimum executable capital denominator to keep ranking stable.
			// Sell-order mode is inherently less certain, so apply a stricter floor.
			minExecutableCapitalForROI := 200_000.0
			if params.SellOrderMode {
				minExecutableCapitalForROI = 1_000_000.0
			}
			roiDenominator := totalDeployed
			if roiDenominator < minExecutableCapitalForROI {
				roiDenominator = minExecutableCapitalForROI
			}
			roiNow = sanitizeFloat((nowProfit / roiDenominator) * 100)
			roiPeriod = sanitizeFloat((periodProfit / roiDenominator) * 100)
			// Cap extreme outlier ROI caused by near-zero source prices and sparse books.
			// 10 000% is already extreme; above this value ranking signal is meaningless.
			const maxROIPct = 10_000.0
			if roiNow > maxROIPct {
				roiNow = maxROIPct
			}
			if roiPeriod > maxROIPct {
				roiPeriod = maxROIPct
			}
			if roiNow < -maxROIPct {
				roiNow = -maxROIPct
			}
			if roiPeriod < -maxROIPct {
				roiPeriod = -maxROIPct
			}
		}
		// MinOrderMargin must be executable "now" margin, not period expectation.
		if params.MinMargin > 0 && marginNow < params.MinMargin {
			continue
		}
		if params.MinPeriodROI > 0 && roiPeriod < params.MinPeriodROI {
			continue
		}
		if params.MinDemandPerDay > 0 && targetDemandPerDay < params.MinDemandPerDay {
			continue
		}

		targetSupplyUnits := regionalFallbackSupplyUnits(row, periodDays)
		targetDOS := 0.0
		if targetDemandPerDay > 0 {
			targetDOS = sanitizeFloat(float64(targetSupplyUnits) / targetDemandPerDay)
			// Cap extreme DOS: demand near-zero with any supply produces astronomic values
			// (observed: 318 349 days). Anything beyond 9 999 days is effectively "never sells".
			const maxDOS = 9_999.0
			if targetDOS > maxDOS {
				targetDOS = maxDOS
			}
		}
		// MaxDOS=0 means filter disabled.
		if params.MaxDOS > 0 && targetDOS > params.MaxDOS {
			continue
		}

		tradeScore := computeTradeScore(regionalTradeScoreInput{
			ROIPeriod:           roiPeriod,
			DemandPerDay:        targetDemandPerDay,
			DOS:                 targetDOS,
			MarginPeriod:        marginPeriod,
			HistoryEntries:      targetStats.windowEntries,
			PeriodDays:          periodDays,
			VolatilityDRVI:      targetStats.drvi,
			PriceDislocationPct: regionalPriceDislocationPct(targetNowPrice, targetPeriodPrice),
			FlowBalanceScore:    regionalFlowBalanceScore(targetDemandPerDay, row.BfSPerDay),
		})
		priceHistory := extractLastNAvgPrices(targetStats.entries, periodDays)

		item := RegionalDayTradeItem{
			TypeID:             row.TypeID,
			TypeName:           row.TypeName,
			SourceSystemID:     row.BuySystemID,
			SourceSystemName:   row.BuySystemName,
			SourceStationName:  chooseNonEmpty(row.BuyStation, row.BuySystemName),
			SourceLocationID:   row.BuyLocationID,
			SourceRegionID:     row.BuyRegionID,
			SourceRegionName:   row.BuyRegionName,
			TargetSystemID:     row.SellSystemID,
			TargetSystemName:   row.SellSystemName,
			TargetStationName:  chooseNonEmpty(row.SellStation, row.SellSystemName),
			TargetLocationID:   row.SellLocationID,
			TargetRegionID:     row.SellRegionID,
			TargetRegionName:   row.SellRegionName,
			PurchaseUnits:      purchaseUnits,
			SourceUnits:        row.SellOrderRemain,
			TargetDemandPerDay: sanitizeFloat(targetDemandPerDay),
			TargetSupplyUnits:  targetSupplyUnits,
			TargetDOS:          targetDOS,
			Assets:             coveredByAssets,
			ActiveOrders:       coveredByOrders,
			SourceAvgPrice:     sanitizeFloat(sourceAvgPrice),
			TargetNowPrice:     sanitizeFloat(targetNowPrice),
			TargetPeriodPrice:  sanitizeFloat(targetPeriodPrice),
			TargetNowProfit:    sanitizeFloat(nowProfit),
			TargetPeriodProfit: sanitizeFloat(periodProfit),
			ROINow:             roiNow,
			ROIPeriod:          roiPeriod,
			CapitalRequired:    sanitizeFloat(capitalRequired),
			ItemVolume:         sanitizeFloat(row.Volume),
			ShippingCost:       sanitizeFloat(shippingCost),
			Jumps:              jumps,
			MarginNow:          marginNow,
			MarginPeriod:       marginPeriod,
			TradeScore:         tradeScore,
			TargetPriceHistory: priceHistory,
			TargetLowestSell:   sanitizeFloat(row.TargetLowestSell),
		}

		if s.SDE != nil {
			if typeInfo, ok := s.SDE.Types[row.TypeID]; ok {
				item.CategoryID = typeInfo.CategoryID
				item.GroupID = typeInfo.GroupID
				if grp, ok := s.SDE.Groups[typeInfo.GroupID]; ok {
					item.GroupName = grp.Name
				}
			}
		}

		hub := hubMap[row.BuySystemID]
		if hub == nil {
			hub = &RegionalDayTradeHub{
				SourceSystemID:   row.BuySystemID,
				SourceSystemName: row.BuySystemName,
				SourceRegionID:   row.BuyRegionID,
				SourceRegionName: row.BuyRegionName,
			}
			if s.SDE != nil {
				if sys, ok := s.SDE.Systems[row.BuySystemID]; ok {
					hub.Security = sanitizeFloat(sys.Security)
				}
			}
			hubMap[row.BuySystemID] = hub
		}

		hub.Items = append(hub.Items, item)
		hub.PurchaseUnits += int64(item.PurchaseUnits)
		hub.SourceUnits += int64(item.SourceUnits)
		hub.TargetDemandPerDay = sanitizeFloat(hub.TargetDemandPerDay + item.TargetDemandPerDay)
		hub.TargetSupplyUnits += item.TargetSupplyUnits
		hub.Assets += item.Assets
		hub.ActiveOrders += item.ActiveOrders
		hub.TargetNowProfit = sanitizeFloat(hub.TargetNowProfit + item.TargetNowProfit)
		hub.TargetPeriodProfit = sanitizeFloat(hub.TargetPeriodProfit + item.TargetPeriodProfit)
		hub.CapitalRequired = sanitizeFloat(hub.CapitalRequired + item.CapitalRequired)
		hub.ShippingCost = sanitizeFloat(hub.ShippingCost + item.ShippingCost)
		hub.ItemCount++
		dosWeight := item.CapitalRequired
		if dosWeight <= 0 {
			dosWeight = float64(item.PurchaseUnits)
		}
		if dosWeight <= 0 {
			dosWeight = 1
		}
		hubDOSWeighted[row.BuySystemID] += item.TargetDOS * dosWeight
		hubDOSWeight[row.BuySystemID] += dosWeight
		totalItems++
	}

	hubs := make([]RegionalDayTradeHub, 0, len(hubMap))
	for _, hub := range hubMap {
		if weight := hubDOSWeight[hub.SourceSystemID]; weight > 0 {
			hub.TargetDOS = sanitizeFloat(hubDOSWeighted[hub.SourceSystemID] / weight)
		} else if hub.TargetDemandPerDay > 0 {
			hub.TargetDOS = sanitizeFloat(float64(hub.TargetSupplyUnits) / hub.TargetDemandPerDay)
		}
		sort.Slice(hub.Items, func(i, j int) bool {
			if hub.Items[i].TargetPeriodProfit == hub.Items[j].TargetPeriodProfit {
				return hub.Items[i].TargetNowProfit > hub.Items[j].TargetNowProfit
			}
			return hub.Items[i].TargetPeriodProfit > hub.Items[j].TargetPeriodProfit
		})
		hubs = append(hubs, *hub)
	}

	sort.Slice(hubs, func(i, j int) bool {
		if hubs[i].TargetPeriodProfit == hubs[j].TargetPeriodProfit {
			return hubs[i].TargetNowProfit > hubs[j].TargetNowProfit
		}
		return hubs[i].TargetPeriodProfit > hubs[j].TargetPeriodProfit
	})

	return hubs, totalItems, targetRegionName, periodDays
}

// FlattenRegionalDayHubs converts grouped regional day-trader output into
// FlipResult rows consumed by the unified ScanResultsTable and history viewer.
func FlattenRegionalDayHubs(hubs []RegionalDayTradeHub) []FlipResult {
	if len(hubs) == 0 {
		return nil
	}
	rows := make([]FlipResult, 0)
	for _, hub := range hubs {
		for _, item := range hub.Items {
			perUnitNowProfit := 0.0
			if item.PurchaseUnits > 0 {
				perUnitNowProfit = item.TargetNowProfit / float64(item.PurchaseUnits)
			}
			dailyProfit := 0.0
			if item.TargetDemandPerDay > 0 {
				dailyProfit = perUnitNowProfit * item.TargetDemandPerDay
			}
			iskPerM3Jump := 0.0
			if item.ItemVolume > 0 && item.Jumps > 0 {
				iskPerM3Jump = perUnitNowProfit / (item.ItemVolume * float64(item.Jumps))
			}

			row := FlipResult{
				TypeID:            item.TypeID,
				TypeName:          item.TypeName,
				Volume:            sanitizeFloat(item.ItemVolume),
				BuyPrice:          sanitizeFloat(item.SourceAvgPrice),
				ExpectedBuyPrice:  sanitizeFloat(item.SourceAvgPrice),
				BuyStation:        chooseNonEmpty(item.SourceStationName, item.SourceSystemName),
				BuySystemName:     item.SourceSystemName,
				BuySystemID:       item.SourceSystemID,
				BuyRegionID:       item.SourceRegionID,
				BuyRegionName:     item.SourceRegionName,
				BuyLocationID:     item.SourceLocationID,
				SellPrice:         sanitizeFloat(item.TargetNowPrice),
				ExpectedSellPrice: sanitizeFloat(item.TargetPeriodPrice),
				SellStation:       chooseNonEmpty(item.TargetStationName, item.TargetSystemName),
				SellSystemName:    item.TargetSystemName,
				SellSystemID:      item.TargetSystemID,
				SellRegionID:      item.TargetRegionID,
				SellRegionName:    item.TargetRegionName,
				SellLocationID:    item.TargetLocationID,
				ProfitPerUnit:     sanitizeFloat(perUnitNowProfit),
				MarginPercent:     sanitizeFloat(item.MarginNow),
				UnitsToBuy:        item.PurchaseUnits,
				BuyOrderRemain:    int32(item.TargetSupplyUnits),
				SellOrderRemain:   item.SourceUnits,
				TotalProfit:       sanitizeFloat(item.TargetNowProfit),
				RealProfit:        sanitizeFloat(item.TargetPeriodProfit),
				ExpectedProfit:    sanitizeFloat(item.TargetPeriodProfit),
				ProfitPerJump: func() float64 {
					if item.Jumps > 0 {
						return sanitizeFloat(item.TargetNowProfit / float64(item.Jumps))
					}
					return sanitizeFloat(item.TargetNowProfit)
				}(),
				BuyJumps:        0,
				SellJumps:       item.Jumps,
				TotalJumps:      item.Jumps,
				DailyVolume:     int64(math.Round(item.TargetDemandPerDay)),
				S2BPerDay:       sanitizeFloat(item.TargetDemandPerDay),
				BuyCompetitors:  0,
				SellCompetitors: 0,
				DailyProfit:     sanitizeFloat(dailyProfit),

				DaySecurity:           sanitizeFloat(hub.Security),
				DaySourceUnits:        item.SourceUnits,
				DayTargetDemandPerDay: sanitizeFloat(item.TargetDemandPerDay),
				DayTargetSupplyUnits:  item.TargetSupplyUnits,
				DayTargetDOS:          sanitizeFloat(item.TargetDOS),
				DayAssets:             item.Assets,
				DayActiveOrders:       item.ActiveOrders,
				DaySourceAvgPrice:     sanitizeFloat(item.SourceAvgPrice),
				DayTargetNowPrice:     sanitizeFloat(item.TargetNowPrice),
				DayTargetPeriodPrice:  sanitizeFloat(item.TargetPeriodPrice),
				DayNowProfit:          sanitizeFloat(item.TargetNowProfit),
				DayPeriodProfit:       sanitizeFloat(item.TargetPeriodProfit),
				DayROINow:             sanitizeFloat(item.ROINow),
				DayROIPeriod:          sanitizeFloat(item.ROIPeriod),
				DayCapitalRequired:    sanitizeFloat(item.CapitalRequired),
				DayShippingCost:       sanitizeFloat(item.ShippingCost),
				DayCategoryID:         item.CategoryID,
				DayGroupID:            item.GroupID,
				DayGroupName:          item.GroupName,
				DayIskPerM3Jump:       sanitizeFloat(iskPerM3Jump),
				DayTradeScore:         sanitizeFloat(item.TradeScore),
				DayPriceHistory:       item.TargetPriceHistory,
				DayTargetLowestSell:   sanitizeFloat(item.TargetLowestSell),
			}
			rows = append(rows, row)
		}
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].DayPeriodProfit == rows[j].DayPeriodProfit {
			return rows[i].DayNowProfit > rows[j].DayNowProfit
		}
		return rows[i].DayPeriodProfit > rows[j].DayPeriodProfit
	})

	return rows
}

func chooseNonEmpty(primary string, fallback string) string {
	if primary != "" {
		return primary
	}
	return fallback
}

// extractLastNAvgPrices returns the last n daily average prices from history entries,
// sorted chronologically. Used to render spark-line price charts in the UI.
func extractLastNAvgPrices(entries []esi.HistoryEntry, n int) []float64 {
	if len(entries) == 0 || n <= 0 {
		return nil
	}
	sorted := make([]esi.HistoryEntry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Date < sorted[j].Date
	})
	start := len(sorted) - n
	if start < 0 {
		start = 0
	}
	result := make([]float64, 0, len(sorted)-start)
	for _, e := range sorted[start:] {
		result = append(result, sanitizeFloat(e.Average))
	}
	return result
}

type regionalTradeScoreInput struct {
	ROIPeriod           float64
	DemandPerDay        float64
	DOS                 float64
	MarginPeriod        float64
	HistoryEntries      int
	PeriodDays          int
	VolatilityDRVI      float64
	PriceDislocationPct float64
	FlowBalanceScore    float64 // 0..1, where 1 means balanced two-sided flow.
}

// blendedRegionalDemandPerDay estimates target liquidation demand from both
// live side-flow and historical region volume.
func blendedRegionalDemandPerDay(row FlipResult, targetStats regionalHistoryStats, periodDays int) float64 {
	liveDemand := math.Max(row.S2BPerDay, 0)
	historyDemand := math.Max(targetStats.demandPerDay, 0)
	historyFlowDemand := 0.0
	if row.DailyVolume > 0 {
		historyFlowDemand = float64(row.DailyVolume)
	}

	demand := 0.0
	switch {
	case liveDemand > 0 && historyDemand > 0:
		historyWeight := 0.45
		minHistoryForTrust := periodDays / 2
		if minHistoryForTrust < 7 {
			minHistoryForTrust = 7
		}
		if targetStats.windowEntries >= minHistoryForTrust {
			historyWeight = 0.55
		}
		divergence := math.Abs(math.Log(liveDemand / historyDemand))
		if divergence > 1.2 {
			// Book flow can spike intraday. Use geometric center under large
			// disagreement to avoid overestimating liquidation speed.
			geometric := math.Sqrt(liveDemand * historyDemand)
			conservative := math.Min(liveDemand, historyDemand)
			demand = 0.7*geometric + 0.3*conservative
		} else {
			demand = (1-historyWeight)*liveDemand + historyWeight*historyDemand
		}
	case liveDemand > 0:
		demand = liveDemand
	case historyDemand > 0:
		demand = historyDemand
	case historyFlowDemand > 0:
		demand = historyFlowDemand
	}

	if historyFlowDemand > 0 && demand > 0 {
		// Cap overconfident side-flow estimates against observed traded volume.
		maxReasonable := math.Max(historyFlowDemand*3, historyFlowDemand+5)
		if demand > maxReasonable {
			demand = maxReasonable
		}
	}

	return sanitizeFloat(math.Max(demand, 0))
}

func minPositivePrice(a, b float64) float64 {
	if a > 0 && b > 0 {
		if a < b {
			return a
		}
		return b
	}
	if a > 0 {
		return a
	}
	if b > 0 {
		return b
	}
	return 0
}

// rejectUntrustedMicroSourcePrice drops rows where source price is implausibly
// tiny relative to destination context while source history is insufficient.
func rejectUntrustedMicroSourcePrice(
	sourcePrice float64,
	referenceSellPrice float64,
	sourceStats regionalHistoryStats,
	periodDays int,
) bool {
	if sourcePrice <= 0 || referenceSellPrice <= 0 {
		return false
	}
	// Absolute sanity: microscopic source versus very high sell context.
	// This catches residual junk rows that can survive even with history.
	dislocation := referenceSellPrice / sourcePrice
	if sourcePrice < 2 && dislocation > 50_000 {
		return true
	}
	if sourcePrice < 5 && dislocation > 120_000 {
		return true
	}

	if sourceStats.windowEntries >= minHistoryEntriesForTrust(periodDays) {
		return false
	}
	if sourcePrice < 1 && dislocation > 200 {
		return true
	}
	if sourcePrice < 5 && dislocation > 1000 {
		return true
	}
	if sourcePrice < 10 && dislocation > 3000 {
		return true
	}
	return false
}

func minHistoryEntriesForTrust(periodDays int) int {
	minHistory := periodDays / 2
	if minHistory < 7 {
		minHistory = 7
	}
	return minHistory
}

// robustRegionalHistoryPrice computes a conservative fair-value proxy for the
// recent period using winsorized VWAP blended with median to suppress spikes.
func robustRegionalHistoryPrice(stats regionalHistoryStats, periodDays int) float64 {
	entries := filterLastNDays(stats.entries, periodDays)
	if len(entries) == 0 {
		return sanitizeFloat(stats.avgPrice)
	}

	prices := make([]float64, 0, len(entries))
	for _, e := range entries {
		if e.Average > 0 {
			prices = append(prices, e.Average)
		}
	}
	if len(prices) == 0 {
		return sanitizeFloat(stats.avgPrice)
	}
	sort.Float64s(prices)

	if len(prices) < 5 {
		if stats.avgPrice > 0 {
			return sanitizeFloat(stats.avgPrice)
		}
		return sanitizeFloat(percentile(prices, 50))
	}

	p10 := percentile(prices, 10)
	p50 := percentile(prices, 50)
	p90 := percentile(prices, 90)

	sumPriceVol := 0.0
	sumVol := 0.0
	for _, e := range entries {
		if e.Average <= 0 || e.Volume <= 0 {
			continue
		}
		p := e.Average
		if p < p10 {
			p = p10
		}
		if p > p90 {
			p = p90
		}
		v := float64(e.Volume)
		sumPriceVol += p * v
		sumVol += v
	}

	winsorVWAP := p50
	if sumVol > 0 {
		winsorVWAP = sumPriceVol / sumVol
	}

	return sanitizeFloat(0.6*winsorVWAP + 0.4*p50)
}

// stabilizedSourceBuyPrice keeps the live source execution price but guards
// against extreme underpriced outliers when history coverage is trustworthy.
func stabilizedSourceBuyPrice(
	liveExpectedBuy float64,
	fallbackBuy float64,
	sourceStats regionalHistoryStats,
	periodDays int,
) float64 {
	sourcePrice := liveExpectedBuy
	if sourcePrice <= 0 {
		sourcePrice = fallbackBuy
	}
	if sourcePrice <= 0 {
		sourcePrice = sourceStats.avgPrice
	}
	if sourcePrice <= 0 {
		return 0
	}

	if sourceStats.windowEntries < minHistoryEntriesForTrust(periodDays) {
		return sanitizeFloat(sourcePrice)
	}

	historyPrice := robustRegionalHistoryPrice(sourceStats, periodDays)
	if historyPrice <= 0 {
		return sanitizeFloat(sourcePrice)
	}

	dislocation := historyPrice / sourcePrice
	switch {
	case dislocation <= 4:
		return sanitizeFloat(sourcePrice)
	case dislocation <= 12:
		return sanitizeFloat(math.Sqrt(sourcePrice * historyPrice))
	default:
		minFloor := historyPrice * 0.2
		if sourcePrice < minFloor {
			sourcePrice = minFloor
		}
		return sanitizeFloat(sourcePrice)
	}
}

// stabilizedTargetPeriodPrice builds period fair value from robust history and
// softly anchors it to live market context in instant/sell-order modes.
func stabilizedTargetPeriodPrice(
	targetStats regionalHistoryStats,
	targetNowPrice float64,
	targetLowestSell float64,
	periodDays int,
	sellOrderMode bool,
) float64 {
	periodPrice := robustRegionalHistoryPrice(targetStats, periodDays)
	if periodPrice <= 0 {
		periodPrice = targetNowPrice
	}
	if periodPrice <= 0 {
		periodPrice = targetLowestSell
	}
	if periodPrice <= 0 {
		return 0
	}

	if sellOrderMode {
		anchor := targetLowestSell
		if anchor <= 0 {
			anchor = targetNowPrice
		}
		if anchor > 0 {
			low := anchor * 0.55
			high := anchor * 1.65
			if periodPrice < low {
				periodPrice = low
			}
			if periodPrice > high {
				periodPrice = high
			}
		}
		return sanitizeFloat(periodPrice)
	}

	if targetNowPrice > 0 {
		maxMult := 3.5
		switch {
		case targetNowPrice < 100:
			maxMult = 8
		case targetNowPrice < 1000:
			maxMult = 6
		}

		maxAllowed := targetNowPrice * maxMult
		if targetLowestSell > 0 {
			askCap := targetLowestSell * 1.25
			if askCap > 0 && askCap < maxAllowed {
				maxAllowed = askCap
			}
		}
		if maxAllowed < targetNowPrice {
			maxAllowed = targetNowPrice
		}
		if periodPrice > maxAllowed {
			periodPrice = maxAllowed
		}

		minAllowed := targetNowPrice * 0.2
		if periodPrice < minAllowed {
			periodPrice = minAllowed
		}
	}

	return sanitizeFloat(periodPrice)
}

func regionalFallbackSupplyUnits(row FlipResult, periodDays int) int64 {
	if row.TargetSellSupply > 0 {
		return row.TargetSellSupply
	}
	if row.BfSPerDay <= 0 {
		return 0
	}
	lookaheadDays := float64(periodDays) / 7
	if lookaheadDays < 2 {
		lookaheadDays = 2
	}
	if lookaheadDays > 5 {
		lookaheadDays = 5
	}
	return int64(math.Round(math.Max(row.BfSPerDay, 0) * lookaheadDays))
}

func regionalPriceDislocationPct(nowPrice, periodPrice float64) float64 {
	if nowPrice <= 0 || periodPrice <= 0 {
		return 0
	}
	return sanitizeFloat(math.Abs(nowPrice-periodPrice) / periodPrice * 100)
}

func regionalFlowBalanceScore(demandPerDay, sourceFlowPerDay float64) float64 {
	if demandPerDay <= 0 || sourceFlowPerDay <= 0 {
		return 0.35
	}
	ratio := demandPerDay / sourceFlowPerDay
	if ratio <= 0 {
		return 0.35
	}
	return sanitizeFloat(clamp01(1 - normalize(math.Abs(math.Log(ratio)), 0, 1.5)))
}

// computeTradeScore returns a 0-100 ranking score approximating a
// "pro-trader" prioritization model:
//  1. Profitability (ROI + margin)
//  2. Liquidity quality (demand + DOS)
//  3. Confidence (history coverage, volatility, price dislocation, flow balance)
//
// with penalties for thin/unstable setups and bonuses for stable liquid setups.
func computeTradeScore(in regionalTradeScoreInput) float64 {
	periodDays := in.PeriodDays
	if periodDays <= 0 {
		periodDays = defaultRegionalPeriodDays
	}
	historyCoverage := normalize(float64(in.HistoryEntries), 0, float64(periodDays))

	roiScore := normalize(in.ROIPeriod, 0, 45) * 100
	marginScore := normalize(in.MarginPeriod, 0, 30) * 100
	profitabilityScore := 0.65*roiScore + 0.35*marginScore

	demandScore := 0.0
	if in.DemandPerDay > 0 {
		demandScore = normalize(math.Log10(in.DemandPerDay+1), math.Log10(2), math.Log10(251)) * 100
	}
	dosScore := 100.0
	if in.DOS > 0 {
		dosScore = (1 - normalize(in.DOS, 2, 45)) * 100
	}
	liquidityScore := 0.55*demandScore + 0.45*dosScore

	minCoverageDays := float64(periodDays) * 0.4
	if minCoverageDays < 3 {
		minCoverageDays = 3
	}
	coverageScore := normalize(float64(in.HistoryEntries), minCoverageDays, float64(periodDays)) * 100

	volatilityScore := 35.0
	if in.HistoryEntries >= 2 {
		volatilityScore = (1 - normalize(in.VolatilityDRVI, 0, 35)) * 100
	}
	dislocationScore := 70.0
	if in.PriceDislocationPct > 0 {
		dislocationScore = (1 - normalize(in.PriceDislocationPct, 0, 25)) * 100
	}
	flowBalance := in.FlowBalanceScore
	if flowBalance <= 0 {
		flowBalance = 0.35
	}
	flowBalance = clamp01(flowBalance)

	confidenceScore := 0.35*coverageScore +
		0.25*volatilityScore +
		0.20*dislocationScore +
		0.20*(flowBalance*100)

	profitWeight := 0.38
	liquidityWeight := 0.37
	confidenceWeight := 0.25
	if in.HistoryEntries == 0 {
		profitWeight = 0.44
		liquidityWeight = 0.40
		confidenceWeight = 0.16
	} else if historyCoverage >= 0.8 {
		profitWeight = 0.35
		liquidityWeight = 0.35
		confidenceWeight = 0.30
	}

	score := profitabilityScore*profitWeight +
		liquidityScore*liquidityWeight +
		confidenceScore*confidenceWeight

	if in.HistoryEntries == 0 {
		score -= 10
	} else if historyCoverage < 0.5 {
		score -= 5
	}
	if in.DOS > 90 {
		score -= 15
	}
	if in.DOS > 180 {
		score -= 10
	}
	if in.DOS > 365 {
		score -= 12
	}
	if in.DOS > 730 {
		score -= 8
	}
	if in.VolatilityDRVI > 60 {
		score -= 10
	}
	if in.DemandPerDay < 1 {
		score -= 8
	}
	if in.MarginPeriod > 220 && in.ROIPeriod < 5 {
		score -= 10
	}
	if in.ROIPeriod > 120 && in.DemandPerDay < 2 {
		score -= 10
	}
	if in.PriceDislocationPct > 35 {
		score -= 10
	}

	if historyCoverage >= 0.9 && in.DemandPerDay >= 20 && in.DOS > 0 && in.DOS <= 15 {
		score += 4
	}
	if in.ROIPeriod >= 15 && in.MarginPeriod >= 12 && in.DemandPerDay >= 10 && in.DOS <= 12 {
		score += 3
	}

	return sanitizeFloat(math.Max(0, math.Min(100, score)))
}

// containsInt32 returns true if needle is present in haystack.
func containsInt32(haystack []int32, needle int32) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}
