package lua

import (
	"log"

	lua "github.com/yuin/gopher-lua"
)

// apiPromptSetPlaceholder returns claudio.prompt.set_placeholder(s) binding.
//
// Lua usage:
//
//	claudio.prompt.set_placeholder("Ask anything...")
//
// Sets the placeholder text shown in the prompt when empty.
// If the prompt is not yet wired (pre-TUI init), the value is stored and
// applied when SetPrompt is called.
func (r *Runtime) apiPromptSetPlaceholder(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		s := L.CheckString(1)
		r.promptMu.Lock()
		defer r.promptMu.Unlock()
		r.promptPlaceholder = s
		if r.prompt != nil {
			r.prompt.SetPlaceholder(s)
		}
		return 0
	}
}

// apiPromptSetMode returns claudio.prompt.set_mode(mode) binding.
//
// Lua usage:
//
//	claudio.prompt.set_mode("vim")    -- enable vim mode
//	claudio.prompt.set_mode("simple") -- disable vim mode
//
// mode must be "vim" or "simple". If the prompt is not yet wired, the
// desired mode is stored and applied when SetPrompt is called.
func (r *Runtime) apiPromptSetMode(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		mode := L.CheckString(1)
		if mode != "vim" && mode != "simple" {
			L.ArgError(1, "mode must be 'vim' or 'simple'")
			return 0
		}
		r.promptMu.Lock()
		defer r.promptMu.Unlock()
		r.promptDesiredMode = mode
		if r.prompt != nil {
			applyPromptMode(r.prompt, mode)
		}
		return 0
	}
}

// apiPromptOnSubmit returns claudio.prompt.on_submit(fn) binding.
//
// Lua usage:
//
//	claudio.prompt.on_submit(function(text)
//	    -- return text to continue (possibly modified)
//	    -- return false to cancel submission
//	    -- return nil/nothing to pass through unchanged
//	    return text
//	end)
//
// Hooks run in registration order. Each hook receives the current text string.
// Returning false (Lua boolean) cancels submission. Returning a string replaces
// the text for subsequent hooks. Returning nil or nothing passes text through.
func (r *Runtime) apiPromptOnSubmit(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		fn := L.CheckFunction(1)
		r.promptHooksMu.Lock()
		r.promptHooks = append(r.promptHooks, luaHandler{plugin: plugin, fn: fn})
		r.promptHooksMu.Unlock()
		return 0
	}
}

// RunPromptHooks runs all registered on_submit hooks in registration order.
//
// Each hook receives the current text. If any hook returns false (Lua boolean),
// submission is cancelled and (text, true) is returned. If a hook returns a
// string, that string replaces the text for subsequent hooks. Nil / no return
// value passes text through unchanged.
//
// Returns the (possibly transformed) text and whether submission was cancelled.
func (r *Runtime) RunPromptHooks(text string) (string, bool) {
	r.promptHooksMu.RLock()
	hooks := make([]luaHandler, len(r.promptHooks))
	copy(hooks, r.promptHooks)
	r.promptHooksMu.RUnlock()

	for _, h := range hooks {
		h.plugin.mu.Lock()
		var result lua.LValue
		func() {
			defer func() {
				if rv := recover(); rv != nil {
					log.Printf("[lua] on_submit handler panic: %v", rv)
				}
			}()
			if err := h.plugin.L.CallByParam(lua.P{
				Fn:      h.fn,
				NRet:    1,
				Protect: true,
			}, lua.LString(text)); err != nil {
				log.Printf("[lua] on_submit handler error: %v", err)
				// Protected call pushes the error value onto the stack;
				// pop it to avoid Lua VM stack growth on repeated errors.
				h.plugin.L.Pop(1)
				return
			}
			result = h.plugin.L.Get(-1)
			h.plugin.L.Pop(1)
		}()
		h.plugin.mu.Unlock()

		if result == nil {
			continue
		}
		switch v := result.(type) {
		case *lua.LNilType:
			// pass through unchanged
		case lua.LBool:
			if !bool(v) {
				return text, true // cancelled
			}
			// true → pass through (treat same as nil)
		case lua.LString:
			text = string(v)
		}
	}
	return text, false
}
