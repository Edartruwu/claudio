package agentselector

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/agents"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// AgentSelectedMsg is sent when the user picks an agent.
type AgentSelectedMsg struct {
	AgentType    string
	DisplayName  string
	SystemPrompt string
	Model        string
	DisallowedTools []string
}

// DismissMsg is sent when the user cancels.
type DismissMsg struct{}

// Model is the agent picker component.
type Model struct {
	agents  []agents.AgentDefinition
	cursor  int
	active  bool
	width   int
	height  int
	offset  int // first visible agent index
	current string // currently active agent type
}

// noneAgent is a sentinel entry that clears the active persona.
var noneAgent = agents.AgentDefinition{
	Type:      "",
	WhenToUse: "Default Claudio (no persona)",
}

// New creates a new agent selector populated with all available agents.
func New(current string, customDirs ...string) Model {
	all := append([]agents.AgentDefinition{noneAgent}, agents.AllAgents(customDirs...)...)
	return Model{
		agents:  all,
		cursor:  indexFor(all, current),
		active:  true,
		current: current,
	}
}

func indexFor(all []agents.AgentDefinition, current string) int {
	for i, a := range all {
		if a.Type == current {
			return i
		}
	}
	return 0
}

func (m Model) IsActive() bool   { return m.active }
func (m *Model) SetWidth(w int)  { m.width = w }
func (m *Model) SetHeight(h int) { m.height = h }

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.active {
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			if m.cursor < m.offset {
				m.offset = m.cursor
			}
		}
	case "down", "j":
		if m.cursor < len(m.agents)-1 {
			m.cursor++
			visible := m.visibleRows()
			if m.cursor >= m.offset+visible {
				m.offset = m.cursor - visible + 1
			}
		}
	case "enter":
		m.active = false
		sel := m.agents[m.cursor]
		return m, func() tea.Msg {
			return AgentSelectedMsg{
				AgentType:       sel.Type,
				DisplayName:     sel.WhenToUse,
				SystemPrompt:    sel.SystemPrompt,
				Model:           sel.Model,
				DisallowedTools: sel.DisallowedTools,
			}
		}
	case "esc", "q":
		m.active = false
		return m, func() tea.Msg { return DismissMsg{} }
	}
	return m, nil
}

func (m Model) View() string {
	if !m.active {
		return ""
	}

	titleStyle := lipgloss.NewStyle().Foreground(styles.Text).Bold(true)
	sectionTitle := lipgloss.NewStyle().Foreground(styles.Primary).Bold(true)
	hintStyle := lipgloss.NewStyle().Foreground(styles.Warning).Italic(true)

	var lines []string
	lines = append(lines, titleStyle.Render("Switch Agent Persona"))
	lines = append(lines, "")
	lines = append(lines, sectionTitle.Render("Available Agents"))

	visible := m.visibleRows()
	end := min(m.offset+visible, len(m.agents))
	showUp := m.offset > 0
	showDown := end < len(m.agents)

	if showUp {
		lines = append(lines, hintStyle.Render("  \u2191 more above"))
	}

	for i := m.offset; i < end; i++ {
		a := m.agents[i]
		selected := i == m.cursor
		isCurrent := a.Type == m.current

		check := ""
		if isCurrent {
			check = " \u2714"
		}

		prefix := "  "
		if selected {
			prefix = lipgloss.NewStyle().Foreground(styles.Primary).Bold(true).Render("\u203A ")
		}

		var nameStyle, descStyle lipgloss.Style
		if selected {
			nameStyle = lipgloss.NewStyle().Foreground(styles.Text).Bold(true)
			descStyle = lipgloss.NewStyle().Foreground(styles.Dim)
		} else {
			nameStyle = lipgloss.NewStyle().Foreground(styles.Dim)
			descStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#4B5563"))
		}

		name := a.Type + check
		label := prefix + nameStyle.Render(name)
		if a.WhenToUse != "" {
			pad := strings.Repeat(" ", max(1, 28-lipgloss.Width(name)))
			label += pad + descStyle.Render(a.WhenToUse)
		}
		lines = append(lines, label)
	}

	if showDown {
		lines = append(lines, hintStyle.Render("  \u2193 more below"))
	}

	lines = append(lines, "")
	lines = append(lines, hintStyle.Render("j/k navigate \u00B7 Enter select \u00B7 Esc cancel"))

	content := strings.Join(lines, "\n")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.Primary).
		Padding(1, 2).
		Width(min(m.width-4, 85)).
		Render(content)

	return lipgloss.NewStyle().
		Width(m.width).
		Align(lipgloss.Center).
		Render(box)
}

// visibleRows returns how many agent rows fit inside the box.
// The box has 2 border lines + 2 padding lines + 3 header lines (title, blank, section) + 2 footer lines (blank, hint) = 9 overhead lines.
func (m Model) visibleRows() int {
	if m.height <= 0 {
		return len(m.agents) // no height set, show all
	}
	const overhead = 9
	v := m.height - overhead
	if v < 1 {
		v = 1
	}
	return v
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
