# Team Template Activation & Tool Registration Investigation

## Executive Summary

This document provides a comprehensive investigation of three key areas in the Claudio Go codebase:

1. **How team templates are activated** and where state is stored
2. **Where team-related tools are registered** and exposed to the principal agent
3. **The mechanism to conditionally exclude team tools** based on template activation

---

## 1. Team Template Activation Mechanism

### Key Finding: In-Memory `activeTeam` Field in TeammateRunner

**Storage Location:** `internal/teams/runner.go`, lines 266 & 936-954

```go
// Line 266: activeTeam field in TeammateRunner struct
type TeammateRunner struct {
    // ... other fields ...
    activeTeam string // explicitly set active team name
}

// Line 936-941: SetActiveTeam method
func (r *TeammateRunner) SetActiveTeam(name string) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.activeTeam = name
}

// Line 943-954: ActiveTeamName method (returns explicitly set team or infers from running teammates)
func (r *TeammateRunner) ActiveTeamName() string {
    r.mu.RLock()
    defer r.mu.RUnlock()
    if r.activeTeam != "" {
        return r.activeTeam
    }
    for _, s := range r.teammates {
        return s.TeamName
    }
    return ""
}
```

### How Team Templates Are "Activated"

**Mechanism:** Not a file-based activation system. Instead:

1. **Explicit Activation:** When a user selects a team template in the TUI
   - `internal/tui/root.go`, line 1815: `m.appCtx.TeamRunner.SetActiveTeam(teamName)`
   - This is called in the `applyTeamContext()` method when a `TeamSelectedMsg` is received

2. **Implicit Activation:** When TeamCreate tool runs
   - `internal/tools/teamcreate.go`, lines 98-99: `t.Runner.SetActiveTeam(team.Name)`
   - After creating a team, it becomes the active team

3. **State Source:** In-memory session state, **not persisted to disk**
   - No config file tracks the "active team"
   - Active team is determined by: (a) explicit SetActiveTeam call, or (b) inferred from running teammates

### Team Template Storage

**File Storage:** `~/.claudio/team-templates/{name}.json`

**Related Files:**
- `internal/teams/templates.go` (lines 28-100): Template struct definitions and load/save logic
  - `LoadTemplates()` (line 37): Loads all *.json files from the templates directory
  - `GetTemplate(name)` (line 58): Loads single template by name
  - `SaveTemplate(name, tmpl)` (line 78): Writes template to disk

**Template Struct** (`internal/teams/templates.go`, lines 28-35):
```go
type TeamTemplate struct {
    Name        string
    Description string
    Members     []TeamTemplateMember
    Model       string
}

type TeamTemplateMember struct {
    Name         string
    SubagentType string
    Model        string
    AdvisorType  string
}
```

### Detection Points in Code

Where "an active team template is detected":

1. **In SaveTeamTemplateTool** (`internal/tools/teamtemplate.go`, lines 44-71)
   - Line 52: `teamName := t.Runner.ActiveTeamName()`
   - Line 53: Returns error if `teamName == ""`
   - **Validation pattern:** Runtime check, not registration-time filtering

2. **In SpawnTeammateTool** (`internal/tools/spawnteammate.go`)
   - Accepts team context via active team set on Runner
   - Auto-creates default team if none active (lines not shown, but inferred from comments)

3. **In TeamCreateTool** (`internal/tools/teamcreate.go`, lines 98-99)
   - After creating team, sets it as active

---

## 2. Tool Registration for Team Tools

### Registration Location

**File:** `internal/tools/registry.go`, lines 357-362

```go
// Team management (Manager injected later)
r.Register(&TeamCreateTool{deferrable: newDeferrable("create team multi-agent collaboration")})
r.Register(&TeamDeleteTool{deferrable: newDeferrable("delete remove team")})
r.Register(&SendMessageTool{deferrable: newDeferrable("send message to team agent")})
r.Register(&SpawnTeammateTool{deferrable: newDeferrable("spawn teammate agent background parallel named")})
r.Register(&SaveTeamTemplateTool{deferrable: newDeferrable("save team template reuse composition")})
r.Register(&InstantiateTeamTool{deferrable: newDeferrable("instantiate team template load roster")})
```

### Registration Method

**In DefaultRegistry() function** (line 300):
- All team tools are registered in `DefaultRegistry()` unconditionally
- **Key point:** Team tools are NOT conditionally registered based on template state
- They are registered with the "deferred" flag (will be sent to LLM as schema-less names unless explicitly requested via ToolSearch)

### Team Tool Files

| Tool | File | Lines | Purpose |
|------|------|-------|---------|
| SpawnTeammate | `internal/tools/spawnteammate.go` | 14-200 | Spawn named teammate agent |
| TeamCreate | `internal/tools/teamcreate.go` | 11-110 | Create new agent team |
| TeamDelete | `internal/tools/teamdelete.go` | - | Delete team |
| SendMessage | `internal/tools/sendmessage.go` | - | Send message to team agent |
| SaveTeamTemplate | `internal/tools/teamtemplate.go` | 12-75 | Save team composition as template |
| InstantiateTeam | `internal/tools/teamtemplate.go` | 76-165 | Create team from saved template |

### Current Tool Registration Flow

**Location:** `internal/tools/registry.go`, DefaultRegistry() method

1. **All tools always registered** (line 300+)
2. **Team tools marked as "deferred"** (lines 357-362)
   - Means they won't be sent as full schemas initially to save tokens
   - But they remain in the tool list and can be discovered via ToolSearch
3. **No conditional filtering** based on team state

---

## 3. How Tools Are Exposed to Principal Agent

### Principal Agent Tool List Construction

**Flow:**

1. **Engine Creation** (`internal/query/engine.go`, lines 162-227)
   - Engine holds: `registry *tools.Registry`
   - Registry populated with ALL tools via `DefaultRegistry()`
   - System prompt set from base prompts

2. **Registry Cloning for Agent Personas** (`internal/tui/root.go`, lines 1724-1777)
   - Line 1745: `filtered := m.registry.Clone()`
   - Lines 1746-1748: Remove disallowed tools by name
   - Line 1763: `m.engine.SetRegistry(filtered)`
   - **This is the FILTERING POINT for agent personas**

3. **Team Context Applied** (`internal/tui/root.go`, lines 1781-1821)
   - Line 1815: `m.appCtx.TeamRunner.SetActiveTeam(teamName)`
   - Team context appended to system prompt (informational, not filtering)
   - NO filtering of tools happens here

### Registry Methods

**Location:** `internal/tools/registry.go`

| Method | Line | Purpose |
|--------|------|---------|
| `Register(t Tool)` | 34-37 | Add tool to registry |
| `All()` | 49-55 | Return all registered tools |
| `Clone()` | 227-245 | Create filtered copy (deep clone with fresh caches) |
| `Remove(name)` | 213-221 | Remove tool by name from cloned registry |
| `APIDefinitionsWithDeferral()` | 75-95 | Return tool schemas (omits undiscovered deferred tools) |
| `DeferredToolNames()` | 102-115 | Return names of deferred-loading tools |

### How System Prompt & Tool List Reach LLM

**Flow:**

1. **BuildSystemWithDeferredTools()** (`internal/query/engine.go`, lines 1206-1222)
   - Builds system prompt with:
     - Base system prompt
     - System context (if any)
     - **Frozen deferred tools reminder** (computed once per session)
   - Example:
     ```
     <system-reminder>
     The following deferred tools are available via ToolSearch:
     WebSearch
     WebFetch
     SpawnTeammate
     TeamCreate
     SendMessage
     ...
     </system-reminder>
     ```

2. **APIDefinitionsWithDeferral()** (`internal/tools/registry.go`, lines 75-95)
   - Called when building request to LLM
   - Returns eager tools as full schemas
   - Omits undiscovered deferred tools from the array (but names listed in system-reminder)
   - Deferred tools available via ToolSearch after model requests them

3. **Engine.buildRequest()** (not shown, but references above point to it)
   - Constructs the full request with:
     - `system`: buildSystemWithDeferredTools() output
     - `tools`: APIDefinitionsWithDeferral(discoveredTools) output

---

## 4. Current Conditional Logic for Team Tools

### Runtime Validation (NOT Registration Filtering)

**Current Pattern:** Team tools are NOT filtered from the registry based on whether a team is active. Instead, they **validate at execution time**.

**Examples:**

1. **SaveTeamTemplateTool.Execute()** (`internal/tools/teamtemplate.go`, lines 44-71)
   ```go
   teamName := t.Runner.ActiveTeamName()
   if teamName == "" {
       return &Result{Content: "no active team — create or join a team first", IsError: true}, nil
   }
   ```
   - Returns error if no active team
   - Tool remains in registry but fails gracefully at runtime

2. **TeamCreateTool.Execute()** (`internal/tools/teamcreate.go`, lines 78-100)
   - Creates team unconditionally
   - Sets it as active (line 99)
   - No pre-condition check

3. **SpawnTeammateTool**
   - Likely accepts team context from active team
   - May auto-create default team if none active

### Why No Registration-Time Filtering?

1. **Deferred tools are infrastructure for token saving** — not context-based filtering
2. **Agent personas have hard filtering** via `DisallowedTools` (applies to agent-type constraints, not context-based state)
3. **Team state is dynamic** — can change during conversation (team created → becomes active)
4. **Graceful degradation preferred** — tools are available but return helpful errors if preconditions aren't met

---

## 5. Where to Gate Team Tools on Template Activation

### Recommendation: Add Conditional Filtering in TUI Layer

**Location:** `internal/tui/root.go`, in `applyTeamContext()` method (currently lines 1781-1821)

**Current Code (lines 1781-1821):**
```go
func (m Model) applyTeamContext(msg teamselector.TeamSelectedMsg) Model {
    // ... creates team and sets active ...
    m.appCtx.TeamRunner.SetActiveTeam(teamName)
    // ...
}
```

**Proposed Enhancement:**

Add a companion method `removeTeamContext()` or modify `applyTeamContext()` to also handle tool filtering:

```go
func (m Model) applyTeamContext(msg teamselector.TeamSelectedMsg) Model {
    // ... existing team creation logic ...
    m.appCtx.TeamRunner.SetActiveTeam(teamName)
    
    // NEW: Filter registry to include team tools
    filtered := m.registry.Clone()
    // Team tools can stay — they validate at runtime
    // (No additional filtering needed for team tools specifically)
    m.engine.SetRegistry(filtered)
    
    // ... existing system prompt append ...
}

func (m Model) removeTeamContext() Model {
    // Called when user de-activates team template
    m.appCtx.TeamRunner.SetActiveTeam("")  // Clear active team
    
    // NEW: If you want to filter team tools when NO team is active:
    // filtered := m.registry.Clone()
    // for _, name := range []string{"SpawnTeammate", "TeamCreate", ...} {
    //     filtered.Remove(name)
    // }
    // m.engine.SetRegistry(filtered)
}
```

### Alternative: Keep Tools Available (Recommended)

**Why the above may NOT be needed:**

1. **Team tools already validate at runtime** — SaveTeamTemplateTool returns error if no active team
2. **Deferred tools are lighter cost** — team tools are deferred, so they don't bloat the initial tool list
3. **Better UX** — user can call TeamCreate without needing to "enable" team mode first
4. **Simpler implementation** — no need for additional filtering logic

**If you DO want to gate team tools:**

- **Cleanest place:** TUI layer, in `applyTeamContext()` (line 1781) or a new `applyTeamMode()` method
- **Mechanism:** Clone registry, remove team tools, set filtered registry on engine
- **Caveat:** You'd need to manage when to remove team tools (when de-activating team, closing team, etc.)

### Clean Place to Gate If Desired

**Option A: In TUI root.go (highest level, closest to user intent)**
- `applyTeamContext()` method (line 1781)
- Can access registry and engine directly
- Already handles team state setup

**Option B: In Engine itself (lower level)**
- Add method like `SetTeamMode(active bool)` to Engine
- Automatically filter/restore tools based on flag
- More contained logic, but less context about user's intent

**Option C: In Registry (lowest level)**
- Add method like `FilterForTeamContext(active bool)` to Registry
- But this mixes concerns (registry shouldn't know about "team context")
- Less recommended

---

## 6. File Paths & Line Numbers Summary

### Team Template Activation

| Component | File | Lines | What |
|-----------|------|-------|------|
| `activeTeam` field | `internal/teams/runner.go` | 266 | Storage for active team name |
| `SetActiveTeam()` | `internal/teams/runner.go` | 936-941 | Set active team |
| `ActiveTeamName()` | `internal/teams/runner.go` | 943-954 | Get active team (explicit or inferred) |
| TUI activation | `internal/tui/root.go` | 1815 | Set active team from TeamSelectedMsg |
| Team struct | `internal/teams/templates.go` | 28-35 | TeamTemplate and TeamTemplateMember |
| Load templates | `internal/teams/templates.go` | 37-56 | LoadTemplates() and GetTemplate() |

### Tool Registration

| Component | File | Lines | What |
|-----------|------|-------|------|
| Team tools registered | `internal/tools/registry.go` | 357-362 | Register all 6 team tools |
| DefaultRegistry() | `internal/tools/registry.go` | 300 | Entry point for all tool registration |
| Remove() | `internal/tools/registry.go` | 213-221 | Remove tool by name |
| Clone() | `internal/tools/registry.go` | 227-245 | Create filtered registry copy |

### Tool List Exposure to Principal Agent

| Component | File | Lines | What |
|-----------|------|-------|------|
| Engine struct | `internal/query/engine.go` | 81-129 | Registry field + system prompt |
| SetRegistry() | `internal/query/engine.go` | 163 | Set registry on engine |
| SetSystem() | `internal/query/engine.go` | 230-231 | Set system prompt on engine |
| buildSystemWithDeferredTools() | `internal/query/engine.go` | 1206-1222 | Append deferred tools reminder |
| APIDefinitionsWithDeferral() | `internal/tools/registry.go` | 75-95 | Return tool schemas with deferral |
| applyAgentPersona() | `internal/tui/root.go` | 1724-1777 | Filter registry for agent, set on engine |
| applyTeamContext() | `internal/tui/root.go` | 1781-1821 | Set active team, append system prompt |

### Runtime Validation Examples

| Tool | File | Lines | Check |
|------|------|-------|-------|
| SaveTeamTemplate | `internal/tools/teamtemplate.go` | 52-55 | Validates active team exists |
| TeamCreate | `internal/tools/teamcreate.go` | 88-99 | Creates unconditionally, sets as active |

---

## 7. Key Insights

### 1. No Persistent "Active Team Template" Config
- Active team is stored in-memory in `TeammateRunner.activeTeam` field
- No `~/.claudio/config.json` or similar file tracks it
- State is session-scoped only

### 2. Team Tools Are Always Registered
- All 6 team tools (SpawnTeammate, TeamCreate, SendMessage, TeamDelete, SaveTeamTemplate, InstantiateTeam) are unconditionally registered in `DefaultRegistry()`
- They are **deferred tools** (schema-less in initial tool list, discoverable via ToolSearch)
- They are NOT filtered based on template activation state

### 3. Tool Filtering Happens at TUI Layer
- **Filtering point:** `applyAgentPersona()` in `internal/tui/root.go` (line 1745-1763)
- Registry is cloned, disallowed tools removed, filtered registry set on engine
- This is where **agent personas** filter tools (based on `DisallowedTools` list)
- **Team context does NOT trigger tool filtering** — only appends system prompt

### 4. Team Tools Validate at Runtime
- SaveTeamTemplateTool returns error if no active team
- Better UX than filtering tools (user can discover error message)
- Follows principle of graceful degradation

### 5. System Prompt Includes Deferred Tools Reminder
- Built once per session in `buildSystemWithDeferredTools()` (line 1206)
- Frozen in `frozenDeferredReminder` to avoid busting Anthropic prompt cache
- Lists all deferred tool names so LLM knows to use ToolSearch to request them

---

## 8. Recommendation for Gating Team Tools

**Do NOT gate team tools on template activation** unless you have a specific reason to hide them. Here's why:

1. **Already deferred** — They don't bloat the initial tool list
2. **Runtime validation** — Tools gracefully fail if preconditions aren't met
3. **Better UX** — User can discover errors in LLM output rather than being blocked upfront
4. **Simpler implementation** — No need for additional filtering logic

**If you DO need to gate them:**

- **Cleanest place:** TUI `applyTeamContext()` method in `internal/tui/root.go` (line 1781)
- **Mechanism:** Clone registry, remove team tools, set filtered registry on engine via `SetRegistry()`
- **Alternative:** Add a `SetTeamMode(active bool)` method to Engine that auto-filters

---

## Appendix: Code References

### SaveTeamTemplateTool Runtime Check
File: `internal/tools/teamtemplate.go`, lines 44-71
```go
func (t *SaveTeamTemplateTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
    var in struct {
        Name string `json:"name"`
    }
    if err := json.Unmarshal(input, &in); err != nil || in.Name == "" {
        return &Result{Content: "name is required", IsError: true}, nil
    }

    teamName := t.Runner.ActiveTeamName()
    if teamName == "" {
        return &Result{Content: "no active team — create or join a team first", IsError: true}, nil
    }
    // ... rest of implementation ...
}
```

### Registry Clone & Remove
File: `internal/tools/registry.go`, lines 213-245
```go
func (r *Registry) Remove(name string) {
    delete(r.tools, name)
    for i, n := range r.order {
        if n == name {
            r.order = append(r.order[:i], r.order[i+1:]...)
            break
        }
    }
}

func (r *Registry) Clone() *Registry {
    clone := NewRegistry()
    for _, name := range r.order {
        clone.Register(r.tools[name])
    }
    for k, v := range r.deferOverride {
        clone.deferOverride[k] = v
    }
    // ... fresh caches setup ...
    return clone
}
```

### Agent Persona Tool Filtering
File: `internal/tui/root.go`, lines 1745-1763
```go
func (m Model) applyAgentPersona(msg agentselector.AgentSelectedMsg) Model {
    // ... validation ...
    
    // Build filtered registry from the original (not previously filtered) registry
    filtered := m.registry.Clone()
    for _, name := range msg.DisallowedTools {
        filtered.Remove(name)
    }

    // ... other setup ...
    
    // Propagate to live engine if it already exists
    if m.engine != nil {
        m.engine.SetSystem(base)
        m.engine.SetRegistry(filtered)  // ← FILTERING HAPPENS HERE
    }
    
    // Store so future engine creation picks it up
    m.systemPrompt = base
    m.registry = filtered

    // ...
}
```

### Team Context Activation
File: `internal/tui/root.go`, lines 1815
```go
func (m Model) applyTeamContext(msg teamselector.TeamSelectedMsg) Model {
    // ...
    m.appCtx.TeamRunner.SetActiveTeam(teamName)  // ← TEAM BECOMES ACTIVE HERE
    
    // Record in InstantiateTeamTool so engine.Close() cleans up.
    // ... rest of team setup ...
}
```
