package outputfilter

import (
	"fmt"
	"strings"
)

// filterCurl filters curl output, stripping verbose connection/header noise
// and truncating large response bodies.
func filterCurl(output string) (string, bool) {
	lines := strings.Split(output, "\n")

	// Detect verbose mode by looking for "< " or "> " or "* " prefixed lines
	isVerbose := false
	for _, line := range lines {
		if len(line) >= 2 && (line[:2] == "< " || line[:2] == "> " || line[:2] == "* ") {
			isVerbose = true
			break
		}
	}

	if !isVerbose {
		// Non-verbose: just truncate body to 100 lines
		return truncateLines(lines, 100), true
	}

	// Verbose mode: keep status line, Content-Type, and body
	var result []string
	var bodyLines []string

	for _, line := range lines {
		if len(line) < 2 {
			// Empty or single-char lines are part of the body
			bodyLines = append(bodyLines, line)
			continue
		}

		prefix := line[:2]

		switch prefix {
		case "< ":
			// Response header — keep only HTTP status and Content-Type
			rest := line[2:]
			if strings.HasPrefix(rest, "HTTP/") {
				result = append(result, rest)
			} else if strings.HasPrefix(strings.ToLower(rest), "content-type:") {
				result = append(result, rest)
			}
			// Strip all other response headers
		case "> ":
			// Request header — strip all
			continue
		case "* ":
			// Connection/TLS info — strip all
			continue
		default:
			// Body line
			bodyLines = append(bodyLines, line)
		}
	}

	// Truncate body to 100 lines
	if len(bodyLines) > 100 {
		result = append(result, bodyLines[:100]...)
		result = append(result, fmt.Sprintf("... [truncated %d more lines]", len(bodyLines)-100))
	} else {
		result = append(result, bodyLines...)
	}

	return strings.TrimSpace(strings.Join(result, "\n")), true
}

// truncateLines truncates to maxLines, appending a truncation notice if needed.
func truncateLines(lines []string, maxLines int) string {
	if len(lines) <= maxLines {
		return strings.Join(lines, "\n")
	}
	result := lines[:maxLines]
	result = append(result, fmt.Sprintf("... [truncated %d more lines]", len(lines)-maxLines))
	return strings.Join(result, "\n")
}
