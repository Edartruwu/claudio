// Package lua: primitive data-list APIs exposed as claudio.sessions.*, claudio.models.*,
// claudio.commands.*, claudio.skills.*, claudio.windows.*, claudio.actions.*.
package lua

import (
	"fmt"
	"log"

	keymapPkg "github.com/Abraxas-365/claudio/internal/tui/keymap"
	"github.com/Abraxas-365/claudio/internal/tui/windows"
	lua "github.com/yuin/gopher-lua"
)

// layoutName maps windows.Layout int to a human-readable string.
func layoutName(l windows.Layout) string {
	switch l {
	case windows.LayoutFloat:
		return "float"
	case windows.LayoutSidebar:
		return "sidebar"
	case windows.LayoutSplitH:
		return "split_h"
	case windows.LayoutSplitV:
		return "split_v"
	default:
		return fmt.Sprintf("unknown(%d)", l)
	}
}

// ── claudio.sessions.list ────────────────────────────────────────────────────

// apiSessionsList returns the claudio.sessions.list(limit?) binding.
//
// Lua usage:
//
//	local sessions = claudio.sessions.list()     -- default limit 50
//	local sessions = claudio.sessions.list(10)
//	-- [{id, title, project_dir, model, created_at, updated_at}, ...]
func (r *Runtime) apiSessionsList(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		limit := L.OptInt(1, 50)

		if r.db == nil {
			L.Push(L.NewTable())
			return 1
		}

		sessions, err := r.db.ListSessions(limit)
		if err != nil {
			log.Printf("[lua] sessions.list: %v", err)
			L.Push(L.NewTable())
			return 1
		}

		result := L.NewTable()
		for i, s := range sessions {
			entry := L.NewTable()
			L.SetField(entry, "id", lua.LString(s.ID))
			L.SetField(entry, "title", lua.LString(s.Title))
			L.SetField(entry, "project_dir", lua.LString(s.ProjectDir))
			L.SetField(entry, "model", lua.LString(s.Model))
			L.SetField(entry, "created_at", lua.LString(s.CreatedAt.Format("2006-01-02T15:04:05Z07:00")))
			L.SetField(entry, "updated_at", lua.LString(s.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")))
			result.RawSetInt(i+1, entry)
		}
		L.Push(result)
		return 1
	}
}

// ── claudio.sessions.search ──────────────────────────────────────────────────

// apiSessionsSearch returns the claudio.sessions.search(query, limit?) binding.
//
// Lua usage:
//
//	local sessions = claudio.sessions.search("auth")
//	local sessions = claudio.sessions.search("auth", 20)
func (r *Runtime) apiSessionsSearch(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		query := L.CheckString(1)
		limit := L.OptInt(2, 50)

		if r.db == nil {
			L.Push(L.NewTable())
			return 1
		}

		sessions, err := r.db.SearchSessions(query, limit)
		if err != nil {
			log.Printf("[lua] sessions.search: %v", err)
			L.Push(L.NewTable())
			return 1
		}

		result := L.NewTable()
		for i, s := range sessions {
			entry := L.NewTable()
			L.SetField(entry, "id", lua.LString(s.ID))
			L.SetField(entry, "title", lua.LString(s.Title))
			L.SetField(entry, "project_dir", lua.LString(s.ProjectDir))
			L.SetField(entry, "model", lua.LString(s.Model))
			L.SetField(entry, "created_at", lua.LString(s.CreatedAt.Format("2006-01-02T15:04:05Z07:00")))
			L.SetField(entry, "updated_at", lua.LString(s.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")))
			result.RawSetInt(i+1, entry)
		}
		L.Push(result)
		return 1
	}
}

// ── claudio.models.list ──────────────────────────────────────────────────────

// apiModelsList returns the claudio.models.list() binding.
//
// Lua usage:
//
//	local models = claudio.models.list()
//	-- [{id, name, provider}, ...]
func (r *Runtime) apiModelsList(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		result := L.NewTable()

		if r.cfg == nil {
			L.Push(result)
			return 1
		}

		idx := 1
		for providerName, pCfg := range r.cfg.Providers {
			for shortcut, modelID := range pCfg.Models {
				entry := L.NewTable()
				L.SetField(entry, "id", lua.LString(modelID))
				L.SetField(entry, "name", lua.LString(shortcut))
				L.SetField(entry, "provider", lua.LString(providerName))
				result.RawSetInt(idx, entry)
				idx++
			}
		}

		L.Push(result)
		return 1
	}
}

// ── claudio.commands.list ────────────────────────────────────────────────────

// apiCommandsList returns the claudio.commands.list() binding.
//
// Lua usage:
//
//	local cmds = claudio.commands.list()
//	-- [{name, description}, ...]
func (r *Runtime) apiCommandsList(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		result := L.NewTable()

		r.commandRegistryMu.Lock()
		reg := r.commandRegistry
		r.commandRegistryMu.Unlock()

		if reg == nil {
			L.Push(result)
			return 1
		}

		for i, cmd := range reg.ListCommands() {
			entry := L.NewTable()
			L.SetField(entry, "name", lua.LString(cmd.Name))
			L.SetField(entry, "description", lua.LString(cmd.Description))
			result.RawSetInt(i+1, entry)
		}

		L.Push(result)
		return 1
	}
}

// ── claudio.skills.list ──────────────────────────────────────────────────────

// apiSkillsList returns the claudio.skills.list() binding.
//
// Lua usage:
//
//	local skills = claudio.skills.list()
//	-- [{name, description, source}, ...]
func (r *Runtime) apiSkillsList(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		result := L.NewTable()

		if r.skills == nil {
			L.Push(result)
			return 1
		}

		for i, sk := range r.skills.All() {
			entry := L.NewTable()
			L.SetField(entry, "name", lua.LString(sk.Name))
			L.SetField(entry, "description", lua.LString(sk.Description))
			L.SetField(entry, "source", lua.LString(sk.Source))
			result.RawSetInt(i+1, entry)
		}

		L.Push(result)
		return 1
	}
}

// ── claudio.windows.list ─────────────────────────────────────────────────────

// apiWindowsList returns the claudio.windows.list() binding.
//
// Lua usage:
//
//	local wins = claudio.windows.list()
//	-- [{id, title, focused, type, agent_name}, ...]
func (r *Runtime) apiWindowsList(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		result := L.NewTable()

		r.windowManagerMu.RLock()
		wm := r.windowManager
		r.windowManagerMu.RUnlock()

		if wm == nil {
			L.Push(result)
			return 1
		}

		for i, w := range wm.AllWindows() {
			entry := L.NewTable()
			L.SetField(entry, "id", lua.LString(w.Name))
			L.SetField(entry, "title", lua.LString(w.Title))
			L.SetField(entry, "focused", lua.LBool(w.Focused))
			L.SetField(entry, "type", lua.LString(layoutName(w.Layout)))
			L.SetField(entry, "agent_name", lua.LString(w.AgentName))
			result.RawSetInt(i+1, entry)
		}

		L.Push(result)
		return 1
	}
}

// ── claudio.windows.read ─────────────────────────────────────────────────────

// apiWindowsRead returns the claudio.windows.read(name) binding.
//
// Lua usage:
//
//	local buf = claudio.windows.read("agent://researcher")
//	-- returns {lines={...}, status="running"|"done"|"error", len=42} or nil
func (r *Runtime) apiWindowsRead(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		name := L.CheckString(1)

		r.windowManagerMu.RLock()
		wm := r.windowManager
		r.windowManagerMu.RUnlock()

		if wm == nil {
			L.Push(lua.LNil)
			return 1
		}

		lb, ok := wm.GetLiveBuffer(name)
		if !ok {
			L.Push(lua.LNil)
			return 1
		}

		lines := lb.Lines()
		linesTbl := L.NewTable()
		for i, line := range lines {
			linesTbl.RawSetInt(i+1, lua.LString(line))
		}

		tbl := L.NewTable()
		L.SetField(tbl, "lines", linesTbl)
		L.SetField(tbl, "status", lua.LString(lb.Status()))
		L.SetField(tbl, "len", lua.LNumber(len(lines)))
		L.Push(tbl)
		return 1
	}
}

// ── claudio.actions.list ─────────────────────────────────────────────────────

// apiActionsList returns the claudio.actions.list() binding.
//
// Lua usage:
//
//	local actions = claudio.actions.list()
//	-- [{id, description, group}, ...]
func (r *Runtime) apiActionsList(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		result := L.NewTable()

		idx := 1
		for _, meta := range keymapPkg.Registry {
			entry := L.NewTable()
			L.SetField(entry, "id", lua.LString(string(meta.ID)))
			L.SetField(entry, "description", lua.LString(meta.Description))
			L.SetField(entry, "group", lua.LString(meta.Group))
			result.RawSetInt(idx, entry)
			idx++
		}

		L.Push(result)
		return 1
	}
}
