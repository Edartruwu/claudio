package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Abraxas-365/claudio/internal/harness"
)

var harnessCmd = &cobra.Command{
	Use:   "harness",
	Short: "Manage harnesses",
}

var harnessInstallCmd = &cobra.Command{
	Use:   "install <source>",
	Short: "Install harness from git URL or local path",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		source := args[0]
		harnessesDir := getHarnessesDir()

		// Create harnessesDir if needed
		if err := os.MkdirAll(harnessesDir, 0755); err != nil {
			return fmt.Errorf("failed to create harnesses directory: %w", err)
		}

		// Print trust warning for git installs
		if isGitURL(source) {
			fmt.Fprintf(os.Stderr, "Warning: Installing from external source. Harness plugins and MCP servers may execute arbitrary code.\n")
		}

		installPath, err := harness.Install(source, harnessesDir)
		if err != nil {
			return err
		}

		// Extract harness name from install path
		name := filepath.Base(installPath)
		fmt.Printf("Installed harness: %s\n", name)
		return nil
	},
}

var harnessListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed harnesses",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		harnessesDir := getHarnessesDir()

		manifests, err := harness.List(harnessesDir)
		if err != nil {
			return err
		}

		if len(manifests) == 0 {
			fmt.Println("No harnesses installed.")
			return nil
		}

		// Print table header
		fmt.Printf("%-20s %-10s %s\n", "NAME", "VERSION", "DESCRIPTION")
		fmt.Println(strings.Repeat("-", 80))

		// Print each harness
		for _, m := range manifests {
			desc := m.Description
			if len(desc) > 50 {
				desc = desc[:47] + "..."
			}
			fmt.Printf("%-20s %-10s %s\n", m.Name, m.Version, desc)
		}

		return nil
	},
}

var harnessRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove installed harness",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		harnessesDir := getHarnessesDir()

		if err := harness.Uninstall(harnessesDir, name); err != nil {
			return err
		}

		fmt.Printf("Removed harness: %s\n", name)
		return nil
	},
}

var harnessValidateCmd = &cobra.Command{
	Use:   "validate [name]",
	Short: "Validate harness(es)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		harnessesDir := getHarnessesDir()

		// Discover all harnesses
		harnesses, err := harness.DiscoverHarnesses(harnessesDir)
		if err != nil {
			return err
		}

		// Filter by name if provided
		if len(args) == 1 {
			name := args[0]
			var found *harness.Harness
			for _, h := range harnesses {
				if h.Manifest.Name == name {
					found = h
					break
				}
			}
			if found == nil {
				return fmt.Errorf("harness not found: %s", name)
			}
			harnesses = []*harness.Harness{found}
		}

		if len(harnesses) == 0 {
			fmt.Println("No harnesses to validate.")
			return nil
		}

		// Validate and collect results
		hasErrors := false
		for _, h := range harnesses {
			errs := harness.ValidateHarness(h)
			if len(errs) == 0 {
				fmt.Printf("%s: OK\n", h.Manifest.Name)
			} else {
				fmt.Printf("%s:\n", h.Manifest.Name)
				for _, e := range errs {
					fmt.Printf("  [%s] %s: %s\n", e.Severity, e.Path, e.Message)
					if e.Severity == "error" {
						hasErrors = true
					}
				}
			}
		}

		if hasErrors {
			return fmt.Errorf("validation errors found")
		}

		return nil
	},
}

func init() {
	harnessCmd.AddCommand(harnessInstallCmd, harnessListCmd, harnessRemoveCmd, harnessValidateCmd)
	rootCmd.AddCommand(harnessCmd)
}

func getHarnessesDir() string {
	cwd, _ := os.Getwd()
	return filepath.Join(cwd, ".claudio", "harnesses")
}

func isGitURL(source string) bool {
	return strings.HasPrefix(source, "http://") ||
		strings.HasPrefix(source, "https://") ||
		strings.HasPrefix(source, "git@") ||
		strings.HasPrefix(source, "gh:")
}
