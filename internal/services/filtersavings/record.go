package filtersavings

import (
	"context"
	"fmt"
	"strings"
)

// Record inserts a filter savings entry for a command invocation.
// The command string is normalized to its base command plus first subcommand
// (e.g. "git diff" from "git diff --stat HEAD~3").
func (s *Service) Record(ctx context.Context, command string, bytesIn, bytesOut int) error {
	normalized := normalizeCommand(command)
	_, err := s.db.Conn().ExecContext(ctx,
		`INSERT INTO filter_savings (command, bytes_in, bytes_out) VALUES (?, ?, ?)`,
		normalized, bytesIn, bytesOut,
	)
	if err != nil {
		return fmt.Errorf("filtersavings: record: %w", err)
	}
	return nil
}

// normalizeCommand extracts the base command and optional first subcommand.
// For example: "git diff --stat HEAD~3" -> "git diff",
// "/usr/bin/ls -la /tmp" -> "ls".
func normalizeCommand(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return ""
	}

	parts := strings.Fields(cmd)

	// Strip path prefix from first token (e.g. /usr/bin/git -> git).
	base := parts[0]
	if idx := strings.LastIndex(base, "/"); idx >= 0 {
		base = base[idx+1:]
	}

	if len(parts) < 2 {
		return base
	}

	// If the second token looks like a subcommand (not a flag), include it.
	second := parts[1]
	if strings.HasPrefix(second, "-") {
		return base
	}
	return base + " " + second
}
