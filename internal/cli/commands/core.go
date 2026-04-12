package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Abraxas-365/claudio/internal/config"
)

// RegisterCoreCommands adds all built-in slash commands.
func RegisterCoreCommands(r *Registry, deps *CommandDeps) {
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
		Name:        "agent",
		Description: "Switch agent persona for this session",
		Execute: func(args string) (string, error) {
			return "", nil // handled directly in TUI root
		},
	})

	r.Register(&Command{
		Name:        "model",
		Aliases:     []string{"m"},
		Description: "Show or change the AI model",
		Execute: func(args string) (string, error) {
			if args == "" {
				info := fmt.Sprintf("Current model: %s", deps.GetModel())
				if deps.GetThinkingLabel != nil {
					info += fmt.Sprintf("\nThinking: %s", deps.GetThinkingLabel())
				}
				return info, nil
			}
			deps.SetModel(args)
			return fmt.Sprintf("Model set to: %s", args), nil
		},
	})

	r.Register(&Command{
		Name:        "compact",
		Description: "Compact conversation history to save context",
		Execute: func(args string) (string, error) {
			keepLast := 10
			if args != "" {
				if _, err := fmt.Sscanf(args, "%d", &keepLast); err != nil {
					return "Usage: /compact [number-of-messages-to-keep]", nil
				}
			}
			summary, err := deps.Compact(keepLast)
			if err != nil {
				return fmt.Sprintf("Compaction failed: %v", err), nil
			}
			if summary == "" {
				return "Nothing to compact (conversation too short).", nil
			}
			return fmt.Sprintf("Compacted conversation. Summary:\n%s", summary), nil
		},
	})

	r.Register(&Command{
		Name:        "cost",
		Description: "Show session cost and token usage",
		Execute: func(args string) (string, error) {
			tokens := deps.GetTokens()
			cost := deps.GetCost()
			return fmt.Sprintf("Session usage:\n  Tokens: %d\n  Cost:   $%.4f", tokens, cost), nil
		},
	})

	r.Register(&Command{
		Name:        "memory",
		Aliases:     []string{"mem"},
		Description: "Memory management: /memory extract (extract from current conversation)",
		Execute: func(args string) (string, error) {
			args = strings.TrimSpace(args)
			switch args {
			case "extract":
				if deps.ExtractMemories == nil {
					return "Memory extraction not available.", nil
				}
				count, err := deps.ExtractMemories()
				if err != nil {
					return fmt.Sprintf("Extraction failed: %v", err), nil
				}
				if count == 0 {
					return "No new memories extracted from this conversation.", nil
				}
				return fmt.Sprintf("Extracted %d memory(ies) from this conversation.", count), nil
			case "":
				return "Usage: /memory extract — extract memories from current conversation\n  Use <Space>im to browse memories", nil
			default:
				return fmt.Sprintf("Unknown subcommand: %s. Use: /memory extract", args), nil
			}
		},
	})

	r.Register(&Command{
		Name:        "session",
		Aliases:     []string{"sessions"},
		Description: "List or manage sessions",
		Execute: func(args string) (string, error) {
			sessions, err := deps.ListSessions(20)
			if err != nil {
				return fmt.Sprintf("Failed to list sessions: %v", err), nil
			}
			if len(sessions) == 0 {
				return "No sessions found.", nil
			}
			var lines []string
			lines = append(lines, "Recent sessions:")
			for _, s := range sessions {
				title := s.Title
				if title == "" {
					title = "(untitled)"
				}
				lines = append(lines, fmt.Sprintf("  %s  %s  [%s]  %s",
					s.ID[:8], title, s.Model, s.UpdatedAt))
			}
			return strings.Join(lines, "\n"), nil
		},
	})

	r.Register(&Command{
		Name:        "config",
		Aliases:     []string{"settings"},
		Description: "View or edit configuration: /config [show|json|edit|path|set key value|validate]",
		Execute: func(args string) (string, error) {
			paths := config.GetPaths()
			cwd, _ := os.Getwd()

			parts := strings.Fields(args)
			subCmd := "show"
			if len(parts) > 0 {
				subCmd = parts[0]
			}

			switch subCmd {
			case "show", "":
				// Load current settings and display formatted
				settings, err := config.Load(cwd)
				if err != nil {
					return fmt.Sprintf("Error loading config: %v", err), nil
				}
				return config.FormatSettings(settings, nil), nil

			case "json":
				// Show raw JSON
				settings, err := config.Load(cwd)
				if err != nil {
					return fmt.Sprintf("Error loading config: %v", err), nil
				}
				return config.FormatSettingsJSON(settings), nil

			case "edit":
				// Open settings in $EDITOR
				editor := os.Getenv("EDITOR")
				if editor == "" {
					editor = os.Getenv("VISUAL")
				}
				if editor == "" {
					editor = "vim"
				}
				target := paths.Settings
				if len(parts) > 1 {
					switch parts[1] {
					case "project":
						target = cwd + "/.claudio/settings.json"
						os.MkdirAll(cwd+"/.claudio", 0755)
						// Create if doesn't exist
						if _, err := os.Stat(target); os.IsNotExist(err) {
							os.WriteFile(target, []byte("{}\n"), 0644)
						}
					case "local":
						target = paths.Local
					case "user":
						target = paths.Settings
					}
				}
				return fmt.Sprintf("[exec:%s %s]", editor, target), nil

			case "path", "paths":
				var lines []string
				lines = append(lines, "Configuration file locations:")
				lines = append(lines, fmt.Sprintf("  User settings:    %s", paths.Settings))
				lines = append(lines, fmt.Sprintf("  Local settings:   %s", paths.Local))
				lines = append(lines, fmt.Sprintf("  Project settings: %s/.claudio/settings.json", cwd))
				lines = append(lines, fmt.Sprintf("  Home directory:   %s", paths.Home))
				lines = append(lines, fmt.Sprintf("  Database:         %s", paths.DB))
				lines = append(lines, fmt.Sprintf("  Skills:           %s", paths.Skills))
				lines = append(lines, fmt.Sprintf("  Rules:            %s", paths.Rules))
				lines = append(lines, fmt.Sprintf("  Agents:           %s", paths.Agents))
				lines = append(lines, fmt.Sprintf("  Memory:           %s", paths.Memory))
				lines = append(lines, fmt.Sprintf("  Logs:             %s", paths.Logs))
				lines = append(lines, fmt.Sprintf("  Plugins:          %s", paths.Plugins))
				return strings.Join(lines, "\n"), nil

			case "set":
				if len(parts) < 3 {
					return "Usage: /config set <key> <value>\n\nKeys: model, permissionMode, compactMode, autoCompact, maxBudget, apiBaseUrl", nil
				}
				key := parts[1]
				value := strings.Join(parts[2:], " ")
				return setConfigValue(paths.Settings, key, value)

			case "validate":
				settings, err := config.Load(cwd)
				if err != nil {
					return fmt.Sprintf("Error loading config: %v", err), nil
				}
				errs := config.ValidateSettings(settings)
				if len(errs) == 0 {
					return "Configuration is valid.", nil
				}
				var lines []string
				lines = append(lines, fmt.Sprintf("Found %d issue(s):", len(errs)))
				for _, e := range errs {
					lines = append(lines, "  - "+e.String())
				}
				return strings.Join(lines, "\n"), nil

			case "trust":
				ts := config.NewTrustStore()
				if len(parts) > 1 && parts[1] == "list" {
					// This would need exposing the projects map; simplified for now
					return "Trust store: " + paths.Home + "/trusted.json", nil
				}
				if ts.IsTrusted(cwd) {
					return fmt.Sprintf("Project %s is trusted.", cwd), nil
				}
				return fmt.Sprintf("Project %s is NOT trusted. Run claudio in the directory to trigger trust prompt.", cwd), nil

			default:
				return "Usage: /config [show|json|edit|path|set key value|validate|trust]\n\n" +
					"  show      — Display current merged configuration\n" +
					"  json      — Show settings as raw JSON\n" +
					"  edit      — Open settings in $EDITOR (add: project, local, user)\n" +
					"  path      — Show all config file locations\n" +
					"  set       — Set a config value: /config set model claude-opus-4-6\n" +
					"  validate  — Check settings for errors\n" +
					"  trust     — Show trust status for current project", nil
			}
		},
	})

	r.Register(&Command{
		Name:        "init",
		Description: "Initialize CLAUDIO.md, skills, and hooks for this project",
		Execute: func(args string) (string, error) {
			return "[skill:init]", nil
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
		Description: "Show git diff (or /diff turn N for per-turn changes)",
		Execute: func(args string) (string, error) {
			// Check for "turn N" subcommand
			args = strings.TrimSpace(args)
			if strings.HasPrefix(args, "turn ") || strings.HasPrefix(args, "--turn ") {
				turnStr := strings.TrimPrefix(strings.TrimPrefix(args, "--turn "), "turn ")
				turnStr = strings.TrimSpace(turnStr)
				var turnNum int
				if _, err := fmt.Sscanf(turnStr, "%d", &turnNum); err != nil {
					return "Usage: /diff turn <number>", nil
				}
				if deps.GetTurnDiff != nil {
					return deps.GetTurnDiff(turnNum), nil
				}
				return "Turn diff tracking not available.", nil
			}

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
		Name:        "vim",
		Description: "Toggle vim keybindings",
		Execute: func(args string) (string, error) {
			enabled := deps.ToggleVim()
			if enabled {
				return "Vim mode enabled (Esc → Normal, i → Insert)", nil
			}
			return "Vim mode disabled", nil
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
		Aliases:     []string{"rn"},
		Description: "Rename this session: no args = AI auto-name, with args = rename to that",
		Execute: func(args string) (string, error) {
			if args != "" {
				if err := deps.RenameSession(args); err != nil {
					return fmt.Sprintf("Failed to rename session: %v", err), nil
				}
				return fmt.Sprintf("Session renamed to: %s", args), nil
			}
			if deps.AutoNameSession == nil {
				return "Auto-naming not available.", nil
			}
			name, err := deps.AutoNameSession()
			if err != nil {
				return fmt.Sprintf("Auto-naming failed: %v", err), nil
			}
			return fmt.Sprintf("Session named: %s", name), nil
		},
	})

	r.Register(&Command{
		Name:        "skills",
		Description: "List available skills",
		Execute: func(args string) (string, error) {
			if deps.ListSkills == nil {
				return "Skills: /commit, /review, /simplify (more from ~/.claudio/skills/)", nil
			}
			skills := deps.ListSkills()
			if len(skills) == 0 {
				return "No skills loaded.", nil
			}
			var lines []string
			lines = append(lines, "Available skills:")
			for _, s := range skills {
				lines = append(lines, fmt.Sprintf("  /%-20s %s", s.Name, s.Description))
			}
			lines = append(lines, "\nUser skills: ~/.claudio/skills/")
			lines = append(lines, "Project skills: .claudio/skills/")
			return strings.Join(lines, "\n"), nil
		},
	})

	r.Register(&Command{
		Name:        "new",
		Description: "Start a new session",
		Execute: func(args string) (string, error) {
			if deps.NewSession == nil {
				return "No session manager available", nil
			}
			if err := deps.NewSession(); err != nil {
				return "", err
			}
			return "__new_session__", nil
		},
	})

	r.Register(&Command{
		Name:        "resume",
		Description: "Resume a previous session by ID prefix",
		Execute: func(args string) (string, error) {
			if args == "" {
				return "Usage: /resume <session-id-prefix>", nil
			}
			return "[resume:" + args + "]", nil
		},
	})

	// Dynamically register all loaded skills as slash commands
	if deps.ListSkills != nil {
		for _, skill := range deps.ListSkills() {
			skillName := skill.Name
			// Don't override existing commands
			if _, exists := r.Get(skillName); exists {
				continue
			}
			r.Register(&Command{
				Name:        skillName,
				Description: skill.Description,
				Execute: func(args string) (string, error) {
					return "[skill:" + skillName + "]" + args, nil
				},
			})
		}
	}

	r.Register(&Command{
		Name:        "mcp",
		Description: "Manage MCP servers: /mcp [status|list]",
		Execute: func(args string) (string, error) {
			return "MCP server management:\n  /mcp status  — show running servers\n  /mcp list    — list configured servers\n\nConfigure in settings.json under \"mcpServers\".", nil
		},
	})

	r.Register(&Command{
		Name:        "audit",
		Description: "Show recent tool audit log",
		Execute: func(args string) (string, error) {
			return "Audit log (stored in ~/.claudio/claudio.db)", nil
		},
	})

	// --- New commands ---

	r.Register(&Command{
		Name:        "clear",
		Description: "Clear conversation history (keeps session)",
		Execute: func(args string) (string, error) {
			return "[action:clear]", nil
		},
	})

	r.Register(&Command{
		Name:        "undo",
		Description: "Undo the last exchange (removes last user + assistant message)",
		Execute: func(args string) (string, error) {
			return "[action:undo]", nil
		},
	})

	r.Register(&Command{
		Name:        "redo",
		Description: "Redo the last undone exchange",
		Execute: func(args string) (string, error) {
			return "[action:redo]", nil
		},
	})

	r.Register(&Command{
		Name:        "themes",
		Description: "List available themes",
		Execute: func(args string) (string, error) {
			return "Available themes:\n  • gruvbox (active)\n\nTheme switching is not yet implemented — only gruvbox is bundled. To request more themes, file an issue.", nil
		},
	})

	r.Register(&Command{
		Name:        "details",
		Description: "Toggle expand/collapse all tool execution details",
		Execute: func(args string) (string, error) {
			return "[action:details]", nil
		},
	})

	r.Register(&Command{
		Name:        "thinking",
		Description: "Toggle visibility of model thinking/reasoning blocks",
		Execute: func(args string) (string, error) {
			return "[action:thinking]", nil
		},
	})

	r.Register(&Command{
		Name:        "editor",
		Aliases:     []string{"e"},
		Description: "Compose a message in your $EDITOR",
		Execute: func(args string) (string, error) {
			return "[action:editor]", nil
		},
	})

	r.Register(&Command{
		Name:        "connect",
		Description: "Configure AI providers and API keys",
		Execute: func(args string) (string, error) {
			return "Provider configuration:\n  Edit ~/.claudio/config.toml or use environment variables:\n    ANTHROPIC_API_KEY=...\n    OPENAI_API_KEY=...\n  Then run /model <model-name> to switch.\n\nA full provider wizard is planned for a future release.", nil
		},
	})

	r.Register(&Command{
		Name:        "fork",
		Description: "Fork current conversation to a background agent",
		Execute: func(args string) (string, error) {
			prompt := args
			if prompt == "" {
				prompt = "Continue working on the current task"
			}
			return "[action:fork:" + prompt + "]", nil
		},
	})

	r.Register(&Command{
		Name:        "export",
		Description: "Export conversation: /export [markdown|json|txt]",
		Execute: func(args string) (string, error) {
			format := "markdown"
			if args != "" {
				format = strings.TrimSpace(args)
			}
			return "[action:export:" + format + "]", nil
		},
	})

	r.Register(&Command{
		Name:        "tasks",
		Description: "Show background tasks and team status",
		Execute: func(args string) (string, error) {
			return "[action:tasks]", nil
		},
	})

	r.Register(&Command{
		Name:        "bug",
		Aliases:     []string{"feedback"},
		Description: "Report a bug or give feedback",
		Execute: func(args string) (string, error) {
			var lines []string
			lines = append(lines, "Report issues or give feedback:")
			lines = append(lines, "  https://github.com/Abraxas-365/claudio/issues")
			lines = append(lines, "")
			lines = append(lines, "Include:")
			lines = append(lines, fmt.Sprintf("  Version: claudio %s", "dev"))
			lines = append(lines, fmt.Sprintf("  OS: %s/%s", runtime.GOOS, runtime.GOARCH))
			lines = append(lines, fmt.Sprintf("  Model: %s", deps.GetModel()))
			return strings.Join(lines, "\n"), nil
		},
	})

	r.Register(&Command{
		Name:        "team",
		Aliases:     []string{"teams"},
		Description: "Use agent teams: /team <prompt> — tells the AI to use a team for your request",
		Execute: func(args string) (string, error) {
			if args == "" {
				// No args: show team listing if available
				if deps.ListTeams != nil {
					if out := deps.ListTeams(); out != "" {
						return out, nil
					}
				}
				return "No teams active. Use /team <prompt> to have the AI create and manage a team for your task.", nil
			}
			// Forward to AI with team instruction
			return "[team:" + args + "]", nil
		},
	})

	// /plugins — list installed plugins
	r.Register(&Command{
		Name:        "plugins",
		Description: "List installed plugins",
		Execute: func(args string) (string, error) {
			if deps.ListPlugins == nil {
				return "Plugin system not available.", nil
			}
			return deps.ListPlugins(), nil
		},
	})

	// /share — export session for sharing
	r.Register(&Command{
		Name:        "share",
		Description: "Export current session for sharing",
		Execute: func(args string) (string, error) {
			if deps.ShareSession == nil {
				return "Session sharing not available.", nil
			}
			path := strings.TrimSpace(args)
			if path == "" {
				path = fmt.Sprintf("claudio-session-%s.json", time.Now().Format("20060102-150405"))
			}
			return deps.ShareSession(path)
		},
	})

	// /teleport — import shared session
	r.Register(&Command{
		Name:        "teleport",
		Description: "Import a shared session file",
		Execute: func(args string) (string, error) {
			if deps.TeleportSession == nil {
				return "Session teleport not available.", nil
			}
			path := strings.TrimSpace(args)
			if path == "" {
				return "Usage: /teleport <path-to-session-file>", nil
			}
			return deps.TeleportSession(path)
		},
	})

	// /keybindings — open keybindings config in editor
	r.Register(&Command{
		Name:        "keybindings",
		Description: "Open keybindings.json in your editor",
		Execute: func(args string) (string, error) {
			kbPath := filepath.Join(config.GetPaths().Home, "keybindings.json")

			// Create template if file doesn't exist
			if _, err := os.Stat(kbPath); os.IsNotExist(err) {
				// Import and write template
				template := []byte("[\n  // Customize your keybindings here.\n  // See default bindings with: claudio keybindings --defaults\n]\n")
				os.WriteFile(kbPath, template, 0644)
			}

			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "vi"
			}
			return fmt.Sprintf("[action:edit:%s:%s]", editor, kbPath), nil
		},
	})

	// /output-style — show or set output formatting style
	r.Register(&Command{
		Name:        "output-style",
		Description: "Show or set output style (normal, concise, verbose, markdown)",
		Execute: func(args string) (string, error) {
			if deps.GetOutputStyle == nil {
				return "Output style not available.", nil
			}
			if args == "" {
				style := deps.GetOutputStyle()
				if style == "" {
					style = "normal"
				}
				return fmt.Sprintf("Current output style: %s\nAvailable: normal, concise, verbose, markdown", style), nil
			}
			args = strings.TrimSpace(strings.ToLower(args))
			switch args {
			case "normal", "concise", "verbose", "markdown":
				deps.SetOutputStyle(args)
				return fmt.Sprintf("Output style set to: %s", args), nil
			default:
				return fmt.Sprintf("Unknown style %q. Available: normal, concise, verbose, markdown", args), nil
			}
		},
	})

	r.Register(&Command{
		Name:        "extra-usage",
		Aliases:     []string{"usage"},
		Description: "Open extra usage settings in browser",
		Execute: func(args string) (string, error) {
			url := "https://claude.ai/settings/usage"
			if err := openBrowser(url); err != nil {
				return fmt.Sprintf("Could not open browser: %v\nVisit: %s", err, url), nil
			}
			return fmt.Sprintf("Opened %s in your browser.", url), nil
		},
	})

	r.Register(&Command{
		Name:        "web",
		Description: "Start web server to access session from browser/phone",
		Execute: func(args string) (string, error) {
			return "", nil // handled directly in TUI root
		},
	})
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	default:
		return fmt.Errorf("unsupported platform %s", runtime.GOOS)
	}
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

	// Network connectivity
	lines = append(lines, "\nNetwork:")
	if netCmd := exec.Command("curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", "--max-time", "5", "https://api.anthropic.com"); true {
		output, err := netCmd.Output()
		if err != nil {
			lines = append(lines, "  ✗ API connectivity: unreachable")
		} else {
			code := strings.TrimSpace(string(output))
			if code == "200" || code == "301" || code == "401" || code == "403" {
				lines = append(lines, fmt.Sprintf("  ✓ API connectivity: ok (HTTP %s)", code))
			} else {
				lines = append(lines, fmt.Sprintf("  ○ API connectivity: HTTP %s", code))
			}
		}
	}

	// Memory system health
	lines = append(lines, "\nMemory:")
	memoryDir := paths.Memory
	if entries, err := os.ReadDir(memoryDir); err == nil {
		mdCount := 0
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				mdCount++
			}
		}
		lines = append(lines, fmt.Sprintf("  ✓ Global memory: %d files", mdCount))
	} else {
		lines = append(lines, "  ○ Global memory: empty")
	}

	// Database integrity
	if _, err := os.Stat(paths.DB); err == nil {
		dbCheck := exec.Command("sqlite3", paths.DB, "PRAGMA integrity_check;")
		if out, err := dbCheck.Output(); err == nil {
			result := strings.TrimSpace(string(out))
			if result == "ok" {
				lines = append(lines, "  ✓ Database: integrity ok")
			} else {
				lines = append(lines, fmt.Sprintf("  ✗ Database: %s", result))
			}
		}
	}

	// Plugins
	if entries, err := os.ReadDir(paths.Plugins); err == nil {
		pluginCount := 0
		for _, e := range entries {
			if !e.IsDir() {
				pluginCount++
			}
		}
		if pluginCount > 0 {
			lines = append(lines, fmt.Sprintf("  ✓ Plugins: %d installed", pluginCount))
		}
	}

	// Platform info
	lines = append(lines, fmt.Sprintf("\nPlatform: %s/%s", runtime.GOOS, runtime.GOARCH))
	lines = append(lines, fmt.Sprintf("  Go: %s", runtime.Version()))

	// Disk space for ~/.claudio/
	if info, err := os.Stat(paths.Home); err == nil && info.IsDir() {
		var totalSize int64
		filepath.Walk(paths.Home, func(_ string, info os.FileInfo, _ error) error {
			if info != nil && !info.IsDir() {
				totalSize += info.Size()
			}
			return nil
		})
		lines = append(lines, fmt.Sprintf("  Disk usage: %.1f MB (%s)", float64(totalSize)/1024/1024, paths.Home))
	}

	// Project config
	cwd, _ := os.Getwd()
	projectConfig := filepath.Join(cwd, ".claudio")
	if _, err := os.Stat(projectConfig); err == nil {
		lines = append(lines, fmt.Sprintf("  ✓ Project config: %s", projectConfig))
	} else {
		lines = append(lines, "  ○ Project config: not initialized (run: claudio init)")
	}

	lines = append(lines, fmt.Sprintf("\n  Time: %s", time.Now().Format(time.RFC3339)))
	return strings.Join(lines, "\n")
}

// setConfigValue updates a single value in a settings.json file.
func setConfigValue(settingsPath, key, value string) (string, error) {
	// Read existing
	var settings map[string]interface{}
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			settings = make(map[string]interface{})
		} else {
			return fmt.Sprintf("Error reading %s: %v", settingsPath, err), nil
		}
	} else {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Sprintf("Error parsing %s: %v", settingsPath, err), nil
		}
	}

	// Parse value type
	var parsed interface{}
	switch {
	case value == "true":
		parsed = true
	case value == "false":
		parsed = false
	case value == "null" || value == "":
		delete(settings, key)
		goto write
	default:
		// Try number
		var n float64
		if _, err := fmt.Sscanf(value, "%f", &n); err == nil && !strings.Contains(value, " ") {
			parsed = n
		} else {
			parsed = value
		}
	}

	settings[key] = parsed

write:
	output, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Sprintf("Error marshaling: %v", err), nil
	}

	dir := filepath.Dir(settingsPath)
	os.MkdirAll(dir, 0755)

	if err := os.WriteFile(settingsPath, output, 0644); err != nil {
		return fmt.Sprintf("Error writing %s: %v", settingsPath, err), nil
	}

	return fmt.Sprintf("Set %s = %v in %s", key, parsed, settingsPath), nil
}
