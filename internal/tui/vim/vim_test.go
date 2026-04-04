package vim_test

import (
	"testing"

	"github.com/Abraxas-365/claudio/internal/tui/vim"
)

func TestModeTransitions(t *testing.T) {
	s := vim.New()

	// Start in normal mode
	if s.Mode != vim.ModeNormal {
		t.Fatalf("expected ModeNormal, got %v", s.Mode)
	}

	// i → Insert
	action := s.HandleKey('i', "hello", 3)
	if s.Mode != vim.ModeInsert {
		t.Fatalf("expected ModeInsert after i, got %v", s.Mode)
	}

	// Escape → Normal
	action = s.HandleKey(27, "hello", 3)
	if s.Mode != vim.ModeNormal {
		t.Fatalf("expected ModeNormal after Escape, got %v", s.Mode)
	}
	if action.Type != vim.ActionSwitchMode {
		t.Fatalf("expected ActionSwitchMode, got %v", action.Type)
	}

	// i → Insert
	action = s.HandleKey('i', "hello", 3)
	if s.Mode != vim.ModeInsert {
		t.Fatalf("expected ModeInsert after 'i', got %v", s.Mode)
	}

	// Back to Normal
	s.HandleKey(27, "hello", 3)

	// v → Visual
	action = s.HandleKey('v', "hello", 3)
	if s.Mode != vim.ModeVisual {
		t.Fatalf("expected ModeVisual after 'v', got %v", s.Mode)
	}

	// Escape → Normal
	s.HandleKey(27, "hello", 3)
	if s.Mode != vim.ModeNormal {
		t.Fatalf("expected ModeNormal after Escape from Visual, got %v", s.Mode)
	}
}

func TestMotions(t *testing.T) {
	s := vim.New()
	s.HandleKey(27, "", 0) // Enter normal mode

	text := "hello world\nfoo bar"

	// h = move left
	action := s.HandleKey('h', text, 5)
	if action.Type != vim.ActionMoveCursor || action.MoveCursor != -1 {
		t.Errorf("h: expected move -1, got %+v", action)
	}

	// l = move right
	action = s.HandleKey('l', text, 5)
	if action.Type != vim.ActionMoveCursor || action.MoveCursor != 1 {
		t.Errorf("l: expected move +1, got %+v", action)
	}

	// w = word forward
	action = s.HandleKey('w', text, 0)
	if action.Type != vim.ActionSetCursor || action.SetCursor != 6 {
		t.Errorf("w from 0: expected cursor 6, got %+v", action)
	}

	// b = word backward
	action = s.HandleKey('b', text, 6)
	if action.Type != vim.ActionSetCursor || action.SetCursor != 0 {
		t.Errorf("b from 6: expected cursor 0, got %+v", action)
	}

	// 0 = line start
	action = s.HandleKey('0', text, 5)
	if action.Type != vim.ActionSetCursor || action.SetCursor != 0 {
		t.Errorf("0: expected cursor 0, got %+v", action)
	}

	// $ = line end
	action = s.HandleKey('$', text, 0)
	if action.Type != vim.ActionSetCursor || action.SetCursor != 11 {
		t.Errorf("$: expected cursor 11, got %+v", action)
	}
}

func TestDelete(t *testing.T) {
	s := vim.New()
	s.HandleKey(27, "", 0) // Normal mode

	text := "hello world"

	// x = delete char
	action := s.HandleKey('x', text, 0)
	if action.Type != vim.ActionDeleteRange || action.DeleteFrom != 0 || action.DeleteTo != 1 {
		t.Errorf("x: expected delete 0-1, got %+v", action)
	}

	// dw = delete word
	s.HandleKey('d', text, 0) // Enter operator pending
	if s.Mode != vim.ModeOperatorPending {
		t.Fatal("expected operator pending mode")
	}
	action = s.HandleKey('w', text, 0)
	if action.Type != vim.ActionDeleteRange {
		t.Errorf("dw: expected ActionDeleteRange, got %+v", action)
	}
}

func TestOpenLineBelow(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		cursor    int
		wantPos   int // SetCursor (insertion point)
		wantText  string
		wantMove  int
	}{
		{
			name:     "o on single line",
			text:     "hello",
			cursor:   2,
			wantPos:  5, // end of "hello"
			wantText: "\n",
			wantMove: 0,
		},
		{
			name:     "o on first line of multiline",
			text:     "hello\nworld",
			cursor:   2,
			wantPos:  5, // position of \n
			wantText: "\n",
			wantMove: 0,
		},
		{
			name:     "o on empty text",
			text:     "",
			cursor:   0,
			wantPos:  0,
			wantText: "\n",
			wantMove: 0,
		},
		{
			name:     "o on last line",
			text:     "hello\nworld",
			cursor:   8,
			wantPos:  11, // end of "world"
			wantText: "\n",
			wantMove: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := vim.New()
			s.HandleKey(27, "", 0) // Normal mode

			action := s.HandleKey('o', tt.text, tt.cursor)

			if action.Type != vim.ActionInsertText {
				t.Fatalf("expected ActionInsertText, got %v", action.Type)
			}
			if action.Text != tt.wantText {
				t.Errorf("text: want %q, got %q", tt.wantText, action.Text)
			}
			if action.SetCursor != tt.wantPos {
				t.Errorf("SetCursor: want %d, got %d", tt.wantPos, action.SetCursor)
			}
			if action.MoveCursor != tt.wantMove {
				t.Errorf("MoveCursor: want %d, got %d", tt.wantMove, action.MoveCursor)
			}
			if s.Mode != vim.ModeInsert {
				t.Errorf("expected ModeInsert, got %v", s.Mode)
			}
		})
	}
}

func TestOpenLineAbove(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		cursor    int
		wantPos   int
		wantText  string
		wantMove  int
	}{
		{
			name:     "O on single line",
			text:     "hello",
			cursor:   2,
			wantPos:  0,
			wantText: "\n",
			wantMove: -1,
		},
		{
			name:     "O on second line of multiline",
			text:     "hello\nworld",
			cursor:   8,
			wantPos:  6, // start of "world"
			wantText: "\n",
			wantMove: -1,
		},
		{
			name:     "O on empty text",
			text:     "",
			cursor:   0,
			wantPos:  0,
			wantText: "\n",
			wantMove: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := vim.New()
			s.HandleKey(27, "", 0) // Normal mode

			action := s.HandleKey('O', tt.text, tt.cursor)

			if action.Type != vim.ActionInsertText {
				t.Fatalf("expected ActionInsertText, got %v", action.Type)
			}
			if action.Text != tt.wantText {
				t.Errorf("text: want %q, got %q", tt.wantText, action.Text)
			}
			if action.SetCursor != tt.wantPos {
				t.Errorf("SetCursor: want %d, got %d", tt.wantPos, action.SetCursor)
			}
			if action.MoveCursor != tt.wantMove {
				t.Errorf("MoveCursor: want %d, got %d", tt.wantMove, action.MoveCursor)
			}
			if s.Mode != vim.ModeInsert {
				t.Errorf("expected ModeInsert, got %v", s.Mode)
			}
		})
	}
}

func TestMoveLineKAtTop(t *testing.T) {
	s := vim.New()
	s.HandleKey(27, "", 0) // Normal mode

	// k at row 0 should stay at row 0
	action := s.HandleKey('k', "hello\nworld", 2)
	if action.Type != vim.ActionSetCursor {
		t.Fatalf("expected ActionSetCursor, got %v", action.Type)
	}
	// cursor at 2 is on row 0; k should clamp to row 0
	if action.SetCursor != 2 {
		t.Errorf("k at top: expected cursor 2 (same row), got %d", action.SetCursor)
	}
}

func TestMoveLineJAtBottom(t *testing.T) {
	s := vim.New()
	s.HandleKey(27, "", 0) // Normal mode

	// j at last row should stay on last row
	action := s.HandleKey('j', "hello\nworld", 8)
	if action.Type != vim.ActionSetCursor {
		t.Fatalf("expected ActionSetCursor, got %v", action.Type)
	}
	// cursor at 8 is on row 1 (last row); j should clamp to last row
	if action.SetCursor != 8 {
		t.Errorf("j at bottom: expected cursor 8 (same row), got %d", action.SetCursor)
	}
}

func TestModeString(t *testing.T) {
	tests := []struct {
		mode vim.Mode
		want string
	}{
		{vim.ModeNormal, "NORMAL"},
		{vim.ModeInsert, "INSERT"},
		{vim.ModeVisual, "VISUAL"},
	}
	for _, tt := range tests {
		if got := tt.mode.String(); got != tt.want {
			t.Errorf("Mode.String() = %q, want %q", got, tt.want)
		}
	}
}
