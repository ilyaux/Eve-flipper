package esi

import (
	"fmt"
	"io"
	"log"
	"net/http"
)

// OpenMarketWindow opens the market details window for a type_id in the EVE client.
// Requires esi-ui.open_window.v1 scope.
// POST https://esi.evetech.net/latest/ui/openwindow/marketdetails/?type_id=123
func (c *Client) OpenMarketWindow(typeID int64, accessToken string) error {
	c.sem <- struct{}{}
	defer func() { <-c.sem }()

	url := fmt.Sprintf("%s/ui/openwindow/marketdetails/?type_id=%d", baseURL, typeID)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", "eve-flipper/1.0 (github.com)")

	log.Printf("[ESI] Sending OpenMarketWindow: type_id=%d, url=%s", typeID, url)
	resp, err := c.http.Do(req)
	if err != nil {
		log.Printf("[ESI] OpenMarketWindow HTTP error: type_id=%d, err=%v", typeID, err)
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[ESI] OpenMarketWindow failed: type_id=%d, status=%d, body=%s", typeID, resp.StatusCode, string(body))
		if resp.StatusCode == 401 {
			return fmt.Errorf("unauthorized (401): missing scope esi-ui.open_window.v1 or token expired. Please re-login via EVE SSO. Details: %s", string(body))
		}
		return fmt.Errorf("ESI error: status %d, body: %s", resp.StatusCode, string(body))
	}

	log.Printf("[ESI] OpenMarketWindow success: type_id=%d, status=204", typeID)
	return nil
}

// SetWaypoint sets an autopilot waypoint in the EVE client.
// Requires esi-ui.write_waypoint.v1 scope.
// POST https://esi.evetech.net/latest/ui/autopilot/waypoint/?destination_id=123&clear_other_waypoints=false&add_to_beginning=false
func (c *Client) SetWaypoint(solarSystemID int64, clearOtherWaypoints, addToBeginning bool, accessToken string) error {
	c.sem <- struct{}{}
	defer func() { <-c.sem }()

	url := fmt.Sprintf("%s/ui/autopilot/waypoint/?destination_id=%d&clear_other_waypoints=%t&add_to_beginning=%t",
		baseURL, solarSystemID, clearOtherWaypoints, addToBeginning)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", "eve-flipper/1.0 (github.com)")

	log.Printf("[ESI] Sending SetWaypoint: system_id=%d, clear=%t, add_to_beginning=%t, url=%s",
		solarSystemID, clearOtherWaypoints, addToBeginning, url)
	resp, err := c.http.Do(req)
	if err != nil {
		log.Printf("[ESI] SetWaypoint HTTP error: system_id=%d, err=%v", solarSystemID, err)
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[ESI] SetWaypoint failed: system_id=%d, status=%d, body=%s", solarSystemID, resp.StatusCode, string(body))
		if resp.StatusCode == 401 {
			return fmt.Errorf("unauthorized (401): missing scope esi-ui.write_waypoint.v1 or token expired. Please re-login via EVE SSO. Details: %s", string(body))
		}
		return fmt.Errorf("ESI error: status %d, body: %s", resp.StatusCode, string(body))
	}

	log.Printf("[ESI] SetWaypoint success: system_id=%d, status=204", solarSystemID)
	return nil
}

// OpenContractWindow opens a contract window in the EVE client.
// Requires esi-ui.open_window.v1 scope.
// POST https://esi.evetech.net/latest/ui/openwindow/contract/?contract_id=123
func (c *Client) OpenContractWindow(contractID int64, accessToken string) error {
	c.sem <- struct{}{}
	defer func() { <-c.sem }()

	url := fmt.Sprintf("%s/ui/openwindow/contract/?contract_id=%d", baseURL, contractID)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", "eve-flipper/1.0 (github.com)")

	log.Printf("[ESI] Sending OpenContractWindow: contract_id=%d, url=%s", contractID, url)
	resp, err := c.http.Do(req)
	if err != nil {
		log.Printf("[ESI] OpenContractWindow HTTP error: contract_id=%d, err=%v", contractID, err)
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[ESI] OpenContractWindow failed: contract_id=%d, status=%d, body=%s", contractID, resp.StatusCode, string(body))
		if resp.StatusCode == 401 {
			return fmt.Errorf("unauthorized (401): missing scope esi-ui.open_window.v1 or token expired. Please re-login via EVE SSO. Details: %s", string(body))
		}
		return fmt.Errorf("ESI error: status %d, body: %s", resp.StatusCode, string(body))
	}

	log.Printf("[ESI] OpenContractWindow success: contract_id=%d, status=204", contractID)
	return nil
}
