// Package lua embeds gopher-lua VMs to provide a Neovim-style plugin system.
// Each plugin runs in an isolated sandbox with a `claudio` global table exposing
// tool registration, event bus, config, hooks, and notification APIs.
package lua

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"sync"

	"github.com/Abraxas-365/claudio/internal/bus"
	"github.com/Abraxas-365/claudio/internal/capabilities"
	"github.com/Abraxas-365/claudio/internal/cli/commands"
	"github.com/Abraxas-365/claudio/internal/config"
	"github.com/Abraxas-365/claudio/internal/hooks"
	lsp "github.com/Abraxas-365/claudio/internal/services/lsp"
	"github.com/Abraxas-365/claudio/internal/services/skills"
	"github.com/Abraxas-365/claudio/internal/storage"
	"github.com/Abraxas-365/claudio/internal/tasks"
	"github.com/Abraxas-365/claudio/internal/teams"
	"github.com/Abraxas-365/claudio/internal/tools"
	keymapPkg "github.com/Abraxas-365/claudio/internal/tui/keymap"
	"github.com/Abraxas-365/claudio/internal/tui/picker"
	"github.com/Abraxas-365/claudio/internal/tui/prompt"
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

// pendingLeaderBinding queues a leader-keymap binding registered before the
// TUI's leader keymap is wired via SetLeaderKeymap.
type pendingLeaderBinding struct {
	seq      string
	actionID keymapPkg.ActionID
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

	// Branch tracking
	branchHdlrsMu sync.RWMutex
	branchHdlrs   []luaHandler

	// Pending capabilities
	pendingCapsMu sync.Mutex
	pendingCaps   []LuaCapability

	// UI extension state — protected by uiMu.
	uiMu                  sync.RWMutex
	StatuslineFn          *lua.LFunction
	statuslinePlugin      *loadedPlugin
	pendingPaletteEntries []PaletteEntry

	// Panel registry (wired after TUI is ready via SetPanelRegistry).
	// Panels queued before wiring are held in pendingPanels.
	panelRegistryMu sync.RWMutex
	panelRegistry   *PanelRegistry

	pendingPanelsMu sync.Mutex
	pendingPanels   []*PanelDef

	// Pending window registrations (registered before WindowManager is wired)
	pendingWindowsMu sync.Mutex
	pendingWindows   []WindowDef

	// Leader keymap (wired after TUI init via SetLeaderKeymap)
	leaderKeymapMu       sync.Mutex
	leaderKeymap         *keymapPkg.Keymap
	pendingLeaderMu      sync.Mutex
	pendingLeaderBindings []pendingLeaderBinding

	// leaderFnUnsubs tracks bus unsubscribe fns for Lua-function-backed leader
	// bindings, keyed by normalised seq. Protected by leaderFnUnsubsMu.
	leaderFnUnsubsMu sync.Mutex
	leaderFnUnsubs   map[string]func()

	// Picker opener (wired after TUI is ready)
	pickerOpenerMu sync.RWMutex
	pickerOpener   func(picker.Config)

	// Open-pane fn (wired after TUI is ready via SetOpenPaneFn)
	openPaneFnMu sync.RWMutex
	openPaneFn   func(agentName string)

	// Team inspection (wired after teams are initialised)
	teamRunnerMu sync.RWMutex
	teamRunner   *teams.TeammateRunner
	teamManagerMu sync.RWMutex
	teamManager  *teams.Manager

	// Prompt (wired after TUI init via SetPrompt)
	promptMu          sync.RWMutex
	prompt            *prompt.Model
	promptPlaceholder string
	promptDesiredMode string // "vim" or "simple"; empty = leave as-is

	// on_submit hooks (append-only; run in registration order)
	promptHooksMu sync.RWMutex
	promptHooks   []luaHandler

	// shutdown context — cancelled by Close() to stop in-flight Lua AI/agent calls.
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc

	// LSP server manager (wired after LSP init)
	lspManagerMu sync.RWMutex
	lspManager   *lsp.ServerManager

	// Background task runtime (wired after task runtime init)
	taskRuntimeMu sync.RWMutex
	taskRuntime   *tasks.Runtime

	// Data providers (wired after TUI init by root.go)
	sessionProviderMu sync.RWMutex
	sessionProvider   SessionProvider

	filesProviderMu sync.RWMutex
	filesProvider   FilesProvider

	tasksProviderMu sync.RWMutex
	tasksProvider   TasksProvider

	tokensProviderMu sync.RWMutex
	tokensProvider   TokensProvider

	// Lightweight AI call (no tools)
	runAICallMu sync.RWMutex
	runAICall   func(ctx context.Context, system, user, model string) (string, error)

	// Full sub-agent call (with tool whitelist)
	runAgentCallMu sync.RWMutex
	runAgentCall   func(ctx context.Context, system, prompt, model string, maxTurns int, allowedTools []string) (string, error)
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
	ctx, cancel := context.WithCancel(context.Background())
	return &Runtime{
		toolReg:        toolReg,
		skills:         skillsReg,
		bus:            eventBus,
		hooks:          hooksMgr,
		cfg:            cfg,
		db:             db,
		caps:           caps,
		leaderFnUnsubs: make(map[string]func()),
		shutdownCtx:    ctx,
		shutdownCancel: cancel,
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

// execString runs a Lua string in a persistent sandboxed state wired to the
// runtime's API. The state is kept alive (added to r.plugins) so that any
// callbacks or sidebar blocks registered during execution remain valid.
func (r *Runtime) execString(code, name string) error {
	L := newSandboxedState()
	plugin := &loadedPlugin{name: name, dir: "", L: L}
	r.injectAPI(L, plugin)
	if err := L.DoString(code); err != nil {
		L.Close()
		return fmt.Errorf("lua: exec %s: %w", name, err)
	}
	r.mu.Lock()
	r.plugins = append(r.plugins, plugin)
	r.mu.Unlock()
	return nil
}

// Close shuts down all Lua VMs, cancels in-flight AI/agent calls, and unsubscribes bus handlers.
func (r *Runtime) Close() {
	r.shutdownCancel()
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
	L.SetField(keymapTable, "map", L.NewFunction(r.apiLeaderKeymapMap(plugin)))
	L.SetField(keymapTable, "unmap", L.NewFunction(r.apiLeaderKeymapUnmap(plugin)))
	L.SetField(claudio, "keymap", keymapTable)

	// claudio.prompt sub-table
	promptTable := L.NewTable()
	L.SetField(promptTable, "set_placeholder", L.NewFunction(r.apiPromptSetPlaceholder(plugin)))
	L.SetField(promptTable, "set_mode", L.NewFunction(r.apiPromptSetMode(plugin)))
	L.SetField(promptTable, "on_submit", L.NewFunction(r.apiPromptOnSubmit(plugin)))
	L.SetField(claudio, "prompt", promptTable)

	// claudio.ai sub-table
	aiTable := L.NewTable()
	L.SetField(aiTable, "run", L.NewFunction(r.apiAIRun(plugin)))
	L.SetField(claudio, "ai", aiTable)

	// claudio.agent sub-table
	agentTable := L.NewTable()
	L.SetField(agentTable, "current", L.NewFunction(r.apiAgentCurrent(plugin)))
	L.SetField(agentTable, "on_change", L.NewFunction(r.apiAgentOnChange(plugin)))
	L.SetField(agentTable, "add_context", L.NewFunction(r.apiAgentAddContext(plugin)))
	L.SetField(agentTable, "set_prompt_suffix", L.NewFunction(r.apiAgentSetPromptSuffix(plugin)))
	L.SetField(agentTable, "list", L.NewFunction(r.apiAgentList(plugin)))
	L.SetField(agentTable, "status", L.NewFunction(r.apiAgentStatus(plugin)))
	L.SetField(agentTable, "spawn", L.NewFunction(r.apiAgentSpawn(plugin)))
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
	L.SetField(sessionTable, "messages", L.NewFunction(r.apiSessionMessages(plugin)))
	L.SetField(claudio, "session", sessionTable)

	// claudio.branch sub-table
	branchTable := L.NewTable()
	L.SetField(branchTable, "current", L.NewFunction(r.apiBranchCurrent(plugin)))
	L.SetField(branchTable, "create", L.NewFunction(r.apiBranchCreate(plugin)))
	L.SetField(branchTable, "parent", L.NewFunction(r.apiBranchParent(plugin)))
	L.SetField(branchTable, "children", L.NewFunction(r.apiBranchChildren(plugin)))
	L.SetField(branchTable, "root", L.NewFunction(r.apiBranchRoot(plugin)))
	L.SetField(branchTable, "messages", L.NewFunction(r.apiBranchMessages(plugin)))
	L.SetField(branchTable, "switch", L.NewFunction(r.apiBranchSwitch(plugin)))
	L.SetField(branchTable, "on_branch", L.NewFunction(r.apiBranchOnBranch(plugin)))
	L.SetField(claudio, "branch", branchTable)

	// claudio.sessions sub-table (list/search all sessions — NOT current session)
	sessionsTable := L.NewTable()
	L.SetField(sessionsTable, "list", L.NewFunction(r.apiSessionsList(plugin)))
	L.SetField(sessionsTable, "search", L.NewFunction(r.apiSessionsSearch(plugin)))
	L.SetField(claudio, "sessions", sessionsTable)

	// claudio.models sub-table
	modelsTable := L.NewTable()
	L.SetField(modelsTable, "list", L.NewFunction(r.apiModelsList(plugin)))
	L.SetField(claudio, "models", modelsTable)

	// claudio.commands sub-table
	commandsTable := L.NewTable()
	L.SetField(commandsTable, "list", L.NewFunction(r.apiCommandsList(plugin)))
	L.SetField(claudio, "commands", commandsTable)

	// claudio.skills sub-table
	skillsTable := L.NewTable()
	L.SetField(skillsTable, "list", L.NewFunction(r.apiSkillsList(plugin)))
	L.SetField(claudio, "skills", skillsTable)

	// claudio.windows sub-table
	windowsTable := L.NewTable()
	L.SetField(windowsTable, "list", L.NewFunction(r.apiWindowsList(plugin)))
	L.SetField(windowsTable, "read", L.NewFunction(r.apiWindowsRead(plugin)))
	L.SetField(claudio, "windows", windowsTable)

	// claudio.actions sub-table
	actionsTable := L.NewTable()
	L.SetField(actionsTable, "list", L.NewFunction(r.apiActionsList(plugin)))
	L.SetField(claudio, "actions", actionsTable)

	// claudio.ui sub-table (real impl from api_tui.go)
	L.SetField(claudio, "ui", r.injectUIAPI(L, plugin))

	// Global settings + config APIs
	r.injectGlobalConfigAPI(L, claudio)

	// Plugin-aware UI extensions (no-op; blocks replaced by claudio.win.new_panel)
	r.injectPluginUIAPI(L, plugin, claudio)

	// claudio.win — new_panel / section API
	r.injectWinAPI(L, plugin, claudio)

	// claudio.buf + claudio.ui.register_window
	r.injectWindowsAPI(L, plugin, claudio)

	// claudio.picker + claudio.finder
	r.injectPickerAPI(L, plugin, claudio)

	// claudio.filter sub-table
	r.injectFilterAPI(L, plugin, claudio)

	// claudio.lsp sub-table
	r.injectLSPAPI(L, plugin, claudio)

	// claudio.session.current, claudio.files, claudio.tasks, claudio.tokens
	r.injectDataAPIs(L, claudio)

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

// SetPanelRegistry wires the panel registry. Any panels queued before this call
// are flushed into the live registry.
func (r *Runtime) SetPanelRegistry(reg *PanelRegistry) {
	r.panelRegistryMu.Lock()
	r.panelRegistry = reg
	r.panelRegistryMu.Unlock()

	r.pendingPanelsMu.Lock()
	pending := r.pendingPanels
	r.pendingPanels = nil
	r.pendingPanelsMu.Unlock()

	for _, p := range pending {
		reg.Register(p)
	}
}

// GetPanelRegistry returns the wired PanelRegistry (nil until TUI is ready).
func (r *Runtime) GetPanelRegistry() *PanelRegistry {
	r.panelRegistryMu.RLock()
	defer r.panelRegistryMu.RUnlock()
	return r.panelRegistry
}

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

// SetPrompt wires the prompt model so claudio.prompt.* calls can mutate it.
// Any placeholder or mode set before TUI init is applied immediately.
//
// Stale-pointer risk: p must remain valid for the lifetime of the Runtime.
// Root passes &m.prompt where m is the root bubbletea Model stored on the heap;
// the address is stable as long as the root Model is not replaced. If the TUI
// ever reinitializes the root Model (e.g. a full restart without process exit),
// SetPrompt must be called again with the new address before any Lua prompt.*
// calls can safely reach the live prompt.
func (r *Runtime) SetPrompt(p *prompt.Model) {
	r.promptMu.Lock()
	defer r.promptMu.Unlock()
	r.prompt = p
	if r.promptPlaceholder != "" {
		p.SetPlaceholder(r.promptPlaceholder)
	}
	if r.promptDesiredMode != "" {
		applyPromptMode(p, r.promptDesiredMode)
	}
}

// applyPromptMode enables or disables vim mode on the prompt.
func applyPromptMode(p *prompt.Model, mode string) {
	switch mode {
	case "vim":
		if !p.IsVimEnabled() {
			p.ToggleVim()
		}
	case "simple":
		if p.IsVimEnabled() {
			p.ToggleVim()
		}
	}
}

// SetLeaderKeymap wires the TUI's leader keymap so that claudio.keymap.map/unmap
// calls take effect immediately. Any bindings registered before this call (e.g.
// from defaults.lua) are flushed now. Lua defaults only apply if the user has
// not already set a binding for that sequence via keymap.json.
func (r *Runtime) SetLeaderKeymap(km *keymapPkg.Keymap) {
	r.leaderKeymapMu.Lock()
	r.leaderKeymap = km
	r.leaderKeymapMu.Unlock()

	r.pendingLeaderMu.Lock()
	pending := r.pendingLeaderBindings
	r.pendingLeaderBindings = nil
	r.pendingLeaderMu.Unlock()

	for _, pb := range pending {
		// Don't overwrite bindings the user has already saved via :map / keymap.json.
		if _, already := km.Resolve(pb.seq); !already {
			km.SetCustom(pb.seq, pb.actionID)
		}
	}
}

// SetPickerOpener wires the opener callback so Lua plugins can open picker overlays.
// The fn is called with a picker.Config whenever Lua requests a picker open.
func (r *Runtime) SetPickerOpener(fn func(picker.Config)) {
	r.pickerOpenerMu.Lock()
	defer r.pickerOpenerMu.Unlock()
	r.pickerOpener = fn
}

// SetOpenPaneFn wires the open-pane callback so Lua plugins can spawn new agent panes.
// agentName is the agent persona to apply; empty string = default Claudio persona.
func (r *Runtime) SetOpenPaneFn(fn func(agentName string)) {
	r.openPaneFnMu.Lock()
	defer r.openPaneFnMu.Unlock()
	r.openPaneFn = fn
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

// SetLSPManager wires the LSP ServerManager so Lua plugins can register and control LSP servers.
func (r *Runtime) SetLSPManager(mgr *lsp.ServerManager) {
	r.lspManagerMu.Lock()
	defer r.lspManagerMu.Unlock()
	r.lspManager = mgr
}

// SetTaskRuntime wires the background task runtime so Lua plugins can list/kill tasks.
func (r *Runtime) SetTaskRuntime(rt *tasks.Runtime) {
	r.taskRuntimeMu.Lock()
	defer r.taskRuntimeMu.Unlock()
	r.taskRuntime = rt
}

// SetSessionProvider wires the session data provider for claudio.session.current().
func (r *Runtime) SetSessionProvider(p SessionProvider) {
	r.sessionProviderMu.Lock()
	defer r.sessionProviderMu.Unlock()
	r.sessionProvider = p
}

// SetFilesProvider wires the files data provider for claudio.files.list().
func (r *Runtime) SetFilesProvider(p FilesProvider) {
	r.filesProviderMu.Lock()
	defer r.filesProviderMu.Unlock()
	r.filesProvider = p
}

// SetTasksProvider wires the tasks data provider for claudio.tasks.list().
func (r *Runtime) SetTasksProvider(p TasksProvider) {
	r.tasksProviderMu.Lock()
	defer r.tasksProviderMu.Unlock()
	r.tasksProvider = p
}

// SetTokensProvider wires the tokens data provider for claudio.tokens.usage().
func (r *Runtime) SetTokensProvider(p TokensProvider) {
	r.tokensProviderMu.Lock()
	defer r.tokensProviderMu.Unlock()
	r.tokensProvider = p
}

// SetRunAICall wires the lightweight AI call callback for claudio.ai.run().
func (r *Runtime) SetRunAICall(fn func(ctx context.Context, system, user, model string) (string, error)) {
	r.runAICallMu.Lock()
	defer r.runAICallMu.Unlock()
	r.runAICall = fn
}

// SetRunAgentCall wires the sub-agent call callback for claudio.agent.spawn().
func (r *Runtime) SetRunAgentCall(fn func(ctx context.Context, system, prompt, model string, maxTurns int, allowedTools []string) (string, error)) {
	r.runAgentCallMu.Lock()
	defer r.runAgentCallMu.Unlock()
	r.runAgentCall = fn
}
