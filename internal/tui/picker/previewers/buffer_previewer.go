package previewers

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/tui/picker"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// BufferPreviewer renders buffer metadata in the picker preview pane.
// For agent:// buffers it shows live/done status. For regular windows it
// shows the buffer name and any metadata stored in the entry.
type BufferPreviewer struct{}

// NewBufferPreviewer returns a BufferPreviewer.
func NewBufferPreviewer() *BufferPreviewer { return &BufferPreviewer{} }

// Render implements picker.Previewer.
func (p *BufferPreviewer) Render(entry picker.Entry, width, height int) string {
	name, _ := entry.Meta["name"].(string)
	if name == "" {
		name = entry.Ordinal
	}

	rw := width - 2
	if rw < 10 {
		rw = 10
	}

	var b strings.Builder

	// Header: buffer name
	nameStyle := lipgloss.NewStyle().Foreground(styles.Primary).Bold(true)
	b.WriteString("  " + nameStyle.Render(name) + "\n")

	// Status line
	isLive, _ := entry.Meta["live"].(bool)
	status, _ := entry.Meta["status"].(string)

	if isLive {
		var statusColor lipgloss.Color
		var statusIcon string
		switch status {
		case "done":
			statusColor = styles.Success
			statusIcon = "✓"
		case "error":
			statusColor = styles.Error
			statusIcon = "✗"
		default:
			statusColor = styles.Warning
			statusIcon = "⟳"
		}
		statusStyle := lipgloss.NewStyle().Foreground(statusColor)
		b.WriteString("  " + statusStyle.Render(statusIcon+" "+status) + "\n")
	} else {
		b.WriteString("  " + dimStyle.Render("static buffer") + "\n")
	}

	// Separator
	b.WriteString("\n" + styles.SeparatorLine(rw) + "\n\n")

	// Extra meta (skip "name", "live", "status" — already shown)
	skip := map[string]bool{"name": true, "live": true, "status": true}
	for k, v := range entry.Meta {
		if skip[k] {
			continue
		}
		b.WriteString(fmt.Sprintf("  %s: %v\n",
			dimStyle.Render(k),
			v,
		))
	}

	return b.String()
}
