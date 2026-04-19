# WebSocket Real-Time Message Flow Analysis

## Executive Summary

**Message flow:** user sends via HTMX → handleSendMessage stores + pushes to hub → fanout broadcasts to WS clients → JS appends messages.

**SessionID matching:** Used in 3 layers:
1. handleSendMessage: path param `{session_id}`
2. hub.Send: looks up sessionID in `hub.sessions` map
3. pushToSessionClients: filters uiClients by sessionID

**Bugs found:** Race condition in reloadMessages + no error handling for hub.Send failures.

---

## 1. Entry Point: ccSend() (JavaScript)

**File:** `internal/comandcenter/web/chat_view.templ` lines 385-422
**Trigger:** User clicks Send button (onclick="ccSend()") or presses Enter

```go
window.ccSend = function() {
  var text = msgInput.value.trim();
  if (stagedFile) {
    // XHR upload to /api/sessions/{sessionId}/upload
    xhr.open('POST', '/api/sessions/' + sessionId + '/upload');
  } else {
    // HTMX POST to /api/sessions/{sessionId}/message
    htmx.ajax('POST', '/api/sessions/' + sessionId + '/message', {
      values: { content: text },
      swap: 'none'  // no DOM response expected
    });
  }
}
```

**Route:** `POST /api/sessions/{session_id}/message` (line 139 in server.go RegisterRoutes)

---

## 2. HTTP Handler: handleSendMessage()

**File:** `internal/comandcenter/web/server.go` lines 615-733
**Signature:** `func (ws *WebServer) handleSendMessage(w http.ResponseWriter, r *http.Request)`

### Flow:
```go
// Line 616: Extract sessionID from URL path
sessionID := r.PathValue("session_id")

// Lines 621-625: Parse form, get content
content := r.FormValue("content")
if content == "" {
  w.WriteHeader(http.StatusNoContent)
  return
}

// Lines 705-712: Store message in DB
msg := cc.Message{
  ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
  SessionID: sessionID,
  Role:      "user",
  Content:   content,
  CreatedAt: time.Now(),
}
_ = ws.storage.InsertMessage(msg)

// Lines 715-723: Render & push user bubble to WS clients
var buf bytes.Buffer
MessageBubble(MessageView{Message: msg}).Render(r.Context(), &buf)
payload, _ := json.Marshal(map[string]string{
  "type": "message.user",
  "html": buf.String(),
})
ws.pushToSessionClients(sessionID, payload)

// Lines 725-732: Forward to agent via hub
payload, _ := json.Marshal(attach.UserMsgPayload{Content: content})
env := attach.Envelope{Type: attach.EventMsgUser, Payload: payload}
if err := ws.hub.Send(sessionID, env); err != nil {
  http.Error(w, "session not connected", http.StatusServiceUnavailable)
  return
}
```

### SessionID Usage:
- **Extracted:** `r.PathValue("session_id")` (line 616)
- **Stored in msg:** `msg.SessionID = sessionID` (line 708)
- **Pushed to clients:** `ws.pushToSessionClients(sessionID, payload)` (line 721)
- **Forwarded to hub:** `ws.hub.Send(sessionID, env)` (line 727)

---

## 3. Hub: SessionID Matching for Send

**File:** `internal/comandcenter/hub.go` lines 88-96

```go
func (h *Hub) Send(sessionID string, env attach.Envelope) error {
  h.mu.RLock()
  conn, ok := h.sessions[sessionID]  // KEY: map lookup by sessionID
  h.mu.RUnlock()
  if !ok {
    return fmt.Errorf("hub: session %s not connected", sessionID)
  }
  return conn.writeEnvelope(env)
}
```

**How sessionID is registered:** `Hub.HandleSession()` (line 325)
```go
h.Register(sessionID, conn)  // sessionID keyed in map
```

**SessionID comes from:** Agent hello message parsed in HandleSession (lines 280-305):
```go
type HelloPayload struct {
  Name         string
  Path         string
  Model        string
  Master       bool
  AgentType    string
  TeamTemplate string
}

// If session doesn't exist, generate new UUID; if exists, use existing ID
sessionID = existing.ID  // or newID()
```

---

## 4. Inbound Flow: Agent Messages → Hub → Broadcast

**File:** `internal/comandcenter/hub.go` lines 300-358

### Main Read Loop:
```go
for {
  var ev attach.Envelope
  if err := conn.readEnvelope(&ev); err != nil {
    break
  }
  
  h.processEvent(sessionID, ev)      // Line 355: Store in DB
  h.Broadcast(sessionID, ev)         // Line 356: Push to UI clients
}
```

**processEvent:** Stores messages in DB based on event type (lines 360-441)
- `EventMsgAssistant` → InsertMessage with role="assistant"
- `EventMsgToolUse` → InsertMessage with role="tool_use" + ToolUseID
- `EventMsgToolResult` → `UpdateMessageOutput(sessionID, toolUseID, output)` (line 402)

**Broadcast:** Sends UIEvent to fanout channel (lines 445-449)
```go
func (h *Hub) Broadcast(sessionID string, env attach.Envelope) {
  select {
  case h.uiBroadcast <- UIEvent{SessionID: sessionID, Envelope: env}:
  default:  // Non-blocking; drops if channel full
  }
}
```

---

## 5. Fanout: HTML Rendering & Push to WS Clients

**File:** `internal/comandcenter/web/server.go` lines 1082-1230

### Init:
```go
// Line 111: Created in NewWebServer
go ws.fanout()

// Line 1083: Read from hub's UIBroadcast channel
ch := ws.hub.UIBroadcast()
for ev := range ch {
  switch ev.Envelope.Type {
```

### Event: EventMsgAssistant (lines 1086-1108)
```go
case attach.EventMsgAssistant:
  msg := envelopeToMessage(ev)
  var buf bytes.Buffer
  MessageBubble(MessageView{Message: *msg}).Render(context.Background(), &buf)
  payload, _ := json.Marshal(map[string]string{
    "type": "message.assistant",
    "html": buf.String(),
  })
  ws.pushToSessionClients(ev.SessionID, payload)
  
  // Also trigger reload so TUI-sent messages appear first
  ws.pushToSessionClients(ev.SessionID, map[string]string{"type": "messages.reload"})
```

### Event: EventMsgToolResult (lines 1144-1158)
```go
case attach.EventMsgToolResult:
  var p attach.ToolResultPayload
  _ = ev.Envelope.UnmarshalPayload(&p)
  resultPayload, _ := json.Marshal(map[string]string{
    "type":       "message.tool_result",
    "toolUseID":  p.ToolUseID,      // KEY: ToolUseID passed to JS
    "output":     p.Output,
  })
  ws.pushToSessionClients(ev.SessionID, resultPayload)
```

---

## 6. Push to UI Clients

**File:** `internal/comandcenter/web/server.go` lines 1068-1079

```go
func (ws *WebServer) pushToSessionClients(sessionID string, payload []byte) {
  ws.mu.RLock()
  defer ws.mu.RUnlock()
  for client := range ws.clients {
    if client.sessionID == sessionID {  // KEY: SessionID filter
      select {
      case client.send <- payload:
      default:  // Drop if full (channel size=64)
      }
    }
  }
}
```

**Client registration:** handleWSUI (lines 1022-1065)
```go
func (ws *WebServer) handleWSUI(w http.ResponseWriter, r *http.Request) {
  sessionID := r.URL.Query().Get("session_id")  // From ?session_id=
  client := &uiClient{
    sessionID: sessionID,
    send:      make(chan []byte, 64),
  }
  ws.addClient(client)  // Registered globally
  
  for {
    select {
    case msg, ok := <-client.send:
      websocket.Message.Send(conn, string(msg))
    case <-done:
      return
    }
  }
}
```

---

## 7. JavaScript Handler: ws.onmessage

**File:** `internal/comandcenter/web/static/app.js` lines 156-280

### Message Types Handled:
```javascript
ws.onmessage = function(e) {
  var data = JSON.parse(e.data);
  
  if (data.type === 'message.assistant') {
    removeTypingBubble();
    appendMessage(data.html);  // Add bubble to DOM
    
  } else if (data.type === 'message.tool_use') {
    appendMessage(data.html);
    
  } else if (data.type === 'message.tool_result') {
    // Find bubble by toolUseID, inject output
    var bubble = document.querySelector('[data-tool-use-id="' + data.toolUseID + '"]');
    if (bubble) {
      var outputSection = bubble.querySelector('.tool-output-section');
      // ... update HTML
    }
    
  } else if (data.type === 'messages.reload') {
    reloadMessages();  // Full reload
    
  } else if (data.type === 'messages.cleared') {
    msgs.innerHTML = '';
  }
}
```

### appendMessage Function (lines 98-110)
```javascript
function appendMessage(html) {
  if (!msgs) return;
  maybeInsertDateDivider();
  var near = isNearBottom();
  var bubble = document.getElementById('typing-bubble');
  if (bubble && !bubble.classList.contains('hidden')) {
    bubble.insertAdjacentHTML('beforebegin', html);
  } else {
    msgs.insertAdjacentHTML('beforeend', html);  // Insert at end
  }
  if (near) msgs.scrollTop = msgs.scrollHeight;
}
```

---

## 8. Data Flow Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│ User sends message                                              │
├─────────────────────────────────────────────────────────────────┤
│                          ↓                                       │
│ Browser: ccSend() (chat_view.templ:385)                          │
│ POST /api/sessions/{sessionId}/message via HTMX                  │
│                          ↓                                       │
│ ┌──────────────────────────────────────────────────────────────┐ │
│ │ handleSendMessage (server.go:615)                            │ │
│ │ 1. sessionID = r.PathValue("session_id")                     │ │
│ │ 2. msg.SessionID = sessionID                                 │ │
│ │ 3. InsertMessage(msg)                                        │ │
│ │ 4. pushToSessionClients(sessionID, userBubble)               │ │
│ │ 5. hub.Send(sessionID, EventMsgUser)                         │ │
│ └──────────────────────────────────────────────────────────────┘ │
│                  ↙                          ↘                    │
│ Immediate: Push user bubble          Forward to agent:           │
│ to WS clients                         hub.sessions[sessionID]     │
│            ↓                                    ↓                 │
│ WS clients see               Agent gets EventMsgUser,            │
│ message immediately           processes, returns events          │
│                                   ↓                              │
│         ┌──────────────────────────────────────┐                │
│         │ Agent sends events via WebSocket     │                │
│         │ (registered in hub.HandleSession)    │                │
│         └──────────────────────────────────────┘                │
│                          ↓                                       │
│ ┌──────────────────────────────────────────────────────────────┐ │
│ │ Hub.HandleSession read loop (hub.go:346)                     │ │
│ │ 1. conn.readEnvelope(&ev)                                    │ │
│ │ 2. processEvent(sessionID, ev)  ← Store in DB                │ │
│ │ 3. Broadcast(sessionID, ev)     ← Send to UI                 │ │
│ └──────────────────────────────────────────────────────────────┘ │
│                          ↓                                       │
│ ┌──────────────────────────────────────────────────────────────┐ │
│ │ WebServer.fanout() (server.go:1082)                          │ │
│ │ Reads from hub.UIBroadcast channel                           │ │
│ │ Renders HTML bubble via templ                                │ │
│ │ Calls pushToSessionClients(sessionID, payload)               │ │
│ └──────────────────────────────────────────────────────────────┘ │
│                          ↓                                       │
│ ┌──────────────────────────────────────────────────────────────┐ │
│ │ pushToSessionClients (server.go:1068)                        │ │
│ │ for client in ws.clients {                                  │ │
│ │   if client.sessionID == sessionID {                         │ │
│ │     client.send <- payload                                   │ │
│ │   }                                                          │ │
│ │ }                                                            │ │
│ └──────────────────────────────────────────────────────────────┘ │
│                          ↓                                       │
│ ┌──────────────────────────────────────────────────────────────┐ │
│ │ handleWSUI write loop (server.go:1050)                       │ │
│ │ for {                                                        │ │
│ │   select {                                                   │ │
│ │   case msg <- client.send:                                   │ │
│ │     websocket.Message.Send(conn, string(msg))                │ │
│ │   }                                                          │ │
│ │ }                                                            │ │
│ └──────────────────────────────────────────────────────────────┘ │
│                          ↓                                       │
│ Browser: ws.onmessage (app.js:156)                              │
│ Parse JSON, dispatch by type                                    │
│                          ↓                                       │
│ ┌──────────────────────────────────────────────────────────────┐ │
│ │ appendMessage() or DOM update (app.js:98)                    │ │
│ │ insertAdjacentHTML('beforeend', html)                        │ │
│ │ Scroll if near bottom                                        │ │
│ └──────────────────────────────────────────────────────────────┘ │
│                          ↓                                       │
│ Message visible in chat UI                                      │
└─────────────────────────────────────────────────────────────────┘
```

---

## SessionID Matching Logic

### Path 1: Outbound (User → Agent)
1. Browser POST → URL path: `/api/sessions/{sessionID}/message`
2. Handler extracts: `r.PathValue("session_id")` → sessionID
3. Message stored: `msg.SessionID = sessionID`
4. Hub lookup: `hub.sessions[sessionID]` (registered by agent on connect)
5. If found: `conn.writeEnvelope(env)` sends to agent
6. If NOT found: `http.Error(...ServiceUnavailable)` — **no automatic retry**

### Path 2: Inbound (Agent → Browser)
1. Agent sends Envelope via registered wsConn
2. Hub.HandleSession receives in read loop, extracts sessionID from context
3. **Critical:** `Broadcast(sessionID, ev)` passes sessionID explicitly
4. fanout() receives UIEvent with SessionID field
5. **All events broadcast to ALL listeners** — fanout doesn't filter by sessionID
6. pushToSessionClients filters: `if client.sessionID == sessionID`

### Path 3: Tool Result Matching
1. Hub receives EventMsgToolResult with ToolUseID
2. Calls: `UpdateMessageOutput(sessionID, toolUseID, output)`
3. SQL: `UPDATE cc_messages SET output=? WHERE session_id=? AND tool_use_id=?`
4. **Both sessionID AND toolUseID used** — no cross-session pollution
5. JS receives `data.toolUseID`, finds bubble via: `[data-tool-use-id="..."]`

---

## Bugs & Races Found

### 1. **Race in reloadMessages() (HIGH PRIORITY)**

**File:** `app.js` lines 131-143

```javascript
function reloadMessages() {
  fetch('/partials/messages/' + sessionId)
    .then(res => res.text())
    .then(html => {
      msgs.innerHTML = html;  // ← CLEARS entire DOM
      // ...
    })
}
```

**Problem:** 
- If WS message arrives with `appendMessage()` between fetch start and innerHTML set, the message is lost
- reloadMessages called on ws.onopen (line 153) and for EventMsgAssistant (line 1106 in fanout)
- **No synchronization**

**Race scenario:**
1. WS reconnects → reloadMessages() starts fetch
2. Agent sends assistant message → fanout pushes 2 payloads:
   - message.assistant (with HTML)
   - messages.reload
3. Fetch still in flight (network delay)
4. ws.onmessage("message.assistant") → appendMessage() tries to add bubble
5. But next ws.onmessage("messages.reload") calls reloadMessages() → clears DOM
6. Fetch finally completes, sets msgs.innerHTML = old data
7. **New message disappears**

**Fix:** Use MutationObserver or async lock to prevent append during reload.

---

### 2. **No Error Recovery if hub.Send Fails (MEDIUM PRIORITY)**

**File:** `server.go` lines 727-730

```go
if err := ws.hub.Send(sessionID, env); err != nil {
  http.Error(w, "session not connected", http.StatusServiceUnavailable)
  return
}
```

**Problem:**
- User message is already stored in DB (line 712) + pushed to UI (line 721)
- If hub.Send fails (agent not connected), message is orphaned — agent never receives it
- UI shows message locally, but agent doesn't see it
- No retry, no queue

**Impact:** User thinks message was sent, but agent is not running.

**Note:** This is architectural (not a bug per se). The system is designed for persistent hub connection. But adds confusion if agent crashes.

---

### 3. **Dropped Messages if pushToSessionClients Channel Full (LOW PRIORITY)**

**File:** `server.go` lines 1073-1076

```go
select {
case client.send <- payload:
default:  // Drop if full
}
```

**Problem:**
- Client channel size = 64 bytes (allocated in handleWSUI line 1031)
- If client is slow to read, new messages are silently dropped
- No error logged

**Mitigated by:** Scroll handler in JS throttles; user won't be that slow. But under stress test (many tabs), could lose messages.

---

### 4. **No Explicit SessionID Validation in handleWSUI (LOW PRIORITY)**

**File:** `server.go` line 1023

```go
sessionID := r.URL.Query().Get("session_id")
```

**Problem:**
- No validation that sessionID exists or matches authenticated user
- Could theoretically register for a session you don't own
- **But:** relies on uiAuth middleware (line 144) for auth

**Fix:** Add `ws.storage.GetSession(sessionID)` check before registering client.

---

## Summary: SessionID Flow Correctness

✅ **sessionID correctly extracted** at entry (r.PathValue)
✅ **sessionID stored in message** before pushing
✅ **sessionID used as map key** in hub (no collisions)
✅ **pushToSessionClients filters correctly** by sessionID
✅ **Tool result matching uses sessionID + toolUseID** (no cross-session pollution)

⚠️ **Race in reloadMessages** clears DOM without guarding appends
⚠️ **No error recovery** if hub.Send fails
⚠️ **No sessionID validation** in WebSocket client

---

## Key Functions Reference

| Function | File | Lines | Purpose |
|----------|------|-------|---------|
| ccSend | chat_view.templ | 385-422 | JS entry point, sends via HTMX |
| handleSendMessage | server.go | 615-733 | HTTP handler, stores + pushes + forwards to hub |
| hub.Send | hub.go | 88-96 | Lookup sessionID in sessions map, write to agent |
| hub.HandleSession | hub.go | 280-358 | Receive from agent, process + broadcast |
| hub.Broadcast | hub.go | 445-449 | Push UIEvent to fanout channel |
| ws.fanout | server.go | 1082-1230 | Consume UIBroadcast, render HTML, push to clients |
| pushToSessionClients | server.go | 1068-1079 | Filter clients by sessionID, push to send channels |
| handleWSUI | server.go | 1022-1065 | WS upgrade, register client, write loop |
| ws.onmessage | app.js | 156-280 | JS handler, parse JSON, append to DOM |
| appendMessage | app.js | 98-110 | Insert HTML at end of messages div |
| reloadMessages | app.js | 131-143 | Reload all messages via HTTP (RACE!) |

