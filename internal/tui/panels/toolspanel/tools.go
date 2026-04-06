// Package toolspanel implements the tools manager side panel.
//
// It lists every registered tool, splits them into "Eager" (loaded with full
// schema on every request) and "Deferred" (loaded on-demand via ToolSearch),
// and lets the user toggle which group a *deferrable* tool belongs to. Tools
// that don't implement DeferrableTool (Bash, Read, Write, Edit, Glob, Grep,
// Agent, plan-mode, ToolSearch) are always eager and cannot be toggled.
package toolspanel

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/tools"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// Panel is the tool manager side panel.
type Panel struct {
	registry *tools.Registry

	active    bool
	width     int
	height    int
	cursor    int
	items     []item
	query     string
	searching bool
}

// item is a single row in the panel.
type item struct {
	name       string
	deferred   bool // current effective state
	deferrable bool // can the user toggle it?
	overridden bool // user has set an explicit override
	hint       string
}

// New creates a new tools panel.
func New(reg *tools.Registry) *Panel {
	return &Panel{registry: reg}
}

func (p *Panel) IsActive() bool { return p.active }

func (p *Panel) Activate() {
	p.active = true
	p.cursor = 0
	p.query = ""
	p.searching = false
	p.refresh()
}

func (p *Panel) Deactivate() { p.active = false }

func (p *Panel) SetSize(w, h int) {
	p.width = w
	p.height = h
}

func (p *Panel) refresh() {
	names := p.registry.Names()
	hints := p.registry.ToolSearchHints()

	all := make([]item, 0, len(names))
	for _, n := range names {
		all = append(all, item{
			name:       n,
			deferred:   p.registry.IsDeferred(n),
			deferrable: p.registry.IsDeferrable(n),
			overridden: p.registry.HasDeferOverride(n),
			hint:       hints[n],
		})
	}

	// Sort: eager group first (alphabetical), then deferred group (alphabetical).
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].deferred != all[j].deferred {
			return !all[i].deferred // eager before deferred
		}
		return all[i].name < all[j].name
	})

	if p.query == "" {
		p.items = all
	} else {
		p.items = nil
		q := strings.ToLower(p.query)
		for _, it := range all {
			if strings.Contains(strings.ToLower(it.name), q) ||
				strings.Contains(strings.ToLower(it.hint), q) {
				p.items = append(p.items, it)
			}
		}
	}
	if p.cursor >= len(p.items) {
		p.cursor = max(0, len(p.items)-1)
	}
}

func (p *Panel) Update(msg tea.KeyMsg) (tea.Cmd, bool) {
	key := msg.String()

	if p.searching {
		switch key {
		case "esc":
			p.searching = false
			p.query = ""
			p.refresh()
			return nil, true
		case "enter":
			p.searching = false
			return nil, true
		case "backspace":
			if len(p.query) > 0 {
				p.query = p.query[:len(p.query)-1]
				p.refresh()
			}
			return nil, true
		default:
			if len(key) == 1 {
				p.query += key
				p.refresh()
				return nil, true
			}
		}
		return nil, true
	}

	switch key {
	case "j", "down":
		if p.cursor < len(p.items)-1 {
			p.cursor++
		}
		return nil, true
	case "k", "up":
		if p.cursor > 0 {
			p.cursor--
		}
		return nil, true
	case "G":
		p.cursor = max(0, len(p.items)-1)
		return nil, true
	case "g":
		p.cursor = 0
		return nil, true
	case "/":
		p.searching = true
		p.query = ""
		return nil, true
	case "enter", " ", "x":
		p.toggleCurrent()
		return nil, true
	case "r":
		// Reset override on the current row.
		if p.cursor < len(p.items) {
			it := p.items[p.cursor]
			if it.overridden {
				p.registry.ClearDeferOverride(it.name)
				p.refresh()
			}
		}
		return nil, true
	}

	return nil, false
}

// toggleCurrent flips the deferred state of the highlighted tool, if it's
// togglable. Non-deferrable tools are silently ignored.
func (p *Panel) toggleCurrent() {
	if p.cursor >= len(p.items) {
		return
	}
	it := p.items[p.cursor]
	if !it.deferrable {
		return
	}
	p.registry.SetDeferOverride(it.name, !it.deferred)
	p.refresh()
}

func (p *Panel) View() string {
	if !p.active {
		return ""
	}

	var b strings.Builder

	title := styles.PanelTitle.Render("Tools")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(styles.SeparatorLine(p.width))
	b.WriteString("\n")

	if p.searching {
		b.WriteString(styles.PanelSearch.Render(" / "))
		b.WriteString(lipgloss.NewStyle().Foreground(styles.Text).Render(p.query))
		b.WriteString(lipgloss.NewStyle().Foreground(styles.Warning).Render("▋"))
		b.WriteString("\n")
	}

	if len(p.items) == 0 {
		b.WriteString(styles.PanelHint.Render("  No tools found"))
		b.WriteString("\n")
	}

	listH := p.height - 6
	if p.searching {
		listH--
	}
	if listH < 3 {
		listH = 3
	}

	startIdx := 0
	if p.cursor >= listH {
		startIdx = p.cursor - listH + 1
	}
	endIdx := startIdx + listH
	if endIdx > len(p.items) {
		endIdx = len(p.items)
	}

	var prevDeferred = -1 // 0 eager, 1 deferred — for group separators
	for i := startIdx; i < endIdx; i++ {
		it := p.items[i]
		curGroup := 0
		if it.deferred {
			curGroup = 1
		}
		if curGroup != prevDeferred {
			label := "EAGER"
			if it.deferred {
				label = "DEFERRED"
			}
			b.WriteString(styles.PanelHint.Render("  " + label))
			b.WriteString("\n")
			prevDeferred = curGroup
		}

		selected := i == p.cursor

		prefix := "  "
		if selected {
			prefix = styles.ViewportCursor.Render("▸ ")
		}

		// State badge.
		var badge string
		if !it.deferrable {
			badge = styles.PanelBadgeBundled.Render(" core ")
		} else if it.deferred {
			badge = styles.PanelBadgeProject.Render(" deferred ")
		} else {
			badge = styles.PanelBadgeUser.Render(" eager ")
		}

		nameStyle := styles.PanelItem
		if selected {
			nameStyle = styles.PanelItemSelected
		}

		name := nameStyle.Render(it.name)
		line1 := prefix + name + " " + badge
		if it.overridden {
			line1 += " " + lipgloss.NewStyle().Foreground(styles.Warning).Render("●")
		}
		b.WriteString(line1)
		b.WriteString("\n")

		if it.hint != "" {
			hint := lipgloss.NewStyle().Foreground(styles.Dim).Render(it.hint)
			b.WriteString("    " + hint)
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(styles.PanelHint.Render("  enter/space toggle · r reset · / search · esc close"))

	return lipgloss.NewStyle().
		Width(p.width).
		Height(p.height).
		Render(b.String())
}
