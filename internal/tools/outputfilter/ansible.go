package outputfilter

import (
	"fmt"
	"strings"
)

// filterAnsible filters ansible command output.
// sub is the full command name (e.g. "ansible-playbook", "ansible", "ansible-inventory").
func filterAnsible(sub, output string) (string, bool) {
	switch sub {
	case "ansible-playbook":
		return filterAnsiblePlaybook(output), true
	case "ansible", "ansible-inventory":
		return filterAnsibleGeneric(output), true
	default:
		return Generic(output), true
	}
}

// filterAnsiblePlaybook filters `ansible-playbook` output, keeping:
// - PLAY [...] and TASK [...] headers
// - ok:, changed:, failed:, skipping:, unreachable: result lines
// - The full PLAY RECAP section
// - Error/fatal lines
// Strips: [WARNING] deprecation lines, verbose JSON blobs, redundant blank lines
func filterAnsiblePlaybook(output string) string {
	lines := strings.Split(output, "\n")
	var result []string
	inRecap := false
	inJSONBlob := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Once we hit PLAY RECAP, collect everything until blank line after it
		if strings.HasPrefix(trimmed, "PLAY RECAP") {
			inRecap = true
			result = append(result, trimmed)
			continue
		}
		if inRecap {
			// Keep all lines until we hit a new PLAY header (shouldn't happen) or end
			result = append(result, trimmed)
			continue
		}

		// Skip empty lines
		if trimmed == "" {
			continue
		}

		// Skip [WARNING] deprecation lines
		if strings.HasPrefix(trimmed, "[WARNING]") {
			continue
		}

		// Detect start/end of verbose JSON blobs (indented lines starting with {
		// or lines that are pure JSON content from verbose task output)
		if strings.HasPrefix(trimmed, "{") && len(trimmed) > 2 {
			inJSONBlob = true
			continue
		}
		if inJSONBlob {
			// End of JSON blob: a line that starts with "}" closes it
			if trimmed == "}" || trimmed == "}," {
				inJSONBlob = false
			}
			continue
		}

		// Keep PLAY [...] header lines
		if strings.HasPrefix(trimmed, "PLAY [") || trimmed == "PLAY" {
			result = append(result, trimmed)
			continue
		}

		// Keep TASK [...] header lines
		if strings.HasPrefix(trimmed, "TASK [") || trimmed == "TASK" {
			result = append(result, trimmed)
			continue
		}

		// Keep result lines: ok, changed, failed, skipping, unreachable, fatal
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "ok:") || strings.HasPrefix(lower, "changed:") ||
			strings.HasPrefix(lower, "failed:") || strings.HasPrefix(lower, "skipping:") ||
			strings.HasPrefix(lower, "unreachable:") || strings.HasPrefix(lower, "fatal:") {
			result = append(result, trimmed)
			continue
		}

		// Keep error lines
		if strings.HasPrefix(lower, "error:") || strings.HasPrefix(lower, "error!") {
			result = append(result, trimmed)
			continue
		}
	}

	if len(result) == 0 {
		return "ansible-playbook: ok (no output)"
	}

	var b strings.Builder
	for _, r := range result {
		fmt.Fprintln(&b, r)
	}
	return strings.TrimSpace(b.String())
}

// filterAnsibleGeneric filters `ansible` and `ansible-inventory` output,
// keeping error lines and per-host status lines (SUCCESS, FAILED, UNREACHABLE, CHANGED).
func filterAnsibleGeneric(output string) string {
	lines := strings.Split(output, "\n")
	var result []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)

		// Keep error / fatal lines
		if strings.HasPrefix(lower, "error:") || strings.HasPrefix(lower, "fatal:") ||
			strings.HasPrefix(lower, "error!") || strings.HasPrefix(lower, "[error]") {
			result = append(result, trimmed)
			continue
		}

		// Keep host status lines: "host | SUCCESS", "host | FAILED", "host | UNREACHABLE", etc.
		// These contain " | " followed by a status keyword at the start of the status part
		if strings.Contains(trimmed, " | ") {
			upper := strings.ToUpper(trimmed)
			if strings.Contains(upper, "| SUCCESS") || strings.Contains(upper, "| FAILED") ||
				strings.Contains(upper, "| UNREACHABLE") || strings.Contains(upper, "| CHANGED") {
				result = append(result, trimmed)
				continue
			}
		}

		// Keep summary lines with hosts= recap (e.g. ansible-inventory or ad-hoc recap)
		if strings.Contains(lower, "hosts=") {
			result = append(result, trimmed)
			continue
		}
	}

	if len(result) == 0 {
		return "ansible: ok"
	}

	var b strings.Builder
	for _, r := range result {
		fmt.Fprintln(&b, r)
	}
	return strings.TrimSpace(b.String())
}


