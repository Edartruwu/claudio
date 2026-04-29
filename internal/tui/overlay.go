package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// overlayCenter composites overlay on top of base, centered in the terminal
// viewport (totalWidth × totalHeight). Unlike lipgloss.Place, this keeps the
// background content visible around the overlay instead of replacing it with
// whitespace. Both strings are multi-line (newline-separated).
func overlayCenter(base, overlay string, totalWidth, totalHeight int) string {
	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")

	oh := len(overlayLines)

	// max visual width of overlay (ANSI-aware)
	ow := 0
	for _, l := range overlayLines {
		if w := ansi.StringWidth(l); w > ow {
			ow = w
		}
	}

	top := (totalHeight - oh) / 2
	left := (totalWidth - ow) / 2
	if top < 0 {
		top = 0
	}
	if left < 0 {
		left = 0
	}

	// ensure base has at least totalHeight lines
	for len(baseLines) < totalHeight {
		baseLines = append(baseLines, "")
	}

	result := make([]string, len(baseLines))
	copy(result, baseLines)

	for i, oLine := range overlayLines {
		bRow := top + i
		if bRow < 0 || bRow >= len(result) {
			continue
		}

		bLine := result[bRow]

		// pad base line to full width so splice always has characters
		bw := ansi.StringWidth(bLine)
		if bw < totalWidth {
			bLine += strings.Repeat(" ", totalWidth-bw)
		}

		lw := ansi.StringWidth(oLine)
		leftPart := ansi.Truncate(bLine, left, "")
		rightPart := ansi.TruncateLeft(bLine, left+lw, "")

		result[bRow] = leftPart + oLine + rightPart
	}

	return strings.Join(result, "\n")
}

// overlayBottomLeft composites overlay on top of base, anchored to the
// bottom-left corner of the container (totalWidth × totalHeight).
// The overlay floats over the background content — no layout shift.
// Both strings are multi-line (newline-separated). ANSI-aware.
func overlayBottomLeft(base, overlay string, totalWidth, totalHeight int) string {
	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")

	oh := len(overlayLines)

	// max visual width of overlay (ANSI-aware)
	ow := 0
	for _, l := range overlayLines {
		if w := ansi.StringWidth(l); w > ow {
			ow = w
		}
	}

	// Pad base to fill the container height
	for len(baseLines) < totalHeight {
		baseLines = append(baseLines, "")
	}

	startRow := totalHeight - oh
	if startRow < 0 {
		startRow = 0
	}

	result := make([]string, len(baseLines))
	copy(result, baseLines)

	for i, oLine := range overlayLines {
		row := startRow + i
		if row < 0 || row >= len(result) {
			continue
		}

		bLine := result[row]
		bw := ansi.StringWidth(bLine)

		// Overlay left portion; keep right portion of base if wider
		if bw > ow {
			rightPart := ansi.TruncateLeft(bLine, ow, "")
			result[row] = oLine + rightPart
		} else {
			result[row] = oLine
		}
	}

	if len(result) > totalHeight {
		result = result[:totalHeight]
	}

	return strings.Join(result, "\n")
}
