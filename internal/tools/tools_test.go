package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Abraxas-365/claudio/internal/tools"
)

func TestRegistry(t *testing.T) {
	r := tools.DefaultRegistry()
	defs := r.APIDefinitions()
	if len(defs) == 0 {
		t.Fatal("expected non-empty API definitions")
	}

	var parsed []tools.APIToolDef
	if err := json.Unmarshal(defs, &parsed); err != nil {
		t.Fatalf("failed to parse API definitions: %v", err)
	}

	expectedTools := []string{
		"Bash", "Read", "Write", "Edit", "Glob", "Grep",
		"WebSearch", "WebFetch", "LSP", "NotebookEdit",
		"TaskCreate", "TaskList", "TaskGet", "TaskUpdate",
		"EnterWorktree", "ExitWorktree", "EnterPlanMode", "ExitPlanMode",
		"Agent",
	}
	if len(parsed) != len(expectedTools) {
		t.Fatalf("expected %d tools, got %d", len(expectedTools), len(parsed))
	}

	for i, name := range expectedTools {
		if parsed[i].Name != name {
			t.Errorf("tool %d: expected %q, got %q", i, name, parsed[i].Name)
		}
	}
}

func TestFileReadTool(t *testing.T) {
	// Create temp file
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0644)

	tool := &tools.FileReadTool{}
	input, _ := json.Marshal(map[string]any{"file_path": path})

	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "line1") {
		t.Errorf("expected content to contain 'line1', got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "1\t") {
		t.Error("expected line numbers in output")
	}
}

func TestFileWriteAndEditTool(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	// Write
	writeTool := &tools.FileWriteTool{}
	writeInput, _ := json.Marshal(map[string]any{
		"file_path": path,
		"content":   "hello world\nfoo bar\n",
	})

	result, err := writeTool.Execute(context.Background(), writeInput)
	if err != nil || result.IsError {
		t.Fatalf("write failed: %v %s", err, result.Content)
	}

	// Edit
	editTool := &tools.FileEditTool{}
	editInput, _ := json.Marshal(map[string]any{
		"file_path":  path,
		"old_string": "foo bar",
		"new_string": "baz qux",
	})

	result, err = editTool.Execute(context.Background(), editInput)
	if err != nil || result.IsError {
		t.Fatalf("edit failed: %v %s", err, result.Content)
	}

	// Verify
	content, _ := os.ReadFile(path)
	if !strings.Contains(string(content), "baz qux") {
		t.Errorf("expected 'baz qux' in file, got: %s", string(content))
	}
	if strings.Contains(string(content), "foo bar") {
		t.Error("expected 'foo bar' to be replaced")
	}
}

func TestGlobTool(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte("package b"), 0644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("text"), 0644)

	tool := &tools.GlobTool{}
	input, _ := json.Marshal(map[string]any{
		"pattern": "*.go",
		"path":    dir,
	})

	result, err := tool.Execute(context.Background(), input)
	if err != nil || result.IsError {
		t.Fatalf("glob failed: %v %s", err, result.Content)
	}

	if !strings.Contains(result.Content, "a.go") || !strings.Contains(result.Content, "b.go") {
		t.Errorf("expected a.go and b.go in results, got: %s", result.Content)
	}
	if strings.Contains(result.Content, "c.txt") {
		t.Error("expected c.txt to be excluded")
	}
}

func TestBashTool(t *testing.T) {
	tool := &tools.BashTool{}
	input, _ := json.Marshal(map[string]any{
		"command": "echo hello",
	})

	result, err := tool.Execute(context.Background(), input)
	if err != nil || result.IsError {
		t.Fatalf("bash failed: %v %s", err, result.Content)
	}

	if strings.TrimSpace(result.Content) != "hello" {
		t.Errorf("expected 'hello', got: %q", result.Content)
	}
}

func TestBashToolSafety(t *testing.T) {
	tool := &tools.BashTool{}
	input, _ := json.Marshal(map[string]any{
		"command": "rm -rf /",
	})

	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected dangerous command to be blocked")
	}
}

func TestFileEditNonUnique(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("aaa\naaa\n"), 0644)

	tool := &tools.FileEditTool{}
	input, _ := json.Marshal(map[string]any{
		"file_path":  path,
		"old_string": "aaa",
		"new_string": "bbb",
	})

	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error for non-unique old_string")
	}
	if !strings.Contains(result.Content, "2 times") {
		t.Errorf("expected message about 2 occurrences, got: %s", result.Content)
	}
}
