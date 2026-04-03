package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Abraxas-365/claudio/internal/prompts"
	"github.com/Abraxas-365/claudio/internal/tools/readcache"
)

// FileWriteTool creates or overwrites files.
type FileWriteTool struct {
	Security  SecurityChecker
	ReadCache *readcache.Cache
}

type fileWriteInput struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

func (t *FileWriteTool) Name() string { return "Write" }

func (t *FileWriteTool) Description() string {
	return prompts.WriteDescription()
}

func (t *FileWriteTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"file_path": {
				"type": "string",
				"description": "Absolute path to the file to write"
			},
			"content": {
				"type": "string",
				"description": "The content to write to the file"
			}
		},
		"required": ["file_path", "content"]
	}`)
}

func (t *FileWriteTool) IsReadOnly() bool { return false }

func (t *FileWriteTool) RequiresApproval(_ json.RawMessage) bool { return true }

func (t *FileWriteTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in fileWriteInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	if in.FilePath == "" {
		return &Result{Content: "No file path provided", IsError: true}, nil
	}

	// Security check
	if t.Security != nil {
		if err := t.Security.CheckPath(in.FilePath); err != nil {
			return &Result{Content: fmt.Sprintf("Access denied: %v", err), IsError: true}, nil
		}
	}

	// Staleness check: if the file exists and was previously read, verify it hasn't
	// changed on disk since the last Read. Guards against clobbering external edits.
	if info, err := os.Stat(in.FilePath); err == nil {
		// File exists — check if we have a cache entry with a matching mtime.
		if t.ReadCache != nil {
			key := readcache.Key{FilePath: in.FilePath, Offset: 1, Limit: 2000}
			if _, ok := t.ReadCache.Get(key); !ok {
				// Cache miss: either never read, or mtime changed since the read.
				// Only block if the file actually exists and might have unseen content.
				_ = info // used for the stat above
				return &Result{
					Content: fmt.Sprintf(
						"File %s exists but has not been read in this session (or has changed since last read). "+
							"Use the Read tool first to review the current contents before overwriting.",
						in.FilePath,
					),
					IsError: true,
				}, nil
			}
		}
	}

	// Create parent directories
	dir := filepath.Dir(in.FilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return &Result{Content: fmt.Sprintf("Failed to create directories: %v", err), IsError: true}, nil
	}

	if err := os.WriteFile(in.FilePath, []byte(in.Content), 0644); err != nil {
		return &Result{Content: fmt.Sprintf("Failed to write file: %v", err), IsError: true}, nil
	}

	lines := countLines(in.Content)
	return &Result{Content: fmt.Sprintf("Successfully wrote %d lines to %s", lines, in.FilePath)}, nil
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	n := 1
	for _, c := range s {
		if c == '\n' {
			n++
		}
	}
	return n
}
