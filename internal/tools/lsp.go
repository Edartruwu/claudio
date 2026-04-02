package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/Abraxas-365/claudio/internal/prompts"
)

// LSPTool provides Language Server Protocol operations.
type LSPTool struct{}

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

func (t *LSPTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in lspInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	switch in.Operation {
	case "goToDefinition":
		return lspGoToDefinition(ctx, in)
	case "findReferences":
		return lspFindReferences(ctx, in)
	case "hover":
		return lspHover(ctx, in)
	case "documentSymbol":
		return lspDocumentSymbols(ctx, in)
	default:
		return &Result{Content: fmt.Sprintf("Unknown operation: %s", in.Operation), IsError: true}, nil
	}
}

// For Go files, we can use `gopls` directly via command line.
// For other languages, we'd need their respective language servers.
// This is a practical approach that works without a persistent LSP connection.

func lspGoToDefinition(ctx context.Context, in lspInput) (*Result, error) {
	if strings.HasSuffix(in.FilePath, ".go") {
		return goplsDefinition(ctx, in.FilePath, in.Line, in.Character)
	}
	// Fallback: use grep to find definition
	return grepDefinition(ctx, in)
}

func lspFindReferences(ctx context.Context, in lspInput) (*Result, error) {
	if in.Symbol == "" {
		return &Result{Content: "Symbol required for findReferences", IsError: true}, nil
	}
	// Use ripgrep as a fast fallback
	grep := &GrepTool{}
	grepInput, _ := json.Marshal(map[string]any{
		"pattern":     in.Symbol,
		"output_mode": "content",
		"context":     1,
	})
	return grep.Execute(ctx, grepInput)
}

func lspHover(ctx context.Context, in lspInput) (*Result, error) {
	if strings.HasSuffix(in.FilePath, ".go") {
		return goplsHover(ctx, in.FilePath, in.Line, in.Character)
	}
	return &Result{Content: "Hover only supported for Go files currently"}, nil
}

func lspDocumentSymbols(ctx context.Context, in lspInput) (*Result, error) {
	if strings.HasSuffix(in.FilePath, ".go") {
		return goplsSymbols(ctx, in.FilePath)
	}
	// Fallback: use ctags or grep for function/class definitions
	return grepSymbols(ctx, in.FilePath)
}

func goplsDefinition(ctx context.Context, file string, line, char int) (*Result, error) {
	pos := fmt.Sprintf("%s:%d:%d", file, line+1, char+1) // gopls uses 1-based
	output, err := runGopls(ctx, "definition", pos)
	if err != nil {
		return &Result{Content: fmt.Sprintf("gopls error: %v", err), IsError: true}, nil
	}
	return &Result{Content: output}, nil
}

func goplsHover(ctx context.Context, file string, line, char int) (*Result, error) {
	pos := fmt.Sprintf("%s:%d:%d", file, line+1, char+1)
	output, err := runGopls(ctx, "hover", pos)
	if err != nil {
		return &Result{Content: fmt.Sprintf("gopls error: %v", err), IsError: true}, nil
	}
	return &Result{Content: output}, nil
}

func goplsSymbols(ctx context.Context, file string) (*Result, error) {
	output, err := runGopls(ctx, "symbols", file)
	if err != nil {
		return &Result{Content: fmt.Sprintf("gopls error: %v", err), IsError: true}, nil
	}
	return &Result{Content: output}, nil
}

func runGopls(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "gopls", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if strings.Contains(err.Error(), "executable file not found") {
			return "", fmt.Errorf("gopls not found — install with: go install golang.org/x/tools/gopls@latest")
		}
		return "", fmt.Errorf("%v: %s", err, stderr.String())
	}
	return stdout.String(), nil
}

func grepDefinition(ctx context.Context, in lspInput) (*Result, error) {
	if in.Symbol == "" {
		return &Result{Content: "Symbol required for definition search", IsError: true}, nil
	}
	// Search for common definition patterns
	patterns := []string{
		fmt.Sprintf("func %s", in.Symbol),
		fmt.Sprintf("class %s", in.Symbol),
		fmt.Sprintf("def %s", in.Symbol),
		fmt.Sprintf("type %s ", in.Symbol),
		fmt.Sprintf("const %s ", in.Symbol),
		fmt.Sprintf("var %s ", in.Symbol),
	}
	pattern := strings.Join(patterns, "|")

	grep := &GrepTool{}
	grepInput, _ := json.Marshal(map[string]any{
		"pattern":     pattern,
		"output_mode": "content",
	})
	return grep.Execute(ctx, grepInput)
}

func grepSymbols(ctx context.Context, file string) (*Result, error) {
	pattern := `^(func|class|def|type|const|var|export|interface|struct)\s+\w+`
	grep := &GrepTool{}
	grepInput, _ := json.Marshal(map[string]any{
		"pattern":     pattern,
		"path":        file,
		"output_mode": "content",
	})
	return grep.Execute(ctx, grepInput)
}

