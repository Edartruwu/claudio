# Architecture & Code Style

## Package Map

The codebase lives entirely under `internal/`. Key areas:

| Package | Role |
|---|---|
| `cmd/claudio` | Entry point — calls `cli.Execute()` with version from ldflags |
| `internal/cli` | Cobra commands; `Version` var injected at build time |
| `internal/app` | Dependency injection / wiring |
| `internal/tools` | All tool implementations (Bash, Read, Write, Edit, Agent, etc.) |
| `internal/tui` | BubbleTea TUI — 15+ subpackages, ~18K LOC |
| `internal/web` | Go `html/template` web UI + Tailwind CSS |
| `internal/storage` | SQLite access layer; 22 embedded versioned migrations in `db.go` |
| `internal/services` | 12 focused services (memory, analytics, compact, lsp, mcp, …) |
| `internal/agents` | Agent orchestration & spawning |
| `internal/teams` | Multi-agent team management |
| `internal/bus` | Event bus — decoupled inter-component messaging |
| `internal/config` | Hierarchical settings; encrypted token storage |
| `internal/security` | Path/command validation, audit logging |
| `internal/hooks` | Hook system (pre/post tool events) |
| `internal/permissions` | Permission enforcement |

## Key Patterns

- **Event bus:** Components communicate via `internal/bus` — prefer publishing events over direct calls across subsystems.
- **Services layer:** Each service in `internal/services/` is a focused, injectable struct. Wire new services through `internal/app`.
- **Embedded migrations:** All schema changes are versioned SQL strings appended to the migration list in `internal/storage/db.go`. The runner is idempotent. Never edit existing entries; only append.
- **Pure Go:** No CGO anywhere. SQLite via `modernc.org/sqlite`. Any dependency that requires CGO is off-limits.
- **Single binary:** The binary must remain self-contained. No runtime dependencies on external processes (except optionally npm for dev CSS builds).

## TUI Architecture

- BubbleTea (Elm-style): `Model` → `Update` → `View`. Keep side effects in `Cmd` returns.
- Vim keybindings are in `internal/tui/vim/` (~1.3K LOC) — extend there, not inline.
- Styles/colors are centralized in the styles subpackage — don't hard-code lipgloss styles elsewhere.

## Web UI

- Templates in `internal/web/` use standard `html/template`.
- CSS is Tailwind, vendored at `internal/web/static/vendor/tailwind.min.css`. Regenerate via `make css` or `make dev` — never edit the vendored file directly.

## Testing

- Test function naming: `TestFeature_Case` (e.g. `TestValidate_EmptyFilePath`).
- Tests live alongside source (`foo_test.go` next to `foo.go`).
- No table-driven tests by default — individual `TestX_Y` functions per case is the established pattern.

## Git Worktrees

- Agent teams run in isolated git worktrees under `.claudio-worktrees/` (gitignored).
- Each worktree starts from the latest commit on main — never assume a worktree sees another's uncommitted changes.
