package storage

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/Abraxas-365/claudio/internal/auth/oauth"
)

// storageData is the on-disk JSON format.
type storageData struct {
	OAuthTokens *oauth.Tokens `json:"oauthTokens,omitempty"`
	APIKey      string        `json:"apiKey,omitempty"`
}

// PlaintextStorage stores credentials in a plaintext JSON file.
// Used as a fallback when no keychain is available.
type PlaintextStorage struct {
	path string
	mu   sync.Mutex
}

// NewPlaintextStorage creates a new plaintext storage at the given path.
func NewPlaintextStorage(path string) *PlaintextStorage {
	return &PlaintextStorage{path: path}
}

func (s *PlaintextStorage) Name() string {
	return "plaintext:" + s.path
}

func (s *PlaintextStorage) ReadTokens() (*oauth.Tokens, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.readFile()
	if err != nil {
		return nil, err
	}
	return data.OAuthTokens, nil
}

func (s *PlaintextStorage) SaveTokens(tokens *oauth.Tokens) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, _ := s.readFile()
	data.OAuthTokens = tokens
	return s.writeFile(data)
}

func (s *PlaintextStorage) ReadAPIKey() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.readFile()
	if err != nil {
		return "", err
	}
	return data.APIKey, nil
}

func (s *PlaintextStorage) SaveAPIKey(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, _ := s.readFile()
	data.APIKey = key
	return s.writeFile(data)
}

func (s *PlaintextStorage) Delete() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return os.Remove(s.path)
}

func (s *PlaintextStorage) readFile() (*storageData, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return &storageData{}, nil
		}
		return nil, err
	}
	var data storageData
	if err := json.Unmarshal(raw, &data); err != nil {
		return &storageData{}, nil // corrupted file, start fresh
	}
	return &data, nil
}

func (s *PlaintextStorage) writeFile(data *storageData) error {
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, raw, 0600)
}
