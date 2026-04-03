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
// SubagentToolCall represents a tool call made by a sub-agent.
type SubagentToolCall struct {
	ToolName   string
	Summary    string
	Result     *string // nil if still running
	IsError    bool
	ToolUseID  string
	DurationMs int64 // -1 = not tracked
}

type ChatMessage struct {
	Type         MessageType
	Content      string
	ToolName     string
	ToolInput    string          // one-line summary
	ToolInputRaw json.RawMessage // full input JSON for expanded view
	ToolUseID    string          // real tool_use_id from API (tool_use and tool_result messages)
	IsError      bool
	Pinned       bool  // pinned messages survive compaction
	IsSubagent   bool  // true if this is a sub-agent tool call
	DurationMs   int64 // -1 = not tracked

	// SubagentTools holds nested tool calls made by a sub-agent.
	// Only populated on MsgToolUse messages where ToolName == "Agent".
	SubagentTools []SubagentToolCall
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
	if maxW > 110 {
		maxW = 110
	}
	if maxW < 40 {
		maxW = 40
	}

	var rendered []string
	var sections []Section
	currentLine := 0
	i := 0
	sectionIdx := 0

	lastWasToolGroup := false

	for i < len(msgs) {
		msg := msgs[i]

		var block string
		var sec Section

		// Add a turn divider before each user message (except the very first)
		if msg.Type == MsgUser && i > 0 {
			divider := renderTurnDivider(maxW)
			rendered = append(rendered, divider)
			divLines := strings.Count(divider, "\n") + 1
			currentLine += divLines + 1
		}

		// Try to group consecutive ToolUse/ToolResult pairs
		if msg.Type == MsgToolUse {
			group := collectToolGroup(msgs, i)
			expanded := expandedGroups[i]
			block = renderToolGroup(group, maxW, expanded)
			sec = Section{MsgIndex: i, IsToolGroup: true}
			i += countGroupMessages(group)
			lastWasToolGroup = true
		} else {
			// Assistant text after a tool group is a continuation — render without ● prefix
			if msg.Type == MsgAssistant && lastWasToolGroup {
				block = renderAssistantContinuation(msg, maxW)
			} else {
				block = renderMessage(msg, maxW)
			}
			sec = Section{MsgIndex: i}
			i++
			lastWasToolGroup = false
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

// isToolGroupPinned checks if any tool in the group is pinned.
func isToolGroupPinned(group []toolPair) bool {
	for _, p := range group {
		if p.use.Pinned {
			return true
		}
	}
	return false
}

// collapseRunThreshold is the minimum consecutive same-tool count to collapse into one row.
const collapseRunThreshold = 3

// toolRun is a consecutive sequence of tool pairs with the same ToolName.
type toolRun struct {
	name  string
	pairs []toolPair
}

// splitIntoRuns groups consecutive same-name pairs together.
func splitIntoRuns(group []toolPair) []toolRun {
	var runs []toolRun
	i := 0
	for i < len(group) {
		name := group[i].use.ToolName
		j := i
		for j < len(group) && group[j].use.ToolName == name {
			j++
		}
		runs = append(runs, toolRun{name: name, pairs: group[i:j]})
		i = j
	}
	return runs
}

// renderToolGroup renders a compact block of tool calls with tree connectors.
// Consecutive runs of the same tool (≥ collapseRunThreshold) are collapsed into
// a single summary row unless the group is expanded.
func renderToolGroup(group []toolPair, maxW int, expanded bool) string {
	nameW := 8

	var lines []string

	if isToolGroupPinned(group) {
		lines = append(lines, styles.PinIcon.Render("📌 pinned"))
	}

	// Build visual items: collapsed runs or individual pairs.
	type visualItem struct {
		isRun bool
		run   []toolPair // populated when isRun == true
		pair  toolPair   // populated when isRun == false
	}
	var items []visualItem
	for _, run := range splitIntoRuns(group) {
		if !expanded && len(run.pairs) >= collapseRunThreshold {
			items = append(items, visualItem{isRun: true, run: run.pairs})
		} else {
			for _, p := range run.pairs {
				items = append(items, visualItem{pair: p})
			}
		}
	}

	for gi, item := range items {
		isLast := gi == len(items)-1

		var connector string
		if len(items) > 1 {
			switch {
			case gi == 0:
				connector = styles.ToolConnector.Render("┌─ ")
			case isLast:
				connector = styles.ToolConnector.Render("└─ ")
			default:
				connector = styles.ToolConnector.Render("├─ ")
			}
		}

		var detailPrefix string
		if len(items) > 1 {
			if isLast {
				detailPrefix = "   "
			} else {
				detailPrefix = styles.ToolConnector.Render("│") + "  "
			}
		}

		if item.isRun {
			lines = append(lines, renderCollapsedRun(item.run, connector, maxW, nameW))
			continue
		}

		p := item.pair
		icon := styles.ToolIcon.Render("⚡ ")
		name := styles.ToolName.Width(nameW).Render(p.use.ToolName)
		summaryText := formatRichSummary(p.use)
		summary := styles.ToolSummary.Render(truncate(summaryText, maxW-nameW-25))
		left := connector + icon + name + summary

		var status string
		if p.result == nil {
			status = styles.SpinnerStyle.Render("⠋") + styles.SpinnerText.Render(" running")
		} else if p.result.IsError {
			brief := firstLine(p.result.Content)
			status = styles.ToolError.Render("✗ " + truncate(brief, 30))
		} else {
			brief := resultBrief(p.result.Content)
			if dur := formatDuration(p.use.DurationMs); dur != "" {
				brief = dur + " · " + brief
			}
			status = styles.ToolSuccess.Render("✓") + styles.ToolSummary.Render(" "+brief)
		}

		gap := maxW - lipgloss.Width(left) - lipgloss.Width(status)
		if gap < 1 {
			gap = 1
		}
		lines = append(lines, left+strings.Repeat(" ", gap)+status)

		if len(p.use.SubagentTools) > 0 {
			lines = append(lines, renderSubagentTools(p.use.SubagentTools, p.use.DurationMs, maxW, nameW, expanded)...)
		}

		if p.result != nil && p.result.IsError && p.result.Content != "" {
			detail := expandedResult(p.result.Content, 5)
			for _, dl := range detail {
				lines = append(lines, detailPrefix+styles.ToolConnector.Render("    │ ")+styles.ToolError.Render(dl))
			}
		}

		if expanded {
			lines = append(lines, renderToolExpanded(p, detailPrefix, maxW)...)
		}
	}

	if !expanded && len(group) > 0 && group[len(group)-1].result != nil {
		hint := styles.ToolExpandHint.Render("    ctrl+o to expand")
		lines = append(lines, hint)
	}

	return strings.Join(lines, "\n")
}

// renderCollapsedRun shows only the latest (last) item in a run, with a dim
// "+N more" hint so the user knows there are hidden entries. ctrl+o reveals all.
func renderCollapsedRun(pairs []toolPair, connector string, maxW, nameW int) string {
	n := len(pairs)

	// Show the last pair — it's the most recent and most relevant.
	last := pairs[n-1]

	icon := styles.ToolIcon.Render("⚡ ")
	name := styles.ToolName.Width(nameW).Render(last.use.ToolName)
	summaryText := formatRichSummary(last.use)
	more := styles.ToolBadge.Render(fmt.Sprintf("+%d", n-1))
	summary := styles.ToolSummary.Render(truncate(summaryText, maxW-nameW-30))
	left := connector + icon + name + more + "  " + summary

	var status string
	// Count running/errors across the whole run for the status indicator.
	running, errors := 0, 0
	for _, p := range pairs {
		if p.result == nil {
			running++
		} else if p.result.IsError {
			errors++
		}
	}
	switch {
	case running > 0:
		status = styles.SpinnerStyle.Render("⠋") + styles.SpinnerText.Render(fmt.Sprintf(" %d/%d", n-running, n))
	case errors > 0:
		status = styles.ToolError.Render(fmt.Sprintf("✗ %d errors", errors))
	default:
		brief := resultBrief(last.result.Content)
		if dur := formatDuration(last.use.DurationMs); dur != "" {
			brief = dur + " · " + brief
		}
		status = styles.ToolSuccess.Render("✓") + styles.ToolSummary.Render(" "+brief)
	}

	gap := maxW - lipgloss.Width(left) - lipgloss.Width(status)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + status
}

// renderSubagentTools renders nested sub-agent tool calls under an Agent tool.
//
// Collapsed (default): one summary line — "Done (28 tool uses · 1m 20s)".
// Expanded (ctrl+o):   full list, consecutive same-tool runs ≥ collapseRunThreshold
//                      are still collapsed into a single row with a +N badge.
func renderSubagentTools(subs []SubagentToolCall, agentDurationMs int64, maxW, nameW int, expanded bool) []string {
	if len(subs) == 0 {
		return nil
	}
	ind := "   "

	if !expanded {
		return []string{renderSubagentSummary(subs, agentDurationMs, ind, maxW)}
	}

	// ── Expanded view ──────────────────────────────────────────────────────
	var lines []string

	// Group consecutive same-name subs into runs, then collapse long ones.
	type visualItem struct {
		isRun bool
		run   []SubagentToolCall
		sub   SubagentToolCall
	}
	var items []visualItem
	for i := 0; i < len(subs); {
		name := subs[i].ToolName
		j := i
		for j < len(subs) && subs[j].ToolName == name {
			j++
		}
		run := subs[i:j]
		if len(run) >= collapseRunThreshold {
			items = append(items, visualItem{isRun: true, run: run})
		} else {
			for _, s := range run {
				items = append(items, visualItem{sub: s})
			}
		}
		i = j
	}

	for gi, item := range items {
		isLast := gi == len(items)-1
		var connector string
		if len(items) > 1 {
			switch {
			case gi == 0:
				connector = styles.ToolConnector.Render("┌─ ")
			case isLast:
				connector = styles.ToolConnector.Render("└─ ")
			default:
				connector = styles.ToolConnector.Render("├─ ")
			}
		}

		if item.isRun {
			n := len(item.run)
			last := item.run[n-1]
			icon := styles.ToolIcon.Render("⚡ ")
			name := styles.ToolName.Width(nameW).Render(last.ToolName)
			more := styles.ToolBadge.Render(fmt.Sprintf("+%d", n-1))
			summary := styles.ToolSummary.Render(truncate(last.Summary, maxW-nameW-30))
			left := ind + connector + icon + name + more + "  " + summary

			running, errors := 0, 0
			for _, s := range item.run {
				if s.Result == nil {
					running++
				} else if s.IsError {
					errors++
				}
			}
			var status string
			switch {
			case running > 0:
				status = styles.SpinnerStyle.Render("⠋") + styles.SpinnerText.Render(fmt.Sprintf(" %d/%d", n-running, n))
			case errors > 0:
				status = styles.ToolError.Render(fmt.Sprintf("✗ %d errors", errors))
			default:
				brief := resultBrief(*last.Result)
				if dur := formatDuration(last.DurationMs); dur != "" {
					brief = dur + " · " + brief
				}
				status = styles.ToolSuccess.Render("✓") + styles.ToolSummary.Render(" "+brief)
			}
			gap := maxW - lipgloss.Width(left) - lipgloss.Width(status)
			if gap < 1 {
				gap = 1
			}
			lines = append(lines, left+strings.Repeat(" ", gap)+status)
			continue
		}

		sub := item.sub
		icon := styles.ToolIcon.Render("⚡ ")
		name := styles.ToolName.Width(nameW).Render(sub.ToolName)
		summary := styles.ToolSummary.Render(truncate(sub.Summary, maxW-nameW-30))
		left := ind + connector + icon + name + summary

		var status string
		if sub.Result == nil {
			status = styles.SpinnerStyle.Render("⠋") + styles.SpinnerText.Render(" running")
		} else if sub.IsError {
			status = styles.ToolError.Render("✗ " + truncate(*sub.Result, 30))
		} else {
			brief := *sub.Result
			if dur := formatDuration(sub.DurationMs); dur != "" {
				brief = dur + " · " + brief
			}
			status = styles.ToolSuccess.Render("✓") + styles.ToolSummary.Render(" "+brief)
		}
		gap := maxW - lipgloss.Width(left) - lipgloss.Width(status)
		if gap < 1 {
			gap = 1
		}
		lines = append(lines, left+strings.Repeat(" ", gap)+status)
	}

	return lines
}

// renderSubagentSummary produces the single collapsed line shown for sub-agent tools.
// Format: "   └─ N tools: 3×Read · 2×Glob · 1×Bash     ✓ done"
func renderSubagentSummary(subs []SubagentToolCall, agentDurationMs int64, ind string, maxW int) string {
	total := len(subs)
	running, errors := 0, 0
	counts := map[string]int{}
	for _, s := range subs {
		counts[s.ToolName]++
		if s.Result == nil {
			running++
		} else if s.IsError {
			errors++
		}
	}

	// Build "3×Read · 2×Glob · 1×Bash" — fixed display order for stability.
	toolOrder := []string{"Read", "Glob", "Grep", "Bash", "Write", "Edit", "Agent", "WebFetch", "WebSearch"}
	var parts []string
	seen := map[string]bool{}
	for _, name := range toolOrder {
		if c, ok := counts[name]; ok {
			if c == 1 {
				parts = append(parts, name)
			} else {
				parts = append(parts, fmt.Sprintf("%d×%s", c, name))
			}
			seen[name] = true
		}
	}
	// Any tools not in the fixed order go at the end.
	for name, c := range counts {
		if !seen[name] {
			if c == 1 {
				parts = append(parts, name)
			} else {
				parts = append(parts, fmt.Sprintf("%d×%s", c, name))
			}
		}
	}

	breakdown := strings.Join(parts, " · ")
	left := ind + styles.ToolConnector.Render("└─ ") +
		styles.ToolSummary.Render(fmt.Sprintf("%d tools: %s", total, breakdown))

	var status string
	switch {
	case running > 0:
		status = styles.SpinnerStyle.Render("⠋") + styles.SpinnerText.Render(fmt.Sprintf(" %d running", running))
	case errors > 0:
		status = styles.ToolError.Render(fmt.Sprintf("✗ %d errors", errors))
	default:
		doneText := "done"
		if dur := formatDuration(agentDurationMs); dur != "" {
			doneText = dur
		}
		status = styles.ToolSuccess.Render("✓") + styles.ToolSummary.Render(" "+doneText)
	}

	gap := maxW - lipgloss.Width(left) - lipgloss.Width(status)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + status
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
	pinPrefix := ""
	if msg.Pinned {
		pinPrefix = styles.PinIcon.Render("📌 ")
	}

	switch msg.Type {
	case MsgUser:
		content := styles.UserContent.Render(msg.Content)
		block := styles.UserBlock.Render(content)
		return pinPrefix + block

	case MsgAssistant:
		prefix := styles.AssistantPrefix.Render("● ")
		rendered := renderMarkdown(msg.Content, maxW-3)
		// Indent continuation lines to align under prefix
		indented := indentContinuation(rendered, indent)
		return pinPrefix + prefix + indented

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
	Model       string
	Tokens      int
	Cost        float64
	Turns       int
	Streaming   bool
	SpinText    string
	Hint        string
	VimMode     string // "", "NORMAL", "INSERT", "VISUAL"
	SessionName string // current session title (empty = untitled)
	PanelName   string // active panel name (empty = none)
	// Context budget
	ContextUsed int // total tokens used in context window
	ContextMax  int // max context window size
	// Background sessions
	BackgroundSessions int // number of sessions running in background
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

	// Session name
	if s.SessionName != "" {
		sessStyle := lipgloss.NewStyle().Foreground(styles.Aqua)
		parts = append(parts, sessStyle.Render(s.SessionName))
	}

	parts = append(parts, styles.StatusModel.Render(s.Model))

	if s.Streaming && s.SpinText != "" {
		parts = append(parts, styles.StatusActive.Render("● "+s.SpinText))
	}

	// Panel indicator
	if s.PanelName != "" {
		panelStyle := lipgloss.NewStyle().Foreground(styles.Primary)
		parts = append(parts, panelStyle.Render("◧ "+s.PanelName))
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

	// Context budget bar
	if s.ContextMax > 0 && s.ContextUsed > 0 {
		parts = append(parts, renderContextBar(s.ContextUsed, s.ContextMax))
	}

	// Background sessions indicator
	if s.BackgroundSessions > 0 {
		bgStyle := lipgloss.NewStyle().Foreground(styles.Warning).Bold(true)
		parts = append(parts, bgStyle.Render(fmt.Sprintf("⚡%d bg", s.BackgroundSessions)))
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
		// Subtract 4 for glamour's document margin (2 chars each side)
		wrapWidth := width - 4
		if wrapWidth < 10 {
			wrapWidth = 10
		}
		r, err := glamour.NewTermRenderer(
			glamour.WithStylesFromJSONBytes(styles.GruvboxGlamourJSON()),
			glamour.WithWordWrap(wrapWidth),
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

// formatDuration converts milliseconds to a short human-readable string.
// Returns "" when ms < 0 (not tracked). Format: "0.5s", "5s", "1m 23s".
func formatDuration(ms int64) string {
	if ms < 0 {
		return ""
	}
	if ms < 1000 {
		return fmt.Sprintf("%.1fs", float64(ms)/1000)
	}
	secs := ms / 1000
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	mins := secs / 60
	rem := secs % 60
	if rem == 0 {
		return fmt.Sprintf("%dm", mins)
	}
	return fmt.Sprintf("%dm %ds", mins, rem)
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

// renderTurnDivider renders a subtle horizontal rule between conversation turns.
func renderTurnDivider(maxW int) string {
	line := strings.Repeat("─", maxW)
	return lipgloss.NewStyle().Foreground(styles.Subtle).Render(line)
}

// renderAssistantContinuation renders assistant text that follows a tool group.
// Uses a dimmer prefix (no ● ) to visually connect it to the previous response.
func renderAssistantContinuation(msg ChatMessage, maxW int) string {
	pinPrefix := ""
	if msg.Pinned {
		pinPrefix = styles.PinIcon.Render("📌 ")
	}
	prefix := styles.AssistantPrefix.Render("  ")
	rendered := renderMarkdown(msg.Content, maxW-3)
	indented := indentContinuation(rendered, indent)
	return pinPrefix + prefix + indented
}

// renderContextBar renders a compact context budget indicator: [████░░] 72%
func renderContextBar(used, max int) string {
	if max <= 0 {
		return ""
	}
	pct := used * 100 / max
	if pct > 100 {
		pct = 100
	}

	barWidth := 8
	filled := pct * barWidth / 100

	// Color based on pressure
	var barColor lipgloss.Color
	switch {
	case pct >= 90:
		barColor = styles.Error // red — critical
	case pct >= 70:
		barColor = styles.Warning // yellow — caution
	default:
		barColor = styles.Success // green — healthy
	}

	filledStyle := lipgloss.NewStyle().Foreground(barColor)
	emptyStyle := lipgloss.NewStyle().Foreground(styles.Subtle)
	pctStyle := lipgloss.NewStyle().Foreground(barColor)

	bar := filledStyle.Render(strings.Repeat("█", filled)) +
		emptyStyle.Render(strings.Repeat("░", barWidth-filled))

	return fmt.Sprintf("[%s] %s", bar, pctStyle.Render(fmt.Sprintf("%d%%", pct)))
}
