// Package agui provides the AGUI agent inspector panel with unified split-view layout.
// Left pane shows agent list with filtering and team grouping.
// Right pane shows agent detail with header, live tools, and conversation feed.
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

// Panel is the AGUI split-view agent inspector implementing panels.Panel.
type Panel struct {
	runner *teams.TeammateRunner

	entries    []*agentEntry  // flat list of all agents, populated on refresh
	cursor     int            // list cursor: index into filteredEntries()
	detailVP   viewport.Model // scrollable right pane viewport
	focusedPane int           // 0 = left (list), 1 = right (detail)
	selectedID string         // agentID of the currently-selected agent for detail view
	atBottom   bool           // whether right pane is scrolled to bottom (auto-scroll flag)

	collapsedTeams map[string]bool // team names → collapsed state
	filterInput    textinput.Model // inline search/filter input
	filtering      bool            // true when filter bar is active

	confirming  bool
	confirmID   string
	confirmName string

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
		atBottom:       true,
		focusedPane:    0, // start with left pane focused
	}
}

// ── panels.Panel interface ────────────────────────────────────────────────────

// IsActive returns true when the panel is visible.
func (p *Panel) IsActive() bool { return p.active }

// Activate makes the panel visible and loads initial data.
func (p *Panel) Activate() {
	p.active = true
	p.cursor = 0
	p.selectedID = ""
	p.filtering = false
	p.filterInput.SetValue("")
	p.focusedPane = 0
	p.atBottom = true
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
	// Calculate detail VP dimensions accounting for left pane, borders, and padding
	leftWidth := p.leftPaneWidth()
	// detailVP gets the remaining width, minus borders
	p.detailVP.Width = p.width - leftWidth - 3 // account for separator and padding
	if p.detailVP.Width < 20 {
		p.detailVP.Width = 20
	}
	p.detailVP.Height = p.contentHeight()
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
	if p.confirming {
		return p.updateConfirm(msg)
	}
	// When filter is active, route most keys to the textinput.
	if p.filtering {
		return p.updateFilter(msg)
	}
	if p.focusedPane == 1 {
		return p.updateRight(msg.String())
	}
	return p.updateLeft(msg.String())
}

// View renders the panel with split layout.
func (p *Panel) View() string {
	if !p.active || !p.ready {
		return ""
	}
	return p.renderSplitView()
}

// Help returns the keybinding hint line shown in the panel footer.
func (p *Panel) Help() string {
	if p.confirming {
		return "y confirm delete · n/esc cancel"
	}
	if p.filtering {
		return "type to filter · esc clear"
	}
	if p.focusedPane == 1 {
		return "j/k scroll · ctrl+d/u half-page · G bottom · g top · e editor · h/tab/esc back"
	}
	hints := "j/k navigate · enter/l select · tab panel · x delete"
	if p.hasRunning() {
		hints += " · d cancel"
	}
	hints += " · m message · / filter · o collapse · r refresh · esc close"
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

// ── HandleTeammateEvent ───────────────────────────────────────────────────────

// HandleTeammateEvent processes real-time agent events for the selected agent.
// Updates the detail view live as new conversation entries arrive.
func (p *Panel) HandleTeammateEvent(event teams.TeammateEvent) tea.Cmd {
	// Only update if this event is for the selected agent
	if event.AgentID != p.selectedID {
		return nil
	}

	// Rebuild the detail content from fresh state
	p.loadConversation()

	// Auto-scroll to bottom if user hasn't manually scrolled up
	if p.atBottom {
		p.detailVP.GotoBottom()
	}

	return nil
}

// ── private helpers ───────────────────────────────────────────────────────────

// contentHeight is the viewport height for right pane: total minus title rows.
func (p *Panel) contentHeight() int {
	h := p.height - 3 // title (1) + separator (1) + hint (1)
	if h < 2 {
		h = 2
	}
	return h
}

// leftPaneWidth calculates the width of the left pane (~30% of total, minimum 20).
func (p *Panel) leftPaneWidth() int {
	w := p.width * 30 / 100
	if w < 20 {
		w = 20
	}
	if w > p.width-40 {
		w = p.width - 40 // ensure right pane gets at least 40 cols
	}
	return w
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
	// Build a set of all tracked agent IDs so we can identify depth-2+ sub-agents.
	trackedIDs := make(map[string]struct{}, len(states))
	for _, s := range states {
		trackedIDs[s.Identity.AgentID] = struct{}{}
	}
	entries := make([]*agentEntry, 0, len(states))
	for _, s := range states {
		// Hide depth-2+ sub-agents: those whose parent is itself a tracked teammate.
		// Agents spawned by the TUI session (parent not in the runner) are shown.
		if s.ParentAgentID != "" {
			if _, parentIsTeammate := trackedIDs[s.ParentAgentID]; parentIsTeammate {
				continue
			}
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
		p.detailVP.SetContent(styles.PanelHint.Render("  Agent not found."))
		return
	}
	content := p.buildConversationDetail(state)
	p.detailVP.SetContent(content)
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

// ── conversation detail building ──────────────────────────────────────────────

// buildConversationDetail constructs the 3-section right pane content:
// header (name/status/model/task), tools section, and conversation feed.
func (p *Panel) buildConversationDetail(state *teams.TeammateState) string {
	rw := p.detailVP.Width - 2 // leave padding
	if rw < 10 {
		rw = 10
	}

	var b strings.Builder

	// Section 1: Header (name, status, elapsed, model, turns, task, error/result)
	b.WriteString(p.buildDetailHeader(state, rw))
	b.WriteString("\n")

	// Section 2: Tools section (last 20 tool calls)
	toolContent := p.buildToolsSection(state, rw)
	if toolContent != "" {
		b.WriteString(styles.SeparatorLine(rw + 2))
		b.WriteString("\n")
		b.WriteString(toolContent)
		b.WriteString("\n")
	}

	// Section 3: Conversation section
	conversationContent := p.formatConversation(state, rw)
	if conversationContent != "" {
		b.WriteString(styles.SeparatorLine(rw + 2))
		b.WriteString("\n")
		b.WriteString(conversationContent)
		b.WriteString("\n")
	}

	// Streaming indicator for running agents.
	if state.Status == teams.StatusWorking {
		spinner := spinFrames[p.tick%len(spinFrames)]
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(styles.Warning).Render("  "+spinner+" streaming") + "\n")
	}

	return b.String()
}

// buildDetailHeader constructs the fixed header section.
func (p *Panel) buildDetailHeader(state *teams.TeammateState, width int) string {
	var b strings.Builder

	// Line 1: Agent name, status icon, elapsed time
	nameStyle := lipgloss.NewStyle().Foreground(styles.Primary).Bold(true)
	if state.Identity.Color != "" {
		nameStyle = nameStyle.Foreground(lipgloss.Color(state.Identity.Color))
	}

	icon := agentStatusIcon(state.Status, p.tick)
	agentName := nameStyle.Render(state.Identity.AgentName)

	var elapsed string
	if !state.StartedAt.IsZero() {
		var d time.Duration
		if !state.FinishedAt.IsZero() {
			d = state.FinishedAt.Sub(state.StartedAt).Truncate(time.Second)
		} else {
			d = time.Since(state.StartedAt).Truncate(time.Second)
		}
		elapsed = "  " + lipgloss.NewStyle().Foreground(styles.Dim).Render(smartDuration(d))
	}

	statusLabel := agentStatusLabel(state.Status)
	line1 := "  " + icon + " " + agentName + "  " + statusLabel + elapsed
	b.WriteString(line1)
	b.WriteString("\n")

	// Line 2: Model and turns
	if state.Model != "" || state.MaxTurns > 0 {
		var line2Parts []string
		if state.Model != "" {
			line2Parts = append(line2Parts, "Model: "+state.Model)
		}
		if state.MaxTurns > 0 {
			progress := state.GetProgress()
			line2Parts = append(line2Parts, fmt.Sprintf("Turn %d/%d", progress.ToolCalls, state.MaxTurns))
		}
		line2 := "  " + strings.Join(line2Parts, "  ·  ")
		b.WriteString(line2)
		b.WriteString("\n")
	}

	// Line 3: Task/Prompt (first 120 chars)
	if state.Prompt != "" {
		task := state.Prompt
		if len(task) > 120 {
			task = task[:117] + "…"
		}
		taskLine := "  " + lipgloss.NewStyle().Foreground(styles.Dim).Italic(true).Render("Task: "+task)
		b.WriteString(taskLine)
		b.WriteString("\n")
	}

	// Line 4: Error message if failed
	if state.Status == teams.StatusFailed && state.Error != "" {
		errorLine := "  " + lipgloss.NewStyle().Foreground(styles.Error).Render("✗ "+state.Error)
		b.WriteString(errorLine)
		b.WriteString("\n")
	}

	// Line 5: Result summary if complete
	if state.Status == teams.StatusComplete && state.Result != "" {
		result := state.Result
		if len(result) > 200 {
			result = result[:197] + "…"
		}
		resultLine := "  " + lipgloss.NewStyle().Foreground(styles.Success).Render("✓ "+result)
		b.WriteString(resultLine)
		b.WriteString("\n")
	}

	return b.String()
}

// buildToolsSection constructs the TOOLS section with live tool calls.
func (p *Panel) buildToolsSection(state *teams.TeammateState, width int) string {
	if state == nil {
		return ""
	}

	// Extract tool calls from conversation
	var toolCalls []teams.ConversationEntry
	for _, entry := range state.Conversation {
		if entry.Type == "tool_start" || entry.Type == "tool_end" {
			toolCalls = append(toolCalls, entry)
		}
	}

	if len(toolCalls) == 0 {
		return ""
	}

	// Keep last 20 tool calls
	if len(toolCalls) > 20 {
		toolCalls = toolCalls[len(toolCalls)-20:]
	}

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(styles.Muted).Render("─── TOOLS ───"))
	b.WriteString("\n")

	// Group tool_start and tool_end entries
	toolMap := make(map[string]*toolCallGroup) // tool name → tool group
	for _, entry := range toolCalls {
		if entry.Type == "tool_start" {
			if _, ok := toolMap[entry.ToolName]; !ok {
				toolMap[entry.ToolName] = &toolCallGroup{name: entry.ToolName}
			}
			toolMap[entry.ToolName].input = entry.Content
			toolMap[entry.ToolName].status = "running"
		} else if entry.Type == "tool_end" {
			if _, ok := toolMap[entry.ToolName]; !ok {
				toolMap[entry.ToolName] = &toolCallGroup{name: entry.ToolName}
			}
			toolMap[entry.ToolName].output = entry.Content
			toolMap[entry.ToolName].status = "done"
		}
	}

	// Render tool calls in order they appear
	seen := make(map[string]bool)
	for _, entry := range toolCalls {
		if entry.Type != "tool_start" {
			continue
		}
		if seen[entry.ToolName] {
			continue // Skip duplicates
		}
		seen[entry.ToolName] = true

		group := toolMap[entry.ToolName]
		if group == nil {
			continue
		}

		// Determine status badge and color
		var statusBadge string
		switch group.status {
		case "running":
			spinner := spinFrames[p.tick%len(spinFrames)]
			statusBadge = lipgloss.NewStyle().Foreground(styles.Warning).Render(spinner)
		case "done":
			statusBadge = lipgloss.NewStyle().Foreground(styles.Success).Render("✓")
		default:
			statusBadge = " "
		}

		// Tool name with skill indicator
		toolNameStyle := lipgloss.NewStyle()
		if group.name == "Skill" {
			toolNameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#e78a1f")) // orange
			statusBadge = lipgloss.NewStyle().Foreground(lipgloss.Color("#e78a1f")).Render("★")
		}
		toolName := toolNameStyle.Render(group.name)

		// Truncate input to 60 chars
		input := group.input
		if len(input) > 60 {
			input = input[:57] + "…"
		}

		line := fmt.Sprintf("  %s %s  %s", statusBadge, toolName, styles.ToolSummary.Render(input))
		b.WriteString(line)
		b.WriteString("\n")
	}

	return b.String()
}

type toolCallGroup struct {
	name   string
	input  string
	output string
	status string // "running", "done"
}

// formatConversation renders the full conversation feed.
func (p *Panel) formatConversation(state *teams.TeammateState, rw int) string {
	entries := state.GetConversation()

	if len(entries) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(styles.Muted).Render("─── CONVERSATION ───"))
	b.WriteString("\n")

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
			toolLine := styles.ToolName.Render("  → " + e.ToolName) + "\n"
			b.WriteString(toolLine)
			if e.Content != "" {
				b.WriteString(styles.ToolSummary.Render("    "+e.Content) + "\n")
			}
			b.WriteString("\n")

		case "tool_end":
			if e.Content != "" {
				b.WriteString(
					lipgloss.NewStyle().Foreground(styles.Success).Render("  ← ") +
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

	return b.String()
}

// plainTextDetail builds a plain-text (no ANSI) snapshot of the agent detail
// suitable for opening in an external editor.
func (p *Panel) plainTextDetail(state *teams.TeammateState) string {
	var b strings.Builder

	// Header
	b.WriteString("# Agent: " + state.Identity.AgentName + "\n")
	b.WriteString("Status: " + string(state.Status) + "\n")
	if state.Model != "" {
		b.WriteString("Model: " + state.Model + "\n")
	}
	if !state.StartedAt.IsZero() {
		b.WriteString("Started: " + state.StartedAt.Format("2006-01-02 15:04:05") + "\n")
		if !state.FinishedAt.IsZero() {
			b.WriteString("Finished: " + state.FinishedAt.Format("2006-01-02 15:04:05") + "\n")
			b.WriteString("Duration: " + smartDuration(state.FinishedAt.Sub(state.StartedAt).Truncate(1e9)) + "\n")
		}
	}
	if state.Prompt != "" {
		b.WriteString("\n## Task\n" + state.Prompt + "\n")
	}
	if state.Error != "" {
		b.WriteString("\n## Error\n" + state.Error + "\n")
	}
	if state.Result != "" {
		b.WriteString("\n## Result\n" + state.Result + "\n")
	}

	// Tool calls
	entries := state.GetConversation()
	var hasTools bool
	for _, e := range entries {
		if e.Type == "tool_start" {
			hasTools = true
			break
		}
	}
	if hasTools {
		b.WriteString("\n## Tools\n")
		for _, e := range entries {
			switch e.Type {
			case "tool_start":
				b.WriteString("→ " + e.ToolName)
				if e.Content != "" {
					b.WriteString("  " + e.Content)
				}
				b.WriteString("\n")
			case "tool_end":
				if e.Content != "" {
					b.WriteString("← " + e.Content + "\n")
				}
			}
		}
	}

	// Full conversation
	b.WriteString("\n## Conversation\n")
	for _, e := range entries {
		switch e.Type {
		case "message_in":
			b.WriteString("\n[IN]\n" + e.Content + "\n")
		case "text":
			b.WriteString("\n[ASSISTANT]\n" + e.Content + "\n")
		case "message_out":
			b.WriteString("\n[OUT]\n" + e.Content + "\n")
		case "complete":
			b.WriteString("\n[COMPLETE]\n" + e.Content + "\n")
		case "error":
			b.WriteString("\n[ERROR]\n" + e.Content + "\n")
		}
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

func (p *Panel) updateConfirm(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch msg.String() {
	case "y", "Y":
		id, name := p.confirmID, p.confirmName
		p.confirming = false
		p.confirmID = ""
		p.confirmName = ""
		if p.runner != nil {
			_ = p.runner.RemoveAgent(id)
		}
		p.refresh()
		return func() tea.Msg {
			return panels.ActionMsg{Type: "agui_toast", Payload: fmt.Sprintf("Agent %q deleted", name)}
		}, true
	default: // n, N, esc, or anything else cancels
		p.confirming = false
		p.confirmID = ""
		p.confirmName = ""
		return nil, true
	}
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
			p.atBottom = true
			if e.status == teams.StatusWorking {
				return ScheduleRefresh(), true
			}
		}
		return nil, true

	case "tab":
		// Switch to right pane
		if p.selectedID != "" {
			p.focusedPane = 1
			return nil, true
		}
		return nil, false

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

	case "x":
		if p.cursor < len(fe) {
			e := fe[p.cursor]
			if e.status != teams.StatusWorking {
				p.confirming = true
				p.confirmID = e.id
				p.confirmName = e.name
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

	case "o":
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
		p.detailVP.ScrollDown(1)
		p.atBottom = false
		return nil, true
	case "k", "up":
		p.detailVP.ScrollUp(1)
		p.atBottom = false
		return nil, true
	case "ctrl+d":
		p.detailVP.HalfPageDown()
		p.atBottom = false
		return nil, true
	case "ctrl+u":
		p.detailVP.HalfPageUp()
		p.atBottom = false
		return nil, true
	case "G":
		p.detailVP.GotoBottom()
		p.atBottom = true
		return nil, true
	case "g":
		p.detailVP.GotoTop()
		p.atBottom = false
		return nil, true
	case "e":
		if p.runner != nil && p.selectedID != "" {
			if state, ok := p.runner.GetState(p.selectedID); ok {
				content := p.plainTextDetail(state)
				return func() tea.Msg { return panels.ActionMsg{Type: "open_in_editor", Payload: content} }, true
			}
		}
		return nil, true
	case "h", "esc", "tab":
		p.focusedPane = 0
		return nil, true
	}
	return nil, false
}

// ── rendering ─────────────────────────────────────────────────────────────────

// renderSplitView renders the split layout with left and right panes.
func (p *Panel) renderSplitView() string {
	leftPane := p.renderLeftPane()
	rightPane := p.renderRightPane()

	// Apply borders to both panes
	leftBorder := p.borderStyle(p.focusedPane == 0)
	rightBorder := p.borderStyle(p.focusedPane == 1)

	leftStyled := leftBorder.Width(p.leftPaneWidth()).Height(p.height).Render(leftPane)
	rightWidth := p.width - p.leftPaneWidth() - 2
	if rightWidth < 20 {
		rightWidth = 20
	}
	rightStyled := rightBorder.Width(rightWidth).Height(p.height).Render(rightPane)

	// Join horizontally
	joined := lipgloss.JoinHorizontal(lipgloss.Top, leftStyled, rightStyled)
	return lipgloss.NewStyle().Width(p.width).Height(p.height).Render(joined)
}

// borderStyle returns a style with the appropriate border (active = primary, inactive = muted).
func (p *Panel) borderStyle(active bool) lipgloss.Style {
	s := lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
	if active {
		s = s.BorderForeground(styles.Primary)
	} else {
		s = s.BorderForeground(styles.Muted)
	}
	return s
}

// renderLeftPane renders the agent list pane.
func (p *Panel) renderLeftPane() string {
	w := p.leftPaneWidth() - 4 // account for borders
	var b strings.Builder

	// 1. Title line with status summary
	titleLeft := styles.PanelTitle.Render("AGENTS")
	summary := p.renderStatusSummary()
	if summary != "" {
		gap := w - lipgloss.Width(titleLeft) - lipgloss.Width(summary)
		if gap < 1 {
			gap = 1
		}
		b.WriteString(titleLeft + strings.Repeat(" ", gap) + summary)
	} else {
		b.WriteString(titleLeft)
	}
	b.WriteString("\n")
	b.WriteString(styles.SeparatorLine(w))
	b.WriteString("\n")

	// 2. Filter bar (1 line, only when filtering).
	fixedLines := 2 // title + sep
	if p.filtering {
		b.WriteString(p.renderFilterBar(w))
		b.WriteString("\n")
		fixedLines++
	}

	// 3. Agent list with scroll indicators.
	availLines := p.height - fixedLines - 3 // account for border and padding
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
				styles.PanelHint.Render("No agents yet.")))
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
			b.WriteString(lipgloss.NewStyle().Foreground(styles.Subtle).PaddingLeft(2).
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
			b.WriteString(lipgloss.NewStyle().Foreground(styles.Subtle).PaddingLeft(2).
				Render(fmt.Sprintf("↓ %d more", hiddenBelow)))
			b.WriteString("\n")
		}
	}

	if p.confirming {
		confirmLine := "\n" + lipgloss.NewStyle().
			Foreground(styles.Error).Bold(true).
			Render(fmt.Sprintf(`  Delete %q? [y/N] `, p.confirmName))
		b.WriteString(confirmLine)
	}

	return b.String()
}

// renderStatusSummary builds the status counts summary line (e.g., "3◐ 1● 1✗").
func (p *Panel) renderStatusSummary() string {
	var working, done, failed int
	for _, e := range p.entries {
		switch e.status {
		case teams.StatusWorking:
			working++
		case teams.StatusComplete:
			done++
		case teams.StatusFailed:
			failed++
		}
	}

	if working == 0 && done == 0 && failed == 0 {
		return ""
	}

	var parts []string
	if working > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(styles.Warning).
			Render(fmt.Sprintf("%d◐", working)))
	}
	if done > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(styles.Success).
			Render(fmt.Sprintf("%d●", done)))
	}
	if failed > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(styles.Error).
			Render(fmt.Sprintf("%d✗", failed)))
	}

	return strings.Join(parts, " ")
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

	// Tool count
	var toolCount string
	if e.state != nil {
		progress := e.state.GetProgress()
		if progress.ToolCalls > 0 {
			toolCount = lipgloss.NewStyle().Foreground(styles.Dim).Render(fmt.Sprintf(" %dt", progress.ToolCalls))
		}
	}

	line1content := prefix + icon + " " + nameStyle.Render(name) + " " + statusLabel + dur + toolCount
	if selected {
		line1content = lipgloss.NewStyle().Background(styles.Subtle).Width(width).Render(line1content)
	}
	line1 := line1content + "\n"

	if !p.twoLineMode() {
		return line1
	}

	// Line 2: italic last activity preview from GetProgress().
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

// renderRightPane renders the agent detail pane.
func (p *Panel) renderRightPane() string {
	w := p.detailVP.Width
	if w < 20 {
		w = 20
	}

	var b strings.Builder

	// Title line
	if p.selectedID != "" && p.runner != nil {
		if state, ok := p.runner.GetState(p.selectedID); ok {
			nameStyle := lipgloss.NewStyle().Foreground(styles.Primary).Bold(true)
			if state.Identity.Color != "" {
				nameStyle = nameStyle.Foreground(lipgloss.Color(state.Identity.Color))
			}
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

			headerContent := agentName + teamName + statusBadge
			b.WriteString(headerContent)
		}
	} else {
		b.WriteString(styles.PanelHint.Render("select an agent"))
	}
	b.WriteString("\n")
	b.WriteString(styles.SeparatorLine(w))
	b.WriteString("\n")

	// Viewport content
	b.WriteString(p.detailVP.View())

	return b.String()
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
