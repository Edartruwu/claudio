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
	"github.com/Abraxas-365/claudio/internal/tui/vim"
	lua "github.com/yuin/gopher-lua"
)

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
