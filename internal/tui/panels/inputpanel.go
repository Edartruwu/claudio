// Package panels defines the Panel interface and shared types for TUI side panels.
package panels

import tea "github.com/charmbracelet/bubbletea"

// InputPanel is an optional interface panels can implement to get
// a persistent input bar rendered below their panel view.
// Root wires this: when HasInput() is true, all keys route to InputUpdate
// instead of the normal Update handler.
type InputPanel interface {
	// HasInput returns true when the input bar should be shown and active.
	HasInput() bool

	// InputUpdate handles a key event for the input bar.
	// Called by root instead of Update while HasInput() is true.
	InputUpdate(msg tea.KeyMsg) tea.Cmd

	// InputView renders the input bar (full panel width, one line).
	// Called after SetSize, so the panel's stored width is already current.
	InputView() string
}
