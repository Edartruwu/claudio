package outputfilter

import (
	"strings"
)

// filterEslint filters ESLint output, keeping error/warning detail lines
// and the summary. Strips blank lines and standalone filename headers.
func filterEslint(output string) string {
	lines := strings.Split(output, "\n")
	var result []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Keep error/warning detail lines (format: "  line:col  error  message  rule-name")
		if (strings.Contains(trimmed, "error") || strings.Contains(trimmed, "warning")) &&
			len(trimmed) > 0 && (trimmed[0] >= '0' && trimmed[0] <= '9') {
			result = append(result, trimmed)
			continue
		}

		// Keep lines with file:line:col format (e.g. "/path/file.js:10:5:")
		if strings.Contains(trimmed, ":") {
			parts := strings.SplitN(trimmed, ":", 4)
			if len(parts) >= 3 && isDigits(parts[1]) {
				result = append(result, trimmed)
				continue
			}
		}

		// Keep summary lines (e.g. "✖ 10 problems (5 errors, 5 warnings)")
		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, "problem") || strings.Contains(lower, "error") && strings.Contains(lower, "warning") {
			result = append(result, trimmed)
			continue
		}

		// Keep lines that start with space (indented detail lines under a file)
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') && trimmed != "" {
			result = append(result, trimmed)
			continue
		}
	}

	if len(result) == 0 {
		return "eslint: ok (no issues)"
	}

	return strings.Join(result, "\n")
}

// filterTsc filters TypeScript compiler output, keeping error lines
// and the final error count summary.
func filterTsc(output string) string {
	lines := strings.Split(output, "\n")
	var result []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Keep error lines: "file.ts(line,col): error TSxxxx: message"
		if strings.Contains(trimmed, "error TS") {
			result = append(result, trimmed)
			continue
		}

		// Keep final summary: "Found X errors." or "Found X errors in Y files."
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "found") && strings.Contains(lower, "error") {
			result = append(result, trimmed)
			continue
		}

		// Strip watch mode lines
		if strings.Contains(lower, "starting compilation") ||
			strings.Contains(lower, "file change detected") ||
			strings.Contains(lower, "watching for file changes") {
			continue
		}
	}

	if len(result) == 0 {
		return "tsc: ok (no errors)"
	}

	return strings.Join(result, "\n")
}

// isDigits returns true if s is non-empty and all digits.
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
