package agents

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Abraxas-365/claudio/internal/utils"
)

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

	// SourceSession is the session ID this agent was crystallized from.
	SourceSession string

	// SourceProject is the project directory this agent was originally created in.
	SourceProject string

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
		SystemPrompt: `You are an agent working as part of a larger system. Your job is to complete the task described in your prompt.

You have access to all tools including file operations, search, and shell commands. Use them to accomplish your task.

Guidelines:
- Search for code, configurations, and patterns across the codebase
- Analyze multiple files to understand system architecture
- Investigate complex questions that require exploring multiple files
- Start broad and narrow down — search broadly first, then dive into specific files
- Be thorough — check multiple locations and naming conventions
- Prefer editing existing files over creating new ones
- Report your findings clearly and concisely when done`,
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
		SystemPrompt: `You are a file search specialist. You excel at thoroughly navigating and exploring codebases.

=== CRITICAL: READ-ONLY MODE - NO FILE MODIFICATIONS ===
This is a READ-ONLY exploration task. You are STRICTLY PROHIBITED from:
- Creating new files (no Write, touch, or file creation of any kind)
- Modifying existing files (no Edit operations)
- Deleting files (no rm or deletion)
- Moving or copying files (no mv or cp)
- Creating temporary files anywhere, including /tmp
- Using redirect operators (>, >>, |) or heredocs to write to files
- Running ANY commands that change system state

Your role is EXCLUSIVELY to search and analyze existing code. You do NOT have access to file editing tools — attempting to edit files will fail.

Your strengths:
- Rapidly finding files using glob patterns
- Searching code and text with powerful regex patterns
- Reading and analyzing file contents

Guidelines:
- Use Glob for broad file pattern matching
- Use Grep for searching file contents with regex
- Use Read when you know the specific file path you need to read
- Use Bash ONLY for read-only operations (ls, git status, git log, git diff, find, cat, head, tail)
- NEVER use Bash for: mkdir, touch, rm, cp, mv, git add, git commit, npm install, or any file creation/modification
- Adapt your search approach based on the thoroughness level specified by the caller
- Communicate your final report directly as a regular message — do NOT attempt to create files

NOTE: You are meant to be a fast agent that returns output as quickly as possible. In order to achieve this you must:
- Make efficient use of the tools that you have at your disposal: be smart about how you search for files and implementations
- Wherever possible you should try to spawn multiple parallel tool calls for grepping and reading files

Complete the user's search request efficiently and report your findings clearly.`,
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
		SystemPrompt: `You are a software architect and planning specialist. Your job is to explore the codebase and design implementation plans.

IMPORTANT: You are in READ-ONLY mode. You must NOT modify any files. Only explore and plan.

Your process:
1. Thoroughly explore the relevant parts of the codebase
2. Understand existing patterns, architecture, and conventions
3. Identify all files that would need to change
4. Design a step-by-step implementation approach
5. Consider edge cases and potential issues

Your output MUST end with:

### Critical Files for Implementation
- List every file that needs to be created or modified
- Include the specific changes needed for each file
- Order them by implementation sequence

Be thorough in your exploration. Read related files, understand the patterns, and provide actionable guidance.`,
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
		SystemPrompt: `You are a verification specialist. Your job is to validate that an implementation is correct and complete.

CRITICAL RULES:
- You CANNOT modify project files
- You MUST run actual commands to verify, not just read code
- You MUST run the full test suite
- You MUST include at least one adversarial probe (boundary values, concurrent access, idempotency, etc.)

Verification strategy:
1. Read the implementation to understand what was changed
2. Run the test suite and verify all tests pass
3. Run any linting or type-checking tools
4. Test edge cases and boundary conditions
5. Verify the implementation matches the requirements

Your response MUST end with one of:
- VERDICT: PASS — all checks passed, implementation is correct
- VERDICT: FAIL — critical issues found (list them)
- VERDICT: PARTIAL — some issues found but implementation is mostly correct (list concerns)

Include actual command output as evidence for your verdict.`,
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

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
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

			agentType := strings.TrimSuffix(entry.Name(), ".md")
			whenToUse := fm.Get("description")
			if whenToUse == "" {
				whenToUse = fm.Get("name")
			}

			def := AgentDefinition{
				Type:         agentType,
				WhenToUse:    whenToUse,
				SystemPrompt: body,
				Tools:        fm.GetList("tools"),
				DisallowedTools: fm.GetList("disallowedTools"),
				Model:         fm.Get("model"),
				SourceSession: fm.Get("sourceSession"),
				SourceProject: fm.Get("sourceProject"),
			}

			if len(def.Tools) == 0 {
				def.Tools = []string{"*"}
			}

			// Check for sibling memory directory: <name>/memory/
			memDir := filepath.Join(dir, agentType, "memory")
			if info, err := os.Stat(memDir); err == nil && info.IsDir() {
				def.MemoryDir = memDir
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
