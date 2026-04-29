package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/tui/panels"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

var (
	separatorStyle  = lipgloss.NewStyle().Foreground(styles.Muted)
	panelFooterBase = lipgloss.NewStyle().Foreground(styles.Muted)
)

// buildSeparator creates a thin vertical line of the given height.
func buildSeparator(height int) string {
	if height <= 0 {
		return ""
	}
	lines := make([]string, height)
	for i := range lines {
		lines[i] = separatorStyle.Render("│")
	}
	return strings.Join(lines, "\n")
}

// placeOverlayAt renders an overlay on top of a base string at the given (x, y)
// position within a container of the specified width and height.
func placeOverlayAt(base, overlay string, x, y, width, height int) string {
	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")

	// Pad base to fill the container height
	for len(baseLines) < height {
		baseLines = append(baseLines, strings.Repeat(" ", width))
	}

	for i, ol := range overlayLines {
		row := y + i
		if row < 0 || row >= len(baseLines) {
			continue
		}

		baseLine := baseLines[row]
		// Expand base line to full width using runes for correct handling
		baseRunes := []rune(baseLine)
		for len(baseRunes) < width {
			baseRunes = append(baseRunes, ' ')
		}

		overlayRunes := []rune(ol)
		// Place overlay runes at position x
		for j, r := range overlayRunes {
			col := x + j
			if col >= 0 && col < len(baseRunes) {
				baseRunes[col] = r
			}
		}
		baseLines[row] = string(baseRunes)
	}

	// Truncate to container height
	if len(baseLines) > height {
		baseLines = baseLines[:height]
	}

	return strings.Join(baseLines, "\n")
}

// renderPanelWithHelp sizes a panel (reserving space for its help footer and
// optional input bar if present) and renders the view plus extras.
// The caller should NOT call panel.SetSize before this — renderPanelWithHelp handles it.
func renderPanelWithHelp(panel panels.Panel, w, h int) string {
	helpText := panel.Help()
	if h < 2 {
		helpText = ""
	}

	// Detect optional input bar — check before SetSize so we can shrink contentH.
	hasInput := false
	if ip, ok := panel.(panels.InputPanel); ok && ip.HasInput() {
		hasInput = true
	}

	contentH := h
	if helpText != "" {
		contentH = h - 1
		if contentH < 1 {
			contentH = 1
		}
	}
	if hasInput {
		contentH--
		if contentH < 1 {
			contentH = 1
		}
	}

	panel.SetSize(w, contentH)
	panelView := panel.View()

	if helpText == "" && !hasInput {
		return panelView
	}

	parts := []string{panelView}
	if hasInput {
		// InputView is called after SetSize so panel.width is already current.
		ip := panel.(panels.InputPanel)
		parts = append(parts, ip.InputView())
	}
	if helpText != "" {
		parts = append(parts, panelFooterBase.Width(w).Render(helpText))
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
