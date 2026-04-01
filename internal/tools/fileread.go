package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// FileReadTool reads file contents with optional line range.
type FileReadTool struct{}

type fileReadInput struct {
	FilePath string `json:"file_path"`
	Offset   int    `json:"offset,omitempty"` // 1-based line number to start from
	Limit    int    `json:"limit,omitempty"`  // number of lines to read
}

func (t *FileReadTool) Name() string { return "Read" }

func (t *FileReadTool) Description() string {
	return `Reads a file from the filesystem. Returns the file content with line numbers. Use offset and limit to read specific ranges of large files.`
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

	if in.Limit == 0 {
		in.Limit = 2000
	}
	if in.Offset == 0 {
		in.Offset = 1
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
			output.WriteString(fmt.Sprintf("\n... (truncated at %d lines, use offset to read more)", in.Limit))
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

	return &Result{Content: output.String()}, nil
}
