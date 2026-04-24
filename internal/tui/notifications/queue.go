// Package notifications provides a priority-based notification queue for the TUI.
package notifications

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// Priority levels for notifications.
type Priority int

const (
	Low       Priority = iota // auto-dismiss 3s
	Medium                    // auto-dismiss 5s
	High                      // auto-dismiss 10s
	Immediate                 // manual dismiss only
)

// MaxItems is the maximum number of notifications in the queue.
const MaxItems = 10

// Notification represents a single notification in the queue.
type Notification struct {
	ID       string
	Text     string
	Priority Priority
	Tag      string // for folding/invalidation
	Count    int    // fold counter (1 = single, 2+ = merged)
	Created  time.Time
	Timeout  time.Duration
}

// DismissMsg is sent when a notification should be dismissed.
type DismissMsg struct {
	ID string
}

// Queue manages a priority-ordered notification queue.
type Queue struct {
	items  []*Notification
	nextID int
}

// New creates a new notification queue.
func New() *Queue {
	return &Queue{}
}

// Push adds a notification to the queue.
// If a notification with the same tag exists, it folds (increments count).
// Returns a tea.Cmd for auto-dismiss timeout.
func (q *Queue) Push(text string, priority Priority, tag string) tea.Cmd {
	// Fold: merge with existing notification of same tag
	if tag != "" {
		for _, n := range q.items {
			if n.Tag == tag {
				n.Count++
				n.Text = text // update text to latest
				n.Created = time.Now()
				return q.autoDismissCmd(n)
			}
		}
	}

	q.nextID++
	id := fmt.Sprintf("notif-%d", q.nextID)

	timeout := timeoutForPriority(priority)

	n := &Notification{
		ID:       id,
		Text:     text,
		Priority: priority,
		Tag:      tag,
		Count:    1,
		Created:  time.Now(),
		Timeout:  timeout,
	}

	q.items = append(q.items, n)

	// Enforce max items (remove oldest low-priority first)
	for len(q.items) > MaxItems {
		q.removeLowestPriority()
	}

	return q.autoDismissCmd(n)
}

// Dismiss removes a notification by ID.
func (q *Queue) Dismiss(id string) {
	for i, n := range q.items {
		if n.ID == id {
			q.items = append(q.items[:i], q.items[i+1:]...)
			return
		}
	}
}

// DismissByTag removes all notifications with the given tag.
func (q *Queue) DismissByTag(tag string) {
	var kept []*Notification
	for _, n := range q.items {
		if n.Tag != tag {
			kept = append(kept, n)
		}
	}
	q.items = kept
}

// Update handles dismiss timer messages.
func (q *Queue) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case DismissMsg:
		q.Dismiss(msg.ID)
	}
	return nil
}

// View renders the notification stack.
func (q *Queue) View(width int) string {
	if len(q.items) == 0 {
		return ""
	}

	var lines []string
	// Show most recent first (reverse order), max 5 visible
	start := 0
	if len(q.items) > 5 {
		start = len(q.items) - 5
	}

	for i := len(q.items) - 1; i >= start; i-- {
		n := q.items[i]
		text := n.Text
		if n.Count > 1 {
			text = fmt.Sprintf("%s (x%d)", text, n.Count)
		}

		style := styleForPriority(n.Priority)
		lines = append(lines, style.Render(text))
	}

	if len(q.items) > 5 {
		lines = append(lines, notifMoreStyle.Render(fmt.Sprintf("  +%d more", len(q.items)-5)))
	}

	content := strings.Join(lines, "\n")

	return notifContainerStyle.Width(width).Render(content)
}

// Count returns the number of notifications in the queue.
func (q *Queue) Count() int {
	return len(q.items)
}

// IsEmpty returns true if the queue has no notifications.
func (q *Queue) IsEmpty() bool {
	return len(q.items) == 0
}

func (q *Queue) autoDismissCmd(n *Notification) tea.Cmd {
	if n.Timeout <= 0 {
		return nil // no auto-dismiss (Immediate priority)
	}
	id := n.ID
	timeout := n.Timeout
	return tea.Tick(timeout, func(time.Time) tea.Msg {
		return DismissMsg{ID: id}
	})
}

func (q *Queue) removeLowestPriority() {
	if len(q.items) == 0 {
		return
	}

	lowestIdx := 0
	for i, n := range q.items {
		if n.Priority < q.items[lowestIdx].Priority {
			lowestIdx = i
		}
	}
	q.items = append(q.items[:lowestIdx], q.items[lowestIdx+1:]...)
}

func timeoutForPriority(p Priority) time.Duration {
	switch p {
	case Low:
		return 3 * time.Second
	case Medium:
		return 5 * time.Second
	case High:
		return 10 * time.Second
	case Immediate:
		return 0 // no auto-dismiss
	default:
		return 5 * time.Second
	}
}

// Pre-allocated notification styles using theme tokens.
var (
	notifMoreStyle      = lipgloss.NewStyle().Foreground(styles.Muted)
	notifContainerStyle = lipgloss.NewStyle().Align(lipgloss.Right)

	notifStyleLow = lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(styles.Muted).
			Background(styles.Surface)

	notifStyleMedium = lipgloss.NewStyle().
				Padding(0, 1).
				Foreground(styles.Text).
				Background(styles.Surface).
				Border(lipgloss.NormalBorder(), false, false, false, true).
				BorderForeground(styles.Secondary)

	notifStyleHigh = lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(styles.Warning).
			Background(styles.Surface).
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(styles.Orange).
			Bold(true)

	notifStyleImmediate = lipgloss.NewStyle().
				Padding(0, 1).
				Foreground(styles.Error).
				Background(styles.Surface).
				Border(lipgloss.NormalBorder(), false, false, false, true).
				BorderForeground(styles.Error).
				Bold(true)
)

func styleForPriority(p Priority) lipgloss.Style {
	switch p {
	case Low:
		return notifStyleLow
	case Medium:
		return notifStyleMedium
	case High:
		return notifStyleHigh
	case Immediate:
		return notifStyleImmediate
	default:
		return notifStyleLow
	}
}
