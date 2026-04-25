package harness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// buildValidHarness creates a harness directory with all expected subdirs and files.
func buildValidHarness(t *testing.T) *Harness {
	t.Helper()
	dir := t.TempDir()

	// Write manifest.
	writeManifest(t, dir, Manifest{Name: "test-harness", Version: "1.0.0"})

	// Create default subdirs.
	for _, sub := range []string{"agents", "skills", "plugins", "team-templates", "rules"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Write a non-empty agent .md file.
	if err := os.WriteFile(filepath.Join(dir, "agents", "my-agent.md"), []byte("# agent"), 0644); err != nil {
		t.Fatal(err)
	}

	// Write a valid template JSON file.
	data, _ := json.Marshal(map[string]string{"name": "test"})
	if err := os.WriteFile(filepath.Join(dir, "team-templates", "team.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// Write an executable plugin (non-Windows).
	pluginPath := filepath.Join(dir, "plugins", "my-plugin")
	if err := os.WriteFile(pluginPath, []byte("#!/bin/sh\necho hi"), 0755); err != nil {
		t.Fatal(err)
	}

	m, err := LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	return &Harness{Manifest: m, Dir: dir}
}

func TestValidateHarness_Valid(t *testing.T) {
	h := buildValidHarness(t)
	errs := ValidateHarness(h)
	// Filter out executable warnings on non-linux (e.g. macOS might behave differently)
	for _, e := range errs {
		t.Logf("finding: %s", e)
	}
	// Should have zero errors (severity == "error").
	for _, e := range errs {
		if e.Severity == "error" {
			t.Errorf("unexpected error: %s", e)
		}
	}
}

func TestValidateHarness_MissingAgentsDir(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, Manifest{Name: "test-harness", Version: "1.0.0"})
	// No subdirs created.

	h := &Harness{
		Manifest: &Manifest{Name: "test-harness", Version: "1.0.0"},
		Dir:      dir,
	}
	errs := ValidateHarness(h)

	var warnings []ValidationError
	for _, e := range errs {
		if e.Severity == "warning" {
			warnings = append(warnings, e)
		}
	}
	if len(warnings) == 0 {
		t.Error("expected at least one warning for missing dirs")
	}
}

func TestValidateHarness_InvalidManifest(t *testing.T) {
	h := &Harness{
		Manifest: &Manifest{Name: "", Version: ""},
		Dir:      t.TempDir(),
	}
	errs := ValidateHarness(h)
	var hasError bool
	for _, e := range errs {
		if e.Severity == "error" {
			hasError = true
		}
	}
	if !hasError {
		t.Error("expected error for invalid manifest")
	}
}

func TestValidateHarness_InvalidTemplateJSON(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, Manifest{Name: "test-harness", Version: "1.0.0"})

	tmplDir := filepath.Join(dir, "team-templates")
	if err := os.MkdirAll(tmplDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmplDir, "bad.json"), []byte("not json {"), 0644); err != nil {
		t.Fatal(err)
	}

	h := &Harness{
		Manifest: &Manifest{Name: "test-harness", Version: "1.0.0"},
		Dir:      dir,
	}
	errs := ValidateHarness(h)

	var hasError bool
	for _, e := range errs {
		if e.Severity == "error" {
			hasError = true
		}
	}
	if !hasError {
		t.Error("expected error for invalid JSON template")
	}
}

func TestValidateHarness_EmptyAgentMD(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, Manifest{Name: "test-harness", Version: "1.0.0"})

	agentDir := filepath.Join(dir, "agents")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Empty .md file.
	if err := os.WriteFile(filepath.Join(agentDir, "empty.md"), []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	h := &Harness{
		Manifest: &Manifest{Name: "test-harness", Version: "1.0.0"},
		Dir:      dir,
	}
	errs := ValidateHarness(h)

	var hasWarning bool
	for _, e := range errs {
		if e.Severity == "warning" {
			hasWarning = true
		}
	}
	if !hasWarning {
		t.Error("expected warning for empty agent .md file")
	}
}

func TestValidateHarness_NonExecutablePlugin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executable check not run on Windows")
	}

	dir := t.TempDir()
	writeManifest(t, dir, Manifest{Name: "test-harness", Version: "1.0.0"})

	pluginDir := filepath.Join(dir, "plugins")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}
	// File without execute bit.
	if err := os.WriteFile(filepath.Join(pluginDir, "my-plugin"), []byte("#!/bin/sh"), 0644); err != nil {
		t.Fatal(err)
	}

	h := &Harness{
		Manifest: &Manifest{Name: "test-harness", Version: "1.0.0"},
		Dir:      dir,
	}
	errs := ValidateHarness(h)

	var hasWarning bool
	for _, e := range errs {
		if e.Severity == "warning" {
			hasWarning = true
		}
	}
	if !hasWarning {
		t.Error("expected warning for non-executable plugin")
	}
}
