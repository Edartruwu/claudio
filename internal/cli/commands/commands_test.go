package commands_test

import (
	"testing"

	"github.com/Abraxas-365/claudio/internal/cli/commands"
)

func TestParse(t *testing.T) {
	tests := []struct {
		input     string
		wantName  string
		wantArgs  string
		wantIsCmd bool
	}{
		{"/help", "help", "", true},
		{"/model claude-opus-4-6", "model", "claude-opus-4-6", true},
		{"/diff HEAD~3", "diff", "HEAD~3", true},
		{"hello world", "", "", false},
		{"", "", "", false},
		{"/", "", "", true},
	}

	for _, tt := range tests {
		name, args, isCmd := commands.Parse(tt.input)
		if name != tt.wantName || args != tt.wantArgs || isCmd != tt.wantIsCmd {
			t.Errorf("Parse(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tt.input, name, args, isCmd, tt.wantName, tt.wantArgs, tt.wantIsCmd)
		}
	}
}

func TestRegistry(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterCoreCommands(r)

	// Test /help exists
	cmd, ok := r.Get("help")
	if !ok {
		t.Fatal("expected /help to be registered")
	}
	output, err := cmd.Execute("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output == "" {
		t.Error("expected non-empty help output")
	}

	// Test aliases
	_, ok = r.Get("h")
	if !ok {
		t.Error("expected /h alias to work")
	}
	_, ok = r.Get("?")
	if !ok {
		t.Error("expected /? alias to work")
	}

	// Test unknown command
	_, ok = r.Get("nonexistent")
	if ok {
		t.Error("expected nonexistent command to not be found")
	}

	// Test /doctor
	cmd, ok = r.Get("doctor")
	if !ok {
		t.Fatal("expected /doctor to be registered")
	}
	output, err = cmd.Execute("")
	if err != nil {
		t.Fatalf("doctor error: %v", err)
	}
	if output == "" {
		t.Error("expected non-empty doctor output")
	}
}

func TestHelpText(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterCoreCommands(r)

	help := r.HelpText()
	if help == "" {
		t.Fatal("expected non-empty help text")
	}
	// Should contain key commands
	for _, cmd := range []string{"/help", "/model", "/commit", "/diff", "/doctor", "/exit"} {
		if !contains(help, cmd) {
			t.Errorf("help text missing %s", cmd)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
