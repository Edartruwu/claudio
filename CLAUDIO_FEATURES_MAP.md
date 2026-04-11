# CLAUDIO COMPLETE FEATURE MAP
## Comprehensive inventory for UX redesign

---

## EXECUTIVE SUMMARY

**Claudio** = Multi-agent AI IDE for terminal & web. User is the "AI pair programmer's pair programmer"—orchestrating agents, tools, workflows, and memory.

- **~50 features** across 6 subsystems
- **2 UIs**: Terminal (Bubble Tea TUI) + Web (Go templates + HTMX + SSE)
- **Multi-agent system** with teams, worktrees, plan approval gates
- **Tool-driven execution** with 38+ tools (read/write/shell/code-intel/web/agents/teams)
- **Persistent memory** across sessions + projects
- **Rich state management** (sessions, tasks, cron, permissions, hooks)

---

## 1. TERMINAL UI (TUI) — 25+ SCREENS

### Root Features
- **Main viewport** - Chat messages with expand/collapse (tool groups, thinking blocks)
- **Multi-window view** - Mirror/split pane toggle (`space+wv`)
- **Vim mode navigation** - Full vim keybindings in viewport + prompt
- **Panel navigation** - 8 side panels with `space+{c,f,m,s,t,o,a,K}` leader keys
- **Session switching** - `space+.` selector, `space+n/p` next/prev
- **Message operations** - Pin (`space+pin`), undo/redo (`/undo`, `/redo`), search (`/`)

### Data Panels (right edge, switchable)
| Panel | Hotkey | What it shows |
|-------|--------|---------------|
| Conversation | `space+c` | Read-only scrollable chat history |
| Files | `space+f` | Files read/modified in session (counts by operation) |
| Memory | `space+m` | Saved facts & project rules (2-tab: memories/rules) |
| Skills | `space+K` | Reusable instructions for agents; searchable, invokable |
| Tasks | `space+t` | Planning tasks (SQLite) + background task output streaming |
| Tools | `space+o` | Tool registry; toggle tools eager→deferred to save tokens |
| Analytics | `space+a` | Token usage, cache hits, USD cost, cache hit % |
| Config | `space+C` | Settings (project/global scope, toggleable, editable) |
| AGUI (Agents) | `space+oa` | Multi-agent inspector; list + detail conversation view |
| STREE (Sessions) | `space+os` | nvim-tree style session tree; expand/rename/delete |
| Team Panel | `space+ot` | Active teammates with status badges, pause/kill controls |

### Modal Dialogs
- **Command Palette** (`/` or `space+p`) - Search & execute/insert commands
- **Agent Picker** (`/agent`) - Persona injection (system prompt + model override)
- **Model Selector** (`space+m`) - Model, thinking mode (adaptive/enabled/disabled), budget picker
- **Team Selector** (`/team`) - Select template to activate ephemeral team
- **Session Picker** (`space+.`) - Search/resume/rename/delete sessions
- **File Picker** (`/attach`) - Fuzzy file selector for context
- **Permissions Dialog** - Tool approval gate (Allow/Deny/Always/Allow-all-tool) with y/n shortcuts
- **Which-Key Help** (`space` hold) - Grouped keybinding reference (auto-hide 300ms)

### Input & Inline Components
- **Prompt input** - Multi-line textarea (vim modes, history, paste collapsing, image attachments)
- **Todo dock** (above prompt) - Collapsible task list (`ctrl+t` toggle)
- **Toast notifications** - Auto-dismiss 1.5s alerts

### Sidebar Blocks (right, weighted layout)
- **Files block** - Quick file list with ✚(added), ✎(modified), ◐(read) icons
- **Tokens block** - Input/output/cache tokens, total USD cost
- **Todos block** - Active planning tasks snapshot

### Keybindings Summary
- **Navigation**: `j/k` (scroll), `G/g` (jump), `/` (search), `ctrl+d/u` (half-page)
- **Leaders**: `space` prefix (see which-key popup)
- **Vim**: Full insert/normal/visual modes in prompt
- **Window**: `space+w` + h/j/k/l (nav), `space+wv` (mirror), `space+q` (close), `space+b` (buffers)
- **Quick**: `space+p` (palette), `space+n/p` (session nav), `space+.` (sessions)

**Note**: Customizable in `~/.claudio/keybindings.json`

---

## 2. WEB UI — HTTP SERVER + SSE

### Architecture
- **Server**: Go `http.ServeMux` with token-based auth
- **Rendering**: Server-side templates (`templ` compiled to Go) + HTMX
- **Real-time**: Server-Sent Events (SSE) for live streaming
- **Database**: SQLite for sessions, messages, audit trail

### Core Routes & Features
| Route | Purpose |
|-------|---------|
| `/` | Home: project selector, session list, new session button |
| `/api/chat/send` | POST user message → spawns query engine |
| `/api/chat/stream` | GET SSE channel for streaming (text, thinking, tools, approvals) |
| `/api/chat/approve` | POST tool approval decision |
| `/api/picker/agents` | GET agent list (HTML partial) |
| `/api/picker/select-agent` | POST switch persona (inject system prompt) |
| `/api/sessions/*` | CRUD sessions (create, list, rename, delete) |
| `/api/commands/*` | Slash commands (/clear, /model, /compact, /thinking, etc.) |
| `/api/autocomplete/*` | Auto-complete for files, agents, commands |
| `/api/nav/*` | Sidebar: agents, teams, recently-used |
| `/static/*` | CSS, JS (embedded in binary) |

### SSE Event Stream (server → browser)
```
"text"             → Assistant streaming
"thinking"         → Thinking block content
"tool_start"       → {id, name, input}
"tool_end"         → {id, name, content, is_error}
"approval_needed"  → {id, name, input} (blocks until user acts)
"plan_approval"    → ExitPlanMode gate
"askuser_request"  → Structured questions dialog
"retry"            → {tool_ids} (tombstone incomplete renders)
"done"             → {tokens, cost} (turn complete)
"error"            → error message (turn failed)
```

---

## 3. CORE AGENT SYSTEM

### Agent Types (subagent_type options)
| Type | Role | Max Turns | Tools | Purpose |
|------|------|-----------|-------|---------|
| `general-purpose` | Multi-task executor | 50 | All | Research, execute, spawn sub-agents |
| `Explore` | Fast code explorer | 25 | All except Agent/Edit/Write/NotebookEdit | Find files, search code, answer architecture Qs |
| `Plan` | Software architect | 30 | Read-only tools only | Design approach, create plans, no execution |
| `verification` | Testing validator | 20 | All except Edit/Write/NotebookEdit | Run tests, lint, validate requirements |
| Custom agents | User-defined personalities | Configurable | Configurable | Loaded from markdown with YAML frontmatter |

### Agent Lifecycle
1. **Spawn**: User types `/agent <type>` or Agent tool invoked
2. **Context injection**: Max turns, model override, memory injection, team context
3. **Run**: Query engine executes streaming loop
4. **Tool execution**: Synchronous tool invocation with user approval gates
5. **Finish**: Turn ends (stop_reason or max_turns), session persisted, memory extracted async

### System Prompts
- Built-in agents have curated system prompts (guidelines, tool context, constraints)
- Custom agents override via markdown body as system prompt
- Persona injection via `AgentSelectedMsg` overrides session model + system prompt

### Model Defaults
- `general-purpose`: Session default (user-chosen)
- `Explore`: `haiku` (fast & cheap)
- `Plan`: Session default (architecture needs full model)
- `verification`: Session default (balanced reasoning)
- Custom: Override via `model: "opus"` in frontmatter

---

## 4. TOOL SYSTEM — 38+ TOOLS

### Tool Categories

#### File I/O (3)
- **Read** - Read file with offset/limit; supports images, PDFs (pageable), Jupyter notebooks
- **Write** - Create new files or overwrite completely (requires approval)
- **Edit** - Exact string replacement (diff-based); rejects files >1MB (requires approval)
- **Glob** - Find files by pattern (e.g., `**/*.go`, max 100 results, sorted by mod time)
- **Grep** - Regex search with ripgrep; multiline, context, file filtering, cached

#### Shell & Execution (2)
- **Bash** - Run commands with timeout (default 2min, max 10min), background support, output capture
- **LSP** - Language Server Protocol (goToDefinition, findReferences, hover, workspace search)

#### Web & Content (2)
- **WebSearch** - Internet search with snippets
- **WebFetch** - Fetch URL → markdown conversion (30s timeout, 100KB output max)

#### Notebooks (1)
- **NotebookEdit** - Edit Jupyter cells (insert/update/delete)

#### Memory & Tasks (6)
- **Memory** - Store/recall long-term facts with scopes (agent/project/global)
- **TaskCreate** - Create planning task with subject & description
- **TaskList** - List all tasks
- **TaskGet** - Get task details by ID
- **TaskUpdate** - Update task status (pending → in_progress → completed)
- **TaskOutput** - Get background task output (streaming, blocking/non-blocking)
- **TaskStop** - Kill background task by ID

#### Scheduling (3)
- **CronCreate** - Create recurring task (`@daily`, `@every 1h`, `14:30` syntax)
- **CronDelete** - Remove scheduled task
- **CronList** - List all cron entries with next run times

#### User Interaction (2)
- **AskUser** - Structured multi-question dialog with options
- **Skill** - Load & execute skill (instruction set) with optional argument interpolation

#### Planning & Workflow (2)
- **EnterPlanMode** - Start plan phase (read-only, returns instructions, plan file path)
- **ExitPlanMode** - Signal plan ready for user approval

#### Multi-Agent & Teams (6)
- **Agent** - Spawn sub-agent (subagent_type, max_turns, isolation="worktree", run_in_background)
- **SpawnTeammate** - Spawn named team member (auto-suffix duplicates, task linking)
- **SendMessage** - Inter-agent communication (to="*" for broadcast)
- **TeamCreate** - Create multi-agent team with shared config
- **TeamDelete** - Delete team and cleanup
- **SaveTeamTemplate** - Save team composition as reusable template
- **InstantiateTeam** - Create team from saved template

#### Git & Worktrees (2)
- **EnterWorktree** - Create isolated git worktree (auto-branch naming)
- **ExitWorktree** - Exit & cleanup worktree (keep/remove, discard changes)

#### Infrastructure (3)
- **ToolSearch** - Fetch deferred tool schemas on-demand (keyword search, direct select)
- **MCP** - Model Context Protocol proxy tools (from external servers)
- **AdvisorTool** - Consult strategic advisor (Opus) for plan review or execution audit

### Tool Execution Flow
1. **Validation**: Schema check, pre-execution checks
2. **Approval**: RequiresApproval check, TUI dialog if needed
3. **Execute**: `Tool.Execute(ctx, input)` → `Result{Content, IsError, Images}`
4. **Result handling**: Stream to user, update state, inject history

### Deferrable Tools
- Optional `DeferrableTool` interface saves tokens
- Initially send only name + search hint, full schema loaded on-demand via ToolSearch
- Auto-activate when service available (e.g., LSP when servers configured)

### Worktree Support
- Context keys: `ctxKeyCwd`, `ctxKeyMainRoot` for path remapping
- File tools operate inside worktree (isolated execution)
- Approval dialogs aware of worktree context

---

## 5. WORKFLOW SYSTEMS

### Plan Mode (5 phases)
1. **Enter**: User calls EnterPlanMode
2. **Explore**: Agent (read-only) examines codebase via Glob/Grep/Read/LSP
3. **Design**: Agent writes plan to `~/.claudio/plans/plan-{timestamp}.md`
4. **Submit**: Agent calls ExitPlanMode, TUI shows approval dialog
5. **Approve**: User greenlight or reject; blocks further execution until approval

**Key constraint**: Agent is read-only throughout (no commits, config changes, file writes)

### Planning Tasks (persistent, UI-visible)
- **Storage**: SQLite with sequential IDs (1, 2, 3, ...)
- **Fields**: Status (○pending, ◐in-progress, ●done), Subject, Description, Owner, BlockedBy dependencies
- **Creation**: Via `TaskCreate` tool or `/task` command
- **UI**: List in Tasks panel with live status spinners, dependencies shown
- **Reuse**: Same task tracked across multiple sessions

### Background Tasks (ephemeral, in-memory)
- **Storage**: Memory + file-based output (up to 5GB cap)
- **Type-prefixed IDs**: `a1` (agent), `b2` (background), `d3` (dream)
- **Lifecycle**: Spawned via sub-agent launch, output streamed to file, auto-cleaned on session end
- **Output viewing**: Via TaskOutput tool (blocking or non-blocking, 30s default timeout)
- **Cancellation**: TaskStop tool or TUI x button on Team Panel

### Cron Jobs (recurring, persistent)
- **Syntax**: `@daily`, `@hourly`, `@every {duration}`, or `HH:MM` (e.g., `14:30`)
- **Storage**: JSON file (not database) with CronEntry records
- **Execution**: External polling triggers due entries, SpawnAgentTask spawns background agent
- **UI**: CronList shows next run times; CronCreate/Delete require approval dialogs

### Teams (multi-agent orchestration)
- **Formation**: User = team-lead, spawned agents = members with suffix naming
- **Config**: Shared model defaults, filesystem paths, auto-compact threshold
- **Spawning**: `SpawnTeammate` with optional `isolation="worktree"` for git-aware parallel work
- **Coordination**: File-based mailboxes (inboxes/{agent_name}.json) for inter-agent messaging
- **Revival**: Agent finishing with `QUESTION: {question}` goes idle; SendMessage answer triggers revival (not re-spawn), preserving history
- **Worktree isolation**: Each agent gets .claudio-worktrees/{team_name}/{agent_id}/ with context-aware path remapping
- **Persistence**: Team config at ~/.claudio/teams/{team_name}/config.json; templates at ~/.claudio/team-templates/

### Sessions (conversation scopes)
- **Hierarchy**: Parent-child relationships (top-level = main conversation, sub-sessions = agents)
- **Storage**: SQLite with UUID, title, agent type, full message history per session
- **Persistence**: After session end, messages replaced with Summary string via compaction
- **Memory attachment**: Agents with persistent memory get MemoryDir for learned patterns
- **Consolidation**: Dreams run background tasks to consolidate memories (async, separate process)

---

## 6. STATE MANAGEMENT SYSTEMS

### Memory (persistent knowledge)
- **Scopes**: Agent > Project > Global (ScopedStore precedence)
- **Storage**: Markdown files in ~/.claudio/memory/, .claudio/memory/, <agent>/memory/
- **Format**: MEMORY.md index file + individual .md entries with type tags
- **Features**: FindRelevant (keyword), SelectRelevant (AI/Haiku-based filtering), 25KB cap per entry
- **Types**: user, feedback, project, reference (user-defined)
- **Access**: Memory tool (Store/Recall/Forget/Summarize)

### Config (settings & defaults)
- **Scopes** (precedence): Env vars > Project (.claudio/settings.json) > Local (~/.claudio/local-settings.json) > User (~/.claudio/settings.json) > Defaults
- **Format**: JSON with 20+ fields (Model, BudgetTokens, PermissionMode, AutoMemoryExtract, etc.)
- **Editable via**: Config panel in TUI, direct JSON editing, CLI flags
- **Trust system**: Project configs require trust acceptance (embedded in trusted.json)
- **MCP/LSP servers**: Configured in settings with per-server config

### Permissions (tool use approval)
- **Model**: Pattern-based rules matching (tool + glob pattern → behavior)
- **Rule types**: PermissionRule{Tool, Pattern, Behavior}
- **Behaviors**: allow, deny, ask (interactive approval)
- **Patterns**: glob, directory, command prefix, exact match
- **Storage**: Embedded in settings.json
- **Enforcement**: Per-tool approval dialogs, patterns checked on tool execution

### Hooks (event-driven scripts)
- **Event types** (15+): PreToolUse, PostToolUse, SessionStart/End, PreCompact, PostCompact, SubagentStart/Stop, TaskCreated/Completed, WorktreeCreate, FileChanged, Notification
- **Triggers**: Tool lifecycle, session events, conversation state, file system
- **Features**: Context-aware (env vars available), timeout (default 10s), async support, blocking (exit 1)
- **Storage**: Nested in settings.json HookDef array
- **Use case**: Custom logging, notifications, auto-actions (e.g., auto-approve certain tools, post-session cleanup)

### Keybindings (customizable)
- **Format**: JSON array (keys → action → context)
- **Scopes**: Context-aware (viewport, prompt, normal, global)
- **Features**: 33+ built-in actions, leader key (space), vim-inspired (j/k, g/G)
- **Reserved**: ctrl+c, esc (cannot rebind)
- **Storage**: ~/.claudio/keybindings.json
- **Override**: Per-user customization without code changes

### Storage (persistence layer)
- **Database**: SQLite with WAL mode (write-ahead logging)
- **Schema**: sessions, messages, events, audit_log, instincts, team_tasks, filter_savings
- **Location**: ~/.claudio/claudio.db (+ -wal, -shm journals)
- **Features**: Session hierarchy, tool audit trail (tool name, input/output, tokens, duration), cascade deletion
- **Migrations**: 19+ versions tracked for schema evolution

---

## 7. SERVICES (Support Libraries)

| Service | Purpose |
|---------|---------|
| **analytics** | Token usage, costs per model, budget enforcement, session cost report |
| **cachetracker** | Prompt cache observability (hit/miss), cache break detection |
| **compact** | Context compression (auto/manual/strategic), token budget, message replacement |
| **difftracker** | File change tracking per turn, git diff capture before/after |
| **filtersavings** | Output filter analytics, byte savings discovery, command recommendations |
| **lsp** | Language Server Protocol lifecycle (gopls, clangd, pyright), code intelligence |
| **mcp** | Model Context Protocol servers, tool registration, idle cleanup |
| **memory** | Persistent knowledge store, markdown-based facts, type tagging |
| **naming** | Session title generation (AI-driven, 2-5 word titles) |
| **notifications** | Desktop/terminal alerts (macOS AppleScript, Linux notify-send) |
| **skills** | Skill registry, loading, matching, source tracking |
| **toolcache** | Large result offloading (>50KB → disk), token savings |

---

## 8. SKILLS SYSTEM

### What is a Skill?
- Reusable instruction set (markdown) with optional YAML frontmatter
- Supports shell interpolation (`!`cmd\``) for live context (git status, etc.)
- Matched to tasks by skill system

### Where Are They Stored?
- **Bundled**: Internal/skills/ (shipped with claudio binary)
- **User**: ~/.claudio/skills/ (custom user skills)
- **Project**: .claudio/skills/ (project-specific domain skills)

### Custom Skills
- Users create .md files with YAML metadata (optional)
- Can include shell commands for dynamic content
- Invoked via `/skill <name>` command or Skill tool in agent workflow

### Built-In Skill Examples
- `/commit` - Conventional commit formatting guide
- `/review` - Structured code review checklist
- `/test` - Test writing strategy
- `/refactor` - Code quality & refactoring patterns
- Custom skill templates for domain-specific tasks

---

## 9. PLUGINS SYSTEM

### Plugin Discovery
- Auto-discovered from ~/.claudio/plugins/ (executables)
- Wrapped as PluginProxyTool instances in tool registry

### Plugin Features
- **Metadata**: Expose via `--describe` flag (name, description, input schema)
- **Schema**: JSON Schema generation via `--schema` flag
- **LSP support**: *.lsp.json configs contribute language servers
- **Integration**: Plugins callable like built-in tools (same execution flow)

### Custom Plugins
- Users write shell scripts or executables
- Claudio auto-registers and calls them with JSON input
- Tool approval & hooks apply to plugins same as built-ins

---

## 10. QUERY ENGINE (Core Runtime)

### Execution Model
1. **Input phase**: Inject user message, system context, memory, git status
2. **API call**: Stream Claude response with token tracking
3. **Tool execution**: Synchronous tool invocation, collect results
4. **Turn completion**: Update analytics, record diffs, extract memories (background task)
5. **Loop**: Continue until stop_reason or max_turns exceeded

### EventHandler Interface
- `OnTextDelta(text)` - Stream text
- `OnThinkingDelta(text)` - Stream thinking blocks
- `OnToolUseStart(tool)` - Tool invocation began
- `OnToolUseEnd(tool, result)` - Tool result arrived
- `OnToolApprovalNeeded(tool)` - Blocks until user approves/denies
- `OnTurnComplete(usage)` - Session ends, stats collected
- `OnError(err)` - Session failed

### Implementation Details
- **Web**: SSE event stream to browser
- **CLI/TUI**: Direct callback handlers for TUI state updates
- **Streaming**: Text, thinking, tool calls, tool results interleaved
- **Synchronous tools**: Block turn progression until complete

---

## 11. ORCHESTRATOR

Beyond agent spawning, coordinates multi-agent workflows:
- **Phases**: research → plan → implement → verify (dependency-aware execution)
- **Results tracking**: Collects output from each phase
- **Sequential/parallel**: Phases respect DependsOn rules
- **Output formatting**: Aggregates results for user review

---

## 12. SECURITY & TRUST

### Trust Verification
- **Project configs** require explicit trust acceptance
- **Trust store**: trusted.json tracks accepted projects
- **Prompts**: User warned if project has hooks or MCP servers

### Permission Enforcement
- **Tool approval**: Interactive dialogs for RequiresApproval tools
- **Denied tools**: DisallowedTools list per agent blocks certain capabilities
- **Content filtering**: Output filters strip sensitive data (secrets, PII)

### Secret Masking
- `SanitizeForLog` utility masks tokens, passwords, API keys
- Hooks and logs redact sensitive environment variables

---

## 13. QUICK FEATURES REFERENCE

### Commands (slash commands in prompt)
- `/agent <type>` - Switch or spawn sub-agent
- `/thinking {adaptive|enabled|disabled}` - Control thinking mode
- `/model <model>` - Override session model
- `/compact` - Compress context manually
- `/clear` - Clear session history
- `/task <subject>` - Create planning task
- `/cron <schedule>` - Create cron job
- `/team <name>` - Create/switch team
- `/skill <name>` - Invoke skill
- `/attach <file>` - Add context file (via file picker)
- `/undo` / `/redo` - Undo/redo turns
- `/memory` - Manage memory (open panel)

### Keyboard Shortcuts (Vim-aware)
- `space+<key>` - Leader commands (see which-key popup)
- `j/k` - Scroll (vim)
- `G/g` - Jump to end/start (vim)
- `e` - Expand tool group
- `v` - Expand thinking block
- `Ctrl+Enter` - Submit message
- `Ctrl+Z` - Undo input
- Tab - Cycle between UI components

### Configuration Knobs
- `model` - Default LLM (haiku, sonnet, opus)
- `budgetTokens` - Input context token limit
- `permissionMode` - ask|allow|deny for tool use
- `autoMemoryExtract` - Auto extract memories on session end
- `memorySelection` - AI-based or simple keyword selection
- `outputStyle` - Console formatting (markdown, plain, etc.)
- `hookProfile` - Enabled hooks subset
- `mcpServers` - External tools via MCP
- `lspServers` - Language server configs per language

---

## 14. KEY UX FLOWS

### Flow 1: Start a Session
1. Run `claudio` in terminal or visit web UI
2. Welcome screen shows recent sessions + hints
3. Type message or resume recent session
4. TUI/web initializes, shows prompt + viewport
5. Type message, press Ctrl+Enter
6. Streaming response appears in viewport

### Flow 2: Spawn Sub-Agent
1. Type `/agent <type>` in prompt
2. Agent picker modal (or auto if type specified)
3. Select agent (Explore for code, Plan for design, etc.)
4. User message becomes agent's task
5. Agent runs independently (status in AGUI panel)
6. Results streamed back into session message history

### Flow 3: Plan Mode
1. Type `/plan` or EnterPlanMode tool
2. Agent goes read-only (Explore tool subset only)
3. Agent explores codebase, proposes approach
4. Agent writes plan to ~/.claudio/plans/plan-*.md
5. ExitPlanMode signals completion
6. TUI shows "Approve Plan?" dialog
7. User reviews and approves/rejects
8. Approval stored, main agent resumes with plan context

### Flow 4: Team Workflow
1. Type `/team <name>` to create team
2. User becomes team-lead
3. Use `/agent <type> <task>` to spawn teammate (in background by default)
4. Team members shown in Team Panel with status badges
5. Use SendMessage to communicate with members
6. Members can RevIVE if they end with QUESTION:
7. Type `/team-save-template` to save composition for reuse

### Flow 5: Manage Tasks
1. Type `/task <subject>` to create planning task
2. Task appears in Tasks panel + task list
3. Agent can TaskCreate/TaskUpdate to assign/update
4. Background tasks streamed to Task panel with live output
5. TaskStop kills background tasks if needed
6. Planning tasks persist across sessions

### Flow 6: Permission Approval
1. Agent calls tool that requires approval
2. TUI/web shows approval dialog (Tool name, input, 4 buttons)
3. User selects: Allow (once) | Deny | Always allow (this tool) | Allow all (this input)
4. Dialog closes, tool executes (or fails if denied)
5. Decision stored in permission cache (no re-prompt for same tool+input)

### Flow 7: Session Context Sharing
1. Chat in main session
2. Type `/agent Explore "analyze src/types.go"`
3. Sub-agent spawned with inherited context:
   - Parent session history (for context)
   - Codebase access (via tools)
   - Memory from parent project
   - Model from session (or override)
4. Sub-agent completes, returns results as message
5. Planning tasks created by sub-agent visible in main session

---

## 15. ARCHITECTURE PATTERNS

### Dependency Injection via Context
```go
ctx = tools.WithMaxTurns(ctx, 50)           // limits agent turns
ctx = tools.WithAgentType(ctx, "Explore")   // labels execution
ctx = tools.WithSubAgentDB(ctx, db, ...)    // enables persistence
```

### Service Lifecycle
- **Stateless**: Memory, Analytics, Cachetracker (fire-and-forget)
- **Stateful**: LSP, MCP (daemon-like service managers)
- **Async**: Memory extraction, dream consolidation (background tasks)

### Event-Driven Hooks
```
Tool execution → PreToolUse hook → Tool runs → PostToolUse hook → Result streams to user
Session ends → SessionEnd hook → Memory extraction task spawned → Async completion
```

### Multi-UI Support
- **Same backend** (query engine) serves both TUI and web
- **EventHandler abstraction**: TUI has tea.Cmd handlers, web has SSE event stream
- **Tool approval**: TUI uses modal dialogs, web uses SSE blocking event + form submission
- **No code duplication**: Business logic in shared query engine

---

## 16. NOTABLE CONSTRAINTS & TRADEOFFS

### Read-Only vs. Modification
- **Read-only agents** (Explore, Plan) cannot Edit/Write/NotebookEdit
- **Enforced at tool level** (DisallowedTools), not a separate privilege system
- **Implication**: Fine-grained control per agent type, but no central authorization layer

### Worktree Isolation
- **Optional**: `Agent` tool with `isolation="worktree"` creates separate git worktree
- **Path remapping**: File tools aware of worktree CWD, transparent path handling
- **Cleanup**: Manual via ExitWorktree (no auto-cleanup, risk of stale worktrees)

### Token Budget
- **Soft limit**: BudgetTokens config sets input context cap
- **Hard limit**: max_tokens from Claude API (~200K)
- **Compaction**: Auto-triggered at 95% usage or 5min idle, not a strict enforcement

### Cron Scheduling
- **No built-in loop**: External code must poll CronStore.Due() and execute
- **Implies**: Cron jobs require background process (daemon) or explicit polling
- **Limitation**: No guaranteed execution timing (polling frequency-dependent)

### Agent Revival
- **QUESTION: syntax**: Agents finishing with `QUESTION:` go idle (not dead)
- **SendMessage trigger**: Only method to revive (no automatic retry)
- **Implication**: Multi-turn agent workflows require explicit message passing

---

## 17. MISSING FEATURES (Non-Existent)

- ❌ **GraphQL API** - Only REST+SSE
- ❌ **Mobile UI** - Terminal & web only
- ❌ **Database migrations UI** - Manual schema management
- ❌ **Scheduled cron daemon** - External polling required
- ❌ **Collaborative editing** - Single-user only
- ❌ **Voice input** - Text only
- ❌ **Real-time collaboration** - No multi-user sessions
- ❌ **Built-in debugger** - Uses LSP for code navigation only

---

## 18. PERFORMANCE CHARACTERISTICS

### Fast Path
- **Message streaming**: SSE/tea.Cmd real-time feedback
- **Deferred tools**: Send only names/hints, full schemas on-demand (token savings)
- **Output filtering**: Strip colors/formatting before streaming (bandwidth savings)
- **Token budget**: Compaction at 95% usage prevents runaway context growth

### Slow Path
- **Memory extraction**: Async background task after session (may take seconds)
- **Dream consolidation**: Separate process, user sees "consolidating..." status
- **Cron execution**: Depends on external polling loop (not real-time)
- **MCP server startup**: Can be slow if many servers configured

---

## SUMMARY: FEATURE MATRIX

| Category | Count | Key Examples |
|----------|-------|--------------|
| **TUI Screens** | 25+ | root, 8 panels, 8 modals, 3 sidebar blocks, prompt, dock |
| **Web Routes** | 15+ | /api/chat/*, /api/picker/*, /api/sessions/*, /api/commands/* |
| **Tools** | 38+ | Read, Write, Edit, Bash, Glob, Grep, LSP, Agent, Task*, Memory, Cron*, Teams*, Skills, Plugins, MCP |
| **Agent Types** | 4+N | general-purpose, Explore, Plan, verification + custom |
| **Agents per Team** | Unlimited | SpawnTeammate batching, worktree isolation per agent |
| **Services** | 12 | Analytics, Memory, Skills, LSP, MCP, Notifications, Compact, DiffTracker, etc. |
| **Keybindings** | 33+ | Customizable, vim-aware, leader-key driven |
| **Config Knobs** | 20+ | Model, budget, permissions, hooks, MCP/LSP servers |
| **Event Types** | 15+ | Tool, session, workspace, file, notification hooks |
| **Databases** | 2 | SQLite (sessions, messages) + Markdown (memory) |

---

## FINAL TAKEAWAYS FOR UX REDESIGN

1. **Two-UI strategy works**: TUI for power users, web for browser convenience. Single backend (query engine) eliminates duplication.

2. **Panel-based architecture**: 8 interchangeable data panels (Conversation, Files, Memory, Skills, Tasks, Tools, Analytics, Config) + dynamic AGUI/STREE/Team panels = flexible information density.

3. **Modal workflows are explicit**: Plan mode, agent picker, model selector, approval gates = user sees decision points, no hidden automations.

4. **Permission model is pattern-based**: Content-aware rules (tool + glob → behavior), not role-based. Enables fine-grained safety without complex privilege hierarchy.

5. **Multi-agent is first-class**: Teams, worktrees, inter-agent messaging, memory isolation = collaborative AI workflows, not just single-agent chat.

6. **Persistent planning**: Planning tasks outlive sessions. Cron jobs persist. Teams persist. UX should emphasize "long-running projects" not just "one conversation".

7. **Streaming > async results**: SSE (web) and tea.Cmd (TUI) for real-time feedback. Users see tool execution live, not post-hoc.

8. **Skills = knowledge reuse**: Skills are markdown templates, not code. Users can add custom domain knowledge without programming.

9. **Hooks = extensibility**: 15+ event types (tool use, session lifecycle, file changes) enable custom automation without plugin code.

10. **Read-only safe exploration**: Explore/Plan agents can't modify code, enabling AI-driven code review and planning without fear of accidental damage.

---

**END OF FEATURE MAP**
