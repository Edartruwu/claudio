package lua

import (
	"os"
	"path/filepath"
)

// pluginDir represents a discovered plugin directory.
type pluginDir struct {
	name string
	dir  string
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
