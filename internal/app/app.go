package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Abraxas-365/claudio/internal/agents"
	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/auth"
	authstorage "github.com/Abraxas-365/claudio/internal/auth/storage"
	"github.com/Abraxas-365/claudio/internal/bus"
	"github.com/Abraxas-365/claudio/internal/config"
	"github.com/Abraxas-365/claudio/internal/hooks"
	"github.com/Abraxas-365/claudio/internal/learning"
	"github.com/Abraxas-365/claudio/internal/models"
	"github.com/Abraxas-365/claudio/internal/plugins"
	"github.com/Abraxas-365/claudio/internal/query"
	"github.com/Abraxas-365/claudio/internal/security"
	"github.com/Abraxas-365/claudio/internal/services/analytics"
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
	Analytics    *analytics.Tracker
	Auditor      *security.Auditor
	TaskRuntime  *tasks.Runtime
	Teams        *teams.Manager
	TeamRunner   *teams.TeammateRunner
	Plugins      *plugins.Registry
	Cron         *tasks.CronStore
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
	if settings.APIBaseURL != "" {
		apiOpts = append(apiOpts, api.WithBaseURL(settings.APIBaseURL))
	}
	if settings.Model != "" {
		apiOpts = append(apiOpts, api.WithModel(settings.Model))
	}

	apiClient := api.NewClient(resolver, apiOpts...)

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

	// Inject security into file/shell tools
	if bash, err := registry.Get("Bash"); err == nil {
		if bt, ok := bash.(*tools.BashTool); ok {
			bt.Security = sec
		}
	}
	if read, err := registry.Get("Read"); err == nil {
		if rt, ok := read.(*tools.FileReadTool); ok {
			rt.Security = sec
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

	// Remove denied tools
	for _, denied := range settings.DenyTools {
		registry.Remove(denied)
	}

	paths := config.GetPaths()
	cwd, _ := os.Getwd()

	// Load hooks
	hooksMgr := hooks.LoadFromSettings(paths.Settings, paths.Local)

	// Load learning store
	learningStore := learning.NewStore(paths.Instincts)
	learningStore.Decay() // prune stale instincts

	// Load skills
	skillsRegistry := skills.LoadAll(paths.Skills, cwd+"/.claudio/skills")

	// Register custom agent directories so GetAgent() can discover them
	agents.SetCustomDirs(paths.Agents, cwd+"/.claudio/agents")

	// Load memory (project-scoped + global fallback)
	projectMemDir := ""
	if projectRoot != "" {
		projectMemDir = config.ProjectMemoryDir(projectRoot)
	}
	memoryStore := memory.NewScopedStore(projectMemDir, paths.Memory)

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

	// Plugins
	pluginReg := plugins.NewRegistry()
	pluginReg.LoadDir(paths.Plugins)
	pluginReg.LoadDir(cwd + "/.claudio/plugins")

	// Cron store
	cronStore := tasks.NewCronStore(filepath.Join(paths.Home, "cron.json"))
	cronStore.Load()

	// Team manager
	teamMgr := teams.NewManager(paths.Home + "/teams")

	// Team runner (uses the same runSubAgent callback)
	teamRunner := teams.NewTeammateRunner(teamMgr, func(ctx context.Context, system, prompt string) (string, error) {
		return runSubAgent(ctx, apiClient, registry, system, prompt)
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
		}
	}
	if td, err := registry.Get("TeamDelete"); err == nil {
		if tool, ok := td.(*tools.TeamDeleteTool); ok {
			tool.Manager = teamMgr
		}
	}
	if sm, err := registry.Get("SendMessage"); err == nil {
		if tool, ok := sm.(*tools.SendMessageTool); ok {
			tool.Manager = teamMgr
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
		Analytics: analyticsTracker,
		Auditor:     auditor,
		TaskRuntime: taskRuntime,
		Teams:       teamMgr,
		TeamRunner:  teamRunner,
		Plugins:     pluginReg,
		Cron:        cronStore,
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

// runSubAgentWithMemory is like runSubAgent but also injects agent-scoped memories into the system prompt.
func runSubAgentWithMemory(ctx context.Context, apiClient *api.Client, parentRegistry *tools.Registry, system, prompt, memoryDir string) (string, error) {
	// Clone the registry so sub-agent has its own copy
	subRegistry := parentRegistry.Clone()

	// Remove the Agent tool from sub-agents to prevent infinite recursion
	subRegistry.Remove("Agent")

	// Inject agent-scoped memories if available
	if memoryDir != "" {
		agentMem := memory.NewStore(memoryDir)
		if memContent := agentMem.ForSystemPrompt(); memContent != "" {
			system = system + "\n\n" + memContent
		}
	}

	// Extract sub-agent observer from context (set by TUI for real-time forwarding)
	observer := tools.GetSubAgentObserver(ctx)

	// Build description for labeling events
	desc := prompt
	if len(desc) > 50 {
		desc = desc[:50]
	}

	// Create a forwarder that captures text AND forwards tool events to parent
	forwarder := &subAgentForwarder{desc: desc, observer: observer}
	engine := query.NewEngine(apiClient, subRegistry, forwarder)
	engine.SetSystem(system)
	if maxTurns := tools.MaxTurnsFromContext(ctx); maxTurns > 0 {
		engine.SetMaxTurns(maxTurns)
	}

	if err := engine.Run(ctx, prompt); err != nil {
		if forwarder.text.Len() > 0 {
			// Return partial output even on error
			return forwarder.text.String() + fmt.Sprintf("\n\n[Agent error: %v]", err), nil
		}
		return "", fmt.Errorf("sub-agent failed: %w", err)
	}

	result := strings.TrimSpace(forwarder.text.String())
	if result == "" {
		return "(agent produced no output)", nil
	}
	return result, nil
}

// subAgentForwarder captures text output from a sub-agent engine and forwards
// tool events to the parent TUI for real-time display.
type subAgentForwarder struct {
	text     strings.Builder
	desc     string
	observer tools.SubAgentObserver // may be nil
}

func (f *subAgentForwarder) OnTextDelta(text string)     { f.text.WriteString(text) }
func (f *subAgentForwarder) OnThinkingDelta(text string) {}
func (f *subAgentForwarder) OnToolUseStart(tu tools.ToolUse) {
	if f.observer != nil {
		f.observer.OnSubAgentToolStart(f.desc, tu)
	}
}
func (f *subAgentForwarder) OnToolUseEnd(tu tools.ToolUse, result *tools.Result) {
	if f.observer != nil {
		f.observer.OnSubAgentToolEnd(f.desc, tu, result)
	}
}
func (f *subAgentForwarder) OnTurnComplete(usage api.Usage)                        {}
func (f *subAgentForwarder) OnToolApprovalNeeded(tu tools.ToolUse) bool            { return true }
func (f *subAgentForwarder) OnCostConfirmNeeded(currentCost, threshold float64) bool { return true }
func (f *subAgentForwarder) OnError(err error)                                     {}
