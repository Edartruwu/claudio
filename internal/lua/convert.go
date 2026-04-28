package lua

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	lua "github.com/yuin/gopher-lua"
)

// jsonToLuaValue converts a Go value (from json.Unmarshal) to a Lua LValue.
func jsonToLuaValue(L *lua.LState, v any) lua.LValue {
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
	case []any:
		tbl := L.NewTable()
		for i, item := range val {
			tbl.RawSetInt(i+1, jsonToLuaValue(L, item))
		}
		return tbl
	case map[string]any:
		tbl := L.NewTable()
		for k, item := range val {
			tbl.RawSetString(k, jsonToLuaValue(L, item))
		}
		return tbl
	default:
		return lua.LString(fmt.Sprintf("%v", val))
	}
}

// jsonBytesToLuaTable decodes JSON bytes into a Lua table.
func jsonBytesToLuaTable(L *lua.LState, data json.RawMessage) (lua.LValue, error) {
	if len(data) == 0 {
		return lua.LNil, nil
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return lua.LNil, err
	}
	return jsonToLuaValue(L, v), nil
}

// luaValueToGo converts a Lua LValue to a Go value suitable for json.Marshal.
func luaValueToGo(lv lua.LValue) any {
	switch v := lv.(type) {
	case *lua.LNilType:
		return nil
	case lua.LBool:
		return bool(v)
	case lua.LNumber:
		return float64(v)
	case lua.LString:
		return string(v)
	case *lua.LTable:
		return luaTableToGo(v)
	default:
		return v.String()
	}
}

// luaTableToGo converts a Lua table to either a []any (array) or map[string]any.
// Heuristic: if all keys are consecutive integers starting at 1, treat as array.
func luaTableToGo(tbl *lua.LTable) any {
	maxN := tbl.MaxN()
	if maxN > 0 {
		// Check if it's purely an array (no string keys)
		hasStringKeys := false
		tbl.ForEach(func(k, _ lua.LValue) {
			if _, ok := k.(lua.LString); ok {
				hasStringKeys = true
			}
		})
		if !hasStringKeys {
			arr := make([]any, 0, maxN)
			for i := 1; i <= maxN; i++ {
				arr = append(arr, luaValueToGo(tbl.RawGetInt(i)))
			}
			return arr
		}
	}

	// Map
	m := make(map[string]any)
	tbl.ForEach(func(k, v lua.LValue) {
		var key string
		switch kt := k.(type) {
		case lua.LString:
			key = string(kt)
		case lua.LNumber:
			key = strconv.FormatFloat(float64(kt), 'f', -1, 64)
		default:
			key = k.String()
		}
		m[key] = luaValueToGo(v)
	})
	return m
}

// luaValueToJSON converts a Lua value to JSON bytes.
func luaValueToJSON(lv lua.LValue) (json.RawMessage, error) {
	return json.Marshal(luaValueToGo(lv))
}

// tableString extracts a string field from a Lua table.
func tableString(L *lua.LState, tbl *lua.LTable, key string) string {
	lv := L.GetField(tbl, key)
	if s, ok := lv.(lua.LString); ok {
		return string(s)
	}
	return ""
}

// tableStringSlice extracts a string slice from a Lua table field.
func tableStringSlice(L *lua.LState, tbl *lua.LTable, key string) []string {
	lv := L.GetField(tbl, key)
	arrTbl, ok := lv.(*lua.LTable)
	if !ok {
		return nil
	}
	var result []string
	arrTbl.ForEach(func(_, v lua.LValue) {
		if s, ok := v.(lua.LString); ok {
			result = append(result, string(s))
		}
	})
	sort.Strings(result)
	return result
}
