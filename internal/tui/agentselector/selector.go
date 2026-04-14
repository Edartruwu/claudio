package agentselector

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/agents"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// AgentSelectedMsg is sent when the user picks an agent.
type AgentSelectedMsg struct {
	AgentType       string
	DisplayName     string
	SystemPrompt    string
	Model           string
	DisallowedTools []string
}

// DismissMsg is sent when the user cancels.
type DismissMsg struct{}

// Model is the agent picker component.
type Model struct {
	agents          []agents.AgentDefinition
	filter          textinput.Model
	cursor          int
	offset          int // scroll offset for the list
	active          bool
	width           int
	height          int
	current         string // currently active agent type
	confirmingReset bool   // true when user must press Enter again to confirm noneAgent
}

// noneAgent is a sentinel entry that clears the active persona.
var noneAgent = agents.AgentDefinition{
	Type:      "",
	WhenToUse: "(none — reset persona)",
}

// New creates a new agent selector populated with all available agents.
func New(current string, customDirs ...string) Model {
	all := append([]agents.AgentDefinition{noneAgent}, agents.AllAgents(customDirs...)...)

	ti := textinput.New()
	ti.Placeholder = "type to filter..."
	ti.Prompt = "> "
	ti.Focus()

	return Model{
		agents:  all,
		filter:  ti,
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

// filtered returns agents matching the current filter query.
func (m Model) filtered() []agents.AgentDefinition {
	q := strings.ToLower(m.filter.Value())
	if q == "" {
		return m.agents
	}
	var out []agents.AgentDefinition
	for _, a := range m.agents {
		name := strings.ToLower(a.Type)
		when := strings.ToLower(a.WhenToUse)
		if strings.Contains(name, q) || strings.Contains(when, q) {
			out = append(out, a)
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
	// listH mirrors the calculation in View() so offset adjustments are consistent.
	listH := max(min(m.height-4, 30)-4, 6)

	switch key.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			if m.cursor < m.offset {
				m.offset = m.cursor
			}
		}
	case "down", "j":
		items := m.filtered()
		if m.cursor < len(items)-1 {
			m.cursor++
			if m.cursor >= m.offset+listH {
				m.offset = m.cursor - listH + 1
			}
		}
	case "enter":
		items := m.filtered()
		if len(items) == 0 {
			m.active = false
			return m, func() tea.Msg { return DismissMsg{} }
		}
		cursor := m.cursor
		if cursor >= len(items) {
			cursor = len(items) - 1
		}
		sel := items[cursor]
		// noneAgent (Type == "") is destructive — require an explicit second Enter.
		if sel.Type == "" {
			if m.confirmingReset {
				// Second Enter: user confirmed — emit the reset.
				m.active = false
				m.confirmingReset = false
				return m, func() tea.Msg {
					return AgentSelectedMsg{
						AgentType:       sel.Type,
						DisplayName:     sel.WhenToUse,
						SystemPrompt:    sel.SystemPrompt,
						Model:           sel.Model,
						DisallowedTools: sel.DisallowedTools,
					}
				}
			}
			// First Enter on noneAgent: enter confirmation state, don't close.
			m.confirmingReset = true
			return m, nil
		}
		// Normal agent selected.
		m.active = false
		m.confirmingReset = false
		return m, func() tea.Msg {
			return AgentSelectedMsg{
				AgentType:       sel.Type,
				DisplayName:     sel.WhenToUse,
				SystemPrompt:    sel.SystemPrompt,
				Model:           sel.Model,
				DisallowedTools: sel.DisallowedTools,
			}
		}
	case "esc":
		if m.confirmingReset {
			// Cancel confirmation — return to normal selector state.
			m.confirmingReset = false
			return m, nil
		}
		m.active = false
		return m, func() tea.Msg { return DismissMsg{} }
	default:
		// Pass to textinput for filtering
		prevVal := m.filter.Value()
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		// Clamp cursor and offset if filter narrowed the list
		if m.filter.Value() != prevVal {
			// Any filter change clears a pending reset confirmation.
			m.confirmingReset = false
			items := m.filtered()
			if m.cursor >= len(items) && len(items) > 0 {
				m.cursor = len(items) - 1
			}
			if len(items) == 0 {
				m.cursor = 0
			}
			// If the cursor landed on noneAgent (index 0) but real agents exist,
			// move it to the first real agent (index 1) to avoid accidental reset.
			if m.cursor == 0 && len(items) > 1 && items[0].Type == "" {
				m.cursor = 1
			}
			// Clamp offset so we don't scroll past the end of the filtered list
			maxOffset := max(len(items)-listH, 0)
			if m.offset > maxOffset {
				m.offset = maxOffset
			}
			if m.cursor < m.offset {
				m.offset = m.cursor
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
	totalH := min(m.height-4, 30)
	if totalH < 10 {
		totalH = 10
	}

	// innerW is the content area inside the border (border takes 1 char each side)
	innerW := totalW - 2
	innerH := totalH - 2 // rows inside the border

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
	// Use stateful offset but clamp it defensively in case height changed
	offset := m.offset
	maxOffset := max(len(items)-listH, 0)
	if offset > maxOffset {
		offset = maxOffset
	}
	if offset < 0 {
		offset = 0
	}
	end := min(offset+listH, len(items))

	var leftLines []string
	leftLines = append(leftLines, m.filter.View())
	leftLines = append(leftLines, "") // blank

	selectedRowStyle := lipgloss.NewStyle().
		Background(styles.Subtle).
		Width(leftW)

	for i := offset; i < end; i++ {
		a := items[i]
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

		name := a.Type
		if name == "" {
			name = "(none — reset persona)"
		}
		line := prefix + nameStyle.Render(name)
		if selected {
			line = selectedRowStyle.Render(line)
		}
		leftLines = append(leftLines, line)
	}

	// Position counter — only show when list is scrollable
	if len(items) > listH {
		counter := lipgloss.NewStyle().Foreground(styles.Dim).
			Render("  " + fmt.Sprintf("%d / %d", cursor+1, len(items)))
		leftLines = append(leftLines, counter)
	}

	// --- right pane lines ---
	var rightLines []string

	titleStyle := lipgloss.NewStyle().Foreground(styles.Text).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(styles.Primary)
	valueStyle := lipgloss.NewStyle().Foreground(styles.Dim)

	if len(items) > 0 {
		sel := items[cursor]

		selName := sel.Type
		if selName == "" {
			selName = "(none)"
		}
		rightLines = append(rightLines, titleStyle.Render(selName))
		rightLines = append(rightLines, "")

		// Model override
		modelVal := sel.Model
		if modelVal == "" {
			modelVal = "default"
		}
		rightLines = append(rightLines, labelStyle.Render("Model: ")+valueStyle.Render(modelVal))
		rightLines = append(rightLines, "")

		// Description / WhenToUse (word-wrapped)
		desc := sel.WhenToUse
		if desc == "(none — reset persona)" {
			desc = "Clears the active agent persona and restores default Claudio behaviour."
		}
		if desc == "" {
			desc = "No description available."
		}
		for _, l := range wordWrap(desc, rightW-1) {
			rightLines = append(rightLines, valueStyle.Render(l))
		}
		rightLines = append(rightLines, "")

		// Disallowed tools
		toolsVal := "none"
		if len(sel.DisallowedTools) > 0 {
			toolsVal = strings.Join(sel.DisallowedTools, ", ")
		}
		rightLines = append(rightLines, labelStyle.Render("Restricted tools:"))
		for _, l := range wordWrap(toolsVal, rightW-1) {
			rightLines = append(rightLines, valueStyle.Render(l))
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

	var hintText string
	if m.confirmingReset {
		hintText = "  Reset to default agent? Press Enter to confirm · Esc to cancel"
	} else {
		hintText = "  type to filter · j/k navigate · enter select · esc cancel"
	}
	hint := lipgloss.NewStyle().Foreground(styles.Warning).Italic(true).Render(hintText)

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
