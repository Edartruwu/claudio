package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/tui/panels"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// panelSplitRatio is the fraction of terminal width given to the main area
// when a side panel is active. The panel gets (1 - ratio).
const panelSplitRatio = 0.65

// panelMinWidth is the minimum width for a side panel to be shown.
// Below this threshold the panel is hidden to avoid cramped rendering.
const panelMinWidth = 30

// splitLayout renders the main content and an optional side panel side-by-side.
// mainView is typically the viewport; totalHeight should be the viewport height.
func splitLayout(mainView string, panel panels.Panel, totalWidth, totalHeight int) string {
	if panel == nil || !panel.IsActive() {
		return mainView
	}

	mainW := int(float64(totalWidth) * panelSplitRatio)
	panelW := totalWidth - mainW - 1 // 1 for the separator

	if panelW < panelMinWidth {
		return mainView
	}

	panel.SetSize(panelW, totalHeight)
	panelView := panel.View()

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
func mainWidth(totalWidth int, panel panels.Panel) int {
	if panel == nil || !panel.IsActive() {
		return totalWidth
	}
	panelW := totalWidth - int(float64(totalWidth)*panelSplitRatio) - 1
	if panelW < panelMinWidth {
		return totalWidth
	}
	return int(float64(totalWidth) * panelSplitRatio)
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
