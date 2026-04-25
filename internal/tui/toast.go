package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// ToastDuration is how long the toast stays visible.
const ToastDuration = 1500 * time.Millisecond

// ToastDismissMsg is sent when the toast should auto-dismiss.
type ToastDismissMsg struct{}

var (
	toastContentStyle = lipgloss.NewStyle().Foreground(styles.Text).Padding(0, 2)
	toastBoxStyle     = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(styles.SurfaceAlt).
				Background(styles.Surface)
)

// Toast is a brief notification overlay that auto-dismisses.
type Toast struct {
	text   string
	active bool
}

// Show activates the toast with the given text.
func (t *Toast) Show(text string) tea.Cmd {
	t.text = text
	t.active = true
	return tea.Tick(ToastDuration, func(time.Time) tea.Msg {
		return ToastDismissMsg{}
	})
}

// Dismiss hides the toast.
func (t *Toast) Dismiss() {
	t.active = false
	t.text = ""
}

// IsActive returns whether the toast is visible.
func (t Toast) IsActive() bool {
	return t.active
}

// View renders the toast as a small centered box.
func (t Toast) View() string {
	if !t.active || t.text == "" {
		return ""
	}

	content := toastContentStyle.Render(t.text)
	box := toastBoxStyle.Render(content)

	return box
}
