package difftracker

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// ---------- helpers ----------

// initGitRepo creates a temporary git repository, sets minimal user config so
// git commit works, and returns the directory path.
func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("command %v: %v\n%s", args, err, out)
		}
	}

	run("git", "init")
	run("git", "config", "user.email", "test@example.com")
	run("git", "config", "user.name", "Tester")

	// Create an initial commit so HEAD exists.
	initialFile := filepath.Join(dir, "README.md")
	if err := os.WriteFile(initialFile, []byte("init\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("git", "add", ".")
	run("git", "commit", "-m", "initial commit")

	return dir
}

// writeAndStage creates/overwrites a file in dir, stages it with git add, and
// leaves it as an unstaged diff (writes the file but does NOT stage it unless
// stage==true), making `git diff` return output.
func modifyFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// stageFile runs `git add <name>` in dir.
func stageFile(t *testing.T, dir, name string) {
	t.Helper()
	cmd := exec.Command("git", "add", name)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add %s: %v\n%s", name, err, out)
	}
}

// commitFile stages and commits a single file.
func commitFile(t *testing.T, dir, name, content, message string) {
	t.Helper()
	modifyFile(t, dir, name, content)
	stageFile(t, dir, name)
	cmd := exec.Command("git", "commit", "-m", message)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
}

// cdToDir changes the process working directory for the duration of the test.
// (The difftracker calls exec.Command("git", ...) with no Dir set, so it uses
// the CWD at the time of the call.)
func cdToDir(t *testing.T, dir string) {
	t.Helper()
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(original) })
}

// ---------- New ----------

func TestNew_InitialState(t *testing.T) {
	tr := New()
	if tr == nil {
		t.Fatal("New() returned nil")
	}
	if tr.Count() != 0 {
		t.Errorf("expected 0 diffs, got %d", tr.Count())
	}
	if tr.turnNum != 0 {
		t.Errorf("expected turnNum 0, got %d", tr.turnNum)
	}
}

// ---------- BeforeTurn ----------

func TestBeforeTurn_IncrementsTurnNum(t *testing.T) {
	dir := initGitRepo(t)
	cdToDir(t, dir)

	tr := New()
	tr.BeforeTurn()
	if tr.turnNum != 1 {
		t.Errorf("expected turnNum 1 after first BeforeTurn, got %d", tr.turnNum)
	}
	tr.BeforeTurn()
	if tr.turnNum != 2 {
		t.Errorf("expected turnNum 2 after second BeforeTurn, got %d", tr.turnNum)
	}
}

func TestBeforeTurn_CapturesBaseline(t *testing.T) {
	dir := initGitRepo(t)
	cdToDir(t, dir)

	tr := New()

	// With a clean repo, baseline should be empty string.
	tr.BeforeTurn()
	if tr.baseline != "" {
		t.Errorf("expected empty baseline on clean repo, got %q", tr.baseline)
	}
}

func TestBeforeTurn_BaselineWithUnstagedChanges(t *testing.T) {
	dir := initGitRepo(t)
	cdToDir(t, dir)

	// Modify a tracked file (not staged) so `git diff` returns output.
	modifyFile(t, dir, "README.md", "modified content\n")

	tr := New()
	tr.BeforeTurn()

	if tr.baseline == "" {
		t.Error("expected non-empty baseline when tracked file is modified")
	}
}

// ---------- AfterTurn ----------

func TestAfterTurn_NoChanges_ReturnsNil(t *testing.T) {
	dir := initGitRepo(t)
	cdToDir(t, dir)

	tr := New()
	tr.BeforeTurn()
	diff := tr.AfterTurn()

	if diff != nil {
		t.Errorf("expected nil diff on clean repo, got %+v", diff)
	}
}

func TestAfterTurn_WithNewUnstagedChange_ReturnsDiff(t *testing.T) {
	dir := initGitRepo(t)
	cdToDir(t, dir)

	tr := New()
	tr.BeforeTurn()

	// Introduce a change AFTER BeforeTurn (simulates tool execution).
	modifyFile(t, dir, "README.md", "new content added by tool\n")

	diff := tr.AfterTurn()
	if diff == nil {
		t.Fatal("expected non-nil diff after file modification")
	}
	if diff.Turn != 1 {
		t.Errorf("expected Turn 1, got %d", diff.Turn)
	}
	if diff.Patch == "" {
		t.Error("expected non-empty Patch")
	}
}

func TestAfterTurn_RecordsTurnNumber(t *testing.T) {
	dir := initGitRepo(t)
	cdToDir(t, dir)

	tr := New()

	// Turn 1: no changes.
	tr.BeforeTurn()
	tr.AfterTurn()

	// Turn 2: make a change.
	tr.BeforeTurn()
	modifyFile(t, dir, "README.md", "turn 2 change\n")
	diff := tr.AfterTurn()

	if diff == nil {
		t.Fatal("expected diff on turn 2")
	}
	if diff.Turn != 2 {
		t.Errorf("expected Turn 2, got %d", diff.Turn)
	}
}

func TestAfterTurn_RecordsModifiedFiles(t *testing.T) {
	dir := initGitRepo(t)
	cdToDir(t, dir)

	tr := New()
	tr.BeforeTurn()

	modifyFile(t, dir, "README.md", "changed\n")

	diff := tr.AfterTurn()
	if diff == nil {
		t.Fatal("expected non-nil diff")
	}
	if len(diff.FilesModified) == 0 {
		t.Error("expected at least one file in FilesModified")
	}

	found := false
	for _, f := range diff.FilesModified {
		if strings.Contains(f, "README.md") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("README.md not in FilesModified: %v", diff.FilesModified)
	}
}

func TestAfterTurn_BaselineEqualsAfter_ReturnsNil(t *testing.T) {
	dir := initGitRepo(t)
	cdToDir(t, dir)

	// Create an unstaged change BEFORE BeforeTurn, so baseline == after.
	modifyFile(t, dir, "README.md", "pre-existing change\n")

	tr := New()
	tr.BeforeTurn()

	// Don't make any additional changes; `git diff` output is same as baseline.
	diff := tr.AfterTurn()
	if diff != nil {
		t.Errorf("expected nil diff when nothing changed during turn, got %+v", diff)
	}
}

// ---------- GetTurn ----------

func TestGetTurn_ReturnsCorrectDiff(t *testing.T) {
	dir := initGitRepo(t)
	cdToDir(t, dir)

	tr := New()
	tr.BeforeTurn()
	modifyFile(t, dir, "README.md", "turn1\n")
	tr.AfterTurn()

	d := tr.GetTurn(1)
	if d == nil {
		t.Fatal("GetTurn(1) returned nil")
	}
	if d.Turn != 1 {
		t.Errorf("expected Turn 1, got %d", d.Turn)
	}
}

func TestGetTurn_MissingTurnReturnsNil(t *testing.T) {
	tr := New()
	if d := tr.GetTurn(99); d != nil {
		t.Errorf("expected nil for non-existent turn, got %+v", d)
	}
}

func TestGetTurn_AfterMultipleTurns(t *testing.T) {
	dir := initGitRepo(t)
	cdToDir(t, dir)

	tr := New()

	// Turn 1 – change README.
	tr.BeforeTurn()
	modifyFile(t, dir, "README.md", "turn1\n")
	tr.AfterTurn()

	// Commit so the file is clean again.
	stageFile(t, dir, "README.md")
	cmd := exec.Command("git", "commit", "-m", "turn1 commit")
	cmd.Dir = dir
	_ = cmd.Run()

	// Turn 2 – change README again.
	tr.BeforeTurn()
	modifyFile(t, dir, "README.md", "turn2\n")
	tr.AfterTurn()

	d1 := tr.GetTurn(1)
	d2 := tr.GetTurn(2)

	if d1 == nil {
		t.Fatal("GetTurn(1) returned nil")
	}
	if d2 == nil {
		t.Fatal("GetTurn(2) returned nil")
	}
	if d1.Turn != 1 {
		t.Errorf("d1.Turn: expected 1, got %d", d1.Turn)
	}
	if d2.Turn != 2 {
		t.Errorf("d2.Turn: expected 2, got %d", d2.Turn)
	}
}

// ---------- All ----------

func TestAll_EmptyInitially(t *testing.T) {
	tr := New()
	if diffs := tr.All(); len(diffs) != 0 {
		t.Errorf("expected empty slice, got %d elements", len(diffs))
	}
}

func TestAll_ReturnsCopy(t *testing.T) {
	dir := initGitRepo(t)
	cdToDir(t, dir)

	tr := New()
	tr.BeforeTurn()
	modifyFile(t, dir, "README.md", "change\n")
	tr.AfterTurn()

	all1 := tr.All()
	all2 := tr.All()

	if len(all1) != len(all2) {
		t.Errorf("All() returned different lengths: %d vs %d", len(all1), len(all2))
	}

	// Mutating the returned slice must not affect the internal state.
	if len(all1) > 0 {
		all1[0].Turn = 999
		internal := tr.All()
		if internal[0].Turn == 999 {
			t.Error("All() returned a reference to internal slice, not a copy")
		}
	}
}

// ---------- Count ----------

func TestCount_InitiallyZero(t *testing.T) {
	tr := New()
	if tr.Count() != 0 {
		t.Errorf("expected 0, got %d", tr.Count())
	}
}

func TestCount_IncreasesWithDiffs(t *testing.T) {
	dir := initGitRepo(t)
	cdToDir(t, dir)

	tr := New()
	tr.BeforeTurn()
	modifyFile(t, dir, "README.md", "change\n")
	tr.AfterTurn()

	if tr.Count() != 1 {
		t.Errorf("expected Count 1, got %d", tr.Count())
	}
}

func TestCount_NoChangeDoesNotIncrement(t *testing.T) {
	dir := initGitRepo(t)
	cdToDir(t, dir)

	tr := New()
	tr.BeforeTurn()
	tr.AfterTurn() // no changes

	if tr.Count() != 0 {
		t.Errorf("expected Count 0 when no changes, got %d", tr.Count())
	}
}

// ---------- parseModifiedFiles ----------

func TestParseModifiedFiles_EmptyString(t *testing.T) {
	files := parseModifiedFiles("")
	if files != nil {
		t.Errorf("expected nil for empty input, got %v", files)
	}
}

func TestParseModifiedFiles_SingleFile(t *testing.T) {
	stat := " README.md | 2 +-\n 1 file changed, 1 insertion(+), 1 deletion(-)"
	files := parseModifiedFiles(stat)
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d: %v", len(files), files)
	}
	if files[0] != "README.md" {
		t.Errorf("expected 'README.md', got %q", files[0])
	}
}

func TestParseModifiedFiles_MultipleFiles(t *testing.T) {
	stat := " foo/bar.go  | 5 ++---\n baz/qux.go  | 3 +--\n 2 files changed, 3 insertions(+), 5 deletions(-)"
	files := parseModifiedFiles(stat)
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(files), files)
	}
	if files[0] != "foo/bar.go" {
		t.Errorf("expected 'foo/bar.go', got %q", files[0])
	}
	if files[1] != "baz/qux.go" {
		t.Errorf("expected 'baz/qux.go', got %q", files[1])
	}
}

func TestParseModifiedFiles_SkipsSummaryLine(t *testing.T) {
	stat := " a.txt | 1 +\n 1 file changed, 1 insertion(+)"
	files := parseModifiedFiles(stat)
	for _, f := range files {
		if strings.Contains(f, "changed") {
			t.Errorf("summary line should be excluded, got %q", f)
		}
	}
}

// ---------- Concurrency ----------

func TestConcurrent_BeforeAfterTurn(t *testing.T) {
	dir := initGitRepo(t)
	cdToDir(t, dir)

	tr := New()
	var wg sync.WaitGroup

	// Run BeforeTurn / AfterTurn pairs sequentially but from multiple goroutines
	// overlapping the read-only AfterTurn parts.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tr.BeforeTurn()
			tr.AfterTurn()
		}(i)
	}
	wg.Wait()
}

func TestConcurrent_AllAndCount(t *testing.T) {
	dir := initGitRepo(t)
	cdToDir(t, dir)

	tr := New()
	tr.BeforeTurn()
	modifyFile(t, dir, "README.md", "c\n")
	tr.AfterTurn()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = tr.All()
		}()
		go func() {
			defer wg.Done()
			_ = tr.Count()
		}()
	}
	wg.Wait()
}
