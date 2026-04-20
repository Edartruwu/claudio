# Investigation: Agent Sync vs Async Message Handling

## Subject
Investigate whether sub-agent message handling differs between synchronous (run_in_background=false) and asynchronous (run_in_background=true) execution, specifically whether parent context receives full sub-agent message history on sync completion.

## Codebase Overview
- **Agent spawning & execution**: `internal/tools/agent.go` (Agent.Execute method)
- **Sub-agent runtime**: `internal/app/app.go` (runSubAgentWithMemory function)
- **Parent engine**: `internal/query/engine.go` (Engine.RunWithBlocks, executeTools, runSingleTool)
- **Message flow**: Sub-agent forwarder → messages sink → parent agent context
- **Team support**: `internal/teams/runner.go` (TeammateRunner, message capture)

## Key Findings

### Finding 1: Sub-agent messages ARE captured but NOT injected into parent on sync
- **Location**: `internal/app/app.go:909-911` (runSubAgentWithMemory)
- **Code**:
  ```go
  if sink := teams.GetMessagesSink(ctx); sink != nil {
    sink(engine.Messages())  // Full engine message history captured
  }
  ```
- **Description**: After a sub-agent engine finishes running, its complete message history is passed to a messages sink callback. For teammates, this sink stores messages in `state.EngineMessages` (line 735).
- **Data touched**: Full `[]api.Message` from sub-agent engine; stored in `TeammateState.EngineMessages`

### Finding 2: Agent.Execute returns ONLY text result, no message injection
- **Location**: `internal/tools/agent.go:406-419` (foreground team execution), `445-467` (foreground direct execution)
- **Code examples**:
  - Line 414: `result := state.Result` (from TeammateState)
  - Line 419: `return &Result{Content: result}, nil` (no InjectedMessages field set)
  - Line 458: `result, err := t.RunAgent(ctx, ...)`
  - Line 466: `return &Result{Content: result}, nil` (same pattern)
- **Description**: All code paths (team + direct, memory-aware + standard, background + foreground) return identical Result structure with only Content populated. NO InjectedMessages field is set, and NO messages are injected regardless of sync/async flag.
- **Data touched**: TeammateState.Result (text string), agent.RunAgent() output (text string)

### Finding 3: Sub-agent forwarder captures only text deltas
- **Location**: `internal/app/app.go:957-965` (subAgentForwarder.OnTextDelta), `986-998` (OnToolUseEnd)
- **Code**:
  ```go
  func (f *subAgentForwarder) OnTextDelta(text string) {
    f.text.WriteString(text)  // Accumulates in f.text
    ...
  }
  
  func (f *subAgentForwarder) OnToolUseEnd(tu tools.ToolUse, result *tools.Result) {
    // No write to f.text — tool events persist to DB but not to text output
    ...
  }
  ```
- **Description**: The forwarder is the sub-engine's handler. Tool calls and results go to database (OnTurnComplete, lines 1009-1039) and observer (for real-time TUI display), but NOT to the accumulated text output (`f.text`). Final result (line 920) is `strings.TrimSpace(forwarder.text.String())` — ONLY assistant text deltas.
- **Data touched**: strings.Builder f.text (accumulated only on OnTextDelta), pendingTools (DB persistence only)

### Finding 4: TUI observer receives tool events but does not inject into parent messages
- **Location**: `internal/tui/root.go:6799-6820` (tuiSubAgentObserver)
- **Code**:
  ```go
  func (o *tuiSubAgentObserver) OnSubAgentText(_ string, _ string) {
    // Text events are not forwarded to the main TUI chat
  }
  ```
- **Description**: Sub-agent tool events are received via observer (OnSubAgentToolStart, OnSubAgentToolEnd) and forwarded to TUI event channel for display (lines 6800-6804), but text events are explicitly discarded (line 6817-6820). Crucially, there is NO code that extracts full message history and injects it into parent agent's messages.
- **Call chain**: tuiSubAgentObserver.OnSubAgentToolStart/End → tuiEvent (via channel) → TUI event handler (root.go:2569-2610 handles subagent_tool_start/end) → updates spinner/pending tools, does NOT inject into conversation messages

### Finding 5: No code path modifies Result.InjectedMessages after sub-agent completion
- **Location**: `internal/query/engine.go:1080-1158` (runSingleTool), `1152-1157` (tool result creation)
- **Code**:
  ```go
  return toolResultBlock{
    Type:      "tool_result",
    ToolUseID: tu.ID,
    Content:   content,
    IsError:   result.IsError,
  }, result.InjectedMessages  // Returns InjectedMessages from result unchanged
  ```
- **Description**: Tool results are added to parent's messages as-is. Result.InjectedMessages field is checked (engine.go ~689-699), but Agent.Execute never populates it. No post-processing step modifies the Result before it's converted to toolResultBlock.

### Finding 6: Messages sink is wired identically for both sync and async
- **Location**: `internal/tools/agent.go:389` (Foreground: !in.RunInBackground), `397` (Background: in.RunInBackground), `446-456` (memory path), `457-467` (standard path)
- **Code**: Lines 389 & 397 show the same flag controls Team execution routing, but both paths call `t.TeamRunner.Spawn()` with `Foreground: !in.RunInBackground`. The messages sink is installed in app.go line 733 and is context-based, set ONCE per agent invocation, NOT conditionally on sync/async.
- **Description**: The messages sink callback (storing EngineMessages) is installed identically for both synchronous and asynchronous execution paths. There is NO conditional logic that changes behavior based on run_in_background flag within the messages sink mechanism.

## Symbol Map
| Symbol | File | Role |
|--------|------|------|
| `Agent.Execute` | `internal/tools/agent.go:322` | Entry point for all agent invocations (sync + async) |
| `runSubAgentWithMemory` | `internal/app/app.go:726` | Executes sub-agent engine and returns text result only |
| `subAgentForwarder` | `internal/app/app.go:936` | Handler receiving engine callbacks; accumulates text to buffer |
| `executeTools` | `internal/query/engine.go:968` | Runs tools in parallel/sequential, builds toolResultBlock from Result |
| `runSingleTool` | `internal/query/engine.go:1080` | Executes one tool, applies hooks/security, returns unchanged InjectedMessages |
| `TeammateRunner.WaitForOne` | `internal/teams/runner.go:500` | Blocks until teammate completes, returns final TeammateState |
| `GetMessagesSink` | `internal/teams/runner.go:193` | Retrieves messages sink from context (if installed) |
| `tuiSubAgentObserver.OnSubAgentToolStart/End` | `internal/tui/root.go:6799-6804` | Forwards tool events to TUI, NOT to parent messages |

## Dependencies & Data Flow

1. **Parent agent invokes Agent tool** → Agent.Execute called with input containing run_in_background flag
2. **Agent.Execute routes** → If team active: TeamRunner.Spawn (line 381), else: direct RunAgent callback (lines 446-467)
3. **Sub-agent creation** → runSubAgentWithMemory (app.go:726):
   - Creates subAgentForwarder as handler
   - Installs messages sink in context (line 733) — sink defined in line 733: `state.EngineMessages = msgs`
   - Wires observer (from context, if present)
   - Creates query.Engine with forwarder as handler
   - Calls engine.Run(ctx, prompt)
4. **Sub-agent execution loop** → engine.Run fires tool execution:
   - Each tool result goes to forwarder.OnToolUseEnd (does NOT write text)
   - Text deltas go to forwarder.OnTextDelta (accumulates in f.text)
   - Tool events also go to observer.OnSubAgentToolStart/End (if observer present)
5. **Sub-agent completion** → runSubAgentWithMemory line 909-910:
   - Calls messages sink with engine.Messages() — FULL history captured
   - Returns forwarder.text.String() as text result (line 920) — ONLY accumulated text
6. **Result returned to parent** → Agent.Execute line 419/455/466:
   - Result.Content = text result only
   - Result.InjectedMessages = nil/empty (never populated)
7. **Parent adds tool result** → engine.go executeTools (lines 1039-1070):
   - runSingleTool returns toolResultBlock with Result.Content + Result.InjectedMessages
   - Both are added to parent's messages as single user message (engine.go ~684-687)

**Key observation**: The messages sink captures full history but it is NOT used to inject into parent. It is only used for revival (resume) and by AdvisorTool (line 417) to inspect teammate's transcript.

## Risks & Observations

### No divergence found between sync and async in static analysis
- The code returns identical Result in both cases
- Messages sink is wired identically
- No conditional logic in Agent.Execute or runSubAgentWithMemory checks run_in_background to change message handling
- **Possible explanations for reported behavior**:
  1. The "massive wall of text" may be the assistant's final response text being naturally verbose (e.g., agent outputting a transcript of its own turns)
  2. The issue may be in the Anthropic API side: parent context contains parent's full message history + agent result. If the agent result text IS the full turn history (unlikely based on code), the parent's next response would reflect it.
  3. The messages sink may be accessed AFTER tool completion in a code path not visible in static analysis (e.g., in TUI event rendering)
  4. The issue may be that on sync, the parent engine's handler receives callbacks from BOTH parent AND sub-agent simultaneously, creating interleaved output perception

### Missing connections
- No code found that reads `state.EngineMessages` and injects it into parent's messages or Result.InjectedMessages
- No code found that generates a summary/transcript of sub-agent turns for injection
- No code found that branches on sync vs async AFTER message sink installation

## Open Questions

1. **When the user sees "massive wall of text" on sync completion, what exactly is in it?** Is it the agent's natural response text, or is it an actual serialization of the message history?

2. **Is the issue at the API request level?** When the parent engine sends the next request to Anthropic after receiving the Agent tool result, does the parent's messages[] somehow include the sub-agent's full history?

3. **Does the TUI render something that LOOKS like full history?** The tuiSubAgentObserver forwards tool events to the TUI display. Could the user be seeing all those tool events rendered as messages in the chat UI, distinct from what goes to the parent agent?

4. **Is there a post-tool callback or cleanup step that modifies messages?** Any hook after PostToolUse (line 1089) that might manipulate parent messages?

5. **On async, where does the agent result eventually appear?** Background tasks use TaskRuntime. Is there a separate code path for displaying their results that differs from the sync tool result injection?

---

## Hypothesis Assessment

**Hypothesis from issue**: "On sync completion, Claudio auto-generates a summary of ALL the sub-agent's message turns and injects that into the parent context."

**Finding**: **HYPOTHESIS NOT CONFIRMED BY STATIC ANALYSIS**.

Code examination shows:
- ✅ Full message history IS captured (messages sink, line 909-910)
- ✅ Messages sink IS called with engine.Messages() regardless of sync/async
- ❌ Captured messages ARE NOT injected into Result.InjectedMessages
- ❌ NO code generates a summary of turns
- ❌ NO conditional divergence between sync/async found
- ❌ Sub-agent forwarder returns ONLY text output, not message history

The messages sink stores `engine.Messages()` in `state.EngineMessages`, but this is used only for revival and advisor tool, NOT for parent message injection.

**To confirm or refute the hypothesis, runtime investigation needed**: Instrument Agent.Execute return point and parent engine's tool result injection to see actual Result.Content and InjectedMessages values on sync completion.
