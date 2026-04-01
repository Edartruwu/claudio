package oauth

import (
	"context"
	"fmt"

	"github.com/pkg/browser"
)

// Service orchestrates the full OAuth PKCE login flow.
type Service struct {
	client *Client
	config Config
}

// NewService creates a new OAuth service.
func NewService(cfg Config) *Service {
	return &Service{
		client: NewClient(cfg),
		config: cfg,
	}
}

// Login performs the full OAuth PKCE flow:
// 1. Generate PKCE verifier + challenge
// 2. Start local callback server
// 3. Open browser to authorize URL
// 4. Wait for callback with auth code
// 5. Exchange code for tokens
// 6. Fetch user profile
func (s *Service) Login(ctx context.Context) (*Tokens, error) {
	// Step 1: Generate PKCE
	verifier, err := GenerateCodeVerifier()
	if err != nil {
		return nil, fmt.Errorf("failed to generate code verifier: %w", err)
	}
	challenge := GenerateCodeChallenge(verifier)

	state, err := GenerateState()
	if err != nil {
		return nil, fmt.Errorf("failed to generate state: %w", err)
	}

	// Step 2: Start callback listener
	listener := NewAuthCodeListener()
	port, err := listener.Start()
	if err != nil {
		return nil, fmt.Errorf("failed to start callback listener: %w", err)
	}
	defer listener.Shutdown()

	redirectURI := listener.RedirectURI()

	// Step 3: Build and open authorize URL
	authorizeURL := s.client.BuildAuthorizeURL(challenge, state, redirectURI)

	fmt.Printf("Opening browser to log in...\n")
	fmt.Printf("If the browser doesn't open, visit:\n%s\n\n", authorizeURL)

	if err := browser.OpenURL(authorizeURL); err != nil {
		fmt.Printf("Could not open browser automatically. Please visit the URL above.\n")
	}

	fmt.Printf("Waiting for authentication on port %d...\n", port)

	// Step 4: Wait for callback
	code, err := listener.WaitForCode(ctx, state)
	if err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	// Step 5: Exchange code for tokens
	tokens, err := s.client.ExchangeCode(code, verifier, redirectURI)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}

	// Step 6: Fetch profile
	profile, err := s.client.FetchProfile(tokens.AccessToken)
	if err != nil {
		// Non-fatal: profile fetch failure shouldn't block login
		fmt.Printf("Warning: could not fetch profile: %v\n", err)
	} else {
		tokens.Profile = profile
		if profile.SubscriptionType != "" {
			tokens.SubscriptionType = profile.SubscriptionType
		}
		if profile.RateLimitTier != "" {
			tokens.RateLimitTier = profile.RateLimitTier
		}
	}

	fmt.Printf("Login successful!\n")
	return tokens, nil
}

// RefreshIfNeeded checks token expiry and refreshes if needed.
func (s *Service) RefreshIfNeeded(tokens *Tokens) (*Tokens, bool, error) {
	if !tokens.IsExpired(5 * 60 * 1e9) { // 5 minute buffer
		return tokens, false, nil
	}
	if tokens.RefreshToken == "" {
		return tokens, false, fmt.Errorf("token expired and no refresh token available")
	}

	newTokens, err := s.client.RefreshToken(tokens.RefreshToken, tokens.Scopes)
	if err != nil {
		return tokens, false, err
	}

	// Preserve fields not returned by refresh
	if newTokens.Account == nil {
		newTokens.Account = tokens.Account
	}
	if newTokens.Profile == nil {
		newTokens.Profile = tokens.Profile
	}
	if newTokens.SubscriptionType == "" {
		newTokens.SubscriptionType = tokens.SubscriptionType
	}

	return newTokens, true, nil
}
