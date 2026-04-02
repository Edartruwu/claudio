package oauth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client handles OAuth token operations.
type Client struct {
	config     Config
	httpClient *http.Client
}

// NewClient creates a new OAuth client with the given config.
func NewClient(cfg Config) *Client {
	return &Client{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// BuildAuthorizeURL constructs the OAuth authorization URL with PKCE parameters.
func (c *Client) BuildAuthorizeURL(challenge, state, redirectURI string) string {
	params := url.Values{
		"client_id":             {c.config.ClientID},
		"response_type":        {"code"},
		"code_challenge":       {challenge},
		"code_challenge_method": {"S256"},
		"redirect_uri":         {redirectURI},
		"state":                {state},
		"scope":                {strings.Join(c.config.Scopes, " ")},
	}
	return c.config.AuthorizeURL + "?" + params.Encode()
}

// ExchangeCode exchanges an authorization code for tokens.
func (c *Client) ExchangeCode(code, verifier, redirectURI, state string) (*Tokens, error) {
	body := map[string]string{
		"grant_type":    "authorization_code",
		"code":          code,
		"code_verifier": verifier,
		"client_id":     c.config.ClientID,
		"redirect_uri":  redirectURI,
		"state":         state,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal token request: %w", err)
	}

	req, err := http.NewRequest("POST", c.config.TokenURL, strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var tokenResp TokenExchangeResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	tokens := &Tokens{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		Scopes:       strings.Split(tokenResp.Scope, " "),
	}
	if tokenResp.ExpiresIn > 0 {
		tokens.ExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	}
	if tokenResp.Account != nil {
		tokens.Account = &Account{
			UUID:             tokenResp.Account.UUID,
			EmailAddress:     tokenResp.Account.EmailAddress,
			OrganizationUUID: tokenResp.Account.OrganizationUUID,
		}
	}

	return tokens, nil
}

// RefreshToken refreshes an expired access token.
func (c *Client) RefreshToken(refreshToken string, scopes []string) (*Tokens, error) {
	body := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
		"client_id":     c.config.ClientID,
		"scope":         strings.Join(scopes, " "),
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal refresh request: %w", err)
	}

	req, err := http.NewRequest("POST", c.config.TokenURL, strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token refresh failed (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var tokenResp TokenExchangeResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode refresh response: %w", err)
	}

	tokens := &Tokens{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		Scopes:       strings.Split(tokenResp.Scope, " "),
	}
	if tokenResp.ExpiresIn > 0 {
		tokens.ExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	}

	return tokens, nil
}

// CreateAPIKey exchanges an OAuth access token for a persistent API key.
func (c *Client) CreateAPIKey(accessToken string) (string, error) {
	req, err := http.NewRequest("POST", c.config.APIKeyURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create API key request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("API key creation request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API key creation failed (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		RawKey string `json:"raw_key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode API key response: %w", err)
	}

	if result.RawKey == "" {
		return "", fmt.Errorf("server returned empty API key")
	}

	return result.RawKey, nil
}

// FetchProfile retrieves the user profile using an access token.
func (c *Client) FetchProfile(accessToken string) (*Profile, error) {
	req, err := http.NewRequest("GET", c.config.ProfileURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("profile request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("profile fetch failed (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var profile Profile
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return nil, fmt.Errorf("failed to decode profile: %w", err)
	}

	return &profile, nil
}
