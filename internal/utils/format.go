package utils

import (
	"fmt"
	"strings"
	"time"
)

// FormatFileSize formats bytes into human-readable size.
func FormatFileSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)

	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.1fTB", float64(bytes)/float64(TB))
	case bytes >= GB:
		return fmt.Sprintf("%.1fGB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1fMB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1fKB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

// FormatDuration formats a duration into human-readable form.
func FormatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		m := int(d.Minutes())
		s := int(d.Seconds()) % 60
		if s == 0 {
			return fmt.Sprintf("%dm", m)
		}
		return fmt.Sprintf("%dm%ds", m, s)
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh%dm", h, m)
}

// FormatTokenCount formats a token count with K/M suffixes.
func FormatTokenCount(tokens int) string {
	if tokens >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(tokens)/1_000_000)
	}
	if tokens >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(tokens)/1_000)
	}
	return fmt.Sprintf("%d", tokens)
}

// FormatCost formats a USD cost value.
func FormatCost(cost float64) string {
	if cost < 0.01 {
		return fmt.Sprintf("$%.4f", cost)
	}
	if cost < 1.0 {
		return fmt.Sprintf("$%.3f", cost)
	}
	return fmt.Sprintf("$%.2f", cost)
}

// FormatPercent formats a percentage.
func FormatPercent(value, total float64) string {
	if total == 0 {
		return "0%"
	}
	return fmt.Sprintf("%.0f%%", value/total*100)
}

// Treeify formats a list of paths as a tree structure.
func Treeify(paths []string) string {
	if len(paths) == 0 {
		return ""
	}

	var sb strings.Builder
	for i, path := range paths {
		prefix := "├── "
		if i == len(paths)-1 {
			prefix = "└── "
		}
		sb.WriteString(prefix + path + "\n")
	}
	return sb.String()
}

// FormatTable formats data as a simple ASCII table.
func FormatTable(headers []string, rows [][]string) string {
	if len(headers) == 0 {
		return ""
	}

	// Calculate column widths
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	var sb strings.Builder

	// Header
	for i, h := range headers {
		sb.WriteString(fmt.Sprintf("%-*s", widths[i]+2, h))
	}
	sb.WriteString("\n")

	// Separator
	for _, w := range widths {
		sb.WriteString(strings.Repeat("-", w+2))
	}
	sb.WriteString("\n")

	// Rows
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) {
				sb.WriteString(fmt.Sprintf("%-*s", widths[i]+2, cell))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// PluralS returns "s" if count != 1, for simple pluralization.
func PluralS(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

// TimeAgo returns a human-readable relative time string.
func TimeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		return fmt.Sprintf("%d minute%s ago", m, PluralS(m))
	case d < 24*time.Hour:
		h := int(d.Hours())
		return fmt.Sprintf("%d hour%s ago", h, PluralS(h))
	case d < 30*24*time.Hour:
		days := int(d.Hours() / 24)
		return fmt.Sprintf("%d day%s ago", days, PluralS(days))
	default:
		return t.Format("2006-01-02")
	}
}
