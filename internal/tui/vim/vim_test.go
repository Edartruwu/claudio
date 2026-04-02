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
