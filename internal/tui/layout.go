package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/tui/panels"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// panelMinWidth is the minimum width for a side panel to be shown.
// Below this threshold the panel is hidden to avoid cramped rendering.
const panelMinWidth = 30

// splitLayout renders the main content and an optional side panel side-by-side.
// mainView is typically the viewport; totalHeight should be the viewport height.
// splitRatio is the fraction of totalWidth given to the main area.
func splitLayout(mainView string, panel panels.Panel, totalWidth, totalHeight int, splitRatio float64) string {
	if panel == nil || !panel.IsActive() {
		return mainView
	}

	mainW := int(float64(totalWidth) * splitRatio)
	panelW := totalWidth - mainW - 1 // 1 for the separator

	if panelW < panelMinWidth {
		return mainView
	}

	// Reserve one line for the help footer if the panel provides hints.
	helpText := panel.Help()
	panelContentH := totalHeight
	if helpText != "" {
		panelContentH = totalHeight - 1
	}

	panel.SetSize(panelW, panelContentH)
	panelView := panel.View()

	// Render optional help footer as a dim 1-line bar.
	if helpText != "" {
		footer := lipgloss.NewStyle().
			Width(panelW).
			Foreground(styles.Muted).
			Render(helpText)
		panelView = lipgloss.JoinVertical(lipgloss.Left, panelView, footer)
	}

	// Force both sides to the exact same height so JoinHorizontal aligns
	mainStyled := lipgloss.NewStyle().
		Width(mainW).
		Height(totalHeight).
		Render(mainView)

	panelStyled := lipgloss.NewStyle().
		Width(panelW).
		Height(totalHeight).
		Render(panelView)

	sep := buildSeparator(totalHeight)

	return lipgloss.JoinHorizontal(lipgloss.Top, mainStyled, sep, panelStyled)
}

// mainWidth returns the width available for the main chat area,
// accounting for an active side panel.
// splitRatio is the fraction of totalWidth given to the main area.
func mainWidth(totalWidth int, panel panels.Panel, splitRatio float64) int {
	if panel == nil || !panel.IsActive() {
		return totalWidth
	}
	panelW := totalWidth - int(float64(totalWidth)*splitRatio) - 1
	if panelW < panelMinWidth {
		return totalWidth
	}
	return int(float64(totalWidth) * splitRatio)
}

// buildSeparator creates a thin vertical line of the given height.
func buildSeparator(height int) string {
	style := lipgloss.NewStyle().Foreground(styles.Muted)
	lines := make([]string, height)
	for i := range lines {
		lines[i] = style.Render("│")
	}
	return strings.Join(lines, "\n")
}
