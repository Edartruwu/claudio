# Investigation Report: Team Agent Result/Completion Reporting Flow

## Subject
Investigation of the complete result/completion reporting flow for team agents, with focus on system prompt injection patterns, result format specifications, and potential conflicts between `### Done` (from runner.go) and `### Result` (from spawnteammate.go).

## Codebase Overview

The team agent system consists of:
- **`internal/teams/runner.go`**: Core TeammateRunner that spawns and manages team agents
- **`internal/tools/spawnteammate.go`**: SpawnTeammate tool (explicit team agent spawner with result format instruction)
- **`internal/tools/agent.go`**: Generic Agent tool (spawns sub-agents, routes to TeammateRunner when team is active)
- **`internal/tui/messages.go`**: UI layer that extracts result summaries from agent output
- **`internal/tui/root.go`**: TUI notification handler that displays completion events to user

The flow involves two distinct spawning paths:
1. **Direct team spawn**: SpawnTeammate tool → TeammateRunner.Spawn()
2. **Generic agent spawn with team**: Agent tool → TeamRunner.Spawn() (when team is active)

## Key Findings

### Finding 1: TeammateRunner Injects `### Done` Header (runner.go:515-571)

**Location**: `internal/teams/runner.go:515-571`

**Description**: The `runTeammate()` function builds a `teammateCtx` string that is **always** injected into every spawned teammate's system prompt (lines 515-571). This context includes a "Completion report" section that specifies the `### Done` header format.

**Full `### Done` instruction**:
```
## Completion report

When your task is done, always end your final response with this section:

### Done
- What was changed or produced, and why
- Test / build / validation results (paste actual output, not summaries)
- Attempts made if anything failed during the process
- Risks, deferred decisions, or anything the team lead should know
```

**Construction**: Built in `fmt.Sprintf()` at line 515, appended to `System` prompt at line 579 (if a System was provided) or used as the entire system prompt at line 581.

**Key line (579)**: `system = cfg.System + "\n\n" + teammateCtx`
- This means `### Done` is ALWAYS present in the final system prompt for ALL teammates spawned via TeammateRunner

### Finding 2: SpawnTeammate Injects `### Result` Header (spawnteammate.go:14-24 and line 278)

**Location**: `internal/tools/spawnteammate.go:14-24` (constant definition) and line 278 (appended to system prompt)

**Description**: SpawnTeammate tool defines a separate `teammateResultInstruction` constant that specifies `### Result` format:

```go
const teammateResultInstruction = `

## Result format (IMPORTANT)
When you finish, your LAST message must be a concise summary of ≤15 lines structured exactly as:

### Result
- **What was done**: one-line summary of the outcome
- **Files changed**: list changed files (or "none")
- **Issues / blockers**: anything unexpected, or "none"

This section is extracted as the notification shown to the orchestrator. Keep it tight — no narration of tool calls, no repeating the task description.`
```

**Appended at line 278**:
```go
agentDef.SystemPrompt += teammateResultInstruction
```

This is appended BEFORE calling `t.Runner.Spawn()` at line 284.

### Finding 3: CONFLICT CONFIRMED - Both Headers Are Injected into SpawnTeammate Agents

**Location**: Interaction between spawnteammate.go:278 and runner.go:579

**Critical Finding**: When SpawnTeammate spawns an agent:
1. Line 278 (spawnteammate.go): `### Result` instruction is appended to `agentDef.SystemPrompt`
2. Line 288 passes this augmented SystemPrompt to `SpawnConfig.System`
3. Line 579 (runner.go): The SpawnConfig.System is prepended to `teammateCtx`, which contains `### Done`

**Final system prompt for SpawnTeammate agents**:
```
[agent_definition_base_system_prompt]
+
[### Result format instruction] (from spawnteammate.go:14-24, appended at line 278)
+
"\n\n"
+
[### Done format instruction] (from runner.go:515-571, added at line 579)
```

**EVIDENCE**: 
- SpawnTeammate.Execute, line 278: `agentDef.SystemPrompt += teammateResultInstruction`
- SpawnTeammate.Execute, line 288: passes modified SystemPrompt to `SpawnConfig{System: agentDef.SystemPrompt}`
- TeammateRunner.runTeammate, line 578-579: `if cfg.System != "" { system = cfg.System + "\n\n" + teammateCtx }`
- Runner always appends `teammateCtx` which contains the full `### Done` block

**This is a direct, factual conflict**: Both `### Result` and `### Done` are present in the same system prompt.

### Finding 4: Agent Tool Routes Through TeammateRunner WITHOUT `### Result` (agent.go:375-388)

**Location**: `internal/tools/agent.go:375-388`

**Description**: When the Agent tool is called and a team is active:
```go
if t.TeamRunner != nil && t.TeamRunner.ActiveTeamName() != "" {
    // ... team setup ...
    state, err := t.TeamRunner.Spawn(teams.SpawnConfig{
        TeamName:   teamName,
        AgentName:  shortName,
        Prompt:     in.Prompt,
        System:     agentDef.SystemPrompt,  // <-- no modification here
        Model:      modelOverride,
        MaxTurns:   maxTurns,
        MemoryDir:  agentDef.MemoryDir,
        Foreground: !in.RunInBackground,
        TaskIDs:    in.TaskIDs,
    })
```

**Critical observation**: The Agent tool passes `agentDef.SystemPrompt` **unmodified** to SpawnConfig.System. It does NOT add the `### Result` instruction like SpawnTeammate does.

**Therefore**: Agent tool agents via TeammateRunner receive ONLY `### Done` (injected by runner.go), NOT `### Result`.

### Finding 5: summaryFromResult Searches Only for `### Done` (messages.go:1180-1185)

**Location**: `internal/tui/messages.go:1180-1185`

**Description**: The extraction function looks ONLY for `### Done`:
```go
func summaryFromResult(text string, maxLines int, maxChars int) string {
    if idx := strings.LastIndex(text, "### Done"); idx != -1 {
        return text[idx:]
    }
    return lastNLines(text, maxLines, maxChars)
}
```

**Behavior**:
- Searches for the last occurrence of `### Done` in agent output
- If found, returns everything from that marker onward
- If NOT found, falls back to extracting last N lines (maxLines, maxChars)

**Important**: This function does NOT recognize or extract `### Result`. If an agent outputs `### Result` but NOT `### Done`, the fallback logic applies (last N lines).

### Finding 6: summaryFromResult Used in Notifications (root.go:2369, 2371, 4307, 4309)

**Location**: `internal/tui/root.go:2369, 2371, 4307, 4309`

**Description**: summaryFromResult is called in two places within team completion event handlers:

1. **Main event handler (lines 2360-2380)**:
   - On AgentCompleted event (line 2369): `summaryFromResult(ev.Text, 15, 600)`
   - On AgentFailed event (line 2371): `summaryFromResult(ev.Text, 15, 600)`
   - Extracts summary with max 15 lines, 600 chars

2. **Secondary event handler (lines 4300-4312)**:
   - On TeammateCompleted event (line 4307): `summaryFromResult(ev.teammateEvent.Text, 15, 600)`
   - On TeammateCompletedFailed event (line 4309): `summaryFromResult(ev.teammateEvent.Text, 15, 600)`

**Usage context**: The extracted summary is embedded in a task notification shown to the orchestrator (user):
```
<task-notification>
Agent '<name>' in team '<team>' completed.
Result summary: [extracted by summaryFromResult]
```

**Purpose**: The summary is displayed to the user as part of the completion notification. It goes to the orchestrator notification system.

### Finding 7: No Other Files Reference `### Result` (grep confirms)

**Search results**: Only `internal/tools/spawnteammate.go` contains `### Result` in the codebase. No other Go files reference, search for, or extract this marker.

## Symbol Map

| Symbol | File | Role |
|--------|------|------|
| `summaryFromResult` | `internal/tui/messages.go:1180` | Extracts `### Done` section from agent output; fallback to last N lines |
| `teammateResultInstruction` | `internal/tools/spawnteammate.go:14` | Constant defining `### Result` format for SpawnTeammate |
| `teammate_ctx` (local var) | `internal/teams/runner.go:515` | Built system prompt fragment containing `### Done` instruction |
| `SpawnTeammateTool.Execute` | `internal/tools/spawnteammate.go:205` | Appends `### Result` to system prompt before calling Runner.Spawn |
| `TeammateRunner.Spawn` | `internal/teams/runner.go:341` | Entry point that delegates to `runTeammate()` |
| `runTeammate` | `internal/teams/runner.go:442` | Core function that injects `### Done` and calls agent executor |
| `AgentTool.Execute` | `internal/tools/agent.go:319` | Routes to TeammateRunner if team active (lines 375-388) |

## Dependencies & Data Flow

### Path 1: SpawnTeammate → Full Dual-Instruction Conflict
```
SpawnTeammate.Execute()
  ├─ Line 278: agentDef.SystemPrompt += "### Result format..."
  └─ Line 284: t.Runner.Spawn(SpawnConfig{System: agentDef.SystemPrompt})
       └─ runner.go:442: runTeammate()
            ├─ Line 515-571: Build teammates_ctx with "### Done format..."
            ├─ Line 579: system = cfg.System + "\n\n" + teammates_ctx
            │            (System from SpawnConfig contains ### Result)
            │            (teammates_ctx contains ### Done)
            └─ Line 624/626: Execute agent with combined system prompt

Agent output:
  └─ root.go:2369/2371: summaryFromResult(output) → searches for "### Done"
```

**Result**: SpawnTeammate agents receive BOTH `### Result` AND `### Done` instructions. The TUI looks only for `### Done`.

### Path 2: Agent Tool with Active Team → Single `### Done` Instruction
```
Agent.Execute(team_is_active)
  ├─ Line 376-377: Get team name, create slug name
  └─ Line 378: t.TeamRunner.Spawn(SpawnConfig{System: agentDef.SystemPrompt})
       └─ runner.go:442: runTeammate()
            ├─ Line 515-571: Build teammates_ctx with "### Done format..."
            ├─ Line 578-579: system = cfg.System + "\n\n" + teammates_ctx
            │                (System is unmodified agentDef.SystemPrompt, NO ### Result)
            │                (teammates_ctx contains ### Done)
            └─ Line 624/626: Execute agent with combined system prompt

Agent output:
  └─ root.go:4307/4309: summaryFromResult(output) → searches for "### Done"
```

**Result**: Agent tool agents receive only `### Done` instruction. No conflict.

### Path 3: Agent Tool with No Active Team → No Team Instructions
```
Agent.Execute(team_NOT_active)
  ├─ Line 419-438 or 441-462: Execute via RunAgent/RunAgentWithMemory
  └─ No team system prompt augmentation; uses only agentDef.SystemPrompt

Agent output:
  └─ Not team-managed; notification flow does not apply
```

**Result**: Non-team agents do not receive `### Done` or `### Result` instructions.

## Risks & Observations

### 1. **CRITICAL: Dual Conflicting Instructions in SpawnTeammate**
   - **Risk**: SpawnTeammate agents receive BOTH `### Result` (line 278) and `### Done` (line 579 via runner.go:515-571)
   - **Symptom**: Agent may be unsure which format to use; unclear which is authoritative
   - **Impact**: User/orchestrator receives notification based on `### Done`, but agent may produce `### Result` as primary format
   - **Status**: CONFIRMED — both are injected into the same system prompt

### 2. **Asymmetric Instructions Between SpawnTeammate and Agent Tool**
   - **Risk**: SpawnTeammate agents get `### Result`, but Agent tool agents get only `### Done`
   - **Expected behavior**: Both paths through TeammateRunner should have consistent instructions
   - **Observation**: SpawnTeammate appears to be the "explicit" path with result format emphasis, while Agent tool agents are treated as "generic" sub-tasks
   - **Inconsistency severity**: MEDIUM — both work, but semantics differ

### 3. **summaryFromResult Fallback Silently Ignores `### Result`**
   - **Risk**: If SpawnTeammate agent outputs ONLY `### Result` (and no `### Done`), the fallback extracts last N lines
   - **Expected behavior**: Either extract `### Result` as fallback, OR ensure `### Done` is always present
   - **Status**: POTENTIAL — code does not account for `### Result` header at all

### 4. **No End-to-End Test Verification**
   - **Risk**: The two result format specs were added separately without integration verification
   - **Evidence**: `teammates_ctx` built in runner.go (team feature) pre-dates SpawnTeammate tool; SpawnTeammate added its own instruction without coordinating removal of conflict
   - **Observation**: SpawnTeammate docs (line 88-137) make no mention of `### Done` format, only of agent spawning semantics

### 5. **Possible Instruction Precedence Issue**
   - **Risk**: When both `### Result` and `### Done` are in system prompt, which takes precedence?
   - **Observation**: `### Result` is appended at line 278 (in SpawnTeammate.Execute), then `### Done` comes in later (line 579 in runTeammate)
   - **Question**: Do LLMs prefer the last instruction or first? Undefined.

## Open Questions

1. **Intentional dual-instruction design?**
   - Was the `### Result` format intentionally designed to coexist with `### Done` in SpawnTeammate agents?
   - Or is `### Result` meant to replace `### Done` for this tool only?

2. **Why does summaryFromResult only search for `### Done`?**
   - Should it also search for `### Result` as an alternative?
   - Should SpawnTeammate agents NOT produce `### Done` at all?

3. **When was `## Result format` added to SpawnTeammate?**
   - Was this added after `### Done` was already in runner.go?
   - Was there a migration plan to consolidate?

4. **Is the asymmetry intentional?**
   - Agent tool agents receive only `### Done` (from runner.go)
   - SpawnTeammate agents receive both (from spawnteammate.go + runner.go)
   - Is this the intended design?

5. **Which format is the canonical specification?**
   - Candidate 1: `### Done` in runner.go (included for ALL teammates)
   - Candidate 2: `### Result` in spawnteammate.go (explicit tool documentation)
   - Should one be deprecated?

## Summary

- **`### Done`** (runner.go:565-569) is injected into ALL teammates spawned via TeammateRunner.
- **`### Result`** (spawnteammate.go:14-24) is injected into agents spawned via SpawnTeammate ONLY.
- **SpawnTeammate agents receive BOTH formats**, creating a direct conflict.
- **Agent tool agents receive ONLY `### Done`** when team is active.
- **summaryFromResult searches ONLY for `### Done`**, ignoring `### Result` entirely.
- **No other system searches for or extracts `### Result`** in the codebase.

The design has two separate completion-reporting specifications that were added independently without full integration. SpawnTeammate's `### Result` instruction is ignored by the TUI notification layer, which exclusively uses `### Done`.

---

## Investigation Method

- Searched codebase for all references to `### Done`, `### Result`, `summaryFromResult`, `TeamRunner`, `Spawn`
- Traced system prompt construction through SpawnTeammate and Agent tool execution paths
- Verified integration points where system prompts are combined and used
- Confirmed extraction logic in TUI notification handlers
- Cross-referenced with completion event flow in root.go

All findings are based on static code analysis. Runtime behavior (e.g., which instruction the LLM actually follows) cannot be determined from code inspection alone.
