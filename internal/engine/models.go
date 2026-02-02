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
	BuyLocationID   int64  `json:"-"`
	SellPrice       float64
	SellStation     string
	SellSystemName  string
	SellSystemID    int32
	SellLocationID  int64  `json:"-"`
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
}

// ContractResult represents a profitable public contract compared to market value.
type ContractResult struct {
	ContractID    int32
	Title         string
	Price         float64 // contract asking price
	MarketValue   float64 // sum of market prices for all items
	Profit        float64
	MarginPercent float64
	Volume        float64 // contract volume in m³
	StationName   string
	ItemCount     int32
	Jumps         int
	ProfitPerJump float64
}

// RouteHop represents a single buy-haul-sell leg within a multi-hop trade route.
type RouteHop struct {
	SystemName     string
	StationName    string
	SystemID       int32  `json:"-"`
	LocationID     int64  `json:"-"`
	DestSystemID   int32  `json:"-"`
	DestSystemName string
	TypeName       string
	TypeID         int32
	BuyPrice       float64
	SellPrice      float64
	Units          int32
	Profit         float64
	Jumps          int // jumps to destination
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
	SystemName      string
	CargoCapacity   float64
	MinMargin       float64
	SalesTaxPercent float64
	MinHops         int
	MaxHops         int
}

// ScanParams holds the input parameters for radius and region scans.
type ScanParams struct {
	CurrentSystemID int32
	CargoCapacity   float64
	BuyRadius       int
	SellRadius      int
	MinMargin       float64
	SalesTaxPercent float64
}
