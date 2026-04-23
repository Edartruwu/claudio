package plugins

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeScript writes a shell script to dir with the given content and makes it executable.
func writeScript(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("writeScript: %v", err)
	}
	return path
}

// TestExecute_StdinReceivesFullJSON verifies the plugin always gets the raw JSON on stdin.
func TestExecute_StdinReceivesFullJSON(t *testing.T) {
	dir := t.TempDir()
	// Script echoes stdin so we can capture what the plugin received.
	scriptPath := writeScript(t, dir, "echo_stdin.sh", "#!/bin/sh\ncat\n")

	tool := &PluginProxyTool{
		PluginPath: scriptPath,
		PluginName: "echo_stdin",
	}

	// Input has fields beyond command/args/input — simulates a rich plugin schema.
	rawInput := json.RawMessage(`{"target":"192.168.1.1","ports":"80,443","scan_type":"syn"}`)

	result, err := tool.Execute(context.Background(), rawInput)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.IsError {
		t.Fatalf("plugin returned error: %s", result.Content)
	}

	var got map[string]string
	if err := json.Unmarshal([]byte(result.Content), &got); err != nil {
		t.Fatalf("output not valid JSON: %v — got: %q", err, result.Content)
	}
	if got["target"] != "192.168.1.1" {
		t.Errorf("target field: want 192.168.1.1, got %q", got["target"])
	}
	if got["ports"] != "80,443" {
		t.Errorf("ports field: want 80,443, got %q", got["ports"])
	}
	if got["scan_type"] != "syn" {
		t.Errorf("scan_type field: want syn, got %q", got["scan_type"])
	}
}

// TestExecute_CLIArgsStillWork verifies command/args fields still forwarded as argv.
func TestExecute_CLIArgsStillWork(t *testing.T) {
	dir := t.TempDir()
	// Script prints its positional arguments.
	scriptPath := writeScript(t, dir, "print_args.sh", "#!/bin/sh\necho \"$@\"\n")

	tool := &PluginProxyTool{
		PluginPath: scriptPath,
		PluginName: "print_args",
	}

	rawInput := json.RawMessage(`{"command":"foo","args":"bar baz"}`)

	result, err := tool.Execute(context.Background(), rawInput)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.IsError {
		t.Fatalf("plugin returned error: %s", result.Content)
	}

	want := "foo bar baz"
	got := result.Content
	// trim trailing newline from echo
	if len(got) > 0 && got[len(got)-1] == '\n' {
		got = got[:len(got)-1]
	}
	if got != want {
		t.Errorf("args: want %q, got %q", want, got)
	}
}

// TestExecute_BothModesSimultaneous verifies plugin receives CLI args AND full JSON stdin together.
func TestExecute_BothModesSimultaneous(t *testing.T) {
	dir := t.TempDir()
	// Script prints first arg then echoes stdin JSON field via jq-free approach:
	// it uses the shell built-in and grep to avoid jq dependency.
	// Simpler: just verify stdin is non-empty and arg is passed.
	scriptPath := writeScript(t, dir, "both.sh", `#!/bin/sh
echo "arg=$1"
cat
`)

	tool := &PluginProxyTool{
		PluginPath: scriptPath,
		PluginName: "both",
	}

	rawInput := json.RawMessage(`{"command":"hello","target":"example.com"}`)

	result, err := tool.Execute(context.Background(), rawInput)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.IsError {
		t.Fatalf("plugin returned error: %s", result.Content)
	}

	content := result.Content
	// Check arg was forwarded.
	if !containsSubstr(content, "arg=hello") {
		t.Errorf("CLI arg not forwarded; output: %q", content)
	}
	// Check stdin JSON was forwarded (plugin echoed it via cat).
	var got map[string]any
	// Extract the JSON portion: everything after the first newline.
	idx := 0
	for idx < len(content) && content[idx] != '\n' {
		idx++
	}
	if idx < len(content) {
		jsonPart := content[idx+1:]
		if err := json.Unmarshal([]byte(jsonPart), &got); err != nil {
			t.Errorf("stdin JSON not valid: %v — got: %q", err, jsonPart)
		} else if got["target"] != "example.com" {
			t.Errorf("stdin target field: want example.com, got %v", got["target"])
		}
	} else {
		t.Errorf("no newline in output, stdin not forwarded; output: %q", content)
	}
}

func containsSubstr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && findSubstr(s, sub))
}

func findSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
