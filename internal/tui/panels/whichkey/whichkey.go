// Package whichkey implements a which-key style popup that shows available
// leader key sequences after a brief timeout.
package whichkey

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// Timeout is the delay before showing the popup after the leader key is pressed.
const Timeout = 300 * time.Millisecond

// TimeoutMsg is sent when the leader key timeout elapses.
type TimeoutMsg struct{}

// Binding represents a single leader key binding.
type Binding struct {
	Key  string
	Desc string
}

// DefaultBindings returns the standard leader key bindings.
// Uses a special layout with grouped rows and special formatting.
func DefaultBindings() []Binding {
	return []Binding{
		// Group 1: Quick commands
		{Key: "p", Desc: "palette"},
		{Key: "f", Desc: "files"},
		{Key: "a", Desc: "agents"},
		{Key: "/", Desc: "search"},
		// Divider row (empty key)
		{Key: "", Desc: ""},
		{Key: ".", Desc: "sessions"},
		{Key: ";", Desc: "recent"},
		// Divider row (empty key)
		{Key: "", Desc: ""},
		{Key: "w", Desc: "windows"},
		{Key: "b", Desc: "buffers"},
		// Divider row (empty key)
		{Key: "", Desc: ""},
		{Key: "C", Desc: "config"},
		{Key: "K", Desc: "skills"},
		{Key: "M", Desc: "memory"},
		{Key: "T", Desc: "tasks"},
		{Key: "O", Desc: "tools"},
		{Key: "A", Desc: "analytics"},
	}
}

// WindowBindings returns bindings for the Space+W sub-menu.
func WindowBindings() []Binding {
	return []Binding{
		{Key: "w", Desc: "cycle"},
		{Key: "h", Desc: "←"},
		{Key: "j", Desc: "↓"},
		{Key: "k", Desc: "↑"},
		{Key: "l", Desc: "→"},
		{Key: "v", Desc: "mirror"},
		{Key: "z", Desc: "zoom"},
		{Key: "q", Desc: "close"},
		{Key: "=", Desc: "reset"},
		{Key: ">", Desc: "widen"},
		{Key: "<", Desc: "narrow"},
	}
}

// SessionBindings returns bindings for the Space+B sub-menu.
func SessionBindings() []Binding {
	return []Binding{
		{Key: "n", Desc: "next"},
		{Key: "p", Desc: "prev"},
		{Key: "c", Desc: "new"},
		{Key: "k", Desc: "kill"},
		{Key: "r", Desc: "rename"},
		{Key: ".", Desc: "alternate"},
	}
}

// PanelBindings returns bindings for the Space+I sub-menu.
func PanelBindings() []Binding {
	return []Binding{
		{Key: "c", Desc: "config"},
		{Key: "k", Desc: "skills"},
		{Key: "m", Desc: "memory/rules"},
		{Key: "a", Desc: "analytics"},
		{Key: "t", Desc: "tasks"},
		{Key: "o", Desc: "tools"},
	}
}

// Model is the which-key popup overlay.
type Model struct {
	active   bool
	bindings []Binding
	width    int
}

// New creates a new which-key popup.
func New() Model {
	return Model{
		bindings: DefaultBindings(),
	}
}

// Show activates the popup with the given bindings.
func (m *Model) Show(bindings []Binding) {
	m.active = true
	m.bindings = bindings
}

// ShowDefault shows the default leader bindings.
func (m *Model) ShowDefault() {
	m.Show(DefaultBindings())
}

// ShowWindow shows the window sub-menu bindings.
func (m *Model) ShowWindow() {
	m.Show(WindowBindings())
}

// ShowSessions shows the session sub-menu bindings.
func (m *Model) ShowSessions() {
	m.Show(SessionBindings())
}

// Hide dismisses the popup.
func (m *Model) Hide() {
	m.active = false
}

// IsActive returns whether the popup is visible.
func (m Model) IsActive() bool {
	return m.active
}

// SetWidth sets the available width for rendering.
func (m *Model) SetWidth(w int) {
	m.width = w
}

// ScheduleTimeout returns a tea.Cmd that sends TimeoutMsg after the delay.
func ScheduleTimeout() tea.Cmd {
	return tea.Tick(Timeout, func(time.Time) tea.Msg {
		return TimeoutMsg{}
	})
}

// View renders the popup.
func (m Model) View() string {
	if !m.active || len(m.bindings) == 0 {
		return ""
	}

	var lines []string
	lines = append(lines, styles.WhichKeyTitle.Render(" <Space> bindings "))
	lines = append(lines, styles.WhichKeySep.Render(strings.Repeat("─", 40)))

	// Check if this is the default bindings with grouped layout (has divider entries)
	hasDividers := false
	for _, b := range m.bindings {
		if b.Key == "" && b.Desc == "" {
			hasDividers = true
			break
		}
	}

	if hasDividers {
		// Grouped layout for default bindings
		var row []string
		for _, b := range m.bindings {
			if b.Key == "" && b.Desc == "" {
				// Divider: render current row and add separator
				if len(row) > 0 {
					lines = append(lines, "  "+strings.Join(row, "    "))
					row = nil
				}
				lines = append(lines, styles.WhichKeySep.Render(strings.Repeat("─", 40)))
			} else {
				// Add binding to current row
				binding := styles.WhichKeyKey.Render(b.Key) + " " + styles.WhichKeyDesc.Render(b.Desc)
				row = append(row, binding)
			}
		}
		// Render final row if any
		if len(row) > 0 {
			lines = append(lines, "  "+strings.Join(row, "    "))
		}
	} else {
		// Regular layout for sub-menus
		for _, b := range m.bindings {
			line := "  " + styles.WhichKeyKey.Render(b.Key) + " " + styles.WhichKeySep.Render("→") + " " + styles.WhichKeyDesc.Render(b.Desc)
			lines = append(lines, line)
		}
	}

	content := strings.Join(lines, "\n")

	box := styles.WhichKeyBorder.
		Padding(0, 1).
		Render(content)

	return box
}
