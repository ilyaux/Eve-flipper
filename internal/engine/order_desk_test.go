package engine

import (
	"math"
	"testing"
	"time"

	"eve-flipper/internal/esi"
)

func TestComputeOrderDesk_QueueEtaAndReprice(t *testing.T) {
	issued := time.Now().UTC().AddDate(0, 0, -1).Format(time.RFC3339)
	player := []esi.CharacterOrder{
		{
			OrderID:      1001,
			TypeID:       34,
			TypeName:     "Tritanium",
			LocationID:   60003760,
			LocationName: "Jita",
			RegionID:     10000002,
			Price:        100,
			VolumeRemain: 10,
			VolumeTotal:  10,
			IsBuyOrder:   false,
			Duration:     90,
			Issued:       issued,
		},
	}
	regional := []esi.MarketOrder{
		{OrderID: 2001, TypeID: 34, LocationID: 60003760, Price: 99, VolumeRemain: 5, IsBuyOrder: false},
		{OrderID: 1001, TypeID: 34, LocationID: 60003760, Price: 100, VolumeRemain: 10, IsBuyOrder: false},
		{OrderID: 2002, TypeID: 34, LocationID: 60003760, Price: 101, VolumeRemain: 30, IsBuyOrder: false},
	}
	history := map[OrderDeskHistoryKey][]esi.HistoryEntry{
		NewOrderDeskHistoryKey(10000002, 34): {
			{Date: "2026-02-01", Volume: 10},
			{Date: "2026-02-02", Volume: 10},
			{Date: "2026-02-03", Volume: 10},
			{Date: "2026-02-04", Volume: 10},
			{Date: "2026-02-05", Volume: 10},
			{Date: "2026-02-06", Volume: 10},
			{Date: "2026-02-07", Volume: 10},
		},
	}

	got := ComputeOrderDesk(player, regional, history, nil, OrderDeskOptions{
		SalesTaxPercent:  8,
		BrokerFeePercent: 1,
		TargetETADays:    1,
		WarnExpiryDays:   2,
	})

	if len(got.Orders) != 1 {
		t.Fatalf("orders len = %d, want 1", len(got.Orders))
	}
	row := got.Orders[0]
	if row.Position != 2 {
		t.Fatalf("position = %d, want 2", row.Position)
	}
	if row.QueueAheadQty != 5 {
		t.Fatalf("queue_ahead_qty = %d, want 5", row.QueueAheadQty)
	}
	if math.Abs(row.ETADays-1.5) > 1e-6 {
		t.Fatalf("eta_days = %v, want 1.5", row.ETADays)
	}
	if row.Recommendation != "reprice" {
		t.Fatalf("recommendation = %q, want reprice", row.Recommendation)
	}
	if math.Abs(row.SuggestedPrice-98.99) > 1e-6 {
		t.Fatalf("suggested_price = %v, want 98.99", row.SuggestedPrice)
	}
	if got.Summary.NeedsReprice != 1 {
		t.Fatalf("summary needs_reprice = %d, want 1", got.Summary.NeedsReprice)
	}
}

func TestComputeOrderDesk_UnknownLiquidityCancelNearExpiry(t *testing.T) {
	issued := time.Now().UTC().AddDate(0, 0, -89).Format(time.RFC3339)
	player := []esi.CharacterOrder{
		{
			OrderID:      1002,
			TypeID:       35,
			TypeName:     "Pyerite",
			LocationID:   60008494,
			LocationName: "Amarr",
			RegionID:     10000043,
			Price:        10,
			VolumeRemain: 100,
			VolumeTotal:  100,
			IsBuyOrder:   true,
			Duration:     90,
			Issued:       issued,
		},
	}

	got := ComputeOrderDesk(player, nil, nil, nil, OrderDeskOptions{
		SalesTaxPercent:  8,
		BrokerFeePercent: 1,
		TargetETADays:    3,
		WarnExpiryDays:   2,
	})

	if len(got.Orders) != 1 {
		t.Fatalf("orders len = %d, want 1", len(got.Orders))
	}
	row := got.Orders[0]
	if row.ETADays != -1 {
		t.Fatalf("eta_days = %v, want -1 for unknown", row.ETADays)
	}
	if row.Recommendation != "cancel" {
		t.Fatalf("recommendation = %q, want cancel", row.Recommendation)
	}
	if got.Summary.NeedsCancel != 1 {
		t.Fatalf("summary needs_cancel = %d, want 1", got.Summary.NeedsCancel)
	}
}

func TestComputeOrderDesk_AvgDailyVolumeIncludesZeroDays(t *testing.T) {
	player := []esi.CharacterOrder{
		{
			OrderID:      3001,
			TypeID:       34,
			TypeName:     "Tritanium",
			LocationID:   60003760,
			LocationName: "Jita",
			RegionID:     10000002,
			Price:        100,
			VolumeRemain: 10,
			VolumeTotal:  10,
			IsBuyOrder:   false,
			Duration:     90,
			Issued:       time.Now().UTC().AddDate(0, 0, -1).Format(time.RFC3339),
		},
	}
	history := map[OrderDeskHistoryKey][]esi.HistoryEntry{
		NewOrderDeskHistoryKey(10000002, 34): {
			{Date: "2026-02-06", Volume: 70},
			{Date: "2026-02-07", Volume: 0},
		},
	}

	got := ComputeOrderDesk(player, nil, history, nil, OrderDeskOptions{
		TargetETADays:  3,
		WarnExpiryDays: 2,
	})

	if len(got.Orders) != 1 {
		t.Fatalf("orders len = %d, want 1", len(got.Orders))
	}
	row := got.Orders[0]
	if math.Abs(row.AvgDailyVolume-10.0) > 1e-6 {
		t.Fatalf("avg_daily_volume = %v, want 10", row.AvgDailyVolume)
	}
}

func TestComputeOrderDesk_UnavailableBookDoesNotAssumeTop(t *testing.T) {
	player := []esi.CharacterOrder{
		{
			OrderID:      4001,
			TypeID:       34,
			TypeName:     "Tritanium",
			LocationID:   60003760,
			LocationName: "Jita",
			RegionID:     10000002,
			Price:        100,
			VolumeRemain: 20,
			VolumeTotal:  20,
			IsBuyOrder:   false,
			Duration:     90,
			Issued:       time.Now().UTC().AddDate(0, 0, -1).Format(time.RFC3339),
		},
	}
	// Regional data exists but handler marked this (region,type) pair as unavailable.
	regional := []esi.MarketOrder{
		{OrderID: 5001, TypeID: 34, LocationID: 60003760, Price: 99, VolumeRemain: 5, IsBuyOrder: false},
	}
	unavailable := map[OrderDeskHistoryKey]bool{
		NewOrderDeskHistoryKey(10000002, 34): true,
	}

	got := ComputeOrderDesk(player, regional, nil, unavailable, OrderDeskOptions{
		TargetETADays:  3,
		WarnExpiryDays: 2,
	})

	if len(got.Orders) != 1 {
		t.Fatalf("orders len = %d, want 1", len(got.Orders))
	}
	row := got.Orders[0]
	if row.BookAvailable {
		t.Fatalf("book_available = true, want false")
	}
	if row.Position != 0 || row.TotalOrders != 0 {
		t.Fatalf("position/total = %d/%d, want 0/0", row.Position, row.TotalOrders)
	}
	if row.Recommendation != "hold" {
		t.Fatalf("recommendation = %q, want hold", row.Recommendation)
	}
}
