# Claudio Architecture Map

## Executive Summary

Claudio is a terminal-first AI coding assistant built in Go with a modular, layered architecture:

1. **Entry Point**: `cmd/claudio/main.go` → `cli.Execute()`
2. **App Bootstrap**: `internal/cli/root.go` (Cobra CLI) → `app.New()` (dependency injection)
3. **Core Engine**: `internal/query/engine.go` (LLM conversation loop)
4. **Tool System**: `internal/tools/registry.go` (50+ tools with deferred loading)
5. **UI Layers**: TUI (Bubble Tea), Web (go-templ + HTMX), CLI (Cobra)

All subsystems are wired into a single `App` struct that flows through the entire application.

---

## Application Startup Flow

```
1. cmd/claudio/main.go
   └─→ cli.Execute()
       └─→ rootCmd.Execute() (Cobra)
           ├─→ PersistentPreRunE:
           │   ├─→ config.Load(projectRoot)      [Load ~/.claudio/claudio.json + env vars + CLI flags]
           │   ├─→ config.EnsureDirs()           [Create ~/.claudio directories]
           │   ├─→ trust.Check()                 [Verify project config if untrusted]
           │   └─→ app.New(settings, projectRoot) [FULL DEPENDENCY INJECTION]
           │       └─→ [See App Initialization below]
           │
           └─→ RunE (based on flags):
               ├─→ Interactive mode (runInteractive) [TUI]
               ├─→ Single prompt mode (runSinglePrompt) [Print mode]
               └─→ Pipe mode (read from stdin)

2. Query Execution (runSinglePrompt):
   ├─→ Auth check (appInstance.Auth.IsLoggedIn())
   ├─→ Token refresh (refresh.CheckAndRefreshIfNeeded)
   ├─→ Set up registry (applyAgentOverrides)
   ├─→ Create query.Engine (query.NewEngineWithConfig)
   │   └─→ Set system prompt, user context, memory index
   └─→ engine.Run(ctx, userMessage)
       └─→ [See Query Engine Loop below]
```

---

## App Initialization (app.New)

The `App` struct is the single source of truth for all application state:

```go
type App struct {
    Config           *config.Settings          // User settings from claudio.json
    Bus              *bus.Bus                  // Event pub/sub
    Storage          authstorage.SecureStorage // Credentials (keychain/env)
    Auth             *auth.Resolver            // Auth credential resolution
    API              *api.Client               // Anthropic/multi-provider API client
    DB               *storage.DB               // SQLite database
    Tools            *tools.Registry           // 50+ tools with deferred loading
    Hooks            *hooks.Manager            // Lifecycle hooks (pre/post tool, session lifecycle)
    Learning         *learning.Store           // Pattern learning (instincts)
    Skills           *skills.Registry          // Instruction extensions
    Memory           *memory.ScopedStore       // Persistent markdown memory
    Analytics        *analytics.Tracker        // Token usage tracking
    FilterSavings    *filtersavings.Service    // Output filter statistics
    Auditor          *security.Auditor         // Audit logging
    TaskRuntime      *tasks.Runtime            // Background task execution
    Teams            *teams.Manager            // Team management
    TeamRunner       *teams.TeammateRunner     // Team execution
    Plugins          *plugins.Registry         // Dynamic plugin discovery
    Cron             *tasks.CronStore          // Cron job persistence
    LSP              *lsp.ServerManager        // LSP server lifecycle
}
```

**Initialization sequence:**

1. **Config & storage**
   - Load settings from `~/.claudio/claudio.json` + env vars
   - Create secure credential storage (keychain/env)
   - Create auth resolver

2. **API client**
   - Register multi-provider APIs (OpenAI, Anthropic, Ollama)
   - Set model shortcuts and routing rules
   - Apply thinking mode, budget, effort level

3. **Database**
   - Open SQLite at `~/.claudio/claudio.db`
   - Initialize schema (sessions, messages, FTS index, audit logs)

4. **Core subsystems**
   - Create event bus
   - Initialize hooks manager (load from `~/.claudio/hooks.json`)
   - Initialize learning store (load from `~/.claudio/instincts.json`)

5. **Security & tool wiring**
   - Create tool registry with 50+ tools
   - Inject security context (deny/allow paths, denied commands)
   - Inject tool-specific configs (output filter, LSP manager, snippet expansion)
   - Remove denied tools

6. **Services**
   - LSP server manager (code intelligence)
   - Memory store (project/global scoped, FTS write-through)
   - Model capabilities cache
   - Analytics tracker
   - Task runtime (background execution)
   - Plugins registry (discover & load from `~/.claudio/plugins/`)

7. **Team & agent infrastructure**
   - Team manager (load from `~/.claudio/teams/`)
   - Team runner (with sub-agent callbacks)
   - Agent discovery (built-in + custom from `~/.claudio/agents/`)

---

## Query Engine Loop (internal/query)

The `Engine` orchestrates the core conversation loop:

```
engine.Run(ctx, userMessage)
  │
  ├─→ Fire SessionStart hook (once per session)
  ├─→ Inject user context (CLAUDE.md) as first user message (once)
  ├─→ Inject memory index as second user message (once)
  │
  └─→ LOOP (maxTurns constraint):
      │
      ├─→ Check mailbox (team messages)
      ├─→ Build full system prompt (static + dynamic sections)
      ├─→ Build API request with:
      │   ├─→ Messages (conversation history)
      │   ├─→ System prompt
      │   ├─→ Tool definitions (APIDefinitionsWithDeferral)
      │   ├─→ Model, thinking mode, thinking budget, effort level
      │   └─→ Prompt caching config
      │
      ├─→ Send to LLM (api.Client.CreateMessageStream)
      │
      ├─→ Stream response:
      │   ├─→ OnTextDelta → handler.OnTextDelta(text)
      │   ├─→ OnThinkingDelta → handler.OnThinkingDelta(text)
      │   ├─→ ToolUse → handler.OnToolUseStart(toolUse)
      │   └─→ Track usage for analytics
      │
      ├─→ Stop reason?
      │
      ├─→ IF tool_use:
      │   ├─→ Fire PreToolUse hook
      │   ├─→ Check permissions (content-pattern rules)
      │   ├─→ Prompt user approval if needed (OnToolApprovalNeeded)
      │   ├─→ Execute tool via registry.Execute()
      │   ├─→ Fire PostToolUse hook (success) or PostToolUseFailure hook
      │   ├─→ Capture result (on disk if > size limit)
      │   ├─→ handler.OnToolUseEnd(toolUse, result)
      │   ├─→ Add tool result to message history
      │   └─→ CONTINUE LOOP (retry request with result)
      │
      ├─→ ELIF max_tokens_exceeded:
      │   ├─→ Escalate max_tokens (normal: 8K → escalated: 64K)
      │   ├─→ Retry request (handler.OnRetry)
      │   └─→ CONTINUE LOOP
      │
      ├─→ ELIF end_turn:
      │   ├─→ Fire OnTurnEnd callback (memory extraction)
      │   ├─→ Check cost threshold (OnCostConfirmNeeded)
      │   └─→ EXIT LOOP
      │
      └─→ ELSE (error or unknown stop reason):
          └─→ handler.OnError(err)
```

---

## Tool System (internal/tools)

### Registry Architecture

```go
type Registry struct {
    tools        map[string]Tool        // Tool lookup by name
    order        []string               // Insertion order
    deferOverride map[string]bool       // Manual defer pinning
}
```

**Key methods:**
- `Register(tool)` – Add tool to registry
- `Get(name)` – Lookup by name
- `All()` – All tools in order
- `APIDefinitions()` – Full schemas (no deferral)
- `APIDefinitionsWithDeferral()` – Schemas with deferred loading support
- `Execute(name, input)` – Run tool with input validation
- `Clone()` – Create copy for sub-agents/filtered contexts

### Tool Categories (50+ tools)

**Core Files:**
- `Bash` – Shell command execution with security context
- `Read` – File reading with cache layer
- `Write` – File creation with snippet expansion
- `Edit` – File editing with context, security, snippets

**Code Intelligence:**
- `Glob` – Pattern file matching with caching
- `Grep` – Content search with ripgrep
- `LSP` – Language server protocol integration (go to def, find refs, hover, etc.)
- `Models` – Model capability lookup

**Agent Coordination:**
- `Agent` – Sub-agent spawning (Explore, Plan, various specialists)
- `Memory` – Persistent memory operations (save/load/search)
- `Recall` – Fast memory vector search
- `Skill` – Skill registry and invocation
- `Tasks` – Background task management (run, monitor, cancel)
- `ToolSearch` – Deferred tool schema lookup

**Team Coordination:**
- `SendMessage` – Send message to team member
- `TeamCreate` – Spawn new team
- `TeamDelete` – Teardown team
- `TeamTemplate` – List/instantiate team templates

**Cron & Scheduling:**
- `CronCreate` – Schedule background job
- `CronDelete` – Cancel cron job
- `CronList` – List active crons

**Multi-Protocol:**
- `MCP` – Model Context Protocol server management
- `WebFetch` – HTTP fetch with headers
- `WebSearch` – Search engine integration

**Other:**
- `AskUser` – Interactive prompts
- `Advisor` (injected) – Advisor tool for plan mode
- Plugins (dynamic) – Proxy tools for executables

### Deferred Loading

Large tools (Glob, Grep, LSP, Agent) implement `DeferrableTool`:

```go
type DeferrableTool interface {
    Tool
    ShouldDefer() bool
}
```

- Full schema sent only if discovered (used in conversation)
- Otherwise, `defer_loading: true` + name only
- Avoids large API payloads for tools user won't call

### Security Injection

```
BashTool:          Injected: security context, output filter, filter recorder
FileReadTool:      Injected: security context, config
FileWriteTool:     Injected: security context
FileEditTool:      Injected: security context
SkillTool:         Injected: skills registry
PluginProxyTool:   Injected: output filter, filter recorder
LSP:               Injected: LSP server manager
```

---

## Permission System (internal/permissions + internal/query)

**Permission rules** are content-pattern matchers defined in `claudio.json`:

```json
{
  "permissionRules": [
    {
      "tool": "Bash",
      "pattern": "^git",
      "behavior": "auto"
    },
    {
      "tool": "Write",
      "pattern": "\\.env$",
      "behavior": "deny"
    }
  ]
}
```

**Evaluation flow (query engine):**

1. Extract content from tool input (command for Bash, path for Read/Write/Edit)
2. Match against all rules for that tool
3. Return behavior: `auto` (approve), `manual` (ask), `deny` (reject)

**Permission modes:**
- `default` – Strict prompting for non-matched or manual rules
- `auto` – Auto-approve matching rules, manual otherwise
- `headless` – Auto-approve all
- `plan` – Special mode for plan tool

---

## Wiring Patterns

### A. Event Bus (internal/bus)

Concurrent pub/sub for lifecycle events:

```go
bus := bus.New()
// Publish event with type + payload
bus.Publish("tool_use_start", json.Marshal(toolUse))
// Subscribe to type or wildcard
bus.Subscribe("tool_*", handler)
```

**Event types:** `tool_use_start`, `tool_use_end`, `session_start`, `session_end`, etc.

### B. Hooks (internal/hooks)

Shell command execution at lifecycle points:

```go
// ~/.claudio/hooks.json
{
  "hooks": [
    {
      "id": "git-auto-commit",
      "type": "command",
      "event": "PostToolUse",
      "command": "git add -A && git commit -m 'auto'",
      "async": true
    }
  ]
}
```

**Events:** `SessionStart`, `SessionEnd`, `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `PreCompact`, `SubagentStart`, `SubagentStop`, `CwdChanged`

### C. Security Context Injection

```go
// At app init:
type SecurityContext struct {
    DenyPaths    []string
    AllowPaths   []string
    DenyCommands []string
}

// Injected into BashTool, FileReadTool, etc:
bashTool.Security = securityContext
```

### D. Skill Tool Auto-Detection

```go
// At app init:
skillsRegistry := skills.LoadAll(systemDir, projectDir)
skillTool.SkillsRegistry = skillsRegistry

// Tools.SkillTool.Execute auto-discovers and invokes skills
// Replaces system prompt section with skill content
```

### E. Agent Overrides (applyAgentOverrides)

```go
agentDef := agents.GetAgent("backend-senior")
filtered := registry.Clone()
// Remove DisallowedTools
for _, name := range agentDef.DisallowedTools {
    filtered.Remove(name)
}
// Merge extra skills for this agent
```

### F. Team Context Injection

```go
// In team runner:
ctx = tools.WithTeamContext(ctx, tools.TeamContext{
    TeamName:  "backend-team",
    AgentName: "code-reviewer",
})

// Sub-agent tools (Agent, SendMessage, etc) read from context
```

---

## Multi-Provider API (internal/api + internal/api/provider)

```
api.Client:
├─→ RegisterProvider(name, provider)  [OpenAI, Anthropic, Ollama]
├─→ AddModelShortcut(alias, modelID)  [e.g., "claude-opus" → full ID]
├─→ AddModelRoute(pattern, provider)  [Route models to specific provider]
├─→ CreateMessage(request)             [Blocking request]
├─→ CreateMessageStream(request)       [Streaming with event handlers]
└─→ SetThinkingMode(), SetBudgetTokens(), SetEffortLevel()

provider.Provider interface:
├─→ NewRequest(model, messages, tools)
├─→ SendRequest()
└─→ TranslateResponse()
```

**Provider-specific behavior:**
- **OpenAI**: Standard `/v1/chat/completions` translation
- **Anthropic**: Native Messages API support
- **Ollama**: Custom `/api/chat` endpoint (native options support for ctx window)

---

## Session & Memory (internal/session + internal/services/memory)

### Sessions (SQLite-backed)

```go
session := session.Start()        // Create new session in DB
session.Resume(sessionID)         // Load existing session
session.AddMessage(msg)           // Append to conversation
session.AddToolMessage(toolUse)   // Append tool result
session.GetMessages()             // Fetch full history
```

**DB schema:**
- `sessions` – Session metadata (ID, title, created_at, finished_at)
- `messages` – Conversation history (role, content)
- `audit_log` – Tool execution audit trail
- `memory_fts` – Full-text search index

### Memory (Markdown + FTS)

```
~/.claudio/memory/
├─→ MEMORY.md          [Index with links to entries]
├─→ entry_name.md      [Individual memory entry]
└─→ FTS index (DB)     [Full-text search]

Entry structure:
├─→ Name               [Unique identifier]
├─→ Type               [user, feedback, project, reference]
├─→ Scope              [project, global, agent]
├─→ Facts[]            [Discrete one-liner facts]
├─→ Tags[]             [Manual tags]
├─→ Concepts[]         [Auto-extracted semantic tags]
└─→ UpdatedAt          [Last modified]
```

**Memory-aware tools:**
- `Memory` tool – Save/load/list/search
- `Recall` tool – Vector-based search
- Engine automatically injects memory index as second user message

---

## Teams & Agents (internal/teams + internal/agents)

### Agent System

**Built-in agents:**
- `general-purpose` – Default multi-purpose assistant
- `Explore` – Fast codebase investigation
- `Plan` – Architecture & implementation planning
- `backend-jr/mid/senior` – Specialized backend engineers
- `frontend-jr/mid/senior` – React/TypeScript specialists
- `go-htmx-frontend-*` – Go + htmx specialists
- `code-investigator` – Symbol tracing & impact analysis
- `pentest` – Offensive security testing
- `qa` – E2E testing & security probes
- ... and more

**Each agent has:**
- `SystemPrompt` – Role definition
- `Tools` – Available tools (can be overridden)
- `DisallowedTools` – Tools to disable
- `Model` – Optional model override
- `MaxTurns` – Conversation limit
- `WhenToUse` – Guidance for selection
- `MemoryDir` – Optional persistent memory

### Team Coordination

```go
type Team struct {
    Name                 string
    LeadAgent            string
    LeadSession          string
    Members              []TeamMember
    AllowPaths           []string
    AutoCompactThreshold int
}

type TeamMember struct {
    Identity    TeammateIdentity
    Status      MemberStatus  // idle, working, complete, failed, shutdown, waiting_for_input
    JoinedAt    time.Time
    TaskID      string
    Model       string
    SubagentType string
}
```

**Coordination mechanisms:**

1. **Mailbox polling** – Each turn, main agent polls for messages from team members
2. **Teammate runner** – Spawns sub-agents with context decorators
3. **State tracking** – TeammateState tracks conversation, progress, model override
4. **Shared task list** – Team members coordinate work via task runtime
5. **Shared memory** – Team can access project/global memory

---

## Learning & Analytics (internal/learning + internal/services/analytics)

### Learning (Instinct System)

```go
type Instinct struct {
    ID         string    // Unique ID
    Pattern    string    // Learned pattern (regex)
    Response   string    // Suggested response
    Category   string    // debugging, workflow, convention, workaround
    Confidence int       // 0-100
    UseCount   int       // Times used
    CreatedAt  time.Time
}

store := learning.NewStore("~/.claudio/instincts.json")
store.Add(instinct)
store.Get(id)
```

**Auto-learning:** Engine can add instincts from successful patterns discovered in sessions.

### Analytics

```go
tracker := analytics.NewTracker(model, maxBudget, analyticsDir)
tracker.Record(sessionID, usage)  // Record token usage & cost
// Writes to ~/.claudio/analytics/
```

---

## Plugins (internal/plugins)

Plugins extend Claudio with custom tools via subprocess invocation:

```
~/.claudio/plugins/
└─→ my-plugin [executable]
    ├─→ Invoked with: my-plugin [input-json]
    ├─→ Returns: JSON with name, description, inputSchema, output
    └─→ Wrapped as ProxyTool in registry
```

**Plugin discovery:**
1. Scan `~/.claudio/plugins/` for executables
2. Load LSP configs from plugin manifests
3. Create ProxyTool for each plugin
4. Register in tool registry

---

## UI Layers

### 1. CLI (internal/cli)

Cobra command-line interface:
- `claudio [prompt]` – Single prompt or interactive
- `claudio detect` – Detect project root
- `claudio init` – Initialize ~/.claudio/
- `claudio web` – Start web server
- `claudio auth login` – Authenticate
- Flags: `--model`, `--budget`, `--headless`, `--agent`, `--team`, `--dangerously-skip-permissions`

### 2. TUI (internal/tui)

Bubble Tea terminal UI:
- **Editor** – Multi-line prompt with syntax highlighting
- **Chat view** – Stream responses in real time
- **File picker** – Browse and attach files
- **Image viewer** – Display images inline
- **Components** – Buttons, input fields, lists, notifications
- **Panels** – Chat, attachments, memory, codebase, docs
- **Docks** – Bottom status bar
- **Permissions dialog** – Approve/deny tool use
- **Agent/model selector** – UI for overrides

### 3. Web (internal/web)

go-templ + HTMX + SSE:
- **HTTP server** – Listens on configurable port
- **Session manager** – Tracks web sessions (separate from CLI)
- **SSE endpoints** – Stream LLM responses
- **Template system** – go-templ for type-safe HTML
- **HTMX handlers** – Out-of-band swaps, polling, etc.

---

## Config System (internal/config)

### Settings Structure

```go
type Settings struct {
    Model              string
    APIBaseURL         string
    MaxBudget          float64
    BudgetTokens       int
    ThinkingMode       string       // "enabled", "disabled"
    EffortLevel        string
    PermissionMode     string       // "default", "auto", "headless"
    PermissionRules    []PermissionRule
    DenyPaths          []string
    AllowPaths         []string
    DenyTools          []string
    OutputFilter       bool
    Providers          map[string]ProviderConfig
    ModelRouting       map[string]string
    LspServers         map[string]LspServerConfig
    Hooks              map[string]HookDef
    Snippets           map[string]SnippetDef
    Advisor            *AdvisorSettings
    // ... more fields
}
```

### Configuration Merging

```
Priority (high to low):
1. CLI flags (--model, --budget, etc.)
2. Project config (~/.claudio/claudio.json in git root)
3. User config (~/.claudio/claudio.json)
4. Environment variables
5. Defaults
```

### File Layout

```
~/.claudio/
├─→ claudio.json              [User settings]
├─→ claudio.db               [SQLite database]
├─→ hooks.json               [Lifecycle hooks]
├─→ instincts.json           [Learned patterns]
├─→ memory/                  [Persistent memory]
│   ├─→ MEMORY.md
│   └─→ entry_*.md
├─→ skills/                  [Instruction extensions]
│   └─→ *.md
├─→ agents/                  [Custom agent definitions]
│   └─→ *.json
├─→ plugins/                 [Executable plugins]
│   └─→ my-plugin
├─→ teams/                   [Team configurations]
│   └─→ team-name/
├─→ cache/                   [Model capabilities, etc.]
│   └─→ model-capabilities.json
├─→ analytics/               [Token usage logs]
│   └─→ *.json
└─→ task-output/             [Background task output files]
    └─→ task-id.log
```

---

## Storage (internal/storage)

SQLite database with structured schema:

```sql
-- Session management
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    title TEXT,
    created_at TIMESTAMP,
    finished_at TIMESTAMP
);

-- Conversation history
CREATE TABLE messages (
    id INTEGER PRIMARY KEY,
    session_id TEXT,
    role TEXT,       -- "user", "assistant"
    content BLOB,    -- JSON-encoded content blocks
    created_at TIMESTAMP
);

-- Audit trail
CREATE TABLE audit_log (
    id INTEGER PRIMARY KEY,
    session_id TEXT,
    tool_name TEXT,
    input TEXT,
    output TEXT,
    status TEXT,     -- "success", "denied", "error"
    timestamp TIMESTAMP
);

-- Full-text search for memory
CREATE VIRTUAL TABLE memory_fts (
    name TEXT,
    content TEXT,
    tags TEXT
);
```

---

## Request/Response Flow (Detailed)

```
User: claudio "analyze this bug"
  │
  ├─→ [CLI] Parse flags, load config, init App
  │
  ├─→ [Query Engine] Run(ctx, "analyze this bug")
  │   │
  │   ├─→ Fire SessionStart hook
  │   ├─→ Inject CLAUDE.md as first user message (once)
  │   ├─→ Inject memory index as second user message (once)
  │   │
  │   └─→ LOOP:
  │       │
  │       ├─→ [System Prompt] Build from:
  │       │   ├─→ Static sections (intro, instructions, tone)
  │       │   ├─→ Tool descriptions (50+ tools)
  │       │   ├─→ Skill content (appended)
  │       │   ├─→ Plugin instructions
  │       │   └─→ Dynamic sections (git status, cwd context)
  │       │
  │       ├─→ [Messages] Current conversation + memory + context
  │       │
  │       ├─→ [API Request] api.Client.CreateMessageStream:
  │       │   ├─→ Resolve model routing (pattern → provider)
  │       │   ├─→ Set thinking budget from config
  │       │   ├─→ Build Anthropic Messages API request
  │       │   ├─→ Attach prompt caching headers
  │       │   └─→ Send to provider with streaming
  │       │
  │       ├─→ [Stream Loop]:
  │       │   ├─→ text chunk:
  │       │   │   ├─→ handler.OnTextDelta(text)
  │       │   │   └─→ [UI] TUI: render to editor, Web: SSE event
  │       │   │
  │       │   ├─→ tool_use block start:
  │       │   │   ├─→ Parse tool name + input
  │       │   │   └─→ handler.OnToolUseStart(toolUse)
  │       │   │
  │       │   └─→ stop_reason:
  │       │       └─→ See stop reason handling below
  │       │
  │       ├─→ [Stop Reason Handling]:
  │       │
  │       │   ├─→ tool_use:
  │       │   │   ├─→ Add assistant message to history
  │       │   │   ├─→ Fire PreToolUse hook
  │       │   │   ├─→ Check permissions:
  │       │   │   │   ├─→ Extract content (cmd for Bash, path for Read, etc.)
  │       │   │   │   ├─→ Match against rules
  │       │   │   │   └─→ Return behavior (auto/manual/deny)
  │       │   │   ├─→ If manual: handler.OnToolApprovalNeeded(toolUse)
  │       │   │   │   └─→ [UI] TUI: show approval dialog, Web: block & ask
  │       │   │   ├─→ If approved:
  │       │   │   │   ├─→ registry.Execute(toolName, input)
  │       │   │   │   │   ├─→ Validate input against schema
  │       │   │   │   │   ├─→ Run tool (Bash, Read, Glob, etc.)
  │       │   │   │   │   └─→ Return result (or defer to disk if > threshold)
  │       │   │   │   ├─→ Fire PostToolUse hook
  │       │   │   │   ├─→ handler.OnToolUseEnd(toolUse, result)
  │       │   │   │   ├─→ Add tool result to message history
  │       │   │   │   └─→ CONTINUE LOOP (retry LLM with result)
  │       │   │   └─→ If denied:
  │       │   │       └─→ Add tool denial message + CONTINUE LOOP
  │       │   │
  │       │   ├─→ end_turn:
  │       │   │   ├─→ Add final assistant message to history
  │       │   │   ├─→ Fire OnTurnEnd callback:
  │       │   │   │   └─→ memory.MemoryExtractor().Extract() [background]
  │       │   │   ├─→ Check cost threshold:
  │       │   │   │   └─→ If over: handler.OnCostConfirmNeeded(currentCost, threshold)
  │       │   │   ├─→ Check maxTurns:
  │       │   │   │   └─→ If reached: EXIT LOOP
  │       │   │   └─→ Check mailbox (team messages)
  │       │   │       └─→ If none or no new turns: EXIT LOOP
  │       │   │
  │       │   ├─→ max_tokens:
  │       │   │   ├─→ Escalate max_tokens (8K → 64K)
  │       │   │   ├─→ handler.OnRetry(toolUses)
  │       │   │   └─→ CONTINUE LOOP (retry with higher limit)
  │       │   │
  │       │   └─→ ERROR / unknown:
  │       │       └─→ handler.OnError(err)
  │       │
  │       └─→ [Turn Complete] handler.OnTurnComplete(usage)
  │           └─→ Record analytics, update cost tracker
  │
  └─→ [Final] Print cost summary to stderr
```

---

## Key Integration Points

### Memory Extraction (OnTurnEnd)

```go
appInstance.MemoryExtractor() → func(messages) {
    // At end of turn, extract:
    // 1. Facts from assistant responses
    // 2. Learned patterns
    // 3. Update memory index
    // (runs in background)
}
```

### Sub-agent Callbacks (Team Runner)

```go
teamRunner.SetRunSubAgent(func(ctx, system, prompt) {
    // Calls runSubAgent() which:
    // 1. Creates new query.Engine with sub-agent system prompt
    // 2. Runs engine.Run(ctx, prompt)
    // 3. Captures response
    // 4. Returns to team lead
})

teamRunner.SetRunSubAgentWithMemory(func(ctx, system, prompt, memoryDir) {
    // Same, but with memory dir:
    // - Memory is project-scoped to that directory
    // - Agent carries learned instincts into team work
})
```

### Skills Resolution

```
1. Load all skills from: bundled + ~/.claudio/skills/ + project/.claudio/skills/
2. Agent can specify ExtraSkillsDir for agent-specific instructions
3. SkillTool.Execute():
   - Looks up skill by name in registry
   - Appends content to system prompt section
   - Re-invokes LLM with expanded prompt
```

### Agent Discovery

```
agents.GetAgent(name) → AgentDefinition {
    // Priority:
    // 1. Custom agents from ~/.claudio/agents/ + ./.claudio/agents/
    // 2. Built-in agents (go source)
}
```

---

## Error Handling & Resilience

### Graceful Degradation

1. **Missing config files** – Defaults applied, no crash
2. **LSP server unavailable** – LSP tool reports error, agent continues
3. **Plugin load failure** – Logged, non-essential plugins skipped
4. **Prompt cache miss** – Falls back to regular caching
5. **Memory extraction timeout** – Logged, session continues

### Recovery Mechanisms

1. **Token escalation** – If max_tokens hit mid-stream, retry with higher limit
2. **Tool result disk offload** – Large outputs saved to disk, reference in message
3. **Conversation compaction** – Auto-compact at 95% context if enabled
4. **Max turns limit** – Prevents infinite loops
5. **Mailbox timeout** – Team messages have deadline before proceeding

---

## Security Model

### Path Access Control

```go
security.CheckPathAccess(path, denyPaths, allowPaths) {
    // Deny list checked first (blocklist > allowlist)
    // Glob patterns supported
    // Expands symlinks and relative paths
}
```

### Command Safety

```go
security.CheckCommandSafety(cmd, denyCommands) {
    // Regex patterns for dangerous commands
    // E.g., deny "rm -rf", "dd", etc.
}
```

### Secret Scanning

```go
security.ScanForSecrets(output) {
    // Detects API keys, tokens, passwords in output
    // Warns before returning to user
}
```

### Audit Trail

```go
auditor.LogToolCall(sessionID, toolName, input, output, status)
// Persists to audit_log table in SQLite
```

---

## Performance Optimizations

1. **Deferred tool loading** – Large tool schemas sent only if used
2. **Grep/Read caching** – Results cached in memory + disk
3. **Prompt caching** – Static prompt sections cached on API
4. **Tool result disk offload** – Large outputs (> threshold) kept off messages
5. **FTS indexing** – Fast memory search via SQLite FTS
6. **Context compaction** – Auto-compact conversation at 95% window
7. **Output filtering** – Strip ANSI codes, duplicates, noise from command output

---

## Known Limitations & TODOs

1. **Multi-session coordination** – Bridge uses Unix sockets (Unix-only)
2. **Parallel agent execution** – Teams are sequential by default (mailbox polling)
3. **Memory scalability** – Very large memory dirs (10K+ entries) may slow FTS
4. **Plugin debugging** – Limited error context from subprocess plugins
5. **TUI accessibility** – No screen reader support in Bubble Tea

---

## Summary: Connection Map

```
cmd/claudio/main.go
  ↓
cli.Execute()
  ↓
app.New(config, projectRoot)
  ├─→ api.Client (OpenAI/Anthropic/Ollama)
  ├─→ tools.Registry (50+ tools)
  ├─→ query.Engine (conversation loop)
  ├─→ bus.Bus (events)
  ├─→ hooks.Manager (lifecycle)
  ├─→ services.Memory (persistent)
  ├─→ teams.Manager (team coordination)
  ├─→ storage.DB (SQLite)
  ├─→ security.Auditor (audit logging)
  ├─→ plugins.Registry (dynamic tools)
  └─→ learning.Store (instinct learning)
       ↓
     UI Layer (TUI, Web, CLI)
       ↓
     User Response
```

