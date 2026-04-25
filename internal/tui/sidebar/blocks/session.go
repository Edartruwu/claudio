package blocks

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

var (
	sessTitleStyle   = lipgloss.NewStyle().Foreground(styles.Text).Bold(true)
	sessMetaStyle    = lipgloss.NewStyle().Foreground(styles.Muted)
	sessElapsedStyle = lipgloss.NewStyle().Foreground(styles.Primary)
	sessEmptyStyle   = lipgloss.NewStyle().Foreground(styles.Muted)
)

// SessionBlock shows the current session name, message count, and elapsed time.
type SessionBlock struct {
	GetTitle    func() string
	GetMsgCount func() int
	GetStart    func() time.Time
}

func NewSessionBlock(getTitle func() string, getMsgCount func() int, getStart func() time.Time) *SessionBlock {
	return &SessionBlock{
		GetTitle:    getTitle,
		GetMsgCount: getMsgCount,
		GetStart:    getStart,
	}
}

func (b *SessionBlock) Title() string  { return "Session" }
func (b *SessionBlock) MinHeight() int { return 2 }
func (b *SessionBlock) Weight() int    { return 1 }

func (b *SessionBlock) Render(width, maxHeight int) string {
	title := ""
	if b.GetTitle != nil {
		title = b.GetTitle()
	}
	msgCount := 0
	if b.GetMsgCount != nil {
		msgCount = b.GetMsgCount()
	}

	var elapsed string
	if b.GetStart != nil {
		t := b.GetStart()
		if !t.IsZero() {
			elapsed = formatElapsed(time.Since(t))
		}
	}

	if title == "" && msgCount == 0 {
		return sessEmptyStyle.Render("  No active session")
	}

	maxTitleW := width - 3
	if maxTitleW < 4 {
		maxTitleW = 4
	}
	if len(title) > maxTitleW {
		title = title[:maxTitleW-1] + "…"
	}

	var lines []string

	if title != "" {
		lines = append(lines, " "+sessTitleStyle.Render(title))
	}

	if maxHeight >= 2 {
		var meta []string
		if msgCount > 0 {
			meta = append(meta, sessMetaStyle.Render(fmt.Sprintf("%d msgs", msgCount)))
		}
		if elapsed != "" {
			meta = append(meta, sessElapsedStyle.Render(elapsed))
		}
		if len(meta) > 0 {
			lines = append(lines, " "+strings.Join(meta, sessMetaStyle.Render("  ")))
		}
	}

	if len(lines) > maxHeight {
		lines = lines[:maxHeight]
	}
	return strings.Join(lines, "\n")
}

func formatElapsed(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		if m == 0 {
			return fmt.Sprintf("%dh", h)
		}
		return fmt.Sprintf("%dh%dm", h, m)
	}
}
