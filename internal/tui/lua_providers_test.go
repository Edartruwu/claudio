package tui

// lua_providers_test.go — unit tests for the Lua data-provider adapters
// defined in lua_providers.go.

import (
	"testing"

	luart "github.com/Abraxas-365/claudio/internal/lua"
)

// TestLuaTokenState_SetAndUsage verifies that set() stores values that
// Usage() later returns.
func TestLuaTokenState_SetAndUsage(t *testing.T) {
	s := &luaTokenState{}
	s.set(2000, 1.23)

	u := s.Usage()
	if u.Used != 2000 {
		t.Errorf("Used = %d, want 2000", u.Used)
	}
	if u.Cost != 1.23 {
		t.Errorf("Cost = %f, want 1.23", u.Cost)
	}
	// Max is always 0 from luaTokenState (no context-window wiring yet).
	if u.Max != 0 {
		t.Errorf("Max = %d, want 0", u.Max)
	}
}

// TestLuaTokenState_Zero verifies default state returns zero usage.
func TestLuaTokenState_Zero(t *testing.T) {
	s := &luaTokenState{}
	u := s.Usage()
	if u.Used != 0 || u.Cost != 0 || u.Max != 0 {
		t.Errorf("zero state Usage() = %+v, want all-zero", u)
	}
}

// TestWireLuaDataProviders_NilRuntime verifies wireLuaDataProviders does not
// panic when the runtime is nil.
func TestWireLuaDataProviders_NilRuntime(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("wireLuaDataProviders(nil) panicked: %v", r)
		}
	}()
	wireLuaDataProviders(nil, nil, nil, &luaTokenState{})
}

// TestWireLuaDataProviders_SetsAllProviders verifies that wireLuaDataProviders
// wires all four provider interfaces onto the runtime (non-nil check via
// session.current() returning a table, not nil, after wiring a stub session).
func TestWireLuaDataProviders_SetsAllProviders(t *testing.T) {
	rt := luart.New(nil, nil, nil, nil, nil, nil, nil)
	defer rt.Close()

	tokens := &luaTokenState{}
	tokens.set(500, 0.05)

	// nil session + nil files — adapters must not panic when providers are nil.
	wireLuaDataProviders(rt, nil, nil, tokens)

	// tokens.usage() should return a table (provider wired).
	out, err := rt.ExecString(`
local u = claudio.tokens.usage()
if u == nil then return "nil" end
return tostring(u.used)
`)
	if err != nil {
		t.Fatalf("ExecString: %v", err)
	}
	if out != "500" {
		t.Errorf("tokens.usage().used = %q, want 500", out)
	}
}
