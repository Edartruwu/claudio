package outputfilter

import (
	"strings"
)

func filterGit(sub, output string) (string, bool) {
	switch sub {
	case "push":
		return filterGitPush(output), true
	case "pull":
		return filterGitPull(output), true
	case "fetch":
		return filterGitFetch(output), true
	case "clone":
		return filterGitClone(output), true
	case "log":
		return Generic(output), true
	default:
		return "", false
	}
}

// filterGitPush strips progress lines, keeps result.
func filterGitPush(output string) string {
	if strings.Contains(output, "Everything up-to-date") {
		return "ok (up-to-date)"
	}

	lines := strings.Split(output, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Skip progress/counting lines
		if isGitTransferNoise(trimmed) {
			continue
		}
		// Keep branch push result lines (contain "->")
		if strings.Contains(trimmed, "->") {
			result = append(result, trimmed)
		}
		// Keep "To <url>" lines
		if strings.HasPrefix(trimmed, "To ") {
			result = append(result, trimmed)
		}
		// Keep error/rejection lines
		if strings.Contains(trimmed, "error") || strings.Contains(trimmed, "rejected") || strings.Contains(trimmed, "failed") {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return "ok"
	}
	return strings.Join(result, "\n")
}

// filterGitPull strips progress lines, summarizes changes.
func filterGitPull(output string) string {
	if strings.Contains(output, "Already up to date") || strings.Contains(output, "Already up-to-date") {
		return "ok (up-to-date)"
	}

	lines := strings.Split(output, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if isGitTransferNoise(trimmed) {
			continue
		}
		// Keep summary lines
		if strings.Contains(trimmed, "file") && strings.Contains(trimmed, "changed") {
			result = append(result, trimmed)
		}
		if strings.Contains(trimmed, "Fast-forward") || strings.Contains(trimmed, "->") {
			result = append(result, trimmed)
		}
		// Keep conflict/error lines
		if strings.Contains(trimmed, "CONFLICT") || strings.Contains(trimmed, "error") || strings.Contains(trimmed, "fatal") {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return "ok"
	}
	return strings.Join(result, "\n")
}

// filterGitFetch strips all progress, keeps ref updates.
func filterGitFetch(output string) string {
	lines := strings.Split(output, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if isGitTransferNoise(trimmed) {
			continue
		}
		// Keep ref update lines and "From" lines
		if strings.Contains(trimmed, "->") || strings.HasPrefix(trimmed, "From ") {
			result = append(result, trimmed)
		}
		if strings.Contains(trimmed, "error") || strings.Contains(trimmed, "fatal") {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return "ok (no updates)"
	}
	return strings.Join(result, "\n")
}

// filterGitClone strips progress lines, keeps result.
func filterGitClone(output string) string {
	lines := strings.Split(output, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if isGitTransferNoise(trimmed) {
			continue
		}
		if strings.HasPrefix(trimmed, "Cloning into") {
			result = append(result, trimmed)
		}
		if strings.Contains(trimmed, "error") || strings.Contains(trimmed, "fatal") {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return "ok"
	}
	return strings.Join(result, "\n")
}

// isGitTransferNoise returns true for git progress/transfer lines.
func isGitTransferNoise(line string) bool {
	lower := strings.ToLower(line)
	noises := []string{
		"enumerating objects",
		"counting objects",
		"compressing objects",
		"receiving objects",
		"resolving deltas",
		"writing objects",
		"remote: enumerating",
		"remote: counting",
		"remote: compressing",
		"remote: total",
		"unpacking objects",
	}
	for _, n := range noises {
		if strings.Contains(lower, n) {
			return true
		}
	}
	// Percentage progress lines
	if strings.Contains(line, "%") && (strings.Contains(line, "(") || strings.Contains(line, "/")) {
		return true
	}
	return false
}
