// Package memorypanel implements the memory and rules browser side panel.
package memorypanel

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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
	focusRight  bool // whether right detail pane is focused
	scrollOffset int // scroll position in right pane
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
	p.focusRight = false
	p.scrollOffset = 0
	p.refresh()
}

func (p *Panel) Deactivate() { p.active = false }

func (p *Panel) SetSize(w, h int) {
	p.width = w
	p.height = h
}

func (p *Panel) refresh() {
	p.entries = p.memoryStore.LoadAll()
	if p.cursor >= len(p.entries) {
		p.cursor = max(0, len(p.entries)-1)
	}
}

func (p *Panel) Update(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch msg.String() {
	case "l", "enter":
		if !p.focusRight {
			p.focusRight = true
		} else if msg.String() == "enter" {
			// enter in right pane: no-op (could open editor in future)
		}
		return nil, true
	case "h":
		p.focusRight = false
		return nil, true
	case "j", "down":
		if p.focusRight {
			if len(p.entries) > 0 && p.cursor < len(p.entries) {
				maxScroll := len(p.entries[p.cursor].Facts) - 1
				if maxScroll < 0 {
					maxScroll = 0
				}
				if p.scrollOffset < maxScroll {
					p.scrollOffset++
				}
			}
		} else {
			if p.cursor < len(p.entries)-1 {
				p.cursor++
				p.scrollOffset = 0
			}
		}
		return nil, true
	case "k", "up":
		if p.focusRight {
			if p.scrollOffset > 0 {
				p.scrollOffset--
			}
		} else {
			if p.cursor > 0 {
				p.cursor--
				p.scrollOffset = 0
			}
		}
		return nil, true
	case "G":
		if !p.focusRight {
			p.cursor = max(0, len(p.entries)-1)
			p.scrollOffset = 0
		}
		return nil, true
	case "g":
		if !p.focusRight {
			p.cursor = 0
			p.scrollOffset = 0
		}
		return nil, true
	case "1":
		p.tab = TabMemories
		p.cursor = 0
		p.scrollOffset = 0
		p.refresh()
		return nil, true
	case "2":
		p.tab = TabRules
		p.cursor = 0
		p.scrollOffset = 0
		p.refresh()
		return nil, true
	case "tab":
		if p.tab == TabMemories {
			p.tab = TabRules
		} else {
			p.tab = TabMemories
		}
		p.cursor = 0
		p.scrollOffset = 0
		p.refresh()
		return nil, true
	case "d":
		if p.tab == TabMemories && p.cursor < len(p.entries) {
			entry := p.entries[p.cursor]
			p.memoryStore.Remove(entry.Name)
			p.scrollOffset = 0
			p.refresh()
		}
		return nil, true
	case "e":
		// Edit selected memory in external editor
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
		// Create a new memory — open a temp file in editor
		if p.tab == TabMemories {
			store := p.memoryStore.ProjectStore()
			if store == nil {
				store = p.memoryStore.GlobalStore()
			}
			if store == nil {
				return nil, true
			}
			// Create a template file
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

	leftW := p.width * 30 / 100
	if leftW < 24 {
		leftW = 24
	}
	rightW := p.width - leftW - 1
	if rightW < 20 {
		rightW = 20
	}

	// ── Left pane (list) ────────────────────────────────
	var lb strings.Builder

	// Tabs
	tab1Style := styles.PanelHint
	tab2Style := styles.PanelHint
	if p.tab == TabMemories {
		tab1Style = styles.PanelTitle
	}
	if p.tab == TabRules {
		tab2Style = styles.PanelTitle
	}
	lb.WriteString(tab1Style.Render(" 1 Memories ") + " " + tab2Style.Render(" 2 Rules "))
	lb.WriteString("\n")
	lb.WriteString(styles.SeparatorLine(leftW - 4))
	lb.WriteString("\n")

	listH := p.height - 8
	if listH < 3 {
		listH = 3
	}

	startIdx := 0
	if p.cursor >= listH {
		startIdx = p.cursor - listH + 1
	}
	endIdx := startIdx + listH
	if endIdx > len(p.entries) {
		endIdx = len(p.entries)
	}

	if len(p.entries) == 0 {
		lb.WriteString(styles.PanelHint.Render("  No entries"))
		lb.WriteString("\n")
	}

	for i := startIdx; i < endIdx; i++ {
		e := p.entries[i]
		prefix := "  "
		if i == p.cursor {
			prefix = "▸ "
		}
		name := e.Name
		maxName := leftW - 10
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
		if i == p.cursor && !p.focusRight {
			lb.WriteString(styles.PanelItemSelected.Render(line))
		} else if i == p.cursor {
			lb.WriteString(styles.PanelItem.Render("▸ " + name + scopeTag))
		} else {
			lb.WriteString(styles.PanelItem.Render(line))
		}
		lb.WriteString("\n")
	}

	leftBorderColor := styles.Muted
	if !p.focusRight {
		leftBorderColor = styles.Primary
	}

	leftBox := lipgloss.NewStyle().
		Width(leftW - 2).Height(p.height - 4).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(leftBorderColor).
		Padding(0, 1).
		Render(lb.String())

	// ── Right pane (detail) ──────────────────────────────
	var rb strings.Builder
	if len(p.entries) > 0 && p.cursor < len(p.entries) {
		e := p.entries[p.cursor]
		rb.WriteString(styles.PanelTitle.Render(e.Name))
		rb.WriteString("\n\n")
		rb.WriteString(styles.PanelHint.Render("Scope: " + e.Scope))
		if e.Type != "" {
			rb.WriteString("  " + styles.PanelHint.Render("Type: "+e.Type))
		}
		rb.WriteString("\n")
		if len(e.Tags) > 0 {
			rb.WriteString(styles.PanelHint.Render("Tags: " + strings.Join(e.Tags, ", ")))
			rb.WriteString("\n")
		}
		if e.Description != "" {
			rb.WriteString(styles.PanelHint.Render("Desc: " + e.Description))
			rb.WriteString("\n")
		}
		rb.WriteString("\n")
		if len(e.Facts) > 0 {
			rb.WriteString(styles.SeparatorLine(rightW - 6))
			rb.WriteString("\n")
			start := p.scrollOffset
			if start >= len(e.Facts) {
				start = 0
			}
			for fi := start; fi < len(e.Facts); fi++ {
				rb.WriteString(fmt.Sprintf("  %d. %s\n", fi+1, e.Facts[fi]))
			}
		}
	} else {
		rb.WriteString(styles.PanelHint.Render("  No entries"))
	}

	rightBorderColor := styles.Muted
	if p.focusRight {
		rightBorderColor = styles.Primary
	}

	rightBox := lipgloss.NewStyle().
		Width(rightW - 2).Height(p.height - 4).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(rightBorderColor).
		Padding(0, 1).
		Render(rb.String())

	// ── Combine ──────────────────────────────────────────
	main := lipgloss.JoinHorizontal(lipgloss.Top, leftBox, rightBox)

	// Footer hints
	var hint string
	if p.focusRight {
		hint = "j/k scroll  h back  esc close"
	} else {
		hint = "j/k navigate  l/enter detail  1/2 tabs  a add  d delete  e edit  r refresh  esc close"
	}
	footer := styles.PanelHint.Render("  " + hint)

	return lipgloss.JoinVertical(lipgloss.Left, main, footer)
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

// Help returns a short keybinding hint line for the panel footer.
func (p *Panel) Help() string {
	if p.focusRight {
		return "j/k scroll · h back · esc close"
	}
	if p.tab == TabMemories {
		return "j/k navigate · l/enter detail · 1/2 tabs · d delete · e edit · a add · r refresh · esc close"
	}
	return "j/k navigate · l/enter detail · 1/2 tabs · esc close"
}
