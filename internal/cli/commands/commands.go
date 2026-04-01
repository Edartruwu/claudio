package commands

import (
	"fmt"
	"sort"
	"strings"
)

// Command represents a slash command.
type Command struct {
	Name        string
	Aliases     []string
	Description string
	Execute     func(args string) (string, error)
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
