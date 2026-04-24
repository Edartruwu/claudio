package app

import (
	"github.com/Abraxas-365/claudio/internal/agents"
	"github.com/Abraxas-365/claudio/internal/plugins"
	"github.com/Abraxas-365/claudio/internal/prompts"
	"github.com/Abraxas-365/claudio/internal/services/skills"
	"github.com/Abraxas-365/claudio/internal/tools"
)

// ApplyAgentExtras merges ExtraSkillsDir and ExtraPluginsDir from the named
// agent definition into registry. Re-wires ToolSearch so it sees any newly
// registered tools. Returns the plugin system-prompt section to append (empty
// when none). Safe to call with agentType == "" — it is a no-op.
func ApplyAgentExtras(registry *tools.Registry, agentType string) string {
	if agentType == "" {
		return ""
	}
	agentDef := agents.GetAgent(agentType)

	// Merge extra skills (additive — global skills remain available)
	if agentDef.ExtraSkillsDir != "" {
		if skillToolRaw, err := registry.Get("Skill"); err == nil {
			if st, ok := skillToolRaw.(*tools.SkillTool); ok {
				mergedReg := skills.NewRegistry()
				for _, s := range st.SkillsRegistry.All() {
					mergedReg.Register(s)
				}
				extraReg := skills.LoadAll("", agentDef.ExtraSkillsDir)
				for _, s := range extraReg.All() {
					mergedReg.Register(s)
				}
				registry.Remove("Skill")
				registry.Register(&tools.SkillTool{
					SkillsRegistry: mergedReg,
					HooksManager:   st.HooksManager,
					ProjectRoot:    st.ProjectRoot,
					ExcludedNames:  st.ExcludedNames,
				})
			}
		}
	}

	// Register extra plugins (additive)
	var pluginSection string
	if agentDef.ExtraPluginsDir != "" {
		extraPluginReg := plugins.NewRegistry()
		extraPluginReg.LoadDir(agentDef.ExtraPluginsDir)
		// Mirror OutputFilterEnabled from existing proxy tools in the registry
		outputFilterEnabled := false
		for _, t := range registry.All() {
			if pt, ok := t.(*plugins.PluginProxyTool); ok {
				outputFilterEnabled = pt.OutputFilterEnabled
				break
			}
		}
		var pluginInfos []prompts.PluginInfo
		for _, p := range extraPluginReg.All() {
			pt := plugins.NewProxyTool(p)
			pt.OutputFilterEnabled = outputFilterEnabled
			registry.Register(pt)
			pluginInfos = append(pluginInfos, prompts.PluginInfo{
				Name:         p.Name,
				Description:  p.Description,
				Instructions: p.Instructions,
			})
		}
		pluginSection = prompts.PluginsSection(pluginInfos)
	}

	// Re-wire ToolSearch so it sees any newly registered agent-specific tools.
	if ts, err := registry.Get("ToolSearch"); err == nil {
		if tst, ok := ts.(*tools.ToolSearchTool); ok {
			tst.SetRegistry(registry)
		}
	}

	return pluginSection
}
