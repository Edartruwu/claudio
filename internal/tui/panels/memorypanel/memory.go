// Package memorypanel implements the memory and rules browser side panel.
package memorypanel

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/services/memory"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// Tab identifies which tab is active.
type Tab int

const (
	TabMemories Tab = iota
	TabRules
)

// Panel is the memory/rules browser side panel.
type Panel struct {
	memoryStore *memory.ScopedStore

	active      bool
	width       int
	height      int
	cursor      int
	tab         Tab
	entries     []*memory.Entry
	focusedPane int // 0 = left list, 1 = right detail
	detailVP    viewport.Model
}

// New creates a new memory/rules browser panel.
func New(mem *memory.ScopedStore) *Panel {
	return &Panel{memoryStore: mem}
}

func (p *Panel) IsActive() bool { return p.active }

func (p *Panel) Activate() {
	p.active = true
	p.cursor = 0
	p.tab = TabMemories
	p.focusedPane = 0
	p.refresh()
}

func (p *Panel) Deactivate() { p.active = false }

func (p *Panel) SetSize(w, h int) {
	p.width = w
	p.height = h

	leftW := p.leftPaneWidth()
	rightW := w - leftW - 4 // leftContent + leftBorder(2) + rightBorder(2)
	if rightW < 20 {
		rightW = 20
	}
	p.detailVP.Width = rightW - 2 // leave padding
	p.detailVP.Height = p.contentHeight()
}

// contentHeight is the scrollable area height: inner pane minus header rows.
func (p *Panel) contentHeight() int {
	// innerH = height - 2 (border). Header = title(1) + blank(1) + meta(1..3) + sep(1) ≈ 5 rows.
	h := p.height - 2 - 5
	if h < 3 {
		h = 3
	}
	return h
}

func (p *Panel) refresh() {
	p.entries = p.memoryStore.LoadAll()
	if p.cursor >= len(p.entries) {
		p.cursor = max(0, len(p.entries)-1)
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
	switch key {
	case "l", "enter":
		p.focusedPane = 1
		p.detailVP.GotoTop()
		return nil, true
	case "tab":
		if p.tab == TabMemories {
			p.tab = TabRules
		} else {
			p.tab = TabMemories
		}
		p.cursor = 0
		p.refresh()
		return nil, true
	case "1":
		p.tab = TabMemories
		p.cursor = 0
		p.refresh()
		return nil, true
	case "2":
		p.tab = TabRules
		p.cursor = 0
		p.refresh()
		return nil, true
	case "j", "down":
		if p.cursor < len(p.entries)-1 {
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
		p.cursor = max(0, len(p.entries)-1)
		p.refreshDetail()
		return nil, true
	case "g":
		p.cursor = 0
		p.refreshDetail()
		return nil, true
	case "d":
		if p.tab == TabMemories && p.cursor < len(p.entries) {
			entry := p.entries[p.cursor]
			p.memoryStore.Remove(entry.Name)
			p.refresh()
		}
		return nil, true
	case "e":
		if p.tab == TabMemories && p.cursor < len(p.entries) {
			entry := p.entries[p.cursor]
			if entry.FilePath != "" {
				editor := os.Getenv("EDITOR")
				if editor == "" {
					editor = "vim"
				}
				cmd := exec.Command(editor, entry.FilePath)
				cmd.Stdin = os.Stdin
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				return tea.ExecProcess(cmd, func(err error) tea.Msg {
					return EditorDoneMsg{Err: err}
				}), true
			}
		}
		return nil, true
	case "a":
		if p.tab == TabMemories {
			store := p.memoryStore.ProjectStore()
			if store == nil {
				store = p.memoryStore.GlobalStore()
			}
			if store == nil {
				return nil, true
			}
			template := "---\nname: new-memory\ndescription: describe this memory\ntype: project\n---\n\nWrite your memory content here.\n"
			tmpFile, err := os.CreateTemp("", "claudio-memory-*.md")
			if err != nil {
				return nil, true
			}
			tmpFile.WriteString(template)
			tmpPath := tmpFile.Name()
			tmpFile.Close()
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "vim"
			}
			cmd := exec.Command(editor, tmpPath)
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			return tea.ExecProcess(cmd, func(err error) tea.Msg {
				return NewMemoryMsg{TmpPath: tmpPath, Err: err}
			}), true
		}
		return nil, true
	case "r":
		p.refresh()
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

// EditorDoneMsg is sent when the external editor closes after editing a memory.
type EditorDoneMsg struct{ Err error }

// NewMemoryMsg is sent when a new memory editor closes.
type NewMemoryMsg struct {
	TmpPath string
	Err     error
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

	tab1Style := styles.PanelHint
	tab2Style := styles.PanelHint
	if p.tab == TabMemories {
		tab1Style = styles.PanelTitle
	}
	if p.tab == TabRules {
		tab2Style = styles.PanelTitle
	}
	lb.WriteString(" " + tab1Style.Render(" 1 Memories ") + " " + tab2Style.Render(" 2 Rules "))
	lb.WriteString("\n")
	lb.WriteString(styles.SeparatorLine(innerW))
	lb.WriteString("\n")

	listH := p.height - 4
	if listH < 3 {
		listH = 3
	}

	if len(p.entries) == 0 {
		lb.WriteString("  " + styles.PanelHint.Render("No entries"))
		lb.WriteString("\n")
	}

	startIdx := 0
	if p.cursor >= listH {
		startIdx = p.cursor - listH + 1
	}
	endIdx := startIdx + listH
	if endIdx > len(p.entries) {
		endIdx = len(p.entries)
	}

	for i := startIdx; i < endIdx; i++ {
		e := p.entries[i]
		prefix := "  "
		if i == p.cursor {
			prefix = "▸ "
		}
		name := e.Name
		maxName := innerW - 6
		if maxName < 4 {
			maxName = 4
		}
		if len(name) > maxName {
			name = name[:maxName-1] + "…"
		}
		scopeTag := " [g]"
		if e.Scope == "project" {
			scopeTag = " [p]"
		}
		line := prefix + name + scopeTag
		if i == p.cursor && p.focusedPane == 0 {
			lb.WriteString(styles.PanelItemSelected.Render(line))
		} else if i == p.cursor {
			lb.WriteString(styles.PanelItem.Render("▸ " + name + scopeTag))
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

	if len(p.entries) == 0 || p.cursor >= len(p.entries) {
		rb.WriteString("  " + styles.PanelHint.Render("No entries"))
		rb.WriteString("\n")
		return rb.String()
	}

	e := p.entries[p.cursor]
	rb.WriteString(" " + styles.PanelTitle.Render(e.Name))
	rb.WriteString("\n\n")
	rb.WriteString(" " + styles.PanelHint.Render("Scope: "+e.Scope))
	if e.Type != "" {
		rb.WriteString("  " + styles.PanelHint.Render("Type: "+e.Type))
	}
	rb.WriteString("\n")
	if len(e.Tags) > 0 {
		rb.WriteString(" " + styles.PanelHint.Render("Tags: "+strings.Join(e.Tags, ", ")))
		rb.WriteString("\n")
	}
	if e.Description != "" {
		rb.WriteString(" " + styles.PanelHint.Render("Desc: "+e.Description))
		rb.WriteString("\n")
	}
	rb.WriteString(styles.SeparatorLine(innerW))
	rb.WriteString("\n")

	// Viewport handles the scrollable facts content
	rb.WriteString(p.detailVP.View())

	return rb.String()
}

func (p *Panel) buildDetailContent() string {
	if len(p.entries) == 0 || p.cursor >= len(p.entries) {
		return ""
	}
	e := p.entries[p.cursor]
	if len(e.Facts) == 0 {
		return styles.PanelHint.Render("  No facts recorded.")
	}

	var md strings.Builder
	for fi, fact := range e.Facts {
		md.WriteString(fmt.Sprintf("%d. %s\n", fi+1, fact))
	}

	mdWidth := p.detailVP.Width - 2
	if mdWidth < 10 {
		mdWidth = 10
	}
	rendered := md.String()
	if r, err := glamour.NewTermRenderer(
		glamour.WithStylesFromJSONBytes(styles.GruvboxGlamourJSON()),
		glamour.WithWordWrap(mdWidth),
	); err == nil {
		if out, err2 := r.Render(rendered); err2 == nil {
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
	if p.tab == TabMemories {
		return "j/k navigate · l/enter detail · tab/1/2 tabs · d delete · e edit · a add · r refresh · esc close"
	}
	return "j/k navigate · l/enter detail · tab/1/2 tabs · esc close"
}

// countTypes returns a map of memory type -> count.
func (p *Panel) countTypes() map[string]int {
	counts := make(map[string]int)
	for _, e := range p.entries {
		if e.Type != "" {
			counts[e.Type]++
		}
	}
	return counts
}
