package tools

import (
	"context"
	"encoding/json"
	"testing"
)

// ---------------------------------------------------------------------------
// ExportDeckPPTXTool / metadata
// ---------------------------------------------------------------------------

func TestExportDeckPPTXTool_Name(t *testing.T) {
	tool := &ExportDeckPPTXTool{}
	if tool.Name() != "ExportDeckPPTX" {
		t.Errorf("expected Name()=%q, got %q", "ExportDeckPPTX", tool.Name())
	}
}

func TestExportDeckPPTXTool_DescriptionNonEmpty(t *testing.T) {
	tool := &ExportDeckPPTXTool{}
	if tool.Description() == "" {
		t.Error("Description() should not be empty")
	}
}

func TestExportDeckPPTXTool_InputSchemaValidJSON(t *testing.T) {
	tool := &ExportDeckPPTXTool{}
	schema := tool.InputSchema()
	var out interface{}
	if err := json.Unmarshal(schema, &out); err != nil {
		t.Errorf("InputSchema() is not valid JSON: %v", err)
	}
}

func TestExportDeckPPTXTool_IsReadOnly(t *testing.T) {
	tool := &ExportDeckPPTXTool{}
	if tool.IsReadOnly() {
		t.Error("ExportDeckPPTXTool should not be read-only")
	}
}

func TestExportDeckPPTXTool_RequiresApproval(t *testing.T) {
	tool := &ExportDeckPPTXTool{}
	if tool.RequiresApproval(nil) {
		t.Error("ExportDeckPPTXTool should not require approval")
	}
}

// ---------------------------------------------------------------------------
// Input validation
// ---------------------------------------------------------------------------

func TestExportDeckPPTXTool_MissingHTMLPath_ReturnsError(t *testing.T) {
	tool := &ExportDeckPPTXTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for missing html_path")
	}
}

func TestExportDeckPPTXTool_InvalidJSON_ReturnsError(t *testing.T) {
	tool := &ExportDeckPPTXTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{bad json`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for invalid JSON input")
	}
}

// ---------------------------------------------------------------------------
// parseSlideCount
// ---------------------------------------------------------------------------

func TestParseSlideCount(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"Converting 5 slides via html2pptx...", 5},
		{"Converting 12 slides via html2pptx...", 12},
		{"no match here", 0},
		{"", 0},
	}
	for _, tt := range tests {
		got := parseSlideCount(tt.input)
		if got != tt.want {
			t.Errorf("parseSlideCount(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
