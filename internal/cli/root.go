package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/Abraxas-365/claudio/internal/agents"
	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/app"
	"github.com/Abraxas-365/claudio/internal/auth/refresh"
	"github.com/Abraxas-365/claudio/internal/config"
	"github.com/Abraxas-365/claudio/internal/prompts"
	"github.com/Abraxas-365/claudio/internal/query"
	"github.com/Abraxas-365/claudio/internal/rules"
	"github.com/Abraxas-365/claudio/internal/services/memory"
	"github.com/Abraxas-365/claudio/internal/session"
	"github.com/Abraxas-365/claudio/internal/snippets"
	"github.com/Abraxas-365/claudio/internal/tasks"
	"github.com/Abraxas-365/claudio/internal/tools"
	"github.com/Abraxas-365/claudio/internal/tui"
	"github.com/Abraxas-365/claudio/internal/utils"
)

var (
	flagModel               string
	flagAgent               string
	flagVerbose             bool
	flagHeadless            bool
	flagContext             string
	flagBudget              float64
	flagResume              string
	flagDangerouslySkipPerm bool
	flagPrint               bool
)

// appInstance is initialized before command execution.
var appInstance *app.App

var rootCmd = &cobra.Command{
	Use:   "claudio [prompt]",
	Short: "Claudio your AI pair programmer in the terminal",
	Long: `Claudio is an open-source AI coding assistant that lives in your terminal.

It understands your codebase, executes tools, manages sessions, and helps you
ship faster — all without leaving the command line. Built in pure Go for speed,
security, and hackability.`,
	Args: cobra.ArbitraryArgs,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip app init for commands that don't need it
		if cmd.Name() == "version" || cmd.Name() == "help" {
			return nil
		}

		cwd, _ := os.Getwd()

		// Find git root for consistent project identity
		projectRoot := config.FindGitRoot(cwd)

		// Trust check: if project has config, verify it's trusted
		if config.HasProjectConfig(projectRoot) {
			ts := config.NewTrustStore()
			if !ts.IsTrusted(projectRoot) {
				info := config.ScanProjectConfig(projectRoot)
				// Only prompt if there are security-relevant configs (hooks, MCP)
				if info.HasHooks || info.HasMCP {
					fmt.Fprint(os.Stderr, config.FormatTrustPrompt(projectRoot, info))
					var answer string
					fmt.Scanln(&answer)
					if strings.ToLower(strings.TrimSpace(answer)) != "y" {
						return fmt.Errorf("project not trusted — skipping project configuration")
					}
				}
				// Mark as trusted
				ts.Trust(projectRoot, info.HasHooks, info.HasMCP, info.HasRules)
			}
		}

		settings, err := config.Load(projectRoot)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		fmt.Fprintf(os.Stderr, "[PERM-DEBUG] projectRoot=%q permissionRules=%d\n", projectRoot, len(settings.PermissionRules))
		for i, r := range settings.PermissionRules {
			fmt.Fprintf(os.Stderr, "[PERM-DEBUG]   rule[%d] tool=%q pattern=%q behavior=%q\n", i, r.Tool, r.Pattern, r.Behavior)
		}

		// Apply CLI flag overrides
		if flagModel != "" {
			settings.Model = flagModel
		}
		if flagBudget > 0 {
			settings.MaxBudget = flagBudget
		}
		if flagDangerouslySkipPerm {
			settings.PermissionMode = "dangerously-skip-permissions"
			fmt.Fprintln(os.Stderr, "\033[33m⚠ WARNING: All permission checks are disabled. Tools will execute without approval.\033[0m")
		}

		a, err := app.New(settings, projectRoot)
		if err != nil {
			return fmt.Errorf("failed to initialize: %w", err)
		}
		appInstance = a
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Detect pipe mode: if stdout is not a terminal, use print mode
		isPiped := !isTerminal()
		if isPiped || flagPrint {
			if len(args) == 0 {
				// Read prompt from stdin when piped
				scanner := bufio.NewScanner(os.Stdin)
				var lines []string
				for scanner.Scan() {
					lines = append(lines, scanner.Text())
				}
				if len(lines) == 0 {
					return fmt.Errorf("no prompt provided")
				}
				return runSinglePrompt(strings.Join(lines, "\n"))
			}
			return runSinglePrompt(strings.Join(args, " "))
		}

		if len(args) > 0 {
			return runSinglePrompt(strings.Join(args, " "))
		}
		return runInteractive()
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagModel, "model", "", "AI model to use (e.g., claude-opus-4-6)")
	rootCmd.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().BoolVar(&flagHeadless, "headless", false, "Run in headless server mode (no TUI)")
	rootCmd.PersistentFlags().StringVar(&flagContext, "context", "", "Load context profile (dev, review, research, or path)")
	rootCmd.PersistentFlags().Float64Var(&flagBudget, "budget", 0, "Session budget limit in USD (0 = unlimited)")
	rootCmd.PersistentFlags().StringVarP(&flagResume, "resume", "r", "", "Resume a previous session (no value = most recent)")
	rootCmd.PersistentFlags().Lookup("resume").NoOptDefVal = "last"
	rootCmd.PersistentFlags().BoolVar(&flagDangerouslySkipPerm, "dangerously-skip-permissions", false, "Skip all permission prompts (use with caution)")
	rootCmd.PersistentFlags().BoolVar(&flagDangerouslySkipPerm, "yolo", false, "Alias for --dangerously-skip-permissions")
	rootCmd.PersistentFlags().BoolVar(&flagPrint, "print", false, "Print-only mode (no TUI, clean stdout for piping)")
	rootCmd.PersistentFlags().StringVar(&flagAgent, "agent", "", "Run as a specific agent persona (e.g., prab, backend-senior)")
}

func Execute() error {
	return rootCmd.Execute()
}

func runSinglePrompt(prompt string) error {
	if !appInstance.Auth.IsLoggedIn() {
		return fmt.Errorf("not logged in. Run: claudio auth login")
	}

	// Proactive token refresh
	if _, err := refresh.CheckAndRefreshIfNeeded(appInstance.Storage, false); err != nil {
		if flagVerbose {
			fmt.Fprintf(os.Stderr, "Warning: token refresh failed: %v\n", err)
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	defer appInstance.Close()

	reg, modelOverride := applyAgentOverrides(appInstance.Tools)
	if modelOverride != "" {
		appInstance.Config.Model = modelOverride
		appInstance.API.SetModel(modelOverride)
	}
	handler := &query.StdoutHandler{Verbose: flagVerbose}
	engine := query.NewEngineWithConfig(appInstance.API, reg, handler, query.EngineConfig{
		Hooks:           appInstance.Hooks,
		Analytics:       appInstance.Analytics,
		TaskRuntime:     appInstance.TaskRuntime,
		Model:           appInstance.Config.Model,
		PermissionMode:  appInstance.Config.PermissionMode,
		PermissionRules: appInstance.Config.PermissionRules,
		OnTurnEnd:       appInstance.MemoryExtractor(),
	})
	engine.SetSystem(buildFullSystemPrompt())
	engine.SetUserContext(prompts.FormatUserContextMessage(buildUserContext(), ""))
	engine.SetSystemContext(buildSystemContext())

	err := engine.Run(ctx, prompt)

	// Print cost summary to stderr
	printCostSummary()

	return err
}

func loadContextProfile(name string) (string, error) {
	paths := config.GetPaths()

	var path string
	switch name {
	case "dev", "review", "research":
		path = paths.Contexts + "/" + name + ".md"
	default:
		path = name
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("could not load context profile %q: %w", name, err)
	}
	return string(data), nil
}

// applyAgentOverrides clones the registry filtered by the --agent flag's DisallowedTools,
// and returns the model override string ("" if no agent or no model override).
func applyAgentOverrides(registry *tools.Registry) (*tools.Registry, string) {
	if flagAgent == "" {
		return registry, ""
	}
	agentDef := agents.GetAgent(flagAgent)
	filtered := registry.Clone()
	for _, name := range agentDef.DisallowedTools {
		filtered.Remove(name)
	}
	model := agentDef.Model
	if resolved, ok := appInstance.API.ResolveModelShortcut(model); ok {
		model = resolved
	}
	return filtered, model
}

// buildFullSystemPrompt gathers all context (rules, context profiles, memory, output style)
// and builds the complete system prompt. CLAUDE.md is NOT included here — it moves to
// the user context message via buildUserContext() + engine.SetUserContext().
func buildFullSystemPrompt() string {
	paths := config.GetPaths()
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()

	// Gather additional context sections
	var sections []string

	// 1. Load context profile if specified
	if flagContext != "" {
		if content, err := loadContextProfile(flagContext); err == nil {
			sections = append(sections, content)
		}
	}

	// 2. Load rules
	rulesReg := rules.LoadAll(
		paths.Rules,
		cwd+"/.claudio/rules",
	)
	rulesReg.LoadCLAUDEMD(cwd, home)
	if rulesContent := rulesReg.ForSystemPrompt(); rulesContent != "" {
		sections = append(sections, rulesContent)
	}

	// 3. Session memory (selection strategy from config)
	if appInstance.Memory != nil {
		switch appInstance.Config.GetMemorySelection() {
		case "ai":
			ctx := context.Background()
			selector := &memory.AISelector{Client: appInstance.API}
			if memContent := appInstance.Memory.ForSystemPromptWithSelection(ctx, cwd, selector); memContent != "" {
				sections = append(sections, memContent)
			}
		case "keyword":
			ctx := context.Background()
			selector := &memory.KeywordSelector{}
			if memContent := appInstance.Memory.ForSystemPromptWithSelection(ctx, cwd, selector); memContent != "" {
				sections = append(sections, memContent)
			}
		case "none":
			// Skip memory loading entirely
		}

		// Inject auto-memory writing instructions only for Anthropic models.
		// External providers (Ollama, Groq, etc.) confuse the Memory-tool
		// guidance with conversation context, causing them to claim they don't
		// know things the user said earlier in the same session.
		if memDir := appInstance.Memory.WriteTargetDir(); memDir != "" {
			if !appInstance.API.IsExternalModel(appInstance.Config.Model) {
				if memInstr := prompts.SessionMemorySection(memDir); memInstr != "" {
					sections = append(sections, memInstr)
				}
			}
		}
	}

	// 4. Learned instincts
	if appInstance.Learning != nil {
		if instinctContent := appInstance.Learning.ForSystemPrompt(cwd); instinctContent != "" {
			sections = append(sections, instinctContent)
		}
	}

	// 5. Output style
	if appInstance.Config.OutputStyle != "" {
		if styleContent := prompts.OutputStyleSection(prompts.OutputStyle(appInstance.Config.OutputStyle)); styleContent != "" {
			sections = append(sections, styleContent)
		}
	}

	// 6. Scratchpad directory
	scratchpadDir := filepath.Join(os.TempDir(), fmt.Sprintf("claudio-%d", os.Getpid()))
	if scratchpadSection := prompts.ScratchpadSection(scratchpadDir); scratchpadSection != "" {
		sections = append(sections, scratchpadSection)
	}

	// 7. Snippet documentation
	if snippetSection := snippets.ForSystemPrompt(appInstance.Config.Snippets); snippetSection != "" {
		sections = append(sections, snippetSection)
	}

	// 8. Installed plugins
	if appInstance.Plugins != nil && len(appInstance.Plugins.All()) > 0 {
		var pluginInfos []prompts.PluginInfo
		for _, p := range appInstance.Plugins.All() {
			pluginInfos = append(pluginInfos, prompts.PluginInfo{
				Name:         p.Name,
				Description:  p.Description,
				Instructions: p.Instructions,
			})
		}
		if pluginSection := prompts.PluginsSection(pluginInfos); pluginSection != "" {
			sections = append(sections, pluginSection)
		}
	}

	// Agent persona override (appended last so it has highest precedence over style/snippets)
	if flagAgent != "" {
		agentDef := agents.GetAgent(flagAgent)
		if agentDef.SystemPrompt != "" {
			sections = append(sections, agentDef.SystemPrompt)
		}
	}

	// Combine all additional context
	additionalCtx := strings.Join(sections, "\n\n")

	return prompts.BuildSystemPrompt(appInstance.Config.Model, additionalCtx)
}

// buildUserContext loads CLAUDE.md/CLAUDIO.md content (raw) for use in the user context message.
func buildUserContext() string {
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	return loadCLAUDEMD(cwd, home)
}

// buildSystemContext returns the formatted git status for appending to the system prompt.
func buildSystemContext() string {
	return prompts.FormatSystemContext(prompts.GetGitStatus())
}

// loadCLAUDEMD finds and loads CLAUDE.md or CLAUDIO.md from the project.
// Walks from git root to cwd, loading files at each level (closer = higher priority).
// Also resolves @path/to/file.md imports inline.
func loadCLAUDEMD(projectDir, homeDir string) string {
	cwd, _ := os.Getwd()

	// Collect directories from project root to cwd
	dirs := collectDirsRootToCwd(projectDir, cwd)

	var parts []string
	for _, dir := range dirs {
		for _, name := range []string{"CLAUDIO.md", "CLAUDE.md", ".claudio/CLAUDE.md"} {
			path := filepath.Join(dir, name)
			if content := utils.ReadFileIfExists(path); content != "" {
				content = resolveImports(content, dir, nil)
				parts = append(parts, content)
				break // only first match per directory
			}
		}
	}

	return strings.Join(parts, "\n\n")
}

// collectDirsRootToCwd returns directories from projectRoot down to cwd (inclusive).
// The result is ordered root-first so closer-to-cwd dirs appear later (higher priority).
func collectDirsRootToCwd(projectRoot, cwd string) []string {
	projectRoot = filepath.Clean(projectRoot)
	cwd = filepath.Clean(cwd)

	if projectRoot == cwd {
		return []string{projectRoot}
	}

	// Walk from cwd upward to projectRoot, collecting dirs
	var stack []string
	current := cwd
	for {
		stack = append(stack, current)
		if current == projectRoot {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root without finding projectRoot
			break
		}
		current = parent
	}

	// Reverse so project root comes first
	for i, j := 0, len(stack)-1; i < j; i, j = i+1, j-1 {
		stack[i], stack[j] = stack[j], stack[i]
	}
	return stack
}

// resolveImports replaces @path/to/file.md references in content with the file's contents.
// Prevents circular imports by tracking already-processed paths.
func resolveImports(content, baseDir string, seen map[string]bool) string {
	if seen == nil {
		seen = make(map[string]bool)
	}

	var result strings.Builder
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)

		// Check for @import pattern: line is just "@path/to/file.md"
		if strings.HasPrefix(trimmed, "@") && strings.HasSuffix(trimmed, ".md") && !strings.Contains(trimmed, " ") {
			importPath := trimmed[1:] // strip leading @

			// Resolve relative paths
			if !filepath.IsAbs(importPath) {
				if strings.HasPrefix(importPath, "~/") {
					home, _ := os.UserHomeDir()
					importPath = filepath.Join(home, importPath[2:])
				} else {
					importPath = filepath.Join(baseDir, importPath)
				}
			}

			importPath = filepath.Clean(importPath)

			if seen[importPath] {
				result.WriteString(line)
				result.WriteString("\n")
				continue
			}
			seen[importPath] = true

			if imported := utils.ReadFileIfExists(importPath); imported != "" {
				imported = resolveImports(imported, filepath.Dir(importPath), seen)
				result.WriteString(imported)
				result.WriteString("\n")
				continue
			}
		}

		result.WriteString(line)
		result.WriteString("\n")
	}

	return strings.TrimRight(result.String(), "\n")
}

func runInteractive() error {
	if !appInstance.Auth.IsLoggedIn() {
		return fmt.Errorf("not logged in. Run: claudio auth login")
	}

	// Proactive token refresh
	if _, err := refresh.CheckAndRefreshIfNeeded(appInstance.Storage, false); err != nil {
		if flagVerbose {
			fmt.Fprintf(os.Stderr, "Warning: token refresh failed: %v\n", err)
		}
	}

	defer appInstance.Close()

	systemPrompt := buildFullSystemPrompt()

	// Start or resume session
	sess := session.New(appInstance.DB)
	if flagResume != "" {
		if flagResume == "last" {
			// Resume most recent session in this project
			recent, err := sess.RecentForProject(1)
			if err != nil || len(recent) == 0 {
				// No previous session — start fresh
				if _, err := sess.Start(appInstance.Config.Model); err != nil && flagVerbose {
					fmt.Fprintf(os.Stderr, "Warning: failed to start session: %v\n", err)
				}
			} else {
				if _, err := sess.Resume(recent[0].ID); err != nil {
					return fmt.Errorf("failed to resume last session: %w", err)
				}
				if flagVerbose {
					fmt.Fprintf(os.Stderr, "Resumed session: %s (%s)\n", recent[0].Title, recent[0].ID[:8])
				}
			}
		} else {
			// Resume by ID prefix
			resumed, err := sess.Resume(flagResume)
			if err != nil {
				return fmt.Errorf("failed to resume session %q: %w", flagResume, err)
			}
			if flagVerbose {
				fmt.Fprintf(os.Stderr, "Resumed session: %s (%s)\n", resumed.Title, resumed.ID[:8])
			}
		}
	} else {
		// Don't create a session yet — it will be created lazily on first message.
		// This avoids polluting the session list with empty sessions.
	}

	reg, modelOverride := applyAgentOverrides(appInstance.Tools)
	if modelOverride != "" {
		appInstance.Config.Model = modelOverride
		appInstance.API.SetModel(modelOverride)
	}
	engineCfg := &query.EngineConfig{
		Hooks:           appInstance.Hooks,
		Analytics:       appInstance.Analytics,
		TaskRuntime:     appInstance.TaskRuntime,
		Model:           appInstance.Config.Model,
		PermissionMode:  appInstance.Config.PermissionMode,
		PermissionRules: appInstance.Config.PermissionRules,
		OnTurnEnd:       appInstance.MemoryExtractor(),
		OnAutoCompact: func(messages []api.Message, summary string) {
			if sess != nil {
				_ = sess.PersistCompacted(messages)
				_ = sess.SaveSummary(summary)
			}
		},
	}
	appCtx := &tui.AppContext{
		Session:     sess,
		Memory:      appInstance.Memory,
		Config:      appInstance.Config,
		Analytics:   appInstance.Analytics,
		Learning:    appInstance.Learning,
		TaskRuntime: appInstance.TaskRuntime,
		DB:          appInstance.DB,
		Hooks:       appInstance.Hooks,
		Rules:       nil, // Rules are loaded separately in system prompt building
		Auditor:     appInstance.Auditor,
		TeamManager: appInstance.Teams,
		TeamRunner:  appInstance.TeamRunner,
	}
	model := tui.New(appInstance.API, reg, systemPrompt, sess,
		tui.WithSkills(appInstance.Skills),
		tui.WithEngineConfig(engineCfg),
		tui.WithAppContext(appCtx),
		tui.WithUserContext(prompts.FormatUserContextMessage(buildUserContext(), "")),
		tui.WithSystemContext(buildSystemContext()),
		tui.WithDB(appInstance.DB),
		tui.WithTeamTemplatesDir(config.GetPaths().TeamTemplates),
	)
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	// Trigger dream task if enough activity has accumulated
	triggerDreamIfNeeded(sess)

	// Print cost summary to stderr on exit
	printCostSummary()

	return nil
}

// isTerminal checks if stdout is connected to a terminal.
func isTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// triggerDreamIfNeeded spawns a background dream task if enough sessions have
// accumulated since the last memory consolidation.
func triggerDreamIfNeeded(sess *session.Session) {
	if appInstance == nil || appInstance.TaskRuntime == nil {
		return
	}

	paths := config.GetPaths()
	dreamStatePath := paths.Home + "/dream-state.json"

	state := tasks.LoadDreamState(dreamStatePath)
	state.RecordSession()

	if !state.ShouldDream() {
		state.Save(dreamStatePath)
		return
	}

	// Get session summary for the dream prompt
	summary := ""
	if sess != nil && sess.Current() != nil {
		summary = sess.Current().Summary
	}
	if summary == "" {
		summary, _, _ = sess.LastSessionSummary()
	}
	if summary == "" {
		state.Save(dreamStatePath)
		return
	}

	cwd, _ := os.Getwd()
	projectRoot := config.FindGitRoot(cwd)
	memDir := config.ProjectMemoryDir(projectRoot)

	_, err := tasks.SpawnDreamTask(appInstance.TaskRuntime, tasks.DreamTaskInput{
		SessionSummary: summary,
		ProjectDir:     cwd,
		MemoryDir:      memDir,
		RunDream: func(ctx context.Context, prompt string) (string, error) {
			if appInstance.API == nil || appInstance.Tools == nil {
				return "", fmt.Errorf("API client not available for dream task")
			}
			handler := &query.StdoutHandler{Verbose: false}
			engine := query.NewEngine(appInstance.API, appInstance.Tools, handler)
			engine.SetSystem("You are a memory consolidation agent.")
			var result strings.Builder
			if runErr := engine.Run(ctx, prompt); runErr != nil {
				return "", runErr
			}
			return result.String(), nil
		},
	})
	if err != nil {
		if flagVerbose {
			fmt.Fprintf(os.Stderr, "Warning: failed to spawn dream task: %v\n", err)
		}
	} else {
		state.RecordDream()
	}
	state.Save(dreamStatePath)
}

// printCostSummary prints session analytics to stderr on exit.
func printCostSummary() {
	if appInstance == nil || appInstance.Analytics == nil {
		return
	}
	tokens := appInstance.Analytics.TotalTokens()
	if tokens == 0 {
		return
	}
	cost := appInstance.Analytics.Cost()
	fmt.Fprintf(os.Stderr, "\n\033[2m%s\033[0m\n", appInstance.Analytics.Report())
	// Save report
	if appInstance.Analytics != nil {
		sessID := "unknown"
		appInstance.Analytics.SaveReport(sessID)
	}
	_ = cost // used by Report() internally
}
