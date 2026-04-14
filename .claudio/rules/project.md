---
name: project-conventions
description: Coding standards and constraints for the Claudio project
---

- Module path is `github.com/Abraxas-365/claudio` — use this for all internal imports
- Build with `make build` (injects version via ldflags); plain `go build` omits version info
- All new packages go under `internal/` — nothing outside `cmd/` and `internal/` should contain business logic
- The `internal/cli.Version` variable is set at build time via ldflags; do not hardcode version strings elsewhere
- Use `modernc.org/sqlite` (pure Go, no CGO) for all SQLite access — do not introduce CGO sqlite bindings
- TUI components use the Bubbletea/Bubbles/Lipgloss stack; new UI elements must follow the existing model/update/view pattern in `internal/tui/`
- Storage migrations live in `internal/storage/migrations/` and `internal/config/migrations/` — add new migrations there, never alter existing ones
- Permission rules use the pattern `allow: Bash(git *)` / `deny: Write(*.env)` — see `internal/permissions/rules.go` for the canonical rule format
- Hook lifecycle events are defined in `internal/bus/events.go` — add new events there only
- Tools must be registered via `internal/tools/registry.go` — do not invoke tool logic directly from outside the tools package
- Session-scoped, agent-scoped, and global memory are distinct — do not conflate them when working in `internal/services/memory/` or `internal/learning/`
- The `internal/agents/crystallize.go` path handles session→agent persona promotion; changes there affect how sessions are persisted as reusable agents
- Cron expressions follow the formats `@every <duration>`, `@daily`, or `HH:MM` — validated in `internal/utils/cron.go`
- Use the `Read` tool to read files — never `cat`, `head`, `tail`, or `sed`
- Use the `Grep` tool to search file contents — never shell `grep` or `rg`
- Use the `Glob` tool to find files — never `find` or `ls`
- Reserve `Bash` for commands that genuinely require shell execution (build, run tests, git ops)
- When spawning teammates from a team template, name agents using the template member name plus a numeric suffix (e.g. `rafael-1`, `rafael-2`, `alex-1`). Reuse these names — do not invent new unique identifiers — to keep the agent list compact and predictable. Once an agent has finished its task, its name is free to be reused for a new spawn
- Always respect the model specified in the team template for each member when spawning via `SpawnTeammate`; never override the model unless the user explicitly requests it

## Architecture

### Package Map

- `cmd/claudio/` — binary entry point; calls `cli.Execute()` and exits
- `internal/app/` — application bootstrap (`App` struct); wires bus, auth, DB, tool registry, hooks, memory, skills, teams, plugins, tasks, LSP, and analytics into one object passed to all commands
- `internal/cli/` — Cobra command tree (`root.go`, `init.go`, `version.go`, `web.go`, `auth.go`, `detect.go`); subcommands live in `internal/cli/commands/` (`commands.go`, `core.go`)
- `internal/api/` — LLM client abstraction (`Client`); multi-provider routing; provider implementations in `internal/api/provider/` (OpenAI, Anthropic, Ollama); OAuth + token refresh under `internal/api/oauth/` and `internal/api/refresh/`
- `internal/query/` — core agentic loop (`engine.go`); sends messages to the LLM, handles streaming, dispatches tool calls, manages compaction and cache, fires hooks; this is where a "turn" happens
- `internal/tools/` — all tool implementations (Bash, Read, Write, Edit, Glob, Grep, WebSearch, WebFetch, Memory, Recall, Tasks, Cron, SendMessage, SpawnTeammate, LSP, Skill, Agent, etc.) plus `registry.go` (`Registry`, `DefaultRegistry()`)
- `internal/session/` — session lifecycle (start, persist, reconstruct, share); thin wrapper over `storage.DB`
- `internal/storage/` — SQLite DB wrapper (`db.go`), sessions table (`sessions.go`), memory FTS index (`memory_fts.go`), audit log (`audit.go`); inline migrations via `db.migrate()`
- `internal/config/` — `Settings` struct (JSON), path helpers (`GetPaths()`), project-level config scanning, trust store, snippet expansion
- `internal/bus/` — in-process pub/sub event bus (`bus.go`); canonical event type constants in `events.go` (session, message, stream, tool, auth events)
- `internal/hooks/` — lifecycle hook runner (`hooks.go`); executes shell commands on `PreToolUse`, `PostToolUse`, `SessionStart`, `UserPromptSubmit`, `PreCompact`, `PostCompact`, `CwdChanged`
- `internal/permissions/` — permission rule matching (`rules.go`); evaluates `allow`/`deny` rules against tool names and input patterns; integrates with `config.PermissionRule`
- `internal/prompts/` — system prompt builder (`system.go`); assembles static sections + dynamic boundary; injected once per engine
- `internal/agents/` — agent definition loading (`agents.go`; reads YAML/JSON from `.claudio/agents/`); session→persona promotion (`crystallize.go`)
- `internal/teams/` — team config (`team.go`), team member mailbox (`mailbox.go`), teammate runner (`runner.go`), team templates (`templates.go`); orchestrates multi-agent collaboration
- `internal/tasks/` — background task runtime (`runtime.go`); task types: `local_bash` (shell), `local_agent` (sub-agent), `dream` (background agent); cron store (`cron.go`); task store (`store.go`)
- `internal/plugins/` — plugin registry (`plugins.go`); loads executable plugins from `.claudio/plugins/`; calls `--describe`, `--schema`, `--instructions` flags; exposes as tools via MCP proxy (`proxy.go`)
- `internal/services/memory/` — memory store (`memory.go`); `Entry` types: user, feedback, project, reference; scoped to project/global/agent; persists as markdown files + FTS index in SQLite
- `internal/services/skills/` — skill registry (`loader.go`); loads `.claudio/skills/*.md` from bundled, user, project, and plugin sources; exposed as `Skill` tool
- `internal/services/mcp/` — MCP server manager (`manager.go`); starts/stops MCP servers defined in `settings.json`; registers their tools into the tool registry
- `internal/services/analytics/` — turn/tool-call counters, cost tracking
- `internal/services/compact/` — conversation compaction strategies (full compact, micro-compact, strategic, time-based)
- `internal/services/lsp/` — LSP server manager; starts language servers per file type; provides go-to-definition, references, hover via the `LSP` tool
- `internal/learning/` — instinct store (`learning.go`); stores pattern→response pairs (`Instinct`) learned from sessions; persisted as JSON
- `internal/rules/` — rules registry (`rules.go`); loads markdown rule files from user (`~/.claudio/rules/`) and project (`.claudio/rules/`) directories; injected into system prompt
- `internal/orchestrator/` — multi-phase orchestration (`orchestrator.go`); runs sequential phases each with a named agent type and prompt; used for structured workflows
- `internal/security/` — path and command safety checks (`security.go`, `auditor.go`); `CheckPathAccess` enforces `denyPaths`/`allowPaths`; `Auditor` logs security events
- `internal/tui/` — Bubbletea TUI root (`root.go`); panels, sidebar, vim mode, plan mode, session runtime (`sessionrt.go`), agent/team selectors, keybindings, notifications, docks
- `internal/bridge/` — IPC bridge (`bridge.go`); Unix-socket-based message passing between agent processes (used by teams)
- `internal/git/` — git utilities (`git.go`): find repo root, worktree management
- `internal/auth/` — API key/token resolver; OAuth flow; secure storage backends
- `internal/models/` — model metadata and capability descriptions
- `internal/snippets/` — snippet expansion (e.g. `$ARGUMENTS` substitution in skills/prompts)
- `internal/keybindings/` — TUI key binding configuration
- `internal/ratelimit/` — rate-limiter for LLM API calls
- `internal/server/` — optional HTTP server mode
- `internal/web/` — web UI assets (if applicable)
- `internal/utils/` — shared helpers: cron parsing, path utilities, string utilities

### Key Files

- **App bootstrap**: `internal/app/app.go` — `App` struct + `New()` factory; read this to understand how everything is wired
- **Tool registry**: `internal/tools/registry.go` — `Registry` struct, `DefaultRegistry()`, `Register()`, `Get()`, `All()`
- **Agentic loop**: `internal/query/engine.go` — `Engine.RunWithBlocks()` is the core turn loop; all LLM calls, tool dispatch, compaction, and hook firing happen here
- **System prompt**: `internal/prompts/system.go` — `BuildSystemPrompt()` assembles the full system prompt from static sections
- **CLI entry**: `internal/cli/root.go` — root Cobra command, `PersistentPreRunE` (trust check, config load, app init), `RunE` (TUI vs headless vs single-prompt routing)
- **Config**: `internal/config/config.go` — `Settings` struct with all JSON fields; `GetPaths()` for data directories
- **Session**: `internal/session/session.go` — `Session.Start()`, `Current()`, `Reconstruct()`
- **Storage/DB**: `internal/storage/db.go` — `Open()`, inline `migrate()` (no separate migration files — SQL is embedded in `db.go`)
- **Memory service**: `internal/services/memory/memory.go` — `Entry`, scopes, `ScopedStore.Save()` / `Load()` / `Search()`
- **Memory FTS**: `internal/storage/memory_fts.go` — full-text search index over memory entries
- **Bus events**: `internal/bus/events.go` — all event type string constants
- **Hook lifecycle**: `internal/hooks/hooks.go` — `Manager`, hook event types, shell execution
- **Permission rules**: `internal/permissions/rules.go` — `Match()` function and pattern helpers
- **Agent definitions**: `internal/agents/agents.go` — `AgentDefinition` struct, loader from `.claudio/agents/`
- **Agent crystallize**: `internal/agents/crystallize.go` — promotes a session into a reusable agent persona
- **Team config**: `internal/teams/team.go` — `TeamConfig`, `TeamMember`, `TeammateIdentity`
- **Team runner**: `internal/teams/runner.go` — spawns and manages team member processes
- **Task runtime**: `internal/tasks/runtime.go` — `Runtime`, `TaskType` (shell/agent/dream), task lifecycle
- **Plugin registry**: `internal/plugins/plugins.go` — `Plugin`, `Registry.LoadDir()`, plugin→tool bridging
- **Skills registry**: `internal/services/skills/loader.go` — `Skill`, `Registry`, source discovery
- **MCP manager**: `internal/services/mcp/manager.go` — `Manager`, `ServerState`, start/stop/register tools
- **Learning store**: `internal/learning/learning.go` — `Instinct`, `Store`, pattern-based behavioral memory
- **Rules loader**: `internal/rules/rules.go` — `Rule`, `Registry`, loads user + project markdown rules
- **TUI root**: `internal/tui/root.go` — Bubbletea `Model`, `Update()`, `View()` for the main TUI

### Wiring Flow

```
Startup:
  main() → cli.Execute()
    → rootCmd.PersistentPreRunE: find git root, trust check, load config
    → app.New(projectRoot, settings):
        bus.New() → authstorage → auth.Resolver → storage.Open() (SQLite + migrations)
        → api.NewClient() → register providers (OpenAI/Anthropic/Ollama) + model shortcuts
        → tools.DefaultRegistry() → attach security context to Bash/Read/Write/Edit
        → hooks.LoadManager() → skills.Registry → memory.ScopedStore
        → plugins.Registry.LoadDir() → register plugin tools
        → mcp.Manager.Start() → register MCP server tools
        → teams.Manager, tasks.Runtime, lsp.ServerManager
    → rootCmd.RunE: build system prompt (prompts.BuildSystemPrompt)
        → if TUI: tui.New(app).Start() (Bubbletea)
        → if headless/single-prompt: query.Engine.Run(userMessage)

Request (interactive turn):
  user input → tui sessionrt → query.Engine.RunWithBlocks(blocks)
    → inject memory index (if enabled) + user context
    → loop: api.Client.Chat(messages) → stream response chunks → handler.OnTextDelta
    → if tool_use in response:
        → hooks.Fire(PreToolUse)
        → permissions.Match(toolName, input) → allow/deny/ask
        → registry.Get(toolName).Execute(ctx, input)
        → hooks.Fire(PostToolUse)
        → append tool result to messages, continue loop
    → on StopReason "end_turn": break loop, fire PostTurn hooks

Memory:
  Memory tool → services/memory.ScopedStore.Save()
    → writes ~/.claudio/projects/{slug}/memory/{name}.md (project scope)
    → or ~/.claudio/memory/{name}.md (global scope)
    → updates storage FTS index (memory_fts table in SQLite)
  Memory index injected as a user message at start of each Engine.RunWithBlocks()

Teams:
  SpawnTeammate tool → teams.Manager → tasks.Runtime.Submit(TypeAgent)
    → bridge.Socket IPC for message passing between agents
    → SendMessage tool → mailbox → target agent's message queue
```

### What Lives Where (quick lookup)

- **Adding a new CLI command** → `internal/cli/commands/` (add to `commands.go`) or `internal/cli/` (top-level subcommand)
- **Adding a new tool** → `internal/tools/<toolname>.go` + call `registry.Register(...)` in `tools.DefaultRegistry()` inside `registry.go`
- **Adding a storage migration** → edit `internal/storage/db.go` `migrate()` — SQL is embedded inline, add a new `CREATE TABLE IF NOT EXISTS` or `ALTER TABLE` block
- **Adding a new bus event** → `internal/bus/events.go` — add a new `const` string
- **Adding a new hook event type** → `internal/hooks/hooks.go` — add to the `Event` const block
- **Adding a new agent type** → create `.claudio/agents/<name>.yaml` with `AgentDefinition` fields; loaded automatically by `internal/agents/agents.go`
- **Adding a new skill** → create `.claudio/skills/<name>.md`; loaded automatically by `internal/services/skills/`
- **Adding a new plugin** → drop an executable in `.claudio/plugins/`; must respond to `--describe`, `--schema`, `--instructions`
- **Adding a new MCP server** → add entry to `mcpServers` in `settings.json`; managed by `internal/services/mcp/`
- **Changing system prompt** → `internal/prompts/system.go` — add/modify a `*Section()` function and include it in `BuildSystemPrompt()`
- **Changing permission logic** → `internal/permissions/rules.go`
- **Changing memory behavior** → `internal/services/memory/memory.go` and `internal/storage/memory_fts.go`
- **Changing TUI layout** → `internal/tui/root.go`, `internal/tui/layout.go`, `internal/tui/panels/`
- **Changing model routing** → `settings.json` `modelRouting` map or `internal/api/client.go` `AddModelRoute()`
