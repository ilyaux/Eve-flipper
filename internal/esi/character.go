package esi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
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
}

// SkillEntry represents a single trained skill.
type SkillEntry struct {
	SkillID       int32 `json:"skill_id"`
	ActiveLevel   int   `json:"active_skill_level"`
	TrainedLevel  int   `json:"trained_skill_level"`
	SkillPoints   int64 `json:"skillpoints_in_skill"`
}

// SkillSheet is the character's skill data.
type SkillSheet struct {
	Skills     []SkillEntry `json:"skills"`
	TotalSP    int64        `json:"total_sp"`
	UnallocSP  int64        `json:"unallocated_sp"`
}

// GetCharacterOrders fetches a character's active market orders.
func GetCharacterOrders(characterID int64, accessToken string) ([]CharacterOrder, error) {
	url := fmt.Sprintf("%s/characters/%d/orders/?datasource=tranquility", baseURL, characterID)
	var orders []CharacterOrder
	if err := authGet(url, accessToken, &orders); err != nil {
		return nil, fmt.Errorf("character orders: %w", err)
	}
	return orders, nil
}

// GetWalletBalance fetches a character's ISK balance.
func GetWalletBalance(characterID int64, accessToken string) (float64, error) {
	url := fmt.Sprintf("%s/characters/%d/wallet/?datasource=tranquility", baseURL, characterID)
	var balance float64
	if err := authGet(url, accessToken, &balance); err != nil {
		return 0, fmt.Errorf("wallet: %w", err)
	}
	return balance, nil
}

// GetSkills fetches a character's trained skills.
func GetSkills(characterID int64, accessToken string) (*SkillSheet, error) {
	url := fmt.Sprintf("%s/characters/%d/skills/?datasource=tranquility", baseURL, characterID)
	var sheet SkillSheet
	if err := authGet(url, accessToken, &sheet); err != nil {
		return nil, fmt.Errorf("skills: %w", err)
	}
	return &sheet, nil
}

// authGet performs an authenticated GET request to an ESI endpoint.
func authGet(url, accessToken string, dst interface{}) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "eve-flipper/1.0 (github.com)")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ESI %d: %s", resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(dst)
}
