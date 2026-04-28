package commands_test

import (
	"strings"
	"testing"

	"github.com/Abraxas-365/claudio/internal/cli/commands"
	"github.com/Abraxas-365/claudio/internal/config"
)

func makeColorschemeRegistry(deps *commands.CommandDeps) *commands.Registry {
	r := commands.NewRegistry()
	commands.RegisterCoreCommands(r, deps)
	return r
}

func TestColorscheme_Gruvbox(t *testing.T) {
	var capturedColors map[string]string
	deps := &commands.CommandDeps{
		SetTheme: func(colors map[string]string) {
			capturedColors = colors
		},
	}
	r := makeColorschemeRegistry(deps)
	cmd, ok := r.Get("colorscheme")
	if !ok {
		t.Fatal("colorscheme command not registered")
	}
	out, err := cmd.Execute("gruvbox")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "gruvbox") {
		t.Errorf("expected output to mention gruvbox, got %q", out)
	}
	if capturedColors == nil {
		t.Fatal("SetTheme was not called")
	}
	// Gruvbox primary is a known color.
	if capturedColors["primary"] != "#d3869b" {
		t.Errorf("gruvbox primary = %q, want #d3869b", capturedColors["primary"])
	}
}

func TestColorscheme_Unknown(t *testing.T) {
	deps := &commands.CommandDeps{
		SetTheme: func(colors map[string]string) {},
	}
	r := makeColorschemeRegistry(deps)
	cmd, ok := r.Get("colorscheme")
	if !ok {
		t.Fatal("colorscheme command not registered")
	}
	_, err := cmd.Execute("bogus-theme-xyz")
	if err == nil {
		t.Fatal("expected error for unknown theme, got nil")
	}
	if !strings.Contains(err.Error(), "bogus-theme-xyz") {
		t.Errorf("error should mention theme name, got: %v", err)
	}
}

func TestColorscheme_NoArgs(t *testing.T) {
	deps := &commands.CommandDeps{
		SetTheme: func(colors map[string]string) {},
	}
	r := makeColorschemeRegistry(deps)
	cmd, ok := r.Get("colorscheme")
	if !ok {
		t.Fatal("colorscheme command not registered")
	}
	out, err := cmd.Execute("")
	if err != nil {
		t.Fatalf("unexpected error listing themes: %v", err)
	}
	// Should list available themes.
	if !strings.Contains(out, "gruvbox") {
		t.Errorf("listing output should mention gruvbox, got %q", out)
	}
	if !strings.Contains(out, "Available") {
		t.Errorf("listing output should say Available, got %q", out)
	}
}

func TestColorscheme_SavesPersistence(t *testing.T) {
	var savedCfg *config.Settings
	cfg := &config.Settings{}
	deps := &commands.CommandDeps{
		SetTheme:   func(colors map[string]string) {},
		GetConfig:  func() *config.Settings { return cfg },
		SaveConfig: func(s *config.Settings) error { savedCfg = s; return nil },
	}
	r := makeColorschemeRegistry(deps)
	cmd, ok := r.Get("colorscheme")
	if !ok {
		t.Fatal("colorscheme command not registered")
	}
	_, err := cmd.Execute("tokyonight")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if savedCfg == nil {
		t.Fatal("SaveConfig was not called")
	}
	if savedCfg.ColorScheme != "tokyonight" {
		t.Errorf("saved ColorScheme = %q, want tokyonight", savedCfg.ColorScheme)
	}
}
