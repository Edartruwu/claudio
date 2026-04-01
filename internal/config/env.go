package config

import "os"

// Env provides typed access to environment variables.
type Env struct{}

// AnthropicAPIKey returns the ANTHROPIC_API_KEY env var.
func (Env) AnthropicAPIKey() string {
	return os.Getenv("ANTHROPIC_API_KEY")
}

// AnthropicAuthToken returns the ANTHROPIC_AUTH_TOKEN env var.
func (Env) AnthropicAuthToken() string {
	return os.Getenv("ANTHROPIC_AUTH_TOKEN")
}

// OAuthToken returns the CLAUDIO_OAUTH_TOKEN env var.
func (Env) OAuthToken() string {
	return os.Getenv("CLAUDIO_OAUTH_TOKEN")
}

// OAuthRefreshToken returns the CLAUDIO_OAUTH_REFRESH_TOKEN env var.
func (Env) OAuthRefreshToken() string {
	return os.Getenv("CLAUDIO_OAUTH_REFRESH_TOKEN")
}

// OAuthScopes returns the CLAUDIO_OAUTH_SCOPES env var.
func (Env) OAuthScopes() string {
	return os.Getenv("CLAUDIO_OAUTH_SCOPES")
}

// APIKeyHelper returns the CLAUDIO_API_KEY_HELPER env var (command to run).
func (Env) APIKeyHelper() string {
	return os.Getenv("CLAUDIO_API_KEY_HELPER")
}

// Model returns the CLAUDIO_MODEL env var.
func (Env) Model() string {
	return os.Getenv("CLAUDIO_MODEL")
}

// APIBaseURL returns the CLAUDIO_API_BASE_URL env var.
func (Env) APIBaseURL() string {
	return os.Getenv("CLAUDIO_API_BASE_URL")
}

// HookProfile returns the CLAUDIO_HOOK_PROFILE env var.
func (Env) HookProfile() string {
	return os.Getenv("CLAUDIO_HOOK_PROFILE")
}

// DisabledHooks returns the CLAUDIO_DISABLED_HOOKS env var.
func (Env) DisabledHooks() string {
	return os.Getenv("CLAUDIO_DISABLED_HOOKS")
}

// GetEnv returns the global Env accessor.
func GetEnv() Env {
	return Env{}
}
