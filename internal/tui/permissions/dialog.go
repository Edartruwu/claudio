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
	DecisionAllowAllTool // allow this tool for any input (pattern: *)
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
			{label: fmt.Sprintf("Allow all %s", tu.Name), decision: DecisionAllowAllTool},
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
			case DecisionAllowAlways, DecisionAllowAllTool:
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

// InlineHeight returns the number of lines InlineView renders.
func (m Model) InlineHeight() int {
	return 4 // title + input + buttons + border
}

// InlineView renders the permission request as an inline dock (full-width, no centering).
// Line 1: warning icon + tool name + brief input summary (1 line)
// Line 2: buttons row
// Wrapped in a PermissionBox border applied at full width.
func (m Model) InlineView() string {
	if !m.active {
		return ""
	}

	w := m.width
	if w <= 0 {
		w = 80
	}

	// Line 1: ⚠  <ToolName> wants to run:  <input summary>
	iconPart := styles.PermissionTitle.Render("⚠  " + m.toolUse.Name + " wants to run:")
	inputOneLine := formatToolInputOneLine(m.toolUse)
	summaryPart := styles.ToolSummary.Render("  " + inputOneLine)
	titleLine := iconPart + summaryPart

	// Line 2: buttons + hint
	var buttons []string
	for i, opt := range m.options {
		var s lipgloss.Style
		if i == m.selected {
			switch opt.decision {
			case DecisionDeny:
				s = styles.ButtonDeny
			case DecisionAllowAlways, DecisionAllowAllTool:
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

	content := titleLine + "\n" + buttonRow

	// Apply box at full width (account for border padding: 2 sides * (1 border + 2 padding) = 6)
	boxW := w - 6
	if boxW < 10 {
		boxW = 10
	}
	return styles.PermissionBox.Width(boxW).Render(content)
}

// formatToolInputOneLine returns a single-line summary of the tool input.
func formatToolInputOneLine(tu tools.ToolUse) string {
	switch tu.Name {
	case "Bash":
		var in struct {
			Command string `json:"command"`
		}
		json.Unmarshal(tu.Input, &in)
		cmd := in.Command
		if len(cmd) > 60 {
			cmd = cmd[:59] + "…"
		}
		return fmt.Sprintf("$ %s", cmd)
	case "Write":
		var in struct {
			FilePath string `json:"file_path"`
		}
		json.Unmarshal(tu.Input, &in)
		return fmt.Sprintf("→ %s", in.FilePath)
	case "Edit":
		var in struct {
			FilePath string `json:"file_path"`
		}
		json.Unmarshal(tu.Input, &in)
		return fmt.Sprintf("✎ %s", in.FilePath)
	default:
		raw := string(tu.Input)
		if len(raw) > 60 {
			raw = raw[:59] + "…"
		}
		return raw
	}
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
