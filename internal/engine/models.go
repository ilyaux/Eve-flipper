package engine

// FlipResult represents a single profitable flip opportunity (buy low at one station, sell high at another).
type FlipResult struct {
	TypeID          int32
	TypeName        string
	Volume          float64
	BuyPrice        float64
	BestAskPrice    float64 `json:"BestAskPrice,omitempty"` // Explicit L1 best ask price at buy location (same level as BuyPrice)
	BestAskQty      int32   `json:"BestAskQty,omitempty"`   // Quantity available strictly at BestAskPrice
	BuyStation      string
	BuySystemName   string
	BuySystemID     int32
	BuyRegionID     int32  `json:"BuyRegionID"`
	BuyRegionName   string `json:"BuyRegionName,omitempty"`
	BuyLocationID   int64  `json:"BuyLocationID,omitempty"`
	SellPrice       float64
	BestBidPrice    float64 `json:"BestBidPrice,omitempty"` // Explicit L1 best bid price at sell location (same level as SellPrice)
	BestBidQty      int32   `json:"BestBidQty,omitempty"`   // Quantity available strictly at BestBidPrice
	SellStation     string
	SellSystemName  string
	SellSystemID    int32
	SellRegionID    int32  `json:"SellRegionID"`
	SellRegionName  string `json:"SellRegionName,omitempty"`
	SellLocationID  int64  `json:"SellLocationID,omitempty"`
	ProfitPerUnit   float64
	MarginPercent   float64
	UnitsToBuy      int32
	BuyOrderRemain  int32
	SellOrderRemain int32
	TotalProfit     float64
	ProfitPerJump   float64
	BuyJumps        int
	SellJumps       int
	TotalJumps      int
	DailyVolume     int64   `json:"DailyVolume"`
	Velocity        float64 `json:"Velocity"`
	PriceTrend      float64 `json:"PriceTrend"`
	S2BPerDay       float64 `json:"S2BPerDay"`   // Estimated daily "sells to buy orders" flow
	BfSPerDay       float64 `json:"BfSPerDay"`   // Estimated daily "buys from sell orders" flow
	S2BBfSRatio     float64 `json:"S2BBfSRatio"` // S2BPerDay / BfSPerDay
	BuyCompetitors  int     `json:"BuyCompetitors"`
	SellCompetitors int     `json:"SellCompetitors"`
	DailyProfit     float64 `json:"DailyProfit"` // ProfitPerUnit * min(UnitsToBuy, DailyVolume)
	// Sell-book supply at the destination market scope for this type.
	// Populated from live destination sell orders (station/system fallback).
	TargetSellSupply int64 `json:"TargetSellSupply,omitempty"`
	// Lowest sell order price at the destination market — used by sell-order mode.
	// Zero when no sell orders are found in the destination scope.
	TargetLowestSell float64 `json:"TargetLowestSell,omitempty"`
	// Execution-aware effective margin after slippage and fees.
	RealMarginPercent float64 `json:"RealMarginPercent,omitempty"`
	// True when market history for this type/region was fetched successfully.
	HistoryAvailable bool `json:"HistoryAvailable"`
	// Execution-plan derived (expected fill prices from order book depth)
	ExpectedBuyPrice      float64 `json:"ExpectedBuyPrice,omitempty"`
	ExpectedSellPrice     float64 `json:"ExpectedSellPrice,omitempty"`
	ExpectedProfit        float64 `json:"ExpectedProfit,omitempty"`
	RealProfit            float64 `json:"RealProfit,omitempty"` // primary KPI: expected net ISK with depth/slippage
	FilledQty             int32   `json:"FilledQty,omitempty"`  // executable profitable quantity from execution simulation
	CanFill               bool    `json:"CanFill"`              // true when requested quantity is executable profitably
	SlippageBuyPct        float64 `json:"SlippageBuyPct,omitempty"`
	SlippageSellPct       float64 `json:"SlippageSellPct,omitempty"`
	FillTimeDays          float64 `json:"FillTimeDays,omitempty"`          // estimated days to complete the full cycle
	LiquidityScore        float64 `json:"LiquidityScore,omitempty"`        // 0-100 score from fill time and history confidence
	LiquidityLabel        string  `json:"LiquidityLabel,omitempty"`        // high | medium | low | thin | unknown
	BacktestDays          int     `json:"BacktestDays,omitempty"`          // number of history days used for fill viability
	BacktestFillRate      float64 `json:"BacktestFillRate,omitempty"`      // % of history days with enough target volume
	BacktestMedianVol     int64   `json:"BacktestMedianVol,omitempty"`     // median daily volume in the backtest window
	CharacterAssets       int64   `json:"CharacterAssets,omitempty"`       // owned asset units for this type in selected scope
	CharacterBuyOrders    int64   `json:"CharacterBuyOrders,omitempty"`    // active buy-order units for this type in selected scope
	CharacterSellOrders   int64   `json:"CharacterSellOrders,omitempty"`   // active sell-order units for this type in selected scope
	RouteSafetyMultiplier float64 `json:"RouteSafetyMultiplier,omitempty"` // backtest route-time safety multiplier from gank risk
	RouteSafetyDanger     string  `json:"RouteSafetyDanger,omitempty"`     // green | yellow | red
	RouteSafetyKills      int     `json:"RouteSafetyKills,omitempty"`
	RouteSafetyISK        float64 `json:"RouteSafetyISK,omitempty"`

	// Regional day-trader enrichments (EVE Guru-style grouped region view).
	DaySecurity           float64   `json:"DaySecurity,omitempty"`
	DaySourceUnits        int32     `json:"DaySourceUnits,omitempty"`
	DayTargetDemandPerDay float64   `json:"DayTargetDemandPerDay,omitempty"`
	DayTargetSupplyUnits  int64     `json:"DayTargetSupplyUnits,omitempty"`
	DayTargetDOS          float64   `json:"DayTargetDOS,omitempty"`
	DayAssets             int64     `json:"DayAssets,omitempty"`
	DayActiveOrders       int64     `json:"DayActiveOrders,omitempty"`
	DaySourceAvgPrice     float64   `json:"DaySourceAvgPrice,omitempty"`
	DayTargetNowPrice     float64   `json:"DayTargetNowPrice,omitempty"`
	DayTargetPeriodPrice  float64   `json:"DayTargetPeriodPrice,omitempty"`
	DayNowProfit          float64   `json:"DayNowProfit,omitempty"`
	DayPeriodProfit       float64   `json:"DayPeriodProfit,omitempty"`
	DayROINow             float64   `json:"DayROINow,omitempty"`
	DayROIPeriod          float64   `json:"DayROIPeriod,omitempty"`
	DayCapitalRequired    float64   `json:"DayCapitalRequired,omitempty"`
	DayShippingCost       float64   `json:"DayShippingCost,omitempty"`
	DayCategoryID         int32     `json:"DayCategoryID,omitempty"`
	DayGroupID            int32     `json:"DayGroupID,omitempty"`
	DayGroupName          string    `json:"DayGroupName,omitempty"`
	DayIskPerM3Jump       float64   `json:"DayIskPerM3Jump,omitempty"`
	DayTradeScore         float64   `json:"DayTradeScore,omitempty"`
	DayPriceHistory       []float64 `json:"DayPriceHistory,omitempty"`
	DayTargetLowestSell   float64   `json:"DayTargetLowestSell,omitempty"`
}

// ContractResult represents a profitable public contract compared to market value.
type ContractResult struct {
	ContractID            int32
	Title                 string
	Price                 float64 // contract asking price
	MarketValue           float64 // sum of market prices for all items
	Profit                float64
	MarginPercent         float64
	ExpectedProfit        float64 // conservative horizon-aware expected profit
	ExpectedMarginPercent float64 // expected profit / contract price * 100
	SellConfidence        float64 // probability of full liquidation within hold horizon (0-100)
	EstLiquidationDays    float64 // bottleneck fill-time estimate for full liquidation
	ConservativeValue     float64 // conservative expected liquidation value after fees
	CarryCost             float64 // opportunity/carry cost for hold horizon
	Volume                float64 // contract volume in m³
	StationName           string
	SystemName            string `json:"SystemName,omitempty"`
	RegionName            string `json:"RegionName,omitempty"`
	LiquidationSystemName string `json:"LiquidationSystemName,omitempty"` // instant mode: chosen sell system inside sell radius
	LiquidationRegionName string `json:"LiquidationRegionName,omitempty"` // region of chosen liquidation system
	ItemCount             int32
	LiquidationJumps      int // jumps from pickup system to liquidation system (instant mode)
	Jumps                 int
	ProfitPerJump         float64
}

// RouteHop represents a single buy-haul-sell leg within a multi-hop trade route.
type RouteHop struct {
	SystemName       string
	StationName      string
	SystemID         int32
	RegionID         int32 `json:"RegionID"` // Market region for execution plan / slippage
	LocationID       int64 `json:"-"`
	EmptyJumps       int   `json:"EmptyJumps,omitempty"` // optional deadhead jumps before this trade hop
	DestSystemID     int32
	DestSystemName   string
	DestStationName  string `json:"DestStationName,omitempty"`
	DestLocationID   int64  `json:"-"`
	TypeName         string
	TypeID           int32
	BuyPrice         float64
	SellPrice        float64
	Units            int32
	Profit           float64
	Jumps            int     // jumps to destination
	VolumeM3         float64 `json:"VolumeM3,omitempty"`
	CargoM3          float64 `json:"CargoM3,omitempty"`
	CargoTrips       int     `json:"CargoTrips,omitempty"`
	ExecutionMinutes float64 `json:"ExecutionMinutes,omitempty"`
	ProfitPerHour    float64 `json:"ProfitPerHour,omitempty"`
	DailyVolume      int64   `json:"DailyVolume,omitempty"`
	FillTimeDays     float64 `json:"FillTimeDays,omitempty"`
	LiquidityScore   float64 `json:"LiquidityScore,omitempty"`
	LiquidityLabel   string  `json:"LiquidityLabel,omitempty"`
}

// RouteResult represents a complete multi-hop trade route with aggregated profit.
type RouteResult struct {
	Hops                        []RouteHop
	TotalProfit                 float64
	TotalJumps                  int
	ProfitPerJump               float64
	HopCount                    int
	TargetSystemName            string  `json:"TargetSystemName,omitempty"` // optional trip destination constraint
	TargetJumps                 int     `json:"TargetJumps,omitempty"`      // deadhead jumps from final trade to target
	CargoM3                     float64 `json:"CargoM3,omitempty"`
	CargoTrips                  int     `json:"CargoTrips,omitempty"`
	ExecutionMinutes            float64 `json:"ExecutionMinutes,omitempty"`
	ProfitPerHour               float64 `json:"ProfitPerHour,omitempty"`
	FillTimeDays                float64 `json:"FillTimeDays,omitempty"`
	LiquidityScore              float64 `json:"LiquidityScore,omitempty"`
	LiquidityLabel              string  `json:"LiquidityLabel,omitempty"`
	HaulingRiskKnown            bool    `json:"HaulingRiskKnown,omitempty"`
	HaulingDanger               string  `json:"HaulingDanger,omitempty"`
	HaulingKills                int     `json:"HaulingKills,omitempty"`
	HaulingISK                  float64 `json:"HaulingISK,omitempty"`
	HaulingRiskScore            float64 `json:"HaulingRiskScore,omitempty"`
	HaulingSafetyMultiplier     float64 `json:"HaulingSafetyMultiplier,omitempty"`
	CargoValueISK               float64 `json:"CargoValueISK,omitempty"`
	CourierCollateralISK        float64 `json:"CourierCollateralISK,omitempty"`
	CourierRewardFloorISK       float64 `json:"CourierRewardFloorISK,omitempty"`
	CourierRewardPerJumpISK     float64 `json:"CourierRewardPerJumpISK,omitempty"`
	CourierProfitAfterRewardISK float64 `json:"CourierProfitAfterRewardISK,omitempty"`
	CourierRiskPremiumPercent   float64 `json:"CourierRiskPremiumPercent,omitempty"`
	CourierViable               bool    `json:"CourierViable,omitempty"`
}

// RouteParams holds the input parameters for multi-hop route search.
type RouteParams struct {
	SystemName       string
	IgnoredSystemIDs []int32
	TargetSystemName string
	CargoCapacity    float64
	// RouteCargoCapacity overrides CargoCapacity for route hauling. This keeps
	// route ship planning independent from other scanner tabs.
	RouteCargoCapacity      float64
	RouteShipProfile        string
	RouteMinutesPerJump     float64
	RouteDockMinutes        float64
	RouteSafetyDelayPercent float64
	RouteMode               string
	MinMargin               float64
	MinISKPerJump           float64
	SalesTaxPercent         float64
	BrokerFeePercent        float64
	// SplitTradeFees enables side-specific fee model.
	// When false, legacy fields above are used.
	SplitTradeFees       bool
	BuyBrokerFeePercent  float64
	SellBrokerFeePercent float64
	BuySalesTaxPercent   float64
	SellSalesTaxPercent  float64
	MinHops              int
	MaxHops              int
	MinRouteSecurity     float64 // 0 = all space; 0.45 = highsec only; 0.7 = min 0.7
	AllowEmptyHops       bool    // allow empty travel legs between trade hops
	IncludeStructures    bool    // true = allow Upwell structure orders; false = NPC stations only
}

// ScanParams holds the input parameters for radius and region scans.
type ScanParams struct {
	CurrentSystemID  int32
	IgnoredSystemIDs []int32
	CargoCapacity    float64 // <=0 = unlimited (disable cargo cap)
	BuyRadius        int
	SellRadius       int
	MinMargin        float64
	SalesTaxPercent  float64
	BrokerFeePercent float64 // 0 = no broker fee (instant trades); >0 = applied to both buy and sell sides
	// SplitTradeFees enables side-specific fee model.
	// When false, legacy fields above are used.
	SplitTradeFees       bool
	BuyBrokerFeePercent  float64
	SellBrokerFeePercent float64
	BuySalesTaxPercent   float64
	SellSalesTaxPercent  float64
	// Advanced filters
	MinDailyVolume  int64   // 0 = no filter
	MaxInvestment   float64 // 0 = no filter (max ISK per position)
	MinItemProfit   float64 // 0 = no filter (min ISK profit per position for regional day trader)
	MinPeriodROI    float64 // 0 = no filter (min period ROI % for regional day trader)
	MaxDOS          float64 // 0 = no filter (max days-of-supply at target for regional day trader)
	MinDemandPerDay float64 // 0 = no filter (min demand units/day at target for regional day trader)
	// PurchaseDemandDays controls target purchase volume as N days of target demand.
	// Example: 0.5 means "buy half of one demand-day". <=0 uses mode-specific defaults.
	PurchaseDemandDays float64
	MinS2BPerDay       float64 // 0 = no filter
	MinBfSPerDay       float64 // 0 = no filter
	MinS2BBfSRatio     float64 // 0 = no filter
	MaxS2BBfSRatio     float64 // 0 = no filter
	AvgPricePeriod     int     // 0 = default period (14 days for regional day trader)
	// Heuristic hauling cost model: ISK per (m3 * jump) used by regional day trader scoring.
	ShippingCostPerM3Jump float64 // 0 = disabled
	// Optional source-side region constraints for regional day trader.
	// Empty = use legacy buy-radius scope from CurrentSystemID.
	SourceRegionIDs []int32
	// Optional sell-side target marketplace constraints for regional day trader.
	TargetMarketSystemID   int32   // 0 = any sell system in scope
	TargetMarketLocationID int64   // 0 = any location in target system/region
	SecurityFilter         string  // "" = all, "highsec", "lowsec", "nullsec"
	MinRouteSecurity       float64 // 0 = all space; 0.45 = highsec only; 0.7 = min 0.7 (route must stay in this security)
	TargetRegionID         int32   // 0 = search all by radius; >0 = search only in this specific region

	// --- Category/group filter for regional day trader ---
	CategoryIDs []int32 // empty = all categories; non-empty = only include these EVE category IDs

	// --- Sell-order mode for regional day trader ---
	// When true, targetNowPrice uses TargetLowestSell (lowest ask at destination)
	// instead of TargetBuyOrderPrice (highest bid). Reflects listing a sell order
	// rather than instantly hitting a buy order. Higher profit, higher risk.
	SellOrderMode bool
	// IncludeStructures keeps Upwell structure orders in scope.
	IncludeStructures bool
	// AccessToken is used for authenticated structure-market reads.
	// Runtime-only: must never be persisted.
	AccessToken string

	// --- Contract-specific filters ---
	MinContractPrice           float64 // Minimum contract price in ISK (0 = use default 10M)
	MaxContractMargin          float64 // Maximum margin % to filter scams (0 = use default 100%)
	MinPricedRatio             float64 // Minimum fraction of items that must have market price (0 = use default 0.8)
	RequireHistory             bool    // If true, skip items without market history
	ContractInstantLiquidation bool    // If true, require immediate sell-side liquidation via buy-book depth (sell radius)
	ContractHoldDays           int     // Non-instant mode: hold horizon in days (0 = default)
	ContractTargetConfidence   float64 // Non-instant mode: minimum full-liquidation probability in % (0 = default)
	ExcludeRigsWithShip        bool    // If true, exclude rig pricing when contract contains a ship
}
