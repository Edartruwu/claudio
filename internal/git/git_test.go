package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// initTempRepo creates a temporary directory with an initialized git repository.
// It configures a local user identity so commits work without global config.
func initTempRepo(t *testing.T) *Repo {
	t.Helper()
	dir := t.TempDir()
	repo := NewRepo(dir)

	cmds := [][]string{
		{"init"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test User"},
	}
	for _, args := range cmds {
		if _, err := repo.RunContext(context.Background(), args...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	return repo
}

// commitFile creates a file in the repo and stages + commits it.
func commitFile(t *testing.T, repo *Repo, name, content, message string) {
	t.Helper()
	path := filepath.Join(repo.Dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := repo.Add(name); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if err := repo.Commit(message); err != nil {
		t.Fatalf("git commit: %v", err)
	}
}

// ---------------------------------------------------------------------------
// NewRepo / IsRepo
// ---------------------------------------------------------------------------

func TestNewRepo(t *testing.T) {
	repo := NewRepo("/some/dir")
	if repo == nil {
		t.Fatal("NewRepo returned nil")
	}
	if repo.Dir != "/some/dir" {
		t.Errorf("Dir = %q; want /some/dir", repo.Dir)
	}
}

func TestIsRepo_True(t *testing.T) {
	repo := initTempRepo(t)
	if !repo.IsRepo() {
		t.Error("IsRepo() = false for an initialised git directory")
	}
}

func TestIsRepo_False(t *testing.T) {
	dir := t.TempDir() // plain directory, not a git repo
	repo := NewRepo(dir)
	if repo.IsRepo() {
		t.Error("IsRepo() = true for a non-git directory")
	}
}

// ---------------------------------------------------------------------------
// Root
// ---------------------------------------------------------------------------

func TestRoot(t *testing.T) {
	repo := initTempRepo(t)
	root, err := repo.Root()
	if err != nil {
		t.Fatalf("Root() error: %v", err)
	}
	// TempDir may use symlinks on macOS – compare via EvalSymlinks.
	want, _ := filepath.EvalSymlinks(repo.Dir)
	got, _ := filepath.EvalSymlinks(root)
	if got != want {
		t.Errorf("Root() = %q; want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// CurrentBranch
// ---------------------------------------------------------------------------

func TestCurrentBranch_AfterInit(t *testing.T) {
	repo := initTempRepo(t)
	// Make an initial commit so HEAD points to a real branch.
	commitFile(t, repo, "README.md", "hello", "initial")

	branch, err := repo.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch() error: %v", err)
	}
	if branch == "" {
		t.Error("CurrentBranch() returned empty string")
	}
}

// ---------------------------------------------------------------------------
// Status / StatusPorcelain
// ---------------------------------------------------------------------------

func TestStatus_Clean(t *testing.T) {
	repo := initTempRepo(t)
	commitFile(t, repo, "file.txt", "data", "initial")

	status, err := repo.Status()
	if err != nil {
		t.Fatalf("Status() error: %v", err)
	}
	if status != "" {
		t.Errorf("Status() = %q; want empty for clean tree", status)
	}
}

func TestStatus_WithChanges(t *testing.T) {
	repo := initTempRepo(t)
	commitFile(t, repo, "file.txt", "original", "initial")

	// Modify the file without staging.
	if err := os.WriteFile(filepath.Join(repo.Dir, "file.txt"), []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}

	status, err := repo.Status()
	if err != nil {
		t.Fatalf("Status() error: %v", err)
	}
	if !strings.Contains(status, "file.txt") {
		t.Errorf("Status() = %q; expected file.txt to appear", status)
	}
}

func TestStatusPorcelain_NewFile(t *testing.T) {
	repo := initTempRepo(t)
	commitFile(t, repo, "a.txt", "aaa", "initial")

	// Create untracked file.
	if err := os.WriteFile(filepath.Join(repo.Dir, "b.txt"), []byte("bbb"), 0644); err != nil {
		t.Fatal(err)
	}

	out, err := repo.StatusPorcelain()
	if err != nil {
		t.Fatalf("StatusPorcelain() error: %v", err)
	}
	if !strings.Contains(out, "b.txt") {
		t.Errorf("StatusPorcelain() = %q; expected b.txt", out)
	}
}

// ---------------------------------------------------------------------------
// Diff / DiffStaged
// ---------------------------------------------------------------------------

func TestDiff_Empty(t *testing.T) {
	repo := initTempRepo(t)
	commitFile(t, repo, "f.txt", "hello", "initial")

	diff, err := repo.Diff()
	if err != nil {
		t.Fatalf("Diff() error: %v", err)
	}
	if diff != "" {
		t.Errorf("Diff() = %q; want empty for clean tree", diff)
	}
}

func TestDiff_WithUnstagedChange(t *testing.T) {
	repo := initTempRepo(t)
	commitFile(t, repo, "f.txt", "hello", "initial")

	if err := os.WriteFile(filepath.Join(repo.Dir, "f.txt"), []byte("world"), 0644); err != nil {
		t.Fatal(err)
	}

	diff, err := repo.Diff()
	if err != nil {
		t.Fatalf("Diff() error: %v", err)
	}
	if !strings.Contains(diff, "f.txt") {
		t.Errorf("Diff() = %q; expected f.txt", diff)
	}
}

func TestDiffStaged(t *testing.T) {
	repo := initTempRepo(t)
	commitFile(t, repo, "f.txt", "hello", "initial")

	if err := os.WriteFile(filepath.Join(repo.Dir, "f.txt"), []byte("world"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := repo.Add("f.txt"); err != nil {
		t.Fatal(err)
	}

	diff, err := repo.DiffStaged()
	if err != nil {
		t.Fatalf("DiffStaged() error: %v", err)
	}
	if !strings.Contains(diff, "f.txt") {
		t.Errorf("DiffStaged() = %q; expected f.txt", diff)
	}
}

// ---------------------------------------------------------------------------
// Log / LogDetailed
// ---------------------------------------------------------------------------

func TestLog(t *testing.T) {
	repo := initTempRepo(t)
	commitFile(t, repo, "a.txt", "a", "first commit")
	commitFile(t, repo, "b.txt", "b", "second commit")

	log, err := repo.Log(5)
	if err != nil {
		t.Fatalf("Log() error: %v", err)
	}
	if !strings.Contains(log, "first commit") && !strings.Contains(log, "second commit") {
		t.Errorf("Log() = %q; expected commit messages", log)
	}
}

func TestLog_LimitN(t *testing.T) {
	repo := initTempRepo(t)
	for i := 0; i < 5; i++ {
		commitFile(t, repo, filepath.Base(t.TempDir()), "x", "commit")
	}

	log, err := repo.Log(2)
	if err != nil {
		t.Fatalf("Log(2) error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(log), "\n")
	if len(lines) > 2 {
		t.Errorf("Log(2) returned %d lines; want ≤ 2", len(lines))
	}
}

func TestLogDetailed(t *testing.T) {
	repo := initTempRepo(t)
	commitFile(t, repo, "x.txt", "x", "detailed commit")

	log, err := repo.LogDetailed(1)
	if err != nil {
		t.Fatalf("LogDetailed() error: %v", err)
	}
	if !strings.Contains(log, "detailed commit") {
		t.Errorf("LogDetailed() = %q; expected commit subject", log)
	}
	// The format includes author name.
	if !strings.Contains(log, "Test User") {
		t.Errorf("LogDetailed() = %q; expected author name", log)
	}
}

// ---------------------------------------------------------------------------
// HasRemote / RemoteURL
// ---------------------------------------------------------------------------

func TestHasRemote_NoRemote(t *testing.T) {
	repo := initTempRepo(t)
	if repo.HasRemote() {
		t.Error("HasRemote() = true; expected false for repo with no remote")
	}
}

func TestRemoteURL_NoRemote(t *testing.T) {
	repo := initTempRepo(t)
	_, err := repo.RemoteURL()
	if err == nil {
		t.Error("RemoteURL() expected error for repo with no remote")
	}
}

// ---------------------------------------------------------------------------
// Add / Commit
// ---------------------------------------------------------------------------

func TestAdd_StagedFile(t *testing.T) {
	repo := initTempRepo(t)
	path := filepath.Join(repo.Dir, "new.txt")
	if err := os.WriteFile(path, []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := repo.Add("new.txt"); err != nil {
		t.Fatalf("Add() error: %v", err)
	}

	staged, err := repo.DiffStaged()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(staged, "new.txt") {
		t.Errorf("DiffStaged() = %q; expected new.txt after Add", staged)
	}
}

func TestCommit_CreatesCommit(t *testing.T) {
	repo := initTempRepo(t)
	commitFile(t, repo, "file.txt", "content", "my message")

	log, err := repo.Log(1)
	if err != nil {
		t.Fatalf("Log() error: %v", err)
	}
	if !strings.Contains(log, "my message") {
		t.Errorf("Log() = %q; expected commit message", log)
	}
}

// ---------------------------------------------------------------------------
// CreateBranch
// ---------------------------------------------------------------------------

func TestCreateBranch(t *testing.T) {
	repo := initTempRepo(t)
	commitFile(t, repo, "base.txt", "base", "initial")

	if err := repo.CreateBranch("feature-x"); err != nil {
		t.Fatalf("CreateBranch() error: %v", err)
	}

	branch, err := repo.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch() error: %v", err)
	}
	if branch != "feature-x" {
		t.Errorf("CurrentBranch() = %q; want feature-x", branch)
	}
}

// ---------------------------------------------------------------------------
// Stash / StashPop
// ---------------------------------------------------------------------------

func TestStashAndPop(t *testing.T) {
	repo := initTempRepo(t)
	commitFile(t, repo, "f.txt", "original", "initial")

	// Make an unstaged change.
	if err := os.WriteFile(filepath.Join(repo.Dir, "f.txt"), []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := repo.Stash("test stash"); err != nil {
		t.Fatalf("Stash() error: %v", err)
	}

	// After stash, working tree should be clean.
	diff, _ := repo.Diff()
	if diff != "" {
		t.Errorf("after Stash(), Diff() = %q; want empty", diff)
	}

	if err := repo.StashPop(); err != nil {
		t.Fatalf("StashPop() error: %v", err)
	}

	// After pop, change should be back.
	diff, _ = repo.Diff()
	if diff == "" {
		t.Error("after StashPop(), Diff() is empty; expected the change back")
	}
}

// ---------------------------------------------------------------------------
// DiffBranch
// ---------------------------------------------------------------------------

func TestDiffBranch(t *testing.T) {
	repo := initTempRepo(t)
	commitFile(t, repo, "base.txt", "base content", "initial")

	baseBranch, err := repo.CurrentBranch()
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.CreateBranch("feature"); err != nil {
		t.Fatal(err)
	}
	commitFile(t, repo, "feature.txt", "feature content", "feature commit")

	diff, err := repo.DiffBranch(baseBranch)
	if err != nil {
		t.Fatalf("DiffBranch() error: %v", err)
	}
	if !strings.Contains(diff, "feature.txt") {
		t.Errorf("DiffBranch() = %q; expected feature.txt", diff)
	}
}

// ---------------------------------------------------------------------------
// RepoInfo
// ---------------------------------------------------------------------------

func TestRepoInfo_ContainsBranch(t *testing.T) {
	repo := initTempRepo(t)
	commitFile(t, repo, "f.txt", "data", "initial")

	info := repo.RepoInfo()
	if !strings.Contains(info, "Branch:") {
		t.Errorf("RepoInfo() = %q; expected Branch:", info)
	}
}

func TestRepoInfo_NotRepo(t *testing.T) {
	dir := t.TempDir()
	repo := NewRepo(dir)

	info := repo.RepoInfo()
	if !strings.Contains(info, "Not a git repository") {
		t.Errorf("RepoInfo() = %q; expected 'Not a git repository'", info)
	}
}

func TestRepoInfo_WithChanges(t *testing.T) {
	repo := initTempRepo(t)
	commitFile(t, repo, "f.txt", "original", "initial")

	if err := os.WriteFile(filepath.Join(repo.Dir, "f.txt"), []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}

	info := repo.RepoInfo()
	if !strings.Contains(info, "Changes:") {
		t.Errorf("RepoInfo() = %q; expected Changes:", info)
	}
}

func TestRepoInfo_CleanTree(t *testing.T) {
	repo := initTempRepo(t)
	commitFile(t, repo, "f.txt", "data", "initial")

	info := repo.RepoInfo()
	if !strings.Contains(info, "Working tree clean") {
		t.Errorf("RepoInfo() = %q; expected 'Working tree clean'", info)
	}
}

// ---------------------------------------------------------------------------
// Blame
// ---------------------------------------------------------------------------

func TestBlame(t *testing.T) {
	repo := initTempRepo(t)
	commitFile(t, repo, "blame.txt", "line one\nline two\n", "blame commit")

	out, err := repo.Blame("blame.txt")
	if err != nil {
		t.Fatalf("Blame() error: %v", err)
	}
	if !strings.Contains(out, "blame.txt") {
		t.Errorf("Blame() = %q; expected filename", out)
	}
}

// ---------------------------------------------------------------------------
// RunContext
// ---------------------------------------------------------------------------

func TestRunContext_CancelledContext(t *testing.T) {
	repo := initTempRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := repo.RunContext(ctx, "status")
	if err == nil {
		t.Error("RunContext() with cancelled context expected error, got nil")
	}
}

func TestRunContext_InvalidCommand(t *testing.T) {
	repo := initTempRepo(t)
	_, err := repo.RunContext(context.Background(), "not-a-real-git-command")
	if err == nil {
		t.Error("RunContext() with invalid command expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// WorktreeAdd / WorktreeRemove
// ---------------------------------------------------------------------------

func TestWorktreeAddRemove(t *testing.T) {
	repo := initTempRepo(t)
	commitFile(t, repo, "main.txt", "main", "initial")

	wtPath := filepath.Join(t.TempDir(), "wt1")
	if err := repo.WorktreeAdd(wtPath, "wt-branch"); err != nil {
		t.Fatalf("WorktreeAdd() error: %v", err)
	}

	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		t.Error("WorktreeAdd() did not create worktree directory")
	}

	if err := repo.WorktreeRemove(wtPath, true); err != nil {
		t.Fatalf("WorktreeRemove() error: %v", err)
	}
}
