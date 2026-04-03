package permissions

import (
	"encoding/json"
	"testing"

	"github.com/Abraxas-365/claudio/internal/config"
)

// ---------------------------------------------------------------------------
// matchPattern
// ---------------------------------------------------------------------------

func TestMatchPattern_Wildcard(t *testing.T) {
	if !matchPattern("*", "anything at all") {
		t.Error("wildcard * should match any content")
	}
	if !matchPattern("*", "") {
		t.Error("wildcard * should match empty string")
	}
}

func TestMatchPattern_ExactMatch(t *testing.T) {
	if !matchPattern("go test ./...", "go test ./...") {
		t.Error("exact pattern should match identical content")
	}
	if matchPattern("go test ./...", "go build ./...") {
		t.Error("exact pattern should not match different content")
	}
}

func TestMatchPattern_PrefixWildcard(t *testing.T) {
	if !matchPattern("go *", "go test ./...") {
		t.Error("prefix wildcard 'go *' should match 'go test ./...'")
	}
	if !matchPattern("go *", "go build -o bin/app ./cmd/app") {
		t.Error("prefix wildcard 'go *' should match any go subcommand")
	}
	if matchPattern("go *", "make build") {
		t.Error("prefix wildcard 'go *' should not match different prefix")
	}
}

func TestMatchPattern_DomainPrefix(t *testing.T) {
	// WebFetch extracts "domain:hostname" (no path), so exact match works
	if !matchPattern("domain:example.com", "domain:example.com") {
		t.Error("exact domain pattern should match")
	}
	if matchPattern("domain:example.com", "domain:evil.com") {
		t.Error("domain pattern should not match different domain")
	}
}

func TestMatchPattern_CaseSensitive(t *testing.T) {
	// matchPattern uses filepath.Match which is case-sensitive
	if !matchPattern("go *", "go test") {
		t.Error("exact case should match")
	}
	if matchPattern("Go *", "go test") {
		t.Error("matchPattern is case-sensitive — different case should not match")
	}
}

// ---------------------------------------------------------------------------
// extractMatchContent
// ---------------------------------------------------------------------------

func TestExtractMatchContent_Bash(t *testing.T) {
	input, _ := json.Marshal(map[string]string{"command": "go test ./..."})
	got := extractMatchContent("Bash", input)
	if got != "go test ./..." {
		t.Errorf("extractMatchContent(Bash) = %q, want %q", got, "go test ./...")
	}
}

func TestExtractMatchContent_BashEmpty(t *testing.T) {
	input, _ := json.Marshal(map[string]string{"command": ""})
	got := extractMatchContent("Bash", input)
	if got != "" {
		t.Errorf("extractMatchContent(Bash with empty command) = %q, want empty", got)
	}
}

func TestExtractMatchContent_Read(t *testing.T) {
	input, _ := json.Marshal(map[string]string{"file_path": "/tmp/test.go"})
	got := extractMatchContent("Read", input)
	if got != "/tmp/test.go" {
		t.Errorf("extractMatchContent(Read) = %q, want %q", got, "/tmp/test.go")
	}
}

func TestExtractMatchContent_Write(t *testing.T) {
	input, _ := json.Marshal(map[string]any{"file_path": "/tmp/out.txt", "content": "hello"})
	got := extractMatchContent("Write", input)
	if got != "/tmp/out.txt" {
		t.Errorf("extractMatchContent(Write) = %q, want %q", got, "/tmp/out.txt")
	}
}

func TestExtractMatchContent_Edit(t *testing.T) {
	input, _ := json.Marshal(map[string]any{"file_path": "/tmp/edit.go", "old_string": "a", "new_string": "b"})
	got := extractMatchContent("Edit", input)
	if got != "/tmp/edit.go" {
		t.Errorf("extractMatchContent(Edit) = %q, want %q", got, "/tmp/edit.go")
	}
}

func TestExtractMatchContent_WebFetch(t *testing.T) {
	input, _ := json.Marshal(map[string]string{"url": "https://example.com/page"})
	got := extractMatchContent("WebFetch", input)
	// WebFetch extracts "domain:" + hostname (path stripped)
	if got != "domain:example.com" {
		t.Errorf("extractMatchContent(WebFetch) = %q, want %q", got, "domain:example.com")
	}
}

func TestExtractMatchContent_UnknownTool(t *testing.T) {
	input, _ := json.Marshal(map[string]string{"anything": "value"})
	got := extractMatchContent("UnknownTool", input)
	// Unknown tools return the raw JSON string for matching
	if got == "" {
		t.Error("extractMatchContent(UnknownTool) should return raw JSON, not empty")
	}
}

func TestExtractMatchContent_InvalidJSON(t *testing.T) {
	got := extractMatchContent("Bash", json.RawMessage(`not json`))
	if got != "" {
		t.Errorf("extractMatchContent with invalid JSON = %q, want empty", got)
	}
}

// ---------------------------------------------------------------------------
// Match
// ---------------------------------------------------------------------------

func TestMatch_NoRules(t *testing.T) {
	input, _ := json.Marshal(map[string]string{"command": "ls"})
	_, matched := Match("Bash", input, nil)
	if matched {
		t.Error("Match with nil rules should not match")
	}

	_, matched = Match("Bash", input, []config.PermissionRule{})
	if matched {
		t.Error("Match with empty rules should not match")
	}
}

func TestMatch_AllowWildcard(t *testing.T) {
	rules := []config.PermissionRule{
		{Tool: "Bash", Pattern: "*", Behavior: "allow"},
	}
	input, _ := json.Marshal(map[string]string{"command": "rm -rf /"})

	behavior, matched := Match("Bash", input, rules)
	if !matched {
		t.Fatal("wildcard Bash(*) rule should match any Bash command")
	}
	if behavior != "allow" {
		t.Errorf("behavior = %q, want %q", behavior, "allow")
	}
}

func TestMatch_AllowPrefix(t *testing.T) {
	rules := []config.PermissionRule{
		{Tool: "Bash", Pattern: "git *", Behavior: "allow"},
	}

	input, _ := json.Marshal(map[string]string{"command": "git status"})
	behavior, matched := Match("Bash", input, rules)
	if !matched || behavior != "allow" {
		t.Error("git * rule should match 'git status'")
	}

	input, _ = json.Marshal(map[string]string{"command": "rm -rf /"})
	_, matched = Match("Bash", input, rules)
	if matched {
		t.Error("git * rule should NOT match 'rm -rf /'")
	}
}

func TestMatch_DenyRule(t *testing.T) {
	rules := []config.PermissionRule{
		{Tool: "Write", Pattern: "*.env", Behavior: "deny"},
	}
	input, _ := json.Marshal(map[string]string{"file_path": "secret.env"})

	behavior, matched := Match("Write", input, rules)
	if !matched {
		t.Fatal("*.env deny rule should match .env file")
	}
	if behavior != "deny" {
		t.Errorf("behavior = %q, want %q", behavior, "deny")
	}
}

func TestMatch_FirstRuleWins(t *testing.T) {
	rules := []config.PermissionRule{
		{Tool: "Bash", Pattern: "git *", Behavior: "deny"},
		{Tool: "Bash", Pattern: "*", Behavior: "allow"},
	}
	input, _ := json.Marshal(map[string]string{"command": "git push --force"})

	behavior, matched := Match("Bash", input, rules)
	if !matched {
		t.Fatal("should match first rule")
	}
	if behavior != "deny" {
		t.Errorf("first matching rule should win: got %q, want %q", behavior, "deny")
	}
}

func TestMatch_ToolNameMismatch(t *testing.T) {
	rules := []config.PermissionRule{
		{Tool: "Bash", Pattern: "*", Behavior: "allow"},
	}
	input, _ := json.Marshal(map[string]string{"file_path": "/tmp/test.go"})

	_, matched := Match("Read", input, rules)
	if matched {
		t.Error("Bash rule should NOT match Read tool")
	}
}

func TestMatch_EmptyContent(t *testing.T) {
	rules := []config.PermissionRule{
		{Tool: "Bash", Pattern: "*", Behavior: "allow"},
	}
	// Empty command → extractMatchContent returns "" → rule is skipped
	input, _ := json.Marshal(map[string]string{"command": ""})

	_, matched := Match("Bash", input, rules)
	if matched {
		t.Error("rule should not match when extractMatchContent returns empty string")
	}
}

func TestMatch_MultipleRulesDifferentTools(t *testing.T) {
	rules := []config.PermissionRule{
		{Tool: "Bash", Pattern: "git *", Behavior: "allow"},
		{Tool: "Read", Pattern: "*", Behavior: "allow"},
		{Tool: "Write", Pattern: "*.env", Behavior: "deny"},
	}

	// Bash git command → matches first rule
	input, _ := json.Marshal(map[string]string{"command": "git log"})
	behavior, matched := Match("Bash", input, rules)
	if !matched || behavior != "allow" {
		t.Error("Bash 'git log' should match 'git *' allow rule")
	}

	// Read any file → matches second rule
	input, _ = json.Marshal(map[string]string{"file_path": "/etc/passwd"})
	behavior, matched = Match("Read", input, rules)
	if !matched || behavior != "allow" {
		t.Error("Read should match wildcard allow rule")
	}

	// Write .env file → matches third rule
	input, _ = json.Marshal(map[string]string{"file_path": ".env"})
	behavior, matched = Match("Write", input, rules)
	if !matched || behavior != "deny" {
		t.Error("Write .env should match deny rule")
	}

	// Write normal file → no matching rule
	input, _ = json.Marshal(map[string]string{"file_path": "main.go"})
	_, matched = Match("Write", input, rules)
	if matched {
		t.Error("Write main.go should not match any rule")
	}
}
