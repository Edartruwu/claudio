package config

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
)

// TrustedProject records when a project directory was accepted.
type TrustedProject struct {
	AcceptedAt time.Time `json:"accepted_at"`
	ProjectDir string    `json:"project_dir"`
	HasHooks   bool      `json:"has_hooks,omitempty"`
	HasMCP     bool      `json:"has_mcp,omitempty"`
	HasRules   bool      `json:"has_rules,omitempty"`
}

// TrustStore manages trusted project directories.
type TrustStore struct {
	path     string
	projects map[string]*TrustedProject
}

// NewTrustStore loads or creates the trust store at ~/.claudio/trusted.json.
func NewTrustStore() *TrustStore {
	paths := GetPaths()
	storePath := filepath.Join(paths.Home, "trusted.json")

	ts := &TrustStore{
		path:     storePath,
		projects: make(map[string]*TrustedProject),
	}
	ts.load()
	return ts
}

// IsTrusted checks if a project directory has been accepted.
func (ts *TrustStore) IsTrusted(projectDir string) bool {
	canonical := canonicalPath(projectDir)
	_, ok := ts.projects[canonical]
	return ok
}

// Trust marks a project directory as accepted.
func (ts *TrustStore) Trust(projectDir string, hasHooks, hasMCP, hasRules bool) error {
	canonical := canonicalPath(projectDir)
	ts.projects[canonical] = &TrustedProject{
		AcceptedAt: time.Now(),
		ProjectDir: canonical,
		HasHooks:   hasHooks,
		HasMCP:     hasMCP,
		HasRules:   hasRules,
	}
	return ts.save()
}

// Untrust removes trust for a project directory.
func (ts *TrustStore) Untrust(projectDir string) error {
	canonical := canonicalPath(projectDir)
	delete(ts.projects, canonical)
	return ts.save()
}

// ProjectSecurityInfo describes what a project's configuration will load.
type ProjectSecurityInfo struct {
	HasSettings bool
	HasHooks    bool
	HasMCP      bool
	HasRules    bool
	HasCLAUDEMD bool
	HasSkills   bool
	HasAgents   bool
	// Details
	HookCount int
	MCPCount  int
	RuleCount int
	SkillCount int
	AgentCount int
}

// ScanProjectConfig examines a project directory and reports what will be loaded.
func ScanProjectConfig(projectDir string) *ProjectSecurityInfo {
	info := &ProjectSecurityInfo{}

	claudioDir := filepath.Join(projectDir, ".claudio")

	// Settings
	if fileExists(filepath.Join(claudioDir, "settings.json")) {
		info.HasSettings = true

		// Check for hooks and MCP in settings
		data, err := os.ReadFile(filepath.Join(claudioDir, "settings.json"))
		if err == nil {
			var settings struct {
				Hooks      json.RawMessage            `json:"hooks"`
				MCPServers map[string]json.RawMessage  `json:"mcpServers"`
			}
			if json.Unmarshal(data, &settings) == nil {
				if len(settings.Hooks) > 2 { // not just "{}"
					info.HasHooks = true
					info.HookCount = countJSONKeys(settings.Hooks)
				}
				if len(settings.MCPServers) > 0 {
					info.HasMCP = true
					info.MCPCount = len(settings.MCPServers)
				}
			}
		}
	}

	// Rules
	if rulesDir := filepath.Join(claudioDir, "rules"); dirExists(rulesDir) {
		entries, _ := os.ReadDir(rulesDir)
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".md") {
				info.RuleCount++
			}
		}
		info.HasRules = info.RuleCount > 0
	}

	// CLAUDE.md / CLAUDIO.md
	for _, name := range []string{"CLAUDIO.md", "CLAUDE.md"} {
		if fileExists(filepath.Join(projectDir, name)) {
			info.HasCLAUDEMD = true
			break
		}
	}
	if fileExists(filepath.Join(claudioDir, "CLAUDE.md")) {
		info.HasCLAUDEMD = true
	}

	// Skills
	if skillsDir := filepath.Join(claudioDir, "skills"); dirExists(skillsDir) {
		entries, _ := os.ReadDir(skillsDir)
		info.SkillCount = len(entries)
		info.HasSkills = info.SkillCount > 0
	}

	// Agents
	if agentsDir := filepath.Join(claudioDir, "agents"); dirExists(agentsDir) {
		entries, _ := os.ReadDir(agentsDir)
		info.AgentCount = len(entries)
		info.HasAgents = info.AgentCount > 0
	}

	return info
}

// FormatTrustPrompt creates a human-readable summary of what project config will be loaded.
func FormatTrustPrompt(projectDir string, info *ProjectSecurityInfo) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Project: %s\n", projectDir))
	sb.WriteString("This project has configuration that will be loaded:\n\n")

	if info.HasSettings {
		sb.WriteString("  [!] .claudio/settings.json — project settings\n")
	}
	if info.HasHooks {
		sb.WriteString(fmt.Sprintf("  [!] Hooks — %d hook event(s) will execute shell commands\n", info.HookCount))
	}
	if info.HasMCP {
		sb.WriteString(fmt.Sprintf("  [!] MCP Servers — %d server(s) will be started\n", info.MCPCount))
	}
	if info.HasRules {
		sb.WriteString(fmt.Sprintf("  [i] Rules — %d rule file(s)\n", info.RuleCount))
	}
	if info.HasCLAUDEMD {
		sb.WriteString("  [i] CLAUDE.md / CLAUDIO.md — project instructions\n")
	}
	if info.HasSkills {
		sb.WriteString(fmt.Sprintf("  [i] Skills — %d custom skill(s)\n", info.SkillCount))
	}
	if info.HasAgents {
		sb.WriteString(fmt.Sprintf("  [i] Agents — %d custom agent(s)\n", info.AgentCount))
	}

	sb.WriteString("\nItems marked [!] can execute code on your machine.\n")
	sb.WriteString("Do you trust this project? (y/n): ")

	return sb.String()
}

// HasProjectConfig returns true if the directory has any .claudio/ configuration.
func HasProjectConfig(projectDir string) bool {
	claudioDir := filepath.Join(projectDir, ".claudio")
	return dirExists(claudioDir) ||
		fileExists(filepath.Join(projectDir, "CLAUDIO.md")) ||
		fileExists(filepath.Join(projectDir, "CLAUDE.md"))
}

// FindGitRoot returns the git root directory, or cwd if not in a repo.
func FindGitRoot(dir string) string {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return dir
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return dir
	}
	return root
}

func (ts *TrustStore) load() {
	data, err := os.ReadFile(ts.path)
	if err != nil {
		return
	}
	json.Unmarshal(data, &ts.projects)
}

func (ts *TrustStore) save() error {
	dir := filepath.Dir(ts.path)
	os.MkdirAll(dir, 0700)

	// Use file lock for concurrent access safety
	lock := flock.New(ts.path + ".lock")
	if err := lock.Lock(); err != nil {
		// If locking fails, write anyway (best effort)
		return ts.writeFile()
	}
	defer lock.Unlock()

	return ts.writeFile()
}

func (ts *TrustStore) writeFile() error {
	data, err := json.MarshalIndent(ts.projects, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ts.path, data, 0600)
}

func canonicalPath(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	// Try to resolve symlinks
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return abs
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func countJSONKeys(data json.RawMessage) int {
	var m map[string]json.RawMessage
	if json.Unmarshal(data, &m) == nil {
		return len(m)
	}
	return 0
}
