# Claudio Tool Registration Architecture: TUI vs Web/ComandCenter Mode

## Overview

Claudio supports three execution paths:
1. **TUI Mode** — Interactive terminal UI with BubbleTea (default when stdout is a terminal)
2. **Headless Mode** — Single-turn query processing with print output
3. **Web Mode (ComandCenter attach)** — Headless engine attached to remote ComandCenter web server

All three modes **share identical tool registry** — there is no separate "web tools" vs "TUI tools" set.

---

## Entry Points & Control Flow

### TUI Mode (Interactive Terminal)
```
cmd/claudio/main.go
  └─> internal/cli.Execute()
      └─> internal/cli/root.go runInteractive()
          ├─> appInstance = app.New(settings, projectRoot)
          │   └─> tools.DefaultRegistry() [internal/tools/registry.go]
          ├─> registry = applyAgentOverrides(appInstance.Tools)
          │   └─> tools.Registry cloned, potentially modified by agent persona
          ├─> reg.Register(advisorTool) [if advisor configured]
          ├─> tui.New(appInstance.API, reg, systemPrompt, session, opts...)
          │   └─> internal/tui/root.go Model struct
          └─> p := tea.NewProgram(model, tea.WithAltScreen())
              └─> BubbleTea event loop
                  └─> prompt.SubmitMsg → Update() → engine.Run()
```

**Key wiring:**
- `appInstance` created before CLI branching
- `appInstance.Tools` is `tools.Registry` populated by `tools.DefaultRegistry()`
- Registry passed to `tui.New()` as parameter
- TUI Model stores registry, no server-side fields (no `ccPusher`, `sessionID`, `webMode`)

---

### Web/ComandCenter Attach Mode (Headless Engine)
```
cmd/claudio/main.go
  └─> internal/cli.Execute()
      └─> internal/cli/root.go runHeadlessAttach()
          │  [--attach flag set, forces --headless=true]
          ├─> appInstance = app.New(settings, projectRoot)
          │   └─> tools.DefaultRegistry()
          ├─> attachClient = attachclient.New(flagAttach, password, flagName, flagMaster)
          │   └─> internal/cli/attachclient/client.go
          ├─> appInstance.InjectAttachClient(attachClient, flagAttach)
          │   └─> Injects into SpawnSession/SpawnTeam/AssignTask tools only
          ├─> registry = applyAgentOverrides(appInstance.Tools)
          ├─> handler = attachclient.NewEventProxy(query.StdoutHandler, attachClient)
          │   └─> Proxies query engine events to ComandCenter
          ├─> engine = query.NewEngineWithConfig(apiClient, registry, handler, cfg)
          │   └─> internal/query/engine.go [NOT TUI Model]
          ├─> engine.Run(ctx, initialPrompt)
          │   └─> Processes initial turn, then enters loop reading app.InjectCh
          └─> attachClient.OnUserMessage(payload => app.InjectPayload(payload))
              └─> Receives PWA user messages → engine processes next turn
```

**Key differences:**
- Uses `query.Engine` directly (not TUI Model)
- AttachClient proxies messages bidirectionally
- Tool registry identical to TUI, injected via same path
- ComandCenter **does not** create or manage engine; it receives WebSocket attach from Claudio CLI

---

### Single-Turn Headless Mode (Print Output)
```
cmd/claudio/main.go
  └─> internal/cli.Execute()
      └─> runSinglePrompt(args)
          ├─> appInstance = app.New(settings, projectRoot)
          ├─> registry = applyAgentOverrides(appInstance.Tools)
          ├─> handler = query.StdoutHandler{Verbose}
          │   └─> OR: attachclient.NewEventProxy(handler, attachClient)
          ├─> engine = query.NewEngineWithConfig(apiClient, registry, handler, cfg)
          └─> engine.Run(ctx, prompt) → exit
```

---

## Tool Registry Architecture

### Registry Creation
**File:** `internal/tools/registry.go`

```go
func DefaultRegistry() *Registry {
    r := New()
    // Register all ~25 tools:
    r.Register(NewBashTool())
    r.Register(NewFileReadTool())
    r.Register(NewFileWriteTool())
    r.Register(NewGlobTool())
    r.Register(NewGrepTool())
    r.Register(NewRenderMockupTool(designsDir))
    r.Register(NewBundleMockupTool(designsDir))
    r.Register(...other tools...)
    return r
}
```

### Security Injection (App Level)
**File:** `internal/app/app.go` New()

```go
// After DefaultRegistry() created:
bash, _ := registry.Get("Bash")
if bt, ok := bash.(*tools.BashTool); ok {
    bt.Security = &SecurityContext{DenyPaths, AllowPaths}
    bt.OutputFilterEnabled = settings.OutputFilter
}

read, _ := registry.Get("Read")
if rt, ok := read.(*tools.FileReadTool); ok {
    rt.Security = sec
    rt.Config = settings
}
// ... etc for Write, LSP, MCP, etc
```

**Result:** Registry with security configured, passed to CLI before branching

### Agent Persona Override
**File:** `internal/cli/root.go` applyAgentOverrides()

```go
reg := appInstance.Tools.Clone() // Fresh copy
// Agent's DisallowedTools filtered from registry
agentDef := agents.GetAgent(flagAgent)
for _, toolName := range agentDef.DisallowedTools {
    reg.Remove(toolName)
}
// Return modified registry
```

### Team Tools Injection
**File:** `internal/tui/root.go` New()

```go
// Base registry includes all tools + team tools
m.baseRegistry = m.registry
// Working registry removes team tools initially
m.registry = m.registry.Clone()
for _, name := range tools.TeamToolNames {
    m.registry.Remove(name)  // SpawnSession, SpawnTeam, AssignTask
}

// When team activated (Update → ApplyTeamContextAtStartup):
// Restore team tools to working registry
```

---

## Tool Configuration by Mode

### What's Different?

#### TUI Mode
- **Registry Location:** Stored in `Model.registry`
- **Agent Selection:** Interactive (user selects via UI)
- **Team Selection:** Interactive (user selects via UI)
- **Tool Approval:** Interactive prompts
- **Model Selection:** Switchable via command `:model`

#### Web Mode (ComandCenter)
- **Registry Location:** Stored in `query.Engine` (no model struct)
- **Agent Selection:** HTTP POST `/api/sessions/{id}/set-agent` → `hub.SetAgent()` → stored in session
- **Team Selection:** HTTP POST `/api/sessions/{id}/set-team` → `hub.SetTeam()` → stored in session
- **Tool Approval:** Same (permission checks apply identically)
- **Model Selection:** HTTP request can include `model_override`

#### Headless Mode (Single-turn)
- **Registry Location:** Stored in `query.Engine`
- **Agent Selection:** CLI flag `--agent`
- **Team Selection:** CLI flag `--team`
- **Tool Approval:** Same
- **Model Selection:** CLI flag `--model`

### What's the Same?

✅ **Tool Set** — Both modes use `tools.DefaultRegistry()`, identical 25+ tools  
✅ **Security** — Both modes use same SecurityContext injection  
✅ **Permissions** — Both modes check permission rules identically  
✅ **Handlers** — Both modes route to query.Engine with tool execution logic  
✅ **File Paths** — Both modes respect AllowPaths/DenyPaths from config  
✅ **Output Filtering** — Both modes use FilterSavings service identically  

---

## How Web Server (ComandCenter) Manages Sessions

**File:** `internal/comandcenter/server.go` + `internal/comandcenter/web/server.go`

### Request Flow
```
Browser (ComandCenter UI) ← HTTPS
  ├─> POST /api/sessions/{id}/message
  │   └─> web/server.go handleSendMessage()
  │       └─> Convert to attach.UserMsgPayload
  │       └─> hub.Send(sessionID, envelope)
  │           └─> WebSocket to Claudio CLI process
  │
  └─> GET /api/agents, /api/teams, /api/projects
      └─> web/server.go handleAPIAgents()
          └─> agents.AllAgents(customDirs...) [read from disk]
```

### Key Point
**Web server does NOT**:
- Create engine instances
- Manage tool registry
- Execute tools
- Know about TUI Model

**Web server DOES**:
- Store session metadata (name, created_at, archived)
- Proxy WebSocket messages ↔ Claudio CLI
- Provide HTTP REST API for UI (agents, teams, projects, designs)
- Store design screenshots in `~/.claudio/designs/{timestamp}/screenshots/`

---

## TUI Model Struct Fields

**File:** `internal/tui/root.go` Model

**No ComandCenter-specific fields:**
```go
type Model struct {
    apiClient       *api.Client
    registry        *tools.Registry      // ← Tool registry
    baseRegistry    *tools.Registry      // ← Pristine copy (with team tools)
    session         *session.Session
    engine          *query.Engine
    appCtx          *tui.AppContext
    // ... 50+ other fields for TUI state (viewport, prompt, spinner, etc.)
    
    // NO: ccPusher, webMode, sessionID, serverURL, etc.
}
```

The TUI Model is **purely for terminal UI state**. It does not know about ComandCenter.

---

## Attach Protocol (ComandCenter ↔ Claudio CLI)

**File:** `internal/attach/protocol.go`

### Events: Claudio → ComandCenter (Outbound)
```go
const (
    EventSessionHello  = "session.hello"      // On connect
    EventMsgAssistant  = "message.assistant"  // Response text
    EventMsgToolUse    = "message.tool_use"   // Tool invocation (name + input)
    EventTaskCreated   = "task.created"       // When task spawned
    EventTaskUpdated   = "task.updated"       // When task status changes
    EventAgentStatus   = "agent.status"       // Agent idle/working/done
    EventSessionBye    = "session.bye"        // On disconnect
)
```

### Events: ComandCenter → Claudio (Inbound)
```go
const (
    EventMsgUser       = "message.user"       // User text from PWA
    EventInterrupt     = "session.interrupt"  // Cancel current turn
    EventSetAgent      = "set_agent"          // Change agent persona
    EventSetTeam       = "set_team"           // Activate team template
)
```

### No Screenshot Event
**Currently missing:** There is NO `EventScreenshotPushed` or `EventDesignUpdate` event in attach/protocol.go.

The screenshot capture workflow is:
1. User runs `RenderMockup(html_path)` tool in TUI or web session
2. Tool saves PNG files to `~/.claudio/designs/{timestamp}/screenshots/`
3. **No event is published** — web UI polls `GET /designs` to refresh gallery

---

## Designs Storage & Gallery

**File:** `internal/config/config.go`
```go
type Paths struct {
    Designs string // ~/.claudio/designs/
}
```

**File:** `internal/tools/render.go` RenderMockupTool
- Takes HTML path
- Invokes Node.js + Playwright to render + capture screenshots
- Saves to `{sessionDir}/screenshots/*.png`
- Default sessionDir: `~/.claudio/designs/{timestamp}/`

**File:** `internal/comandcenter/web/server.go`
```go
func (ws *WebServer) handleDesignGallery(w http.ResponseWriter, r *http.Request) {
    designsDir := config.GetPaths().Designs
    // Scan for sessions with screenshots/*.png, bundle/mockup.html, handoff/spec.md
    // Return HTML template: designs.html
}

func (ws *WebServer) handleDesignStatic(w http.ResponseWriter, r *http.Request) {
    // Serve static files: GET /designs/static/{id}/{rest...}
    // Resolve with path traversal protection
}
```

---

## Bus Events

**File:** `internal/bus/events.go` — defines all event types published on the event bus.

**Existing constants:**
- `EventSessionStart`, `EventSessionEnd`, `EventSessionCompact`
- `EventMessageUser`, `EventMessageAssistant`, `EventMessageSystem`
- `EventStreamStart`, `EventStreamChunk`, `EventStreamDone`, `EventStreamError`
- `EventToolStart`, `EventToolEnd`, `EventToolPermission`
- `EventAuthLogin`, `EventAuthLogout`, `EventAuthRefresh`
- `EventMCPConnect`, `EventMCPDisconnect`, `EventMCPToolCall`
- `EventInstinctLearned`, `EventInstinctEvolved`
- `EventRateLimitChanged`
- `EventAuditEntry`

**Missing:** No screenshot or design-related events in the **internal bus**.

---

## Answer to Key Questions

### 1. Is the TUI model used in web/CC mode?
**No.** Web mode (ComandCenter attach) uses `query.Engine` directly, not `tui.Model`. The TUI Model is only instantiated in interactive terminal mode.

### 2. How does the web server spin up agent/tool processing for a session?
**The web server does NOT.** The Claudio CLI process (running with `--attach`) creates the engine and tool registry. The web server is purely a UI/proxy — it forwards user messages via WebSocket to the attached Claudio CLI, which executes tools and sends results back.

### 3. Tool registration for web sessions?
**Same path as TUI:** `app.New() → tools.DefaultRegistry()` → passed to `query.NewEngineWithConfig()`. No server-side tool loading.

### 4. Any CC-mode wiring in root.go?
**No.** `internal/tui/root.go` has no "cc" or "comandcenter" or "web" references. It's purely TUI-focused. ComandCenter integration lives in `internal/cli/root.go` (attachClient setup) and `internal/comandcenter/web/server.go` (HTTP handlers).

### 5. Model struct fields related to CC/web mode?
**None.** Model contains registry, appCtx, session, engine — all generic, no server references.

### 6. Existing bus events?
**Yes.** 18 event types defined in `internal/bus/events.go` — no screenshot event currently.

### 7. Design screenshot event patterns?
**No pattern exists.** Screenshots are saved to disk; gallery is served via HTTP polling (`GET /designs`), not via event bus.

---

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                     CLI Entry Point                          │
│                  cmd/claudio/main.go                         │
│                    cli.Execute()                             │
└────────────────────────────┬────────────────────────────────┘
                             │
                 app.New(settings, projectRoot)
                    ↓ (creates before branching)
                tools.DefaultRegistry()  ← All 25+ tools
                    ↓
                Security injection (Bash, Read, Write, LSP, MCP)
                    ↓
        ┌───────────────────┬────────────────────┬──────────────┐
        │                   │                    │              │
    TUI Mode           Headless Mode        Web Mode (Attach)
   [TERMINAL]         [PRINT OUTPUT]       [REMOTE UI]
        │                   │                    │
        ├─ runInteractive()  ├─ runSinglePrompt()├─ runHeadlessAttach()
        │                   │                    │
        ├─ applyAgentOverrides()                │
        │                   │                    ├─ attachClient = new(url, pwd)
        ├─ tui.New(         │                    │
        │   apiClient,      ├─ query.NewEngine  │
        │   registry,       │   (apiClient,      ├─ query.NewEngine
        │   systemPrompt,   │    registry,       │   (apiClient,
        │   session)        │    handler)        │    registry,
        │                   │                    │    handler → EventProxy)
        ├─ tea.NewProgram   │  └─ engine.Run     │
        │   (model)         │     (ctx, prompt)  ├─ app.InjectAttachClient()
        │                   │                    │   (into team tools)
        │ BubbleTea loop    │ ↓ exits after      │
        │ ↓                 │ 1 turn             ├─ engine.Run(ctx, initialPrompt)
        │ Update()          │                    │  ↓ reads app.InjectCh
        │ ↓                 └────────────────────┤
        └─ engine.Run()         Shared:          ├─ WebSocket ↔ ComandCenter
           (via prompt           • Query engine  │  ↓
           SubmitMsg)            • Same registry │ eventProxy → CC UI
                                 • Tool exec     └─ Loop on injectCh messages
                                 • Permissions
```

---

## Summary for Implementation

### For adding a "Design Screenshot Pushed" event:

1. **Add event constant** in `internal/bus/events.go`:
   ```go
   const EventDesignScreenshotPushed = "design.screenshot_pushed"
   ```

2. **Define payload struct** in `internal/attach/protocol.go`:
   ```go
   type DesignScreenshotPayload struct {
       SessionID   string   `json:"session_id"`
       DesignID    string   `json:"design_id"`  // timestamp
       Screenshots []string `json:"screenshots"` // filenames
       BundlePath  string   `json:"bundle_path,omitempty"`
   }
   ```

3. **Publish from tool** in `internal/tools/render.go` or query engine:
   ```go
   appInstance.Bus.Publish(bus.Event{
       Type:    bus.EventDesignScreenshotPushed,
       Payload: json.Marshal(DesignScreenshotPayload{...}),
   })
   ```

4. **Add to attach protocol** if ComandCenter should know:
   ```go
   const EventDesignScreenshotPushed = "design.screenshot_pushed" // C→CC
   ```

5. **Subscribe in web UI** (or attach client) to refresh gallery automatically.

---

## Files Reference

| Path | Role |
|------|------|
| `cmd/claudio/main.go` | Entry point |
| `internal/cli/root.go` | CLI flag parsing, TUI/headless branching |
| `internal/cli/attachclient/client.go` | ComandCenter PWA attach client |
| `internal/app/app.go` | Dependency injection, tool security setup |
| `internal/tools/registry.go` | Tool registry creation |
| `internal/tools/render.go` | Screenshot capture tool |
| `internal/tools/bundle.go` | Design bundle handoff tool |
| `internal/tui/root.go` | TUI Model struct (25+ methods) |
| `internal/query/engine.go` | Query execution engine (used by web+headless) |
| `internal/comandcenter/server.go` | ComandCenter HTTP API routes |
| `internal/comandcenter/web/server.go` | Browser UI (templates, routes) |
| `internal/comandcenter/hub.go` | WebSocket session manager |
| `internal/attach/protocol.go` | Attach protocol constants + payloads |
| `internal/bus/events.go` | Event bus constants |
| `internal/config/config.go` | Paths struct (includes Designs) |

