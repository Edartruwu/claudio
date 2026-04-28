package sidebar

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

var (
	sidebarTitleStyle = lipgloss.NewStyle().Foreground(styles.Primary).Bold(true)
	sidebarSepStyle   = lipgloss.NewStyle().Foreground(styles.SurfaceAlt)
)

// Sidebar stacks multiple Blocks vertically in a fixed-width column.
// All blocks are sourced from the BlockRegistry, which can be updated at any
// time — View() always reads the current snapshot.
type Sidebar struct {
	registry *BlockRegistry
	width    int
	height   int
}

// New creates a sidebar backed by the given registry.
func New(registry *BlockRegistry) *Sidebar {
	return &Sidebar{registry: registry}
}

// SetSize tells the sidebar its available dimensions.
func (s *Sidebar) SetSize(w, h int) {
	s.width = w
	s.height = h
}

// Width returns the configured width.
func (s *Sidebar) Width() int { return s.width }

// View renders all blocks stacked vertically.
func (s *Sidebar) View() string {
	if s.registry == nil {
		return ""
	}
	blocks := s.registry.Blocks()
	if s.width < 8 || s.height < 4 || len(blocks) == 0 {
		return ""
	}

	// Filter out blocks that have no content (MinHeight == 0 would skip)
	live := make([]Block, 0, len(blocks))
	for _, b := range blocks {
		if b.MinHeight() > 0 {
			live = append(live, b)
		}
	}
	if len(live) == 0 {
		return ""
	}

	// Reserve 2 lines per block for the title + separator
	headerLines := len(live) * 2
	contentLines := s.height - headerLines
	if contentLines < len(live) {
		contentLines = len(live)
	}

	// Distribute content lines by weight
	totalWeight := 0
	for _, b := range live {
		totalWeight += b.Weight()
	}
	if totalWeight == 0 {
		totalWeight = len(live)
	}

	heights := make([]int, len(live))
	remaining := contentLines
	for i, b := range live {
		if i == len(live)-1 {
			heights[i] = remaining
		} else {
			h := contentLines * b.Weight() / totalWeight
			if h < b.MinHeight() {
				h = b.MinHeight()
			}
			heights[i] = h
			remaining -= h
		}
		if remaining < 0 {
			remaining = 0
		}
	}

	sepW := s.width - 1
	if sepW < 1 {
		sepW = 1
	}
	sep := sidebarSepStyle.Render(strings.Repeat("─", sepW))

	var parts []string
	for i, b := range live {
		title := sidebarTitleStyle.Render(" " + b.Title())
		parts = append(parts, title)
		parts = append(parts, sep)
		content := b.Render(s.width, heights[i])
		parts = append(parts, content)
		if i < len(live)-1 {
			parts = append(parts, "")
		}
	}

	return strings.Join(parts, "\n")
}
