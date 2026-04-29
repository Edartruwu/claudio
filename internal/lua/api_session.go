package lua

import (
	"log"

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

// apiSessionMessages returns the claudio.session.messages(limit) binding.
//
// Lua usage:
//
//	local msgs = claudio.session.messages(10)
//	-- returns [{role="user", content="..."}, ...]
func (r *Runtime) apiSessionMessages(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		limit := L.OptInt(1, 20)

		r.sessionMu.RLock()
		sessionID := r.currentSessionID
		r.sessionMu.RUnlock()

		if sessionID == "" || r.db == nil {
			L.Push(L.NewTable())
			return 1
		}

		records, err := r.db.GetMessages(sessionID)
		if err != nil {
			log.Printf("[lua] session.messages: %v", err)
			L.Push(L.NewTable())
			return 1
		}

		// Only keep user/assistant messages with content
		var filtered []struct{ role, content string }
		for _, rec := range records {
			if rec.Role == "user" || rec.Role == "assistant" {
				if rec.Content != "" {
					filtered = append(filtered, struct{ role, content string }{rec.Role, rec.Content})
				}
			}
		}

		// Apply limit from end (most recent messages)
		if limit > 0 && len(filtered) > limit {
			filtered = filtered[len(filtered)-limit:]
		}

		result := L.NewTable()
		for _, msg := range filtered {
			entry := L.NewTable()
			L.SetField(entry, "role", lua.LString(msg.role))
			L.SetField(entry, "content", lua.LString(msg.content))
			result.Append(entry)
		}
		L.Push(result)
		return 1
	}
}
