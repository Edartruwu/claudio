// Package skillspanel implements the skills browser side panel.
package skillspanel

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
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

	active      bool
	width       int
	height      int
	cursor      int
	items       []*skills.Skill
	query       string
	searching   bool
	focusedPane int // 0 = left list, 1 = right detail
	detailVP    viewport.Model
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
	p.focusedPane = 0
	p.refresh()
}

func (p *Panel) Deactivate() { p.active = false }

func (p *Panel) SetSize(w, h int) {
	p.width = w
	p.height = h

	leftW := p.leftPaneWidth()
	rightW := w - leftW - 4
	if rightW < 20 {
		rightW = 20
	}
	p.detailVP.Width = rightW - 2
	p.detailVP.Height = p.contentHeight()
}

func (p *Panel) contentHeight() int {
	// innerH = height-2 (border). Header = title(1) + blank(1) + desc(1) + sep(1) = 4 rows.
	h := p.height - 2 - 4
	if h < 3 {
		h = 3
	}
	return h
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
	p.refreshDetail()
}

func (p *Panel) refreshDetail() {
	p.detailVP.SetContent(p.buildDetailContent())
}

func (p *Panel) Update(msg tea.KeyMsg) (tea.Cmd, bool) {
	if p.focusedPane == 1 {
		return p.updateRight(msg.String())
	}
	return p.updateLeft(msg.String())
}

func (p *Panel) updateLeft(key string) (tea.Cmd, bool) {
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
	case "l", "enter":
		if key == "l" {
			p.focusedPane = 1
			p.detailVP.GotoTop()
			return nil, true
		}
		// enter = invoke skill
		if p.cursor < len(p.items) {
			s := p.items[p.cursor]
			return func() tea.Msg {
				return InvokeSkillMsg{Name: s.Name, Content: s.Content}
			}, true
		}
		return nil, true
	case "j", "down":
		if p.cursor < len(p.items)-1 {
			p.cursor++
			p.refreshDetail()
		}
		return nil, true
	case "k", "up":
		if p.cursor > 0 {
			p.cursor--
			p.refreshDetail()
		}
		return nil, true
	case "G":
		p.cursor = max(0, len(p.items)-1)
		p.refreshDetail()
		return nil, true
	case "g":
		p.cursor = 0
		p.refreshDetail()
		return nil, true
	case "/":
		p.searching = true
		p.query = ""
		return nil, true
	}

	return nil, false
}

func (p *Panel) updateRight(key string) (tea.Cmd, bool) {
	switch key {
	case "j", "down":
		p.detailVP.ScrollDown(1)
		return nil, true
	case "k", "up":
		p.detailVP.ScrollUp(1)
		return nil, true
	case "ctrl+d":
		p.detailVP.HalfPageDown()
		return nil, true
	case "ctrl+u":
		p.detailVP.HalfPageUp()
		return nil, true
	case "G":
		p.detailVP.GotoBottom()
		return nil, true
	case "g":
		p.detailVP.GotoTop()
		return nil, true
	case "h", "tab", "esc":
		p.focusedPane = 0
		return nil, true
	}
	return nil, false
}

func (p *Panel) View() string {
	if !p.active {
		return ""
	}
	return p.renderSplitView()
}

func (p *Panel) renderSplitView() string {
	innerH := p.height - 2
	if innerH < 2 {
		innerH = 2
	}

	leftW := p.leftPaneWidth()
	rightW := p.width - leftW - 4
	if rightW < 20 {
		rightW = 20
	}

	leftBorderColor := styles.Muted
	if p.focusedPane == 0 {
		leftBorderColor = styles.Primary
	}
	rightBorderColor := styles.Muted
	if p.focusedPane == 1 {
		rightBorderColor = styles.Primary
	}

	leftBorder := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(leftBorderColor)
	rightBorder := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(rightBorderColor)

	leftStyled := leftBorder.Width(leftW).Height(innerH).Render(p.renderLeftPane(leftW))
	rightStyled := rightBorder.Width(rightW).Height(innerH).Render(p.renderRightPane(rightW))

	return lipgloss.JoinHorizontal(lipgloss.Top, leftStyled, rightStyled)
}

func (p *Panel) leftPaneWidth() int {
	w := p.width * 30 / 100
	if w < 24 {
		w = 24
	}
	if w > p.width-40 {
		w = p.width - 40
	}
	return w
}

func (p *Panel) renderLeftPane(w int) string {
	innerW := w - 2
	var lb strings.Builder

	lb.WriteString(" " + styles.PanelTitle.Render("Skills"))
	lb.WriteString("\n")
	lb.WriteString(styles.SeparatorLine(innerW))
	lb.WriteString("\n")

	if p.searching {
		lb.WriteString(" " + styles.PanelSearch.Render("/") + " ")
		lb.WriteString(lipgloss.NewStyle().Foreground(styles.Text).Render(p.query))
		lb.WriteString(lipgloss.NewStyle().Foreground(styles.Warning).Render("▋"))
		lb.WriteString("\n")
	}

	listH := p.height - 4
	if p.searching {
		listH--
	}
	if listH < 3 {
		listH = 3
	}

	if len(p.items) == 0 {
		lb.WriteString("  " + styles.PanelHint.Render("No skills found"))
		lb.WriteString("\n")
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
			prefix = "▸ "
		}

		var badge string
		switch s.Source {
		case "bundled":
			badge = styles.PanelBadgeBundled.Render(" built-in ")
		case "user":
			badge = styles.PanelBadgeUser.Render(" user ")
		case "project":
			badge = styles.PanelBadgeProject.Render(" project ")
		}

		maxName := innerW - 14
		if maxName < 4 {
			maxName = 4
		}
		name := s.Name
		if len(name) > maxName {
			name = name[:maxName-1] + "…"
		}

		line := prefix + name + " " + badge
		if selected && p.focusedPane == 0 {
			lb.WriteString(styles.PanelItemSelected.Render(line))
		} else if selected {
			lb.WriteString(styles.PanelItem.Render("▸ " + name + " " + badge))
		} else {
			lb.WriteString(styles.PanelItem.Render(line))
		}
		lb.WriteString("\n")
	}

	return lb.String()
}

func (p *Panel) renderRightPane(w int) string {
	innerW := w - 2
	var rb strings.Builder

	if p.cursor >= len(p.items) {
		rb.WriteString("  " + styles.PanelHint.Render("No skills"))
		return rb.String()
	}

	s := p.items[p.cursor]
	rb.WriteString(" " + styles.PanelTitle.Render(s.Name))
	if s.Description != "" {
		rb.WriteString("\n\n")
		rb.WriteString(" " + styles.PanelHint.Render(s.Description))
	}
	rb.WriteString("\n\n")
	rb.WriteString(styles.SeparatorLine(innerW))
	rb.WriteString("\n")

	rb.WriteString(p.detailVP.View())

	return rb.String()
}

func (p *Panel) buildDetailContent() string {
	if p.cursor >= len(p.items) {
		return ""
	}
	s := p.items[p.cursor]
	if s.Content == "" {
		return styles.PanelHint.Render("  No content.")
	}

	mdWidth := p.detailVP.Width - 2
	if mdWidth < 10 {
		mdWidth = 10
	}
	rendered := s.Content
	if r, err := glamour.NewTermRenderer(
		glamour.WithStylesFromJSONBytes(styles.GruvboxGlamourJSON()),
		glamour.WithWordWrap(mdWidth),
	); err == nil {
		if out, err2 := r.Render(s.Content); err2 == nil {
			rendered = out
		}
	}
	return rendered
}

// Help returns a short keybinding hint line for the panel footer.
func (p *Panel) Help() string {
	if p.focusedPane == 1 {
		return "j/k scroll · ctrl+d/u half-page · G bottom · g top · tab/h back · esc close"
	}
	return "j/k navigate · l detail · / search · enter invoke · esc close"
}
