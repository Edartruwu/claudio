package skills

import (
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Skill represents a loaded skill definition.
type Skill struct {
	Name         string      `json:"name"`
	Description  string      `json:"description"`
	Content      string      `json:"content"` // The prompt/instruction content
	Source       string      `json:"source"`  // "bundled", "user", "project", "plugin"
	FilePath     string      `json:"file_path,omitempty"`
	SkillDir     string      `json:"skill_dir,omitempty"` // directory containing the skill file; empty for flat .md files
	Paths        []string    `json:"paths,omitempty"`
	Hooks        []SkillHook `json:"hooks,omitempty"`
	Capabilities []string    `json:"capabilities,omitempty"`
	Agents       []string    `json:"agents,omitempty"`       // allowlist of agent names; empty = all agents
	RequireTeam  bool        `json:"require_team,omitempty"` // if true, hidden when no team template is loaded
}

// SkillHook defines a hook that auto-registers when the skill is invoked.
type SkillHook struct {
	Event   string `yaml:"event"`   // "PreToolUse", "PostToolUse", etc.
	Matcher string `yaml:"matcher"` // tool name glob, e.g. "Write|Edit" or "*"
	Command string `yaml:"command"` // shell command to run
	Timeout int    `yaml:"timeout"` // ms, 0 = use default
	Async   bool   `yaml:"async"`   // non-blocking
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

// All returns all loaded skills sorted by name for deterministic ordering.
// Consistent ordering is required so the Skill tool description stays stable
// across turns and doesn't bust the Anthropic prompt cache.
func (r *Registry) All() []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Skill, 0, len(r.skills))
	for _, s := range r.skills {
		result = append(result, s)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// Clone returns a deep copy of the registry.
func (r *Registry) Clone() *Registry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c := NewRegistry()
	for k, v := range r.skills {
		c.skills[k] = v
	}
	return c
}

// Remove deletes a skill by name. No-op if not found.
func (r *Registry) Remove(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.skills, name)
}

// FilterSkills returns a new registry containing only skills visible to the
// given agent. All four conditions must pass for a skill to be included:
//  1. Agent filter:      skill.Agents is empty OR agentName is in skill.Agents
//  2. Capability filter: skill.Capabilities is empty OR intersection with agentCaps is non-empty
//  3. Team filter:       skill.RequireTeam is false OR hasActiveTeam is true
//  4. Exclusion filter:  skill name is not in excluded list (case-insensitive)
func (r *Registry) FilterSkills(agentName string, agentCaps []string, hasActiveTeam bool, excluded []string) *Registry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := NewRegistry()
	for _, s := range r.skills {
		// 1. Agent filter
		if len(s.Agents) > 0 {
			found := false
			for _, a := range s.Agents {
				if strings.EqualFold(a, agentName) {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		// 2. Capability filter
		if len(s.Capabilities) > 0 {
			found := false
		capLoop:
			for _, ac := range agentCaps {
				for _, sc := range s.Capabilities {
					if ac == sc {
						found = true
						break capLoop
					}
				}
			}
			if !found {
				continue
			}
		}
		// 3. Team filter
		if s.RequireTeam && !hasActiveTeam {
			continue
		}
		// 4. Exclusion filter
		if len(excluded) > 0 {
			skip := false
			for _, ex := range excluded {
				if strings.EqualFold(ex, s.Name) {
					skip = true
					break
				}
			}
			if skip {
				continue
			}
		}
		out.skills[s.Name] = s
	}
	return out
}

// FilterByCapabilities is a compatibility alias for FilterSkills with no agent
// name, team, or exclusion filters applied. Prefer FilterSkills for new call sites.
func (r *Registry) FilterByCapabilities(agentCaps []string) *Registry {
	return r.FilterSkills("", agentCaps, true, nil)
}

// MigrateLegacyCaps converts the deprecated "team" capability to RequireTeam=true.
// Returns the cleaned capability list and whether RequireTeam should be set.
// Logs a deprecation warning if "team" is found.
func MigrateLegacyCaps(skillName string, caps []string) (newCaps []string, requireTeam bool) {
	for _, c := range caps {
		if c == "team" {
			if !requireTeam {
				log.Printf("[skills] DEPRECATED: skill %q uses capabilities=[\"team\"]; use require_team=true instead", skillName)
			}
			requireTeam = true
		} else {
			newCaps = append(newCaps, c)
		}
	}
	return
}

// LoadAll loads skills from all sources: bundled, user, project, and any extra dirs.
// Extra dirs (e.g. from installed harnesses) are loaded after project skills with source="harness".
func LoadAll(userSkillsDir, projectSkillsDir string, extraDirs ...string) *Registry {
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

	// 4. Harness skill dirs (installed harnesses)
	for _, dir := range extraDirs {
		if dir != "" {
			loadFromDir(r, dir, "harness")
		}
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
			for _, fname := range []string{"SKILL.md", "skill.md", "index.md", "README.md"} {
				path := filepath.Join(dir, entry.Name(), fname)
				if content, err := os.ReadFile(path); err == nil {
					name, desc, paths, hooks, caps, agents, requireTeam, body := parseSkillFile(string(content))
					if name == "" {
						name = entry.Name()
					}
					r.Register(&Skill{
						Name:         name,
						Description:  desc,
						Content:      body,
						Source:       source,
						FilePath:     path,
						SkillDir:     filepath.Join(dir, entry.Name()),
						Paths:        paths,
						Hooks:        hooks,
						Capabilities: caps,
						Agents:       agents,
						RequireTeam:  requireTeam,
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
			name, desc, paths, hooks, caps, agents, requireTeam, body := parseSkillFile(string(content))
			if name == "" {
				name = strings.TrimSuffix(entry.Name(), ".md")
			}
			r.Register(&Skill{
				Name:         name,
				Description:  desc,
				Content:      body,
				Source:       source,
				FilePath:     path,
				Paths:        paths,
				Hooks:        hooks,
				Capabilities: caps,
				Agents:       agents,
				RequireTeam:  requireTeam,
			})
		}
	}
}

// parseSkillFile extracts frontmatter (name, description, paths, hooks, capabilities, agents, require_team) and body from a skill file.
func parseSkillFile(content string) (name, description string, paths []string, hooks []SkillHook, capabilities []string, agents []string, requireTeam bool, body string) {
	lines := strings.Split(content, "\n")

	// internal frontmatter struct for yaml unmarshalling
	type frontmatterData struct {
		Name         string      `yaml:"name"`
		Description  string      `yaml:"description"`
		Paths        []string    `yaml:"paths"`
		Hooks        []SkillHook `yaml:"hooks"`
		Capabilities []string    `yaml:"capabilities"`
		Agents       []string    `yaml:"agents"`
		RequireTeam  bool        `yaml:"require_team"`
	}

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
			// Parse frontmatter with yaml.v3
			var fm frontmatterData
			if err := yaml.Unmarshal([]byte(strings.Join(lines[1:endIdx], "\n")), &fm); err == nil {
				name = fm.Name
				description = fm.Description
				paths = fm.Paths
				hooks = fm.Hooks
				agents = fm.Agents
				// Migrate legacy "team" capability from frontmatter
				capabilities, requireTeam = MigrateLegacyCaps(fm.Name, fm.Capabilities)
				if fm.RequireTeam {
					requireTeam = true
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
// LoadBundled registers all bundled skills into an existing registry.
func LoadBundled(r *Registry) {
	for _, s := range bundledSkills() {
		r.Register(s)
	}
}

// BundledSkillContent returns the content of a named bundled skill, or empty string if not found.
func BundledSkillContent(name string) string {
	for _, s := range bundledSkills() {
		if s.Name == name {
			return s.Content
		}
	}
	return ""
}

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
			Description: "Review changed code for reuse, quality, and efficiency, then fix any issues found",
			Content:     simplifySkillContent,
			Source:      "bundled",
		},
		{
			Name:        "update-config",
			Description: "Configure Claudio settings via settings.json — hooks, permissions, env vars, MCP servers",
			Content:     updateConfigSkillContent,
			Source:      "bundled",
		},
		{
			Name:        "debug",
			Description: "Diagnose issues with the current session — check logs, environment, and configuration",
			Content:     debugSkillContent,
			Source:      "bundled",
		},
		{
			Name:        "batch",
			Description: "Orchestrate parallel work across multiple worktrees for large-scale changes",
			Content:     batchSkillContent,
			Source:      "bundled",
		},
		{
			Name:        "pr",
			Description: "Create a pull request with a well-structured description",
			Content:     prSkillContent,
			Source:      "bundled",
		},
		{
			Name:        "test",
			Description: "Run tests and fix failures",
			Content:     testSkillContent,
			Source:      "bundled",
		},
		{
			Name:        "security-review",
			Description: "OWASP Top 10 security review of code changes",
			Content:     securityReviewSkillContent,
			Source:      "bundled",
		},
		{
			Name:        "refactor",
			Description: "Refactor code for clarity, performance, and maintainability",
			Content:     refactorSkillContent,
			Source:      "bundled",
		},
		{
			Name:        "setup-snippets",
			Description: "Analyze the project and configure snippet expansion for common boilerplate patterns",
			Content:     setupSnippetsSkillContent,
			Source:      "bundled",
		},
		{
			Name:        "init",
			Description: "Initialize CLAUDIO.md, skills, and hooks for this project",
			Content:     initSkillContent,
			Source:      "bundled",
		},
		{
			Name:        "harness",
			Description: "Design and build a domain-specific agent team harness for this project. Use when asked to 'build a harness', 'design an agent team', 'set up a harness', or 'create specialist agents'. Generates .claudio/agents/ definitions, .claudio/skills/ orchestrators, .claudio/team-templates/ roster, and .claudio/harness.json manifest. Also handles harness audits, extensions, and maintenance.",
			Content:     harnessSkillContent,
			Source:      "bundled",
		},
		{
			Name:        "caveman",
			Description: "Ultra-compressed communication mode. Cuts token usage ~75% by speaking like caveman while keeping full technical accuracy.",
			Content:     cavemanSkillContent,
			Source:      "bundled",
		},
		{
			Name:        "caveman-review",
			Description: "Ultra-compressed code review comments. Each comment is one line: location, problem, fix.",
			Content:     cavemanReviewSkillContent,
			Source:      "bundled",
		},
		{
			Name:         "design-system",
			Description:  "Extract a design token system from reference assets, URLs, or descriptions",
			Content:      designSystemSkillContent,
			Source:       "bundled",
			Capabilities: []string{"design"},
		},
		{
			Name:         "mockup",
			Description:  "Generate a complete multi-screen HTML mockup from a product brief",
			Content:      mockupSkillContent,
			Source:       "bundled",
			Capabilities: []string{"design"},
		},
		{
			Name:         "handoff",
			Description:  "Generate a developer handoff package from a completed mockup",
			Content:      handoffSkillContent,
			Source:       "bundled",
			Capabilities: []string{"design"},
		},
		{
			Name:         "hifi",
			Description:  "Generate high-fidelity mockups with multiple design variations and a live Tweaks panel for toggling colors, fonts, density, and dark mode",
			Content:      hifiSkillContent,
			Source:       "bundled",
			Capabilities: []string{"design"},
		},
		{
			Name:         "wireframe",
			Description:  "Generate fast lo-fi grayscale wireframes with annotations for early-stage ideation — no design tokens, no polish, just structure",
			Content:      wireframeSkillContent,
			Source:       "bundled",
			Capabilities: []string{"design"},
		},
		{
			Name:         "prototype",
			Description:  "Generate stateful interactive prototypes with multi-screen flows, animated transitions, loading states, modals, and a hotspot mode for stakeholder walkthroughs",
			Content:      prototypeSkillContent,
			Source:       "bundled",
			Capabilities: []string{"design"},
		},
		{
			Name:         "design-direction-advisor",
			Description:  "When brief is vague, generates 3 differentiated design directions from distinct philosophies. Outputs live HTML demos side-by-side. User picks one direction before proceeding to hifi/prototype.",
			Content:      designDirectionAdvisorSkillContent,
			Source:       "bundled",
			Capabilities: []string{"design"},
		},
	}
}

var commitSkillContent = `## Context

- ` + "`git status`" + `: !` + "`git status`" + `
- ` + "`git diff HEAD`" + `: !` + "`git diff HEAD`" + `
- ` + "`git log --oneline -5`" + `: !` + "`git log --oneline -5`" + `

## User Notes
$ARGUMENTS

## Git Safety Protocol

- NEVER update the git config
- NEVER run destructive git commands (reset --hard, push --force, clean -f, branch -D) unless the user explicitly requests it
- NEVER skip hooks (--no-verify, --no-gpg-sign, etc) unless the user explicitly requests it
- NEVER force push to main/master — warn the user if they request it
- Always create NEW commits rather than amending (unless the user asks); if a pre-commit hook fails, fix and create a NEW commit
- Prefer staging specific files over ` + "`git add -A`" + ` or ` + "`git add .`" + `
- NEVER commit unless explicitly asked

## Your task

Based on the context above:
1. Stage relevant untracked files (specific files, not ` + "`git add -A`" + `)
2. Create the commit using a HEREDOC:
` + "```" + `
git commit -m "$(cat <<'EOF'
Commit message here.
EOF
)"
` + "```" + `
3. Run ` + "`git status`" + ` after the commit to verify
4. If a pre-commit hook fails: fix the issue and create a NEW commit

Important:
- DO NOT push unless the user explicitly asks
- Do not create an empty commit if there are no changes
- Do not commit files with secrets (.env, credentials.json, etc)`

var reviewSkillContent = `You are being asked to review code changes. Follow this checklist:

## User Input
$ARGUMENTS

## Review Process

1. **Gather context**: Run ` + "`git diff`" + ` and ` + "`git diff --cached`" + ` to see all changes
2. **Read the changed files** in full to understand the broader context

## Review Checklist

### Correctness
- Does the code do what it claims to do?
- Are there off-by-one errors, null pointer issues, or race conditions?
- Are edge cases handled?

### Security (OWASP Top 10)
- Input validation: SQL injection, command injection, XSS, path traversal
- Authentication/authorization: proper access controls
- Sensitive data: no hardcoded secrets, proper handling of PII
- Dependencies: known vulnerabilities in new dependencies

### Performance
- Unnecessary allocations or copies
- N+1 query patterns
- Missing indices for new queries
- Unbounded growth (memory leaks, unbounded channels/slices)

### Maintainability
- Clear naming conventions
- Reasonable function complexity (single responsibility)
- Adequate error handling at system boundaries
- Tests cover the critical paths

### API Design
- Backwards compatibility (if applicable)
- Consistent with existing patterns in the codebase
- Proper error responses and status codes

## Output Format

For each issue found, provide:
- **Severity**: Critical / Warning / Suggestion
- **File:Line**: The specific location
- **Issue**: What's wrong
- **Fix**: How to fix it

End with a summary: APPROVE, REQUEST CHANGES, or NEEDS DISCUSSION.`

var simplifySkillContent = `You are being asked to review changed code for reuse, quality, and efficiency, then fix any issues found.

## User Input
$ARGUMENTS

## Process

1. Run ` + "`git diff`" + ` to see what changed
2. Read the changed files in full

## Review Dimensions

### Reuse
- Are there existing utilities in the codebase that could replace new code?
- Is there duplicated logic that could be consolidated?
- Could any new helper be replaced by a standard library function?

### Quality
- Is the code clear and idiomatic for the language?
- Are naming conventions consistent with the rest of the codebase?
- Is the abstraction level appropriate (not too abstract, not too concrete)?
- Are there any code smells (long functions, deep nesting, magic numbers)?

### Efficiency
- Are there unnecessary operations (redundant loops, repeated computations)?
- Could data structures be chosen more appropriately?
- Are there obvious performance improvements without sacrificing readability?

## Action

After identifying issues, **fix them directly** using the Edit tool. Don't just report — actually make the improvements.

Report what you changed and why.`

var updateConfigSkillContent = `You are being asked to configure Claudio settings. Help the user modify their settings.json file.

## User Input
$ARGUMENTS

## Configuration Locations

- **User settings**: ~/.claudio/settings.json (applies to all projects)
- **Project settings**: .claudio/settings.json (applies to this project only)
- **Local settings**: ~/.claudio/local-settings.json (machine-specific overrides)

## Available Settings

` + "```json" + `
{
  "model": "claude-sonnet-4-6",       // Default model
  "permissionMode": "default",        // "default", "auto", "headless"
  "autoCompact": false,               // Auto-compact conversation
  "sessionPersist": true,             // Persist sessions to SQLite
  "denyPaths": [],                    // Paths tools cannot access
  "denyTools": [],                    // Tools to disable
  "allowPaths": [],                   // Additional allowed paths
  "mcpServers": {},                   // MCP server configurations
  "apiBaseUrl": "https://api.anthropic.com",
  "maxBudget": 0                      // Session cost limit in USD (0 = unlimited)
}
` + "```" + `

## MCP Server Configuration

` + "```json" + `
{
  "mcpServers": {
    "server-name": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem"],
      "env": {},
      "type": "stdio"
    }
  }
}
` + "```" + `

## Process

1. Read the current settings file(s) to understand what's already configured
2. Ask the user what they want to change if not clear
3. Make the changes using the Edit or Write tool
4. Verify the JSON is valid`

var debugSkillContent = `You are being asked to diagnose issues with the current Claudio session.

## User Input
$ARGUMENTS

## Diagnostic Steps

1. **Check environment**:
   - Run ` + "`which claudio`" + ` to verify installation
   - Run ` + "`claudio version`" + ` to check version
   - Check if required tools are available: git, rg (ripgrep), gopls, node

2. **Check configuration**:
   - Read ~/.claudio/settings.json (user settings)
   - Read .claudio/settings.json (project settings) if it exists
   - Check for ANTHROPIC_API_KEY or auth status

3. **Check logs**:
   - Look for debug logs in ~/.claudio/logs/
   - Check for recent error patterns

4. **Check connectivity**:
   - Verify API endpoint is reachable
   - Check for proxy configuration

## Report

Provide a structured diagnostic report:
- Environment: OK / Issues found
- Configuration: OK / Issues found
- Authentication: OK / Issues found
- Connectivity: OK / Issues found

For each issue, suggest a fix.`

var batchSkillContent = `You are being asked to orchestrate parallel work across multiple worktrees.

## User Input
$ARGUMENTS

## Process

### Phase 1: Plan
1. Enter plan mode to understand the scope of work
2. Decompose the task into independent units of work
3. Each unit should be independently testable and mergeable
4. Get user approval on the plan

### Phase 2: Execute
1. For each unit of work, spawn an Agent with isolation: "worktree"
2. Each agent works in its own worktree with its own branch
3. Each agent should create a commit when done
4. Run up to 5 agents in parallel

### Phase 3: Aggregate
1. Track progress of all agents
2. Report results: which succeeded, which failed
3. List all branches/PRs created

## Important
- Each worktree agent gets a clear, self-contained task description
- Include file paths and specific instructions in each agent prompt
- Never delegate understanding — each prompt must be complete`

var prSkillContent = `## Context

- ` + "`git branch --show-current`" + `: !` + "`git branch --show-current`" + `
- ` + "`git status`" + `: !` + "`git status`" + `
- ` + "`git diff HEAD`" + `: !` + "`git diff HEAD`" + `
- ` + "`git log main...HEAD --oneline`" + `: !` + "`git log main...HEAD --oneline 2>/dev/null || git log master...HEAD --oneline 2>/dev/null || git log -10 --oneline`" + `
- Existing PR: !` + "`gh pr view --json number,url 2>/dev/null || echo '(none)'`" + `

## User Notes
$ARGUMENTS

## Git Safety Protocol

- NEVER update the git config
- NEVER run destructive git commands unless the user explicitly requests it
- NEVER skip hooks (--no-verify, --no-gpg-sign, etc) unless the user explicitly requests it
- NEVER force push to main/master — warn the user if they request it

## Your task

Analyze ALL commits in this branch (not just the latest). Then:
1. Create a new branch if currently on main/master (use ` + "`whoami`" + ` as the branch name prefix)
2. Push the branch to origin with ` + "`-u`" + ` flag
3. If a PR already exists (see Existing PR above): update with ` + "`gh pr edit`" + `
   Otherwise: create with ` + "`gh pr create`" + `:
` + "```" + `
gh pr create --title "Short title under 70 chars" --body "$(cat <<'EOF'
## Summary
<1-3 bullet points>

## Test plan
[Checklist of testing TODOs...]
EOF
)"
` + "```" + `
4. Return the PR URL when done`

var testSkillContent = `You are being asked to run tests and fix any failures.

## User Input
$ARGUMENTS

## Process

1. **Discover test commands**: Check for:
   - Go: ` + "`go test ./...`" + `
   - Node: ` + "`npm test`" + ` or ` + "`npx jest`" + ` or ` + "`npx vitest`" + `
   - Python: ` + "`pytest`" + ` or ` + "`python -m pytest`" + `
   - Rust: ` + "`cargo test`" + `
   - Look at package.json scripts, Makefile targets, or CI config for the canonical test command

2. **Run the full test suite** using the appropriate command

3. **Analyze failures**:
   - Read the test file to understand what's being tested
   - Read the implementation code that the test exercises
   - Identify the root cause (not just the symptom)

4. **Fix failures**:
   - Fix the implementation, not the test (unless the test is wrong)
   - Run the specific failing test to verify the fix
   - Run the full suite again to check for regressions

5. **Report results**: Which tests passed/failed, what you fixed, any remaining issues`

var securityReviewSkillContent = `You are being asked to perform an OWASP Top 10 security review.

## User Input
$ARGUMENTS

## Process

1. Run ` + "`git diff`" + ` and ` + "`git diff --cached`" + ` to identify changes
2. Read all changed files completely

## OWASP Top 10 Checklist

### A01: Broken Access Control
- Are there authorization checks on all endpoints/routes?
- Is there path traversal risk in file operations?
- Are CORS policies properly configured?

### A02: Cryptographic Failures
- Are secrets hardcoded or stored in plaintext?
- Is sensitive data encrypted at rest and in transit?
- Are deprecated crypto algorithms used (MD5, SHA1 for security)?

### A03: Injection
- SQL injection: Are queries parameterized?
- Command injection: Is user input passed to shell commands?
- XSS: Is user input properly escaped in HTML output?
- Template injection: Is user input used in template rendering?

### A04: Insecure Design
- Are rate limits in place for sensitive operations?
- Are there proper input validation boundaries?

### A05: Security Misconfiguration
- Are debug modes disabled in production?
- Are default credentials removed?
- Are unnecessary features disabled?

### A06: Vulnerable Components
- Are there known CVEs in dependencies?
- Are dependency versions pinned?

### A07: Authentication Failures
- Are passwords properly hashed (bcrypt, argon2)?
- Is MFA supported for sensitive operations?
- Are session tokens properly managed?

### A08: Data Integrity Failures
- Are CI/CD pipelines secure?
- Is code signing in place?
- Are software updates verified?

### A09: Logging & Monitoring
- Are security events logged?
- Are logs sanitized (no secrets in logs)?
- Is there alerting for suspicious patterns?

### A10: Server-Side Request Forgery (SSRF)
- Can user input control outbound requests?
- Are internal endpoints protected from SSRF?

## Output

For each finding:
- **Severity**: Critical / High / Medium / Low / Info
- **Category**: OWASP A01-A10
- **Location**: file:line
- **Issue**: Description
- **Remediation**: How to fix

End with: SECURE / NEEDS FIXES / CRITICAL ISSUES`

var setupSnippetsSkillContent = `You are being asked to analyze this project and configure snippet expansion.

## User Input
$ARGUMENTS

Snippet expansion lets the AI write shorthand like ` + "`~errw(db.Query(ctx, id), \"fetch user\")`" + ` instead of full boilerplate. A deterministic expander replaces it before writing to disk.

## Process

1. **Detect project languages**: Run ` + "`find . -maxdepth 3 -type f \\( -name '*.go' -o -name '*.py' -o -name '*.ts' -o -name '*.tsx' -o -name '*.js' -o -name '*.jsx' -o -name '*.rs' \\) | head -50`" + ` to identify what languages are used.

2. **Read existing config**: Check if ` + "`.claudio/settings.json`" + ` exists. If it does, read it — we'll merge snippets into the existing config.

3. **Analyze common patterns**: For each language found, read 3-5 representative source files to identify repetitive boilerplate patterns. Pay special attention to:
   - **Error handling libraries**: Look for custom error packages (e.g., errx, pkg/errors, eris). Check for error registries, error types, and wrapping patterns. The snippets should match the project's actual error conventions, NOT default to fmt.Errorf.
   - **Go**: if err != nil patterns, error wrapping style (fmt.Errorf, errx.Wrap, errors.Wrap, custom), test functions, HTTP handlers, struct builders
   - **Python**: try/except blocks, FastAPI/Flask endpoints, test functions, dataclass/Pydantic models
   - **TypeScript/JavaScript**: try/catch, React components, test cases (Jest/Vitest), API handlers
   - **Rust**: Result/match error handling, anyhow/thiserror patterns, test modules, impl blocks

4. **Build snippet definitions**: Create snippets for the top 5-8 most repetitive patterns found. Each snippet must have:
   - ` + "`name`" + `: short, memorable (e.g., "errw", "test", "handler", "component")
   - ` + "`params`" + `: the parts that vary between uses
   - ` + "`template`" + `: Go text/template string with ` + "`{{.paramName}}`" + ` placeholders
   - ` + "`lang`" + `: file extension filter (e.g., "go", "py", "ts")

   Available context variables (resolved automatically from enclosing function):
   - ` + "`{{.ReturnZeros}}`" + ` — correct zero values for the function's return types (Go)
   - ` + "`{{.FuncName}}`" + ` — enclosing function name (all languages)
   - ` + "`{{.ReturnType}}`" + ` — return type annotation (Python, TS, Rust)
   - ` + "`{{.result}}`" + ` — default variable name for results (defaults to "result")

## Reference: snippet examples by language

### Go — standard error handling
` + "```json" + `
{"name": "errw", "params": ["call", "msg"], "lang": "go",
 "template": "{{.result}}, err := {{.call}}\nif err != nil {\n\treturn {{.ReturnZeros}}, fmt.Errorf(\"{{.msg}}: %w\", err)\n}"}
` + "```" + `
Usage: ` + "`~errw(db.QueryRow(ctx, id), \"fetch user\")`" + `

### Go — custom error library (errx-style with types and registries)
If the project uses a custom error package with typed errors and registries, generate these variants instead of the standard fmt.Errorf snippet:
` + "```json" + `
{"name": "errw", "params": ["call", "msg"], "lang": "go",
 "template": "{{.result}}, err := {{.call}}\nif err != nil {\n\treturn {{.ReturnZeros}}, errx.Wrap(err, \"{{.msg}}\", errx.TypeInternal)\n}"}
` + "```" + `
Usage: ` + "`~errw(s.repo.GetByID(ctx, id), \"fetch entity\")`" + ` — wraps with internal type (most common)

` + "```json" + `
{"name": "errwt", "params": ["call", "msg", "type"], "lang": "go",
 "template": "{{.result}}, err := {{.call}}\nif err != nil {\n\treturn {{.ReturnZeros}}, errx.Wrap(err, \"{{.msg}}\", errx.Type{{.type}})\n}"}
` + "```" + `
Usage: ` + "`~errwt(s.repo.FindByID(ctx, id), \"find tenant\", NotFound)`" + ` — wraps with explicit type

` + "```json" + `
{"name": "errn", "params": ["call"], "lang": "go",
 "template": "{{.result}}, err := {{.call}}\nif err != nil {\n\treturn {{.ReturnZeros}}, err\n}"}
` + "```" + `
Usage: ` + "`~errn(s.repo.Update(ctx, entity))`" + ` — propagates error as-is

` + "```json" + `
{"name": "errd", "params": ["errfn"], "lang": "go",
 "template": "return {{.ReturnZeros}}, {{.errfn}}"}
` + "```" + `
Usage: ` + "`~errd(ErrJobNotFound())`" + ` — returns a domain error from a registry

` + "```json" + `
{"name": "errdc", "params": ["code", "cause"], "lang": "go",
 "template": "return {{.ReturnZeros}}, ErrRegistry.NewWithCause({{.code}}, {{.cause}})"}
` + "```" + `
Usage: ` + "`~errdc(CodeJobNotFound, err)`" + ` — registry error with underlying cause

` + "```json" + `
{"name": "errdd", "params": ["errfn", "key", "val"], "lang": "go",
 "template": "return {{.ReturnZeros}}, {{.errfn}}.WithDetail(\"{{.key}}\", {{.val}})"}
` + "```" + `
Usage: ` + "`~errdd(ErrNotFound(), \"job_id\", jobID)`" + ` — domain error with detail metadata

### Go — test scaffolding
` + "```json" + `
{"name": "test", "params": ["name"], "lang": "go",
 "template": "func Test{{.name}}(t *testing.T) {\n\tt.Run(\"{{.name}}\", func(t *testing.T) {\n\t\t// TODO\n\t})\n}"}
` + "```" + `
Usage: ` + "`~test(CreateUser)`" + `

### Go — HTTP handler (Fiber/Chi/stdlib)
` + "```json" + `
{"name": "handler", "params": ["name", "method"], "lang": "go",
 "template": "func (h *Handlers) {{.name}}(c *fiber.Ctx) error {\n\tctx := c.Context()\n\t// TODO\n\treturn c.JSON(fiber.Map{\"ok\": true})\n}"}
` + "```" + `
Usage: ` + "`~handler(CreateJob, POST)`" + `

### Python — try/except
` + "```json" + `
{"name": "tryw", "params": ["call", "msg"], "lang": "py",
 "template": "try:\n    result = {{.call}}\nexcept Exception as e:\n    raise RuntimeError(\"{{.msg}}\") from e"}
` + "```" + `
Usage: ` + "`~tryw(db.fetch_user(user_id), \"fetch user failed\")`" + `

### Python — FastAPI endpoint
` + "```json" + `
{"name": "endpoint", "params": ["method", "path", "name"], "lang": "py",
 "template": "@router.{{.method}}(\"{{.path}}\")\nasync def {{.name}}(request: Request):\n    pass"}
` + "```" + `
Usage: ` + "`~endpoint(post, /api/users, create_user)`" + `

### Python — pytest
` + "```json" + `
{"name": "test", "params": ["name"], "lang": "py",
 "template": "def test_{{.name}}():\n    # Arrange\n\n    # Act\n\n    # Assert\n    assert True"}
` + "```" + `
Usage: ` + "`~test(create_user_validates_email)`" + `

### Python — Pydantic model
` + "```json" + `
{"name": "model", "params": ["name"], "lang": "py",
 "template": "class {{.name}}(BaseModel):\n    class Config:\n        from_attributes = True"}
` + "```" + `
Usage: ` + "`~model(UserResponse)`" + `

### TypeScript — try/catch
` + "```json" + `
{"name": "tryw", "params": ["call", "msg"], "lang": "ts",
 "template": "try {\n  const result = {{.call}};\n} catch (error) {\n  throw new Error(\"{{.msg}}\", { cause: error });\n}"}
` + "```" + `
Usage: ` + "`~tryw(await fetchUser(id), \"failed to fetch user\")`" + `

### TypeScript — React component
` + "```json" + `
{"name": "component", "params": ["name"], "lang": "tsx",
 "template": "interface {{.name}}Props {}\n\nexport function {{.name}}({}: {{.name}}Props) {\n  return <div />;\n}"}
` + "```" + `
Usage: ` + "`~component(UserProfile)`" + `

### TypeScript — API handler
` + "```json" + `
{"name": "api", "params": ["name"], "lang": "ts",
 "template": "export async function {{.name}}(req: Request): Promise<Response> {\n  try {\n    // TODO\n    return Response.json({ ok: true });\n  } catch (error) {\n    return Response.json({ error: \"Internal error\" }, { status: 500 });\n  }\n}"}
` + "```" + `
Usage: ` + "`~api(createUser)`" + `

### TypeScript — Jest/Vitest test
` + "```json" + `
{"name": "test", "params": ["desc"], "lang": "ts",
 "template": "describe(\"{{.desc}}\", () => {\n  it(\"should work\", () => {\n    // Arrange\n\n    // Act\n\n    // Assert\n    expect(true).toBe(true);\n  });\n});"}
` + "```" + `
Usage: ` + "`~test(UserService)`" + `

### Rust — error propagation with context (anyhow)
` + "```json" + `
{"name": "errw", "params": ["call", "msg"], "lang": "rs",
 "template": "let {{.result}} = {{.call}}.map_err(|e| anyhow::anyhow!(\"{{.msg}}: {}\", e))?;"}
` + "```" + `
Usage: ` + "`~errw(db.get_user(id).await, \"fetch user\")`" + `

### Rust — custom error variant (thiserror)
` + "```json" + `
{"name": "errd", "params": ["variant", "msg"], "lang": "rs",
 "template": "return Err(Error::{{.variant}}(\"{{.msg}}\".into()));"}
` + "```" + `
Usage: ` + "`~errd(NotFound, \"user not found\")`" + `

### Rust — test function
` + "```json" + `
{"name": "test", "params": ["name"], "lang": "rs",
 "template": "#[test]\nfn test_{{.name}}() {\n    // Arrange\n\n    // Act\n\n    // Assert\n}"}
` + "```" + `
Usage: ` + "`~test(create_user)`" + `

### Rust — async test (tokio)
` + "```json" + `
{"name": "atest", "params": ["name"], "lang": "rs",
 "template": "#[tokio::test]\nasync fn test_{{.name}}() {\n    // Arrange\n\n    // Act\n\n    // Assert\n}"}
` + "```" + `
Usage: ` + "`~atest(fetch_user)`" + `

### Rust — impl block
` + "```json" + `
{"name": "impl", "params": ["type"], "lang": "rs",
 "template": "impl {{.type}} {\n    pub fn new() -> Self {\n        Self {}\n    }\n}"}
` + "```" + `
Usage: ` + "`~impl(UserService)`" + `

5. **Write config**: Create or update ` + "`.claudio/settings.json`" + ` with the snippets config. Use the examples above as reference but ADAPT them to match the project's actual patterns:
   - If the project uses errx (or any custom error lib), use errx-style snippets, NOT fmt.Errorf
   - If the project uses Fiber, use Fiber handler templates; if Chi, use Chi patterns
   - If the project uses anyhow, use anyhow snippets; if thiserror, use thiserror patterns
   - Match the project's actual coding style (tab vs space, brace placement, etc.)

   If the file already exists with other settings, merge the ` + "`snippets`" + ` key — do NOT overwrite other config.

6. **Show the user** what was configured: list each snippet with its name, what it does, and a usage example.

## Rules
- Only create snippets for patterns that appear at least 3 times in the codebase
- Keep snippet names short (max 10 chars) — the AI will type these frequently
- Template must be valid Go text/template syntax
- Do NOT create snippets for unique business logic — only for mechanical, repetitive patterns
- Prefer fewer high-impact snippets over many marginal ones
- If the project already has snippets configured, suggest additions rather than replacing existing ones
- ALWAYS match the project's error handling conventions — inspect imports and error patterns before choosing templates
- The errx-style examples above show how to handle projects with error registries and typed errors — look for patterns like ErrRegistry, ErrorCode, errx.Wrap, errx.New, WithDetail`

var refactorSkillContent = `You are being asked to refactor code.

## User Input
$ARGUMENTS

## Process

1. **Understand the scope**: Read the files to be refactored and their dependencies
2. **Identify issues**:
   - Code duplication (DRY violations)
   - Functions that are too long (>50 lines)
   - Deep nesting (>3 levels)
   - Poor naming (unclear variable/function names)
   - God objects (classes/structs doing too much)
   - Missing abstractions or over-abstractions
   - Unused code (dead code)

3. **Plan changes**: Identify the minimal set of changes that improve quality without changing behavior

4. **Execute**:
   - Make changes incrementally (one logical change at a time)
   - Preserve all external behavior (this is refactoring, not rewriting)
   - Run tests after each change to verify no regressions

5. **Verify**:
   - Run the full test suite
   - Compare behavior before/after

## Rules
- Do NOT change behavior — only structure
- Do NOT add new features during refactoring
- Keep changes reviewable (small, focused diffs)
- If tests don't exist, write them BEFORE refactoring`

var initSkillContent = `Set up a minimal CLAUDIO.md (and optionally skills and hooks) for this repo. CLAUDIO.md is loaded into every Claudio session, so it must be concise — only include what Claudio would get wrong without it.

## User Input
$ARGUMENTS

## Phase 1: Ask what to set up

Use AskUserQuestion to find out what the user wants:

- "Which CLAUDIO.md files should /init set up?"
  Options: "Project CLAUDIO.md" | "Personal CLAUDIO.local.md" | "Both project + personal"
  Description for project: "Team-shared instructions checked into source control — architecture, coding standards, common workflows."
  Description for personal: "Your private preferences for this project (gitignored, not shared) — your role, sandbox URLs, preferred test data, workflow quirks."

- "Also set up skills and hooks?"
  Options: "Skills + hooks" | "Skills only" | "Hooks only" | "Neither, just CLAUDIO.md"
  Description for skills: "On-demand capabilities you or Claudio invoke with ` + "`/skill-name`" + ` — good for repeatable workflows and reference knowledge."
  Description for hooks: "Deterministic shell commands that run on tool events (e.g., format after every edit). Claudio can't skip them."

## Phase 2: Explore the codebase

Launch a subagent to survey the codebase, and ask it to read key files to understand the project: manifest files (package.json, Cargo.toml, pyproject.toml, go.mod, pom.xml, etc.), README, Makefile/build configs, CI config, existing CLAUDIO.md, .claudio/rules/, .cursor/rules or .cursorrules, .github/copilot-instructions.md, .windsurfrules, .clinerules, .mcp.json.

Detect:
- Build, test, and lint commands (especially non-standard ones)
- Languages, frameworks, and package manager
- Project structure (monorepo with workspaces, multi-module, or single project)
- Code style rules that differ from language defaults
- Non-obvious gotchas, required env vars, or workflow quirks
- Existing .claudio/skills/ and .claudio/rules/ directories
- Formatter configuration (prettier, biome, ruff, black, gofmt, rustfmt, or a unified format script like ` + "`npm run format`" + ` / ` + "`make fmt`" + `)
- Git worktree usage: run ` + "`git worktree list`" + ` to check if this repo has multiple worktrees (only relevant if the user wants a personal CLAUDIO.local.md)

Note what you could NOT figure out from code alone — these become interview questions.

## Phase 3: Fill in the gaps

Use AskUserQuestion to gather what you still need to write good CLAUDIO.md files and skills. Ask only things the code can't answer.

If the user chose project CLAUDIO.md or both: ask about codebase practices — non-obvious commands, gotchas, branch/PR conventions, required env setup, testing quirks. Skip things already in README or obvious from manifest files. Do not mark any options as "recommended" — this is about how their team works, not best practices.

If the user chose personal CLAUDIO.local.md or both: ask about them, not the codebase. Do not mark any options as "recommended" — this is about their personal preferences, not best practices. Examples:
  - What's their role on the team? (e.g., "backend engineer", "data scientist", "new hire onboarding")
  - How familiar are they with this codebase and its languages/frameworks? (so Claudio can calibrate explanation depth)
  - Do they have personal sandbox URLs, test accounts, API key paths, or local setup details Claudio should know?
  - Only if Phase 2 found multiple git worktrees: ask whether their worktrees are nested inside the main repo (e.g., ` + "`.claudio/worktrees/<name>/`" + `) or siblings/external. If nested, the upward file walk finds the main repo's CLAUDIO.local.md automatically. If sibling/external, personal content should live in ` + "`~/.claudio/<project-name>-instructions.md`" + ` and each worktree gets a one-line CLAUDIO.local.md stub that imports it: ` + "`@~/.claudio/<project-name>-instructions.md`" + `. Never put this import in the project CLAUDIO.md.
  - Any communication preferences? (e.g., "be terse", "always explain tradeoffs", "don't summarize at the end")

**Synthesize a proposal from Phase 2 findings** — e.g., format-on-edit if a formatter exists, a ` + "`/verify`" + ` skill if tests exist, a CLAUDIO.md note for anything from the gap-fill answers that's a guideline rather than a workflow. For each, pick the artifact type that fits, **constrained by the Phase 1 skills+hooks choice**:

  - **Hook** (stricter) — deterministic shell command on a tool event; Claudio can't skip it. Fits mechanical, fast, per-edit steps: formatting, linting, running a quick test on the changed file.
  - **Skill** (on-demand) — you or Claudio invoke ` + "`/skill-name`" + ` when you want it. Fits workflows that don't belong on every edit: deep verification, session reports, deploys.
  - **CLAUDIO.md note** (looser) — influences Claudio's behavior but not enforced. Fits communication/thinking preferences.

  **Respect Phase 1's skills+hooks choice as a hard filter**: if the user picked "Skills only", downgrade any hook you'd suggest to a skill or a CLAUDIO.md note. If "Hooks only", downgrade skills to hooks where mechanically possible or to notes. If "Neither", everything becomes a CLAUDIO.md note. Never propose an artifact type the user didn't opt into.

**Show the proposal via AskUserQuestion, not as a separate text message**. Structure it as:
  - ` + "`question`" + `: short and plain, e.g. "Does this proposal look right?"
  - Keep previews compact. One line per item. Example preview content:

    • **Format-on-edit hook** (automatic) — ` + "`gofmt -w <file>`" + ` via PostToolUse
    • **/verify skill** (on-demand) — ` + "`make lint && go test ./...`" + `
    • **CLAUDIO.md note** (guideline) — "run lint/test before marking done"

  - Option labels stay short ("Looks good", "Drop the hook", "Drop the skill").

**Build the preference queue** from the accepted proposal. Each entry: {type: hook|skill|note, description, target file, any Phase-2-sourced details like the actual test/format command}. Phases 4-7 consume this queue.

## Phase 4: Write CLAUDIO.md (if user chose project or both)

Write a **lean index** CLAUDIO.md at the project root — 20-30 lines max. CLAUDIO.md is a quick-reference index, not a documentation dump. Detailed guidance lives in ` + "`.claudio/rules/`" + ` files (written in Phase 4b).

**Decision rule — what belongs in CLAUDIO.md directly vs. a rules file:**
- **In CLAUDIO.md directly**: hard constraints ("no CGO"), daily-use non-standard commands, critical env var notes, commit/PR conventions. Things agents must never forget.
- **In a rules file (` + "`@`" + `-referenced)**: code style patterns, architecture map, testing conventions, anything detailed or explanatory.

**Consume ` + "`note`" + ` entries from the Phase 3 preference queue whose target is CLAUDIO.md** — add each as a concise line in the most relevant section.

**Structure for CLAUDIO.md:**

` + "```" + `
# CLAUDIO.md

This file provides guidance to Claudio (github.com/Abraxas-365/claudio) when working with code in this repository.

[One-line project description — only if not obvious from repo name]

## Build & Test
[Only non-standard commands, flags, or sequences. Skip commands obvious from manifest files.]

## Key Constraints
[Hard rules agents must never violate — e.g., "No CGO", "Never edit existing migrations"]

## Conventions
[Commit style, branch naming, PR rules — one line each]

## Rules
@.claudio/rules/code-style.md
@.claudio/rules/testing.md
@.claudio/rules/architecture.md
[Only @-reference files you actually create in Phase 4b]
` + "```" + `

Every line must pass this test: "Would removing this cause Claudio to make mistakes on daily tasks?" If no, cut it or move it to a rules file.

Exclude from CLAUDIO.md:
- File-by-file structure or component lists (Claudio discovers these; put summary in architecture.md)
- Standard language conventions Claudio already knows
- Generic advice ("write clean code", "handle errors")
- Detailed explanations — those go in rules files

Prefix the file with the standard header shown above.

If CLAUDIO.md already exists: read it carefully, then ALWAYS regenerate a complete improved version (leaner index + @references). Show the user what changed and why. Never skip or say "looks fine" — the user ran /init specifically because they want an update.

For monorepos or multi-module projects: mention that subdirectory CLAUDIO.md files can be added for module-specific instructions. Offer to create them if the user wants.

## Phase 4b: Generate .claudio/rules/ files

Create focused rules files under ` + "`.claudio/rules/`" + ` based on the Phase 2 codebase survey. These files hold the detailed guidance that would bloat CLAUDIO.md.

**Create only files for which you found real content in Phase 2.** Do not create empty or generic files.

### Candidate files (create only if relevant content exists):

**` + "`.claudio/rules/code-style.md`" + `** — create if you found style conventions beyond language defaults:
- Naming conventions (variables, types, files, packages)
- Formatting rules that differ from defaults
- Import ordering or grouping rules
- Patterns to follow (e.g., "use functional options pattern for config structs")
- Patterns to avoid (e.g., "never use init() for side effects")

**` + "`.claudio/rules/testing.md`" + `** — create if you found testing conventions or a non-trivial test setup:
- Test naming convention (e.g., ` + "`TestFeature_Case`" + `)
- Test file location rules (co-located vs. separate ` + "`testdata/`" + ` dir)
- Mocking patterns and which libraries are used
- What to test (unit vs. integration boundaries)
- How to run a single test or a subset

**` + "`.claudio/rules/architecture.md`" + `** — create if the project has non-obvious structure:
- Package/module map (one line per package: name → role)
- Key architectural patterns (event bus, service layer, repository pattern, etc.)
- Dependency rules (what can import what)
- Key interfaces or entry points agents should know

**Additional rules files** — create if you found substantive content:
- ` + "`.claudio/rules/security.md`" + ` — auth patterns, secrets handling, never-do rules
- ` + "`.claudio/rules/api.md`" + ` — API conventions, error format, versioning
- ` + "`.claudio/rules/database.md`" + ` — migration rules, ORM patterns, query conventions

**Rules for each file:**
- Concise and specific — not exhaustive documentation
- Use bullet points; one rule per bullet
- No generic advice that applies to all codebases
- Do not duplicate what's already in CLAUDIO.md
- Add an ` + "`@`" + ` reference in CLAUDIO.md for every file you create (under the ` + "`## Rules`" + ` section)

## Phase 4c: Seed Memory with architectural facts

After generating rules files, seed Memory with durable architectural knowledge that agents would otherwise need to investigate from scratch.

Call ` + "`Memory(action=\"save\")`" + ` for each entry below. **Only save facts that survive a ` + "`git clone`" + `** — durable, not session-specific.

### Required entries (if content exists):

**Entry 1: architecture** — package map and key entry points
` + "```" + `
Memory(action="save", name="architecture", description="Package map and key architectural facts for this repo", facts=[
  "<package-name>: <one-sentence role>",
  "<package-name>: <one-sentence role>",
  ... (one fact per major package/module, max 8)
], tags=["codebase-map", "architecture"])
` + "```" + `

**Entry 2+: decision-* entries** — one per hard constraint or architectural decision discovered in Phase 2:
` + "```" + `
Memory(action="save", name="decision-<slug>", description="<decision title>", facts=[
  "<specific, actionable statement about the constraint or decision>"
], tags=["constraint", "<relevant-tag>"])
` + "```" + `

Examples of decision entries worth saving:
- ` + "`decision-no-cgo`" + `: "Project must remain pure Go — no CGO. SQLite via modernc.org/sqlite."
- ` + "`decision-db-migrations`" + `: "Never alter existing migration SQL — append-only. Existing migrations are immutable."
- ` + "`decision-auth`" + `: "Auth uses JWT with RS256. Signing key loaded from AUTH_PRIVATE_KEY env var."
- ` + "`decision-monorepo`" + `: "Monorepo with independent Go modules per service. Each has its own go.mod."

**Rules for Memory seeding:**
- Max 5-8 facts per entry
- Each fact = one sentence, specific and actionable (not "the project uses Go")
- Skip facts already obvious from CLAUDIO.md — Memory is for deeper knowledge agents need without reading files
- Skip generic facts any Go/JS/Python project would have
- If no architectural decisions worth saving, skip the decision-* entries entirely
- After saving, confirm which entries were created (you'll list them in Phase 8)

## Phase 5: Write CLAUDIO.local.md (if user chose personal or both)

Write a minimal CLAUDIO.local.md at the project root. After creating it, add ` + "`CLAUDIO.local.md`" + ` to .gitignore.

**Consume ` + "`note`" + ` entries from the Phase 3 preference queue whose target is CLAUDIO.local.md**.

Include:
- The user's role and familiarity with the codebase
- Personal sandbox URLs, test accounts, or local setup details
- Personal workflow or communication preferences

If CLAUDIO.local.md already exists: read it carefully, then ALWAYS regenerate a complete improved version. Show the user what changed and why. Never skip — the user ran /init because they want an update.

If Phase 2 found multiple git worktrees and the user confirmed they use sibling/external worktrees: write personal content to ` + "`~/.claudio/<project-name>-instructions.md`" + ` and make CLAUDIO.local.md a one-line stub: ` + "`@~/.claudio/<project-name>-instructions.md`" + `.

## Phase 6: Suggest and create skills (if user chose "Skills + hooks" or "Skills only")

Skills add capabilities Claudio can use on demand. They live in ` + "`.claudio/skills/<name>/skill.md`" + ` with YAML frontmatter:

` + "```yaml" + `
---
name: <skill-name>
description: <what the skill does and when to use it>
---

<Instructions for Claudio>
` + "```" + `

**First, consume ` + "`skill`" + ` entries from the Phase 3 preference queue.** For each queued skill preference:
- Name it from the preference (e.g., "verify", "deploy", "report")
- Write the body using the user's own words from the interview plus Phase 2 findings
- Ask a quick follow-up if underspecified

**Then suggest additional skills** when you find:
- Reference knowledge for specific tasks (conventions, patterns for a subsystem)
- Repeatable workflows the user would want to trigger directly (deploy, release, verify changes)

If ` + "`.claudio/skills/`" + ` already exists with skills, review them first. Do not overwrite existing skills.

Both the user (` + "`/skill-name`" + `) and Claudio can invoke skills by default.

## Phase 7: Suggest additional optimizations

Tell the user you're going to suggest a few additional optimizations.

Check the environment and ask about each gap (use AskUserQuestion):

- **GitHub CLI**: Run ` + "`which gh`" + `. If missing AND the project uses GitHub (check ` + "`git remote -v`" + ` for github.com), ask if they want to install it. Explain it lets Claudio help with commits, PRs, issues, and code review.

- **Linting**: If Phase 2 found no lint config for the project's language, ask if they want Claudio to set up linting. Explain linting catches issues early and gives Claudio fast feedback.

- **Proposal-sourced hooks** (if user chose "Skills + hooks" or "Hooks only"): Consume ` + "`hook`" + ` entries from the Phase 3 preference queue. Hooks are configured in ` + "`.claudio/settings.json`" + ` (project-shared) or ` + "`~/.claudio/settings.json`" + ` (user-level). If Phase 2 found a formatter and the queue has no formatting hook, offer format-on-edit as a fallback.

  Hook events:
  - "after every edit" → ` + "`PostToolUse`" + ` with matcher ` + "`Write|Edit`" + `
  - "when Claudio finishes" → ` + "`Stop`" + ` event
  - "before running bash" → ` + "`PreToolUse`" + ` with matcher ` + "`Bash`" + `
  - "before committing" → NOT a settings hook (can't filter Bash by content). Route to a git pre-commit hook (` + "`.git/hooks/pre-commit`" + `, husky, etc.) — offer to write one.

  Hook config format in ` + "`.claudio/settings.json`" + `:
  ` + "```json" + `
  {
    "hooks": {
      "PostToolUse": [
        {
          "matcher": "Write|Edit",
          "hooks": [{ "type": "command", "command": "gofmt -w \"$CLAUDIO_TOOL_INPUT_PATH\"" }]
        }
      ]
    }
  }
  ` + "```" + `

Act on each "yes" before moving on.

## Phase 8: Summary and next steps

Recap what was set up. Use this structure:

**Files written:**
- ` + "`CLAUDIO.md`" + ` — lean index (list the sections)
- ` + "`.claudio/rules/<name>.md`" + ` — for each rules file created (list them with one-line description of what's in each)
- Any skills, hooks, or CLAUDIO.local.md created in Phases 5-7

**Memory seeded:**
- List each Memory entry by name and what it captures (e.g., ` + "`architecture`" + ` — package map with N packages, ` + "`decision-no-cgo`" + ` — CGO constraint)
- If no Memory entries were created, say so and briefly explain why (e.g., "no durable architectural decisions found beyond what's in CLAUDIO.md")

**How to extend:**
- Add rules: create ` + "`.claudio/rules/<topic>.md`" + ` and add ` + "`@.claudio/rules/<topic>.md`" + ` to the ` + "`## Rules`" + ` section of CLAUDIO.md
- Update Memory: use ` + "`Memory(action=\"append\")`" + ` to add facts to existing entries, or ` + "`Memory(action=\"save\")`" + ` for new entries
- Re-run ` + "`/init`" + ` anytime to re-scan the codebase and refresh all files

Remind the user these files are a starting point — review and tweak as the project evolves.

Then present a well-formatted to-do list of additional suggestions relevant to this repo. Most impactful first:
- If frontend code was detected (React, Vue, Svelte, etc.): mention that browser/screenshot testing tools help Claudio verify UI changes visually.
- If you found gaps in Phase 7 (missing GitHub CLI, missing linting) and the user said no: list them with a one-line reason.
- If tests are missing or sparse: suggest setting up a test framework so Claudio can verify its own changes.
- Always suggest: browse and customize ` + "`.claudio/skills/`" + ` to add project-specific workflows.`

var harnessSkillContent = `You are building a domain-specific agent team harness for this project.

## User Input
$ARGUMENTS

A harness is a reusable multi-agent architecture that decomposes complex, recurring tasks into coordinated specialist agents. It produces:
- ` + "`.claudio/agents/<name>.md`" + ` — one file per specialist role
- ` + "`.claudio/skills/<harness-name>/skill.md`" + ` — an orchestrator skill that assembles and runs the team
- ` + "`.claudio/team-templates/<harness-name>.lua`" + ` — team roster for InstantiateTeam
- ` + "`.claudio/harness.json`" + ` — manifest for ` + "`claudio harness install`" + `
- An entry in CLAUDIO.md documenting when and how to invoke it

---

## Phase 0: Audit existing harnesses AND existing agents

Before designing anything, check for prior work:

1. Read CLAUDIO.md — look for an "## Agent Harnesses" section
2. List ` + "`.claudio/agents/`" + ` and ` + "`.claudio/skills/`" + ` directories (project-local)
3. For each existing agent file, read its frontmatter ` + "`description`" + ` and note:
   - Agent name
   - Domain / specialization
   - Whether it has accumulated memory (presence of a memory dir alongside its definition)
4. If a harness already exists for this domain or a closely related one, decide:
   - **Extend** — add agents/phases to the existing harness
   - **Repair** — fix broken references, stale agent files, placeholder text
   - **Replace** — start fresh (only if the old harness is fundamentally wrong)
   - **Create** — no prior harness exists

Tell the user what you found — both prior harnesses AND the inventory of existing agents that could be reused. This inventory directly feeds Phase 4's reuse-vs-create decisions.

If extending or repairing, skip to the relevant phase.

---

## Phase 1: Understand the request

Use AskUserQuestion to clarify what the user wants:

- What recurring task or domain should the harness cover? (e.g., "full-stack feature implementation", "security audit pipeline", "research and report generation", "code migration at scale")
- What is the desired output? (e.g., "working code + tests + docs", "structured report", "migrated files + PR")
- Who are the likely consumers? (e.g., the team daily, on-demand by the user, CI-triggered)
- Should the harness write files, or just produce analysis/reports?

Skip questions where the task description already answers them clearly.

---

## Phase 2: Explore the project

Launch an Explore subagent to survey the codebase. It should identify:
- Languages, frameworks, and build tools
- Project structure (monorepo, multi-module, single package)
- Existing ` + "`.claudio/agents/`" + ` definitions (avoid duplicating roles that exist)
- Existing ` + "`.claudio/skills/`" + ` (avoid overwriting)
- Testing conventions and CI setup
- Coding patterns and style conventions
- Key subsystems or domains (e.g., auth, payments, data layer)

Return a structured summary: languages, key patterns, existing agents, existing skills.

---

## Phase 3: Choose execution mode and architecture pattern

### Step 1: Agent Teams vs Sub-agents

Agent teams and sub-agents are fundamentally different execution modes. Choose deliberately:

| Criterion | Agent Teams | Sub-agents |
|-----------|-------------|------------|
| Communication | Members can message each other via SendMessage | No inter-agent communication — results return to caller |
| Lifecycle | Run persistently, can be sent follow-up tasks | Spawn, execute, return result, die |
| Coordination | Shared task list via TaskCreate/TaskUpdate | Caller manages all coordination |
| Overhead | Higher — team setup, message routing, shutdown | Lower — just a function call with a prompt |
| Isolation | Each member gets its own worktree (if configured) | Shares caller's worktree |
| Best for | Collaborative work, review loops, cross-agent debate | Independent parallel tasks, quick delegation |

**Decision rule**: Use agent teams when agents need to communicate with each other during execution. Use sub-agents when each agent's task is fully independent and results only need to be combined at the end.

### Step 2: Select architecture pattern

Choose ONE primary pattern (or a justified composite):

**1. Pipeline** — sequential stages, each feeding the next.
` + "```" + `
[Analyze] → [Design] → [Implement] → [Verify]
` + "```" + `
Best for: tasks with strict ordering dependencies.
Execution: sub-agents (each stage completes before the next starts).

**2. Fan-out / Fan-in** — parallel specialists, then integration.
` + "```" + `
              ┌→ [Specialist A] ─┐
[Dispatcher] →├→ [Specialist B] ─┼→ [Integrator]
              └→ [Specialist C] ─┘
` + "```" + `
Best for: multi-perspective analysis, parallel implementation of independent modules.
Execution: agent teams if specialists benefit from cross-talk; sub-agents if fully independent.

**3. Expert Pool** — a router selects the right expert per input.
` + "```" + `
[Router] → { Security Expert | Performance Expert | Architecture Expert }
` + "```" + `
Best for: variable input types each needing different handling.
Execution: sub-agents (only invoke the relevant expert).

**4. Producer-Reviewer** — create, validate, rework loop.
` + "```" + `
[Producer] → [Reviewer] → (issues?) → [Producer] retry (max 3 rounds)
` + "```" + `
Best for: output quality with objective acceptance criteria.
Execution: agent teams (real-time feedback loop between producer and reviewer).
Always set a retry cap (2–3 rounds maximum).

**5. Supervisor** — central coordinator with dynamic task assignment.
` + "```" + `
              ┌→ [Worker A]
[Supervisor] ─┼→ [Worker B]  ← adjusts assignments based on progress
              └→ [Worker C]
` + "```" + `
Best for: variable workload, runtime task partitioning.
Execution: agent teams (shared task list enables dynamic assignment).

**6. Hierarchical Delegation** — leads decompose and delegate to sub-specialists.
` + "```" + `
[Director] → [Lead A] → { Worker A1, Worker A2 }
           → [Lead B] → { Worker B1 }
` + "```" + `
Best for: natural hierarchical decomposition (frontend/backend, by-module).
Execution: team for level-1, sub-agents for level-2. Keep depth ≤ 2 levels.

### Composite patterns

| Composite | Structure | Example |
|-----------|-----------|---------|
| Fan-out + Producer-Reviewer | Each specialist has its own reviewer | Multi-language translation with per-language QA |
| Pipeline + Fan-out | Sequential phases with a parallel stage | Analysis → parallel coding → integration test |
| Supervisor + Expert Pool | Supervisor routes dynamically to experts | Ticket triage — supervisor assigns to domain experts |
| Fan-out + QA cross-validation | Parallel work then cross-agent review | Feature implementation → agents review each other's work |

Present your pattern choice, execution mode, and rationale to the user using AskUserQuestion before proceeding. Include a simple ASCII diagram of the proposed team structure.

---

## Phase 4: Design the agent roster

For each agent in the chosen pattern:

1. **Define the role** — one clear specialization per agent. If two roles can be combined without loss of focus, combine them.
2. **Reuse-vs-create check (REQUIRED)** — before assigning a type, scan the project-local inventory you built in Phase 0:
   - **REUSE** an existing project agent if its description clearly matches this role's domain and responsibilities. Reusing brings the agent's accumulated memory into the team — memory IS preserved when teammates spawn.
   - **CREATE NEW** if no existing agent matches, OR if the closest match is too generic for a specialist role, OR if reusing would dilute an existing agent's persona.
   - **Decision rule**: prefer reuse when in doubt. Memory is valuable; duplicating personas fragments learnings.
   - Tell the user explicitly: "Role X → reusing existing agent ` + "`<name>`" + `" or "Role X → creating new agent ` + "`<name>`" + ` (no suitable existing agent)".
3. **Assign a type** (only for newly-created agents):
   - ` + "`general-purpose`" + ` — needs web access, broad tools, or can't be constrained
   - ` + "`Explore`" + ` — read-only analysis, codebase investigation, no writes
   - ` + "`Plan`" + ` — architecture and planning, no writes
   - Custom (defined in ` + "`.claudio/agents/<name>.md`" + `) — complex persona with specific tooling, reusable across sessions
4. **Decide communication protocol** — what messages this agent sends/receives via SendMessage
5. **Decide task protocol** — what task types this agent creates or claims via TaskCreate/TaskUpdate

### Agent separation criteria

| Factor | Split | Merge |
|--------|-------|-------|
| Expertise domain | Different domains → split | Overlapping → merge |
| Parallelism | Can run independently → split | Sequential dependency → consider merge |
| Context load | Heavy context → split (protect window) | Light → merge |
| Reusability | Useful across harnesses → split into own file | One-off → inline prompt |

### Skill vs Agent distinction

**Agents** define "who" — a persona with expertise, working principles, and communication protocols.
**Skills** define "how" — a reusable workflow or procedure that any agent (or the user) can invoke.
If a behavior is persona-dependent (it matters *who* does it), make it an agent. If it's a procedure anyone could follow, make it a skill.

Present the roster to the user for approval before writing any files.

---

## Phase 5: Write agent definition files

For each agent that warrants a dedicated file (complex persona, reusable, or needs specific tooling), write ` + "`.claudio/agents/<name>.md`" + `:

` + "```markdown" + `
---
name: agent-name
description: "1-2 sentence role summary. Write this as a TRIGGER — include keywords that help Claudio match user requests to this agent. Example: 'Security specialist for code audits, vulnerability scanning, and dependency analysis' not 'An agent that does security things'."
---

# Agent Name — One-line role summary

You are a [domain] specialist responsible for [core function].

## Core responsibilities
1. Responsibility one
2. Responsibility two
3. Responsibility three

## Working principles
- Principle 1 (specific to this role, not generic advice)
- Principle 2
- Always validate your output against [specific criterion]

## Input / output protocol
- **Input**: Receives [what] from [whom] via [mechanism: TaskCreate / SendMessage / file]
- **Output**: Writes [what] to [where] or sends [what] to [whom]
- **Format**: [file format, schema, or message structure]

## Team communication protocol
- **Receives from**: [agent name] — [message type and content]
- **Sends to**: [agent name] — [message type and content]
- **Broadcasts**: Only when [specific condition] (SendMessage({to: "all"}) is expensive — use sparingly)
- **Task claims**: Claims tasks of type [task type] from the shared task list

## QA protocol
- Before marking any task complete, verify: [specific acceptance criteria]
- Cross-check your output against: [related agent's output or project conventions]
- If you find inconsistencies with another agent's work, message them directly before reporting completion

## Error handling
- If [failure condition]: [recovery action]
- Maximum retries: [N] — after that, report failure to [coordinator] and halt
- On timeout: write partial results to _workspace/<name>-partial.md then notify coordinator

## Collaboration
- Works with: [list of peer agents and relationship]
- Depends on: [upstream agents or artifacts]
- Produces for: [downstream agents or final output]
` + "```" + `

### Description writing guide

The description field in the frontmatter is critical — it determines whether Claudio can find and invoke this agent. Write it as a "pushy" trigger:

**Good descriptions** (trigger-rich):
- "Full-stack feature implementer for React frontend and Go backend — handles API design, UI components, database migrations, and integration tests"
- "Security auditor for vulnerability scanning, dependency analysis, OWASP checks, and penetration test planning"

**Bad descriptions** (too vague):
- "An agent that helps with features"
- "Security agent"

Include the key nouns and verbs a user would naturally say when requesting this type of work.

**Important**: Do not write a file for agents whose entire behavior can be expressed in a short inline prompt within the orchestrator skill. Reserve agent files for personas that are reused across harnesses or are complex enough to need their own specification.

---

## Phase 5b: Generate team template

Create ` + "`.claudio/team-templates/<harness-name>.lua`" + ` with the roster from Phase 4:

` + "```lua" + `
return {
  name = "<harness-name>-team",
  description = "<one-line summary of what this team does>",
  members = {
    { name = "<agent-display-name>", subagent_type = "<agent-type>", model = "<model-id>" },
    { name = "<agent-display-name>", subagent_type = "<agent-type>", model = "<model-id>" },
  }
}
` + "```" + `

Model selection guidance:
- **haiku** — simple/boilerplate agents, junior-level tasks, fast turnaround
- **sonnet** — standard work, mid-level reasoning, most agents default here
- **opus** — architecture decisions, complex reasoning, senior-level judgment

This template enables ` + "`InstantiateTeam`" + ` to spin up the full team without the orchestrator manually creating each member. The orchestrator skill can reference this template instead of hardcoding the roster.

---

## Phase 5d: Per-agent skills and plugins (optional)

Each agent can have its own private skills and plugins that are merged on top of the global registries when that agent spawns. Use this when a specialist agent needs workflows or tools that no other agent should see.

### Directory structure

**Directory-form agent** (preferred for agents with extras):
` + "```" + `
.claudio/agents/<name>/
  AGENT.md          ← agent definition
  skills/           ← agent-private skills (auto-detected)
    my-workflow/
      skill.md
  plugins/          ← agent-private plugins (auto-detected)
    my-tool           ← executable plugin
` + "```" + `

**Flat-file agent with sibling dirs**:
` + "```" + `
.claudio/agents/
  <name>.md         ← agent definition
  <name>/           ← sibling dir (same name, no extension)
    skills/
      my-workflow/skill.md
    plugins/
      my-tool
` + "```" + `

No frontmatter fields needed — Claudio auto-detects ` + "`skills/`" + ` and ` + "`plugins/`" + ` subdirectories alongside the agent definition and merges them at spawn time.

### When to use per-agent skills/plugins

- **Per-agent skill**: a domain-specific workflow only this agent runs (e.g., a security auditor's vulnerability report template, a data engineer's pipeline validation checklist)
- **Per-agent plugin**: an external tool or binary only this agent needs (e.g., a scanner binary for a security agent, a custom linter for a code-quality agent)
- **Do not use** for workflows that are shared across multiple agents — put those in ` + "`.claudio/skills/`" + ` instead

### Example: security agent with private workflow

` + "```" + `
.claudio/agents/security-auditor/
  AGENT.md
  skills/
    owasp-checklist/
      skill.md      ← OWASP Top 10 review steps, only security-auditor uses this
  plugins/
    semgrep          ← static analysis tool binary
` + "```" + `

When ` + "`security-auditor`" + ` spawns, it sees all global skills PLUS ` + "`owasp-checklist`" + ` and all global plugins PLUS ` + "`semgrep`" + `. Other agents spawned in the same session do NOT see these extras.

---

## Phase 5c: Generate harness manifest

Create ` + "`.claudio/harness.json`" + `:

` + "```json" + `
{
  "name": "<harness-name>",
  "version": "1.0.0",
  "description": "<one-line summary>",
  "agents": "agents/",
  "skills": "skills/",
  "templates": "team-templates/",
  "plugins": "plugins/",
  "rules": "rules.md",
  "mcp_servers": {},
  "agent_tool_filters": {}
}
` + "```" + `

Notes:
- ` + "`plugins/`" + `, ` + "`rules.md`" + `, ` + "`mcp_servers`" + `, and ` + "`agent_tool_filters`" + ` are optional — only include if the harness needs them
- The manifest makes the harness compatible with ` + "`claudio harness install`" + ` for portable installation into other projects
- All paths are relative to ` + "`.claudio/`" + ` — never use absolute or home-dir paths

---

## Phase 6: Write the orchestrator skill

Write ` + "`.claudio/skills/<harness-name>/skill.md`" + `. Choose the appropriate template based on the execution mode decided in Phase 3.

### Template A: Agent Team Mode (default)

` + "```markdown" + `
---
name: <harness-name>
description: "<Trigger-rich description. Include task keywords, domain terms, and common phrasings users would say. Example: 'Full-stack feature implementation — designs API, builds UI, writes tests, and creates PR' not 'Runs the feature harness'.>"
---

# <Harness Name> Orchestrator

You are the lead orchestrator for the <domain> harness.

## When invoked

Typical invocations:
- "/<harness-name> <input>"
- "<natural language request matching the description>"

## Architecture

<Pattern name>: <one-sentence description of the flow>

` + "```" + `
<ASCII diagram>
` + "```" + `

## Phase 1: Context and setup

1. Read CLAUDIO.md and any relevant project docs for current conventions
2. Create _workspace/<harness-name>/ for shared artifacts
3. Create the initial task backlog using TaskCreate:
   - Each task: clear owner, input, expected output, acceptance criteria
4. Announce the plan to the user

## Phase 2: Launch the team

Use SpawnTeammate to add members to the active team:

Then send each member specific context:
SendMessage({to: "<agent-a>", message: "<detailed context: what files to read, what to produce, where to write output>"})

Rules:
- Prefer targeted messages ({to: "name"}) over broadcasts ({to: "all"})
- Each initial task must be SELF-CONTAINED — the agent has zero context from your conversation
- Include file paths, conventions, and acceptance criteria in the first message

## Phase 3: Monitor, coordinate, and QA

- Check TaskList periodically to track progress
- When an agent completes, relay relevant output to dependent agents
- If a Producer-Reviewer loop is active, cap retries at 3 rounds
- If an agent reports a blocker, reassign or unblock via SendMessage

### QA cross-validation (if applicable)
After primary work completes, have agents review each other's output:
- SendMessage({to: "<agent-a>", message: "Review <agent-b>'s output at _workspace/<harness-name>/<agent-b>-output.md. Check for [specific criteria]. Report issues."})
- Resolve any conflicts between agent outputs before synthesis

## Phase 4: Synthesize output

1. Read all artifacts from _workspace/<harness-name>/
2. Resolve conflicts or inconsistencies
3. Produce the final output: [format and location]
4. Clean up: delete _workspace/<harness-name>/ if output is in final location
5. Report to user: what was produced, where, any issues, follow-up suggestions

## Follow-up tasks

After completing the main work, suggest concrete next steps:
- "Run tests with: [command]"
- "Review the changes in: [file list]"
- "Extend this harness by adding a [specialist] agent for [gap]"
` + "```" + `

### Template B: Sub-agent Mode (lightweight)

` + "```markdown" + `
---
name: <harness-name>
description: "<trigger-rich description>"
---

# <Harness Name> Orchestrator

## Architecture

<Pattern name> using sub-agents (no inter-agent communication needed).

## Execution

### Step 1: Prepare context
Read [relevant files/docs]. Partition the work into independent units.

### Step 2: Fan out
Launch sub-agents in parallel:

Agent({description: "<agent-a> task", prompt: "<FULL self-contained instructions including all file paths, conventions, and output location>", run_in_background: true})
Agent({description: "<agent-b> task", prompt: "<FULL self-contained instructions>", run_in_background: true})

### Step 3: Collect and synthesize
Wait for all agents to complete. Read their outputs from _workspace/<harness-name>/.
Resolve conflicts. Produce final output.

### Step 4: Report
Tell the user what was produced and suggest follow-up actions.
` + "```" + `

### Orchestrator writing principles

1. **Self-contained first messages** — each agent starts with ZERO context. The initial task/message must include everything: file paths, conventions, what to read, what to produce, where to write.
2. **Trigger-rich descriptions** — the skill description must include keywords users naturally say. "Full-stack feature implementation with API design, React components, and test coverage" triggers on many requests. "Feature harness" triggers on almost nothing.
3. **Follow-up suggestions** — always end with concrete next steps so the harness doesn't become dead code.
4. **QA is not optional** — every harness with 2+ agents producing output should include a cross-validation step where agents review each other's work.

**Adapt all placeholders** to the specific domain, agents, and pattern chosen. Do not leave generic placeholder text in the final file.

---

## Phase 7: Register in CLAUDIO.md

Add a harness entry to CLAUDIO.md (or create it if absent):

` + "```markdown" + `
## Agent Harnesses

### <harness-name>
**Invoke**: /<harness-name> <input>
**Pattern**: <pattern name> (<team|sub-agent>)
**Agents**: <comma-separated list with one-line role>
**Output**: <what it produces and where>
**Use when**: <specific trigger conditions — when should the user reach for this?>
**Created**: <date>
` + "```" + `

Read CLAUDIO.md first. If a harness section already exists, append to it. Never overwrite unrelated content.

---

## Phase 8: Validate structure

Before finishing, run these checks:

### 1. File integrity
- Read each written file — verify NO placeholder text remains (no ` + "`<brackets>`" + `, no ` + "`[TODO]`" + `, no ` + "`...`" + `)
- Verify agent names are consistent across all files (orchestrator references match agent file names exactly)
- Check that every agent referenced in the orchestrator has a corresponding file (if it needed one per Phase 5)
- Verify ` + "`.claudio/agents/`" + ` and ` + "`.claudio/skills/`" + ` directories exist

### 2. Team template and manifest integrity
- Verify ` + "`.claudio/team-templates/<harness-name>.lua`" + ` exists and returns a valid table
- Verify ` + "`.claudio/harness.json`" + ` exists and references correct directories
- Confirm no ` + "`~/.claudio/`" + ` paths appear anywhere in generated files — all output must be project-scoped

### 3. Trigger verification
Test that the skill description triggers correctly. Mentally evaluate these queries:
- Write 3 queries that SHOULD trigger this harness (natural phrasings a user would say)
- Write 3 queries that should NOT trigger it (related but different tasks)
- If any should-trigger query wouldn't match the description, revise the description

### 4. Dry-run check
Mentally walk through a realistic invocation:
- Does Phase 1 (setup) create the right workspace structure?
- Does Phase 2 (launch) give each agent enough context to start working immediately?
- Does Phase 3 (monitor) handle the most likely failure mode?
- Does Phase 4 (synthesize) produce the promised output format?

If any check fails, fix the issue before reporting success.

---

## Phase 9: Evolution protocol

Every harness should be designed to improve over time. Add this section to the orchestrator skill:

` + "```markdown" + `
## Evolution

### When to modify this harness
- An agent consistently produces low-quality output → refine its working principles or split its responsibilities
- A new domain/subsystem is added to the project → add a specialist agent
- The team consistently hits the same blocker → add a dedicated agent or skill to handle it
- An agent is rarely used → merge it into another agent or remove it

### Changelog
| Date | Change | Reason |
|------|--------|--------|
| <creation-date> | Initial creation | <original request> |
` + "```" + `

---

## Phase 10: Report to user

Summarize everything created:
- Files created (with full paths)
- Agent roster with one-line role summaries
- Architecture pattern and execution mode
- Team template path: ` + "`.claudio/team-templates/<harness-name>.lua`" + `
- Harness manifest path: ` + "`.claudio/harness.json`" + `
- How to invoke: ` + "`/<harness-name> <example input>`" + `
- How to install in another project: ` + "`claudio harness install <path-to-this-project/.claudio>`" + `
- 3 example invocations showing different use cases
- Suggested next steps:
  - Run the harness on a real task to validate
  - Extend with additional specialists if gaps emerge
  - Run ` + "`/harness`" + ` again in extend mode to add agents later
  - Share the harness: copy ` + "`.claudio/`" + ` dir or use ` + "`claudio harness install`" + `

---

## Reference: Real-world examples

These condensed examples show how patterns map to real harnesses. Use them as inspiration, not templates.

### Example 1: Research Team (Fan-out, Agent Team)
` + "```" + `
              ┌→ [docs-researcher]     ─┐
[lead]       →├→ [community-researcher] ─┼→ [lead synthesizes]
              ├→ [code-researcher]      ─┤
              └→ [security-researcher]  ─┘
` + "```" + `
4 researchers investigate a topic from different angles (official docs, community posts, code examples, security implications). Lead orchestrator dispatches, collects, and synthesizes into a structured report at ` + "`_workspace/research/final.md`" + `. Agent team mode because researchers benefit from cross-referencing each other's findings mid-research.

### Example 2: Code Review (Fan-out + Discussion, Agent Team)
` + "```" + `
              ┌→ [security-reviewer]  ──┐
[orchestrator]├→ [perf-reviewer]      ──┼→ [orchestrator merges]
              └→ [correctness-reviewer]─┘
              (reviewers can message each other to debate findings)
` + "```" + `
3 reviewers analyze changed files from different angles. After initial review, reviewers discuss disagreements via SendMessage before the orchestrator compiles a final review. Agent team mode because cross-reviewer debate catches issues that individual reviews miss.

### Example 3: Feature Implementation (Pipeline + Fan-out, Agent Team)
` + "```" + `
[planner] → [architect] → SpawnTeammate (×3):
              ┌→ [backend-dev]  ─┐
              ├→ [frontend-dev] ─┼→ [qa-reviewer] → [orchestrator]
              └→ [test-writer]  ─┘
` + "```" + `
Pipeline for planning/design, then fan-out for parallel implementation, then QA review. Planner and architect are sub-agents (sequential, no cross-talk needed). Implementation team uses agent teams (backend/frontend/test can coordinate on shared interfaces). QA reviewer cross-validates all three outputs.

---

## Design principles

**One role per agent** — if an agent has two unrelated responsibilities, split it.
**Communication is value** — agent teams outperform isolated agents because members can challenge, extend, and correct each other in real time. Design communication protocols deliberately.
**Workspace discipline** — all inter-agent artifacts go to ` + "`_workspace/`" + `. Never scatter outputs across the project.
**Fail gracefully** — every agent must have an error path. Harnesses that can't handle failure are brittle.
**Reuse over duplication** — if a specialist role exists in another harness, reference the same agent file.
**Calibrate depth** — for Hierarchical Delegation, stay at ≤ 2 levels. Agent teams cannot be nested.
**Minimal agents** — start with the smallest roster that covers the domain. You can always add specialists later.
**QA is structural** — cross-validation between agents is not optional for production harnesses. Build it into the orchestrator, not as an afterthought.
**Descriptions are triggers** — a skill/agent that can't be found is useless. Write descriptions that match how users actually talk.
**Evolve, don't rebuild** — extend working harnesses incrementally. Only replace when fundamentally broken.`

var cavemanSkillContent = `Respond terse like smart caveman. All technical substance stay. Only fluff die.

## Persistence

ACTIVE EVERY RESPONSE. No revert after many turns. No filler drift. Still active if unsure. Off only: "stop caveman" / "normal mode" from the human user.

Default: **ultra**. Switch: ` + "`/caveman lite|full|ultra`" + `.

## Rules

Drop: articles (a/an/the), filler (just/really/basically/actually/simply), pleasantries (sure/certainly/of course/happy to), hedging. Fragments OK. Short synonyms (big not extensive, fix not "implement a solution for"). Technical terms exact. Code blocks unchanged. Errors quoted exact.

Pattern: ` + "`[thing] [action] [reason]. [next step].`" + `

Not: "Sure! I'd be happy to help you with that. The issue you're experiencing is likely caused by..."
Yes: "Bug in auth middleware. Token expiry check use ` + "`<`" + ` not ` + "`<=`" + `. Fix:"

## Intensity

| Level | What change |
|-------|------------|
| **lite** | No filler/hedging. Keep articles + full sentences. Professional but tight |
| **full** | Drop articles, fragments OK, short synonyms. Classic caveman |
| **ultra** | Abbreviate (DB/auth/config/req/res/fn/impl), strip conjunctions, arrows for causality (X → Y), one word when one word enough |
| **wenyan-lite** | Semi-classical. Drop filler/hedging but keep grammar structure, classical register |
| **wenyan-full** | Maximum classical terseness. Fully 文言文. 80-90% character reduction. Classical sentence patterns |
| **wenyan-ultra** | Extreme abbreviation while keeping classical Chinese feel. Maximum compression |

Example — "Why React component re-render?"
- ultra: "Inline obj prop → new ref → re-render. ` + "`useMemo`" + `."

Example — "Explain database connection pooling."
- ultra: "Pool = reuse DB conn. Skip handshake → fast under load."

## Auto-Clarity

Drop caveman for: security warnings, irreversible action confirmations, multi-step sequences where fragment order risks misread, user asks to clarify or repeats question, **structured protocol output** (e.g. "### Done" completion reports — use exact header and all required fields, caveman style inside the fields is fine). Resume caveman after clear part done.

Example — destructive op:
> **Warning:** This will permanently delete all rows in the ` + "`users`" + ` table and cannot be undone.
> ` + "```sql" + `
> DROP TABLE users;
> ` + "```" + `
> Caveman resume. Verify backup exist first.

## Boundaries

Code/commits/PRs: write normal. "stop caveman" or "normal mode" from human user: revert. Level persist until changed or session end.`

var cavemanReviewSkillContent = `Write code review comments terse and actionable. One line per finding. Location, problem, fix. No throat-clearing.

## User Input
$ARGUMENTS

## Rules

**Format:** ` + "`L<line>: <problem>. <fix>.`" + ` — or ` + "`<file>:L<line>: ...`" + ` when reviewing multi-file diffs.

**Severity prefix (optional, when mixed):**
- 🔴 bug: — broken behavior, will cause incident
- 🟡 risk: — works but fragile (race, missing null check, swallowed error)
- 🔵 nit: — style, naming, micro-optim. Author can ignore
- ❓ q: — genuine question, not a suggestion

**Drop:**
- "I noticed that...", "It seems like...", "You might want to consider..."
- "This is just a suggestion but..." — use nit: instead
- "Great work!", "Looks good overall but..." — say it once at the top, not per comment
- Restating what the line does — the reviewer can read the diff
- Hedging ("perhaps", "maybe", "I think") — if unsure use q:

**Keep:**
- Exact line numbers
- Exact symbol/function/variable names in backticks
- Concrete fix, not "consider refactoring this"
- The *why* if the fix isn't obvious from the problem statement

## Examples

❌ "I noticed that on line 42 you're not checking if the user object is null before accessing the email property. This could potentially cause a crash if the user is not found in the database. You might want to add a null check here."

✅ ` + "`L42: 🔴 bug: user can be null after .find(). Add guard before .email.`" + `

❌ "It looks like this function is doing a lot of things and might benefit from being broken up into smaller functions for readability."

✅ ` + "`L88-140: 🔵 nit: 50-line fn does 4 things. Extract validate/normalize/persist.`" + `

❌ "Have you considered what happens if the API returns a 429? I think we should probably handle that case."

✅ ` + "`L23: 🟡 risk: no retry on 429. Wrap in withBackoff(3).`" + `

## Auto-Clarity

Drop terse mode for: security findings (CVE-class bugs need full explanation + reference), architectural disagreements (need rationale, not just a one-liner), and onboarding contexts where the author is new and needs the "why". In those cases write a normal paragraph, then resume terse for the rest.

## Boundaries

Reviews only — does not write the code fix, does not approve/request-changes, does not run linters. Output the comment(s) ready to paste into the PR. "stop caveman-review" or "normal mode": revert to verbose review style.`

var designSystemSkillContent = `You are extracting a design token system from reference material. Follow this workflow exactly.

## Brief
$ARGUMENTS

## Brand Asset Protocol (run before token extraction when a specific brand is named)

When the user names a specific brand, product, or company, execute this 5-step protocol before extracting any design tokens. Skip to Step 1 only for generic/abstract design systems with no named brand.

### BA-1 · Ask Once — Collect All Assets Upfront

Do not ask piecemeal. Ask once with the full checklist:

` + "```" + `
For <brand/product>, which of these do you have? (priority order):
1. Logo (SVG / hi-res PNG) — required for any brand
2. Product photos / official renders — required for physical products
3. UI screenshots / interface assets — required for digital products (App/SaaS)
4. Color values (HEX / RGB / brand palette)
5. Typography list (Display / Body font names)
6. Brand guidelines PDF / Figma design system / brand website URL

Share what you have; I'll search/download the rest.
` + "```" + `

### BA-2 · Search Official Channels

Search in this order per asset type:

| Asset | Search path |
|---|---|
| Logo | ` + "`<brand>.com/brand`" + ` · ` + "`<brand>.com/press`" + ` · ` + "`<brand>.com/press-kit`" + ` · inline SVG in site ` + "`<head>`" + ` |
| Product image | Brand product page hero · official press kit · YouTube launch film stills |
| UI screenshots | App Store / Google Play product page · official demos · brand Twitter/X posts |
| Colors | Site inline CSS / Tailwind config · brand guidelines PDF |
| Fonts | Site ` + "`<link rel=stylesheet>`" + ` · Google Fonts reference · brand guidelines |

If logo is not found on official channels, stop and ask the user — do not proceed without a logo.

### BA-3 · Quality Gate: "5-10-2-8" Rule

For all assets except logo (logo: use if found, no quality gate needed):

- **5 sources**: search across at least 5 channels before picking
- **10 candidates**: collect at least 10 candidate images before curating
- **2 finals**: pick the 2 best for use in the design
- **8/10 minimum**: each asset must score ≥ 8/10 on:
  1. Resolution (≥ 2000px for print/large screen)
  2. Source legitimacy (official > public domain > free stock)
  3. Brand alignment (matches known brand aesthetic)
  4. Composition quality (clean background, good angle)
  5. Narrative clarity (image tells one clear story)

Assets scoring < 8/10: use an honest placeholder (gray block + "Asset pending" label) or AI-generate using official reference as base. Never use substandard assets to meet a deadline.

### BA-4 · Write brand-spec.md

Write ` + "`brand-spec.md`" + ` alongside ` + "`design-system.json`" + ` in the session dir:

` + "```markdown" + `
# <Brand> · Brand Spec
> Date: YYYY-MM-DD
> Asset sources: <list download sources>
> Completeness: complete / partial / inferred

## Core Assets

### Logo
- Primary: assets/<brand>/logo.svg
- Reversed (light-on-dark): assets/<brand>/logo-white.svg
- Usage: <where it appears>

### Product Images (physical products)
- Hero: assets/<brand>/product-hero.png (score: X/10)
- Detail: assets/<brand>/product-detail.png (score: X/10)

### UI Screenshots (digital products)
- Home: assets/<brand>/ui-home.png
- Feature: assets/<brand>/ui-feature.png

## Design Tokens
- Primary: #XXXXXX (source: <where extracted from>)
- Background: #XXXXXX
- Ink: #XXXXXX
- Accent: #XXXXXX
- Display font: <name>
- Body font: <name>

## Prohibited
- <colors or patterns explicitly forbidden by brand guidelines>

## Brand voice keywords
- <3-5 adjectives>
` + "```" + `

### BA-5 · Fallback Handling

| Missing asset | Action |
|---|---|
| Logo not found | Stop — ask user before proceeding |
| Product image not found | AI-generate using official reference as base; label "AI-generated placeholder" |
| UI screenshots not found | Ask user to share their own account screenshot |
| Colors not found | Infer from 3 design directions; label as "inferred — confirm with brand team" |

Never silently fill missing assets with generic gradients or CSS silhouettes — they strip brand identity. An honest placeholder beats a fake asset.

## Step 1 — Gather Reference Material

Ask the user for one or more of:
- A screenshot path (you will use vision to analyze colors and typography)
- A live URL (use RenderMockup to capture, then analyze the screenshot)
- A codebase path (read CSS custom properties, Tailwind config, theme files)
- Brand asset files (SVG files, font metadata)
- A plain description (e.g. "dark fintech app, deep navy + gold accents, Inter font")

If the user provides nothing, ask before proceeding.

## Step 2 — Extract Tokens by Source Type

### From a screenshot
Use vision to identify:
- Dominant background color → background
- Primary action color (buttons, links, CTAs) → primary
- Secondary action color → secondary
- Accent / highlight color → accent
- Surface color (cards, panels) → surface
- Primary text color → text
- Muted text color (labels, captions) → textMuted
- Font families in use (heading, body)
- Approximate spacing rhythm (4px or 8px base?)
- Border radius style (sharp / medium / rounded / pill)

### From a live URL
1. Call RenderMockup with the URL to capture a screenshot
2. Analyze the screenshot as above

### From a codebase
Search for and read:
- CSS custom properties (` + "`--color-*`" + `, ` + "`--font-*`" + `, ` + "`--spacing-*`" + `, ` + "`--radius-*`" + `)
- Tailwind config (` + "`tailwind.config.js`" + ` / ` + "`tailwind.config.ts`" + `) → ` + "`theme.extend`" + `
- Theme files (` + "`theme.ts`" + `, ` + "`tokens.ts`" + `, ` + "`colors.ts`" + `, ` + "`design-tokens.json`" + `)
- Styled-components / Emotion theme objects

### From brand assets (SVG / fonts)
- Read SVG files: extract ` + "`fill`" + ` and ` + "`stroke`" + ` hex values
- Font metadata: extract family names from ` + "`font-face`" + ` declarations or font file names

### From a description
Derive a coherent palette and type system from the description. Make explicit choices and state them.

## Step 3 — Build the Token Object

Produce a JSON object with this exact schema:

` + "```json" + `
{
  "colors": {
    "primary":    "#hex",
    "secondary":  "#hex",
    "accent":     "#hex",
    "background": "#hex",
    "surface":    "#hex",
    "text":       "#hex",
    "textMuted":  "#hex"
  },
  "fonts": {
    "heading": "Font Name",
    "body":    "Font Name",
    "mono":    "Font Name"
  },
  "spacing": {
    "xs":  "4px",
    "sm":  "8px",
    "md":  "16px",
    "lg":  "24px",
    "xl":  "40px",
    "2xl": "64px"
  },
  "radius": {
    "sm":   "4px",
    "md":   "8px",
    "lg":   "16px",
    "full": "9999px"
  },
  "shadows": {
    "sm": "0 1px 2px rgba(0,0,0,0.05)",
    "md": "0 4px 6px rgba(0,0,0,0.1)",
    "lg": "0 10px 15px rgba(0,0,0,0.15)"
  }
}
` + "```" + `

Rules:
- All color values must be hex (e.g. #1a1a2e) — no rgb(), no hsl()
- Font names must be quoted strings ready for CSS font-family
- Spacing and radius values must be px strings
- Shadow values must be valid CSS box-shadow strings
- If a token cannot be determined, use a sensible default and note it

## Step 4 — Save Output

Call ` + "`CreateDesignSession`" + ` with ` + "`name: \"design-system\"`" + ` to get the absolute ` + "`session_dir`" + `.

Save the JSON to: ` + "`{session_dir}/design-system.json`" + ` using the Write tool with the full absolute path returned by the tool.

## Step 5 — Confirm

Report:
- Total token count (count all leaf values)
- Palette preview: print each color key with its hex value
- Font choices
- Output path confirmed`

var mockupSkillContent = `You are generating a complete multi-screen HTML mockup. Follow this workflow exactly.

## Brief
$ARGUMENTS

## Design Session Management

**Existing design (iterate/update):** Call ` + "`ListDesigns`" + ` first. If a session exists for this project, use its ` + "`session_dir`" + ` as your working directory for all file writes.

**New design:** Call ` + "`CreateDesignSession`" + ` with an optional ` + "`name`" + ` (e.g. ` + "`\"dashboard-mockup\"`" + `). Use the returned ` + "`session_dir`" + ` for ALL file writes. Never create a ` + "`designs/`" + ` folder in the project root.

> The tool creates the session dir and copies all starter JSX files into it automatically.

Your ` + "`screens.jsx`" + ` file MUST use these exports — do NOT reimplement them:
- **design-canvas.jsx**: ` + "`DesignCanvas`" + `, ` + "`DCSection`" + `, ` + "`DCArtboard`" + ` — pan/zoom canvas
- **device-frames.jsx**: device frames for all screens
- **ui-kit.jsx**: all UI components — ` + "`Button`" + `, ` + "`Input`" + `, ` + "`NavBar`" + `, ` + "`Sidebar`" + `, ` + "`Card`" + `, ` + "`Badge`" + `, ` + "`Modal`" + `, ` + "`Tabs`" + `, ` + "`Toast`" + ` etc.
- **chart-kit.jsx** (access via ` + "`window.ChartKit`" + `): ` + "`LineChart`" + `, ` + "`BarChart`" + `, ` + "`PieChart`" + `, ` + "`DonutChart`" + `, ` + "`AreaChart`" + `, ` + "`SparkLine`" + ` — use for any data visualisation

## Step 1 — Brief Clarification

Ask the user for:
1. **Product brief** — what is this product? Who uses it? What is the core action?
2. **Target screens** — which screens to generate? Default if not specified: Landing, Dashboard, Detail, Settings
3. **Design tokens** — path to a design-system.json file (optional; if not provided, you will define tokens inline)
4. **Platform** — web (desktop-first), web (mobile-first), or both? Default: web desktop-first

If the user has already provided these, skip asking and proceed.

## Step 2 — Design Direction

Before generating any HTML, state your design direction in one paragraph:
- Aesthetic (e.g. "clean SaaS, light mode, generous whitespace, Inter font, blue primary")
- Layout approach (e.g. "sidebar nav, content area, card-based data display")
- Color mood (e.g. "professional, trustworthy, calm")

Commit to one direction. Do not hedge or offer alternatives at this stage.

## Step 3 — Load or Define Tokens

If a design-system.json path was provided:
- Read the file
- Extract colors, fonts, spacing, radius, shadows

If no file provided, define inline tokens matching your stated direction:
` + "```json" + `
{
  "colors": { "primary": "#hex", "background": "#hex", "surface": "#hex", "text": "#hex", "textMuted": "#hex" },
  "fonts":  { "heading": "Inter", "body": "Inter", "mono": "JetBrains Mono" }
}
` + "```" + `

## Step 3.5 — Set Output Directory

If no existing session was found above, call ` + "`CreateDesignSession`" + ` with an optional ` + "`name`" + `. Use the returned ` + "`session_dir`" + ` for all file writes — this is also the ` + "`session_dir`" + ` argument for ` + "`RenderMockup`" + ` and ` + "`BundleMockup`" + `.

## Step 4 — Generate Screens (screens.jsx)

Write a single ` + "`screens.jsx`" + ` file that renders all screens using the starter globals.

At the top of ` + "`screens.jsx`" + `, destructure what you need from the loaded starters:
` + "```" + `jsx
const { DesignCanvas, DCSection, DCArtboard } = window;
const { IOSDevice, AndroidDevice, DesktopBrowser } = window;
const { Button, Input, Select, NavBar, Sidebar, Card, Badge, Modal, Tabs, Toast, useToast, ToastContainer } = window;
const { LineChart, BarChart, PieChart, DonutChart, AreaChart, SparkLine } = window.ChartKit;
` + "```" + `

Then write your screen components using these primitives. Do NOT redefine them.

Structure your screens using ` + "`DesignCanvas`" + ` with ` + "`DCSection`" + ` and ` + "`DCArtboard`" + ` for layout:
- Wrap mobile screens in ` + "`IOSDevice`" + ` or ` + "`AndroidDevice`"  + `
- Wrap desktop screens in ` + "`DesktopBrowser`" + `
- Use ` + "`Button`" + `, ` + "`Card`" + `, ` + "`NavBar`" + `, ` + "`Sidebar`" + ` etc. for UI components
- Use ` + "`LineChart`" + `, ` + "`BarChart`" + ` etc. for any data visualisation
- Include realistic placeholder content — not lorem ipsum

Save as ` + "`screens.jsx`" + ` using the Write tool.

## Step 5 — Generate index.html

Create ` + "`index.html`" + ` with this exact script load order:

` + "```" + `html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Mockup</title>
  <script src="https://unpkg.com/react@18/umd/react.development.js" crossorigin></script>
  <script src="https://unpkg.com/react-dom@18/umd/react-dom.development.js" crossorigin></script>
  <script src="https://unpkg.com/@babel/standalone/babel.min.js"></script>
</head>
<body style="margin:0;background:#f5f5f5;">
  <div id="root"></div>
  <!-- Starters (local copies) -->
  <script type="text/babel" src="design-canvas.jsx"></script>
  <script type="text/babel" src="device-frames.jsx"></script>
  <script type="text/babel" src="ui-kit.jsx"></script>
  <script type="text/babel" src="chart-kit.jsx"></script>
  <!-- Your screens (written by AI) -->
  <script type="text/babel" src="screens.jsx"></script>
</body>
</html>
` + "```" + `

Save as ` + "`index.html`" + ` in the same directory as ` + "`screens.jsx`" + `.

## Step 6 — Render and Verify

1. Call ` + "`RenderMockup`" + ` with ` + "`html_path`" + ` = path to ` + "`index.html`" + ` and ` + "`session_dir`" + ` = DESIGNS_BASE
2. Call ` + "`VerifyMockup`" + ` with the full-canvas screenshot path returned by RenderMockup

If ` + "`pass: false`" + `:
- Read the ` + "`issues`" + ` list from VerifyMockup output
- Fix each blocking issue in the relevant screen HTML file(s)
- Re-render and re-verify (pass ` + "`session_dir`" + ` = DESIGNS_BASE each time)
- Repeat up to 3 cycles maximum
- After 3 cycles, report remaining issues to the user and stop

If ` + "`pass: true`" + `: proceed to Step 7.

## Step 7 — Bundle

Call ` + "`BundleMockup`" + ` with:
- ` + "`entry_html`" + `: path to ` + "`index.html`" + `
- ` + "`session_dir`" + `: DESIGNS_BASE
- ` + "`embed_cdn`" + `: true

The tool writes ` + "`{DESIGNS_BASE}/bundle/mockup.html`" + ` and pushes a clickable link to the chat. Do NOT show the raw file path — only show the URL returned by the tool.

## Step 8 — Report

Tell the user:
- List of all screen files generated
- The bundle URL (from BundleMockup tool output — show this, not the file path)
- Overall VerifyMockup score
- Any issues that could not be auto-fixed (if any)`

var handoffSkillContent = `You are generating a developer handoff package from a completed mockup. Follow this workflow exactly.

## Brief
$ARGUMENTS

## Step 1 — Discover Design Sessions

Call ` + "`ListDesigns`" + ` (no parameters needed — it reads the current project automatically).

It returns all sessions for this project. Each session has:
- ` + "`session_dir`" + ` — absolute path to the session directory
- ` + "`session`" + ` — session name / timestamp
- ` + "`screens[]`" + ` — list of screen names
- ` + "`has_handoff`" + ` — true if handoff already exists
- ` + "`bundle_path`" + ` — path to ` + "`bundle/mockup.html`" + ` if bundled

Pick the correct session (usually the most recent, or whichever the user specified). Use its ` + "`session_dir`" + ` as ` + "`SESSION_DIR`" + ` in all subsequent steps.

If no sessions exist, tell the user to run a design skill first (` + "`/hifi`" + `, ` + "`/mockup`" + `, etc.).

## Step 2 — Generate Handoff Package

Call ` + "`ExportHandoff`" + ` with:
- ` + "`mockup_dir`" + ` — ` + "`{SESSION_DIR}/bundle`" + ` (contains ` + "`mockup.html`" + `)
- ` + "`session_dir`" + ` — ` + "`SESSION_DIR`" + ` (so output lands in the same session)
- ` + "`framework`" + ` — ask the user which framework they are implementing in (` + "`react`" + ` / ` + "`vue`" + ` / ` + "`svelte`" + ` / ` + "`vanilla`" + `); default ` + "`react`" + ` if not specified
- ` + "`design_tokens`" + ` — pass path to ` + "`design-system.json`" + ` if it exists in ` + "`SESSION_DIR`" + ` or parent designs dir
- ` + "`project_name`" + ` — use the git repo name or directory name

` + "`ExportHandoff`" + ` automatically generates:
- ` + "`handoff/spec.md`" + ` — component inventory, token usage, interaction spec, asset list
- ` + "`handoff/tokens-used.json`" + ` — token → locations map
- ` + "`handoff/tokens.json`" + ` — full token set (copied from session)
- ` + "`handoff/tokens.css`" + ` — CSS custom properties ready to import
- ` + "`handoff/tailwind.config.js`" + ` — Tailwind config derived from design tokens

## Step 3 — Review Handoff Output

Read ` + "`handoff/spec.md`" + ` and verify it contains:
- Component inventory table (all major UI components listed)
- Token usage map (colors, fonts, spacing referenced)
- Interaction spec per screen (buttons, links, modals, form behavior)
- Asset list (fonts, icons, images)
- Implementation notes (responsive breakpoints, a11y flags)

If any section is thin or missing, supplement it:
- Read the screen HTML files from ` + "`SESSION_DIR/screenshots/`" + ` or ` + "`bundle/mockup.html`" + ` directly
- Append missing detail to ` + "`handoff/spec.md`" + ` using the Write tool

## Step 4 — Fidelity Verification (after implementation)

After the implementation agent builds the UI, verify it matches the design.

Call ` + "`ReviewDesignFidelity`" + ` with:
- ` + "`session_name`" + ` — the session name from Step 1 (or omit to use newest)
- ` + "`screens`" + ` — array mapping each design screen to its implemented counterpart:
  - ` + "`name`" + ` — screen name exactly as returned by ` + "`ListDesigns`" + `
  - ` + "`url`" + ` — live URL of the implemented page (preferred, e.g. ` + "`http://localhost:8080/dashboard`" + `)
  - OR ` + "`template_path`" + ` — path to the HTML template file if no server is running
  - ` + "`css_paths`" + ` — list any CSS files needed to render the template correctly

Pass threshold: ` + "`overall_score >= 75`" + `. If score is below 75, report which screens failed and what differs visually.

## Step 4 — Confirm

Report to the user:
- Session used (name + path)
- Handoff package location (` + "`handoff/`" + ` dir)
- Files generated (spec.md, tokens.css, tailwind.config.js, etc.)
- Component count + token count from spec
- Any accessibility or contrast issues flagged

## Next Steps (tell the user)

Once the dev agent has implemented the UI, verify fidelity by running ` + "`/test`" + ` and asking it to call ` + "`ReviewDesignFidelity`" + ` against the live implementation. Pass threshold is score ≥ 75.`

var hifiSkillContent = `You are generating high-fidelity mockups with named design variations and a live Tweaks panel. Follow this workflow exactly.

## Brief
$ARGUMENTS

## Anti-AI-Slop Doctrine (read before generating anything)

AI slop = the visual "maximum common denominator" baked into training data. Using it makes every design look like every other AI output — stripping brand identity. Avoid the following patterns. The only legitimate exception is when the brand itself uses this element (documented in brand-spec.md).

### 20 Tropes to Avoid

| # | Trope | Why it's slop | When it's OK |
|---|---|---|---|
| 1 | Aggressive purple/violet gradients | Default "tech/AI/SaaS" shorthand — appears on thousands of landing pages | Brand spec explicitly uses it (e.g. Linear) |
| 2 | Emoji as icons | "Not professional enough, add emoji to pad" pattern | Brand targets children, or brand style is emoji-native (Notion) |
| 3 | Rounded card + left colored border accent | 2020-2024 Material/Tailwind era cliché, now visual noise | User explicitly requests it, or it's in brand spec |
| 4 | SVG-drawn human faces / scenes | AI-drawn SVG figures have broken proportions and uncanny valley feel | Almost never — use real photos or honest placeholder |
| 5 | CSS silhouette replacing real product image | Results in "generic tech animation" — any brand looks identical | Almost never — run Brand Asset Protocol first |
| 6 | Inter/Roboto/Arial as display (headline) font | Too ubiquitous to signal "designed product" | Brand spec explicitly specifies these (e.g. Stripe uses tuned Inter) |
| 7 | Cyber neon / "#0D1117" dark background | GitHub dark mode clone — ubiquitous in developer tool imitators | Developer tool brand that deliberately adopts this direction |
| 8 | Glassmorphism blur cards everywhere | 2021 trend, now overused in every AI demo | Used sparingly as a single accent element, not the whole UI |
| 9 | Gradient hero banners with centered white text | Default "make it look premium" move that screams template | Part of documented brand direction |
| 10 | Decorative data slop (fake stats, random icons) | Numbers and icons without meaning dilute trust | Real data is provided by user |
| 11 | Icon for every bullet / heading | "Padding the design with visual weight" anti-pattern | Design system explicitly uses icon-label pattern |
| 12 | Generic world map or globe graphic | AI placeholder for "global/international" | Content is literally geographic |
| 13 | Three-column feature grid with icon + title + body | Default SaaS landing page template, no differentiation | Brief explicitly requires feature comparison |
| 14 | Floating chat bubble in lower right | Overused SaaS support widget visual cliché | Product is literally a chat widget |
| 15 | Testimonial cards with stock photo avatar + stars | Low-trust visual, users have learned to ignore | Real testimonials with real attribution are provided |
| 16 | "As seen in" logo parade (Forbes, TechCrunch, etc.) | Hollow social proof without context | Actual press coverage is documented |
| 17 | Animated typing cursor on hero headline | "We're a startup" signal — too common | Animation is thematically relevant (e.g. code editor product) |
| 18 | Confetti / celebration animation on CTA | Cheap dopamine — overused in growth-hacking templates | Genuine milestone celebration moment in the product |
| 19 | Dark sidebar + white content area as default layout | Chrome-era admin panel default, no personality | Design system specifies this layout explicitly |
| 20 | Lorem ipsum placeholder text | Signals "design not thought through" — use real or realistic copy | Never — always use product-relevant placeholder copy |

### Positive Rules (what to do instead)

- Use "text-wrap: pretty" + CSS Grid + advanced CSS — these signal real craftsmanship
- Use oklch() or brand-spec colors only — never invent new hex values during generation
- Apply "one element at 120%, rest at 80%" — precision in one signature detail > uniform mediocrity
- Choose display fonts that match the brand's energy — not whatever ships with the OS
- Earn every element's presence — if it doesn't serve the content, remove it

## Design Session Management

**Existing design (iterate/update):** Call ` + "`ListDesigns`" + ` first. If a session exists for this project, use its ` + "`session_dir`" + ` as your working directory for all file writes.

**New design:** Call ` + "`CreateDesignSession`" + ` with an optional ` + "`name`" + ` (e.g. ` + "`\"dashboard-hifi\"`" + `). Use the returned ` + "`session_dir`" + ` for ALL file writes. Never create a ` + "`designs/`" + ` folder in the project root.

> The tool creates the session dir and copies all starter JSX files into it automatically.

Your ` + "`screens.jsx`" + ` file MUST use these exports — do NOT reimplement them:
- **design-canvas.jsx**: ` + "`DesignCanvas`" + `, ` + "`DCSection`" + `, ` + "`DCArtboard`" + `, ` + "`DCPostIt`" + ` — wrap all screens in a pan/zoom canvas
- **device-frames.jsx**: ` + "`IOSDevice`" + `, ` + "`AndroidDevice`" + `, ` + "`DesktopBrowser`" + ` — always wrap mobile/desktop screens in a device frame
- **ui-kit.jsx**: ` + "`Button`" + `, ` + "`Input`" + `, ` + "`Select`" + `, ` + "`Modal`" + `, ` + "`NavBar`" + `, ` + "`Sidebar`" + `, ` + "`Card`" + `, ` + "`Badge`" + `, ` + "`Tabs`" + `, ` + "`Toast`" + ` etc — use for all UI components
- **chart-kit.jsx** (access via ` + "`window.ChartKit`" + `): ` + "`LineChart`" + `, ` + "`BarChart`" + `, ` + "`PieChart`" + `, ` + "`DonutChart`" + `, ` + "`AreaChart`" + `, ` + "`SparkLine`" + ` — use for any data visualisation

## Step 1 — Clarification

Ask the user:
1. **Product name** — what is this product called?
2. **Audience** — who are the primary users?
3. **Brand / existing design system** — provide a design-system.json path, a URL, or a description. If none, you will invent tokens.
4. **Tone** — playful / serious / corporate / other?
5. **Variation count** — how many named design directions? Default: 3.
6. **Variation briefs** — one sentence per variation describing its visual direction (e.g. "Minimal light", "Bold dark", "Warm editorial"). If not provided, invent distinct directions.

If the user has already answered these, skip asking and proceed.

## Step 2 — Define CSS Variable Tokens

Before writing any HTML, define the token set for each variation. Every variation must have its own palette block. Use only CSS custom properties — no hardcoded hex, px, or font-name values anywhere in the final HTML.

Required token names (add more as needed):

` + "```" + `
--color-bg
--color-surface
--color-surface-alt
--color-primary
--color-primary-hover
--color-on-primary
--color-text
--color-muted
--color-border
--color-accent
--font-family
--font-family-heading
--spacing-unit
--border-radius
--radius-sm
--radius-md
--radius-lg
--shadow-sm
--shadow-md
--shadow-lg
--transition-duration
--animation-easing
` + "```" + `

## Step 3 — Generate screens.jsx with Tab Strip and Variations

Write ` + "`screens.jsx`" + ` — a React file that exports the full hi-fi UI with tab strip, variation containers, and tweaks panel. This file uses the starter globals loaded by ` + "`index.html`" + `.

At the top of ` + "`screens.jsx`" + `, destructure what you need:
` + "```" + `jsx
const { DesignCanvas, DCSection, DCArtboard } = window;
const { IOSDevice, AndroidDevice, DesktopBrowser } = window;
const { Button, Input, Select, NavBar, Sidebar, Card, Badge, Modal, Tabs, Toast, useToast, ToastContainer } = window;
const { LineChart, BarChart, PieChart, DonutChart, AreaChart, SparkLine } = window.ChartKit;
` + "```" + `

Then write your components using these primitives — do NOT redefine them.

### Tab strip (inside the React render)

` + "```html" + `
<nav id="variation-tabs" style="position:sticky;top:0;z-index:1000;display:flex;gap:0;background:var(--color-surface);border-bottom:1px solid var(--color-border);">
  <button class="tab-btn active" data-target="variation-0" onclick="showVariation(0)" style="padding:0.75rem 1.5rem;border:none;cursor:pointer;font-family:var(--font-family);background:var(--color-primary);color:var(--color-on-primary);">Variation 1 Name</button>
  <button class="tab-btn" data-target="variation-1" onclick="showVariation(1)" style="padding:0.75rem 1.5rem;border:none;cursor:pointer;font-family:var(--font-family);background:var(--color-surface);color:var(--color-text);">Variation 2 Name</button>
  <button class="tab-btn" data-target="variation-2" onclick="showVariation(2)" style="padding:0.75rem 1.5rem;border:none;cursor:pointer;font-family:var(--font-family);background:var(--color-surface);color:var(--color-text);">Variation 3 Name</button>
</nav>
` + "```" + `

### Variation containers

Wrap each variation in:

` + "```html" + `
<div id="variation-0" class="variation" style="display:block;">
  <!-- full page content for variation 1 -->
</div>
<div id="variation-1" class="variation" style="display:none;">
  <!-- full page content for variation 2 -->
</div>
<div id="variation-2" class="variation" style="display:none;">
  <!-- full page content for variation 3 -->
</div>
` + "```" + `

Tab switching script:

` + "```html" + `
<script>
function showVariation(idx) {
  document.querySelectorAll('.variation').forEach(function(el, i) {
    el.style.display = i === idx ? 'block' : 'none';
  });
  document.querySelectorAll('.tab-btn').forEach(function(btn, i) {
    btn.style.background = i === idx ? 'var(--color-primary)' : 'var(--color-surface)';
    btn.style.color = i === idx ? 'var(--color-on-primary)' : 'var(--color-text)';
  });
}
</script>
` + "```" + `

Each variation must be a **complete, realistic page** with navigation, hero or header, content sections, and footer. Use product-relevant copy — not lorem ipsum.

## Step 4 — Embed Live Tweaks Panel

Embed the following snippet verbatim inside ` + "`<body>`" + ` before ` + "`</body>`" + `. Populate the ` + "`palettes`" + ` object using your actual variation token values:

` + "```html" + `
<div id="tweaks-panel" style="position:fixed;bottom:1rem;right:1rem;z-index:9999;background:var(--color-surface);border:1px solid var(--color-border);border-radius:var(--radius-md);padding:var(--spacing-unit);font-family:var(--font-family);font-size:0.75rem;box-shadow:var(--shadow-lg);min-width:200px;">
  <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:calc(var(--spacing-unit) * 0.5);">
    <strong style="color:var(--color-text);">Live Tweaks</strong>
    <button id="tweaks-toggle" onclick="var b=document.getElementById('tweaks-body');b.style.display=b.style.display==='none'?'block':'none'" style="background:none;border:none;cursor:pointer;color:var(--color-text);font-size:1rem;">&#8963;</button>
  </div>
  <div id="tweaks-body">
    <label style="display:block;margin-bottom:calc(var(--spacing-unit) * 0.5);color:var(--color-text);">Palette
      <select id="palette-select" onchange="applyPalette(this.value)" style="display:block;width:100%;margin-top:0.25rem;padding:0.25rem;font-family:var(--font-family);border:1px solid var(--color-border);border-radius:var(--radius-sm);background:var(--color-surface-alt);color:var(--color-text);">
        <option value="default">Default</option>
        <option value="warm">Warm</option>
        <option value="cool">Cool</option>
        <option value="high-contrast">High Contrast</option>
      </select>
    </label>
    <label style="display:block;margin-bottom:calc(var(--spacing-unit) * 0.5);color:var(--color-text);">Font
      <select onchange="applyFont(this.value)" style="display:block;width:100%;margin-top:0.25rem;padding:0.25rem;font-family:var(--font-family);border:1px solid var(--color-border);border-radius:var(--radius-sm);background:var(--color-surface-alt);color:var(--color-text);">
        <option value="sans">Sans</option>
        <option value="serif">Serif</option>
        <option value="mono">Mono</option>
      </select>
    </label>
    <label style="display:block;margin-bottom:calc(var(--spacing-unit) * 0.5);color:var(--color-text);">Density
      <select onchange="applyDensity(this.value)" style="display:block;width:100%;margin-top:0.25rem;padding:0.25rem;font-family:var(--font-family);border:1px solid var(--color-border);border-radius:var(--radius-sm);background:var(--color-surface-alt);color:var(--color-text);">
        <option value="comfortable" selected>Comfortable</option>
        <option value="compact">Compact</option>
        <option value="spacious">Spacious</option>
      </select>
    </label>
    <label style="display:block;margin-bottom:calc(var(--spacing-unit) * 0.5);color:var(--color-text);">Border Radius
      <select onchange="applyBorderRadius(this.value)" style="display:block;width:100%;margin-top:0.25rem;padding:0.25rem;font-family:var(--font-family);border:1px solid var(--color-border);border-radius:var(--radius-sm);background:var(--color-surface-alt);color:var(--color-text);">
        <option value="none">Sharp (0)</option>
        <option value="subtle">Subtle (4px)</option>
        <option value="rounded" selected>Rounded (8px)</option>
        <option value="large">Large (16px)</option>
        <option value="pill">Pill (9999px)</option>
      </select>
    </label>
    <label style="display:block;margin-bottom:calc(var(--spacing-unit) * 0.5);color:var(--color-text);">Spacing Scale
      <select onchange="applySpacingUnit(this.value)" style="display:block;width:100%;margin-top:0.25rem;padding:0.25rem;font-family:var(--font-family);border:1px solid var(--color-border);border-radius:var(--radius-sm);background:var(--color-surface-alt);color:var(--color-text);">
        <option value="tight">Tight (0.75rem)</option>
        <option value="comfortable" selected>Comfortable (1rem)</option>
        <option value="airy">Airy (1.5rem)</option>
      </select>
    </label>
    <label style="display:block;margin-bottom:calc(var(--spacing-unit) * 0.5);color:var(--color-text);">Motion Speed
      <select onchange="applyMotion(this.value)" style="display:block;width:100%;margin-top:0.25rem;padding:0.25rem;font-family:var(--font-family);border:1px solid var(--color-border);border-radius:var(--radius-sm);background:var(--color-surface-alt);color:var(--color-text);">
        <option value="instant">Instant (0ms)</option>
        <option value="fast">Fast (150ms)</option>
        <option value="normal" selected>Normal (300ms)</option>
        <option value="slow">Slow (500ms)</option>
      </select>
    </label>
    <label style="display:block;margin-bottom:calc(var(--spacing-unit) * 0.5);color:var(--color-text);">Easing
      <select onchange="applyEasing(this.value)" style="display:block;width:100%;margin-top:0.25rem;padding:0.25rem;font-family:var(--font-family);border:1px solid var(--color-border);border-radius:var(--radius-sm);background:var(--color-surface-alt);color:var(--color-text);">
        <option value="ease-out" selected>Ease Out (natural)</option>
        <option value="ease-in-out">Ease In-Out (balanced)</option>
        <option value="spring">Spring (overshoot)</option>
        <option value="linear">Linear (mechanical)</option>
      </select>
    </label>
    <button onclick="toggleDark()" style="display:block;width:100%;padding:0.375rem var(--spacing-unit);background:var(--color-primary);color:var(--color-on-primary);border:none;border-radius:var(--radius-sm);cursor:pointer;font-family:var(--font-family);">Toggle Dark / Light</button>
  </div>
</div>
<script>
(function() {
  var R = document.documentElement;
  var dark = false;
  // REPLACE these palette values with your actual design tokens
  var palettes = {
    'default':       {'--color-bg':'#ffffff','--color-surface':'#f8f9fa','--color-surface-alt':'#f1f3f5','--color-primary':'#2563eb','--color-primary-hover':'#1d4ed8','--color-on-primary':'#ffffff','--color-text':'#111827','--color-muted':'#6b7280','--color-border':'#e5e7eb','--color-accent':'#7c3aed'},
    'warm':          {'--color-bg':'#fffbf5','--color-surface':'#fef3e2','--color-surface-alt':'#fde8c8','--color-primary':'#ea580c','--color-primary-hover':'#c2410c','--color-on-primary':'#ffffff','--color-text':'#1c0a00','--color-muted':'#92400e','--color-border':'#fcd9aa','--color-accent':'#d97706'},
    'cool':          {'--color-bg':'#f0f9ff','--color-surface':'#e0f2fe','--color-surface-alt':'#bae6fd','--color-primary':'#0284c7','--color-primary-hover':'#0369a1','--color-on-primary':'#ffffff','--color-text':'#0c1a2e','--color-muted':'#475569','--color-border':'#7dd3fc','--color-accent':'#6366f1'},
    'high-contrast': {'--color-bg':'#000000','--color-surface':'#1a1a1a','--color-surface-alt':'#2a2a2a','--color-primary':'#facc15','--color-primary-hover':'#eab308','--color-on-primary':'#000000','--color-text':'#ffffff','--color-muted':'#d1d5db','--color-border':'#404040','--color-accent':'#f472b6'}
  };
  var fonts = {
    'sans':  'Inter, system-ui, -apple-system, sans-serif',
    'serif': 'Georgia, "Times New Roman", serif',
    'mono':  '"JetBrains Mono", "Fira Code", "Courier New", monospace'
  };
  var densities = {
    'compact':     '0.75rem',
    'comfortable': '1rem',
    'spacious':    '1.5rem'
  };
  window.applyPalette = function(name) {
    var p = palettes[name] || palettes['default'];
    Object.keys(p).forEach(function(k) { R.style.setProperty(k, p[k]); });
  };
  window.applyFont = function(name) {
    R.style.setProperty('--font-family', fonts[name] || fonts['sans']);
    R.style.setProperty('--font-family-heading', fonts[name] || fonts['sans']);
  };
  window.applyDensity = function(name) {
    R.style.setProperty('--spacing-unit', densities[name] || densities['comfortable']);
  };
  var borderRadii = {
    'none':    { '--border-radius': '0', '--radius-sm': '0', '--radius-md': '0', '--radius-lg': '0' },
    'subtle':  { '--border-radius': '4px', '--radius-sm': '2px', '--radius-md': '4px', '--radius-lg': '8px' },
    'rounded': { '--border-radius': '8px', '--radius-sm': '4px', '--radius-md': '8px', '--radius-lg': '16px' },
    'large':   { '--border-radius': '16px', '--radius-sm': '8px', '--radius-md': '16px', '--radius-lg': '24px' },
    'pill':    { '--border-radius': '9999px', '--radius-sm': '9999px', '--radius-md': '9999px', '--radius-lg': '9999px' }
  };
  window.applyBorderRadius = function(name) {
    var r = borderRadii[name] || borderRadii['rounded'];
    Object.keys(r).forEach(function(k) { R.style.setProperty(k, r[k]); });
  };
  var spacingUnits = { 'tight': '0.75rem', 'comfortable': '1rem', 'airy': '1.5rem' };
  window.applySpacingUnit = function(name) {
    R.style.setProperty('--spacing-unit', spacingUnits[name] || spacingUnits['comfortable']);
  };
  var motionSpeeds = { 'instant': '0ms', 'fast': '150ms', 'normal': '300ms', 'slow': '500ms' };
  window.applyMotion = function(name) {
    R.style.setProperty('--transition-duration', motionSpeeds[name] || motionSpeeds['normal']);
  };
  var easings = {
    'ease-out':    'cubic-bezier(0.0, 0, 0.2, 1)',
    'ease-in-out': 'cubic-bezier(0.4, 0, 0.2, 1)',
    'spring':      'cubic-bezier(0.34, 1.56, 0.64, 1)',
    'linear':      'linear'
  };
  window.applyEasing = function(name) {
    R.style.setProperty('--animation-easing', easings[name] || easings['ease-out']);
  };
  window.toggleDark = function() {
    dark = !dark;
    if (dark) {
      R.style.setProperty('--color-bg', '#0f172a');
      R.style.setProperty('--color-surface', '#1e293b');
      R.style.setProperty('--color-surface-alt', '#334155');
      R.style.setProperty('--color-text', '#f1f5f9');
      R.style.setProperty('--color-muted', '#94a3b8');
      R.style.setProperty('--color-border', '#334155');
    } else {
      window.applyPalette(document.getElementById('palette-select').value || 'default');
    }
  };
})();
</script>
` + "```" + `

## Step 5 — CSS Token Block

At the top of ` + "`<style>`" + ` in ` + "`<head>`" + `, set default token values using ` + "`:root`" + `. Example structure:

` + "```html" + `
<style>
  :root {
    --color-bg: #ffffff;
    --color-surface: #f8f9fa;
    --color-surface-alt: #f1f3f5;
    --color-primary: #2563eb;
    --color-primary-hover: #1d4ed8;
    --color-on-primary: #ffffff;
    --color-text: #111827;
    --color-muted: #6b7280;
    --color-border: #e5e7eb;
    --color-accent: #7c3aed;
    --font-family: Inter, system-ui, sans-serif;
    --font-family-heading: Inter, system-ui, sans-serif;
    --spacing-unit: 1rem;
    --border-radius: 8px;
    --radius-sm: 0.25rem;
    --radius-md: 0.5rem;
    --radius-lg: 1rem;
    --shadow-sm: 0 1px 2px rgba(0,0,0,0.05);
    --shadow-md: 0 4px 6px rgba(0,0,0,0.07);
    --shadow-lg: 0 10px 15px rgba(0,0,0,0.10);
    --transition-duration: 300ms;
    --animation-easing: cubic-bezier(0.0, 0, 0.2, 1);
  }
  body {
    background: var(--color-bg);
    color: var(--color-text);
    font-family: var(--font-family);
    margin: 0;
  }
  /* All subsequent rules reference only var(--token-name) — no hardcoded values */
</style>
` + "```" + `

## Step 6 — Save and Render

Use the ` + "`session_dir`" + ` returned by ` + "`CreateDesignSession`" + ` (or the existing session's ` + "`session_dir`" + ` from ` + "`ListDesigns`" + `) for all file writes. This is also the ` + "`session_dir`" + ` argument for ` + "`RenderMockup`" + ` and ` + "`BundleMockup`" + `.

Save your two output files to ` + "`{session_dir}/`" + `:

1. ` + "`screens.jsx`" + ` — the tab strip, variation containers, tweaks panel, and screen content (uses starter globals)
2. ` + "`index.html`" + ` — shell with CDN scripts + CSS token block + starters + screens.jsx:

` + "```" + `html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Hi-Fi Mockup</title>
  <script src="https://unpkg.com/react@18/umd/react.development.js" crossorigin></script>
  <script src="https://unpkg.com/react-dom@18/umd/react-dom.development.js" crossorigin></script>
  <script src="https://unpkg.com/@babel/standalone/babel.min.js"></script>
  <style>
    /* CSS token block from Step 5 goes here */
    :root { /* ... token values ... */ }
    body { background: var(--color-bg); color: var(--color-text); font-family: var(--font-family); margin: 0; }
  </style>
</head>
<body>
  <div id="root"></div>
  <!-- Starters (local copies) -->
  <script type="text/babel" src="design-canvas.jsx"></script>
  <script type="text/babel" src="device-frames.jsx"></script>
  <script type="text/babel" src="ui-kit.jsx"></script>
  <script type="text/babel" src="chart-kit.jsx"></script>
  <!-- Your screens (written by AI) -->
  <script type="text/babel" src="screens.jsx"></script>
</body>
</html>
` + "```" + `

Then:
3. Call ` + "`RenderMockup`" + ` with ` + "`html_path`" + ` = path to ` + "`index.html`" + ` and ` + "`session_dir`" + ` = DESIGNS_BASE
4. Call ` + "`VerifyMockup`" + ` with the screenshot path returned by RenderMockup and ` + "`threshold: 75`" + `

If ` + "`pass: false`" + `:
- Read the ` + "`issues`" + ` list
- Fix each blocking issue in ` + "`screens.jsx`" + `
- Re-render and re-verify (pass ` + "`session_dir`" + ` = DESIGNS_BASE each time)
- Repeat up to 3 cycles; after 3, report remaining issues and stop

If ` + "`pass: true`" + `: proceed to Step 7.

## Step 7 — Bundle

Call ` + "`BundleMockup`" + ` with:
- ` + "`entry_html`" + `: path to ` + "`index.html`" + `
- ` + "`session_dir`" + `: DESIGNS_BASE
- ` + "`embed_cdn`" + `: true

The tool writes ` + "`{DESIGNS_BASE}/bundle/mockup.html`" + ` and pushes a clickable link to the chat. Do NOT show the raw file path — only show the URL returned by the tool.

## Step 8 — Report

Tell the user:
- The bundle URL (from BundleMockup tool output — show this, not the file path)
- Variation names and their design directions
- VerifyMockup score
- Any unfixed issues`

var wireframeSkillContent = `You are generating fast lo-fi grayscale wireframes for early-stage ideation. Follow this workflow exactly.

## Brief
$ARGUMENTS

## Design Session Management

**Existing design (iterate/update):** Call ` + "`ListDesigns`" + ` first. If a session exists for this project, use its ` + "`session_dir`" + ` as your working directory for all file writes.

**New design:** Call ` + "`CreateDesignSession`" + ` with an optional ` + "`name`" + ` (e.g. ` + "`\"dashboard-wireframe\"`" + `). Use the returned ` + "`session_dir`" + ` for ALL file writes. Never create a ` + "`designs/`" + ` folder in the project root.

> The tool creates the session dir and copies all starter JSX files into it automatically.

Your ` + "`screens.jsx`" + ` file MUST use these exports:
- **design-canvas.jsx**: ` + "`DesignCanvas`" + `, ` + "`DCSection`" + `, ` + "`DCArtboard`" + ` — canvas for artboard layout
- **device-frames.jsx**: ` + "`IOSDevice`" + `, ` + "`AndroidDevice`" + `, ` + "`DesktopBrowser`" + ` — device frames for wireframes

## Step 1 — Clarification

Ask the user ONE question only:
**What screens or flows should the wireframe show, and what user actions should each screen demonstrate?**

Example response: "Home screen with search, product listing, and checkout flow with 3 screens: list view, product detail, and cart review."

Do NOT ask about brand, colors, typography, or visual polish — wireframes are intentionally unstyled.

If the user has already answered this, proceed immediately to Step 2.

## Step 2 — Generate screens.jsx

Write ` + "`screens.jsx`" + ` — a React file with all wireframe screens. This file uses the starter globals loaded by ` + "`index.html`" + `.

At the top of ` + "`screens.jsx`" + `, destructure what you need:
` + "```" + `jsx
const { DesignCanvas, DCSection, DCArtboard } = window;
const { IOSDevice, AndroidDevice, DesktopBrowser } = window;
` + "```" + `

Then write your wireframe screen components using ` + "`DesignCanvas`" + ` / ` + "`DCArtboard`" + ` for layout and ` + "`IOSDevice`" + ` / ` + "`DesktopBrowser`" + ` to wrap mobile/desktop frames.

### Wireframe style rules

Keep the intentionally lo-fi, grayscale aesthetic inside each artboard:

### Structure
- **Vertical scroll layout** — all screens stacked top-to-bottom, no tabs or JS navigation needed
- **Grayscale only** — use only: #000, #333, #666, #999, #ccc, #eee, #fff. No Tailwind colors, no design tokens, no color classes
- **Boxes and placeholders** — use grey rectangles with centered ` + "`×`" + ` for images (no actual images)
- **Labels and annotations** — text labels on interactive elements, brief descriptions
- **Rough on purpose** — lo-fi aesthetic: simple borders, basic layout, no shadows or fancy effects

### Wireframe content structure per screen

` + "```html" + `
<div class="screen" style="border-top:2px solid #000;padding:2rem;margin:2rem 0;">
  <h2 style="font-family:Arial,sans-serif;color:#000;margin:0 0 1rem 0;">Screen Name</h2>
  
  <!-- Interactive element with annotation marker -->
  <button style="padding:0.75rem 1rem;background:#ccc;border:1px solid #000;cursor:pointer;font-family:Arial,sans-serif;color:#000;">
    Button ①
  </button>
  
  <!-- Image placeholder -->
  <div style="background:#eee;border:1px solid #999;width:200px;height:150px;display:flex;align-items:center;justify-content:center;font-family:Arial,sans-serif;color:#999;font-size:2rem;">×</div>
  
  <!-- Form example -->
  <input type="text" placeholder="Input field" style="padding:0.5rem;border:1px solid #999;font-family:Arial,sans-serif;display:block;margin:1rem 0;width:300px;">
  
  <!-- Annotation legend table for this screen -->
  <table style="margin-top:1rem;border-collapse:collapse;font-family:Arial,sans-serif;font-size:0.9rem;color:#333;">
    <tr style="border-bottom:1px solid #ccc;">
      <td style="padding:0.5rem;text-align:left;width:40px;"><strong>①</strong></td>
      <td style="padding:0.5rem;">Primary action button — triggers checkout flow</td>
    </tr>
  </table>
</div>
` + "```" + `

### Annotation markers

Use numbered markers (①②③ etc.) on interactive elements. Below each screen, add a legend table mapping each marker to a brief UX note:

` + "```html" + `
<table style="margin-top:1rem;border-collapse:collapse;font-family:Arial,sans-serif;font-size:0.9rem;">
  <tr style="border-bottom:1px solid #ccc;">
    <td style="padding:0.5rem;width:30px;"><strong>①</strong></td>
    <td style="padding:0.5rem;">Search bar — suggests products as user types</td>
  </tr>
  <tr style="border-bottom:1px solid #ccc;">
    <td style="padding:0.5rem;"><strong>②</strong></td>
    <td style="padding:0.5rem;">Product card — tappable to view detail</td>
  </tr>
  <tr style="border-bottom:1px solid #ccc;">
    <td style="padding:0.5rem;"><strong>③</strong></td>
    <td style="padding:0.5rem;">Add to cart — updates cart count in header</td>
  </tr>
</table>
` + "```" + `

### Optional: Flow diagram

If the user described a multi-screen flow, add a simple ASCII or SVG storyboard at the top showing the sequence:

` + "```html" + `
<div style="padding:2rem;border-bottom:2px solid #000;font-family:Arial,sans-serif;">
  <h3 style="margin:0 0 1rem 0;color:#000;">Flow: Home → Product → Cart → Review</h3>
  <!-- Optional: dashed SVG arrows between screen names -->
</div>
` + "```" + `

## Step 3 — Output

Use the ` + "`session_dir`" + ` returned by ` + "`CreateDesignSession`" + ` (or the existing session's ` + "`session_dir`" + ` from ` + "`ListDesigns`" + `) for all file writes.

Save your two output files to ` + "`{session_dir}/`" + `:

1. ` + "`screens.jsx`" + ` — all wireframe screen components (uses ` + "`DesignCanvas`" + `, ` + "`DCArtboard`" + `, device frames)
2. ` + "`index.html`" + ` — shell with CDN + starters + screens.jsx:

` + "```" + `html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Wireframe</title>
  <script src="https://unpkg.com/react@18/umd/react.development.js" crossorigin></script>
  <script src="https://unpkg.com/react-dom@18/umd/react-dom.development.js" crossorigin></script>
  <script src="https://unpkg.com/@babel/standalone/babel.min.js"></script>
</head>
<body style="margin:0;background:#fff;">
  <div id="root"></div>
  <!-- Starters (local copies) -->
  <script type="text/babel" src="design-canvas.jsx"></script>
  <script type="text/babel" src="device-frames.jsx"></script>
  <!-- Your screens (written by AI) -->
  <script type="text/babel" src="screens.jsx"></script>
</body>
</html>
` + "```" + `

Then call ` + "`BundleMockup`" + ` with:
- ` + "`entry_html`" + `: path to ` + "`index.html`" + `
- ` + "`session_dir`" + `: DESIGNS_BASE
- ` + "`embed_cdn`" + `: true

The tool writes ` + "`{DESIGNS_BASE}/bundle/mockup.html`" + ` and pushes a clickable link. Do NOT specify ` + "`output_path`" + ` — let ` + "`session_dir`" + ` control it.

**Do NOT call RenderMockup or VerifyMockup** — wireframes skip the render/verify loop entirely for speed.

## Step 4 — Report

Tell the user:
- Bundle path
- Screens covered in the wireframe
- Key flows demonstrated
- Ask: "Which screen should we explore further, or shall we move to hi-fi with /hifi?"

Done. Wireframe is intentionally rough — move fast.`

var prototypeSkillContent = `You are generating stateful interactive prototypes with multi-screen flows. Follow this workflow exactly.

## Brief
$ARGUMENTS

## Junior Designer Workflow: Assumption → Reasoning → Placeholder → Iterate

Do not generate a complete prototype from a blank slate. Use this structured approach on every task:

### Step 0-A · Declare Assumptions (before writing any code)

At the very top of the first file you write, add an HTML comment block:

` + "```html" + `
<!--
ASSUMPTIONS
===========
User behavior:
  - Assumed users arrive via [entry point] and want to [primary goal]
  - Assumed [X] is the most common task; [Y] is secondary

Content:
  - [Field name] will contain [estimated character count / data format]
  - [Image slot] will be [dimensions / content type]

Interaction model:
  - Assumed [gesture/click/keyboard] triggers [action]
  - Assumed [state A] always precedes [state B]

Open questions for user:
  - [ ] Is [assumption X] correct?
  - [ ] What happens when [edge case Y]?
-->
` + "```" + `

### Step 0-B · Reasoning (explain each flow decision inline)

For each screen transition or interaction decision, add a brief comment explaining WHY this flow makes sense:

` + "```jsx" + `
// REASONING: Login → Dashboard direct flow because most users return daily
// and don't need onboarding after first session. Onboarding only fires on
// account.isNew === true.
` + "```" + `

### Step 0-C · Placeholder Discipline

Use explicit, honest placeholders — never invented data:

| Instead of | Use |
|---|---|
| Fake user names | ` + "`[User Name]`" + ` or ` + "`<!-- await: real user data -->`" + ` |
| Made-up statistics | ` + "`[Metric: source pending]`" + ` |
| Generic product images | Gray block + ` + "`\"Product image · 400×300 · pending\"`" + ` |
| Lorem ipsum | ` + "`[Copy: headline for feature X]`" + ` |
| Random numbers in charts | Clearly labeled ` + "`/* mock data — replace with API */`" + ` |

Placeholder rule: a labeled gray block is better than a confident fake.

### Step 0-D · Iterate — Flag What Needs Real Data

At the end of the file, add an iteration checklist:

` + "```html" + `
<!--
ITERATION CHECKLIST
===================
Replace before handoff:
  [ ] [Placeholder A] — needs: <what real data/asset is required>
  [ ] [Placeholder B] — needs: <API endpoint / user content>

Confirm with user:
  [ ] [Flow decision X] — assumption logged above
  [ ] [Interaction Y] — needs UX validation

Deferred to production:
  [ ] [Animation Z] — needs real performance budget
-->
` + "```" + `

Show the assumptions + iteration checklist to the user early — before the prototype is complete. An incorrect assumption caught at step 0 costs nothing; caught after full implementation, it costs a rewrite.

## Design Session Management

**Existing design (iterate/update):** Call ` + "`ListDesigns`" + ` first. If a session exists for this project, use its ` + "`session_dir`" + ` as your working directory for all file writes.

**New design:** Call ` + "`CreateDesignSession`" + ` with an optional ` + "`name`" + ` (e.g. ` + "`\"dashboard-prototype\"`" + `). Use the returned ` + "`session_dir`" + ` for ALL file writes. Never create a ` + "`designs/`" + ` folder in the project root.

> The tool creates the session dir and copies all starter JSX files into it automatically.

Your ` + "`screens.jsx`" + ` file MUST use these exports:
- **stage.jsx**: ` + "`Stage`" + `, ` + "`Sprite`" + `, ` + "`useSprite`" + `, ` + "`Easing`" + `, ` + "`interpolate`" + `, ` + "`clamp`" + `, ` + "`PlaybackBar`" + ` — animation engine with timeline scrubber
- **device-frames.jsx**: ` + "`IOSDevice`" + `, ` + "`AndroidDevice`" + `, ` + "`DesktopBrowser`" + ` — wrap screens in device frames
- **motion.jsx**: ` + "`Particles`" + `, ` + "`Confetti`" + `, ` + "`useSpring`" + `, ` + "`CountUp`" + `, ` + "`TypeWriter`" + ` — animation primitives for rich motion

## Step 1 — Ask About the Flow

Use AskUserQuestion to ask:

- **"What user flows should the prototype cover?"**
  Example: "login → dashboard → settings" or "onboarding → create project → invite team"

- **"What is the entry point screen?"**
  Example: "login" or "home page"

- **"What interactions should be demonstrated?"**
  Checklist options: "Form submission", "Modal/drawer open/close", "Loading states", "Navigation", "Tab switching", "Accordion/collapse", "Dropdown menu", "Error states"

## Step 2 — Load or Define Design Tokens

1. Call ` + "`ListDesigns`" + ` to discover existing sessions. If a ` + "`design-system.json`" + ` file exists inside any session dir, read it with the Read tool. Extract:
   - Color palette (primary, secondary, accent, background, text, border, error, success, warning)
   - Typography (font family, font sizes for body/heading, line heights, weights)
   - Spacing scale (base unit, then multiples: 8px, 16px, 24px, 32px, etc.)
   - Border radii (sm, md, lg)
   - Shadows (sm, md, lg)
   - Transitions (duration, easing)

3. If not exists: Use inline defaults:
` + "```json" + `
{
  "colors": {
    "primary": "#3b82f6",
    "secondary": "#10b981",
    "accent": "#7c3aed",
    "background": "#ffffff",
    "surface": "#f9fafb",
    "text": "#1f2937",
    "textMuted": "#6b7280",
    "border": "#e5e7eb",
    "error": "#ef4444",
    "success": "#10b981",
    "warning": "#f59e0b"
  },
  "typography": {
    "fontFamily": "Inter, system-ui, sans-serif",
    "fontSize": { "sm": "12px", "base": "14px", "lg": "16px", "xl": "20px", "2xl": "24px" },
    "fontWeight": { "normal": 400, "medium": 500, "semibold": 600, "bold": 700 },
    "lineHeight": { "tight": "1.25", "normal": "1.5", "relaxed": "1.75" }
  },
  "spacing": { "xs": "4px", "sm": "8px", "md": "16px", "lg": "24px", "xl": "32px", "2xl": "48px" },
  "radius": { "sm": "4px", "md": "8px", "lg": "12px" },
  "shadow": {
    "sm": "0 1px 2px rgba(0,0,0,0.05)",
    "md": "0 4px 6px rgba(0,0,0,0.10)",
    "lg": "0 10px 15px rgba(0,0,0,0.15)"
  },
  "transition": { "duration": "300ms", "easing": "cubic-bezier(0.4, 0, 0.2, 1)" }
}
` + "```" + `

## Step 3 — Design the Screen Map

Create a simple structure:
` + "```js" + `
const screens = {
  "login": {
    title: "Login",
    next: "dashboard",
    actions: { submit: "dashboard" }
  },
  "dashboard": {
    title: "Dashboard",
    previous: "login",
    next: "settings",
    actions: { settings: "settings", logout: "login" }
  },
  "settings": {
    title: "Settings",
    previous: "dashboard",
    actions: { save: "dashboard" }
  }
};
` + "```" + `

Map each screen with:
- ` + "`title`" + ` — display name
- ` + "`previous`" + ` — screen to go back to (for breadcrumb/back button)
- ` + "`next`" + ` — default forward flow (for primary button)
- ` + "`actions`" + ` — { actionLabel: targetScreen } map (forms, nav, etc.)

## Step 4 — Generate screens.jsx

Write ` + "`screens.jsx`" + ` — the React app with all screens and interactions. This file uses the starter globals loaded by ` + "`index.html`" + ` (stage.jsx for animation, device-frames.jsx for device chrome, motion.jsx for effects).

At the top of ` + "`screens.jsx`" + `, destructure what you need:
` + "```" + `jsx
const { Stage, Sprite, useSprite, Easing, interpolate, clamp, PlaybackBar } = window;
const { IOSDevice, AndroidDevice, DesktopBrowser } = window;
const { Particles, Confetti, useSpring, CountUp, TypeWriter } = window;
` + "```" + `

Then write your screen components using these primitives — do NOT redefine them. Use ` + "`Stage`" + ` + ` + "`Sprite`" + ` for animated flows, ` + "`IOSDevice`" + ` / ` + "`DesktopBrowser`" + ` to wrap screens in device frames, ` + "`useSpring`" + ` / ` + "`CountUp`" + ` for rich motion.

The ` + "`screens.jsx`" + ` React app structure:

` + "```html" + `
<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Interactive Prototype</title>
  <script crossorigin src="https://unpkg.com/react@18/umd/react.production.min.js"></script>
  <script crossorigin src="https://unpkg.com/react-dom@18/umd/react-dom.production.min.js"></script>
  <script src="https://unpkg.com/@babel/standalone/babel.min.js"></script>
  <style>
    :root {
      --color-primary: #3b82f6;
      --color-bg: #ffffff;
      --color-text: #1f2937;
      --spacing-md: 16px;
      --radius-md: 8px;
      --transition: 300ms cubic-bezier(0.4, 0, 0.2, 1);
    }
    * { box-sizing: border-box; }
    body { margin: 0; font-family: var(--font-family, system-ui); background: var(--color-bg); }
    
    .app { width: 100%; height: 100vh; display: flex; overflow: hidden; }
    .viewport { flex: 1; position: relative; overflow: hidden; }
    .screen {
      position: absolute;
      inset: 0;
      opacity: 0;
      pointer-events: none;
      transition: opacity var(--transition), transform var(--transition);
    }
    .screen.active { opacity: 1; pointer-events: auto; }
    .screen.exit-left { transform: translateX(-100%); }
    .screen.exit-right { transform: translateX(100%); }
    .screen.enter-left { opacity: 1; transform: translateX(0); }
    .screen.enter-right { opacity: 1; transform: translateX(0); }
    
    .screen__content { padding: var(--spacing-md); max-width: 600px; margin: 0 auto; }
    .breadcrumb { display: flex; gap: 8px; margin-bottom: 16px; font-size: 12px; color: var(--color-textMuted, #6b7280); }
    .breadcrumb a { cursor: pointer; color: var(--color-primary); text-decoration: none; }
    .breadcrumb a:hover { text-decoration: underline; }
    
    .form { display: flex; flex-direction: column; gap: 12px; }
    .form-group { display: flex; flex-direction: column; gap: 4px; }
    .form-group label { font-weight: 500; font-size: 14px; }
    .form-group input { padding: 8px 12px; border: 1px solid var(--color-border, #e5e7eb); border-radius: var(--radius-md); }
    
    .button {
      padding: 10px 16px;
      background: var(--color-primary);
      color: white;
      border: none;
      border-radius: var(--radius-md);
      cursor: pointer;
      font-size: 14px;
      transition: background var(--transition);
    }
    .button:hover { background: #2563eb; }
    .button:disabled { background: #d1d5db; cursor: not-allowed; }
    
    .modal-overlay { position: fixed; inset: 0; background: rgba(0, 0, 0, 0.5); display: flex; align-items: center; justify-content: center; opacity: 0; pointer-events: none; transition: opacity var(--transition); }
    .modal-overlay.open { opacity: 1; pointer-events: auto; }
    .modal { background: white; border-radius: var(--radius-md); padding: 24px; max-width: 400px; width: 90%; }
    
    .spinner { width: 40px; height: 40px; border: 3px solid #e5e7eb; border-top-color: var(--color-primary); border-radius: 50%; animation: spin 0.8s linear infinite; }
    @keyframes spin { to { transform: rotate(360deg); } }
    .loading { display: flex; justify-content: center; align-items: center; height: 200px; }
    
    .hotspot { position: relative; }
    [data-hotspot] { outline: none; }
    body.hotspot-mode [data-hotspot]::after {
      content: attr(data-hotspot-label);
      position: absolute;
      top: -5px;
      left: 50%;
      transform: translateX(-50%) translateY(-100%);
      background: #3b82f6;
      color: white;
      padding: 4px 8px;
      border-radius: 4px;
      font-size: 11px;
      white-space: nowrap;
      z-index: 1000;
      pointer-events: none;
      box-shadow: 0 2px 8px rgba(0,0,0,0.2);
      animation: pulse-outline 1.5s ease-in-out infinite;
    }
    body.hotspot-mode [data-hotspot] {
      outline: 2px dashed #3b82f6;
      outline-offset: 2px;
      animation: pulse-glow 1.5s ease-in-out infinite;
    }
    @keyframes pulse-glow {
      0%, 100% { outline-color: #3b82f6; }
      50% { outline-color: #60a5fa; }
    }
    @keyframes pulse-outline {
      0%, 100% { opacity: 1; }
      50% { opacity: 0.7; }
    }
    
    .hotspot-toggle {
      position: fixed;
      top: 16px;
      right: 16px;
      z-index: 999;
      padding: 8px 12px;
      background: #f3f4f6;
      border: 1px solid #d1d5db;
      border-radius: var(--radius-md);
      cursor: pointer;
      font-size: 12px;
      font-weight: 500;
      transition: background var(--transition);
    }
    .hotspot-toggle:hover { background: #e5e7eb; }
    .hotspot-toggle.active { background: var(--color-primary); color: white; border-color: var(--color-primary); }
  </style>
</head>
<body>
  <div id="root"></div>
  
  <script type="text/babel">
    const { useState } = React;
    
    const screens = {
      /* FILLED FROM STEP 3 ABOVE */
    };
    
    function App() {
      const [currentScreen, setCurrentScreen] = useState('LOGIN_SCREEN');
      const [isLoading, setIsLoading] = useState(false);
      const [formData, setFormData] = useState({});
      const [openModals, setOpenModals] = useState({});
      const [hotspotMode, setHotspotMode] = useState(false);
      
      /* Screen render functions — one per screen */
      const renderScreen = () => {
        switch(currentScreen) {
          case 'login':
            return (
              <div className="screen__content">
                <h1>Login</h1>
                <div className="form">
                  <div className="form-group">
                    <label>Email</label>
                    <input type="email" value={formData.email || ''} onChange={(e) => setFormData({...formData, email: e.target.value})} />
                  </div>
                  <div className="form-group">
                    <label>Password</label>
                    <input type="password" value={formData.password || ''} onChange={(e) => setFormData({...formData, password: e.target.value})} />
                  </div>
                  <button className="button" data-hotspot="true" data-hotspot-label="Sign In" onClick={() => { setIsLoading(true); setTimeout(() => { setCurrentScreen('dashboard'); setIsLoading(false); }, 1000); }} disabled={isLoading}>
                    {isLoading ? <span className="spinner"></span> : 'Sign In'}
                  </button>
                </div>
              </div>
            );
          case 'dashboard':
            return (
              <div className="screen__content">
                <h1>Dashboard</h1>
                <p>Welcome! This is the main dashboard.</p>
                <button className="button" data-hotspot="true" data-hotspot-label="Go to Settings" onClick={() => setCurrentScreen('settings')}>Settings</button>
                <button className="button" data-hotspot="true" data-hotspot-label="Logout" onClick={() => setCurrentScreen('login')} style={{marginLeft: '8px', background: '#6b7280'}}>Logout</button>
              </div>
            );
          case 'settings':
            return (
              <div className="screen__content">
                <h1>Settings</h1>
                <div className="form">
                  <div className="form-group">
                    <label>Display Name</label>
                    <input type="text" value={formData.name || ''} onChange={(e) => setFormData({...formData, name: e.target.value})} />
                  </div>
                  <button className="button" data-hotspot="true" data-hotspot-label="Save Changes" onClick={() => { setIsLoading(true); setTimeout(() => { setCurrentScreen('dashboard'); setIsLoading(false); }, 800); }} disabled={isLoading}>
                    {isLoading ? <span className="spinner"></span> : 'Save'}
                  </button>
                  <button className="button" data-hotspot="true" data-hotspot-label="Back" onClick={() => setCurrentScreen('dashboard')} style={{background: '#9ca3af'}}>Back</button>
                </div>
              </div>
            );
          default:
            return <div>Unknown screen</div>;
        }
      };
      
      return (
        <div className="app">
          <button className={` + "\"" + `hotspot-toggle ${hotspotMode ? 'active' : ''}` + "\"" + `} onClick={() => setHotspotMode(!hotspotMode)}>🔍 Hotspots {hotspotMode ? 'ON' : 'OFF'}</button>
          <div className="viewport" style={{body: hotspotMode ? 'hotspot-mode' : ''}}>
            <div className={` + "\"" + `screen active` + "\"" + `}>
              <div className="breadcrumb">
                {screens[currentScreen]?.previous && <a onClick={() => setCurrentScreen(screens[currentScreen].previous)}>← Back</a>}
                <span>/ {currentScreen.replace(/_/g, ' ')}</span>
              </div>
              {isLoading && <div className="loading"><div className="spinner"></div></div>}
              {!isLoading && renderScreen()}
            </div>
          </div>
        </div>
      );
    }
    
    const root = ReactDOM.createRoot(document.getElementById('root'));
    root.render(<App />);
  </script>
</body>
</html>
` + "```" + `

**Key patterns:**

- ` + "`currentScreen`" + ` state — tracks visible screen by name (e.g., "login", "dashboard")
- ` + "`setCurrentScreen(name)`" + ` — navigate to a screen
- ` + "`isLoading`" + ` state — shows spinner overlay + disables buttons during async (simulated with setTimeout)
- ` + "`formData`" + ` state — holds form input values across navigation
- ` + "`openModals`" + ` state — { modalName: boolean } for modal visibility
- ` + "`hotspotMode`" + ` state — toggles body.hotspot-mode class
- ` + "`[data-hotspot]`" + ` attributes on clickable elements for hotspot labeling
- CSS transitions (slide/fade) applied via classes ` + "`exit-left`" + `, ` + "`enter-right`" + `, etc.
- Breadcrumb rendered dynamically from screens map

## Step 5 — Add Hotspot Mode Implementation

In the ` + "`<style>`" + ` section (already above), the CSS handles hotspot display:

- ` + "`body.hotspot-mode [data-hotspot]::after`" + ` — shows label with pulsing animation
- ` + "`body.hotspot-mode [data-hotspot]`" + ` — pulsing blue outline + dashed outline
- Hotspot toggle button (top-right) — toggles the mode

In the ` + "`<body>`" + ` tag after the app mounts, ensure the class is synced:
` + "```js" + `
useEffect(() => {
  if (hotspotMode) {
    document.body.classList.add('hotspot-mode');
  } else {
    document.body.classList.remove('hotspot-mode');
  }
}, [hotspotMode]);
` + "```" + `

## Step 6 — Save to Disk

Use the ` + "`session_dir`" + ` returned by ` + "`CreateDesignSession`" + ` (or the existing session's ` + "`session_dir`" + ` from ` + "`ListDesigns`" + `) for all file writes. This is also the ` + "`session_dir`" + ` argument for ` + "`RenderMockup`" + ` and ` + "`BundleMockup`" + `.

Save your two output files to ` + "`{session_dir}/`" + `:

1. ` + "`screens.jsx`" + ` — the React app with all screens and interactions (uses starter globals)
2. ` + "`index.html`" + ` — shell with CDN + starters + screens.jsx:

` + "```" + `html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Interactive Prototype</title>
  <script src="https://unpkg.com/react@18/umd/react.development.js" crossorigin></script>
  <script src="https://unpkg.com/react-dom@18/umd/react-dom.development.js" crossorigin></script>
  <script src="https://unpkg.com/@babel/standalone/babel.min.js"></script>
  <style>
    :root {
      --color-primary: #3b82f6;
      --color-bg: #ffffff;
      --color-text: #1f2937;
      --spacing-md: 16px;
      --radius-md: 8px;
      --transition: 300ms cubic-bezier(0.4, 0, 0.2, 1);
    }
    * { box-sizing: border-box; }
    body { margin: 0; font-family: var(--font-family, system-ui); background: var(--color-bg); }
  </style>
</head>
<body>
  <div id="root"></div>
  <!-- Starters (local copies) -->
  <script type="text/babel" src="stage.jsx"></script>
  <script type="text/babel" src="device-frames.jsx"></script>
  <script type="text/babel" src="motion.jsx"></script>
  <!-- Your screens (written by AI) -->
  <script type="text/babel" src="screens.jsx"></script>
</body>
</html>
` + "```" + `

## Step 7 — Render and Verify

1. Call ` + "`RenderMockup`" + ` with ` + "`html_path`" + ` = path to ` + "`index.html`" + ` and ` + "`session_dir`" + ` = DESIGNS_BASE
2. Call ` + "`VerifyMockup`" + ` with the screenshot path and ` + "`threshold: 75`" + `

If ` + "`pass: false`" + `:
- Read the ` + "`issues`" + ` list
- Fix each blocking issue in ` + "`screens.jsx`" + `
- Re-render and re-verify (pass ` + "`session_dir`" + ` = DESIGNS_BASE each time)
- Repeat up to 3 cycles; after 3, report remaining issues and stop

If ` + "`pass: true`" + `: proceed to Step 8.

## Step 8 — Bundle

Call ` + "`BundleMockup`" + ` with:
- ` + "`entry_html`" + `: path to ` + "`index.html`" + `
- ` + "`session_dir`" + `: DESIGNS_BASE
- ` + "`embed_cdn`" + `: true

The tool writes ` + "`{DESIGNS_BASE}/bundle/mockup.html`" + ` and pushes a clickable link to the chat. Do NOT show the raw file path — only show the URL returned by the tool.

## Step 9 — Report

Tell the user:
- The bundle URL (from BundleMockup tool output — show this, not the file path)
- Screens covered in the flow
- Interactions demonstrated (forms, loading states, navigation, etc.)
- Hotspot mode usage note
- VerifyMockup score
- Any unfixed issues

Then suggest: ` + "`Use /handoff to generate a developer spec from this prototype.`" + `
`

var designDirectionAdvisorSkillContent = `You are a design direction advisor. When the user's brief is vague or lacks specific visual direction, you generate 3 differentiated design directions as live HTML demos. The user picks one direction before you proceed to full hifi/prototype execution.

## Brief
$ARGUMENTS

## When to Use This Skill

**Trigger conditions (any one is sufficient):**
- Brief is vague ("make it look good", "help me design", "something nice")
- No design reference provided (no Figma, no screenshot, no brand spec)
- User explicitly asks for options ("give me directions", "show me styles", "what would work?")
- Project has no design context (no existing design system, no reference)

**Skip and hand off to /hifi if:**
- User has given a specific visual reference (Figma, screenshot, brand guidelines)
- User has stated a clear style ("Apple-style", "Bloomberg editorial feel")
- Task is a small edit or iteration on existing design

## Step 1 — Understand the Brief

Before generating directions, ask at most 3 questions if the brief is truly empty:
1. Who is the target audience? (e.g. enterprise users, consumers, developers)
2. What is the core purpose? (e.g. dashboard, landing page, app)
3. What emotional tone should it have? (e.g. trustworthy, playful, cutting-edge)

If any of these can be inferred from context, infer — do not ask.

## Step 2 — Restate the Brief (100-150 words)

Summarize the essential design problem in your own words: audience, purpose, emotional tone, and key constraints. End with: "Based on this understanding, here are 3 design directions."

## Step 3 — Select 3 Directions from Different Philosophies

Choose 3 directions that come from different philosophical families. The 20 available philosophies, grouped:

**Information Architecture Family (01-04):** Pentagram (Michael Bierut) · Stamen Design · Information Architects · Fathom Information Design
**Motion Poetry Family (05-08):** Locomotive · Active Theory · Field.io · Resn
**Minimalism Family (09-12):** Experimental Jetset · Müller-Brockmann · Build Studio · Sagmeister & Walsh
**Avant-Garde Family (13-16):** Zach Lieberman · Raven Kwok · Ash Thorp · Territory Studio
**Eastern Philosophy Family (17-20):** Takram · Kenya Hara · Irma Boom · Neo Shen

**Mandatory rule:** All 3 directions must come from different families. No two directions from the same family.

**Recommended combinations by project type:**

| Project Type | Suggested Direction Mix |
|---|---|
| SaaS / Dashboard | Information Architecture + Minimalism + Eastern Philosophy |
| Creative Agency / Portfolio | Motion Poetry + Avant-Garde + Minimalism |
| Consumer App | Minimalism + Eastern Philosophy + Information Architecture |
| Data-Heavy Product | Information Architecture + Minimalism + Motion Poetry |
| Developer Tool | Information Architecture + Minimalism + Avant-Garde |

For each direction, state:
- **Name:** "[Philosopher/Studio] — [2-word tagline]" (e.g. "Kenya Hara — Radical Emptiness")
- **Rationale:** 50-80 words explaining why this philosophy fits the user's specific brief
- **Visual signatures:** 3-4 bullet points (color approach, typography style, layout density, one signature element)
- **Emotional keywords:** 3-5 adjectives

## Step 4 — Generate 3 Live HTML Demos in One File

Produce a single self-contained HTML file with all 3 direction demos displayed side by side (or stacked on mobile). Each demo is a 600×400px card mockup of the actual product — not an abstract style board.

The HTML file structure:

` + "```html" + `
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Design Directions — [Product Name]</title>
  <style>
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body { font-family: system-ui, sans-serif; background: #f5f5f5; padding: 2rem; }
    h1 { text-align: center; margin-bottom: 0.5rem; font-size: 1.25rem; color: #111; }
    p.subtitle { text-align: center; color: #666; font-size: 0.875rem; margin-bottom: 2rem; }
    .directions { display: grid; grid-template-columns: repeat(auto-fit, minmax(600px, 1fr)); gap: 2rem; }
    .direction { background: white; border-radius: 12px; overflow: hidden; box-shadow: 0 4px 24px rgba(0,0,0,0.08); }
    .direction-header { padding: 1rem 1.5rem; border-bottom: 1px solid #e5e5e5; }
    .direction-name { font-weight: 600; font-size: 1rem; color: #111; margin-bottom: 0.25rem; }
    .direction-rationale { font-size: 0.8rem; color: #666; line-height: 1.5; }
    .demo-frame { width: 100%; height: 400px; position: relative; overflow: hidden; }
    .direction-footer { padding: 1rem 1.5rem; border-top: 1px solid #e5e5e5; display: flex; align-items: center; justify-content: space-between; }
    .keywords { display: flex; gap: 0.5rem; flex-wrap: wrap; }
    .keyword { background: #f3f4f6; padding: 0.2rem 0.6rem; border-radius: 999px; font-size: 0.75rem; color: #555; }
    .pick-btn { padding: 0.5rem 1.25rem; border: none; border-radius: 8px; font-size: 0.875rem; font-weight: 600; cursor: pointer; transition: opacity 0.2s; }
    .pick-btn:hover { opacity: 0.85; }
    .picked { outline: 3px solid #2563eb; outline-offset: 3px; }
    .confirmation { display: none; text-align: center; margin-top: 2rem; padding: 1.5rem; background: #eff6ff; border-radius: 12px; border: 1px solid #bfdbfe; }
    .confirmation.show { display: block; }
    .confirmation strong { color: #1d4ed8; font-size: 1.1rem; }
    .confirmation p { color: #3730a3; font-size: 0.875rem; margin-top: 0.5rem; }
  </style>
</head>
<body>
  <h1>Design Directions — [Product Name]</h1>
  <p class="subtitle">Pick the direction that resonates. You can also mix: "A's color + C's layout".</p>
  <div class="directions">

    <!-- Direction A -->
    <div class="direction" id="dir-a">
      <div class="direction-header">
        <div class="direction-name">A · [Philosophy Name] — [Tagline]</div>
        <div class="direction-rationale">[50-80 word rationale for why this fits the brief]</div>
      </div>
      <div class="demo-frame">
        <!-- FULL SELF-CONTAINED MOCKUP FOR DIRECTION A -->
        <!-- Use inline styles only. No external dependencies. -->
        <!-- Must look like a real product UI, not an abstract style board. -->
        <!-- Minimum: header/nav + one content section + one interactive element -->
      </div>
      <div class="direction-footer">
        <div class="keywords">
          <span class="keyword">[adjective 1]</span>
          <span class="keyword">[adjective 2]</span>
          <span class="keyword">[adjective 3]</span>
        </div>
        <button class="pick-btn" style="background:[direction-A-primary];color:[on-primary];" onclick="pickDirection('a', 'A · [Name]')">Pick this direction →</button>
      </div>
    </div>

    <!-- Direction B -->
    <div class="direction" id="dir-b">
      <!-- same structure as Direction A, different content -->
    </div>

    <!-- Direction C -->
    <div class="direction" id="dir-c">
      <!-- same structure as Direction A, different content -->
    </div>

  </div>

  <div class="confirmation" id="confirmation">
    <strong>Direction <span id="chosen-label"></span> selected</strong>
    <p>Tell me to proceed with <code>/hifi</code> or <code>/prototype</code> and I will execute in this direction.</p>
  </div>

  <script>
    var current = null;
    function pickDirection(id, label) {
      if (current) document.getElementById('dir-' + current).classList.remove('picked');
      current = id;
      document.getElementById('dir-' + id).classList.add('picked');
      document.getElementById('chosen-label').textContent = label;
      document.getElementById('confirmation').classList.add('show');
      document.getElementById('confirmation').scrollIntoView({ behavior: 'smooth', block: 'nearest' });
    }
  </script>
</body>
</html>
` + "```" + `

### Rules for the demo frames

Each 600×400px demo frame must:
- Show the actual product context (not an abstract color swatch)
- Use the philosophy's actual color palette, typography weight, and layout density
- Include at least: a header/nav, a headline, one content element, one CTA or interactive element
- Use only inline styles — no external CSS, no CDN fonts (use system font stacks that match the direction's feel)
- Be visually distinct from the other two directions — a user should be able to tell them apart at a glance

Typography system fonts by direction family:
- Information Architecture: ` + "`'Helvetica Neue', Arial, sans-serif`" + ` (tight tracking, heavy weight hierarchy)
- Motion Poetry: ` + "`system-ui, sans-serif`" + ` (large scale contrast, display vs body split)
- Minimalism: ` + "`Georgia, serif`" + ` or ` + "`'Gill Sans', sans-serif`" + ` (controlled rhythm)
- Avant-Garde: ` + "`'Courier New', monospace`" + ` or ` + "`system-ui`" + ` with extreme weight contrast
- Eastern Philosophy: ` + "`'Palatino', serif`" + ` or ` + "`system-ui`" + ` with generous whitespace and light weights

## Step 5 — Save and Present

1. Write the HTML file to a local path (e.g. ` + "`_temp/design-directions.html`" + `)
2. Present the 3 directions with their names and 2-sentence rationale in the chat
3. Tell the user: "Open the HTML file to see interactive demos. Click 'Pick this direction →' to select, then tell me to proceed with /hifi or /prototype."

## Step 6 — User Picks → Hand Off

When user picks a direction (or describes a mix), respond:

1. Confirm the chosen direction and its visual signatures
2. State the assumptions you're carrying forward (typography, palette, layout density)
3. Ask if they want: (a) proceed to /hifi for static mockup, or (b) proceed to /prototype for interactive flow
4. Execute the chosen skill with the design direction as context

If user wants to mix directions (e.g. "A's color + C's layout"), create a merged direction brief before handing off.`
