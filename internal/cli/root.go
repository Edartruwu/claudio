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
	"github.com/Abraxas-365/claudio/internal/query"
	"github.com/Abraxas-365/claudio/internal/tui"
)

var (
	flagModel    string
	flagVerbose  bool
	flagHeadless bool
	flagContext  string
	flagBudget   float64
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
		settings, err := config.Load(cwd)
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
	engine := query.NewEngine(appInstance.API, appInstance.Tools, handler)

	// Load context profile if specified
	if flagContext != "" {
		systemPrompt, err := loadContextProfile(flagContext)
		if err != nil {
			return err
		}
		engine.SetSystem(systemPrompt)
	}

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

	// Load context profile
	var systemPrompt string
	if flagContext != "" {
		var err error
		systemPrompt, err = loadContextProfile(flagContext)
		if err != nil {
			return err
		}
	}

	model := tui.New(appInstance.API, appInstance.Tools, systemPrompt)
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	return nil
}
