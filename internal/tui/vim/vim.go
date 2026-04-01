package vim

import (
	"strings"
	"unicode"
)

// Mode represents the current vim mode.
type Mode int

const (
	ModeNormal Mode = iota
	ModeInsert
	ModeVisual
	ModeOperatorPending
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
	default:
		return "UNKNOWN"
	}
}

// State holds all vim mode state.
type State struct {
	Mode           Mode
	PendingOp      rune // 'd', 'y', 'c', etc.
	Count          int  // numeric prefix
	Register       rune // " register
	LastSearch     string
	Clipboard      string
	VisualStart    int // cursor position when visual mode started
	CountBuf       string
}

// New creates a new vim state starting in insert mode.
func New() *State {
	return &State{Mode: ModeInsert}
}

// Action represents the result of processing a key in vim mode.
type Action struct {
	Type       ActionType
	Text       string // for insert text, yank content, etc.
	Count      int    // repeat count
	MoveCursor int    // relative cursor movement (-N or +N)
	SetCursor  int    // absolute cursor position (-1 = no change)
	DeleteFrom int    // delete range start
	DeleteTo   int    // delete range end
	SwitchMode Mode
}

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
)

// HandleKey processes a keystroke and returns the resulting action.
// `text` is the full buffer content, `cursor` is current position.
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
	}
	return Action{Type: ActionNone}
}

func (s *State) handleInsert(key rune) Action {
	if key == 27 { // Escape
		s.Mode = ModeNormal
		return Action{Type: ActionSwitchMode, SwitchMode: ModeNormal}
	}
	// In insert mode, pass through to textarea
	return Action{Type: ActionNone}
}

func (s *State) handleNormal(key rune, text string, cursor int) Action {
	// Handle count prefix
	if key >= '1' && key <= '9' || (key == '0' && s.CountBuf != "") {
		s.CountBuf += string(key)
		return Action{Type: ActionNone}
	}

	count := 1
	if s.CountBuf != "" {
		for _, ch := range s.CountBuf {
			count = count*10 + int(ch-'0')
		}
		s.CountBuf = ""
	}

	switch key {
	// Mode switching
	case 'i':
		s.Mode = ModeInsert
		return Action{Type: ActionSwitchMode, SwitchMode: ModeInsert}
	case 'a':
		s.Mode = ModeInsert
		return Action{Type: ActionSwitchMode, SwitchMode: ModeInsert, MoveCursor: 1}
	case 'I':
		s.Mode = ModeInsert
		lineStart := lineStartPos(text, cursor)
		return Action{Type: ActionSwitchMode, SwitchMode: ModeInsert, SetCursor: lineStart}
	case 'A':
		s.Mode = ModeInsert
		lineEnd := lineEndPos(text, cursor)
		return Action{Type: ActionSwitchMode, SwitchMode: ModeInsert, SetCursor: lineEnd}
	case 'o':
		s.Mode = ModeInsert
		lineEnd := lineEndPos(text, cursor)
		return Action{Type: ActionInsertText, Text: "\n", SetCursor: lineEnd + 1, SwitchMode: ModeInsert}
	case 'O':
		s.Mode = ModeInsert
		lineStart := lineStartPos(text, cursor)
		return Action{Type: ActionInsertText, Text: "\n", SetCursor: lineStart, SwitchMode: ModeInsert}
	case 'v':
		s.Mode = ModeVisual
		s.VisualStart = cursor
		return Action{Type: ActionSwitchMode, SwitchMode: ModeVisual}

	// Motions
	case 'h':
		return Action{Type: ActionMoveCursor, MoveCursor: -count}
	case 'l':
		return Action{Type: ActionMoveCursor, MoveCursor: count}
	case 'j':
		return moveLine(text, cursor, count)
	case 'k':
		return moveLine(text, cursor, -count)
	case 'w':
		pos := wordForward(text, cursor, count)
		return Action{Type: ActionSetCursor, SetCursor: pos}
	case 'b':
		pos := wordBackward(text, cursor, count)
		return Action{Type: ActionSetCursor, SetCursor: pos}
	case 'e':
		pos := wordEnd(text, cursor, count)
		return Action{Type: ActionSetCursor, SetCursor: pos}
	case '0':
		return Action{Type: ActionSetCursor, SetCursor: lineStartPos(text, cursor)}
	case '$':
		return Action{Type: ActionSetCursor, SetCursor: lineEndPos(text, cursor)}
	case 'g':
		// gg = go to start
		return Action{Type: ActionSetCursor, SetCursor: 0}
	case 'G':
		return Action{Type: ActionSetCursor, SetCursor: len(text)}

	// Operators
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

	// Direct actions
	case 'x':
		if cursor < len(text) {
			return Action{Type: ActionDeleteRange, DeleteFrom: cursor, DeleteTo: cursor + count}
		}
	case 'p':
		if s.Clipboard != "" {
			return Action{Type: ActionPaste, Text: s.Clipboard, SetCursor: cursor + 1}
		}
	case 'u':
		return Action{Type: ActionUndo}
	case 'r' - 96: // Ctrl+R
		return Action{Type: ActionRedo}

	case 27: // Escape — stay in normal mode
		return Action{Type: ActionNone}
	}

	return Action{Type: ActionNone}
}

func (s *State) handleVisual(key rune, text string, cursor int) Action {
	switch key {
	case 27: // Escape
		s.Mode = ModeNormal
		return Action{Type: ActionSwitchMode, SwitchMode: ModeNormal}
	case 'd', 'x':
		from, to := s.VisualStart, cursor
		if from > to {
			from, to = to, from
		}
		s.Clipboard = text[from : to+1]
		s.Mode = ModeNormal
		return Action{Type: ActionDeleteRange, DeleteFrom: from, DeleteTo: to + 1, SwitchMode: ModeNormal}
	case 'y':
		from, to := s.VisualStart, cursor
		if from > to {
			from, to = to, from
		}
		s.Clipboard = text[from : to+1]
		s.Mode = ModeNormal
		return Action{Type: ActionYank, Text: s.Clipboard, SwitchMode: ModeNormal}
	case 'c':
		from, to := s.VisualStart, cursor
		if from > to {
			from, to = to, from
		}
		s.Mode = ModeInsert
		return Action{Type: ActionDeleteRange, DeleteFrom: from, DeleteTo: to + 1, SwitchMode: ModeInsert}
	// Motions extend selection
	case 'h':
		return Action{Type: ActionMoveCursor, MoveCursor: -1}
	case 'l':
		return Action{Type: ActionMoveCursor, MoveCursor: 1}
	case 'w':
		pos := wordForward(text, cursor, 1)
		return Action{Type: ActionSetCursor, SetCursor: pos}
	case 'b':
		pos := wordBackward(text, cursor, 1)
		return Action{Type: ActionSetCursor, SetCursor: pos}
	}
	return Action{Type: ActionNone}
}

func (s *State) handleOperatorPending(key rune, text string, cursor int) Action {
	s.Mode = ModeNormal
	op := s.PendingOp
	count := s.Count
	s.PendingOp = 0
	s.Count = 0

	// dd, yy, cc — operate on whole line
	if key == op {
		lineStart := lineStartPos(text, cursor)
		lineEnd := lineEndPos(text, cursor)
		if lineEnd < len(text) {
			lineEnd++ // include newline
		}
		content := text[lineStart:lineEnd]

		switch op {
		case 'd':
			s.Clipboard = content
			return Action{Type: ActionDeleteRange, DeleteFrom: lineStart, DeleteTo: lineEnd}
		case 'y':
			s.Clipboard = content
			return Action{Type: ActionYank, Text: content}
		case 'c':
			s.Clipboard = content
			s.Mode = ModeInsert
			return Action{Type: ActionDeleteRange, DeleteFrom: lineStart, DeleteTo: lineEnd, SwitchMode: ModeInsert}
		}
	}

	// Operator + motion
	var targetPos int
	switch key {
	case 'w':
		targetPos = wordForward(text, cursor, count)
	case 'b':
		targetPos = wordBackward(text, cursor, count)
	case 'e':
		targetPos = wordEnd(text, cursor, count) + 1
	case '$':
		targetPos = lineEndPos(text, cursor)
	case '0':
		targetPos = lineStartPos(text, cursor)
	default:
		return Action{Type: ActionNone}
	}

	from, to := cursor, targetPos
	if from > to {
		from, to = to, from
	}
	content := ""
	if from < len(text) && to <= len(text) {
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

// --- Motion helpers ---

func lineStartPos(text string, cursor int) int {
	if cursor <= 0 {
		return 0
	}
	i := cursor - 1
	for i >= 0 && text[i] != '\n' {
		i--
	}
	return i + 1
}

func lineEndPos(text string, cursor int) int {
	i := cursor
	for i < len(text) && text[i] != '\n' {
		i++
	}
	return i
}

func moveLine(text string, cursor, lines int) Action {
	// Find current line and column
	currentLine, col := posToLineCol(text, cursor)
	targetLine := currentLine + lines
	if targetLine < 0 {
		targetLine = 0
	}

	allLines := strings.Split(text, "\n")
	if targetLine >= len(allLines) {
		targetLine = len(allLines) - 1
	}

	pos := lineColToPos(text, targetLine, col)
	return Action{Type: ActionSetCursor, SetCursor: pos}
}

func posToLineCol(text string, pos int) (line, col int) {
	for i := 0; i < pos && i < len(text); i++ {
		if text[i] == '\n' {
			line++
			col = 0
		} else {
			col++
		}
	}
	return
}

func lineColToPos(text string, line, col int) int {
	currentLine := 0
	pos := 0
	for pos < len(text) {
		if currentLine == line {
			break
		}
		if text[pos] == '\n' {
			currentLine++
		}
		pos++
	}
	// Now at start of target line, advance by col
	for i := 0; i < col && pos < len(text) && text[pos] != '\n'; i++ {
		pos++
	}
	return pos
}

func wordForward(text string, cursor, count int) int {
	pos := cursor
	runes := []rune(text)
	for c := 0; c < count; c++ {
		// Skip current word
		for pos < len(runes) && !unicode.IsSpace(runes[pos]) {
			pos++
		}
		// Skip whitespace
		for pos < len(runes) && unicode.IsSpace(runes[pos]) {
			pos++
		}
	}
	if pos > len(runes) {
		pos = len(runes)
	}
	return pos
}

func wordBackward(text string, cursor, count int) int {
	pos := cursor
	runes := []rune(text)
	for c := 0; c < count; c++ {
		// Skip whitespace behind
		for pos > 0 && unicode.IsSpace(runes[pos-1]) {
			pos--
		}
		// Skip word behind
		for pos > 0 && !unicode.IsSpace(runes[pos-1]) {
			pos--
		}
	}
	if pos < 0 {
		pos = 0
	}
	return pos
}

func wordEnd(text string, cursor, count int) int {
	pos := cursor + 1
	runes := []rune(text)
	for c := 0; c < count; c++ {
		// Skip whitespace
		for pos < len(runes) && unicode.IsSpace(runes[pos]) {
			pos++
		}
		// Go to end of word
		for pos < len(runes)-1 && !unicode.IsSpace(runes[pos+1]) {
			pos++
		}
	}
	if pos >= len(runes) {
		pos = len(runes) - 1
	}
	if pos < 0 {
		pos = 0
	}
	return pos
}
