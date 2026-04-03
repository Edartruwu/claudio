# claudio

## Build & Test
- Build: `make build` (outputs to `bin/claudio`) or `go build -o bin/claudio ./cmd/claudio`
- Run dev: `go run -ldflags "-s -w -X github.com/Abraxas-365/claudio/internal/cli.Version=dev" ./cmd/claudio`
- Test: `go test ./...`
- Version is injected via ldflags: `-X github.com/Abraxas-365/claudio/internal/cli.Version=$(VERSION)`

## Module
- Module path: `github.com/Abraxas-365/claudio`
- Requires Go 1.26+

## Architecture
- Entry point: `cmd/claudio/main.go`
- All non-entry code lives under `internal/` — never import internal packages from outside this module
- Key subsystems:
  - `internal/tui/` — Bubbletea TUI (lipgloss/bubbles for styling)
  - `internal/agents/` — agent orchestration and crystallization
  - `internal/teams/` — multi-agent team coordination (mailbox, runner)
  - `internal/tasks/` — cron, dream, shell, agent tasks
  - `internal/tools/` — all tool implementations (bash, file ops, LSP, MCP, etc.)
  - `internal/tools/outputfilter/` — RTK-style output filtering to reduce token usage on command outputs (git, go, cargo, npm, etc.)
  - `internal/tools/readcache/` — deduplicates file reads by caching content; returns stub if file unchanged
  - `internal/storage/` — SQLite persistence via `modernc.org/sqlite` (no CGo)
  - `internal/bus/` — internal event bus
  - `internal/config/` — config with migrations, validation, trust
  - `internal/session/` — session management and sharing
  - `internal/api/` — API client and providers
  - `internal/snippets/` — token-efficient snippet expansion; `~name(args)` patterns in generated code expand via templates (lang-filtered)
  - `internal/learning/` — instinct learning; extracts patterns from sessions and replays them in future sessions to avoid repeated mistakes
  - `internal/services/memory/` — persistent memory across sessions (markdown files in `~/.claudio/memory/`); scoped: agent > project > global priority
  - `internal/services/skills/` — skill registry; loads bundled, user (`~/.claudio/skills/`), and project (`.claudio/skills/`) skill definitions
  - `internal/services/compact/` — smart conversation compaction; strategies: auto (token threshold), strategic (phase boundaries), manual
  - `internal/services/analytics/` — token usage tracking, cost calculation, and budget enforcement per session
  - `internal/services/toolcache/` — offloads oversized tool results (>50KB) to disk to reduce tokens per turn
  - `internal/services/difftracker/` — tracks file changes (git diffs) per conversation turn
  - `internal/services/cachetracker/` — monitors prompt cache misses and infers causes (new message, system change)
  - `internal/services/lsp/` — LSP server lifecycle management for code intelligence (go-to-def, references, hover)
  - `internal/services/mcp/` — MCP server lifecycle manager; lazy-starts servers, tracks status, idle shutdown
  - `internal/services/notifications/` — native OS desktop notifications (macOS/Linux)
  - `internal/services/plugins/` — bridges plugin executables to the tool system as `plugin_<name>` tools
  - `internal/plugins/` — plugin discovery; finds executables in `~/.claudio/plugins/`
  - `internal/hooks/` — lifecycle event hooks (PreToolUse, PostToolUse, SessionStart, etc.); runs shell commands at each stage
  - `internal/query/` — query engine; core agentic loop that streams API responses, dispatches tools, handles approvals
  - `internal/orchestrator/` — multi-phase agent workflows with sequential/parallel phase execution
  - `internal/bridge/` — cross-session communication via Unix domain sockets for parallel agents/worktrees
  - `internal/security/` — sandbox (deny-by-default paths/commands) and audit logging of all tool executions to SQLite
  - `internal/auth/` — multi-source auth resolution (env vars, OAuth, keychain, apiKeyHelper) with secure token storage
  - `internal/server/` — headless HTTP API (`--headless` flag) for IDE integration and remote access
  - `internal/git/` — git helpers
  - `internal/rules/` — rule loading and evaluation
  - `internal/keybindings/` — keybinding configuration

## Patterns
- TUI uses Bubbletea's Elm architecture (Model/Update/View) — all UI state changes go through `Update`
- Events flow through `internal/bus/` — add new event types in `internal/bus/events.go`
- Tools must be registered in `internal/tools/registry.go`
- Config migrations live in `internal/config/migrations/` — add new files there for schema changes
- Storage migrations live in `internal/storage/migrations/`
- Permission rules use pattern syntax: `allow: Bash(git *)`, `deny: Write(*.env)` — see `internal/permissions/rules.go`
- Snippets use `~name(args)` syntax in code; definitions live in config as `SnippetDef` with Go `text/template` bodies; lang-filtered by file extension
- Skills are loaded from three sources in priority order: project (`.claudio/skills/`), user (`~/.claudio/skills/`), bundled — invoked with `/skill-name`
- Memory entries are markdown files with YAML frontmatter; scoped stores layer agent > project > global with fallback reads
- Hooks are configured per-event in config; they receive JSON on stdin with tool name, input, and session context
- Plugins are executable files in `~/.claudio/plugins/`; they get registered as tools prefixed with `plugin_` and receive args via stdin/JSON
- Output filters are per-command-type (git, build, generic); they strip noise from tool output to save tokens (RTK-style)
- The query engine is the core agentic loop — it streams API responses, dispatches tool calls, handles approval flows, and fires hook events
- Compaction has three strategies: `auto` (80% token threshold), `strategic` (phase boundaries + tool count), `manual` (user-triggered only)

## Gotchas
- SQLite uses `modernc.org/sqlite` (pure Go, no CGo required) — do not add a CGo sqlite dependency
- `internal/cli/version.go` holds the `Version` var — it's set only via ldflags at build time, not hardcoded
- Do not import anything from `internal/` across subsystems without checking for circular dependencies (bus, utils, config are safe shared deps)
- `internal/tui/vim/` is a full vim state machine — modifications must handle all modes: normal, insert, visual, operator-pending, registers
- Snippet expansion happens at code-generation time, not at runtime — the `~name()` markers should never appear in final output
- Memory scopes (agent, project, global) are distinct directories — do not mix reads/writes across scopes without going through `ScopedStore`
- The bridge uses Unix domain sockets in a temp dir — socket cleanup is critical on session end to avoid stale sockets
- Security sandbox deny-lists (`DefaultDenyPaths`, `DefaultDenyCommands`) are always active — explicit overrides needed to bypass
- Tool cache threshold is 50KB by default — oversized results get a disk reference placeholder, not inline content
- Instincts (learned patterns) decay: confidence drops if not used — prune low-confidence instincts periodically