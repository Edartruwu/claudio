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
