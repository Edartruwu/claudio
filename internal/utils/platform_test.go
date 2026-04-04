package utils

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPlatform(t *testing.T) {
	got := Platform()
	if got != runtime.GOOS {
		t.Errorf("Platform() = %q, want %q", got, runtime.GOOS)
	}
}

func TestIsMacOS(t *testing.T) {
	want := runtime.GOOS == "darwin"
	if got := IsMacOS(); got != want {
		t.Errorf("IsMacOS() = %v, want %v", got, want)
	}
}

func TestIsLinux(t *testing.T) {
	want := runtime.GOOS == "linux"
	if got := IsLinux(); got != want {
		t.Errorf("IsLinux() = %v, want %v", got, want)
	}
}

func TestIsWindows(t *testing.T) {
	want := runtime.GOOS == "windows"
	if got := IsWindows(); got != want {
		t.Errorf("IsWindows() = %v, want %v", got, want)
	}
}

func TestArch(t *testing.T) {
	got := Arch()
	if got != runtime.GOARCH {
		t.Errorf("Arch() = %q, want %q", got, runtime.GOARCH)
	}
}

func TestHomeDir(t *testing.T) {
	expected, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir:", err)
	}
	if got := HomeDir(); got != expected {
		t.Errorf("HomeDir() = %q, want %q", got, expected)
	}
}

func TestExpandHome_TildePrefix(t *testing.T) {
	home := HomeDir()
	got := ExpandHome("~/foo/bar")
	want := filepath.Join(home, "foo/bar")
	if got != want {
		t.Errorf("ExpandHome(~/foo/bar) = %q, want %q", got, want)
	}
}

func TestExpandHome_NoTilde(t *testing.T) {
	input := "/absolute/path"
	if got := ExpandHome(input); got != input {
		t.Errorf("ExpandHome(%q) = %q, want %q", input, got, input)
	}
}

func TestExpandHome_TildeOnly(t *testing.T) {
	// "~" alone — no trailing slash, should not be expanded
	got := ExpandHome("~")
	if got != "~" {
		t.Errorf("ExpandHome(~) = %q, want %q", got, "~")
	}
}

func TestWhich_KnownBinary(t *testing.T) {
	// "sh" is virtually guaranteed to exist everywhere
	got := Which("sh")
	if got == "" {
		t.Skip("sh not found on this system")
	}
	if !strings.Contains(got, "sh") {
		t.Errorf("Which(sh) = %q, expected to contain 'sh'", got)
	}
}

func TestWhich_UnknownBinary(t *testing.T) {
	got := Which("__nonexistent_binary_xyz__")
	if got != "" {
		t.Errorf("Which(__nonexistent__) = %q, want empty string", got)
	}
}

func TestIsExecutableAvailable_Known(t *testing.T) {
	if !IsExecutableAvailable("sh") {
		t.Skip("sh not available on this system")
	}
}

func TestIsExecutableAvailable_Unknown(t *testing.T) {
	if IsExecutableAvailable("__nonexistent_binary_xyz__") {
		t.Error("expected IsExecutableAvailable to return false for unknown binary")
	}
}

func TestDefaultShell(t *testing.T) {
	got := DefaultShell()
	if got == "" {
		t.Error("DefaultShell() returned empty string")
	}
}

func TestDefaultShell_FromEnv(t *testing.T) {
	t.Setenv("SHELL", "/usr/bin/zsh")
	got := DefaultShell()
	if got != "/usr/bin/zsh" {
		t.Errorf("DefaultShell() = %q, want /usr/bin/zsh", got)
	}
}

func TestDefaultShell_FallbackWhenUnset(t *testing.T) {
	// Temporarily unset SHELL
	orig := os.Getenv("SHELL")
	os.Unsetenv("SHELL")
	defer func() {
		if orig != "" {
			os.Setenv("SHELL", orig)
		}
	}()

	got := DefaultShell()
	if got == "" {
		t.Error("DefaultShell() returned empty string when SHELL unset")
	}
}

func TestShellName(t *testing.T) {
	t.Setenv("SHELL", "/usr/bin/bash")
	got := ShellName()
	if got != "bash" {
		t.Errorf("ShellName() = %q, want %q", got, "bash")
	}
}

func TestTempDir(t *testing.T) {
	got := TempDir()
	if got == "" {
		t.Error("TempDir() returned empty string")
	}
	if got != os.TempDir() {
		t.Errorf("TempDir() = %q, want %q", got, os.TempDir())
	}
}

func TestXDGDataHome_FromEnv(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/custom/data")
	got := XDGDataHome()
	if got != "/custom/data" {
		t.Errorf("XDGDataHome() = %q, want /custom/data", got)
	}
}

func TestXDGDataHome_Default(t *testing.T) {
	os.Unsetenv("XDG_DATA_HOME")
	got := XDGDataHome()
	want := filepath.Join(HomeDir(), ".local", "share")
	if got != want {
		t.Errorf("XDGDataHome() = %q, want %q", got, want)
	}
}

func TestXDGConfigHome_FromEnv(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/config")
	got := XDGConfigHome()
	if got != "/custom/config" {
		t.Errorf("XDGConfigHome() = %q, want /custom/config", got)
	}
}

func TestXDGConfigHome_Default(t *testing.T) {
	os.Unsetenv("XDG_CONFIG_HOME")
	got := XDGConfigHome()
	want := filepath.Join(HomeDir(), ".config")
	if got != want {
		t.Errorf("XDGConfigHome() = %q, want %q", got, want)
	}
}

func TestXDGCacheHome_FromEnv(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "/custom/cache")
	got := XDGCacheHome()
	if got != "/custom/cache" {
		t.Errorf("XDGCacheHome() = %q, want /custom/cache", got)
	}
}

func TestXDGCacheHome_Default(t *testing.T) {
	os.Unsetenv("XDG_CACHE_HOME")
	got := XDGCacheHome()
	want := filepath.Join(HomeDir(), ".cache")
	if got != want {
		t.Errorf("XDGCacheHome() = %q, want %q", got, want)
	}
}
