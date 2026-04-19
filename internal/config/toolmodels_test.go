package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeTempSettingsFile(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(p, data, 0644); err != nil {
		t.Fatalf("writeFile: %v", err)
	}
	return p
}

// TestSettings_ToolModels_MergeOverlay verifies overlay ToolModels keys
// are merged into base settings (keys in overlay win; base keys kept).
func TestSettings_ToolModels_MergeOverlay(t *testing.T) {
	base := &Settings{
		ToolModels: map[string]string{"A": "model-a"},
	}
	overlayFile := writeTempSettingsFile(t, map[string]any{
		"toolModels": map[string]string{"B": "model-b"},
	})
	mergeFromFile(base, overlayFile)

	if base.ToolModels["A"] != "model-a" {
		t.Errorf("key A: want %q, got %q", "model-a", base.ToolModels["A"])
	}
	if base.ToolModels["B"] != "model-b" {
		t.Errorf("key B: want %q, got %q", "model-b", base.ToolModels["B"])
	}
}

// TestSettings_ToolModels_PartialOverride confirms that when base has {A: "x"}
// and overlay has {B: "y"}, the merged result contains both keys.
func TestSettings_ToolModels_PartialOverride(t *testing.T) {
	base := &Settings{
		ToolModels: map[string]string{"ReviewDesignFidelity": "model-x"},
	}
	overlayFile := writeTempSettingsFile(t, map[string]any{
		"toolModels": map[string]string{"VerifyMockup": "model-y"},
	})
	mergeFromFile(base, overlayFile)

	if v := base.ToolModels["ReviewDesignFidelity"]; v != "model-x" {
		t.Errorf("ReviewDesignFidelity: want %q, got %q", "model-x", v)
	}
	if v := base.ToolModels["VerifyMockup"]; v != "model-y" {
		t.Errorf("VerifyMockup: want %q, got %q", "model-y", v)
	}
	if len(base.ToolModels) != 2 {
		t.Errorf("expected 2 entries, got %d: %v", len(base.ToolModels), base.ToolModels)
	}
}

// TestSettings_ToolModels_OverlayWins verifies overlay value replaces base value
// for the same key.
func TestSettings_ToolModels_OverlayWins(t *testing.T) {
	base := &Settings{
		ToolModels: map[string]string{"Recall": "old-model"},
	}
	overlayFile := writeTempSettingsFile(t, map[string]any{
		"toolModels": map[string]string{"Recall": "new-model"},
	})
	mergeFromFile(base, overlayFile)

	if v := base.ToolModels["Recall"]; v != "new-model" {
		t.Errorf("Recall: want %q, got %q", "new-model", v)
	}
}
