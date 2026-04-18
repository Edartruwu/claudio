# ComandCenter Web UI Investigation Report

## Subject
Comprehensive technical review of ComandCenter web UI architecture: message rendering, real-time updates, gallery/screenshot serving, image support, and tool result display.

---

## Codebase Overview

**Location**: `internal/comandcenter/web/`

**Key subsystems:**
- **WebServer** — main HTTP + WebSocket handler (`server.go:265`)
- **Storage** — persistent session/message/attachment DB layer
- **Hub** — in-memory event broadcast bus (UI events)
- **Templates** — Go `html/template` for server-side rendering + inline JS for client-side interactivity
- **Static files** — `/static/app.js` for WebSocket client + fetch-based interactions

**Architecture**:
- Backend: HTTP/WebSocket server with htmx + htmx AJAX for message submission
- Frontend: Vanilla JS (no React/Vue), WebSocket for real-time updates, template-rendered HTML with inline event handlers
- Data flow: Backend → hub → `fanout()` goroutine → WebSocket JSON → client JS → DOM mutations

---

## Key Findings

### 1. Message Rendering & Data Structure

**Struct**: `cc.Message` (`internal/comandcenter/session.go:24`)
```
ID              string
SessionID       string
Role            string      // assistant|user|tool_use
Content         string      // plain text or markdown
AgentName       string      // agent identity (optional)
CreatedAt       time.Time
ReplyToSession  string      // @mention routing
QuotedContent   string      // reply quote preview
```

**Attachment struct**: `cc.Attachment` (`internal/comandcenter/session.go:58`)
```
ID              string
SessionID       string
MessageID       string      // links to Message.ID (empty if session-level)
Filename        string      // stored on disk
OriginalName    string      // original upload name
MimeType        string      // e.g. image/jpeg, application/pdf
Size            int64
CreatedAt       time.Time
```

**Template rendering**: `internal/comandcenter/web/templates/components/message_bubble.html`
- Master template `message-bubble` renders 3 roles:
  - **user**: right-aligned, white bubble
  - **assistant**: left-aligned, green "msg-bubble-assistant", renders Content as markdown via `{{renderMD .Content}}`
  - **tool_use**: left-aligned, gray "msg-bubble-tool", truncates first 80 chars

**Image rendering**: YES, fully supported inline
- Lines 19–40 (user messages) + lines 49–70 (assistant):
  - Checks `isImage .MimeType` helper function
  - Renders `<img src="/uploads/{{.SessionID}}/{{.Filename}}" ... style="max-width:240px;max-height:240px;" loading="lazy">`
  - Wraps in `<a href="/uploads/...">` for click-to-enlarge
  - Falls back to file download link for non-image attachments

**MessageView wrapper**: `internal/comandcenter/web/server.go:259`
- Combines `cc.Message` + `[]cc.Attachment` for template rendering
- Attachments fetched per-message in `handlePartialMessages` (line 686–705)

---

### 2. Real-Time Updates Mechanism

**Transport**: WebSocket (not SSE, not polling)

**Handler**: `handleWSUI()` (`internal/comandcenter/web/server.go:1084`)
- Upgrades HTTP to WebSocket
- Each client = `uiClient` struct (sessionID + send channel)
- Manages client lifecycle: add on connect, remove on disconnect
- Goroutine read loop detects disconnects, write loop sends buffered messages

**Event source**: `fanout()` goroutine (`internal/comandcenter/web/server.go:1144`)
- Subscribes to `ws.hub.UIBroadcast()` channel
- Receives `attach.Event` envelopes with type enum
- Handles 2 event types:
  1. **EventMsgAssistant** (line 1148): renders message bubble → JSON `{"type":"message.assistant","html":"..."}`
  2. **EventMsgToolUse** (line 1166): renders bubble + sends separate typing indicator `{"type":"typing","tool":"...","agentName":"..."}`
- Broadcasts JSON payloads to all session-specific clients via `pushToSessionClients(sessionID, payload)`

**Client receiver**: `internal/comandcenter/web/static/app.js`
- Establishes WebSocket at app init (line 115–208)
- JSON message handler (line 127–181) dispatches on `data.type`:
  - `message.assistant` → removes typing bubble, appends HTML
  - `message.user` → appends user message bubble
  - `message.tool_use` → appends tool bubble
  - `typing` → shows transient typing bubble + updates header
  - `messages.cleared` → wipes entire message list
  - `messages.compacted` → reloads full message list from server
- Reconnect on close (3s backoff)
- Reload messages on connection restore (catch missed updates during outage)

**Push bubble function**: `pushMsgBubble()` (`internal/comandcenter/web/server.go:1071`)
- Used when user sends text-only message (htmx POST → server → renders bubble → pushes to WS clients)
- Template renders via `bubbleTmpl.ExecuteTemplate()` → JSON serialization

---

### 3. Gallery & Screenshot Serving

**Handler**: `handleDesignGallery()` (`internal/comandcenter/web/server.go:1650`)

**Data source**:
- Reads designs directory from `config.GetPaths().Designs`
- Scans subdirectories for session IDs
- Per session, checks for:
  - `{id}/bundle/mockup.html` → `HasBundle` flag
  - `{id}/handoff/spec.md` → `HasHandoff` flag
  - `{id}/screenshots/*.png` → appends filenames to `Screenshots` slice

**Struct**: `DesignSession` (`internal/comandcenter/web/server.go:1636`)
```
ID          string
HasBundle   bool
HasHandoff  bool
Screenshots []string    // list of .png filenames
```

**Template rendering**: `internal/comandcenter/web/templates/designs.html`
- Renders grid of design session cards
- Screenshot thumbnail: `<img src="/designs/static/{{.ID}}/screenshots/{{index .Screenshots 0}}" ...>`
- Badge shows count: `{{len .Screenshots}} shot(s)`
- Action buttons:
  - "Open Bundle" → `<a href="/designs/static/{{.ID}}/bundle/mockup.html" target="_blank">`
  - "Download" → `<a href="/designs/static/{{.ID}}/handoff/spec.md" download>`

**Serving screenshots**: Static file handler via `/designs/static/...` route
- Serves files directly from disk (likely static middleware, not custom handler visible in outline)
- No custom validation shown; relies on bundle creation tools to place files

---

### 4. Image Upload & File Serving

**Upload handler**: `handleUpload()` (`internal/comandcenter/web/server.go:1412`)

**Process**:
1. Parse multipart form (32 MB limit)
2. Extract file from `file` form field
3. MIME detection: read first 512 bytes, call `http.DetectContentType()`, respect Content-Type header
4. Generate random filename: `cc.NewID() + ext`
5. Create per-session directory: `{uploadsDir}/{sessionID}/`
6. Write file to disk
7. Create `cc.Message` (role: "user", Content: caption)
8. Create `cc.Attachment` linked to message
9. Render message bubble template
10. Push bubble to WebSocket clients
11. Forward file path to headless Claudio session (line 1521+)

**File serving**: `handleServeFile()` (`internal/comandcenter/web/server.go:1541`)
- Route: `/uploads/{sessionID}/{filename}`
- Path traversal protection: rejects `..` and `/\`
- Serves via `http.ServeFile()`

**URL construction in template** (`message_bubble.html:21-22`):
```html
<a href="/uploads/{{.SessionID}}/{{.Filename}}" target="_blank">
  <img src="/uploads/{{.SessionID}}/{{.Filename}}" ...>
</a>
```

**Image MIME detection** (template function `isImage`):
- Tests `MimeType` (stored in DB) against image/* prefix
- Applied at render time for conditional `<img>` vs download link

---

### 5. Tool Results & Display

**Tool result message role**: `"tool_use"` in `cc.Message.Role`

**Rendering** (`message_bubble.html:2-8`):
- Truncates Content to first 80 chars via `{{truncate 80 .Content}}`
- Shows emoji `🔧` prefix
- Displays optional AgentName with muted color
- No inline image rendering for tool results (only user/assistant messages)

**Tool result content handling**:
- Content field stores tool output as plain text
- No structured parsing for file output vs text (no distinction in UI)
- Attachments mechanism unused for tool results (only user messages carry attachments)
- Tool results expected to embed file paths as text (e.g., `/path/to/output.txt`)

**Transient typing indicator**: Separate `typing` event (fanout line 1190–1198)
- Payload: `{"type":"typing","tool":"...","agentName":"..."}`
- Client displays in `#typing-bubble` with label: `"{AgentName} is running {tool}..."`
- Removed when assistant response arrives

---

## Symbol Map

| Symbol | File | Role |
|--------|------|------|
| `Message` | `internal/comandcenter/session.go:24` | Core message struct: role, content, agent name |
| `Attachment` | `internal/comandcenter/session.go:58` | File attachment: MIME, filename, session link |
| `WebServer` | `internal/comandcenter/web/server.go:265` | Main HTTP + WS server |
| `MessageView` | `internal/comandcenter/web/server.go:259` | Template wrapper: Message + []Attachment |
| `DesignSession` | `internal/comandcenter/web/server.go:1636` | Gallery session: HasBundle, HasHandoff, Screenshots |
| `handleWSUI` | `internal/comandcenter/web/server.go:1084` | WebSocket upgrade handler |
| `fanout` | `internal/comandcenter/web/server.go:1144` | Event-to-WS broadcast loop |
| `handleDesignGallery` | `internal/comandcenter/web/server.go:1650` | Gallery page handler |
| `handleUpload` | `internal/comandcenter/web/server.go:1412` | Multipart file upload → attachment + message |
| `handleServeFile` | `internal/comandcenter/web/server.go:1541` | Static file serve with path traversal checks |
| `handlePartialMessages` | `internal/comandcenter/web/server.go:685` | Fetch message history + attachments |
| `message-bubble` | `internal/comandcenter/web/templates/components/message_bubble.html:1` | Master message template: user/assistant/tool_use roles |
| `designs.html` | `internal/comandcenter/web/templates/designs.html:1` | Gallery page template |
| `initWS()` | `internal/comandcenter/web/static/app.js:115` | Client: WebSocket connect + reconnect |
| `pushMsgBubble` | `internal/comandcenter/web/server.go:1071` | Server: render bubble + push to WS |

---

## Dependencies & Data Flow

**Message create → Display**:
1. User submits via textarea or file upload
2. `handleSendMessage()` or `handleUpload()` → creates `cc.Message` + (optional) `cc.Attachment`
3. Storage persists to DB
4. Hub broadcasts `attach.EventMsgAssistant` or `attach.EventMsgUser` (sourced from headless Claudio or upload handler)
5. `fanout()` goroutine receives event, renders `message-bubble` template, serializes JSON
6. WebSocket pushes JSON to connected clients
7. `app.js` parses JSON, inserts HTML via `appendMessage()`
8. DOM updated with rendered bubble

**Attachment retrieval → Rendering**:
1. `handlePartialMessages()` fetches messages + attachments from storage
2. Groups attachments by `messageID`
3. Constructs `[]MessageView` combining Message + Attachments
4. Renders `messages-partial` template (iterates MessageView collection)
5. Each message bubble checks `isImage` helper
6. If image: renders `<img src="/uploads/{sessionID}/{filename}">`
7. Link wraps image for click-to-enlarge

**Gallery display**:
1. User navigates to `/designs` route
2. `handleDesignGallery()` scans design directory filesystem
3. Collects session metadata: HasBundle, HasHandoff, Screenshots
4. Renders `designs.html` template with `[]DesignSession`
5. Screenshots served from static middleware (`/designs/static/{id}/screenshots/{filename}`)
6. Handoff spec served via HTTP (markdown download)
7. Bundle served as static HTML mockup

---

## Risks & Observations

### 1. **No distinction between text and file output in tool results**
- Tool results stored as plain `Content` string, no structured format
- Templates truncate to 80 chars; long output invisible
- No syntax highlighting or code block rendering for tool output
- **Risk**: Tool output containing file paths or structured data is unformatted in UI

### 2. **Attachments only for user messages**
- Tool results cannot carry file attachments via the `Attachment` mechanism
- Assistant responses have no attachment support (only user + tool_use do)
- **Risk**: If design wants to show assistant-generated files, current schema doesn't support it

### 3. **Screenshot serving has no access control**
- `/designs/static/{id}/screenshots/{filename}` directly serves from filesystem
- No validation that file belongs to authenticated user's session
- **Risk**: If design dir path enumeration is possible, unauthorized screenshot access

### 4. **MIME type stored in DB, not re-detected on serve**
- `isImage` template helper relies on `Attachment.MimeType` field
- If MIME type corrupted/spoofed in DB, image detection fails silently
- No re-validation on `handleServeFile()`

### 5. **WebSocket message loss during disconnect**
- `fanout()` pushes to in-memory channels; no persistence
- If client disconnects mid-turn, queued bubbles dropped
- Recovery via full `reloadMessages()` on reconnect (acceptable but lossy)

### 6. **No image annotation or markup capability**
- Images rendered inline but no built-in drawing/annotation tools
- Template has no UI for image editing or cropping
- **Note**: User can upload screenshots, but no in-app annotation

### 7. **Tool result content truncation**
- Line 5 of `message_bubble.html`: `{{truncate 80 .Content}}`
- Only first 80 chars of tool output visible in bubble
- User must click elsewhere to see full output (if supported elsewhere)
- **Risk**: Long tool outputs silently clipped

---

## Open Questions

1. **Where is full tool output stored/retrieved?**
   - DB schema shows only `Content` string. Are long outputs stored elsewhere (files, separate table)?
   - Does headless Claudio session store tool results separately?

2. **What is `attach.Event` envelope?**
   - Code references `attach.EventMsgAssistant`, `attach.EventMsgToolUse`
   - Need to find `attach` package definition to understand payload structure
   - What other event types exist?

3. **How does `/designs/static/` route get registered?**
   - No handler visible in `handleDesignGallery()` or server routes
   - Likely registered elsewhere as static file middleware; need to check `routes()` method fully

4. **Does renderMD function support inline images in markdown?**
   - Template calls `{{renderMD .Content}}` on assistant messages
   - If markdown contains `![alt](url)` syntax, are images rendered?
   - Separate from Attachment mechanism?

5. **Is there pagination for attachments?**
   - `handlePartialMessages()` fetches all attachments for session
   - For session with 1000s of attachments, performance implications?

6. **WebSocket payload size limits?**
   - Bubble HTML can be large (especially with image data URIs)
   - Any compression or streaming for large messages?

---

## Implementation Notes for Extensions

**To add image annotation:**
- Extend `Attachment` struct with `AnnotationData string` (JSON-serialized shapes/text)
- Create annotation editor modal in chat_view.html
- Add POST `/api/sessions/{id}/attachments/{attID}/annotate` handler

**To display full tool output:**
- Either:
  - Expand truncation in template (remove `truncate 80`)
  - Create modal dialog showing full Content on hover/click
  - Store tool output separately in file, link via Attachment

**To improve screenshot gallery:**
- Add lightbox modal for full-resolution preview
- Implement drag-to-reorder or tagging system
- Add search by session metadata

**To support image upload in assistant responses:**
- Extend `Message` struct with `Attachments []Attachment` (currently only in MessageView)
- Update storage schema to allow message-attachment links for assistant role
- Modify fanout() to include attachments in pushed bubble

---

End of investigation.
