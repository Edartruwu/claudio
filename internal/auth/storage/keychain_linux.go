//go:build linux

package storage

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/Abraxas-365/claudio/internal/auth/oauth"
)

const (
	secretLabel     = "Claudio credentials"
	secretAttrKey   = "application"
	secretAttrValue = "claudio"
)

// KeychainStorage stores credentials using libsecret (secret-tool) on Linux.
type KeychainStorage struct {
	mu        sync.Mutex
	cache     *storageData
	cacheTime time.Time
}

// NewKeychainStorage creates a Linux keychain storage.
// Falls back to plaintext if secret-tool is not available.
func NewKeychainStorage() SecureStorage {
	// Check if secret-tool is available
	if _, err := exec.LookPath("secret-tool"); err != nil {
		return NewPlaintextStorage(defaultCredentialsPath())
	}
	return &KeychainStorage{}
}

func defaultCredentialsPath() string {
	return "" // Will be set by the composite storage
}

func (s *KeychainStorage) Name() string {
	return "Linux Secret Service"
}

func (s *KeychainStorage) ReadTokens() (*oauth.Tokens, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.read()
	if err != nil {
		return nil, err
	}
	return data.OAuthTokens, nil
}

func (s *KeychainStorage) SaveTokens(tokens *oauth.Tokens) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, _ := s.read()
	data.OAuthTokens = tokens
	return s.write(data)
}

func (s *KeychainStorage) ReadAPIKey() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.read()
	if err != nil {
		return "", err
	}
	return data.APIKey, nil
}

func (s *KeychainStorage) SaveAPIKey(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, _ := s.read()
	data.APIKey = key
	return s.write(data)
}

func (s *KeychainStorage) Delete() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cache = nil
	cmd := exec.Command("secret-tool", "clear", secretAttrKey, secretAttrValue)
	cmd.Run() // Ignore error (may not exist)
	return nil
}

func (s *KeychainStorage) read() (*storageData, error) {
	if s.cache != nil && time.Since(s.cacheTime) < cacheTTL {
		return s.cache, nil
	}

	cmd := exec.Command("secret-tool", "lookup", secretAttrKey, secretAttrValue)
	output, err := cmd.Output()
	if err != nil {
		return &storageData{}, nil // Not found
	}

	hexStr := strings.TrimSpace(string(output))
	if hexStr == "" {
		return &storageData{}, nil
	}

	raw, err := hex.DecodeString(hexStr)
	if err != nil {
		// Try as plain JSON (backwards compat)
		raw = []byte(hexStr)
	}

	var data storageData
	if err := json.Unmarshal(raw, &data); err != nil {
		return &storageData{}, nil
	}

	s.cache = &data
	s.cacheTime = time.Now()
	return &data, nil
}

func (s *KeychainStorage) write(data *storageData) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}

	hexPayload := hex.EncodeToString(raw)

	// secret-tool store reads the secret from stdin
	cmd := exec.Command("secret-tool", "store",
		"--label", secretLabel,
		secretAttrKey, secretAttrValue)
	cmd.Stdin = strings.NewReader(hexPayload)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("secret-tool store failed: %w", err)
	}

	s.cache = data
	s.cacheTime = time.Now()
	return nil
}
