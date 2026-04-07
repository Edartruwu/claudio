// Package sidebar provides a composable right-side panel made of stacked blocks.
package sidebar

// Block is a self-contained widget that can be placed inside the sidebar.
type Block interface {
	// Title returns the section header (shown above the block).
	Title() string
	// Render draws the block content into width×maxHeight cells.
	// It must not exceed maxHeight lines.
	Render(width, maxHeight int) string
	// MinHeight is the minimum number of lines this block needs to be useful.
	MinHeight() int
	// Weight controls how much vertical space this block gets relative to others.
	Weight() int
}
