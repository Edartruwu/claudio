package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/Abraxas-365/claudio/internal/app"
	"github.com/Abraxas-365/claudio/internal/auth/refresh"
	"github.com/Abraxas-365/claudio/internal/config"
	"github.com/Abraxas-365/claudio/internal/prompts"
	"github.com/Abraxas-365/claudio/internal/query"
	"github.com/Abraxas-365/claudio/internal/rules"
	"github.com/Abraxas-365/claudio/internal/session"
	"github.com/Abraxas-365/claudio/internal/tui"
	"github.com/Abraxas-365/claudio/internal/utils"
)

var (
	flagModel              string
	flagVerbose            bool
	flagHeadless           bool
	flagContext            string
	flagBudget             float64
	flagResume             string
	flagDangerouslySkipPerm bool
)

// appInstance is initialized before command execution.
var appInstance *app.App

var rootCmd = &cobra.Command{
	Use:   "claudio [prompt]",
	Short: "AI-powered coding assistant for the terminal",
	Long: `Claudio is an open-source terminal AI assistant that helps with software engineering tasks.
Built in Go with a focus on performance, security, and extensibility.`,
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

		a, err := app.New(settings)
		if err != nil {
			return fmt.Errorf("failed to initialize: %w", err)
		}
		appInstance = a
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
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
	rootCmd.PersistentFlags().StringVar(&flagResume, "resume", "", "Resume a previous session by ID prefix")
	rootCmd.PersistentFlags().BoolVar(&flagDangerouslySkipPerm, "dangerously-skip-permissions", false, "Skip all permission prompts (use with caution)")
	rootCmd.PersistentFlags().BoolVar(&flagDangerouslySkipPerm, "yolo", false, "Alias for --dangerously-skip-permissions")
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

	handler := &query.StdoutHandler{Verbose: flagVerbose}
	engine := query.NewEngineWithConfig(appInstance.API, appInstance.Tools, handler, query.EngineConfig{
		Hooks:          appInstance.Hooks,
		Analytics:      appInstance.Analytics,
		TaskRuntime:    appInstance.TaskRuntime,
		Model:          appInstance.Config.Model,
		PermissionMode: appInstance.Config.PermissionMode,
	})
	engine.SetSystem(buildFullSystemPrompt())

	return engine.Run(ctx, prompt)
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

// buildFullSystemPrompt gathers all context (rules, CLAUDE.md, context profiles)
// and builds the complete system prompt.
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

	// 2. Load CLAUDE.md / CLAUDIO.md
	claudeMD := loadCLAUDEMD(cwd, home)
	if claudeMD != "" {
		sections = append(sections, prompts.CLAUDEMDSection(claudeMD))
	}

	// 3. Load rules
	rulesReg := rules.LoadAll(
		paths.Rules,
		cwd+"/.claudio/rules",
	)
	rulesReg.LoadCLAUDEMD(cwd, home)
	if rulesContent := rulesReg.ForSystemPrompt(); rulesContent != "" {
		sections = append(sections, rulesContent)
	}

	// 4. Session memory
	if appInstance.Memory != nil {
		if memContent := appInstance.Memory.ForSystemPrompt(); memContent != "" {
			sections = append(sections, memContent)
		}
	}

	// 5. Learned instincts
	if appInstance.Learning != nil {
		if instinctContent := appInstance.Learning.ForSystemPrompt(cwd); instinctContent != "" {
			sections = append(sections, instinctContent)
		}
	}

	// Combine all additional context
	additionalCtx := strings.Join(sections, "\n\n")

	return prompts.BuildSystemPrompt(appInstance.Config.Model, additionalCtx)
}

// loadCLAUDEMD finds and loads CLAUDE.md or CLAUDIO.md from the project.
func loadCLAUDEMD(projectDir, homeDir string) string {
	// Check project directory
	for _, name := range []string{"CLAUDIO.md", "CLAUDE.md", ".claudio/CLAUDE.md"} {
		path := projectDir + "/" + name
		if content := utils.ReadFileIfExists(path); content != "" {
			return content
		}
	}
	return ""
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
		// Resume a previous session
		resumed, err := sess.Resume(flagResume)
		if err != nil {
			return fmt.Errorf("failed to resume session %q: %w", flagResume, err)
		}
		if flagVerbose {
			fmt.Fprintf(os.Stderr, "Resumed session: %s (%s)\n", resumed.Title, resumed.ID[:8])
		}
	} else {
		if _, err := sess.Start(appInstance.Config.Model); err != nil {
			if flagVerbose {
				fmt.Fprintf(os.Stderr, "Warning: failed to start session: %v\n", err)
			}
		}
	}

	engineCfg := &query.EngineConfig{
		Hooks:          appInstance.Hooks,
		Analytics:      appInstance.Analytics,
		TaskRuntime:    appInstance.TaskRuntime,
		Model:          appInstance.Config.Model,
		PermissionMode: appInstance.Config.PermissionMode,
	}
	model := tui.New(appInstance.API, appInstance.Tools, systemPrompt, sess,
		tui.WithSkills(appInstance.Skills),
		tui.WithEngineConfig(engineCfg),
	)
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	return nil
}
