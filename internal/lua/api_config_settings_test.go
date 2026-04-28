package lua

import (
	"testing"

	"github.com/Abraxas-365/claudio/internal/bus"
	"github.com/Abraxas-365/claudio/internal/capabilities"
	"github.com/Abraxas-365/claudio/internal/config"
	"github.com/Abraxas-365/claudio/internal/hooks"
	"github.com/Abraxas-365/claudio/internal/services/skills"
	"github.com/Abraxas-365/claudio/internal/tools"
	lua "github.com/yuin/gopher-lua"
)

// testRuntimeWithSettings creates a Runtime with a pre-populated Settings.
func testRuntimeWithSettings(t *testing.T, s *config.Settings) *Runtime {
	t.Helper()
	return New(
		tools.NewRegistry(),
		skills.NewRegistry(),
		bus.New(),
		hooks.NewManager(hooks.HooksConfig{}),
		s,
		nil,
		capabilities.New(),
	)
}

// execConfig runs Lua code that can access claudio.config.* and returns the LState.
// The plugin is loaded as "test-plugin"; caller inspects globals.
func execConfig(t *testing.T, rt *Runtime, code string) *loadedPlugin {
	t.Helper()
	dir := writePlugin(t, "test-plugin", code)
	if err := rt.LoadPlugin("test-plugin", dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}
	return rt.plugins[len(rt.plugins)-1]
}

// globalStr returns a string global from the plugin's LState.
func globalStr(t *testing.T, p *loadedPlugin, name string) string {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.L.GetGlobal(name).String()
}

// globalLVal returns the raw LValue of a global.
func globalLVal(t *testing.T, p *loadedPlugin, name string) lua.LValue {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.L.GetGlobal(name)
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestConfigAPI_Get_String(t *testing.T) {
	s := &config.Settings{Model: "claude-sonnet-4-6"}
	rt := testRuntimeWithSettings(t, s)
	defer rt.Close()

	p := execConfig(t, rt, `result = claudio.config.get("model")`)

	got := globalStr(t, p, "result")
	if got != "claude-sonnet-4-6" {
		t.Errorf("get(model) = %q, want %q", got, "claude-sonnet-4-6")
	}
}

func TestConfigAPI_Get_Int(t *testing.T) {
	s := &config.Settings{CompactKeepN: 7}
	rt := testRuntimeWithSettings(t, s)
	defer rt.Close()

	p := execConfig(t, rt, `result = claudio.config.get("compactKeepN")`)

	v := globalLVal(t, p, "result")
	n, ok := v.(lua.LNumber)
	if !ok {
		t.Fatalf("expected LNumber, got %T (%v)", v, v)
	}
	if int(n) != 7 {
		t.Errorf("get(compactKeepN) = %d, want 7", int(n))
	}
}

func TestConfigAPI_Get_Bool(t *testing.T) {
	s := &config.Settings{AutoCompact: false}
	rt := testRuntimeWithSettings(t, s)
	defer rt.Close()

	p := execConfig(t, rt, `result = claudio.config.get("autoCompact")`)

	v := globalLVal(t, p, "result")
	b, ok := v.(lua.LBool)
	if !ok {
		t.Fatalf("expected LBool, got %T (%v)", v, v)
	}
	if bool(b) != false {
		t.Errorf("get(autoCompact) = %v, want false", bool(b))
	}
}

func TestConfigAPI_Set_String(t *testing.T) {
	s := &config.Settings{Model: "claude-haiku"}
	rt := testRuntimeWithSettings(t, s)
	defer rt.Close()

	execConfig(t, rt, `claudio.config.set("model", "claude-opus-4-6")`)

	if s.Model != "claude-opus-4-6" {
		t.Errorf("Settings.Model = %q, want %q", s.Model, "claude-opus-4-6")
	}
}

func TestConfigAPI_Set_Int(t *testing.T) {
	s := &config.Settings{CompactKeepN: 5}
	rt := testRuntimeWithSettings(t, s)
	defer rt.Close()

	execConfig(t, rt, `claudio.config.set("compactKeepN", 10)`)

	if s.CompactKeepN != 10 {
		t.Errorf("Settings.CompactKeepN = %d, want 10", s.CompactKeepN)
	}
}

func TestConfigAPI_Set_Bool(t *testing.T) {
	b := false
	s := &config.Settings{AutoMemoryExtract: &b}
	rt := testRuntimeWithSettings(t, s)
	defer rt.Close()

	execConfig(t, rt, `claudio.config.set("autoMemoryExtract", true)`)

	if s.AutoMemoryExtract == nil {
		t.Fatal("AutoMemoryExtract is nil after set")
	}
	if !*s.AutoMemoryExtract {
		t.Errorf("*Settings.AutoMemoryExtract = false, want true")
	}
}

func TestConfigAPI_OnChange(t *testing.T) {
	s := &config.Settings{Model: "old-model"}
	rt := testRuntimeWithSettings(t, s)
	defer rt.Close()

	// Register handler then immediately set
	p := execConfig(t, rt, `
captured_new = nil
captured_old = nil
claudio.config.on_change("model", function(new_val, old_val)
  captured_new = new_val
  captured_old = old_val
end)
claudio.config.set("model", "new-model")
`)

	if got := globalStr(t, p, "captured_new"); got != "new-model" {
		t.Errorf("captured_new = %q, want %q", got, "new-model")
	}
	if got := globalStr(t, p, "captured_old"); got != "old-model" {
		t.Errorf("captured_old = %q, want %q", got, "old-model")
	}
}

func TestConfigAPI_UnknownKey(t *testing.T) {
	s := &config.Settings{}
	rt := testRuntimeWithSettings(t, s)
	defer rt.Close()

	// get of unknown key → nil, no error
	p := execConfig(t, rt, `result = claudio.config.get("totally_unknown_key_xyz")`)
	v := globalLVal(t, p, "result")
	if v.Type() != lua.LTNil {
		t.Errorf("get(unknown) = %v (%T), want nil", v, v)
	}

	// set of unknown key → raises Lua error caught by LoadPlugin
	s2 := &config.Settings{}
	rt2 := testRuntimeWithSettings(t, s2)
	defer rt2.Close()
	dir := writePlugin(t, "bad-key", `claudio.config.set("no_such_key", "x")`)
	err := rt2.LoadPlugin("bad-key", dir)
	if err == nil {
		t.Error("set(unknown key) should error, got nil")
	}
}
