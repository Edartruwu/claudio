// Package panels defines the Panel interface and shared types for TUI side panels.
package panels

import tea "github.com/charmbracelet/bubbletea"

// Panel is the interface that all side panels implement.
// Panels render in the right portion of a split layout and receive
// key events when they have focus.
type Panel interface {
	// IsActive returns true when the panel is visible and should be rendered.
	IsActive() bool

	// Activate makes the panel visible and refreshes its data.
	Activate()

	// Deactivate hides the panel.
	Deactivate()

	// SetSize sets the available width and height for the panel.
	SetSize(w, h int)

	// Update handles a key event. Returns a tea.Cmd and whether the key was consumed.
	Update(msg tea.KeyMsg) (tea.Cmd, bool)

	// View renders the panel content.
	View() string
}

// CloseMsg is sent by a panel when it wants to close itself.
type CloseMsg struct{}

// ActionMsg is sent by a panel when it wants the root model to perform an action.
type ActionMsg struct {
	Type    string // action type identifier
	Payload any    // action-specific data
}
