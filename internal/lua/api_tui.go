package lua

import (
	"fmt"
	"strings"

	"github.com/Abraxas-365/claudio/internal/tui/styles"
	"github.com/charmbracelet/lipgloss"
	lua "github.com/yuin/gopher-lua"
)

// SidebarBlockDef holds a sidebar block registered by a Lua plugin.
type SidebarBlockDef struct {
	ID       string
	Title    string
	Plugin   *loadedPlugin
	RenderFn *lua.LFunction
}

// registerTUIAPI registers the claudio.ui table into L.
//
// Lua surface:
//
//	claudio.ui.set_color(slot, hex)          -- mutate one semantic color slot
//	claudio.ui.set_theme(table)              -- batch mutate colors
//	claudio.ui.set_border(style)             -- "rounded"|"block"|"double"|"normal"|"hidden"
//	claudio.ui.get_colors()                  -- returns table of current hex values
func registerTUIAPI(L *lua.LState, claudioTbl *lua.LTable) {
	ui := L.NewTable()

	L.SetField(ui, "set_color", L.NewFunction(apiSetColor))
	L.SetField(ui, "set_theme", L.NewFunction(apiSetTheme))
	L.SetField(ui, "set_border", L.NewFunction(apiSetBorder))
	L.SetField(ui, "get_colors", L.NewFunction(apiGetColors))

	L.SetField(claudioTbl, "ui", ui)
}

// colorSlots maps Lua slot names → pointers to the styles package color vars.
var colorSlots = map[string]*lipgloss.Color{
	"primary":    &styles.Primary,
	"secondary":  &styles.Secondary,
	"success":    &styles.Success,
	"warning":    &styles.Warning,
	"error":      &styles.Error,
	"muted":      &styles.Muted,
	"surface":    &styles.Surface,
	"surface_alt": &styles.SurfaceAlt,
	"text":       &styles.Text,
	"dim":        &styles.Dim,
	"subtle":     &styles.Subtle,
	"orange":     &styles.Orange,
	"aqua":       &styles.Aqua,
}

// apiSetColor implements claudio.ui.set_color(slot, hex).
func apiSetColor(L *lua.LState) int {
	slot := strings.ToLower(L.CheckString(1))
	hex := L.CheckString(2)

	ptr, ok := colorSlots[slot]
	if !ok {
		L.RaiseError("unknown color slot %q — valid slots: primary, secondary, success, warning, error, muted, surface, surface_alt, text, dim, subtle, orange, aqua", slot)
		return 0
	}
	if !strings.HasPrefix(hex, "#") {
		L.RaiseError("color must be a hex string starting with #, got %q", hex)
		return 0
	}
	*ptr = lipgloss.Color(hex)
	styles.RebuildAll()
	return 0
}

// apiSetTheme implements claudio.ui.set_theme(table).
func apiSetTheme(L *lua.LState) int {
	tbl := L.CheckTable(1)

	var errs []string
	tbl.ForEach(func(k, v lua.LValue) {
		slot := strings.ToLower(k.String())
		hex, ok := v.(lua.LString)
		if !ok {
			errs = append(errs, fmt.Sprintf("slot %q: value must be a string, got %T", slot, v))
			return
		}
		ptr, found := colorSlots[slot]
		if !found {
			errs = append(errs, fmt.Sprintf("unknown color slot %q", slot))
			return
		}
		if !strings.HasPrefix(string(hex), "#") {
			errs = append(errs, fmt.Sprintf("slot %q: color must start with #, got %q", slot, hex))
			return
		}
		*ptr = lipgloss.Color(string(hex))
	})

	if len(errs) > 0 {
		L.RaiseError("set_theme errors:\n  %s", strings.Join(errs, "\n  "))
		return 0
	}

	styles.RebuildAll()
	return 0
}

// apiSetBorder implements claudio.ui.set_border(style).
func apiSetBorder(L *lua.LState) int {
	name := strings.ToLower(L.CheckString(1))
	valid := map[string]bool{"rounded": true, "block": true, "double": true, "normal": true, "hidden": true}
	if !valid[name] {
		L.RaiseError("unknown border style %q — valid: rounded, block, double, normal, hidden", name)
		return 0
	}
	styles.SetBorderStyle(name)
	styles.RebuildAll()
	return 0
}

// apiGetColors implements claudio.ui.get_colors() → table.
func apiGetColors(L *lua.LState) int {
	tbl := L.NewTable()
	for slot, ptr := range colorSlots {
		L.SetField(tbl, slot, lua.LString(string(*ptr)))
	}
	L.Push(tbl)
	return 1
}

// injectPluginUIAPI adds plugin-aware UI bindings to the claudio.ui sub-table.
// Called from injectAPI after injectGlobalConfigAPI has set up the ui table.
func (r *Runtime) injectPluginUIAPI(L *lua.LState, plugin *loadedPlugin, claudio *lua.LTable) {
	uiTable, ok := L.GetField(claudio, "ui").(*lua.LTable)
	if !ok || uiTable == nil {
		uiTable = L.NewTable()
		L.SetField(claudio, "ui", uiTable)
	}
	L.SetField(uiTable, "register_sidebar_block", L.NewFunction(r.apiRegisterSidebarBlock(plugin)))
}

// apiRegisterSidebarBlock implements claudio.ui.register_sidebar_block({id, title, render}).
//
// Lua surface:
//
//	claudio.ui.register_sidebar_block({
//	  id     = "my-block",
//	  title  = "My Plugin",
//	  render = function(ctx) return "content string" end,
//	})
func (r *Runtime) apiRegisterSidebarBlock(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		tbl := L.CheckTable(1)

		idVal := L.GetField(tbl, "id")
		id, ok := idVal.(lua.LString)
		if !ok || string(id) == "" {
			L.RaiseError("register_sidebar_block: id must be a non-empty string")
			return 0
		}

		titleVal := L.GetField(tbl, "title")
		title, ok := titleVal.(lua.LString)
		if !ok {
			L.RaiseError("register_sidebar_block: title must be a string")
			return 0
		}

		renderVal := L.GetField(tbl, "render")
		renderFn, ok := renderVal.(*lua.LFunction)
		if !ok {
			L.RaiseError("register_sidebar_block: render must be a function")
			return 0
		}

		def := SidebarBlockDef{
			ID:       string(id),
			Title:    string(title),
			Plugin:   plugin,
			RenderFn: renderFn,
		}

		r.pendingSidebarBlocksMu.Lock()
		r.pendingSidebarBlocks = append(r.pendingSidebarBlocks, def)
		r.pendingSidebarBlocksMu.Unlock()

		return 0
	}
}
