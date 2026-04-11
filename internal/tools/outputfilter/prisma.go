package outputfilter

import (
	"fmt"
	"strings"
)

// filterPrisma dispatches Prisma CLI output filtering by sub-command.
func filterPrisma(sub, output string) (string, bool) {
	switch sub {
	case "generate":
		return filterPrismaGenerate(output), true
	case "migrate":
		return filterPrismaMigrate(output), true
	case "db":
		return filterPrismaDb(output), true
	case "studio", "format", "validate":
		return Generic(output), true
	default:
		return "", false
	}
}

// filterPrismaGenerate filters `prisma generate` output.
func filterPrismaGenerate(output string) string {
	lines := strings.Split(output, "\n")
	var result []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)

		// Skip verbose loading/environment lines
		if strings.HasPrefix(lower, "environment variables loaded") ||
			strings.HasPrefix(lower, "prisma schema loaded") ||
			strings.Contains(lower, "loading prisma") {
			continue
		}

		// Skip progress dots/spinners
		if spinnerRe.MatchString(trimmed) {
			continue
		}

		// Keep success, error, warning, and important info lines
		if strings.Contains(lower, "generated") || strings.Contains(lower, "error") ||
			strings.Contains(lower, "warning") || strings.Contains(lower, "✔") ||
			strings.Contains(lower, "✓") || strings.Contains(lower, "done") ||
			strings.Contains(lower, "generator") || strings.Contains(trimmed, "ms") {
			result = append(result, trimmed)
		}
	}

	if len(result) == 0 {
		return "prisma generate: ok"
	}
	return strings.Join(result, "\n")
}

// filterPrismaMigrate filters `prisma migrate dev/deploy/status/reset` output.
func filterPrismaMigrate(output string) string {
	lines := strings.Split(output, "\n")
	var result []string
	count := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if len(result) > 0 && result[len(result)-1] != "" {
				result = append(result, "")
			}
			continue
		}
		lower := strings.ToLower(trimmed)

		// Skip verbose schema loading lines
		if strings.HasPrefix(lower, "environment variables loaded") ||
			strings.HasPrefix(lower, "prisma schema loaded") ||
			strings.Contains(lower, "loading prisma") {
			continue
		}

		// Keep migration status lines, errors, and summaries
		if strings.Contains(lower, "migration") || strings.Contains(lower, "applied") ||
			strings.Contains(lower, "created") || strings.Contains(lower, "error") ||
			strings.Contains(lower, "warning") || strings.Contains(lower, "database") ||
			strings.Contains(lower, "drift") || strings.Contains(lower, "pending") ||
			strings.Contains(lower, "✔") || strings.Contains(lower, "✓") ||
			strings.Contains(lower, "up to date") || strings.Contains(lower, "failed") {
			count++
			if count <= 50 {
				result = append(result, trimmed)
			}
		}
	}

	if len(result) == 0 {
		return Generic(output)
	}
	if count > 50 {
		result = append(result, fmt.Sprintf("... +%d more lines", count-50))
	}
	return strings.TrimSpace(strings.Join(result, "\n"))
}

// filterPrismaDb filters `prisma db push`, `prisma db pull`, `prisma db seed` output.
func filterPrismaDb(output string) string {
	lines := strings.Split(output, "\n")
	var result []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)

		// Skip verbose schema loading lines
		if strings.HasPrefix(lower, "environment variables loaded") ||
			strings.HasPrefix(lower, "prisma schema loaded") ||
			strings.Contains(lower, "loading prisma") {
			continue
		}

		// Skip progress spinners
		if spinnerRe.MatchString(trimmed) {
			continue
		}

		// Keep meaningful lines
		if strings.Contains(lower, "changes") || strings.Contains(lower, "applied") ||
			strings.Contains(lower, "error") || strings.Contains(lower, "warning") ||
			strings.Contains(lower, "created") || strings.Contains(lower, "done") ||
			strings.Contains(lower, "seeding") || strings.Contains(lower, "seed") ||
			strings.Contains(lower, "✔") || strings.Contains(lower, "✓") ||
			strings.Contains(lower, "database") || strings.Contains(lower, "schema") {
			result = append(result, trimmed)
		}
	}

	if len(result) == 0 {
		return "prisma db: ok"
	}
	return strings.Join(result, "\n")
}
