package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Abraxas-365/claudio/internal/prompts"
	"github.com/Abraxas-365/claudio/internal/snippets"
	"github.com/Abraxas-365/claudio/internal/tools/readcache"
)

// isPlanFilePath returns true if the path is inside the claudio plans directory.
// Plan files are created and managed by claudio itself, so they should always
// be writable without a permission prompt or staleness check.
func isPlanFilePath(path string) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	planDir := filepath.Join(home, ".claudio", "plans") + string(filepath.Separator)
	return strings.HasPrefix(filepath.Clean(path)+string(filepath.Separator), planDir)
}

// FileWriteTool creates or overwrites files.
type FileWriteTool struct {
	Security      SecurityChecker
	ReadCache     *readcache.Cache
	SnippetConfig *snippets.Config
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

func (t *FileWriteTool) RequiresApproval(input json.RawMessage) bool {
	var in fileWriteInput
	if json.Unmarshal(input, &in) == nil && isPlanFilePath(in.FilePath) {
		return false // plan files are always auto-accepted
	}
	return true
}

// Validate runs pre-approval checks so errors surface before the user types "allow".
func (t *FileWriteTool) Validate(input json.RawMessage) *Result {
	var in fileWriteInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}
	}
	if in.FilePath == "" {
		return &Result{Content: "No file path provided", IsError: true}
	}
	if t.Security != nil {
		if err := t.Security.CheckPath(in.FilePath); err != nil {
			return &Result{Content: fmt.Sprintf("Access denied: %v", err), IsError: true}
		}
	}
	// Staleness check: if the file exists and has content, it must have been read
	// (or written) in this session. Empty files are skipped — there is nothing to
	// review before overwriting. Files written earlier in the session are also
	// skipped because the model already knows what it just wrote.
	if !isPlanFilePath(in.FilePath) {
		if info, err := os.Stat(in.FilePath); err == nil && info.Size() > 0 {
			if t.ReadCache != nil {
				if t.ReadCache.WrittenSince(in.FilePath) {
					return nil
				}
				key := readcache.Key{FilePath: in.FilePath, Offset: 1, Limit: 2000}
				if _, ok := t.ReadCache.Get(key); !ok {
					return &Result{
						Content: fmt.Sprintf(
							"File %s exists but has not been read in this session (or has changed since last read). "+
								"Use the Read tool first to review the current contents before overwriting.",
							in.FilePath,
						),
						IsError: true,
					}
				}
			}
		}
	}
	return nil
}

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

	// Create parent directories
	dir := filepath.Dir(in.FilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return &Result{Content: fmt.Sprintf("Failed to create directories: %v", err), IsError: true}, nil
	}

	// Expand snippets before writing
	if t.SnippetConfig != nil && t.SnippetConfig.Enabled {
		in.Content = snippets.Expand(t.SnippetConfig, in.FilePath, in.Content)
	}

	if err := os.WriteFile(in.FilePath, []byte(in.Content), 0644); err != nil {
		return &Result{Content: fmt.Sprintf("Failed to write file: %v", err), IsError: true}, nil
	}

	// Invalidate any cached Read results so the next Read re-reads from disk,
	// then mark the file as written so a follow-up Write to the same path
	// doesn't require a fresh Read first (the model just wrote it — it knows
	// what's in it).
	if t.ReadCache != nil {
		t.ReadCache.Invalidate(in.FilePath)
		if info, err := os.Stat(in.FilePath); err == nil {
			t.ReadCache.MarkWritten(in.FilePath, info.ModTime())
		}
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
