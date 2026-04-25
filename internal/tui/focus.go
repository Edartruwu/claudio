package tui

// Focus tracks which component has input focus.
type Focus int

const (
	FocusPrompt        Focus = iota
	FocusViewport                    // vim-navigable chat viewport
	FocusPermission                  // tool approval dialog
	FocusModelSelector               // model picker
	FocusAgentSelector               // agent persona picker
	FocusTeamSelector                // team template picker
	FocusPanel                       // any side panel has focus
	FocusPlanApproval                // plan approval dialog after ExitPlanMode
	FocusAskUser                     // AskUser question dialog
	FocusAgentDetail                 // full-screen agent conversation overlay
	FocusFiles                       // file changes panel has focus
)

// PanelID identifies which panel is currently active.
type PanelID int

const (
	PanelNone PanelID = iota
	PanelSessions
	PanelConfig
	PanelSkills
	PanelMemory
	PanelAnalytics
	PanelTasks
	PanelTools
	PanelConversation
	PanelSessionTree
	PanelAgentGUI
)

// OverlayMode controls how a panel is rendered on top of the chat viewport.
type OverlayMode int

const (
	OverlayCentered   OverlayMode = iota // centered modal ~70% w, ~70% h
	OverlayDrawer                        // left side drawer ~35% w, full h
	OverlayFullscreen                    // replaces viewport entirely
	OverlayRightSplit                    // right side 50% split, viewport shrinks left
)

// panelOverlayMode returns the overlay rendering mode for a given panel.
func panelOverlayMode(id PanelID) OverlayMode {
	switch id {
	case PanelAgentGUI, PanelSessionTree, PanelTasks, PanelMemory, PanelSkills:
		return OverlayFullscreen
	case PanelSessions:
		return OverlayDrawer
	case PanelConversation:
		return OverlayRightSplit
	default:
		return OverlayCentered
	}
}
