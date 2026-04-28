// Package lua embeds gopher-lua VMs to provide a Neovim-style plugin system.
// Each plugin runs in an isolated sandbox with a `claudio` global table exposing
// tool registration, event bus, config, hooks, and notification APIs.
package lua

import (
	"fmt"
	"log"
	"sync"

	"github.com/Abraxas-365/claudio/internal/bus"
	"github.com/Abraxas-365/claudio/internal/capabilities"
	"github.com/Abraxas-365/claudio/internal/cli/commands"
	"github.com/Abraxas-365/claudio/internal/config"
	"github.com/Abraxas-365/claudio/internal/hooks"
	"github.com/Abraxas-365/claudio/internal/services/skills"
	"github.com/Abraxas-365/claudio/internal/storage"
	"github.com/Abraxas-365/claudio/internal/tools"
	"github.com/Abraxas-365/claudio/internal/tui/vim"
	lua "github.com/yuin/gopher-lua"
)

// pendingCommand holds a command registered from Lua before commandRegistry is wired.
type pendingCommand struct {
	cmd *commands.Command
}

// Runtime manages Lua plugin lifecycle: loading, sandbox creation, and shutdown.
type Runtime struct {
	toolReg *tools.Registry
	skills  *skills.Registry
	bus     *bus.Bus
	hooks   *hooks.Manager
	cfg     *config.Settings
	db      *storage.DB
	caps    *capabilities.Registry

	commandRegistry *commands.Registry
	keymapRegistry  *vim.KeymapRegistry
	pendingCommands []*pendingCommand

	mu      sync.Mutex
	plugins []*loadedPlugin
}

// loadedPlugin tracks a single plugin's Lua VM and cleanup handles.
type loadedPlugin struct {
	name   string
	dir    string
	L      *lua.LState
	mu     sync.Mutex // protects L (gopher-lua is NOT goroutine-safe)
	unsubs []func()   // bus unsubscribe handles
}

// New creates a Lua plugin runtime wired to the given app dependencies.
func New(
	toolReg *tools.Registry,
	skillsReg *skills.Registry,
	eventBus *bus.Bus,
	hooksMgr *hooks.Manager,
	cfg *config.Settings,
	db *storage.DB,
	caps *capabilities.Registry,
) *Runtime {
	return &Runtime{
		toolReg: toolReg,
		skills:  skillsReg,
		bus:     eventBus,
		hooks:   hooksMgr,
		cfg:     cfg,
		db:      db,
		caps:    caps,
	}
}

// SetCommandRegistry wires a command registry into the runtime and flushes any
// commands that were registered from Lua before this call.
func (r *Runtime) SetCommandRegistry(reg *commands.Registry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commandRegistry = reg
	for _, p := range r.pendingCommands {
		reg.Register(p.cmd)
	}
	r.pendingCommands = nil
}

// SetKeymapRegistry wires a keymap registry into the runtime.
func (r *Runtime) SetKeymapRegistry(reg *vim.KeymapRegistry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.keymapRegistry = reg
}

// LoadAll scans pluginsDir for subdirectories containing init.lua and loads each
// in an isolated sandbox. Errors per-plugin are logged but do NOT stop loading.
func (r *Runtime) LoadAll(pluginsDir string) error {
	dirs, err := discoverPlugins(pluginsDir)
	if err != nil {
		return fmt.Errorf("lua: discover plugins: %w", err)
	}
	for _, pd := range dirs {
		if loadErr := r.LoadPlugin(pd.name, pd.dir); loadErr != nil {
			log.Printf("[lua] plugin %s: %v", pd.name, loadErr)
		}
	}
	return nil
}

// LoadPlugin loads a single init.lua from dir in a new sandboxed LState.
func (r *Runtime) LoadPlugin(name, dir string) (retErr error) {
	// Catch panics from gopher-lua
	defer func() {
		if rv := recover(); rv != nil {
			retErr = fmt.Errorf("panic loading plugin %s: %v", name, rv)
		}
	}()

	L := newSandboxedState()

	plugin := &loadedPlugin{
		name: name,
		dir:  dir,
		L:    L,
	}

	r.injectAPI(L, plugin)

	initFile := dir + "/init.lua"
	if err := L.DoFile(initFile); err != nil {
		L.Close()
		return fmt.Errorf("load %s: %w", initFile, err)
	}

	r.mu.Lock()
	r.plugins = append(r.plugins, plugin)
	r.mu.Unlock()

	log.Printf("[lua] loaded plugin: %s", name)
	return nil
}

// Close shuts down all Lua VMs and unsubscribes bus handlers.
func (r *Runtime) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.plugins {
		for _, unsub := range p.unsubs {
			unsub()
		}
		p.L.Close()
	}
	r.plugins = nil
}

// injectAPI registers the `claudio` global table with all API bindings.
func (r *Runtime) injectAPI(L *lua.LState, plugin *loadedPlugin) {
	claudio := L.NewTable()

	L.SetField(claudio, "register_tool", L.NewFunction(r.apiRegisterTool(plugin)))
	L.SetField(claudio, "register_skill", L.NewFunction(r.apiRegisterSkill(plugin)))
	L.SetField(claudio, "subscribe", L.NewFunction(r.apiSubscribe(plugin)))
	L.SetField(claudio, "publish", L.NewFunction(r.apiPublish(plugin)))
	L.SetField(claudio, "get_config", L.NewFunction(r.apiGetConfig(plugin)))
	L.SetField(claudio, "set_config", L.NewFunction(r.apiSetConfig(plugin)))
	L.SetField(claudio, "register_hook", L.NewFunction(r.apiRegisterHook(plugin)))
	L.SetField(claudio, "notify", L.NewFunction(r.apiNotify(plugin)))
	L.SetField(claudio, "log", L.NewFunction(r.apiLog(plugin)))
	L.SetField(claudio, "register_keymap", L.NewFunction(r.apiRegisterKeymap(plugin)))
	L.SetField(claudio, "register_command", L.NewFunction(r.apiRegisterCommand(plugin)))
	L.SetField(claudio, "cmd", L.NewFunction(r.apiCmd(plugin)))

	// keymap subtable: del + list (register_keymap stays at top level for compat)
	keymapTbl := L.NewTable()
	L.SetField(keymapTbl, "del", L.NewFunction(r.apiKeymapDel(plugin)))
	L.SetField(keymapTbl, "list", L.NewFunction(r.apiKeymapList(plugin)))
	L.SetField(claudio, "keymap", keymapTbl)

	L.SetGlobal("claudio", claudio)
}
