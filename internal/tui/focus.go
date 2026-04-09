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
	PanelAgents
	PanelTools
	PanelConversation
	PanelSessionTree // TODO: implemented in stree package
	PanelAgentGUI    // TODO: implemented in agui package
)
