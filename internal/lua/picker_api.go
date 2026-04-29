// Package lua: picker and finder APIs exposed as claudio.picker.* and claudio.finder.*.
package lua

import (
	"context"
	"fmt"
	"log"

	"github.com/Abraxas-365/claudio/internal/tui/picker"
	"github.com/Abraxas-365/claudio/internal/tui/picker/finders"
	lua "github.com/yuin/gopher-lua"
)

// finderMeta is the Lua metatable name for picker.Finder userdata.
const finderMeta = "claudio.Finder"

// injectPickerAPI adds claudio.picker and claudio.finder sub-tables.
func (r *Runtime) injectPickerAPI(L *lua.LState, plugin *loadedPlugin, claudio *lua.LTable) {
	// Register finder metatable (idempotent — gopher-lua NewTypeMetatable is safe).
	mt := L.NewTypeMetatable(finderMeta)
	L.SetField(mt, "__index", mt)

	// claudio.finder sub-table
	finderTable := L.NewTable()
	L.SetField(finderTable, "from_table", L.NewFunction(r.apiFinderFromTable(plugin)))
	L.SetField(finderTable, "from_fn", L.NewFunction(r.apiFinderFromFn(plugin)))
	L.SetField(claudio, "finder", finderTable)

	// claudio.picker sub-table
	pickerTable := L.NewTable()
	L.SetField(pickerTable, "buffers", L.NewFunction(r.apiPickerBuffers(plugin)))
	L.SetField(pickerTable, "agents", L.NewFunction(r.apiPickerAgents(plugin)))
	L.SetField(pickerTable, "commands", L.NewFunction(r.apiPickerCommands(plugin)))
	L.SetField(pickerTable, "skills", L.NewFunction(r.apiPickerSkills(plugin)))
	L.SetField(pickerTable, "open", L.NewFunction(r.apiPickerOpen(plugin)))
	L.SetField(claudio, "picker", pickerTable)
}

// ── claudio.finder.from_table ─────────────────────────────────────────────────

// apiFinderFromTable implements claudio.finder.from_table(list) -> Finder userdata.
// Each list element may be a string or a table {value, display?, ordinal?}.
func (r *Runtime) apiFinderFromTable(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		tbl := L.CheckTable(1)
		var entries []picker.Entry
		tbl.ForEach(func(_, v lua.LValue) {
			switch sv := v.(type) {
			case lua.LString:
				s := string(sv)
				entries = append(entries, picker.Entry{
					Value:   s,
					Display: s,
					Ordinal: s,
				})
			case *lua.LTable:
				val := luaToString(L.GetField(sv, "value"))
				display := luaToString(L.GetField(sv, "display"))
				ordinal := luaToString(L.GetField(sv, "ordinal"))
				if display == "" {
					display = val
				}
				if ordinal == "" {
					ordinal = val
				}
				entries = append(entries, picker.Entry{
					Value:   val,
					Display: display,
					Ordinal: ordinal,
				})
			}
		})
		f := finders.NewTableFinder(entries)
		ud := L.NewUserData()
		ud.Value = picker.Finder(f)
		L.SetMetatable(ud, L.GetTypeMetatable(finderMeta))
		L.Push(ud)
		return 1
	}
}

// ── claudio.finder.from_fn ────────────────────────────────────────────────────

// luaFnFinder is a picker.Finder that calls a Lua function to produce entries.
// The function is invoked once per Find() call; it must return a Lua table.
type luaFnFinder struct {
	plugin *loadedPlugin
	fn     *lua.LFunction
}

func (f *luaFnFinder) Find(ctx context.Context) <-chan picker.Entry {
	f.plugin.mu.Lock()
	var entries []picker.Entry
	if err := f.plugin.L.CallByParam(lua.P{
		Fn:      f.fn,
		NRet:    1,
		Protect: true,
	}); err != nil {
		log.Printf("[lua] finder.from_fn call error: %v", err)
	} else {
		result := f.plugin.L.Get(-1)
		f.plugin.L.Pop(1)
		if tbl, ok := result.(*lua.LTable); ok {
			tbl.ForEach(func(_, v lua.LValue) {
				switch sv := v.(type) {
				case lua.LString:
					s := string(sv)
					entries = append(entries, picker.Entry{Value: s, Display: s, Ordinal: s})
				case *lua.LTable:
					val := luaToString(f.plugin.L.GetField(sv, "value"))
					display := luaToString(f.plugin.L.GetField(sv, "display"))
					ordinal := luaToString(f.plugin.L.GetField(sv, "ordinal"))
					if display == "" {
						display = val
					}
					if ordinal == "" {
						ordinal = val
					}
					entries = append(entries, picker.Entry{Value: val, Display: display, Ordinal: ordinal})
				}
			})
		}
	}
	f.plugin.mu.Unlock()

	ch := make(chan picker.Entry, len(entries))
	go func() {
		defer close(ch)
		for _, e := range entries {
			select {
			case <-ctx.Done():
				return
			case ch <- e:
			}
		}
	}()
	return ch
}

func (f *luaFnFinder) Close() {}

// apiFinderFromFn implements claudio.finder.from_fn(fn) -> Finder userdata.
func (r *Runtime) apiFinderFromFn(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		fn := L.CheckFunction(1)
		f := &luaFnFinder{plugin: plugin, fn: fn}
		ud := L.NewUserData()
		ud.Value = picker.Finder(f)
		L.SetMetatable(ud, L.GetTypeMetatable(finderMeta))
		L.Push(ud)
		return 1
	}
}

// ── opener helper ─────────────────────────────────────────────────────────────

// callPickerOpener reads the stored opener and invokes it; logs if not wired.
func (r *Runtime) callPickerOpener(cfg picker.Config) {
	r.pickerOpenerMu.RLock()
	opener := r.pickerOpener
	r.pickerOpenerMu.RUnlock()
	if opener == nil {
		log.Printf("[lua] picker: no opener wired (TUI not ready)")
		return
	}
	opener(cfg)
}

// ── built-in convenience pickers ─────────────────────────────────────────────

// apiPickerBuffers implements claudio.picker.buffers().
func (r *Runtime) apiPickerBuffers(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		r.windowManagerMu.RLock()
		wm := r.windowManager
		r.windowManagerMu.RUnlock()
		if wm == nil {
			L.RaiseError("claudio.picker.buffers: window manager not available")
			return 0
		}
		r.callPickerOpener(picker.Config{
			Title:  "Buffers",
			Finder: finders.NewBufferFinder(wm),
		})
		return 0
	}
}

// apiPickerAgents implements claudio.picker.agents().
func (r *Runtime) apiPickerAgents(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		r.teamRunnerMu.RLock()
		runner := r.teamRunner
		r.teamRunnerMu.RUnlock()
		if runner == nil {
			L.RaiseError("claudio.picker.agents: team runner not available")
			return 0
		}
		r.callPickerOpener(picker.Config{
			Title:  "Agents",
			Finder: finders.NewAgentFinder(runner),
		})
		return 0
	}
}

// apiPickerCommands implements claudio.picker.commands().
func (r *Runtime) apiPickerCommands(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		r.commandRegistryMu.Lock()
		reg := r.commandRegistry
		r.commandRegistryMu.Unlock()
		if reg == nil {
			L.RaiseError("claudio.picker.commands: command registry not available")
			return 0
		}
		r.callPickerOpener(picker.Config{
			Title:  "Commands",
			Finder: finders.NewCommandFinder(reg),
		})
		return 0
	}
}

// apiPickerSkills implements claudio.picker.skills().
func (r *Runtime) apiPickerSkills(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		if r.skills == nil {
			L.RaiseError("claudio.picker.skills: skills registry not available")
			return 0
		}
		r.callPickerOpener(picker.Config{
			Title:  "Skills",
			Finder: finders.NewSkillFinder(r.skills),
		})
		return 0
	}
}

// ── claudio.picker.open ───────────────────────────────────────────────────────

// apiPickerOpen implements claudio.picker.open({title, finder, previewer?, layout?, on_select?}).
func (r *Runtime) apiPickerOpen(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		opts := L.CheckTable(1)

		// title (required)
		titleVal := L.GetField(opts, "title")
		title, ok := titleVal.(lua.LString)
		if !ok || string(title) == "" {
			L.RaiseError("claudio.picker.open: 'title' must be a non-empty string")
			return 0
		}

		// finder (required)
		finderVal := L.GetField(opts, "finder")
		ud, isUD := finderVal.(*lua.LUserData)
		if !isUD {
			L.RaiseError("claudio.picker.open: 'finder' must be a finder userdata")
			return 0
		}
		f, isFinder := ud.Value.(picker.Finder)
		if !isFinder {
			L.RaiseError("claudio.picker.open: 'finder' is not a valid Finder")
			return 0
		}

		cfg := picker.Config{
			Title:  string(title),
			Finder: f,
		}

		// layout (optional string)
		if lv, ok2 := L.GetField(opts, "layout").(lua.LString); ok2 {
			cfg.Layout = picker.LayoutStrategy(string(lv))
		}

		// previewer (optional — false disables, default nil = no previewer set)
		prevVal := L.GetField(opts, "previewer")
		if b, ok2 := prevVal.(lua.LBool); ok2 && !bool(b) {
			cfg.Previewer = nil // explicitly disabled
		}

		// on_select (optional Lua callback)
		onSelVal := L.GetField(opts, "on_select")
		if onSelFn, ok2 := onSelVal.(*lua.LFunction); ok2 {
			capturedFn := onSelFn
			capturedPlugin := plugin
			cfg.OnSelect = func(entry picker.Entry) {
				capturedPlugin.mu.Lock()
				defer capturedPlugin.mu.Unlock()
				entryTbl := capturedPlugin.L.NewTable()
				capturedPlugin.L.SetField(entryTbl, "display", lua.LString(entry.Display))
				capturedPlugin.L.SetField(entryTbl, "ordinal", lua.LString(entry.Ordinal))
				if s, ok3 := entry.Value.(string); ok3 {
					capturedPlugin.L.SetField(entryTbl, "value", lua.LString(s))
				} else {
					capturedPlugin.L.SetField(entryTbl, "value", lua.LString(fmt.Sprintf("%v", entry.Value)))
				}
				if err := capturedPlugin.L.CallByParam(lua.P{
					Fn:      capturedFn,
					NRet:    0,
					Protect: true,
				}, entryTbl); err != nil {
					log.Printf("[lua] picker on_select error: %v", err)
				}
			}
		}

		r.callPickerOpener(cfg)
		return 0
	}
}
