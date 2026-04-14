# Message Flow Diagrams

## High-Level Architecture: TUI ↔ Engine Event Loop

```
┌─────────────────────────────────────────────────────────────┐
│                     Bubble Tea Main Loop                    │
│                    (root.Model.Update)                      │
└─────────────────────────────────────────────────────────────┘
               ↑                                      ↓
               │                                      │
    Input Msg  │                                      │ Cmd (Batch)
  (KeyMsg,etc) │                                      │
               │                                      ↓
         ┌─────────────────────────────────────────────────┐
         │ Model.Update(msg tea.Msg)                       │
         │ (root.go:660 - 2100+ lines)                     │
         │                                                 │
         │ Handles 30+ message types:                      │
         │  - tea.KeyMsg, tea.WindowSizeMsg              │
         │  - agentselector.AgentSelectedMsg             │
         │  - engineEventMsg, engineDoneMsg              │
         │  - timerTickMsg, logoTickMsg                  │
         │  - permissions.ResponseMsg                    │
         │  - panels.ActionMsg, etc.                     │
         └─────────────────────────────────────────────────┘
               ↑                                      ↓
               │                                      │
    ┌──────────┴──────────┐              ┌───────────┴────────────┐
    │                     │              │                        │
    │  Async Event Loop   │              │  Compute / State Change│
    │  (waitForEvent)     │              │                        │
    │                     │              │ - Apply agent persona  │
    └──────────┬──────────┘              │ - Apply team context   │
               ↑                          │ - Process user input   │
               │                          │ - Handle tool approval │
               │                          └───────────┬────────────┘
               │                                      │
        ┌──────┴──────────────────────────────┬──────┘
        │                                     │
        │  eventCh <- tuiEvent                │
        │  (internal channel)                 │
        │                                     │
        └──────┬──────────────────────────────┘
               │
        ┌──────┴──────────────────────────────┐
        │                                     │
        │    Background Goroutine             │
        │  (engine.Run or streaming)          │
        │                                     │
        │  Sends events:                      │
        │  - text_delta → OnTextDelta()       │
        │  - thinking_delta → ...             │
        │  - tool_use_start → ...             │
        │  - tool_use_end → ...               │
        │  - done → engineDoneMsg             │
        │  - askuser_request → ...            │
        │                                     │
        └─────────────────────────────────────┘
```

---

## Message Flow: Agent/Team Selection

```
User Input (KeyMsg)
    │
    ├─ Space+a or /agent command
    │  │
    │  ├─> Model.Update() routes to agent selector
    │  │   (line 1203-1209)
    │  │
    │  └─> agentselector.Model.Update()
    │      (agentselector/selector.go:93)
    │      │
    │      └─ User presses Enter
    │         │
    │         └─> Return Bubble Tea Cmd with closure:
    │            func() tea.Msg { return AgentSelectedMsg{...} }
    │
    └─> Back to Model.Update() (line 1338)
        │
        └─> case agentselector.AgentSelectedMsg:
           ├─ m.focus = FocusPrompt
           ├─ m.currentAgent = msg.AgentType
           ├─ m = m.applyAgentPersona(msg)
           │  (line 1735)
           │  ├─ Merges system prompt from agent def
           │  ├─ Sets DisallowedTools
           │  └─ Updates engine system context
           └─ Return updated model
```

---

## Message Flow: Streaming and Completion

```
User Submits Query (handleSubmit)
    │
    ├─ m.streaming = true
    ├─ m.engine.SetMessages(...)
    │
    └─ Spawn background goroutine:
       │
       │  go func() {
       │    err := m.engine.Run(ctx, text)
       │    m.eventCh <- tuiEvent{typ: "done", err: err}
       │  }()
       │
       └─ Return Cmd batch:
          ├─ m.spinner.Tick()           (1st tick)
          ├─ m.waitForEvent()           (blocks on eventCh)
          └─ tea.Tick(1s, timerTickMsg) (viewport refresh)


During Streaming
    │
    ├─ Engine calls handler callbacks:
    │  ├─ OnTextDelta(text)
    │  ├─ OnThinkingDelta(text)
    │  ├─ OnToolUseStart(toolUse)
    │  └─ OnToolUseEnd(toolUse, result)
    │
    └─ Each callback emits to eventCh:
       │
       └─ tuiEvent{typ: "text_delta", text: ...}
          tuiEvent{typ: "tool_use_start", toolUse: ...}
          ... etc ...


Engine Completion
    │
    └─ engine.Run() returns (nil or error)
       │
       └─ Goroutine emits:
          m.eventCh <- tuiEvent{typ: "done", err: error}
          │
          └─> waitForEvent() receives
              │
              └─> Model.Update(engineEventMsg)
                  (line 1589)
                  │
                  └─> handleEngineEvent(tuiEvent)
                      (line 2201)
                      │
                      └─> Process event type "done"
                          │
                          ├─ m.streaming = false
                          ├─ m.focus = FocusPrompt
                          ├─ Process messageQueue
                          └─ Return m.waitForEvent()
                             (re-enter event loop)
```

---

## Message Flow: Timer-Based Events

```
Init() called (line 639)
    │
    └─> tea.Batch(
           m.waitForEvent(),
           tea.Tick(200ms, func() { return logoTickMsg{} })
           ...
        )
           │
           └─> logoTickMsg fires every 200ms
               │
               └─> Model.Update(logoTickMsg)
                   (line 1522)
                   │
                   ├─ if m.isWelcomeScreen():
                   │  ├─ m.logoFrame++
                   │  ├─ m.refreshViewport()
                   │  └─ return m, tea.Tick(200ms, ...)
                   │
                   └─ else: stop (don't reschedule)


When Streaming Starts
    │
    └─> returnCmd = tea.Tick(1s, func() { return timerTickMsg{} })
        (line 2197)
        │
        └─> timerTickMsg fires every 1s
            │
            └─> Model.Update(timerTickMsg)
                (line 1515)
                │
                ├─ if m.streaming:
                │  ├─ m.refreshViewport()
                │  └─ return m, tea.Tick(1s, ...)
                │
                └─ else: stop (stream finished)
```

---

## Message Flow: Panel and Dialog Messages

```
User presses Space (leader key in Normal mode)
    │
    ├─> m.leaderSeq = "pending"
    ├─> tea.Tick(timeout, whichkey.TimeoutMsg)
    │
    └─> Display which-key popup
        │
        └─> User selects action (e.g., "Show Team")
            │
            └─> Set m.focus = FocusPanel
                m.activePanel = teampanel.NewPanel()
                │
                └─> Panel.Update() gets keys until dismissed
                    (line 856-863)
                    │
                    ├─ Panel processes: esc, q → m.closePanel()
                    ├─ Panel emits: teampanel.RefreshMsg
                    │  (periodically via tea.Tick 500ms)
                    │
                    └─ Return m.waitForEvent()


Panel Refresh Loop
    │
    └─> tea.Tick(500ms, func() { return RefreshMsg{} })
        (teampanel/panel.go:91)
        │
        └─> Model.Update(teampanel.RefreshMsg)
            (root.go:1541)
            │
            └─> if m.activePanel is teampanel:
               ├─ cmd := tp.HandleRefresh()
               ├─ Fetch fresh team state
               └─ Refresh panel display


User presses Action in Panel
    │
    └─> Panel emits: panels.ActionMsg{Type: "...", Payload: ...}
        │
        └─> Model.Update(panels.ActionMsg)
            (root.go:1559)
            │
            ├─ case "agent_message":
            │  └─ m.prompt.SetValue(">>" + name)
            │
            ├─ case "agent_detail":
            │  └─ m.openAgentDetail(agentID)
            │
            ├─ case "exit_team":
            │  └─ m.closePanel()
            │
            └─ case "agui_toast":
               └─ m.addMessage(ChatMessage{...})
```

---

## Message Flow: Current No Auto-Reset Path

```
┌─────────────────────────────────────────┐
│    Timer-Based Message Flow             │
│    (Currently: Animation Only)          │
└─────────────────────────────────────────┘
                    │
        ┌───────────┴───────────┐
        │                       │
    timerTickMsg            logoTickMsg
    (1 sec during            (200ms
     streaming)           on welcome)
        │                       │
        ├─ refreshViewport      └─ m.logoFrame++
        │  (update spinner)        refreshViewport
        └─ reschedule
           (no agent check)


┌─────────────────────────────────────────┐
│  Panel Refresh Loop                     │
│  (Currently: State Update Only)         │
└─────────────────────────────────────────┘
                    │
        ┌───────────┴──────────┬──────────┐
        │                      │          │
    teampanel             taskspanel   agui
    (500ms tick)          (500ms tick) (500ms)
        │                      │
        └──────────┬───────────┘
                   │
           Panel.HandleRefresh()
                   │
            (Fetch new state)
                   │
           └─ NO agent/team reset logic


┌──────────────────────────────────────────┐
│  TO ADD Auto-Reset                       │
│                                          │
│  Option 1: New timer message             │
│    - autoResetTickerMsg{turns, cost}    │
│    - Trigger every N seconds or turns    │
│    - Check conditions in Update()        │
│                                          │
│  Option 2: Hook-based callback           │
│    - Engine fires hook.TurnEnd           │
│    - TUI observes and emits msg          │
│                                          │
│  Option 3: Panel-driven                  │
│    - Panel detects need via metrics      │
│    - Emits ActionMsg type:"reset_agent"  │
└──────────────────────────────────────────┘
```

---

## Data Flow: Agent Persona Application

```
AgentSelectedMsg received
    │
    ├─ msg.AgentType       (e.g., "frontend-senior")
    ├─ msg.SystemPrompt    (e.g., "You are a React expert...")
    ├─ msg.Model           (e.g., "claude-3-5-sonnet")
    ├─ msg.DisallowedTools (e.g., ["bash"])
    │
    └─> applyAgentPersona(msg) [line 1735]
        │
        ├─ m.currentAgent = msg.AgentType
        ├─ m.systemPrompt += "\n\n" + msg.SystemPrompt
        │  (APPENDED, not replaced)
        │
        ├─ m.apiClient.SetModel(msg.Model)
        │  (Updates API client)
        │
        ├─ Store DisallowedTools somewhere
        │  (Used by engine for tool filtering)
        │
        └─> m.engine.SetSystem(m.systemPrompt)
            (Updates engine with merged prompt)
            │
            └─ On next m.engine.Run():
               └─ New turns use updated system prompt
                  (only affects NEW tool calls, not history)
```

---

## Data Flow: Team Context Application

```
TeamSelectedMsg received
    │
    ├─ msg.TemplateName  (e.g., "frontend-team-2b61573e")
    ├─ msg.IsEphemeral   (e.g., true for "New Ephemeral Team")
    ├─ msg.Description   (e.g., "React specialist team")
    ├─ msg.Members       ([]TeamTemplateMember)
    │
    └─> applyTeamContext(msg) [line 1792]
        │
        ├─ Store team name in m.currentTeam (if applicable)
        ├─ Build roster block from msg.Members
        │
        ├─ m.systemPrompt += "\n\n" + rostBlock
        │  (e.g., "The team is ready: Frontend-Senior, Backend-Mid...")
        │
        ├─ If m.appCtx.TeamRunner != nil:
        │  └─ m.appCtx.TeamRunner.SetTeam(template)
        │     (Configures team execution engine)
        │
        └─> m.engine.SetSystem(m.systemPrompt)
            (Updates engine with team context)
            │
            └─ On next m.engine.Run():
               └─ Claude sees team roster in system prompt
                  └─ Subsequent tool calls can use >>agentname
```

---

## Critical State Transitions

```
Idle State
    │
    ├─> User submits query
    │   │
    │   └─ m.streaming = true
    │      m.focus = FocusPrompt
    │      Spawn engine goroutine
    │
    └─> Streaming State
       │
       ├─ timerTickMsg fires → refreshViewport (spinner animation)
       ├─ engineEventMsg fires → accumulate text/tools
       ├─ User presses Ctrl+C → m.cancelFunc() → context cancelled
       │                                           ↓
       │                                      engine.Run() returns
       │                                      ↓
       │                                      eventCh <- done
       │
       └─ engineDoneMsg arrives
          │
          ├─ m.streaming = false
          ├─ m.focus = FocusPrompt
          ├─ Process m.messageQueue
          │
          └─> Back to Idle State


Agent Reset (if auto-added)
    │
    ├─ [NEW MESSAGE TYPE emitted]
    │  autoResetMsg or panels.ActionMsg{type: "reset_agent"}
    │
    ├─ Guard: !m.streaming (NO reset during streaming)
    │
    └─> m.focus = FocusAgentSelector
       m.agentSelector = agentselector.New(m.currentAgent)
       │
       └─ User can now pick new agent (or press Esc to cancel)
          │
          ├─ If Enter → AgentSelectedMsg → applyAgentPersona()
          │            ↓
          │       m.focus = FocusPrompt (back to Idle)
          │
          └─ If Esc → DismissMsg → m.focus = FocusPrompt (no change)
```

---

## Key Entry/Exit Points for Message Types

| Message Type | Entry | Exit/Handler |
|---|---|---|
| `tea.KeyMsg` | Terminal input | Update() line 715+ |
| `tea.WindowSizeMsg` | Terminal resize | Update() line 664 |
| `AgentSelectedMsg` | agentselector.Update() | Update() line 1338 |
| `TeamSelectedMsg` | teamselector.Update() | Update() line 1350 |
| `engineEventMsg` | waitForEvent() channel | Update() line 1589 |
| `engineDoneMsg` | Wrapped in engineEventMsg | Update() line 1592 |
| `timerTickMsg` | Scheduled in Init or Update | Update() line 1515 |
| `logoTickMsg` | Scheduled in Init | Update() line 1522 |
| `panels.ActionMsg` | Panel callbacks | Update() line 1559 |
| `permissions.ResponseMsg` | Permission dialog | Update() line 1361 |

