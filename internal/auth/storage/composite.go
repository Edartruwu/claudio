package storage

import (
	"runtime"

	"github.com/Abraxas-365/claudio/internal/auth/oauth"
	"github.com/Abraxas-365/claudio/internal/config"
)

// CompositeStorage tries keychain first, falls back to plaintext.
type CompositeStorage struct {
	primary  SecureStorage
	fallback SecureStorage
}

// NewDefaultStorage creates the platform-appropriate secure storage.
func NewDefaultStorage() SecureStorage {
	paths := config.GetPaths()
	plaintext := NewPlaintextStorage(paths.Credentials)

	switch runtime.GOOS {
	case "darwin", "linux":
		keychain := NewKeychainStorage()
		return &CompositeStorage{
			primary:  keychain,
			fallback: plaintext,
		}
	default:
		return plaintext
	}
}

func (s *CompositeStorage) Name() string {
	return s.primary.Name() + " (with fallback)"
}

func (s *CompositeStorage) ReadTokens() (*oauth.Tokens, error) {
	tokens, err := s.primary.ReadTokens()
	if err == nil && tokens != nil {
		return tokens, nil
	}
	return s.fallback.ReadTokens()
}

func (s *CompositeStorage) SaveTokens(tokens *oauth.Tokens) error {
	if err := s.primary.SaveTokens(tokens); err != nil {
		return s.fallback.SaveTokens(tokens)
	}
	return nil
}

func (s *CompositeStorage) ReadAPIKey() (string, error) {
	key, err := s.primary.ReadAPIKey()
	if err == nil && key != "" {
		return key, nil
	}
	return s.fallback.ReadAPIKey()
}

func (s *CompositeStorage) SaveAPIKey(key string) error {
	if err := s.primary.SaveAPIKey(key); err != nil {
		return s.fallback.SaveAPIKey(key)
	}
	return nil
}

func (s *CompositeStorage) Delete() error {
	s.primary.Delete()
	s.fallback.Delete()
	return nil
}
