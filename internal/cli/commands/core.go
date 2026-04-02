package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
		Name:        "model",
		Aliases:     []string{"m"},
		Description: "Show or change the AI model",
		Execute: func(args string) (string, error) {
			if args == "" {
				return fmt.Sprintf("Current model: %s", deps.GetModel()), nil
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
		Description: "Rename the current session",
		Execute: func(args string) (string, error) {
			if args == "" {
				return "Usage: /rename <name>", nil
			}
			if err := deps.RenameSession(args); err != nil {
				return fmt.Sprintf("Failed to rename session: %v", err), nil
			}
			return fmt.Sprintf("Session renamed to: %s", args), nil
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
					return "[skill:" + skillName + "]", nil
				},
			})
		}
	}

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
