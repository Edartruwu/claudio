// Package analyticspanel implements the analytics dashboard side panel.
package analyticspanel

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/services/analytics"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// Panel is the analytics dashboard side panel.
type Panel struct {
	tracker *analytics.Tracker

	active bool
	width  int
	height int
}

// New creates a new analytics dashboard panel.
func New(tracker *analytics.Tracker) *Panel {
	return &Panel{tracker: tracker}
}

func (p *Panel) IsActive() bool { return p.active }
func (p *Panel) Activate()      { p.active = true }
func (p *Panel) Deactivate()    { p.active = false }

func (p *Panel) SetSize(w, h int) {
	p.width = w
	p.height = h
}

func (p *Panel) Update(msg tea.KeyMsg) (tea.Cmd, bool) {
	return nil, false
}

func (p *Panel) View() string {
	if !p.active {
		return ""
	}

	var b strings.Builder

	title := styles.PanelTitle.Render("Analytics")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(styles.SeparatorLine(p.width))
	b.WriteString("\n\n")

	labelStyle := lipgloss.NewStyle().Foreground(styles.Dim).Width(16)
	valStyle := lipgloss.NewStyle().Foreground(styles.Text).Bold(true)

	inputTok := p.tracker.InputTokens()
	outputTok := p.tracker.OutputTokens()
	cacheRead := p.tracker.CacheReadTokens()
	cacheCreate := p.tracker.CacheCreateTokens()
	totalTok := p.tracker.CumulativeTokens()
	cost := p.tracker.Cost()
	cacheHitRate := p.tracker.CacheHitRate()

	// Token breakdown
	b.WriteString(labelStyle.Render("  Input tokens"))
	b.WriteString(valStyle.Render(formatTokens(inputTok)))
	b.WriteString("\n")

	b.WriteString(labelStyle.Render("  Output tokens"))
	b.WriteString(valStyle.Render(formatTokens(outputTok)))
	b.WriteString("\n")

	cacheStyle := lipgloss.NewStyle().Foreground(styles.Success).Bold(true)
	b.WriteString(labelStyle.Render("  Cache read"))
	b.WriteString(cacheStyle.Render(formatTokens(cacheRead)))
	b.WriteString("\n")

	b.WriteString(labelStyle.Render("  Cache create"))
	b.WriteString(valStyle.Render(formatTokens(cacheCreate)))
	b.WriteString("\n")

	b.WriteString(labelStyle.Render("  Total tokens"))
	b.WriteString(valStyle.Render(formatTokens(totalTok)))
	b.WriteString("\n")

	b.WriteString(labelStyle.Render("  Cache hit rate"))
	hitColor := styles.Error
	if cacheHitRate >= 80 {
		hitColor = styles.Success
	} else if cacheHitRate >= 50 {
		hitColor = styles.Warning
	}
	hitStyle := lipgloss.NewStyle().Foreground(hitColor).Bold(true)
	b.WriteString(hitStyle.Render(fmt.Sprintf("%.1f%%", cacheHitRate)))
	b.WriteString("\n\n")

	// Cost
	costStyle := lipgloss.NewStyle().Foreground(styles.Warning).Bold(true)
	b.WriteString(labelStyle.Render("  Cost"))
	b.WriteString(costStyle.Render(fmt.Sprintf("$%.4f", cost)))
	b.WriteString("\n\n")

	// Budget gauge
	warning, exceeded := p.tracker.CheckBudget()
	if warning != "" || exceeded {
		b.WriteString(styles.SeparatorLine(p.width))
		b.WriteString("\n")
		b.WriteString(labelStyle.Render("  Budget"))

		maxBudget := p.tracker.MaxBudget()
		if maxBudget > 0 {
			pct := cost / maxBudget
			if pct > 1 {
				pct = 1
			}
			gauge := renderGauge(pct, p.width-20)
			b.WriteString(gauge)
			b.WriteString(fmt.Sprintf(" %.0f%%", pct*100))
		}
		b.WriteString("\n")

		if exceeded {
			b.WriteString(styles.ErrorStyle.Render("  ⚠ Budget exceeded!"))
			b.WriteString("\n")
		} else if warning != "" {
			warnStyle := lipgloss.NewStyle().Foreground(styles.Warning)
			b.WriteString(warnStyle.Render("  ⚠ " + warning))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(styles.PanelHint.Render("  esc close"))

	return lipgloss.NewStyle().
		Width(p.width).
		Height(p.height).
		Render(b.String())
}

func formatTokens(n int) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

func renderGauge(pct float64, width int) string {
	if width < 5 {
		width = 5
	}
	filled := int(pct * float64(width))
	if filled > width {
		filled = width
	}

	var color lipgloss.Color
	switch {
	case pct >= 0.9:
		color = styles.Error
	case pct >= 0.7:
		color = styles.Warning
	default:
		color = styles.Success
	}

	filledStyle := lipgloss.NewStyle().Foreground(color)
	emptyStyle := lipgloss.NewStyle().Foreground(styles.Subtle)

	return "[" + filledStyle.Render(strings.Repeat("█", filled)) +
		emptyStyle.Render(strings.Repeat("░", width-filled)) + "]"
}

// Help returns a short keybinding hint line for the panel footer.
func (p *Panel) Help() string {
	return "esc close"
}
