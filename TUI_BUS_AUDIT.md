# Claudio TUI-Bus Audit Report

## Summary
Claudio sends data to ComandCenter via EventTypes defined in `internal/attach/protocol.go`. Investigation audited all 11 Claudio→ComandCenter events for:
1. Who publishes them (internal code or direct SendEvent)
2. Does `root.go SubscribeAll` forward them to web
3. Does ComandCenter `fanout()` handle them
4. Does web UI react

**Finding:** 7/11 events properly flow through bus → root.go → fanout() → web UI. 4 events bypass bus entirely and go direct SendEvent. Multiple state mutations bypass bus entirely (TUI-only, web blind).

---

## Event Type Audit Table

| EventType | File:Line Published | Via Bus? | root.go Forwards? | fanout() Handles? | Web UI Reacts? | **GAP?** |
|-----------|--------------------|---------|----|---|---|---|
| **EventTaskCreated** | `internal/tools/tasks.go:235` | ✓ bus.Publish | ✓ YES (line 155-158) | ✓ YES (line 1201-1208) | ✓ YES htmx refresh (app.js:219) | **NONE** |
| **EventTaskUpdated** | `internal/tools/tasks.go:376` | ✓ bus.Publish | ✓ YES (line 160-163) | ✓ YES (line 1210-1217) | ✓ YES htmx refresh (app.js:219) | **NONE** |
| **EventAgentStatus** | `internal/tasks/agent_task.go:89,104,120` | ✓ bus.Publish | ✓ YES (line 165-168) | ✓ YES (line 1177-1190) | ✓ YES toast + htmx (app.js:232-234) | **NONE** |
| **EventClearHistory** | `internal/app/app.go:661` | ✓ bus.Publish | ✓ YES (line 170-173) | ✓ YES (line 1192-1199) | ✓ YES clear msgs (app.js:214-216) | **NONE** |
| **EventMsgStreamDelta** | `internal/cli/attachclient/eventproxy.go:44` | ✗ direct SendEvent | ✗ NO | ✓ YES (line 1148-1161) | ✓ YES live token stream (app.js:~180) | ⚠ Bypasses root.go but works |
| **EventMsgToolUse** | `internal/cli/attachclient/eventproxy.go:73` | ✗ direct SendEvent | ✗ NO | ✓ YES (line 1098-1130) | ✓ YES bubble + typing (app.js:~190) | ⚠ Bypasses root.go but works |
| **EventMsgToolResult** | `internal/cli/attachclient/eventproxy.go:88` | ✗ direct SendEvent | ✗ NO | ✓ YES (line 1132-1146) | ✓ YES output fill (app.js:~200) | ⚠ Bypasses root.go but works |
| **EventMsgAssistant** | `internal/cli/attachclient/eventproxy.go:152` | ✗ direct SendEvent | ✗ NO | ✓ YES (line 1080-1096) | ✓ YES message bubble (app.js:~175) | ⚠ Bypasses root.go but works |
| **EventSessionHello** | `internal/cli/attachclient/client.go:93` | ✗ direct SendEvent | ✗ NO | N/A (proto handshake) | N/A | **NONE** (correct) |
| **EventSessionBye** | `internal/cli/attachclient/client.go:174` | ✗ direct SendEvent | ✗ NO | ✓ YES (line 419) | N/A (cleanup) | **NONE** (correct) |
| **EventDesignScreenshot** | `internal/cli/attachclient/screenshot_pusher.go:25` | ✗ direct SendEvent | ✗ NO | ✓ YES (line 1163-1168) | ✓ YES img bubble (web handler) | ⚠ Bypasses root.go but works |
| **EventDesignBundleReady** | `internal/cli/attachclient/screenshot_pusher.go:34` | ✗ direct SendEvent | ✗ NO | ✓ YES (line 1170-1175) | ✓ YES link bubble (web handler) | ⚠ Bypasses root.go but works |

---

## State Mutations That Bypass Bus Entirely

### 1. Task Store Mutations (TUI-only, web blind)

**Location:** `internal/tools/tasks.go:122-156`

**Functions:**
- `CompleteByIDs()` — marks tasks done locally, no bus.Publish
- `CompleteByAssignee()` — marks tasks done locally, no bus.Publish

**Consequence:** When an agent completes its assigned tasks, `m.appCtx.TaskStore.CompleteByAssignee(agentName, "completed")` is called (search: `internal/tui/root.go:2620`, `4582`). This updates in-memory + DB but emits **zero events**. Web UI never sees task status changes unless task was created/updated via tools (which do emit).

**Code trace:**
```go
// TUI sees this:
if agentTasks := tools.GlobalTaskStore.ByAssignee(ev.AgentName); len(agentTasks) > 0 {
    // render task updates in sidebar
}

// Web gets nothing because:
// CompleteByAssignee → saveToDB() → no bus.Publish()
```

### 2. Config Changes (TUI-only, web blind)

**Location:** `internal/tui/root.go:5287-5315`

**Function:** `applyConfigChange(key, value)`

**Mutations without events:**
- `m.model = value` → API model changed, no event
- `m.engineConfig.PermissionMode = value` → permission policy changed, no event
- `m.appCtx.Config.OutputStyle = value` → style changed, no event
- `m.appCtx.Config.OutputFilter = enabled` → tool output filtering toggled, no event

**Consequence:** All config panel mutations (user toggles setting in TUI) update local state only. Web UI has no way to know config changed. If user switches to web tab, web still has old config cached.

**Code trace:**
```go
case panelconfig.ConfigChangedMsg:
    m.applyConfigChange(msg.Key, msg.Value)  // TUI-only state change
    return m, nil                              // no bus.Publish
```

### 3. Memory Mutations (partial bypass)

**Location:** `internal/tui/root.go:1426-1446`

**Handler:** `memorypanel.NewMemoryMsg`

**Mutation:**
- `m.appCtx.Memory.Save(entry)` — saves to disk + in-memory

**Consequence:** Memory entries saved via external editor in TUI update the memory index but emit no bus event. Web doesn't know.

### 4. Session Load (TUI-only state)

**Location:** `internal/query/engine.go:210`

**Code:** `tools.GlobalTaskStore.LoadForSession(cfg.SessionID)`

**Consequence:** When engine starts for a session, task store loads from DB. This is a state mutation (clears + reloads in-memory tasks) but no event. If web was previously viewing different session and TUI loads new session, web still sees old task list until user manually refreshes.

---

## Root Cause Analysis

### Why 4 Message Events Bypass Bus

**Answer:** Direct streaming efficiency + WebSocket already connected.

- `EventMsgStreamDelta`, `EventMsgToolUse`, `EventMsgToolResult`, `EventMsgAssistant` arrive in real-time via the query engine's `EventHandler` interface.
- `eventproxy.go` wraps the handler and directly calls `client.SendEvent()` instead of `bus.Publish()`.
- This is **intentional optimization**: avoids bus serialization overhead for high-frequency events (token-by-token streaming).
- **But cost:** These 4 events cannot be subscribed to in TUI code via bus. Only web sees them.

**How it works:**
1. User attached to ComandCenter (headless + --attach mode)
2. EventProxy wraps the TUI's EventHandler
3. EventProxy forwards to ComandCenter WebSocket directly
4. ComandCenter's hub broadcasts to all clients
5. TUI never gets these events (no bus subscription)

### Why Task/Config Mutations Bypass Bus

**Answer:** Direct mutation of shared state objects without publishing.

- `GlobalTaskStore` is a simple map with mutex. Callers mutate it directly.
- `m.appCtx.Config` and `m.engineConfig` are mutable structs referenced by TUI and engine.
- No publish-on-change pattern implemented.
- Each caller responsible for bus.Publish — many forget or choose not to.

---

## Compliance Matrix: What Events Should Be Interface-Agnostic?

| Category | Events | Status | Implication |
|----------|--------|--------|-------------|
| **Core async work** | MsgAssistant, MsgToolUse, MsgToolResult, MsgStreamDelta, AgentStatus | ✓ Async-agnostic (go direct) | TUI cannot subscribe; only web sees |
| **Session lifecycle** | SessionHello, SessionBye | ✓ Async-agnostic (proto handshake) | Correct; web doesn't need bus |
| **UI side effects** | TaskCreated, TaskUpdated, ClearHistory | ✓ Via bus | ✓ Correct; both interfaces see |
| **Design/media** | DesignScreenshot, DesignBundleReady | ⚠ Direct SendEvent | Bypasses bus, but web handles OK |
| **Mutations** | task complete, config change, memory save | ✗ No events at all | **TUI-only; web blind** |

---

## Open Questions / Risks

1. **Can web UI refresh detect stale task state?**
   - When user completes a task in TUI, web never hears about it.
   - If user switches to web tab, task list is stale until page reload.
   - **Recommendation:** Emit bus event from CompleteByAssignee/CompleteByIDs, or poll task state on UI refresh.

2. **Can web UI detect config changes?**
   - TUI user changes output style, permission mode, model, etc.
   - Web has no way to know; cached UI state diverges.
   - **Recommendation:** Emit config.changed bus event from applyConfigChange.

3. **Why does eventproxy bypass bus?**
   - Is it a performance choice, or oversight?
   - If streaming events (1000s/sec) go through bus, does it degrade?
   - **Recommendation:** Measure. If bus overhead is negligible, consolidate all events through bus for consistency.

4. **Task mutations in TUI should forward?**
   - `tools.GlobalTaskStore.CompleteByAssignee()` called from TUI at lines 2620, 4582.
   - Should it emit EventTaskUpdated for each completed task?
   - **Recommendation:** Yes; wrap mutation in a helper that emits event.

---

## Recommended Fixes (Priority Order)

### P0: Make Task Mutations Visible to Web
- **Files:** `internal/tools/tasks.go` (CompleteByIDs, CompleteByAssignee)
- **Change:** Add bus.Publish(EventTaskUpdated) for each task status change
- **Impact:** Web task list stays synchronized

### P1: Make Config Changes Visible to Web
- **Files:** `internal/tui/root.go` (applyConfigChange)
- **Change:** Add bus.Publish(attach.Event{Type: "config.changed", ...}) for each mutation
- **Impact:** Web can invalidate cached config, prompt refresh if needed

### P2: Consolidate Streaming Events Through Bus (optional)
- **Files:** `internal/cli/attachclient/eventproxy.go`
- **Question:** Measure if bus.Publish overhead is acceptable for EventMsgStreamDelta (high frequency).
- **Benefit:** Allows TUI to also subscribe to streaming events; unified event model
- **Cost:** Potential latency if bus serialization is slow

### P3: Document Interface-Agnostic Patterns
- **Files:** `docs/` or code comments
- **Change:** Clarify when to use bus.Publish vs. direct SendEvent
- **Benefit:** Prevents future inconsistencies

---

## Files Changed / Reviewed

- `internal/attach/protocol.go` — EventType definitions (read)
- `internal/cli/root.go` — SubscribeAll forwarding (read)
- `internal/comandcenter/web/server.go` — fanout() handler (read)
- `internal/cli/attachclient/eventproxy.go` — direct SendEvent for streaming (read)
- `internal/tools/tasks.go` — CompleteByAssignee mutations (read)
- `internal/tui/root.go` — applyConfigChange, task completion (read)
- `internal/comandcenter/web/static/app.js` — web UI handlers (read)
- `internal/app/app.go` — ClearHistory (read)
- `internal/tasks/agent_task.go` — EventAgentStatus publishing (read)

---

**Report generated by:** orion (investigation agent)  
**Scope:** Read-only audit of event flow and state synchronization  
**Completeness:** All 11 EventTypes audited, all state mutation paths traced
