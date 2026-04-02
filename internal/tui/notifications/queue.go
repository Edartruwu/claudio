// Package notifications provides a priority-based notification queue for the TUI.
package notifications

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
		lines = append(lines, lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6B7280")).
			Render(fmt.Sprintf("  +%d more", len(q.items)-5)))
	}

	content := strings.Join(lines, "\n")

	// Align to the right side of the screen
	return lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Right).
		Render(content)
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

func styleForPriority(p Priority) lipgloss.Style {
	base := lipgloss.NewStyle().
		Padding(0, 1).
		MarginBottom(0)

	switch p {
	case Low:
		return base.
			Foreground(lipgloss.Color("#9CA3AF")).
			Background(lipgloss.Color("#1F2937"))
	case Medium:
		return base.
			Foreground(lipgloss.Color("#E5E7EB")).
			Background(lipgloss.Color("#1F2937")).
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(lipgloss.Color("#3B82F6"))
	case High:
		return base.
			Foreground(lipgloss.Color("#FBBF24")).
			Background(lipgloss.Color("#1F2937")).
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(lipgloss.Color("#F59E0B")).
			Bold(true)
	case Immediate:
		return base.
			Foreground(lipgloss.Color("#F87171")).
			Background(lipgloss.Color("#1F2937")).
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(lipgloss.Color("#EF4444")).
			Bold(true)
	default:
		return base
	}
}
