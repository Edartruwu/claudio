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

	expectedTools := []string{
		"Bash", "Read", "Write", "Edit", "Glob", "Grep",
		"Agent", "ToolSearch",
		"WebSearch", "WebFetch", "LSP", "NotebookEdit",
		"TaskCreate", "TaskList", "TaskGet", "TaskUpdate",
		"EnterWorktree", "ExitWorktree", "EnterPlanMode", "ExitPlanMode",
		"TaskStop", "TaskOutput",
		"TeamCreate", "TeamDelete", "SendMessage",
		"Memory",
		"CronCreate", "CronDelete", "CronList",
		"AskUser",
	}

	// Test full definitions (no deferral)
	fullDefs := r.APIDefinitions()
	var fullParsed []tools.APIToolDef
	if err := json.Unmarshal(fullDefs, &fullParsed); err != nil {
		t.Fatalf("failed to parse full API definitions: %v", err)
	}
	if len(fullParsed) != len(expectedTools) {
		t.Fatalf("expected %d tools, got %d", len(expectedTools), len(fullParsed))
	}
	for i, name := range expectedTools {
		if fullParsed[i].Name != name {
			t.Errorf("tool %d: expected %q, got %q", i, name, fullParsed[i].Name)
		}
		if fullParsed[i].Description == "" {
			t.Errorf("full tool %q should have a description", name)
		}
	}

	// Test deferred definitions — undiscovered deferred tools must be absent entirely
	deferredDefs := r.APIDefinitionsWithDeferral(nil)
	var deferredParsed []tools.APIToolDef
	if err := json.Unmarshal(deferredDefs, &deferredParsed); err != nil {
		t.Fatalf("failed to parse deferred API definitions: %v", err)
	}

	deferredNames := map[string]bool{
		"WebSearch": true, "WebFetch": true, "LSP": true, "NotebookEdit": true,
		"TaskCreate": true, "TaskList": true, "TaskGet": true, "TaskUpdate": true,
		"EnterWorktree": true, "ExitWorktree": true, "EnterPlanMode": true, "ExitPlanMode": true,
		"TaskStop": true, "TaskOutput": true, "TeamCreate": true, "TeamDelete": true, "SendMessage": true,
		"Memory": true,
		"CronCreate": true, "CronDelete": true, "CronList": true,
		"AskUser": true,
	}
	for _, def := range deferredParsed {
		if deferredNames[def.Name] {
			t.Errorf("undiscovered deferred tool %q should be absent from the tools array", def.Name)
		}
		if def.Description == "" {
			t.Errorf("tool %q present in array must have a description", def.Name)
		}
	}

	// Test that discovered tools appear with full schemas
	discovered := map[string]bool{"WebSearch": true, "TaskCreate": true}
	partialDefs := r.APIDefinitionsWithDeferral(discovered)
	var partialParsed []tools.APIToolDef
	json.Unmarshal(partialDefs, &partialParsed)
	partialByName := map[string]tools.APIToolDef{}
	for _, def := range partialParsed {
		partialByName[def.Name] = def
	}
	for _, name := range []string{"WebSearch", "TaskCreate"} {
		def, ok := partialByName[name]
		if !ok {
			t.Errorf("discovered tool %q should be present in the tools array", name)
			continue
		}
		if def.Description == "" {
			t.Errorf("discovered tool %q should have a description", name)
		}
	}
	// Undiscovered deferred tools should still be absent
	if _, ok := partialByName["WebFetch"]; ok {
		t.Errorf("undiscovered tool WebFetch should still be absent when only WebSearch/TaskCreate are discovered")
	}

	// Verify deferred definitions are significantly smaller than full
	if len(deferredDefs) >= len(fullDefs) {
		t.Errorf("deferred defs (%d bytes) should be smaller than full defs (%d bytes)",
			len(deferredDefs), len(fullDefs))
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
