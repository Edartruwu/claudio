package docks

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/tasks"
	"github.com/Abraxas-365/claudio/internal/tools"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// TodoDock shows active planning tasks inline above the prompt.
type TodoDock struct {
	runtime  *tasks.Runtime
	expanded bool
	width    int
}

// NewTodoDock creates a new TodoDock backed by the given runtime.
func NewTodoDock(rt *tasks.Runtime) *TodoDock {
	return &TodoDock{runtime: rt}
}

// IsActive returns true when there are in-progress planning tasks.
func (d *TodoDock) IsActive() bool {
	if d.runtime == nil {
		return false
	}
	items := tools.GlobalTaskStore.List()
	for _, t := range items {
		if t.Status == "in_progress" || t.Status == "pending" {
			return true
		}
	}
	return false
}

// SetWidth sets the available display width.
func (d *TodoDock) SetWidth(w int) {
	d.width = w
}

// ToggleExpanded flips the dock's expanded state.
func (d *TodoDock) ToggleExpanded() {
	d.expanded = !d.expanded
}

// Height returns the number of lines this dock will render.
func (d *TodoDock) Height() int {
	if d.expanded {
		items := d.planItems()
		h := len(items)
		if h > 6 {
			h = 6
		}
		if h < 1 {
			h = 1
		}
		return h
	}
	return 1
}

// Update handles key events. ctrl+t toggles expanded; returns (nil, true) when consumed.
func (d *TodoDock) Update(msg tea.KeyMsg) (tea.Cmd, bool) {
	if msg.String() == "ctrl+t" {
		d.expanded = !d.expanded
		return nil, true
	}
	return nil, false
}

// View renders the dock content. Returns empty string when there are no tasks.
func (d *TodoDock) View() string {
	items := d.planItems()
	if len(items) == 0 {
		return ""
	}

	bg := lipgloss.NewStyle().
		Foreground(styles.Text).
		Width(d.width)

	label := lipgloss.NewStyle().
		Foreground(styles.Text).
		Bold(true)

	hint := lipgloss.NewStyle().
		Foreground(styles.Warning)

	if d.expanded {
		return d.expandedView(items, bg, label, hint)
	}
	return d.collapsedView(items, bg, label, hint)
}

func (d *TodoDock) collapsedView(items []*tools.Task, bg, label, hint lipgloss.Style) string {

	var parts []string
	shown := 0
	for _, t := range items {
		if shown >= 4 {
			break
		}
		parts = append(parts, taskBadge(t))
		shown++
	}

	extra := len(items) - shown
	var sb strings.Builder
	sb.WriteString("  ")
	sb.WriteString(label.Render("Tasks:"))
	sb.WriteString(" ")
	sb.WriteString(strings.Join(parts, "  "))
	if extra > 0 {
		sb.WriteString("  ")
		sb.WriteString(label.Render(fmt.Sprintf("+%d more", extra)))
	}
	sb.WriteString("  ")
	sb.WriteString(hint.Render("<space>t expand"))

	return bg.Render(sb.String())
}

func (d *TodoDock) expandedView(items []*tools.Task, bg, label, hint lipgloss.Style) string {

	limit := len(items)
	if limit > 6 {
		limit = 6
	}

	var lines []string
	for i := 0; i < limit; i++ {
		t := items[i]
		icon := taskStatusIcon(t)
		maxName := d.width - 8
		if maxName < 5 {
			maxName = 5
		}
		name := t.Subject
		if len(name) > maxName {
			name = name[:maxName-1] + "…"
		}
		line := fmt.Sprintf("  %s  %s", icon, lipgloss.NewStyle().Foreground(styles.Text).Render(name))
		lines = append(lines, bg.Render(line))
	}

	if len(items) > 6 {
		extra := label.Render(fmt.Sprintf("  +%d more  ", len(items)-6)) + hint.Render("<space>t collapse")
		lines = append(lines, bg.Render(extra))
	} else {
		// hint on last line
		lines[len(lines)-1] = bg.Render(
			fmt.Sprintf("  %s  %s  %s",
				taskStatusIcon(items[limit-1]),
				lipgloss.NewStyle().Foreground(styles.Text).Render(items[limit-1].Subject),
				hint.Render("<space>t collapse"),
			),
		)
	}

	return strings.Join(lines, "\n")
}

// taskBadge renders a small [icon name] badge for a task.
func taskBadge(t *tools.Task) string {
	icon := taskStatusIcon(t)
	name := t.Subject
	if len(name) > 12 {
		name = name[:11] + "…"
	}
	return fmt.Sprintf("[%s] %s", icon, lipgloss.NewStyle().Foreground(styles.Dim).Render(name))
}

func taskStatusIcon(t *tools.Task) string {
	switch t.Status {
	case "completed":
		return lipgloss.NewStyle().Foreground(styles.Success).Render("✓")
	case "in_progress":
		return lipgloss.NewStyle().Foreground(styles.Warning).Render("◐")
	case "pending":
		return lipgloss.NewStyle().Foreground(styles.Muted).Render("○")
	default: // failed or unknown
		return lipgloss.NewStyle().Foreground(styles.Error).Render("✗")
	}
}

// planItems returns all non-deleted tasks sorted numerically by ID.
func (d *TodoDock) planItems() []*tools.Task {
	items := tools.GlobalTaskStore.List()
	sort.Slice(items, func(i, j int) bool {
		ni, _ := strconv.Atoi(items[i].ID)
		nj, _ := strconv.Atoi(items[j].ID)
		return ni < nj
	})
	return items
}
