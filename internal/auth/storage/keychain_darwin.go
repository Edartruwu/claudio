//go:build darwin

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
	keychainService = "Claudio"
	keychainAccount = "default"
	cacheTTL        = 30 * time.Second
)

// KeychainStorage stores credentials in macOS Keychain.
type KeychainStorage struct {
	mu        sync.Mutex
	cache     *storageData
	cacheTime time.Time
}

// NewKeychainStorage creates a new macOS Keychain storage.
func NewKeychainStorage() *KeychainStorage {
	return &KeychainStorage{}
}

func (s *KeychainStorage) Name() string {
	return "macOS Keychain"
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
	cmd := exec.Command("security", "delete-generic-password",
		"-s", keychainService,
		"-a", keychainAccount)
	cmd.Run() // Ignore error (may not exist)
	return nil
}

func (s *KeychainStorage) read() (*storageData, error) {
	// Check cache
	if s.cache != nil && time.Since(s.cacheTime) < cacheTTL {
		return s.cache, nil
	}

	cmd := exec.Command("security", "find-generic-password",
		"-s", keychainService,
		"-a", keychainAccount,
		"-w")
	output, err := cmd.Output()
	if err != nil {
		return &storageData{}, nil // Not found
	}

	// Decode hex payload
	hexStr := strings.TrimSpace(string(output))
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

	// Hex-encode to avoid shell escaping issues
	hexPayload := hex.EncodeToString(raw)

	// Delete existing entry first
	deleteCmd := exec.Command("security", "delete-generic-password",
		"-s", keychainService,
		"-a", keychainAccount)
	deleteCmd.Run() // Ignore error

	// Add new entry
	addCmd := exec.Command("security", "add-generic-password",
		"-s", keychainService,
		"-a", keychainAccount,
		"-w", hexPayload,
		"-U") // Update if exists
	if err := addCmd.Run(); err != nil {
		return fmt.Errorf("keychain write failed: %w", err)
	}

	// Update cache
	s.cache = data
	s.cacheTime = time.Now()
	return nil
}
