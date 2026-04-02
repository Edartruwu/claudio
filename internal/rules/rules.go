// Package rules loads and manages project/user rules that get injected
// into the system prompt to guide AI behavior.
package rules

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Abraxas-365/claudio/internal/utils"
)

// Rule represents a loaded rule.
type Rule struct {
	Name    string
	Content string
	Source  string // "user", "project", "managed"
	Path    string
}

// Registry holds all loaded rules.
type Registry struct {
	rules []*Rule
}

// NewRegistry creates an empty rule registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// LoadAll loads rules from all standard locations.
// Priority: project rules > user rules (project rules override user rules with same name).
func LoadAll(userRulesDir, projectRulesDir string) *Registry {
	r := NewRegistry()

	// Load user rules (~/.claudio/rules/)
	if userRulesDir != "" {
		r.loadDir(userRulesDir, "user")
	}

	// Load project rules (.claudio/rules/)
	if projectRulesDir != "" {
		r.loadDir(projectRulesDir, "project")
	}

	return r
}

// LoadCLAUDEMD loads CLAUDE.md / CLAUDIO.md files from project root and user home.
func (r *Registry) LoadCLAUDEMD(projectDir, homeDir string) {
	// Project-level CLAUDE.md / CLAUDIO.md
	for _, name := range []string{"CLAUDIO.md", "CLAUDE.md", ".claudio/CLAUDE.md"} {
		path := filepath.Join(projectDir, name)
		if content := utils.ReadFileIfExists(path); content != "" {
			r.rules = append(r.rules, &Rule{
				Name:    name,
				Content: content,
				Source:  "project",
				Path:    path,
			})
			break
		}
	}

	// User-level CLAUDE.md
	if homeDir != "" {
		path := filepath.Join(homeDir, ".claudio", "CLAUDE.md")
		if content := utils.ReadFileIfExists(path); content != "" {
			r.rules = append(r.rules, &Rule{
				Name:    "user-claude-md",
				Content: content,
				Source:  "user",
				Path:    path,
			})
		}
	}
}

// All returns all loaded rules.
func (r *Registry) All() []*Rule {
	return r.rules
}

// ForSystemPrompt returns all rules formatted for injection into the system prompt.
func (r *Registry) ForSystemPrompt() string {
	if len(r.rules) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("# Project Rules & Instructions\n\n")
	sb.WriteString("The following rules and instructions have been configured. You MUST follow them.\n\n")

	for _, rule := range r.rules {
		sb.WriteString("## ")
		sb.WriteString(rule.Name)
		sb.WriteString(" (")
		sb.WriteString(rule.Source)
		sb.WriteString(")\n\n")
		sb.WriteString(rule.Content)
		sb.WriteString("\n\n")
	}

	return sb.String()
}

// Count returns the number of loaded rules.
func (r *Registry) Count() int {
	return len(r.rules)
}

func (r *Registry) loadDir(dir, source string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			// Recurse into subdirectories (e.g., rules/golang/, rules/security/)
			subdir := filepath.Join(dir, entry.Name())
			r.loadDir(subdir, source)
			continue
		}

		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		content := string(data)
		fm, body := utils.ParseFrontmatter(content)

		name := fm.Get("name")
		if name == "" {
			name = strings.TrimSuffix(entry.Name(), ".md")
		}

		r.rules = append(r.rules, &Rule{
			Name:    name,
			Content: body,
			Source:  source,
			Path:    path,
		})
	}
}
