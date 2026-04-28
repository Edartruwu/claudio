<div align="center">

```
  ██████╗██╗      █████╗ ██╗   ██╗██████╗ ██╗ ██████╗
 ██╔════╝██║     ██╔══██╗██║   ██║██╔══██╗██║██╔═══██╗
 ██║     ██║     ███████║██║   ██║██║  ██║██║██║   ██║
 ██║     ██║     ██╔══██║██║   ██║██║  ██║██║██║   ██║
 ╚██████╗███████╗██║  ██║╚██████╔╝██████╔╝██║╚██████╔╝
  ╚═════╝╚══════╝╚═╝  ╚═╝ ╚═════╝ ╚═════╝ ╚═╝ ╚═════╝
```

### The Neovim of AI coding agents

**Configure everything in Lua · Parallel multi-agent teams · Vim-grade TUI · Single Go binary**

[![Go Version](https://img.shields.io/badge/go-1.26%2B-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](#license)
[![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux-lightgrey)](#requirements)
[![Pure Go](https://img.shields.io/badge/CGO-free-success)](#key-constraints)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](#contributing)

[**Quick Start**](#-quick-start) · [**Install**](#-installation) · [**Features**](#-features) · [**Why Claudio?**](#-why-claudio) · [**Docs**](#table-of-contents)

</div>

---

## The Philosophy

Claudio is to AI coding what Neovim is to text editing.

The binary ships with sensible defaults and a complete feature set. Everything else — keymaps, colorschemes, providers, tools, commands, hooks, sidebar widgets — is configured in `~/.claudio/init.lua`. No flags, no JSON soup, no recompiling. Your config lives in one file and travels with you.


```lua
-- ~/.claudio/init.lua — the only config file you need

claudio.colorscheme("tokyonight")
claudio.config.set("model", "claude-opus-4-6")

claudio.register_provider({
  name = "groq", type = "openai",
  base_url = "https://api.groq.com/openai/v1",
  api_key = "$GROQ_API_KEY",
  routes = { "llama-*" },
})

claudio.register_keymap({ mode = "normal", key = "K", action = "docs",
  handler = function() claudio.notify("docs") end })
```

---

## ✨ Features

<table>
<tr>
<td width="50%" valign="top">

### Lua Runtime (Neovim-style)
`~/.claudio/init.lua` controls everything: model, theme, keymaps, providers, commands, hooks, capabilities, sidebar blocks. Same philosophy as Neovim — binary ships compiled defaults, your Lua overrides them. No recompile ever.

</td>
<td width="50%" valign="top">

### Parallel Multi-Agent Teams
Real agent parallelism — not just sub-agents. `Prab` (your PM) plans, creates tasks, and spawns specialists into isolated git worktrees. Workers run simultaneously via goroutines, communicate via file-based mailboxes, and merge back when done.

</td>
</tr>
<tr>
<td valign="top">

### Vim-Grade TUI
Full modal state machine — normal, insert, visual, operator-pending — with registers, text objects, counts, and `.` repeat. Press `:` to open a Neovim-style command line: `:set model opus`, `:colorscheme gruvbox`, `:lua claudio.notify("hi")`.

</td>
<td valign="top">

### Agent Harnesses
Build reusable multi-agent architectures with `/harness`. Six patterns: Pipeline, Fan-out, Expert Pool, Producer-Reviewer, Supervisor, Hierarchical Delegation. Invoked with a single slash command forever after.

</td>
</tr>
<tr>
<td valign="top">

### Scoped Persistent Memory
Three-scope facts-based memory (project / global / agent). Background extraction after every session. `/dream` consolidation detects contradictions. `Recall` semantic search. Cache-safe injection that never breaks prompt caching.

</td>
<td valign="top">

### Two-Brain Advisor
Cheap executor (Haiku) does the work; expensive advisor (Opus) consults at PLAN and REVIEW — at most twice per task. Senior judgment at a fraction of the cost. Configurable per-agent in team templates.

</td>
</tr>
<tr>
<td valign="top">

### 11-Layer Token Efficiency
Prompt caching, microcompaction, disk offload for large results, duplicate read dedup, image compression, output filtering (38 built-in commands), Lua filter engine, source-code filter, message merging, deferred tool schemas, snippet expansion.

</td>
<td valign="top">

### Agent Crystallization
Promote any session into a reusable agent persona with its own memory, tools, and standing instructions. Crystallized agents carry accumulated memory into every team run — no cold-start rebuilding.

</td>
</tr>
<tr>
<td valign="top">

### Lua Plugin System
Community plugins live in `~/.claudio/plugins/*/init.lua`. Install with `claudio plugin install`. Each plugin gets the full `claudio.*` API — register tools, skills, commands, providers, keymaps, hooks exactly like your personal `init.lua`.

</td>
<td valign="top">

### Command Center
`comandcenter` — a WhatsApp-style browser PWA for remote sessions, push notifications, file uploads, and multi-session management. Attach any number of `claudio` sessions to a single hub.

</td>
</tr>
<tr>
<td valign="top">

### Scheduled Tasks
Cron-style recurring agent jobs: `@every 1h`, `@daily`, `HH:MM`. Inline or background execution. Shared across all sessions when running with `comandcenter`.

</td>
<td valign="top">

### Single Go Binary
Pure Go, zero runtime dependencies. `modernc.org/sqlite` keeps it CGO-free. `go install` in one line.

</td>
</tr>
</table>

---

## 🚀 Quick Start

```bash
# 1. Install
go install github.com/Abraxas-365/claudio/cmd/claudio@latest

# 2. Authenticate
claudio auth login          # Anthropic OAuth — or set ANTHROPIC_API_KEY

# 3. Bootstrap your project
cd your-project
claudio                     # launches the TUI
/init                       # AI-guided project setup: CLAUDIO.md + skills + hooks

# 4. Start building
```

> **Tip:** `claudio --resume` picks up your last session. `claudio "fix the failing test"` runs a one-shot prompt.

### Use any model or provider

```lua
-- ~/.claudio/init.lua
claudio.register_provider({
  name     = "groq",
  type     = "openai",
  base_url = "https://api.groq.com/openai/v1",
  api_key  = "$GROQ_API_KEY",
  routes   = { "llama-*" },
})

claudio.register_provider({
  name     = "ollama",
  type     = "ollama",
  base_url = "http://localhost:11434",
  routes   = { "qwen*" },
})

claudio.config.set("model", "llama-3.3-70b-versatile")
```

```bash
claudio --model gpt-4o                   # OpenAI
claudio --model llama-3.3-70b-versatile  # Groq
claudio --model qwen2.5-coder            # Local Ollama
```

Or switch live: `:set model llama-3.3-70b-versatile`

### Spawn a full agent team

```
claudio
/agent              ← pick your principal agent (orchestrator / PM)
/team               ← pick a team template (worker roster)

"Build the OAuth module with JWT tokens"

Lead agent:
  → Explores codebase, creates plan, asks one clarifying question
  → TaskCreate × 3 (service layer, migrations, tests)
  → SpawnTeammate (backend-mid)    (parallel, isolated worktree)
  → SpawnTeammate (backend-jr)     (parallel, isolated worktree)
  → SpawnTeammate (backend-senior) (parallel, isolated worktree)
  → Merges branches, runs build, reports back
```

---

## 🆚 Why Claudio?

Claudio is built ground-up in Go for engineers who want **more control, more agents, and fewer dependencies**.

|  | **Claudio** | Claude Code |
|---|---|---|
| 🏗️ **Runtime** | Single Go binary — zero runtime deps | Node.js / TypeScript |
| 🔌 **Extensibility** | Full Lua runtime — tools, keymaps, themes, providers, hooks from `init.lua`. No recompile. | Extension API in beta |
| 🤝 **Multi-agent teams** | Parallel workers in isolated worktrees, mailbox messaging, `/harness` patterns | ❌ |
| 💎 **Session-as-agent** | Crystallize sessions into reusable personas with accumulated memory | ❌ |
| 🧠 **Memory** | Scoped (project/agent/global), facts-based, `Recall` semantic search, `/dream` consolidation, cache-safe | Single directory |
| 🗜️ **Token efficiency** | 11-layer optimization stack | Basic prompt caching |
| 📦 **Plugins** | Lua plugins via `~/.claudio/plugins/` — full `claudio.*` API | ❌ |
| ✂️ **Snippet expansion** | `~name(args)` → full boilerplate; zero extra AI tokens | ❌ |
| 🧑‍💼 **Two-Brain Advisor** | Cheap executor + expensive advisor at PLAN/REVIEW only | ❌ |
| ⏰ **Cron tasks** | `@every 1h`, `@daily`, `HH:MM` — inline or background | Feature-gated |
| 🌐 **Web / Mobile UI** | `comandcenter` — WhatsApp-style PWA, push notifications | ❌ |
| 🌉 **Cross-session comms** | Unix-socket bridge for parallel worktrees | ❌ |
| ⌨️ **Vim mode** | Full state machine + registers + `:` command line (like Neovim) | Basic vi-mode |
| 💾 **Persistence** | SQLite + file-based | File-based only |
| 🔭 **LSP integration** | Config-driven language servers — go-to-definition, find-refs, hover | ❌ |

---

## 📋 Requirements

| | |
|---|---|
| **Go** | 1.26+ (for building from source) |
| **OS** | macOS, Linux (Windows experimental) |
| **Auth** | Anthropic API key or OAuth — Groq, OpenAI, Ollama also supported |
| **Git** | Required for project root detection and worktrees |

**Optional:** `$EDITOR` for external editing · Language servers (gopls, pyright, …) for LSP · MCP servers for extended tools.

---

## 📦 Installation

### Option 1 — `go install` (fastest)

```bash
# CLI / TUI
go install github.com/Abraxas-365/claudio/cmd/claudio@latest

# Command Center server (optional — for the web/mobile UI)
go install github.com/Abraxas-365/claudio/cmd/comandcenter@latest
```

Make sure `$GOPATH/bin` (or `$HOME/go/bin`) is on your `$PATH`.

### Option 2 — From source

```bash
git clone https://github.com/Abraxas-365/claudio
cd claudio
make build              # injects version via ldflags
sudo mv claudio /usr/local/bin/
go build -o comandcenter ./cmd/comandcenter
sudo mv comandcenter /usr/local/bin/
```

### Verify

```bash
claudio --help
claudio --version
```

---

## 🎮 Usage Modes

```bash
claudio                                  # interactive TUI (default)
claudio "explain this codebase"          # one-shot prompt
echo "fix the bug in main.go" | claudio  # pipe mode
claudio --resume                         # resume last session
claudio --headless                       # headless one-shot (no TUI)

# Command Center — browser/mobile UI + remote sessions
comandcenter --password mysecret --port 8080
claudio --attach http://localhost:8080 --name "my-session" --master
```

---

## Table of Contents

- [The Philosophy](#the-philosophy)
- [Why Claudio?](#-why-claudio)
- [Requirements](#-requirements)
- [Installation](#-installation)
- [Quick Start](#-quick-start)
- [Lua Configuration](#lua-configuration)
  - [init.lua — personal config](#initlua--personal-config)
  - [Load order](#load-order)
  - [Lua Plugins](#lua-plugins)
  - [Full API surface](#full-api-surface)
  - [:checkhealth](#checkhealth)
- [Project Setup](#project-setup)
  - [/init — Project setup skill](#init--project-setup-skill)
  - [Configuration hierarchy](#configuration-hierarchy)
  - [Settings reference](#settings-reference)
  - [CLAUDIO.md / CLAUDE.md](#claudemd--claudemd)
  - [Permission Rules](#permission-rules)
- [CLI Flags](#cli-flags)
- [Interactive Commands](#interactive-commands)
- [Keybindings](#keybindings)
- [Vim Mode & `:` Command Line](#vim-mode---command-line)
- [Context Management](#context-management)
- [Token Efficiency](#token-efficiency)
- [Memory System](#memory-system)
- [Tools](#tools)
- [Agents](#agents)
  - [Built-in agent roster](#built-in-agent-roster)
  - [Custom agents](#custom-agents)
  - [Agent crystallization](#agent-crystallization)
- [Orchestrator & Multi-Agent Teams](#orchestrator--multi-agent-teams)
  - [The Perfect Workflow](#the-perfect-workflow)
  - [Team templates](#team-templates)
  - [Two-Brain Advisor](#-two-brain-advisor)
- [Harness — Agent Team Architecture](#harness--agent-team-architecture)
  - [The 6 patterns](#the-6-patterns)
  - [Building a harness with /harness](#building-a-harness-with-harness)
- [Security](#security)
- [Hooks](#hooks)
- [Scheduled Tasks (Cron)](#scheduled-tasks-cron)
- [Session Sharing](#session-sharing)
- [Plugins](#plugins)
- [Model Configuration](#model-configuration)
- [Output Styles](#output-styles)
- [Snippet Expansion](#snippet-expansion-experimental)
- [Keybinding Customization](#keybinding-customization)
- [Per-Turn Diff Tracking](#per-turn-diff-tracking)
- [Command Center (Web / Mobile UI)](#command-center-web--mobile-ui)
- [Headless Mode](#headless-mode)
- [Filesystem Layout](#filesystem-layout)
- [Architecture](#architecture)
- [License](#license)

---

## Lua Configuration

Claudio embeds a Lua runtime (gopher-lua — pure Go, no CGO) that gives you full control over every aspect of your setup without recompiling. The philosophy is the same as Neovim: the binary ships compiled defaults, and your `~/.claudio/init.lua` overrides everything on top.

### `init.lua` — personal config

```lua
-- ~/.claudio/init.lua

-- ── Model & settings ──────────────────────────────────────────
claudio.config.set("model", "claude-opus-4-6")
claudio.config.set("caveman", true)
claudio.config.set("compactMode", "strategic")

-- ── Theme ─────────────────────────────────────────────────────
claudio.colorscheme("tokyonight")
-- or fine-grained:
claudio.ui.set_color("primary", "#7aa2f7")
claudio.ui.set_border("rounded")

-- ── Keymaps ───────────────────────────────────────────────────
claudio.register_keymap({ mode = "normal", key = "K", action = "show_docs",
  handler = function() claudio.notify("docs") end })

-- ── Providers ─────────────────────────────────────────────────
claudio.register_provider({
  name     = "groq",
  type     = "openai",
  base_url = "https://api.groq.com/openai/v1",
  api_key  = "$GROQ_API_KEY",
  routes   = { "llama-*" },
})

claudio.register_provider({
  name     = "ollama",
  type     = "ollama",
  base_url = "http://localhost:11434",
  routes   = { "qwen*" },
})

-- ── Hooks ─────────────────────────────────────────────────────
claudio.subscribe("tool.executed", function(e)
  if e.tool_name == "Bash" then
    claudio.log("[audit] " .. tostring(e.input))
  end
end)

-- ── Commands ──────────────────────────────────────────────────
claudio.register_command({
  name        = "standup",
  description = "Print git log for standup",
  execute     = function(args)
    return "run: git log --oneline --since=yesterday"
  end,
})

-- ── Capabilities ──────────────────────────────────────────────
claudio.register_capability("database", {
  tools = { "SQLQuery", "SchemaInspect", "MigrationRun" }
})

-- ── Sidebar block ─────────────────────────────────────────────
claudio.ui.register_sidebar_block({
  id       = "git-status",
  title    = "Git",
  render   = function() return claudio.cmd("git status --short") end,
  priority = 10,
})
```

### Load order

```
1. internal/lua/defaults.lua   ← embedded binary defaults
2. ~/.claudio/init.lua         ← your personal config
3. ~/.claudio/plugins/*/       ← community plugins
4. .claudio/init.lua           ← project overrides (per-repo, wins over personal)
```

Each layer overrides the one before. Project config wins over personal; personal wins over defaults.

### Lua Plugins

Community plugins are directories under `~/.claudio/plugins/`, each with an `init.lua`:

```
~/.claudio/plugins/
  claudio-jira/
    init.lua
  claudio-database/
    init.lua
```

Install via CLI:
```bash
claudio plugin install https://github.com/someone/claudio-jira
claudio plugin list
claudio plugin remove claudio-jira
claudio plugin info claudio-jira
```

A plugin's `init.lua` receives the same `claudio.*` API — it can register tools, skills, commands, providers, keymaps, capabilities, and hooks exactly like your personal `init.lua`.

### Full API surface

| Namespace | Methods |
|-----------|---------|
| `claudio.*` | `register_tool`, `register_skill`, `register_hook`, `register_command`, `register_provider`, `register_capability`, `register_keymap`, `subscribe`, `publish`, `notify`, `log`, `cmd`, `colorscheme` |
| `claudio.config.*` | `get(key)`, `set(key, val)`, `on_change(key, fn)` |
| `claudio.keymap.*` | `set(mode, key, action, fn)`, `del(mode, key)`, `list(mode)` |
| `claudio.ui.*` | `set_color(slot, hex)`, `set_theme(table)`, `set_border(name)`, `get_colors()`, `set_statusline(fn)`, `popup(opts)`, `register_palette_entry(entry)`, `register_sidebar_block(opts)` |
| `claudio.agent.*` | `current()`, `on_change(fn)`, `add_context(text)`, `set_prompt_suffix(name, text)` |
| `claudio.session.*` | `id()`, `title()`, `on_start(fn)`, `on_end(fn)`, `on_message(fn)` |

**Color slots for `set_color`:** `primary`, `secondary`, `success`, `warning`, `error`, `muted`, `surface`, `surface_alt`, `text`, `dim`, `subtle`, `orange`, `aqua`

**Built-in colorschemes:** `tokyonight`, `gruvbox`, `catppuccin`, `nord`, `dracula`

### `:checkhealth`

Press `:checkhealth` (or `:health`) in the TUI for a diagnostics report:

```
Lua Plugins
  ✓ claudio-jira     (loaded)
  ✗ claudio-broken   (error: attempt to index a nil value)

Capabilities
  design       (4 factories)
  database     (3 factories)

Config
  model:          claude-opus-4-6
  permissionMode: default
  compactMode:    strategic

LSP
  gopls          go, gomod
```

---

## Project Setup

### `/init` — Project setup skill

> **Recommended:** Run `/init` inside the TUI rather than `claudio init`. The TUI version is AI-powered — it surveys your codebase, interviews you, and generates a tailored `CLAUDIO.md`, skills, and hooks.

```
claudio        # start the TUI
/init          # run the init skill
```

The `/init` skill walks through several phases:

1. Asks setup questions (scope, branch conventions, gotchas)
2. Surveys the codebase with a sub-agent
3. Writes `CLAUDIO.md` and optionally `CLAUDIO.local.md` (gitignored personal overrides)
4. Creates project skills under `.claudio/skills/`
5. Suggests hooks and GitHub CLI integrations

**CLI fallback:** `claudio init` creates the `.claudio/` scaffold without the interactive interview.

```
.claudio/
  settings.json      # Project-specific settings (overrides global)
  rules/             # Project-specific rules
    project.md
  skills/            # Project-specific skills
  agents/            # Project-specific agent definitions
  memory/            # Project-scoped memories
  .gitignore
CLAUDIO.md           # Project instructions for the AI
```

### Configuration hierarchy

Settings are resolved with priority (highest first):

```
Environment variables         CLAUDIO_MODEL, CLAUDIO_API_BASE_URL, …
       |
.claudio/init.lua             Project Lua config (per-repo, committed)
       |
~/.claudio/plugins/*/init.lua Community plugins
       |
~/.claudio/init.lua           Personal Lua config — keymaps, theme, providers
       |
internal/lua/defaults.lua     Embedded defaults (compiled into binary)
       |
~/.claudio/state.json         Machine-written state only
```

**Human config lives in `init.lua`**, not JSON. Everything intentional goes in `~/.claudio/init.lua`. `state.json` is machine-written and you never touch it.

**Scalar values** (model, permissionMode) are overridden by higher priority. **Lists** (denyTools, denyPaths) are appended across layers.

### TUI config editor

Open with `<Space>ic`:
- **P** badge = setting from project scope
- **G** badge = setting from global scope
- `tab` to switch scope, `enter` to toggle/cycle (saved immediately)

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
  "autoMemoryExtract": false,
  "memorySelection": "ai",
  "outputStyle": "normal",
  "cavemanMode": "",
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
| `model` | any model ID | Default AI model |
| `thinkingMode` | `""`, `adaptive`, `enabled`, `disabled` | Extended thinking mode |
| `budgetTokens` | token count | Thinking budget when mode is `enabled` |
| `effortLevel` | `low`, `medium`, `high` | Reasoning depth |
| `permissionMode` | `default`, `auto`, `plan` | Tool approval behavior |
| `permissionRules` | array of rules | Content-pattern rules |
| `autoMemoryExtract` | `true`/`false` | Auto-extract memories after each turn |
| `memorySelection` | `ai`, `keyword`, `none` | How memories are selected for system prompt |
| `outputStyle` | `normal`, `concise`, `verbose`, `markdown` | Response formatting |
| `costConfirmThreshold` | USD amount, 0 = disabled | Pause at this cost |
| `denyTools` | list of tool names | Disable specific tools |
| `compactMode` | `auto`, `manual`, `strategic` | When to compact history |
| `compactKeepN` | integer (default `10`) | Messages kept after compaction |
| `maxBudget` | USD, 0 = unlimited | Session spend limit |
| `outputFilter` | `true`/`false` | RTK-style command output filtering |
| `cavemanMode` | `""`, `lite`, `full`, `ultra` | Compressed output mode |
| `toolModels` | `map[string]string` | Per-tool model override |
| `publicUrl` | string | Public base URL for bundle share links |

### CLAUDIO.md / CLAUDE.md

Place a `CLAUDIO.md` or `CLAUDE.md` in your project root. Searched paths (first match wins per directory):
1. `./CLAUDIO.md`
2. `./CLAUDE.md`
3. `./.claudio/CLAUDE.md`

**Subdirectory discovery:** Claudio walks from your cwd up to the git root, loading files at each level.

**@imports:** Include other markdown files:

```markdown
# My Project

@docs/conventions.md
@docs/architecture.md
```

### Permission Rules

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

Rules are evaluated in order; first match wins. Behaviors: `allow` (skip approval), `deny` (block), `ask` (show dialog).

---

## CLI Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--model` | | AI model override |
| `--version` | `-v` | Print version and exit |
| `--resume` | `-r` | Resume last session |
| `--session` | `-s` | Resume session by ID |
| `--headless` | | One-shot without TUI |
| `--no-persist` | | Disable session persistence |
| `--attach` | | Attach to a ComandCenter server |
| `--name` | | Session name (for attach) |
| `--master` | | Mark as master session (for attach) |
| `--permission` | `-p` | Permission mode: `default`, `auto`, `plan` |
| `--cwd` | | Working directory override |

---

## Interactive Commands

### Vim Command Line (`:` mode)

Press `:` in normal vim mode — exactly like Neovim:

| Command | Description |
|---------|-------------|
| `:lua <code>` | Execute Lua live — `:lua claudio.notify("hi")`, `:lua claudio.ui.set_color(...)` |
| `:set <key> [value]` | Read or write any config — `:set model`, `:set caveman true` |
| `:colorscheme <name>` | Switch theme — `tokyonight`, `gruvbox`, `catppuccin`, `nord`, `dracula` |
| `:checkhealth` | Diagnose plugins, capabilities, config, LSP |
| `:health` | Alias for `:checkhealth` |
| `:<command>` | Any `/command` also works as a `:command` |

Plugins can register new `:` commands with `claudio.register_command()`.

### Slash Commands

| Command | Aliases | Description |
|---------|---------|-------------|
| `/help` | `h`, `?` | Show available commands |
| `/model` | `m` | Show or change the AI model |
| `/compact [instruction]` | | Compact conversation history |
| `/cost` | | Show session cost and token usage |
| `/memory extract` | `mem` | Manually extract memories |
| `/session` | `sessions` | List or manage sessions |
| `/resume [id]` | | Resume a previous session |
| `/new` | | Start a new session |
| `/rename [title]` | | Rename the current session |
| `/config` | `settings` | View/edit configuration |
| `/commit` | | Create a git commit with AI-generated message |
| `/diff [args]` | | Show git diff |
| `/status` | | Show git status |
| `/share [path]` | | Export session for sharing |
| `/teleport <path>` | | Import a shared session file |
| `/plugins` | | List installed plugins |
| `/gain` | | Show token savings from output filters |
| `/discover` | | Show commands that ran without a filter |
| `/output-style [style]` | | Show or set output style |
| `/caveman [lite\|full\|ultra\|off]` | | Toggle compressed output mode |
| `/keybindings` | | Open keybindings.json in `$EDITOR` |
| `/vim` | | Toggle vim keybindings |
| `/skills` | | List available skills |
| `/tasks` | | Show background tasks and team status |
| `/agent` | | Pick an agent persona for this session |
| `/team` | | Pick a team template |
| `/dream` | | Consolidate and clean up memories |
| `/audit` | | Show recent tool audit log |
| `/export [format]` | | Export conversation (markdown, json, txt) |
| `/undo` | | Undo the last exchange |
| `/doctor` | | Diagnose environment issues |
| `/mcp` | | Manage MCP servers |
| `/harness <description>` | | Build a reusable multi-agent architecture |
| `/exit` | `quit`, `q` | Exit Claudio |

---

## Keybindings

### Global

| Key | Action |
|-----|--------|
| `Ctrl+C` | Cancel streaming / quit |
| `Ctrl+G` | Open prompt in `$EDITOR` |
| `Ctrl+V` | Paste image from clipboard |
| `Shift+Tab` | Cycle permission mode |
| `Esc` | Dismiss overlays / cancel streaming |

### Viewport (conversation view)

Enter with `<Space>wk` or (in vim normal mode with empty prompt) scroll with `j`/`k`:

| Key | Action |
|-----|--------|
| `j` / `k` | Navigate between message sections |
| `Ctrl+D` / `Ctrl+U` | Jump 5 sections down/up |
| `g` / `G` | Jump to top/bottom |
| `/` | Search messages |
| `p` | Pin/unpin message (pinned = survives compaction) |
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
| `c` | Configuration | View/edit settings with scope badges |
| `m` | Memory | Browse, search, edit, add, delete memories |
| `k` | Skills | Browse available skills |
| `a` | Analytics | Session statistics and cache metrics |
| `t` | Tasks | Background tasks and team agent status |

---

## Vim Mode & `:` Command Line

Toggle vim mode with `/vim`. Full modal state machine:

- **Normal mode** (`Esc`): `hjkl`, `w/b/e` word motion, `f/F/t/T` char search, `.` repeat, `d/c/y` operators, text objects (`iw`, `i"`, `i(`, …), registers (`"a` prefix), counts (`3dw`), `%` bracket matching
- **Insert mode** (`i`): standard editing
- **Visual mode** (`v`): character/line selection with operators
- **Operator-pending mode**: after `d/c/y`

Press `:` in normal mode to open the command line — a live Lua REPL and config interface. Press `Tab` for wildmenu completion on commands and arguments.

### Keybinding Customization

Customize in `~/.claudio/keybindings.json` or via Lua:

```lua
-- ~/.claudio/init.lua
claudio.keymap.set("normal", "<leader>gr", "git_refs", function()
  claudio.cmd("git log --oneline -20")
end)
```

Or via the `/keybindings` command to open the JSON in your editor.

---

## Context Management

### Context budget bar

```
[████████░░] 72%
```

Colors: green (< 70%), yellow (70–90%), red (> 90%). Auto-compaction at 95%.

### Message pinning

Press `p` in viewport mode to pin messages. Pinned messages survive compaction.

### Compaction

Tiered as context fills:
- **70%**: partial compact (clear old tool results)
- **90%**: suggest full compaction
- **95%**: force compact (summarize old messages, keep last N + pinned)

Manual: `/compact [instruction]` — optional instruction guides what the summary focuses on.

---

## Token Efficiency

An 11-layer optimization stack to minimize cost and keep long sessions within the context window.

### Prompt caching

Every request marks the last system prompt block with `cache_control: {type: "ephemeral"}`. Cached input tokens cost ~10× less. In practice the system prompt (instructions, memories, tool descriptions) is only paid for in full once per session.

### Microcompaction

After every tool-execution turn, `MicroCompact` clears read-heavy tool results older than the last 6 and larger than 2 KB. Affected tools: `Bash`, `Read`, `Glob`, `Grep`, `WebFetch`, `WebSearch`, `LSP`, `ToolSearch`. Runs continuously without any threshold.

### Tool result disk offload

Results larger than **50 KB** are written to disk and replaced with a compact placeholder. Cleaned up when the session ends.

### Output filtering (RTK-style)

When `"outputFilter": true`, Bash outputs pass through three filter layers before entering context:

1. **Lua filters** — registered via `claudio.filter.register()` in your `init.lua`; highest priority
2. **38 built-in filters** — Go, Rust, JS/TS, Python, JVM, .NET, Swift, DevOps, Docker, and more
3. **Generic filters** — ANSI stripping, blank line collapse, duplicate line dedup, progress bar removal, long-line truncation

Custom filters live in `.claudio/init.lua` alongside the rest of your config:

```lua
-- .claudio/init.lua
claudio.filter.register("my-tool", {
  match_command        = "^my-tool$",
  strip_ansi           = true,
  max_lines            = 50,
  on_empty             = "my-tool: ok",
  strip_lines_matching = { "^Downloading", "^Resolving" },

  -- optional: full control via transform function
  transform = function(output)
    return output:gsub("^Progress:%s*%d+%%\n", "")
  end,
})
```

**Declarative fields** (all optional):

| Field | Type | Effect |
|---|---|---|
| `match_command` | regex string | Only apply to commands matching this pattern |
| `strip_ansi` | bool | Strip ANSI escape codes |
| `replace` | `{pattern, replacement}[]` | Regex replacements applied in order |
| `strip_lines_matching` | string[] | Remove lines matching any pattern |
| `keep_lines_matching` | string[] | Remove lines **not** matching any pattern |
| `truncate_lines_at` | int | Truncate lines longer than N chars |
| `head_lines` | int | Keep only first N lines |
| `tail_lines` | int | Keep only last N lines |
| `max_lines` | int | Alias for `tail_lines` |
| `on_empty` | string | Return this string if output becomes empty |
| `transform` | function | Called with final output string; return value replaces it |

**Manage registered filters at runtime:**

```lua
claudio.filter.list()       -- returns table of registered filter names
claudio.filter.unregister("my-tool")
```

> **Legacy:** `.claudio/filters.toml` is still loaded and respected, but Lua filters take priority. New projects should use `init.lua`.

Set `CLAUDIO_NO_FILTER=1` to bypass all filters, `CLAUDIO_FILTER_DEBUG=1` to log which filter matched.

### Source-code filter (`codeFilterLevel`)

```json
{ "codeFilterLevel": "minimal" }
```

| Level | Effect |
|-------|--------|
| `none` | Raw file content |
| `minimal` | Strips comments, preserves doc comments, collapses blanks |
| `aggressive` | Function/type signatures + imports only; bodies replaced with `// ...` |

Only applies to full-file reads of files > 500 lines.

### CavemanMode

Reduces output tokens by ~65–75% via terse communication rules:

```json
{ "cavemanMode": "full" }
```

| Mode | Style |
|------|-------|
| `lite` | No filler/hedging, keeps full sentences |
| `full` | Drops articles, fragments OK, `[thing] [action] [reason]` |
| `ultra` | Maximum compression — abbreviations, arrows for causality, one word when one word is enough |

Code blocks and security warnings are always written with full clarity regardless of mode.

### Summary

| Technique | Typical saving |
|-----------|---------------|
| Prompt caching | ~90% discount on system tokens per turn |
| Microcompaction | Continuous reduction of old tool result bulk |
| Tool result disk offload | Caps single-result payload at 50 KB |
| Duplicate read cache | Eliminates redundant file read tokens |
| Image compression | Reduces image payloads to ≤500 KB |
| Output filtering (38 commands) | 60–90% reduction on noisy command outputs |
| Lua filter engine | User-customizable via `init.lua`, supports transform functions |
| Source-code filter | Strips comments from large files |
| Deferred tool schemas | Saves full schema cost for unused tools |
| Snippet expansion | Reduces AI output tokens for repetitive boilerplate |
| CavemanMode | 65–75% reduction in assistant text tokens |

---

## Memory System

Three-scope, facts-based memory. Cache-safe — never breaks prompt caching.

### Scopes

| Scope | Path | Purpose |
|-------|------|---------|
| **Project** | `~/.claudio/projects/<project-slug>/memory/` | Repo conventions, decisions, architecture |
| **Global** | `~/.claudio/memory/` | Cross-project preferences and personal style |
| **Agent** | `~/.claudio/agents/<name>/memory/` | Per-crystallized-agent knowledge |

Resolution priority: **Agent > Project > Global**.

**Scope decision rule:** "Would this be true in a completely different project?"
- Yes → `global` · No → `project` · Persona-specific → `agent`

### Entry format

```markdown
---
name: jwt-config
description: JWT token configuration for this API
type: project
scope: project
tags: [auth, jwt, token]
facts:
  - JWT tokens expire in 24h
  - Refresh threshold is 20h — issue new token if TTL < 4h
  - Secret stored in .env.local under JWT_SECRET
  - Signing algorithm is RS256
concepts:
  - token-lifecycle
  - session-management
---
```

### How the agent uses memory

A **lean memory index** is injected into the first human turn (not the system prompt — cache is never broken):

```
## Your Memory Index

### Project Memories
- jwt-config [auth,jwt]: JWT configuration — "Expires in 24h" | "RS256 signing"
- no-orm [db,sql]: DB rules — "Never use GORM" | "Raw SQL via modernc.org/sqlite"
```

The agent then calls:
- **`Memory(action="read", name="...")`** — load full facts for a specific entry
- **`Recall(context="...")`** — semantic search across all scopes
- **`Memory(action="append", name="...", fact="...")`** — add one fact (no full rewrite)
- **`Memory(action="save", ...)`** — create a new entry

### Background extraction

Background memory extraction is disabled by default. Enable with `"autoMemoryExtract": true`.

### Dream consolidation (`/dream`)

`/dream` runs a consolidation agent that:
1. Lists all existing memories
2. Detects contradictions and deletes stale facts
3. Appends new facts to existing entries
4. Creates new memories for new learnings
5. Promotes project-scope entries to global when they reflect universal preferences

Run `/dream` at the end of a productive session to keep memory accurate.

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

Stored in `~/.claudio/instincts.json`. Confidence-scored patterns that decay after 30 days. Categories: `debugging`, `workflow`, `convention`, `workaround`.

---

## Tools

Core tools are always loaded; deferred tools load on-demand via `ToolSearch`.

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

### Deferred (on-demand via ToolSearch)

| Tool | Description |
|------|-------------|
| **Memory** | Search, list, read persistent memories |
| **Recall** | Semantic memory search |
| **WebSearch** / **WebFetch** | Web search and URL fetching |
| **LSP** | Language server operations |
| **NotebookEdit** | Jupyter notebook editing |
| **TaskCreate/List/Get/Update** | Task management |
| **EnterPlanMode** / **ExitPlanMode** | Planning workflow |
| **EnterWorktree** / **ExitWorktree** | Git worktree isolation |
| **TaskStop** / **TaskOutput** | Background task control |
| **TeamCreate** / **SpawnTeammate** / **SendMessage** | Multi-agent teams |
| **InstantiateTeam** | Restore a saved team template |
| **CronCreate** / **CronDelete** / **CronList** | Scheduled recurring tasks |
| **AskUser** | Ask user structured questions |

Disable any tool: `"denyTools": ["ToolName"]` in settings.

### LSP (Language Server Protocol)

Config-driven — no servers are built-in. Configure via settings:

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
    }
  }
}
```

Or drop a `*.lsp.json` file in `~/.claudio/plugins/`. Servers start lazily on first use and shut down after 5 minutes of inactivity.

**Operations:** `goToDefinition`, `findReferences`, `hover`, `documentSymbol`, `workspaceSymbol`, `goToImplementation`, `prepareCallHierarchy`, `incomingCalls`, `outgoingCalls`.

---

## Agents

### Built-in types

| Type | Model | Description |
|------|-------|-------------|
| `general-purpose` | inherit | Multi-step tasks, code search, research |
| `Explore` | haiku | Fast read-only codebase exploration |
| `Plan` | inherit | Design implementation plans (read-only) |
| `verification` | inherit | Validate implementations, run tests |

### Custom agents

**Flat file** — `~/.claudio/agents/<name>.md`:

```markdown
---
description: Expert Go backend developer
tools: "*"
model: opus
---

You are an expert Go backend developer...
```

**Directory form** — `~/.claudio/agents/<name>/` (preferred when you need agent-specific plugins or skills):

```
agents/
  my-agent/
    AGENT.md          ← same front-matter + body as flat form
    plugins/          ← executables loaded only for this agent
    skills/           ← skills loaded only for this agent
      my-skill/
        SKILL.md
```

#### AGENT.md front-matter reference

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Display name |
| `description` | string | When-to-use guidance shown in agent selector |
| `model` | string | `haiku`, `sonnet`, or `opus` |
| `tools` | string or list | `"*"` for all tools, or an explicit allow-list |
| `disallowedTools` | list | Tool names to block even when `tools: "*"` |
| `capabilities` | list | Opt-in feature sets (e.g. `design`) |
| `autoLoadSkills` | list | Skills pre-loaded at spawn — no model invocation needed |
| `maxTurns` | int | Max agentic turns before stopping |

### Agent crystallization

Crystallize a session's knowledge into a reusable agent persona with its own memory directory. Crystallized agents carry accumulated memory into every team run — no cold-start rebuilding.

---

## Orchestrator & Multi-Agent Teams

Real parallelism — not just sequential sub-agents. Workers run simultaneously in isolated git worktrees.

### How it works

```
┌───────────────┐  TeamCreate      ┌─────────┐
│  Principal    │─────────────────▶│ Manager │
│  Agent (lead) │                  └─────────┘
│               │  SpawnTeammate    ┌──────────────────────────┐
│               │─────────────────▶│ TeammateRunner            │
│               │                  │  [backend-mid]   worker   │──┐ each runs its own
│               │                  │  [backend-jr]    worker   │  │ LLM loop + worktree
│               │                  │  [backend-senior] worker  │  │ in parallel
│               │                  └──────────────────────────┘  │
│               │    on completion:                               │
│               │◀──────────── Mailbox (file JSON + flock) ◀──────┘
└───────────────┘
```

1. **Team creation** — creates a team config and inbox directory under `~/.claudio/teams/{name}/`
2. **Spawning** — each teammate launches as a goroutine running a full `query.Engine` with its own git worktree branch
3. **Worktree isolation** — parallel agents never conflict on the filesystem; lead merges when agents complete
4. **Messaging** — file-based JSON inboxes with `flock`. Direct messages, broadcasts, structured control messages.
5. **Completion** — teammate sends completion message to lead's mailbox; lead picks it up on next turn
6. **Task tracking** — `TaskCreate` tasks auto-complete when the agent finishes; persisted to SQLite

### Team tools

| Tool | Description |
|------|-------------|
| `TeamCreate` | Create a new team (caller becomes lead) |
| `SpawnTeammate` | Spawn a named teammate from a crystallized agent persona |
| `SendMessage` | Direct or broadcast messages between agents |
| `InstantiateTeam` | Re-create a team from a saved template |
| `TaskCreate` / `TaskUpdate` | Create and track tasks |
| `PurgeTeammates` | Remove done/idle teammates to keep context clean |

### The Perfect Workflow

```
/agent               ← Step 1: pick your principal agent (the one you'll talk to)
/team                ← Step 2: pick a team template (the workers it can spawn)

"Build the OAuth module with JWT tokens"
```

The principal agent:
1. Clarifies scope (one question if ambiguous)
2. Explores codebase with a `code-investigator` worker
3. Presents plan and waits for confirmation
4. Creates tasks, spawns workers in parallel
5. Monitors via mailbox, merges worktrees, runs build
6. Reports back to you

**Key rules:**
- `/agent` selects your **principal** (lead / orchestrator). Any agent type can be the principal — pick one whose expertise fits the session.
- `/team` selects the **worker roster** the principal can draw from. The principal decides which workers to spawn and when.
- You interact only with the principal. Workers report back to it, not to you.

### Team templates

A team template is a saved roster of agent types. Save it once, reuse it across any session forever. The names you give workers inside a team are just labels — what matters is which agent *type* each worker uses.

```bash
InstantiateTeam("my-backend-team")
# → creates team + pre-registers all members with their subagent_type
```

**Built-in templates:**

| Template | Workers |
|----------|---------|
| `backend-team` | backend-senior (opus), backend-mid (sonnet), backend-jr (haiku), devops, qa, code-investigator |
| `frontend-team` | frontend-senior (opus), frontend-mid (sonnet), frontend-jr (haiku), qa, code-investigator |
| `fullstack-team` | backend senior + mid + jr, frontend senior + mid + jr, devops, qa, investigator |
| `go-fullstack-team` | backend senior + mid + jr, go-htmx-frontend senior + mid + jr, devops, qa, investigator |
| `efficient-team` | all workers on Haiku with Opus advisors at PLAN/REVIEW only (maximum throughput at minimum cost) |

Pick interactively with `/team` — opens a fuzzy picker showing all saved templates. You can also define your own templates in `~/.claudio/team-templates/`.

### Sync vs async spawning

| Mode | Behaviour | Use when |
|------|-----------|----------|
| `run_in_background: false` (default) | Lead blocks until agent completes | You need the result before the next step |
| `run_in_background: true` | Lead continues; completion arrives via mailbox | Parallel fire-and-forget tasks |

Background agents auto-open the **Agents panel** so you can watch live progress.

---

## 🧠 Two-Brain Advisor

Splits cognitive work into two roles:

| Role | Model | Job |
|------|-------|-----|
| **Executor** | Cheap (e.g. Haiku) | Reads files, runs tools, writes code |
| **Advisor** | Expensive (e.g. Opus) | Strategic thinking at PLAN and REVIEW only — never touches files |

The advisor is consulted **at most twice per task**: once before writing code (PLAN), once when done (REVIEW).

### Consultation protocol

```
advisor(
  mode: "plan",
  orientation_summary: "Codebase uses repository pattern…",
  proposed_approach:   "Add JWT middleware: 1) parse token 2) inject claims 3) gate routes",
  decision_needed:     "Middleware at router level or per-handler?"
)

advisor(
  mode: "review",
  original_plan:     "Add JWT middleware at router level…",
  execution_summary: "Added JWTMiddleware, wired in router.go, added 3 tests",
  confidence:        "high — tests pass, all routes protected"
)
```

REVIEW returns exactly one verdict: `PASS`, `NEEDS_FIX <what>`, or `INCOMPLETE <what>`.

### Enable for any agent

```json
{
  "advisor": {
    "subagentType": "advisor-sr",
    "model": "claude-opus-4-6",
    "maxUses": 6
  }
}
```

### Per-member advisor in team templates

```json
{
  "name": "efficient-team",
  "members": [
    {
      "name": "worker-1",
      "subagent_type": "backend-mid",
      "model": "claude-haiku-4-5-20251001",
      "advisor": {
        "subagent_type": "advisor-sr",
        "model": "claude-opus-4-6",
        "max_uses": 4
      }
    }
  ]
}
```

**Cost profile of `efficient-team`:** every task costs at most 2 Opus calls (plan + review). All other turns run on Haiku. Dramatically cheaper than senior-on-Opus, better than junior-without-guidance.

---

## Harness — Agent Team Architecture

A **harness** is a reusable multi-agent architecture for a specific domain. Build it once with `/harness`, invoke it forever with a single slash command.

```
.claudio/
  agents/
    analyst.md          ← specialist role definitions
    implementer.md
    reviewer.md
  skills/
    feature-harness/
      skill.md          ← orchestrator skill
CLAUDIO.md              ← harness invocation docs
```

### The 6 patterns

#### 1. Pipeline
Sequential stages — each stage's output feeds the next.
```
[Analyze] → [Design] → [Implement] → [Verify]
```
**Use when** each stage depends strongly on the prior one.

#### 2. Fan-out / Fan-in
Parallel specialists work the same input; an integrator merges results.
```
              ┌→ [Specialist A] ─┐
[Dispatcher] →├→ [Specialist B] ─┼→ [Integrator]
              └→ [Specialist C] ─┘
```
**Use when** the task benefits from multiple independent perspectives.

#### 3. Expert Pool
A router calls only the expert relevant to each task.
```
[Router] → { Security Expert | Performance Expert | Architecture Expert }
```
**Use when** input type varies and each type needs different handling.

#### 4. Producer-Reviewer
A producer creates output; a reviewer validates it and triggers rework if needed.
```
[Producer] → [Reviewer] → (issues found) → [Producer] retry
                        → (approved)     → done
```
**Use when** output quality must be verifiable. Always cap retries at 2–3 rounds.

#### 5. Supervisor
A coordinator tracks progress and dynamically assigns work.
```
              ┌→ [Worker A]
[Supervisor] ─┼→ [Worker B]   ← dynamically reassigns
              └→ [Worker C]
```
**Use when** the total workload is unknown upfront or optimal assignment requires runtime info.

#### 6. Hierarchical Delegation
Lead agents decompose the problem and delegate sub-problems to their own specialists.
```
[Director] → [Lead A] → [Worker A1]
                      → [Worker A2]
           → [Lead B] → [Worker B1]
```
**Use when** the problem decomposes naturally into distinct sub-domains.

#### Composite patterns

| Composite | Example |
|-----------|---------|
| Fan-out + Producer-Reviewer | Multi-language translation — 4 parallel translators, each with native-speaker reviewer |
| Pipeline + Fan-out | Analysis (sequential) → parallel implementation by subsystem → integration test |
| Supervisor + Expert Pool | Support queue — supervisor routes tickets to domain experts dynamically |

### Building a harness with `/harness`

```
/harness full-stack feature implementation
/harness security audit pipeline
/harness research and report generation
```

The `/harness` skill runs 11 phases automatically:

0. **Audit** — inventories existing harnesses and crystallized agents; decides create vs extend vs repair
1. **Clarify** — asks what the harness covers, what it outputs, who uses it
2. **Explore** — scans project for languages, frameworks, conventions
3. **Select pattern** — picks the best-fit architecture with an ASCII diagram; asks approval
4. **Design roster** — checks existing crystallized agents first (reuse brings memory); creates only what's missing
5. **Write agent files** — `.claudio/agents/<name>.md` with trigger-rich descriptions and QA protocols
6. **Write orchestrator** — `.claudio/skills/<harness-name>/skill.md` with QA cross-validation built in
7. **Register in CLAUDIO.md** — adds invocation docs
8. **Validate** — checks for placeholder text, verifies agent name consistency, does a dry-run walkthrough
9. **Set up evolution** — adds changelog table for incremental extension
10. **Report** — summary, roster, 3 example invocations, next steps

### Using a generated harness

```
/feature-harness add user notification preferences
```

The orchestrator creates `_workspace/feature-harness/`, builds the task backlog, spawns the team, and synthesizes final output.

---

## Security

| Feature | Description |
|---------|-------------|
| **Permission modes** | `default` (ask), `auto` (allow all), `plan` (read-only) |
| **Permission rules** | Content-pattern matching — `allow: Bash(git *)`, `deny: Write(*.env)` |
| **Cost thresholds** | Configurable cost confirmation dialog |
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
| `PreCompact` / `PostCompact` | Before/after compaction |
| `SessionStart` / `SessionEnd` | Session lifecycle |
| `Stop` | After AI finishes responding |
| `UserPromptSubmit` | Before processing user input |
| `SubagentStart` / `SubagentStop` | Sub-agent lifecycle |
| `TaskCreated` / `TaskCompleted` | Task lifecycle |
| `WorktreeCreate` / `WorktreeRemove` | Git worktree lifecycle |
| `ConfigChange` | Setting changed |
| `CwdChanged` | Working directory changed |
| `FileChanged` | Watched file modified |
| `Notification` | System notification |

Environment variables available in hooks: `CLAUDIO_EVENT`, `CLAUDIO_TOOL_NAME`, `CLAUDIO_SESSION_ID`, `CLAUDIO_MODEL`, `CLAUDIO_TASK_ID`, `CLAUDIO_WORKTREE_PATH`, `CLAUDIO_CONFIG_KEY`, `CLAUDIO_FILE_PATH`.

Exit code 1 from a `PreToolUse` hook blocks the action.

---

## Scheduled Tasks (Cron)

Recurring agent jobs. Tasks stored in `~/.claudio/cron.json`, polled every 60 seconds.

```
CronCreate:
  schedule: "@every 1h"
  prompt:   "Check for failing tests and open a GitHub issue if any"
  type:     "background"
```

Supported schedules: `@every <duration>`, `@hourly`, `@daily`, `HH:MM`.

| Mode | How it runs |
|------|-------------|
| **inline** | Injects prompt as user message into the target session |
| **background** | Spawns isolated engine; result stored in session history |

Manage with `CronCreate`, `CronList`, `CronDelete`. When running with `comandcenter`, crons are shared across all attached sessions.

---

## Session Sharing

```bash
# Export current session
/share my-session.json

# Import on another machine
/teleport my-session.json
```

The shared file contains messages, model, summary, and metadata.

---

## Plugins

### Lua plugins (`~/.claudio/plugins/*/init.lua`)

```bash
claudio plugin install https://github.com/someone/claudio-jira
claudio plugin list
claudio plugin remove claudio-jira
```

Full `claudio.*` API available in every plugin's `init.lua`.

### Binary plugins (`~/.claudio/plugins/`)

Executables in `~/.claudio/plugins/` are auto-discovered and exposed as tools:

```bash
echo '#!/bin/bash
echo "Hello from plugin!"' > ~/.claudio/plugins/greet.sh
chmod +x ~/.claudio/plugins/greet.sh
```

Binary plugins receive env vars: `CLAUDIO_SESSION_ID`, `CLAUDIO_MODEL`, `CLAUDIO_CWD`. Use `--describe` to provide a description.

### claudio-codex — Pre-built Code Index Plugin

[**claudio-codex**](https://github.com/Abraxas-365/claudio-codex) (Go, tree-sitter) builds a structural index of your codebase and exposes it as a deferred tool. Symbol lookups cost ~50 tokens instead of thousands.

```bash
curl -fsSL https://raw.githubusercontent.com/Abraxas-365/claudio-codex/main/install.sh | sh
cd your-project && claudio-codex index
```

| Command | Description |
|---------|-------------|
| `search <query>` | Find symbols by name |
| `refs <symbol>` | All call sites referencing a symbol |
| `context <symbol>` | Definition + source + callers + callees |
| `impact <symbol> [depth]` | Transitive callers (blast radius) |
| `outline <file>` | All symbols in a file |
| `structure` | High-level codebase overview |
| `hotspots [limit]` | Most-referenced symbols |


---

## Model Configuration

### Multi-provider support

```lua
-- ~/.claudio/init.lua
claudio.register_provider({
  name = "groq", type = "openai",
  base_url = "https://api.groq.com/openai/v1",
  api_key = "$GROQ_API_KEY",
  routes = { "llama-*", "mixtral-*" },
})

claudio.register_provider({
  name = "openai", type = "openai",
  base_url = "https://api.openai.com/v1",
  api_key = "$OPENAI_API_KEY",
  routes = { "gpt-*", "o1*" },
})

claudio.register_provider({
  name = "ollama", type = "ollama",
  base_url = "http://localhost:11434",
  routes = { "qwen*", "llama3*" },
})
```

Models without a matching route use the default Anthropic backend. Thinking, effort, and prompt caching are Anthropic-only.

### Extended Thinking

| Mode | Setting | Description |
|------|---------|-------------|
| Auto | `""` | Adaptive thinking for supported models |
| Adaptive | `"adaptive"` | Model decides when and how much |
| Enabled | `"enabled"` | Always think with `budgetTokens` budget |
| Disabled | `"disabled"` | No extended thinking |

### Effort Level

`low` / `medium` (default) / `high` — controls reasoning depth independently from thinking.

---

## Output Styles

| Style | Description |
|-------|-------------|
| `normal` | Default behavior |
| `concise` | Brief, direct. Skip preamble and summaries. |
| `verbose` | Detailed explanations with reasoning and examples. |
| `markdown` | Well-structured Markdown with headers, code blocks, tables. |

Switch with `/output-style [style]` or `:set outputStyle concise`.

---

## Snippet Expansion (Experimental)

Write shorthand that expands to full boilerplate — zero extra AI tokens:

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

`~errw(db.QueryRow(ctx, id), "query user")` inside `func GetUser(id int) (*User, error)` expands to:

```go
result, err := db.QueryRow(ctx, id)
if err != nil {
    return nil, fmt.Errorf("query user: %w", err)
}
```

`{{.ReturnZeros}}` is resolved from the enclosing function's return types via `go/ast`. For Python, TypeScript, JavaScript, and Rust, regex-based resolution is used.

Context variables resolved automatically:

| Variable | Description | Languages |
|----------|-------------|-----------|
| `{{.ReturnZeros}}` | Zero values for the enclosing function's return types | Go |
| `{{.FuncName}}` | Enclosing function name | Go, Python, TS/JS, Rust |
| `{{.ReturnType}}` | Return type annotation | Python, TS/JS, Rust |

---

## Per-Turn Diff Tracking

After each AI turn, Claudio snapshots any changed files and stores the per-turn diff in the session database. View with `/diff turn N`. This lets you see exactly what changed in each turn of a long session — or undo a specific turn without reverting everything.

---

## Command Center (Web / Mobile UI)

`comandcenter` is a WhatsApp-style browser PWA for managing Claudio sessions remotely.

```bash
# Start the server
comandcenter --password mysecret --port 8080

# Connect a Claudio session
claudio --attach http://localhost:8080 --name "my-session" --master
```

**Features:**
- Multi-session management with a sidebar session list
- Real-time WebSocket streaming of agent responses
- File uploads and image attachments
- Push notifications (iOS, Android, desktop via Web Push)
- Sub-agent status tracking with live progress cards
- Message deletion
- Cron task integration

**Install PWA** — open in Safari/Chrome, use "Add to Home Screen" for standalone mode with push notifications.

---

## Headless Mode

One-shot execution without a TUI — useful for scripts, CI, and automation:

```bash
# Run a single prompt and exit
claudio --headless "fix the failing test in main_test.go"

# With model override
claudio --headless --model claude-haiku-4-5-20251001 "summarize the git log"

# Pipe mode
echo "what does this function do?" | claudio
```

Output is streamed to stdout. Exit code 0 on success, non-zero on error.

---

## Filesystem Layout

```
~/.claudio/
  init.lua                          ← your personal Lua config
  settings.json                     ← global settings (JSON)
  state.json                        ← machine-written state
  keybindings.json                  ← custom key bindings
  cron.json                         ← scheduled tasks
  instincts.json                    ← learned instincts
  memory/                           ← global-scope memories
  agents/                           ← built-in agent types (principal or worker)
    backend-senior/
    backend-mid/
    frontend-senior/
    …                               ← add your own custom agents here
  team-templates/                   ← reusable team templates
    backend-team.json
    frontend-team.json
    …                               ← define your own team compositions here
  plugins/                          ← Lua plugins + binary plugins
    claudio-jira/init.lua
    claudio-codex
  projects/
    <project-slug>/
      memory/                       ← project-scoped memories
      designs/                      ← design session outputs

.claudio/                           ← per-project (committed to git)
  init.lua                          ← project Lua config overrides
  settings.json                     ← project settings
  CLAUDE.md                         ← project instructions (alt location)
  agents/                           ← project-specific agents
  skills/                           ← project-specific skills
  rules/                            ← project rules
  filters.toml                      ← legacy output filter overrides (prefer init.lua)
  .gitignore

CLAUDIO.md                          ← project instructions for the AI
```

---

## Architecture

```
cmd/
  claudio/          ← CLI entry point (calls cli.Execute())
  comandcenter/     ← Web server entry point

internal/
  cli/              ← Cobra commands; Version injected via ldflags
  app/              ← Dependency injection / wiring
  tools/            ← All tool implementations
  tui/              ← BubbleTea TUI (~18K LOC, 15+ subpackages)
  web/              ← html/template web UI + Tailwind CSS
  storage/          ← SQLite; 22+ versioned migrations in db.go
  services/         ← 12 focused services (memory, compact, lsp, mcp, …)
  agents/           ← Agent orchestration & spawning
  teams/            ← Multi-agent team management
  bus/              ← Event bus — decoupled inter-component messaging
  config/           ← Hierarchical settings; encrypted token storage
  security/         ← Path/command validation, audit logging
  hooks/            ← Hook system (pre/post tool events)
  permissions/      ← Permission enforcement
  lua/              ← Lua runtime (gopher-lua), API bindings
  capabilities/     ← Dynamic capability registry
  snippets/         ← Snippet expansion engine
  query/            ← LLM conversation engine + turn lifecycle
```

**Key constraints:**
- No CGO — pure Go (`modernc.org/sqlite` for SQLite)
- Single binary — no runtime dependencies on external processes
- BubbleTea TUI: `Model → Update → View` — side effects only in `Cmd` returns
- Event bus for cross-component communication — prefer bus over direct calls

---

## License

MIT — see [LICENSE](LICENSE).

---

<div align="center">

Built with Go · Powered by Claude · Inspired by Neovim

[GitHub](https://github.com/Abraxas-365/claudio) · [Issues](https://github.com/Abraxas-365/claudio/issues) · [Discussions](https://github.com/Abraxas-365/claudio/discussions)

</div>
