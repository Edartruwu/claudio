package teamselector

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
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
	filter  textinput.Model
	cursor  int
	active  bool
	width   int
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

	ti := textinput.New()
	ti.Placeholder = "type to filter..."
	ti.Prompt = "> "
	ti.Focus()

	return Model{
		entries: entries,
		filter:  ti,
		active:  true,
	}
}

func (m Model) IsActive() bool  { return m.active }
func (m *Model) SetWidth(w int) { m.width = w }

// entryName returns the display name for a list entry.
func entryName(e entry) string {
	if e.ephemeral {
		return "ephemeral"
	}
	return e.tmpl.Name
}

// filtered returns entries matching the current filter query.
func (m Model) filtered() []entry {
	q := strings.ToLower(m.filter.Value())
	if q == "" {
		return m.entries
	}
	var out []entry
	for _, e := range m.entries {
		name := strings.ToLower(entryName(e))
		if strings.Contains(name, q) {
			out = append(out, e)
		}
	}
	return out
}

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
		items := m.filtered()
		if m.cursor < len(items)-1 {
			m.cursor++
		}
	case "enter":
		m.active = false
		items := m.filtered()
		if len(items) == 0 {
			return m, func() tea.Msg { return DismissMsg{} }
		}
		cursor := m.cursor
		if cursor >= len(items) {
			cursor = len(items) - 1
		}
		e := items[cursor]
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
	case "esc":
		m.active = false
		return m, func() tea.Msg { return DismissMsg{} }
	default:
		// Pass to textinput for filtering
		prevVal := m.filter.Value()
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		// Clamp cursor if filter narrowed the list
		if m.filter.Value() != prevVal {
			items := m.filtered()
			if m.cursor >= len(items) && len(items) > 0 {
				m.cursor = len(items) - 1
			}
			if len(items) == 0 {
				m.cursor = 0
			}
		}
		return m, cmd
	}
	return m, nil
}

func (m Model) View() string {
	if !m.active {
		return ""
	}

	// --- dimensions ---
	totalW := min(m.width-4, 90)
	if totalW < 44 {
		totalW = 44
	}

	// inner content area inside the border
	innerW := totalW - 2
	innerH := 18 // fixed height for the overlay content

	leftW := innerW * 2 / 5
	rightW := innerW - leftW - 1 // 1 char for │ separator

	// --- set textinput width ---
	m.filter.Width = leftW - len(m.filter.Prompt) - 1
	if m.filter.Width < 4 {
		m.filter.Width = 4
	}

	items := m.filtered()

	// clamp cursor
	cursor := m.cursor
	if len(items) == 0 {
		cursor = 0
	} else if cursor >= len(items) {
		cursor = len(items) - 1
	}

	// --- left pane lines ---
	// Row 0: filter input; Row 1: blank; Rows 2+: list items
	listH := innerH - 2
	offset := 0
	if cursor >= listH {
		offset = cursor - listH + 1
	}
	end := min(offset+listH, len(items))

	var leftLines []string
	leftLines = append(leftLines, m.filter.View())
	leftLines = append(leftLines, "") // blank

	for i := offset; i < end; i++ {
		e := items[i]
		selected := i == cursor

		var prefix string
		var nameStyle lipgloss.Style
		if selected {
			prefix = lipgloss.NewStyle().Foreground(styles.Primary).Bold(true).Render("▶ ")
			nameStyle = lipgloss.NewStyle().Foreground(styles.Text).Bold(true)
		} else {
			prefix = "  "
			nameStyle = lipgloss.NewStyle().Foreground(styles.Dim)
		}

		name := entryName(e)
		if e.ephemeral {
			name = "ephemeral (ad-hoc)"
		}
		leftLines = append(leftLines, prefix+nameStyle.Render(name))
	}

	// --- right pane lines ---
	var rightLines []string

	titleStyle := lipgloss.NewStyle().Foreground(styles.Text).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(styles.Primary)
	valueStyle := lipgloss.NewStyle().Foreground(styles.Dim)
	memberKeyStyle := lipgloss.NewStyle().Foreground(styles.Text)

	if len(items) > 0 {
		e := items[cursor]

		if e.ephemeral {
			rightLines = append(rightLines, titleStyle.Render("ephemeral"))
			rightLines = append(rightLines, "")
			rightLines = append(rightLines, valueStyle.Render("No pre-registered members."))
			rightLines = append(rightLines, valueStyle.Render("Spawn agents on-demand."))
		} else {
			tmpl := e.tmpl
			rightLines = append(rightLines, titleStyle.Render(tmpl.Name))
			rightLines = append(rightLines, "")

			if tmpl.Description != "" {
				for _, l := range wordWrap(tmpl.Description, rightW-1) {
					rightLines = append(rightLines, valueStyle.Render(l))
				}
				rightLines = append(rightLines, "")
			}

			if len(tmpl.Members) > 0 {
				rightLines = append(rightLines, labelStyle.Render("Members:"))
				for _, mem := range tmpl.Members {
					name := fmt.Sprintf("  %-12s", mem.Name)
					subtype := mem.SubagentType
					line := memberKeyStyle.Render(name) + valueStyle.Render(subtype)
					rightLines = append(rightLines, line)
				}
			} else {
				rightLines = append(rightLines, valueStyle.Render("No members defined."))
			}
		}
	}

	// --- pad both panes to innerH rows ---
	for len(leftLines) < innerH {
		leftLines = append(leftLines, "")
	}
	for len(rightLines) < innerH {
		rightLines = append(rightLines, "")
	}

	// --- build rows ---
	leftStyle := lipgloss.NewStyle().Width(leftW).MaxWidth(leftW)
	rightStyle := lipgloss.NewStyle().Width(rightW).MaxWidth(rightW)
	sepStyle := lipgloss.NewStyle().Foreground(styles.Dim)

	var rows []string
	for i := 0; i < innerH; i++ {
		l := leftStyle.Render(leftLines[i])
		r := rightStyle.Render(rightLines[i])
		sep := sepStyle.Render("│")
		rows = append(rows, l+sep+r)
	}

	body := strings.Join(rows, "\n")

	// --- outer box ---
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.Primary).
		Width(innerW).
		Render(body)

	hint := lipgloss.NewStyle().Foreground(styles.Warning).Italic(true).
		Render("  type to filter · j/k navigate · enter select · esc cancel")

	full := lipgloss.JoinVertical(lipgloss.Left, box, hint)

	return lipgloss.NewStyle().
		Width(m.width).
		Align(lipgloss.Center).
		Render(full)
}

// wordWrap breaks s into lines of at most maxW runes.
func wordWrap(s string, maxW int) []string {
	if maxW <= 0 {
		return []string{s}
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	cur := ""
	for _, w := range words {
		if cur == "" {
			cur = w
		} else if len(cur)+1+len(w) <= maxW {
			cur += " " + w
		} else {
			lines = append(lines, cur)
			cur = w
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
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
