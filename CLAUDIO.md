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
  - `internal/storage/` — SQLite persistence via `modernc.org/sqlite` (no CGo)
  - `internal/bus/` — internal event bus
  - `internal/config/` — config with migrations, validation, trust
  - `internal/session/` — session management and sharing
  - `internal/api/` — API client and providers

## Patterns
- TUI uses Bubbletea's Elm architecture (Model/Update/View) — all UI state changes go through `Update`
- Events flow through `internal/bus/` — add new event types in `internal/bus/events.go`
- Tools must be registered in `internal/tools/registry.go`
- Config migrations live in `internal/config/migrations/` — add new files there for schema changes
- Storage migrations live in `internal/storage/migrations/`
- Permission rules use pattern syntax: `allow: Bash(git *)`, `deny: Write(*.env)` — see `internal/permissions/rules.go`

## Gotchas
- SQLite uses `modernc.org/sqlite` (pure Go, no CGo required) — do not add a CGo sqlite dependency
- `internal/cli/version.go` holds the `Version` var — it's set only via ldflags at build time, not hardcoded
- Do not import anything from `internal/` across subsystems without checking for circular dependencies (bus, utils, config are safe shared deps)
- `internal/tui/vim/` is a full vim state machine — modifications must handle all modes: normal, insert, visual, operator-pending, registers