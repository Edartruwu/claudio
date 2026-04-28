package lua

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadProjectInit_MissingFile — no error when .claudio/init.lua doesn't exist.
func TestLoadProjectInit_MissingFile(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	dir := t.TempDir() // empty dir, no .claudio/init.lua
	if err := rt.LoadProjectInit(dir); err != nil {
		t.Fatalf("expected nil for missing project init, got: %v", err)
	}
}

// TestLoadProjectInit_ExecutesFile — creates a temp dir with .claudio/init.lua,
// verifies the file is executed (registers a tool).
func TestLoadProjectInit_ExecutesFile(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	dir := t.TempDir()
	claudioDir := filepath.Join(dir, ".claudio")
	if err := os.MkdirAll(claudioDir, 0755); err != nil {
		t.Fatal(err)
	}

	lua := `
claudio.register_tool({
  name        = "project_init_tool",
  description = "registered from project init",
  execute     = function(input) return "ok" end,
})
`
	if err := os.WriteFile(filepath.Join(claudioDir, "init.lua"), []byte(lua), 0644); err != nil {
		t.Fatal(err)
	}

	if err := rt.LoadProjectInit(dir); err != nil {
		t.Fatalf("LoadProjectInit: %v", err)
	}

	if _, err := rt.toolReg.Get("project_init_tool"); err != nil {
		t.Errorf("expected project_init_tool to be registered: %v", err)
	}
}

// TestLoadAll_ScansPluginsDir — verifies LoadAll uses the given plugins/ path.
func TestLoadAll_ScansPluginsDir(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	// Create a plugins/ dir (not lua-plugins/) with a plugin inside.
	pluginsDir := t.TempDir()
	pluginDir := filepath.Join(pluginsDir, "myplugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}

	lua := `
claudio.register_tool({
  name        = "plugins_dir_tool",
  description = "loaded from plugins dir",
  execute     = function(input) return "ok" end,
})
`
	if err := os.WriteFile(filepath.Join(pluginDir, "init.lua"), []byte(lua), 0644); err != nil {
		t.Fatal(err)
	}

	if err := rt.LoadAll(pluginsDir); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	if _, err := rt.toolReg.Get("plugins_dir_tool"); err != nil {
		t.Errorf("expected plugins_dir_tool to be registered: %v", err)
	}
}
