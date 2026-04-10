# OutputFilter Implementation Pattern Analysis

This document captures the exact patterns used in the claudio codebase for implementing the `outputFilter` feature, which uses raw JSON merge logic to handle boolean false values correctly.

---

## 1. Settings Struct Definition

**File:** `internal/config/config.go` (lines 14-86)

### Full Settings Struct (relevant fields):
```go
type Settings struct {
	// AI model settings
	Model        string `json:"model,omitempty"`
	SmallModel   string `json:"smallModel,omitempty"`
	ThinkingMode string `json:"thinkingMode,omitempty"`
	BudgetTokens int    `json:"budgetTokens,omitempty"`
	EffortLevel  string `json:"effortLevel,omitempty"`

	// Permission settings
	PermissionMode string `json:"permissionMode,omitempty"`

	// Session settings
	AutoCompact    bool   `json:"autoCompact,omitempty"`
	CompactMode    string `json:"compactMode,omitempty"`
	CompactKeepN   int    `json:"compactKeepN,omitempty"`
	SessionPersist bool   `json:"sessionPersist,omitempty"`

	// Memory settings
	AutoMemoryExtract *bool  `json:"autoMemoryExtract,omitempty"`
	MemorySelection   string `json:"memorySelection,omitempty"`

	// Security settings
	DenyPaths  []string `json:"denyPaths,omitempty"`
	DenyTools  []string `json:"denyTools,omitempty"`
	AllowPaths []string `json:"allowPaths,omitempty"`

	// Permission pattern rules (content-based allow/deny per tool)
	PermissionRules []PermissionRule `json:"permissionRules,omitempty"`

	// Editor command template for opening files
	EditorCmd string `json:"editorCmd,omitempty"`

	// Output style
	OutputStyle string `json:"outputStyle,omitempty"`

	// Cost threshold for confirmation dialog (USD, 0 = disabled)
	CostConfirmThreshold float64 `json:"costConfirmThreshold,omitempty"`

	// Hook profiles
	HookProfile string `json:"hookProfile,omitempty"`

	// MCP servers
	MCPServers map[string]MCPServerConfig `json:"mcpServers,omitempty"`

	// API configuration
	APIBaseURL string `json:"apiBaseUrl,omitempty"`
	ProxyURL   string `json:"proxyUrl,omitempty"`

	// Multi-provider configuration
	Providers    map[string]ProviderConfig `json:"providers,omitempty"`
	ModelRouting map[string]string         `json:"modelRouting,omitempty"`

	// Token budget
	MaxBudget float64 `json:"maxBudget,omitempty"`

	// Output filter (RTK-style token reduction for command output)
	OutputFilter bool `json:"outputFilter,omitempty"`

	// Snippet expansion (AI writes shorthand, expander fills boilerplate)
	Snippets *snippets.Config `json:"snippets,omitempty"`

	// LSP servers (config-driven, no hardcoded defaults)
	LspServers map[string]LspServerConfig `json:"lspServers,omitempty"`

	// Sidebar configuration
	Sidebar *SidebarConfig `json:"sidebar,omitempty"`

	// AgentAutoDeleteAfter controls how many human messages of inactivity cause
	// a done agent to be removed from memory. Default: 3. Set to -1 to never
	// auto-delete.
	AgentAutoDeleteAfter int `json:"agent_auto_delete_after,omitempty"`
}
```

**Key Point:** `OutputFilter` is a boolean field with `omitempty` tag (line 71).

---

## 2. Raw JSON Merge Logic for outputFilter

**File:** `internal/config/config.go` (lines 355-361)

### Exact Code for Merge Pattern:
```go
	// Use raw JSON to detect explicit outputFilter presence (handles both true and false)
	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) == nil {
		if _, ok := raw["outputFilter"]; ok {
			settings.OutputFilter = overlay.OutputFilter
		}
	}
```

**Pattern Explanation:**
1. Parse the JSON data into a `map[string]json.RawMessage` to inspect the raw keys
2. Check if `outputFilter` key exists in the raw JSON (works for both `true` and `false`)
3. If present, apply the overlay value (already unmarshaled into `overlay.OutputFilter`)
4. This bypasses the `omitempty` problem where `false` wouldn't be in the struct marshal

**Why This Works:**
- `struct` tags with `omitempty` drop zero-valued fields from JSON marshaling
- Direct struct field checks (`if overlay.OutputFilter != false`) can't distinguish "not set" from "set to false"
- Raw JSON checking detects presence regardless of the value
- Once presence is confirmed, we can safely read the unmarshaled boolean value

---

## 3. BuildSystemPrompt Function

**File:** `internal/prompts/system.go` (lines 17-58)

### Full Function Signature and Implementation:
```go
// BuildSystemPrompt constructs the full system prompt for Claudio.
//
// The prompt is split at SystemPromptDynamicBoundary:
//   - Everything before the boundary is static — identical across all sessions
//     and suitable for cross-session prompt caching.
//   - Everything after the boundary is dynamic — contains cwd, date, model,
//     CLAUDE.md content, etc. that change per session.
func BuildSystemPrompt(model string, additionalContext string) string {
	staticSections := []string{
		introSection(),
		systemSection(),
		doingTasksSection(),
		actionsSection(),
		usingToolsSection(),
		toneAndStyleSection(),
		outputEfficiencySection(),
		sessionGuidanceSection(),
	}

	dynamicSections := []string{
		environmentSection(model),
	}
	if additionalContext != "" {
		dynamicSections = append(dynamicSections, additionalContext)
	}

	var staticParts, dynamicParts []string
	for _, s := range staticSections {
		if s != "" {
			staticParts = append(staticParts, s)
		}
	}
	for _, s := range dynamicSections {
		if s != "" {
			dynamicParts = append(dynamicParts, s)
		}
	}

	return strings.Join(staticParts, "\n\n") +
		"\n\n" + SystemPromptDynamicBoundary + "\n\n" +
		strings.Join(dynamicParts, "\n\n")
}
```

**Key Design:**
- Static sections (intro, system, tasks, etc.) are cacheable and identical across sessions
- Dynamic sections (environment, additional context) vary per session
- Sections are assembled in two separate groups, joined with a boundary marker
- The `SystemPromptDynamicBoundary` constant (`"__SYSTEM_PROMPT_DYNAMIC_BOUNDARY__"`) separates static from dynamic

**Called From:**
1. `internal/cli/root.go` - buildSystemPrompt() function
2. `internal/web/sessions.go` - ProjectSession initialization (2 locations)

---

## 4. BuildSystemPrompt Call Sites

### Call Site 1: `internal/cli/root.go`

**Context (~20 lines):**
```go
	if flagAgent != "" {
		agentDef := agents.GetAgent(flagAgent)
		if agentDef.SystemPrompt != "" {
			sections = append(sections, agentDef.SystemPrompt)
		}
	}

	additionalCtx := strings.Join(sections, "\n\n")

	return prompts.BuildSystemPrompt(appInstance.Config.Model, additionalCtx)
}

func buildUserContext() string {
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	return loadCLAUDEMD(cwd, home)
}
```

**Pattern:** Additional context from agent defs and CLAUDE.md are joined and passed as `additionalContext`.

### Call Site 2: `internal/web/sessions.go` (First Usage)

**Context (~20 lines):**
```go
	apiOpts = append(apiOpts, api.WithBaseURL(settings.APIBaseURL))
	}
	client := api.NewClient(resolver, apiOpts...)

	registry := tools.DefaultRegistry()

	oldDir, _ := os.Getwd()
	os.Chdir(projectPath)
	systemPrompt := prompts.BuildSystemPrompt(model, "")
	os.Chdir(oldDir)

	_, cancel := context.WithCancel(context.Background())

	title := dbSess.Title
	if title == "" {
		title = dbSess.ID
	}

	ps := &ProjectSession{
```

**Pattern:** Call with empty additional context (`""`), with directory change to projectPath before building prompt.

### Call Site 3: `internal/web/sessions.go` (Second Usage)

**Context (~20 lines):**
```go
	apiOpts = append(apiOpts, api.WithBaseURL(settings.APIBaseURL))
	}
	client := api.NewClient(resolver, apiOpts...)

	registry := tools.DefaultRegistry()

	oldDir, _ := os.Getwd()
	os.Chdir(projectPath)
	systemPrompt := prompts.BuildSystemPrompt(settings.Model, "")
	os.Chdir(oldDir)

	var id string
	if sm.db != nil {
		model := settings.Model
		if model == "" {
			model = "claude-sonnet-4-6"
```

**Pattern:** Similar to call site 2, using `settings.Model` instead of the model parameter.

---

## 5. OutputFilter Config Panel Display

**File:** `internal/tui/panels/config/config.go` (lines 161-167)

### Display Row Around Line 161:
```go
	if m.MaxBudget > 0 {
		addR("maxBudget", fmt.Sprintf("$%.2f", m.MaxBudget), p.source("maxBudget"))
	} else {
		addR("maxBudget", "unlimited", ScopeGlobal)
	}

	addE("outputFilter", fmt.Sprintf("%v", m.OutputFilter), "bool", p.source("outputFilter"))

	if m.OutputStyle != "" {
		addE("outputStyle", m.OutputStyle, "cycle", p.source("outputStyle"))
	} else {
		addE("outputStyle", "normal", "cycle", ScopeGlobal)
	}
```

**Pattern:**
- `addE(key, displayValue, editType, scope)` - adds an editable entry
- `fmt.Sprintf("%v", m.OutputFilter)` - converts boolean to string "true" or "false"
- `"bool"` edit type - indicates this is a boolean toggle
- `p.source("outputFilter")` - determines if value comes from project or global config

---

## 6. OutputFilter Toggle Handler

**File:** `internal/tui/panels/config/config.go` (lines 409-411)

### Toggle Handler Around Line 409:
```go
	case "outputFilter":
		target.OutputFilter = !p.merged.OutputFilter
		newVal = fmt.Sprintf("%v", target.OutputFilter)
```

**Pattern in Context (lines 358-425):**
```go
func (p *Panel) toggleEntry(idx int) (string, string) {
	e := p.entries[idx]
	p.ensureProjectConfig()
	target := p.project
	var newVal string

	switch e.Key {
	case "model":
		models := []string{"claude-sonnet-4-6", "claude-opus-4-6", "claude-haiku-4-5-20251001"}
		// Append provider model IDs so they can be cycled through
		for _, pc := range p.merged.Providers {
			for _, modelID := range pc.Models {
				models = append(models, modelID)
			}
		}
		target.Model = cycleValue(p.merged.Model, models, "claude-sonnet-4-6")
		newVal = target.Model
	// ... other cases ...
	case "outputFilter":
		target.OutputFilter = !p.merged.OutputFilter
		newVal = fmt.Sprintf("%v", target.OutputFilter)
	case "outputStyle":
		modes := []string{"normal", "concise", "verbose", "markdown"}
		current := p.merged.OutputStyle
		if current == "" {
			current = "normal"
		}
		target.OutputStyle = cycleValue(current, modes, "normal")
		newVal = target.OutputStyle
	}

	p.saveProjectSetting(e.Key, newVal)
	p.reloadMerged()
	return e.Key, newVal
}
```

**Key Pattern:**
1. Toggle: `target.OutputFilter = !p.merged.OutputFilter`
2. Convert to string: `newVal = fmt.Sprintf("%v", target.OutputFilter)`
3. Save to disk: `p.saveProjectSetting(e.Key, newVal)`
4. Reload merged config: `p.reloadMerged()`

---

## 7. OutputFilter Save Logic

**File:** `internal/tui/panels/config/config.go` (lines 445-475)

### saveProjectSetting Method (Raw JSON Merge Pattern):
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

	// Marshal the full project settings to pick up struct field values
	data, _ := json.Marshal(p.project)
	var fresh map[string]json.RawMessage
	json.Unmarshal(data, &fresh)

	// Write the changed key. For bool fields, omitempty drops "false" from
	// the struct marshal, so we fall back to encoding the value directly.
	if raw, ok := fresh[key]; ok {
		existing[key] = raw
	} else if value == "true" || value == "false" {
		existing[key] = json.RawMessage(value)
	} else {
		valJSON, _ := json.Marshal(value)
		existing[key] = valJSON
	}

	out, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(p.projectPath, out, 0644)
}
```

**Key Pattern for Boolean Handling (lines 464-471):**
1. Try to use the marshaled struct value if present: `if raw, ok := fresh[key]`
2. If not found (omitempty dropped it), encode the string value directly as raw JSON
3. For booleans: `existing[key] = json.RawMessage(value)` where value is "true" or "false"
4. This creates valid JSON without quotes: `"outputFilter": true` or `"outputFilter": false`

---

## 8. OutputFilter Scope Detection

**File:** `internal/tui/panels/config/config.go` (lines 266-276)

### Scope Detection for outputFilter:
```go
	case "outputFilter":
		if p.projectPath != "" {
			if data, err := os.ReadFile(p.projectPath); err == nil {
				var raw map[string]json.RawMessage
				if json.Unmarshal(data, &raw) == nil {
					if _, ok := raw["outputFilter"]; ok {
						return ScopeProject
					}
				}
			}
		}
```

**Pattern:** Same raw JSON key detection as the merge logic — check if key exists in project file using raw JSON map.

---

## 9. OutputFilter Applied to BashTool

**File:** `internal/app/app.go` (lines 172-176)

### Initialization During App Setup:
```go
	// Inject security into file/shell tools
	if bash, err := registry.Get("Bash"); err == nil {
		if bt, ok := bash.(*tools.BashTool); ok {
			bt.Security = sec
			bt.OutputFilterEnabled = settings.OutputFilter
		}
	}
```

**Pattern:**
- Get the BashTool from registry
- Assert to `*tools.BashTool` type
- Set `OutputFilterEnabled` field from settings

---

## 10. OutputFilter Applied at Runtime (TUI)

**File:** `internal/tui/root.go` (lines 4725-4735)

### Runtime Config Change Handler:
```go
	case "outputFilter":
		enabled := value == "true"
		if m.appCtx != nil && m.appCtx.Config != nil {
			m.appCtx.Config.OutputFilter = enabled
		}
		if bash, err := m.registry.Get("Bash"); err == nil {
			if bt, ok := bash.(*tools.BashTool); ok {
				bt.OutputFilterEnabled = enabled
			}
		}
```

**Pattern:**
1. Convert string value to boolean: `enabled := value == "true"`
2. Update the app config: `m.appCtx.Config.OutputFilter = enabled`
3. Get BashTool from registry and update its flag immediately for live effect

---

## Summary of Key Patterns

### 1. **Boolean False Handling Pattern**
   - Problem: `omitempty` struct tag drops `false` values from JSON marshaling
   - Solution: Use raw JSON (`map[string]json.RawMessage`) to detect presence
   - Locations: `config.go:355-361` and `config.go:266-276`

### 2. **System Prompt Assembly**
   - Separate static (cacheable) and dynamic (per-session) sections
   - Join with boundary marker for caching infrastructure
   - Pass additional context (agent defs, CLAUDE.md) as separate parameter

### 3. **Config Change Flow (TUI)**
   - Toggle: negate current value
   - Display: format as string "true"/"false"
   - Save: use raw JSON merge to preserve false values
   - Apply: update both config and BashTool immediately for live effect

### 4. **Scope Detection**
   - Use raw JSON key presence to determine project vs global scope
   - Same pattern in both merge logic and UI display

### 5. **Tool Integration**
   - BashTool has `OutputFilterEnabled` boolean field
   - Set at app initialization from `settings.OutputFilter`
   - Update live during runtime via TUI config panel
