package windows

import (
	"strings"
	"testing"
)

func newTestBuffer(name, content string) *Buffer {
	return &Buffer{
		Name:   name,
		Render: func(w, h int) string { return content },
	}
}

func newTestWindow(name string, layout Layout) *Window {
	return &Window{
		Name:   name,
		Title:  name,
		Buffer: newTestBuffer(name, "content:"+name),
		Layout: layout,
	}
}

func TestRegisterAndGet(t *testing.T) {
	m := New()
	w := newTestWindow("a", LayoutFloat)
	m.Register(w)
	if got := m.Get("a"); got != w {
		t.Fatal("Get returned wrong window")
	}
	if m.Get("nope") != nil {
		t.Fatal("Get should return nil for unknown")
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	m := New()
	m.Register(newTestWindow("dup", LayoutFloat))
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate register")
		}
	}()
	m.Register(newTestWindow("dup", LayoutFloat))
}

func TestOpenCloseToggle(t *testing.T) {
	m := New()
	w := newTestWindow("win", LayoutFloat)
	m.Register(w)

	// Open unknown → error.
	if err := m.Open("nope"); err == nil {
		t.Fatal("expected error for unknown window")
	}

	// Open.
	if err := m.Open("win"); err != nil {
		t.Fatal(err)
	}
	if !w.IsOpen() {
		t.Fatal("window should be open")
	}

	// Close.
	m.Close("win")
	if w.IsOpen() {
		t.Fatal("window should be closed")
	}

	// Toggle open.
	m.Toggle("win")
	if !w.IsOpen() {
		t.Fatal("toggle should open")
	}

	// Toggle close.
	m.Toggle("win")
	if w.IsOpen() {
		t.Fatal("toggle should close")
	}

	// Toggle unknown — no panic.
	m.Toggle("nope")
}

func TestFocusedFloat(t *testing.T) {
	m := New()
	a := newTestWindow("a", LayoutFloat)
	b := newTestWindow("b", LayoutFloat)
	s := newTestWindow("s", LayoutSidebar)
	m.Register(a)
	m.Register(b)
	m.Register(s)

	// No open → nil.
	if m.FocusedFloat() != nil {
		t.Fatal("no open windows, should be nil")
	}

	// Open sidebar → still nil (not float).
	_ = m.Open("s")
	if m.FocusedFloat() != nil {
		t.Fatal("sidebar not a float")
	}

	// Open a → focused.
	_ = m.Open("a")
	if f := m.FocusedFloat(); f != a {
		t.Fatalf("expected a, got %v", f)
	}

	// Open b → b on top.
	_ = m.Open("b")
	if f := m.FocusedFloat(); f != b {
		t.Fatalf("expected b, got %v", f)
	}
	if !b.Focused {
		t.Fatal("b should be focused")
	}
	if a.Focused {
		t.Fatal("a should not be focused")
	}

	// Close b → a focused again.
	m.Close("b")
	if f := m.FocusedFloat(); f != a {
		t.Fatal("a should be focused after closing b")
	}
}

func TestOpenFloatsOrder(t *testing.T) {
	m := New()
	a := newTestWindow("a", LayoutFloat)
	a.ZIndex = 1
	b := newTestWindow("b", LayoutFloat)
	b.ZIndex = 2
	c := newTestWindow("c", LayoutSidebar)
	m.Register(a)
	m.Register(b)
	m.Register(c)

	_ = m.Open("a")
	_ = m.Open("b")
	_ = m.Open("c")

	floats := m.OpenFloats()
	if len(floats) != 2 {
		t.Fatalf("expected 2 floats, got %d", len(floats))
	}
	if floats[0] != a || floats[1] != b {
		t.Fatal("float order wrong")
	}
}

func TestSortByZIndex(t *testing.T) {
	m := New()
	a := newTestWindow("a", LayoutFloat)
	a.ZIndex = 10
	b := newTestWindow("b", LayoutFloat)
	b.ZIndex = 1

	m.Register(a)
	m.Register(b)
	_ = m.Open("a")
	_ = m.Open("b")

	// b opened after a → b is on top in stack. Sort by ZIndex.
	m.SortByZIndex()
	floats := m.OpenFloats()
	if floats[0] != b || floats[1] != a {
		t.Fatal("sort by z-index failed")
	}
	// a has higher z → should be focused.
	if !a.Focused {
		t.Fatal("a (higher z) should be focused after sort")
	}
}

func TestRenderOverlayNoFloats(t *testing.T) {
	m := New()
	base := "hello\nworld"
	got := m.RenderOverlay(base, 80, 24)
	if got != base {
		t.Fatal("no floats → base should pass through")
	}
}

func TestRenderOverlayWithFloat(t *testing.T) {
	m := New()
	w := newTestWindow("popup", LayoutFloat)
	w.Width = 30
	w.Height = 10
	m.Register(w)
	_ = m.Open("popup")

	result := m.RenderOverlay("base", 80, 24)
	if !strings.Contains(result, "content:popup") {
		t.Fatal("overlay should contain buffer content")
	}
}

func TestWindowViewNilBuffer(t *testing.T) {
	w := &Window{Name: "empty"}
	if w.View(10, 10) != "" {
		t.Fatal("nil buffer should return empty")
	}
	w.Buffer = &Buffer{Name: "norender"}
	if w.View(10, 10) != "" {
		t.Fatal("nil render func should return empty")
	}
}

func TestOpenAlreadyOpen(t *testing.T) {
	m := New()
	a := newTestWindow("a", LayoutFloat)
	b := newTestWindow("b", LayoutFloat)
	m.Register(a)
	m.Register(b)

	_ = m.Open("a")
	_ = m.Open("b")
	// Re-open a → should move to top.
	_ = m.Open("a")

	if f := m.FocusedFloat(); f != a {
		t.Fatal("re-opened a should be on top")
	}
	// Stack should have b, a (no duplicate).
	floats := m.OpenFloats()
	if len(floats) != 2 {
		t.Fatalf("expected 2 floats, got %d", len(floats))
	}
}

func TestCloseUnknown(t *testing.T) {
	m := New()
	// No panic.
	m.Close("nope")
}
