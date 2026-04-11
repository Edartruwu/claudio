# RTK (Rust Token Killer) - Complete Exploration Report

## Executive Summary

**RTK** is a high-performance Rust-based CLI proxy tool designed to minimize LLM token consumption by filtering and compressing command outputs. It achieves 60-90% token savings on common development operations through smart filtering, grouping, truncation, and deduplication.

- **Language**: Rust (100% pure Rust)
- **Total Code**: ~21,000 lines of Rust across 100+ files
- **Binary Size**: <5MB (stripped, optimized)
- **Startup Time**: <10ms overhead
- **Supported Commands**: 100+ commands across 8 ecosystems
- **Installation**: Homebrew, cargo, script install, pre-built binaries

---

## Directory Structure

### Root Structure
```
rtk/
├── src/                          # Main Rust source (11 modules)
│   ├── main.rs                   # CLI entry point, command routing
│   ├── analytics/                # Token savings dashboards
│   ├── cmds/                     # Command filter modules (8 ecosystems)
│   ├── core/                     # Infrastructure (config, tracking, filters)
│   ├── discover/                 # History analysis & command rewriting
│   ├── filters/                  # Built-in TOML filters (70+ files)
│   ├── hooks/                    # LLM agent hook system
│   ├── learn/                    # CLI correction detection
│   └── parser/                   # Output parser infrastructure
├── docs/                         # Architecture & contribution guides
├── hooks/                        # Deployed hook artifacts
├── tests/                        # Integration tests
├── Cargo.toml                    # Rust dependencies
├── Cargo.lock                    # Locked versions
├── CLAUDE.md                     # Claude Code guidance
├── CONTRIBUTING.md               # Contribution guide
├── CHANGELOG.md                  # Release history (63KB)
└── scripts/                      # Build/test automation
```

---

## Module Breakdown

### 1. **src/main.rs** - CLI Router
- **Purpose**: Entry point and command-line argument parsing
- **Key Type**: `Commands` enum (50+ variants)
- **Dependencies**: Clap for argument parsing
- **Global Flags**:
  - `-v, --verbose`: Verbosity levels (-v, -vv, -vvv)
  - `-u, --ultra-compact`: ASCII icons, inline format
  - `--skip-env`: Set SKIP_ENV_VALIDATION=1 for child processes

### 2. **src/cmds/** - Command Filter Modules (8 Ecosystems)

#### 2a. **src/cmds/git/** - Git & VCS
| Command | Output | Example |
|---------|--------|---------|
| `rtk git status` | Compact status | "3 files, 2 staged" |
| `rtk git log` | One-line commits | "abc1234 fix: issue description" |
| `rtk git diff` | Condensed diff | "10 files changed, +50 -30" |
| `rtk git add` | Result | "ok" |
| `rtk git commit` | Result + hash | "ok abc1234" |
| `rtk git push` | Result + branch | "ok main" |
| `rtk git pull` | Stats | "ok 3 files +10 -2" |
| `rtk git branch` | Compact branches | "* main, develop, feature/x" |
| `rtk git show` | Commit summary + stat + diff | Condensed view |

**Sub-modules**:
- `git.rs` - Core git filtering
- `diff_cmd.rs` - Ultra-condensed diff
- `gh_cmd.rs` - GitHub CLI (gh) filtering
- `gt_cmd.rs` - Graphite stacked PR tool

#### 2b. **src/cmds/rust/** - Rust Ecosystem
| Command | Output | Features |
|---------|--------|----------|
| `rtk cargo build` | -80% tokens | Progress → summary |
| `rtk cargo test` | -90% tokens | Failures only |
| `rtk cargo clippy` | -80% tokens | Grouped violations |
| `rtk cargo check` | -80% tokens | Errors grouped by file |

**Sub-modules**:
- `cargo_cmd.rs` - Cargo command filtering
- `runner.rs` - Generic fallback runner (errors-only, tests-only modes)

#### 2c. **src/cmds/js/** - JavaScript/TypeScript/Node
| Command | Output | Features |
|---------|--------|----------|
| `rtk npm run <script>` | Boilerplate stripped | Clean output |
| `rtk pnpm list` | Dependency tree | Compact format |
| `rtk pnpm install` | Installation summary | Progress removed |
| `rtk vitest run` | -90% tokens | Failures only |
| `rtk playwright test` | E2E results | Failures only |
| `rtk tsc` | Grouped errors | By file |
| `rtk lint` | ESLint violations | Grouped by rule |
| `rtk prettier --check` | Files needing format | Compact list |
| `rtk prisma generate` | No ASCII art | Clean output |
| `rtk next build` | -80% tokens | Compact output |

**Sub-modules**:
- `npm_cmd.rs`, `pnpm_cmd.rs` - Package managers
- `vitest_cmd.rs`, `playwright_cmd.rs` - Test runners
- `tsc_cmd.rs` - TypeScript compiler
- `lint_cmd.rs` - ESLint (cross-ecosystem router)
- `prettier_cmd.rs` - Code formatter
- `prisma_cmd.rs` - ORM tool
- `next_cmd.rs` - Next.js build tool

#### 2d. **src/cmds/python/** - Python Ecosystem
| Command | Output | Features |
|---------|--------|----------|
| `rtk pytest` | -90% tokens | Failures only |
| `rtk ruff check` | -80% tokens | JSON, grouped |
| `rtk mypy` | Type errors | Grouped by file |
| `rtk pip list` | Package list | Auto-detects uv |
| `rtk pip outdated` | Outdated only | Compact |

**Sub-modules**:
- `pytest_cmd.rs`, `ruff_cmd.rs`, `mypy_cmd.rs`, `pip_cmd.rs`

#### 2e. **src/cmds/ruby/** - Ruby Ecosystem
| Command | Output | Features |
|---------|--------|----------|
| `rtk rake test` | Minitest output | Compact |
| `rtk rspec` | RSpec results | JSON, -60%+ |
| `rtk rubocop` | Linting violations | JSON, grouped |

#### 2f. **src/cmds/go/** - Go Ecosystem
| Command | Output | Features |
|---------|--------|----------|
| `rtk go test` | NDJSON output | -90% tokens |
| `rtk go build` | Build errors | Grouped |
| `rtk golangci-lint` | Linting results | JSON, -85% |

#### 2g. **src/cmds/dotnet/** - .NET Ecosystem
| Command | Output | Features |
|---------|--------|----------|
| `rtk dotnet build` | Compact output | Build summary |
| `rtk dotnet test` | Test failures | Grouped |
| `rtk dotnet format` | Format report | Parsed |

**Sub-modules**:
- `dotnet_cmd.rs`, `binlog.rs`, `dotnet_trx.rs`, `dotnet_format_report.rs`

#### 2h. **src/cmds/cloud/** - Cloud & Containers
| Command | Output | Features |
|---------|--------|----------|
| `rtk aws sts get-caller-identity` | One-line | Identity only |
| `rtk aws ec2 describe-instances` | Compact list | Instance summary |
| `rtk aws lambda list-functions` | Name/runtime | Strips secrets |
| `rtk aws logs get-log-events` | Timestamps + msgs | Filtered |
| `rtk docker ps` | -80% tokens | Compact containers |
| `rtk docker logs <container>` | Deduplicated | Repeated lines counted |
| `rtk kubectl pods` | Compact list | Pod summary |
| `rtk curl <url>` | Auto-JSON detection | Schema output |
| `rtk wget <url>` | Download progress | Stripped |
| `rtk psql` | Compact tables | Borders stripped |

**Sub-modules**:
- `aws_cmd.rs`, `container.rs`, `curl_cmd.rs`, `psql_cmd.rs`, `wget_cmd.rs`

#### 2i. **src/cmds/system/** - System Utilities
| Command | Output | Features |
|---------|--------|----------|
| `rtk ls [args]` | Token-optimized | Tree format |
| `rtk tree [args]` | Compact tree | Grouped |
| `rtk read <file>` | Smart reading | 3 filter levels |
| `rtk find <pattern>` | Compact results | Grouped by dir |
| `rtk grep <pattern>` | Grouped search | By file |
| `rtk json <file>` | Structure only | Or full with depth |
| `rtk log <file>` | Deduplicated | Repeated counted |
| `rtk wc [args]` | Compact output | Paths stripped |
| `rtk env --filter <name>` | Filtered vars | Sensitive masked |
| `rtk deps [path]` | Dependencies | Project summary |
| `rtk summary <cmd>` | 2-line summary | Heuristic-based |
| `rtk format [args]` | Universal formatter | Auto-detects prettier/black/ruff |
| `rtk smart <file>` | 2-line heuristic | Code summary |
| `rtk err <cmd>` | Errors/warnings only | Run command |
| `rtk test <cmd>` | Test failures only | Run tests |

**Sub-modules**:
- `ls.rs`, `tree.rs`, `read.rs`, `find_cmd.rs`, `grep_cmd.rs`
- `json_cmd.rs`, `log_cmd.rs`, `wc_cmd.rs`, `env_cmd.rs`, `deps.rs`
- `summary.rs`, `format_cmd.rs`, `local_llm.rs`, `constants.rs`

### 3. **src/core/** - Infrastructure (Leaf Module)
| Module | Purpose |
|--------|---------|
| `config.rs` | Configuration loading (TOML) |
| `tracking.rs` | SQLite database for token savings |
| `filter.rs` | TOML filter engine (8-stage pipeline) |
| `runner.rs` | Command execution with exit code propagation |
| `tee.rs` | Output recovery for failures |
| `telemetry.rs` | Anonymous usage tracking |
| `display_helpers.rs` | Color/formatting utilities |
| `toml_filter.rs` | TOML parsing & validation |
| `utils.rs` | Shared utilities (truncate, strip_ansi, token_count, etc.) |

**Key Infrastructure**:
- **Token Tracking**: SQLite schema with `commands` table (timestamp, original_cmd, rtk_cmd, tokens, savings_pct)
- **TOML Filter Pipeline**: 8-stage filtering (strip_ansi → replace → match_output → strip_lines → truncate_lines → head/tail → max_lines → on_empty)
- **Three-tier Filter Lookup**:
  1. `.rtk/filters.toml` (project-local, requires trust)
  2. `~/.config/rtk/filters.toml` (user-global)
  3. Built-in filters (compiled in at build time)

### 4. **src/analytics/** - Token Savings Dashboards
| Command | Purpose |
|---------|---------|
| `rtk gain` | Summary stats |
| `rtk gain --graph` | ASCII graph (30 days) |
| `rtk gain --history` | Recent command history |
| `rtk gain --daily/--weekly/--monthly` | Breakdown by period |
| `rtk gain --all --format json` | JSON export |
| `rtk cc-economics` | Claude Code spending vs RTK savings |
| `rtk session` | RTK adoption across sessions |

**Sub-modules**:
- `gain.rs` - Token savings analytics
- `cc_economics.rs` - Cost reduction estimates
- `ccusage.rs` - Claude Code spending data parsing
- `session_cmd.rs` - Adoption metrics

### 5. **src/discover/** - History Analysis & Command Rewriting
| Module | Purpose |
|--------|---------|
| `lexer.rs` | Tokenizes shell commands (handles quotes, escapes, redirects) |
| `provider.rs` | Extracts commands from Claude Code JSONL sessions |
| `registry.rs` | Command classification against 60+ regex patterns |
| `rules.rs` | Rewrite patterns (pattern → rtk_cmd mapping) |
| `report.rs` | Discovery report generation |

**Key Features**:
- **Live Command Rewriting**: Hooks call `rtk rewrite "raw cmd"` → returns `rtk <cmd>` equivalent
- **History Analysis**: `rtk discover` finds commands that could have been rewritten but weren't
- **Compound Command Splitting**: Handles `&&`, `||`, `;`, `|` operators
- **Smart Passthrough**: Skips commands with `--json`, `--template`, `RTK_DISABLED=1`, etc.

### 6. **src/hooks/** - LLM Agent Integration
| Module | Purpose |
|--------|---------|
| `init.rs` | Hook installation flows (6 modes) |
| `hook_check.rs` | Hook validation |
| `integrity.rs` | SHA-256 verification |
| `permissions.rs` | Permission model (deny/ask/allow) |
| `rewrite_cmd.rs` | CLI bridge to discovery registry |
| `hook_cmd.rs` | Gemini CLI & Copilot hook processors |
| `hook_audit_cmd.rs` | Hook usage metrics |
| `verify_cmd.rs` | Inline filter testing |
| `trust.rs` | Project-local TOML filter trust |

**Installation Modes** (via `rtk init`):
1. **Claude Code** (`-g`) - Hook + SHA256 + RTK.md + settings.json patch
2. **Cursor** (`-g --agent cursor`) - Cursor hook + hooks.json
3. **Windsurf** (`-g --agent windsurf`) - `.windsurfrules`
4. **Cline** (`--agent cline`) - `.clinerules`
5. **Gemini CLI** (`--gemini`) - Gemini BeforeTool processor
6. **Copilot** (`--copilot`) - Copilot preToolUse processor
7. **Codex** (`--codex`) - RTK.md + AGENTS.md

**Permission Model**:
```
Deny (exit 2) > Ask (exit 3) > Allow (exit 0) > Default (ask)
```

### 7. **src/learn/** - CLI Correction Detection
| Type | Purpose |
|------|---------|
| `detector.rs` | Detects fail-then-succeed CLI patterns |
| `report.rs` | Generates correction reports |

**Features**:
- Analyzes Claude Code session history
- Identifies recurring CLI mistakes
- Auto-generates `.claude/rules/cli-corrections.md`
- Error types: UnknownFlag, CommandNotFound, WrongSyntax, WrongPath, MissingArg, PermissionDenied

### 8. **src/parser/** - Output Parsing Infrastructure
**Three-Tier Parsing Strategy**:
1. **Tier 1 (Full)**: Complete JSON parsing
2. **Tier 2 (Degraded)**: Partial parsing with regex fallback
3. **Tier 3 (Passthrough)**: Truncated raw output

**Canonical Types**:
- `TestResult` - For test runners (vitest, playwright, pytest, etc.)
- `LintResult` - For linters (eslint, tsc, ruff, etc.)
- `DependencyState` - For package managers
- `BuildOutput` - For build tools

**Format Modes**:
- Compact (default): Summary only, top 5-10 items
- Verbose: Full details, all items (up to 20)
- Ultra: ASCII symbols, 30-50% extra token reduction

### 9. **src/filters/** - Built-in TOML Filters (70+ Files)

**Filter Scope**:
- Designed for predictable, line-by-line text output
- Achieve 60%+ savings through regex filtering
- Must preserve command output format

**Filter Examples**:
```
File: brew-install.toml          # Strip "Using ..." lines
File: df.toml                     # Keep essential rows only
File: terraform-plan.toml         # Strip progress, keep summary
File: docker-compose.toml         # Compact service view
File: poetry-install.toml         # Installation summary
```

**8-Stage Pipeline** (in order):
1. `strip_ansi` - Remove ANSI escape codes
2. `replace` - Regex substitutions with backreferences
3. `match_output` - Short-circuit rules
4. `strip_lines_matching` - Filter lines by regex
5. `keep_lines_matching` - Keep only matching lines
6. `truncate_lines_at` - Truncate long lines
7. `head/tail_lines` - Keep first/last N lines
8. `max_lines` - Absolute line cap
9. `on_empty` - Fallback message if empty

---

## Key Features & Capabilities

### 1. **Token Savings**
- **30-minute session**: ~118,000 tokens (standard) → ~23,900 tokens (with rtk) = **-80%**
- Per-command savings:
  - `ls`/`tree`: -80%
  - `cat`/`read`: -70%
  - `grep`/`rg`: -80%
  - `git status`: -80%
  - `git diff`: -75%
  - `cargo test`: -90%
  - `pytest`: -90%
  - `docker ps`: -80%

### 2. **Auto-Rewrite Hooks**
- Transparent command rewriting (e.g., `git status` → `rtk git status`)
- Installed for 6+ LLM agents (Claude Code, Cursor, Windsurf, Cline, Gemini, Copilot)
- Zero token overhead (rewrite happens before execution)
- Integrity verification (SHA-256)
- Permission model (deny/ask/allow)

### 3. **Filtering Strategies**
- **Smart Filtering**: Removes noise, comments, whitespace, boilerplate
- **Grouping**: Aggregates similar items (files by directory, errors by type)
- **Truncation**: Keeps relevant context, cuts redundancy
- **Deduplication**: Collapses repeated log lines with counts

### 4. **Analytics & Insights**
- Token savings tracking (SQLite database)
- Command history (when/where/savings per command)
- Adoption metrics (session-level)
- Cost reduction estimates (Claude Code spending vs RTK savings)
- Discovery of missed opportunities (`rtk discover`)
- CLI correction learning (`rtk learn`)

### 5. **Cross-Platform Support**
- macOS (Intel & Apple Silicon)
- Linux (x86_64, aarch64, musl)
- Windows (MSVC)

### 6. **Configuration**
- **TOML-based**: `~/.config/rtk/filters.toml`, `.rtk/filters.toml` (project-local)
- **Sections**:
  - `[tracking]` - Database, history days
  - `[display]` - Colors, emoji, max_width
  - `[tee]` - Output recovery
  - `[telemetry]` - Usage tracking
  - `[hooks]` - Exclude commands
  - `[limits]` - Grep max results, status limits

### 7. **Command-Specific Features**

#### Git
- Respects all git global options: `-C`, `-c`, `--git-dir`, `--work-tree`, `--no-pager`, etc.
- Exit code propagation for CI/CD
- Cross-command imports (diff format shared with gh, git)

#### JavaScript/TypeScript
- Auto-detects pnpm/yarn/npm via lockfiles
- Supports modern stack: pnpm, vitest, Next.js, TypeScript, Playwright, Prisma
- Cross-ecosystem routing (lint → python mypy if Python project detected)

#### Testing
- **Mode 1 (test)**: Show failures only from test runners
- **Mode 2 (err)**: Show errors/warnings only from any command
- Supports: pytest, cargo test, vitest, playwright, jest, go test, rspec, rake

#### Output Recovery (Tee)
- Captures full output to temp files on failure
- Provides `[full output: ...]` recovery link
- Configurable mode: failures, always, never

### 8. **CLI Tools & Subcommands**

**Major Commands**:
- `rtk ls`, `rtk tree` - Directory listing
- `rtk read` - Smart file reading (3 filter levels)
- `rtk git` - Git subcommands (status, log, diff, add, commit, push, pull, branch, etc.)
- `rtk cargo` - Cargo subcommands (build, test, clippy, check)
- `rtk npm`, `rtk pnpm` - Package managers
- `rtk aws` - AWS CLI (sts, s3, ec2, lambda, logs, cf)
- `rtk docker`, `rtk kubectl` - Container tools
- `rtk pytest`, `rtk ruff`, `rtk mypy` - Python tools
- `rtk go` - Go commands
- `rtk dotnet` - .NET commands
- `rtk curl`, `rtk wget` - HTTP/download
- `rtk gain` - Analytics dashboard
- `rtk discover` - Missed opportunities
- `rtk learn` - CLI correction learning
- `rtk init` - Hook installation
- `rtk rewrite` - Command rewriting (used by hooks)
- `rtk proxy` - Passthrough + tracking
- `rtk trust` - TOML filter trust management
- `rtk verify` - Hook integrity check

---

## Technical Architecture

### Execution Flow

```
1. User/Hook Calls RTK
   ↓
2. main.rs Routes to Command Handler
   ↓
3. Command Module (cmds/*/):
   a. Execute underlying command (via runner)
   b. Capture stdout/stderr
   c. Apply TOML filter (if available)
   d. Apply command-specific filtering (Rust code)
   e. Track token savings (core/tracking)
   f. Output compressed result
   ↓
4. Hook Checks Exit Code
   - 0 = Success, allow execution
   - Non-0 = Failure, error handling
```

### Command Rewriting Flow

```
1. Hook intercepts: "git status"
   ↓
2. Calls: "rtk rewrite git status"
   ↓
3. discover/lexer.rs tokenizes input
   ↓
4. discover/registry.rs classifies command
   ↓
5. Matches against 60+ regex rules
   ↓
6. Returns: "rtk git status" (or passthrough if no match)
   ↓
7. Hook executes rewritten command
   ↓
8. core/tracking.rs records metrics
```

### Token Counting
- **Algorithm**: `ceil(chars / 4.0)` (approximate tokens)
- **Tracking**: Original command output size vs filtered output size
- **Database**: SQLite `commands` table with:
  - `timestamp` (UTC ISO8601)
  - `original_cmd`, `rtk_cmd`
  - `project_path` (cwd for scoping)
  - `input_tokens`, `output_tokens`, `saved_tokens`, `savings_pct`
  - `exec_time_ms`

---

## Dependencies

**Key Crates**:
- `clap` (4) - CLI argument parsing
- `anyhow` - Error handling
- `rusqlite` - SQLite database
- `regex`/`lazy_static` - Pattern matching
- `serde`/`serde_json` - JSON parsing
- `toml` - Configuration
- `walkdir`/`ignore` - File traversal
- `chrono` - Date/time
- `colored` - Terminal colors
- `tempfile` - Temp file management
- `sha2` - Hashing
- `which` - Command resolution

**No async** (single-threaded by design for <10ms startup)

---

## Build & Packaging

### Release Optimizations
```toml
[profile.release]
opt-level = 3
lto = true          # Link-time optimization
codegen-units = 1
panic = "abort"
strip = true        # Strip debug symbols
```

### Distributions
- **Homebrew**: `brew install rtk`
- **Linux DEB**: `cargo deb` (via cargo-deb)
- **Linux RPM**: `cargo generate-rpm`
- **Source**: `cargo install --git https://github.com/rtk-ai/rtk`

---

## Development Workflow

### Build & Test
```bash
cargo build                   # Debug
cargo build --release         # Optimized
cargo test                    # All tests
cargo test <test_name>        # Specific
cargo fmt && cargo clippy     # Quality gates
rtk cargo build               # Using rtk (token-optimized)
```

### Quality Gates (Pre-commit)
```bash
cargo fmt --all && cargo clippy --all-targets && cargo test --all
```

### Smoke Tests
```bash
bash scripts/test-all.sh      # Requires installed binary
```

### Performance Testing
```bash
hyperfine 'rtk git log -10' --warmup 3    # Before
cargo build --release
hyperfine 'target/release/rtk git log -10' --warmup 3  # After (<10ms)
```

---

## Integrations & External Services

### LLM Agents (Hook Support)
1. **Claude Code** (Claude API)
2. **Cursor Agent** (editor + CLI)
3. **Windsurf IDE** (Cascade agent)
4. **Cline / Roo Code** (VS Code)
5. **Gemini CLI** (Google)
6. **GitHub Copilot** (VS Code + CLI)
7. **Codex CLI** (OpenAI - via Claudio)

### External APIs
- **Claude Code Sessions**: JSONL file parsing (for discover/learn)
- **AWS CLI**: JSON output parsing + filtering
- **Git**: Via native `git` command + custom parsing

### Configuration Sources
- Claude Code `settings.json` (permission rules)
- Cursor `settings.json`
- Local `.claude/hooks/` directory (hook integrity)

---

## File Count & Statistics

| Category | Count |
|----------|-------|
| Rust source files | 100+ |
| TOML filters | 70+ |
| Total lines of Rust | ~21,000 |
| Main modules | 11 |
| Supported commands | 100+ |
| Command subcommands | 50+ |
| Regex rewrite rules | 60+ |

---

## Notable Design Patterns

### 1. **Command Proxy Architecture**
- main.rs routes via `Commands` enum
- Each command has dedicated module in cmds/*/
- Specialized filters + generic fallback (runner.rs)

### 2. **TOML Filter Pipeline**
- Build-time concatenation (build.rs)
- 8-stage processing (order matters)
- Inline tests per filter

### 3. **Three-Tier Parsing**
- Full JSON (Tier 1), Degraded regex (Tier 2), Passthrough (Tier 3)
- Graceful degradation on tool version changes

### 4. **Compound Command Splitting**
- Lexer tokenizes shell syntax (quotes, escapes, redirects)
- Registry splits on operators (&&, ||, ;) and pipes
- Each segment rewritten independently

### 5. **Hook Integrity**
- SHA-256 verification (prevent tampering)
- Permission precedence (deny > ask > allow)
- Atomic file writes (tempfile + rename)

### 6. **Token Tracking**
- SQLite persistence (project-scoped queries)
- Original vs filtered token count
- Per-command execution time

---

## Future Roadmap (From Code)

### Parser Infrastructure (Phase 4-5)
- [ ] Migrate more commands to `OutputParser` trait
- [ ] VitestParser, PlaywrightParser, PnpmParser
- [ ] Extend tracking.db with parse tier metrics
- [ ] `rtk parse-health` command

### Module Migrations
- [ ] lint_cmd.rs → EslintParser
- [ ] tsc_cmd.rs → TscParser
- [ ] gh_cmd.rs → GhParser

---

## Summary

RTK is a mature, well-architected Rust CLI tool with:
- **Broad command coverage** (100+ commands, 8 ecosystems)
- **Clean modular design** (8 independent command domains + 3 support domains)
- **Production-ready infrastructure** (tracking DB, hook system, integrity verification)
- **Strong token optimization** (60-90% savings through multi-strategy filtering)
- **LLM-first integration** (6+ agent hooks, discovery, learning)
- **Zero startup overhead** (<10ms, single-threaded)

It serves as a critical efficiency tool for LLM-based development workflows, transparently rewriting commands and compressing their output to maximize token efficiency.
