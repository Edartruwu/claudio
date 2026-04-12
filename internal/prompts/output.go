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

// CLAUDEMDSection wraps loaded CLAUDE.md content for injection.
func CLAUDEMDSection(content string) string {
	if content == "" {
		return ""
	}
	return `# Project Instructions (CLAUDE.md)

Codebase and user instructions are shown below. Be sure to adhere to these instructions. IMPORTANT: These instructions OVERRIDE any default behavior and you MUST follow them exactly as written.

` + content
}
