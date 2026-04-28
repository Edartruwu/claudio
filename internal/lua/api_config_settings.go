package lua

import (
	"fmt"

	"github.com/Abraxas-365/claudio/internal/config"
	lua "github.com/yuin/gopher-lua"
)

// configChangeHandler pairs a Lua function with the LState it belongs to.
// Each plugin has its own LState, so we carry it alongside the function.
type configChangeHandler struct {
	L  *lua.LState
	fn *lua.LFunction
}

// registerConfigSettingsAPI wires the claudio.config sub-table into claudioTbl.
//
// Lua surface:
//
//	claudio.config.get("model")               -- returns current value or nil
//	claudio.config.set("model", "claude-...")  -- mutates Settings in-place
//	claudio.config.on_change("model", fn)      -- fn(new, old) called on every set()
func (r *Runtime) registerConfigSettingsAPI(L *lua.LState, claudioTbl *lua.LTable) {
	cfg := L.NewTable()

	L.SetField(cfg, "get", L.NewFunction(r.apiConfigGet()))
	L.SetField(cfg, "set", L.NewFunction(r.apiConfigSet(L)))
	L.SetField(cfg, "on_change", L.NewFunction(r.apiConfigOnChange(L)))

	L.SetField(claudioTbl, "config", cfg)
}

// ── get ───────────────────────────────────────────────────────────────────────

func (r *Runtime) apiConfigGet() lua.LGFunction {
	return func(L *lua.LState) int {
		key := L.CheckString(1)
		r.mu.Lock()
		s := r.cfg
		r.mu.Unlock()
		if s == nil {
			L.Push(lua.LNil)
			return 1
		}
		val, err := settingsGetValue(L, s, key)
		if err != nil {
			// Unknown key → nil (graceful)
			L.Push(lua.LNil)
			return 1
		}
		L.Push(val)
		return 1
	}
}

// ── set ───────────────────────────────────────────────────────────────────────

func (r *Runtime) apiConfigSet(callerL *lua.LState) lua.LGFunction {
	return func(L *lua.LState) int {
		key := L.CheckString(1)
		newLVal := L.Get(2)

		r.mu.Lock()
		s := r.cfg
		r.mu.Unlock()

		if s == nil {
			L.RaiseError("claudio.config.set: settings not initialised")
			return 0
		}

		// Capture old value before mutation
		oldLVal, _ := settingsGetValue(L, s, key)

		if err := settingsSetValue(s, key, newLVal); err != nil {
			L.RaiseError("claudio.config.set: %s", err.Error())
			return 0
		}

		// Fire on_change handlers (re-read new value for canonical form)
		newCanonical, _ := settingsGetValue(L, s, key)
		r.fireChangeHandlers(key, newCanonical, oldLVal)

		return 0
	}
}

// ── on_change ─────────────────────────────────────────────────────────────────

func (r *Runtime) apiConfigOnChange(callerL *lua.LState) lua.LGFunction {
	return func(L *lua.LState) int {
		key := L.CheckString(1)
		fn := L.CheckFunction(2)

		r.mu.Lock()
		if r.changeHandlers == nil {
			r.changeHandlers = make(map[string][]configChangeHandler)
		}
		r.changeHandlers[key] = append(r.changeHandlers[key], configChangeHandler{L: L, fn: fn})
		r.mu.Unlock()

		return 0
	}
}

// fireChangeHandlers calls all registered on_change handlers for key.
// oldVal/newVal are LValues from the *calling* LState; each handler's own
// LState receives equivalent primitive values re-pushed there.
func (r *Runtime) fireChangeHandlers(key string, newVal, oldVal lua.LValue) {
	r.mu.Lock()
	handlers := append([]configChangeHandler(nil), r.changeHandlers[key]...)
	r.mu.Unlock()

	for _, h := range handlers {
		h.L.Push(h.fn)
		h.L.Push(luaValueForState(h.L, newVal))
		h.L.Push(luaValueForState(h.L, oldVal))
		if err := h.L.PCall(2, 0, nil); err != nil {
			// Log but don't crash — same policy as hook errors
			_ = err
		}
	}
}

// luaValueForState copies a primitive LValue into a (possibly different) LState.
// Tables are not supported here — config values are always scalars.
func luaValueForState(_ *lua.LState, v lua.LValue) lua.LValue {
	return v // LString, LNumber, LBool, LNil are all value types — safe to copy
}

// ── helpers ───────────────────────────────────────────────────────────────────

// settingsGetValue reads a named Settings field and returns it as an LValue.
// Returns (LNil, error) for unknown keys.
func settingsGetValue(L *lua.LState, s *config.Settings, key string) (lua.LValue, error) {
	switch key {
	case "model":
		return lua.LString(s.Model), nil
	case "smallModel":
		return lua.LString(s.SmallModel), nil
	case "permissionMode":
		return lua.LString(s.PermissionMode), nil
	case "compactMode":
		return lua.LString(s.CompactMode), nil
	case "compactKeepN":
		return lua.LNumber(s.CompactKeepN), nil
	case "sessionPersist":
		return lua.LBool(s.SessionPersist), nil
	case "hookProfile":
		return lua.LString(s.HookProfile), nil
	case "autoCompact":
		return lua.LBool(s.AutoCompact), nil
	case "caveman":
		if s.Caveman == nil {
			return lua.LFalse, nil
		}
		return lua.LBool(*s.Caveman), nil
	case "outputStyle":
		return lua.LString(s.OutputStyle), nil
	case "outputFilter":
		return lua.LBool(s.OutputFilter), nil
	case "autoMemoryExtract":
		if s.AutoMemoryExtract == nil {
			return lua.LFalse, nil
		}
		return lua.LBool(*s.AutoMemoryExtract), nil
	case "memorySelection":
		return lua.LString(s.MemorySelection), nil
	case "maxBudget":
		return lua.LNumber(s.MaxBudget), nil
	default:
		return lua.LNil, fmt.Errorf("unknown config key %q", key)
	}
}

// settingsSetValue mutates the named Settings field with the given LValue.
// Returns an error for unknown keys or type mismatches.
func settingsSetValue(s *config.Settings, key string, val lua.LValue) error {
	switch key {
	case "model":
		s.Model = luaToString(val)
	case "smallModel":
		s.SmallModel = luaToString(val)
	case "permissionMode":
		s.PermissionMode = luaToString(val)
	case "compactMode":
		s.CompactMode = luaToString(val)
	case "compactKeepN":
		n, err := luaToInt(val)
		if err != nil {
			return fmt.Errorf("compactKeepN: %w", err)
		}
		s.CompactKeepN = n
	case "sessionPersist":
		b, err := luaToBool(val)
		if err != nil {
			return fmt.Errorf("sessionPersist: %w", err)
		}
		s.SessionPersist = b
	case "hookProfile":
		s.HookProfile = luaToString(val)
	case "autoCompact":
		b, err := luaToBool(val)
		if err != nil {
			return fmt.Errorf("autoCompact: %w", err)
		}
		s.AutoCompact = b
	case "caveman":
		b, err := luaToBool(val)
		if err != nil {
			return fmt.Errorf("caveman: %w", err)
		}
		s.Caveman = &b
	case "outputStyle":
		s.OutputStyle = luaToString(val)
	case "outputFilter":
		b, err := luaToBool(val)
		if err != nil {
			return fmt.Errorf("outputFilter: %w", err)
		}
		s.OutputFilter = b
	case "autoMemoryExtract":
		b, err := luaToBool(val)
		if err != nil {
			return fmt.Errorf("autoMemoryExtract: %w", err)
		}
		s.AutoMemoryExtract = &b
	case "memorySelection":
		s.MemorySelection = luaToString(val)
	case "maxBudget":
		f, err := luaToFloat(val)
		if err != nil {
			return fmt.Errorf("maxBudget: %w", err)
		}
		s.MaxBudget = f
	default:
		return fmt.Errorf("unknown config key %q", key)
	}
	return nil
}

// ── type coercions ────────────────────────────────────────────────────────────

func luaToString(v lua.LValue) string {
	if s, ok := v.(lua.LString); ok {
		return string(s)
	}
	return v.String()
}

func luaToInt(v lua.LValue) (int, error) {
	if n, ok := v.(lua.LNumber); ok {
		return int(n), nil
	}
	return 0, fmt.Errorf("expected number, got %s", v.Type())
}

func luaToBool(v lua.LValue) (bool, error) {
	if b, ok := v.(lua.LBool); ok {
		return bool(b), nil
	}
	return false, fmt.Errorf("expected boolean, got %s", v.Type())
}

func luaToFloat(v lua.LValue) (float64, error) {
	if n, ok := v.(lua.LNumber); ok {
		return float64(n), nil
	}
	return 0, fmt.Errorf("expected number, got %s", v.Type())
}
