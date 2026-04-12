package outputfilter

import (
	"regexp"
	"strings"
)

// MatchRule is a short-circuit rule: if Pattern matches the full output blob,
// return Message. If Unless is non-empty and also matches, skip this rule.
type MatchRule struct {
	Pattern string // regex
	Message string
	Unless  string // optional guard regex
}

// MatchOutput checks output against rules in order. Returns (message, true) on
// first match where Unless does NOT also match. Returns ("", false) if no match.
// Malformed regexes are skipped silently.
func MatchOutput(output string, rules []MatchRule) (string, bool) {
	for _, rule := range rules {
		// Compile pattern regex
		patternRe, err := regexp.Compile(rule.Pattern)
		if err != nil {
			continue // skip malformed regex
		}

		// Check if pattern matches
		if !patternRe.MatchString(output) {
			continue
		}

		// If Unless is specified, check if it matches
		if rule.Unless != "" {
			unlessRe, err := regexp.Compile(rule.Unless)
			if err != nil {
				// Malformed unless regex, treat as not matching (don't skip the rule)
			} else if unlessRe.MatchString(output) {
				// Unless matched, skip this rule
				continue
			}
		}

		// Pattern matched and unless didn't (or wasn't set), return
		return rule.Message, true
	}

	return "", false
}

// ReplaceRule is a regex substitution applied line-by-line.
type ReplaceRule struct {
	Pattern     string // regex
	Replacement string
}

// ApplyReplace applies rules sequentially (output of rule N feeds rule N+1).
// Each rule is applied to every line. Malformed regexes are skipped with no error.
func ApplyReplace(output string, rules []ReplaceRule) string {
	lines := strings.Split(output, "\n")

	for _, rule := range rules {
		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			continue // skip malformed regex
		}

		// Apply to each line
		for i, line := range lines {
			lines[i] = re.ReplaceAllString(line, rule.Replacement)
		}
	}

	return strings.Join(lines, "\n")
}

// KeepLinesMatching keeps only lines where at least one pattern matches.
// Empty lines are kept regardless. Malformed patterns are skipped.
func KeepLinesMatching(output string, patterns []string) string {
	if len(patterns) == 0 {
		return output
	}

	// Compile all patterns first
	compiledPatterns := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			continue // skip malformed pattern
		}
		compiledPatterns = append(compiledPatterns, re)
	}

	if len(compiledPatterns) == 0 {
		return output // no valid patterns, keep everything
	}

	lines := strings.Split(output, "\n")
	var result []string

	for _, line := range lines {
		// Keep empty lines
		if strings.TrimSpace(line) == "" {
			result = append(result, line)
			continue
		}

		// Check if line matches any pattern
		matches := false
		for _, re := range compiledPatterns {
			if re.MatchString(line) {
				matches = true
				break
			}
		}

		if matches {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}

// StripLinesMatching drops lines matching any pattern.
// Empty lines are never dropped by this function.
// Malformed patterns are skipped.
func StripLinesMatching(output string, patterns []string) string {
	if len(patterns) == 0 {
		return output
	}

	// Compile all patterns first
	compiledPatterns := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			continue // skip malformed pattern
		}
		compiledPatterns = append(compiledPatterns, re)
	}

	if len(compiledPatterns) == 0 {
		return output // no valid patterns, keep everything
	}

	lines := strings.Split(output, "\n")
	var result []string

	for _, line := range lines {
		// Never drop empty lines
		if strings.TrimSpace(line) == "" {
			result = append(result, line)
			continue
		}

		// Check if line matches any pattern
		matches := false
		for _, re := range compiledPatterns {
			if re.MatchString(line) {
				matches = true
				break
			}
		}

		if !matches {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}

// TailLines returns the last n lines of output. If n >= line count, returns output unchanged.
func TailLines(output string, n int) string {
	if n < 0 {
		return output
	}

	lines := strings.Split(output, "\n")

	if n >= len(lines) {
		return output
	}

	// Return last n lines
	result := lines[len(lines)-n:]
	return strings.Join(result, "\n")
}

// HeadLines returns the first n lines. If n >= line count, returns output unchanged.
func HeadLines(output string, n int) string {
	if n < 0 {
		return output
	}

	lines := strings.Split(output, "\n")

	if n >= len(lines) {
		return output
	}

	// Return first n lines
	result := lines[:n]
	return strings.Join(result, "\n")
}

// MaxLines caps output to the first n lines (alias for HeadLines for schema clarity).
func MaxLines(output string, n int) string {
	return HeadLines(output, n)
}

// TruncateLinesAt truncates any line longer than n characters, appending "...".
func TruncateLinesAt(output string, n int) string {
	lines := strings.Split(output, "\n")

	for i, line := range lines {
		if len(line) > n {
			// Truncate to n-3 characters and add "..."
			if n < 3 {
				lines[i] = "..."
			} else {
				lines[i] = line[:n-3] + "..."
			}
		}
	}

	return strings.Join(lines, "\n")
}

// OnEmpty returns fallback if output is empty or whitespace-only after trimming.
func OnEmpty(output, fallback string) string {
	if strings.TrimSpace(output) == "" {
		return fallback
	}
	return output
}
