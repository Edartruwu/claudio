package lua

import (
	"github.com/Abraxas-365/claudio/internal/services/skills"
	lua "github.com/yuin/gopher-lua"
)

// apiRegisterSkill returns the claudio.register_skill(tbl) binding.
//
// Lua usage:
//
//	claudio.register_skill({
//	  name         = "my-skill",
//	  description  = "Does something useful",
//	  content      = "# My Skill\n\nDo X then Y...",
//	  capabilities = {"cap1", "cap2"},  -- optional
//	})
func (r *Runtime) apiRegisterSkill(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		tbl := L.CheckTable(1)
		name := tableString(L, tbl, "name")
		desc := tableString(L, tbl, "description")
		content := tableString(L, tbl, "content")
		caps := tableStringSlice(L, tbl, "capabilities")

		if name == "" {
			L.ArgError(1, "register_skill: name required")
			return 0
		}

		skill := &skills.Skill{
			Name:         name,
			Description:  desc,
			Content:      content,
			Source:       "plugin:" + plugin.name,
			Capabilities: caps,
		}
		r.skills.Register(skill)
		return 0
	}
}
