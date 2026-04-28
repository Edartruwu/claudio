// Package lua — claudio.win.new_panel API implementation.
package lua

import (
	lua "github.com/yuin/gopher-lua"
)

const luaPanelTypeName = "Panel"

// injectWinAPI adds the claudio.win sub-table with new_panel.
func (r *Runtime) injectWinAPI(L *lua.LState, plugin *loadedPlugin, claudio *lua.LTable) {
	// Register Panel userdata metatable (once per LState).
	mt := L.NewTypeMetatable(luaPanelTypeName)
	index := L.NewTable()
	L.SetField(index, "add_section", L.NewFunction(r.panelAddSection(plugin)))
	L.SetField(index, "remove_section", L.NewFunction(panelRemoveSection))
	L.SetField(index, "show", L.NewFunction(panelShow))
	L.SetField(index, "hide", L.NewFunction(panelHide))
	L.SetField(index, "toggle", L.NewFunction(panelToggle))
	L.SetField(mt, "__index", index)

	winTable := L.NewTable()
	L.SetField(winTable, "new_panel", L.NewFunction(r.apiNewPanel(plugin)))
	L.SetField(claudio, "win", winTable)
}

// apiNewPanel implements claudio.win.new_panel(opts) → Panel userdata.
func (r *Runtime) apiNewPanel(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		opts := L.CheckTable(1)

		position := "left"
		if v, ok := L.GetField(opts, "position").(lua.LString); ok {
			position = string(v)
		}
		switch position {
		case "left", "right", "bottom":
		default:
			L.RaiseError("new_panel: position must be 'left', 'right', or 'bottom'")
			return 0
		}

		width := 30
		if v, ok := L.GetField(opts, "width").(lua.LNumber); ok && int(v) > 0 {
			width = int(v)
		}
		height := 10
		if v, ok := L.GetField(opts, "height").(lua.LNumber); ok && int(v) > 0 {
			height = int(v)
		}

		panel := &PanelDef{
			Position: position,
			Width:    width,
			Height:   height,
			Visible:  true,
		}

		// If registry already wired → register now; else queue pending.
		r.panelRegistryMu.RLock()
		reg := r.panelRegistry
		r.panelRegistryMu.RUnlock()

		if reg != nil {
			reg.Register(panel)
		} else {
			r.pendingPanelsMu.Lock()
			r.pendingPanels = append(r.pendingPanels, panel)
			r.pendingPanelsMu.Unlock()
		}

		ud := L.NewUserData()
		ud.Value = panel
		L.SetMetatable(ud, L.GetTypeMetatable(luaPanelTypeName))
		L.Push(ud)
		return 1
	}
}

// checkPanel extracts PanelDef from first arg (self).
func checkPanel(L *lua.LState) *PanelDef {
	ud := L.CheckUserData(1)
	if p, ok := ud.Value.(*PanelDef); ok {
		return p
	}
	L.ArgError(1, "Panel expected")
	return nil
}

// panelAddSection implements panel:add_section({id, title, weight, min_height, render}).
func (r *Runtime) panelAddSection(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		p := checkPanel(L)
		if p == nil {
			return 0
		}
		opts := L.CheckTable(2)

		id := ""
		if v, ok := L.GetField(opts, "id").(lua.LString); ok {
			id = string(v)
		}
		if id == "" {
			L.RaiseError("add_section: id must be a non-empty string")
			return 0
		}

		title := ""
		if v, ok := L.GetField(opts, "title").(lua.LString); ok {
			title = string(v)
		}

		weight := 1
		if v, ok := L.GetField(opts, "weight").(lua.LNumber); ok {
			weight = int(v)
		}

		minHeight := 1
		if v, ok := L.GetField(opts, "min_height").(lua.LNumber); ok {
			minHeight = int(v)
		}

		renderFn, ok := L.GetField(opts, "render").(*lua.LFunction)
		if !ok {
			L.RaiseError("add_section: render must be a function")
			return 0
		}

		sec := &SectionDef{
			ID:        id,
			Title:     title,
			Weight:    weight,
			MinHeight: minHeight,
			plugin:    plugin,
			renderFn:  renderFn,
		}

		p.Mu.Lock()
		p.AddSection(sec)
		p.Mu.Unlock()

		return 0
	}
}

// panelRemoveSection implements panel:remove_section(id).
func panelRemoveSection(L *lua.LState) int {
	p := checkPanel(L)
	if p == nil {
		return 0
	}
	id := L.CheckString(2)
	p.Mu.Lock()
	p.RemoveSection(id)
	p.Mu.Unlock()
	return 0
}

// panelShow implements panel:show().
func panelShow(L *lua.LState) int {
	p := checkPanel(L)
	if p == nil {
		return 0
	}
	p.Mu.Lock()
	p.Visible = true
	p.Mu.Unlock()
	return 0
}

// panelHide implements panel:hide().
func panelHide(L *lua.LState) int {
	p := checkPanel(L)
	if p == nil {
		return 0
	}
	p.Mu.Lock()
	p.Visible = false
	p.Mu.Unlock()
	return 0
}

// panelToggle implements panel:toggle().
func panelToggle(L *lua.LState) int {
	p := checkPanel(L)
	if p == nil {
		return 0
	}
	p.Mu.Lock()
	p.Visible = !p.Visible
	p.Mu.Unlock()
	return 0
}
