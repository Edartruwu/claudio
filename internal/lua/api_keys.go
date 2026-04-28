package lua

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/Abraxas-365/claudio/internal/bus"
	"github.com/Abraxas-365/claudio/internal/tui/keymap"
	"github.com/Abraxas-365/claudio/internal/tui/vim"
	lua "github.com/yuin/gopher-lua"
)

// luaActionNames maps the Lua-friendly dot-notation action names used in
// claudio.keymap.map() to the actual ActionID constants. The ActionID
// constants themselves use dashes in some places; this table normalises
// the discrepancy so callers can use consistent dot-separated names.
var luaActionNames = map[string]keymap.ActionID{
	"window.cycle":        keymap.ActionWindowCycle,
	"window.close":        keymap.ActionWindowClose,
	"window.focus.up":     keymap.ActionWindowFocusUp,
	"window.focus.down":   keymap.ActionWindowFocusDown,
	"window.focus.left":   keymap.ActionWindowFocusLeft,
	"window.focus.right":  keymap.ActionWindowFocusRight,
	"window.split.v":      keymap.ActionWindowSplitVertical,
	"window.float.close":  keymap.ActionFloatWindowClose,
	"window.float.hint":   keymap.ActionFloatWindowHint,
	"buffer.next":         keymap.ActionBufferNext,
	"buffer.prev":         keymap.ActionBufferPrev,
	"buffer.new":          keymap.ActionBufferNew,
	"buffer.close":        keymap.ActionBufferClose,
	"buffer.rename":       keymap.ActionBufferRename,
	"buffer.alternate":    keymap.ActionBufferAlternate,
	"buffer.list":         keymap.ActionBufferList,
	"panel.skills":        keymap.ActionPanelSkills,
	"panel.memory":        keymap.ActionPanelMemory,
	"panel.tasks":         keymap.ActionPanelTasks,
	"panel.tools":         keymap.ActionPanelTools,
	"panel.analytics":     keymap.ActionPanelAnalytics,
	"panel.files":         keymap.ActionPanelFiles,
	"panel.session_tree":  keymap.ActionPanelSessionTree,
	"panel.agent_gui":     keymap.ActionPanelAgentGUI,
	"picker.session":      keymap.ActionSessionPicker,
	"picker.recent":       keymap.ActionSessionRecent,
	"picker.search":       keymap.ActionSearch,
	"picker.commands":     keymap.ActionCommandPalette,
	"picker.buffers":      keymap.ActionPickerBuffers,
	"picker.agents":       keymap.ActionPickerAgents,
	"editor.edit":         keymap.ActionEditorEditPrompt,
	"editor.view":         keymap.ActionEditorViewSection,
	"todo.toggle":         keymap.ActionTodoDock,
}

// normalizeLeaderSeq strips the "<space>" prefix from a leader sequence string
// returning just the key suffix stored in the keymap.
// e.g. "<space>ww" → "ww", "<space>K" → "K".
func normalizeLeaderSeq(seq string) string {
	lower := strings.ToLower(seq)
	for _, pfx := range []string{"<space>", "<leader>"} {
		if strings.HasPrefix(lower, pfx) {
			return seq[len(pfx):]
		}
	}
	return seq
}

// apiRegisterKeymap returns the claudio.register_keymap(tbl) binding.
//
// Lua usage:
//
//	claudio.register_keymap({
//	  key         = "ctrl+p",   -- key string (single char or named key)
//	  mode        = "normal",   -- "normal", "insert", "visual"
//	  description = "my action",
//	  handler     = function(event) ... end,  -- called when key fires (side-effects only)
//	})
//
// The handler function receives a table: { key, mode, plugin }.
// It does NOT return an action — side effects happen via claudio.notify/publish.
// When the key fires the Go side publishes a "keymap.action" bus event; the handler
// is called asynchronously via a matching bus subscription.
func (r *Runtime) apiRegisterKeymap(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		tbl := L.CheckTable(1)

		keyStr := tableString(L, tbl, "key")
		modeStr := tableString(L, tbl, "mode")
		desc := tableString(L, tbl, "description")
		handlerVal := L.GetField(tbl, "handler")

		if keyStr == "" {
			L.ArgError(1, "register_keymap: key required")
			return 0
		}
		if modeStr == "" {
			L.ArgError(1, "register_keymap: mode required")
			return 0
		}

		key, ok := parseKeyRune(keyStr)
		if !ok {
			L.ArgError(1, "register_keymap: invalid key: "+keyStr)
			return 0
		}
		mode, ok := parseVimMode(modeStr)
		if !ok {
			L.ArgError(1, "register_keymap: invalid mode: "+modeStr)
			return 0
		}

		// Each keymap gets a unique bus event so only its subscriber fires.
		eventType := fmt.Sprintf("keymap.action.%s.%s.%s", plugin.name, modeStr, keyStr)

		// Register the Go-side keymap that publishes the event when the key fires.
		vim.RegisterKeymap(vim.Keymap{
			Key:         key,
			Mode:        mode,
			Description: desc,
			Handler: func(k rune, text string, cursor int, count int, s *vim.State) vim.Action {
				payload, _ := json.Marshal(map[string]any{
					"key":    keyStr,
					"mode":   modeStr,
					"plugin": plugin.name,
					"count":  count,
				})
				r.bus.Publish(bus.Event{
					Type:      eventType,
					Payload:   payload,
					Timestamp: time.Now(),
				})
				return vim.Action{Type: vim.ActionNone}
			},
		})

		// Wire the Lua handler function to the bus event (if provided).
		if fn, ok := handlerVal.(*lua.LFunction); ok {
			unsub := r.bus.Subscribe(eventType, func(e bus.Event) {
				plugin.mu.Lock()
				defer plugin.mu.Unlock()

				defer func() {
					if rv := recover(); rv != nil {
						log.Printf("[lua] plugin %s keymap handler panic: %v", plugin.name, rv)
					}
				}()

				evtTbl := plugin.L.NewTable()
				plugin.L.SetField(evtTbl, "key", lua.LString(keyStr))
				plugin.L.SetField(evtTbl, "mode", lua.LString(modeStr))
				plugin.L.SetField(evtTbl, "plugin", lua.LString(plugin.name))

				if err := plugin.L.CallByParam(lua.P{
					Fn:      fn,
					NRet:    0,
					Protect: true,
				}, evtTbl); err != nil {
					log.Printf("[lua] plugin %s keymap handler error: %v", plugin.name, err)
				}
			})

			plugin.mu.Lock()
			plugin.unsubs = append(plugin.unsubs, unsub)
			plugin.mu.Unlock()
		}

		return 0
	}
}

// apiKeymapDel returns the claudio.keymap.del(mode, key) binding.
//
// Lua usage:
//
//	claudio.keymap.del("normal", "j")
//
// Removes the keymap from the runtime's keymapRegistry (if set) and from
// the package-level default registry.
func (r *Runtime) apiKeymapDel(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		modeStr := L.CheckString(1)
		keyStr := L.CheckString(2)

		mode, ok := parseVimMode(modeStr)
		if !ok {
			L.ArgError(1, "keymap.del: invalid mode: "+modeStr)
			return 0
		}
		key, ok := parseKeyRune(keyStr)
		if !ok {
			L.ArgError(2, "keymap.del: invalid key: "+keyStr)
			return 0
		}

		// Remove from instance registry if wired.
		r.mu.Lock()
		reg := r.keymapRegistry
		r.mu.Unlock()

		if reg != nil {
			reg.Delete(mode, key)
		}
		// Also remove from package-level default registry.
		vim.DefaultRegistry().Delete(mode, key)

		return 0
	}
}

// apiKeymapList returns the claudio.keymap.list(mode) binding.
//
// Lua usage:
//
//	local maps = claudio.keymap.list("normal")
//	for _, m in ipairs(maps) do
//	  print(m.key .. " -> " .. m.action)
//	end
//
// Returns a Lua array of tables: { key, action, description }.
// "action" is the description string (no function refs cross the boundary).
// Unknown mode → empty table.
func (r *Runtime) apiKeymapList(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		modeStr := L.CheckString(1)

		mode, ok := parseVimMode(modeStr)
		if !ok {
			// Unknown mode → return empty table.
			L.Push(L.NewTable())
			return 1
		}

		r.mu.Lock()
		reg := r.keymapRegistry
		r.mu.Unlock()

		var keymaps []vim.Keymap
		if reg != nil {
			keymaps = reg.AllForMode(mode)
		} else {
			keymaps = vim.DefaultRegistry().AllForMode(mode)
		}

		result := L.NewTable()
		for i, km := range keymaps {
			entry := L.NewTable()
			L.SetField(entry, "key", lua.LString(string(km.Key)))
			L.SetField(entry, "action", lua.LString(km.Description))
			L.SetField(entry, "description", lua.LString(km.Description))
			result.RawSetInt(i+1, entry)
		}
		L.Push(result)
		return 1
	}
}

// apiLeaderKeymapMap returns the claudio.keymap.map(seq, action, opts?) binding.
//
// Lua usage:
//
//	claudio.keymap.map("<space>ww", "window.cycle")
//	claudio.keymap.map("<space>m",  function(evt) ... end, { desc = "My action" })
//
// seq is a leader sequence like "<space>ww". action is either an ActionID
// string (looked up in luaActionNames) or a Lua function. For Lua functions a
// synthetic ActionID "lua.fn.<seq>" is registered in the leader keymap and a
// bus subscription fires the function when the sequence is triggered.
func (r *Runtime) apiLeaderKeymapMap(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		seqRaw := L.CheckString(1)
		actionVal := L.Get(2)

		seq := normalizeLeaderSeq(seqRaw)
		if seq == "" {
			L.ArgError(1, "keymap.map: seq must not be empty after stripping leader prefix")
			return 0
		}

		switch av := actionVal.(type) {
		case lua.LString:
			// ActionID string path.
			actionStr := string(av)
			actionID, ok := luaActionNames[actionStr]
			if !ok {
				// Allow raw ActionID strings (in case caller uses the constant directly).
				actionID = keymap.ActionID(actionStr)
				if !keymap.ValidAction(actionID) {
					L.ArgError(2, "keymap.map: unknown action: "+actionStr)
					return 0
				}
			}
			r.setLeaderBinding(seq, actionID)

		case *lua.LFunction:
			// Lua function path: assign synthetic ActionID, subscribe bus event.
			syntheticID := keymap.ActionID("lua.fn." + seq)
			eventType := "leader." + string(syntheticID)

			r.setLeaderBinding(seq, syntheticID)

			// Subscribe the Lua function to the bus event that dispatchAction fires.
			unsub := r.bus.Subscribe(eventType, func(e bus.Event) {
				plugin.mu.Lock()
				defer plugin.mu.Unlock()

				defer func() {
					if rv := recover(); rv != nil {
						log.Printf("[lua] plugin %s leader handler panic: %v", plugin.name, rv)
					}
				}()

				evtTbl := plugin.L.NewTable()
				plugin.L.SetField(evtTbl, "seq", lua.LString(seqRaw))
				plugin.L.SetField(evtTbl, "plugin", lua.LString(plugin.name))

				if err := plugin.L.CallByParam(lua.P{
					Fn:      av,
					NRet:    0,
					Protect: true,
				}, evtTbl); err != nil {
					log.Printf("[lua] plugin %s leader handler error: %v", plugin.name, err)
				}
			})

			plugin.mu.Lock()
			plugin.unsubs = append(plugin.unsubs, unsub)
			plugin.mu.Unlock()

		default:
			L.ArgError(2, "keymap.map: action must be a string or function")
			return 0
		}

		return 0
	}
}

// apiLeaderKeymapUnmap returns the claudio.keymap.unmap(seq) binding.
//
// Lua usage:
//
//	claudio.keymap.unmap("<space>ww")
//
// Removes the leader binding for seq. Safe to call before the keymap is wired.
func (r *Runtime) apiLeaderKeymapUnmap(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		seqRaw := L.CheckString(1)
		seq := normalizeLeaderSeq(seqRaw)
		if seq == "" {
			return 0
		}

		r.leaderKeymapMu.Lock()
		km := r.leaderKeymap
		r.leaderKeymapMu.Unlock()

		if km != nil {
			km.Delete(seq)
		}
		// Also remove from pending list so it won't be applied on SetLeaderKeymap.
		r.pendingLeaderMu.Lock()
		filtered := r.pendingLeaderBindings[:0]
		for _, pb := range r.pendingLeaderBindings {
			if pb.seq != seq {
				filtered = append(filtered, pb)
			}
		}
		r.pendingLeaderBindings = filtered
		r.pendingLeaderMu.Unlock()

		return 0
	}
}

// setLeaderBinding applies a leader binding to the keymap if it is already
// wired, otherwise buffers it for when SetLeaderKeymap is called.
func (r *Runtime) setLeaderBinding(seq string, actionID keymap.ActionID) {
	r.leaderKeymapMu.Lock()
	km := r.leaderKeymap
	r.leaderKeymapMu.Unlock()

	if km != nil {
		// Only apply if the user has not already set this binding via keymap.json.
		if _, already := km.Resolve(seq); !already {
			km.SetCustom(seq, actionID)
		}
		return
	}

	// Buffer for later application via SetLeaderKeymap.
	r.pendingLeaderMu.Lock()
	r.pendingLeaderBindings = append(r.pendingLeaderBindings, pendingLeaderBinding{
		seq:      seq,
		actionID: actionID,
	})
	r.pendingLeaderMu.Unlock()
}

// LoadKeybindings loads a keybindings.lua file (e.g. ~/.claudio/keybindings.lua) in a
// sandboxed Lua VM. The file may call claudio.register_keymap(), claudio.log(), and
// claudio.notify(). Missing files are silently skipped.
func (r *Runtime) LoadKeybindings(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	L := newSandboxedState()
	plugin := &loadedPlugin{
		name: "keybindings",
		dir:  filepath.Dir(path),
		L:    L,
	}

	claudio := L.NewTable()
	L.SetField(claudio, "register_keymap", L.NewFunction(r.apiRegisterKeymap(plugin)))
	L.SetField(claudio, "log", L.NewFunction(r.apiLog(plugin)))
	L.SetField(claudio, "notify", L.NewFunction(r.apiNotify(plugin)))
	L.SetGlobal("claudio", claudio)

	if err := L.DoFile(path); err != nil {
		L.Close()
		return fmt.Errorf("keybindings.lua: %w", err)
	}

	r.mu.Lock()
	r.plugins = append(r.plugins, plugin)
	r.mu.Unlock()

	log.Printf("[lua] loaded keybindings: %s", path)
	return nil
}

// parseVimMode converts a Lua mode string to a vim.Mode.
func parseVimMode(s string) (vim.Mode, bool) {
	switch strings.ToLower(s) {
	case "normal":
		return vim.ModeNormal, true
	case "insert":
		return vim.ModeInsert, true
	case "visual":
		return vim.ModeVisual, true
	case "operator", "operator_pending", "op_pending", "op-pending":
		return vim.ModeOperatorPending, true
	default:
		return 0, false
	}
}

// parseKeyRune converts a key string to a rune.
// Supports: single character ("i", "a"), named keys ("escape", "esc"),
// and Ctrl shortcuts ("ctrl+r", "ctrl-r", "<c-r>").
func parseKeyRune(s string) (rune, bool) {
	lower := strings.ToLower(strings.TrimSpace(s))

	// Named special keys
	switch lower {
	case "escape", "esc":
		return 27, true
	case "enter", "return", "cr":
		return 13, true
	case "tab":
		return 9, true
	case "backspace", "bs":
		return 127, true
	case "space":
		return ' ', true
	}

	// Ctrl+letter: "ctrl+r", "ctrl-r", "<c-r>"
	if after, ok := cutCtrlPrefix(lower); ok {
		r := []rune(after)
		if len(r) == 1 && r[0] >= 'a' && r[0] <= 'z' {
			return r[0] - 'a' + 1, true
		}
		return 0, false
	}

	// Single printable character
	r := []rune(s)
	if len(r) == 1 && !unicode.IsControl(r[0]) {
		return r[0], true
	}

	return 0, false
}

// cutCtrlPrefix strips a ctrl modifier prefix and returns the remainder.
func cutCtrlPrefix(s string) (string, bool) {
	for _, prefix := range []string{"ctrl+", "ctrl-", "<c-"} {
		if strings.HasPrefix(s, prefix) {
			rest := strings.TrimPrefix(s, prefix)
			rest = strings.TrimSuffix(rest, ">") // strip trailing > for <c-x> form
			return rest, true
		}
	}
	return "", false
}
