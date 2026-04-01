package oauth

import "time"

// Tokens holds OAuth token data returned from the token endpoint.
type Tokens struct {
	AccessToken      string    `json:"accessToken"`
	RefreshToken     string    `json:"refreshToken,omitempty"`
	ExpiresAt        time.Time `json:"expiresAt,omitempty"`
	Scopes           []string  `json:"scopes,omitempty"`
	SubscriptionType string    `json:"subscriptionType,omitempty"` // pro, max, team, enterprise
	RateLimitTier    string    `json:"rateLimitTier,omitempty"`
	Account          *Account  `json:"tokenAccount,omitempty"`
	Profile          *Profile  `json:"profile,omitempty"`
}

// Account holds OAuth account info from the token exchange response.
type Account struct {
	UUID             string `json:"uuid"`
	EmailAddress     string `json:"emailAddress"`
	OrganizationUUID string `json:"organizationUuid"`
}

// Profile holds user profile info from the profile endpoint.
type Profile struct {
	Name             string `json:"name,omitempty"`
	Email            string `json:"email,omitempty"`
	SubscriptionType string `json:"subscription_type,omitempty"`
	RateLimitTier    string `json:"rate_limit_tier,omitempty"`
}

// TokenExchangeResponse is the raw response from /v1/oauth/token.
type TokenExchangeResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	TokenType    string `json:"token_type"`
	Account      *struct {
		UUID             string `json:"uuid"`
		EmailAddress     string `json:"email_address"`
		OrganizationUUID string `json:"organization_uuid"`
	} `json:"account,omitempty"`
}

// IsExpired reports whether the access token has expired or will expire within the given buffer.
func (t *Tokens) IsExpired(buffer time.Duration) bool {
	if t.ExpiresAt.IsZero() {
		return false // No expiry set
	}
	return time.Now().Add(buffer).After(t.ExpiresAt)
}
