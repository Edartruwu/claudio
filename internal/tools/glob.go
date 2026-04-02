package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Abraxas-365/claudio/internal/prompts"
)

// GlobTool finds files matching glob patterns.
type GlobTool struct{}

type globInput struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"` // base directory, defaults to cwd
}

func (t *GlobTool) Name() string { return "Glob" }

func (t *GlobTool) Description() string {
	return prompts.GlobDescription()
}

func (t *GlobTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {
				"type": "string",
				"description": "Glob pattern to match files (e.g., '**/*.go', 'src/**/*.ts')"
			},
			"path": {
				"type": "string",
				"description": "Base directory to search in (defaults to current working directory)"
			}
		},
		"required": ["pattern"]
	}`)
}

func (t *GlobTool) IsReadOnly() bool { return true }

func (t *GlobTool) RequiresApproval(_ json.RawMessage) bool { return false }

func (t *GlobTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in globInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	if in.Pattern == "" {
		return &Result{Content: "No pattern provided", IsError: true}, nil
	}

	baseDir := in.Path
	if baseDir == "" {
		var err error
		baseDir, err = os.Getwd()
		if err != nil {
			return &Result{Content: fmt.Sprintf("Failed to get cwd: %v", err), IsError: true}, nil
		}
	}

	type fileEntry struct {
		path    string
		modTime int64
	}

	var matches []fileEntry

	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip inaccessible paths
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Skip hidden directories
		if info.IsDir() && strings.HasPrefix(info.Name(), ".") && info.Name() != "." {
			return filepath.SkipDir
		}
		// Skip node_modules
		if info.IsDir() && info.Name() == "node_modules" {
			return filepath.SkipDir
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(baseDir, path)
		if err != nil {
			return nil
		}

		matched, err := filepath.Match(in.Pattern, relPath)
		if err != nil {
			// Try matching just the filename for simple patterns
			matched, _ = filepath.Match(in.Pattern, info.Name())
		}

		// Handle ** patterns manually (filepath.Match doesn't support **)
		if !matched && strings.Contains(in.Pattern, "**") {
			matched = matchDoublestar(in.Pattern, relPath)
		}

		if matched {
			matches = append(matches, fileEntry{path: relPath, modTime: info.ModTime().Unix()})
		}

		return nil
	})

	if err != nil {
		return &Result{Content: fmt.Sprintf("Error walking directory: %v", err), IsError: true}, nil
	}

	// Sort by modification time (newest first)
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].modTime > matches[j].modTime
	})

	if len(matches) == 0 {
		return &Result{Content: "No files matched the pattern"}, nil
	}

	const maxResults = 500
	var output strings.Builder
	for i, m := range matches {
		if i >= maxResults {
			fmt.Fprintf(&output, "\n... and %d more files", len(matches)-maxResults)
			break
		}
		output.WriteString(m.path)
		output.WriteString("\n")
	}

	return &Result{Content: strings.TrimSpace(output.String())}, nil
}

// matchDoublestar handles ** glob patterns.
func matchDoublestar(pattern, path string) bool {
	// Split pattern on **
	parts := strings.Split(pattern, "**")
	if len(parts) != 2 {
		return false
	}

	prefix := strings.TrimRight(parts[0], "/")
	suffix := strings.TrimLeft(parts[1], "/")

	// Check if path starts with prefix (if any)
	if prefix != "" && !strings.HasPrefix(path, prefix) {
		return false
	}

	// Check if path ends matching the suffix pattern
	if suffix == "" {
		return true
	}

	// Match suffix against the filename or path tail
	matched, _ := filepath.Match(suffix, filepath.Base(path))
	if matched {
		return true
	}

	// Try matching suffix against progressively longer path tails
	parts2 := strings.Split(path, "/")
	for i := range parts2 {
		tail := strings.Join(parts2[i:], "/")
		if matched, _ := filepath.Match(suffix, tail); matched {
			return true
		}
	}

	return false
}
