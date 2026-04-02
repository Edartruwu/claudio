// Package plugins provides plugin discovery and loading for Claudio.
// Plugins are executable scripts or binaries in ~/.claudio/plugins/ that
// extend Claudio with custom commands and tools.
package plugins

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Plugin represents a discovered plugin.
type Plugin struct {
	Name        string // Derived from filename
	Path        string // Absolute path to the executable
	Description string // From --describe flag or first line of output
	IsScript    bool   // True if it's a script (not compiled binary)
}

// Registry holds all discovered plugins.
type Registry struct {
	plugins []*Plugin
}

// NewRegistry creates an empty plugin registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// LoadDir discovers plugins in the given directory.
// Plugins are executable files (scripts or binaries).
func (r *Registry) LoadDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading plugins dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Check if executable
		if info.Mode()&0111 == 0 {
			continue
		}

		name := entry.Name()
		// Strip common extensions
		for _, ext := range []string{".sh", ".bash", ".py", ".rb", ".js"} {
			name = strings.TrimSuffix(name, ext)
		}

		plugin := &Plugin{
			Name:     name,
			Path:     path,
			IsScript: !isCompiledBinary(path),
		}

		// Try to get description
		plugin.Description = getPluginDescription(path)

		r.plugins = append(r.plugins, plugin)
	}

	return nil
}

// All returns all discovered plugins.
func (r *Registry) All() []*Plugin {
	return r.plugins
}

// Get returns a plugin by name.
func (r *Registry) Get(name string) *Plugin {
	for _, p := range r.plugins {
		if p.Name == name {
			return p
		}
	}
	return nil
}

// Execute runs a plugin with the given arguments.
func (p *Plugin) Execute(args ...string) (string, error) {
	cmd := exec.Command(p.Path, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("plugin %s failed: %w", p.Name, err)
	}
	return string(output), nil
}

// FormatList returns a formatted string listing all plugins.
func (r *Registry) FormatList() string {
	if len(r.plugins) == 0 {
		return "No plugins installed."
	}

	var sb strings.Builder
	sb.WriteString("Installed plugins:\n")
	for _, p := range r.plugins {
		desc := p.Description
		if desc == "" {
			desc = "(no description)"
		}
		sb.WriteString(fmt.Sprintf("  %-20s %s\n", p.Name, desc))
	}
	return sb.String()
}

func getPluginDescription(path string) string {
	cmd := exec.Command(path, "--describe")
	output, err := cmd.Output()
	if err == nil && len(output) > 0 {
		// Use first line
		lines := strings.SplitN(string(output), "\n", 2)
		return strings.TrimSpace(lines[0])
	}
	return ""
}

func isCompiledBinary(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	// Read first few bytes to check for magic numbers
	buf := make([]byte, 4)
	n, err := f.Read(buf)
	if err != nil || n < 4 {
		return false
	}

	// ELF magic
	if buf[0] == 0x7f && buf[1] == 'E' && buf[2] == 'L' && buf[3] == 'F' {
		return true
	}
	// Mach-O magic (macOS)
	if buf[0] == 0xfe && buf[1] == 0xed && buf[2] == 0xfa {
		return true
	}
	if buf[0] == 0xcf && buf[1] == 0xfa && buf[2] == 0xed && buf[3] == 0xfe {
		return true
	}

	return false
}
