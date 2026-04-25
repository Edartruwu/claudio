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

var (
	todoLabelStyle   = lipgloss.NewStyle().Foreground(styles.Text).Bold(true)
	todoHintStyle    = lipgloss.NewStyle().Foreground(styles.Warning)
	todoTextStyle    = lipgloss.NewStyle().Foreground(styles.Text)
	todoDimStyle     = lipgloss.NewStyle().Foreground(styles.Dim)
	todoIconSuccess  = lipgloss.NewStyle().Foreground(styles.Success).Render("✓")
	todoIconProgress = lipgloss.NewStyle().Foreground(styles.Warning).Render("◐")
	todoIconPending  = lipgloss.NewStyle().Foreground(styles.Muted).Render("○")
	todoIconFailed   = lipgloss.NewStyle().Foreground(styles.Error).Render("✗")
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

// IsActive returns true when there are any non-deleted planning tasks.
func (d *TodoDock) IsActive() bool {
	if d.runtime == nil {
		return false
	}
	return len(tools.GlobalTaskStore.List()) > 0
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

	bg := todoTextStyle.Width(d.width)
	label := todoLabelStyle
	hint := todoHintStyle

	if d.expanded {
		return d.expandedView(items, bg, label, hint)
	}
	return d.collapsedView(items, bg, label, hint)
}

func (d *TodoDock) collapsedView(items []*tools.Task, bg, label, hint lipgloss.Style) string {
	var done, inProgress, pending int
	for _, t := range items {
		switch t.Status {
		case "completed":
			done++
		case "in_progress":
			inProgress++
		default:
			pending++
		}
	}

	var sb strings.Builder
	sb.WriteString("  ")
	sb.WriteString(label.Render("Tasks:"))
	sb.WriteString("  ")
	sb.WriteString(todoTextStyle.Render(fmt.Sprintf("%d/%d done", done, len(items))))
	if inProgress > 0 {
		sb.WriteString("  ")
		sb.WriteString(todoIconProgress + " " + todoDimStyle.Render(fmt.Sprintf("%d running", inProgress)))
	}
	if pending > 0 {
		sb.WriteString("  ")
		sb.WriteString(todoIconPending + " " + todoDimStyle.Render(fmt.Sprintf("%d pending", pending)))
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
		name := t.Title
		if len(name) > maxName {
			name = name[:maxName-1] + "…"
		}
		line := fmt.Sprintf("  %s  %s", icon, todoTextStyle.Render(name))
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
				todoTextStyle.Render(items[limit-1].Title),
				hint.Render("<space>t collapse"),
			),
		)
	}

	return strings.Join(lines, "\n")
}

// taskBadge renders a small [icon name] badge for a task.
func taskBadge(t *tools.Task) string {
	icon := taskStatusIcon(t)
	name := t.Title
	if len(name) > 12 {
		name = name[:11] + "…"
	}
	return fmt.Sprintf("[%s] %s", icon, todoDimStyle.Render(name))
}

func taskStatusIcon(t *tools.Task) string {
	switch t.Status {
	case "completed":
		return todoIconSuccess
	case "in_progress":
		return todoIconProgress
	case "pending":
		return todoIconPending
	default: // failed or unknown
		return todoIconFailed
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
