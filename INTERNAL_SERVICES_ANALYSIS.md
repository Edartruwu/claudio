# Internal Services Package Analysis

## Directory Structure

```
internal/services/
├── analytics/           # Token usage, cost tracking, and budget enforcement
│   ├── analytics.go     # Core Tracker type
│   └── analytics_test.go
├── cachetracker/        # Prompt cache miss tracking and TTL expiry
│   ├── tracker.go
│   └── cachetracker_test.go
├── compact/             # Message compaction, budget enforcement, and pairing repair
│   ├── compact.go       # Main compaction logic
│   ├── budget.go        # Per-message tool result budget enforcement
│   ├── pairing.go       # Tool use/result pairing repair
│   ├── compact_test.go
│   ├── budget_test.go
│   ├── pairing_test.go
│   └── integration_test.go
├── difftracker/         # Git-based file change tracking per turn
│   ├── tracker.go
│   └── difftracker_test.go
├── filtersavings/       # Output filter analytics
│   ├── service.go       # Core Service type
│   ├── record.go        # Recording filter savings events
│   ├── stats.go         # Aggregate statistics queries
│   ├── discover.go      # Finding opportunities for new filters
│   └── service_test.go
├── lsp/                 # Language Server Protocol lifecycle management
│   ├── server.go        # ServerManager and ServerInstance types
│   └── server_test.go
├── mcp/                 # Model Context Protocol server management
│   ├── manager.go       # Manager and ServerState types
│   └── (no tests yet)
├── memory/              # Persistent cross-session memory storage
│   ├── memory.go        # Store, Entry, and init patterns
│   ├── scoped.go        # ScopedStore (agent/project/global priority)
│   ├── extract.go       # Background memory extraction agent
│   └── (no tests)
├── naming/              # Session naming via LLM
│   └── naming.go        # GenerateSessionName function
├── notifications/       # Desktop/terminal notifications
│   └── notifications.go # Notifier interface and impls
├── skills/              # Skill registry and loading
│   └── loader.go        # Registry type and LoadAll function
└── toolcache/           # On-disk caching for oversized tool results
    ├── cache.go         # Store type
    └── cache_test.go
```

**Total: 5,804 lines of Go code (non-test)**
**Total: 14 service packages**

---

## Service Types & Interfaces

### 1. **analytics** — Token Usage & Cost Tracking
**Responsibility**: Track API consumption (input/output/cache tokens), compute costs, enforce budgets, report analytics.

**Key Types**:
```go
// ModelPricing defines per-million-token pricing
type ModelPricing struct {
    InputPerMTok      float64
    OutputPerMTok     float64
    CacheReadPerMTok  float64
    CacheWritePerMTok float64
}

// Tracker accumulates token usage and cost for a session
type Tracker struct {
    mu                sync.Mutex
    model             string
    inputTokens       int
    outputTokens      int
    cacheReadTokens   int
    cacheCreateTokens int
    lastContextTokens int  // input+cacheRead+cacheCreate from most recent API call
    toolCalls         int
    apiCalls          int
    startTime         time.Time
    maxBudget         float64
    saveDir           string
}
```

**Exported Functions/Methods**:
- `NewTracker(model string, maxBudget float64, saveDir string) *Tracker` — Create tracker
- `RecordUsage(inputTokens, outputTokens, cacheReadTokens, cacheCreateTokens int)` — Record API call tokens
- `RecordToolCall()` — Increment tool call counter
- `TotalTokens() int` — Get current context window usage (last API call only, NOT cumulative)
- `CumulativeTokens() int` — Sum all tokens across all turns (for analytics/billing)
- `InputTokens() int`, `OutputTokens() int`, `CacheReadTokens() int`, `CacheCreateTokens() int` — Token breakdowns
- `CacheHitRate() float64` — Percentage of input tokens from cache (0-100)
- `MaxBudget() float64` — Get budget limit
- `Cost() float64` — Estimated cost in USD
- `CheckBudget() (warning string, exceeded bool)` — Check if budget exceeded or warning threshold (80%) reached
- `Report() string` — Formatted session report
- `SaveReport(sessionID string) error` — Persist analytics to disk as JSON

**Known Pricing** (hardcoded in `KnownPricing` map):
- `claude-opus-4-6`, `claude-opus-4-5`: Input $15, Output $75 per MTok
- `claude-sonnet-4-6`, `claude-sonnet-4-5`: Input $3, Output $15 per MTok
- `claude-haiku-4-5-20251001`: Input $0.25, Output $1.25 per MTok
- Falls back to keyword matching (opus, haiku) or defaults to Sonnet

**Thread Safety**: Mutex-protected

---

### 2. **cachetracker** — Prompt Cache Miss Analysis
**Responsibility**: Track when/why prompt cache is invalidated, monitor cache TTL expiry.

**Key Types**:
```go
type BreakReason string
// Constants: BreakReasonNewUserMessage, BreakReasonSystemChanged, BreakReasonUnknown

type Event struct {
    Turn              int
    Reason            BreakReason
    CacheCreateTokens int
    At                time.Time
}

type Tracker struct {
    mu           sync.Mutex
    events       []Event
    lastSystem   string
    lastMsgCount int
    turn         int
}

type ExpiryWatcher struct {
    mu          sync.Mutex
    lastAPICall time.Time
    ttl         time.Duration
}
```

**Exported Methods**:
- `(t *Tracker) Record(cacheCreate int, systemPrompt string, msgCount int) BreakReason` — Record after each API response; infers reason for cache miss
- `(t *Tracker) Events() []Event` — Get all recorded cache misses
- `(t *Tracker) Summary() string` — Human-readable cache efficiency summary
- `NewExpiryWatcher(ttl time.Duration) *ExpiryWatcher` — Create TTL watcher
- `(w *ExpiryWatcher) RecordCall()` — Mark time of API call
- `(w *ExpiryWatcher) IsExpired() bool` — Check if cache TTL likely expired
- `(w *ExpiryWatcher) TimeSinceLastCall() time.Duration` — Duration since last call

**Thread Safety**: Mutex-protected for both Tracker and ExpiryWatcher

---

### 3. **compact** — Message Compaction & Budget Enforcement
**Responsibility**: Summarize old messages to save tokens, enforce per-message tool result budgets, repair broken tool_use/tool_result pairs.

**Key Types**:
```go
type Strategy string  // StrategyAuto, StrategyManual, StrategyStrategic

type State struct {
    TotalTokens    int
    MaxTokens      int
    ToolCallCount  int
    PhaseChanges   int
    LastPhase      string  // "exploring", "planning", "implementing", "testing"
    ForceThreshold int     // % of context window to trigger full compact (0 = default 95%)
}

type ReplacementState struct {
    SeenIDs      map[string]bool  // tool_use_id → seen?
    Replacements map[string]string // tool_use_id → replacement text
}
```

**Exported Functions**:
- `Compact(ctx context.Context, client *api.Client, messages []api.Message, keepLast int, instruction string, pinnedIndices ...map[int]bool) ([]api.Message, string, error)` — Summarize old messages; pins specific indices to preserve verbatim
- `EnforceToolResultBudget(messages []api.Message, state *ReplacementState, store *toolcache.Store) []api.Message` — Apply per-message budget; persists large results to disk
- `EnsureToolResultPairing(messages []api.Message) []api.Message` — Repair broken tool_use/tool_result pairs after compaction

**State Methods**:
- `(s *State) ShouldSuggest(strategy Strategy) bool` — Check compaction suggestion threshold
- `(s *State) ShouldPartialCompact() bool` — Partial compaction (>70% context)
- `(s *State) ShouldFullCompact() bool` — Full compaction (>90% context)
- `(s *State) ShouldForce() bool` — Mandatory compaction (>95% or custom ForceThreshold)
- `(s *State) DetectPhase(recentTools []string) string` — Infer phase from tool usage

**ReplacementState Methods**:
- `NewReplacementState() *ReplacementState` — Create empty state

**Compaction Strategy**:
1. **Summarization**: Uses API to create structured summary; model is instructed NOT to call tools
2. **Preserved**: `keepLast` recent messages + pinned messages
3. **Budget**: Per-message tool results capped at 200KB; oversized results persisted to disk, replaced with 2KB preview
4. **Pairing**: Synthetic tool_result placeholders added for missing results (prevents API rejection)

**Key Constants**:
- `PerMessageBudget = 200_000` bytes
- `PreviewSize = 2000` bytes

---

### 4. **difftracker** — Git-Based Change Tracking
**Responsibility**: Capture before/after git state per turn; track which files were modified.

**Key Types**:
```go
type TurnDiff struct {
    Turn          int
    FilesModified []string
    Summary       string  // git diff --stat output
    Patch         string  // full diff output
}

type Tracker struct {
    mu       sync.Mutex
    diffs    []TurnDiff
    turnNum  int
    baseline string  // git diff output before turn
}
```

**Exported Methods**:
- `New() *Tracker` — Create tracker
- `(t *Tracker) BeforeTurn()` — Capture baseline git state before tools execute
- `(t *Tracker) AfterTurn() *TurnDiff` — Compute delta; returns nil if no changes
- `(t *Tracker) GetTurn(n int) *TurnDiff` — Get diff for specific turn
- `(t *Tracker) All() []TurnDiff` — Get all recorded diffs
- `(t *Tracker) Count() int` — Total turn count

**Implementation**: Shells out to `git diff` and `git diff --stat`; parses output to extract file list

**Thread Safety**: Mutex-protected

---

### 5. **filtersavings** — Output Filter Analytics
**Responsibility**: Track bytes saved by output filtering; discover commands that could benefit from filters.

**Key Types**:
```go
type Service struct {
    db *storage.DB  // SQLite connection
}

type Stats struct {
    TotalBytesIn  int64
    TotalBytesOut int64
    TotalSaved    int64
    SavingsPct    float64
    RecordCount   int64
}

type CommandStat struct {
    Command    string
    BytesIn    int64
    BytesOut   int64
    Saved      int64
    SavingsPct float64
    Count      int64
}

type DiscoverySuggestion struct {
    Command     string
    AvgBytesIn  int64
    Occurrences int64
}
```

**Exported Methods**:
- `NewService(db *storage.DB) *Service` — Create service
- `(s *Service) Record(ctx context.Context, command string, bytesIn, bytesOut int) error` — Insert filter savings record
- `(s *Service) GetStats(ctx context.Context) (Stats, error)` — Aggregate statistics
- `(s *Service) GetTopCommands(ctx context.Context, limit int) ([]CommandStat, error)` — Top commands by bytes saved
- `(s *Service) Discover(ctx context.Context, limit int) ([]DiscoverySuggestion, error)` — Commands with no filter opportunity (bytes_in == bytes_out), ranked by size

**Database Schema**: Assumes table `filter_savings` with columns: `command`, `bytes_in`, `bytes_out`

**Command Normalization**: Extracts base command + optional first subcommand (e.g., "git diff --stat HEAD~3" → "git diff")

---

### 6. **lsp** — Language Server Protocol
**Responsibility**: Start/stop LSP servers (gopls, etc.); manage lifecycle; route requests to appropriate server per file extension.

**Key Types**:
```go
type ServerConfig struct {
    Name       string
    Command    string
    Args       []string
    Extensions []string  // e.g., [".go", ".mod"]
    Env        map[string]string
    RootDir    string    // set at start time
}

type ServerInstance struct {
    Config    ServerConfig
    Process   *exec.Cmd
    Stdin     io.WriteCloser
    Stdout    *bufio.Reader
    Ready     bool
    StartedAt time.Time
    LastUsed  time.Time
    mu        sync.Mutex
    nextID    int  // for LSP message IDs
}

type ServerManager struct {
    mu          sync.RWMutex
    servers     map[string]*ServerInstance   // keyed by name
    configs     map[string]ServerConfig      // keyed by name
    extMap      map[string]string            // extension → server name
    idleTimeout time.Duration                // default 5 minutes
}
```

**Exported Methods**:
- `NewServerManager(cfgs map[string]config.LspServerConfig) *ServerManager` — Create manager
- `(m *ServerManager) HasServers() bool` — Check if servers configured
- `(m *ServerManager) HasConnected() bool` — Check if any running
- `(m *ServerManager) ServerForFile(filePath string) string` — Route by file extension
- `(m *ServerManager) StartServer(ctx context.Context, name, rootDir string) error` — Start server; sends initialize request
- `(m *ServerManager) StopServer(name string) error` — Graceful shutdown (3s timeout)
- `(m *ServerManager) StopAll()` — Stop all running servers
- `(m *ServerManager) GetServer(ctx context.Context, filePath string) (*ServerInstance, error)` — Get/start server for file; auto-starts if needed
- `(m *ServerManager) CleanIdle()` — Stop servers idle >5 minutes
- `(m *ServerManager) Status() map[string]string` — Status of all configured servers
- `(s *ServerInstance) GoToDefinition(filePath string, line, character int) (json.RawMessage, error)`
- `(s *ServerInstance) FindReferences(filePath string, line, character int) (json.RawMessage, error)`
- `(s *ServerInstance) Hover(filePath string, line, character int) (json.RawMessage, error)`
- `(s *ServerInstance) DocumentSymbols(filePath string) (json.RawMessage, error)`

**LSP Protocol**: JSON-RPC 2.0 over stdin/stdout; uses Content-Length header

**Project Root Detection**: Looks for `.git`, `go.mod`, `package.json`, `Cargo.toml`, `pyproject.toml`

**Thread Safety**: RWMutex for manager; mutex for instance

---

### 7. **mcp** — Model Context Protocol
**Responsibility**: Start/stop MCP servers (on-demand); track tool availability; publish connection events.

**Key Types**:
```go
type ServerState struct {
    Name      string
    Config    config.MCPServerConfig
    Client    *tools.MCPClient
    StartedAt time.Time
    LastUsed  time.Time
    ToolCount int
    Status    string  // "running", "stopped", "error"
    Error     string
}

type Manager struct {
    mu       sync.RWMutex
    servers  map[string]*ServerState
    configs  map[string]config.MCPServerConfig
    registry *tools.Registry
    bus      *bus.Bus
    idleTime time.Duration  // default 5 minutes
}
```

**Exported Methods**:
- `NewManager(configs map[string]config.MCPServerConfig, registry *tools.Registry, eventBus *bus.Bus) *Manager` — Create manager
- `(m *Manager) StartServer(ctx context.Context, name string) error` — Start MCP server on-demand
- `(m *Manager) StopServer(name string) error` — Stop server
- `(m *Manager) Status() []ServerState` — Status of all servers
- `(m *Manager) StopIdle()` — Stop servers idle >5 minutes
- `(m *Manager) StopAll()` — Stop all servers

**Event Publishing**: Publishes `EventMCPConnect` / `EventMCPDisconnect` to event bus

**Dependencies**: `tools.MCPClient`, `tools.Registry`, `bus.Bus`, `config.MCPServerConfig`

**Thread Safety**: RWMutex-protected

---

### 8. **memory** — Persistent Cross-Session Memory
**Responsibility**: Store/retrieve memories as markdown files with YAML frontmatter; support multi-scope lookup (agent > project > global); optional FTS indexing.

**Key Types**:
```go
type Entry struct {
    Name        string
    Description string
    Type        string  // TypeUser, TypeFeedback, TypeProject, TypeReference
    Scope       string  // ScopeProject, ScopeGlobal, ScopeAgent
    Facts       []string
    Content     string
    FilePath    string
    Tags        []string
    Concepts    []string
    UpdatedAt   time.Time
    SourceFiles map[string]string  // path → sha256 digest
}

type Store struct {
    dir string
    fts *storage.FTSIndex  // optional
}

type ScopedStore struct {
    project *Store
    global  *Store
    agent   *Store
    fts     *storage.FTSIndex
}
```

**Store Methods**:
- `NewStore(dir string) *Store` — Create store for directory
- `(s *Store) Dir() string` — Get backing directory
- `(s *Store) SetFTS(fts *storage.FTSIndex)` — Attach FTS index
- `(s *Store) Save(entry *Entry) error` — Write entry to disk + update index
- `(s *Store) Load(name string) (*Entry, error)` — Load single entry
- `(s *Store) LoadAll() []*Entry` — Load all entries
- `(s *Store) FindRelevant(context string) []*Entry` — Find relevant entries (text search)
- `(s *Store) Remove(name string) error` — Delete entry
- `(s *Store) AppendFact(name, fact string) error` — Append fact to entry
- `(s *Store) RemoveFact(name string, factIndex int) error` — Remove fact by index
- `(s *Store) ReplaceFact(name string, factIndex int, newFact string) error` — Replace fact

**ScopedStore Methods** (adds scope hierarchy):
- `NewScopedStore(projectDir, globalDir string, db *sql.DB) *ScopedStore` — Create scoped store
- `(s *ScopedStore) SetAgentStore(dir string)` — Set agent-scoped memories
- `(s *ScopedStore) Save(entry *Entry) error` — Write to appropriate scope
- `(s *ScopedStore) LoadAll() []*Entry` — Load all with dedup (agent > project > global)
- `(s *ScopedStore) FindRelevant(context string) []*Entry` — Multi-scope search
- `(s *ScopedStore) FTSSearch(query string, limit int) []*Entry` — BM25 ranked search
- `(s *ScopedStore) BuildIndex(ttlDays int) string` — Rich index with scope headers
- `(s *ScopedStore) SyncFTS()` — Reconcile .md files against FTS meta on startup
- `(s *ScopedStore) Load(name string) (*Entry, error)` — Load from any scope

**Scope Priority**: Agent > Project > Global (higher priority wins on name conflict)

**Memory Extraction** (`extract.go`):
- `BuildExtractorCallback(cfg ExtractorConfig) func(messages []api.Message)` — Create OnTurnEnd callback
- `ExtractFromMessages(client *api.Client, store *ScopedStore, messages []api.Message) int` — Auto-extract memories from conversation (uses Haiku)

**Storage Format**: YAML frontmatter + optional body; Facts stored as YAML list

**Thread Safety**: Mutex-protected in Store; scoped queries use ordering for consistency

---

### 9. **naming** — Session Naming
**Responsibility**: Generate concise 2-5 word session titles from conversation context.

**Exported Functions**:
- `GenerateSessionName(ctx context.Context, client *api.Client, model string, msgs []api.Message) (string, error)` — Call model to produce title

**Process**: 
1. Extract first 10 messages
2. Truncate each to 300 chars
3. Format as "[role]: content" lines
4. Send to model with max 20 tokens
5. Trim quotes/punctuation

---

### 10. **notifications** — Desktop/Terminal Notifications
**Responsibility**: Send notifications via platform-appropriate mechanisms.

**Key Types**:
```go
type Notifier interface {
    Notify(title, body string) error
}

type OSNotifier struct{}        // Native OS notifications
type TerminalNotifier struct{}  // Terminal escape sequences
type MultiNotifier struct {
    notifiers []Notifier
}
type NoopNotifier struct{}      // No-op implementation
```

**Exported Functions**:
- `NewOSNotifier() Notifier` — Create OS notifier (macOS: osascript, Linux: notify-send)
- `NewTerminalNotifier() Notifier` — Create terminal notifier (OSC 9 + bell)
- `NewMultiNotifier(notifiers ...Notifier) Notifier` — Create multi-notifier
- `(n *OSNotifier) Notify(title, body string) error`
- `(t *TerminalNotifier) Notify(title, body string) error`
- `(m *MultiNotifier) Notify(title, body string) error` — Broadcasts to all; returns first error

**Platform Support**: macOS, Linux; others unsupported

---

### 11. **skills** — Skill Registry & Loading
**Responsibility**: Load and manage skill definitions from bundled/user/project/plugin sources.

**Key Types**:
```go
type Skill struct {
    Name        string
    Description string
    Content     string  // prompt/instruction content
    Source      string  // "bundled", "user", "project", "plugin"
    FilePath    string
    SkillDir    string  // directory containing skill file; empty for flat .md files
}

type Registry struct {
    mu     sync.RWMutex
    skills map[string]*Skill
}
```

**Exported Methods**:
- `NewRegistry() *Registry` — Create registry
- `(r *Registry) Register(skill *Skill)` — Add skill
- `(r *Registry) Get(name string) (*Skill, bool)` — Retrieve by name
- `(r *Registry) All() []*Skill` — Get all sorted by name (deterministic for prompt cache)

**Exported Functions**:
- `LoadAll(userSkillsDir, projectSkillsDir string) *Registry` — Load from all sources in order: bundled → user → project

**Thread Safety**: RWMutex-protected; All() returns sorted slice for cache stability

---

### 12. **toolcache** — On-Disk Tool Result Caching
**Responsibility**: Persist oversized tool results to disk; replace in-line with preview + placeholder.

**Key Types**:
```go
type Store struct {
    dir       string
    threshold int  // bytes before persisting (default 50KB)
    mu        sync.Mutex
    index     map[string]string  // tool_use_id → file path
}
```

**Exported Methods**:
- `New(dir string, threshold int) (*Store, error)` — Create store
- `(s *Store) MaybePersist(toolUseID, content string) string` — Persist if >threshold; returns placeholder or original
- `(s *Store) Get(toolUseID string) (string, bool)` — Retrieve persisted result
- `(s *Store) Cleanup()` — Delete all persisted files

**Placeholder Format**:
```
[tool result persisted to disk — N bytes total]
Preview (first 2000 bytes):
<truncated content>
```

**Default Threshold**: 50KB

**Thread Safety**: Mutex-protected

---

## Initialization Patterns

### Typical Service Creation Flow

```go
// Analytics
tracker := analytics.NewTracker(model, maxBudget, saveDir)

// Cache/Diff Tracking
cacheTracker := cachetracker.Tracker{}  // zero-init
diffTracker := difftracker.New()

// Memory (multi-scope)
scopedMem := memory.NewScopedStore(projectMemDir, globalMemDir, db)
scopedMem.SetAgentStore(agentMemDir)
if err := scopedMem.SyncFTS(); err != nil { ... }

// LSP Manager
lspMgr := lsp.NewServerManager(lspConfigs)

// MCP Manager
mcpMgr := mcp.NewManager(mcpConfigs, toolRegistry, eventBus)

// Skills
skillRegistry := skills.LoadAll(userSkillsDir, projectSkillsDir)

// Tool Cache
tcStore, err := toolcache.New(cacheDir, 0)  // use default 50KB threshold

// Filter Savings
filterSvc := filtersavings.NewService(sqliteDB)

// Notifications
notifier := notifications.NewOSNotifier()
// or
notifier := notifications.NewMultiNotifier(
    notifications.NewOSNotifier(),
    notifications.NewTerminalNotifier(),
)

// Compaction
replState := compact.NewReplacementState()  // carry across turns

// Naming (called per-session)
name, err := naming.GenerateSessionName(ctx, apiClient, model, messages)

// Difftracker (called per-turn)
diffTracker.BeforeTurn()
// ... tool execution ...
diff := diffTracker.AfterTurn()
```

---

## Dependencies Graph

### External Dependencies

Each service imports from:

| Service | Imports From |
|---------|--------------|
| **analytics** | stdlib only (`json`, `time`, `sync`, `path/filepath`, `fmt`, `strings`) |
| **cachetracker** | stdlib only (`sync`, `time`, `fmt`) |
| **compact** | `api`, `tools/readcache`, stdlib |
| **difftracker** | stdlib only (`exec`, `strings`, `sync`) |
| **filtersavings** | `storage` (local), stdlib |
| **lsp** | `api`, `config`, stdlib |
| **mcp** | `api` (?), `bus`, `config`, `tools`, stdlib |
| **memory** | `storage` (local), stdlib |
| **naming** | `api`, stdlib |
| **notifications** | stdlib only |
| **skills** | stdlib only |
| **toolcache** | stdlib only |

### Service-to-Service Dependencies

```
compact
  ├── uses toolcache.Store (budget.go line 167)
  └── uses api.Client (for summarization)

memory
  ├── uses storage.FTSIndex (optional, write-through)
  └── uses api.Client (extract.go, for Haiku)

mcp
  ├── uses tools.Registry
  └── uses bus.Bus (for events)

lsp
  ├── uses config.LspServerConfig
  └── uses api (?implicitly through LSP protocol)

filtersavings
  └── uses storage.DB
```

### No Circular Dependencies

Services are purely hierarchical; no cycles.

---

## Thread Safety & Concurrency Patterns

### Mutex-Protected Services

| Service | Type | Scope |
|---------|------|-------|
| **analytics.Tracker** | `sync.Mutex` | Field-level; protects all token/cost fields |
| **cachetracker.Tracker** | `sync.Mutex` | Protects events + tracking state |
| **cachetracker.ExpiryWatcher** | `sync.Mutex` | Protects lastAPICall |
| **difftracker.Tracker** | `sync.Mutex` | Protects diffs + baseline |
| **lsp.ServerManager** | `sync.RWMutex` | Readers for lookup, writers for start/stop |
| **lsp.ServerInstance** | `sync.Mutex` | Protects nextID counter |
| **mcp.Manager** | `sync.RWMutex` | Readers for status, writers for lifecycle |
| **memory.Store** | (none — file-level atomicity) | Assumes OS atomicity |
| **skills.Registry** | `sync.RWMutex` | Readers for Get/All, writers for Register |
| **toolcache.Store** | `sync.Mutex` | Protects index map |

### Safe Patterns

1. **RWMutex for read-heavy services** (lsp, mcp, skills) — allow concurrent reads
2. **Mutex for write-intensive** (analytics, cache tracking) — simpler logic
3. **Zero-alloc types** (difftracker, cachetracker) — can be zero-initialized
4. **Immutable returns** — memory returns copies of slices, not pointers to internal state

---

## Gotchas & Patterns Specific to Services

### 1. **Analytics: TotalTokens vs CumulativeTokens**
- **TotalTokens()** = last API call's context window usage only (input + cacheRead + cacheCreate)
  - Use for: Checking context window percentage
- **CumulativeTokens()** = sum across ALL turns (can be huge)
  - Use for: Cost/billing breakdown, analytics
- **Not the same thing!** Common mistake to confuse them.

### 2. **Cache Tracker: Inferred Reason**
- Detects cache miss reasons by comparing system prompt and message count
- If multiple things changed, only detects ONE reason (in priority order)
- `BreakReasonUnknown` used when cache miss happens but reason can't be inferred

### 3. **Compact: Budget Enforcement is Stateful**
- `ReplacementState` must be **created once per session** and carried through all turns
- Re-applies cached replacements **byte-identically** to preserve prompt cache
- Once a tool_result is "seen", its fate (replaced or frozen) is **permanent** for that turn and all future turns
- Read tool results are **skipped** (not persisted) because model would need Read tool to retrieve them

### 4. **Compact: Pinned Messages**
- `Compact()` can preserve specific message indices verbatim (not summarized)
- Useful for keeping system prompts, initial context, or critical tool outputs
- Only "old" messages (before cutoff) are summarized; recent + pinned are preserved

### 5. **LSP: Idle Timeout & Graceful Shutdown**
- Servers have 3s to exit gracefully before SIGKILL
- Auto-cleaning: `CleanIdle()` stops servers unused >5 minutes
- Root detection walks up directory tree; can start server in wrong root if cwd is ambiguous

### 6. **LSP: Request Routing by Extension**
- Extension → server name mapping is **case-insensitive**
- Extensions normalized to lowercase with "." prefix (e.g., "go" → ".go")
- If multiple servers claim same extension, last one in config wins

### 7. **Memory: Scope Priority is Read-Only**
- Agent > Project > Global is a **read priority**, not a write target
- `Save()` writes to the scope specified in `Entry.Scope` field
- If no scope given, writes to project (or global if no project)
- `LoadAll()` deduplicates by name across scopes; higher priority wins

### 8. **Memory: FTS Sync on Startup**
- `SyncFTS()` reconciles .md files against FTS meta table
- New/modified files are re-indexed
- Orphan FTS rows (file deleted) are removed
- Best-effort operation; errors logged but don't abort

### 9. **Memory Extraction: Haiku Only**
- Auto-extraction uses `claude-haiku-4-5-20251001` (cheap)
- Runs on turn end if enough context (default: ≥4 user turns)
- Avoids duplicating existing memories by name
- Returns count of new memories saved

### 10. **Skills: Deterministic Ordering**
- `Registry.All()` returns **sorted by name** (important for prompt cache stability)
- If you iterate manually, iterate via `All()` not by direct map access
- Bundled skills load first, then user, then project (later sources override earlier)

### 11. **Tool Cache: Threshold Behavior**
- `MaybePersist()` returns original content if **≤ threshold**
- Returns placeholder if **> threshold**
- Placeholder includes file size + first 2KB preview
- If write fails, falls back to inline (truncated)

### 12. **Diff Tracker: Git Integration**
- Shells out to `git diff` and `git diff --stat`
- **No git status** — only tracks tracked files
- Untracked files won't appear in diffs
- Parsing is fragile; handles comments but may miss edge cases

### 13. **Filter Savings: Command Normalization**
- "git diff --stat HEAD~3" → "git diff"
- "/usr/bin/ls -la" → "ls"
- Subcommand inclusion based on second token presence (not a flag)
- Unknown commands treated as single token (no subcommand)

### 14. **Notifications: Platform Fallback**
- macOS: uses `osascript` (built-in)
- Linux: tries `notify-send` (may not be installed)
- MultiNotifier returns first error if any notifier fails
- NoopNotifier useful for tests/no-op mode

### 15. **LSP: Project Root Detection**
- Checks for markers: `.git`, `go.mod`, `package.json`, `Cargo.toml`, `pyproject.toml`
- Walks up directory tree; stops at first match
- Requires at least one marker; otherwise defaults to current dir

---

## Service Integration Example

```go
package main

import (
    "context"
    "github.com/Abraxas-365/claudio/internal/services/analytics"
    "github.com/Abraxas-365/claudio/internal/services/compact"
    "github.com/Abraxas-365/claudio/internal/services/cachetracker"
    "github.com/Abraxas-365/claudio/internal/services/toolcache"
    "github.com/Abraxas-365/claudio/internal/api"
)

func main() {
    // Initialize services
    tracker := analytics.NewTracker("claude-opus-4-5", 10.0, "/tmp/analytics")
    replState := compact.NewReplacementState()
    tcStore, _ := toolcache.New("/tmp/toolcache", 0)
    cacheTracker := cachetracker.Tracker{}

    // Simulate API call
    tracker.RecordUsage(1000, 500, 200, 100)
    tracker.RecordToolCall()
    cacheTracker.Record(100, "system prompt", 5)

    // Check budget
    warning, exceeded := tracker.CheckBudget()
    if exceeded {
        println("Budget exceeded:", warning)
    }

    // Compaction decision
    state := compact.State{
        TotalTokens: tracker.CumulativeTokens(),
        MaxTokens:   200_000,
        ToolCallCount: 1,
    }
    if state.ShouldFullCompact() {
        // Trigger compaction
        messages := []api.Message{} // populated from conversation
        messages = compact.EnforceToolResultBudget(messages, replState, tcStore)
        messages = compact.EnsureToolResultPairing(messages)
    }

    // Report
    println(tracker.Report())
}
```

---

## Summary Table

| Service | Type | Init Pattern | Thread-Safe | Key Responsibility |
|---------|------|--------------|-------------|-------------------|
| **analytics** | Tracker | NewTracker() | Mutex | Token tracking + budget |
| **cachetracker** | Tracker | zero-init | Mutex | Cache miss tracking |
| **compact** | Multiple | NewReplacementState() | n/a | Message summarization + budget |
| **difftracker** | Tracker | New() | Mutex | Git change tracking |
| **filtersavings** | Service | NewService() | DB-level | Filter analytics |
| **lsp** | ServerManager | NewServerManager() | RWMutex | LSP lifecycle |
| **mcp** | Manager | NewManager() | RWMutex | MCP lifecycle |
| **memory** | ScopedStore | NewScopedStore() | Mutex (per Store) | Cross-session memory |
| **naming** | Function | GenerateSessionName() | n/a | Session titling |
| **notifications** | Interface | NewOSNotifier() | n/a | User notifications |
| **skills** | Registry | LoadAll() | RWMutex | Skill management |
| **toolcache** | Store | New() | Mutex | Large tool result caching |

