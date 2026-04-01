package permissions

import (
	"encoding/json"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

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
		case "n":
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

	w := m.width
	if w == 0 {
		w = 60
	}

	// Title
	title := styles.PermissionTitle.Render(fmt.Sprintf("🔒 %s wants to use: %s", "Claudio", m.toolUse.Name))

	// Tool input summary
	inputSummary := formatToolInput(m.toolUse)

	// Options
	var opts string
	for i, opt := range m.options {
		style := styles.FooterPill
		if i == m.selected {
			if opt.decision == DecisionDeny {
				style = styles.PermissionDeny
			} else {
				style = styles.PermissionAllow
			}
		}
		if i > 0 {
			opts += "  "
		}
		opts += style.Render(opt.label)
	}

	hint := styles.SpinnerText.Render("  (y)es  (n)o  ←/→ select  enter confirm")

	content := title + "\n\n" + inputSummary + "\n\n" + opts + hint

	return styles.PermissionBox.Width(w - 4).Render(content)
}

func formatToolInput(tu tools.ToolUse) string {
	switch tu.Name {
	case "Bash":
		var in struct{ Command string }
		json.Unmarshal(tu.Input, &in)
		return styles.ToolInput.Render("$ " + in.Command)
	case "Write":
		var in struct {
			FilePath string
			Content  string
		}
		json.Unmarshal(tu.Input, &in)
		return styles.ToolInput.Render("→ " + in.FilePath)
	case "Edit":
		var in struct{ FilePath string }
		json.Unmarshal(tu.Input, &in)
		return styles.ToolInput.Render("✎ " + in.FilePath)
	default:
		s := string(tu.Input)
		if len(s) > 200 {
			s = s[:200] + "..."
		}
		return styles.ToolInput.Render(s)
	}
}
