# claudio

## Build & Test
- Build: `make build` (outputs to `bin/claudio`) or `go build -o bin/claudio ./cmd/claudio`
- Run dev: `make run` (injects version via ldflags)
- Test: `go test ./...` or `make test`
- Lint: `make lint` (requires golangci-lint)
- Version is injected at build time via `-X github.com/Abraxas-365/claudio/internal/cli.Version=$(VERSION)`

## Module
- Module path: `github.com/Abraxas-365/claudio`
- Requires Go 1.26+

## Architecture
- `cmd/claudio/main.go` — entrypoint only
- `internal/cli/` — Cobra commands; `Version` var lives here
- `internal/tui/` — Bubbletea TUI (lipgloss/bubbles for styling)
- `internal/app/` — core application wiring
- `internal/orchestrator/` — multi-agent orchestration
- `internal/agents/` — agent definitions and crystallization
- `internal/tools/` — all tool implementations; register via `internal/tools/registry.go`
- `internal/storage/` — SQLite persistence (`modernc.org/sqlite`, no CGO)
- `internal/config/` — layered config with migrations; env vars prefixed `CLAUDIO_`
- `internal/bus/` — internal event bus
- `internal/teams/` — multi-agent team coordination with mailbox messaging
- `internal/tasks/` — cron, dream, shell, and agent task scheduling
- `internal/services/` — background services (memory, LSP, MCP, compaction, etc.)

## Key Conventions
- All code lives under `internal/` — no public packages except `cmd/`
- New tools must be registered in `internal/tools/registry.go`
- Config changes may require a migration in `internal/config/migrations/`
- Storage migrations live in `internal/storage/migrations/`
- Event types are defined in `internal/bus/events.go` — add new events there
- Permission rules use pattern syntax: `allow: Bash(git *)`, `deny: Write(*.env)`

## Gotchas
- SQLite uses `modernc.org/sqlite` (pure Go, no CGO required)
- TUI uses Bubbletea's elm-architecture — messages flow through `Update()`, never mutate state directly outside the model
- Config is layered (env > project > local > global > defaults); lists are **appended** across layers, not overridden
- `internal/tui/vim/` implements a full vim state machine — be careful with state transitions when modifying