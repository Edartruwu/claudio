package prompt

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// SubmitMsg is sent when the user presses Enter to submit their input.
type SubmitMsg struct {
	Text string
}

// Model is the prompt input component.
type Model struct {
	textarea textarea.Model
	focused  bool
	width    int
	history  []string
	histIdx  int
}

// New creates a new prompt input model.
func New() Model {
	ta := textarea.New()
	ta.Placeholder = "Message Claudio..."
	ta.Focus()
	ta.CharLimit = 100000
	ta.MaxHeight = 10
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.Base = lipgloss.NewStyle()
	ta.BlurredStyle.Base = lipgloss.NewStyle()

	return Model{
		textarea: ta,
		focused:  true,
		histIdx:  -1,
	}
}

// SetWidth sets the prompt width.
func (m *Model) SetWidth(w int) {
	m.width = w
	m.textarea.SetWidth(w - 4) // account for border padding
}

// Focus gives focus to the prompt.
func (m *Model) Focus() {
	m.focused = true
	m.textarea.Focus()
}

// Blur removes focus from the prompt.
func (m *Model) Blur() {
	m.focused = false
	m.textarea.Blur()
}

// Reset clears the input.
func (m *Model) Reset() {
	m.textarea.Reset()
	m.histIdx = -1
}

// Value returns the current input text.
func (m *Model) Value() string {
	return m.textarea.Value()
}

// Update handles input events.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.focused {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			// Submit on Enter (without modifiers)
			text := strings.TrimSpace(m.textarea.Value())
			if text == "" {
				return m, nil
			}
			// Save to history
			m.history = append(m.history, text)
			m.histIdx = -1
			m.textarea.Reset()
			return m, func() tea.Msg {
				return SubmitMsg{Text: text}
			}

		case tea.KeyUp:
			// History navigation when cursor is on first line
			if m.textarea.Line() == 0 && len(m.history) > 0 {
				if m.histIdx == -1 {
					m.histIdx = len(m.history) - 1
				} else if m.histIdx > 0 {
					m.histIdx--
				}
				m.textarea.SetValue(m.history[m.histIdx])
				m.textarea.CursorEnd()
				return m, nil
			}

		case tea.KeyDown:
			// History navigation
			if m.textarea.Line() == m.textarea.LineCount()-1 && m.histIdx >= 0 {
				if m.histIdx < len(m.history)-1 {
					m.histIdx++
					m.textarea.SetValue(m.history[m.histIdx])
				} else {
					m.histIdx = -1
					m.textarea.Reset()
				}
				return m, nil
			}

		case tea.KeyCtrlC:
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
}

// View renders the prompt.
func (m Model) View() string {
	style := styles.PromptFocused
	if !m.focused {
		style = styles.PromptBlurred
	}

	return style.Width(m.width - 2).Render(m.textarea.View())
}
