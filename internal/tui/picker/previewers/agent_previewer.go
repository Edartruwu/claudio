// Package previewers provides Previewer implementations for the picker.
package previewers

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/teams"
	"github.com/Abraxas-365/claudio/internal/tui/picker"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// TeammateRunner is the minimal interface AgentPreviewer needs.
type TeammateRunner interface {
	GetState(agentID string) (*teams.TeammateState, bool)
}

// AgentPreviewer renders a live agent snapshot in the picker preview pane.
type AgentPreviewer struct {
	runner TeammateRunner
}

// NewAgentPreviewer creates an AgentPreviewer backed by runner.
func NewAgentPreviewer(runner TeammateRunner) *AgentPreviewer {
	return &AgentPreviewer{runner: runner}
}

// Render implements picker.Previewer. Renders header + tools + conversation
// for the agent identified by entry.Meta["agentID"].
func (p *AgentPreviewer) Render(entry picker.Entry, width, height int) string {
	agentID, _ := entry.Meta["agentID"].(string)
	if agentID == "" {
		return dimStyle.Render("no agent ID")
	}

	state, ok := p.runner.GetState(agentID)
	if !ok || state == nil {
		return dimStyle.Render("agent not found")
	}

	rw := width - 2
	if rw < 10 {
		rw = 10
	}

	var b strings.Builder

	// Header
	b.WriteString(renderHeader(state, rw))
	b.WriteString("\n")

	// Tools section
	toolContent := renderToolsSection(state, rw)
	if toolContent != "" {
		b.WriteString(styles.SeparatorLine(rw))
		b.WriteString("\n")
		b.WriteString(toolContent)
		b.WriteString("\n")
	}

	// Conversation section
	convContent := renderConversation(state, rw)
	if convContent != "" {
		b.WriteString(styles.SeparatorLine(rw))
		b.WriteString("\n")
		b.WriteString(convContent)
		b.WriteString("\n")
	}

	// Streaming indicator
	if state.Status == teams.StatusWorking {
		b.WriteString("\n" + warningStyle.Render("  ⟳ streaming") + "\n")
	}

	return b.String()
}

// ---------------------------------------------------------------------------
// Rendering helpers (stateless — no spinner tick, just static icons)
// ---------------------------------------------------------------------------

func renderHeader(state *teams.TeammateState, width int) string {
	var b strings.Builder

	// Line 1: name + status icon + elapsed
	nameStyle := primaryBold
	if state.Identity.Color != "" {
		nameStyle = nameStyle.Foreground(lipgloss.Color(state.Identity.Color))
	}

	icon := statusIcon(state.Status)
	name := nameStyle.Render(state.Identity.AgentName)

	var elapsed string
	if !state.StartedAt.IsZero() {
		var d time.Duration
		if !state.FinishedAt.IsZero() {
			d = state.FinishedAt.Sub(state.StartedAt).Truncate(time.Second)
		} else {
			d = time.Since(state.StartedAt).Truncate(time.Second)
		}
		elapsed = "  " + dimStyle.Render(fmtDuration(d))
	}

	statusLbl := statusLabel(state.Status)
	b.WriteString("  " + icon + " " + name + "  " + statusLbl + elapsed + "\n")

	// Line 2: model + turns
	if state.Model != "" || state.MaxTurns > 0 {
		var parts []string
		if state.Model != "" {
			parts = append(parts, "Model: "+state.Model)
		}
		if state.MaxTurns > 0 {
			prog := state.GetProgress()
			parts = append(parts, fmt.Sprintf("Turn %d/%d", prog.ToolCalls, state.MaxTurns))
		}
		b.WriteString("  " + dimStyle.Render(strings.Join(parts, "  ·  ")) + "\n")
	}

	// Line 3: prompt (120 char cap)
	if state.Prompt != "" {
		task := state.Prompt
		if len(task) > 120 {
			task = task[:117] + "…"
		}
		b.WriteString("  " + dimItalic.Render("Task: "+task) + "\n")
	}

	// Line 4: error
	if state.Status == teams.StatusFailed && state.Error != "" {
		b.WriteString("  " + errorStyle.Render("✗ "+state.Error) + "\n")
	}

	// Line 5: result
	if state.Status == teams.StatusComplete && state.Result != "" {
		result := state.Result
		if len(result) > 200 {
			result = result[:197] + "…"
		}
		b.WriteString("  " + successStyle.Render("✓ "+result) + "\n")
	}

	return b.String()
}

func renderToolsSection(state *teams.TeammateState, width int) string {
	if state == nil {
		return ""
	}

	// Collect tool_start/tool_end entries
	var toolCalls []teams.ConversationEntry
	for _, e := range state.Conversation {
		if e.Type == "tool_start" || e.Type == "tool_end" {
			toolCalls = append(toolCalls, e)
		}
	}
	if len(toolCalls) == 0 {
		return ""
	}
	if len(toolCalls) > 20 {
		toolCalls = toolCalls[len(toolCalls)-20:]
	}

	// Build name → status map
	type toolGroup struct {
		input  string
		status string
	}
	toolMap := make(map[string]*toolGroup)
	for _, e := range toolCalls {
		if e.Type == "tool_start" {
			if _, ok := toolMap[e.ToolName]; !ok {
				toolMap[e.ToolName] = &toolGroup{}
			}
			toolMap[e.ToolName].input = e.Content
			toolMap[e.ToolName].status = "running"
		} else if e.Type == "tool_end" {
			if _, ok := toolMap[e.ToolName]; !ok {
				toolMap[e.ToolName] = &toolGroup{}
			}
			toolMap[e.ToolName].status = "done"
		}
	}

	var b strings.Builder
	b.WriteString(mutedStyle.Render("─── TOOLS ───") + "\n")

	seen := make(map[string]bool)
	for _, e := range toolCalls {
		if e.Type != "tool_start" || seen[e.ToolName] {
			continue
		}
		seen[e.ToolName] = true

		grp := toolMap[e.ToolName]
		if grp == nil {
			continue
		}

		var badge string
		switch grp.status {
		case "running":
			badge = warningStyle.Render("⟳")
		case "done":
			badge = successStyle.Render("✓")
		default:
			badge = " "
		}

		input := grp.input
		if len(input) > 60 {
			input = input[:57] + "…"
		}

		b.WriteString(fmt.Sprintf("  %s %s  %s\n",
			badge,
			lipgloss.NewStyle().Render(e.ToolName),
			styles.ToolSummary.Render(input),
		))
	}

	return b.String()
}

func renderConversation(state *teams.TeammateState, width int) string {
	entries := state.GetConversation()
	if len(entries) == 0 {
		return ""
	}

	// Keep last 30 entries to avoid blowing the viewport
	if len(entries) > 30 {
		entries = entries[len(entries)-30:]
	}

	var b strings.Builder
	b.WriteString(mutedStyle.Render("─── CONVERSATION ───") + "\n")

	for _, e := range entries {
		switch e.Type {
		case "message_in":
			b.WriteString(styles.UserPrefix.Render("> ") + wrapText(e.Content, width) + "\n\n")
		case "text":
			b.WriteString(styles.AssistantPrefix.Render("< ") + wrapText(e.Content, width) + "\n\n")
		case "message_out":
			b.WriteString(aquaStyle.Render("↗ ") + wrapText(e.Content, width) + "\n\n")
		case "tool_start":
			b.WriteString(styles.ToolName.Render("  → "+e.ToolName) + "\n")
			if e.Content != "" {
				b.WriteString(styles.ToolSummary.Render("    "+e.Content) + "\n")
			}
			b.WriteString("\n")
		case "tool_end":
			if e.Content != "" {
				b.WriteString(successStyle.Render("  ← ") + styles.ToolSummary.Render(e.Content) + "\n\n")
			}
		case "complete":
			result := e.Content
			if result == "" {
				result = "done"
			}
			b.WriteString(successStyle.Render("  ✓ complete") + "\n")
			b.WriteString(styles.ToolSummary.Render("  "+result) + "\n\n")
		case "error":
			b.WriteString(styles.ToolError.Render("  ✗ "+e.Content) + "\n\n")
		}
	}

	return b.String()
}

// ---------------------------------------------------------------------------
// Status helpers
// ---------------------------------------------------------------------------

func statusIcon(s teams.MemberStatus) string {
	switch s {
	case teams.StatusWorking:
		return warningStyle.Render("⟳")
	case teams.StatusComplete:
		return successStyle.Render("✓")
	case teams.StatusFailed:
		return errorStyle.Render("✗")
	case teams.StatusShutdown:
		return mutedStyle.Render("⊘")
	case teams.StatusWaitingForInput:
		return warningStyle.Render("?")
	default:
		return dimStyle.Render("○")
	}
}

func statusLabel(s teams.MemberStatus) string {
	var color lipgloss.Color
	switch s {
	case teams.StatusWorking:
		color = styles.Warning
	case teams.StatusComplete:
		color = styles.Success
	case teams.StatusFailed:
		color = styles.Error
	case teams.StatusShutdown, teams.StatusWaitingForInput:
		color = styles.Muted
	default:
		color = styles.Dim
	}
	return lipgloss.NewStyle().Foreground(color).Render(string(s))
}

func fmtDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	mins := int(d.Minutes())
	secs := int(d.Seconds()) % 60
	if secs == 0 {
		return fmt.Sprintf("%dm", mins)
	}
	return fmt.Sprintf("%dm%ds", mins, secs)
}

func wrapText(content string, maxWidth int) string {
	if maxWidth < 4 {
		maxWidth = 4
	}
	lines := strings.Split(content, "\n")
	var out []string
	for _, l := range lines {
		for len(l) > maxWidth {
			out = append(out, l[:maxWidth])
			l = l[maxWidth:]
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

// ---------------------------------------------------------------------------
// Pre-allocated styles
// ---------------------------------------------------------------------------

var (
	primaryBold  = lipgloss.NewStyle().Foreground(styles.Primary).Bold(true)
	dimStyle     = lipgloss.NewStyle().Foreground(styles.Dim)
	dimItalic    = lipgloss.NewStyle().Foreground(styles.Dim).Italic(true)
	mutedStyle   = lipgloss.NewStyle().Foreground(styles.Muted)
	warningStyle = lipgloss.NewStyle().Foreground(styles.Warning)
	successStyle = lipgloss.NewStyle().Foreground(styles.Success)
	errorStyle   = lipgloss.NewStyle().Foreground(styles.Error)
	aquaStyle    = lipgloss.NewStyle().Foreground(styles.Aqua)
)
