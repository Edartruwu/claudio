package main

import (
	"os"
	"testing"

	"github.com/Abraxas-365/claudio-plugin-caido/config"
)

func TestConfigLoad(t *testing.T) {
	// Clean env
	os.Unsetenv("CAIDO_URL")
	os.Unsetenv("CAIDO_PAT")
	os.Unsetenv("CAIDO_BODY_LIMIT")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load failed: %v", err)
	}

	// Should have loaded from ~/.claudio/plugins/caido.json
	if cfg.URL != "http://localhost:9999" {
		t.Errorf("expected URL http://localhost:9999, got %s", cfg.URL)
	}

	if cfg.PAT != "test-token" {
		t.Errorf("expected PAT test-token, got %s", cfg.PAT)
	}

	if cfg.BodyLimit != 5000 {
		t.Errorf("expected BodyLimit 5000, got %d", cfg.BodyLimit)
	}

	// Test env var override
	os.Setenv("CAIDO_URL", "http://override:8888")
	cfg, err = config.Load()
	if err != nil {
		t.Fatalf("config.Load with env override failed: %v", err)
	}

	if cfg.URL != "http://override:8888" {
		t.Errorf("env override failed: expected http://override:8888, got %s", cfg.URL)
	}
}
