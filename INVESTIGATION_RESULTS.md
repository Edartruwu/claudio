# Investigation: `internal/teams` Package

## Summary
Comprehensive analysis of the teams package including Manager, ListTemplates, TeamTemplate, and existing template selection/spawning logic.

---

## 1. Manager Type/Struct

### Definition
**Location:** `internal/teams/team.go:82-87`

```go
// Manager handles team lifecycle and coordination.
type Manager struct {
	mu           sync.RWMutex
	teamsDir     string
	templatesDir string
	active       map[string]*TeamConfig // keyed by team name
}
```

### Fields
- **`mu`** (`sync.RWMutex`) - Reader/writer mutex for thread-safe access to the active teams map
- **`teamsDir`** (`string`) - Base directory where team configurations are stored (e.g., `~/.claudio/teams/`)
- **`templatesDir`** (`string`) - Directory where team templates are stored (e.g., `~/.claudio/team-templates/`)
- **`active`** (`map[string]*TeamConfig`) - In-memory cache of loaded teams, keyed by team name

### Constructor
**`NewManager(teamsDir, templatesDir string) *Manager`** (lines 89-102)
- Creates both directories with mode 0700 if they don't exist
- Initializes an empty `active` map
- Calls `m.loadActive()` to load all existing team configs from disk into the in-memory cache

### Manager Methods

1. **`SaveAsTemplate(teamName, templateName string) (*TeamTemplate, error)`** (lines 105-135)
   - Saves a team's non-lead members as a reusable template
   - Returns the saved `*TeamTemplate` or error

2. **`ListTemplates() []TeamTemplate`** (lines 138-140)
   - Returns all saved team templates (full list, not paginated)

3. **`GetTemplate(name string) (*TeamTemplate, error)`** (lines 143-145)
   - Returns a single template by name

4. **`CreateTeam(name, description, sessionID, model string) (*TeamConfig, error)`** (lines 148-191)
   - Creates a new team with the calling agent as lead
   - Pre-populates with a single lead member ("team-lead@{name}")
   - Creates team directories and saves config to disk

5. **`SetAutoCompactThreshold(teamName string, threshold int)`** (lines 194-201)
   - Sets the team-level auto-compact threshold (percentage)

6. **`SetMemberAdvisorConfig(teamName, agentName string, cfg *AdvisorConfig)`** (lines 204-220)
   - Stores an AdvisorConfig on an existing team member
   - Called by InstantiateTeam after AddMember to persist advisor spec

7. **`AddMember(teamName, agentName, model, prompt, subagentType string, autoCompactThreshold ...int) (*TeamMember, error)`** (lines 223-266)
   - Adds a teammate to an existing team
   - Handles duplicate members: replaces if previous is in terminal state, errors if still working
   - Auto-assigns color based on member index

8. **`UpdateMemberStatus(teamName, agentID string, status MemberStatus)`** (lines 269-285)
   - Changes a member's status (enum: StatusIdle, StatusWorking, StatusComplete, etc.)

9. **`UpdateMemberSystemPrompt(teamName, agentID, systemPrompt string)`** (lines 288-304)
   - Persists a member's resolved system prompt

10. **`GetTeam(name string) (*TeamConfig, bool)`** (lines 307-312)
    - Returns a team by name (read-only lookup)

11. **`ListTeams() []*TeamConfig`** (lines 315-323)
    - Returns all active teams as pointers

12. **`RemoveMember(teamName, agentID string) error`** (lines 326-341)
    - Removes a member from team config (safe to call on inactive agents)

13. **`DeleteTeam(name string) error`** (lines 344-366)
    - Removes a team and cleans up its directory
    - Checks for active members first, errors if any non-lead member is still working

14. **`GetMember(teamName, agentID string) (*TeamMember, bool)`** (lines 369-383)
    - Returns a member by agent ID (read-only lookup)

15. **`ActiveMembers(teamName string) []*TeamMember`** (lines 386-402)
    - Returns non-lead members that are currently working

16. **`AllMembers(teamName string) []*TeamMember`** (lines 405-421)
    - Returns all non-lead members (regardless of status)

17. **`TeamsDir() string`** (lines 424-426)
    - Returns the base teams directory

18. **`FormatTeamStatus(teamName string) string`** (lines 429-465)
    - Returns a human-readable team summary with member statuses

19. **`saveConfig(team *TeamConfig) error`** (lines 467-477)
    - Private method: saves team config to disk as JSON

20. **`loadActive()`** (lines 479-498)
    - Private method: loads all team configs from disk into memory during initialization

---

## 2. ListTemplates Function

### Full Signature and Implementation
**Location:** `internal/teams/team.go:138-140`

```go
// ListTemplates returns all saved team templates.
func (m *Manager) ListTemplates() []TeamTemplate {
	return LoadTemplates(m.templatesDir)
}
```

### Returns
- **Type:** `[]TeamTemplate` (slice of TeamTemplate structs)
- **Behavior:**
  - Delegates to the package-level `LoadTemplates()` function
  - Reads all `*.json` files from `m.templatesDir`
  - Returns parsed templates as a slice
  - Returns `nil` or empty slice if directory doesn't exist or has no templates

### Implementation Detail: LoadTemplates()
**Location:** `internal/teams/templates.go:37-55`

```go
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
```

---

## 3. TeamTemplate Type

### Definition
**Location:** `internal/teams/templates.go:28-34`

```go
// TeamTemplate is a reusable team composition stored at ~/.claudio/team-templates/{name}.json.
type TeamTemplate struct {
	Name                 string               `json:"name"`
	Description          string               `json:"description,omitempty"`
	Model                string               `json:"model,omitempty"` // team default model
	AutoCompactThreshold int                  `json:"autoCompactThreshold,omitempty"` // % context to trigger compact for all members
	Members              []TeamTemplateMember `json:"members"`
}
```

### Fields for Name/Description
- **`Name`** (`string`, `json:"name"`) - Required; template name (e.g., "backend-team", "research-team")
- **`Description`** (`string`, `json:"description,omitempty"`) - Optional; human-readable description

### Additional Fields
- **`Model`** (`string`) - Optional team-level default model override
- **`AutoCompactThreshold`** (`int`) - Optional; percentage of context window to trigger auto-compaction
- **`Members`** (`[]TeamTemplateMember`) - Required; list of pre-defined team member slots

### TeamTemplateMember Sub-struct
**Location:** `internal/teams/templates.go:19-25`

```go
type TeamTemplateMember struct {
	Name                 string          `json:"name"`
	SubagentType         string          `json:"subagent_type"`
	Model                string          `json:"model,omitempty"`                  // per-member model override
	AutoCompactThreshold int             `json:"autoCompactThreshold,omitempty"`   // % context to trigger compact (overrides team-level)
	Advisor              *AdvisorConfig  `json:"advisor,omitempty"`
}
```

---

## 4. Template Selection Logic

### A. Team Template Picker Component
**Location:** `internal/tui/teamselector/selector.go`

A full Bubble Tea TUI component for selecting templates from the disk.

#### Key Exports:
- **`TeamSelectedMsg`** - Message sent when user picks a template
  - `TemplateName` - Template name (empty if ephemeral)
  - `IsEphemeral` - True if user chose "new ephemeral team"
  - `Description` - Template description
  - `Members` - Roster summary

- **`DismissMsg`** - Sent when user cancels (Esc)

- **`Model`** - The picker component (implements `tea.Model` interface)
  - `New(templatesDir string) Model` - Constructor loads templates from disk
  - `Update(msg tea.Msg) (Model, tea.Cmd)` - Handles key events (j/k navigate, filter, enter, esc)
  - `View() string` - Renders a 2-pane overlay:
    - **Left pane:** Filterable list of templates + ephemeral option
    - **Right pane:** Description and member roster of selected template

#### Features:
- Real-time filtering (type to search)
- Navigation: j/k or arrow keys
- Selection: Enter
- Dismissal: Esc
- Shows member roster with names and subagent types
- "ephemeral" option always appears first (for ad-hoc teams)

---

### B. InstantiateTeam Tool
**Location:** `internal/tools/teamtemplate.go:78-184`

Tool that creates a team from a saved template, pre-registering all members.

#### Signature:
```go
type InstantiateTeamTool struct {
	deferrable
	Runner           *teams.TeammateRunner
	Manager          *teams.Manager
	GetSessionID     func() string
	InstantiatedTeam string // name of team created
}

func (t *InstantiateTeamTool) Name() string { return "InstantiateTeam" }

func (t *InstantiateTeamTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error)
```

#### Input Schema:
```json
{
  "template_name": "string (required)",
  "team_name": "string (optional, but recommended)"
}
```

#### Workflow:
1. Load template by name via `Manager.GetTemplate()`
2. Create team via `Manager.CreateTeam()`
3. Pre-register all members via `Manager.AddMember()`
4. If member has `Advisor` config, call `Manager.SetMemberAdvisorConfig()`
5. Set active team in runner: `Runner.SetActiveTeam(teamName)`
6. Return formatted roster message

#### Key Feature:
- Team name defaults to "{template_name}-{sessionID[:8]}" if not provided
- Documentation emphasizes always providing a scoped team_name to avoid conflicts

---

### C. SpawnTeammate Tool
**Location:** `internal/tools/spawnteammate.go:14-314`

Tool that spawns a named team member through the TeammateRunner.

#### Signature:
```go
type SpawnTeammateTool struct {
	deferrable
	Runner          *teams.TeammateRunner
	Manager         *teams.Manager
	SessionID       string
	AvailableModels []string
}

func (t *SpawnTeammateTool) Name() string { return "SpawnTeammate" }

func (t *SpawnTeammateTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error)
```

#### Input Schema:
```json
{
  "name": "string (required, e.g. 'a1', 'tester')",
  "subagent_type": "string (required, e.g. 'backend-mid')",
  "prompt": "string (required, the task)",
  "model": "string (optional override)",
  "max_turns": "number (optional, 0 = unlimited)",
  "run_in_background": "boolean (optional, default true)",
  "task_ids": "array[string] (optional)",
  "isolation": "string (optional, 'worktree' for git isolation)"
}
```

#### Workflow:
1. Auto-create a default team if none is active (`Manager.CreateTeam()`)
2. Resolve agent name (auto-suffix if already running: "maya" → "maya-2")
3. Spawn via `Runner.Spawn(SpawnConfig{...})`
4. If `run_in_background: false`, wait synchronously via `Runner.WaitForOne()`

#### Key Features:
- **Parallel instance handling:** Auto-suffixes duplicate names (maya → maya-2, maya-3, etc.)
- **Upsert semantics:** Re-spawning a finished agent creates a clean instance (no history)
- **Task integration:** Prepends assigned task metadata to prompt
- **Name for messaging:** Resolved name used in `SendMessage()` calls

---

### D. TeammateRunner.Spawn()
**Location:** `internal/teams/runner.go:341-440`

Low-level spawn function called by SpawnTeammate.

#### Signature:
```go
type SpawnConfig struct {
	TeamName             string
	AgentName            string
	Prompt               string
	System               string         // system prompt override
	Model                string         // model override
	SubagentType         string         // agent definition used
	MaxTurns             int            // optional max agentic turns
	Isolation            string         // "worktree" for git worktree isolation
	MemoryDir            string         // optional agent-scoped memory directory
	Foreground           bool           // true when blocking (suppresses task-notification)
	TaskIDs              []string       // task IDs to auto-complete on finish
	AutoCompactThreshold int            // % context window for auto-compact
	ParentAgentID        string         // for parent-child tracking
	AdvisorConfig        *AdvisorConfig // optional advisor tool injection
}

func (r *TeammateRunner) Spawn(cfg SpawnConfig) (*TeammateState, error)
```

#### Key Behavior:
- Resolves model/threshold from team defaults if not specified
- Resolves advisor config from stored member settings
- Defaults to worktree isolation if inside a git repo
- Creates worktree with branch: `claudio/{teamName}/{agentName}-{runID}`
- Adds member to team via `Manager.AddMember()`
- Launches goroutine to run agent via `r.runAgent()` callback
- Returns `*TeammateState` immediately (async by default)

---

## 5. Relationship Diagram

```
Manager (team.go)
  ├─ ListTemplates() → LoadTemplates()
  │   └─ reads ~/.claudio/team-templates/*.json
  │
  ├─ GetTemplate(name) → loads single template
  │
  ├─ CreateTeam() → creates TeamConfig on disk
  │
  ├─ AddMember() → mutates team, saves to disk
  │
  └─ SaveAsTemplate() → writes TeamTemplate to disk

TeamTemplate (templates.go)
  ├─ Name, Description (name/label fields)
  ├─ Members []*TeamTemplateMember (roster)
  └─ Model, AutoCompactThreshold (defaults)

TUI Picker (teamselector/selector.go)
  ├─ New(templatesDir) → creates Model
  ├─ Model.Update() → handles j/k/filter/enter/esc
  ├─ Model.View() → renders 2-pane overlay
  └─ Emits → TeamSelectedMsg or DismissMsg

InstantiateTeam Tool (teamtemplate.go)
  ├─ Load template via Manager.GetTemplate()
  ├─ Create team via Manager.CreateTeam()
  ├─ Pre-register members via Manager.AddMember() + SetMemberAdvisorConfig()
  └─ Returns roster summary

SpawnTeammate Tool (spawnteammate.go)
  ├─ Auto-create default team if needed
  ├─ Resolve name (auto-suffix duplicates)
  ├─ Call Runner.Spawn() with SpawnConfig
  └─ Wait (if foreground) or return immediately (background)

TeammateRunner.Spawn() (runner.go)
  ├─ Resolve model/threshold from team defaults
  ├─ Create git worktree if in repo
  ├─ Add member via Manager.AddMember()
  └─ Launch goroutine with agent context
```

---

## 6. Data Flow Example: Template → Team → Spawned Agent

1. **User picks template via TUI picker:**
   - Loads templates from disk via `LoadTemplates()`
   - User selects "backend-team" template
   - Picker emits `TeamSelectedMsg{TemplateName: "backend-team", Members: [...]}`

2. **Lead calls InstantiateTeam tool:**
   - Loads template: `Manager.GetTemplate("backend-team")`
   - Creates team: `Manager.CreateTeam("backend-team-proj1", desc, sessionID, model)`
   - Pre-registers members: `Manager.AddMember()` for each template member
   - Updates advisor config if present: `Manager.SetMemberAdvisorConfig()`
   - Sets active: `Runner.SetActiveTeam("backend-team-proj1")`

3. **Lead calls SpawnTeammate tool:**
   - Input: `{name: "migrator", subagent_type: "backend-mid", prompt: "..."}`
   - Resolves name: "migrator" (not running, so use as-is)
   - Calls: `Runner.Spawn(SpawnConfig{TeamName: "backend-team-proj1", AgentName: "migrator", ...})`
   - Runner:
     - Resolves model from team defaults
     - Creates worktree: `.claudio-worktrees/claudio/backend-team-proj1/migrator-{timestamp}`
     - Adds member: `Manager.AddMember("backend-team-proj1", "migrator", ...)`
     - Launches goroutine: `r.runAgent(ctx, systemPrompt, prompt)`

4. **Agent completes:**
   - Result stored in `TeammateState.Result`
   - Status updated: `Manager.UpdateMemberStatus(..., StatusComplete)`
   - Worktree commits if changes made

---

## Key Insights

### No Built-in Picker Function
- The teams package does **not** expose a high-level "pick template" function
- The picker is purely a TUI component (`teamselector/selector.go`)
- The tool layer (InstantiateTeam, SpawnTeammate) manages the selection workflow

### Template → Team → Member Flow
- **Templates** are static JSON files (immutable blueprints)
- **Teams** are mutable configs on disk with member status tracking
- **Members** are added dynamically; they can be spawned on-demand via SpawnTeammate

### Advisor Injection
- Advisors are stored in `TeamTemplateMember.Advisor` in the template
- When instantiating, advisor config is persisted to the team member via `SetMemberAdvisorConfig()`
- When spawning, advisor config is resolved from the team member's stored value
- Allows the tool layer to inject advisor tools at spawn time

### Thread Safety
- Manager uses `sync.RWMutex` for safe concurrent access to teams
- TeammateRunner uses separate locks for teammates and children
- All mutations go through Manager methods, ensuring consistent disk state

### Worktree Management
- Worktrees are created lazily in `runTeammate()`, not in `Spawn()`
- Branch naming: `claudio/{teamName}/{agentName}-{timestamp}`
- Prevents collisions with concurrent/repeated runs

