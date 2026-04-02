package vim

import "strings"

// Mode represents the current vim mode.
type Mode int

const (
	ModeNormal Mode = iota
	ModeInsert
	ModeVisual
	ModeOperatorPending
	ModeCharSearch // waiting for char after f/F/t/T
	ModeReplace    // waiting for char after r
)

// String returns the display name of the mode.
func (m Mode) String() string {
	switch m {
	case ModeNormal:
		return "NORMAL"
	case ModeInsert:
		return "INSERT"
	case ModeVisual:
		return "VISUAL"
	case ModeOperatorPending:
		return "OP-PENDING"
	case ModeCharSearch:
		return "NORMAL"
	case ModeReplace:
		return "NORMAL"
	default:
		return "UNKNOWN"
	}
}

// State holds all vim mode state.
type State struct {
	Mode        Mode
	PendingOp   rune   // 'd', 'y', 'c', etc.
	Count       int    // numeric prefix
	CountBuf    string // buffer for building numeric prefix
	Register    rune   // " register
	LastSearch  string
	Clipboard   string
	VisualStart int // cursor position when visual mode started

	// Character search state (f/F/t/T)
	charSearchDir  int  // +1 forward, -1 backward
	charSearchTill bool // true for t/T (stop before char)
	lastCharSearch rune
	lastCharDir    int
	lastCharTill   bool

	// Repeat (.) state
	lastChange    []rune // keys of last change
	recording     []rune // currently recording change keys
	isRecording   bool
	lastChangeOp  rune // for operator-based changes
	lastChangeTxt string

	// Pending text object: 'i' or 'a' after operator
	pendingInner bool // true after 'i' in operator pending
	pendingOuter bool // true after 'a' in operator pending
}

// New creates a new vim state starting in insert mode.
func New() *State {
	return &State{Mode: ModeNormal}
}

// Action represents the result of processing a key in vim mode.
// SetCursor uses -1 as sentinel for "no change" (use NoPos constant).
type Action struct {
	Type       ActionType
	Text       string // for insert text, yank content, etc.
	Count      int    // repeat count
	MoveCursor int    // relative cursor movement (-N or +N)
	SetCursor  int    // absolute cursor position (NoPos = no change)
	DeleteFrom int    // delete range start
	DeleteTo   int    // delete range end
	SwitchMode Mode
}

// NoPos indicates no cursor position change in an Action.
const NoPos = -1

// ActionType identifies what action to perform.
type ActionType int

const (
	ActionNone ActionType = iota
	ActionMoveCursor
	ActionSetCursor
	ActionDeleteRange
	ActionYank
	ActionPaste
	ActionInsertText
	ActionSwitchMode
	ActionUndo
	ActionRedo
	ActionSearchForward
	ActionSearchBackward
	ActionReplaceChar // replace character under cursor
	ActionToggleCase  // toggle case of character under cursor
	ActionJoinLines   // join current line with next
)

// HandleKey processes a keystroke and returns the resulting action.
func (s *State) HandleKey(key rune, text string, cursor int) Action {
	switch s.Mode {
	case ModeInsert:
		return s.handleInsert(key)
	case ModeNormal:
		return s.handleNormal(key, text, cursor)
	case ModeVisual:
		return s.handleVisual(key, text, cursor)
	case ModeOperatorPending:
		return s.handleOperatorPending(key, text, cursor)
	case ModeCharSearch:
		return s.handleCharSearch(key, text, cursor)
	case ModeReplace:
		return s.handleReplace(key, text, cursor)
	}
	return Action{Type: ActionNone}
}

// ── Insert Mode ─────────────────────────────────────────

func (s *State) handleInsert(key rune) Action {
	if key == 27 { // Escape
		s.Mode = ModeNormal
		return Action{Type: ActionSwitchMode, SwitchMode: ModeNormal, SetCursor: NoPos}
	}
	return Action{Type: ActionNone}
}

// ── Normal Mode ─────────────────────────────────────────

func (s *State) handleNormal(key rune, text string, cursor int) Action {
	// Numeric prefix
	if key >= '1' && key <= '9' || (key == '0' && s.CountBuf != "") {
		s.CountBuf += string(key)
		return Action{Type: ActionNone}
	}

	count := s.consumeCount()

	switch key {
	// ── Mode switching ──
	case 'i':
		s.Mode = ModeInsert
		s.startRecording()
		return Action{Type: ActionSwitchMode, SwitchMode: ModeInsert, SetCursor: NoPos}
	case 'a':
		s.Mode = ModeInsert
		s.startRecording()
		return Action{Type: ActionSwitchMode, SwitchMode: ModeInsert, MoveCursor: 1, SetCursor: NoPos}
	case 'I':
		s.Mode = ModeInsert
		s.startRecording()
		pos := firstNonBlank(text, cursor)
		return Action{Type: ActionSwitchMode, SwitchMode: ModeInsert, SetCursor: pos}
	case 'A':
		s.Mode = ModeInsert
		s.startRecording()
		return Action{Type: ActionSwitchMode, SwitchMode: ModeInsert, SetCursor: lineEndPos(text, cursor)}
	case 'o':
		s.Mode = ModeInsert
		s.startRecording()
		end := lineEndPos(text, cursor)
		return Action{Type: ActionInsertText, Text: "\n", SetCursor: end + 1, SwitchMode: ModeInsert}
	case 'O':
		s.Mode = ModeInsert
		s.startRecording()
		start := lineStartPos(text, cursor)
		return Action{Type: ActionInsertText, Text: "\n", SetCursor: start, SwitchMode: ModeInsert}
	case 'v':
		s.Mode = ModeVisual
		s.VisualStart = cursor
		return Action{Type: ActionSwitchMode, SwitchMode: ModeVisual, SetCursor: NoPos}

	// ── Motions ──
	case 'h':
		return Action{Type: ActionMoveCursor, MoveCursor: -count}
	case 'l':
		return Action{Type: ActionMoveCursor, MoveCursor: count}
	case 'j':
		return moveLine(text, cursor, count)
	case 'k':
		return moveLine(text, cursor, -count)
	case 'w':
		return Action{Type: ActionSetCursor, SetCursor: wordForward(text, cursor, count)}
	case 'W':
		return Action{Type: ActionSetCursor, SetCursor: wordForwardBig(text, cursor, count)}
	case 'b':
		return Action{Type: ActionSetCursor, SetCursor: wordBackward(text, cursor, count)}
	case 'B':
		return Action{Type: ActionSetCursor, SetCursor: wordBackwardBig(text, cursor, count)}
	case 'e':
		return Action{Type: ActionSetCursor, SetCursor: wordEnd(text, cursor, count)}
	case 'E':
		return Action{Type: ActionSetCursor, SetCursor: wordEndBig(text, cursor, count)}
	case '0':
		return Action{Type: ActionSetCursor, SetCursor: lineStartPos(text, cursor)}
	case '^', '_':
		return Action{Type: ActionSetCursor, SetCursor: firstNonBlank(text, cursor)}
	case '$':
		return Action{Type: ActionSetCursor, SetCursor: lineEndPos(text, cursor)}
	case 'g':
		return Action{Type: ActionSetCursor, SetCursor: 0}
	case 'G':
		return Action{Type: ActionSetCursor, SetCursor: len(text)}
	case '{':
		return Action{Type: ActionSetCursor, SetCursor: paragraphBackward(text, cursor, count)}
	case '}':
		return Action{Type: ActionSetCursor, SetCursor: paragraphForward(text, cursor, count)}
	case '%':
		if pos := matchBracket(text, cursor); pos >= 0 {
			return Action{Type: ActionSetCursor, SetCursor: pos}
		}

	// ── Character search ──
	case 'f':
		s.Mode = ModeCharSearch
		s.charSearchDir = 1
		s.charSearchTill = false
		s.Count = count
		return Action{Type: ActionNone}
	case 'F':
		s.Mode = ModeCharSearch
		s.charSearchDir = -1
		s.charSearchTill = false
		s.Count = count
		return Action{Type: ActionNone}
	case 't':
		s.Mode = ModeCharSearch
		s.charSearchDir = 1
		s.charSearchTill = true
		s.Count = count
		return Action{Type: ActionNone}
	case 'T':
		s.Mode = ModeCharSearch
		s.charSearchDir = -1
		s.charSearchTill = true
		s.Count = count
		return Action{Type: ActionNone}
	case ';':
		if s.lastCharSearch != 0 {
			pos := findChar(text, cursor, s.lastCharSearch, s.lastCharDir, s.lastCharTill, count)
			if pos >= 0 {
				return Action{Type: ActionSetCursor, SetCursor: pos}
			}
		}
	case ',':
		if s.lastCharSearch != 0 {
			pos := findChar(text, cursor, s.lastCharSearch, -s.lastCharDir, s.lastCharTill, count)
			if pos >= 0 {
				return Action{Type: ActionSetCursor, SetCursor: pos}
			}
		}

	// ── Operators ──
	case 'd':
		s.Mode = ModeOperatorPending
		s.PendingOp = 'd'
		s.Count = count
		return Action{Type: ActionNone}
	case 'y':
		s.Mode = ModeOperatorPending
		s.PendingOp = 'y'
		s.Count = count
		return Action{Type: ActionNone}
	case 'c':
		s.Mode = ModeOperatorPending
		s.PendingOp = 'c'
		s.Count = count
		return Action{Type: ActionNone}

	// ── Shortcuts (operator + implicit motion) ──
	case 'D': // d$
		end := lineEndPos(text, cursor)
		if cursor < end {
			s.Clipboard = text[cursor:end]
			return Action{Type: ActionDeleteRange, DeleteFrom: cursor, DeleteTo: end}
		}
	case 'C': // c$
		s.Mode = ModeInsert
		end := lineEndPos(text, cursor)
		if cursor < end {
			s.Clipboard = text[cursor:end]
			return Action{Type: ActionDeleteRange, DeleteFrom: cursor, DeleteTo: end, SwitchMode: ModeInsert}
		}
		return Action{Type: ActionSwitchMode, SwitchMode: ModeInsert, SetCursor: NoPos}
	case 'S': // cc
		s.Mode = ModeInsert
		start := lineStartPos(text, cursor)
		end := lineEndPos(text, cursor)
		s.Clipboard = text[start:end]
		return Action{Type: ActionDeleteRange, DeleteFrom: start, DeleteTo: end, SwitchMode: ModeInsert}
	case 'Y': // yy
		start := lineStartPos(text, cursor)
		end := lineEndPos(text, cursor)
		if end < len(text) {
			end++
		}
		s.Clipboard = text[start:end]
		return Action{Type: ActionYank, Text: s.Clipboard}

	// ── Direct actions ──
	case 'x':
		end := cursor + count
		if end > len(text) {
			end = len(text)
		}
		if cursor < end {
			s.Clipboard = text[cursor:end]
			return Action{Type: ActionDeleteRange, DeleteFrom: cursor, DeleteTo: end}
		}
	case 'r':
		s.Mode = ModeReplace
		s.Count = count
		return Action{Type: ActionNone}
	case 'p':
		if s.Clipboard != "" {
			return Action{Type: ActionPaste, Text: s.Clipboard, SetCursor: cursor + 1}
		}
	case 'P':
		if s.Clipboard != "" {
			return Action{Type: ActionPaste, Text: s.Clipboard, SetCursor: cursor}
		}
	case 'u':
		return Action{Type: ActionUndo}
	case 18: // Ctrl+R
		return Action{Type: ActionRedo}
	case 'J':
		return Action{Type: ActionJoinLines}
	case '~':
		return Action{Type: ActionToggleCase}
	case '.':
		// Repeat last change — handled by prompt layer
		return s.replayLastChange(text, cursor)

	case 27: // Escape
		return Action{Type: ActionNone}
	}

	return Action{Type: ActionNone}
}

// ── Visual Mode ─────────────────────────────────────────

func (s *State) handleVisual(key rune, text string, cursor int) Action {
	switch key {
	case 27: // Escape
		s.Mode = ModeNormal
		return Action{Type: ActionSwitchMode, SwitchMode: ModeNormal, SetCursor: NoPos}
	case 'd', 'x':
		from, to := orderRange(s.VisualStart, cursor)
		to++ // inclusive
		if to > len(text) {
			to = len(text)
		}
		s.Clipboard = text[from:to]
		s.Mode = ModeNormal
		return Action{Type: ActionDeleteRange, DeleteFrom: from, DeleteTo: to, SwitchMode: ModeNormal}
	case 'y':
		from, to := orderRange(s.VisualStart, cursor)
		to++
		if to > len(text) {
			to = len(text)
		}
		s.Clipboard = text[from:to]
		s.Mode = ModeNormal
		return Action{Type: ActionYank, Text: s.Clipboard, SwitchMode: ModeNormal}
	case 'c':
		from, to := orderRange(s.VisualStart, cursor)
		to++
		if to > len(text) {
			to = len(text)
		}
		s.Clipboard = text[from:to]
		s.Mode = ModeInsert
		return Action{Type: ActionDeleteRange, DeleteFrom: from, DeleteTo: to, SwitchMode: ModeInsert}
	case '~':
		from, to := orderRange(s.VisualStart, cursor)
		to++
		if to > len(text) {
			to = len(text)
		}
		s.Mode = ModeNormal
		return Action{Type: ActionToggleCase, DeleteFrom: from, DeleteTo: to, SwitchMode: ModeNormal}
	case 'U':
		from, to := orderRange(s.VisualStart, cursor)
		to++
		if to > len(text) {
			to = len(text)
		}
		upper := strings.ToUpper(text[from:to])
		s.Mode = ModeNormal
		return Action{Type: ActionDeleteRange, DeleteFrom: from, DeleteTo: to, Text: upper, SwitchMode: ModeNormal}
	case 'u':
		from, to := orderRange(s.VisualStart, cursor)
		to++
		if to > len(text) {
			to = len(text)
		}
		lower := strings.ToLower(text[from:to])
		s.Mode = ModeNormal
		return Action{Type: ActionDeleteRange, DeleteFrom: from, DeleteTo: to, Text: lower, SwitchMode: ModeNormal}

	// Motions extend selection
	case 'h':
		return Action{Type: ActionMoveCursor, MoveCursor: -1}
	case 'l':
		return Action{Type: ActionMoveCursor, MoveCursor: 1}
	case 'j':
		return moveLine(text, cursor, 1)
	case 'k':
		return moveLine(text, cursor, -1)
	case 'w':
		return Action{Type: ActionSetCursor, SetCursor: wordForward(text, cursor, 1)}
	case 'W':
		return Action{Type: ActionSetCursor, SetCursor: wordForwardBig(text, cursor, 1)}
	case 'b':
		return Action{Type: ActionSetCursor, SetCursor: wordBackward(text, cursor, 1)}
	case 'B':
		return Action{Type: ActionSetCursor, SetCursor: wordBackwardBig(text, cursor, 1)}
	case 'e':
		return Action{Type: ActionSetCursor, SetCursor: wordEnd(text, cursor, 1)}
	case '0':
		return Action{Type: ActionSetCursor, SetCursor: lineStartPos(text, cursor)}
	case '^', '_':
		return Action{Type: ActionSetCursor, SetCursor: firstNonBlank(text, cursor)}
	case '$':
		return Action{Type: ActionSetCursor, SetCursor: lineEndPos(text, cursor)}
	case 'g':
		return Action{Type: ActionSetCursor, SetCursor: 0}
	case 'G':
		return Action{Type: ActionSetCursor, SetCursor: len(text)}
	case '{':
		return Action{Type: ActionSetCursor, SetCursor: paragraphBackward(text, cursor, 1)}
	case '}':
		return Action{Type: ActionSetCursor, SetCursor: paragraphForward(text, cursor, 1)}
	case '%':
		if pos := matchBracket(text, cursor); pos >= 0 {
			return Action{Type: ActionSetCursor, SetCursor: pos}
		}
	}
	return Action{Type: ActionNone}
}

// ── Operator Pending Mode ───────────────────────────────

func (s *State) handleOperatorPending(key rune, text string, cursor int) Action {
	op := s.PendingOp
	count := s.Count

	// Escape cancels
	if key == 27 {
		s.Mode = ModeNormal
		s.PendingOp = 0
		s.Count = 0
		return Action{Type: ActionNone}
	}

	// Text objects: 'i' or 'a' prefix
	if key == 'i' && !s.pendingInner && !s.pendingOuter {
		s.pendingInner = true
		return Action{Type: ActionNone}
	}
	if key == 'a' && !s.pendingInner && !s.pendingOuter {
		s.pendingOuter = true
		return Action{Type: ActionNone}
	}

	// Handle text objects
	if s.pendingInner || s.pendingOuter {
		inner := s.pendingInner
		s.pendingInner = false
		s.pendingOuter = false
		s.Mode = ModeNormal
		s.PendingOp = 0
		s.Count = 0
		return s.applyTextObject(op, key, inner, text, cursor)
	}

	s.Mode = ModeNormal
	s.PendingOp = 0
	s.Count = 0

	// dd, yy, cc — operate on whole line
	if key == op {
		start := lineStartPos(text, cursor)
		end := lineEndPos(text, cursor)
		if end < len(text) {
			end++ // include newline
		}
		content := text[start:end]

		switch op {
		case 'd':
			s.Clipboard = content
			return Action{Type: ActionDeleteRange, DeleteFrom: start, DeleteTo: end}
		case 'y':
			s.Clipboard = content
			return Action{Type: ActionYank, Text: content}
		case 'c':
			s.Clipboard = content
			s.Mode = ModeInsert
			return Action{Type: ActionDeleteRange, DeleteFrom: start, DeleteTo: end, SwitchMode: ModeInsert}
		}
	}

	// Operator + motion
	targetPos := -1
	switch key {
	case 'w':
		targetPos = wordForward(text, cursor, count)
	case 'W':
		targetPos = wordForwardBig(text, cursor, count)
	case 'b':
		targetPos = wordBackward(text, cursor, count)
	case 'B':
		targetPos = wordBackwardBig(text, cursor, count)
	case 'e':
		targetPos = wordEnd(text, cursor, count) + 1
	case 'E':
		targetPos = wordEndBig(text, cursor, count) + 1
	case '$':
		targetPos = lineEndPos(text, cursor)
	case '0':
		targetPos = lineStartPos(text, cursor)
	case '^', '_':
		targetPos = firstNonBlank(text, cursor)
	case 'j':
		// Delete/yank/change whole lines
		start := lineStartPos(text, cursor)
		end := lineEndPos(text, cursor)
		for i := 0; i < count; i++ {
			if end < len(text) {
				end++
				end = lineEndPos(text, end)
			}
		}
		if end < len(text) {
			end++
		}
		return s.applyOperator(op, text, start, end)
	case 'k':
		end := lineEndPos(text, cursor)
		if end < len(text) {
			end++
		}
		start := lineStartPos(text, cursor)
		for i := 0; i < count; i++ {
			if start > 0 {
				start--
				start = lineStartPos(text, start)
			}
		}
		return s.applyOperator(op, text, start, end)
	case 'f', 'F', 't', 'T':
		// operator + char search: enter char search sub-mode
		s.Mode = ModeCharSearch
		s.PendingOp = op
		s.charSearchDir = 1
		s.charSearchTill = false
		if key == 'F' || key == 'T' {
			s.charSearchDir = -1
		}
		if key == 't' || key == 'T' {
			s.charSearchTill = true
		}
		s.Count = count
		return Action{Type: ActionNone}
	case 'G':
		targetPos = len(text)
	case 'g':
		targetPos = 0
	case '{':
		targetPos = paragraphBackward(text, cursor, count)
	case '}':
		targetPos = paragraphForward(text, cursor, count)
	default:
		return Action{Type: ActionNone}
	}

	if targetPos < 0 {
		return Action{Type: ActionNone}
	}
	from, to := orderRange(cursor, targetPos)
	return s.applyOperator(op, text, from, to)
}

// ── Char Search Mode ────────────────────────────────────

func (s *State) handleCharSearch(key rune, text string, cursor int) Action {
	savedOp := s.PendingOp
	s.PendingOp = 0
	s.Mode = ModeNormal
	count := s.Count
	if count == 0 {
		count = 1
	}
	s.Count = 0

	if key == 27 { // Escape cancels
		return Action{Type: ActionNone}
	}

	// Save for ; and , repeat
	s.lastCharSearch = key
	s.lastCharDir = s.charSearchDir
	s.lastCharTill = s.charSearchTill

	pos := findChar(text, cursor, key, s.charSearchDir, s.charSearchTill, count)
	if pos < 0 {
		return Action{Type: ActionNone}
	}

	// If we had a pending operator, apply it
	if savedOp != 0 {
		from, to := orderRange(cursor, pos)
		if s.charSearchDir > 0 {
			to++ // inclusive for forward char search with operator
		}
		return s.applyOperator(savedOp, text, from, to)
	}

	return Action{Type: ActionSetCursor, SetCursor: pos}
}

// ── Replace Mode ────────────────────────────────────────

func (s *State) handleReplace(key rune, text string, cursor int) Action {
	s.Mode = ModeNormal
	count := s.Count
	s.Count = 0

	if key == 27 { // Escape cancels
		return Action{Type: ActionNone}
	}

	// Replace `count` characters starting at cursor with `key`
	end := cursor + count
	if end > len(text) {
		end = len(text)
	}
	if cursor >= end {
		return Action{Type: ActionNone}
	}

	replacement := strings.Repeat(string(key), end-cursor)
	return Action{Type: ActionReplaceChar, DeleteFrom: cursor, DeleteTo: end, Text: replacement}
}

// ── Helpers ─────────────────────────────────────────────

func (s *State) consumeCount() int {
	count := 1
	if s.CountBuf != "" {
		count = 0
		for _, ch := range s.CountBuf {
			count = count*10 + int(ch-'0')
		}
		s.CountBuf = ""
	}
	return count
}

func (s *State) applyOperator(op rune, text string, from, to int) Action {
	if from > to {
		from, to = to, from
	}
	if from < 0 {
		from = 0
	}
	if to > len(text) {
		to = len(text)
	}
	content := ""
	if from < to {
		content = text[from:to]
	}

	switch op {
	case 'd':
		s.Clipboard = content
		return Action{Type: ActionDeleteRange, DeleteFrom: from, DeleteTo: to}
	case 'y':
		s.Clipboard = content
		return Action{Type: ActionYank, Text: content}
	case 'c':
		s.Clipboard = content
		s.Mode = ModeInsert
		return Action{Type: ActionDeleteRange, DeleteFrom: from, DeleteTo: to, SwitchMode: ModeInsert}
	}
	return Action{Type: ActionNone}
}

// applyTextObject handles text objects like iw, aw, i", a(, etc.
func (s *State) applyTextObject(op rune, obj rune, inner bool, text string, cursor int) Action {
	var from, to int
	found := false

	switch obj {
	case 'w': // word
		from, to = textObjWord(text, cursor, inner)
		found = true
	case 'W': // WORD
		from, to = textObjWordBig(text, cursor, inner)
		found = true
	case '"', '\'', '`':
		from, to = delimiterObject(text, cursor, obj, inner)
		found = from >= 0
	case '(', ')', 'b':
		from, to = bracketObject(text, cursor, '(', ')', inner)
		found = from >= 0
	case '[', ']':
		from, to = bracketObject(text, cursor, '[', ']', inner)
		found = from >= 0
	case '{', '}', 'B':
		from, to = bracketObject(text, cursor, '{', '}', inner)
		found = from >= 0
	case '<', '>':
		from, to = bracketObject(text, cursor, '<', '>', inner)
		found = from >= 0
	}

	if !found {
		return Action{Type: ActionNone}
	}
	return s.applyOperator(op, text, from, to)
}

func (s *State) startRecording() {
	s.isRecording = true
	s.recording = nil
}

func (s *State) stopRecording() {
	if s.isRecording {
		s.lastChange = s.recording
		s.isRecording = false
		s.recording = nil
	}
}

// RecordKey should be called by the prompt layer for each key in insert mode.
func (s *State) RecordKey(key rune) {
	if s.isRecording {
		s.recording = append(s.recording, key)
	}
}

func (s *State) replayLastChange(text string, cursor int) Action {
	// Simple dot repeat: just return ActionNone for now
	// Full replay would need the prompt layer to re-inject keys
	return Action{Type: ActionNone}
}

// ── Motion Helpers ──────────────────────────────────────

func lineStartPos(text string, cursor int) int {
	if cursor <= 0 {
		return 0
	}
	i := cursor
	if i > 0 {
		i--
	}
	for i > 0 && text[i] != '\n' {
		i--
	}
	if text[i] == '\n' {
		return i + 1
	}
	return 0
}

func lineEndPos(text string, cursor int) int {
	for i := cursor; i < len(text); i++ {
		if text[i] == '\n' {
			return i
		}
	}
	return len(text)
}

func firstNonBlank(text string, cursor int) int {
	start := lineStartPos(text, cursor)
	end := lineEndPos(text, cursor)
	for i := start; i < end; i++ {
		if text[i] != ' ' && text[i] != '\t' {
			return i
		}
	}
	return start
}

func moveLine(text string, cursor, lines int) Action {
	row, col := posToRowCol(text, cursor)
	targetRow := row + lines
	if targetRow < 0 {
		targetRow = 0
	}
	maxRow := strings.Count(text, "\n")
	if targetRow > maxRow {
		targetRow = maxRow
	}
	pos := rowColToPos(text, targetRow, col)
	return Action{Type: ActionSetCursor, SetCursor: pos}
}

func posToRowCol(text string, pos int) (row, col int) {
	for i := 0; i < pos && i < len(text); i++ {
		if text[i] == '\n' {
			row++
			col = 0
		} else {
			col++
		}
	}
	return
}

func rowColToPos(text string, targetRow, col int) int {
	row := 0
	pos := 0
	// Find start of target row
	for pos < len(text) && row < targetRow {
		if text[pos] == '\n' {
			row++
		}
		pos++
	}
	// Advance by col within the line
	for i := 0; i < col && pos < len(text) && text[pos] != '\n'; i++ {
		pos++
	}
	return pos
}

// ── Word Motions ────────────────────────────────────────

func isWordChar(r byte) bool {
	return r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

func wordForward(text string, cursor, count int) int {
	pos := cursor
	for c := 0; c < count && pos < len(text); c++ {
		// Skip current word (word chars or punct)
		if pos < len(text) && isWordChar(text[pos]) {
			for pos < len(text) && isWordChar(text[pos]) {
				pos++
			}
		} else if pos < len(text) && !isSpace(text[pos]) {
			for pos < len(text) && !isSpace(text[pos]) && !isWordChar(text[pos]) {
				pos++
			}
		}
		// Skip whitespace
		for pos < len(text) && isSpace(text[pos]) {
			pos++
		}
	}
	return pos
}

func wordBackward(text string, cursor, count int) int {
	pos := cursor
	for c := 0; c < count && pos > 0; c++ {
		// Skip whitespace behind
		for pos > 0 && isSpace(text[pos-1]) {
			pos--
		}
		// Skip word chars or punct behind
		if pos > 0 && isWordChar(text[pos-1]) {
			for pos > 0 && isWordChar(text[pos-1]) {
				pos--
			}
		} else if pos > 0 {
			for pos > 0 && !isSpace(text[pos-1]) && !isWordChar(text[pos-1]) {
				pos--
			}
		}
	}
	return pos
}

func wordEnd(text string, cursor, count int) int {
	pos := cursor
	for c := 0; c < count && pos < len(text)-1; c++ {
		pos++ // move off current char
		// Skip whitespace
		for pos < len(text) && isSpace(text[pos]) {
			pos++
		}
		// Go to end of word
		if pos < len(text) && isWordChar(text[pos]) {
			for pos < len(text)-1 && isWordChar(text[pos+1]) {
				pos++
			}
		} else {
			for pos < len(text)-1 && !isSpace(text[pos+1]) && !isWordChar(text[pos+1]) {
				pos++
			}
		}
	}
	if pos >= len(text) {
		pos = len(text) - 1
	}
	if pos < 0 {
		pos = 0
	}
	return pos
}

// WORD motions (space-delimited)

func wordForwardBig(text string, cursor, count int) int {
	pos := cursor
	for c := 0; c < count && pos < len(text); c++ {
		for pos < len(text) && !isSpace(text[pos]) {
			pos++
		}
		for pos < len(text) && isSpace(text[pos]) {
			pos++
		}
	}
	return pos
}

func wordBackwardBig(text string, cursor, count int) int {
	pos := cursor
	for c := 0; c < count && pos > 0; c++ {
		for pos > 0 && isSpace(text[pos-1]) {
			pos--
		}
		for pos > 0 && !isSpace(text[pos-1]) {
			pos--
		}
	}
	return pos
}

func wordEndBig(text string, cursor, count int) int {
	pos := cursor
	for c := 0; c < count && pos < len(text)-1; c++ {
		pos++
		for pos < len(text) && isSpace(text[pos]) {
			pos++
		}
		for pos < len(text)-1 && !isSpace(text[pos+1]) {
			pos++
		}
	}
	if pos >= len(text) {
		pos = len(text) - 1
	}
	if pos < 0 {
		pos = 0
	}
	return pos
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// ── Char Search ─────────────────────────────────────────

func findChar(text string, cursor int, ch rune, dir int, till bool, count int) int {
	target := byte(ch)
	if dir > 0 {
		pos := cursor
		for c := 0; c < count; c++ {
			pos++
			for pos < len(text) && text[pos] != '\n' && text[pos] != target {
				pos++
			}
			if pos >= len(text) || text[pos] == '\n' {
				return -1
			}
		}
		if till {
			pos--
		}
		return pos
	}
	// backward
	pos := cursor
	for c := 0; c < count; c++ {
		pos--
		for pos >= 0 && text[pos] != '\n' && text[pos] != target {
			pos--
		}
		if pos < 0 || text[pos] == '\n' {
			return -1
		}
	}
	if till {
		pos++
	}
	return pos
}

// ── Paragraph Motions ───────────────────────────────────

func paragraphForward(text string, cursor, count int) int {
	pos := cursor
	for c := 0; c < count; c++ {
		// Skip non-empty lines
		for pos < len(text) && text[pos] != '\n' {
			pos++
		}
		if pos < len(text) {
			pos++
		}
		// Skip empty lines
		for pos < len(text) && text[pos] == '\n' {
			pos++
		}
		// Find next empty line
		for pos < len(text) {
			if text[pos] == '\n' {
				break
			}
			pos++
		}
	}
	return pos
}

func paragraphBackward(text string, cursor, count int) int {
	pos := cursor
	for c := 0; c < count; c++ {
		if pos > 0 {
			pos--
		}
		// Skip current empty lines
		for pos > 0 && text[pos] == '\n' {
			pos--
		}
		// Find previous empty line
		for pos > 0 {
			if text[pos] == '\n' && (pos == 0 || text[pos-1] == '\n') {
				break
			}
			pos--
		}
	}
	return pos
}

// ── Bracket Matching ────────────────────────────────────

var bracketPairs = map[byte]byte{
	'(': ')', ')': '(',
	'[': ']', ']': '[',
	'{': '}', '}': '{',
	'<': '>', '>': '<',
}

func matchBracket(text string, cursor int) int {
	if cursor >= len(text) {
		return -1
	}
	ch := text[cursor]
	match, ok := bracketPairs[ch]
	if !ok {
		return -1
	}

	// Determine direction
	dir := 1
	if ch == ')' || ch == ']' || ch == '}' || ch == '>' {
		dir = -1
	}

	depth := 1
	pos := cursor + dir
	for pos >= 0 && pos < len(text) && depth > 0 {
		if text[pos] == ch {
			depth++
		} else if text[pos] == match {
			depth--
		}
		if depth > 0 {
			pos += dir
		}
	}
	if depth == 0 {
		return pos
	}
	return -1
}

// ── Text Objects ────────────────────────────────────────

func textObjWord(text string, cursor int, inner bool) (int, int) {
	if cursor >= len(text) {
		return cursor, cursor
	}

	from := cursor
	to := cursor

	if isWordChar(text[cursor]) {
		// Expand to word boundaries
		for from > 0 && isWordChar(text[from-1]) {
			from--
		}
		for to < len(text)-1 && isWordChar(text[to+1]) {
			to++
		}
		to++ // exclusive end
		if !inner {
			// Include trailing whitespace
			for to < len(text) && isSpace(text[to]) {
				to++
			}
		}
	} else if isSpace(text[cursor]) {
		// On whitespace: select whitespace
		for from > 0 && isSpace(text[from-1]) {
			from--
		}
		for to < len(text)-1 && isSpace(text[to+1]) {
			to++
		}
		to++
	} else {
		// Punctuation
		for from > 0 && !isSpace(text[from-1]) && !isWordChar(text[from-1]) {
			from--
		}
		for to < len(text)-1 && !isSpace(text[to+1]) && !isWordChar(text[to+1]) {
			to++
		}
		to++
		if !inner {
			for to < len(text) && isSpace(text[to]) {
				to++
			}
		}
	}

	return from, to
}

func textObjWordBig(text string, cursor int, inner bool) (int, int) {
	if cursor >= len(text) {
		return cursor, cursor
	}

	from := cursor
	to := cursor

	if isSpace(text[cursor]) {
		for from > 0 && isSpace(text[from-1]) {
			from--
		}
		for to < len(text)-1 && isSpace(text[to+1]) {
			to++
		}
		to++
	} else {
		for from > 0 && !isSpace(text[from-1]) {
			from--
		}
		for to < len(text)-1 && !isSpace(text[to+1]) {
			to++
		}
		to++
		if !inner {
			for to < len(text) && isSpace(text[to]) {
				to++
			}
		}
	}
	return from, to
}

func delimiterObject(text string, cursor int, delim rune, inner bool) (int, int) {
	d := byte(delim)

	// Search backward for opening delimiter
	from := cursor - 1
	for from >= 0 && text[from] != d {
		from--
	}
	if from < 0 {
		return -1, -1
	}

	// Search forward for closing delimiter
	to := cursor + 1
	if cursor < len(text) && text[cursor] == d {
		// Cursor is on the delimiter — search forward from cursor+1
		to = cursor + 1
	}
	for to < len(text) && text[to] != d {
		to++
	}
	if to >= len(text) {
		return -1, -1
	}

	if inner {
		return from + 1, to // exclusive end, between delimiters
	}
	return from, to + 1 // include delimiters
}

func bracketObject(text string, cursor int, open, close byte, inner bool) (int, int) {
	// Find matching brackets around cursor
	var from, to int

	// Search backward for unmatched open bracket
	depth := 0
	from = cursor
	for from >= 0 {
		if text[from] == close {
			depth++
		} else if text[from] == open {
			if depth == 0 {
				break
			}
			depth--
		}
		from--
	}
	if from < 0 {
		return -1, -1
	}

	// Search forward for matching close
	depth = 0
	to = cursor
	for to < len(text) {
		if text[to] == open {
			depth++
		} else if text[to] == close {
			if depth == 0 {
				break
			}
			depth--
		}
		to++
	}
	if to >= len(text) {
		return -1, -1
	}

	if inner {
		return from + 1, to
	}
	return from, to + 1
}

func orderRange(a, b int) (int, int) {
	if a > b {
		return b, a
	}
	return a, b
}
