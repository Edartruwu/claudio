package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ValidationError represents a settings validation issue.
type ValidationError struct {
	Field   string
	Message string
	Hint    string
}

func (e ValidationError) String() string {
	s := fmt.Sprintf("%s: %s", e.Field, e.Message)
	if e.Hint != "" {
		s += fmt.Sprintf(" (hint: %s)", e.Hint)
	}
	return s
}

// ValidateSettings checks settings for common errors and returns warnings.
func ValidateSettings(s *Settings) []ValidationError {
	var errs []ValidationError

	// Model validation
	if s.Model != "" {
		validModels := []string{
			"claude-opus-4-6", "claude-sonnet-4-6", "claude-haiku-4-5",
			"claude-opus-4-5", "claude-sonnet-4-5",
			"claude-opus-4", "claude-sonnet-4",
		}
		found := false
		for _, m := range validModels {
			if strings.Contains(s.Model, m) || strings.HasPrefix(s.Model, m) {
				found = true
				break
			}
		}
		if !found {
			errs = append(errs, ValidationError{
				Field:   "model",
				Message: fmt.Sprintf("unknown model %q", s.Model),
				Hint:    "valid models: claude-opus-4-6, claude-sonnet-4-6, claude-haiku-4-5-20251001",
			})
		}
	}

	// Permission mode validation
	if s.PermissionMode != "" {
		valid := map[string]bool{"default": true, "auto": true, "headless": true, "plan": true}
		if !valid[s.PermissionMode] {
			errs = append(errs, ValidationError{
				Field:   "permissionMode",
				Message: fmt.Sprintf("unknown permission mode %q", s.PermissionMode),
				Hint:    "valid modes: default, auto, headless, plan",
			})
		}
	}

	// Compact mode validation
	if s.CompactMode != "" {
		valid := map[string]bool{"auto": true, "manual": true, "strategic": true}
		if !valid[s.CompactMode] {
			errs = append(errs, ValidationError{
				Field:   "compactMode",
				Message: fmt.Sprintf("unknown compact mode %q", s.CompactMode),
				Hint:    "valid modes: auto, manual, strategic",
			})
		}
	}

	// Budget validation
	if s.MaxBudget < 0 {
		errs = append(errs, ValidationError{
			Field:   "maxBudget",
			Message: "budget cannot be negative",
		})
	}

	// DenyPaths validation
	for _, p := range s.DenyPaths {
		if !strings.Contains(p, "*") && !strings.HasPrefix(p, "~") && !strings.HasPrefix(p, "/") {
			errs = append(errs, ValidationError{
				Field:   "denyPaths",
				Message: fmt.Sprintf("path %q should be absolute or start with ~ or contain *", p),
			})
		}
	}

	// MCP server validation
	for name, srv := range s.MCPServers {
		if srv.Command == "" && srv.URL == "" {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("mcpServers.%s", name),
				Message: "server needs either command or url",
			})
		}
		if srv.Type != "" {
			valid := map[string]bool{"stdio": true, "sse": true, "http": true}
			if !valid[srv.Type] {
				errs = append(errs, ValidationError{
					Field:   fmt.Sprintf("mcpServers.%s.type", name),
					Message: fmt.Sprintf("unknown type %q", srv.Type),
					Hint:    "valid types: stdio, sse, http",
				})
			}
		}
	}

	return errs
}

// --- Content Truncation ---

const maxCLAUDEMDChars = 40_000

// TruncateContent truncates content to the max allowed characters.
func TruncateContent(content string, maxChars int) string {
	if maxChars <= 0 {
		maxChars = maxCLAUDEMDChars
	}
	if len(content) <= maxChars {
		return content
	}
	return content[:maxChars] + "\n\n... (content truncated at " + fmt.Sprintf("%d", maxChars) + " characters)"
}

// --- @include Directive Support ---

var includePattern = regexp.MustCompile(`(?m)^@(\S+)\s*$`)

// ExpandIncludes processes @path directives in markdown content.
// Supports: @./relative, @~/home, @/absolute
// Prevents circular references via visited set.
func ExpandIncludes(content, baseDir string) string {
	return expandIncludesRecursive(content, baseDir, make(map[string]bool), 0)
}

func expandIncludesRecursive(content, baseDir string, visited map[string]bool, depth int) string {
	if depth > 10 {
		return content // prevent infinite recursion
	}

	return includePattern.ReplaceAllStringFunc(content, func(match string) string {
		// Extract the path (strip the leading @)
		path := strings.TrimSpace(match[1:])

		// Resolve the path
		resolved := resolveIncludePath(path, baseDir)
		if resolved == "" {
			return match // keep original if can't resolve
		}

		// Check for circular reference
		canonical := canonicalPath(resolved)
		if visited[canonical] {
			return fmt.Sprintf("<!-- circular include: %s -->", path)
		}

		// Check if file exists and has allowed extension
		if !isAllowedInclude(resolved) {
			return match
		}

		data, err := os.ReadFile(resolved)
		if err != nil {
			return match // keep original if file not readable
		}

		visited[canonical] = true
		includeDir := filepath.Dir(resolved)
		included := expandIncludesRecursive(string(data), includeDir, visited, depth+1)
		delete(visited, canonical) // allow same file in different branches

		return included
	})
}

func resolveIncludePath(path, baseDir string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	if strings.HasPrefix(path, "./") || !strings.HasPrefix(path, "/") {
		return filepath.Join(baseDir, path)
	}
	return path // absolute path
}

var allowedExtensions = map[string]bool{
	".md": true, ".txt": true, ".json": true, ".yaml": true, ".yml": true,
	".toml": true, ".cfg": true, ".conf": true, ".ini": true,
	".go": true, ".py": true, ".rs": true, ".ts": true, ".js": true,
	".java": true, ".kt": true, ".rb": true, ".php": true, ".c": true,
	".cpp": true, ".h": true, ".hpp": true, ".cs": true, ".swift": true,
	".sh": true, ".bash": true, ".zsh": true, ".fish": true,
}

func isAllowedInclude(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return allowedExtensions[ext]
}

// --- Settings Display ---

// FormatSettings returns a human-readable display of current settings with source tracking.
func FormatSettings(s *Settings, sources map[string]string) string {
	var sb strings.Builder

	// Compute sources by comparing against defaults and each file
	if sources == nil {
		sources = ComputeSources(s)
	}

	sb.WriteString("Current Configuration\n")
	sb.WriteString("=====================\n\n")

	writeField := func(name, value, source string) {
		if value == "" {
			value = "(not set)"
		}
		src := ""
		if source != "" {
			src = fmt.Sprintf("  [%s]", source)
		}
		sb.WriteString(fmt.Sprintf("  %-20s %-30s%s\n", name+":", value, src))
	}

	sb.WriteString("Model & AI:\n")
	writeField("model", s.Model, sources["model"])
	writeField("smallModel", s.SmallModel, sources["smallModel"])

	sb.WriteString("\nPermissions:\n")
	writeField("permissionMode", s.PermissionMode, sources["permissionMode"])
	if len(s.DenyPaths) > 0 {
		writeField("denyPaths", fmt.Sprintf("%d path(s)", len(s.DenyPaths)), sources["denyPaths"])
		for _, p := range s.DenyPaths {
			sb.WriteString(fmt.Sprintf("  %-20s   %s\n", "", p))
		}
	}
	if len(s.AllowPaths) > 0 {
		writeField("allowPaths", fmt.Sprintf("%d path(s)", len(s.AllowPaths)), sources["allowPaths"])
	}
	if len(s.DenyTools) > 0 {
		writeField("denyTools", strings.Join(s.DenyTools, ", "), sources["denyTools"])
	}

	sb.WriteString("\nSession:\n")
	writeField("autoCompact", fmt.Sprintf("%v", s.AutoCompact), sources["autoCompact"])
	writeField("compactMode", s.CompactMode, sources["compactMode"])
	writeField("sessionPersist", fmt.Sprintf("%v", s.SessionPersist), sources["sessionPersist"])
	writeField("hookProfile", s.HookProfile, sources["hookProfile"])

	sb.WriteString("\nNetwork:\n")
	writeField("apiBaseUrl", s.APIBaseURL, sources["apiBaseUrl"])
	if s.ProxyURL != "" {
		writeField("proxyUrl", s.ProxyURL, sources["proxyUrl"])
	}

	sb.WriteString("\nBudget:\n")
	if s.MaxBudget > 0 {
		writeField("maxBudget", fmt.Sprintf("$%.2f", s.MaxBudget), sources["maxBudget"])
	} else {
		writeField("maxBudget", "unlimited", "")
	}

	if len(s.MCPServers) > 0 {
		sb.WriteString("\nMCP Servers:\n")
		for name, srv := range s.MCPServers {
			detail := srv.Command
			if srv.URL != "" {
				detail = srv.URL
			}
			stype := srv.Type
			if stype == "" {
				stype = "stdio"
			}
			sb.WriteString(fmt.Sprintf("  %-20s %s (%s)\n", name+":", detail, stype))
		}
	}

	sb.WriteString("\nConfig Files:\n")
	paths := GetPaths()
	sb.WriteString(fmt.Sprintf("  User:    %s", paths.Settings))
	if fileExists(paths.Settings) {
		sb.WriteString(" (exists)\n")
	} else {
		sb.WriteString(" (not found)\n")
	}

	sb.WriteString(fmt.Sprintf("  Local:   %s", paths.Local))
	if fileExists(paths.Local) {
		sb.WriteString(" (exists)\n")
	} else {
		sb.WriteString("\n")
	}

	cwd, _ := os.Getwd()
	projectSettings := filepath.Join(cwd, ".claudio", "settings.json")
	sb.WriteString(fmt.Sprintf("  Project: %s", projectSettings))
	if fileExists(projectSettings) {
		sb.WriteString(" (exists)\n")
	} else {
		sb.WriteString("\n")
	}

	return sb.String()
}

// ComputeSources determines where each settings value came from.
func ComputeSources(s *Settings) map[string]string {
	sources := make(map[string]string)
	defaults := DefaultSettings()
	p := GetPaths()
	cwd, _ := os.Getwd()

	// Check each file to see what it contributes
	userSettings := loadSettingsFile(p.Settings)
	localSettings := loadSettingsFile(p.Local)
	projectSettings := loadSettingsFile(filepath.Join(cwd, ".claudio", "settings.json"))

	checkSource := func(key, current, defaultVal string) string {
		if projectSettings != nil {
			if v := getField(projectSettings, key); v != "" && v == current {
				return "project"
			}
		}
		if localSettings != nil {
			if v := getField(localSettings, key); v != "" && v == current {
				return "local"
			}
		}
		if v := os.Getenv("CLAUDIO_" + strings.ToUpper(key)); v != "" {
			return "env"
		}
		if userSettings != nil {
			if v := getField(userSettings, key); v != "" && v == current {
				return "user"
			}
		}
		if current == defaultVal {
			return "default"
		}
		return ""
	}

	sources["model"] = checkSource("model", s.Model, defaults.Model)
	sources["smallModel"] = checkSource("smallModel", s.SmallModel, defaults.SmallModel)
	sources["permissionMode"] = checkSource("permissionMode", s.PermissionMode, defaults.PermissionMode)
	sources["compactMode"] = checkSource("compactMode", s.CompactMode, defaults.CompactMode)
	sources["apiBaseUrl"] = checkSource("apiBaseUrl", s.APIBaseURL, defaults.APIBaseURL)
	sources["hookProfile"] = checkSource("hookProfile", s.HookProfile, defaults.HookProfile)

	return sources
}

func loadSettingsFile(path string) map[string]interface{} {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var m map[string]interface{}
	json.Unmarshal(data, &m)
	return m
}

func getField(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

// FormatSettingsJSON returns the current settings as formatted JSON.
func FormatSettingsJSON(s *Settings) string {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return string(data)
}
