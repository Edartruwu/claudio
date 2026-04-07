package blocks

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// TokensBlock shows token usage and estimated cost.
type TokensBlock struct {
	GetTokens func() int
	GetCost   func() float64
}

func NewTokensBlock(getTokens func() int, getCost func() float64) *TokensBlock {
	return &TokensBlock{GetTokens: getTokens, GetCost: getCost}
}

func (b *TokensBlock) Title() string  { return "Usage" }
func (b *TokensBlock) MinHeight() int { return 2 }
func (b *TokensBlock) Weight() int    { return 1 }

func (b *TokensBlock) Render(width, maxHeight int) string {
	tokens := 0
	cost := 0.0
	if b.GetTokens != nil {
		tokens = b.GetTokens()
	}
	if b.GetCost != nil {
		cost = b.GetCost()
	}

	// Context limit bar (200k for Claude Sonnet as baseline)
	const contextLimit = 200_000
	pct := float64(tokens) / float64(contextLimit)
	if pct > 1 {
		pct = 1
	}

	barW := width - 4
	if barW < 4 {
		barW = 4
	}
	filled := int(pct * float64(barW))

	barColor := styles.Success
	if pct > 0.75 {
		barColor = styles.Warning
	}
	if pct > 0.90 {
		barColor = styles.Error
	}

	filledStr := strings.Repeat("█", filled)
	emptyStr := strings.Repeat("░", barW-filled)
	bar := lipgloss.NewStyle().Foreground(barColor).Render(filledStr) +
		lipgloss.NewStyle().Foreground(styles.Muted).Render(emptyStr)

	tokenStr := lipgloss.NewStyle().Foreground(styles.Text).Render(
		fmt.Sprintf(" %s / 200k", formatTokens(tokens)),
	)
	costStr := lipgloss.NewStyle().Foreground(styles.Muted).Render(
		fmt.Sprintf(" $%.4f", cost),
	)

	lines := []string{
		" " + bar,
		tokenStr + "  " + costStr,
	}

	// Trim to maxHeight
	if len(lines) > maxHeight {
		lines = lines[:maxHeight]
	}
	return strings.Join(lines, "\n")
}

func formatTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}
