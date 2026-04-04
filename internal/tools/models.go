package tools

import (
	"encoding/json"
)

// buildModelEnum returns a JSON array string for the model enum field.
// Only includes models explicitly passed in — no hardcoded defaults.
// The caller (app.go) is responsible for including the right models
// based on which providers are configured.
func buildModelEnum(models []string) string {
	// Deduplicate while preserving order
	seen := make(map[string]bool, len(models))
	deduped := make([]string, 0, len(models))
	for _, m := range models {
		if m != "" && !seen[m] {
			seen[m] = true
			deduped = append(deduped, m)
		}
	}

	b, _ := json.Marshal(deduped)
	return string(b)
}
