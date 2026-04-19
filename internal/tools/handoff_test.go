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

// ---------------------------------------------------------------------------
// parseComponentAnnotations — COMPONENT: ... annotation parsing
// ---------------------------------------------------------------------------

func TestParseComponentAnnotations_Empty(t *testing.T) {
	files := []fileContent{{path: "index.html", name: "index.html", content: ""}}
	specs := parseComponentAnnotations(files)
	if len(specs) != 0 {
		t.Errorf("expected 0 component specs from empty HTML, got %d", len(specs))
	}
}

func TestParseComponentAnnotations_Single(t *testing.T) {
	html := `<!-- COMPONENT: Button
states: default, hover, active
breakpoints: mobile, desktop
tokens: primary-blue, neutral-gray
measurements: 44px height min
-->
<button class="btn">Click</button>`
	files := []fileContent{{path: "screen-1.html", name: "screen-1.html", content: html}}
	specs := parseComponentAnnotations(files)

	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	spec := specs[0]
	if spec.Name != "Button" {
		t.Errorf("expected Name=Button, got %s", spec.Name)
	}
	if len(spec.States) != 3 {
		t.Errorf("expected 3 states, got %d: %v", len(spec.States), spec.States)
	}
	if spec.States[0] != "default" {
		t.Errorf("expected states[0]=default, got %s", spec.States[0])
	}
	if len(spec.Breakpoints) != 2 {
		t.Errorf("expected 2 breakpoints, got %d: %v", len(spec.Breakpoints), spec.Breakpoints)
	}
	if len(spec.Tokens) != 2 {
		t.Errorf("expected 2 tokens, got %d: %v", len(spec.Tokens), spec.Tokens)
	}
	if spec.Measurements != "44px height min" {
		t.Errorf("expected measurements='44px height min', got %q", spec.Measurements)
	}
	if len(spec.ScreenNames) != 1 || spec.ScreenNames[0] != "1" {
		t.Errorf("expected ScreenNames=[1], got %v", spec.ScreenNames)
	}
}

func TestParseComponentAnnotations_MultipleAnnotations(t *testing.T) {
	html := `<!-- COMPONENT: Button
states: default, hover
-->
<button>Btn1</button>
<!-- COMPONENT: Card
states: default
-->
<div class="card">Card1</div>`
	files := []fileContent{{path: "screen.html", name: "screen.html", content: html}}
	specs := parseComponentAnnotations(files)

	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}
	if specs[0].Name != "Button" || specs[1].Name != "Card" {
		t.Errorf("expected Button then Card, got %s then %s", specs[0].Name, specs[1].Name)
	}
}

func TestParseComponentAnnotations_MissingOptionalFields(t *testing.T) {
	// Only Name + Measurements, no states/breakpoints/tokens
	html := `<!-- COMPONENT: Modal
measurements: 600px × 400px
-->
<div class="modal">Content</div>`
	files := []fileContent{{path: "screen.html", name: "screen.html", content: html}}
	specs := parseComponentAnnotations(files)

	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	spec := specs[0]
	if spec.Name != "Modal" {
		t.Errorf("expected Name=Modal, got %s", spec.Name)
	}
	if len(spec.States) != 0 {
		t.Errorf("expected 0 states when field absent, got %d", len(spec.States))
	}
	if len(spec.Breakpoints) != 0 {
		t.Errorf("expected 0 breakpoints when field absent, got %d", len(spec.Breakpoints))
	}
	if len(spec.Tokens) != 0 {
		t.Errorf("expected 0 tokens when field absent, got %d", len(spec.Tokens))
	}
	if spec.Measurements != "600px × 400px" {
		t.Errorf("expected measurements='600px × 400px', got %q", spec.Measurements)
	}
}

func TestParseComponentAnnotations_MultilineAnnotationMultipleFiles(t *testing.T) {
	// Same component name in two files → merged ScreenNames
	html1 := `<!-- COMPONENT: Button
states: default, hover
breakpoints: mobile
--><button>B1</button>`
	html2 := `<!-- COMPONENT: Button
states: active
breakpoints: desktop
--><button>B2</button>`
	files := []fileContent{
		{path: "screen-a.html", name: "screen-a.html", content: html1},
		{path: "screen-b.html", name: "screen-b.html", content: html2},
	}
	specs := parseComponentAnnotations(files)

	if len(specs) != 1 {
		t.Fatalf("expected 1 merged spec, got %d", len(specs))
	}
	spec := specs[0]
	if spec.Name != "Button" {
		t.Errorf("expected Name=Button, got %s", spec.Name)
	}
	if len(spec.ScreenNames) != 2 {
		t.Errorf("expected 2 screen names, got %d: %v", len(spec.ScreenNames), spec.ScreenNames)
	}
	// States should merge uniquely
	if len(spec.States) != 3 || !containsString(spec.States, "default") || !containsString(spec.States, "hover") || !containsString(spec.States, "active") {
		t.Errorf("expected merged states [default, hover, active], got %v", spec.States)
	}
	// Breakpoints should merge uniquely
	if len(spec.Breakpoints) != 2 || !containsString(spec.Breakpoints, "mobile") || !containsString(spec.Breakpoints, "desktop") {
		t.Errorf("expected merged breakpoints [mobile, desktop], got %v", spec.Breakpoints)
	}
}

func TestParseComponentAnnotations_MalformedAnnotation(t *testing.T) {
	// No closing comment — regex requires --> so no match
	html := `<!-- COMPONENT: Button
states: default
<button>Btn</button>`
	files := []fileContent{{path: "screen.html", name: "screen.html", content: html}}
	specs := parseComponentAnnotations(files)

	// Regex won't match unclosed (no -->), so 0 specs expected
	if len(specs) != 0 {
		t.Errorf("expected 0 specs for unclosed annotation, got %d", len(specs))
	}
}

func TestParseComponentAnnotations_FieldsCaseInsensitive(t *testing.T) {
	html := `<!-- COMPONENT: Label
STATES: on, off
BreakPoints: tablet
Tokens: color-primary
Measurements: 16px wide
-->
<label>Lbl</label>`
	files := []fileContent{{path: "screen.html", name: "screen.html", content: html}}
	specs := parseComponentAnnotations(files)

	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	spec := specs[0]
	if len(spec.States) != 2 {
		t.Errorf("expected case-insensitive STATES parsed, got %d states", len(spec.States))
	}
	if len(spec.Breakpoints) != 1 {
		t.Errorf("expected case-insensitive BreakPoints parsed, got %d", len(spec.Breakpoints))
	}
	if len(spec.Tokens) != 1 {
		t.Errorf("expected case-insensitive Tokens parsed, got %d", len(spec.Tokens))
	}
}

// ---------------------------------------------------------------------------
// parseInteractionAnnotations — INTERACTION: ... annotation parsing
// ---------------------------------------------------------------------------

func TestParseInteractionAnnotations_Empty(t *testing.T) {
	files := []fileContent{{path: "index.html", name: "index.html", content: ""}}
	interactions := parseInteractionAnnotations(files)
	if len(interactions) != 0 {
		t.Errorf("expected 0 interactions from empty HTML, got %d", len(interactions))
	}
}

func TestParseInteractionAnnotations_Single(t *testing.T) {
	html := `<!-- INTERACTION: button.submit → click → validate form → show success message -->
<button class="submit">Submit</button>`
	files := []fileContent{{path: "screen.html", name: "screen.html", content: html}}
	interactions := parseInteractionAnnotations(files)

	if len(interactions) != 1 {
		t.Fatalf("expected 1 interaction, got %d", len(interactions))
	}
	ia := interactions[0]
	if ia.Element != "button.submit" {
		t.Errorf("expected Element=button.submit, got %q", ia.Element)
	}
	if ia.Trigger != "click" {
		t.Errorf("expected Trigger=click, got %q", ia.Trigger)
	}
	if ia.Action != "validate form" {
		t.Errorf("expected Action=validate form, got %q", ia.Action)
	}
	if ia.Result != "show success message" {
		t.Errorf("expected Result=show success message, got %q", ia.Result)
	}
}

func TestParseInteractionAnnotations_MultipleInteractions(t *testing.T) {
	html := `<!-- INTERACTION: button.login → click → authenticate → redirect to dashboard -->
<button>Login</button>
<!-- INTERACTION: input.email → focus → clear error → enable submit -->
<input type="email" />`
	files := []fileContent{{path: "screen.html", name: "screen.html", content: html}}
	interactions := parseInteractionAnnotations(files)

	if len(interactions) != 2 {
		t.Fatalf("expected 2 interactions, got %d", len(interactions))
	}
	if interactions[0].Element != "button.login" {
		t.Errorf("expected first element=button.login")
	}
	if interactions[1].Element != "input.email" {
		t.Errorf("expected second element=input.email")
	}
}

func TestParseInteractionAnnotations_WithTransition(t *testing.T) {
	html := `<!-- INTERACTION: modal.delete → click → close modal → shown hidden | transition: fade-out 300ms -->
<div class="modal">Delete?</div>`
	files := []fileContent{{path: "screen.html", name: "screen.html", content: html}}
	interactions := parseInteractionAnnotations(files)

	if len(interactions) != 1 {
		t.Fatalf("expected 1 interaction, got %d", len(interactions))
	}
	ia := interactions[0]
	if ia.Element != "modal.delete" {
		t.Errorf("expected Element=modal.delete, got %q", ia.Element)
	}
	if ia.Action != "close modal" {
		t.Errorf("expected Action=close modal, got %q", ia.Action)
	}
	if ia.Result != "shown hidden" {
		t.Errorf("expected Result=shown hidden, got %q", ia.Result)
	}
	if ia.Transition != "fade-out 300ms" {
		t.Errorf("expected Transition=fade-out 300ms, got %q", ia.Transition)
	}
}

func TestParseInteractionAnnotations_PartialFields(t *testing.T) {
	// Only element and trigger, no action/result
	html := `<!-- INTERACTION: link.home → click -->
<a href="/">Home</a>`
	files := []fileContent{{path: "screen.html", name: "screen.html", content: html}}
	interactions := parseInteractionAnnotations(files)

	if len(interactions) != 1 {
		t.Fatalf("expected 1 interaction, got %d", len(interactions))
	}
	ia := interactions[0]
	if ia.Element != "link.home" {
		t.Errorf("expected Element=link.home, got %q", ia.Element)
	}
	if ia.Trigger != "click" {
		t.Errorf("expected Trigger=click, got %q", ia.Trigger)
	}
	if ia.Action != "" {
		t.Errorf("expected empty Action, got %q", ia.Action)
	}
	if ia.Result != "" {
		t.Errorf("expected empty Result, got %q", ia.Result)
	}
}

func TestParseInteractionAnnotations_DeduplicateIdentical(t *testing.T) {
	// Exact same raw annotation in two files — should deduplicate
	html1 := `<!-- INTERACTION: button.submit → click → send form → success -->
<button>Submit</button>`
	html2 := `<!-- INTERACTION: button.submit → click → send form → success -->
<button>Submit</button>`
	files := []fileContent{
		{path: "screen-a.html", name: "screen-a.html", content: html1},
		{path: "screen-b.html", name: "screen-b.html", content: html2},
	}
	interactions := parseInteractionAnnotations(files)

	if len(interactions) != 1 {
		t.Errorf("expected 1 deduplicated interaction, got %d", len(interactions))
	}
}

func TestParseInteractionAnnotations_MalformedArrow(t *testing.T) {
	// Wrong separator (dash instead of arrow) — should still parse with empty/partial fields
	html := `<!-- INTERACTION: button - click - send - ok -->
<button>Click</button>`
	files := []fileContent{{path: "screen.html", name: "screen.html", content: html}}
	interactions := parseInteractionAnnotations(files)

	// Should parse as single-part element with no split
	if len(interactions) != 1 {
		t.Fatalf("expected 1 interaction, got %d", len(interactions))
	}
	ia := interactions[0]
	// All in element field since no " → " found
	if ia.Element == "" {
		t.Errorf("expected Element to be populated")
	}
}

func TestParseInteractionAnnotations_SkipsTraceback(t *testing.T) {
	// Should parse without panic on empty result
	html := `<!-- INTERACTION: -->
<div></div>`
	files := []fileContent{{path: "screen.html", name: "screen.html", content: html}}
	interactions := parseInteractionAnnotations(files)

	// Empty INTERACTION comment should still match regex but parse to empty
	if len(interactions) > 1 {
		t.Errorf("expected at most 1 (or 0) interactions, got %d", len(interactions))
	}
}

// Helper: containsString checks if a slice contains a value
func containsString(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}
