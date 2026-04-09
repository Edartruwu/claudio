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

	active  bool
	width   int
	height  int
	cursor  int
	tab     Tab
	entries []*memory.Entry
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
	case "j", "down":
		if p.cursor < len(p.entries)-1 {
			p.cursor++
		}
		return nil, true
	case "k", "up":
		if p.cursor > 0 {
			p.cursor--
		}
		return nil, true
	case "G":
		p.cursor = max(0, len(p.entries)-1)
		return nil, true
	case "g":
		p.cursor = 0
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
	case "d":
		if p.tab == TabMemories && p.cursor < len(p.entries) {
			entry := p.entries[p.cursor]
			p.memoryStore.Remove(entry.Name)
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

	var b strings.Builder

	// Tabs with type counts
	typeCounts := p.countTypes()
	tabMem := fmt.Sprintf("  Memories (%d)  ", len(p.entries))
	tabRules := "  Rules  "
	activeTab := lipgloss.NewStyle().Foreground(styles.Text).Bold(true).Underline(true)
	inactiveTab := lipgloss.NewStyle().Foreground(styles.Dim)

	if p.tab == TabMemories {
		b.WriteString(activeTab.Render(tabMem))
		b.WriteString(inactiveTab.Render(tabRules))
	} else {
		b.WriteString(inactiveTab.Render(tabMem))
		b.WriteString(activeTab.Render(tabRules))
	}
	b.WriteString("\n")

	// Type summary bar
	if p.tab == TabMemories && len(typeCounts) > 0 {
		var badges []string
		if n, ok := typeCounts["user"]; ok {
			badges = append(badges, styles.PanelBadgeUser.Render(fmt.Sprintf(" user:%d ", n)))
		}
		if n, ok := typeCounts["feedback"]; ok {
			badges = append(badges, lipgloss.NewStyle().Foreground(styles.Surface).Background(styles.Warning).Render(fmt.Sprintf(" feedback:%d ", n)))
		}
		if n, ok := typeCounts["project"]; ok {
			badges = append(badges, styles.PanelBadgeProject.Render(fmt.Sprintf(" project:%d ", n)))
		}
		if n, ok := typeCounts["reference"]; ok {
			badges = append(badges, styles.PanelBadge.Render(fmt.Sprintf(" ref:%d ", n)))
		}
		b.WriteString("  " + strings.Join(badges, " "))
		b.WriteString("\n")
	}

	b.WriteString(styles.SeparatorLine(p.width))
	b.WriteString("\n")

	if len(p.entries) == 0 {
		b.WriteString(styles.PanelHint.Render("  No entries found"))
		b.WriteString("\n")
	}

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

	for i := startIdx; i < endIdx; i++ {
		e := p.entries[i]
		selected := i == p.cursor

		prefix := "  "
		if selected {
			prefix = styles.ViewportCursor.Render("▸ ")
		}

		// Type badge
		var badge string
		switch e.Type {
		case "user":
			badge = styles.PanelBadgeUser.Render(" user ")
		case "feedback":
			badge = lipgloss.NewStyle().Foreground(styles.Surface).Background(styles.Warning).Render(" feedback ")
		case "project":
			badge = styles.PanelBadgeProject.Render(" project ")
		case "reference":
			badge = styles.PanelBadge.Render(" ref ")
		}

		// Scope indicator
		scopeStr := ""
		if e.Scope != "" {
			scopeStyle := lipgloss.NewStyle().Foreground(styles.Subtle)
			scopeStr = scopeStyle.Render(" [" + e.Scope + "]")
		}

		nameStyle := styles.PanelItem
		if selected {
			nameStyle = styles.PanelItemSelected
		}

		name := nameStyle.Render(e.Name)
		desc := lipgloss.NewStyle().Foreground(styles.Dim).Render(e.Description)

		b.WriteString(prefix + name + " " + badge + scopeStr)
		b.WriteString("\n")
		b.WriteString("    " + desc)
		b.WriteString("\n")
	}

	// Preview selected entry
	if p.cursor < len(p.entries) {
		content := p.entries[p.cursor].Content
		maxPreview := 400
		if p.height < 30 {
			maxPreview = 200
		}
		if len(content) > maxPreview {
			content = content[:maxPreview] + "..."
		}
		b.WriteString("\n")
		b.WriteString(styles.SeparatorLine(p.width))
		b.WriteString("\n")
		preview := lipgloss.NewStyle().Foreground(styles.Muted)
		b.WriteString(preview.Render(content))
	}

	b.WriteString("\n")
	hint := "  j/k navigate · tab switch · esc close"
	if p.tab == TabMemories {
		hint = "  j/k · d delete · e edit · a add · r refresh · esc close"
	}
	b.WriteString(styles.PanelHint.Render(hint))

	return lipgloss.NewStyle().
		Width(p.width).
		Height(p.height).
		Render(b.String())
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
	if p.tab == TabMemories {
		return "j/k navigate · tab switch · d delete · e edit · a add · r refresh · esc close"
	}
	return "j/k navigate · tab switch · esc close"
}
