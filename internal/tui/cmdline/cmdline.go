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

	// popup/wildmenu state for argument completion
	popup       []string
	popupIdx    int
	popupActive bool
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
	m.popup = nil
	m.popupIdx = 0
	m.popupActive = false
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
		if m.popupActive {
			m.popup = nil
			m.popupActive = false
			return m, nil
		}
		m.Deactivate()
		return m, func() tea.Msg { return CancelMsg{} }

	case tea.KeyEnter:
		if m.popupActive && len(m.popup) > 0 {
			name, _ := parseCmdLine(m.input.Value())
			selected := m.popup[m.popupIdx]
			raw := name + " " + selected
			m.input.SetValue(raw)
			m.popup = nil
			m.popupActive = false
			m.Deactivate()
			if len(m.history) == 0 || m.history[len(m.history)-1] != raw {
				m.history = append(m.history, raw)
			}
			cmdName, args := parseCmdLine(raw)
			return m, func() tea.Msg {
				return ExecuteMsg{Raw: raw, Name: cmdName, Args: args}
			}
		}
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
		if m.popupActive && len(m.popup) > 0 {
			m.popupIdx--
			if m.popupIdx < 0 {
				m.popupIdx = len(m.popup) - 1
			}
			return m, nil
		}
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
		if m.popupActive && len(m.popup) > 0 {
			m.popupIdx = (m.popupIdx + 1) % len(m.popup)
			return m, nil
		}
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
		if strings.Contains(m.input.Value(), " ") {
			m.argComplete()
		} else {
			m.autocomplete()
		}
		return m, nil

	case tea.KeySpace:
		// Reset autocomplete and popup state when user types a space (starts arguments).
		m.suggestions = nil
		m.suggIdx = 0
		m.popup = nil
		m.popupIdx = 0
		m.popupActive = false
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
		// rebuild popup after space
		m.argComplete()
		return m, cmd
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	// Rebuild arg popup after any character change
	if strings.Contains(m.input.Value(), " ") {
		m.argComplete()
	} else {
		m.suggestions = nil
		m.suggIdx = 0
		m.popup = nil
		m.popupActive = false
	}
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

// argComplete builds the argument popup for commands that support it.
func (m *Model) argComplete() {
	if m.registry == nil {
		return
	}
	val := m.input.Value()
	cmdName, argPrefix := parseCmdLine(val)
	if cmdName == "" {
		m.popup = nil
		m.popupActive = false
		return
	}
	cmd, ok := m.registry.Get(cmdName)
	if !ok || cmd.ArgCompleter == nil {
		m.popup = nil
		m.popupActive = false
		return
	}
	completions := cmd.ArgCompleter(argPrefix)
	if len(completions) == 0 {
		m.popup = nil
		m.popupActive = false
		return
	}
	m.popup = completions
	m.popupActive = true
	if m.popupIdx >= len(m.popup) {
		m.popupIdx = 0
	}
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

	if !m.popupActive || len(m.popup) == 0 {
		return bar
	}

	// Render popup above the bar (max 12 items visible)
	maxVisible := 12
	start := 0
	if m.popupIdx >= maxVisible {
		start = m.popupIdx - maxVisible + 1
	}
	end := start + maxVisible
	if end > len(m.popup) {
		end = len(m.popup)
	}

	itemWidth := m.width - 2
	var rows []string
	for i := start; i < end; i++ {
		item := m.popup[i]
		style := lipgloss.NewStyle().
			Width(itemWidth).
			Padding(0, 1)
		if i == m.popupIdx {
			style = style.Background(styles.Primary).Foreground(styles.Surface).Bold(true)
		} else {
			style = style.Background(styles.SurfaceAlt).Foreground(styles.Text)
		}
		rows = append(rows, style.Render(item))
	}

	popup := strings.Join(rows, "\n")
	return popup + "\n" + bar
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
