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

// ---------------------------------------------------------------------------
// InjectInfiniteCanvas — unit tests
// ---------------------------------------------------------------------------

func TestInjectInfiniteCanvas_TransparentAppWrapper(t *testing.T) {
	input := `<html><head></head><body><div id="root"><div style="background: #D4E5F3"><p>content</p></div></div></body></html>`
	output := InjectInfiniteCanvas(input)
	if !strings.Contains(output, `#root>div{background:transparent!important}`) {
		t.Errorf("expected transparent bg override rule in output, got:\n%s", output)
	}
}

func TestInjectInfiniteCanvas_CanvasRootInjected(t *testing.T) {
	input := `<html><head></head><body><div id="root"></div></body></html>`
	output := InjectInfiniteCanvas(input)
	if !strings.Contains(output, `id="cc-canvas-root"`) {
		t.Errorf("expected cc-canvas-root in output, got:\n%s", output)
	}
	if !strings.Contains(output, `id="cc-canvas-content"`) {
		t.Errorf("expected cc-canvas-content in output, got:\n%s", output)
	}
}

func TestInjectInfiniteCanvas_CanvasCSSInjectedInHead(t *testing.T) {
	input := `<html><head></head><body><p>hi</p></body></html>`
	output := InjectInfiniteCanvas(input)
	cssIdx := strings.Index(output, `id="cc-canvas-style"`)
	headCloseIdx := strings.Index(output, `</head>`)
	if cssIdx == -1 {
		t.Fatal("cc-canvas-style not found in output")
	}
	if headCloseIdx == -1 {
		t.Fatal("</head> not found in output")
	}
	if cssIdx > headCloseIdx {
		t.Errorf("canvas CSS should appear before </head>: css at %d, </head> at %d", cssIdx, headCloseIdx)
	}
}

func TestInjectInfiniteCanvas_NoHeadFallback(t *testing.T) {
	// No </head> — CSS must be prepended to the document, not dropped.
	input := `<body><p>no head tag</p></body>`
	output := InjectInfiniteCanvas(input)
	if !strings.Contains(output, `id="cc-canvas-style"`) {
		t.Errorf("expected canvas CSS injected even without </head>, got:\n%s", output)
	}
	// CSS should appear before the original content
	cssIdx := strings.Index(output, `id="cc-canvas-style"`)
	bodyIdx := strings.Index(output, `<body>`)
	if cssIdx > bodyIdx {
		t.Errorf("CSS should be prepended before <body>: css at %d, <body> at %d", cssIdx, bodyIdx)
	}
}

func TestInjectInfiniteCanvas_NoBodyUnmodified(t *testing.T) {
	// No <body> tag — canvas shell must not be injected.
	input := `<p>no body tag here</p>`
	output := InjectInfiniteCanvas(input)
	if strings.Contains(output, `id="cc-canvas-root"`) {
		t.Errorf("expected no canvas-root injection when no <body> tag present, got:\n%s", output)
	}
	if strings.Contains(output, `id="cc-canvas-content"`) {
		t.Errorf("expected no canvas-content injection when no <body> tag present, got:\n%s", output)
	}
}
