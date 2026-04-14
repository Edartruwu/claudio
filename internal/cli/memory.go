package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Abraxas-365/claudio/internal/config"
	"github.com/Abraxas-365/claudio/internal/services/memory"
	"github.com/spf13/cobra"
)

var memoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Manage project memory entries",
}

var memoryListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all memory entries for the current project",
	RunE: func(cmd *cobra.Command, args []string) error {
		projectRoot := detectProjectRoot()
		store := memory.NewStore(config.ProjectMemoryDir(projectRoot))
		entries := store.LoadAll()
		if len(entries) == 0 {
			fmt.Println("no memory entries found")
			return nil
		}
		for _, e := range entries {
			fmt.Printf("%s: %s\n", e.Name, e.Description)
		}
		return nil
	},
}

var memoryInvalidateCmd = &cobra.Command{
	Use:   "invalidate <key>",
	Short: "Remove a named memory entry for the current project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		projectRoot := detectProjectRoot()
		store := memory.NewStore(config.ProjectMemoryDir(projectRoot))

		// Check existence first — Remove silently swallows file-not-found errors.
		if _, err := store.Load(key); err != nil {
			if strings.Contains(err.Error(), "not found") {
				fmt.Printf("warning: memory '%s' not found, nothing to invalidate\n", key)
				return nil
			}
			// Unexpected load error — still idempotent, warn and exit 0.
			fmt.Printf("warning: could not check memory '%s': %v\n", key, err)
			return nil
		}

		if err := store.Remove(key); err != nil {
			fmt.Fprintf(os.Stderr, "error: failed to invalidate memory '%s': %v\n", key, err)
			os.Exit(1)
		}

		fmt.Printf("invalidated: %s\n", key)
		return nil
	},
}

// detectProjectRoot tries git rev-parse first, falls back to os.Getwd.
func detectProjectRoot() string {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err == nil {
		root := strings.TrimSpace(string(out))
		if root != "" {
			return root
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

func init() {
	memoryCmd.AddCommand(memoryListCmd)
	memoryCmd.AddCommand(memoryInvalidateCmd)
	rootCmd.AddCommand(memoryCmd)
}
