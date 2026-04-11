package outputfilter

import (
	"strings"
)

// filterJest filters jest and vitest test output.
// Both tools produce the same output shape, so one filter covers both.
func filterJest(sub, output string) (string, bool) {
	_ = sub
	lines := strings.Split(output, "\n")

	// Collect summary lines (Tests: ..., Test Suites: ...)
	var summaryLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Tests:") || strings.HasPrefix(trimmed, "Test Suites:") {
			summaryLines = append(summaryLines, trimmed)
		}
	}

	// If no summary found at all, return unhandled so Generic() runs
	if len(summaryLines) == 0 {
		return "", false
	}

	// Determine if there are any failures
	hasFailed := false
	for _, s := range summaryLines {
		if strings.Contains(s, "failed") {
			hasFailed = true
			break
		}
	}

	// All passing: return just the summary lines
	if !hasFailed {
		return strings.Join(summaryLines, "\n"), true
	}

	// Failures present: collect FAIL filename lines, ● blocks, and summary
	var result []string
	inCoverage := false
	inFailureBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect start of coverage table and skip it
		if strings.HasPrefix(trimmed, "-------") || strings.HasPrefix(trimmed, "File ") ||
			strings.Contains(trimmed, "% Stmts") || strings.Contains(trimmed, "Coverage summary") {
			inCoverage = true
		}
		if inCoverage {
			// Coverage table ends at an empty line after the table
			if trimmed == "" {
				inCoverage = false
			}
			continue
		}

		// Summary lines — always keep
		if strings.HasPrefix(trimmed, "Tests:") || strings.HasPrefix(trimmed, "Test Suites:") {
			inFailureBlock = false
			result = append(result, trimmed)
			continue
		}

		// FAIL filename lines — keep (provides context)
		if strings.HasPrefix(trimmed, "FAIL ") {
			inFailureBlock = false
			result = append(result, trimmed)
			continue
		}

		// PASS lines — strip (they're noise when there are failures)
		if strings.HasPrefix(trimmed, "PASS ") {
			inFailureBlock = false
			continue
		}

		// Skip passing test lines (✓, ✔, √) — always, even inside failure blocks
		if strings.HasPrefix(trimmed, "✓") || strings.HasPrefix(trimmed, "✔") ||
			strings.HasPrefix(trimmed, "√") {
			continue
		}

		// ● marks start of a failure block — keep the name
		if strings.HasPrefix(trimmed, "●") {
			inFailureBlock = true
			result = append(result, trimmed)
			continue
		}

		if inFailureBlock {
			// Skip stack trace lines (at Object, at ..., file:line:col patterns)
			if strings.HasPrefix(trimmed, "at ") || strings.HasPrefix(trimmed, "at Object") {
				continue
			}
			if trimmed == "" {
				// Blank line inside failure block — keep as separator but don't end block
				result = append(result, "")
				continue
			}
			result = append(result, "  "+truncate(trimmed, 120))
			continue
		}

		// Skip spinner/progress/watch lines
		if strings.HasPrefix(trimmed, "RUNS ") || trimmed == "" {
			continue
		}
	}

	// Trim trailing blank lines
	for len(result) > 0 && result[len(result)-1] == "" {
		result = result[:len(result)-1]
	}

	if len(result) == 0 {
		return strings.Join(summaryLines, "\n"), true
	}
	return strings.Join(result, "\n"), true
}
