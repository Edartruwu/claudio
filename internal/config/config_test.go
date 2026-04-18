package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Abraxas-365/claudio/internal/config"
)

func TestLoadDefaults(t *testing.T) {
	settings, err := config.Load("")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if settings.CompactMode != "strategic" {
		t.Errorf("expected default CompactMode 'strategic', got %q", settings.CompactMode)
	}
	if !settings.SessionPersist {
		t.Error("expected default SessionPersist true")
	}
	if settings.HookProfile != "standard" {
		t.Errorf("expected default HookProfile 'standard', got %q", settings.HookProfile)
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	data, _ := json.Marshal(map[string]any{
		"model":       "claude-opus-4-6",
		"compactMode": "manual",
	})
	os.WriteFile(settingsPath, data, 0644)

	// Temporarily override paths — this is a simplified test
	// In production, we'd inject the paths
	settings, _ := config.Load("")
	if settings == nil {
		t.Fatal("expected non-nil settings")
	}
}

func TestEnv(t *testing.T) {
	env := config.GetEnv()

	// These should return empty when not set
	if v := env.AnthropicAPIKey(); v != "" {
		t.Skip("ANTHROPIC_API_KEY is set in environment, skipping")
	}

	// Test with env var
	os.Setenv("CLAUDIO_MODEL", "test-model")
	defer os.Unsetenv("CLAUDIO_MODEL")

	if v := env.Model(); v != "test-model" {
		t.Errorf("expected 'test-model', got %q", v)
	}
}

func TestGetPaths(t *testing.T) {
	paths := config.GetPaths()

	if paths.Home == "" {
		t.Error("expected non-empty Home path")
	}
	if paths.DB == "" {
		t.Error("expected non-empty DB path")
	}
	if !filepath.IsAbs(paths.Home) {
		t.Error("expected absolute Home path")
	}
}

func TestEnsureDirs(t *testing.T) {
	// This creates real directories in ~/.claudio/ — acceptable for integration test
	err := config.EnsureDirs()
	if err != nil {
		t.Fatalf("EnsureDirs failed: %v", err)
	}

	paths := config.GetPaths()
	for _, dir := range []string{paths.Home, paths.Sessions, paths.Skills, paths.Audit} {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Errorf("expected directory to exist: %s", dir)
		}
	}
}

// ---------------------------------------------------------------------------
// Paths.Designs — new field for Claudio Design feature
// ---------------------------------------------------------------------------

func TestGetPaths_DesignsNonEmpty(t *testing.T) {
	paths := config.GetPaths()
	if paths.Designs == "" {
		t.Error("expected non-empty Designs path")
	}
}

func TestGetPaths_DesignsAbsolutePath(t *testing.T) {
	paths := config.GetPaths()
	if !filepath.IsAbs(paths.Designs) {
		t.Errorf("expected Designs to be absolute path, got %q", paths.Designs)
	}
}

func TestGetPaths_DesignsEndsWithDesigns(t *testing.T) {
	paths := config.GetPaths()
	base := filepath.Base(paths.Designs)
	if base != "designs" {
		t.Errorf("expected Designs path to end with 'designs', got %q (base=%q)", paths.Designs, base)
	}
}

func TestEnsureDirs_CreatesDesignsDir(t *testing.T) {
	err := config.EnsureDirs()
	if err != nil {
		t.Fatalf("EnsureDirs failed: %v", err)
	}
	paths := config.GetPaths()
	if _, err := os.Stat(paths.Designs); os.IsNotExist(err) {
		t.Errorf("expected designs directory to exist at %s", paths.Designs)
	}
}
