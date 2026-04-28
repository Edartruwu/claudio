package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Abraxas-365/claudio/internal/config"
)

var pluginCmd = &cobra.Command{
	Use:   "plugin",
	Short: "Manage Lua plugins",
}

var pluginInstallCmd = &cobra.Command{
	Use:   "install <source>",
	Short: "Install a Lua plugin from a local path or git URL",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		source := args[0]

		pluginsDir, err := luaPluginsDir()
		if err != nil {
			return err
		}

		if isGitURL(source) {
			return pluginInstallGit(source, pluginsDir)
		}
		return pluginInstallLocal(source, pluginsDir)
	},
}

var pluginListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed Lua plugins",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		pluginsDir, err := luaPluginsDir()
		if err != nil {
			return err
		}

		entries, err := os.ReadDir(pluginsDir)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("No plugins installed. Use 'claudio plugin install <path>' to add one.")
				return nil
			}
			return fmt.Errorf("failed to read plugins directory: %w", err)
		}

		type pluginEntry struct {
			name string
			path string
		}

		var plugins []pluginEntry
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			initPath := filepath.Join(pluginsDir, e.Name(), "init.lua")
			if _, err := os.Stat(initPath); err == nil {
				plugins = append(plugins, pluginEntry{
					name: e.Name(),
					path: filepath.Join(pluginsDir, e.Name()),
				})
			}
		}

		if len(plugins) == 0 {
			fmt.Println("No plugins installed. Use 'claudio plugin install <path>' to add one.")
			return nil
		}

		fmt.Printf("%-20s %s\n", "NAME", "PATH")
		fmt.Println(strings.Repeat("-", 80))
		for _, p := range plugins {
			fmt.Printf("%-20s %s\n", p.name, p.path)
		}

		return nil
	},
}

var pluginRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove an installed Lua plugin",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		pluginsDir, err := luaPluginsDir()
		if err != nil {
			return err
		}

		pluginDir := filepath.Join(pluginsDir, name)
		if _, err := os.Stat(pluginDir); os.IsNotExist(err) {
			return fmt.Errorf("plugin '%s' not found", name)
		}

		fmt.Printf("This will permanently remove plugin '%s'. Continue? [y/N]: ", name)
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(answer)

		if answer != "y" && answer != "Y" {
			fmt.Println("Aborted.")
			return nil
		}

		if err := os.RemoveAll(pluginDir); err != nil {
			return fmt.Errorf("failed to remove plugin: %w", err)
		}

		fmt.Printf("Removed plugin '%s'\n", name)
		return nil
	},
}

var pluginInfoCmd = &cobra.Command{
	Use:   "info <name>",
	Short: "Show information about an installed Lua plugin",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		pluginsDir, err := luaPluginsDir()
		if err != nil {
			return err
		}

		pluginDir := filepath.Join(pluginsDir, name)
		if _, err := os.Stat(pluginDir); os.IsNotExist(err) {
			return fmt.Errorf("plugin '%s' not found", name)
		}

		initPath := filepath.Join(pluginDir, "init.lua")
		_, initErr := os.Stat(initPath)
		initExists := initErr == nil

		fmt.Printf("Name:  %s\n", name)
		fmt.Printf("Path:  %s\n", pluginDir)
		if initExists {
			fmt.Printf("Entry: init.lua (exists)\n")
		} else {
			fmt.Printf("Entry: init.lua (missing)\n")
		}

		if initExists {
			fmt.Println()
			fmt.Println("--- init.lua preview (first 10 lines) ---")
			if err := printFirstLines(initPath, 10); err != nil {
				return fmt.Errorf("failed to read init.lua: %w", err)
			}
		}

		return nil
	},
}

func init() {
	pluginCmd.AddCommand(pluginInstallCmd, pluginListCmd, pluginRemoveCmd, pluginInfoCmd)
	rootCmd.AddCommand(pluginCmd)
}

// luaPluginsDir returns ~/.claudio/lua-plugins, creating it if needed.
func luaPluginsDir() (string, error) {
	dir := filepath.Join(config.GetPaths().Home, "lua-plugins")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create lua-plugins directory: %w", err)
	}
	return dir, nil
}

// pluginInstallLocal copies a local directory containing init.lua into pluginsDir.
func pluginInstallLocal(source, pluginsDir string) error {
	info, err := os.Stat(source)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("source '%s' is not a directory", source)
	}

	initPath := filepath.Join(source, "init.lua")
	if _, err := os.Stat(initPath); os.IsNotExist(err) {
		return fmt.Errorf("source directory has no init.lua")
	}

	name := filepath.Base(filepath.Clean(source))
	dest := filepath.Join(pluginsDir, name)

	if err := copyDir(source, dest); err != nil {
		return fmt.Errorf("failed to copy plugin: %w", err)
	}

	fmt.Printf("Installed plugin '%s'\n", name)
	return nil
}

// pluginInstallGit clones a git URL into pluginsDir.
func pluginInstallGit(url, pluginsDir string) error {
	gitBin, err := exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("git not found in PATH — install git to use remote plugins")
	}

	name := repoNameFromURL(url)
	dest := filepath.Join(pluginsDir, name)

	fmt.Fprintf(os.Stderr, "Warning: Installing from external source. Lua plugins may execute arbitrary code.\n")

	gitCmd := exec.Command(gitBin, "clone", url, dest)
	gitCmd.Stdout = os.Stdout
	gitCmd.Stderr = os.Stderr
	if err := gitCmd.Run(); err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}

	initPath := filepath.Join(dest, "init.lua")
	if _, err := os.Stat(initPath); os.IsNotExist(err) {
		_ = os.RemoveAll(dest)
		return fmt.Errorf("cloned repo has no init.lua")
	}

	fmt.Printf("Installed plugin '%s'\n", name)
	return nil
}

// repoNameFromURL extracts the repo name from a git URL.
func repoNameFromURL(url string) string {
	// Strip trailing slash
	url = strings.TrimRight(url, "/")

	// Get last path segment
	parts := strings.Split(url, "/")
	name := parts[len(parts)-1]

	// Handle git@ URLs: git@github.com:user/repo.git
	if idx := strings.LastIndex(name, ":"); idx >= 0 {
		name = name[idx+1:]
	}

	// Strip .git suffix
	name = strings.TrimSuffix(name, ".git")

	if name == "" {
		return "plugin"
	}
	return name
}

// copyDir recursively copies src directory to dst.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}

		return copyFile(path, target, info.Mode())
	})
}

// copyFile copies a single file from src to dst.
func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// printFirstLines prints up to n lines from a file.
func printFirstLines(path string, n int) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() && count < n {
		fmt.Println(scanner.Text())
		count++
	}
	return scanner.Err()
}
