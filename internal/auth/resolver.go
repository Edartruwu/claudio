package auth

import (
	"os/exec"
	"strings"

	"github.com/Abraxas-365/claudio/internal/auth/oauth"
	"github.com/Abraxas-365/claudio/internal/auth/storage"
	"github.com/Abraxas-365/claudio/internal/config"
)

// Source identifies where an auth credential came from.
type Source string

const (
	SourceAnthropicAuthToken Source = "ANTHROPIC_AUTH_TOKEN"
	SourceOAuthTokenEnv      Source = "CLAUDIO_OAUTH_TOKEN"
	SourceAPIKeyHelper       Source = "apiKeyHelper"
	SourceAnthropicAPIKey    Source = "ANTHROPIC_API_KEY"
	SourceKeychainAPIKey     Source = "keychain"
	SourceOAuthKeychain      Source = "oauth"
	SourceNone               Source = "none"
)

// Result holds the resolved authentication credential.
type Result struct {
	Source  Source
	Token   string
	IsOAuth bool
}

// Resolver resolves authentication credentials from multiple sources.
type Resolver struct {
	env     config.Env
	storage storage.SecureStorage
}

// NewResolver creates a new auth resolver.
func NewResolver(store storage.SecureStorage) *Resolver {
	return &Resolver{
		env:     config.GetEnv(),
		storage: store,
	}
}

// Resolve returns the highest-priority available auth credential.
// Priority (highest to lowest):
// 1. ANTHROPIC_AUTH_TOKEN env var
// 2. CLAUDIO_OAUTH_TOKEN env var
// 3. apiKeyHelper command
// 4. ANTHROPIC_API_KEY env var
// 5. API key from keychain/credentials
// 6. OAuth tokens from keychain/credentials
func (r *Resolver) Resolve() Result {
	// 1. ANTHROPIC_AUTH_TOKEN
	if token := r.env.AnthropicAuthToken(); token != "" {
		return Result{Source: SourceAnthropicAuthToken, Token: token, IsOAuth: true}
	}

	// 2. CLAUDIO_OAUTH_TOKEN
	if token := r.env.OAuthToken(); token != "" {
		return Result{Source: SourceOAuthTokenEnv, Token: token, IsOAuth: true}
	}

	// 3. API key helper command
	if helper := r.env.APIKeyHelper(); helper != "" {
		if key, err := runAPIKeyHelper(helper); err == nil && key != "" {
			return Result{Source: SourceAPIKeyHelper, Token: key, IsOAuth: false}
		}
	}

	// 4. ANTHROPIC_API_KEY env var
	if key := r.env.AnthropicAPIKey(); key != "" {
		return Result{Source: SourceAnthropicAPIKey, Token: key, IsOAuth: false}
	}

	// 5. API key from storage
	if key, err := r.storage.ReadAPIKey(); err == nil && key != "" {
		return Result{Source: SourceKeychainAPIKey, Token: key, IsOAuth: false}
	}

	// 6. OAuth tokens from storage
	if tokens, err := r.storage.ReadTokens(); err == nil && tokens != nil && tokens.AccessToken != "" {
		// Check if token has inference scope
		if hasScope(tokens.Scopes, "user:inference") {
			return Result{Source: SourceOAuthKeychain, Token: tokens.AccessToken, IsOAuth: true}
		}
	}

	return Result{Source: SourceNone}
}

// IsLoggedIn reports whether any auth credential is available.
func (r *Resolver) IsLoggedIn() bool {
	result := r.Resolve()
	return result.Source != SourceNone
}

// GetOAuthTokens returns OAuth tokens from storage (for refresh checks).
func (r *Resolver) GetOAuthTokens() (*oauth.Tokens, error) {
	return r.storage.ReadTokens()
}

func runAPIKeyHelper(command string) (string, error) {
	parts := strings.Fields(command)
	cmd := exec.Command(parts[0], parts[1:]...)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func hasScope(scopes []string, target string) bool {
	for _, s := range scopes {
		if s == target {
			return true
		}
	}
	return false
}
