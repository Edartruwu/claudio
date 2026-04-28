package picker

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// ── Layout entry points ───────────────────────────────────────────────────────

// renderHorizontal: results pane (left 60%) | preview pane (right 40%).
// Prompt lives at the bottom of the results pane.
// A rounded Tokyo-Night border wraps the entire picker; subtract 2 cols and
// 2 rows from inner dimensions to account for the border.
func renderHorizontal(m Model, width, height int) string {
	innerW := width - 2
	innerH := height - 2
	if innerW < 1 {
		innerW = 1
	}
	if innerH < 1 {
		innerH = 1
	}

	hasPrev := m.cfg.Previewer != nil && len(m.filtered) > 0

	leftW := innerW
	if hasPrev {
		leftW = innerW * 6 / 10
	}
	rightW := innerW - leftW

	left := buildResultsPane(m, leftW, innerH)
	if !hasPrev || rightW <= 0 {
		return pickerBorderStyle.Render(left)
	}

	right := buildPreviewPane(m, rightW, innerH)
	return pickerBorderStyle.Render(lipgloss.JoinHorizontal(lipgloss.Top, left, right))
}

// renderVertical: preview pane (top 40%) → results list → prompt (bottom).
// A rounded Tokyo-Night border wraps the entire picker; subtract 2 cols and
// 2 rows from inner dimensions to account for the border.
func renderVertical(m Model, width, height int) string {
	innerW := width - 2
	innerH := height - 2
	if innerW < 1 {
		innerW = 1
	}
	if innerH < 1 {
		innerH = 1
	}

	hasPrev := m.cfg.Previewer != nil && len(m.filtered) > 0

	if !hasPrev {
		return pickerBorderStyle.Render(buildResultsPane(m, innerW, innerH))
	}

	prevH := innerH * 4 / 10
	if prevH < 3 {
		prevH = 3
	}
	listH := innerH - prevH

	preview := buildPreviewPane(m, innerW, prevH)
	results := buildResultsPane(m, innerW, listH)
	return pickerBorderStyle.Render(lipgloss.JoinVertical(lipgloss.Left, preview, results))
}

// renderDropdown: compact centered modal (~60% wide, ~40% tall), no preview.
func renderDropdown(m Model, width, height int) string {
	boxW := width * 6 / 10
	if boxW < 40 {
		boxW = 40
	}
	if boxW > width {
		boxW = width
	}

	boxH := height * 4 / 10
	if boxH < 6 {
		boxH = 6
	}
	if boxH > height {
		boxH = height
	}

	// Inner dimensions (border consumes 2 chars on each axis).
	innerW := boxW - 2
	innerH := boxH - 2
	if innerW < 1 {
		innerW = 1
	}
	if innerH < 1 {
		innerH = 1
	}

	inner := buildResultsPane(m, innerW, innerH)
	box := dropdownStyle.Width(boxW - 2).Render(inner)

	// Centre the box horizontally and vertically.
	leftPad := (width - boxW) / 2
	if leftPad < 0 {
		leftPad = 0
	}
	topPad := (height - boxH) / 2
	if topPad < 0 {
		topPad = 0
	}

	leftStr := strings.Repeat(" ", leftPad)
	var sb strings.Builder
	emptyLine := strings.Repeat(" ", width)
	for i := 0; i < topPad; i++ {
		sb.WriteString(emptyLine + "\n")
	}
	for _, line := range strings.Split(box, "\n") {
		sb.WriteString(leftStr + line + "\n")
	}
	return sb.String()
}

// renderIvy: full-width bottom overlay (~30% height).
// Prompt at top of strip, results below; preview on right when enabled.
// A rounded Tokyo-Night border wraps the content strip; subtract 2 cols and
// 2 rows from inner dimensions to account for the border.
func renderIvy(m Model, width, height int) string {
	hasPrev := m.cfg.Previewer != nil && len(m.filtered) > 0

	stripH := height * 3 / 10
	if stripH < 4 {
		stripH = 4
	}
	if stripH > height {
		stripH = height
	}

	// Inner dims account for border (2 cols, 2 rows).
	innerW := width - 2
	innerH := stripH - 2
	if innerW < 1 {
		innerW = 1
	}
	if innerH < 1 {
		innerH = 1
	}

	topPad := height - stripH

	var strip string
	if hasPrev {
		leftW := innerW * 6 / 10
		rightW := innerW - leftW
		left := buildResultsPane(m, leftW, innerH)
		right := buildPreviewPane(m, rightW, innerH)
		strip = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	} else {
		strip = buildResultsPane(m, innerW, innerH)
	}

	bordered := pickerBorderStyle.Render(strip)

	var sb strings.Builder
	for i := 0; i < topPad; i++ {
		sb.WriteString("\n")
	}
	sb.WriteString(bordered)
	return sb.String()
}

// ── Pane builders ─────────────────────────────────────────────────────────────

// buildResultsPane renders the prompt + scrollable results list into width×height.
// Layout (top to bottom):
//
//	[title bar]  — 1 line, omitted when cfg.Title == ""
//	[item list]  — fills remaining space minus prompt
//	[prompt]     — always 1 line at the bottom
func buildResultsPane(m Model, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}

	titleH := 0
	if m.cfg.Title != "" {
		titleH = 1
	}
	promptH := 1
	listH := height - titleH - promptH
	if listH < 0 {
		listH = 0
	}

	lines := make([]string, 0, height)

	// ── Title ──
	if m.cfg.Title != "" {
		title := titleStyle.Width(width).Render(" " + m.cfg.Title)
		lines = append(lines, title)
	}

	// ── Item list ──
	// Keep selectedIdx visible: scroll the window when cursor is out of view.
	scrollOffset := 0
	if listH > 0 && m.selectedIdx >= listH {
		scrollOffset = m.selectedIdx - listH + 1
	}

	for row := 0; row < listH; row++ {
		idx := scrollOffset + row
		if idx >= len(m.filtered) {
			// Empty row — pad to preserve layout width.
			lines = append(lines, strings.Repeat(" ", width))
			continue
		}

		entry := m.filtered[idx]
		selected := idx == m.selectedIdx
		inMulti := m.multiSelect[entry.Ordinal]

		// Prefix: multi-selected marker takes priority over selection arrow.
		prefix := "  "
		if inMulti {
			prefix = "● "
		}

		// Display text: prefer Ordinal (plain text) for reliable width math.
		text := entry.Ordinal
		if text == "" {
			text = entry.Display
		}

		line := prefix + text

		var rendered string
		if selected {
			rendered = itemSelectedStyle.Width(width).Render(line)
		} else {
			rendered = itemStyle.Width(width).Render(line)
		}
		lines = append(lines, rendered)
	}

	// ── Prompt ──
	cursor := "█"
	promptLine := promptPrefixStyle.Render("> ") +
		promptTextStyle.Render(m.prompt+cursor)
	lines = append(lines, promptLine)

	return strings.Join(lines, "\n")
}

// buildPreviewPane calls cfg.Previewer.Render and applies previewScroll.
// Returns a blank pane of the correct dimensions when no selection exists.
func buildPreviewPane(m Model, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}

	if m.cfg.Previewer == nil || len(m.filtered) == 0 {
		return previewStyle.Width(width).Height(height).Render("")
	}

	entry := m.filtered[m.selectedIdx]
	content := m.cfg.Previewer.Render(entry, width, height)

	// Apply scroll offset by slicing rendered lines.
	allLines := strings.Split(content, "\n")
	start := m.previewScroll
	if start >= len(allLines) {
		start = max(0, len(allLines)-1)
	}
	end := start + height
	if end > len(allLines) {
		end = len(allLines)
	}
	visible := allLines[start:end]

	// Pad to height so JoinHorizontal aligns correctly.
	for len(visible) < height {
		visible = append(visible, "")
	}

	return previewStyle.Width(width).Render(strings.Join(visible, "\n"))
}

// ── Styles ────────────────────────────────────────────────────────────────────

var (
	// Title bar: Primary bold, full-width background.
	titleStyle = lipgloss.NewStyle().
			Foreground(styles.Primary).
			Bold(true).
			Background(styles.SurfaceAlt)

	// Normal list item: dim foreground.
	itemStyle = lipgloss.NewStyle().
			Foreground(styles.Dim)

	// Selected list item: bright text + bold + left border accent.
	itemSelectedStyle = lipgloss.NewStyle().
				Foreground(styles.Text).
				Bold(true).
				BorderStyle(lipgloss.Border{Left: "▌"}).
				BorderLeft(true).
				BorderForeground(styles.Primary)

	// Prompt prefix "> ".
	promptPrefixStyle = lipgloss.NewStyle().
				Foreground(styles.Primary).
				Bold(true)

	// Prompt input text.
	promptTextStyle = lipgloss.NewStyle().
			Foreground(styles.Text)

	// Preview pane: dim muted text, left border separator.
	previewStyle = lipgloss.NewStyle().
			Foreground(styles.Dim).
			BorderStyle(lipgloss.Border{Left: "│"}).
			BorderLeft(true).
			BorderForeground(styles.Subtle)

	// Dropdown modal box: rounded border.
	dropdownStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(styles.SurfaceAlt).
			Padding(0, 1)

	// Outer picker border — Telescope-style rounded border.
	// Applied by renderHorizontal, renderVertical, and renderIvy as the last
	// compositing step so the entire picker (title + list + preview + prompt)
	// appears inside a single unified floating window frame.
	pickerBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(styles.SurfaceAlt)
)
