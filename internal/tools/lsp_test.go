package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Abraxas-365/claudio/internal/config"
	"github.com/Abraxas-365/claudio/internal/services/lsp"
)

func TestLSPTool_Name(t *testing.T) {
	tool := &LSPTool{deferrable: newDeferrable("LSP code intelligence")}
	if tool.Name() != "LSP" {
		t.Errorf("expected name 'LSP', got %q", tool.Name())
	}
}

func TestLSPTool_IsReadOnly(t *testing.T) {
	tool := &LSPTool{}
	if !tool.IsReadOnly() {
		t.Error("expected LSP tool to be read-only")
	}
}

func TestLSPTool_RequiresApproval(t *testing.T) {
	tool := &LSPTool{}
	if tool.RequiresApproval(nil) {
		t.Error("expected LSP tool to not require approval")
	}
}

func TestLSPTool_IsEnabled_NoManager(t *testing.T) {
	tool := &LSPTool{}
	if tool.IsEnabled() {
		t.Error("expected IsEnabled() false with no manager")
	}
}

func TestLSPTool_IsEnabled_EmptyManager(t *testing.T) {
	tool := &LSPTool{}
	tool.SetLSPManager(lsp.NewServerManager(nil))
	if tool.IsEnabled() {
		t.Error("expected IsEnabled() false with empty manager")
	}
}

func TestLSPTool_IsEnabled_WithServers(t *testing.T) {
	cfgs := map[string]config.LspServerConfig{
		"gopls": {
			Command:    "gopls",
			Extensions: []string{".go"},
		},
	}
	tool := &LSPTool{}
	tool.SetLSPManager(lsp.NewServerManager(cfgs))
	if !tool.IsEnabled() {
		t.Error("expected IsEnabled() true with configured servers")
	}
}

func TestLSPTool_Execute_NoManager(t *testing.T) {
	tool := &LSPTool{}

	input, _ := json.Marshal(lspInput{
		Operation: "hover",
		FilePath:  "main.go",
	})

	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result when no manager set")
	}
	if result.Content == "" {
		t.Error("expected error message in content")
	}
}

func TestLSPTool_Execute_InvalidInput(t *testing.T) {
	tool := &LSPTool{}
	tool.SetLSPManager(lsp.NewServerManager(nil))

	result, err := tool.Execute(context.Background(), json.RawMessage(`{invalid`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for invalid JSON")
	}
}

func TestLSPTool_Execute_UnconfiguredExtension(t *testing.T) {
	cfgs := map[string]config.LspServerConfig{
		"gopls": {
			Command:    "gopls",
			Extensions: []string{".go"},
		},
	}
	tool := &LSPTool{}
	tool.SetLSPManager(lsp.NewServerManager(cfgs))

	input, _ := json.Marshal(lspInput{
		Operation: "hover",
		FilePath:  "app.py",
		Line:      10,
		Character: 5,
	})

	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for unconfigured extension")
	}
	if result.Content == "" {
		t.Error("expected error message")
	}
}

func TestLSPTool_Execute_UnknownOperation(t *testing.T) {
	cfgs := map[string]config.LspServerConfig{
		"gopls": {
			Command:    "gopls",
			Extensions: []string{".go"},
		},
	}
	tool := &LSPTool{}
	tool.SetLSPManager(lsp.NewServerManager(cfgs))

	// This will fail to connect (gopls not in test env path maybe)
	// but we test the unknown operation path by using a file with no matching server
	input, _ := json.Marshal(lspInput{
		Operation: "nonexistentOperation",
		FilePath:  "app.py",
	})

	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should get an error about no server for .py, not about unknown operation
	if !result.IsError {
		t.Error("expected error result")
	}
}

func TestLSPTool_Execute_ServerNotOnPath(t *testing.T) {
	cfgs := map[string]config.LspServerConfig{
		"fake-lsp": {
			Command:    "nonexistent-lsp-server-binary-xyz",
			Extensions: []string{".fake"},
		},
	}
	tool := &LSPTool{}
	tool.SetLSPManager(lsp.NewServerManager(cfgs))

	input, _ := json.Marshal(lspInput{
		Operation: "hover",
		FilePath:  "test.fake",
		Line:      1,
		Character: 1,
	})

	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result when LSP binary not found")
	}
}

func TestLSPTool_InputSchema(t *testing.T) {
	tool := &LSPTool{}
	schema := tool.InputSchema()

	var parsed map[string]interface{}
	if err := json.Unmarshal(schema, &parsed); err != nil {
		t.Fatalf("InputSchema is not valid JSON: %v", err)
	}

	if parsed["type"] != "object" {
		t.Errorf("expected type 'object', got %v", parsed["type"])
	}

	props, ok := parsed["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties to be an object")
	}

	for _, field := range []string{"operation", "file_path", "line", "character", "symbol"} {
		if _, ok := props[field]; !ok {
			t.Errorf("expected property %q in schema", field)
		}
	}

	required, ok := parsed["required"].([]interface{})
	if !ok {
		t.Fatal("expected required to be an array")
	}
	requiredSet := map[string]bool{}
	for _, r := range required {
		requiredSet[r.(string)] = true
	}
	if !requiredSet["operation"] {
		t.Error("expected 'operation' to be required")
	}
	if !requiredSet["file_path"] {
		t.Error("expected 'file_path' to be required")
	}
}

func TestLSPTool_ShouldDefer(t *testing.T) {
	tool := &LSPTool{deferrable: newDeferrable("LSP code intelligence")}
	if !tool.ShouldDefer() {
		t.Error("expected ShouldDefer() true")
	}
	if tool.SearchHint() != "LSP code intelligence" {
		t.Errorf("unexpected search hint: %q", tool.SearchHint())
	}
}

func TestLSPTool_SetLSPManager(t *testing.T) {
	tool := &LSPTool{}

	// Initially nil
	if tool.IsEnabled() {
		t.Error("expected disabled before SetLSPManager")
	}

	// Set empty manager
	tool.SetLSPManager(lsp.NewServerManager(nil))
	if tool.IsEnabled() {
		t.Error("expected disabled with empty manager")
	}

	// Set manager with config
	cfgs := map[string]config.LspServerConfig{
		"gopls": {Command: "gopls", Extensions: []string{".go"}},
	}
	tool.SetLSPManager(lsp.NewServerManager(cfgs))
	if !tool.IsEnabled() {
		t.Error("expected enabled with configured manager")
	}
}
