# Session Management - Quick Reference & Diagrams

## State Flow Diagram

```
┌────────────────────────────────────────────────────────────────────────┐
│                           MODEL STRUCT                                  │
│  (Main TUI state holder — internal/tui/root.go:61)                     │
├────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ACTIVE SESSION STATE          │  SESSION SWITCHING STATE               │
│  ├─ messages []ChatMessage     │  ├─ prevSessionID string              │
│  ├─ streaming bool             │  ├─ vpCursor int                      │
│  ├─ totalTokens int            │  └─ vpSections []Section              │
│  ├─ totalCost float64          │                                        │
│  ├─ engine *query.Engine       │  BACKGROUND SESSIONS                  │
│  ├─ cancelFunc context.Cancel  │  └─ sessionRuntimes map[string]*...   │
│  ├─ eventCh chan tuiEvent      │     (keeps background streams alive)  │
│  └─ approvalCh chan bool       │                                        │
│                                │  SESSION MANAGER                       │
│  PANELS & LAYOUT               │  ├─ session *session.Session          │
│  ├─ activePanel panels.Panel   │  ├─ viewport viewport.Model           │
│  ├─ activePanelID PanelID      │  └─ panelPool map[PanelID]panels...   │
│  ├─ panelSplitRatio float64    │                                        │
│  └─ focus Focus                │  CONVERSATION SYNC                    │
│                                │  └─ panelPool[PanelConversation]      │
│                                │     mirrors main viewport              │
└────────────────────────────────────────────────────────────────────────┘
```

---

## Action Flow: Session Switching

```
User presses <leader>bn (next session)
    ↓
handleLeaderKey() → dispatchAction(ActionBufferNext)
    ↓
dispatchAction() case ActionBufferNext:
    ↓
switchSessionRelative(+1)
    ├─ m.session.RecentForProject(20)
    ├─ Find current idx in list
    ├─ Calculate next = (idx + 1) % len
    └─ Call doSwitchSession(sessions[next].ID)
        ↓
    doSwitchSession(nextID)
        ├─ Is nextID in m.sessionRuntimes? (background session?)
        │  YES: m.restoreSessionRuntime(rt)
        │   │   ├─ Copy rt.Engine → m.engine
        │   │   ├─ Copy rt.Messages → m.messages
        │   │   ├─ Copy rt.Streaming → m.streaming
        │   │   └─ Replay rt.TeammateEvents
        │   │
        │   NO: m.session.Resume(nextID)
        │        ├─ Load session from storage.DB
        │        ├─ Load messages from DB
        │        └─ m.engine = nil (recreate on next message)
        │
        ├─ m.refreshViewport()
        │  └─ renderMessages() + SetContent()
        │
        └─ return
    ↓
Show toast notification
    ↓
Viewport renders new session's messages
```

---

## Background Session Management

```
Current Session Streaming
    ↓
User presses <leader>bn (switch session)
    ↓
doSwitchSession(nextID):
    1. if m.streaming: m.saveSessionRuntime(curID)
    
saveSessionRuntime(curID):
    ├─ rt := NewSessionRuntime(curID)
    ├─ Copy all state to rt
    │  ├─ rt.Engine = m.engine       (KEEP REFERENCE!)
    │  ├─ rt.EventCh = m.eventCh
    │  ├─ rt.Messages = m.messages
    │  ├─ rt.Streaming = true
    │  └─ ... (all other state)
    │
    ├─ m.sessionRuntimes[curID] = rt
    ├─ rt.StartBackgroundDrain()
    │   └─ drainStop := make(chan struct{})
    │   └─ go sr.drainLoop()
    │
    └─ Reset m.* for next session
        ├─ m.engine = nil
        ├─ m.eventCh = make(chan..., 64) [NEW]
        ├─ m.messages = nil
        └─ ... (all reset)

Background Drain Loop (in separate goroutine):
    for {
        select {
        case <-sr.drainStop: return
        case event := <-sr.EventCh:
            sr.processEvent(event)  // accumulates to rt.Messages
        }
    }

When User Switches Back:
    doSwitchSession(curID)
        ├─ Check m.sessionRuntimes[curID]
        ├─ rt := m.sessionRuntimes[curID]  [EXISTS!]
        ├─ m.restoreSessionRuntime(rt)
        │  ├─ rt.StopBackgroundDrain()
        │  └─ Copy rt.* → m.*
        │
        └─ m.streaming = true  (RESUME!)
           m.engine continues executing
           m.eventCh continues receiving
```

---

## Session Storage Hierarchy

```
┌─────────────────────────────────────────────────────────┐
│          Storage Layer (internal/storage)                │
│  SQLite Database: sessions, messages tables              │
│  storage.Session {ID, Title, ProjectDir, Model, ...}    │
└──────────────────┬──────────────────────────────────────┘
                   │
┌──────────────────▼──────────────────────────────────────┐
│      Session Manager (internal/session/session.go)       │
│  Wrapper around storage.DB                               │
│  Methods:                                                │
│  ├─ Start(model)         → Create new session            │
│  ├─ Resume(id)           → Load session from DB          │
│  ├─ Current()            → Get active session            │
│  ├─ RecentForProject(n)  → List recent sessions          │
│  ├─ AddMessage()         → Persist message               │
│  ├─ GetMessages()        → Load messages                 │
│  └─ Delete(id)           → Remove session                │
└──────────────────┬──────────────────────────────────────┘
                   │
┌──────────────────▼──────────────────────────────────────┐
│      TUI Runtime (internal/tui/root.go)                  │
│  Active session execution state                          │
│  Model.session *session.Session                          │
│  ├─ m.messages []ChatMessage        (current)            │
│  ├─ m.streaming bool                                     │
│  ├─ m.engine *query.Engine                               │
│  └─ m.eventCh chan tuiEvent                              │
│                                                           │
│  Background Sessions (m.sessionRuntimes)                 │
│  ├─ sessionRuntimes["id1"] *SessionRuntime               │
│  │  ├─ .Engine (keeps executing)                         │
│  │  ├─ .EventCh (keeps receiving)                        │
│  │  ├─ .Messages (accumulated)                           │
│  │  └─ drainLoop() goroutine (running)                   │
│  │                                                        │
│  └─ sessionRuntimes["id2"] *SessionRuntime               │
│     └─ ... (more background sessions)                    │
└─────────────────────────────────────────────────────────┘
```

---

## Focus & Layout Architecture

```
┌────────────────────────────────────────────────────────────┐
│                        FOCUS STATES                         │
├────────────────────────────────────────────────────────────┤
│                                                             │
│  FocusPrompt ────────────────────┐                          │
│    (user input field)             │ (cycle with ww)        │
│                                   ↓                         │
│  FocusViewport ◄─────────────────┤                          │
│    (chat messages, vim nav)       │                         │
│                                   ↓                         │
│  FocusPanel ◄─────────────────────┘                         │
│    (side panels: Skills, Memory, etc.)                      │
│                                                             │
│  Also possible:                                             │
│  ├─ FocusPermission (tool approval)                        │
│  ├─ FocusModelSelector (model picker)                      │
│  ├─ FocusFiles (file changes)                              │
│  └─ ... (7 total)                                          │
└────────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────────┐
│                    LAYOUT RENDERING                         │
├────────────────────────────────────────────────────────────┤
│                                                             │
│  View() {                                                   │
│    1. Render viewport (m.viewport.View())                  │
│       └─ Contains m.messages rendered by renderMessages()  │
│                                                             │
│    2. Overlay dialogs if needed                            │
│       ├─ Model selector                                    │
│       ├─ Agent selector                                    │
│       ├─ Permission dialog                                 │
│       └─ Session picker                                    │
│                                                             │
│    3. Join with side panel (if active)                     │
│       └─ splitLayout(vpView, activePanel, w, h, ratio)    │
│          ├─ Panel must be ≥30 chars wide                   │
│          ├─ Ratio default: 0.65 main, 0.35 panel          │
│          └─ Separator: single │ character                  │
│                                                             │
│    4. OR join with files panel or sidebar                  │
│       ├─ Files panel: 35% width                            │
│       └─ Sidebar: token/cost display                       │
│                                                             │
│    5. Append prompt below main area                        │
│       └─ m.prompt.View()                                   │
│                                                             │
│    6. Append toast notification                            │
│       └─ m.toast.View()                                    │
│  }                                                          │
└────────────────────────────────────────────────────────────┘
```

---

## Session Panel (Picker) Overlay

```
┌─────────────────────────────────────────────────────────┐
│                 SESSION PICKER OVERLAY                    │
│        (Telescope-style picker, pressing .<Space>)        │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  Appears as overlay on viewport:                         │
│  ┌──────────────────────────────────────┐               │
│  │  type         Session Query          │               │
│  │  ──────────────────────────────────  │               │
│  │  ► 1  My Project Session   5m ago    │  ◄─ cursor   │
│  │    2  Review Code          2h ago    │               │
│  │    3  Debug Issue           1d ago   │               │
│  └──────────────────────────────────────┘               │
│                                                          │
│  Modes:                                                  │
│  ├─ modeSearch: type to filter, select with enter      │
│  │  ├─ d: delete current                                │
│  │  ├─ r: rename current                                │
│  │  └─ <,>: toggle scope (project vs all)              │
│  │                                                       │
│  ├─ modeConfirmDelete: y/n confirm                     │
│  │                                                       │
│  └─ modeRename: type new name, enter to save           │
│                                                          │
│  ResumeSessionMsg → doSwitchSession(id)                │
└─────────────────────────────────────────────────────────┘
```

---

## Key Bindings Quick Map

```
Buffer/Session Management:
  <leader>bn          (bn)  →  ActionBufferNext       [switchSessionRelative(+1)]
  <leader>bp          (bp)  →  ActionBufferPrev       [switchSessionRelative(-1)]
  <leader>bc          (bc)  →  ActionBufferNew        [createNewSession()]
  <leader>bk          (bk)  →  ActionBufferClose      [deleteCurrentSession()]
  <leader>br          (br)  →  ActionBufferRename     [renameCurrentSession()]
  <leader>,<enter>    (,\n) →  ActionBufferAlternate  [switchToAlternateSession()]

Navigation/Picker:
  <leader>.           (.)   →  ActionSessionPicker    [openSessionPicker()]
  <leader>;           (;)   →  ActionSessionRecent    [openSessionPicker()]
  <leader>/           (/)   →  ActionSearch           [openSessionPicker()]

Window/Layout:
  <leader>ww          (ww)  →  ActionWindowCycle      [cycle focus]
  <leader>wz          (wz)  →  ActionWindowZoom       [toggle panel zoom]
  <leader>wq          (wq)  →  ActionWindowClose      [close panel]

Panels (direct):
  <leader>K           (K)   →  ActionPanelSkills      [togglePanel(PanelSkills)]
  <leader>M           (M)   →  ActionPanelMemory      [togglePanel(PanelMemory)]
  <leader>T           (T)   →  ActionPanelTasks       [togglePanel(PanelTasks)]
  <leader>C           (C)   →  ActionPanelConfig      [togglePanel(PanelConfig)]
  <leader>f           (f)   →  ActionPanelFiles       [togglePanel(PanelFiles)]
```

See internal/tui/keymap/keymap.go:30-89 for full defaults.

---

## Viewport Content Sync

```
refreshViewport() {
    msg_list := m.messages
    
    IF thinkingHidden:
        filter out MsgThinking blocks
    
    result := renderMessages(
        msg_list,
        m.viewport.Width,
        m.expandedGroups,   // which tool groups are expanded
        cursorIdx,          // if FocusViewport, which message
        m.toolSpinFrame,    // for braille spinner
        m.thinkingExpanded  // expanded thinking blocks
    )
    
    content := result.Content
    m.vpSections := result.Sections  // metadata for each rendered section
    
    IF m.streaming:
        content += spinner.View()
    
    m.viewport.SetContent(content)  // ◄─ Main viewport
    
    // SYNC TO CONVERSATION PANEL
    if conv, ok := m.panelPool[PanelConversation]; ok {
        conv.SetContent(content)  // ◄─ Mirror panel
    }
}
```

---

## SessionRuntime Background Drain

```
┌─ SessionRuntime {
│  ├─ Engine *query.Engine        (shared with Model during run)
│  ├─ EventCh chan tuiEvent       (shared, receives events from Engine)
│  ├─ Messages []ChatMessage      (accumulated while backgrounded)
│  ├─ Streaming bool              (true while engine is executing)
│  ├─ TeammateEvents []tuiEvent   (buffered teammate notifications)
│  └─ draining bool               (goroutine is running)
│
└─ StartBackgroundDrain() {
    go drainLoop() {
        for {
            select {
            case <-drainStop:
                return          // Stop when switching back
            case event := <-EventCh:
                processEvent(event)  // Accumulate to Messages/Streaming
            }
        }
    }
}

Event Processing While Backgrounded:
  ├─ text_delta        → StreamText += delta, update Messages[-1]
  ├─ thinking_delta    → SpinText = "Thinking deeply..."
  ├─ tool_start        → Append MsgToolUse to Messages
  ├─ tool_end          → Append MsgToolResult to Messages
  ├─ approval_needed   → Auto-approve (can't show dialog)
  ├─ turn_complete     → Increment Turns, TotalTokens, TotalCost
  ├─ teammate_event    → Buffer in TeammateEvents (replay on foreground)
  └─ done              → Set Streaming=false, SpinText=""

When Foregrounding (restoreSessionRuntime):
  ├─ StopBackgroundDrain()        (send drainStop signal)
  ├─ Copy rt.* → m.*              (restore state)
  ├─ m.engine continues           (same reference!)
  ├─ m.eventCh continues          (same channel!)
  └─ Replay TeammateEvents        (turn into task notifications)
```

---

## Session Deletion Flow

```
deleteCurrentSession() {
    current := m.session.Current()
    oldID := current.ID
    oldTitle := current.Title
    
    // 1. Cancel current streaming
    if m.streaming:
        m.cancelFunc()  // Stop engine
    
    // 2. Kill background runtime if exists
    if rt := m.sessionRuntimes[oldID]:
        rt.StopBackgroundDrain()    // Stop drain goroutine
        rt.CancelFunc()             // Stop engine
        delete(m.sessionRuntimes, oldID)
    
    // 3. Create new empty session FIRST
    m.session.Start(m.model)
    
    // 4. Reset Model state
    m.messages = nil
    m.streaming = false
    m.engine = nil
    m.eventCh = make(chan...)
    ... (reset all)
    
    // 5. Actually delete old session
    m.session.Delete(oldID)
    
    // 6. Show notification
    m.toast.Show(fmt.Sprintf(" × %s (deleted)", oldTitle))
}

Invariant: User always has at least one session
```

---

## Model Fields Cheat Sheet

| Field | Type | Purpose |
|-------|------|---------|
| `messages` | `[]ChatMessage` | Current session's messages |
| `streaming` | `bool` | True if engine is executing |
| `engine` | `*query.Engine` | Query engine for current session |
| `eventCh` | `chan tuiEvent` | Receives events from engine |
| `sessionRuntimes` | `map[string]*SessionRuntime` | Background sessions |
| `session` | `*session.Session` | Session manager (storage wrapper) |
| `viewport` | `viewport.Model` | Main chat message display |
| `focus` | `Focus` | Where keyboard input goes |
| `activePanel` | `panels.Panel` | Current side panel (if any) |
| `panelPool` | `map[PanelID]panels.Panel` | Cached panel instances |
| `panelSplitRatio` | `float64` | Viewport/panel width ratio (0.65) |
| `prevSessionID` | `string` | For alternate session switch |
| `vpCursor` | `int` | Message cursor in viewport |
| `vpSections` | `[]Section` | Rendered section metadata |
| `totalTokens` | `int` | Cumulative tokens used |
| `totalCost` | `float64` | Cumulative API cost |
| `expandedGroups` | `map[int]bool` | Which tool groups shown |

---

## File Path Index

All paths relative to repository root:

| Feature | File | Lines |
|---------|------|-------|
| **Model Definition** | internal/tui/root.go | 61–200 |
| **Session Actions** | internal/tui/keymap/actions.go | 24–31 |
| **Key Bindings** | internal/tui/keymap/keymap.go | 30–89 |
| **Action Dispatch** | internal/tui/root.go | 3352–3610 |
| **Switch Relative** | internal/tui/root.go | 3644–3692 |
| **Switch Core** | internal/tui/root.go | 3723–3847 |
| **Alternate Switch** | internal/tui/root.go | 3694–3721 |
| **Save/Restore** | internal/tui/root.go | 3881–3999 |
| **Session Picker** | internal/tui/panels/sessions/sessions.go | 37–200+ |
| **Background Runtime** | internal/tui/sessionrt.go | 16–179 |
| **Focus Constants** | internal/tui/focus.go | 1–36 |
| **Layout** | internal/tui/layout.go | 19–64 |
| **Viewport Refresh** | internal/tui/root.go | 4942–4992 |
| **Main View** | internal/tui/root.go | 5259–5450 |
| **Conversation Panel** | internal/tui/panels/conversationpanel/panel.go | 1–89 |
| **Session Manager** | internal/session/session.go | 14–150+ |

