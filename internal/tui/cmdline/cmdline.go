// Package cmdline implements an nvim-style ":" command line for the TUI.
// Press ":" in normal vim mode → command line appears at the bottom.
// Tab autocompletes, Enter executes, Esc cancels.
package cmdline

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/cli/commands"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// ExecuteMsg is sent when the user presses Enter.
type ExecuteMsg struct {
	Raw  string // full raw input e.g. "theme tokyonight"
	Name string // command name e.g. "theme"
	Args string // everything after name e.g. "tokyonight"
}

// CancelMsg is sent when the user presses Esc.
type CancelMsg struct{}

// Model is the cmdline BubbleTea component.
type Model struct {
	input    textinput.Model
	active   bool
	width    int
	registry *commands.Registry

	history []string
	histIdx int // -1 = not browsing history

	// autocomplete state
	suggestions []string
	suggIdx     int
}

// New creates a cmdline model backed by the given command registry.
func New(reg *commands.Registry) Model {
	ti := textinput.New()
	ti.Prompt = ":"
	ti.PromptStyle = lipgloss.NewStyle().Foreground(styles.Primary).Bold(true)
	ti.TextStyle = lipgloss.NewStyle().Foreground(styles.Text)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(styles.Muted)
	ti.CharLimit = 1024

	return Model{
		input:    ti,
		registry: reg,
		histIdx:  -1,
	}
}

// SetWidth sets the render width.
func (m *Model) SetWidth(w int) {
	m.width = w
	m.input.Width = w - 2 // leave room for border/padding
}

// Activate opens the command line and focuses the input.
func (m *Model) Activate() {
	m.active = true
	m.input.SetValue("")
	m.input.Focus()
	m.histIdx = -1
	m.suggestions = nil
	m.suggIdx = 0
}

// Deactivate closes the command line.
func (m *Model) Deactivate() {
	m.active = false
	m.input.Blur()
}

// IsActive reports whether the command line is open.
func (m Model) IsActive() bool { return m.active }

// Value returns the current input text.
func (m Model) Value() string { return m.input.Value() }

// Update handles key events while active.
func (m Model) Update(msg tea.KeyMsg) (Model, tea.Cmd) {
	if !m.active {
		return m, nil
	}

	switch msg.Type {
	case tea.KeyEsc:
		m.Deactivate()
		return m, func() tea.Msg { return CancelMsg{} }

	case tea.KeyEnter:
		raw := strings.TrimSpace(m.input.Value())
		m.Deactivate()
		if raw == "" {
			return m, func() tea.Msg { return CancelMsg{} }
		}
		// Push to history (deduplicate consecutive)
		if len(m.history) == 0 || m.history[len(m.history)-1] != raw {
			m.history = append(m.history, raw)
		}
		name, args := parseCmdLine(raw)
		return m, func() tea.Msg {
			return ExecuteMsg{Raw: raw, Name: name, Args: args}
		}

	case tea.KeyUp:
		// Browse history backwards
		if len(m.history) == 0 {
			break
		}
		if m.histIdx == -1 {
			m.histIdx = len(m.history) - 1
		} else if m.histIdx > 0 {
			m.histIdx--
		}
		m.input.SetValue(m.history[m.histIdx])
		m.input.CursorEnd()
		return m, nil

	case tea.KeyDown:
		// Browse history forwards
		if m.histIdx == -1 {
			break
		}
		m.histIdx++
		if m.histIdx >= len(m.history) {
			m.histIdx = -1
			m.input.SetValue("")
		} else {
			m.input.SetValue(m.history[m.histIdx])
			m.input.CursorEnd()
		}
		return m, nil

	case tea.KeyTab:
		m.autocomplete()
		return m, nil

	case tea.KeySpace:
		// Reset autocomplete state when user types a space (starts arguments).
		m.suggestions = nil
		m.suggIdx = 0
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
		return m, cmd
	}

	// Reset suggestions when user types normally
	if msg.Type == tea.KeyRunes || msg.Type == tea.KeyBackspace || msg.Type == tea.KeyDelete || msg.Type == tea.KeySpace {
		m.suggestions = nil
		m.suggIdx = 0
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// autocomplete fills the input with the next matching command name.
func (m *Model) autocomplete() {
	if m.registry == nil {
		return
	}
	// Only complete the first word (command name).
	val := m.input.Value()
	if strings.Contains(val, " ") {
		return
	}

	if len(m.suggestions) == 0 {
		// Build suggestion list from registry.
		for _, cmd := range m.registry.ListCommands() {
			if strings.HasPrefix(cmd.Name, val) {
				m.suggestions = append(m.suggestions, cmd.Name)
			}
		}
		m.suggIdx = 0
	}

	if len(m.suggestions) == 0 {
		return
	}

	m.input.SetValue(m.suggestions[m.suggIdx])
	m.input.CursorEnd()
	m.suggIdx = (m.suggIdx + 1) % len(m.suggestions)
}

// View renders the command line bar.
func (m Model) View() string {
	if !m.active {
		return ""
	}

	bar := lipgloss.NewStyle().
		Width(m.width).
		Background(styles.Surface).
		Foreground(styles.Text).
		Padding(0, 1).
		Render(m.input.View())

	return bar
}

// parseCmdLine splits "name args..." into name and args.
func parseCmdLine(raw string) (name, args string) {
	parts := strings.SplitN(raw, " ", 2)
	name = parts[0]
	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}
	return
}
