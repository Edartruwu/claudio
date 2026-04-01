package commands

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Abraxas-365/claudio/internal/config"
)

// RegisterCoreCommands adds all built-in slash commands.
func RegisterCoreCommands(r *Registry) {
	r.Register(&Command{
		Name:        "help",
		Aliases:     []string{"h", "?"},
		Description: "Show available commands",
		Execute: func(args string) (string, error) {
			return r.HelpText(), nil
		},
	})

	r.Register(&Command{
		Name:        "clear",
		Description: "Clear the screen",
		Execute: func(args string) (string, error) {
			return "\033[2J\033[H", nil // ANSI clear
		},
	})

	r.Register(&Command{
		Name:        "model",
		Aliases:     []string{"m"},
		Description: "Show or change the AI model",
		Execute: func(args string) (string, error) {
			if args == "" {
				return "Current model: (set via --model flag or settings.json)", nil
			}
			// TODO: update model in app state
			return fmt.Sprintf("Model set to: %s", args), nil
		},
	})

	r.Register(&Command{
		Name:        "compact",
		Description: "Compact conversation history to save context",
		Execute: func(args string) (string, error) {
			// TODO: trigger compaction in query engine
			return "Compaction triggered. Use /compact --keep 20 to control how many messages to keep.", nil
		},
	})

	r.Register(&Command{
		Name:        "cost",
		Description: "Show session cost and token usage",
		Execute: func(args string) (string, error) {
			// TODO: read from session state
			return "Token usage and cost tracking (displayed in footer)", nil
		},
	})

	r.Register(&Command{
		Name:        "session",
		Aliases:     []string{"sessions"},
		Description: "List or manage sessions",
		Execute: func(args string) (string, error) {
			// TODO: list sessions from DB
			return "Session management: /session list, /session resume <id>", nil
		},
	})

	r.Register(&Command{
		Name:        "config",
		Description: "Show or edit configuration",
		Execute: func(args string) (string, error) {
			paths := config.GetPaths()
			return fmt.Sprintf("Config files:\n  User: %s\n  Local: %s\n  Home: %s",
				paths.Settings, paths.Local, paths.Home), nil
		},
	})

	r.Register(&Command{
		Name:        "commit",
		Description: "Create a git commit with AI-generated message",
		Execute: func(args string) (string, error) {
			return "[skill:commit] Analyzing changes and creating commit...", nil
		},
	})

	r.Register(&Command{
		Name:        "diff",
		Description: "Show git diff",
		Execute: func(args string) (string, error) {
			cmd := exec.Command("git", "diff")
			if args != "" {
				cmd = exec.Command("git", "diff", args)
			}
			output, err := cmd.Output()
			if err != nil {
				return fmt.Sprintf("git diff error: %v", err), nil
			}
			if len(output) == 0 {
				return "No changes", nil
			}
			return string(output), nil
		},
	})

	r.Register(&Command{
		Name:        "status",
		Description: "Show git status",
		Execute: func(args string) (string, error) {
			output, err := exec.Command("git", "status", "--short").Output()
			if err != nil {
				return fmt.Sprintf("git status error: %v", err), nil
			}
			if len(output) == 0 {
				return "Working tree clean", nil
			}
			return string(output), nil
		},
	})

	r.Register(&Command{
		Name:        "doctor",
		Description: "Diagnose environment issues",
		Execute: func(args string) (string, error) {
			return runDoctor(), nil
		},
	})

	r.Register(&Command{
		Name:        "version",
		Description: "Show version",
		Execute: func(args string) (string, error) {
			return "claudio dev", nil
		},
	})

	r.Register(&Command{
		Name:        "exit",
		Aliases:     []string{"quit", "q"},
		Description: "Exit Claudio",
		Execute: func(args string) (string, error) {
			return "", fmt.Errorf("__exit__")
		},
	})

	r.Register(&Command{
		Name:        "rename",
		Description: "Rename the current session",
		Execute: func(args string) (string, error) {
			if args == "" {
				return "Usage: /rename <name>", nil
			}
			// TODO: update session title
			return fmt.Sprintf("Session renamed to: %s", args), nil
		},
	})

	r.Register(&Command{
		Name:        "skills",
		Description: "List available skills",
		Execute: func(args string) (string, error) {
			return "Skills: /commit, /review, /simplify (more from ~/.claudio/skills/)", nil
		},
	})

	r.Register(&Command{
		Name:        "mcp",
		Description: "Manage MCP servers",
		Execute: func(args string) (string, error) {
			return "MCP server management: /mcp status, /mcp start <name>, /mcp stop <name>", nil
		},
	})

	r.Register(&Command{
		Name:        "audit",
		Description: "Show recent tool audit log",
		Execute: func(args string) (string, error) {
			return "Audit log (stored in ~/.claudio/claudio.db)", nil
		},
	})
}

func runDoctor() string {
	var lines []string
	lines = append(lines, "Claudio Environment Check")
	lines = append(lines, "========================")

	// Check tools
	checks := []struct {
		name string
		cmd  string
		args []string
	}{
		{"git", "git", []string{"--version"}},
		{"rg (ripgrep)", "rg", []string{"--version"}},
		{"gopls", "gopls", []string{"version"}},
		{"node", "node", []string{"--version"}},
	}

	for _, c := range checks {
		cmd := exec.Command(c.cmd, c.args...)
		output, err := cmd.Output()
		if err != nil {
			lines = append(lines, fmt.Sprintf("  ✗ %s: not found", c.name))
		} else {
			ver := strings.TrimSpace(strings.Split(string(output), "\n")[0])
			lines = append(lines, fmt.Sprintf("  ✓ %s: %s", c.name, ver))
		}
	}

	// Check config
	paths := config.GetPaths()
	if _, err := os.Stat(paths.Settings); err == nil {
		lines = append(lines, fmt.Sprintf("  ✓ Settings: %s", paths.Settings))
	} else {
		lines = append(lines, fmt.Sprintf("  ○ Settings: not configured (%s)", paths.Settings))
	}

	if _, err := os.Stat(paths.DB); err == nil {
		lines = append(lines, fmt.Sprintf("  ✓ Database: %s", paths.DB))
	}

	// Auth
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey != "" {
		lines = append(lines, "  ✓ Auth: ANTHROPIC_API_KEY set")
	} else {
		if _, err := os.Stat(paths.Credentials); err == nil {
			lines = append(lines, "  ✓ Auth: credentials file found")
		} else {
			lines = append(lines, "  ✗ Auth: not configured (run: claudio auth login)")
		}
	}

	lines = append(lines, fmt.Sprintf("\n  Time: %s", time.Now().Format(time.RFC3339)))
	return strings.Join(lines, "\n")
}
