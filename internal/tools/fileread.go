package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Abraxas-365/claudio/internal/prompts"
	"github.com/Abraxas-365/claudio/internal/tools/readcache"
)

// FileReadTool reads file contents with optional line range.
type FileReadTool struct {
	Security  SecurityChecker
	ReadCache *readcache.Cache
}

type fileReadInput struct {
	FilePath string `json:"file_path"`
	Offset   int    `json:"offset,omitempty"` // 1-based line number to start from
	Limit    int    `json:"limit,omitempty"`  // number of lines to read
}

func (t *FileReadTool) Name() string { return "Read" }

func (t *FileReadTool) Description() string {
	return prompts.ReadDescription()
}

func (t *FileReadTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"file_path": {
				"type": "string",
				"description": "Absolute path to the file to read"
			},
			"offset": {
				"type": "number",
				"description": "Line number to start reading from (1-based)"
			},
			"limit": {
				"type": "number",
				"description": "Maximum number of lines to read (default: 2000)"
			}
		},
		"required": ["file_path"]
	}`)
}

func (t *FileReadTool) IsReadOnly() bool { return true }

func (t *FileReadTool) RequiresApproval(_ json.RawMessage) bool { return false }

func (t *FileReadTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in fileReadInput
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

	if in.Limit == 0 {
		in.Limit = 2000
	}
	if in.Offset == 0 {
		in.Offset = 1
	}

	// Reject files larger than 256 KB before reading — forces the model to
	// use Grep/Glob to find the relevant section instead of reading everything.
	const maxFileBytes = 256 * 1024
	if info, err := os.Stat(in.FilePath); err == nil && info.Size() > maxFileBytes {
		return &Result{
			Content: fmt.Sprintf(
				"File too large to read in full (%d KB). Use Grep to search for specific content, or read a targeted range with offset and limit parameters.",
				info.Size()/1024,
			),
			IsError: true,
		}, nil
	}

	// Check cache before hitting disk. On a hit, return a stub instead of
	// the full content — the model already has the content from the earlier
	// Read result in this conversation and can refer back to it.
	// This matches Claude Code's read deduplication behaviour and avoids
	// re-spending tokens on unchanged files.
	if t.ReadCache != nil {
		key := readcache.Key{FilePath: in.FilePath, Offset: in.Offset, Limit: in.Limit}
		if _, ok := t.ReadCache.Get(key); ok {
			return &Result{Content: "File unchanged since last read. The content from the earlier Read tool_result in this conversation is still current — refer to that instead of re-reading."}, nil
		}
	}

	file, err := os.Open(in.FilePath)
	if err != nil {
		return &Result{Content: fmt.Sprintf("Error opening file: %v", err), IsError: true}, nil
	}
	defer file.Close()

	var output strings.Builder
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB line buffer

	lineNum := 0
	linesRead := 0

	for scanner.Scan() {
		lineNum++
		if lineNum < in.Offset {
			continue
		}
		if linesRead >= in.Limit {
			// Return an error instead of silent truncation so the model knows
			// it only got a partial view and must use offset/limit to read more.
			output.WriteString(fmt.Sprintf(
				"\n[truncated at line %d — file has more content. Use offset=%d with limit to read the next section, or use Grep to search for specific content.]",
				lineNum, lineNum+1,
			))
			break
		}
		fmt.Fprintf(&output, "%d\t%s\n", lineNum, scanner.Text())
		linesRead++
	}

	if err := scanner.Err(); err != nil {
		return &Result{Content: fmt.Sprintf("Error reading file: %v", err), IsError: true}, nil
	}

	if linesRead == 0 {
		if lineNum == 0 {
			return &Result{Content: "(empty file)"}, nil
		}
		return &Result{Content: fmt.Sprintf("No lines in range (file has %d lines)", lineNum), IsError: true}, nil
	}

	content := output.String()

	// Reject output that would exceed ~25k tokens (estimated at 4 chars/token).
	// An error is better than silent truncation: the model knows it needs to
	// use offset/limit or Grep to find the specific section it cares about.
	const maxOutputTokens = 25_000
	const charsPerToken = 4
	if len(content) > maxOutputTokens*charsPerToken {
		return &Result{
			Content: fmt.Sprintf(
				"File section too large (%d KB, ~%d tokens). Use offset and limit to read a specific range, or use Grep to search for the content you need.",
				len(content)/1024, len(content)/charsPerToken,
			),
			IsError: true,
		}, nil
	}

	// Store in cache for deduplication
	if t.ReadCache != nil {
		if info, err := os.Stat(in.FilePath); err == nil {
			key := readcache.Key{FilePath: in.FilePath, Offset: in.Offset, Limit: in.Limit}
			t.ReadCache.Put(key, content, info.ModTime())
		}
	}

	return &Result{Content: content}, nil
}
