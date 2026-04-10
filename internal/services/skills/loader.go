package skills

import (
	"os"
	"path/filepath"
	"sort"
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
	SkillDir    string `json:"skill_dir,omitempty"` // directory containing the skill file; empty for flat .md files
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
			for _, fname := range []string{"SKILL.md", "skill.md", "index.md", "README.md"} {
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
						SkillDir:    filepath.Join(dir, entry.Name()),
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
// LoadBundled registers all bundled skills into an existing registry.
func LoadBundled(r *Registry) {
	for _, s := range bundledSkills() {
		r.Register(s)
	}
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
			Description: "Design and build a domain-specific agent team harness for this project. Use when asked to 'build a harness', 'design an agent team', 'set up a harness', or 'create specialist agents'. Generates .claudio/agents/ definitions, .claudio/skills/ orchestrators, and registers the harness in CLAUDIO.md. Also handles harness audits, extensions, and maintenance.",
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
			Name:        "caveman-commit",
			Description: "Ultra-compressed commit message generator. Conventional Commits format. Subject ≤50 chars, body only when why isn't obvious.",
			Content:     cavemanCommitSkillContent,
			Source:      "bundled",
		},
		{
			Name:        "caveman-review",
			Description: "Ultra-compressed code review comments. Each comment is one line: location, problem, fix.",
			Content:     cavemanReviewSkillContent,
			Source:      "bundled",
		},
	}
}

var commitSkillContent = `## Context

- ` + "`git status`" + `: !` + "`git status`" + `
- ` + "`git diff HEAD`" + `: !` + "`git diff HEAD`" + `
- ` + "`git log --oneline -5`" + `: !` + "`git log --oneline -5`" + `

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

Write a minimal CLAUDIO.md at the project root. Every line must pass this test: "Would removing this cause Claudio to make mistakes?" If no, cut it.

**Consume ` + "`note`" + ` entries from the Phase 3 preference queue whose target is CLAUDIO.md** — add each as a concise line in the most relevant section.

Include:
- Build/test/lint commands Claudio can't guess (non-standard scripts, flags, or sequences)
- Code style rules that DIFFER from language defaults (e.g., "prefer type over interface")
- Testing instructions and quirks (e.g., "run single test with: pytest -k 'test_name'")
- Repo etiquette (branch naming, PR conventions, commit style)
- Required env vars or setup steps
- Non-obvious gotchas or architectural decisions
- Important parts from existing AI coding tool configs (.cursor/rules, .cursorrules, .github/copilot-instructions.md, .windsurfrules, .clinerules)

Exclude:
- File-by-file structure or component lists (Claudio can discover these)
- Standard language conventions Claudio already knows
- Generic advice ("write clean code", "handle errors")
- Detailed API docs or long references — use ` + "`@path/to/import`" + ` syntax instead
- Information that changes frequently — reference the source with ` + "`@path/to/import`" + `
- Commands obvious from manifest files (e.g., standard "npm test", "cargo test", "pytest")

Be specific: "Use 2-space indentation in TypeScript" is better than "Format code properly."

Do not repeat yourself. Do not make up sections like "Common Development Tasks" or "Tips for Development".

Prefix the file with:

` + "```" + `
# CLAUDIO.md

This file provides guidance to Claudio (github.com/Abraxas-365/claudio) when working with code in this repository.
` + "```" + `

If CLAUDIO.md already exists: read it carefully, then ALWAYS regenerate a complete improved version. Show the user what changed and why. Never skip or say "looks fine" — the user ran /init specifically because they want an update.

For projects with multiple concerns, suggest organizing instructions into ` + "`.claudio/rules/`" + ` as separate focused files (e.g., ` + "`code-style.md`" + `, ` + "`testing.md`" + `, ` + "`security.md`" + `). These are loaded automatically alongside CLAUDIO.md.

For monorepos or multi-module projects: mention that subdirectory CLAUDIO.md files can be added for module-specific instructions. Offer to create them if the user wants.

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

Recap what was set up — which files were written and the key points in each. Remind the user these files are a starting point: they should review and tweak them, and can run ` + "`/init`" + ` again anytime to re-scan.

Then present a well-formatted to-do list of additional suggestions relevant to this repo. Most impactful first:
- If frontend code was detected (React, Vue, Svelte, etc.): mention that browser/screenshot testing tools help Claudio verify UI changes visually.
- If you found gaps in Phase 7 (missing GitHub CLI, missing linting) and the user said no: list them with a one-line reason.
- If tests are missing or sparse: suggest setting up a test framework so Claudio can verify its own changes.
- Always suggest: browse and customize ` + "`.claudio/skills/`" + ` to add project-specific workflows.`

var harnessSkillContent = `You are building a domain-specific agent team harness for this project.

A harness is a reusable multi-agent architecture that decomposes complex, recurring tasks into coordinated specialist agents. It produces:
- ` + "`.claudio/agents/<name>.md`" + ` — one file per specialist role
- ` + "`.claudio/skills/<harness-name>/skill.md`" + ` — an orchestrator skill that assembles and runs the team
- An entry in CLAUDIO.md documenting when and how to invoke it

---

## Phase 0: Audit existing harnesses AND existing agents

Before designing anything, check for prior work:

1. Read CLAUDIO.md — look for an "## Agent Harnesses" section
2. List ` + "`.claudio/agents/`" + ` and ` + "`.claudio/skills/`" + ` directories (project-local)
3. List ` + "`~/.claudio/agents/`" + ` (global) — crystallized agents available across projects
4. For each existing agent file, read its frontmatter ` + "`description`" + ` and note:
   - Agent name
   - Domain / specialization
   - Whether it has accumulated memory (presence of a memory dir alongside its definition)
5. If a harness already exists for this domain or a closely related one, decide:
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
2. **Reuse-vs-create check (REQUIRED)** — before assigning a type, scan the inventory you built in Phase 0:
   - **REUSE** an existing crystallized agent if its description clearly matches this role's domain and responsibilities. Reusing brings the agent's accumulated memory into the team — that memory IS preserved when teammates spawn (memory dirs are plumbed through the runner).
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

TeamCreate({
  name: "<harness-name>-team",
  members: [
    // For roles backed by an existing crystallized agent, use that agent's
    // exact name in the "agent" field — its persona AND accumulated memory
    // will be loaded into the teammate session.
    { name: "<display-name>", agent: "<existing-or-new-agent>", task: "<initial instruction with FULL context — agent has no prior knowledge of THIS conversation, even if it has memory from prior sessions>" },
    { name: "<agent-b>", agent: "<agent-b>", task: "<initial instruction>" }
  ]
})

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

### 2. Trigger verification
Test that the skill description triggers correctly. Mentally evaluate these queries:
- Write 3 queries that SHOULD trigger this harness (natural phrasings a user would say)
- Write 3 queries that should NOT trigger it (related but different tasks)
- If any should-trigger query wouldn't match the description, revise the description

### 3. Dry-run check
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
- How to invoke: ` + "`/<harness-name> <example input>`" + `
- 3 example invocations showing different use cases
- Suggested next steps:
  - Run the harness on a real task to validate
  - Extend with additional specialists if gaps emerge
  - Run ` + "`/harness`" + ` again in extend mode to add agents later

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
[planner] → [architect] → TeamCreate:
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

Default: **full**. Switch: ` + "`/caveman lite|full|ultra`" + `.

## Rules

Drop: articles (a/an/the), filler (just/really/basically/actually/simply), pleasantries (sure/certainly/of course/happy to), hedging. Fragments OK. Short synonyms (big not extensive, fix not "implement a solution for"). Technical terms exact. Code blocks unchanged. Errors quoted exact.

Pattern: ` + "`[thing] [action] [reason]. [next step].`" + `

Not: "Sure! I'd be happy to help you with that. The issue you're experiencing is likely caused by..."
Yes: "Bug in auth middleware. Token expiry check use ` + "`<`" + ` not ` + "`<=`" + `. Fix:"

## Intensity

|-------|------------|

Example — "Why React component re-render?"

Example — "Explain database connection pooling."

## Auto-Clarity

Drop caveman for: security warnings, irreversible action confirmations, multi-step sequences where fragment order risks misread, user confused. Resume caveman after clear part done.

Example — destructive op:
> **Warning:** This will permanently delete all rows in the ` + "`users`" + ` table and cannot be undone.
> ` + "```sql" + `
> DROP TABLE users;
> ` + "```" + `
> Caveman resume. Verify backup exist first.

## Boundaries

Code/commits/PRs: write normal. "stop caveman" or "normal mode": revert. Level persist until changed or session end.`

var cavemanCommitSkillContent = `Write commit messages terse and exact. Conventional Commits format. No fluff. Why over what.

## Rules

**Subject line:**
- ` + "`<type>(<scope>): <imperative summary>`" + ` — ` + "`<scope>`" + ` optional
- Types: ` + "`feat`" + `, ` + "`fix`" + `, ` + "`refactor`" + `, ` + "`perf`" + `, ` + "`docs`" + `, ` + "`test`" + `, ` + "`chore`" + `, ` + "`build`" + `, ` + "`ci`" + `, ` + "`style`" + `, ` + "`revert`" + `
- Imperative mood: "add", "fix", "remove" — not "added", "adds", "adding"
- ≤50 chars when possible, hard cap 72
- No trailing period
- Match project convention for capitalization after the colon

**Body (only if needed):**
- Skip entirely when subject is self-explanatory
- Add body only for: non-obvious *why*, breaking changes, migration notes, linked issues
- Wrap at 72 chars
- Bullets ` + "`-`" + ` not ` + "`*`" + `
- Reference issues/PRs at end: ` + "`Closes #42`" + `, ` + "`Refs #17`" + `

**What NEVER goes in:**
- "This commit does X", "I", "we", "now", "currently" — the diff says what
- "As requested by..." — use Co-authored-by trailer
- "Generated with Claude Code" or any AI attribution
- Emoji (unless project convention requires)
- Restating the file name when scope already says it

## Examples

Diff: new endpoint for user profile with body explaining the why
- ❌ "feat: add a new endpoint to get user profile information from the database"
- ✅
  ` + "```" + `
  feat(api): add GET /users/:id/profile

  Mobile client needs profile data without the full user payload
  to reduce LTE bandwidth on cold-launch screens.

  Closes #128
  ` + "```" + `

Diff: breaking API change
- ✅
  ` + "```" + `
  feat(api)!: rename /v1/orders to /v1/checkout

  BREAKING CHANGE: clients on /v1/orders must migrate to /v1/checkout
  before 2026-06-01. Old route returns 410 after that date.
  ` + "```" + `

## Auto-Clarity

Always include body for: breaking changes, security fixes, data migrations, anything reverting a prior commit. Never compress these into subject-only — future debuggers need the context.

## Boundaries

Only generates the commit message. Does not run ` + "`git commit`" + `, does not stage files, does not amend. Output the message as a code block ready to paste. "stop caveman-commit" or "normal mode": revert to verbose commit style.`

var cavemanReviewSkillContent = `Write code review comments terse and actionable. One line per finding. Location, problem, fix. No throat-clearing.

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
