// Package keymap implements a remappable keymap system for the TUI.
// Actions are typed constants that represent bindable operations.
// The Keymap type maps key sequences to actions and supports runtime
// remapping via :map/:unmap commands with persistence to config.
package keymap

// ActionID is a typed string identifying a bindable action.
type ActionID string

const (
	// Window management
	ActionWindowCycle         ActionID = "window.cycle"
	ActionWindowFocusLeft     ActionID = "window.focus-left"
	ActionWindowFocusDown     ActionID = "window.focus-down"
	ActionWindowFocusUp       ActionID = "window.focus-up"
	ActionWindowFocusRight    ActionID = "window.focus-right"
	ActionWindowSplitVertical ActionID = "window.split-vertical"
	ActionWindowClose         ActionID = "window.close"

	// Buffer/session management
	ActionBufferNext      ActionID = "buffer.next"
	ActionBufferPrev      ActionID = "buffer.prev"
	ActionBufferNew       ActionID = "buffer.new"
	ActionBufferClose     ActionID = "buffer.close"
	ActionBufferRename    ActionID = "buffer.rename"
	ActionBufferList      ActionID = "buffer.list"
	ActionBufferAlternate ActionID = "buffer.alternate"

	// Panels
	ActionPanelSessionTree ActionID = "panel.session-tree"
	ActionPanelAgentGUI    ActionID = "panel.agent-gui"
	ActionPanelSkills      ActionID = "panel.skills"
	ActionPanelMemory      ActionID = "panel.memory"
	ActionPanelTasks       ActionID = "panel.tasks"
	ActionPanelTools       ActionID = "panel.tools"
	ActionPanelAnalytics   ActionID = "panel.analytics"
	ActionPanelFiles       ActionID = "panel.files"
	// Navigation
	ActionSessionPicker  ActionID = "session.picker"
	ActionSessionRecent  ActionID = "session.recent"
	ActionSearch         ActionID = "search"
	ActionCommandPalette ActionID = "command-palette"

	// Editor
	ActionEditorEditPrompt  ActionID = "editor.edit-prompt"
	ActionEditorViewSection ActionID = "editor.view-section"

	// Misc
	ActionTodoDock ActionID = "todo.dock"
)

// ActionMeta holds metadata for a registered action.
type ActionMeta struct {
	ID          ActionID
	Description string
	Group       string // "window", "buffer", "panel", "navigation", "editor", "misc"
}

// Registry maps every action ID to its metadata.
var Registry = map[ActionID]ActionMeta{
	// Window management
	ActionWindowCycle:         {ActionWindowCycle, "Cycle windows", "window"},
	ActionWindowFocusLeft:     {ActionWindowFocusLeft, "Focus left window", "window"},
	ActionWindowFocusDown:     {ActionWindowFocusDown, "Focus prompt", "window"},
	ActionWindowFocusUp:       {ActionWindowFocusUp, "Focus viewport", "window"},
	ActionWindowFocusRight:    {ActionWindowFocusRight, "Focus right panel", "window"},
	ActionWindowSplitVertical: {ActionWindowSplitVertical, "Mirror panel", "window"},
	ActionWindowClose:         {ActionWindowClose, "Close panel", "window"},

	// Buffer/session management
	ActionBufferNext:      {ActionBufferNext, "Next session", "buffer"},
	ActionBufferPrev:      {ActionBufferPrev, "Previous session", "buffer"},
	ActionBufferNew:       {ActionBufferNew, "New session", "buffer"},
	ActionBufferClose:     {ActionBufferClose, "Kill session", "buffer"},
	ActionBufferRename:    {ActionBufferRename, "Rename session", "buffer"},
	ActionBufferList:      {ActionBufferList, "List sessions", "buffer"},
	ActionBufferAlternate: {ActionBufferAlternate, "Alternate session", "buffer"},

	// Panels
	ActionPanelSessionTree: {ActionPanelSessionTree, "Session tree (STREE)", "panel"},
	ActionPanelAgentGUI:    {ActionPanelAgentGUI, "Agent inspector (AGUI)", "panel"},
	ActionPanelSkills:      {ActionPanelSkills, "Skills panel", "panel"},
	ActionPanelMemory:      {ActionPanelMemory, "Memory panel", "panel"},
	ActionPanelTasks:       {ActionPanelTasks, "Tasks panel", "panel"},
	ActionPanelTools:       {ActionPanelTools, "Tools panel", "panel"},
	ActionPanelAnalytics:   {ActionPanelAnalytics, "Analytics panel", "panel"},
	ActionPanelFiles:       {ActionPanelFiles, "Files panel", "panel"},
	// Navigation
	ActionSessionPicker:  {ActionSessionPicker, "Session picker", "navigation"},
	ActionSessionRecent:  {ActionSessionRecent, "Recent sessions", "navigation"},
	ActionSearch:         {ActionSearch, "Search sessions", "navigation"},
	ActionCommandPalette: {ActionCommandPalette, "Command palette", "navigation"},

	// Editor
	ActionEditorEditPrompt:  {ActionEditorEditPrompt, "Edit prompt in $EDITOR", "editor"},
	ActionEditorViewSection: {ActionEditorViewSection, "View section in $EDITOR", "editor"},

	// Misc
	ActionTodoDock: {ActionTodoDock, "Toggle todo dock", "misc"},
}

// ValidAction returns true if the given action ID exists in the registry.
func ValidAction(id ActionID) bool {
	_, ok := Registry[id]
	return ok
}

// Groups returns all unique group names from the registry.
func Groups() []string {
	seen := map[string]bool{}
	var groups []string
	for _, meta := range Registry {
		if !seen[meta.Group] {
			seen[meta.Group] = true
			groups = append(groups, meta.Group)
		}
	}
	return groups
}
