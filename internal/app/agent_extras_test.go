package app

import (
	"testing"

	"github.com/Abraxas-365/claudio/internal/services/skills"
	"github.com/Abraxas-365/claudio/internal/tools"
)

func buildRegistryWithSkillTool(t *testing.T) *tools.Registry {
	t.Helper()
	reg := tools.NewRegistry()
	reg.Register(&tools.SkillTool{
		SkillsRegistry: skills.NewRegistry(),
	})
	return reg
}

// TestApplyAgentExtras_EmptyType — no-op, returns empty string, registry unchanged.
func TestApplyAgentExtras_EmptyType(t *testing.T) {
	reg := buildRegistryWithSkillTool(t)
	before := len(reg.All())
	if got := ApplyAgentExtras(reg, ""); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
	if len(reg.All()) != before {
		t.Errorf("registry mutated on empty agentType")
	}
}

// TestApplyAgentExtras_NoExtrasDir — known agent with no ExtraSkillsDir/ExtraPluginsDir.
// general-purpose always exists and has no extra dirs.
func TestApplyAgentExtras_NoExtrasDir(t *testing.T) {
	reg := buildRegistryWithSkillTool(t)
	before := len(reg.All())
	if got := ApplyAgentExtras(reg, "general-purpose"); got != "" {
		t.Errorf("expected no plugin section for agent with no extras, got %q", got)
	}
	if len(reg.All()) != before {
		t.Errorf("registry mutated for agent with no extras dirs")
	}
}

// TestApplyAgentExtras_NoSkillTool — registry without SkillTool does not panic.
func TestApplyAgentExtras_NoSkillTool(t *testing.T) {
	reg := tools.NewRegistry() // no SkillTool
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("panicked with no SkillTool: %v", r)
		}
	}()
	ApplyAgentExtras(reg, "general-purpose")
}

// TestApplyAgentExtras_PreservesExistingSkills — global skills survive the merge.
func TestApplyAgentExtras_PreservesExistingSkills(t *testing.T) {
	globalReg := skills.NewRegistry()
	globalReg.Register(&skills.Skill{Name: "global-skill", Description: "global"})
	reg := tools.NewRegistry()
	reg.Register(&tools.SkillTool{SkillsRegistry: globalReg})

	// general-purpose has no ExtraSkillsDir so no merge occurs,
	// but the SkillTool must still be intact afterwards.
	ApplyAgentExtras(reg, "general-purpose")

	st, err := reg.Get("Skill")
	if err != nil {
		t.Fatalf("SkillTool missing after ApplyAgentExtras: %v", err)
	}
	skillTool, ok := st.(*tools.SkillTool)
	if !ok {
		t.Fatalf("unexpected Skill tool type %T", st)
	}
	if _, ok := skillTool.SkillsRegistry.Get("global-skill"); !ok {
		t.Errorf("global skill dropped after ApplyAgentExtras")
	}
}
