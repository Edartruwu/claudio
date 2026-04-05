package plugins

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Abraxas-365/claudio/internal/config"
)

func TestLoadLspConfigs_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	result := LoadLspConfigs(dir)

	if len(result) != 0 {
		t.Errorf("expected empty result, got %v", result)
	}
}

func TestLoadLspConfigs_NonexistentDir(t *testing.T) {
	result := LoadLspConfigs("/nonexistent/path/that/does/not/exist")

	if len(result) != 0 {
		t.Errorf("expected empty result for nonexistent dir, got %v", result)
	}
}

func TestLoadLspConfigs_SingleFile(t *testing.T) {
	dir := t.TempDir()

	servers := map[string]config.LspServerConfig{
		"gopls": {
			Command:    "gopls",
			Args:       []string{"serve"},
			Extensions: []string{".go", ".mod"},
		},
	}

	data, _ := json.Marshal(servers)
	os.WriteFile(filepath.Join(dir, "go.lsp.json"), data, 0644)

	result := LoadLspConfigs(dir)

	if len(result) != 1 {
		t.Fatalf("expected 1 server, got %d", len(result))
	}
	if result["gopls"].Command != "gopls" {
		t.Errorf("expected command 'gopls', got %q", result["gopls"].Command)
	}
	if len(result["gopls"].Extensions) != 2 {
		t.Errorf("expected 2 extensions, got %d", len(result["gopls"].Extensions))
	}
}

func TestLoadLspConfigs_MultipleFiles(t *testing.T) {
	dir := t.TempDir()

	goServers := map[string]config.LspServerConfig{
		"gopls": {
			Command:    "gopls",
			Args:       []string{"serve"},
			Extensions: []string{".go"},
		},
	}
	tsServers := map[string]config.LspServerConfig{
		"tsserver": {
			Command:    "typescript-language-server",
			Args:       []string{"--stdio"},
			Extensions: []string{".ts", ".tsx"},
		},
	}

	goData, _ := json.Marshal(goServers)
	tsData, _ := json.Marshal(tsServers)

	os.WriteFile(filepath.Join(dir, "go.lsp.json"), goData, 0644)
	os.WriteFile(filepath.Join(dir, "typescript.lsp.json"), tsData, 0644)

	result := LoadLspConfigs(dir)

	if len(result) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(result))
	}
	if result["gopls"].Command != "gopls" {
		t.Errorf("expected gopls command")
	}
	if result["tsserver"].Command != "typescript-language-server" {
		t.Errorf("expected typescript-language-server command")
	}
}

func TestLoadLspConfigs_IgnoresNonLspJson(t *testing.T) {
	dir := t.TempDir()

	// Write a regular .json file (not *.lsp.json)
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"key":"value"}`), 0644)

	// Write a .txt file
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hello"), 0644)

	// Write an actual lsp.json
	servers := map[string]config.LspServerConfig{
		"gopls": {Command: "gopls", Extensions: []string{".go"}},
	}
	data, _ := json.Marshal(servers)
	os.WriteFile(filepath.Join(dir, "go.lsp.json"), data, 0644)

	result := LoadLspConfigs(dir)

	if len(result) != 1 {
		t.Fatalf("expected 1 server (only from .lsp.json), got %d", len(result))
	}
}

func TestLoadLspConfigs_IgnoresDirectories(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "something.lsp.json"), 0755) // directory, not file

	result := LoadLspConfigs(dir)
	if len(result) != 0 {
		t.Errorf("expected empty result when .lsp.json is a directory, got %v", result)
	}
}

func TestLoadLspConfigs_SkipsInvalidJson(t *testing.T) {
	dir := t.TempDir()

	// Write invalid JSON
	os.WriteFile(filepath.Join(dir, "bad.lsp.json"), []byte(`not valid json{{{`), 0644)

	// Write valid JSON
	servers := map[string]config.LspServerConfig{
		"gopls": {Command: "gopls", Extensions: []string{".go"}},
	}
	data, _ := json.Marshal(servers)
	os.WriteFile(filepath.Join(dir, "go.lsp.json"), data, 0644)

	result := LoadLspConfigs(dir)

	// Should skip the bad file and load the good one
	if len(result) != 1 {
		t.Fatalf("expected 1 server (bad file skipped), got %d", len(result))
	}
}

func TestLoadLspConfigs_MultipleServersInOneFile(t *testing.T) {
	dir := t.TempDir()

	servers := map[string]config.LspServerConfig{
		"gopls": {
			Command:    "gopls",
			Args:       []string{"serve"},
			Extensions: []string{".go"},
		},
		"rust-analyzer": {
			Command:    "rust-analyzer",
			Extensions: []string{".rs"},
		},
	}

	data, _ := json.Marshal(servers)
	os.WriteFile(filepath.Join(dir, "multi.lsp.json"), data, 0644)

	result := LoadLspConfigs(dir)

	if len(result) != 2 {
		t.Fatalf("expected 2 servers from single file, got %d", len(result))
	}
}

func TestLoadLspConfigs_LaterFileOverridesEarlier(t *testing.T) {
	dir := t.TempDir()

	// Two files with the same server name — last one read wins
	// (file system ordering may vary, but both should be loaded without error)
	servers1 := map[string]config.LspServerConfig{
		"gopls": {Command: "gopls-v1", Extensions: []string{".go"}},
	}
	servers2 := map[string]config.LspServerConfig{
		"gopls": {Command: "gopls-v2", Extensions: []string{".go"}},
	}

	data1, _ := json.Marshal(servers1)
	data2, _ := json.Marshal(servers2)

	os.WriteFile(filepath.Join(dir, "a.lsp.json"), data1, 0644)
	os.WriteFile(filepath.Join(dir, "b.lsp.json"), data2, 0644)

	result := LoadLspConfigs(dir)

	if len(result) != 1 {
		t.Fatalf("expected 1 server (same key), got %d", len(result))
	}
	// One of the two should win
	cmd := result["gopls"].Command
	if cmd != "gopls-v1" && cmd != "gopls-v2" {
		t.Errorf("expected gopls-v1 or gopls-v2, got %q", cmd)
	}
}

func TestLoadLspConfigs_WithEnv(t *testing.T) {
	dir := t.TempDir()

	servers := map[string]config.LspServerConfig{
		"gopls": {
			Command:    "gopls",
			Extensions: []string{".go"},
			Env:        map[string]string{"GOFLAGS": "-mod=vendor"},
		},
	}

	data, _ := json.Marshal(servers)
	os.WriteFile(filepath.Join(dir, "go.lsp.json"), data, 0644)

	result := LoadLspConfigs(dir)

	if result["gopls"].Env["GOFLAGS"] != "-mod=vendor" {
		t.Errorf("expected GOFLAGS env, got %v", result["gopls"].Env)
	}
}

func TestLoadLspConfigs_UnreadableFile(t *testing.T) {
	dir := t.TempDir()

	path := filepath.Join(dir, "noperm.lsp.json")
	os.WriteFile(path, []byte(`{"gopls":{"command":"gopls","extensions":[".go"]}}`), 0644)
	os.Chmod(path, 0000) // make unreadable
	defer os.Chmod(path, 0644)

	result := LoadLspConfigs(dir)

	// Should gracefully skip unreadable files
	if len(result) != 0 {
		t.Errorf("expected empty result for unreadable file, got %v", result)
	}
}
