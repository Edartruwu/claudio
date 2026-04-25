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

// focusedPane identifies which pane has keyboard focus.
type focusedPane int

const (
	paneList   focusedPane = 0
	paneDetail focusedPane = 1
)

// spinner frames for running tasks.
var spinFrames = []string{"◐", "◓", "●", "◑"}

// Pre-allocated styles to avoid per-frame allocations.
var (
	tpPrimaryBold  = lipgloss.NewStyle().Foreground(styles.Primary).Bold(true)
	tpDimStyle     = lipgloss.NewStyle().Foreground(styles.Dim)
	tpDimItalic    = lipgloss.NewStyle().Foreground(styles.Dim).Italic(true)
	tpMutedStyle   = lipgloss.NewStyle().Foreground(styles.Muted)
	tpMutedItalic  = lipgloss.NewStyle().Foreground(styles.Muted).Italic(true)
	tpWarningStyle = lipgloss.NewStyle().Foreground(styles.Warning)
	tpSuccessStyle = lipgloss.NewStyle().Foreground(styles.Success)
	tpErrorStyle   = lipgloss.NewStyle().Foreground(styles.Error)
	tpAquaStyle    = lipgloss.NewStyle().Foreground(styles.Aqua)
	tpOrangeStyle  = lipgloss.NewStyle().Foreground(styles.Orange)
	tpBorderBase   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
)

// Panel is the unified tasks panel with full-screen split layout.
type Panel struct {
	runtime *tasks.Runtime

	active bool
	width  int
	height int

	tab    tab
	cursor int
	tick   int // increments on each refresh, drives spinner
	focus  focusedPane

	// Planning tab state
	planItems []*tools.Task

	// Background tab state
	bgItems []*tasks.TaskState

	// Right pane: rendered detail lines + scroll
	detailLines  []string
	detailScroll int
}

// New creates a new unified tasks panel.
func New(rt *tasks.Runtime) *Panel {
	return &Panel{runtime: rt}
}

func (p *Panel) IsActive() bool { return p.active }

func (p *Panel) Activate() {
	p.active = true
	p.cursor = 0
	p.focus = paneList
	p.detailLines = nil
	p.detailScroll = 0
	p.refresh()
}

func (p *Panel) Deactivate() {
	p.active = false
	p.detailLines = nil
}

func (p *Panel) SetSize(w, h int) {
	p.width = w
	p.height = h
}

// HandleRefresh is called when a RefreshMsg arrives.
func (p *Panel) HandleRefresh() tea.Cmd {
	p.tick++
	p.refresh()
	if p.hasRunningBg() {
		return tickCmd()
	}
	return nil
}

// ScheduleRefresh returns a cmd that immediately starts a refresh tick.
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

	p.clampCursor()
	p.buildDetailLines()
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

// buildDetailLines populates p.detailLines for the currently selected item.
func (p *Panel) buildDetailLines() {
	if p.tab == tabPlanning {
		if p.cursor >= len(p.planItems) {
			p.detailLines = nil
			return
		}
		p.detailLines = p.buildPlanDetail(p.planItems[p.cursor])
	} else {
		if p.cursor >= len(p.bgItems) {
			p.detailLines = nil
			return
		}
		p.detailLines = p.buildBgDetail(p.bgItems[p.cursor])
	}
}

func (p *Panel) buildPlanDetail(t *tools.Task) []string {
	var md strings.Builder
	statusLabel := map[string]string{
		"pending":     "⬜ Pending",
		"in_progress": "🔄 In Progress",
		"completed":   "✅ Completed",
	}[t.Status]
	if statusLabel == "" {
		statusLabel = t.Status
	}
	md.WriteString(fmt.Sprintf("# %s\n\n", t.Title))
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

	contentWidth := p.rightPaneWidth() - 4
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
	return strings.Split(rendered, "\n")
}

func (p *Panel) buildBgDetail(t *tasks.TaskState) []string {
	var b strings.Builder

	// Header section
	b.WriteString(tpPrimaryBold.Render(t.Description))
	b.WriteString("\n\n")

	// Metadata rows
	typeLabel := map[tasks.TaskType]string{
		tasks.TypeShell: "bash",
		tasks.TypeAgent: "agent",
		tasks.TypeDream: "dream",
	}[t.Type]
	if typeLabel == "" {
		typeLabel = "task"
	}
	b.WriteString(tpMutedStyle.Render("Type:   ") + tpAquaStyle.Render(typeLabel) + "\n")
	b.WriteString(tpMutedStyle.Render("ID:     ") + tpDimStyle.Render(t.ID) + "\n")

	statusColor := statusStyleBg(t.Status)
	b.WriteString(tpMutedStyle.Render("Status: ") + statusColor.Render(string(t.Status)) + "\n")

	dur := smartDuration(t.StartTime, t.EndTime)
	b.WriteString(tpMutedStyle.Render("Time:   ") + tpDimStyle.Render(dur) + "\n")

	if t.ExitCode != nil {
		exitStr := fmt.Sprintf("%d", *t.ExitCode)
		exitStyle := tpSuccessStyle
		if *t.ExitCode != 0 {
			exitStyle = tpErrorStyle
		}
		b.WriteString(tpMutedStyle.Render("Exit:   ") + exitStyle.Render(exitStr) + "\n")
	}
	if t.Error != "" {
		b.WriteString("\n" + tpErrorStyle.Render("Error:") + "\n")
		b.WriteString(tpDimStyle.Render(t.Error) + "\n")
	}
	if t.Command != "" {
		b.WriteString("\n" + tpMutedItalic.Render("Command:") + "\n")
		b.WriteString(tpDimStyle.Render(t.Command) + "\n")
	}

	// Output section
	b.WriteString("\n" + tpMutedItalic.Render("Output:") + "\n")
	b.WriteString(strings.Repeat("─", p.rightPaneWidth()-4) + "\n")

	if t.OutputFile != "" {
		f, err := os.Open(t.OutputFile)
		if err == nil {
			defer f.Close()
			var lines []string
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				lines = append(lines, scanner.Text())
			}
			if len(lines) == 0 {
				b.WriteString(tpMutedStyle.Render("(no output yet)") + "\n")
			} else {
				for _, ln := range lines {
					if len(ln) > p.rightPaneWidth()-6 {
						ln = ln[:p.rightPaneWidth()-9] + "..."
					}
					b.WriteString(tpDimStyle.Render("> "+ln) + "\n")
				}
			}
		} else {
			b.WriteString(tpMutedStyle.Render("(output unavailable)") + "\n")
		}
	} else {
		b.WriteString(tpMutedStyle.Render("(no output file)") + "\n")
	}

	return strings.Split(b.String(), "\n")
}

func statusStyleBg(s tasks.TaskStatus) lipgloss.Style {
	switch s {
	case tasks.StatusRunning:
		return tpWarningStyle
	case tasks.StatusCompleted:
		return tpSuccessStyle
	case tasks.StatusFailed:
		return tpErrorStyle
	case tasks.StatusKilled:
		return tpMutedStyle
	default:
		return tpDimStyle
	}
}

// ── Key handling ──────────────────────────────────────────────────────────────

func (p *Panel) Update(msg tea.KeyMsg) (tea.Cmd, bool) {
	if p.focus == paneDetail {
		return p.updateDetail(msg)
	}
	return p.updateList(msg)
}

func (p *Panel) updateList(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch msg.String() {
	case "1":
		if p.tab != tabPlanning {
			p.tab = tabPlanning
			p.cursor = 0
			p.detailScroll = 0
			p.refresh()
		}
		return nil, true
	case "2":
		if p.tab != tabBackground {
			p.tab = tabBackground
			p.cursor = 0
			p.detailScroll = 0
			p.refresh()
			if p.hasRunningBg() {
				return tickCmd(), true
			}
		}
		return nil, true
	case "tab":
		if p.tab == tabPlanning {
			p.tab = tabBackground
			p.refresh()
			if p.hasRunningBg() {
				return tickCmd(), true
			}
		} else {
			p.tab = tabPlanning
			p.refresh()
		}
		p.cursor = 0
		p.detailScroll = 0
		return nil, true

	case "j", "down":
		if p.cursor < p.itemCount()-1 {
			p.cursor++
			p.detailScroll = 0
			p.buildDetailLines()
		}
		return nil, true
	case "k", "up":
		if p.cursor > 0 {
			p.cursor--
			p.detailScroll = 0
			p.buildDetailLines()
		}
		return nil, true
	case "g":
		p.cursor = 0
		p.detailScroll = 0
		p.buildDetailLines()
		return nil, true
	case "G":
		p.cursor = max(0, p.itemCount()-1)
		p.detailScroll = 0
		p.buildDetailLines()
		return nil, true

	case "l", "enter", "right":
		// Move focus to detail pane
		if p.itemCount() > 0 {
			p.focus = paneDetail
		}
		return nil, true

	case "x":
		if p.tab == tabBackground && p.cursor < len(p.bgItems) {
			t := p.bgItems[p.cursor]
			if t.Status == tasks.StatusRunning {
				p.runtime.Kill(t.ID)
				p.refresh()
			}
		}
		return nil, true
	case "r":
		p.refresh()
		if p.tab == tabBackground && p.hasRunningBg() {
			return tickCmd(), true
		}
		return nil, true
	}
	return nil, false
}

func (p *Panel) updateDetail(msg tea.KeyMsg) (tea.Cmd, bool) {
	detailH := p.detailContentHeight()
	maxScroll := len(p.detailLines) - detailH
	if maxScroll < 0 {
		maxScroll = 0
	}
	switch msg.String() {
	case "esc", "q", "h", "left":
		p.focus = paneList
	case "j", "down":
		if p.detailScroll < maxScroll {
			p.detailScroll++
		}
	case "k", "up":
		if p.detailScroll > 0 {
			p.detailScroll--
		}
	case "ctrl+d":
		p.detailScroll += detailH / 2
		if p.detailScroll > maxScroll {
			p.detailScroll = maxScroll
		}
	case "ctrl+u":
		p.detailScroll -= detailH / 2
		if p.detailScroll < 0 {
			p.detailScroll = 0
		}
	case "g":
		p.detailScroll = 0
	case "G":
		p.detailScroll = maxScroll
	default:
		return nil, false
	}
	return nil, true
}

// ── Layout helpers ────────────────────────────────────────────────────────────

func (p *Panel) leftPaneWidth() int {
	w := p.width * 30 / 100
	if w < 22 {
		w = 22
	}
	if w > p.width-40 {
		w = p.width - 40
	}
	return w
}

func (p *Panel) rightPaneWidth() int {
	// total = left(+2 border) + right(+2 border)
	rw := p.width - p.leftPaneWidth() - 4
	if rw < 20 {
		rw = 20
	}
	return rw
}

func (p *Panel) innerHeight() int {
	h := p.height - 2 // subtract border top+bottom
	if h < 2 {
		h = 2
	}
	return h
}

func (p *Panel) detailContentHeight() int {
	// innerHeight minus title(1) + sep(1) + hint(1) rows
	h := p.innerHeight() - 3
	if h < 1 {
		h = 1
	}
	return h
}

func (p *Panel) borderStyle(active bool) lipgloss.Style {
	s := tpBorderBase
	if active {
		return s.BorderForeground(styles.Primary)
	}
	return s.BorderForeground(styles.Muted)
}

// ── View ──────────────────────────────────────────────────────────────────────

func (p *Panel) View() string {
	if !p.active {
		return ""
	}

	leftPane := p.renderLeftPane()
	rightPane := p.renderRightPane()

	innerH := p.innerHeight()
	leftStyled := p.borderStyle(p.focus == paneList).
		Width(p.leftPaneWidth()).Height(innerH).Render(leftPane)

	rw := p.rightPaneWidth()
	rightStyled := p.borderStyle(p.focus == paneDetail).
		Width(rw).Height(innerH).Render(rightPane)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftStyled, rightStyled)
}

// renderLeftPane renders the task list pane.
func (p *Panel) renderLeftPane() string {
	w := p.leftPaneWidth() - 4 // subtract border
	var b strings.Builder

	// Title + tab bar
	titleStr := styles.PanelTitle.Render("TASKS")
	b.WriteString(titleStr)
	b.WriteString("\n")
	b.WriteString(styles.SeparatorLine(w))
	b.WriteString("\n")

	// Tabs
	activeTab := lipgloss.NewStyle().Foreground(styles.Primary).Bold(true).Underline(true)
	inactiveTab := lipgloss.NewStyle().Foreground(styles.Muted)
	tab1, tab2 := inactiveTab.Render("1 Plan"), inactiveTab.Render("2 BG")
	if p.tab == tabPlanning {
		tab1 = activeTab.Render("1 Plan")
	} else {
		tab2 = activeTab.Render("2 BG")
	}
	b.WriteString(tab1 + "  " + tab2)
	b.WriteString("\n")
	b.WriteString(styles.SeparatorLine(w))
	b.WriteString("\n")

	// List
	listH := p.innerHeight() - 5 // title + sep + tabs + sep + hint
	if listH < 1 {
		listH = 1
	}

	var items int
	if p.tab == tabPlanning {
		items = len(p.planItems)
	} else {
		items = len(p.bgItems)
	}

	if items == 0 {
		b.WriteString(tpMutedStyle.Render("  (empty)"))
		b.WriteString("\n")
	} else {
		startIdx := 0
		if p.cursor >= listH {
			startIdx = p.cursor - listH + 1
		}
		endIdx := startIdx + listH
		if endIdx > items {
			endIdx = items
		}
		if p.tab == tabPlanning {
			for i := startIdx; i < endIdx; i++ {
				b.WriteString(p.renderPlanRow(p.planItems[i], i == p.cursor, w))
				b.WriteString("\n")
			}
		} else {
			for i := startIdx; i < endIdx; i++ {
				b.WriteString(p.renderBgRow(p.bgItems[i], i == p.cursor, w))
				b.WriteString("\n")
			}
		}
	}

	// Running count for BG tab
	if p.tab == tabBackground {
		running := 0
		for _, t := range p.bgItems {
			if t.Status == tasks.StatusRunning {
				running++
			}
		}
		if running > 0 {
			b.WriteString("\n" + tpWarningStyle.Render(fmt.Sprintf("  %d running", running)) + "\n")
		}
	}

	return b.String()
}

// renderRightPane renders the detail pane for the selected item.
func (p *Panel) renderRightPane() string {
	w := p.rightPaneWidth() - 4 // subtract border
	var b strings.Builder

	// Header
	b.WriteString(tpPrimaryBold.Render("  Detail"))
	b.WriteString("\n")
	b.WriteString(styles.SeparatorLine(w))
	b.WriteString("\n")

	if p.itemCount() == 0 || len(p.detailLines) == 0 {
		b.WriteString(tpMutedStyle.Render("  Select a task"))
		return b.String()
	}

	contentH := p.detailContentHeight()
	start := p.detailScroll
	end := start + contentH
	if end > len(p.detailLines) {
		end = len(p.detailLines)
	}
	for i := start; i < end; i++ {
		b.WriteString(p.detailLines[i])
		b.WriteString("\n")
	}

	// Scroll hint
	totalLines := len(p.detailLines)
	if totalLines > contentH {
		pct := p.detailScroll * 100 / (totalLines - contentH)
		b.WriteString("\n" + styles.SeparatorLine(w) + "\n")
		b.WriteString(tpMutedStyle.Render(fmt.Sprintf("  j/k scroll · g/G top/bot   %d%%", pct)))
	}

	return b.String()
}

// ── Row renderers ─────────────────────────────────────────────────────────────

func (p *Panel) renderPlanRow(t *tools.Task, selected bool, w int) string {
	prefix := "  "
	if selected {
		prefix = styles.ViewportCursor.Render("▸ ")
	}

	var icon string
	switch t.Status {
	case "in_progress":
		icon = tpWarningStyle.Render("◐")
	case "completed":
		icon = tpSuccessStyle.Render("●")
	default:
		icon = tpDimStyle.Render("○")
	}

	idBadge := tpMutedStyle.Render(fmt.Sprintf("#%s", t.ID))

	maxSubject := w - 12
	if maxSubject < 5 {
		maxSubject = 5
	}
	subject := t.Title
	if len(subject) > maxSubject {
		subject = subject[:maxSubject-1] + "…"
	}
	nameStyle := styles.PanelItem
	if selected {
		nameStyle = styles.PanelItemSelected
	}

	row := prefix + icon + " " + idBadge + " " + nameStyle.Render(subject)
	if t.AssignedTo != "" {
		assignee := tpAquaStyle.Render("@" + t.AssignedTo)
		if len(row)+len(t.AssignedTo)+2 <= w {
			row += " " + assignee
		}
	}
	return row
}

func (p *Panel) renderBgRow(t *tasks.TaskState, selected bool, w int) string {
	prefix := "  "
	if selected {
		prefix = styles.ViewportCursor.Render("▸ ")
	}

	var icon string
	switch t.Status {
	case tasks.StatusRunning:
		frame := spinFrames[p.tick%len(spinFrames)]
		icon = tpWarningStyle.Render(frame)
	case tasks.StatusCompleted:
		icon = tpSuccessStyle.Render("●")
	case tasks.StatusFailed:
		icon = tpErrorStyle.Render("✗")
	case tasks.StatusKilled:
		icon = tpMutedStyle.Render("⊘")
	default:
		icon = tpDimStyle.Render("○")
	}

	var typeBadge string
	switch t.Type {
	case tasks.TypeShell:
		typeBadge = tpAquaStyle.Render("[sh]")
	case tasks.TypeAgent:
		typeBadge = lipgloss.NewStyle().Foreground(styles.Primary).Render("[ag]")
	case tasks.TypeDream:
		typeBadge = tpOrangeStyle.Render("[dr]")
	default:
		typeBadge = tpDimStyle.Render("[--]")
	}

	desc := t.Description
	if desc == "" {
		desc = t.Command
	}
	if desc == "" {
		desc = t.ID
	}
	maxDesc := w - 16
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

	dur := smartDuration(t.StartTime, t.EndTime)
	return prefix + icon + " " + typeBadge + " " + nameStyle.Render(desc) + " " + tpDimStyle.Render(dur)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Help returns a short keybinding hint line for the panel footer.
func (p *Panel) Help() string {
	if p.focus == paneDetail {
		return "j/k scroll · ctrl+d/u half-page · g/G top/bot · h/esc back"
	}
	if p.tab == tabPlanning {
		return "j/k nav · l/enter detail · 1/2 tab · r refresh · esc close"
	}
	return "j/k nav · l/enter detail · x kill · 1/2 tab · r refresh · esc close"
}
