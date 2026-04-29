package lua

import (
	"encoding/json"
	"log"
	"strconv"

	"github.com/Abraxas-365/claudio/internal/bus"
	lua "github.com/yuin/gopher-lua"
)

// apiBranchCurrent returns the claudio.branch.current() binding.
//
// Lua usage:
//
//	local info = claudio.branch.current()
//	-- returns {id, title, parent_id, branch_from_message_id, depth} or nil
func (r *Runtime) apiBranchCurrent(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		r.sessionMu.RLock()
		sessionID := r.currentSessionID
		r.sessionMu.RUnlock()

		if sessionID == "" || r.db == nil {
			L.Push(lua.LNil)
			return 1
		}

		sess, err := r.db.GetSession(sessionID)
		if err != nil || sess == nil || sess.BranchFromMessageID == nil {
			L.Push(lua.LNil)
			return 1
		}

		// Compute depth by walking parent chain.
		depth := 0
		parentID := sess.ParentSessionID
		for parentID != "" {
			depth++
			parent, err := r.db.GetSession(parentID)
			if err != nil || parent == nil {
				break
			}
			parentID = parent.ParentSessionID
		}

		tbl := L.NewTable()
		L.SetField(tbl, "id", lua.LString(sess.ID))
		L.SetField(tbl, "title", lua.LString(sess.Title))
		L.SetField(tbl, "parent_id", lua.LString(sess.ParentSessionID))
		L.SetField(tbl, "branch_from_message_id", lua.LNumber(*sess.BranchFromMessageID))
		L.SetField(tbl, "depth", lua.LNumber(depth))
		L.Push(tbl)
		return 1
	}
}

// apiBranchCreate returns the claudio.branch.create(message_id?) binding.
//
// Lua usage:
//
//	local branch, err = claudio.branch.create()       -- branch from last msg
//	local branch, err = claudio.branch.create(42)     -- branch from msg 42
func (r *Runtime) apiBranchCreate(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		r.sessionMu.RLock()
		sessionID := r.currentSessionID
		r.sessionMu.RUnlock()

		if sessionID == "" || r.db == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("no active session"))
			return 2
		}

		sess, err := r.db.GetSession(sessionID)
		if err != nil || sess == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("session not found"))
			return 2
		}

		var messageID int64
		arg := L.Get(1)
		if num, ok := arg.(lua.LNumber); ok {
			messageID = int64(num)
		} else {
			// Use last message.
			msgs, err := r.db.GetMessages(sessionID)
			if err != nil || len(msgs) == 0 {
				L.Push(lua.LNil)
				L.Push(lua.LString("no messages to branch from"))
				return 2
			}
			messageID = msgs[len(msgs)-1].ID
		}

		branch, err := r.db.CreateBranchSession(sessionID, messageID, sess.ProjectDir, sess.Model)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		r.NotifyBranchCreated(branch.ID, sessionID, strconv.FormatInt(messageID, 10))

		tbl := L.NewTable()
		L.SetField(tbl, "id", lua.LString(branch.ID))
		L.SetField(tbl, "title", lua.LString(branch.Title))
		L.Push(tbl)
		L.Push(lua.LNil)
		return 2
	}
}

// apiBranchParent returns the claudio.branch.parent() binding.
//
// Lua usage:
//
//	local parent = claudio.branch.parent()  -- {id, title, model} or nil
func (r *Runtime) apiBranchParent(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		r.sessionMu.RLock()
		sessionID := r.currentSessionID
		r.sessionMu.RUnlock()

		if sessionID == "" || r.db == nil {
			L.Push(lua.LNil)
			return 1
		}

		sess, err := r.db.GetSession(sessionID)
		if err != nil || sess == nil || sess.ParentSessionID == "" {
			L.Push(lua.LNil)
			return 1
		}

		parent, err := r.db.GetSession(sess.ParentSessionID)
		if err != nil || parent == nil {
			L.Push(lua.LNil)
			return 1
		}

		tbl := L.NewTable()
		L.SetField(tbl, "id", lua.LString(parent.ID))
		L.SetField(tbl, "title", lua.LString(parent.Title))
		L.SetField(tbl, "model", lua.LString(parent.Model))
		L.Push(tbl)
		return 1
	}
}

// apiBranchChildren returns the claudio.branch.children() binding.
//
// Lua usage:
//
//	local kids = claudio.branch.children()
//	-- [{id, title, branch_from_message_id}, ...]
func (r *Runtime) apiBranchChildren(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		r.sessionMu.RLock()
		sessionID := r.currentSessionID
		r.sessionMu.RUnlock()

		if sessionID == "" || r.db == nil {
			L.Push(L.NewTable())
			return 1
		}

		branches, err := r.db.GetSessionBranches(sessionID)
		if err != nil {
			log.Printf("[lua] branch.children: %v", err)
			L.Push(L.NewTable())
			return 1
		}

		result := L.NewTable()
		for _, b := range branches {
			entry := L.NewTable()
			L.SetField(entry, "id", lua.LString(b.ID))
			L.SetField(entry, "title", lua.LString(b.Title))
			if b.BranchFromMessageID != nil {
				L.SetField(entry, "branch_from_message_id", lua.LNumber(*b.BranchFromMessageID))
			}
			result.Append(entry)
		}
		L.Push(result)
		return 1
	}
}

// apiBranchRoot returns the claudio.branch.root() binding.
//
// Lua usage:
//
//	local root = claudio.branch.root()  -- {id, title}
func (r *Runtime) apiBranchRoot(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		r.sessionMu.RLock()
		sessionID := r.currentSessionID
		r.sessionMu.RUnlock()

		if sessionID == "" || r.db == nil {
			L.Push(lua.LNil)
			return 1
		}

		root, err := r.db.GetRootSession(sessionID)
		if err != nil {
			log.Printf("[lua] branch.root: %v", err)
			L.Push(lua.LNil)
			return 1
		}

		tbl := L.NewTable()
		L.SetField(tbl, "id", lua.LString(root.ID))
		L.SetField(tbl, "title", lua.LString(root.Title))
		L.Push(tbl)
		return 1
	}
}

// apiBranchMessages returns the claudio.branch.messages(session_id?) binding.
//
// Lua usage:
//
//	local msgs = claudio.branch.messages()          -- current session
//	local msgs = claudio.branch.messages("abc-123") -- specific session
//	-- [{role, content}, ...]
func (r *Runtime) apiBranchMessages(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		var targetID string
		if arg, ok := L.Get(1).(lua.LString); ok {
			targetID = string(arg)
		} else {
			r.sessionMu.RLock()
			targetID = r.currentSessionID
			r.sessionMu.RUnlock()
		}

		if targetID == "" || r.db == nil {
			L.Push(L.NewTable())
			return 1
		}

		records, err := r.db.GetBranchMessages(targetID)
		if err != nil {
			log.Printf("[lua] branch.messages: %v", err)
			L.Push(L.NewTable())
			return 1
		}

		result := L.NewTable()
		for _, rec := range records {
			if rec.Role == "user" || rec.Role == "assistant" {
				if rec.Content != "" {
					entry := L.NewTable()
					L.SetField(entry, "role", lua.LString(rec.Role))
					L.SetField(entry, "content", lua.LString(rec.Content))
					result.Append(entry)
				}
			}
		}
		L.Push(result)
		return 1
	}
}

// apiBranchSwitch returns the claudio.branch.switch(session_id) binding.
//
// Lua usage:
//
//	local ok, err = claudio.branch.switch("abc-123")
func (r *Runtime) apiBranchSwitch(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		targetID := L.CheckString(1)

		if r.bus == nil {
			L.Push(lua.LFalse)
			L.Push(lua.LString("event bus not available"))
			return 2
		}

		if r.db != nil {
			sess, err := r.db.GetSession(targetID)
			if err != nil || sess == nil {
				L.Push(lua.LFalse)
				L.Push(lua.LString("session not found"))
				return 2
			}
		}

		payload, _ := json.Marshal(map[string]string{"session_id": targetID})
		r.bus.Publish(bus.Event{
			Type:    "session.switch",
			Payload: json.RawMessage(payload),
		})

		L.Push(lua.LTrue)
		L.Push(lua.LNil)
		return 2
	}
}

// apiBranchOnBranch returns the claudio.branch.on_branch(fn) binding.
//
// Lua usage:
//
//	claudio.branch.on_branch(function(branch_id, parent_id, message_id) ... end)
func (r *Runtime) apiBranchOnBranch(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		fn := L.CheckFunction(1)
		r.branchHdlrsMu.Lock()
		r.branchHdlrs = append(r.branchHdlrs, luaHandler{plugin: plugin, fn: fn})
		r.branchHdlrsMu.Unlock()
		return 0
	}
}

// NotifyBranchCreated fires all registered on_branch handlers.
// Called from TUI or Lua after a branch session is created.
func (r *Runtime) NotifyBranchCreated(branchID, parentID, messageID string) {
	r.branchHdlrsMu.RLock()
	handlers := make([]luaHandler, len(r.branchHdlrs))
	copy(handlers, r.branchHdlrs)
	r.branchHdlrsMu.RUnlock()

	for _, h := range handlers {
		h.plugin.mu.Lock()
		func() {
			defer func() {
				if rv := recover(); rv != nil {
					log.Printf("[lua] on_branch handler panic: %v", rv)
				}
			}()
			_ = h.plugin.L.CallByParam(lua.P{Fn: h.fn, NRet: 0, Protect: true},
				lua.LString(branchID), lua.LString(parentID), lua.LString(messageID))
		}()
		h.plugin.mu.Unlock()
	}

	// Also publish bus event for non-Lua subscribers.
	if r.bus != nil {
		payload, _ := json.Marshal(map[string]string{
			"branch_id":  branchID,
			"parent_id":  parentID,
			"message_id": messageID,
		})
		r.bus.Publish(bus.Event{
			Type:    "branch.created",
			Payload: json.RawMessage(payload),
		})
	}
}


