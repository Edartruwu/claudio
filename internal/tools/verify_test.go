package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// NewVerifyMockupTool / metadata
// ---------------------------------------------------------------------------

func TestNewVerifyMockupTool_NotNil(t *testing.T) {
	tool := NewVerifyMockupTool("/tmp/designs", nil, "")
	if tool == nil {
		t.Error("NewVerifyMockupTool returned nil")
	}
}

func TestVerifyMockupTool_Name(t *testing.T) {
	tool := NewVerifyMockupTool("/tmp/designs", nil, "")
	if tool.Name() != "VerifyMockup" {
		t.Errorf("expected Name()=%q, got %q", "VerifyMockup", tool.Name())
	}
}

func TestVerifyMockupTool_DescriptionNonEmpty(t *testing.T) {
	tool := NewVerifyMockupTool("/tmp/designs", nil, "")
	if tool.Description() == "" {
		t.Error("Description() should not be empty")
	}
}

func TestVerifyMockupTool_InputSchemaValidJSON(t *testing.T) {
	tool := NewVerifyMockupTool("/tmp/designs", nil, "")
	schema := tool.InputSchema()
	var out interface{}
	if err := json.Unmarshal(schema, &out); err != nil {
		t.Errorf("InputSchema() is not valid JSON: %v", err)
	}
}

func TestVerifyMockupTool_IsReadOnly(t *testing.T) {
	tool := NewVerifyMockupTool("/tmp/designs", nil, "")
	if !tool.IsReadOnly() {
		t.Error("VerifyMockupTool should be read-only")
	}
}

func TestVerifyMockupTool_RequiresApproval(t *testing.T) {
	tool := NewVerifyMockupTool("/tmp/designs", nil, "")
	if tool.RequiresApproval(nil) {
		t.Error("VerifyMockupTool should not require approval")
	}
}

// ---------------------------------------------------------------------------
// Execute — validation paths (no client call needed)
// ---------------------------------------------------------------------------

func TestVerifyMockupTool_MissingScreenshotPath_ReturnsError(t *testing.T) {
	tool := NewVerifyMockupTool("/tmp/designs", nil, "")
	input, _ := json.Marshal(map[string]string{
		"design_brief": "A login page",
	})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when screenshot_path is missing")
	}
	if !strings.Contains(result.Content, "screenshot_path") {
		t.Errorf("expected error to mention 'screenshot_path', got: %s", result.Content)
	}
}

func TestVerifyMockupTool_MissingDesignBrief_ReturnsError(t *testing.T) {
	tool := NewVerifyMockupTool("/tmp/designs", nil, "")
	input, _ := json.Marshal(map[string]string{
		"screenshot_path": "/some/path.png",
	})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when design_brief is missing")
	}
	if !strings.Contains(result.Content, "design_brief") {
		t.Errorf("expected error to mention 'design_brief', got: %s", result.Content)
	}
}

func TestVerifyMockupTool_InvalidJSON_ReturnsError(t *testing.T) {
	tool := NewVerifyMockupTool("/tmp/designs", nil, "")
	result, err := tool.Execute(context.Background(), json.RawMessage(`{bad`))
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true on invalid JSON input")
	}
}

// ---------------------------------------------------------------------------
// criticModel — priority: env > constructor > default
// ---------------------------------------------------------------------------

func TestVerifyMockupTool_CriticModel_Default(t *testing.T) {
	tool := NewVerifyMockupTool("/tmp/designs", nil, "")
	got := tool.criticModel()
	if got == "" {
		t.Error("criticModel() should return a non-empty default model")
	}
}

func TestVerifyMockupTool_CriticModel_ConstructorOverride(t *testing.T) {
	tool := NewVerifyMockupTool("/tmp/designs", nil, "my-critic-model")
	got := tool.criticModel()
	if got != "my-critic-model" {
		t.Errorf("expected criticModel()=%q, got %q", "my-critic-model", got)
	}
}

func TestVerifyMockupTool_CriticModel_EnvOverride(t *testing.T) {
	t.Setenv("CLAUDIO_DESIGN_CRITIC_MODEL", "env-critic-model")
	tool := NewVerifyMockupTool("/tmp/designs", nil, "constructor-model")
	got := tool.criticModel()
	if got != "env-critic-model" {
		t.Errorf("expected env override %q, got %q", "env-critic-model", got)
	}
}

// ---------------------------------------------------------------------------
// JSON response parsing — replicate stripping logic to verify expected behavior
// ---------------------------------------------------------------------------

// verifyStripFences replicates the markdown fence stripping logic from Execute.
func verifyStripFences(s string) string {
	cleaned := strings.TrimSpace(s)
	if strings.HasPrefix(cleaned, "```") {
		if idx := strings.Index(cleaned, "\n"); idx != -1 {
			cleaned = cleaned[idx+1:]
		}
		if idx := strings.LastIndex(cleaned, "```"); idx != -1 {
			cleaned = cleaned[:idx]
		}
		cleaned = strings.TrimSpace(cleaned)
	}
	return cleaned
}

func TestVerifyJSONParsing_CleanJSON(t *testing.T) {
	raw := `{"overall_score": 82, "pass": true, "dimensions": [], "blocking_issues": [], "suggestions": [], "raw_critique": "looks good"}`
	cleaned := verifyStripFences(raw)

	var out VerifyMockupOutput
	if err := json.Unmarshal([]byte(cleaned), &out); err != nil {
		t.Errorf("failed to parse clean JSON: %v", err)
	}
	if out.OverallScore != 82 {
		t.Errorf("expected OverallScore=82, got %d", out.OverallScore)
	}
	if !out.Pass {
		t.Error("expected Pass=true")
	}
}

func TestVerifyJSONParsing_MarkdownWrapped(t *testing.T) {
	raw := "```json\n{\"overall_score\": 60, \"pass\": false, \"dimensions\": [], \"blocking_issues\": [\"missing contrast\"], \"suggestions\": [], \"raw_critique\": \"needs work\"}\n```"
	cleaned := verifyStripFences(raw)

	var out VerifyMockupOutput
	if err := json.Unmarshal([]byte(cleaned), &out); err != nil {
		t.Errorf("failed to parse markdown-wrapped JSON after stripping: %v\ncleaned=%q", err, cleaned)
	}
	if out.OverallScore != 60 {
		t.Errorf("expected OverallScore=60, got %d", out.OverallScore)
	}
	if out.Pass {
		t.Error("expected Pass=false")
	}
	if len(out.BlockingIssues) != 1 || out.BlockingIssues[0] != "missing contrast" {
		t.Errorf("unexpected BlockingIssues: %v", out.BlockingIssues)
	}
}

func TestVerifyJSONParsing_MarkdownNoLang(t *testing.T) {
	raw := "```\n{\"overall_score\": 75, \"pass\": true, \"dimensions\": [], \"blocking_issues\": [], \"suggestions\": [\"add padding\"], \"raw_critique\": \"ok\"}\n```"
	cleaned := verifyStripFences(raw)

	var out VerifyMockupOutput
	if err := json.Unmarshal([]byte(cleaned), &out); err != nil {
		t.Errorf("failed to parse ``` wrapped JSON: %v\ncleaned=%q", err, cleaned)
	}
	if out.OverallScore != 75 {
		t.Errorf("expected OverallScore=75, got %d", out.OverallScore)
	}
}
