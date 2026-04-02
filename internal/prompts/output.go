package prompts

// OutputStyle represents different verbosity modes.
type OutputStyle string

const (
	// StyleDefault is the standard output style.
	StyleDefault OutputStyle = "default"
	// StyleVerbose provides detailed explanations.
	StyleVerbose OutputStyle = "verbose"
	// StyleConcise provides minimal output.
	StyleConcise OutputStyle = "concise"
	// StyleBrief provides the most terse output possible.
	StyleBrief OutputStyle = "brief"
)

// OutputStyleSection returns the system prompt section for the given output style.
func OutputStyleSection(style OutputStyle) string {
	switch style {
	case StyleVerbose:
		return `# Output Style: Verbose

Provide detailed explanations and reasoning. Include:
- Step-by-step breakdowns of your approach
- Explanation of why you chose certain approaches over alternatives
- Detailed error analysis when things go wrong
- Code comments explaining non-obvious logic
- Context about how changes relate to the broader codebase`

	case StyleConcise:
		return `# Output Style: Concise

Be direct and minimal. Rules:
- Lead with the action or answer, skip preamble
- One sentence where one sentence will do
- Skip "I'll" and "Let me" prefixes — just do it
- Only explain when the user wouldn't understand without it
- No trailing summaries of what you just did`

	case StyleBrief:
		return `# Output Style: Brief

Ultra-minimal output. Rules:
- Respond in 1-2 sentences maximum unless code output
- Never explain what you're about to do — just do it
- Never summarize what you just did
- Only speak when you need user input or hit a blocker
- Code and tool calls need no accompanying text
- If you can answer with just a tool call, do that`

	default:
		return "" // default style, no extra instructions needed
	}
}

// SessionMemorySection returns instructions for session memory management.
func SessionMemorySection(memoryDir string) string {
	if memoryDir == "" {
		return ""
	}
	return `# Session Memory

You have access to a persistent memory system at ` + memoryDir + `/memory/. Use it to:
- Remember user preferences and working patterns
- Track project-specific context across sessions
- Store learned patterns and corrections

Memory types:
- **user**: Information about the user (role, preferences, expertise)
- **feedback**: Corrections and confirmations from the user
- **project**: Ongoing work, goals, and context
- **reference**: Pointers to external resources

Save memories as markdown files with YAML frontmatter containing name, description, and type fields.
Keep an index in MEMORY.md.`
}

// CLAUDEMDSection wraps loaded CLAUDE.md content for injection.
func CLAUDEMDSection(content string) string {
	if content == "" {
		return ""
	}
	return `# Project Instructions (CLAUDE.md)

Codebase and user instructions are shown below. Be sure to adhere to these instructions. IMPORTANT: These instructions OVERRIDE any default behavior and you MUST follow them exactly as written.

` + content
}
