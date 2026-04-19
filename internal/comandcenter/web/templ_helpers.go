package web

import (
	"fmt"
	"html/template"
	"strconv"
	"strings"
	"time"

	cc "github.com/Abraxas-365/claudio/internal/comandcenter"
)

// itoa converts an int to its decimal string representation.
func itoa(i int) string { return strconv.Itoa(i) }

// sessionRow holds data for a single session row in the sidebar.
// Shared between templ components (session_row.templ, sessions.templ, chat_list.templ)
// and the server handlers that build the row list.
type sessionRow struct {
	Session     cc.Session
	LastMessage string
	UnreadCount int
}

// RelTime formats a time.Time as a human-friendly relative timestamp.
func RelTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 min ago"
		}
		return itoa(m) + " mins ago"
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hr ago"
		}
		return itoa(h) + " hrs ago"
	default:
		return t.Format("Jan 2")
	}
}

// FirstChar returns the uppercase first rune of s, or "?" if empty.
func FirstChar(s string) string {
	if len(s) == 0 {
		return "?"
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0]))
}

// Truncate returns the first n runes of s, appending "…" if truncated.
func Truncate(n int, s string) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// AvatarColor returns a CSS color variable based on name length.
func AvatarColor(s string) string {
	colors := []string{
		"var(--color-brand)",
		"var(--color-ai)",
		"var(--color-tool)",
		"var(--color-cron)",
		"var(--color-error)",
	}
	if len(s) == 0 {
		return colors[0]
	}
	return colors[len(s)%len(colors)]
}

// IsImage reports whether a MIME type is an image.
func IsImage(mimeType string) bool {
	return strings.HasPrefix(mimeType, "image/")
}

// RenderMD converts markdown content to sanitized HTML.
// Returns template.HTML so templ can output it unescaped via @templ.Raw().
func RenderMD(content string) template.HTML {
	return template.HTML(renderMarkdown(content))
}

// HasPrefix reports whether s starts with prefix.
func HasPrefix(s, prefix string) bool {
	return strings.HasPrefix(s, prefix)
}

// ToolName extracts the tool name from "ToolName: {json}" content.
func ToolName(s string) string {
	if i := strings.Index(s, ": "); i > 0 {
		return s[:i]
	}
	return s
}

// ToolInput extracts the JSON input from "ToolName: {json}" content.
func ToolInput(s string) string {
	if i := strings.Index(s, ": "); i > 0 {
		return strings.TrimSpace(s[i+2:])
	}
	return ""
}

// FormatTokens formats a token count for display: "—" if 0, raw number if <1000, "1.2K" if ≥1000.
func FormatTokens(n int) string {
	if n == 0 {
		return "—"
	}
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%.1fK", float64(n)/1000)
}

// sidebarHiddenClass returns CSS classes to hide sidebar on mobile when a session is active.
func sidebarHiddenClass(sessionID string) string {
	if sessionID != "" {
		return " hidden md:flex"
	}
	return ""
}

// mainHiddenClass returns CSS classes to hide main area on mobile when no session is active.
func mainHiddenClass(sessionID string) string {
	if sessionID == "" {
		return " hidden md:flex"
	}
	return ""
}


