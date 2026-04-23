package harness

import (
	"os"
	"path/filepath"
	"testing"
)

// setupHarness creates a minimal harness directory inside root with the given name+version.
func setupHarness(t *testing.T, root, name, version string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	writeManifest(t, dir, Manifest{Name: name, Version: version})
	return dir
}

func TestDiscoverHarnesses_Empty(t *testing.T) {
	dir := t.TempDir()
	harnesses, err := DiscoverHarnesses(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(harnesses) != 0 {
		t.Errorf("want 0, got %d", len(harnesses))
	}
}

func TestDiscoverHarnesses_NonExistentDir(t *testing.T) {
	harnesses, err := DiscoverHarnesses("/nonexistent/path/xyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(harnesses) != 0 {
		t.Errorf("want 0, got %d", len(harnesses))
	}
}

func TestDiscoverHarnesses_SortedByName(t *testing.T) {
	root := t.TempDir()
	setupHarness(t, root, "zebra-tools", "1.0.0")
	setupHarness(t, root, "alpha-pack", "1.0.0")
	setupHarness(t, root, "mid-tools", "1.0.0")

	harnesses, err := DiscoverHarnesses(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(harnesses) != 3 {
		t.Fatalf("want 3, got %d", len(harnesses))
	}
	names := []string{harnesses[0].Manifest.Name, harnesses[1].Manifest.Name, harnesses[2].Manifest.Name}
	want := []string{"alpha-pack", "mid-tools", "zebra-tools"}
	for i, n := range want {
		if names[i] != n {
			t.Errorf("index %d: got %q, want %q", i, names[i], n)
		}
	}
}

func TestDiscoverHarnesses_SkipNoManifest(t *testing.T) {
	root := t.TempDir()
	// valid harness
	setupHarness(t, root, "valid", "1.0.0")
	// dir without harness.json
	if err := os.MkdirAll(filepath.Join(root, "no-manifest"), 0755); err != nil {
		t.Fatal(err)
	}

	harnesses, err := DiscoverHarnesses(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(harnesses) != 1 {
		t.Errorf("want 1 harness, got %d", len(harnesses))
	}
}

func TestCollectAgentDirs(t *testing.T) {
	root := t.TempDir()
	d1 := setupHarness(t, root, "h1", "1.0.0")
	d2 := setupHarness(t, root, "h2", "1.0.0")

	harnesses := []*Harness{
		{Manifest: &Manifest{Name: "h1", Version: "1"}, Dir: d1},
		{Manifest: &Manifest{Name: "h2", Version: "1"}, Dir: d2},
	}

	dirs := CollectAgentDirs(harnesses)
	if len(dirs) != 2 {
		t.Errorf("want 2 dirs, got %d: %v", len(dirs), dirs)
	}
}

func TestCollectMCPServers_NoCollision(t *testing.T) {
	harnesses := []*Harness{
		{
			Manifest: &Manifest{
				Name:    "h1",
				Version: "1",
				MCPServers: map[string]MCPServerConfig{
					"server-a": {Command: "cmd-a"},
				},
			},
		},
		{
			Manifest: &Manifest{
				Name:    "h2",
				Version: "1",
				MCPServers: map[string]MCPServerConfig{
					"server-b": {Command: "cmd-b"},
				},
			},
		},
	}

	servers, err := CollectMCPServers(harnesses)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(servers) != 2 {
		t.Errorf("want 2 servers, got %d", len(servers))
	}
}

func TestCollectMCPServers_Collision(t *testing.T) {
	harnesses := []*Harness{
		{
			Manifest: &Manifest{
				Name:    "h1",
				Version: "1",
				MCPServers: map[string]MCPServerConfig{
					"same-name": {Command: "cmd-1"},
				},
			},
		},
		{
			Manifest: &Manifest{
				Name:    "h2",
				Version: "1",
				MCPServers: map[string]MCPServerConfig{
					"same-name": {Command: "cmd-2"},
				},
			},
		},
	}

	_, err := CollectMCPServers(harnesses)
	if err == nil {
		t.Fatal("expected collision error")
	}
}

func TestCollectToolFilters_LastWins(t *testing.T) {
	harnesses := []*Harness{
		{
			Manifest: &Manifest{
				Name:    "h1",
				Version: "1",
				AgentToolFilters: map[string]AgentToolFilter{
					"general-purpose": {AllowedMCPTools: []string{"tool-a"}},
				},
			},
		},
		{
			Manifest: &Manifest{
				Name:    "h2",
				Version: "1",
				AgentToolFilters: map[string]AgentToolFilter{
					"general-purpose": {AllowedMCPTools: []string{"tool-b"}},
				},
			},
		},
	}

	filters := CollectToolFilters(harnesses)
	f, ok := filters["general-purpose"]
	if !ok {
		t.Fatal("missing general-purpose filter")
	}
	if len(f.AllowedMCPTools) != 1 || f.AllowedMCPTools[0] != "tool-b" {
		t.Errorf("last wins: got %v", f.AllowedMCPTools)
	}
}
