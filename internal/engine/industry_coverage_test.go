package engine

import "testing"

func TestComputeIndustryCoverageCoversMaterialsAndBPOBlueprint(t *testing.T) {
	got := ComputeIndustryCoverage(
		[]IndustryCoverageMaterialNeed{
			{TypeID: 34, TypeName: "Tritanium", RequiredQty: 100},
			{TypeID: 35, TypeName: "Pyerite", RequiredQty: 50},
		},
		[]IndustryCoverageBlueprintNeed{
			{BlueprintTypeID: 1001, BlueprintName: "Widget Blueprint", Activity: "manufacturing", RequiredRuns: 10},
		},
		map[int32]int64{
			34: 120,
			35: 50,
		},
		[]IndustryCoverageBlueprintStock{
			{BlueprintTypeID: 1001, BlueprintName: "Widget Blueprint", Quantity: 1, IsBPO: true, ME: 10, TE: 20},
		},
	)

	if !got.Summary.CanStartNow {
		t.Fatalf("CanStartNow = false, want true: %#v", got.Summary)
	}
	if got.Summary.MaterialsMissing != 0 || got.Summary.BlueprintsMissing != 0 {
		t.Fatalf("unexpected missing summary: %#v", got.Summary)
	}
	if got.Summary.MaterialCoveragePct != 100 {
		t.Fatalf("material coverage pct = %v, want 100", got.Summary.MaterialCoveragePct)
	}
	if len(got.Blueprints) != 1 || got.Blueprints[0].Status != "ready" || got.Blueprints[0].BPOQty != 1 {
		t.Fatalf("blueprint coverage = %#v", got.Blueprints)
	}
	if len(got.Actions) == 0 || got.Actions[len(got.Actions)-1].Action != "start_jobs" {
		t.Fatalf("actions = %#v, want final start_jobs", got.Actions)
	}
}

func TestComputeIndustryCoverageReportsShortagesAndBPCRuns(t *testing.T) {
	got := ComputeIndustryCoverage(
		[]IndustryCoverageMaterialNeed{
			{TypeID: 34, TypeName: "Tritanium", RequiredQty: 100},
			{TypeID: 34, TypeName: "Tritanium", RequiredQty: 50},
			{TypeID: 36, TypeName: "Mexallon", RequiredQty: 10},
		},
		[]IndustryCoverageBlueprintNeed{
			{BlueprintTypeID: 2001, BlueprintName: "Ship Blueprint", Activity: "manufacturing", RequiredRuns: 10},
		},
		map[int32]int64{
			34: 90,
		},
		[]IndustryCoverageBlueprintStock{
			{BlueprintTypeID: 2001, BlueprintName: "Ship Blueprint", Quantity: 1, IsBPO: false, AvailableRuns: 4, ME: 4, TE: 8},
		},
	)

	if got.Summary.CanStartNow {
		t.Fatalf("CanStartNow = true, want false")
	}
	if got.Summary.MaterialsMissing != 2 {
		t.Fatalf("MaterialsMissing = %d, want 2", got.Summary.MaterialsMissing)
	}
	if got.Summary.MissingUnits != 70 {
		t.Fatalf("MissingUnits = %d, want 70", got.Summary.MissingUnits)
	}
	if len(got.Blueprints) != 1 {
		t.Fatalf("blueprints len = %d, want 1", len(got.Blueprints))
	}
	if got.Blueprints[0].Status != "partial" || got.Blueprints[0].AvailableRuns != 4 || got.Blueprints[0].RequiredRuns != 10 {
		t.Fatalf("blueprint row = %#v", got.Blueprints[0])
	}
	foundBuy := false
	foundAcquireBP := false
	for _, action := range got.Actions {
		if action.Action == "buy_missing" && action.TypeID == 34 && action.Quantity == 60 && action.Blocking {
			foundBuy = true
		}
		if action.Action == "acquire_blueprint" && action.TypeID == 2001 && action.Blocking {
			foundAcquireBP = true
		}
	}
	if !foundBuy || !foundAcquireBP {
		t.Fatalf("actions = %#v, want buy_missing and acquire_blueprint blockers", got.Actions)
	}
}
