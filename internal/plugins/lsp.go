package plugins

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/Abraxas-365/claudio/internal/config"
)

// LoadLspConfigs discovers *.lsp.json files in the plugins directory
// and returns the merged LSP server configs.
// Each file should contain a JSON object mapping server names to LspServerConfig.
func LoadLspConfigs(pluginDir string) map[string]config.LspServerConfig {
	result := make(map[string]config.LspServerConfig)

	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		return result
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		// Match *.lsp.json
		base := entry.Name()
		if len(base) < len(".lsp.json") || base[len(base)-len(".lsp.json"):] != ".lsp.json" {
			continue
		}

		path := filepath.Join(pluginDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var servers map[string]config.LspServerConfig
		if err := json.Unmarshal(data, &servers); err != nil {
			continue
		}

		for k, v := range servers {
			result[k] = v
		}
	}

	return result
}
