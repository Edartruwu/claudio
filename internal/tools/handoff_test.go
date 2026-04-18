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
// NewExportHandoffTool / metadata
// ---------------------------------------------------------------------------

func TestNewExportHandoffTool_NotNil(t *testing.T) {
	tool := NewExportHandoffTool("/tmp/designs")
	if tool == nil {
		t.Error("NewExportHandoffTool returned nil")
	}
}

func TestExportHandoffTool_Name(t *testing.T) {
	tool := NewExportHandoffTool("/tmp/designs")
	if tool.Name() != "ExportHandoff" {
		t.Errorf("expected Name()=%q, got %q", "ExportHandoff", tool.Name())
	}
}

func TestExportHandoffTool_DescriptionNonEmpty(t *testing.T) {
	tool := NewExportHandoffTool("/tmp/designs")
	if tool.Description() == "" {
		t.Error("Description() should not be empty")
	}
}

func TestExportHandoffTool_InputSchemaValidJSON(t *testing.T) {
	tool := NewExportHandoffTool("/tmp/designs")
	schema := tool.InputSchema()
	var out interface{}
	if err := json.Unmarshal(schema, &out); err != nil {
		t.Errorf("InputSchema() is not valid JSON: %v", err)
	}
}

func TestExportHandoffTool_IsReadOnly(t *testing.T) {
	tool := NewExportHandoffTool("/tmp/designs")
	if tool.IsReadOnly() {
		t.Error("ExportHandoffTool should not be read-only")
	}
}

func TestExportHandoffTool_RequiresApproval(t *testing.T) {
	tool := NewExportHandoffTool("/tmp/designs")
	if tool.RequiresApproval(nil) {
		t.Error("ExportHandoffTool should not require approval")
	}
}

// ---------------------------------------------------------------------------
// parseComponents — component detection (package-internal function)
// ---------------------------------------------------------------------------

func TestParseComponents_DetectsBtn(t *testing.T) {
	html := `<div class="btn btn-primary">Click me</div>`
	components := parseComponents(html)

	found := false
	for _, c := range components {
		if c.Name == "Button" {
			found = true
			if c.Count == 0 {
				t.Error("Button component count should be > 0")
			}
			break
		}
	}
	if !found {
		t.Errorf("expected Button component, got %+v", components)
	}
}

func TestParseComponents_DetectsCard(t *testing.T) {
	html := `<div class="card card-body shadow"><p>Content</p></div>`
	components := parseComponents(html)

	found := false
	for _, c := range components {
		if c.Name == "Card" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected Card component, got %+v", components)
	}
}

func TestParseComponents_EmptyHTML_NoComponents(t *testing.T) {
	components := parseComponents("")
	if len(components) != 0 {
		t.Errorf("expected 0 components from empty HTML, got %d", len(components))
	}
}

func TestParseComponents_NoClasses_NoComponents(t *testing.T) {
	html := `<div><p>Hello world</p></div>`
	components := parseComponents(html)
	if len(components) != 0 {
		t.Errorf("expected 0 components, got %d: %+v", len(components), components)
	}
}

func TestParseComponents_MultipleComponents(t *testing.T) {
	html := `<nav class="navbar"><div class="btn">Go</div><div class="card">Info</div></nav>`
	components := parseComponents(html)
	if len(components) < 3 {
		t.Errorf("expected at least 3 components (nav, button, card), got %d: %+v", len(components), components)
	}
}

// ---------------------------------------------------------------------------
// Execute — token cross-reference
// ---------------------------------------------------------------------------

func TestExportHandoffTool_TokenCrossReference(t *testing.T) {
	dir := t.TempDir()

	// Create index.html containing a token key substring
	htmlContent := `<html><body><div class="btn" style="color: primary-blue;">Hello</div></body></html>`
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(htmlContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create design-system.json with a token whose key appears in HTML
	tokensContent := `{"primary-blue": "#0057FF", "secondary-red": "#FF4400"}`
	tokensPath := filepath.Join(dir, "design-system.json")
	if err := os.WriteFile(tokensPath, []byte(tokensContent), 0644); err != nil {
		t.Fatal(err)
	}

	tool := NewExportHandoffTool(dir)
	input, _ := json.Marshal(ExportHandoffInput{
		MockupDir:    dir,
		DesignTokens: tokensPath,
		ProjectName:  "test-project",
	})

	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute returned IsError=true: %s", result.Content)
	}

	// Parse the output JSON from result.Content
	var out ExportHandoffOutput
	contentPart := result.Content
	if idx := strings.Index(contentPart, "\n\nWarnings:"); idx != -1 {
		contentPart = contentPart[:idx]
	}
	if err := json.Unmarshal([]byte(contentPart), &out); err != nil {
		t.Fatalf("could not parse result JSON: %v\ncontent: %s", err, result.Content)
	}

	if !out.Success {
		t.Error("expected Success=true")
	}

	// tokens-used.json should contain "primary-blue" (it appears in HTML)
	tokensUsedData, err := os.ReadFile(out.TokensUsedPath)
	if err != nil {
		t.Fatalf("could not read tokens-used.json: %v", err)
	}
	if !strings.Contains(string(tokensUsedData), "primary-blue") {
		t.Errorf("expected 'primary-blue' in tokens-used.json, got: %s", string(tokensUsedData))
	}
	// "secondary-red" does NOT appear in HTML, should not be in used tokens
	if strings.Contains(string(tokensUsedData), "secondary-red") {
		t.Errorf("unexpected 'secondary-red' in tokens-used.json — it was not in the HTML")
	}
}

// ---------------------------------------------------------------------------
// Execute — validation paths
// ---------------------------------------------------------------------------

func TestExportHandoffTool_MissingMockupDir_ReturnsError(t *testing.T) {
	tool := NewExportHandoffTool(t.TempDir())
	input, _ := json.Marshal(map[string]string{})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when mockup_dir is missing")
	}
}

func TestExportHandoffTool_NoIndexHTML_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	tool := NewExportHandoffTool(dir)
	input, _ := json.Marshal(ExportHandoffInput{MockupDir: dir})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when mockup_dir has no index.html")
	}
}

func TestExportHandoffTool_InvalidJSON_ReturnsError(t *testing.T) {
	tool := NewExportHandoffTool(t.TempDir())
	result, err := tool.Execute(context.Background(), json.RawMessage(`{bad`))
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true on invalid JSON input")
	}
}

// ---------------------------------------------------------------------------
// Execute — component detection via full Execute flow
// ---------------------------------------------------------------------------

func TestExportHandoffTool_ComponentDetection_BtnAndCard(t *testing.T) {
	dir := t.TempDir()
	htmlContent := `<html><body>
<div class="btn btn-primary">Submit</div>
<div class="card shadow">Content</div>
</body></html>`
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(htmlContent), 0644); err != nil {
		t.Fatal(err)
	}

	tool := NewExportHandoffTool(dir)
	input, _ := json.Marshal(ExportHandoffInput{MockupDir: dir})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute returned IsError=true: %s", result.Content)
	}

	// Parse output
	contentPart := result.Content
	if idx := strings.Index(contentPart, "\n\nWarnings:"); idx != -1 {
		contentPart = contentPart[:idx]
	}
	var out ExportHandoffOutput
	if err := json.Unmarshal([]byte(contentPart), &out); err != nil {
		t.Fatalf("could not parse result JSON: %v\ncontent: %s", err, result.Content)
	}

	if out.ComponentCount == 0 {
		t.Errorf("expected component_count > 0 for HTML with btn and card, got 0")
	}

	// spec.md should mention Button and Card components
	specData, err := os.ReadFile(out.SpecPath)
	if err != nil {
		t.Fatalf("could not read spec.md: %v", err)
	}
	specStr := string(specData)
	if !strings.Contains(specStr, "Button") {
		t.Error("spec.md should contain 'Button' component")
	}
	if !strings.Contains(specStr, "Card") {
		t.Error("spec.md should contain 'Card' component")
	}
}
