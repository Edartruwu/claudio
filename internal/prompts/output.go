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
	// StyleMarkdown forces well-structured markdown output.
	StyleMarkdown OutputStyle = "markdown"
)

// OutputStyleConfig holds configuration for an output style, including whether
// to keep the default coding task instructions alongside the style prompt.
type OutputStyleConfig struct {
	Name                   string
	Prompt                 string
	KeepCodingInstructions bool
}

// GetOutputStyleConfig returns the full config for a style (name, prompt, keepCodingInstructions).
// Returns nil for the default style (no changes needed).
func GetOutputStyleConfig(style OutputStyle) *OutputStyleConfig {
	switch style {
	case StyleVerbose:
		return &OutputStyleConfig{
			Name:                   "Verbose",
			Prompt:                 OutputStyleSection(StyleVerbose),
			KeepCodingInstructions: true,
		}
	case StyleConcise:
		return &OutputStyleConfig{
			Name:                   "Concise",
			Prompt:                 OutputStyleSection(StyleConcise),
			KeepCodingInstructions: true,
		}
	case StyleBrief:
		return &OutputStyleConfig{
			Name:                   "Brief",
			Prompt:                 OutputStyleSection(StyleBrief),
			KeepCodingInstructions: true,
		}
	case StyleMarkdown:
		return &OutputStyleConfig{
			Name:                   "Markdown",
			Prompt:                 OutputStyleSection(StyleMarkdown),
			KeepCodingInstructions: true,
		}
	default:
		return nil
	}
}

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

	case StyleMarkdown:
		return `# Output Style: Markdown

Always format your output as well-structured Markdown. Rules:
- Use headers (##, ###) to organize sections
- Use code blocks with language identifiers for all code
- Use bullet points and numbered lists for structured information
- Use bold and italic for emphasis
- Use tables when comparing or listing structured data
- Include horizontal rules between major sections`

	default:
		return "" // default style, no extra instructions needed
	}
}

// SessionMemorySection returns instructions for session memory management.
// It tells the agent how to proactively read and write persistent memories
// using the regular Read/Write tools against the given memory directory.
func SessionMemorySection(memoryDir string) string {
	if memoryDir == "" {
		return ""
	}
	return "# Auto Memory\n\n" +
		"You have a persistent, file-based memory system at `" + memoryDir + "`.\n" +
		"This directory already exists — write to it directly with the Write tool. " +
		"Do not run mkdir or check for its existence.\n\n" +
		"Build up this memory over time so future sessions (and other agents " +
		"reusing this persona) start with everything you have already learned. " +
		"Memories persist across sessions; plans and tasks do not. If something " +
		"will still be useful next time you are spawned, save it as a memory.\n\n" +
		"## When to save\n\n" +
		"Save memories proactively whenever you learn something durable, including:\n" +
		"- The user explicitly asks you to remember (or forget) something — do it immediately\n" +
		"- The user corrects you, expresses a preference, or rejects an approach\n" +
		"- You discover a non-obvious project fact (architecture, conventions, constraints)\n" +
		"- You find a pointer to an external system, doc, or API worth remembering\n" +
		"- You identify a recurring pattern, false positive, or trap to avoid next time\n\n" +
		"Do NOT save:\n" +
		"- Things already obvious from reading the codebase\n" +
		"- Standard best practices the model already knows\n" +
		"- Ephemeral task state (use plans/todos for that)\n" +
		"- Secrets, credentials, or sensitive personal data\n\n" +
		"## Memory types\n\n" +
		"- `user` — facts about the user (role, expertise, preferences)\n" +
		"- `feedback` — corrections, what worked, what to avoid\n" +
		"- `project` — project goals, architecture, ongoing work, decisions\n" +
		"- `reference` — pointers to external systems, docs, APIs\n\n" +
		"## How to save (two steps)\n\n" +
		"**Step 1.** Write the memory to its own markdown file inside `" + memoryDir + "` " +
		"using the Write tool. Use a short kebab-case filename (e.g. `prefers-table-driven-tests.md`) " +
		"and include YAML frontmatter:\n\n" +
		"```markdown\n" +
		"---\n" +
		"name: prefers-table-driven-tests\n" +
		"description: User prefers Go table-driven test pattern\n" +
		"type: feedback\n" +
		"---\n\n" +
		"User pushed back twice when I wrote function-per-case tests.\n" +
		"Always use t.Run with a slice of test cases for Go tests in this project.\n" +
		"```\n\n" +
		"**Step 2.** Update `" + memoryDir + "/MEMORY.md` (the index) by reading it " +
		"and adding a one-line pointer to the new file. If MEMORY.md does not exist, " +
		"create it. Keep entries concise — the index is loaded into context every session.\n\n" +
		"## How to update or delete\n\n" +
		"Memories are just markdown files — you can edit and delete them with your " +
		"normal tools. Keep them accurate over time:\n" +
		"- **Update** an existing memory with the `Edit` tool when a fact changes, " +
		"a preference is refined, or you learn additional context. Prefer editing the " +
		"existing file over creating a near-duplicate.\n" +
		"- **Delete** a memory (using a shell `rm` via the Bash tool) when it becomes " +
		"wrong, obsolete, or the user asks you to forget it. Also remove its line from " +
		"`MEMORY.md`.\n" +
		"- **Rename** by writing the new file and deleting the old one, then updating " +
		"the index.\n" +
		"- Before saving a new memory, check if a related one already exists (use the " +
		"`Memory` tool's `search`) and update it instead of duplicating.\n\n" +
		"## When to access\n\n" +
		"Use the `Memory` tool (`list`, `search`, `read`) to recall details on demand. " +
		"Relevant memories are already injected into your context at session start, " +
		"so you usually do not need to re-read them — but search the store when the " +
		"user mentions something that might already be remembered."
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
