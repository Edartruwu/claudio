# Investigation Report: Deferred Tools, Plugin Instructions, and Memory Index

## 1. How Deferred Tools Work (the full flow)

### Subject
Deferred tools are a token-saving mechanism where tool schemas are listed by name in a `<system-reminder>` block rather than sent inline with every API request. They are only fetched on-demand when the model calls `ToolSearch`.

### Key Findings

#### A. Tools Marked as "Deferred"
- **Location:** `internal/tools/deferrable.go:5-14`
- **Mechanism:** Tools implement the `DeferrableTool` interface with methods:
  - `ShouldDefer() bool` — returns true if tool should defer loading
  - `SearchHint() string` — returns search hint for ToolSearch keyword matching
- **Implementation:** The `deferrable` struct provides reusable embedding; tools embed this and return `true` from `ShouldDefer()`
- **Example deferred tools:** WebSearch, WebFetch, NotebookEdit, TaskCreate, TaskList, TaskGet, TaskUpdate, TaskStop, TaskOutput, SendMessage, TeamCreate, TeamDelete, EnterWorktree, ExitWorktree, and all plugin tools (via `PluginProxyTool`)

#### B. Building the System Reminder Block
- **Location:** `internal/query/engine.go:1190-1206` — method `buildSystemWithDeferredTools()`
- **Field:** `frozenDeferredReminder` at line 98 (Engine struct)
- **Process:**
  1. Called once per session (frozen on first build)
  2. Checks if `frozenDeferredReminder == ""` (line 1197)
  3. Calls `e.registry.DeferredToolNames()` (line 1198) to get list of deferred tool names
  4. Builds the reminder block (lines 1200-1201):
     ```
     \n\n<system-reminder>\nThe following deferred tools are available via ToolSearch:\n[tool names, one per line]\n</system-reminder>
     ```
  5. Block is appended to base system prompt + system context (line 1205)
  6. Stored in `frozenDeferredReminder` to avoid reconstruction per turn

#### C. ToolSearch Meta-Tool
- **Location:** `internal/tools/toolsearch.go:10-153`
- **Type:** `ToolSearchTool` struct
- **Key methods:**
  - `Name()` at line 15: returns "ToolSearch"
  - `Description()` at lines 17-28: explains deferred tool concept and query syntax
  - `InputSchema()` at lines 30-46: JSON schema accepting `query` (string, required) and `max_results` (number, default 5)
  - `Execute()` at lines 48-145: performs tool lookup and returns matched schemas
    - Direct selection: `"select:TaskCreate,TaskUpdate"` → returns only named tools (lines 66-74)
    - Keyword search: `"notebook jupyter"` → searches tool names and hints, returns top matches by score (lines 75-119)
    - Returns matched tools as `<functions>...JSON...</functions>` blocks (lines 136-142)

#### D. When Tools Get Injected: Token-on-Demand Mechanism
- **Location:** `internal/query/engine.go` around line 433 (in Query method)
- **Code:** `Tools: e.registry.APIDefinitionsWithDeferral(e.discoveredTools)`
- **Process:**
  1. On each API request, `APIDefinitionsWithDeferral(discoveredTools map[string]bool)` is called
  2. Tools in `discoveredTools` map get full schemas (name + description + inputSchema)
  3. Tools NOT in `discoveredTools` that `ShouldDefer()` are **omitted entirely** from the tools array
  4. Their names are already in the system reminder (frozen), so model knows they exist
  5. Model can call `ToolSearch` to load any deferred tool on demand

#### E. Tracking Discovered Tools
- **Location:** `internal/query/engine.go:1208-1244` — method `trackDiscoveredTools(input json.RawMessage)`
- **When called:** After executing ToolSearch (parsed from execution input)
- **Field:** `discoveredTools` map at line 97 (Engine struct)
- **Logic:**
  - For `"select:..."` queries (lines 1218-1224): marks each named tool as discovered
  - For keyword queries (lines 1226-1243): re-runs matching logic and marks all matching tools as discovered
- **Persistence:** Discovered tools stay in `discoveredTools` map for the rest of the session

#### F. Deferral Resolution Logic
- **Location:** `internal/tools/registry.go:126-155` — function `resolveDeferral()`
- **Called from:** `DeferredToolNames()` (line 106) and `APIDefinitionsWithDeferral()` (line 79)
- **Rules applied (in order):**
  1. Line 128-129: Non-deferrable tools → return false (always eager)
  2. Line 133-136: Check user override via `deferOverride` map
  3. Line 141-143: Already discovered → return false (load full schema)
  4. Line 148-151: Check AutoActivatable interface; if backing service available → return false (auto-load)
  5. Line 154: Otherwise → return true (defer)

#### G. API-Side Handling (No defer_loading flag in actual implementation)
- **Location:** `internal/api/client.go:1020-1041` — function `applyPromptCaching()`
- **Process:**
  1. Iterates tool definitions array
  2. Filters to non-deferred tools (line 1032: checks if deferred, then `continue`)
  3. Applies `cache_control: ephemeral` to first non-deferred tool definition (line 1035)
  4. Note: Deferred tools are never sent to API (excluded in `APIDefinitionsWithDeferral`), so no need for defer_loading flag on wire

#### H. System Reminder Lifecycle
- **Field:** `frozenDeferredReminder` in Engine struct (line 98)
- **Built:** Once on first `buildSystemWithDeferredTools()` call
- **Frozen:** Never rebuilt for that session (saves computation and ensures model sees consistent tool list)
- **Override capability:** User can set per-tool via `registry.SetDeferOverride(name string, deferred bool)` (line 182 in registry.go)

---

## 2. How Plugins Load Their Instructions

### Subject
Plugins provide `--instructions` markdown that gets injected into the system prompt so the model knows to prefer plugin tools over built-in tools like Grep/Glob/Read.

### Key Findings

#### A. Plugin Discovery and Loading
- **Location:** `internal/plugins/plugins.go:17-24` (Plugin struct definition)
- **Type:** `Plugin` struct with fields:
  - `Name`: derived from filename (after stripping extensions like .sh, .py)
  - `Path`: absolute path to executable
  - `Description`: from `--describe` flag (first line of output)
  - `Schema`: from `--schema` flag (optional JSON schema)
  - `Instructions`: from `--instructions` flag (markdown)
  - `IsScript`: boolean indicating if it's a script or compiled binary

#### B. Plugin Discovery Flow
- **Location:** `internal/plugins/plugins.go:36-84` — function `Registry.LoadDir(dir string)`
- **Process:**
  1. Line 39: Reads directory for all files
  2. Line 47-50: Filters to executable files only (checks `info.Mode()&0111 != 0`)
  3. Line 63-67: Strips common extensions (.sh, .py, .rb, .js)
  4. Line 76: Calls `getPluginDescription(path)` → runs `plugin --describe`
  5. Line 77: Calls `getPluginSchema(path)` → runs `plugin --schema`
  6. Line 78: Calls `getPluginInstructions(path)` → runs `plugin --instructions`
  7. Line 80: Appends `Plugin` struct to registry

#### C. Plugin Instruction Extraction
- **Location:** `internal/plugins/plugins.go:153-160` — function `getPluginInstructions(path string) string`
- **Process:**
  1. Line 154: Executes `path --instructions` as subprocess
  2. Line 156: If successful and output non-empty, returns trimmed output
  3. Line 159: Returns empty string if command fails or no output

#### D. System Prompt Injection Format
- **Location:** `internal/prompts/system.go:208-235` — function `PluginsSection(plugins []PluginInfo) string`
- **Input:** Array of `PluginInfo` structs (lines 202-206) with Name, Description, Instructions
- **Output format:**
  ```
  # Installed Plugins
  The following plugins are installed as deferred tools. To use a plugin, call ToolSearch with the plugin name to fetch its schema, then call it.
  
  ## plugin_<name>
  <instructions or description>
  
  ## plugin_<name2>
  <instructions or description2>
  ...
  ```
- **Details:**
  - Line 216: Heading "# Installed Plugins"
  - Line 217-218: Explanatory text
  - Line 221: Each plugin gets heading `## plugin_<name>`
  - Lines 222-224: If Instructions non-empty, use that verbatim
  - Lines 226-231: Otherwise use Description field

#### E. Where PluginsSection is Called
- **Location 1:** `internal/app/app.go:303-313`
  - Line 277-279: Loads plugins from `~/.claudio/plugins/` and `./.claudio/plugins/`
  - Line 280-289: Registers each as a tool via `PluginProxyTool` wrapper
  - Line 304-310: Creates `pluginInfos` array from all plugins
  - Line 312: Calls `prompts.PluginsSection(pluginInfos)`
  - Injects result into `teamRunner.PluginsSection` field

- **Location 2:** `internal/app/app.go:690-702`
  - When spawning team agents, loads agent-specific plugins
  - Line 695: Calls `prompts.PluginsSection()` with extra plugins
  - Appends result to existing team runner plugins section

#### F. System Prompt Integration
- **Location:** `internal/tui/root.go:5218-5251` — function `Model.buildFullSystemPrompt() string`
- **Flow:**
  1. Lines 5222-5248: Collects sections (rules, learning, output style, snippets)
  2. Line 5250: Joins sections
  3. Line 5251: Calls `prompts.BuildSystemPrompt(m.model, additionalCtx)`
  4. Note: PluginsSection is NOT explicitly called here for main TUI
  5. It's pre-built by app layer and injected into team agents

#### G. Plugin Registration as Deferred Tools
- **Location:** `internal/tools/proxy.go` (PluginProxyTool wrapper)
- **Mechanism:** Each plugin becomes a deferred tool via `PluginProxyTool`
- **In registry:** Plugins registered like any other tool but with `ShouldDefer() = true`
- **Availability:** Plugin names appear in the `<system-reminder>` block alongside other deferred tools

---

## 3. Memory Index Injection and Controls

### Subject
Memory is persistent across sessions and is injected as a user message at session start. The index is built on-demand and controls what memories get surfaced to the model.

### Key Findings

#### A. Memory Index Building
- **Location:** `internal/services/memory/scoped.go:122-152` — method `ScopedStore.BuildIndex() string`
- **Format:** Multi-section markdown with scope headers
  ```
  ### Global Memories
  - entry1 [tags]: description — "fact1" | "fact2"
  - entry2: description — "fact1"
  
  ### Project Memories
  - entry3: description — "fact1" | "fact2"
  
  ### Agent Memories
  - entry4: description — "fact1"
  ```
- **Per-entry format:** `- name [tags]: description — "fact1" | "fact2"`
  - Name: entry name
  - Tags: optional comma-separated list (if present)
  - Description: one-liner
  - Facts: first 2 facts shown, each truncated to 60 chars
- **Scope priority:** agent > project > global (higher priority memories hide lower-scope duplicates)

#### B. Building Index Lines (Per Store)
- **Location:** `internal/services/memory/memory.go:265-316` — method `Store.BuildIndexLines() string`
- **Process:**
  1. Line 268: Loads all entries via `LoadAll()`
  2. Lines 274-276: Sorts by `UpdatedAt` descending (newest first)
  3. Lines 279-313: For each entry, builds one-line summary
  4. Line 315: Returns compact multi-line string
- **Limits enforced by `enforceIndexLimits()` (lines 495-507):**
  - Max 200 lines (const `maxIndexLines` at line 18)
  - Max 25KB (const `maxIndexBytes` at line 19)
  - Trimming removes oldest entries first (because of reverse chronological sort)

#### C. Memory Entry Structure
- **Location:** `internal/services/memory/memory.go:27-39` — struct `Entry`
- **Fields for decay/expiry:**
  - `UpdatedAt time.Time` (line 38): last modification timestamp (used for sorting in BuildIndex)
  - `Tags []string` (line 36): user-provided tags (available for filtering)
  - `Concepts []string` (line 37): semantic tags (could be used for relevance filtering)
  - `Scope string` (line 32): "global", "project", or "agent" (controls priority and write target)
  - `Type string` (line 31): "user", "feedback", "project", "reference" (available for filtering)
- **Current decay mechanism:** Implicit via `maxIndexLines` limit (oldest entries removed when limit exceeded)
- **No explicit TTL:** UpdatedAt is set (line 94) but not used for automatic expiry/trimming; only used for sort order

#### D. Injection into Session
- **Location:** `internal/query/engine.go:347-353` (inside Query() method)
- **Process:**
  1. Line 347: Check `!memoryIndexInjected && memoryIndexMsg != "" && (len(e.messages) == 0 || e.userContextInjected)`
  2. Line 348-353: Add memory index as a separate user message
  3. Sets `memoryIndexInjected = true` to prevent re-injection
  4. Placed after userContext message (second injection point)
- **Trigger condition:** Only injected once per session when conversation is fresh OR just after user context injection

#### E. Setting Memory Index
- **Location 1:** `internal/query/engine.go:264-267` — method `Engine.SetMemoryIndex(index string)`
- **Effect:** Stores index in `memoryIndexMsg` (line 265), sets `memoryIndexInjected = false` (line 266)

- **Location 2:** `internal/tui/root.go:2107-2112` (session start in `handleSubmit`)
- **Code:**
  ```go
  if m.appCtx != nil && m.appCtx.Memory != nil {
    idx := m.appCtx.Memory.BuildIndex()
    if idx != "" {
      m.engine.SetMemoryIndex("## Your Memory Index\n\n" + idx)
    }
  }
  ```

- **Location 3:** `internal/tui/root.go:4129-4134` (session switch in `doSwitchSession`)
- **Code:** Same pattern as handleSubmit

#### F. Memory Index Persistence Model
- **Timing:**
  - Built once when session starts (via `SetMemoryIndex` call from TUI)
  - Not rebuilt during session (frozen for duration)
  - Rebuilt on next session start
- **Storage:** Three separate directories managed by `ScopedStore` (lines 22-24 in scoped.go):
  - `s.agent`: agent-scoped memories (if running as crystallized agent)
  - `s.project`: project-scoped memories (default write target)
  - `s.global`: global/user-level memories (fallback)

#### G. Compaction and Memory Index
- **Location:** `internal/services/compact/compact.go`
- **Finding:** **Compaction does NOT touch memory index**
  - No memory-related code in compact.go
  - Compaction only affects conversation history/messages
  - Memory index remains stable across compaction

#### H. Existing Mechanisms for Decay/Trim
- **Implicit size-based:** `enforceIndexLimits()` removes entries when:
  - Index exceeds 200 lines OR 25KB
  - Removes oldest entries first (due to reverse chronological sort at line 275)
- **No explicit features:**
  - No time-based TTL (could be added using `UpdatedAt`)
  - No access count decay (Entry struct has no access tracking)
  - No manual archival mechanism
  - No concept-based filtering in index building

#### I. Memory Relevance Finding
- **Location:** `internal/services/memory/memory.go:239-254` — method `Store.FindRelevant(context string) []*Entry`
- **When used:** Not currently called by engine (index is static per session)
- **If used:** Would match against name, description, facts, content, tags (line 248)
- **Potential use:** Could be called to build dynamic index per turn (currently not done)

---

## Symbol Map

| Symbol | File | Role |
|--------|------|------|
| `DeferrableTool` | `internal/tools/types.go` | Interface for tools that can defer loading |
| `ShouldDefer()` | `internal/tools/types.go` | Method indicating if tool should defer |
| `SearchHint()` | `internal/tools/types.go` | Search hint for ToolSearch keyword matching |
| `deferrable` | `internal/tools/deferrable.go:5` | Embeddable struct implementing DeferrableTool |
| `buildSystemWithDeferredTools()` | `internal/query/engine.go:1190` | Builds system prompt with deferred tool names |
| `frozenDeferredReminder` | `internal/query/engine.go:98` | Cached system-reminder block (built once per session) |
| `ToolSearchTool` | `internal/tools/toolsearch.go:11` | Meta-tool that fetches deferred tool schemas on demand |
| `Execute()` | `internal/tools/toolsearch.go:48` | ToolSearch execute method (keyword or direct selection) |
| `APIDefinitionsWithDeferral()` | `internal/tools/registry.go:75` | Returns tool definitions, omitting undiscovered deferred tools |
| `DeferredToolNames()` | `internal/tools/registry.go:102` | Returns names of tools that should be deferred |
| `trackDiscoveredTools()` | `internal/query/engine.go:1208` | Marks tools as discovered after ToolSearch execution |
| `discoveredTools` | `internal/query/engine.go:97` | Map of tools fetched via ToolSearch (persists across turns) |
| `resolveDeferral()` | `internal/tools/registry.go:126` | Applies deferral rules (override, discovery, auto-activate) |
| `SetDeferOverride()` | `internal/tools/registry.go:182` | User-facing override to pin tool deferral state |
| `Plugin` | `internal/plugins/plugins.go:17` | Plugin struct with Name, Path, Description, Schema, Instructions |
| `LoadDir()` | `internal/plugins/plugins.go:36` | Discovers and loads plugins from a directory |
| `getPluginInstructions()` | `internal/plugins/plugins.go:153` | Executes plugin with `--instructions` flag |
| `PluginsSection()` | `internal/prompts/system.go:211` | Converts plugin list to system prompt markdown |
| `PluginInfo` | `internal/prompts/system.go:202` | Struct holding Name, Description, Instructions for system prompt |
| `SetMemoryIndex()` | `internal/query/engine.go:264` | Sets memory index for injection at session start |
| `memoryIndexMsg` | `internal/query/engine.go:108` | Stores the memory index markdown (injected once) |
| `memoryIndexInjected` | `internal/query/engine.go:109` | Flag ensuring memory index injected only once per session |
| `BuildIndex()` | `internal/services/memory/scoped.go:124` | Builds multi-scope memory index with headers |
| `BuildIndexLines()` | `internal/services/memory/memory.go:267` | Builds compact per-store index (one line per entry) |
| `Entry` | `internal/services/memory/memory.go:28` | Memory entry struct with UpdatedAt, Tags, Concepts, Scope, Type |
| `enforceIndexLimits()` | `internal/services/memory/memory.go:495` | Trims index to 200 lines and 25KB, removing oldest first |
| `ScopedStore` | `internal/services/memory/scoped.go:21` | Manages memories across agent/project/global scopes |
| `FindRelevant()` | `internal/services/memory/memory.go:241` | Finds memories relevant to context (currently unused by engine) |

---

## Dependencies & Data Flow

### Deferred Tools Flow
1. **App startup** → Load plugins, register as deferred tools
2. **Session init** → Engine builds frozen reminder block with deferred tool names
3. **API request** → `APIDefinitionsWithDeferral()` filters tools:
   - Deferred + not discovered → omitted entirely
   - Deferred + discovered → included with full schema
   - Non-deferred → always included
4. **Model calls ToolSearch** → Engine tracks in `discoveredTools` map
5. **Next API request** → Discovered tools now sent with full schemas (persists for session)

### Plugin Instruction Flow
1. **App startup** → Plugin registry loads from `~/.claudio/plugins/` and `./.claudio/plugins/`
2. **For each plugin** → Execute `--instructions` flag, capture output
3. **System prompt build** → `PluginsSection()` formats plugin info into markdown section
4. **Session start** → Model receives instructions in system prompt telling it to prefer plugin tools
5. **Model calls ToolSearch** → Fetches full plugin schema from PluginProxyTool

### Memory Index Flow
1. **Session init** → TUI calls `m.appCtx.Memory.BuildIndex()` in handleSubmit/doSwitchSession
2. **BuildIndex** → ScopedStore aggregates memories from agent/project/global (in priority order)
3. **SetMemoryIndex** → Engine receives index, stores in `memoryIndexMsg`
4. **First Query() call** → Memory index injected as second user message (after user context)
5. **Frozen** → Index stays same for entire session (not rebuilt mid-conversation)
6. **Next session** → Index rebuilt fresh from current memory files on disk

---

## Risks & Observations

1. **Deferred tools system-reminder is frozen once built:**
   - If tools are added/removed mid-session via plugins or registry changes, model won't see them
   - Mitigation: Usually fine for fixed tool sets, but dynamic plugin loading could be problematic
   - No runtime mechanism to refresh the reminder block

2. **Discovered tools are never "forgotten":**
   - Once a tool is fetched via ToolSearch, it stays in `discoveredTools` map forever
   - If same session is resumed (e.g., via team agent continuation), discovered tools persist
   - This is intentional (saves tokens) but worth noting for state management
   - Could cause issues if tool availability changes mid-session

3. **Memory index built once per session, never refreshed:**
   - If user adds memories mid-conversation, new memories won't appear in the index sent to the model
   - Model won't know about them until next session starts
   - Could be unexpected if user expects real-time memory integration
   - `FindRelevant()` is defined but never called by engine

4. **No TTL or decay for memory entries:**
   - Only mechanism is implicit: size limits (200 lines, 25KB) remove oldest entries
   - Entries without `UpdatedAt` usage could accumulate indefinitely if they fit size budget
   - No concept of "archived" or "inactive" memories
   - No configuration UI for adjusting limits

5. **Plugin --instructions block placed in system prompt:**
   - This means plugin instructions are static for entire session
   - If plugin is updated mid-session, model won't see new instructions
   - Matches deferred tools pattern but could surprise users
   - Instructions are loaded once at app startup

6. **ToolSearch result format — delayed schema injection:**
   - Returns tools as `<functions>...JSON...</functions>` blocks
   - Engine parses and adds to `discoveredTools` map
   - After first ToolSearch, model's understanding and API's available tools may diverge for that turn
   - Full schema not sent until next API request (on second turn after ToolSearch)

7. **Memory index size limits are hardcoded:**
   - `maxIndexLines = 200` and `maxIndexBytes = 25*1024` at lines 18-19 in memory.go
   - No configuration mechanism; if user has many memories, oldest ones will be silently trimmed
   - No warning when limits are hit

8. **Plugin instructions executed at startup, not on-demand:**
   - Each plugin is called with `--instructions`, `--describe`, `--schema` once during app init
   - If plugin is slow or has side effects, it impacts startup time
   - No caching/memoization across sessions

---

## Open Questions

1. **Does auto-activation of deferred tools work correctly in practice?**
   - `AutoActivatable` interface is used in `resolveDeferral()` but unclear which tools implement it
   - Need to verify: if LSP or plugin backing service is unavailable, is the tool actually deferred?
   - Could tools be pinned as active when their service is actually down?

2. **How are agent-specific plugins integrated into PluginsSection?**
   - Code shows `prompts.PluginsSection()` is called twice (app init at line 312 and agent crystallization at line 695)
   - Unclear if both sections are combined or if second one replaces the first
   - What's the order when both global and agent plugins exist?

3. **Memory index size limits (200 lines, 25KB) — are these tuneable?**
   - Hardcoded as constants `maxIndexLines` and `maxIndexBytes`
   - No configuration mechanism visible in settings or config
   - Are these limits ever exposed to user? Can they be changed?

4. **Is memory index ever rebuilt during an active session?**
   - Current code freezes index once injected (`memoryIndexInjected = true`)
   - `FindRelevant()` is defined but never called by engine
   - Was per-turn dynamic index building intended but not implemented?

5. **Tool override mechanism — where can users disable deferred loading?**
   - `SetDeferOverride()` exists and is registered in Registry
   - But no visible CLI command or config UI that exposes this
   - Is this a programmatic-only feature?

6. **Does the discovered tools map survive session resume/continuation?**
   - `discoveredTools` is local to Engine instance
   - When session is resumed, is it a fresh Engine? Or the same one?
   - Token savings would be lost if it's a fresh instance

7. **What happens if ToolSearch returns 0 results?**
   - Engine returns "No tools matched the query" (line 122 in toolsearch.go)
   - Does model then try again with different query? Or does it get stuck?
   - Any retry logic or error recovery?
