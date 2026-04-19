package tools

import (
	"testing"

	"github.com/Abraxas-365/claudio/internal/config"
)

func TestResolveToolModel_ToolModelsOverride(t *testing.T) {
	cfg := &config.Settings{
		SmallModel: "claude-haiku-4-5-20251001",
		ToolModels: map[string]string{"ReviewDesignFidelity": "claude-opus-4-6"},
	}
	got := ResolveToolModel("ReviewDesignFidelity", cfg)
	if got != "claude-opus-4-6" {
		t.Errorf("want %q (toolModels override), got %q", "claude-opus-4-6", got)
	}
}

func TestResolveToolModel_FallsBackToSmallModel(t *testing.T) {
	cfg := &config.Settings{
		SmallModel: "my-small-model",
		ToolModels: map[string]string{"OtherTool": "other-model"},
	}
	got := ResolveToolModel("ReviewDesignFidelity", cfg)
	if got != "my-small-model" {
		t.Errorf("want %q (smallModel fallback), got %q", "my-small-model", got)
	}
}

func TestResolveToolModel_FallsBackToDefault(t *testing.T) {
	got := ResolveToolModel("ReviewDesignFidelity", nil)
	const want = "claude-haiku-4-5-20251001"
	if got != want {
		t.Errorf("want %q (hardcoded default), got %q", want, got)
	}
}

func TestResolveToolModel_EmptyToolModelsEntry(t *testing.T) {
	cfg := &config.Settings{
		SmallModel: "small-model-fallback",
		ToolModels: map[string]string{"ReviewDesignFidelity": ""},
	}
	got := ResolveToolModel("ReviewDesignFidelity", cfg)
	if got != "small-model-fallback" {
		t.Errorf("want %q (empty entry falls through to smallModel), got %q", "small-model-fallback", got)
	}
}

func TestResolveToolModel_EmptyCfgSmallModel(t *testing.T) {
	cfg := &config.Settings{
		SmallModel: "",
		ToolModels: nil,
	}
	const want = "claude-haiku-4-5-20251001"
	got := ResolveToolModel("Recall", cfg)
	if got != want {
		t.Errorf("want %q (default when smallModel empty), got %q", want, got)
	}
}
