package lua

import (
	lua "github.com/yuin/gopher-lua"
)

// apiRegisterCapability returns the claudio.register_capability(name, tools_table) binding.
//
// Lua usage:
//
//	claudio.register_capability("database", { "SQLQuery", "SchemaInspect" })
func (r *Runtime) apiRegisterCapability(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		name := L.CheckString(1)
		tbl := L.CheckTable(2)

		var toolNames []string
		tbl.ForEach(func(_, v lua.LValue) {
			if s, ok := v.(lua.LString); ok {
				toolNames = append(toolNames, string(s))
			}
		})

		r.pendingCapsMu.Lock()
		r.pendingCaps = append(r.pendingCaps, LuaCapability{Name: name, ToolNames: toolNames})
		r.pendingCapsMu.Unlock()
		return 0
	}
}

// apiAgentCurrent returns the claudio.agent.current() binding.
//
// Lua usage:
//
//	local name = claudio.agent.current()  -- returns string or nil
func (r *Runtime) apiAgentCurrent(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		r.agentMu.RLock()
		name := r.currentAgent
		r.agentMu.RUnlock()
		if name == "" {
			L.Push(lua.LNil)
		} else {
			L.Push(lua.LString(name))
		}
		return 1
	}
}

// apiAgentOnChange returns the claudio.agent.on_change(fn) binding.
//
// Lua usage:
//
//	claudio.agent.on_change(function(new_agent, old_agent) ... end)
func (r *Runtime) apiAgentOnChange(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		fn := L.CheckFunction(1)
		r.agentMu.Lock()
		r.agentChangeHdlrs = append(r.agentChangeHdlrs, luaHandler{plugin: plugin, fn: fn})
		r.agentMu.Unlock()
		return 0
	}
}

// apiAgentAddContext returns the claudio.agent.add_context(str) binding.
//
// Lua usage:
//
//	claudio.agent.add_context("Always prefer tabs over spaces.")
func (r *Runtime) apiAgentAddContext(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		s := L.CheckString(1)
		r.extraContextMu.Lock()
		r.extraContext = append(r.extraContext, s)
		r.extraContextMu.Unlock()
		return 0
	}
}

// apiAgentSetPromptSuffix returns the claudio.agent.set_prompt_suffix(fn) binding.
// Only one suffix function is active at a time; subsequent calls replace the previous one.
//
// Lua usage:
//
//	claudio.agent.set_prompt_suffix(function(agent_name) return "" end)
func (r *Runtime) apiAgentSetPromptSuffix(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		fn := L.CheckFunction(1)
		r.promptSuffixMu.Lock()
		r.promptSuffixHdlr = &luaHandler{plugin: plugin, fn: fn}
		r.promptSuffixMu.Unlock()
		return 0
	}
}
