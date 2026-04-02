// Package keybindings provides customizable keyboard shortcuts for the TUI.
// Users can override defaults via ~/.claudio/keybindings.json.
package keybindings

import (
	"encoding/json"
	"os"
	"strings"
)

// Action identifies what a keybinding does.
type Action string

// All available keybinding actions.
const (
	// Navigation
	ActionFocusViewport Action = "focus_viewport"
	ActionFocusPrompt   Action = "focus_prompt"

	// Session management
	ActionOpenSessions    Action = "open_sessions"
	ActionRecentSessions  Action = "recent_sessions"
	ActionAlternateSession Action = "alternate_session"
	ActionSearchSessions  Action = "search_sessions"
	ActionNextSession     Action = "next_session"
	ActionPrevSession     Action = "prev_session"
	ActionCreateSession   Action = "create_session"
	ActionDeleteSession   Action = "delete_session"
	ActionRenameSession   Action = "rename_session"

	// Panels
	ActionOpenConfig    Action = "open_config"
	ActionOpenSkills    Action = "open_skills"
	ActionOpenMemory    Action = "open_memory"
	ActionOpenAnalytics Action = "open_analytics"
	ActionOpenTasks     Action = "open_tasks"

	// Tool controls
	ActionToggleExpand     Action = "toggle_expand"
	ActionToggleLastExpand Action = "toggle_last_expand"
	ActionCyclePermission  Action = "cycle_permission"

	// Viewport navigation
	ActionScrollDown     Action = "scroll_down"
	ActionScrollUp       Action = "scroll_up"
	ActionHalfPageDown   Action = "half_page_down"
	ActionHalfPageUp     Action = "half_page_up"
	ActionGotoTop        Action = "goto_top"
	ActionGotoBottom     Action = "goto_bottom"

	// Editor
	ActionExternalEditor Action = "external_editor"
	ActionPasteImage     Action = "paste_image"
)

// Binding maps a key sequence to an action in a specific context.
type Binding struct {
	Keys    string `json:"keys"`              // e.g., "space b n", "ctrl+g", "shift+tab"
	Action  Action `json:"action"`
	Context string `json:"context,omitempty"` // "global", "viewport", "prompt", "normal"; empty = global
}

// KeyMap holds all keybindings and provides lookup.
type KeyMap struct {
	bindings []Binding
	reserved map[string]bool
}

// Load reads keybindings from a JSON file and merges with defaults.
// User bindings override defaults for the same action.
func Load(path string) *KeyMap {
	km := DefaultKeyMap()

	data, err := os.ReadFile(path)
	if err != nil {
		return km // file doesn't exist, use defaults
	}

	var userBindings []Binding
	if err := json.Unmarshal(data, &userBindings); err != nil {
		return km // invalid JSON, use defaults
	}

	// Build action → user binding map
	overrides := make(map[Action]Binding)
	for _, b := range userBindings {
		overrides[b.Action] = b
	}

	// Replace defaults with user overrides
	var merged []Binding
	seen := make(map[Action]bool)
	for _, b := range km.bindings {
		if override, ok := overrides[b.Action]; ok {
			merged = append(merged, override)
			seen[b.Action] = true
		} else {
			merged = append(merged, b)
		}
		seen[b.Action] = true
	}

	// Add any user bindings for actions not in defaults
	for _, b := range userBindings {
		if !seen[b.Action] {
			merged = append(merged, b)
		}
	}

	km.bindings = merged
	return km
}

// DefaultKeyMap returns the built-in default keybindings.
func DefaultKeyMap() *KeyMap {
	return &KeyMap{
		bindings: []Binding{
			// Leader (space) bindings (context: normal = vim normal mode)
			{Keys: "space .", Action: ActionOpenSessions, Context: "normal"},
			{Keys: "space ;", Action: ActionRecentSessions, Context: "normal"},
			{Keys: "space ,", Action: ActionAlternateSession, Context: "normal"},
			{Keys: "space /", Action: ActionSearchSessions, Context: "normal"},

			// Buffer management
			{Keys: "space b n", Action: ActionNextSession, Context: "normal"},
			{Keys: "space b p", Action: ActionPrevSession, Context: "normal"},
			{Keys: "space b c", Action: ActionCreateSession, Context: "normal"},
			{Keys: "space b k", Action: ActionDeleteSession, Context: "normal"},
			{Keys: "space b r", Action: ActionRenameSession, Context: "normal"},

			// Info panels
			{Keys: "space i c", Action: ActionOpenConfig, Context: "normal"},
			{Keys: "space i k", Action: ActionOpenSkills, Context: "normal"},
			{Keys: "space i m", Action: ActionOpenMemory, Context: "normal"},
			{Keys: "space i a", Action: ActionOpenAnalytics, Context: "normal"},
			{Keys: "space i t", Action: ActionOpenTasks, Context: "normal"},

			// Window focus
			{Keys: "space w k", Action: ActionFocusViewport, Context: "normal"},
			{Keys: "space w j", Action: ActionFocusPrompt, Context: "normal"},

			// Global shortcuts
			{Keys: "shift+tab", Action: ActionCyclePermission, Context: "global"},
			{Keys: "ctrl+o", Action: ActionToggleLastExpand, Context: "global"},
			{Keys: "ctrl+g", Action: ActionExternalEditor, Context: "prompt"},
			{Keys: "ctrl+v", Action: ActionPasteImage, Context: "prompt"},

			// Viewport navigation (when focused)
			{Keys: "j", Action: ActionScrollDown, Context: "viewport"},
			{Keys: "k", Action: ActionScrollUp, Context: "viewport"},
			{Keys: "ctrl+d", Action: ActionHalfPageDown, Context: "viewport"},
			{Keys: "ctrl+u", Action: ActionHalfPageUp, Context: "viewport"},
			{Keys: "g", Action: ActionGotoTop, Context: "viewport"},
			{Keys: "G", Action: ActionGotoBottom, Context: "viewport"},
			{Keys: "enter", Action: ActionToggleExpand, Context: "viewport"},
		},
		reserved: map[string]bool{
			"ctrl+c": true, // always exits
			"esc":    true, // always dismisses
		},
	}
}

// Lookup finds the action for a key sequence in the given context.
// Checks context-specific bindings first, then global bindings.
func (km *KeyMap) Lookup(keys string, context string) (Action, bool) {
	keys = normalizeKeys(keys)

	// First pass: exact context match
	for _, b := range km.bindings {
		if normalizeKeys(b.Keys) == keys && (b.Context == context || b.Context == "") {
			return b.Action, true
		}
	}

	// Second pass: global fallback
	if context != "global" {
		for _, b := range km.bindings {
			if normalizeKeys(b.Keys) == keys && b.Context == "global" {
				return b.Action, true
			}
		}
	}

	return "", false
}

// IsReserved returns true if the key sequence cannot be rebound.
func (km *KeyMap) IsReserved(keys string) bool {
	return km.reserved[normalizeKeys(keys)]
}

// BindingsForContext returns all bindings matching a context (for which-key display).
func (km *KeyMap) BindingsForContext(context string) []Binding {
	var result []Binding
	for _, b := range km.bindings {
		if b.Context == context || b.Context == "global" || b.Context == "" {
			result = append(result, b)
		}
	}
	return result
}

// GenerateTemplate returns a JSON template users can customize.
func GenerateTemplate() []byte {
	km := DefaultKeyMap()
	data, _ := json.MarshalIndent(km.bindings, "", "  ")
	return data
}

func normalizeKeys(keys string) string {
	return strings.ToLower(strings.TrimSpace(keys))
}
