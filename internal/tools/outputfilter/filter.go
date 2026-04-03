// Package outputfilter provides RTK-style output filtering to reduce token
// usage for command outputs. It detects the command being run and applies
// command-specific filters, then applies generic filters on top.
package outputfilter

import "strings"

// Filter applies output filtering to a command's combined stdout+stderr.
// It detects the command type and applies the most specific filter available,
// then applies generic filters on top.
func Filter(command, output string) string {
	if output == "" {
		return output
	}

	cmd := normalizeCommand(command)

	// Try command-specific filters first
	if filtered, ok := filterByCommand(cmd, output); ok {
		return filtered
	}

	// Fall back to generic filters only
	return Generic(output)
}

// filterByCommand dispatches to a command-specific filter if one exists.
func filterByCommand(cmd, output string) (string, bool) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return "", false
	}

	base := parts[0]
	sub := ""
	if len(parts) > 1 {
		sub = parts[1]
	}

	switch base {
	case "git":
		return filterGit(sub, output)
	case "go":
		return filterGo(sub, output)
	case "cargo":
		return filterCargo(sub, output)
	case "npm", "pnpm", "yarn":
		return filterNpm(sub, output)
	case "pip", "pip3":
		return filterPip(sub, output)
	case "docker":
		return filterDocker(sub, output)
	case "make":
		return filterMake(output)
	}

	return "", false
}

// normalizeCommand strips leading env vars, shell prefixes, etc.
func normalizeCommand(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	// Strip leading env vars: FOO=bar cmd -> cmd
	for {
		if idx := strings.Index(cmd, "="); idx > 0 {
			// Check there's no space before the =
			prefix := cmd[:idx]
			if !strings.Contains(prefix, " ") {
				// Skip past the value
				rest := cmd[idx+1:]
				if spaceIdx := strings.Index(rest, " "); spaceIdx >= 0 {
					cmd = strings.TrimSpace(rest[spaceIdx:])
					continue
				}
			}
		}
		break
	}
	return cmd
}
