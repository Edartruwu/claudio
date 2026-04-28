package sessions

// Tree mode for the session picker — netrw-style nested branch view.
//
// Keys:
//   j / k        move cursor
//   enter        open selected session → ResumeSessionMsg
//   tab / l      expand/collapse node children
//   b            branch from selected session's last message → BranchFromSessionMsg
//   t            back to flat list mode
//   esc          close picker

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/storage"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// ── Tree-mode style vars ─────────────────────────────────────────────────────

var (
	treeLineStyle     = lipgloss.NewStyle().Foreground(styles.Subtle)
	treeSelectedStyle = lipgloss.NewStyle().Foreground(styles.Text).Bold(true)
	treeDimStyle      = lipgloss.NewStyle().Foreground(styles.Muted)
	treeBranchStyle   = lipgloss.NewStyle().Foreground(styles.Aqua)
	treeCurrentStyle  = lipgloss.NewStyle().Foreground(styles.Success)
	treeHintStyle     = lipgloss.NewStyle().Foreground(styles.Subtle).Italic(true)
	treeMutedStyle    = lipgloss.NewStyle().Foreground(styles.Dim)
)

// treeNode is a flattened tree entry for rendering.
type treeNode struct {
	session     *storage.Session
	depth       int
	hasChildren bool
	isLast      bool   // last child at this depth level (affects connector char)
	parentLines []bool // for each ancestor depth: true = │ needed, false = space
}

// BranchFromSessionMsg is emitted when the user presses 'b' in tree mode.
// Root should call session.Branch using the last message of SessionID.
type BranchFromSessionMsg struct {
	SessionID string
}

// loadTreeNodes rebuilds the flat nodes list from root sessions.
// Root sessions are all sessions whose ParentSessionID is "".
func (p *Panel) loadTreeNodes() {
	if p.db == nil {
		return
	}

	// Fetch all sessions as the pool.
	allSessions, err := p.session.Search("", 200)
	if err != nil {
		return
	}

	// Find root sessions (no parent, not a user-branch session).
	var roots []storage.Session
	for _, s := range allSessions {
		if s.ParentSessionID == "" && s.BranchFromMessageID == nil {
			roots = append(roots, s)
		}
	}

	p.nodes = nil
	for _, root := range roots {
		node, err := p.db.GetSessionTree(root.ID)
		if err != nil {
			// Fall back: add session without children
			p.nodes = append(p.nodes, treeNode{
				session: &root,
				depth:   0,
			})
			continue
		}
		flattenNode(node, 0, nil, &p.nodes, p.expanded)
	}
}

// flattenNode recursively flattens a SessionNode tree into p.nodes.
// parentLines tracks whether each ancestor level needs a │ connector.
func flattenNode(node *storage.SessionNode, depth int, parentLines []bool, out *[]treeNode, expanded map[string]bool) {
	if node == nil {
		return
	}

	hasChildren := len(node.Children) > 0
	tn := treeNode{
		session:     node.Session,
		depth:       depth,
		hasChildren: hasChildren,
		parentLines: append([]bool{}, parentLines...),
	}
	*out = append(*out, tn)

	if hasChildren && expanded[node.Session.ID] {
		childLines := append(append([]bool{}, parentLines...), true)
		for i, child := range node.Children {
			isLast := i == len(node.Children)-1
			if isLast {
				// This branch doesn't need a trailing │ at its level
				childLines[len(childLines)-1] = false
			}
			flattenNode(child, depth+1, childLines, out, expanded)
			// Reset for next sibling
			childLines[len(childLines)-1] = true
		}
	}
}

// updateTree handles key presses when in tree mode.
func (p *Panel) updateTree(key string) (tea.Cmd, bool) {
	switch key {
	case "esc", "ctrl+c":
		p.active = false
		return nil, true

	case "t":
		// Back to flat list
		p.treeMode = false
		return nil, true

	case "j", "down", "ctrl+j", "ctrl+n":
		if p.treeCursor < len(p.nodes)-1 {
			p.treeCursor++
		}
		return nil, true

	case "k", "up", "ctrl+k", "ctrl+p":
		if p.treeCursor > 0 {
			p.treeCursor--
		}
		return nil, true

	case "enter":
		if p.treeCursor < len(p.nodes) {
			id := p.nodes[p.treeCursor].session.ID
			return func() tea.Msg { return ResumeSessionMsg{SessionID: id} }, true
		}
		return nil, true

	case "tab", "l":
		// Expand/collapse children
		if p.treeCursor < len(p.nodes) {
			node := p.nodes[p.treeCursor]
			if node.hasChildren {
				id := node.session.ID
				p.expanded[id] = !p.expanded[id]
				p.loadTreeNodes()
				// Keep cursor in bounds
				if p.treeCursor >= len(p.nodes) {
					p.treeCursor = max(0, len(p.nodes)-1)
				}
			}
		}
		return nil, true

	case "b":
		// Branch from selected session's last message
		if p.treeCursor < len(p.nodes) {
			id := p.nodes[p.treeCursor].session.ID
			return func() tea.Msg { return BranchFromSessionMsg{SessionID: id} }, true
		}
		return nil, true
	}

	return nil, false
}

// viewTree renders the tree mode UI.
func (p *Panel) viewTree() string {
	boxW := p.width - 2
	boxH := p.height
	innerW := boxW - 2
	listH := boxH - 4

	body := p.renderTreeList(innerW, listH)
	styled := sessBoxBaseStyle.Width(innerW).Height(listH).Render(body)

	// Bottom hint
	hint := treeHintStyle.Render("  j/k nav · enter open · tab expand · b branch · t list · esc close")
	if innerW < lipgloss.Width(hint)+4 {
		hint = treeHintStyle.Render(" enter open · tab expand")
	}

	content := styled + "\n" + hint

	borderColor := styles.SurfaceAlt
	box := sessBoxBaseStyle.
		BorderForeground(borderColor).
		Width(boxW).
		Render(content)

	// Inject tree title
	lines := strings.Split(box, "\n")
	if len(lines) > 0 {
		title := treeBranchStyle.Render(" Session Tree ")
		top := []rune(lines[0])
		lt := []rune(title)
		if 3+len(lt) < len(top) {
			copy(top[3:], lt)
		}
		lines[0] = string(top)
		box = strings.Join(lines, "\n")
	}

	return box
}

// renderTreeList renders the flat node list with netrw-style tree connectors.
func (p *Panel) renderTreeList(w, h int) string {
	if len(p.nodes) == 0 {
		return treeDimStyle.Render("  No sessions")
	}

	currentID := ""
	if cur := p.session.Current(); cur != nil {
		currentID = cur.ID
	}

	// Scrolling window
	startIdx := 0
	if p.treeCursor >= h {
		startIdx = p.treeCursor - h + 1
	}
	endIdx := min(startIdx+h, len(p.nodes))

	var lines []string
	for i := startIdx; i < endIdx; i++ {
		tn := p.nodes[i]
		selected := i == p.treeCursor

		line := renderTreeLine(tn, selected, tn.session.ID == currentID, w, p.expanded)
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// renderTreeLine renders a single tree node line.
func renderTreeLine(tn treeNode, selected, isCurrent bool, w int, expanded map[string]bool) string {
	// Build indent prefix
	var prefix strings.Builder
	for _, needLine := range tn.parentLines {
		if needLine {
			prefix.WriteString(treeLineStyle.Render("│  "))
		} else {
			prefix.WriteString("   ")
		}
	}

	// Connector for this node
	if tn.depth > 0 {
		if tn.isLast {
			prefix.WriteString(treeLineStyle.Render("└─ "))
		} else {
			prefix.WriteString(treeLineStyle.Render("├─ "))
		}
	}

	// Expand indicator
	expandMark := " "
	if tn.hasChildren {
		if expanded[tn.session.ID] {
			expandMark = treeLineStyle.Render("▾")
		} else {
			expandMark = treeLineStyle.Render("▸")
		}
	}

	// Title
	title := sessionLabel(*tn.session)
	prefixW := tn.depth*3 + 2 // approx width of tree prefix
	maxTitle := w - prefixW - 4
	if maxTitle < 5 {
		maxTitle = 5
	}
	if len(title) > maxTitle {
		title = title[:maxTitle-1] + "…"
	}

	var titleStr string
	switch {
	case selected && isCurrent:
		titleStr = treeSelectedStyle.Render(expandMark + " " + title) + " " + treeCurrentStyle.Render("●")
	case selected:
		titleStr = treeSelectedStyle.Render(expandMark+" "+title)
	case isCurrent:
		titleStr = treeMutedStyle.Render(expandMark+" ") + treeCurrentStyle.Render(title)
	default:
		titleStr = treeDimStyle.Render(expandMark + " " + title)
	}

	// Selection indicator
	indicator := "  "
	if selected {
		indicator = treeBranchStyle.Render("> ")
	}

	// Branch indicator for branch sessions
	branchMark := ""
	if tn.session.BranchFromMessageID != nil {
		branchMark = treeBranchStyle.Render(" ⎇")
	}

	return indicator + prefix.String() + titleStr + branchMark
}

// max returns the larger of two ints (for Go < 1.21 compat).
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ── Expose loadTreeNodes for test/reload access ─────────────────────────────

// TreeNodeCount returns how many nodes are in the current tree view.
// Used for testing.
func (p *Panel) TreeNodeCount() int { return len(p.nodes) }

// summary for a session (short label with branch indicator)
func treeSessionSummary(s *storage.Session) string {
	label := sessionLabel(*s)
	if s.BranchFromMessageID != nil {
		return fmt.Sprintf("⎇ %s", label)
	}
	return label
}

var _ = treeSessionSummary // suppress unused warning
