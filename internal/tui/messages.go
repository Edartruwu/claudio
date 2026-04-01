package tui

import (
	"fmt"
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
	Type      MessageType
	Content   string
	ToolName  string
	ToolInput string
	IsError   bool
}

// renderMessage renders a ChatMessage to a styled string.
func renderMessage(msg ChatMessage, width int) string {
	maxWidth := width - 4
	if maxWidth < 40 {
		maxWidth = 40
	}

	switch msg.Type {
	case MsgUser:
		prefix := styles.UserPrefix.String()
		content := styles.UserMessage.Width(maxWidth).Render(msg.Content)
		return prefix + content

	case MsgAssistant:
		prefix := styles.AssistantPrefix.String()
		rendered := renderMarkdown(msg.Content, maxWidth)
		return prefix + rendered

	case MsgToolUse:
		header := styles.ToolHeader.Render("⚡ " + msg.ToolName)
		input := styles.ToolInput.Width(maxWidth).Render(truncate(msg.ToolInput, 200))
		return header + "\n" + input

	case MsgToolResult:
		if msg.IsError {
			return styles.ToolError.Width(maxWidth).Render("✗ " + truncate(msg.Content, 500))
		}
		content := truncate(msg.Content, 500)
		lines := strings.Split(content, "\n")
		if len(lines) > 10 {
			content = strings.Join(lines[:10], "\n") + fmt.Sprintf("\n... (%d more lines)", len(lines)-10)
		}
		return styles.ToolResult.Width(maxWidth).Render("✓ " + content)

	case MsgThinking:
		return styles.ThinkingStyle.Width(maxWidth).Render("💭 " + truncate(msg.Content, 200))

	case MsgError:
		return styles.ErrorStyle.Width(maxWidth).Render("✗ " + msg.Content)

	case MsgSystem:
		return styles.SpinnerText.Width(maxWidth).Render("ℹ " + msg.Content)

	default:
		return msg.Content
	}
}

var mdRenderer *glamour.TermRenderer

func renderMarkdown(content string, width int) string {
	if content == "" {
		return ""
	}

	// Lazy init renderer
	if mdRenderer == nil {
		r, err := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(width),
		)
		if err != nil {
			return content
		}
		mdRenderer = r
	}

	rendered, err := mdRenderer.Render(content)
	if err != nil {
		return content
	}

	return strings.TrimSpace(rendered)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// renderFooter renders the status footer with pills.
func renderFooter(width int, model string, tokens int, cost float64, mode string) string {
	modelPill := styles.FooterPill.Render(model)
	tokenPill := styles.FooterPill.Render(fmt.Sprintf("%dk tokens", tokens/1000))
	costPill := styles.FooterPill.Render(fmt.Sprintf("$%.2f", cost))

	var modePill string
	if mode != "" {
		modePill = styles.FooterPillActive.Render(mode)
	}

	left := lipgloss.JoinHorizontal(lipgloss.Center, modelPill, tokenPill, costPill, modePill)

	helpPill := styles.FooterPill.Render("? help")
	right := helpPill

	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}

	return left + strings.Repeat(" ", gap) + right
}
