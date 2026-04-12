package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
)

type Config struct {
	URL       string
	PAT       string
	BodyLimit int
}

// Load reads config from ~/.claudio/plugins/caido.json and overrides with env vars.
// Defaults: URL="http://127.0.0.1:8080", BodyLimit=2000
// Error only if file exists but is malformed JSON.
func Load() (*Config, error) {
	cfg := &Config{
		URL:       "http://127.0.0.1:8080",
		BodyLimit: 2000,
	}

	// Try to read config file
	home, err := os.UserHomeDir()
	if err != nil {
		return cfg, nil // No home dir, use defaults
	}

	configPath := filepath.Join(home, ".claudio", "plugins", "caido.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, use defaults (no error)
			return cfg, nil
		}
		// File exists but unreadable
		return nil, err
	}

	// Parse JSON
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	// Override with env vars
	if url := os.Getenv("CAIDO_URL"); url != "" {
		cfg.URL = url
	}
	if pat := os.Getenv("CAIDO_PAT"); pat != "" {
		cfg.PAT = pat
	}
	if limit := os.Getenv("CAIDO_BODY_LIMIT"); limit != "" {
		if n, err := strconv.Atoi(limit); err == nil {
			cfg.BodyLimit = n
		}
	}

	return cfg, nil
}
