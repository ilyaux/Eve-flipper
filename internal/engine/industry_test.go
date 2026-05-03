package engine

import (
	"math"
	"testing"

	"eve-flipper/internal/esi"
	"eve-flipper/internal/sde"
)

// sumJobCosts sums JobCost for all nodes where ShouldBuild && !IsBase, recursively.

func TestSumJobCosts_EmptyAndBase(t *testing.T) {
	a := &IndustryAnalyzer{}
	// Nil node would panic; we don't call with nil. Base node with ShouldBuild=false has no job cost.
	base := &MaterialNode{IsBase: true, ShouldBuild: false}
	if got := a.sumJobCosts(base); got != 0 {
		t.Errorf("sumJobCosts(base node) = %v, want 0", got)
	}
}

func TestSumJobCosts_SingleLevel(t *testing.T) {
	a := &IndustryAnalyzer{}
	root := &MaterialNode{IsBase: false, ShouldBuild: true, JobCost: 100.0, Children: nil}
	if got := a.sumJobCosts(root); got != 100 {
		t.Errorf("sumJobCosts(single node JobCost=100) = %v, want 100", got)
	}
}

func TestSumJobCosts_Tree(t *testing.T) {
	a := &IndustryAnalyzer{}
	// Root: JobCost 50, ShouldBuild true. Child1: 30, Child2: 20. Total = 50+30+20 = 100
	child1 := &MaterialNode{IsBase: false, ShouldBuild: true, JobCost: 30, Children: nil}
	child2 := &MaterialNode{IsBase: false, ShouldBuild: true, JobCost: 20, Children: nil}
	root := &MaterialNode{IsBase: false, ShouldBuild: true, JobCost: 50, Children: []*MaterialNode{child1, child2}}
	if got := a.sumJobCosts(root); got != 100 {
		t.Errorf("sumJobCosts(tree 50+30+20) = %v, want 100", got)
	}
}

func TestSumJobCosts_SkipsNonBuildAndBase(t *testing.T) {
	a := &IndustryAnalyzer{}
	// Root ShouldBuild=false -> no root JobCost. Child ShouldBuild=true -> count child only.
	child := &MaterialNode{IsBase: false, ShouldBuild: true, JobCost: 25, Children: nil}
	root := &MaterialNode{IsBase: false, ShouldBuild: false, JobCost: 100, Children: []*MaterialNode{child}}
	if got := a.sumJobCosts(root); got != 25 {
		t.Errorf("sumJobCosts(root skip, child count) = %v, want 25", got)
	}
}

func TestGetBlueprintInfo_DelegatesToSDE(t *testing.T) {
	// Minimal SDE: IndustryData with one product -> blueprint
	ind := sde.NewIndustryData()
	bp := &sde.Blueprint{ProductTypeID: 999, ProductQuantity: 2}
	ind.Blueprints[100] = bp
	ind.ProductToBlueprint[999] = 100

	a := &IndustryAnalyzer{SDE: &sde.Data{Industry: ind}}

	got, ok := a.GetBlueprintInfo(999)
	if !ok || got != bp {
		t.Errorf("GetBlueprintInfo(999) = %v, %v; want bp, true", got, ok)
	}
	_, ok = a.GetBlueprintInfo(888)
	if ok {
		t.Error("GetBlueprintInfo(888) should be false")
	}
}

func TestResolveMarketRegion_PrefersSystemOverStation(t *testing.T) {
	a := &IndustryAnalyzer{
		SDE: &sde.Data{
			Systems: map[int32]*sde.SolarSystem{
				30000142: {ID: 30000142, RegionID: 10000002},
				30002187: {ID: 30002187, RegionID: 10000043},
			},
			Stations: map[int64]*sde.Station{
				60008494: {ID: 60008494, SystemID: 30002187},
			},
			Regions: map[int32]*sde.Region{
				10000002: {ID: 10000002, Name: "The Forge"},
				10000043: {ID: 10000043, Name: "Domain"},
			},
		},
	}

	regionID, regionName := a.resolveMarketRegion(IndustryParams{
		SystemID:  30000142,
		StationID: 60008494,
	})

	if regionID != 10000002 {
		t.Fatalf("regionID = %d, want 10000002", regionID)
	}
	if regionName != "The Forge" {
		t.Fatalf("regionName = %q, want The Forge", regionName)
	}
}

func TestResolveMarketRegion_UsesStationWhenSystemMissing(t *testing.T) {
	a := &IndustryAnalyzer{
		SDE: &sde.Data{
			Systems: map[int32]*sde.SolarSystem{
				30000142: {ID: 30000142, RegionID: 10000002},
			},
			Stations: map[int64]*sde.Station{
				60003760: {ID: 60003760, SystemID: 30000142},
			},
			Regions: map[int32]*sde.Region{
				10000002: {ID: 10000002, Name: "The Forge"},
			},
		},
	}

	regionID, regionName := a.resolveMarketRegion(IndustryParams{
		SystemID:  0,
		StationID: 60003760,
	})

	if regionID != 10000002 {
		t.Fatalf("regionID = %d, want 10000002", regionID)
	}
	if regionName != "The Forge" {
		t.Fatalf("regionName = %q, want The Forge", regionName)
	}
}

func TestMergeMarketPrices_StationOverridesRegionWithFallback(t *testing.T) {
	region := map[int32]float64{
		34:    5.0,  // fallback only
		35:    12.0, // overridden by station
		11399: 1.5,  // fallback only
	}
	station := map[int32]float64{
		35: 9.5,  // station override
		36: 20.0, // station-only type
	}

	got := mergeMarketPrices(region, station)

	if got[34] != 5.0 {
		t.Fatalf("type 34 = %v, want 5.0", got[34])
	}
	if got[35] != 9.5 {
		t.Fatalf("type 35 = %v, want 9.5", got[35])
	}
	if got[36] != 20.0 {
		t.Fatalf("type 36 = %v, want 20.0", got[36])
	}
	if got[11399] != 1.5 {
		t.Fatalf("type 11399 = %v, want 1.5", got[11399])
	}
}

func TestAnalyze_EndToEndInjectedPricing(t *testing.T) {
	sdeData := newTestIndustrySDE()
	a := &IndustryAnalyzer{
		SDE:           sdeData,
		IndustryCache: esi.NewIndustryCache(),
		getAllAdjustedPrices: func(_ *esi.IndustryCache) (map[int32]float64, error) {
			return map[int32]float64{
				34:   1.0,
				1001: 2.0,
				1002: 3.0,
			}, nil
		},
		getSystemCostIndex: func(_ *esi.IndustryCache, systemID int32) (*esi.SystemCostIndices, error) {
			if systemID != 30000142 {
				t.Fatalf("systemID = %d, want 30000142", systemID)
			}
			return &esi.SystemCostIndices{Manufacturing: 0.1}, nil
		},
		fetchMarketPricesFn: func(_ IndustryParams) (map[int32]float64, error) {
			return map[int32]float64{
				34:   1.0,
				1000: 300.0,
				1001: 20.0,
				1002: 15.0,
			}, nil
		},
	}

	progress := make([]string, 0, 5)
	result, err := a.Analyze(IndustryParams{
		TypeID:             1000,
		Runs:               2,
		SystemID:           30000142,
		BrokerFee:          5,
		SalesTaxPercent:    10,
		MaterialEfficiency: 0,
		TimeEfficiency:     0,
	}, func(msg string) {
		progress = append(progress, msg)
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(progress) != 6 {
		t.Fatalf("progress count = %d, want 6", len(progress))
	}

	if result.TotalQuantity != 2 {
		t.Fatalf("TotalQuantity = %d, want 2", result.TotalQuantity)
	}
	if result.RegionID != 10000002 || result.RegionName != "The Forge" {
		t.Fatalf("region = (%d, %q), want (10000002, The Forge)", result.RegionID, result.RegionName)
	}
	if !industryAlmostEqual(result.SystemCostIndex, 0.1) {
		t.Fatalf("SystemCostIndex = %v, want 0.1", result.SystemCostIndex)
	}
	if !industryAlmostEqual(result.MarketBuyPrice, 600.0) {
		t.Fatalf("MarketBuyPrice = %v, want 600", result.MarketBuyPrice)
	}
	if !industryAlmostEqual(result.TotalBuildCost, 223.0) {
		t.Fatalf("TotalBuildCost = %v, want 223", result.TotalBuildCost)
	}
	if !industryAlmostEqual(result.OptimalBuildCost, 223.0) {
		t.Fatalf("OptimalBuildCost = %v, want 223", result.OptimalBuildCost)
	}
	if !industryAlmostEqual(result.TotalJobCost, 13.0) {
		t.Fatalf("TotalJobCost = %v, want 13", result.TotalJobCost)
	}
	if !industryAlmostEqual(result.SellRevenue, 513.0) {
		t.Fatalf("SellRevenue = %v, want 513", result.SellRevenue)
	}
	if !industryAlmostEqual(result.Profit, 290.0) {
		t.Fatalf("Profit = %v, want 290", result.Profit)
	}
	if !industryAlmostEqual(result.ISKPerHour, 54.375) {
		t.Fatalf("ISKPerHour = %v, want 54.375 using full activity-chain time", result.ISKPerHour)
	}
	if result.MaterialTree == nil {
		t.Fatalf("MaterialTree is nil")
	}
	if !result.MaterialTree.ShouldBuild {
		t.Fatalf("root should_build = false, want true")
	}

	byType := map[int32]*MaterialNode{}
	for _, child := range result.MaterialTree.Children {
		byType[child.TypeID] = child
	}
	componentNode := byType[1001]
	if componentNode == nil {
		t.Fatalf("component node (1001) missing")
	}
	if !componentNode.ShouldBuild {
		t.Fatalf("component node should_build = false, want true")
	}
	baseNode := byType[1002]
	if baseNode == nil {
		t.Fatalf("base material node (1002) missing")
	}
	if baseNode.ShouldBuild {
		t.Fatalf("base material node should_build = true, want false")
	}

	if len(result.FlatMaterials) != 2 {
		t.Fatalf("flat materials len = %d, want 2", len(result.FlatMaterials))
	}
	flatByType := map[int32]*FlatMaterial{}
	for _, m := range result.FlatMaterials {
		flatByType[m.TypeID] = m
	}
	if flatByType[1002] == nil || flatByType[1002].Quantity != 10 {
		t.Fatalf("flat material 1002 = %+v, want quantity 10", flatByType[1002])
	}
	if flatByType[34] == nil || flatByType[34].Quantity != 60 {
		t.Fatalf("flat material 34 = %+v, want quantity 60", flatByType[34])
	}
}

func TestAnalyze_UsesDepthAwareBuyCostAndInstantSellProfit(t *testing.T) {
	sdeData := newTestIndustrySDE()
	a := &IndustryAnalyzer{
		SDE:           sdeData,
		IndustryCache: esi.NewIndustryCache(),
		getAllAdjustedPrices: func(_ *esi.IndustryCache) (map[int32]float64, error) {
			return map[int32]float64{
				34:   1.0,
				1001: 2.0,
				1002: 3.0,
			}, nil
		},
		getSystemCostIndex: func(_ *esi.IndustryCache, systemID int32) (*esi.SystemCostIndices, error) {
			return &esi.SystemCostIndices{Manufacturing: 0.1}, nil
		},
		fetchMarketPricesFn: func(_ IndustryParams) (map[int32]float64, error) {
			return map[int32]float64{
				34:   1.0,
				1000: 300.0,
				1001: 20.0,
				1002: 15.0,
			}, nil
		},
		fetchMarketBooksFn: func(_ IndustryParams) (map[int32][]esi.MarketOrder, map[int32][]esi.MarketOrder, error) {
			return map[int32][]esi.MarketOrder{
					34: {
						{TypeID: 34, Price: 1, VolumeRemain: 60},
					},
					1000: {
						{TypeID: 1000, Price: 300, VolumeRemain: 1},
						{TypeID: 1000, Price: 400, VolumeRemain: 1},
					},
					1001: {
						{TypeID: 1001, Price: 20, VolumeRemain: 20},
					},
					1002: {
						{TypeID: 1002, Price: 15, VolumeRemain: 20},
					},
				},
				map[int32][]esi.MarketOrder{
					1000: {
						{TypeID: 1000, Price: 250, VolumeRemain: 2, IsBuyOrder: true},
					},
				},
				nil
		},
	}

	result, err := a.Analyze(IndustryParams{
		TypeID:             1000,
		Runs:               2,
		SystemID:           30000142,
		BrokerFee:          5,
		SalesTaxPercent:    10,
		MaterialEfficiency: 0,
		TimeEfficiency:     0,
	}, func(string) {})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if !industryAlmostEqual(result.MarketBuyPrice, 700.0) {
		t.Fatalf("MarketBuyPrice = %v, want depth-aware 700", result.MarketBuyPrice)
	}
	if !result.InstantSellAvailable {
		t.Fatalf("InstantSellAvailable = false, want true")
	}
	if !industryAlmostEqual(result.InstantSellRevenue, 450.0) {
		t.Fatalf("InstantSellRevenue = %v, want 450", result.InstantSellRevenue)
	}
	if !industryAlmostEqual(result.MakerSellRevenue, 513.0) {
		t.Fatalf("MakerSellRevenue = %v, want 513", result.MakerSellRevenue)
	}
	if !industryAlmostEqual(result.SellRevenue, result.InstantSellRevenue) {
		t.Fatalf("SellRevenue = %v, want conservative instant revenue %v", result.SellRevenue, result.InstantSellRevenue)
	}
	if !industryAlmostEqual(result.Profit, 227.0) {
		t.Fatalf("Profit = %v, want instant liquidation profit 227", result.Profit)
	}
}

func TestBuildMaterialTree_AppliesMEEAndMaxDepth(t *testing.T) {
	a := &IndustryAnalyzer{
		SDE: newTestIndustrySDE(),
		marketPrices: map[int32]float64{
			1000: 300,
			1001: 20,
			1002: 15,
			34:   1,
		},
	}

	tree := a.buildMaterialTree(1000, 2, IndustryParams{
		MaxDepth:           1,
		MaterialEfficiency: 10,
		StructureBonus:     1,
	}, 0)
	if tree.IsBase {
		t.Fatalf("root IsBase = true, want false")
	}
	if len(tree.Children) != 2 {
		t.Fatalf("children len = %d, want 2", len(tree.Children))
	}

	byType := map[int32]*MaterialNode{}
	for _, child := range tree.Children {
		byType[child.TypeID] = child
	}
	component := byType[1001]
	if component == nil {
		t.Fatalf("component child missing")
	}
	if component.Quantity != 18 {
		t.Fatalf("component quantity = %d, want 18", component.Quantity)
	}
	if !component.IsBase {
		t.Fatalf("component IsBase = false, want true because max depth reached")
	}
}

func TestCalculateCosts_PrefersBuyingWhenCheaper(t *testing.T) {
	a := &IndustryAnalyzer{
		SDE: newTestIndustrySDE(),
		marketPrices: map[int32]float64{
			1001: 5,
			34:   10,
		},
		adjustedPrices: map[int32]float64{
			34: 1,
		},
	}

	tree := a.buildMaterialTree(1001, 1, IndustryParams{MaxDepth: 10}, 0)
	a.calculateCosts(tree, 0.1, IndustryParams{})

	if tree.ShouldBuild {
		t.Fatalf("tree.ShouldBuild = true, want false")
	}
	if !industryAlmostEqual(tree.BuyPrice, 5.0) {
		t.Fatalf("BuyPrice = %v, want 5", tree.BuyPrice)
	}
	if !industryAlmostEqual(tree.BuildCost, 30.3) {
		t.Fatalf("BuildCost = %v, want 30.3", tree.BuildCost)
	}
	if !industryAlmostEqual(tree.JobCost, 0.3) {
		t.Fatalf("JobCost = %v, want 0.3", tree.JobCost)
	}
}

func TestAnalyze_ReactionActivityUsesReactionMaterialsAndCostIndex(t *testing.T) {
	ind := sde.NewIndustryData()
	ind.Blueprints[3000] = &sde.Blueprint{
		BlueprintTypeID: 3000,
		ProductTypeID:   4000,
		ProductQuantity: 2,
		Activities: map[string]*sde.ActivityData{
			"reaction": {
				Time: 600,
				Materials: []sde.BlueprintMaterial{
					{TypeID: 34, Quantity: 5},
				},
				Products: []sde.BlueprintProduct{
					{TypeID: 4000, Quantity: 2},
				},
			},
		},
	}
	ind.ProductToBlueprint[4000] = 3000
	a := &IndustryAnalyzer{
		SDE: &sde.Data{
			Types: map[int32]*sde.ItemType{
				34:   {ID: 34, Name: "Tritanium"},
				3000: {ID: 3000, Name: "Reaction Formula"},
				4000: {ID: 4000, Name: "Reacted Material"},
			},
			Systems: map[int32]*sde.SolarSystem{
				30000142: {ID: 30000142, Name: "Jita", RegionID: 10000002},
			},
			Regions:  map[int32]*sde.Region{10000002: {ID: 10000002, Name: "The Forge"}},
			Industry: ind,
		},
		IndustryCache: esi.NewIndustryCache(),
		getAllAdjustedPrices: func(_ *esi.IndustryCache) (map[int32]float64, error) {
			return map[int32]float64{34: 1}, nil
		},
		getSystemCostIndex: func(_ *esi.IndustryCache, _ int32) (*esi.SystemCostIndices, error) {
			return &esi.SystemCostIndices{Manufacturing: 0.01, Reaction: 0.2}, nil
		},
		fetchMarketPricesFn: func(_ IndustryParams) (map[int32]float64, error) {
			return map[int32]float64{34: 10, 4000: 100}, nil
		},
		fetchMarketBooksFn: func(_ IndustryParams) (map[int32][]esi.MarketOrder, map[int32][]esi.MarketOrder, error) {
			return nil, nil, nil
		},
	}

	result, err := a.Analyze(IndustryParams{
		TypeID:       4000,
		Runs:         2,
		ActivityMode: "reaction",
		SystemID:     30000142,
	}, func(string) {})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if result.TotalQuantity != 4 {
		t.Fatalf("TotalQuantity = %d, want 4", result.TotalQuantity)
	}
	if result.MaterialTree.Activity != "reaction" {
		t.Fatalf("root activity = %q, want reaction", result.MaterialTree.Activity)
	}
	if !industryAlmostEqual(result.TotalBuildCost, 102) {
		t.Fatalf("TotalBuildCost = %v, want 102", result.TotalBuildCost)
	}
	if !industryAlmostEqual(result.TotalJobCost, 2) {
		t.Fatalf("TotalJobCost = %v, want reaction-index job cost 2", result.TotalJobCost)
	}
	if len(result.FlatMaterials) != 1 || result.FlatMaterials[0].TypeID != 34 || result.FlatMaterials[0].Quantity != 10 {
		t.Fatalf("flat materials = %+v, want 10 Tritanium", result.FlatMaterials)
	}
	if len(result.ActivityPlan) != 1 || result.ActivityPlan[0].Activity != "reaction" {
		t.Fatalf("activity plan = %+v, want one reaction step", result.ActivityPlan)
	}
}

func TestAnalyze_InventionAddsExpectedBPCCost(t *testing.T) {
	ind := sde.NewIndustryData()
	ind.Blueprints[5001] = &sde.Blueprint{
		BlueprintTypeID: 5001,
		ProductTypeID:   5000,
		ProductQuantity: 1,
		Time:            1000,
		Materials:       []sde.BlueprintMaterial{{TypeID: 34, Quantity: 10}},
		Activities: map[string]*sde.ActivityData{
			"manufacturing": {
				Time:      1000,
				Materials: []sde.BlueprintMaterial{{TypeID: 34, Quantity: 10}},
				Products:  []sde.BlueprintProduct{{TypeID: 5000, Quantity: 1}},
			},
		},
	}
	ind.ProductToBlueprint[5000] = 5001
	ind.Blueprints[5100] = &sde.Blueprint{
		BlueprintTypeID: 5100,
		Activities: map[string]*sde.ActivityData{
			"invention": {
				Time:      100,
				Materials: []sde.BlueprintMaterial{{TypeID: 6001, Quantity: 2}},
				Products:  []sde.BlueprintProduct{{TypeID: 5001, Quantity: 10, Probability: 0.4}},
			},
		},
	}
	a := &IndustryAnalyzer{
		SDE: &sde.Data{
			Types: map[int32]*sde.ItemType{
				34:   {ID: 34, Name: "Tritanium"},
				5000: {ID: 5000, Name: "T2 Module"},
				5001: {ID: 5001, Name: "T2 Module Blueprint"},
				5100: {ID: 5100, Name: "T1 Module Blueprint"},
				6001: {ID: 6001, Name: "Datacore"},
			},
			Systems:  map[int32]*sde.SolarSystem{30000142: {ID: 30000142, Name: "Jita", RegionID: 10000002}},
			Regions:  map[int32]*sde.Region{10000002: {ID: 10000002, Name: "The Forge"}},
			Industry: ind,
		},
		IndustryCache: esi.NewIndustryCache(),
		getAllAdjustedPrices: func(_ *esi.IndustryCache) (map[int32]float64, error) {
			return map[int32]float64{34: 1, 6001: 50}, nil
		},
		getSystemCostIndex: func(_ *esi.IndustryCache, _ int32) (*esi.SystemCostIndices, error) {
			return &esi.SystemCostIndices{Manufacturing: 0, Invention: 0.1}, nil
		},
		fetchMarketPricesFn: func(_ IndustryParams) (map[int32]float64, error) {
			return map[int32]float64{34: 5, 5000: 1000, 6001: 100}, nil
		},
		fetchMarketBooksFn: func(_ IndustryParams) (map[int32][]esi.MarketOrder, map[int32][]esi.MarketOrder, error) {
			return nil, nil, nil
		},
	}

	result, err := a.Analyze(IndustryParams{
		TypeID:       5000,
		Runs:         20,
		ActivityMode: "invention",
		SystemID:     30000142,
	}, func(string) {})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if !industryAlmostEqual(result.InventionAttempts, 5) {
		t.Fatalf("InventionAttempts = %v, want 5", result.InventionAttempts)
	}
	if !industryAlmostEqual(result.InventionProbability, 0.4) {
		t.Fatalf("InventionProbability = %v, want 0.4", result.InventionProbability)
	}
	if !industryAlmostEqual(result.InventionCost, 1050) {
		t.Fatalf("InventionCost = %v, want 1050", result.InventionCost)
	}
	if !industryAlmostEqual(result.OptimalBuildCost, 2050) {
		t.Fatalf("OptimalBuildCost = %v, want build 1000 + invention 1050", result.OptimalBuildCost)
	}
	if len(result.ActivityPlan) < 2 || result.ActivityPlan[0].Activity != "invention" || result.ActivityPlan[1].Activity != "manufacturing" {
		t.Fatalf("activity plan = %+v, want invention then manufacturing", result.ActivityPlan)
	}
}

func TestAnalyze_TypeNotFound(t *testing.T) {
	a := &IndustryAnalyzer{
		SDE: &sde.Data{
			Types: map[int32]*sde.ItemType{},
		},
	}

	_, err := a.Analyze(IndustryParams{TypeID: 999999}, func(string) {})
	if err == nil {
		t.Fatalf("Analyze should fail for unknown type")
	}
}

func industryAlmostEqual(got, want float64) bool {
	return math.Abs(got-want) < 0.000001
}

func newTestIndustrySDE() *sde.Data {
	ind := sde.NewIndustryData()

	ind.Blueprints[2000] = &sde.Blueprint{
		BlueprintTypeID: 2000,
		ProductTypeID:   1000,
		ProductQuantity: 1,
		Time:            3600,
		Materials: []sde.BlueprintMaterial{
			{TypeID: 1001, Quantity: 10},
			{TypeID: 1002, Quantity: 5},
		},
	}
	ind.ProductToBlueprint[1000] = 2000

	ind.Blueprints[2001] = &sde.Blueprint{
		BlueprintTypeID: 2001,
		ProductTypeID:   1001,
		ProductQuantity: 1,
		Time:            600,
		Materials: []sde.BlueprintMaterial{
			{TypeID: 34, Quantity: 3},
		},
	}
	ind.ProductToBlueprint[1001] = 2001

	return &sde.Data{
		Types: map[int32]*sde.ItemType{
			34:   {ID: 34, Name: "Tritanium", Volume: 0.01},
			1000: {ID: 1000, Name: "Final Item", Volume: 5},
			1001: {ID: 1001, Name: "Build Component", Volume: 1},
			1002: {ID: 1002, Name: "Base Component", Volume: 0.5},
		},
		Systems: map[int32]*sde.SolarSystem{
			30000142: {ID: 30000142, Name: "Jita", RegionID: 10000002},
		},
		Regions: map[int32]*sde.Region{
			10000002: {ID: 10000002, Name: "The Forge"},
		},
		Stations: map[int64]*sde.Station{},
		Industry: ind,
	}
}
