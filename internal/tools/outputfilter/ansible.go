package outputfilter

import (
	"strings"
)

// filterAnsible filters ansible/ansible-playbook output, keeping only
// actionable information: play/task headers for changed/failed tasks,
// the PLAY RECAP block, and stripping ok-status noise and JSON blobs.
func filterAnsible(sub, output string) (string, bool) {
	lines := strings.Split(output, "\n")
	var result []string
	inRecap := false
	keepNextTask := false
	var pendingTask string

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Once we hit PLAY RECAP, keep everything after it
		if strings.HasPrefix(trimmed, "PLAY RECAP") {
			inRecap = true
			result = append(result, line)
			continue
		}
		if inRecap {
			if trimmed != "" {
				result = append(result, line)
			}
			continue
		}

		// Keep PLAY headers
		if strings.HasPrefix(trimmed, "PLAY [") {
			result = append(result, line)
			continue
		}

		// Buffer TASK headers — only emit if followed by changed/fatal
		if strings.HasPrefix(trimmed, "TASK [") {
			// Skip Gathering Facts tasks (noise when successful)
			if strings.Contains(trimmed, "Gathering Facts") {
				pendingTask = ""
				keepNextTask = false
				continue
			}
			pendingTask = line
			keepNextTask = false
			continue
		}

		// fatal: lines — always keep, and emit the buffered TASK header
		if strings.HasPrefix(trimmed, "fatal:") {
			if pendingTask != "" {
				result = append(result, pendingTask)
				pendingTask = ""
			}
			result = append(result, line)
			keepNextTask = true
			continue
		}

		// changed: lines — keep (strip JSON blob), emit buffered TASK header
		if strings.HasPrefix(trimmed, "changed:") {
			if pendingTask != "" {
				result = append(result, pendingTask)
				pendingTask = ""
			}
			// Strip JSON blob after =>
			clean := trimmed
			if idx := strings.Index(clean, " => "); idx >= 0 {
				clean = clean[:idx]
			}
			result = append(result, clean)
			keepNextTask = true
			continue
		}

		// ok: lines — skip (success = noise)
		if strings.HasPrefix(trimmed, "ok:") {
			pendingTask = "" // discard the buffered task
			continue
		}

		// skipping: lines — skip
		if strings.HasPrefix(trimmed, "skipping:") {
			continue
		}

		// Lines after a fatal/changed that are part of the error detail
		if keepNextTask && trimmed != "" {
			// Keep error message lines but stop at next task/play boundary
			if strings.HasPrefix(trimmed, "TASK [") || strings.HasPrefix(trimmed, "PLAY [") {
				// Re-process this line
				i--
				keepNextTask = false
				continue
			}
			// Strip JSON detail blocks
			if strings.HasPrefix(trimmed, "\"") || strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "}") {
				continue
			}
			if strings.Contains(trimmed, "\"msg\":") || strings.Contains(trimmed, "\"stderr\":") {
				result = append(result, "  "+trimmed)
			}
			continue
		}

		// Skip blank lines and other noise
	}

	if len(result) == 0 {
		return "ansible: ok (no changes)", true
	}

	// Collapse multiple blank lines
	output2 := strings.Join(result, "\n")
	for strings.Contains(output2, "\n\n\n") {
		output2 = strings.ReplaceAll(output2, "\n\n\n", "\n\n")
	}

	return strings.TrimSpace(output2), true
}


