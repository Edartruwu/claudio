package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Settings represents the merged configuration from all sources.
type Settings struct {
	// AI model settings
	Model        string `json:"model,omitempty"`
	SmallModel   string `json:"smallModel,omitempty"`
	ThinkingMode string `json:"thinkingMode,omitempty"` // "adaptive", "enabled", "disabled", "" (auto)
	BudgetTokens int    `json:"budgetTokens,omitempty"` // thinking budget when mode is "enabled"
	EffortLevel  string `json:"effortLevel,omitempty"`  // "low", "medium", "high", "" (default=medium)

	// Permission settings
	PermissionMode string `json:"permissionMode,omitempty"` // "default", "auto", "headless"

	// Session settings
	AutoCompact    bool   `json:"autoCompact,omitempty"`
	CompactMode    string `json:"compactMode,omitempty"` // "auto", "manual", "strategic"
	SessionPersist bool   `json:"sessionPersist,omitempty"`

	// Memory settings
	AutoMemoryExtract *bool  `json:"autoMemoryExtract,omitempty"` // auto-extract memories on turn end (default: true)
	MemorySelection   string `json:"memorySelection,omitempty"`   // "ai" (Haiku), "keyword", "none" (default: "ai")

	// Security settings
	DenyPaths  []string `json:"denyPaths,omitempty"`
	DenyTools  []string `json:"denyTools,omitempty"`
	AllowPaths []string `json:"allowPaths,omitempty"`

	// Permission pattern rules (content-based allow/deny per tool)
	PermissionRules []PermissionRule `json:"permissionRules,omitempty"`

	// Output style
	OutputStyle string `json:"outputStyle,omitempty"` // "normal", "concise", "verbose", "markdown"

	// Cost threshold for confirmation dialog (USD, 0 = disabled)
	CostConfirmThreshold float64 `json:"costConfirmThreshold,omitempty"`

	// Hook profiles
	HookProfile string `json:"hookProfile,omitempty"` // "minimal", "standard", "strict"

	// MCP servers
	MCPServers map[string]MCPServerConfig `json:"mcpServers,omitempty"`

	// API configuration
	APIBaseURL string `json:"apiBaseUrl,omitempty"`
	ProxyURL   string `json:"proxyUrl,omitempty"`

	// Multi-provider configuration
	Providers    map[string]ProviderConfig `json:"providers,omitempty"`
	ModelRouting map[string]string         `json:"modelRouting,omitempty"` // glob pattern -> provider name

	// Token budget
	MaxBudget float64 `json:"maxBudget,omitempty"`
}

// ProviderConfig defines a non-default API provider.
type ProviderConfig struct {
	APIBase string            `json:"apiBase"`            // Base URL (e.g. "https://api.groq.com/openai/v1")
	APIKey  string            `json:"apiKey,omitempty"`   // API key or "$ENV_VAR" reference
	Type    string            `json:"type"`               // "openai" or "anthropic"
	Models  map[string]string `json:"models,omitempty"`   // shortcut -> model ID (e.g. "llama": "llama-3.3-70b-versatile")
}

// PermissionRule defines a content-pattern permission for a specific tool.
type PermissionRule struct {
	Tool     string `json:"tool"`     // tool name: "Bash", "Write", "*"
	Pattern  string `json:"pattern"`  // glob pattern matched against tool-specific content
	Behavior string `json:"behavior"` // "allow", "deny", "ask"
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
	Projects    string // ~/.claudio/projects/
	Logs        string // ~/.claudio/logs/
	Plans       string // ~/.claudio/plans/
	Cache       string // ~/.claudio/cache/
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
			Projects:    filepath.Join(base, "projects"),
			Logs:        filepath.Join(base, "logs"),
			Plans:       filepath.Join(base, "plans"),
			Cache:       filepath.Join(base, "cache"),
			DB:          filepath.Join(base, "claudio.db"),
			Instincts:   filepath.Join(base, "instincts.json"),
		}
	})
	return paths
}

// EnsureDirs creates all required directories and bootstraps default config files.
func EnsureDirs() error {
	p := GetPaths()
	dirs := []string{p.Home, p.Sessions, p.Plugins, p.Skills, p.Audit, p.Contexts, p.Rules, p.Agents, p.Memory, p.Projects, p.Logs, p.Plans, p.Cache}
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
	if overlay.ThinkingMode != "" {
		settings.ThinkingMode = overlay.ThinkingMode
	}
	if overlay.BudgetTokens > 0 {
		settings.BudgetTokens = overlay.BudgetTokens
	}
	if overlay.EffortLevel != "" {
		settings.EffortLevel = overlay.EffortLevel
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
	for _, r := range overlay.PermissionRules {
		dup := false
		for _, existing := range settings.PermissionRules {
			if existing.Tool == r.Tool && existing.Pattern == r.Pattern && existing.Behavior == r.Behavior {
				dup = true
				break
			}
		}
		if !dup {
			settings.PermissionRules = append(settings.PermissionRules, r)
		}
	}
	if overlay.OutputStyle != "" {
		settings.OutputStyle = overlay.OutputStyle
	}
	if overlay.CostConfirmThreshold > 0 {
		settings.CostConfirmThreshold = overlay.CostConfirmThreshold
	}
	if overlay.AutoMemoryExtract != nil {
		settings.AutoMemoryExtract = overlay.AutoMemoryExtract
	}
	if overlay.MemorySelection != "" {
		settings.MemorySelection = overlay.MemorySelection
	}
	if len(overlay.DenyTools) > 0 {
		settings.DenyTools = append(settings.DenyTools, overlay.DenyTools...)
	}
	if len(overlay.Providers) > 0 {
		if settings.Providers == nil {
			settings.Providers = make(map[string]ProviderConfig)
		}
		for k, v := range overlay.Providers {
			settings.Providers[k] = v
		}
	}
	if len(overlay.ModelRouting) > 0 {
		if settings.ModelRouting == nil {
			settings.ModelRouting = make(map[string]string)
		}
		for k, v := range overlay.ModelRouting {
			settings.ModelRouting[k] = v
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

// SanitizeProjectPath converts a filesystem path into a safe directory name.
// e.g. "/Users/abraxas/Personal/claudio" -> "users-abraxas-personal-claudio"
func SanitizeProjectPath(path string) string {
	path = strings.ToLower(filepath.Clean(path))
	path = strings.TrimPrefix(path, "/")
	path = strings.ReplaceAll(path, "/", "-")
	path = strings.ReplaceAll(path, "\\", "-")
	path = strings.ReplaceAll(path, " ", "-")
	// Remove any double dashes
	for strings.Contains(path, "--") {
		path = strings.ReplaceAll(path, "--", "-")
	}
	return path
}

// ProjectMemoryDir returns the memory directory for a specific project.
// Uses the git root (or cwd) to create a stable, project-scoped path.
// IsAutoMemoryExtract returns whether automatic memory extraction is enabled.
func (s *Settings) IsAutoMemoryExtract() bool {
	if s.AutoMemoryExtract == nil {
		return true // default: enabled
	}
	return *s.AutoMemoryExtract
}

// GetMemorySelection returns the memory selection strategy ("ai", "keyword", "none").
func (s *Settings) GetMemorySelection() string {
	if s.MemorySelection == "" {
		return "ai" // default
	}
	return s.MemorySelection
}

func ProjectMemoryDir(projectRoot string) string {
	p := GetPaths()
	slug := SanitizeProjectPath(projectRoot)
	return filepath.Join(p.Projects, slug, "memory")
}
