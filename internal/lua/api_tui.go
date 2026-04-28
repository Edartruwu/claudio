// Package lua — UI extension APIs exposed as claudio.ui.*
package lua

import (
	"encoding/json"
	"log"
	"time"

	"github.com/Abraxas-365/claudio/internal/bus"
	"github.com/Abraxas-365/claudio/internal/tui/sidebar"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
	lua "github.com/yuin/gopher-lua"
)

// StatuslineCtx holds contextual data passed to the Lua statusline function.
type StatuslineCtx struct {
	Mode    string
	Model   string
	Tokens  int
	Session string
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

// PendingPaletteEntries returns all palette entries registered by plugins.
func (r *Runtime) PendingPaletteEntries() []PaletteEntry {
	r.uiMu.RLock()
	defer r.uiMu.RUnlock()
	out := make([]PaletteEntry, len(r.pendingPaletteEntries))
	copy(out, r.pendingPaletteEntries)
	return out
}

// injectUIAPI registers claudio.ui.* bindings and returns the table.
func (r *Runtime) injectUIAPI(L *lua.LState, plugin *loadedPlugin) *lua.LTable {
	ui := L.NewTable()
	L.SetField(ui, "set_statusline", L.NewFunction(r.apiSetStatusline(plugin)))
	L.SetField(ui, "popup", L.NewFunction(r.apiPopup(plugin)))
	L.SetField(ui, "register_palette_entry", L.NewFunction(r.apiRegisterPaletteEntry()))
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

// injectPluginUIAPI is a no-op retained for call-site compatibility.
// Sidebar blocks have been replaced by claudio.win.new_panel.
func (r *Runtime) injectPluginUIAPI(_ *lua.LState, _ *loadedPlugin, _ *lua.LTable) {}

// SetBlockRegistry is a deprecated no-op. Retained so root.go compiles until
// the TUI agent replaces the call with SetPanelRegistry.
// Deprecated: use SetPanelRegistry instead.
func (r *Runtime) SetBlockRegistry(_ *sidebar.BlockRegistry) {}

// GetSidebarBlocks is a deprecated no-op. Returns nil.
// Deprecated: panels now accessed via GetPanelRegistry().
func (r *Runtime) GetSidebarBlocks() []struct{} { return nil }
