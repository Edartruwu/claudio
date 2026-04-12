# User Stories: Tools, Memory, Permissions, Config, Web UI

**Version:** 1.0  
**Domain:** Terminal UI (TUI) & Web UI  
**Numbering:** UH-200 → UH-260+  
**Status:** Draft — Implementation Ready

---

## TOOLS PANEL (space+o)

### UH-200: View active tool registry
**As a** developer  
**I want** to see all available tools (deferred + eager) in a searchable list  
**So that** I can understand what capabilities are available and manage token impact

**Acceptance Criteria:**
- [ ] Tools panel (space+o) displays 38+ tools sorted by category (File I/O, Shell, Web, Memory, etc.)
- [ ] Each row shows: tool name, eager/deferred status, brief description, token impact estimate
- [ ] Search bar filters tools by keyword (e.g., "read", "agent", "web")
- [ ] Deferred tools marked with `[deferred]` badge in `info` color
- [ ] Cursor can navigate rows; enter expands tool detail (full docs, schema preview)

**Design Notes:** List item (§5.2), search input styled like prompt, tool row anatomy similar to task items (§5.13)  
**Priority:** P1

---

### UH-201: Toggle tool eagerness to save tokens
**As a** power user  
**I want** to toggle deferred tools between eager and deferred loading  
**So that** I can reduce initial token footprint for expensive tool schemas

**Acceptance Criteria:**
- [ ] Focused tool row shows action hint: `t:toggle eagerness`
- [ ] Pressing `t` toggles `eager` ↔ `deferred` for selected tool
- [ ] Change persists in memory for session (not saved to config by default)
- [ ] Toggle affects **next** tool invocation; current turn unaffected
- [ ] Success toast confirms change (1.5s auto-dismiss)

**Design Notes:** Status badge (§5.5) updates immediately; toggle state reflected in row prefix  
**Priority:** P2

---

### UH-202: View tool execution result in detail
**As a** developer  
**I want** to expand any tool call in history and see full input/output  
**So that** I can debug tool behavior or review exact data passed

**Acceptance Criteria:**
- [ ] Tool call row (§5.14) in conversation shows collapsed preview
- [ ] Pressing `e` (or enter) expands tool row to show: name, input (JSON), output (truncated + scrollable), duration
- [ ] Input and output syntax-highlighted (JSON for config, markdown for text)
- [ ] Collapse via `esc` or `c` key
- [ ] Token count shown in expanded view (`input_tokens`, `output_tokens`)

**Design Notes:** Expand/collapse uses `e` key per design system (§6). Input styled `info`, output `fg3`  
**Priority:** P2

---

### UH-203: View token impact per tool call
**As a** engineer  
**I want** to see token breakdown for each tool call (input + output + cache)  
**So that** I can identify expensive operations and optimize session context

**Acceptance Criteria:**
- [ ] Tool call row shows inline token count: `15→82 tokens` (input→output)
- [ ] Expanded view includes cache hit indicator: `[cache hit: 3 out of 4 attempts]`
- [ ] If cache miss, shows reason (token count changed, context reset, etc.)
- [ ] "Top expensive tools" summary in Analytics panel (space+a)

**Design Notes:** Token counts rendered `muted` `fg4` per typography (§3.3). Cache badge styled `success`  
**Priority:** P2

---

## TOOL CALL DISPLAY (Chat)

### UH-204: Render tool calls with collapsible groups
**As a** user  
**I want** tool calls grouped by phase (thinking → calls → results) and collapsible  
**So that** I can focus on text output and expand tool details only when needed

**Acceptance Criteria:**
- [ ] Tool call row (§5.14) rendered inline in conversation between text blocks
- [ ] Multiple consecutive tool calls grouped under collapse header: `⚡ 3 tool calls` (collapsed default)
- [ ] Expand via `e` key shows: name, status, brief result (truncated)
- [ ] Icon colors: `⚡` warning (in-progress), `✓` success (done), `✗` error (failed)
- [ ] Each tool in group shows connectors (`├─`, `└─`) for tree appearance

**Design Notes:** Tool row component (§5.14), status badges (§5.5), collapse triggered by `e` key  
**Priority:** P1

---

### UH-205: Approve tool execution before proceeding
**As a** security-conscious user  
**I want** a permission dialog to appear for tools requiring approval  
**So that** I can control risky operations (file writes, shell commands)

**Acceptance Criteria:**
- [ ] Modal dialog appears with: tool name, file path / command snippet, 4 buttons
- [ ] Buttons: `[Allow]` `[Deny]` `[Always Allow]` `[Allow All Tools This Turn]`
- [ ] Keyboard shortcuts: `a` (Allow), `d` (Deny), `!` (Always), `A` (All)
- [ ] Dialog border colored `warning` (#fabd2f) to indicate caution
- [ ] Denying blocks tool, returns error message to conversation

**Design Notes:** Modal (§5.9), button styling (§5.4), dialog context shows in status bar  
**Priority:** P1

---

## PERMISSIONS SYSTEM

### UH-206: Create permission rules via dialog
**As a** project lead  
**I want** to define permission rules (tool + glob pattern → allow/deny/ask)  
**So that** I can auto-approve safe operations and block dangerous ones

**Acceptance Criteria:**
- [ ] Config panel (space+C) has "Permissions" section with "Add Rule" button
- [ ] Dialog prompts: tool name (autocomplete from 38+), pattern (glob or `*`), behavior (allow/deny/ask)
- [ ] Pattern examples shown: `src/**/*.go`, `/tmp/*`, `*` (any file)
- [ ] Rule persists to project `.claudio/settings.json` under `permissions` array
- [ ] Rules display as: `[tool] [pattern] → [behavior]` in list

**Design Notes:** Modal with input fields (§5.3), button actions (§5.4), list items (§5.2)  
**Priority:** P2

---

### UH-207: Review and delete permission rules
**As a** administrator  
**I want** to list, review, and delete existing permission rules  
**So that** I can maintain security policy as team needs change

**Acceptance Criteria:**
- [ ] Permissions section lists rules with: tool name, pattern, behavior, "delete" option
- [ ] Focused rule highlights and shows full regex if complex
- [ ] Pressing `d` deletes rule after `(y/n)` confirmation
- [ ] Changes save immediately to settings.json
- [ ] Empty state shows: `No permission rules` (allows all by permissionMode default)

**Design Notes:** List items (§5.2), confirmation prompt styled `error` border, empty state (§6.4)  
**Priority:** P2

---

### UH-208: View permission mode (allow/ask/deny default)
**As a** user  
**I want** to see and change the global permission mode  
**So that** I can switch between strict (ask all), permissive (allow all), or paranoid (deny all)

**Acceptance Criteria:**
- [ ] Config panel shows: `Permission Mode: [ask | allow | deny]` as selector
- [ ] Pressing `tab` or `↓` cycles through modes
- [ ] Selected mode highlighted in `primary`
- [ ] Tooltip explains behavior: "ask" → per-tool dialog, "allow" → auto-approve, "deny" → block all
- [ ] Change persists to `.claudio/settings.json` (not just session)

**Design Notes:** Inline selector styled as toggle, status bar hints describe modes  
**Priority:** P2

---

## HOOKS SYSTEM

### UH-209: View configured hooks and event types
**As a** engineer  
**I want** to see all configured hooks grouped by event type (PreToolUse, PostToolUse, SessionEnd, etc.)  
**So that** I can understand automation in the session

**Acceptance Criteria:**
- [ ] Config panel has "Hooks" section with collapsible groups by event type
- [ ] Each group shows: event name, number of hooks, examples (first 2-3)
- [ ] Collapse header: `▸ PreToolUse (2 hooks)` expands to list
- [ ] Hook rows show: command (truncated), description, enabled/disabled toggle
- [ ] Click to expand shows: full command, env vars available, timeout

**Design Notes:** Collapsible sections (toggle prefix `▸`), list items (§5.2), status badges (§5.5)  
**Priority:** P2

---

### UH-210: Enable/disable hooks without editing config
**As a** user  
**I want** to toggle hooks on/off from the Config panel  
**So that** I can experiment with automation without modifying files

**Acceptance Criteria:**
- [ ] Hook row shows inline toggle: `[enabled]` or `[disabled]` in `success`/`muted`
- [ ] Pressing `t` on focused hook toggles state
- [ ] Change applies immediately to session (no restart needed)
- [ ] Toast confirms: `Hook 'post-build' disabled`
- [ ] Disabled hooks NOT saved to config (session-only change)

**Design Notes:** Status badge toggle (§5.5), toast (§5.15), `t` key per design  
**Priority:** P3

---

## MEMORY PANEL (space+m)

### UH-211: View scoped memory (agent, project, global)
**As a** agent orchestrator  
**I want** to see memory entries organized by scope (agent > project > global)  
**So that** I can understand context available to the current session

**Acceptance Criteria:**
- [ ] Memory panel opens with 2 tabs: `1 Memories  2 Rules`
- [ ] Memories tab shows entries grouped by scope with headers: `Agent Memory`, `Project Memory`, `Global Memory`
- [ ] Each entry shows: type badge (`user`, `feedback`, `project`, `reference`), title, preview (first 50 chars), size (e.g., `1.2 KB`)
- [ ] Scope precedence shown in status: "Agent overrides project overrides global"
- [ ] Empty scopes omitted (e.g., if no agent memory, skip that header)

**Design Notes:** Tab bar (§5.10), status badge (§5.5), list items (§5.2), panel (§5.1)  
**Priority:** P1

---

### UH-212: Search and filter memories
**As a** user  
**I want** to search memories by keyword or type  
**So that** I can quickly find relevant context without scrolling

**Acceptance Criteria:**
- [ ] Search bar at top of Memories tab; placeholder: `search memories by keyword...`
- [ ] Typing filters entries in real-time (case-insensitive)
- [ ] Filter by type via buttons below search: `all  user  feedback  project  reference`
- [ ] Focused button highlighted in `primary`; inactive in `muted`
- [ ] Results show match count: `3 of 12 memories`
- [ ] `esc` clears search

**Design Notes:** Input field (§5.3), button states (§5.4), search pattern from command palette  
**Priority:** P2

---

### UH-213: View memory entry details and edit
**As a** user  
**I want** to expand a memory entry to see full content and edit it  
**So that** I can refine facts or add notes

**Acceptance Criteria:**
- [ ] Pressing `enter` or `e` on memory row expands to full-screen overlay
- [ ] Overlay shows: title, type badge, scope, full content (scrollable)
- [ ] Edit button at bottom: `[Edit]  [Delete]  [Copy]`
- [ ] Edit opens text editor (prompt-like textarea), allows in-place modification
- [ ] Save button commits to memory store; cancel discards changes
- [ ] Memory size cap enforced (25 KB per entry per design)

**Design Notes:** Overlay modal (§5.9), textarea input (§5.3), button actions (§5.4)  
**Priority:** P2

---

### UH-214: Create new memory entry
**As a** user  
**I want** to manually add a fact or project rule to memory  
**So that** I can document learnings without agent involvement

**Acceptance Criteria:**
- [ ] Memory panel shows `[+ New]` button above entry list
- [ ] Pressing `+` opens dialog: title input, type selector, scope selector (agent/project/global), content textarea
- [ ] Scope dropdown shows: current agent, "Project", "Global"; default to Project
- [ ] Content textarea has `vim` keybindings; ctrl+s saves, esc cancels
- [ ] Saves to `.claudio/memory/MEMORY.md` + individual `*.md` entry
- [ ] Success toast confirms and returns to list

**Design Notes:** Modal dialog (§5.9), input fields (§5.3), textarea (§5.3)  
**Priority:** P2

---

### UH-215: View and manage memory rules
**As a** project lead  
**I want** to define and edit memory extraction rules  
**So that** I can control which facts are automatically learned from sessions

**Acceptance Criteria:**
- [ ] Rules tab in Memory panel shows rules list (each rule = one row)
- [ ] Rule row shows: pattern (e.g., "extract errors"), scope, enabled/disabled toggle
- [ ] Expand rule to see: full regex pattern, action (store/ignore), description
- [ ] Edit button in expanded view opens rule editor
- [ ] Pressing `d` deletes rule (no confirmation, undo via `/undo` only)
- [ ] `[+ New Rule]` adds rule via dialog

**Design Notes:** Tab bar (§5.10), list items (§5.2), expand/collapse (§5.1)  
**Priority:** P3

---

## SKILLS PANEL (space+K)

### UH-216: View skill registry with search
**As a** developer  
**I want** to browse all available skills (built-in + custom) with search  
**So that** I can discover and invoke domain-specific instructions

**Acceptance Criteria:**
- [ ] Skills panel displays skills grouped by source: `Bundled`, `Project`, `User`
- [ ] Each skill row shows: icon (📜), name, brief description (1 line), `[invoke]` button
- [ ] Search bar filters by name or description keyword
- [ ] Focused skill highlights; pressing `i` or `enter` invokes (or shows args prompt)
- [ ] Skill count shown: `12 bundled · 3 project · 1 user`

**Design Notes:** List items (§5.2), search input (§5.3), section headers per group, button actions  
**Priority:** P2

---

### UH-217: Invoke skill with optional arguments
**As a** user  
**I want** to invoke a skill from the panel, optionally passing arguments  
**So that** I can inject reusable instructions into the current turn

**Acceptance Criteria:**
- [ ] Pressing `i` on focused skill prompts for arguments (if skill expects them)
- [ ] Argument prompt shows placeholder: "enter arguments (or leave blank)"
- [ ] Submitting inserts skill content + arguments into prompt as `/skill <name> <args>`
- [ ] If skill has no template variables, inserts directly without prompt
- [ ] Skill content (markdown + shell interpolations) injected above prompt
- [ ] Status bar shows: `Skill '<name>' ready to send`

**Design Notes:** Modal input (§5.3), prompt state updated, status bar (§5.8)  
**Priority:** P2

---

### UH-218: View skill source and edit custom skills
**As a** developer  
**I want** to view the markdown source of a skill and edit custom ones  
**So that** I can understand the instruction or modify for my use case

**Acceptance Criteria:**
- [ ] Expanding skill row shows: source file path, last modified date, word count
- [ ] Custom skills (project + user) show `[Edit]` button; bundled skills show `[View]` button only
- [ ] Pressing `v` opens read-only overlay for bundled; edit mode for custom
- [ ] Edit mode allows in-place modification; `ctrl+s` saves to disk
- [ ] YAML frontmatter (metadata) shown at top; custom vars listed
- [ ] Save triggers reload; skill becomes available immediately

**Design Notes:** Overlay modal (§5.9), syntax highlighting for markdown/YAML, textarea (§5.3)  
**Priority:** P3

---

## ANALYTICS PANEL (space+a)

### UH-219: View session token usage summary
**As a** cost-conscious user  
**I want** to see total tokens used in the current session (input, output, cache)  
**So that** I can understand cost and stay within budget

**Acceptance Criteria:**
- [ ] Analytics panel top section shows: `12.3k / 100k` (input / budget), cost in USD `$0.04`, cache hit % `78%`
- [ ] Breakdown row below: `Input: 8.2k  Output: 4.1k  Cache Hit: 3.2k`
- [ ] Per-model breakdown: `sonnet: 8.1k ($0.03)  haiku: 4.2k ($0.01)`
- [ ] Token bar (progress bar) shows usage vs budget; color shifts from `success` → `warning` → `error` at 75%/95%
- [ ] Hover/expand shows: tokens per turn, compaction events triggered

**Design Notes:** Status bar (§5.8), progress indicator (§5.6), color semantic (§2.4)  
**Priority:** P1

---

### UH-220: View cache hit details
**As a** engineer  
**I want** to see which tool calls benefited from cache  
**So that** I can understand prompt caching effectiveness

**Acceptance Criteria:**
- [ ] Analytics panel has section: `Prompt Cache`
- [ ] Shows total cache hits, cache size (KB), cache creation time (e.g., "created 2h ago")
- [ ] List below: tool call row + `[cache hit 94% match]` indicator
- [ ] Expand to see: full prompt sent, cache hash, tokens saved by this hit
- [ ] Cache reset indicator (if cache invalidated): `✗ cache reset 3 times (context change)`
- [ ] Tooltip explains: "Cache stays valid until context changes (new memory, new file, etc.)"

**Design Notes:** Status badge (§5.5), progress indicator (§5.6), tool row (§5.14)  
**Priority:** P2

---

### UH-221: View cost breakdown by model and date
**As a** project lead  
**I want** to see cumulative cost trends by model and session date  
**So that** I can budget and optimize which models to use

**Acceptance Criteria:**
- [ ] Analytics panel has tabs: `This Session  Today  All Time`
- [ ] Session tab shows current turn-by-turn costs (already done in UH-219)
- [ ] Today tab: hourly breakdown (cost per hour), total for day
- [ ] All Time tab: weekly breakdown (or per-project if project context exists)
- [ ] Model selector shows breakdown: `sonnet: $5.20 (60%)  haiku: $3.10 (40%)`
- [ ] Expandable cost details per model version

**Design Notes:** Tab bar (§5.10), list items (§5.2), progress bar for percentages  
**Priority:** P3

---

## CONFIG PANEL (space+C)

### UH-222: View and edit settings with scoping
**As a** user  
**I want** to see all project + global settings and edit them from the panel  
**So that** I can adjust behavior without touching config files

**Acceptance Criteria:**
- [ ] Config panel shows settings grouped by scope: `Project Override  User Local  Global Default`
- [ ] Each setting row: name, current value, source indicator (e.g., `[project]`, `[user]`)
- [ ] Value shown as: toggle (for bools), dropdown (for enums), text input (for strings)
- [ ] Pressing `enter` on focused setting enters edit mode; `esc` cancels
- [ ] Save writes to appropriate file (project → `.claudio/settings.json`, user → `~/.claudio/settings.json`)
- [ ] Scope precedence shown in help: "Project overrides User overrides Global"

**Design Notes:** List items with inline editors (§5.3), status indicators per setting, panel (§5.1)  
**Priority:** P1

---

### UH-223: Switch default model from Config panel
**As a** user  
**I want** to change the default LLM without opening a modal  
**So that** I can switch models quickly during a session

**Acceptance Criteria:**
- [ ] Config panel shows `model` setting with dropdown: `haiku  sonnet  opus  ...`
- [ ] Focused setting shows current model in `primary`
- [ ] Pressing `↓/↑` or `space` cycles models (or shows dropdown)
- [ ] Selection applies to **next turn** (current streaming message unaffected)
- [ ] Status bar updates: `model: sonnet` to reflect change
- [ ] Tooltip shows model context limits and cost/token ratio

**Design Notes:** Dropdown selector (inline, cycle with ↓/↑), status bar (§5.8), semantic info tooltip  
**Priority:** P2

---

### UH-224: Configure token budget and compaction threshold
**As a** project lead  
**I want** to set session token budget and auto-compaction trigger point  
**So that** I can enforce context limits for long sessions

**Acceptance Criteria:**
- [ ] Config panel shows: `budgetTokens: [input field]`, `compactionThreshold: [percentage selector]`
- [ ] Budget shows current session usage inline: `100000 / 150000`
- [ ] Edit mode allows numeric input; validation enforces range (1K–300K)
- [ ] Threshold dropdown: `75% (aggressive)  85%  95% (lazy)`
- [ ] Save applies immediately; next query respects new limits
- [ ] Tooltip explains: "Token budget gates context size; compaction triggers at threshold to stay under limit"

**Design Notes:** Numeric input with validation (§5.3), dropdown selector, inline status hint  
**Priority:** P2

---

### UH-225: Review and manage hook profiles
**As a** administrator  
**I want** to enable/disable hook profiles (e.g., "logging", "notifications", "git-integration")  
**So that** I can control which automations run without editing individual hooks

**Acceptance Criteria:**
- [ ] Config panel has "Hook Profiles" section with checkbox list
- [ ] Profiles shown: `logging (3 hooks)  notifications (2 hooks)  git-integration (1 hook)`
- [ ] Unchecked profile disables all its hooks for the session
- [ ] Change applies immediately; affected hooks' status shown in Hooks section
- [ ] Profiles persist to session (not config) unless explicitly saved
- [ ] Help text explains: "Profiles group hooks by purpose; disable to skip related automations"

**Design Notes:** Checkbox list (toggle prefix), status badges (§5.5), inline counts  
**Priority:** P3

---

## WEB UI SCREENS

### UH-226: Web UI — Chat view with SSE streaming
**As a** web user  
**I want** to type messages and see streaming responses in real-time  
**So that** I can converse with Claude via browser without terminal

**Acceptance Criteria:**
- [ ] Page layout: sidebar (navigation) + main (chat) + right panel (optional context)
- [ ] Chat history renders messages with: avatar, sender label (You / Assistant), timestamp
- [ ] New messages stream in real-time via SSE (`text`, `thinking`, `tool_start`, `tool_end`)
- [ ] Message input (textarea) at bottom with `Ctrl+Enter` or `[Send]` button
- [ ] Thinking blocks render collapsible; tool calls render with status badges
- [ ] Scrolls to latest message; user can scroll up to view history

**Design Notes:** HTML partials (templ), HTMX for form submission, SSE event stream, responsive grid layout  
**Priority:** P1

---

### UH-227: Web UI — Tool approval dialog
**As a** web user  
**I want** to approve/deny tool execution via browser modal  
**So that** I can control risky operations from web interface

**Acceptance Criteria:**
- [ ] SSE event `approval_needed` triggers modal: tool name, file path, 4 buttons
- [ ] Buttons: `Allow  Deny  Always  Allow All` with click handlers
- [ ] POST to `/api/chat/approve` with decision; modal closes, execution proceeds/fails
- [ ] Button styles: green (allow), red (deny), blue (always), gray (all)
- [ ] Timeout alert (optional): "Approval expires in 30s"
- [ ] Page shows "Awaiting approval..." spinner during wait

**Design Notes:** Modal overlay (HTML), form submission via HTMX POST, SSE blocking pattern  
**Priority:** P1

---

### UH-228: Web UI — Session list and create
**As a** web user  
**I want** to view all sessions, create new ones, and switch between them  
**So that** I can manage multiple conversation threads from browser

**Acceptance Criteria:**
- [ ] Home page (`/`) shows: "New Session" button, session list (reverse chronological)
- [ ] Session list rows: title, agent type, created date, last message preview, [delete] button
- [ ] Click row to open session; auto-redirects to `/session/{id}`
- [ ] `[New Session]` button opens picker: template (optional), agent type (dropdown), [Create]
- [ ] Delete button shows `(y/n)` confirmation inline
- [ ] Search bar filters sessions by title or content snippet

**Design Notes:** HTML list, HTMX POST to `/api/sessions/create`, table-like layout responsive  
**Priority:** P1

---

### UH-229: Web UI — Agent view with live status
**As a** team lead  
**I want** to see active agents, their tasks, and live status updates  
**So that** I can monitor team progress from browser

**Acceptance Criteria:**
- [ ] Sidebar or dedicated `/agents` page lists active agents
- [ ] Agent card shows: name, model, status badge (running/idle/error), task, duration
- [ ] SSE events update agent status in real-time (no page refresh)
- [ ] Click agent to expand: full conversation history, send message to agent
- [ ] `[Kill]` button stops agent; confirmation dialog required
- [ ] Agent detail shows: tokens used, model, team assignment

**Design Notes:** Agent card component (§5.12 from TUI design, adapted to HTML), HTMX polling alternative to SSE, live update via HTMX  
**Priority:** P2

---

### UH-230: Web UI — Tool call details modal
**As a** user  
**I want** to inspect tool input/output in detail from web chat  
**So that** I can debug tool behavior or review exact data

**Acceptance Criteria:**
- [ ] Tool call row in chat renders with inline summary: tool name, status, duration
- [ ] Click or expand icon opens modal: full input (JSON), full output (text/JSON), token count
- [ ] Input/output syntax-highlighted (code block styling)
- [ ] Modal has `[Copy Input]`, `[Copy Output]`, `[Close]` buttons
- [ ] Scrollable if content exceeds modal height
- [ ] Close via X button or `esc` key

**Design Notes:** HTML modal (templ), syntax highlighting (highlight.js or server-side), responsive sizing  
**Priority:** P2

---

### UH-231: Web UI — Model selector and thinking mode
**As a** user  
**I want** to change the model and thinking mode from web interface  
**So that** I can adjust behavior without CLI commands

**Acceptance Criteria:**
- [ ] Header or chat settings shows: `Model: [dropdown]  Thinking: [toggle]  Budget: [slider]`
- [ ] Model dropdown lists available models with token limits, cost per K
- [ ] Thinking mode: three-way toggle (disabled/adaptive/enabled)
- [ ] Budget slider shows: current session usage, max budget (with dollar estimate)
- [ ] Changes apply to **next message** (current streaming unaffected)
- [ ] Settings persist to session cookie (not account-level)

**Design Notes:** HTML form, HTMX POST to `/api/commands/model`, responsive dropdowns/sliders  
**Priority:** P2

---

### UH-232: Web UI — Memory panel (session-scoped)
**As a** web user  
**I want** to view and manage memories from browser  
**So that** I can understand context without terminal access

**Acceptance Criteria:**
- [ ] Chat sidebar or dedicated panel shows memory entries (agent/project/global scopes)
- [ ] Search bar filters memories by keyword
- [ ] Type filter buttons: `all  user  feedback  project  reference`
- [ ] Click memory to expand: full content, edit button, delete button
- [ ] Edit mode: textarea with save/cancel buttons
- [ ] `[+ New]` button creates memory via modal form

**Design Notes:** Sidebar panel (HTML partial), HTMX for search/filter, edit form modal  
**Priority:** P2

---

### UH-233: Web UI — Config editor
**As a** user  
**I want** to view and edit settings from web UI  
**So that** I can manage configuration without terminal

**Acceptance Criteria:**
- [ ] Settings page (`/settings`) shows grouped sections: Model, Tokens, Permissions, Hooks
- [ ] Each setting: label, current value, edit input
- [ ] Toggle settings: checkbox; dropdown: HTML select; string: text input
- [ ] Save button at bottom: POST to `/api/config/update`
- [ ] Success toast: "Settings saved"
- [ ] Scope indicator (project/user/global) shown per setting
- [ ] Changes apply immediately to current session and persist to file

**Design Notes:** HTML form, HTMX POST, form validation before submit, status toast  
**Priority:** P2

---

### UH-234: Web UI — File picker for context attachment
**As a** user  
**I want** to attach files to my message via browser file picker  
**So that** I can include code/docs in conversation without CLI

**Acceptance Criteria:**
- [ ] Chat input shows `[+ Attach]` button or drag-drop zone
- [ ] Clicking opens native file picker (single or multi-select)
- [ ] Selected files shown as pills above input: `[file.go]` `[README.md]` with remove `✕`
- [ ] File pills styled as badges (§5.5)
- [ ] Max 10 files per message; size limit per file (50MB default)
- [ ] Send button includes files in POST body or as separate request

**Design Notes:** HTML file input, HTMX file upload, drag-drop via JS, pill badges  
**Priority:** P2

---

### UH-235: Web UI — Sidebar blocks (Files, Tokens, Todos)
**As a** user  
**I want** quick access to session context (files touched, tokens used, active todos) in sidebar  
**So that** I can monitor session state without opening panels

**Acceptance Criteria:**
- [ ] Sidebar shows 3 collapsible blocks: Files, Tokens, Todos
- [ ] Files block: list of files read/written/modified (icons: ✓/✎/◐)
- [ ] Tokens block: input/output/cache counts, total cost in USD
- [ ] Todos block: active planning tasks (3-item preview, link to full list)
- [ ] Block headers are clickable links to full views (Files panel, Analytics panel, Tasks panel)
- [ ] Blocks update via HTMX polling or SSE on message completion

**Design Notes:** HTML sidebar (responsive nav), HTMX updates, icon badges (§5.5), collapsible sections  
**Priority:** P2

---

### UH-236: Web UI — Sidebar navigation (Sessions, Agents, Teams)
**As a** user  
**I want** to navigate between sessions, view agents, and manage teams  
**So that** I can orchestrate multi-agent work from browser

**Acceptance Criteria:**
- [ ] Sidebar has tabs: `Sessions  Agents  Teams`
- [ ] Sessions tab: tree of current project's sessions (parent/children relationships shown)
- [ ] Agents tab: list of active agents in team (if in team), each with status badge
- [ ] Teams tab: available team templates and active teams, `[+ Create Team]` button
- [ ] Click to switch session (auto-redirects), expand agent detail, or create team
- [ ] Search bar filters by name
- [ ] Responsive collapse for mobile (hamburger menu)

**Design Notes:** Sidebar nav (HTML), tab bar (templ), tree rendering (nested lists or HTMX swaps)  
**Priority:** P2

---

### UH-237: Web UI — Analytics view (tokens, cost, cache)
**As a** analyst  
**I want** to see token usage trends and cost breakdown across sessions  
**So that** I can optimize spend and cache strategy

**Acceptance Criteria:**
- [ ] Analytics page (`/analytics`) shows: total USD cost (today, week, month, all-time)
- [ ] Tab views: `By Model  By Date  By Agent  Cache Performance`
- [ ] By Model: pie chart (or bar) showing cost distribution across haiku/sonnet/opus
- [ ] By Date: line chart showing daily cost trend
- [ ] By Agent: table with agent name, task count, total cost, avg cost per task
- [ ] Cache Performance: table showing cache hit %, bytes saved, time saved
- [ ] Filters: date range picker, agent filter, model filter

**Design Notes:** HTML tables, optional charting library (chart.js), responsive grid, export CSV button  
**Priority:** P3

---

## DIALOGS & MODALS

### UH-238: Model selector dialog (space+m in TUI; web dropdown)
**As a** user  
**I want** a modal to choose model, thinking mode, and token budget quickly  
**So that** I can adjust execution strategy before sending a message

**Acceptance Criteria:**
- [ ] Dialog title: "Run Settings"
- [ ] Model selector (dropdown or button grid): haiku, sonnet, opus, custom
- [ ] Thinking mode (3-way toggle): "Disabled", "Adaptive", "Enabled" with descriptions
- [ ] Budget picker (slider or numeric input): range 1K–300K, shows USD estimate
- [ ] Preview row: "Estimated cost: $0.12 for this turn"
- [ ] Buttons: `[Apply]  [Cancel]`; Apply updates session defaults for next turn

**Design Notes:** Modal (§5.9), button actions (§5.4), toggle/selector (§5.3), tooltip explanations  
**Priority:** P1

---

### UH-239: File picker dialog (/attach command)
**As a** user  
**I want** a fuzzy-searchable file picker to attach context files  
**So that** I can add code files without typing full paths

**Acceptance Criteria:**
- [ ] Dialog title: "Attach Files"
- [ ] Search bar filters files from cwd recursively (fuzzy match on filename)
- [ ] Results show file path (relative) and size
- [ ] Multi-select via checkbox or shift+click
- [ ] Selected count shown: `2 selected`
- [ ] Buttons: `[Attach (2)]  [Cancel]`; Attach adds files to prompt context
- [ ] Max 10 files per attach action

**Design Notes:** Modal (§5.9), search input (§5.3), checkbox list (§5.2), status hint  
**Priority:** P1

---

### UH-240: Permissions approval dialog
**As a** user  
**I want** to approve/deny/allow-always tool execution via modal  
**So that** I can control risky operations with clear options

**Acceptance Criteria:**
- [ ] Modal border colored `warning` (#fabd2f)
- [ ] Title: "Permission Required"
- [ ] Rows: Tool name (bold), File path (info color), Description (dim)
- [ ] 4 buttons: `[Allow]  [Deny]  [Always Allow]  [Allow All Tools]`
- [ ] Keyboard shortcuts: `a` (Allow), `d` (Deny), `!` (Always), `A` (All)
- [ ] Denying shows error message inline; Allow proceeds with tool execution

**Design Notes:** Modal (§5.9) with `warning` border, button styling (§5.4), help hint with shortcuts  
**Priority:** P1

---

## SIDEBAR BLOCKS

### UH-241: Files block (sidebar right, collapsible)
**As a** user  
**I want** a quick view of files read/written/modified in the session  
**So that** I can see session impact at a glance

**Acceptance Criteria:**
- [ ] Block title: `Files` (colored `info`)
- [ ] Content: list of files with operation icons: `✓` (read), `✎` (written), `◐` (modified)
- [ ] File paths truncated to 30ch; hover shows full path
- [ ] Click file to jump to conversation message that touched it
- [ ] Max 5 files shown; `[more]` link opens full Files panel (space+f)
- [ ] Counter badge: `Files (12)` showing total count

**Design Notes:** Sidebar block component, icon badges (§5.5), truncate text with hover (tooltip pattern)  
**Priority:** P2

---

### UH-242: Tokens block (sidebar, always visible)
**As a** cost-conscious user  
**I want** token usage visible in sidebar without opening Analytics panel  
**So that** I can monitor budget at a glance

**Acceptance Criteria:**
- [ ] Block title: `Tokens`
- [ ] Lines: `Input: 8.2k  Output: 4.1k  Cache: 3.2k  Total: 15.5k / 100k`
- [ ] Usage bar: visual progress from 0 to budget, color shifts success → warning → error
- [ ] Cost line: `Cost: $0.04`
- [ ] Percentage indicator: `15.5% of budget`
- [ ] Click or `?` shows tooltip: "Update budget in Config (space+C)"

**Design Notes:** Sidebar block, progress bar (§5.6), semantic colors (§2.4), inline hint  
**Priority:** P1

---

### UH-243: Todos block (sidebar, collapsible)
**As a** user  
**I want** active planning tasks visible in sidebar  
**So that** I can keep planning goals in view during conversation

**Acceptance Criteria:**
- [ ] Block title: `Todos`
- [ ] List shows up to 3 active (non-completed) tasks: status icon, ID, subject
- [ ] Each row: `◐ #3 Write cache layer  @agent-1`
- [ ] Click to navigate to task in Tasks panel (space+t)
- [ ] If more than 3: `[+2 more]` link opens full Tasks panel
- [ ] Empty state: `No active todos` (italic, muted)

**Design Notes:** Sidebar block, list items (§5.2), status badges (§5.5), task display pattern  
**Priority:** P2

---

## END OF USER STORIES

---

**Total Stories:** 44 (UH-200 → UH-243)

**Priority Breakdown:**
- **P1 (Core workflow):** 14 stories — Essential for first release
- **P2 (Important):** 24 stories — Enhance usability, add depth
- **P3 (Nice-to-have):** 6 stories — Optimization, advanced features

**Implementation Order Recommendation:**
1. **Phase 1 (P1):** Tools panel, tool calls display, chat streaming, permission dialog, memory view, config editing, session list, model selector, file picker, tokens block
2. **Phase 2 (P2):** Search/filter UX, details modals, web UI panels, sidebar blocks, analytics basics
3. **Phase 3 (P3):** Hook management, rule editing, cost trends, advanced analytics

---

**Design References:**
- Section numbers refer to `docs/design-system.md`
- Component library: §5.1 – §5.15
- Interaction patterns: §6.1 – §6.5
- Color system: §2.1 – §2.5
- Typography: §3.1 – §3.4

---

**Notes for Implementation:**
- Web UI assumes Go `templ` templates + HTMX for interactivity
- TUI uses Bubbletea for event loop, Lipgloss for styling, Bubbles components
- All modals support keyboard shortcuts; no mouse-only UX
- SSE streaming is core pattern for web; TUI uses `tea.Cmd` for equivalent
- Design system tokens available in `styles/theme.go`
