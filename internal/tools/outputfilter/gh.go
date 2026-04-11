package outputfilter

import (
	"fmt"
	"strings"
)

// filterGh dispatches GitHub CLI output filtering by sub-command.
func filterGh(sub, output string) (string, bool) {
	switch sub {
	case "pr":
		return filterGhPr(output), true
	case "issue":
		return filterGhIssue(output), true
	case "run":
		return filterGhRun(output), true
	default:
		return "", false
	}
}

// filterGhPr filters `gh pr list`, `gh pr view`, `gh pr diff` output.
func filterGhPr(output string) string {
	lines := strings.Split(output, "\n")
	var result []string
	count := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Skip ANSI spinner noise
		if spinnerRe.MatchString(trimmed) {
			continue
		}
		// For diff output, keep +/- lines and headers; skip index/similarity lines
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(trimmed, "index ") || strings.HasPrefix(trimmed, "similarity index") ||
			strings.HasPrefix(trimmed, "rename from") || strings.HasPrefix(trimmed, "rename to") ||
			strings.HasPrefix(lower, "binary files") {
			continue
		}
		count++
		if count <= 200 {
			result = append(result, truncate(trimmed, 200))
		}
	}

	if len(result) == 0 {
		return "gh pr: no output"
	}
	if count > 200 {
		result = append(result, fmt.Sprintf("... +%d more lines", count-200))
	}
	return strings.Join(result, "\n")
}

// filterGhIssue filters `gh issue list` and `gh issue view` output.
func filterGhIssue(output string) string {
	lines := strings.Split(output, "\n")
	var result []string
	count := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if spinnerRe.MatchString(trimmed) {
			continue
		}
		count++
		if count <= 50 {
			result = append(result, truncate(trimmed, 200))
		}
	}

	if len(result) == 0 {
		return "gh issue: no output"
	}
	if count > 50 {
		result = append(result, fmt.Sprintf("... +%d more items", count-50))
	}
	return strings.Join(result, "\n")
}

// filterGhRun filters `gh run list`, `gh run view`, `gh run watch` output.
func filterGhRun(output string) string {
	lines := strings.Split(output, "\n")
	var result []string
	count := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if spinnerRe.MatchString(trimmed) {
			continue
		}
		// Drop pure progress refresh lines (carriage returns)
		if strings.Contains(line, "\r") {
			continue
		}
		lower := strings.ToLower(trimmed)
		// For watch output: only keep status/result lines
		if strings.Contains(lower, "completed") || strings.Contains(lower, "failed") ||
			strings.Contains(lower, "queued") || strings.Contains(lower, "in_progress") ||
			strings.Contains(lower, "success") || strings.Contains(lower, "error") ||
			strings.Contains(lower, "cancelled") || strings.HasPrefix(trimmed, "✓") ||
			strings.HasPrefix(trimmed, "✗") || strings.HasPrefix(trimmed, "X") ||
			strings.HasPrefix(trimmed, "STATUS") || strings.HasPrefix(trimmed, "Run") ||
			strings.Contains(trimmed, "\t") { // tab-separated list rows
			count++
			if count <= 30 {
				result = append(result, truncate(trimmed, 200))
			}
			continue
		}
	}

	if len(result) == 0 {
		// Fallback: show all non-empty lines up to limit
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" && !spinnerRe.MatchString(trimmed) {
				result = append(result, truncate(trimmed, 200))
			}
		}
		if len(result) > 30 {
			result = result[:30]
			result = append(result, fmt.Sprintf("... truncated"))
		}
	}

	if len(result) == 0 {
		return "gh run: no output"
	}
	return strings.Join(result, "\n")
}
