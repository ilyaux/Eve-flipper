package engine

import (
	"sort"

	"eve-flipper/internal/sde"
)

// BatchOptimize assembles a cargo manifest from flip opportunities using greedy knapsack.
// Sorts by ISK profit per m³, greedily fills until capacity or budget exhausted.
// Returns a CargoManifest with allocated items and utilization metrics.
func BatchOptimize(results []FlipResult, cargoM3 float64, budget float64, sde *sde.Data) CargoManifest {
	if cargoM3 <= 0 || len(results) == 0 {
		return CargoManifest{Items: []BatchItem{}}
	}

	// Filter to only profitable items and compute m³ per unit
	type weightedResult struct {
		flip      FlipResult
		volumeM3  float64 // m³ per unit
		profitM3  float64 // ISK profit per m³ (higher = better)
	}

	var candidates []weightedResult
	for i := range results {
		r := results[i]
		if r.RealProfit <= 0 || r.UnitsToBuy <= 0 {
			continue
		}

		// Lookup item volume from SDE
		typeData, ok := sde.Types[r.TypeID]
		if !ok || typeData.Volume == 0 {
			continue // skip items without volume info
		}

		itemM3 := typeData.Volume
		totalM3 := float64(r.UnitsToBuy) * itemM3
		profitPerM3 := r.RealProfit / totalM3
		if profitPerM3 <= 0 {
			continue
		}

		candidates = append(candidates, weightedResult{
			flip:     r,
			volumeM3: itemM3,
			profitM3: profitPerM3,
		})
	}

	if len(candidates) == 0 {
		return CargoManifest{Items: []BatchItem{}}
	}

	// Sort by profit per m³ descending (greedy: best ROI first)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].profitM3 > candidates[j].profitM3
	})

	// Greedy fill: iterate through candidates, allocate as much as fits
	var items []BatchItem
	usedM3 := 0.0
	usedISK := 0.0
	totalProfit := 0.0

	for _, cand := range candidates {
		if usedM3 >= cargoM3 {
			break // capacity exhausted
		}

		r := cand.flip
		remainingM3 := cargoM3 - usedM3
		maxUnitsByVolume := int32(remainingM3 / cand.volumeM3)

		// Cap by budget: assume we need capital equal to the profit we'll make
		remainingBudget := budget - usedISK
		maxUnitsByBudget := int32(remainingBudget / r.RealProfit * float64(r.UnitsToBuy))

		// Allocate minimum of: volume-constrained, budget-constrained, or available units
		allocated := maxUnitsByVolume
		if maxUnitsByBudget < allocated {
			allocated = maxUnitsByBudget
		}
		if r.UnitsToBuy < allocated {
			allocated = r.UnitsToBuy
		}

		if allocated <= 0 {
			continue
		}

		allocatedM3 := float64(allocated) * cand.volumeM3
		allocatedISK := r.RealProfit * float64(allocated) / float64(r.UnitsToBuy)
		allocatedProfit := r.RealProfit * float64(allocated) / float64(r.UnitsToBuy)

		items = append(items, BatchItem{
			TypeID:         r.TypeID,
			TypeName:       r.TypeName,
			AllocatedUnits: allocated,
			AllocatedM3:    allocatedM3,
			AllocatedISK:   allocatedISK,
			ProfitPerUnit:  r.RealProfit / float64(r.UnitsToBuy),
			TotalProfit:    allocatedProfit,
			BuyStation:     r.BuyStation,
			SellStation:    r.SellStation,
			TotalJumps:     r.TotalJumps,
		})

		usedM3 += allocatedM3
		usedISK += allocatedISK
		totalProfit += allocatedProfit
	}

	capacityUsed := 0.0
	if cargoM3 > 0 {
		capacityUsed = usedM3 / cargoM3
	}
	budgetUsed := 0.0
	if budget > 0 {
		budgetUsed = usedISK / budget
	}

	return CargoManifest{
		Items:        items,
		TotalM3:      usedM3,
		TotalISK:     usedISK,
		TotalProfit:  totalProfit,
		CapacityUsed: capacityUsed,
		BudgetUsed:   budgetUsed,
	}
}
