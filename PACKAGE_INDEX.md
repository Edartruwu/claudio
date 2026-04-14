# Claudio Package Index

Quick reference guide to all 35 internal packages and their responsibilities.

## Foundational (Config, Storage, Auth)

| Package | Responsibility | Key Files |
|---------|---|---|
| **config** | Settings management, config merging, file paths | config.go, env.go, trust.go, validation.go |
| **auth** | Credential resolution from multiple sources | resolver.go, oauth/, refresh/, storage/ |
| **storage** | SQLite database abstraction (sessions, messages, audit, FTS) | db.go, sessions.go, audit.go, memory_fts.go |
| **security** | Path/command validation, secret scanning, audit logging | sandbox.go, audit.go |

## API & LLM

| Package | Responsibility | Key Files |
|---------|---|---|
| **api** | Anthropic Messages API client with streaming, thinking, caching | client.go, provider.go, usage.go |
| **api/provider** | Multi-provider abstraction (OpenAI, Anthropic, Ollama) | provider.go, anthropic.go, openai.go, ollama.go, translator.go |
| **models** | Model capabilities caching (max tokens, thinking support, etc.) | capabilities.go |

## Execution Engine & Tools

| Package | Responsibility | Key Files |
|---------|---|---|
| **query** | Main conversation loop with tool execution, permissions, hooks | engine.go, context.go, output.go, sections.go |
| **tools** | Tool registry (50+ tools), deferred loading, execution | registry.go, types.go + 40+ tool implementations |
| **permissions** | Content-pattern permission rule evaluation | rules.go, capabilities.go |
| **orchestrator** | Multi-agent workflow coordination with phases | orchestrator.go |

## Agents & Teams

| Package | Responsibility | Key Files |
|---------|---|---|
| **agents** | Agent definitions, discovery, capability metadata | agents.go, crystallize.go, orchestrator.go |
| **teams** | Team management, member coordination, mailbox messaging | team.go, runner.go, mailbox.go, templates.go |
| **session** | Session lifecycle management (start, resume, add messages) | session.go, sharing.go |
| **tasks** | Background task runtime, cron scheduling, dream consolidation | runtime.go, store.go, agent_task.go, cron.go, dream.go, shell.go |

## Services

| Package | Responsibility | Key Files |
|---------|---|---|
| **services/memory** | Persistent cross-session memory (markdown files + FTS) | memory.go, extract.go, scoped.go, loader.go |
| **services/skills** | Skill registry (instruction extensions) | loader.go |
| **services/lsp** | Language Server Protocol client manager | (files in services/lsp/) |
| **services/mcp** | Model Context Protocol server management | manager.go |
| **services/analytics** | Token usage tracking and cost calculation | (files in services/analytics/) |
| **services/cachetracker** | Prompt caching metrics | (in services/) |
| **services/compact** | Conversation compaction state management | (in services/) |
| **services/difftracker** | Git diff tracking | (in services/) |
| **services/filtersavings** | Output filter statistics | (in services/) |
| **services/notifications** | User notifications | (in services/) |
| **services/naming** | Automatic session naming/titling | (in services/) |
| **services/toolcache** | Tool execution result caching | (in services/) |

## Events & Extensions

| Package | Responsibility | Key Files |
|---------|---|---|
| **bus** | Concurrent pub/sub event system | bus.go, events.go |
| **hooks** | Lifecycle event shell command execution | hooks.go |
| **plugins** | Dynamic plugin discovery and loading | plugins.go, lsp.go, proxy.go, reconstruct.go |
| **learning** | Pattern extraction and instinct learning | learning.go, capabilities.go |

## Utility & Infrastructure

| Package | Responsibility | Key Files |
|---------|---|---|
| **prompts** | System prompt construction with caching boundary | system.go, context.go, sections.go, tools.go, output.go, advisor.go |
| **git** | Git operations (status, diff, log, stash) | (git operations utilities) |
| **ratelimit** | API rate limiting | (rate limiting logic) |
| **rules** | Configuration-based rule system | (rule evaluation) |
| **snippets** | Snippet expansion for boilerplate | (snippet logic) |
| **keybindings** | Keyboard shortcut definitions | (keybinding definitions) |
| **utils** | General utilities and helpers | (utility functions) |
| **bridge** | Cross-session communication via Unix sockets | bridge.go, git.go, rules.go |
| **query** | (see above - core execution) | |

## UI Layers

| Package | Responsibility | Key Files |
|---------|---|---|
| **cli** | Cobra CLI with command routing | root.go, detect.go, init.go, web.go, auth.go, commands/ |
| **tui** | Bubble Tea terminal UI with full component system | editor.go, attachments.go, focus.go, context.go, layout.go + subpackages |
| **web** | go-templ + HTMX + SSE browser UI | server.go, handler.go, sessions.go, static/, templates/ |

## Meta

| Package | Responsibility | Key Files |
|---------|---|---|
| **app** | Central dependency injection and wiring | app.go |

---

## Dependency Graph (Simplified)

```
cmd/claudio
  └─ cli (Cobra)
     └─ app (wiring)
        ├─ config (settings)
        ├─ auth (credentials)
        ├─ storage (SQLite)
        ├─ api (LLM client)
        │  └─ api/provider (multi-provider)
        ├─ tools (50+ tools)
        │  ├─ security (path/cmd validation)
        │  ├─ plugins (dynamic tools)
        │  ├─ services/lsp (code intelligence)
        │  └─ services/mcp (protocol servers)
        ├─ query (execution engine)
        │  ├─ permissions (rules)
        │  ├─ hooks (lifecycle)
        │  ├─ bus (events)
        │  └─ models (capabilities)
        ├─ agents (agent metadata)
        ├─ teams (team coordination)
        │  └─ session (session lifecycle)
        ├─ services/memory (persistent memory)
        ├─ services/skills (instruction extensions)
        ├─ learning (instinct learning)
        ├─ prompts (system prompt construction)
        ├─ orchestrator (workflow coordination)
        └─ tasks (background execution)

UI: tui, web, cli
```

---

## Tool Registry (50+ Tools)

### File Operations
- **Bash** – Shell command execution
- **Read** – File reading with cache
- **Write** – File creation with snippets
- **Edit** – File editing
- **Glob** – Pattern file matching
- **Grep** – Content search

### Code Intelligence
- **LSP** – Language server integration
- **Models** – Model capability lookup

### Agent Coordination
- **Agent** – Sub-agent spawning
- **Memory** – Memory operations
- **Recall** – Memory search
- **Skill** – Skill invocation
- **Tasks** – Task management
- **ToolSearch** – Deferred tool lookup

### Team Coordination
- **SendMessage** – Team messaging
- **TeamCreate** – Spawn team
- **TeamDelete** – Teardown team
- **TeamTemplate** – Template management

### Scheduling
- **CronCreate** – Schedule job
- **CronDelete** – Cancel job
- **CronList** – List active crons

### Multi-Protocol
- **MCP** – Model Context Protocol
- **WebFetch** – HTTP fetch
- **WebSearch** – Search integration

### Other
- **AskUser** – Interactive prompts
- **Advisor** – Plan mode advisor (injected)
- **Plugins** – Dynamic plugin proxies

---

## Configuration Files

Located in `~/.claudio/`:

| File | Purpose |
|------|---------|
| **claudio.json** | User settings (model, providers, hooks, LSP, snippets, etc.) |
| **claudio.db** | SQLite database (sessions, messages, audit, FTS) |
| **hooks.json** | Lifecycle event handlers |
| **instincts.json** | Learned patterns |
| **memory/** | Persistent memory entries + index |
| **skills/** | Custom instruction files |
| **agents/** | Custom agent definitions |
| **plugins/** | Executable plugins |
| **teams/** | Saved team configurations |
| **cache/model-capabilities.json** | Model capabilities cache |
| **analytics/** | Token usage logs |
| **task-output/** | Background task output files |

---

## Common Patterns

### 1. Tool Invocation
```
registry.Get("Bash")
  → *tools.BashTool
  → tool.Execute(input)
  → tools.Result{Output, Error, ExitCode}
```

### 2. Sub-Agent Spawning
```
Agent tool.Execute()
  → agents.GetAgent(name)
  → Create new query.Engine with agent's system prompt
  → engine.Run(ctx, prompt)
  → Return response
```

### 3. Memory Extraction
```
Turn ends (end_turn stop reason)
  → engine.OnTurnEnd callback fires
  → memory.MemoryExtractor().Extract(messages)
  → Parse facts + concepts from assistant response
  → memory.Save(entry)
```

### 4. Team Coordination
```
Team lead creates team
  → teams.Manager.Create(config)
  → For each member: teams.TeammateRunner.Spawn(member)
    → Create sub-agent context with decorator
    → Inject SubAgentObserver for progress tracking
    → Run sub-agent.Run(ctx, prompt)
  → Poll mailbox each turn for member responses
```

### 5. Permission Evaluation
```
Tool use arrives
  → Extract content (cmd for Bash, path for Read/Write/Edit)
  → permissions.Match(toolName, content, rules)
  → Return behavior (auto/manual/deny)
  → If manual: prompt user
  → If denied: reject tool
```

### 6. Skill Resolution
```
Skill tool.Execute(name)
  → skillsRegistry.Get(name)
  → Append skill content to system prompt
  → Re-invoke LLM with expanded system prompt
  → Return LLM response as tool result
```

---

## Entry Points

| Command | Handler | Flow |
|---------|---------|------|
| `claudio prompt` | `runSinglePrompt()` | Single turn, print output |
| `claudio` (interactive) | `runInteractive()` | Multi-turn TUI |
| `claudio web` | Start web server | Browser-based UI |
| `claudio detect` | Detect project | Find git root |
| `claudio init` | Initialize ~/.claudio/ | Setup directories |

