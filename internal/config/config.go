package config

// WatchlistItem represents an item being tracked in the watchlist.
type WatchlistItem struct {
	TypeID         int32   `json:"type_id"`
	TypeName       string  `json:"type_name"`
	AddedAt        string  `json:"added_at"`
	AlertMinMargin float64 `json:"alert_min_margin"` // 0 = no alert
}

// Config holds application settings (in-memory representation).
// Persistence is handled by internal/db package.
type Config struct {
	SystemName      string  `json:"system_name"`
	CargoCapacity   float64 `json:"cargo_capacity"`
	BuyRadius       int     `json:"buy_radius"`
	SellRadius      int     `json:"sell_radius"`
	MinMargin       float64 `json:"min_margin"`
	SalesTaxPercent float64 `json:"sales_tax_percent"`
	Opacity         int     `json:"opacity"`
	WindowX         int     `json:"window_x"`
	WindowY         int     `json:"window_y"`
	WindowW         int     `json:"window_w"`
	WindowH         int     `json:"window_h"`
}

// Default returns a Config with sensible defaults.
func Default() *Config {
	return &Config{
		CargoCapacity:   5000,
		BuyRadius:       5,
		SellRadius:      10,
		MinMargin:       5,
		SalesTaxPercent: 8,
		Opacity:         230,
		WindowW:         800,
		WindowH:         600,
	}
}
