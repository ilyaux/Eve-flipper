package api

import (
	"testing"
	"time"

	"eve-flipper/internal/db"
	"eve-flipper/internal/esi"
)

func TestReconcilePaperTradeWithRuntime_SuggestsSoldFromTransactions(t *testing.T) {
	created := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	trade := db.PaperTrade{
		ID:              42,
		Status:          db.PaperTradeStatusPlanned,
		TypeID:          34,
		TypeName:        "Tritanium",
		PlannedQuantity: 10,
		BuyLocationID:   60003760,
		SellLocationID:  60008494,
		ActualBuyPrice:  0,
		ActualSellPrice: 0,
		CreatedAt:       created,
	}
	runtime := paperTradeRuntime{
		Transactions: []esi.WalletTransaction{
			{
				Date:       time.Now().UTC().Add(-90 * time.Minute).Format(time.RFC3339),
				TypeID:     34,
				LocationID: 60003760,
				UnitPrice:  5,
				Quantity:   4,
				IsBuy:      true,
			},
			{
				Date:       time.Now().UTC().Add(-80 * time.Minute).Format(time.RFC3339),
				TypeID:     34,
				LocationID: 60003760,
				UnitPrice:  6,
				Quantity:   6,
				IsBuy:      true,
			},
			{
				Date:       time.Now().UTC().Add(-30 * time.Minute).Format(time.RFC3339),
				TypeID:     34,
				LocationID: 60008494,
				UnitPrice:  8,
				Quantity:   10,
				IsBuy:      false,
			},
		},
	}

	row := reconcilePaperTradeWithRuntime(trade, runtime)
	if row.SuggestedStatus != db.PaperTradeStatusSold {
		t.Fatalf("status = %q, want sold", row.SuggestedStatus)
	}
	if row.Confidence != "high" {
		t.Fatalf("confidence = %q, want high", row.Confidence)
	}
	if row.MatchedBuyQty != 10 || row.MatchedSellQty != 10 {
		t.Fatalf("matched buy/sell = %d/%d, want 10/10", row.MatchedBuyQty, row.MatchedSellQty)
	}
	if row.AvgBuyPrice != 5.6 || row.AvgSellPrice != 8 {
		t.Fatalf("avg buy/sell = %.2f/%.2f, want 5.60/8.00", row.AvgBuyPrice, row.AvgSellPrice)
	}
	if row.SuggestedPatch == nil {
		t.Fatal("expected suggested patch")
	}
	if row.SuggestedPatch.Status != db.PaperTradeStatusSold ||
		row.SuggestedPatch.ActualQuantity != 10 ||
		row.SuggestedPatch.ActualBuyPrice != 5.6 ||
		row.SuggestedPatch.ActualSellPrice != 8 {
		t.Fatalf("patch = %#v", row.SuggestedPatch)
	}
}

func TestReconcilePaperTradeWithRuntime_IgnoresOldTransactions(t *testing.T) {
	created := time.Now().UTC().Add(-2 * time.Hour)
	trade := db.PaperTrade{
		ID:              7,
		Status:          db.PaperTradeStatusPlanned,
		TypeID:          35,
		TypeName:        "Pyerite",
		PlannedQuantity: 10,
		CreatedAt:       created.Format(time.RFC3339),
	}
	runtime := paperTradeRuntime{
		Transactions: []esi.WalletTransaction{
			{
				Date:      created.Add(-24 * time.Hour).Format(time.RFC3339),
				TypeID:    35,
				UnitPrice: 3,
				Quantity:  10,
				IsBuy:     true,
			},
		},
		Orders: []esi.CharacterOrder{
			{
				TypeID:       35,
				VolumeRemain: 10,
				IsBuyOrder:   true,
			},
		},
	}

	row := reconcilePaperTradeWithRuntime(trade, runtime)
	if row.MatchedBuyQty != 0 {
		t.Fatalf("matched buy = %d, want 0", row.MatchedBuyQty)
	}
	if row.OpenBuyQty != 10 || row.SuggestedStatus != db.PaperTradeStatusPlanned || row.Confidence != "medium" {
		t.Fatalf("row = %#v", row)
	}
}
