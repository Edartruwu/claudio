package tools

import (
	"context"
	"encoding/json"
	"testing"
)

// ---------------------------------------------------------------------------
// ExportVideoTool / metadata
// ---------------------------------------------------------------------------

func TestExportVideoTool_Name(t *testing.T) {
	tool := &ExportVideoTool{}
	if tool.Name() != "ExportVideo" {
		t.Errorf("expected Name()=%q, got %q", "ExportVideo", tool.Name())
	}
}

func TestExportVideoTool_DescriptionNonEmpty(t *testing.T) {
	tool := &ExportVideoTool{}
	if tool.Description() == "" {
		t.Error("Description() should not be empty")
	}
}

func TestExportVideoTool_InputSchemaValidJSON(t *testing.T) {
	tool := &ExportVideoTool{}
	schema := tool.InputSchema()
	var out interface{}
	if err := json.Unmarshal(schema, &out); err != nil {
		t.Errorf("InputSchema() is not valid JSON: %v", err)
	}
}

func TestExportVideoTool_IsReadOnly(t *testing.T) {
	tool := &ExportVideoTool{}
	if tool.IsReadOnly() {
		t.Error("ExportVideoTool should not be read-only")
	}
}

func TestExportVideoTool_RequiresApproval(t *testing.T) {
	tool := &ExportVideoTool{}
	if tool.RequiresApproval(nil) {
		t.Error("ExportVideoTool should not require approval")
	}
}

// ---------------------------------------------------------------------------
// Input validation
// ---------------------------------------------------------------------------

func TestExportVideoTool_MissingHTMLPath_ReturnsError(t *testing.T) {
	tool := &ExportVideoTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for missing html_path")
	}
}

func TestExportVideoTool_InvalidFormat_ReturnsError(t *testing.T) {
	tool := &ExportVideoTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"html_path":"/tmp/test.html","format":"avi"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for invalid format")
	}
}

func TestExportVideoTool_InvalidJSON_ReturnsError(t *testing.T) {
	tool := &ExportVideoTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{bad json`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for invalid JSON input")
	}
}
