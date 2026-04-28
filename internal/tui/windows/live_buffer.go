package windows

import (
	"strings"
	"sync"
)

// LiveBuffer is an append-only, thread-safe line buffer designed for streaming
// output (e.g. running shell commands, agent output). It can be exposed to the
// window system via Buffer().
//
// Status values: "running" | "done" | "error"
type LiveBuffer struct {
	name   string
	lines  []string
	mu     sync.RWMutex
	done   bool
	status string // "running", "done", "error"
}

// NewLiveBuffer creates a LiveBuffer in the "running" state.
func NewLiveBuffer(name string) *LiveBuffer {
	return &LiveBuffer{
		name:   name,
		status: "running",
	}
}

// Append adds a line to the buffer. Safe to call from any goroutine.
func (b *LiveBuffer) Append(line string) {
	b.mu.Lock()
	b.lines = append(b.lines, line)
	b.mu.Unlock()
}

// SetDone transitions the buffer to a terminal state.
// status must be "done" or "error"; any other value defaults to "done".
func (b *LiveBuffer) SetDone(status string) {
	if status != "done" && status != "error" {
		status = "done"
	}
	b.mu.Lock()
	b.done = true
	b.status = status
	b.mu.Unlock()
}

// Done reports whether the buffer has reached a terminal state.
func (b *LiveBuffer) Done() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.done
}

// Status returns the current status string ("running", "done", or "error").
func (b *LiveBuffer) Status() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.status
}

// Lines returns a snapshot of all lines accumulated so far.
// The returned slice is a copy; safe to use outside the lock.
func (b *LiveBuffer) Lines() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	snap := make([]string, len(b.lines))
	copy(snap, b.lines)
	return snap
}

// Buffer returns a *Buffer whose Render func displays a viewport-sized window
// into the live content (most-recent lines, no scrolling). The Buffer is
// suitable for passing to a Window or windows.Manager.
//
// The returned *Buffer captures a reference to this LiveBuffer; Render always
// reflects the latest content at call time.
func (b *LiveBuffer) Buffer() *Buffer {
	return &Buffer{
		Name: b.name,
		Render: func(width, height int) string {
			lines := b.Lines()

			// Trim each line to fit width.
			if width > 0 {
				for i, l := range lines {
					runes := []rune(l)
					if len(runes) > width {
						lines[i] = string(runes[:width])
					}
				}
			}

			// Show only the most-recent `height` lines (tail view).
			if height > 0 && len(lines) > height {
				lines = lines[len(lines)-height:]
			}

			return strings.Join(lines, "\n")
		},
	}
}
