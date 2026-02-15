package esi

import (
	"fmt"
	"io"
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

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == 401 {
			return fmt.Errorf("unauthorized (401): missing scope esi-ui.open_window.v1 or token expired. Please re-login via EVE SSO. Details: %s", string(body))
		}
		return fmt.Errorf("ESI error: status %d, body: %s", resp.StatusCode, string(body))
	}

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

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == 401 {
			return fmt.Errorf("unauthorized (401): missing scope esi-ui.open_window.v1 or token expired. Please re-login via EVE SSO. Details: %s", string(body))
		}
		return fmt.Errorf("ESI error: status %d, body: %s", resp.StatusCode, string(body))
	}

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

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == 401 {
			return fmt.Errorf("unauthorized (401): missing scope esi-ui.open_window.v1 or token expired. Please re-login via EVE SSO. Details: %s", string(body))
		}
		return fmt.Errorf("ESI error: status %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}
