# Investigation Report: Comandcenter Web UI — Popover Menu & Message Flow

## Subject
How the `+` popover menu works, where "Clear Conversation", "Compact", and "Interrupt" are wired up in JS/templates, what endpoints handle them, and whether model selection exists in the web UI.

## Codebase Overview

**File Structure:**
- `/internal/comandcenter/web/server.go` (82 symbols) — HTTP handlers, endpoint routes, slash command intercepts
- `/internal/comandcenter/web/static/app.js` (155 lines) — WebSocket client, file upload, @mention autocomplete, push notifications
- `/internal/comandcenter/web/templates/chat_view.html` (731 lines) — chat UI, popover menu, input bar, file browser modal
- `/internal/storage/sessions.go` — `cc.Session` struct definition
- `/internal/services/compact/compact.go` — message compaction service (imported but not in web dir)

**Entry points:**
- Web server listens on `http.ServeMux` at `:PORT`
- WebSocket at `/ws/ui?session_id=<id>`
- REST API endpoints at `/api/sessions/{id}/*`
- Message send via form POST or XHR to `/api/sessions/{session_id}/message`

---

## Key Findings

### 1. Plus Popover Menu HTML & Event Handlers

**Location:** `/internal/comandcenter/web/templates/chat_view.html:95–129`

The `+` popover is a `<div id="plus-popover">` with four buttons:

| Button | Line | Handler | Action |
|--------|------|---------|--------|
| **Interrupt** | 99–104 | `onclick="ccInterrupt()"` | Calls async fetch to POST `/api/sessions/{id}/interrupt` |
| **Clear Conversation** | 105–110 | `onclick="ccSendCommand('/clear')"` | Calls async fetch to POST `/api/sessions/{id}/message` with body `{content: "/clear"}` |
| **Compact** | 111–116 | `onclick="ccSendCommand('/compact')"` | Calls async fetch to POST `/api/sessions/{id}/message` with body `{content: "/compact"}` |
| **Upload File** | 117–122 | `onclick="document.getElementById('file-input').click();..."` | Triggers hidden file input, then closes popover |
| **Browse Session Files** | 123–128 | `onclick="ccOpenFileBrowser();..."` | Opens file browser modal, closes popover |

**Plus button toggle logic:**  
Line 166–171: `<button id="plus-btn">` uses inline `onclick` to toggle `.hidden` class on `#plus-popover`:
```javascript
onclick="(function(){var p=document.getElementById('plus-popover');p.classList.toggle('hidden');})()"
```

**Popover closes on outside click:**  
Lines 234–241 in `chat_view.html` add a global click listener that hides popover if click is outside.

---

### 2. JavaScript Functions for Popover Menu Items

All defined in `/internal/comandcenter/web/templates/chat_view.html` inside `<script>` tags:

#### **ccInterrupt()** — Line 591–612
```javascript
window.ccInterrupt = async function() {
    document.getElementById('plus-popover').classList.add('hidden');
    var sessionId = '{{.Session.ID}}';
    var token = localStorage.getItem('cc_token');
    try {
      var res = await fetch('/api/sessions/' + sessionId + '/interrupt', {
        method: 'POST',
        headers: { 'Authorization': 'Bearer ' + (token || '') }
      });
      if (res.ok) {
        ccToast('Turn interrupted', true);
      } else if (res.status === 503) {
        ccToast('No active turn', false);
      } else if (res.status === 404) {
        ccToast('Session not found', false);
      } else {
        ccToast('Request failed', false);
      }
    } catch (e) {
      ccToast('Request failed', false);
    }
};
```
**Call chain:** Popover button → `ccInterrupt()` → `POST /api/sessions/{id}/interrupt` (async fetch)

#### **ccSendCommand(cmd)** — Line 615–629
```javascript
window.ccSendCommand = async function(cmd) {
    document.getElementById('plus-popover').classList.add('hidden');
    var sessionId = '{{.Session.ID}}';
    var token = localStorage.getItem('cc_token') || '';
    try {
      var res = await fetch('/api/sessions/' + sessionId + '/message', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + token },
        body: JSON.stringify({ content: cmd })
      });
      ccToast(res.ok ? cmd + ' sent' : 'Failed', res.ok);
    } catch (e) {
      ccToast('Request failed', false);
    }
};
```
**Used for:** `/clear` and `/compact` commands  
**Call chain:** Popover button → `ccSendCommand('/clear' | '/compact')` → `POST /api/sessions/{id}/message` with JSON body (async fetch)

---

### 3. Backend Endpoint: Slash Commands Interception

**Location:** `/internal/comandcenter/web/server.go:675–751` (handleSendMessage)

Route: `POST /api/sessions/{session_id}/message` (line 296)

**Slash command intercept logic:**

1. **`/clear` command** (lines 687–705):
   ```go
   if strings.TrimSpace(content) == "/clear" {
       if err := ws.storage.DeleteMessages(sessionID); err != nil {
           http.Error(w, "storage error", http.StatusInternalServerError)
           return
       }
       confirm := cc.Message{
           ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
           SessionID: sessionID,
           Role:      "assistant",
           AgentName: "system",
           Content:   "Conversation cleared. ✓",
           CreatedAt: time.Now(),
       }
       _ = ws.storage.InsertMessage(confirm)
       ws.pushMsgBubble(sessionID, confirm)
       w.WriteHeader(http.StatusNoContent)
       return
   }
   ```
   - Deletes all messages for session → inserts confirmation message → pushes to WS clients
   - Returns 204 No Content
   - **Backend-only**, no API call to external service

2. **`/compact` command** (lines 707–712):
   ```go
   if strings.HasPrefix(strings.TrimSpace(content), "/compact") {
       instruction := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(content), "/compact"))
       ws.handleCompact(w, sessionID, instruction)
       return
   }
   ```
   - Extracts instruction text after `/compact` (e.g., `/compact summarize in bullets` → `instruction = "summarize in bullets"`)
   - Delegates to `handleCompact()`

**handleCompact() full logic** (lines 867–942):
1. Validates `apiClient` is configured (required for calling compact service)
2. Fetches all messages from DB (up to 1000, reversed to oldest-first)
3. Calls `compact.Compact(ctx, ws.apiClient, apiMsgs, 10, instruction)` — **API call**
4. Deletes old messages, inserts compacted ones
5. Inserts confirmation message with summary (max 200 chars) and pushes to WS

**Data touched:**
- `ws.storage`: DB read/write (messages)
- `ws.apiClient`: external API call for compaction (needs model configured)
- `ws.hub`: WS push to UI clients

---

### 4. Backend Endpoint: Interrupt

**Location:** `/internal/comandcenter/web/server.go:1165–1176` (handleInterruptSession)

Route: `POST /api/sessions/{id}/interrupt` (line 309)

```go
func (ws *WebServer) handleInterruptSession(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id")
    if _, err := ws.storage.GetSession(id); err != nil {
        http.Error(w, "session not found", http.StatusNotFound)
        return
    }
    if !ws.hub.Interrupt(id) {
        http.Error(w, "no active turn", http.StatusServiceUnavailable)
        return
    }
    w.WriteHeader(http.StatusOK)
}
```

- Validates session exists
- Calls `ws.hub.Interrupt(id)` — sends interrupt signal to active engine turn
- Returns 200 on success, 404 if session unknown, 503 if no active turn
- **No database write**, just signal propagation

---

### 5. Message Send Flow

**Flow chart:**

```
User types message in textarea #msg-input
    ↓
Click send button #send-btn → ccSend()
    ↓
[2 paths]
├─ If file staged:
│  ├ XHR POST /api/sessions/{id}/upload (FormData with file + optional caption)
│  └ Progress bar updates (lines 282–328)
│
└─ If text-only:
   └ htmx.ajax('POST', '/api/sessions/{id}/message', {values: {content: text}, swap: 'none'})
       ↓
    [Backend handleSendMessage]
    ├─ Detects /clear → intercept + DB delete
    ├─ Detects /compact → intercept + handleCompact
    ├─ Detects @mention → route to target session
    └─ Normal path → ws.hub.Send(sessionID, envelope) → queues to agent
```

**Line 322–325 (htmx send):**
```javascript
htmx.ajax('POST', '/api/sessions/' + sessionId + '/message', {
    values: { content: text },
    swap: 'none'
});
```
- No DOM swap (fire-and-forget)
- Response handling done via WebSocket

**WebSocket loop** (`handleWSUI`, line 1010+):
- Messages stream back as `type: 'message.user'`, `'message.assistant'`, `'typing'`, `'message.tool_use'`
- JavaScript appends to `#messages` div, scrolls to bottom
- Typing indicator shown/hidden based on event type

---

### 6. Model Selection — NOT FOUND

**cc.Session struct has Model field** (line 15 in `/internal/storage/sessions.go`):
```go
type Session struct {
    ID              string
    Title           string
    ProjectDir      string
    Model           string        // ← exists
    CreatedAt       time.Time
    UpdatedAt       time.Time
    Summary         string
    ParentSessionID string
    AgentType       string
    TeamTemplate    string
}
```

**BUT:**
- Zero endpoint in `/internal/comandcenter/web/server.go` reads or updates session model
- Zero UI control in templates to select/change model
- Model is stored but **not exposed in web UI**
- Likely set during session creation (CLI or API client), not editable in web

**Conclusion:** Model selection is **not implemented in the web UI**. The Session.Model field exists but is never touched by web handlers.

---

## Symbol Map

| Symbol | File | Role |
|--------|------|------|
| `handleSendMessage` | `server.go:675` | POST handler for `/api/sessions/{id}/message`; intercepts `/clear` and `/compact` |
| `handleCompact` | `server.go:867` | Executes compact service; calls API client to summarize messages |
| `handleInterruptSession` | `server.go:1165` | POST handler for `/api/sessions/{id}/interrupt`; signals active turn to stop |
| `ccInterrupt()` | `chat_view.html:591` | JS function; calls interrupt endpoint async |
| `ccSendCommand(cmd)` | `chat_view.html:615` | JS function; sends slash command to message endpoint async |
| `ccSend()` | `chat_view.html:282` | JS function; decides between XHR (file) or htmx (text) send |
| `plus-popover` | `chat_view.html:96` | Hidden div; toggled by plus-btn onclick; contains 5 menu buttons |
| `handleWSUI` | `server.go:1010` | WebSocket handler; streams messages and typing events to client |
| `RegisterRoutes` | `server.go:274` | Sets up all HTTP routes |

---

## Dependencies & Data Flow

**User initiates action from `+` popover:**

1. **Interrupt flow:**
   - `ccInterrupt()` (JS) → fetch POST `/api/sessions/{id}/interrupt`
   - Backend validates session exists, calls `hub.Interrupt(id)`
   - Returns 200 or error; toast shown to user
   - **No database mutation**, signal only

2. **Clear flow:**
   - `ccSendCommand('/clear')` (JS) → fetch POST `/api/sessions/{id}/message` with `{content: '/clear'}`
   - Backend detects string match → `storage.DeleteMessages(sessionID)` → insert confirmation msg → push to WS
   - All messages deleted, confirmation bubble shown in chat
   - Returns 204 No Content

3. **Compact flow:**
   - `ccSendCommand('/compact [instruction]')` (JS) → fetch POST `/api/sessions/{id}/message` with JSON body
   - Backend detects prefix match → extracts instruction → calls `handleCompact()`
   - `handleCompact()`:
     - Fetches all messages from DB
     - Calls `compact.Compact(apiClient, ...)` — external API call
     - Deletes old DB messages, inserts compacted ones
     - Inserts confirmation with summary (capped 200 chars)
     - Pushes confirmation to WS clients
   - Returns 204 No Content

4. **Normal message flow:**
   - `ccSend()` (JS) → htmx.ajax POST `/api/sessions/{id}/message` with text
   - Backend queues to `hub.Send()` → agent processes
   - WebSocket receives `message.user`, `message.assistant`, `typing`, `message.tool_use` events
   - JS appends to DOM, scrolls

---

## Risks & Observations

1. **No error handling in popover buttons** — User may not know if interrupt/clear failed because toast auto-hides in 2.5s (line 588)
   
2. **Model field exists but never set in web UI** — Sessions have a Model field but no UI control to change it. Model likely set only at creation time (CLI/API client), immutable in web.

3. **Compact requires external API client** — If `ws.apiClient == nil`, compact fails with 503. No warning UI if not configured. (Line 868–870)

4. **Clear is destructive, no confirmation** — Users can wipe all messages with one click. Popover has no confirm dialog (unlike file delete which uses htmx:confirm). (Line 688)

5. **FileInput event listener depends on ID selectors** — If HTML structure changes, file upload breaks silently (lines 244, 268, 441)

6. **Token stored in localStorage** — Bearer token for API calls lives in `localStorage`, exposed to XSS. (Line 594, 618)

7. **Inline onclick handlers** — All buttons use inline `onclick` attributes; no event delegation or class-based selectors. Harder to maintain than data-driven approach.

8. **Slash commands only work in main message send** — `/clear` and `/compact` are intercepted in `handleSendMessage()`, not in `handleSendMessageByName()`. Routing @mention messages cannot use slash commands.

---

## Open Questions

1. **Where is model selection intended to happen?** Is it CLI/API only, or planned for web UI?

2. **How does the compact service know which model to use?** Does it infer from `apiClient` config, or use session's `Model` field?

3. **Why is `cc_token` in localStorage instead of httpOnly cookie?** Security consideration or deliberate choice?

4. **Are there other slash commands besides `/clear` and `/compact`?** Only these two are intercepted in `handleSendMessage()`.

5. **What triggers agent mode change or re-prompt?** Session's `AgentType` and `TeamTemplate` fields exist but not exposed in web UI.

---

## Appendix: Route Summary

| Method | Path | Handler | Auth | Purpose |
|--------|------|---------|------|---------|
| POST | `/api/sessions/{session_id}/message` | handleSendMessage | Yes | Send message, intercept `/clear` and `/compact` |
| POST | `/api/sessions/{id}/interrupt` | handleInterruptSession | Yes | Interrupt active turn |
| POST | `/api/sessions/{session_id}/upload` | handleUpload | Yes | Upload file with optional caption |
| GET | `/api/sessions/list` | handleAPISessions | Yes | List sessions (for @mention autocomplete) |
| GET | `/ws/ui` | handleWSUI | Yes | WebSocket: stream messages and events |

