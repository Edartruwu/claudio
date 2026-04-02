package app

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/auth"
	authstorage "github.com/Abraxas-365/claudio/internal/auth/storage"
	"github.com/Abraxas-365/claudio/internal/bus"
	"github.com/Abraxas-365/claudio/internal/config"
	"github.com/Abraxas-365/claudio/internal/hooks"
	"github.com/Abraxas-365/claudio/internal/learning"
	"github.com/Abraxas-365/claudio/internal/query"
	"github.com/Abraxas-365/claudio/internal/security"
	"github.com/Abraxas-365/claudio/internal/services/analytics"
	"github.com/Abraxas-365/claudio/internal/services/memory"
	"github.com/Abraxas-365/claudio/internal/services/skills"
	"github.com/Abraxas-365/claudio/internal/storage"
	"github.com/Abraxas-365/claudio/internal/tasks"
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
	Memory    *memory.Store
	Analytics    *analytics.Tracker
	Auditor      *security.Auditor
	TaskRuntime  *tasks.Runtime
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
func New(settings *config.Settings) (*App, error) {
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

	// Load memory
	memoryStore := memory.NewStore(paths.Memory)

	// Analytics tracker
	analyticsTracker := analytics.NewTracker(settings.Model, settings.MaxBudget, paths.Home+"/analytics")

	// Auditor
	auditor := security.NewAuditor(db, eventBus)

	// Task runtime for background execution
	taskRuntime := tasks.NewRuntime(paths.Home + "/task-output")

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
	}, nil
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
	// Clone the registry so sub-agent has its own copy
	subRegistry := parentRegistry.Clone()

	// Remove the Agent tool from sub-agents to prevent infinite recursion
	subRegistry.Remove("Agent")

	// Create a collector handler that captures text output
	collector := &agentOutputCollector{}
	engine := query.NewEngine(apiClient, subRegistry, collector)
	engine.SetSystem(system)

	if err := engine.Run(ctx, prompt); err != nil {
		if collector.text.Len() > 0 {
			// Return partial output even on error
			return collector.text.String() + fmt.Sprintf("\n\n[Agent error: %v]", err), nil
		}
		return "", fmt.Errorf("sub-agent failed: %w", err)
	}

	result := strings.TrimSpace(collector.text.String())
	if result == "" {
		return "(agent produced no output)", nil
	}
	return result, nil
}

// agentOutputCollector captures text output from a sub-agent engine.
type agentOutputCollector struct {
	text strings.Builder
}

func (c *agentOutputCollector) OnTextDelta(text string)                              { c.text.WriteString(text) }
func (c *agentOutputCollector) OnThinkingDelta(text string)                          {}
func (c *agentOutputCollector) OnToolUseStart(tu tools.ToolUse)                      {}
func (c *agentOutputCollector) OnToolUseEnd(tu tools.ToolUse, result *tools.Result)  {}
func (c *agentOutputCollector) OnTurnComplete(usage api.Usage)                       {}
func (c *agentOutputCollector) OnToolApprovalNeeded(tu tools.ToolUse) bool           { return true } // auto-approve in sub-agents
func (c *agentOutputCollector) OnError(err error)                                    {}
