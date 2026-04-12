# PRD: Caido Claudio Plugin

## Summary

Build a Claudio plugin binary (`~/.claudio/plugins/caido`) that exposes Caido's full feature surface to Claude as lean CLI-style commands. Same capabilities as the community MCP server (`c0tton-fluff/caido-mcp-server`) but as a Claudio native plugin — no MCP overhead, controlled output size, explicit context budget per call.

---

## Problem

- MCP server loads all 34 tool schemas into context at session start, even if unused.
- MCP protocol is persistent stdio JSON-RPC — more moving parts.
- Claudio's native plugin model is simpler: one binary call, one result, you control output size.

---

## Goal

A Go binary that:
1. Responds to Claudio's plugin discovery protocol (`--describe`, `--schema`, `--instructions`)
2. Exposes all Caido capabilities as subcommands via the `args` parameter
3. Uses [`caido-community/sdk-go`](https://github.com/caido-community/sdk-go) for all GraphQL calls
4. Keeps all responses ≤ 2KB by default (configurable), matching MCP server's body limit
5. Ships with a `setup` subcommand that auto-configures auth and validates connection

---

## Architecture

```
~/.claudio/plugins/caido          ← compiled binary, chmod +x
~/.claudio/plugins/caido.json     ← config: url, pat, body_limit

Plugin discovery protocol:
  caido --describe      → one-line description
  caido --schema        → JSON schema for args + input params
  caido --instructions  → pentest system prompt for Claude

Runtime invocation:
  args: "history -f 'req.host.eq:\"target.com\"' -n 20"
  args: "replay --raw - " + stdin: raw HTTP request bytes
  args: "finding-create --request-id abc123 --title 'IDOR in /api/users'"
```

### Config file (`~/.claudio/plugins/caido.json`)

```json
{
  "url": "http://127.0.0.1:8080",
  "pat": "your-personal-access-token",
  "body_limit": 2000
}
```

Config precedence: env vars override file.
- `CAIDO_URL` overrides `url`
- `CAIDO_PAT` overrides `pat`
- `CAIDO_BODY_LIMIT` overrides `body_limit`

---

## Commands (34 total — parity with MCP)

### Setup & Auth

| Command | Description |
|---|---|
| `setup` | Interactive setup: validate Caido URL, save PAT, test connection |
| `status` | Print instance info + auth status |

### HTTP History

| Command | Flags | Description |
|---|---|---|
| `history` | `-f HTTPQL`, `-n N`, `--after CURSOR` | List requests. Default limit 20, max 100 |
| `request <id>` | `--headers`, `--body`, `--offset N`, `--limit N` | Full request+response details |

### Replay

| Command | Flags | Description |
|---|---|---|
| `replay` | `--host`, `--port`, `--tls`, `--session-id`; raw HTTP on stdin | Send request via Replay, poll response |
| `replay-sessions` | | List all Replay sessions |
| `replay-entry <id>` | `--offset N`, `--limit N` | Get replay entry with response |

### Automate (Fuzzing)

| Command | Flags | Description |
|---|---|---|
| `automate-sessions` | | List fuzzing sessions |
| `automate-session <id>` | | Session details + entry list |
| `automate-entry <id>` | `-n N`, `--after CURSOR` | Fuzz results and payloads |
| `automate-control` | `--action start|pause|resume|cancel`, `--session <id>`, `--task <id>` | Control fuzzing task |

### Findings

| Command | Flags | Description |
|---|---|---|
| `findings` | | List all security findings |
| `finding-create` | `--request-id`, `--title`, `--description` | Create finding linked to request |
| `findings-delete` | `--ids`, `--reporter` | Delete findings |
| `findings-export` | `--ids`, `--reporter` | Export findings for reporting |

### Sitemap & Scope

| Command | Flags | Description |
|---|---|---|
| `sitemap` | | Browse discovered endpoint hierarchy |
| `scopes` | | List target scopes |
| `scope-create` | `--name`, `--allow`, `--deny` | Create scope with allow/deny lists |

### Projects

| Command | Flags | Description |
|---|---|---|
| `projects` | | List projects, mark current |
| `project-select <id>` | | Switch active project |

### Workflows

| Command | Flags | Description |
|---|---|---|
| `workflows` | | List automation workflows |
| `workflow-run` | `--id`, `--type active|convert`, `--request-id`, `--input` | Execute workflow |
| `workflow-toggle` | `--id`, `--enabled true|false` | Enable/disable workflow |

### Tamper (Match & Replace)

| Command | Flags | Description |
|---|---|---|
| `tamper` | | List all tamper rule collections |
| `tamper-create` | `--collection-id`, `--name`, `--condition`, `--sources` | Create tamper rule |
| `tamper-toggle` | `--id`, `--enabled true|false` | Enable/disable rule |
| `tamper-delete` | `--id` | Delete tamper rule |

### Intercept

| Command | Flags | Description |
|---|---|---|
| `intercept-status` | | Get intercept state (PAUSED/RUNNING) |
| `intercept-control` | `--action pause|resume` | Pause or resume intercept |
| `intercept-list` | `-f HTTPQL`, `-n N`, `--after CURSOR` | List queued intercept entries |
| `intercept-forward <id>` | `--raw` (base64 modified request, optional) | Forward intercepted request |
| `intercept-drop <id>` | | Drop intercepted request |

### Environments & Filters

| Command | Flags | Description |
|---|---|---|
| `envs` | | List environments and variables |
| `env-select <id>` | | Switch active environment |
| `filters` | | List saved HTTPQL filter presets |

---

## Plugin Discovery Protocol

### `--describe`
```
Caido proxy control — query HTTP history, replay requests, manage findings, intercept traffic
```

### `--schema`
Full JSON Schema for `args` (string, required) and `input` (string, optional stdin for raw HTTP).

```json
{
  "type": "object",
  "required": ["args"],
  "properties": {
    "args": {
      "type": "string",
      "description": "Subcommand and flags. Examples: 'history -f req.host.eq:\"target.com\"', 'replay --host target.com', 'finding-create --request-id abc --title IDOR'"
    },
    "input": {
      "type": "string",
      "description": "Raw HTTP request bytes for replay/intercept-forward commands"
    }
  }
}
```

### `--instructions`
System prompt injected into Claude's context once at session start:

```markdown
## Caido Pentest Plugin

You have access to a live Caido proxy instance. Use it to analyze captured traffic and actively test for vulnerabilities.

### Pentest loop
1. `history -f <HTTPQL>` to find interesting endpoints
2. `request <id>` to inspect full request/response
3. `replay` with crafted payload via stdin to test
4. Analyze response — status code, body, headers, timing
5. `finding-create` when vulnerability confirmed

### HTTPQL quick reference
- `req.host.eq:"target.com"` — filter by host
- `req.method.eq:"POST"` — filter by method
- `res.status.eq:500` — filter by response code
- `req.path.cont:"/api/admin"` — path contains string

### Vulnerability patterns to test
- **IDOR**: Replay request, swap `id` parameter to another user's ID
- **SQLi**: Inject `'`, `1'--`, `1 OR 1=1` into string parameters
- **Path traversal**: Try `../../etc/passwd` in file path params
- **Auth bypass**: Replay with missing/empty Authorization header
- **Mass assignment**: Add extra fields like `{"role":"admin"}` to POST bodies
- **JWT `alg:none`**: Replace JWT, set alg to none, strip signature

### Security
Never store or log credentials found in traffic. Report findings with `finding-create`.
```

---

## Setup Command Flow

`caido setup` does:

1. Check `CAIDO_URL` / prompt for URL (default `http://127.0.0.1:8080`)
2. Test reachability — GET `{url}/` — report error if Caido not running
3. Print instructions: "Open Caido → Settings → Developer → Personal Access Tokens → New"
4. Prompt for PAT
5. Test PAT — call `instance` query via GraphQL
6. Save `~/.claudio/plugins/caido.json` with url + pat
7. Print: "Connected to Caido vX.X.X. Plugin ready."

---

## Security Requirements

- Redact `Authorization`, `Cookie`, `Set-Cookie`, `X-Api-Key` headers in all output (same as MCP server)
- PAT stored with `chmod 0600` on config file
- PAT never printed in output or error messages
- Body limit default 2KB — prevents context flooding from large responses
- All string inputs length-validated before GraphQL call

---

## Output Format

All commands output JSON to stdout. Errors to stderr, non-zero exit.

```json
// history
{"requests": [...], "next_cursor": "abc123", "total": 47}

// request
{"id": "abc", "method": "POST", "host": "target.com", "path": "/api/users",
 "request_headers": {...}, "request_body": "...", "response_status": 200, ...}

// finding-create
{"id": "xyz", "title": "IDOR in /api/users", "request_id": "abc"}

// error
{"error": "not authenticated: run 'caido setup' first"}
```

---

## Implementation Plan

### Phase 1 — Skeleton
- `main.go`: flag parsing for `--describe`, `--schema`, `--instructions`, then dispatch to subcommand
- `config.go`: load `caido.json`, env var override, default URL
- `client.go`: wrap `caido-community/sdk-go` with auth header injection
- `setup.go`: interactive setup command

### Phase 2 — Core commands
- `history.go`: `history`, `request`
- `replay.go`: `replay`, `replay-sessions`, `replay-entry`
- `findings.go`: `findings`, `finding-create`, `findings-delete`, `findings-export`

### Phase 3 — Full parity
- `automate.go`: all automate commands
- `intercept.go`: all intercept commands
- `scope.go`, `sitemap.go`, `projects.go`
- `workflows.go`, `tamper.go`, `envs.go`, `filters.go`

### Phase 4 — Install script
- `install.sh`: download binary for OS/arch, place at `~/.claudio/plugins/caido`, chmod +x, run `caido setup`

---

## Install Script UX

```bash
curl -fsSL https://raw.githubusercontent.com/USER/caido-claudio-plugin/main/install.sh | bash
```

Script:
1. Detect OS + arch
2. Download release binary from GitHub releases
3. Place at `~/.claudio/plugins/caido`
4. `chmod +x`
5. Run `caido setup`
6. Print "Done. Restart Claudio to activate."

---

## Dependencies

| Dep | Purpose |
|---|---|
| `github.com/caido-community/sdk-go` | GraphQL client for Caido local API |
| `github.com/spf13/cobra` | Subcommand parsing |
| Standard library only otherwise | Keep binary small |

---

## Acceptance Criteria

- [ ] Binary placed at `~/.claudio/plugins/caido` responds correctly to `--describe`, `--schema`, `--instructions`
- [ ] `caido setup` walks user through auth and saves config
- [ ] All 34 MCP-equivalent commands implemented
- [ ] Response bodies capped at 2KB default
- [ ] Sensitive headers redacted in all output
- [ ] `install.sh` works on macOS arm64, macOS amd64, Linux amd64, Linux arm64
- [ ] `go build ./...` passes with no CGO
- [ ] Plugin activates in Claudio after restart — visible in tool list as `plugin_caido`
