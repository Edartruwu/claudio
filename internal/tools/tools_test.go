package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Abraxas-365/claudio/internal/tools"
	"github.com/Abraxas-365/claudio/internal/tools/grepcache"
	"github.com/Abraxas-365/claudio/internal/tools/readcache"
)

func TestRegistry(t *testing.T) {
	r := tools.DefaultRegistry()

	expectedTools := []string{
		"Bash", "Read", "Write", "Edit", "Glob", "Grep",
		"Agent", "Skill", "EnterPlanMode", "ExitPlanMode", "ToolSearch",
		"WebSearch", "WebFetch", "LSP", "NotebookEdit",
		"TaskCreate", "TaskList", "TaskGet", "TaskUpdate",
		"EnterWorktree", "ExitWorktree",
		"BgTaskList", "TaskStop", "TaskOutput",
		"SendMessage",
		"SpawnTeammate", "InstantiateTeam", "PurgeTeammates", "ListTeammates",
		"SendToSession", "SpawnSession",
		"Memory", "Recall",
		"CronCreate", "CronDelete", "CronList",
		"AskUser", "ListDesigns",
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
		"EnterWorktree": true, "ExitWorktree": true,
		"TaskStop": true, "TaskOutput": true, "SendMessage": true,
		"SendToSession": true, "SpawnSession": true,
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

func TestBashToolTimeout(t *testing.T) {
	tool := &tools.BashTool{}
	// Use a short timeout (500ms) with a command that sleeps forever.
	input, _ := json.Marshal(map[string]any{
		"command": "sleep 3600",
		"timeout": 500,
	})

	start := time.Now()
	result, err := tool.Execute(context.Background(), input)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(result.Content, "timed out") {
		t.Errorf("expected timeout message, got: %s", result.Content)
	}
	// Must return well before the sleep duration. Allow generous margin
	// for CI, but it must not hang for minutes.
	if elapsed > 10*time.Second {
		t.Errorf("timeout took too long: %v (expected <10s)", elapsed)
	}
}

func TestBashToolTimeoutKillsChildProcesses(t *testing.T) {
	// This is the actual bug scenario: bash spawns a child process that
	// inherits stdout/stderr pipes. Without process-group killing,
	// cmd.Run() blocks forever even after bash is killed because the
	// child keeps the pipes open.
	tool := &tools.BashTool{}
	// bash -c spawns a subshell; the subshell spawns "sleep 3600" as a child.
	// The nested bash ensures there's a child process that outlives the parent.
	input, _ := json.Marshal(map[string]any{
		"command": "bash -c 'echo started; sleep 3600'",
		"timeout": 500,
	})

	start := time.Now()
	result, err := tool.Execute(context.Background(), input)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(result.Content, "timed out") {
		t.Errorf("expected timeout message, got: %s", result.Content)
	}
	// The key assertion: must not hang. The old code would block here
	// because the child "sleep 3600" kept the pipes open.
	if elapsed > 10*time.Second {
		t.Errorf("process group kill failed — hung for %v (expected <10s)", elapsed)
	}
}

func TestBashToolContextCancellation(t *testing.T) {
	tool := &tools.BashTool{}
	input, _ := json.Marshal(map[string]any{
		"command": "sleep 3600",
	})

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after 300ms
	go func() {
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := tool.Execute(ctx, input)
	elapsed := time.Since(start)

	// Context cancellation may surface as an error return or a result error
	_ = err

	if elapsed > 10*time.Second {
		t.Errorf("context cancellation took too long: %v", elapsed)
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

func TestPlanModeToolsAreNotDeferred(t *testing.T) {
	r := tools.DefaultRegistry()

	for _, name := range []string{"EnterPlanMode", "ExitPlanMode"} {
		tool, err := r.Get(name)
		if err != nil {
			t.Fatalf("tool %q not found: %v", name, err)
		}

		// If the tool implements DeferrableTool, ShouldDefer must return false.
		if dt, ok := tool.(tools.DeferrableTool); ok {
			if dt.ShouldDefer() {
				t.Errorf("%s should NOT be deferred — AI must always see its description to proactively enter/exit plan mode", name)
			}
		}
		// If the tool does NOT implement DeferrableTool at all, that's also correct (non-deferred).
	}

	// Also verify they appear with full descriptions in deferred definitions (no discovery needed)
	deferredDefs := r.APIDefinitionsWithDeferral(nil) // no discovered tools
	var parsed []tools.APIToolDef
	json.Unmarshal(deferredDefs, &parsed)

	byName := map[string]tools.APIToolDef{}
	for _, def := range parsed {
		byName[def.Name] = def
	}

	for _, name := range []string{"EnterPlanMode", "ExitPlanMode"} {
		def, ok := byName[name]
		if !ok {
			t.Errorf("%s should be present in deferred definitions (always loaded)", name)
			continue
		}
		if def.Description == "" {
			t.Errorf("%s should have a full description even in deferred mode", name)
		}
	}
}

// ── FileReadTool ReadCache deduplication ─────────────────────────────────────

// TestFileReadTool_ReadCache_HitReturnsDeupMessage verifies that reading the same
// unchanged file twice returns the dedup stub on the second call instead of the
// full content. This is the mechanism that prevents re-read loops after MicroCompact
// clears old tool results without invalidating the cache.
func TestFileReadTool_ReadCache_HitReturnsDedupMessage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "engine.go")
	os.WriteFile(path, []byte("package query\n"), 0644)

	rc := readcache.New(64)
	tool := &tools.FileReadTool{ReadCache: rc}
	input, _ := json.Marshal(map[string]any{"file_path": path})

	// First read: cache miss → full content returned and cached.
	first, err := tool.Execute(context.Background(), input)
	if err != nil || first.IsError {
		t.Fatalf("first read failed: %v / %s", err, first.Content)
	}
	if !strings.Contains(first.Content, "package query") {
		t.Fatalf("first read: expected file content, got: %s", first.Content)
	}

	// Second read of the same unchanged file: cache hit → dedup stub returned.
	second, err := tool.Execute(context.Background(), input)
	if err != nil || second.IsError {
		t.Fatalf("second read failed: %v / %s", err, second.Content)
	}
	if strings.Contains(second.Content, "package query") {
		t.Fatal("second read: should NOT return full file content on cache hit")
	}
	if !strings.Contains(second.Content, "File unchanged since last read") {
		t.Fatalf("second read: dedup stub should contain 'File unchanged since last read'; got: %s", second.Content)
	}
}

// TestFileReadTool_ReadCache_NilCacheAlwaysReadsFile confirms that when ReadCache is
// nil (e.g. sub-agents with no shared cache) every call reads from disk normally.
func TestFileReadTool_ReadCache_NilCacheAlwaysReadsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.go")
	os.WriteFile(path, []byte("content\n"), 0644)

	tool := &tools.FileReadTool{} // no ReadCache

	for i := 0; i < 3; i++ {
		input, _ := json.Marshal(map[string]any{"file_path": path})
		result, err := tool.Execute(context.Background(), input)
		if err != nil || result.IsError {
			t.Fatalf("read %d failed: %v / %s", i, err, result.Content)
		}
		if !strings.Contains(result.Content, "content") {
			t.Fatalf("read %d: expected file content, got: %s", i, result.Content)
		}
	}
}

// TestFileReadTool_ReadCache_ModifiedFileBypassesCache verifies that if the file
// changes after the first read, the cache entry is invalidated and the new content
// is returned on the next read.
func TestFileReadTool_ReadCache_ModifiedFileBypassesCache(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "changing.go")
	os.WriteFile(path, []byte("original\n"), 0644)

	rc := readcache.New(64)
	tool := &tools.FileReadTool{ReadCache: rc}
	input, _ := json.Marshal(map[string]any{"file_path": path})

	// First read: caches "original".
	first, _ := tool.Execute(context.Background(), input)
	if !strings.Contains(first.Content, "original") {
		t.Fatalf("first read: expected 'original', got: %s", first.Content)
	}

	// Overwrite the file (mtime changes).
	os.WriteFile(path, []byte("updated\n"), 0644)

	// Second read: mtime mismatch → cache miss → new content returned.
	second, err := tool.Execute(context.Background(), input)
	if err != nil || second.IsError {
		t.Fatalf("second read failed: %v / %s", err, second.Content)
	}
	if !strings.Contains(second.Content, "updated") {
		t.Fatalf("second read after file change: expected 'updated', got: %s", second.Content)
	}
}

// ── GrepTool cache deduplication ─────────────────────────────────────────────

// TestGrepTool_Cache_DeduplicatesIdenticalSearch verifies that running the same
// grep twice returns a cached result on the second call without re-executing rg.
// This is the main fix for `Grep "." file` being used as a Read substitute
// and then repeated across turns.
func TestGrepTool_Cache_DeduplicatesIdenticalSearch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.go")
	os.WriteFile(path, []byte("package web\n\nfunc Serve() {}\n"), 0644)

	gc := grepcache.New(64)
	tool := &tools.GrepTool{Cache: gc}

	input, _ := json.Marshal(map[string]any{
		"pattern":     ".",
		"path":        path,
		"output_mode": "content",
	})

	first, err := tool.Execute(context.Background(), input)
	if err != nil || first.IsError {
		t.Fatalf("first grep failed: %v / %s", err, first.Content)
	}
	if !strings.Contains(first.Content, "package web") {
		t.Fatalf("first grep: expected file content, got: %s", first.Content)
	}

	// Second call with identical input and unchanged file: must return cached result.
	second, err := tool.Execute(context.Background(), input)
	if err != nil || second.IsError {
		t.Fatalf("second grep failed: %v / %s", err, second.Content)
	}
	if second.Content != first.Content {
		t.Fatalf("second grep should return identical cached content\ngot:  %s\nwant: %s", second.Content, first.Content)
	}
}

// TestGrepTool_Cache_NilCacheAlwaysRuns confirms that when Cache is nil every
// invocation runs rg normally.
func TestGrepTool_Cache_NilCacheAlwaysRuns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.go")
	os.WriteFile(path, []byte("func A() {}\n"), 0644)

	tool := &tools.GrepTool{} // no cache

	input, _ := json.Marshal(map[string]any{"pattern": "func", "path": path, "output_mode": "content"})
	result, err := tool.Execute(context.Background(), input)
	if err != nil || result.IsError {
		t.Fatalf("grep failed: %v / %s", err, result.Content)
	}
	if !strings.Contains(result.Content, "func A") {
		t.Fatalf("expected grep result, got: %s", result.Content)
	}
}

// TestGrepTool_Cache_DifferentFlagsAreSeparateEntries verifies that changing
// any input field (e.g. IgnoreCase) results in a separate cache entry.
func TestGrepTool_Cache_DifferentFlagsAreSeparateEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.go")
	os.WriteFile(path, []byte("HELLO\nhello\n"), 0644)

	gc := grepcache.New(64)
	tool := &tools.GrepTool{Cache: gc}

	caseSensitive, _ := json.Marshal(map[string]any{"pattern": "HELLO", "path": path, "output_mode": "content"})
	caseInsensitive, _ := json.Marshal(map[string]any{"pattern": "HELLO", "path": path, "output_mode": "content", "-i": true})

	r1, _ := tool.Execute(context.Background(), caseSensitive)
	r2, _ := tool.Execute(context.Background(), caseInsensitive)

	// Case-sensitive: only "HELLO" line; case-insensitive: both lines.
	if strings.Count(r1.Content, "hello") != 0 || strings.Count(r1.Content, "HELLO") == 0 {
		t.Fatalf("case-sensitive result unexpected: %s", r1.Content)
	}
	if !strings.Contains(r2.Content, "HELLO") || !strings.Contains(r2.Content, "hello") {
		t.Fatalf("case-insensitive result unexpected: %s", r2.Content)
	}
}

// TestFileReadTool_ReadCache_NudgesAwayFromGrep verifies the dedup stub message
// does NOT suggest using Grep as a workaround (which caused the bypass loop).
func TestFileReadTool_ReadCache_NudgesAwayFromGrep(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "handler.go")
	os.WriteFile(path, []byte("package web\n"), 0644)

	rc := readcache.New(64)
	tool := &tools.FileReadTool{ReadCache: rc}
	input, _ := json.Marshal(map[string]any{"file_path": path})

	tool.Execute(context.Background(), input) // prime cache

	second, _ := tool.Execute(context.Background(), input)
	if strings.Contains(strings.ToLower(second.Content), "use grep") {
		t.Fatalf("dedup stub must not suggest 'use Grep' — causes bypass loop; got: %s", second.Content)
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
