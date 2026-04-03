package provider

import (
	"os"
	"strings"
)

// Config defines the settings for a provider loaded from configuration.
type Config struct {
	APIBase string `json:"apiBase"`         // Base URL (e.g. "https://api.groq.com/openai/v1")
	APIKey  string `json:"apiKey,omitempty"` // API key or "$ENV_VAR" reference
	Type    string `json:"type"`            // "openai" or "anthropic"
}

// ResolveAPIKey resolves the API key, dereferencing env var references.
func (c Config) ResolveAPIKey() string {
	if strings.HasPrefix(c.APIKey, "$") {
		return os.Getenv(c.APIKey[1:])
	}
	return c.APIKey
}
