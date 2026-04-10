// Package agui provides the AGUI two-mode agent inspector panel.
// List mode shows all agents as a rich scrollable list (default).
// Detail mode shows the selected agent's full-screen conversation.
// Opens with <Space>oa or the :AGUI command.
package agui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
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

// panelMode is the two-mode display state.
type panelMode int

const (
	modeList   panelMode = 0 // default: rich scrollable agent list
	modeDetail panelMode = 1 // full-screen conversation for selected agent
)

// Panel is the AGUI two-mode agent inspector implementing panels.Panel.
type Panel struct {
	runner *teams.TeammateRunner

	entries    []*agentEntry  // flat list of all agents, populated on refresh
	cursor     int            // list-mode cursor: index into filteredEntries()
	rightVP    viewport.Model // full-width scrollable conversation (detail mode)
	mode       panelMode      // current display mode
	selectedID string         // agentID of the currently viewed agent

	collapsedTeams map[string]bool // team names → collapsed state
	filterInput    textinput.Model // inline search/filter input
	filtering      bool            // true when filter bar is active

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

// displayItem is one row in the list — either a group header or an agent.
type displayItem struct {
	isHeader   bool
	headerName string // only for headers
	collapsed  bool   // only for headers — whether the group is folded
	entryIdx   int    // index into filteredEntries() (only for agents)
	entry      *agentEntry
}

// group holds agents that share the same team name.
type group struct {
	name     string
	entries  []*agentEntry
	startIdx int // index of first entry in filteredEntries()
}

// New creates a new AGUI panel with the given teammate runner.
func New(runner *teams.TeammateRunner) *Panel {
	fi := textinput.New()
	fi.Placeholder = "filter agents…"
	fi.CharLimit = 40
	return &Panel{
		runner:         runner,
		collapsedTeams: make(map[string]bool),
		filterInput:    fi,
	}
}

// ── panels.Panel interface ────────────────────────────────────────────────────

// IsActive returns true when the panel is visible.
func (p *Panel) IsActive() bool { return p.active }

// Activate makes the panel visible and loads initial data.
func (p *Panel) Activate() {
	p.active = true
	p.cursor = 0
	p.mode = modeList
	p.selectedID = ""
	p.filtering = false
	p.filterInput.SetValue("")
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
	p.rightVP.Width = w
	p.rightVP.Height = p.contentHeight()
	p.ready = true
	if p.selectedID != "" {
		p.loadConversation()
	}
}

// Update handles a key event and returns a command plus whether the key was consumed.
func (p *Panel) Update(msg tea.KeyMsg) (tea.Cmd, bool) {
	if !p.active {
		return nil, false
	}
	// When filter is active, route most keys to the textinput.
	if p.filtering {
		return p.updateFilter(msg)
	}
	if p.mode == modeDetail {
		return p.updateRight(msg.String())
	}
	return p.updateLeft(msg.String())
}

// View renders the panel in the current mode.
func (p *Panel) View() string {
	if !p.active || !p.ready {
		return ""
	}
	if p.mode == modeDetail {
		return lipgloss.NewStyle().Width(p.width).Height(p.height).Render(p.renderDetail())
	}
	return lipgloss.NewStyle().Width(p.width).Height(p.height).Render(p.renderList())
}

// Help returns the keybinding hint line shown in the panel footer.
func (p *Panel) Help() string {
	if p.filtering {
		return "type to filter · esc clear"
	}
	if p.mode == modeDetail {
		return "j/k scroll · ctrl+d/u half-page · G bottom · g top · h/esc back"
	}
	hints := "j/k navigate · enter/l open"
	if p.hasRunning() {
		hints += " · d cancel"
	}
	hints += " · m message · / filter · o/tab collapse · r refresh · esc close"
	return hints
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

// contentHeight is the viewport height: total minus title rows.
func (p *Panel) contentHeight() int {
	h := p.height - 3 // title (1) + separator (1) + hint (1)
	if h < 2 {
		h = 2
	}
	return h
}

// twoLineMode returns true when the panel is tall enough for two-line agent rows.
func (p *Panel) twoLineMode() bool {
	return p.height > 20
}

func (p *Panel) refresh() {
	if p.runner == nil {
		p.entries = nil
		return
	}
	states := p.runner.AllStates()
	entries := make([]*agentEntry, 0, len(states))
	for _, s := range states {
		if s.ParentAgentID != "" {
			continue
		}
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

	if p.selectedID != "" && p.ready {
		p.loadConversation()
	}
}

func (p *Panel) clampCursor() {
	fe := p.filteredEntries()
	if len(fe) == 0 {
		p.cursor = 0
		return
	}
	if p.cursor >= len(fe) {
		p.cursor = len(fe) - 1
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
	// Auto-scroll to bottom for running agents so latest output is visible.
	if state.Status == teams.StatusWorking {
		p.rightVP.GotoBottom()
	}
}

// ── filter helpers ────────────────────────────────────────────────────────────

// filteredEntries returns entries matching the current filter query.
func (p *Panel) filteredEntries() []*agentEntry {
	q := strings.TrimSpace(p.filterInput.Value())
	if q == "" {
		return p.entries
	}
	q = strings.ToLower(q)
	var out []*agentEntry
	for _, e := range p.entries {
		if strings.Contains(strings.ToLower(e.name), q) ||
			strings.Contains(strings.ToLower(e.team), q) ||
			strings.Contains(strings.ToLower(string(e.status)), q) {
			out = append(out, e)
		}
	}
	return out
}

// ── group + display-item building ────────────────────────────────────────────

func (p *Panel) buildGroups(entries []*agentEntry) []group {
	var groups []group
	seen := make(map[string]int) // team name → index into groups
	abs := 0
	for _, e := range entries {
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

// buildDisplayItems converts groups into the flat ordered list of rows to render.
// When only one team exists, group headers are omitted (flat list).
func (p *Panel) buildDisplayItems(entries []*agentEntry, groups []group) []displayItem {
	multiTeam := len(groups) > 1
	var items []displayItem
	for _, g := range groups {
		if multiTeam {
			collapsed := p.collapsedTeams[g.name]
			items = append(items, displayItem{
				isHeader:   true,
				headerName: g.name,
				collapsed:  collapsed,
			})
			if !collapsed {
				for i, e := range g.entries {
					items = append(items, displayItem{entryIdx: g.startIdx + i, entry: e})
				}
			}
		} else {
			for i, e := range g.entries {
				items = append(items, displayItem{entryIdx: g.startIdx + i, entry: e})
			}
		}
	}
	return items
}

// itemHeight returns the number of terminal lines one display item occupies.
func (p *Panel) itemHeight(item displayItem) int {
	if item.isHeader {
		return 1
	}
	if p.twoLineMode() {
		return 2
	}
	return 1
}

// visibleRange computes the slice [start, end) of displayItems that fit in
// availLines, keeping the cursor item visible via backward-fill centering.
// Returns hidden agent counts above and below.
func (p *Panel) visibleRange(items []displayItem, availLines int) (start, end, hiddenAbove, hiddenBelow int) {
	if len(items) == 0 || availLines <= 0 {
		return 0, 0, 0, 0
	}

	// Find which display item corresponds to the cursor.
	cursorDI := -1
	for i, item := range items {
		if !item.isHeader && item.entryIdx == p.cursor {
			cursorDI = i
			break
		}
	}
	// If cursor agent is not in items (collapsed or filtered out), start at top.
	if cursorDI == -1 {
		cursorDI = 0
	}

	// Walk backward from cursor to fill availLines (backward-fill centering).
	startIdx := cursorDI
	used := 0
	for i := cursorDI; i >= 0; i-- {
		h := p.itemHeight(items[i])
		if used+h > availLines && i < cursorDI {
			break
		}
		used += h
		startIdx = i
	}

	// Walk forward from startIdx until we exhaust availLines.
	endIdx := startIdx
	used = 0
	for i := startIdx; i < len(items); i++ {
		h := p.itemHeight(items[i])
		if used+h > availLines {
			break
		}
		used += h
		endIdx = i + 1
	}

	// Count hidden agents (headers don't count toward the indicator number).
	for i := 0; i < startIdx; i++ {
		if !items[i].isHeader {
			hiddenAbove++
		}
	}
	for i := endIdx; i < len(items); i++ {
		if !items[i].isHeader {
			hiddenBelow++
		}
	}

	return startIdx, endIdx, hiddenAbove, hiddenBelow
}

// ── conversation formatting ───────────────────────────────────────────────────

func (p *Panel) formatConversation(state *teams.TeammateState) string {
	entries := state.GetConversation()
	rw := p.width - 4 // leave some padding
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
			content := wrapContent(e.Content, rw)
			b.WriteString(prefix + content + "\n\n")

		case "text":
			// Assistant text output.
			prefix := styles.AssistantPrefix.Render("< ")
			content := wrapContent(e.Content, rw)
			b.WriteString(prefix + content + "\n\n")

		case "message_out":
			// Message sent out to another agent or back to lead.
			prefix := lipgloss.NewStyle().Foreground(styles.Aqua).Render("↗ ")
			content := wrapContent(e.Content, rw)
			b.WriteString(prefix + content + "\n\n")

		case "tool_start":
			toolLine := styles.ToolName.Render("  ⚙ "+e.ToolName) + "\n"
			b.WriteString(toolLine)
			if e.Content != "" {
				b.WriteString(styles.ToolSummary.Render("    "+e.Content) + "\n")
			}
			b.WriteString("\n")

		case "tool_end":
			if e.Content != "" {
				b.WriteString(
					lipgloss.NewStyle().Foreground(styles.Success).Render("  ✓ ") +
						styles.ToolSummary.Render(e.Content) + "\n\n")
			}

		case "complete":
			result := e.Content
			if result == "" {
				result = "done"
			}
			b.WriteString(lipgloss.NewStyle().Foreground(styles.Success).Render("  ✓ complete") + "\n")
			b.WriteString(styles.ToolSummary.Render("  "+result) + "\n\n")

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

// wrapContent wraps long lines to maxWidth without truncating content.
func wrapContent(content string, maxWidth int) string {
	if maxWidth < 4 {
		maxWidth = 4
	}
	lines := strings.Split(content, "\n")
	var out []string
	for _, l := range lines {
		for len(l) > maxWidth {
			out = append(out, l[:maxWidth])
			l = l[maxWidth:]
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

// ── key handlers ─────────────────────────────────────────────────────────────

// updateFilter routes keystrokes to the filter input.
func (p *Panel) updateFilter(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch msg.String() {
	case "esc":
		p.filtering = false
		p.filterInput.SetValue("")
		p.filterInput.Blur()
		p.cursor = 0
		p.clampCursor()
		return nil, true
	}
	var cmd tea.Cmd
	p.filterInput, cmd = p.filterInput.Update(msg)
	// Clamp cursor when filter changes.
	p.clampCursor()
	return cmd, true
}

func (p *Panel) updateLeft(key string) (tea.Cmd, bool) {
	fe := p.filteredEntries()

	switch key {
	case "j", "down":
		if p.cursor < len(fe)-1 {
			p.cursor++
		}
		return nil, true

	case "k", "up":
		if p.cursor > 0 {
			p.cursor--
		}
		return nil, true

	case "enter", "l":
		if p.cursor < len(fe) {
			e := fe[p.cursor]
			p.selectedID = e.id
			p.loadConversation()
			p.mode = modeDetail
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
		if p.cursor < len(fe) {
			e := fe[p.cursor]
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

	case "m":
		// Message agent: emit ActionMsg for root to pre-fill prompt.
		if p.cursor < len(fe) {
			e := fe[p.cursor]
			agentName := e.name
			return func() tea.Msg {
				return panels.ActionMsg{Type: "send_to_agent", Payload: agentName}
			}, true
		}
		return nil, true

	case "/":
		p.filtering = true
		p.filterInput.Focus()
		return nil, true

	case "o", "tab":
		// Toggle collapsed state for the group of the currently selected agent.
		if p.cursor < len(fe) {
			teamKey := fe[p.cursor].team
			if teamKey == "" {
				teamKey = "Ad-hoc"
			}
			// Only toggle if there are multiple groups.
			groups := p.buildGroups(fe)
			if len(groups) > 1 {
				p.collapsedTeams[teamKey] = !p.collapsedTeams[teamKey]
			}
		}
		return nil, true

	case "esc":
		// Do NOT consume esc in list mode — root.go handles "esc" to close
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
		p.mode = modeList
		return nil, true
	}
	return nil, false
}

// ── rendering ─────────────────────────────────────────────────────────────────

// renderList renders the full-width agent list (list mode).
func (p *Panel) renderList() string {
	w := p.width
	var b strings.Builder

	// 1. Title line with hint shortcuts (1 line).
	titleLeft := styles.PanelTitle.Render("Agents")
	hints := lipgloss.NewStyle().Foreground(styles.Muted).Render("[/] filter  [r]ef")
	gap := w - lipgloss.Width(titleLeft) - lipgloss.Width(hints)
	if gap < 1 {
		gap = 1
	}
	b.WriteString(titleLeft + strings.Repeat(" ", gap) + hints)
	b.WriteString("\n")
	b.WriteString(styles.SeparatorLine(w))
	b.WriteString("\n")

	// 2. Status summary (1 line, hidden when all counts are zero).
	summary := p.renderStatusSummary()
	if summary != "" {
		b.WriteString(summary)
		b.WriteString("\n")
	}

	// 3. Filter bar (1 line, only when filtering).
	fixedLines := 2 // title + sep
	if summary != "" {
		fixedLines++ // + summary
	}
	if p.filtering {
		b.WriteString(p.renderFilterBar(w))
		b.WriteString("\n")
		fixedLines++
	}

	// 4. Agent list with scroll indicators.
	availLines := p.height - fixedLines
	if availLines < 2 {
		availLines = 2
	}

	fe := p.filteredEntries()
	if len(fe) == 0 {
		if p.filtering && p.filterInput.Value() != "" {
			b.WriteString(styles.PanelHint.Render("  No matching agents"))
			b.WriteString("\n")
		} else {
			b.WriteString(lipgloss.NewStyle().Width(w).Align(lipgloss.Center).Render(
				styles.PanelHint.Render("No agents running yet.")))
			b.WriteString("\n")
			b.WriteString(lipgloss.NewStyle().Width(w).Align(lipgloss.Center).Render(
				styles.PanelHint.Render("Spawn one with SpawnTeammate or")))
			b.WriteString("\n")
			b.WriteString(lipgloss.NewStyle().Width(w).Align(lipgloss.Center).Render(
				styles.PanelHint.Render("use /agent to set a persona.")))
			b.WriteString("\n")
		}
	} else {
		groups := p.buildGroups(fe)
		items := p.buildDisplayItems(fe, groups)
		startIdx, endIdx, hiddenAbove, hiddenBelow := p.visibleRange(items, availLines)
		// Re-budget availLines if scroll indicators will be shown.
		adjusted := availLines
		if hiddenAbove > 0 {
			adjusted--
		}
		if hiddenBelow > 0 {
			adjusted--
		}
		if adjusted < availLines {
			startIdx, endIdx, hiddenAbove, hiddenBelow = p.visibleRange(items, adjusted)
		}

		// Scroll up indicator.
		if hiddenAbove > 0 {
			b.WriteString(lipgloss.NewStyle().Foreground(styles.Subtle).PaddingLeft(4).
				Render(fmt.Sprintf("↑ %d more", hiddenAbove)))
			b.WriteString("\n")
		}

		// Visible rows.
		for i := startIdx; i < endIdx; i++ {
			item := items[i]
			if item.isHeader {
				b.WriteString(p.renderGroupHeader(item.headerName, item.collapsed, w))
			} else {
				b.WriteString(p.renderAgentLine(item.entry, item.entryIdx == p.cursor, w))
			}
		}

		// Scroll down indicator.
		if hiddenBelow > 0 {
			b.WriteString(lipgloss.NewStyle().Foreground(styles.Subtle).PaddingLeft(4).
				Render(fmt.Sprintf("↓ %d more", hiddenBelow)))
			b.WriteString("\n")
		}
	}

	return lipgloss.NewStyle().Width(w).Height(p.height).Render(b.String())
}

// renderStatusSummary builds the "N running · N waiting · N done" line.
func (p *Panel) renderStatusSummary() string {
	var running, waiting, done int
	for _, e := range p.entries {
		switch e.status {
		case teams.StatusWorking:
			running++
		case teams.StatusWaitingForInput:
			waiting++
		case teams.StatusComplete, teams.StatusFailed, teams.StatusShutdown:
			done++
		}
	}

	if running == 0 && waiting == 0 && done == 0 {
		return ""
	}

	sep := lipgloss.NewStyle().Foreground(styles.Muted).Render(" · ")
	runStr := lipgloss.NewStyle().Foreground(styles.Warning).Render(fmt.Sprintf("%d running", running))
	waitStr := lipgloss.NewStyle().Foreground(styles.Warning).Render(fmt.Sprintf("%d waiting", waiting))
	doneStr := lipgloss.NewStyle().Foreground(styles.Success).Render(fmt.Sprintf("%d done", done))

	return "  " + runStr + sep + waitStr + sep + doneStr
}

// renderFilterBar renders the active filter input line.
func (p *Panel) renderFilterBar(width int) string {
	prefix := lipgloss.NewStyle().Foreground(styles.Warning).Bold(true).Render("/")
	input := p.filterInput.View()
	line := " " + prefix + " " + input
	if lipgloss.Width(line) > width {
		line = line[:width]
	}
	return line
}

// renderGroupHeader renders a collapsible group header line.
// Folded: ▸ team-name (N)   Unfolded: ▾ team-name
func (p *Panel) renderGroupHeader(name string, collapsed bool, width int) string {
	arrow := "▾"
	if collapsed {
		// Count agents in the collapsed group to show the count.
		count := 0
		for _, e := range p.entries {
			teamKey := e.team
			if teamKey == "" {
				teamKey = "Ad-hoc"
			}
			if teamKey == name {
				count++
			}
		}
		arrow = fmt.Sprintf("▸ %s (%d)", name, count)
		return lipgloss.NewStyle().
			Foreground(styles.Muted).
			Bold(true).
			PaddingLeft(1).
			Render(arrow) + "\n"
	}
	label := arrow + " " + name
	return lipgloss.NewStyle().
		Foreground(styles.Muted).
		Bold(true).
		PaddingLeft(1).
		Render(label) + "\n"
}

// renderAgentLine renders one agent in the list (1 or 2 lines based on height).
func (p *Panel) renderAgentLine(e *agentEntry, selected bool, width int) string {
	prefix := "  "
	if selected {
		prefix = styles.ViewportCursor.Render("▸ ")
	}

	icon := agentStatusIcon(e.status, p.tick)

	// Name: colored by agent color if set, bold when selected.
	maxNameWidth := width - 14 // prefix(2) + icon(1) + space(1) + status(~6) + duration(~4)
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

	// Duration.
	dur := ""
	if e.state != nil {
		var d time.Duration
		if !e.state.FinishedAt.IsZero() {
			d = e.state.FinishedAt.Sub(e.state.StartedAt).Truncate(time.Second)
		} else if !e.state.StartedAt.IsZero() {
			d = time.Since(e.state.StartedAt).Truncate(time.Second)
		}
		if d > 0 {
			dur = lipgloss.NewStyle().Foreground(styles.Dim).Render(" " + smartDuration(d))
		}
	}

	line1content := prefix + icon + " " + nameStyle.Render(name) + " " + statusLabel + dur
	if selected {
		line1content = lipgloss.NewStyle().Background(styles.Subtle).Width(width).Render(line1content)
	}
	line1 := line1content + "\n"

	if !p.twoLineMode() {
		return line1
	}

	// Line 2: italic last message preview from GetProgress().
	activity := ""
	if e.state != nil {
		progress := e.state.GetProgress()
		if len(progress.Activities) > 0 {
			activity = progress.Activities[len(progress.Activities)-1]
		}
	}
	maxActivity := width - 4
	if maxActivity < 4 {
		maxActivity = 4
	}
	if len(activity) > maxActivity {
		activity = activity[:maxActivity-1] + "…"
	}
	if activity == "" {
		activity = " "
	}
	line2content := "  " + lipgloss.NewStyle().Foreground(styles.Muted).Italic(true).Render(activity)
	if selected {
		line2content = lipgloss.NewStyle().Background(styles.Subtle).Width(width).Render(line2content)
	}
	line2 := line2content + "\n"

	return line1 + line2
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

// renderDetail renders the full-width conversation detail mode.
func (p *Panel) renderDetail() string {
	w := p.width
	var b strings.Builder

	// Header line: "← agentName  team  [status] spinner  [esc]"
	if p.selectedID != "" && p.runner != nil {
		if state, ok := p.runner.GetState(p.selectedID); ok {
			nameStyle := lipgloss.NewStyle().Foreground(styles.Primary).Bold(true)
			if state.Identity.Color != "" {
				nameStyle = nameStyle.Foreground(lipgloss.Color(state.Identity.Color))
			}

			backArrow := lipgloss.NewStyle().Foreground(styles.Muted).Render("← ")
			agentName := nameStyle.Render(state.Identity.AgentName)
			teamName := ""
			if state.TeamName != "" {
				teamName = "  " + lipgloss.NewStyle().Foreground(styles.Dim).Render(state.TeamName)
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
			statusBadge := "  " + lipgloss.NewStyle().Foreground(statusColor).Render("["+string(state.Status)+"]")

			spinnerStr := ""
			if state.Status == teams.StatusWorking {
				frame := spinFrames[p.tick%len(spinFrames)]
				spinnerStr = " " + lipgloss.NewStyle().Foreground(styles.Warning).Render(frame)
			}

			escHint := lipgloss.NewStyle().Foreground(styles.Muted).Render("[esc]")

			headerContent := backArrow + agentName + teamName + statusBadge + spinnerStr
			headerWidth := lipgloss.Width(headerContent)
			escWidth := lipgloss.Width(escHint)
			gap := w - headerWidth - escWidth
			if gap < 1 {
				gap = 1
			}
			b.WriteString(headerContent + strings.Repeat(" ", gap) + escHint)
		} else {
			b.WriteString(styles.PanelHint.Render("← " + p.selectedID))
		}
	} else {
		b.WriteString(styles.PanelHint.Render("  select an agent"))
	}
	b.WriteString("\n")
	b.WriteString(styles.SeparatorLine(w))
	b.WriteString("\n")

	b.WriteString(p.rightVP.View())

	return lipgloss.NewStyle().Width(w).Height(p.height).Render(b.String())
}

// smartDuration formats a duration concisely.
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
