# Investigation Report: First Message After `/clear` Gets No Response

## Subject
Investigated why the first user message after CommandCenter's `/clear` command receives no AI response, while subsequent messages work fine.

## Codebase Overview
- **CC Server**: `internal/comandcenter/web/server.go` handles `/clear` HTTP endpoint (~line 697). Deletes DB messages, sends `EventClearHistory` to attached claudio engine via `hub.Send()`, returns HTTP 204.
- **Engine Side**: `internal/cli/root.go` runs in headless/attach mode. Registers callbacks for attach client events (~lines 499-507):
  - `onClearHistory`: calls `appInstance.ClearHistory()` then `engine.SetMessages(nil)`
  - `onSetMessages`: calls `engine.SetMessages(msgs)`
  - `onUserMessage`: calls `appInstance.InjectPayload(payload)` → sends to `InjectCh` (cap 8)
- **Main Loop**: `internal/cli/root.go:569-623` is a select loop that reads from `appInstance.InjectCh` and calls `engine.Run()` or `engine.RunWithImages()`.
- **Query Engine**: `internal/query/engine.go` orchestrates AI conversation.

## Key Findings

### Finding 1: Injection Flags Not Reset After Clear
- **Location**: `internal/query/engine.go:274-278` (SetMessages method)
- **Issue**: When `SetMessages(nil)` is called, injection state flags are NOT reset:
  - `userContextInjected`, `memoryIndexInjected`, `cavemanInjected`
- **Design Pattern Violation**: SetUserContext (line 300), SetMemoryIndex (line 307), SetCavemanMsg (line 314) all reset their flags. SetMessages should too.
- **Impact**: First turn after clear skips re-injecting preambles, causing inconsistent state.
- **Fix Applied**: Modified `SetMessages()` to reset flags when `len(msgs) == 0`.

### Finding 2: Race Condition in RunWithBlocks
- **Location**: `internal/query/engine.go:392-450` 
- **Issue**: RunWithBlocks reads/writes `e.messages` WITHOUT mutex, while SetMessages holds `e.mu`. This is a classic data race.
- **Race Timeline**:
  1. readLoop goroutine receives EventClearHistory
  2. onClearHistory callback: `engine.SetMessages(nil)` acquires lock, clears messages
  3. Main loop calls RunWithBlocks()
  4. **RunWithBlocks reads e.messages WITHOUT lock** → RACE!
- **Consequence**: Corrupted slice state, lost messages, malformed API calls.
- **Fix Applied**: Protected message initialization block with `e.mu.Lock() / e.mu.Unlock()`.

### Finding 3: Root Cause
The combination of:
1. Race condition (unprotected read in RunWithBlocks)
2. Injection flags not reset (inconsistent state)
3. Both occurring when EventClearHistory arrives mid-turn

Results in the first user message after `/clear` producing no response.

## Symbol Map
| Symbol | File | Role |
|--------|------|------|
| `SetMessages` | `internal/query/engine.go:274` | Replaces messages (FIXED: now resets injection flags) |
| `RunWithBlocks` | `internal/query/engine.go:392` | Executes turn, injects preambles (FIXED: mutex-protected) |
| `onClearHistory` callback | `internal/cli/root.go:499-501` | Calls appInstance.ClearHistory() + engine.SetMessages(nil) |
| `Messages()` | `internal/query/engine.go:264` | RLock-protected read |

## Fixes Applied

### Fix 1: Reset Injection Flags in SetMessages
**Commit**: 84b78bd
```go
if len(msgs) == 0 {
    e.userContextInjected = false
    e.memoryIndexInjected = false
    e.cavemanInjected = false
}
```
Follows pattern of SetUserContext/SetMemoryIndex/SetCavemanMsg. Ensures preambles re-inject after clear.

### Fix 2: Protect RunWithBlocks with Mutex
**Commit**: f53bae1
Added `e.mu.Lock()` / `e.mu.Unlock()` around message initialization block. Eliminates race with SetMessages.

## Verification
Test: User `/clear` → send first message → verify AI response generated and shown.
