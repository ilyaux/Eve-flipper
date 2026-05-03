package esi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

// CharacterOrder represents a character's market order.
type CharacterOrder struct {
	OrderID      int64   `json:"order_id"`
	TypeID       int32   `json:"type_id"`
	LocationID   int64   `json:"location_id"`
	RegionID     int32   `json:"region_id"`
	Price        float64 `json:"price"`
	VolumeRemain int32   `json:"volume_remain"`
	VolumeTotal  int32   `json:"volume_total"`
	IsBuyOrder   bool    `json:"is_buy_order"`
	Duration     int     `json:"duration"`
	Issued       string  `json:"issued"`
	// Enriched fields (filled by server)
	TypeName     string `json:"type_name,omitempty"`
	LocationName string `json:"location_name,omitempty"`
}

// HistoricalOrder represents a completed/cancelled/expired order.
type HistoricalOrder struct {
	OrderID      int64   `json:"order_id"`
	TypeID       int32   `json:"type_id"`
	LocationID   int64   `json:"location_id"`
	RegionID     int32   `json:"region_id"`
	Price        float64 `json:"price"`
	VolumeRemain int32   `json:"volume_remain"`
	VolumeTotal  int32   `json:"volume_total"`
	IsBuyOrder   bool    `json:"is_buy_order"`
	State        string  `json:"state"` // cancelled, expired, fulfilled
	Issued       string  `json:"issued"`
	// Enriched fields
	TypeName     string `json:"type_name,omitempty"`
	LocationName string `json:"location_name,omitempty"`
}

// WalletTransaction represents a wallet transaction.
type WalletTransaction struct {
	TransactionID int64   `json:"transaction_id"`
	Date          string  `json:"date"`
	TypeID        int32   `json:"type_id"`
	LocationID    int64   `json:"location_id"`
	UnitPrice     float64 `json:"unit_price"`
	Quantity      int32   `json:"quantity"`
	IsBuy         bool    `json:"is_buy"`
	// Enriched fields
	TypeName     string `json:"type_name,omitempty"`
	LocationName string `json:"location_name,omitempty"`
}

// CharacterAsset represents an asset row from character inventory.
type CharacterAsset struct {
	ItemID          int64  `json:"item_id"`
	TypeID          int32  `json:"type_id"`
	LocationID      int64  `json:"location_id"`
	LocationType    string `json:"location_type"`
	LocationFlag    string `json:"location_flag"`
	Quantity        int64  `json:"quantity"`
	IsSingleton     bool   `json:"is_singleton"`
	IsBlueprintCopy bool   `json:"is_blueprint_copy"`
	TypeName        string `json:"type_name,omitempty"`
	LocationName    string `json:"location_name,omitempty"`
}

// CharacterBlueprint represents a blueprint owned by character.
type CharacterBlueprint struct {
	ItemID             int64  `json:"item_id"`
	TypeID             int32  `json:"type_id"`
	LocationID         int64  `json:"location_id"`
	LocationFlag       string `json:"location_flag"`
	Quantity           int64  `json:"quantity"`
	TimeEfficiency     int32  `json:"time_efficiency"`
	MaterialEfficiency int32  `json:"material_efficiency"`
	Runs               int64  `json:"runs"`
}

// CharacterIndustryJob represents an active or recently completed character industry job.
type CharacterIndustryJob struct {
	JobID                int64   `json:"job_id"`
	InstallerID          int64   `json:"installer_id"`
	FacilityID           int64   `json:"facility_id"`
	StationID            int64   `json:"station_id,omitempty"`
	ActivityID           int32   `json:"activity_id"`
	BlueprintID          int64   `json:"blueprint_id"`
	BlueprintTypeID      int32   `json:"blueprint_type_id"`
	BlueprintLocationID  int64   `json:"blueprint_location_id"`
	OutputLocationID     int64   `json:"output_location_id"`
	Runs                 int32   `json:"runs"`
	Cost                 float64 `json:"cost"`
	LicensedRuns         int32   `json:"licensed_runs,omitempty"`
	Probability          float64 `json:"probability,omitempty"`
	ProductTypeID        int32   `json:"product_type_id,omitempty"`
	Status               string  `json:"status"`
	Duration             int64   `json:"duration"`
	StartDate            string  `json:"start_date"`
	EndDate              string  `json:"end_date"`
	PauseDate            string  `json:"pause_date,omitempty"`
	CompletedDate        string  `json:"completed_date,omitempty"`
	CompletedCharacterID int64   `json:"completed_character_id,omitempty"`
	SuccessfulRuns       int32   `json:"successful_runs,omitempty"`
	ProductTypeName      string  `json:"product_type_name,omitempty"`
	BlueprintTypeName    string  `json:"blueprint_type_name,omitempty"`
	FacilityName         string  `json:"facility_name,omitempty"`
}

// SkillEntry represents a single trained skill.
type SkillEntry struct {
	SkillID      int32 `json:"skill_id"`
	ActiveLevel  int   `json:"active_skill_level"`
	TrainedLevel int   `json:"trained_skill_level"`
	SkillPoints  int64 `json:"skillpoints_in_skill"`
}

// SkillSheet is the character's skill data.
type SkillSheet struct {
	Skills    []SkillEntry `json:"skills"`
	TotalSP   int64        `json:"total_sp"`
	UnallocSP int64        `json:"unallocated_sp"`
}

// CharacterLocation represents the character's current location.
type CharacterLocation struct {
	SolarSystemID int32 `json:"solar_system_id"`
	StationID     int64 `json:"station_id,omitempty"`
	StructureID   int64 `json:"structure_id,omitempty"`
}

// --- Authenticated requests using the shared Client transport ---

// AuthGetJSON performs an authenticated GET to an ESI endpoint using the shared HTTP
// transport (connection pooling, keep-alive). Uses the lightweight semaphore so that
// character API calls never compete with bulk scan page fetches.
func (c *Client) AuthGetJSON(url, accessToken string, dst interface{}) error {
	c.sem <- struct{}{} // acquire lightweight semaphore

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		<-c.sem
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "eve-flipper/1.0 (github.com)")

	resp, err := c.http.Do(req)
	if err != nil {
		<-c.sem
		return err
	}

	statusCode := resp.StatusCode
	if statusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		<-c.sem
		return fmt.Errorf("ESI %d: %s", statusCode, string(body))
	}

	decErr := json.NewDecoder(resp.Body).Decode(dst)
	resp.Body.Close()
	<-c.sem
	return decErr
}

// GetCharacterOrders fetches a character's active market orders.
func (c *Client) GetCharacterOrders(characterID int64, accessToken string) ([]CharacterOrder, error) {
	url := fmt.Sprintf("%s/characters/%d/orders/?datasource=tranquility", baseURL, characterID)
	var orders []CharacterOrder
	if err := c.AuthGetJSON(url, accessToken, &orders); err != nil {
		return nil, fmt.Errorf("character orders: %w", err)
	}
	return orders, nil
}

// GetWalletBalance fetches a character's ISK balance.
func (c *Client) GetWalletBalance(characterID int64, accessToken string) (float64, error) {
	url := fmt.Sprintf("%s/characters/%d/wallet/?datasource=tranquility", baseURL, characterID)
	var balance float64
	if err := c.AuthGetJSON(url, accessToken, &balance); err != nil {
		return 0, fmt.Errorf("wallet: %w", err)
	}
	return balance, nil
}

// GetSkills fetches a character's trained skills.
func (c *Client) GetSkills(characterID int64, accessToken string) (*SkillSheet, error) {
	url := fmt.Sprintf("%s/characters/%d/skills/?datasource=tranquility", baseURL, characterID)
	var sheet SkillSheet
	if err := c.AuthGetJSON(url, accessToken, &sheet); err != nil {
		return nil, fmt.Errorf("skills: %w", err)
	}
	return &sheet, nil
}

// GetOrderHistory fetches all pages of a character's completed/cancelled/expired orders.
// ESI may return multiple pages via X-Pages header; this fetches them all concurrently.
func (c *Client) GetOrderHistory(characterID int64, accessToken string) ([]HistoricalOrder, error) {
	historyURL := fmt.Sprintf("%s/characters/%d/orders/history/?datasource=tranquility", baseURL, characterID)

	// Fetch page 1 to discover total pages.
	c.sem <- struct{}{}

	req, err := http.NewRequest("GET", historyURL+"&page=1", nil)
	if err != nil {
		<-c.sem
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "eve-flipper/1.0 (github.com)")

	resp, err := c.http.Do(req)
	if err != nil {
		<-c.sem
		return nil, fmt.Errorf("order history page 1: %w", err)
	}

	totalPages := 1
	if p := resp.Header.Get("X-Pages"); p != "" {
		if tp, parseErr := strconv.Atoi(p); parseErr == nil && tp > 1 {
			totalPages = tp
		}
	}

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		<-c.sem
		return nil, fmt.Errorf("order history: ESI %d: %s", resp.StatusCode, string(body))
	}

	var page1 []HistoricalOrder
	decErr := json.NewDecoder(resp.Body).Decode(&page1)
	resp.Body.Close()
	<-c.sem

	if decErr != nil {
		return nil, fmt.Errorf("order history decode: %w", decErr)
	}

	if totalPages <= 1 {
		return page1, nil
	}

	// Fetch remaining pages concurrently.
	type pageResult struct {
		data []HistoricalOrder
		err  error
	}
	results := make(chan pageResult, totalPages-1)

	for p := 2; p <= totalPages; p++ {
		go func(pageNum int) {
			pageURL := fmt.Sprintf("%s&page=%d", historyURL, pageNum)
			var data []HistoricalOrder
			if fetchErr := c.AuthGetJSON(pageURL, accessToken, &data); fetchErr != nil {
				results <- pageResult{err: fetchErr}
				return
			}
			results <- pageResult{data: data}
		}(p)
	}

	all := make([]HistoricalOrder, 0, len(page1)*totalPages)
	all = append(all, page1...)
	for i := 0; i < totalPages-1; i++ {
		r := <-results
		if r.err != nil {
			continue // skip failed pages, return what we have
		}
		all = append(all, r.data...)
	}
	return all, nil
}

// GetWalletTransactions fetches a character's wallet transactions.
func (c *Client) GetWalletTransactions(characterID int64, accessToken string) ([]WalletTransaction, error) {
	url := fmt.Sprintf("%s/characters/%d/wallet/transactions/?datasource=tranquility", baseURL, characterID)
	var txns []WalletTransaction
	if err := c.AuthGetJSON(url, accessToken, &txns); err != nil {
		return nil, fmt.Errorf("wallet transactions: %w", err)
	}
	return txns, nil
}

// GetCharacterAssets fetches all pages of character assets.
func (c *Client) GetCharacterAssets(characterID int64, accessToken string) ([]CharacterAsset, error) {
	assetsURL := fmt.Sprintf("%s/characters/%d/assets/?datasource=tranquility", baseURL, characterID)

	// Fetch page 1 to discover total pages.
	c.sem <- struct{}{}

	req, err := http.NewRequest("GET", assetsURL+"&page=1", nil)
	if err != nil {
		<-c.sem
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "eve-flipper/1.0 (github.com)")

	resp, err := c.http.Do(req)
	if err != nil {
		<-c.sem
		return nil, fmt.Errorf("assets page 1: %w", err)
	}

	totalPages := 1
	if p := resp.Header.Get("X-Pages"); p != "" {
		if tp, parseErr := strconv.Atoi(p); parseErr == nil && tp > 1 {
			totalPages = tp
		}
	}

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		<-c.sem
		return nil, fmt.Errorf("assets: ESI %d: %s", resp.StatusCode, string(body))
	}

	var page1 []CharacterAsset
	decErr := json.NewDecoder(resp.Body).Decode(&page1)
	resp.Body.Close()
	<-c.sem
	if decErr != nil {
		return nil, fmt.Errorf("assets decode: %w", decErr)
	}

	if totalPages <= 1 {
		return page1, nil
	}

	type pageResult struct {
		data []CharacterAsset
		err  error
	}
	results := make(chan pageResult, totalPages-1)

	for p := 2; p <= totalPages; p++ {
		go func(pageNum int) {
			pageURL := fmt.Sprintf("%s&page=%d", assetsURL, pageNum)
			var data []CharacterAsset
			if fetchErr := c.AuthGetJSON(pageURL, accessToken, &data); fetchErr != nil {
				results <- pageResult{err: fetchErr}
				return
			}
			results <- pageResult{data: data}
		}(p)
	}

	all := make([]CharacterAsset, 0, len(page1)*totalPages)
	all = append(all, page1...)
	for i := 0; i < totalPages-1; i++ {
		r := <-results
		if r.err != nil {
			continue // keep partial success behavior
		}
		all = append(all, r.data...)
	}
	return all, nil
}

// GetCharacterBlueprints fetches all pages of character blueprints.
func (c *Client) GetCharacterBlueprints(characterID int64, accessToken string) ([]CharacterBlueprint, error) {
	blueprintsURL := fmt.Sprintf("%s/characters/%d/blueprints/?datasource=tranquility", baseURL, characterID)

	// Fetch page 1 to discover total pages.
	c.sem <- struct{}{}

	req, err := http.NewRequest("GET", blueprintsURL+"&page=1", nil)
	if err != nil {
		<-c.sem
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "eve-flipper/1.0 (github.com)")

	resp, err := c.http.Do(req)
	if err != nil {
		<-c.sem
		return nil, fmt.Errorf("blueprints page 1: %w", err)
	}

	totalPages := 1
	if p := resp.Header.Get("X-Pages"); p != "" {
		if tp, parseErr := strconv.Atoi(p); parseErr == nil && tp > 1 {
			totalPages = tp
		}
	}

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		<-c.sem
		return nil, fmt.Errorf("blueprints: ESI %d: %s", resp.StatusCode, string(body))
	}

	var page1 []CharacterBlueprint
	decErr := json.NewDecoder(resp.Body).Decode(&page1)
	resp.Body.Close()
	<-c.sem
	if decErr != nil {
		return nil, fmt.Errorf("blueprints decode: %w", decErr)
	}

	if totalPages <= 1 {
		return page1, nil
	}

	type pageResult struct {
		data []CharacterBlueprint
		err  error
	}
	results := make(chan pageResult, totalPages-1)

	for p := 2; p <= totalPages; p++ {
		go func(pageNum int) {
			pageURL := fmt.Sprintf("%s&page=%d", blueprintsURL, pageNum)
			var data []CharacterBlueprint
			if fetchErr := c.AuthGetJSON(pageURL, accessToken, &data); fetchErr != nil {
				results <- pageResult{err: fetchErr}
				return
			}
			results <- pageResult{data: data}
		}(p)
	}

	all := make([]CharacterBlueprint, 0, len(page1)*totalPages)
	all = append(all, page1...)
	for i := 0; i < totalPages-1; i++ {
		r := <-results
		if r.err != nil {
			continue // keep partial success behavior
		}
		all = append(all, r.data...)
	}
	return all, nil
}

// GetCharacterIndustryJobs fetches a character's industry jobs.
func (c *Client) GetCharacterIndustryJobs(characterID int64, accessToken string, includeCompleted bool) ([]CharacterIndustryJob, error) {
	url := fmt.Sprintf("%s/characters/%d/industry/jobs/?datasource=tranquility&include_completed=%t", baseURL, characterID, includeCompleted)
	var jobs []CharacterIndustryJob
	if err := c.AuthGetJSON(url, accessToken, &jobs); err != nil {
		return nil, fmt.Errorf("character industry jobs: %w", err)
	}
	return jobs, nil
}

// GetCharacterLocation fetches a character's current location (system/station).
func (c *Client) GetCharacterLocation(characterID int64, accessToken string) (*CharacterLocation, error) {
	url := fmt.Sprintf("%s/characters/%d/location/?datasource=tranquility", baseURL, characterID)
	var loc CharacterLocation
	if err := c.AuthGetJSON(url, accessToken, &loc); err != nil {
		return nil, fmt.Errorf("location: %w", err)
	}
	return &loc, nil
}

// CharacterRolesResponse represents the character's corporation roles.
type CharacterRolesResponse struct {
	Roles []string `json:"roles"`
}

// GetCharacterRoles fetches a character's corporation roles.
// Requires esi-characters.read_corporation_roles.v1 scope.
func (c *Client) GetCharacterRoles(characterID int64, accessToken string) (*CharacterRolesResponse, error) {
	url := fmt.Sprintf("%s/characters/%d/roles/?datasource=tranquility", baseURL, characterID)
	var roles CharacterRolesResponse
	if err := c.AuthGetJSON(url, accessToken, &roles); err != nil {
		return nil, fmt.Errorf("character roles: %w", err)
	}
	return &roles, nil
}

// GetCharacterCorporationID fetches which corporation a character belongs to.
func (c *Client) GetCharacterCorporationID(characterID int64) (int32, error) {
	url := fmt.Sprintf("%s/characters/%d/?datasource=tranquility", baseURL, characterID)
	var info struct {
		CorporationID int32 `json:"corporation_id"`
	}
	if err := c.GetJSON(url, &info); err != nil {
		return 0, fmt.Errorf("character info: %w", err)
	}
	return info.CorporationID, nil
}
