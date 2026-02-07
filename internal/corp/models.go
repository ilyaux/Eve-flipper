package corp

// CorpInfo holds basic corporation identity.
type CorpInfo struct {
	CorporationID int32  `json:"corporation_id"`
	Name          string `json:"name"`
	Ticker        string `json:"ticker"`
	MemberCount   int    `json:"member_count"`
}

// CorpWalletDivision represents one of the 7 corporation wallet divisions.
type CorpWalletDivision struct {
	Division int     `json:"division"` // 1-7
	Name     string  `json:"name"`     // user-friendly division name
	Balance  float64 `json:"balance"`  // ISK balance
}

// CorpJournalEntry mirrors ESI GET /corporations/{id}/wallets/{division}/journal/.
type CorpJournalEntry struct {
	ID             int64   `json:"id"`
	Date           string  `json:"date"` // ISO 8601
	RefType        string  `json:"ref_type"`
	Amount         float64 `json:"amount"`
	Balance        float64 `json:"balance"`
	Description    string  `json:"description"`
	FirstPartyID   int64   `json:"first_party_id,omitempty"`
	SecondPartyID  int64   `json:"second_party_id,omitempty"`
	FirstPartyName string  `json:"first_party_name,omitempty"`  // enriched
	SecondPartyName string `json:"second_party_name,omitempty"` // enriched
}

// CorpTransaction mirrors ESI GET /corporations/{id}/wallets/{division}/transactions/.
type CorpTransaction struct {
	TransactionID int64   `json:"transaction_id"`
	Date          string  `json:"date"`
	TypeID        int32   `json:"type_id"`
	TypeName      string  `json:"type_name,omitempty"` // enriched from SDE
	Quantity      int32   `json:"quantity"`
	UnitPrice     float64 `json:"unit_price"`
	IsBuy         bool    `json:"is_buy"`
	LocationID    int64   `json:"location_id"`
	LocationName  string  `json:"location_name,omitempty"` // enriched
	ClientID      int64   `json:"client_id"`
	ClientName    string  `json:"client_name,omitempty"` // enriched
}

// CorpMember represents a corporation member with tracking data.
// Combines /corporations/{id}/members/ and /corporations/{id}/membertracking/.
type CorpMember struct {
	CharacterID int64  `json:"character_id"`
	Name        string `json:"name"`
	// From membertracking (requires Director or equivalent)
	LastLogin  string `json:"last_login,omitempty"`  // ISO 8601
	LogoffDate string `json:"logoff_date,omitempty"` // ISO 8601
	ShipTypeID int32  `json:"ship_type_id,omitempty"`
	ShipName   string `json:"ship_name,omitempty"` // enriched from SDE
	LocationID int64  `json:"location_id,omitempty"`
	SystemID   int32  `json:"system_id,omitempty"`
	SystemName string `json:"system_name,omitempty"` // enriched
	// Roles (from /corporations/{id}/roles/)
	Roles []string `json:"roles,omitempty"`
}

// CorpIndustryJob mirrors ESI GET /corporations/{id}/industry/jobs/.
type CorpIndustryJob struct {
	JobID           int32  `json:"job_id"`
	InstallerID     int64  `json:"installer_id"`
	InstallerName   string `json:"installer_name,omitempty"` // enriched
	Activity        string `json:"activity"`                 // manufacturing, researching_time_efficiency, etc.
	BlueprintTypeID int32  `json:"blueprint_type_id"`
	ProductTypeID   int32  `json:"product_type_id"`
	ProductName     string `json:"product_name,omitempty"` // enriched from SDE
	Status          string `json:"status"`                 // active, delivered, cancelled, paused, ready
	Runs            int32  `json:"runs"`
	StartDate       string `json:"start_date"`
	EndDate         string `json:"end_date"`
	LocationID      int64  `json:"location_id"`
	LocationName    string `json:"location_name,omitempty"` // enriched
}

// CorpMiningEntry mirrors ESI GET /corporation/{id}/mining/observers/{observer_id}/.
type CorpMiningEntry struct {
	CharacterID   int64  `json:"character_id"`
	CharacterName string `json:"character_name,omitempty"` // enriched
	Date          string `json:"date"`                     // YYYY-MM-DD
	TypeID        int32  `json:"type_id"`
	TypeName      string `json:"type_name,omitempty"` // enriched from SDE
	Quantity      int64  `json:"quantity"`             // units mined
}

// CorpMarketOrder mirrors ESI GET /corporations/{id}/orders/.
type CorpMarketOrder struct {
	OrderID       int64   `json:"order_id"`
	CharacterID   int64   `json:"character_id"`
	CharacterName string  `json:"character_name,omitempty"` // enriched
	TypeID        int32   `json:"type_id"`
	TypeName      string  `json:"type_name,omitempty"` // enriched from SDE
	Price         float64 `json:"price"`
	VolumeRemain  int32   `json:"volume_remain"`
	VolumeTotal   int32   `json:"volume_total"`
	IsBuyOrder    bool    `json:"is_buy_order"`
	LocationID    int64   `json:"location_id"`
	LocationName  string  `json:"location_name,omitempty"` // enriched
	Issued        string  `json:"issued"`
	Duration      int     `json:"duration"` // days
	RegionID      int32   `json:"region_id"`
}

// ============================================================
// Dashboard aggregated response
// ============================================================

// CorpDashboard is the top-level response for GET /api/corp/dashboard.
type CorpDashboard struct {
	Info    CorpInfo             `json:"info"`
	IsDemo  bool                `json:"is_demo"`
	Wallets []CorpWalletDivision `json:"wallets"`
	// Aggregated financials
	TotalBalance   float64              `json:"total_balance"`
	Revenue30d     float64              `json:"revenue_30d"`
	Expenses30d    float64              `json:"expenses_30d"`
	NetIncome30d   float64              `json:"net_income_30d"`
	Revenue7d      float64              `json:"revenue_7d"`
	Expenses7d     float64              `json:"expenses_7d"`
	NetIncome7d    float64              `json:"net_income_7d"`
	// Breakdown by income source
	IncomeBySource []IncomeSource       `json:"income_by_source"`
	// Daily P&L for chart (last 90 days)
	DailyPnL       []DailyPnLEntry      `json:"daily_pnl"`
	// Top contributors
	TopContributors []MemberContribution `json:"top_contributors"`
	// Member summary
	MemberSummary   MemberSummary        `json:"member_summary"`
	// Active industry
	IndustrySummary IndustrySummary      `json:"industry_summary"`
	// Mining summary
	MiningSummary   MiningSummary        `json:"mining_summary"`
	// Market orders summary
	MarketSummary   MarketSummary        `json:"market_summary"`
}

// IncomeSource represents a category of income/expense.
type IncomeSource struct {
	Category string  `json:"category"` // bounties, market, mining, pi, industry, taxes, srp, other
	Label    string  `json:"label"`    // human-readable
	Amount   float64 `json:"amount"`   // positive = income, negative = expense
	Percent  float64 `json:"percent"`  // share of total income (0-100)
}

// DailyPnLEntry is one day of corp financial data for charting.
type DailyPnLEntry struct {
	Date         string  `json:"date"` // YYYY-MM-DD
	Revenue      float64 `json:"revenue"`
	Expenses     float64 `json:"expenses"`
	NetIncome    float64 `json:"net_income"`
	Cumulative   float64 `json:"cumulative"`
	Transactions int     `json:"transactions"` // journal entry count
}

// MemberContribution represents a member's economic contribution.
type MemberContribution struct {
	CharacterID int64   `json:"character_id"`
	Name        string  `json:"name"`
	TotalISK    float64 `json:"total_isk"`    // total ISK generated for corp
	Category    string  `json:"category"`     // primary role: miner, ratter, trader, etc.
	IsOnline    bool    `json:"is_online"`    // recently active
}

// MemberSummary holds aggregated member activity stats.
type MemberSummary struct {
	TotalMembers  int `json:"total_members"`
	ActiveLast7d  int `json:"active_last_7d"`
	ActiveLast30d int `json:"active_last_30d"`
	Inactive30d   int `json:"inactive_30d"`
	// Role breakdown
	Miners        int `json:"miners"`
	Ratters       int `json:"ratters"`
	Traders       int `json:"traders"`
	Industrialists int `json:"industrialists"`
	PvPers        int `json:"pvpers"`
	Other         int `json:"other"`
}

// IndustrySummary holds aggregated industry stats.
type IndustrySummary struct {
	ActiveJobs       int     `json:"active_jobs"`
	CompletedJobs30d int     `json:"completed_jobs_30d"`
	ProductionValue  float64 `json:"production_value"` // estimated ISK value
	TopProducts      []ProductEntry `json:"top_products"`
}

// ProductEntry represents a product being manufactured.
type ProductEntry struct {
	TypeID       int32   `json:"type_id"`
	TypeName     string  `json:"type_name"`
	Runs         int32   `json:"runs"`
	Jobs         int     `json:"jobs"`
	EstimatedISK float64 `json:"estimated_isk,omitempty"` // runs × adjusted price
}

// MiningSummary holds aggregated mining stats.
type MiningSummary struct {
	TotalVolume30d int64   `json:"total_volume_30d"` // units
	EstimatedISK   float64 `json:"estimated_isk"`    // estimated ISK value
	ActiveMiners   int     `json:"active_miners"`
	TopOres        []OreEntry `json:"top_ores"`
}

// OreEntry represents a mined ore type.
type OreEntry struct {
	TypeID       int32   `json:"type_id"`
	TypeName     string  `json:"type_name"`
	Quantity     int64   `json:"quantity"`
	EstimatedISK float64 `json:"estimated_isk,omitempty"` // quantity × adjusted price
}

// MarketSummary holds aggregated market order stats.
type MarketSummary struct {
	ActiveBuyOrders  int     `json:"active_buy_orders"`
	ActiveSellOrders int     `json:"active_sell_orders"`
	TotalBuyValue    float64 `json:"total_buy_value"`
	TotalSellValue   float64 `json:"total_sell_value"`
	UniqueTraders    int     `json:"unique_traders"`
}

// CharacterRoles holds a character's corporation roles.
type CharacterRoles struct {
	Roles        []string `json:"roles"`
	IsDirector   bool     `json:"is_director"`
	CorporationID int32   `json:"corporation_id"`
}
