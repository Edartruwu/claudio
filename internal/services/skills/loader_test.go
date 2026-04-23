package skills

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// bundledSkills — new design skills
// ---------------------------------------------------------------------------

func TestBundledSkills_ContainsDesignSystem(t *testing.T) {
	skills := bundledSkills()
	for _, s := range skills {
		if s.Name == "design-system" {
			return
		}
	}
	t.Error("bundledSkills() should contain skill named 'design-system'")
}

func TestBundledSkills_ContainsMockup(t *testing.T) {
	skills := bundledSkills()
	for _, s := range skills {
		if s.Name == "mockup" {
			return
		}
	}
	t.Error("bundledSkills() should contain skill named 'mockup'")
}

func TestBundledSkills_ContainsHandoff(t *testing.T) {
	skills := bundledSkills()
	for _, s := range skills {
		if s.Name == "handoff" {
			return
		}
	}
	t.Error("bundledSkills() should contain skill named 'handoff'")
}

func TestBundledSkills_DesignSystemContentNonEmpty(t *testing.T) {
	skills := bundledSkills()
	for _, s := range skills {
		if s.Name == "design-system" {
			if s.Content == "" {
				t.Error("design-system skill Content should not be empty")
			}
			return
		}
	}
	t.Error("design-system skill not found")
}

func TestBundledSkills_MockupContentNonEmpty(t *testing.T) {
	skills := bundledSkills()
	for _, s := range skills {
		if s.Name == "mockup" {
			if s.Content == "" {
				t.Error("mockup skill Content should not be empty")
			}
			return
		}
	}
	t.Error("mockup skill not found")
}

func TestBundledSkills_HandoffContentNonEmpty(t *testing.T) {
	skills := bundledSkills()
	for _, s := range skills {
		if s.Name == "handoff" {
			if s.Content == "" {
				t.Error("handoff skill Content should not be empty")
			}
			return
		}
	}
	t.Error("handoff skill not found")
}

func TestBundledSkills_DesignSystemSource(t *testing.T) {
	skills := bundledSkills()
	for _, s := range skills {
		if s.Name == "design-system" {
			if s.Source != "bundled" {
				t.Errorf("expected Source=%q, got %q", "bundled", s.Source)
			}
			return
		}
	}
}

func TestBundledSkills_MockupSource(t *testing.T) {
	skills := bundledSkills()
	for _, s := range skills {
		if s.Name == "mockup" {
			if s.Source != "bundled" {
				t.Errorf("expected Source=%q, got %q", "bundled", s.Source)
			}
			return
		}
	}
}

func TestBundledSkills_HandoffSource(t *testing.T) {
	skills := bundledSkills()
	for _, s := range skills {
		if s.Name == "handoff" {
			if s.Source != "bundled" {
				t.Errorf("expected Source=%q, got %q", "bundled", s.Source)
			}
			return
		}
	}
}

// ---------------------------------------------------------------------------
// LoadAll — design skills reachable via registry
// ---------------------------------------------------------------------------

func TestLoadAll_DesignSkillsInRegistry(t *testing.T) {
	r := LoadAll("", "")
	for _, name := range []string{"design-system", "mockup", "handoff"} {
		s, ok := r.Get(name)
		if !ok {
			t.Errorf("expected skill %q in registry", name)
			continue
		}
		if s.Content == "" {
			t.Errorf("skill %q should have non-empty Content", name)
		}
	}
}

// ---------------------------------------------------------------------------
// LoadAll — variadic extra dirs (harness skills)
// ---------------------------------------------------------------------------

func TestLoadAll_NoExtraDirs_LoadsUserAndProjectOnly(t *testing.T) {
	userDir := t.TempDir()
	projectDir := t.TempDir()

	// Write user skill
	writeTestSkillFile(t, userDir, "user-skill.md", "user-skill", "User skill")

	// Write project skill
	writeTestSkillFile(t, projectDir, "project-skill.md", "project-skill", "Project skill")

	r := LoadAll(userDir, projectDir)

	// Verify both loaded
	if s, ok := r.Get("user-skill"); !ok || s.Source != "user" {
		t.Errorf("expected user-skill with source=user, got ok=%v source=%q", ok, s.Source)
	}
	if s, ok := r.Get("project-skill"); !ok || s.Source != "project" {
		t.Errorf("expected project-skill with source=project, got ok=%v source=%q", ok, s.Source)
	}
}

func TestLoadAll_WithExtraDirs_LoadsHarnessSkills(t *testing.T) {
	userDir := t.TempDir()
	projectDir := t.TempDir()
	extraDir := t.TempDir()

	// Write skills in each dir
	writeTestSkillFile(t, userDir, "user-skill.md", "user-skill", "User skill")
	writeTestSkillFile(t, projectDir, "project-skill.md", "project-skill", "Project skill")
	writeTestSkillFile(t, extraDir, "harness-skill.md", "harness-skill", "Harness skill")

	r := LoadAll(userDir, projectDir, extraDir)

	// Verify all loaded with correct sources
	if s, ok := r.Get("user-skill"); !ok || s.Source != "user" {
		t.Errorf("user-skill: ok=%v source=%q", ok, s.Source)
	}
	if s, ok := r.Get("project-skill"); !ok || s.Source != "project" {
		t.Errorf("project-skill: ok=%v source=%q", ok, s.Source)
	}
	if s, ok := r.Get("harness-skill"); !ok || s.Source != "harness" {
		t.Errorf("harness-skill: ok=%v source=%q", ok, s.Source)
	}
}

func TestLoadAll_MultipleExtraDirs_AllLoaded(t *testing.T) {
	userDir := t.TempDir()
	projectDir := t.TempDir()
	extra1 := t.TempDir()
	extra2 := t.TempDir()

	writeTestSkillFile(t, extra1, "harness1.md", "harness1", "Harness 1")
	writeTestSkillFile(t, extra2, "harness2.md", "harness2", "Harness 2")

	r := LoadAll(userDir, projectDir, extra1, extra2)

	// Both extra dir skills loaded with source=harness
	if s, ok := r.Get("harness1"); !ok || s.Source != "harness" {
		t.Errorf("harness1: ok=%v source=%q", ok, s.Source)
	}
	if s, ok := r.Get("harness2"); !ok || s.Source != "harness" {
		t.Errorf("harness2: ok=%v source=%q", ok, s.Source)
	}
}

func TestLoadAll_DuplicateName_HarnessWinsOverProject(t *testing.T) {
	userDir := t.TempDir()
	projectDir := t.TempDir()
	extraDir := t.TempDir()

	// Same skill name in project and extra dir
	writeTestSkillFile(t, projectDir, "shared.md", "shared", "Project version")
	writeTestSkillFile(t, extraDir, "shared.md", "shared", "Harness version")

	r := LoadAll(userDir, projectDir, extraDir)

	// Harness wins (loaded after project, latest registration wins)
	s, ok := r.Get("shared")
	if !ok {
		t.Error("shared skill not found")
	}
	if s.Source != "harness" {
		t.Errorf("expected harness to win, got source=%q", s.Source)
	}
	if s.Content != "Harness version" {
		t.Errorf("expected harness version in content, got %q", s.Content)
	}
}

func TestLoadAll_NonexistentExtraDir_SkippedNoError(t *testing.T) {
	userDir := t.TempDir()
	projectDir := t.TempDir()
	nonexistent := "/tmp/this-dir-does-not-exist-" + t.Name()

	writeTestSkillFile(t, userDir, "user-skill.md", "user-skill", "User skill")

	// Should not panic or error on non-existent dir
	r := LoadAll(userDir, projectDir, nonexistent)

	if s, ok := r.Get("user-skill"); !ok || s.Source != "user" {
		t.Errorf("user-skill: ok=%v source=%q", ok, s.Source)
	}
}

func TestLoadAll_ExtraDirWithDirectorySkills_Loaded(t *testing.T) {
	userDir := t.TempDir()
	projectDir := t.TempDir()
	extraDir := t.TempDir()

	// Create directory-form skill (skill.md inside dir)
	harnessDirPath := filepath.Join(extraDir, "dir-skill")
	os.Mkdir(harnessDirPath, 0755)
	skillPath := filepath.Join(harnessDirPath, "skill.md")
	writeTestSkillFileAtPath(t, skillPath, "dir-skill", "Directory skill")

	r := LoadAll(userDir, projectDir, extraDir)

	if s, ok := r.Get("dir-skill"); !ok || s.Source != "harness" {
		t.Errorf("dir-skill: ok=%v source=%q", ok, s.Source)
	}
}

func TestLoadAll_ExtraDir_SkillPathSet(t *testing.T) {
	userDir := t.TempDir()
	projectDir := t.TempDir()
	extraDir := t.TempDir()

	writeTestSkillFile(t, extraDir, "test.md", "test", "Test skill")

	r := LoadAll(userDir, projectDir, extraDir)

	s, ok := r.Get("test")
	if !ok {
		t.Fatal("test skill not found")
	}
	expected := filepath.Join(extraDir, "test.md")
	if s.FilePath != expected {
		t.Errorf("expected FilePath=%q, got %q", expected, s.FilePath)
	}
}

func TestLoadAll_EmptyExtraDirs_Ignored(t *testing.T) {
	userDir := t.TempDir()
	projectDir := t.TempDir()

	writeTestSkillFile(t, userDir, "user.md", "user", "User skill")

	// Pass empty string in extra dirs — should be skipped
	r := LoadAll(userDir, projectDir, "")

	if s, ok := r.Get("user"); !ok || s.Source != "user" {
		t.Errorf("user: ok=%v source=%q", ok, s.Source)
	}
}

// Helper to write a simple skill file with YAML frontmatter.
func writeTestSkillFile(t *testing.T, dir, filename, name, description string) {
	path := filepath.Join(dir, filename)
	writeTestSkillFileAtPath(t, path, name, description)
}

func writeTestSkillFileAtPath(t *testing.T, path, name, description string) {
	content := `---
name: ` + name + `
description: ` + description + `
---
` + description
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write skill file %s: %v", path, err)
	}
}
