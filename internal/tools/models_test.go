package tools

import (
	"encoding/json"
	"testing"
)

func TestBuildModelEnum_Empty(t *testing.T) {
	result := buildModelEnum(nil)
	if result != "[]" {
		t.Errorf("expected '[]', got %q", result)
	}
}

func TestBuildModelEnum_EmptySlice(t *testing.T) {
	result := buildModelEnum([]string{})
	if result != "[]" {
		t.Errorf("expected '[]', got %q", result)
	}
}

func TestBuildModelEnum_SingleModel(t *testing.T) {
	result := buildModelEnum([]string{"gpt-4o"})

	var models []string
	if err := json.Unmarshal([]byte(result), &models); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if len(models) != 1 || models[0] != "gpt-4o" {
		t.Errorf("expected [gpt-4o], got %v", models)
	}
}

func TestBuildModelEnum_MultipleModels(t *testing.T) {
	input := []string{"gpt-4o", "claude-3-5-sonnet", "gemini-pro"}
	result := buildModelEnum(input)

	var models []string
	if err := json.Unmarshal([]byte(result), &models); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if len(models) != 3 {
		t.Errorf("expected 3 models, got %d: %v", len(models), models)
	}
	for i, want := range input {
		if models[i] != want {
			t.Errorf("index %d: expected %q, got %q", i, want, models[i])
		}
	}
}

func TestBuildModelEnum_DeduplicatesModels(t *testing.T) {
	input := []string{"gpt-4o", "claude-3-5-sonnet", "gpt-4o", "gpt-4o"}
	result := buildModelEnum(input)

	var models []string
	if err := json.Unmarshal([]byte(result), &models); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if len(models) != 2 {
		t.Errorf("expected 2 deduplicated models, got %d: %v", len(models), models)
	}
	if models[0] != "gpt-4o" || models[1] != "claude-3-5-sonnet" {
		t.Errorf("unexpected order or values: %v", models)
	}
}

func TestBuildModelEnum_FiltersEmptyStrings(t *testing.T) {
	input := []string{"gpt-4o", "", "claude-3-5-sonnet", ""}
	result := buildModelEnum(input)

	var models []string
	if err := json.Unmarshal([]byte(result), &models); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if len(models) != 2 {
		t.Errorf("expected 2 models (empty strings filtered), got %d: %v", len(models), models)
	}
	for _, m := range models {
		if m == "" {
			t.Error("empty string should have been filtered out")
		}
	}
}

func TestBuildModelEnum_PreservesOrder(t *testing.T) {
	input := []string{"zzz-model", "aaa-model", "mmm-model"}
	result := buildModelEnum(input)

	var models []string
	if err := json.Unmarshal([]byte(result), &models); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	for i, want := range input {
		if models[i] != want {
			t.Errorf("index %d: expected %q, got %q (order not preserved)", i, want, models[i])
		}
	}
}

func TestBuildModelEnum_ReturnsValidJSON(t *testing.T) {
	tests := []struct {
		name   string
		models []string
	}{
		{"nil", nil},
		{"empty", []string{}},
		{"single", []string{"model-a"}},
		{"multiple", []string{"model-a", "model-b"}},
		{"duplicates", []string{"model-a", "model-a"}},
		{"with-empty", []string{"", "model-a", ""}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := buildModelEnum(tc.models)
			var out []string
			if err := json.Unmarshal([]byte(result), &out); err != nil {
				t.Errorf("buildModelEnum(%v) returned invalid JSON %q: %v", tc.models, result, err)
			}
		})
	}
}

func TestBuildModelEnum_AllDuplicates(t *testing.T) {
	input := []string{"same", "same", "same"}
	result := buildModelEnum(input)

	var models []string
	if err := json.Unmarshal([]byte(result), &models); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if len(models) != 1 || models[0] != "same" {
		t.Errorf("expected [same] after dedup, got %v", models)
	}
}

func TestBuildModelEnum_AllEmpty(t *testing.T) {
	input := []string{"", "", ""}
	result := buildModelEnum(input)

	if result != "[]" {
		t.Errorf("expected '[]' when all models are empty strings, got %q", result)
	}
}
