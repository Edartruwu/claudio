package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// NewBundleMockupTool / metadata
// ---------------------------------------------------------------------------

func TestNewBundleMockupTool_NotNil(t *testing.T) {
	tool := NewBundleMockupTool("/tmp/designs")
	if tool == nil {
		t.Error("NewBundleMockupTool returned nil")
	}
}

func TestBundleMockupTool_Name(t *testing.T) {
	tool := NewBundleMockupTool("/tmp/designs")
	if tool.Name() != "BundleMockup" {
		t.Errorf("expected Name()=%q, got %q", "BundleMockup", tool.Name())
	}
}

func TestBundleMockupTool_DescriptionNonEmpty(t *testing.T) {
	tool := NewBundleMockupTool("/tmp/designs")
	if tool.Description() == "" {
		t.Error("Description() should not be empty")
	}
}

func TestBundleMockupTool_InputSchemaValidJSON(t *testing.T) {
	tool := NewBundleMockupTool("/tmp/designs")
	schema := tool.InputSchema()
	var out interface{}
	if err := json.Unmarshal(schema, &out); err != nil {
		t.Errorf("InputSchema() is not valid JSON: %v", err)
	}
}

func TestBundleMockupTool_IsReadOnly(t *testing.T) {
	tool := NewBundleMockupTool("/tmp/designs")
	if tool.IsReadOnly() {
		t.Error("BundleMockupTool should not be read-only")
	}
}

func TestBundleMockupTool_RequiresApproval(t *testing.T) {
	tool := NewBundleMockupTool("/tmp/designs")
	if tool.RequiresApproval(nil) {
		t.Error("BundleMockupTool should not require approval")
	}
}

// ---------------------------------------------------------------------------
// Execute — local script inlining
// ---------------------------------------------------------------------------

func TestBundleMockupTool_LocalScriptInlined(t *testing.T) {
	dir := t.TempDir()

	// Write JS file
	jsContent := `console.log("hello from foo");`
	jsPath := filepath.Join(dir, "foo.js")
	if err := os.WriteFile(jsPath, []byte(jsContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Write HTML with local script reference
	htmlContent := `<html><body><script src="./foo.js"></script></body></html>`
	htmlPath := filepath.Join(dir, "index.html")
	if err := os.WriteFile(htmlPath, []byte(htmlContent), 0644); err != nil {
		t.Fatal(err)
	}

	outPath := filepath.Join(dir, "out.html")
	falseVal := false
	input, _ := json.Marshal(BundleMockupInput{
		EntryHTML:  htmlPath,
		OutputPath: outPath,
		EmbedCDN:   &falseVal,
	})

	tool := NewBundleMockupTool(dir)
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute returned IsError=true: %s", result.Content)
	}

	// Check output file contains inlined JS
	bundled, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("could not read output file: %v", err)
	}
	if !strings.Contains(string(bundled), `console.log("hello from foo")`) {
		t.Errorf("expected inlined JS in output, got:\n%s", string(bundled))
	}
	// src="./foo.js" should be gone
	if strings.Contains(string(bundled), `src="./foo.js"`) {
		t.Error("original <script src=> tag should be removed after inlining")
	}
}

func TestBundleMockupTool_MissingLocalScript_KeepsOriginalTag(t *testing.T) {
	dir := t.TempDir()

	// HTML references a file that does NOT exist
	htmlContent := `<html><body><script src="./missing.js"></script></body></html>`
	htmlPath := filepath.Join(dir, "index.html")
	if err := os.WriteFile(htmlPath, []byte(htmlContent), 0644); err != nil {
		t.Fatal(err)
	}

	outPath := filepath.Join(dir, "out.html")
	falseVal := false
	input, _ := json.Marshal(BundleMockupInput{
		EntryHTML:  htmlPath,
		OutputPath: outPath,
		EmbedCDN:   &falseVal,
	})

	tool := NewBundleMockupTool(dir)
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	// Tool should succeed (with a warning) — not IsError
	if result.IsError {
		t.Fatalf("expected success with warning, got IsError=true: %s", result.Content)
	}

	bundled, _ := os.ReadFile(outPath)
	// Original tag should be preserved
	if !strings.Contains(string(bundled), "missing.js") {
		t.Error("expected original tag with missing.js to be preserved")
	}
	// Warning should mention the missing file
	if !strings.Contains(result.Content, "missing.js") {
		t.Errorf("expected warning about missing.js in result, got: %s", result.Content)
	}
}

// ---------------------------------------------------------------------------
// Execute — CDN scripts not processed by local handler
// ---------------------------------------------------------------------------

func TestBundleMockupTool_CDNScript_SkippedByLocalHandler_EmbedFalse(t *testing.T) {
	dir := t.TempDir()

	// HTML contains a CDN script tag
	htmlContent := `<html><body><script src="https://cdn.example.com/foo.js"></script></body></html>`
	htmlPath := filepath.Join(dir, "index.html")
	if err := os.WriteFile(htmlPath, []byte(htmlContent), 0644); err != nil {
		t.Fatal(err)
	}

	outPath := filepath.Join(dir, "out.html")
	falseVal := false
	input, _ := json.Marshal(BundleMockupInput{
		EntryHTML:  htmlPath,
		OutputPath: outPath,
		EmbedCDN:   &falseVal,
	})

	tool := NewBundleMockupTool(dir)
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute returned IsError=true: %s", result.Content)
	}

	bundled, _ := os.ReadFile(outPath)
	// CDN tag should remain intact (not inlined, not mangled)
	if !strings.Contains(string(bundled), "https://cdn.example.com/foo.js") {
		t.Error("CDN script src should be preserved when embed_cdn=false")
	}
}

func TestBundleMockupTool_CDNEmbedTrue_FakeURL_NoFanicOnFail(t *testing.T) {
	dir := t.TempDir()

	// HTML with an unreachable CDN URL
	htmlContent := `<html><body><script src="https://invalid.test.invalid/no-such-lib.js"></script></body></html>`
	htmlPath := filepath.Join(dir, "index.html")
	if err := os.WriteFile(htmlPath, []byte(htmlContent), 0644); err != nil {
		t.Fatal(err)
	}

	outPath := filepath.Join(dir, "out.html")
	trueVal := true
	input, _ := json.Marshal(BundleMockupInput{
		EntryHTML:  htmlPath,
		OutputPath: outPath,
		EmbedCDN:   &trueVal,
	})

	tool := NewBundleMockupTool(dir)
	// Should not panic — CDN fetch failure should be handled gracefully
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute should not return a Go error on CDN fetch failure: %v", err)
	}
	// IsError should be false (CDN failure is a warning, not fatal)
	if result.IsError {
		t.Errorf("CDN fetch failure should produce warning, not IsError=true; got: %s", result.Content)
	}
}

// ---------------------------------------------------------------------------
// Execute — missing entry_html
// ---------------------------------------------------------------------------

func TestBundleMockupTool_MissingEntryHTML_ReturnsError(t *testing.T) {
	tool := NewBundleMockupTool(t.TempDir())
	input, _ := json.Marshal(map[string]string{})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when entry_html is missing")
	}
}

func TestBundleMockupTool_InvalidJSON_ReturnsError(t *testing.T) {
	tool := NewBundleMockupTool(t.TempDir())
	result, err := tool.Execute(context.Background(), json.RawMessage(`{bad json`))
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true on invalid JSON input")
	}
}
