package harness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeManifest(t *testing.T, dir string, m Manifest) {
	t.Helper()
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), data, 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

func TestLoadManifest_Valid(t *testing.T) {
	dir := t.TempDir()
	want := Manifest{Name: "my-harness", Version: "1.0.0", Description: "test"}
	writeManifest(t, dir, want)

	got, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != want.Name {
		t.Errorf("name: got %q, want %q", got.Name, want.Name)
	}
	if got.Version != want.Version {
		t.Errorf("version: got %q, want %q", got.Version, want.Version)
	}
}

func TestLoadManifest_Missing(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadManifest(dir)
	if err == nil {
		t.Fatal("expected error for missing harness.json")
	}
}

func TestLoadManifest_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadManifest(dir)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestManifest_Validate_OK(t *testing.T) {
	m := &Manifest{Name: "my-harness", Version: "1.0.0"}
	if err := m.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestManifest_Validate_MissingName(t *testing.T) {
	m := &Manifest{Version: "1.0.0"}
	if err := m.Validate(); err == nil {
		t.Error("expected error for missing name")
	}
}

func TestManifest_Validate_MissingVersion(t *testing.T) {
	m := &Manifest{Name: "my-harness"}
	if err := m.Validate(); err == nil {
		t.Error("expected error for missing version")
	}
}

func TestManifest_Validate_InvalidNameFormat(t *testing.T) {
	cases := []string{"My Harness", "my_harness", "MyHarness", "-bad", "bad-"}
	for _, name := range cases {
		m := &Manifest{Name: name, Version: "1.0.0"}
		if err := m.Validate(); err == nil {
			t.Errorf("expected error for name %q", name)
		}
	}
}

func TestManifest_Validate_ValidNameFormats(t *testing.T) {
	cases := []string{"mh", "my-harness", "my-harness-v2", "claudio-pentesting"}
	for _, name := range cases {
		m := &Manifest{Name: name, Version: "1.0.0"}
		if err := m.Validate(); err != nil {
			t.Errorf("unexpected error for name %q: %v", name, err)
		}
	}
}

func TestManifest_DirResolvers_Defaults(t *testing.T) {
	m := &Manifest{Name: "h", Version: "1"}
	base := "/project/.claudio/harnesses/h"

	check := func(name string, got []string, wantSuffix string) {
		if len(got) != 1 {
			t.Errorf("%s: want 1 dir, got %d", name, len(got))
			return
		}
		if got[0] != filepath.Join(base, wantSuffix) {
			t.Errorf("%s: got %q, want %q", name, got[0], filepath.Join(base, wantSuffix))
		}
	}

	check("AgentDirs", m.AgentDirs(base), "agents")
	check("SkillDirs", m.SkillDirs(base), "skills")
	check("PluginDirs", m.PluginDirs(base), "plugins")
	check("TemplateDirs", m.TemplateDirs(base), "team-templates")
	check("RulePaths", m.RulePaths(base), "rules")
}

func TestManifest_DirResolvers_Custom(t *testing.T) {
	m := &Manifest{
		Name:      "h",
		Version:   "1",
		Agents:    []string{"custom/agents", "extra/agents"},
		Skills:    []string{"my-skills"},
	}
	base := "/base"

	agentDirs := m.AgentDirs(base)
	if len(agentDirs) != 2 {
		t.Fatalf("AgentDirs: want 2, got %d", len(agentDirs))
	}
	if agentDirs[0] != "/base/custom/agents" {
		t.Errorf("AgentDirs[0]: got %q", agentDirs[0])
	}

	skillDirs := m.SkillDirs(base)
	if len(skillDirs) != 1 || skillDirs[0] != "/base/my-skills" {
		t.Errorf("SkillDirs: got %v", skillDirs)
	}
}
