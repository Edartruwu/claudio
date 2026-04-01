package storage

import (
	"github.com/Abraxas-365/claudio/internal/auth/oauth"
)

// SecureStorage provides secure credential storage.
type SecureStorage interface {
	Name() string
	ReadTokens() (*oauth.Tokens, error)
	SaveTokens(tokens *oauth.Tokens) error
	ReadAPIKey() (string, error)
	SaveAPIKey(key string) error
	Delete() error
}
