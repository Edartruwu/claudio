package lua

import (
	"fmt"
	"os"
	"path/filepath"
)

// pluginDir represents a discovered plugin directory.
type pluginDir struct {
	name string
	dir  string
}

// LoadUserInit loads the Lua file at path if it exists. If the file is absent,
// nil is returned (missing user init is not an error). Any parse or runtime
// error in the file is returned to the caller.
func (r *Runtime) LoadUserInit(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("lua: read %s: %w", path, err)
	}
	return r.execString(string(data), path)
}

// discoverPlugins scans baseDir for subdirectories containing init.lua.
// Returns the list of discovered plugins sorted by directory name.
func discoverPlugins(baseDir string) ([]pluginDir, error) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no plugins dir = no plugins
		}
		return nil, err
	}

	var result []pluginDir
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		dir := filepath.Join(baseDir, name)
		initFile := filepath.Join(dir, "init.lua")
		if _, err := os.Stat(initFile); err == nil {
			result = append(result, pluginDir{name: name, dir: dir})
		}
	}
	return result, nil
}
