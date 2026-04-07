package teamselector

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/teams"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// TeamSelectedMsg is sent when the user picks a template or creates an ephemeral team.
type TeamSelectedMsg struct {
	// TemplateName is non-empty when an existing template was chosen.
	TemplateName string
	// IsEphemeral is true when the user chose "new ephemeral team".
	IsEphemeral bool
	// Description is the template description (for context injection).
	Description string
	// Members summarises the roster for the system-prompt addendum.
	Members []teams.TeamTemplateMember
}

// DismissMsg is sent when the user cancels.
type DismissMsg struct{}

type entry struct {
	tmpl      *teams.TeamTemplate // nil for the ephemeral sentinel
	ephemeral bool
}

// Model is the team template picker component.
type Model struct {
	entries []entry
	cursor  int
	active  bool
	width   int
	filter  string // future: fuzzy filter
}

// New creates a new team selector loaded from templatesDir.
// Pass the path to ~/.claudio/team-templates/.
func New(templatesDir string) Model {
	mgr := teams.NewManager("", templatesDir)
	tmpls := mgr.ListTemplates()

	var entries []entry
	// ephemeral option always first
	entries = append(entries, entry{ephemeral: true})
	for i := range tmpls {
		t := tmpls[i]
		entries = append(entries, entry{tmpl: &t})
	}
	return Model{
		entries: entries,
		active:  true,
	}
}

func (m Model) IsActive() bool  { return m.active }
func (m *Model) SetWidth(w int) { m.width = w }

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
		}
	case "down", "j":
		if m.cursor < len(m.entries)-1 {
			m.cursor++
		}
	case "enter":
		m.active = false
		e := m.entries[m.cursor]
		if e.ephemeral {
			return m, func() tea.Msg {
				return TeamSelectedMsg{IsEphemeral: true}
			}
		}
		tmpl := e.tmpl
		return m, func() tea.Msg {
			return TeamSelectedMsg{
				TemplateName: tmpl.Name,
				Description:  tmpl.Description,
				Members:      tmpl.Members,
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

	titleStyle   := lipgloss.NewStyle().Foreground(styles.Text).Bold(true)
	sectionStyle := lipgloss.NewStyle().Foreground(styles.Primary).Bold(true)
	hintStyle    := lipgloss.NewStyle().Foreground(styles.Warning).Italic(true)
	dimStyle     := lipgloss.NewStyle().Foreground(styles.Dim)
	boldStyle    := lipgloss.NewStyle().Foreground(styles.Text).Bold(true)
	memberStyle  := lipgloss.NewStyle().Foreground(lipgloss.Color("#4B5563"))

	var lines []string
	lines = append(lines, titleStyle.Render("Select Team Template"))
	lines = append(lines, "")
	lines = append(lines, sectionStyle.Render("Available Teams"))

	for i, e := range m.entries {
		selected := i == m.cursor
		prefix := "  "
		if selected {
			prefix = lipgloss.NewStyle().Foreground(styles.Primary).Bold(true).Render("› ")
		}

		if e.ephemeral {
			nameS := dimStyle.Render("ephemeral")
			if selected {
				nameS = boldStyle.Render("ephemeral")
			}
			pad := strings.Repeat(" ", max(1, 28-len("ephemeral")))
			desc := memberStyle.Render("new unnamed team, no pre-registered members")
			lines = append(lines, prefix+nameS+pad+desc)
			continue
		}

		tmpl := e.tmpl
		nameS := dimStyle.Render(tmpl.Name)
		if selected {
			nameS = boldStyle.Render(tmpl.Name)
		}
		pad := strings.Repeat(" ", max(1, 28-len(tmpl.Name)))

		desc := tmpl.Description
		if desc == "" {
			desc = buildRosterSummary(tmpl.Members)
		}
		descS := memberStyle.Render(desc)

		lines = append(lines, prefix+nameS+pad+descS)

		// Show member roster under the selected entry
		if selected && len(tmpl.Members) > 0 {
			for _, mem := range tmpl.Members {
				mline := fmt.Sprintf("       • %s (%s)", mem.Name, mem.SubagentType)
				if mem.Model != "" {
					mline += " " + mem.Model
				}
				lines = append(lines, memberStyle.Render(mline))
			}
		}
	}

	lines = append(lines, "")
	lines = append(lines, hintStyle.Render("j/k navigate · Enter select · Esc cancel"))

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

func buildRosterSummary(members []teams.TeamTemplateMember) string {
	names := make([]string, 0, len(members))
	for _, m := range members {
		names = append(names, fmt.Sprintf("%s(%s)", m.Name, m.SubagentType))
	}
	return strings.Join(names, ", ")
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
