package lua

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Abraxas-365/claudio/internal/bus"
	"github.com/Abraxas-365/claudio/internal/capabilities"
	"github.com/Abraxas-365/claudio/internal/config"
	"github.com/Abraxas-365/claudio/internal/hooks"
	"github.com/Abraxas-365/claudio/internal/services/skills"
	"github.com/Abraxas-365/claudio/internal/tools"
)

// testRuntime creates a Runtime with real lightweight deps for testing.
func testRuntime(t *testing.T) *Runtime {
	t.Helper()
	return New(
		tools.NewRegistry(),
		skills.NewRegistry(),
		bus.New(),
		hooks.NewManager(hooks.HooksConfig{}),
		&config.Settings{},
		nil, // no DB needed for unit tests
		capabilities.New(),
	)
}

// writePlugin creates a temp plugin dir with init.lua content, returns dir.
func writePlugin(t *testing.T, name, luaCode string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "init.lua"), []byte(luaCode), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRegisterToolAndExecute(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "hello", `
claudio.register_tool({
  name        = "hello_world",
  description = "Says hello",
  schema      = '{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}',
  execute     = function(input)
    return { content = "Hello, " .. input.name .. "!" }
  end
})
`)

	if err := rt.LoadPlugin("hello", dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	tool, err := rt.toolReg.Get("hello_world")
	if err != nil {
		t.Fatalf("tool not registered: %v", err)
	}

	if tool.Description() != "Says hello" {
		t.Errorf("description = %q, want %q", tool.Description(), "Says hello")
	}

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"name":"World"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Content != "Hello, World!" {
		t.Errorf("content = %q, want %q", result.Content, "Hello, World!")
	}
	if result.IsError {
		t.Error("unexpected IsError")
	}
}

func TestRegisterToolErrorResult(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "err-tool", `
claudio.register_tool({
  name    = "fail_tool",
  description = "always fails",
  execute = function(input)
    return { content = "oops", is_error = true }
  end
})
`)
	if err := rt.LoadPlugin("err-tool", dir); err != nil {
		t.Fatal(err)
	}

	tool, _ := rt.toolReg.Get("fail_tool")
	result, _ := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if !result.IsError {
		t.Error("expected IsError = true")
	}
	if result.Content != "oops" {
		t.Errorf("content = %q, want %q", result.Content, "oops")
	}
}

func TestRegisterSkill(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "skill-plugin", `
claudio.register_skill({
  name        = "my-skill",
  description = "A test skill",
  content     = "# Hello\nDo stuff.",
})
`)
	if err := rt.LoadPlugin("skill-plugin", dir); err != nil {
		t.Fatal(err)
	}

	skill, ok := rt.skills.Get("my-skill")
	if !ok {
		t.Fatal("skill not registered")
	}
	if skill.Source != "plugin:skill-plugin" {
		t.Errorf("source = %q, want %q", skill.Source, "plugin:skill-plugin")
	}
	if skill.Content != "# Hello\nDo stuff." {
		t.Errorf("content = %q", skill.Content)
	}
}

func TestSubscribeAndPublish(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "bus-plugin", `
received = nil
claudio.subscribe("test.event", function(event)
  received = event.payload.msg
end)
`)
	if err := rt.LoadPlugin("bus-plugin", dir); err != nil {
		t.Fatal(err)
	}

	// Publish event
	payload, _ := json.Marshal(map[string]string{"msg": "hello-bus"})
	rt.bus.Publish(bus.Event{
		Type:    "test.event",
		Payload: payload,
	})

	// Give handler time to run (synchronous in gopher-lua bus, but just in case)
	time.Sleep(10 * time.Millisecond)

	// Verify the global was set in Lua
	plugin := rt.plugins[0]
	plugin.mu.Lock()
	val := plugin.L.GetGlobal("received")
	plugin.mu.Unlock()

	if val.String() != "hello-bus" {
		t.Errorf("received = %q, want %q", val.String(), "hello-bus")
	}
}

func TestPublishFromLua(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	var mu sync.Mutex
	var received string
	rt.bus.Subscribe("plugin.test.custom", func(e bus.Event) {
		mu.Lock()
		defer mu.Unlock()
		var payload map[string]string
		json.Unmarshal(e.Payload, &payload)
		received = payload["key"]
	})

	dir := writePlugin(t, "pub-plugin", `
claudio.publish("plugin.test.custom", { key = "value123" })
`)
	if err := rt.LoadPlugin("pub-plugin", dir); err != nil {
		t.Fatal(err)
	}

	time.Sleep(10 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if received != "value123" {
		t.Errorf("received = %q, want %q", received, "value123")
	}
}

func TestGetSetConfig(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "cfg-plugin", `
claudio.set_config("theme", "dark")
local val = claudio.get_config("theme")
result_theme = val
`)
	if err := rt.LoadPlugin("cfg-plugin", dir); err != nil {
		t.Fatal(err)
	}

	// Check Lua got the right value back
	plugin := rt.plugins[0]
	plugin.mu.Lock()
	val := plugin.L.GetGlobal("result_theme")
	plugin.mu.Unlock()
	if val.String() != "dark" {
		t.Errorf("result_theme = %q, want %q", val.String(), "dark")
	}

	// Check Go side
	cfg := rt.cfg.GetPluginConfig("cfg-plugin")
	if cfg["theme"] != "dark" {
		t.Errorf("config theme = %v, want %q", cfg["theme"], "dark")
	}
}

func TestNotifyDoesNotPanic(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "notify-plugin", `
claudio.notify("test message", "info")
claudio.notify("warn message", "warn")
claudio.log("debug log line")
`)
	if err := rt.LoadPlugin("notify-plugin", dir); err != nil {
		t.Fatal(err)
	}
	// If we get here without panic, test passes
}

func TestSandboxDangerousDisabled(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	for _, fn := range []string{"dofile", "loadfile", "load", "loadstring"} {
		dir := writePlugin(t, "sandbox-"+fn, `
local f = `+fn+`
if f ~= nil then
  error("DANGEROUS: `+fn+` is available")
end
`)
		if err := rt.LoadPlugin("sandbox-"+fn, dir); err != nil {
			t.Errorf("%s should be nil but plugin errored: %v", fn, err)
		}
	}
}

func TestPluginSyntaxErrorCaught(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "bad-syntax", `this is not valid lua!!!`)
	err := rt.LoadPlugin("bad-syntax", dir)
	if err == nil {
		t.Fatal("expected error for bad syntax")
	}
}

func TestPluginRuntimeErrorCaught(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "bad-runtime", `error("intentional error")`)
	err := rt.LoadPlugin("bad-runtime", dir)
	if err == nil {
		t.Fatal("expected error for runtime error")
	}
	if !strings.Contains(err.Error(), "intentional error") {
		t.Errorf("error = %v, want to contain 'intentional error'", err)
	}
}

func TestLoadAllScansDir(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	base := t.TempDir()

	// Create two plugins
	for _, name := range []string{"alpha", "beta"} {
		dir := filepath.Join(base, name)
		os.MkdirAll(dir, 0755)
		os.WriteFile(filepath.Join(dir, "init.lua"), []byte(`
claudio.register_tool({
  name    = "`+name+`_tool",
  description = "`+name+` tool",
  execute = function(input) return { content = "`+name+`" } end
})
`), 0644)
	}

	if err := rt.LoadAll(base); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	for _, name := range []string{"alpha_tool", "beta_tool"} {
		if _, err := rt.toolReg.Get(name); err != nil {
			t.Errorf("tool %s not found: %v", name, err)
		}
	}
}

func TestLoadAllNonexistentDir(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	// Should not error on missing dir
	if err := rt.LoadAll("/nonexistent/path/lua-plugins"); err != nil {
		t.Fatalf("LoadAll should not error on missing dir: %v", err)
	}
}

func TestToolExecuteLuaError(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "err-exec", `
claudio.register_tool({
  name    = "err_exec_tool",
  description = "errors on execute",
  execute = function(input)
    error("boom")
  end
})
`)
	if err := rt.LoadPlugin("err-exec", dir); err != nil {
		t.Fatal(err)
	}

	tool, _ := rt.toolReg.Get("err_exec_tool")
	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute should not return Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError")
	}
	if !strings.Contains(result.Content, "boom") {
		t.Errorf("content = %q, want to contain 'boom'", result.Content)
	}
}

func TestCloseUnsubscribesBus(t *testing.T) {
	rt := testRuntime(t)

	dir := writePlugin(t, "unsub-plugin", `
claudio.subscribe("test.close", function(event)
  -- noop
end)
`)
	if err := rt.LoadPlugin("unsub-plugin", dir); err != nil {
		t.Fatal(err)
	}

	if len(rt.plugins[0].unsubs) != 1 {
		t.Fatalf("expected 1 unsub handle, got %d", len(rt.plugins[0].unsubs))
	}

	rt.Close()

	if len(rt.plugins) != 0 {
		t.Error("expected plugins cleared after Close")
	}
}

// ── defaults / user-init tests ──────────────────────────────────────────────

func TestLoadDefaults_SetsModel(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	if err := rt.LoadDefaults(); err != nil {
		t.Fatalf("LoadDefaults: %v", err)
	}

	if rt.cfg.Model != "claude-sonnet-4-6" {
		t.Errorf("Model = %q, want %q", rt.cfg.Model, "claude-sonnet-4-6")
	}
	if rt.cfg.CompactMode != "strategic" {
		t.Errorf("CompactMode = %q, want %q", rt.cfg.CompactMode, "strategic")
	}
	if !rt.cfg.SessionPersist {
		t.Error("SessionPersist should be true")
	}
}

func TestLoadDefaults_UserCanOverride(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	if err := rt.LoadDefaults(); err != nil {
		t.Fatalf("LoadDefaults: %v", err)
	}

	// Override the model via a subsequent execString (simulates user init)
	override := `claudio.config.set("model", "my-custom-model")`
	if err := rt.execString(override, "test-override"); err != nil {
		t.Fatalf("execString: %v", err)
	}

	if rt.cfg.Model != "my-custom-model" {
		t.Errorf("Model = %q, want %q", rt.cfg.Model, "my-custom-model")
	}
}

func TestLoadUserInit_MissingFileNoError(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	if err := rt.LoadUserInit("/nonexistent/path/init.lua"); err != nil {
		t.Fatalf("LoadUserInit on missing file should return nil, got: %v", err)
	}
}

func TestLoadUserInit_ExecutesFile(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	dir := t.TempDir()
	initPath := filepath.Join(dir, "init.lua")
	if err := os.WriteFile(initPath, []byte(`claudio.config.set("model", "from-user-init")`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := rt.LoadUserInit(initPath); err != nil {
		t.Fatalf("LoadUserInit: %v", err)
	}

	if rt.cfg.Model != "from-user-init" {
		t.Errorf("Model = %q, want %q", rt.cfg.Model, "from-user-init")
	}
}

func TestRegisterHook(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "hook-plugin", `
claudio.register_hook("PostToolUse", "Write", function(ctx)
  hook_called = ctx.tool_name
end)
`)
	if err := rt.LoadPlugin("hook-plugin", dir); err != nil {
		t.Fatal(err)
	}

	// Trigger hook
	rt.hooks.Run(context.Background(), hooks.PostToolUse, hooks.HookContext{
		Event:    hooks.PostToolUse,
		ToolName: "Write",
	})

	// Check Lua global
	plugin := rt.plugins[0]
	plugin.mu.Lock()
	val := plugin.L.GetGlobal("hook_called")
	plugin.mu.Unlock()

	if val.String() != "Write" {
		t.Errorf("hook_called = %q, want %q", val.String(), "Write")
	}
}
