package engine

import (
	"math"
	"sort"

	"eve-flipper/internal/esi"
)

// ExecutionPlanRequest is the input for computing an execution plan (slippage simulation).
type ExecutionPlanRequest struct {
	TypeID     int32
	RegionID   int32
	LocationID int64 // 0 = whole region
	Quantity   int32 // desired buy/sell volume
	IsBuy      bool  // true = simulate buying (walk sell orders), false = simulate selling (walk buy orders)
}

// DepthLevel represents one price level in the fill curve.
type DepthLevel struct {
	Price        float64 `json:"price"`
	Volume       int32   `json:"volume"`
	Cumulative   int32   `json:"cumulative"`
	VolumeFilled int32   `json:"volume_filled"` // how much of this level we consume for requested Q
}

// ExecutionPlanResult is the output of the slippage simulator.
type ExecutionPlanResult struct {
	BestPrice       float64      `json:"best_price"`        // top of book
	ExpectedPrice   float64      `json:"expected_price"`    // volume-weighted avg fill price
	SlippagePercent float64      `json:"slippage_percent"`  // (expected - best) / best * 100
	TotalCost       float64      `json:"total_cost"`        // expected price * filled quantity (buy cost / sell revenue for fillable part)
	DepthLevels     []DepthLevel `json:"depth_levels"`      // fill curve (first N levels until Q filled)
	TotalDepth      int32        `json:"total_depth"`       // total volume in book (for this type/location)
	CanFill         bool         `json:"can_fill"`          // book has enough volume for Q
	OptimalSlices   int          `json:"optimal_slices"`    // suggested number of orders to split into
	SuggestedMinGap int          `json:"suggested_min_gap"` // minutes between slices (simple heuristic)
	// Impact is set when market history is available (Kyle's λ, √V impact, TWAP n*).
	Impact *ImpactEstimate `json:"impact,omitempty"`
}

// ComputeExecutionPlan walks the order book and computes expected fill price, slippage, and suggested slicing.
// orders: sell orders for buy simulation (or buy orders for sell simulation), already filtered by type and optional location.
func ComputeExecutionPlan(orders []esi.MarketOrder, quantity int32, isBuy bool) ExecutionPlanResult {
	var out ExecutionPlanResult
	if quantity <= 0 || len(orders) == 0 {
		return out
	}

	// Aggregate volume at each price level (same price = sum volume)
	type level struct {
		price  float64
		volume int32
	}
	levelMap := make(map[float64]int32)
	filteredDepth := int32(0)
	for _, o := range orders {
		// Buy simulation consumes sell orders (asks), sell simulation consumes buy orders (bids).
		if isBuy && o.IsBuyOrder {
			continue
		}
		if !isBuy && !o.IsBuyOrder {
			continue
		}
		levelMap[o.Price] += o.VolumeRemain
		filteredDepth += o.VolumeRemain
	}
	// If side-filter removed everything, return empty result rather than
	// silently using wrong-side orders which would produce incorrect prices.
	if filteredDepth == 0 {
		return out
	}
	var levels []level
	for p, v := range levelMap {
		levels = append(levels, level{p, v})
	}

	// Sort by price: for buy we walk from lowest ask; for sell from highest bid
	sort.Slice(levels, func(i, j int) bool {
		if isBuy {
			return levels[i].price < levels[j].price
		}
		return levels[i].price > levels[j].price
	})

	if len(levels) == 0 {
		return out
	}

	out.BestPrice = levels[0].price
	out.TotalDepth = 0
	for _, lv := range levels {
		out.TotalDepth += lv.volume
	}

	// Walk book and fill Q
	remaining := quantity
	var costSum float64
	var filled int32

	for _, lv := range levels {
		if remaining <= 0 {
			break
		}
		vol := lv.volume
		if vol > remaining {
			vol = remaining
		}
		remaining -= vol
		costSum += lv.price * float64(vol)
		filled += vol
		out.DepthLevels = append(out.DepthLevels, DepthLevel{
			Price:        lv.price,
			Volume:       lv.volume,
			VolumeFilled: vol,
		})
	}

	// Cumulative for display
	cum := int32(0)
	for i := range out.DepthLevels {
		cum += out.DepthLevels[i].VolumeFilled
		out.DepthLevels[i].Cumulative = cum
	}

	out.CanFill = remaining <= 0
	if filled == 0 {
		return out
	}

	out.ExpectedPrice = costSum / float64(filled)
	if out.BestPrice > 0 {
		out.SlippagePercent = (out.ExpectedPrice - out.BestPrice) / out.BestPrice * 100
		if !isBuy {
			out.SlippagePercent = -out.SlippagePercent // for sell, we get less than best
		}
	}
	out.TotalCost = out.ExpectedPrice * float64(filled)

	// Optimal slicing: participation-rate model.
	// Each slice should not exceed targetPct of total book depth to avoid
	// excessive price impact. This aligns with the same principle used in
	// OptimalSlicesVolume (impact.go): n* = ceil(Q / (targetPct × Depth)).
	const targetPct = 0.05 // max 5% of book depth per slice
	sliceSize := float64(out.TotalDepth) * targetPct
	if sliceSize < 10 {
		sliceSize = 10 // floor: even for illiquid items, at least 10 units per slice
	}
	n := int(math.Ceil(float64(quantity) / sliceSize))
	if n < 1 {
		n = 1
	}
	if n > 20 {
		n = 20
	}
	out.OptimalSlices = n
	// Suggest gap: scale with number of slices.
	// More slices → longer gaps to let the book replenish.
	switch {
	case n <= 1:
		out.SuggestedMinGap = 0
	case n <= 3:
		out.SuggestedMinGap = 5
	case n <= 8:
		out.SuggestedMinGap = 10
	default:
		out.SuggestedMinGap = 15
	}

	return out
}
