package sidebar

// registry_test.go — unit tests for BlockRegistry.

import (
	"testing"
)

// stubBlock is a minimal Block for testing.
type stubBlock struct {
	title     string
	weight    int
	minHeight int
}

func (s *stubBlock) Title() string              { return s.title }
func (s *stubBlock) Weight() int                { return s.weight }
func (s *stubBlock) MinHeight() int             { return s.minHeight }
func (s *stubBlock) Render(w, h int) string     { return s.title }

// TestBlockRegistry_Empty verifies Blocks() on empty registry returns empty slice.
func TestBlockRegistry_Empty(t *testing.T) {
	reg := NewBlockRegistry()
	if blocks := reg.Blocks(); len(blocks) != 0 {
		t.Errorf("empty registry Blocks() len = %d, want 0", len(blocks))
	}
}

// TestBlockRegistry_Register_Single verifies a single block is stored and retrievable.
func TestBlockRegistry_Register_Single(t *testing.T) {
	reg := NewBlockRegistry()
	reg.Register(&stubBlock{title: "Alpha", weight: 1})

	blocks := reg.Blocks()
	if len(blocks) != 1 {
		t.Fatalf("Blocks() len = %d, want 1", len(blocks))
	}
	if blocks[0].Title() != "Alpha" {
		t.Errorf("Title = %q, want Alpha", blocks[0].Title())
	}
}

// TestBlockRegistry_Register_Multiple verifies multiple blocks are all stored.
func TestBlockRegistry_Register_Multiple(t *testing.T) {
	reg := NewBlockRegistry()
	reg.Register(&stubBlock{title: "A", weight: 1})
	reg.Register(&stubBlock{title: "B", weight: 2})
	reg.Register(&stubBlock{title: "C", weight: 3})

	if len(reg.Blocks()) != 3 {
		t.Errorf("Blocks() len = %d, want 3", len(reg.Blocks()))
	}
}

// TestBlockRegistry_SortedByWeightDescending verifies Blocks() returns blocks
// in descending weight order.
func TestBlockRegistry_SortedByWeightDescending(t *testing.T) {
	reg := NewBlockRegistry()
	reg.Register(&stubBlock{title: "Low", weight: 1})
	reg.Register(&stubBlock{title: "High", weight: 10})
	reg.Register(&stubBlock{title: "Mid", weight: 5})

	blocks := reg.Blocks()
	if len(blocks) != 3 {
		t.Fatalf("Blocks() len = %d, want 3", len(blocks))
	}
	if blocks[0].Title() != "High" {
		t.Errorf("blocks[0] = %q, want High (highest weight first)", blocks[0].Title())
	}
	if blocks[1].Title() != "Mid" {
		t.Errorf("blocks[1] = %q, want Mid", blocks[1].Title())
	}
	if blocks[2].Title() != "Low" {
		t.Errorf("blocks[2] = %q, want Low", blocks[2].Title())
	}
}

// TestBlockRegistry_Blocks_ReturnsCopy verifies that mutating the returned slice
// does not affect the registry.
func TestBlockRegistry_Blocks_ReturnsCopy(t *testing.T) {
	reg := NewBlockRegistry()
	reg.Register(&stubBlock{title: "Original", weight: 1})

	snap := reg.Blocks()
	snap[0] = &stubBlock{title: "Mutated", weight: 99}

	after := reg.Blocks()
	if after[0].Title() != "Original" {
		t.Errorf("registry block mutated via snapshot — got %q, want Original", after[0].Title())
	}
}

// TestLuaBlock_Interface verifies LuaBlock satisfies Block and its accessors return correct values.
func TestLuaBlock_Interface(t *testing.T) {
	called := false
	b := NewLuaBlock("my-id", "My Title", 7, 3, func(w, h int) string {
		called = true
		return "rendered"
	})

	if b.Title() != "My Title" {
		t.Errorf("Title() = %q, want My Title", b.Title())
	}
	if b.Weight() != 7 {
		t.Errorf("Weight() = %d, want 7", b.Weight())
	}
	if b.MinHeight() != 3 {
		t.Errorf("MinHeight() = %d, want 3", b.MinHeight())
	}
	out := b.Render(80, 10)
	if out != "rendered" {
		t.Errorf("Render() = %q, want rendered", out)
	}
	if !called {
		t.Error("Render() did not invoke renderFn")
	}
}
