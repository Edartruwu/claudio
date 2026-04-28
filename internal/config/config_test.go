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

	// Most user-facing defaults are now set by defaults.lua, not Go code.
	// Only structural defaults remain in DefaultSettings().
	if settings.APIBaseURL != "https://api.anthropic.com" {
		t.Errorf("expected default APIBaseURL, got %q", settings.APIBaseURL)
	}
	if settings.AgentAutoDeleteAfter != 3 {
		t.Errorf("expected default AgentAutoDeleteAfter=3, got %d", settings.AgentAutoDeleteAfter)
	}
	if settings.CodeFilterLevel != "none" {
		t.Errorf("expected default CodeFilterLevel='none', got %q", settings.CodeFilterLevel)
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
// state.json migration tests
// ---------------------------------------------------------------------------

func TestLoadSettings_MigratesFromSettingsJson(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	statePath := filepath.Join(dir, "state.json")

	// Old settings.json with pluginConfigs (machine field)
	old := map[string]any{
		"model": "claude-opus-4-6",
		"pluginConfigs": map[string]any{
			"my-plugin": map[string]any{"token": "secret-123"},
		},
	}
	data, _ := json.Marshal(old)
	os.WriteFile(settingsPath, data, 0644)

	// state.json does NOT exist yet
	s := config.DefaultSettings()
	config.LoadMachineStateFrom(s, statePath, settingsPath)

	// PluginConfigs should be available
	if s.PluginConfigs == nil {
		t.Fatal("expected PluginConfigs to be migrated")
	}
	pc := s.PluginConfigs["my-plugin"]
	if pc == nil || pc["token"] != "secret-123" {
		t.Errorf("expected token=secret-123, got %v", pc)
	}

	// state.json should now exist (migration wrote it)
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		t.Error("expected state.json to be created by migration")
	}

	// Verify state.json content
	stateData, _ := os.ReadFile(statePath)
	var ms config.MachineState
	if err := json.Unmarshal(stateData, &ms); err != nil {
		t.Fatalf("state.json unmarshal: %v", err)
	}
	if ms.PluginConfigs["my-plugin"]["token"] != "secret-123" {
		t.Error("state.json should contain migrated pluginConfigs")
	}
}

func TestSaveSettings_WritesStateJson(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	statePath := filepath.Join(dir, "state.json")

	s := config.DefaultSettings()
	s.Model = "claude-opus-4-6"
	s.PluginConfigs = map[string]map[string]any{
		"test-plugin": {"key": "val"},
	}

	err := config.SaveSettingsTo(s, settingsPath, statePath)
	if err != nil {
		t.Fatalf("SaveSettingsTo: %v", err)
	}

	// settings.json should NOT have pluginConfigs
	settingsData, _ := os.ReadFile(settingsPath)
	var raw map[string]json.RawMessage
	json.Unmarshal(settingsData, &raw)
	if _, ok := raw["pluginConfigs"]; ok {
		t.Error("settings.json should not contain pluginConfigs")
	}

	// settings.json should still have model
	var cfg map[string]any
	json.Unmarshal(settingsData, &cfg)
	if cfg["model"] != "claude-opus-4-6" {
		t.Errorf("expected model in settings.json, got %v", cfg["model"])
	}

	// state.json should have pluginConfigs
	stateData, _ := os.ReadFile(statePath)
	var ms config.MachineState
	json.Unmarshal(stateData, &ms)
	if ms.PluginConfigs["test-plugin"]["key"] != "val" {
		t.Error("state.json should contain pluginConfigs")
	}
}

func TestLoadSettings_StateJsonTakesPrecedence(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	statePath := filepath.Join(dir, "state.json")

	// settings.json has old pluginConfigs
	old := map[string]any{
		"pluginConfigs": map[string]any{
			"p": map[string]any{"v": "old"},
		},
	}
	data, _ := json.Marshal(old)
	os.WriteFile(settingsPath, data, 0644)

	// state.json has newer pluginConfigs
	ms := config.MachineState{
		PluginConfigs: map[string]map[string]any{
			"p": {"v": "new"},
		},
	}
	msData, _ := json.MarshalIndent(ms, "", "  ")
	os.WriteFile(statePath, msData, 0644)

	s := config.DefaultSettings()
	config.LoadMachineStateFrom(s, statePath, settingsPath)

	// state.json value should win
	if s.PluginConfigs["p"]["v"] != "new" {
		t.Errorf("expected state.json to take precedence, got %v", s.PluginConfigs["p"]["v"])
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
