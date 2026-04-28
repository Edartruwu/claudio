package lua

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Abraxas-365/claudio/internal/bus"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
	"github.com/charmbracelet/lipgloss"
)

// resetColors saves and restores color state around tests that mutate styles package vars.
func resetColors(t *testing.T) {
	t.Helper()
	orig := styles.Primary
	t.Cleanup(func() { styles.Primary = orig; styles.RebuildAll() })
}

// newTestState returns a Runtime whose plugins dir has a no-op init.lua loaded.
// Use it when tests need to call Lua code directly via rt.ExecString.
func newTestState(t *testing.T) *Runtime {
	t.Helper()
	rt := testRuntime(t)
	t.Cleanup(func() { rt.Close() })
	dir := writePlugin(t, "tui_state_test", `-- no-op`)
	if err := rt.LoadPlugin("tui_state_test", dir); err != nil {
		t.Fatalf("newTestState LoadPlugin: %v", err)
	}
	return rt
}

// TestUIAPI_SetStatusline_Stored verifies that claudio.ui.set_statusline stores the function.
func TestUIAPI_SetStatusline_Stored(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "statusline_plugin", `
claudio.ui.set_statusline(function(ctx)
  return ctx.mode .. "|" .. ctx.model
end)
`)
	if err := rt.LoadPlugin("statusline_plugin", dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	rt.uiMu.RLock()
	fn := rt.StatuslineFn
	p := rt.statuslinePlugin
	rt.uiMu.RUnlock()

	if fn == nil {
		t.Fatal("StatuslineFn not stored after set_statusline call")
	}
	if p == nil {
		t.Fatal("statuslinePlugin not stored after set_statusline call")
	}
}

// TestUIAPI_RenderStatusline verifies that the Lua fn is called and returns the right string.
func TestUIAPI_RenderStatusline(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "sl_render", `
claudio.ui.set_statusline(function(ctx)
  return ctx.mode .. "|" .. ctx.model .. "|tokens:" .. ctx.tokens
end)
`)
	if err := rt.LoadPlugin("sl_render", dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	ctx := StatuslineCtx{
		Mode:    "normal",
		Model:   "claude-test",
		Tokens:  42,
		Session: "my-session",
	}
	got := rt.RenderStatusline(ctx)
	want := "normal|claude-test|tokens:42"
	if got != want {
		t.Errorf("RenderStatusline = %q; want %q", got, want)
	}
}

// TestUIAPI_Popup_PublishesBusEvent verifies that claudio.ui.popup publishes the "ui.popup" event.
func TestUIAPI_Popup_PublishesBusEvent(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	type popupPayload struct {
		Title   string `json:"title"`
		Content string `json:"content"`
		Width   int    `json:"width"`
		Height  int    `json:"height"`
	}

	var (
		mu      sync.Mutex
		received *popupPayload
	)

	rt.bus.Subscribe("ui.popup", func(event bus.Event) {
		var p popupPayload
		if err := json.Unmarshal(event.Payload, &p); err != nil {
			return
		}
		mu.Lock()
		received = &p
		mu.Unlock()
	})

	dir := writePlugin(t, "popup_plugin", `
claudio.ui.popup({
  title   = "Test Popup",
  content = "Hello from plugin!",
  width   = 80,
  height  = 12,
})
`)
	if err := rt.LoadPlugin("popup_plugin", dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	// Bus subscribers are called synchronously in Publish; no sleep needed.
	// But give a brief window in case of any async buffering.
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		mu.Lock()
		r := received
		mu.Unlock()
		if r != nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	mu.Lock()
	r := received
	mu.Unlock()

	if r == nil {
		t.Fatal("ui.popup bus event not received")
	}
	if r.Title != "Test Popup" {
		t.Errorf("popup title = %q; want %q", r.Title, "Test Popup")
	}
	if !strings.Contains(r.Content, "Hello from plugin!") {
		t.Errorf("popup content = %q; want to contain 'Hello from plugin!'", r.Content)
	}
	if r.Width != 80 {
		t.Errorf("popup width = %d; want 80", r.Width)
	}
	if r.Height != 12 {
		t.Errorf("popup height = %d; want 12", r.Height)
	}
}

// TestUIAPI_RegisterPaletteEntry_Stored verifies pending palette entries are stored.
func TestUIAPI_RegisterPaletteEntry_Stored(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "palette_plugin", `
claudio.ui.register_palette_entry({
  name   = "Reload Plugins",
  action = "reload_plugins",
})
claudio.ui.register_palette_entry({
  name        = "Open Debug Panel",
  action      = "open_debug",
  description = "Opens the plugin debug panel",
})
`)
	if err := rt.LoadPlugin("palette_plugin", dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	entries := rt.PendingPaletteEntries()
	if len(entries) != 2 {
		t.Fatalf("PendingPaletteEntries count = %d; want 2", len(entries))
	}
	if entries[0].Name != "Reload Plugins" {
		t.Errorf("entry[0].Name = %q; want 'Reload Plugins'", entries[0].Name)
	}
	if entries[0].Action != "reload_plugins" {
		t.Errorf("entry[0].Action = %q; want 'reload_plugins'", entries[0].Action)
	}
	if entries[1].Name != "Open Debug Panel" {
		t.Errorf("entry[1].Name = %q; want 'Open Debug Panel'", entries[1].Name)
	}
	if entries[1].Description != "Opens the plugin debug panel" {
		t.Errorf("entry[1].Description = %q; want description set", entries[1].Description)
	}
}

// TestWinAPI_NewPanel_Stored verifies that claudio.win.new_panel creates a panel
// and add_section populates it correctly.
func TestWinAPI_NewPanel_Stored(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "panel-block", `
local p = claudio.win.new_panel({ position = "left", width = 25 })
p:add_section({
  id     = "my-section",
  title  = "My Plugin",
  weight = 2,
  min_height = 4,
  render = function(w, h) return "hello from plugin" end,
})
`)
	if err := rt.LoadPlugin("panel-block", dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	rt.pendingPanelsMu.Lock()
	panels := rt.pendingPanels
	rt.pendingPanelsMu.Unlock()

	if len(panels) != 1 {
		t.Fatalf("pending panels len = %d, want 1", len(panels))
	}

	p := panels[0]
	if p.Position != "left" {
		t.Errorf("Position = %q, want %q", p.Position, "left")
	}
	if p.Width != 25 {
		t.Errorf("Width = %d, want 25", p.Width)
	}

	p.Mu.Lock()
	nsec := len(p.Sections)
	p.Mu.Unlock()
	if nsec != 1 {
		t.Fatalf("Sections len = %d, want 1", nsec)
	}

	sec := p.Sections[0]
	if sec.ID != "my-section" {
		t.Errorf("Section ID = %q, want %q", sec.ID, "my-section")
	}
	if sec.Title != "My Plugin" {
		t.Errorf("Section Title = %q, want %q", sec.Title, "My Plugin")
	}
	if sec.Weight != 2 {
		t.Errorf("Section Weight = %d, want 2", sec.Weight)
	}
	if sec.MinHeight != 4 {
		t.Errorf("Section MinHeight = %d, want 4", sec.MinHeight)
	}

	got := sec.CallRender(40, 10)
	if got != "hello from plugin" {
		t.Errorf("CallRender = %q, want %q", got, "hello from plugin")
	}
}

// TestWinAPI_NewPanel_MissingSectionID verifies that missing section id raises an error.
func TestWinAPI_NewPanel_MissingSectionID(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "panel-no-id", `
local p = claudio.win.new_panel({ position = "left" })
p:add_section({
  title  = "No ID",
  render = function(w, h) return "" end,
})
`)
	err := rt.LoadPlugin("panel-no-id", dir)
	if err == nil {
		t.Fatal("expected error for missing section id, got nil")
	}
}

// TestWinAPI_NewPanel_MissingSectionRender verifies that missing render fn raises an error.
func TestWinAPI_NewPanel_MissingSectionRender(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "panel-no-render", `
local p = claudio.win.new_panel({ position = "left" })
p:add_section({
  id    = "block",
  title = "No Render",
})
`)
	err := rt.LoadPlugin("panel-no-render", dir)
	if err == nil {
		t.Fatal("expected error for missing render, got nil")
	}
}

// TestRebuildAll_DoesNotPanic ensures RebuildAll can be called without panicking.
func TestRebuildAll_DoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("RebuildAll panicked: %v", r)
		}
	}()
	styles.RebuildAll()
}

// TestSurfaceAltSlot verifies surface_alt (underscore) slot works.
func TestSurfaceAltSlot(t *testing.T) {
	origSurfaceAlt := styles.SurfaceAlt
	t.Cleanup(func() { styles.SurfaceAlt = origSurfaceAlt; styles.RebuildAll() })
	rt := newTestState(t)
	_, err := rt.ExecString(`claudio.ui.set_color("surface_alt", "#101010")`)
	if err != nil {
		t.Fatalf("surface_alt: %v", err)
	}
	if styles.SurfaceAlt != lipgloss.Color("#101010") {
		t.Errorf("SurfaceAlt: got %q", styles.SurfaceAlt)
	}
}
