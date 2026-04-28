package lua

import (
	"testing"

	"github.com/Abraxas-365/claudio/internal/tui/windows"
	lua "github.com/yuin/gopher-lua"
)

// newTestRuntime returns a minimal Runtime suitable for unit tests.
func newTestRuntime() *Runtime {
	return &Runtime{}
}

// TestBufNewStoresBuffer verifies claudio.buf.new creates a Buffer userdata.
func TestBufNewStoresBuffer(t *testing.T) {
	rt := newTestRuntime()
	L := newSandboxedState()
	defer L.Close()

	plugin := &loadedPlugin{name: "test", L: L}

	// Inject API into L so claudio global exists.
	claudio := L.NewTable()
	rt.injectWindowsAPI(L, plugin, claudio)
	L.SetGlobal("claudio", claudio)

	// Call claudio.buf.new and check return is LUserData with a *windows.Buffer.
	err := L.DoString(`
		buf = claudio.buf.new({
			name   = "test-buf",
			render = function(w, h) return "content" end,
		})
	`)
	if err != nil {
		t.Fatalf("claudio.buf.new failed: %v", err)
	}

	udVal := L.GetGlobal("buf")
	ud, ok := udVal.(*lua.LUserData)
	if !ok {
		t.Fatalf("expected LUserData, got %T", udVal)
	}
	buf, ok := ud.Value.(*windows.Buffer)
	if !ok {
		t.Fatalf("expected *windows.Buffer inside userdata, got %T", ud.Value)
	}
	if buf.Name != "test-buf" {
		t.Errorf("buffer name: got %q, want %q", buf.Name, "test-buf")
	}
	if buf.Render == nil {
		t.Error("buffer render func should not be nil")
	}
	// Render should call back into Lua and return "content".
	got := buf.Render(80, 24)
	if got != "content" {
		t.Errorf("render result: got %q, want %q", got, "content")
	}
}

// TestRegisterWindowQueuesWhenNoManager verifies register_window queues when manager is unset.
func TestRegisterWindowQueuesWhenNoManager(t *testing.T) {
	rt := newTestRuntime()
	L := newSandboxedState()
	defer L.Close()

	plugin := &loadedPlugin{name: "test", L: L}
	claudio := L.NewTable()
	rt.injectWindowsAPI(L, plugin, claudio)
	L.SetGlobal("claudio", claudio)

	err := L.DoString(`
		local buf = claudio.buf.new({
			name   = "q-panel",
			render = function(w, h) return "queued" end,
		})
		claudio.ui.register_window({
			name   = "QueuedPanel",
			buffer = buf,
			layout = "float",
			title  = "Queued",
		})
	`)
	if err != nil {
		t.Fatalf("register_window failed: %v", err)
	}

	rt.pendingWindowsMu.Lock()
	n := len(rt.pendingWindows)
	rt.pendingWindowsMu.Unlock()

	if n != 1 {
		t.Errorf("expected 1 pending window, got %d", n)
	}
	if rt.pendingWindows[0].Window.Name != "QueuedPanel" {
		t.Errorf("wrong window name: %q", rt.pendingWindows[0].Window.Name)
	}
}

// TestRegisterWindowCallsManagerWhenSet verifies register_window calls Manager.Register when wired.
func TestRegisterWindowCallsManagerWhenSet(t *testing.T) {
	rt := newTestRuntime()
	wm := windows.New()
	rt.SetWindowManager(wm)

	L := newSandboxedState()
	defer L.Close()
	plugin := &loadedPlugin{name: "test", L: L}
	claudio := L.NewTable()
	rt.injectWindowsAPI(L, plugin, claudio)
	L.SetGlobal("claudio", claudio)

	err := L.DoString(`
		local buf = claudio.buf.new({
			name   = "live-buf",
			render = function(w, h) return "live" end,
		})
		claudio.ui.register_window({
			name   = "LivePanel",
			buffer = buf,
			layout = "sidebar",
			title  = "Live",
		})
	`)
	if err != nil {
		t.Fatalf("register_window (live) failed: %v", err)
	}

	w := wm.Get("LivePanel")
	if w == nil {
		t.Fatal("window not registered with manager")
	}
	if w.Layout != windows.LayoutSidebar {
		t.Errorf("wrong layout: got %v, want LayoutSidebar", w.Layout)
	}
	if w.Title != "Live" {
		t.Errorf("wrong title: got %q, want %q", w.Title, "Live")
	}
}

// TestSetWindowManagerFlushesQueue verifies pending windows are flushed on SetWindowManager.
func TestSetWindowManagerFlushesQueue(t *testing.T) {
	rt := newTestRuntime()
	L := newSandboxedState()
	defer L.Close()
	plugin := &loadedPlugin{name: "test", L: L}
	claudio := L.NewTable()
	rt.injectWindowsAPI(L, plugin, claudio)
	L.SetGlobal("claudio", claudio)

	// Register before manager is wired → goes to pending queue.
	err := L.DoString(`
		local buf = claudio.buf.new({
			name   = "flush-buf",
			render = function(w, h) return "" end,
		})
		claudio.ui.register_window({
			name   = "FlushPanel",
			buffer = buf,
			layout = "float",
			title  = "Flush",
		})
	`)
	if err != nil {
		t.Fatalf("pre-manager register_window failed: %v", err)
	}

	// Now wire manager — pending should flush.
	wm := windows.New()
	rt.SetWindowManager(wm)

	if wm.Get("FlushPanel") == nil {
		t.Fatal("FlushPanel not flushed to manager after SetWindowManager")
	}

	// Queue should be cleared.
	rt.pendingWindowsMu.Lock()
	n := len(rt.pendingWindows)
	rt.pendingWindowsMu.Unlock()
	if n != 0 {
		t.Errorf("pending queue not cleared after flush, got %d items", n)
	}
}
