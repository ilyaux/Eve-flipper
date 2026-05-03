package engine

import "sort"

// IndustryCoverageMaterialNeed is a required material row from an industry plan.
type IndustryCoverageMaterialNeed struct {
	TypeID      int32  `json:"type_id"`
	TypeName    string `json:"type_name"`
	RequiredQty int64  `json:"required_qty"`
}

// IndustryCoverageBlueprintNeed is a required blueprint row from an industry activity plan.
type IndustryCoverageBlueprintNeed struct {
	BlueprintTypeID int32  `json:"blueprint_type_id"`
	BlueprintName   string `json:"blueprint_name"`
	Activity        string `json:"activity"`
	RequiredRuns    int64  `json:"required_runs"`
}

// IndustryCoverageBlueprintStock is an owned blueprint row aggregated from ESI.
type IndustryCoverageBlueprintStock struct {
	BlueprintTypeID int32  `json:"blueprint_type_id"`
	BlueprintName   string `json:"blueprint_name"`
	Quantity        int64  `json:"quantity"`
	IsBPO           bool   `json:"is_bpo"`
	AvailableRuns   int64  `json:"available_runs"`
	ME              int32  `json:"me"`
	TE              int32  `json:"te"`
}

type IndustryCoverageMaterialRow struct {
	TypeID       int32   `json:"type_id"`
	TypeName     string  `json:"type_name"`
	RequiredQty  int64   `json:"required_qty"`
	AvailableQty int64   `json:"available_qty"`
	MissingQty   int64   `json:"missing_qty"`
	CoveragePct  float64 `json:"coverage_pct"`
	Status       string  `json:"status"`
}

type IndustryCoverageBlueprintRow struct {
	BlueprintTypeID int32   `json:"blueprint_type_id"`
	BlueprintName   string  `json:"blueprint_name"`
	Activity        string  `json:"activity"`
	RequiredRuns    int64   `json:"required_runs"`
	OwnedQty        int64   `json:"owned_qty"`
	BPOQty          int64   `json:"bpo_qty"`
	BPCQty          int64   `json:"bpc_qty"`
	AvailableRuns   int64   `json:"available_runs"`
	BestME          int32   `json:"best_me"`
	BestTE          int32   `json:"best_te"`
	CoveragePct     float64 `json:"coverage_pct"`
	Status          string  `json:"status"`
}

type IndustryCoverageAction struct {
	Step         int    `json:"step"`
	Action       string `json:"action"`
	Status       string `json:"status"`
	Label        string `json:"label"`
	Detail       string `json:"detail,omitempty"`
	TypeID       int32  `json:"type_id,omitempty"`
	TypeName     string `json:"type_name,omitempty"`
	Quantity     int64  `json:"quantity,omitempty"`
	RequiredQty  int64  `json:"required_qty,omitempty"`
	AvailableQty int64  `json:"available_qty,omitempty"`
	MissingQty   int64  `json:"missing_qty,omitempty"`
	Blocking     bool   `json:"blocking"`
}

type IndustryCoverageSummary struct {
	Materials           int     `json:"materials"`
	MaterialsCovered    int     `json:"materials_covered"`
	MaterialsMissing    int     `json:"materials_missing"`
	RequiredUnits       int64   `json:"required_units"`
	AvailableUnits      int64   `json:"available_units"`
	MissingUnits        int64   `json:"missing_units"`
	MaterialCoveragePct float64 `json:"material_coverage_pct"`
	Blueprints          int     `json:"blueprints"`
	BlueprintsReady     int     `json:"blueprints_ready"`
	BlueprintsMissing   int     `json:"blueprints_missing"`
	CanStartNow         bool    `json:"can_start_now"`
}

type IndustryCoverageResult struct {
	Summary    IndustryCoverageSummary        `json:"summary"`
	Materials  []IndustryCoverageMaterialRow  `json:"materials"`
	Blueprints []IndustryCoverageBlueprintRow `json:"blueprints"`
	Actions    []IndustryCoverageAction       `json:"actions"`
	Warnings   []string                       `json:"warnings,omitempty"`
}

func ComputeIndustryCoverage(
	materialNeeds []IndustryCoverageMaterialNeed,
	blueprintNeeds []IndustryCoverageBlueprintNeed,
	assetsByType map[int32]int64,
	blueprintStock []IndustryCoverageBlueprintStock,
) IndustryCoverageResult {
	result := IndustryCoverageResult{}
	materialNeedByType := make(map[int32]IndustryCoverageMaterialNeed, len(materialNeeds))
	for _, need := range materialNeeds {
		if need.TypeID <= 0 || need.RequiredQty <= 0 {
			continue
		}
		row := materialNeedByType[need.TypeID]
		if row.TypeID == 0 {
			row.TypeID = need.TypeID
			row.TypeName = need.TypeName
		}
		if row.TypeName == "" {
			row.TypeName = need.TypeName
		}
		row.RequiredQty += need.RequiredQty
		materialNeedByType[need.TypeID] = row
	}

	result.Materials = make([]IndustryCoverageMaterialRow, 0, len(materialNeedByType))
	for _, need := range materialNeedByType {
		available := assetsByType[need.TypeID]
		if available < 0 {
			available = 0
		}
		missing := need.RequiredQty - available
		if missing < 0 {
			missing = 0
		}
		coveragePct := 100.0
		if need.RequiredQty > 0 {
			covered := available
			if covered > need.RequiredQty {
				covered = need.RequiredQty
			}
			coveragePct = float64(covered) / float64(need.RequiredQty) * 100
		}
		status := "covered"
		if missing > 0 {
			status = "partial"
			if available <= 0 {
				status = "missing"
			}
		}
		result.Materials = append(result.Materials, IndustryCoverageMaterialRow{
			TypeID:       need.TypeID,
			TypeName:     need.TypeName,
			RequiredQty:  need.RequiredQty,
			AvailableQty: available,
			MissingQty:   missing,
			CoveragePct:  coveragePct,
			Status:       status,
		})
		result.Summary.RequiredUnits += need.RequiredQty
		availableForSummary := available
		if availableForSummary > need.RequiredQty {
			availableForSummary = need.RequiredQty
		}
		result.Summary.AvailableUnits += availableForSummary
		result.Summary.MissingUnits += missing
		if missing == 0 {
			result.Summary.MaterialsCovered++
		} else {
			result.Summary.MaterialsMissing++
		}
	}
	sort.SliceStable(result.Materials, func(i, j int) bool {
		if result.Materials[i].MissingQty != result.Materials[j].MissingQty {
			return result.Materials[i].MissingQty > result.Materials[j].MissingQty
		}
		if result.Materials[i].RequiredQty != result.Materials[j].RequiredQty {
			return result.Materials[i].RequiredQty > result.Materials[j].RequiredQty
		}
		return result.Materials[i].TypeName < result.Materials[j].TypeName
	})
	result.Summary.Materials = len(result.Materials)
	if result.Summary.RequiredUnits > 0 {
		result.Summary.MaterialCoveragePct = float64(result.Summary.AvailableUnits) / float64(result.Summary.RequiredUnits) * 100
	} else {
		result.Summary.MaterialCoveragePct = 100
	}

	blueprintNeedByType := make(map[int32]IndustryCoverageBlueprintNeed, len(blueprintNeeds))
	for _, need := range blueprintNeeds {
		if need.BlueprintTypeID <= 0 {
			continue
		}
		if need.RequiredRuns <= 0 {
			need.RequiredRuns = 1
		}
		row := blueprintNeedByType[need.BlueprintTypeID]
		if row.BlueprintTypeID == 0 {
			row.BlueprintTypeID = need.BlueprintTypeID
			row.BlueprintName = need.BlueprintName
			row.Activity = need.Activity
		}
		if row.BlueprintName == "" {
			row.BlueprintName = need.BlueprintName
		}
		if row.Activity == "" {
			row.Activity = need.Activity
		} else if need.Activity != "" && row.Activity != need.Activity {
			row.Activity = "mixed"
		}
		row.RequiredRuns += need.RequiredRuns
		blueprintNeedByType[need.BlueprintTypeID] = row
	}

	type bpAgg struct {
		name          string
		ownedQty      int64
		bpoQty        int64
		bpcQty        int64
		availableRuns int64
		bestME        int32
		bestTE        int32
	}
	stockByType := make(map[int32]bpAgg, len(blueprintStock))
	for _, stock := range blueprintStock {
		if stock.BlueprintTypeID <= 0 {
			continue
		}
		qty := stock.Quantity
		if qty <= 0 {
			qty = 1
		}
		agg := stockByType[stock.BlueprintTypeID]
		if agg.name == "" {
			agg.name = stock.BlueprintName
		}
		agg.ownedQty += qty
		if stock.IsBPO {
			agg.bpoQty += qty
		} else {
			agg.bpcQty += qty
			if stock.AvailableRuns > 0 {
				agg.availableRuns += stock.AvailableRuns
			} else {
				agg.availableRuns += qty
			}
		}
		if stock.ME > agg.bestME {
			agg.bestME = stock.ME
		}
		if stock.TE > agg.bestTE {
			agg.bestTE = stock.TE
		}
		stockByType[stock.BlueprintTypeID] = agg
	}

	result.Blueprints = make([]IndustryCoverageBlueprintRow, 0, len(blueprintNeedByType))
	for _, need := range blueprintNeedByType {
		stock := stockByType[need.BlueprintTypeID]
		name := need.BlueprintName
		if name == "" {
			name = stock.name
		}
		ready := stock.bpoQty > 0 || stock.availableRuns >= need.RequiredRuns
		status := "ready"
		coveragePct := 100.0
		if !ready {
			status = "missing"
			if stock.availableRuns > 0 || stock.ownedQty > 0 {
				status = "partial"
			}
			if need.RequiredRuns > 0 {
				coveragePct = float64(stock.availableRuns) / float64(need.RequiredRuns) * 100
				if coveragePct > 100 {
					coveragePct = 100
				}
			}
		}
		result.Blueprints = append(result.Blueprints, IndustryCoverageBlueprintRow{
			BlueprintTypeID: need.BlueprintTypeID,
			BlueprintName:   name,
			Activity:        need.Activity,
			RequiredRuns:    need.RequiredRuns,
			OwnedQty:        stock.ownedQty,
			BPOQty:          stock.bpoQty,
			BPCQty:          stock.bpcQty,
			AvailableRuns:   stock.availableRuns,
			BestME:          stock.bestME,
			BestTE:          stock.bestTE,
			CoveragePct:     coveragePct,
			Status:          status,
		})
		if ready {
			result.Summary.BlueprintsReady++
		} else {
			result.Summary.BlueprintsMissing++
		}
	}
	sort.SliceStable(result.Blueprints, func(i, j int) bool {
		leftReady := result.Blueprints[i].Status == "ready"
		rightReady := result.Blueprints[j].Status == "ready"
		if leftReady != rightReady {
			return !leftReady
		}
		if result.Blueprints[i].RequiredRuns != result.Blueprints[j].RequiredRuns {
			return result.Blueprints[i].RequiredRuns > result.Blueprints[j].RequiredRuns
		}
		return result.Blueprints[i].BlueprintName < result.Blueprints[j].BlueprintName
	})
	result.Summary.Blueprints = len(result.Blueprints)
	result.Summary.CanStartNow = result.Summary.MaterialsMissing == 0 && result.Summary.BlueprintsMissing == 0
	result.Actions = buildIndustryCoverageActions(result)
	return result
}

func buildIndustryCoverageActions(result IndustryCoverageResult) []IndustryCoverageAction {
	actions := make([]IndustryCoverageAction, 0, len(result.Materials)*2+len(result.Blueprints)+1)
	add := func(action IndustryCoverageAction) {
		action.Step = len(actions) + 1
		actions = append(actions, action)
	}

	for _, row := range result.Materials {
		useQty := row.AvailableQty
		if useQty > row.RequiredQty {
			useQty = row.RequiredQty
		}
		if useQty > 0 {
			add(IndustryCoverageAction{
				Action:       "use_stock",
				Status:       "ready",
				Label:        "Use stock",
				Detail:       row.TypeName,
				TypeID:       row.TypeID,
				TypeName:     row.TypeName,
				Quantity:     useQty,
				RequiredQty:  row.RequiredQty,
				AvailableQty: row.AvailableQty,
				MissingQty:   row.MissingQty,
			})
		}
		if row.MissingQty > 0 {
			add(IndustryCoverageAction{
				Action:       "buy_missing",
				Status:       "needed",
				Label:        "Buy missing",
				Detail:       row.TypeName,
				TypeID:       row.TypeID,
				TypeName:     row.TypeName,
				Quantity:     row.MissingQty,
				RequiredQty:  row.RequiredQty,
				AvailableQty: row.AvailableQty,
				MissingQty:   row.MissingQty,
				Blocking:     true,
			})
		}
	}

	for _, row := range result.Blueprints {
		if row.Status == "ready" {
			detail := "BPO"
			if row.BPOQty <= 0 {
				detail = "BPC runs"
			}
			add(IndustryCoverageAction{
				Action:       "use_blueprint",
				Status:       "ready",
				Label:        "Use blueprint",
				Detail:       detail + ": " + row.BlueprintName,
				TypeID:       row.BlueprintTypeID,
				TypeName:     row.BlueprintName,
				Quantity:     row.RequiredRuns,
				RequiredQty:  row.RequiredRuns,
				AvailableQty: row.AvailableRuns,
			})
			continue
		}
		add(IndustryCoverageAction{
			Action:       "acquire_blueprint",
			Status:       row.Status,
			Label:        "Acquire blueprint",
			Detail:       row.BlueprintName,
			TypeID:       row.BlueprintTypeID,
			TypeName:     row.BlueprintName,
			Quantity:     row.RequiredRuns,
			RequiredQty:  row.RequiredRuns,
			AvailableQty: row.AvailableRuns,
			MissingQty:   row.RequiredRuns - row.AvailableRuns,
			Blocking:     true,
		})
	}

	if result.Summary.CanStartNow {
		add(IndustryCoverageAction{
			Action:   "start_jobs",
			Status:   "ready",
			Label:    "Start jobs",
			Detail:   "materials and blueprints covered",
			Quantity: int64(result.Summary.Blueprints),
		})
	} else {
		add(IndustryCoverageAction{
			Action:     "resolve_blockers",
			Status:     "blocked",
			Label:      "Resolve blockers",
			Detail:     "buy missing stock or acquire blueprint capacity",
			MissingQty: result.Summary.MissingUnits,
			Blocking:   true,
		})
	}
	return actions
}
