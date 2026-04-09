# Session Management Architecture - Comprehensive Map

## 1. DIRECTORY STRUCTURE

### internal/tui/
Primary TUI components and session-related files:
```
internal/tui/
├── root.go                          [195 symbols] - Main Model struct & logic
├── sessionrt.go                     - SessionRuntime for background sessions
├── focus.go                         - Focus constants (FocusPrompt, FocusViewport, etc.)
├── layout.go                        - Layout rendering with panel integration
├── keymap/
│   ├── actions.go                   - Action definitions (ActionBufferNext, etc.)
│   └── keymap.go                    - Key binding system
├── panels/
│   ├── conversationpanel/panel.go  - Mirror panel with viewport
│   ├── sessions/sessions.go         - Session picker overlay
│   ├── panel.go                     - Panel interface definition
│   └── [other panels...]
└── [other components...]
```

### internal/services/
Services used by TUI (no dedicated session service):
```
internal/services/
├── compact/       - Message compaction for long sessions
├── memory/        - Memory extraction & management
├── analytics/     - Usage tracking
├── lsp/          - Language server support
├── mcp/          - Model Context Protocol
├── skills/       - Skill loading
├── toolcache/    - Tool caching
├── naming/       - Naming utilities
├── difftracker/  - Diff tracking
├── cachetracker/ - Cache tracking
└── notifications/ - Notification queue (in tui/)
```

### internal/session/
Session persistence layer (NOT in services/):
```
internal/session/
├── session.go       - Session manager wrapper
├── sharing.go       - Session export/import format
└── reconstruct.go   - Message reconstruction utilities
```

---

## 2. ROOT MODEL (root.go, lines 61-200)

### Primary Session-Related Fields

```go
// From root.go:61 (Model struct)

// Active session state
messages       []ChatMessage              // line 92
streaming      bool                       // line 95
streamText     *strings.Builder           // line 96
model          string                     // line 97
totalTokens    int                        // line 98
totalCost      float64                    // line 99
turns          int                        // line 101
spinText       string                     // line 102
expandedGroups map[int]bool              // line 104
lastToolGroup  int                        // line 108
toolStartTimes map[string]time.Time      // line 109

// Session switching state
prevSessionID  string                     // line 115 - for alternate (Ctrl+Shift+P) switching
vpCursor       int                        // line 116 - viewport message cursor
vpSections     []Section                  // line 117 - cached section metadata

// Concurrent session runtimes
sessionRuntimes map[string]*SessionRuntime // line 130 - background sessions kept alive

// Session manager
session        *session.Session            // line 147 - wrapper around storage.DB

// Engine (query execution)
engine         *query.Engine              // line 136
cancelFunc     context.CancelFunc         // line 140
eventCh        chan tuiEvent              // line 141
approvalCh     chan bool                  // line 142
```

### Key Session-Related Methods

**switchSessionRelative(dir int) (bool, tea.Cmd)** — root.go:3644
- Called by: ActionBufferNext (dir=1), ActionBufferPrev (dir=-1)
- Fetches recent sessions via m.session.RecentForProject(20)
- Finds current session index
- Wraps around (last → first, first → last)
- Calls doSwitchSession()
- Shows toast notification

**doSwitchSession(id string)** — root.go:3723
- Core session switching logic
- Saves current session runtime if streaming: m.saveSessionRuntime(cur.ID)
- Checks sessionRuntimes map for background runtime
  - If found: restores it and deletes from map
  - If not: calls m.session.Resume(id) to load from storage
- Resets or restores conversation state
- Calls m.refreshViewport()
- Returns (no explicit return type shown, called from switchSessionRelative which returns cmd)

**switchToAlternateSession() (bool, tea.Cmd)** — root.go:3694
- Maps to ActionBufferAlternate (`,\n` keybinding)
- Uses m.prevSessionID (set when switching away)
- Calls doSwitchSession(m.prevSessionID)
- Shows toast with session title
- Resumes streaming if needed via m.resumeStreamingCmds()

**createNewSession() (bool, tea.Cmd)** — root.go:4002
- Maps to ActionBufferNew (`bc` keybinding)
- Saves current session if streaming
- Calls m.session.Start(m.model)
- Fully resets Model state (messages=nil, streaming=false, etc.)
- Fresh engine and event channels created

**deleteCurrentSession() (bool, tea.Cmd)** — root.go:4043
- Maps to ActionBufferClose (`bk` keybinding)
- Cancels any active streaming
- Removes background runtime if exists
- Creates new session FIRST (don't leave user without a session)
- Deletes old session from storage
- Switches to new session
- Shows toast "Deleted: <title>"

**renameCurrentSession() (bool, tea.Cmd)** — root.go:4101
- Maps to ActionBufferRename (`br` keybinding)
- Currently just shows message: "Use /rename <title> to rename this session"
- Actual rename happens via /rename command (not action handler)

**saveSessionRuntime(sessionID string)** — root.go:3881
- Extracts current Model state → SessionRuntime struct
- Stored in m.sessionRuntimes[sessionID]
- Starts background drain: rt.StartBackgroundDrain()
- Resets Model state for next session:
  - m.engine = nil
  - m.eventCh = make(chan tuiEvent, 64) [new channel]
  - m.messages = nil
  - m.streaming = false
  - etc.
- Re-wires teammate event handler to new eventCh

**restoreSessionRuntime(rt *SessionRuntime)** — root.go:3929
- Inverse of saveSessionRuntime()
- Stops background drain: rt.StopBackgroundDrain()
- Locks rt.mu and copies all state back to Model:
  - m.engine = rt.Engine
  - m.messages = rt.Messages
  - m.streaming = rt.Streaming
  - etc.
- Replays buffered teammate events accumulated while backgrounded
- Restarts spinner if streaming

**resumeStreamingCmds() tea.Cmd** — root.go:3873
- Returns spinner.Tick() + waitForEvent() if m.streaming
- Called when foregrounding a session that's still streaming
- Keeps spinner animating and event loop running

**openSessionPicker() (bool, tea.Cmd)** — root.go:3627
- Maps to ActionSessionPicker (`.` keybinding)
- Creates/activates m.sessionPicker overlay
- Sets focus to FocusPanel
- User can search/filter/delete sessions

**refreshViewport()** — root.go:4942
- Renders m.messages into viewport.SetContent()
- Also syncs to conversation panel if active: conv.SetContent(content)
- Filters thinking blocks if m.thinkingHidden
- Calls renderMessages() with viewport width

---

## 3. SESSION SWITCHING ACTIONS

### Action Definitions (internal/tui/keymap/actions.go, lines 24-31)

```go
const (
    ActionBufferNext      ActionID = "buffer.next"      // line 25
    ActionBufferPrev      ActionID = "buffer.prev"      // line 26
    ActionBufferNew       ActionID = "buffer.new"       // line 27
    ActionBufferClose     ActionID = "buffer.close"     // line 28
    ActionBufferRename    ActionID = "buffer.rename"    // line 29
    ActionBufferList      ActionID = "buffer.list"      // line 30
    ActionBufferAlternate ActionID = "buffer.alternate" // line 31
)
```

### Key Bindings (internal/tui/keymap/keymap.go, lines 46-54)

```go
// defaultBindings() map[string]ActionID
"bn": ActionBufferNext,           // <leader>bn = next session
"bp": ActionBufferPrev,           // <leader>bp = previous session
"bc": ActionBufferNew,            // <leader>bc = new session
"bk": ActionBufferClose,          // <leader>bk = close/delete session
"br": ActionBufferRename,         // <leader>br = rename session
",\n": ActionBufferAlternate,     // <leader>,<enter> = alternate session
".": ActionSessionPicker,         // <leader>. = session picker overlay
";": ActionSessionRecent,         // <leader>; = recent sessions (same as picker)
```

### Action Dispatch (root.go, lines 3485-3508)

```go
// dispatchAction(action keymap.ActionID) tea.Cmd — line 3352

case keymap.ActionBufferNext:
    _, cmd := m.switchSessionRelative(1)
    return cmd                                          // line 3487

case keymap.ActionBufferPrev:
    _, cmd := m.switchSessionRelative(-1)
    return cmd                                          // line 3491

case keymap.ActionBufferNew:
    _, cmd := m.createNewSession()
    return cmd                                          // line 3495

case keymap.ActionBufferClose:
    _, cmd := m.deleteCurrentSession()
    return cmd                                          // line 3499

case keymap.ActionBufferRename:
    _, cmd := m.renameCurrentSession()
    return cmd                                          // line 3503

case keymap.ActionBufferAlternate:
    _, cmd := m.switchToAlternateSession()
    return cmd                                          // line 3507

// ActionSessionPicker, ActionSessionRecent, ActionSearch (line 3570)
case keymap.ActionSessionPicker, keymap.ActionSessionRecent, keymap.ActionSearch:
    _, cmd := m.openSessionPicker()
    return cmd                                          // line 3571
```

---

## 4. FOCUS SYSTEM (focus.go, lines 1-36)

### Focus Constants

```go
type Focus int

const (
    FocusPrompt        Focus = iota  // line 7 - user input field
    FocusViewport                    // line 8 - chat message viewport (vim-navigable)
    FocusPermission                  // line 9 - tool approval dialog
    FocusModelSelector               // line 10 - model picker
    FocusAgentSelector               // line 11 - agent persona picker
    FocusTeamSelector                // line 12 - team template picker
    FocusPanel                       // line 13 - side panel (Skills, Memory, etc.)
    FocusPlanApproval                // line 14 - plan approval dialog
    FocusAskUser                     // line 15 - AskUser dialog
    FocusAgentDetail                 // line 16 - full-screen agent conversation
    FocusFiles                       // line 17 - file changes panel
)
```

### Panel IDs

```go
type PanelID int

const (
    PanelNone PanelID = iota         // line 24
    PanelSessions                    // line 25
    PanelConfig                      // line 26
    PanelSkills                      // line 27
    PanelMemory                      // line 28
    PanelAnalytics                   // line 29
    PanelTasks                       // line 30
    PanelAgents                      // line 31
    PanelTools                       // line 32
    PanelConversation                // line 33
    PanelSessionTree                 // line 34
    PanelAgentGUI                    // line 35
)
```

### Focus Management in dispatchAction()

Focus flow when cycling with ActionWindowCycle:
- FocusPrompt → FocusViewport (if len(vpSections) > 0)
- FocusViewport → FocusPanel (if panel active) or FocusPrompt
- FocusPanel → FocusPrompt

Session switching doesn't change focus except:
- openSessionPicker() sets FocusPanel
- Session switch completes with focus intact

---

## 5. LAYOUT RENDERING (layout.go, lines 1-88)

### splitLayout() Function — line 19

```go
func splitLayout(mainView string, panel panels.Panel, 
                 totalWidth, totalHeight int, splitRatio float64) string
```

- Takes main viewport view and side panel
- Returns joined horizontal view with separator
- Panel minimum width: 30 chars (panelMinWidth constant, line 14)
- If panel width < 30, just returns mainView (hides panel)
- splitRatio: fraction of totalWidth for main area (default 0.65)

### Main Layout (root.go, View() function — line 5259)

Rendering pipeline:
1. **Viewport** - messages rendered via refreshViewport() → m.viewport.SetContent(content)
2. **Overlays** - dialogs overlaid on viewport (model selector, permission, etc.)
3. **Panel** - if active, joined horizontally via splitLayout()
4. **Files panel** - if active and no side panel, joined horizontally
5. **Sidebar** - if enabled and no other panels, joined horizontally

```go
// View() — line 5259
// 1. Call layout() for size calculations — line 5265
// 2. Join viewport + active panel side-by-side — line 5333
//    if hasPanel && m.focus != FocusAgentDetail
// 3. Join viewport + files panel — line 5341
//    else if m.filesPanel != nil && m.filesPanel.IsActive()
// 4. Build & join sidebar — line 5347-5354
//    else if m.sidebar != nil
// 5. Append prompt below main area — line 5357+
```

### Panel Integration

Panel pool: m.panelPool map[PanelID]panels.Panel
- Panels are created once and reused
- openPanel() retrieves or creates panel, caches in pool
- togglePanel() activates/deactivates panel
- Panels stay in memory even when not visible

---

## 6. CONVERSATION PANEL (conversationpanel/panel.go, lines 1-89)

**Purpose**: Mirror of main viewport, allowing user to browse history while prompt stays focused

### Panel Structure

```go
type Panel struct {
    viewport viewport.Model  // line 13 - embedded lipgloss viewport
    width    int            // line 14
    height   int            // line 15
    active   bool           // line 16 - visibility flag
    ready    bool           // line 17 - size has been set
}
```

### SetContent() Method — line 26

```go
func (p *Panel) SetContent(content string) {
    if p.ready {
        p.viewport.SetContent(content)
    }
}
```
- Called from root.go:4981 when main viewport is updated
- Keeps mirror in sync with rendered messages
- Only works after SetSize() has been called

### Size Management — line 42

```go
func (p *Panel) SetSize(w, h int) {
    p.width = w
    p.height = h
    p.viewport.Width = w
    p.viewport.Height = h - 1  // reserve 1 line for Help()
    p.ready = true
}
```
- Set during layout calculation in splitLayout()
- Reserves 1 line for help footer

### Input Handling — lines 51-76

Vim-style navigation (j/k, ctrl+d/u, G, g):
- j/down: scroll down 1
- k/up: scroll up 1
- ctrl+d: half-page down
- ctrl+u: half-page up
- G: bottom
- g: top

---

## 7. VIEWPORT OPERATIONS

### SetContent() Calls

**1. Initial viewport creation** (root.go:265)
```go
vp := viewport.New(80, 20)
vp.SetContent("")
```

**2. Main refresh** (root.go:4977)
```go
// After renderMessages(...)
m.viewport.SetContent(content)
```

**3. Conversation panel sync** (root.go:4981)
```go
if cp, ok := m.panelPool[PanelConversation]; ok {
    if conv, ok := cp.(*conversationpanel.Panel); ok {
        conv.SetContent(content)
    }
}
```

**4. AGUI panel viewport** (panels/agui/panel.go:230)
```go
p.rightVP.SetContent(content)
```

### Content Generation — root.go:4942 (refreshViewport)

```go
func (m *Model) refreshViewport() {
    var content string
    
    if len(m.messages) == 0 && !m.streaming {
        content = m.welcomeScreen()      // line 4946
        m.vpSections = nil
    } else {
        // Filter thinking blocks if hidden
        msgs := m.messages
        if m.thinkingHidden {
            // ... filter logic ...
        }
        result := renderMessages(msgs, m.viewport.Width,
                                m.expandedGroups, cursorIdx,
                                m.toolSpinFrame, m.thinkingExpanded)
        content = result.Content                 // line 4965
        m.vpSections = result.Sections          // line 4966
        
        // Append inline spinner when streaming
        if m.streaming {
            spinView := m.spinner.View()
            if spinView != "" {
                content += "\n\n" + spinView
            }
        }
    }
    
    m.viewport.SetContent(content)              // line 4977
    // Sync to conversation panel...
}
```

---

## 8. SESSION PANEL (panels/sessions/sessions.go, lines 1-200+)

### Purpose
Telescope-style session picker overlay for browsing/filtering sessions

### Panel Structure

```go
type Panel struct {
    session   *session.Session
    active    bool
    width     int
    height    int
    cursor    int
    sessions  []storage.Session
    filtered  []storage.Session
    query     string
    mode      mode                // modeSearch, modeConfirmDelete, modeRename
    scopeAll  bool               // false = project, true = all projects
    renameText string
}
```

### Key Methods

**refresh()** — line 77
- Fetches sessions based on scope (project or all)
- Calls applyFilter() to match against query

**applyFilter()** — line 86
- Filters sessions by query string (case-insensitive)
- Searches in label, project dir, and model

**Update()** — line 104
- Handles delete confirmation (y/n)
- Handles rename mode (character input, enter to confirm)
- Normal search mode: navigate with arrows, j/k, delete (d), rename (r)

**View()** — returns formatted list
- Shows session title, time ago, model
- Highlights current cursor position

---

## 9. SESSION RUNTIME (sessionrt.go, lines 1-200)

**Purpose**: Keeps streaming sessions alive in the background when user switches sessions

### SessionRuntime Structure

```go
type SessionRuntime struct {
    mu sync.Mutex
    
    SessionID string
    
    // Engine state — keeps execution alive
    Engine     *query.Engine
    CancelFunc context.CancelFunc
    EventCh    chan tuiEvent
    ApprovalCh chan bool
    
    // Conversation state — accumulated messages
    Messages       []ChatMessage
    StreamText     *strings.Builder
    Streaming      bool
    TotalTokens    int
    TotalCost      float64
    Turns          int
    ExpandedGroups map[int]bool
    LastToolGroup  int
    SpinText       string
    MessageQueue   []string
    ToolStartTimes map[string]time.Time
    
    // Buffered teammate events — replayed when foregrounded
    TeammateEvents []tuiEvent
    
    // Background drain — goroutine consuming events
    draining   bool
    drainStop  chan struct{}
}
```

### Key Methods

**NewSessionRuntime(sessionID string)** — line 49
- Creates new runtime with fresh channels and maps
- EventCh: buffered channel with 64 slots
- ExpandedGroups, ToolStartTimes initialized

**StartBackgroundDrain()** — line 61
- Starts goroutine that consumes EventCh while session is backgrounded
- Prevents channel from filling up and blocking engine
- Accumulates events in sr.TeammateEvents for replay

**StopBackgroundDrain()** — line 75
- Called when foregrounding the session
- Closes drainStop channel to signal goroutine to exit
- Sets sr.draining = false

**drainLoop()** — line 85
- Runs in background
- Selects on sr.drainStop and sr.EventCh
- Calls sr.processEvent() for each event

**processEvent(event tuiEvent)** — line 102
- Accumulates state from engine events:
  - text_delta: appends to StreamText, updates messages
  - thinking_delta: updates SpinText
  - tool_start/tool_end: accumulates tool messages
  - approval_needed: auto-approves
  - turn_complete: updates tokens/cost
  - done: finalizes, sets Streaming=false
  - teammate_event: buffers for replay

---

## 10. SESSION SERVICE & PERSISTENCE

### Session Manager (internal/session/session.go, lines 1-150+)

**NOT in internal/services/** — instead in internal/session/

```go
type Session struct {
    db      *storage.DB
    current *storage.Session
}

// Core methods:
New(db *storage.DB) *Session
Start(model string) (*storage.Session, error)
Current() *storage.Session
Resume(id string) (*storage.Session, error)
List(limit int) ([]storage.Session, error)
AddMessage(role, content, msgType string) error
GetMessages() ([]storage.MessageRecord, error)
SetTitle(title string) error
SaveSummary(summary string) error
RenameByID(id, title string) error
Search(query string, limit int) ([]storage.Session, error)
RecentForProject(limit int) ([]storage.Session, error)
Delete(id string) error
```

**Note**: No dedicated session management service in internal/services/.
Session logic is split:
- internal/session/session.go — API wrapper around storage
- internal/tui/sessionrt.go — Runtime management for concurrent sessions
- internal/storage/db.go — Persistent storage (SQLite)

### Storage Layer (storage package)

Sessions stored in database:
- storage.Session struct contains: ID, Title, ProjectDir, Model, UpdatedAt, etc.
- m.db methods: CreateSession(), GetSession(), ListSessions(), DeleteSession(), etc.

---

## 11. UPDATE FLOW FOR SESSION SWITCHING

### Main Update Loop — root.go (implicit)

The TUI receives Bubble Tea events (key presses, tui events from engine):

1. **Key press event**
   - Parsed by handleLeaderKey() or regular key handler
   - Mapped to ActionID via keymap
   - Dispatched to dispatchAction()

2. **dispatchAction() for session actions**
   ```go
   case keymap.ActionBufferNext:
       _, cmd := m.switchSessionRelative(1)  // calls doSwitchSession()
       return cmd
   ```

3. **switchSessionRelative(dir) execution**
   - Fetches m.session.RecentForProject(20)
   - Finds current session index
   - Calculates next = (idx + dir) % len(sessions)
   - Calls m.doSwitchSession(sessions[next].ID)
   - Returns spinner toast command

4. **doSwitchSession(id) execution**
   ```go
   // Save current session's streaming state if needed
   if cur := m.session.Current(); cur != nil {
       m.prevSessionID = cur.ID
       if m.streaming {
           m.saveSessionRuntime(cur.ID)  // Background drain starts
       }
   }
   
   // Check for background runtime
   if rt, ok := m.sessionRuntimes[id]; ok {
       m.restoreSessionRuntime(rt)       // Restore state + replay events
       delete(m.sessionRuntimes, id)
       m.session.Resume(id)
       m.refreshViewport()
       return
   }
   
   // Load from storage
   resumed, err := m.session.Resume(id)  // Load session from DB
   // ... clear messages, reset engine, load messages from DB
   m.refreshViewport()
   ```

5. **Model state after switch**
   - m.messages loaded from DB (or from rt if background runtime)
   - m.streaming may be true (if session was backgrounded while streaming)
   - m.engine is nil (will be created when user sends next message)
   - Focus unchanged (usually FocusPrompt)
   - Viewport refreshed with new messages

6. **Rendering**
   - View() called each frame
   - Shows new session's messages in viewport
   - Toast shows session switch notification

---

## 12. CONCURRENT SESSION MANAGEMENT FLOW

### Session Lifecycle Diagram

```
START NEW SESSION
    ↓
m.session.Start(model)
    ↓
m.messages = nil, m.streaming = false
m.engine = nil, m.eventCh fresh
    ↓
USER SENDS MESSAGE
    ↓
m.engine created, streaming starts
    ↓
USER SWITCHES SESSION (while streaming)
    ↓
m.saveSessionRuntime(curID)
    ├─ Create rt := NewSessionRuntime(curID)
    ├─ rt.Engine = m.engine (keep reference)
    ├─ rt.EventCh = m.eventCh (transfer channel)
    ├─ rt.Streaming = true
    ├─ rt.StartBackgroundDrain() [goroutine consumes EventCh]
    └─ m.sessionRuntimes[curID] = rt
    ↓
m.doSwitchSession(nextID)
    ├─ Check m.sessionRuntimes[nextID]
    ├─ If exists:
    │   ├─ m.restoreSessionRuntime(rt) [restore state + replay events]
    │   └─ m.session.Resume(nextID)
    └─ If not:
        └─ m.session.Resume(nextID) [load from DB]
    ↓
USER CONTINUES IN NEW SESSION
    ↓
USER SWITCHES BACK
    ↓
m.restoreSessionRuntime(rt)
    ├─ rt.StopBackgroundDrain() [stop consuming EventCh]
    └─ Copy rt.* → m.* [restore streaming state]
    ↓
m.resumeStreamingCmds()
    └─ spinner.Tick() + waitForEvent()
    ↓
STREAMING CONTINUES IN ORIGINAL SESSION
    ↓
DELETE SESSION
    ↓
m.deleteCurrentSession()
    ├─ Cancel streaming (m.cancelFunc())
    ├─ If rt exists:
    │   ├─ rt.StopBackgroundDrain()
    │   ├─ rt.CancelFunc() [cancel engine]
    │   └─ delete(m.sessionRuntimes, id)
    ├─ m.session.Start() [create new empty session]
    └─ m.session.Delete(oldID) [remove from storage]
```

### Key Invariant

At any moment:
- Exactly ONE session is "foreground" — m.session.Current()
- Zero or more sessions are "background" — entries in m.sessionRuntimes
- Background sessions with m.streaming=true have drain goroutines running
- Each background session has its own Engine + EventCh running independently
- Switching foreground = swap m.messages, m.engine, m.eventCh references

---

## 13. EVENT FLOW FOR SESSION SWITCHES

### Event Channel Architecture

```
Current foreground session:
    m.engine → m.eventCh [buffered 64]
           ↓
    m.Update() reads from m.eventCh
           ↓
    Renders m.messages in viewport
           ↓
    m.refreshViewport()

Background session:
    rt.Engine → rt.EventCh [buffered 64, same reference!]
           ↓
    rt.drainLoop() reads from rt.EventCh
           ↓
    rt.processEvent(event) accumulates state
           ↓
    rt.Messages, rt.StreamText, rt.Streaming updated

When switching back:
    m.eventCh = rt.EventCh [restore reference]
           ↓
    m.Update() resumes reading from same channel
           ↓
    rt.TeammateEvents replayed (teammate notifications)
```

**Critical**: The same EventCh reference is passed to both:
- m.Update() when session is foreground
- rt.drainLoop() when session is background

No event is lost because the buffered channel holds events until consumed.

---

## 14. VIEWPORT CONTENT FLOW

```
renderMessages(m.messages, viewport.Width, ...)
    ↓
Returns {Content: string, Sections: []Section}
    ↓
m.viewport.SetContent(content)
    ↓
Root.View() calls m.viewport.View()
    ↓
Viewport renders with scrolling
    ↓
Conversation panel synced:
    m.panelPool[PanelConversation].SetContent(content)
```

---

## 15. KEY FILES & LINE REFERENCES

| File | Lines | Purpose |
|------|-------|---------|
| internal/tui/root.go | 61-200 | Model struct definition |
| internal/tui/root.go | 262-280 | New() constructor |
| internal/tui/root.go | 3352-3610 | dispatchAction() handler |
| internal/tui/root.go | 3485-3508 | Buffer/session action cases |
| internal/tui/root.go | 3627-3642 | openSessionPicker() |
| internal/tui/root.go | 3644-3692 | switchSessionRelative() |
| internal/tui/root.go | 3694-3721 | switchToAlternateSession() |
| internal/tui/root.go | 3723-3847 | doSwitchSession() |
| internal/tui/root.go | 3873-3878 | resumeStreamingCmds() |
| internal/tui/root.go | 3881-3926 | saveSessionRuntime() |
| internal/tui/root.go | 3929-3999 | restoreSessionRuntime() |
| internal/tui/root.go | 4002-4041 | createNewSession() |
| internal/tui/root.go | 4043-4099 | deleteCurrentSession() |
| internal/tui/root.go | 4101-4105 | renameCurrentSession() |
| internal/tui/root.go | 4942-4992 | refreshViewport() |
| internal/tui/root.go | 5259-5450 | View() main layout |
| internal/tui/sessionrt.go | 16-46 | SessionRuntime struct |
| internal/tui/sessionrt.go | 49-57 | NewSessionRuntime() |
| internal/tui/sessionrt.go | 61-72 | StartBackgroundDrain() |
| internal/tui/sessionrt.go | 75-83 | StopBackgroundDrain() |
| internal/tui/sessionrt.go | 85-99 | drainLoop() |
| internal/tui/sessionrt.go | 102-179 | processEvent() |
| internal/tui/focus.go | 1-36 | Focus & PanelID constants |
| internal/tui/layout.go | 19-64 | splitLayout() |
| internal/tui/keymap/actions.go | 10-31 | Action constants |
| internal/tui/keymap/actions.go | 59-120 | Action metadata registry |
| internal/tui/keymap/keymap.go | 30-89 | defaultBindings() |
| internal/tui/panels/conversationpanel/panel.go | 11-30 | Panel struct & SetContent() |
| internal/tui/panels/sessions/sessions.go | 37-85 | Panel struct & refresh() |
| internal/session/session.go | 14-150 | Session manager |

---

## 16. SUMMARY OF ARCHITECTURE

**Session Management is distributed across three layers:**

1. **Storage (internal/session + internal/storage)**
   - Persistent session metadata (title, project, model, messages)
   - Database: SQLite with sessions table

2. **Runtime (internal/tui/root.go + internal/tui/sessionrt.go)**
   - In-memory session state during execution
   - Concurrent runtimes for background sessions
   - Background drain goroutines for event consumption

3. **UI (internal/tui/panels/sessions + internal/tui/keymap)**
   - Session switching actions (buffer next/prev/new/delete)
   - Session picker overlay (telescope-style)
   - Focus management and layout integration

**Key innovation**: SessionRuntime + background drain allows multiple sessions to stream concurrently while only one is visible at a time. When switching sessions, the foreground session's execution keeps running in the background until the user switches back or deletes it.

