package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Abraxas-365/claudio/internal/services/skills"
	"github.com/Abraxas-365/claudio/internal/tools"
)

// buildRegistry returns a registry with the given skill names pre-registered.
func buildRegistry(names ...string) *skills.Registry {
	r := skills.NewRegistry()
	for _, n := range names {
		r.Register(&skills.Skill{
			Name:        n,
			Description: n + " description",
			Content:     n + " content",
		})
	}
	return r
}

func newSkillTool(reg *skills.Registry, excluded ...string) *tools.SkillTool {
	return &tools.SkillTool{
		SkillsRegistry: reg,
		ExcludedNames:  excluded,
	}
}

// --- formatSkillList / Description visibility ---

func TestSkillTool_ExcludedNames_HiddenFromDescription(t *testing.T) {
	reg := buildRegistry("caveman", "commit", "review")
	st := newSkillTool(reg, "caveman")

	desc := st.Description()
	if strings.Contains(desc, "caveman") {
		t.Errorf("Description should not contain excluded skill 'caveman', got:\n%s", desc)
	}
	if !strings.Contains(desc, "commit") {
		t.Errorf("Description should contain non-excluded skill 'commit'")
	}
	if !strings.Contains(desc, "review") {
		t.Errorf("Description should contain non-excluded skill 'review'")
	}
}

func TestSkillTool_ExcludedNames_CacheStable(t *testing.T) {
	// Description() must return the same string on repeated calls (cache must not bust).
	reg := buildRegistry("caveman", "commit")
	st := newSkillTool(reg, "caveman")

	first := st.Description()
	second := st.Description()
	if first != second {
		t.Errorf("Description() cache not stable: got different values across calls")
	}
}

func TestSkillTool_ExcludedNames_CaseInsensitive(t *testing.T) {
	reg := buildRegistry("Caveman", "commit")
	// Exclude with different casing — should still hide it.
	st := newSkillTool(reg, "caveman")

	desc := st.Description()
	if strings.Contains(strings.ToLower(desc), "caveman") {
		t.Errorf("Description should not contain excluded skill regardless of case")
	}
}

func TestSkillTool_EmptyExcludes_AllSkillsVisible(t *testing.T) {
	reg := buildRegistry("caveman", "commit", "review")
	st := newSkillTool(reg) // no exclusions

	desc := st.Description()
	for _, name := range []string{"caveman", "commit", "review"} {
		if !strings.Contains(desc, name) {
			t.Errorf("Description should contain %q when no exclusions set", name)
		}
	}
}

// --- Execute path ---

func TestSkillTool_ExcludedNames_BlocksExecution(t *testing.T) {
	reg := buildRegistry("caveman", "commit")
	st := newSkillTool(reg, "caveman")

	input, _ := json.Marshal(map[string]string{"skill": "caveman"})
	res, err := st.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Errorf("Execute should return an error result for excluded skill 'caveman'")
	}
	// Error message must say "not found".
	if !strings.Contains(res.Content, "not found") {
		t.Errorf("expected 'not found' in error, got: %s", res.Content)
	}
	// The "Available skills:" list must not include caveman — only non-excluded skills.
	if idx := strings.Index(res.Content, "Available skills:"); idx != -1 {
		availablePart := res.Content[idx:]
		if strings.Contains(availablePart, "caveman") {
			t.Errorf("available skills list should not include excluded 'caveman', got: %s", availablePart)
		}
	}
}

func TestSkillTool_ExcludedNames_BlocksExecution_CaseInsensitive(t *testing.T) {
	reg := buildRegistry("caveman", "commit")
	st := newSkillTool(reg, "caveman")

	// Try invoking with different casing.
	input, _ := json.Marshal(map[string]string{"skill": "Caveman"})
	res, err := st.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Errorf("Execute should block excluded skill regardless of invocation casing")
	}
}

func TestSkillTool_ExcludedNames_NonExcludedSkillExecutes(t *testing.T) {
	reg := buildRegistry("caveman", "commit")
	st := newSkillTool(reg, "caveman")

	input, _ := json.Marshal(map[string]string{"skill": "commit"})
	res, err := st.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Errorf("Execute should succeed for non-excluded skill 'commit', got: %s", res.Content)
	}
}

// --- Injection idempotency: caveman content injected once via system prompt ---

func TestSkillTool_ExcludedNames_NoDoubleInjection(t *testing.T) {
	// When caveman is excluded from SkillTool, calling Skill("caveman") fails,
	// so caveman content is only injected once (via system prompt wiring), not twice.
	reg := buildRegistry("caveman", "commit")
	st := newSkillTool(reg, "caveman")

	input, _ := json.Marshal(map[string]string{"skill": "caveman"})
	res, _ := st.Execute(context.Background(), input)

	// InjectedMessages must be empty — caveman must not be re-injected via skill call.
	if len(res.InjectedMessages) > 0 {
		t.Errorf("excluded skill must not inject messages, got %d injected messages", len(res.InjectedMessages))
	}
}

// --- Available names helper (used in error messages) ---

func TestSkillTool_ExcludedNames_NotInAvailableNamesError(t *testing.T) {
	reg := buildRegistry("caveman", "commit", "review")
	st := newSkillTool(reg, "caveman")

	// Trigger the "not found" error path by requesting a non-existent skill.
	input, _ := json.Marshal(map[string]string{"skill": "nonexistent"})
	res, _ := st.Execute(context.Background(), input)

	if strings.Contains(res.Content, "caveman") {
		t.Errorf("available names in error message should not include excluded skill, got: %s", res.Content)
	}
	if !strings.Contains(res.Content, "commit") {
		t.Errorf("available names should still list non-excluded skills, got: %s", res.Content)
	}
}
