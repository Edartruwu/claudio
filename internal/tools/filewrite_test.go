package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Abraxas-365/claudio/internal/tools/readcache"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// mockSecurity is a SecurityChecker that denies paths containing "forbidden".
type mockSecurity struct {
	denySubstring string
}

func (m *mockSecurity) CheckPath(path string) error {
	if m.denySubstring != "" && strings.Contains(path, m.denySubstring) {
		return fmt.Errorf("path %q is denied", path)
	}
	return nil
}

func (m *mockSecurity) CheckCommand(cmd string) error { return nil }

func writeInput(filePath, content string) json.RawMessage {
	b, _ := json.Marshal(fileWriteInput{FilePath: filePath, Content: content})
	return b
}

// tmpFile creates a temp file with content and returns its path. Caller should defer os.Remove.
func tmpFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "fw-test-*")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}

// cacheFile adds the file to the ReadCache so it passes the staleness check.
func cacheFile(t *testing.T, rc *readcache.Cache, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	rc.Put(readcache.Key{FilePath: path, Offset: 1, Limit: 2000}, "cached", info.ModTime())
}

// ---------------------------------------------------------------------------
// Validate tests
// ---------------------------------------------------------------------------

func TestValidate_InvalidJSON(t *testing.T) {
	tool := &FileWriteTool{}
	result := tool.Validate(context.Background(), json.RawMessage(`{bad json`))
	if result == nil || !result.IsError {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(result.Content, "Invalid input") {
		t.Errorf("unexpected message: %s", result.Content)
	}
}

func TestValidate_EmptyFilePath(t *testing.T) {
	tool := &FileWriteTool{}
	result := tool.Validate(context.Background(), writeInput("", "content"))
	if result == nil || !result.IsError {
		t.Fatal("expected error for empty file path")
	}
	if !strings.Contains(result.Content, "No file path") {
		t.Errorf("unexpected message: %s", result.Content)
	}
}

func TestValidate_SecurityDeny(t *testing.T) {
	tool := &FileWriteTool{
		Security: &mockSecurity{denySubstring: "forbidden"},
	}
	result := tool.Validate(context.Background(), writeInput("/tmp/forbidden/file.txt", "x"))
	if result == nil || !result.IsError {
		t.Fatal("expected error for denied path")
	}
	if !strings.Contains(result.Content, "Access denied") {
		t.Errorf("unexpected message: %s", result.Content)
	}
}

func TestValidate_SecurityAllow(t *testing.T) {
	tool := &FileWriteTool{
		Security: &mockSecurity{denySubstring: "forbidden"},
	}
	// Non-existent file, no ReadCache — should pass (new file creation).
	result := tool.Validate(context.Background(), writeInput("/tmp/safe-nonexistent-"+fmt.Sprint(time.Now().UnixNano()), "x"))
	if result != nil {
		t.Errorf("expected nil for allowed path, got: %s", result.Content)
	}
}

func TestValidate_NilSecurity(t *testing.T) {
	tool := &FileWriteTool{}
	// Non-existent file, no ReadCache → pass.
	result := tool.Validate(context.Background(), writeInput("/tmp/no-sec-"+fmt.Sprint(time.Now().UnixNano()), "x"))
	if result != nil {
		t.Errorf("expected nil, got: %s", result.Content)
	}
}

func TestValidate_StalenessCheck_FileExistsNotRead(t *testing.T) {
	path := tmpFile(t, "existing content")
	rc := readcache.New(16)
	tool := &FileWriteTool{ReadCache: rc}

	result := tool.Validate(context.Background(), writeInput(path, "new content"))
	if result == nil || !result.IsError {
		t.Fatal("expected staleness error when file exists but not read")
	}
	if !strings.Contains(result.Content, "has not been read") {
		t.Errorf("unexpected message: %s", result.Content)
	}
}

func TestValidate_StalenessCheck_FileExistsAndRead(t *testing.T) {
	path := tmpFile(t, "existing content")
	rc := readcache.New(16)
	cacheFile(t, rc, path)
	tool := &FileWriteTool{ReadCache: rc}

	result := tool.Validate(context.Background(), writeInput(path, "new content"))
	if result != nil {
		t.Errorf("expected nil when file was read, got: %s", result.Content)
	}
}

func TestValidate_StalenessCheck_FileChangedAfterRead(t *testing.T) {
	path := tmpFile(t, "original")
	rc := readcache.New(16)
	cacheFile(t, rc, path)

	// Modify the file so mtime changes, invalidating cache entry.
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(path, []byte("externally changed"), 0644); err != nil {
		t.Fatal(err)
	}

	tool := &FileWriteTool{ReadCache: rc}
	result := tool.Validate(context.Background(), writeInput(path, "overwrite"))
	if result == nil || !result.IsError {
		t.Fatal("expected staleness error when file changed after read")
	}
	if !strings.Contains(result.Content, "has not been read") {
		t.Errorf("unexpected message: %s", result.Content)
	}
}

func TestValidate_NewFile_NoReadCacheRequired(t *testing.T) {
	dir := t.TempDir()
	newPath := filepath.Join(dir, "brand-new.txt")
	rc := readcache.New(16)
	tool := &FileWriteTool{ReadCache: rc}

	result := tool.Validate(context.Background(), writeInput(newPath, "hello"))
	if result != nil {
		t.Errorf("new file should not require read cache, got: %s", result.Content)
	}
}

func TestValidate_StalenessCheck_EmptyFileSkipped(t *testing.T) {
	// An existing empty file should not require a prior Read.
	path := tmpFile(t, "")
	rc := readcache.New(16)
	tool := &FileWriteTool{ReadCache: rc}

	result := tool.Validate(context.Background(), writeInput(path, "new content"))
	if result != nil {
		t.Errorf("empty file should bypass staleness check, got: %s", result.Content)
	}
}

func TestValidate_NilReadCache_SkipsStaleness(t *testing.T) {
	path := tmpFile(t, "existing")
	tool := &FileWriteTool{} // no ReadCache

	result := tool.Validate(context.Background(), writeInput(path, "overwrite"))
	if result != nil {
		t.Errorf("without ReadCache, staleness should be skipped, got: %s", result.Content)
	}
}

func TestValidate_PlanFile_BypassesStaleness(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}
	planDir := filepath.Join(home, ".claudio", "plans")
	// We don't create the file — isPlanFilePath only checks the prefix.
	// But Validate only does the staleness check if the file exists, so make a real file.
	if err := os.MkdirAll(planDir, 0755); err != nil {
		t.Skip("cannot create plan dir")
	}
	planFile := filepath.Join(planDir, "test-plan.md")
	if err := os.WriteFile(planFile, []byte("plan"), 0644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(planFile)

	rc := readcache.New(16)
	// Don't cache it — normally this would fail staleness, but plan files are exempt.
	tool := &FileWriteTool{ReadCache: rc}

	result := tool.Validate(context.Background(), writeInput(planFile, "updated plan"))
	if result != nil {
		t.Errorf("plan files should bypass staleness, got: %s", result.Content)
	}
}

func TestValidate_SecurityCheckedBeforeStaleness(t *testing.T) {
	// Even if file exists and isn't cached, security denial should come first.
	path := tmpFile(t, "existing")
	rc := readcache.New(16)
	tool := &FileWriteTool{
		Security:  &mockSecurity{denySubstring: filepath.Base(path)},
		ReadCache: rc,
	}

	result := tool.Validate(context.Background(), writeInput(path, "x"))
	if result == nil || !result.IsError {
		t.Fatal("expected error")
	}
	// Should be security error, not staleness error.
	if !strings.Contains(result.Content, "Access denied") {
		t.Errorf("expected Access denied, got: %s", result.Content)
	}
}

func TestValidate_PassReturnsNil(t *testing.T) {
	dir := t.TempDir()
	newPath := filepath.Join(dir, "new.txt")
	tool := &FileWriteTool{
		Security:  &mockSecurity{},
		ReadCache: readcache.New(16),
	}
	result := tool.Validate(context.Background(), writeInput(newPath, "content"))
	if result != nil {
		t.Errorf("expected nil for valid input, got: %s", result.Content)
	}
}

// ---------------------------------------------------------------------------
// Validate — additional edge cases
// ---------------------------------------------------------------------------

func TestValidate_EmptyContent(t *testing.T) {
	// Empty content is valid — writing an empty file is allowed.
	dir := t.TempDir()
	tool := &FileWriteTool{}
	result := tool.Validate(context.Background(), writeInput(filepath.Join(dir, "empty.txt"), ""))
	if result != nil {
		t.Errorf("empty content should be valid, got: %s", result.Content)
	}
}

func TestValidate_MissingFieldsInJSON(t *testing.T) {
	// JSON with only content but no file_path → empty file path error.
	tool := &FileWriteTool{}
	result := tool.Validate(context.Background(), json.RawMessage(`{"content":"hello"}`))
	if result == nil || !result.IsError {
		t.Fatal("expected error for missing file_path")
	}
	if !strings.Contains(result.Content, "No file path") {
		t.Errorf("unexpected: %s", result.Content)
	}
}

func TestValidate_ExtraFieldsInJSON(t *testing.T) {
	// Extra unknown fields should not cause failure.
	tool := &FileWriteTool{}
	dir := t.TempDir()
	input := fmt.Sprintf(`{"file_path":"%s/x.txt","content":"y","extra":"z"}`, dir)
	result := tool.Validate(context.Background(), json.RawMessage(input))
	if result != nil {
		t.Errorf("extra fields should not fail, got: %s", result.Content)
	}
}

func TestValidate_NestedDirectory_NewFile(t *testing.T) {
	// Writing to deeply nested non-existent directory — Validate should pass (dirs created in Execute).
	dir := t.TempDir()
	deep := filepath.Join(dir, "a", "b", "c", "file.txt")
	tool := &FileWriteTool{ReadCache: readcache.New(16)}
	result := tool.Validate(context.Background(), writeInput(deep, "deep"))
	if result != nil {
		t.Errorf("non-existent nested path should pass validate, got: %s", result.Content)
	}
}

// ---------------------------------------------------------------------------
// Validatable interface conformance
// ---------------------------------------------------------------------------

func TestFileWriteTool_ImplementsValidatable(t *testing.T) {
	var _ Validatable = (*FileWriteTool)(nil)
}

func TestValidate_WorktreeCwd_UsesMappedPathForStaleness(t *testing.T) {
	// Simulates a worktree agent: the file exists in both main repo and worktree.
	// The agent read it (cache populated with the worktree path).
	// Validate must consult the worktree path, not the main-repo path.
	mainRoot, worktreeRoot, relPath := setupWorktreeScenario(t)

	rc := readcache.New(16)
	wtPath := filepath.Join(worktreeRoot, relPath)
	// Populate cache with the worktree-remapped path using the real mtime
	// (as Read would have done via os.Stat).
	cacheFile(t, rc, wtPath)

	tool := &FileWriteTool{ReadCache: rc}
	mainPath := filepath.Join(mainRoot, relPath)

	ctx := WithCwd(context.Background(), worktreeRoot)
	ctx = WithMainRoot(ctx, mainRoot)

	// Validate with the main-repo path (as an agent would send it).
	// It should PASS because the cache has the worktree-mapped version.
	result := tool.Validate(ctx, writeInput(mainPath, "NEW CONTENT\n"))
	if result != nil {
		t.Errorf("expected nil (file was read via worktree path), got: %s", result.Content)
	}
}

func TestValidate_WorktreeCwd_NoContext_StillRejectsUnread(t *testing.T) {
	// Without worktree context, an existing unread file must still fail.
	path := tmpFile(t, "existing content")
	rc := readcache.New(16)
	tool := &FileWriteTool{ReadCache: rc}

	result := tool.Validate(context.Background(), writeInput(path, "overwrite"))
	if result == nil || !result.IsError {
		t.Fatal("expected staleness error for unread file without worktree context")
	}
}

// ---------------------------------------------------------------------------
// Execute tests — ensure Execute still works correctly after refactoring
// ---------------------------------------------------------------------------

func TestExecute_NewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.txt")
	tool := &FileWriteTool{}

	result, err := tool.Execute(context.Background(), writeInput(path, "hello\nworld"))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "2 lines") {
		t.Errorf("expected 2 lines, got: %s", result.Content)
	}

	got, _ := os.ReadFile(path)
	if string(got) != "hello\nworld" {
		t.Errorf("file content = %q, want %q", got, "hello\nworld")
	}
}

func TestExecute_OverwriteExistingFile(t *testing.T) {
	path := tmpFile(t, "old")
	rc := readcache.New(16)
	cacheFile(t, rc, path)
	tool := &FileWriteTool{ReadCache: rc}

	result, err := tool.Execute(context.Background(), writeInput(path, "new"))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	got, _ := os.ReadFile(path)
	if string(got) != "new" {
		t.Errorf("file content = %q, want %q", got, "new")
	}
}

func TestExecute_CreatesParentDirectories(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "deep", "file.txt")
	tool := &FileWriteTool{}

	result, err := tool.Execute(context.Background(), writeInput(path, "deep"))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	got, _ := os.ReadFile(path)
	if string(got) != "deep" {
		t.Errorf("file content = %q, want %q", got, "deep")
	}
}

func TestExecute_InvalidJSON(t *testing.T) {
	tool := &FileWriteTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{bad`))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestExecute_EmptyFilePath(t *testing.T) {
	tool := &FileWriteTool{}
	result, err := tool.Execute(context.Background(), writeInput("", "x"))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content, "No file path") {
		t.Errorf("expected No file path error, got: %s", result.Content)
	}
}

func TestExecute_SecurityDeny(t *testing.T) {
	tool := &FileWriteTool{
		Security: &mockSecurity{denySubstring: "secret"},
	}
	result, err := tool.Execute(context.Background(), writeInput("/tmp/secret/file", "x"))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content, "Access denied") {
		t.Errorf("expected Access denied, got: %s", result.Content)
	}
}

func TestExecute_EmptyContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")
	tool := &FileWriteTool{}

	result, err := tool.Execute(context.Background(), writeInput(path, ""))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "0 lines") {
		t.Errorf("expected 0 lines, got: %s", result.Content)
	}
}

// ---------------------------------------------------------------------------
// RequiresApproval tests
// ---------------------------------------------------------------------------

func TestRequiresApproval_NormalFile(t *testing.T) {
	tool := &FileWriteTool{}
	if !tool.RequiresApproval(writeInput("/tmp/normal.txt", "x")) {
		t.Error("normal files should require approval")
	}
}

func TestRequiresApproval_PlanFile(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}
	planPath := filepath.Join(home, ".claudio", "plans", "my-plan.md")
	tool := &FileWriteTool{}
	if tool.RequiresApproval(writeInput(planPath, "x")) {
		t.Error("plan files should not require approval")
	}
}

func TestRequiresApproval_InvalidJSON(t *testing.T) {
	tool := &FileWriteTool{}
	// If JSON is bad, isPlanFilePath won't match → requires approval (safe default).
	if !tool.RequiresApproval(json.RawMessage(`{bad`)) {
		t.Error("invalid JSON should default to requiring approval")
	}
}

// ---------------------------------------------------------------------------
// isPlanFilePath edge cases
// ---------------------------------------------------------------------------

func TestIsPlanFilePath_ExactDir(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot get home dir")
	}
	// File inside plans dir
	if !isPlanFilePath(filepath.Join(home, ".claudio", "plans", "a.md")) {
		t.Error("expected true for file in plans dir")
	}
	// Nested subdirectory
	if !isPlanFilePath(filepath.Join(home, ".claudio", "plans", "sub", "b.md")) {
		t.Error("expected true for file in plans subdir")
	}
	// Outside plans
	if isPlanFilePath(filepath.Join(home, ".claudio", "config.json")) {
		t.Error("expected false for file outside plans dir")
	}
	// Path with similar prefix but not plans
	if isPlanFilePath(filepath.Join(home, ".claudio", "plans-backup", "a.md")) {
		t.Error("expected false for plans-backup")
	}
}

// ---------------------------------------------------------------------------
// countLines tests
// ---------------------------------------------------------------------------

func TestCountLines(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"a", 1},
		{"a\n", 2},
		{"a\nb", 2},
		{"a\nb\nc", 3},
		{"\n", 2},
		{"\n\n", 3},
	}
	for _, tc := range tests {
		got := countLines(tc.input)
		if got != tc.want {
			t.Errorf("countLines(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}
