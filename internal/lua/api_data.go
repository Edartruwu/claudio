// Package lua — data provider APIs exposed as claudio.session.*, claudio.files.*,
// claudio.tasks.*, claudio.tokens.* for Lua plugins (especially sidebar.lua).
package lua

import (
	"path/filepath"

	lua "github.com/yuin/gopher-lua"
)

// ---------------------------------------------------------------------------
// Provider interfaces — injected by root.go after TUI init
// ---------------------------------------------------------------------------

// SessionProvider supplies session metadata for claudio.session.current().
type SessionProvider interface {
	CurrentID() string
	CurrentName() string
	CurrentModel() string
}

// FileEntry represents a single file in the sidebar file list.
type FileEntry struct {
	Path  string
	Name  string
	IsDir bool
}

// FilesProvider supplies the list of files touched in the current session.
type FilesProvider interface {
	List() []FileEntry
}

// TaskEntry represents a task for the Lua API surface.
type TaskEntry struct {
	ID         string
	Title      string
	Status     string
	ActiveForm string
}

// TasksProvider supplies the list of tracked tasks.
type TasksProvider interface {
	List() []TaskEntry
}

// TokenUsage holds token usage stats for claudio.tokens.usage().
type TokenUsage struct {
	Used int
	Max  int
	Cost float64
}

// TokensProvider supplies token usage data.
type TokensProvider interface {
	Usage() TokenUsage
}

// ---------------------------------------------------------------------------
// Lua API injection
// ---------------------------------------------------------------------------

// injectDataAPIs adds claudio.session.current, claudio.files.list,
// claudio.tasks.list, and claudio.tokens.usage to the claudio global table.
// Called from injectAPI.
func (r *Runtime) injectDataAPIs(L *lua.LState, claudio *lua.LTable) {
	// Extend existing claudio.session table with current()
	sessionTbl, ok := L.GetField(claudio, "session").(*lua.LTable)
	if ok && sessionTbl != nil {
		L.SetField(sessionTbl, "current", L.NewFunction(r.apiSessionCurrent()))
	}

	// claudio.files sub-table
	filesTbl := L.NewTable()
	L.SetField(filesTbl, "list", L.NewFunction(r.apiFilesList()))
	L.SetField(claudio, "files", filesTbl)

	// claudio.tasks sub-table
	tasksTbl := L.NewTable()
	L.SetField(tasksTbl, "list", L.NewFunction(r.apiTasksList()))
	L.SetField(tasksTbl, "background", L.NewFunction(r.apiTasksBackground()))
	L.SetField(tasksTbl, "kill", L.NewFunction(r.apiTasksKill()))
	L.SetField(claudio, "tasks", tasksTbl)

	// claudio.tokens sub-table
	tokensTbl := L.NewTable()
	L.SetField(tokensTbl, "usage", L.NewFunction(r.apiTokensUsage()))
	L.SetField(claudio, "tokens", tokensTbl)
}

// ---------------------------------------------------------------------------
// API implementations
// ---------------------------------------------------------------------------

// apiSessionCurrent returns claudio.session.current() → {id, name, model} or nil.
func (r *Runtime) apiSessionCurrent() lua.LGFunction {
	return func(L *lua.LState) int {
		r.sessionProviderMu.RLock()
		p := r.sessionProvider
		r.sessionProviderMu.RUnlock()
		if p == nil {
			L.Push(lua.LNil)
			return 1
		}
		tbl := L.NewTable()
		L.SetField(tbl, "id", lua.LString(p.CurrentID()))
		L.SetField(tbl, "name", lua.LString(p.CurrentName()))
		L.SetField(tbl, "model", lua.LString(p.CurrentModel()))
		L.Push(tbl)
		return 1
	}
}

// apiFilesList returns claudio.files.list() → array of {path, name, is_dir}.
func (r *Runtime) apiFilesList() lua.LGFunction {
	return func(L *lua.LState) int {
		r.filesProviderMu.RLock()
		p := r.filesProvider
		r.filesProviderMu.RUnlock()
		if p == nil {
			L.Push(L.NewTable())
			return 1
		}
		files := p.List()
		arr := L.NewTable()
		for _, f := range files {
			entry := L.NewTable()
			L.SetField(entry, "path", lua.LString(f.Path))
			name := f.Name
			if name == "" {
				name = filepath.Base(f.Path)
			}
			L.SetField(entry, "name", lua.LString(name))
			L.SetField(entry, "is_dir", lua.LBool(f.IsDir))
			arr.Append(entry)
		}
		L.Push(arr)
		return 1
	}
}

// apiTasksList returns claudio.tasks.list() → array of {id, title, status, active_form}.
func (r *Runtime) apiTasksList() lua.LGFunction {
	return func(L *lua.LState) int {
		r.tasksProviderMu.RLock()
		p := r.tasksProvider
		r.tasksProviderMu.RUnlock()
		if p == nil {
			L.Push(L.NewTable())
			return 1
		}
		tasks := p.List()
		arr := L.NewTable()
		for _, t := range tasks {
			entry := L.NewTable()
			L.SetField(entry, "id", lua.LString(t.ID))
			L.SetField(entry, "title", lua.LString(t.Title))
			L.SetField(entry, "status", lua.LString(t.Status))
			L.SetField(entry, "active_form", lua.LString(t.ActiveForm))
			arr.Append(entry)
		}
		L.Push(arr)
		return 1
	}
}

// apiTasksBackground returns claudio.tasks.background() → array of background task states.
//
// Lua usage:
//
//	local bg = claudio.tasks.background()
//	-- [{id, type, status, description, command?, agent_type?, start_time, end_time?, error?, exit_code?, tool_calls, tokens_used}, ...]
func (r *Runtime) apiTasksBackground() lua.LGFunction {
	return func(L *lua.LState) int {
		r.taskRuntimeMu.RLock()
		rt := r.taskRuntime
		r.taskRuntimeMu.RUnlock()

		arr := L.NewTable()
		if rt == nil {
			L.Push(arr)
			return 1
		}

		for i, t := range rt.List(false) {
			entry := L.NewTable()
			L.SetField(entry, "id", lua.LString(t.ID))
			L.SetField(entry, "type", lua.LString(string(t.Type)))
			L.SetField(entry, "status", lua.LString(string(t.Status)))
			L.SetField(entry, "description", lua.LString(t.Description))
			L.SetField(entry, "start_time", lua.LString(t.StartTime.Format("2006-01-02T15:04:05Z07:00")))
			L.SetField(entry, "tool_calls", lua.LNumber(t.ToolCalls))
			L.SetField(entry, "tokens_used", lua.LNumber(t.TokensUsed))
			if t.Command != "" {
				L.SetField(entry, "command", lua.LString(t.Command))
			}
			if t.AgentType != "" {
				L.SetField(entry, "agent_type", lua.LString(t.AgentType))
			}
			if t.EndTime != nil {
				L.SetField(entry, "end_time", lua.LString(t.EndTime.Format("2006-01-02T15:04:05Z07:00")))
			}
			if t.Error != "" {
				L.SetField(entry, "error", lua.LString(t.Error))
			}
			if t.ExitCode != nil {
				L.SetField(entry, "exit_code", lua.LNumber(*t.ExitCode))
			}
			arr.RawSetInt(i+1, entry)
		}

		L.Push(arr)
		return 1
	}
}

// apiTasksKill returns claudio.tasks.kill(id) → true/nil, err_string.
//
// Lua usage:
//
//	local ok, err = claudio.tasks.kill("task-abc123")
func (r *Runtime) apiTasksKill() lua.LGFunction {
	return func(L *lua.LState) int {
		id := L.CheckString(1)

		r.taskRuntimeMu.RLock()
		rt := r.taskRuntime
		r.taskRuntimeMu.RUnlock()

		if rt == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("task runtime not available"))
			return 2
		}

		if err := rt.Kill(id); err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		L.Push(lua.LTrue)
		L.Push(lua.LNil)
		return 2
	}
}

// apiTokensUsage returns claudio.tokens.usage() → {used, max, cost} or nil.
func (r *Runtime) apiTokensUsage() lua.LGFunction {
	return func(L *lua.LState) int {
		r.tokensProviderMu.RLock()
		p := r.tokensProvider
		r.tokensProviderMu.RUnlock()
		if p == nil {
			L.Push(lua.LNil)
			return 1
		}
		u := p.Usage()
		tbl := L.NewTable()
		L.SetField(tbl, "used", lua.LNumber(u.Used))
		L.SetField(tbl, "max", lua.LNumber(u.Max))
		L.SetField(tbl, "cost", lua.LNumber(u.Cost))
		L.Push(tbl)
		return 1
	}
}
