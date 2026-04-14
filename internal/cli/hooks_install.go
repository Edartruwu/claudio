package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

const postCommitHookScript = `#!/bin/bash
# Auto-invalidate Claudio memory entries for packages changed in this commit.
# Installed by: claudio hooks install-git
# Do not edit manually — re-run 'claudio hooks install-git --force' to update.

set -euo pipefail

# Skip silently if claudio is not installed
if ! command -v claudio &>/dev/null; then
  exit 0
fi

# Get files changed in this commit
changed=$(git diff HEAD~1 --name-only 2>/dev/null) || exit 0
[ -z "$changed" ] && exit 0

declare -A keys
while IFS= read -r f; do
  case "$f" in
    internal/bus/*)         keys["pkg-bus"]=1 ;;
    internal/storage/*)     keys["pkg-storage"]=1 ;;
    internal/cli/*)         keys["pkg-cli"]=1 ;;
    internal/config/*)      keys["pkg-config"]=1 ;;
    internal/security/*)    keys["pkg-security"]=1 ;;
    internal/permissions/*) keys["pkg-permissions"]=1 ;;
    internal/hooks/*)       keys["pkg-hooks"]=1 ;;
    internal/web/*)         keys["pkg-web"]=1 ;;
    internal/tui/*)         keys["pkg-tui"]=1 ;;
    internal/teams/*)       keys["pkg-teams"]=1 ;;
    internal/agents/*)      keys["pkg-agents"]=1 ;;
    internal/tools/*)       keys["pkg-tools"]=1 ;;
    internal/services/*)    keys["pkg-services"]=1 ;;
    internal/app/*)         keys["pkg-app"]=1 ;;
    cmd/*|*.go|go.mod|go.sum) keys["architecture"]=1 ;;
    .claudio/*)             keys["architecture"]=1 ;;
  esac
done <<< "$changed"

for key in "${!keys[@]}"; do
  claudio memory invalidate "$key" && echo "[claudio] invalidated memory: $key" || true
done
`

var hooksCmd = &cobra.Command{
	Use:   "hooks",
	Short: "Manage Claudio git hooks",
}

var hooksInstallGitCmd = &cobra.Command{
	Use:   "install-git",
	Short: "Install a post-commit git hook that auto-invalidates Claudio memory entries",
	RunE: func(cmd *cobra.Command, args []string) error {
		force, _ := cmd.Flags().GetBool("force")

		// Find the git root by walking up from cwd
		gitRoot, err := findGitRoot()
		if err != nil {
			fmt.Fprintln(os.Stderr, "not a git repository")
			return fmt.Errorf("not a git repository")
		}

		target := filepath.Join(gitRoot, ".git", "hooks", "post-commit")

		// Ensure hooks directory exists
		hooksDir := filepath.Dir(target)
		if err := os.MkdirAll(hooksDir, 0755); err != nil {
			return fmt.Errorf("failed to create hooks directory: %w", err)
		}

		// Check if the file already exists
		if _, err := os.Stat(target); err == nil && !force {
			fmt.Fprintln(os.Stderr, "post-commit hook already exists. Use --force to overwrite.")
			os.Exit(1)
		}

		// Write the hook script
		if err := os.WriteFile(target, []byte(postCommitHookScript), 0755); err != nil {
			return fmt.Errorf("failed to write hook script: %w", err)
		}

		// Ensure the file is executable
		if err := os.Chmod(target, 0755); err != nil {
			return fmt.Errorf("failed to make hook executable: %w", err)
		}

		fmt.Println("installed post-commit hook at .git/hooks/post-commit")
		return nil
	},
}

// findGitRoot walks up the directory tree from cwd until it finds a .git directory.
func findGitRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		gitDir := filepath.Join(dir, ".git")
		info, err := os.Stat(gitDir)
		if err == nil && info.IsDir() {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			break
		}
		dir = parent
	}

	return "", errors.New("not a git repository")
}

func init() {
	hooksInstallGitCmd.Flags().Bool("force", false, "Overwrite existing hook if present")
	hooksCmd.AddCommand(hooksInstallGitCmd)
	rootCmd.AddCommand(hooksCmd)
}
