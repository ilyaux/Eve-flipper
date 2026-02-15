package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
)

// ContractItemResponse represents a contract item in the API response
type ContractItemResponse struct {
	TypeID             int32   `json:"type_id"`
	TypeName           string  `json:"type_name"`
	Quantity           int32   `json:"quantity"`
	IsIncluded         bool    `json:"is_included"`
	IsBlueprintCopy    bool    `json:"is_blueprint_copy"`
	RecordID           int64   `json:"record_id"`
	ItemID             int64   `json:"item_id"`
	MaterialEfficiency int     `json:"material_efficiency,omitempty"`
	TimeEfficiency     int     `json:"time_efficiency,omitempty"`
	Runs               int     `json:"runs,omitempty"`
	Flag               int     `json:"flag,omitempty"`
	Singleton          bool    `json:"singleton,omitempty"`
	Damage             float64 `json:"damage,omitempty"`
}

// ContractDetailsResponse represents contract details with items
type ContractDetailsResponse struct {
	ContractID int32                  `json:"contract_id"`
	Items      []ContractItemResponse `json:"items"`
}

// handleGetContractItems returns the items for a specific contract
// GET /api/contracts/{contract_id}/items
func (s *Server) handleGetContractItems(w http.ResponseWriter, r *http.Request) {
	// Extract contract_id from path
	contractIDStr := r.PathValue("contract_id")
	contractID, err := strconv.ParseInt(contractIDStr, 10, 32)
	if err != nil {
		http.Error(w, `{"error":"invalid_contract_id"}`, http.StatusBadRequest)
		return
	}

	// Fetch contract items from ESI
	items, err := s.esi.FetchContractItems(int32(contractID))
	if err != nil {
		log.Printf("[API] FetchContractItems error: contract_id=%d, err=%v", contractID, err)
		http.Error(w, `{"error":"esi_error"}`, http.StatusInternalServerError)
		return
	}

	// Convert to response format with type names
	responseItems := make([]ContractItemResponse, 0, len(items))
	for _, item := range items {
		typeName := ""
		if t, ok := s.sdeData.Types[item.TypeID]; ok {
			typeName = t.Name
		}

		resp := ContractItemResponse{
			TypeID:          item.TypeID,
			TypeName:        typeName,
			Quantity:        item.Quantity,
			IsIncluded:      item.IsIncluded,
			IsBlueprintCopy: item.IsBlueprintCopy,
			RecordID:        item.RecordID,
			ItemID:          item.ItemID,
		}

		// Only include blueprint fields if relevant
		if item.IsBlueprintCopy || item.MaterialEfficiency > 0 || item.TimeEfficiency > 0 || item.Runs > 0 {
			resp.MaterialEfficiency = item.MaterialEfficiency
			resp.TimeEfficiency = item.TimeEfficiency
			resp.Runs = item.Runs
		}
		if item.Flag != 0 {
			resp.Flag = item.Flag
		}
		if item.Singleton {
			resp.Singleton = true
		}
		if item.Damage > 0 {
			resp.Damage = item.Damage
		}

		responseItems = append(responseItems, resp)
	}

	response := ContractDetailsResponse{
		ContractID: int32(contractID),
		Items:      responseItems,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
