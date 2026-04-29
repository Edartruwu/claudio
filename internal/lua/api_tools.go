package lua

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/Abraxas-365/claudio/internal/tools"
	lua "github.com/yuin/gopher-lua"
)

// apiRegisterTool returns the claudio.register_tool(tbl) binding.
//
// Lua usage:
//
//	claudio.register_tool({
//	  name        = "my_tool",
//	  description = "Does something",
//	  schema      = '{"type":"object","properties":{...}}',
//	  execute     = function(input) return { content = "result" } end,
//	})
func (r *Runtime) apiRegisterTool(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		tbl := L.CheckTable(1)
		name := tableString(L, tbl, "name")
		desc := tableString(L, tbl, "description")
		schemaStr := tableString(L, tbl, "schema")
		executeFn := L.GetField(tbl, "execute")

		if name == "" {
			L.ArgError(1, "register_tool: name required")
			return 0
		}
		if executeFn == lua.LNil {
			L.ArgError(1, "register_tool: execute function required")
			return 0
		}

		var schema json.RawMessage
		if schemaStr != "" {
			schema = json.RawMessage(schemaStr)
		} else {
			schema = json.RawMessage(`{"type":"object","properties":{}}`)
		}

		// Parse visibility filter fields (same pattern as api_skills.go).
		agentsList := tableStringSlice(L, tbl, "agents")
		capsList := tableStringSlice(L, tbl, "capabilities")
		requireTeam := tableBool(L, tbl, "require_team")

		// Parse deferred: bool | table { agents = {...}, capabilities = {...} }
		var deferCond tools.DeferredCondition
		var searchHint string
		deferVal := L.GetField(tbl, "deferred")
		switch v := deferVal.(type) {
		case lua.LBool:
			if bool(v) {
				deferCond.Always = true
			}
		case *lua.LTable:
			deferCond.Agents = tableStringSlice(L, v, "agents")
			deferCond.Capabilities = tableStringSlice(L, v, "capabilities")
		}
		hintVal := L.GetField(tbl, "search_hint")
		if s, ok := hintVal.(lua.LString); ok {
			searchHint = string(s)
		}

		tool := &luaTool{
			plugin:      plugin,
			toolName:    name,
			desc:        desc,
			schema:      schema,
			executeFn:   executeFn,
			filter: tools.ToolFilter{
				Agents:       agentsList,
				Capabilities: capsList,
				RequireTeam:  requireTeam,
			},
			deferCond:  deferCond,
			searchHint: searchHint,
		}
		r.toolReg.Register(tool)
		return 0
	}
}

// luaTool implements tools.Tool backed by a Lua function.
// It optionally implements FilterableTool, DeferrableTool, and ConditionalDeferrableTool.
type luaTool struct {
	plugin     *loadedPlugin
	toolName   string
	desc       string
	schema     json.RawMessage
	executeFn  lua.LValue
	mu         sync.Mutex // serialize access to plugin.L
	filter     tools.ToolFilter
	deferCond  tools.DeferredCondition
	searchHint string
}

func (t *luaTool) Name() string                            { return t.toolName }
func (t *luaTool) Description() string                     { return t.desc }
func (t *luaTool) InputSchema() json.RawMessage            { return t.schema }
func (t *luaTool) IsReadOnly() bool                        { return false }
func (t *luaTool) RequiresApproval(_ json.RawMessage) bool { return false }

// Filter implements tools.FilterableTool.
func (t *luaTool) Filter() tools.ToolFilter { return t.filter }

// isDeferrable returns true if any deferral condition is set.
func (t *luaTool) isDeferrable() bool {
	return t.deferCond.Always || len(t.deferCond.Agents) > 0 || len(t.deferCond.Capabilities) > 0
}

// ShouldDefer implements tools.DeferrableTool.
func (t *luaTool) ShouldDefer() bool { return t.deferCond.Always }

// SearchHint implements tools.DeferrableTool.
func (t *luaTool) SearchHint() string {
	if t.searchHint != "" {
		return t.searchHint
	}
	return t.desc
}

// ShouldDeferForAgent implements tools.ConditionalDeferrableTool.
func (t *luaTool) ShouldDeferForAgent(agentName string, agentCaps []string) bool {
	return t.deferCond.ShouldDeferForAgent(agentName, agentCaps)
}

// Execute calls the Lua execute function with the JSON input decoded to a Lua table.
func (t *luaTool) Execute(ctx context.Context, input json.RawMessage) (result *tools.Result, retErr error) {
	// Catch panics from gopher-lua
	defer func() {
		if rv := recover(); rv != nil {
			retErr = fmt.Errorf("lua plugin %s tool %s panic: %v", t.plugin.name, t.toolName, rv)
		}
	}()

	t.mu.Lock()
	defer t.mu.Unlock()

	L := t.plugin.L

	// Decode input JSON → Lua table
	inputVal, err := jsonBytesToLuaTable(L, input)
	if err != nil {
		return &tools.Result{Content: fmt.Sprintf("invalid input JSON: %v", err), IsError: true}, nil
	}

	// Call execute(input) → result table
	if err := L.CallByParam(lua.P{
		Fn:      t.executeFn,
		NRet:    1,
		Protect: true,
	}, inputVal); err != nil {
		return &tools.Result{Content: fmt.Sprintf("lua error: %v", err), IsError: true}, nil
	}

	// Get return value
	ret := L.Get(-1)
	L.Pop(1)

	return luaReturnToResult(L, ret), nil
}

// luaReturnToResult converts a Lua return value to a tools.Result.
func luaReturnToResult(L *lua.LState, lv lua.LValue) *tools.Result {
	switch v := lv.(type) {
	case lua.LString:
		return &tools.Result{Content: string(v)}
	case *lua.LTable:
		content := tableString(L, v, "content")
		isError := false
		if errVal := L.GetField(v, "is_error"); errVal == lua.LTrue {
			isError = true
		}
		return &tools.Result{Content: content, IsError: isError}
	case *lua.LNilType:
		return &tools.Result{Content: ""}
	default:
		return &tools.Result{Content: lv.String()}
	}
}
