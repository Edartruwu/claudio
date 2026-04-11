package outputfilter

import (
	"fmt"
	"strings"
)

// filterRake filters `rake` task output.
func filterRake(sub, output string) (string, bool) {
	_ = sub // rake sub-commands are task names, not structured
	lines := strings.Split(output, "\n")
	var errors []string
	var result []string
	totalLines := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		totalLines++
		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, "error") || strings.Contains(lower, "failed") ||
			strings.HasPrefix(lower, "rake aborted") || strings.HasPrefix(lower, "don't know how") {
			errors = append(errors, trimmed)
		}
	}

	if len(errors) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "rake: %d error(s)\n", len(errors))
		for _, e := range errors {
			fmt.Fprintf(&b, "  %s\n", truncate(e, 120))
		}
		return strings.TrimSpace(b.String()), true
	}

	// Short output: pass through
	if totalLines <= 20 {
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				result = append(result, trimmed)
			}
		}
		return strings.Join(result, "\n"), true
	}

	return Generic(output), true
}

// filterRspec filters `rspec` test output — show failures + summary only.
func filterRspec(sub, output string) (string, bool) {
	_ = sub
	lines := strings.Split(output, "\n")
	var failures []string
	var summary []string
	inFailure := false
	failureDepth := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect failure block header (numbered failures like "1) SomeSpec...")
		if len(trimmed) > 2 && trimmed[0] >= '1' && trimmed[0] <= '9' && trimmed[1] == ')' {
			inFailure = true
			failureDepth = 0
			failures = append(failures, trimmed)
			continue
		}

		if inFailure {
			if trimmed == "" {
				failureDepth++
				if failureDepth >= 2 {
					inFailure = false
					failures = append(failures, "")
				}
				continue
			}
			failureDepth = 0
			failures = append(failures, "  "+truncate(trimmed, 120))
			continue
		}

		// Summary lines: contain "example" or "failure" or timing
		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, "example") || strings.Contains(lower, "failure") ||
			strings.Contains(lower, "pending") || strings.HasPrefix(trimmed, "Finished in") ||
			strings.HasPrefix(trimmed, "Randomized with seed") {
			summary = append(summary, trimmed)
		}
	}

	// If all passing (no failures), just show summary
	if len(failures) == 0 {
		if len(summary) == 0 {
			return "rspec: ok (no output)", true
		}
		return strings.Join(summary, "\n"), true
	}

	var b strings.Builder
	b.WriteString("Failures:\n\n")
	for _, f := range failures {
		fmt.Fprintln(&b, f)
	}
	if len(summary) > 0 {
		b.WriteString("\n")
		for _, s := range summary {
			fmt.Fprintln(&b, s)
		}
	}
	return strings.TrimSpace(b.String()), true
}

// filterRubocop filters `rubocop` lint output — show offenses + summary only.
func filterRubocop(sub, output string) (string, bool) {
	_ = sub
	lines := strings.Split(output, "\n")
	var offenses []string
	var summary []string
	var fileHeader string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)

		// File path lines (start with / or relative path followed by a newline of offenses)
		if strings.HasPrefix(trimmed, "Inspecting") {
			continue // skip progress line
		}
		if strings.HasPrefix(trimmed, "/") || (strings.Contains(trimmed, ".rb") && !strings.Contains(trimmed, ":")) {
			fileHeader = trimmed
			continue
		}

		// Offense lines: "path/file.rb:line:col: C: message"
		if strings.Contains(trimmed, ".rb:") {
			if fileHeader != "" {
				offenses = append(offenses, fileHeader)
				fileHeader = ""
			}
			offenses = append(offenses, truncate(trimmed, 150))
			continue
		}
		fileHeader = "" // reset if not offense

		// Summary: "x files inspected, y offenses detected"
		if strings.Contains(lower, "file") && strings.Contains(lower, "inspected") {
			summary = append(summary, trimmed)
			continue
		}
		if strings.Contains(lower, "offense") || strings.Contains(lower, "no offenses") {
			summary = append(summary, trimmed)
			continue
		}
	}

	if len(offenses) == 0 {
		if len(summary) == 0 {
			return "rubocop: no offenses", true
		}
		return strings.Join(summary, "\n"), true
	}

	var b strings.Builder
	shown := 0
	for _, o := range offenses {
		if shown >= 40 {
			fmt.Fprintf(&b, "... +%d more offenses\n", len(offenses)-40)
			break
		}
		fmt.Fprintln(&b, o)
		shown++
	}
	if len(summary) > 0 {
		b.WriteString("\n")
		for _, s := range summary {
			fmt.Fprintln(&b, s)
		}
	}
	return strings.TrimSpace(b.String()), true
}

// filterBundle filters `bundle install` and `bundle update` output.
func filterBundle(sub, output string) (string, bool) {
	switch sub {
	case "install", "update":
		// fall through
	default:
		return "", false
	}

	lines := strings.Split(output, "\n")
	var result []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		// Skip fetching/installing individual gem progress
		if strings.HasPrefix(lower, "fetching ") || strings.HasPrefix(lower, "installing ") {
			continue
		}
		// Keep bundle complete, error, warning lines
		if strings.HasPrefix(lower, "bundle complete") || strings.Contains(lower, "bundle updated") ||
			strings.Contains(lower, "gems now installed") || strings.HasPrefix(lower, "bundler:") ||
			strings.Contains(lower, "error") || strings.Contains(lower, "warning") ||
			strings.Contains(lower, "conflict") || strings.Contains(lower, "could not find") ||
			strings.HasPrefix(trimmed, "Using ") || strings.HasPrefix(lower, "gem ") {
			result = append(result, trimmed)
		}
	}

	if len(result) == 0 {
		return "bundle " + sub + ": ok", true
	}
	// Cap output
	if len(result) > 30 {
		extra := len(result) - 30
		result = result[:30]
		result = append(result, fmt.Sprintf("... +%d more lines", extra))
	}
	return strings.Join(result, "\n"), true
}
