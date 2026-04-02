package permissions

import (
	"encoding/json"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/tools"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// Decision represents the user's permission choice.
type Decision int

const (
	DecisionPending Decision = iota
	DecisionAllow
	DecisionDeny
	DecisionAllowAlways
)

// ResponseMsg is sent when the user makes a permission decision.
type ResponseMsg struct {
	ToolUse  tools.ToolUse
	Decision Decision
}

// Model is the permission dialog component.
type Model struct {
	toolUse  tools.ToolUse
	selected int
	options  []option
	active   bool
	width    int
}

type option struct {
	label    string
	decision Decision
}

// New creates a new permission dialog for a tool use.
func New(tu tools.ToolUse) Model {
	return Model{
		toolUse: tu,
		active:  true,
		options: []option{
			{label: "Allow", decision: DecisionAllow},
			{label: "Deny", decision: DecisionDeny},
			{label: "Always allow", decision: DecisionAllowAlways},
		},
	}
}

// IsActive returns whether the dialog is showing.
func (m *Model) IsActive() bool {
	return m.active
}

// SetWidth sets the dialog width.
func (m *Model) SetWidth(w int) {
	m.width = w
}

// Update handles input for the permission dialog.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.active {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "left", "h":
			if m.selected > 0 {
				m.selected--
			}
		case "right", "l":
			if m.selected < len(m.options)-1 {
				m.selected++
			}
		case "enter":
			m.active = false
			return m, func() tea.Msg {
				return ResponseMsg{
					ToolUse:  m.toolUse,
					Decision: m.options[m.selected].decision,
				}
			}
		case "y":
			m.active = false
			return m, func() tea.Msg {
				return ResponseMsg{
					ToolUse:  m.toolUse,
					Decision: DecisionAllow,
				}
			}
		case "n", "esc", "ctrl+c":
			m.active = false
			return m, func() tea.Msg {
				return ResponseMsg{
					ToolUse:  m.toolUse,
					Decision: DecisionDeny,
				}
			}
		}
	}

	return m, nil
}

// View renders the permission dialog.
func (m Model) View() string {
	if !m.active {
		return ""
	}

	boxW := m.width - 8
	if boxW > 72 {
		boxW = 72
	}

	// Title
	title := styles.PermissionTitle.Render("🔒  Claudio wants to run:")

	// Tool input summary
	inputSummary := formatToolInput(m.toolUse)

	// Buttons
	var buttons []string
	for i, opt := range m.options {
		var s lipgloss.Style
		if i == m.selected {
			switch opt.decision {
			case DecisionDeny:
				s = styles.ButtonDeny
			case DecisionAllowAlways:
				s = styles.ButtonAllowAlways
			default:
				s = styles.ButtonAllow
			}
		} else {
			s = styles.ButtonInactive
		}
		buttons = append(buttons, s.Render(opt.label))
	}

	hint := styles.StatusHint.Render("  y/n · ←→ · enter")
	buttonRow := lipgloss.JoinHorizontal(lipgloss.Center, buttons...) + hint

	content := title + "\n\n" + inputSummary + "\n\n" + buttonRow

	box := styles.PermissionBox.Width(boxW).Render(content)

	// Center horizontally
	return lipgloss.NewStyle().
		Width(m.width).
		Align(lipgloss.Center).
		Render(box)
}

func formatToolInput(tu tools.ToolUse) string {
	s := styles.ToolSummary
	switch tu.Name {
	case "Bash":
		var in struct {
			Command string `json:"command"`
		}
		json.Unmarshal(tu.Input, &in)
		return s.Render(fmt.Sprintf("$ %s", in.Command))
	case "Write":
		var in struct {
			FilePath string `json:"file_path"`
			Content  string `json:"content"`
		}
		json.Unmarshal(tu.Input, &in)
		preview := in.Content
		if len(preview) > 200 {
			preview = preview[:200] + "…"
		}
		return s.Render(fmt.Sprintf("→  %s\n\n%s", in.FilePath, preview))
	case "Edit":
		var in struct {
			FilePath  string `json:"file_path"`
			OldString string `json:"old_string"`
			NewString string `json:"new_string"`
		}
		json.Unmarshal(tu.Input, &in)
		detail := fmt.Sprintf("✎  %s", in.FilePath)
		if in.OldString != "" {
			old := in.OldString
			if len(old) > 100 {
				old = old[:100] + "…"
			}
			new := in.NewString
			if len(new) > 100 {
				new = new[:100] + "…"
			}
			detail += fmt.Sprintf("\n  - %s\n  + %s", old, new)
		}
		return s.Render(detail)
	default:
		raw := string(tu.Input)
		if len(raw) > 200 {
			raw = raw[:200] + "…"
		}
		return s.Render(raw)
	}
}
