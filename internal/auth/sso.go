package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	authorizeURL = "https://login.eveonline.com/v2/oauth/authorize"
	tokenURL     = "https://login.eveonline.com/v2/oauth/token"
	verifyURL    = "https://login.eveonline.com/oauth/verify"
)

// SSOConfig holds EVE SSO OAuth2 configuration.
type SSOConfig struct {
	ClientID     string
	ClientSecret string
	CallbackURL  string
	Scopes       string
}

// TokenResponse is the response from the EVE SSO token endpoint.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// CharacterInfo is the response from token verification.
type CharacterInfo struct {
	CharacterID   int64  `json:"CharacterID"`
	CharacterName string `json:"CharacterName"`
}

// GenerateState creates a random state string for CSRF protection.
func GenerateState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

// BuildAuthURL constructs the EVE SSO authorization URL.
func (c *SSOConfig) BuildAuthURL(state string) string {
	params := url.Values{
		"response_type": {"code"},
		"redirect_uri":  {c.CallbackURL},
		"client_id":     {c.ClientID},
		"scope":         {c.Scopes},
		"state":         {state},
	}
	return authorizeURL + "?" + params.Encode()
}

// ExchangeCode exchanges an authorization code for tokens.
func (c *SSOConfig) ExchangeCode(code string) (*TokenResponse, error) {
	data := url.Values{
		"grant_type": {"authorization_code"},
		"code":       {code},
	}
	return c.tokenRequest(data)
}

// RefreshToken refreshes an expired access token.
func (c *SSOConfig) RefreshToken(refreshToken string) (*TokenResponse, error) {
	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}
	return c.tokenRequest(data)
}

func (c *SSOConfig) tokenRequest(data url.Values) (*TokenResponse, error) {
	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(c.ClientID, c.ClientSecret)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("token request failed (%d): %s", resp.StatusCode, string(body))
	}

	var tok TokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}
	return &tok, nil
}

// VerifyToken verifies an access token and returns character info.
func VerifyToken(accessToken string) (*CharacterInfo, error) {
	req, err := http.NewRequest("GET", verifyURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("verify token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("verify failed: status %d", resp.StatusCode)
	}

	var info CharacterInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("parse verify: %w", err)
	}
	return &info, nil
}
