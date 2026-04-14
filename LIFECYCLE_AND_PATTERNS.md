# Claudio Lifecycle & Interaction Patterns

Detailed documentation of key lifecycle events, interaction sequences, and architectural patterns.

---

## Session Lifecycle

### New Session

```
User: claudio "prompt"
  │
  ├─→ session.Start()
  │   ├─→ DB: INSERT INTO sessions (id, title, created_at)
  │   └─→ Return session.Session with ID
  │
  ├─→ query.Engine.Run(ctx, "prompt")
  │   ├─→ Fire SessionStart hook
  │   │   └─→ Execute ~/.claudio/hooks.json[SessionStart] commands
  │   ├─→ Inject CLAUDE.md as first user message (once)
  │   ├─→ Inject memory index as second user message (once)
  │   └─→ Enter conversation loop
  │
  └─→ Session ends
      ├─→ Fire SessionEnd hook
      └─→ DB: UPDATE sessions SET finished_at WHERE id
```

### Resume Session

```
User: claudio --resume [session_id]
  │
  ├─→ session.Resume(sessionID)
  │   ├─→ DB: SELECT * FROM messages WHERE session_id
  │   └─→ Load full conversation history
  │
  ├─→ query.Engine.SetMessages(historyFromDB)
  ├─→ query.Engine.Run(ctx, newPrompt)
  │   ├─→ Fire SessionStart hook (only once per process)
  │   ├─→ Inject user context/memory (only once per process)
  │   └─→ Continue with new turn appended to history
  │
  └─→ New messages appended to DB
```

---

## Request/Response Sequence (Detailed)

### Single Turn Anatomy

```
1. Setup Phase
   ├─→ Inject user context (CLAUDE.md) if not yet injected
   ├─→ Inject memory index if not yet injected
   ├─→ Poll team mailbox for incoming messages
   └─→ Add incoming team messages to conversation

2. Prompt Preparation
   ├─→ Build full system prompt:
   │   ├─→ Static sections (intro, instructions, etc.)
   │   ├─→ Tool descriptions (APIDefinitionsWithDeferral)
   │   ├─→ Skill content (appended from skill registry)
   │   ├─→ Plugin instructions
   │   └─→ Dynamic sections (git status, current working dir)
   │
   ├─→ Check if prompt caching eligible
   │   └─→ Use frozen system prompt from cache if available
   │
   └─→ Build messages array:
       ├─→ Historical messages (from conversation)
       ├─→ User context message (injected once)
       ├─→ Memory index message (injected once)
       ├─→ Team messages (injected this turn)
       └─→ New user message (from this turn)

3. API Request
   ├─→ Resolve model routing:
   │   ├─→ User specified model? Use it.
   │   ├─→ Agent override? Use it.
   │   └─→ Config default? Use it.
   │
   ├─→ Resolve provider:
   │   ├─→ Model routing rules: pattern → provider
   │   └─→ Register provider with api.Client
   │
   ├─→ Build request:
   │   ├─→ model: (resolved above)
   │   ├─→ messages: (from step 2)
   │   ├─→ system: (from step 2)
   │   ├─→ tools: (APIDefinitionsWithDeferral)
   │   ├─→ max_tokens: 8192 (or escalated 64K on retry)
   │   ├─→ thinking:
   │   │   ├─→ type: "enabled" if config.ThinkingMode == "enabled"
   │   │   └─→ budget_tokens: config.BudgetTokens
   │   ├─→ metadata (prompt caching, effort level)
   │   └─→ Extra headers: User-Agent, X-Request-ID
   │
   ├─→ Configure caching:
   │   ├─→ Mark system prompt as cacheable
   │   └─→ Freeze system prompt for future turns
   │
   └─→ api.Client.CreateMessageStream(request)
       └─→ Send to provider (OpenAI-compat or Anthropic native)

4. Streaming Loop
   ├─→ FOR EACH event in stream:
   │   │
   │   ├─→ content_block_start:
   │   │   └─→ Track block type (text, tool_use)
   │   │
   │   ├─→ content_block_delta:
   │   │   │
   │   │   ├─→ IF text: handler.OnTextDelta(deltaText)
   │   │   │   └─→ UI: render to chat, update line-by-line
   │   │   │
   │   │   ├─→ IF thinking: handler.OnThinkingDelta(deltaText)
   │   │   │   └─→ UI: render to collapsible thinking pane
   │   │   │
   │   │   └─→ IF tool_input: accumulate JSON chunks
   │   │
   │   ├─→ content_block_stop:
   │   │   └─→ Track completed block (text or tool_use)
   │   │
   │   ├─→ message_delta:
   │   │   ├─→ usage: update token tracking
   │   │   └─→ stop_reason: check for max_tokens, end_turn, tool_use
   │   │
   │   └─→ message_stop:
   │       └─→ finalize usage tracking
   │
   └─→ After stream: process stop_reason (see below)

5. Stop Reason Handling

   5a. stop_reason: tool_use
   ├─→ Extract all tool_use blocks from response
   ├─→ Add assistant message to conversation history
   │   └─→ DB: INSERT INTO messages (session_id, role='assistant', content=json)
   │
   ├─→ FOR EACH tool_use:
   │   │
   │   ├─→ Fire PreToolUse hook
   │   │   └─→ Execute ~/.claudio/hooks.json[PreToolUse] commands
   │   │
   │   ├─→ Evaluate permissions:
   │   │   ├─→ permissions.Match(toolName, extractedContent, rules)
   │   │   └─→ behavior: auto, manual, or deny
   │   │
   │   ├─→ IF behavior == deny:
   │   │   ├─→ Log to audit_log
   │   │   └─→ handler.OnToolApprovalNeeded() → return false
   │   │
   │   ├─→ IF behavior == manual:
   │   │   └─→ handler.OnToolApprovalNeeded(toolUse)
   │   │       └─→ UI: show dialog, wait for user response
   │   │
   │   ├─→ IF approved:
   │   │   │
   │   │   ├─→ tools.Execute(toolName, input):
   │   │   │   ├─→ Get tool from registry
   │   │   │   ├─→ Validate input against tool schema
   │   │   │   ├─→ Run tool (Bash, Read, Glob, etc.)
   │   │   │   ├─→ Capture output (on disk if > threshold)
   │   │   │   └─→ Return tools.Result{Output, Error, ExitCode}
   │   │   │
   │   │   ├─→ Fire PostToolUse hook (on success)
   │   │   │   └─→ Execute ~/.claudio/hooks.json[PostToolUse] commands
   │   │   │
   │   │   ├─→ handler.OnToolUseEnd(toolUse, result)
   │   │   │   └─→ UI: mark tool as complete, show result
   │   │   │
   │   │   ├─→ Log to audit_log (success)
   │   │   │
   │   │   ├─→ Add tool result message to conversation:
   │   │   │   └─→ DB: INSERT INTO messages (role='user', content=resultJson)
   │   │   │
   │   │   └─→ CONTINUE LOOP (jump to step 3: make new API request)
   │   │
   │   └─→ IF denied or error:
   │       ├─→ Fire PostToolUseFailure hook
   │       ├─→ Log to audit_log (failure)
   │       ├─→ Add denial/error message to conversation
   │       └─→ CONTINUE LOOP (jump to step 3)
   │
   └─→ After all tools: max_tokens reached?
       └─→ See 5c below

   5b. stop_reason: end_turn
   ├─→ Add final assistant message to conversation history
   │   └─→ DB: INSERT INTO messages (session_id, role='assistant', content=finalJson)
   │
   ├─→ handler.OnTurnComplete(usage)
   │   └─→ Update analytics with token count + cost
   │
   ├─→ Fire OnTurnEnd callback (background memory extraction):
   │   └─→ memory.MemoryExtractor().Extract(messages)
   │       ├─→ Parse facts + concepts from assistant response
   │       ├─→ auto_learning: check if conversation matches learned instincts
   │       └─→ memory.Save(entry) for new facts
   │
   ├─→ Check cost threshold:
   │   └─→ IF session_cost > CostConfirmThreshold:
   │       └─→ handler.OnCostConfirmNeeded(cost, threshold)
   │           └─→ UI: show cost dialog, wait for user response
   │
   ├─→ Check maxTurns:
   │   └─→ IF maxTurns > 0 AND turnCount >= maxTurns:
   │       └─→ EXIT LOOP (stop accepting new turns)
   │
   ├─→ Check team mailbox:
   │   └─→ IF team lead and team members have new messages:
   │       ├─→ Poll mailbox for incoming messages
   │       └─→ If messages, CONTINUE LOOP (new turn with team input)
   │
   └─→ OTHERWISE: EXIT LOOP (session complete)

   5c. stop_reason: max_tokens
   ├─→ handler.OnRetry(toolUses)  [tombstone previous partial tool_use renders]
   │
   ├─→ Escalate max_tokens:
   │   └─→ normal (8K) → escalated (64K)
   │
   ├─→ Add assistant message with partial response to history
   │
   ├─→ CONTINUE LOOP (jump to step 3 with escalated max_tokens)
   │   └─→ Note: escalatedMaxTokens is sticky for remaining turns
   │
   └─→ On success: reset maxTokens to normal for next fresh API call

   5d. stop_reason: max_completion_tokens or other
   ├─→ Log error
   └─→ handler.OnError(err)
       └─→ EXIT LOOP

6. Cleanup
   ├─→ Fire SessionEnd hook
   │   └─→ Execute ~/.claudio/hooks.json[SessionEnd] commands
   │
   └─→ Print cost summary to stderr
```

---

## Tool Execution Lifecycle

### Tool Registry Lookup & Execution

```
1. engine.RunWithBlocks() receives tool_use from LLM
   └─→ toolUse = {name: "Bash", id: "tool_123", input: {command: "ls"}}

2. registry.Execute(toolUse.name, toolUse.input)
   │
   ├─→ Get tool by name
   │   └─→ registry.tools[toolName] = &tools.BashTool{}
   │
   ├─→ Validate input:
   │   ├─→ tool.InputSchema() returns JSON schema
   │   └─→ Validate input against schema
   │
   ├─→ Execute tool:
   │   ├─→ For Bash: exec command with security context applied
   │   ├─→ For Read: read file, apply cache, check path access
   │   ├─→ For Glob: pattern match with cache
   │   ├─→ For Agent: spawn sub-agent, run with sub-agent system prompt
   │   └─→ For Skill: lookup skill, append to system prompt, re-invoke LLM
   │
   ├─→ Capture result:
   │   ├─→ IF output > size_threshold (e.g., 1MB):
   │   │   ├─→ Save to disk: ~/.claudio/task-output/tool_123.log
   │   │   └─→ Return reference: {ref: "tool_123.log", size: bytes}
   │   └─→ ELSE:
   │       └─→ Return full output inline
   │
   └─→ Return tools.Result{Output, Error, ExitCode}
```

### Security Injection

```
At app.New():
├─→ Create SecurityContext from config:
│   ├─→ DenyPaths: ["*.env", "/.aws/"]
│   ├─→ AllowPaths: ["/home/user/project/"]
│   └─→ DenyCommands: ["rm -rf", "dd", ":("]
│
├─→ Get Bash tool from registry
├─→ Inject: bashTool.Security = securityContext
│
└─→ Tool execution applies at runtime:
    ├─→ Bash: securityContext.CheckCommand(cmd)
    └─→ Read/Write/Edit: securityContext.CheckPath(path)
```

---

## Hook Execution Lifecycle

### Hook System

```
~/.claudio/hooks.json:
{
  "hooks": [
    {
      "id": "git-commit",
      "type": "command",
      "event": "PostToolUse",
      "matcher": {"tool": "Write"},
      "command": "git add -A && git commit -m 'auto'",
      "timeout": 5,
      "async": true,
      "description": "Auto-commit after file writes"
    }
  ]
}
```

### Hook Firing Sequence

```
1. In query engine, fire hook:
   │
   ├─→ hooks.Manager.Fire(event, context)
   │   ├─→ event: SessionStart, PreToolUse, PostToolUse, etc.
   │   ├─→ context: {sessionID, toolName, input, output, cwd, ...}
   │
   └─→ For each matching hook:
       │
       ├─→ Create subprocess: sh -c "command"
       │
       ├─→ Set environment:
       │   ├─→ CLAUDIO_SESSION_ID=xyz
       │   ├─→ CLAUDIO_TOOL_NAME=Bash
       │   ├─→ CLAUDIO_CWD=/current/dir
       │   └─→ ... (context variables)
       │
       ├─→ Wait with timeout (async=false) or fire & forget (async=true)
       │
       ├─→ Log output / errors
       │
       └─→ Continue regardless of exit code
```

---

## Memory Lifecycle

### Memory Extraction (OnTurnEnd)

```
Turn ends with stop_reason: end_turn
  │
  ├─→ engine.onTurnEnd callback fires (in background)
  │
  └─→ memory.MemoryExtractor().Extract(messages):
      │
      ├─→ Parse assistant response:
      │   ├─→ Extract facts (discrete one-liners)
      │   ├─→ Extract concepts (semantic tags)
      │   └─→ Detect type (user, feedback, project, reference)
      │
      ├─→ Save memory entries:
      │   │
      │   ├─→ memory.Save(entry):
      │   │   ├─→ Decide scope: project, global, or agent
      │   │   │   ├─→ Project-scoped: ~/.claudio/memory/PROJECT_ID/
      │   │   │   ├─→ Global: ~/.claudio/memory/
      │   │   │   └─→ Agent-scoped: ~/.claudio/agents/AGENT_NAME/memory/
      │   │   │
      │   │   ├─→ Write entry to markdown: entry_name.md
      │   │   │   ├─→ Front matter (YAML)
      │   │   │   ├─→ Facts (bulleted list)
      │   │   │   ├─→ Tags (comma-separated)
      │   │   │   └─→ Concepts (comma-separated)
      │   │   │
      │   │   ├─→ Update MEMORY.md index
      │   │   │   └─→ Add link: [entry_name](entry_name.md)
      │   │   │
      │   │   └─→ Write to FTS index (SQLite)
      │   │       └─→ memory_fts.insert(entry_name, content, tags)
      │   │
      │   └─→ Auto-learning (optional):
      │       ├─→ Analyze conversation for learned patterns
      │       ├─→ learning.Store.Add(instinct)
      │       └─→ Next session loads these patterns
      │
      └─→ On memory refresh (post-compaction):
          ├─→ Rebuild index from memory files
          ├─→ Generate lean summary for new conversation era
          └─→ Inject as new memory index message on next turn
```

### Memory Injection (Session Start)

```
1. Session begins
   │
   ├─→ memory.ScopedStore.Load()
   │   ├─→ Load from project memory (if project-scoped)
   │   ├─→ Fall back to global memory
   │   └─→ Return lean index (top N entries by recency/relevance)
   │
   ├─→ engine.SetMemoryIndex(index)
   │   └─→ Inject as second user message (after CLAUDE.md)
   │
   ├─→ On turn 1:
   │   ├─→ IF memoryIndexMsg not yet injected:
   │   │   └─→ Add as user message before making API call
   │   └─→ memoryIndexInjected = true

   └─→ On memory refresh (post-compaction):
       ├─→ Rebuild index from memory files
       ├─→ Call engine.onMemoryRefresh()
       └─→ Inject fresh index as new user message

```

---

## Sub-Agent Spawning Lifecycle

### Agent Tool Execution

```
LLM uses Agent tool:
  │
  ├─→ Agent.Execute(input):
  │   ├─→ input = {type: "Explore", prompt: "find all API endpoints"}
  │
  ├─→ agents.GetAgent(type):
  │   ├─→ Check custom agent defs (~/.claudio/agents/)
  │   ├─→ Check built-in agents (internal/agents/)
  │   └─→ Return AgentDefinition{SystemPrompt, Tools, Model, ...}
  │
  ├─→ Filter registry by agent's DisallowedTools:
  │   ├─→ registry.Clone()
  │   ├─→ For each DisallowedTool: filtered.Remove(name)
  │   └─→ Merge agent-specific skills if ExtraSkillsDir set
  │
  ├─→ Create sub-agent query engine:
  │   ├─→ query.NewEngineWithConfig(apiClient, filtered, handler, config)
  │   ├─→ engine.SetSystem(agentDef.SystemPrompt)
  │   ├─→ engine.SetUserContext(subAgentContext)
  │   └─→ engine.SetMaxTurns(agentDef.MaxTurns)
  │
  ├─→ Inject SubAgentObserver context:
  │   ├─→ ctx = tools.WithSubAgentObserver(ctx, observer)
  │   ├─→ Observer tracks progress (messages, tool calls)
  │   └─→ UI: show sub-agent progress in real-time
  │
  ├─→ Optionally override model:
  │   └─→ IF agentDef.Model != "": override for sub-agent
  │
  ├─→ Run sub-agent:
  │   └─→ subEngine.Run(ctx, input.prompt)
  │       └─→ (same execution loop as main agent)
  │
  ├─→ Capture response:
  │   ├─→ Collect all messages from sub-agent
  │   └─→ Return as tool result
  │
  └─→ Main agent continues with sub-agent's response as context
```

### Sub-Agent Memory

```
IF agent has MemoryDir configured:
  │
  ├─→ Load agent-specific memory:
  │   ├─→ ~/.claudio/agents/AGENT_NAME/memory/
  │   └─→ memory.NewScopedStore(agentMemDir, ...)
  │
  ├─→ Inject agent-specific memory index
  │
  ├─→ Extract new memories into agent-scoped dir
  │
  └─→ Agent carries learned patterns across team invocations
```

---

## Team Coordination Lifecycle

### Team Creation & Spawning

```
1. User or team lead creates team:
   │
   ├─→ teams.Manager.Create(config):
   │   ├─→ config = {name, leadAgent, members[], allowPaths[], ...}
   │   ├─→ Create directory: ~/.claudio/teams/TEAM_NAME/
   │   └─→ Save config.json
   │
   └─→ teams.TeammateRunner.Spawn(member):
       │
       ├─→ Generate member ID
       ├─→ Create member mailbox directory
       │
       ├─→ Create context with decorators:
       │   ├─→ tools.WithTeamContext(ctx, {TeamName, AgentName})
       │   ├─→ tools.WithSubAgentObserver(ctx, obs)
       │   │   └─→ Tracks conversation, progress, state
       │   └─→ Propagate model override, maxTurns, autoCompactThreshold
       │
       ├─→ Spawn member as sub-agent:
       │   └─→ runSubAgent(ctx, memberSystemPrompt, memberPrompt)
       │       └─→ Creates query.Engine, runs member.Run()
       │
       └─→ Member completes or fails:
           ├─→ TeammateState.Status = complete | failed
           └─→ Lead continues (polls mailbox, dispatches next task)

2. Team coordination loop:
   │
   ├─→ WHILE team has work:
   │   │
   │   ├─→ Lead takes next task
   │   ├─→ Dispatch to available team members
   │   │
   │   ├─→ Each turn, lead polls mailbox:
   │   │   └─→ Check ~/.claudio/teams/TEAM_NAME/mailbox/ for member messages
   │   │       ├─→ IF member completed task: acknowledge, add to conversation
   │   │       └─→ IF member stuck: reassign, escalate, or fail
   │   │
   │   └─→ Lead incorporates member responses, continues
   │
   └─→ Team completes or lead gives up

3. Team shutdown:
   │
   ├─→ Lead sends shutdown signal to all members
   ├─→ Each member receives StopMessage, exits gracefully
   └─→ Clean up mailboxes, save team state
```

### Mailbox Polling

```
Each turn in query engine:
  │
  ├─→ IF team mode:
  │   │
  │   ├─→ mailboxPoller() callback fires
  │   │   ├─→ Check for new member messages
  │   │   ├─→ Check for stopMessage signals
  │   │   └─→ Return []string{messages...}
  │   │
  │   ├─→ IF messages received:
  │   │   ├─→ Add as user messages to conversation
  │   │   └─→ Continue loop (new turn with team input)
  │   │
  │   └─→ IF stopMessage received:
  │       └─→ Add message, EXIT LOOP
  │
  └─→ ELSE (not team mode):
      └─→ No polling, continue as normal
```

---

## Learning (Instinct) Lifecycle

### Auto-Learning

```
1. Successful pattern detected:
   │
   ├─→ System identifies repeatable pattern:
   │   ├─→ "When user asks for debug help, run specific debug command"
   │   ├─→ "When compiling Go, use these flags"
   │   └─→ "When deploying, always run tests first"
   │
   ├─→ learning.Store.Add(instinct):
   │   ├─→ instinct = {
   │   │     Pattern: "git.*debug",
   │   │     Response: "run: git log --oneline -n 20",
   │   │     Category: "workflow",
   │   │     Confidence: 85,
   │   │   }
   │   └─→ Saved to ~/.claudio/instincts.json
   │
   └─→ Next session loads learned patterns

2. Pattern matching:
   │
   ├─→ On session start:
   │   └─→ learning.Store.Load()
   │       └─→ Load all instincts from .json
   │
   ├─→ When LLM response arrives:
   │   ├─→ Extract patterns from assistant message
   │   ├─→ Check against learned instincts
   │   └─→ IF matches:
   │       ├─→ Confidence high? Suggest pattern
   │       └─→ learning.Instinct.UseCount++
   │
   └─→ Over time: high-confidence patterns get automatic suggestions
```

---

## Prompt Caching Lifecycle

### Cache Setup

```
1. First API call:
   │
   ├─→ Build system prompt (static + dynamic sections)
   ├─→ Mark system as cacheable:
   │   └─→ Cache control: ephemeral / TTL 5min
   │
   ├─→ Send request WITH cache headers:
   │   └─→ X-Anthropic-Cache-Control: {"type": "ephemeral"}
   │
   ├─→ API response includes:
   │   ├─→ cache_creation_input_tokens: 5000
   │   ├─→ cache_read_input_tokens: 0
   │   └─→ usage: {input_tokens: 5100, ...}
   │
   └─→ Freeze system prompt for subsequent turns

2. Subsequent API calls (same system):
   │
   ├─→ Use frozen system prompt (no reconstruction)
   ├─→ Send request WITH cache headers
   │
   ├─→ API response includes:
   │   ├─→ cache_creation_input_tokens: 0
   │   ├─→ cache_read_input_tokens: 5000  ← CACHE HIT!
   │   └─→ usage: {input_tokens: 500, ...}  ← FEWER TOKENS!
   │
   └─→ Continue conversation at reduced cost

3. Cache expiry:
   │
   ├─→ 5 min passes since first cache write
   ├─→ OR cwd changes (CwdChanged hook fires)
   ├─→ OR major prompt change triggered
   │
   └─→ Rebuild system prompt, reset cache
```

---

## Conversation Compaction Lifecycle

### Auto-Compaction (at 95% context)

```
1. Check context usage:
   │
   ├─→ IN query engine loop, track token usage
   ├─→ IF usage >= 95% of context_window:
   │   │
   │   └─→ Trigger auto-compaction

2. Compaction phase:
   │
   ├─→ Summarize conversation:
   │   ├─→ Identify key facts, decisions, problems
   │   ├─→ Build lean summary of conversation era
   │   └─→ Strip repetitive, low-value messages
   │
   ├─→ Replace old messages:
   │   ├─→ Delete old messages from history
   │   ├─→ Insert single "system" message with summary
   │   └─→ Keep recent N messages (last 10-20 turns)
   │
   ├─→ Refresh memory index:
   │   ├─→ engine.onMemoryRefresh() callback
   │   ├─→ Rebuild memory index from memory files
   │   └─→ Inject fresh index as user message
   │
   ├─→ Reset prompt cache:
   │   └─→ New system prompt (no old cache)
   │
   └─→ Continue conversation with compacted history

3. Compaction metrics:
   │
   ├─→ Before: 150K tokens
   ├─→ After: 50K tokens (66% reduction)
   ├─→ Continue with newly available 100K token space
   │
   └─→ Engine.onAutoCompact() callback fires
       └─→ Auditor logs compaction event
```

---

## Configuration Merging & Trust Flow

### Config Load Sequence

```
1. User runs: claudio --model claude-opus "prompt"
   │
   ├─→ PersistentPreRunE fires:
   │
   ├─→ Detect project root:
   │   ├─→ config.FindGitRoot(cwd)
   │   ├─→ Search upward for .git/
   │   └─→ Return project root (or cwd if not in git)
   │
   ├─→ Trust check (IF project has local config):
   │   ├─→ config.HasProjectConfig(projectRoot)
   │   │   └─→ Check for ~/.claudio/config.json in git root
   │   │
   │   ├─→ IF untrusted AND config has hooks/MCP:
   │   │   ├─→ Show trust prompt
   │   │   ├─→ config.FormatTrustPrompt(projectRoot, info)
   │   │   └─→ Scan for: hooks, MCP servers, permission rules
   │   │
   │   └─→ IF user approves: config.TrustStore.Trust(projectRoot)
   │
   ├─→ Load settings (priority order):
   │   ├─→ 1. User config (~/.claudio/claudio.json)
   │   ├─→ 2. Project config (git_root/.claudio/claudio.json) [if trusted]
   │   ├─→ 3. Environment variables (CLAUDIO_MODEL, CLAUDIO_API_KEY)
   │   └─→ 4. CLI flags (--model)
   │
   ├─→ Apply CLI overrides:
   │   ├─→ flagModel != "" → settings.Model = flagModel
   │   ├─→ flagBudget > 0 → settings.MaxBudget = flagBudget
   │   └─→ flagDangerouslySkipPerm → settings.PermissionMode = "dangerously-skip-permissions"
   │
   └─→ config.Settings object ready for app.New()
```

---

## Summary: Key Interaction Patterns

1. **Request/Response**: LLM turns, tool execution, result injection, retry on max_tokens
2. **Sessions**: Start, resume, append messages, cleanup
3. **Memory**: Extract facts at turn end, inject index at session start, rebuild on compaction
4. **Teams**: Spawn members, poll mailbox each turn, coordinate via messages
5. **Permissions**: Evaluate rules, prompt user, log audit trail
6. **Hooks**: Fire at lifecycle points (session start/end, pre/post tool)
7. **Agents**: Spawn sub-agents with filtered toolset, capture response
8. **Caching**: Freeze system prompt, reuse on subsequent calls, expire on major changes
9. **Compaction**: Summarize at 95% context, refresh memory index, continue
10. **Trust**: Scan project config for security-relevant settings, prompt user once

