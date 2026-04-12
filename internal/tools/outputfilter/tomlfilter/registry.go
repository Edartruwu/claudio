package tomlfilter

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
)

// Registry holds compiled filters loaded from TOML files. Filters are matched
// in order; the first filter whose match_command regex matches wins.
type Registry struct {
	filters []compiledFilter
}

var (
	defaultRegistry *Registry
	registryOnce    sync.Once

	// builtinFS is set by the outputfilter package at init time via SetBuiltinFS.
	builtinFS   fs.FS
	builtinOnce sync.Once
)

// SetBuiltinFS sets the embedded filesystem containing built-in filter TOMLs.
// Must be called before DefaultRegistry() is first accessed.
func SetBuiltinFS(f fs.FS) {
	builtinOnce.Do(func() {
		builtinFS = f
	})
}

// DefaultRegistry returns the singleton registry, initializing it on first call.
// Load priority (first match wins): project-local → user-global → built-in.
func DefaultRegistry() *Registry {
	registryOnce.Do(func() {
		defaultRegistry = newRegistry()
	})
	return defaultRegistry
}

// NewRegistryFromTOML creates a registry from a single TOML string. This is
// useful for testing.
func NewRegistryFromTOML(data string) (*Registry, error) {
	r := &Registry{}
	if err := r.loadFromString(data); err != nil {
		return nil, err
	}
	return r, nil
}

// Apply tries each filter in order. The first filter whose match_command regex
// matches the command runs the 8-stage pipeline and returns the result.
// Returns ("", false) if no filter matches or if CLAUDIO_NO_FILTER=1.
func (r *Registry) Apply(command, output string) (string, bool) {
	if os.Getenv("CLAUDIO_NO_FILTER") == "1" {
		return "", false
	}

	debug := os.Getenv("CLAUDIO_FILTER_DEBUG") == "1"

	for i := range r.filters {
		cf := &r.filters[i]
		if cf.commandRe.MatchString(command) {
			linesBefore := strings.Count(output, "\n") + 1
			result := cf.apply(output)
			if debug {
				linesAfter := strings.Count(result, "\n") + 1
				fmt.Fprintf(os.Stderr, "[claudio-filter] matched %q (%d\u2192%d lines)\n",
					cf.name, linesBefore, linesAfter)
			}
			return result, true
		}
	}

	return "", false
}

func newRegistry() *Registry {
	r := &Registry{}

	// Priority 1: project-local .claudio/filters.toml
	r.loadFile(filepath.Join(".claudio", "filters.toml"))

	// Priority 2: user-global ~/.config/claudio/filters.toml
	if configDir, err := os.UserConfigDir(); err == nil {
		r.loadFile(filepath.Join(configDir, "claudio", "filters.toml"))
	}

	// Priority 3: built-in embedded filters
	if builtinFS != nil {
		r.loadFromFS(builtinFS)
	}

	return r
}

func (r *Registry) loadFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return // file doesn't exist, silently skip
	}

	if err := r.loadFromString(string(data)); err != nil {
		fmt.Fprintf(os.Stderr, "[claudio-filter] warning: %s: %v\n", path, err)
	}
}

func (r *Registry) loadFromFS(f fs.FS) {
	entries, err := fs.ReadDir(f, ".")
	if err != nil {
		// Try reading from "filters" subdirectory (embed.FS preserves directory structure)
		entries, err = fs.ReadDir(f, "filters")
		if err != nil {
			return
		}
		// Wrap to read from subdirectory
		sub, subErr := fs.Sub(f, "filters")
		if subErr != nil {
			return
		}
		f = sub
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}
		data, err := fs.ReadFile(f, entry.Name())
		if err != nil {
			fmt.Fprintf(os.Stderr, "[claudio-filter] warning: embedded %s: %v\n", entry.Name(), err)
			continue
		}
		if err := r.loadFromString(string(data)); err != nil {
			fmt.Fprintf(os.Stderr, "[claudio-filter] warning: embedded %s: %v\n", entry.Name(), err)
		}
	}
}

func (r *Registry) loadFromString(data string) error {
	var ff filterFile
	if _, err := toml.Decode(data, &ff); err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	if ff.SchemaVersion != 1 {
		return fmt.Errorf("unsupported schema_version %d (expected 1)", ff.SchemaVersion)
	}

	for name, def := range ff.Filters {
		// Reject mutually exclusive strip/keep
		if len(def.StripLinesMatching) > 0 && len(def.KeepLinesMatching) > 0 {
			fmt.Fprintf(os.Stderr, "[claudio-filter] warning: filter %q has both strip_lines_matching and keep_lines_matching; skipping\n", name)
			continue
		}

		cf, err := compile(name, def)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[claudio-filter] warning: filter %q: %v; skipping\n", name, err)
			continue
		}

		r.filters = append(r.filters, cf)
	}

	return nil
}
