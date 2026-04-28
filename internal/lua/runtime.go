// Package lua embeds gopher-lua VMs to provide a Neovim-style plugin system.
// Each plugin runs in an isolated sandbox with a `claudio` global table exposing
// tool registration, event bus, config, hooks, and notification APIs.
package lua

import (
	_ "embed"
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
	"github.com/Abraxas-365/claudio/internal/teams"
	"github.com/Abraxas-365/claudio/internal/tools"
	"github.com/Abraxas-365/claudio/internal/tui/picker"
	"github.com/Abraxas-365/claudio/internal/tui/vim"
	"github.com/Abraxas-365/claudio/internal/tui/windows"
	lua "github.com/yuin/gopher-lua"
)

//go:embed defaults.lua
var defaultsLua string

// luaHandler pairs a plugin's Lua VM with a registered Lua function for safe callbacks.
type luaHandler struct {
	plugin *loadedPlugin
	fn     *lua.LFunction
}

// LuaCapability holds a capability name and tool names registered by a Lua plugin.
type LuaCapability struct {
	Name      string
	ToolNames []string
}

// pendingCommand queues a command registered before commandRegistry is wired.
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

	mu      sync.Mutex
	plugins []*loadedPlugin

	// Config change handlers
	changeHandlersMu sync.RWMutex
	changeHandlers   map[string][]configChangeHandler

	// Command registry (wired after TUI/CLI init)
	commandRegistryMu sync.Mutex
	commandRegistry   *commands.Registry

	// Keymap registry (wired after TUI init)
	keymapRegistryMu sync.Mutex
	keymapRegistry   *vim.KeymapRegistry

	// Window manager (wired after TUI init)
	windowManagerMu sync.RWMutex
	windowManager   *windows.Manager

	// Pending commands (registered before registry is wired)
	pendingCommandsMu sync.Mutex
	pendingCommands   []pendingCommand

	// Pending providers
	pendingProvidersMu sync.Mutex
	pendingProviders   []LuaProviderConfig

	// Agent tracking
	agentMu          sync.RWMutex
	currentAgent     string
	agentChangeHdlrs []luaHandler
	extraContextMu   sync.RWMutex
	extraContext     []string
	promptSuffixMu   sync.RWMutex
	promptSuffixHdlr *luaHandler

	// Session tracking
	sessionMu           sync.RWMutex
	currentSessionID    string
	currentSessionTitle string
	sessionStartHdlrs   []luaHandler
	sessionEndHdlrs     []luaHandler
	messageHdlrs        []luaHandler

	// Pending capabilities
	pendingCapsMu sync.Mutex
	pendingCaps   []LuaCapability

	// UI extension state — protected by uiMu.
	uiMu                  sync.RWMutex
	StatuslineFn          *lua.LFunction
	statuslinePlugin      *loadedPlugin
	pendingWhichkeyGroups []WhichkeyGroup
	pendingPaletteEntries []PaletteEntry

	// Pending sidebar blocks (registered before TUI is wired)
	pendingSidebarBlocksMu sync.Mutex
	pendingSidebarBlocks   []SidebarBlockDef

	// Pending window registrations (registered before WindowManager is wired)
	pendingWindowsMu sync.Mutex
	pendingWindows   []WindowDef

	// Picker opener (wired after TUI is ready)
	pickerOpenerMu sync.RWMutex
	pickerOpener   func(picker.Config)

	// Team inspection (wired after teams are initialised)
	teamRunnerMu sync.RWMutex
	teamRunner   *teams.TeammateRunner
	teamManagerMu sync.RWMutex
	teamManager  *teams.Manager
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

// LoadDefaults executes the embedded defaults.lua, setting initial config values
// on the Runtime's Settings before user init or plugins run.
func (r *Runtime) LoadDefaults() error {
	return r.execString(defaultsLua, "defaults.lua")
}

// ExecString runs a Lua string in a transient sandboxed state — public API for :lua REPL.
// Returns any string value left on the stack and any execution error.
func (r *Runtime) ExecString(code string) (string, error) {
	L := newSandboxedState()
	defer L.Close()
	r.injectAPI(L, &loadedPlugin{name: "<repl>"})
	if err := L.DoString(code); err != nil {
		return "", err
	}
	// Return top-of-stack string if present
	if top := L.GetTop(); top > 0 {
		if s, ok := L.Get(-1).(lua.LString); ok {
			return string(s), nil
		}
	}
	return "", nil
}

// execString runs a Lua string in a transient sandboxed state wired to the
// runtime's API. The state is closed after execution; it is NOT added to
// r.plugins. Use this for one-shot init scripts (defaults, user init).
func (r *Runtime) execString(code, name string) error {
	L := newSandboxedState()
	defer L.Close()
	dummy := &loadedPlugin{name: name, dir: ""}
	r.injectAPI(L, dummy)
	if err := L.DoString(code); err != nil {
		return fmt.Errorf("lua: exec %s: %w", name, err)
	}
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
	L.SetField(claudio, "register_provider", L.NewFunction(r.apiRegisterProvider(plugin)))
	L.SetField(claudio, "register_capability", L.NewFunction(r.apiRegisterCapability(plugin)))

	// claudio.keymap sub-table
	keymapTable := L.NewTable()
	L.SetField(keymapTable, "del", L.NewFunction(r.apiKeymapDel(plugin)))
	L.SetField(keymapTable, "list", L.NewFunction(r.apiKeymapList(plugin)))
	L.SetField(claudio, "keymap", keymapTable)

	// claudio.agent sub-table
	agentTable := L.NewTable()
	L.SetField(agentTable, "current", L.NewFunction(r.apiAgentCurrent(plugin)))
	L.SetField(agentTable, "on_change", L.NewFunction(r.apiAgentOnChange(plugin)))
	L.SetField(agentTable, "add_context", L.NewFunction(r.apiAgentAddContext(plugin)))
	L.SetField(agentTable, "set_prompt_suffix", L.NewFunction(r.apiAgentSetPromptSuffix(plugin)))
	L.SetField(agentTable, "list", L.NewFunction(r.apiAgentList(plugin)))
	L.SetField(agentTable, "status", L.NewFunction(r.apiAgentStatus(plugin)))
	L.SetField(claudio, "agent", agentTable)

	// claudio.teams sub-table
	teamsTable := L.NewTable()
	L.SetField(teamsTable, "list", L.NewFunction(r.apiTeamsList(plugin)))
	L.SetField(teamsTable, "members", L.NewFunction(r.apiTeamsMembers(plugin)))
	L.SetField(claudio, "teams", teamsTable)

	// claudio.session sub-table
	sessionTable := L.NewTable()
	L.SetField(sessionTable, "id", L.NewFunction(r.apiSessionID(plugin)))
	L.SetField(sessionTable, "title", L.NewFunction(r.apiSessionTitle(plugin)))
	L.SetField(sessionTable, "on_start", L.NewFunction(r.apiSessionOnStart(plugin)))
	L.SetField(sessionTable, "on_end", L.NewFunction(r.apiSessionOnEnd(plugin)))
	L.SetField(sessionTable, "on_message", L.NewFunction(r.apiSessionOnMessage(plugin)))
	L.SetField(claudio, "session", sessionTable)

	// claudio.ui sub-table (real impl from api_tui.go)
	L.SetField(claudio, "ui", r.injectUIAPI(L, plugin))

	// Global settings + config APIs
	r.injectGlobalConfigAPI(L, claudio)

	// Plugin-aware UI extensions (sidebar blocks, etc.)
	r.injectPluginUIAPI(L, plugin, claudio)

	// claudio.buf + claudio.ui.register_window
	r.injectWindowsAPI(L, plugin, claudio)

	// claudio.picker + claudio.finder
	r.injectPickerAPI(L, plugin, claudio)

	L.SetGlobal("claudio", claudio)
}

// SetCommandRegistry wires the command registry and flushes pending commands.
func (r *Runtime) SetCommandRegistry(reg *commands.Registry) {
	r.commandRegistryMu.Lock()
	defer r.commandRegistryMu.Unlock()
	r.commandRegistry = reg
	r.pendingCommandsMu.Lock()
	defer r.pendingCommandsMu.Unlock()
	for _, p := range r.pendingCommands {
		reg.Register(p.cmd)
	}
	r.pendingCommands = nil
}

// GetSidebarBlocks returns a snapshot of all sidebar blocks registered by plugins.

// SetKeymapRegistry wires the keymap registry.
func (r *Runtime) SetKeymapRegistry(reg *vim.KeymapRegistry) {
	r.keymapRegistryMu.Lock()
	defer r.keymapRegistryMu.Unlock()
	r.keymapRegistry = reg
}

// SetWindowManager wires the window manager and flushes any pending window registrations.
func (r *Runtime) SetWindowManager(wm *windows.Manager) {
	r.windowManagerMu.Lock()
	defer r.windowManagerMu.Unlock()
	r.windowManager = wm
	r.pendingWindowsMu.Lock()
	defer r.pendingWindowsMu.Unlock()
	for _, def := range r.pendingWindows {
		wm.Register(def.Window)
	}
	r.pendingWindows = nil
}

// GetWindowManager returns the wired window manager (nil until TUI is ready).
func (r *Runtime) GetWindowManager() *windows.Manager {
	r.windowManagerMu.RLock()
	defer r.windowManagerMu.RUnlock()
	return r.windowManager
}

// SetPickerOpener wires the opener callback so Lua plugins can open picker overlays.
// The fn is called with a picker.Config whenever Lua requests a picker open.
func (r *Runtime) SetPickerOpener(fn func(picker.Config)) {
	r.pickerOpenerMu.Lock()
	defer r.pickerOpenerMu.Unlock()
	r.pickerOpener = fn
}

// SetTeamRunner wires the TeammateRunner so Lua plugins can inspect agent state.
func (r *Runtime) SetTeamRunner(runner *teams.TeammateRunner) {
	r.teamRunnerMu.Lock()
	defer r.teamRunnerMu.Unlock()
	r.teamRunner = runner
}

// SetTeamManager wires the team Manager so Lua plugins can inspect team configuration.
func (r *Runtime) SetTeamManager(mgr *teams.Manager) {
	r.teamManagerMu.Lock()
	defer r.teamManagerMu.Unlock()
	r.teamManager = mgr
}
