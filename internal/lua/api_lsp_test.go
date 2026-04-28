package lua

import (
	"testing"

	"github.com/Abraxas-365/claudio/internal/config"
	lsp "github.com/Abraxas-365/claudio/internal/services/lsp"
)

// newLSPRuntime creates a testRuntime with a wired ServerManager (no real servers).
func newLSPRuntime(t *testing.T) (*Runtime, *lsp.ServerManager) {
	t.Helper()
	rt := testRuntime(t)
	mgr := lsp.NewServerManager(nil) // start with no servers
	rt.SetLSPManager(mgr)
	return rt, mgr
}

// TestLspAPI_RegisterServer verifies that register_server adds a config to the manager.
func TestLspAPI_RegisterServer(t *testing.T) {
	rt, mgr := newLSPRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "lsp-register", `
claudio.lsp.register_server({
  name       = "testls",
  command    = "echo",
  args       = { "--version" },
  extensions = { ".test", "test2" },
  env        = { MY_VAR = "hello" },
})
`)
	if err := rt.LoadPlugin("lsp-register", dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	servers := mgr.ListServers()
	if _, ok := servers["testls"]; !ok {
		t.Fatalf("expected 'testls' in ListServers(), got %v", servers)
	}

	// Verify ext mapping (should handle both ".test" and ".test2")
	if got := mgr.ServerForFile("foo.test"); got != "testls" {
		t.Errorf("ServerForFile(.test) = %q, want %q", got, "testls")
	}
	if got := mgr.ServerForFile("foo.test2"); got != "testls" {
		t.Errorf("ServerForFile(.test2) = %q, want %q", got, "testls")
	}
}

// TestLspAPI_RegisterServer_MissingName verifies that omitting 'name' returns an error.
func TestLspAPI_RegisterServer_MissingName(t *testing.T) {
	rt, _ := newLSPRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "lsp-no-name", `
local _, err = claudio.lsp.register_server({ command = "echo" })
lsp_err = err
`)
	if err := rt.LoadPlugin("lsp-no-name", dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	plugin := rt.plugins[0]
	plugin.mu.Lock()
	errVal := plugin.L.GetGlobal("lsp_err")
	plugin.mu.Unlock()

	if errVal.String() == "" || errVal.String() == "nil" {
		t.Errorf("expected non-empty error string, got %q", errVal.String())
	}
}

// TestLspAPI_NilManagerGraceful verifies all functions handle nil manager without panic.
func TestLspAPI_NilManagerGraceful(t *testing.T) {
	rt := testRuntime(t) // NO SetLSPManager
	defer rt.Close()

	dir := writePlugin(t, "lsp-nil-mgr", `
-- register_server should return nil+err
local ok, err = claudio.lsp.register_server({ name = "x", command = "echo" })
nil_reg_err = err

-- enable should return nil+err
local ok2, err2 = claudio.lsp.enable("x")
nil_enable_err = err2

-- disable should silently no-op
claudio.lsp.disable("x")

-- list should return empty table
local servers = claudio.lsp.list()
nil_list_len = 0
for _ in pairs(servers) do nil_list_len = nil_list_len + 1 end

-- query functions should return nil+err
local r, qerr = claudio.lsp.hover("/tmp/foo.go", 0, 0)
nil_hover_err = qerr
`)
	if err := rt.LoadPlugin("lsp-nil-mgr", dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	plugin := rt.plugins[0]
	plugin.mu.Lock()
	regErr := plugin.L.GetGlobal("nil_reg_err")
	enableErr := plugin.L.GetGlobal("nil_enable_err")
	listLen := plugin.L.GetGlobal("nil_list_len")
	hoverErr := plugin.L.GetGlobal("nil_hover_err")
	plugin.mu.Unlock()

	if regErr.String() == "" || regErr.String() == "nil" {
		t.Errorf("register_server: expected error, got %q", regErr)
	}
	if enableErr.String() == "" || enableErr.String() == "nil" {
		t.Errorf("enable: expected error, got %q", enableErr)
	}
	if listLen.String() != "0" {
		t.Errorf("list length = %q, want 0", listLen.String())
	}
	if hoverErr.String() == "" || hoverErr.String() == "nil" {
		t.Errorf("hover: expected error, got %q", hoverErr)
	}
}

// TestLspAPI_List verifies that list() returns configured servers.
func TestLspAPI_List(t *testing.T) {
	rt, mgr := newLSPRuntime(t)
	defer rt.Close()

	// Pre-register a server via Go API
	mgr.RegisterServer("golsp", config.LspServerConfig{
		Command:    "echo",
		Extensions: []string{".go"},
	})

	dir := writePlugin(t, "lsp-list", `
local servers = claudio.lsp.list()
lsp_list_count = #servers
lsp_first_name = servers[1] and servers[1].name or ""
lsp_first_running = servers[1] and servers[1].running or false
`)
	if err := rt.LoadPlugin("lsp-list", dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	plugin := rt.plugins[0]
	plugin.mu.Lock()
	count := plugin.L.GetGlobal("lsp_list_count")
	name := plugin.L.GetGlobal("lsp_first_name")
	running := plugin.L.GetGlobal("lsp_first_running")
	plugin.mu.Unlock()

	if count.String() != "1" {
		t.Errorf("list count = %q, want 1", count.String())
	}
	if name.String() != "golsp" {
		t.Errorf("first server name = %q, want golsp", name.String())
	}
	if running.String() != "false" {
		t.Errorf("running = %q, want false (not started)", running.String())
	}
}

// TestLspAPI_Disable verifies that disable() removes the server config.
func TestLspAPI_Disable(t *testing.T) {
	rt, mgr := newLSPRuntime(t)
	defer rt.Close()

	mgr.RegisterServer("tmp-ls", config.LspServerConfig{
		Command:    "echo",
		Extensions: []string{".tmp"},
	})

	// Confirm it's registered
	before := mgr.ListServers()
	if _, ok := before["tmp-ls"]; !ok {
		t.Fatal("precondition: tmp-ls not in ListServers")
	}

	dir := writePlugin(t, "lsp-disable", `claudio.lsp.disable("tmp-ls")`)
	if err := rt.LoadPlugin("lsp-disable", dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	after := mgr.ListServers()
	if _, ok := after["tmp-ls"]; ok {
		t.Error("expected 'tmp-ls' removed after disable(), still present")
	}
}

// TestLspAPI_EnableUnknownServer verifies enable() returns an error for unconfigured names.
func TestLspAPI_EnableUnknownServer(t *testing.T) {
	rt, _ := newLSPRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "lsp-enable-unknown", `
local ok, err = claudio.lsp.enable("nonexistent-server")
enable_err = err
`)
	if err := rt.LoadPlugin("lsp-enable-unknown", dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	plugin := rt.plugins[0]
	plugin.mu.Lock()
	errVal := plugin.L.GetGlobal("enable_err")
	plugin.mu.Unlock()

	if errVal.String() == "" || errVal.String() == "nil" {
		t.Errorf("expected error for unknown server, got %q", errVal)
	}
}

// TestLspAPI_QueryNoServerForFile verifies hover/go_to_definition etc. return err
// when no server handles the file extension.
func TestLspAPI_QueryNoServerForFile(t *testing.T) {
	rt, _ := newLSPRuntime(t) // manager has no servers configured
	defer rt.Close()

	dir := writePlugin(t, "lsp-query-no-server", `
local r, err = claudio.lsp.hover("/tmp/foo.go", 1, 1)
hover_err = err
local r2, err2 = claudio.lsp.go_to_definition("/tmp/foo.go", 1, 1)
def_err = err2
local r3, err3 = claudio.lsp.find_references("/tmp/foo.go", 1, 1)
refs_err = err3
local r4, err4 = claudio.lsp.document_symbols("/tmp/foo.go")
syms_err = err4
`)
	if err := rt.LoadPlugin("lsp-query-no-server", dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	plugin := rt.plugins[0]
	plugin.mu.Lock()
	hoverErr := plugin.L.GetGlobal("hover_err")
	defErr := plugin.L.GetGlobal("def_err")
	refsErr := plugin.L.GetGlobal("refs_err")
	symsErr := plugin.L.GetGlobal("syms_err")
	plugin.mu.Unlock()

	for name, val := range map[string]string{
		"hover_err": hoverErr.String(),
		"def_err":   defErr.String(),
		"refs_err":  refsErr.String(),
		"syms_err":  symsErr.String(),
	} {
		if val == "" || val == "nil" {
			t.Errorf("%s: expected error string, got %q", name, val)
		}
	}
}

// TestLspAPI_RegisterServer_SettingsUnaffected verifies that the original settings
// are not mutated when Lua registers a server (regression: settings.json must still work).
func TestLspAPI_RegisterServer_SettingsUnaffected(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	// Wire manager built from settings (simulating app.go pattern)
	settingsCfgs := map[string]config.LspServerConfig{
		"from-settings": {Command: "echo", Extensions: []string{".settings"}},
	}
	mgr := lsp.NewServerManager(settingsCfgs)
	rt.SetLSPManager(mgr)

	dir := writePlugin(t, "lsp-additive", `
claudio.lsp.register_server({
  name       = "from-lua",
  command    = "true",
  extensions = { ".lua" },
})
`)
	if err := rt.LoadPlugin("lsp-additive", dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	servers := mgr.ListServers()
	if _, ok := servers["from-settings"]; !ok {
		t.Error("'from-settings' server lost after Lua register_server")
	}
	if _, ok := servers["from-lua"]; !ok {
		t.Error("'from-lua' server not added")
	}
	if len(servers) != 2 {
		t.Errorf("expected 2 servers, got %d: %v", len(servers), servers)
	}
}
