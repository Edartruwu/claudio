package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Abraxas-365/claudio/internal/prompts"
	"github.com/Abraxas-365/claudio/internal/snippets"
	"github.com/Abraxas-365/claudio/internal/tools/readcache"
)

// FileEditTool performs exact string replacement in files.
type FileEditTool struct {
	Security      SecurityChecker
	ReadCache     *readcache.Cache
	SnippetConfig *snippets.Config
}

type fileEditInput struct {
	FilePath   string `json:"file_path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

func (t *FileEditTool) Name() string { return "Edit" }

func (t *FileEditTool) Description() string {
	return prompts.EditDescription()
}

func (t *FileEditTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"file_path": {
				"type": "string",
				"description": "Absolute path to the file to edit"
			},
			"old_string": {
				"type": "string",
				"description": "The exact text to find and replace"
			},
			"new_string": {
				"type": "string",
				"description": "The text to replace it with"
			},
			"replace_all": {
				"type": "boolean",
				"description": "Replace all occurrences (default: false)"
			}
		},
		"required": ["file_path", "old_string", "new_string"]
	}`)
}

func (t *FileEditTool) IsReadOnly() bool { return false }

func (t *FileEditTool) RequiresApproval(input json.RawMessage) bool {
	var in fileEditInput
	if json.Unmarshal(input, &in) == nil && isPlanFilePath(in.FilePath) {
		return false // plan files are always auto-accepted
	}
	return true
}

func (t *FileEditTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in fileEditInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	if in.FilePath == "" {
		return &Result{Content: "No file path provided", IsError: true}, nil
	}

	// Remap path into worktree when running with worktree isolation.
	in.FilePath = RemapPathForWorktree(ctx, in.FilePath)

	// Security check
	if t.Security != nil {
		if err := t.Security.CheckPath(in.FilePath); err != nil {
			return &Result{Content: fmt.Sprintf("Access denied: %v", err), IsError: true}, nil
		}
	}

	if in.OldString == "" {
		return &Result{Content: "old_string cannot be empty", IsError: true}, nil
	}
	if in.OldString == in.NewString {
		return &Result{Content: "old_string and new_string are identical", IsError: true}, nil
	}

	// Reject very large files before reading into memory
	const maxEditBytes = 1 * 1024 * 1024 // 1MB
	if info, err := os.Stat(in.FilePath); err == nil && info.Size() > maxEditBytes {
		return &Result{
			Content: fmt.Sprintf("File too large to edit (%d KB). Use Read with offset+limit to find the exact lines, then re-try Edit with a smaller, more targeted old_string.", info.Size()/1024),
			IsError: true,
		}, nil
	}

	content, err := os.ReadFile(in.FilePath)
	if err != nil {
		return &Result{Content: fmt.Sprintf("Failed to read file: %v", err), IsError: true}, nil
	}

	text := string(content)

	// Expand snippets in new_string using full file context for resolution
	if t.SnippetConfig != nil && t.SnippetConfig.Enabled {
		in.NewString = snippets.ExpandWithContext(t.SnippetConfig, in.FilePath, in.NewString, text)
	}

	if !in.ReplaceAll {
		// Check uniqueness
		count := strings.Count(text, in.OldString)
		if count == 0 {
			return &Result{Content: "old_string not found in file", IsError: true}, nil
		}
		if count > 1 {
			return &Result{
				Content: fmt.Sprintf("old_string found %d times — must be unique. Provide more context or use replace_all.", count),
				IsError: true,
			}, nil
		}
		text = strings.Replace(text, in.OldString, in.NewString, 1)
	} else {
		count := strings.Count(text, in.OldString)
		if count == 0 {
			return &Result{Content: "old_string not found in file", IsError: true}, nil
		}
		text = strings.ReplaceAll(text, in.OldString, in.NewString)
	}

	if err := os.WriteFile(in.FilePath, []byte(text), 0644); err != nil {
		return &Result{Content: fmt.Sprintf("Failed to write file: %v", err), IsError: true}, nil
	}

	// Invalidate the read cache so the next Read re-reads from disk.
	// This matches claude-code's pattern: Edit updates readFileState with
	// offset=undefined, which prevents the dedup check from matching.
	if t.ReadCache != nil {
		t.ReadCache.Invalidate(in.FilePath)
	}

	return &Result{Content: fmt.Sprintf("Successfully edited %s", in.FilePath)}, nil
}
