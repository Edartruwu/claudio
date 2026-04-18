package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// NewRenderMockupTool / metadata
// ---------------------------------------------------------------------------

func TestNewRenderMockupTool_NotNil(t *testing.T) {
	tool := NewRenderMockupTool("/tmp/designs")
	if tool == nil {
		t.Error("NewRenderMockupTool returned nil")
	}
}

func TestRenderMockupTool_Name(t *testing.T) {
	tool := NewRenderMockupTool("/tmp/designs")
	if tool.Name() != "RenderMockup" {
		t.Errorf("expected Name()=%q, got %q", "RenderMockup", tool.Name())
	}
}

func TestRenderMockupTool_DescriptionNonEmpty(t *testing.T) {
	tool := NewRenderMockupTool("/tmp/designs")
	if tool.Description() == "" {
		t.Error("Description() should not be empty")
	}
}

func TestRenderMockupTool_InputSchemaValidJSON(t *testing.T) {
	tool := NewRenderMockupTool("/tmp/designs")
	schema := tool.InputSchema()
	var out interface{}
	if err := json.Unmarshal(schema, &out); err != nil {
		t.Errorf("InputSchema() is not valid JSON: %v", err)
	}
}

func TestRenderMockupTool_IsReadOnly(t *testing.T) {
	tool := NewRenderMockupTool("/tmp/designs")
	if tool.IsReadOnly() {
		t.Error("RenderMockupTool should not be read-only")
	}
}

func TestRenderMockupTool_RequiresApproval(t *testing.T) {
	tool := NewRenderMockupTool("/tmp/designs")
	if tool.RequiresApproval(nil) {
		t.Error("RenderMockupTool should not require approval")
	}
}

// ---------------------------------------------------------------------------
// checkPrerequisites — node not in PATH returns error containing "Node"
// ---------------------------------------------------------------------------

func TestRenderMockupTool_CheckPrerequisites_NoNode(t *testing.T) {
	// Clear PATH so exec.Command("node") cannot find node
	t.Setenv("PATH", "/nonexistent-path-for-testing-xyz")

	tool := NewRenderMockupTool(t.TempDir())
	err := tool.checkPrerequisites()
	if err == nil {
		t.Skip("node binary found even with cleared PATH — skipping prerequisite test")
	}
	// Error should mention Node.js
	if !strings.Contains(err.Error(), "Node") && !strings.Contains(strings.ToLower(err.Error()), "node") {
		t.Errorf("expected error to mention 'node', got: %s", err.Error())
	}
}

// ---------------------------------------------------------------------------
// Execute — validation paths (checked before node is invoked)
// ---------------------------------------------------------------------------

func TestRenderMockupTool_MissingHTMLPath_ReturnsError(t *testing.T) {
	// We only test the case where node IS available; if not, Execute returns
	// early with a prereq error — that's also acceptable (IsError=true).
	tool := NewRenderMockupTool(t.TempDir())
	input, _ := json.Marshal(map[string]string{})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when html_path is missing")
	}
}

func TestRenderMockupTool_InvalidJSON_ReturnsError(t *testing.T) {
	tool := NewRenderMockupTool(t.TempDir())
	result, err := tool.Execute(context.Background(), json.RawMessage(`{bad`))
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true on invalid JSON input")
	}
}

// ---------------------------------------------------------------------------
// Execute — when node is absent, error mentions "node"
// ---------------------------------------------------------------------------

func TestRenderMockupTool_NoNode_Execute_MentionsNode(t *testing.T) {
	t.Setenv("PATH", "/nonexistent-path-for-testing-xyz")

	dir := t.TempDir()
	htmlPath := filepath.Join(dir, "index.html")
	if err := os.WriteFile(htmlPath, []byte("<html><body>test</body></html>"), 0644); err != nil {
		t.Fatal(err)
	}

	tool := NewRenderMockupTool(dir)
	input, _ := json.Marshal(map[string]string{"html_path": htmlPath})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Skip("node found even with cleared PATH — skipping")
	}
	if !strings.Contains(result.Content, "Node") && !strings.Contains(strings.ToLower(result.Content), "node") {
		t.Errorf("expected error to mention 'node', got: %s", result.Content)
	}
}
