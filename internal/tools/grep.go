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

// GrepTool searches file contents using ripgrep.
type GrepTool struct{}

type grepInput struct {
	Pattern    string `json:"pattern"`
	Path       string `json:"path,omitempty"`
	Glob       string `json:"glob,omitempty"`        // file filter e.g. "*.go"
	Type       string `json:"type,omitempty"`         // file type e.g. "go", "py"
	OutputMode string `json:"output_mode,omitempty"`  // "content", "files_with_matches", "count"
	Context    int    `json:"context,omitempty"`       // lines of context (-C)
	ContextC   int    `json:"-C,omitempty"`            // alias for context
	BeforeCtx  int    `json:"-B,omitempty"`            // lines before match
	AfterCtx   int    `json:"-A,omitempty"`            // lines after match
	IgnoreCase bool   `json:"-i,omitempty"`            // case insensitive
	LineNumbers bool  `json:"-n,omitempty"`            // show line numbers
	HeadLimit  int    `json:"head_limit,omitempty"`    // limit output entries
	Offset     int    `json:"offset,omitempty"`        // skip first N entries
	Multiline  bool   `json:"multiline,omitempty"`     // enable multiline mode
}

func (t *GrepTool) Name() string { return "Grep" }

func (t *GrepTool) Description() string {
	return prompts.GrepDescription()
}

func (t *GrepTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {
				"type": "string",
				"description": "The regular expression pattern to search for in file contents"
			},
			"path": {
				"type": "string",
				"description": "File or directory to search in (rg PATH). Defaults to current working directory."
			},
			"glob": {
				"type": "string",
				"description": "Glob pattern to filter files (e.g. \"*.js\", \"*.{ts,tsx}\") - maps to rg --glob"
			},
			"type": {
				"type": "string",
				"description": "File type to search (rg --type). Common types: js, py, rust, go, java, etc."
			},
			"output_mode": {
				"type": "string",
				"enum": ["content", "files_with_matches", "count"],
				"description": "Output mode: \"content\" shows matching lines, \"files_with_matches\" shows file paths (default), \"count\" shows match counts."
			},
			"context": {
				"type": "number",
				"description": "Number of lines to show before and after each match (rg -C). Requires output_mode: \"content\"."
			},
			"-A": {
				"type": "number",
				"description": "Number of lines to show after each match (rg -A). Requires output_mode: \"content\"."
			},
			"-B": {
				"type": "number",
				"description": "Number of lines to show before each match (rg -B). Requires output_mode: \"content\"."
			},
			"-i": {
				"type": "boolean",
				"description": "Case insensitive search (rg -i)"
			},
			"-n": {
				"type": "boolean",
				"description": "Show line numbers in output (rg -n). Defaults to true."
			},
			"head_limit": {
				"type": "number",
				"description": "Limit output to first N lines/entries. Defaults to 250 when unspecified. Pass 0 for unlimited."
			},
			"offset": {
				"type": "number",
				"description": "Skip first N lines/entries before applying head_limit."
			},
			"multiline": {
				"type": "boolean",
				"description": "Enable multiline mode where . matches newlines and patterns can span lines (rg -U --multiline-dotall). Default: false."
			}
		},
		"required": ["pattern"]
	}`)
}

func (t *GrepTool) IsReadOnly() bool { return true }

func (t *GrepTool) RequiresApproval(_ json.RawMessage) bool { return false }

func (t *GrepTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in grepInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	if in.Pattern == "" {
		return &Result{Content: "No pattern provided", IsError: true}, nil
	}

	// Build rg command
	args := []string{"--no-heading", "--color=never"}

	// Output mode
	mode := in.OutputMode
	if mode == "" {
		mode = "files_with_matches"
	}
	switch mode {
	case "files_with_matches":
		args = append(args, "-l")
	case "count":
		args = append(args, "-c")
	case "content":
		args = append(args, "-n") // line numbers
	}

	if in.IgnoreCase {
		args = append(args, "-i")
	}

	if in.Multiline {
		args = append(args, "-U", "--multiline-dotall")
	}

	ctxLines := in.Context
	if in.ContextC > 0 {
		ctxLines = in.ContextC
	}
	if ctxLines > 0 && mode == "content" {
		args = append(args, fmt.Sprintf("-C%d", ctxLines))
	}
	if in.BeforeCtx > 0 && mode == "content" {
		args = append(args, fmt.Sprintf("-B%d", in.BeforeCtx))
	}
	if in.AfterCtx > 0 && mode == "content" {
		args = append(args, fmt.Sprintf("-A%d", in.AfterCtx))
	}

	if in.Glob != "" {
		args = append(args, "--glob", in.Glob)
	}

	if in.Type != "" {
		args = append(args, "--type", in.Type)
	}

	maxResults := in.HeadLimit
	if maxResults == 0 {
		maxResults = 250
	}

	args = append(args, in.Pattern)

	if in.Path != "" {
		args = append(args, in.Path)
	}

	cmd := exec.CommandContext(ctx, "rg", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	output := stdout.String()

	// rg exits with 1 when no matches found — that's not an error
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return &Result{Content: "No matches found"}, nil
		}
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 2 {
			return &Result{Content: fmt.Sprintf("Grep error: %s", stderr.String()), IsError: true}, nil
		}
		// rg not found — fall back to grep
		if strings.Contains(err.Error(), "executable file not found") {
			return t.fallbackGrep(ctx, in)
		}
	}

	// Truncate output
	lines := strings.Split(output, "\n")
	if len(lines) > maxResults {
		output = strings.Join(lines[:maxResults], "\n")
		output += fmt.Sprintf("\n... (%d more results)", len(lines)-maxResults)
	}

	if output == "" {
		return &Result{Content: "No matches found"}, nil
	}

	return &Result{Content: strings.TrimSpace(output)}, nil
}

// fallbackGrep uses system grep when ripgrep isn't available.
func (t *GrepTool) fallbackGrep(ctx context.Context, in grepInput) (*Result, error) {
	args := []string{"-rn", "--color=never"}

	if in.IgnoreCase {
		args = append(args, "-i")
	}

	args = append(args, in.Pattern)

	path := in.Path
	if path == "" {
		path = "."
	}
	args = append(args, path)

	cmd := exec.CommandContext(ctx, "grep", args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Run()

	output := stdout.String()
	if output == "" {
		return &Result{Content: "No matches found"}, nil
	}
	return &Result{Content: strings.TrimSpace(output)}, nil
}
