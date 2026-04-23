package teams

import (
	"os"
	"path/filepath"
	"testing"
)

// Helper: write test template JSON
func writeTemplate(t *testing.T, dir, name string, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, name+".json")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// TestLoadTemplates_SingleDir: single dir with templates loads all
func TestLoadTemplates_SingleDir(t *testing.T) {
	dir := t.TempDir()
	writeTemplate(t, dir, "team-a", `{"name":"team-a","description":"Team A","members":[]}`)
	writeTemplate(t, dir, "team-b", `{"name":"team-b","description":"Team B","members":[]}`)

	result := LoadTemplates(dir)
	if len(result) != 2 {
		t.Fatalf("want 2 templates, got %d", len(result))
	}
	names := map[string]bool{result[0].Name: true, result[1].Name: true}
	if !names["team-a"] || !names["team-b"] {
		t.Errorf("want team-a and team-b, got %v", names)
	}
}

// TestLoadTemplates_MultipleDirs: multiple dirs, first-wins on collision
func TestLoadTemplates_MultipleDirs(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	writeTemplate(t, dir1, "shared", `{"name":"shared","description":"From dir1","members":[]}`)
	writeTemplate(t, dir1, "only-in-1", `{"name":"only-in-1","description":"Dir1 only","members":[]}`)
	writeTemplate(t, dir2, "shared", `{"name":"shared","description":"From dir2","members":[]}`)
	writeTemplate(t, dir2, "only-in-2", `{"name":"only-in-2","description":"Dir2 only","members":[]}`)

	result := LoadTemplates(dir1, dir2)
	if len(result) != 3 {
		t.Fatalf("want 3 templates, got %d", len(result))
	}

	// Find "shared", verify it came from dir1
	var shared *TeamTemplate
	for i := range result {
		if result[i].Name == "shared" {
			shared = &result[i]
			break
		}
	}
	if shared == nil {
		t.Fatal("want 'shared' template")
	}
	if shared.Description != "From dir1" {
		t.Errorf("want 'From dir1', got %q", shared.Description)
	}

	// Verify all unique names present
	names := make(map[string]bool)
	for _, tm := range result {
		names[tm.Name] = true
	}
	if !names["shared"] || !names["only-in-1"] || !names["only-in-2"] {
		t.Errorf("want shared, only-in-1, only-in-2; got %v", names)
	}
}

// TestLoadTemplates_EmptyDir: empty dir returns empty
func TestLoadTemplates_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	result := LoadTemplates(dir)
	if len(result) != 0 {
		t.Fatalf("want 0, got %d", len(result))
	}
}

// TestLoadTemplates_NonexistentDir: missing dir skipped, no error
func TestLoadTemplates_NonexistentDir(t *testing.T) {
	result := LoadTemplates("/nonexistent/path/to/templates")
	if len(result) != 0 {
		t.Fatalf("want 0, got %d", len(result))
	}
}

// TestLoadTemplates_InvalidJSON: bad JSON skipped gracefully
func TestLoadTemplates_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	writeTemplate(t, dir, "bad", `{invalid json`)
	writeTemplate(t, dir, "good", `{"name":"good","description":"OK","members":[]}`)

	result := LoadTemplates(dir)
	if len(result) != 1 {
		t.Fatalf("want 1 (bad skipped), got %d", len(result))
	}
	if result[0].Name != "good" {
		t.Errorf("want 'good', got %q", result[0].Name)
	}
}

// TestLoadTemplates_NoArgs: zero dirs returns empty
func TestLoadTemplates_NoArgs(t *testing.T) {
	result := LoadTemplates()
	if len(result) != 0 {
		t.Fatalf("want 0, got %d", len(result))
	}
}

// TestLoadTemplates_EmptyStringArg: empty string skipped
func TestLoadTemplates_EmptyStringArg(t *testing.T) {
	dir := t.TempDir()
	writeTemplate(t, dir, "team", `{"name":"team","description":"T","members":[]}`)
	result := LoadTemplates("", dir, "")
	if len(result) != 1 {
		t.Fatalf("want 1, got %d", len(result))
	}
}

// TestGetTemplate_FoundInPrimary: found in primary dir
func TestGetTemplate_FoundInPrimary(t *testing.T) {
	dir := t.TempDir()
	writeTemplate(t, dir, "test-team", `{"name":"test-team","description":"Primary","members":[]}`)

	result, err := GetTemplate(dir, "test-team")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "test-team" {
		t.Errorf("want 'test-team', got %q", result.Name)
	}
	if result.Description != "Primary" {
		t.Errorf("want 'Primary', got %q", result.Description)
	}
}

// TestGetTemplate_NotFoundAnywhere: not found returns error
func TestGetTemplate_NotFoundAnywhere(t *testing.T) {
	dir := t.TempDir()
	_, err := GetTemplate(dir, "nonexistent")
	if err == nil {
		t.Fatal("want error, got nil")
	}
}

// TestGetTemplate_NotInPrimary_FoundInExtra: not in primary, found in extra
func TestGetTemplate_NotInPrimary_FoundInExtra(t *testing.T) {
	dirPrimary := t.TempDir()
	dirExtra := t.TempDir()
	writeTemplate(t, dirExtra, "extra-team", `{"name":"extra-team","description":"From extra","members":[]}`)

	result, err := GetTemplate(dirPrimary, "extra-team", dirExtra)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "extra-team" {
		t.Errorf("want 'extra-team', got %q", result.Name)
	}
	if result.Description != "From extra" {
		t.Errorf("want 'From extra', got %q", result.Description)
	}
}

// TestGetTemplate_FoundInBoth_PrimaryWins: found in both, primary wins
func TestGetTemplate_FoundInBoth_PrimaryWins(t *testing.T) {
	dirPrimary := t.TempDir()
	dirExtra := t.TempDir()
	writeTemplate(t, dirPrimary, "shared", `{"name":"shared","description":"From primary","members":[]}`)
	writeTemplate(t, dirExtra, "shared", `{"name":"shared","description":"From extra","members":[]}`)

	result, err := GetTemplate(dirPrimary, "shared", dirExtra)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Description != "From primary" {
		t.Errorf("want 'From primary', got %q", result.Description)
	}
}

// TestGetTemplate_MultipleExtraDirs: searches extra dirs in order
func TestGetTemplate_MultipleExtraDirs(t *testing.T) {
	dirPrimary := t.TempDir()
	dirExtra1 := t.TempDir()
	dirExtra2 := t.TempDir()
	writeTemplate(t, dirExtra1, "team-x", `{"name":"team-x","description":"From extra1","members":[]}`)
	writeTemplate(t, dirExtra2, "team-y", `{"name":"team-y","description":"From extra2","members":[]}`)

	resultX, err := GetTemplate(dirPrimary, "team-x", dirExtra1, dirExtra2)
	if err != nil {
		t.Fatalf("unexpected error for team-x: %v", err)
	}
	if resultX.Description != "From extra1" {
		t.Errorf("want 'From extra1', got %q", resultX.Description)
	}

	resultY, err := GetTemplate(dirPrimary, "team-y", dirExtra1, dirExtra2)
	if err != nil {
		t.Fatalf("unexpected error for team-y: %v", err)
	}
	if resultY.Description != "From extra2" {
		t.Errorf("want 'From extra2', got %q", resultY.Description)
	}
}

// TestGetTemplate_EmptyPrimaryDir: primary dir empty, found in extra
func TestGetTemplate_EmptyPrimaryDir(t *testing.T) {
	dirPrimary := t.TempDir()
	dirExtra := t.TempDir()
	writeTemplate(t, dirExtra, "team", `{"name":"team","description":"Extra team","members":[]}`)

	result, err := GetTemplate(dirPrimary, "team", dirExtra)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "team" {
		t.Errorf("want 'team', got %q", result.Name)
	}
}

// TestGetTemplate_InvalidJSON: bad JSON returns error
func TestGetTemplate_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	writeTemplate(t, dir, "bad", `{not json}`)

	_, err := GetTemplate(dir, "bad")
	if err == nil {
		t.Fatal("want error for invalid JSON, got nil")
	}
}

// TestGetTemplate_EmptyStringDir: empty string skipped
func TestGetTemplate_EmptyStringDir(t *testing.T) {
	dirExtra := t.TempDir()
	writeTemplate(t, dirExtra, "team", `{"name":"team","description":"T","members":[]}`)

	result, err := GetTemplate("", "team", dirExtra)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "team" {
		t.Errorf("want 'team', got %q", result.Name)
	}
}

// TestGetTemplate_NameNotInFile: name inferred from filename
func TestGetTemplate_NameNotInFile(t *testing.T) {
	dir := t.TempDir()
	writeTemplate(t, dir, "inferred", `{"description":"No name field","members":[]}`)

	result, err := GetTemplate(dir, "inferred")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "inferred" {
		t.Errorf("want 'inferred', got %q", result.Name)
	}
}

// TestGetTemplate_ComplexMembers: template with members and advisor config
func TestGetTemplate_ComplexMembers(t *testing.T) {
	dir := t.TempDir()
	content := `{
  "name": "complex-team",
  "description": "Team with members",
  "model": "sonnet",
  "autoCompactThreshold": 80,
  "members": [
    {
      "name": "backend-eng",
      "subagent_type": "backend-mid",
      "model": "sonnet",
      "autoCompactThreshold": 75,
      "advisor": {
        "subagent_type": "backend-senior",
        "model": "opus",
        "max_uses": 5
      }
    }
  ]
}`
	writeTemplate(t, dir, "complex-team", content)

	result, err := GetTemplate(dir, "complex-team")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Members) != 1 {
		t.Fatalf("want 1 member, got %d", len(result.Members))
	}
	m := result.Members[0]
	if m.Name != "backend-eng" {
		t.Errorf("want name 'backend-eng', got %q", m.Name)
	}
	if m.Model != "sonnet" {
		t.Errorf("want model 'sonnet', got %q", m.Model)
	}
	if m.AutoCompactThreshold != 75 {
		t.Errorf("want threshold 75, got %d", m.AutoCompactThreshold)
	}
	if m.Advisor == nil {
		t.Fatal("want advisor, got nil")
	}
	if m.Advisor.SubagentType != "backend-senior" {
		t.Errorf("want advisor type 'backend-senior', got %q", m.Advisor.SubagentType)
	}
	if m.Advisor.MaxUses != 5 {
		t.Errorf("want max_uses 5, got %d", m.Advisor.MaxUses)
	}
}
