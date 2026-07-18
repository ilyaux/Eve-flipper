package api

import (
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"eve-flipper/internal/engine"
	"eve-flipper/internal/esi"
)

type itemSearchResult struct {
	TypeID     int32   `json:"type_id"`
	TypeName   string  `json:"type_name"`
	Volume     float64 `json:"volume"`
	GroupID    int32   `json:"group_id"`
	GroupName  string  `json:"group_name,omitempty"`
	CategoryID int32   `json:"category_id"`
	Relevance  int     `json:"-"`
}

type itemMarketSummary struct {
	RegionID             int32           `json:"region_id"`
	RegionName           string          `json:"region_name"`
	BestAsk              float64         `json:"best_ask"`
	BestAskVolume        int64           `json:"best_ask_volume"`
	BestBid              float64         `json:"best_bid"`
	BestBidVolume        int64           `json:"best_bid_volume"`
	Spread               float64         `json:"spread"`
	SpreadPercent        float64         `json:"spread_percent"`
	SellOrderCount       int             `json:"sell_order_count"`
	BuyOrderCount        int             `json:"buy_order_count"`
	SellUnits            int64           `json:"sell_units"`
	BuyUnits             int64           `json:"buy_units"`
	SellValueISK         float64         `json:"sell_value_isk"`
	BuyValueISK          float64         `json:"buy_value_isk"`
	SellUnitsWithin5Pct  int64           `json:"sell_units_within_5_pct"`
	BuyUnitsWithin5Pct   int64           `json:"buy_units_within_5_pct"`
	BuyPressurePct       float64         `json:"buy_pressure_pct"`
	LiquidityScore       float64         `json:"liquidity_score"`
	EstimatedSpreadValue float64         `json:"estimated_spread_value"`
	DepthBands           []itemDepthBand `json:"depth_bands,omitempty"`
}

type itemHistorySummary struct {
	Days              int     `json:"days"`
	AvgPrice          float64 `json:"avg_price"`
	AvgVolume         float64 `json:"avg_volume"`
	AvgValueISK       float64 `json:"avg_value_isk"`
	LowPrice          float64 `json:"low_price"`
	HighPrice         float64 `json:"high_price"`
	PriceChangePct    float64 `json:"price_change_pct"`
	VolumeChangePct   float64 `json:"volume_change_pct"`
	VolatilityPct     float64 `json:"volatility_pct"`
	LiquidityDays5Pct float64 `json:"liquidity_days_5_pct"`
}

type itemCharacterSummary struct {
	Assets           int64   `json:"assets"`
	ActiveBuyOrders  int64   `json:"active_buy_orders"`
	ActiveSellOrders int64   `json:"active_sell_orders"`
	AssetValueISK    float64 `json:"asset_value_isk"`
	ActiveBuyISK     float64 `json:"active_buy_isk"`
	ActiveSellISK    float64 `json:"active_sell_isk"`
	ExposureISK      float64 `json:"exposure_isk"`
	CoverageDays     float64 `json:"coverage_days"`
}

type itemPersonalTradeSummary struct {
	BuyQuantity      int64   `json:"buy_quantity"`
	SellQuantity     int64   `json:"sell_quantity"`
	BuyISK           float64 `json:"buy_isk"`
	SellISK          float64 `json:"sell_isk"`
	MatchedQuantity  int64   `json:"matched_quantity"`
	RealizedPnL      float64 `json:"realized_pnl"`
	RealizedROIPct   float64 `json:"realized_roi_pct"`
	TurnoverISK      float64 `json:"turnover_isk"`
	LastTradeDate    string  `json:"last_trade_date,omitempty"`
	ArchivedRowsUsed int     `json:"archived_rows_used"`
}

type itemRestockSummary struct {
	Signal              string   `json:"signal"`
	Reason              string   `json:"reason"`
	SuggestedAction     string   `json:"suggested_action"`
	RecommendedMaxUnits int64    `json:"recommended_max_units"`
	RecommendedMaxISK   float64  `json:"recommended_max_isk"`
	CurrentCoverage     int64    `json:"current_coverage"`
	CoverageDays        float64  `json:"coverage_days"`
	TargetCoverageDays  float64  `json:"target_coverage_days"`
	MissingUnits        int64    `json:"missing_units"`
	MaxEntryPrice       float64  `json:"max_entry_price"`
	TargetSellPrice     float64  `json:"target_sell_price"`
	WorstCaseExitPrice  float64  `json:"worst_case_exit_price"`
	MinSpreadPct        float64  `json:"min_spread_pct"`
	EdgeScore           float64  `json:"edge_score"`
	ConfidencePct       float64  `json:"confidence_pct"`
	RiskFlags           []string `json:"risk_flags,omitempty"`
}

type itemDepthBand struct {
	Band         string  `json:"band"`
	SellUnits    int64   `json:"sell_units"`
	SellValueISK float64 `json:"sell_value_isk"`
	BuyUnits     int64   `json:"buy_units"`
	BuyValueISK  float64 `json:"buy_value_isk"`
}

type itemRecentTrade struct {
	Date         string  `json:"date"`
	Side         string  `json:"side"`
	Quantity     int64   `json:"quantity"`
	UnitPrice    float64 `json:"unit_price"`
	ValueISK     float64 `json:"value_isk"`
	LocationName string  `json:"location_name,omitempty"`
}

type itemPeerTradeSummary struct {
	Scope            string  `json:"scope"`
	Label            string  `json:"label"`
	ArchivedRowsUsed int     `json:"archived_rows_used"`
	BuyQuantity      int64   `json:"buy_quantity"`
	SellQuantity     int64   `json:"sell_quantity"`
	MatchedQuantity  int64   `json:"matched_quantity"`
	TurnoverISK      float64 `json:"turnover_isk"`
	RealizedPnL      float64 `json:"realized_pnl"`
	RealizedROIPct   float64 `json:"realized_roi_pct"`
	LastTradeDate    string  `json:"last_trade_date,omitempty"`
}

type itemEdgeSummary struct {
	Label            string  `json:"label"`
	Score            float64 `json:"score"`
	ConfidencePct    float64 `json:"confidence_pct"`
	Recommendation   string  `json:"recommendation"`
	MaxPositionUnits int64   `json:"max_position_units"`
	MaxPositionISK   float64 `json:"max_position_isk"`
	DailyCapacity    float64 `json:"daily_capacity"`
	Reason           string  `json:"reason"`
}

type itemIntelligenceResponse struct {
	TypeID       int32                    `json:"type_id"`
	TypeName     string                   `json:"type_name"`
	Volume       float64                  `json:"volume"`
	GroupID      int32                    `json:"group_id"`
	GroupName    string                   `json:"group_name,omitempty"`
	CategoryID   int32                    `json:"category_id"`
	Market       itemMarketSummary        `json:"market"`
	History      itemHistorySummary       `json:"history"`
	Character    itemCharacterSummary     `json:"character"`
	Personal     itemPersonalTradeSummary `json:"personal"`
	Peer         itemPeerTradeSummary     `json:"peer"`
	Edge         itemEdgeSummary          `json:"edge"`
	Restock      itemRestockSummary       `json:"restock"`
	RecentTrades []itemRecentTrade        `json:"recent_trades,omitempty"`
	Warnings     []string                 `json:"warnings,omitempty"`
}

func (s *Server) handleItemSearch(w http.ResponseWriter, r *http.Request) {
	if !s.isReady() {
		writeError(w, http.StatusServiceUnavailable, "SDE not loaded yet")
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) > 128 {
		query = query[:128]
	}
	limit := 25
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = clampInt(n, 1, 100)
		}
	}
	if query == "" {
		writeJSON(w, []itemSearchResult{})
		return
	}
	queryLower := strings.ToLower(query)
	typeIDQuery, _ := strconv.ParseInt(query, 10, 32)

	s.mu.RLock()
	sdeData := s.sdeData
	s.mu.RUnlock()

	results := make([]itemSearchResult, 0, limit)
	for typeID, item := range sdeData.Types {
		nameLower := strings.ToLower(item.Name)
		relevance := 99
		switch {
		case typeIDQuery > 0 && int32(typeIDQuery) == typeID:
			relevance = 0
		case nameLower == queryLower:
			relevance = 1
		case strings.HasPrefix(nameLower, queryLower):
			relevance = 2
		case strings.Contains(nameLower, queryLower):
			relevance = 3
		default:
			continue
		}
		groupName := ""
		if group, ok := sdeData.Groups[item.GroupID]; ok {
			groupName = group.Name
		}
		results = append(results, itemSearchResult{
			TypeID:     typeID,
			TypeName:   item.Name,
			Volume:     item.Volume,
			GroupID:    item.GroupID,
			GroupName:  groupName,
			CategoryID: item.CategoryID,
			Relevance:  relevance,
		})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Relevance != results[j].Relevance {
			return results[i].Relevance < results[j].Relevance
		}
		return results[i].TypeName < results[j].TypeName
	})
	if len(results) > limit {
		results = results[:limit]
	}
	writeJSON(w, results)
}

func (s *Server) handleItemIntelligence(w http.ResponseWriter, r *http.Request) {
	if !s.isReady() {
		writeError(w, http.StatusServiceUnavailable, "SDE not loaded yet")
		return
	}
	typeID64, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("type_id")), 10, 32)
	if err != nil || typeID64 <= 0 {
		writeError(w, http.StatusBadRequest, "invalid type_id")
		return
	}
	regionID := engine.JitaRegionID
	if raw := strings.TrimSpace(r.URL.Query().Get("region_id")); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 32); err == nil && parsed > 0 {
			regionID = int32(parsed)
		}
	}
	typeID := int32(typeID64)
	userID := userIDFromRequest(r)

	s.mu.RLock()
	sdeData := s.sdeData
	item, ok := sdeData.Types[typeID]
	regionName := ""
	if region, okRegion := sdeData.Regions[regionID]; okRegion {
		regionName = region.Name
	}
	groupName := ""
	if ok {
		if group, okGroup := sdeData.Groups[item.GroupID]; okGroup {
			groupName = group.Name
		}
	}
	s.mu.RUnlock()
	if !ok {
		writeError(w, http.StatusNotFound, "type_id not found")
		return
	}

	resp := itemIntelligenceResponse{
		TypeID:     typeID,
		TypeName:   item.Name,
		Volume:     item.Volume,
		GroupID:    item.GroupID,
		GroupName:  groupName,
		CategoryID: item.CategoryID,
		Market: itemMarketSummary{
			RegionID:   regionID,
			RegionName: regionName,
		},
	}

	orders, orderErr := s.esi.FetchRegionOrdersByTypeContext(r.Context(), regionID, typeID)
	if orderErr != nil {
		resp.Warnings = append(resp.Warnings, "market orders unavailable: "+orderErr.Error())
	} else {
		resp.Market = summarizeItemOrders(regionID, regionName, orders)
	}

	history, historyErr := s.cachedMarketHistory(regionID, typeID)
	if historyErr != nil {
		resp.Warnings = append(resp.Warnings, "market history unavailable: "+historyErr.Error())
	} else {
		resp.History = summarizeItemHistory(history, 30)
		if resp.History.AvgVolume > 0 && resp.Market.SellUnitsWithin5Pct > 0 {
			resp.History.LiquidityDays5Pct = float64(resp.Market.SellUnitsWithin5Pct) / resp.History.AvgVolume
		}
	}

	if snapshot := s.loadRegionalInventorySnapshot(userID, regionID, 0, 0, nil); snapshot != nil {
		assetValue := resp.Market.BestBid
		if assetValue <= 0 {
			assetValue = resp.History.AvgPrice
		}
		activeBuyValue := resp.Market.BestBid
		if activeBuyValue <= 0 {
			activeBuyValue = resp.History.AvgPrice
		}
		activeSellValue := resp.Market.BestAsk
		if activeSellValue <= 0 {
			activeSellValue = resp.History.AvgPrice
		}
		assets := snapshot.AssetsByType[typeID]
		activeBuy := snapshot.ActiveBuyByType[typeID]
		activeSell := snapshot.ActiveSellByType[typeID]
		resp.Character = itemCharacterSummary{
			Assets:           assets,
			ActiveBuyOrders:  activeBuy,
			ActiveSellOrders: activeSell,
			AssetValueISK:    float64(assets) * assetValue,
			ActiveBuyISK:     float64(activeBuy) * activeBuyValue,
			ActiveSellISK:    float64(activeSell) * activeSellValue,
		}
		resp.Character.ExposureISK = resp.Character.AssetValueISK + resp.Character.ActiveBuyISK + resp.Character.ActiveSellISK
		if resp.History.AvgVolume > 0 {
			resp.Character.CoverageDays = float64(assets+activeSell) / resp.History.AvgVolume
		}
	}
	if s.db != nil {
		if txns, txErr := s.db.ListArchivedWalletTransactions(userID, nil, time.Time{}, 100000); txErr == nil {
			s.enrichWalletTransactionTypeNames(txns)
			resp.Personal = summarizeItemPersonalTrades(typeID, txns)
			resp.Peer = summarizeItemPeerTrades(txns, "group", groupName, func(tx esi.WalletTransaction) bool {
				if sdeData == nil {
					return false
				}
				t, ok := sdeData.Types[tx.TypeID]
				return ok && t.GroupID == item.GroupID
			})
			resp.RecentTrades = recentItemTrades(typeID, txns, 12)
		} else {
			resp.Warnings = append(resp.Warnings, "archived personal transactions unavailable: "+txErr.Error())
		}
	}
	resp.Restock = buildItemRestockSummary(resp)
	resp.Edge = buildItemEdgeSummary(resp)

	writeJSON(w, resp)
}

func (s *Server) cachedMarketHistory(regionID, typeID int32) ([]esi.HistoryEntry, error) {
	if s.db != nil {
		if entries, ok := s.db.GetMarketHistory(regionID, typeID); ok {
			return entries, nil
		}
	}
	entries, err := s.esi.FetchMarketHistory(regionID, typeID)
	if err != nil {
		return nil, err
	}
	if s.db != nil {
		s.db.SetMarketHistory(regionID, typeID, entries)
	}
	return entries, nil
}

func summarizeItemOrders(regionID int32, regionName string, orders []esi.MarketOrder) itemMarketSummary {
	summary := itemMarketSummary{RegionID: regionID, RegionName: regionName}
	bestAsk := math.MaxFloat64
	bestBid := 0.0
	type depthOrder struct {
		price float64
		qty   int64
	}
	sells := make([]depthOrder, 0)
	buys := make([]depthOrder, 0)
	for _, order := range orders {
		if order.VolumeRemain <= 0 || order.Price <= 0 {
			continue
		}
		qty := int64(order.VolumeRemain)
		value := order.Price * float64(qty)
		if order.IsBuyOrder {
			summary.BuyOrderCount++
			summary.BuyUnits += qty
			summary.BuyValueISK += value
			buys = append(buys, depthOrder{price: order.Price, qty: qty})
			if order.Price > bestBid {
				bestBid = order.Price
			}
			continue
		}
		summary.SellOrderCount++
		summary.SellUnits += qty
		summary.SellValueISK += value
		sells = append(sells, depthOrder{price: order.Price, qty: qty})
		if order.Price < bestAsk {
			bestAsk = order.Price
		}
	}
	if bestAsk < math.MaxFloat64 {
		summary.BestAsk = bestAsk
	}
	if bestBid > 0 {
		summary.BestBid = bestBid
	}
	sort.Slice(sells, func(i, j int) bool { return sells[i].price < sells[j].price })
	sort.Slice(buys, func(i, j int) bool { return buys[i].price > buys[j].price })
	if len(sells) > 0 && sells[0].price == summary.BestAsk {
		for _, order := range sells {
			if order.price != summary.BestAsk {
				break
			}
			summary.BestAskVolume += order.qty
		}
	}
	if len(buys) > 0 && buys[0].price == summary.BestBid {
		for _, order := range buys {
			if order.price != summary.BestBid {
				break
			}
			summary.BestBidVolume += order.qty
		}
	}
	if summary.BestAsk > 0 && summary.BestBid > 0 {
		summary.Spread = summary.BestAsk - summary.BestBid
		summary.SpreadPercent = summary.Spread / summary.BestAsk * 100
	}
	for _, pct := range []float64{1, 5, 10} {
		band := itemDepthBand{Band: strconv.FormatFloat(pct, 'f', 0, 64) + "%"}
		if summary.BestAsk > 0 {
			limit := summary.BestAsk * (1 + pct/100)
			for _, order := range sells {
				if order.price > limit {
					break
				}
				band.SellUnits += order.qty
				band.SellValueISK += order.price * float64(order.qty)
			}
		}
		if summary.BestBid > 0 {
			limit := summary.BestBid * (1 - pct/100)
			for _, order := range buys {
				if order.price < limit {
					break
				}
				band.BuyUnits += order.qty
				band.BuyValueISK += order.price * float64(order.qty)
			}
		}
		if pct == 5 {
			summary.SellUnitsWithin5Pct = band.SellUnits
			summary.BuyUnitsWithin5Pct = band.BuyUnits
			total := band.BuyValueISK + band.SellValueISK
			if total > 0 {
				summary.BuyPressurePct = band.BuyValueISK / total * 100
			}
		}
		summary.DepthBands = append(summary.DepthBands, band)
	}
	if summary.Spread > 0 && summary.SellUnitsWithin5Pct > 0 && summary.BuyUnitsWithin5Pct > 0 {
		executable := summary.SellUnitsWithin5Pct
		if summary.BuyUnitsWithin5Pct < executable {
			executable = summary.BuyUnitsWithin5Pct
		}
		summary.EstimatedSpreadValue = summary.Spread * float64(executable)
		depthScore := math.Min(float64(executable)/1000*40, 40)
		orderScore := math.Min(float64(summary.SellOrderCount+summary.BuyOrderCount)/40*30, 30)
		pressureScore := 30 - math.Min(math.Abs(summary.BuyPressurePct-50), 30)
		summary.LiquidityScore = math.Max(0, math.Min(100, depthScore+orderScore+pressureScore))
	}
	return summary
}

func summarizeItemHistory(entries []esi.HistoryEntry, days int) itemHistorySummary {
	if len(entries) == 0 || days <= 0 {
		return itemHistorySummary{}
	}
	sorted := append([]esi.HistoryEntry(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Date < sorted[j].Date })
	if len(sorted) > days {
		sorted = sorted[len(sorted)-days:]
	}
	sumVolume := 0.0
	weightedPrice := 0.0
	sumValue := 0.0
	minPrice := math.MaxFloat64
	maxPrice := 0.0
	firstHalfVolume := 0.0
	secondHalfVolume := 0.0
	for idx, entry := range sorted {
		volume := float64(entry.Volume)
		if volume <= 0 {
			volume = 1
		}
		sumVolume += volume
		weightedPrice += entry.Average * volume
		sumValue += entry.Average * volume
		if idx < len(sorted)/2 {
			firstHalfVolume += volume
		} else {
			secondHalfVolume += volume
		}
		if entry.Average > 0 && entry.Average < minPrice {
			minPrice = entry.Average
		}
		if entry.Average > maxPrice {
			maxPrice = entry.Average
		}
	}
	avgPrice := 0.0
	if sumVolume > 0 {
		avgPrice = weightedPrice / sumVolume
	}
	changePct := 0.0
	first := sorted[0].Average
	last := sorted[len(sorted)-1].Average
	if first > 0 && last > 0 {
		changePct = (last - first) / first * 100
	}
	volumeChangePct := 0.0
	if firstHalfVolume > 0 {
		volumeChangePct = (secondHalfVolume - firstHalfVolume) / firstHalfVolume * 100
	}
	volatilityPct := 0.0
	if avgPrice > 0 && maxPrice > 0 && minPrice < math.MaxFloat64 {
		volatilityPct = (maxPrice - minPrice) / avgPrice * 100
	}
	if minPrice == math.MaxFloat64 {
		minPrice = 0
	}
	return itemHistorySummary{
		Days:            len(sorted),
		AvgPrice:        avgPrice,
		AvgVolume:       sumVolume / float64(len(sorted)),
		AvgValueISK:     sumValue / float64(len(sorted)),
		LowPrice:        minPrice,
		HighPrice:       maxPrice,
		PriceChangePct:  changePct,
		VolumeChangePct: volumeChangePct,
		VolatilityPct:   volatilityPct,
	}
}

func summarizeItemPersonalTrades(typeID int32, txns []esi.WalletTransaction) itemPersonalTradeSummary {
	var out itemPersonalTradeSummary
	for _, tx := range txns {
		if tx.TypeID != typeID || tx.Quantity <= 0 || tx.UnitPrice <= 0 {
			continue
		}
		qty := int64(tx.Quantity)
		value := tx.UnitPrice * float64(qty)
		out.ArchivedRowsUsed++
		out.TurnoverISK += value
		if tx.Date > out.LastTradeDate {
			out.LastTradeDate = tx.Date
		}
		if tx.IsBuy {
			out.BuyQuantity += qty
			out.BuyISK += value
		} else {
			out.SellQuantity += qty
			out.SellISK += value
		}
	}
	if out.BuyQuantity > 0 && out.SellQuantity > 0 {
		out.MatchedQuantity = out.BuyQuantity
		if out.SellQuantity < out.MatchedQuantity {
			out.MatchedQuantity = out.SellQuantity
		}
		avgBuy := out.BuyISK / float64(out.BuyQuantity)
		avgSell := out.SellISK / float64(out.SellQuantity)
		matchedCost := avgBuy * float64(out.MatchedQuantity)
		out.RealizedPnL = (avgSell - avgBuy) * float64(out.MatchedQuantity)
		if matchedCost > 0 {
			out.RealizedROIPct = out.RealizedPnL / matchedCost * 100
		}
	}
	return out
}

func summarizeItemPeerTrades(txns []esi.WalletTransaction, scope string, label string, match func(esi.WalletTransaction) bool) itemPeerTradeSummary {
	out := itemPeerTradeSummary{Scope: scope, Label: label}
	if out.Label == "" {
		out.Label = scope
	}
	if match == nil {
		return out
	}
	var buyISK float64
	var sellISK float64
	for _, tx := range txns {
		if tx.Quantity <= 0 || tx.UnitPrice <= 0 || !match(tx) {
			continue
		}
		qty := int64(tx.Quantity)
		value := tx.UnitPrice * float64(qty)
		out.ArchivedRowsUsed++
		out.TurnoverISK += value
		if tx.Date > out.LastTradeDate {
			out.LastTradeDate = tx.Date
		}
		if tx.IsBuy {
			out.BuyQuantity += qty
			buyISK += value
		} else {
			out.SellQuantity += qty
			sellISK += value
		}
	}
	if out.BuyQuantity > 0 && out.SellQuantity > 0 {
		out.MatchedQuantity = out.BuyQuantity
		if out.SellQuantity < out.MatchedQuantity {
			out.MatchedQuantity = out.SellQuantity
		}
		avgBuy := buyISK / float64(out.BuyQuantity)
		avgSell := sellISK / float64(out.SellQuantity)
		matchedCost := avgBuy * float64(out.MatchedQuantity)
		out.RealizedPnL = (avgSell - avgBuy) * float64(out.MatchedQuantity)
		if matchedCost > 0 {
			out.RealizedROIPct = out.RealizedPnL / matchedCost * 100
		}
	}
	return out
}

func recentItemTrades(typeID int32, txns []esi.WalletTransaction, limit int) []itemRecentTrade {
	if limit <= 0 {
		return nil
	}
	out := make([]itemRecentTrade, 0, limit)
	for _, tx := range txns {
		if tx.TypeID != typeID || tx.Quantity <= 0 || tx.UnitPrice <= 0 {
			continue
		}
		side := "sell"
		if tx.IsBuy {
			side = "buy"
		}
		qty := int64(tx.Quantity)
		out = append(out, itemRecentTrade{
			Date:         tx.Date,
			Side:         side,
			Quantity:     qty,
			UnitPrice:    tx.UnitPrice,
			ValueISK:     tx.UnitPrice * float64(qty),
			LocationName: tx.LocationName,
		})
		if len(out) >= limit {
			break
		}
	}
	return out
}

func buildItemRestockSummary(resp itemIntelligenceResponse) itemRestockSummary {
	currentCoverage := resp.Character.Assets + resp.Character.ActiveSellOrders
	avgDailyVolume := resp.History.AvgVolume
	if avgDailyVolume <= 0 {
		avgDailyVolume = float64(resp.Market.BuyUnits+resp.Market.SellUnits) / 60
	}
	targetCoverageDays := 3.0
	if resp.History.VolatilityPct > 40 {
		targetCoverageDays = 1.5
	}
	recommendedUnits := int64(math.Ceil(avgDailyVolume * targetCoverageDays))
	if recommendedUnits < 1 && resp.Market.BuyOrderCount+resp.Market.SellOrderCount > 0 {
		recommendedUnits = 1
	}
	missingUnits := recommendedUnits
	if recommendedUnits > 0 && currentCoverage > 0 {
		recommendedUnits -= currentCoverage
		if recommendedUnits < 0 {
			recommendedUnits = 0
		}
		missingUnits = recommendedUnits
	}
	minSpread := 8.0
	if resp.History.VolatilityPct > 0 {
		minSpread += math.Min(resp.History.VolatilityPct/2, 20)
	}
	if resp.Personal.ArchivedRowsUsed >= 5 && resp.Personal.RealizedROIPct < 0 {
		minSpread += 5
	}
	signal := "watch"
	reason := "insufficient personal history"
	if resp.Personal.ArchivedRowsUsed >= 5 {
		if resp.Personal.RealizedPnL > 0 && resp.Personal.RealizedROIPct >= 5 {
			signal = "good_edge"
			reason = "positive realized personal history"
		} else if resp.Personal.RealizedPnL < 0 {
			signal = "avoid"
			reason = "negative realized personal history"
		}
	}
	if currentCoverage <= 0 && signal != "avoid" {
		reason += "; no current stock/orders"
	}
	if resp.Market.LiquidityScore > 0 && resp.Market.LiquidityScore < 35 {
		reason += "; thin orderbook"
	}
	unitCost := resp.Market.BestAsk
	if unitCost <= 0 {
		unitCost = resp.History.AvgPrice
	}
	maxEntryPrice := 0.0
	targetSellPrice := 0.0
	worstCaseExitPrice := 0.0
	if resp.Market.BestBid > 0 && minSpread > 0 {
		maxEntryPrice = resp.Market.BestBid / (1 + minSpread/100)
	}
	if unitCost > 0 && minSpread > 0 {
		targetSellPrice = unitCost * (1 + minSpread/100)
		worstCaseExitPrice = unitCost * (1 + math.Max(minSpread-5, 0)/100)
	}
	coverageDays := 0.0
	if avgDailyVolume > 0 {
		coverageDays = float64(currentCoverage) / avgDailyVolume
	}
	riskFlags := make([]string, 0, 4)
	if resp.History.VolatilityPct > 30 {
		riskFlags = append(riskFlags, "volatile price history")
	}
	if resp.Market.LiquidityScore > 0 && resp.Market.LiquidityScore < 35 {
		riskFlags = append(riskFlags, "thin executable depth")
	}
	if resp.Personal.ArchivedRowsUsed >= 5 && resp.Personal.RealizedPnL < 0 {
		riskFlags = append(riskFlags, "negative personal item history")
	}
	if resp.Peer.ArchivedRowsUsed >= 10 && resp.Peer.RealizedPnL < 0 {
		riskFlags = append(riskFlags, "negative group history")
	}
	if resp.Market.Spread <= 0 {
		riskFlags = append(riskFlags, "crossed or negative spread")
		recommendedUnits = 0
		missingUnits = 0
		reason += "; crossed or negative spread"
	} else if resp.Market.SpreadPercent < minSpread {
		riskFlags = append(riskFlags, "spread below guard")
		recommendedUnits = 0
		missingUnits = 0
		reason += "; spread below guard"
	}
	edge := computeItemEdgeScore(resp)
	confidence := computeItemConfidence(resp)
	action := "watch"
	switch {
	case signal == "avoid" || len(riskFlags) >= 3:
		action = "avoid"
	case recommendedUnits > 0 && edge >= 65:
		action = "restock"
	case currentCoverage > 0:
		action = "hold"
	}
	return itemRestockSummary{
		CurrentCoverage:     currentCoverage,
		CoverageDays:        coverageDays,
		TargetCoverageDays:  targetCoverageDays,
		MissingUnits:        missingUnits,
		Signal:              signal,
		Reason:              strings.TrimPrefix(reason, "; "),
		SuggestedAction:     action,
		RecommendedMaxUnits: recommendedUnits,
		RecommendedMaxISK:   unitCost * float64(recommendedUnits),
		MaxEntryPrice:       maxEntryPrice,
		TargetSellPrice:     targetSellPrice,
		WorstCaseExitPrice:  worstCaseExitPrice,
		MinSpreadPct:        minSpread,
		EdgeScore:           edge,
		ConfidencePct:       confidence,
		RiskFlags:           riskFlags,
	}
}

func buildItemEdgeSummary(resp itemIntelligenceResponse) itemEdgeSummary {
	score := resp.Restock.EdgeScore
	if score == 0 {
		score = computeItemEdgeScore(resp)
	}
	confidence := resp.Restock.ConfidencePct
	if confidence == 0 {
		confidence = computeItemConfidence(resp)
	}
	unitCost := resp.Market.BestAsk
	if unitCost <= 0 {
		unitCost = resp.History.AvgPrice
	}
	label := "Unknown edge"
	recommendation := "Collect more market and personal history before sizing up."
	switch {
	case score >= 75:
		label = "Good edge"
		recommendation = "Candidate for a controlled restock if capital and order slots allow."
	case score >= 55:
		label = "Tradable"
		recommendation = "Trade small size and watch fill speed."
	case score >= 35:
		label = "Weak edge"
		recommendation = "Only enter with extra spread or confirmed demand."
	default:
		label = "Avoid"
		recommendation = "Market or personal history is not strong enough right now."
	}
	if resp.Restock.SuggestedAction != "" {
		recommendation = resp.Restock.SuggestedAction + ": " + recommendation
	}
	return itemEdgeSummary{
		Label:            label,
		Score:            score,
		ConfidencePct:    confidence,
		Recommendation:   recommendation,
		MaxPositionUnits: resp.Restock.RecommendedMaxUnits,
		MaxPositionISK:   unitCost * float64(resp.Restock.RecommendedMaxUnits),
		DailyCapacity:    resp.History.AvgVolume,
		Reason:           resp.Restock.Reason,
	}
}

func computeItemEdgeScore(resp itemIntelligenceResponse) float64 {
	score := 45.0
	if resp.Market.SpreadPercent > 0 {
		score += math.Min(resp.Market.SpreadPercent, 30)
	}
	if resp.Market.LiquidityScore > 0 {
		score += (resp.Market.LiquidityScore - 50) * 0.25
	}
	if resp.History.AvgVolume > 0 {
		score += math.Min(resp.History.AvgVolume/20, 10)
	}
	if resp.History.VolatilityPct > 0 {
		score -= math.Min(resp.History.VolatilityPct/3, 15)
	}
	if resp.Personal.ArchivedRowsUsed >= 5 {
		score += math.Max(-20, math.Min(resp.Personal.RealizedROIPct, 20)) * 0.7
	}
	if resp.Peer.ArchivedRowsUsed >= 10 {
		score += math.Max(-15, math.Min(resp.Peer.RealizedROIPct, 15)) * 0.5
	}
	if resp.Character.CoverageDays > 7 {
		score -= 8
	}
	return math.Max(0, math.Min(100, score))
}

func computeItemConfidence(resp itemIntelligenceResponse) float64 {
	confidence := 10.0
	if resp.History.Days > 0 {
		confidence += math.Min(float64(resp.History.Days)/30*25, 25)
	}
	if resp.Market.SellOrderCount+resp.Market.BuyOrderCount > 0 {
		confidence += math.Min(float64(resp.Market.SellOrderCount+resp.Market.BuyOrderCount)/50*20, 20)
	}
	if resp.Personal.ArchivedRowsUsed > 0 {
		confidence += math.Min(float64(resp.Personal.ArchivedRowsUsed)/25*25, 25)
	}
	if resp.Peer.ArchivedRowsUsed > 0 {
		confidence += math.Min(float64(resp.Peer.ArchivedRowsUsed)/100*20, 20)
	}
	return math.Max(0, math.Min(100, confidence))
}
