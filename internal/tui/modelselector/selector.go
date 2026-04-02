package modelselector

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// ModelOption represents a selectable model.
type ModelOption struct {
	Label       string // Display name (e.g. "Default (recommended)")
	ID          string // Model identifier (e.g. "claude-sonnet-4-6")
	Description string // Short description
}

// DefaultModels returns the standard model options.
func DefaultModels() []ModelOption {
	return []ModelOption{
		{
			Label:       "Default (recommended)",
			ID:          "claude-sonnet-4-6",
			Description: "Sonnet 4.6 - Best for everyday tasks",
		},
		{
			Label:       "Opus",
			ID:          "claude-opus-4-6",
			Description: "Opus 4.6 with 1M context - Most capable for complex work",
		},
		{
			Label:       "Haiku",
			ID:          "claude-haiku-4-5-20251001",
			Description: "Haiku 4.5 - Fastest for quick answers",
		},
	}
}

// ModelSelectedMsg is sent when the user picks a model.
type ModelSelectedMsg struct {
	ModelID string
	Label   string
}

// DismissMsg is sent when the user cancels.
type DismissMsg struct{}

// Model is the model selector component.
type Model struct {
	options  []ModelOption
	selected int
	current  string // current model ID (shown with checkmark)
	active   bool
	width    int
}

// New creates a new model selector.
func New(currentModel string) Model {
	options := DefaultModels()
	selected := 0
	for i, o := range options {
		if o.ID == currentModel {
			selected = i
			break
		}
	}
	return Model{
		options:  options,
		selected: selected,
		current:  currentModel,
		active:   true,
	}
}

// IsActive returns whether the selector is visible.
func (m Model) IsActive() bool {
	return m.active
}

// SetWidth updates the display width.
func (m *Model) SetWidth(w int) {
	m.width = w
}

// Update handles key events.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.active {
		return m, nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch keyMsg.String() {
	case "up", "k":
		if m.selected > 0 {
			m.selected--
		}
	case "down", "j":
		if m.selected < len(m.options)-1 {
			m.selected++
		}
	case "1":
		m.selected = 0
	case "2":
		if len(m.options) > 1 {
			m.selected = 1
		}
	case "3":
		if len(m.options) > 2 {
			m.selected = 2
		}
	case "enter":
		m.active = false
		opt := m.options[m.selected]
		return m, func() tea.Msg {
			return ModelSelectedMsg{ModelID: opt.ID, Label: opt.Label}
		}
	case "esc", "q":
		m.active = false
		return m, func() tea.Msg {
			return DismissMsg{}
		}
	}

	return m, nil
}

// View renders the model selector dialog.
func (m Model) View() string {
	if !m.active {
		return ""
	}

	titleStyle := lipgloss.NewStyle().
		Foreground(styles.Text).
		Bold(true)

	subtitleStyle := lipgloss.NewStyle().
		Foreground(styles.Dim)

	hintStyle := lipgloss.NewStyle().
		Foreground(styles.Warning).
		Italic(true)

	var lines []string
	lines = append(lines, titleStyle.Render("Select model"))
	lines = append(lines, subtitleStyle.Render(
		"Switch between Claude models. Applies to this session and future sessions."))
	lines = append(lines, "")

	labelWidth := 28
	for _, opt := range m.options {
		if len(opt.Label)+4 > labelWidth {
			labelWidth = len(opt.Label) + 4
		}
	}

	for i, opt := range m.options {
		num := fmt.Sprintf("%d. ", i+1)
		label := opt.Label
		check := ""
		if opt.ID == m.current {
			check = " \u2714" // checkmark
		}

		var numStyle, nameStyle, descStyle lipgloss.Style

		if i == m.selected {
			numStyle = lipgloss.NewStyle().
				Foreground(styles.Primary).
				Bold(true)
			nameStyle = lipgloss.NewStyle().
				Foreground(styles.Text).
				Bold(true)
			descStyle = lipgloss.NewStyle().
				Foreground(styles.Dim)
		} else {
			numStyle = lipgloss.NewStyle().
				Foreground(styles.Dim)
			nameStyle = lipgloss.NewStyle().
				Foreground(styles.Dim)
			descStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#4B5563"))
		}

		prefix := "  "
		if i == m.selected {
			prefix = lipgloss.NewStyle().
				Foreground(styles.Primary).
				Bold(true).
				Render("\u203A ")
		}

		nameText := nameStyle.Render(label + check)
		labelPad := strings.Repeat(" ", max(1, labelWidth-lipgloss.Width(label+check)))

		line := prefix + numStyle.Render(num) + nameText + labelPad + descStyle.Render(opt.Description)
		lines = append(lines, line)
	}

	lines = append(lines, "")
	lines = append(lines, hintStyle.Render("Enter to confirm \u00B7 Esc to exit"))

	content := strings.Join(lines, "\n")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.Primary).
		Padding(1, 2).
		Width(min(m.width-4, 80)).
		Render(content)

	// Center horizontally
	return lipgloss.NewStyle().
		Width(m.width).
		Align(lipgloss.Center).
		Render(box)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
