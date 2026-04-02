package oauth

import "os"

const (
	// DefaultClientID is the OAuth client ID for Claude Code.
	DefaultClientID = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"

	// DefaultAuthorizeURL is the OAuth authorization endpoint.
	// Uses claude.com/cai/ path to match official Claude Code's attribution flow.
	DefaultAuthorizeURL = "https://claude.com/cai/oauth/authorize"

	// DefaultTokenURL is the OAuth token exchange endpoint.
	DefaultTokenURL = "https://platform.claude.com/v1/oauth/token"

	// DefaultProfileURL is the user profile endpoint.
	DefaultProfileURL = "https://api.anthropic.com/api/oauth/profile"

	// DefaultAPIKeyURL is the endpoint to create an API key from an OAuth token.
	DefaultAPIKeyURL = "https://api.anthropic.com/api/oauth/claude_cli/create_api_key"

	// DefaultManualRedirectURI is used when browser redirect isn't available.
	DefaultManualRedirectURI = "https://platform.claude.com/oauth/code/callback"
)

// DefaultScopes for Claude.ai subscribers.
var DefaultScopes = []string{
	"user:profile",
	"user:inference",
	"user:sessions:claude_code",
	"user:mcp_servers",
	"user:file_upload",
}

// ConsoleScopes for Anthropic Console (API key generation).
var ConsoleScopes = []string{
	"org:create_api_key",
	"user:profile",
}

// Config holds OAuth configuration, which can be overridden via env vars.
type Config struct {
	ClientID     string
	AuthorizeURL string
	TokenURL     string
	ProfileURL   string
	APIKeyURL    string
	Scopes       []string
}

// DefaultConfig returns the standard OAuth config with env var overrides.
func DefaultConfig() Config {
	cfg := Config{
		ClientID:     DefaultClientID,
		AuthorizeURL: DefaultAuthorizeURL,
		TokenURL:     DefaultTokenURL,
		ProfileURL:   DefaultProfileURL,
		APIKeyURL:    DefaultAPIKeyURL,
		Scopes:       DefaultScopes,
	}

	if v := os.Getenv("CLAUDIO_OAUTH_CLIENT_ID"); v != "" {
		cfg.ClientID = v
	}
	if v := os.Getenv("CLAUDIO_CUSTOM_OAUTH_URL"); v != "" {
		cfg.AuthorizeURL = v + "/authorize"
		cfg.TokenURL = v + "/token"
	}

	return cfg
}
