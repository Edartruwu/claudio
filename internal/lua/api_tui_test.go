package lua

import (
	"strings"
	"testing"

	"github.com/Abraxas-365/claudio/internal/tui/styles"
	"github.com/charmbracelet/lipgloss"
	lua "github.com/yuin/gopher-lua"
)

// resetColors restores default Gruvbox palette after each test.
func resetColors(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		styles.Primary = lipgloss.Color("#d3869b")
		styles.Secondary = lipgloss.Color("#83a598")
		styles.Success = lipgloss.Color("#b8bb26")
		styles.Warning = lipgloss.Color("#fabd2f")
		styles.Error = lipgloss.Color("#fb4934")
		styles.Muted = lipgloss.Color("#928374")
		styles.Surface = lipgloss.Color("#282828")
		styles.SurfaceAlt = lipgloss.Color("#3c3836")
		styles.Text = lipgloss.Color("#ebdbb2")
		styles.Dim = lipgloss.Color("#bdae93")
		styles.Subtle = lipgloss.Color("#504945")
		styles.Orange = lipgloss.Color("#fe8019")
		styles.Aqua = lipgloss.Color("#8ec07c")
		styles.RebuildAll()
	})
}

func newTestState(t *testing.T) *lua.LState {
	t.Helper()
	L := lua.NewState()
	t.Cleanup(L.Close)
	claudio := L.NewTable()
	registerTUIAPI(L, claudio)
	L.SetGlobal("claudio", claudio)
	return L
}

// TestSetColor_ValidSlot mutates a single color slot and verifies the package var changed.
func TestSetColor_ValidSlot(t *testing.T) {
	resetColors(t)
	L := newTestState(t)

	err := L.DoString(`claudio.ui.set_color("primary", "#aabbcc")`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if styles.Primary != lipgloss.Color("#aabbcc") {
		t.Errorf("Primary: got %q, want #aabbcc", styles.Primary)
	}
}

// TestSetColor_UnknownSlot returns an error for unknown slots.
func TestSetColor_UnknownSlot(t *testing.T) {
	L := newTestState(t)
	err := L.DoString(`claudio.ui.set_color("nonexistent", "#ffffff")`)
	if err == nil {
		t.Error("expected error for unknown slot")
	}
	if !strings.Contains(err.Error(), "unknown color slot") {
		t.Errorf("error should mention 'unknown color slot', got: %v", err)
	}
}

// TestSetColor_MissingHash returns an error when hex lacks '#'.
func TestSetColor_MissingHash(t *testing.T) {
	L := newTestState(t)
	err := L.DoString(`claudio.ui.set_color("primary", "aabbcc")`)
	if err == nil {
		t.Error("expected error for hex without #")
	}
}

// TestSetTheme_BatchUpdate sets multiple slots at once.
func TestSetTheme_BatchUpdate(t *testing.T) {
	resetColors(t)
	L := newTestState(t)

	err := L.DoString(`
claudio.ui.set_theme({
  primary   = "#111111",
  secondary = "#222222",
  success   = "#333333",
})
`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if styles.Primary != lipgloss.Color("#111111") {
		t.Errorf("primary: got %q", styles.Primary)
	}
	if styles.Secondary != lipgloss.Color("#222222") {
		t.Errorf("secondary: got %q", styles.Secondary)
	}
	if styles.Success != lipgloss.Color("#333333") {
		t.Errorf("success: got %q", styles.Success)
	}
}

// TestSetTheme_UnknownSlotErrors reports unknown slots.
func TestSetTheme_UnknownSlotErrors(t *testing.T) {
	L := newTestState(t)
	err := L.DoString(`claudio.ui.set_theme({ doesnotexist = "#ffffff" })`)
	if err == nil {
		t.Error("expected error for unknown slot in set_theme")
	}
}

// TestSetTheme_TokyoNight smoke-tests a real community theme.
func TestSetTheme_TokyoNight(t *testing.T) {
	resetColors(t)
	L := newTestState(t)

	err := L.DoString(`
claudio.ui.set_theme({
  primary     = "#7aa2f7",
  secondary   = "#9ece6a",
  success     = "#9ece6a",
  warning     = "#e0af68",
  error       = "#f7768e",
  muted       = "#565f89",
  surface     = "#1a1b26",
  surface_alt = "#24283b",
  text        = "#c0caf5",
  dim         = "#9aa5ce",
  subtle      = "#414868",
  orange      = "#ff9e64",
  aqua        = "#73daca",
})
`)
	if err != nil {
		t.Fatalf("tokyonight theme failed: %v", err)
	}
	if styles.Primary != lipgloss.Color("#7aa2f7") {
		t.Errorf("primary: got %q", styles.Primary)
	}
	if styles.Surface != lipgloss.Color("#1a1b26") {
		t.Errorf("surface: got %q", styles.Surface)
	}
}

// TestSetBorder_ValidStyles verifies all valid border names are accepted.
func TestSetBorder_ValidStyles(t *testing.T) {
	L := newTestState(t)
	for _, name := range []string{"rounded", "block", "double", "normal", "hidden"} {
		err := L.DoString(`claudio.ui.set_border("` + name + `")`)
		if err != nil {
			t.Errorf("set_border(%q) failed: %v", name, err)
		}
	}
}

// TestSetBorder_InvalidName returns an error for unknown border names.
func TestSetBorder_InvalidName(t *testing.T) {
	L := newTestState(t)
	err := L.DoString(`claudio.ui.set_border("neon-glow")`)
	if err == nil {
		t.Error("expected error for unknown border style")
	}
	if !strings.Contains(err.Error(), "unknown border style") {
		t.Errorf("error should mention 'unknown border style', got: %v", err)
	}
}

// TestGetColors_ReturnsTable verifies get_colors() returns a table with all slots.
func TestGetColors_ReturnsTable(t *testing.T) {
	L := newTestState(t)
	err := L.DoString(`
local c = claudio.ui.get_colors()
assert(type(c) == "table", "expected table")
assert(c.primary ~= nil, "primary missing")
assert(c.secondary ~= nil, "secondary missing")
assert(c.success ~= nil, "success missing")
assert(c.warning ~= nil, "warning missing")
assert(c.error ~= nil, "error missing")
assert(c.surface ~= nil, "surface missing")
assert(c.text ~= nil, "text missing")
`)
	if err != nil {
		t.Fatalf("get_colors assertion failed: %v", err)
	}
}

// TestGetColors_ReflectsChanges verifies get_colors() returns updated values after set_color.
func TestGetColors_ReflectsChanges(t *testing.T) {
	resetColors(t)
	L := newTestState(t)
	err := L.DoString(`
claudio.ui.set_color("primary", "#deadbe")
local c = claudio.ui.get_colors()
assert(c.primary == "#deadbe", "expected #deadbe, got " .. tostring(c.primary))
`)
	if err != nil {
		t.Fatalf("get_colors reflect test failed: %v", err)
	}
}

// TestUIAPI_RegisterSidebarBlock_Stored verifies that register_sidebar_block
// stores the block definition in the runtime and it is retrievable via GetSidebarBlocks.
func TestUIAPI_RegisterSidebarBlock_Stored(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "sidebar-block", `
claudio.ui.register_sidebar_block({
  id     = "my-block",
  title  = "My Plugin",
  render = function(ctx) return "hello from plugin" end,
})
`)
	if err := rt.LoadPlugin("sidebar-block", dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	blocks := rt.GetSidebarBlocks()
	if len(blocks) != 1 {
		t.Fatalf("GetSidebarBlocks len = %d, want 1", len(blocks))
	}

	b := blocks[0]
	if b.ID != "my-block" {
		t.Errorf("ID = %q, want %q", b.ID, "my-block")
	}
	if b.Title != "My Plugin" {
		t.Errorf("Title = %q, want %q", b.Title, "My Plugin")
	}
	if b.RenderFn == nil {
		t.Error("RenderFn is nil, want non-nil")
	}
	if b.Plugin == nil {
		t.Error("Plugin is nil, want non-nil")
	}
}

// TestUIAPI_RegisterSidebarBlock_MissingID verifies that missing id raises an error.
func TestUIAPI_RegisterSidebarBlock_MissingID(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "sidebar-no-id", `
claudio.ui.register_sidebar_block({
  title  = "No ID",
  render = function(ctx) return "" end,
})
`)
	err := rt.LoadPlugin("sidebar-no-id", dir)
	if err == nil {
		t.Fatal("expected error for missing id, got nil")
	}
}

// TestUIAPI_RegisterSidebarBlock_MissingRender verifies that missing render fn raises an error.
func TestUIAPI_RegisterSidebarBlock_MissingRender(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "sidebar-no-render", `
claudio.ui.register_sidebar_block({
  id    = "block",
  title = "No Render",
})
`)
	err := rt.LoadPlugin("sidebar-no-render", dir)
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
	resetColors(t)
	L := newTestState(t)
	err := L.DoString(`claudio.ui.set_color("surface_alt", "#101010")`)
	if err != nil {
		t.Fatalf("surface_alt: %v", err)
	}
	if styles.SurfaceAlt != lipgloss.Color("#101010") {
		t.Errorf("SurfaceAlt: got %q", styles.SurfaceAlt)
	}
}
