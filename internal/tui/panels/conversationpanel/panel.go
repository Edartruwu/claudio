// Package conversationpanel implements the conversation mirror side panel.
// It shows a read-only scrollable copy of the main chat viewport, allowing
// the user to browse history while the prompt stays focused.
package conversationpanel

import (
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// Panel is the conversation mirror side panel.
type Panel struct {
	viewport viewport.Model
	width    int
	height   int
	active   bool
	ready    bool
}

// New creates a new conversation mirror panel.
func New() *Panel {
	return &Panel{}
}

// SetContent updates the panel content to mirror the main chat viewport.
func (p *Panel) SetContent(content string) {
	if p.ready {
		p.viewport.SetContent(content)
	}
}

// IsActive returns true when the panel is visible and should be rendered.
func (p *Panel) IsActive() bool { return p.active }

// Activate makes the panel visible.
func (p *Panel) Activate() { p.active = true }

// Deactivate hides the panel.
func (p *Panel) Deactivate() { p.active = false }

// SetSize sets the available width and height for the panel.
func (p *Panel) SetSize(w, h int) {
	p.width = w
	p.height = h
	p.viewport.Width = w
	p.viewport.Height = h - 1 // reserve 1 line for Help() footer
	p.ready = true
}

// Update handles a key event. Returns a tea.Cmd and whether the key was consumed.
func (p *Panel) Update(msg tea.KeyMsg) (tea.Cmd, bool) {
	if !p.active {
		return nil, false
	}
	switch msg.String() {
	case "j", "down":
		p.viewport.ScrollDown(1)
		return nil, true
	case "k", "up":
		p.viewport.ScrollUp(1)
		return nil, true
	case "ctrl+d":
		p.viewport.HalfPageDown()
		return nil, true
	case "ctrl+u":
		p.viewport.HalfPageUp()
		return nil, true
	case "G":
		p.viewport.GotoBottom()
		return nil, true
	case "g":
		p.viewport.GotoTop()
		return nil, true
	}
	return nil, false
}

// View renders the panel content.
func (p *Panel) View() string {
	if !p.ready {
		return ""
	}
	return p.viewport.View()
}

// Help returns a short one-line hint string for keybindings.
func (p *Panel) Help() string {
	return "j/k scroll · ctrl+d/u half-page · G bottom · g top · esc close"
}
