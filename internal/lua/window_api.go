// Package lua: window/buffer APIs exposed as claudio.buf.* and claudio.ui.register_window.
package lua

import (
	"log"

	"github.com/Abraxas-365/claudio/internal/tui/windows"
	lua "github.com/yuin/gopher-lua"
)

// WindowDef queues a window registration before WindowManager is wired.
type WindowDef struct {
	Window *windows.Window
}

// injectWindowsAPI adds claudio.buf sub-table and extends claudio.ui with register_window.
func (r *Runtime) injectWindowsAPI(L *lua.LState, plugin *loadedPlugin, claudio *lua.LTable) {
	// claudio.buf sub-table
	bufTable := L.NewTable()
	L.SetField(bufTable, "new", L.NewFunction(r.apiBufNew(plugin)))
	L.SetField(claudio, "buf", bufTable)

	// Extend claudio.ui with register_window (ui table created by injectUIAPI already)
	uiVal := L.GetField(claudio, "ui")
	uiTable, ok := uiVal.(*lua.LTable)
	if !ok || uiTable == nil {
		uiTable = L.NewTable()
		L.SetField(claudio, "ui", uiTable)
	}
	L.SetField(uiTable, "register_window", L.NewFunction(r.apiRegisterWindow()))
}

// apiBufNew implements claudio.buf.new({name, render}) -> LUserData(*windows.Buffer).
func (r *Runtime) apiBufNew(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		opts := L.CheckTable(1)

		nameVal := L.GetField(opts, "name")
		name, ok := nameVal.(lua.LString)
		if !ok {
			L.RaiseError("claudio.buf.new: 'name' must be a string")
			return 0
		}

		renderVal := L.GetField(opts, "render")
		renderFn, isFn := renderVal.(*lua.LFunction)
		if !isFn {
			L.RaiseError("claudio.buf.new: 'render' must be a function")
			return 0
		}

		bufName := string(name)
		buf := &windows.Buffer{
			Name: bufName,
			Render: func(width, height int) string {
				plugin.mu.Lock()
				defer plugin.mu.Unlock()
				defer func() {
					if rv := recover(); rv != nil {
						log.Printf("[lua] buf %q render panic: %v", bufName, rv)
					}
				}()
				if err := plugin.L.CallByParam(lua.P{
					Fn:      renderFn,
					NRet:    1,
					Protect: true,
				}, lua.LNumber(width), lua.LNumber(height)); err != nil {
					log.Printf("[lua] buf %q render error: %v", bufName, err)
					return ""
				}
				result := plugin.L.Get(-1)
				plugin.L.Pop(1)
				if s, ok2 := result.(lua.LString); ok2 {
					return string(s)
				}
				return ""
			},
		}

		ud := L.NewUserData()
		ud.Value = buf
		L.Push(ud)
		return 1
	}
}

// layoutFromString maps a Lua layout string to windows.Layout.
func layoutFromString(s string) windows.Layout {
	switch s {
	case "float":
		return windows.LayoutFloat
	case "sidebar":
		return windows.LayoutSidebar
	case "splith":
		return windows.LayoutSplitH
	case "splitv":
		return windows.LayoutSplitV
	default:
		return windows.LayoutFloat
	}
}

// apiRegisterWindow implements claudio.ui.register_window({name, buffer, layout, title}).
func (r *Runtime) apiRegisterWindow() lua.LGFunction {
	return func(L *lua.LState) int {
		opts := L.CheckTable(1)

		nameVal := L.GetField(opts, "name")
		name, ok := nameVal.(lua.LString)
		if !ok {
			L.RaiseError("claudio.ui.register_window: 'name' must be a string")
			return 0
		}

		bufVal := L.GetField(opts, "buffer")
		ud, isUD := bufVal.(*lua.LUserData)
		if !isUD {
			L.RaiseError("claudio.ui.register_window: 'buffer' must be a buf userdata")
			return 0
		}
		buf, isBuf := ud.Value.(*windows.Buffer)
		if !isBuf {
			L.RaiseError("claudio.ui.register_window: 'buffer' is not a valid Buffer")
			return 0
		}

		layoutStr := "float"
		if lv, ok2 := L.GetField(opts, "layout").(lua.LString); ok2 {
			layoutStr = string(lv)
		}
		layout := layoutFromString(layoutStr)

		title := string(name)
		if tv, ok2 := L.GetField(opts, "title").(lua.LString); ok2 {
			title = string(tv)
		}

		w := &windows.Window{
			Name:   string(name),
			Title:  title,
			Buffer: buf,
			Layout: layout,
		}

		r.windowManagerMu.Lock()
		wm := r.windowManager
		r.windowManagerMu.Unlock()

		if wm != nil {
			wm.Register(w)
		} else {
			r.pendingWindowsMu.Lock()
			r.pendingWindows = append(r.pendingWindows, WindowDef{Window: w})
			r.pendingWindowsMu.Unlock()
		}
		return 0
	}
}
