package lua

import (
	lua "github.com/yuin/gopher-lua"
)

// newSandboxedState creates a new LState with only safe stdlib modules loaded.
// Dangerous functions (os.execute, io.popen, load, loadfile, dofile, loadstring)
// are removed or never loaded.
func newSandboxedState() *lua.LState {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})

	// Open safe libs only
	for _, lib := range []struct {
		name string
		fn   lua.LGFunction
	}{
		{lua.BaseLibName, lua.OpenBase},
		{lua.TabLibName, lua.OpenTable},
		{lua.StringLibName, lua.OpenString},
		{lua.MathLibName, lua.OpenMath},
	} {
		L.Push(L.NewFunction(lib.fn))
		L.Push(lua.LString(lib.name))
		L.Call(1, 0)
	}

	// Remove dangerous base globals
	for _, name := range []string{"dofile", "loadfile", "load", "loadstring"} {
		L.SetGlobal(name, lua.LNil)
	}

	return L
}
