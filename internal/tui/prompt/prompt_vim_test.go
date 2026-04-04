package prompt

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Abraxas-365/claudio/internal/tui/vim"
)

// helper to create a prompt with text and cursor at a specific position in Normal mode.
func setupNormal(t *testing.T, text string, cursorPos int) Model {
	t.Helper()
	m := New()
	m.textarea.SetValue(text)
	m.setFlatCursor(cursorPos)
	// Ensure we're in Normal mode
	m.vimState.Mode = vim.ModeNormal
	return m
}

func cursorOf(m *Model) int {
	return m.flatCursorPos()
}

func TestVim_i_InsertsAtCursor(t *testing.T) {
	m := setupNormal(t, "hello", 3) // cursor on 'l' (index 3)

	// Press 'i' — should switch to Insert at same position (3)
	m, _ = m.handleVimKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})

	if m.vimState.Mode != vim.ModeInsert {
		t.Fatalf("expected Insert mode, got %v", m.vimState.Mode)
	}
	pos := cursorOf(&m)
	if pos != 3 {
		t.Errorf("'i' at pos 3: expected cursor at 3, got %d", pos)
	}
}

func TestVim_a_InsertsAfterCursor(t *testing.T) {
	m := setupNormal(t, "hello", 3) // cursor on 'l' (index 3)

	// Press 'a' — should switch to Insert at cursor+1 (4)
	m, _ = m.handleVimKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})

	if m.vimState.Mode != vim.ModeInsert {
		t.Fatalf("expected Insert mode, got %v", m.vimState.Mode)
	}
	pos := cursorOf(&m)
	if pos != 4 {
		t.Errorf("'a' at pos 3: expected cursor at 4, got %d", pos)
	}
}

func TestVim_a_AtEnd(t *testing.T) {
	m := setupNormal(t, "hello", 4) // cursor on last char 'o' (index 4)

	// Press 'a' — should switch to Insert at cursor+1 (5, which is len(text))
	m, _ = m.handleVimKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})

	if m.vimState.Mode != vim.ModeInsert {
		t.Fatalf("expected Insert mode, got %v", m.vimState.Mode)
	}
	pos := cursorOf(&m)
	if pos != 5 {
		t.Errorf("'a' at pos 4 (end): expected cursor at 5, got %d", pos)
	}
}

func TestVim_i_AtBeginning(t *testing.T) {
	m := setupNormal(t, "hello", 0)

	m, _ = m.handleVimKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})

	pos := cursorOf(&m)
	if pos != 0 {
		t.Errorf("'i' at pos 0: expected cursor at 0, got %d", pos)
	}
}

func TestVim_A_GoesToEndOfLine(t *testing.T) {
	m := setupNormal(t, "hello", 2)

	m, _ = m.handleVimKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})

	if m.vimState.Mode != vim.ModeInsert {
		t.Fatalf("expected Insert mode, got %v", m.vimState.Mode)
	}
	pos := cursorOf(&m)
	if pos != 5 {
		t.Errorf("'A' at pos 2: expected cursor at 5 (end), got %d", pos)
	}
}

func TestVim_I_GoesToStartOfLine(t *testing.T) {
	m := setupNormal(t, "  hello", 4) // cursor in middle

	m, _ = m.handleVimKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'I'}})

	if m.vimState.Mode != vim.ModeInsert {
		t.Fatalf("expected Insert mode, got %v", m.vimState.Mode)
	}
	pos := cursorOf(&m)
	// firstNonBlank of "  hello" is 2
	if pos != 2 {
		t.Errorf("'I' on '  hello': expected cursor at 2 (first non-blank), got %d", pos)
	}
}

func TestVim_h_DoesNotGoPastZero(t *testing.T) {
	m := setupNormal(t, "hello", 0)

	m, _ = m.handleVimKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})

	pos := cursorOf(&m)
	if pos != 0 {
		t.Errorf("'h' at pos 0: expected cursor at 0, got %d", pos)
	}
}

func TestVim_l_DoesNotGoPastLastChar(t *testing.T) {
	m := setupNormal(t, "hello", 4) // last char 'o'

	m, _ = m.handleVimKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})

	pos := cursorOf(&m)
	if pos != 4 {
		t.Errorf("'l' at pos 4 (last char): expected cursor at 4, got %d", pos)
	}
}

func TestVim_l_Moves(t *testing.T) {
	m := setupNormal(t, "hello", 1)

	m, _ = m.handleVimKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})

	pos := cursorOf(&m)
	if pos != 2 {
		t.Errorf("'l' at pos 1: expected cursor at 2, got %d", pos)
	}
}

func TestVim_h_Moves(t *testing.T) {
	m := setupNormal(t, "hello", 3)

	m, _ = m.handleVimKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})

	pos := cursorOf(&m)
	if pos != 2 {
		t.Errorf("'h' at pos 3: expected cursor at 2, got %d", pos)
	}
}

func TestVim_o_SingleLine(t *testing.T) {
	m := setupNormal(t, "hello", 2)

	m, _ = m.handleVimKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})

	if m.vimState.Mode != vim.ModeInsert {
		t.Fatalf("expected Insert mode, got %v", m.vimState.Mode)
	}
	text := m.textarea.Value()
	if text != "hello\n" {
		t.Errorf("after 'o': expected %q, got %q", "hello\n", text)
	}
	pos := cursorOf(&m)
	// Cursor should be on the new empty line (after the \n = position 6)
	if pos != 6 {
		t.Errorf("after 'o': expected cursor at 6, got %d", pos)
	}
}

func TestVim_o_MultiLine(t *testing.T) {
	m := setupNormal(t, "hello\nworld", 2) // cursor on first line

	m, _ = m.handleVimKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})

	text := m.textarea.Value()
	if text != "hello\n\nworld" {
		t.Errorf("after 'o': expected %q, got %q", "hello\n\nworld", text)
	}
	pos := cursorOf(&m)
	// Cursor should be on the new empty line between "hello" and "world" (position 6)
	if pos != 6 {
		t.Errorf("after 'o': expected cursor at 6 (new empty line), got %d", pos)
	}
}

func TestVim_o_EmptyText(t *testing.T) {
	m := setupNormal(t, "", 0)

	m, _ = m.handleVimKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})

	text := m.textarea.Value()
	if text != "\n" {
		t.Errorf("after 'o' on empty: expected %q, got %q", "\n", text)
	}
	pos := cursorOf(&m)
	if pos != 1 {
		t.Errorf("after 'o' on empty: expected cursor at 1, got %d", pos)
	}
}

func TestVim_O_SingleLine(t *testing.T) {
	m := setupNormal(t, "hello", 2)

	m, _ = m.handleVimKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'O'}})

	if m.vimState.Mode != vim.ModeInsert {
		t.Fatalf("expected Insert mode, got %v", m.vimState.Mode)
	}
	text := m.textarea.Value()
	if text != "\nhello" {
		t.Errorf("after 'O': expected %q, got %q", "\nhello", text)
	}
	pos := cursorOf(&m)
	// Cursor should be on the new line at the top (position 0)
	if pos != 0 {
		t.Errorf("after 'O': expected cursor at 0 (new top line), got %d", pos)
	}
}

func TestVim_O_MultiLine(t *testing.T) {
	m := setupNormal(t, "hello\nworld", 8) // cursor on "world"

	m, _ = m.handleVimKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'O'}})

	text := m.textarea.Value()
	if text != "hello\n\nworld" {
		t.Errorf("after 'O': expected %q, got %q", "hello\n\nworld", text)
	}
	pos := cursorOf(&m)
	// Cursor should be on the new empty line (position 6)
	if pos != 6 {
		t.Errorf("after 'O': expected cursor at 6 (new empty line), got %d", pos)
	}
}

func TestVim_CursorLine_And_LineCount(t *testing.T) {
	m := New()
	m.textarea.SetValue("hello\nworld\nfoo")

	if m.LineCount() != 3 {
		t.Errorf("LineCount: expected 3, got %d", m.LineCount())
	}

	// Move cursor to top
	m.setFlatCursor(0)
	if m.CursorLine() != 0 {
		t.Errorf("CursorLine after setFlatCursor(0): expected 0, got %d", m.CursorLine())
	}

	// Move cursor to second line
	m.setFlatCursor(6) // 'w' in "world"
	if m.CursorLine() != 1 {
		t.Errorf("CursorLine after setFlatCursor(6): expected 1, got %d", m.CursorLine())
	}
}
