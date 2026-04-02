package tui

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// MessageType identifies the kind of message.
type MessageType int

const (
	MsgUser MessageType = iota
	MsgAssistant
	MsgToolUse
	MsgToolResult
	MsgThinking
	MsgError
	MsgSystem
)

// ChatMessage represents a rendered message in the conversation.
type ChatMessage struct {
	Type         MessageType
	Content      string
	ToolName     string
	ToolInput    string          // one-line summary
	ToolInputRaw json.RawMessage // full input JSON for expanded view
	IsError      bool
}

// ── Rendering ─────────────────────────────────────────────

const indent = "   " // 3-space indent to align under prefix text

// Section describes a rendered block in the viewport.
type Section struct {
	MsgIndex     int  // index into messages array
	IsToolGroup  bool // true if this section is a tool group
	LineStart    int  // first line of this section in rendered output
	LineCount    int  // number of lines in this section
}

// RenderResult holds the rendered messages and section metadata.
type RenderResult struct {
	Content  string
	Sections []Section
}

// renderMessages renders all messages, grouping consecutive tool pairs.
// cursorSection is the section index to highlight (-1 = none).
func renderMessages(msgs []ChatMessage, width int, expandedGroups map[int]bool, cursorSection int) RenderResult {
	maxW := width - 4
	if maxW < 40 {
		maxW = 40
	}

	var rendered []string
	var sections []Section
	currentLine := 0
	i := 0
	sectionIdx := 0

	for i < len(msgs) {
		msg := msgs[i]

		var block string
		var sec Section

		// Try to group consecutive ToolUse/ToolResult pairs
		if msg.Type == MsgToolUse {
			group := collectToolGroup(msgs, i)
			expanded := expandedGroups[i]
			block = renderToolGroup(group, maxW, expanded)
			sec = Section{MsgIndex: i, IsToolGroup: true}
			i += countGroupMessages(group)
		} else {
			block = renderMessage(msg, maxW)
			sec = Section{MsgIndex: i}
			i++
		}

		// Apply cursor highlight
		if sectionIdx == cursorSection {
			block = highlightSection(block, maxW)
		}

		lineCount := strings.Count(block, "\n") + 1
		sec.LineStart = currentLine
		sec.LineCount = lineCount
		sections = append(sections, sec)

		rendered = append(rendered, block)
		currentLine += lineCount + 1 // +1 for the blank line between sections
		sectionIdx++
	}

	return RenderResult{
		Content:  strings.Join(rendered, "\n\n"),
		Sections: sections,
	}
}

// highlightSection adds a left border marker to indicate the cursor position.
func highlightSection(block string, maxW int) string {
	cursor := styles.ViewportCursor
	lines := strings.Split(block, "\n")
	for i, line := range lines {
		if i == 0 {
			lines[i] = cursor.Render("▌") + " " + line
		} else {
			lines[i] = cursor.Render("▌") + " " + line
		}
	}
	return strings.Join(lines, "\n")
}

// countGroupMessages counts how many messages a tool group spans.
func countGroupMessages(group []toolPair) int {
	n := 0
	for _, p := range group {
		n++ // tool use
		if p.result != nil {
			n++ // tool result
		}
	}
	return n
}

// toolPair is a ToolUse matched with its ToolResult.
type toolPair struct {
	use    ChatMessage
	result *ChatMessage // nil if still running
}

// collectToolGroup gathers consecutive ToolUse(+ToolResult) pairs starting at idx.
func collectToolGroup(msgs []ChatMessage, idx int) []toolPair {
	var group []toolPair
	for i := idx; i < len(msgs); i++ {
		if msgs[i].Type != MsgToolUse {
			break
		}
		pair := toolPair{use: msgs[i]}
		// Check if next message is the matching result
		if i+1 < len(msgs) && msgs[i+1].Type == MsgToolResult {
			r := msgs[i+1]
			pair.result = &r
			i++ // skip the result
		}
		group = append(group, pair)
	}
	return group
}

// renderToolGroup renders a compact block of tool calls with tree connectors.
func renderToolGroup(group []toolPair, maxW int, expanded bool) string {
	nameW := 8 // fixed column for tool name

	var lines []string
	for gi, p := range group {
		isLast := gi == len(group)-1

		// Tree connector prefix
		var connector string
		if len(group) > 1 {
			if isLast {
				connector = styles.ToolConnector.Render("└─ ")
			} else if gi == 0 {
				connector = styles.ToolConnector.Render("┌─ ")
			} else {
				connector = styles.ToolConnector.Render("├─ ")
			}
		}

		// Left side: icon + name + summary
		icon := styles.ToolIcon.Render("⚡ ")
		name := styles.ToolName.Width(nameW).Render(p.use.ToolName)
		summaryText := formatRichSummary(p.use)
		summary := styles.ToolSummary.Render(truncate(summaryText, maxW-nameW-25))

		left := connector + icon + name + summary

		// Right side: status
		var status string
		if p.result == nil {
			status = styles.SpinnerStyle.Render("⠋") + styles.SpinnerText.Render(" running")
		} else if p.result.IsError {
			brief := firstLine(p.result.Content)
			status = styles.ToolError.Render("✗ " + truncate(brief, 30))
		} else {
			brief := resultBrief(p.result.Content)
			status = styles.ToolSuccess.Render("✓") + styles.ToolSummary.Render(" "+brief)
		}

		gap := maxW - lipgloss.Width(left) - lipgloss.Width(status)
		if gap < 1 {
			gap = 1
		}
		lines = append(lines, left+strings.Repeat(" ", gap)+status)

		// Tree continuation prefix for detail lines
		var detailPrefix string
		if len(group) > 1 {
			if isLast {
				detailPrefix = "   "
			} else {
				detailPrefix = styles.ToolConnector.Render("│") + "  "
			}
		}

		// Error details (always shown)
		if p.result != nil && p.result.IsError && p.result.Content != "" {
			detail := expandedResult(p.result.Content, 5)
			for _, dl := range detail {
				lines = append(lines, detailPrefix+styles.ToolConnector.Render("    │ ")+styles.ToolError.Render(dl))
			}
		}

		// Expanded details
		if expanded {
			lines = append(lines, renderToolExpanded(p, detailPrefix, maxW)...)
		}
	}

	// Expand hint for collapsed groups
	if !expanded && len(group) > 0 && group[len(group)-1].result != nil {
		hint := styles.ToolExpandHint.Render("    ctrl+o to expand")
		lines = append(lines, hint)
	}

	return strings.Join(lines, "\n")
}

// formatRichSummary produces a one-line summary with richer context per tool type.
func formatRichSummary(msg ChatMessage) string {
	if msg.ToolInputRaw == nil {
		return msg.ToolInput
	}

	switch msg.ToolName {
	case "Bash":
		var in struct {
			Command     string `json:"command"`
			Description string `json:"description"`
		}
		if json.Unmarshal(msg.ToolInputRaw, &in) == nil && in.Command != "" {
			cmd := "$ " + in.Command
			if in.Description != "" {
				cmd += "  — " + in.Description
			}
			return cmd
		}

	case "Read":
		var in struct {
			FilePath string `json:"file_path"`
			Offset   int    `json:"offset"`
			Limit    int    `json:"limit"`
		}
		if json.Unmarshal(msg.ToolInputRaw, &in) == nil && in.FilePath != "" {
			short := shortenPath(in.FilePath)
			if in.Offset > 0 || in.Limit > 0 {
				if in.Offset > 0 && in.Limit > 0 {
					return fmt.Sprintf("%s:%d-%d", short, in.Offset, in.Offset+in.Limit-1)
				} else if in.Offset > 0 {
					return fmt.Sprintf("%s:%d+", short, in.Offset)
				} else {
					return fmt.Sprintf("%s (first %d lines)", short, in.Limit)
				}
			}
			return short
		}

	case "Write":
		var in struct {
			FilePath string `json:"file_path"`
			Content  string `json:"content"`
		}
		if json.Unmarshal(msg.ToolInputRaw, &in) == nil && in.FilePath != "" {
			short := shortenPath(in.FilePath)
			size := len(in.Content)
			return fmt.Sprintf("→ %s (%s)", short, humanSize(size))
		}

	case "Edit":
		var in struct {
			FilePath  string `json:"file_path"`
			OldString string `json:"old_string"`
			NewString string `json:"new_string"`
		}
		if json.Unmarshal(msg.ToolInputRaw, &in) == nil && in.FilePath != "" {
			short := shortenPath(in.FilePath)
			return fmt.Sprintf("✎ %s", short)
		}

	case "Grep":
		var in struct {
			Pattern string `json:"pattern"`
			Path    string `json:"path"`
			Glob    string `json:"glob"`
			Type    string `json:"type"`
		}
		if json.Unmarshal(msg.ToolInputRaw, &in) == nil && in.Pattern != "" {
			s := fmt.Sprintf(`"%s"`, in.Pattern)
			if in.Glob != "" {
				s += " in " + in.Glob
			} else if in.Type != "" {
				s += " in *." + in.Type
			}
			if in.Path != "" {
				s += " " + shortenPath(in.Path)
			}
			return s
		}

	case "Glob":
		var in struct {
			Pattern string `json:"pattern"`
			Path    string `json:"path"`
		}
		if json.Unmarshal(msg.ToolInputRaw, &in) == nil && in.Pattern != "" {
			s := in.Pattern
			if in.Path != "" {
				s += " in " + shortenPath(in.Path)
			}
			return s
		}

	case "Agent":
		var in struct {
			Description  string `json:"description"`
			SubagentType string `json:"subagent_type"`
		}
		if json.Unmarshal(msg.ToolInputRaw, &in) == nil {
			s := ""
			if in.SubagentType != "" {
				s = "[" + in.SubagentType + "] "
			}
			if in.Description != "" {
				s += in.Description
			}
			return s
		}
	}

	// Fallback to the pre-computed summary
	return msg.ToolInput
}

// renderToolExpanded renders expanded detail lines for a tool pair.
func renderToolExpanded(p toolPair, prefix string, maxW int) []string {
	var lines []string
	detailIndent := prefix + "    "

	switch p.use.ToolName {
	case "Edit":
		if p.use.ToolInputRaw != nil {
			var in struct {
				OldString  string `json:"old_string"`
				NewString  string `json:"new_string"`
				ReplaceAll bool   `json:"replace_all"`
			}
			if json.Unmarshal(p.use.ToolInputRaw, &in) == nil {
				if in.ReplaceAll {
					lines = append(lines, detailIndent+styles.ToolDescription.Render("replace all occurrences"))
				}
				// Show diff preview
				oldLines := strings.Split(in.OldString, "\n")
				newLines := strings.Split(in.NewString, "\n")
				maxDiff := 4
				for i, ol := range oldLines {
					if i >= maxDiff {
						lines = append(lines, detailIndent+styles.ToolConnector.Render("│ ")+styles.ToolDiffOld.Render("  ... ("+fmt.Sprintf("%d", len(oldLines)-maxDiff)+" more lines)"))
						break
					}
					lines = append(lines, detailIndent+styles.ToolConnector.Render("│ ")+styles.ToolDiffOld.Render("- "+truncate(ol, maxW-20)))
				}
				for i, nl := range newLines {
					if i >= maxDiff {
						lines = append(lines, detailIndent+styles.ToolConnector.Render("│ ")+styles.ToolDiffNew.Render("  ... ("+fmt.Sprintf("%d", len(newLines)-maxDiff)+" more lines)"))
						break
					}
					lines = append(lines, detailIndent+styles.ToolConnector.Render("│ ")+styles.ToolDiffNew.Render("+ "+truncate(nl, maxW-20)))
				}
			}
		}

	case "Write":
		if p.use.ToolInputRaw != nil {
			var in struct {
				Content string `json:"content"`
			}
			if json.Unmarshal(p.use.ToolInputRaw, &in) == nil && in.Content != "" {
				preview := strings.Split(in.Content, "\n")
				maxLines := 6
				if len(preview) > maxLines {
					preview = preview[:maxLines]
				}
				for _, l := range preview {
					lines = append(lines, detailIndent+styles.ToolConnector.Render("│ ")+styles.ToolResultPreview.Render(truncate(l, maxW-20)))
				}
				totalLines := strings.Count(in.Content, "\n") + 1
				if totalLines > maxLines {
					lines = append(lines, detailIndent+styles.ToolConnector.Render("│ ")+styles.ToolDescription.Render(fmt.Sprintf("... %d more lines", totalLines-maxLines)))
				}
			}
		}

	case "Bash":
		if p.use.ToolInputRaw != nil {
			var in struct {
				Description     string `json:"description"`
				RunInBackground bool   `json:"run_in_background"`
				Timeout         int    `json:"timeout"`
			}
			if json.Unmarshal(p.use.ToolInputRaw, &in) == nil {
				if in.Description != "" {
					lines = append(lines, detailIndent+styles.ToolDescription.Render(in.Description))
				}
				var flags []string
				if in.RunInBackground {
					flags = append(flags, "background")
				}
				if in.Timeout > 0 {
					flags = append(flags, fmt.Sprintf("timeout: %ds", in.Timeout/1000))
				}
				if len(flags) > 0 {
					lines = append(lines, detailIndent+styles.ToolDescription.Render("("+strings.Join(flags, ", ")+")"))
				}
			}
		}
	}

	// Show result content preview (for success cases too)
	if p.result != nil && !p.result.IsError && p.result.Content != "" {
		resultLines := strings.Split(p.result.Content, "\n")
		maxShow := 8
		if len(resultLines) > maxShow {
			resultLines = resultLines[:maxShow]
		}
		if len(resultLines) > 0 && !(len(resultLines) == 1 && strings.TrimSpace(resultLines[0]) == "") {
			lines = append(lines, detailIndent+styles.ToolDescription.Render("output:"))
			for _, rl := range resultLines {
				lines = append(lines, detailIndent+styles.ToolConnector.Render("│ ")+styles.ToolResultPreview.Render(truncate(rl, maxW-20)))
			}
			totalRL := strings.Count(p.result.Content, "\n") + 1
			if totalRL > maxShow {
				lines = append(lines, detailIndent+styles.ToolConnector.Render("│ ")+styles.ToolDescription.Render(fmt.Sprintf("... %d more lines", totalRL-maxShow)))
			}
		}
	}

	return lines
}

// shortenPath shortens a file path for display by using basename or relative notation.
func shortenPath(p string) string {
	// If path is short enough, show as-is
	if len(p) <= 50 {
		return p
	}
	// Show last 2 path components
	dir := filepath.Dir(p)
	base := filepath.Base(p)
	parent := filepath.Base(dir)
	if parent != "." && parent != "/" {
		return "…/" + parent + "/" + base
	}
	return base
}

// humanSize formats a byte count for display.
func humanSize(bytes int) string {
	switch {
	case bytes >= 1024*1024:
		return fmt.Sprintf("%.1fMB", float64(bytes)/(1024*1024))
	case bytes >= 1024:
		return fmt.Sprintf("%.1fkB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

// renderMessage renders a single ChatMessage.
func renderMessage(msg ChatMessage, maxW int) string {
	switch msg.Type {
	case MsgUser:
		prefix := styles.UserPrefix.Render("❯ ")
		content := styles.UserContent.Width(maxW - 2).Render(msg.Content)
		return prefix + content

	case MsgAssistant:
		prefix := styles.AssistantPrefix.Render("● ")
		rendered := renderMarkdown(msg.Content, maxW-3)
		// Indent continuation lines to align under prefix
		indented := indentContinuation(rendered, indent)
		return prefix + indented

	case MsgToolUse:
		// Standalone tool use (no grouping fallback)
		icon := styles.ToolIcon.Render("⚡ ")
		name := styles.ToolName.Render(msg.ToolName)
		summary := styles.ToolSummary.Render("  " + truncate(msg.ToolInput, maxW-20))
		return icon + name + summary

	case MsgToolResult:
		// Standalone result (no grouping fallback)
		if msg.IsError {
			return indent + styles.ToolError.Render("✗ "+truncate(msg.Content, maxW-6))
		}
		brief := resultBrief(msg.Content)
		return indent + styles.ToolSuccess.Render("✓ ") + styles.ToolSummary.Render(brief)

	case MsgThinking:
		return styles.ThinkingStyle.Width(maxW).Render("💭 " + truncate(msg.Content, 200))

	case MsgError:
		return styles.ErrorStyle.Width(maxW).Render("✗ " + msg.Content)

	case MsgSystem:
		return styles.SystemStyle.Width(maxW).Render("ℹ " + msg.Content)

	default:
		return msg.Content
	}
}

// ── Status Bar ────────────────────────────────────────────

// StatusBarState holds info for the status bar.
type StatusBarState struct {
	Model     string
	Tokens    int
	Cost      float64
	Turns     int
	Streaming bool
	SpinText  string
	Hint      string
	VimMode   string // "", "NORMAL", "INSERT", "VISUAL"
}

func renderStatusBar(width int, s StatusBarState) string {
	sep := styles.Sep()

	// Left: vim mode + model + streaming indicator + tokens + cost + turns
	var parts []string

	if s.VimMode != "" {
		var modeStyle lipgloss.Style
		switch s.VimMode {
		case "NORMAL":
			modeStyle = lipgloss.NewStyle().Background(styles.Secondary).Foreground(styles.Surface).Bold(true).Padding(0, 1)
		case "VISUAL":
			modeStyle = lipgloss.NewStyle().Background(styles.Primary).Foreground(styles.Surface).Bold(true).Padding(0, 1)
		case "VIEWPORT":
			modeStyle = lipgloss.NewStyle().Background(styles.Warning).Foreground(styles.Surface).Bold(true).Padding(0, 1)
		default: // INSERT
			modeStyle = lipgloss.NewStyle().Background(styles.Success).Foreground(styles.Surface).Bold(true).Padding(0, 1)
		}
		parts = append(parts, modeStyle.Render(s.VimMode))
	}

	parts = append(parts, styles.StatusModel.Render(s.Model))

	if s.Streaming && s.SpinText != "" {
		parts = append(parts, styles.StatusActive.Render("● "+s.SpinText))
	}

	if s.Tokens > 0 {
		var tokStr string
		if s.Tokens >= 1000 {
			tokStr = fmt.Sprintf("%.1fk tokens", float64(s.Tokens)/1000)
		} else {
			tokStr = fmt.Sprintf("%d tokens", s.Tokens)
		}
		parts = append(parts, tokStr)
	}

	if s.Cost > 0 {
		parts = append(parts, fmt.Sprintf("$%.2f", s.Cost))
	}

	if s.Turns > 0 {
		parts = append(parts, fmt.Sprintf("%d turns", s.Turns))
	}

	left := strings.Join(parts, styles.StatusSeparator.Render(" │ "))

	// Right: contextual hint
	right := ""
	if s.Hint != "" {
		right = styles.StatusHint.Render(s.Hint)
	}

	gap := width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}

	content := " " + left + strings.Repeat(" ", gap) + right + " "

	// Full-width background
	_ = sep // used in left join
	return styles.StatusBar.Width(width).Render(content)
}

// ── Markdown ─────────────────────────────────────────────

var (
	cachedRenderer *glamour.TermRenderer
	cachedWidth    int
)

func renderMarkdown(content string, width int) string {
	if content == "" {
		return ""
	}

	// Reuse renderer unless width changed
	if cachedRenderer == nil || cachedWidth != width {
		r, err := glamour.NewTermRenderer(
			glamour.WithStylesFromJSONBytes(styles.GruvboxGlamourJSON()),
			glamour.WithWordWrap(width),
		)
		if err != nil {
			return content
		}
		cachedRenderer = r
		cachedWidth = width
	}

	rendered, err := cachedRenderer.Render(content)
	if err != nil {
		return content
	}

	return strings.TrimSpace(rendered)
}

// ── Helpers ──────────────────────────────────────────────

func truncate(s string, max int) string {
	if max < 4 {
		max = 4
	}
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "\u2026"
}

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}

// resultBrief produces a short summary of a tool result.
func resultBrief(content string) string {
	lines := strings.Split(content, "\n")
	n := len(lines)
	size := len(content)

	if size == 0 {
		return "ok"
	}
	if n <= 3 && size <= 80 {
		return strings.TrimSpace(content)
	}

	var sizeStr string
	switch {
	case size >= 1024*1024:
		sizeStr = fmt.Sprintf("%.1fMB", float64(size)/(1024*1024))
	case size >= 1024:
		sizeStr = fmt.Sprintf("%.1fkB", float64(size)/1024)
	default:
		sizeStr = fmt.Sprintf("%dB", size)
	}
	return fmt.Sprintf("%d lines | %s", n, sizeStr)
}

// expandedResult returns the first n lines of content.
func expandedResult(content string, n int) []string {
	lines := strings.Split(content, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return lines
}

// indentContinuation indents all lines after the first.
func indentContinuation(s, prefix string) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= 1 {
		return s
	}
	for i := 1; i < len(lines); i++ {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}
