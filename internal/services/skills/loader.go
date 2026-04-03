package skills

import (
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
	}
}

var commitSkillContent = `You are being asked to create a git commit. Follow these steps carefully:

1. Run the following bash commands in parallel, each using the Bash tool:
  - Run a git status command to see all untracked files. IMPORTANT: Never use the -uall flag.
  - Run a git diff command to see both staged and unstaged changes that will be committed.
  - Run a git log --oneline -10 command to see recent commit messages, so that you can follow this repository's commit message style.

2. Analyze all staged changes (both previously staged and newly added) and draft a commit message:
  - Summarize the nature of the changes (eg. new feature, enhancement, bug fix, refactoring, test, docs, etc.)
  - Ensure the message accurately reflects the changes and their purpose (i.e. "add" means a wholly new feature, "update" means an enhancement, "fix" means a bug fix, etc.)
  - Do not commit files that likely contain secrets (.env, credentials.json, etc). Warn the user if they specifically request to commit those files
  - Draft a concise (1-2 sentences) commit message that focuses on the "why" rather than the "what"

3. Run the following commands:
   - Add relevant untracked files to the staging area (prefer specific files over "git add -A")
   - Create the commit using a HEREDOC for the message:
     git commit -m "$(cat <<'EOF'
     Your commit message here.
     EOF
     )"
   - Run git status after the commit to verify success

4. If the commit fails due to pre-commit hook: fix the issue and create a NEW commit (never amend)

Important:
- NEVER run additional commands to read or explore code, besides git bash commands
- DO NOT push to the remote repository unless the user explicitly asks
- Never use git commands with the -i flag (interactive mode is not supported)
- If there are no changes to commit, do not create an empty commit`

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

var prSkillContent = `You are being asked to create a pull request. Follow these steps carefully:

1. Run the following bash commands in parallel:
   - Run git status to see all untracked files (never use -uall flag)
   - Run git diff to see both staged and unstaged changes
   - Check if the current branch tracks a remote branch
   - Run git log and ` + "`git diff main...HEAD`" + ` (or master) to understand the full commit history

2. Analyze ALL changes that will be included (all commits, not just the latest):
   - Keep the PR title short (under 70 characters)
   - Use the description/body for details, not the title

3. Run the following commands:
   - Create new branch if needed
   - Push to remote with -u flag if needed
   - Create PR using ` + "`gh pr create`" + `:
     ` + "```" + `
     gh pr create --title "the pr title" --body "$(cat <<'EOF'
     ## Summary
     <1-3 bullet points>

     ## Test plan
     [Bulleted markdown checklist of testing TODOs...]
     EOF
     )"
     ` + "```" + `

Important:
- Return the PR URL when you're done
- DO NOT push to the remote unless creating the PR requires it`

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
