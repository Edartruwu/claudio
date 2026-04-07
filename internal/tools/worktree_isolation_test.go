package tools

// This test file verifies that file tools (Read, Edit, Write) honor the
// worktree CWD override set by WithCwd. This is critical for team agents
// spawned with `isolation: "worktree"` — their file operations must land
// inside the worktree, not the main repository.
//
// As of the initial version of this file, these tests are EXPECTED TO FAIL.
// They document the bug: Read/Edit/Write ignore CwdFromContext and operate
// directly on absolute paths, breaking worktree isolation. Bash/Glob/Grep
// already honor the CWD override; these three tools do not.
//
// Scenario under test: an agent's conversation context refers to files at
// /main/repo/file.go, but the agent is running in a worktree at
// /main/repo/.claudio-worktrees/branch/. When the agent calls
// Edit("/main/repo/file.go", ...), the edit must land in the worktree copy,
// not the main repo.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Abraxas-365/claudio/internal/tools/readcache"
)

// setupWorktreeScenario builds a fake "main repo" and a fake "worktree" with
// the same file laid out at the same relative path. Returns (mainRoot,
// worktreeRoot, relPath). The file in the main repo contains "MAIN"; the file
// in the worktree contains "WORKTREE".
func setupWorktreeScenario(t *testing.T) (mainRoot, worktreeRoot, relPath string) {
	t.Helper()
	base := t.TempDir()
	mainRoot = filepath.Join(base, "main")
	worktreeRoot = filepath.Join(base, "wt")
	relPath = filepath.Join("internal", "foo.go")

	for _, root := range []string{mainRoot, worktreeRoot} {
		if err := os.MkdirAll(filepath.Join(root, "internal"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(mainRoot, relPath), []byte("MAIN\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktreeRoot, relPath), []byte("WORKTREE\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return mainRoot, worktreeRoot, relPath
}

// readFile reads a file and returns its contents or fails the test.
func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// -----------------------------------------------------------------------------
// Write
// -----------------------------------------------------------------------------

func TestWrite_WithWorktreeCwd_RemapsMainRepoPathIntoWorktree(t *testing.T) {
	mainRoot, worktreeRoot, relPath := setupWorktreeScenario(t)

	tool := &FileWriteTool{ReadCache: readcache.New(64)}
	// Simulate the agent seeing the main-repo absolute path in its conversation
	// context (from the parent agent) and calling Write with it.
	mainPath := filepath.Join(mainRoot, relPath)

	// Mark the main path as "read" so staleness check passes — the Read tool
	// would have populated this when the parent shared the file.
	tool.ReadCache.Put(readcache.Key{FilePath: mainPath, Offset: 1, Limit: 2000}, "MAIN\n", time.Now())
	tool.ReadCache.MarkWritten(mainPath, time.Now())

	ctx := WithCwd(context.Background(), worktreeRoot)
	ctx = WithMainRoot(ctx, mainRoot)
	raw, _ := json.Marshal(fileWriteInput{FilePath: mainPath, Content: "NEW WORKTREE CONTENT\n"})
	res, err := tool.Execute(ctx, raw)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %s", res.Content)
	}

	// Main repo file MUST be untouched.
	if got := readFile(t, filepath.Join(mainRoot, relPath)); got != "MAIN\n" {
		t.Errorf("main repo file was modified; worktree isolation broken. got=%q want=%q", got, "MAIN\n")
	}
	// Worktree file MUST reflect the write.
	if got := readFile(t, filepath.Join(worktreeRoot, relPath)); got != "NEW WORKTREE CONTENT\n" {
		t.Errorf("worktree file was not updated. got=%q want=%q", got, "NEW WORKTREE CONTENT\n")
	}
}

func TestWrite_WithoutCwd_WritesToAbsolutePath(t *testing.T) {
	// Regression: when no CWD override is set, Write should continue to
	// behave exactly as before (absolute path is used verbatim).
	dir := t.TempDir()
	target := filepath.Join(dir, "hello.txt")

	tool := &FileWriteTool{ReadCache: readcache.New(64)}
	raw, _ := json.Marshal(fileWriteInput{FilePath: target, Content: "hello"})
	res, err := tool.Execute(context.Background(), raw)
	if err != nil || res.IsError {
		t.Fatalf("execute: err=%v result=%s", err, res.Content)
	}
	if got := readFile(t, target); got != "hello" {
		t.Errorf("got %q want %q", got, "hello")
	}
}

// -----------------------------------------------------------------------------
// Edit
// -----------------------------------------------------------------------------

func TestEdit_WithWorktreeCwd_RemapsMainRepoPathIntoWorktree(t *testing.T) {
	mainRoot, worktreeRoot, relPath := setupWorktreeScenario(t)

	tool := &FileEditTool{ReadCache: readcache.New(64)}
	mainPath := filepath.Join(mainRoot, relPath)
	wtPath := filepath.Join(worktreeRoot, relPath)

	// The agent's Read tool, if remapped correctly, would have read the
	// worktree file. Seed the cache for both paths so staleness checks pass
	// regardless of which path the tool ends up using — this test is about
	// WHERE the write lands, not about cache bookkeeping.
	tool.ReadCache.Put(readcache.Key{FilePath: mainPath, Offset: 1, Limit: 2000}, "WORKTREE\n", time.Now())
	tool.ReadCache.Put(readcache.Key{FilePath: wtPath, Offset: 1, Limit: 2000}, "WORKTREE\n", time.Now())

	ctx := WithCwd(context.Background(), worktreeRoot)
	ctx = WithMainRoot(ctx, mainRoot)
	raw, _ := json.Marshal(fileEditInput{
		FilePath:  mainPath,
		OldString: "WORKTREE",
		NewString: "EDITED",
	})
	res, err := tool.Execute(ctx, raw)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %s", res.Content)
	}

	if got := readFile(t, filepath.Join(mainRoot, relPath)); got != "MAIN\n" {
		t.Errorf("main repo file was modified; worktree isolation broken. got=%q", got)
	}
	if got := readFile(t, filepath.Join(worktreeRoot, relPath)); got != "EDITED\n" {
		t.Errorf("worktree file not edited. got=%q want=%q", got, "EDITED\n")
	}
}

// -----------------------------------------------------------------------------
// Read
// -----------------------------------------------------------------------------

func TestRead_WithWorktreeCwd_RemapsMainRepoPathIntoWorktree(t *testing.T) {
	mainRoot, worktreeRoot, relPath := setupWorktreeScenario(t)

	tool := &FileReadTool{ReadCache: readcache.New(64)}
	mainPath := filepath.Join(mainRoot, relPath)

	ctx := WithCwd(context.Background(), worktreeRoot)
	ctx = WithMainRoot(ctx, mainRoot)
	raw, _ := json.Marshal(fileReadInput{FilePath: mainPath})
	res, err := tool.Execute(ctx, raw)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %s", res.Content)
	}

	// The agent expects to see the worktree's copy (the one it can safely
	// mutate). If Read serves the main-repo copy, the agent will think the
	// state of the worktree matches the main repo — and any subsequent
	// Edit based on that content will be wrong.
	if !contains(res.Content, "WORKTREE") {
		t.Errorf("Read returned main repo content instead of worktree content. content=%q", res.Content)
	}
	if contains(res.Content, "MAIN") {
		t.Errorf("Read leaked main repo content into worktree agent. content=%q", res.Content)
	}
}

// -----------------------------------------------------------------------------
// Path outside the main repo must NOT be remapped
// -----------------------------------------------------------------------------

func TestWrite_WithWorktreeCwd_DoesNotRemapUnrelatedAbsolutePath(t *testing.T) {
	// When an agent writes to a file that is NOT under the main-repo root
	// (e.g., a temp file, a file in $HOME), the CWD override must not
	// accidentally redirect the write into the worktree.
	mainRoot, worktreeRoot, _ := setupWorktreeScenario(t)

	tool := &FileWriteTool{ReadCache: readcache.New(64)}
	outside := filepath.Join(t.TempDir(), "unrelated.txt")
	ctx := WithCwd(context.Background(), worktreeRoot)
	ctx = WithMainRoot(ctx, mainRoot)

	raw, _ := json.Marshal(fileWriteInput{FilePath: outside, Content: "outside"})
	res, err := tool.Execute(ctx, raw)
	if err != nil || res.IsError {
		t.Fatalf("execute: err=%v result=%s", err, res.Content)
	}
	if got := readFile(t, outside); got != "outside" {
		t.Errorf("unrelated path was mis-remapped: got %q", got)
	}
}

// -----------------------------------------------------------------------------
// Relative paths — the most common case in practice. The agent's prompt
// usually instructs it in terms of repo-relative paths like
// "internal/tui/theme.go", which os.Open() would normally resolve against the
// process CWD (the main repo root). With worktree isolation active, those
// must instead be resolved against the worktree root, otherwise the agent
// silently writes to the main tree.
// -----------------------------------------------------------------------------

func TestWrite_WithWorktreeCwd_RemapsRelativePathIntoWorktree(t *testing.T) {
	mainRoot, worktreeRoot, relPath := setupWorktreeScenario(t)

	tool := &FileWriteTool{ReadCache: readcache.New(64)}
	wtAbs := filepath.Join(worktreeRoot, relPath)
	tool.ReadCache.Put(readcache.Key{FilePath: wtAbs, Offset: 1, Limit: 2000}, "WORKTREE\n", time.Now())
	tool.ReadCache.MarkWritten(wtAbs, time.Now())

	ctx := WithCwd(context.Background(), worktreeRoot)
	ctx = WithMainRoot(ctx, mainRoot)
	raw, _ := json.Marshal(fileWriteInput{FilePath: relPath, Content: "FROM RELATIVE\n"})
	res, err := tool.Execute(ctx, raw)
	if err != nil || res.IsError {
		t.Fatalf("execute: err=%v result=%s", err, res.Content)
	}

	if got := readFile(t, filepath.Join(mainRoot, relPath)); got != "MAIN\n" {
		t.Errorf("relative-path write leaked into main repo. got=%q", got)
	}
	if got := readFile(t, wtAbs); got != "FROM RELATIVE\n" {
		t.Errorf("worktree file not written via relative path. got=%q", got)
	}
}

func TestRead_WithWorktreeCwd_RemapsRelativePathIntoWorktree(t *testing.T) {
	mainRoot, worktreeRoot, relPath := setupWorktreeScenario(t)

	tool := &FileReadTool{ReadCache: readcache.New(64)}
	ctx := WithCwd(context.Background(), worktreeRoot)
	ctx = WithMainRoot(ctx, mainRoot)
	raw, _ := json.Marshal(fileReadInput{FilePath: relPath})
	res, err := tool.Execute(ctx, raw)
	if err != nil || res.IsError {
		t.Fatalf("execute: err=%v result=%s", err, res.Content)
	}
	if !contains(res.Content, "WORKTREE") || contains(res.Content, "MAIN") {
		t.Errorf("relative-path Read served main tree instead of worktree. content=%q", res.Content)
	}
}

func TestEdit_WithWorktreeCwd_RemapsRelativePathIntoWorktree(t *testing.T) {
	mainRoot, worktreeRoot, relPath := setupWorktreeScenario(t)

	tool := &FileEditTool{ReadCache: readcache.New(64)}
	wtAbs := filepath.Join(worktreeRoot, relPath)
	tool.ReadCache.Put(readcache.Key{FilePath: wtAbs, Offset: 1, Limit: 2000}, "WORKTREE\n", time.Now())

	ctx := WithCwd(context.Background(), worktreeRoot)
	ctx = WithMainRoot(ctx, mainRoot)
	raw, _ := json.Marshal(fileEditInput{
		FilePath:  relPath,
		OldString: "WORKTREE",
		NewString: "EDITED",
	})
	res, err := tool.Execute(ctx, raw)
	if err != nil || res.IsError {
		t.Fatalf("execute: err=%v result=%s", err, res.Content)
	}
	if got := readFile(t, filepath.Join(mainRoot, relPath)); got != "MAIN\n" {
		t.Errorf("relative-path Edit leaked into main repo. got=%q", got)
	}
	if got := readFile(t, wtAbs); got != "EDITED\n" {
		t.Errorf("worktree file not edited via relative path. got=%q", got)
	}
}

// contains is a tiny helper to avoid pulling in strings just for this.
func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
