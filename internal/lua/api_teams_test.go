package lua

import (
	"testing"

	"github.com/Abraxas-365/claudio/internal/teams"
	lua "github.com/yuin/gopher-lua"
)

// runLuaResult executes luaCode in a transient state wired to rt and returns the
// top-of-stack Lua value. Fatals on load/exec error.
func runLuaResult(t *testing.T, rt *Runtime, luaCode string) lua.LValue {
	t.Helper()
	L := newSandboxedState()
	defer L.Close()
	plugin := &loadedPlugin{name: "<test>"}
	rt.injectAPI(L, plugin)
	if err := L.DoString(luaCode); err != nil {
		t.Fatalf("lua exec error: %v", err)
	}
	if L.GetTop() == 0 {
		return lua.LNil
	}
	return L.Get(-1)
}

// ── claudio.agent.list ────────────────────────────────────────────────────────

func TestAgentList_NilRunner(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	val := runLuaResult(t, rt, `return claudio.agent.list()`)
	tbl, ok := val.(*lua.LTable)
	if !ok {
		t.Fatalf("expected table, got %T", val)
	}
	if tbl.Len() != 0 {
		t.Errorf("expected empty table, got len=%d", tbl.Len())
	}
}

func TestAgentList_EmptyRunner(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	mgr := teams.NewManager(t.TempDir())
	runner := teams.NewTeammateRunner(mgr, nil)
	rt.SetTeamRunner(runner)

	val := runLuaResult(t, rt, `return claudio.agent.list()`)
	tbl, ok := val.(*lua.LTable)
	if !ok {
		t.Fatalf("expected table, got %T", val)
	}
	if tbl.Len() != 0 {
		t.Errorf("expected empty table for runner with no agents, got len=%d", tbl.Len())
	}
}

// ── claudio.agent.status ──────────────────────────────────────────────────────

func TestAgentStatus_NilRunner(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	val := runLuaResult(t, rt, `return claudio.agent.status("nonexistent@team")`)
	if val != lua.LNil {
		t.Errorf("expected nil for nil runner, got %v", val)
	}
}

func TestAgentStatus_UnknownID(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	mgr := teams.NewManager(t.TempDir())
	runner := teams.NewTeammateRunner(mgr, nil)
	rt.SetTeamRunner(runner)

	val := runLuaResult(t, rt, `return claudio.agent.status("ghost@nowhere")`)
	if val != lua.LNil {
		t.Errorf("expected nil for unknown agent, got %v", val)
	}
}

// ── claudio.teams.list ────────────────────────────────────────────────────────

func TestTeamsList_NilManager(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	val := runLuaResult(t, rt, `return claudio.teams.list()`)
	tbl, ok := val.(*lua.LTable)
	if !ok {
		t.Fatalf("expected table, got %T", val)
	}
	if tbl.Len() != 0 {
		t.Errorf("expected empty table, got len=%d", tbl.Len())
	}
}

func TestTeamsList_WithTeams(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	mgr := teams.NewManager(t.TempDir())
	if _, err := mgr.CreateTeam("alpha", "first team", "sess1", ""); err != nil {
		t.Fatalf("CreateTeam alpha: %v", err)
	}
	if _, err := mgr.CreateTeam("beta", "second team", "sess2", ""); err != nil {
		t.Fatalf("CreateTeam beta: %v", err)
	}
	rt.SetTeamManager(mgr)

	val := runLuaResult(t, rt, `return claudio.teams.list()`)
	tbl, ok := val.(*lua.LTable)
	if !ok {
		t.Fatalf("expected table, got %T", val)
	}
	if tbl.Len() != 2 {
		t.Errorf("expected 2 teams, got %d", tbl.Len())
	}

	// Collect names into a set for order-independent check.
	names := map[string]bool{}
	tbl.ForEach(func(_, v lua.LValue) {
		names[v.String()] = true
	})
	for _, want := range []string{"alpha", "beta"} {
		if !names[want] {
			t.Errorf("team %q not in list; got %v", want, names)
		}
	}
}

// ── claudio.teams.members ────────────────────────────────────────────────────

func TestTeamsMembers_NilManager(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	val := runLuaResult(t, rt, `return claudio.teams.members("noteam")`)
	tbl, ok := val.(*lua.LTable)
	if !ok {
		t.Fatalf("expected table, got %T", val)
	}
	if tbl.Len() != 0 {
		t.Errorf("expected empty table, got len=%d", tbl.Len())
	}
}

func TestTeamsMembers_WithMembers(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	mgr := teams.NewManager(t.TempDir())
	if _, err := mgr.CreateTeam("my-team", "test team", "sess1", ""); err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	if _, err := mgr.AddMember("my-team", "researcher", "", "Do research", "research-mid"); err != nil {
		t.Fatalf("AddMember researcher: %v", err)
	}
	if _, err := mgr.AddMember("my-team", "coder", "", "Write code", "backend-mid"); err != nil {
		t.Fatalf("AddMember coder: %v", err)
	}
	rt.SetTeamManager(mgr)

	val := runLuaResult(t, rt, `return claudio.teams.members("my-team")`)
	tbl, ok := val.(*lua.LTable)
	if !ok {
		t.Fatalf("expected table, got %T", val)
	}
	if tbl.Len() != 2 {
		t.Errorf("expected 2 members, got %d", tbl.Len())
	}

	// Each entry must have id, name, role, status fields.
	tbl.ForEach(func(_, v lua.LValue) {
		entry, ok := v.(*lua.LTable)
		if !ok {
			t.Errorf("member entry is %T, want *lua.LTable", v)
			return
		}
		for _, field := range []string{"id", "name", "role", "status"} {
			if entry.RawGetString(field) == lua.LNil {
				t.Errorf("member entry missing field %q", field)
			}
		}
	})
}

func TestTeamsMembers_UnknownTeam(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	mgr := teams.NewManager(t.TempDir())
	rt.SetTeamManager(mgr)

	val := runLuaResult(t, rt, `return claudio.teams.members("ghost-team")`)
	tbl, ok := val.(*lua.LTable)
	if !ok {
		t.Fatalf("expected table, got %T", val)
	}
	if tbl.Len() != 0 {
		t.Errorf("expected empty table for unknown team, got len=%d", tbl.Len())
	}
}
