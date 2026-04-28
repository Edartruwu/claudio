package lua

import (
	lua "github.com/yuin/gopher-lua"
)

// apiSessionID returns the claudio.session.id() binding.
//
// Lua usage:
//
//	local id = claudio.session.id()
func (r *Runtime) apiSessionID(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		r.sessionMu.RLock()
		id := r.currentSessionID
		r.sessionMu.RUnlock()
		if id == "" {
			L.Push(lua.LNil)
		} else {
			L.Push(lua.LString(id))
		}
		return 1
	}
}

// apiSessionTitle returns the claudio.session.title() binding.
//
// Lua usage:
//
//	local title = claudio.session.title()
func (r *Runtime) apiSessionTitle(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		r.sessionMu.RLock()
		title := r.currentSessionTitle
		r.sessionMu.RUnlock()
		if title == "" {
			L.Push(lua.LNil)
		} else {
			L.Push(lua.LString(title))
		}
		return 1
	}
}

// apiSessionOnStart returns the claudio.session.on_start(fn) binding.
//
// Lua usage:
//
//	claudio.session.on_start(function(session_id, title) ... end)
func (r *Runtime) apiSessionOnStart(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		fn := L.CheckFunction(1)
		r.sessionMu.Lock()
		r.sessionStartHdlrs = append(r.sessionStartHdlrs, luaHandler{plugin: plugin, fn: fn})
		r.sessionMu.Unlock()
		return 0
	}
}

// apiSessionOnEnd returns the claudio.session.on_end(fn) binding.
//
// Lua usage:
//
//	claudio.session.on_end(function(session_id) ... end)
func (r *Runtime) apiSessionOnEnd(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		fn := L.CheckFunction(1)
		r.sessionMu.Lock()
		r.sessionEndHdlrs = append(r.sessionEndHdlrs, luaHandler{plugin: plugin, fn: fn})
		r.sessionMu.Unlock()
		return 0
	}
}

// apiSessionOnMessage returns the claudio.session.on_message(fn) binding.
//
// Lua usage:
//
//	claudio.session.on_message(function(role, content) ... end)
func (r *Runtime) apiSessionOnMessage(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		fn := L.CheckFunction(1)
		r.sessionMu.Lock()
		r.messageHdlrs = append(r.messageHdlrs, luaHandler{plugin: plugin, fn: fn})
		r.sessionMu.Unlock()
		return 0
	}
}
