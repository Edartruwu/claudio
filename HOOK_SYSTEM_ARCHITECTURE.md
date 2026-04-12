# Claude-Mem Hook System Architecture - Complete Map

## 1. ALL HOOK TYPES

| Event Type | Claude Code Hook | Cursor Hook | Handler File | Purpose |
|---|---|---|---|---|
| **context** | SessionStart | N/A | context.ts | Inject project context from past sessions |
| **session-init** | UserPromptSubmit | beforeSubmitPrompt | session-init.ts | Initialize session DB + start SDK agent |
| **observation** | PostToolUse | afterMCPExecution / afterShellExecution | observation.ts | Capture & store tool usage observations |
| **file-context** | PreToolUse (Read) | N/A | file-context.ts | Inject file edit history when reading |
| **file-edit** | N/A | afterFileEdit | file-edit.ts | Capture file edit observations |
| **user-message** | SessionStart (parallel) | N/A | user-message.ts | Display context to user via stderr |
| **summarize** | Stop | stop | summarize.ts | Queue summary + poll until complete |
| **session-complete** | SessionEnd | N/A | session-complete.ts | Cleanup session from active map |

## 2. DIRECTORY STRUCTURE

```
src/
├── cli/
│   ├── hook-command.ts                    # Main entry point (hookCommand function)
│   ├── types.ts                           # NormalizedHookInput, HookResult, EventHandler
│   ├── handlers/
│   │   ├── index.ts                       # EventHandler factory (getEventHandler)
│   │   ├── context.ts                     # SessionStart - inject context
│   │   ├── session-init.ts                # UserPromptSubmit - init session
│   │   ├── observation.ts                 # PostToolUse - capture observations
│   │   ├── file-context.ts                # PreToolUse for Read - file timeline
│   │   ├── file-edit.ts                   # Cursor afterFileEdit - capture edits
│   │   ├── user-message.ts                # SessionStart - display to user
│   │   ├── summarize.ts                   # Stop - queue & poll summary
│   │   └── session-complete.ts            # SessionEnd - cleanup
│   └── adapters/
│       ├── index.ts                       # Platform adapter factory (getPlatformAdapter)
│       ├── claude-code.ts                 # Session_id → sessionId, tool_name → toolName
│       ├── cursor.ts                      # Conversation_id → sessionId, workspace_roots[0] → cwd
│       ├── gemini-cli.ts                  # Gemini format mapping
│       ├── windsurf.ts                    # Windsurf format mapping
│       └── raw.ts                         # Generic/fallback adapter
├── shared/
│   └── hook-constants.ts                  # HOOK_TIMEOUTS, HOOK_EXIT_CODES, getTimeout()
└── hooks/
    └── hook-response.ts                   # STANDARD_HOOK_RESPONSE constant

plugin/hooks/
└── hooks.json                             # Claude Code hook configuration

cursor-hooks/
└── hooks.json                             # Cursor hook configuration
```

## 3. HOOK LIFECYCLE (CLAUDE CODE)

```
SessionStart (triggered: startup|clear|compact)
  └─→ Setup pre-hook: smart-install.js + worker start (300s timeout)
  └─→ Context Hook: context handler (60s timeout)
       └─→ GET /api/context/inject
       └─→ Returns additionalContext
  └─→ User-Message: user-message handler parallel (60s timeout)
       └─→ Queries /api/context/inject with colors
       └─→ Writes formatted output to stderr

UserPromptSubmit (triggered: every user prompt)
  └─→ session-init handler (60s timeout)
       └─→ POST /api/sessions/init
       └─→ Starts SDK agent (Claude Code only, not Cursor)
       └─→ Runs semantic injection if CLAUDE_MEM_SEMANTIC_INJECT=true
       └─→ Queries /api/context/semantic if enabled

PreToolUse (triggered: before Read tool)
  └─→ file-context handler (2000s timeout)
       └─→ GET /api/observations/by-file?path=...
       └─→ Returns observation timeline (deduped, 1 per session)
       └─→ Gates on file size (skip if < 1500 bytes)

PostToolUse (triggered: after any tool execution)
  └─→ observation handler (120s timeout)
       └─→ POST /api/sessions/observations
       └─→ Captures tool_name, tool_input, tool_response

Stop (triggered: user closes session)
  └─→ summarize handler (120s timeout) [PHASE 1]
       └─→ 1. Extract last assistant message from transcript
       └─→ 2. POST /api/sessions/summarize (queue request)
       └─→ 3. Poll GET /api/sessions/status until queueLength=0 (110s max)
       └─→ 4. POST /api/sessions/complete (cleanup)
  └─→ session-complete handler (30s timeout) [PHASE 2 - backup]
       └─→ POST /api/sessions/complete (removes from active sessions map)

SessionEnd (triggered: after Stop)
  └─→ session-complete handler (30s timeout)
       └─→ POST /api/sessions/complete (safety net backup)
```

## 4. HOOK LIFECYCLE (CURSOR)

```
beforeSubmitPrompt (triggered: user submits prompt)
  ├─→ session-init.sh
  │   └─→ worker-service.cjs hook cursor session-init
  └─→ context-inject.sh
      └─→ worker-service.cjs hook cursor context

afterMCPExecution (triggered: MCP tools)
  └─→ save-observation.sh
      └─→ worker-service.cjs hook cursor observation

afterShellExecution (triggered: shell commands)
  └─→ save-observation.sh
      └─→ worker-service.cjs hook cursor observation

afterFileEdit (triggered: file changes)
  └─→ save-file-edit.sh
      └─→ worker-service.cjs hook cursor file-edit

stop (triggered: close session)
  └─→ session-summary.sh
      └─→ worker-service.cjs hook cursor summarize

NOTE: Cursor doesn't provide transcripts, so summarize quality is reduced
NOTE: Cursor hooks.json defined in cursor-hooks/hooks.json for integration
```

## 5. HOOK TRIGGERING POINTS - DETAILED

### Claude Code: plugin/hooks/hooks.json

| # | Event | Matcher | Command | Handler(s) | Timeout |
|---|-------|---------|---------|-----------|---------|
| 1 | Setup | "*" | scripts/setup.sh | (setup only) | 300s |
| 2 | SessionStart | "startup\|clear\|compact" | smart-install.js + worker start + context hook | context, user-message | 300s, 60s, 60s |
| 3 | UserPromptSubmit | (none) | worker-service.cjs hook claude-code session-init | session-init | 60s |
| 4 | PostToolUse | "*" | worker-service.cjs hook claude-code observation | observation | 120s |
| 5 | PreToolUse | "Read" | worker-service.cjs hook claude-code file-context | file-context | 2000s |
| 6 | Stop | (none) | worker-service.cjs hook claude-code summarize | summarize | 120s |
| 7 | SessionEnd | (none) | worker-service.cjs hook claude-code session-complete | session-complete | 30s |

### Cursor: cursor-hooks/hooks.json

| Event | Command | Handler |
|-------|---------|---------|
| beforeSubmitPrompt | session-init.sh | session-init |
| beforeSubmitPrompt | context-inject.sh | context |
| afterMCPExecution | save-observation.sh | observation |
| afterShellExecution | save-observation.sh | observation |
| afterFileEdit | save-file-edit.sh | file-edit |
| stop | session-summary.sh | summarize |

## 6. HOOK COMMAND ENTRY POINT

**File:** `src/cli/hook-command.ts`

**Function:** `hookCommand(platform: string, event: string, options?: HookCommandOptions): Promise<number>`

**Flow:**
1. Get platform adapter via `getPlatformAdapter(platform)`
2. Get event handler via `getEventHandler(event)`
3. Read JSON from stdin via `readJsonFromStdin()`
4. Normalize input via `adapter.normalizeInput(rawInput)`
5. Execute handler via `handler.execute(input)`
6. Format output via `adapter.formatOutput(result)`
7. Write JSON to stdout via `console.log(JSON.stringify(output))`
8. Exit with exit code

**Exit Code Logic:**
- `isWorkerUnavailableError(error)` classifies errors:
  - Transport failures, timeouts, HTTP 5xx → exit 0 (graceful)
  - HTTP 4xx, TypeError, ReferenceError → exit 2 (blocking)
  - All other unknown errors → exit 0 (conservative)

## 7. HANDLER PATTERN

Each handler implements `EventHandler` interface:

```typescript
interface EventHandler {
  execute(input: NormalizedHookInput): Promise<HookResult>;
}

interface NormalizedHookInput {
  sessionId: string;
  cwd: string;
  platform?: string;
  prompt?: string;
  toolName?: string;
  toolInput?: unknown;
  toolResponse?: unknown;
  transcriptPath?: string;
  filePath?: string;       // Cursor-specific
  edits?: unknown[];       // Cursor-specific
  metadata?: Record<string, unknown>;
}

interface HookResult {
  continue?: boolean;
  suppressOutput?: boolean;
  hookSpecificOutput?: {
    hookEventName: string;
    additionalContext: string;
    systemMessage?: string;
    permissionDecision?: 'allow' | 'deny';
    updatedInput?: Record<string, unknown>;
  };
  exitCode?: number;
}
```

**Common Handler Pattern:**
```typescript
async execute(input: NormalizedHookInput): Promise<HookResult> {
  // 1. Ensure worker is running
  const workerReady = await ensureWorkerRunning();
  if (!workerReady) {
    return { continue: true, suppressOutput: true, exitCode: HOOK_EXIT_CODES.SUCCESS };
  }

  // 2. Validate inputs
  if (!input.sessionId || !input.cwd) {
    throw new Error('Missing required fields');
  }

  // 3. Check if project is excluded
  const settings = SettingsDefaultsManager.loadFromFile(USER_SETTINGS_PATH);
  if (isProjectExcluded(input.cwd, settings.CLAUDE_MEM_EXCLUDED_PROJECTS)) {
    return { continue: true, suppressOutput: true };
  }

  // 4. Call worker API
  const response = await workerHttpRequest('/api/sessions/...', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ ... })
  });

  if (!response.ok) {
    logger.warn('HOOK', 'API call failed, skipping gracefully', { status: response.status });
    return { continue: true, suppressOutput: true, exitCode: HOOK_EXIT_CODES.SUCCESS };
  }

  // 5. Return result with context if needed
  return { continue: true, suppressOutput: true, hookSpecificOutput: { ... } };
}
```

## 8. HANDLER RESPONSIBILITIES

### context.ts (SessionStart)
- Loads settings from USER_SETTINGS_PATH
- Queries worker via GET `/api/context/inject?projects=...&platformSource=...`
- Returns `additionalContext` for Claude to inject
- Also returns `systemMessage` if `CLAUDE_MEM_CONTEXT_SHOW_TERMINAL_OUTPUT=true`

### session-init.ts (UserPromptSubmit)
- Validates sessionId provided (skip Codex CLI / unknown platforms)
- Checks project not excluded
- POSTs to `/api/sessions/init` with contentSessionId, project, prompt
- Initializes SDK agent if Claude Code (not Cursor)
- Runs semantic injection if `CLAUDE_MEM_SEMANTIC_INJECT=true`
- Queries `/api/context/semantic` for relevant past observations
- Returns `additionalContext` from semantic search if available

### observation.ts (PostToolUse)
- Validates sessionId, cwd provided
- Checks project not excluded
- Extracts tool_name, tool_input, tool_response
- POSTs to `/api/sessions/observations`
- Gracefully degrades if worker unavailable

### file-context.ts (PreToolUse for Read)
- Gates on file size (skip if < 1500 bytes)
- Validates cwd provided
- Queries `/api/observations/by-file` with relative path
- Deduplicates observations (1 per session, ranked by specificity)
- Returns timeline formatted as observation history
- Allows read to proceed with limit=1 (token efficient)

### file-edit.ts (Cursor afterFileEdit)
- Captures filePath and edits from Cursor event
- Validates filePath provided
- Sends as `write_file` observation to `/api/sessions/observations`
- Preserves file-specific metadata

### user-message.ts (SessionStart, parallel)
- Runs in parallel to context handler
- Queries `/api/context/inject` with colors param for Cursor/Claude Code
- Writes formatted timeline to stderr for user visibility
- Displays link to live dashboard (http://localhost:{port}/)
- Always exits with 0 (non-critical, never blocks)

### summarize.ts (Stop, Phase 1)
- Validates transcriptPath provided
- Extracts last assistant message from transcript
- POSTs to `/api/sessions/summarize` to queue work
- Polls `/api/sessions/status` until `queueLength=0`
- Waits up to 110s within 120s timeout
- Then POSTs to `/api/sessions/complete` to clean up session

### session-complete.ts (SessionEnd, Phase 2 - Backup)
- POSTs to `/api/sessions/complete` via contentSessionId
- Removes session from active sessions map
- Safety net for summarize handler cleanup
- Fixes Issue #842 (orphan reaper cleanup)

## 9. INPUT NORMALIZATION (Adapters)

**Pattern:** Each platform sends different field names. Adapters normalize to `NormalizedHookInput`.

### claude-code.ts
- `session_id` → `sessionId`
- `cwd` → `cwd`
- `tool_name` → `toolName`
- `tool_input` → `toolInput`
- `tool_response` → `toolResponse`
- `transcript_path` → `transcriptPath`

### cursor.ts
- `conversation_id` / `generation_id` / `id` → `sessionId`
- `workspace_roots[0]` / `cwd` → `cwd`
- `prompt` / `query` / `input` / `message` → `prompt`
- `tool_name` → `toolName` (or detect "Bash" from command field)
- `result_json` → `toolResponse`
- `file_path` → `filePath`
- `edits` → `edits`
- Shell commands: detected from `command` field, mapped to toolName="Bash"

### gemini-cli.ts
- Handles Gemini-specific field naming variations

### raw.ts
- Fallback adapter (accepts both camelCase and snake_case)
- Used for Codex CLI and unknown platforms

### windsurf.ts
- Maps Windsurf-specific field names

## 10. OUTPUT FORMATTING (Adapters)

### claude-code.ts formatOutput()
```typescript
// Only emit Claude Code hook contract fields
// Unrecognized fields cause "JSON validation failed"
{
  hookSpecificOutput?: {
    hookEventName: string;
    additionalContext: string;
    permissionDecision?: 'allow' | 'deny';
  },
  systemMessage?: string
}
```

### cursor.ts formatOutput()
```typescript
// Cursor expects simpler response
{ continue: boolean }
```

## 11. EXIT CODES & ERROR HANDLING

| Code | Meaning | Classification | Examples |
|------|---------|-----------------|----------|
| 0 | SUCCESS | Graceful degradation | Worker unavailable, timeout, HTTP 5xx, transport errors |
| 1 | FAILURE | Generic error | Generic errors |
| 2 | BLOCKING_ERROR | Handler/client bug | HTTP 4xx, TypeError, ReferenceError, SyntaxError |
| 3 | USER_MESSAGE_ONLY | Cursor custom | (Used by Cursor handlers) |

**Error Classification Logic (isWorkerUnavailableError):**
- ✅ Transport failures: ECONNREFUSED, ECONNRESET, EPIPE, ETIMEDOUT, ENOTFOUND, socket hang up → exit 0
- ✅ Timeout errors: "timed out", "timeout" → exit 0
- ✅ HTTP 5xx status codes → exit 0
- ✅ HTTP 429 (rate limit) → exit 0
- ❌ HTTP 4xx client errors → exit 2 (our bug)
- ❌ Programming errors: TypeError, ReferenceError, SyntaxError → exit 2

**Result when worker unavailable:**
```typescript
{ continue: true, suppressOutput: true, exitCode: HOOK_EXIT_CODES.SUCCESS }
```

## 12. HOOK CONSTANTS

**File:** `src/shared/hook-constants.ts`

### HOOK_TIMEOUTS
```typescript
DEFAULT: 300000,            // 5 min (standard HTTP timeout)
HEALTH_CHECK: 3000,         // 3s (worker health check)
POST_SPAWN_WAIT: 15000,     // 15s (wait after daemon spawn)
READINESS_WAIT: 30000,      // 30s (wait for DB + search init)
PORT_IN_USE_WAIT: 3000,     // 3s (wait when port occupied)
WORKER_STARTUP_WAIT: 1000,  // 1s
PRE_RESTART_SETTLE_DELAY: 2000,  // 2s (file sync before restart)
POWERSHELL_COMMAND: 10000,  // 10s (PowerShell process enumeration)
WINDOWS_MULTIPLIER: 1.5     // Platform-specific adjustment
```

### HOOK_EXIT_CODES
```typescript
SUCCESS: 0,              // Graceful degradation
FAILURE: 1,              // Generic error
BLOCKING_ERROR: 2,       // Handler/client bug
USER_MESSAGE_ONLY: 3     // Cursor user-message handler
```

### getTimeout(baseTimeout: number): number
Multiplies by `WINDOWS_MULTIPLIER (1.5)` on Windows platform, returns unchanged on others.

## 13. KEY ARCHITECTURAL PATTERNS

### 1. Worker Separation
- All hooks communicate via HTTP to worker daemon (`worker-service.cjs`)
- Worker handles DB operations, embeddings, summarization
- Enables graceful degradation if worker unavailable
- Hooks never block main session if worker fails

### 2. Graceful Degradation
- Every handler calls `ensureWorkerRunning()` first
- If worker unavailable: return empty/default result
- Errors classified as worker-unavailable exit with 0 (don't block user)

### 3. Multi-Point Context Injection
- **SessionStart**: General project context (recent sessions & observations)
- **UserPromptSubmit**: Semantic context (Chroma similarity search)
- **PreToolUse (Read)**: File-specific observation timeline

### 4. Two-Phase Cleanup (Stop Hook Critical)
- **Phase 1 (Stop/summarize)**: Queue work, poll until done (120s timeout)
  - Extracts transcript, queues summarize, polls /api/sessions/status
  - Calls /api/sessions/complete to cleanup
- **Phase 2 (SessionEnd/session-complete)**: Backup cleanup (30s timeout)
  - Safety net if Stop hook doesn't run
  - Removes session from active sessions map (fixes orphan reaper issue #842)

### 5. Platform Abstraction (Adapter Pattern)
- Shields handlers from platform-specific quirks
- Handlers only know `NormalizedHookInput`
- Each platform adapter handles its own format mapping
- New platforms added by creating adapter + registering in `getPlatformAdapter()`

### 6. Progressive Disclosure (File Context)
- Returns observation timeline instead of full file content
- Allows read with limit=1 (token efficient)
- Provides Claude full context without payload size bloat
- Gates on file size (skip if < 1500 bytes to avoid overhead)

### 7. Semantic Injection (Session-Init)
- Queries Chroma embedding database for related observations
- Searches past observations for semantic similarity to current prompt
- Injected on every prompt (except Cursor which lacks transcript)
- Controlled by `CLAUDE_MEM_SEMANTIC_INJECT` setting (default: true)
- Limit configurable via `CLAUDE_MEM_SEMANTIC_INJECT_LIMIT` (default: 5)

### 8. Project Exclusion (Per-Handler)
- Every observation/context handler checks `isProjectExcluded()`
- Prevents tracking of user-excluded projects
- Controlled by `CLAUDE_MEM_EXCLUDED_PROJECTS` setting (comma-separated paths)

## 14. COMPLETE HOOK FLOW DIAGRAM

```
Claude Code / Cursor IDE Session
    ↓
    └→ Detects Lifecycle Event
       (SessionStart, UserPromptSubmit, PostToolUse, Stop)
    ↓
    └→ Executes Hook Script (Bash)
       Command: worker-service.cjs hook <platform> <event>
       Passes JSON stdin with event metadata
    ↓
    └→ hookCommand() Entry Point (hook-command.ts)
       1. Suppress stderr (hook output is JSON only)
       2. Get platform adapter (claudeCode, cursor, gemini, etc.)
       3. Get event handler (context, session-init, observation, etc.)
       4. Read JSON from stdin
       5. Normalize via adapter.normalizeInput()
    ↓
    └→ Handler.execute(NormalizedHookInput)
       1. Check worker running (ensureWorkerRunning)
       2. Validate required fields
       3. Check project not excluded
       4. POST/GET to worker API endpoints
       5. Return HookResult { continue, suppressOutput, hookSpecificOutput }
    ↓
    └→ Format Output
       1. Adapter formats output for platform
       2. Write JSON to stdout via console.log()
       3. Restore stderr
    ↓
    └→ Exit Process
       1. Determine exit code (0 or 2)
       2. process.exit(exitCode)
    ↓
    └→ IDE Processes Hook Output
       - Injects additionalContext into session
       - Updates tool input if permissionDecision
       - Continues or stops based on continue flag
       - Observations stored in worker DB
       - Context injected on next SessionStart
```

## 15. FILES TO EXAMINE FOR EACH TASK

| Goal | Key Files | Notes |
|------|-----------|-------|
| Understand hook entry | `hook-command.ts`, `handlers/index.ts` | Main dispatcher |
| Add new hook type | `handlers/[new-handler].ts`, `handlers/index.ts`, adapters/ | Implement EventHandler interface |
| Change platform adapter | `adapters/[platform].ts`, `types.ts` | Update normalizeInput/formatOutput |
| Modify context injection | `handlers/context.ts`, `handlers/session-init.ts` | Query /api/context/* endpoints |
| Fix observation capture | `handlers/observation.ts` | POST /api/sessions/observations |
| Debug hook lifecycle | `plugin/hooks/hooks.json`, `cursor-hooks/hooks.json` | Hook triggering configuration |
| Add graceful degradation | `handlers/[handler].ts` | Use ensureWorkerRunning() pattern |
| Change exit codes | `hook-command.ts` (isWorkerUnavailableError), `hook-constants.ts` | Error classification logic |
| Understand platform differences | `adapters/claude-code.ts`, `adapters/cursor.ts` | Compare normalizeInput mappings |
| Fix two-phase cleanup | `handlers/summarize.ts`, `handlers/session-complete.ts` | Stop + SessionEnd coordination |

---

**Source Repository:** /Users/abraxas/Personal/claude-mem/  
**Last Updated:** 2025-01-13  
**Architecture Version:** 2.1 (Current with PreToolUse + file-context)
