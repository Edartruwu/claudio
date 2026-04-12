# Code Structure Analysis

## 1. `internal/tui/root.go` — Model struct definition

### Model Struct (Lines 71-189)
```go
type Model struct {
	// Components
	viewport      viewport.Model
	prompt        prompt.Model
	spinner       components.SpinnerModel
	permission    permissions.Model
	palette       commandpalette.Model
	filePicker    filepicker.Model
	modelSelector  modelselector.Model
	agentSelector  agentselector.Model
	teamSelector   teamselector.Model
	teamTemplatesDir string // path to ~/.claudio/team-templates
	currentAgent     string // type of the active persona ("" = default Claudio)
	baseSystemPrompt string // system prompt before any agent persona is applied
	baseModel        string // model before any agent override
	whichKey       whichkey.Model
	sessionPicker  *panelsessions.Panel
	toast          Toast
	todoDock       *docks.TodoDock
	filesPanel     *filespanel.Panel
	fileOps        []filespanel.FileOp
	sidebar        *sidebar.Sidebar
	sidebarFiles   *sidebarblocks.FilesBlock

	// Panels
	activePanel   panels.Panel
	activePanelID PanelID
	lastPanelID   PanelID              // last panel opened, for wl/wv to reopen
	panelPool     map[PanelID]panels.Panel // pooled panel instances; keyed by PanelID

	// State
	messages       []ChatMessage
	focus          Focus
	width, height  int
	streaming      bool
	streamText     *strings.Builder
	model          string
	totalTokens    int
	totalCost      float64
	usageTracker   *api.UsageTracker
	turns          int
	spinText       string // current spinner status text
	toolSpinFrame  int    // braille spinner frame counter for in-progress tool status
	expandedGroups  map[int]bool          // tool group msg indices that are expanded
	thinkingExpanded map[int]bool         // message index → thinking block expanded state
	thinkingHidden   bool                 // /thinking toggle: hide all MsgThinking blocks
	undoStash        []ChatMessage        // last user+assistant exchange popped by /undo, restored by /redo
	lastToolGroup   int                   // msg index of the last tool group start (-1 = none)
	toolStartTimes  map[string]time.Time  // ToolUseID → execution start time
	km              *keymap.Keymap // remappable key bindings
	leaderSeq       string       // leader key sequence in progress ("", "pending", "w", "b", "i", ",")
	leaderSeqGen    int          // incremented each time a new timeout is scheduled; stale TimeoutMsgs are ignored
	prevSessionID   string       // for alternate session switching
	vpCursor        int          // viewport section cursor (-1 = none)
	vpSections      []Section    // cached section metadata from last render
	messageQueue    []string     // messages queued while streaming

	// Viewport search
	vpSearchActive  bool     // true when search input is shown
	vpSearchQuery   string   // current search text
	vpSearchMatches []int    // section indices that match
	vpSearchIdx     int      // current match index in vpSearchMatches

	// Message pinning — maps ChatMessage index to pinned state
	pinnedMsgIndices map[int]bool

	// Concurrent session runtimes — keeps background sessions alive
	sessionRuntimes map[string]*SessionRuntime

	// Per-window session state — enables independent buffers per pane.
	// mainWindow tracks the session shown in the main viewport (left side).
	// rightWindow tracks the session shown in the conversation mirror panel (right side).
	// When rightWindow.sessionID == "" or matches mainWindow.sessionID, the panel mirrors
	// the main viewport content (backward-compatible default).
	mainWindow  WindowState
	rightWindow WindowState

	// App context for panels
	appCtx *AppContext

	// Engine integration
	engine                *query.Engine
	engineRef             **query.Engine // optional external pointer updated whenever engine is set
	pendingEngineMessages []api.Message
	apiClient             *api.Client
	registry     *tools.Registry
	cancelFunc   context.CancelFunc
	eventCh      chan tuiEvent
	approvalCh   chan bool
	systemPrompt   string
	userContext    string // CLAUDE.md injected as first user message
	systemContext  string // git status appended to system prompt
	commands       *commands.Registry
	session        *session.Session
	db             *storage.DB // for sub-agent persistence
	skills         *skills.Registry
	engineConfig   *query.EngineConfig
	planModeActive      bool   // true while the AI is in plan mode (EnterPlanMode called)
	planFilePath        string // path of the current plan file (set by EnterPlanMode)
	planApprovalCursor  int    // selected option in the plan approval dialog (0-3)
	tooSmall            bool   // true if terminal is too small (< 60×20)

	// Rate limit state
	rateLimitWarning string
	rateLimitError   string
	isUsingOverage   bool

	askUserDialog *askUserDialogState // active AskUser question dialog (nil = not showing)

	pendingModelRestore  string // non-empty = restore this model after current interaction finishes
	resumeSummarySet     bool   // true once the resumed session summary has been appended to systemPrompt

	// Agent detail overlay
	agentDetail *agentDetailOverlay
	prevFocus   Focus // saved focus before opening agent detail

	// Welcome screen logo animation
	logoFrame int // increments on each logoTickMsg to drive the color-wave animation
}
```

### WindowState struct (Lines 64-68)
```go
type WindowState struct {
	sessionID string        // session displayed in this window
	messages  []ChatMessage // rendered messages for this window (only used by right window)
	title     string        // cached session title for display
}
```

---

## 2. handleCommand Method (Lines 2400-2853)

**Location**: Lines 2400-2853

**Key control flow**:

1. **Lines 2400-2415**: Model shortcut commands (`/sonnet`, `/opus`, `/haiku`)
   - Resolves shortcut to actual modelID
   - Sets `m.pendingModelRestore` to restore model after interaction
   - Calls `m.handleSubmit(args)` with args

2. **Lines 2417-2432**: `/model` command — interactive model selector
   - Builds extra models from provider shortcuts
   - Sets focus to `FocusModelSelector`
   - Returns with no command

3. **Lines 2435-2459**: Buffer/agent/team commands (`:ls`, `/buffers`, `/agent`, `/team`)
   - `:ls` / `:buffers` → `m.showBufferList()`
   - `/agent` → `agentselector.New()`, sets `FocusAgentSelector`
   - `/team` → `teamselector.New()`, sets `FocusTeamSelector`

4. **Lines 2462-2465**: `/agui` — agent inspector panel
   - Calls `m.togglePanel(PanelAgentGUI)`

5. **Lines 2467-2493**: `/compact` — direct handling
   - Checks for `m.engine`, gets messages
   - Builds pinned indices from ChatMessages
   - Runs `compact.Compact()` in background
   - Returns `compactDoneMsg` via tea.Cmd

6. **Lines 2495-2521**: `/memory extract` — direct handling
   - Checks `m.appCtx.Memory`, `m.engine`
   - Calls `memory.ExtractFromMessages()`
   - Adds system message with count

7. **Lines 2523-2533**: `/vim` — toggle directly
   - Calls `m.prompt.ToggleVim()`
   - Adds system message confirming state

8. **Lines 2535-2580**: `/gain` and `/discover` — filter savings stats
   - Accesses `m.appCtx.FilterSavings`
   - Formats stats into system message

9. **Lines 2613-2645**: Keymap commands — `/map`, `/unmap`, `/maps`
   - Manipulates `m.km` (keymap.Keymap)

10. **Lines 2675-2850**: Generic command registry lookup & action parsing
    - Looks up command in `m.commands` registry (line 2675)
    - Executes command (line 2682)
    - Handles special action prefixes:
      - `[action:clear]` (line 2703) — clears messages, engine history
      - `[action:details]` (line 2717) — toggles tool group expansion
      - `[action:thinking]` (line 2740) — hides/shows thinking blocks
      - `[action:editor]` (line 2750) — opens external editor
      - `[action:undo]` (line 2755) — pops trailing exchange into `m.undoStash`
      - `[action:redo]` (line 2781) — restores from `m.undoStash`
      - `[team:PROMPT]` (line 2796) — team invocation wrapper
      - `[skill:NAME]` (line 2829) — skill content invocation

11. **Lines 2848-2850**: Default system message display
    - `m.addMessage(ChatMessage{Type: MsgSystem, Content: output})`

---

## 3. Session ID Access Patterns

### Via `m.session` (Primary)
- **Type**: `*session.Session`
- **Get current session ID**: 
  ```go
  if m.session != nil && m.session.Current() != nil {
      sessionID = m.session.Current().ID
  }
  ```
- **Usage locations**:
  - Line 1720: Getting sessionID for team creation
  - Line 1842: Getting sessionID for team context
  - Line 1998: Setting `m.engineConfig.SessionID`
  - Line 2046: Passing to `tools.WithSubAgentDB()`

### Via `m.mainWindow.sessionID` (Per-window state)
- **Type**: `string`
- **Used for**: Tracking which session is displayed in main viewport vs right panel
- **Related**: `m.rightWindow.sessionID` (line 3604)

### Via `m.appCtx.Session` (Via AppContext)
- **Type**: `*session.Session`
- **Wired in at startup via `WithAppContext()` option**
- **Alternative path to access sessions through app context**

---

## 4. System Messages Display Pattern

### Adding System Messages
```go
// Line 5206-5210
func (m *Model) addMessage(msg ChatMessage) {
	m.messages = append(m.messages, msg)
	// Persist to DB
	m.persistMessage(msg)
}
```

### Examples of system message additions (MsgSystem type):
- Line 2406: `"Usage: /%s <your question>"`
- Line 2413: `"Using %s for this message"`
- Line 2515: `"No new memories extracted from this conversation."`
- Line 2517: `"Extracted %d memory(ies) from this conversation."`
- Line 2527-2529: Vim mode enabled/disabled
- Line 2577, 2608, 2670: Stats and results formatting
- Line 2736: `"Tool details: " + label`
- Line 2746: `"Thinking blocks: " + label`
- Line 2766-2777: Undo/redo messages
- Line 2848: Default command output display

### ChatMessage struct fields (relevant)
```go
type ChatMessage struct {
	Type      MessageType // MsgSystem, MsgUser, MsgAssistant, MsgError, etc.
	Content   string      // Display text
	ToolName  string      // For tool-related messages
	ToolUseID string      // For tool execution tracking
	// ... other fields
}
```

### Persistence behavior (Lines 5212-5235)
- Only `MsgUser`, `MsgAssistant`, `MsgToolUse`, `MsgToolResult` are persisted to DB
- `MsgSystem`, `MsgError`, `MsgThinking` messages are **NOT persisted** (line 5228)
- Persists via `m.session.Current()` (line 5213)

---

## 5. `internal/web/server.go`

### Server struct definition (Lines 34-47)
```go
type Server struct {
	config      Config
	mux         *http.ServeMux
	sessions    *SessionManager
	skills      *skills.Registry
	db          *storage.DB
	teams       *teams.Manager
	tokens      map[string]time.Time // auth token -> expiry
	mu          sync.RWMutex
	ProjectPath string
	SessionID   string // the one session this server instance owns
	AgentType   string // optional agent type from CLI flag
	TeamTemplate string // optional team template from CLI flag
}
```

### New() constructor (Lines 49-76)
```go
func New(cfg Config, skillsRegistry *skills.Registry) *Server {
	// Open the shared global DB so web sessions are persisted alongside CLI sessions.
	db, err := storage.Open(config.GetPaths().DB)
	if err != nil {
		log.Printf("Warning: failed to open DB for session persistence: %v", err)
	}

	paths := config.GetPaths()
	teamMgr := teams.NewManager(paths.Home+"/teams", paths.TeamTemplates)

	projectPath, _ := os.Getwd()

	s := &Server{
		config:       cfg,
		mux:          http.NewServeMux(),
		sessions:     NewSessionManager(db),
		skills:       skillsRegistry,
		db:           db,
		teams:        teamMgr,
		tokens:       make(map[string]time.Time),
		ProjectPath:  projectPath,
		AgentType:    cfg.Agent,
		TeamTemplate: cfg.Team,
	}
	s.registerRoutes()
	return s
}
```

### Start() method (Lines 78-119)
```go
func (s *Server) Start() error {
	host := s.config.Host
	if host == "" {
		host = "127.0.0.1"
	}
	
	// Use net.Listen to get actual bound port when port is 0 (random)
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, s.config.Port))
	if err != nil {
		return err
	}
	
	// Get the actual port that was assigned
	actualPort := listener.Addr().(*net.TCPAddr).Port
	
	// Auto-create the single session for this server instance
	sess, err := s.sessions.Create(s.ProjectPath, "Session 1")
	if err != nil {
		listener.Close()
		return err
	}
	s.SessionID = sess.ID
	
	// Store agent/team info on the session if provided
	if s.AgentType != "" {
		sess.AgentType = s.AgentType
	}
	if s.TeamTemplate != "" {
		sess.TeamTemplate = s.TeamTemplate
	}
	
	srv := &http.Server{
		Handler:      s.mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // SSE needs no write timeout
		IdleTimeout:  120 * time.Second,
	}
	
	fmt.Printf("claudio web → http://localhost:%d\n", actualPort)
	return srv.Serve(listener)
}
```

### Session initialization in Start()
- Line 95: `s.sessions.Create(s.ProjectPath, "Session 1")` — creates default session
- Line 100: `s.SessionID = sess.ID` — stores session ID on server instance
- Lines 103-108: Stores optional `AgentType` and `TeamTemplate` on the session

---

## 6. `internal/web/sessions.go` — SessionManager.Get() (Lines 127-150)

```go
func (sm *SessionManager) Get(sessionID string) *ProjectSession {
	sm.mu.RLock()
	sess := sm.sessions[sessionID]
	sm.mu.RUnlock()
	if sess != nil {
		return sess
	}

	// Try loading from DB
	if sm.db == nil {
		return nil
	}
	dbSess, err := sm.db.GetSession(sessionID)
	if err != nil || dbSess == nil {
		return nil
	}
	loaded, err := sm.loadDBSession(dbSess)
	if err != nil {
		return nil
	}
	sm.mu.Lock()
	sm.sessions[loaded.ID] = loaded
	sm.mu.Unlock()
	return loaded
}
```

### Load flow:
1. **Line 128-130**: Check in-memory sessions cache (`sm.sessions` map)
2. **Line 131-133**: If not cached, try loading from DB via `sm.db.GetSession(sessionID)`
3. **Line 143-149**: Load from DB entry, cache it, return
4. **Fallback**: Return `nil` if not found anywhere

---

## 7. `internal/cli/commands/core.go` — RegisterCoreCommands

### Function signature (Line 16)
```go
func RegisterCoreCommands(r *Registry, deps *CommandDeps) {
```

### Registration pattern (used throughout, e.g., lines 17-24)
```go
r.Register(&Command{
	Name:        "help",
	Aliases:     []string{"h", "?"},
	Description: "Show available commands",
	Execute: func(args string) (string, error) {
		return r.HelpText(), nil
	},
})
```

### Key command registrations:
- **help** (line 17): Show available commands
- **clear** (line 26): Clear screen
- **agent** (line 34): Switch agent persona
- **model** (line 42): Show/change AI model
- **compact** (line 59): Compact conversation history
- **cost** (line 80): Show session cost
- **memory** (line 90): Memory management (`/memory extract`)
- **session** (line 117): List/manage sessions
- **config** (line 143): View/edit configuration
- **commit** (line 268): Create git commit
- **diff** (line 276): Show git diff
- **status** (line 309): Show git status
- **doctor** (line 324): Diagnose environment
- **vim** (line 332): Toggle vim keybindings
- **rename** (line 361): Rename session
- **skills** (line 383): List available skills
- **new** (line 405): Start new session
- **team** (line 577): Use agent teams
- And many more (through line 691)

### All commands share:
- **Name**: Command identifier (e.g., "help")
- **Aliases**: Short forms (e.g., ["h", "?"])
- **Description**: Help text
- **Execute(args string)**: Closure that receives remaining arguments after command name, returns (output string, error)

### Full file span: Lines 1-691

---

## AppContext struct (from `internal/tui/context.go`, Lines 21-35)

```go
type AppContext struct {
	Session     *session.Session
	Memory      *memory.ScopedStore
	Config      *config.Settings
	Analytics     *analytics.Tracker
	FilterSavings *filtersavings.Service
	Learning      *learning.Store
	TaskRuntime *tasks.Runtime
	DB          *storage.DB
	Hooks       *hooks.Manager
	Rules       *rules.Registry
	Auditor     *security.Auditor
	TeamManager *teams.Manager
	TeamRunner  *teams.TeammateRunner
}
```

**Wired into Model via**: `WithAppContext(ctx *AppContext)` option function (line 38-40)
