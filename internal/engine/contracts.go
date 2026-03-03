package engine

import (
	"context"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

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
	// DefaultContractHoldDays is the default holding horizon for non-instant mode.
	DefaultContractHoldDays = 7
	// DefaultContractTargetConfidence is the default minimum full-liquidation probability (%).
	DefaultContractTargetConfidence = 80.0
	// ContractFillParticipation is a conservative share of daily market volume we expect to capture.
	ContractFillParticipation = 0.35
	// ContractConservativePriceHaircut is an additional conservative markdown on expected proceeds.
	ContractConservativePriceHaircut = 0.03
	// ContractDailyCarryRate models opportunity/carry cost of locked capital per day.
	ContractDailyCarryRate = 0.001
	// ContractShipModuleValueFactor discounts module value when a contract contains a ship.
	// Public ESI does not reliably expose fitted-state metadata for all items.
	ContractShipModuleValueFactor = 0.55
)

// Capitals and related hulls that cannot enter highsec via gates.
// Keep list intentionally conservative; name-based fallback below covers unknown IDs.
var highsecRestrictedShipGroupIDs = map[int32]struct{}{
	30:   {}, // Titan
	485:  {}, // Dreadnought
	547:  {}, // Carrier
	659:  {}, // Supercarrier
	883:  {}, // Capital Industrial Ship (Rorqual)
	1538: {}, // Force Auxiliary
}

func isHighsecRestrictedShipGroup(groupID int32, groupName string) bool {
	if _, ok := highsecRestrictedShipGroupIDs[groupID]; ok {
		return true
	}
	// Name-based fallback for future groups not present in hardcoded IDs.
	name := strings.ToLower(strings.TrimSpace(groupName))
	switch name {
	case "titan", "dreadnought", "carrier", "supercarrier", "force auxiliary", "capital industrial ship":
		return true
	default:
		return false
	}
}

func isHighsecSecurity(security float64) bool {
	return security >= 0.45
}

// getRigSizeClass returns the size class of a rig: 1=Small, 2=Medium, 3=Large/Capital, 0=Unknown.
// Checks item name for size keywords (since some rig groups are type-based, not size-based).
func getRigSizeClass(itemName string) int {
	nameLower := strings.ToLower(itemName)
	if strings.Contains(nameLower, "small") {
		return 1
	}
	if strings.Contains(nameLower, "medium") {
		return 2
	}
	if strings.Contains(nameLower, "large") || strings.Contains(nameLower, "capital") {
		return 3
	}
	return 0 // Unknown size
}

// getShipSizeClass returns the size class of a ship based on groupID: 1=Small, 2=Medium, 3=Large, 0=Unknown.
// Uses EVE ship group IDs from SDE.
func getShipSizeClass(groupID int32) int {
	// Small: Frigate(25), Destroyer(420), Interceptor(831), Stealth Bomber(834), etc.
	if groupID == 25 || groupID == 420 || groupID == 324 || groupID == 831 || groupID == 834 || groupID == 893 || groupID == 1527 || groupID == 2016 {
		return 1 // Small (Frigate/Destroyer class)
	}
	// Medium: Cruiser(26), Battlecruiser(419,1201), Industrial(28), etc.
	if groupID == 26 || groupID == 419 || groupID == 1201 || groupID == 28 || groupID == 358 || groupID == 832 || groupID == 833 || groupID == 894 || groupID == 906 || groupID == 963 || groupID == 1305 || groupID == 1534 || groupID == 2017 || groupID == 2018 {
		return 2 // Medium (Cruiser/BC class)
	}
	// Large: Battleship(27), Capital ships, etc.
	if groupID == 27 || groupID == 381 || groupID == 485 || groupID == 547 || groupID == 659 || groupID == 883 || groupID == 898 || groupID == 900 || groupID == 902 || groupID == 1538 || groupID == 2019 {
		return 3 // Large (Battleship/Capital class)
	}
	return 0 // Unknown
}

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
	// Accept accidental percent-like inputs (e.g. 80 instead of 0.8) and clamp.
	if minPricedRatio > 1 {
		minPricedRatio = minPricedRatio / 100
	}
	if minPricedRatio > 1 {
		minPricedRatio = 1
	}
	if minPricedRatio < 0.1 {
		minPricedRatio = 0.1
	}
	return
}

func contractSellValueMultiplier(params ScanParams) float64 {
	_, _, sellBroker, sellTax := tradeFeePercents(tradeFeeInputs{
		SplitTradeFees:       params.SplitTradeFees,
		BrokerFeePercent:     params.BrokerFeePercent,
		SalesTaxPercent:      params.SalesTaxPercent,
		BuyBrokerFeePercent:  params.BuyBrokerFeePercent,
		SellBrokerFeePercent: params.SellBrokerFeePercent,
		BuySalesTaxPercent:   params.BuySalesTaxPercent,
		SellSalesTaxPercent:  params.SellSalesTaxPercent,
	})

	// Instant liquidation sells immediately into existing buy orders:
	// no broker fee is paid, only sales tax.
	if params.ContractInstantLiquidation {
		m := 1.0 - sellTax/100
		if m < 0 {
			return 0
		}
		return m
	}
	// Market-estimate mode assumes placing sell orders on market:
	// sales tax + broker fee on sell side.
	feePercent := sellTax + sellBroker
	m := 1.0 - feePercent/100
	if m < 0 {
		return 0
	}
	return m
}

func contractHoldDays(params ScanParams) int {
	if params.ContractHoldDays <= 0 {
		return DefaultContractHoldDays
	}
	if params.ContractHoldDays > 180 {
		return 180
	}
	return params.ContractHoldDays
}

func contractTargetConfidence(params ScanParams) float64 {
	if params.ContractTargetConfidence <= 0 {
		return DefaultContractTargetConfidence
	}
	if params.ContractTargetConfidence > 100 {
		return 100
	}
	return params.ContractTargetConfidence
}

func effectiveDailyVolume(pd *itemPriceData) float64 {
	if pd == nil {
		return 0
	}
	if pd.DailyVolume > 0 {
		return pd.DailyVolume
	}
	// Fallback proxy when history is unavailable: treat current book depth
	// as roughly two weeks of turnover.
	if pd.TotalSellVol > 0 {
		return float64(pd.TotalSellVol) / 14.0
	}
	return 0
}

func estimateFillDays(quantity int32, dailyVol float64) float64 {
	if quantity <= 0 {
		return 0
	}
	if dailyVol <= 0 {
		return math.Inf(1)
	}
	executablePerDay := dailyVol * ContractFillParticipation
	if executablePerDay <= 0 {
		return math.Inf(1)
	}
	return float64(quantity) / executablePerDay
}

func fillProbabilityWithinDays(fillDays, horizonDays float64) float64 {
	if horizonDays <= 0 {
		return 0
	}
	if fillDays <= 0 {
		return 1
	}
	if math.IsInf(fillDays, 1) {
		return 0
	}
	p := 1 - math.Exp(-horizonDays/fillDays)
	if p < 0 {
		return 0
	}
	if p > 1 {
		return 1
	}
	return p
}

func contractCarryDays(holdDays int, estLiqDays float64) float64 {
	if holdDays <= 0 {
		return 0
	}
	carryDays := float64(holdDays)
	if estLiqDays > 0 && estLiqDays < carryDays {
		carryDays = estLiqDays
	}
	return carryDays
}

func checkContextCanceled(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func isLikelyFittedRig(item esi.ContractItem) bool {
	if item.Singleton {
		return true
	}
	// ESI flags for rig slots in inventory APIs can vary by endpoint/client.
	// Keep both known ranges to avoid valuing non-removable fitted rigs.
	if item.Flag >= 92 && item.Flag <= 99 {
		return true
	}
	if item.Flag >= 46 && item.Flag <= 53 {
		return true
	}
	return false
}

func shouldExcludeRigWithShip(item esi.ContractItem, rigName string, shipSizeClass int, forceExclude bool) bool {
	if shipSizeClass == 0 {
		return false
	}
	if forceExclude || isLikelyFittedRig(item) {
		return true
	}
	rigSize := getRigSizeClass(rigName)
	return rigSize > 0 && rigSize == shipSizeClass
}

func blockedContractTypeID(items []esi.ContractItem) int32 {
	for _, item := range items {
		if item.Quantity <= 0 {
			continue
		}
		if isMarketDisabledType(item.TypeID) {
			return item.TypeID
		}
	}
	return 0
}

type instantValuationItem struct {
	TypeID      int32
	Quantity    int32
	Label       string
	ValueFactor float64
}

type instantLiquidationChoice struct {
	SystemID    int32
	MarketValue float64
	PricedCount int
	ItemCount   int32
	TopItems    []string
}

func (s *Scanner) contractItemLabel(typeID int32, resolvedTypeNames map[int32]string) string {
	if s != nil && s.SDE != nil {
		if typeInfo, ok := s.SDE.Types[typeID]; ok {
			if name := strings.TrimSpace(typeInfo.Name); name != "" {
				return name
			}
		}
	}
	if resolvedTypeNames != nil {
		if name, ok := resolvedTypeNames[typeID]; ok && strings.TrimSpace(name) != "" {
			return name
		}
	}
	name := ""
	if s != nil && s.ESI != nil {
		name = strings.TrimSpace(s.ESI.TypeName(typeID))
	}
	if name == "" {
		name = fmt.Sprintf("Type %d", typeID)
	}
	if resolvedTypeNames != nil {
		resolvedTypeNames[typeID] = name
	}
	return name
}

// selectInstantLiquidationSystem chooses a single liquidation system where all
// contract items can be sold with available buy-book depth.
func selectInstantLiquidationSystem(
	items []instantValuationItem,
	buyBooksByTypeBySystem map[int32]map[int32][]esi.MarketOrder,
	systemAllowed func(systemID int32) bool,
) (instantLiquidationChoice, bool) {
	if len(items) == 0 {
		return instantLiquidationChoice{}, false
	}

	candidates := make(map[int32]bool)
	firstType := true
	plansByTypeBySystem := make(map[int32]map[int32]ExecutionPlanResult, len(items))

	for _, item := range items {
		typeBooks := buyBooksByTypeBySystem[item.TypeID]
		if len(typeBooks) == 0 {
			return instantLiquidationChoice{}, false
		}

		plans := make(map[int32]ExecutionPlanResult, len(typeBooks))
		for systemID, book := range typeBooks {
			if systemAllowed != nil && !systemAllowed(systemID) {
				continue
			}
			plan := ComputeExecutionPlan(book, item.Quantity, false)
			if !plan.CanFill || plan.ExpectedPrice <= 0 {
				continue
			}
			plans[systemID] = plan
		}
		if len(plans) == 0 {
			return instantLiquidationChoice{}, false
		}
		plansByTypeBySystem[item.TypeID] = plans

		if firstType {
			for systemID := range plans {
				candidates[systemID] = true
			}
			firstType = false
			continue
		}
		for systemID := range candidates {
			if _, ok := plans[systemID]; !ok {
				delete(candidates, systemID)
			}
		}
		if len(candidates) == 0 {
			return instantLiquidationChoice{}, false
		}
	}

	best := instantLiquidationChoice{MarketValue: -1}
	for systemID := range candidates {
		value := 0.0
		itemCount := int32(0)
		topItems := make([]string, 0, len(items))

		for _, item := range items {
			plan, ok := plansByTypeBySystem[item.TypeID][systemID]
			if !ok || plan.ExpectedPrice <= 0 {
				value = -1
				break
			}
			value += plan.ExpectedPrice * float64(item.Quantity) * item.ValueFactor
			itemCount += item.Quantity
			if item.Quantity > 1 {
				topItems = append(topItems, fmt.Sprintf("%dx %s", item.Quantity, item.Label))
			} else {
				topItems = append(topItems, item.Label)
			}
		}
		if value < 0 {
			continue
		}
		if value > best.MarketValue || (value == best.MarketValue && (best.SystemID == 0 || systemID < best.SystemID)) {
			best = instantLiquidationChoice{
				SystemID:    systemID,
				MarketValue: value,
				PricedCount: len(items),
				ItemCount:   itemCount,
				TopItems:    topItems,
			}
		}
	}

	if best.SystemID == 0 || best.PricedCount == 0 || best.MarketValue <= 0 {
		return instantLiquidationChoice{}, false
	}
	return best, true
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
	return s.ScanContractsWithContext(context.Background(), params, progress)
}

// ScanContractsWithContext is cancellation-aware variant of ScanContracts.
func (s *Scanner) ScanContractsWithContext(ctx context.Context, params ScanParams, progress func(string)) ([]ContractResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := checkContextCanceled(ctx); err != nil {
		return nil, err
	}
	emitProgress := func(msg string) {
		if progress == nil {
			return
		}
		if checkContextCanceled(ctx) != nil {
			return
		}
		progress(msg)
	}

	// Get effective filter values
	minContractPrice, maxContractMargin, minPricedRatio := getContractFilters(params)

	emitProgress("Finding systems within radius...")
	ignored := ignoredSystemSetFromIDs(params.IgnoredSystemIDs)
	var buySystems map[int32]int
	if params.MinRouteSecurity > 0 {
		buySystems = s.SDE.Universe.SystemsWithinRadiusMinSecurity(params.CurrentSystemID, params.BuyRadius, params.MinRouteSecurity)
	} else {
		buySystems = s.SDE.Universe.SystemsWithinRadius(params.CurrentSystemID, params.BuyRadius)
	}
	buySystems = filterSystemDistanceMap(buySystems, ignored)
	if len(buySystems) == 0 {
		emitProgress("No systems remain after applying ignored systems filter.")
		return []ContractResult{}, nil
	}
	buyRegions := s.SDE.Universe.RegionsInSet(buySystems)
	contractInstant := params.ContractInstantLiquidation

	var sellSystems map[int32]int
	var sellRegions map[int32]bool
	if contractInstant {
		if params.MinRouteSecurity > 0 {
			sellSystems = s.SDE.Universe.SystemsWithinRadiusMinSecurity(params.CurrentSystemID, params.SellRadius, params.MinRouteSecurity)
		} else {
			sellSystems = s.SDE.Universe.SystemsWithinRadius(params.CurrentSystemID, params.SellRadius)
		}
		sellSystems = filterSystemDistanceMap(sellSystems, ignored)
		if len(sellSystems) == 0 {
			emitProgress("No sell systems remain after applying ignored systems filter.")
			return []ContractResult{}, nil
		}
		sellRegions = s.SDE.Universe.RegionsInSet(sellSystems)
	}

	log.Printf("[DEBUG] ScanContracts: buySystems=%d, buyRegions=%d, minPrice=%.0f, maxMargin=%.1f",
		len(buySystems), len(buyRegions), minContractPrice, maxContractMargin)

	// Fetch market orders and contracts in parallel
	var sellOrders []esi.MarketOrder
	var buyOrdersForLiquidation []esi.MarketOrder
	var allContracts []esi.PublicContract
	var contractsMu sync.Mutex
	var wg sync.WaitGroup
	var failedContractRegions int32

	emitProgress(fmt.Sprintf("Fetching market orders + contracts from %d regions...", len(buyRegions)))

	wg.Add(2)
	go func() {
		defer wg.Done()
		sellOrders = s.fetchOrders(buyRegions, "sell", buySystems)
	}()
	if contractInstant {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buyOrdersForLiquidation = s.fetchOrders(sellRegions, "buy", sellSystems)
		}()
	}
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
					atomic.AddInt32(&failedContractRegions, 1)
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
	fetchDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(fetchDone)
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-fetchDone:
	}

	log.Printf("[DEBUG] ScanContracts: %d sell orders, %d contracts total", len(sellOrders), len(allContracts))
	if contractInstant {
		log.Printf("[DEBUG] ScanContracts: instant liquidation enabled, %d buy orders in sell radius", len(buyOrdersForLiquidation))
	}
	if failed := atomic.LoadInt32(&failedContractRegions); failed > 0 {
		emitProgress(fmt.Sprintf("Warning: contracts missing in %d regions due to ESI errors", failed))
	}

	// Build location -> system map from market orders (covers player structures
	// that are not present in SDE.Stations).
	marketLocationSystems := make(map[int64]int32, len(sellOrders))
	for _, o := range sellOrders {
		if o.LocationID == 0 || o.SystemID == 0 {
			continue
		}
		if _, exists := marketLocationSystems[o.LocationID]; !exists {
			marketLocationSystems[o.LocationID] = o.SystemID
		}
	}
	for _, o := range buyOrdersForLiquidation {
		if o.LocationID == 0 || o.SystemID == 0 {
			continue
		}
		if _, exists := marketLocationSystems[o.LocationID]; !exists {
			marketLocationSystems[o.LocationID] = o.SystemID
		}
	}

	// Instant liquidation pricing input: buy-book depth by type and system.
	buyOrdersByTypeBySystem := make(map[int32]map[int32][]esi.MarketOrder)
	if contractInstant {
		for _, o := range buyOrdersForLiquidation {
			if o.SystemID == 0 {
				continue
			}
			bySystem, ok := buyOrdersByTypeBySystem[o.TypeID]
			if !ok {
				bySystem = make(map[int32][]esi.MarketOrder)
				buyOrdersByTypeBySystem[o.TypeID] = bySystem
			}
			bySystem[o.SystemID] = append(bySystem[o.SystemID], o)
		}
	}

	// Build sell orders by type for additionalCost calculation (items buyer must provide).
	// We need sell orders to calculate the cost of BUYING these items.
	sellOrdersByType := make(map[int32][]esi.MarketOrder)
	for _, o := range sellOrders {
		sellOrdersByType[o.TypeID] = append(sellOrdersByType[o.TypeID], o)
	}

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

	// Filter contracts: only item_exchange, not expired, price > threshold, reachable location
	var candidates []esi.PublicContract
	for _, c := range allContracts {
		if err := checkContextCanceled(ctx); err != nil {
			return nil, err
		}
		if c.Type != "item_exchange" {
			continue
		}
		if c.IsExpired() {
			continue
		}
		if c.Price < minContractPrice {
			continue // skip scam/bait contracts with very low prices
		}
		// Pre-filter: skip contracts in unknown or unreachable locations.
		// If we can't map location -> system, we can't verify accessibility.
		sysID := s.locationToSystem(c.StartLocationID, marketLocationSystems)
		if sysID == 0 {
			continue
		}
		if _, ok := buySystems[sysID]; !ok {
			continue // contract station is outside buy radius
		}
		candidates = append(candidates, c)
	}

	log.Printf("[DEBUG] ScanContracts: %d item_exchange candidates after filtering (location + price)", len(candidates))
	emitProgress(fmt.Sprintf("Evaluating %d contracts...", len(candidates)))

	if len(candidates) == 0 {
		return nil, nil
	}

	// Fetch items for all candidates
	contractIDs := make([]int32, len(candidates))
	for i, c := range candidates {
		contractIDs[i] = c.ContractID
	}

	contractItemsCh := make(chan map[int32][]esi.ContractItem, 1)
	go func() {
		contractItemsCh <- s.ESI.FetchContractItemsBatch(contractIDs, s.ContractItemsCache, func(done, total int) {
			emitProgress(fmt.Sprintf("Fetching contract items %d/%d...", done, total))
		})
	}()

	var contractItems map[int32][]esi.ContractItem
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case contractItems = <-contractItemsCh:
	}

	log.Printf("[DEBUG] ScanContracts: fetched items for %d contracts", len(contractItems))
	if missing := len(candidates) - len(contractItems); missing > 0 {
		emitProgress(fmt.Sprintf("Warning: missing contract items for %d contracts; results may be incomplete", missing))
	}

	// Collect unique type IDs that need history lookup (estimate mode only).
	if !contractInstant {
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

		// Fetch market history for pricing validation — prefer trade hub region for VWAP
		primaryRegion := bestHubRegion(buyRegions)

		if s.History != nil && len(typeIDsNeedHistory) > 0 {
			emitProgress(fmt.Sprintf("Fetching market history for %d item types...", len(typeIDsNeedHistory)))
			historyDone := make(chan struct{})
			go func() {
				s.fetchContractItemsHistory(typeIDsNeedHistory, priceData, primaryRegion)
				close(historyDone)
			}()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-historyDone:
			}
		}

		log.Printf("[DEBUG] ScanContracts: enriched %d types with history", len(typeIDsNeedHistory))
	}

	// Calculate profit for each contract.
	sellValueMult := contractSellValueMultiplier(params)
	holdDays := contractHoldDays(params)
	targetConfidence := contractTargetConfidence(params)
	resolvedTypeNames := make(map[int32]string)

	var results []ContractResult

	for _, contract := range candidates {
		if err := checkContextCanceled(ctx); err != nil {
			return nil, err
		}
		items, ok := contractItems[contract.ContractID]
		if !ok || len(items) == 0 {
			continue
		}
		blockedTypeID := blockedContractTypeID(items)
		if blockedTypeID != 0 {
			log.Printf("[DEBUG] Contract %d: skipping - contains market-disabled type %d", contract.ContractID, blockedTypeID)
			continue
		}

		var marketValue float64
		var itemCount int32
		var pricedCount int        // how many item types we could price
		var totalTypes int         // total included item types (non-BPC/BPO/filtered)
		var topItems []string      // for generating title
		var lowVolumeItems int     // items with suspicious low trading volume
		var highDeviationItems int // items where sell price deviates significantly from VWAP
		fullLiquidationProb := 1.0
		maxFillDays := 0.0
		expectedGrossByFill := 0.0
		var additionalCost float64      // cost of items buyer must provide (PLEX, etc.)
		var unpricedAdditionalItems int // items buyer must provide that we couldn't price
		includedQtyByType := make(map[int32]int32)
		additionalQtyByType := make(map[int32]int32)
		liquidationSystemID := int32(0)

		// FIRST PASS: detect ship presence for fitted-risk handling.
		shipSizeClass := 0 // 0=no ship, 1=small, 2=medium, 3=large
		hasHighsecRestrictedShip := false
		for _, item := range items {
			if !item.IsIncluded || item.Quantity <= 0 {
				continue
			}
			if typeInfo, ok := s.SDE.Types[item.TypeID]; ok && typeInfo.CategoryID == 6 { // ships
				sizeClass := getShipSizeClass(typeInfo.GroupID)
				if sizeClass > 0 && sizeClass > shipSizeClass {
					shipSizeClass = sizeClass
				}
				groupName := ""
				if g, ok := s.SDE.Groups[typeInfo.GroupID]; ok {
					groupName = g.Name
				}
				if isHighsecRestrictedShipGroup(typeInfo.GroupID, groupName) {
					hasHighsecRestrictedShip = true
				}
			}
		}

		hasBPO := false

		// SECOND PASS: normalize and aggregate quantities by type to avoid
		// double-counting order-book depth for repeated lines.
		for _, item := range items {
			if item.Quantity <= 0 {
				continue
			}

			// Items buyer must provide on top of ISK price.
			if !item.IsIncluded {
				additionalQtyByType[item.TypeID] += item.Quantity
				continue
			}
			// BPCs have no reliable generic market valuation.
			if item.IsBlueprintCopy {
				continue
			}
			// Damaged items are too uncertain in public ESI context.
			if item.Damage > 0 {
				continue
			}

			typeInfo, hasTypeInfo := s.SDE.Types[item.TypeID]
			if hasTypeInfo {
				nameLower := strings.ToLower(typeInfo.Name)
				// BPOs are excluded: valuation is highly dependent on research state.
				if strings.Contains(nameLower, "blueprint") {
					hasBPO = true
					continue
				}
				// Rig handling (fitted-risk control).
				if typeInfo.IsRig && shouldExcludeRigWithShip(item, typeInfo.Name, shipSizeClass, params.ExcludeRigsWithShip) {
					continue
				}
			}

			includedQtyByType[item.TypeID] += item.Quantity
		}

		totalTypes = len(includedQtyByType)

		// Price additional required items (must be fully priceable to trust total cost).
		for typeID, qty := range additionalQtyByType {
			var itemCost float64
			couldPrice := false

			if contractInstant {
				// Buy required item from sell book.
				book := sellOrdersByType[typeID]
				if len(book) > 0 {
					plan := ComputeExecutionPlan(book, qty, true)
					if plan.CanFill && plan.ExpectedPrice > 0 {
						itemCost = plan.ExpectedPrice * float64(qty)
						couldPrice = true
					}
				}
			} else {
				pd, ok := priceData[typeID]
				if ok && pd.MinSellPrice > 0 && pd.MinSellPrice != math.MaxFloat64 {
					itemCost = pd.MinSellPrice * float64(qty)
					couldPrice = true
				}
			}

			if couldPrice {
				additionalCost += itemCost
			} else {
				unpricedAdditionalItems++
			}
		}

		// Price included items once per type (aggregated quantity).
		instantItems := make([]instantValuationItem, 0, len(includedQtyByType))
		for typeID, qty := range includedQtyByType {
			typeInfo, hasTypeInfo := s.SDE.Types[typeID]
			itemLabel := s.contractItemLabel(typeID, resolvedTypeNames)

			valueFactor := 1.0
			// Conservative haircut: when a ship is present, module value is uncertain
			// because public ESI lacks reliable fitted-state flags for all cases.
			if shipSizeClass > 0 && hasTypeInfo && typeInfo.CategoryID == 7 && !typeInfo.IsRig {
				valueFactor = ContractShipModuleValueFactor
			}

			if contractInstant {
				instantItems = append(instantItems, instantValuationItem{
					TypeID:      typeID,
					Quantity:    qty,
					Label:       itemLabel,
					ValueFactor: valueFactor,
				})
				continue
			}

			pd, ok := priceData[typeID]
			if !ok || pd.MinSellPrice == 0 || pd.MinSellPrice == math.MaxFloat64 {
				continue
			}

			var usePrice float64
			if pd.HasHistory && pd.VWAP > 0 {
				if pd.MinSellPrice < pd.VWAP*0.5 {
					usePrice = math.Min(pd.VWAP*0.7, pd.MinSellPrice*2)
					highDeviationItems++
				} else {
					usePrice = math.Min(pd.VWAP, pd.MinSellPrice)
				}
			} else {
				if params.RequireHistory {
					continue
				}
				usePrice = pd.MinSellPrice
			}

			if pd.DailyVolume < MinDailyVolumeForContract {
				lowVolumeItems++
			}

			pricedCount++
			itemValue := usePrice * float64(qty) * valueFactor
			marketValue += itemValue
			itemCount += qty

			dailyVol := effectiveDailyVolume(pd)
			fillDays := estimateFillDays(qty, dailyVol)
			itemFillProb := fillProbabilityWithinDays(fillDays, float64(holdDays))
			fullLiquidationProb *= itemFillProb
			if math.IsInf(fillDays, 1) {
				if maxFillDays < float64(holdDays)*10 {
					maxFillDays = float64(holdDays) * 10
				}
			} else if fillDays > maxFillDays {
				maxFillDays = fillDays
			}
			expectedGrossByFill += itemValue * itemFillProb

			if qty > 1 {
				topItems = append(topItems, fmt.Sprintf("%dx %s", qty, itemLabel))
			} else {
				topItems = append(topItems, itemLabel)
			}
		}
		if contractInstant {
			var liquidationSystemAllowed func(int32) bool
			if hasHighsecRestrictedShip {
				liquidationSystemAllowed = func(systemID int32) bool {
					if sys, ok := s.SDE.Systems[systemID]; ok {
						return !isHighsecSecurity(sys.Security)
					}
					// Unknown security metadata: keep system eligible.
					return true
				}
			}

			choice, ok := selectInstantLiquidationSystem(instantItems, buyOrdersByTypeBySystem, liquidationSystemAllowed)
			if !ok {
				continue
			}
			liquidationSystemID = choice.SystemID
			// Defensive safety net: restricted capitals must not liquidate in highsec.
			if hasHighsecRestrictedShip {
				if liqSys, ok := s.SDE.Systems[liquidationSystemID]; ok && isHighsecSecurity(liqSys.Security) {
					continue
				}
			}
			marketValue = choice.MarketValue
			pricedCount = choice.PricedCount
			itemCount = choice.ItemCount
			topItems = append(topItems, choice.TopItems...)
		}

		// Skip contracts that are purely BPOs — unreliable market pricing
		if hasBPO && totalTypes == 0 {
			continue
		}
		if unpricedAdditionalItems > 0 {
			log.Printf("[DEBUG] Contract %d: skipping - couldn't price %d additional items (IsIncluded=false)",
				contract.ContractID, unpricedAdditionalItems)
			continue
		}
		if totalTypes == 0 || pricedCount == 0 {
			continue
		}
		if float64(pricedCount)/float64(totalTypes) < minPricedRatio {
			continue
		}
		if contractInstant && pricedCount < totalTypes {
			continue
		}

		// This heuristic is useful only when history is mandatory; otherwise it is too
		// punitive for thin items without history (DailyVolume=0 fallback path).
		if params.RequireHistory && pricedCount > 0 && float64(lowVolumeItems)/float64(pricedCount) > 0.5 {
			continue
		}
		if pricedCount > 0 && float64(highDeviationItems)/float64(pricedCount) > 0.3 {
			continue
		}

		if marketValue <= 0 {
			continue
		}

		totalCost := contract.Price + additionalCost
		effectiveValue := marketValue * sellValueMult
		profit := effectiveValue - totalCost
		if profit <= 0 {
			continue
		}

		margin := safeDiv(profit, totalCost) * 100
		if margin > maxContractMargin {
			continue
		}

		expectedProfit := profit
		expectedMargin := margin
		sellConfidencePct := 100.0
		estLiqDays := 0.0
		conservativeValue := effectiveValue
		carryCost := 0.0

		if !contractInstant {
			sellConfidencePct = fullLiquidationProb * 100
			if sellConfidencePct < targetConfidence {
				continue
			}
			estLiqDays = maxFillDays
			conservativeGross := expectedGrossByFill * (1.0 - ContractConservativePriceHaircut)
			conservativeValue = conservativeGross * sellValueMult
			carryCost = totalCost * ContractDailyCarryRate * contractCarryDays(holdDays, estLiqDays)
			expectedProfit = conservativeValue - totalCost - carryCost
			if expectedProfit <= 0 {
				continue
			}
			expectedMargin = safeDiv(expectedProfit, totalCost) * 100
		}

		if expectedMargin < params.MinMargin {
			continue
		}

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
		sysID := s.locationToSystem(contract.StartLocationID, marketLocationSystems)
		sysName := ""
		regionName := ""
		liquidationSystemName := ""
		liquidationRegionName := ""
		if sysID != 0 {
			sysName = s.systemName(sysID)
			if sys, ok := s.SDE.Systems[sysID]; ok {
				regionName = s.regionName(sys.RegionID)
			}
		}
		if contractInstant && liquidationSystemID != 0 {
			liquidationSystemName = s.systemName(liquidationSystemID)
			if liqSys, ok := s.SDE.Systems[liquidationSystemID]; ok {
				liquidationRegionName = s.regionName(liqSys.RegionID)
			}
		}
		if strings.HasPrefix(stationName, "Location ") || strings.HasPrefix(stationName, "Structure ") {
			if eveName := s.ESI.EVERefStructureName(contract.StartLocationID); eveName != "" {
				stationName = eveName
			} else if sysName != "" {
				stationName = fmt.Sprintf("Structure @ %s", sysName)
			}
		}

		pickupJumps := 0
		if sysID != 0 {
			if d, ok := buySystems[sysID]; ok {
				pickupJumps = d
			} else {
				pickupJumps = s.jumpsBetweenWithSecurity(params.CurrentSystemID, sysID, params.MinRouteSecurity)
			}
		}
		jumps := pickupJumps
		liquidationJumps := 0
		if contractInstant && sysID != 0 && liquidationSystemID != 0 {
			liquidationJumps = s.jumpsBetweenWithSecurity(sysID, liquidationSystemID, params.MinRouteSecurity)
			if liquidationJumps >= UnreachableJumps {
				continue
			}
			jumps += liquidationJumps
		}

		kpiProfit := profit
		if !contractInstant {
			kpiProfit = expectedProfit
		}
		profitPerJump := 0.0
		if jumps > 0 {
			profitPerJump = kpiProfit / float64(jumps)
		}

		results = append(results, ContractResult{
			ContractID:            contract.ContractID,
			Title:                 title,
			Price:                 contract.Price,
			MarketValue:           marketValue,
			Profit:                sanitizeFloat(profit),
			MarginPercent:         sanitizeFloat(margin),
			ExpectedProfit:        sanitizeFloat(expectedProfit),
			ExpectedMarginPercent: sanitizeFloat(expectedMargin),
			SellConfidence:        sanitizeFloat(sellConfidencePct),
			EstLiquidationDays:    sanitizeFloat(estLiqDays),
			ConservativeValue:     sanitizeFloat(conservativeValue),
			CarryCost:             sanitizeFloat(carryCost),
			Volume:                contract.Volume,
			StationName:           stationName,
			SystemName:            sysName,
			RegionName:            regionName,
			LiquidationSystemName: liquidationSystemName,
			LiquidationRegionName: liquidationRegionName,
			ItemCount:             itemCount,
			LiquidationJumps:      liquidationJumps,
			Jumps:                 jumps,
			ProfitPerJump:         sanitizeFloat(profitPerJump),
		})
	}

	log.Printf("[DEBUG] ScanContracts: %d profitable results", len(results))

	// Sort by profit descending, keep top 100
	sort.Slice(results, func(i, j int) bool {
		left := results[i].ExpectedProfit
		if left == 0 {
			left = results[i].Profit
		}
		right := results[j].ExpectedProfit
		if right == 0 {
			right = results[j].Profit
		}
		return left > right
	})
	// Cap to prevent server overload on contract results
	if len(results) > MaxUnlimitedResults {
		results = results[:MaxUnlimitedResults]
	}

	if err := checkContextCanceled(ctx); err != nil {
		return nil, err
	}
	emitProgress(fmt.Sprintf("Found %d profitable contracts", len(results)))
	return results, nil
}

// locationToSystem maps a station/structure ID to its solar system ID.
func (s *Scanner) locationToSystem(locationID int64, marketLocationSystems map[int64]int32) int32 {
	if station, ok := s.SDE.Stations[locationID]; ok {
		return station.SystemID
	}
	if marketLocationSystems != nil {
		if sysID, ok := marketLocationSystems[locationID]; ok {
			return sysID
		}
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

// bestHubRegion picks the highest-priority trade hub region from the set,
// falling back to the lowest numeric ID for determinism.
func bestHubRegion(regions map[int32]bool) int32 {
	best := int32(0)
	bestPri := int(^uint(0) >> 1) // max int
	for rid := range regions {
		if pri, ok := hubRegionPriority[rid]; ok && pri < bestPri {
			best = rid
			bestPri = pri
		} else if best == 0 || (bestPri == int(^uint(0)>>1) && rid < best) {
			best = rid
		}
	}
	return best
}
