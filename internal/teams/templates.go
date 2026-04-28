package teams

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// AdvisorConfig specifies an advisor for a team member.
type AdvisorConfig struct {
	SubagentType string `json:"subagent_type,omitempty"` // resolves to an AgentDefinition (e.g. "backend-senior")
	Model        string `json:"model,omitempty"`         // model override for advisor (e.g. "claude-opus-4-6")
	MaxUses      int    `json:"max_uses,omitempty"`      // per-session call budget (0 = unlimited)
}

// TeamTemplateMember defines a member slot in a team template.
type TeamTemplateMember struct {
	Name                 string         `json:"name"`
	SubagentType         string         `json:"subagent_type"`
	Model                string         `json:"model,omitempty"`                // per-member model override
	AutoCompactThreshold int            `json:"autoCompactThreshold,omitempty"` // % context to trigger compact (overrides team-level)
	Advisor              *AdvisorConfig `json:"advisor,omitempty"`
}

// TeamTemplate is a reusable team composition stored at ~/.claudio/team-templates/{name}.json.
type TeamTemplate struct {
	Name                 string               `json:"name"`
	Description          string               `json:"description,omitempty"`
	Model                string               `json:"model,omitempty"` // team default model
	AutoCompactThreshold int                  `json:"autoCompactThreshold,omitempty"` // % context to trigger compact for all members
	Members              []TeamTemplateMember `json:"members"`
}

// LoadTemplates reads all *.lua and *.json files from the given dirs and returns parsed templates.
// Lua takes precedence if same name exists as both.
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
		// Collect names from both .lua and .json; build set of lua names for precedence.
		luaNames := make(map[string]struct{})
		jsonNames := make(map[string]struct{})
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if strings.HasSuffix(e.Name(), ".lua") {
				luaNames[strings.TrimSuffix(e.Name(), ".lua")] = struct{}{}
			} else if strings.HasSuffix(e.Name(), ".json") {
				jsonNames[strings.TrimSuffix(e.Name(), ".json")] = struct{}{}
			}
		}
		// Process lua names first, then json names (skipping those shadowed by lua).
		processName := func(name string) {
			if _, exists := seen[name]; exists {
				return
			}
			t, err := GetTemplate(dir, name)
			if err != nil {
				return
			}
			seen[name] = struct{}{}
			out = append(out, *t)
		}
		for name := range luaNames {
			processName(name)
		}
		for name := range jsonNames {
			if _, shadowed := luaNames[name]; shadowed {
				continue // lua already handled it
			}
			processName(name)
		}
	}
	return out
}

// GetTemplate loads a single template by name, searching dirs in order (first match wins).
// Tries name+".lua" before name+".json" in each dir.
func GetTemplate(dir, name string, extraDirs ...string) (*TeamTemplate, error) {
	allDirs := append([]string{dir}, extraDirs...)
	for _, d := range allDirs {
		if d == "" {
			continue
		}
		// Try Lua first.
		luaPath := filepath.Join(d, name+".lua")
		if _, err := os.Stat(luaPath); err == nil {
			return parseLuaTemplate(luaPath, name)
		}
		// Fall back to JSON.
		jsonPath := filepath.Join(d, name+".json")
		data, err := os.ReadFile(jsonPath)
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

// parseLuaTemplate executes a Lua file and maps the returned table to a TeamTemplate.
func parseLuaTemplate(path, name string) (*TeamTemplate, error) {
	L := lua.NewState()
	defer L.Close()

	if err := L.DoFile(path); err != nil {
		return nil, fmt.Errorf("lua template %q: %w", name, err)
	}

	val := L.Get(-1)
	tbl, ok := val.(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("lua template %q: expected table return, got %T", name, val)
	}

	tmpl := &TeamTemplate{}

	if v := tbl.RawGetString("name"); v != lua.LNil {
		tmpl.Name = v.String()
	}
	if tmpl.Name == "" {
		tmpl.Name = name
	}
	if v := tbl.RawGetString("description"); v != lua.LNil {
		tmpl.Description = v.String()
	}
	if v := tbl.RawGetString("model"); v != lua.LNil {
		tmpl.Model = v.String()
	}
	if v := tbl.RawGetString("auto_compact_threshold"); v != lua.LNil {
		if n, ok := v.(lua.LNumber); ok {
			tmpl.AutoCompactThreshold = int(n)
		}
	}

	membersVal := tbl.RawGetString("members")
	if membersTbl, ok := membersVal.(*lua.LTable); ok {
		membersTbl.ForEach(func(_, mv lua.LValue) {
			mt, ok := mv.(*lua.LTable)
			if !ok {
				return
			}
			m := TeamTemplateMember{}
			if v := mt.RawGetString("name"); v != lua.LNil {
				m.Name = v.String()
			}
			if v := mt.RawGetString("subagent_type"); v != lua.LNil {
				m.SubagentType = v.String()
			}
			if v := mt.RawGetString("model"); v != lua.LNil {
				m.Model = v.String()
			}
			if v := mt.RawGetString("auto_compact_threshold"); v != lua.LNil {
				if n, ok := v.(lua.LNumber); ok {
					m.AutoCompactThreshold = int(n)
				}
			}
			if advVal := mt.RawGetString("advisor"); advVal != lua.LNil {
				if advTbl, ok := advVal.(*lua.LTable); ok {
					adv := &AdvisorConfig{}
					if v := advTbl.RawGetString("subagent_type"); v != lua.LNil {
						adv.SubagentType = v.String()
					}
					if v := advTbl.RawGetString("model"); v != lua.LNil {
						adv.Model = v.String()
					}
					if v := advTbl.RawGetString("max_uses"); v != lua.LNil {
						if n, ok := v.(lua.LNumber); ok {
							adv.MaxUses = int(n)
						}
					}
					m.Advisor = adv
				}
			}
			tmpl.Members = append(tmpl.Members, m)
		})
	}

	return tmpl, nil
}

// SaveTemplate writes t to {dir}/{t.Name}.json.
// Machine-generated output is always JSON; humans write Lua.
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
