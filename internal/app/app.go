package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Abraxas-365/claudio/internal/agents"
	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/attach"
	"github.com/Abraxas-365/claudio/internal/api/provider"
	"github.com/Abraxas-365/claudio/internal/auth"
	"github.com/Abraxas-365/claudio/internal/capabilities"
	authstorage "github.com/Abraxas-365/claudio/internal/auth/storage"
	"github.com/Abraxas-365/claudio/internal/bus"
	"github.com/Abraxas-365/claudio/internal/config"
	"github.com/Abraxas-365/claudio/internal/harness"
	"github.com/Abraxas-365/claudio/internal/hooks"
	luart "github.com/Abraxas-365/claudio/internal/lua"
	"github.com/Abraxas-365/claudio/internal/tui/windows"
	"github.com/Abraxas-365/claudio/internal/learning"
	"github.com/Abraxas-365/claudio/internal/models"
	"github.com/Abraxas-365/claudio/internal/plugins"
	"github.com/Abraxas-365/claudio/internal/prompts"
	"github.com/Abraxas-365/claudio/internal/query"
	"github.com/Abraxas-365/claudio/internal/services/lsp"
	"github.com/Abraxas-365/claudio/internal/security"
	"github.com/Abraxas-365/claudio/internal/services/analytics"
	"github.com/Abraxas-365/claudio/internal/services/filtersavings"
	"github.com/Abraxas-365/claudio/internal/services/memory"
	"github.com/Abraxas-365/claudio/internal/services/mcp"
	"github.com/Abraxas-365/claudio/internal/services/skills"
	"github.com/Abraxas-365/claudio/internal/storage"
	"github.com/Abraxas-365/claudio/internal/tasks"
	"github.com/Abraxas-365/claudio/internal/teams"
	"github.com/Abraxas-365/claudio/internal/tools"
)

// ccSendRef is a thread-safe holder for a tools.AttachClient used to forward
// sub-agent messages to ComandCenter (cc_messages) via the attach WebSocket.
// It is created during App.New() and set later when InjectAttachClient is called.
type ccSendRef struct {
	mu  sync.RWMutex
	cli tools.AttachClient
}

func (r *ccSendRef) set(c tools.AttachClient) {
	r.mu.Lock()
	r.cli = c
	r.mu.Unlock()
}

func (r *ccSendRef) get() tools.AttachClient {
	r.mu.RLock()
	c := r.cli
	r.mu.RUnlock()
	return c
}

// Private context keys for cc sender injection into sub-agent context.
type ctxKeyCCSend struct{}

func withCCSend(ctx context.Context, ref *ccSendRef) context.Context {
	return context.WithValue(ctx, ctxKeyCCSend{}, ref)
}

func ccSendFromCtx(ctx context.Context) *ccSendRef {
	if v, ok := ctx.Value(ctxKeyCCSend{}).(*ccSendRef); ok {
		return v
	}
	return nil
}

// App holds all shared application dependencies.
type App struct {
	Config    *config.Settings
	Profile   string
	Bus       *bus.Bus
	Storage   authstorage.SecureStorage
	Auth      *auth.Resolver
	API       *api.Client
	DB        *storage.DB
	Tools     *tools.Registry
	Hooks     *hooks.Manager
	Learning  *learning.Store
	Skills    *skills.Registry
	Memory    *memory.ScopedStore
	Analytics     *analytics.Tracker
	FilterSavings *filtersavings.Service
	Auditor       *security.Auditor
	TaskRuntime  *tasks.Runtime
	Teams        *teams.Manager
	TeamRunner   *teams.TeammateRunner
	Plugins      *plugins.Registry
	Cron         *tasks.CronStore
	CronRunner   *tasks.CronRunner
	LSP          *lsp.ServerManager
	MCPManager          *mcp.Manager
	Capabilities        *capabilities.Registry
	LuaRuntime          *luart.Runtime
	WindowManager       *windows.Manager
	HarnessTemplateDirs []string
	InjectCh            chan attach.UserMsgPayload
	InterruptCh         chan struct{}

	// ccSend forwards sub-agent messages to ComandCenter cc_messages via attach WS.
	// Nil until InjectAttachClient is called (i.e. only in --attach mode).
	ccSend *ccSendRef
}

// SecurityContext wraps config-based security settings for tool injection.
type SecurityContext struct {
	DenyPaths    []string
	AllowPaths   []string
	DenyCommands []string
}

// CheckPath validates file access.
func (s *SecurityContext) CheckPath(path string) error {
	return security.CheckPathAccess(path, s.DenyPaths, s.AllowPaths)
}

// CheckCommand validates shell commands.
func (s *SecurityContext) CheckCommand(cmd string) error {
	return security.CheckCommandSafety(cmd, s.DenyCommands)
}

// New creates a new App with all dependencies wired up.
// projectRoot is the git root (or cwd) used for project-scoped memory.
// profile selects the auth profile; empty string uses the active profile from config.
func New(settings *config.Settings, projectRoot string, profile ...string) (*App, error) {
	if err := config.EnsureDirs(); err != nil {
		return nil, err
	}

	activeProfile := ""
	if len(profile) > 0 && profile[0] != "" {
		activeProfile = profile[0]
	} else {
		activeProfile = config.GetActiveProfile()
	}

	eventBus := bus.New()
	store := authstorage.NewDefaultStorage(activeProfile)
	resolver := auth.NewResolver(store)

	// Open SQLite database
	db, err := storage.Open(config.GetPaths().DB)
	if err != nil {
		return nil, err
	}

	var apiOpts []api.ClientOption
	apiOpts = append(apiOpts, api.WithStorage(store))
	if settings.APIBaseURL != "" {
		apiOpts = append(apiOpts, api.WithBaseURL(settings.APIBaseURL))
	}
	if settings.Model != "" {
		apiOpts = append(apiOpts, api.WithModel(settings.Model))
	}

	apiClient := api.NewClient(resolver, apiOpts...)

	// Register multi-provider routes
	for name, pc := range settings.Providers {
		apiKey := pc.APIKey
		if strings.HasPrefix(apiKey, "$") {
			apiKey = os.Getenv(apiKey[1:])
		}
		var p api.Provider
		switch pc.Type {
		case "openai":
			op := provider.NewOpenAI(name, pc.APIBase, apiKey)
			if pc.ContextWindow > 0 {
				op.WithNumCtx(pc.ContextWindow)
			}
			p = op
		case "anthropic":
			p = provider.NewAnthropic(pc.APIBase, apiKey)
		case "ollama":
			// Native Ollama provider — uses /api/chat (not /v1/chat/completions)
			// because Ollama's OpenAI-compat endpoint silently drops `options`,
			// preventing num_ctx from being set (defaults to 2048 → context loss).
			olp := provider.NewOllama(name, pc.APIBase)
			if pc.ContextWindow > 0 {
				olp.WithNumCtx(pc.ContextWindow)
			}
			if len(pc.NoToolsModels) > 0 {
				olp.WithNoToolsModels(pc.NoToolsModels)
			}
			p = olp
		default:
			// Default to openai-compatible
			op := provider.NewOpenAI(name, pc.APIBase, apiKey)
			if pc.ContextWindow > 0 {
				op.WithNumCtx(pc.ContextWindow)
			}
			p = op
		}
		apiClient.RegisterProvider(name, p)
		// Register model shortcuts from provider config
		for shortcut, modelID := range pc.Models {
			apiClient.AddModelShortcut(shortcut, modelID)
		}
	}
	for pattern, provName := range settings.ModelRouting {
		apiClient.AddModelRoute(pattern, provName)
	}

	// Apply thinking and effort settings from config
	if settings.ThinkingMode != "" {
		apiClient.SetThinkingMode(settings.ThinkingMode)
	}
	if settings.BudgetTokens > 0 {
		apiClient.SetBudgetTokens(settings.BudgetTokens)
	}
	if settings.EffortLevel != "" {
		apiClient.SetEffortLevel(settings.EffortLevel)
	}

	// Register core tools with security
	registry := tools.DefaultRegistry()

	// Create security context from config
	sec := &SecurityContext{
		DenyPaths:  settings.DenyPaths,
		AllowPaths: settings.AllowPaths,
	}

	// Initialize filter savings analytics service
	filterSvc := filtersavings.NewService(db)

	// Inject security into file/shell tools
	if bash, err := registry.Get("Bash"); err == nil {
		if bt, ok := bash.(*tools.BashTool); ok {
			bt.Security = sec
			bt.OutputFilterEnabled = settings.OutputFilter
			if settings.OutputFilter {
				bt.FilterRecorder = func(cmd string, bytesIn, bytesOut int) {
					_ = filterSvc.Record(context.Background(), cmd, bytesIn, bytesOut)
				}
			}
		}
	}
	if read, err := registry.Get("Read"); err == nil {
		if rt, ok := read.(*tools.FileReadTool); ok {
			rt.Security = sec
			rt.Config = settings
		}
	}
	if write, err := registry.Get("Write"); err == nil {
		if wt, ok := write.(*tools.FileWriteTool); ok {
			wt.Security = sec
		}
	}
	if edit, err := registry.Get("Edit"); err == nil {
		if et, ok := edit.(*tools.FileEditTool); ok {
			et.Security = sec
		}
	}

	// Configure snippet expansion on Write/Edit tools
	registry.SetSnippetConfig(settings.Snippets)

	// Inject event bus into task tools
	registry.SetBus(eventBus)

	// Inject SmallModel client into TaskUpdate for verification nudge
	registry.SetSmallModelClient(apiClient, settings.SmallModel)

	// Remove denied tools
	for _, denied := range settings.DenyTools {
		registry.Remove(denied)
	}

	paths := config.GetPaths()
	cwd, _ := os.Getwd()

	// Discover installed harnesses (.claudio/harnesses/<name>/harness.json)
	harnessesDir := filepath.Join(cwd, ".claudio", "harnesses")
	harnesses, _ := harness.DiscoverHarnesses(harnessesDir)

	// Merge harness MCP servers into settings (project settings take priority on name collision)
	if harnessMCP, err := harness.CollectMCPServers(harnesses); err == nil {
		for name, hcfg := range harnessMCP {
			if _, exists := settings.MCPServers[name]; !exists {
				if settings.MCPServers == nil {
					settings.MCPServers = make(map[string]config.MCPServerConfig)
				}
				settings.MCPServers[name] = config.MCPServerConfig{
					Command: hcfg.Command,
					Args:    hcfg.Args,
					Env:     hcfg.Env,
					Type:    hcfg.Type,
					URL:     hcfg.URL,
				}
			}
		}
	} else {
		fmt.Fprintf(os.Stderr, "Warning: harness MCP server conflict: %v\n", err)
	}

	// Initialize LSP server manager from settings + plugin configs
	lspCfgs := make(map[string]config.LspServerConfig)
	for k, v := range settings.LspServers {
		lspCfgs[k] = v
	}
	// Merge plugin-provided LSP configs (settings take priority)
	pluginLspCfgs := plugins.LoadLspConfigs(paths.Plugins)
	for k, v := range pluginLspCfgs {
		if _, exists := lspCfgs[k]; !exists {
			lspCfgs[k] = v
		}
	}
	// Merge Lua LSP configs from ~/.claudio/lsp/*.lua (takes priority over *.lsp.json)
	luaLspCfgs := plugins.LoadLuaLspConfigs(paths.LSP)
	for k, v := range luaLspCfgs {
		lspCfgs[k] = v
	}
	lspManager := lsp.NewServerManager(lspCfgs)
	registry.SetLSPManager(lspManager)

	// Load hooks
	hooksMgr := hooks.LoadFromSettings(paths.Settings, paths.Local)

	// Load learning store
	learningStore := learning.NewStore(paths.Instincts)
	learningStore.Decay() // prune stale instincts

	// Load skills (bundled → user → project → harness)
	skillsRegistry := skills.LoadAll(paths.Skills, cwd+"/.claudio/skills", harness.CollectSkillDirs(harnesses)...)

	// Inject skills registry into SkillTool so all agents (main + teammates) can
	// auto-detect and invoke skills. Clone() propagates the pointer to sub-agents.
	if st, err := registry.Get("Skill"); err == nil {
		if skillTool, ok := st.(*tools.SkillTool); ok {
			skillTool.SkillsRegistry = skillsRegistry
			skillTool.HooksManager = hooksMgr
			skillTool.ProjectRoot = cwd
			if settings.CavemanEnabled() {
				skillTool.ExcludedNames = []string{"caveman"}
			}
		}
	}

	// Register custom agent directories so GetAgent() can discover them (user, project, harnesses)
	{
		// Priority order (last-write-wins in typeMap): harness < global < project-local
		agentDirs := append(harness.CollectAgentDirs(harnesses), paths.Agents, cwd+"/.claudio/agents")
		agents.SetCustomDirs(agentDirs...)
	}

	// Load memory (project-scoped + global fallback)
	projectMemDir := ""
	if projectRoot != "" {
		projectMemDir = config.ProjectMemoryDir(projectRoot)
	}
	memoryStore := memory.NewScopedStore(projectMemDir, paths.Memory, db.Conn())
	memoryStore.SyncFTS()

	// Model capabilities cache (check for user-provided cache, fallback to defaults)
	modelCache := models.LoadCache(filepath.Join(paths.Cache, "model-capabilities.json"))
	if modelCache.MaxContext("claude-opus-4-6") == 0 {
		modelCache = models.NewDefaultCache()
	}
	models.SetGlobalCache(modelCache)

	// Analytics tracker
	analyticsTracker := analytics.NewTracker(settings.Model, settings.MaxBudget, paths.Home+"/analytics")

	// Auditor
	auditor := security.NewAuditor(db, eventBus)

	// Task runtime for background execution
	taskRuntime := tasks.NewRuntime(paths.Home + "/task-output")

	// Plugins — discover and register as tools (user, project, harnesses)
	pluginReg := plugins.NewRegistry()
	pluginReg.LoadDir(paths.Plugins)
	pluginReg.LoadDir(cwd + "/.claudio/plugins")
	for _, pdir := range harness.CollectPluginDirs(harnesses) {
		pluginReg.LoadDir(pdir)
	}
	for _, p := range pluginReg.All() {
		pt := plugins.NewProxyTool(p)
		pt.OutputFilterEnabled = settings.OutputFilter
		if settings.OutputFilter {
			pt.FilterRecorder = func(cmd string, bytesIn, bytesOut int) {
				_ = filterSvc.Record(context.Background(), cmd, bytesIn, bytesOut)
			}
		}
		registry.Register(pt)
	}

	// Cron store + runner
	cronStore := tasks.NewCronStore(filepath.Join(paths.Home, "cron.json"))
	cronStore.Load()

	cronRunner := tasks.NewCronRunner(cronStore)
	cronRunner.ResolveModelFn = func(agentName string) (string, string) {
		// Try model shorthand first
		if modelID := resolveModelAlias(agentName); modelID != agentName {
			return modelID, ""
		}
		// Try loading as agent definition
		agentDef := agents.GetAgent(agentName)
		model := agentDef.Model
		if model == "" {
			model = "claude-sonnet-4-6"
		} else {
			model = resolveModelAlias(model)
		}
		return model, agentDef.SystemPrompt
	}

	// ccSend is a shared mutable reference for forwarding sub-agent messages to cc_messages.
	// Created here so the teamRunner closures can capture it; set later by InjectAttachClient.
	ccSend := &ccSendRef{}

	// Team manager (primary templates dir = user, then project-scoped, then harness dirs)
	var teamMgr *teams.Manager
	{
		// Priority order (first-match wins): project-local > global > harness
		templatesDirs := []string{
			filepath.Join(cwd, ".claudio", "team-templates"),
			paths.TeamTemplates,
		}
		templatesDirs = append(templatesDirs, harness.CollectTemplateDirs(harnesses)...)
		teamMgr = teams.NewManager(paths.Home+"/teams", templatesDirs...)
	}

	// Wire team-active check into SkillTool so team-gated skills (e.g. batch) only
	// appear when a team is active.
	if st, err := registry.Get("Skill"); err == nil {
		if skillTool, ok := st.(*tools.SkillTool); ok {
			skillTool.HasActiveTeam = func() bool {
				return teamMgr != nil && teamMgr.HasActiveTeam()
			}
		}
	}

	// Message injection channel for headless mode
	injectCh := make(chan attach.UserMsgPayload, 8)
	interruptCh := make(chan struct{}, 1)

	// Build sub-agent engine config (caveman injection mirrors main agent path).
	subAgentCfg := query.EngineConfig{}
	if settings.CavemanEnabled() {
		if c := skills.BundledSkillContent("caveman"); c != "" {
			subAgentCfg.CavemanMsg = "**CAVEMAN ULTRA MODE ACTIVE — respond in caveman ultra for the entire session. Active for all agents and sub-agents. Only the human user can disable with \"stop caveman\" or \"normal mode\".**\n\n" + c + "\n\nLevel: ultra.\n\n**EXCEPTION — structured protocol output:** Always use exact format for `### Done` completion reports (exact header, all required bullet fields). Caveman style inside the fields is fine. Never skip or rename the header."
		}
	}

	// Team runner (uses the same runSubAgent callback)
	teamRunner := teams.NewTeammateRunner(teamMgr, func(ctx context.Context, system, prompt string) (string, error) {
		return runSubAgentWithMemory(ctx, apiClient, registry, system, prompt, "", subAgentCfg, eventBus)
	})
	teamRunner.Settings = settings
	teamRunner.SetBus(eventBus)
	// Inject plugin instructions so sub-agents know to prefer plugin tools over Grep/Glob/Read.
	if len(pluginReg.All()) > 0 {
		var pluginInfos []prompts.PluginInfo
		for _, p := range pluginReg.All() {
			pluginInfos = append(pluginInfos, prompts.PluginInfo{
				Name:         p.Name,
				Description:  p.Description,
				Instructions: p.Instructions,
			})
		}
		teamRunner.PluginsSection = prompts.PluginsSection(pluginInfos)
	}

	// Memory-aware runner: used when a teammate is backed by a crystallized
	// agent with its own memory directory. Lets reusable agents carry their
	// accumulated memory into team work.
	teamRunner.SetRunAgentWithMemory(func(ctx context.Context, system, prompt, memoryDir string) (string, error) {
		return runSubAgentWithMemory(ctx, apiClient, registry, system, prompt, memoryDir, subAgentCfg, eventBus)
	})

	// Revive callback: continues an existing agent conversation by restoring
	// engine history via context and running the new message as the next user
	// turn. Memory dir is honored if the state was backed by a crystallized agent.
	teamRunner.SetRunAgentResume(func(ctx context.Context, system, memoryDir string, history []api.Message, newMessage string) (string, error) {
		ctx = teams.WithResumeHistory(ctx, history)
		return runSubAgentWithMemory(ctx, apiClient, registry, system, newMessage, memoryDir, subAgentCfg, eventBus)
	})

	// Wire per-teammate context decorator: injects a SubAgentObserver that
	// populates TeammateState.Conversation and Progress in real time.
	teamRunner.SetContextDecorator(func(ctx context.Context, state *teams.TeammateState) context.Context {
		obs := &teammateObserver{state: state, runner: teamRunner}
		ctx = tools.WithSubAgentObserver(ctx, obs)
		ctx = tools.WithTeamContext(ctx, tools.TeamContext{
			TeamName:   state.TeamName,
			AgentName:  state.Identity.AgentName,
			Foreground: state.Foreground,
		})
		// Inject cc sender so runSubAgentWithMemory can forward sub-agent messages
		// to cc_messages via the attach WebSocket (only effective in --attach mode).
		ctx = withCCSend(ctx, ccSend)
		// Propagate model override so runSubAgentWithMemory picks it up
		if state.Model != "" {
			ctx = tools.WithSubAgentModel(ctx, state.Model)
		}
		// Propagate maxTurns if specified
		if state.MaxTurns > 0 {
			ctx = tools.WithMaxTurns(ctx, state.MaxTurns)
		}
		// Propagate auto-compact threshold if specified
		if state.AutoCompactThreshold > 0 {
			ctx = tools.WithCompactThreshold(ctx, state.AutoCompactThreshold)
		}
		// Team members are depth 1; their Explore sub-agents will be depth 2
		ctx = tools.WithAgentDepth(ctx, 1)
		// Inject advisor tool when the member was spawned with an advisor config.
		if state.AdvisorConfig != nil {
			var advisorSystemPrompt string
			var advisorModel string
			if state.AdvisorConfig.SubagentType != "" {
				advisorDef := agents.GetAgent(state.AdvisorConfig.SubagentType)
				advisorSystemPrompt = advisorDef.SystemPrompt + "\n\n" + prompts.AdvisorSystemPrompt()
				advisorModel = state.AdvisorConfig.Model
				if advisorModel == "" {
					advisorModel = advisorDef.Model
				}
			} else {
				advisorSystemPrompt = prompts.AdvisorSystemPrompt()
				advisorModel = state.AdvisorConfig.Model
			}
			count := 0
			advisorTool := tools.NewAdvisorTool(tools.AdvisorToolConfig{
				Definition:  agents.AgentDefinition{SystemPrompt: advisorSystemPrompt},
				Model:       advisorModel,
				MaxUses:     state.AdvisorConfig.MaxUses,
				UsedCount:   &count,
				GetMessages: state.GetEngineMessages,
				Client:      apiClient,
			})
			ctx = tools.WithExtraTool(ctx, advisorTool)
		}
		return ctx
	})

	// Wire CWD injector for worktree isolation.
	// Both the worktree path and the main repo root are stored in context so
	// file tools (Read, Edit, Write) can remap absolute main-repo paths into
	// the equivalent paths inside the agent's worktree.
	teamRunner.SetCwdInjector(func(ctx context.Context, worktreePath, mainRoot string) context.Context {
		ctx = tools.WithCwd(ctx, worktreePath)
		ctx = tools.WithMainRoot(ctx, mainRoot)
		return ctx
	})

	// Persist tasks to SQLite so they survive restarts
	tools.GlobalTaskStore.SetDB(db.Conn())

	// Wire task completer for auto-updating tasks when agents finish
	teamRunner.SetTaskCompleter(func(taskIDs []string, status, sessionID string) {
		tools.GlobalTaskStore.CompleteByIDs(taskIDs, status, sessionID)
	})

	// Inject task runtime into tools that support background execution
	if bash, err := registry.Get("Bash"); err == nil {
		if bt, ok := bash.(*tools.BashTool); ok {
			bt.TaskRuntime = taskRuntime
		}
	}
	if agent, err := registry.Get("Agent"); err == nil {
		if at, ok := agent.(*tools.AgentTool); ok {
			at.TaskRuntime = taskRuntime
			at.ParentRegistry = registry
			at.TeamRunner = teamRunner
			at.EventBus = eventBus
			// Wire available models: Anthropic aliases + provider shortcuts
			at.AvailableModels = buildAvailableModels(apiClient)
			// Wire real sub-agent execution
			at.RunAgent = func(ctx context.Context, system, prompt string) (string, error) {
				return runSubAgentWithMemory(ctx, apiClient, registry, system, prompt, "", subAgentCfg, eventBus)
			}
			at.RunAgentWithMemory = func(ctx context.Context, system, prompt, memoryDir string) (string, error) {
				return runSubAgentWithMemory(ctx, apiClient, registry, system, prompt, memoryDir, subAgentCfg, eventBus)
			}
		}
	}
	if stop, err := registry.Get("TaskStop"); err == nil {
		if st, ok := stop.(*tools.TaskStopTool); ok {
			st.Runtime = taskRuntime
		}
	}
	if output, err := registry.Get("TaskOutput"); err == nil {
		if ot, ok := output.(*tools.TaskOutputTool); ok {
			ot.Runtime = taskRuntime
		}
	}
	if list, err := registry.Get("BgTaskList"); err == nil {
		if lt, ok := list.(*tools.BgTaskListTool); ok {
			lt.Runtime = taskRuntime
		}
	}

	// Start configured MCP servers and register their tools
	var globalMCPMgr *mcp.Manager
	if len(settings.MCPServers) > 0 {
		globalMCPMgr = mcp.NewManager(settings.MCPServers, registry, eventBus)
		ctx := context.Background()
		for name := range settings.MCPServers {
			if err := globalMCPMgr.StartServer(ctx, name); err != nil {
				// Log but don't fail startup
				fmt.Fprintf(os.Stderr, "Warning: MCP server %q failed to start: %v\n", name, err)
				continue
			}
			// Register MCP tools into main registry
			for _, state := range globalMCPMgr.Status() {
				if state.Name == name && state.Status == "running" && state.Client != nil {
					for _, mcpToolDef := range state.Client.Tools() {
						proxy := tools.NewMCPProxyTool(state.Client, name, mcpToolDef)
						registry.Register(proxy)
					}
				}
			}
		}
	}

	// Inject team manager into team tools
	if sm, err := registry.Get("SendMessage"); err == nil {
		if tool, ok := sm.(*tools.SendMessageTool); ok {
			tool.Manager = teamMgr
			tool.Runner = teamRunner
		}
	}
	if st, err := registry.Get("SpawnTeammate"); err == nil {
		if tool, ok := st.(*tools.SpawnTeammateTool); ok {
			tool.Runner = teamRunner
			tool.Manager = teamMgr
			tool.AvailableModels = buildAvailableModels(apiClient)
		}
	}
	if it, err := registry.Get("InstantiateTeam"); err == nil {
		if tool, ok := it.(*tools.InstantiateTeamTool); ok {
			tool.Runner = teamRunner
			tool.Manager = teamMgr
		}
	}
	if pt, err := registry.Get("PurgeTeammates"); err == nil {
		if tool, ok := pt.(*tools.PurgeTeammatesTool); ok {
			tool.Runner = teamRunner
		}
	}

	// Wire memory store into MemoryTool and RecallTool
	if memTool, err := registry.Get("Memory"); err == nil {
		if mt, ok := memTool.(*tools.MemoryTool); ok {
			mt.Store = memoryStore
		}
	}
	if recallTool, err := registry.Get("Recall"); err == nil {
		if rt, ok := recallTool.(*tools.RecallTool); ok {
			rt.Store = memoryStore
			rt.Client = apiClient
			rt.Model = tools.ResolveToolModel("Recall", settings)
		}
	}

	// Inject cron store into cron tools
	if cc, err := registry.Get("CronCreate"); err == nil {
		if tool, ok := cc.(*tools.CronCreateTool); ok {
			tool.Store = cronStore
		}
	}
	if cd, err := registry.Get("CronDelete"); err == nil {
		if tool, ok := cd.(*tools.CronDeleteTool); ok {
			tool.Store = cronStore
		}
	}
	if cl, err := registry.Get("CronList"); err == nil {
		if tool, ok := cl.(*tools.CronListTool); ok {
			tool.Store = cronStore
		}
	}

	// Build dynamic capability registry and register built-in "design" capability.
	// Must happen before any agent spawns so IsKnown() and ApplyToRegistry() work.
	capReg := capabilities.New()
	capReg.Register("design", capabilities.DesignFactories()...)
	tools.SetCapabilityRegistry(capReg)
	agents.SetCapabilityRegistry(capReg)

	// Initialize Lua plugin runtime (after all registries are ready)
	luaRuntime := luart.New(registry, skillsRegistry, eventBus, hooksMgr, settings, db, capReg)

	// 1. Embedded defaults (sets model, compactMode, etc. from defaults.lua)
	if err := luaRuntime.LoadDefaults(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: lua defaults: %v\n", err)
	}

	// 2. User init (~/.claudio/init.lua) — missing file is silently skipped
	luaUserInit := filepath.Join(config.GetPaths().Home, "init.lua")
	if err := luaRuntime.LoadUserInit(luaUserInit); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: lua user init: %v\n", err)
	}

	// 3. Plugins (~/.claudio/plugins/*/init.lua)
	if err := luaRuntime.LoadAll(config.GetPaths().Plugins); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: lua plugins: %v\n", err)
	}

	// 4. Project-local init (.claudio/init.lua in cwd)
	if workingDir, err := os.Getwd(); err == nil {
		if err := luaRuntime.LoadProjectInit(workingDir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: lua project init: %v\n", err)
		}
	}

	// Apply any providers registered by Lua plugins / init.lua
	luaRuntime.ApplyProviders(apiClient)

	// Initialize window manager; wire it into the Lua runtime so plugins can
	// open/close floating windows via claudio.window API once it is exposed.
	windowMgr := windows.New()
	luaRuntime.SetWindowManager(windowMgr)

	// Wire team runner and manager so Lua plugins can inspect agent/team state.
	luaRuntime.SetTeamRunner(teamRunner)
	luaRuntime.SetTeamManager(teamMgr)

	return &App{
		Config:    settings,
		Profile:   activeProfile,
		Bus:       eventBus,
		Storage:   store,
		Auth:      resolver,
		API:       apiClient,
		DB:        db,
		Tools:     registry,
		Hooks:     hooksMgr,
		Learning:  learningStore,
		Skills:    skillsRegistry,
		Memory:    memoryStore,
		Analytics:     analyticsTracker,
		FilterSavings: filterSvc,
		Auditor:       auditor,
		TaskRuntime: taskRuntime,
		Teams:       teamMgr,
		TeamRunner:  teamRunner,
		Plugins:     pluginReg,
		Cron:        cronStore,
		CronRunner:  cronRunner,
		LSP:         lspManager,
		MCPManager:          globalMCPMgr,
		Capabilities:        capReg,
		LuaRuntime:          luaRuntime,
		WindowManager:       windowMgr,
		// Priority order for TUI (first-match wins): project-local > harness
		// (~/.claudio/team-templates is prepended by TUI as the primary/writable dir)
		HarnessTemplateDirs: append([]string{filepath.Join(cwd, ".claudio", "team-templates")}, harness.CollectTemplateDirs(harnesses)...),
		InjectCh:            injectCh,
		InterruptCh:         interruptCh,
		ccSend:              ccSend,
	}, nil
}

// InjectAttachClient injects the attach client and URL into session coordination tools.
// Called from root.go after attachClient is established.
// client should implement tools.AttachClient interface.
func (a *App) InjectAttachClient(client interface{}, url string) {
	if client != nil {
		if st, err := a.Tools.Get("SendToSession"); err == nil {
			if tool, ok := st.(*tools.SendToSessionTool); ok {
				if ac, ok := client.(tools.AttachClient); ok {
					tool.AttachClient = ac
					tool.AttachURL = url
				}
			}
		}
		// Wire cc sender so sub-agent messages get forwarded to cc_messages.
		if ac, ok := client.(tools.AttachClient); ok {
			a.ccSend.set(ac)
		}
	}
	
	if url != "" {
		if sp, err := a.Tools.Get("SpawnSession"); err == nil {
			if tool, ok := sp.(*tools.SpawnSessionTool); ok {
				tool.AttachURL = url
			}
		}
	}
}

// SubscribeTaskEvents subscribes to EventTaskCreated and EventTaskUpdated on the app bus
// and calls broadcastFn for each, enabling a hub to forward events to web UI clients.
// Call this after the hub is wired, e.g. app.SubscribeTaskEvents(hub.Broadcast).
func (a *App) SubscribeTaskEvents(broadcastFn func(sessionID string, env attach.Envelope)) {
	a.Bus.Subscribe(attach.EventTaskCreated, func(event bus.Event) {
		env := attach.Envelope{Type: attach.EventTaskCreated, Payload: event.Payload}
		broadcastFn(event.SessionID, env)
	})
	a.Bus.Subscribe(attach.EventTaskUpdated, func(event bus.Event) {
		env := attach.Envelope{Type: attach.EventTaskUpdated, Payload: event.Payload}
		broadcastFn(event.SessionID, env)
	})
}

// SubscribeAgentEvents subscribes to EventAgentStatus and EventTeamChanged on the app bus
// and routes each to broadcastFn so the hub can forward them to web UI clients.
// Call this after the hub is wired, e.g. app.SubscribeAgentEvents(hub.Broadcast).
func (a *App) SubscribeAgentEvents(broadcastFn func(sessionID string, env attach.Envelope)) {
	a.Bus.Subscribe(attach.EventAgentStatus, func(event bus.Event) {
		env := attach.Envelope{Type: attach.EventAgentStatus, Payload: event.Payload}
		broadcastFn(event.SessionID, env)
	})
	a.Bus.Subscribe(attach.EventTeamChanged, func(event bus.Event) {
		env := attach.Envelope{Type: attach.EventTeamChanged, Payload: event.Payload}
		broadcastFn(event.SessionID, env)
	})
}

// InjectPayload sends a UserMsgPayload to the inject channel for headless mode processing.
// Non-blocking — if channel full, drops with log (no blocking, no panic).
func (a *App) InjectPayload(p attach.UserMsgPayload) {
	select {
	case a.InjectCh <- p:
		// sent successfully
	default:
		// channel full, drop
		fmt.Fprintf(os.Stderr, "Warning: message injection channel full, dropping message\n")
	}
}

// InjectMessage is a backward-compat shim that wraps content in a UserMsgPayload.
func (a *App) InjectMessage(content string) {
	a.InjectPayload(attach.UserMsgPayload{Content: content})
}

// Interrupt signals the headless engine loop to cancel the current turn.
// Non-blocking — if already signaled or no turn running, the signal is dropped.
func (a *App) Interrupt() {
	select {
	case a.InterruptCh <- struct{}{}:
	default:
	}
}

// ClearHistory wipes the session message history from the DB.
// Called when ComandCenter forwards an EventClearHistory from the web UI.
func (a *App) ClearHistory(sessionID string) {
	if a.DB == nil || sessionID == "" {
		return
	}
	_ = a.DB.DeleteAllMessages(sessionID)
	if a.Bus != nil {
		payload, _ := json.Marshal(attach.ClearHistoryPayload{SessionID: sessionID})
		a.Bus.Publish(bus.Event{Type: attach.EventClearHistory, Payload: payload})
	}
}

// MemoryExtractor returns a callback for background memory extraction at end-of-turn.
// Returns nil if the app doesn't have the required dependencies or if disabled in config.
func (a *App) MemoryExtractor() func(messages []api.Message) {
	if a.API == nil || a.Memory == nil {
		return nil
	}
	if !a.Config.IsAutoMemoryExtract() {
		return nil
	}
	return memory.BuildExtractorCallback(memory.ExtractorConfig{
		Client:   a.API,
		Store:    a.Memory,
		MinTurns: 4,
	})
}

// Close cleans up resources.
func (a *App) Close() error {
	if a.LuaRuntime != nil {
		a.LuaRuntime.Close()
	}
	if a.DB != nil {
		return a.DB.Close()
	}
	return nil
}

// buildAvailableModels returns the list of model names the AI can pick from.
// Includes Anthropic aliases when Anthropic is reachable (default provider),
// plus all configured provider model shortcuts.
func buildAvailableModels(apiClient *api.Client) []string {
	var models []string

	// Anthropic aliases are available when using the default Anthropic API
	// (always true unless the user overrides the base URL to a non-Anthropic provider)
	currentModel := apiClient.GetModel()
	if strings.Contains(currentModel, "claude") || currentModel == "" {
		models = append(models, "sonnet", "opus", "haiku")
	}

	// Add all provider model shortcuts
	for shortcut := range apiClient.GetModelShortcuts() {
		models = append(models, shortcut)
	}

	return models
}

// resolveModelAlias converts short aliases ("haiku", "sonnet", "opus") to full model IDs.
// Returns the input unchanged if it's already a full ID or empty.
func resolveModelAlias(alias string) string {
	switch alias {
	case "haiku":
		return "claude-haiku-4-5-20251001"
	case "sonnet":
		return "claude-sonnet-4-6"
	case "opus":
		return "claude-opus-4-6"
	default:
		return alias
	}
}

// runSubAgentWithMemory is like runSubAgent but also injects agent-scoped memories into the system prompt.
func runSubAgentWithMemory(ctx context.Context, apiClient *api.Client, parentRegistry *tools.Registry, system, prompt, memoryDir string, cfg query.EngineConfig, eventBus *bus.Bus) (string, error) {
	// Each sub-agent gets its own task runtime so background task completions
	// route to the sub-agent's engine (not the parent's). Use a temp dir for
	// output since sub-agent tasks are ephemeral.
	subOutputDir, err := os.MkdirTemp("", "claudio-sub-tasks-*")
	if err != nil {
		return "", fmt.Errorf("create sub-agent task output dir: %w", err)
	}
	subRuntime := tasks.NewRuntime(subOutputDir)
	cfg.TaskRuntime = subRuntime

	// Clone the registry so sub-agent has its own copy
	subRegistry := parentRegistry.Clone()

	// Inject the sub-agent's own runtime into its BashTool so auto-promoted
	// background tasks land on the sub-agent's CompletionCh.
	subRegistry.SetTaskRuntime(subRuntime)

	// Wire agent-scoped memory store if this agent has its own memory dir
	if memoryDir != "" {
		if memTool, err := subRegistry.Get("Memory"); err == nil {
			if mt, ok := memTool.(*tools.MemoryTool); ok {
				mt.Store.SetAgentStore(memoryDir)
			}
		}
		if recallTool, err := subRegistry.Get("Recall"); err == nil {
			if rt, ok := recallTool.(*tools.RecallTool); ok && rt.Store != nil {
				rt.Store.SetAgentStore(memoryDir)
			}
		}
	}

	// Inject any per-agent extra tools (e.g. AdvisorTool) placed in context by
	// the context decorator. These are registered into the cloned registry so they
	// are only available to this specific agent run.
	for _, t := range tools.ExtraToolsFromContext(ctx) {
		subRegistry.Register(t)
	}

	// Load per-agent extra skills and plugins — delegated to app.ApplyAgentExtras
	// so the logic lives in one place and all frontends benefit automatically.
	if agentType := agents.AgentTypeFromContext(ctx); agentType != "" {
		if section := ApplyAgentExtras(subRegistry, agentType); section != "" {
			system += "\n\n" + section
		}
	}

	// Re-wire ToolSearch so it sees the cloned registry (including any newly
	// registered agent-specific plugins), not the original pre-clone registry.
	if ts, err := subRegistry.Get("ToolSearch"); err == nil {
		if tst, ok := ts.(*tools.ToolSearchTool); ok {
			tst.SetRegistry(subRegistry)
		}
	}

	// Depth tracking (via context) prevents infinite recursion — no need to
	// remove the Agent tool entirely. Teammates can still spawn read-only
	// exploration sub-agents (e.g. Explore) up to maxAgentDepth.

	// Apply model override from context (set by AgentTool from agentDef.Model or caller's model param).
	if modelOverride := tools.SubAgentModelFromContext(ctx); modelOverride != "" {
		resolved := resolveModelAlias(modelOverride)
		if resolved != apiClient.GetModel() {
			apiClient = api.NewClientFromExisting(apiClient, resolved)
		}
	}

	// Extract sub-agent observer from context (set by TUI for real-time forwarding)
	observer := tools.GetSubAgentObserver(ctx)

	// Build description for labeling events
	desc := prompt
	if len(desc) > 50 {
		desc = desc[:50]
	}

	// Create a sub-session in the DB for real-time persistence (mirrors claude-code's subagents/ files)
	var subSessionID string
	var subDB *storage.DB
	if dbCtx := tools.SubAgentDBFromContext(ctx); dbCtx != nil && dbCtx.DB != nil {
		subDB = dbCtx.DB
		cwd, _ := os.Getwd()
		// Extract agent type from context (best-effort; falls back to "agent")
		agentType := agents.AgentTypeFromContext(ctx)
		if agentType == "" {
			agentType = "agent"
		}
		if subSess, err := subDB.CreateSubSession(dbCtx.ParentID, agentType, cwd, dbCtx.Model); err == nil {
			subSessionID = subSess.ID
			// Persist the initial user prompt
			_ = subDB.AddMessage(subSessionID, "user", prompt, "text", "", "")
		}
	}

	// Extract cc sender and agent name from context (set by teamRunner context decorator).
	// Both are nil/empty for non-team sub-agents, which is fine — forwarder no-ops.
	ccRef := ccSendFromCtx(ctx)
	ccAgentName := ""
	foreground := false
	if tc := tools.TeamContextFromCtx(ctx); tc != nil {
		ccAgentName = tc.AgentName
		foreground = tc.Foreground
	}

	// Create a forwarder that captures text AND forwards tool events to parent
	forwarder := &subAgentForwarder{
		desc:        desc,
		observer:    observer,
		db:          subDB,
		sessionID:   subSessionID,
		ccSend:      ccRef,
		ccAgentName: ccAgentName,
		foreground:  foreground,
	}
	engine := query.NewEngineWithConfig(apiClient, subRegistry, forwarder, cfg)
	engine.SetSubAgent(true)
	if eventBus != nil {
		engine.SetEventBus(eventBus)
	}
	engine.SetSystem(system)
	if maxTurns := tools.MaxTurnsFromContext(ctx); maxTurns > 0 {
		engine.SetMaxTurns(maxTurns)
	}
	if threshold := tools.CompactThresholdFromContext(ctx); threshold > 0 {
		engine.SetCompactThreshold(threshold)
	}

	// Wire mailbox poller for team agents so they can receive messages mid-run
	if tc := tools.TeamContextFromCtx(ctx); tc != nil {
		teamsDir := filepath.Join(os.Getenv("HOME"), ".claudio", "teams")
		mb := teams.NewMailbox(teamsDir, tc.TeamName)
		agentName := tc.AgentName
		engine.SetMailboxPoller(func() []string {
			msgs, err := mb.ReadUnread(agentName)
			if err != nil || len(msgs) == 0 {
				return nil
			}
			var result []string
			for _, m := range msgs {
				result = append(result, fmt.Sprintf("[From %s]: %s", m.From, m.Text))
			}
			return result
		})
		// Wire the wake channel so pollMailbox can drain signals correctly.
		engine.SetMailboxNotifyChan(mb.NotifyChan())
	}

	// If resume history was injected via context, restore it before running.
	// Used by team agent revival to continue the existing conversation.
	if h := teams.GetResumeHistory(ctx); len(h) > 0 {
		engine.SetMessages(h)
	}

	runErr := engine.Run(ctx, prompt)

	// If a messages sink was installed, hand it the final engine messages
	// regardless of whether Run succeeded — revival needs the history even
	// on partial completions.
	if sink := teams.GetMessagesSink(ctx); sink != nil {
		sink(engine.Messages())
	}

	if runErr != nil {
		if forwarder.lastTurn.Len() > 0 {
			return forwarder.lastTurn.String() + fmt.Sprintf("\n\n[Agent error: %v]", runErr), nil
		}
		return "", fmt.Errorf("sub-agent failed: %w", runErr)
	}

	result := strings.TrimSpace(forwarder.lastTurn.String())
	if result == "" {
		return "(agent produced no output)", nil
	}
	return result, nil
}

// subAgentForwarder captures text output from a sub-agent engine and forwards
// tool events to the parent TUI for real-time display. It also persists
// messages to a sub-session in the DB for crash recovery (mirrors claude-code).
//
// Persistence ordering matches reconstructEngineMessages expectations:
//   assistant text row → tool_use rows → tool_result rows
//
// All data for a turn is buffered in memory and flushed atomically at
// OnTurnComplete, so only complete turns land in the DB (same as claude-code).
type subAgentForwarder struct {
	text      strings.Builder
	desc      string
	observer  tools.SubAgentObserver // may be nil
	db        *storage.DB            // nil = no persistence
	sessionID string

	// ccSend forwards messages to ComandCenter cc_messages via attach WS.
	// nil when not in --attach mode or for non-team sub-agents.
	// Suppressed entirely for foreground agents: their result already arrives
	// via tool_result; sending via ccSend would loop back through the hub and
	// inject the sub-agent's message into the principal's own context.
	ccSend      *ccSendRef
	ccAgentName string // display name used as agent_name in cc_messages
	foreground  bool   // true = skip all ccSend sends

	// lastTurn holds only the current/last assistant turn text.
	// Reset at the start of each new turn so the sync return path
	// can return just the final output instead of the full transcript.
	lastTurn       strings.Builder
	newTurnPending bool // set by OnTurnComplete; consumed by OnTextDelta

	// per-turn buffers — flushed atomically at OnTurnComplete
	pendingText  strings.Builder
	pendingTools []subAgentToolCall
}

// subAgentToolCall buffers one tool use + its result for deferred DB write.
type subAgentToolCall struct {
	id     string
	name   string
	input  string // JSON-encoded input
	result string // tool result content (empty until OnToolUseEnd fires)
	done   bool   // true once OnToolUseEnd has fired
}

func (f *subAgentForwarder) OnTextDelta(text string) {
	// Start of a new assistant turn — reset the last-turn buffer.
	if f.newTurnPending {
		f.lastTurn.Reset()
		f.newTurnPending = false
	}
	f.text.WriteString(text)
	f.lastTurn.WriteString(text)
	if f.db != nil && f.sessionID != "" {
		f.pendingText.WriteString(text)
	}
	if f.observer != nil {
		f.observer.OnSubAgentText(f.desc, text)
	}
	// Forward streaming delta to hub so the web UI agent log shows live text.
	// Suppressed for foreground agents: sending via the parent's attach client
	// causes the hub to loop the message back to the parent's own connection,
	// injecting sub-agent text into the principal's conversation context.
	if !f.foreground && f.ccSend != nil && f.ccAgentName != "" {
		if c := f.ccSend.get(); c != nil {
			_ = c.SendEvent(attach.EventMsgStreamDelta, attach.StreamDeltaPayload{
				Delta:       text,
				Accumulated: f.lastTurn.String(),
				AgentName:   f.ccAgentName,
			})
		}
	}
}

func (f *subAgentForwarder) OnThinkingDelta(text string) {}

func (f *subAgentForwarder) OnToolUseStart(tu tools.ToolUse) {
	if f.observer != nil {
		f.observer.OnSubAgentToolStart(f.desc, tu)
	}
	if f.db != nil && f.sessionID != "" {
		inputJSON, _ := json.Marshal(tu.Input)
		if !json.Valid(inputJSON) {
			inputJSON = []byte("{}")
		}
		f.pendingTools = append(f.pendingTools, subAgentToolCall{
			id:    tu.ID,
			name:  tu.Name,
			input: string(inputJSON),
		})
	}
}

func (f *subAgentForwarder) OnToolUseEnd(tu tools.ToolUse, result *tools.Result) {
	if f.observer != nil {
		f.observer.OnSubAgentToolEnd(f.desc, tu, result)
	}
	if f.db != nil && f.sessionID != "" && result != nil {
		for i := range f.pendingTools {
			if f.pendingTools[i].id == tu.ID {
				f.pendingTools[i].result = result.Content
				f.pendingTools[i].done = true
				break
			}
		}
	}
}

// OnTurnComplete flushes the buffered turn to the DB in the order that
// reconstructEngineMessages expects:
//  1. assistant text (type=text)
//  2. tool_use rows  (type=tool_use, role=assistant)
//  3. tool_result rows (type=tool_result, role=user)
//
// Only completed tool pairs are written; orphaned tool_uses (no result received
// before this call) are dropped — matching claude-code's filterUnresolvedToolUses.
func (f *subAgentForwarder) OnTurnComplete(usage api.Usage) {
	// Signal that the next OnTextDelta begins a new assistant turn.
	f.newTurnPending = true

	// Capture pending state before early return so cc forwarding still works.
	txt := f.pendingText.String()
	f.pendingText.Reset()

	var completed []subAgentToolCall
	for _, tc := range f.pendingTools {
		if tc.done {
			completed = append(completed, tc)
		}
	}
	f.pendingTools = nil

	// --- Native DB writes (sub-agent local SQLite) ---
	if f.db != nil && f.sessionID != "" {
		// 1. Assistant text
		if txt != "" {
			_ = f.db.AddMessage(f.sessionID, "assistant", txt, "text", "", "")
		}
		// 2. tool_use rows (all before any tool_result)
		for _, tc := range completed {
			_ = f.db.AddMessage(f.sessionID, "assistant", tc.input, "tool_use", tc.id, tc.name)
		}
		// 3. tool_result rows
		for _, tc := range completed {
			_ = f.db.AddMessage(f.sessionID, "user", tc.result, "tool_result", tc.id, tc.name)
		}
	}

	// --- cc_messages writes via attach WebSocket (--attach mode, team members only) ---
	// The hub on the ComandCenter side receives these events and writes to cc_messages,
	// making messages visible in the agent logs drawer (handleAgentLogs queries by agentName).
	// Suppressed for foreground agents: the hub broadcasts back to the parent's own
	// attach connection, injecting sub-agent messages into the principal's context.
	// Foreground results already arrive cleanly via the tool_result path.
	if !f.foreground && f.ccSend != nil && f.ccAgentName != "" {
		if cli := f.ccSend.get(); cli != nil {
			// 1. Assistant text
			if txt != "" {
				_ = cli.SendEvent(attach.EventMsgAssistant, attach.AssistantMsgPayload{
					Content:   txt,
					AgentName: f.ccAgentName,
				})
			}
			// 2. tool_use events (must arrive before tool_result so hub can UPDATE the row)
			for _, tc := range completed {
				_ = cli.SendEvent(attach.EventMsgToolUse, attach.ToolUsePayload{
					ID:        tc.id,
					Tool:      tc.name,
					Input:     json.RawMessage(tc.input),
					AgentName: f.ccAgentName,
				})
			}
			// 3. tool_result events
			for _, tc := range completed {
				_ = cli.SendEvent(attach.EventMsgToolResult, attach.ToolResultPayload{
					ToolUseID: tc.id,
					Output:    tc.result,
					AgentName: f.ccAgentName,
				})
			}
		}
	}
}

func (f *subAgentForwarder) OnToolApprovalNeeded(tu tools.ToolUse) bool             { return true }
func (f *subAgentForwarder) OnCostConfirmNeeded(currentCost, threshold float64) bool { return true }
func (f *subAgentForwarder) OnError(err error)                                       {}
func (f *subAgentForwarder) OnRetry(_ []tools.ToolUse)                               {}

// teammateObserver implements SubAgentObserver for a specific teammate,
// updating its ConversationEntry list and Progress in real time.
type teammateObserver struct {
	state    *teams.TeammateState
	runner   *teams.TeammateRunner
	textBuf  strings.Builder
}

func (o *teammateObserver) OnSubAgentText(_ string, text string) {
	o.textBuf.WriteString(text)
}

func (o *teammateObserver) OnSubAgentToolStart(_ string, tu tools.ToolUse) {
	// Flush pending text
	if o.textBuf.Len() > 0 {
		o.state.AddConversation(teams.ConversationEntry{
			Time:    time.Now(),
			Type:    "text",
			Content: o.textBuf.String(),
		})
		o.textBuf.Reset()
	}

	o.state.IncrToolCalls()
	o.state.SetCurrentTool(tu.Name)
	o.state.AddActivity(tu.Name)
	o.state.AddConversation(teams.ConversationEntry{
		Time:     time.Now(),
		Type:     "tool_start",
		Content:  truncateRawInput(tu.Input),
		ToolName: tu.Name,
	})

	o.runner.EmitEvent(teams.TeammateEvent{
		TeamName:  o.state.TeamName,
		AgentID:   o.state.Identity.AgentID,
		AgentName: o.state.Identity.AgentName,
		Type:      "tool_start",
		ToolName:  tu.Name,
		Input:     truncateRawInput(tu.Input),
		Color:     o.state.Identity.Color,
	})
}

func (o *teammateObserver) OnSubAgentToolEnd(_ string, tu tools.ToolUse, result *tools.Result) {
	o.state.SetCurrentTool("")
	content := ""
	if result != nil {
		content = result.Content
		if len(content) > 1000 {
			content = content[:1000] + "..."
		}
	}
	o.state.AddConversation(teams.ConversationEntry{
		Time:     time.Now(),
		Type:     "tool_end",
		Content:  content,
		ToolName: tu.Name,
	})

	// Emit tool_end event for TUI real-time updates
	o.runner.EmitEvent(teams.TeammateEvent{
		TeamName:  o.state.TeamName,
		AgentID:   o.state.Identity.AgentID,
		AgentName: o.state.Identity.AgentName,
		Type:      "tool_end",
		ToolName:  tu.Name,
		Text:      content,
		Color:     o.state.Identity.Color,
	})
}

func truncateRawInput(input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}
	var m map[string]interface{}
	if err := json.Unmarshal(input, &m); err != nil {
		s := string(input)
		if len(s) > 200 {
			return s[:200] + "..."
		}
		return s
	}
	// For common tools, extract the most useful field
	for _, key := range []string{"command", "file_path", "pattern", "query"} {
		if v, ok := m[key]; ok {
			s := fmt.Sprintf("%v", v)
			if len(s) > 200 {
				return s[:200] + "..."
			}
			return s
		}
	}
	s := string(input)
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}
