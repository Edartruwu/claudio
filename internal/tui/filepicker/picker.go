package filepicker

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// Item represents a file or directory entry.
type Item struct {
	Path  string // display path (as the user would type it)
	IsDir bool
}

// SelectMsg is sent when the user selects a file.
type SelectMsg struct {
	Path  string // the selected path to insert
	IsDir bool   // true when the selection is a directory
}

// BrowseDirMsg is sent when the user selects a directory to browse into it.
type BrowseDirMsg struct {
	Query string // the new query (directory path with trailing /)
}

// Model is the file picker palette.
type Model struct {
	filtered []Item
	selected int
	query    string
	active   bool
	width    int
	maxItems int
	baseDir  string // project root / cwd
}

// New creates a new file picker rooted at the given directory.
func New(baseDir string) Model {
	return Model{
		baseDir:  baseDir,
		maxItems: 8,
	}
}

// SetWidth updates the display width.
func (m *Model) SetWidth(w int) {
	m.width = w
}

// IsActive returns whether the picker is visible.
func (m Model) IsActive() bool {
	return m.active
}

// Activate shows the picker and scans for files matching the query.
func (m *Model) Activate(query string) {
	m.active = true
	m.query = query
	m.scan()
	m.selected = 0
}

// Deactivate hides the picker.
func (m *Model) Deactivate() {
	m.active = false
	m.query = ""
	m.selected = 0
	m.filtered = nil
}

// Update handles key events when the picker is active.
// Returns true if the key was consumed.
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
			item := m.filtered[m.selected]
			if item.IsDir {
				// Navigate into the directory instead of selecting it
				newQuery := item.Path + "/"
				m.query = newQuery
				m.scan()
				m.selected = 0
				return func() tea.Msg {
					return BrowseDirMsg{Query: newQuery}
				}, true
			}
			return func() tea.Msg {
				return SelectMsg{Path: item.Path, IsDir: false}
			}, true
		}
		return nil, false

	case "esc":
		m.Deactivate()
		return nil, true
	}

	return nil, false
}

// UpdateQuery re-scans with a new query.
func (m *Model) UpdateQuery(query string) {
	m.query = query
	m.scan()
	if m.selected >= len(m.filtered) {
		m.selected = 0
	}
}

var skipDirs = map[string]bool{
	"node_modules": true, "vendor": true, "__pycache__": true,
	"dist": true, "build": true, ".git": true,
}

// scan finds files/dirs matching the query.
// Supports relative paths like ../, ../../, ../internal/cli, etc.
// Also supports ~ home-directory expansion like ~/Documents/foo.
func (m *Model) scan() {
	m.filtered = nil

	// Split query into a directory prefix and a fuzzy filter part.
	// e.g. "../../internal/cl" → scanDir="../../internal", fuzzy="cl"
	// e.g. "../"              → scanDir="..",              fuzzy=""
	// e.g. "cl"               → scanDir=".",               fuzzy="cl"
	// e.g. ""                 → scanDir=".",               fuzzy=""
	// e.g. "~/Documents/foo"  → scanDir="~/Documents",     fuzzy="foo"  (expanded)

	var scanAbs string
	var fuzzy string
	var displayPrefix string

	if m.query == "~" || strings.HasPrefix(m.query, "~/") {
		// Expand ~ to home directory
		home, err := os.UserHomeDir()
		if err != nil {
			return
		}
		// Replace ~ with absolute home path for path computations
		expanded := home + strings.TrimPrefix(m.query, "~")
		scanRel, fz := splitQueryPath(expanded)
		fuzzy = fz
		scanAbs = filepath.Clean(scanRel)
		// Build display prefix: keep ~/... notation for readability
		expandedPrefix := strings.TrimPrefix(scanRel, home)
		if scanRel == home {
			displayPrefix = "~/"
		} else {
			displayPrefix = "~" + expandedPrefix + string(filepath.Separator)
		}
	} else {
		scanRel, fz := splitQueryPath(m.query)
		fuzzy = fz
		// Resolve the scan directory relative to baseDir
		scanAbs = filepath.Clean(filepath.Join(m.baseDir, scanRel))
		// The display prefix: what we prepend to results so they look like
		// what the user typed. e.g. if they typed "../", results show "../foo".
		if scanRel != "." {
			displayPrefix = filepath.Clean(scanRel) + string(filepath.Separator)
		} else {
			displayPrefix = ""
		}
	}

	info, err := os.Stat(scanAbs)
	if err != nil || !info.IsDir() {
		return
	}

	// If user explicitly typed a dot (e.g. "~/.config"), allow hidden files.
	showHidden := strings.HasPrefix(fuzzy, ".")

	// Use ReadDir to list only direct children — no deep walk needed since we
	// always show/fuzzy-match at depth 0 only. A deep WalkDir would hit the
	// entry cap inside large directories (e.g. ~/Applications) before reaching
	// sibling dirs the user actually wants (e.g. ~/Personal).
	entries, err := os.ReadDir(scanAbs)
	if err != nil {
		return
	}

	fq := strings.ToLower(fuzzy)
	for _, entry := range entries {
		name := entry.Name()

		// Skip hidden unless user explicitly typed a leading dot.
		if strings.HasPrefix(name, ".") && !showHidden {
			continue
		}
		if entry.IsDir() && skipDirs[name] {
			continue
		}

		// Fuzzy filter (empty fuzzy means show all)
		if fq != "" && !fuzzyMatch(strings.ToLower(name), fq) {
			continue
		}

		m.filtered = append(m.filtered, Item{
			Path:  displayPrefix + name,
			IsDir: entry.IsDir(),
		})
	}

	// Sort: directories first, then alphabetically
	sort.Slice(m.filtered, func(i, j int) bool {
		if m.filtered[i].IsDir != m.filtered[j].IsDir {
			return m.filtered[i].IsDir
		}
		return m.filtered[i].Path < m.filtered[j].Path
	})
}

// splitQueryPath splits a query like "../../internal/cl" into
// a directory to scan ("../../internal") and a fuzzy filter ("cl").
func splitQueryPath(query string) (dir, fuzzy string) {
	if query == "" {
		return ".", ""
	}

	// If it ends with /, the whole thing is a directory
	if strings.HasSuffix(query, "/") {
		return filepath.Clean(query), ""
	}

	// Split at the last separator
	lastSep := strings.LastIndexByte(query, '/')
	if lastSep < 0 {
		// No separator: fuzzy match in cwd
		return ".", query
	}

	return filepath.Clean(query[:lastSep+1]), query[lastSep+1:]
}

// fuzzyMatch checks if all characters in pattern appear in order in str.
func fuzzyMatch(str, pattern string) bool {
	pi := 0
	for si := 0; si < len(str) && pi < len(pattern); si++ {
		if str[si] == pattern[pi] {
			pi++
		}
	}
	return pi == len(pattern)
}

// View renders the file picker.
func (m Model) View() string {
	if !m.active || len(m.filtered) == 0 {
		return ""
	}

	var lines []string
	visible := m.filtered
	if len(visible) > m.maxItems {
		visible = visible[:m.maxItems]
	}

	for i, item := range visible {
		display := item.Path
		if item.IsDir {
			display += "/"
		}

		var line string
		if i == m.selected {
			prefix := styles.PalettePrefix.Render("\u203A ")
			line = prefix + styles.PaletteItemSelected.Render(display)
		} else {
			line = styles.PickerAdd.Render("+ ") + styles.PaletteItem.Render(display)
		}
		lines = append(lines, line)
	}

	content := strings.Join(lines, "\n")

	return lipgloss.NewStyle().
		Padding(0, 2).
		Render(content)
}
