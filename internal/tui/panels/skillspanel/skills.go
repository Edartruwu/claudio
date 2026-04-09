// Package skillspanel implements the skills browser side panel.
package skillspanel

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/services/skills"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// InvokeSkillMsg is emitted when the user selects a skill to invoke.
type InvokeSkillMsg struct {
	Name    string
	Content string
}

// Panel is the skills browser side panel.
type Panel struct {
	registry *skills.Registry

	active   bool
	width    int
	height   int
	cursor   int
	items    []*skills.Skill
	query    string
	searching bool
}

// New creates a new skills browser panel.
func New(reg *skills.Registry) *Panel {
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
	all := p.registry.All()
	if p.query == "" {
		p.items = all
	} else {
		p.items = nil
		q := strings.ToLower(p.query)
		for _, s := range all {
			if strings.Contains(strings.ToLower(s.Name), q) ||
				strings.Contains(strings.ToLower(s.Description), q) {
				p.items = append(p.items, s)
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
	case "enter":
		if p.cursor < len(p.items) {
			s := p.items[p.cursor]
			return func() tea.Msg {
				return InvokeSkillMsg{Name: s.Name, Content: s.Content}
			}, true
		}
		return nil, true
	case "/":
		p.searching = true
		p.query = ""
		return nil, true
	}

	return nil, false
}

func (p *Panel) View() string {
	if !p.active {
		return ""
	}

	var b strings.Builder

	title := styles.PanelTitle.Render("Skills")
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
		b.WriteString(styles.PanelHint.Render("  No skills found"))
		b.WriteString("\n")
	}

	// Skill list
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

	for i := startIdx; i < endIdx; i++ {
		s := p.items[i]
		selected := i == p.cursor

		prefix := "  "
		if selected {
			prefix = styles.ViewportCursor.Render("▸ ")
		}

		// Source badge
		var badge string
		switch s.Source {
		case "bundled":
			badge = styles.PanelBadgeBundled.Render(" built-in ")
		case "user":
			badge = styles.PanelBadgeUser.Render(" user ")
		case "project":
			badge = styles.PanelBadgeProject.Render(" project ")
		}

		nameStyle := styles.PanelItem
		if selected {
			nameStyle = styles.PanelItemSelected
		}

		name := nameStyle.Render(s.Name)
		desc := lipgloss.NewStyle().Foreground(styles.Dim).Render(s.Description)

		line1 := prefix + name + " " + badge
		line2 := "    " + desc
		b.WriteString(line1)
		b.WriteString("\n")
		b.WriteString(line2)
		b.WriteString("\n")
	}

	// Preview of selected skill
	if p.cursor < len(p.items) {
		s := p.items[p.cursor]
		content := s.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		b.WriteString("\n")
		b.WriteString(styles.SeparatorLine(p.width))
		b.WriteString("\n")
		previewStyle := lipgloss.NewStyle().Foreground(styles.Muted)
		b.WriteString(previewStyle.Render(content))
	}

	b.WriteString("\n")
	b.WriteString(styles.PanelHint.Render("  enter invoke · / search · esc close"))

	return lipgloss.NewStyle().
		Width(p.width).
		Height(p.height).
		Render(b.String())
}

// Help returns a short keybinding hint line for the panel footer.
func (p *Panel) Help() string {
	return "j/k navigate · / search · enter invoke · esc close"
}
