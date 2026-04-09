// Package filespanel implements a side panel showing files touched during the session.
package filespanel

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// FileStatus tracks how a file was interacted with.
type FileStatus int

const (
	FileRead     FileStatus = iota // read-only access
	FileModified                   // edited or multi-edited
	FileAdded                      // written (created or overwritten)
)

// FileEntry holds info about one file touched in the session.
type FileEntry struct {
	Path   string
	Status FileStatus
	Count  int // number of operations
}

// FileOp represents a single file operation observed in the session.
type FileOp struct {
	Path      string
	Operation string // "read", "write", "edit", "glob", "grep"
}

// OpenFileMsg is emitted when the user presses enter on a row, requesting
// the host TUI to open the selected file in an external editor.
type OpenFileMsg struct {
	Path string
}

// Panel shows files touched by tool calls in the current session.
type Panel struct {
	active  bool
	focused bool
	width   int
	height  int
	cursor  int
	entries []FileEntry
}

// SetFocused marks whether this panel has keyboard focus.
func (p *Panel) SetFocused(f bool) { p.focused = f }

// New creates a new files panel.
func New() *Panel {
	return &Panel{}
}

func (p *Panel) IsActive() bool      { return p.active }
func (p *Panel) Activate()           { p.active = true }
func (p *Panel) Deactivate()         { p.active = false }
func (p *Panel) SetSize(w, h int)    { p.width = w; p.height = h }

// Update handles key events. Implements panels.Panel.
func (p *Panel) Update(msg tea.KeyMsg) (tea.Cmd, bool) {
	if !p.active {
		return nil, false
	}
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
	case "enter":
		if p.cursor >= 0 && p.cursor < len(p.entries) {
			path := p.entries[p.cursor].Path
			return func() tea.Msg { return OpenFileMsg{Path: path} }, true
		}
		return nil, true
	case "q", "esc":
		p.active = false
		return nil, true
	}
	return nil, false
}

// Refresh rebuilds the entries from a list of file operations.
func (p *Panel) Refresh(ops []FileOp) {
	byPath := make(map[string]*FileEntry)
	for _, op := range ops {
		e, ok := byPath[op.Path]
		if !ok {
			e = &FileEntry{Path: op.Path}
			byPath[op.Path] = e
		}
		e.Count++
		switch op.Operation {
		case "write":
			if e.Status < FileAdded {
				e.Status = FileAdded
			}
		case "edit", "multiedit":
			if e.Status < FileModified {
				e.Status = FileModified
			}
		default: // read, grep, glob
			// don't downgrade status
		}
	}

	p.entries = nil
	for _, e := range byPath {
		if e.Status > FileRead {
			p.entries = append(p.entries, *e)
		}
	}
	sort.Slice(p.entries, func(i, j int) bool {
		// Sort by status desc (Added > Modified > Read), then path
		if p.entries[i].Status != p.entries[j].Status {
			return p.entries[i].Status > p.entries[j].Status
		}
		return p.entries[i].Path < p.entries[j].Path
	})
	if p.cursor >= len(p.entries) {
		p.cursor = maxInt(0, len(p.entries)-1)
	}
}

// View renders the panel.
func (p *Panel) View() string {
	if !p.active {
		return ""
	}
	if p.width < 10 || p.height < 3 {
		return ""
	}

	titleStyle := lipgloss.NewStyle().Foreground(styles.Primary).Bold(true)
	addStyle := lipgloss.NewStyle().Foreground(styles.Success)
	modStyle := lipgloss.NewStyle().Foreground(styles.Warning)
	readStyle := lipgloss.NewStyle().Foreground(styles.Text)
	selectedStyle := lipgloss.NewStyle().Background(styles.Primary).Foreground(styles.Surface).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(styles.Muted)

	maxPathW := p.width - 8
	if maxPathW < 10 {
		maxPathW = 10
	}

	header := titleStyle.Render(fmt.Sprintf(" Files (%d) ", len(p.entries)))
	sepW := p.width - 2
	if sepW < 1 {
		sepW = 1
	}
	sep := strings.Repeat("─", sepW)

	var lines []string
	lines = append(lines, header)
	lines = append(lines, dimStyle.Render(sep))

	visible := p.height - 4
	if visible < 1 {
		visible = 1
	}

	start := 0
	if p.cursor >= visible {
		start = p.cursor - visible + 1
	}

	for i := start; i < len(p.entries) && i < start+visible; i++ {
		e := p.entries[i]

		var icon string
		var iconSt lipgloss.Style
		switch e.Status {
		case FileAdded:
			icon = "✚"
			iconSt = addStyle
		case FileModified:
			icon = "✎"
			iconSt = modStyle
		default:
			icon = "○"
			iconSt = readStyle
		}

		path := e.Path
		// Shorten long paths
		if len(path) > maxPathW {
			path = "…" + path[len(path)-maxPathW+1:]
		}

		if i == p.cursor {
			renderedIcon := iconSt.Render(icon)
			row := " " + renderedIcon + " " + path
			if e.Count > 1 {
				row += fmt.Sprintf(" (%d)", e.Count)
			}
			lines = append(lines, selectedStyle.Width(p.width).Render(row))
			continue
		}
		count := ""
		if e.Count > 1 {
			count = dimStyle.Render(fmt.Sprintf(" (%d)", e.Count))
		}
		row := " " + iconSt.Render(icon) + " " + path + count
		lines = append(lines, row)
	}

	if len(p.entries) == 0 {
		lines = append(lines, dimStyle.Render("  No files accessed yet"))
	}

	// Hint line at the bottom
	var hintText string
	if p.focused {
		hintText = lipgloss.NewStyle().Foreground(styles.Warning).Render("j/k nav · esc close")
	} else {
		hintText = dimStyle.Render("<space>f focus · esc close")
	}
	lines = append(lines, " "+hintText)

	return strings.Join(lines, "\n")
}

// ExtractFileOps extracts file operations from raw tool call data.
// toolName is the tool name (e.g. "Read", "Write", "Edit").
// inputJSON is the raw JSON input to the tool.
func ExtractFileOps(toolName string, inputJSON []byte) []FileOp {
	switch strings.ToLower(toolName) {
	case "read":
		var in struct {
			FilePath string `json:"file_path"`
		}
		if json.Unmarshal(inputJSON, &in) == nil && in.FilePath != "" {
			return []FileOp{{Path: in.FilePath, Operation: "read"}}
		}
	case "write":
		var in struct {
			FilePath string `json:"file_path"`
		}
		if json.Unmarshal(inputJSON, &in) == nil && in.FilePath != "" {
			return []FileOp{{Path: in.FilePath, Operation: "write"}}
		}
	case "edit":
		var in struct {
			FilePath string `json:"file_path"`
		}
		if json.Unmarshal(inputJSON, &in) == nil && in.FilePath != "" {
			return []FileOp{{Path: in.FilePath, Operation: "edit"}}
		}
	case "multiedit":
		var in struct {
			Edits []struct {
				FilePath string `json:"file_path"`
			} `json:"edits"`
		}
		if json.Unmarshal(inputJSON, &in) == nil {
			var ops []FileOp
			for _, e := range in.Edits {
				if e.FilePath != "" {
					ops = append(ops, FileOp{Path: e.FilePath, Operation: "edit"})
				}
			}
			return ops
		}
	case "glob":
		var in struct {
			Pattern string `json:"pattern"`
			Path    string `json:"path"`
		}
		if json.Unmarshal(inputJSON, &in) == nil {
			dir := in.Path
			if dir == "" {
				dir = "."
			}
			return []FileOp{{Path: filepath.Join(dir, in.Pattern), Operation: "glob"}}
		}
	case "grep":
		var in struct {
			Path string `json:"path"`
		}
		if json.Unmarshal(inputJSON, &in) == nil && in.Path != "" {
			return []FileOp{{Path: in.Path, Operation: "read"}}
		}
	}
	return nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Help returns a short keybinding hint line for the panel footer.
func (p *Panel) Help() string {
	return "j/k navigate · enter open · q/esc close"
}
