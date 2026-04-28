package lua

import (
	"context"
	"log"

	lua "github.com/yuin/gopher-lua"
)

// apiAIRun implements claudio.ai.run(opts, callback).
//
// Lua usage:
//
//	claudio.ai.run({
//	  system = "You are a summarizer",
//	  user   = "Summarize this conversation",
//	  model  = "claude-haiku-4-5-20251001",  -- optional
//	}, function(result, err) end)
func (r *Runtime) apiAIRun(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		opts := L.CheckTable(1)
		fn := L.CheckFunction(2)

		system := luaStringField(opts, "system")
		user := luaStringField(opts, "user")
		model := luaStringField(opts, "model")

		r.runAICallMu.RLock()
		cb := r.runAICall
		r.runAICallMu.RUnlock()

		if cb == nil {
			callLuaCallback(plugin, fn, "", "claudio.ai.run: not available")
			return 0
		}

		go func() {
			result, err := cb(context.Background(), system, user, model)
			errStr := ""
			if err != nil {
				errStr = err.Error()
			}
			callLuaCallback(plugin, fn, result, errStr)
		}()

		return 0
	}
}

// apiAgentSpawn implements claudio.agent.spawn(opts, callback).
//
// Lua usage:
//
//	claudio.agent.spawn({
//	  prompt    = "Investigate the codebase",
//	  system    = "You are a code investigator",  -- optional
//	  model     = "claude-haiku-4-5-20251001",    -- optional
//	  max_turns = 10,                              -- optional
//	  tools     = {"Bash", "Read", "Glob"},        -- optional, nil = all tools
//	}, function(result, err) end)
func (r *Runtime) apiAgentSpawn(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		opts := L.CheckTable(1)
		fn := L.CheckFunction(2)

		prompt := luaStringField(opts, "prompt")
		system := luaStringField(opts, "system")
		model := luaStringField(opts, "model")
		maxTurns := int(luaNumberField(opts, "max_turns"))

		var allowedTools []string
		if toolsVal := opts.RawGetString("tools"); toolsVal != lua.LNil {
			if toolsTbl, ok := toolsVal.(*lua.LTable); ok {
				toolsTbl.ForEach(func(_, v lua.LValue) {
					if s, ok := v.(lua.LString); ok {
						allowedTools = append(allowedTools, string(s))
					}
				})
			}
		}

		r.runAgentCallMu.RLock()
		cb := r.runAgentCall
		r.runAgentCallMu.RUnlock()

		if cb == nil {
			callLuaCallback(plugin, fn, "", "claudio.agent.spawn: not available")
			return 0
		}

		go func() {
			result, err := cb(context.Background(), system, prompt, model, maxTurns, allowedTools)
			errStr := ""
			if err != nil {
				errStr = err.Error()
			}
			callLuaCallback(plugin, fn, result, errStr)
		}()

		return 0
	}
}

// callLuaCallback acquires the plugin lock and calls a Lua callback with (result, err).
func callLuaCallback(plugin *loadedPlugin, fn *lua.LFunction, result, errStr string) {
	plugin.mu.Lock()
	defer plugin.mu.Unlock()
	defer func() {
		if rv := recover(); rv != nil {
			log.Printf("[lua] callback panic: %v", rv)
		}
	}()
	errVal := lua.LValue(lua.LNil)
	if errStr != "" {
		errVal = lua.LString(errStr)
	}
	_ = plugin.L.CallByParam(lua.P{Fn: fn, NRet: 0, Protect: true},
		lua.LString(result), errVal)
}

// luaStringField extracts a string field from a Lua table, returning "" if absent.
func luaStringField(tbl *lua.LTable, key string) string {
	v := tbl.RawGetString(key)
	if s, ok := v.(lua.LString); ok {
		return string(s)
	}
	return ""
}

// luaNumberField extracts a number field from a Lua table, returning 0 if absent.
func luaNumberField(tbl *lua.LTable, key string) float64 {
	v := tbl.RawGetString(key)
	if n, ok := v.(lua.LNumber); ok {
		return float64(n)
	}
	return 0
}
