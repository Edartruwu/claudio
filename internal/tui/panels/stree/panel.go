// Package stree implements the STREE session explorer side panel.
// It renders sessions as expandable folder nodes (nvim-tree style), with children
// for the conversation, memory entries, and files referenced in each session.
package stree

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/services/memory"
	"github.com/Abraxas-365/claudio/internal/storage"
	panelsessions "github.com/Abraxas-365/claudio/internal/tui/panels/sessions"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// ── Node types ─────────────────────────────────────────────────────────────

// NodeKind identifies the type of a tree node.
type NodeKind int

const (
	NodeSession      NodeKind = iota // folder node — a session
	NodeConversation                 // "conversation" child — switch to session
	NodeMemory                       // memory entry child — open in $EDITOR
	NodeFile                         // referenced file child — open in $EDITOR
)

// Node is a single row in the session tree.
type Node struct {
	Kind     NodeKind
	Label    string // display text
	Data     string // session ID, memory name, or file path
	Depth    int    // 0 = session folder, 1 = child
	Expanded bool   // only relevant for NodeSession
}

// ── Messages ────────────────────────────────────────────────────────────────

// editorDoneMsg is returned when the external editor process exits.
type editorDoneMsg struct{ err error }

// DeleteSessionMsg is emitted when the user confirms deletion of a session.
// Root handles this by switching away and removing the session.
type DeleteSessionMsg struct{ ID string }

// ── Panel ───────────────────────────────────────────────────────────────────

// Panel is the STREE session explorer side panel.
type Panel struct {
	db           *storage.DB
	memorySvc    *memory.ScopedStore
	getCurrentID func() string // returns the currently active session ID

	// Tree state
	sessions []storage.Session // all top-level sessions
	nodes    []Node            // flat visible node list (rebuilt on load/expand)
	cursor   int

	// Layout
	width  int
	height int
	active bool

	// Interaction state
	confirmDelete string // non-empty: waiting for second 'd' to confirm deletion
	statusMsg     string // one-line status/hint shown at bottom
}

// New creates a new STREE panel.
// db is used to list sessions and fetch message history.
// mem is optional; if non-nil, memory entries are shown under expanded sessions.
// getCurrentID returns the ID of the currently active session (called at render time).
func New(db *storage.DB, mem *memory.ScopedStore, getCurrentID func() string) *Panel {
	return &Panel{
		db:           db,
		memorySvc:    mem,
		getCurrentID: getCurrentID,
	}
}

// ── Panel interface ─────────────────────────────────────────────────────────

func (p *Panel) IsActive() bool { return p.active }

func (p *Panel) Activate() {
	p.active = true
	p.cursor = 0
	p.confirmDelete = ""
	p.statusMsg = ""
	p.load()
}

func (p *Panel) Deactivate() {
	p.active = false
	p.confirmDelete = ""
	p.statusMsg = ""
}

func (p *Panel) SetSize(w, h int) {
	p.width = w
	p.height = h
}

func (p *Panel) Update(msg tea.KeyMsg) (tea.Cmd, bool) {
	key := msg.String()

	// If in confirm-delete mode, only accept 'd' (confirm) or 'esc'/'any' (cancel).
	if p.confirmDelete != "" {
		switch key {
		case "d":
			id := p.confirmDelete
			p.confirmDelete = ""
			p.statusMsg = ""
			return p.doDelete(id), true
		default:
			p.confirmDelete = ""
			p.statusMsg = ""
			return nil, true
		}
	}

	switch key {
	case "j", "down":
		if p.cursor < len(p.nodes)-1 {
			p.cursor++
		}
		return nil, true

	case "k", "up":
		if p.cursor > 0 {
			p.cursor--
		}
		return nil, true

	case "G":
		p.cursor = max(0, len(p.nodes)-1)
		return nil, true

	case "g":
		p.cursor = 0
		return nil, true

	case "enter", "o":
		return p.handleActivate(), true

	case "d":
		return p.handleDelete(), true

	case "r":
		return p.handleRename(), true

	case "R":
		// Reload sessions, preserve expanded state.
		p.load()
		p.statusMsg = "Refreshed"
		return nil, true

	case "g y":
		// Yank node label to clipboard.
		return p.yankLabel(), true

	default:
		return nil, false
	}
}

func (p *Panel) View() string {
	if !p.active {
		return ""
	}

	var b strings.Builder

	// ── Header ──
	title := styles.PanelTitle.Render("STREE")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(styles.SeparatorLine(p.width))
	b.WriteString("\n")

	// ── Node list ──
	listH := p.height - 6 // reserve header (2) + separator (1) + footer (3)
	if listH < 2 {
		listH = 2
	}

	if len(p.nodes) == 0 {
		b.WriteString(styles.PanelHint.Render("  No sessions found"))
		b.WriteString("\n")
	} else {
		startIdx := 0
		if p.cursor >= listH {
			startIdx = p.cursor - listH + 1
		}
		endIdx := startIdx + listH
		if endIdx > len(p.nodes) {
			endIdx = len(p.nodes)
		}

		currentID := ""
		if p.getCurrentID != nil {
			currentID = p.getCurrentID()
		}

		for i := startIdx; i < endIdx; i++ {
			node := p.nodes[i]
			selected := i == p.cursor
			b.WriteString(p.renderNode(node, selected, currentID))
			b.WriteString("\n")
		}
	}

	// ── Footer ──
	b.WriteString(styles.SeparatorLine(p.width))
	b.WriteString("\n")

	var hint string
	if p.statusMsg != "" {
		hint = p.statusMsg
	} else {
		hint = "j/k nav · enter open · o expand · R reload · esc close"
	}
	b.WriteString(styles.PanelHint.Render(hint))

	return lipgloss.NewStyle().
		Width(p.width).
		Height(p.height).
		Render(b.String())
}

func (p *Panel) Help() string {
	return "j/k navigate · enter/o open · d delete · r rename · R refresh · gy yank · esc close"
}

// ── Rendering ──────────────────────────────────────────────────────────────

func (p *Panel) renderNode(node Node, selected bool, currentID string) string {
	var icon, label string
	indent := strings.Repeat("  ", node.Depth)

	switch node.Kind {
	case NodeSession:
		arrow := "▸"
		if node.Expanded {
			arrow = "▾"
		}
		folderIcon := "📁"
		label = indent + arrow + " " + folderIcon + " " + node.Label
		// Active session marker
		if node.Data == currentID {
			marker := lipgloss.NewStyle().Foreground(styles.Success).Render(" [●]")
			label = label + marker
		}

	case NodeConversation:
		icon = "~"
		label = indent + "├ " + lipgloss.NewStyle().Foreground(styles.Aqua).Render(icon) + " " + node.Label

	case NodeMemory:
		icon = "*"
		label = indent + "├ " + lipgloss.NewStyle().Foreground(styles.Warning).Render(icon) + " " + node.Label

	case NodeFile:
		icon = "-"
		label = indent + "└ " + lipgloss.NewStyle().Foreground(styles.Muted).Render(icon) + " " + node.Label
	}

	if selected {
		labelStyle := lipgloss.NewStyle().Foreground(styles.Text).Bold(true)
		cursorStr := styles.ViewportCursor.Render("▸")
		// Replace leading spaces with cursor indicator for the selected row.
		stripped := strings.TrimLeft(label, " ")
		spacesCount := len(label) - len(stripped)
		prefix := ""
		if spacesCount > 0 {
			prefix = strings.Repeat(" ", spacesCount-1) + cursorStr
		} else {
			prefix = cursorStr
		}
		return labelStyle.Render(prefix + " " + stripped)
	}

	return lipgloss.NewStyle().Foreground(styles.Dim).Render(label)
}

// ── Data loading ───────────────────────────────────────────────────────────

// load reloads the session list from the database and rebuilds the node tree.
func (p *Panel) load() {
	if p.db == nil {
		return
	}

	sessions, err := p.db.ListSessions(100)
	if err != nil {
		p.statusMsg = fmt.Sprintf("Error: %v", err)
		return
	}
	p.sessions = sessions

	// Preserve expanded state by session ID.
	expanded := make(map[string]bool)
	for _, n := range p.nodes {
		if n.Kind == NodeSession && n.Expanded {
			expanded[n.Data] = true
		}
	}

	p.rebuildNodes(expanded)

	// Clamp cursor.
	if p.cursor >= len(p.nodes) {
		p.cursor = max(0, len(p.nodes)-1)
	}
}

// rebuildNodes constructs the flat visible node list from p.sessions.
// expanded is a set of session IDs that should be shown with their children.
func (p *Panel) rebuildNodes(expanded map[string]bool) {
	p.nodes = p.nodes[:0]

	for _, s := range p.sessions {
		isExpanded := expanded[s.ID]
		title := s.Title
		if title == "" {
			title = s.ID[:min(8, len(s.ID))] + "…"
		}

		sessionNode := Node{
			Kind:     NodeSession,
			Label:    title,
			Data:     s.ID,
			Depth:    0,
			Expanded: isExpanded,
		}
		p.nodes = append(p.nodes, sessionNode)

		if isExpanded {
			children := p.loadChildren(s.ID)
			p.nodes = append(p.nodes, children...)
		}
	}
}

// loadChildren fetches and returns child nodes for the given session.
// Children are: conversation, memory entries, referenced files.
func (p *Panel) loadChildren(sessionID string) []Node {
	var children []Node

	// 1. Conversation node — always present.
	children = append(children, Node{
		Kind:  NodeConversation,
		Label: "conversation",
		Data:  sessionID,
		Depth: 1,
	})

	// 2. Memory entries (project-level, same for all sessions).
	if p.memorySvc != nil {
		entries := p.memorySvc.LoadAll()
		for _, e := range entries {
			name := e.Name
			if name == "" {
				name = e.Description
			}
			children = append(children, Node{
				Kind:  NodeMemory,
				Label: name,
				Data:  e.FilePath, // use FilePath for editor opening; fall back to Name
				Depth: 1,
			})
		}
	}

	// 3. Files referenced in this session's message history.
	if p.db != nil {
		messages, err := p.db.GetMessages(sessionID)
		if err == nil {
			filePaths := extractFilePaths(messages)
			for _, path := range filePaths {
				children = append(children, Node{
					Kind:  NodeFile,
					Label: path,
					Data:  path,
					Depth: 1,
				})
			}
		}
	}

	return children
}

// extractFilePaths scans tool_use messages for file path arguments.
// Supports tools: Read, Write, Edit (field: file_path), Glob (field: pattern, only absolute paths).
func extractFilePaths(messages []storage.MessageRecord) []string {
	seen := make(map[string]bool)
	var paths []string

	for _, msg := range messages {
		if msg.Type != "tool_use" {
			continue
		}

		var input map[string]json.RawMessage
		if err := json.Unmarshal([]byte(msg.Content), &input); err != nil {
			continue
		}

		switch msg.ToolName {
		case "Read", "Write", "Edit":
			if raw, ok := input["file_path"]; ok {
				var path string
				if err := json.Unmarshal(raw, &path); err == nil && path != "" && !seen[path] {
					seen[path] = true
					paths = append(paths, path)
				}
			}
		case "Glob":
			if raw, ok := input["pattern"]; ok {
				var pattern string
				if err := json.Unmarshal(raw, &pattern); err == nil && pattern != "" && !seen[pattern] {
					// Only include if it looks like an absolute file path (no glob wildcards).
					if filepath.IsAbs(pattern) && !strings.ContainsAny(pattern, "*?[") {
						seen[pattern] = true
						paths = append(paths, pattern)
					}
				}
			}
		}
	}

	return paths
}

// ── Key handlers ───────────────────────────────────────────────────────────

// handleActivate acts on the currently selected node.
func (p *Panel) handleActivate() tea.Cmd {
	if len(p.nodes) == 0 {
		return nil
	}
	node := p.nodes[p.cursor]

	switch node.Kind {
	case NodeSession:
		// Toggle expand/collapse.
		p.toggleExpand(p.cursor)
		return nil

	case NodeConversation:
		// Switch to that session.
		return func() tea.Msg {
			return panelsessions.ResumeSessionMsg{SessionID: node.Data}
		}

	case NodeMemory:
		// Open memory file in editor (read-only).
		if node.Data != "" {
			return p.openInEditor(node.Data, true)
		}
		// No file path: show status.
		p.statusMsg = "Memory has no file path"
		return nil

	case NodeFile:
		// Open file in editor.
		if node.Data != "" {
			return p.openInEditor(node.Data, false)
		}
		return nil
	}
	return nil
}

// toggleExpand expands or collapses the session node at the given index.
func (p *Panel) toggleExpand(idx int) {
	if idx < 0 || idx >= len(p.nodes) {
		return
	}
	node := &p.nodes[idx]
	if node.Kind != NodeSession {
		return
	}

	node.Expanded = !node.Expanded

	// Rebuild the expanded map from current state.
	expanded := make(map[string]bool)
	for _, n := range p.nodes {
		if n.Kind == NodeSession && n.Expanded {
			expanded[n.Data] = true
		}
	}
	// The node we just toggled.
	expanded[node.Data] = node.Expanded

	// Remember cursor session.
	cursorSessionID := node.Data

	p.rebuildNodes(expanded)

	// Restore cursor to the same session node.
	for i, n := range p.nodes {
		if n.Kind == NodeSession && n.Data == cursorSessionID {
			p.cursor = i
			break
		}
	}
}

// handleDelete initiates or confirms deletion of a session.
func (p *Panel) handleDelete() tea.Cmd {
	if len(p.nodes) == 0 {
		return nil
	}
	node := p.nodes[p.cursor]
	if node.Kind != NodeSession {
		return nil
	}

	// First press: show confirmation prompt.
	title := node.Label
	p.confirmDelete = node.Data
	p.statusMsg = fmt.Sprintf("Delete '%s'? Press d again to confirm", title)
	return nil
}

// doDelete deletes the session with the given ID from the database.
func (p *Panel) doDelete(sessionID string) tea.Cmd {
	if p.db == nil {
		p.statusMsg = "Error: no database"
		return nil
	}

	// Find title for status message.
	title := sessionID
	for _, s := range p.sessions {
		if s.ID == sessionID {
			title = s.Title
			if title == "" {
				title = s.ID[:min(8, len(s.ID))] + "…"
			}
			break
		}
	}

	if err := p.db.DeleteSession(sessionID); err != nil {
		p.statusMsg = fmt.Sprintf("Delete failed: %v", err)
		return nil
	}

	p.statusMsg = fmt.Sprintf("Deleted '%s'", title)
	p.load() // refresh list
	return func() tea.Msg {
		return DeleteSessionMsg{ID: sessionID}
	}
}

// handleRename shows a hint since inline rename requires a separate input model.
func (p *Panel) handleRename() tea.Cmd {
	if len(p.nodes) == 0 {
		return nil
	}
	node := p.nodes[p.cursor]
	if node.Kind != NodeSession {
		return nil
	}
	p.statusMsg = "Use <Space>br to rename the active session"
	return nil
}

// yankLabel copies the current node's label to the clipboard.
func (p *Panel) yankLabel() tea.Cmd {
	if len(p.nodes) == 0 {
		return nil
	}
	label := p.nodes[p.cursor].Label

	// Try macOS pbcopy, then Linux xclip/xsel.
	var cmd *exec.Cmd
	if _, err := exec.LookPath("pbcopy"); err == nil {
		cmd = exec.Command("pbcopy")
	} else if _, err := exec.LookPath("xclip"); err == nil {
		cmd = exec.Command("xclip", "-selection", "clipboard")
	} else if _, err := exec.LookPath("xsel"); err == nil {
		cmd = exec.Command("xsel", "--clipboard", "--input")
	}

	if cmd != nil {
		cmd.Stdin = strings.NewReader(label)
		if err := cmd.Run(); err == nil {
			p.statusMsg = fmt.Sprintf("Yanked: %s", label)
		} else {
			p.statusMsg = "Yank failed"
		}
	} else {
		p.statusMsg = "No clipboard tool found (pbcopy/xclip/xsel)"
	}
	return nil
}

// ── Editor ─────────────────────────────────────────────────────────────────

// openInEditor opens the given file path in $EDITOR via tea.ExecProcess.
func (p *Panel) openInEditor(path string, readOnly bool) tea.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		editor = "nvim"
	}

	var args []string
	if readOnly {
		args = []string{"-R", path}
	} else {
		args = []string{path}
	}

	cmd := exec.Command(editor, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return editorDoneMsg{err: err}
	})
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
