package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Abraxas-365/claudio/internal/agents"
	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/api/provider"
	"github.com/Abraxas-365/claudio/internal/auth"
	authstorage "github.com/Abraxas-365/claudio/internal/auth/storage"
	"github.com/Abraxas-365/claudio/internal/bus"
	"github.com/Abraxas-365/claudio/internal/config"
	"github.com/Abraxas-365/claudio/internal/hooks"
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

// App holds all shared application dependencies.
type App struct {
	Config    *config.Settings
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
	LSP          *lsp.ServerManager
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
func New(settings *config.Settings, projectRoot string) (*App, error) {
	if err := config.EnsureDirs(); err != nil {
		return nil, err
	}

	eventBus := bus.New()
	store := authstorage.NewDefaultStorage()
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

	// Remove denied tools
	for _, denied := range settings.DenyTools {
		registry.Remove(denied)
	}

	paths := config.GetPaths()
	cwd, _ := os.Getwd()

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
	lspManager := lsp.NewServerManager(lspCfgs)
	registry.SetLSPManager(lspManager)

	// Load hooks
	hooksMgr := hooks.LoadFromSettings(paths.Settings, paths.Local)

	// Load learning store
	learningStore := learning.NewStore(paths.Instincts)
	learningStore.Decay() // prune stale instincts

	// Load skills
	skillsRegistry := skills.LoadAll(paths.Skills, cwd+"/.claudio/skills")

	// Inject skills registry into SkillTool so all agents (main + teammates) can
	// auto-detect and invoke skills. Clone() propagates the pointer to sub-agents.
	if st, err := registry.Get("Skill"); err == nil {
		if skillTool, ok := st.(*tools.SkillTool); ok {
			skillTool.SkillsRegistry = skillsRegistry
			skillTool.HooksManager = hooksMgr
			skillTool.ProjectRoot = cwd
		}
	}

	// Register custom agent directories so GetAgent() can discover them
	agents.SetCustomDirs(paths.Agents, cwd+"/.claudio/agents")

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

	// Plugins — discover and register as tools
	pluginReg := plugins.NewRegistry()
	pluginReg.LoadDir(paths.Plugins)
	pluginReg.LoadDir(cwd + "/.claudio/plugins")
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

	// Cron store
	cronStore := tasks.NewCronStore(filepath.Join(paths.Home, "cron.json"))
	cronStore.Load()

	// Team manager
	teamMgr := teams.NewManager(paths.Home+"/teams", paths.TeamTemplates)

	// Team runner (uses the same runSubAgent callback)
	teamRunner := teams.NewTeammateRunner(teamMgr, func(ctx context.Context, system, prompt string) (string, error) {
		return runSubAgent(ctx, apiClient, registry, system, prompt)
	})
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
		return runSubAgentWithMemory(ctx, apiClient, registry, system, prompt, memoryDir)
	})

	// Revive callback: continues an existing agent conversation by restoring
	// engine history via context and running the new message as the next user
	// turn. Memory dir is honored if the state was backed by a crystallized agent.
	teamRunner.SetRunAgentResume(func(ctx context.Context, system, memoryDir string, history []api.Message, newMessage string) (string, error) {
		ctx = teams.WithResumeHistory(ctx, history)
		return runSubAgentWithMemory(ctx, apiClient, registry, system, newMessage, memoryDir)
	})

	// Wire per-teammate context decorator: injects a SubAgentObserver that
	// populates TeammateState.Conversation and Progress in real time.
	teamRunner.SetContextDecorator(func(ctx context.Context, state *teams.TeammateState) context.Context {
		obs := &teammateObserver{state: state, runner: teamRunner}
		ctx = tools.WithSubAgentObserver(ctx, obs)
		ctx = tools.WithTeamContext(ctx, tools.TeamContext{
			TeamName:  state.TeamName,
			AgentName: state.Identity.AgentName,
		})
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
	teamRunner.SetTaskCompleter(func(taskIDs []string, status string) {
		tools.GlobalTaskStore.CompleteByIDs(taskIDs, status)
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
			// Wire available models: Anthropic aliases + provider shortcuts
			at.AvailableModels = buildAvailableModels(apiClient)
			// Wire real sub-agent execution
			at.RunAgent = func(ctx context.Context, system, prompt string) (string, error) {
				return runSubAgent(ctx, apiClient, registry, system, prompt)
			}
			at.RunAgentWithMemory = func(ctx context.Context, system, prompt, memoryDir string) (string, error) {
				return runSubAgentWithMemory(ctx, apiClient, registry, system, prompt, memoryDir)
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

	// Start configured MCP servers and register their tools
	if len(settings.MCPServers) > 0 {
		mcpMgr := mcp.NewManager(settings.MCPServers, registry, eventBus)
		ctx := context.Background()
		for name := range settings.MCPServers {
			if err := mcpMgr.StartServer(ctx, name); err != nil {
				// Log but don't fail startup
				fmt.Fprintf(os.Stderr, "Warning: MCP server %q failed to start: %v\n", name, err)
				continue
			}
			// Register MCP tools into main registry
			for _, state := range mcpMgr.Status() {
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
	if tc, err := registry.Get("TeamCreate"); err == nil {
		if tool, ok := tc.(*tools.TeamCreateTool); ok {
			tool.Manager = teamMgr
			tool.Runner = teamRunner
			tool.AvailableModels = buildAvailableModels(apiClient)
		}
	}
	if td, err := registry.Get("TeamDelete"); err == nil {
		if tool, ok := td.(*tools.TeamDeleteTool); ok {
			tool.Manager = teamMgr
			tool.Runner = teamRunner
		}
	}
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
	if stt, err := registry.Get("SaveTeamTemplate"); err == nil {
		if tool, ok := stt.(*tools.SaveTeamTemplateTool); ok {
			tool.Runner = teamRunner
			tool.Manager = teamMgr
		}
	}
	if it, err := registry.Get("InstantiateTeam"); err == nil {
		if tool, ok := it.(*tools.InstantiateTeamTool); ok {
			tool.Runner = teamRunner
			tool.Manager = teamMgr
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
			rt.Model = settings.SmallModel
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

	return &App{
		Config:    settings,
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
		LSP:         lspManager,
	}, nil
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
	if a.DB != nil {
		return a.DB.Close()
	}
	return nil
}

// runSubAgent creates a new query.Engine with the given system prompt and
// runs a single prompt through it, capturing all text output.
func runSubAgent(ctx context.Context, apiClient *api.Client, parentRegistry *tools.Registry, system, prompt string) (string, error) {
	return runSubAgentWithMemory(ctx, apiClient, parentRegistry, system, prompt, "")
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
func runSubAgentWithMemory(ctx context.Context, apiClient *api.Client, parentRegistry *tools.Registry, system, prompt, memoryDir string) (string, error) {
	// Clone the registry so sub-agent has its own copy
	subRegistry := parentRegistry.Clone()

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

	// Load per-agent extra skills and plugins when the agent type resolves to a
	// definition that has ExtraSkillsDir or ExtraPluginsDir set.
	if agentType := tools.AgentTypeFromContext(ctx); agentType != "" {
		agentDef := agents.GetAgent(agentType)

		// Merge extra skills (additive — global skills remain available)
		if agentDef.ExtraSkillsDir != "" {
			if skillToolRaw, err := subRegistry.Get("Skill"); err == nil {
				if st, ok := skillToolRaw.(*tools.SkillTool); ok {
					// Clone the existing skills registry so we don't mutate the global one
					mergedReg := skills.NewRegistry()
					for _, s := range st.SkillsRegistry.All() {
						mergedReg.Register(s)
					}
					// Load extra skills from the agent's skills dir and merge in
					extraReg := skills.LoadAll("", agentDef.ExtraSkillsDir)
					for _, s := range extraReg.All() {
						mergedReg.Register(s)
					}
					// Replace the SkillTool with a fresh instance using the merged registry
					subRegistry.Remove("Skill")
					subRegistry.Register(&tools.SkillTool{
						SkillsRegistry: mergedReg,
						HooksManager:   st.HooksManager,
						ProjectRoot:    st.ProjectRoot,
					})
				}
			}
		}

		// Register extra plugins (additive)
		if agentDef.ExtraPluginsDir != "" {
			extraPluginReg := plugins.NewRegistry()
			extraPluginReg.LoadDir(agentDef.ExtraPluginsDir)
			// Mirror OutputFilterEnabled from existing proxy tools in the registry
			outputFilterEnabled := false
			for _, t := range subRegistry.All() {
				if pt, ok := t.(*plugins.PluginProxyTool); ok {
					outputFilterEnabled = pt.OutputFilterEnabled
					break
				}
			}
			for _, p := range extraPluginReg.All() {
				pt := plugins.NewProxyTool(p)
				pt.OutputFilterEnabled = outputFilterEnabled
				subRegistry.Register(pt)
			}
			// Inject extra plugin instructions into the system prompt so the
			// model knows the plugin commands and subcommands.
			var pluginInfos []prompts.PluginInfo
			for _, p := range extraPluginReg.All() {
				pluginInfos = append(pluginInfos, prompts.PluginInfo{
					Name:         p.Name,
					Description:  p.Description,
					Instructions: p.Instructions,
				})
			}
			if section := prompts.PluginsSection(pluginInfos); section != "" {
				system += "\n\n" + section
			}
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
		agentType := tools.AgentTypeFromContext(ctx)
		if agentType == "" {
			agentType = "agent"
		}
		if subSess, err := subDB.CreateSubSession(dbCtx.ParentID, agentType, cwd, dbCtx.Model); err == nil {
			subSessionID = subSess.ID
			// Persist the initial user prompt
			_ = subDB.AddMessage(subSessionID, "user", prompt, "text", "", "")
		}
	}

	// Create a forwarder that captures text AND forwards tool events to parent
	forwarder := &subAgentForwarder{
		desc:      desc,
		observer:  observer,
		db:        subDB,
		sessionID: subSessionID,
	}
	engine := query.NewEngine(apiClient, subRegistry, forwarder)
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
		if forwarder.text.Len() > 0 {
			return forwarder.text.String() + fmt.Sprintf("\n\n[Agent error: %v]", runErr), nil
		}
		return "", fmt.Errorf("sub-agent failed: %w", runErr)
	}

	result := strings.TrimSpace(forwarder.text.String())
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
	f.text.WriteString(text)
	if f.db != nil && f.sessionID != "" {
		f.pendingText.WriteString(text)
	}
	if f.observer != nil {
		f.observer.OnSubAgentText(f.desc, text)
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
	if f.db == nil || f.sessionID == "" {
		return
	}

	// 1. Assistant text
	if txt := f.pendingText.String(); txt != "" {
		_ = f.db.AddMessage(f.sessionID, "assistant", txt, "text", "", "")
		f.pendingText.Reset()
	}

	// Filter to only completed pairs (drop orphaned tool_uses)
	var completed []subAgentToolCall
	for _, tc := range f.pendingTools {
		if tc.done {
			completed = append(completed, tc)
		}
	}

	// 2. tool_use rows (all before any tool_result)
	for _, tc := range completed {
		_ = f.db.AddMessage(f.sessionID, "assistant", tc.input, "tool_use", tc.id, tc.name)
	}

	// 3. tool_result rows
	for _, tc := range completed {
		_ = f.db.AddMessage(f.sessionID, "user", tc.result, "tool_result", tc.id, tc.name)
	}

	f.pendingTools = nil
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
