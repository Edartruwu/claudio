# Claudio Attach Protocol — M2 Integration Findings

**READ-ONLY INVESTIGATION**  
Scope: User message injection, event forwarding, callback hooks  
Files: `attachclient/client.go`, `root.go`, `app.go`, `query/engine.go`, `attach/protocol.go`

---

## 1. ATTACHCLIENT EVENT FORWARDING (client.go)

**Current state: MINIMAL — only inbound msg callback wired, no outbound.**

### Events handled:
- ✅ **EventSessionHello** (line 76–81): sent on connect
- ✅ **EventSessionBye** (line 134): sent on close
- ✅ **EventMsgUser** (line 162–175): inbound only; parsed + callback fired

### Events NOT forwarded to ComandCenter:
- ❌ **EventMsgAssistant** — no send logic
- ❌ **EventMsgToolUse** — no send logic  
- ❌ **EventTaskCreated/Updated** — no send logic
- ❌ **EventAgentStatus** — no send logic

### User message injection:
- **Callback registered** (line 116–120): `OnUserMessage(fn func(attach.UserMsgPayload))`
- **Current behavior** (root.go:137–140): callback fired but **TODO — not injected into session**
  ```go
  // For now, just log
  log.Printf("Received user message from ComandCenter: %q\n", payload.Content)
  ```

---

## 2. ROOT.GO WIRING (cli/root.go)

### Attach client init (lines 125–158):
```
flagAttach → client.New() → client.Connect() → OnUserMessage(callback) → SubscribeAll(callback)
```

### Bus subscription exists (line 143–152) BUT **incomplete**:
```go
appInstance.Bus.SubscribeAll(func(event bus.Event) {
  // TODO: map bus events to attach protocol events and forward
  switch event.Type {
  case "message.assistant": // TODO: Parse and forward
  case "task.created":       // TODO: Parse and forward
  }
})
```

### CRITICAL GAP: User message NOT injected into session
- Line 137–140: callback registered, msg logged only
- **No mechanism to push msg into `m.engine.Run()` or TUI input queue**
- ComandCenter can send, Claudio receives, but msg **dies in callback**

---

## 3. APP.GO HOOKS & BUS (internal/app/app.go)

### App struct (lines 39–60):
```go
type App struct {
  Bus *bus.Bus          // Event bus for pub/sub
  // ... other fields
}
```

### Bus created at init (line 86):
```go
eventBus := bus.New()
```

### NO callback hooks for:
- Assistant text (no `OnAssistantMsg` field)
- Tool use (no `OnToolUse` field)
- Task events (no `OnTask` field)
- Agent status (no `OnAgentStatus` field)

**Events only flow through Bus.Publish() → observers.**

---

## 4. QUERY ENGINE EVENTS (internal/query/engine.go)

### EventHandler interface (lines 27–49):
```go
type EventHandler interface {
  OnTextDelta(text string)
  OnThinkingDelta(text string)
  OnToolUseStart(toolUse tools.ToolUse)
  OnToolUseEnd(toolUse tools.ToolUse, result *tools.Result)
  OnTurnComplete(usage api.Usage)
  OnError(err error)
  OnToolApprovalNeeded(toolUse tools.ToolUse) bool
  OnRetry(toolUses []tools.ToolUse)
  OnCostConfirmNeeded(currentCost, threshold float64) bool
}
```

### Handler called from engine:
- **OnToolUseStart** (line 813): fired immediately on tool_use block
- **OnTextDelta** (lines 422, 442, 463, 833): streamed text deltas
- **OnThinkingDelta** (line 839): extended thinking text
- **OnToolUseEnd** (lines 935, 942, 949, 957, 966): after tool result
- **OnTurnComplete** (line 537): after full response

### Forwarding path for attachclient:
TUI instantiates engine with handler → handler fires callbacks → **need bridge to client.SendEvent()**

---

## 5. TUI MESSAGE INJECTION (internal/tui/root.go)

### User message entry point (line 2012–2199):
```go
func (m Model) handleSubmit(text string) (tea.Model, tea.Cmd)
```

**Steps:**
1. Parse text, check for commands (line 2032)
2. Queue if streaming (line 2042–2049)
3. Create ChatMessage{Type: MsgUser} (line 2086)
4. Refresh viewport (line 2087)
5. Wire up engine + fire `engine.Run(ctx, apiText)` (line 2186–2192)

### Message flow:
```
User input → handleSubmit() → addMessage() → engine.Run() → handler callbacks → rendered on screen
```

### For external injection (ComandCenter → Claudio):
- **Option A**: Inject into TUI's `prompt.Model` input field (no exported API)
- **Option B**: Call `handleSubmit(text)` directly (requires TUI instance ref, not exported)
- **Option C**: Inject into engine directly via `engine.Run(ctx, text)` (requires engine ref, available via `engineRef`)
- **Option D**: Create internal channel in Model for message queue (like `messageQueue` at line 129)

---

## 6. BUS EVENT TYPES (internal/bus/events.go)

**Defined but NOT all published:**

```
Session: session.start, session.end, session.compact
Message: message.user, message.assistant, message.system
Stream:  stream.start, stream.chunk, stream.done, stream.error
Tool:    tool.start, tool.end, tool.permission
Auth:    auth.login, auth.logout, auth.refresh
MCP:     mcp.connect, mcp.disconnect, mcp.tool_call
Learning: instinct.learned, instinct.evolved
Rate:    ratelimit.changed
Audit:   audit.entry (✅ actually published in security/audit.go:47, 66)
MCP:     (✅ actually published in services/mcp/manager.go:89, 112)
```

**Only confirmed published:**
- `bus.EventAuditEntry` (security/audit.go)
- `bus.EventMCPConnect` / `EventMCPDisconnect` (mcp/manager.go)

**MISSING publishers for M2:**
- `message.assistant` — needs engine integration
- `message.tool_use` — needs engine integration
- `task.created` / `task.updated` — needs task runtime integration
- `agent.status` — needs team runner integration

---

## 7. ATTACH PROTOCOL EVENTS (internal/attach/protocol.go)

### Claudio → ComandCenter (lines 8–17):
```
EventSessionHello    = "session.hello"
EventMsgAssistant    = "message.assistant"    ← NOT forwarded yet
EventMsgToolUse      = "message.tool_use"     ← NOT forwarded yet
EventTaskCreated     = "task.created"         ← NOT forwarded yet
EventTaskUpdated     = "task.updated"         ← NOT forwarded yet
EventAgentStatus     = "agent.status"         ← NOT forwarded yet
EventSessionBye      = "session.bye"
```

### ComandCenter → Claudio (lines 19–22):
```
EventMsgUser = "message.user"                 ← wired + logged, NOT injected
```

### Payloads defined (lines 30–85):
- `HelloPayload`: name, path, model, master
- `AssistantMsgPayload`: content, agent_name
- `ToolUsePayload`: tool, input, agent_name
- `TaskCreatedPayload`: id, title, assigned_to, status
- `TaskUpdatedPayload`: id, status, output
- `AgentStatusPayload`: name, status, current_task
- `UserMsgPayload`: content, attachments[], from_session

---

## M2 REQUIREMENTS — INTEGRATION GAPS

### 1. **Inbound user messages** (BLOCKED)
- ✅ Received via `client.OnUserMessage()`
- ❌ Not injected into engine/TUI
- **Fix**: Add channel/callback to push msg into `engine.Run()` or TUI input queue

### 2. **Outbound assistant text**
- ✅ Available via `handler.OnTextDelta()`
- ❌ No EventHandler bridge to `client.SendEvent(EventMsgAssistant, ...)`
- **Fix**: Wrap handler in proxy that calls both TUI + client.SendEvent()

### 3. **Outbound tool use**
- ✅ Available via `handler.OnToolUseStart()` + `OnToolUseEnd()`
- ❌ No forwarding to `client.SendEvent(EventMsgToolUse, ...)`
- **Fix**: Same proxy handler wrapping

### 4. **Task events**
- ✅ Task payloads exist in attach/protocol.go
- ❌ No bus publishers or app hooks for task.created/updated
- **Fix**: Hook task.Runtime to publish to bus → attachclient listens + forwards

### 5. **Agent status events**
- ✅ AgentStatusPayload exists
- ❌ No hook into team runner for status changes
- **Fix**: Hook team runner to publish to bus → attachclient listens + forwards

### 6. **Bus event forwarding**
- ✅ Bus.SubscribeAll() wired in root.go (line 143)
- ❌ Switch statement is TODO (lines 146–151)
- **Fix**: Map bus event types to attach payloads, call client.SendEvent()

---

## ARCHITECTURE SUMMARY

```
ComandCenter (external)
    ↓ WebSocket
attachclient.Client
    ↓ (inbound)
onUserMessage callback
    ❌ NOT WIRED → Engine/TUI input
    
Query Engine
    ↓ EventHandler callbacks
    ├─ OnTextDelta / OnThinkingDelta / OnToolUse*
    ❌ NOT WIRED → client.SendEvent()
    
App.Bus
    ├─ Publish: message.assistant, task.created, agent.status
    ❌ MISSING: publishers for most events
    ├─ Subscribe: root.go (line 143)
    ❌ TODO: forward to client.SendEvent()
```

---

## IMMEDIATE ACTIONS FOR M2

1. **User message injection**: Add `injectMessage(text string)` method to TUI Model that calls `handleSubmit()`
   - Called from: `attachclient.OnUserMessage()` callback
   - Requires: export Model method or add internal channel

2. **Event handler proxy**: Wrap TUI's event handler to intercept engine callbacks
   - Capture: OnTextDelta, OnToolUseStart, OnToolUseEnd, OnThinkingDelta
   - Forward: `client.SendEvent(EventMsgAssistant, ...)`, `client.SendEvent(EventMsgToolUse, ...)`

3. **Bus subscription completion**: Replace TODO in root.go:146–151
   - Map `bus.Event.Type` → attach protocol event type
   - Marshal payload → `client.SendEvent()`

4. **Add missing bus publishers**:
   - Engine: publish `message.assistant` on OnTextDelta
   - Tasks: publish `task.created`/`task.updated` from task.Runtime
   - Teams: publish `agent.status` from team runner

5. **Session relay**: Populate `SessionID` in all outbound events for ComandCenter routing

---

## FILES TO MODIFY

| File | Line | Action |
|------|------|--------|
| `internal/cli/root.go` | 136–157 | Complete user msg injection + bus forwarding TODO |
| `internal/cli/attachclient/client.go` | 116–120 | Export callback, add message queue |
| `internal/tui/root.go` | 2012+ | Add public method to inject external messages |
| `internal/query/engine.go` | 27–49 | Extend EventHandler (or proxy wrapper) |
| `internal/app/app.go` | — | Add hooks for outbound event propagation |
| `internal/bus/events.go` | — | Already complete; add publishers elsewhere |

