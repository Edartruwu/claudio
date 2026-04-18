package agents

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// joinStrings
// ---------------------------------------------------------------------------

func TestJoinStrings_Empty(t *testing.T) {
	if got := joinStrings(nil); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestJoinStrings_Single(t *testing.T) {
	if got := joinStrings([]string{"hello"}); got != "hello" {
		t.Errorf("expected %q, got %q", "hello", got)
	}
}

func TestJoinStrings_Multiple(t *testing.T) {
	got := joinStrings([]string{"a", "b", "c"})
	want := "a, b, c"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// ---------------------------------------------------------------------------
// sanitizeAgentName
// ---------------------------------------------------------------------------

func TestSanitizeAgentName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello World", "hello-world"},
		{"My Agent", "my-agent"},
		{"  spaces  ", "--spaces--"},
		{"special!@#chars", "specialchars"},
		{"UPPERCASE", "uppercase"},
		{"with_underscore", "with_underscore"},
		{"numbers123", "numbers123"},
		{"", ""},
		{strings.Repeat("a", 60), strings.Repeat("a", 50)}, // truncated to 50
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := sanitizeAgentName(tc.input)
			if got != tc.want {
				t.Errorf("sanitizeAgentName(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestSanitizeAgentName_MaxLen50(t *testing.T) {
	long := strings.Repeat("x", 100)
	got := sanitizeAgentName(long)
	if len(got) > 50 {
		t.Errorf("expected len <= 50, got %d", len(got))
	}
}

// ---------------------------------------------------------------------------
// BuiltInAgents
// ---------------------------------------------------------------------------

func TestBuiltInAgents_Count(t *testing.T) {
	agents := BuiltInAgents()
	if len(agents) != 5 {
		t.Errorf("expected 5 built-in agents, got %d", len(agents))
	}
}

func TestBuiltInAgents_Types(t *testing.T) {
	agents := BuiltInAgents()
	types := make(map[string]bool)
	for _, a := range agents {
		types[a.Type] = true
	}
	for _, expected := range []string{"general-purpose", "Explore", "Plan", "verification", "design"} {
		if !types[expected] {
			t.Errorf("expected agent type %q not found", expected)
		}
	}
}

func TestBuiltInAgents_NoEmptyTypes(t *testing.T) {
	for _, a := range BuiltInAgents() {
		if a.Type == "" {
			t.Error("built-in agent has empty Type")
		}
	}
}

func TestBuiltInAgents_NoEmptySystemPrompts(t *testing.T) {
	for _, a := range BuiltInAgents() {
		if a.SystemPrompt == "" {
			t.Errorf("agent %q has empty SystemPrompt", a.Type)
		}
	}
}

// ---------------------------------------------------------------------------
// GeneralPurposeAgent
// ---------------------------------------------------------------------------

func TestGeneralPurposeAgent(t *testing.T) {
	a := GeneralPurposeAgent()
	if a.Type != "general-purpose" {
		t.Errorf("expected type %q, got %q", "general-purpose", a.Type)
	}
	if a.MaxTurns != 50 {
		t.Errorf("expected MaxTurns=50, got %d", a.MaxTurns)
	}
	if len(a.Tools) != 1 || a.Tools[0] != "*" {
		t.Errorf("expected Tools=[*], got %v", a.Tools)
	}
	if a.ReadOnly {
		t.Error("general-purpose agent should not be read-only")
	}
	if a.WhenToUse == "" {
		t.Error("WhenToUse should not be empty")
	}
}

// ---------------------------------------------------------------------------
// ExploreAgent
// ---------------------------------------------------------------------------

func TestExploreAgent(t *testing.T) {
	a := ExploreAgent()
	if a.Type != "Explore" {
		t.Errorf("expected type %q, got %q", "Explore", a.Type)
	}
	if a.MaxTurns != 25 {
		t.Errorf("expected MaxTurns=25, got %d", a.MaxTurns)
	}
	if !a.ReadOnly {
		t.Error("Explore agent should be read-only")
	}
	if a.Model != "haiku" {
		t.Errorf("expected Model=haiku, got %q", a.Model)
	}
	if len(a.DisallowedTools) == 0 {
		t.Error("Explore agent should have disallowed tools")
	}
}

// ---------------------------------------------------------------------------
// PlanAgent
// ---------------------------------------------------------------------------

func TestPlanAgent(t *testing.T) {
	a := PlanAgent()
	if a.Type != "Plan" {
		t.Errorf("expected type %q, got %q", "Plan", a.Type)
	}
	if a.MaxTurns != 30 {
		t.Errorf("expected MaxTurns=30, got %d", a.MaxTurns)
	}
	if !a.ReadOnly {
		t.Error("Plan agent should be read-only")
	}
	if len(a.DisallowedTools) == 0 {
		t.Error("Plan agent should have disallowed tools")
	}
}

// ---------------------------------------------------------------------------
// VerificationAgent
// ---------------------------------------------------------------------------

func TestVerificationAgent(t *testing.T) {
	a := VerificationAgent()
	if a.Type != "verification" {
		t.Errorf("expected type %q, got %q", "verification", a.Type)
	}
	if a.MaxTurns != 20 {
		t.Errorf("expected MaxTurns=20, got %d", a.MaxTurns)
	}
	if !a.ReadOnly {
		t.Error("verification agent should be read-only")
	}
	if len(a.DisallowedTools) == 0 {
		t.Error("verification agent should have disallowed tools")
	}
}

// ---------------------------------------------------------------------------
// GetAgent
// ---------------------------------------------------------------------------

func TestGetAgent_KnownTypes(t *testing.T) {
	// Reset custom dirs so we only get built-ins.
	SetCustomDirs()

	tests := []string{"general-purpose", "Explore", "Plan", "verification"}
	for _, typ := range tests {
		t.Run(typ, func(t *testing.T) {
			a := GetAgent(typ)
			if a.Type != typ {
				t.Errorf("GetAgent(%q) returned type %q", typ, a.Type)
			}
		})
	}
}

func TestGetAgent_Unknown_FallsBackToGeneralPurpose(t *testing.T) {
	SetCustomDirs()
	a := GetAgent("totally-unknown-type-xyz")
	if a.Type != "general-purpose" {
		t.Errorf("expected fallback to general-purpose, got %q", a.Type)
	}
}

// ---------------------------------------------------------------------------
// AgentTypesList
// ---------------------------------------------------------------------------

func TestAgentTypesList_ContainsBuiltInTypes(t *testing.T) {
	SetCustomDirs()
	list := AgentTypesList()
	for _, typ := range []string{"general-purpose", "Explore", "Plan", "verification"} {
		if !strings.Contains(list, typ) {
			t.Errorf("AgentTypesList() missing type %q", typ)
		}
	}
}

func TestAgentTypesList_ContainsWhenToUse(t *testing.T) {
	SetCustomDirs()
	list := AgentTypesList()
	if list == "" {
		t.Error("AgentTypesList() returned empty string")
	}
	// Each line should have a colon separator between type and WhenToUse.
	if !strings.Contains(list, ":") {
		t.Error("AgentTypesList() lines should contain ':' separator")
	}
}

func TestAgentTypesList_ToolsLabel(t *testing.T) {
	SetCustomDirs()
	list := AgentTypesList()
	if !strings.Contains(list, "Tools:") {
		t.Error("AgentTypesList() should contain 'Tools:' label")
	}
}

// ---------------------------------------------------------------------------
// LoadCustomAgents
// ---------------------------------------------------------------------------

func TestLoadCustomAgents_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	result := LoadCustomAgents(dir)
	if len(result) != 0 {
		t.Errorf("expected 0 agents from empty dir, got %d", len(result))
	}
}

func TestLoadCustomAgents_NonExistentDir(t *testing.T) {
	result := LoadCustomAgents("/this/path/does/not/exist")
	if len(result) != 0 {
		t.Errorf("expected 0 agents from non-existent dir, got %d", len(result))
	}
}

func TestLoadCustomAgents_ValidAgentFile(t *testing.T) {
	dir := t.TempDir()
	content := `---
description: A custom test agent
tools: ["BashTool", "ReadFile"]
---

You are a custom test agent. Do the things.`
	if err := os.WriteFile(filepath.Join(dir, "custom-agent.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result := LoadCustomAgents(dir)
	if len(result) != 1 {
		t.Fatalf("expected 1 custom agent, got %d", len(result))
	}
	a := result[0]
	if a.Type != "custom-agent" {
		t.Errorf("expected type %q, got %q", "custom-agent", a.Type)
	}
	if a.WhenToUse != "A custom test agent" {
		t.Errorf("expected WhenToUse %q, got %q", "A custom test agent", a.WhenToUse)
	}
	if !strings.Contains(a.SystemPrompt, "custom test agent") {
		t.Errorf("SystemPrompt should contain body text, got %q", a.SystemPrompt)
	}
}

func TestLoadCustomAgents_MissingBody_Skipped(t *testing.T) {
	dir := t.TempDir()
	// Frontmatter only, no body.
	content := "---\ndescription: No body\n---\n"
	if err := os.WriteFile(filepath.Join(dir, "nobody.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result := LoadCustomAgents(dir)
	if len(result) != 0 {
		t.Errorf("agent with empty body should be skipped, got %d agents", len(result))
	}
}

func TestLoadCustomAgents_NonMdFileIgnored(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agent.txt"), []byte("some content"), 0644); err != nil {
		t.Fatal(err)
	}
	result := LoadCustomAgents(dir)
	if len(result) != 0 {
		t.Errorf("non-.md file should be ignored, got %d agents", len(result))
	}
}

func TestLoadCustomAgents_DefaultToolsWildcard(t *testing.T) {
	dir := t.TempDir()
	// No "tools" in frontmatter → should default to ["*"]
	content := "---\ndescription: Defaults agent\n---\n\nDo stuff."
	if err := os.WriteFile(filepath.Join(dir, "defaults.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result := LoadCustomAgents(dir)
	if len(result) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(result))
	}
	if len(result[0].Tools) != 1 || result[0].Tools[0] != "*" {
		t.Errorf("expected default Tools=[*], got %v", result[0].Tools)
	}
}

func TestLoadCustomAgents_DisallowedTools(t *testing.T) {
	dir := t.TempDir()
	content := `---
description: restricted agent
disallowedTools: ["Edit", "Write"]
---

You are restricted.`
	if err := os.WriteFile(filepath.Join(dir, "restricted.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result := LoadCustomAgents(dir)
	if len(result) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(result))
	}
	if len(result[0].DisallowedTools) != 2 {
		t.Errorf("expected 2 disallowed tools, got %v", result[0].DisallowedTools)
	}
}

func TestLoadCustomAgents_WithMemoryDir(t *testing.T) {
	dir := t.TempDir()

	// Create agent file
	content := "---\ndescription: Memory agent\n---\n\nI remember things."
	if err := os.WriteFile(filepath.Join(dir, "mem-agent.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Create memory directory
	memDir := filepath.Join(dir, "mem-agent", "memory")
	if err := os.MkdirAll(memDir, 0755); err != nil {
		t.Fatal(err)
	}

	result := LoadCustomAgents(dir)
	if len(result) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(result))
	}
	if result[0].MemoryDir != memDir {
		t.Errorf("expected MemoryDir=%q, got %q", memDir, result[0].MemoryDir)
	}
}

func TestLoadCustomAgents_SourceSessionAndProject(t *testing.T) {
	dir := t.TempDir()
	content := `---
description: session agent
sourceSession: sess-123
sourceProject: proj-abc
---

I have a source session.`
	if err := os.WriteFile(filepath.Join(dir, "session-agent.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result := LoadCustomAgents(dir)
	if len(result) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(result))
	}
	if result[0].SourceSession != "sess-123" {
		t.Errorf("expected SourceSession=sess-123, got %q", result[0].SourceSession)
	}
	if result[0].SourceProject != "proj-abc" {
		t.Errorf("expected SourceProject=proj-abc, got %q", result[0].SourceProject)
	}
}

func TestLoadCustomAgents_SubdirIgnored(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "subdir")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatal(err)
	}
	// Directories should be ignored even with .md-like names
	result := LoadCustomAgents(dir)
	if len(result) != 0 {
		t.Errorf("subdirectory should be ignored, got %d agents", len(result))
	}
}

// ---------------------------------------------------------------------------
// AllAgents
// ---------------------------------------------------------------------------

func TestAllAgents_IncludesBuiltIns(t *testing.T) {
	agents := AllAgents()
	if len(agents) < 4 {
		t.Errorf("expected at least 4 agents, got %d", len(agents))
	}
}

func TestAllAgents_CustomOverridesBuiltIn(t *testing.T) {
	dir := t.TempDir()
	// Override the "Plan" agent
	content := "---\ndescription: Overridden planner\n---\n\nCustom plan logic."
	if err := os.WriteFile(filepath.Join(dir, "Plan.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	all := AllAgents(dir)
	for _, a := range all {
		if a.Type == "Plan" {
			if a.WhenToUse != "Overridden planner" {
				t.Errorf("custom agent should override built-in: WhenToUse=%q", a.WhenToUse)
			}
			return
		}
	}
	t.Error("expected to find Plan agent")
}

func TestAllAgents_CustomAddedIfNew(t *testing.T) {
	dir := t.TempDir()
	content := "---\ndescription: Brand new agent\n---\n\nDoes new things."
	if err := os.WriteFile(filepath.Join(dir, "brand-new.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	all := AllAgents(dir)
	found := false
	for _, a := range all {
		if a.Type == "brand-new" {
			found = true
			break
		}
	}
	if !found {
		t.Error("custom agent 'brand-new' should be present in AllAgents")
	}
}

func TestAllAgents_BuiltInsAppearFirst(t *testing.T) {
	all := AllAgents()
	builtIn := BuiltInAgents()
	if len(all) < len(builtIn) {
		t.Fatal("AllAgents returned fewer agents than BuiltInAgents")
	}
	for i, a := range builtIn {
		if all[i].Type != a.Type {
			t.Errorf("position %d: expected built-in type %q, got %q", i, a.Type, all[i].Type)
		}
	}
}

// ---------------------------------------------------------------------------
// SetCustomDirs / getCustomDirs
// ---------------------------------------------------------------------------

func TestSetCustomDirs(t *testing.T) {
	SetCustomDirs("/tmp/dir1", "/tmp/dir2")
	dirs := getCustomDirs()
	if len(dirs) != 2 {
		t.Fatalf("expected 2 dirs, got %d", len(dirs))
	}
	if dirs[0] != "/tmp/dir1" || dirs[1] != "/tmp/dir2" {
		t.Errorf("unexpected dirs: %v", dirs)
	}
	// Reset
	SetCustomDirs()
	dirs = getCustomDirs()
	if len(dirs) != 0 {
		t.Errorf("expected 0 dirs after reset, got %d", len(dirs))
	}
}

func TestSetCustomDirs_Concurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			SetCustomDirs("/tmp/concurrent")
			_ = getCustomDirs()
		}()
	}
	wg.Wait()
	SetCustomDirs() // cleanup
}

// ---------------------------------------------------------------------------
// AgentDefinition zero value
// ---------------------------------------------------------------------------

func TestAgentDefinition_ZeroValue(t *testing.T) {
	var a AgentDefinition
	if a.Type != "" {
		t.Error("Type should be empty")
	}
	if a.MaxTurns != 0 {
		t.Error("MaxTurns should be 0")
	}
	if a.ReadOnly {
		t.Error("ReadOnly should be false")
	}
	if len(a.Tools) != 0 {
		t.Error("Tools should be nil/empty")
	}
	if len(a.DisallowedTools) != 0 {
		t.Error("DisallowedTools should be nil/empty")
	}
}
