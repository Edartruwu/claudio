// Package lua — UI extension APIs exposed as claudio.ui.*
package lua

import (
	"encoding/json"
	"log"
	"time"

	"github.com/Abraxas-365/claudio/internal/bus"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
	lua "github.com/yuin/gopher-lua"
)

// SidebarBlockDef holds a sidebar block registered by a Lua plugin.
type SidebarBlockDef struct {
	ID       string
	Title    string
	Plugin   *loadedPlugin
	RenderFn *lua.LFunction
}

// StatuslineCtx holds contextual data passed to the Lua statusline function.
type StatuslineCtx struct {
	Mode    string
	Model   string
	Tokens  int
	Session string
}

// WhichkeyEntry is a single key binding contributed by a plugin.
type WhichkeyEntry struct {
	Key  string
	Desc string
}

// WhichkeyGroup is a named group of key bindings registered by a plugin.
type WhichkeyGroup struct {
	Group    string
	Bindings []WhichkeyEntry
}

// PaletteEntry is a command palette entry registered by a plugin.
type PaletteEntry struct {
	Name        string
	Action      string
	Description string
}

// RenderStatusline calls the Lua statusline function with the given ctx.
// Returns "" if no function is registered or on any error.
func (r *Runtime) RenderStatusline(ctx StatuslineCtx) string {
	r.uiMu.RLock()
	fn := r.StatuslineFn
	p := r.statuslinePlugin
	r.uiMu.RUnlock()

	if fn == nil || p == nil {
		return ""
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	defer func() {
		if rv := recover(); rv != nil {
			log.Printf("[lua] RenderStatusline panic: %v", rv)
		}
	}()

	ctxTbl := p.L.NewTable()
	p.L.SetField(ctxTbl, "mode", lua.LString(ctx.Mode))
	p.L.SetField(ctxTbl, "model", lua.LString(ctx.Model))
	p.L.SetField(ctxTbl, "tokens", lua.LNumber(ctx.Tokens))
	p.L.SetField(ctxTbl, "session", lua.LString(ctx.Session))

	if err := p.L.CallByParam(lua.P{
		Fn:      fn,
		NRet:    1,
		Protect: true,
	}, ctxTbl); err != nil {
		log.Printf("[lua] statusline fn error: %v", err)
		return ""
	}

	result := p.L.Get(-1)
	p.L.Pop(1)

	if s, ok := result.(lua.LString); ok {
		return string(s)
	}
	return ""
}

// PendingWhichkeyGroups returns all whichkey groups registered by plugins.
func (r *Runtime) PendingWhichkeyGroups() []WhichkeyGroup {
	r.uiMu.RLock()
	defer r.uiMu.RUnlock()
	out := make([]WhichkeyGroup, len(r.pendingWhichkeyGroups))
	copy(out, r.pendingWhichkeyGroups)
	return out
}

// PendingPaletteEntries returns all palette entries registered by plugins.
func (r *Runtime) PendingPaletteEntries() []PaletteEntry {
	r.uiMu.RLock()
	defer r.uiMu.RUnlock()
	out := make([]PaletteEntry, len(r.pendingPaletteEntries))
	copy(out, r.pendingPaletteEntries)
	return out
}

// CallRender calls the Lua render function for this sidebar block with the given dimensions.
func (b *SidebarBlockDef) CallRender(width, height int) string {
	if b.Plugin == nil || b.RenderFn == nil {
		return ""
	}
	b.Plugin.mu.Lock()
	defer b.Plugin.mu.Unlock()
	defer func() {
		if rv := recover(); rv != nil {
			log.Printf("[lua] sidebar block %q render panic: %v", b.ID, rv)
		}
	}()
	if err := b.Plugin.L.CallByParam(lua.P{
		Fn:      b.RenderFn,
		NRet:    1,
		Protect: true,
	}, lua.LNumber(width), lua.LNumber(height)); err != nil {
		log.Printf("[lua] sidebar block %q render error: %v", b.ID, err)
		return ""
	}
	result := b.Plugin.L.Get(-1)
	b.Plugin.L.Pop(1)
	if s, ok := result.(lua.LString); ok {
		return string(s)
	}
	return ""
}

// GetSidebarBlocks returns all sidebar blocks registered by plugins.
func (r *Runtime) GetSidebarBlocks() []SidebarBlockDef {
	r.uiMu.RLock()
	defer r.uiMu.RUnlock()
	out := make([]SidebarBlockDef, len(r.pendingSidebarBlocks))
	copy(out, r.pendingSidebarBlocks)
	return out
}

// injectUIAPI registers claudio.ui.* bindings and returns the table.
func (r *Runtime) injectUIAPI(L *lua.LState, plugin *loadedPlugin) *lua.LTable {
	ui := L.NewTable()
	L.SetField(ui, "set_statusline", L.NewFunction(r.apiSetStatusline(plugin)))
	L.SetField(ui, "popup", L.NewFunction(r.apiPopup(plugin)))
	L.SetField(ui, "register_whichkey", L.NewFunction(r.apiRegisterWhichkey()))
	L.SetField(ui, "register_palette_entry", L.NewFunction(r.apiRegisterPaletteEntry()))
	L.SetField(ui, "register_sidebar_block", L.NewFunction(r.apiRegisterSidebarBlock(plugin)))
	// Color / theme controls
	L.SetField(ui, "set_color", L.NewFunction(func(L *lua.LState) int {
		slot := L.CheckString(1)
		hex := L.CheckString(2)
		if err := styles.SetColor(slot, hex); err != nil {
			L.RaiseError("set_color: %v", err)
		}
		styles.RebuildAll()
		return 0
	}))
	L.SetField(ui, "set_theme", L.NewFunction(func(L *lua.LState) int {
		tbl := L.CheckTable(1)
		colors := map[string]string{}
		tbl.ForEach(func(k, v lua.LValue) {
			if ks, ok := k.(lua.LString); ok {
				colors[string(ks)] = lua.LVAsString(v)
			}
		})
		styles.SetTheme(colors)
		styles.RebuildAll()
		return 0
	}))
	L.SetField(ui, "set_border", L.NewFunction(func(L *lua.LState) int {
		styles.SetBorderStyle(L.CheckString(1))
		styles.RebuildAll()
		return 0
	}))
	L.SetField(ui, "get_colors", L.NewFunction(func(L *lua.LState) int {
		tbl := L.NewTable()
		for k, v := range styles.GetColors() {
			L.SetField(tbl, k, lua.LString(v))
		}
		L.Push(tbl)
		return 1
	}))
	return ui
}



// apiSetStatusline returns the claudio.ui.set_statusline(fn) binding.
//
// Lua usage:
//
//	claudio.ui.set_statusline(function(ctx)
//	  return ctx.mode .. " | " .. ctx.model .. " | tokens:" .. ctx.tokens
//	end)
func (r *Runtime) apiSetStatusline(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		fn := L.CheckFunction(1)
		r.uiMu.Lock()
		r.StatuslineFn = fn
		r.statuslinePlugin = plugin
		r.uiMu.Unlock()
		return 0
	}
}

// apiPopup returns the claudio.ui.popup(opts) binding.
// Publishes a "ui.popup" bus event consumed by root.go.
//
// Lua usage:
//
//	claudio.ui.popup({ title = "Plugin Output", content = "Hello!", width = 60, height = 10 })
func (r *Runtime) apiPopup(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		opts := L.CheckTable(1)

		title := ""
		content := ""
		width := 60
		height := 10

		if v := opts.RawGetString("title"); v != lua.LNil {
			title = lua.LVAsString(v)
		}
		if v := opts.RawGetString("content"); v != lua.LNil {
			content = lua.LVAsString(v)
		}
		if v := opts.RawGetString("width"); v != lua.LNil {
			if n, ok := v.(lua.LNumber); ok && n > 0 {
				width = int(n)
			}
		}
		if v := opts.RawGetString("height"); v != lua.LNil {
			if n, ok := v.(lua.LNumber); ok && n > 0 {
				height = int(n)
			}
		}

		payload, _ := json.Marshal(map[string]interface{}{
			"title":   title,
			"content": content,
			"width":   width,
			"height":  height,
		})

		r.bus.Publish(bus.Event{
			Type:      "ui.popup",
			Payload:   payload,
			Timestamp: time.Now(),
		})
		return 0
	}
}

// apiRegisterWhichkey returns the claudio.ui.register_whichkey(group, bindings) binding.
//
// Lua usage:
//
//	claudio.ui.register_whichkey("Plugin", {
//	  { key = "p", desc = "Open plugin panel" },
//	  { key = "r", desc = "Reload plugin" },
//	})
func (r *Runtime) apiRegisterWhichkey() lua.LGFunction {
	return func(L *lua.LState) int {
		group := L.CheckString(1)
		bindingsTbl := L.CheckTable(2)

		var entries []WhichkeyEntry
		bindingsTbl.ForEach(func(_, v lua.LValue) {
			tbl, ok := v.(*lua.LTable)
			if !ok {
				return
			}
			key := lua.LVAsString(tbl.RawGetString("key"))
			desc := lua.LVAsString(tbl.RawGetString("desc"))
			if key != "" {
				entries = append(entries, WhichkeyEntry{Key: key, Desc: desc})
			}
		})

		r.uiMu.Lock()
		r.pendingWhichkeyGroups = append(r.pendingWhichkeyGroups, WhichkeyGroup{
			Group:    group,
			Bindings: entries,
		})
		r.uiMu.Unlock()
		return 0
	}
}

// apiRegisterPaletteEntry returns the claudio.ui.register_palette_entry(entry) binding.
//
// Lua usage:
//
//	claudio.ui.register_palette_entry({
//	  name    = "Reload Plugins",
//	  action  = "reload_plugins",
//	  handler = function() claudio.notify("reloading...") end,
//	})
func (r *Runtime) apiRegisterPaletteEntry() lua.LGFunction {
	return func(L *lua.LState) int {
		opts := L.CheckTable(1)

		name := lua.LVAsString(opts.RawGetString("name"))
		action := lua.LVAsString(opts.RawGetString("action"))
		desc := ""
		if v := opts.RawGetString("description"); v != lua.LNil {
			desc = lua.LVAsString(v)
		}
		// handler fn is intentionally ignored here;
		// palette routing is done by Name via bus events or commands.

		r.uiMu.Lock()
		r.pendingPaletteEntries = append(r.pendingPaletteEntries, PaletteEntry{
			Name:        name,
			Action:      action,
			Description: desc,
		})
		r.uiMu.Unlock()
		return 0
	}
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
