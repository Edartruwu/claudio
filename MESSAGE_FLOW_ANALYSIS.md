# Claudio Message-Passing Flow Analysis

## Executive Summary

This document maps the message flow in claudio's TUI to identify automatic paths for resetting active agent/team mid-session. The architecture uses **Bubble Tea** (Go TUI framework) with a central `root.Model` that dispatches all message types through a large `Update()` method, paired with an external **query engine** that streams events asynchronously through channels.

---

## 1. Main TUI Update Handler and Message Types

### Location
- **File**: `internal/tui/root.go`
- **Function**: `func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd)` at **line 660**
- **Handler Size**: ~2,100 lines of switch/case logic

### Message Type Handling

The Update handler processes **30+ distinct Bubble Tea message types**:

#### Core Streaming Messages
| Message Type | Source | Handler | Effect |
|---|---|---|---|
| `engineEventMsg` | Query engine via channel | line 1589 | Streams text, tool calls, thinking |
| `engineDoneMsg` | Query engine completion | line 1592 | Finalizes response, resumes event loop |
| `timerTickMsg` | Timer during streaming | line 1515 | 1-second viewport refresh tick |
| `logoTickMsg` | Welcome screen animation | line 1522 | 200ms logo color-wave animation |

#### Agent/Team Selection Messages
| Message Type | Source | Handler | Effect |
|---|---|---|---|
| `agentselector.AgentSelectedMsg` | Agent picker modal | line 1338 | Sets `m.currentAgent`, applies persona |
| `teamselector.TeamSelectedMsg` | Team picker modal | line 1350 | Applies team context & system prompt |

#### Input & Command Messages
| Message Type | Source | Handler | Effect |
|---|---|---|---|
| `prompt.SubmitMsg` | Prompt input | line 1258 | Submits user text, launches engine |
| `commandpalette.SelectMsg` | Command palette | line 1261 | Executes command |
| `commandpalette.CompleteMsg` | Command palette tab | line 1266 | Auto-inserts command prefix |

#### UI Control Messages
| Message Type | Source | Handler | Effect |
|---|---|---|---|
| `tea.KeyMsg` | Keyboard input | line 715 | Routing: Ctrl+C, Esc, Space (leader), etc. |
| `tea.WindowSizeMsg` | Terminal resize | line 664 | Recalculates layout |
| `prompt.VimEscapeMsg` | Vim mode exit | line 681 | No-op (prevent modal dismiss) |

#### Panel & Dialog Messages
| Message Type | Source | Handler | Effect |
|---|---|---|---|
| `permissions.ResponseMsg` | Permission dialog | line 1361 | Tool approval response |
| `panelsessions.ResumeSessionMsg` | Session picker | line 1378 | Switches session |
| `skillspanel.InvokeSkillMsg` | Skills panel | line 1390 | Launches skill as command |
| `panelconfig.ConfigChangedMsg` | Config panel | line 1407 | Applies config updates |
| `memorypanel.EditorDoneMsg` | Memory editor | line 1411 | Refreshes memory entries |
| `memorypanel.NewMemoryMsg` | Memory creator | line 1420 | Saves new memory entry |

#### File & Editor Messages
| Message Type | Source | Handler | Effect |
|---|---|---|---|
| `filepicker.SelectMsg` | File picker | line 1281 | Inserts file path or image |
| `editorFinishedMsg` | External editor | line 1442 | Loads editor content into prompt |
| `filespanel.OpenFileMsg` | Files panel | line 1453 | Opens file in external editor |
| `fileEditorFinishedMsg` | File editor exit | line 1468 | Returns to prompt |

#### Async Operation Messages
| Message Type | Source | Handler | Effect |
|---|---|---|---|
| `clipboardImageMsg` | Paste Ctrl+V | line 1364 | Adds image from clipboard |
| `modelselector.ModelSelectedMsg` | Model picker | line 1317 | Changes model + settings |
| `compactDoneMsg` | Async compaction | line 1632 | Persists compacted messages |
| `planCompactDoneMsg` | Plan mode compaction | line 1654 | Handles plan-specific compaction |

#### Panel Refresh Messages
| Message Type | Source | Handler | Effect |
|---|---|---|---|
| `taskspanel.RefreshMsg` | Tasks panel | line 1531 | Refreshes task list |
| `teampanel.RefreshMsg` | Team panel | line 1541 | Refreshes team status |
| `agui.RefreshMsg` | AGUI panel | line 1550 | Refreshes agent UI |
| `panels.ActionMsg` | Panel callbacks | line 1559 | Agent message, detail, team exit, toast |

---

## 2. Root TUI Model and Message Definitions

### Root Model Location
- **File**: `internal/tui/root.go`
- **Type**: `type Model struct` (starts ~line 47)
- **Key Fields**:
  - `currentAgent string` - currently active agent type
  - `streaming bool` - whether engine is currently running
  - `eventCh chan tuiEvent` - async event channel from engine
  - `cancelFunc context.CancelFunc` - cancels running engine
  - `engine *query.Engine` - query engine instance
  - `systemPrompt string` - agent persona system prompt
  - `userContext string` - user context (CLAUDE.md)

### Agent/Team Selection Messages

#### `agentselector.AgentSelectedMsg`
- **File**: `internal/tui/agentselector/selector.go` (line 15)
- **Struct Fields**:
  ```go
  type AgentSelectedMsg struct {
    AgentType       string
    DisplayName     string
    SystemPrompt    string
    Model           string
    DisallowedTools []string
  }
  ```
- **Creator**: `selector.go` line 132 - user presses Enter in agent picker
- **Handler**: `root.go` line 1338 `case agentselector.AgentSelectedMsg:`
  - Sets `m.currentAgent = msg.AgentType`
  - Calls `m.applyAgentPersona(msg)` (line 1735)

#### `teamselector.TeamSelectedMsg`
- **File**: `internal/tui/teamselector/selector.go` (line 15)
- **Struct Fields**:
  ```go
  type TeamSelectedMsg struct {
    TemplateName string
    IsEphemeral  bool
    Description  string
    Members      []teams.TeamTemplateMember
  }
  ```
- **Creator**: `selector.go` line 128 or 133 - user presses Enter in team picker
- **Handler**: `root.go` line 1350 `case teamselector.TeamSelectedMsg:`
  - Calls `m.applyTeamContext(msg)` (line 1792)
  - Injects team roster into system prompt

### Message Definitions File
- **File**: `internal/tui/messages.go` - defines all TUI message types for chat, rendering
  - `ChatMessage` struct with `Type`, `Content`, `ToolName`, etc.
  - `MessageType` constants: `MsgUser`, `MsgAssistant`, `MsgToolUse`, `MsgToolResult`, `MsgThinking`, `MsgError`, `MsgSystem`
  - Custom tea.Msg types:
    - `timerTickMsg struct{}` (line 250)
    - `logoTickMsg struct{}` (line 251)
    - `engineEventMsg` (wraps `tuiEvent`)
    - `engineDoneMsg` (wraps error)
    - `compactDoneMsg`
    - `planCompactDoneMsg`
    - `clipboardImageMsg`
    - `editorFinishedMsg`
    - `askUserEditorFinishedMsg`
    - `planEditorFinishedMsg`

---

## 3. Engine Run() Method and Return Behavior

### Location
- **File**: `internal/query/engine.go`
- **Primary Method**: `func (e *Engine) Run(ctx context.Context, userMessage string) error` (line 313)
- **Entry Points**:
  - `Run()` → delegates to `RunWithImages()` (line 318)
  - `RunWithImages()` → delegates to `RunWithBlocks()` (line 327)
  - `RunWithBlocks()` → main implementation

### Run() Method Structure (lines 327-600+)

```
RunWithBlocks():
  1. Inject user context (if first turn)
  2. Inject memory index (if first turn)
  3. Append user message to message history
  4. Enter main loop:
     - Check ctx.Err() → return if cancelled
     - Auto-compact if 95% of context window used
     - Call e.stream() to fetch response from Claude
     - Handler callbacks: OnTextDelta, OnThinkingDelta, OnToolUseStart, OnToolUseEnd
     - If stop_reason == "end_turn": RETURN nil (success)
     - If stop_reason == "max_tokens" && !toolUses: handle continuation
     - If tool_uses: process each tool and loop
     - Handle tool approval, cost confirmation
  5. Return nil on success OR error on failure
```

### Return Values
| Return | Meaning | Emits |
|---|---|---|
| `nil` | Streaming complete, end_turn reached | `engineDoneMsg{err: nil}` |
| `error` | Streaming failed or cancelled | `engineDoneMsg{err: <error>}` |
| `ctx.Err()` | User cancelled (Ctrl+C) | `engineDoneMsg{err: ctx.Err()}` |

### Engine-to-TUI Event Channel
- **Type**: `tuiEvent` (internal struct with `typ`, `text`, `thinking`, `toolUse`, `result`, `err`)
- **Goroutine**: Engine runs in background goroutine spawned at line 2186
- **Channel Receiver**: `root.go` line 648 - `waitForEvent()` blocks on `m.eventCh`
- **Emitted Events**:
  - `"text_delta"` → `OnTextDelta()`
  - `"thinking_delta"` → `OnThinkingDelta()`
  - `"tool_use_start"` → `OnToolUseStart()`
  - `"tool_use_end"` → `OnToolUseEnd()`
  - `"askuser_request"` → Permission dialog
  - `"done"` → `engineDoneMsg` (final)

---

## 4. AgentSelectedMsg and TeamSelectedMsg Definitions

### AgentSelectedMsg
- **Definition File**: `internal/tui/agentselector/selector.go` line 15-22
- **Creator Function**: Implicit in `Update()` method (line 131-139)
  ```go
  // User presses Enter in agent picker
  return m, func() tea.Msg {
    return AgentSelectedMsg{
      AgentType:       sel.Type,
      DisplayName:     sel.WhenToUse,
      SystemPrompt:    sel.SystemPrompt,
      Model:           sel.Model,
      DisallowedTools: sel.DisallowedTools,
    }
  }
  ```
- **Sender**: Returns from command (Bubble Tea closure)
- **Receiver**: `root.go` line 1338

### TeamSelectedMsg
- **Definition File**: `internal/tui/teamselector/selector.go` line 15-25
- **Creator Function**: Implicit in `Update()` method (line 127-138)
  ```go
  // User presses Enter in team picker
  if e.ephemeral {
    return m, func() tea.Msg {
      return TeamSelectedMsg{IsEphemeral: true}
    }
  }
  tmpl := e.tmpl
  return m, func() tea.Msg {
    return TeamSelectedMsg{
      TemplateName: tmpl.Name,
      Description:  tmpl.Description,
      Members:      tmpl.Members,
    }
  }
  ```
- **Sender**: Returns from command (Bubble Tea closure)
- **Receiver**: `root.go` line 1350

---

## 5. Timer-Based and Background Goroutine Patterns

### Timer-Based Messages

#### Streaming Timer (engineEventMsg-dependent)
- **Location**: `root.go` line 2197
- **Pattern**: `tea.Tick(time.Second, func() tea.Msg { return timerTickMsg{} })`
- **Purpose**: 1-second refresh loop to animate spinner during streaming
- **Handler**: line 1515
  ```go
  case timerTickMsg:
    if m.streaming {
      m.refreshViewport()
      return m, tea.Tick(time.Second, func(time.Time) tea.Msg { return timerTickMsg{} })
    }
  ```
- **Auto-Reset**: Yes, re-schedules itself each tick if `m.streaming` is true

#### Welcome Screen Logo Animation
- **Location**: `root.go` line 644 (Init)
- **Pattern**: `tea.Tick(200*time.Millisecond, func() tea.Msg { return logoTickMsg{} })`
- **Purpose**: Animates color-wave on welcome screen logo
- **Handler**: line 1522
  ```go
  case logoTickMsg:
    if m.isWelcomeScreen() {
      m.logoFrame++
      m.refreshViewport()
      return m, tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg { return logoTickMsg{} })
    }
  ```
- **Auto-Reset**: Yes, re-schedules if still on welcome screen

#### Panel Refresh Tickers
- **Locations**:
  - `teampanel/panel.go` line 91: `tea.Tick(500*time.Millisecond, ...)` → `teampanel.RefreshMsg`
  - `panels/taskspanel/tasks.go` line 118: `tea.Tick(500*time.Millisecond, ...)` → `taskspanel.RefreshMsg`
  - `panels/agui/panel.go` line 30: `tea.Tick(500*time.Millisecond, ...)` → `agui.RefreshMsg`
- **Purpose**: Periodically refresh team/task/AGUI panel states

#### Which-Key Menu Timeout
- **Location**: `whichkey/whichkey.go` line 199
- **Pattern**: `tea.Tick(Timeout, func() tea.Msg { return TimeoutMsg{} })`
- **Purpose**: Close leader-key menu if no follow-up key pressed

#### Toast Notifications
- **Location**: `toast.go` line 28
- **Pattern**: `tea.Tick(ToastDuration, func() tea.Msg { return ToastDismissMsg{} })`
- **Purpose**: Auto-dismiss toast after timeout

### Background Goroutines

#### Engine Streaming Goroutine
- **Location**: `root.go` line 2186-2195
- **Pattern**:
  ```go
  go func() {
    var err error
    if hasAttachments {
      err = m.engine.RunWithBlocks(ctx, blocks)
    } else {
      err = m.engine.Run(ctx, apiText)
    }
    m.eventCh <- tuiEvent{typ: "done", err: err}
  }()
  ```
- **Lifetime**: Runs until `Run()` completes or context cancelled
- **Channel**: Sends events to `m.eventCh` throughout execution
- **Purpose**: Streaming doesn't block TUI event loop

#### Sub-Agent Observer Goroutine
- **Location**: `root.go` line 2129-2133
- **Pattern**:
  ```go
  go func() {
    for req := range reqCh {
      eventCh <- tuiEvent{typ: "askuser_request", askUserReq: req}
    }
  }()
  ```
- **Purpose**: Forward sub-agent permission requests to TUI in real-time
- **Lifetime**: Runs until permission channel closes

#### Session Runtime Background Drain
- **File**: `internal/tui/sessionrt.go` line 59-99
- **Pattern**: `StartBackgroundDrain()` spawns goroutine to consume EventCh when session backgrounded
  ```go
  func (sr *SessionRuntime) StartBackgroundDrain() {
    go sr.drainLoop()
  }
  
  func (sr *SessionRuntime) drainLoop() {
    for {
      select {
      case <-sr.drainStop:
        return
      case event, ok := <-sr.EventCh:
        if !ok { return }
        sr.processEvent(event)
      }
    }
  }
  ```
- **Purpose**: Buffer engine events while user works on another session
- **Lifetime**: Until `StopBackgroundDrain()` called or EventCh closed

#### Memory Refresh Callback
- **Location**: `root.go` line 2152-2158
- **Pattern**: 
  ```go
  m.engine.SetMemoryRefreshFunc(func() string {
    // Refresh memory index after auto-compaction
  })
  ```
- **Purpose**: Called during auto-compaction in engine to re-build memory index
- **Async**: Runs in engine's context

#### Auto-Compaction Callback (Optional)
- **Location**: `root.go` line 400 in engine (async)
- **Pattern**:
  ```go
  if e.onAutoCompact != nil {
    go e.onAutoCompact(msgsCopy, summary)
  }
  ```
- **Purpose**: Background persistence of compacted messages
- **Lifetime**: Fire-and-forget

---

## Critical Paths for Agent/Team Reset

### Path 1: User Selection (Explicit)
```
User presses Space+a (or command) 
  → Focus = FocusAgentSelector
  → agentselector.Update() captures Enter
  → Emits AgentSelectedMsg
  → root.Update() line 1338
  → applyAgentPersona(msg)
```

### Path 2: Team Selection (Explicit)
```
User presses Space+t (or command)
  → Focus = FocusTeamSelector
  → teamselector.Update() captures Enter
  → Emits TeamSelectedMsg
  → root.Update() line 1350
  → applyTeamContext(msg)
```

### Path 3: Automatic Reset via Timer
**Currently No Automatic Reset Detected**
- Timer messages only refresh viewport/animation (timerTickMsg, logoTickMsg)
- No auto-agent/team-selection logic triggered by timeout
- Panel refresh tickers only update panel state, not agent/team

### Path 4: Implicit Reset via Config Change
- `panelconfig.ConfigChangedMsg` (line 1407) → `m.applyConfigChange()`
- Could trigger agent/team reset if config changes agent setting
- **Not currently implemented**

### Path 5: Session Switch with Persona
- `panelsessions.ResumeSessionMsg` (line 1378)
- Switches session but **does NOT reset agent/team**
- Each session maintains its own `currentAgent`

---

## Key Observations

### 1. **Streaming State Isolation**
- `m.streaming = true` at line 2095 prevents user input until completion
- `engineDoneMsg` (line 1592) resets `m.streaming = false`
- No agent/team change can occur during streaming

### 2. **Focus System Controls Agent/Team Selectors**
- Focus values: `FocusAgentSelector`, `FocusTeamSelector`, `FocusPrompt`, `FocusPanel`, etc.
- Selectors only emit messages when focus is on them

### 3. **No Built-In Mid-Session Auto-Reset**
- Agent persona sticks until user explicitly selects new one
- Team context persists across multiple turns
- **Implication**: Adding auto-reset would require:
  - New message type (e.g., `autoResetAgentMsg`)
  - Timer trigger in panel refresh or new dedicated ticker
  - New handler in Update() switch
  - Setter/getter for reset thresholds (turns, time, cost)

### 4. **Event Channel is Single-Source-of-Truth for Streaming**
- `waitForEvent()` (line 648) blocks on `m.eventCh`
- Engine sends **only** `engineEventMsg` during streaming
- Other messages still queued but not processed until `engineDoneMsg`

### 5. **Session Runtime Enables Multi-Session Backgrounding**
- `SessionRuntime.StartBackgroundDrain()` allows concurrent session streaming
- Each session has its own `EventCh` and message buffer
- Could enable **per-session auto-reset** logic

---

## Files Summary

| File | Role |
|---|---|
| `internal/tui/root.go` | Main TUI model + 1,640-line Update handler + Init + event loop |
| `internal/tui/messages.go` | Chat message types, timer message types, custom tea.Msg definitions |
| `internal/tui/agentselector/selector.go` | Agent picker component + AgentSelectedMsg emitter |
| `internal/tui/teamselector/selector.go` | Team picker component + TeamSelectedMsg emitter |
| `internal/tui/sessionrt.go` | Multi-session runtime with background drain goroutine |
| `internal/query/engine.go` | Query engine Run() loop + streaming event emitter |

---

## Actionable Insights for Auto-Reset Implementation

**Requirement**: Trigger agent/team selection mid-session automatically based on conditions.

**Implementation Options**:

1. **Timer-Based Reset** (E.g., reset every N turns)
   - Add new `autoResetTickerMsg` in `messages.go`
   - In `Init()`, spawn `tea.Tick(...)` for auto-reset interval
   - In Update(), on `autoResetTickerMsg`: check conditions (streaming?, turn count?)
   - If conditions met: set `m.focus = FocusAgentSelector` to trigger picker

2. **Event-Based Reset** (E.g., after every N completed turns)
   - In `handleEngineEvent()` line 2201, track turn completion
   - Increment turn counter on `"done"` event
   - Emit `autoResetMsg` to Update() if threshold crossed

3. **Hook-Based Reset** (E.g., after specific tool calls)
   - Use `hooks.Manager` to listen for tool completion
   - Fire custom hook at turn end
   - TUI observes hook and triggers reset

4. **Memory/Panel-Driven Reset**
   - AGUI panel detects agent switch need
   - Emits `panels.ActionMsg` with type `"reset_agent"`
   - Update() handler processes and opens selector

