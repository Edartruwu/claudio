// Package taskspanel implements the unified tasks side panel.
// It shows two tabs: planning tasks (AI-visible work items from GlobalTaskStore)
// and background tasks (running shell/agent/dream processes from tasks.Runtime).
package taskspanel

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/tasks"
	"github.com/Abraxas-365/claudio/internal/tools"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// RefreshMsg triggers a panel refresh (sent by the tick cmd).
type RefreshMsg struct{}

// tab identifies which panel tab is active.
type tab int

const (
	tabPlanning   tab = 0
	tabBackground tab = 1
)

// spinner frames for running tasks.
var spinFrames = []string{"◐", "◓", "●", "◑"}

// Panel is the unified tasks side panel.
type Panel struct {
	runtime *tasks.Runtime

	active bool
	width  int
	height int

	tab    tab
	cursor int
	tick   int // increments on each refresh, drives spinner

	// Planning tab state
	planItems []*tools.Task

	// Background tab state
	bgItems      []*tasks.TaskState
	showOutput   bool
	outputLines  []string
	outputScroll int

	// Detail overlay state (planning tab)
	showDetail      bool
	detailTask      *tools.Task
	detailLines     []string // word-wrapped rendered lines
	detailScroll    int
}

// New creates a new unified tasks panel.
func New(rt *tasks.Runtime) *Panel {
	return &Panel{runtime: rt}
}

func (p *Panel) IsActive() bool { return p.active }

func (p *Panel) Activate() {
	p.active = true
	p.cursor = 0
	p.outputLines = nil
	p.showOutput = false
	p.showDetail = false
	p.detailTask = nil
	p.detailLines = nil
	p.refresh()
}

func (p *Panel) Deactivate() {
	p.active = false
	p.outputLines = nil
	p.showDetail = false
	p.detailTask = nil
	p.detailLines = nil
}

func (p *Panel) SetSize(w, h int) {
	p.width = w
	p.height = h
}

// HandleRefresh is called when a RefreshMsg arrives. Returns a tick cmd if
// any background task is still running.
func (p *Panel) HandleRefresh() tea.Cmd {
	p.tick++
	p.refresh()
	if p.tab == tabBackground && p.showOutput {
		p.loadOutput()
	}
	if p.hasRunningBg() {
		return tickCmd()
	}
	return nil
}

// ScheduleRefresh returns a cmd that immediately starts a refresh tick.
// Call this when the background tab becomes active.
func ScheduleRefresh() tea.Cmd {
	return tickCmd()
}

func tickCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
		return RefreshMsg{}
	})
}

func (p *Panel) refresh() {
	// Planning items — sorted numerically by ID
	items := tools.GlobalTaskStore.List()
	sort.Slice(items, func(i, j int) bool {
		ni, _ := strconv.Atoi(items[i].ID)
		nj, _ := strconv.Atoi(items[j].ID)
		return ni < nj
	})
	p.planItems = items

	// Background items
	if p.runtime != nil {
		p.bgItems = p.runtime.List(false)
	}

	// Clamp cursor
	p.clampCursor()
}

func (p *Panel) clampCursor() {
	n := p.itemCount()
	if n == 0 {
		p.cursor = 0
		return
	}
	if p.cursor >= n {
		p.cursor = n - 1
	}
}

func (p *Panel) itemCount() int {
	if p.tab == tabPlanning {
		return len(p.planItems)
	}
	return len(p.bgItems)
}

func (p *Panel) hasRunningBg() bool {
	for _, t := range p.bgItems {
		if t.Status == tasks.StatusRunning {
			return true
		}
	}
	return false
}

func (p *Panel) loadOutput() {
	if p.tab != tabBackground || p.cursor >= len(p.bgItems) {
		p.outputLines = nil
		return
	}
	t := p.bgItems[p.cursor]
	if t.OutputFile == "" {
		p.outputLines = []string{"(no output file)"}
		return
	}
	f, err := os.Open(t.OutputFile)
	if err != nil {
		p.outputLines = []string{"(output unavailable)"}
		return
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	// Keep last N lines
	const maxLines = 20
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	p.outputLines = lines
	// Auto-scroll to bottom
	p.outputScroll = 0
}

func (p *Panel) Update(msg tea.KeyMsg) (tea.Cmd, bool) {
	// Detail overlay captures all keys
	if p.showDetail {
		switch msg.String() {
		case "esc", "q", "enter":
			p.showDetail = false
			p.detailTask = nil
			p.detailLines = nil
		case "j", "down":
			maxScroll := len(p.detailLines) - (p.height - 6)
			if maxScroll < 0 {
				maxScroll = 0
			}
			if p.detailScroll < maxScroll {
				p.detailScroll++
			}
		case "k", "up":
			if p.detailScroll > 0 {
				p.detailScroll--
			}
		case "g":
			p.detailScroll = 0
		case "G":
			maxScroll := len(p.detailLines) - (p.height - 6)
			if maxScroll < 0 {
				maxScroll = 0
			}
			p.detailScroll = maxScroll
		}
		return nil, true
	}

	switch msg.String() {
	// Tab switching
	case "1":
		if p.tab != tabPlanning {
			p.tab = tabPlanning
			p.cursor = 0
			p.showOutput = false
			p.refresh()
		}
		return nil, true
	case "2":
		if p.tab != tabBackground {
			p.tab = tabBackground
			p.cursor = 0
			p.showOutput = false
			p.refresh()
			if p.hasRunningBg() {
				return tickCmd(), true
			}
		}
		return nil, true
	case "tab":
		if p.tab == tabPlanning {
			p.tab = tabBackground
			p.showOutput = false
			p.refresh()
			if p.hasRunningBg() {
				return tickCmd(), true
			}
		} else {
			p.tab = tabPlanning
			p.showOutput = false
			p.refresh()
		}
		p.cursor = 0
		return nil, true

	// Navigation
	case "j", "down":
		if p.cursor < p.itemCount()-1 {
			p.cursor++
			if p.showOutput {
				p.loadOutput()
			}
		}
		return nil, true
	case "k", "up":
		if p.cursor > 0 {
			p.cursor--
			if p.showOutput {
				p.loadOutput()
			}
		}
		return nil, true
	case "g":
		p.cursor = 0
		if p.showOutput {
			p.loadOutput()
		}
		return nil, true
	case "G":
		p.cursor = max(0, p.itemCount()-1)
		if p.showOutput {
			p.loadOutput()
		}
		return nil, true

	// Actions
	case "x":
		if p.tab == tabBackground && p.cursor < len(p.bgItems) {
			t := p.bgItems[p.cursor]
			if t.Status == tasks.StatusRunning {
				p.runtime.Kill(t.ID)
				p.refresh()
			}
		}
		return nil, true
	case "o":
		if p.tab == tabBackground {
			p.showOutput = !p.showOutput
			if p.showOutput {
				p.loadOutput()
			}
		}
		return nil, true
	case "r":
		p.refresh()
		if p.tab == tabBackground && p.showOutput {
			p.loadOutput()
		}
		if p.tab == tabBackground && p.hasRunningBg() {
			return tickCmd(), true
		}
		return nil, true
	case "enter":
		if p.tab == tabPlanning && p.cursor < len(p.planItems) {
			t := p.planItems[p.cursor]
			p.openDetail(t)
		}
		return nil, true
	}
	return nil, false
}

func (p *Panel) openDetail(t *tools.Task) {
	p.showDetail = true
	p.detailScroll = 0
	p.detailTask = t

	// Build markdown content
	var md strings.Builder
	statusLabel := map[string]string{
		"pending":     "⬜ Pending",
		"in_progress": "🔄 In Progress",
		"completed":   "✅ Completed",
	}[t.Status]
	if statusLabel == "" {
		statusLabel = t.Status
	}
	md.WriteString(fmt.Sprintf("# %s\n\n", t.Subject))
	md.WriteString(fmt.Sprintf("**ID:** `#%s`", t.ID))
	if t.AssignedTo != "" {
		md.WriteString(fmt.Sprintf("  **Assigned to:** `@%s`", t.AssignedTo))
	}
	md.WriteString(fmt.Sprintf("  **Status:** %s\n\n", statusLabel))
	if t.Description != "" {
		md.WriteString("---\n\n")
		md.WriteString(t.Description)
		md.WriteString("\n")
	} else {
		md.WriteString("*No description provided.*\n")
	}

	// Render markdown via glamour
	contentWidth := p.width - 4
	if contentWidth < 10 {
		contentWidth = 10
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStylesFromJSONBytes(styles.GruvboxGlamourJSON()),
		glamour.WithWordWrap(contentWidth),
	)
	rendered := md.String()
	if err == nil {
		if out, err2 := r.Render(rendered); err2 == nil {
			rendered = out
		}
	}

	// Split into lines for scrolling
	p.detailLines = strings.Split(rendered, "\n")
}

func (p *Panel) renderDetail() string {
	var b strings.Builder

	// Header
	titleStyle := lipgloss.NewStyle().Foreground(styles.Primary).Bold(true)
	hintStyle := styles.PanelHint

	b.WriteString(titleStyle.Render("  Task Detail"))
	b.WriteString("\n")
	b.WriteString(styles.SeparatorLine(p.width))
	b.WriteString("\n")

	// Scrollable content area
	contentH := p.height - 5 // header(1) + sep(1) + bottom_sep(1) + hint(1) + padding(1)
	if contentH < 1 {
		contentH = 1
	}

	start := p.detailScroll
	end := start + contentH
	if end > len(p.detailLines) {
		end = len(p.detailLines)
	}

	for i := start; i < end; i++ {
		b.WriteString(p.detailLines[i])
		b.WriteString("\n")
	}

	// Scroll indicator
	totalLines := len(p.detailLines)
	scrollPct := 0
	if totalLines > contentH {
		scrollPct = (p.detailScroll * 100) / (totalLines - contentH)
	} else {
		scrollPct = 100
	}
	scrollInfo := fmt.Sprintf("%d%%", scrollPct)
	if totalLines > contentH {
		scrollInfo += fmt.Sprintf(" (%d/%d)", p.detailScroll+contentH, totalLines)
	}

	b.WriteString(styles.SeparatorLine(p.width))
	b.WriteString("\n")
	hint := fmt.Sprintf("  j/k scroll · g/G top/bot · esc close   %s", scrollInfo)
	b.WriteString(hintStyle.Render(hint))

	return lipgloss.NewStyle().
		Width(p.width).
		Height(p.height).
		Render(b.String())
}

func (p *Panel) View() string {
	if !p.active {
		return ""
	}

	if p.showDetail {
		return p.renderDetail()
	}

	var b strings.Builder

	// ── Tab bar ──────────────────────────────────────────────
	tab1Label := " 1 Planning "
	tab2Label := " 2 Background "
	activeTabStyle := lipgloss.NewStyle().
		Foreground(styles.Primary).
		Bold(true).
		Underline(true)
	inactiveTabStyle := lipgloss.NewStyle().
		Foreground(styles.Muted)

	var tab1, tab2 string
	if p.tab == tabPlanning {
		tab1 = activeTabStyle.Render(tab1Label)
		tab2 = inactiveTabStyle.Render(tab2Label)
	} else {
		tab1 = inactiveTabStyle.Render(tab1Label)
		tab2 = activeTabStyle.Render(tab2Label)
	}

	titleRow := styles.PanelTitle.Render("Tasks") + "  " + tab1 + tab2
	b.WriteString(titleRow)
	b.WriteString("\n")
	b.WriteString(styles.SeparatorLine(p.width))
	b.WriteString("\n")

	if p.tab == tabPlanning {
		p.renderPlanning(&b)
	} else {
		p.renderBackground(&b)
	}

	return lipgloss.NewStyle().
		Width(p.width).
		Height(p.height).
		Render(b.String())
}

// ── Planning tab ──────────────────────────────────────────────────────────────

func (p *Panel) renderPlanning(b *strings.Builder) {
	if len(p.planItems) == 0 {
		b.WriteString(styles.PanelHint.Render("  No planning tasks"))
		b.WriteString("\n")
		p.writeHint(b, "  tab switch · esc close")
		return
	}

	// How much height to give the list vs detail pane
	detailH := 0
	listH := p.height - 5 // header + separator + hint rows
	if listH < 3 {
		listH = 3
	}

	// If a task with description is selected, show detail pane
	var sel *tools.Task
	if p.cursor < len(p.planItems) {
		sel = p.planItems[p.cursor]
	}
	if sel != nil && sel.Description != "" {
		detailH = 4
		listH -= detailH + 1 // +1 for separator
	}
	if listH < 1 {
		listH = 1
	}

	// Scrolling window
	startIdx := 0
	if p.cursor >= listH {
		startIdx = p.cursor - listH + 1
	}
	endIdx := startIdx + listH
	if endIdx > len(p.planItems) {
		endIdx = len(p.planItems)
	}

	for i := startIdx; i < endIdx; i++ {
		t := p.planItems[i]
		selected := i == p.cursor
		b.WriteString(p.renderPlanRow(t, selected))
		b.WriteString("\n")
	}

	// Detail pane
	if sel != nil && sel.Description != "" {
		b.WriteString(styles.SeparatorLine(p.width))
		b.WriteString("\n")
		descTitle := lipgloss.NewStyle().Foreground(styles.Muted).Italic(true).Render("  Description:")
		b.WriteString(descTitle)
		b.WriteString("\n")
		lines := wordWrap(sel.Description, p.width-4)
		shown := 0
		for _, ln := range lines {
			if shown >= detailH-2 {
				break
			}
			b.WriteString(lipgloss.NewStyle().Foreground(styles.Dim).PaddingLeft(4).Render(ln))
			b.WriteString("\n")
			shown++
		}
	}

	b.WriteString("\n")
	p.writeHint(b, "  j/k nav · enter detail · tab switch · r refresh · esc close")
}

func (p *Panel) renderPlanRow(t *tools.Task, selected bool) string {
	prefix := "  "
	if selected {
		prefix = styles.ViewportCursor.Render("▸ ")
	}

	// Status icon + color
	var icon string
	switch t.Status {
	case "in_progress":
		icon = lipgloss.NewStyle().Foreground(styles.Warning).Render("◐")
	case "completed":
		icon = lipgloss.NewStyle().Foreground(styles.Success).Render("●")
	default: // pending
		icon = lipgloss.NewStyle().Foreground(styles.Dim).Render("○")
	}

	// ID badge
	idBadge := lipgloss.NewStyle().Foreground(styles.Muted).Render(fmt.Sprintf("#%s", t.ID))

	// Subject (truncated)
	maxSubject := p.width - 14
	if maxSubject < 5 {
		maxSubject = 5
	}
	subject := t.Subject
	if len(subject) > maxSubject {
		subject = subject[:maxSubject-1] + "…"
	}
	nameStyle := styles.PanelItem
	if selected {
		nameStyle = styles.PanelItemSelected
	}
	subjectStr := nameStyle.Render(subject)

	// Assignee badge
	ownerStr := ""
	if t.AssignedTo != "" {
		ownerStr = " " + lipgloss.NewStyle().Foreground(styles.Aqua).Render("@"+t.AssignedTo)
	}

	return prefix + icon + " " + idBadge + " " + subjectStr + ownerStr
}

// ── Background tab ────────────────────────────────────────────────────────────

func (p *Panel) renderBackground(b *strings.Builder) {
	if len(p.bgItems) == 0 {
		b.WriteString(styles.PanelHint.Render("  No background tasks"))
		b.WriteString("\n")
		p.writeHint(b, "  tab switch · esc close")
		return
	}

	// Split height between list and output pane
	outputPaneH := 0
	listH := p.height - 5
	if p.showOutput {
		outputPaneH = 8
		listH -= outputPaneH + 1
	}
	if listH < 2 {
		listH = 2
	}

	// Scrolling window
	startIdx := 0
	if p.cursor >= listH {
		startIdx = p.cursor - listH + 1
	}
	endIdx := startIdx + listH
	if endIdx > len(p.bgItems) {
		endIdx = len(p.bgItems)
	}

	for i := startIdx; i < endIdx; i++ {
		t := p.bgItems[i]
		selected := i == p.cursor
		b.WriteString(p.renderBgRow(t, selected))
		b.WriteString("\n")
	}

	// Output pane
	if p.showOutput {
		b.WriteString(styles.SeparatorLine(p.width))
		b.WriteString("\n")
		outTitle := lipgloss.NewStyle().Foreground(styles.Muted).Italic(true).Render("  Output:")
		b.WriteString(outTitle)
		b.WriteString("\n")
		shown := 0
		for _, ln := range p.outputLines {
			if shown >= outputPaneH-2 {
				break
			}
			if len(ln) > p.width-4 {
				ln = ln[:p.width-7] + "..."
			}
			b.WriteString(lipgloss.NewStyle().Foreground(styles.Dim).PaddingLeft(2).Render("> " + ln))
			b.WriteString("\n")
			shown++
		}
		if len(p.outputLines) == 0 {
			b.WriteString(styles.PanelHint.Render("    (no output yet)"))
			b.WriteString("\n")
		}
	}

	// Running count footer
	running := 0
	for _, t := range p.bgItems {
		if t.Status == tasks.StatusRunning {
			running++
		}
	}
	b.WriteString("\n")
	if running > 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(styles.Warning).PaddingLeft(2).
			Render(fmt.Sprintf("%d running", running)))
		b.WriteString("\n")
	}

	hint := "  j/k · x kill · o output · tab switch · esc close"
	p.writeHint(b, hint)
}

func (p *Panel) renderBgRow(t *tasks.TaskState, selected bool) string {
	prefix := "  "
	if selected {
		prefix = styles.ViewportCursor.Render("▸ ")
	}

	// Status icon — animated spinner for running
	var icon string
	switch t.Status {
	case tasks.StatusRunning:
		frame := spinFrames[p.tick%len(spinFrames)]
		icon = lipgloss.NewStyle().Foreground(styles.Warning).Render(frame)
	case tasks.StatusCompleted:
		icon = lipgloss.NewStyle().Foreground(styles.Success).Render("●")
	case tasks.StatusFailed:
		icon = lipgloss.NewStyle().Foreground(styles.Error).Render("✗")
	case tasks.StatusKilled:
		icon = lipgloss.NewStyle().Foreground(styles.Muted).Render("⊘")
	default:
		icon = lipgloss.NewStyle().Foreground(styles.Dim).Render("○")
	}

	// Type badge
	var typeBadge string
	switch t.Type {
	case tasks.TypeShell:
		typeBadge = lipgloss.NewStyle().Foreground(styles.Aqua).Render("[bash]")
	case tasks.TypeAgent:
		typeBadge = lipgloss.NewStyle().Foreground(styles.Primary).Render("[agent]")
	case tasks.TypeDream:
		typeBadge = lipgloss.NewStyle().Foreground(styles.Orange).Render("[dream]")
	default:
		typeBadge = lipgloss.NewStyle().Foreground(styles.Dim).Render("[task]")
	}

	// Description
	desc := t.Description
	if desc == "" {
		desc = t.Command
	}
	if desc == "" {
		desc = t.ID
	}
	maxDesc := p.width - 22
	if maxDesc < 5 {
		maxDesc = 5
	}
	if len(desc) > maxDesc {
		desc = desc[:maxDesc-1] + "…"
	}
	nameStyle := styles.PanelItem
	if selected {
		nameStyle = styles.PanelItemSelected
	}
	descStr := nameStyle.Render(desc)

	// Duration — smarter format
	dur := smartDuration(t.StartTime, t.EndTime)
	durStr := lipgloss.NewStyle().Foreground(styles.Dim).Render(dur)

	row := prefix + icon + " " + typeBadge + " " + descStr + " " + durStr

	// Exit code for failed bash tasks
	if selected && t.ExitCode != nil && t.Status == tasks.StatusFailed {
		exitStr := lipgloss.NewStyle().Foreground(styles.Error).
			Render(fmt.Sprintf(" (exit %d)", *t.ExitCode))
		row += exitStr
	}
	// Error detail inline for selected
	if selected && t.Error != "" {
		errLine := "\n    " + styles.ErrorStyle.Render(truncate(t.Error, p.width-6))
		row += errLine
	}

	return row
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (p *Panel) writeHint(b *strings.Builder, hint string) {
	b.WriteString(styles.SeparatorLine(p.width))
	b.WriteString("\n")
	b.WriteString(styles.PanelHint.Render(hint))
}

func smartDuration(start time.Time, end *time.Time) string {
	var d time.Duration
	if end != nil {
		d = end.Sub(start)
	} else {
		d = time.Since(start)
	}
	d = d.Truncate(time.Second)
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

func wordWrap(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	var lines []string
	for _, paragraph := range strings.Split(text, "\n") {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		current := ""
		for _, w := range words {
			if current == "" {
				current = w
			} else if len(current)+1+len(w) <= width {
				current += " " + w
			} else {
				lines = append(lines, current)
				current = w
			}
		}
		if current != "" {
			lines = append(lines, current)
		}
	}
	return lines
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Help returns a short keybinding hint line for the panel footer.
func (p *Panel) Help() string {
	if p.tab == tabPlanning {
		return "j/k navigate · enter detail · tab switch · r refresh · esc close"
	}
	return "j/k navigate · x kill · o output · tab switch · r refresh · esc close"
}
