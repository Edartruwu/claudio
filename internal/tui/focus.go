package tui

// Focus tracks which component has input focus.
type Focus int

const (
	FocusPrompt        Focus = iota
	FocusViewport                    // vim-navigable chat viewport
	FocusPermission                  // tool approval dialog
	FocusModelSelector               // model picker
	FocusPanel                       // any side panel has focus
	FocusPlanApproval                // plan approval dialog after ExitPlanMode
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
)
