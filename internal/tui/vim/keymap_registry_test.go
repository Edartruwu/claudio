package vim_test

import (
	"testing"

	"github.com/Abraxas-365/claudio/internal/tui/vim"
)

func TestKeymapRegistry_Register(t *testing.T) {
	r := vim.NewKeymapRegistry()

	called := false
	km := vim.Keymap{
		Key:         'z',
		Mode:        vim.ModeNormal,
		Description: "test key",
		Handler: func(key rune, text string, cursor int, count int, s *vim.State) vim.Action {
			called = true
			return vim.Action{Type: vim.ActionNone}
		},
	}
	r.Register(km)

	found, ok := r.Lookup('z', vim.ModeNormal)
	if !ok {
		t.Fatal("expected Lookup to find registered keymap")
	}
	if found.Description != "test key" {
		t.Errorf("description mismatch: got %q", found.Description)
	}

	// Call the handler to verify it works end-to-end.
	s := vim.New()
	found.Handler('z', "", 0, 1, s)
	if !called {
		t.Error("handler was not called")
	}
}

func TestKeymapRegistry_Lookup_MissingKey(t *testing.T) {
	r := vim.NewKeymapRegistry()

	_, ok := r.Lookup('z', vim.ModeNormal)
	if ok {
		t.Error("expected Lookup to return false for unregistered key")
	}

	// Different mode from a registered key also misses.
	r.Register(vim.Keymap{
		Key:  'q',
		Mode: vim.ModeInsert,
		Handler: func(key rune, text string, cursor int, count int, s *vim.State) vim.Action {
			return vim.Action{Type: vim.ActionNone}
		},
	})
	_, ok = r.Lookup('q', vim.ModeNormal) // wrong mode
	if ok {
		t.Error("expected Lookup to miss when mode does not match")
	}
}

func TestKeymapRegistry_Override(t *testing.T) {
	r := vim.NewKeymapRegistry()

	r.Register(vim.Keymap{
		Key: 'x', Mode: vim.ModeNormal, Description: "first",
		Handler: func(key rune, text string, cursor int, count int, s *vim.State) vim.Action {
			return vim.Action{Type: vim.ActionNone}
		},
	})
	r.Register(vim.Keymap{
		Key: 'x', Mode: vim.ModeNormal, Description: "second",
		Handler: func(key rune, text string, cursor int, count int, s *vim.State) vim.Action {
			return vim.Action{Type: vim.ActionUndo} // different action
		},
	})

	found, ok := r.Lookup('x', vim.ModeNormal)
	if !ok {
		t.Fatal("expected Lookup to find overridden keymap")
	}
	if found.Description != "second" {
		t.Errorf("expected latest registration to win, got description %q", found.Description)
	}

	s := vim.New()
	action := found.Handler('x', "", 0, 1, s)
	if action.Type != vim.ActionUndo {
		t.Errorf("expected ActionUndo from override, got %v", action.Type)
	}
}

func TestKeymapRegistry_All(t *testing.T) {
	r := vim.NewKeymapRegistry()

	for _, key := range []rune{'a', 'b', 'c'} {
		k := key // capture
		r.Register(vim.Keymap{
			Key: k, Mode: vim.ModeNormal,
			Handler: func(key rune, text string, cursor int, count int, s *vim.State) vim.Action {
				return vim.Action{Type: vim.ActionNone}
			},
		})
	}
	r.Register(vim.Keymap{
		Key: 'a', Mode: vim.ModeInsert, // same key, different mode
		Handler: func(key rune, text string, cursor int, count int, s *vim.State) vim.Action {
			return vim.Action{Type: vim.ActionNone}
		},
	})

	all := r.All()
	if len(all) != 4 {
		t.Errorf("expected 4 keymaps, got %d", len(all))
	}
}
