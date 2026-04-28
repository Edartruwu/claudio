package lua

import (
	"testing"

	"github.com/Abraxas-365/claudio/internal/cli/commands"
	"github.com/Abraxas-365/claudio/internal/tui/vim"
)

// TestCommandAPI_Register verifies that register_command stores the command
// in the pending list when no registry is wired.
func TestCommandAPI_Register(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "cmd-reg", `
claudio.register_command({
  name        = "hello",
  description = "Say hello",
  execute     = function(args) return "hi " .. args end,
})
`)
	if err := rt.LoadPlugin("cmd-reg", dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	rt.mu.Lock()
	n := len(rt.pendingCommands)
	rt.mu.Unlock()

	if n != 1 {
		t.Fatalf("pendingCommands len = %d, want 1", n)
	}
	if rt.pendingCommands[0].cmd.Name != "hello" {
		t.Errorf("pending command name = %q, want %q", rt.pendingCommands[0].cmd.Name, "hello")
	}
}

// TestCommandAPI_Execute_Pending verifies that SetCommandRegistry flushes
// pending commands and they are then callable.
func TestCommandAPI_Execute_Pending(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "cmd-flush", `
claudio.register_command({
  name        = "greet",
  description = "Greet",
  aliases     = { "gr" },
  execute     = function(args) return "hello " .. args end,
})
`)
	if err := rt.LoadPlugin("cmd-flush", dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	reg := commands.NewRegistry()
	rt.SetCommandRegistry(reg)

	// Pending commands should be flushed.
	rt.mu.Lock()
	pending := len(rt.pendingCommands)
	rt.mu.Unlock()
	if pending != 0 {
		t.Errorf("pendingCommands not flushed: len = %d", pending)
	}

	// Command should be in registry.
	cmd, ok := reg.Get("greet")
	if !ok {
		t.Fatal("command 'greet' not found in registry")
	}

	result, err := cmd.Execute("world")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result != "hello world" {
		t.Errorf("result = %q, want %q", result, "hello world")
	}

	// Alias should also work.
	if _, ok := reg.Get("gr"); !ok {
		t.Error("alias 'gr' not registered")
	}
}

// TestCommandAPI_Cmd verifies claudio.cmd() dispatches to the registry.
func TestCommandAPI_Cmd(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	// Wire a command registry with a test command.
	reg := commands.NewRegistry()
	reg.Register(&commands.Command{
		Name:        "echo",
		Description: "Echo args",
		Execute:     func(args string) (string, error) { return "echo:" + args, nil },
	})
	rt.SetCommandRegistry(reg)

	dir := writePlugin(t, "cmd-call", `
result = claudio.cmd("echo foo bar")
`)
	if err := rt.LoadPlugin("cmd-call", dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	plugin := rt.plugins[0]
	plugin.mu.Lock()
	val := plugin.L.GetGlobal("result")
	plugin.mu.Unlock()

	if val.String() != "echo:foo bar" {
		t.Errorf("result = %q, want %q", val.String(), "echo:foo bar")
	}
}

// TestKeymapAPI_Del verifies keymap.del removes the key from the registry.
func TestKeymapAPI_Del(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	// Wire an instance registry with a known keymap.
	reg := vim.NewKeymapRegistry()
	reg.Register(vim.Keymap{
		Key:         'j',
		Mode:        vim.ModeNormal,
		Description: "move down",
		Handler:     func(key rune, text string, cursor int, count int, s *vim.State) vim.Action { return vim.Action{} },
	})
	rt.SetKeymapRegistry(reg)

	// Verify key exists before del.
	if _, ok := reg.Lookup('j', vim.ModeNormal); !ok {
		t.Fatal("keymap 'j' normal not found before del")
	}

	dir := writePlugin(t, "keymap-del", `
claudio.keymap.del("normal", "j")
`)
	if err := rt.LoadPlugin("keymap-del", dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	if _, ok := reg.Lookup('j', vim.ModeNormal); ok {
		t.Error("keymap 'j' normal still present after del")
	}
}

// TestKeymapAPI_List verifies keymap.list returns all maps for a mode.
func TestKeymapAPI_List(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	reg := vim.NewKeymapRegistry()
	reg.Register(vim.Keymap{Key: 'a', Mode: vim.ModeInsert, Description: "insert a"})
	reg.Register(vim.Keymap{Key: 'b', Mode: vim.ModeInsert, Description: "insert b"})
	reg.Register(vim.Keymap{Key: 'x', Mode: vim.ModeNormal, Description: "normal x"})
	rt.SetKeymapRegistry(reg)

	dir := writePlugin(t, "keymap-list", `
maps = claudio.keymap.list("insert")
list_len = #maps
`)
	if err := rt.LoadPlugin("keymap-list", dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	plugin := rt.plugins[0]
	plugin.mu.Lock()
	val := plugin.L.GetGlobal("list_len")
	plugin.mu.Unlock()

	if val.String() != "2" {
		t.Errorf("list_len = %q, want 2", val.String())
	}
}

// TestKeymapAPI_List_EmptyMode verifies unknown mode returns empty table.
func TestKeymapAPI_List_EmptyMode(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	reg := vim.NewKeymapRegistry()
	rt.SetKeymapRegistry(reg)

	dir := writePlugin(t, "keymap-list-empty", `
maps = claudio.keymap.list("visual")
list_len = #maps
`)
	if err := rt.LoadPlugin("keymap-list-empty", dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	plugin := rt.plugins[0]
	plugin.mu.Lock()
	val := plugin.L.GetGlobal("list_len")
	plugin.mu.Unlock()

	if val.String() != "0" {
		t.Errorf("list_len = %q, want 0", val.String())
	}
}
