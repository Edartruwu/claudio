# Investigation Report: Session State, Agent/Team Flags, Task Events, and Config Mutations

## Q1 — Task Completion Events

### CompleteByIDs & CompleteByAssignee
**Location:** `internal/tools/tasks.go:122-156`

```go
// CompleteByIDs:122-139
func (s *TaskStore) CompleteByIDs(ids []string, status string) []*Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	var affected []*Task
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[strings.TrimPrefix(id, "#")] = true
	}
	for _, t := range s.tasks {
		if idSet[t.ID] && (t.Status == "pending" || t.Status == "in_progress") {
			t.Status = status
			t.UpdatedAt = time.Now()
			s.saveToDB(t)
			affected = append(affected, t)
		}
	}
	return affected
}

// CompleteByAssignee:143-156
func (s *TaskStore) CompleteByAssignee(agentName, status string) []*Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	var affected []*Task
	for _, t := range s.tasks {
		if t.AssignedTo == agentName && (t.Status == "pending" || t.Status == "in_progress") {
			t.Status = status
			t.UpdatedAt = time.Now()
			s.saveToDB(t)
			affected = append(affected, t)
		}
	}
	return affected
}
```

**Finding:** Neither method calls `bus.Publish()`. They only mutate in-memory state and save to DB. **No event is fired.**

### TaskCreateTool Pattern (Event Publishing)
**Location:** `internal/tools/tasks.go:202-241`

```go
func (t *TaskCreateTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	// ... parse input ...
	store := GlobalTaskStore
	store.mu.Lock()
	store.nextID++
	id := fmt.Sprintf("%d", store.nextID)
	task := &Task{ /* ... */ }
	store.tasks[id] = task
	store.saveToDB(task)
	store.mu.Unlock()

	// Publish event ← THIS IS THE PATTERN
	if t.bus != nil {
		payload, _ := json.Marshal(attach.TaskCreatedPayload{
			ID:          id,
			Title:       in.Subject,
			Description: in.Description,
			AssignedTo:  in.AssignedTo,
			Status:      "pending",
		})
		t.bus.Publish(bus.Event{
			Type:    attach.EventTaskCreated,
			Payload: payload,
		})
	}
	return &Result{Content: fmt.Sprintf("Task #%s created: %s", id, in.Subject)}, nil
}
```

**Key Pattern:**
1. Marshal payload to `attach.TaskCreatedPayload`/`attach.TaskUpdatedPayload`
2. Call `t.bus.Publish(bus.Event{Type: attach.EventTaskCreated/Updated, Payload: payload})`
3. ComandCenter receives via `internal/cli/root.go:153-176` subscribe handler

**Gap:** `CompleteByIDs` and `CompleteByAssignee` do not publish `EventTaskUpdated`. Need to add:
```go
// In both methods, after t.saveToDB(t):
if needsPublish {  // e.g., attach client exists
	payload, _ := json.Marshal(attach.TaskUpdatedPayload{
		ID:     t.ID,
		Status: status,
	})
	bus.Publish(bus.Event{
		Type:    attach.EventTaskUpdated,
		Payload: payload,
	})
}
```

---

## Q2 — Session Schema + Agent/Team Fields

### Sessions Table Schema
**Location:** `internal/storage/db.go:76-179` (migrations)

```sql
-- Migration 1 (base table)
CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	title TEXT NOT NULL DEFAULT '',
	project_dir TEXT NOT NULL DEFAULT '',
	model TEXT NOT NULL DEFAULT '',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	summary TEXT DEFAULT ''
)

-- Migration 7: add parent_session_id
ALTER TABLE sessions ADD COLUMN parent_session_id TEXT NOT NULL DEFAULT ''

-- Migration 8: add agent_type ✓
ALTER TABLE sessions ADD COLUMN agent_type TEXT NOT NULL DEFAULT ''

-- Migration 20: add team_template ✓
ALTER TABLE sessions ADD COLUMN team_template TEXT NOT NULL DEFAULT ''
```

**Latest migration version:** 22 (memory_fts_meta table)

### Session Struct
**Location:** `internal/storage/sessions.go:11-22`

```go
type Session struct {
	ID              string
	Title           string
	ProjectDir      string
	Model           string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Summary         string
	ParentSessionID string // non-empty for sub-agent sessions
	AgentType       string // e.g. "general-purpose", "Explore"
	TeamTemplate    string // e.g. "backend-team", optional template name
}
```

**Columns verified:**
- ✓ `agent_type` column (migration 8)
- ✓ `team_template` column (migration 20)

---

## Q3 — --agent and --team Flag Wiring

### Flag Declaration
**Location:** `internal/cli/root.go:43-57`

```go
var (
	flagModel               string
	flagAgent               string  // ← Set by --agent flag
	flagTeam                string  // ← Set by --team flag
	// ...
)

// Initialization
rootCmd.PersistentFlags().StringVar(&flagAgent, "agent", "", "Run as a specific agent persona (e.g., prab, backend-senior)")
rootCmd.PersistentFlags().StringVar(&flagTeam, "team", "", "Pre-load a team template at startup (e.g., backend-team)")
```

### Agent/Team Resolution Flow

#### Headless+Attach Mode (runHeadlessAttach)
**Location:** `internal/cli/root.go:360-401`

```go
// Line 360-387: Create session and fall back to stored config
sess := session.New(appInstance.DB)
// ... session creation/resume ...

// Fall back to stored agent/team config when CLI flags are absent.
if cur := sess.Current(); cur != nil {
	if flagAgent == "" {
		flagAgent = cur.AgentType  // ← Restore from DB
	}
	if flagTeam == "" {
		flagTeam = cur.TeamTemplate  // ← Restore from DB
	}
}

// Line 390-401: Load team from flag
if flagTeam != "" {
	templatesDir := config.GetPaths().TeamTemplates
	if tmpl, err := teams.GetTemplate(templatesDir, flagTeam); err == nil {
		instantiateTeamDirect(tmpl, sessionID)
	}
}
```

#### Agent Loading (Headless)
**Location:** `internal/cli/root.go:425-441`

```go
reg, modelOverride, extraPluginInfos := applyAgentOverrides(appInstance.Tools)
// flagAgent checked here ↓

// Line 432-434: Get capabilities from agent definition
var caps []string
if flagAgent != "" {
	caps = agents.GetAgent(flagAgent).Capabilities
}
```

**applyAgentOverrides Function:** `internal/cli/root.go:664-738`
```go
func applyAgentOverrides(registry *tools.Registry) (*tools.Registry, string, []prompts.PluginInfo) {
	if flagAgent == "" {
		return registry, "", nil
	}
	agentDef := agents.GetAgent(flagAgent)  // ← Load agent by name
	filtered := registry.Clone()
	for _, name := range agentDef.DisallowedTools {
		filtered.Remove(name)
	}
	// ... filter tools, merge skills, load plugins ...
	model := agentDef.Model
	if resolved, ok := appInstance.API.ResolveModelShortcut(model); ok {
		model = resolved
	}
	return filtered, model, extraPluginInfos
}
```

#### TUI Mode (runInteractive)
**Location:** `internal/cli/root.go:1080-1108`

```go
// Line 1081-1092: Apply --agent flag
if flagAgent != "" {
	agentDef := agents.GetAgent(flagAgent)
	msg := agentselector.AgentSelectedMsg{
		AgentType:       agentDef.Type,
		DisplayName:     agentDef.Type,
		SystemPrompt:    agentDef.SystemPrompt,
		Model:           agentDef.Model,
		DisallowedTools: agentDef.DisallowedTools,
		Capabilities:    agentDef.Capabilities,
	}
	model = model.ApplyAgentPersonaAtStartup(msg)
}

// Line 1094-1108: Apply --team flag
if flagTeam != "" {
	teamTemplatesDir := config.GetPaths().TeamTemplates
	tmpl, err := teams.GetTemplate(teamTemplatesDir, flagTeam)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: team template %q not found: %v\n", flagTeam, err)
	} else {
		msg := teamselector.TeamSelectedMsg{
			TemplateName: tmpl.Name,
			Description:  tmpl.Description,
			Members:      tmpl.Members,
		}
		model = model.ApplyTeamContextAtStartup(msg, appCtx)
	}
}
```

### Where Agent Config Carried
**Type:** `agents.AgentDefinition` (resolved via `agents.GetAgent(flagAgent)`)
**Lifecycle:**
- Flags set via `--agent` and `--team` at CLI startup
- Resume fallback → checks `sess.Current().AgentType` and `sess.Current().TeamTemplate`
- Never re-checked from flags during session; always uses resumed/stored values after init

---

## Q4 — applyConfigChange Scope

**Location:** `internal/tui/root.go:5286-5315`

```go
func (m *Model) applyConfigChange(key, value string) {
	switch key {
	case "model":
		m.model = value
		m.apiClient.SetModel(value)
		m.addMessage(ChatMessage{Type: MsgSystem, Content: fmt.Sprintf("Model changed to %s", value)})
		m.refreshViewport()
	case "permissionMode":
		if m.engineConfig != nil {
			m.engineConfig.PermissionMode = value
		}
	case "outputStyle":
		if m.appCtx != nil && m.appCtx.Config != nil {
			m.appCtx.Config.OutputStyle = value
		}
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
	}
	// Other settings (autoMemoryExtract, memorySelection, compactMode, etc.)
	// are read from config at the point of use, so saving to disk is sufficient.
}
```

**Mutations:**
- ✓ `model` → `m.model`, `m.apiClient`
- ✓ `permissionMode` → `m.engineConfig.PermissionMode`
- ✓ `outputStyle` → `m.appCtx.Config.OutputStyle`
- ✓ `outputFilter` → `m.appCtx.Config.OutputFilter`, `BashTool.OutputFilterEnabled`

**Does NOT mutate:**
- ✗ Active agent (no case for "agent")
- ✗ Active team (no case for "team")

**Note:** Other config (autoMemoryExtract, memorySelection, compactMode) read at point-of-use; changes saved but not cached in live structs.

---

## Q5 — Session Resume Path

### Resume in Headless+Attach
**Location:** `internal/cli/root.go:360-377` (flagName-based resume) + `internal/cli/root.go:390-401` (team/agent restore)

```go
sess := session.New(appInstance.DB)
projectDir, _ := os.Getwd()
if flagName != "" {
	if existing, err := sess.FindByTitle(flagName, projectDir); err == nil && existing != nil {
		if _, err := sess.Resume(existing.ID); err != nil && flagVerbose {
			fmt.Fprintf(os.Stderr, "Warning: failed to resume session %q: %v\n", flagName, err)
		}
	} else {
		if _, err := sess.Start(appInstance.Config.Model); err != nil && flagVerbose {
			fmt.Fprintf(os.Stderr, "Warning: failed to start session: %v\n", err)
		}
		_ = sess.SetTitle(flagName)
	}
}

// Fall back to stored agent/team config
if cur := sess.Current(); cur != nil {
	if flagAgent == "" {
		flagAgent = cur.AgentType      // ← Restored from DB
	}
	if flagTeam == "" {
		flagTeam = cur.TeamTemplate    // ← Restored from DB
	}
}
```

**Note:** Does not re-read agent/team from anywhere else — always uses CLI flags first, then falls back to stored session values.

### Resume in TUI Mode
**Location:** `internal/cli/root.go:964-994`

```go
sess := session.New(appInstance.DB)
if flagResume != "" {
	if flagResume == "last" {
		recent, err := sess.RecentForProject(1)
		if err != nil || len(recent) == 0 {
			if _, err := sess.Start(appInstance.Config.Model); err != nil && flagVerbose {
				fmt.Fprintf(os.Stderr, "Warning: failed to start session: %v\n", err)
			}
		} else {
			if _, err := sess.Resume(recent[0].ID); err != nil {
				return fmt.Errorf("failed to resume last session: %w", err)
			}
		}
	} else {
		resumed, err := sess.Resume(flagResume)
		if err != nil {
			return fmt.Errorf("failed to resume session %q: %w", flagResume, err)
		}
	}
} else {
	// Don't create a session yet — lazy on first message
}
```

**Note:** TUI does NOT restore agent/team from stored session automatically. Instead, relies on flag values or user action via TUI.

---

## Q6 — EventConfigChanged / EventAgentChanged & Full Event List

### Event Types Defined
**Location:** `internal/attach/protocol.go:8-30`

```go
// Events: Claudio → ComandCenter
const (
	EventSessionHello    = "session.hello"
	EventMsgAssistant    = "message.assistant"
	EventMsgToolUse      = "message.tool_use"
	EventTaskCreated     = "task.created"
	EventTaskUpdated     = "task.updated"
	EventAgentStatus     = "agent.status"
	EventSessionBye      = "session.bye"
	EventDesignScreenshot = "design.screenshot"
	EventDesignBundleReady = "design.bundle_ready"
	EventMsgStreamDelta    = "message.stream_delta"
	EventMsgToolResult     = "message.tool_result"
)

// Events: ComandCenter → Claudio
const (
	EventMsgUser       = "message.user"
	EventInterrupt     = "session.interrupt"
	EventSetAgent      = "set_agent"
	EventSetTeam       = "set_team"
	EventClearHistory  = "session.clear"
)
```

**Finding:** No `EventConfigChanged` or `EventAgentChanged` exists. Only:
- **Claudio → ComandCenter:** 11 events (task, agent status, design, messages)
- **ComandCenter → Claudio:** 5 events (`EventSetAgent`, `EventSetTeam` for agent/team changes)

### EventSetAgent / EventSetTeam Payloads
**Location:** `internal/attach/protocol.go:32-40`

```go
type SetAgentPayload struct {
	AgentType string `json:"agent_type"`
}

type SetTeamPayload struct {
	TeamName string `json:"team_name"`
}
```

### Consumer in Headless+Attach
**Location:** `internal/cli/root.go:403-416` (OnSetTeam handler)

```go
attachClient.OnSetTeam(func(payload attach.SetTeamPayload) {
	templatesDir := config.GetPaths().TeamTemplates
	tmpl, err := teams.GetTemplate(templatesDir, payload.TeamName)
	if err != nil {
		log.Printf("set_team: team template %q not found", payload.TeamName)
		return
	}
	sessionID := ""
	if cur := sess.Current(); cur != nil {
		sessionID = cur.ID
	}
	instantiateTeamDirect(tmpl, sessionID)
})
```

**Note:** `OnSetAgent` handler NOT shown — appears to exist but not wired in headless flow yet.

---

## Summary of Gaps & Implementation Needs

1. **Task Event Publishing:** `CompleteByIDs()` and `CompleteByAssignee()` need to fire `EventTaskUpdated` events.
2. **Config Change Events:** No event types for config mutations (model, permission, output style). `applyConfigChange()` mutates local state only.
3. **Agent Change Events:** `EventSetAgent` payload exists but no consumer handler shown in current code.
4. **Session Agent/Team Persistence:** Schema supports storage (migrations 8 & 20); restore flow partially implemented (headless mode restores from DB if CLI flags absent).
5. **TUI Agent/Team Restore:** TUI mode does not auto-restore agent/team from resumed session; relies on flag or user action.
