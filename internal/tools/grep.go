package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
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
	IgnoreCase bool   `json:"ignore_case,omitempty"`  // -i flag
	MaxResults int    `json:"max_results,omitempty"`   // limit results
}

func (t *GrepTool) Name() string { return "Grep" }

func (t *GrepTool) Description() string {
	return `Searches file contents using ripgrep. Supports regex patterns, file type filtering, and context lines. Use output_mode "files_with_matches" to just get file paths, or "content" for matching lines.`
}

func (t *GrepTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {
				"type": "string",
				"description": "Regex pattern to search for"
			},
			"path": {
				"type": "string",
				"description": "Directory or file to search in (defaults to cwd)"
			},
			"glob": {
				"type": "string",
				"description": "Glob pattern to filter files (e.g., '*.go')"
			},
			"type": {
				"type": "string",
				"description": "File type to search (e.g., 'go', 'py', 'js')"
			},
			"output_mode": {
				"type": "string",
				"enum": ["content", "files_with_matches", "count"],
				"description": "Output mode (default: files_with_matches)"
			},
			"context": {
				"type": "number",
				"description": "Lines of context around matches"
			},
			"ignore_case": {
				"type": "boolean",
				"description": "Case-insensitive search"
			},
			"max_results": {
				"type": "number",
				"description": "Maximum number of results (default: 250)"
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

	if in.Context > 0 && mode == "content" {
		args = append(args, fmt.Sprintf("-C%d", in.Context))
	}

	if in.Glob != "" {
		args = append(args, "--glob", in.Glob)
	}

	if in.Type != "" {
		args = append(args, "--type", in.Type)
	}

	maxResults := in.MaxResults
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
