package engine

import (
	"math"
	"sort"
	"time"

	"eve-flipper/internal/esi"
)

// OrderDeskHistoryKey identifies (region, type) history buckets.
type OrderDeskHistoryKey [2]int32

// NewOrderDeskHistoryKey creates a stable key for history lookup.
func NewOrderDeskHistoryKey(regionID, typeID int32) OrderDeskHistoryKey {
	return OrderDeskHistoryKey{regionID, typeID}
}

// OrderDeskOptions controls recommendation and economics assumptions.
type OrderDeskOptions struct {
	SalesTaxPercent  float64
	BrokerFeePercent float64
	TargetETADays    float64
	WarnExpiryDays   int
}

// OrderDeskSettings are echoed in the response.
type OrderDeskSettings struct {
	SalesTaxPercent  float64 `json:"sales_tax_percent"`
	BrokerFeePercent float64 `json:"broker_fee_percent"`
	TargetETADays    float64 `json:"target_eta_days"`
	WarnExpiryDays   int     `json:"warn_expiry_days"`
}

// OrderDeskSummary aggregates order health for quick triage.
type OrderDeskSummary struct {
	TotalOrders     int     `json:"total_orders"`
	BuyOrders       int     `json:"buy_orders"`
	SellOrders      int     `json:"sell_orders"`
	NeedsReprice    int     `json:"needs_reprice"`
	NeedsCancel     int     `json:"needs_cancel"`
	TotalNotional   float64 `json:"total_notional"`
	MedianETADays   float64 `json:"median_eta_days"`
	AvgETADays      float64 `json:"avg_eta_days"`
	WorstETADays    float64 `json:"worst_eta_days"`
	UnknownETACount int     `json:"unknown_eta_count"`
}

// OrderDeskOrder is one actionable row in the execution desk.
type OrderDeskOrder struct {
	OrderID             int64   `json:"order_id"`
	TypeID              int32   `json:"type_id"`
	TypeName            string  `json:"type_name"`
	LocationID          int64   `json:"location_id"`
	LocationName        string  `json:"location_name"`
	RegionID            int32   `json:"region_id"`
	IsBuyOrder          bool    `json:"is_buy_order"`
	Price               float64 `json:"price"`
	VolumeRemain        int32   `json:"volume_remain"`
	VolumeTotal         int32   `json:"volume_total"`
	Notional            float64 `json:"notional"`
	NetUnitISK          float64 `json:"net_unit_isk"`
	NetNotional         float64 `json:"net_notional"`
	Position            int     `json:"position"`
	TotalOrders         int     `json:"total_orders"`
	BookAvailable       bool    `json:"book_available"`
	BestPrice           float64 `json:"best_price"`
	SuggestedPrice      float64 `json:"suggested_price"`
	UndercutAmount      float64 `json:"undercut_amount"`
	UndercutPct         float64 `json:"undercut_pct"`
	QueueAheadQty       int64   `json:"queue_ahead_qty"`
	TopPriceQty         int64   `json:"top_price_qty"`
	AvgDailyVolume      float64 `json:"avg_daily_volume"`
	EstimatedFillPerDay float64 `json:"estimated_fill_per_day"`
	ETADays             float64 `json:"eta_days"` // -1 = unknown
	IssuedAt            string  `json:"issued_at"`
	ExpiresAt           string  `json:"expires_at"`
	DaysToExpire        int     `json:"days_to_expire"` // -1 if unknown
	Recommendation      string  `json:"recommendation"` // hold | reprice | cancel
	Reason              string  `json:"reason"`
}

// OrderDeskResponse is the full API payload for the order desk tab.
type OrderDeskResponse struct {
	Summary  OrderDeskSummary  `json:"summary"`
	Orders   []OrderDeskOrder  `json:"orders"`
	Settings OrderDeskSettings `json:"settings"`
}

func normalizeOrderDeskOptions(opt OrderDeskOptions) OrderDeskOptions {
	if opt.SalesTaxPercent < 0 {
		opt.SalesTaxPercent = 0
	}
	if opt.SalesTaxPercent > 100 {
		opt.SalesTaxPercent = 100
	}
	if opt.BrokerFeePercent < 0 {
		opt.BrokerFeePercent = 0
	}
	if opt.BrokerFeePercent > 100 {
		opt.BrokerFeePercent = 100
	}
	if opt.TargetETADays <= 0 {
		opt.TargetETADays = 3
	}
	if opt.WarnExpiryDays <= 0 {
		opt.WarnExpiryDays = 2
	}
	return opt
}

// ComputeOrderDesk builds actionable order management analytics:
// position in book, queue ahead, ETA and repricing/cancel recommendations.
func ComputeOrderDesk(
	playerOrders []esi.CharacterOrder,
	regionOrders []esi.MarketOrder,
	historyByKey map[OrderDeskHistoryKey][]esi.HistoryEntry,
	unavailableBooks map[OrderDeskHistoryKey]bool,
	opt OrderDeskOptions,
) OrderDeskResponse {
	opt = normalizeOrderDeskOptions(opt)

	out := OrderDeskResponse{
		Orders: []OrderDeskOrder{},
		Settings: OrderDeskSettings{
			SalesTaxPercent:  opt.SalesTaxPercent,
			BrokerFeePercent: opt.BrokerFeePercent,
			TargetETADays:    opt.TargetETADays,
			WarnExpiryDays:   opt.WarnExpiryDays,
		},
	}
	if len(playerOrders) == 0 {
		return out
	}

	type bookKey struct {
		locationID int64
		typeID     int32
		isBuy      bool
	}
	book := make(map[bookKey][]esi.MarketOrder)
	for _, o := range regionOrders {
		k := bookKey{locationID: o.LocationID, typeID: o.TypeID, isBuy: o.IsBuyOrder}
		book[k] = append(book[k], o)
	}

	etaKnown := make([]float64, 0, len(playerOrders))
	now := time.Now().UTC()
	out.Orders = make([]OrderDeskOrder, 0, len(playerOrders))

	for _, po := range playerOrders {
		row := OrderDeskOrder{
			OrderID:        po.OrderID,
			TypeID:         po.TypeID,
			TypeName:       po.TypeName,
			LocationID:     po.LocationID,
			LocationName:   po.LocationName,
			RegionID:       po.RegionID,
			IsBuyOrder:     po.IsBuyOrder,
			Price:          po.Price,
			VolumeRemain:   po.VolumeRemain,
			VolumeTotal:    po.VolumeTotal,
			Notional:       po.Price * float64(po.VolumeRemain),
			IssuedAt:       po.Issued,
			DaysToExpire:   -1,
			ETADays:        -1,
			BookAvailable:  true,
			Recommendation: "hold",
			Reason:         "on track",
		}

		if po.IsBuyOrder {
			row.NetUnitISK = po.Price * (1 + opt.BrokerFeePercent/100.0)
		} else {
			row.NetUnitISK = po.Price * (1 - (opt.BrokerFeePercent+opt.SalesTaxPercent)/100.0)
			if row.NetUnitISK < 0 {
				row.NetUnitISK = 0
			}
		}
		row.NetNotional = row.NetUnitISK * float64(po.VolumeRemain)

		if issuedAt, err := time.Parse(time.RFC3339, po.Issued); err == nil {
			expAt := issuedAt.AddDate(0, 0, po.Duration)
			row.ExpiresAt = expAt.Format(time.RFC3339)
			row.DaysToExpire = int(math.Ceil(expAt.Sub(now).Hours() / 24.0))
			if row.DaysToExpire < 0 {
				row.DaysToExpire = 0
			}
		}

		hk := NewOrderDeskHistoryKey(po.RegionID, po.TypeID)
		if unavailableBooks != nil && unavailableBooks[hk] {
			row.BookAvailable = false
			row.Position = 0
			row.TotalOrders = 0
			row.BestPrice = 0
			row.SuggestedPrice = po.Price
		} else {
			k := bookKey{locationID: po.LocationID, typeID: po.TypeID, isBuy: po.IsBuyOrder}
			orders := book[k]
			if len(orders) > 0 {
				sorted := make([]esi.MarketOrder, len(orders))
				copy(sorted, orders)
				if po.IsBuyOrder {
					sort.Slice(sorted, func(i, j int) bool {
						if sorted[i].Price == sorted[j].Price {
							return sorted[i].OrderID < sorted[j].OrderID
						}
						return sorted[i].Price > sorted[j].Price
					})
				} else {
					sort.Slice(sorted, func(i, j int) bool {
						if sorted[i].Price == sorted[j].Price {
							return sorted[i].OrderID < sorted[j].OrderID
						}
						return sorted[i].Price < sorted[j].Price
					})
				}

				row.BestPrice = sorted[0].Price
				for _, o := range sorted {
					if o.Price != row.BestPrice {
						break
					}
					row.TopPriceQty += int64(o.VolumeRemain)
				}

				pos := 1
				var queueAhead int64
				playerFound := false
				for _, o := range sorted {
					if o.OrderID == po.OrderID {
						playerFound = true
						break
					}
					queueAhead += int64(o.VolumeRemain)
					pos++
				}
				if !playerFound {
					pos = 1
					queueAhead = 0
					for _, o := range sorted {
						if orderDeskBetterPrice(po.IsBuyOrder, o.Price, po.Price) {
							queueAhead += int64(o.VolumeRemain)
							pos++
						}
					}
				}
				row.Position = pos
				row.QueueAheadQty = queueAhead
				row.TotalOrders = len(sorted)
				if row.TotalOrders < row.Position {
					row.TotalOrders = row.Position
				}
				if row.TotalOrders == 0 {
					row.TotalOrders = 1
				}

				if po.IsBuyOrder {
					if row.BestPrice > po.Price {
						row.UndercutAmount = row.BestPrice - po.Price
					}
					row.SuggestedPrice = row.BestPrice + 0.01
				} else {
					if row.BestPrice < po.Price {
						row.UndercutAmount = po.Price - row.BestPrice
					}
					row.SuggestedPrice = row.BestPrice - 0.01
					if row.SuggestedPrice < 0.01 {
						row.SuggestedPrice = 0.01
					}
				}
				if row.Position == 1 {
					row.SuggestedPrice = po.Price
				}
				if po.Price > 0 {
					row.UndercutPct = row.UndercutAmount / po.Price * 100.0
				}
			} else {
				row.Position = 1
				row.TotalOrders = 1
				row.BestPrice = po.Price
				row.SuggestedPrice = po.Price
			}
		}

		row.AvgDailyVolume = orderDeskAvgDailyVolume(historyByKey[hk], 7)
		row.EstimatedFillPerDay = row.AvgDailyVolume
		if row.EstimatedFillPerDay > 0 && row.VolumeRemain > 0 {
			row.ETADays = (float64(row.QueueAheadQty) + float64(row.VolumeRemain)) / row.EstimatedFillPerDay
			etaKnown = append(etaKnown, row.ETADays)
		}

		row.Recommendation, row.Reason = orderDeskRecommendation(row, opt)
		out.Orders = append(out.Orders, row)
	}

	for _, row := range out.Orders {
		out.Summary.TotalOrders++
		out.Summary.TotalNotional += row.Notional
		if row.IsBuyOrder {
			out.Summary.BuyOrders++
		} else {
			out.Summary.SellOrders++
		}
		switch row.Recommendation {
		case "reprice":
			out.Summary.NeedsReprice++
		case "cancel":
			out.Summary.NeedsCancel++
		}
		if row.ETADays < 0 {
			out.Summary.UnknownETACount++
		}
	}
	if len(etaKnown) > 0 {
		var total float64
		for _, v := range etaKnown {
			total += v
			if v > out.Summary.WorstETADays {
				out.Summary.WorstETADays = v
			}
		}
		out.Summary.AvgETADays = total / float64(len(etaKnown))
		out.Summary.MedianETADays = orderDeskMedian(etaKnown)
	}

	sort.Slice(out.Orders, func(i, j int) bool {
		pi := orderDeskActionPriority(out.Orders[i].Recommendation)
		pj := orderDeskActionPriority(out.Orders[j].Recommendation)
		if pi != pj {
			return pi < pj
		}
		if out.Orders[i].ETADays == out.Orders[j].ETADays {
			return out.Orders[i].Notional > out.Orders[j].Notional
		}
		// Unknown ETA goes last.
		if out.Orders[i].ETADays < 0 {
			return false
		}
		if out.Orders[j].ETADays < 0 {
			return true
		}
		return out.Orders[i].ETADays > out.Orders[j].ETADays
	})

	return out
}

func orderDeskBetterPrice(isBuy bool, a, b float64) bool {
	if isBuy {
		return a > b
	}
	return a < b
}

func orderDeskAvgDailyVolume(entries []esi.HistoryEntry, days int) float64 {
	if len(entries) == 0 || days <= 0 {
		return 0
	}
	volByDate := make(map[string]float64, len(entries))
	latestDate := ""
	for _, e := range entries {
		if e.Date == "" {
			continue
		}
		if e.Date > latestDate {
			latestDate = e.Date
		}
		if e.Volume > 0 {
			volByDate[e.Date] += float64(e.Volume)
		}
	}
	if latestDate == "" {
		return 0
	}
	end, err := time.Parse("2006-01-02", latestDate)
	if err != nil {
		return 0
	}
	total := 0.0
	for i := 0; i < days; i++ {
		d := end.AddDate(0, 0, -i).Format("2006-01-02")
		total += volByDate[d]
	}
	return total / float64(days)
}

func orderDeskRecommendation(row OrderDeskOrder, opt OrderDeskOptions) (string, string) {
	if !row.BookAvailable {
		return "hold", "market book unavailable"
	}

	if row.ETADays < 0 {
		if row.DaysToExpire >= 0 && row.DaysToExpire <= opt.WarnExpiryDays {
			return "cancel", "low liquidity near expiry"
		}
		return "hold", "insufficient liquidity history"
	}

	if row.DaysToExpire >= 0 && row.DaysToExpire <= 1 && row.ETADays > float64(row.DaysToExpire)+0.5 {
		return "cancel", "unlikely to fill before expiry"
	}

	if row.Position > 1 && row.DaysToExpire >= 0 && row.DaysToExpire <= opt.WarnExpiryDays {
		return "reprice", "undercut near expiry"
	}

	if row.Position > 1 && row.ETADays > opt.TargetETADays {
		return "reprice", "eta above target"
	}

	if row.Position == 1 && row.ETADays > opt.TargetETADays*2 {
		return "hold", "top of book but slow market"
	}

	return "hold", "on track"
}

func orderDeskActionPriority(action string) int {
	switch action {
	case "cancel":
		return 0
	case "reprice":
		return 1
	default:
		return 2
	}
}

func orderDeskMedian(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	s := make([]float64, len(values))
	copy(s, values)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}
