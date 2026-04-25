package blocks

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

var (
	tokBarMutedStyle = lipgloss.NewStyle().Foreground(styles.Muted)
	tokTextStyle     = lipgloss.NewStyle().Foreground(styles.Text)
	tokMutedStyle    = lipgloss.NewStyle().Foreground(styles.Muted)
	tokModelStyle    = lipgloss.NewStyle().Foreground(styles.Primary).Bold(true)
	tokPctStyle      = lipgloss.NewStyle().Foreground(styles.Muted)
)

// TokensBlock shows token usage, cost, model name, and context %.
type TokensBlock struct {
	GetTokens     func() int
	GetCost       func() float64
	GetMaxContext func() int
	GetModel      func() string
}

func NewTokensBlock(getTokens func() int, getCost func() float64, getMaxContext func() int, getModel func() string) *TokensBlock {
	return &TokensBlock{
		GetTokens:     getTokens,
		GetCost:       getCost,
		GetMaxContext: getMaxContext,
		GetModel:      getModel,
	}
}

func (b *TokensBlock) Title() string  { return "Usage" }
func (b *TokensBlock) MinHeight() int { return 3 }
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

	contextLimit := 200_000
	if b.GetMaxContext != nil {
		if v := b.GetMaxContext(); v > 0 {
			contextLimit = v
		}
	}

	model := ""
	if b.GetModel != nil {
		model = b.GetModel()
	}

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
	barFilledStyle := lipgloss.NewStyle().Foreground(barColor)

	filledStr := strings.Repeat("█", filled)
	emptyStr := strings.Repeat("░", barW-filled)
	bar := barFilledStyle.Render(filledStr) + tokBarMutedStyle.Render(emptyStr)

	pctStr := tokPctStyle.Render(fmt.Sprintf("%.0f%%", pct*100))
	tokenStr := tokTextStyle.Render(fmt.Sprintf(" %s / %s", formatTokens(tokens), formatTokens(contextLimit)))
	costStr := tokMutedStyle.Render(fmt.Sprintf(" $%.4f", cost))

	lines := []string{
		" " + bar + " " + pctStr,
		tokenStr + "  " + costStr,
	}

	// Model name line (only if there's room and model is known)
	if model != "" && maxHeight >= 3 {
		// Trim model name to fit
		maxModelW := width - 3
		if len(model) > maxModelW {
			model = model[:maxModelW-1] + "…"
		}
		lines = append(lines, " "+tokModelStyle.Render(model))
	}

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
