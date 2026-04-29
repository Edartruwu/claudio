package lua

// api_data_test.go — tests for claudio.session.current(), claudio.files.list(),
// claudio.tasks.list(), claudio.tokens.usage() and the BlockRegistry flush
// behaviour in SetBlockRegistry.

import (
	"testing"

	"github.com/Abraxas-365/claudio/internal/tui/sidebar"
)

// ---------------------------------------------------------------------------
// Stub providers
// ---------------------------------------------------------------------------

type stubSessionProvider struct{ id, name, model string }

func (s stubSessionProvider) CurrentID() string    { return s.id }
func (s stubSessionProvider) CurrentName() string  { return s.name }
func (s stubSessionProvider) CurrentModel() string { return s.model }

type stubFilesProvider struct{ entries []FileEntry }

func (s stubFilesProvider) List() []FileEntry { return s.entries }

type stubTasksProvider struct{ entries []TaskEntry }

func (s stubTasksProvider) List() []TaskEntry { return s.entries }

type stubTokensProvider struct{ usage TokenUsage }

func (s stubTokensProvider) Usage() TokenUsage { return s.usage }

// ---------------------------------------------------------------------------
// claudio.session.current()
// ---------------------------------------------------------------------------

// TestDataAPI_SessionCurrent_WithProvider checks that session.current() returns
// a table with id/name/model fields when a provider is wired.
func TestDataAPI_SessionCurrent_WithProvider(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	rt.SetSessionProvider(stubSessionProvider{id: "s1", name: "My Session", model: "claude-opus"})

	out, err := rt.ExecString(`
local s = claudio.session.current()
if s == nil then return "nil" end
return s.id .. "|" .. s.name .. "|" .. s.model
`)
	if err != nil {
		t.Fatalf("ExecString: %v", err)
	}
	want := "s1|My Session|claude-opus"
	if out != want {
		t.Errorf("session.current() = %q, want %q", out, want)
	}
}

// TestDataAPI_SessionCurrent_NilProvider checks that session.current() returns nil
// when no provider is wired (graceful degradation).
func TestDataAPI_SessionCurrent_NilProvider(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	// No provider wired.
	out, err := rt.ExecString(`
local s = claudio.session.current()
if s == nil then return "nil" else return "not-nil" end
`)
	if err != nil {
		t.Fatalf("ExecString: %v", err)
	}
	if out != "nil" {
		t.Errorf("expected nil without provider, got %q", out)
	}
}

// ---------------------------------------------------------------------------
// claudio.files.list()
// ---------------------------------------------------------------------------

// TestDataAPI_FilesList_WithProvider checks that files.list() returns entries.
func TestDataAPI_FilesList_WithProvider(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	rt.SetFilesProvider(stubFilesProvider{entries: []FileEntry{
		{Path: "/a/foo.go", Name: "foo.go"},
		{Path: "/a/bar.go", Name: "bar.go"},
	}})

	out, err := rt.ExecString(`
local files = claudio.files.list()
return tostring(#files)
`)
	if err != nil {
		t.Fatalf("ExecString: %v", err)
	}
	if out != "2" {
		t.Errorf("files.list() len = %q, want 2", out)
	}
}

// TestDataAPI_FilesList_NilProvider checks that files.list() returns empty table
// (not nil) when no provider is wired.
func TestDataAPI_FilesList_NilProvider(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	out, err := rt.ExecString(`
local files = claudio.files.list()
if files == nil then return "nil" end
return tostring(#files)
`)
	if err != nil {
		t.Fatalf("ExecString: %v", err)
	}
	if out != "0" {
		t.Errorf("files.list() without provider = %q, want 0", out)
	}
}

// ---------------------------------------------------------------------------
// claudio.tasks.list()
// ---------------------------------------------------------------------------

// TestDataAPI_TasksList_WithProvider checks that tasks.list() returns entries.
func TestDataAPI_TasksList_WithProvider(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	rt.SetTasksProvider(stubTasksProvider{entries: []TaskEntry{
		{ID: "t1", Title: "Fix bug", Status: "open"},
		{ID: "t2", Title: "Write tests", Status: "in_progress"},
	}})

	out, err := rt.ExecString(`
local tasks = claudio.tasks.list()
return tostring(#tasks)
`)
	if err != nil {
		t.Fatalf("ExecString: %v", err)
	}
	if out != "2" {
		t.Errorf("tasks.list() len = %q, want 2", out)
	}
}

// TestDataAPI_TasksList_NilProvider checks tasks.list() returns empty table without provider.
func TestDataAPI_TasksList_NilProvider(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	out, err := rt.ExecString(`
local tasks = claudio.tasks.list()
if tasks == nil then return "nil" end
return tostring(#tasks)
`)
	if err != nil {
		t.Fatalf("ExecString: %v", err)
	}
	if out != "0" {
		t.Errorf("tasks.list() without provider = %q, want 0", out)
	}
}

// ---------------------------------------------------------------------------
// claudio.tokens.usage()
// ---------------------------------------------------------------------------

// TestDataAPI_TokensUsage_WithProvider checks tokens.usage() returns a table
// with used/max/cost fields.
func TestDataAPI_TokensUsage_WithProvider(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	rt.SetTokensProvider(stubTokensProvider{usage: TokenUsage{Used: 1000, Max: 8000, Cost: 0.05}})

	out, err := rt.ExecString(`
local u = claudio.tokens.usage()
if u == nil then return "nil" end
return tostring(u.used) .. "|" .. tostring(u.max)
`)
	if err != nil {
		t.Fatalf("ExecString: %v", err)
	}
	want := "1000|8000"
	if out != want {
		t.Errorf("tokens.usage() = %q, want %q", out, want)
	}
}

// TestDataAPI_TokensUsage_NilProvider checks tokens.usage() returns nil without provider.
func TestDataAPI_TokensUsage_NilProvider(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	out, err := rt.ExecString(`
local u = claudio.tokens.usage()
if u == nil then return "nil" else return "not-nil" end
`)
	if err != nil {
		t.Fatalf("ExecString: %v", err)
	}
	if out != "nil" {
		t.Errorf("expected nil without provider, got %q", out)
	}
}

// ---------------------------------------------------------------------------
// SetBlockRegistry — flush pending blocks
// ---------------------------------------------------------------------------

// TestSetBlockRegistry_FlushesPreRegisteredBlocks verifies that blocks
// registered via register_sidebar_block before SetBlockRegistry is called
// are flushed into the registry when SetBlockRegistry is called.
func TestSetBlockRegistry_FlushesPreRegisteredBlocks(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	// Register a block before the registry is wired.
	dir := writePlugin(t, "flush-test", `
claudio.ui.register_sidebar_block({
  id     = "flush-block",
  title  = "Flush Block",
  weight = 5,
  render = function(w, h) return "flushed" end,
})
`)
	if err := rt.LoadPlugin("flush-test", dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	// Verify it is in the pending queue.
	if pending := rt.GetSidebarBlocks(); len(pending) != 1 {
		t.Fatalf("pending blocks = %d, want 1 before SetBlockRegistry", len(pending))
	}

	// Wire the registry — should flush pending blocks.
	reg := sidebar.NewBlockRegistry()
	rt.SetBlockRegistry(reg)

	// Pending queue must now be empty.
	if pending := rt.GetSidebarBlocks(); len(pending) != 0 {
		t.Errorf("pending blocks = %d after SetBlockRegistry, want 0", len(pending))
	}

	// Registry must have the block.
	blocks := reg.Blocks()
	if len(blocks) != 1 {
		t.Fatalf("registry blocks = %d, want 1", len(blocks))
	}
	if blocks[0].Title() != "Flush Block" {
		t.Errorf("block title = %q, want %q", blocks[0].Title(), "Flush Block")
	}
}

// TestSetBlockRegistry_DirectRegistration verifies that blocks registered
// via register_sidebar_block after SetBlockRegistry go directly into the
// registry without passing through the pending queue.
func TestSetBlockRegistry_DirectRegistration(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	reg := sidebar.NewBlockRegistry()
	rt.SetBlockRegistry(reg)

	// Register a block AFTER the registry is already wired.
	dir := writePlugin(t, "direct-test", `
claudio.ui.register_sidebar_block({
  id     = "direct-block",
  title  = "Direct Block",
  weight = 2,
  render = function(w, h) return "direct" end,
})
`)
	if err := rt.LoadPlugin("direct-test", dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	// Pending queue must stay empty — block went straight to registry.
	if pending := rt.GetSidebarBlocks(); len(pending) != 0 {
		t.Errorf("pending blocks = %d, want 0 (direct registration)", len(pending))
	}

	// Block must be in registry.
	blocks := reg.Blocks()
	if len(blocks) != 1 {
		t.Fatalf("registry blocks = %d, want 1", len(blocks))
	}
	if blocks[0].Title() != "Direct Block" {
		t.Errorf("block title = %q, want %q", blocks[0].Title(), "Direct Block")
	}
}

// ---------------------------------------------------------------------------
// defaults.lua — registers exactly 4 blocks
// ---------------------------------------------------------------------------

// TestDefaultsLua_RegistersFourSidebarBlocks loads defaults.lua into a runtime
// with a wired BlockRegistry and verifies exactly 4 sidebar blocks are
// registered (files, session, todos/tasks, tokens).
func TestDefaultsLua_RegistersFourSidebarBlocks(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	reg := sidebar.NewBlockRegistry()
	rt.SetBlockRegistry(reg)

	// Wire providers so render functions do not panic on nil.
	rt.SetSessionProvider(stubSessionProvider{id: "s1", name: "Test", model: "m"})
	rt.SetFilesProvider(stubFilesProvider{})
	rt.SetTasksProvider(stubTasksProvider{})
	rt.SetTokensProvider(stubTokensProvider{usage: TokenUsage{Used: 100, Max: 8000, Cost: 0.01}})

	if err := rt.LoadDefaults(); err != nil {
		t.Fatalf("LoadDefaults: %v", err)
	}

	blocks := reg.Blocks()
	if len(blocks) != 4 {
		t.Errorf("expected 4 sidebar blocks from defaults.lua, got %d", len(blocks))
	}
}

// TestSidebarBlock_Render_NoNilPanic verifies that Render() on a Lua-backed
// sidebar block does not panic even when providers return empty data.
func TestSidebarBlock_Render_NoNilPanic(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	reg := sidebar.NewBlockRegistry()
	rt.SetBlockRegistry(reg)
	rt.SetSessionProvider(stubSessionProvider{})
	rt.SetFilesProvider(stubFilesProvider{})
	rt.SetTasksProvider(stubTasksProvider{})
	rt.SetTokensProvider(stubTokensProvider{usage: TokenUsage{Used: 0, Max: 0, Cost: 0}})

	if err := rt.LoadDefaults(); err != nil {
		t.Fatalf("LoadDefaults: %v", err)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("sidebar block Render panicked: %v", r)
		}
	}()

	for _, block := range reg.Blocks() {
		_ = block.Render(40, 10)
	}
}
