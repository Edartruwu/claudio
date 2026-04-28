package keymap

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Abraxas-365/claudio/internal/config"
)

// Binding pairs a key sequence with its action metadata for display.
type Binding struct {
	KeySeq string
	Action ActionMeta
}

// Keymap maps key sequences to action IDs. It tracks user overrides
// separately from defaults so only changes are persisted.
type Keymap struct {
	bindings  map[string]ActionID // effective bindings (defaults + overrides)
	overrides map[string]ActionID // user-defined only, for persistence
	removed   map[string]bool    // keys explicitly removed by user (to persist removals)
}

// defaultBindings returns the project's default key-to-action mapping.
// These mirror the hardcoded bindings from handleLeaderKey.
func defaultBindings() map[string]ActionID {
	return map[string]ActionID{
		// Window sub-menu
		"ww": ActionWindowCycle,
		"wh": ActionWindowFocusLeft,
		"wj": ActionWindowFocusDown,
		"wk": ActionWindowFocusUp,
		"wl": ActionWindowFocusRight,
		"wp": ActionWindowFocusDown, // alias: wj/wp both go to prompt
		"wv": ActionWindowSplitVertical,
		"wq": ActionWindowClose,
		"wc": ActionFloatWindowClose,
		"wo": ActionFloatWindowHint,

		// Buffer/session sub-menu
		"bn": ActionBufferNext,
		"bp": ActionBufferPrev,
		"bc": ActionBufferNew,
		"bk": ActionBufferClose,
		"br": ActionBufferRename,
		"bl": ActionBufferList,

		// Buffer alternate (,<enter>)
		",\n": ActionBufferAlternate,

		// Panels (direct leader keys)
		"op": ActionPanelSessionTree,
		"oa": ActionPanelAgentGUI,
		"C":  ActionPanelConfig,
		"K":  ActionPanelSkills,
		"M":  ActionPanelMemory,
		"T":  ActionPanelTasks,
		"O":  ActionPanelTools,
		"A":  ActionPanelAnalytics,
		"f":  ActionPanelFiles,
		"a":  ActionPanelAgentGUI,

		// Info panel sub-menu (i prefix)
		"ic": ActionPanelConfig,
		"ik": ActionPanelSkills,
		"im": ActionPanelMemory,
		"ia": ActionPanelAnalytics,
		"it": ActionPanelTasks,
		"io": ActionPanelTools,

		// Navigation
		".": ActionSessionPicker,
		"/": ActionSearch,
		";": ActionSessionRecent,
		"p": ActionCommandPalette,

		// Editor
		"e":  ActionEditorEditPrompt,
		"ev": ActionEditorViewSection,

		// Misc
		"t": ActionTodoDock,
	}
}

// Default returns a Keymap with the project's default bindings.
func Default() *Keymap {
	km := &Keymap{
		bindings:  defaultBindings(),
		overrides: make(map[string]ActionID),
		removed:   make(map[string]bool),
	}
	return km
}

// Resolve returns the ActionID for a key sequence, false if not bound.
func (km *Keymap) Resolve(keySeq string) (ActionID, bool) {
	a, ok := km.bindings[keySeq]
	return a, ok
}

// HasPrefix returns true if keySeq is a prefix of any binding but not a
// complete binding itself. This tells the leader key handler to keep
// collecting keys (and show which-key hints).
func (km *Keymap) HasPrefix(prefix string) bool {
	for k := range km.bindings {
		if len(k) > len(prefix) && strings.HasPrefix(k, prefix) {
			return true
		}
	}
	return false
}

// IsPrefix returns true if keySeq is a prefix of any binding (may also be
// a complete binding). Used to decide whether to start a sub-sequence.
func (km *Keymap) IsPrefix(prefix string) bool {
	for k := range km.bindings {
		if strings.HasPrefix(k, prefix) {
			return true
		}
	}
	return false
}

// Set adds or overwrites a binding. The override is persisted to config.
func (km *Keymap) Set(keySeq string, action ActionID) error {
	if !ValidAction(action) {
		return fmt.Errorf("unknown action: %s", action)
	}
	km.bindings[keySeq] = action
	km.overrides[keySeq] = action
	delete(km.removed, keySeq)
	return km.save()
}

// Unset removes a binding. The removal is persisted to config.
func (km *Keymap) Unset(keySeq string) error {
	if _, ok := km.bindings[keySeq]; !ok {
		return fmt.Errorf("no binding for key sequence: %s", keySeq)
	}
	delete(km.bindings, keySeq)
	delete(km.overrides, keySeq)
	// If this was a default binding, mark it as explicitly removed.
	if _, isDefault := defaultBindings()[keySeq]; isDefault {
		km.removed[keySeq] = true
	}
	return km.save()
}

// List returns all bindings sorted by key sequence, optionally filtered by
// action group (e.g. "window", "buffer"). Pass "" for all bindings.
func (km *Keymap) List(group string) []Binding {
	var result []Binding
	for keySeq, actionID := range km.bindings {
		meta, ok := Registry[actionID]
		if !ok {
			continue
		}
		if group != "" && meta.Group != group {
			continue
		}
		result = append(result, Binding{KeySeq: keySeq, Action: meta})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].KeySeq < result[j].KeySeq
	})
	return result
}

// BindingsForPrefix returns all bindings whose key sequences start with
// the given prefix. The returned Binding.KeySeq contains only the suffix
// after the prefix (for which-key display).
func (km *Keymap) BindingsForPrefix(prefix string) []Binding {
	var result []Binding
	for keySeq, actionID := range km.bindings {
		if !strings.HasPrefix(keySeq, prefix) || keySeq == prefix {
			continue
		}
		meta, ok := Registry[actionID]
		if !ok {
			continue
		}
		suffix := keySeq[len(prefix):]
		result = append(result, Binding{KeySeq: suffix, Action: meta})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].KeySeq < result[j].KeySeq
	})
	return result
}

// ── Persistence ─────────────────────────────────────────

// keymapFile is stored alongside settings.json in ~/.claudio/
const keymapFileName = "keymap.json"

// persistedKeymap is the JSON schema for keymap overrides.
type persistedKeymap struct {
	Bindings map[string]ActionID `json:"bindings,omitempty"` // user overrides
	Removed  []string           `json:"removed,omitempty"`  // explicitly removed defaults
}

func keymapPath() string {
	p := config.GetPaths()
	return filepath.Join(p.Home, keymapFileName)
}

// Load reads keymap overrides from config and merges onto Default().
func Load() (*Keymap, error) {
	km := Default()
	path := keymapPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return km, nil // no overrides, use defaults
		}
		return km, fmt.Errorf("reading keymap config: %w", err)
	}

	var persisted persistedKeymap
	if err := json.Unmarshal(data, &persisted); err != nil {
		return km, fmt.Errorf("parsing keymap config: %w", err)
	}

	// Apply removals
	for _, key := range persisted.Removed {
		delete(km.bindings, key)
		km.removed[key] = true
	}

	// Apply overrides
	for keySeq, actionID := range persisted.Bindings {
		if ValidAction(actionID) {
			km.bindings[keySeq] = actionID
			km.overrides[keySeq] = actionID
		}
	}

	return km, nil
}

// save persists only user overrides (not the full default set).
func (km *Keymap) save() error {
	if len(km.overrides) == 0 && len(km.removed) == 0 {
		// Nothing to persist — remove file if it exists
		os.Remove(keymapPath())
		return nil
	}

	persisted := persistedKeymap{
		Bindings: km.overrides,
	}
	for key := range km.removed {
		persisted.Removed = append(persisted.Removed, key)
	}
	sort.Strings(persisted.Removed)

	data, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling keymap config: %w", err)
	}
	return os.WriteFile(keymapPath(), data, 0644)
}
