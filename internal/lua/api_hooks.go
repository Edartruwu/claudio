package lua

import (
	"context"
	"fmt"
	"log"

	"github.com/Abraxas-365/claudio/internal/hooks"
	lua "github.com/yuin/gopher-lua"
)

// apiRegisterHook returns the claudio.register_hook(event, matcher, handler) binding.
//
// Lua usage:
//
//	claudio.register_hook("PostToolUse", "Write", function(ctx)
//	  claudio.log("Write tool just ran")
//	end)
//
// Registers a Go-side inline hook that calls back into the Lua function.
// The hook is registered via hooks.Manager.RegisterInlineHook.
func (r *Runtime) apiRegisterHook(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		eventType := L.CheckString(1)
		matcher := L.CheckString(2)
		handler := L.CheckFunction(3)

		hookFn := func(ctx context.Context, hctx hooks.HookContext) error {
			plugin.mu.Lock()
			defer plugin.mu.Unlock()

			defer func() {
				if rv := recover(); rv != nil {
					log.Printf("[lua] plugin %s hook panic: %v", plugin.name, rv)
				}
			}()

			// Build context table for Lua
			ctxTbl := plugin.L.NewTable()
			plugin.L.SetField(ctxTbl, "event", lua.LString(string(hctx.Event)))
			plugin.L.SetField(ctxTbl, "tool_name", lua.LString(hctx.ToolName))
			plugin.L.SetField(ctxTbl, "tool_input", lua.LString(hctx.ToolInput))
			plugin.L.SetField(ctxTbl, "tool_output", lua.LString(hctx.ToolOutput))
			plugin.L.SetField(ctxTbl, "session_id", lua.LString(hctx.SessionID))

			if err := plugin.L.CallByParam(lua.P{
				Fn:      handler,
				NRet:    0,
				Protect: true,
			}, ctxTbl); err != nil {
				return fmt.Errorf("lua hook error: %w", err)
			}
			return nil
		}

		if r.hooks != nil {
			r.hooks.RegisterInlineHook(hooks.Event(eventType), matcher, hookFn)
		}
		return 0
	}
}
