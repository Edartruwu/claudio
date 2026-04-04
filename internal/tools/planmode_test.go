package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestEnterPlanModeResult_ContainsPlanFilePrefix verifies that the result
// returned by EnterPlanModeTool.Execute contains the exact prefix "Plan file: "
// (capital P, no leading "the ") so that the TUI can parse the path correctly.
func TestEnterPlanModeResult_ContainsPlanFilePrefix(t *testing.T) {
	tool := &EnterPlanModeTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Execute returned nil result")
	}

	const wantPrefix = "Plan file: "
	if !strings.Contains(result.Content, wantPrefix) {
		t.Errorf("result content does not contain %q\ngot: %s", wantPrefix, result.Content)
	}
}

// TestEnterPlanModeResult_DoesNotContainOldPrefix ensures the old lowercase
// variant "the plan file: " is not present, which would cause the TUI to fail
// to extract the plan file path.
func TestEnterPlanModeResult_DoesNotContainOldPrefix(t *testing.T) {
	tool := &EnterPlanModeTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}

	const badPrefix = "the plan file: "
	if strings.Contains(strings.ToLower(result.Content), badPrefix) {
		t.Errorf("result content contains deprecated lowercase prefix %q; use %q instead",
			badPrefix, "Plan file: ")
	}
}

// TestEnterPlanModeResult_PathExtraction verifies that the path after
// "Plan file: " can be correctly extracted (i.e. it stops at the newline and
// does not bleed into subsequent lines of the instruction text).
func TestEnterPlanModeResult_PathExtraction(t *testing.T) {
	tool := &EnterPlanModeTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}

	const prefix = "Plan file: "
	idx := strings.Index(result.Content, prefix)
	if idx < 0 {
		t.Fatalf("prefix %q not found in result content", prefix)
	}

	rest := result.Content[idx+len(prefix):]
	// Stop at the newline, just like root.go does.
	if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
		rest = rest[:nl]
	}
	path := strings.TrimSpace(rest)

	if path == "" {
		t.Error("extracted plan file path is empty")
	}
	// The path should look like a real filesystem path, not a sentence fragment.
	if strings.Contains(path, " ") {
		t.Errorf("extracted path contains spaces, likely bleed-through from instruction text: %q", path)
	}
	if !strings.HasSuffix(path, ".md") {
		t.Errorf("expected plan file to end in .md, got %q", path)
	}
}

// TestEnterPlanModeTool_Name checks the tool name is stable (the TUI and
// engine switch on this value).
func TestEnterPlanModeTool_Name(t *testing.T) {
	tool := &EnterPlanModeTool{}
	if got := tool.Name(); got != "EnterPlanMode" {
		t.Errorf("Name() = %q, want %q", got, "EnterPlanMode")
	}
}

// TestEnterPlanModeTool_IsReadOnly ensures EnterPlanMode is marked read-only.
func TestEnterPlanModeTool_IsReadOnly(t *testing.T) {
	tool := &EnterPlanModeTool{}
	if !tool.IsReadOnly() {
		t.Error("EnterPlanModeTool.IsReadOnly() = false, want true")
	}
}
