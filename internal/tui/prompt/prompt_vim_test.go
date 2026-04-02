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
