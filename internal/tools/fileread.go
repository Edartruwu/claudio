package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Abraxas-365/claudio/internal/config"
	imageutil "github.com/Abraxas-365/claudio/internal/imageutil"
	"github.com/Abraxas-365/claudio/internal/prompts"
	"github.com/Abraxas-365/claudio/internal/tools/outputfilter/codefilter"
	"github.com/Abraxas-365/claudio/internal/tools/readcache"
)

// FileReadTool reads file contents with optional line range.
type FileReadTool struct {
	Security  SecurityChecker
	ReadCache *readcache.Cache
	Config    *config.Settings
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

	// Remap path into worktree when running with worktree isolation.
	in.FilePath = RemapPathForWorktree(ctx, in.FilePath)

	// Security check
	if t.Security != nil {
		if err := t.Security.CheckPath(in.FilePath); err != nil {
			return &Result{Content: fmt.Sprintf("Access denied: %v", err), IsError: true}, nil
		}
	}

	// Image files — return as vision content blocks instead of text
	if imageutil.IsImageFile(in.FilePath) {
		data, mt, err := imageutil.ReadImageFile(in.FilePath)
		if err != nil {
			return &Result{Content: fmt.Sprintf("Error reading image: %v", err), IsError: true}, nil
		}
		return &Result{
			Content: fmt.Sprintf("[Image: %s]", filepath.Base(in.FilePath)),
			Images:  []ImageData{{MediaType: mt, Data: data}},
		}, nil
	}

	// Remember whether the caller explicitly provided offset / limit so we can
	// decide later whether to apply comment-stripping filters.
	originalOffset := in.Offset
	originalLimit := in.Limit

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

	// Read all lines into memory so we can apply comment-stripping filters
	// before formatting with line numbers.
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB line buffer

	var allLines []string
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return &Result{Content: fmt.Sprintf("Error reading file: %v", err), IsError: true}, nil
	}

	// Apply comment-stripping filter for large files — but only when the caller
	// did not specify an explicit offset or limit (i.e. they want the default
	// full-file view).  When a range is requested the caller needs exact content.
	filterLevel := "minimal"
	if t.Config != nil && t.Config.CodeFilterLevel != "" {
		filterLevel = t.Config.CodeFilterLevel
	}
	if originalOffset == 0 && originalLimit == 0 && len(allLines) > 500 && filterLevel != "none" {
		lang := codefilter.DetectLanguage(
			strings.TrimPrefix(filepath.Ext(in.FilePath), "."),
		)
		rawContent := strings.Join(allLines, "\n")
		var filtered string
		switch filterLevel {
		case "aggressive":
			filtered = codefilter.AggressiveFilter(rawContent, lang)
		default: // "minimal" or any unrecognised value
			filtered = codefilter.MinimalFilter(rawContent, lang)
		}
		if len(allLines) > 2000 {
			filtered = codefilter.SmartTruncate(filtered, 2000, lang)
		}
		allLines = strings.Split(filtered, "\n")
		// strings.Split on a newline-terminated string produces a trailing
		// empty element — drop it so line counts stay accurate.
		if len(allLines) > 0 && allLines[len(allLines)-1] == "" {
			allLines = allLines[:len(allLines)-1]
		}
	} else if originalOffset == 0 && originalLimit == 0 && len(allLines) > 2000 {
		// Even with filterLevel "none", apply SmartTruncate as a safety net for
		// very long files to avoid token-budget explosions.
		lang := codefilter.DetectLanguage(
			strings.TrimPrefix(filepath.Ext(in.FilePath), "."),
		)
		rawContent := strings.Join(allLines, "\n")
		truncated := codefilter.SmartTruncate(rawContent, 2000, lang)
		allLines = strings.Split(truncated, "\n")
		if len(allLines) > 0 && allLines[len(allLines)-1] == "" {
			allLines = allLines[:len(allLines)-1]
		}
	}

	totalLines := len(allLines)

	// Build the cat-n style output, honouring offset / limit.
	var output strings.Builder
	linesRead := 0

	for i, line := range allLines {
		lineNum := i + 1
		if lineNum < in.Offset {
			continue
		}
		if linesRead >= in.Limit {
			// Return a notice instead of silent truncation so the model knows
			// it only got a partial view and must use offset/limit to read more.
			output.WriteString(fmt.Sprintf(
				"\n[truncated at line %d — file has more content. Use offset=%d with limit to read the next section, or use Grep to search for specific content.]",
				lineNum, lineNum+1,
			))
			break
		}
		fmt.Fprintf(&output, "%d\t%s\n", lineNum, line)
		linesRead++
	}

	if linesRead == 0 {
		if totalLines == 0 {
			return &Result{Content: "(empty file)"}, nil
		}
		return &Result{Content: fmt.Sprintf("No lines in range (file has %d lines)", totalLines), IsError: true}, nil
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
