// Package agui provides the AGUI two-pane agent inspector panel.
// Left pane shows all agents grouped by team with status icons.
// Right pane shows the selected agent's full conversation history.
// Opens with <Space>oa or the :AGUI command.
package agui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/teams"
	"github.com/Abraxas-365/claudio/internal/tui/panels"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// spinner frames for running agents.
var spinFrames = []string{"◐", "◓", "●", "◑"}

// RefreshMsg triggers a panel refresh (auto-refresh for running agents).
type RefreshMsg struct{}

// ScheduleRefresh returns a cmd to fire a refresh after 500 ms.
func ScheduleRefresh() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
		return RefreshMsg{}
	})
}

// Panel is the AGUI two-pane agent inspector implementing panels.Panel.
type Panel struct {
	runner *teams.TeammateRunner

	entries    []*agentEntry  // flat list of all agents, populated on refresh
	cursor     int            // left-pane cursor index
	rightVP    viewport.Model // right-pane scrollable conversation
	focusRight bool           // true when right pane has keyboard focus
	selectedID string         // agentID of the currently viewed agent

	width  int
	height int
	active bool
	ready  bool // true after SetSize has been called
	tick   int  // spinner animation counter
}

// agentEntry is a cached snapshot of one agent's state for list rendering.
type agentEntry struct {
	id     string
	name   string
	team   string // team name, empty for ad-hoc agents
	status teams.MemberStatus
	state  *teams.TeammateState
}

// New creates a new AGUI panel with the given teammate runner.
func New(runner *teams.TeammateRunner) *Panel {
	return &Panel{runner: runner}
}

// ── panels.Panel interface ────────────────────────────────────────────────────

// IsActive returns true when the panel is visible.
func (p *Panel) IsActive() bool { return p.active }

// Activate makes the panel visible and loads initial data.
func (p *Panel) Activate() {
	p.active = true
	p.cursor = 0
	p.focusRight = false
	p.selectedID = ""
	p.refresh()
}

// Deactivate hides the panel.
func (p *Panel) Deactivate() {
	p.active = false
}

// SetSize sets the available width and height for the panel.
func (p *Panel) SetSize(w, h int) {
	p.width = w
	p.height = h
	rw := p.rightPaneWidth()
	rh := p.contentHeight()
	p.rightVP.Width = rw
	p.rightVP.Height = rh
	p.ready = true
	// Reload content in case we already had an agent selected.
	if p.selectedID != "" {
		p.loadConversation()
	}
}

// Update handles a key event and returns a command plus whether the key was consumed.
func (p *Panel) Update(msg tea.KeyMsg) (tea.Cmd, bool) {
	if !p.active {
		return nil, false
	}
	key := msg.String()
	if p.focusRight {
		return p.updateRight(key)
	}
	return p.updateLeft(key)
}

// View renders the two-pane layout.
func (p *Panel) View() string {
	if !p.active || !p.ready {
		return ""
	}
	leftStr := p.renderLeft()
	rightStr := p.renderRight()
	body := lipgloss.JoinHorizontal(lipgloss.Top, leftStr, rightStr)
	return lipgloss.NewStyle().Width(p.width).Height(p.height).Render(body)
}

// Help returns the keybinding hint line shown in the panel footer.
func (p *Panel) Help() string {
	if p.focusRight {
		return "j/k scroll · ctrl+d/u half-page · G bottom · g top · h/esc back"
	}
	return "j/k navigate · enter/l open · d cancel · r refresh · esc close"
}

// ── HandleRefresh ─────────────────────────────────────────────────────────────

// HandleRefresh is called when a RefreshMsg arrives.
// It increments the spinner tick, refreshes agent state, and reschedules
// a tick if any agents are still running.
func (p *Panel) HandleRefresh() tea.Cmd {
	p.tick++
	p.refresh()
	if p.hasRunning() {
		return ScheduleRefresh()
	}
	return nil
}

// ── private helpers ───────────────────────────────────────────────────────────

func (p *Panel) leftPaneWidth() int {
	if p.width <= 0 {
		return 35
	}
	lw := p.width * 35 / 100
	if lw < 20 {
		lw = 20
	}
	return lw
}

func (p *Panel) rightPaneWidth() int {
	lw := p.leftPaneWidth()
	rw := p.width - lw
	if rw < 10 {
		rw = 10
	}
	return rw
}

// contentHeight is the viewport height: total height minus title + hint rows.
func (p *Panel) contentHeight() int {
	h := p.height - 3 // title (1) + separator (1) + hint (1)
	if h < 2 {
		h = 2
	}
	return h
}

func (p *Panel) refresh() {
	if p.runner == nil {
		p.entries = nil
		return
	}
	states := p.runner.AllStates()
	entries := make([]*agentEntry, 0, len(states))
	for _, s := range states {
		e := &agentEntry{
			id:     s.Identity.AgentID,
			name:   s.Identity.AgentName,
			team:   s.TeamName,
			status: s.Status,
			state:  s,
		}
		entries = append(entries, e)
	}
	p.entries = entries
	p.clampCursor()

	// Reload the right pane if an agent is already selected.
	if p.selectedID != "" && p.ready {
		p.loadConversation()
	}
}

func (p *Panel) clampCursor() {
	if len(p.entries) == 0 {
		p.cursor = 0
		return
	}
	if p.cursor >= len(p.entries) {
		p.cursor = len(p.entries) - 1
	}
}

func (p *Panel) hasRunning() bool {
	for _, e := range p.entries {
		if e.status == teams.StatusWorking {
			return true
		}
	}
	return false
}

func (p *Panel) loadConversation() {
	if p.runner == nil || p.selectedID == "" || !p.ready {
		return
	}
	state, ok := p.runner.GetState(p.selectedID)
	if !ok {
		p.rightVP.SetContent(styles.PanelHint.Render("  Agent not found."))
		return
	}
	content := p.formatConversation(state)
	p.rightVP.SetContent(content)
}

func (p *Panel) formatConversation(state *teams.TeammateState) string {
	entries := state.GetConversation()
	rw := p.rightPaneWidth() - 4 // leave some padding
	if rw < 10 {
		rw = 10
	}

	if len(entries) == 0 {
		if state.Status == teams.StatusWorking {
			spinner := spinFrames[p.tick%len(spinFrames)]
			return "\n" + lipgloss.NewStyle().Foreground(styles.Warning).Render("  "+spinner+" waiting for first output…") + "\n"
		}
		return styles.PanelHint.Render("  No conversation yet.")
	}

	var b strings.Builder

	for _, e := range entries {
		switch e.Type {
		case "message_in":
			// Incoming message (user/lead → this agent).
			prefix := styles.UserPrefix.Render("> ")
			content := truncateLines(e.Content, rw, 6)
			b.WriteString(prefix + content + "\n\n")

		case "text":
			// Assistant text output.
			prefix := styles.AssistantPrefix.Render("< ")
			content := truncateLines(e.Content, rw, 8)
			b.WriteString(prefix + content + "\n\n")

		case "message_out":
			// Message sent out to another agent or back to lead.
			prefix := lipgloss.NewStyle().Foreground(styles.Aqua).Render("↗ ")
			content := truncateLines(e.Content, rw, 4)
			b.WriteString(prefix + content + "\n\n")

		case "tool_start":
			toolLine := styles.ToolName.Render("  ⚙ "+e.ToolName) + "\n"
			b.WriteString(toolLine)
			if e.Content != "" {
				preview := e.Content
				if len(preview) > 200 {
					preview = preview[:197] + "…"
				}
				b.WriteString(styles.ToolSummary.Render("    "+preview) + "\n")
			}
			b.WriteString("\n")

		case "tool_end":
			if e.Content != "" {
				preview := e.Content
				if len(preview) > 200 {
					preview = preview[:197] + "…"
				}
				b.WriteString(
					lipgloss.NewStyle().Foreground(styles.Success).Render("  ✓ ") +
						styles.ToolSummary.Render(preview) + "\n\n")
			}

		case "complete":
			result := e.Content
			if result == "" {
				result = "done"
			}
			b.WriteString(lipgloss.NewStyle().Foreground(styles.Success).Render("  ✓ complete") + "\n")
			b.WriteString(styles.ToolSummary.Render("  "+truncateLines(result, rw, 4)) + "\n\n")

		case "error":
			b.WriteString(styles.ToolError.Render("  ✗ "+e.Content) + "\n\n")
		}
	}

	// Streaming indicator for running agents.
	if state.Status == teams.StatusWorking {
		spinner := spinFrames[p.tick%len(spinFrames)]
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(styles.Warning).Render("  "+spinner+" streaming") + "\n")
	}

	return b.String()
}

// truncateLines returns the content with each line limited to maxWidth chars
// and the whole text limited to maxLines lines.
func truncateLines(content string, maxWidth, maxLines int) string {
	lines := strings.Split(content, "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		lines = append(lines, "  …")
	}
	for i, l := range lines {
		if len(l) > maxWidth {
			lines[i] = l[:maxWidth-1] + "…"
		}
	}
	return strings.Join(lines, "\n")
}

// ── key handlers ─────────────────────────────────────────────────────────────

func (p *Panel) updateLeft(key string) (tea.Cmd, bool) {
	switch key {
	case "j", "down":
		if p.cursor < len(p.entries)-1 {
			p.cursor++
		}
		return nil, true

	case "k", "up":
		if p.cursor > 0 {
			p.cursor--
		}
		return nil, true

	case "enter", "l":
		if p.cursor < len(p.entries) {
			e := p.entries[p.cursor]
			p.selectedID = e.id
			p.loadConversation()
			p.focusRight = true
			p.rightVP.GotoBottom()
			if e.status == teams.StatusWorking {
				return ScheduleRefresh(), true
			}
		}
		return nil, true

	case "r":
		p.refresh()
		if p.hasRunning() {
			return ScheduleRefresh(), true
		}
		return nil, true

	case "d":
		if p.cursor < len(p.entries) {
			e := p.entries[p.cursor]
			if e.status == teams.StatusWorking && p.runner != nil {
				_ = p.runner.Kill(e.id)
				p.refresh()
				name := e.name
				return func() tea.Msg {
					return panels.ActionMsg{Type: "agui_toast", Payload: fmt.Sprintf("Agent %s cancelled", name)}
				}, true
			}
		}
		return nil, true

	case "esc":
		// Do NOT consume esc in left pane — root.go handles "esc" to close
		// the panel when the panel doesn't consume the key.
		return nil, false
	}
	return nil, false
}

func (p *Panel) updateRight(key string) (tea.Cmd, bool) {
	switch key {
	case "j", "down":
		p.rightVP.ScrollDown(1)
		return nil, true
	case "k", "up":
		p.rightVP.ScrollUp(1)
		return nil, true
	case "ctrl+d":
		p.rightVP.HalfPageDown()
		return nil, true
	case "ctrl+u":
		p.rightVP.HalfPageUp()
		return nil, true
	case "G":
		p.rightVP.GotoBottom()
		return nil, true
	case "g":
		p.rightVP.GotoTop()
		return nil, true
	case "h", "esc":
		p.focusRight = false
		return nil, true
	}
	return nil, false
}

// ── rendering ─────────────────────────────────────────────────────────────────

func (p *Panel) renderLeft() string {
	lw := p.leftPaneWidth()
	ch := p.contentHeight()
	var b strings.Builder

	// Title + separator (2 lines).
	b.WriteString(styles.PanelTitle.Render("AGUI"))
	b.WriteString("\n")
	b.WriteString(styles.SeparatorLine(lw))
	b.WriteString("\n")

	if len(p.entries) == 0 {
		b.WriteString(styles.PanelHint.Render("  No agents"))
		b.WriteString("\n")
	} else {
		groups := p.buildGroups()

		maxLines := ch // lines available for agent rows (below header)
		linesUsed := 0

		for _, g := range groups {
			if linesUsed >= maxLines {
				break
			}
			// Group header line.
			groupLabel := lipgloss.NewStyle().
				Foreground(styles.Muted).
				Bold(true).
				PaddingLeft(1).
				Render(g.name)
			b.WriteString(groupLabel + "\n")
			linesUsed++

			for i, e := range g.entries {
				if linesUsed >= maxLines {
					break
				}
				absIdx := g.startIdx + i
				b.WriteString(p.renderAgentLine(e, absIdx == p.cursor, lw))
				linesUsed++
			}
		}
	}

	return lipgloss.NewStyle().Width(lw).Height(p.height).Render(b.String())
}

// group holds agents for one team during rendering.
type group struct {
	name     string
	entries  []*agentEntry
	startIdx int // absolute cursor index for first entry in this group
}

func (p *Panel) buildGroups() []group {
	var groups []group
	seen := make(map[string]int) // team name → groups index
	abs := 0
	for _, e := range p.entries {
		teamKey := e.team
		if teamKey == "" {
			teamKey = "Ad-hoc"
		}
		if idx, ok := seen[teamKey]; ok {
			groups[idx].entries = append(groups[idx].entries, e)
		} else {
			seen[teamKey] = len(groups)
			groups = append(groups, group{name: teamKey, startIdx: abs, entries: []*agentEntry{e}})
		}
		abs++
	}
	return groups
}

func (p *Panel) renderAgentLine(e *agentEntry, selected bool, width int) string {
	prefix := "  "
	if selected {
		prefix = styles.ViewportCursor.Render("▸ ")
	}

	icon := agentStatusIcon(e.status, p.tick)

	maxNameWidth := width - 12 // room for prefix + icon + status label
	if maxNameWidth < 4 {
		maxNameWidth = 4
	}
	name := e.name
	if len(name) > maxNameWidth {
		name = name[:maxNameWidth-1] + "…"
	}

	nameStyle := lipgloss.NewStyle().Foreground(styles.Text)
	if selected {
		nameStyle = nameStyle.Bold(true)
	}
	if e.state != nil && e.state.Identity.Color != "" {
		nameStyle = nameStyle.Foreground(lipgloss.Color(e.state.Identity.Color))
	}

	statusLabel := agentStatusLabel(e.status)

	return prefix + icon + " " + nameStyle.Render(name) + " " + statusLabel + "\n"
}

func agentStatusIcon(s teams.MemberStatus, tick int) string {
	switch s {
	case teams.StatusWorking:
		frame := spinFrames[tick%len(spinFrames)]
		return lipgloss.NewStyle().Foreground(styles.Warning).Render(frame)
	case teams.StatusComplete:
		return lipgloss.NewStyle().Foreground(styles.Success).Render("✓")
	case teams.StatusFailed:
		return lipgloss.NewStyle().Foreground(styles.Error).Render("✗")
	case teams.StatusShutdown:
		return lipgloss.NewStyle().Foreground(styles.Muted).Render("⊘")
	case teams.StatusWaitingForInput:
		return lipgloss.NewStyle().Foreground(styles.Warning).Render("?")
	default:
		return lipgloss.NewStyle().Foreground(styles.Dim).Render("○")
	}
}

func agentStatusLabel(s teams.MemberStatus) string {
	var color lipgloss.Color
	switch s {
	case teams.StatusWorking:
		color = styles.Warning
	case teams.StatusComplete:
		color = styles.Success
	case teams.StatusFailed:
		color = styles.Error
	case teams.StatusShutdown, teams.StatusWaitingForInput:
		color = styles.Muted
	default:
		color = styles.Dim
	}
	return lipgloss.NewStyle().Foreground(color).Render(string(s))
}

func (p *Panel) renderRight() string {
	rw := p.rightPaneWidth()
	var b strings.Builder

	// Title line for right pane.
	if p.selectedID != "" && p.runner != nil {
		if state, ok := p.runner.GetState(p.selectedID); ok {
			nameStyle := lipgloss.NewStyle().Foreground(styles.Primary).Bold(true)
			if state.Identity.Color != "" {
				nameStyle = nameStyle.Foreground(lipgloss.Color(state.Identity.Color))
			}
			var statusColor lipgloss.Color
			switch state.Status {
			case teams.StatusWorking:
				statusColor = styles.Warning
			case teams.StatusComplete:
				statusColor = styles.Success
			case teams.StatusFailed:
				statusColor = styles.Error
			default:
				statusColor = styles.Muted
			}
			statusStr := lipgloss.NewStyle().Foreground(statusColor).Render("— " + string(state.Status))
			b.WriteString(nameStyle.Render(state.Identity.AgentName) + " " + statusStr)
		} else {
			b.WriteString(styles.PanelHint.Render(p.selectedID))
		}
	} else {
		b.WriteString(styles.PanelHint.Render("  select an agent"))
	}
	b.WriteString("\n")
	b.WriteString(styles.SeparatorLine(rw))
	b.WriteString("\n")

	// Focus indicator: left border when right pane is focused.
	vpView := p.rightVP.View()
	if p.focusRight {
		vpView = lipgloss.NewStyle().
			BorderLeft(true).
			BorderStyle(lipgloss.Border{Left: "▌"}).
			BorderForeground(styles.Primary).
			Render(vpView)
	}
	b.WriteString(vpView)

	return lipgloss.NewStyle().Width(rw).Height(p.height).Render(b.String())
}
