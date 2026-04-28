package lua

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Abraxas-365/claudio/internal/bus"
)

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

// TestUIAPI_RegisterWhichkey_Stored verifies pending whichkey groups are stored.
func TestUIAPI_RegisterWhichkey_Stored(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "wk_plugin", `
claudio.ui.register_whichkey("Plugin", {
  { key = "p", desc = "Open plugin panel" },
  { key = "r", desc = "Reload plugin" },
})
`)
	if err := rt.LoadPlugin("wk_plugin", dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	groups := rt.PendingWhichkeyGroups()
	if len(groups) != 1 {
		t.Fatalf("PendingWhichkeyGroups count = %d; want 1", len(groups))
	}
	g := groups[0]
	if g.Group != "Plugin" {
		t.Errorf("group name = %q; want 'Plugin'", g.Group)
	}
	if len(g.Bindings) != 2 {
		t.Fatalf("bindings count = %d; want 2", len(g.Bindings))
	}
	if g.Bindings[0].Key != "p" || g.Bindings[0].Desc != "Open plugin panel" {
		t.Errorf("binding[0] = {%q,%q}; want {p, Open plugin panel}", g.Bindings[0].Key, g.Bindings[0].Desc)
	}
	if g.Bindings[1].Key != "r" || g.Bindings[1].Desc != "Reload plugin" {
		t.Errorf("binding[1] = {%q,%q}; want {r, Reload plugin}", g.Bindings[1].Key, g.Bindings[1].Desc)
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
