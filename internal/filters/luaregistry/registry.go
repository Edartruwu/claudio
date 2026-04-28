// Package luaregistry provides a thread-safe store for Lua-registered output
// filters. Filters registered here take priority over TOML and Go built-ins.
package luaregistry

import (
	"regexp"
	"sync"

	"github.com/Abraxas-365/claudio/internal/tools/outputfilter/tomlfilter"
	lua "github.com/yuin/gopher-lua"
)

// Entry holds a Lua-registered filter: declarative FilterDef fields plus an
// optional transform LFunction that runs under the plugin VM mutex.
type Entry struct {
	Name      string
	Def       tomlfilter.FilterDef
	CommandRe *regexp.Regexp

	// Transform is an optional Lua function called after the declarative
	// pipeline. It receives the current output string and returns a new one.
	Transform *lua.LFunction

	// VM is the plugin's LState — needed to call Transform.
	VM *lua.LState

	// Mu is the plugin VM's mutex — MUST be held when calling Transform.
	Mu *sync.Mutex
}

var (
	mu       sync.RWMutex
	registry = make(map[string]*Entry)
)

// Register adds or replaces a named Lua filter.
func Register(name string, e *Entry) {
	mu.Lock()
	defer mu.Unlock()
	registry[name] = e
}

// Unregister removes a named Lua filter.
func Unregister(name string) {
	mu.Lock()
	defer mu.Unlock()
	delete(registry, name)
}

// Lookup finds the first Lua filter whose match_command regex matches cmd.
// Returns nil, false if no filter matches.
func Lookup(cmd string) (*Entry, bool) {
	mu.RLock()
	defer mu.RUnlock()
	for _, e := range registry {
		if e.CommandRe != nil && e.CommandRe.MatchString(cmd) {
			return e, true
		}
	}
	return nil, false
}

// List returns names of all registered Lua filters.
func List() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}

// Reset clears all entries. Used in tests.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	registry = make(map[string]*Entry)
}
