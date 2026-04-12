// Package codefilter provides source-code comment-stripping filters to reduce
// token usage when the AI reads large source files.  It is analogous to rtk's
// filter.rs FilterLevel system (None / Minimal / Aggressive).
package codefilter

import (
	"fmt"
	"regexp"
	"strings"
)

// FilterLevel controls how aggressively comments are stripped.
type FilterLevel int

const (
	None       FilterLevel = iota // no filtering
	Minimal                       // strip comments, collapse excess blank lines
	Aggressive                    // keep only signatures + imports
)

// Language identifies the programming language of a source file.
type Language int

const (
	LangUnknown    Language = iota
	LangRust
	LangPython
	LangJavaScript
	LangTypeScript
	LangGo
	LangC
	LangCpp
	LangJava
	LangRuby
	LangShell
	LangData // JSON, YAML, TOML, XML, CSV, MD, lock — never filter
)

// CommentPatterns holds the comment delimiters for a language.
type CommentPatterns struct {
	Line       string // line comment prefix, e.g. "//"
	BlockStart string // block comment open,  e.g. "/*"
	BlockEnd   string // block comment close, e.g. "*/"
	DocLine    string // doc-comment prefix to KEEP, e.g. "///" (Rust)
}

// langPatterns returns the comment patterns for the given language.
func langPatterns(lang Language) CommentPatterns {
	switch lang {
	case LangRust:
		return CommentPatterns{Line: "//", BlockStart: "/*", BlockEnd: "*/", DocLine: "///"}
	case LangGo, LangJavaScript, LangTypeScript, LangC, LangCpp, LangJava:
		return CommentPatterns{Line: "//", BlockStart: "/*", BlockEnd: "*/"}
	case LangPython, LangRuby, LangShell:
		return CommentPatterns{Line: "#"}
	default:
		return CommentPatterns{}
	}
}

// DetectLanguage returns the Language for the given file extension (without
// the leading dot, lowercase).
func DetectLanguage(ext string) Language {
	switch strings.ToLower(ext) {
	case "rs":
		return LangRust
	case "py", "pyw":
		return LangPython
	case "js", "mjs", "cjs":
		return LangJavaScript
	case "ts", "tsx":
		return LangTypeScript
	case "go":
		return LangGo
	case "c", "h":
		return LangC
	case "cpp", "cc", "cxx", "hpp", "hh":
		return LangCpp
	case "java":
		return LangJava
	case "rb":
		return LangRuby
	case "sh", "bash", "zsh":
		return LangShell
	case "json", "jsonc", "json5", "yaml", "yml", "toml", "xml",
		"csv", "tsv", "graphql", "gql", "sql", "md", "markdown",
		"txt", "env", "lock":
		return LangData
	default:
		return LangUnknown
	}
}

// MinimalFilter strips standalone line/block comments and collapses runs of
// more than two consecutive blank lines to two.  Data files are returned
// unchanged.  Files whose language has no known comment syntax are also
// returned unchanged.
func MinimalFilter(content string, lang Language) string {
	if lang == LangData {
		return content
	}

	p := langPatterns(lang)
	if p.Line == "" && p.BlockStart == "" {
		return content
	}

	lines := strings.Split(content, "\n")
	result := make([]string, 0, len(lines))

	inBlockComment := false
	inPyDocstring := false
	pyDocDelim := ""
	consecutiveBlanks := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// ── Python / Ruby docstring tracking ────────────────────────────────
		if lang == LangPython {
			if inPyDocstring {
				result = append(result, strings.TrimRight(line, " \t"))
				consecutiveBlanks = 0
				if strings.Contains(trimmed, pyDocDelim) {
					inPyDocstring = false
					pyDocDelim = ""
				}
				continue
			}
			pyDocHandled := false
			for _, delim := range []string{`"""`, `'''`} {
				if strings.HasPrefix(trimmed, delim) {
					rest := trimmed[len(delim):]
					if strings.Contains(rest, delim) {
						// Single-line docstring: keep it
						result = append(result, strings.TrimRight(line, " \t"))
						consecutiveBlanks = 0
					} else {
						// Multi-line docstring opens here
						inPyDocstring = true
						pyDocDelim = delim
						result = append(result, strings.TrimRight(line, " \t"))
						consecutiveBlanks = 0
					}
					pyDocHandled = true
					break
				}
			}
			if pyDocHandled {
				continue
			}
		}

		// ── Block comment handling ───────────────────────────────────────────
		if p.BlockStart != "" {
			if inBlockComment {
				if strings.Contains(line, p.BlockEnd) {
					inBlockComment = false
				}
				continue
			}
			if strings.HasPrefix(trimmed, p.BlockStart) {
				afterOpen := trimmed[len(p.BlockStart):]
				if strings.Contains(afterOpen, p.BlockEnd) {
					continue // single-line block comment — skip
				}
				inBlockComment = true
				continue
			}
		}

		// ── Line comment handling ────────────────────────────────────────────
		if p.Line != "" && strings.HasPrefix(trimmed, p.Line) {
			// Keep doc-comment lines (e.g., Rust ///)
			if p.DocLine != "" && strings.HasPrefix(trimmed, p.DocLine) {
				// fall through to keep
			} else {
				continue // skip plain comment line
			}
		}

		// ── Blank-line collapsing ────────────────────────────────────────────
		if trimmed == "" {
			consecutiveBlanks++
			if consecutiveBlanks > 2 {
				continue
			}
		} else {
			consecutiveBlanks = 0
		}

		result = append(result, strings.TrimRight(line, " \t"))
	}

	return strings.Join(result, "\n")
}

// Compiled patterns used by AggressiveFilter and SmartTruncate.
var (
	// importPattern matches import/use/require/package lines.
	importPattern = regexp.MustCompile(
		`^(use |import |from |require\(|#include|#!|package )`,
	)
	// funcSigPattern matches function definitions (not type declarations).
	funcSigPattern = regexp.MustCompile(
		`^(pub\s+)?(async\s+)?(fn|def|function|func)\s+\w+`,
	)
	// anySigPattern matches any kind of declaration: func, class, struct, etc.
	anySigPattern = regexp.MustCompile(
		`^(pub\s+)?(async\s+)?(fn|def|function|func|class|struct|enum|trait|interface|type)\s+\w+`,
	)
	// declPattern matches top-level variable / constant declarations.
	declPattern = regexp.MustCompile(
		`^(pub\s+)?(const|static|let|var)\s+`,
	)
)

// AggressiveFilter keeps only function/class/struct/interface signatures,
// imports, and top-level declarations; function bodies are replaced with a
// placeholder comment.  Data files receive MinimalFilter only.
func AggressiveFilter(content string, lang Language) string {
	if lang == LangData {
		return MinimalFilter(content, lang)
	}

	filtered := MinimalFilter(content, lang)
	lines := strings.Split(filtered, "\n")

	result := make([]string, 0, len(lines))
	braceDepth := 0
	inFuncBody := false
	emittedImpl := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if braceDepth == 0 {
			// At top level: only emit important lines.
			important := importPattern.MatchString(trimmed) ||
				anySigPattern.MatchString(trimmed) ||
				declPattern.MatchString(trimmed) ||
				trimmed == ""

			if !important {
				continue
			}

			result = append(result, line)

			// Track brace depth opened by this line.
			opens := strings.Count(line, "{")
			closes := strings.Count(line, "}")
			braceDepth += opens - closes

			if braceDepth > 0 {
				inFuncBody = funcSigPattern.MatchString(trimmed)
				emittedImpl = false
			}
		} else {
			// Inside a body (depth ≥ 1).
			opens := strings.Count(line, "{")
			closes := strings.Count(line, "}")
			newDepth := braceDepth + opens - closes

			if newDepth <= 0 {
				// Returning to top level.
				if inFuncBody && !emittedImpl {
					result = append(result, "    // ... implementation")
				}
				result = append(result, line) // closing }
				braceDepth = 0
				inFuncBody = false
				emittedImpl = false
			} else if inFuncBody {
				// Skip body lines; emit marker once.
				if !emittedImpl {
					result = append(result, "    // ... implementation")
					emittedImpl = true
				}
				braceDepth = newDepth
			} else {
				// Inside a non-function body (struct fields, etc.) — keep.
				result = append(result, line)
				braceDepth = newDepth
			}
		}
	}

	return strings.Join(result, "\n")
}

// SmartTruncate truncates content to at most maxLines, preserving important
// lines (function/class signatures, imports) from the tail of the file.
// An overflow marker is appended at the end with exact counts so that:
//
//	N + originalKept == total
//
// where N is the number in "// ... N more lines (total: T)".
func SmartTruncate(content string, maxLines int, _ Language) string {
	lines := strings.Split(content, "\n")
	total := len(lines)
	if total <= maxLines {
		return content
	}

	half := maxLines / 2
	result := make([]string, 0, maxLines+5)

	// Always keep the first half unconditionally.
	result = append(result, lines[:half]...)
	originalKept := half // count of lines taken from the original

	omitted := 0 // lines skipped since last "important" line

	for _, line := range lines[half:] {
		trimmed := strings.TrimSpace(line)

		important := importPattern.MatchString(trimmed) ||
			anySigPattern.MatchString(trimmed) ||
			declPattern.MatchString(trimmed) ||
			strings.HasPrefix(trimmed, "pub ") ||
			strings.HasPrefix(trimmed, "export ") ||
			trimmed == "}" ||
			trimmed == "{"

		if important {
			if omitted > 0 {
				result = append(result, fmt.Sprintf("    // ... %d lines omitted", omitted))
				omitted = 0
			}
			result = append(result, line)
			originalKept++
		} else {
			omitted++
		}
	}

	// Append the final overflow marker.  moreLines + originalKept == total.
	moreLines := total - originalKept
	if moreLines > 0 {
		result = append(result, fmt.Sprintf("// ... %d more lines (total: %d)", moreLines, total))
	}

	return strings.Join(result, "\n")
}
