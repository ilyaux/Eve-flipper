package engine

import (
	"math"
	"sort"
	"time"

	"eve-flipper/internal/esi"
)

// PortfolioOptimization is the response for the portfolio optimizer tab.
type PortfolioOptimization struct {
	Assets               []AssetStats            `json:"assets"`
	CorrelationMatrix    [][]float64             `json:"correlation_matrix"`
	CurrentWeights       []float64               `json:"current_weights"`
	OptimalWeights       []float64               `json:"optimal_weights"`
	MinVarWeights        []float64               `json:"min_var_weights"`
	EfficientFrontier    []FrontierPoint         `json:"efficient_frontier"`
	DiversificationRatio float64                 `json:"diversification_ratio"`
	CurrentSharpe        float64                 `json:"current_sharpe"`
	OptimalSharpe        float64                 `json:"optimal_sharpe"`
	MinVarSharpe         float64                 `json:"min_var_sharpe"`
	HHI                  float64                 `json:"hhi"` // Herfindahl-Hirschman Index (0-1)
	Suggestions          []AllocationSuggestion  `json:"suggestions"`
	OptimizerReady       bool                    `json:"optimizer_ready"`
	Diagnostic           *OptimizerDiagnostic    `json:"diagnostic,omitempty"`
	Capital              PortfolioCapital        `json:"capital"`
	PositionRisks        []PortfolioPositionRisk `json:"position_risks"`
	Warnings             []string                `json:"warnings,omitempty"`
}

// OptimizerDiagnostic is returned when optimization fails to help users understand why.
type OptimizerDiagnostic struct {
	TotalTransactions int              `json:"total_transactions"` // how many txns were in the input
	WithinLookback    int              `json:"within_lookback"`    // how many passed the date filter
	UniqueDays        int              `json:"unique_days"`        // how many distinct calendar days
	UniqueItems       int              `json:"unique_items"`       // how many distinct items
	QualifiedItems    int              `json:"qualified_items"`    // items with >= minOptimizerDays
	MinDaysRequired   int              `json:"min_days_required"`  // current threshold
	TopItems          []DiagnosticItem `json:"top_items"`          // top items by trading days
}

// DiagnosticItem shows a single item's stats for the diagnostic view.
type DiagnosticItem struct {
	TypeID       int32  `json:"type_id"`
	TypeName     string `json:"type_name"`
	TradingDays  int    `json:"trading_days"`
	Transactions int    `json:"transactions"`
}

// AssetStats describes a single tradeable item in the portfolio.
type AssetStats struct {
	TypeID        int32   `json:"type_id"`
	TypeName      string  `json:"type_name"`
	AvgDailyPnL   float64 `json:"avg_daily_pnl"`  // mean daily P&L in ISK
	Volatility    float64 `json:"volatility"`     // daily std dev of P&L
	SharpeRatio   float64 `json:"sharpe_ratio"`   // annualized
	CurrentWeight float64 `json:"current_weight"` // fraction of total capital
	TotalInvested float64 `json:"total_invested"`
	TotalPnL      float64 `json:"total_pnl"`
	TradingDays   int     `json:"trading_days"`
}

// FrontierPoint is a point on the efficient frontier.
type FrontierPoint struct {
	Risk   float64 `json:"risk"`   // portfolio std dev (daily)
	Return float64 `json:"return"` // portfolio expected daily return
}

// AllocationSuggestion recommends increasing or decreasing allocation to an item.
type AllocationSuggestion struct {
	TypeID     int32   `json:"type_id"`
	TypeName   string  `json:"type_name"`
	Action     string  `json:"action"` // "increase", "decrease", "hold"
	CurrentPct float64 `json:"current_pct"`
	OptimalPct float64 `json:"optimal_pct"`
	DeltaPct   float64 `json:"delta_pct"` // optimal - current
	Reason     string  `json:"reason"`
}

// PortfolioCapital describes deployable capital, open inventory, and order exposure.
type PortfolioCapital struct {
	WalletISK          float64  `json:"wallet_isk"`
	InventoryCostISK   float64  `json:"inventory_cost_isk"`
	InventoryMarkISK   float64  `json:"inventory_mark_isk"`
	ActiveBuyOrderISK  float64  `json:"active_buy_order_isk"`
	ActiveSellOrderISK float64  `json:"active_sell_order_isk"`
	UsedCapitalISK     float64  `json:"used_capital_isk"`
	TotalExposureISK   float64  `json:"total_exposure_isk"`
	EstimatedEquityISK float64  `json:"estimated_equity_isk"`
	FreeCapitalPct     float64  `json:"free_capital_pct"`
	LockedBuyPct       float64  `json:"locked_buy_pct"`
	InventoryPct       float64  `json:"inventory_pct"`
	SellBacklogPct     float64  `json:"sell_backlog_pct"`
	ConcentrationHHI   float64  `json:"concentration_hhi"`
	TopExposurePct     float64  `json:"top_exposure_pct"`
	RiskScore          float64  `json:"risk_score"`
	RiskLevel          string   `json:"risk_level"`
	Warnings           []string `json:"warnings,omitempty"`
}

// PortfolioPositionRisk is an item-level capital and risk recommendation.
type PortfolioPositionRisk struct {
	TypeID              int32   `json:"type_id"`
	TypeName            string  `json:"type_name"`
	InventoryQty        int64   `json:"inventory_qty"`
	AssetQty            int64   `json:"asset_qty"`
	AssetBacked         bool    `json:"asset_backed"`
	InventoryCostISK    float64 `json:"inventory_cost_isk"`
	InventoryMarkISK    float64 `json:"inventory_mark_isk"`
	InventorySource     string  `json:"inventory_source"`
	UnrealizedPnL       float64 `json:"unrealized_pnl"`
	UnrealizedROIPct    float64 `json:"unrealized_roi_pct"`
	ActiveBuyQty        int64   `json:"active_buy_qty"`
	ActiveBuyISK        float64 `json:"active_buy_isk"`
	ActiveSellQty       int64   `json:"active_sell_qty"`
	ActiveSellISK       float64 `json:"active_sell_isk"`
	RecentSellQty       int64   `json:"recent_sell_qty"`
	AvgDailySellQty     float64 `json:"avg_daily_sell_qty"`
	DaysToLiquidate     float64 `json:"days_to_liquidate"`
	RealizedPnL         float64 `json:"realized_pnl"`
	AvgDailyPnL         float64 `json:"avg_daily_pnl"`
	TradingDays         int     `json:"trading_days"`
	ExposureISK         float64 `json:"exposure_isk"`
	ExposurePct         float64 `json:"exposure_pct"`
	TargetPct           float64 `json:"target_pct"`
	DeltaPct            float64 `json:"delta_pct"`
	ConcentrationRisk   float64 `json:"concentration_risk"`
	LiquidityRisk       float64 `json:"liquidity_risk"`
	BacklogRisk         float64 `json:"backlog_risk"`
	LossRisk            float64 `json:"loss_risk"`
	StaleRisk           float64 `json:"stale_risk"`
	RiskScore           float64 `json:"risk_score"`
	RiskLevel           string  `json:"risk_level"`
	Action              string  `json:"action"`
	Reason              string  `json:"reason"`
	MaxCapitalISK       float64 `json:"max_capital_isk"`
	SuggestedBuyISK     float64 `json:"suggested_buy_isk"`
	SuggestedSellISK    float64 `json:"suggested_sell_isk"`
	MarkPrice           float64 `json:"mark_price"`
	MarkPriceSource     string  `json:"mark_price_source"`
	OldestInventoryDate string  `json:"oldest_inventory_date"`
}

const (
	// minOptimizerDays is the minimum number of days an item must have traded
	// to be included in the optimization. Lowered to 3 because the ESI wallet
	// transactions endpoint only returns a single page (~1000 recent entries)
	// which may span just a few calendar days for active traders.
	minOptimizerDays = 3
	// maxOptimizerAssets limits the optimization to the top N items by capital.
	maxOptimizerAssets = 20
	// frontierPoints is how many points to sample on the efficient frontier.
	frontierPoints = 30
)

// ComputePortfolioOptimization runs Markowitz mean-variance optimization
// on the player's trading portfolio derived from wallet transactions.
// Returns (result, nil) on success, or (nil, diagnostic) when there isn't enough data.
func ComputePortfolioOptimization(txns []esi.WalletTransaction, lookbackDays int) (*PortfolioOptimization, *OptimizerDiagnostic) {
	if len(txns) == 0 {
		return nil, &OptimizerDiagnostic{
			MinDaysRequired: minOptimizerDays,
		}
	}

	now := time.Now().UTC()
	cutoff := now.AddDate(0, 0, -lookbackDays)

	// Phase 1: Build per-item daily P&L series.
	// Key = typeID, value = map[dayString]pnl.
	type itemDayPnL struct {
		pnlByDay     map[string]float64
		totalBought  float64
		totalSold    float64
		typeName     string
		transactions int
	}
	items := make(map[int32]*itemDayPnL)
	allDays := make(map[string]bool)
	withinLookback := 0

	type optBuyLot struct {
		unitPrice float64
		remaining int32
	}
	buyQueues := make(map[int32][]optBuyLot)
	sortedTxns := make([]esi.WalletTransaction, len(txns))
	copy(sortedTxns, txns)
	sort.SliceStable(sortedTxns, func(i, j int) bool {
		if sortedTxns[i].Date == sortedTxns[j].Date {
			return sortedTxns[i].TransactionID < sortedTxns[j].TransactionID
		}
		return sortedTxns[i].Date < sortedTxns[j].Date
	})

	for _, tx := range sortedTxns {
		t, err := time.Parse(time.RFC3339, tx.Date)
		if err != nil {
			continue
		}
		if tx.IsBuy {
			if !t.Before(cutoff) {
				withinLookback++
				item, ok := items[tx.TypeID]
				if !ok {
					item = &itemDayPnL{
						pnlByDay: make(map[string]float64),
						typeName: tx.TypeName,
					}
					items[tx.TypeID] = item
				}
				item.transactions++
				item.totalBought += tx.UnitPrice * float64(tx.Quantity)
			}
			buyQueues[tx.TypeID] = append(buyQueues[tx.TypeID], optBuyLot{
				unitPrice: tx.UnitPrice,
				remaining: tx.Quantity,
			})
			continue
		}

		if !t.Before(cutoff) {
			withinLookback++
		}

		sellQty := tx.Quantity
		queue := buyQueues[tx.TypeID]
		for sellQty > 0 && len(queue) > 0 {
			lot := &queue[0]
			matched := lot.remaining
			if matched > sellQty {
				matched = sellQty
			}
			if !t.Before(cutoff) {
				day := t.Format("2006-01-02")
				item, ok := items[tx.TypeID]
				if !ok {
					item = &itemDayPnL{
						pnlByDay: make(map[string]float64),
						typeName: tx.TypeName,
					}
					items[tx.TypeID] = item
				}
				item.transactions++
				item.pnlByDay[day] += (tx.UnitPrice - lot.unitPrice) * float64(matched)
				item.totalSold += tx.UnitPrice * float64(matched)
				allDays[day] = true
			}
			lot.remaining -= matched
			sellQty -= matched
			if lot.remaining <= 0 {
				queue = queue[1:]
			}
		}
		buyQueues[tx.TypeID] = queue
	}

	// Sort all days.
	sortedDays := make([]string, 0, len(allDays))
	for d := range allDays {
		sortedDays = append(sortedDays, d)
	}
	sort.Strings(sortedDays)

	// Phase 2: Filter to items with enough trading days and select top N by capital.
	type assetCandidate struct {
		typeID       int32
		typeName     string
		totalBought  float64
		tradingDays  int
		dailyPnL     []float64
		dailyReturns []float64
	}
	var candidates []assetCandidate

	for typeID, item := range items {
		tradingDays := len(item.pnlByDay)
		if tradingDays < minOptimizerDays {
			continue
		}

		// Normalize daily cashflows by item capital scale so covariance and weights
		// are not dominated by absolute ISK size differences between items.
		capitalScale := math.Max(item.totalBought, item.totalSold)
		if capitalScale <= 0 {
			continue
		}

		// Build aligned daily series (using all days, 0 for days with no activity).
		dailyPnL := make([]float64, len(sortedDays))
		returns := make([]float64, len(sortedDays))
		for i, day := range sortedDays {
			if pnl, ok := item.pnlByDay[day]; ok {
				dailyPnL[i] = pnl
				returns[i] = pnl / capitalScale
			}
		}

		candidates = append(candidates, assetCandidate{
			typeID:       typeID,
			typeName:     item.typeName,
			totalBought:  item.totalBought,
			tradingDays:  tradingDays,
			dailyPnL:     dailyPnL,
			dailyReturns: returns,
		})
	}

	if len(candidates) < 2 {
		// Build diagnostic: show top items by trading days so the user understands what's happening.
		diag := &OptimizerDiagnostic{
			TotalTransactions: len(txns),
			WithinLookback:    withinLookback,
			UniqueDays:        len(allDays),
			UniqueItems:       len(items),
			QualifiedItems:    len(candidates),
			MinDaysRequired:   minOptimizerDays,
		}
		// Collect all items sorted by trading days (desc).
		type itemInfo struct {
			typeID       int32
			typeName     string
			tradingDays  int
			transactions int
		}
		var allItems []itemInfo
		for tid, item := range items {
			allItems = append(allItems, itemInfo{
				typeID:       tid,
				typeName:     item.typeName,
				tradingDays:  len(item.pnlByDay),
				transactions: item.transactions,
			})
		}
		sort.Slice(allItems, func(i, j int) bool {
			return allItems[i].tradingDays > allItems[j].tradingDays
		})
		limit := 10
		if len(allItems) < limit {
			limit = len(allItems)
		}
		for _, it := range allItems[:limit] {
			diag.TopItems = append(diag.TopItems, DiagnosticItem{
				TypeID:       it.typeID,
				TypeName:     it.typeName,
				TradingDays:  it.tradingDays,
				Transactions: it.transactions,
			})
		}
		return nil, diag
	}

	// Sort by total invested (descending) and take top N.
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].totalBought > candidates[j].totalBought
	})
	if len(candidates) > maxOptimizerAssets {
		candidates = candidates[:maxOptimizerAssets]
	}

	n := len(candidates)
	T := len(sortedDays)

	// Phase 3: Compute mean returns and covariance matrix.
	means := make([]float64, n)
	for i, c := range candidates {
		means[i] = mean(c.dailyReturns)
	}

	// Build returns matrix: n assets x T days.
	returnsMatrix := make([][]float64, n)
	for i, c := range candidates {
		returnsMatrix[i] = c.dailyReturns
	}

	// Covariance matrix with Ledoit-Wolf shrinkage for stability.
	covMatrix := ledoitWolfCov(returnsMatrix, means, T)

	// Correlation matrix for display.
	corrMatrix := make([][]float64, n)
	for i := 0; i < n; i++ {
		corrMatrix[i] = make([]float64, n)
		for j := 0; j < n; j++ {
			si := math.Sqrt(covMatrix[i][i])
			sj := math.Sqrt(covMatrix[j][j])
			if si > 0 && sj > 0 {
				corrMatrix[i][j] = covMatrix[i][j] / (si * sj)
			} else {
				corrMatrix[i][j] = 0
			}
			// Clamp to [-1, 1].
			if corrMatrix[i][j] > 1 {
				corrMatrix[i][j] = 1
			}
			if corrMatrix[i][j] < -1 {
				corrMatrix[i][j] = -1
			}
		}
	}

	// Phase 4: Current weights (by capital invested).
	totalCapital := 0.0
	for _, c := range candidates {
		totalCapital += c.totalBought
	}
	currentWeights := make([]float64, n)
	for i, c := range candidates {
		if totalCapital > 0 {
			currentWeights[i] = c.totalBought / totalCapital
		}
	}

	// Phase 5: Long-only optimization via projected gradient descent.
	// Properly solves the constrained QP: min w'Σw s.t. w >= 0, 1'w = 1
	// instead of the naive approach of solving unconstrained and clamping negatives.
	// The naive clamp-and-renormalize is not a valid QP projection and produces
	// suboptimal weights that may not lie on the efficient frontier.
	minVarWeights := solveLongOnlyMinVar(covMatrix)
	optimalWeights := solveLongOnlyMaxSharpe(means, covMatrix)

	// Phase 6: Compute portfolio metrics for each allocation.
	currentSharpe := portfolioSharpe(currentWeights, means, covMatrix)
	optimalSharpe := portfolioSharpe(optimalWeights, means, covMatrix)
	minVarSharpe := portfolioSharpe(minVarWeights, means, covMatrix)

	// Diversification ratio: weighted avg vol / portfolio vol.
	divRatio := 0.0
	portVar := portfolioVariance(currentWeights, covMatrix)
	if portVar > 0 {
		weightedAvgVol := 0.0
		for i := 0; i < n; i++ {
			weightedAvgVol += currentWeights[i] * math.Sqrt(covMatrix[i][i])
		}
		divRatio = weightedAvgVol / math.Sqrt(portVar)
	}

	// HHI (Herfindahl-Hirschman Index): sum of squared weights. 1/n = perfectly diversified.
	hhi := 0.0
	for _, w := range currentWeights {
		hhi += w * w
	}

	// Scale return-space risk/return metrics to ISK/day using current deployed capital.
	// This keeps frontend charts in familiar units while optimization itself remains
	// based on normalized returns.
	referenceCapital := totalCapital
	if referenceCapital <= 0 {
		referenceCapital = 1
	}

	// Phase 7: Long-only efficient frontier.
	// Computed by solving the constrained QP at each target return level,
	// ensuring all plotted points are achievable with long-only portfolios.
	frontier := computeLongOnlyFrontier(means, covMatrix, frontierPoints)
	for i := range frontier {
		frontier[i].Risk *= referenceCapital
		frontier[i].Return *= referenceCapital
	}

	// Phase 8: Build asset stats.
	assetStats := make([]AssetStats, n)
	for i, c := range candidates {
		volRet := math.Sqrt(variance(c.dailyReturns))
		sr := 0.0
		if volRet > 0 {
			sr = (means[i] / volRet) * math.Sqrt(365)
		}
		avgDailyPnL := means[i] * referenceCapital
		vol := volRet * referenceCapital
		totalPnL := 0.0
		for _, pnl := range c.dailyPnL {
			totalPnL += pnl
		}
		assetStats[i] = AssetStats{
			TypeID:        c.typeID,
			TypeName:      c.typeName,
			AvgDailyPnL:   avgDailyPnL,
			Volatility:    vol,
			SharpeRatio:   sr,
			CurrentWeight: currentWeights[i],
			TotalInvested: c.totalBought,
			TotalPnL:      totalPnL,
			TradingDays:   c.tradingDays,
		}
	}

	// Phase 9: Generate suggestions.
	suggestions := generateSuggestions(assetStats, currentWeights, optimalWeights)

	return &PortfolioOptimization{
		Assets:               assetStats,
		CorrelationMatrix:    corrMatrix,
		CurrentWeights:       currentWeights,
		OptimalWeights:       optimalWeights,
		MinVarWeights:        minVarWeights,
		EfficientFrontier:    frontier,
		DiversificationRatio: divRatio,
		CurrentSharpe:        currentSharpe,
		OptimalSharpe:        optimalSharpe,
		MinVarSharpe:         minVarSharpe,
		HHI:                  hhi,
		Suggestions:          suggestions,
		OptimizerReady:       true,
	}, nil
}

// ComputePortfolioOptimizationWithContext adds active-order and inventory-aware
// capital/risk recommendations around the historical optimizer.
func ComputePortfolioOptimizationWithContext(txns []esi.WalletTransaction, orders []esi.CharacterOrder, walletISK float64, lookbackDays int) *PortfolioOptimization {
	return ComputePortfolioOptimizationWithRuntime(txns, orders, nil, walletISK, lookbackDays, false)
}

// ComputePortfolioOptimizationWithRuntime adds active-order, wallet, and
// asset-snapshot context around the historical optimizer. When
// assetSnapshotComplete is true, ESI assets are treated as the authoritative
// current inventory for items already present in the trading portfolio.
func ComputePortfolioOptimizationWithRuntime(txns []esi.WalletTransaction, orders []esi.CharacterOrder, assets []esi.CharacterAsset, walletISK float64, lookbackDays int, assetSnapshotComplete bool) *PortfolioOptimization {
	result, diag := ComputePortfolioOptimization(txns, lookbackDays)
	if result == nil {
		result = &PortfolioOptimization{
			Assets:            []AssetStats{},
			CorrelationMatrix: [][]float64{},
			CurrentWeights:    []float64{},
			OptimalWeights:    []float64{},
			MinVarWeights:     []float64{},
			EfficientFrontier: []FrontierPoint{},
			Suggestions:       []AllocationSuggestion{},
			OptimizerReady:    false,
			Diagnostic:        diag,
		}
	} else {
		result.OptimizerReady = true
	}

	capital, positions, warnings := buildPortfolioCapitalRisk(txns, orders, assets, walletISK, lookbackDays, result, assetSnapshotComplete)
	result.Capital = capital
	result.PositionRisks = positions
	result.Warnings = append(result.Warnings, warnings...)
	return result
}

type portfolioPositionAgg struct {
	typeID          int32
	typeName        string
	inventoryQty    int64
	inventoryCost   float64
	oldestInventory string
	assetQty        int64
	assetKnown      bool
	assetReconciled bool
	costEstimated   bool
	activeBuyQty    int64
	activeBuyISK    float64
	activeSellQty   int64
	activeSellISK   float64
	recentSellQty   int64
	recentSellISK   float64
	sellDays        map[string]bool
	realizedPnL     float64
	avgDailyPnL     float64
	tradingDays     int
	targetPct       float64
}

func buildPortfolioCapitalRisk(txns []esi.WalletTransaction, orders []esi.CharacterOrder, assets []esi.CharacterAsset, walletISK float64, lookbackDays int, opt *PortfolioOptimization, assetSnapshotComplete bool) (PortfolioCapital, []PortfolioPositionRisk, []string) {
	if lookbackDays <= 0 {
		lookbackDays = 90
	}
	if lookbackDays > 365 {
		lookbackDays = 365
	}

	pnl := ComputePortfolioPnLWithOptions(txns, PortfolioPnLOptions{
		LookbackDays:         lookbackDays,
		LedgerLimit:          0,
		IncludeUnmatchedSell: false,
	})

	positions := make(map[int32]*portfolioPositionAgg)
	get := func(typeID int32, typeName string) *portfolioPositionAgg {
		if typeID <= 0 {
			return nil
		}
		p := positions[typeID]
		if p == nil {
			p = &portfolioPositionAgg{typeID: typeID, typeName: typeName, sellDays: make(map[string]bool)}
			positions[typeID] = p
		}
		if p.typeName == "" && typeName != "" {
			p.typeName = typeName
		}
		return p
	}

	for _, pos := range pnl.OpenPositions {
		p := get(pos.TypeID, pos.TypeName)
		if p == nil {
			continue
		}
		p.inventoryQty += pos.Quantity
		p.inventoryCost += pos.CostBasis
		if p.oldestInventory == "" || (pos.OldestLotDate != "" && pos.OldestLotDate < p.oldestInventory) {
			p.oldestInventory = pos.OldestLotDate
		}
	}
	for _, item := range pnl.TopItems {
		p := get(item.TypeID, item.TypeName)
		if p == nil {
			continue
		}
		p.realizedPnL = item.NetPnL
		p.tradingDays = item.Transactions
		if lookbackDays > 0 {
			p.avgDailyPnL = item.NetPnL / float64(lookbackDays)
		}
	}

	now := time.Now().UTC()
	cutoff := now.AddDate(0, 0, -lookbackDays)
	for _, tx := range txns {
		t, err := time.Parse(time.RFC3339, tx.Date)
		if err != nil || t.Before(cutoff) || tx.IsBuy || tx.Quantity <= 0 {
			continue
		}
		p := get(tx.TypeID, tx.TypeName)
		if p == nil {
			continue
		}
		p.recentSellQty += int64(tx.Quantity)
		p.recentSellISK += tx.UnitPrice * float64(tx.Quantity)
		p.sellDays[t.Format("2006-01-02")] = true
	}

	var activeBuyISK, activeSellISK float64
	for _, order := range orders {
		if order.TypeID <= 0 || order.VolumeRemain <= 0 || order.Price <= 0 {
			continue
		}
		p := get(order.TypeID, order.TypeName)
		if p == nil {
			continue
		}
		notional := order.Price * float64(order.VolumeRemain)
		if order.IsBuyOrder {
			p.activeBuyQty += int64(order.VolumeRemain)
			p.activeBuyISK += notional
			activeBuyISK += notional
		} else {
			p.activeSellQty += int64(order.VolumeRemain)
			p.activeSellISK += notional
			activeSellISK += notional
		}
	}

	if assetSnapshotComplete {
		assetQtyByType := make(map[int32]int64)
		for _, asset := range assets {
			qty := portfolioAssetInventoryQty(asset)
			if qty <= 0 {
				continue
			}
			assetQtyByType[asset.TypeID] += qty
		}
		for _, p := range positions {
			p.assetKnown = true
			p.assetQty = assetQtyByType[p.typeID]
			reconcilePortfolioInventoryWithAssets(p)
		}
	}

	targetByType := make(map[int32]float64)
	if opt != nil && len(opt.Assets) == len(opt.OptimalWeights) {
		for i, asset := range opt.Assets {
			targetByType[asset.TypeID] = opt.OptimalWeights[i] * 100
			if p := positions[asset.TypeID]; p != nil {
				p.avgDailyPnL = asset.AvgDailyPnL
				p.tradingDays = asset.TradingDays
			}
		}
	}
	for typeID, pct := range targetByType {
		if p := positions[typeID]; p != nil {
			p.targetPct = pct
		}
	}

	rows := make([]PortfolioPositionRisk, 0, len(positions))
	var inventoryCost, inventoryMark, totalExposure float64
	var assetReconciled, assetZeroed, assetCostEstimated bool
	for _, p := range positions {
		if p == nil {
			continue
		}
		markPrice, source := portfolioMarkPrice(p)
		positionMark := markPrice * float64(p.inventoryQty)
		if positionMark <= 0 && p.inventoryCost <= 0 && p.activeSellISK > 0 {
			positionMark = p.activeSellISK
			markPrice = 0
			if p.activeSellQty > 0 {
				markPrice = p.activeSellISK / float64(p.activeSellQty)
			}
			source = "active_sell"
		}
		if p.inventoryQty > 0 && p.inventoryCost <= 0 && positionMark > 0 {
			p.inventoryCost = positionMark
			p.costEstimated = true
		}
		inventoryExposure := math.Max(p.inventoryCost, positionMark)
		inventoryExposure = math.Max(inventoryExposure, p.activeSellISK)
		exposure := inventoryExposure + p.activeBuyISK
		if exposure <= 0 && p.activeBuyISK > 0 {
			exposure = p.activeBuyISK
		}
		inventoryCost += p.inventoryCost
		inventoryMark += positionMark
		totalExposure += exposure
		if p.assetReconciled {
			assetReconciled = true
			if p.assetQty == 0 {
				assetZeroed = true
			}
		}
		if p.costEstimated {
			assetCostEstimated = true
		}
		rows = append(rows, PortfolioPositionRisk{
			TypeID:              p.typeID,
			TypeName:            p.typeName,
			InventoryQty:        p.inventoryQty,
			AssetQty:            p.assetQty,
			AssetBacked:         p.assetKnown,
			InventoryCostISK:    p.inventoryCost,
			InventoryMarkISK:    positionMark,
			InventorySource:     portfolioInventorySource(p),
			UnrealizedPnL:       positionMark - p.inventoryCost,
			ActiveBuyQty:        p.activeBuyQty,
			ActiveBuyISK:        p.activeBuyISK,
			ActiveSellQty:       p.activeSellQty,
			ActiveSellISK:       p.activeSellISK,
			RecentSellQty:       p.recentSellQty,
			AvgDailySellQty:     avgDailySellQty(p),
			RealizedPnL:         p.realizedPnL,
			AvgDailyPnL:         p.avgDailyPnL,
			TradingDays:         p.tradingDays,
			ExposureISK:         exposure,
			TargetPct:           p.targetPct,
			MarkPrice:           markPrice,
			MarkPriceSource:     source,
			OldestInventoryDate: p.oldestInventory,
		})
	}

	for i := range rows {
		if rows[i].InventoryCostISK > 0 {
			rows[i].UnrealizedROIPct = rows[i].UnrealizedPnL / rows[i].InventoryCostISK * 100
		}
		if rows[i].AvgDailySellQty > 0 {
			rows[i].DaysToLiquidate = float64(rows[i].InventoryQty+rows[i].ActiveSellQty) / rows[i].AvgDailySellQty
		} else if rows[i].InventoryQty+rows[i].ActiveSellQty > 0 {
			rows[i].DaysToLiquidate = 999
		}
		if totalExposure > 0 {
			rows[i].ExposurePct = rows[i].ExposureISK / totalExposure * 100
		}
		rows[i].DeltaPct = rows[i].TargetPct - rows[i].ExposurePct
		scorePortfolioPositionRisk(&rows[i], walletISK, totalExposure)
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].RiskScore == rows[j].RiskScore {
			return rows[i].ExposureISK > rows[j].ExposureISK
		}
		return rows[i].RiskScore > rows[j].RiskScore
	})

	capital := PortfolioCapital{
		WalletISK:          walletISK,
		InventoryCostISK:   inventoryCost,
		InventoryMarkISK:   inventoryMark,
		ActiveBuyOrderISK:  activeBuyISK,
		ActiveSellOrderISK: activeSellISK,
		UsedCapitalISK:     math.Max(inventoryCost, inventoryMark) + activeBuyISK,
		TotalExposureISK:   totalExposure,
		EstimatedEquityISK: walletISK + inventoryMark,
		Warnings:           []string{},
	}
	if capital.EstimatedEquityISK <= 0 {
		capital.EstimatedEquityISK = walletISK + inventoryCost
	}
	denom := walletISK + capital.UsedCapitalISK
	if denom > 0 {
		capital.FreeCapitalPct = walletISK / denom * 100
		capital.LockedBuyPct = activeBuyISK / denom * 100
		capital.InventoryPct = inventoryCost / denom * 100
	}
	if totalExposure > 0 {
		var hhi float64
		for _, row := range rows {
			w := row.ExposureISK / totalExposure
			hhi += w * w
			if row.ExposurePct > capital.TopExposurePct {
				capital.TopExposurePct = row.ExposurePct
			}
		}
		capital.ConcentrationHHI = hhi
		capital.SellBacklogPct = activeSellISK / totalExposure * 100
	}
	capital.RiskScore = portfolioCapitalRiskScore(capital, rows)
	capital.RiskLevel = portfolioRiskLevel(capital.RiskScore)
	if capital.TopExposurePct > 45 {
		capital.Warnings = append(capital.Warnings, "single_item_concentration")
	}
	if capital.LockedBuyPct > 50 {
		capital.Warnings = append(capital.Warnings, "buy_orders_lock_most_capital")
	}
	if capital.SellBacklogPct > 80 {
		capital.Warnings = append(capital.Warnings, "large_sell_backlog")
	}
	if assetReconciled {
		capital.Warnings = append(capital.Warnings, "asset_inventory_reconciled")
	}
	if assetZeroed {
		capital.Warnings = append(capital.Warnings, "stale_txn_inventory_absent_from_assets")
	}
	if assetCostEstimated {
		capital.Warnings = append(capital.Warnings, "asset_cost_basis_estimated")
	}

	warnings := make([]string, 0, len(capital.Warnings))
	warnings = append(warnings, capital.Warnings...)
	return capital, rows, warnings
}

func portfolioAssetInventoryQty(asset esi.CharacterAsset) int64 {
	if asset.TypeID <= 0 || asset.IsBlueprintCopy {
		return 0
	}
	if asset.Quantity > 0 {
		return asset.Quantity
	}
	if asset.IsSingleton {
		return 1
	}
	return 0
}

func reconcilePortfolioInventoryWithAssets(p *portfolioPositionAgg) {
	if p == nil || !p.assetKnown {
		return
	}
	txnQty := p.inventoryQty
	txnCost := p.inventoryCost
	if txnQty == p.assetQty {
		return
	}
	p.assetReconciled = true
	p.inventoryQty = p.assetQty
	if p.assetQty <= 0 {
		p.inventoryCost = 0
		p.oldestInventory = ""
		return
	}
	if txnQty > 0 && txnCost > 0 {
		p.inventoryCost = txnCost / float64(txnQty) * float64(p.assetQty)
		return
	}
	p.inventoryCost = 0
	p.costEstimated = true
}

func portfolioInventorySource(p *portfolioPositionAgg) string {
	if p == nil {
		return ""
	}
	if p.assetKnown {
		if p.assetReconciled {
			if p.assetQty <= 0 {
				return "assets_zero"
			}
			if p.costEstimated {
				return "assets_estimated_cost"
			}
			return "assets"
		}
		if p.assetQty > 0 {
			return "assets_match"
		}
		return "assets_zero"
	}
	if p.inventoryQty > 0 {
		return "transactions"
	}
	if p.activeSellQty > 0 {
		return "active_sell"
	}
	if p.activeBuyQty > 0 {
		return "active_buy"
	}
	return ""
}

func portfolioMarkPrice(p *portfolioPositionAgg) (float64, string) {
	if p == nil {
		return 0, ""
	}
	if p.activeSellQty > 0 && p.activeSellISK > 0 {
		return p.activeSellISK / float64(p.activeSellQty), "active_sell"
	}
	if p.recentSellQty > 0 && p.recentSellISK > 0 {
		return p.recentSellISK / float64(p.recentSellQty), "recent_sell"
	}
	if p.inventoryQty > 0 && p.inventoryCost > 0 {
		return p.inventoryCost / float64(p.inventoryQty), "cost"
	}
	return 0, ""
}

func avgDailySellQty(p *portfolioPositionAgg) float64 {
	if p == nil || p.recentSellQty <= 0 {
		return 0
	}
	days := len(p.sellDays)
	if days <= 0 {
		days = 1
	}
	return float64(p.recentSellQty) / float64(days)
}

func scorePortfolioPositionRisk(row *PortfolioPositionRisk, walletISK, totalExposure float64) {
	if row == nil {
		return
	}
	row.ConcentrationRisk = clampFloat(row.ExposurePct/35*100, 0, 100)
	if row.DaysToLiquidate >= 999 {
		row.LiquidityRisk = 100
	} else {
		row.LiquidityRisk = clampFloat(row.DaysToLiquidate/14*100, 0, 100)
	}
	if row.AvgDailySellQty > 0 {
		row.BacklogRisk = clampFloat((float64(row.ActiveSellQty)/row.AvgDailySellQty)/7*100, 0, 100)
	} else if row.ActiveSellQty > 0 {
		row.BacklogRisk = 80
	}
	if row.UnrealizedROIPct < 0 {
		row.LossRisk = clampFloat(math.Abs(row.UnrealizedROIPct)/30*100, 0, 100)
	}
	if row.RealizedPnL < 0 && row.InventoryCostISK > 0 {
		row.LossRisk = math.Max(row.LossRisk, clampFloat(math.Abs(row.RealizedPnL)/row.InventoryCostISK*100, 0, 100))
	}
	row.StaleRisk = staleInventoryRisk(row.OldestInventoryDate)
	row.RiskScore = clampFloat(
		row.ConcentrationRisk*0.35+
			row.LiquidityRisk*0.25+
			row.BacklogRisk*0.20+
			row.LossRisk*0.15+
			row.StaleRisk*0.05,
		0,
		100,
	)
	row.RiskLevel = portfolioRiskLevel(row.RiskScore)

	row.Action = "hold"
	row.Reason = "balanced"
	switch {
	case row.LossRisk >= 60 && row.LiquidityRisk >= 60:
		row.Action = "liquidate"
		row.Reason = "negative_slow_inventory"
	case row.ConcentrationRisk >= 80:
		row.Action = "reduce"
		row.Reason = "over_concentrated"
	case row.BacklogRisk >= 70 && row.ActiveBuyISK > 0:
		row.Action = "pause_buy"
		row.Reason = "sell_backlog"
	case row.DeltaPct < -5:
		row.Action = "reduce"
		row.Reason = "above_target"
	case row.DeltaPct > 5 && row.RiskScore < 45 && row.AvgDailyPnL >= 0:
		row.Action = "increase"
		row.Reason = "below_target_good_risk"
	case row.LiquidityRisk >= 80:
		row.Action = "reduce"
		row.Reason = "slow_liquidation"
	case row.LossRisk >= 55:
		row.Action = "reduce"
		row.Reason = "negative_pnl"
	}

	if row.TargetPct > 0 && totalExposure > 0 {
		row.MaxCapitalISK = totalExposure * row.TargetPct / 100
	}
	switch row.Action {
	case "increase":
		deltaISK := math.Max(row.MaxCapitalISK-row.ExposureISK, 0)
		if walletISK > 0 {
			deltaISK = math.Min(deltaISK, walletISK*0.30)
		}
		row.SuggestedBuyISK = deltaISK
	case "reduce", "liquidate", "pause_buy":
		if row.MaxCapitalISK > 0 {
			row.SuggestedSellISK = math.Max(row.ExposureISK-row.MaxCapitalISK, 0)
		}
		if row.Action == "liquidate" || row.SuggestedSellISK <= 0 {
			row.SuggestedSellISK = math.Max(row.InventoryMarkISK, row.ActiveSellISK)
		}
	}
}

func staleInventoryRisk(oldest string) float64 {
	if oldest == "" {
		return 0
	}
	t, err := time.Parse("2006-01-02", oldest)
	if err != nil {
		return 0
	}
	ageDays := time.Since(t).Hours() / 24
	return clampFloat((ageDays-14)/46*100, 0, 100)
}

func portfolioCapitalRiskScore(cap PortfolioCapital, rows []PortfolioPositionRisk) float64 {
	score := 0.0
	score += clampFloat(cap.TopExposurePct/45*100, 0, 100) * 0.40
	score += clampFloat(cap.LockedBuyPct/50*100, 0, 100) * 0.20
	score += clampFloat(cap.SellBacklogPct/80*100, 0, 100) * 0.20
	maxRowRisk := 0.0
	for _, row := range rows {
		if row.RiskScore > maxRowRisk {
			maxRowRisk = row.RiskScore
		}
	}
	score += maxRowRisk * 0.20
	return clampFloat(score, 0, 100)
}

func portfolioRiskLevel(score float64) string {
	switch {
	case score >= 70:
		return "high"
	case score >= 35:
		return "medium"
	default:
		return "low"
	}
}

func clampFloat(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

// --- Linear algebra utilities for small matrices ---

// ledoitWolfCov computes a shrinkage covariance matrix using the oracle
// approximating shrinkage estimator from Ledoit & Wolf (2004).
// Target: μ̄·I (scaled identity with average variance on the diagonal).
// Shrinkage intensity: α* = min(β̂²/δ̂², 1), the data-driven optimal.
//
// Reference: O. Ledoit, M. Wolf, "A well-conditioned estimator for
// large-dimensional covariance matrices", J. Multivariate Analysis (2004).
func ledoitWolfCov(returns [][]float64, means []float64, T int) [][]float64 {
	n := len(returns)

	// Step 1: Sample covariance matrix S (unbiased, Bessel's correction).
	sample := make([][]float64, n)
	for i := 0; i < n; i++ {
		sample[i] = make([]float64, n)
		for j := 0; j <= i; j++ {
			cov := 0.0
			for t := 0; t < T; t++ {
				cov += (returns[i][t] - means[i]) * (returns[j][t] - means[j])
			}
			if T > 1 {
				cov /= float64(T - 1)
			}
			sample[i][j] = cov
			sample[j][i] = cov
		}
	}

	// Step 2: Shrinkage target F = μ̄·I (average variance on diagonal).
	avgVar := 0.0
	for i := 0; i < n; i++ {
		avgVar += sample[i][i]
	}
	avgVar /= float64(n)

	// Step 3: Compute optimal shrinkage intensity (Ledoit-Wolf 2004).
	// δ² = ||S − F||²_F  (squared Frobenius distance from sample to target)
	dSq := 0.0
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			targetIJ := 0.0
			if i == j {
				targetIJ = avgVar
			}
			diff := sample[i][j] - targetIJ
			dSq += diff * diff
		}
	}

	// β̂² = (1/T²) Σ_k ||z_k z_k' − S||²_F
	// where z_k = centered observation vector for period k.
	// This estimates the total squared estimation error of S.
	bSq := 0.0
	for k := 0; k < T; k++ {
		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				diff := (returns[i][k]-means[i])*(returns[j][k]-means[j]) - sample[i][j]
				bSq += diff * diff
			}
		}
	}
	bSq /= float64(T) * float64(T)

	// α* = min(β̂²/δ², 1)
	alpha := 0.0
	if dSq > 1e-15 {
		alpha = bSq / dSq
	}
	if alpha > 1 {
		alpha = 1
	}

	// Step 4: Shrunk covariance Σ̂ = (1−α)·S + α·F
	shrunk := make([][]float64, n)
	for i := 0; i < n; i++ {
		shrunk[i] = make([]float64, n)
		for j := 0; j < n; j++ {
			shrunk[i][j] = (1 - alpha) * sample[i][j]
			if i == j {
				shrunk[i][j] += alpha * avgVar
			}
		}
	}

	return shrunk
}

// invertMatrix inverts a square matrix using Gauss-Jordan elimination.
// Returns nil if the matrix is singular.
func invertMatrix(m [][]float64) [][]float64 {
	n := len(m)
	if n == 0 {
		return nil
	}

	// Augmented matrix [M | I].
	aug := make([][]float64, n)
	for i := 0; i < n; i++ {
		aug[i] = make([]float64, 2*n)
		for j := 0; j < n; j++ {
			aug[i][j] = m[i][j]
		}
		aug[i][n+i] = 1
	}

	// Forward elimination with partial pivoting.
	for col := 0; col < n; col++ {
		// Find pivot.
		maxVal := math.Abs(aug[col][col])
		maxRow := col
		for row := col + 1; row < n; row++ {
			if math.Abs(aug[row][col]) > maxVal {
				maxVal = math.Abs(aug[row][col])
				maxRow = row
			}
		}
		if maxVal < 1e-12 {
			return nil // singular
		}
		// Swap rows.
		aug[col], aug[maxRow] = aug[maxRow], aug[col]

		// Scale pivot row.
		scale := aug[col][col]
		for j := 0; j < 2*n; j++ {
			aug[col][j] /= scale
		}

		// Eliminate column.
		for row := 0; row < n; row++ {
			if row == col {
				continue
			}
			factor := aug[row][col]
			for j := 0; j < 2*n; j++ {
				aug[row][j] -= factor * aug[col][j]
			}
		}
	}

	// Extract inverse.
	inv := make([][]float64, n)
	for i := 0; i < n; i++ {
		inv[i] = make([]float64, n)
		for j := 0; j < n; j++ {
			inv[i][j] = aug[i][n+j]
		}
	}
	return inv
}

func identityMatrix(n int) [][]float64 {
	m := make([][]float64, n)
	for i := 0; i < n; i++ {
		m[i] = make([]float64, n)
		m[i][i] = 1
	}
	return m
}

func matVecMul(m [][]float64, v []float64) []float64 {
	n := len(m)
	result := make([]float64, n)
	for i := 0; i < n; i++ {
		for j := 0; j < len(v); j++ {
			result[i] += m[i][j] * v[j]
		}
	}
	return result
}

func dotProduct(a, b []float64) float64 {
	sum := 0.0
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}

// projectOntoSimplex projects vector v onto the probability simplex
// Δ = {x ∈ ℝⁿ : x ≥ 0, Σxᵢ = 1} using the exact O(n log n) algorithm
// from Duchi et al. (2008), "Efficient projections onto the l1-ball".
// Modifies v in place.
func projectOntoSimplex(v []float64) {
	n := len(v)
	if n == 0 {
		return
	}

	// Sort a copy in descending order.
	u := make([]float64, n)
	copy(u, v)
	sort.Float64s(u)
	// Reverse to descending.
	for i, j := 0, n-1; i < j; i, j = i+1, j-1 {
		u[i], u[j] = u[j], u[i]
	}

	// Find ρ: largest index j (1-based) such that u[j] - (Σ_{i=1..j} u[i] - 1)/j > 0.
	cumSum := 0.0
	rho := 0
	for j := 0; j < n; j++ {
		cumSum += u[j]
		if u[j]-(cumSum-1)/float64(j+1) > 0 {
			rho = j
		}
	}

	// Threshold θ.
	cumSum = 0
	for j := 0; j <= rho; j++ {
		cumSum += u[j]
	}
	theta := (cumSum - 1) / float64(rho+1)

	// Project.
	for i := range v {
		v[i] -= theta
		if v[i] < 0 {
			v[i] = 0
		}
	}
}

// solveLongOnlyMinVar finds the minimum-variance portfolio with long-only constraints
// by solving: min w'Σw  s.t. w ≥ 0, 1'w = 1
// using projected gradient descent onto the probability simplex.
// For n ≤ 20 (maxOptimizerAssets), this converges in well under 1 ms.
func solveLongOnlyMinVar(cov [][]float64) []float64 {
	n := len(cov)
	if n == 0 {
		return nil
	}

	// Initial weights: equal (feasible point on simplex).
	w := make([]float64, n)
	for i := range w {
		w[i] = 1.0 / float64(n)
	}

	// Step size: 1 / (2·trace(Σ)). Conservative upper bound since
	// the Lipschitz constant of ∇(w'Σw) = 2Σw is L = 2·λ_max(Σ) ≤ 2·trace(Σ).
	trace := 0.0
	for i := 0; i < n; i++ {
		trace += cov[i][i]
	}
	if trace <= 0 {
		return w
	}
	stepSize := 1.0 / (2 * trace)

	const maxIter = 1000
	const tol = 1e-10

	for iter := 0; iter < maxIter; iter++ {
		// Gradient of w'Σw is 2·Σ·w.
		grad := matVecMul(cov, w)

		prevW := make([]float64, n)
		copy(prevW, w)
		for i := range w {
			w[i] -= stepSize * 2 * grad[i]
		}

		// Project onto probability simplex.
		projectOntoSimplex(w)

		// Check convergence: max|w_new - w_old|.
		maxDiff := 0.0
		for i := range w {
			d := math.Abs(w[i] - prevW[i])
			if d > maxDiff {
				maxDiff = d
			}
		}
		if maxDiff < tol {
			break
		}
	}

	return w
}

// solveLongOnlyMaxSharpe finds the maximum Sharpe ratio (tangency) portfolio
// with long-only constraints. Scans the risk-aversion parameter λ ≥ 0 and
// for each solves: min w'Σw − λ·μ'w  s.t. w ≥ 0, 1'w = 1, then picks the
// solution with the highest Sharpe ratio. This is a robust approach for small n.
func solveLongOnlyMaxSharpe(means []float64, cov [][]float64) []float64 {
	n := len(means)
	if n == 0 {
		return nil
	}

	bestSharpe := -math.MaxFloat64
	var bestW []float64

	// Scan λ from 0 (min-variance) to large (max-return emphasis).
	// Logarithmic spacing gives good coverage of the efficient frontier.
	const numScans = 50
	for k := 0; k <= numScans; k++ {
		var lambda float64
		if k == 0 {
			lambda = 0
		} else {
			t := float64(k) / float64(numScans)
			lambda = 0.001 * math.Pow(100000, t) // 0.001 to 100
		}

		w := solveLongOnlyQP(means, cov, lambda)
		sr := portfolioSharpe(w, means, cov)
		if sr > bestSharpe {
			bestSharpe = sr
			bestW = w
		}
	}

	if bestW == nil {
		bestW = make([]float64, n)
		for i := range bestW {
			bestW[i] = 1.0 / float64(n)
		}
	}

	return bestW
}

// solveLongOnlyQP solves: min w'Σw − λ·μ'w  s.t. w ≥ 0, 1'w = 1
// via projected gradient descent onto the simplex. The parameter λ controls
// the tradeoff between variance minimization and return maximization.
func solveLongOnlyQP(means []float64, cov [][]float64, lambda float64) []float64 {
	n := len(cov)
	if n == 0 {
		return nil
	}

	w := make([]float64, n)
	for i := range w {
		w[i] = 1.0 / float64(n)
	}

	// Lipschitz constant of ∇f = 2Σw − λμ is 2·λ_max(Σ) ≤ 2·trace(Σ).
	// The linear term −λμ has zero Hessian, so the Lipschitz constant is unchanged.
	trace := 0.0
	for i := 0; i < n; i++ {
		trace += cov[i][i]
	}
	if trace <= 0 {
		return w
	}
	stepSize := 1.0 / (2 * trace)

	const maxIter = 1000
	const tol = 1e-10

	for iter := 0; iter < maxIter; iter++ {
		grad := matVecMul(cov, w)

		prevW := make([]float64, n)
		copy(prevW, w)
		for i := range w {
			w[i] -= stepSize * (2*grad[i] - lambda*means[i])
		}

		projectOntoSimplex(w)

		maxDiff := 0.0
		for i := range w {
			d := math.Abs(w[i] - prevW[i])
			if d > maxDiff {
				maxDiff = d
			}
		}
		if maxDiff < tol {
			break
		}
	}

	return w
}

func portfolioVariance(w []float64, cov [][]float64) float64 {
	n := len(w)
	v := 0.0
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			v += w[i] * w[j] * cov[i][j]
		}
	}
	return v
}

func portfolioReturn(w, means []float64) float64 {
	r := 0.0
	for i := range w {
		r += w[i] * means[i]
	}
	return r
}

func portfolioSharpe(w, means []float64, cov [][]float64) float64 {
	ret := portfolioReturn(w, means)
	vol := math.Sqrt(portfolioVariance(w, cov))
	if vol <= 0 {
		return 0
	}
	return (ret / vol) * math.Sqrt(365)
}

// computeLongOnlyFrontier traces the efficient frontier for long-only portfolios
// by scanning the risk-aversion parameter λ and solving the constrained QP
// at each level. This ensures every plotted point is achievable without shorting.
func computeLongOnlyFrontier(means []float64, cov [][]float64, numPoints int) []FrontierPoint {
	if len(means) == 0 || numPoints < 2 {
		return nil
	}

	// Scan λ from 0 (min-variance) to large (max-return).
	// Use logarithmic spacing for good coverage across the frontier.
	type rawPoint struct {
		risk, ret float64
	}
	var raw []rawPoint

	for k := 0; k < numPoints*2; k++ {
		var lambda float64
		if k == 0 {
			lambda = 0
		} else {
			t := float64(k) / float64(numPoints*2-1)
			lambda = 0.001 * math.Pow(1000000, t) // 0.001 to 1000
		}

		w := solveLongOnlyQP(means, cov, lambda)
		risk := math.Sqrt(portfolioVariance(w, cov))
		ret := portfolioReturn(w, means)
		raw = append(raw, rawPoint{risk: risk, ret: ret})
	}

	// Sort by risk ascending.
	sort.Slice(raw, func(i, j int) bool {
		return raw[i].risk < raw[j].risk
	})

	// Remove dominated points: keep only those with monotonically increasing return.
	var clean []rawPoint
	maxRet := -math.MaxFloat64
	for _, p := range raw {
		if p.ret > maxRet {
			clean = append(clean, p)
			maxRet = p.ret
		}
	}

	// Deduplicate points that are too close (within 0.1% of risk range).
	if len(clean) == 0 {
		return nil
	}
	riskRange := clean[len(clean)-1].risk - clean[0].risk
	minGap := riskRange * 0.001
	if minGap < 1e-12 {
		minGap = 1e-12
	}

	frontier := []FrontierPoint{{Risk: clean[0].risk, Return: clean[0].ret}}
	for _, p := range clean[1:] {
		last := frontier[len(frontier)-1]
		if p.risk-last.Risk >= minGap {
			frontier = append(frontier, FrontierPoint{Risk: p.risk, Return: p.ret})
		}
	}

	// Downsample to requested number of points if we have too many.
	if len(frontier) > numPoints {
		sampled := make([]FrontierPoint, numPoints)
		for i := 0; i < numPoints; i++ {
			idx := i * (len(frontier) - 1) / (numPoints - 1)
			sampled[i] = frontier[idx]
		}
		frontier = sampled
	}

	return frontier
}

func generateSuggestions(assets []AssetStats, current, optimal []float64) []AllocationSuggestion {
	var suggestions []AllocationSuggestion
	for i, a := range assets {
		curPct := current[i] * 100
		optPct := optimal[i] * 100
		delta := optPct - curPct

		action := "hold"
		reason := ""

		if delta > 3 {
			action = "increase"
			if a.SharpeRatio > 1 {
				reason = "high_sharpe"
			} else {
				reason = "diversification"
			}
		} else if delta < -3 {
			action = "decrease"
			if a.SharpeRatio < 0 {
				reason = "negative_returns"
			} else if a.Volatility > 0 && a.AvgDailyPnL/a.Volatility < 0.1 {
				reason = "poor_risk_adjusted"
			} else {
				reason = "overweight"
			}
		}

		suggestions = append(suggestions, AllocationSuggestion{
			TypeID:     a.TypeID,
			TypeName:   a.TypeName,
			Action:     action,
			CurrentPct: curPct,
			OptimalPct: optPct,
			DeltaPct:   delta,
			Reason:     reason,
		})
	}

	// Sort: decreases first, then increases.
	sort.Slice(suggestions, func(i, j int) bool {
		if suggestions[i].Action != suggestions[j].Action {
			// decrease < hold < increase (show decreases first)
			order := map[string]int{"decrease": 0, "increase": 1, "hold": 2}
			return order[suggestions[i].Action] < order[suggestions[j].Action]
		}
		return math.Abs(suggestions[i].DeltaPct) > math.Abs(suggestions[j].DeltaPct)
	})

	return suggestions
}
