# CLAUDIO.md

This file provides guidance to Claudio (github.com/Abraxas-365/claudio) when working with code in this repository.
Env(db credentials, server port, etc) variables in Makefile.

## Build & Test

- `make build` — build binary (injects version via ldflags); never use plain `go build`
- `make test` — run all tests (`go test ./...`)
- `make lint` — run linter (`golangci-lint run ./...`)
- Run a single test: `go test ./internal/path/to/pkg -run TestName`

## Key Constraints

- No CGO — the project must remain pure Go (this is why we use `modernc.org/sqlite`)
- Never alter existing migration SQL in `internal/storage/db.go` — append new versioned entries only
- Architectural rules and code style are in @.claudio/rules/project.md

## Web UI

- Edit HTML templates in `internal/web/` — never edit `internal/web/static/vendor/tailwind.min.css` directly
- Run `make dev` to develop the web UI (starts CSS watcher + server together); requires npm

## Conventions

- Commit messages follow Conventional Commits: `feat:`, `fix:`, `chore:`, `docs:`, etc.
