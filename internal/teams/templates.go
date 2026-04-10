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
	SubagentType string `json:"subagent_type"`           // resolves to an AgentDefinition (e.g. "backend-senior")
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

// LoadTemplates reads all *.json files from dir and returns the parsed templates.
func LoadTemplates(dir string) []TeamTemplate {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []TeamTemplate
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		t, err := GetTemplate(dir, name)
		if err != nil {
			continue
		}
		out = append(out, *t)
	}
	return out
}

// GetTemplate loads a single template by name from dir.
func GetTemplate(dir, name string) (*TeamTemplate, error) {
	path := filepath.Join(dir, name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("template %q not found", name)
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
