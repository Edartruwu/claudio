package utils

import (
	"strings"
	"unicode/utf8"
)

// Truncate truncates a string to maxLen characters, appending "..." if truncated.
func Truncate(s string, maxLen int) string {
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	runes := []rune(s)
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
}

// FirstLine returns the first non-empty line of a string.
func FirstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

// IndentLines indents each line of a string with the given prefix.
func IndentLines(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n")
}

// CountLines returns the number of lines in a string.
func CountLines(s string) int {
	if s == "" {
		return 0
	}
	n := 1
	for _, c := range s {
		if c == '\n' {
			n++
		}
	}
	return n
}

// ContainsAny checks if s contains any of the given substrings.
func ContainsAny(s string, substrs ...string) bool {
	lower := strings.ToLower(s)
	for _, sub := range substrs {
		if strings.Contains(lower, strings.ToLower(sub)) {
			return true
		}
	}
	return false
}

// SanitizeForLog removes potentially sensitive information from a string.
func SanitizeForLog(s string) string {
	// Mask common secret patterns
	patterns := []struct{ prefix, mask string }{
		{"sk-", "sk-****"},
		{"sk_test_", "sk_test_****"},
		{"AKIA", "AKIA****"},
		{"ghp_", "ghp_****"},
		{"gho_", "gho_****"},
	}

	result := s
	for _, p := range patterns {
		if idx := strings.Index(result, p.prefix); idx >= 0 {
			end := idx + len(p.prefix)
			for end < len(result) && result[end] != ' ' && result[end] != '\n' && result[end] != '"' {
				end++
			}
			result = result[:idx] + p.mask + result[end:]
		}
	}
	return result
}
