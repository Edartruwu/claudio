// Package taskspanel implements the background tasks side panel.
package taskspanel

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/tasks"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// RefreshMsg triggers a panel refresh.
type RefreshMsg struct{}

// Panel is the background tasks side panel.
type Panel struct {
	runtime *tasks.Runtime

	active bool
	width  int
	height int
	cursor int
	items  []*tasks.TaskState
}

// New creates a new background tasks panel.
func New(rt *tasks.Runtime) *Panel {
	return &Panel{runtime: rt}
}

func (p *Panel) IsActive() bool { return p.active }

func (p *Panel) Activate() {
	p.active = true
	p.cursor = 0
	p.refresh()
}

func (p *Panel) Deactivate() { p.active = false }

func (p *Panel) SetSize(w, h int) {
	p.width = w
	p.height = h
}

func (p *Panel) refresh() {
	p.items = p.runtime.List(false) // all tasks, not just running
	if p.cursor >= len(p.items) {
		p.cursor = max(0, len(p.items)-1)
	}
}

// ScheduleRefresh returns a cmd that sends RefreshMsg after 2 seconds.
func ScheduleRefresh() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return RefreshMsg{}
	})
}

func (p *Panel) Update(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch msg.String() {
	case "j", "down":
		if p.cursor < len(p.items)-1 {
			p.cursor++
		}
		return nil, true
	case "k", "up":
		if p.cursor > 0 {
			p.cursor--
		}
		return nil, true
	case "x":
		if p.cursor < len(p.items) {
			t := p.items[p.cursor]
			if t.Status == tasks.StatusRunning {
				p.runtime.Kill(t.ID)
				p.refresh()
			}
		}
		return nil, true
	case "r":
		p.refresh()
		return nil, true
	}
	return nil, false
}

func (p *Panel) View() string {
	if !p.active {
		return ""
	}

	var b strings.Builder

	title := styles.PanelTitle.Render("Background Tasks")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(styles.SeparatorLine(p.width))
	b.WriteString("\n")

	if len(p.items) == 0 {
		b.WriteString(styles.PanelHint.Render("  No background tasks"))
		b.WriteString("\n")
	}

	listH := p.height - 5
	if listH < 3 {
		listH = 3
	}

	startIdx := 0
	if p.cursor >= listH {
		startIdx = p.cursor - listH + 1
	}
	endIdx := startIdx + listH
	if endIdx > len(p.items) {
		endIdx = len(p.items)
	}

	for i := startIdx; i < endIdx; i++ {
		t := p.items[i]
		selected := i == p.cursor

		prefix := "  "
		if selected {
			prefix = styles.ViewportCursor.Render("▸ ")
		}

		// Status icon
		var icon string
		var iconStyle lipgloss.Style
		switch t.Status {
		case tasks.StatusRunning:
			iconStyle = lipgloss.NewStyle().Foreground(styles.Warning)
			icon = iconStyle.Render("◐")
		case tasks.StatusCompleted:
			iconStyle = lipgloss.NewStyle().Foreground(styles.Success)
			icon = iconStyle.Render("●")
		case tasks.StatusFailed:
			iconStyle = lipgloss.NewStyle().Foreground(styles.Error)
			icon = iconStyle.Render("✗")
		case tasks.StatusKilled:
			iconStyle = lipgloss.NewStyle().Foreground(styles.Muted)
			icon = iconStyle.Render("⊘")
		default:
			iconStyle = lipgloss.NewStyle().Foreground(styles.Dim)
			icon = iconStyle.Render("○")
		}

		// Description
		desc := t.Description
		if desc == "" {
			desc = t.ID
		}
		if len(desc) > p.width-15 {
			desc = desc[:p.width-18] + "..."
		}

		nameStyle := styles.PanelItem
		if selected {
			nameStyle = styles.PanelItemSelected
		}

		// Duration
		dur := time.Since(t.StartTime).Truncate(time.Second).String()
		durStyle := lipgloss.NewStyle().Foreground(styles.Dim)

		line := prefix + icon + " " + nameStyle.Render(desc) + " " + durStyle.Render(dur)
		b.WriteString(line)
		b.WriteString("\n")

		// Error detail for failed tasks
		if selected && t.Error != "" {
			errLine := "    " + styles.ErrorStyle.Render(t.Error)
			b.WriteString(errLine)
			b.WriteString("\n")
		}
	}

	// Running count
	running := 0
	for _, t := range p.items {
		if t.Status == tasks.StatusRunning {
			running++
		}
	}
	if running > 0 {
		b.WriteString("\n")
		runStyle := lipgloss.NewStyle().Foreground(styles.Warning)
		b.WriteString(runStyle.Render(fmt.Sprintf("  %d running", running)))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.PanelHint.Render("  x kill · r refresh · esc close"))

	return lipgloss.NewStyle().
		Width(p.width).
		Height(p.height).
		Render(b.String())
}
