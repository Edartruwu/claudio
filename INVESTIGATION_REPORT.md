# Investigation Report: /clear and /compact Flow Analysis

## Subject
Deep trace of TUI/Web clear and compact flows, including event routing, database mutations, engine state sync, and test coverage.

---

## Codebase Overview
- **CLI entry:** `internal/cli/root.go` (engine setup, event callbacks)
- **TUI UI:** `internal/tui/root.go` (user interactions, action dispatch)
- **Web UI:** `internal/comandcenter/web/server.go` (HTTP handlers, WS push)
- **Engine:** `internal/query/engine.go` (in-memory message state)
- **App layer:** `internal/app/app.go` (DB ops, event publishing)
- **IPC protocol:** `internal/attach/protocol.go` (event defs)
- **WS client:** `internal/cli/attachclient/client.go` (event callbacks, attachment to ComandCenter)
- **Tests:** `internal/app/app_test.go`, `internal/comandcenter/web/server_test.go`

---

## Key Findings

### 1. TUI /clear Flow
- **Location:** `internal/tui/root.go:2993-3005`
- **Trigger:** User presses key/button → `output == "[action:clear]"`
- **Actions:**
  1. Wipe UI: `m.messages = nil`
  2. Reset state: `m.streamText.Reset()`, `m.turns = 0`, tokens/cost = 0
  3. Engine sync: `m.engine.SetMessages(nil)` 
  4. **No DB call** — in-memory only
  5. Refresh viewport

### 2. TUI /compact Flow
- **Location:** `internal/tui/root.go:390-400`
- **Actions:**
  1. Get engine messages: `msgs := m.engine.Messages()`
  2. Build pinned indices
  3. Call compact service: `compact.Compact(...)`
  4. Engine sync: `m.engine.SetMessages(compacted)`
  5. **No DB call in TUI** — persistence via OnAutoCompact callback (CLI layer)

### 3. Web /clear Flow
- **Location:** `internal/comandcenter/web/server.go:697-723`
- **Actions:**
  1. **DB delete:** `ws.storage.DeleteMessages(sessionID)`
  2. **DB cleanup:** `ws.storage.DeleteNativeMessages(sessionID)`
  3. Insert confirm bubble
  4. **Send event:** `ws.hub.Send(sessionID, clearEnv)` → EventClearHistory to attached claudio
  5. Push `{"type": "messages.cleared"}` to browser clients
  6. Return 204

### 4. Web /compact Flow
- **Location:** `internal/comandcenter/web/server.go:883-990`
- **Actions:**
  1. Load messages: `ws.storage.ListMessages(sessionID, 1000)`
  2. Return 202 immediately
  3. Spawn goroutine:
     - Call compact service
     - **DB replace:** Delete + re-insert all messages
     - Also update native storage
     - Push confirm bubble
  4. **CRITICAL GAP:** No event sent to claudio engine → stale in-memory state

### 5. CLI Event Reception
- **Location:** `internal/cli/root.go:476-478`
- **EventClearHistory callback:**
  ```go
  attachClient.OnClearHistory(func() {
      appInstance.ClearHistory(currentSessionID)
  })
  ```
  - Calls `app.ClearHistory()` → DB delete + publish EventClearHistory
  - **Does NOT call `engine.SetMessages(nil)`** → engine state stale

### 6. Engine.SetMessages Signature
- **Location:** `internal/query/engine.go:269-271`
- Simple assignment: `e.messages = msgs`
- **NOT thread-safe** (no mutex)
- Used after: TUI clear/compact, CLI init, session switch, undo/redo

### 7. Attach Protocol Events
- **Location:** `internal/attach/protocol.go:33`
- Event: `EventClearHistory = "session.clear"` (ComandCenter → Claudio)
- Payload: `ClearHistoryPayload{SessionID string}`
- **No EventSetMessages exists** → web compact can't notify engine

### 8. App Layer
- **Location:** `internal/app/app.go:654-663`
- `ClearHistory()`: Delete DB + publish EventClearHistory on bus
- Does NOT touch engine state

---

## Symbol Map

| Symbol | File | Role |
|--------|------|------|
| action clear handler | `internal/tui/root.go:2993` | UI wipe + engine.SetMessages(nil) |
| compact callback | `internal/tui/root.go:390-400` | Engine.SetMessages(compacted) after service |
| OnAutoCompact callback | `internal/cli/root.go:489-492` | Persist + save summary after engine compact |
| /clear handler | `internal/comandcenter/web/server.go:697-723` | Delete DB, send EventClearHistory |
| handleCompact | `internal/comandcenter/web/server.go:883-990` | Async compact, replace DB (missing engine event) |
| SetMessages | `internal/query/engine.go:269` | Direct slice assignment (not thread-safe) |
| ClearHistory | `internal/app/app.go:654` | Delete DB + publish event (no engine sync) |
| OnClearHistory | `internal/cli/attachclient/client.go:148` | Callback register for EventClearHistory |

---

## Tests

### Clear
1. `TestApp_ClearHistory_PublishesEvent` (app_test.go:82-114): Publishes EventClearHistory
2. `TestHandleSendMessage_Clear` (web/server_test.go:784-829): /clear deletes DB messages
3. `TestHandleSendMessage_Clear_AlsoClearsNativeMessages` (web/server_test.go:912-981): /clear deletes both storages

### Compact
1. `TestHandleSendMessage_Compact_NoAPIClient` (web/server_test.go:832-909): Returns 503 if no API client
2. `TestHandleSendMessage_Compact_ReadsNativeMessages` (web/server_test.go:986-1026): Reads native messages
3. `TestHandleSendMessage_Compact_NothingToCompact` (web/server_test.go:1029-1057): Empty history returns early

---

## Client Callbacks

Registered on `Client` (internal/cli/attachclient/client.go):

| Callback | Line | Event | Signature |
|----------|------|-------|-----------|
| OnUserMessage | 127-132 | EventMsgUser | `func(attach.UserMsgPayload)` |
| OnInterrupt | 134-139 | EventInterrupt | `func()` |
| OnSetAgent | 141-146 | EventSetAgent | `func(attach.SetAgentPayload)` |
| OnClearHistory | 148-153 | EventClearHistory | `func()` |
| OnSetTeam | 155-160 | EventSetTeam | `func(attach.SetTeamPayload)` |

**Missing:** OnCompact, OnSetMessages

---

## Critical Gaps & Risks

### 🔴 Web /compact Does Not Notify Claudio Engine
- Web compact deletes + re-inserts all DB messages
- **No EventSetMessages or EventCompactDone sent to claudio**
- If claudio attached: engine.messages ≠ DB after web compact
- **Fix:** Add EventCompactDone to protocol, send after goroutine completes

### 🟡 Engine.SetMessages Not Thread-Safe
- Direct assignment: `e.messages = msgs` (no mutex)
- Concurrent read/write → data race potential
- **Likelihood:** Low (single-threaded event loops), but fragile

### 🟡 TUI /clear Doesn't Persist to DB
- Clears UI + engine only, no DB delete
- User switches to web UI → messages reappear
- **Likely intended behavior:** TUI ephemeral, DB persistent

### 🟡 App.ClearHistory Doesn't Sync Engine
- Deletes DB + publishes event, but doesn't call engine.SetMessages(nil)
- Engine keeps stale messages until restart
- **Fix:** Accept engine ref, call SetMessages(nil)

---

## Open Questions

1. Is TUI /clear meant to be ephemeral? (not persist to DB)
2. Should web /compact send engine event?
3. Should engine.SetMessages acquire mutex?
4. Why doesn't app.ClearHistory take engine ref?

---

## Summary: What Gets Deleted

| Op | TUI UI | Engine | DB | Web | Claudio Event |
|----|--------|--------|-----|-----|-----|
| TUI /clear | ✅ | ✅ | ❌ | ❌ | ❌ |
| Web /clear | ✅ | ❌ | ✅ | ✅ | ✅ (engine not updated) |
| TUI /compact | ✅ | ✅ | ✅ | ❌ | ❌ |
| Web /compact | ✅ | ❌ | ✅ | ✅ | ❌ **MISSING** |

---

Report complete. All findings verified against source code.
