package engine

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"eve-flipper/internal/esi"
	"eve-flipper/internal/sde"
)

// IndustryParams holds parameters for industry analysis.
type IndustryParams struct {
	TypeID              int32   // Target item to analyze
	Runs                int32   // Number of runs (default 1)
	MaterialEfficiency  int32   // Blueprint ME (0-10)
	TimeEfficiency      int32   // Blueprint TE (0-20)
	SystemID            int32   // Manufacturing system
	FacilityTax         float64 // Facility tax % (default 0)
	StructureBonus      float64 // Structure material bonus % (e.g., 1% for Raitaru)
	BrokerFee           float64 // Broker fee % when buying materials / product (default 0)
	SalesTaxPercent     float64 // Sales tax % when selling product (for display / future use)
	ReprocessingYield   float64 // Reprocessing efficiency (0-1, e.g., 0.50 for 50%)
	IncludeReprocessing bool    // Whether to consider reprocessing ore as alternative
	MaxDepth            int     // Max recursion depth (default 10)
}

// MaterialNode represents a node in the production tree.
type MaterialNode struct {
	TypeID       int32            `json:"type_id"`
	TypeName     string           `json:"type_name"`
	Quantity     int32            `json:"quantity"`      // Required quantity
	IsBase       bool             `json:"is_base"`       // True if cannot be further produced
	BuyPrice     float64          `json:"buy_price"`     // Market buy price (sell orders)
	BuildCost    float64          `json:"build_cost"`    // Total cost to build (materials + job cost)
	ShouldBuild  bool             `json:"should_build"`  // True if building is cheaper than buying
	JobCost      float64          `json:"job_cost"`      // Manufacturing job installation cost
	Children     []*MaterialNode  `json:"children"`      // Required sub-materials
	Blueprint    *BlueprintInfo   `json:"blueprint"`     // Blueprint info if buildable
	Depth        int              `json:"depth"`         // Depth in tree
}

// BlueprintInfo contains blueprint information for display.
type BlueprintInfo struct {
	BlueprintTypeID int32 `json:"blueprint_type_id"`
	ProductQuantity int32 `json:"product_quantity"`
	ME              int32 `json:"me"`
	TE              int32 `json:"te"`
	Time            int32 `json:"time"` // Manufacturing time in seconds
}

// IndustryAnalysis is the result of analyzing a production chain.
type IndustryAnalysis struct {
	TargetTypeID       int32          `json:"target_type_id"`
	TargetTypeName     string         `json:"target_type_name"`
	Runs               int32          `json:"runs"`
	TotalQuantity      int32          `json:"total_quantity"`
	MarketBuyPrice     float64        `json:"market_buy_price"`     // Cost to buy ready product
	TotalBuildCost     float64        `json:"total_build_cost"`     // Cost to build from scratch
	OptimalBuildCost   float64        `json:"optimal_build_cost"`   // Cost with optimal buy/build decisions
	Savings            float64        `json:"savings"`              // MarketBuyPrice - OptimalBuildCost
	SavingsPercent     float64        `json:"savings_percent"`
	TotalJobCost       float64        `json:"total_job_cost"`       // Sum of all job installation costs
	MaterialTree       *MaterialNode  `json:"material_tree"`
	FlatMaterials      []*FlatMaterial `json:"flat_materials"`      // Flattened list of base materials
	SystemCostIndex    float64        `json:"system_cost_index"`
	RegionID           int32          `json:"region_id"`   // Market region for execution plan
	RegionName         string         `json:"region_name"`  // Optional display name
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
	SDE             *sde.Data
	ESI             *esi.Client
	IndustryCache   *esi.IndustryCache
	adjustedPrices  map[int32]float64
	marketPrices    map[int32]float64 // Best sell order prices
}

// NewIndustryAnalyzer creates a new analyzer.
func NewIndustryAnalyzer(sdeData *sde.Data, esiClient *esi.Client) *IndustryAnalyzer {
	return &IndustryAnalyzer{
		SDE:           sdeData,
		ESI:           esiClient,
		IndustryCache: esi.NewIndustryCache(),
	}
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

	// Get type info
	typeInfo, ok := a.SDE.Types[params.TypeID]
	if !ok {
		return nil, fmt.Errorf("type %d not found", params.TypeID)
	}

	progress("Fetching market prices...")
	
	// Fetch adjusted prices for job cost calculation
	adjustedPrices, err := a.ESI.GetAllAdjustedPrices(a.IndustryCache)
	if err != nil {
		log.Printf("Warning: failed to fetch adjusted prices: %v", err)
		adjustedPrices = make(map[int32]float64)
	}
	a.adjustedPrices = adjustedPrices

	// Fetch market prices (best sell orders) for buy/build comparison
	progress("Fetching sell order prices...")
	marketPrices, err := a.fetchMarketPrices(params)
	if err != nil {
		log.Printf("Warning: failed to fetch market prices: %v", err)
		marketPrices = make(map[int32]float64)
	}
	a.marketPrices = marketPrices

	// Get system cost index
	var costIndex float64
	if params.SystemID != 0 {
		progress("Fetching system cost index...")
		idx, err := a.ESI.GetSystemCostIndex(a.IndustryCache, params.SystemID)
		if err != nil {
			log.Printf("Warning: failed to fetch cost index: %v", err)
		} else {
			costIndex = idx.Manufacturing
		}
	}

	progress("Building production tree...")
	
	// Build material tree recursively
	tree := a.buildMaterialTree(params.TypeID, params.Runs, params, 0)
	
	// Calculate costs
	progress("Calculating optimal costs...")
	a.calculateCosts(tree, costIndex, params)

	// Flatten materials for shopping list
	flatMaterials := a.flattenMaterials(tree)

	// Calculate totals (cost to buy product on market; include broker fee)
	marketBuyPrice := a.marketPrices[params.TypeID] * float64(params.Runs)
	if bp, ok := a.SDE.Industry.GetBlueprintForProduct(params.TypeID); ok {
		marketBuyPrice *= float64(bp.ProductQuantity)
	}
	if params.BrokerFee > 0 {
		marketBuyPrice *= (1.0 + params.BrokerFee/100)
	}

	optimalCost := tree.BuildCost
	if tree.BuyPrice < tree.BuildCost && tree.BuyPrice > 0 {
		optimalCost = tree.BuyPrice
	}

	savings := marketBuyPrice - optimalCost
	savingsPercent := 0.0
	if marketBuyPrice > 0 {
		savingsPercent = savings / marketBuyPrice * 100
	}

	totalJobCost := a.sumJobCosts(tree)

	regionID := int32(0)
	regionName := ""
	if params.SystemID != 0 {
		if sys, ok := a.SDE.Systems[params.SystemID]; ok {
			regionID = sys.RegionID
			if r, ok := a.SDE.Regions[regionID]; ok {
				regionName = r.Name
			}
		}
	}

	return &IndustryAnalysis{
		TargetTypeID:     params.TypeID,
		TargetTypeName:   typeInfo.Name,
		Runs:             params.Runs,
		TotalQuantity:    tree.Quantity,
		MarketBuyPrice:   marketBuyPrice,
		TotalBuildCost:   tree.BuildCost,
		OptimalBuildCost: optimalCost,
		Savings:          savings,
		SavingsPercent:   savingsPercent,
		TotalJobCost:     totalJobCost,
		MaterialTree:     tree,
		FlatMaterials:    flatMaterials,
		SystemCostIndex:  costIndex,
		RegionID:         regionID,
		RegionName:       regionName,
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
		BuyPrice: a.marketPrices[typeID] * float64(quantity),
	}

	// Check if we can build this item
	bp, hasBP := a.SDE.Industry.GetBlueprintForProduct(typeID)
	if !hasBP || depth >= params.MaxDepth {
		node.IsBase = true
		return node
	}

	// Calculate how many runs we need
	runsNeeded := quantity / bp.ProductQuantity
	if quantity%bp.ProductQuantity != 0 {
		runsNeeded++
	}

	node.Blueprint = &BlueprintInfo{
		BlueprintTypeID: bp.BlueprintTypeID,
		ProductQuantity: bp.ProductQuantity,
		ME:              params.MaterialEfficiency,
		TE:              params.TimeEfficiency,
		Time:            bp.CalculateTimeWithTE(runsNeeded, params.TimeEfficiency),
	}

	// Get materials with ME applied
	materials := bp.CalculateMaterialsWithME(runsNeeded, params.MaterialEfficiency)

	// Apply structure bonus if any
	if params.StructureBonus > 0 {
		for i := range materials {
			reduction := float64(materials[i].Quantity) * params.StructureBonus / 100
			materials[i].Quantity -= int32(reduction)
			if materials[i].Quantity < runsNeeded {
				materials[i].Quantity = runsNeeded
			}
		}
	}

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

	// Calculate job installation cost
	// Formula: EIV * cost_index * (1 + facility_tax)
	eiv := a.calculateEIV(node)
	node.JobCost = eiv * costIndex * (1 + params.FacilityTax/100)

	node.BuildCost = materialCost + node.JobCost

	// Decide: buy or build
	if node.BuyPrice > 0 && node.BuyPrice < node.BuildCost {
		node.ShouldBuild = false
	} else {
		node.ShouldBuild = true
	}
}

// calculateEIV calculates Estimated Item Value for job cost.
func (a *IndustryAnalyzer) calculateEIV(node *MaterialNode) float64 {
	// EIV is sum of adjusted_price * quantity for all materials
	var eiv float64
	for _, child := range node.Children {
		price := a.adjustedPrices[child.TypeID]
		eiv += price * float64(child.Quantity)
	}
	return eiv
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

// fetchMarketPrices fetches best sell order prices for materials.
// Uses Jita (The Forge region) as default market.
// Results are cached for 10 minutes.
func (a *IndustryAnalyzer) fetchMarketPrices(params IndustryParams) (map[int32]float64, error) {
	// The Forge region ID (Jita)
	regionID := int32(10000002)
	
	// Use cached prices if available
	return a.ESI.GetCachedMarketPrices(a.IndustryCache, regionID)
}

// GetBlueprintInfo returns blueprint information for a type.
func (a *IndustryAnalyzer) GetBlueprintInfo(typeID int32) (*sde.Blueprint, bool) {
	return a.SDE.Industry.GetBlueprintForProduct(typeID)
}

// SearchResult holds a search result with relevance score.
type SearchResult struct {
	TypeID      int32  `json:"type_id"`
	TypeName    string `json:"type_name"`
	HasBlueprint bool  `json:"has_blueprint"`
	relevance   int    // 0 = exact, 1 = starts with, 2 = contains
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
