package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// NewRegistry / Count / All
// ---------------------------------------------------------------------------

func TestNewRegistry_Empty(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry() returned nil")
	}
	if r.Count() != 0 {
		t.Errorf("Count() = %d; want 0", r.Count())
	}
	if got := r.All(); len(got) != 0 {
		t.Errorf("All() = %v; want empty slice", got)
	}
}

// ---------------------------------------------------------------------------
// ForSystemPrompt
// ---------------------------------------------------------------------------

func TestForSystemPrompt_Empty(t *testing.T) {
	r := NewRegistry()
	if got := r.ForSystemPrompt(); got != "" {
		t.Errorf("ForSystemPrompt() = %q; want empty for empty registry", got)
	}
}

func TestForSystemPrompt_WithRules(t *testing.T) {
	r := NewRegistry()
	r.rules = append(r.rules, &Rule{
		Name:    "my-rule",
		Content: "do something important",
		Source:  "project",
	})

	out := r.ForSystemPrompt()
	if !strings.Contains(out, "my-rule") {
		t.Errorf("ForSystemPrompt() missing rule name; got %q", out)
	}
	if !strings.Contains(out, "do something important") {
		t.Errorf("ForSystemPrompt() missing rule content; got %q", out)
	}
	if !strings.Contains(out, "project") {
		t.Errorf("ForSystemPrompt() missing rule source; got %q", out)
	}
	if !strings.Contains(out, "# Project Rules") {
		t.Errorf("ForSystemPrompt() missing header; got %q", out)
	}
}

func TestForSystemPrompt_MultipleRules(t *testing.T) {
	r := NewRegistry()
	r.rules = append(r.rules,
		&Rule{Name: "rule-a", Content: "content-a", Source: "user"},
		&Rule{Name: "rule-b", Content: "content-b", Source: "project"},
	)
	out := r.ForSystemPrompt()
	if !strings.Contains(out, "rule-a") || !strings.Contains(out, "rule-b") {
		t.Errorf("ForSystemPrompt() missing rules; got %q", out)
	}
	// rule-a should appear before rule-b
	if strings.Index(out, "rule-a") > strings.Index(out, "rule-b") {
		t.Error("ForSystemPrompt() rules not in order")
	}
}

// ---------------------------------------------------------------------------
// loadDir / LoadAll
// ---------------------------------------------------------------------------

func TestLoadAll_EmptyDirs(t *testing.T) {
	r := LoadAll("", "")
	if r.Count() != 0 {
		t.Errorf("LoadAll with empty dirs: Count() = %d; want 0", r.Count())
	}
}

func TestLoadAll_NonExistentDirs(t *testing.T) {
	r := LoadAll("/nonexistent/user/rules", "/nonexistent/project/rules")
	if r.Count() != 0 {
		t.Errorf("LoadAll with nonexistent dirs: Count() = %d; want 0", r.Count())
	}
}

func TestLoadAll_LoadsMarkdownFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "rule1.md"), []byte("# Rule One\nDo X"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rule2.md"), []byte("# Rule Two\nDo Y"), 0644); err != nil {
		t.Fatal(err)
	}
	// Non-markdown file should be ignored.
	if err := os.WriteFile(filepath.Join(dir, "ignore.txt"), []byte("ignored"), 0644); err != nil {
		t.Fatal(err)
	}

	r := LoadAll("", dir)
	if r.Count() != 2 {
		t.Errorf("Count() = %d; want 2", r.Count())
	}
}

func TestLoadAll_UserVsProject(t *testing.T) {
	userDir := t.TempDir()
	projDir := t.TempDir()

	os.WriteFile(filepath.Join(userDir, "shared.md"), []byte("user content"), 0644)
	os.WriteFile(filepath.Join(projDir, "shared.md"), []byte("project content"), 0644)

	r := LoadAll(userDir, projDir)
	// Both loaded; order: user first, then project.
	all := r.All()
	if len(all) != 2 {
		t.Fatalf("Count() = %d; want 2", len(all))
	}
	if all[0].Source != "user" {
		t.Errorf("first rule Source = %q; want user", all[0].Source)
	}
	if all[1].Source != "project" {
		t.Errorf("second rule Source = %q; want project", all[1].Source)
	}
}

func TestLoadAll_FrontmatterName(t *testing.T) {
	dir := t.TempDir()
	content := "---\nname: custom-name\n---\nSome rule content"
	if err := os.WriteFile(filepath.Join(dir, "rule.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	r := LoadAll("", dir)
	if r.Count() != 1 {
		t.Fatalf("Count() = %d; want 1", r.Count())
	}
	rule := r.All()[0]
	if rule.Name != "custom-name" {
		t.Errorf("Name = %q; want custom-name", rule.Name)
	}
	if !strings.Contains(rule.Content, "Some rule content") {
		t.Errorf("Content = %q; expected body text", rule.Content)
	}
}

func TestLoadAll_DefaultNameFromFilename(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "my-rule.md"), []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	r := LoadAll("", dir)
	if r.Count() != 1 {
		t.Fatalf("Count() = %d; want 1", r.Count())
	}
	if r.All()[0].Name != "my-rule" {
		t.Errorf("Name = %q; want my-rule", r.All()[0].Name)
	}
}

func TestLoadAll_RecursesSubdirectories(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "subgroup")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "top.md"), []byte("top"), 0644)
	os.WriteFile(filepath.Join(subDir, "sub.md"), []byte("sub"), 0644)

	r := LoadAll("", dir)
	if r.Count() != 2 {
		t.Errorf("Count() = %d; want 2 (top + sub)", r.Count())
	}
}

func TestLoadAll_PathSetCorrectly(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "myrule.md")
	if err := os.WriteFile(filePath, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	r := LoadAll("", dir)
	if r.Count() != 1 {
		t.Fatalf("Count() = %d; want 1", r.Count())
	}
	if r.All()[0].Path != filePath {
		t.Errorf("Path = %q; want %q", r.All()[0].Path, filePath)
	}
}

// ---------------------------------------------------------------------------
// LoadCLAUDEMD
// ---------------------------------------------------------------------------

func TestLoadCLAUDEMD_UserLevel(t *testing.T) {
	homeDir := t.TempDir()
	claudioDir := filepath.Join(homeDir, ".claudio")
	if err := os.MkdirAll(claudioDir, 0755); err != nil {
		t.Fatal(err)
	}
	claudeMD := filepath.Join(claudioDir, "CLAUDE.md")
	if err := os.WriteFile(claudeMD, []byte("user level instructions"), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewRegistry()
	// Use homeDir as both projectDir and homeDir so cwd walk is trivial.
	r.LoadCLAUDEMD(homeDir, homeDir)

	if r.Count() == 0 {
		t.Fatal("LoadCLAUDEMD() loaded nothing; expected user CLAUDE.md")
	}

	var found bool
	for _, rule := range r.All() {
		if strings.Contains(rule.Content, "user level instructions") {
			found = true
		}
	}
	if !found {
		t.Error("LoadCLAUDEMD() did not load user CLAUDE.md content")
	}
}

func TestLoadCLAUDEMD_ProjectLevel_CLAUDEMD(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "CLAUDE.md"), []byte("project instructions"), 0644); err != nil {
		t.Fatal(err)
	}

	// Chdir into the temp dir so that os.Getwd() inside LoadCLAUDEMD
	// returns a path under projectDir, preventing the cwd walk from
	// picking up real CLAUDIO.md / CLAUDE.md files from the repo tree.
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })

	r := NewRegistry()
	r.LoadCLAUDEMD(projectDir, "")

	if r.Count() == 0 {
		t.Fatal("LoadCLAUDEMD() loaded nothing; expected project CLAUDE.md")
	}
	if !strings.Contains(r.All()[0].Content, "project instructions") {
		t.Errorf("content = %q; expected project instructions", r.All()[0].Content)
	}
}

func TestLoadCLAUDEMD_ProjectLevel_CLAUDIOMD(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "CLAUDIO.md"), []byte("claudio instructions"), 0644); err != nil {
		t.Fatal(err)
	}

	// Chdir into the temp dir so that os.Getwd() inside LoadCLAUDEMD
	// returns a path under projectDir, preventing the cwd walk from
	// picking up real CLAUDIO.md / CLAUDE.md files from the repo tree.
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })

	r := NewRegistry()
	r.LoadCLAUDEMD(projectDir, "")

	if r.Count() == 0 {
		t.Fatal("LoadCLAUDEMD() loaded nothing; expected CLAUDIO.md")
	}
	if !strings.Contains(r.All()[0].Content, "claudio instructions") {
		t.Errorf("content = %q; expected claudio instructions", r.All()[0].Content)
	}
}

func TestLoadCLAUDEMD_EmptyDirs(t *testing.T) {
	r := NewRegistry()
	r.LoadCLAUDEMD("", "")
	// Should not panic and should load nothing (no user CLAUDE.md without homeDir).
	// The project walk still happens from cwd.
	// We just verify no panic and the registry is usable.
	_ = r.ForSystemPrompt()
}

// ---------------------------------------------------------------------------
// collectDirsRootToCwd
// ---------------------------------------------------------------------------

func TestCollectDirsRootToCwd_SameDir(t *testing.T) {
	dir := t.TempDir()
	dirs := collectDirsRootToCwd(dir, dir)
	if len(dirs) != 1 || dirs[0] != dir {
		t.Errorf("collectDirsRootToCwd(same, same) = %v; want [%s]", dirs, dir)
	}
}

func TestCollectDirsRootToCwd_ParentToChild(t *testing.T) {
	parent := t.TempDir()
	child := filepath.Join(parent, "child")
	grandchild := filepath.Join(child, "grandchild")
	if err := os.MkdirAll(grandchild, 0755); err != nil {
		t.Fatal(err)
	}

	dirs := collectDirsRootToCwd(parent, grandchild)
	if len(dirs) < 2 {
		t.Fatalf("collectDirsRootToCwd() = %v; want at least [parent, child, grandchild]", dirs)
	}
	// First entry should be the root.
	if filepath.Clean(dirs[0]) != filepath.Clean(parent) {
		t.Errorf("first dir = %q; want %q", dirs[0], parent)
	}
	// Last entry should be the leaf (cwd).
	if filepath.Clean(dirs[len(dirs)-1]) != filepath.Clean(grandchild) {
		t.Errorf("last dir = %q; want %q", dirs[len(dirs)-1], grandchild)
	}
}

func TestCollectDirsRootToCwd_RootNotAncestor(t *testing.T) {
	// When root is not an ancestor of cwd, the function should walk up until
	// it hits the filesystem root and return the collected directories.
	dir := t.TempDir()
	dirs := collectDirsRootToCwd("/totally/different/path", dir)
	// Should at least contain the cwd.
	if len(dirs) == 0 {
		t.Error("collectDirsRootToCwd() returned empty slice")
	}
}
