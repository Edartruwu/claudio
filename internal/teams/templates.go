package teams

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AdvisorConfig specifies an advisor for a team member.
type AdvisorConfig struct {
	SubagentType string `json:"subagent_type,omitempty"` // resolves to an AgentDefinition (e.g. "backend-senior")
	Model        string `json:"model,omitempty"`         // model override for advisor (e.g. "claude-opus-4-6")
	MaxUses      int    `json:"max_uses,omitempty"`      // per-session call budget (0 = unlimited)
}

// TeamTemplateMember defines a member slot in a team template.
type TeamTemplateMember struct {
	Name                 string          `json:"name"`
	SubagentType         string          `json:"subagent_type"`
	Model                string          `json:"model,omitempty"`                  // per-member model override
	AutoCompactThreshold int             `json:"autoCompactThreshold,omitempty"`   // % context to trigger compact (overrides team-level)
	Advisor              *AdvisorConfig  `json:"advisor,omitempty"`
}

// TeamTemplate is a reusable team composition stored at ~/.claudio/team-templates/{name}.json.
type TeamTemplate struct {
	Name                 string               `json:"name"`
	Description          string               `json:"description,omitempty"`
	Model                string               `json:"model,omitempty"` // team default model
	AutoCompactThreshold int                  `json:"autoCompactThreshold,omitempty"` // % context to trigger compact for all members
	Members              []TeamTemplateMember `json:"members"`
}

// LoadTemplates reads all *.json files from the given dirs and returns parsed templates.
// First occurrence of a template name wins (user templates override harness ones).
// Accepts zero or more dirs; missing dirs are silently skipped.
func LoadTemplates(dirs ...string) []TeamTemplate {
	seen := make(map[string]struct{})
	var out []TeamTemplate
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ".json")
			if _, exists := seen[name]; exists {
				continue // first wins
			}
			t, err := GetTemplate(dir, name)
			if err != nil {
				continue
			}
			seen[name] = struct{}{}
			out = append(out, *t)
		}
	}
	return out
}

// GetTemplate loads a single template by name, searching dirs in order (first match wins).
// Callers that previously passed a single dir can still call GetTemplate(dir, name).
func GetTemplate(dir, name string, extraDirs ...string) (*TeamTemplate, error) {
	allDirs := append([]string{dir}, extraDirs...)
	for _, d := range allDirs {
		if d == "" {
			continue
		}
		path := filepath.Join(d, name+".json")
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		var t TeamTemplate
		if err := json.Unmarshal(data, &t); err != nil {
			return nil, fmt.Errorf("invalid template %q: %w", name, err)
		}
		if t.Name == "" {
			t.Name = name
		}
		return &t, nil
	}
	return nil, fmt.Errorf("template %q not found", name)
}

// SaveTemplate writes t to {dir}/{t.Name}.json.
func SaveTemplate(dir string, t TeamTemplate) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, t.Name+".json")
	return os.WriteFile(path, data, 0600)
}
