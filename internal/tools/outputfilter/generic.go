package outputfilter

import (
	"regexp"
	"strings"
)

const (
	maxLineWidth       = 500
	maxConsecutiveDups = 2
	maxConsecutiveBlanks = 1
)

// ansiRe matches ANSI escape sequences.
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// progressRe matches common progress indicators.
var progressRe = regexp.MustCompile(`(?i)^\s*[\[({]?[\s=>#\-\.]+[\])}]?\s*\d+%`)

// spinnerChars are characters commonly used for CLI spinners.
var spinnerRe = regexp.MustCompile(`^[\s]*[⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏|/\-\\]+\s`)

// Generic applies generic output filters: ANSI strip, blank collapse,
// dedup, progress bar removal, and long line truncation.
func Generic(output string) string {
	// Strip ANSI escape codes
	output = ansiRe.ReplaceAllString(output, "")

	lines := strings.Split(output, "\n")
	var result []string

	blankCount := 0
	dupCount := 0
	lastLine := ""

	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t\r")

		// Skip progress bars and spinners
		if progressRe.MatchString(trimmed) || spinnerRe.MatchString(trimmed) {
			continue
		}

		// Collapse consecutive blank lines
		if trimmed == "" {
			blankCount++
			if blankCount <= maxConsecutiveBlanks {
				result = append(result, "")
			}
			continue
		}
		blankCount = 0

		// Deduplicate consecutive identical lines
		if trimmed == lastLine {
			dupCount++
			if dupCount == maxConsecutiveDups+1 {
				// Replace the entry that would have been the 3rd dup
				result = append(result, "") // placeholder, we'll fix at the end
			}
			continue
		}
		if dupCount > maxConsecutiveDups {
			// Close off the dedup group
			result = append(result, formatDupMarker(lastLine, dupCount+1))
		}
		dupCount = 0
		lastLine = trimmed

		// Truncate long lines
		if len(trimmed) > maxLineWidth {
			trimmed = trimmed[:maxLineWidth-3] + "..."
		}

		result = append(result, trimmed)
	}

	// Handle trailing dups
	if dupCount > maxConsecutiveDups {
		result = append(result, formatDupMarker(lastLine, dupCount+1))
	}

	// Clean up placeholder empty lines from dedup
	return strings.TrimSpace(strings.Join(result, "\n"))
}

func formatDupMarker(line string, count int) string {
	truncated := line
	if len(truncated) > 80 {
		truncated = truncated[:77] + "..."
	}
	return truncated + " (repeated " + strings.Repeat("", 0) + itoa(count) + " times)"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
