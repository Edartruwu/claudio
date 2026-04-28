// Package lua: claudio.lsp Lua API module.
//
// Exposes LSP server lifecycle management and code-intelligence queries to
// Lua plugins so users can register, start, stop, and query LSP servers at
// runtime instead of (or in addition to) settings.json lspServers.
package lua

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/Abraxas-365/claudio/internal/config"
	lsp "github.com/Abraxas-365/claudio/internal/services/lsp"
	lua "github.com/yuin/gopher-lua"
)

// injectLSPAPI adds the claudio.lsp sub-table to the claudio global.
func (r *Runtime) injectLSPAPI(L *lua.LState, plugin *loadedPlugin, claudio *lua.LTable) {
	lspTable := L.NewTable()
	L.SetField(lspTable, "register_server", L.NewFunction(r.apiLspRegisterServer(plugin)))
	L.SetField(lspTable, "enable", L.NewFunction(r.apiLspEnable(plugin)))
	L.SetField(lspTable, "disable", L.NewFunction(r.apiLspDisable(plugin)))
	L.SetField(lspTable, "list", L.NewFunction(r.apiLspList(plugin)))
	L.SetField(lspTable, "hover", L.NewFunction(r.apiLspHover(plugin)))
	L.SetField(lspTable, "go_to_definition", L.NewFunction(r.apiLspGoToDefinition(plugin)))
	L.SetField(lspTable, "find_references", L.NewFunction(r.apiLspFindReferences(plugin)))
	L.SetField(lspTable, "document_symbols", L.NewFunction(r.apiLspDocumentSymbols(plugin)))
	L.SetField(claudio, "lsp", lspTable)
}

// lspManager returns the wired ServerManager or nil.
func (r *Runtime) getLSPManager() *lsp.ServerManager {
	r.lspManagerMu.RLock()
	defer r.lspManagerMu.RUnlock()
	return r.lspManager
}

// ── claudio.lsp.register_server ──────────────────────────────────────────────

// apiLspRegisterServer implements:
//
//	claudio.lsp.register_server({
//	  name       = "gopls",
//	  command    = "gopls",
//	  args       = { "-remote=auto" },
//	  extensions = { ".go", ".mod" },
//	  env        = { GOFLAGS = "-mod=vendor" },
//	})
//
// Returns nothing on success; (nil, errstr) on failure.
func (r *Runtime) apiLspRegisterServer(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		tbl := L.CheckTable(1)

		mgr := r.getLSPManager()
		if mgr == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("lsp manager not available"))
			return 2
		}

		nameVal := L.GetField(tbl, "name")
		if nameVal == lua.LNil {
			L.Push(lua.LNil)
			L.Push(lua.LString("register_server: 'name' is required"))
			return 2
		}
		name := luaToString(nameVal)
		if name == "" {
			L.Push(lua.LNil)
			L.Push(lua.LString("register_server: 'name' is required"))
			return 2
		}

		cfg := config.LspServerConfig{
			Command: luaToString(L.GetField(tbl, "command")),
		}

		// args = string array
		if v := L.GetField(tbl, "args"); v != lua.LNil {
			if argsTbl, ok := v.(*lua.LTable); ok {
				argsTbl.ForEach(func(_, val lua.LValue) {
					if s, ok := val.(lua.LString); ok {
						cfg.Args = append(cfg.Args, string(s))
					}
				})
			}
		}

		// extensions = string array
		if v := L.GetField(tbl, "extensions"); v != lua.LNil {
			if extTbl, ok := v.(*lua.LTable); ok {
				extTbl.ForEach(func(_, val lua.LValue) {
					if s, ok := val.(lua.LString); ok {
						cfg.Extensions = append(cfg.Extensions, string(s))
					}
				})
			}
		}

		// env = string→string map
		if v := L.GetField(tbl, "env"); v != lua.LNil {
			if envTbl, ok := v.(*lua.LTable); ok {
				cfg.Env = make(map[string]string)
				envTbl.ForEach(func(k, val lua.LValue) {
					if ks, ok := k.(lua.LString); ok {
						cfg.Env[string(ks)] = luaToString(val)
					}
				})
			}
		}

		mgr.RegisterServer(name, cfg)
		return 0
	}
}

// ── claudio.lsp.enable ───────────────────────────────────────────────────────

// apiLspEnable implements:
//
//	claudio.lsp.enable("gopls")                -- uses cwd as rootDir
//	claudio.lsp.enable("gopls", "/path/root")  -- explicit rootDir
//
// Returns nothing on success; (nil, errstr) on failure.
func (r *Runtime) apiLspEnable(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		name := L.CheckString(1)
		rootDir := L.OptString(2, "")
		if rootDir == "" {
			rootDir, _ = os.Getwd()
		}

		mgr := r.getLSPManager()
		if mgr == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("lsp manager not available"))
			return 2
		}

		if err := mgr.StartServer(context.Background(), name, rootDir); err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		return 0
	}
}

// ── claudio.lsp.disable ──────────────────────────────────────────────────────

// apiLspDisable implements:
//
//	claudio.lsp.disable("gopls")  -- stops + removes config
//
// No-op if server is not configured. Returns nothing.
func (r *Runtime) apiLspDisable(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		name := L.CheckString(1)

		mgr := r.getLSPManager()
		if mgr == nil {
			return 0
		}

		mgr.UnregisterServer(name)
		return 0
	}
}

// ── claudio.lsp.list ─────────────────────────────────────────────────────────

// apiLspList implements:
//
//	local servers = claudio.lsp.list()
//	-- returns: { { name="gopls", running=true }, ... }
func (r *Runtime) apiLspList(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		result := L.NewTable()

		mgr := r.getLSPManager()
		if mgr == nil {
			L.Push(result)
			return 1
		}

		i := 1
		for name, running := range mgr.ListServers() {
			entry := L.NewTable()
			L.SetField(entry, "name", lua.LString(name))
			L.SetField(entry, "running", lua.LBool(running))
			result.RawSetInt(i, entry)
			i++
		}

		L.Push(result)
		return 1
	}
}

// ── query helpers ────────────────────────────────────────────────────────────

// lspQuery is the shared pattern for all single-file LSP query operations.
// fn receives the resolved absolute file path and returns (json.RawMessage, error).
func lspQuery(L *lua.LState, mgr *lsp.ServerManager, filePath string, fn func(srv *lsp.ServerInstance, absPath string) (json.RawMessage, error)) int {
	if mgr == nil {
		L.Push(lua.LNil)
		L.Push(lua.LString("lsp manager not available"))
		return 2
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		absPath = filePath
	}

	srv, err := mgr.GetServer(context.Background(), absPath)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	raw, err := fn(srv, absPath)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	if raw == nil {
		L.Push(lua.LNil)
		return 1
	}

	// Decode JSON into Lua value for ergonomic use from Lua.
	var decoded interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		// Fallback: return raw JSON string
		L.Push(lua.LString(string(raw)))
		return 1
	}
	L.Push(goToLua(L, decoded))
	return 1
}

// ── claudio.lsp.hover ────────────────────────────────────────────────────────

// apiLspHover implements:
//
//	local result = claudio.lsp.hover(file_path, line, char)
func (r *Runtime) apiLspHover(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		filePath := L.CheckString(1)
		line := L.CheckInt(2)
		char := L.CheckInt(3)
		return lspQuery(L, r.getLSPManager(), filePath, func(srv *lsp.ServerInstance, absPath string) (json.RawMessage, error) {
			return srv.Hover(absPath, line, char)
		})
	}
}

// ── claudio.lsp.go_to_definition ─────────────────────────────────────────────

// apiLspGoToDefinition implements:
//
//	local result = claudio.lsp.go_to_definition(file_path, line, char)
func (r *Runtime) apiLspGoToDefinition(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		filePath := L.CheckString(1)
		line := L.CheckInt(2)
		char := L.CheckInt(3)
		return lspQuery(L, r.getLSPManager(), filePath, func(srv *lsp.ServerInstance, absPath string) (json.RawMessage, error) {
			return srv.GoToDefinition(absPath, line, char)
		})
	}
}

// ── claudio.lsp.find_references ──────────────────────────────────────────────

// apiLspFindReferences implements:
//
//	local result = claudio.lsp.find_references(file_path, line, char)
func (r *Runtime) apiLspFindReferences(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		filePath := L.CheckString(1)
		line := L.CheckInt(2)
		char := L.CheckInt(3)
		return lspQuery(L, r.getLSPManager(), filePath, func(srv *lsp.ServerInstance, absPath string) (json.RawMessage, error) {
			return srv.FindReferences(absPath, line, char)
		})
	}
}

// ── claudio.lsp.document_symbols ─────────────────────────────────────────────

// apiLspDocumentSymbols implements:
//
//	local result = claudio.lsp.document_symbols(file_path)
func (r *Runtime) apiLspDocumentSymbols(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		filePath := L.CheckString(1)
		return lspQuery(L, r.getLSPManager(), filePath, func(srv *lsp.ServerInstance, absPath string) (json.RawMessage, error) {
			return srv.DocumentSymbols(absPath)
		})
	}
}

// ── JSON → Lua conversion ─────────────────────────────────────────────────────

// goToLua recursively converts a Go value (from json.Unmarshal) to a Lua value.
func goToLua(L *lua.LState, v interface{}) lua.LValue {
	if v == nil {
		return lua.LNil
	}
	switch val := v.(type) {
	case bool:
		return lua.LBool(val)
	case float64:
		return lua.LNumber(val)
	case string:
		return lua.LString(val)
	case []interface{}:
		tbl := L.NewTable()
		for i, item := range val {
			tbl.RawSetInt(i+1, goToLua(L, item))
		}
		return tbl
	case map[string]interface{}:
		tbl := L.NewTable()
		for k, item := range val {
			L.SetField(tbl, k, goToLua(L, item))
		}
		return tbl
	default:
		return lua.LString("")
	}
}
