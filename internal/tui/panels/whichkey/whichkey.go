// Package whichkey implements a which-key style popup that shows available
// leader key sequences after a brief timeout.
package whichkey

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
func DefaultBindings() []Binding {
	return []Binding{
		{Key: "p", Desc: "palette"},
		{Key: "f", Desc: "file changes"},
		{Key: "t", Desc: "todo dock"},
		{Key: "a", Desc: "agents"},
		{Key: ".", Desc: "sessions"},
		{Key: "b", Desc: "buffer..."},
		{Key: "i", Desc: "info panels..."},
		{Key: "w", Desc: "window..."},
	}
}

// WindowBindings returns bindings for the Space+W sub-menu.
func WindowBindings() []Binding {
	return []Binding{
		{Key: "k", Desc: "viewport"},
		{Key: "j", Desc: "prompt"},
	}
}

// SessionBindings returns bindings for the Space+B sub-menu.
func SessionBindings() []Binding {
	return []Binding{
		{Key: "n", Desc: "next session"},
		{Key: "p", Desc: "prev session"},
		{Key: "c", Desc: "create session"},
		{Key: "k", Desc: "kill session"},
		{Key: "r", Desc: "rename session"},
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

	keyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#fabd2f")). // warning/yellow
		Bold(true)

	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#bdae93")) // dim

	sepStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#504945")) // subtle

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#d3869b")). // primary
		Bold(true)

	var lines []string
	lines = append(lines, titleStyle.Render(" <Space> "))
	lines = append(lines, sepStyle.Render(strings.Repeat("─", 22)))

	for _, b := range m.bindings {
		line := "  " + keyStyle.Render(b.Key) + " " + sepStyle.Render("→") + " " + descStyle.Render(b.Desc)
		lines = append(lines, line)
	}

	content := strings.Join(lines, "\n")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#3c3836")). // surfaceAlt
		Padding(0, 1).
		Render(content)

	return box
}
