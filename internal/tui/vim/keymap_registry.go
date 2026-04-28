package vim

import (
	"strings"
	"sync"
)

// KeyHandler processes a key press in vim mode.
// Parameters: key rune, text string, cursor int, count int (numeric prefix, ≥1), s *State.
// Returns the Action to perform.
type KeyHandler func(key rune, text string, cursor int, count int, s *State) Action

// Keymap represents a single key binding.
type Keymap struct {
	Key         rune
	Mode        Mode
	Description string
	Handler     KeyHandler
}

// KeymapRegistry holds all registered keymaps indexed by mode → key.
// Last registration wins (plugins can override defaults).
type KeymapRegistry struct {
	mu      sync.RWMutex
	keymaps map[Mode]map[rune]Keymap
}

// NewKeymapRegistry creates an empty registry.
func NewKeymapRegistry() *KeymapRegistry {
	return &KeymapRegistry{
		keymaps: make(map[Mode]map[rune]Keymap),
	}
}

// Register adds or replaces a keymap. Last registration wins.
func (r *KeymapRegistry) Register(km Keymap) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.keymaps[km.Mode]; !ok {
		r.keymaps[km.Mode] = make(map[rune]Keymap)
	}
	r.keymaps[km.Mode][km.Key] = km
}

// Lookup finds a keymap for the given key and mode. Returns false if not found.
func (r *KeymapRegistry) Lookup(key rune, mode Mode) (Keymap, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if modeMap, ok := r.keymaps[mode]; ok {
		if km, ok := modeMap[key]; ok {
			return km, true
		}
	}
	return Keymap{}, false
}

// All returns a snapshot of all registered keymaps.
func (r *KeymapRegistry) All() []Keymap {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var all []Keymap
	for _, modeMap := range r.keymaps {
		for _, km := range modeMap {
			all = append(all, km)
		}
	}
	return all
}

// defaultRegistry is the package-level registry used by all State instances
// unless overridden. Populated in init() with DefaultKeymaps.
var defaultRegistry *KeymapRegistry

func init() {
	defaultRegistry = NewKeymapRegistry()
	for _, km := range DefaultKeymaps() {
		defaultRegistry.Register(km)
	}
}

// RegisterKeymap adds a keymap to the package-level default registry.
// Intended for use by Lua plugins at startup — last registration wins.
func RegisterKeymap(km Keymap) {
	defaultRegistry.Register(km)
}

// DefaultKeymaps returns all built-in vim keymaps extracted from the original
// switch-statement handlers. Behavior is identical to the previous hardcoded version.
func DefaultKeymaps() []Keymap {
	var km []Keymap

	// ── Insert Mode ─────────────────────────────────────────────────────────────

	km = append(km, Keymap{
		Key: 27, Mode: ModeInsert, Description: "escape to normal",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			s.Mode = ModeNormal
			return Action{Type: ActionSwitchMode, SwitchMode: ModeNormal, SetCursor: NoPos}
		},
	})

	// ── Normal Mode — mode switches ──────────────────────────────────────────────

	km = append(km, Keymap{
		Key: 'i', Mode: ModeNormal, Description: "insert before cursor",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			s.Mode = ModeInsert
			s.startRecording()
			return Action{Type: ActionSwitchMode, SwitchMode: ModeInsert, SetCursor: NoPos}
		},
	})
	km = append(km, Keymap{
		Key: 'a', Mode: ModeNormal, Description: "insert after cursor",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			s.Mode = ModeInsert
			s.startRecording()
			return Action{Type: ActionSwitchMode, SwitchMode: ModeInsert, MoveCursor: 1, SetCursor: NoPos}
		},
	})
	km = append(km, Keymap{
		Key: 'I', Mode: ModeNormal, Description: "insert at line start",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			s.Mode = ModeInsert
			s.startRecording()
			pos := firstNonBlank(text, cursor)
			return Action{Type: ActionSwitchMode, SwitchMode: ModeInsert, SetCursor: pos}
		},
	})
	km = append(km, Keymap{
		Key: 'A', Mode: ModeNormal, Description: "insert at line end",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			s.Mode = ModeInsert
			s.startRecording()
			return Action{Type: ActionSwitchMode, SwitchMode: ModeInsert, SetCursor: lineEndPos(text, cursor)}
		},
	})
	km = append(km, Keymap{
		Key: 'o', Mode: ModeNormal, Description: "open line below",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			s.Mode = ModeInsert
			s.startRecording()
			end := lineEndPos(text, cursor)
			return Action{Type: ActionInsertText, Text: "\n", SetCursor: end, SwitchMode: ModeInsert}
		},
	})
	km = append(km, Keymap{
		Key: 'O', Mode: ModeNormal, Description: "open line above",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			s.Mode = ModeInsert
			s.startRecording()
			start := lineStartPos(text, cursor)
			return Action{Type: ActionInsertText, Text: "\n", SetCursor: start, SwitchMode: ModeInsert, MoveCursor: -1}
		},
	})
	km = append(km, Keymap{
		Key: 'v', Mode: ModeNormal, Description: "visual mode",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			s.Mode = ModeVisual
			s.VisualStart = cursor
			return Action{Type: ActionSwitchMode, SwitchMode: ModeVisual, SetCursor: NoPos}
		},
	})

	// ── Normal Mode — motions ────────────────────────────────────────────────────

	km = append(km, Keymap{
		Key: 'h', Mode: ModeNormal, Description: "move left",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionMoveCursor, MoveCursor: -count}
		},
	})
	km = append(km, Keymap{
		Key: 'l', Mode: ModeNormal, Description: "move right",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionMoveCursor, MoveCursor: count}
		},
	})
	km = append(km, Keymap{
		Key: 'j', Mode: ModeNormal, Description: "move down",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return moveLine(text, cursor, count)
		},
	})
	km = append(km, Keymap{
		Key: 'k', Mode: ModeNormal, Description: "move up",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return moveLine(text, cursor, -count)
		},
	})
	km = append(km, Keymap{
		Key: 'w', Mode: ModeNormal, Description: "word forward",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionSetCursor, SetCursor: wordForward(text, cursor, count)}
		},
	})
	km = append(km, Keymap{
		Key: 'W', Mode: ModeNormal, Description: "WORD forward",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionSetCursor, SetCursor: wordForwardBig(text, cursor, count)}
		},
	})
	km = append(km, Keymap{
		Key: 'b', Mode: ModeNormal, Description: "word backward",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionSetCursor, SetCursor: wordBackward(text, cursor, count)}
		},
	})
	km = append(km, Keymap{
		Key: 'B', Mode: ModeNormal, Description: "WORD backward",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionSetCursor, SetCursor: wordBackwardBig(text, cursor, count)}
		},
	})
	km = append(km, Keymap{
		Key: 'e', Mode: ModeNormal, Description: "word end",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionSetCursor, SetCursor: wordEnd(text, cursor, count)}
		},
	})
	km = append(km, Keymap{
		Key: 'E', Mode: ModeNormal, Description: "WORD end",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionSetCursor, SetCursor: wordEndBig(text, cursor, count)}
		},
	})
	km = append(km, Keymap{
		Key: '0', Mode: ModeNormal, Description: "line start",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionSetCursor, SetCursor: lineStartPos(text, cursor)}
		},
	})
	km = append(km, Keymap{
		Key: '^', Mode: ModeNormal, Description: "first non-blank",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionSetCursor, SetCursor: firstNonBlank(text, cursor)}
		},
	})
	km = append(km, Keymap{
		Key: '_', Mode: ModeNormal, Description: "first non-blank",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionSetCursor, SetCursor: firstNonBlank(text, cursor)}
		},
	})
	km = append(km, Keymap{
		Key: '$', Mode: ModeNormal, Description: "line end",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionSetCursor, SetCursor: lineEndPos(text, cursor)}
		},
	})
	km = append(km, Keymap{
		Key: 'g', Mode: ModeNormal, Description: "go to buffer start",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionSetCursor, SetCursor: 0}
		},
	})
	km = append(km, Keymap{
		Key: 'G', Mode: ModeNormal, Description: "go to buffer end",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionSetCursor, SetCursor: len(text)}
		},
	})
	km = append(km, Keymap{
		Key: '{', Mode: ModeNormal, Description: "paragraph backward",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionSetCursor, SetCursor: paragraphBackward(text, cursor, count)}
		},
	})
	km = append(km, Keymap{
		Key: '}', Mode: ModeNormal, Description: "paragraph forward",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionSetCursor, SetCursor: paragraphForward(text, cursor, count)}
		},
	})
	km = append(km, Keymap{
		Key: '%', Mode: ModeNormal, Description: "match bracket",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			if pos := matchBracket(text, cursor); pos >= 0 {
				return Action{Type: ActionSetCursor, SetCursor: pos}
			}
			return Action{Type: ActionNone}
		},
	})

	// ── Normal Mode — character search ───────────────────────────────────────────

	km = append(km, Keymap{
		Key: 'f', Mode: ModeNormal, Description: "find char forward",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			s.Mode = ModeCharSearch
			s.charSearchDir = 1
			s.charSearchTill = false
			s.Count = count
			return Action{Type: ActionNone}
		},
	})
	km = append(km, Keymap{
		Key: 'F', Mode: ModeNormal, Description: "find char backward",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			s.Mode = ModeCharSearch
			s.charSearchDir = -1
			s.charSearchTill = false
			s.Count = count
			return Action{Type: ActionNone}
		},
	})
	km = append(km, Keymap{
		Key: 't', Mode: ModeNormal, Description: "till char forward",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			s.Mode = ModeCharSearch
			s.charSearchDir = 1
			s.charSearchTill = true
			s.Count = count
			return Action{Type: ActionNone}
		},
	})
	km = append(km, Keymap{
		Key: 'T', Mode: ModeNormal, Description: "till char backward",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			s.Mode = ModeCharSearch
			s.charSearchDir = -1
			s.charSearchTill = true
			s.Count = count
			return Action{Type: ActionNone}
		},
	})
	km = append(km, Keymap{
		Key: ';', Mode: ModeNormal, Description: "repeat char search",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			if s.lastCharSearch != 0 {
				pos := findChar(text, cursor, s.lastCharSearch, s.lastCharDir, s.lastCharTill, count)
				if pos >= 0 {
					return Action{Type: ActionSetCursor, SetCursor: pos}
				}
			}
			return Action{Type: ActionNone}
		},
	})
	km = append(km, Keymap{
		Key: ',', Mode: ModeNormal, Description: "repeat char search reverse",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			if s.lastCharSearch != 0 {
				pos := findChar(text, cursor, s.lastCharSearch, -s.lastCharDir, s.lastCharTill, count)
				if pos >= 0 {
					return Action{Type: ActionSetCursor, SetCursor: pos}
				}
			}
			return Action{Type: ActionNone}
		},
	})

	// ── Normal Mode — operators ──────────────────────────────────────────────────

	km = append(km, Keymap{
		Key: 'd', Mode: ModeNormal, Description: "delete operator",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			s.Mode = ModeOperatorPending
			s.PendingOp = 'd'
			s.Count = count
			return Action{Type: ActionNone}
		},
	})
	km = append(km, Keymap{
		Key: 'y', Mode: ModeNormal, Description: "yank operator",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			s.Mode = ModeOperatorPending
			s.PendingOp = 'y'
			s.Count = count
			return Action{Type: ActionNone}
		},
	})
	km = append(km, Keymap{
		Key: 'c', Mode: ModeNormal, Description: "change operator",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			s.Mode = ModeOperatorPending
			s.PendingOp = 'c'
			s.Count = count
			return Action{Type: ActionNone}
		},
	})

	// ── Normal Mode — shortcuts ──────────────────────────────────────────────────

	km = append(km, Keymap{
		Key: 'D', Mode: ModeNormal, Description: "delete to line end",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			end := lineEndPos(text, cursor)
			if cursor < end {
				s.Clipboard = text[cursor:end]
				return Action{Type: ActionDeleteRange, DeleteFrom: cursor, DeleteTo: end}
			}
			return Action{Type: ActionNone}
		},
	})
	km = append(km, Keymap{
		Key: 'C', Mode: ModeNormal, Description: "change to line end",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			s.Mode = ModeInsert
			end := lineEndPos(text, cursor)
			if cursor < end {
				s.Clipboard = text[cursor:end]
				return Action{Type: ActionDeleteRange, DeleteFrom: cursor, DeleteTo: end, SwitchMode: ModeInsert}
			}
			return Action{Type: ActionSwitchMode, SwitchMode: ModeInsert, SetCursor: NoPos}
		},
	})
	km = append(km, Keymap{
		Key: 'S', Mode: ModeNormal, Description: "change whole line",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			s.Mode = ModeInsert
			start := lineStartPos(text, cursor)
			end := lineEndPos(text, cursor)
			s.Clipboard = text[start:end]
			return Action{Type: ActionDeleteRange, DeleteFrom: start, DeleteTo: end, SwitchMode: ModeInsert}
		},
	})
	km = append(km, Keymap{
		Key: 'Y', Mode: ModeNormal, Description: "yank line",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			start := lineStartPos(text, cursor)
			end := lineEndPos(text, cursor)
			if end < len(text) {
				end++
			}
			s.Clipboard = text[start:end]
			return Action{Type: ActionYank, Text: s.Clipboard}
		},
	})

	// ── Normal Mode — direct actions ─────────────────────────────────────────────

	km = append(km, Keymap{
		Key: 'x', Mode: ModeNormal, Description: "delete char under cursor",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			end := cursor + count
			if end > len(text) {
				end = len(text)
			}
			if cursor < end {
				s.Clipboard = text[cursor:end]
				return Action{Type: ActionDeleteRange, DeleteFrom: cursor, DeleteTo: end}
			}
			return Action{Type: ActionNone}
		},
	})
	km = append(km, Keymap{
		Key: 'r', Mode: ModeNormal, Description: "replace char",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			s.Mode = ModeReplace
			s.Count = count
			return Action{Type: ActionNone}
		},
	})
	km = append(km, Keymap{
		Key: 'p', Mode: ModeNormal, Description: "paste after cursor",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			if s.Clipboard != "" {
				return Action{Type: ActionPaste, Text: s.Clipboard, SetCursor: cursor + 1}
			}
			return Action{Type: ActionNone}
		},
	})
	km = append(km, Keymap{
		Key: 'P', Mode: ModeNormal, Description: "paste before cursor",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			if s.Clipboard != "" {
				return Action{Type: ActionPaste, Text: s.Clipboard, SetCursor: cursor}
			}
			return Action{Type: ActionNone}
		},
	})
	km = append(km, Keymap{
		Key: 'u', Mode: ModeNormal, Description: "undo",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionUndo}
		},
	})
	km = append(km, Keymap{
		Key: 18, Mode: ModeNormal, Description: "redo (ctrl+r)", // Ctrl+R
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionRedo}
		},
	})
	km = append(km, Keymap{
		Key: 'J', Mode: ModeNormal, Description: "join lines",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionJoinLines}
		},
	})
	km = append(km, Keymap{
		Key: '~', Mode: ModeNormal, Description: "toggle case",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionToggleCase}
		},
	})
	km = append(km, Keymap{
		Key: '.', Mode: ModeNormal, Description: "repeat last change",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return s.replayLastChange(text, cursor)
		},
	})
	km = append(km, Keymap{
		Key: 27, Mode: ModeNormal, Description: "escape (no-op in normal)",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionNone}
		},
	})

	// ── Visual Mode ──────────────────────────────────────────────────────────────

	km = append(km, Keymap{
		Key: 27, Mode: ModeVisual, Description: "escape to normal",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			s.Mode = ModeNormal
			return Action{Type: ActionSwitchMode, SwitchMode: ModeNormal, SetCursor: NoPos}
		},
	})
	km = append(km, Keymap{
		Key: 'd', Mode: ModeVisual, Description: "delete selection",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			from, to := orderRange(s.VisualStart, cursor)
			to++
			if to > len(text) {
				to = len(text)
			}
			s.Clipboard = text[from:to]
			s.Mode = ModeNormal
			return Action{Type: ActionDeleteRange, DeleteFrom: from, DeleteTo: to, SwitchMode: ModeNormal}
		},
	})
	km = append(km, Keymap{
		Key: 'x', Mode: ModeVisual, Description: "delete selection",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			from, to := orderRange(s.VisualStart, cursor)
			to++
			if to > len(text) {
				to = len(text)
			}
			s.Clipboard = text[from:to]
			s.Mode = ModeNormal
			return Action{Type: ActionDeleteRange, DeleteFrom: from, DeleteTo: to, SwitchMode: ModeNormal}
		},
	})
	km = append(km, Keymap{
		Key: 'y', Mode: ModeVisual, Description: "yank selection",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			from, to := orderRange(s.VisualStart, cursor)
			to++
			if to > len(text) {
				to = len(text)
			}
			s.Clipboard = text[from:to]
			s.Mode = ModeNormal
			return Action{Type: ActionYank, Text: s.Clipboard, SwitchMode: ModeNormal}
		},
	})
	km = append(km, Keymap{
		Key: 'c', Mode: ModeVisual, Description: "change selection",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			from, to := orderRange(s.VisualStart, cursor)
			to++
			if to > len(text) {
				to = len(text)
			}
			s.Clipboard = text[from:to]
			s.Mode = ModeInsert
			return Action{Type: ActionDeleteRange, DeleteFrom: from, DeleteTo: to, SwitchMode: ModeInsert}
		},
	})
	km = append(km, Keymap{
		Key: '~', Mode: ModeVisual, Description: "toggle case of selection",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			from, to := orderRange(s.VisualStart, cursor)
			to++
			if to > len(text) {
				to = len(text)
			}
			s.Mode = ModeNormal
			return Action{Type: ActionToggleCase, DeleteFrom: from, DeleteTo: to, SwitchMode: ModeNormal}
		},
	})
	km = append(km, Keymap{
		Key: 'U', Mode: ModeVisual, Description: "uppercase selection",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			from, to := orderRange(s.VisualStart, cursor)
			to++
			if to > len(text) {
				to = len(text)
			}
			upper := strings.ToUpper(text[from:to])
			s.Mode = ModeNormal
			return Action{Type: ActionDeleteRange, DeleteFrom: from, DeleteTo: to, Text: upper, SwitchMode: ModeNormal}
		},
	})
	km = append(km, Keymap{
		Key: 'u', Mode: ModeVisual, Description: "lowercase selection",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			from, to := orderRange(s.VisualStart, cursor)
			to++
			if to > len(text) {
				to = len(text)
			}
			lower := strings.ToLower(text[from:to])
			s.Mode = ModeNormal
			return Action{Type: ActionDeleteRange, DeleteFrom: from, DeleteTo: to, Text: lower, SwitchMode: ModeNormal}
		},
	})

	// Visual mode motions (extend selection; count always 1 — matches original behavior)
	km = append(km, Keymap{
		Key: 'h', Mode: ModeVisual, Description: "move left",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionMoveCursor, MoveCursor: -1}
		},
	})
	km = append(km, Keymap{
		Key: 'l', Mode: ModeVisual, Description: "move right",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionMoveCursor, MoveCursor: 1}
		},
	})
	km = append(km, Keymap{
		Key: 'j', Mode: ModeVisual, Description: "move down",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return moveLine(text, cursor, 1)
		},
	})
	km = append(km, Keymap{
		Key: 'k', Mode: ModeVisual, Description: "move up",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return moveLine(text, cursor, -1)
		},
	})
	km = append(km, Keymap{
		Key: 'w', Mode: ModeVisual, Description: "word forward",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionSetCursor, SetCursor: wordForward(text, cursor, 1)}
		},
	})
	km = append(km, Keymap{
		Key: 'W', Mode: ModeVisual, Description: "WORD forward",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionSetCursor, SetCursor: wordForwardBig(text, cursor, 1)}
		},
	})
	km = append(km, Keymap{
		Key: 'b', Mode: ModeVisual, Description: "word backward",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionSetCursor, SetCursor: wordBackward(text, cursor, 1)}
		},
	})
	km = append(km, Keymap{
		Key: 'B', Mode: ModeVisual, Description: "WORD backward",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionSetCursor, SetCursor: wordBackwardBig(text, cursor, 1)}
		},
	})
	km = append(km, Keymap{
		Key: 'e', Mode: ModeVisual, Description: "word end",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionSetCursor, SetCursor: wordEnd(text, cursor, 1)}
		},
	})
	km = append(km, Keymap{
		Key: '0', Mode: ModeVisual, Description: "line start",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionSetCursor, SetCursor: lineStartPos(text, cursor)}
		},
	})
	km = append(km, Keymap{
		Key: '^', Mode: ModeVisual, Description: "first non-blank",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionSetCursor, SetCursor: firstNonBlank(text, cursor)}
		},
	})
	km = append(km, Keymap{
		Key: '_', Mode: ModeVisual, Description: "first non-blank",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionSetCursor, SetCursor: firstNonBlank(text, cursor)}
		},
	})
	km = append(km, Keymap{
		Key: '$', Mode: ModeVisual, Description: "line end",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionSetCursor, SetCursor: lineEndPos(text, cursor)}
		},
	})
	km = append(km, Keymap{
		Key: 'g', Mode: ModeVisual, Description: "go to buffer start",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionSetCursor, SetCursor: 0}
		},
	})
	km = append(km, Keymap{
		Key: 'G', Mode: ModeVisual, Description: "go to buffer end",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionSetCursor, SetCursor: len(text)}
		},
	})
	km = append(km, Keymap{
		Key: '{', Mode: ModeVisual, Description: "paragraph backward",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionSetCursor, SetCursor: paragraphBackward(text, cursor, 1)}
		},
	})
	km = append(km, Keymap{
		Key: '}', Mode: ModeVisual, Description: "paragraph forward",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			return Action{Type: ActionSetCursor, SetCursor: paragraphForward(text, cursor, 1)}
		},
	})
	km = append(km, Keymap{
		Key: '%', Mode: ModeVisual, Description: "match bracket",
		Handler: func(key rune, text string, cursor int, count int, s *State) Action {
			if pos := matchBracket(text, cursor); pos >= 0 {
				return Action{Type: ActionSetCursor, SetCursor: pos}
			}
			return Action{Type: ActionNone}
		},
	})

	return km
}
