package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// ManifestFile is the filename expected inside each harness directory.
const ManifestFile = "harness.json"

// validName matches lowercase alphanumeric strings with optional hyphens.
var validName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// MCPServerConfig mirrors config.MCPServerConfig without importing internal/config.
// The wiring layer (app.go) is responsible for converting between the two.
type MCPServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Type    string            `json:"type,omitempty"`
	URL     string            `json:"url,omitempty"`
}

// AgentToolFilter restricts or expands tools available to an agent type.
type AgentToolFilter struct {
	AllowedMCPTools []string `json:"allowed_mcp_tools,omitempty"`
	DisallowedTools []string `json:"disallowed_tools,omitempty"`
}

// Manifest is the parsed contents of harness.json.
type Manifest struct {
	Name             string                     `json:"name"`
	Version          string                     `json:"version"`
	Description      string                     `json:"description,omitempty"`
	Author           string                     `json:"author,omitempty"`
	MinClaudioVer    string                     `json:"min_claudio_version,omitempty"`
	Agents           []string                   `json:"agents,omitempty"`
	Skills           []string                   `json:"skills,omitempty"`
	Plugins          []string                   `json:"plugins,omitempty"`
	Templates        []string                   `json:"templates,omitempty"`
	Rules            []string                   `json:"rules,omitempty"`
	MCPServers       map[string]MCPServerConfig  `json:"mcp_servers,omitempty"`
	AgentToolFilters map[string]AgentToolFilter  `json:"agent_tool_filters,omitempty"`
}

// LoadManifest reads harness.json from dir and parses it.
func LoadManifest(dir string) (*Manifest, error) {
	path := filepath.Join(dir, ManifestFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("harness: read manifest %s: %w", path, err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("harness: parse manifest %s: %w", path, err)
	}
	return &m, nil
}

// Validate checks required fields and format constraints.
func (m *Manifest) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("harness: manifest missing required field 'name'")
	}
	if !validName.MatchString(m.Name) {
		return fmt.Errorf("harness: invalid name %q (must match ^[a-z0-9]+(-[a-z0-9]+)*$)", m.Name)
	}
	if m.Version == "" {
		return fmt.Errorf("harness: manifest %q missing required field 'version'", m.Name)
	}
	return nil
}

// AgentDirs resolves agent directories relative to base.
// Defaults to ["agents/"] when the Agents field is empty.
func (m *Manifest) AgentDirs(base string) []string {
	return resolveDirs(base, m.Agents, "agents")
}

// SkillDirs resolves skill directories relative to base.
// Defaults to ["skills/"] when the Skills field is empty.
func (m *Manifest) SkillDirs(base string) []string {
	return resolveDirs(base, m.Skills, "skills")
}

// PluginDirs resolves plugin directories relative to base.
// Defaults to ["plugins/"] when the Plugins field is empty.
func (m *Manifest) PluginDirs(base string) []string {
	return resolveDirs(base, m.Plugins, "plugins")
}

// TemplateDirs resolves team-template directories relative to base.
// Defaults to ["team-templates/"] when the Templates field is empty.
func (m *Manifest) TemplateDirs(base string) []string {
	return resolveDirs(base, m.Templates, "team-templates")
}

// RulePaths resolves rule file paths relative to base.
// Defaults to ["rules/"] when the Rules field is empty.
func (m *Manifest) RulePaths(base string) []string {
	return resolveDirs(base, m.Rules, "rules")
}

// resolveDirs joins each entry in paths against base; uses defaultDir if paths is empty.
func resolveDirs(base string, paths []string, defaultDir string) []string {
	if len(paths) == 0 {
		return []string{filepath.Join(base, defaultDir)}
	}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		out = append(out, filepath.Join(base, p))
	}
	return out
}
