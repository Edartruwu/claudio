package lsp

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/Abraxas-365/claudio/internal/config"
)

func TestNewServerManager_Empty(t *testing.T) {
	m := NewServerManager(nil)
	if m.HasServers() {
		t.Error("expected HasServers() false for nil config")
	}
	if m.HasConnected() {
		t.Error("expected HasConnected() false when no servers running")
	}

	m2 := NewServerManager(map[string]config.LspServerConfig{})
	if m2.HasServers() {
		t.Error("expected HasServers() false for empty config")
	}
}

func TestNewServerManager_WithConfigs(t *testing.T) {
	cfgs := map[string]config.LspServerConfig{
		"gopls": {
			Command:    "gopls",
			Args:       []string{"serve"},
			Extensions: []string{".go", ".mod"},
		},
		"tsserver": {
			Command:    "typescript-language-server",
			Args:       []string{"--stdio"},
			Extensions: []string{".ts", ".tsx", ".js", ".jsx"},
		},
	}

	m := NewServerManager(cfgs)

	if !m.HasServers() {
		t.Error("expected HasServers() true")
	}
	if m.HasConnected() {
		t.Error("expected HasConnected() false before any server started")
	}
}

func TestServerForFile_ExtensionRouting(t *testing.T) {
	cfgs := map[string]config.LspServerConfig{
		"gopls": {
			Command:    "gopls",
			Args:       []string{"serve"},
			Extensions: []string{".go", ".mod"},
		},
		"tsserver": {
			Command:    "typescript-language-server",
			Args:       []string{"--stdio"},
			Extensions: []string{".ts", ".tsx", ".js"},
		},
	}

	m := NewServerManager(cfgs)

	tests := []struct {
		file     string
		expected string
	}{
		{"main.go", "gopls"},
		{"go.mod", "gopls"},
		{"src/app.ts", "tsserver"},
		{"src/app.tsx", "tsserver"},
		{"src/index.js", "tsserver"},
		{"README.md", ""},       // no server
		{"style.css", ""},       // no server
		{"Makefile", ""},        // no server (no extension)
	}

	for _, tt := range tests {
		got := m.ServerForFile(tt.file)
		if got != tt.expected {
			t.Errorf("ServerForFile(%q) = %q, want %q", tt.file, got, tt.expected)
		}
	}
}

func TestServerForFile_CaseInsensitive(t *testing.T) {
	cfgs := map[string]config.LspServerConfig{
		"gopls": {
			Command:    "gopls",
			Extensions: []string{".Go"}, // uppercase in config
		},
	}

	m := NewServerManager(cfgs)

	// Should match regardless of case in the config
	if got := m.ServerForFile("main.go"); got != "gopls" {
		t.Errorf("expected gopls for main.go, got %q", got)
	}
}

func TestServerForFile_NoDotPrefix(t *testing.T) {
	// Extensions specified without the dot prefix should still work
	cfgs := map[string]config.LspServerConfig{
		"gopls": {
			Command:    "gopls",
			Extensions: []string{"go", "mod"},
		},
	}

	m := NewServerManager(cfgs)

	if got := m.ServerForFile("main.go"); got != "gopls" {
		t.Errorf("expected gopls for main.go, got %q", got)
	}
	if got := m.ServerForFile("go.mod"); got != "gopls" {
		t.Errorf("expected gopls for go.mod, got %q", got)
	}
}

func TestStartServer_CommandNotFound(t *testing.T) {
	cfgs := map[string]config.LspServerConfig{
		"fake": {
			Command:    "definitely-not-a-real-lsp-binary-xyz",
			Extensions: []string{".fake"},
		},
	}

	m := NewServerManager(cfgs)
	err := m.StartServer(context.Background(), "fake", t.TempDir())

	if err == nil {
		t.Fatal("expected error for missing command")
	}
	if !strings.Contains(err.Error(), "not found on PATH") {
		t.Errorf("expected 'not found on PATH' error, got: %v", err)
	}
}

func TestStartServer_UnknownName(t *testing.T) {
	m := NewServerManager(nil)
	err := m.StartServer(context.Background(), "nonexistent", t.TempDir())

	if err == nil {
		t.Fatal("expected error for unknown server name")
	}
	if !strings.Contains(err.Error(), "no LSP config") {
		t.Errorf("expected 'no LSP config' error, got: %v", err)
	}
}

func TestGetServer_NoServerForFile(t *testing.T) {
	cfgs := map[string]config.LspServerConfig{
		"gopls": {
			Command:    "gopls",
			Extensions: []string{".go"},
		},
	}

	m := NewServerManager(cfgs)
	_, err := m.GetServer(context.Background(), "/some/file.py")

	if err == nil {
		t.Fatal("expected error for unconfigured extension")
	}
	if !strings.Contains(err.Error(), "no LSP server configured") {
		t.Errorf("expected 'no LSP server configured' error, got: %v", err)
	}
}

func TestStatus_NoServers(t *testing.T) {
	m := NewServerManager(nil)
	status := m.Status()

	if len(status) != 0 {
		t.Errorf("expected empty status, got %v", status)
	}
}

func TestStatus_ConfiguredNotRunning(t *testing.T) {
	cfgs := map[string]config.LspServerConfig{
		"gopls": {
			Command:    "gopls",
			Extensions: []string{".go"},
		},
	}

	m := NewServerManager(cfgs)
	status := m.Status()

	if len(status) != 1 {
		t.Fatalf("expected 1 status entry, got %d", len(status))
	}
	if !strings.Contains(status["gopls"], "not running") {
		t.Errorf("expected 'not running' status, got %q", status["gopls"])
	}
}

func TestStopServer_NotRunning(t *testing.T) {
	m := NewServerManager(nil)
	// Should not error when stopping a server that isn't running
	if err := m.StopServer("nonexistent"); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestStopAll_Empty(t *testing.T) {
	m := NewServerManager(nil)
	// Should not panic or error
	m.StopAll()
}

func TestCleanIdle_Empty(t *testing.T) {
	m := NewServerManager(nil)
	// Should not panic or error
	m.CleanIdle()
}

func TestFromLspServerConfig(t *testing.T) {
	cfg := config.LspServerConfig{
		Command:    "gopls",
		Args:       []string{"serve", "-rpc.trace"},
		Extensions: []string{".go"},
		Env:        map[string]string{"GOFLAGS": "-mod=vendor"},
	}

	sc := FromLspServerConfig("gopls", cfg)

	if sc.Name != "gopls" {
		t.Errorf("expected name 'gopls', got %q", sc.Name)
	}
	if sc.Command != "gopls" {
		t.Errorf("expected command 'gopls', got %q", sc.Command)
	}
	if len(sc.Args) != 2 || sc.Args[0] != "serve" {
		t.Errorf("expected args [serve -rpc.trace], got %v", sc.Args)
	}
	if len(sc.Extensions) != 1 || sc.Extensions[0] != ".go" {
		t.Errorf("expected extensions [.go], got %v", sc.Extensions)
	}
	if sc.Env["GOFLAGS"] != "-mod=vendor" {
		t.Errorf("expected GOFLAGS env, got %v", sc.Env)
	}
}

func TestFindProjectRoot(t *testing.T) {
	// Create a temp dir structure with a go.mod marker
	root := t.TempDir()
	subdir := root + "/pkg/internal"
	if err := mkdirAll(subdir); err != nil {
		t.Fatal(err)
	}

	// Write a go.mod in root
	writeFile(t, root+"/go.mod", "module test")

	got := findProjectRoot(subdir)
	if got != root {
		t.Errorf("expected root %q, got %q", root, got)
	}
}

func TestFindProjectRoot_NoMarker(t *testing.T) {
	// When no marker exists, should return filesystem root (or the dir itself at some point)
	dir := t.TempDir() + "/deep/nested/path"
	if err := mkdirAll(dir); err != nil {
		t.Fatal(err)
	}

	got := findProjectRoot(dir)
	// Should not panic and should return some directory
	if got == "" {
		t.Error("expected non-empty result")
	}
}

func TestExtensionOverlap_LastConfigWins(t *testing.T) {
	// When two servers claim the same extension, the last one in iteration wins.
	// Since map iteration is non-deterministic, this tests that at least one is chosen.
	cfgs := map[string]config.LspServerConfig{
		"server-a": {
			Command:    "a",
			Extensions: []string{".js"},
		},
		"server-b": {
			Command:    "b",
			Extensions: []string{".js"},
		},
	}

	m := NewServerManager(cfgs)
	got := m.ServerForFile("app.js")

	if got != "server-a" && got != "server-b" {
		t.Errorf("expected one of server-a or server-b, got %q", got)
	}
}

// helpers

func mkdirAll(path string) error {
	return os.MkdirAll(path, 0755)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
