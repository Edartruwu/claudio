// Package sidebar — PanelHost renders Lua-registered panels at a given position.
package sidebar

import (
	"sort"
	"strings"

	"github.com/Abraxas-365/claudio/internal/lua"
	"github.com/charmbracelet/lipgloss"
)

// PanelHost renders panels sourced from a lua.PanelRegistry at a given position.
type PanelHost struct {
	registry *lua.PanelRegistry
}

// New creates a PanelHost backed by the given registry.
// registry may be nil — the host silently renders nothing.
func New(registry *lua.PanelRegistry) *PanelHost {
	return &PanelHost{registry: registry}
}

// HasPanels reports whether any visible panels are registered at position.
func (h *PanelHost) HasPanels(position string) bool {
	if h.registry == nil {
		return false
	}
	return len(h.registry.Panels(position)) > 0
}

// Width returns the preferred width of the widest visible panel at position.
// If no panels are registered or all have Width==0, defaultW is returned.
func (h *PanelHost) Width(position string, defaultW int) int {
	if h.registry == nil {
		return defaultW
	}
	panels := h.registry.Panels(position)
	if len(panels) == 0 {
		return 0
	}
	max := 0
	for _, p := range panels {
		p.Mu.Lock()
		w := p.Width
		p.Mu.Unlock()
		if w > max {
			max = w
		}
	}
	if max == 0 {
		return defaultW
	}
	return max
}

// View renders all visible panels at position into a string of dimensions (width × height).
// Panels are stacked vertically; sections within each panel share height by Weight.
func (h *PanelHost) View(position string, width, height int) string {
	if h.registry == nil || width <= 0 || height <= 0 {
		return ""
	}

	panels := h.registry.Panels(position)
	if len(panels) == 0 {
		return ""
	}

	var out []string
	remainingH := height

	for _, p := range panels {
		p.Mu.Lock()
		sections := make([]*lua.SectionDef, len(p.Sections))
		copy(sections, p.Sections)
		p.Mu.Unlock()

		if len(sections) == 0 {
			continue
		}

		// Sort sections by weight ascending (lower = higher priority / more space).
		sort.Slice(sections, func(i, j int) bool {
			return sections[i].Weight < sections[j].Weight
		})

		totalWeight := 0
		for _, s := range sections {
			totalWeight += s.Weight
		}
		if totalWeight <= 0 {
			totalWeight = 1
		}

		for _, sec := range sections {
			if remainingH <= 0 {
				break
			}

			// Proportional height by weight, respecting MinHeight.
			sectionH := (sec.Weight * remainingH) / totalWeight
			if sectionH < sec.MinHeight {
				sectionH = sec.MinHeight
			}
			if sectionH > remainingH {
				sectionH = remainingH
			}

			// Render section header if it has a title.
			if sec.Title != "" {
				headerStyle := lipgloss.NewStyle().
					Width(width).
					Bold(true).
					Foreground(lipgloss.Color("#928374"))
				out = append(out, headerStyle.Render(sec.Title))
				sectionH--
				remainingH--
				if sectionH <= 0 {
					continue
				}
			}

			// Render section body.
			content := sec.Render(width, sectionH)
			if content != "" {
				out = append(out, content)
			}

			remainingH -= sectionH
		}
	}

	return strings.Join(out, "\n")
}
