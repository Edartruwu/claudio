package commandpalette

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// Item represents a command in the palette.
type Item struct {
	Name        string
	Description string
}

// SelectMsg is sent when the user selects a command.
type SelectMsg struct {
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
		maxItems: 8,
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

	case "tab", "enter":
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

// UpdateQuery updates the filter query.
func (m *Model) UpdateQuery(query string) {
	m.query = query
	m.filter()
	if m.selected >= len(m.filtered) {
		m.selected = 0
	}
}

func (m *Model) filter() {
	if m.query == "" {
		m.filtered = m.items
		return
	}

	q := strings.ToLower(m.query)
	m.filtered = nil
	for _, item := range m.items {
		if fuzzyMatch(strings.ToLower(item.Name), q) ||
			fuzzyMatch(strings.ToLower(item.Description), q) {
			m.filtered = append(m.filtered, item)
		}
	}
}

func fuzzyMatch(str, pattern string) bool {
	pi := 0
	for si := 0; si < len(str) && pi < len(pattern); si++ {
		if str[si] == pattern[pi] {
			pi++
		}
	}
	return pi == len(pattern)
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

		maxDesc := m.width - nameW - 6
		if maxDesc < 10 {
			maxDesc = 10
		}
		if len(desc) > maxDesc {
			desc = desc[:maxDesc-1] + "\u2026"
		}

		var line string
		if i == m.selected {
			prefix := styles.PalettePrefix.Render("\u203A ")
			n := styles.PaletteItemSelected.Width(nameW).Render(name)
			d := styles.PaletteDescSelected.Render(desc)
			line = prefix + n + d
		} else {
			n := styles.PaletteItem.Width(nameW + 2).Render("  " + name)
			d := styles.PaletteDesc.Render(desc)
			line = n + d
		}
		lines = append(lines, line)
	}

	content := strings.Join(lines, "\n")

	return lipgloss.NewStyle().
		Padding(0, 2).
		Render(content)
}
