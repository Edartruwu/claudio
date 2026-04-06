package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/Abraxas-365/claudio/internal/prompts"
	"github.com/Abraxas-365/claudio/internal/services/lsp"
)

// LSPTool provides Language Server Protocol operations.
type LSPTool struct {
	deferrable
	manager *lsp.ServerManager
}

type lspInput struct {
	Operation string `json:"operation"` // goToDefinition, findReferences, hover, documentSymbol
	FilePath  string `json:"file_path"`
	Line      int    `json:"line,omitempty"`
	Character int    `json:"character,omitempty"`
	Symbol    string `json:"symbol,omitempty"` // for workspace symbol search
}

func (t *LSPTool) Name() string { return "LSP" }

func (t *LSPTool) Description() string {
	return prompts.LSPDescription()
}

func (t *LSPTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"operation": {
				"type": "string",
				"enum": ["goToDefinition", "findReferences", "hover", "documentSymbol"],
				"description": "The LSP operation to perform"
			},
			"file_path": {
				"type": "string",
				"description": "Path to the file"
			},
			"line": {
				"type": "number",
				"description": "Line number (0-based)"
			},
			"character": {
				"type": "number",
				"description": "Column number (0-based)"
			},
			"symbol": {
				"type": "string",
				"description": "Symbol name for workspace search"
			}
		},
		"required": ["operation", "file_path"]
	}`)
}

func (t *LSPTool) IsReadOnly() bool                        { return true }
func (t *LSPTool) RequiresApproval(_ json.RawMessage) bool { return false }

// IsEnabled returns true only when at least one LSP server is configured.
func (t *LSPTool) IsEnabled() bool {
	return t.manager != nil && t.manager.HasServers()
}

// AutoActivate returns true when LSP servers are configured, causing the tool
// to skip deferral so the AI knows code intelligence is available immediately.
func (t *LSPTool) AutoActivate() bool {
	return t.IsEnabled()
}

// SetLSPManager injects the LSP server manager.
func (t *LSPTool) SetLSPManager(m *lsp.ServerManager) {
	t.manager = m
}

func (t *LSPTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in lspInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	if t.manager == nil {
		return &Result{Content: "No LSP servers configured. Add lspServers to your settings.json or install an LSP plugin.", IsError: true}, nil
	}

	// Resolve to absolute path
	absPath, err := filepath.Abs(in.FilePath)
	if err != nil {
		absPath = in.FilePath
	}

	// Check if we have a server for this file type
	if t.manager.ServerForFile(absPath) == "" {
		ext := filepath.Ext(absPath)
		return &Result{Content: fmt.Sprintf("No LSP server configured for %s files. Add one to lspServers in settings.json.", ext), IsError: true}, nil
	}

	srv, err := t.manager.GetServer(ctx, absPath)
	if err != nil {
		return &Result{Content: fmt.Sprintf("LSP server error: %v", err), IsError: true}, nil
	}

	var result json.RawMessage

	switch in.Operation {
	case "goToDefinition":
		result, err = srv.GoToDefinition(absPath, in.Line, in.Character)
	case "findReferences":
		result, err = srv.FindReferences(absPath, in.Line, in.Character)
	case "hover":
		result, err = srv.Hover(absPath, in.Line, in.Character)
	case "documentSymbol":
		result, err = srv.DocumentSymbols(absPath)
	default:
		return &Result{Content: fmt.Sprintf("Unknown operation: %s", in.Operation), IsError: true}, nil
	}

	if err != nil {
		return &Result{Content: fmt.Sprintf("LSP error: %v", err), IsError: true}, nil
	}

	if result == nil {
		return &Result{Content: "No results"}, nil
	}

	// Format the output
	out, err := json.MarshalIndent(json.RawMessage(result), "", "  ")
	if err != nil {
		return &Result{Content: string(result)}, nil
	}

	output := string(out)
	const maxLSPBytes = 20_000
	if len(output) > maxLSPBytes {
		output = output[:maxLSPBytes] + fmt.Sprintf("\n[LSP output truncated at %d bytes]", maxLSPBytes)
	}

	return &Result{Content: output}, nil
}
