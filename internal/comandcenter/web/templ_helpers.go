package web

import (
	"fmt"
	"html/template"
	"sort"
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

// IsAgentStatusLine returns true for messages composed entirely of agent
// status lines (e.g. "⏳ agent — done", "✅agent — done") that should
// be filtered from chat history. Matches any line starting with a status
// emoji (⏳✅❌) and ending with "done" regardless of dash style.
func IsAgentStatusLine(content string) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return false
	}
	lines := strings.Split(trimmed, "\n")
	for _, line := range lines {
		l := strings.TrimSpace(line)
		if l == "" {
			continue
		}
		if !isAgentStatusSingle(l) {
			return false
		}
	}
	return true
}

// isAgentStatusSingle matches a single line like "⏳ name — done" or "✅name - done".
// Checks: starts with status emoji + contains "done" at end + short enough to be a status line.
func isAgentStatusSingle(line string) bool {
	if len(line) > 200 {
		return false
	}
	hasPrefix := strings.HasPrefix(line, "⏳") ||
		strings.HasPrefix(line, "✅") ||
		strings.HasPrefix(line, "❌")
	if !hasPrefix {
		return false
	}
	lower := strings.ToLower(line)
	return strings.HasSuffix(lower, "done") ||
		strings.Contains(lower, "— done") ||
		strings.Contains(lower, "- done") ||
		strings.Contains(lower, "– done") ||
		strings.Contains(lower, "-- done")
}

// CountStatusLines counts non-empty lines in agent status content.
func CountStatusLines(content string) int {
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(content), "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

// StatusLines splits agent status content into non-empty lines.
func StatusLines(content string) []string {
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(content), "\n") {
		if l := strings.TrimSpace(line); l != "" {
			lines = append(lines, l)
		}
	}
	return lines
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

// formatElapsed formats seconds as "Xm Ys" (e.g. "2m 14s") or just "Xs" when < 60s.
func formatElapsed(secs int) string {
	if secs <= 0 {
		return "0s"
	}
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	m := secs / 60
	s := secs % 60
	return fmt.Sprintf("%dm %ds", m, s)
}

// agentAvatarClass returns the CSS class list for the agent avatar circle,
// adding the pulse animation class when the agent is running.
func agentAvatarClass(status string) string {
	base := "flex items-center justify-center font-semibold flex-shrink-0"
	if status == "running" {
		return base + " agent-pulse"
	}
	return base
}



// imgURL returns the serving URL for an attachment.
func imgURL(att cc.Attachment) string {
	return fmt.Sprintf("/uploads/%s/%s", att.SessionID, att.Filename)
}

// FilterImages returns only image attachments.
func FilterImages(atts []cc.Attachment) []cc.Attachment {
	var out []cc.Attachment
	for _, a := range atts {
		if IsImage(a.MimeType) {
			out = append(out, a)
		}
	}
	return out
}

// FilterNonImages returns only non-image attachments.
func FilterNonImages(atts []cc.Attachment) []cc.Attachment {
	var out []cc.Attachment
	for _, a := range atts {
		if !IsImage(a.MimeType) {
			out = append(out, a)
		}
	}
	return out
}

// --- File browser helpers ---

// breadcrumbSegment is a single segment in the file browser breadcrumb.
type breadcrumbSegment struct {
	Name string
	Path string
}

// breadcrumbSegments splits a path relative to root into clickable segments.
func breadcrumbSegments(currentPath, rootPath string) []breadcrumbSegment {
	rel := relBrowsePath(currentPath, rootPath)
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		return nil
	}
	parts := strings.Split(rel, "/")
	segs := make([]breadcrumbSegment, 0, len(parts))
	acc := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		acc += "/" + p
		segs = append(segs, breadcrumbSegment{Name: p, Path: acc})
	}
	return segs
}

// relBrowsePath returns abs relative to root, or abs if not under root.
func relBrowsePath(abs, root string) string {
	if root == "" || !strings.HasPrefix(abs, root) {
		return abs
	}
	rel := abs[len(root):]
	if rel == "" {
		return "/"
	}
	return rel
}

// joinPath joins a directory path and a filename.
func joinPath(dir, name string) string {
	dir = strings.TrimSuffix(dir, "/")
	return dir + "/" + name
}

// sortedBrowseItems returns items sorted: directories first, then files.
func sortedBrowseItems(items []browseItem) []browseItem {
	sorted := make([]browseItem, len(items))
	copy(sorted, items)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].IsDir != sorted[j].IsDir {
			return sorted[i].IsDir
		}
		return strings.ToLower(sorted[i].Name) < strings.ToLower(sorted[j].Name)
	})
	return sorted
}

// fmtFileSize formats bytes as human-readable size.
func fmtFileSize(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
}

// boolStr returns "1" for true, "0" for false (for data attributes).
func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// prefixIDs prepends "#" to each task ID for display (e.g. ["1","3"] → ["#1","#3"]).
func prefixIDs(ids []string) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = "#" + id
	}
	return out
}

// --- Mention dropdown helpers ---

// mentionSession is a minimal session for the mention dropdown.
type mentionSession struct {
	ID     string
	Name   string
	Status string
}


