package outputfilter

import (
	"strings"
)

// filterPytest filters pytest test output.
func filterPytest(sub, output string) (string, bool) {
	_ = sub
	lines := strings.Split(output, "\n")

	// Find the final result line (=== X passed/failed ... ===)
	finalLine := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "=") && strings.HasSuffix(trimmed, "=") &&
			(strings.Contains(trimmed, "passed") || strings.Contains(trimmed, "failed") ||
				strings.Contains(trimmed, "error")) {
			finalLine = trimmed
		}
	}

	// Check if there are any failures or errors
	hasFailed := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "FAILED ") || strings.HasPrefix(trimmed, "ERROR ") {
			hasFailed = true
			break
		}
	}

	// All passing: return just the final result line
	if !hasFailed {
		if finalLine != "" {
			return finalLine, true
		}
		return "", false
	}

	// Failures present: collect FAILED/ERROR lines, short summary section, and final line
	var result []string
	inShortSummary := false
	inHeader := true // skip the platform/rootdir/plugins header block
	headerEnded := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect end of header (first blank line after header, or first test result line)
		if inHeader && !headerEnded {
			if trimmed == "" || strings.Contains(trimmed, "collected") ||
				strings.HasPrefix(trimmed, "PASSED") || strings.HasPrefix(trimmed, "FAILED") ||
				strings.HasPrefix(trimmed, "ERROR") || strings.HasPrefix(trimmed, "test_") ||
				(strings.HasPrefix(trimmed, "=") && strings.HasSuffix(trimmed, "=")) {
				headerEnded = true
				inHeader = false
			} else {
				// Still in header — skip platform, rootdir, plugins lines
				continue
			}
		}

		// Skip blank lines outside of summary section
		if trimmed == "" && !inShortSummary {
			continue
		}

		// Detect short test summary info section
		if strings.Contains(trimmed, "short test summary info") {
			inShortSummary = true
			result = append(result, trimmed)
			continue
		}

		// End of short summary section (next === line that isn't the summary header)
		if inShortSummary && strings.HasPrefix(trimmed, "=") && strings.HasSuffix(trimmed, "=") &&
			!strings.Contains(trimmed, "short test summary info") {
			inShortSummary = false
			// This is likely the final result line — add it
			result = append(result, trimmed)
			continue
		}

		if inShortSummary {
			result = append(result, trimmed)
			continue
		}

		// Skip PASSED lines (passing tests are noise)
		if strings.HasSuffix(trimmed, "PASSED") || strings.Contains(trimmed, " PASSED ") {
			continue
		}

		// Skip collecting lines
		if strings.HasPrefix(trimmed, "collecting") || strings.Contains(trimmed, "collected") {
			continue
		}

		// Keep FAILED and ERROR test lines
		if strings.HasPrefix(trimmed, "FAILED ") || strings.HasPrefix(trimmed, "ERROR ") {
			result = append(result, trimmed)
			continue
		}

		// Keep the final result line
		if strings.HasPrefix(trimmed, "=") && strings.HasSuffix(trimmed, "=") &&
			(strings.Contains(trimmed, "passed") || strings.Contains(trimmed, "failed") ||
				strings.Contains(trimmed, "error")) {
			result = append(result, trimmed)
			continue
		}
	}

	// Trim trailing blank lines
	for len(result) > 0 && result[len(result)-1] == "" {
		result = result[:len(result)-1]
	}

	if len(result) == 0 && finalLine != "" {
		return finalLine, true
	}
	if len(result) == 0 {
		return "", false
	}
	return strings.Join(result, "\n"), true
}
