package outputfilter

import (
	"fmt"
	"strings"
)

// filterKubectl dispatches Kubernetes CLI output filtering by sub-command.
func filterKubectl(sub, output string) (string, bool) {
	switch sub {
	case "get":
		return filterKubectlGet(output), true
	case "logs":
		return filterKubectlLogs(output), true
	case "describe":
		return filterKubectlDescribe(output), true
	case "apply", "delete", "create", "patch":
		return filterKubectlApply(output), true
	default:
		return "", false
	}
}

// filterKubectlGet filters `kubectl get pods/services/deployments` output.
// Keeps header + up to 30 rows, strips annotation noise.
func filterKubectlGet(output string) string {
	lines := strings.Split(output, "\n")
	var result []string
	count := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Keep header line
		if strings.HasPrefix(strings.ToUpper(trimmed), "NAME") {
			result = append(result, trimmed)
			continue
		}
		// Skip annotation/label dump lines typical in -o wide
		if strings.HasPrefix(trimmed, "Annotations:") || strings.HasPrefix(trimmed, "Labels:") {
			continue
		}
		count++
		if count <= 30 {
			result = append(result, truncate(trimmed, 200))
		}
	}

	if len(result) == 0 {
		return "kubectl get: no resources found"
	}
	if count > 30 {
		result = append(result, fmt.Sprintf("... +%d more resources", count-30))
	}
	return strings.Join(result, "\n")
}

// filterKubectlLogs filters `kubectl logs` output — show last N lines.
func filterKubectlLogs(output string) string {
	lines := strings.Split(output, "\n")
	var nonEmpty []string
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			nonEmpty = append(nonEmpty, truncate(trimmed, 300))
		}
	}

	const maxLines = 50
	if len(nonEmpty) == 0 {
		return "kubectl logs: no output"
	}
	if len(nonEmpty) <= maxLines {
		return strings.Join(nonEmpty, "\n")
	}

	// Show last maxLines lines with a header
	tail := nonEmpty[len(nonEmpty)-maxLines:]
	header := fmt.Sprintf("[Showing last %d of %d log lines]", maxLines, len(nonEmpty))
	return header + "\n" + strings.Join(tail, "\n")
}

// filterKubectlDescribe filters `kubectl describe pod/service` output.
// Keeps key sections, strips verbose annotation/event noise.
func filterKubectlDescribe(output string) string {
	lines := strings.Split(output, "\n")
	var result []string
	inAnnotations := false
	inManagedFields := false
	annotationLines := 0
	const maxAnnotationLines = 3

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Track annotation sections — show a few, then skip
		if strings.HasPrefix(trimmed, "Annotations:") {
			inAnnotations = true
			annotationLines = 0
			result = append(result, trimmed)
			continue
		}
		if strings.HasPrefix(trimmed, "ManagedFields:") || strings.HasPrefix(trimmed, "managedFields:") {
			inManagedFields = true
			continue
		}
		if inManagedFields {
			// End managed fields on next top-level key
			if trimmed != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
				inManagedFields = false
			} else {
				continue
			}
		}
		if inAnnotations {
			// Annotations end when we hit a non-indented line
			if trimmed == "" || (!strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t")) {
				inAnnotations = false
			} else {
				annotationLines++
				if annotationLines <= maxAnnotationLines {
					result = append(result, trimmed)
				} else if annotationLines == maxAnnotationLines+1 {
					result = append(result, "  ...")
				}
				continue
			}
		}

		if trimmed == "" {
			// Allow single blank line as section separator
			if len(result) > 0 && result[len(result)-1] != "" {
				result = append(result, "")
			}
			continue
		}

		result = append(result, truncate(trimmed, 200))
	}

	if len(result) == 0 {
		return "kubectl describe: no output"
	}
	// Cap total lines
	if len(result) > 100 {
		result = result[:100]
		result = append(result, "... (truncated)")
	}
	return strings.TrimSpace(strings.Join(result, "\n"))
}

// filterKubectlApply filters `kubectl apply/delete/create/patch` output.
func filterKubectlApply(output string) string {
	lines := strings.Split(output, "\n")
	var result []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		// Keep configured/unchanged/created/deleted/error lines
		if strings.Contains(lower, "configured") || strings.Contains(lower, "created") ||
			strings.Contains(lower, "deleted") || strings.Contains(lower, "unchanged") ||
			strings.Contains(lower, "error") || strings.Contains(lower, "warning") ||
			strings.Contains(lower, "patched") {
			result = append(result, trimmed)
		}
	}

	if len(result) == 0 {
		return Generic(output)
	}
	return strings.Join(result, "\n")
}
