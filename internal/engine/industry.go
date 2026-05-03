package engine

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strings"

	"eve-flipper/internal/esi"
	"eve-flipper/internal/sde"
)

// IndustryParams holds parameters for industry analysis.
type IndustryParams struct {
	TypeID              int32   // Target item to analyze
	Runs                int32   // Number of runs (default 1)
	ActivityMode        string  // auto/manufacturing/reaction/invention
	MaterialEfficiency  int32   // Blueprint ME (0-10)
	TimeEfficiency      int32   // Blueprint TE (0-20)
	SystemID            int32   // Manufacturing system
	StationID           int64   // Optional: specific station/structure for price lookup (0 = region-wide)
	FacilityTax         float64 // Facility tax % (default 0)
	StructureBonus      float64 // Structure material bonus % (e.g., 1% for Raitaru)
	BrokerFee           float64 // Broker fee % when buying materials / product (default 0)
	SalesTaxPercent     float64 // Sales tax % when selling product (for display / future use)
	ReprocessingYield   float64 // Reprocessing efficiency (0-1, e.g., 0.50 for 50%)
	IncludeReprocessing bool    // Whether to consider reprocessing ore as alternative
	MaxDepth            int     // Max recursion depth (default 10)
	OwnBlueprint        bool    // true = user owns BP (default), false = must buy
	BlueprintCost       float64 // ISK cost of blueprint (BPO or BPC)
	BlueprintIsBPO      bool    // true = BPO (amortize over runs), false = BPC (one-time)
	InventionChance     float64 // Optional invention chance override in percent (0 = SDE probability)
	DecryptorCost       float64 // Optional per-attempt decryptor cost
	InventionOutputRuns int32   // Optional successful BPC runs override
}

// MaterialNode represents a node in the production tree.
type MaterialNode struct {
	TypeID       int32           `json:"type_id"`
	TypeName     string          `json:"type_name"`
	Quantity     int32           `json:"quantity"`      // Required quantity
	Activity     string          `json:"activity"`      // manufacturing/reaction/base
	Runs         int32           `json:"runs"`          // Blueprint runs needed for this node
	IsBase       bool            `json:"is_base"`       // True if cannot be further produced
	BuyPrice     float64         `json:"buy_price"`     // Market buy price (sell orders)
	MaterialCost float64         `json:"material_cost"` // Sum of chosen child material costs
	BuildCost    float64         `json:"build_cost"`    // Total cost to build (materials + job cost)
	ShouldBuild  bool            `json:"should_build"`  // True if building is cheaper than buying
	JobCost      float64         `json:"job_cost"`      // Manufacturing job installation cost
	Children     []*MaterialNode `json:"children"`      // Required sub-materials
	Blueprint    *BlueprintInfo  `json:"blueprint"`     // Blueprint info if buildable
	Depth        int             `json:"depth"`         // Depth in tree
}

// BlueprintInfo contains blueprint information for display.
type BlueprintInfo struct {
	BlueprintTypeID int32   `json:"blueprint_type_id"`
	ProductQuantity int32   `json:"product_quantity"`
	ME              int32   `json:"me"`
	TE              int32   `json:"te"`
	Time            int32   `json:"time"` // Manufacturing time in seconds
	Activity        string  `json:"activity"`
	Probability     float64 `json:"probability,omitempty"`
}

// IndustryActivityStep is one executable activity in the industry plan.
type IndustryActivityStep struct {
	Activity         string  `json:"activity"`
	BlueprintTypeID  int32   `json:"blueprint_type_id"`
	BlueprintName    string  `json:"blueprint_name"`
	ProductTypeID    int32   `json:"product_type_id"`
	ProductName      string  `json:"product_name"`
	Runs             float64 `json:"runs"`
	OutputQuantity   int32   `json:"output_quantity"`
	MaterialCost     float64 `json:"material_cost"`
	JobCost          float64 `json:"job_cost"`
	TotalCost        float64 `json:"total_cost"`
	TimeSeconds      int32   `json:"time_seconds"`
	Probability      float64 `json:"probability,omitempty"`
	ExpectedAttempts float64 `json:"expected_attempts,omitempty"`
	Reason           string  `json:"reason,omitempty"`
}

// IndustryAnalysis is the result of analyzing a production chain.
type IndustryAnalysis struct {
	TargetTypeID          int32                  `json:"target_type_id"`
	TargetTypeName        string                 `json:"target_type_name"`
	Runs                  int32                  `json:"runs"`
	TotalQuantity         int32                  `json:"total_quantity"`
	MarketBuyPrice        float64                `json:"market_buy_price"`   // Cost to buy ready product (from sell orders, no broker fee)
	TotalBuildCost        float64                `json:"total_build_cost"`   // Cost to build from scratch
	OptimalBuildCost      float64                `json:"optimal_build_cost"` // Cost with optimal buy/build decisions
	Savings               float64                `json:"savings"`            // MarketBuyPrice - OptimalBuildCost
	SavingsPercent        float64                `json:"savings_percent"`
	SellRevenue           float64                `json:"sell_revenue"`       // Revenue after sales tax + broker fee
	Profit                float64                `json:"profit"`             // SellRevenue - OptimalBuildCost
	ProfitPercent         float64                `json:"profit_percent"`     // Profit / OptimalBuildCost * 100
	MakerSellRevenue      float64                `json:"maker_sell_revenue"` // Listing at visible ask after tax + broker fee
	MakerSellProfit       float64                `json:"maker_sell_profit"`
	InstantSellRevenue    float64                `json:"instant_sell_revenue"` // Selling into visible buy orders after sales tax
	InstantSellProfit     float64                `json:"instant_sell_profit"`
	InstantSellAvailable  bool                   `json:"instant_sell_available"`
	ISKPerHour            float64                `json:"isk_per_hour"`       // Profit / manufacturing hours
	ManufacturingTime     int32                  `json:"manufacturing_time"` // Total time in seconds
	TotalActivityTime     int32                  `json:"total_activity_time"`
	TotalJobCost          float64                `json:"total_job_cost"` // Sum of all job installation costs
	ManufacturingCost     float64                `json:"manufacturing_cost"`
	ReactionCost          float64                `json:"reaction_cost"`
	InventionCost         float64                `json:"invention_cost"`
	InventionJobCost      float64                `json:"invention_job_cost"`
	InventionAttempts     float64                `json:"invention_attempts"`
	InventionProbability  float64                `json:"invention_probability"`
	ActivityMode          string                 `json:"activity_mode"`
	ActivityPlan          []IndustryActivityStep `json:"activity_plan"`
	MaterialTree          *MaterialNode          `json:"material_tree"`
	FlatMaterials         []*FlatMaterial        `json:"flat_materials"` // Flattened list of base materials
	SystemCostIndex       float64                `json:"system_cost_index"`
	RegionID              int32                  `json:"region_id"`               // Market region for execution plan
	RegionName            string                 `json:"region_name"`             // Optional display name
	BlueprintCostIncluded float64                `json:"blueprint_cost_included"` // BP cost added to build cost
}

// FlatMaterial is a simplified material for the shopping list.
type FlatMaterial struct {
	TypeID     int32   `json:"type_id"`
	TypeName   string  `json:"type_name"`
	Quantity   int32   `json:"quantity"`
	UnitPrice  float64 `json:"unit_price"`
	TotalPrice float64 `json:"total_price"`
	Volume     float64 `json:"volume"`
}

// IndustryAnalyzer performs industry calculations.
type IndustryAnalyzer struct {
	SDE                  *sde.Data
	ESI                  *esi.Client
	IndustryCache        *esi.IndustryCache
	adjustedPrices       map[int32]float64
	marketPrices         map[int32]float64 // Best sell order prices
	marketSellOrders     map[int32][]esi.MarketOrder
	marketBuyOrders      map[int32][]esi.MarketOrder
	systemCostIndices    *esi.SystemCostIndices
	getAllAdjustedPrices func(cache *esi.IndustryCache) (map[int32]float64, error)
	getSystemCostIndex   func(cache *esi.IndustryCache, systemID int32) (*esi.SystemCostIndices, error)
	fetchMarketPricesFn  func(params IndustryParams) (map[int32]float64, error)
	fetchMarketBooksFn   func(params IndustryParams) (map[int32][]esi.MarketOrder, map[int32][]esi.MarketOrder, error)
}

// NewIndustryAnalyzer creates a new analyzer.
func NewIndustryAnalyzer(sdeData *sde.Data, esiClient *esi.Client) *IndustryAnalyzer {
	return &IndustryAnalyzer{
		SDE:           sdeData,
		ESI:           esiClient,
		IndustryCache: esi.NewIndustryCache(),
	}
}

func (a *IndustryAnalyzer) ensureIndustryCache() {
	if a.IndustryCache == nil {
		a.IndustryCache = esi.NewIndustryCache()
	}
}

func (a *IndustryAnalyzer) loadAdjustedPrices() (map[int32]float64, error) {
	a.ensureIndustryCache()
	if a.getAllAdjustedPrices != nil {
		return a.getAllAdjustedPrices(a.IndustryCache)
	}
	if a.ESI == nil {
		return nil, fmt.Errorf("esi client unavailable")
	}
	return a.ESI.GetAllAdjustedPrices(a.IndustryCache)
}

func (a *IndustryAnalyzer) loadSystemCostIndex(systemID int32) (*esi.SystemCostIndices, error) {
	a.ensureIndustryCache()
	if a.getSystemCostIndex != nil {
		return a.getSystemCostIndex(a.IndustryCache, systemID)
	}
	if a.ESI == nil {
		return nil, fmt.Errorf("esi client unavailable")
	}
	return a.ESI.GetSystemCostIndex(a.IndustryCache, systemID)
}

func (a *IndustryAnalyzer) loadMarketPrices(params IndustryParams) (map[int32]float64, error) {
	if a.fetchMarketPricesFn != nil {
		return a.fetchMarketPricesFn(params)
	}
	if a.ESI == nil {
		return nil, fmt.Errorf("esi client unavailable")
	}
	return a.fetchMarketPrices(params)
}

func (a *IndustryAnalyzer) loadMarketBooks(params IndustryParams) (map[int32][]esi.MarketOrder, map[int32][]esi.MarketOrder, error) {
	if a.fetchMarketBooksFn != nil {
		return a.fetchMarketBooksFn(params)
	}
	if a.ESI == nil {
		return nil, nil, fmt.Errorf("esi client unavailable")
	}
	return a.fetchMarketBooks(params)
}

// Analyze performs full industry analysis for a given item.
func (a *IndustryAnalyzer) Analyze(params IndustryParams, progress func(string)) (*IndustryAnalysis, error) {
	if params.Runs <= 0 {
		params.Runs = 1
	}
	if params.MaxDepth <= 0 {
		params.MaxDepth = 10
	}
	if params.ReprocessingYield <= 0 {
		params.ReprocessingYield = 0.50 // Default 50%
	}
	params.ActivityMode = normalizeIndustryActivityMode(params.ActivityMode)
	if params.InventionOutputRuns < 0 {
		params.InventionOutputRuns = 0
	}
	if params.DecryptorCost < 0 {
		params.DecryptorCost = 0
	}

	// Get type info
	typeInfo, ok := a.SDE.Types[params.TypeID]
	if !ok {
		return nil, fmt.Errorf("type %d not found", params.TypeID)
	}

	progress("Fetching market prices...")

	// Fetch adjusted prices for job cost calculation
	adjustedPrices, err := a.loadAdjustedPrices()
	if err != nil {
		log.Printf("Warning: failed to fetch adjusted prices: %v", err)
		adjustedPrices = make(map[int32]float64)
	}
	a.adjustedPrices = adjustedPrices

	// Fetch market prices (best sell orders) for buy/build comparison
	progress("Fetching sell order prices...")
	marketPrices, err := a.loadMarketPrices(params)
	if err != nil {
		log.Printf("Warning: failed to fetch market prices: %v", err)
		marketPrices = make(map[int32]float64)
	}
	a.marketPrices = marketPrices
	a.marketSellOrders = nil
	a.marketBuyOrders = nil

	progress("Fetching order book depth...")
	marketSellOrders, marketBuyOrders, err := a.loadMarketBooks(params)
	if err != nil {
		log.Printf("Warning: failed to fetch market order books: %v", err)
	} else {
		a.marketSellOrders = marketSellOrders
		a.marketBuyOrders = marketBuyOrders
	}

	// Get system cost index
	var costIndex float64
	a.systemCostIndices = nil
	if params.SystemID != 0 {
		progress("Fetching system cost index...")
		idx, err := a.loadSystemCostIndex(params.SystemID)
		if err != nil {
			log.Printf("Warning: failed to fetch cost index: %v", err)
		} else {
			a.systemCostIndices = idx
			costIndex = idx.Manufacturing
		}
	}

	progress("Building production tree...")

	// FIX #1: Treat params.Runs as actual blueprint runs.
	// Calculate total items produced: runs × productQuantity.
	totalQuantity := params.Runs
	if bp, ok := a.SDE.Industry.GetBlueprintForProduct(params.TypeID); ok {
		activity := a.activityForProduct(bp, params.TypeID, params.ActivityMode)
		productQty, _ := blueprintProductForActivity(bp, params.TypeID, activity)
		if productQty <= 0 {
			productQty = bp.ProductQuantity
		}
		if productQty <= 0 {
			productQty = 1
		}
		totalQuantity = params.Runs * productQty
	}

	// Build material tree recursively using totalQuantity as desired items
	tree := a.buildMaterialTree(params.TypeID, totalQuantity, params, 0)

	// Calculate costs
	progress("Calculating optimal costs...")
	a.calculateCosts(tree, costIndex, params)
	if params.ActivityMode != "auto" && !tree.IsBase {
		tree.ShouldBuild = true
	}

	// Flatten materials for shopping list
	flatMaterials := a.flattenMaterials(tree)

	// MarketBuyPrice is cost to buy from visible sell-order depth (no broker fee).
	marketBuyPrice := a.marketBuyCost(params.TypeID, totalQuantity)

	optimalCost := tree.BuildCost
	if params.ActivityMode == "auto" && tree.BuyPrice < tree.BuildCost && tree.BuyPrice > 0 {
		optimalCost = tree.BuyPrice
	}

	// Blueprint acquisition cost (user doesn't own it)
	var bpCostIncluded float64
	if !params.OwnBlueprint && params.BlueprintCost > 0 {
		if params.BlueprintIsBPO {
			bpCostIncluded = params.BlueprintCost / float64(params.Runs)
		} else {
			bpCostIncluded = params.BlueprintCost
		}
		optimalCost += bpCostIncluded
	}

	inventionStep, hasInvention := a.calculateInventionStep(params, tree, costIndex)
	var inventionCost, inventionJobCost, inventionAttempts, inventionProbability float64
	if hasInvention {
		inventionCost = inventionStep.TotalCost
		inventionJobCost = inventionStep.JobCost
		inventionAttempts = inventionStep.ExpectedAttempts
		inventionProbability = inventionStep.Probability
		optimalCost += inventionCost
	}

	savings := marketBuyPrice - optimalCost
	savingsPercent := 0.0
	if marketBuyPrice > 0 {
		savingsPercent = savings / marketBuyPrice * 100
	}

	// FIX #6: Calculate profit if you sell the built product.
	// Revenue = sell price × quantity × (1 - salesTax%) × (1 - brokerFee%)
	makerSellRevenue := a.marketBestAsk(params.TypeID) * float64(totalQuantity) *
		(1.0 - params.SalesTaxPercent/100) *
		(1.0 - params.BrokerFee/100)
	instantSellRevenue, instantSellAvailable := a.marketInstantSellRevenue(
		params.TypeID,
		totalQuantity,
		1.0-params.SalesTaxPercent/100,
	)
	sellRevenue := makerSellRevenue
	if instantSellAvailable {
		sellRevenue = instantSellRevenue
	}
	profit := sellRevenue - optimalCost
	profitPercent := 0.0
	if optimalCost > 0 {
		profitPercent = profit / optimalCost * 100
	}

	// Manufacturing time for ISK/hour
	var mfgTime int32
	if tree.Blueprint != nil {
		mfgTime = tree.Blueprint.Time
	}
	iskPerHour := 0.0
	if mfgTime > 0 {
		iskPerHour = profit / (float64(mfgTime) / 3600.0)
	}

	totalJobCost := a.sumJobCosts(tree)
	if hasInvention {
		totalJobCost += inventionJobCost
	}
	activityPlan := a.buildActivityPlan(tree)
	if hasInvention {
		activityPlan = append([]IndustryActivityStep{inventionStep}, activityPlan...)
	}
	manufacturingCost, reactionCost := sumActivityPlanCosts(activityPlan)
	totalActivityTime := sumActivityPlanTime(activityPlan)
	if totalActivityTime == 0 {
		totalActivityTime = mfgTime
	}
	if totalActivityTime > 0 {
		iskPerHour = profit / (float64(totalActivityTime) / 3600.0)
	}

	regionID, regionName := a.resolveMarketRegion(params)

	return &IndustryAnalysis{
		TargetTypeID:          params.TypeID,
		TargetTypeName:        typeInfo.Name,
		Runs:                  params.Runs,
		TotalQuantity:         totalQuantity,
		MarketBuyPrice:        marketBuyPrice,
		TotalBuildCost:        tree.BuildCost,
		OptimalBuildCost:      optimalCost,
		Savings:               savings,
		SavingsPercent:        savingsPercent,
		SellRevenue:           sellRevenue,
		Profit:                profit,
		ProfitPercent:         profitPercent,
		MakerSellRevenue:      makerSellRevenue,
		MakerSellProfit:       makerSellRevenue - optimalCost,
		InstantSellRevenue:    instantSellRevenue,
		InstantSellProfit:     instantSellRevenue - optimalCost,
		InstantSellAvailable:  instantSellAvailable,
		ISKPerHour:            iskPerHour,
		ManufacturingTime:     totalActivityTime,
		TotalActivityTime:     totalActivityTime,
		TotalJobCost:          totalJobCost,
		ManufacturingCost:     manufacturingCost,
		ReactionCost:          reactionCost,
		InventionCost:         inventionCost,
		InventionJobCost:      inventionJobCost,
		InventionAttempts:     inventionAttempts,
		InventionProbability:  inventionProbability,
		ActivityMode:          params.ActivityMode,
		ActivityPlan:          activityPlan,
		MaterialTree:          tree,
		FlatMaterials:         flatMaterials,
		SystemCostIndex:       costIndex,
		RegionID:              regionID,
		RegionName:            regionName,
		BlueprintCostIncluded: bpCostIncluded,
	}, nil
}

// buildMaterialTree recursively builds the material tree.
func (a *IndustryAnalyzer) buildMaterialTree(typeID int32, quantity int32, params IndustryParams, depth int) *MaterialNode {
	typeName := ""
	if t, ok := a.SDE.Types[typeID]; ok {
		typeName = t.Name
	}

	node := &MaterialNode{
		TypeID:   typeID,
		TypeName: typeName,
		Quantity: quantity,
		Depth:    depth,
		BuyPrice: a.marketBuyCost(typeID, quantity),
		Activity: "base",
	}

	// Check if we can build this item
	bp, hasBP := a.SDE.Industry.GetBlueprintForProduct(typeID)
	if !hasBP || depth >= params.MaxDepth {
		node.IsBase = true
		return node
	}
	activity := a.activityForProduct(bp, typeID, params.ActivityMode)
	if activity == "" {
		node.IsBase = true
		return node
	}
	productQuantity, probability := blueprintProductForActivity(bp, typeID, activity)
	if productQuantity <= 0 {
		productQuantity = 1
	}

	// Calculate how many runs we need
	runsNeeded := quantity / productQuantity
	if quantity%productQuantity != 0 {
		runsNeeded++
	}
	node.Activity = activity
	node.Runs = runsNeeded

	node.Blueprint = &BlueprintInfo{
		BlueprintTypeID: bp.BlueprintTypeID,
		ProductQuantity: productQuantity,
		ME:              params.MaterialEfficiency,
		TE:              params.TimeEfficiency,
		Time:            calculateActivityTime(bp, activity, runsNeeded, params.TimeEfficiency),
		Activity:        activity,
		Probability:     probability,
	}

	// FIX #5: Apply ME and structure bonus in a single step before ceiling
	// to avoid rounding errors from intermediate truncation.
	// EVE formula: max(runs, ceil(base × runs × (1-ME/100) × (1-structureBonus/100)))
	materials := calculateActivityMaterials(bp, activity, runsNeeded, params.MaterialEfficiency, params.StructureBonus)

	// Build children recursively
	for _, mat := range materials {
		child := a.buildMaterialTree(mat.TypeID, mat.Quantity, params, depth+1)
		node.Children = append(node.Children, child)
	}

	return node
}

// calculateCosts calculates build costs bottom-up and decides buy vs build.
func (a *IndustryAnalyzer) calculateCosts(node *MaterialNode, costIndex float64, params IndustryParams) {
	// First, calculate costs for all children
	for _, child := range node.Children {
		a.calculateCosts(child, costIndex, params)
	}

	if node.IsBase {
		// Base material - can only buy
		node.BuildCost = node.BuyPrice
		node.ShouldBuild = false
		return
	}

	// Calculate material cost (sum of optimal costs for children)
	var materialCost float64
	for _, child := range node.Children {
		if child.ShouldBuild {
			materialCost += child.BuildCost
		} else {
			materialCost += child.BuyPrice
		}
	}
	node.MaterialCost = materialCost

	// Calculate job installation cost
	// Formula: EIV * cost_index * (1 + facility_tax)
	eiv := a.calculateEIV(node)
	node.JobCost = eiv * a.costIndexForActivity(node.Activity, costIndex) * (1 + params.FacilityTax/100)

	node.BuildCost = materialCost + node.JobCost

	// Decide: buy or build
	if node.BuyPrice > 0 && node.BuyPrice < node.BuildCost {
		node.ShouldBuild = false
	} else {
		node.ShouldBuild = true
	}
}

// calculateEIV calculates Estimated Item Value for job cost.
// FIX #2: EVE uses BASE material quantities (before ME) for EIV, not ME-reduced.
// Formula: EIV = sum(adjusted_price × base_quantity × runs)
func (a *IndustryAnalyzer) calculateEIV(node *MaterialNode) float64 {
	bp, ok := a.SDE.Industry.GetBlueprintForProduct(node.TypeID)
	if !ok || bp == nil {
		return 0
	}
	activity := node.Activity
	if activity == "" {
		activity = a.activityForProduct(bp, node.TypeID, "")
	}
	productQuantity, _ := blueprintProductForActivity(bp, node.TypeID, activity)
	if productQuantity <= 0 {
		productQuantity = 1
	}

	// Calculate actual blueprint runs for this node
	runsNeeded := node.Quantity / productQuantity
	if node.Quantity%productQuantity != 0 {
		runsNeeded++
	}

	var eiv float64
	for _, mat := range activityMaterials(bp, activity) {
		price := a.adjustedPrices[mat.TypeID]
		// Use base_quantity × runs (NOT ME-adjusted quantities)
		eiv += price * float64(mat.Quantity) * float64(runsNeeded)
	}
	return eiv
}

func normalizeIndustryActivityMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "manufacturing", "reaction", "invention":
		return strings.ToLower(strings.TrimSpace(mode))
	default:
		return "auto"
	}
}

func (a *IndustryAnalyzer) activityForProduct(bp *sde.Blueprint, productTypeID int32, preferred string) string {
	if bp == nil {
		return ""
	}
	preferred = normalizeIndustryActivityMode(preferred)
	if (preferred == "auto" || preferred == "manufacturing") && bp.ProductTypeID == productTypeID && len(bp.Materials) > 0 {
		return "manufacturing"
	}
	if preferred != "auto" && preferred != "invention" {
		if activityProduces(bp, preferred, productTypeID) {
			return preferred
		}
	}
	if activityProduces(bp, "manufacturing", productTypeID) {
		return "manufacturing"
	}
	if activityProduces(bp, "reaction", productTypeID) {
		return "reaction"
	}
	if preferred != "auto" && preferred != "invention" {
		if _, ok := bp.Activities[preferred]; ok {
			return preferred
		}
	}
	return ""
}

func activityProduces(bp *sde.Blueprint, activity string, productTypeID int32) bool {
	if bp == nil {
		return false
	}
	act := bp.Activities[activity]
	if act == nil {
		return false
	}
	for _, product := range act.Products {
		if product.TypeID == productTypeID {
			return true
		}
	}
	return false
}

func blueprintProductForActivity(bp *sde.Blueprint, productTypeID int32, activity string) (int32, float64) {
	if bp == nil {
		return 0, 0
	}
	act := bp.Activities[activity]
	if act == nil {
		return bp.ProductQuantity, 0
	}
	for _, product := range act.Products {
		if product.TypeID == productTypeID {
			return product.Quantity, normalizeProbability(product.Probability)
		}
	}
	if len(act.Products) > 0 {
		return act.Products[0].Quantity, normalizeProbability(act.Products[0].Probability)
	}
	return bp.ProductQuantity, 0
}

func activityMaterials(bp *sde.Blueprint, activity string) []sde.BlueprintMaterial {
	if bp == nil {
		return nil
	}
	if act := bp.Activities[activity]; act != nil {
		return act.Materials
	}
	return bp.Materials
}

func calculateActivityMaterials(bp *sde.Blueprint, activity string, runs, me int32, structureBonus float64) []sde.BlueprintMaterial {
	materials := activityMaterials(bp, activity)
	if len(materials) == 0 || runs <= 0 {
		return nil
	}
	result := make([]sde.BlueprintMaterial, 0, len(materials))
	switch activity {
	case "manufacturing":
		if me < 0 {
			me = 0
		}
		if me > 10 {
			me = 10
		}
		if structureBonus < 0 {
			structureBonus = 0
		}
		meMultiplier := 1.0 - float64(me)/100.0
		structureMultiplier := 1.0 - structureBonus/100.0
		for _, mat := range materials {
			qty := int32(math.Ceil(float64(mat.Quantity) * float64(runs) * meMultiplier * structureMultiplier))
			if qty < runs {
				qty = runs
			}
			result = append(result, sde.BlueprintMaterial{TypeID: mat.TypeID, Quantity: qty})
		}
	default:
		for _, mat := range materials {
			result = append(result, sde.BlueprintMaterial{TypeID: mat.TypeID, Quantity: mat.Quantity * runs})
		}
	}
	return result
}

func calculateActivityTime(bp *sde.Blueprint, activity string, runs, te int32) int32 {
	if bp == nil || runs <= 0 {
		return 0
	}
	baseTime := bp.Time
	if act := bp.Activities[activity]; act != nil && act.Time > 0 {
		baseTime = act.Time
	}
	if activity == "manufacturing" {
		if te < 0 {
			te = 0
		}
		if te > 20 {
			te = 20
		}
		return int32(float64(baseTime) * float64(runs) * (1.0 - float64(te)/100.0))
	}
	return baseTime * runs
}

func normalizeProbability(probability float64) float64 {
	if probability <= 0 {
		return 0
	}
	if probability > 1 {
		probability /= 100
	}
	if probability > 1 {
		return 1
	}
	return probability
}

func (a *IndustryAnalyzer) costIndexForActivity(activity string, fallback float64) float64 {
	if a.systemCostIndices == nil {
		return fallback
	}
	switch activity {
	case "reaction":
		if a.systemCostIndices.Reaction > 0 {
			return a.systemCostIndices.Reaction
		}
	case "invention":
		if a.systemCostIndices.Invention > 0 {
			return a.systemCostIndices.Invention
		}
	default:
		if a.systemCostIndices.Manufacturing > 0 {
			return a.systemCostIndices.Manufacturing
		}
	}
	return fallback
}

func (a *IndustryAnalyzer) calculateInventionStep(params IndustryParams, tree *MaterialNode, fallbackCostIndex float64) (IndustryActivityStep, bool) {
	if params.ActivityMode != "invention" || tree == nil || tree.Blueprint == nil {
		return IndustryActivityStep{}, false
	}
	sourceBP, product, ok := a.findInventionForBlueprint(tree.Blueprint.BlueprintTypeID)
	if !ok || sourceBP == nil || product.TypeID == 0 {
		return IndustryActivityStep{}, false
	}
	chance := normalizeProbability(product.Probability)
	if params.InventionChance > 0 {
		chance = normalizeProbability(params.InventionChance)
	}
	if chance <= 0 {
		return IndustryActivityStep{}, false
	}
	outputRuns := product.Quantity
	if params.InventionOutputRuns > 0 {
		outputRuns = params.InventionOutputRuns
	}
	if outputRuns <= 0 {
		outputRuns = 1
	}
	successesNeeded := math.Ceil(float64(params.Runs) / float64(outputRuns))
	if successesNeeded < 1 {
		successesNeeded = 1
	}
	expectedAttempts := successesNeeded / chance
	attemptMaterials := calculateActivityMaterials(sourceBP, "invention", 1, 0, 0)
	materialCostPerAttempt := 0.0
	eivPerAttempt := 0.0
	for _, mat := range attemptMaterials {
		materialCostPerAttempt += a.marketBuyCost(mat.TypeID, mat.Quantity)
		eivPerAttempt += a.adjustedPrices[mat.TypeID] * float64(mat.Quantity)
	}
	jobCostPerAttempt := eivPerAttempt * a.costIndexForActivity("invention", fallbackCostIndex) * (1 + params.FacilityTax/100)
	totalPerAttempt := materialCostPerAttempt + jobCostPerAttempt + params.DecryptorCost
	step := IndustryActivityStep{
		Activity:         "invention",
		BlueprintTypeID:  sourceBP.BlueprintTypeID,
		BlueprintName:    a.typeName(sourceBP.BlueprintTypeID),
		ProductTypeID:    product.TypeID,
		ProductName:      a.typeName(product.TypeID),
		Runs:             expectedAttempts,
		OutputQuantity:   int32(math.Ceil(successesNeeded)) * outputRuns,
		MaterialCost:     materialCostPerAttempt * expectedAttempts,
		JobCost:          jobCostPerAttempt * expectedAttempts,
		TotalCost:        totalPerAttempt * expectedAttempts,
		TimeSeconds:      int32(math.Ceil(float64(calculateActivityTime(sourceBP, "invention", 1, 0)) * expectedAttempts)),
		Probability:      chance,
		ExpectedAttempts: expectedAttempts,
		Reason:           "expected_bpc_cost",
	}
	return step, true
}

func (a *IndustryAnalyzer) findInventionForBlueprint(blueprintTypeID int32) (*sde.Blueprint, sde.BlueprintProduct, bool) {
	if a == nil || a.SDE == nil || a.SDE.Industry == nil {
		return nil, sde.BlueprintProduct{}, false
	}
	for _, bp := range a.SDE.Industry.Blueprints {
		act := bp.Activities["invention"]
		if act == nil {
			continue
		}
		for _, product := range act.Products {
			if product.TypeID == blueprintTypeID {
				return bp, product, true
			}
		}
	}
	return nil, sde.BlueprintProduct{}, false
}

func (a *IndustryAnalyzer) buildActivityPlan(root *MaterialNode) []IndustryActivityStep {
	var out []IndustryActivityStep
	var walk func(*MaterialNode)
	walk = func(node *MaterialNode) {
		if node == nil {
			return
		}
		for _, child := range node.Children {
			walk(child)
		}
		if node.IsBase || !node.ShouldBuild || node.Blueprint == nil {
			return
		}
		out = append(out, IndustryActivityStep{
			Activity:        node.Activity,
			BlueprintTypeID: node.Blueprint.BlueprintTypeID,
			BlueprintName:   a.typeName(node.Blueprint.BlueprintTypeID),
			ProductTypeID:   node.TypeID,
			ProductName:     node.TypeName,
			Runs:            float64(node.Runs),
			OutputQuantity:  node.Quantity,
			MaterialCost:    node.MaterialCost,
			JobCost:         node.JobCost,
			TotalCost:       node.BuildCost,
			TimeSeconds:     node.Blueprint.Time,
			Probability:     node.Blueprint.Probability,
		})
	}
	walk(root)
	return out
}

func sumActivityPlanCosts(steps []IndustryActivityStep) (manufacturingCost, reactionCost float64) {
	for _, step := range steps {
		switch step.Activity {
		case "reaction":
			reactionCost += step.TotalCost
		case "manufacturing":
			manufacturingCost += step.TotalCost
		}
	}
	return manufacturingCost, reactionCost
}

func sumActivityPlanTime(steps []IndustryActivityStep) int32 {
	var total int64
	for _, step := range steps {
		if step.TimeSeconds > 0 {
			total += int64(step.TimeSeconds)
		}
	}
	const maxInt32 = int64(1<<31 - 1)
	if total > maxInt32 {
		return int32(maxInt32)
	}
	return int32(total)
}

func (a *IndustryAnalyzer) typeName(typeID int32) string {
	if a != nil && a.SDE != nil {
		if t, ok := a.SDE.Types[typeID]; ok {
			return t.Name
		}
	}
	return fmt.Sprintf("Type %d", typeID)
}

// flattenMaterials creates a shopping list of base materials.
func (a *IndustryAnalyzer) flattenMaterials(root *MaterialNode) []*FlatMaterial {
	materialMap := make(map[int32]*FlatMaterial)
	a.collectBaseMaterials(root, materialMap)

	// Convert to slice and sort by total price
	result := make([]*FlatMaterial, 0, len(materialMap))
	for _, m := range materialMap {
		result = append(result, m)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].TotalPrice > result[j].TotalPrice
	})

	return result
}

// collectBaseMaterials recursively collects materials that should be bought.
func (a *IndustryAnalyzer) collectBaseMaterials(node *MaterialNode, materials map[int32]*FlatMaterial) {
	// If we should buy this node (not build), add it to the list (node.BuyPrice already includes broker)
	if !node.ShouldBuild || node.IsBase {
		if existing, ok := materials[node.TypeID]; ok {
			existing.Quantity += node.Quantity
			existing.TotalPrice += node.BuyPrice
			existing.UnitPrice = existing.TotalPrice / float64(existing.Quantity)
		} else {
			volume := 0.0
			if t, ok := a.SDE.Types[node.TypeID]; ok {
				volume = t.Volume
			}
			materials[node.TypeID] = &FlatMaterial{
				TypeID:     node.TypeID,
				TypeName:   node.TypeName,
				Quantity:   node.Quantity,
				UnitPrice:  node.BuyPrice / float64(node.Quantity),
				TotalPrice: node.BuyPrice,
				Volume:     volume * float64(node.Quantity),
			}
		}
		return
	}

	// Otherwise, recurse into children
	for _, child := range node.Children {
		a.collectBaseMaterials(child, materials)
	}
}

// sumJobCosts calculates total job costs for all building steps.
func (a *IndustryAnalyzer) sumJobCosts(node *MaterialNode) float64 {
	total := 0.0
	if node.ShouldBuild && !node.IsBase {
		total += node.JobCost
	}
	for _, child := range node.Children {
		total += a.sumJobCosts(child)
	}
	return total
}

// resolveMarketRegion chooses the market region for pricing.
// Priority: selected manufacturing system -> selected NPC station -> The Forge (Jita).
func (a *IndustryAnalyzer) resolveMarketRegion(params IndustryParams) (int32, string) {
	// Default: The Forge (Jita)
	regionID := int32(10000002)
	regionName := ""

	// Prefer explicit manufacturing system.
	if params.SystemID != 0 {
		if sys, ok := a.SDE.Systems[params.SystemID]; ok && sys.RegionID != 0 {
			regionID = sys.RegionID
		}
	} else if params.StationID != 0 {
		// Fallback: infer region from NPC station metadata in SDE.
		// Player structures are not present in SDE station map.
		if st, ok := a.SDE.Stations[params.StationID]; ok {
			if sys, ok := a.SDE.Systems[st.SystemID]; ok && sys.RegionID != 0 {
				regionID = sys.RegionID
			}
		}
	}

	if r, ok := a.SDE.Regions[regionID]; ok {
		regionName = r.Name
	}
	return regionID, regionName
}

func mergeMarketPrices(regionPrices, stationPrices map[int32]float64) map[int32]float64 {
	out := make(map[int32]float64, len(regionPrices)+len(stationPrices))
	for typeID, price := range regionPrices {
		out[typeID] = price
	}
	for typeID, price := range stationPrices {
		// Station-specific price wins when available.
		out[typeID] = price
	}
	return out
}

func groupIndustryOrdersByType(orders []esi.MarketOrder, locationID int64, isBuy bool) map[int32][]esi.MarketOrder {
	out := make(map[int32][]esi.MarketOrder)
	for _, o := range orders {
		if locationID != 0 && o.LocationID != locationID {
			continue
		}
		if o.VolumeRemain <= 0 || o.Price <= 0 {
			continue
		}
		o.IsBuyOrder = isBuy
		out[o.TypeID] = append(out[o.TypeID], o)
	}
	return out
}

func (a *IndustryAnalyzer) fetchMarketBooks(params IndustryParams) (map[int32][]esi.MarketOrder, map[int32][]esi.MarketOrder, error) {
	regionID, _ := a.resolveMarketRegion(params)

	sellOrders, err := a.ESI.FetchRegionOrders(regionID, "sell")
	if err != nil {
		return nil, nil, err
	}
	buyOrders, err := a.ESI.FetchRegionOrders(regionID, "buy")
	if err != nil {
		return nil, nil, err
	}

	return groupIndustryOrdersByType(sellOrders, params.StationID, false),
		groupIndustryOrdersByType(buyOrders, params.StationID, true),
		nil
}

func (a *IndustryAnalyzer) marketBestAsk(typeID int32) float64 {
	if price := a.marketPrices[typeID]; price > 0 {
		return price
	}
	orders := a.marketSellOrders[typeID]
	best := 0.0
	for _, o := range orders {
		if o.Price <= 0 || o.VolumeRemain <= 0 {
			continue
		}
		if best == 0 || o.Price < best {
			best = o.Price
		}
	}
	return best
}

func (a *IndustryAnalyzer) marketBuyCost(typeID int32, quantity int32) float64 {
	if quantity <= 0 {
		return 0
	}
	if orders := a.marketSellOrders[typeID]; len(orders) > 0 {
		plan := ComputeExecutionPlan(orders, quantity, true)
		if plan.CanFill && plan.TotalCost > 0 && !math.IsNaN(plan.TotalCost) && !math.IsInf(plan.TotalCost, 0) {
			return plan.TotalCost
		}
	}
	return a.marketBestAsk(typeID) * float64(quantity)
}

func (a *IndustryAnalyzer) marketInstantSellRevenue(typeID int32, quantity int32, revenueMult float64) (float64, bool) {
	if quantity <= 0 || revenueMult <= 0 {
		return 0, false
	}
	orders := a.marketBuyOrders[typeID]
	if len(orders) == 0 {
		return 0, false
	}
	plan := ComputeExecutionPlan(orders, quantity, false)
	if !plan.CanFill || plan.TotalCost <= 0 || math.IsNaN(plan.TotalCost) || math.IsInf(plan.TotalCost, 0) {
		return 0, false
	}
	return plan.TotalCost * revenueMult, true
}

// fetchMarketPrices fetches best sell order prices for materials.
// If StationID is provided, station-specific prices are used with per-item fallback
// to regional prices so missing station liquidity doesn't zero out pricing.
func (a *IndustryAnalyzer) fetchMarketPrices(params IndustryParams) (map[int32]float64, error) {
	regionID, _ := a.resolveMarketRegion(params)

	regionPrices, err := a.ESI.GetCachedMarketPrices(a.IndustryCache, regionID)
	if err != nil {
		return nil, err
	}

	if params.StationID == 0 {
		return regionPrices, nil
	}

	stationPrices, err := a.ESI.GetCachedMarketPricesByLocation(a.IndustryCache, regionID, params.StationID)
	if err != nil {
		// Graceful fallback: station-level fetch failed, keep regional pricing.
		log.Printf("Warning: failed to fetch station prices for location %d in region %d: %v",
			params.StationID, regionID, err)
		return regionPrices, nil
	}
	if len(stationPrices) == 0 {
		// No visible liquidity on selected station/structure; use region fallback.
		return regionPrices, nil
	}

	return mergeMarketPrices(regionPrices, stationPrices), nil
}

// GetBlueprintInfo returns blueprint information for a type.
func (a *IndustryAnalyzer) GetBlueprintInfo(typeID int32) (*sde.Blueprint, bool) {
	return a.SDE.Industry.GetBlueprintForProduct(typeID)
}

// SearchResult holds a search result with relevance score.
type SearchResult struct {
	TypeID       int32  `json:"type_id"`
	TypeName     string `json:"type_name"`
	HasBlueprint bool   `json:"has_blueprint"`
	relevance    int    // 0 = exact, 1 = starts with, 2 = contains
}

// SearchBuildableItems returns items matching the query.
// Searches all market items and indicates if they have a blueprint.
// Results are sorted by relevance: exact match > starts with > contains.
func (a *IndustryAnalyzer) SearchBuildableItems(query string, limit int) []SearchResult {
	if limit <= 0 {
		limit = 20
	}

	queryLower := strings.ToLower(strings.TrimSpace(query))
	if queryLower == "" {
		return []SearchResult{}
	}

	var results []SearchResult

	// Search ALL types (not just those with blueprints)
	for typeID, t := range a.SDE.Types {
		nameLower := strings.ToLower(t.Name)

		// Check for match and determine relevance
		var relevance int
		if nameLower == queryLower {
			relevance = 0 // Exact match - highest priority
		} else if strings.HasPrefix(nameLower, queryLower) {
			relevance = 1 // Starts with - high priority
		} else if strings.Contains(nameLower, queryLower) {
			relevance = 2 // Contains - normal priority
		} else {
			continue // No match
		}

		// Check if this item has a blueprint (safely)
		hasBlueprint := false
		if a.SDE.Industry != nil {
			_, hasBlueprint = a.SDE.Industry.ProductToBlueprint[typeID]
		}

		results = append(results, SearchResult{
			TypeID:       typeID,
			TypeName:     t.Name,
			HasBlueprint: hasBlueprint,
			relevance:    relevance,
		})
	}

	// Sort: items with blueprints first, then by relevance, then alphabetically
	sort.Slice(results, func(i, j int) bool {
		// Prioritize items with blueprints
		if results[i].HasBlueprint != results[j].HasBlueprint {
			return results[i].HasBlueprint
		}
		if results[i].relevance != results[j].relevance {
			return results[i].relevance < results[j].relevance
		}
		return results[i].TypeName < results[j].TypeName
	})

	// Limit results
	if len(results) > limit {
		results = results[:limit]
	}

	return results
}
