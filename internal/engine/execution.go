package engine

import (
	"math"
	"sort"

	"eve-flipper/internal/esi"
)

func clampInt64ToInt32(v int64) int32 {
	if v <= 0 {
		return 0
	}
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(v)
}

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
	VolumeFilled    int32        `json:"volume_filled"`     // quantity that can be filled from the walked book
	DepthLevels     []DepthLevel `json:"depth_levels"`      // fill curve (first N levels until Q filled)
	TotalDepth      int32        `json:"total_depth"`       // total volume in book (for this type/location)
	CanFill         bool         `json:"can_fill"`          // book has enough volume for Q
	OptimalSlices   int          `json:"optimal_slices"`    // suggested number of orders to split into
	SuggestedMinGap int          `json:"suggested_min_gap"` // minutes between slices (simple heuristic)
	// Impact is set when market history is available (Kyle's λ, √V impact, TWAP n*).
	Impact *ImpactEstimate `json:"impact,omitempty"`
	Quote  *ExecutionQuote `json:"quote,omitempty"`
}

type ExecutionQuoteFeeInputs struct {
	SplitTradeFees       bool
	BrokerFeePercent     float64
	SalesTaxPercent      float64
	BuyBrokerFeePercent  float64
	SellBrokerFeePercent float64
	BuySalesTaxPercent   float64
	SellSalesTaxPercent  float64
}

type ExecutionQuoteSide struct {
	RegionID        int32               `json:"region_id,omitempty"`
	SystemID        int32               `json:"system_id,omitempty"`
	LocationID      int64               `json:"location_id,omitempty"`
	VWAP            float64             `json:"vwap"`
	BestPrice       float64             `json:"best_price"`
	GrossISK        float64             `json:"gross_isk"`
	FeeISK          float64             `json:"fee_isk"`
	FilledQty       int32               `json:"filled_qty"`
	CanFill         bool                `json:"can_fill"`
	TotalDepth      int32               `json:"total_depth"`
	SlippagePercent float64             `json:"slippage_percent"`
	Plan            ExecutionPlanResult `json:"plan"`
}

type ExecutionQuoteCacheInfo struct {
	BuyAgeSeconds  int64 `json:"buy_age_seconds,omitempty"`
	SellAgeSeconds int64 `json:"sell_age_seconds,omitempty"`
	BuyTTLSeconds  int64 `json:"buy_ttl_seconds,omitempty"`
	SellTTLSeconds int64 `json:"sell_ttl_seconds,omitempty"`
	Stale          bool  `json:"stale"`
}

type ExecutionQuote struct {
	TypeID                int32                    `json:"type_id"`
	RequestedQty          int32                    `json:"requested_qty"`
	FillQty               int32                    `json:"fill_qty"`
	PartialReason         string                   `json:"partial_reason,omitempty"`
	BuyVWAP               float64                  `json:"buy_vwap"`
	SellVWAP              float64                  `json:"sell_vwap"`
	BuyGross              float64                  `json:"buy_gross"`
	SellGross             float64                  `json:"sell_gross"`
	BuyFees               float64                  `json:"buy_fees"`
	SellFees              float64                  `json:"sell_fees"`
	TotalFees             float64                  `json:"total_fees"`
	ShippingCost          float64                  `json:"shipping_cost"`
	ShippingJumps         int                      `json:"shipping_jumps"`
	ShippingCostPerM3Jump float64                  `json:"shipping_cost_per_m3_jump"`
	PackagedVolumeM3      float64                  `json:"packaged_volume_m3"`
	FilledVolumeM3        float64                  `json:"filled_volume_m3"`
	NetProfit             float64                  `json:"net_profit"`
	ProfitPerUnit         float64                  `json:"profit_per_unit"`
	ROIPercent            float64                  `json:"roi_percent"`
	Decision              string                   `json:"decision"`
	Warnings              []string                 `json:"warnings,omitempty"`
	Cache                 *ExecutionQuoteCacheInfo `json:"cache,omitempty"`
	Buy                   ExecutionQuoteSide       `json:"buy"`
	Sell                  ExecutionQuoteSide       `json:"sell"`
}

type ExecutionQuoteInput struct {
	TypeID                int32
	RequestedQty          int32
	BuyRegionID           int32
	BuySystemID           int32
	BuyLocationID         int64
	SellRegionID          int32
	SellSystemID          int32
	SellLocationID        int64
	BuyOrders             []esi.MarketOrder
	SellOrders            []esi.MarketOrder
	PackagedVolumeM3      float64
	ShippingCostPerM3Jump float64
	ShippingJumps         int
	Fees                  ExecutionQuoteFeeInputs
	Warnings              []string
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
		volume int64
	}
	levelMap := make(map[float64]int64)
	filteredDepth := int64(0)
	for _, o := range orders {
		// Buy simulation consumes sell orders (asks), sell simulation consumes buy orders (bids).
		if isBuy && o.IsBuyOrder {
			continue
		}
		if !isBuy && !o.IsBuyOrder {
			continue
		}
		if o.VolumeRemain <= 0 {
			continue
		}
		vol := int64(o.VolumeRemain)
		levelMap[o.Price] += vol
		filteredDepth += vol
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
	totalDepthAcc := int64(0)
	for _, lv := range levels {
		totalDepthAcc += lv.volume
	}
	out.TotalDepth = clampInt64ToInt32(totalDepthAcc)

	// Walk book and fill Q
	remaining := int64(quantity)
	var costSum float64
	var filled int64

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
			Volume:       clampInt64ToInt32(lv.volume),
			VolumeFilled: clampInt64ToInt32(vol),
		})
	}

	// Cumulative for display
	cum := int64(0)
	for i := range out.DepthLevels {
		cum += int64(out.DepthLevels[i].VolumeFilled)
		out.DepthLevels[i].Cumulative = clampInt64ToInt32(cum)
	}

	out.CanFill = remaining <= 0
	if filled == 0 {
		return out
	}

	out.VolumeFilled = clampInt64ToInt32(filled)
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
	sliceSize := float64(totalDepthAcc) * targetPct
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

func minPositiveInt32(values ...int32) int32 {
	var out int32
	for _, v := range values {
		if v <= 0 {
			return 0
		}
		if out == 0 || v < out {
			out = v
		}
	}
	return out
}

func executionPartialReason(requested, fill, buyFill, sellFill int32) string {
	if fill <= 0 {
		switch {
		case buyFill <= 0 && sellFill <= 0:
			return "missing_buy_and_sell_book"
		case buyFill <= 0:
			return "missing_buy_book"
		case sellFill <= 0:
			return "missing_sell_book"
		default:
			return "no_executable_quantity"
		}
	}
	if fill >= requested {
		return ""
	}
	buyShort := buyFill < requested
	sellShort := sellFill < requested
	switch {
	case buyShort && sellShort:
		return "buy_and_sell_depth"
	case buyShort:
		return "buy_depth"
	case sellShort:
		return "sell_depth"
	default:
		return "partial_depth"
	}
}

func appendExecutionWarning(warnings []string, warning string) []string {
	if warning == "" {
		return warnings
	}
	for _, existing := range warnings {
		if existing == warning {
			return warnings
		}
	}
	return append(warnings, warning)
}

func executionQuoteDecision(requestedQty, fillQty int32, netProfit float64, warnings []string) string {
	if requestedQty <= 0 || fillQty <= 0 || netProfit <= 0 {
		return "DANGER"
	}
	if fillQty < requestedQty {
		return "CHANGED"
	}
	for _, warning := range warnings {
		switch warning {
		case "buy_order_cache_stale",
			"sell_order_cache_stale",
			"market_cache_age_high",
			"missing_packaged_volume",
			"structure_market_cache_age_unavailable":
			return "CHANGED"
		}
	}
	return "SAFE"
}

func quoteSide(regionID, systemID int32, locationID int64, plan ExecutionPlanResult, gross, fee float64) ExecutionQuoteSide {
	return ExecutionQuoteSide{
		RegionID:        regionID,
		SystemID:        systemID,
		LocationID:      locationID,
		VWAP:            plan.ExpectedPrice,
		BestPrice:       plan.BestPrice,
		GrossISK:        gross,
		FeeISK:          fee,
		FilledQty:       plan.VolumeFilled,
		CanFill:         plan.CanFill,
		TotalDepth:      plan.TotalDepth,
		SlippagePercent: plan.SlippagePercent,
		Plan:            plan,
	}
}

// ComputeExecutionQuote produces the shared buy+sell execution model used for
// "revalidate before undock" style decisions. Both sides are recomputed at the
// executable quantity so VWAP, fees, shipping, and ROI use one fill quantity.
func ComputeExecutionQuote(in ExecutionQuoteInput) ExecutionQuote {
	requested := in.RequestedQty
	if requested < 0 {
		requested = 0
	}
	packagedVolume := in.PackagedVolumeM3
	if packagedVolume < 0 {
		packagedVolume = 0
	}
	shippingRate := in.ShippingCostPerM3Jump
	if shippingRate < 0 {
		shippingRate = 0
	}
	shippingJumps := in.ShippingJumps
	if shippingJumps < 0 {
		shippingJumps = 0
	}

	quote := ExecutionQuote{
		TypeID:                in.TypeID,
		RequestedQty:          requested,
		PackagedVolumeM3:      packagedVolume,
		ShippingJumps:         shippingJumps,
		ShippingCostPerM3Jump: shippingRate,
		Decision:              "DANGER",
		Warnings:              append([]string(nil), in.Warnings...),
	}
	if requested <= 0 {
		quote.PartialReason = "no_requested_quantity"
		quote.Warnings = appendExecutionWarning(quote.Warnings, quote.PartialReason)
		return quote
	}

	buyInitial := ComputeExecutionPlan(in.BuyOrders, requested, true)
	sellInitial := ComputeExecutionPlan(in.SellOrders, requested, false)
	fillQty := minPositiveInt32(requested, buyInitial.VolumeFilled, sellInitial.VolumeFilled)
	quote.PartialReason = executionPartialReason(requested, fillQty, buyInitial.VolumeFilled, sellInitial.VolumeFilled)
	if quote.PartialReason != "" {
		quote.Warnings = appendExecutionWarning(quote.Warnings, quote.PartialReason)
	}
	if fillQty <= 0 {
		quote.Buy = quoteSide(in.BuyRegionID, in.BuySystemID, in.BuyLocationID, buyInitial, buyInitial.TotalCost, 0)
		quote.Sell = quoteSide(in.SellRegionID, in.SellSystemID, in.SellLocationID, sellInitial, sellInitial.TotalCost, 0)
		return quote
	}

	buyPlan := buyInitial
	sellPlan := sellInitial
	if fillQty != requested {
		buyPlan = ComputeExecutionPlan(in.BuyOrders, fillQty, true)
		sellPlan = ComputeExecutionPlan(in.SellOrders, fillQty, false)
	}

	buyCostMult, sellRevenueMult := tradeFeeMultipliers(tradeFeeInputs{
		SplitTradeFees:       in.Fees.SplitTradeFees,
		BrokerFeePercent:     in.Fees.BrokerFeePercent,
		SalesTaxPercent:      in.Fees.SalesTaxPercent,
		BuyBrokerFeePercent:  in.Fees.BuyBrokerFeePercent,
		SellBrokerFeePercent: in.Fees.SellBrokerFeePercent,
		BuySalesTaxPercent:   in.Fees.BuySalesTaxPercent,
		SellSalesTaxPercent:  in.Fees.SellSalesTaxPercent,
	})
	buyGross := buyPlan.TotalCost
	sellGross := sellPlan.TotalCost
	buyFees := buyGross * (buyCostMult - 1)
	if buyFees < 0 {
		buyFees = 0
	}
	sellFees := sellGross * (1 - sellRevenueMult)
	if sellFees < 0 {
		sellFees = 0
	}
	shippingCost := shippingRate * packagedVolume * float64(fillQty) * float64(shippingJumps)
	filledVolume := packagedVolume * float64(fillQty)
	buyNet := buyGross + buyFees
	sellNet := sellGross - sellFees
	netProfit := sellNet - buyNet - shippingCost
	deployed := buyNet + shippingCost
	roi := 0.0
	if deployed > 0 {
		roi = netProfit / deployed * 100
	}

	quote.FillQty = fillQty
	quote.BuyVWAP = buyPlan.ExpectedPrice
	quote.SellVWAP = sellPlan.ExpectedPrice
	quote.BuyGross = buyGross
	quote.SellGross = sellGross
	quote.BuyFees = buyFees
	quote.SellFees = sellFees
	quote.TotalFees = buyFees + sellFees
	quote.ShippingCost = shippingCost
	quote.FilledVolumeM3 = filledVolume
	quote.NetProfit = netProfit
	quote.ProfitPerUnit = netProfit / float64(fillQty)
	quote.ROIPercent = roi
	if netProfit <= 0 {
		quote.Warnings = appendExecutionWarning(quote.Warnings, "unprofitable_after_fees_shipping")
		if quote.PartialReason == "" {
			quote.PartialReason = "unprofitable_after_fees"
		}
	}
	if packagedVolume <= 0 {
		quote.Warnings = appendExecutionWarning(quote.Warnings, "missing_packaged_volume")
	}
	quote.Decision = executionQuoteDecision(requested, fillQty, netProfit, quote.Warnings)

	quote.Buy = quoteSide(in.BuyRegionID, in.BuySystemID, in.BuyLocationID, buyPlan, buyGross, buyFees)
	quote.Sell = quoteSide(in.SellRegionID, in.SellSystemID, in.SellLocationID, sellPlan, sellGross, sellFees)
	return quote
}
