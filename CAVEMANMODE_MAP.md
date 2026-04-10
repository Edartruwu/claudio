# CavemanMode Complete Flow Map

## 1. STRUCT DEFINITION & PERSISTENCE KEY

### **File: `internal/config/config.go`** (Lines 73-75)
```go
// Caveman mode — injects terse communication rules into system prompt
// Values: "" (off), "lite", "full", "ultra"
CavemanMode string `json:"cavemanMode,omitempty"`
```

**Persistence Key**: `"cavemanMode"` in JSON

**Storage Paths**:
- Global: `~/.claudio/settings.json` (via `config.GetPaths().Settings`)
- Project: `.claudio/settings.json` (detected via `config.FindGitRoot()`)
- Config loaded in priority order: env vars > project > local > user

---

## 2. CONFIG LOADING AT STARTUP

### **File: `internal/config/config.go`** (Lines 230-251)
```go
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
```

### Merge Logic (Lines 253-393)
- Read JSON from file into overlay `Settings` struct
- Check raw JSON to detect explicit `cavemanMode` presence (handles zero/empty values)
- Lines 365-368 specifically:
```go
if _, ok := raw["cavemanMode"]; ok {
	settings.CavemanMode = overlay.CavemanMode
}
```

---

## 3. AGENT TOOL - CAVEMAN MODE INJECTION

### **File: `internal/app/app.go`** (Lines 375)
```go
// Wire into Agent tool at startup (New() function, line ~361-376)
if agent, err := registry.Get("Agent"); err == nil {
	if at, ok := agent.(*tools.AgentTool); ok {
		at.GetCavemanMode = func() string { return settings.CavemanMode }
	}
}
```

### **File: `internal/tools/agent.go`** (Lines 227-228)
```go
// GetCavemanMode returns the current caveman mode so sub-agents inherit it.
GetCavemanMode func() string
```

### **File: `internal/tools/agent.go`** (Lines 335-340)
```go
// Inherit caveman mode from the parent session.
if t.GetCavemanMode != nil {
	if cs := prompts.CavemanSection(t.GetCavemanMode()); cs != "" {
		agentDef.SystemPrompt += "\n\n" + cs
	}
}
```

**How it works**: 
- Agent calls `GetCavemanMode()` closure to fetch current mode
- Passes mode to `prompts.CavemanSection()` to generate rules
- Appends rules to agent's system prompt

---

## 4. SPAWNTEAMMATE TOOL - CAVEMAN MODE INHERITANCE

### **File: `internal/app/app.go`** (Lines 436)
```go
// Wire into SpawnTeammate tool at startup
if st, err := registry.Get("SpawnTeammate"); err == nil {
	if tool, ok := st.(*tools.SpawnTeammateTool); ok {
		tool.GetCavemanMode = func() string { return settings.CavemanMode }
	}
}
```

### **File: `internal/tools/spawnteammate.go`** (Lines 35, 281-285)
```go
type SpawnTeammateTool struct {
	// ...
	GetCavemanMode  func() string
}

// In Execute():
if t.GetCavemanMode != nil {
	if cs := prompts.CavemanSection(t.GetCavemanMode()); cs != "" {
		agentDef.SystemPrompt += "\n\n" + cs
	}
}
```

**How it works**: Same pattern as Agent tool — spawned teammates inherit parent's caveman mode.

---

## 5. CAVEMAN RULES - SYSTEM PROMPT INJECTION

### **File: `internal/prompts/system.go`** (Lines 24, 42-44)
```go
func BuildSystemPrompt(model string, additionalContext string, cavemanMode string) string {
	// ... staticSections built ...
	dynamicSections := []string{
		environmentSection(model),
	}
	if additionalContext != "" {
		dynamicSections = append(dynamicSections, additionalContext)
	}
	if cs := cavemanSection(cavemanMode); cs != "" {
		dynamicSections = append(dynamicSections, cs)
	}
```

### **File: `internal/prompts/system.go`** (Lines 314-327)
```go
// CavemanSection returns the caveman communication rules for the given mode.
// Returns "" when mode is empty or unrecognized.
func CavemanSection(mode string) string { return cavemanSection(mode) }

func cavemanSection(mode string) string {
	switch mode {
	case "lite":
		return "# Communication Style\nDrop filler words, hedging, and pleasantries. Keep articles and full sentences. Professional but no fluff."
	case "full":
		return "# Communication Style\nRespond terse like smart caveman. Drop articles (a/an/the), filler (just/really/basically/actually), pleasantries (sure/certainly/happy to), hedging. Fragments OK. Short synonyms. Pattern: [thing] [action] [reason]. [next step]. Code/commits/security: write normal. User says 'normal' or 'stop caveman' to deactivate."
	case "ultra":
		return "# Communication Style\nULTRA TERSE. Abbreviate (DB/auth/config/req/res/fn/impl). Strip conjunctions. Arrows for causality (X → Y). One word when one word enough. Fragments only. Code unchanged."
	default:
		return ""
	}
}
```

**Modes & Rules**:
- `""` (empty): Off, returns empty string
- `"lite"`: Drop filler, keep articles and full sentences
- `"full"`: Terse, fragments OK, drop articles and filler
- `"ultra"`: Maximum terseness, abbreviations, arrows, one word where possible

---

## 6. TUI TOGGLE HANDLER - GET & SET

### **File: `internal/tui/root.go`** (Lines 453-461)
```go
GetCavemanMode: func() string {
	if m.appCtx != nil && m.appCtx.Config != nil {
		return m.appCtx.Config.CavemanMode
	}
	return ""
},
SetCavemanMode: func(mode string) {
	m.applyConfigChange("cavemanMode", mode)
},
```

---

## 7. TUI TOGGLE EXECUTION & SYSTEM PROMPT REBUILD

### **File: `internal/tui/root.go`** (Lines 4745-4755)
```go
case "cavemanMode":
	if m.appCtx != nil && m.appCtx.Config != nil {
		m.appCtx.Config.CavemanMode = value
	}
	// Rebuild system prompt to inject/remove caveman rules
	newPrompt := prompts.BuildSystemPrompt(m.model, "", value)
	m.baseSystemPrompt = newPrompt
	m.systemPrompt = newPrompt
	if m.engine != nil {
		m.engine.SetSystem(newPrompt)
	}
```

**What happens when toggled**:
1. Update `m.appCtx.Config.CavemanMode` in memory
2. Rebuild full system prompt with new mode
3. Update both base and active prompts
4. Inject into current engine to affect next message

---

## 8. CONFIG PANEL - DISPLAY & CYCLING

### **File: `internal/tui/panels/config/config.go`** (Lines 163-167)
```go
if m.CavemanMode != "" {
	addE("cavemanMode", m.CavemanMode, "cycle", p.source("cavemanMode"))
} else {
	addE("cavemanMode", "off", "cycle", ScopeGlobal)
}
```

### **File: `internal/tui/panels/config/config.go`** (Lines 422-425)
```go
case "cavemanMode":
	modes := []string{"", "lite", "full", "ultra"}
	target.CavemanMode = cycleValue(p.merged.CavemanMode, modes, "")
	newVal = target.CavemanMode
```

**Cycling Logic**:
- Cycles through: `["", "lite", "full", "ultra"]`
- Empty string ("") represents "off"
- Displayed as "off" in UI when empty

---

## 9. CONFIG PERSISTENCE TO DISK

### **File: `internal/tui/panels/config/config.go`** (Lines 457-489)
```go
// saveProjectSetting writes a single key to the project config file using
// raw JSON merge, which correctly handles bool false values (omitempty).
func (p *Panel) saveProjectSetting(key, value string) {
	dir := filepath.Dir(p.projectPath)
	os.MkdirAll(dir, 0755)

	var existing map[string]json.RawMessage
	if data, err := os.ReadFile(p.projectPath); err == nil {
		json.Unmarshal(data, &existing)
	}
	if existing == nil {
		existing = make(map[string]json.RawMessage)
	}

	valJSON, _ := json.Marshal(value)
	existing[key] = valJSON

	out, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(p.projectPath, out, 0644)
}
```

### **File: `internal/tui/panels/config/config.go`** (Lines 436-438)
```go
p.saveProjectSetting(e.Key, newVal)
p.reloadMerged()
return e.Key, newVal
```

**Save Flow**:
1. User toggles in config panel
2. `Toggle()` updates in-memory `Settings` object
3. Calls `saveProjectSetting("cavemanMode", newVal)`
4. Reads existing `.claudio/settings.json`, merges new value
5. Writes back to disk as JSON
6. Reloads config to reflect disk state
7. GUI reflects saved value on next render

---

## SUMMARY TABLE

| Component | File | Lines | Purpose |
|-----------|------|-------|---------|
| **Struct Definition** | `internal/config/config.go` | 73-75 | `CavemanMode string` field |
| **Load from Disk** | `internal/config/config.go` | 230-251, 365-368 | Merge all sources (user → project) |
| **Agent Tool Closure** | `internal/app/app.go` | 375 | Wire `GetCavemanMode()` |
| **Agent Inheritance** | `internal/tools/agent.go` | 227-228, 335-340 | Sub-agents inherit mode |
| **Teammate Inheritance** | `internal/tools/spawnteammate.go` | 35, 281-285 | Teammates inherit mode |
| **Rule Generation** | `internal/prompts/system.go` | 314-327 | Generate terse rules per mode |
| **System Prompt Build** | `internal/prompts/system.go` | 24, 42-44 | Inject rules into prompt |
| **TUI Get Closure** | `internal/tui/root.go` | 453-458 | Query current mode |
| **TUI Set Closure** | `internal/tui/root.go` | 459-461 | Toggle via `applyConfigChange()` |
| **TUI Apply Change** | `internal/tui/root.go` | 4745-4755 | Update prompt, inject into engine |
| **Config Panel Display** | `internal/tui/panels/config/config.go` | 163-167 | Show as "off" or mode name |
| **Config Panel Cycling** | `internal/tui/panels/config/config.go` | 422-425 | Cycle through modes list |
| **Save to Disk** | `internal/tui/panels/config/config.go` | 457-489 | Write to `.claudio/settings.json` |

---

## EXECUTION PATH: USER TOGGLES IN TUI

```
User presses key in config panel
  ↓
config.Toggle() 
  ↓
Switch on key "cavemanMode" 
  ↓
target.CavemanMode = cycleValue(...) 
  ↓
saveProjectSetting("cavemanMode", newVal) → writes .claudio/settings.json
  ↓
reloadMerged() → reloads from disk
  ↓
Panel re-renders with new value
  ↓
User confirms
  ↓
root.SetCavemanMode(mode) called (from TUI handler)
  ↓
applyConfigChange("cavemanMode", mode)
  ↓
Update m.appCtx.Config.CavemanMode
  ↓
BuildSystemPrompt() with new mode
  ↓
Rebuild rules + inject into engine
  ↓
Next message uses new rules
```

---

## INHERITANCE PATH: SUB-AGENTS

```
Main session has CavemanMode = "ultra"
  ↓
User calls Agent tool or SpawnTeammate
  ↓
Tool calls GetCavemanMode() closure
  ↓
Returns "ultra"
  ↓
Tool calls prompts.CavemanSection("ultra")
  ↓
Returns ultra-terse rules string
  ↓
Appends to agentDef.SystemPrompt
  ↓
Sub-agent receives system prompt with injected rules
  ↓
Sub-agent uses "ultra" mode for all responses
```
