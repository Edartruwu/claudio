# Investigation Report: Config Tab Agent/Team Display Bug

## Subject
Why ComandCenter Config tab always shows "Default (no agent)" even when claudio started with `--agent` flag.

## Codebase Overview

**Key paths:**
- ComandCenter web server: `internal/comandcenter/web/server.go` (106 symbols)
- Session model (DB): `internal/storage/sessions.go`, `internal/comandcenter/session.go`
- CLI startup: `internal/cli/root.go`
- Attach protocol: `internal/attach/protocol.go`, `internal/cli/attachclient/client.go`
- Config tab (web): reads from `/api/sessions` endpoint

**Two separate session stores:**
1. `sessions` table — used by internal TUI, populated by `internal/storage/sessions.go`
2. `cc_sessions` table — used by ComandCenter web UI, populated by `internal/comandcenter/storage.go`

**Entry points:**
- ComandCenter spawns claudio via launcher: `cmd/comandcenter/launcher.go`
- Session connects to ComandCenter via WebSocket attachment: `internal/cli/attachclient/client.go`
- Config tab reads from `/api/sessions` → `handleAPISessions` → `ListSessions()` on `cc_sessions` table

---

## Key Findings

### Finding 1: Config Tab Reads From `cc_sessions.agent_type` (Always Empty)
- **Location:** `internal/comandcenter/web/server.go:1334-1345` (handleAPISessions)
- **Location:** `internal/comandcenter/storage.go:328-364` (ListSessions queries cc_sessions)
- **Description:** Config tab endpoint returns list of sessions from `cc_sessions` table, reading `agent_type` column (with COALESCE to empty string on NULL).
- **Call chain:** 
  - Config tab JS → `GET /api/sessions/list` 
  - → `handleAPISessions` (server.go:1334)
  - → `storage.ListSessions("")` (storage.go:328)
  - → SQL: `SELECT ... COALESCE(agent_type,'') ... FROM cc_sessions` (storage.go:329)
- **Data touched:** `cc_sessions.agent_type` column — initialized to NULL/empty, never populated during startup
- **Result:** Always reads empty string, displays "Default (no agent)"

### Finding 2: CLI Flag `--agent` Never Saved to `cc_sessions` Table
- **Location:** `internal/cli/root.go:221` (flag defined)
- **Location:** `internal/cli/root.go:952-963` (flag applied to TUI at startup)
- **Description:** `--agent` flag is read and applied to the TUI model's system prompt + engine configuration, but is never persisted to ANY database table.
- **Call chain:**
  - Root.go:221 — flag defined
  - Root.go:952-963 — flag passed to TUI startup (ApplyAgentPersonaAtStartup)
  - TUI applies agent to local model state only
  - No DB write occurs
- **Data touched:** TUI state, engine registry, system prompt — no DB tables modified
- **Scope:** TUI-only impact; ComandCenter web UI has no visibility

### Finding 3: HelloPayload (Attachment Protocol) Missing Agent/Team Fields
- **Location:** `internal/attach/protocol.go:44-50` (HelloPayload struct)
- **Location:** `internal/cli/attachclient/client.go:17-30` (Client struct)
- **Location:** `internal/cli/attachclient/client.go:80-86` (Connect method builds hello)
- **Description:** When claudio connects to ComandCenter via WebSocket, it sends HelloPayload with only Name, Path, Master fields. No AgentType or TeamTemplate fields exist in the protocol.
- **Struct definition:**
  ```go
  // HelloPayload (protocol.go:45-50) — missing agent+team
  type HelloPayload struct {
    Name   string
    Path   string
    Model  string,omitempty
    Master bool,omitempty
  }
  ```
- **How it's built (client.go:81-85):**
  ```go
  hello := attach.HelloPayload{
    Name:   c.name,
    Path:   cwd,
    Master: c.master,
    // NO agent or team provided!
  }
  ```
- **Root cause:** Client struct also missing agent/team fields (client.go:17-30)
- **Impact:** ComandCenter hub has no way to receive agent/team value from the claudio process at connect time

### Finding 4: AttachClient Created Without Agent/Team Info
- **Location:** `internal/cli/root.go:143` (attachClient.New)
- **Location:** `internal/cli/attachclient/client.go:32-40` (New constructor)
- **Description:** When attachClient created, it's passed only serverURL, password, name, master — agent and team flags not provided.
- **Code:**
  ```go
  // root.go:143
  client := attachclient.New(flagAttach, password, flagName, flagMaster)
  // flagAgent and flagTeam NOT passed!
  ```
- **Missing fields in Client struct:**
  ```go
  // client.go:17-30
  type Client struct {
    serverURL, password, name, master // ✓ populated
    // NO agent, team fields
  }
  ```
- **Impact:** AttachClient can't send agent/team to ComandCenter even if it wanted to

### Finding 5: Session Pre-Registration POST Body Doesn't Accept Agent/Team
- **Location:** `internal/comandcenter/server.go:142-182` (handlePreRegisterSession)
- **Description:** When launcher spawns a session, it immediately POSTs to `/api/sessions` to pre-register (so UI shows "active" before claudio connects). But the POST body struct only accepts Name, Path, Master — no agent/team fields.
- **Code (server.go:143-147):**
  ```go
  var body struct {
    Name   string
    Path   string
    Master bool
    // NO AgentType, TeamTemplate
  }
  ```
- **Call chain:**
  - `cmd/comandcenter/launcher.go:139-168` builds CLI args with `--agent` + `--team`
  - Args passed to `claudio --attach ... --agent X --team Y` subprocess
  - But NO HTTP POST sends agent/team to hub during pre-registration
  - Session created in `cc_sessions` with `agent_type = NULL` (server.go:161-175)
- **Impact:** Even if claudio receives agent/team via CLI, that info never reaches cc_sessions table

### Finding 6: Session Created in `cc_sessions` With Empty Agent/Team
- **Location:** `internal/comandcenter/server.go:161-175` (buildSession in handlePreRegisterSession)
- **Description:** Session struct initialized with only Name, Path, Master, Status, CreatedAt, LastActiveAt. AgentType and TeamTemplate fields exist (comandcenter/session.go:19-20) but are left zero-valued when session first created.
- **Code (server.go:161-175):**
  ```go
  sess := Session{
    Name:         body.Name,
    Path:         body.Path,
    Master:       body.Master,
    Status:       "active",
    CreatedAt:    now,
    LastActiveAt: now,
    // AgentType and TeamTemplate left as empty strings!
  }
  ```
- **Where they could be set later:** Only via explicit `UpdateSessionConfig` call (storage.go:245), which is triggered by `handleSetAgent`/`handleSetTeam` when user manually changes in Config tab.
- **Impact:** Agent/team only appear in Config tab AFTER user manually selects them; startup-provided values lost

---

## Symbol Map

| Symbol | File | Role |
|--------|------|------|
| `Session` (cc struct) | `internal/comandcenter/session.go:10-21` | Web session model with `AgentType`, `TeamTemplate` fields |
| `HelloPayload` | `internal/attach/protocol.go:44-50` | Attachment hello message — missing agent/team fields |
| `Client` (attachclient) | `internal/cli/attachclient/client.go:17-30` | ComandCenter connection — missing agent/team fields |
| `ListSessions` (Storage) | `internal/comandcenter/storage.go:328` | Fetches sessions from `cc_sessions` table |
| `UpdateSessionConfig` (Storage) | `internal/comandcenter/storage.go:245` | Updates `cc_sessions.agent_type`, `team_template` |
| `handleAPISessions` (WebServer) | `internal/comandcenter/web/server.go:1334` | GET `/api/sessions/list` endpoint for Config tab |
| `handlePreRegisterSession` (Server) | `internal/comandcenter/server.go:142` | POST `/api/sessions` pre-registration (launcher calls) |
| `handleSetAgent` (WebServer) | `internal/comandcenter/web/server.go:1405` | Handler for manual agent selection in Config tab |
| `ApplyAgentPersonaAtStartup` | `internal/tui/root.go:1918` | Applies agent to TUI state (not persisted to DB) |
| `flagAgent` | `internal/cli/root.go:45, 221` | CLI flag `--agent` definition |
| `buildCmd` (launcher) | `cmd/comandcenter/launcher.go:139` | Constructs claudio process args with `--agent`, `--team` |

---

## Dependencies & Data Flow

### Startup Flow (Where It Breaks)

1. **ComandCenter launcher** reads `cc-config.json` with SessionConfig containing Agent + Team fields
2. **buildCmd** (launcher.go:139) adds `--agent X --team Y` to claudio subprocess args
3. **Launcher POST** pre-registers session to `/api/sessions` **WITHOUT agent/team in body**
   - Session created in `cc_sessions` with `agent_type = NULL`
4. **Claudio process starts** with `--agent X --team Y` flags
5. **root.go:143** creates `attachClient.New()` **WITHOUT passing agent/team**
6. **root.go:952-963** applies `flagAgent` to TUI state (local only, no DB)
7. **attachClient.Connect()** sends `HelloPayload` **missing agent/team fields**
8. **ComandCenter hub** receives hello, updates session status to "active" but has no agent/team data
9. **Config tab loads** `/api/sessions`, reads `cc_sessions.agent_type = NULL`, displays "Default (no agent)"

### Current Workaround (Manual Selection)

1. User opens Config tab
2. Selects agent from dropdown → `POST /api/sessions/{id}/set-agent` → `handleSetAgent` (web/server.go:1405)
3. Handler calls `UpdateSessionConfig` → writes to `cc_sessions.agent_type`
4. Next Config tab load reads the saved value

---

## Risks & Observations

### Critical Issues

1. **Agent/Team Startup Values Lost**
   - Launcher correctly passes `--agent X` to subprocess
   - Claudio process receives and applies it (TUI works fine)
   - But value never reaches ComandCenter database
   - Config tab always shows empty even though agent is active in terminal

2. **Protocol Gap**
   - `HelloPayload` designed before agent feature added to ComandCenter
   - Attachment protocol incomplete — no way to transmit agent/team at connect time

3. **Attachment Client Incomplete**
   - Client struct has no agent/team fields
   - Constructor signature doesn't accept them
   - Even if protocol fixed, client wouldn't have the data to send

4. **Two Session Stores Desynchronized**
   - `sessions` table (internal TUI DB) updated by claudio process
   - `cc_sessions` table (web UI DB) only updated via explicit HTTP calls
   - At startup, agent info flows to `sessions` but not `cc_sessions`

### Dead Paths

- `UpdateSessionConfig` in storage.go only called by web handlers (`handleSetAgent`, `handleSetTeam`)
- No code path calls it during session initialization
- Agent/team from CLI never reach `cc_sessions`

### Symptom vs Root Cause

- **Symptom:** Config tab shows "Default (no agent)"
- **Root cause:** Multiple missing links:
  1. AttachClient lacks agent/team fields
  2. HelloPayload protocol doesn't include agent/team
  3. Server hello handler doesn't expect agent/team
  4. No code persists agent/team to `cc_sessions` at startup

---

## Open Questions

1. **Should agent/team be persisted to `cc_sessions` at startup?**
   - If yes, which component should persist: launcher? claudio process? ComandCenter hub?

2. **Is the two-table design intentional?**
   - `sessions` table managed by internal session manager
   - `cc_sessions` table managed by ComandCenter
   - Or should they be unified?

3. **What about `--model` flag?**
   - Is it also not persisted to `cc_sessions.model` at startup?
   - Same issue likely applies

---

## Implementation Path (Not Executed — Investigation Only)

To fix, need to:

1. **Extend HelloPayload** (protocol.go) with `AgentType`, `TeamTemplate` fields
2. **Extend AttachClient.Client** struct with `agent`, `team` fields
3. **Modify attachclient.New** constructor to accept agent/team params
4. **Pass flagAgent, flagTeam to attachclient.New** in root.go:143
5. **Build HelloPayload with agent/team** in client.go:81-85
6. **Parse agent/team in ComandCenter hub** when handling hello
7. **Create session in cc_sessions with agent/team** from hello payload instead of pre-registration
   - OR update pre-registration endpoint to accept agent/team in POST body
   - OR have hub update session after receiving hello

Would also need to verify `--model` flag has similar fix.
