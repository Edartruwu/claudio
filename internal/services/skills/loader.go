package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Skill represents a loaded skill definition.
type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"` // The prompt/instruction content
	Source      string `json:"source"`  // "bundled", "user", "project", "plugin"
	FilePath    string `json:"file_path,omitempty"`
}

// Registry holds all loaded skills.
type Registry struct {
	mu     sync.RWMutex
	skills map[string]*Skill
}

// NewRegistry creates a new skill registry.
func NewRegistry() *Registry {
	return &Registry{
		skills: make(map[string]*Skill),
	}
}

// Register adds a skill.
func (r *Registry) Register(skill *Skill) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.skills[skill.Name] = skill
}

// Get retrieves a skill by name.
func (r *Registry) Get(name string) (*Skill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.skills[name]
	return s, ok
}

// All returns all loaded skills.
func (r *Registry) All() []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Skill, 0, len(r.skills))
	for _, s := range r.skills {
		result = append(result, s)
	}
	return result
}

// LoadAll loads skills from all sources: bundled, user, project.
func LoadAll(userSkillsDir, projectSkillsDir string) *Registry {
	r := NewRegistry()

	// 1. Bundled skills
	for _, s := range bundledSkills() {
		r.Register(s)
	}

	// 2. User skills (~/.claudio/skills/)
	if userSkillsDir != "" {
		loadFromDir(r, userSkillsDir, "user")
	}

	// 3. Project skills (.claudio/skills/)
	if projectSkillsDir != "" {
		loadFromDir(r, projectSkillsDir, "project")
	}

	return r
}

func loadFromDir(r *Registry, dir, source string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			// Look for skill.md or index.md inside the directory
			for _, fname := range []string{"skill.md", "index.md", "README.md"} {
				path := filepath.Join(dir, entry.Name(), fname)
				if content, err := os.ReadFile(path); err == nil {
					name, desc, body := parseSkillFile(string(content))
					if name == "" {
						name = entry.Name()
					}
					r.Register(&Skill{
						Name:        name,
						Description: desc,
						Content:     body,
						Source:      source,
						FilePath:    path,
					})
					break
				}
			}
		} else if strings.HasSuffix(entry.Name(), ".md") {
			path := filepath.Join(dir, entry.Name())
			content, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			name, desc, body := parseSkillFile(string(content))
			if name == "" {
				name = strings.TrimSuffix(entry.Name(), ".md")
			}
			r.Register(&Skill{
				Name:        name,
				Description: desc,
				Content:     body,
				Source:      source,
				FilePath:    path,
			})
		}
	}
}

// parseSkillFile extracts frontmatter (name, description) and body from a skill file.
func parseSkillFile(content string) (name, description, body string) {
	lines := strings.Split(content, "\n")

	// Check for YAML frontmatter
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		endIdx := -1
		for i := 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "---" {
				endIdx = i
				break
			}
		}
		if endIdx > 0 {
			// Parse frontmatter
			for _, line := range lines[1:endIdx] {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "name:") {
					name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
					name = strings.Trim(name, `"'`)
				}
				if strings.HasPrefix(line, "description:") {
					description = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
					description = strings.Trim(description, `"'`)
				}
			}
			body = strings.Join(lines[endIdx+1:], "\n")
			return
		}
	}

	body = content
	// Try to extract name from first heading
	for _, line := range lines {
		if strings.HasPrefix(line, "# ") {
			name = strings.TrimPrefix(line, "# ")
			break
		}
	}
	return
}

// bundledSkills returns the built-in skills.
func bundledSkills() []*Skill {
	return []*Skill{
		{
			Name:        "commit",
			Description: "Create a git commit with a well-crafted message",
			Content:     commitSkillContent,
			Source:      "bundled",
		},
		{
			Name:        "review",
			Description: "Review code changes for quality and security",
			Content:     reviewSkillContent,
			Source:      "bundled",
		},
		{
			Name:        "simplify",
			Description: "Review changed code for reuse, quality, and efficiency",
			Content:     simplifySkillContent,
			Source:      "bundled",
		},
	}
}

var commitSkillContent = `Analyze all staged and unstaged changes, then create a git commit:
1. Run git status and git diff to understand changes
2. Draft a concise commit message focusing on "why" not "what"
3. Stage relevant files (avoid secrets, .env files)
4. Create the commit`

var reviewSkillContent = `Review the code changes for:
1. Correctness — does it do what it claims?
2. Security — OWASP top 10, injection, XSS, etc.
3. Performance — unnecessary allocations, N+1 queries
4. Maintainability — clear naming, reasonable complexity
5. Tests — are changes covered?

Provide specific, actionable feedback.`

var simplifySkillContent = fmt.Sprintf(`Review changed code for:
1. Reuse — are there existing utilities that could be used?
2. Quality — is the code clear and idiomatic?
3. Efficiency — are there unnecessary operations?
Fix any issues found.`)
