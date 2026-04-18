package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Abraxas-365/claudio/internal/utils"
)

// knownCapabilities is the set of valid capability tokens agents may declare.
var knownCapabilities = map[string]bool{
	"design": true,
}

var (
	customDirsMu sync.RWMutex
	customDirs   []string
)

// SetCustomDirs registers directories to search for custom agent definitions.
// Call this during app initialization before agents are resolved.
func SetCustomDirs(dirs ...string) {
	customDirsMu.Lock()
	defer customDirsMu.Unlock()
	customDirs = dirs
}

// getCustomDirs returns the registered custom agent directories.
func getCustomDirs() []string {
	customDirsMu.RLock()
	defer customDirsMu.RUnlock()
	return customDirs
}

// GetCustomDirs returns the registered custom agent directories (exported for TUI use).
func GetCustomDirs() []string { return getCustomDirs() }

// AgentDefinition describes a built-in agent type.
type AgentDefinition struct {
	// Type is the unique identifier for this agent type (e.g., "general-purpose", "Explore").
	Type string

	// WhenToUse describes when to use this agent.
	WhenToUse string

	// SystemPrompt is the system prompt used when spawning this agent.
	SystemPrompt string

	// Tools lists tools the agent can use. "*" means all tools.
	Tools []string

	// DisallowedTools lists tools explicitly denied to the agent.
	DisallowedTools []string

	// Model overrides the model for this agent type ("haiku", "sonnet", "opus", or "" for inherit).
	Model string

	// ReadOnly indicates the agent cannot modify files.
	ReadOnly bool

	// MemoryDir is the agent's own memory directory (for crystallized session-agents).
	MemoryDir string

	// ExtraSkillsDir is the agent-specific skills directory (merged on top of global skills).
	ExtraSkillsDir string

	// ExtraPluginsDir is the agent-specific plugins directory (merged on top of global plugins).
	ExtraPluginsDir string

	// SourceSession is the session ID this agent was crystallized from.
	SourceSession string

	// SourceProject is the project directory this agent was originally created in.
	SourceProject string

	// Capabilities lists opt-in tool sets this agent can access.
	// Known values: "design" (enables RenderMockup, VerifyMockup, BundleMockup).
	// Built-in agents set this in code; custom agents set it via frontmatter.
	Capabilities []string

	// MaxTurns limits the number of agentic turns (API calls) for this agent.
	// 0 means unlimited. Prevents runaway agents from consuming excessive tokens.
	MaxTurns int
}

// BuiltInAgents returns all built-in agent definitions.
func BuiltInAgents() []AgentDefinition {
	return []AgentDefinition{
		GeneralPurposeAgent(),
		ExploreAgent(),
		PlanAgent(),
		VerificationAgent(),
		DesignAgent(),
	}
}

// GetAgent returns the agent definition for the given type, or the general-purpose agent if not found.
// Searches both built-in and custom agents from registered directories.
func GetAgent(agentType string) AgentDefinition {
	for _, a := range AllAgents(getCustomDirs()...) {
		if a.Type == agentType {
			return a
		}
	}
	return GeneralPurposeAgent()
}

// AgentTypesList returns a formatted string of all agent types for use in the Agent tool prompt.
func AgentTypesList() string {
	agents := AllAgents(getCustomDirs()...)
	var lines string
	for _, a := range agents {
		toolsStr := "all tools"
		if len(a.Tools) > 0 && a.Tools[0] != "*" {
			toolsStr = joinStrings(a.Tools)
		}
		if len(a.DisallowedTools) > 0 {
			toolsStr += " (except " + joinStrings(a.DisallowedTools) + ")"
		}
		lines += "- " + a.Type + ": " + a.WhenToUse + " (Tools: " + toolsStr + ")\n"
	}
	return lines
}

func joinStrings(ss []string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += ", "
		}
		result += s
	}
	return result
}

// GeneralPurposeAgent returns the general-purpose agent definition.
func GeneralPurposeAgent() AgentDefinition {
	return AgentDefinition{
		Type:     "general-purpose",
		MaxTurns: 50,
		WhenToUse: "General-purpose agent for researching complex questions, searching for code, and executing multi-step tasks. When you are searching for a keyword or file and are not confident that you will find the right match in the first few tries use this agent to perform the search for you.",
		Tools:     []string{"*"},
		SystemPrompt: `You are an agent working as part of a larger system. Complete the task described in your prompt.

## Process

1. **Plan** — before acting, briefly state what you intend to do and why
2. **Execute** — use tools to accomplish the task; validate each step with tool output before proceeding
3. **Verify** — after implementation, run tests or commands to confirm the result is correct
4. **Report** — summarize what was done, what changed, and flag anything unexpected

## Tool guidance

- Glob — find files by name pattern (prefer over Bash find)
- Grep — search file contents by regex (prefer over Bash grep)
- Read — read a specific file (prefer over Bash cat/head/tail)
- Edit — modify an existing file (always Read first)
- Write — create a new file (only when no existing file can be edited)
- Bash — system commands, running tests, git operations, build commands

## Escalation

Stop and report back if:
- The task requires architectural decisions outside the stated scope
- You discover the problem is significantly larger than described
- You are blocked and cannot make progress after two attempts`,
	}
}

// ExploreAgent returns the codebase exploration agent definition.
func ExploreAgent() AgentDefinition {
	return AgentDefinition{
		Type:     "Explore",
		MaxTurns: 25,
		WhenToUse: "Fast agent specialized for exploring codebases. Use this when you need to quickly find files by patterns (eg. \"src/components/**/*.tsx\"), search code for keywords (eg. \"API endpoints\"), or answer questions about the codebase (eg. \"how do API endpoints work?\"). When calling this agent, specify the desired thoroughness level: \"quick\" for basic searches, \"medium\" for moderate exploration, or \"very thorough\" for comprehensive analysis across multiple locations and naming conventions.",
		Tools:     []string{"*"},
		DisallowedTools: []string{"Agent", "ExitPlanMode", "Edit", "Write", "NotebookEdit"},
		Model:    "haiku",
		ReadOnly: true,
		SystemPrompt: `You are a codebase exploration specialist. Your only job is to find and analyze — never modify.

## READ-ONLY MODE — no file modifications

You do NOT have access to Edit, Write, or NotebookEdit. Any attempt to create or modify files will fail. Never use Bash to write files (no >, >>, heredocs, touch, mkdir, rm, cp, mv).

## Tool guidance

- **Glob** — find files by name pattern; use this first for broad discovery
- **Grep** — search file contents with regex; use for symbol/keyword lookup
- **Read** — read a specific file at a known path (prefer over Bash cat/head/tail)
- **Bash** — read-only shell ops only: ls, git log, git diff, git status
- Run multiple Glob/Grep/Read calls **in parallel** whenever possible — this is the main speed lever

## Process

1. Start broad (Glob patterns, Grep for key symbols) then narrow to specific files
2. Adapt thoroughness to the level requested by the caller: quick / medium / very thorough
3. Validate findings by cross-referencing — don't rely on a single match

## Output format

End your response with:

### Findings
- Key files and their roles
- Relevant symbols, patterns, or answers

### Observations
- Anything surprising, inconsistent, or worth flagging to the caller

Communicate findings as a direct message — do NOT write files.` +
		"\n\n## Memory — check first, save after\n\n" +
		"**Before exploring**, check if fresh findings are already cached:\n" +
		"- Call `Memory(action=\"search\", query=\"<primary directory or topic>\")` first\n" +
		"- If a relevant entry is found, return it directly — skip re-exploring (save the tokens)\n\n" +
		"**After completing exploration** of a directory or architectural area (medium or very thorough depth):\n" +
		"1. Derive the memory key from the explored path: lowercase, replace `/` with `-`, strip leading `/`\n" +
		"   - `internal/tools` → `internal-tools`  |  `src/components/auth` → `src-components-auth`  |  `lib/` → `lib`\n" +
		"2. Save: `Memory(action=\"save\", name=\"<key>\", description=\"<one-line summary>\", facts=[\"<fact1>\", ...], tags=[\"codebase-map\"])`\n" +
		"3. Facts: one sentence each, max 8, focus on what a future agent needs to navigate this area\n\n" +
		"Skip saving for: quick single-file reads, targeted symbol lookups, or searches that reveal nothing structural.",
	}
}

// PlanAgent returns the planning agent definition.
func PlanAgent() AgentDefinition {
	return AgentDefinition{
		Type:     "Plan",
		MaxTurns: 30,
		WhenToUse: "Software architect agent for designing implementation plans. Use this when you need to plan the implementation strategy for a task. Returns step-by-step plans, identifies critical files, and considers architectural trade-offs.",
		Tools:     []string{"*"},
		DisallowedTools: []string{"Agent", "ExitPlanMode", "Edit", "Write", "NotebookEdit"},
		ReadOnly: true,
		SystemPrompt: `You are a software architect. Your job is to explore the codebase and produce a concrete implementation plan. You do NOT write code — you produce the plan that coding agents will execute.

## READ-ONLY MODE — no file modifications

You do NOT have access to Edit, Write, or NotebookEdit. Explore only.

## Process

1. **Explore** — read relevant files, understand existing patterns, conventions, and constraints
2. **Identify trade-offs** — surface at least two approaches with their pros/cons before picking one
3. **Design** — produce a step-by-step plan; every step must be unambiguous enough that a coding agent can execute it without asking clarifying questions
4. **Flag risks** — call out edge cases, migration concerns, breaking changes, or unknowns

If something is unclear, ask **one focused question** before proceeding — don't assume.

## Output format

Your response MUST end with:

### Implementation Plan
Ordered list of steps. For each step: what to do, which file(s), and why.

### Critical Files
- path/to/file.go — what changes and why
- (ordered by implementation sequence)

### Risks & Open Questions
- Any unknowns or decisions deferred to the implementer`,
	}
}

// VerificationAgent returns the verification agent definition.
func VerificationAgent() AgentDefinition {
	return AgentDefinition{
		Type:     "verification",
		MaxTurns: 20,
		WhenToUse: "Verification agent for validating implementation work. Use after non-trivial implementation to verify correctness.",
		Tools:     []string{"*"},
		DisallowedTools: []string{"Edit", "Write", "NotebookEdit"},
		ReadOnly: true,
		SystemPrompt: `You are a verification specialist. Your job is to validate that an implementation is correct, complete, and safe — using actual commands, not code reading alone.

## Rules

- You CANNOT modify project source files (Edit/Write/NotebookEdit are blocked for source code)
- Every claim in your verdict MUST be backed by actual tool/command output — no assumptions
- You MUST run the full test suite
- You MUST include at least one adversarial probe (boundary values, concurrent access, idempotency, error paths)

## Verification process

1. **Read** the implementation — understand what changed and why
2. **Build** — compile or type-check to catch static errors
3. **Test** — run the full test suite; capture output
4. **Lint** — run the linter if available
5. **Probe** — exercise at least one edge case or adversarial input not covered by existing tests
6. **Cross-check** — confirm the implementation matches the stated requirements

## Tool guidance

- Use **Bash** for all commands (tests, build, lint, git diff)
- Use **Read** to examine source files
- Use **Grep** to check for regressions or missing error handling

## Output format

End with exactly one of:

**VERDICT: PASS** — all checks passed; implementation is correct and complete
**VERDICT: FAIL** — critical issues found:
  - [issue 1]
  - [issue 2]
**VERDICT: PARTIAL** — implementation mostly correct; concerns:
  - [concern 1]

Include command output as evidence. Do not omit failures.`,
	}
}

// DesignAgent returns the design agent definition.
func DesignAgent() AgentDefinition {
	return AgentDefinition{
		Type:         "design",
		MaxTurns:     80,
		Model:        "claude-sonnet-4-6",
		Capabilities: []string{"design"},
		WhenToUse:    "UI/UX design agent that generates interactive mockups as self-contained HTML. Use for creating app screens, landing pages, dashboards, and design system exploration. Produces verified, exportable HTML prototypes.",
		Tools:        []string{"*"},
		DisallowedTools: []string{"ExitPlanMode"},
		SystemPrompt: designSystemPrompt,
	}
}

// LoadCustomAgents loads agent definitions from markdown files in a directory.
// Custom agents use frontmatter for configuration and markdown body for the system prompt.
//
// Frontmatter fields:
//   - name/description: Agent description (used in whenToUse)
//   - tools: Comma-separated list of allowed tools (or "*" for all)
//   - disallowedTools: Comma-separated list of denied tools
//   - model: Model override (haiku, sonnet, opus)
func LoadCustomAgents(dirs ...string) []AgentDefinition {
	var custom []AgentDefinition

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		// Track agent types loaded via directory-form to avoid double-loading.
		// Directory-form wins over flat-file when both exist for the same agent name.
		dirFormLoaded := make(map[string]bool)

		for _, entry := range entries {
			if entry.IsDir() {
				// Directory-form agent: look for the definition markdown inside the dir.
				agentType := entry.Name()
				agentDir := filepath.Join(dir, agentType)

				// Priority order: AGENT.md > agent.md > <dirname>.md
				candidates := []string{
					filepath.Join(agentDir, "AGENT.md"),
					filepath.Join(agentDir, "agent.md"),
					filepath.Join(agentDir, agentType+".md"),
				}

				var data []byte
				var found bool
				for _, candidate := range candidates {
					d, err := os.ReadFile(candidate)
					if err == nil {
						data = d
						found = true
						break
					}
				}
				if !found {
					continue
				}

				fm, body := utils.ParseFrontmatter(string(data))
				if body == "" {
					continue
				}

				whenToUse := fm.Get("description")
				if whenToUse == "" {
					whenToUse = fm.Get("name")
				}

				def := AgentDefinition{
					Type:            agentType,
					WhenToUse:       whenToUse,
					SystemPrompt:    body,
					Tools:           fm.GetList("tools"),
					DisallowedTools: fm.GetList("disallowedTools"),
					Model:           fm.Get("model"),
					SourceSession:   fm.Get("sourceSession"),
					SourceProject:   fm.Get("sourceProject"),
					Capabilities:    fm.GetList("capabilities"),
				}

				for _, cap := range def.Capabilities {
					if !knownCapabilities[cap] {
						fmt.Fprintf(os.Stderr, "warning: agent %q declares unknown capability %q (ignored)\n", agentType, cap)
					}
				}

				if len(def.Tools) == 0 {
					def.Tools = []string{"*"}
				}

				// Detect subdirs: memory/, skills/, plugins/
				memDir := filepath.Join(agentDir, "memory")
				if info, err := os.Stat(memDir); err == nil && info.IsDir() {
					def.MemoryDir = memDir
				}
				skillsDir := filepath.Join(agentDir, "skills")
				if info, err := os.Stat(skillsDir); err == nil && info.IsDir() {
					def.ExtraSkillsDir = skillsDir
				}
				pluginsDir := filepath.Join(agentDir, "plugins")
				if info, err := os.Stat(pluginsDir); err == nil && info.IsDir() {
					def.ExtraPluginsDir = pluginsDir
				}

				dirFormLoaded[agentType] = true
				custom = append(custom, def)
				continue
			}

			if !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}

			agentType := strings.TrimSuffix(entry.Name(), ".md")

			// Skip flat-file if a directory-form agent with the same name was already loaded.
			if dirFormLoaded[agentType] {
				continue
			}

			path := filepath.Join(dir, entry.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}

			fm, body := utils.ParseFrontmatter(string(data))
			if body == "" {
				continue
			}

			whenToUse := fm.Get("description")
			if whenToUse == "" {
				whenToUse = fm.Get("name")
			}

			def := AgentDefinition{
				Type:            agentType,
				WhenToUse:       whenToUse,
				SystemPrompt:    body,
				Tools:           fm.GetList("tools"),
				DisallowedTools: fm.GetList("disallowedTools"),
				Model:           fm.Get("model"),
				SourceSession:   fm.Get("sourceSession"),
				SourceProject:   fm.Get("sourceProject"),
				Capabilities:    fm.GetList("capabilities"),
			}

			for _, cap := range def.Capabilities {
				if !knownCapabilities[cap] {
					fmt.Fprintf(os.Stderr, "warning: agent %q declares unknown capability %q (ignored)\n", agentType, cap)
				}
			}

			if len(def.Tools) == 0 {
				def.Tools = []string{"*"}
			}

			// Check for sibling subdirs: <name>/memory/, <name>/skills/, <name>/plugins/
			siblingDir := filepath.Join(dir, agentType)
			memDir := filepath.Join(siblingDir, "memory")
			if info, err := os.Stat(memDir); err == nil && info.IsDir() {
				def.MemoryDir = memDir
			}
			skillsDir := filepath.Join(siblingDir, "skills")
			if info, err := os.Stat(skillsDir); err == nil && info.IsDir() {
				def.ExtraSkillsDir = skillsDir
			}
			pluginsDir := filepath.Join(siblingDir, "plugins")
			if info, err := os.Stat(pluginsDir); err == nil && info.IsDir() {
				def.ExtraPluginsDir = pluginsDir
			}

			custom = append(custom, def)
		}
	}

	return custom
}

// AllAgents returns built-in agents merged with custom agents from the given directories.
// Custom agents with the same type name as a built-in agent override the built-in.
func AllAgents(customDirs ...string) []AgentDefinition {
	builtIn := BuiltInAgents()
	custom := LoadCustomAgents(customDirs...)

	// Custom agents override built-in by type
	typeMap := make(map[string]AgentDefinition)
	for _, a := range builtIn {
		typeMap[a.Type] = a
	}
	for _, a := range custom {
		typeMap[a.Type] = a
	}

	// Preserve order: built-in first, then new custom types
	var result []AgentDefinition
	seen := make(map[string]bool)
	for _, a := range builtIn {
		result = append(result, typeMap[a.Type])
		seen[a.Type] = true
	}
	for _, a := range custom {
		if !seen[a.Type] {
			result = append(result, a)
		}
	}
	return result
}
