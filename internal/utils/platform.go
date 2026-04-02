package utils

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Platform returns the current OS name.
func Platform() string {
	return runtime.GOOS
}

// IsMacOS returns true if running on macOS.
func IsMacOS() bool {
	return runtime.GOOS == "darwin"
}

// IsLinux returns true if running on Linux.
func IsLinux() bool {
	return runtime.GOOS == "linux"
}

// IsWindows returns true if running on Windows.
func IsWindows() bool {
	return runtime.GOOS == "windows"
}

// Arch returns the CPU architecture.
func Arch() string {
	return runtime.GOARCH
}

// HomeDir returns the user's home directory.
func HomeDir() string {
	home, _ := os.UserHomeDir()
	return home
}

// ExpandHome expands ~ to the user's home directory in a path.
func ExpandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home := HomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

// Which finds an executable in PATH (like the `which` command).
func Which(name string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return path
}

// IsExecutableAvailable checks if a command exists in PATH.
func IsExecutableAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// DefaultShell returns the user's default shell.
func DefaultShell() string {
	shell := os.Getenv("SHELL")
	if shell != "" {
		return shell
	}
	if IsWindows() {
		return "cmd.exe"
	}
	return "/bin/sh"
}

// ShellName returns just the name of the shell (e.g., "zsh", "bash").
func ShellName() string {
	shell := DefaultShell()
	return filepath.Base(shell)
}

// TempDir returns the system temp directory.
func TempDir() string {
	return os.TempDir()
}

// XDGDataHome returns the XDG data directory.
func XDGDataHome() string {
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return dir
	}
	return filepath.Join(HomeDir(), ".local", "share")
}

// XDGConfigHome returns the XDG config directory.
func XDGConfigHome() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return dir
	}
	return filepath.Join(HomeDir(), ".config")
}

// XDGCacheHome returns the XDG cache directory.
func XDGCacheHome() string {
	if dir := os.Getenv("XDG_CACHE_HOME"); dir != "" {
		return dir
	}
	return filepath.Join(HomeDir(), ".cache")
}
