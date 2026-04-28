package lua

import (
	lua "github.com/yuin/gopher-lua"
)

// injectGlobalConfigAPI adds claudio.config and claudio.ui sub-tables to the
// claudio global. These are distinct from the per-plugin claudio.set_config /
// claudio.get_config APIs: they read and write directly to the shared
// config.Settings instance rather than a plugin-namespaced map.
func (r *Runtime) injectGlobalConfigAPI(L *lua.LState, claudio *lua.LTable) {
	// claudio.config — delegate to the canonical implementation.
	r.registerConfigSettingsAPI(L, claudio)

	// claudio.ui stub entries — ensures defaults.lua never errors if ui api not yet wired.
	// Real implementations are added by registerTUIAPI (api_tui.go).
	uiTable, ok := L.GetField(claudio, "ui").(*lua.LTable)
	if !ok || uiTable == nil {
		uiTable = L.NewTable()
		L.SetField(claudio, "ui", uiTable)
	}
	if L.GetField(uiTable, "set_theme") == lua.LNil {
		L.SetField(uiTable, "set_theme", L.NewFunction(func(L *lua.LState) int { return 0 }))
	}
	if L.GetField(uiTable, "set_border") == lua.LNil {
		L.SetField(uiTable, "set_border", L.NewFunction(func(L *lua.LState) int { return 0 }))
	}
}

// apiGlobalConfigSet returns the claudio.config.set(key, value) binding.
// Supported keys mirror the fields of config.Settings used in defaults.lua.
//
// Lua usage:
//
//	claudio.config.set("model", "claude-sonnet-4-6")
func (r *Runtime) apiGlobalConfigSet() lua.LGFunction {
	return func(L *lua.LState) int {
		key := L.CheckString(1)
		val := L.Get(2)
		r.applyConfigSetting(key, val)
		return 0
	}
}

// apiGlobalConfigGet returns the claudio.config.get(key) binding.
//
// Lua usage:
//
//	local m = claudio.config.get("model")
func (r *Runtime) apiGlobalConfigGet() lua.LGFunction {
	return func(L *lua.LState) int {
		key := L.CheckString(1)
		L.Push(r.configSettingToLua(L, key))
		return 1
	}
}

// applyConfigSetting writes a Lua value into the appropriate Settings field.
func (r *Runtime) applyConfigSetting(key string, val lua.LValue) {
	switch key {
	case "model":
		if s, ok := val.(lua.LString); ok {
			r.cfg.Model = string(s)
		}
	case "smallModel":
		if s, ok := val.(lua.LString); ok {
			r.cfg.SmallModel = string(s)
		}
	case "permissionMode":
		if s, ok := val.(lua.LString); ok {
			r.cfg.PermissionMode = string(s)
		}
	case "compactMode":
		if s, ok := val.(lua.LString); ok {
			r.cfg.CompactMode = string(s)
		}
	case "compactKeepN":
		if n, ok := val.(lua.LNumber); ok {
			r.cfg.CompactKeepN = int(n)
		}
	case "sessionPersist":
		if b, ok := val.(lua.LBool); ok {
			r.cfg.SessionPersist = bool(b)
		}
	case "hookProfile":
		if s, ok := val.(lua.LString); ok {
			r.cfg.HookProfile = string(s)
		}
	case "autoCompact":
		if b, ok := val.(lua.LBool); ok {
			r.cfg.AutoCompact = bool(b)
		}
	case "caveman":
		if b, ok := val.(lua.LBool); ok {
			bv := bool(b)
			r.cfg.Caveman = &bv
		}
	case "outputStyle":
		if s, ok := val.(lua.LString); ok {
			r.cfg.OutputStyle = string(s)
		}
	case "outputFilter":
		if b, ok := val.(lua.LBool); ok {
			r.cfg.OutputFilter = bool(b)
		}
	case "autoMemoryExtract":
		if b, ok := val.(lua.LBool); ok {
			bv := bool(b)
			r.cfg.AutoMemoryExtract = &bv
		}
	case "memorySelection":
		if s, ok := val.(lua.LString); ok {
			r.cfg.MemorySelection = string(s)
		}
	}
}

// configSettingToLua reads a Settings field by key and returns its Lua value.
func (r *Runtime) configSettingToLua(L *lua.LState, key string) lua.LValue {
	switch key {
	case "model":
		return lua.LString(r.cfg.Model)
	case "smallModel":
		return lua.LString(r.cfg.SmallModel)
	case "permissionMode":
		return lua.LString(r.cfg.PermissionMode)
	case "compactMode":
		return lua.LString(r.cfg.CompactMode)
	case "compactKeepN":
		return lua.LNumber(r.cfg.CompactKeepN)
	case "sessionPersist":
		return lua.LBool(r.cfg.SessionPersist)
	case "hookProfile":
		return lua.LString(r.cfg.HookProfile)
	case "autoCompact":
		return lua.LBool(r.cfg.AutoCompact)
	case "caveman":
		if r.cfg.Caveman != nil {
			return lua.LBool(*r.cfg.Caveman)
		}
		return lua.LNil
	case "outputStyle":
		return lua.LString(r.cfg.OutputStyle)
	case "outputFilter":
		return lua.LBool(r.cfg.OutputFilter)
	case "autoMemoryExtract":
		if r.cfg.AutoMemoryExtract != nil {
			return lua.LBool(*r.cfg.AutoMemoryExtract)
		}
		return lua.LNil
	case "memorySelection":
		return lua.LString(r.cfg.MemorySelection)
	}
	return lua.LNil
}
