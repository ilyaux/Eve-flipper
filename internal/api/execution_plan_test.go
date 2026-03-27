package api

import (
	"testing"

	"eve-flipper/internal/esi"
)

func TestFilterExecutionPlanOrders_StructureScopedLocation(t *testing.T) {
	const (
		typeID      = int32(40520)
		structureID = int64(1_024_000_000_001)
		otherLocID  = int64(1_024_000_000_002)
	)

	orders := []esi.MarketOrder{
		{TypeID: typeID, LocationID: structureID, SystemID: 30000142, Price: 700},
		{TypeID: typeID, LocationID: otherLocID, SystemID: 30000142, Price: 710},
		{TypeID: 34, LocationID: structureID, SystemID: 30000142, Price: 1},
	}

	got := filterExecutionPlanOrders(orders, typeID, 0, structureID)

	if len(got) != 1 {
		t.Fatalf("filtered length = %d, want 1", len(got))
	}
	if got[0].LocationID != structureID {
		t.Fatalf("location_id = %d, want %d", got[0].LocationID, structureID)
	}
}

func TestFilterExecutionPlanOrders_SystemScopeUsesRequestedSystem(t *testing.T) {
	const typeID = int32(40520)

	orders := []esi.MarketOrder{
		{TypeID: typeID, SystemID: 30000142, LocationID: 60003760, Price: 700},
		{TypeID: typeID, SystemID: 30002187, LocationID: 60008494, Price: 710},
		{TypeID: 34, SystemID: 30000142, LocationID: 60003760, Price: 1},
	}

	got := filterExecutionPlanOrders(orders, typeID, 30000142, 0)

	if len(got) != 1 {
		t.Fatalf("filtered length = %d, want 1", len(got))
	}
	if got[0].SystemID != 30000142 {
		t.Fatalf("system_id = %d, want 30000142", got[0].SystemID)
	}
}

func TestFilterExecutionPlanOrders_RegionScopeIncludesAllSystems(t *testing.T) {
	const typeID = int32(40520)

	orders := []esi.MarketOrder{
		{TypeID: typeID, SystemID: 30000142, LocationID: 60003760, Price: 700},
		{TypeID: typeID, SystemID: 30002187, LocationID: 60008494, Price: 710},
		{TypeID: 34, SystemID: 30000142, LocationID: 60003760, Price: 1},
	}

	got := filterExecutionPlanOrders(orders, typeID, 0, 0)

	if len(got) != 2 {
		t.Fatalf("filtered length = %d, want 2", len(got))
	}
}
