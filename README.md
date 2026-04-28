<div align="center">

```
  ██████╗██╗      █████╗ ██╗   ██╗██████╗ ██╗ ██████╗
 ██╔════╝██║     ██╔══██╗██║   ██║██╔══██╗██║██╔═══██╗
 ██║     ██║     ███████║██║   ██║██║  ██║██║██║   ██║
 ██║     ██║     ██╔══██║██║   ██║██║  ██║██║██║   ██║
 ╚██████╗███████╗██║  ██║╚██████╔╝██████╔╝██║╚██████╔╝
  ╚═════╝╚══════╝╚═╝  ╚═╝ ╚═════╝ ╚═════╝ ╚═╝ ╚═════╝
```

### The open-source AI coding agent for your terminal

**Multi-agent teams · Lua-configurable runtime · Vim-grade TUI · Single Go binary**

[![Go Version](https://img.shields.io/badge/go-1.26%2B-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](#license)
[![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux-lightgrey)](#requirements)
[![Pure Go](https://img.shields.io/badge/CGO-free-success)](#key-constraints)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](#contributing)

[**Quick Start**](#-quick-start) · [**Install**](#-installation) · [**Features**](#-features) · [**Docs**](#table-of-contents) · [**Why Claudio?**](#-why-claudio)

</div>

---

## ✨ Features

<table>
<tr>
<td width="33%" valign="top">

### 🤖 Multi-Agent Teams
Built-in orchestration with mailbox messaging, parallel execution, and hierarchical delegation patterns via `/harness`.

</td>
<td width="33%" valign="top">

### 🧠 Persistent Memory
Facts-based entries across project/agent/global scopes. Cache-safe index injection, Recall semantic search, background extraction, and `/dream` consolidation.

</td>
<td width="33%" valign="top">

### ⚡ Token Efficient
11-layer optimization: prompt caching, RTK output filtering, snippet expansion, microcompaction, dedup, image compression.

</td>
</tr>
<tr>
<td valign="top">

### ⌨️ Vim-Grade TUI
Full state machine — normal, insert, visual, operator-pending modes with registers. Built on Bubbletea.

</td>
<td valign="top">

### 💎 Session Crystallization
Promote any session into a reusable agent persona — with its own memory, tools, and instructions.

</td>
<td valign="top">

### 🔌 Lua Runtime
Full `~/.claudio/init.lua` config — register tools, keymaps, themes, commands, providers, capabilities, and hooks without recompiling. Same philosophy as Neovim.

</td>
</tr>
<tr>
<td valign="top">

### ⏰ Scheduled Tasks
Cron-style recurring agent jobs: `@every 1h`, `@daily`, or `HH:MM`.

</td>
<td valign="top">

### 🌐 Command Center
`comandcenter` — a WhatsApp-style browser/PWA for remote sessions, file uploads, push notifications, and multi-session management.

</td>
<td valign="top">

### 📦 Single Binary
Pure Go, zero runtime, no Node.js. `modernc.org/sqlite` keeps it CGO-free.

</td>
</tr>
<tr>
<td valign="top">

### 🧠 Two-Brain Advisor
Cheap executor (Haiku) does the work; expensive advisor (Opus) consults at PLAN and REVIEW — at most twice per task. Senior judgment at a fraction of the cost.

</td>
</tr>
</table>

---

## 🚀 Quick Start

```bash
# 1. Install
go install github.com/Abraxas-365/claudio/cmd/claudio@latest

# 2. Authenticate with Anthropic (Claude)
claudio auth login

# 3. Bootstrap your project
cd your-project
claudio          # launches the TUI
/init            # AI-guided project setup

# 4. Start coding
```

> 💡 **Tip:** `claudio --resume` picks up your last session. `claudio "fix the failing test"` runs a one-shot prompt without the TUI.

#### Using a different provider

`claudio auth login` is the quickest path — it authenticates with Anthropic so you can use Claude models out of the box. But Claudio is **model-agnostic**: you can register any provider in `~/.claudio/init.lua` and route models to Groq, OpenAI, Ollama, Together, vLLM, or any OpenAI-compatible endpoint:

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

Then launch with any routed model:

```bash
claudio --model gpt-4o                   # OpenAI
claudio --model llama-3.3-70b-versatile  # Groq
claudio --model qwen2.5-coder            # Local Ollama
```

Or switch live in the TUI: `:set model llama-3.3-70b-versatile`

See [Lua Configuration](#lua-configuration) and [Model Configuration](#model-configuration) for the full reference.

---

## Table of Contents

- [Why Claudio?](#why-claudio)
- [Requirements](#requirements)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Lua Configuration](#lua-configuration)
  - [init.lua — personal config](#initlua--personal-config)
  - [Plugins](#lua-plugins)
  - [Full API surface](#full-api-surface)
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
  - [The Perfect Workflow](#the-perfect-workflow)
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
- [Command Center (Web / Mobile UI)](#command-center-web--mobile-ui)
- [Headless Mode](#headless-mode)
- [Filesystem Layout](#filesystem-layout)
- [Architecture](#architecture)
- [License](#license)

---

## 🆚 Why Claudio?

Claudio is built ground-up in Go for engineers who want **more control, more agents, and fewer dependencies**.

|  | **Claudio** | Claude Code |
|---|---|---|
| 🏗️ **Runtime** | Single Go binary, no runtime | Node.js / TypeScript |
| 🤝 **Multi-agent teams** | Built-in orchestration, mailbox messaging | ❌ |
| 💎 **Session-as-agent** | Crystallize sessions into reusable personas | ❌ |
| 🧠 **Memory** | Scoped (project/agent/global), facts-based, Recall semantic search, `/dream` consolidation, cache-safe injection | Single directory |
| 🗜️ **Token efficiency** | 11-layer optimization stack | Basic prompt caching |
| ✂️ **Snippet expansion** | `~name(args)` shorthand → full code templates | ❌ |
| 🎯 **Learned instincts** | Confidence-scored patterns replayed across sessions | ❌ |
| ⏰ **Cron tasks** | `@every 1h`, `@daily`, `HH:MM` | Feature-gated |
| 🌐 **Web / Mobile UI** | `comandcenter` — WhatsApp-style PWA, push notifications, file uploads, multi-session hub | ❌ |
| 🌉 **Cross-session comms** | Unix-socket bridge for parallel worktrees | ❌ |
| ⌨️ **Vim mode** | Full state machine + registers + `:` command line | Basic vi-mode |
| 💾 **Persistence** | SQLite + file-based | File-based only |
| 🔌 **Extensibility** | Lua runtime — register tools, providers, keymaps, themes, commands from `init.lua`. No recompile. | Extension API in beta |

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

### Option 2 — From source (recommended for contributors)

```bash
git clone https://github.com/Abraxas-365/claudio
cd claudio
make build              # injects version via ldflags
sudo mv claudio /usr/local/bin/
# also build the command center server
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
comandcenter --password mysecret --port 8080  # start the server
claudio --attach http://localhost:8080 \
        --name "my-session" --master          # connect a Claudio session to it
```

---

## Lua Configuration

Claudio embeds a Lua runtime (gopher-lua — pure Go, no CGO) that gives you full control over every aspect of your setup without recompiling. The philosophy is the same as Neovim: the binary ships compiled defaults, and your `~/.claudio/init.lua` overrides everything on top.

### `init.lua` — personal config

```lua
-- ~/.claudio/init.lua  (your personal Claudio config)

-- ── Model & settings ─────────────────────────────────────
claudio.config.set("model", "claude-opus-4-6")
claudio.config.set("caveman", true)
claudio.config.set("compactMode", "strategic")

-- ── Theme ─────────────────────────────────────────────────
claudio.colorscheme("tokyonight")
-- or fine-grained:
claudio.ui.set_color("primary", "#7aa2f7")
claudio.ui.set_border("rounded")

-- ── Keymaps ───────────────────────────────────────────────
claudio.register_keymap({ mode = "normal", key = "K", action = "show_docs",
  handler = function() claudio.notify("docs") end })

-- ── Providers ─────────────────────────────────────────────
claudio.register_provider({
  name     = "groq",
  type     = "openai",
  base_url = "https://api.groq.com/openai/v1",
  api_key  = "$GROQ_API_KEY",
  routes   = { "llama-*" },
})

-- ── Hooks ─────────────────────────────────────────────────
claudio.subscribe("tool.executed", function(e)
  if e.tool_name == "Bash" then
    claudio.log("[audit] " .. tostring(e.input))
  end
end)

-- ── Commands ──────────────────────────────────────────────
claudio.register_command({
  name        = "standup",
  description = "Print git log for standup",
  execute     = function(args)
    return "run: git log --oneline --since=yesterday"
  end,
})

-- ── Capabilities ──────────────────────────────────────────
claudio.register_capability("database", {
  tools = { "SQLQuery", "SchemaInspect", "MigrationRun" }
})
```

### Load order

```
1. internal/lua/defaults.lua   ← embedded binary defaults (model, compactMode, etc.)
2. ~/.claudio/init.lua         ← your personal config
3. ~/.claudio/plugins/*/       ← community plugins
4. .claudio/init.lua           ← project overrides (per-repo)
```

Each layer can override the one before. Project config wins over personal config, personal wins over defaults.

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
| `claudio.ui.*` | `set_color(slot, hex)`, `set_theme(table)`, `set_border(name)`, `get_colors()`, `set_statusline(fn)`, `popup(opts)`, `register_whichkey(group, bindings)`, `register_palette_entry(entry)`, `register_sidebar_block(opts)` |
| `claudio.agent.*` | `current()`, `on_change(fn)`, `add_context(text)`, `set_prompt_suffix(name, text)` |
| `claudio.session.*` | `id()`, `title()`, `on_start(fn)`, `on_end(fn)`, `on_message(fn)` |

**Color slots for `set_color`:** `primary`, `secondary`, `success`, `warning`, `error`, `muted`, `surface`, `surface_alt`, `text`, `dim`, `subtle`, `orange`, `aqua`

**Built-in colorschemes:** `tokyonight`, `gruvbox`, `catppuccin`, `nord`, `dracula`

### `:checkhealth`

Press `:checkhealth` (or `:health`) to see a diagnostics report:

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

Settings are resolved with priority (highest first):

```
Environment variables         CLAUDIO_MODEL, CLAUDIO_API_BASE_URL, etc.
       |
.claudio/init.lua             Project Lua config (per-repo, committed to git)
       |
~/.claudio/plugins/*/init.lua Community plugins (loaded after user init)
       |
~/.claudio/init.lua           Your personal Lua config — keymaps, theme, providers
       |
internal/lua/defaults.lua     Embedded defaults (compiled into binary)
       |
~/.claudio/state.json         Machine-written state only (plugin data, etc.)
```

**Human config lives in `init.lua`**, not JSON. Everything intentional — model, keymaps, theme, providers, hooks — goes in `~/.claudio/init.lua`. `state.json` is machine-written and you never touch it.

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
| `compactKeepN` | integer (default `10`) | Number of recent messages to keep after compaction |
| `maxBudget` | USD amount, 0 = unlimited | Session spend limit |
| `outputFilter` | `true`/`false` | RTK-style command output filtering (see below) |
| `cavemanMode` | `""`, `lite`, `full`, `ultra` | Compressed output mode (see [CavemanMode](#cavemanmode)) |
| `eagerPlugins` | `[]string` | Plugin names to load as eager (non-deferred) tools. Names without `plugin_` prefix. E.g. `["claudio-codex"]` |
| `toolModels` | `map[string]string` | Per-tool model override. Key = tool name, value = model ID. E.g. `{"ReviewDesignFidelity": "claude-haiku-4-5"}` |
| `publicUrl` | `string` | Public base URL for bundle share links (e.g. `https://myserver.ts.net`) |

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
| `--headless` | | Run without TUI (one-shot or attach mode) |
| `--context` | | Load context profile (`dev`, `review`, `research`, or a file path) |
| `--budget` | | Session spend limit in USD (0 = unlimited) |
| `--resume` | `-r` | Resume a previous session (no value = most recent, or pass session ID) |
| `--print` | | Print-only mode (no TUI, clean stdout for piping) |
| `--dangerously-skip-permissions` | `--yolo` | Skip all permission prompts |
| `--attach` | | ComandCenter server URL to attach to (e.g. `http://localhost:8080`). Forces `--headless`. |
| `--name` | | Session display name in ComandCenter |
| `--master` | | Mark this session as the master — enables `SendToSession` and `SpawnSession` tools |

---

## Interactive Commands

### Vim Command Line (`:` mode)

Press `:` in normal vim mode to open the command line (exactly like Neovim):

| Command | Description |
|---------|-------------|
| `:lua <code>` | Execute Lua in the live REPL — `claudio.notify("hi")`, `claudio.ui.set_color(...)` |
| `:set <key> [value]` | Read or write any config setting — `:set model`, `:set caveman true` |
| `:colorscheme <name>` | Switch theme — `tokyonight`, `gruvbox`, `catppuccin`, `nord`, `dracula` |
| `:checkhealth` | Diagnose plugins, capabilities, config, LSP — like `:checkhealth` in Neovim |
| `:health` | Alias for `:checkhealth` |
| `:<command>` | Any `/command` works as a `:command` too |

### Slash Commands

| Command | Aliases | Description |
|---------|---------|-------------|
| `/help` | `h`, `?` | Show available commands |
| `/model` | `m` | Show or change the AI model |
| `/compact [instruction]` | | Compact conversation history. Optional instruction guides what to focus on in the summary (e.g. `/compact focus on architecture decisions`). Default messages kept is set by `compactKeepN` in settings. |
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
| `/gain` | | Show token savings from output filters — bytes stripped per command, savings %, top commands |
| `/discover` | | Show commands that ran without a filter — opportunities to reduce token usage |
| `/output-style [style]` | | Show or set output style (normal, concise, verbose, markdown) |
| `/caveman [lite\|full\|ultra\|off]` | | Toggle caveman mode for compressed output (see [CavemanMode](#cavemanmode)) |
| `/keybindings` | | Open keybindings.json in your editor |
| `/vim` | | Toggle vim keybindings |
| `/skills` | | List available skills |
| `/tasks` | | Show background tasks and team status |
| `/agent` | | Pick an agent persona to become the lead for this session (e.g. `prab` as PM) |
| `/team` | | Pick a team template — the workers the lead will coordinate |
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

### `:` Command Line

Press `:` in normal vim mode to open the command line (like Neovim). Built-in commands:

| Command | Description |
|---------|-------------|
| `:lua <code>` | Execute Lua in the live runtime — `:lua claudio.notify("hi")` |
| `:set <key> [value]` | Read or write any config setting — `:set model claude-opus-4-6` |
| `:colorscheme <name>` | Switch theme — `tokyonight`, `gruvbox`, `catppuccin`, `nord`, `dracula` |
| `:checkhealth` / `:health` | Diagnose plugins, capabilities, config, LSP |
| `:model` | Show or change the AI model |
| `:vim` | Toggle vim mode |

All `/` slash commands also work as `:` commands. Plugins can register new commands with `claudio.register_command()`.

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

Manual compaction: `/compact [instruction]` — runs a full compact using the model to summarize old messages. Pass an optional text instruction to guide what the summary focuses on (e.g. `/compact keep only the decisions about database schema`). The number of recent messages kept is controlled by `compactKeepN` in `settings.json` (default: 10).

Compacted messages are **persisted to the session database**, so resuming a session after `/compact` loads the compacted history — not the original uncompacted messages.

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

**How it works:** after a command runs, the output passes through three filter layers in order:

1. **TOML filter engine** — a declarative 8-stage pipeline checked first. Matches by command name and applies stages in order:
   - `strip_ansi` — remove ANSI/OSC escape sequences
   - `replace` — chainable regex substitutions applied line-by-line
   - `match_output` — short-circuit: if the full output matches a pattern, emit a single summary message (with optional `unless` guard to skip when errors are present)
   - `strip_lines_matching` / `keep_lines_matching` — blacklist or whitelist lines by regex (mutually exclusive)
   - `truncate_lines_at` — cap individual line length
   - `head_lines` / `tail_lines` — keep only the first or last N lines
   - `max_lines` — hard cap on total output lines after all other stages
   - `on_empty` — fallback message when filtering produces empty output

2. **Built-in command filters** — 38 hardcoded filters for the most common tools:

   | Category | Commands |
   |----------|----------|
   | Go | `go test` (JSON + plain), `go build`, `go vet` |
   | Rust | `cargo build`, `cargo test`, `cargo clippy` |
   | JavaScript | `npm/pnpm/yarn install`, `nx`, `turbo`, `biome`, `oxlint`, `vitest`, `eslint`, `tsc` |
   | Python | `pip install`, `poetry`, `uv`, `pre-commit` |
   | JVM | `gradle`, `maven` |
   | .NET | `dotnet build/restore/test` |
   | Swift | `swift build`, `xcodebuild` |
   | Ruby/Elixir/PHP | `mix`, `composer install` |
   | DevOps | `helm`, `gcloud`, `tofu`, `ansible-playbook` |
   | Containers | `docker build`, `docker pull/push` |
   | Linting | `shellcheck`, `hadolint`, `yamllint`, `markdownlint` |
   | System | `systemctl`, `rsync`, `ssh`, `jq`, `ollama` |
   | Build | `make`, `just`, `task` |
   | Package managers | `mise` |
   | SCM/tools | `jira` |

3. **Generic filters** (applied to all commands, or as fallback):
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

#### Custom TOML filters

You can add your own filters or override built-ins without recompiling. Claudio loads filter files from three locations in priority order:

| Priority | Path | Notes |
|----------|------|-------|
| 1 (highest) | `.claudio/filters.toml` | Project-local overrides |
| 2 | `~/.config/claudio/filters.toml` | User-global overrides |
| 3 (lowest) | Built-in | Embedded at compile time |

Example filter file:

```toml
schema_version = 1

[filters.my-tool]
description    = "Strip noisy progress from my-tool"
match_command  = "^my-tool$"
strip_ansi     = true
max_lines      = 50
on_empty       = "my-tool: ok"

strip_lines_matching = [
  "^Downloading",
  "^Resolving",
  "\\[=+>?\\s*\\]",   # progress bars
]

[[match_output]]
pattern = "Success"
message = "my-tool: completed successfully"
unless  = "error|warn"
```

Set `CLAUDIO_NO_FILTER=1` to bypass all filtering, or `CLAUDIO_FILTER_DEBUG=1` to log which filter matched each command.

#### Plugin tool filtering

Plugin tools (e.g. `claudio-tmux`) pass through the same TOML filter pipeline as Bash when `outputFilter: true`. The filter key is the plugin's `command` field — e.g. `"run"`, `"cat"`, `"lsp"` for claudio-tmux.

This means you can add entries in `.claudio/filters.toml` to filter any plugin's output:

```toml
[filters.tmux-run]
description   = "Trim verbose output from tmux run calls"
match_command = "^run$"
max_lines     = 100
strip_lines_matching = ["^npm warn", "^\\s*$"]

[filters.tmux-cat]
description   = "Cap raw pane captures"
match_command = "^cat$"
max_lines     = 50
```

Any plugin added in the future automatically inherits this support — no code changes needed, just add a filter block matching its command name.

**claudio-tmux also has its own low-level filter layer** (independent of claudio's config) that strips terminal garbage before claudio ever sees the output: OSC escape sequences, shell prompt lines, braille spinners, progress bars. This runs unconditionally on all tmux output. You can add project-local overrides in `.claudio-tmux/filters.toml` using the same TOML schema. Set `CLAUDIO_TMUX_NO_FILTER=1` to bypass it.

The two layers are complementary:
```
tmux raw → claudio-tmux filter (terminal garbage) → claudio outputFilter (semantic noise) → context
```

#### Source-code filter (`codeFilterLevel`)

When Claudio reads source files via the `Read` tool, it can strip comments and boilerplate before the content enters the context window — reducing token usage on large files without losing the information that matters.

Configure it in the TUI config panel (`<Space>ic`) or in `settings.json`:

```json
{
  "codeFilterLevel": "minimal"
}
```

| Level | What it does |
|-------|-------------|
| `none` *(default)* | Raw file content, no filtering applied |
| `minimal` | Strips line and block comments, preserves doc comments (`///`, `/** */`), collapses blank lines |
| `aggressive` | Keeps only function/type signatures and imports; replaces bodies with `// ...` |

**When filtering applies:** only on full-file reads (no `offset`/`limit` specified) of files longer than 500 lines. Files over 2000 lines also get `SmartTruncate` — a structurally-aware truncation that preserves function signatures and imports even when cutting.

**Per-project override:** set `codeFilterLevel` in `.claudio/settings.json` to use a different level for comment-heavy repos (e.g. `"aggressive"` for auto-generated code).

> **Cache caveat:** the read cache key is `(filePath, offset, limit)` — it does not include `codeFilterLevel`. If you change the filter level mid-session, previously cached file reads will return the old filtered content until the file is modified on disk or the session restarts. In practice this rarely matters since the level is set once per project, but be aware when toggling it interactively.

### CavemanMode

Caveman mode reduces output token usage by ~65-75% by injecting terse communication rules into the system prompt. Inspired by [github.com/JuliusBrussee/caveman](https://github.com/JuliusBrussee/caveman), it maintains full technical accuracy while dropping unnecessary filler.

**How to enable:**

- **Interactive command:** `/caveman [lite|full|ultra|off]` (cycles through modes at runtime)
- **Config panel:** `<Space>ic`, then toggle — cycles off → lite → full → ultra → off
- **Settings file:** Set `cavemanMode` in `~/.claudio/settings.json` or `.claudio/settings.json`

```json
{
  "cavemanMode": "full"
}
```

**Three modes:**

| Mode | Style | Example |
|------|-------|---------|
| `lite` | No filler/hedging, keeps articles and full sentences | "The API endpoint requires authentication" |
| `full` | Classic caveman: drops articles, fragments OK, short synonyms, `[thing] [action] [reason]` pattern | "API endpoint needs auth. Use Bearer token for requests." |
| `ultra` | Maximum compression: abbreviations (DB/auth/config/req/res/fn/impl), arrows for causality (X → Y), one word when one word is enough | "API req → Bearer token. Impl in auth handler." |

**Always normal:** Code blocks, git commits, and security warnings are always written with full grammar and clarity, regardless of mode. This ensures critical information is never compressed.

**Disable:** Use `/caveman off` or set `cavemanMode` to `""` (empty string) in settings.

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
| Output filtering (38 commands) | `internal/tools/outputfilter/` | 60-90% reduction on noisy command outputs |
| TOML filter engine | `internal/tools/outputfilter/tomlfilter/` | User-customizable declarative filters, no recompile |
| Plugin output filtering | `internal/plugins/proxy.go` | Same TOML pipeline applied to all plugin tool results |
| Source-code filter | `internal/tools/outputfilter/codefilter/` | Strips comments from large files before context entry |
| claudio-tmux terminal filter | `claudio-tmux/internal/tomlfilter/` | Strips OSC/spinner/prompt noise before output reaches claudio |
| Deferred tool schemas | `internal/tools/registry.go` | Saves full schema cost for unused tools |
| Snippet expansion | `internal/snippets/` | Reduces AI output tokens for repetitive boilerplate |

---

## Memory System

Three-scope, facts-based memory architecture. No system prompt pollution — cache stays intact.

### Scopes

| Scope | Path | Purpose |
|-------|------|---------|
| **Project** | `~/.claudio/projects/<project-slug>/memory/` | This repo's conventions, decisions, architecture |
| **Global** | `~/.claudio/memory/` | Cross-project user preferences and personal style |
| **Agent** | `~/.claudio/agents/<name>/memory/` | Per-crystallized-agent accumulated knowledge |

Resolution priority: **Agent > Project > Global** — name conflicts use the higher-priority scope.

**Scope decision rule:** ask *"would this be true in a completely different project?"*
- Yes → `global` (e.g. "always use table-driven tests")
- No → `project` (e.g. "JWT secret is in .env.local")
- Persona-specific → `agent`

### Entry format

Memories are markdown files with YAML frontmatter. Content is stored as discrete **facts** — one sentence each — not prose:

```markdown
---
name: jwt-config
description: JWT token configuration for this API
type: project
scope: project
tags: [auth, jwt, token]
updated_at: 2026-04-12T10:00:00Z
facts:
  - JWT tokens expire in 24h
  - Refresh threshold is 20h — issue new token if TTL < 4h
  - Secret stored in .env.local under JWT_SECRET
  - Signing algorithm is RS256
concepts:
  - token-lifecycle
  - session-management
  - authentication
---
```

Memory types: `user`, `feedback`, `project`, `reference`.

**Concepts** are optional semantic tags broader than `tags`. They improve FTS search recall by bridging vocabulary gaps — a query for "how do we handle login sessions?" can match a `concepts: [session-management]` entry even with no literal word overlap.

### How the agent uses memory

At session start, a **lean memory index** is injected into the first human turn (not the system prompt — cache is never broken):

```
## Your Memory Index

### Global Memories
- prefer-table-tests [testing,go]: User's Go testing style — "Always use t.Run" | "Never write single-case tests"

### Project Memories
- jwt-config [auth,jwt,token]: JWT configuration — "Expires in 24h" | "RS256 signing"
- no-orm [db,sql]: DB rules — "Never use GORM" | "Raw SQL via modernc.org/sqlite"
```

Entries are sorted by `updated_at DESC` — most recently active work appears first. If the index exceeds 200 lines or 25 KB, the oldest entries are trimmed (never the newest). The agent sees what exists without loading full content (~10 tokens per entry). It then:

- **`Memory(action="read", name="...")`** — load full facts for a specific entry
- **`Recall(context="...")`** — semantic search across all scopes using the configured small model
- **`Memory(action="append", name="...", fact="...")`** — add one fact to an existing entry (no full rewrite)
- **`Memory(action="save", ...)`** — create a new entry with `facts[]`
- **`Memory(action="delete-fact", name="...", index=N)`** — remove a specific fact
- **`Memory(action="delete", name="...")`** — remove an entire entry

### Memory tool actions

| Action | Description |
|--------|-------------|
| `list` | List all entries (name + description + scope) |
| `search` | Keyword search across name, description, tags, facts |
| `read` | Load full facts for a named entry |
| `save` | Create new entry with `facts[]`, `tags[]`, `concepts[]`, `type`, `scope` |
| `append` | Add one fact to existing entry — preferred over full rewrites |
| `replace-fact` | Replace a fact at a specific index |
| `delete-fact` | Remove a fact at a specific index |
| `delete` | Delete entire entry |

### Recall tool

`Recall(context="...")` finds the most relevant memories for your current task using a two-tier search:

1. **SQLite FTS5 pre-filter** — BM25-ranked full-text search across all facts, concepts, descriptions, and tags. Fast, free, no API call. Returns up to 20 candidates.
2. **LLM ranking** — the configured small model picks the top 5 from those candidates. Only runs when there are more than 5 candidates.

Recall searches all scopes automatically (global + project + agent). Unlike `Memory(search)` which does exact keyword matching, Recall understands intent and synonyms via the FTS porter stemmer and the `concepts` field.

Use it before starting significant tasks or when you're unsure what relevant context might exist.

**FTS sync:** The FTS index is rebuilt from `.md` files on every startup — files are always the source of truth. If a memory file is edited manually or the database is deleted, the index self-heals on next launch.

### Background extraction

After 4+ turns, a background agent (Haiku) reviews the conversation and automatically extracts memories into the project scope. Disable with `"autoMemoryExtract": false`.

### Dream consolidation (`/dream`)

`/dream` runs a consolidation agent that reviews the current session conversation (user and assistant messages only — no tool noise) and:

1. Lists all existing memories
2. Detects contradictions — deletes or updates stale facts
3. Appends new facts to existing entries where relevant
4. Creates new memories for genuinely new learnings
5. Promotes project-scope entries to global when they reflect universal user preferences

Run `/dream` at the end of a productive session to keep the memory store accurate and contradiction-free.

### Memory in teams

| Role | Scopes visible | Can write? |
|------|---------------|------------|
| **Orchestrator** | Global + Project + own Agent (if crystallized) | ✅ |
| **Ephemeral teammate** (no `MemoryDir`) | None | ❌ |
| **Crystallized teammate** (has `MemoryDir`) | Own agent scope | ✅ to their agent dir |

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
| **TeamCreate** / **SendMessage** | Multi-agent teams |
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

Agents can be defined in two forms — a **flat file** or a **directory**. The directory form is preferred when you need agent-specific plugins or skills.

**Flat file** — `~/.claudio/agents/<name>.md`:

```markdown
---
description: Expert Go backend developer
tools: "*"
model: opus
---

You are an expert Go backend developer...
```

**Directory form** — `~/.claudio/agents/<name>/`:

```
agents/
  my-agent/
    AGENT.md          ← same front-matter + body as the flat .md form
    plugins/          ← executables loaded only for this agent
      my-plugin       ← binary plugin (e.g. claudio-assistant-os)
    skills/           ← skills loaded only for this agent
      my-skill/
        SKILL.md      ← skill instructions + front-matter
        references/   ← optional reference docs consumed by the skill
          <category>/
            *.md
```

`AGENT.md` uses the same front-matter schema as the flat file form. Plugins placed in `agents/<name>/plugins/` are injected into the agent's tool list automatically — they are **not** available to other agents. Skills placed in `agents/<name>/skills/<skill-name>/` are prepended to the agent's system prompt when a session starts.

#### AGENT.md front-matter reference

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Display name for the agent |
| `description` | string | When-to-use guidance shown in the agent selector |
| `model` | string | `haiku`, `sonnet`, or `opus` |
| `tools` | string or list | `"*"` for all tools, or an explicit allow-list |
| `disallowedTools` | list | Tool names to block even when `tools: "*"` |
| `allowedMCPTools` | list | MCP tool names this agent may call |
| `capabilities` | list | Opt-in feature sets (e.g. `design` enables mockup tools) |
| `autoLoadSkills` | list | Skill names pre-loaded into the system prompt **at spawn time** — no model invocation needed |
| `maxTurns` | int | Max agentic turns before the agent stops (`0` = unlimited) |
| `sourceSession` | string | Session ID to resume from |
| `sourceProject` | string | Project context to attach |

**`autoLoadSkills`** is the key field for giving an agent standing instructions without requiring it to call `Skill(name)` first. The named skills are resolved from the global registry (bundled + `~/.claudio/skills/` + `.claudio/skills/`) merged with the agent's own `skills/` directory:

```yaml
---
name: frontend-senior
description: Senior frontend engineer (opus). Deep expertise in React, performance, accessibility, and design systems.
model: opus
tools: "*"
autoLoadSkills:
  - ui-ux-pro-max
  - commit
---

You are a senior frontend engineer...
```

Both skills are injected into the system prompt automatically — the agent starts every session already knowing the UI/UX guidelines and commit conventions.

#### Global auto-load (`settings.json`)

You can also pre-load skills globally for all agents without editing each `AGENT.md`:

```json
{
  "autoLoadSkills": {
    "onStart": ["caveman"],
    "onAgentSpawn": ["commit"]
  }
}
```

| Key | When injected |
|-----|--------------|
| `onStart` | Into the **main session agent** at startup |
| `onAgentSpawn` | Into **every spawned sub-agent** (teammates, background agents) |

Per-agent `autoLoadSkills` in `AGENT.md` stack on top of `onAgentSpawn` — both are injected.

#### Example: built-in agent roster (`~/.claudio/agents/`)

Claudio ships with a ready-to-use set of agents in `~/.claudio/agents/`. There are **two roles** in the multi-agent workflow:

> **Product manager / team lead** — loaded with `/agent` to become the orchestrator for the session.
> **Workers** — loaded by the PM via `SpawnTeammate` when a team template is instantiated.

**`prab/` — the product manager** (loaded with `/agent prab`):

```
~/.claudio/agents/prab/
  AGENT.md              ← persona definition (front-matter + system prompt)
  plugins/
    claudio-assistant-os  ← macOS productivity plugin (Reminders, Mail, Calendar, Notes)
    claudio-tmux          ← tmux session control plugin
  skills/
    assistant-os/
      SKILL.md          ← instructions for using the assistant-os plugin
```

```markdown
---
name: Prab
description: Tech lead and project manager. Use Prab to discuss ideas, explore trade-offs, create plans, break down work into tasks, and assign those tasks to the right agents.
model: sonnet
tools: *
---

You are Prab, a seasoned tech lead and project manager. You have deep engineering experience but your
primary role is to think at the system level — bridging ideas and execution by planning clearly and
delegating to the right agents.
```

**Worker agents** (spawned by the PM — never invoked directly):

All worker agents use the **directory form** (`~/.claudio/agents/<name>/AGENT.md`). Agents that need framework-specific skills ship those skills inside their own directory.

| Directory | `subagent_type` | Model | Skills | Role |
|-----------|-----------------|-------|--------|------|
| `backend-jr/` | `backend-jr` | haiku | — | Simple CRUD, boilerplate, straightforward tests |
| `backend-mid/` | `backend-mid` | sonnet | — | Standard features, refactors, well-scoped work |
| `backend-senior/` | `backend-senior` | opus | — | Architecture, complex problem-solving, high-stakes changes |
| `frontend-jr/` | `frontend-jr` | haiku | `tux-tui` | Simple components, style fixes, copy changes |
| `frontend-mid/` | `frontend-mid` | sonnet | `tux-tui` | Component refactors, state wiring, UI tests |
| `frontend-senior/` | `frontend-senior` | opus | `tux-tui` | Design systems, rendering strategy, bundle optimization |
| `go-htmx-frontend-jr/` | `go-htmx-frontend-jr` | haiku | `go-htmx` | Go + htmx partials, style fixes, simple htmx wiring |
| `go-htmx-frontend-mid/` | `go-htmx-frontend-mid` | sonnet | `go-htmx` | Server-rendered UIs, htmx interactions, OOB swaps, SSE |
| `go-htmx-frontend-senior/` | `go-htmx-frontend-senior` | opus | `go-htmx` | Hypermedia architecture, template systems, rendering strategy |
| `htmx-ux-jr/` | `htmx-ux-jr` | haiku | — | Simple UI/UX: color palettes, single components, quick notes |
| `htmx-ux-mid/` | `htmx-ux-mid` | sonnet | — | Component sets, page layouts, interaction flows |
| `htmx-ux-senior/` | `htmx-ux-senior` | opus | — | Design systems, cross-screen UX strategy, accessibility |
| `code-investigator/` | `code-investigator` | haiku | — | Symbol tracing, call-graph analysis, codebase mapping |
| `devops/` | `devops` | sonnet | — | CI/CD pipelines, Dockerfiles, Kubernetes, cloud infra |
| `qa/` | `qa` | sonnet | — | E2E tests, API contract validation, OWASP security testing |
| `research-jr/` | `research-jr` | haiku | — | Well-scoped research: competitor lookups, feature lists |
| `research-mid/` | `research-mid` | sonnet | — | User research, competitive analysis, opportunity mapping |
| `research-senior/` | `research-senior` | opus | — | Product discovery, go-to-market strategy, research synthesis |
| `advisor-sr/` | `advisor-sr` | opus | — | Strategic advisor: validates plans and audits outcomes |
| `pentest/` | `pentest` | haiku | — | Authorized web app penetration testing |

Each agent directory contains at minimum an `AGENT.md`:

```markdown
---
name: backend-jr
description: Fast backend engineer (haiku). Best for simple, well-scoped tasks — CRUD, small fixes,
  boilerplate, and straightforward tests. Avoid tasks needing multi-step architectural reasoning.
model: haiku
tools: "*"
---

You are a capable backend engineer. You execute well-scoped, clearly defined tasks efficiently and
write clean, correct code.

## Rules (follow all of them — do not skip any)

1. **Read first** — read every relevant file before touching anything
2. **State your plan** — one short paragraph: what you will change and where
3. **Implement** — follow the existing style and conventions exactly; no extras
4. **Run tests and linter** — fix all failures before reporting
5. **Report** — summarize what changed, paste key test output, flag anything unexpected
```

Agents with bundled skills (e.g. `frontend-jr`) look like:

```
~/.claudio/agents/frontend-jr/
  AGENT.md
  skills/
    tux-tui/
      SKILL.md
```

The `tux-tui` skill is injected into the agent's system prompt automatically at session start — it is not available to other agents.

### Agent crystallization

Crystallize a session's knowledge into a reusable agent persona with its own memory directory. The agent is then invocable from any project.

---

## Orchestrator & Multi-Agent Teams

Claudio supports spawning parallel worker agents ("teammates") coordinated by a team lead through a **file-based mailbox pattern**. The calling agent becomes the team lead and can spawn, message, and monitor teammates — each of which runs a full LLM conversation loop in its own goroutine.

### How it works

```
┌─────────────┐  TeamCreate      ┌─────────┐  creates config + inboxes
│  Team Lead  │─────────────────▶│ Manager │
│  (you/LLM)  │                  └─────────┘
│             │  SpawnTeammate    ┌──────────────┐
│             │─────────────────▶│TeammateRunner│  Spawn() → goroutines
│             │                  └──────┬───────┘
│             │           ┌────────────▼────────────┐
│             │           │ Teammate 1 (backend-mid) │──┐
│             │           │ Teammate 2 (backend-mid) │  │ each runs its own
│             │           │ Teammate 3 (backend-sr)  │  │ LLM conversation
│             │           └─────────────────────────┘  │  + worktree
│             │                       │                │
│             │    on completion:     │                │
│             │    mailbox → lead     ▼                │
│             │◀──────────────── Mailbox ◀─────────────┘
│             │               (file JSON + flock)
└─────────────┘
```

1. **Team creation** — creates a team config and inbox directory under `~/.claudio/teams/{name}/`
2. **Spawning** — each teammate launches as a goroutine running a full `query.Engine` with its own worktree (git-isolated branch). Sub-agents use depth tracking (max depth 2) to allow exploration sub-agents while preventing infinite recursion.
3. **Worktree isolation** — each teammate gets a fresh `git worktree` branch (`claudio/team/<name>-<id>`), so parallel agents never conflict on the filesystem. Lead merges changes when agents complete.
4. **Messaging** — agents communicate via file-based JSON inboxes with file locking (`flock`). Supports direct messages, broadcasts (`*`), and structured control messages.
5. **Completion** — when a teammate finishes, it sends a completion message to the team lead's mailbox. The lead's engine picks it up on the next turn and injects it as a `system-reminder`.
6. **Task tracking** — tasks created with `TaskCreate` can be linked to agents via `task_ids`. They auto-complete when the agent finishes and persist to SQLite across restarts.

### Team tools (available to the LLM)

| Tool | Description |
|------|-------------|
| `TeamCreate` | Create a new team (caller becomes lead) |
| `SpawnTeammate` | Spawn a named teammate from a crystallized agent persona |
| `SendMessage` | Send direct or broadcast messages between agents |
| `InstantiateTeam` | Re-create a team from a saved template |
| `TaskCreate` | Create a tracked task, optionally assigned to an agent |
| `TaskUpdate` | Update task status / description |
| `PurgeTeammates` | Remove all done/idle teammates from memory to keep context clean |

### Team templates — reusable team compositions

Save a team once, reuse it forever:

```bash
# In any future session, restore the full team in one call
InstantiateTeam("backend-team")
# → creates the team + pre-registers all members with their subagent_type
```

Template JSON format (`~/.claudio/team-templates/backend-team.json`):

```json
{
  "name": "backend-team",
  "description": "Backend feature team — mid for tasks, senior for architecture",
  "members": [
    { "name": "implementer", "subagent_type": "backend-mid",    "model": "claude-sonnet-4-6" },
    { "name": "architect",   "subagent_type": "backend-senior", "model": "claude-opus-4-6"   }
  ]
}
```

Each member can optionally include an `advisor` block — a cheap-executor / expensive-advisor split that gives a member access to a senior advisor model at critical decision points without running the whole agent on the expensive model. See [Two-Brain Advisor](#-two-brain-advisor) for the full config reference and cost rationale.

#### Built-in team templates (`~/.claudio/team-templates/`)

Four production-ready templates are included. These define the **worker roster** — not the team lead. The lead is always chosen separately with `/agent` (see [The Perfect Workflow](#the-perfect-workflow)).

**`backend-team.json`** — backend implementation + QA + DevOps:

```json
{
  "name": "backend-team",
  "description": "Full backend feature team — implementation, architecture, QA, and DevOps. Senior for architecture and complex decisions, mid for standard features, junior for boilerplate and simple tasks, QA for testing and security validation, DevOps for pipelines and infrastructure.",
  "members": [
    { "name": "rafael", "subagent_type": "backend-senior",    "model": "claude-opus-4-6" },
    { "name": "alex",   "subagent_type": "backend-mid",       "model": "claude-sonnet-4-6" },
    { "name": "sam",    "subagent_type": "backend-jr",        "model": "claude-haiku-4-5-20251001" },
    { "name": "kai",    "subagent_type": "devops",            "model": "claude-sonnet-4-6" },
    { "name": "quinn",  "subagent_type": "qa",                "model": "claude-sonnet-4-6" },
    { "name": "orion",  "subagent_type": "code-investigator", "model": "claude-haiku-4-5-20251001" }
  ]
}
```

**`frontend-team.json`** — UI architecture + components + accessibility QA:

```json
{
  "name": "frontend-team",
  "description": "Full frontend feature team — UI architecture, component implementation, and QA. Senior for design-system decisions and performance-critical work, mid for standard features and refactors, junior for boilerplate and simple components, QA for accessibility audits and E2E tests.",
  "members": [
    { "name": "sofia",  "subagent_type": "frontend-senior",   "model": "claude-opus-4-6" },
    { "name": "maya",   "subagent_type": "frontend-mid",      "model": "claude-sonnet-4-6" },
    { "name": "leo",    "subagent_type": "frontend-jr",       "model": "claude-haiku-4-5-20251001" },
    { "name": "quinn",  "subagent_type": "qa",                "model": "claude-sonnet-4-6" },
    { "name": "orion",  "subagent_type": "code-investigator", "model": "claude-haiku-4-5-20251001" }
  ]
}
```

**`fullstack-team.json`** — end-to-end product team spanning API and UI:

```json
{
  "name": "fullstack-team",
  "description": "Full-stack product team — backend architecture, frontend UI, QA, and DevOps. Use for end-to-end features that touch both API and UI layers. Senior engineers own architecture and high-risk changes; mid-level handle standard features; junior handles boilerplate; QA validates correctness and security; DevOps owns pipelines and infra.",
  "members": [
    { "name": "rafael", "subagent_type": "backend-senior",         "model": "claude-opus-4-6" },
    { "name": "alex",   "subagent_type": "backend-mid",            "model": "claude-sonnet-4-6" },
    { "name": "sofia",  "subagent_type": "frontend-senior",        "model": "claude-opus-4-6" },
    { "name": "maya",   "subagent_type": "frontend-mid",           "model": "claude-sonnet-4-6" },
    { "name": "leo",    "subagent_type": "frontend-jr",            "model": "claude-haiku-4-5-20251001" },
    { "name": "sam",    "subagent_type": "backend-jr",             "model": "claude-haiku-4-5-20251001" },
    { "name": "quinn",  "subagent_type": "qa",                     "model": "claude-sonnet-4-6" },
    { "name": "kai",    "subagent_type": "devops",                 "model": "claude-sonnet-4-6" },
    { "name": "orion",  "subagent_type": "code-investigator",      "model": "claude-haiku-4-5-20251001" }
  ]
}
```

**`go-fullstack-team.json`** — full Go stack with server-rendered htmx + go/template UI:

```json
{
  "name": "go-fullstack-team",
  "description": "Full Go fullstack feature team — backend implementation, server-rendered UI with htmx + go tmpl, architecture, QA, and DevOps. Senior engineers for architecture and complex decisions, mid for standard features, juniors for boilerplate and simple tasks, QA for testing and security validation, DevOps for pipelines and infrastructure.",
  "members": [
    { "name": "rafael", "subagent_type": "backend-senior",         "model": "claude-opus-4-6" },
    { "name": "alex",   "subagent_type": "backend-mid",            "model": "claude-sonnet-4-6" },
    { "name": "sam",    "subagent_type": "backend-jr",             "model": "claude-haiku-4-5-20251001" },
    { "name": "luna",   "subagent_type": "go-htmx-frontend-senior","model": "claude-opus-4-6" },
    { "name": "nova",   "subagent_type": "go-htmx-frontend-mid",   "model": "claude-sonnet-4-6" },
    { "name": "pixel",  "subagent_type": "go-htmx-frontend-jr",    "model": "claude-haiku-4-5-20251001" },
    { "name": "kai",    "subagent_type": "devops",                 "model": "claude-sonnet-4-6" },
    { "name": "quinn",  "subagent_type": "qa",                     "model": "claude-sonnet-4-6" },
    { "name": "orion",  "subagent_type": "code-investigator",      "model": "claude-haiku-4-5-20251001" }
  ]
}
```

Pick a template interactively with `/team` in the TUI — opens a picker showing all saved templates with their member rosters. The selected template is injected into the agent's system prompt so it knows which agents to use and when.

**Dynamic scaling:** templates define *roles*, not headcount. The lead can always spawn additional agents of the same `subagent_type` with different names for parallel tasks:

```
InstantiateTeam("backend-team")
SpawnTeammate(name="implementer-1", subagent_type="backend-mid", prompt="task A", task_ids=["1"])
SpawnTeammate(name="implementer-2", subagent_type="backend-mid", prompt="task B", task_ids=["2"])
SpawnTeammate(name="architect",     subagent_type="backend-senior", prompt="task C", task_ids=["3"])
```

### The Perfect Workflow

The intended pattern is a clean separation between **orchestrator** and **workers**:

```
/agent prab          ← Step 1: choose your product manager for this session
/team backend-team   ← Step 2: load the worker roster Prab will delegate to
```

Then just describe what you want to build. Prab plans, creates tasks, and spawns workers. You never talk to the workers directly.

```
You: /agent prab
You: /team backend-team
You: "Build the OAuth module with JWT tokens"

Prab:
  1. Clarifies scope (one question if ambiguous)
  2. Explores codebase briefly with code-investigator or codex
  3. Presents plan:
       - Task 1: OAuth service layer (backend-mid)
       - Task 2: DB migrations (backend-jr)
       - Task 3: Architecture review + tests (backend-senior)
  4. After your confirmation:
       InstantiateTeam("backend-team-oauth")
       TaskCreate("OAuth service layer")  → id=1
       TaskCreate("DB migrations")        → id=2
       TaskCreate("Review + tests")       → id=3
       SpawnTeammate("alex",    backend-mid,    "Write OAuth service",  task_ids=["1"])
       SpawnTeammate("sam",     backend-jr,     "Write migrations",     task_ids=["2"])
       SpawnTeammate("rafael",  backend-senior, "Review + write tests", task_ids=["3"])
  5. All 3 run in parallel in isolated git worktrees
  6. Tasks auto-complete as agents finish
  7. Prab merges worktrees, runs build, reports back to you
```

**Key rules:**
- `/agent` selects the **lead** (orchestrator/PM). Use `prab` for general-purpose planning.
- `/team` selects the **worker roster** (template). The lead decides which workers to spawn and when.
- The lead is never in the team template — it's always selected separately.
- You interact only with the lead. Workers report back to the lead, not to you.

### Example: parallel feature implementation

```
You: /agent prab
You: /team backend-team
You: "Build the OAuth module — split across agents"

Prab:
  1. InstantiateTeam("backend-team-oauth")
  2. TaskCreate × 3 (service layer, migrations, tests)
  3. SpawnTeammate("alex",   backend-mid,    "Write OAuth service",   task_ids=["1"])
  4. SpawnTeammate("sam",    backend-jr,     "Write migrations",      task_ids=["2"])
  5. SpawnTeammate("rafael", backend-senior, "Review + write tests",  task_ids=["3"])
  → All 3 run in parallel in isolated worktrees
  → Tasks auto-complete when agents finish
  → Prab merges worktrees + does final build check
```

### Example: code review team

```
You: "Review the changes in this PR"

Claudio:
  1. Creates team "review-team"
  2. SpawnTeammate "security" → "Check for security issues"
  3. SpawnTeammate "style"    → "Check code style and naming"
  4. SpawnTeammate "tests"    → "Verify test coverage"
  5. All run in parallel, report back to lead
  6. Lead consolidates findings into a single review summary
```

### Sync vs async spawning

| Mode | Behaviour | Use when |
|------|-----------|----------|
| `run_in_background: false` (default) | Lead blocks until agent completes, result returned inline | You need the result before the next step |
| `run_in_background: true` | Lead continues immediately; completion arrives via mailbox poll | Parallel fire-and-forget tasks |

Background agents auto-open the **Agents panel** (`space a`) in the TUI so you can watch live progress without losing your prompt focus.

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

### Team lifecycle and cleanup

Teams are **ephemeral by design** — created for a task, deleted when done. The durable, reusable parts (agent personas, learned memory, orchestration logic) live in dedicated locations that outlive any single run:

| What | Where | Survives team deletion? |
|------|-------|------------------------|
| Agent persona | `.claudio/agents/<name>.md` | ✅ Yes |
| Agent memory | `.claudio/memory/agents/<name>/` | ✅ Yes |
| Orchestration logic | `.claudio/skills/<harness-name>/skill.md` | ✅ Yes |
| Team config + inboxes | `~/.claudio/teams/<team>/` | ❌ Deleted |
| In-memory runner state | Process memory | ❌ Cleared |

Team cleanup is handled automatically when a harness finishes — cancels running members, drains goroutines, deletes config/inboxes from disk, and clears in-memory state. This means you can invoke the same harness repeatedly without state accumulating across runs. Each invocation gets a fresh team with clean inboxes.

### Using crystallized agents as teammates

Any agent you've [crystallized](#agent-crystallization) can be used directly as a team member. When a harness (or the LLM) spawns an agent via `subagent_type: "<agent-name>"`, Claudio:

1. Loads the agent's persona prompt from `.claudio/agents/<name>.md`
2. **Injects the agent's accumulated memory** — everything it learned across prior sessions carries into the team run
3. Assigns the agent a team identity (name, color, mailbox) for this run

This is the right way to get "warm" agents in a team. Instead of wasting time rebuilding context from scratch on every invocation, the agent picks up where its memory left off.

```
# Agent "security-auditor" was crystallized from a prior session.
# When a harness spawns it as a teammate, its memory is automatically loaded.

/security-harness audit the payments module
# → Spawns security-auditor teammate WITH prior memory about your codebase conventions,
#   known false-positive patterns, and project-specific security rules
```

The `/harness` skill accounts for this: Phase 0 now inventories all existing crystallized agents, and Phase 4 explicitly checks whether any existing agent matches each role before creating a new one. Reusing an existing agent is preferred — it brings accumulated memory and avoids fragmenting learnings across duplicate personas.

---

## 🧠 Two-Brain Advisor

The **Two-Brain Advisor** pattern splits the cognitive work of a task into two roles:

| Role | Model | Job |
|------|-------|-----|
| **Executor** | Cheap (e.g. Haiku) | Reads files, runs tools, writes code — all the mechanical work |
| **Advisor** | Expensive (e.g. Opus) | Thinks strategically at two fixed moments; never touches files or runs commands |

The advisor is consulted **at most twice per task**: once when the executor has oriented itself and formed a plan (PLAN mode), and once when the executor believes the task is done (REVIEW mode). Outside those two moments the expensive model is completely idle.

This delivers senior-level judgment at a fraction of the cost of running a senior model for every turn.

### Consultation protocol

The executor calls the built-in `advisor` tool with a structured brief. There are two modes:

**PLAN mode** — called once after orientation, before writing any code:

```
advisor(
  mode: "plan",
  orientation_summary: "Codebase uses repository pattern. The auth module lives in internal/auth/.",
  proposed_approach:   "Add JWT middleware: 1) parse token 2) inject claims into context 3) gate routes",
  decision_needed:     "Should I add the middleware at the router level or per-handler?",
  context_notes:       "No existing middleware abstraction. Two existing protected routes."
)
```

**REVIEW mode** — called once after the executor thinks the task is complete:

```
advisor(
  mode: "review",
  original_plan:        "Add JWT middleware at router level, inject claims, gate /api/* routes",
  execution_summary:    "Added JWTMiddleware in internal/auth/middleware.go, wired in router.go, added 3 tests",
  outcome_artifacts:    "internal/auth/middleware.go, internal/router/router.go, internal/auth/middleware_test.go",
  confidence:           "high — tests pass, all routes protected"
)
```

### Advisor behavioral rules

The built-in advisor (and the `advisor-sr` agent persona) operates under strict constraints:

- **≤ 100 words** per response — bullet lists, no prose paragraphs
- **PLAN mode** returns a numbered execution plan with `⚠` risk flags on uncertain steps
- **REVIEW mode** returns exactly one of three verdicts:
  - `PASS` — work is complete and correct, executor can declare done
  - `NEEDS_FIX <what>` — specific problem found; executor must address it
  - `INCOMPLETE <what>` — original plan item was skipped; executor must finish it
- **No clarifying questions** — the advisor acts on what it receives
- **No praise** — only actionable signals
- **Completeness before correctness** — an incomplete correct solution is worse than a complete imperfect one

The advisor's tool access is intentionally restricted: it can call **WebSearch** and **WebFetch** to look things up, but it cannot read files, run commands, or write to the filesystem. All ground truth comes from the executor's brief.

### Enabling the advisor for a principal agent

Set in `~/.claudio/settings.json` (global) or `.claudio/settings.json` (project-level):

```json
{
  "advisor": {
    "model": "claude-opus-4-6"
  }
}
```

With a custom agent persona and usage cap:

```json
{
  "advisor": {
    "subagentType": "advisor-sr",
    "model": "claude-opus-4-6",
    "maxUses": 6
  }
}
```

- `model` — required; the model used for the advisor's reasoning loop
- `subagentType` — optional; loads a crystallized agent from `.claudio/agents/<name>.md` as the advisor persona
- `maxUses` — optional; hard cap on advisor calls per session (`0` = unlimited). Default is off (null = advisor disabled).

### Configuring per-member advisors in team templates

Any team member can have its own advisor. Add an `advisor` block to the member entry:

```json
{
  "name": "efficient-team",
  "description": "Haiku executors with Opus advisors — maximum throughput at minimum cost",
  "members": [
    {
      "name": "rafael",
      "subagent_type": "backend-senior",
      "model": "claude-haiku-4-5-20251001",
      "advisor": {
        "subagent_type": "advisor-sr",
        "model": "claude-opus-4-6",
        "max_uses": 4
      }
    },
    {
      "name": "alex",
      "subagent_type": "backend-mid",
      "model": "claude-haiku-4-5-20251001",
      "advisor": {
        "subagent_type": "advisor-sr",
        "model": "claude-opus-4-6",
        "max_uses": 4
      }
    },
    { "name": "quinn", "subagent_type": "qa",               "model": "claude-haiku-4-5-20251001" },
    { "name": "orion", "subagent_type": "code-investigator", "model": "claude-haiku-4-5-20251001" }
  ]
}
```

Members without an `advisor` block run without one. QA and code-investigator roles typically don't need advisors — their work is mechanical or read-only.

### The `efficient-team` template

The built-in `efficient-team.json` template puts all workers on Haiku and gives the developers an Opus advisor:

| Member | Executor | Advisor |
|--------|----------|---------|
| `rafael` | backend-senior on Haiku | advisor-sr on Opus (max 4 uses) |
| `alex` | backend-mid on Haiku | advisor-sr on Opus (max 4 uses) |
| `sam` | backend-jr on Haiku | advisor-sr on Opus (max 4 uses) |
| `kai` | devops on Haiku | advisor-sr on Opus (max 4 uses) |
| `quinn` | qa on Haiku | — |
| `orion` | code-investigator on Haiku | — |

**Cost profile:** every task costs at most 2 Opus calls (plan + review). All other turns run on Haiku. This is dramatically cheaper than running senior agents on Opus for every conversation turn — and produces better outcomes than running junior agents without any strategic guidance.

### Creating a custom advisor agent

Create `~/.claudio/agents/advisor-sr.md` (or `.claudio/agents/advisor-sr.md` for project-scoped):

```markdown
---
name: advisor-sr
description: Senior advisor — strategic PLAN + REVIEW for executor agents
---

You are a senior engineering advisor. You are consulted at exactly two moments per task:
PLAN mode (before the executor writes code) and REVIEW mode (after the executor believes it is done).

PLAN mode — return a numbered execution plan:
1. <step>
2. <step>  ⚠ <risk if applicable>
...

REVIEW mode — return exactly one verdict on its own line:
PASS
NEEDS_FIX <specific problem>
INCOMPLETE <which plan item was skipped>

Rules:
- ≤ 100 words total
- Bullet lists only, no prose paragraphs
- No clarifying questions — act on what you receive
- No praise — only actionable signals
- Completeness before correctness
```

### How the advisor is wired up

The advisor is **not** something the executor manually spawns via `SpawnTeammate`. Instead, when a team member has an `advisor` block in its template, the runtime automatically injects an `advisor` tool into that agent's tool list at spawn time. The agent calls it like any other tool:

```
advisor(mode="plan", orientation_summary="...", proposed_approach="...", decision_needed="...")
advisor(mode="review", original_plan="...", execution_summary="...", outcome_artifacts="...", confidence="high")
```

The `max_uses` counter is a shared pointer — the same counter tracks both PLAN and REVIEW calls, so `max_uses: 4` means at most 4 total advisor consultations across the agent's lifetime, not 4 per mode.

### How context flows to the advisor

The advisor receives context from two sources simultaneously:

**1. The agent's live message history (automatic)**

At call time, the runtime invokes a `getMessages()` callback that captures the executor's full conversation history up to that moment. This history is then compressed via `compact.Compact` — summarized down to ~10 messages using the instruction:

> *"Summarize the work done so far, preserving key decisions, findings, and errors."*

If compression fails, the original uncompressed history is used as a fallback.

**2. The structured brief (explicit, written by the agent)**

The agent fills in the brief fields when calling the tool. In PLAN mode: `orientation_summary`, `proposed_approach`, `decision_needed`, and optionally `context_notes`. In REVIEW mode: `original_plan`, `execution_summary`, `outcome_artifacts`, and `confidence`.

**How they're combined**

The compressed history is used as the base conversation. The formatted brief is appended as the final user message on top of it. The advisor then sees:

```
[compressed history of what the agent did so far]
     ↓
[structured brief: what the agent found, plans, and needs decided]
```

This gives the advisor both the raw work trail and a synthesized summary — the brief fills in intent; the history provides evidence.

An `ensureAlternatingRoles` pass fixes any consecutive same-role messages before the payload is sent (the Anthropic API rejects non-alternating role sequences).

The advisor's own tool-calling loop is capped at 5 iterations. Its tool access is intentionally restricted to **WebSearch** and **WebFetch** only — it cannot read files, run commands, or write anything. All ground truth comes from what the executor describes in the brief.

### Prompt composition rule

How the advisor's system prompt is assembled depends on whether `subagentType` is set:

| Config | Advisor system prompt |
|--------|-----------------------|
| No `subagentType` | Built-in `AdvisorSystemPrompt()` only |
| `subagentType` set | Agent's persona prompt **+** `AdvisorSystemPrompt()` appended after |

The executor also receives an `AdvisorProtocolSection()` injection: instructions on when to call the `advisor` tool and how to write a well-formed brief.

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

0. **Audits** — inventories existing harnesses AND crystallized agents in `.claudio/agents/`, `~/.claudio/agents/`, `.claudio/skills/`, and CLAUDIO.md; decides whether to create, extend, repair, or replace
1. **Clarifies** — asks what task the harness covers, what it should output, and who will use it
2. **Explores** — scans your project to understand languages, frameworks, existing agents/skills, and coding conventions
3. **Selects execution mode and pattern** — chooses Agent Teams vs Sub-agents based on whether inter-agent communication is needed, then picks the best-fit architecture pattern with an ASCII diagram; asks for your approval
4. **Designs roster** — for each role, first checks whether an existing crystallized agent matches (reuse brings accumulated memory); only creates new agents for gaps; reports "reusing X" or "creating Y" for every role
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

Claudio supports recurring agent tasks via a lightweight cron runner. Tasks are stored in `~/.claudio/cron.json` and polled every 60 seconds.

### Creating a cron

Use the `CronCreate` tool inside a session (requires approval):

```
CronCreate:
  schedule: "@every 1h"
  prompt:   "Check for failing tests and open a GitHub issue if any"
  type:     "background"   # or "inline"
```

Supported schedules: `@every <duration>` (e.g., `@every 30m`, `@every 2h`), `@hourly`, `@daily`, `HH:MM` (e.g., `09:00`).

### Execution modes

| Mode | How it runs |
|------|-------------|
| **inline** | Injects the prompt as a user message into the target session. The cron ID is embedded in the message (`[cron_id: xxx]`) so the AI can call `CronDelete` when the task is done. |
| **background** | Spawns an isolated engine with no TUI attachment. Result is stored as an assistant message in the session history. Ideal for monitoring, summarization, or notification tasks. |

### Managing crons

| Tool | Description |
|------|-------------|
| `CronCreate` | Create a new scheduled task |
| `CronList` | List all crons with schedule, type, and next-run time |
| `CronDelete` | Delete a cron by ID |

### ComandCenter integration

When running `comandcenter`, crons are shared across all attached `claudio` sessions:
- **Inline crons** are delivered to the target session via the hub WebSocket.
- **Background crons** spawn an isolated engine, store the result in the ComandCenter DB, and broadcast it to all connected browser clients in real time.

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

### claudio-screenshot — Visual Feedback Plugin

[**claudio-screenshot**](https://github.com/Abraxas-365/claudio-screenshot) is a first-party plugin that lets the AI take screenshots of any web page and see them inline — no drag-and-drop needed. Screenshots are automatically compressed (resized to ≤1024px, re-encoded as JPEG quality 72) before being sent as vision blocks, cutting image token cost by 3–8×.

**Install:**

```bash
git clone https://github.com/Abraxas-365/claudio-screenshot
cd claudio-screenshot
make install   # builds and copies binary into ~/.claudio/plugins/
```

**macOS — sign after install:**

```bash
codesign --force --sign - ~/.claudio/plugins/claudio-screenshot
```

**Usage (the AI calls this automatically, or you can run it directly):**

```bash
# Screenshot any page
claudio-screenshot take http://localhost:8080

# Full page capture
claudio-screenshot take http://localhost:8080 --full-page

# Save a session for protected pages (interactive — supports OAuth, OTP, SSO)
claudio-screenshot session save myapp --url http://localhost:8080 --interactive

# Screenshot a protected page using a saved session
claudio-screenshot take http://localhost:8080/dashboard --session myapp --full-page
```

Once installed, just ask Claudio: *"take a screenshot of localhost:8080/dashboard"* — it will capture, compress, and analyze the UI inline.

---

### Eager plugin loading

By default all plugins are **deferred** — their schema is loaded on demand via `ToolSearch`, keeping the system prompt lean. To make a plugin always available (eager), add its name to `eagerPlugins` in your settings:

```json
{ "eagerPlugins": ["claudio-codex"] }
```

Names are without the `plugin_` prefix. Eager plugins appear in the tool list on every turn without requiring a `ToolSearch` call first.

---

## Model Configuration

### Multi-Provider Support

Claudio supports routing models to different API providers (Groq, OpenAI, Ollama, Together, vLLM, or any OpenAI-compatible endpoint) alongside the default Anthropic backend.

Configure providers in `~/.claudio/init.lua`:

```lua
-- Groq (fast, free tier)
claudio.register_provider({
  name     = "groq",
  type     = "openai",
  base_url = "https://api.groq.com/openai/v1",
  api_key  = "$GROQ_API_KEY",
  routes   = { "llama-*", "mixtral-*", "gemma*" },
})

-- OpenAI
claudio.register_provider({
  name     = "openai",
  type     = "openai",
  base_url = "https://api.openai.com/v1",
  api_key  = "$OPENAI_API_KEY",
  routes   = { "gpt-*", "o1*" },
})

-- Ollama (local)
claudio.register_provider({
  name     = "ollama",
  type     = "ollama",
  base_url = "http://localhost:11434",
  routes   = { "qwen*", "llama3*" },
})
```

| Field | Description |
|-------|-------------|
| `providers.<name>.apiBase` | Base URL for the provider's API |
| `providers.<name>.apiKey` | API key (plain string or `$ENV_VAR` to read from environment) |
| `providers.<name>.type` | `"openai"` for OpenAI-compatible APIs, `"anthropic"` for Anthropic-compatible |
| `modelRouting.<pattern>` | Glob pattern mapping model names to a provider name |

Models that don't match any routing pattern use the default Anthropic backend. To use a routed model, set it with `--model`, via `:set model <name>` in the TUI, or in `init.lua`:

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

## 🎨 Design Agent

Claudio can create interactive HTML mockups, export developer handoff bundles, and verify that your implementation visually matches the design — all from the same session.

### Overview

Design sessions are stored at `~/.claudio/projects/{project-slug}/designs/{session-name}/`. Each session contains:
- `manifest.json` (v2 format) — screen list with breakpoint + viewport metadata per screen
- `screenshots/` — PNG captures of each artboard
- `bundle/mockup.html` — self-contained offline-capable bundle (written by `BundleMockup`)

The `/ui-ux-pro-max` skill gives agents access to 50+ styles, 161 color palettes, 57 font pairings, and design system guidance.

### Design tools

| Tool | Availability | Description |
|------|-------------|-------------|
| `ListDesigns` | All agents | List all design sessions for the current project |
| `BundleMockup` | `design` capability | Inline all assets into a single self-contained HTML file with an infinite pan/zoom canvas |
| `ExportHandoff` | `design` capability | Package a session into a developer handoff bundle: `spec.md`, component inventory, interaction map, `tokens-used.json` |
| `ReviewDesignFidelity` | All agents | Compare rendered HTML templates or live URLs against saved design screenshots via vision AI; returns fidelity score 0–100; pass threshold ≥ 75 |

> `BundleMockup` and `ExportHandoff` are only registered when the active agent declares the `design` capability. `ListDesigns` and `ReviewDesignFidelity` are always available.

### Typical workflow

```
1. Design agent generates mockup → RenderMockup (screenshots each screen)
2. BundleMockup        → self-contained HTML (shareable link via publicUrl)
3. ExportHandoff       → spec.md + component inventory for the dev team
4. Dev implements UI
5. ReviewDesignFidelity → compares live pages against design screenshots → score ≥ 75 = pass
```

**ReviewDesignFidelity** accepts either a local template path or a live URL (screenshotted via Playwright):

```json
{
  "screens": [
    { "name": "Dashboard", "url": "http://localhost:8080/dashboard" },
    { "name": "Login",     "template_path": "templates/login.html", "css_paths": ["static/app.css"] }
  ],
  "session_name": "my-design-session"
}
```

Pass threshold: `overall_score >= 75`. Results include per-screen gaps and suggestions.

### Design capability and teams

**`capabilities: ["design"]` belongs only on standalone (principal) agents — never on team sub-agents.**

#### Why

Design tools (`RenderMockup`, `BundleMockup`, `CreateDesignSession`, `ExportHandoff`) are *creation* tools. They write sessions to `~/.claudio/projects/{slug}/designs/`, render screenshots, and produce bundles for human review. This is inherently a human-in-the-loop workflow: the output is meant to be seen, critiqued, and approved before any implementation begins.

Granting `design` capability to a team sub-agent breaks this in several ways:

1. **No human review gate.** A sub-agent generating mockups autonomously inside a pipeline means the design is consumed by the next agent without ever being seen or approved by a human. Low-quality or wrong-direction output propagates silently.

2. **Shared mutable state outside worktrees.** Design sessions live in `~/.claudio/projects/` — completely outside the isolated git worktrees that team agents run in. Two parallel sub-agents with `design` capability writing to the same designs directory will race and clobber each other's sessions.

3. **Wrong tool for the job.** What a team implementation agent actually needs is `ReviewDesignFidelity` — to *verify* that the UI they built matches an already-approved design. That tool is available to all agents by default and is the correct integration point between design and implementation.

#### The correct split

```
Phase 1 — Design (standalone principal agent, human in the loop)
  human → /hifi or /mockup skill
  agent → CreateDesignSession → RenderMockup → BundleMockup
  human → reviews interactive canvas, requests iterations
  human → approves session

Phase 2 — Implementation (team, autonomous)
  spawn team with approved session name/dir as context
  → frontend agent implements UI
  → QA agent calls ReviewDesignFidelity against live pages
  → score ≥ 75 = pass
```

#### How to wire the handoff

In your team task description, pass the approved session directory:

```
Design session: ~/.claudio/projects/myapp/designs/20260425-dashboard/
Screens: Dashboard, Login, Settings
Fidelity target: overall_score >= 75
```

QA agent calls `ReviewDesignFidelity` with that session name. No `design` capability needed anywhere in the team.

#### Agent definition example

```yaml
# ✅ Correct — standalone design agent
---
name: design
capabilities: ["design"]
model: opus
---
You are a senior product designer...
```

```yaml
# ❌ Wrong — team sub-agent with design capability
---
name: ui-designer
capabilities: ["design"]   # Don't do this in a team member
---
```

```yaml
# ✅ Correct — team frontend agent, no design capability needed
---
name: frontend
tools: "*"
# ReviewDesignFidelity is available to all agents by default
---
```

### Manifest format (v2)

```json
{
  "version": 2,
  "session": "my-design-session",
  "created_at": "2025-01-01T00:00:00Z",
  "screens": [
    {
      "name": "Dashboard",
      "breakpoint": "desktop",
      "viewport": { "width": 1440, "height": 900 }
    },
    {
      "name": "Dashboard Mobile",
      "breakpoint": "mobile",
      "viewport": { "width": 390, "height": 844 }
    }
  ]
}
```

`ReviewDesignFidelity` uses the per-screen viewport from the manifest when no global `viewport_width`/`viewport_height` override is provided, and groups results `by_breakpoint` in the output.

### Prerequisites for ReviewDesignFidelity

```bash
# Node.js >= 18 required
node --version

# Install Playwright Chromium
npx playwright install chromium
```

---

## Command Center (Web / Mobile UI)

**ComandCenter** is a separate server binary that provides a WhatsApp-style browser and mobile chat interface for Claudio sessions. It acts as a hub: one or more `claudio` processes attach to it over WebSocket, and browser/mobile clients interact with those sessions through the web UI.

```
[ claudio --attach ]  ←→  [ comandcenter server ]  ←→  [ browser / PWA ]
[ claudio --attach ]         (hub, storage, crons)
```

### Starting the server

```bash
comandcenter --password mysecret --port 8080
# → http://localhost:8080
```

| Flag | Default | Description |
|------|---------|-------------|
| `--password` | _(required)_ | Shared password for browser login |
| `--port` | `8080` | HTTP port |
| `--db` | `~/.claudio/comandcenter.db` | SQLite database path |
| `--data-dir` | `~/.claudio/uploads/` | File upload storage |

### Attaching a Claudio session

```bash
claudio --attach http://localhost:8080 \
        --name "my-project" \
        --master
```

| Flag | Description |
|------|-------------|
| `--attach <url>` | ComandCenter server URL. Forces `--headless`; engine runs in a persistent loop. |
| `--name <name>` | Session display name in the web UI. Resumes an existing session of the same name on reconnect. |
| `--master` | Marks this session as the master — enables the `SendToSession` and `SpawnSession` tools. |

Auth for `--attach` is read from `COMANDCENTER_PASSWORD` environment variable.

### Web UI features

#### Chat list + session management
- Sessions list with active/inactive status indicators
- Create, archive, and delete sessions
- Responsive layout: single column on mobile, two-panel on desktop

#### Real-time messaging (WebSocket)
- Messages stream from the attached Claudio engine to the browser in real time
- Markdown rendered with GFM (tables, strikethrough, task lists, syntax highlighting)
- Date dividers and typing indicator

#### File uploads
- WhatsApp-style file picker — select file, preview before sending
- Uploaded files are forwarded to the Claudio engine and processed by the AI
- Served back via `/api/uploads/...`

#### @mentions — cross-session routing
Messages prefixed with `@session-name` are routed to the named session. The reply appears in both the originating session and the target session (WhatsApp reply-quote style).

#### Session info panel
Slide-in panel (from header) with three tabs:
- **Tasks** — tasks created by the AI (`TaskCreate`), with status badges and markdown detail view
- **Team** — active agents and their status
- **Crons** — list of scheduled tasks; delete crons from the UI

#### Push notifications (PWA)
- Progressive Web App — installable on iOS/Android
- Push notifications when the AI finishes a response while the app is in the background
- Subscription management via `/api/push/subscribe`

#### Interrupt
- Interrupt the running AI turn via the `+` menu → **Interrupt**, or `POST /api/sessions/{id}/interrupt`

### API (Bearer auth)

Machine-readable endpoints for scripting or integrations:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/sessions` | GET | List sessions |
| `/api/sessions/{id}/message` | POST | Send a message to a session |
| `/api/sessions/by-name/{name}/message` | POST | Send to session by name (creates if missing) |
| `/api/sessions/{id}/interrupt` | POST | Interrupt current turn |
| `/api/sessions/{id}/archive` | PATCH | Archive a session |
| `/api/sessions/{id}` | DELETE | Delete a session |
| `/api/sessions/{id}/upload` | POST | Upload a file |
| `/api/crons` | GET | List scheduled tasks |
| `/api/crons/{id}` | DELETE | Delete a cron |
| `/api/push/subscribe` | POST/DELETE | Manage push subscriptions |
| `/api/push/vapid-public-key` | GET | VAPID public key for push |
| `/ws/attach` | WebSocket | Claudio session attach endpoint |
| `/ws/ui` | WebSocket | Browser real-time message stream |

All `/api/*` endpoints require `Authorization: Bearer <password>`.

### Architecture notes

- Pure Go binary — no Node.js, no build step required
- HTML rendered server-side with `html/template`; CSS via Tailwind (vendored)
- WebSocket at `/ws/ui` pushes rendered message HTML to all connected browser clients
- Sessions and messages stored in a dedicated SQLite DB (`comandcenter.db`) — separate from `~/.claudio/claudio.db`
- Cron runner is embedded in the server; cron state is shared from `~/.claudio/cron.json`

---

## Headless Mode

`--headless` disables the TUI and runs a single prompt non-interactively:

```bash
claudio --headless "run the tests and summarize failures"
```

It is also the engine mode used by `--attach`: when connecting to a ComandCenter server, `claudio` runs headless in a persistent loop, processing messages injected from the web UI.

```bash
# Persistent session attached to ComandCenter
COMANDCENTER_PASSWORD=mysecret \
  claudio --attach http://localhost:8080 --name "my-project" --master
```

The process stays alive, reconnects on drop, and resumes the same session by name.

---

## Filesystem Layout

```
~/.claudio/                    # Global config directory
  init.lua                     # YOUR personal config — keymaps, theme, providers, hooks
  settings.json                # Remaining machine-readable settings (model choice, etc.)
  state.json                   # Machine-written state only (plugin data) — never hand-edit
  credentials.json             # Auth credentials
  lsp/                         # LSP server configs (Lua)
    go.lua                     # gopls config
    typescript.lua             # tsserver config
  plugins/                     # Lua plugins (community / third-party)
    <name>/
      init.lua                 # Plugin entry point
  team-templates/              # Team composition templates (Lua or JSON)
    backend-team.lua
  agents/                      # Custom agent definitions
    <name>/                    # Directory-form agent (preferred)
      AGENT.md                 # Agent persona
      skills/                  # Agent-specific skills
        <skill-name>/
          SKILL.md
  skills/                      # User skills
  rules/                       # User rules
  contexts/                    # Context profiles
  plans/                       # Plan mode files
  data/                        # Runtime data — never version-control
    claudio.db                 # SQLite (sessions, messages, audit)
    cache/                     # Model capabilities cache
    sessions/
    memory/
    uploads/                   # Files uploaded via the web UI
    projects/                  # Per-project data
      <project-slug>/memory/

.claudio/                      # Per-project config (created by /init or claudio init)
  init.lua                     # Project Lua overrides (wins over ~/.claudio/init.lua)
  settings.json                # Project settings (overrides global)
  rules/                       # Project rules
  skills/                      # Project skills
  agents/                      # Project agents
  memory/                      # Project memories
CLAUDIO.md                     # Project instructions
```

> **Version-control tip:** commit `~/.claudio/init.lua`, `lsp/`, `team-templates/`, `agents/`, `skills/` to your dotfiles repo. Never commit `state.json` or `data/`.

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

## 🤝 Contributing

Contributions are welcome! Claudio is built with strict architectural conventions — please review [`.claudio/rules/project.md`](.claudio/rules/project.md) and [`CLAUDIO.md`](CLAUDIO.md) before opening a PR.

```bash
make build      # build with version injection
make test       # run all tests
make lint       # golangci-lint
```

**Key constraints:**
- Pure Go — no CGO (we use `modernc.org/sqlite`)
- Never alter existing migration files — only add new ones
- Business logic lives under `internal/`

---

## 📄 License

Released under the [MIT License](LICENSE).

<div align="center">

**Built with ❤️ in Go** · [Report a bug](https://github.com/Abraxas-365/claudio/issues) · [Star on GitHub](https://github.com/Abraxas-365/claudio)

</div>
