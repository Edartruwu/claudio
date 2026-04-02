package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Settings represents the merged configuration from all sources.
type Settings struct {
	// AI model settings
	Model      string `json:"model,omitempty"`
	SmallModel string `json:"smallModel,omitempty"`

	// Permission settings
	PermissionMode string `json:"permissionMode,omitempty"` // "default", "auto", "headless"

	// Session settings
	AutoCompact    bool   `json:"autoCompact,omitempty"`
	CompactMode    string `json:"compactMode,omitempty"` // "auto", "manual", "strategic"
	SessionPersist bool   `json:"sessionPersist,omitempty"`

	// Security settings
	DenyPaths  []string `json:"denyPaths,omitempty"`
	DenyTools  []string `json:"denyTools,omitempty"`
	AllowPaths []string `json:"allowPaths,omitempty"`

	// Hook profiles
	HookProfile string `json:"hookProfile,omitempty"` // "minimal", "standard", "strict"

	// MCP servers
	MCPServers map[string]MCPServerConfig `json:"mcpServers,omitempty"`

	// API configuration
	APIBaseURL string `json:"apiBaseUrl,omitempty"`
	ProxyURL   string `json:"proxyUrl,omitempty"`

	// Token budget
	MaxBudget float64 `json:"maxBudget,omitempty"`
}

// MCPServerConfig defines an MCP server connection.
type MCPServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Type    string            `json:"type,omitempty"` // "stdio", "sse", "http"
	URL     string            `json:"url,omitempty"`
}

// Paths holds all relevant filesystem paths.
type Paths struct {
	Home        string // ~/.claudio/
	Settings    string // ~/.claudio/settings.json
	Local       string // ~/.claudio/local-settings.json
	Credentials string // ~/.claudio/credentials.json
	Sessions    string // ~/.claudio/sessions/
	Plugins     string // ~/.claudio/plugins/
	Skills      string // ~/.claudio/skills/
	Audit       string // ~/.claudio/audit/
	Contexts    string // ~/.claudio/contexts/
	Rules       string // ~/.claudio/rules/
	Agents      string // ~/.claudio/agents/
	Memory      string // ~/.claudio/memory/
	Logs        string // ~/.claudio/logs/
	Plans       string // ~/.claudio/plans/
	DB          string // ~/.claudio/claudio.db
	Instincts   string // ~/.claudio/instincts.json
}

var (
	paths     *Paths
	pathsOnce sync.Once
)

// GetPaths returns the standard filesystem paths for Claudio.
func GetPaths() *Paths {
	pathsOnce.Do(func() {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		base := filepath.Join(home, ".claudio")
		paths = &Paths{
			Home:        base,
			Settings:    filepath.Join(base, "settings.json"),
			Local:       filepath.Join(base, "local-settings.json"),
			Credentials: filepath.Join(base, "credentials.json"),
			Sessions:    filepath.Join(base, "sessions"),
			Plugins:     filepath.Join(base, "plugins"),
			Skills:      filepath.Join(base, "skills"),
			Audit:       filepath.Join(base, "audit"),
			Contexts:    filepath.Join(base, "contexts"),
			Rules:       filepath.Join(base, "rules"),
			Agents:      filepath.Join(base, "agents"),
			Memory:      filepath.Join(base, "memory"),
			Logs:        filepath.Join(base, "logs"),
			Plans:       filepath.Join(base, "plans"),
			DB:          filepath.Join(base, "claudio.db"),
			Instincts:   filepath.Join(base, "instincts.json"),
		}
	})
	return paths
}

// EnsureDirs creates all required directories and bootstraps default config files.
func EnsureDirs() error {
	p := GetPaths()
	dirs := []string{p.Home, p.Sessions, p.Plugins, p.Skills, p.Audit, p.Contexts, p.Rules, p.Agents, p.Memory, p.Logs, p.Plans}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}

	// Bootstrap default settings.json if it doesn't exist
	if _, err := os.Stat(p.Settings); os.IsNotExist(err) {
		defaults := DefaultSettings()
		data, _ := json.MarshalIndent(defaults, "", "  ")
		os.WriteFile(p.Settings, data, 0644)
	}

	return nil
}

// DefaultSettings returns settings with sensible defaults.
func DefaultSettings() *Settings {
	return &Settings{
		Model:          "claude-sonnet-4-6",
		SmallModel:     "claude-haiku-4-5-20251001",
		PermissionMode: "default",
		CompactMode:    "strategic",
		SessionPersist: true,
		HookProfile:    "standard",
		APIBaseURL:     "https://api.anthropic.com",
	}
}

// Load reads and merges settings from all sources.
// Priority (highest to lowest): env vars > project > local > user
func Load(projectDir string) (*Settings, error) {
	merged := DefaultSettings()

	p := GetPaths()

	// 1. User settings (~/.claudio/settings.json)
	mergeFromFile(merged, p.Settings)

	// 2. Local settings (~/.claudio/local-settings.json)
	mergeFromFile(merged, p.Local)

	// 3. Project settings (.claudio/settings.json)
	if projectDir != "" {
		projectSettings := filepath.Join(projectDir, ".claudio", "settings.json")
		mergeFromFile(merged, projectSettings)
	}

	// 4. Environment overrides
	applyEnvOverrides(merged)

	return merged, nil
}

func mergeFromFile(settings *Settings, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return // File doesn't exist or unreadable — skip
	}
	// Merge non-zero values from file into settings
	var overlay Settings
	if err := json.Unmarshal(data, &overlay); err != nil {
		return
	}
	if overlay.Model != "" {
		settings.Model = overlay.Model
	}
	if overlay.SmallModel != "" {
		settings.SmallModel = overlay.SmallModel
	}
	if overlay.PermissionMode != "" {
		settings.PermissionMode = overlay.PermissionMode
	}
	if overlay.CompactMode != "" {
		settings.CompactMode = overlay.CompactMode
	}
	if overlay.HookProfile != "" {
		settings.HookProfile = overlay.HookProfile
	}
	if overlay.APIBaseURL != "" {
		settings.APIBaseURL = overlay.APIBaseURL
	}
	if overlay.ProxyURL != "" {
		settings.ProxyURL = overlay.ProxyURL
	}
	if overlay.MaxBudget > 0 {
		settings.MaxBudget = overlay.MaxBudget
	}
	if len(overlay.DenyPaths) > 0 {
		settings.DenyPaths = append(settings.DenyPaths, overlay.DenyPaths...)
	}
	if len(overlay.AllowPaths) > 0 {
		settings.AllowPaths = append(settings.AllowPaths, overlay.AllowPaths...)
	}
	if len(overlay.MCPServers) > 0 {
		if settings.MCPServers == nil {
			settings.MCPServers = make(map[string]MCPServerConfig)
		}
		for k, v := range overlay.MCPServers {
			settings.MCPServers[k] = v
		}
	}
}

func applyEnvOverrides(settings *Settings) {
	if v := os.Getenv("CLAUDIO_MODEL"); v != "" {
		settings.Model = v
	}
	if v := os.Getenv("CLAUDIO_API_BASE_URL"); v != "" {
		settings.APIBaseURL = v
	}
	if v := os.Getenv("CLAUDIO_HOOK_PROFILE"); v != "" {
		settings.HookProfile = v
	}
}
