package engine

// FlipResult represents a single profitable flip opportunity (buy low at one station, sell high at another).
type FlipResult struct {
	TypeID          int32
	TypeName        string
	Volume          float64
	BuyPrice        float64
	BuyStation      string
	BuySystemName   string
	BuySystemID     int32
	BuyRegionID     int32  `json:"BuyRegionID"`
	BuyRegionName   string `json:"BuyRegionName,omitempty"`
	BuyLocationID   int64  `json:"BuyLocationID,omitempty"`
	SellPrice       float64
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
	// Execution-plan derived (expected fill prices from order book depth)
	ExpectedBuyPrice  float64 `json:"ExpectedBuyPrice,omitempty"`
	ExpectedSellPrice float64 `json:"ExpectedSellPrice,omitempty"`
	ExpectedProfit    float64 `json:"ExpectedProfit,omitempty"`
	RealProfit        float64 `json:"RealProfit,omitempty"` // primary KPI: expected net ISK with depth/slippage
	FilledQty         int32   `json:"FilledQty,omitempty"`  // executable profitable quantity from execution simulation
	CanFill           bool    `json:"CanFill"`              // true when requested quantity is executable profitably
	SlippageBuyPct    float64 `json:"SlippageBuyPct,omitempty"`
	SlippageSellPct   float64 `json:"SlippageSellPct,omitempty"`
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
	ItemCount             int32
	Jumps                 int
	ProfitPerJump         float64
}

// RouteHop represents a single buy-haul-sell leg within a multi-hop trade route.
type RouteHop struct {
	SystemName      string
	StationName     string
	SystemID        int32
	RegionID        int32 `json:"RegionID"` // Market region for execution plan / slippage
	LocationID      int64 `json:"-"`
	DestSystemID    int32
	DestSystemName  string
	DestStationName string `json:"DestStationName,omitempty"`
	DestLocationID  int64  `json:"-"`
	TypeName        string
	TypeID          int32
	BuyPrice        float64
	SellPrice       float64
	Units           int32
	Profit          float64
	Jumps           int // jumps to destination
}

// RouteResult represents a complete multi-hop trade route with aggregated profit.
type RouteResult struct {
	Hops          []RouteHop
	TotalProfit   float64
	TotalJumps    int
	ProfitPerJump float64
	HopCount      int
}

// RouteParams holds the input parameters for multi-hop route search.
type RouteParams struct {
	SystemName       string
	CargoCapacity    float64
	MinMargin        float64
	SalesTaxPercent  float64
	BrokerFeePercent float64
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
}

// ScanParams holds the input parameters for radius and region scans.
type ScanParams struct {
	CurrentSystemID  int32
	CargoCapacity    float64
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
	MinDailyVolume   int64   // 0 = no filter
	MaxInvestment    float64 // 0 = no filter (max ISK per position)
	MinS2BPerDay     float64 // 0 = no filter
	MinBfSPerDay     float64 // 0 = no filter
	MinS2BBfSRatio   float64 // 0 = no filter
	MaxS2BBfSRatio   float64 // 0 = no filter
	SecurityFilter   string  // "" = all, "highsec", "lowsec", "nullsec"
	MinRouteSecurity float64 // 0 = all space; 0.45 = highsec only; 0.7 = min 0.7 (route must stay in this security)
	TargetRegionID   int32   // 0 = search all by radius; >0 = search only in this specific region

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
