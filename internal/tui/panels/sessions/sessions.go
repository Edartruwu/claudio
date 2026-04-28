// Package sessions implements a Telescope-style session picker overlay.
package sessions

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/session"
	"github.com/Abraxas-365/claudio/internal/storage"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// ── Package-level style vars ─────────────────────────────────────────────────

var (
	sessDimStyle             = lipgloss.NewStyle().Foreground(styles.Dim)
	sessTextStyle            = lipgloss.NewStyle().Foreground(styles.Text)
	sessSelectedTitleStyle   = lipgloss.NewStyle().Foreground(styles.Text).Bold(true)
	sessSubtleStyle          = lipgloss.NewStyle().Foreground(styles.Subtle)
	sessWarningStyle         = lipgloss.NewStyle().Foreground(styles.Warning)
	sessSelectedIndicatorStyle = lipgloss.NewStyle().Foreground(styles.Warning).Bold(true)
	sessSuccessStyle         = lipgloss.NewStyle().Foreground(styles.Success)
	sessCurrentBadgeStyle    = lipgloss.NewStyle().Foreground(styles.Success).Bold(true)
	sessHeaderStyle          = lipgloss.NewStyle().Foreground(styles.Aqua).Bold(true)
	sessMutedStyle           = lipgloss.NewStyle().Foreground(styles.Muted)
	sessHintStyle            = lipgloss.NewStyle().Foreground(styles.Subtle).Italic(true)
	sessWarnStyle            = lipgloss.NewStyle().Foreground(styles.Error).Bold(true)
	sessRenamePromptStyle    = lipgloss.NewStyle().Foreground(styles.Aqua).Bold(true)
	sessBadgeBaseStyle       = lipgloss.NewStyle().Foreground(styles.Surface)
	sessBoxBaseStyle         = lipgloss.NewStyle().
					Border(lipgloss.RoundedBorder()).
					Padding(0, 1)
	sessPaneBaseStyle = lipgloss.NewStyle()
)

// ResumeSessionMsg is emitted when the user selects a session to resume.
type ResumeSessionMsg struct {
	SessionID string
}

// DeleteSessionMsg is emitted when the user deletes a session.
type DeleteSessionMsg struct {
	SessionID string
}

// mode tracks the picker's input mode.
type mode int

const (
	modeSearch mode = iota
	modeConfirmDelete
	modeRename
)

// Panel is the Telescope-style session picker overlay.
type Panel struct {
	session *session.Session
	db      *storage.DB // optional; enables tree mode

	active   bool
	width    int
	height   int
	cursor   int
	sessions []storage.Session
	filtered []storage.Session
	query    string
	mode     mode
	scopeAll bool // false = this project, true = all projects

	// Rename state
	renameText string

	// Tree mode
	treeMode   bool
	nodes      []treeNode
	treeCursor int
	expanded   map[string]bool
}

// New creates a new session picker.
func New(sess *session.Session) *Panel {
	return &Panel{session: sess}
}

// NewWithDB creates a new session picker with database access (enables tree mode).
func NewWithDB(sess *session.Session, db *storage.DB) *Panel {
	return &Panel{session: sess, db: db, expanded: make(map[string]bool)}
}

func (p *Panel) IsActive() bool { return p.active }

func (p *Panel) Activate() {
	p.active = true
	p.cursor = 0
	p.query = ""
	p.mode = modeSearch
	p.refresh()
}

func (p *Panel) Deactivate() { p.active = false }

func (p *Panel) SetSize(w, h int) {
	p.width = w
	p.height = h
}

func (p *Panel) refresh() {
	if p.scopeAll {
		p.sessions, _ = p.session.Search("", 100)
	} else {
		p.sessions, _ = p.session.RecentForProject(50)
	}
	p.applyFilter()
}

func (p *Panel) applyFilter() {
	if p.query == "" {
		p.filtered = p.sessions
	} else {
		p.filtered = nil
		q := strings.ToLower(p.query)
		for _, s := range p.sessions {
			searchable := strings.ToLower(sessionLabel(s) + " " + s.ProjectDir + " " + s.Model)
			if strings.Contains(searchable, q) {
				p.filtered = append(p.filtered, s)
			}
		}
	}
	if p.cursor >= len(p.filtered) {
		p.cursor = max(0, len(p.filtered)-1)
	}
}

func (p *Panel) Update(msg tea.KeyMsg) (tea.Cmd, bool) {
	key := msg.String()

	// ── Tree mode ──
	if p.treeMode {
		return p.updateTree(key)
	}

	// ── Delete confirmation ──
	if p.mode == modeConfirmDelete {
		switch key {
		case "y":
			if p.cursor < len(p.filtered) {
				_ = p.session.Delete(p.filtered[p.cursor].ID)
				p.mode = modeSearch
				p.refresh()
			}
			return nil, true
		default:
			p.mode = modeSearch
			return nil, true
		}
	}

	// ── Rename mode ──
	if p.mode == modeRename {
		switch key {
		case "esc":
			p.mode = modeSearch
			return nil, true
		case "enter":
			if p.cursor < len(p.filtered) {
				_ = p.session.RenameByID(p.filtered[p.cursor].ID, p.renameText)
				p.mode = modeSearch
				p.refresh()
			}
			return nil, true
		case "backspace":
			if len(p.renameText) > 0 {
				p.renameText = p.renameText[:len(p.renameText)-1]
			}
			return nil, true
		default:
			if len(key) == 1 && key[0] >= 32 {
				p.renameText += key
			}
			return nil, true
		}
	}

	// ── Normal search mode ──
	switch key {
	case "esc", "ctrl+c":
		p.active = false
		return nil, true
	case "up", "ctrl+p", "ctrl+k":
		if p.cursor > 0 {
			p.cursor--
		}
		return nil, true
	case "down", "ctrl+n", "ctrl+j":
		if p.cursor < len(p.filtered)-1 {
			p.cursor++
		}
		return nil, true
	case "enter":
		if p.cursor < len(p.filtered) {
			id := p.filtered[p.cursor].ID
			return func() tea.Msg { return ResumeSessionMsg{SessionID: id} }, true
		}
		return nil, true
	case "ctrl+d":
		if p.cursor < len(p.filtered) {
			cur := p.session.Current()
			if cur == nil || p.filtered[p.cursor].ID != cur.ID {
				p.mode = modeConfirmDelete
			}
		}
		return nil, true
	case "ctrl+a":
		p.scopeAll = !p.scopeAll
		p.cursor = 0
		p.refresh()
		return nil, true
	case "ctrl+r":
		if p.cursor < len(p.filtered) {
			p.mode = modeRename
			p.renameText = sessionLabel(p.filtered[p.cursor])
		}
		return nil, true
	case "t":
		// Toggle tree mode (only if db is available)
		if p.db != nil {
			p.treeMode = true
			p.treeCursor = 0
			if p.expanded == nil {
				p.expanded = make(map[string]bool)
			}
			p.loadTreeNodes()
		}
		return nil, true
	case "backspace":
		if len(p.query) > 0 {
			p.query = p.query[:len(p.query)-1]
			p.applyFilter()
		}
		return nil, true
	default:
		if len(key) == 1 && key[0] >= 32 {
			p.query += key
			p.applyFilter()
			return nil, true
		}
	}

	return nil, false
}

// ── View ────────────────────────────────────────────────

func (p *Panel) View() string {
	if !p.active {
		return ""
	}

	// Guard against uninitialised sizes.
	if p.width < 10 || p.height < 5 {
		return ""
	}

	// Tree mode uses a different layout.
	if p.treeMode {
		return p.viewTree()
	}
	// Border adds +2 cols and +2 rows to the total. Padding(0,1) is INSIDE Width.
	// So Width(boxW) total = boxW+2. Use boxW = p.width-2 so total = p.width.
	// innerW = boxW - 2 (padding only; border is outside Width).
	boxW := p.width - 2
	boxH := p.height

	innerW := boxW - 2 // padding (1+1) is inside Width; content area = boxW - 2
	leftW := innerW * 50 / 100
	rightW := innerW - leftW - 3

	// body(listH) + "\n"(1) + bottomBar(1) = listH+2 content rows.
	// Border adds 2 rows → total = listH+4. Want total = boxH → listH = boxH-4.
	listH := boxH - 4

	// ── Left pane: results ──
	leftContent := p.renderResults(leftW, listH)

	// ── Right pane: preview ──
	rightContent := p.renderPreview(rightW, listH)

	// Force consistent heights
	leftStyled := sessPaneBaseStyle.Width(leftW).Height(listH).Render(leftContent)
	rightStyled := sessPaneBaseStyle.Width(rightW).Height(listH).Render(rightContent)
	sep := verticalSep(listH)

	body := lipgloss.JoinHorizontal(lipgloss.Top, leftStyled, sep, rightStyled)

	// ── Bottom bar ──
	bottomBar := p.renderBottomBar(innerW)

	content := body + "\n" + bottomBar

	// ── Box border ──
	borderColor := styles.SurfaceAlt
	box := sessBoxBaseStyle.
		BorderForeground(borderColor).
		Width(boxW).
		Render(content)

	// Inject titles into border
	box = injectBorderTitle(box, " Results ", " Preview ", leftW+4)

	return box
}

func (p *Panel) renderResults(w, h int) string {
	var lines []string

	currentID := ""
	if cur := p.session.Current(); cur != nil {
		currentID = cur.ID
	}

	cwd, _ := os.Getwd()

	if len(p.filtered) == 0 {
		return sessDimStyle.Render("  No sessions found")
	}

	// Scrolling window
	startIdx := 0
	if p.cursor >= h {
		startIdx = p.cursor - h + 1
	}
	endIdx := min(startIdx+h, len(p.filtered))

	for i := startIdx; i < endIdx; i++ {
		s := p.filtered[i]
		selected := i == p.cursor
		isCurrent := s.ID == currentID
		isLocal := s.ProjectDir == cwd

		// Indicator column
		indicator := "  "
		if selected {
			indicator = sessSelectedIndicatorStyle.Render("> ")
		} else if isCurrent {
			indicator = sessSuccessStyle.Render("● ")
		}

		// Title
		title := sessionLabel(s)
		maxTitle := w - 22
		if maxTitle < 10 {
			maxTitle = 10
		}
		if len(title) > maxTitle {
			title = title[:maxTitle-1] + "…"
		}

		titleStyle := sessDimStyle
		if selected {
			titleStyle = sessSelectedTitleStyle
		}

		// Dir indicator (only if different from cwd)
		dirTag := ""
		if !isLocal {
			dirName := filepath.Base(s.ProjectDir)
			dirTag = sessSubtleStyle.Render(" [" + dirName + "]")
		}

		// Date
		date := s.UpdatedAt.Format("01/02 15:04")

		left := indicator + titleStyle.Render(title) + dirTag
		right := sessSubtleStyle.Render(date)
		gap := w - lipgloss.Width(left) - lipgloss.Width(right)
		if gap < 1 {
			gap = 1
		}

		lines = append(lines, left+strings.Repeat(" ", gap)+right)
	}

	return strings.Join(lines, "\n")
}

func (p *Panel) renderPreview(w, h int) string {
	if p.cursor >= len(p.filtered) {
		return sessDimStyle.Render("  No selection")
	}

	s := p.filtered[p.cursor]

	var lines []string

	// Title
	title := sessionLabel(s)
	lines = append(lines, sessHeaderStyle.Render(" "+title))
	lines = append(lines, "")

	// Metadata
	lines = append(lines, sessDimStyle.Render("  ID      ")+sessTextStyle.Render(s.ID[:8]))
	lines = append(lines, sessDimStyle.Render("  Model   ")+modelBadge(s.Model))
	lines = append(lines, sessDimStyle.Render("  Dir     ")+sessTextStyle.Render(shortenDir(s.ProjectDir, w-12)))
	lines = append(lines, sessDimStyle.Render("  Date    ")+sessTextStyle.Render(s.UpdatedAt.Format("2006-01-02 15:04:05")))

	// Current badge
	if cur := p.session.Current(); cur != nil && cur.ID == s.ID {
		lines = append(lines, "")
		lines = append(lines, sessCurrentBadgeStyle.Render("  ● Current session"))
	}

	// Summary
	lines = append(lines, "")
	if s.Summary != "" {
		lines = append(lines, sessDimStyle.Render("  Summary"))
		for _, sl := range wrapText(s.Summary, w-4) {
			if len(lines) >= h-3 {
				lines = append(lines, sessSubtleStyle.Render("  ..."))
				break
			}
			lines = append(lines, "  "+sessMutedStyle.Render(sl))
		}
	}

	// Fill to bottom, add hints
	for len(lines) < h-2 {
		lines = append(lines, "")
	}
	lines = append(lines, sessHintStyle.Render("  enter open · ctrl+r rename"))
	lines = append(lines, sessHintStyle.Render("  ctrl+d delete · esc close"))

	return strings.Join(lines, "\n")
}

func (p *Panel) renderBottomBar(w int) string {
	switch p.mode {
	case modeConfirmDelete:
		if p.cursor < len(p.filtered) {
			name := sessionLabel(p.filtered[p.cursor])
			return sessWarnStyle.Render("  Delete ") + sessDimStyle.Render("\""+name+"\"") + sessWarnStyle.Render("? ") +
				sessDimStyle.Render("(y to confirm, any key to cancel)")
		}
	case modeRename:
		prompt := sessRenamePromptStyle.Render("  Rename: ")
		text := sessTextStyle.Render(p.renameText)
		cursor := sessWarningStyle.Render("│")
		return prompt + text + cursor
	}

	// Search mode
	icon := sessSelectedIndicatorStyle.Render("> ")
	text := sessTextStyle.Render(p.query)
	cursor := sessWarningStyle.Render("│")

	left := icon + text + cursor

	scope := "project"
	if p.scopeAll {
		scope = "all"
	}
	count := sessDimStyle.
		Render(fmt.Sprintf("%d / %d [%s]", len(p.filtered), len(p.sessions), scope))

	gap := w - lipgloss.Width(left) - lipgloss.Width(count)
	if gap < 1 {
		gap = 1
	}

	return left + strings.Repeat(" ", gap) + count
}

// ── Helpers ─────────────────────────────────────────────

func sessionLabel(s storage.Session) string {
	if s.Title != "" {
		return s.Title
	}
	// Fallback: use project dir basename + short ID
	base := filepath.Base(s.ProjectDir)
	short := s.ID
	if len(short) > 8 {
		short = short[:8]
	}
	return base + "/" + short
}

func modelBadge(model string) string {
	var label string
	var color lipgloss.Color
	switch {
	case strings.Contains(model, "opus"):
		label = " opus "
		color = styles.Primary
	case strings.Contains(model, "haiku"):
		label = " haiku "
		color = styles.Aqua
	default:
		label = " sonnet "
		color = styles.Warning
	}
	return sessBadgeBaseStyle.Background(color).Render(label)
}

func shortenDir(p string, maxW int) string {
	home, _ := os.UserHomeDir()
	if home != "" && strings.HasPrefix(p, home) {
		p = "~" + p[len(home):]
	}
	if len(p) > maxW {
		parts := strings.Split(p, string(filepath.Separator))
		if len(parts) > 3 {
			p = filepath.Join(parts[0], "…", parts[len(parts)-2], parts[len(parts)-1])
		}
		if len(p) > maxW {
			p = p[:maxW-1] + "…"
		}
	}
	return p
}

func verticalSep(height int) string {
	lines := make([]string, height)
	for i := range lines {
		lines[i] = sessSubtleStyle.Render(" │ ")
	}
	return strings.Join(lines, "\n")
}

func injectBorderTitle(box, leftTitle, rightTitle string, splitAt int) string {
	lines := strings.Split(box, "\n")
	if len(lines) < 2 {
		return box
	}

	// Top border: inject titles
	top := []rune(lines[0])
	lt := []rune(sessDimStyle.Render(leftTitle))
	rt := []rune(sessDimStyle.Render(rightTitle))

	// Left title near start
	pos := 3
	if pos+len(lt) < len(top) {
		copy(top[pos:], lt)
	}

	// Right title near split point
	rPos := splitAt
	if rPos+len(rt) < len(top) {
		copy(top[rPos:], rt)
	}

	lines[0] = string(top)
	return strings.Join(lines, "\n")
}

func wrapText(text string, maxW int) []string {
	if maxW <= 0 {
		maxW = 40
	}
	words := strings.Fields(text)
	var lines []string
	current := ""
	for _, word := range words {
		if current == "" {
			current = word
		} else if len(current)+1+len(word) <= maxW {
			current += " " + word
		} else {
			lines = append(lines, current)
			current = word
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

// Help returns a short keybinding hint line for the panel footer.
func (p *Panel) Help() string {
	if p.treeMode {
		return "j/k navigate · enter open · tab expand · b branch · t list · esc close"
	}
	if p.mode == modeRename {
		return "enter confirm · esc cancel"
	}
	if p.mode == modeConfirmDelete {
		return "y confirm delete · any cancel"
	}
	return "↑/↓ navigate · enter open · ctrl+r rename · ctrl+d delete · ctrl+a all · t tree · esc close"
}
