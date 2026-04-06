# CLAUDIO.md

This file provides guidance to Claudio (github.com/Abraxas-365/claudio) when working with code in this repository.

## Build & Test

- `make build` — build binary (injects version via ldflags); never use plain `go build`
- `make test` — run all tests (`go test ./...`)
- `make lint` — run linter (`golangci-lint run ./...`)
- Run a single test: `go test ./internal/path/to/pkg -run TestName`

## Key Constraints

- No CGO — the project must remain pure Go (this is why we use `modernc.org/sqlite`)
- Never alter existing migration files — only add new ones
- Architectural rules and code style are in @.claudio/rules/project.md
