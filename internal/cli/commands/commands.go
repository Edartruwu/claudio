package commands

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Abraxas-365/claudio/internal/config"
)

// Command represents a slash command.
type Command struct {
	Name        string
	Aliases     []string
	Description string
	Execute      func(args string) (string, error)
	ArgCompleter func(argPrefix string) []string
}

// CommandDeps provides access to app state for commands that need it.
type CommandDeps struct {
	GetModel        func() string
	SetModel        func(model string)
	GetThinkingLabel func() string // returns human-readable thinking mode
	Compact         func(keepLast int, instruction string) (string, error)
	GetTokens       func() int
	GetCost         func() float64
	ListSessions    func(limit int) ([]SessionInfo, error)
	RenameSession   func(title string) error
	ToggleVim       func() bool // returns new state (true=enabled)
	// Session lifecycle
	NewSession    func() error
	// Memory
	ExtractMemories func() (int, error) // manually trigger extraction, returns count of memories saved
	RunDream        func(hint string) (string, error) // consolidate session memories, returns result
	// Skills for dynamic registration
	ListSkills    func() []SkillInfo
	// Plugins
	ListPlugins   func() string
	// Diff tracking
	GetTurnDiff   func(turn int) string
	// Output style
	GetOutputStyle func() string
	SetOutputStyle func(style string)
	// Session sharing
	ShareSession  func(path string) (string, error)
	TeleportSession func(path string) (string, error)
	// Teams
	ListTeams func() string // returns formatted team/agent listing
	// AutoNameSession generates a name for the current session via AI
	AutoNameSession func() (string, error)
	// Lua
	ExecLua func(code string) (string, error)
	// UI/Theme
	SetTheme  func(colors map[string]string)
	SetColor  func(slot, hex string) error
	SetBorder func(style string) error
	GetColors func() map[string]string
	// Config
	GetConfig  func() *config.Settings
	SaveConfig func(*config.Settings) error
	// Window manager
	OpenWindow  func(name string) error
	CloseWindow func(name string)
}

// SkillInfo holds skill data for command registration.
type SkillInfo struct {
	Name        string
	Description string
}

// SessionInfo holds session data for display.
type SessionInfo struct {
	ID        string
	Title     string
	Model     string
	UpdatedAt string
}

// Registry holds all registered slash commands.
type Registry struct {
	commands map[string]*Command
}

// NewRegistry creates a new command registry.
func NewRegistry() *Registry {
	return &Registry{commands: make(map[string]*Command)}
}

// Register adds a command.
func (r *Registry) Register(cmd *Command) {
	r.commands[cmd.Name] = cmd
	for _, alias := range cmd.Aliases {
		r.commands[alias] = cmd
	}
}

// Get looks up a command by name.
func (r *Registry) Get(name string) (*Command, bool) {
	cmd, ok := r.commands[name]
	return cmd, ok
}

// Parse checks if input starts with "/" and returns the command name and args.
func Parse(input string) (name, args string, isCommand bool) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return "", "", false
	}

	input = input[1:] // strip "/"
	parts := strings.SplitN(input, " ", 2)
	name = parts[0]
	if len(parts) > 1 {
		args = parts[1]
	}
	return name, args, true
}

// ListCommands returns all unique commands (deduplicating aliases), sorted by name.
func (r *Registry) ListCommands() []*Command {
	seen := make(map[*Command]bool)
	var cmds []*Command
	for _, cmd := range r.commands {
		if !seen[cmd] {
			seen[cmd] = true
			cmds = append(cmds, cmd)
		}
	}
	sort.Slice(cmds, func(i, j int) bool {
		return cmds[i].Name < cmds[j].Name
	})
	return cmds
}

// Execute looks up a command by name and runs it with the given args.
func (r *Registry) Execute(name, args string) (string, error) {
	cmd, ok := r.commands[name]
	if !ok {
		return "", fmt.Errorf("unknown command: %s", name)
	}
	if cmd.Execute == nil {
		return "", fmt.Errorf("command %q has no executor", name)
	}
	return cmd.Execute(args)
}

// HelpText returns formatted help for all commands.
func (r *Registry) HelpText() string {
	// Deduplicate (aliases point to same command)
	seen := make(map[*Command]bool)
	var cmds []*Command
	for _, cmd := range r.commands {
		if !seen[cmd] {
			seen[cmd] = true
			cmds = append(cmds, cmd)
		}
	}

	sort.Slice(cmds, func(i, j int) bool {
		return cmds[i].Name < cmds[j].Name
	})

	var lines []string
	for _, cmd := range cmds {
		line := fmt.Sprintf("  /%s", cmd.Name)
		if len(cmd.Aliases) > 0 {
			line += fmt.Sprintf(" (/%s)", strings.Join(cmd.Aliases, ", /"))
		}
		line += fmt.Sprintf(" — %s", cmd.Description)
		lines = append(lines, line)
	}

	return "Available commands:\n" + strings.Join(lines, "\n")
}
