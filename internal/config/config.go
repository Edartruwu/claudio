package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Abraxas-365/claudio/internal/snippets"
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
	CompactMode    string `json:"compactMode,omitempty"`    // "auto", "manual", "strategic"
	CompactKeepN   int    `json:"compactKeepN,omitempty"`   // number of recent messages to keep after compaction (default 10)
	SessionPersist bool   `json:"sessionPersist,omitempty"`

	// Memory settings
	AutoMemoryExtract       *bool  `json:"autoMemoryExtract,omitempty"`       // auto-extract memories on turn end (default: false)
	MemorySelection         string `json:"memorySelection,omitempty"`         // "ai" (Haiku), "keyword", "none" (default: "none")
	MemoryIndexTTLDays      *int   `json:"memoryIndexTTLDays,omitempty"`      // filter out entries older than N days from inline index (default: 30)
	MemoryRefreshOnCompact  *bool  `json:"memoryRefreshOnCompact,omitempty"`  // rebuild memory index after compaction (default: true)

	// Security settings
	DenyPaths  []string `json:"denyPaths,omitempty"`
	DenyTools  []string `json:"denyTools,omitempty"`
	AllowPaths []string `json:"allowPaths,omitempty"`

	// Permission pattern rules (content-based allow/deny per tool)
	PermissionRules []PermissionRule `json:"permissionRules,omitempty"`

	// Editor command template for opening files (e.g. "nvim -c 'Gvdiffsplit ORIG_HEAD' {file}")
	// {file} is replaced with the file path. Falls back to $VISUAL/$EDITOR if empty.
	EditorCmd string `json:"editorCmd,omitempty"`

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
	PublicURL  string `json:"publicUrl,omitempty"`

	// Multi-provider configuration
	Providers    map[string]ProviderConfig `json:"providers,omitempty"`
	ModelRouting map[string]string         `json:"modelRouting,omitempty"` // glob pattern -> provider name

	// Token budget
	MaxBudget float64 `json:"maxBudget,omitempty"`

	// Output filter (RTK-style token reduction for command output)
	OutputFilter bool `json:"outputFilter,omitempty"`

	// CodeFilterLevel controls comment-stripping when reading source files
	// longer than 500 lines. Values: "none" (default), "minimal", "aggressive".
	CodeFilterLevel string `json:"codeFilterLevel,omitempty"`

	// Snippet expansion (AI writes shorthand, expander fills boilerplate)
	Snippets *snippets.Config `json:"snippets,omitempty"`

	// LSP servers (config-driven, no hardcoded defaults)
	LspServers map[string]LspServerConfig `json:"lspServers,omitempty"`

	// Sidebar configuration
	Sidebar *SidebarConfig `json:"sidebar,omitempty"`

	// Advisor settings (nil = advisor is off)
	Advisor *AdvisorSettings `json:"advisor,omitempty"`

	// AgentAutoDeleteAfter controls how many human messages of inactivity cause
	// a done agent to be removed from memory. Default: 3. Set to -1 to never
	// auto-delete.
	AgentAutoDeleteAfter int `json:"agent_auto_delete_after,omitempty"`

	// Caveman enables ultra-compressed communication mode for all agents.
	Caveman *bool `json:"caveman,omitempty"`

	// Design configuration
	Design DesignConfig `json:"design,omitempty"`
}

// DesignConfig holds configuration for the Claudio Design agent skills.
// EnabledSkills is a whitelist — if non-empty, only these design skills are available.
// DisabledSkills is a deny list — these design skills are hidden. Ignored if EnabledSkills is set.
type DesignConfig struct {
	EnabledSkills  []string `json:"enabledSkills,omitempty"`
	DisabledSkills []string `json:"disabledSkills,omitempty"`
}

// SidebarConfig controls the persistent right-side panel.
type SidebarConfig struct {
	Enabled bool     `json:"enabled"`
	Width   int      `json:"width,omitempty"` // default 32
	Blocks  []string `json:"blocks,omitempty"` // e.g. ["files","todos","tokens"]
}

// AdvisorSettings configures the advisor agent.
type AdvisorSettings struct {
	SubagentType string `json:"subagentType,omitempty"`
	Model        string `json:"model,omitempty"`
	MaxUses      int    `json:"maxUses,omitempty"` // 0 = unlimited
}

// LspServerConfig defines an LSP server connection.
type LspServerConfig struct {
	Command    string            `json:"command"`
	Args       []string          `json:"args,omitempty"`
	Extensions []string          `json:"extensions"`         // file extensions this server handles (e.g. [".go", ".mod"])
	Env        map[string]string `json:"env,omitempty"`
}

// ProviderConfig defines a non-default API provider.
type ProviderConfig struct {
	APIBase       string            `json:"apiBase"`                 // Base URL (e.g. "https://api.groq.com/openai/v1")
	APIKey        string            `json:"apiKey,omitempty"`        // API key or "$ENV_VAR" reference
	Type          string            `json:"type"`                    // "openai", "anthropic", or "ollama"
	Models        map[string]string `json:"models,omitempty"`        // shortcut -> model ID (e.g. "llama": "llama-3.3-70b-versatile")
	ContextWindow int               `json:"contextWindow,omitempty"` // Ollama num_ctx override (default 2048 is too small)
	NoToolsModels []string          `json:"noToolsModels,omitempty"` // Ollama: glob patterns for models that don't support tool calling (e.g. ["deepseek-r1*"])
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
	Agents        string // ~/.claudio/agents/
	TeamTemplates string // ~/.claudio/team-templates/
	Memory        string // ~/.claudio/memory/
	Projects    string // ~/.claudio/projects/
	Logs        string // ~/.claudio/logs/
	Plans       string // ~/.claudio/plans/
	Cache       string // ~/.claudio/cache/
	Designs     string // ~/.claudio/designs/
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
			Agents:        filepath.Join(base, "agents"),
			TeamTemplates: filepath.Join(base, "team-templates"),
			Memory:        filepath.Join(base, "memory"),
			Projects:    filepath.Join(base, "projects"),
			Logs:        filepath.Join(base, "logs"),
			Plans:       filepath.Join(base, "plans"),
			Cache:       filepath.Join(base, "cache"),
			Designs:     filepath.Join(base, "designs"),
			DB:          filepath.Join(base, "claudio.db"),
			Instincts:   filepath.Join(base, "instincts.json"),
		}
	})
	return paths
}

// EnsureDirs creates all required directories and bootstraps default config files.
func EnsureDirs() error {
	p := GetPaths()
	dirs := []string{p.Home, p.Sessions, p.Plugins, p.Skills, p.Audit, p.Contexts, p.Rules, p.Agents, p.TeamTemplates, p.Memory, p.Projects, p.Logs, p.Plans, p.Cache, p.Designs}
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
		Model:                "claude-sonnet-4-6",
		SmallModel:           "claude-haiku-4-5-20251001",
		PermissionMode:       "default",
		CompactMode:          "strategic",
		SessionPersist:       true,
		HookProfile:          "standard",
		APIBaseURL:           "https://api.anthropic.com",
		AgentAutoDeleteAfter: 3,
		CodeFilterLevel:      "none",
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
	if overlay.CompactKeepN > 0 {
		settings.CompactKeepN = overlay.CompactKeepN
	}
	if overlay.HookProfile != "" {
		settings.HookProfile = overlay.HookProfile
	}
	if overlay.APIBaseURL != "" {
		settings.APIBaseURL = overlay.APIBaseURL
	}
	if overlay.PublicURL != "" {
		settings.PublicURL = overlay.PublicURL
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
	if overlay.CodeFilterLevel != "" {
		settings.CodeFilterLevel = overlay.CodeFilterLevel
	}
	if overlay.EditorCmd != "" {
		settings.EditorCmd = overlay.EditorCmd
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
	if overlay.MemoryIndexTTLDays != nil {
		settings.MemoryIndexTTLDays = overlay.MemoryIndexTTLDays
	}
	if overlay.MemoryRefreshOnCompact != nil {
		settings.MemoryRefreshOnCompact = overlay.MemoryRefreshOnCompact
	}
	if len(overlay.DenyTools) > 0 {
		settings.DenyTools = append(settings.DenyTools, overlay.DenyTools...)
	}
	if len(overlay.Design.EnabledSkills) > 0 {
		settings.Design.EnabledSkills = append(settings.Design.EnabledSkills, overlay.Design.EnabledSkills...)
	}
	if len(overlay.Design.DisabledSkills) > 0 {
		settings.Design.DisabledSkills = append(settings.Design.DisabledSkills, overlay.Design.DisabledSkills...)
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
	// Use raw JSON to detect explicit outputFilter presence (handles both true and false)
	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) == nil {
		if _, ok := raw["outputFilter"]; ok {
			settings.OutputFilter = overlay.OutputFilter
		}
	}
	if len(overlay.LspServers) > 0 {
		if settings.LspServers == nil {
			settings.LspServers = make(map[string]LspServerConfig)
		}
		for k, v := range overlay.LspServers {
			settings.LspServers[k] = v
		}
	}
	if overlay.Sidebar != nil {
		settings.Sidebar = overlay.Sidebar
	}
	if overlay.Advisor != nil {
		settings.Advisor = overlay.Advisor
	}
	if overlay.AgentAutoDeleteAfter != 0 {
		settings.AgentAutoDeleteAfter = overlay.AgentAutoDeleteAfter
	}
	if overlay.Snippets != nil {
		if settings.Snippets == nil {
			settings.Snippets = overlay.Snippets
		} else {
			// Project-level overrides the enabled flag
			settings.Snippets.Enabled = overlay.Snippets.Enabled
			// Project-level snippets extend global ones
			settings.Snippets.Snippets = append(settings.Snippets.Snippets, overlay.Snippets.Snippets...)
		}
	}
	if overlay.Caveman != nil {
		settings.Caveman = overlay.Caveman
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
	if v := os.Getenv("CLAUDIO_CAVEMAN"); v == "true" || v == "1" {
		b := true
		settings.Caveman = &b
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
// GetCompactKeepN returns the number of recent messages to keep after compaction.
func (s *Settings) GetCompactKeepN() int {
	if s.CompactKeepN <= 0 {
		return 10
	}
	return s.CompactKeepN
}

// GetAgentAutoDeleteAfter returns the number of human messages of inactivity
// after which a done agent is removed. Returns 3 if not configured.
// A value of -1 means never auto-delete.
func (s *Settings) GetAgentAutoDeleteAfter() int {
	if s.AgentAutoDeleteAfter == 0 {
		return -1
	}
	return s.AgentAutoDeleteAfter
}

// IsAutoMemoryExtract returns whether automatic memory extraction is enabled.
func (s *Settings) IsAutoMemoryExtract() bool {
	if s.AutoMemoryExtract == nil {
		return false // default: disabled
	}
	return *s.AutoMemoryExtract
}

// GetMemorySelection returns the memory selection strategy ("ai", "keyword", "none").
func (s *Settings) GetMemorySelection() string {
	if s.MemorySelection == "" {
		return "none" // default
	}
	return s.MemorySelection
}

// GetMemoryIndexTTLDays returns the number of days before memory entries are excluded
// from the inline index. Defaults to 30 days if not configured.
func (s *Settings) GetMemoryIndexTTLDays() int {
	if s.MemoryIndexTTLDays != nil {
		return *s.MemoryIndexTTLDays
	}
	return 30
}

// GetMemoryRefreshOnCompact returns whether to rebuild the memory index after compaction.
// Defaults to true if not configured.
func (s *Settings) GetMemoryRefreshOnCompact() bool {
	if s.MemoryRefreshOnCompact != nil {
		return *s.MemoryRefreshOnCompact
	}
	return true
}

// CavemanEnabled returns whether the caveman ultra-compressed mode is enabled.
func (s *Settings) CavemanEnabled() bool {
	return s.Caveman != nil && *s.Caveman
}

func ProjectMemoryDir(projectRoot string) string {
	p := GetPaths()
	slug := SanitizeProjectPath(projectRoot)
	return filepath.Join(p.Projects, slug, "memory")
}

// ProjectDesignsDir returns the designs directory for a specific project.
// Uses the git root (or cwd) to create a stable, project-scoped path.
func ProjectDesignsDir(projectRoot string) string {
	p := GetPaths()
	slug := SanitizeProjectPath(projectRoot)
	return filepath.Join(p.Projects, slug, "designs")
}
