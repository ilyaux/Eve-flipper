package sde

import (
	"encoding/json"
	"fmt"
	"math"

	"eve-flipper/internal/logger"
)

// Blueprint represents a manufacturing blueprint from the SDE.
type Blueprint struct {
	BlueprintTypeID int32                    // The blueprint item type ID
	ProductTypeID   int32                    // What this blueprint produces
	ProductQuantity int32                    // How many items are produced per run
	Materials       []BlueprintMaterial      // Required materials for manufacturing
	Time            int32                    // Manufacturing time in seconds (base)
	Activities      map[string]*ActivityData // All activities (manufacturing, copying, invention, etc.)
}

// BlueprintMaterial represents a single material requirement.
type BlueprintMaterial struct {
	TypeID   int32 // Material type ID
	Quantity int32 // Base quantity required (before ME)
}

// ActivityData holds data for a specific blueprint activity.
type ActivityData struct {
	Time      int32               // Base time in seconds
	Materials []BlueprintMaterial // Materials for this activity
	Products  []BlueprintProduct  // Products of this activity
	Skills    []BlueprintSkill    // Required skills
}

// BlueprintProduct represents what an activity produces.
type BlueprintProduct struct {
	TypeID      int32
	Quantity    int32
	Probability float64 // For invention, etc.
}

// BlueprintSkill represents a required skill.
type BlueprintSkill struct {
	TypeID int32
	Level  int32
}

// ReprocessingMaterial represents ore -> mineral conversion.
type ReprocessingMaterial struct {
	TypeID int32 // Source type (ore)
	Yields []MaterialYield
}

// MaterialYield represents a single material output from reprocessing.
type MaterialYield struct {
	TypeID   int32 // Output material type
	Quantity int32 // Base quantity (100% yield)
}

// IndustryData holds all industry-related SDE data.
type IndustryData struct {
	Blueprints         map[int32]*Blueprint            // blueprintTypeID -> Blueprint
	ProductToBlueprint map[int32]int32                 // productTypeID -> blueprintTypeID
	Reprocessing       map[int32]*ReprocessingMaterial // oreTypeID -> yields
	BaseCategories     map[int32]bool                  // categoryIDs that are "base" materials (minerals, PI, etc.)
}

// NewIndustryData creates a new IndustryData instance.
func NewIndustryData() *IndustryData {
	return &IndustryData{
		Blueprints:         make(map[int32]*Blueprint),
		ProductToBlueprint: make(map[int32]int32),
		Reprocessing:       make(map[int32]*ReprocessingMaterial),
		BaseCategories:     make(map[int32]bool),
	}
}

// LoadIndustry loads industry-related data from the SDE.
func (d *Data) LoadIndustry(extractDir string) (*IndustryData, error) {
	ind := NewIndustryData()

	if err := ind.loadBlueprints(extractDir); err != nil {
		return nil, fmt.Errorf("load blueprints: %w", err)
	}

	if err := ind.loadReprocessing(extractDir); err != nil {
		return nil, fmt.Errorf("load reprocessing: %w", err)
	}

	return ind, nil
}

// loadBlueprints loads blueprint data from SDE
// Tries multiple file names as SDE format varies
func (ind *IndustryData) loadBlueprints(dir string) error {
	// Try different possible file names
	fileNames := []string{"industryBlueprints", "blueprints", "industryActivityMaterials"}

	for _, name := range fileNames {
		count := 0
		err := readJSONL(dir, name, func(raw json.RawMessage) error {
			count++
			return ind.parseBlueprintLine(raw)
		})
		if err != nil {
			return err
		}
		if count > 0 {
			logger.Info("SDE", fmt.Sprintf("Loaded %d blueprints from %s.jsonl", count, name))
			return nil
		}
	}

	logger.Warn("SDE", "No blueprint files found")
	return nil
}

// parseBlueprintLine parses a single blueprint JSON line
func (ind *IndustryData) parseBlueprintLine(raw json.RawMessage) error {
	// SDE structure:
	// {
	//   "_key": blueprintTypeID,
	//   "activities": {
	//     "manufacturing": {
	//       "time": 3600,
	//       "materials": [{"typeID": 34, "quantity": 1000}, ...],
	//       "products": [{"typeID": 645, "quantity": 1}]
	//     },
	//     "copying": {...},
	//     ...
	//   }
	// }
	var bp struct {
		Key        int32 `json:"_key"`
		Activities struct {
			Manufacturing *struct {
				Time      int32 `json:"time"`
				Materials []struct {
					TypeID   int32 `json:"typeID"`
					Quantity int32 `json:"quantity"`
				} `json:"materials"`
				Products []struct {
					TypeID   int32 `json:"typeID"`
					Quantity int32 `json:"quantity"`
				} `json:"products"`
				Skills []struct {
					TypeID int32 `json:"typeID"`
					Level  int32 `json:"level"`
				} `json:"skills"`
			} `json:"manufacturing"`
			Invention *struct {
				Time      int32 `json:"time"`
				Materials []struct {
					TypeID   int32 `json:"typeID"`
					Quantity int32 `json:"quantity"`
				} `json:"materials"`
				Products []struct {
					TypeID      int32   `json:"typeID"`
					Quantity    int32   `json:"quantity"`
					Probability float64 `json:"probability"`
				} `json:"products"`
			} `json:"invention"`
			Reaction *struct {
				Time      int32 `json:"time"`
				Materials []struct {
					TypeID   int32 `json:"typeID"`
					Quantity int32 `json:"quantity"`
				} `json:"materials"`
				Products []struct {
					TypeID   int32 `json:"typeID"`
					Quantity int32 `json:"quantity"`
				} `json:"products"`
			} `json:"reaction"`
		} `json:"activities"`
	}

	if err := json.Unmarshal(raw, &bp); err != nil {
		return err
	}

	blueprint := &Blueprint{
		BlueprintTypeID: bp.Key,
		Activities:      make(map[string]*ActivityData),
	}

	// Process manufacturing activity (main focus)
	if mfg := bp.Activities.Manufacturing; mfg != nil {
		blueprint.Time = mfg.Time

		for _, m := range mfg.Materials {
			blueprint.Materials = append(blueprint.Materials, BlueprintMaterial{
				TypeID:   m.TypeID,
				Quantity: m.Quantity,
			})
		}

		if len(mfg.Products) > 0 {
			blueprint.ProductTypeID = mfg.Products[0].TypeID
			blueprint.ProductQuantity = mfg.Products[0].Quantity
			if blueprint.ProductQuantity == 0 {
				blueprint.ProductQuantity = 1
			}
		}

		// Store full activity data
		actData := &ActivityData{
			Time: mfg.Time,
		}
		for _, m := range mfg.Materials {
			actData.Materials = append(actData.Materials, BlueprintMaterial{
				TypeID: m.TypeID, Quantity: m.Quantity,
			})
		}
		for _, p := range mfg.Products {
			actData.Products = append(actData.Products, BlueprintProduct{
				TypeID: p.TypeID, Quantity: p.Quantity,
			})
		}
		for _, s := range mfg.Skills {
			actData.Skills = append(actData.Skills, BlueprintSkill{
				TypeID: s.TypeID, Level: s.Level,
			})
		}
		blueprint.Activities["manufacturing"] = actData
	}

	// Process reaction activity (for moon goo, etc.)
	if rxn := bp.Activities.Reaction; rxn != nil {
		actData := &ActivityData{
			Time: rxn.Time,
		}
		for _, m := range rxn.Materials {
			actData.Materials = append(actData.Materials, BlueprintMaterial{
				TypeID: m.TypeID, Quantity: m.Quantity,
			})
		}
		for _, p := range rxn.Products {
			actData.Products = append(actData.Products, BlueprintProduct{
				TypeID: p.TypeID, Quantity: p.Quantity,
			})
		}
		blueprint.Activities["reaction"] = actData

		// If no manufacturing product, use reaction product
		if blueprint.ProductTypeID == 0 && len(rxn.Products) > 0 {
			blueprint.ProductTypeID = rxn.Products[0].TypeID
			blueprint.ProductQuantity = rxn.Products[0].Quantity
		}
	}

	// Process invention activity. Invention products are usually blueprint copies,
	// so they are kept in Activities but are not mapped as normal build products.
	if inv := bp.Activities.Invention; inv != nil {
		actData := &ActivityData{
			Time: inv.Time,
		}
		for _, m := range inv.Materials {
			actData.Materials = append(actData.Materials, BlueprintMaterial{
				TypeID: m.TypeID, Quantity: m.Quantity,
			})
		}
		for _, p := range inv.Products {
			actData.Products = append(actData.Products, BlueprintProduct{
				TypeID: p.TypeID, Quantity: p.Quantity, Probability: p.Probability,
			})
		}
		blueprint.Activities["invention"] = actData
	}

	// Only store blueprints that produce something
	if blueprint.ProductTypeID != 0 {
		ind.Blueprints[bp.Key] = blueprint
		if mfg := blueprint.Activities["manufacturing"]; mfg != nil {
			for _, product := range mfg.Products {
				if product.TypeID != 0 {
					ind.ProductToBlueprint[product.TypeID] = bp.Key
				}
			}
		}
		if rxn := blueprint.Activities["reaction"]; rxn != nil {
			for _, product := range rxn.Products {
				if product.TypeID != 0 {
					ind.ProductToBlueprint[product.TypeID] = bp.Key
				}
			}
		}
	}

	return nil
}

// loadReprocessing loads reprocessing/refining data from typeMaterials.jsonl
func (ind *IndustryData) loadReprocessing(dir string) error {
	return readJSONL(dir, "typeMaterials", func(raw json.RawMessage) error {
		// SDE structure:
		// {
		//   "_key": typeID (ore),
		//   "materials": [{"typeID": 34, "quantity": 1000}, ...]
		// }
		var tm struct {
			Key       int32 `json:"_key"`
			Materials []struct {
				TypeID   int32 `json:"typeID"`
				Quantity int32 `json:"quantity"`
			} `json:"materials"`
		}

		if err := json.Unmarshal(raw, &tm); err != nil {
			return err
		}

		if len(tm.Materials) == 0 {
			return nil
		}

		rm := &ReprocessingMaterial{
			TypeID: tm.Key,
		}
		for _, m := range tm.Materials {
			rm.Yields = append(rm.Yields, MaterialYield{
				TypeID:   m.TypeID,
				Quantity: m.Quantity,
			})
		}
		ind.Reprocessing[tm.Key] = rm

		return nil
	})
}

// GetBlueprintForProduct returns the blueprint that produces the given type.
func (ind *IndustryData) GetBlueprintForProduct(typeID int32) (*Blueprint, bool) {
	bpID, ok := ind.ProductToBlueprint[typeID]
	if !ok {
		return nil, false
	}
	bp, ok := ind.Blueprints[bpID]
	return bp, ok
}

// CalculateMaterialsWithME calculates required materials with Material Efficiency applied.
// ME ranges from 0-10 (each level reduces materials by 1%).
func (bp *Blueprint) CalculateMaterialsWithME(runs int32, me int32) []BlueprintMaterial {
	return bp.CalculateMaterialsWithMEAndStructure(runs, me, 0)
}

// CalculateMaterialsWithMEAndStructure calculates required materials with both ME and
// structure bonus applied in a single step (before ceiling) to avoid rounding errors.
// EVE formula: max(runs, ceil(base * runs * (1 - ME/100) * (1 - structureBonus/100)))
func (bp *Blueprint) CalculateMaterialsWithMEAndStructure(runs int32, me int32, structureBonus float64) []BlueprintMaterial {
	if me < 0 {
		me = 0
	}
	if me > 10 {
		me = 10
	}
	if structureBonus < 0 {
		structureBonus = 0
	}

	meMultiplier := 1.0 - float64(me)/100.0
	structureMultiplier := 1.0 - structureBonus/100.0

	result := make([]BlueprintMaterial, len(bp.Materials))

	for i, mat := range bp.Materials {
		// Combined formula: max(runs, ceil(base * runs * (1 - ME/100) * (1 - structureBonus/100)))
		baseQty := float64(mat.Quantity) * float64(runs) * meMultiplier * structureMultiplier
		finalQty := int32(math.Ceil(baseQty))
		if finalQty < runs {
			finalQty = runs
		}
		result[i] = BlueprintMaterial{
			TypeID:   mat.TypeID,
			Quantity: finalQty,
		}
	}

	return result
}

// CalculateTimeWithTE calculates manufacturing time with Time Efficiency applied.
// TE ranges from 0-20 (each level reduces time by 1%).
func (bp *Blueprint) CalculateTimeWithTE(runs int32, te int32) int32 {
	if te < 0 {
		te = 0
	}
	if te > 20 {
		te = 20
	}

	teMultiplier := 1.0 - float64(te)/100.0
	return int32(float64(bp.Time) * float64(runs) * teMultiplier)
}

// IsBaseMaterial returns true if the type is a "base" material that cannot be further broken down
// (minerals, PI materials, moon materials, etc.)
func (ind *IndustryData) IsBaseMaterial(typeID int32) bool {
	// Check if there's no blueprint to make this item
	_, hasBlueprint := ind.ProductToBlueprint[typeID]
	return !hasBlueprint
}
