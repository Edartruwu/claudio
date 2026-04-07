package blocks

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/tools"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// TodosBlock shows the current task list from the global task store.
type TodosBlock struct{}

func NewTodosBlock() *TodosBlock { return &TodosBlock{} }

func (b *TodosBlock) Title() string  { return "Tasks" }
func (b *TodosBlock) MinHeight() int { return 1 }
func (b *TodosBlock) Weight() int    { return 2 }

func (b *TodosBlock) Render(width, maxHeight int) string {
	items := tools.GlobalTaskStore.List()
	if len(items) == 0 {
		return lipgloss.NewStyle().Foreground(styles.Muted).Render("  No tasks")
	}

	doneStyle := lipgloss.NewStyle().Foreground(styles.Muted).Strikethrough(true)
	activeStyle := lipgloss.NewStyle().Foreground(styles.Primary).Bold(true)
	pendStyle := lipgloss.NewStyle().Foreground(styles.Text)
	dimStyle := lipgloss.NewStyle().Foreground(styles.Muted)

	maxLabelW := width - 5
	if maxLabelW < 8 {
		maxLabelW = 8
	}

	var lines []string
	for i, t := range items {
		if i >= maxHeight {
			lines = append(lines, dimStyle.Render(fmt.Sprintf("  +%d more", len(items)-i)))
			break
		}
		label := t.Subject
		if len(label) > maxLabelW {
			label = label[:maxLabelW-1] + "…"
		}
		var row string
		switch t.Status {
		case "completed", "done":
			row = " ✓ " + doneStyle.Render(label)
		case "in_progress":
			row = " ● " + activeStyle.Render(label)
		default:
			row = " ○ " + pendStyle.Render(label)
		}
		lines = append(lines, row)
	}
	return strings.Join(lines, "\n")
}
