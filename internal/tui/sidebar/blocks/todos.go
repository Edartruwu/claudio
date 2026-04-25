package blocks

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/tools"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

var (
	todoDoneStyle   = lipgloss.NewStyle().Foreground(styles.Muted).Strikethrough(true)
	todoActiveStyle = lipgloss.NewStyle().Foreground(styles.Primary).Bold(true)
	todoPendStyle   = lipgloss.NewStyle().Foreground(styles.Text)
	todoDimStyle    = lipgloss.NewStyle().Foreground(styles.Muted)
	todoEmptyStyle  = lipgloss.NewStyle().Foreground(styles.Muted)
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
		return todoEmptyStyle.Render("  No tasks")
	}

	maxLabelW := width - 5
	if maxLabelW < 8 {
		maxLabelW = 8
	}

	var lines []string
	for i, t := range items {
		if i >= maxHeight {
			lines = append(lines, todoDimStyle.Render(fmt.Sprintf("  +%d more", len(items)-i)))
			break
		}
		label := t.Title
		if len(label) > maxLabelW {
			label = label[:maxLabelW-1] + "…"
		}
		var row string
		switch t.Status {
		case "completed", "done":
			row = " ✓ " + todoDoneStyle.Render(label)
		case "in_progress":
			row = " ● " + todoActiveStyle.Render(label)
		default:
			row = " ○ " + todoPendStyle.Render(label)
		}
		lines = append(lines, row)
	}
	return strings.Join(lines, "\n")
}
