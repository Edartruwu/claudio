# RTK Commands Complete Reference Matrix

## Quick Command Lookup by Category

### 📁 File Operations

| Command | Arguments | Output | Savings | Features |
|---------|-----------|--------|---------|----------|
| `rtk ls` | `[args...]` | Tree view | -80% | Supports all native ls flags (-l, -a, -h, -R) |
| `rtk tree` | `[args...]` | Compact tree | -80% | Supports all native tree flags (-L, -d, -a) |
| `rtk read <file>` | `-l none\|minimal\|aggressive`, `--max-lines N`, `--tail-lines N`, `-n` | Smart filtered | -70% | 3 filter levels, line truncation |
| `rtk find <pattern> <path>` | Supports native find flags | Grouped by dir | -80% | Auto-groups by parent directory |
| `rtk grep <pattern> <path>` | `-l`, `-t <type>`, `-n`, extra ripgrep args | Grouped by file | -80% | Max results configurable, context mode |
| `rtk diff <file1> <file2>` | `[file2]` or stdin | Condensed | -75% | Ultra-compressed diff view |
| `rtk json <file>` | `-d depth`, `--schema` | Structure only | -90% | Schema mode strips all values |
| `rtk wc` | `[files...]` | Compact | -80% | Strips path padding |
| `rtk log <file>` | stdin or file | Deduplicated | -85% | Repeated lines counted |
| `rtk smart <file>` | `-m model`, `--force-download` | 2-line summary | -95% | Heuristic code summary |
| `rtk summary <cmd>` | Command to run | 2-line summary | -95% | Runs command, generates summary |

---

### 🔗 Git & Version Control

| Command | Arguments | Output | Savings | Features |
|---------|-----------|--------|---------|----------|
| `rtk git status` | `-s` (porcelain), standard flags | Compact | -80% | Aggregates file counts |
| `rtk git log` | `-n`, `--oneline`, `--graph`, all native flags | 1 line/commit | -80% | Supports all git log options |
| `rtk git diff` | `--cached`, `--stat`, all git flags | Condensed | -75% | Counts changed files/lines |
| `rtk git show` | commit flags | Summary + diff | -75% | Commit info + stat + diff |
| `rtk git add` | `[files...]`, flags | "ok" | -92% | Single-word result |
| `rtk git commit` | `-m "msg"`, `--amend`, flags | "ok <hash>" | -92% | Shows commit hash |
| `rtk git push` | remote, branch, flags | "ok <branch>" | -92% | Shows target branch |
| `rtk git pull` | remote, branch, flags | "ok <stats>" | -92% | Shows file/line stats |
| `rtk git branch` | `-d`, `-D`, `-m`, flags | Compact list | -80% | Shows current branch marker |
| `rtk git stash` | `list`, `pop`, `drop`, flags | Condensed | -85% | Applies to all stash subcommands |
| `rtk git merge` | branch, flags | Result | -85% | Fast-forward detection |
| `rtk git rebase` | args, flags | Condensed | -85% | Progress simplification |
| `rtk gh pr list` | Service-specific | Compact | -80% | Owner/title/status only |
| `rtk gh pr view <n>` | PR number | Summary + checks | -75% | Check status aggregated |
| `rtk gh issue list` | Service-specific | Compact | -80% | Title/status/author |
| `rtk gh run list` | Repo-specific | Run status | -80% | Status summary |
| `rtk gt <subcommand>` | Graphite args | Compact | -80% | Stacked PR operations |

---

### 🦀 Rust Ecosystem

| Command | Arguments | Output | Savings | Features |
|---------|-----------|--------|---------|----------|
| `rtk cargo build` | `--release`, features, etc | Build summary | -80% | Progress → summary |
| `rtk cargo test` | `--lib`, `--doc`, `-- <args>` | Failures only | -90% | Shows failure names/locations |
| `rtk cargo clippy` | `--all-targets`, features, etc | Grouped violations | -80% | Groups by lint rule |
| `rtk cargo check` | Features, etc | Errors grouped | -80% | Groups errors by file |
| `rtk cargo fmt` | `--check`, `--all`, etc | File list | -85% | Shows files needing format |
| `rtk cargo run` | `--`, args to pass | Output filtered | -70% | Strips boilerplate |
| `rtk cargo doc` | `--open`, features, etc | Summary | -80% | Build completion info |
| `rtk cargo outdated` | `-R`, `--root-deps`, etc | Compact list | -80% | Current → latest versions |

---

### 🟨 JavaScript / TypeScript / Node

| Command | Arguments | Output | Savings | Features |
|---------|-----------|--------|---------|----------|
| `rtk npm run <script>` | Script name + args | Boilerplate stripped | -70% | Auto-detects npm/yarn/pnpm |
| `rtk npm list` | `--depth N`, `--prod`, etc | Compact tree | -80% | Nested dependency view |
| `rtk npm outdated` | Version range filters | Upgrade paths | -80% | Current → latest |
| `rtk npm install` | Package names, flags | Summary | -85% | Progress removed |
| `rtk pnpm list` | `--depth N`, `--prod`, etc | Compact tree | -80% | Auto-detects pnpm |
| `rtk pnpm install` | Package names, flags | Summary | -85% | Installation summary |
| `rtk vitest run` | `--reporter json`, file patterns | Failures only | -90% | Test summary + failures |
| `rtk vitest watch` | File patterns | Watch summary | -80% | Real-time update format |
| `rtk playwright test` | File patterns, tags, etc | E2E results | -90% | Browser results summary |
| `rtk tsc` | `--noEmit`, `--strict`, etc | Grouped errors | -80% | Groups by file/rule |
| `rtk lint` | File patterns, flags | Rule violations | -80% | Groups by rule/file |
| `rtk prettier --check` | Path patterns | Files needing format | -80% | Only files listed |
| `rtk prettier --write` | Path patterns | Modified count | -85% | Summary only |
| `rtk format` | Auto-detects formatter | Unified output | -80% | prettier/black/ruff |
| `rtk prisma generate` | Schema flags | Clean output | -95% | Strips ASCII art |
| `rtk prisma db push` | `--skip-generate`, etc | Push summary | -85% | Changes applied |
| `rtk next build` | `--profile`, etc | Build summary | -80% | Webpack output compressed |
| `rtk npx <tool>` | Tool + args | Intelligent routing | Varies | Routes to specialized filters |

---

### 🐍 Python Ecosystem

| Command | Arguments | Output | Savings | Features |
|---------|-----------|--------|---------|----------|
| `rtk pytest` | `--tb short`, file patterns, markers | Failures only | -90% | Summary + failure details |
| `rtk pytest --cov` | Coverage flags | Coverage summary | -85% | % per file |
| `rtk ruff check` | File patterns, rules | JSON, grouped | -80% | Violations by rule |
| `rtk ruff format --check` | File patterns | Files needing format | -85% | Compact list |
| `rtk mypy` | Type check args | Type errors grouped | -80% | Groups by file/error |
| `rtk pip list` | `--outdated`, `--user`, etc | Package list | -80% | Auto-detects uv |
| `rtk pip outdated` | Version filters | Upgradeable pkgs | -80% | Current → latest |
| `rtk pip install` | Package specs | Summary | -85% | Progress removed |

---

### 💎 Ruby Ecosystem

| Command | Arguments | Output | Savings | Features |
|---------|-----------|--------|---------|----------|
| `rtk rake test` | `TEST=file`, spec filters | Test summary | -90% | Minitest format compact |
| `rtk rspec` | File patterns, tags, etc | Test results | -60-80% | JSON format, failures highlighted |
| `rtk rubocop` | `--auto-correct`, files, etc | Violations grouped | -80% | JSON format, by rule |
| `rtk bundle install` | Gemfile specs | Summary | -85% | Strips "Using" lines |

---

### 🐹 Go Ecosystem

| Command | Arguments | Output | Savings | Features |
|---------|-----------|--------|---------|----------|
| `rtk go test` | `./...`, `-v`, `-cover`, etc | Test results | -90% | NDJSON format |
| `rtk go test ./...` | Multiple packages | Aggregated | -90% | Package summaries |
| `rtk go build` | Package paths | Build output | -80% | Error summarization |
| `rtk golangci-lint run` | File paths, rules | JSON violations | -85% | Groups by linter |
| `rtk go mod tidy` | Flags | Summary | -90% | Module cleanup summary |
| `rtk go mod download` | Module specs | Download status | -85% | Progress removed |

---

### 🐙 .NET Ecosystem

| Command | Arguments | Output | Savings | Features |
|---------|-----------|--------|---------|----------|
| `rtk dotnet build` | Project file, config | Build summary | -80% | Progress → summary |
| `rtk dotnet test` | Project/solution | Test failures | -90% | Failures only |
| `rtk dotnet restore` | Project specs | Restore summary | -85% | Progress removed |
| `rtk dotnet format` | Check/write mode | Format report | -80% | Files changed |
| `rtk dotnet clean` | Target cleanup | Cleanup summary | -90% | Simple result |

---

### ☁️ AWS & Cloud Services

| Command | Arguments | Output | Savings | Features |
|---------|-----------|--------|---------|----------|
| `rtk aws sts get-caller-identity` | Default | One-line identity | -90% | Account/user/ARN |
| `rtk aws ec2 describe-instances` | Filters | Instance list | -85% | ID/type/state/IP |
| `rtk aws s3 ls` | Bucket path | Bucket list | -80% | With tee recovery |
| `rtk aws s3 ls s3://bucket` | Path filters | Object list | -80% | Truncated with recovery |
| `rtk aws lambda list-functions` | Region filters | Function list | -80% | Name/runtime/memory |
| `rtk aws dynamodb scan` | Table specs | Item list | -80% | Type unwrapped |
| `rtk aws logs get-log-events` | Log group/stream | Timestamped msgs | -85% | Msg only |
| `rtk aws cloudformation describe-stack-events` | Stack ID | Event summary | -80% | Failures first |
| `rtk aws iam list-roles` | Filters | Role list | -80% | Policy docs stripped |
| `rtk docker ps` | Container flags | Container list | -80% | ID/image/status |
| `rtk docker images` | Image flags | Image list | -80% | ID/repo/size |
| `rtk docker logs <container>` | Tail, follow, etc | Deduplicated logs | -85% | Repeated lines counted |
| `rtk docker compose ps` | Service names | Service status | -80% | Service summary |
| `rtk kubectl pods` | Namespace, selector | Pod list | -80% | Name/status/age |
| `rtk kubectl logs <pod>` | Container name | Deduplicated logs | -85% | Repeated counted |
| `rtk kubectl services` | Namespace, selector | Service list | -80% | IP/port/type |
| `rtk curl <url>` | URL + curl flags | JSON/schema | -85% | Auto-JSON detection |
| `rtk wget <url>` | URL, `-O` output | Download | -90% | Progress bars stripped |
| `rtk psql` | Connection string | Query results | -80% | Table borders stripped |

---

### 📊 Analytics & Meta Commands

| Command | Arguments | Output | Savings | Features |
|---------|-----------|--------|---------|----------|
| `rtk gain` | No args | Summary stats | N/A | All-time totals |
| `rtk gain --graph` | No args | ASCII graph | N/A | Last 30 days |
| `rtk gain --history` | `-H` flag | Recent commands | N/A | Last 50 commands |
| `rtk gain --daily` | No args | Day breakdown | N/A | Daily totals |
| `rtk gain --weekly` | No args | Weekly breakdown | N/A | Weekly totals |
| `rtk gain --monthly` | No args | Monthly breakdown | N/A | Monthly totals |
| `rtk gain --all` | Combines all | All metrics | N/A | Comprehensive view |
| `rtk gain --format json` | Output format | JSON export | N/A | For dashboards |
| `rtk gain --quota` | Tier (pro/5x/20x) | Quota savings | N/A | Subscription model |
| `rtk discover` | No args | Missed saves | N/A | Current project only |
| `rtk discover --all` | No args | All projects | N/A | All projects scanned |
| `rtk discover --since <days>` | Days number | Scoped timeframe | N/A | Last N days |
| `rtk session` | No args | Adoption metrics | N/A | Session-level stats |
| `rtk learn` | No args | CLI corrections | N/A | Error patterns |
| `rtk learn --write-rules` | Write flag | Generates .md | N/A | Creates .claude/rules/ |
| `rtk cc-economics` | No args | Spending analysis | N/A | Claude Code spending |
| `rtk cc-economics --daily` | Breakdown flag | Daily breakdown | N/A | By day |

---

### 🔧 Configuration & System

| Command | Arguments | Output | Savings | Features |
|---------|-----------|--------|---------|----------|
| `rtk init` | `-g`, `--agent <target>`, `--gemini`, `--codex`, etc | Installation | N/A | Hook installation (6 agents) |
| `rtk init -g` | Global flag | Default install | N/A | Claude Code + settings patch |
| `rtk init --agent cursor` | Agent target | Cursor hook | N/A | Cursor-specific |
| `rtk init --agent windsurf` | Agent target | Windsurf rules | N/A | Windsurf-specific |
| `rtk init --agent cline` | Agent target | Cline rules | N/A | Cline-specific |
| `rtk init --gemini` | Gemini flag | Gemini processor | N/A | Gemini CLI |
| `rtk init --codex` | Codex flag | Codex setup | N/A | Via Claudio |
| `rtk config --create` | Create flag | Config file | N/A | Default TOML |
| `rtk env` | `-f <name>`, `--show-all` | Env vars | -80% | Sensitive masked |
| `rtk env -f PATH` | Filter flag | PATH only | -95% | Filtered output |
| `rtk env -f AWS` | Filter flag | AWS vars only | -95% | AWS-specific |
| `rtk deps` | `[path]` | Dependencies | -90% | Project summary |
| `rtk proxy <cmd>` | Raw command | Unfiltered | 0% | Passthrough + track |
| `rtk trust` | List flag | Trusted projects | N/A | Trust management |
| `rtk trust --list` | List projects | All trusted | N/A | List view |
| `rtk untrust` | No args | Revoke trust | N/A | Current directory |
| `rtk verify` | `--filter name`, `--require-all` | Hook verification | N/A | Integrity check |
| `rtk rewrite <cmd>` | Raw command | Rewritten cmd | N/A | Single source of truth |
| `rtk hook gemini` | Stdin JSON | Processed output | N/A | Gemini processor |
| `rtk hook copilot` | Stdin JSON | Processed output | N/A | Copilot processor |
| `rtk hook-audit` | `-s <days>` | Hook metrics | N/A | Usage audit |

---

### ⚙️ Generic Runners (Fallback Mode)

| Command | Arguments | Output | Savings | Features |
|---------|-----------|--------|---------|----------|
| `rtk err <cmd>` | Command to run | Errors/warnings | Varies | Generic error filter |
| `rtk test <cmd>` | Test command | Test failures | -90% | Generic test filter |

---

## Global Flags

```bash
-u, --ultra-compact           # ASCII icons, inline format (Level 2 optimizations)
-v, --verbose                 # Increase verbosity (-v, -vv, -vvv)
--skip-env                     # Set SKIP_ENV_VALIDATION=1 for child processes
                              # (Next.js, tsc, lint, prisma)
```

---

## Command Argument Types

### Common Patterns

| Pattern | Means | Examples |
|---------|-------|----------|
| `<arg>` | Required positional | `rtk read myfile.txt` |
| `[arg]` | Optional positional | `rtk ls [path]` |
| `[args...]` | Zero or more args | `rtk find . -name "*.rs"` |
| `[trailing_var_arg]` | All remaining args | `rtk cargo test -- --nocapture` |

### Special Handling

- **Git global options** prepended: `-C`, `-c`, `--git-dir`, `--work-tree`, `--no-pager`, etc.
- **Native find/tree/ls flags** fully supported via `allow_hyphen_values`
- **Double-dash** (`--`) preserved for test argument separation (cargo test -- args)
- **Environment variables** in commands preserved and re-prepended after rewrite
- **Redirects** (`2>&1`, `>/dev/null`) stripped, processed, re-appended

---

## Token Savings Benchmark

### Per-Command Typical Savings

| Operation | Frequency | Std Tokens | RTK Tokens | Savings |
|-----------|-----------|-----------|-----------|---------|
| `ls` / `tree` | 10x | 2,000 | 400 | **-80%** |
| `cat` / `read` | 20x | 40,000 | 12,000 | **-70%** |
| `grep` / `rg` | 8x | 16,000 | 3,200 | **-80%** |
| `git status` | 10x | 3,000 | 600 | **-80%** |
| `git diff` | 5x | 10,000 | 2,500 | **-75%** |
| `git log` | 5x | 2,500 | 500 | **-80%** |
| `git add/commit/push` | 8x | 1,600 | 120 | **-92%** |
| `cargo test` | 5x | 25,000 | 2,500 | **-90%** |
| `pytest` | 4x | 8,000 | 800 | **-90%** |
| `docker ps` | 3x | 900 | 180 | **-80%** |
| **30-min session total** | | **~118K** | **~23.9K** | **-80%** |

---

## Command Routing Decision Tree

```
User Input Command
        ↓
[Main Router - main.rs]
        ↓
    ┌───┴───┬───┬───┬───┬───┬───┬───┐
    ↓       ↓   ↓   ↓   ↓   ↓   ↓   ↓
  System  Git Rust JS  Py Ruby Go .NET Cloud
  Files   VCS       TS      
   (8)    (8)  (4) (10) (5) (3) (4) (4)  (5)
        
Each has:
├─ TOML filter (if available)
├─ Command-specific Rust filter
├─ Token tracking
└─ Graceful fallback (raw command)
```

---

## Filter Coverage by Ecosystem

### System (8 filters)
- `ls`, `tree`, `read`, `find`, `grep`, `diff`, `log`, `env`, `json`, `wc`, `deps`, `smart`, `summary`, `err`, `test`

### Git (3 command families)
- `git` (11 subcommands), `gh` (4 subcommands), `gt` (Graphite)

### Rust (4 filters)
- `cargo` (8 subcommands), `clippy`, `check`, `fmt`

### JavaScript (10+ filters)
- `npm`, `pnpm`, `npm run`, `vitest`, `playwright`, `tsc`, `lint`, `prettier`, `prisma`, `next`, `npx`

### Python (5 filters)
- `pytest`, `ruff`, `mypy`, `pip`, `poetry`

### Ruby (3 filters)
- `rake`, `rspec`, `rubocop`, `bundle`

### Go (4 filters)
- `go` (test, build, mod), `golangci-lint`

### .NET (4 filters)
- `dotnet` (build, test, restore, format, clean)

### Cloud (5 command families)
- `aws` (10+ services), `docker`, `kubectl`, `curl`, `wget`, `psql`

---

## Installation Method Comparison

| Method | Command | Platform | Binary Size | Version |
|--------|---------|----------|-------------|---------|
| Homebrew | `brew install rtk` | macOS, Linux | Auto-downloaded | Latest |
| Cargo | `cargo install --git ...` | Unix, Windows | Built locally | Latest |
| Script | `curl -fsSL ... \| sh` | Linux, macOS | Auto-downloaded | Latest |
| Binary | Download .tar.gz/.zip | All | Pre-built | Specific |
| Source | `cargo build --release` | Rust toolchain | Built locally | Custom |

---

## Configuration Sections (TOML)

```toml
[tracking]
enabled = true
history_days = 90
database_path = "/custom/path"          # Optional

[display]
colors = true
emoji = true
max_width = 120

[tee]
enabled = true
mode = "failures"                        # failures | always | never
max_files = 20
max_file_size = 1048576
directory = "/custom/tee/dir"

[telemetry]
enabled = true

[hooks]
exclude_commands = ["curl", "playwright"]

[limits]
grep_max_results = 200
grep_max_per_file = 25
status_max_files = 15
status_max_untracked = 10
```

---

## Hook Agent Support Matrix

| Agent | Install Cmd | Hook Type | Permission Model | Special Handling |
|-------|-------------|-----------|------------------|------------------|
| Claude Code | `rtk init -g` | Bash hook | deny/ask/allow | Default agent |
| Cursor | `rtk init --agent cursor` | hooks.json | deny/ask/allow | Editor + CLI |
| Windsurf | `rtk init --agent windsurf` | .windsurfrules | deny/ask/allow | Cascade agent |
| Cline | `rtk init --agent cline` | .clinerules | deny/ask/allow | VS Code ext |
| Gemini CLI | `rtk init --gemini` | BeforeTool hook | allow/deny only | No ask mode |
| Copilot | `rtk init --copilot` | preToolUse hook | allow/deny/ask | VS Code + CLI |
| Codex | `rtk init --codex` | via Claudio | ask parsed | OpenAI |

---

## Error Recovery Features

### Output Tee (Failure Recovery)
- Captures full output to temp files on command failure
- Provides `[full output: ...]` recovery link
- Modes: failures, always, never
- Configurable max files (20), max size (1MB)
- Enabled by default

### Integrity Verification
- SHA-256 hash verification for hooks
- Tamper detection with clear messaging
- Hash storage at `~/.claude/hooks/.rtk-hook.sha256`
- Can be verified with `rtk verify`

### Permission Checks
- Evaluated at runtime, capped exit codes (0, 2, 3)
- Claude Code settings.json inspection
- Precedence: deny > ask > allow > default
- Permission denial doesn't block agent execution

---

This matrix covers all 100+ commands rtk supports. Use CTRL+F to search by tool name or use the category sections to find related commands.
