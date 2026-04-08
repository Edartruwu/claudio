// Package teampanel provides a TUI side panel for managing agent teams.
// Shows active teammates with status, progress, mailbox, and controls.
package teampanel

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/teams"
	"github.com/Abraxas-365/claudio/internal/tui/panels"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// RefreshMsg triggers a panel refresh.
type RefreshMsg struct{}

// spinner frames for working agents.
var spinFrames = []string{"◐", "◓", "●", "◑"}

// Panel is the agent team side panel implementing panels.Panel.
type Panel struct {
	manager *teams.Manager
	runner  *teams.TeammateRunner

	active bool
	width  int
	height int

	cursor int
	tick   int // drives spinner animation

	agents []*agentItem // cached snapshot
}

type agentItem struct {
	name     string
	id       string
	color    string
	model    string
	prompt   string
	worktree string
	status   teams.MemberStatus
	state    *teams.TeammateState
}

// New creates a new team panel.
func New(manager *teams.Manager, runner *teams.TeammateRunner) *Panel {
	return &Panel{
		manager: manager,
		runner:  runner,
	}
}

func (p *Panel) IsActive() bool { return p.active }

func (p *Panel) Activate() {
	p.active = true
	p.cursor = 0
	p.refresh()
}

func (p *Panel) Deactivate() {
	p.active = false
}

func (p *Panel) SetSize(w, h int) {
	p.width = w
	p.height = h
}

// HandleRefresh is called on RefreshMsg. Returns a tick cmd if agents are working.
func (p *Panel) HandleRefresh() tea.Cmd {
	p.tick++
	p.refresh()
	if p.hasWorking() {
		return tickCmd()
	}
	return nil
}

// ScheduleRefresh returns a cmd to start refresh ticks.
func ScheduleRefresh() tea.Cmd {
	return tickCmd()
}

func tickCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
		return RefreshMsg{}
	})
}

func (p *Panel) refresh() {
	p.agents = nil
	if p.manager == nil || p.runner == nil {
		return
	}
	teamName := p.runner.ActiveTeamName()
	if teamName == "" {
		return
	}
	team, ok := p.manager.GetTeam(teamName)
	if !ok {
		return
	}
	for _, mem := range team.Members {
		if mem.Identity.IsLead {
			continue
		}
		item := &agentItem{
			name:   mem.Identity.AgentName,
			id:     mem.Identity.AgentID,
			color:  mem.Identity.Color,
			model:  mem.Model,
			prompt: mem.Prompt,
			status: mem.Status,
		}
		if state, ok := p.runner.GetState(mem.Identity.AgentID); ok {
			item.state = state
			if item.model == "" && state.Model != "" {
				item.model = state.Model
			}
			if state.WorktreePath != "" {
				item.worktree = state.WorktreePath
			}
		}
		p.agents = append(p.agents, item)
	}
	p.clampCursor()
}

func (p *Panel) clampCursor() {
	if len(p.agents) == 0 {
		p.cursor = 0
		return
	}
	if p.cursor >= len(p.agents) {
		p.cursor = len(p.agents) - 1
	}
}

func (p *Panel) hasWorking() bool {
	for _, a := range p.agents {
		if a.status == teams.StatusWorking {
			return true
		}
	}
	return false
}

// SelectedAgent returns the name and ID of the currently selected agent.
func (p *Panel) SelectedAgent() (name, id string) {
	if p.cursor < len(p.agents) {
		a := p.agents[p.cursor]
		return a.name, a.id
	}
	return "", ""
}

func (p *Panel) Update(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch msg.String() {
	case "j", "down":
		if p.cursor < len(p.agents)-1 {
			p.cursor++
		}
		return nil, true
	case "k", "up":
		if p.cursor > 0 {
			p.cursor--
		}
		return nil, true
	case "enter":
		if p.cursor < len(p.agents) {
			a := p.agents[p.cursor]
			return func() tea.Msg {
				return panels.ActionMsg{Type: "agent_detail", Payload: a.id}
			}, true
		}
		return nil, true
	case "m":
		if p.cursor < len(p.agents) {
			a := p.agents[p.cursor]
			return func() tea.Msg {
				return panels.ActionMsg{Type: "agent_message", Payload: a.name}
			}, true
		}
		return nil, true
	case "s":
		if p.cursor < len(p.agents) {
			a := p.agents[p.cursor]
			return func() tea.Msg {
				return panels.ActionMsg{Type: "agent_share", Payload: a.name}
			}, true
		}
		return nil, true
	case "f":
		if p.cursor < len(p.agents) {
			a := p.agents[p.cursor]
			return func() tea.Msg {
				return panels.ActionMsg{Type: "agent_forward", Payload: a.name}
			}, true
		}
		return nil, true
	case "x":
		if p.cursor < len(p.agents) {
			a := p.agents[p.cursor]
			if a.status == teams.StatusWorking {
				p.runner.Kill(a.id)
				p.refresh()
			}
		}
		return nil, true
	case "r":
		p.refresh()
		if p.hasWorking() {
			return tickCmd(), true
		}
		return nil, true
	case "q":
		// Exit team view (without deleting)
		if p.runner != nil {
			p.runner.SetActiveTeam("")
			p.agents = nil
			return func() tea.Msg {
				return panels.ActionMsg{Type: "exit_team"}
			}, true
		}
		return nil, true
	case "t":
		// Cycle through available teams
		if p.runner != nil {
			names := p.runner.ListTeamNames()
			if len(names) > 1 {
				current := p.runner.ActiveTeamName()
				idx := 0
				for i, n := range names {
					if n == current {
						idx = i
						break
					}
				}
				next := names[(idx+1)%len(names)]
				p.runner.SetActiveTeam(next)
				p.cursor = 0
				p.refresh()
			}
		}
		return nil, true
	}
	return nil, false
}

func (p *Panel) View() string {
	if !p.active {
		return ""
	}

	var b strings.Builder

	// Title
	teamName := ""
	if p.runner != nil {
		teamName = p.runner.ActiveTeamName()
	}
	title := "Agents"
	if teamName != "" {
		title = fmt.Sprintf("Agents · %s", teamName)
	}
	b.WriteString(styles.PanelTitle.Render(title))
	b.WriteString("\n")
	b.WriteString(styles.SeparatorLine(p.width))
	b.WriteString("\n")

	if len(p.agents) == 0 {
		b.WriteString(styles.PanelHint.Render("  No active agents"))
		b.WriteString("\n")
		writeHint(&b, p.width, "  esc close")
		return renderPanel(b.String(), p.width, p.height)
	}

	// Summary line
	working := 0
	waiting := 0
	for _, a := range p.agents {
		switch a.status {
		case teams.StatusWorking:
			working++
		case teams.StatusWaitingForInput:
			waiting++
		}
	}
	summaryText := fmt.Sprintf("%d/%d working", working, len(p.agents))
	if waiting > 0 {
		summaryText += fmt.Sprintf(", %d waiting", waiting)
	}
	summary := lipgloss.NewStyle().Foreground(styles.Dim).PaddingLeft(2).Render(summaryText)
	b.WriteString(summary)
	b.WriteString("\n\n")

	// Agent list — compute visible range based on per-agent line height.
	availLines := p.height - 8 // lines available after header + summary + hints
	if availLines < 3 {
		availLines = 3
	}

	// Walk backward from cursor until we fill availLines, to find startIdx.
	startIdx := p.cursor
	used := 0
	for i := p.cursor; i >= 0; i-- {
		h := p.agentHeight(p.agents[i])
		if used+h > availLines && i < p.cursor {
			break
		}
		used += h
		startIdx = i
	}

	// Walk forward from startIdx until we exhaust availLines, to find endIdx.
	endIdx := startIdx
	used = 0
	for i := startIdx; i < len(p.agents); i++ {
		h := p.agentHeight(p.agents[i])
		if used+h > availLines {
			break
		}
		used += h
		endIdx = i + 1
	}

	for i := startIdx; i < endIdx; i++ {
		b.WriteString(p.renderAgent(p.agents[i], i == p.cursor))
	}

	// Scroll indicators
	if startIdx > 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(styles.Subtle).PaddingLeft(4).
			Render(fmt.Sprintf("↑ %d more", startIdx)))
		b.WriteString("\n")
	}
	if endIdx < len(p.agents) {
		b.WriteString(lipgloss.NewStyle().Foreground(styles.Subtle).PaddingLeft(4).
			Render(fmt.Sprintf("↓ %d more", len(p.agents)-endIdx)))
		b.WriteString("\n")
	}

	// Mailbox preview with messages
	if p.runner != nil {
		mailbox := p.runner.GetMailbox()
		if mailbox != nil {
			allMsgs := mailbox.AllMessages()
			var pendingMsgs []teams.Message
			for _, msg := range allMsgs {
				if !msg.Read {
					pendingMsgs = append(pendingMsgs, msg)
				}
			}
			if len(pendingMsgs) > 0 {
				b.WriteString("\n")
				b.WriteString(styles.SeparatorLine(p.width))
				b.WriteString("\n")
				label := fmt.Sprintf("✉ Mailbox (%d)", len(pendingMsgs))
				mailLabel := lipgloss.NewStyle().Foreground(styles.Warning).Bold(true).PaddingLeft(2).
					Render(label)
				b.WriteString(mailLabel)
				b.WriteString("\n")

				// Show last N pending messages
				maxShow := 6
				start := 0
				if len(pendingMsgs) > maxShow {
					start = len(pendingMsgs) - maxShow
				}
				for _, msg := range pendingMsgs[start:] {
					p.renderMailMessage(&b, msg)
				}
				if start > 0 {
					b.WriteString(lipgloss.NewStyle().Foreground(styles.Subtle).PaddingLeft(4).
						Render(fmt.Sprintf("… %d older pending", start)))
					b.WriteString("\n")
				}
			}
		}
	}

	b.WriteString("\n")
	writeHint(&b, p.width, "  j/k ⏎detail t team q exit m msg s share f fwd x kill")

	return renderPanel(b.String(), p.width, p.height)
}

// agentHeight returns the number of terminal lines renderAgent will produce.
func (p *Panel) agentHeight(a *agentItem) int {
	n := 1 // name line
	if a.prompt != "" {
		n++ // prompt preview
	}
	if a.worktree != "" {
		n++ // worktree path
	}
	if a.state != nil {
		progress := a.state.GetProgress()
		if len(progress.Activities) > 0 && progress.Activities[len(progress.Activities)-1] != "" {
			n++ // activity line
		}
		n++ // stats line
	}
	return n
}

func (p *Panel) renderAgent(a *agentItem, selected bool) string {
	var lines strings.Builder

	// First line: icon + name + duration
	prefix := "  "
	if selected {
		prefix = styles.ViewportCursor.Render("▸ ")
	}

	icon := statusIcon(a.status, p.tick)
	var nameStyle lipgloss.Style
	if a.color != "" {
		nameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(a.color)).Bold(true)
	} else {
		nameStyle = styles.AgentStyle(a.name)
	}
	if selected {
		nameStyle = nameStyle.Underline(true)
	}

	dur := ""
	if a.state != nil {
		var d time.Duration
		if !a.state.FinishedAt.IsZero() {
			d = a.state.FinishedAt.Sub(a.state.StartedAt).Truncate(time.Second)
		} else {
			d = time.Since(a.state.StartedAt).Truncate(time.Second)
		}
		dur = lipgloss.NewStyle().Foreground(styles.Dim).Render(fmt.Sprintf(" %s", smartDuration(d)))
	}

	statusLabel := ""
	switch a.status {
	case teams.StatusComplete:
		statusLabel = lipgloss.NewStyle().Foreground(styles.Success).Render(" ✓")
	case teams.StatusFailed:
		statusLabel = lipgloss.NewStyle().Foreground(styles.Error).Render(" ✗")
	case teams.StatusShutdown:
		statusLabel = lipgloss.NewStyle().Foreground(styles.Muted).Render(" ⊘")
	case teams.StatusWaitingForInput:
		statusLabel = lipgloss.NewStyle().Foreground(styles.Warning).Render(" ?")
	}

	modelTag := ""
	if a.model != "" {
		modelTag = lipgloss.NewStyle().Foreground(styles.Dim).Render(" (" + a.model + ")")
	}

	lines.WriteString(prefix + icon + " " + nameStyle.Render(a.name) + modelTag + statusLabel + dur + "\n")

	// Prompt preview line
	if a.prompt != "" {
		preview := a.prompt
		maxW := p.width - 8
		if maxW < 10 {
			maxW = 10
		}
		if len(preview) > maxW {
			preview = preview[:maxW-1] + "…"
		}
		lines.WriteString("    " + lipgloss.NewStyle().Foreground(styles.Dim).Italic(true).Render(preview) + "\n")
	}

	// Worktree path
	if a.worktree != "" {
		wtLabel := lipgloss.NewStyle().Foreground(styles.Dim).Render("    ⎇ " + a.worktree)
		lines.WriteString(wtLabel + "\n")
	}

	// Activity line
	if a.state != nil {
		progress := a.state.GetProgress()
		activities := progress.Activities
		toolCalls := progress.ToolCalls

		activity := ""
		if len(activities) > 0 {
			activity = activities[len(activities)-1]
		}
		if activity != "" {
			maxW := p.width - 8
			if len(activity) > maxW {
				activity = activity[:maxW-1] + "…"
			}
			lines.WriteString("    " + lipgloss.NewStyle().Foreground(styles.Muted).Render(activity) + "\n")
		}

		// Third line: stats
		stats := lipgloss.NewStyle().Foreground(styles.Subtle).
			Render(fmt.Sprintf("    %dt", toolCalls))
		lines.WriteString(stats + "\n")
	}

	return lines.String()
}

func (p *Panel) renderMailMessage(b *strings.Builder, msg teams.Message) {
	fromColor := lipgloss.Color(msg.Color)
	if msg.Color == "" {
		fromColor = styles.Dim
	}
	fromStyle := lipgloss.NewStyle().Foreground(fromColor).Bold(true)
	arrow := lipgloss.NewStyle().Foreground(styles.Dim).Render(" → ")
	toStyle := lipgloss.NewStyle().Foreground(styles.Subtle)

	header := "    " + fromStyle.Render(msg.From) + arrow + toStyle.Render(msg.To)

	// Timestamp
	ago := time.Since(msg.Timestamp).Truncate(time.Second)
	ts := lipgloss.NewStyle().Foreground(styles.Dim).Render(" " + smartDuration(ago))
	header += ts

	b.WriteString(header + "\n")

	// Message preview
	preview := msg.Summary
	if preview == "" {
		preview = msg.Text
	}
	maxW := p.width - 8
	if maxW < 10 {
		maxW = 10
	}
	if len(preview) > maxW {
		preview = preview[:maxW-1] + "…"
	}
	b.WriteString("      " + lipgloss.NewStyle().Foreground(styles.Muted).Render(preview) + "\n")
}

// TeamSummary returns a one-line summary for the status bar.
func (p *Panel) TeamSummary() string {
	if p.runner == nil {
		return ""
	}
	teamName := p.runner.ActiveTeamName()
	if teamName == "" {
		return ""
	}
	working := p.runner.WorkingCount()
	total := len(p.agents)
	if total == 0 {
		// Refresh to get count
		p.refresh()
		total = len(p.agents)
	}
	if total == 0 {
		return ""
	}
	if working == 0 {
		// All agents finished — don't clutter the status bar
		return ""
	}
	return fmt.Sprintf("team:%s %d/%d ◐", teamName, working, total)
}

// UnreadCount returns total unread mailbox messages.
func (p *Panel) UnreadCount() int {
	if p.runner == nil {
		return 0
	}
	mb := p.runner.GetMailbox()
	if mb == nil {
		return 0
	}
	return mb.TotalUnreadCount()
}

func statusIcon(s teams.MemberStatus, tick int) string {
	switch s {
	case teams.StatusWorking:
		frame := spinFrames[tick%len(spinFrames)]
		return lipgloss.NewStyle().Foreground(styles.Warning).Render(frame)
	case teams.StatusComplete:
		return lipgloss.NewStyle().Foreground(styles.Success).Render("●")
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

func smartDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	mins := int(d.Minutes())
	secs := int(d.Seconds()) % 60
	if secs == 0 {
		return fmt.Sprintf("%dm", mins)
	}
	return fmt.Sprintf("%dm%ds", mins, secs)
}

func writeHint(b *strings.Builder, width int, hint string) {
	b.WriteString(styles.SeparatorLine(width))
	b.WriteString("\n")
	b.WriteString(styles.PanelHint.Render(hint))
}

func renderPanel(content string, width, height int) string {
	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Render(content)
}
