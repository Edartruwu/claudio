// Package docks provides inline dock components that appear between the
// viewport and prompt in the TUI layout.
package docks

import tea "github.com/charmbracelet/bubbletea"

// Dock is the interface implemented by all inline dock components.
type Dock interface {
	// IsActive returns true when this dock should be shown.
	IsActive() bool
	// SetWidth sets the available display width.
	SetWidth(w int)
	// View renders the dock content (no trailing newline).
	View() string
	// Height returns the number of lines this dock will render.
	Height() int
	// Update handles a key event. Returns (Cmd, consumed bool).
	Update(msg tea.KeyMsg) (tea.Cmd, bool)
}

// ActiveDock returns the highest-priority active dock from the provided list,
// or nil if none are active.
func ActiveDock(docks []Dock) Dock {
	for _, d := range docks {
		if d != nil && d.IsActive() {
			return d
		}
	}
	return nil
}
