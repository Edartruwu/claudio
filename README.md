# Claudio

> **An open-source AI coding assistant for the terminal, built in Go.**
> Multi-agent orchestration · persistent memory · team coordination · full vim mode · zero runtime dependencies.

```
  ██████╗██╗      █████╗ ██╗   ██╗██████╗ ██╗ ██████╗
 ██╔════╝██║     ██╔══██╗██║   ██║██╔══██╗██║██╔═══██╗
 ██║     ██║     ███████║██║   ██║██║  ██║██║██║   ██║
 ██║     ██║     ██╔══██║██║   ██║██║  ██║██║██║   ██║
 ╚██████╗███████╗██║  ██║╚██████╔╝██████╔╝██║╚██████╔╝
  ╚═════╝╚══════╝╚═╝  ╚═╝ ╚═════╝ ╚═════╝ ╚═╝ ╚═════╝
```

---

## Table of Contents

- [Why Claudio?](#why-claudio)
- [Requirements](#requirements)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Project Setup](#project-setup)
  - [/init — Project setup skill](#init--project-setup-skill)
  - [Configuration hierarchy](#configuration-hierarchy)
  - [TUI config editor](#tui-config-editor)
  - [Settings reference](#settings-reference)
  - [CLAUDIO.md / CLAUDE.md](#claudemd--claudemd)
  - [Permission Rules](#permission-rules)
- [CLI Flags](#cli-flags)
- [Interactive Commands](#interactive-commands)
- [Keybindings](#keybindings)
- [Context Management](#context-management)
- [Token Efficiency](#token-efficiency)
- [Memory System](#memory-system)
- [Tools](#tools)
- [Agents](#agents)
- [Orchestrator & Multi-Agent Teams](#orchestrator--multi-agent-teams)
- [Harness — Agent Team Architecture](#harness--agent-team-architecture)
  - [The 6 patterns](#the-6-patterns)
  - [Building a harness with /harness](#building-a-harness-with-harness)
  - [Using a generated harness](#using-a-generated-harness)
  - [Agent definition files](#agent-definition-files)
  - [Orchestrator skill](#orchestrator-skill)
- [Security](#security)
- [Hooks](#hooks)
- [Scheduled Tasks (Cron)](#scheduled-tasks-cron)
- [Session Sharing](#session-sharing)
- [Plugins](#plugins)
- [Model Configuration](#model-configuration)
- [Output Styles](#output-styles)
- [Snippet Expansion (Experimental)](#snippet-expansion-experimental)
- [Keybinding Customization](#keybinding-customization)
- [Per-Turn Diff Tracking](#per-turn-diff-tracking)
- [Web UI](#web-ui)
- [Headless / API Mode](#headless--api-mode)
- [Filesystem Layout](#filesystem-layout)
- [Architecture](#architecture)
- [License](#license)

---

## Why Claudio?

| | Claudio | Claude Code |
|---|---|---|
| **Language** | Go (single binary, no runtime) | Node.js/TypeScript |
| **Multi-agent teams** | Built-in orchestration, mailbox messaging, team management | Not supported |
| **Session-as-agent** | Crystallize sessions into reusable agent personas with their own memory | Not supported |
| **Memory** | Scoped (project/agent/global), AI-powered selection, background extraction, dream consolidation | Single project directory |
| **Context management** | Tiered compaction (partial + full), message pinning, `/` search in viewport, memory tool | Basic |
| **Token efficiency** | 11-layer optimization: prompt caching, RTK-style output filtering, snippet expansion, microcompaction, tool result offload, read dedup, image compression, deferred tool schemas | Basic prompt caching |
| **Snippet expansion** | `~name(args)` shorthand expands to full code via templates — saves output tokens on repetitive boilerplate | Not supported |
| **Learned instincts** | Patterns extracted from sessions, confidence-scored, replayed in future sessions to avoid repeated mistakes | Not supported |
| **Cron tasks** | Schedule recurring agent tasks (`@every 1h`, `@daily`, `HH:MM`) | Feature-gated |
| **Web UI** | Full browser chat UI (`claudio web`) with streaming, tool approval, plan mode, AskUser, model selector | Not supported |
| **Cross-session comms** | Bridge via Unix sockets for parallel agents in worktrees | Not supported |
| **Vim mode** | Full state machine (normal, insert, visual, operator-pending, registers) | Basic vi-mode |
| **Persistence** | SQLite + file-based | File-based only |

---

## Requirements

- **Go 1.26+** (for building from source)
- **Anthropic API key** or OAuth login (additional providers like Groq, OpenAI, Ollama also supported)
- **Git** (for project root detection, worktrees)
- **OS:** macOS, Linux (Windows: experimental)

### Optional

- `$EDITOR` (vim, nvim, etc.) for external editing features
- Language servers for LSP integration (gopls, pyright, etc.)
- MCP servers for extended tool capabilities

---

## Installation

### From source (recommended)

```bash
git clone https://github.com/Abraxas-365/claudio
cd claudio
go build -o claudio ./cmd/claudio
```

Move the binary to your PATH:

```bash
# macOS / Linux
sudo mv claudio /usr/local/bin/

# Or add to your local bin
mv claudio ~/.local/bin/
```

### With `go install`

```bash
go install github.com/Abraxas-365/claudio/cmd/claudio@latest
```

This places the binary in `$GOPATH/bin` (or `$HOME/go/bin` by default). Make sure that directory is in your `$PATH`.

### Verify installation

```bash
claudio --help
```

---

## Quick Start

```bash
# 1. Authenticate with Anthropic
claudio auth login

# 2. Start the TUI in your project and run /init
cd your-project
claudio
# then type: /init

# 3. Start coding
claudio
```

### Usage modes

```bash
# Interactive TUI (default)
claudio

# Single prompt (no TUI)
claudio "explain this codebase"

# Pipe mode
echo "fix the bug in main.go" | claudio

# Resume last session
claudio --resume

# Headless API server
claudio --headless
```

---

## Project Setup

### `/init` — Project setup skill

> **Recommended:** Run `/init` inside the TUI (`claudio`) rather than the `claudio init` CLI command. The TUI version is AI-powered and interactive — it surveys your codebase, interviews you with targeted questions, and generates a tailored `CLAUDIO.md`, skills, and hook suggestions in one session.

```
claudio        # start the TUI
/init          # run the init skill
```

The `/init` skill walks through several phases:

1. Asks a few setup questions (scope, branch conventions, gotchas)
2. Surveys the codebase with a subagent (structure, languages, frameworks, CI)
3. Fills gaps with follow-up questions and shows you the proposed `CLAUDIO.md`
4. Writes `CLAUDIO.md` and optionally `CLAUDIO.local.md` (personal overrides, gitignored)
5. Creates project skills under `.claudio/skills/`
6. Suggests hooks and GitHub CLI integrations

**CLI fallback (`claudio init`):**

If you prefer a non-interactive bootstrap, `claudio init` creates the `.claudio/` scaffold and a starter `CLAUDIO.md` without the interactive interview. You can then refine with `/init` inside the TUI.

```
.claudio/
  settings.json      # Project-specific settings (overrides global)
  rules/             # Project-specific rules
    project.md       # Example rule template
  skills/            # Project-specific skills
  agents/            # Project-specific agent definitions
  memory/            # Project-scoped memories
  .gitignore         # Ignores local-only files
CLAUDIO.md           # Project instructions for the AI
```

### Configuration hierarchy

Settings are merged with priority (highest first):

```
Environment variables    CLAUDIO_MODEL, CLAUDIO_API_BASE_URL, etc.
       |
.claudio/settings.json  Project config (per-repo, committed to git)
       |
~/.claudio/local.json   Local overrides (per-machine, not committed)
       |
~/.claudio/settings.json  Global user config
       |
Built-in defaults
```

**Scalar values** (model, permissionMode) are overridden by higher priority. **Lists** (denyTools, denyPaths) are appended across layers. Resources like agents, skills, and rules from **both** `~/.claudio/` and `.claudio/` are loaded and merged.

### TUI config editor

Open with `<Space>ic`. The panel shows:
- **P** badge for settings from project scope
- **G** badge for settings from global scope
- `tab` to switch which scope you're editing
- `enter` to toggle/cycle values (saved immediately)

### Settings reference

```json
{
  "model": "claude-sonnet-4-6",
  "smallModel": "claude-haiku-4-5-20251001",
  "thinkingMode": "",
  "budgetTokens": 0,
  "effortLevel": "medium",
  "permissionMode": "default",
  "autoCompact": false,
  "compactMode": "strategic",
  "sessionPersist": true,
  "hookProfile": "standard",
  "autoMemoryExtract": true,
  "memorySelection": "ai",
  "outputStyle": "normal",
  "costConfirmThreshold": 0,
  "apiBaseUrl": "https://api.anthropic.com",
  "maxBudget": 0,
  "denyPaths": [],
  "allowPaths": [],
  "denyTools": [],
  "permissionRules": [],
  "mcpServers": {}
}
```

| Setting | Values | Description |
|---------|--------|-------------|
| `model` | any Claude model ID | Default AI model |
| `thinkingMode` | `""`, `adaptive`, `enabled`, `disabled` | Extended thinking mode |
| `budgetTokens` | token count (e.g., `32000`) | Thinking budget when mode is `enabled` |
| `effortLevel` | `low`, `medium`, `high` | Reasoning depth (default: medium) |
| `permissionMode` | `default`, `auto`, `plan` | Tool approval behavior |
| `permissionRules` | array of rules | Content-pattern rules (see below) |
| `autoMemoryExtract` | `true`/`false` | Auto-extract memories after each turn |
| `memorySelection` | `ai`, `keyword`, `none` | How memories are selected for system prompt |
| `outputStyle` | `normal`, `concise`, `verbose`, `markdown` | Response formatting style |
| `costConfirmThreshold` | USD amount, 0 = disabled | Pause for confirmation at this cost |
| `denyTools` | list of tool names | Disable specific tools (e.g. `["Memory", "WebSearch"]`) |
| `compactMode` | `auto`, `manual`, `strategic` | When to compact conversation history |
| `maxBudget` | USD amount, 0 = unlimited | Session spend limit |
| `outputFilter` | `true`/`false` | RTK-style command output filtering (see below) |

### CLAUDIO.md / CLAUDE.md

Place a `CLAUDIO.md` or `CLAUDE.md` in your project root with project-specific instructions. These are automatically loaded into the system prompt.

Searched paths (first match wins per directory):
1. `./CLAUDIO.md`
2. `./CLAUDE.md`
3. `./.claudio/CLAUDE.md`

**Subdirectory discovery:** Claudio walks from your current working directory up to the git root, loading CLAUDIO.md/CLAUDE.md at each level. Files closer to your cwd have higher priority.

**@imports:** Include other markdown files with `@path/to/file.md`:

```markdown
# My Project

@docs/conventions.md
@docs/architecture.md
```

Relative paths resolve from the CLAUDIO.md file's directory. `@~/path` resolves from home. Circular imports are detected and skipped.

### Permission Rules

Content-pattern rules allow fine-grained tool permissions beyond mode-based control:

```json
{
  "permissionRules": [
    {"tool": "Bash", "pattern": "git *", "behavior": "allow"},
    {"tool": "Bash", "pattern": "rm -rf *", "behavior": "deny"},
    {"tool": "Write", "pattern": "*.test.*", "behavior": "allow"},
    {"tool": "*", "pattern": "*.env", "behavior": "deny"}
  ]
}
```

Rules are evaluated in order; first match wins. Behaviors: `allow` (skip approval), `deny` (block), `ask` (show dialog). Pattern matching is tool-aware: Bash matches against the command, Read/Write/Edit match against the file path.

---

## CLI Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--model` | | AI model override (e.g., `claude-opus-4-6`) |
| `--verbose` | `-v` | Enable verbose output |
| `--headless` | | Run as HTTP API server (no TUI) |
| `--context` | | Load context profile (`dev`, `review`, `research`, or a file path) |
| `--budget` | | Session spend limit in USD (0 = unlimited) |
| `--resume` | `-r` | Resume a previous session (no value = most recent, or pass session ID) |
| `--print` | | Print-only mode (no TUI, clean stdout for piping) |
| `--dangerously-skip-permissions` | `--yolo` | Skip all permission prompts |

---

## Interactive Commands

| Command | Aliases | Description |
|---------|---------|-------------|
| `/help` | `h`, `?` | Show available commands |
| `/model` | `m` | Show or change the AI model |
| `/compact [n]` | | Compact conversation history (keep last n messages) |
| `/cost` | | Show session cost and token usage |
| `/memory extract` | `mem` | Manually extract memories from current conversation |
| `/session` | `sessions` | List or manage sessions |
| `/resume [id]` | | Resume a previous session by ID prefix |
| `/new` | | Start a new session |
| `/rename [title]` | | Rename the current session |
| `/config` | `settings` | View/edit configuration |
| `/commit` | | Create a git commit with AI-generated message |
| `/diff [args]` | | Show git diff (or `/diff turn N` for per-turn changes) |
| `/status` | | Show git status |
| `/share [path]` | | Export session for sharing |
| `/teleport <path>` | | Import a shared session file |
| `/plugins` | | List installed plugins |
| `/output-style [style]` | | Show or set output style (normal, concise, verbose, markdown) |
| `/keybindings` | | Open keybindings.json in your editor |
| `/vim` | | Toggle vim keybindings |
| `/skills` | | List available skills |
| `/tasks` | | Show background tasks and team status |
| `/team` | | Manage agent teams |
| `/audit` | | Show recent tool audit log |
| `/export [format]` | | Export conversation (markdown, json, txt) |
| `/undo` | | Undo the last exchange |
| `/doctor` | | Diagnose environment issues |
| `/mcp` | | Manage MCP servers |
| `/exit` | `quit`, `q` | Exit Claudio |

---

## Keybindings

### Global

| Key | Action |
|-----|--------|
| `Ctrl+C` | Cancel streaming / quit |
| `Ctrl+G` | Open prompt in external editor (`$EDITOR`) |
| `Ctrl+V` | Paste image from clipboard |
| `Shift+Tab` | Cycle permission mode |
| `Esc` | Dismiss overlays / cancel streaming |

### Viewport (conversation view)

Enter viewport mode with `<Space>wk` or (in vim normal mode with empty prompt) just scroll with `j`/`k`.

| Key | Action |
|-----|--------|
| `j` / `k` | Navigate between message sections |
| `Ctrl+D` / `Ctrl+U` | Jump 5 sections down/up |
| `g` / `G` | Jump to top/bottom |
| `/` | Search messages (type query, `enter` to confirm, `n`/`N` to navigate matches) |
| `p` | Pin/unpin message (pinned messages survive compaction) |
| `Enter` / `Ctrl+O` | Toggle tool group expansion |
| `i` / `q` / `Esc` | Return to prompt |

### Leader Sequences (`Space` = leader key)

| Sequence | Action |
|----------|--------|
| `<Space>wk` | Focus viewport |
| `<Space>wj` | Focus prompt |
| `<Space>bn` / `<Space>bp` | Next / previous session |
| `<Space>bc` | Create new session |
| `<Space>bk` | Delete current session |
| `<Space>.` | Open session picker (telescope-style) |
| `<Space>,<Enter>` | Switch to alternate session |

### Panels (`<Space>i` + key)

| Key | Panel | Description |
|-----|-------|-------------|
| `c` | Configuration | View/edit settings with project/global scope |
| `m` | Memory | Browse, search, edit, add, delete memories |
| `k` | Skills | Browse available skills |
| `a` | Analytics | Session statistics |
| `t` | Tasks | Background tasks and team status |

### Vim Mode

Toggle with `/vim`. Full state machine: `i` (insert), `Esc` (normal), `hjkl`, `w/b/e` (word motion), `f/F/t/T` (char search), `.` (repeat), `d/c/y` (operators), text objects, registers, counts, `%` (bracket matching).

---

## Context Management

### Context budget bar

The status bar shows a visual indicator of context window usage:

```
[████████░░] 72%
```

Colors: green (< 70%), yellow (70-90%), red (> 90%). Auto-compaction triggers at 95%.

### Message pinning

Press `p` in viewport mode to pin important messages. Pinned messages are preserved through compaction instead of being summarized away.

### Memory tool

The AI has access to a `Memory` tool that can search, list, and read memories on demand during conversation. This means the AI can look up relevant context without needing all memories loaded in the system prompt.

### Compaction

Tiered compaction as context approaches the window limit:
- **70%**: partial compact (clear old tool results to save tokens)
- **90%**: suggest full compaction
- **95%**: force full compact (summarize old messages, keep last 10 + pinned)

Manual compaction: `/compact [n]` (keep last n messages). `/compact partial` clears old tool results without summarizing.

---

## Token Efficiency

Claudio implements a multi-layer token optimization stack to minimize API cost and keep long sessions within the context window.

### Prompt caching

Every request marks the last system prompt block with `cache_control: {type: "ephemeral"}`. The Anthropic API caches everything up to that point server-side for 5 minutes. Cached input tokens cost ~10× less than normal input tokens. In practice this means the system prompt (which can be hundreds of tokens of instructions, memories, rules, and tool descriptions) is only paid for in full once per session.

Cache reads and writes are tracked in the analytics panel (`<Space>ia`). When more than 5 minutes pass between turns, Claudio warns that the cache has likely expired so the first response will be slightly slower.

### Output token slot reservation

`max_tokens` defaults to **8 192** rather than the model maximum. This matters because the API reserves `max_tokens` worth of capacity from the context window even if the model finishes early. A lower default leaves more room for input. If the model hits the limit mid-response, Claudio automatically retries the same request with `max_tokens = 64 000` before surfacing an error.

### Diminishing returns detection

When the model continues past a `max_tokens` stop with no tool calls (continuation mode), Claudio injects "Please continue from where you left off." and tracks how many output tokens each continuation produces. If output tokens drop by more than 50% compared to the prior continuation, or after 5 consecutive continuations, the loop stops — preventing wasted spend on a response that is tapering off.

### Microcompaction

After every tool-execution turn, `compact.MicroCompact` scans the message history and clears read-heavy tool results that are older than the last 6 results and larger than 2 KB. Affected tools: `Bash`, `Read`, `Glob`, `Grep`, `WebFetch`, `WebSearch`, `LSP`, `ToolSearch`. Content is replaced with `[result cleared — N bytes]`. This runs continuously (no threshold required) and keeps the message payload lean throughout long sessions, complementing the tiered threshold-based compaction at 70/90/95%.

### Tool result disk offload

Tool results larger than **50 KB** are written to `$TMPDIR/claudio-tool-results/` and replaced in the API payload with a compact placeholder (`[tool result on disk: id, N bytes]`). The files are cleaned up when the session ends. This prevents single large outputs (e.g., a long bash command or a web fetch) from consuming a disproportionate share of the context window.

### Duplicate read deduplication

`FileReadTool` maintains an in-session LRU cache (256 entries) keyed by `(path, offset, limit)`. Cache entries are invalidated automatically when the file's mtime changes. If the model reads the same file section more than once, subsequent reads return the cached result without hitting disk or adding a duplicate large block to the conversation history.

### Image compression

Before base64-encoding an image (from file or clipboard), Claudio checks whether it exceeds **500 KB**. If it does, it decodes the image and re-encodes as JPEG at descending quality levels (85 → 70 → 55 → 40) until it fits. This keeps image tokens predictable and avoids the hard ~3.75 MB API limit for most real-world screenshots and diagrams.

### Message merging

Before each API call, adjacent plain-text user messages are merged into a single message. This reduces per-message overhead and avoids edge cases with consecutive same-role messages. Tool result blocks are never merged.

### Output filtering (RTK-style)

When `"outputFilter": true` is set in your config, Claudio applies intelligent output filtering to Bash command results before they enter the context window. Inspired by [RTK](https://github.com/rtk-ai/rtk), this can reduce token usage by 60-90% on noisy command outputs.

Toggle it in the TUI config panel (`<Space>ic`) or set it in `settings.json`:

```json
{
  "outputFilter": true
}
```

**How it works:** after a command runs, the output passes through two filter layers:

1. **Command-specific filters** — recognizes the command and strips noise particular to that tool:

   | Command | What gets filtered |
   |---------|-------------------|
   | `git push/pull/fetch/clone` | Transfer progress (enumerating, counting, compressing objects, percentages), keeps branch results and errors |
   | `go test` | Keeps only failures + summary line, supports both plain and JSON (`-json`) output |
   | `go build/vet` | Keeps only error/issue lines with file locations, adds count |
   | `cargo build/test/clippy` | Strips compile progress, keeps errors/warnings/failures |
   | `npm/pnpm/yarn install` | Strips download progress, keeps "added N packages" and vulnerability warnings |
   | `pip install` | Strips download/collection lines, keeps success/error |
   | `docker build` | Strips layer download progress, keeps step headers and final result |
   | `docker pull/push` | Strips per-layer progress, keeps digest and status |
   | `make` | For long successful builds, applies generic filtering |

2. **Generic filters** (applied to all commands, or as fallback):
   - Strips ANSI escape codes
   - Collapses 3+ consecutive blank lines to 1
   - Deduplicates 3+ identical consecutive lines → `line (repeated N times)`
   - Removes progress bars (`[=====>   ] 45%`) and spinner characters
   - Truncates lines longer than 500 characters

**Examples:**

`git push` (before: ~15 lines, ~200 tokens):
```
Enumerating objects: 5, done.
Counting objects: 100% (5/5), done.
Compressing objects: 100% (3/3), done.
Writing objects: 100% (3/3), 1.23 KiB | 1.23 MiB/s, done.
Total 3 (delta 2), reused 0 (delta 0)
remote: Resolving deltas: 100% (2/2), completed with 2 local objects.
To github.com/user/repo.git
   abc1234..def5678  main -> main
```

After filtering (~2 lines, ~20 tokens):
```
To github.com/user/repo.git
   abc1234..def5678  main -> main
```

`go build` with errors (before: mixed with package headers):
```
# github.com/user/repo/pkg
pkg/foo.go:10:5: undefined: bar
pkg/foo.go:15:2: cannot use x (type int) as type string
```

After filtering:
```
Go build: 2 errors
1. pkg/foo.go:10:5: undefined: bar
2. pkg/foo.go:15:2: cannot use x (type int) as type string
```

### Deferred tool definitions

Infrequently-used tools (web, LSP, notebooks, tasks, teams, etc.) are sent with a stub schema (`{"type":"object"}`) instead of their full JSON schema. The model discovers them on demand via `ToolSearch`, at which point the full schema is included in the next request. This saves the token cost of sending dozens of tool descriptions on every turn when most of them will never be used.

### Summary

| Technique | Where | Typical saving |
|-----------|-------|---------------|
| Prompt caching | `internal/api/client.go` | ~90% discount on system tokens per turn |
| Output slot reservation | `internal/query/engine.go` | Frees input capacity equal to difference vs model max |
| Diminishing returns stop | `internal/query/engine.go` | Avoids runaway continuation spend |
| Microcompaction | `internal/services/compact/compact.go` | Continuous reduction of old tool result bulk |
| Tool result disk offload | `internal/services/toolcache/` | Caps single-result payload at 50 KB |
| Duplicate read cache | `internal/tools/readcache/` | Eliminates redundant file read tokens |
| Image compression | `internal/tui/images.go` | Reduces image payloads to ≤500 KB |
| Message merging | `internal/query/engine.go` | Reduces per-message overhead |
| Output filtering | `internal/tools/outputfilter/` | 60-90% reduction on noisy command outputs |
| Deferred tool schemas | `internal/tools/registry.go` | Saves full schema cost for unused tools |
| Snippet expansion | `internal/snippets/` | Reduces AI output tokens for repetitive boilerplate |

---

## Memory System

Three-layer memory architecture:

### Persistent Memory (file-based)

Markdown files with YAML frontmatter across three scopes:

| Scope | Path | Purpose |
|-------|------|---------|
| **Project** | `.claudio/memory/` | Project-specific knowledge |
| **Global** | `~/.claudio/memory/` | Cross-project user preferences |
| **Agent** | `~/.claudio/agents/<name>/memory/` | Agent-specific knowledge |

Resolution priority: **Agent > Project > Global**. Higher-priority scopes win on name conflicts.

Memory types: `user`, `feedback`, `project`, `reference`.

```markdown
---
name: user prefers terse output
description: skip trailing summaries
type: feedback
---

Don't summarize at the end of responses.
**Why:** user explicitly asked.
**How to apply:** skip trailing summaries.
```

### Memory selection strategies

| Strategy | Setting | Description |
|----------|---------|-------------|
| `ai` | `"memorySelection": "ai"` | Haiku selects top 5 most relevant memories (default) |
| `keyword` | `"memorySelection": "keyword"` | Fast substring matching |
| `none` | `"memorySelection": "none"` | Don't load memories into system prompt |

### Background extraction

After 4+ turns, a background agent (Haiku) reviews the conversation and automatically extracts memories. Disable with `"autoMemoryExtract": false`.

Manual extraction: `/memory extract`

### Memory panel (`<Space>im`)

| Key | Action |
|-----|--------|
| `j` / `k` | Navigate |
| `d` | Delete selected memory |
| `e` | Edit in `$EDITOR` |
| `a` | Add new memory |
| `r` | Refresh list |
| `tab` | Switch Memories/Rules tabs |

### Learned Instincts

Stored in `~/.claudio/instincts.json`. Patterns with confidence scoring that decays after 30 days. Categories: `debugging`, `workflow`, `convention`, `workaround`.

### Dream Consolidation

A background "dream" agent reviews accumulated sessions (24h + 5 sessions) and extracts cross-session patterns into persistent memories.

---

## Tools

Core tools are always loaded; deferred tools load on-demand via `ToolSearch` to save context.

### Core (always available)

| Tool | Description |
|------|-------------|
| **Bash** | Execute shell commands |
| **Read** | Read files (images, PDFs, notebooks) |
| **Write** | Create or overwrite files |
| **Edit** | Exact string replacement |
| **Glob** | Find files by pattern |
| **Grep** | Search file contents (ripgrep) |
| **Agent** | Spawn sub-agents |
| **ToolSearch** | Discover deferred tools |

### Deferred (on-demand)

| Tool | Description |
|------|-------------|
| **Memory** | Search, list, read persistent memories |
| **WebSearch** / **WebFetch** | Web search and URL fetching |
| **LSP** | Language server operations |
| **NotebookEdit** | Jupyter notebook editing |
| **TaskCreate/List/Get/Update** | Task management |
| **EnterPlanMode** / **ExitPlanMode** | Planning workflow |
| **EnterWorktree** / **ExitWorktree** | Git worktree isolation |
| **TaskStop** / **TaskOutput** | Background task control |
| **TeamCreate** / **TeamDelete** / **SendMessage** | Multi-agent teams |
| **CronCreate** / **CronDelete** / **CronList** | Scheduled recurring tasks |
| **AskUser** | Ask user structured questions with options |

Disable any tool with `"denyTools": ["ToolName"]` in settings.

### LSP (Language Server Protocol)

The LSP tool provides code intelligence (go-to-definition, find-references, hover, document symbols) by connecting to language servers. It is **config-driven** — no servers are built-in; you configure them via settings or plugins.

#### Option 1: Settings

Add `lspServers` to your `~/.claudio/settings.json`:

```json
{
  "lspServers": {
    "gopls": {
      "command": "gopls",
      "args": ["serve"],
      "extensions": [".go", ".mod"]
    },
    "typescript": {
      "command": "typescript-language-server",
      "args": ["--stdio"],
      "extensions": [".ts", ".tsx", ".js", ".jsx"]
    },
    "rust-analyzer": {
      "command": "rust-analyzer",
      "extensions": [".rs"]
    },
    "pyright": {
      "command": "pyright-langserver",
      "args": ["--stdio"],
      "extensions": [".py"]
    }
  }
}
```

Each server config supports:

| Field | Required | Description |
|-------|----------|-------------|
| `command` | yes | Executable name (must be on `$PATH`) |
| `args` | no | Command-line arguments |
| `extensions` | yes | File extensions this server handles (with or without leading `.`) |
| `env` | no | Extra environment variables (e.g., `{"GOFLAGS": "-mod=vendor"}`) |

#### Option 2: Plugin files

Drop a `*.lsp.json` file in `~/.claudio/plugins/`:

```json
// ~/.claudio/plugins/go.lsp.json
{
  "gopls": {
    "command": "gopls",
    "args": ["serve"],
    "extensions": [".go", ".mod"]
  }
}
```

Multiple servers can be defined in one file. Multiple `*.lsp.json` files are merged. Settings-defined servers take priority over plugin-defined ones.

#### Behavior

- **Deferred tool**: The LSP tool only appears when at least one server is configured. The AI discovers it via `ToolSearch`.
- **Lazy start**: Servers start on first use and auto-detect the project root (looks for `.git`, `go.mod`, `package.json`, etc.).
- **Idle cleanup**: Servers shut down after 5 minutes of inactivity.
- **Extension routing**: Each file is routed to the server that registered its extension.

#### Prerequisites

Install the language server binary for your language:

```bash
# Go
go install golang.org/x/tools/gopls@latest

# TypeScript/JavaScript
npm install -g typescript-language-server typescript

# Rust
rustup component add rust-analyzer

# Python
pip install pyright
```

---

## Agents

### Built-in types

| Type | Model | Description |
|------|-------|-------------|
| `general-purpose` | inherit | Multi-step tasks, code search, research |
| `Explore` | haiku | Fast read-only codebase exploration |
| `Plan` | inherit | Design implementation plans (read-only) |
| `verification` | inherit | Validate implementations, runs tests |

### Custom agents

Create markdown files in `~/.claudio/agents/` or `.claudio/agents/`:

```markdown
---
description: Expert Go backend developer
tools: "*"
model: opus
---

You are an expert Go backend developer...
```

### Agent crystallization

Crystallize a session's knowledge into a reusable agent persona with its own memory directory. The agent is then invocable from any project.

---

## Orchestrator & Multi-Agent Teams

Claudio supports spawning parallel worker agents ("teammates") coordinated by a team lead through a **file-based mailbox pattern**. The calling agent becomes the team lead and can spawn, message, and monitor teammates — each of which runs a full LLM conversation loop in its own goroutine.

### How it works

```
┌─────────────┐  TeamCreate   ┌─────────┐  creates config + inboxes/
│  Team Lead  │──────────────▶│ Manager │
│  (you/LLM)  │               └─────────┘
│             │  Agent tool    ┌──────────────┐
│             │───────────────▶│TeammateRunner│  Spawn() → goroutines
│             │               └──────┬───────┘
│             │               ┌──────▼───────┐
│             │               │ Teammate 1   │──┐
│             │               │ Teammate 2   │  │ each runs its own
│             │               │ Teammate 3   │  │ LLM conversation
│             │               └──────────────┘  │
│             │                      │          │
│             │    on completion:    │          │
│             │    mailbox → lead    ▼          │
│             │◀──────────────── Mailbox ◀──────┘
│             │               (file JSON        
│             │                + flock)          
└─────────────┘
```

1. **Team creation** — creates a team config and inbox directory under `~/.claudio/teams/{name}/`
2. **Spawning** — each teammate launches as a goroutine running a full `query.Engine` (LLM loop with tool access). Sub-agents get a cloned tool registry with the `Agent` tool **removed** to prevent infinite recursion.
3. **Messaging** — agents communicate via file-based JSON inboxes with file locking (`flock`). Supports direct messages, broadcasts (`*`), and structured control messages (shutdown requests, plan approvals).
4. **Completion** — when a teammate finishes, it automatically sends a completion message to the team lead's inbox with its result.
5. **Cleanup** — the lead can kill individual teammates or the whole team. `DeleteTeam` fails if members are still active.

### Team commands

```bash
# Create a team (you become the lead)
/team create my-team "Research and implement auth system"

# Spawn teammates with specific tasks
/team spawn my-team researcher "Research OAuth libraries for Go"
/team spawn my-team implementer "Implement the auth middleware"

# Send a direct message to a teammate
/team message my-team researcher "Focus on JWT-based approaches"

# Broadcast to all teammates
/team message my-team * "Wrap up, we're merging in 10 minutes"

# Check team status
/team status my-team

# Delete the team when done
/team delete my-team
```

### Team tools (available to the LLM)

| Tool | Description |
|------|-------------|
| `TeamCreate` | Create a new team (caller becomes lead) |
| `TeamDelete` | Delete a team (fails if members still active) |
| `SendMessage` | Send direct or broadcast messages between agents |

The `Agent` tool handles spawning — when a team exists, the LLM can spawn teammates as background agents that join the team.

### Example: parallel research and implementation

```
You: "Set up a team to add OAuth support to our API"

Claudio (as team lead):
  1. Creates team "oauth-team"
  2. Spawns "researcher" → "Find the best Go OAuth2 library, compare options"
  3. Spawns "implementer" → "Implement OAuth2 middleware once researcher reports back"
  4. Researcher finishes → sends findings to team lead inbox
  5. Lead forwards relevant info to implementer via SendMessage
  6. Implementer finishes → sends completion message
  7. Lead reviews results and reports back to you
```

### Example: code review team

```
You: "Create a review team for the changes in this PR"

Claudio:
  1. Creates team "review-team"
  2. Spawns "security-reviewer" → "Check for security issues in the diff"
  3. Spawns "style-reviewer" → "Check code style and naming conventions"
  4. Spawns "test-reviewer" → "Verify test coverage for new code"
  5. Each reviewer works in parallel, sends findings to lead
  6. Lead consolidates all feedback into a single review summary
```

### Teammate identity and status

Each teammate gets a deterministic ID (`name@team`), a color from the gruvbox palette, and a tracked status:

| Status | Icon | Meaning |
|--------|------|---------|
| Idle | `○` | Waiting for work |
| Working | `◐` | Currently running |
| Complete | `●` | Finished successfully |
| Failed | `✗` | Encountered an error |
| Shutdown | `⊘` | Cancelled by lead |

View live status in the TUI tasks panel (`<Space>it`) or with `/team status`.

### Mailbox internals

Messages are stored as JSON arrays in per-agent inbox files:

```
~/.claudio/teams/my-team/
  config.json                    # team config, member list
  inboxes/
    team-lead.json               # lead's inbox
    researcher.json              # researcher's inbox
    implementer.json             # implementer's inbox
```

All inbox reads/writes are protected by file locks (`flock`) to prevent corruption from concurrent access. Messages support both plain text and structured payloads (shutdown requests, approval responses).

---

## Harness — Agent Team Architecture

A **harness** is a reusable multi-agent architecture for a specific domain or recurring task. Instead of assembling an ad-hoc team each time, you build the harness once — it defines which specialist agents exist, how they communicate, and what pattern they follow — and then invoke it with a single command whenever you need it.

Harnesses live entirely in your project:

```
.claudio/
  agents/
    analyst.md          ← specialist role definitions
    implementer.md
    reviewer.md
  skills/
    feature-harness/
      skill.md          ← orchestrator that assembles & runs the team
CLAUDIO.md              ← documents how to invoke each harness
```

### The 6 patterns

Every harness is built around one of six architectural patterns (or a justified composite of them).

---

#### 1. Pipeline

Sequential stages where each stage's output feeds directly into the next.

```
[Analyze] → [Design] → [Implement] → [Verify]
```

**Use when** each stage depends strongly on the prior stage's output and cannot start before it finishes.

**Example**: feature spec → architecture plan → code → test suite.

**Strength**: clear handoff points, easy to reason about.
**Watch out for**: a slow stage blocks everything downstream. Keep each stage as independent as possible.

---

#### 2. Fan-out / Fan-in

Parallel specialists each work the same input from a different angle, then an integrator merges all results.

```
              ┌→ [Specialist A] ─┐
[Dispatcher] →├→ [Specialist B] ─┼→ [Integrator]
              └→ [Specialist C] ─┘
```

**Use when** the task benefits from multiple independent perspectives simultaneously.

**Example**: research task — one agent checks official docs, one scans community forums, one reads source code, one evaluates security implications → integrator writes the final report.

**Strength**: the most natural fit for agent teams. Specialists can share discoveries in real time via `SendMessage`, so one agent's finding can redirect another's search mid-flight — a compounding quality gain impossible with a single agent.

**Watch out for**: the integrator becoming a bottleneck. Give it a clear merge protocol.

---

#### 3. Expert Pool

A router inspects each task and calls only the expert(s) relevant to it.

```
[Router] → { Security Expert | Performance Expert | Architecture Expert }
```

**Use when** input type varies and each type needs fundamentally different handling.

**Example**: code review router — security changes go to the security expert, hot-path changes to the performance expert, structural changes to the architecture expert.

**Strength**: efficient — only the relevant specialist runs.
**Watch out for**: router classification accuracy. A misclassification wastes a specialist call and may miss issues.

> Sub-agents are usually better than a full team here — you only need one expert at a time, so a persistent team adds overhead with no benefit.

---

#### 4. Producer-Reviewer

A producer creates output; a reviewer validates it against objective criteria and triggers a rework loop if issues are found.

```
[Producer] → [Reviewer] → (issues found) → [Producer] retry
                        → (approved)     → done
```

**Use when** output quality must be verifiable and clear acceptance criteria exist.

**Example**: code generation → test runner + lint checker → revise until passing.

**Strength**: enforces a quality gate without human review on every iteration.
**Watch out for**: infinite loops. Always cap retries at 2–3 rounds. After the cap, surface the unresolved issues to the user rather than silently failing.

---

#### 5. Supervisor

A central coordinator tracks progress and dynamically assigns work to workers based on current state.

```
              ┌→ [Worker A]
[Supervisor] ─┼→ [Worker B]   ← supervisor reassigns based on who finishes first
              └→ [Worker C]
```

**Use when** the total workload is unknown upfront or the optimal assignment can only be decided at runtime.

**Example**: large-scale migration — supervisor reads the full file list, creates a task per file, assigns batches to workers, and rebalances as workers finish at different speeds.

**Difference from Fan-out**: Fan-out assigns work upfront and it stays fixed. Supervisor assigns work dynamically as capacity becomes available.

**Strength**: handles variable workloads gracefully. Shared task list (`TaskCreate`/`TaskUpdate`) makes the supervisor pattern a natural fit for Claudio's team tools.
**Watch out for**: the supervisor becoming a bottleneck. Delegate in large enough chunks that the coordination overhead is negligible.

---

#### 6. Hierarchical Delegation

Lead agents decompose the problem and delegate sub-problems to their own specialists.

```
[Director] → [Lead A] → [Worker A1]
                      → [Worker A2]
           → [Lead B] → [Worker B1]
```

**Use when** the problem decomposes naturally into distinct sub-domains, each large enough to warrant its own team.

**Example**: full-stack feature — director → frontend lead (UI + state + tests) + backend lead (API + DB + tests).

**Claudio constraint**: agent teams cannot be nested — a team member cannot create its own team. Implement level-1 as a team and level-2 as sub-agents, or flatten the hierarchy into a single team.

**Watch out for**: depth beyond 2 levels. Context gets lossy and latency compounds. If you feel you need 3 levels, flatten the bottom two.

---

#### Composite patterns

Real harnesses often combine two patterns:

| Composite | Structure | Example |
|-----------|-----------|---------|
| Fan-out + Producer-Reviewer | Each specialist has a paired reviewer | Multi-language translation — 4 specialists translate in parallel, each feeds their own native-speaker reviewer |
| Pipeline + Fan-out | Sequential phases with a parallel stage in the middle | Analysis (sequential) → parallel implementation by subsystem → integration test (sequential) |
| Supervisor + Expert Pool | Supervisor routes tasks to experts dynamically | Support queue — supervisor reads tickets, routes each to the domain expert with spare capacity |

---

### Building a harness with `/harness`

The `/harness` built-in skill guides you through designing and generating a complete harness for your project. It runs 11 phases automatically:

```
/harness <domain description>
```

**Examples:**

```
/harness full-stack feature implementation
/harness security audit pipeline
/harness research and report generation
/harness large-scale code migration
```

**What it does:**

0. **Audits** — checks for existing harnesses in `.claudio/agents/`, `.claudio/skills/`, and CLAUDIO.md; decides whether to create, extend, repair, or replace
1. **Clarifies** — asks what task the harness covers, what it should output, and who will use it
2. **Explores** — scans your project to understand languages, frameworks, existing agents/skills, and coding conventions
3. **Selects execution mode and pattern** — chooses Agent Teams vs Sub-agents based on whether inter-agent communication is needed, then picks the best-fit architecture pattern with an ASCII diagram; asks for your approval
4. **Designs roster** — defines each specialist role, its type (`Explore`, `Plan`, `general-purpose`, or a custom persona), communication protocol, and QA responsibilities
5. **Writes agent files** — generates `.claudio/agents/<name>.md` with trigger-rich descriptions, QA protocols, and team communication specs
6. **Writes orchestrator** — generates `.claudio/skills/<harness-name>/skill.md` using the appropriate template (Agent Team mode or Sub-agent mode) with QA cross-validation built in
7. **Registers in CLAUDIO.md** — adds an entry documenting how to invoke the harness
8. **Validates structure** — checks for placeholder text, verifies agent name consistency, runs trigger verification (3 should-match + 3 shouldn't-match queries), and performs a dry-run walkthrough
9. **Sets up evolution** — adds a changelog table and modification criteria to the orchestrator so the harness can be extended incrementally
10. **Reports** — summarizes files created, agent roster, 3 example invocations, and concrete next steps

---

### Using a generated harness

Once `/harness` has run, invoking your harness is a single command:

```
/<harness-name> <input>
```

For example, if you built a `feature-harness`:

```
/feature-harness add user notification preferences
```

The orchestrator skill takes over: it creates a `_workspace/feature-harness/` directory, builds the task backlog, spawns the team via `TeamCreate`, coordinates agent communication, and synthesizes the final output.

You can also invoke it conversationally:

```
Run the feature harness on the payments refactor
```

Claudio will recognize the harness from CLAUDIO.md and trigger the orchestrator skill.

**Workspace layout** (created automatically by the orchestrator):

```
_workspace/
  <harness-name>/
    <agent-a>-output.md    ← each agent writes here
    <agent-b>-output.md
    errors.md              ← failed steps logged here
    final.md               ← synthesized output (or actual files for code harnesses)
```

---

### Agent definition files

Each specialist is defined in `.claudio/agents/<name>.md`. This is a markdown file with a YAML front-matter header:

```markdown
---
name: analyst
description: "Codebase analyst. Triggered when exploration, mapping, or dependency analysis is needed."
---

# Analyst — Codebase exploration specialist

You are a codebase analyst responsible for understanding structure, dependencies, and patterns.

## Core responsibilities
1. Map the relevant subsystems for the task at hand
2. Identify dependencies and potential impact areas
3. Surface conventions and patterns the implementer must follow

## Input / output protocol
- **Input**: Receives task description from the orchestrator via TaskCreate
- **Output**: Writes findings to `_workspace/<harness>/analyst-output.md`
- **Format**: Structured markdown — summary, subsystems map, key files, conventions

## Team communication protocol
- **Receives from**: orchestrator — initial task + scope
- **Sends to**: implementer — relevant file paths and conventions
- **Task claims**: claims tasks of type `analysis` from the shared task list

## Error handling
- If a subsystem is too large to fully map: document what was covered and flag the gap
- On timeout: write partial findings and notify the orchestrator
```

Agents in `.claudio/agents/` are automatically available to Claudio across all sessions in that project. The `description` field is used to match the agent to tasks — write it with trigger keywords in mind.

---

### Orchestrator skill

The orchestrator lives in `.claudio/skills/<harness-name>/skill.md`. It is the harness entry point — it sets up the workspace, spawns the team, monitors progress, and synthesizes output.

Key sections of an orchestrator:

```markdown
## Phase 2: Launch the team

TeamCreate({
  name: "feature-team",
  members: [
    { name: "analyst",     agent: "analyst",     task: "Map the codebase for: <input>" },
    { name: "implementer", agent: "implementer", task: "Implement once analyst reports" },
    { name: "reviewer",    agent: "reviewer",    task: "Review implementer output" }
  ]
})
```

```markdown
## Phase 3: Coordinate

- Use SendMessage({to: "implementer", message: "..."}) to relay analyst findings
- Use TaskList to monitor progress
- Cap Producer-Reviewer loops at 3 rounds
```

```markdown
## Phase 4: Synthesize

- Read all _workspace/<harness>/*-output.md files
- Resolve conflicts between agent outputs
- Write final.md or apply code changes directly
```

The orchestrator is just a skill file — it runs in the main Claudio session as the team lead, with full access to all tools including `TeamCreate` and `SendMessage`.

---

| Feature | Description |
|---------|-------------|
| **Permission modes** | `default` (ask), `auto` (allow all), `plan` (read-only) |
| **Permission rules** | Content-pattern matching (`allow: Bash(git *)`, `deny: Write(*.env)`) |
| **Cost thresholds** | Configurable cost confirmation dialog (`costConfirmThreshold`) |
| **Trust system** | Projects with hooks/MCP require explicit trust |
| **Audit trail** | All tool executions logged to SQLite (`/audit`) |
| **Secret scanning** | Tool output scanned and redacted for API keys/tokens |
| **Path safety** | `denyPaths` / `allowPaths` / `denyTools` in settings |

---

## Hooks

19 lifecycle events for automation and custom workflows. Configure in `settings.json` under `"hooks"`:

```json
{
  "hooks": {
    "PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "echo $CLAUDIO_TOOL_NAME"}]}],
    "PostCompact": [{"matcher": "*", "hooks": [{"type": "command", "command": "notify-send 'Compacted'"}]}]
  }
}
```

| Event | When it fires |
|-------|---------------|
| `PreToolUse` / `PostToolUse` / `PostToolUseFailure` | Before/after tool execution |
| `PreCompact` / `PostCompact` | Before/after conversation compaction |
| `SessionStart` / `SessionEnd` | Session lifecycle |
| `Stop` | After AI finishes responding |
| `UserPromptSubmit` | Before processing user input |
| `SubagentStart` / `SubagentStop` | Before/after sub-agent execution |
| `TaskCreated` / `TaskCompleted` | Task lifecycle |
| `WorktreeCreate` / `WorktreeRemove` | Git worktree lifecycle |
| `ConfigChange` | When a setting is changed |
| `CwdChanged` | Working directory change |
| `FileChanged` | Watched file modified |
| `Notification` | System notification |

Hooks receive context via environment variables: `CLAUDIO_EVENT`, `CLAUDIO_TOOL_NAME`, `CLAUDIO_SESSION_ID`, `CLAUDIO_MODEL`, `CLAUDIO_TASK_ID`, `CLAUDIO_WORKTREE_PATH`, `CLAUDIO_CONFIG_KEY`, `CLAUDIO_FILE_PATH`. Exit code 1 blocks the action (for `PreToolUse`).

---

## Scheduled Tasks (Cron)

Schedule recurring agent tasks:

```json
// Via the CronCreate tool or programmatically
{"schedule": "@every 1h", "prompt": "Check for failing tests"}
{"schedule": "@daily", "prompt": "Review open PRs"}
{"schedule": "09:00", "prompt": "Summarize overnight changes"}
```

Supported schedules: `@every <duration>` (e.g., `1h`, `30m`), `@daily`, `@hourly`, `HH:MM`. Due tasks execute as background agents at session start.

---

## Session Sharing

Export and import sessions across machines:

```bash
# Export current session
/share my-session.json

# Import on another machine
/teleport my-session.json
```

The shared file contains messages, model, summary, and metadata.

---

## Plugins

Executable scripts or binaries in `~/.claudio/plugins/` are auto-discovered:

```bash
# Create a plugin
echo '#!/bin/bash
echo "Hello from plugin!"' > ~/.claudio/plugins/greet.sh
chmod +x ~/.claudio/plugins/greet.sh

# List plugins
/plugins

# Run a plugin (registered as /greet)
/greet
```

Plugins receive env vars: `CLAUDIO_SESSION_ID`, `CLAUDIO_MODEL`, `CLAUDIO_CWD`. Use `--describe` flag to provide a description.

### claudio-codex — Pre-built Code Index Plugin

[**claudio-codex**](https://github.com/Abraxas-365/claudio-codex) is a first-party plugin (written in Go, using tree-sitter) that builds a structural index of your codebase and exposes it as a deferred tool. Instead of burning thousands of tokens on repeated Grep/Read sweeps, the AI can answer "where is X defined?", "what calls Y?", or "what's the impact of changing Z?" in ~50 tokens.

**Install (one-liner):**

```bash
curl -fsSL https://raw.githubusercontent.com/Abraxas-365/claudio-codex/main/install.sh | sh
```

**Or build from source:**

```bash
git clone https://github.com/Abraxas-365/claudio-codex
cd claudio-codex
make install-plugin   # builds and copies binary into ~/.claudio/plugins/
```

**Index your project:**

```bash
cd your-project
claudio-codex index   # run once; queries auto-refresh on subsequent calls
```

**Supported commands** (invoked by the AI via the deferred tool):

| Command | Description |
|---------|-------------|
| `search <query>` | Search for symbols by name |
| `refs <symbol>` | Find all call sites referencing a symbol |
| `context <symbol>` | Full context: definition + source + callers + callees |
| `impact <symbol> [depth]` | Show transitive callers (blast radius of a change) |
| `trace <symbol> [depth]` | Show outgoing calls from a symbol |
| `outline <file>` | List all symbols in a file |
| `structure` | High-level codebase overview |
| `hotspots [limit]` | Most-referenced symbols |

Once installed and indexed, Claudio automatically prefers `claudio-codex` over raw file searches for symbol lookups — dramatically reducing token usage on large codebases.

---

## Model Configuration

### Multi-Provider Support

Claudio supports routing models to different API providers (Groq, OpenAI, Ollama, Together, vLLM, or any OpenAI-compatible endpoint) alongside the default Anthropic backend.

Configure providers and routing rules in your settings (`~/.claudio/settings.json` or `.claudio/settings.json`):

```json
{
  "providers": {
    "groq": {
      "apiBase": "https://api.groq.com/openai/v1",
      "apiKey": "$GROQ_API_KEY",
      "type": "openai"
    },
    "openai": {
      "apiBase": "https://api.openai.com/v1",
      "apiKey": "$OPENAI_API_KEY",
      "type": "openai"
    },
    "ollama": {
      "apiBase": "http://localhost:11434/v1",
      "type": "openai"
    }
  },
  "modelRouting": {
    "llama-*": "groq",
    "mixtral-*": "groq",
    "gemma*": "groq",
    "gpt-*": "openai",
    "o1*": "openai",
    "qwen*": "ollama"
  }
}
```

| Field | Description |
|-------|-------------|
| `providers.<name>.apiBase` | Base URL for the provider's API |
| `providers.<name>.apiKey` | API key (plain string or `$ENV_VAR` to read from environment) |
| `providers.<name>.type` | `"openai"` for OpenAI-compatible APIs, `"anthropic"` for Anthropic-compatible |
| `modelRouting.<pattern>` | Glob pattern mapping model names to a provider name |

Models that don't match any routing pattern use the default Anthropic backend. To use a routed model, set it with `--model` or in `settings.json`:

```bash
# Use Groq's Llama model
claudio --model llama-3.3-70b-versatile

# Use OpenAI
claudio --model gpt-4o

# Use local Ollama
claudio --model qwen2.5-coder
```

Thinking, effort, and prompt caching features are Anthropic-only and are automatically skipped for non-Anthropic providers.

### Extended Thinking

Control the model's reasoning process:

| Mode | Setting | Description |
|------|---------|-------------|
| Auto | `""` | Adaptive thinking for supported models (default) |
| Adaptive | `"adaptive"` | Model decides when and how much to think |
| Enabled | `"enabled"` | Always think with a configurable token budget |
| Disabled | `"disabled"` | No extended thinking |

When using `enabled` mode, set `budgetTokens` (e.g., `32000` for 32k tokens).

### Effort Level

Control reasoning depth independently from thinking:

| Level | Description |
|-------|-------------|
| `low` | Quick, minimal overhead |
| `medium` | Balanced speed and intelligence (default) |
| `high` | Comprehensive, extensive reasoning |

Configure in settings or switch at runtime via `/model`.

### Model Capabilities Cache

Model capabilities (context window, max output tokens) are cached in `~/.claudio/cache/model-capabilities.json`. Falls back to hardcoded defaults if no cache exists.

---

## Output Styles

Control response formatting with `/output-style` or the `outputStyle` setting:

| Style | Description |
|-------|-------------|
| `normal` | Default behavior |
| `concise` | Brief, direct responses. Skip preamble and summaries. |
| `verbose` | Detailed explanations with reasoning and examples. |
| `markdown` | Well-structured Markdown with headers, code blocks, tables. |

---

## Snippet Expansion (Experimental)

> **Status: Experimental.** This feature is new and the snippet format may change in future releases.

Snippet expansion lets the AI write shorthand like `~errw(db.Query(ctx, id), "fetch user")` instead of full boilerplate. A deterministic expander replaces the shorthand with the full code before writing to disk -- zero extra AI tokens spent on the expansion.

The expander is **context-aware**: for Go files, it parses the enclosing function's return types using `go/ast` and fills in the correct zero values automatically. For Python, TypeScript, JavaScript, and Rust, it uses regex-based resolution.

### Why

Every time the AI writes `if err != nil { return ... }`, it spends ~40 tokens on mechanical boilerplate. With snippets, it writes `~errw(call, msg)` (~15 tokens) and the expander handles the rest. Across a session with dozens of error-handling sites, the savings compound.

### Configuration

Enable in `~/.claudio/settings.json` (global) or `.claudio/settings.json` (project):

```json
{
  "snippets": {
    "enabled": true,
    "snippets": [
      {
        "name": "errw",
        "params": ["call", "msg"],
        "lang": "go",
        "template": "{{.result}}, err := {{.call}}\nif err != nil {\n\treturn {{.ReturnZeros}}, fmt.Errorf(\"{{.msg}}: %w\", err)\n}"
      }
    ]
  }
}
```

### Snippet definition fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | yes | Snippet name (used as `~name(...)` in code) |
| `params` | yes | List of parameter names the AI passes as arguments |
| `template` | yes | Go `text/template` string with `{{.paramName}}` placeholders |
| `lang` | no | File extension filter (`go`, `py`, `ts`, `rs`, etc.). Omit for all languages. |

### Context variables

These are resolved automatically from the surrounding code -- the AI does not fill them in:

| Variable | Description | Languages |
|----------|-------------|-----------|
| `{{.ReturnZeros}}` | Comma-separated zero values for the enclosing function's return types (excluding the final `error`) | Go |
| `{{.FuncName}}` | Name of the enclosing function | Go, Python, TS/JS, Rust |
| `{{.ReturnType}}` | Return type annotation | Python, TS/JS, Rust |
| `{{.result}}` | Default variable name for the result (`result` if not overridden) | All |

---

### Go examples

#### Standard error wrapping

```json
{
  "name": "errw",
  "params": ["call", "msg"],
  "lang": "go",
  "template": "{{.result}}, err := {{.call}}\nif err != nil {\n\treturn {{.ReturnZeros}}, fmt.Errorf(\"{{.msg}}: %w\", err)\n}"
}
```

`~errw(db.QueryRow(ctx, id), "query user")` inside `func GetUser(id int) (*User, error)` expands to:

```go
result, err := db.QueryRow(ctx, id)
if err != nil {
    return nil, fmt.Errorf("query user: %w", err)
}
```

`ReturnZeros` is resolved from the enclosing function: `nil` for pointers/interfaces/slices/maps, `0` for numeric types, `""` for strings. For `(string, int, error)` it produces `"", 0`.

#### Custom error libraries (errx, pkg/errors, etc.)

Templates are just strings -- they can produce any valid code. Projects with custom error types like `errx` can define snippets that match their conventions:

```json
{
  "snippets": {
    "enabled": true,
    "snippets": [
      {
        "name": "errw",
        "params": ["call", "msg"],
        "lang": "go",
        "template": "{{.result}}, err := {{.call}}\nif err != nil {\n\treturn {{.ReturnZeros}}, errx.Wrap(err, \"{{.msg}}\", errx.TypeInternal)\n}"
      },
      {
        "name": "errwt",
        "params": ["call", "msg", "type"],
        "lang": "go",
        "template": "{{.result}}, err := {{.call}}\nif err != nil {\n\treturn {{.ReturnZeros}}, errx.Wrap(err, \"{{.msg}}\", errx.Type{{.type}})\n}"
      },
      {
        "name": "errn",
        "params": ["call"],
        "lang": "go",
        "template": "{{.result}}, err := {{.call}}\nif err != nil {\n\treturn {{.ReturnZeros}}, err\n}"
      },
      {
        "name": "errd",
        "params": ["errfn"],
        "lang": "go",
        "template": "return {{.ReturnZeros}}, {{.errfn}}"
      },
      {
        "name": "errdc",
        "params": ["code", "cause"],
        "lang": "go",
        "template": "return {{.ReturnZeros}}, ErrRegistry.NewWithCause({{.code}}, {{.cause}})"
      },
      {
        "name": "errdd",
        "params": ["errfn", "key", "val"],
        "lang": "go",
        "template": "return {{.ReturnZeros}}, {{.errfn}}.WithDetail(\"{{.key}}\", {{.val}})"
      }
    ]
  }
}
```

Usage in a DDD service:

```go
func (s *ApplicationService) WithdrawApplication(
    ctx context.Context,
    applicationID kernel.ApplicationID,
    candidateID kernel.CandidateID,
) error {
    // Wrap with internal type (most common)
    ~errw(s.appRepo.GetByID(ctx, applicationID), "fetch application")

    // Wrap with explicit type
    ~errwt(s.tenantRepo.FindByID(ctx, req.TenantID), "find tenant", NotFound)

    // Propagate error as-is
    ~errn(s.appRepo.Update(ctx, app))

    // Return a domain error from a registry
    ~errd(ErrApplicationNotFound())

    // Registry error with underlying cause
    ~errdc(CodeApplicationNotFound, err)

    // Domain error with detail metadata
    ~errdd(ErrUnauthorizedAccess(), "candidate_id", candidateID)
}
```

`~errwt(s.tenantRepo.FindByID(ctx, req.TenantID), "find tenant", NotFound)` expands to:

```go
result, err := s.tenantRepo.FindByID(ctx, req.TenantID)
if err != nil {
    return errx.Wrap(err, "find tenant", errx.TypeNotFound)
}
```

`~errd(ErrApplicationNotFound())` expands to:

```go
return application.ErrApplicationNotFound()
```

The `errdc` and `errdd` snippets work the same way -- the arguments are passed through literally, so they work with any registry or error constructor across your modules.

#### Test scaffolding

```json
{
  "name": "test",
  "params": ["name"],
  "lang": "go",
  "template": "func Test{{.name}}(t *testing.T) {\n\tt.Run(\"{{.name}}\", func(t *testing.T) {\n\t\t// TODO\n\t})\n}"
}
```

`~test(GetUser)` expands to:

```go
func TestGetUser(t *testing.T) {
    t.Run("GetUser", func(t *testing.T) {
        // TODO
    })
}
```

#### HTTP handler (Fiber / Chi / stdlib)

```json
{
  "name": "handler",
  "params": ["name", "method"],
  "lang": "go",
  "template": "func (h *Handlers) {{.name}}(c *fiber.Ctx) error {\n\tctx := c.Context()\n\t// TODO\n\treturn c.JSON(fiber.Map{\"ok\": true})\n}"
}
```

`~handler(CreateJob, POST)` expands to:

```go
func (h *Handlers) CreateJob(c *fiber.Ctx) error {
    ctx := c.Context()
    // TODO
    return c.JSON(fiber.Map{"ok": true})
}
```

---

### Python examples

#### Try/except with logging

```json
{
  "name": "tryw",
  "params": ["call", "msg"],
  "lang": "py",
  "template": "try:\n    result = {{.call}}\nexcept Exception as e:\n    raise RuntimeError(\"{{.msg}}\") from e"
}
```

`~tryw(db.fetch_user(user_id), "fetch user failed")` expands to:

```python
try:
    result = db.fetch_user(user_id)
except Exception as e:
    raise RuntimeError("fetch user failed") from e
```

#### FastAPI endpoint

```json
{
  "name": "endpoint",
  "params": ["method", "path", "name"],
  "lang": "py",
  "template": "@router.{{.method}}(\"{{.path}}\")\nasync def {{.name}}(request: Request):\n    pass"
}
```

`~endpoint(post, /api/users, create_user)` expands to:

```python
@router.post("/api/users")
async def create_user(request: Request):
    pass
```

#### Pytest function

```json
{
  "name": "test",
  "params": ["name"],
  "lang": "py",
  "template": "def test_{{.name}}():\n    # Arrange\n\n    # Act\n\n    # Assert\n    assert True"
}
```

`~test(create_user_validates_email)` expands to:

```python
def test_create_user_validates_email():
    # Arrange

    # Act

    # Assert
    assert True
```

#### Pydantic model

```json
{
  "name": "model",
  "params": ["name"],
  "lang": "py",
  "template": "class {{.name}}(BaseModel):\n    class Config:\n        from_attributes = True"
}
```

---

### TypeScript / JavaScript examples

#### Try/catch with typed error

```json
{
  "name": "tryw",
  "params": ["call", "msg"],
  "lang": "ts",
  "template": "try {\n  const result = {{.call}};\n} catch (error) {\n  throw new Error(\"{{.msg}}\", { cause: error });\n}"
}
```

`~tryw(await fetchUser(id), "failed to fetch user")` expands to:

```typescript
try {
  const result = await fetchUser(id);
} catch (error) {
  throw new Error("failed to fetch user", { cause: error });
}
```

#### React component

```json
{
  "name": "component",
  "params": ["name"],
  "lang": "tsx",
  "template": "interface {{.name}}Props {}\n\nexport function {{.name}}({}: {{.name}}Props) {\n  return <div />;\n}"
}
```

`~component(UserProfile)` expands to:

```tsx
interface UserProfileProps {}

export function UserProfile({}: UserProfileProps) {
  return <div />;
}
```

#### Express / Next.js API handler

```json
{
  "name": "api",
  "params": ["name"],
  "lang": "ts",
  "template": "export async function {{.name}}(req: Request): Promise<Response> {\n  try {\n    // TODO\n    return Response.json({ ok: true });\n  } catch (error) {\n    return Response.json({ error: \"Internal error\" }, { status: 500 });\n  }\n}"
}
```

#### Jest / Vitest test

```json
{
  "name": "test",
  "params": ["desc"],
  "lang": "ts",
  "template": "describe(\"{{.desc}}\", () => {\n  it(\"should work\", () => {\n    // Arrange\n\n    // Act\n\n    // Assert\n    expect(true).toBe(true);\n  });\n});"
}
```

---

### Rust examples

#### Result error propagation with context

```json
{
  "name": "errw",
  "params": ["call", "msg"],
  "lang": "rs",
  "template": "let {{.result}} = {{.call}}.map_err(|e| anyhow::anyhow!(\"{{.msg}}: {}\", e))?;"
}
```

`~errw(db.get_user(id).await, "fetch user")` expands to:

```rust
let result = db.get_user(id).await.map_err(|e| anyhow::anyhow!("fetch user: {}", e))?;
```

#### thiserror custom error variant

```json
{
  "name": "errd",
  "params": ["variant", "msg"],
  "lang": "rs",
  "template": "return Err(Error::{{.variant}}(\"{{.msg}}\".into()));"
}
```

#### Test function

```json
{
  "name": "test",
  "params": ["name"],
  "lang": "rs",
  "template": "#[test]\nfn test_{{.name}}() {\n    // Arrange\n\n    // Act\n\n    // Assert\n}"
}
```

#### Async test (tokio)

```json
{
  "name": "atest",
  "params": ["name"],
  "lang": "rs",
  "template": "#[tokio::test]\nasync fn test_{{.name}}() {\n    // Arrange\n\n    // Act\n\n    // Assert\n}"
}
```

#### Impl block

```json
{
  "name": "impl",
  "params": ["type"],
  "lang": "rs",
  "template": "impl {{.type}} {\n    pub fn new() -> Self {\n        Self {}\n    }\n}"
}
```

---

### Cross-language snippets

Snippets without a `lang` field expand in any file. Language-tagged snippets only expand in matching files:

```json
{
  "snippets": {
    "enabled": true,
    "snippets": [
      {
        "name": "todo",
        "params": ["msg"],
        "template": "// TODO: {{.msg}}"
      },
      {
        "name": "errw",
        "params": ["call", "msg"],
        "lang": "go",
        "template": "{{.result}}, err := {{.call}}\nif err != nil {\n\treturn {{.ReturnZeros}}, errx.Wrap(err, \"{{.msg}}\", errx.TypeInternal)\n}"
      },
      {
        "name": "tryw",
        "params": ["call", "msg"],
        "lang": "py",
        "template": "try:\n    result = {{.call}}\nexcept Exception as e:\n    raise RuntimeError(\"{{.msg}}\") from e"
      }
    ]
  }
}
```

### Global vs. project config

| Scope | File | Behavior |
|-------|------|----------|
| Global | `~/.claudio/settings.json` | Base snippets available in all projects |
| Project | `.claudio/settings.json` | Can override `enabled` flag and add project-specific snippets |

Project config extends global: if global defines `errw` and project defines `handler`, both are available. If the project sets `"enabled": false`, all snippets are disabled for that project regardless of global setting.

> **Tip:** You can use the `/setup-snippets` skill to quickly set up snippets for your project. Just run it and Claudio will generate snippet definitions tailored to your codebase.

### How it works internally

1. When snippets are enabled, their documentation is injected into the system prompt (once, at session start -- prompt cache friendly)
2. The AI writes `~name(args)` in code passed to the Write or Edit tool
3. Before content hits disk, the expander finds `~name(...)` patterns, parses arguments (respecting nested parens and string literals), resolves context variables from the file, and executes the template
4. The expanded code is what actually gets written

Unknown snippet names pass through unchanged. If the template fails, an error comment is inserted instead. The AI can always fall back to writing full code.

---

## Keybinding Customization

Create `~/.claudio/keybindings.json` to override default shortcuts:

```json
[
  {"keys": "space b n", "action": "next_session", "context": "normal"},
  {"keys": "ctrl+s", "action": "open_sessions", "context": "global"}
]
```

Run `/keybindings` to open the config in your editor. Reserved keys (`ctrl+c`, `esc`) cannot be rebound.

---

## Per-Turn Diff Tracking

Claudio tracks file changes per conversation turn:

```bash
# Show what changed during turn 3
/diff turn 3

# Show current git diff (unchanged)
/diff
```

---

## Web UI

Claudio ships a full browser-based chat interface — useful when you're on a remote machine, want to share access with a teammate, or just prefer a GUI over the terminal.

```bash
claudio web --port 3000 --password mysecret
# → http://127.0.0.1:3000
```

### Starting the server

| Flag | Default | Description |
|------|---------|-------------|
| `--port` | `3000` | Port to listen on |
| `--host` | `127.0.0.1` | Bind address (`0.0.0.0` to expose on LAN) |
| `--password` | _(required)_ | Password for the login page |

The server uses a session cookie (24 h expiry) — no API key is ever sent to the browser.

### Features

#### Multi-session workspace
- Open multiple independent sessions per project from the sidebar
- Create, rename, and delete sessions without losing conversation history
- Switch between sessions instantly; each keeps its own context and token counters

#### Full chat streaming
- AI responses stream token-by-token in real time via SSE
- Thinking blocks (extended reasoning) rendered inline with a collapsible header
- Tool calls shown with name + input as they execute, result shown when done
- Markdown rendered with syntax-highlighted code blocks

#### Tool approval (interactive)
When the AI calls a tool that requires permission, an overlay appears mid-stream:

```
┌─────────────────────────────────────┐
│ ⚠ Tool Requires Approval            │
│  Bash                               │
│  ┌───────────────────────────────┐  │
│  │ rm -rf ./build                │  │
│  └───────────────────────────────┘  │
│                  [Deny]  [Approve]  │
└─────────────────────────────────────┘
```

Approving or denying resumes the stream immediately.

#### Plan mode approval (inline card)
When the AI finishes planning (`ExitPlanMode`), an inline card appears in the chat:

```
┌─────────────────────────────────────┐  ← yellow border
│ 📄 Plan Ready for Review            │
│ The AI has finished planning.       │
├─────────────────────────────────────┤
│ [✓ Approve (auto-accept)]  [✓ Approve│
│ [✗ Reject]  [✎ Feedback]            │
└─────────────────────────────────────┘
```

- **Approve (auto-accept)** — proceed with implementation, auto-accept all file edits
- **Approve** — proceed, manually approve each edit
- **Reject** — ask the AI to revise the plan
- **Feedback** — opens a text input; your note is sent as the next message

#### AskUser (inline card)
When the AI needs clarification (`AskUser`), an inline card appears:

```
┌─────────────────────────────────────┐  ← blue border
│ ❓ Question from AI                  │
│ Which database should I use?        │
│ [PostgreSQL]  [SQLite]  [MongoDB]   │
└─────────────────────────────────────┘
```

If the AI provides options, they appear as buttons. Otherwise a free-text input is shown. Your choice is sent as the next message.

#### Model selector
Click the **MODEL** badge in the status bar (or the model row in the Config panel) to open the model picker:

- Lists all supported models (Opus 4.6, Sonnet 4.6, Haiku 4.5, and any configured external providers)
- Highlights the currently active model
- Takes effect immediately for the current session

#### Config panel
The right-side Config panel shows:
- Current model (clickable — opens model selector)
- Permission mode
- Project path

#### Analytics panel
Live token counters per session:
- Input / output tokens
- Cache read / cache create tokens
- Total token count

#### Tasks panel
Displays tasks created by the AI via the `TaskCreate` tool, with status badges (`pending`, `in_progress`, `done`).

#### Autocomplete
- `@filename` — file path autocomplete from the project tree
- `/command` — slash command list
- `@agent` — agent name list

### Architecture notes

- The server is a single Go binary — no Node.js, no build step
- HTML is rendered server-side with [templ](https://templ.guide); no SPA framework
- Streaming uses Server-Sent Events (SSE) with a replay buffer for reconnects
- Each browser session maps 1:1 to a `query.Engine` instance, preserving full conversation context across messages
- Auth uses a secure random token in an `HttpOnly` cookie — the Anthropic API key never leaves the server

---

## Headless / API Mode

```bash
claudio --headless
```

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/v1/messages` | POST | Send message (streaming via SSE) |
| `/v1/tools` | GET | List available tools |
| `/v1/health` | GET | Health check |
| `/v1/status` | GET | Session status |

---

## Filesystem Layout

```
~/.claudio/                    # Global config directory
  settings.json                # User settings
  local-settings.json          # Machine-local overrides
  credentials.json             # Auth credentials
  claudio.db                   # SQLite (sessions, messages, audit)
  instincts.json               # Learned patterns
  memory/                      # Global memories
  agents/                      # Custom agent definitions
  skills/                      # User skills
  rules/                       # User rules
  contexts/                    # Context profiles
  plugins/                     # Executable plugins
  plans/                       # Plan mode files
  cache/                       # Model capabilities cache
  cron.json                    # Scheduled task definitions
  keybindings.json             # Custom keybindings (user-created)
  projects/                    # Per-project data
    <project-slug>/memory/     # Project-scoped memories

.claudio/                      # Per-project config (created by /init or claudio init)
  settings.json                # Project settings (overrides global)
  rules/                       # Project rules
  skills/                      # Project skills
  agents/                      # Project agents
  memory/                      # Project memories
CLAUDIO.md                     # Project instructions
```

---

## Architecture

Built with:
- **[Bubbletea](https://github.com/charmbracelet/bubbletea)** -- TUI framework (Elm architecture)
- **[Lipgloss](https://github.com/charmbracelet/lipgloss)** -- Terminal styling
- **[Cobra](https://github.com/spf13/cobra)** -- CLI framework
- **[modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite)** -- Pure Go SQLite (no CGO)

### Key packages

| Package | Purpose |
|---------|---------|
| `internal/query` | Conversation loop, streaming, tool execution |
| `internal/tools` | Tool definitions and registry |
| `internal/agents` | Agent definitions and crystallization |
| `internal/services/memory` | Scoped memory, extraction, AI selection |
| `internal/tasks` | Background task runtime |
| `internal/teams` | Multi-agent coordination |
| `internal/tui` | Terminal UI (viewport, panels, vim, search) |
| `internal/config` | Config loading, merging, trust |
| `internal/hooks` | 19 lifecycle event hooks |
| `internal/security` | Audit, secret scanning, path safety |
| `internal/permissions` | Content-pattern permission rules |
| `internal/models` | Model capabilities cache |
| `internal/keybindings` | Customizable keyboard shortcuts |
| `internal/plugins` | Plugin discovery and execution |
| `internal/snippets` | Context-aware snippet expansion for Write/Edit tools |

---

## License

MIT
