package prompt

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
	"github.com/Abraxas-365/claudio/internal/tui/vim"
)

// SubmitMsg is sent when the user presses Enter to submit their input.
type SubmitMsg struct {
	Text   string
	Images []api.UserContentBlock
}

// VimEscapeMsg signals that Escape was consumed by vim (switch to Normal).
// The root model should NOT treat this as a cancel/dismiss.
type VimEscapeMsg struct{}

// Model is the prompt input component.
type Model struct {
	textarea   textarea.Model
	focused    bool
	width      int
	history    []string
	histIdx    int
	showHint   bool
	vimState   *vim.State
	vimEnabled bool
	undoStack  []string // simple undo ring buffer

	// Paste collapsing
	pastedContents map[int]string  // paste ID → full content
	nextPasteID    int             // next paste ID counter
	pasteBuffer    strings.Builder // accumulates text during bracketed paste
	isPasting      bool            // currently inside a bracketed paste

	// Image attachments
	images      []ImageAttachment
	nextImageID int
}

// ImageAttachment represents an image attached to the prompt.
type ImageAttachment struct {
	ID        int
	FileName  string // display name
	MediaType string // MIME type
	Data      string // base64-encoded
}

const (
	maxUndo        = 50
	pasteThreshold = 200 // chars — pastes above this are collapsed
)

var pasteRefRe = regexp.MustCompile(`\[Pasted text #(\d+)(?: \+\d+ lines)?\]`)
var imageRefRe = regexp.MustCompile(`\[Image #\d+: [^\]]+\]`)

// StripImageRefs removes [Image #N: filename] references from text.
func StripImageRefs(s string) string {
	return strings.TrimSpace(imageRefRe.ReplaceAllString(s, ""))
}

// New creates a new prompt input model.
func New() Model {
	ta := textarea.New()
	ta.Placeholder = "Message Claudio..."
	ta.Focus()
	ta.CharLimit = 100000
	ta.MaxHeight = 10
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.Base = lipgloss.NewStyle()
	ta.BlurredStyle.Base = lipgloss.NewStyle()

	return Model{
		textarea:       ta,
		focused:        true,
		histIdx:        -1,
		showHint:       true,
		vimState:       vim.New(),
		vimEnabled:     true,
		pastedContents: make(map[int]string),
	}
}

// SetWidth sets the prompt width.
func (m *Model) SetWidth(w int) {
	m.width = w
	m.textarea.SetWidth(w - 3)
}

// Focus gives focus to the prompt.
func (m *Model) Focus() {
	m.focused = true
	m.textarea.Focus()
}

// Blur removes focus from the prompt.
func (m *Model) Blur() {
	m.focused = false
	m.textarea.Blur()
}

// Reset clears the input.
func (m *Model) Reset() {
	m.textarea.Reset()
	m.histIdx = -1
}

// Value returns the current input text.
func (m *Model) Value() string {
	return m.textarea.Value()
}

// SetValue sets the prompt input text.
func (m *Model) SetValue(s string) {
	m.textarea.SetValue(s)
	m.textarea.CursorEnd()
}

// SetPlaceholder sets the placeholder text shown when the prompt is empty.
func (m *Model) SetPlaceholder(s string) {
	m.textarea.Placeholder = s
}

// Placeholder returns the current placeholder text.
func (m *Model) Placeholder() string {
	return m.textarea.Placeholder
}

// ToggleVim enables/disables vim mode.
func (m *Model) ToggleVim() {
	m.vimEnabled = !m.vimEnabled
	if m.vimEnabled {
		m.vimState = vim.New() // starts in Insert mode
	}
}

// IsVimEnabled returns whether vim mode is active.
func (m *Model) IsVimEnabled() bool {
	return m.vimEnabled
}

// VimModeString returns the current vim mode name, or "" if vim is disabled.
func (m *Model) VimModeString() string {
	if !m.vimEnabled {
		return ""
	}
	return m.vimState.Mode.String()
}

// IsVimNormal returns true if vim is enabled and in Normal mode.
func (m *Model) IsVimNormal() bool {
	return m.vimEnabled && m.vimState.Mode == vim.ModeNormal
}

// EnterVimInsert forces vim mode into Insert (no-op when vim is disabled).
func (m *Model) EnterVimInsert() {
	if m.vimEnabled {
		m.vimState.Mode = vim.ModeInsert
	}
}

// CursorLine returns the current cursor line (0-based) in the textarea.
func (m *Model) CursorLine() int {
	return m.textarea.Line()
}

// LineCount returns the total number of lines in the textarea.
func (m *Model) LineCount() int {
	return m.textarea.LineCount()
}

// Update handles input events.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.focused {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Bracketed paste interception: accumulate paste text
		if msg.Paste {
			if !m.isPasting {
				m.isPasting = true
				m.pasteBuffer.Reset()
			}
			switch msg.Type {
			case tea.KeyRunes:
				for _, r := range msg.Runes {
					m.pasteBuffer.WriteRune(r)
				}
			case tea.KeyEnter:
				m.pasteBuffer.WriteRune('\n')
			case tea.KeySpace:
				m.pasteBuffer.WriteRune(' ')
			case tea.KeyTab:
				m.pasteBuffer.WriteRune('\t')
			}
			// Bubbletea delivers the entire bracketed paste in a single KeyMsg,
			// so finalize immediately instead of waiting for the next keystroke.
			m.finalizePaste()
			return m, nil
		}

		// Finalize any pending paste on first non-paste key (safety fallback)
		if m.isPasting {
			m.finalizePaste()
		}

		// Vim mode: Escape in Insert → Normal (consume, don't propagate)
		if m.vimEnabled && msg.Type == tea.KeyEscape && m.vimState.Mode == vim.ModeInsert {
			m.vimState.Mode = vim.ModeNormal
			return m, func() tea.Msg { return VimEscapeMsg{} }
		}

		// Vim mode: in Normal/Visual/OperatorPending → intercept all keys
		if m.vimEnabled && m.vimState.Mode != vim.ModeInsert {
			return m.handleVimKey(msg)
		}

		// Normal flow (Insert mode or vim disabled)
		switch msg.Type {
		case tea.KeyEnter:
			text := m.ExpandedValue()
			text = strings.TrimSpace(text)
			if text == "" {
				return m, nil
			}
			m.history = append(m.history, m.textarea.Value()) // store collapsed form in history
			m.histIdx = -1
			m.showHint = false
			m.textarea.Reset()
			m.pastedContents = make(map[int]string) // clear pastes on submit
			return m, func() tea.Msg {
				return SubmitMsg{Text: text}
			}

		case tea.KeyUp:
			if m.textarea.Line() == 0 && len(m.history) > 0 {
				if m.histIdx == -1 {
					m.histIdx = len(m.history) - 1
				} else if m.histIdx > 0 {
					m.histIdx--
				}
				m.textarea.SetValue(m.history[m.histIdx])
				m.textarea.CursorEnd()
				return m, nil
			}

		case tea.KeyDown:
			if m.textarea.Line() == m.textarea.LineCount()-1 && m.histIdx >= 0 {
				if m.histIdx < len(m.history)-1 {
					m.histIdx++
					m.textarea.SetValue(m.history[m.histIdx])
				} else {
					m.histIdx = -1
					m.textarea.Reset()
				}
				return m, nil
			}

		case tea.KeyCtrlC:
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	m.syncImagesFromText()
	m.syncPastesFromText()
	return m, cmd
}

// handleVimKey processes keys when vim is in Normal/Visual/OperatorPending mode.
func (m Model) handleVimKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	// Convert tea.KeyMsg to rune
	var key rune
	switch msg.Type {
	case tea.KeyRunes:
		if len(msg.Runes) == 0 {
			return m, nil
		}
		key = msg.Runes[0]
	case tea.KeyEscape:
		key = 27
	case tea.KeyEnter:
		// In Normal mode, Enter submits (like Insert mode Enter)
		text := m.ExpandedValue()
		text = strings.TrimSpace(text)
		if text == "" {
			return m, nil
		}
		m.history = append(m.history, m.textarea.Value())
		m.histIdx = -1
		m.showHint = false
		m.textarea.Reset()
		m.pastedContents = make(map[int]string)
		m.vimState.Mode = vim.ModeInsert // reset to insert after submit
		return m, func() tea.Msg {
			return SubmitMsg{Text: text}
		}
	default:
		// Ignore special keys in Normal mode
		return m, nil
	}

	text := m.textarea.Value()
	cursor := m.flatCursorPos()
	action := m.vimState.HandleKey(key, text, cursor)
	return m.applyVimAction(action, cursor)
}

// applyVimAction translates a vim Action into textarea operations.
func (m Model) applyVimAction(action vim.Action, cursor int) (Model, tea.Cmd) {
	text := m.textarea.Value()

	// In Normal mode, cursor stays on last char (len-1), not after it.
	// In Insert mode, cursor can go after last char (len).
	maxCursor := len(text)
	if m.vimState.Mode == vim.ModeNormal || m.vimState.Mode == vim.ModeVisual {
		if maxCursor > 0 {
			maxCursor = len(text) - 1
		}
	}

	switch action.Type {
	case vim.ActionNone:
		// nothing

	case vim.ActionMoveCursor:
		m.setFlatCursor(clamp(cursor+action.MoveCursor, 0, maxCursor))

	case vim.ActionSetCursor:
		if action.SetCursor != vim.NoPos {
			m.setFlatCursor(clamp(action.SetCursor, 0, maxCursor))
		}

	case vim.ActionDeleteRange:
		m.pushUndo(text)
		from := clamp(action.DeleteFrom, 0, len(text))
		to := clamp(action.DeleteTo, 0, len(text))
		if from > to {
			from, to = to, from
		}
		// If action includes replacement text (e.g. visual u/U), insert it
		var newText string
		if action.Text != "" {
			newText = text[:from] + action.Text + text[to:]
		} else {
			newText = text[:from] + text[to:]
		}
		m.textarea.SetValue(newText)
		m.setFlatCursor(clamp(from, 0, len(newText)))

	case vim.ActionYank:
		// Clipboard is managed by vim.State

	case vim.ActionPaste:
		if action.Text != "" {
			m.pushUndo(text)
			pos := clamp(cursor, 0, len(text))
			if action.SetCursor != vim.NoPos {
				pos = clamp(action.SetCursor, 0, len(text))
			}
			newText := text[:pos] + action.Text + text[pos:]
			m.textarea.SetValue(newText)
			m.setFlatCursor(pos + len(action.Text))
		}

	case vim.ActionInsertText:
		if action.Text != "" {
			m.pushUndo(text)
			pos := cursor
			if action.SetCursor != vim.NoPos {
				pos = action.SetCursor
			}
			pos = clamp(pos, 0, len(text))
			newText := text[:pos] + action.Text + text[pos:]
			m.textarea.SetValue(newText)
			finalPos := pos + len(action.Text) + action.MoveCursor
			m.setFlatCursor(clamp(finalPos, 0, len(newText)))
		}

	case vim.ActionSwitchMode:
		// Compute target cursor position
		targetPos := cursor
		if action.SetCursor != vim.NoPos {
			targetPos = action.SetCursor
		}
		targetPos += action.MoveCursor
		targetPos = clamp(targetPos, 0, len(text))

		if action.Text != "" {
			m.pushUndo(text)
			insertPos := cursor
			if action.SetCursor != vim.NoPos {
				insertPos = action.SetCursor
			}
			insertPos = clamp(insertPos, 0, len(text))
			newText := text[:insertPos] + action.Text + text[insertPos:]
			m.textarea.SetValue(newText)
			m.setFlatCursor(insertPos + len(action.Text))
		} else {
			m.setFlatCursor(targetPos)
		}

	case vim.ActionReplaceChar:
		m.pushUndo(text)
		from := clamp(action.DeleteFrom, 0, len(text))
		to := clamp(action.DeleteTo, 0, len(text))
		newText := text[:from] + action.Text + text[to:]
		m.textarea.SetValue(newText)
		m.setFlatCursor(clamp(from+len(action.Text)-1, 0, len(newText)))

	case vim.ActionToggleCase:
		m.pushUndo(text)
		if action.DeleteFrom > 0 || action.DeleteTo > 0 {
			// Visual mode range
			from := clamp(action.DeleteFrom, 0, len(text))
			to := clamp(action.DeleteTo, 0, len(text))
			toggled := toggleCase(text[from:to])
			newText := text[:from] + toggled + text[to:]
			m.textarea.SetValue(newText)
			m.setFlatCursor(from)
		} else if cursor < len(text) {
			// Single char under cursor
			toggled := toggleCase(string(text[cursor]))
			newText := text[:cursor] + toggled + text[cursor+1:]
			m.textarea.SetValue(newText)
			m.setFlatCursor(clamp(cursor+1, 0, len(newText)))
		}

	case vim.ActionJoinLines:
		end := lineEndPos(text, cursor)
		if end < len(text) {
			m.pushUndo(text)
			// Remove newline and leading whitespace on next line
			joinPos := end
			next := end + 1
			for next < len(text) && (text[next] == ' ' || text[next] == '\t') {
				next++
			}
			newText := text[:joinPos] + " " + text[next:]
			m.textarea.SetValue(newText)
			m.setFlatCursor(joinPos)
		}

	case vim.ActionUndo:
		if len(m.undoStack) > 0 {
			prev := m.undoStack[len(m.undoStack)-1]
			m.undoStack = m.undoStack[:len(m.undoStack)-1]
			m.textarea.SetValue(prev)
			m.textarea.CursorEnd()
		}

	case vim.ActionRedo:
		// Not implemented
	}

	return m, nil
}

func toggleCase(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			b.WriteRune(r - 32)
		} else if r >= 'A' && r <= 'Z' {
			b.WriteRune(r + 32)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func lineEndPos(text string, cursor int) int {
	for i := cursor; i < len(text); i++ {
		if text[i] == '\n' {
			return i
		}
	}
	return len(text)
}

// ── Cursor Bridge ────────────────────────────────────────

// flatCursorPos converts the textarea's (row, col) to a flat byte offset.
func (m *Model) flatCursorPos() int {
	text := m.textarea.Value()
	lines := strings.Split(text, "\n")
	row := m.textarea.Line()
	col := m.textarea.LineInfo().ColumnOffset

	pos := 0
	for i := 0; i < row && i < len(lines); i++ {
		pos += len(lines[i]) + 1
	}
	pos += col
	if pos > len(text) {
		pos = len(text)
	}
	return pos
}

// setFlatCursor navigates the textarea to a flat byte offset.
func (m *Model) setFlatCursor(pos int) {
	text := m.textarea.Value()
	if pos > len(text) {
		pos = len(text)
	}
	if pos < 0 {
		pos = 0
	}

	// Convert flat pos to (row, col)
	row, col := 0, 0
	for i := 0; i < pos && i < len(text); i++ {
		if text[i] == '\n' {
			row++
			col = 0
		} else {
			col++
		}
	}

	// Reset to top-left first, then navigate to target.
	// This avoids relying on textarea's internal cursor state which
	// can be stale after SetValue() calls.
	m.textarea.CursorStart()
	for m.textarea.Line() > 0 {
		m.textarea.CursorUp()
	}
	m.textarea.SetCursor(0)

	// Navigate to target row
	for i := 0; i < row; i++ {
		m.textarea.CursorDown()
	}
	m.textarea.SetCursor(col)
}

func (m *Model) pushUndo(text string) {
	m.undoStack = append(m.undoStack, text)
	if len(m.undoStack) > maxUndo {
		m.undoStack = m.undoStack[1:]
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ── Paste Handling ───────────────────────────────────────

// finalizePaste processes accumulated paste buffer.
func (m *Model) finalizePaste() {
	m.isPasting = false
	text := m.pasteBuffer.String()
	m.pasteBuffer.Reset()

	if text == "" {
		return
	}

	if len(text) < pasteThreshold {
		// Short paste: insert directly
		m.textarea.InsertString(text)
		return
	}

	// Long paste: collapse to a reference
	m.nextPasteID++
	id := m.nextPasteID
	m.pastedContents[id] = text

	lines := strings.Count(text, "\n")
	var ref string
	if lines > 0 {
		ref = fmt.Sprintf("[Pasted text #%d +%d lines]", id, lines)
	} else {
		ref = fmt.Sprintf("[Pasted text #%d]", id)
	}

	m.textarea.InsertString(ref)
}

// ExpandedValue returns the prompt text with all paste references expanded
// to their original content. Used for submitting and external editor.
func (m *Model) ExpandedValue() string {
	text := m.textarea.Value()
	if len(m.pastedContents) == 0 {
		return text
	}

	return pasteRefRe.ReplaceAllStringFunc(text, func(match string) string {
		subs := pasteRefRe.FindStringSubmatch(match)
		if len(subs) < 2 {
			return match
		}
		var id int
		fmt.Sscanf(subs[1], "%d", &id)
		if content, ok := m.pastedContents[id]; ok {
			return content
		}
		return match
	})
}

// SetValueWithCollapse sets the prompt text, re-collapsing any large blocks
// that match stored paste content. Used after returning from external editor.
func (m *Model) SetValueWithCollapse(content string) {
	// If there are stored pastes, try to re-collapse them
	for id, pastedText := range m.pastedContents {
		if strings.Contains(content, pastedText) {
			lines := strings.Count(pastedText, "\n")
			var ref string
			if lines > 0 {
				ref = fmt.Sprintf("[Pasted text #%d +%d lines]", id, lines)
			} else {
				ref = fmt.Sprintf("[Pasted text #%d]", id)
			}
			content = strings.Replace(content, pastedText, ref, 1)
		}
	}
	m.textarea.SetValue(content)
	m.textarea.CursorEnd()
}

// ── Image Attachments ────────────────────────────────────

// AddImage attaches an image to the prompt and inserts a reference.
func (m *Model) AddImage(fileName, mediaType, base64Data string) {
	m.nextImageID++
	m.images = append(m.images, ImageAttachment{
		ID:        m.nextImageID,
		FileName:  fileName,
		MediaType: mediaType,
		Data:      base64Data,
	})

	ref := fmt.Sprintf("[Image #%d: %s]", m.nextImageID, fileName)
	m.textarea.InsertString(ref)
}

// Images returns all attached images.
func (m *Model) Images() []ImageAttachment {
	return m.images
}

// ClearImages removes all image attachments.
func (m *Model) ClearImages() {
	m.images = nil
}

// syncImagesFromText removes image attachments whose references no longer
// appear in the textarea text (e.g. user deleted the [Image #N: ...] token).
func (m *Model) syncImagesFromText() {
	if len(m.images) == 0 {
		return
	}
	text := m.textarea.Value()
	kept := m.images[:0]
	for _, img := range m.images {
		ref := fmt.Sprintf("[Image #%d:", img.ID)
		if strings.Contains(text, ref) {
			kept = append(kept, img)
		}
	}
	m.images = kept
}

// syncPastesFromText removes paste entries whose references no longer appear
// in the textarea (e.g. user deleted the [Pasted text #N ...] token).
func (m *Model) syncPastesFromText() {
	if len(m.pastedContents) == 0 {
		return
	}
	text := m.textarea.Value()
	for id := range m.pastedContents {
		ref := fmt.Sprintf("[Pasted text #%d", id)
		if !strings.Contains(text, ref) {
			delete(m.pastedContents, id)
		}
	}
}

// ImageCount returns the number of attached images.
func (m *Model) ImageCount() int {
	return len(m.images)
}

// View renders the prompt with a left accent bar.
// Height returns the rendered height of the prompt in terminal rows.
func (m Model) Height() int {
	return lipgloss.Height(m.View())
}

func (m Model) View() string {
	bar := styles.PromptBarFocused
	if !m.focused {
		bar = styles.PromptBarBlurred
	}

	content := bar.Width(m.width - 2).Render(m.textarea.View())

	// Build and render context pills (images, pastes, @mentions)
	pills := BuildPills(m.images, m.pastedContents, m.textarea.Value())
	pillsRow := RenderPills(pills, m.width)
	if pillsRow != "" {
		divider := lipgloss.NewStyle().Foreground(styles.Subtle).Render(strings.Repeat("─", m.width-3))
		content = pillsRow + "\n" + divider + "\n" + content
	}

	return content
}
