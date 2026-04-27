package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// ---------------------------------------------------------------------------
// VerifyPrototypeTool / metadata
// ---------------------------------------------------------------------------

func TestVerifyPrototypeTool_Name(t *testing.T) {
	tool := &VerifyPrototypeTool{}
	if tool.Name() != "VerifyPrototype" {
		t.Errorf("expected Name()=%q, got %q", "VerifyPrototype", tool.Name())
	}
}

func TestVerifyPrototypeTool_DescriptionNonEmpty(t *testing.T) {
	tool := &VerifyPrototypeTool{}
	if tool.Description() == "" {
		t.Error("Description() should not be empty")
	}
}

func TestVerifyPrototypeTool_InputSchemaValidJSON(t *testing.T) {
	tool := &VerifyPrototypeTool{}
	schema := tool.InputSchema()
	var out interface{}
	if err := json.Unmarshal(schema, &out); err != nil {
		t.Errorf("InputSchema() is not valid JSON: %v", err)
	}
}

func TestVerifyPrototypeTool_IsReadOnly(t *testing.T) {
	tool := &VerifyPrototypeTool{}
	if !tool.IsReadOnly() {
		t.Error("VerifyPrototypeTool should be read-only")
	}
}

func TestVerifyPrototypeTool_RequiresApproval(t *testing.T) {
	tool := &VerifyPrototypeTool{}
	if tool.RequiresApproval(nil) {
		t.Error("VerifyPrototypeTool should not require approval")
	}
}

// ---------------------------------------------------------------------------
// Input validation
// ---------------------------------------------------------------------------

func TestVerifyPrototypeTool_MissingHTMLPath_ReturnsError(t *testing.T) {
	tool := &VerifyPrototypeTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for missing html_path")
	}
}

func TestVerifyPrototypeTool_InvalidJSON_ReturnsError(t *testing.T) {
	tool := &VerifyPrototypeTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{bad json`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for invalid JSON input")
	}
}

// ---------------------------------------------------------------------------
// Happy-path test — mock python3 via PATH injection
// ---------------------------------------------------------------------------

// TestVerifyPrototypeTool_HappyPath mocks the python3 subprocess by writing a
// tiny shell script (or batch file on Windows) named "python3" into a temp dir
// and prepending that dir to PATH. The fake python3 ignores all arguments and
// outputs valid JSON matching verifyPrototypeOutput, so no real Playwright
// installation is required.
func TestVerifyPrototypeTool_HappyPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH-injection mock not supported on Windows")
	}

	// Create a temp dir for our fake executables.
	binDir := t.TempDir()

	// Write fake python3 script — outputs expected JSON, exits 0 for every invocation.
	fakePy := filepath.Join(binDir, "python3")
	script := `#!/bin/sh
echo '{"passed": true, "steps_completed": 2, "steps_total": 2, "console_errors": [], "screenshots": [], "failure_reason": "", "duration_ms": 150}'
`
	if err := os.WriteFile(fakePy, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake python3: %v", err)
	}

	// Prepend binDir to PATH so exec.Command("python3", ...) finds our fake.
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+":"+origPath)

	// Create a minimal HTML file so os.Stat passes.
	htmlDir := t.TempDir()
	htmlPath := filepath.Join(htmlDir, "test.html")
	if err := os.WriteFile(htmlPath, []byte("<html></html>"), 0o644); err != nil {
		t.Fatalf("write html file: %v", err)
	}

	tool := &VerifyPrototypeTool{}
	input := json.RawMessage(`{"html_path":"` + htmlPath + `","click_sequence":[]}`)

	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute returned error result: %s", result.Content)
	}

	// Parse the result content back to verify fields.
	var out verifyPrototypeOutput
	if err := json.Unmarshal([]byte(result.Content), &out); err != nil {
		// Content may have trailing warnings section — extract JSON prefix.
		t.Fatalf("could not parse result content as JSON: %v\ncontent: %s", err, result.Content)
	}
	if !out.Passed {
		t.Errorf("expected Passed=true, got false")
	}
	if out.StepsCompleted != 2 {
		t.Errorf("expected StepsCompleted=2, got %d", out.StepsCompleted)
	}
}
