# Auto-Reset Paths: Quick Reference

## Where to Hook for Mid-Session Agent/Team Reset

### Current Reset Triggers (User-Initiated Only)

| Trigger | File | Line | Message | Handler |
|---------|------|------|---------|---------|
| User: Space+a | `agentselector/selector.go` | 131 | `AgentSelectedMsg` | `root.go:1338` |
| User: Space+t | `teamselector/selector.go` | 128 | `TeamSelectedMsg` | `root.go:1350` |
| User: `/agent NAME` | `root.go` | 1261 | `commandpalette.SelectMsg` | Line 1261 |

---

## Automatic Reset Implementation Points

### Option A: Timer-Driven Reset (Every N seconds/minutes)

**Entry Point**: `root.go` `Init()` (line 639)

```go
// Add to Init() batch:
tea.Tick(resetInterval, func(time.Time) tea.Msg { 
  return autoResetTickerMsg{} 
})
```

**Handler**: Add to Update() switch (line 660):

```go
case autoResetTickerMsg:
  if !m.streaming && shouldAutoReset(m) {
    m.focus = FocusAgentSelector
    m.agentSelector = agentselector.New(m.currentAgent)
    return m, nil
  }
  // Re-schedule
  return m, tea.Tick(resetInterval, ...)
```

**Pros**: Simple, predictable  
**Cons**: Fixed interval ignores context (turn count, cost, inactivity)

---

### Option B: Turn-Count-Driven Reset

**Entry Point**: `handleEngineEvent()` (line 2201)

```go
case "done":
  m.turns++
  if m.turns%resetTurnsInterval == 0 && shouldAutoReset(m) {
    m.focus = FocusAgentSelector
    m.agentSelector = agentselector.New(m.currentAgent)
  }
```

**Pros**: Scales with conversation intensity  
**Cons**: Requires tracking turn counter

---

### Option C: Cost-Driven Reset

**Entry Point**: `handleEngineEvent()` (line 2201)

```go
case "done":
  m.totalCost += event.usage.TotalCost
  if m.totalCost >= costThresholdForReset && shouldAutoReset(m) {
    // Trigger selector
  }
```

**Pros**: Aligns with business constraints  
**Cons**: Cost data may not be immediate

---

### Option D: Inactivity-Driven Reset

**Entry Point**: Background monitor goroutine (new)

```go
// Start in Init():
go m.monitorInactivity(context.Background())

// In goroutine:
func (m *Model) monitorInactivity(ctx context.Context) {
  ticker := time.NewTicker(inactivityCheckInterval)
  for {
    select {
    case <-ctx.Done():
      return
    case <-ticker.C:
      if timeSinceLastInput > inactivityThreshold {
        m.eventCh <- tuiEvent{typ: "inactivity_timeout"}
      }
    }
  }
}
```

**Handler**: Add to Update() switch:

```go
case engineEventMsg:
  if msg.typ == "inactivity_timeout" && shouldAutoReset(m) {
    m.focus = FocusAgentSelector
    // ...
  }
```

**Pros**: Detects long pauses between user interactions  
**Cons**: Requires background goroutine, may interfere during long tool runs

---

### Option E: Hook-Based Reset (Most Flexible)

**Entry Point**: `hooks.Manager` (in query engine)

```go
// In engine config:
cfg.Hooks.Subscribe(hooks.TurnEnd, func(ctx context.Context) {
  // Check reset conditions
  if shouldResetAgent(cfg.TurnsCompleted, cfg.Cost) {
    eventCh <- tuiEvent{typ: "reset_needed"}
  }
})
```

**Handler**: Add to Update() switch:

```go
case engineEventMsg:
  if event.typ == "reset_needed" {
    m.focus = FocusAgentSelector
    // ...
  }
```

**Pros**: Decoupled from TUI main loop, respects context  
**Cons**: Requires hook infrastructure (already exists)

---

### Option F: Panel-Initiated Reset (AGUI/Team Panel)

**Entry Point**: Panel's `Update()` method detects need

```go
// In teampanel/panel.go or agui/panel.go:
if needsAgentSwitch(panelState) {
  return cmd, func() tea.Msg {
    return panels.ActionMsg{Type: "reset_agent"}
  }
}
```

**Handler**: `root.go` line 1559 (in `panels.ActionMsg` switch):

```go
case "reset_agent":
  m.focus = FocusAgentSelector
  m.agentSelector = agentselector.New(m.currentAgent)
  return m, nil
```

**Pros**: Driven by visible team/agent state, no separate timer  
**Cons**: Requires panel to have reset logic

---

## Decision Matrix

| Criterion | Timer | Turn | Cost | Inactivity | Hook | Panel |
|-----------|-------|------|------|------------|------|-------|
| **Simplicity** | ✅ | ✓ | ✓ | ✗ | ✓ | ✓ |
| **Respects Streaming** | ✓ | ✅ | ✅ | ✓ | ✅ | ✅ |
| **Low Overhead** | ✅ | ✅ | ✓ | ✗ | ✓ | ✅ |
| **Customizable Threshold** | ✗ | ✅ | ✅ | ✓ | ✅ | ✓ |
| **Visible Feedback** | ✗ | ✓ | ✓ | ✓ | ✗ | ✅ |
| **Interrupt Prevention** | ✗ | ✅ | ✅ | ✗ | ✅ | ✅ |

**Recommendation**: **Hook-based (Option E)** if integration exists; **Turn-count (Option B)** for quick MVP.

---

## Code Locations for Implementation

### Files to Modify

1. **`internal/tui/messages.go`**
   - Add new message type: `type autoResetMsg struct { Reason string }`
   - Register in timer-based or event-based variant

2. **`internal/tui/root.go`**
   - `Init()` (line 639): Add timer if timer-based
   - `Update()` (line 660): Add case handler for reset message
   - Add helper: `shouldAutoReset(m *Model) bool` to check conditions

3. **`internal/query/engine.go`** (if hook-based)
   - Register hook callback in `NewEngine()`
   - Emit event when threshold crossed

4. **`internal/tui/sessionrt.go`** (if multi-session reset)
   - Track reset state per session in `SessionRuntime`
   - Sync on session switch

---

## Testing Checklist

- [ ] Agent selector opens without blocking other input
- [ ] Timer doesn't fire while streaming
- [ ] Reset doesn't clear conversation history
- [ ] Reset persists to session storage (if applicable)
- [ ] Multiple resets in succession don't cause race conditions
- [ ] Session switch doesn't interfere with reset timing
- [ ] Cost/turn counters reset properly after manual agent switch

---

## Related Message Types Already in System

**For reference when implementing reset message**:

```go
// From messages.go
type timerTickMsg struct{}          // 1-second tick
type logoTickMsg struct{}            // 200ms logo animation
type engineEventMsg tuiEvent         // Streaming events
type engineDoneMsg struct { err error } // Stream completion
```

**For reference when checking conditions**:

```go
// In Model struct (root.go)
m.streaming              // bool - whether engine active
m.turns                  // int - turn counter (may need to add)
m.totalTokens            // int - accumulated tokens
m.totalCost              // float64 - accumulated cost
m.session.Current()      // *session.Session - current session
m.currentAgent           // string - active agent type
```

