package snippets

import (
	"fmt"
	"regexp"
	"strings"
	"text/template"
)

// SnippetDef defines a single snippet template.
type SnippetDef struct {
	Name     string   `json:"name"`
	Params   []string `json:"params"`
	Template string   `json:"template"`
	Lang     string   `json:"lang,omitempty"` // file extension filter: "go", "py", "ts", etc.
}

// Config holds all snippet definitions and the enabled flag.
type Config struct {
	Enabled  bool         `json:"enabled"`
	Snippets []SnippetDef `json:"snippets"`
}

// snippetPattern matches ~name(args) allowing nested parens one level deep.
// Examples: ~errw(db.Query(ctx, id), "fetch user")
var snippetPattern = regexp.MustCompile(`~(\w+)\(`)

// Expand finds all ~name(...) patterns in content and expands them using the
// snippet definitions in cfg. filePath is used for language filtering and
// context resolution (enclosing function return types, etc.).
func Expand(cfg *Config, filePath string, content string) string {
	return expandCore(cfg, filePath, content, "")
}

// ExpandWithContext is like Expand but uses contextSource (e.g., the full file)
// for resolving context variables like ReturnZeros and FuncName, instead of the
// content being expanded. This is useful for Edit operations where the content
// is a fragment but the resolver needs the full file to find the enclosing function.
func ExpandWithContext(cfg *Config, filePath string, content string, contextSource string) string {
	return expandCore(cfg, filePath, content, contextSource)
}

func expandCore(cfg *Config, filePath string, content string, contextSource string) string {
	if cfg == nil || !cfg.Enabled || len(cfg.Snippets) == 0 {
		return content
	}

	ext := fileExt(filePath)

	// Build lookup by name, filtered by language
	lookup := make(map[string]*SnippetDef)
	for i := range cfg.Snippets {
		s := &cfg.Snippets[i]
		if s.Lang != "" && s.Lang != ext {
			continue
		}
		lookup[s.Name] = s
	}
	if len(lookup) == 0 {
		return content
	}

	// Use contextSource for resolution if provided, otherwise use content itself
	resolveSource := content
	if contextSource != "" {
		resolveSource = contextSource
	}

	// Iteratively find and replace snippets (no nesting support)
	result := content
	for {
		loc := snippetPattern.FindStringIndex(result)
		if loc == nil {
			break
		}

		// Extract name
		nameMatch := snippetPattern.FindStringSubmatch(result[loc[0]:])
		if nameMatch == nil {
			break
		}
		name := nameMatch[1]

		// Find matching closing paren (handles one level of nested parens)
		argsStart := loc[0] + len(nameMatch[0]) // position right after the opening paren
		closeIdx := findClosingParen(result, argsStart)
		if closeIdx < 0 {
			// No closing paren found — skip this match
			// Replace the ~ with a placeholder to avoid infinite loop, then restore
			result = result[:loc[0]] + "\x00" + result[loc[0]+1:]
			continue
		}

		argsStr := result[argsStart:closeIdx]

		def, ok := lookup[name]
		if !ok {
			// Unknown snippet — leave as-is, skip past it
			result = result[:loc[0]] + "\x00" + result[loc[0]+1:]
			continue
		}

		expanded := expandSnippet(def, argsStr, filePath, resolveSource, loc[0])
		result = result[:loc[0]] + expanded + result[closeIdx+1:]
	}

	// Restore any placeholders
	result = strings.ReplaceAll(result, "\x00", "~")
	return result
}

// findClosingParen finds the index of the closing ')' that matches the open
// paren whose contents start at pos. Handles nested parens.
func findClosingParen(s string, pos int) int {
	depth := 1
	inStr := byte(0) // tracks whether we're inside a string literal
	for i := pos; i < len(s); i++ {
		ch := s[i]
		if inStr != 0 {
			if ch == inStr && (i == 0 || s[i-1] != '\\') {
				inStr = 0
			}
			continue
		}
		switch ch {
		case '"', '\'', '`':
			inStr = ch
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// expandSnippet executes a single snippet's template with the parsed args and context.
func expandSnippet(def *SnippetDef, argsStr, filePath, fullContent string, offset int) string {
	args := parseArgs(argsStr)

	// Build template data from params
	data := make(map[string]string)
	for i, param := range def.Params {
		if i < len(args) {
			data[param] = strings.TrimSpace(args[i])
		} else {
			data[param] = ""
		}
	}

	// Add context-resolved variables
	ctx := ResolveContext(filePath, fullContent, offset)
	for k, v := range ctx {
		data[k] = v
	}

	// For errw-style snippets, provide a sensible default variable name
	if _, ok := data["result"]; !ok {
		data["result"] = "result"
	}

	tmpl, err := template.New("snippet").Parse(def.Template)
	if err != nil {
		return fmt.Sprintf("/* snippet %q template error: %v */", def.Name, err)
	}

	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err != nil {
		return fmt.Sprintf("/* snippet %q exec error: %v */", def.Name, err)
	}
	return sb.String()
}

// parseArgs splits a comma-separated argument string, respecting nested parens and strings.
func parseArgs(s string) []string {
	var args []string
	depth := 0
	inStr := byte(0)
	start := 0

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inStr != 0 {
			if ch == inStr && (i == 0 || s[i-1] != '\\') {
				inStr = 0
			}
			continue
		}
		switch ch {
		case '"', '\'', '`':
			inStr = ch
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				args = append(args, s[start:i])
				start = i + 1
			}
		}
	}
	if start < len(s) {
		args = append(args, s[start:])
	}
	return args
}

// fileExt returns the file extension without the dot (e.g., "go", "py", "ts").
func fileExt(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			return path[i+1:]
		}
		if path[i] == '/' || path[i] == '\\' {
			return ""
		}
	}
	return ""
}

// ForSystemPrompt returns a markdown section describing available snippets,
// suitable for injection into the system prompt. This content is static for the
// lifetime of the session to avoid busting the Anthropic prompt cache.
func ForSystemPrompt(cfg *Config) string {
	if cfg == nil || !cfg.Enabled || len(cfg.Snippets) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("# Snippet Expansion\n\n")
	sb.WriteString("When using the Write or Edit tools, you can use snippet shorthands instead of writing boilerplate. ")
	sb.WriteString("Snippets are expanded deterministically before content is written to disk.\n\n")

	sb.WriteString("Syntax: `~name(arg1, arg2)` — the expander replaces this with the full template.\n\n")

	sb.WriteString("Rules:\n")
	sb.WriteString("- ALWAYS prefer snippets over writing equivalent boilerplate manually.\n")
	sb.WriteString("- Snippets with a language tag only expand in files of that language.\n")
	sb.WriteString("- Context variables (`{{.ReturnZeros}}`, `{{.FuncName}}`) are resolved automatically from the enclosing function — do NOT fill them in yourself.\n")
	sb.WriteString("- Arguments can contain nested parentheses and strings: `~errw(db.Query(ctx, \"SELECT *\"), \"query\")`\n\n")

	// Group by language for readability
	type group struct {
		lang     string
		snippets []SnippetDef
	}
	groups := map[string]*group{}
	var order []string
	for _, s := range cfg.Snippets {
		key := s.Lang
		if key == "" {
			key = "any"
		}
		g, ok := groups[key]
		if !ok {
			g = &group{lang: key}
			groups[key] = g
			order = append(order, key)
		}
		g.snippets = append(g.snippets, s)
	}

	for _, key := range order {
		g := groups[key]
		if key == "any" {
			sb.WriteString("## All languages\n\n")
		} else {
			sb.WriteString(fmt.Sprintf("## %s\n\n", key))
		}

		for _, s := range g.snippets {
			args := strings.Join(s.Params, ", ")
			sb.WriteString(fmt.Sprintf("`~%s(%s)`\n", s.Name, args))

			// Show expanded template so the AI understands what it produces
			sb.WriteString("```\n")
			sb.WriteString(s.Template)
			if !strings.HasSuffix(s.Template, "\n") {
				sb.WriteString("\n")
			}
			sb.WriteString("```\n\n")
		}
	}

	// One concrete example to anchor the AI's behavior
	if example := buildExample(cfg.Snippets); example != "" {
		sb.WriteString("## Example\n\n")
		sb.WriteString(example)
	}

	return sb.String()
}

// buildExample generates a concrete before/after example from the first snippet
// that has params, so the AI sees exactly how to use the syntax.
func buildExample(defs []SnippetDef) string {
	// Prefer an errw-style snippet for the example since it's the most impactful
	var pick *SnippetDef
	for i := range defs {
		if len(defs[i].Params) > 0 {
			if pick == nil || defs[i].Name == "errw" {
				pick = &defs[i]
			}
		}
	}
	if pick == nil {
		return ""
	}

	args := make([]string, len(pick.Params))
	for i, p := range pick.Params {
		switch p {
		case "call":
			args[i] = "db.Get(ctx, id)"
		case "msg":
			args[i] = `"fetch record"`
		case "name":
			args[i] = "MyFunction"
		default:
			args[i] = p
		}
	}

	var sb strings.Builder
	sb.WriteString("Instead of writing the full boilerplate, write:\n")
	sb.WriteString(fmt.Sprintf("```\n~%s(%s)\n```\n", pick.Name, strings.Join(args, ", ")))
	sb.WriteString("The expander produces the full code with correct types automatically.\n")
	return sb.String()
}
