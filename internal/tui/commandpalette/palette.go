package commandpalette

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

var (
	paletteWrapStyle        = lipgloss.NewStyle().Padding(0, 1)
	paletteRowSelectedBase  = lipgloss.NewStyle().Foreground(styles.Primary).Bold(true)
)

// Item represents a command in the palette.
type Item struct {
	Name        string
	Description string
}

// SelectMsg is sent when the user selects a command (enter).
type SelectMsg struct {
	Name string
}

// CompleteMsg is sent when the user tab-completes a command into the prompt.
type CompleteMsg struct {
	Name string
}

// Model is the command palette component.
type Model struct {
	items    []Item
	filtered []Item
	selected int
	query    string
	active   bool
	width    int
	maxItems int
}

// New creates a new command palette.
func New(items []Item) Model {
	return Model{
		items:    items,
		filtered: items,
		maxItems: 10,
	}
}

// SetWidth updates the display width.
func (m *Model) SetWidth(w int) {
	m.width = w
}

// IsActive returns whether the palette is visible.
func (m Model) IsActive() bool {
	return m.active
}

// Activate shows the palette and filters with the given query.
func (m *Model) Activate(query string) {
	m.active = true
	m.query = query
	m.filter()
	m.selected = 0
}

// Deactivate hides the palette.
func (m *Model) Deactivate() {
	m.active = false
	m.query = ""
	m.selected = 0
}

// Update handles key events when the palette is active.
func (m *Model) Update(msg tea.KeyMsg) (tea.Cmd, bool) {
	if !m.active {
		return nil, false
	}

	switch msg.String() {
	case "down", "ctrl+n":
		if len(m.filtered) > 0 {
			m.selected = (m.selected + 1) % len(m.filtered)
		}
		return nil, true

	case "up", "ctrl+p":
		if len(m.filtered) > 0 {
			m.selected--
			if m.selected < 0 {
				m.selected = len(m.filtered) - 1
			}
		}
		return nil, true

	case "tab":
		if len(m.filtered) > 0 && m.selected < len(m.filtered) {
			return func() tea.Msg {
				return CompleteMsg{Name: m.filtered[m.selected].Name}
			}, true
		}
		return nil, true

	case "enter":
		if len(m.filtered) > 0 && m.selected < len(m.filtered) {
			return func() tea.Msg {
				return SelectMsg{Name: m.filtered[m.selected].Name}
			}, true
		}
		return nil, true

	case "esc":
		m.Deactivate()
		return nil, true
	}

	return nil, false
}

// AddItems appends extra items (e.g. from Lua plugins) to the palette.
func (m *Model) AddItems(items []Item) {
	m.items = append(m.items, items...)
	m.filter()
}

// UpdateQuery updates the filter query.
func (m *Model) UpdateQuery(query string) {
	m.query = query
	m.filter()
	if m.selected >= len(m.filtered) {
		m.selected = 0
	}
}

type scoredItem struct {
	item  Item
	score int
}

func (m *Model) filter() {
	if m.query == "" {
		m.filtered = m.items
		return
	}

	q := strings.ToLower(m.query)
	var scored []scoredItem
	for _, item := range m.items {
		name := strings.ToLower(item.Name)
		desc := strings.ToLower(item.Description)

		// Prefix match on name is best
		if strings.HasPrefix(name, q) {
			scored = append(scored, scoredItem{item, 100 - len(name)})
		} else if strings.Contains(name, q) {
			scored = append(scored, scoredItem{item, 50 - len(name)})
		} else if s, ok := fuzzyScore(name, q); ok {
			scored = append(scored, scoredItem{item, s})
		} else if strings.Contains(desc, q) {
			scored = append(scored, scoredItem{item, -50})
		} else if _, ok := fuzzyScore(desc, q); ok {
			scored = append(scored, scoredItem{item, -100})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	m.filtered = nil
	for _, s := range scored {
		m.filtered = append(m.filtered, s.item)
	}
}

// fuzzyScore returns a score and whether the pattern matches.
// Higher scores mean better matches (more consecutive chars, earlier positions).
func fuzzyScore(str, pattern string) (int, bool) {
	pi := 0
	score := 0
	consecutive := 0
	for si := 0; si < len(str) && pi < len(pattern); si++ {
		if str[si] == pattern[pi] {
			pi++
			consecutive++
			score += consecutive * 2 // reward consecutive matches
			if si == pi-1 {
				score += 3 // reward matching at start
			}
		} else {
			consecutive = 0
		}
	}
	if pi < len(pattern) {
		return 0, false
	}
	return score, true
}

// View renders the palette.
func (m Model) View() string {
	if !m.active || len(m.filtered) == 0 {
		return ""
	}

	nameW := 18

	var lines []string
	visible := m.filtered
	if len(visible) > m.maxItems {
		visible = visible[:m.maxItems]
	}

	for i, item := range visible {
		name := "/" + item.Name
		desc := item.Description

		var line string
		if i == m.selected {
			rowStyle := paletteRowSelectedBase.Copy().Width(m.width - 2)
			prefix := styles.PalettePrefix.Render("\u203A ")
			n := styles.PaletteItemSelected.Width(nameW).Render(name)
			d := styles.PaletteDescSelected.Render(desc)
			line = rowStyle.Render(prefix + n + d)
		} else {
			n := styles.PaletteItem.Width(nameW + 2).Render("  " + name)
			descW := m.width - 2 - nameW - 2
			if descW < 0 {
				descW = 0
			}
			d := styles.PaletteDesc.Width(descW).Render(desc)
			line = n + d
		}
		lines = append(lines, line)
	}

	content := strings.Join(lines, "\n")

	return paletteWrapStyle.Render(content)
}
