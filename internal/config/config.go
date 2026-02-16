package config

// WatchlistItem represents an item being tracked in the watchlist.
type WatchlistItem struct {
	TypeID         int32   `json:"type_id"`
	TypeName       string  `json:"type_name"`
	AddedAt        string  `json:"added_at"`
	AlertMinMargin float64 `json:"alert_min_margin"` // 0 = no alert
	AlertEnabled   bool    `json:"alert_enabled"`
	AlertMetric    string  `json:"alert_metric"`    // margin_percent | total_profit | profit_per_unit | daily_volume
	AlertThreshold float64 `json:"alert_threshold"` // threshold for selected metric
}

// Config holds application settings (in-memory representation).
// Persistence is handled by internal/db package.
type Config struct {
	SystemName           string  `json:"system_name"`
	CargoCapacity        float64 `json:"cargo_capacity"`
	BuyRadius            int     `json:"buy_radius"`
	SellRadius           int     `json:"sell_radius"`
	MinMargin            float64 `json:"min_margin"`
	SalesTaxPercent      float64 `json:"sales_tax_percent"`
	BrokerFeePercent     float64 `json:"broker_fee_percent"`
	SplitTradeFees       bool    `json:"split_trade_fees"`
	BuyBrokerFeePercent  float64 `json:"buy_broker_fee_percent"`
	SellBrokerFeePercent float64 `json:"sell_broker_fee_percent"`
	BuySalesTaxPercent   float64 `json:"buy_sales_tax_percent"`
	SellSalesTaxPercent  float64 `json:"sell_sales_tax_percent"`
	AlertTelegram        bool    `json:"alert_telegram"`
	AlertDiscord         bool    `json:"alert_discord"`
	AlertDesktop         bool    `json:"alert_desktop"`
	AlertTelegramToken   string  `json:"alert_telegram_token"`
	AlertTelegramChatID  string  `json:"alert_telegram_chat_id"`
	AlertDiscordWebhook  string  `json:"alert_discord_webhook"`
	Opacity              int     `json:"opacity"`
	WindowX              int     `json:"window_x"`
	WindowY              int     `json:"window_y"`
	WindowW              int     `json:"window_w"`
	WindowH              int     `json:"window_h"`
}

// Default returns a Config with sensible defaults.
func Default() *Config {
	return &Config{
		CargoCapacity:        5000,
		BuyRadius:            5,
		SellRadius:           10,
		MinMargin:            5,
		SalesTaxPercent:      8,
		BrokerFeePercent:     0,
		SplitTradeFees:       false,
		BuyBrokerFeePercent:  0,
		SellBrokerFeePercent: 0,
		BuySalesTaxPercent:   0,
		SellSalesTaxPercent:  8,
		AlertDesktop:         true,
		Opacity:              230,
		WindowW:              800,
		WindowH:              600,
	}
}
