package lua

import (
	"fmt"
	"regexp"
	"sort"

	"github.com/Abraxas-365/claudio/internal/filters/luaregistry"
	"github.com/Abraxas-365/claudio/internal/tools/outputfilter/tomlfilter"
	lua "github.com/yuin/gopher-lua"
)

// injectFilterAPI registers the claudio.filter sub-table on the claudio global.
func (r *Runtime) injectFilterAPI(L *lua.LState, plugin *loadedPlugin, claudio *lua.LTable) {
	filterTable := L.NewTable()
	L.SetField(filterTable, "register", L.NewFunction(r.apiFilterRegister(plugin)))
	L.SetField(filterTable, "unregister", L.NewFunction(r.apiFilterUnregister(plugin)))
	L.SetField(filterTable, "list", L.NewFunction(r.apiFilterList(plugin)))
	L.SetField(claudio, "filter", filterTable)
}

// apiFilterRegister returns the claudio.filter.register(name, config) binding.
//
// Lua usage:
//
//	claudio.filter.register("my-filter", {
//	  match_command        = "^mycommand",
//	  strip_ansi           = true,
//	  replace              = { { pattern = "foo", replacement = "bar" } },
//	  strip_lines_matching = { "^#" },
//	  keep_lines_matching  = { "ERROR" },
//	  truncate_lines_at    = 120,
//	  head_lines           = 50,
//	  tail_lines           = 20,
//	  max_lines            = 100,
//	  on_empty             = "(no output)",
//	  transform            = function(output) return output end,
//	})
func (r *Runtime) apiFilterRegister(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		name := L.CheckString(1)
		tbl := L.CheckTable(2)

		matchCmd := tableString(L, tbl, "match_command")
		if matchCmd == "" {
			L.ArgError(2, "match_command required")
			return 0
		}

		cmdRe, err := regexp.Compile(matchCmd)
		if err != nil {
			L.ArgError(2, fmt.Sprintf("match_command invalid regex: %v", err))
			return 0
		}

		def := tomlfilter.FilterDef{
			MatchCommand:       matchCmd,
			StripAnsi:          tableBool(L, tbl, "strip_ansi"),
			StripLinesMatching: tableStringSlice(L, tbl, "strip_lines_matching"),
			KeepLinesMatching:  tableStringSlice(L, tbl, "keep_lines_matching"),
			OnEmpty:            tableString(L, tbl, "on_empty"),
		}

		// Optional int pointer fields
		if v := tableOptInt(L, tbl, "truncate_lines_at"); v != nil {
			def.TruncateLinesAt = v
		}
		if v := tableOptInt(L, tbl, "head_lines"); v != nil {
			def.HeadLines = v
		}
		if v := tableOptInt(L, tbl, "tail_lines"); v != nil {
			def.TailLines = v
		}
		if v := tableOptInt(L, tbl, "max_lines"); v != nil {
			def.MaxLines = v
		}

		// Replace rules: array of {pattern, replacement} tables
		if replVal := L.GetField(tbl, "replace"); replVal != lua.LNil {
			if replTbl, ok := replVal.(*lua.LTable); ok {
				replTbl.ForEach(func(_, v lua.LValue) {
					if rt, ok := v.(*lua.LTable); ok {
						def.Replace = append(def.Replace, tomlfilter.ReplaceRule{
							Pattern:     tableString(L, rt, "pattern"),
							Replacement: tableString(L, rt, "replacement"),
						})
					}
				})
			}
		}

		// Optional transform function
		var transformFn *lua.LFunction
		if tfVal := L.GetField(tbl, "transform"); tfVal != lua.LNil {
			if fn, ok := tfVal.(*lua.LFunction); ok {
				transformFn = fn
			}
		}

		entry := &luaregistry.Entry{
			Name:      name,
			Def:       def,
			CommandRe: cmdRe,
			Transform: transformFn,
			VM:        L,
			Mu:        &plugin.mu,
		}

		luaregistry.Register(name, entry)
		return 0
	}
}

// apiFilterUnregister returns the claudio.filter.unregister(name) binding.
func (r *Runtime) apiFilterUnregister(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		name := L.CheckString(1)
		luaregistry.Unregister(name)
		return 0
	}
}

// apiFilterList returns the claudio.filter.list() binding.
func (r *Runtime) apiFilterList(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		names := luaregistry.List()
		sort.Strings(names)
		tbl := L.NewTable()
		for i, name := range names {
			tbl.RawSetInt(i+1, lua.LString(name))
		}
		L.Push(tbl)
		return 1
	}
}

// tableBool extracts a boolean field from a Lua table.
func tableBool(L *lua.LState, tbl *lua.LTable, key string) bool {
	v := L.GetField(tbl, key)
	if b, ok := v.(lua.LBool); ok {
		return bool(b)
	}
	return false
}

// tableOptInt reads an optional integer field, returning nil if absent.
func tableOptInt(L *lua.LState, tbl *lua.LTable, key string) *int {
	v := L.GetField(tbl, key)
	if n, ok := v.(lua.LNumber); ok {
		i := int(n)
		return &i
	}
	return nil
}
