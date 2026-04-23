package harness

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
)

// Harness represents a discovered, installed harness.
type Harness struct {
	Manifest *Manifest
	Dir      string // absolute path to installed harness root
}

// DiscoverHarnesses scans immediate subdirectories of harnessesDir for harness.json.
// Returns harnesses sorted by name. Returns empty slice if harnessesDir does not exist.
func DiscoverHarnesses(harnessesDir string) ([]*Harness, error) {
	entries, err := os.ReadDir(harnessesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*Harness{}, nil
		}
		return nil, fmt.Errorf("harness: read harnesses dir %s: %w", harnessesDir, err)
	}

	var harnesses []*Harness
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(harnessesDir, entry.Name())
		m, err := LoadManifest(dir)
		if err != nil {
			log.Printf("harness: skipping %s: %v", dir, err)
			continue
		}
		harnesses = append(harnesses, &Harness{Manifest: m, Dir: dir})
	}

	sort.Slice(harnesses, func(i, j int) bool {
		return harnesses[i].Manifest.Name < harnesses[j].Manifest.Name
	})

	if harnesses == nil {
		return []*Harness{}, nil
	}
	return harnesses, nil
}

// CollectAgentDirs aggregates all agent directories from all harnesses.
func CollectAgentDirs(harnesses []*Harness) []string {
	return collectDirs(harnesses, func(h *Harness) []string {
		return h.Manifest.AgentDirs(h.Dir)
	})
}

// CollectSkillDirs aggregates all skill directories from all harnesses.
func CollectSkillDirs(harnesses []*Harness) []string {
	return collectDirs(harnesses, func(h *Harness) []string {
		return h.Manifest.SkillDirs(h.Dir)
	})
}

// CollectPluginDirs aggregates all plugin directories from all harnesses.
func CollectPluginDirs(harnesses []*Harness) []string {
	return collectDirs(harnesses, func(h *Harness) []string {
		return h.Manifest.PluginDirs(h.Dir)
	})
}

// CollectTemplateDirs aggregates all template directories from all harnesses.
func CollectTemplateDirs(harnesses []*Harness) []string {
	return collectDirs(harnesses, func(h *Harness) []string {
		return h.Manifest.TemplateDirs(h.Dir)
	})
}

// CollectRulePaths aggregates all rule paths from all harnesses.
func CollectRulePaths(harnesses []*Harness) []string {
	return collectDirs(harnesses, func(h *Harness) []string {
		return h.Manifest.RulePaths(h.Dir)
	})
}

// CollectMCPServers merges MCP server configs from all harnesses.
// Returns an error if the same server name is declared by more than one harness.
func CollectMCPServers(harnesses []*Harness) (map[string]MCPServerConfig, error) {
	merged := make(map[string]MCPServerConfig)
	origin := make(map[string]string) // server name → harness name
	for _, h := range harnesses {
		for name, cfg := range h.Manifest.MCPServers {
			if prev, exists := origin[name]; exists {
				return nil, fmt.Errorf(
					"harness: MCP server %q declared by both %q and %q",
					name, prev, h.Manifest.Name,
				)
			}
			merged[name] = cfg
			origin[name] = h.Manifest.Name
		}
	}
	return merged, nil
}

// CollectToolFilters merges agent tool filters from all harnesses.
// Last harness wins per agent type.
func CollectToolFilters(harnesses []*Harness) map[string]AgentToolFilter {
	merged := make(map[string]AgentToolFilter)
	for _, h := range harnesses {
		for agentType, filter := range h.Manifest.AgentToolFilters {
			merged[agentType] = filter
		}
	}
	return merged
}

// collectDirs is a helper that runs fn for each harness and appends all results.
func collectDirs(harnesses []*Harness, fn func(*Harness) []string) []string {
	var out []string
	for _, h := range harnesses {
		out = append(out, fn(h)...)
	}
	return out
}
