package lua

import (
	lua "github.com/yuin/gopher-lua"
)

// apiGetConfig returns the claudio.get_config(key) binding.
//
// Lua usage:
//
//	local val = claudio.get_config("theme")  -- reads plugin's own config namespace
func (r *Runtime) apiGetConfig(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		key := L.CheckString(1)
		cfg := r.cfg.GetPluginConfig(plugin.name)
		val, ok := cfg[key]
		if !ok {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(goToLuaValue(L, val))
		return 1
	}
}

// apiSetConfig returns the claudio.set_config(key, value) binding.
//
// Lua usage:
//
//	claudio.set_config("theme", "dark")
func (r *Runtime) apiSetConfig(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		key := L.CheckString(1)
		val := L.Get(2)
		cfg := r.cfg.GetPluginConfig(plugin.name)
		cfg[key] = luaValueToGo(val)
		return 0
	}
}

// goToLuaValue converts a Go value (from config map) to a Lua LValue.
// This is similar to jsonToLuaValue but works with Go native types directly.
func goToLuaValue(L *lua.LState, v any) lua.LValue {
	if v == nil {
		return lua.LNil
	}
	switch val := v.(type) {
	case bool:
		return lua.LBool(val)
	case float64:
		return lua.LNumber(val)
	case int:
		return lua.LNumber(float64(val))
	case string:
		return lua.LString(val)
	case []any:
		tbl := L.NewTable()
		for i, item := range val {
			tbl.RawSetInt(i+1, goToLuaValue(L, item))
		}
		return tbl
	case map[string]any:
		tbl := L.NewTable()
		for k, item := range val {
			tbl.RawSetString(k, goToLuaValue(L, item))
		}
		return tbl
	default:
		// Fallback: convert to string
		return jsonToLuaValue(L, v)
	}
}
