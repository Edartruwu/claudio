package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/agents"
	"github.com/Abraxas-365/claudio/internal/prompts"
	"github.com/Abraxas-365/claudio/internal/snippets"
	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/cli/commands"
	"github.com/Abraxas-365/claudio/internal/ratelimit"
	"github.com/Abraxas-365/claudio/internal/config"
	"github.com/Abraxas-365/claudio/internal/query"
	"github.com/Abraxas-365/claudio/internal/services/compact"
	"github.com/Abraxas-365/claudio/internal/services/memory"
	"github.com/Abraxas-365/claudio/internal/services/naming"
	"github.com/Abraxas-365/claudio/internal/services/skills"
	"github.com/Abraxas-365/claudio/internal/session"
	"github.com/Abraxas-365/claudio/internal/storage"
	"github.com/Abraxas-365/claudio/internal/tasks"
	"github.com/Abraxas-365/claudio/internal/tools"
	"github.com/Abraxas-365/claudio/internal/tui/agentselector"
	"github.com/Abraxas-365/claudio/internal/tui/teamselector"
	"github.com/Abraxas-365/claudio/internal/tui/commandpalette"
	"github.com/Abraxas-365/claudio/internal/tui/components"
	"github.com/Abraxas-365/claudio/internal/tui/filepicker"
	"github.com/Abraxas-365/claudio/internal/tui/modelselector"
	"github.com/Abraxas-365/claudio/internal/tui/panels"
	"github.com/Abraxas-365/claudio/internal/tui/panels/conversationpanel"
	panelsessions "github.com/Abraxas-365/claudio/internal/tui/panels/sessions"
	"github.com/Abraxas-365/claudio/internal/tui/panels/analyticspanel"
	panelconfig "github.com/Abraxas-365/claudio/internal/tui/panels/config"
	"github.com/Abraxas-365/claudio/internal/tui/panels/memorypanel"
	"github.com/Abraxas-365/claudio/internal/tui/panels/skillspanel"
	"github.com/Abraxas-365/claudio/internal/tui/panels/taskspanel"
	"github.com/Abraxas-365/claudio/internal/tui/panels/toolspanel"
	"github.com/Abraxas-365/claudio/internal/tui/panels/stree"
	"github.com/Abraxas-365/claudio/internal/tui/keymap"
	"github.com/Abraxas-365/claudio/internal/tui/panels/whichkey"
	"github.com/Abraxas-365/claudio/internal/utils"
	"github.com/Abraxas-365/claudio/internal/tui/docks"
	"github.com/Abraxas-365/claudio/internal/tui/panels/filespanel"
	"github.com/Abraxas-365/claudio/internal/tui/sidebar"
	sidebarblocks "github.com/Abraxas-365/claudio/internal/tui/sidebar/blocks"
	"github.com/Abraxas-365/claudio/internal/tui/panels/agui"
	"github.com/Abraxas-365/claudio/internal/tui/teampanel"
	"github.com/Abraxas-365/claudio/internal/teams"
	"github.com/Abraxas-365/claudio/internal/tui/permissions"
	"github.com/Abraxas-365/claudio/internal/tui/prompt"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// WindowState holds per-window session state so the main viewport and the
// conversation mirror panel can display different sessions independently.
type WindowState struct {
	sessionID string        // session displayed in this window
	messages  []ChatMessage // rendered messages for this window (only used by right window)
	title     string        // cached session title for display
}

// Model is the root Bubble Tea model for the TUI.
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
	pendingToolCount    int             // tools in-flight this turn (tool_start - tool_end)
	pendingPostToolText *strings.Builder // text_delta buffered while tools are in-flight
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
	baseRegistry *tools.Registry // pristine registry with all tools; used to restore team tools on activation
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

// ToolCallEntry represents a single tool call in the real-time feed.
type ToolCallEntry struct {
	ToolName string // name of the tool
	Input    string // truncated input (≤80 chars)
	Output   string // truncated output (≤120 chars)
	Status   string // "running", "done", "error"
	IsSkill  bool   // true if this is a skill invocation
}

// agentDetailOverlay holds state for the full-screen agent conversation view.
type agentDetailOverlay struct {
	state     *teams.TeammateState
	scroll    int              // vertical scroll offset
	toolCalls []ToolCallEntry // live tool call feed (max 20 entries)
}

// askUserDialogState holds the state for an interactive AskUser question dialog.
type askUserDialogState struct {
	questions    []tools.AskQuestion
	qIdx         int               // current question index
	optCursor    int               // cursor within current question's options
	answers      map[string]string // question label → selected answer
	multiSelected map[int]bool     // for multi_select: which option indices are selected
	responseCh   chan<- tools.AskUserResponse
	freeText     string // typed text when "Other" option is selected
	typingOther  bool   // true when user is typing a custom answer
}

// tuiEvent wraps query engine events for the Bubble Tea message loop.
type tuiEvent struct {
	typ           string
	text          string
	toolUse       tools.ToolUse
	toolUses      []tools.ToolUse          // for "retry" events
	result        *tools.Result
	usage         api.Usage
	err           error
	askUserReq    tools.AskUserRequest     // for "askuser_request" events
	teammateEvent *teams.TeammateEvent     // for "teammate_event" events
}

// Tea messages
type (
	engineEventMsg   tuiEvent
	engineDoneMsg    struct{ err error }
	clipboardImageMsg struct {
		data      string // base64-encoded
		mediaType string
		err       error
	}
	timerTickMsg  struct{}
	logoTickMsg   struct{} // drives the welcome-screen logo color-wave animation
	compactDoneMsg struct {
		compacted []api.Message
		summary   string
		err       error
	}
	planCompactDoneMsg struct {
		compacted []api.Message
		summary   string
		err       error
		submitMsg string // message to submit after compaction
	}
)

// ModelOption configures optional TUI model fields.
type ModelOption func(*Model)

// WithSkills sets the skills registry for skill invocation.
func WithSkills(s *skills.Registry) ModelOption {
	return func(m *Model) { m.skills = s }
}

// WithEngineConfig sets the engine configuration for hooks/analytics/permissions.
func WithEngineConfig(cfg *query.EngineConfig) ModelOption {
	return func(m *Model) { m.engineConfig = cfg }
}

// WithUserContext sets the CLAUDE.md user context message to inject as the first user turn.
func WithUserContext(ctx string) ModelOption {
	return func(m *Model) { m.userContext = ctx }
}

// WithSystemContext sets the git status context appended to the system prompt.
func WithSystemContext(ctx string) ModelOption {
	return func(m *Model) { m.systemContext = ctx }
}

// WithDB sets the storage DB for sub-agent session persistence.
func WithDB(db *storage.DB) ModelOption {
	return func(m *Model) { m.db = db }
}

// WithTeamTemplatesDir sets the directory where team templates are stored.
func WithTeamTemplatesDir(dir string) ModelOption {
	return func(m *Model) { m.teamTemplatesDir = dir }
}

// WithEngineRef provides an external **query.Engine pointer that will be updated
// whenever the TUI creates or reassigns its principal engine. This allows callers
// (e.g. the advisor tool GetMessages callback) to access the live engine.
func WithEngineRef(ref **query.Engine) ModelOption {
	return func(m *Model) { m.engineRef = ref }
}

// New creates a new TUI model.
func New(apiClient *api.Client, registry *tools.Registry, systemPrompt string, sess *session.Session, opts ...ModelOption) Model {
	vp := viewport.New(80, 20)
	vp.SetContent("")

	m := Model{
		viewport:       vp,
		prompt:         prompt.New(),
		spinner:        components.NewSpinner(),
		focus:          FocusPrompt,
		model:          apiClient.GetModel(),
		apiClient:      apiClient,
		registry:       registry,
		eventCh:        make(chan tuiEvent, 64),
		systemPrompt:     systemPrompt,
		baseSystemPrompt: systemPrompt,
		baseModel:        apiClient.GetModel(),
		streamText:          &strings.Builder{},
		pendingPostToolText: &strings.Builder{},
		session:        sess,
		expandedGroups:   make(map[int]bool),
		thinkingExpanded: make(map[int]bool),
		lastToolGroup:    -1,
		lastPanelID:      PanelConfig,
		toolStartTimes:  make(map[string]time.Time),
		vpCursor:        -1,
		km:              loadKeymap(),
		whichKey:        whichkey.New(),
		sessionRuntimes: make(map[string]*SessionRuntime),
		panelPool:       make(map[PanelID]panels.Panel),
	}

	// Apply options
	for _, opt := range opts {
		opt(&m)
	}

	// Keep a pristine base registry (includes team tools). Active registry starts
	// without team tools — they are injected back only when a team template is activated.
	if m.registry != nil {
		m.baseRegistry = m.registry
		m.registry = m.registry.Clone()
		for _, name := range tools.TeamToolNames {
			m.registry.Remove(name)
		}
	}

	// Initialize docks (requires appCtx which is set by WithAppContext option above)
	if m.appCtx != nil {
		m.todoDock = docks.NewTodoDock(m.appCtx.TaskRuntime)
	} else {
		m.todoDock = docks.NewTodoDock(nil)
	}
	m.filesPanel = filespanel.New()

	// Wire keymap into which-key for dynamic binding display
	m.whichKey.SetKeymap(m.km)

	// Initialize sidebar
	m.sidebarFiles = sidebarblocks.NewFilesBlock()
	m.sidebar = m.buildSidebar()

	cmdRegistry := commands.NewRegistry()
	commands.RegisterCoreCommands(cmdRegistry, &commands.CommandDeps{
		GetModel: func() string { return m.model },
		SetModel: func(model string) {
			m.model = model
			apiClient.SetModel(model)
			m.usageTracker = api.NewUsageTracker(model, 0)
		},
		GetThinkingLabel: func() string { return apiClient.ThinkingLabel() },
		Compact: func(keepLast int, instruction string) (string, error) {
			if m.engine == nil {
				return "", fmt.Errorf("no active conversation")
			}
			msgs := m.engine.Messages()
			// Build pinned indices from ChatMessages
			pinned := m.buildPinnedEngineIndices()
			compacted, summary, err := compact.Compact(
				context.Background(), apiClient, msgs, keepLast, instruction, pinned,
			)
			if err != nil {
				return "", err
			}
			m.engine.SetMessages(compacted)
			return summary, nil
		},
		GetTokens: func() int { return m.totalTokens },
		GetCost:   func() float64 { return m.totalCost },
		ListSessions: func(limit int) ([]commands.SessionInfo, error) {
			if sess == nil {
				return nil, fmt.Errorf("no session manager")
			}
			sessions, err := sess.List(limit)
			if err != nil {
				return nil, err
			}
			var infos []commands.SessionInfo
			for _, s := range sessions {
				infos = append(infos, commands.SessionInfo{
					ID:        s.ID,
					Title:     s.Title,
					Model:     s.Model,
					UpdatedAt: s.UpdatedAt.Format("2006-01-02 15:04"),
				})
			}
			return infos, nil
		},
		RenameSession: func(title string) error {
			if sess == nil {
				return fmt.Errorf("no active session")
			}
			return sess.SetTitle(title)
		},
		AutoNameSession: func() (string, error) {
			if sess == nil {
				return "", fmt.Errorf("no active session")
			}
			if m.engine == nil {
				return "", fmt.Errorf("no active conversation")
			}
			msgs := m.engine.Messages()
			if len(msgs) == 0 {
				return "", fmt.Errorf("no messages to name from")
			}
			smallModel := "claude-haiku-4-5-20251001"
			if m.appCtx != nil && m.appCtx.Config != nil && m.appCtx.Config.SmallModel != "" {
				smallModel = m.appCtx.Config.SmallModel
			}
			name, err := naming.GenerateSessionName(context.Background(), apiClient, smallModel, msgs)
			if err != nil {
				return "", err
			}
			if err := sess.SetTitle(name); err != nil {
				return "", err
			}
			return name, nil
		},
		ToggleVim: func() bool {
			m.prompt.ToggleVim()
			return m.prompt.IsVimEnabled()
		},
		NewSession: func() error {
			if sess == nil {
				return fmt.Errorf("no session manager")
			}
			if cur := sess.Current(); cur != nil {
				m.prevSessionID = cur.ID
			}
			if _, err := sess.Start(m.model); err != nil {
				return err
			}
			m.messages = nil
			m.streamText.Reset()
			m.turns = 0
			m.totalTokens = 0
			m.totalCost = 0
			m.usageTracker = api.NewUsageTracker(m.model, 0)
			m.refreshViewport()
			return nil
		},
		ExtractMemories: func() (int, error) {
			if m.engine == nil {
				return 0, fmt.Errorf("no active conversation")
			}
			if m.appCtx == nil || m.appCtx.Memory == nil {
				return 0, fmt.Errorf("memory store not available")
			}
			msgs := m.engine.Messages()
			if len(msgs) == 0 {
				return 0, fmt.Errorf("no messages in conversation")
			}
			count := memory.ExtractFromMessages(m.apiClient, m.appCtx.Memory, msgs)
			return count, nil
		},
		RunDream: func(hint string) (string, error) {
			if sess == nil {
				return "", fmt.Errorf("no session manager")
			}
			if m.appCtx == nil || m.appCtx.TaskRuntime == nil {
				return "", fmt.Errorf("task runtime not available")
			}
			// Get all session messages
			msgs, err := sess.GetMessages()
			if err != nil {
				return "", fmt.Errorf("failed to get messages: %w", err)
			}
			// Filter to user and assistant text messages only (skip tool_use, tool_result)
			var conversation strings.Builder
			for _, msg := range msgs {
				if msg.Type != "" {
					continue // skip tool_use and tool_result
				}
				if msg.Role == "user" {
					conversation.WriteString("User: ")
					conversation.WriteString(msg.Content)
					conversation.WriteString("\n\n")
				} else if msg.Role == "assistant" {
					conversation.WriteString("Assistant: ")
					conversation.WriteString(msg.Content)
					conversation.WriteString("\n\n")
				}
			}
			if conversation.Len() == 0 {
				return "No conversation to consolidate.", nil
			}
			// Get project/memory dirs
			cwd, _ := os.Getwd()
			projectRoot := config.FindGitRoot(cwd)
			memDir := config.ProjectMemoryDir(projectRoot)
			// Spawn dream task
			taskState, err := tasks.SpawnDreamTask(m.appCtx.TaskRuntime, tasks.DreamTaskInput{
				SessionSummary: conversation.String(),
				ProjectDir:     cwd,
				MemoryDir:      memDir,
				RunDream: func(ctx context.Context, prompt string) (string, error) {
					if m.appCtx == nil || m.appCtx.TaskRuntime == nil {
						return "", fmt.Errorf("app context not available")
					}
					smallModel := "claude-haiku-4-5-20251001"
					if m.appCtx != nil && m.appCtx.Config != nil && m.appCtx.Config.SmallModel != "" {
						smallModel = m.appCtx.Config.SmallModel
					}
					var output strings.Builder
					handler := &query.CollectHandler{Builder: &output}
					cwd, _ := os.Getwd()
					engine := query.NewEngineWithConfig(m.apiClient, m.registry, handler, query.EngineConfig{
						Model: smallModel,
					})
					engine.SetSystem("You are a memory consolidation agent for the claudio project at " + cwd + ". " +
						"You have access to the Memory tool (save/append/replace-fact/delete-fact/delete/read/list/search) " +
						"and the Recall tool for semantic search. " +
						"Your job is to review the conversation and keep the memory store accurate, current, and contradiction-free.")
					if runErr := engine.Run(ctx, prompt); runErr != nil {
						return "", runErr
					}
					return output.String(), nil
				},
			})
			if err != nil {
				return "", fmt.Errorf("failed to spawn dream: %w", err)
			}
			return fmt.Sprintf("Dream consolidation started (ID: %s). Consolidating memories in background.", taskState.ID), nil
		},
		ListSkills: func() []commands.SkillInfo {
			if m.skills == nil {
				return nil
			}
			var infos []commands.SkillInfo
			for _, s := range m.skills.All() {
				infos = append(infos, commands.SkillInfo{
					Name:        s.Name,
					Description: s.Description,
				})
			}
			return infos
		},
		ListTeams: func() string {
			if m.appCtx == nil || m.appCtx.TeamManager == nil {
				return ""
			}
			teamsList := m.appCtx.TeamManager.ListTeams()
			if len(teamsList) == 0 {
				return ""
			}
			var sb strings.Builder
			activeTeam := ""
			if m.appCtx.TeamRunner != nil {
				activeTeam = m.appCtx.TeamRunner.ActiveTeamName()
			}
			sb.WriteString("Teams:\n")
			for _, t := range teamsList {
				marker := "  "
				if t.Name == activeTeam {
					marker = "▶ "
				}
				sb.WriteString(fmt.Sprintf("%s%s — %s (%d members)\n", marker, t.Name, t.Description, len(t.Members)))
				for _, mem := range t.Members {
					status := string(mem.Status)
					if m.appCtx.TeamRunner != nil {
						if st, ok := m.appCtx.TeamRunner.GetState(mem.Identity.AgentID); ok {
							status = string(st.GetStatus())
						}
					}
					sb.WriteString(fmt.Sprintf("    • %s [%s]\n", mem.Identity.AgentName, status))
				}
			}
			return strings.TrimRight(sb.String(), "\n")
		},
	})
	m.commands = cmdRegistry

	// Build palette items from registered commands + model shortcuts
	paletteItems := buildPaletteItems(cmdRegistry)
	for shortcut, modelID := range m.apiClient.GetModelShortcuts() {
		paletteItems = append(paletteItems, commandpalette.Item{
			Name:        shortcut,
			Description: fmt.Sprintf("Use %s for next message", modelID),
		})
	}
	m.palette = commandpalette.New(paletteItems)

	// File picker for @ mentions
	cwd, _ := os.Getwd()
	m.filePicker = filepicker.New(cwd)

	// Wire teammate event handler to TUI event channel
	if m.appCtx != nil && m.appCtx.TeamRunner != nil {
		m.appCtx.TeamRunner.SetEventHandler(&tuiTeammateEventHandler{ch: m.eventCh})
	}

	// Wire rate limit listener to TUI event channel
	ratelimit.OnStatusChange(func(limits ratelimit.Limits) {
		m.eventCh <- tuiEvent{typ: "ratelimit_changed"}
	})

	// Initialize main window state from session (may be nil if lazy-created)
	m.syncMainWindowState()

	return m
}

func buildPaletteItems(reg *commands.Registry) []commandpalette.Item {
	cmds := reg.ListCommands()
	items := make([]commandpalette.Item, 0, len(cmds))
	for _, cmd := range cmds {
		items = append(items, commandpalette.Item{
			Name:        cmd.Name,
			Description: cmd.Description,
		})
	}
	return items
}

// Init initializes the model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tea.SetWindowTitle("Claudio"),
		tea.EnableBracketedPaste,
		m.waitForEvent(),
		tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg { return logoTickMsg{} }),
	)
}

func (m Model) waitForEvent() tea.Cmd {
	return func() tea.Msg {
		event, ok := <-m.eventCh
		if !ok {
			return nil
		}
		return engineEventMsg(event)
	}
}

// ── Update ───────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.tooSmall = msg.Width < 60 || msg.Height < 20
		m.palette.SetWidth(m.width)
		m.filePicker.SetWidth(m.width)
		m.modelSelector.SetWidth(m.width)
		m.agentSelector.SetWidth(m.width)
		m.agentSelector.SetHeight(m.height)
		m.layout()
		// Clamp YOffset after viewport height is recalculated to prevent it from pointing past visible area
		if m.viewport.YOffset > m.viewport.TotalLineCount()-m.viewport.Height {
			m.viewport.GotoBottom()
		}
		m.refreshViewport()
		return m, nil

	case prompt.VimEscapeMsg:
		// Vim consumed Escape (Insert→Normal). Don't dismiss anything.
		return m, nil

	case ToastDismissMsg:
		m.toast.Dismiss()
		return m, nil

	case whichkey.TimeoutMsg:
		// Ignore stale timeouts from previous leader sequences.
		if msg.Gen != m.leaderSeqGen {
			return m, nil
		}
		// If the sequence was already fully dispatched, don't show the popup.
		if m.leaderSeq == "" {
			return m, nil
		}
		// Show which-key popup based on current leader sequence, reading from keymap.
		prefix := m.leaderSeq
		if prefix == "" {
			// "pending" was normalised to "" — show top-level bindings
			prefix = ""
		}
		bindings := m.km.BindingsForPrefix(prefix)
		if len(bindings) > 0 {
			wkBindings := make([]whichkey.Binding, len(bindings))
			for i, b := range bindings {
				wkBindings[i] = whichkey.Binding{Key: b.KeySeq, Desc: b.Action.Description}
			}
			m.whichKey.Show(wkBindings)
			m.whichKey.SetWidth(m.width)
		}
		return m, nil

	case tea.KeyMsg:
		// Dismiss which-key popup on any keypress
		if m.whichKey.IsActive() {
			m.whichKey.Hide()
		}
		switch msg.String() {
		case "shift+tab":
			// Cycle permission mode: default → auto → plan → default
			if !m.streaming {
				m.cyclePermissionMode()
				return m, nil
			}
		case "ctrl+c":
			if m.streaming && m.cancelFunc != nil {
				// First Ctrl+C during streaming: cancel and preserve partial response
				m.cancelFunc()
				m.finalizeStreamingMessage()
				m.streaming = false
				m.spinText = ""
				m.spinner.Stop()
				m.prompt.Focus()
				m.focus = FocusPrompt
				if m.pendingModelRestore != "" {
					m.model = m.pendingModelRestore
					m.apiClient.SetModel(m.pendingModelRestore)
					m.pendingModelRestore = ""
				}
				m.addMessage(ChatMessage{Type: MsgSystem, Content: "Cancelled — partial response preserved"})
				m.refreshViewport()
				return m, nil
			}
			// Not streaming: quit
			if m.cancelFunc != nil {
				m.cancelFunc()
			}
			// Kill running teammates before exiting
			if m.appCtx != nil && m.appCtx.TeamRunner != nil {
				m.appCtx.TeamRunner.KillAll()
				m.appCtx.TeamRunner.WaitForAll(3 * time.Second)
			}
			return m, tea.Quit
		case "ctrl+o":
			// In viewport mode, let the viewport handler deal with cursor-aware expansion
			if m.focus == FocusViewport {
				break
			}
			// Outside viewport: toggle the last tool group
			if m.lastToolGroup >= 0 {
				m.expandedGroups[m.lastToolGroup] = !m.expandedGroups[m.lastToolGroup]
				m.refreshViewport()
			}
			return m, nil
		case "ctrl+p":
			// Toggle command palette — inject "/" so updatePaletteState keeps it open
			if m.streaming {
				return m, nil
			}
			if m.palette.IsActive() {
				m.palette.Deactivate()
				m.prompt.SetValue("")
			} else {
				m.filePicker.Deactivate()
				m.prompt.SetValue("/")
				m.prompt.EnterVimInsert()
				m.focus = FocusPrompt
				m.prompt.Focus()
				m.updatePaletteState()
			}
			return m, nil

		case "ctrl+g":
			// Let the plan approval dialog handle ctrl+g itself.
			if m.focus == FocusPlanApproval {
				return m.handlePlanApprovalKey(msg)
			}
			// Open external editor with current prompt content
			if m.focus == FocusPrompt && !m.streaming {
				content := m.prompt.ExpandedValue()
				m.prompt.Blur()
				return m, openExternalEditor(content)
			}
			return m, nil
		case "ctrl+v":
			// Try to paste image from clipboard
			if m.focus == FocusPrompt && !m.streaming {
				return m, func() tea.Msg {
					// Quick check for image, then read if present
					if !HasClipboardImage() {
						return clipboardImageMsg{err: fmt.Errorf("no image")}
					}
					data, mediaType, err := ReadClipboardImage()
					return clipboardImageMsg{data: data, mediaType: mediaType, err: err}
				}
			}
		case "esc":
			// In Insert mode, Escape always goes to Normal (standard vim).
			// Don't let this fall through to the panel close handler.
			if m.prompt.IsVimEnabled() && !m.prompt.IsVimNormal() {
				break // fall through to prompt.Update below, not to panel handler
			}
			// In Normal mode during streaming: do nothing (use Ctrl+C to cancel)
			// This allows navigating (Space+wk, etc.) without killing the stream.
			// Exception: allow esc to close an open side panel even while streaming.
			if m.streaming && m.focus != FocusPanel {
				return m, nil
			}
			if m.filePicker.IsActive() {
				m.filePicker.Deactivate()
				return m, nil
			}
			if m.palette.IsActive() {
				m.palette.Deactivate()
				m.prompt.Reset()
				return m, nil
			}
		}

		// Session picker overlay (Telescope-style)
		if m.sessionPicker != nil && m.sessionPicker.IsActive() {
			cmd, _ := m.sessionPicker.Update(msg)
			if !m.sessionPicker.IsActive() {
				m.sessionPicker = nil
				m.focus = FocusPrompt
				m.prompt.Focus()
			}
			return m, cmd
		}

		// Files panel focus mode
		if m.focus == FocusFiles && m.filesPanel != nil {
			cmd, consumed := m.filesPanel.Update(msg)
			if !consumed || !m.filesPanel.IsActive() {
				m.filesPanel.SetFocused(false)
				m.focus = FocusPrompt
				m.prompt.Focus()
				m.refreshViewport()
			}
			return m, cmd
		}

		// Panel focus mode: delegate all keys to active panel
		if m.focus == FocusPanel && m.activePanel != nil {
			cmd, consumed := m.activePanel.Update(msg)
			if consumed {
				// Check if panel closed itself after consuming the key.
				if !m.activePanel.IsActive() {
					m.closePanel()
				}
				return m, cmd
			}
			// Panel didn't consume — close keys are handled here; everything
			// else falls through to the root key handlers below so that
			// unrecognized keys (e.g. panel resize <,>) still reach root.
			switch msg.String() {
			case "esc", "q":
				m.closePanel()
				return m, nil
			}
			// Fall through: do NOT return here so the key bubbles to root.
		}

		// Viewport focus mode: section-based navigation with cursor
		if m.focus == FocusViewport {
			// Search input mode
			if m.vpSearchActive {
				switch msg.String() {
				case "esc":
					m.vpSearchActive = false
					m.vpSearchQuery = ""
					m.vpSearchMatches = nil
					m.refreshViewport()
					return m, nil
				case "enter":
					m.vpSearchActive = false
					// Keep matches and cursor on first match
					if len(m.vpSearchMatches) > 0 {
						m.vpCursor = m.vpSearchMatches[m.vpSearchIdx]
						m.refreshViewport()
						m.scrollToSection(m.vpCursor)
					}
					return m, nil
				case "backspace":
					if len(m.vpSearchQuery) > 0 {
						m.vpSearchQuery = m.vpSearchQuery[:len(m.vpSearchQuery)-1]
						m.updateSearchMatches()
						m.refreshViewport()
					}
					return m, nil
				default:
					// Only accept printable characters
					if len(msg.String()) == 1 && msg.String()[0] >= 32 {
						m.vpSearchQuery += msg.String()
						m.updateSearchMatches()
						if len(m.vpSearchMatches) > 0 {
							m.vpCursor = m.vpSearchMatches[0]
							m.vpSearchIdx = 0
							m.scrollToSection(m.vpCursor)
						}
						m.refreshViewport()
					}
					return m, nil
				}
			}

			// Leader key sequences
			if m.leaderSeq != "" {
				result, cmd := m.handleLeaderKey(msg.String())
				if result {
					return m, cmd
				}
			}

			maxSection := len(m.vpSections) - 1
			switch msg.String() {
			case "j":
				// Move cursor to next section
				if m.vpCursor < maxSection {
					m.vpCursor++
					m.refreshViewport()
					m.scrollToSection(m.vpCursor)
				}
				return m, nil
			case "k":
				// Move cursor to previous section
				if m.vpCursor > 0 {
					m.vpCursor--
					m.refreshViewport()
					m.scrollToSection(m.vpCursor)
				}
				return m, nil
			case "n":
				// Next search match
				if len(m.vpSearchMatches) > 0 {
					m.vpSearchIdx = (m.vpSearchIdx + 1) % len(m.vpSearchMatches)
					m.vpCursor = m.vpSearchMatches[m.vpSearchIdx]
					m.refreshViewport()
					m.scrollToSection(m.vpCursor)
				}
				return m, nil
			case "N":
				// Previous search match
				if len(m.vpSearchMatches) > 0 {
					m.vpSearchIdx--
					if m.vpSearchIdx < 0 {
						m.vpSearchIdx = len(m.vpSearchMatches) - 1
					}
					m.vpCursor = m.vpSearchMatches[m.vpSearchIdx]
					m.refreshViewport()
					m.scrollToSection(m.vpCursor)
				}
				return m, nil
			case "ctrl+d":
				// Jump 5 sections down
				m.vpCursor += 5
				if m.vpCursor > maxSection {
					m.vpCursor = maxSection
				}
				m.refreshViewport()
				m.scrollToSection(m.vpCursor)
				return m, nil
			case "ctrl+u":
				// Jump 5 sections up
				m.vpCursor -= 5
				if m.vpCursor < 0 {
					m.vpCursor = 0
				}
				m.refreshViewport()
				m.scrollToSection(m.vpCursor)
				return m, nil
			case "G":
				m.vpCursor = maxSection
				m.refreshViewport()
				m.scrollToSection(m.vpCursor)
				return m, nil
			case "g":
				m.vpCursor = 0
				m.refreshViewport()
				m.scrollToSection(m.vpCursor)
				return m, nil
			case "enter", "ctrl+o":
				// Toggle expand/collapse on the tool group at cursor
				if tgIdx := m.sectionToolGroupIdx(m.vpCursor); tgIdx >= 0 {
					m.expandedGroups[tgIdx] = !m.expandedGroups[tgIdx]
					m.refreshViewport()
					m.scrollToSection(m.vpCursor)
				} else if m.vpCursor >= 0 && m.vpCursor < len(m.vpSections) {
					// Toggle thinking block if this section is a MsgThinking message
					msgIdx := m.vpSections[m.vpCursor].MsgIndex
					if msgIdx >= 0 && msgIdx < len(m.messages) && m.messages[msgIdx].Type == MsgThinking {
						m.thinkingExpanded[msgIdx] = !m.thinkingExpanded[msgIdx]
						m.refreshViewport()
						m.scrollToSection(m.vpCursor)
					}
				}
				return m, nil
			case "p":
				// Toggle pin on current section's message
				if m.vpCursor >= 0 && m.vpCursor < len(m.vpSections) {
					msgIdx := m.vpSections[m.vpCursor].MsgIndex
					if msgIdx >= 0 && msgIdx < len(m.messages) {
						m.messages[msgIdx].Pinned = !m.messages[msgIdx].Pinned
						// Also pin the paired tool result if this is a tool use
						if m.messages[msgIdx].Type == MsgToolUse && msgIdx+1 < len(m.messages) && m.messages[msgIdx+1].Type == MsgToolResult {
							m.messages[msgIdx+1].Pinned = m.messages[msgIdx].Pinned
						}
						m.refreshViewport()
					}
				}
				return m, nil
			case "d":
				// Delete the interaction at the cursor (user turn + all responses).
				// This removes the messages from the API context to save tokens.
				if !m.streaming && m.vpCursor >= 0 && m.vpCursor < len(m.vpSections) {
					msgIdx := m.vpSections[m.vpCursor].MsgIndex
					if msgIdx >= 0 && msgIdx < len(m.messages) {
						m.deleteInteraction(msgIdx)
						m.refreshViewport()
						if len(m.vpSections) == 0 {
							m.vpCursor = -1
						} else {
							if m.vpCursor >= len(m.vpSections) {
								m.vpCursor = len(m.vpSections) - 1
							}
							if m.vpCursor >= 0 {
								m.scrollToSection(m.vpCursor)
							}
						}
					}
				}
				return m, nil
			case "/":
				// Enter search mode
				m.vpSearchActive = true
				m.vpSearchQuery = ""
				m.vpSearchMatches = nil
				m.vpSearchIdx = 0
				return m, nil
			case "]b":
				// Next session (same as <Space>bn)
				_, cmd := m.switchSessionRelative(1)
				return m, cmd
			case "[b":
				// Previous session (same as <Space>bp)
				_, cmd := m.switchSessionRelative(-1)
				return m, cmd
			case "]m":
				// Jump to next message section
				if m.vpCursor >= 0 && len(m.vpSections) > 0 {
					// Find next section after current cursor
					for i := m.vpCursor + 1; i < len(m.vpSections); i++ {
						// Any non-tool-group section is a message boundary
						if !m.vpSections[i].IsToolGroup {
							m.vpCursor = i
							m.refreshViewport()
							m.scrollToSection(m.vpCursor)
							return m, nil
						}
					}
				} else if m.vpCursor < 0 && len(m.vpSections) > 0 {
					// Start from first non-tool-group section
					for i := 0; i < len(m.vpSections); i++ {
						if !m.vpSections[i].IsToolGroup {
							m.vpCursor = i
							m.refreshViewport()
							m.scrollToSection(m.vpCursor)
							return m, nil
						}
					}
				}
				return m, nil
			case "[m":
				// Jump to previous message section
				if m.vpCursor > 0 && len(m.vpSections) > 0 {
					// Find previous section before current cursor
					for i := m.vpCursor - 1; i >= 0; i-- {
						// Any non-tool-group section is a message boundary
						if !m.vpSections[i].IsToolGroup {
							m.vpCursor = i
							m.refreshViewport()
							m.scrollToSection(m.vpCursor)
							return m, nil
						}
					}
				} else if m.vpCursor < 0 && len(m.vpSections) > 0 {
					// Start from last non-tool-group section
					for i := len(m.vpSections) - 1; i >= 0; i-- {
						if !m.vpSections[i].IsToolGroup {
							m.vpCursor = i
							m.refreshViewport()
							m.scrollToSection(m.vpCursor)
							return m, nil
						}
					}
				}
				return m, nil
			case "i", "esc", "q":
				m.focus = FocusPrompt
				m.vpCursor = -1
				m.vpSearchActive = false
				m.vpSearchQuery = ""
				m.vpSearchMatches = nil
				m.prompt.Focus()
				m.refreshViewport()
				return m, nil
			case " ":
				m.leaderSeq = "pending"
				m.leaderSeqGen++
				return m, whichkey.ScheduleTimeout(m.leaderSeqGen)
			}
			return m, nil
		}

		// Leader key: <Space> in Normal mode (works with or without text)
		if m.focus == FocusPrompt && m.prompt.IsVimNormal() {
			// Active leader sequence — dispatch to handler
			if m.leaderSeq != "" {
				result, cmd := m.handleLeaderKey(msg.String())
				if result {
					return m, cmd
				}
			}
			// Start leader sequence on Space
			if msg.String() == " " {
				m.leaderSeq = "pending"
				m.leaderSeqGen++
				return m, whichkey.ScheduleTimeout(m.leaderSeqGen)
			}
		}

		// Welcome screen number keys: 1-3 to resume recent sessions (only when prompt is empty)
		if m.focus == FocusPrompt && m.isWelcomeScreen() && m.prompt.Value() == "" {
			switch msg.String() {
			case "1", "2", "3":
				idx := int(msg.String()[0] - '1') // 0, 1, or 2
				if m.session != nil {
					recent, _ := m.session.RecentForProject(3)
					if idx < len(recent) {
						m.doSwitchSession(recent[idx].ID)
						return m, m.resumeStreamingCmds()
					}
				}
			}
		}

		// Viewport scrolling shortcuts: vim Normal mode
		if m.focus == FocusPrompt && m.prompt.IsVimNormal() {
			emptyPrompt := m.prompt.Value() == ""
			atTopLine := m.prompt.CursorLine() == 0
			atBottomLine := m.prompt.CursorLine() >= m.prompt.LineCount()-1

			switch msg.String() {
			case "j":
				if emptyPrompt || atBottomLine {
					m.viewport.ScrollDown(1)
					return m, nil
				}
			case "k":
				if emptyPrompt || atTopLine {
					m.viewport.ScrollUp(1)
					return m, nil
				}
			case "ctrl+d":
				m.viewport.HalfPageDown()
				return m, nil
			case "ctrl+u":
				m.viewport.HalfPageUp()
				return m, nil
			case "G":
				if emptyPrompt {
					m.viewport.GotoBottom()
					return m, nil
				}
			case "g":
				if emptyPrompt {
					m.viewport.GotoTop()
					return m, nil
				}
			}
		}

		// Model selector gets priority when active
		if m.focus == FocusModelSelector {
			var cmd tea.Cmd
			m.modelSelector, cmd = m.modelSelector.Update(msg)
			cmds = append(cmds, cmd)
			return m, tea.Batch(cmds...)
		}

		// Agent selector gets priority when active
		if m.focus == FocusAgentSelector {
			var cmd tea.Cmd
			m.agentSelector, cmd = m.agentSelector.Update(msg)
			cmds = append(cmds, cmd)
			return m, tea.Batch(cmds...)
		}

		// Team selector gets priority when active
		if m.focus == FocusTeamSelector {
			var cmd tea.Cmd
			m.teamSelector, cmd = m.teamSelector.Update(msg)
			cmds = append(cmds, cmd)
			return m, tea.Batch(cmds...)
		}

		// Permission dialog
		if m.focus == FocusPermission {
			var cmd tea.Cmd
			m.permission, cmd = m.permission.Update(msg)
			cmds = append(cmds, cmd)
			return m, tea.Batch(cmds...)
		}

		// Plan approval dialog
		if m.focus == FocusPlanApproval {
			return m.handlePlanApprovalKey(msg)
		}

		// AskUser question dialog
		if m.focus == FocusAskUser {
			return m.handleAskUserKey(msg)
		}

		// Agent detail overlay
		if m.focus == FocusAgentDetail {
			return m.handleAgentDetailKey(msg)
		}

		// Command palette intercepts keys when active
		if m.focus == FocusPrompt && !m.streaming && m.palette.IsActive() {
			if cmd, consumed := m.palette.Update(msg); consumed {
				cmds = append(cmds, cmd)
				return m, tea.Batch(cmds...)
			}
		}

		// File picker intercepts keys when active
		if m.focus == FocusPrompt && !m.streaming && m.filePicker.IsActive() {
			if cmd, consumed := m.filePicker.Update(msg); consumed {
				cmds = append(cmds, cmd)
				return m, tea.Batch(cmds...)
			}
		}

	case prompt.SubmitMsg:
		return m.handleSubmit(msg.Text)

	case commandpalette.SelectMsg:
		m.palette.Deactivate()
		m.prompt.Reset()
		return m.handleCommand(msg.Name, "")

	case commandpalette.CompleteMsg:
		// Tab: insert command into prompt so user can continue typing args
		m.palette.Deactivate()
		m.prompt.SetValue("/" + msg.Name + " ")
		return m, nil

	case filepicker.BrowseDirMsg:
		// User selected a directory: update prompt to browse into it
		val := m.prompt.Value()
		atIdx := strings.LastIndex(val, "@")
		if atIdx >= 0 {
			m.prompt.SetValue(val[:atIdx] + "@" + msg.Query)
		}
		return m, nil

	case filepicker.SelectMsg:
		m.filePicker.Deactivate()
		// Check if selected file is an image
		if IsImageFile(msg.Path) {
			absPath := msg.Path
			if strings.HasPrefix(absPath, "~/") {
				home, _ := os.UserHomeDir()
				absPath = home + absPath[1:]
			} else if !filepath.IsAbs(absPath) {
				cwd, _ := os.Getwd()
				absPath = filepath.Join(cwd, absPath)
			}
			data, mediaType, err := ReadImageFile(absPath)
			if err != nil {
				m.addMessage(ChatMessage{Type: MsgError, Content: "Image: " + err.Error()})
				m.refreshViewport()
			} else {
				val := m.prompt.Value()
				atIdx := strings.LastIndex(val, "@")
				if atIdx >= 0 {
					m.prompt.SetValue(val[:atIdx])
				}
				m.prompt.AddImage(filepath.Base(msg.Path), mediaType, data)
				m.addMessage(ChatMessage{Type: MsgSystem, Content: fmt.Sprintf("📎 Attached %s", filepath.Base(msg.Path))})
				m.refreshViewport()
			}
			return m, nil
		}
		// Regular file: insert path as @mention
		val := m.prompt.Value()
		atIdx := strings.LastIndex(val, "@")
		if atIdx >= 0 {
			m.prompt.SetValue(val[:atIdx] + "@" + msg.Path + " ")
		}
		return m, nil

	case modelselector.ModelSelectedMsg:
		m.focus = FocusPrompt
		m.prompt.Focus()
		m.model = msg.ModelID
		m.apiClient.SetModel(msg.ModelID)
		m.apiClient.SetThinkingMode(msg.ThinkingMode)
		if msg.BudgetTokens > 0 {
			m.apiClient.SetBudgetTokens(msg.BudgetTokens)
		}
		m.apiClient.SetEffortLevel(msg.EffortLevel)
		thinkLabel := m.apiClient.ThinkingLabel()
		effortLabel := m.apiClient.EffortLabel()
		m.addMessage(ChatMessage{Type: MsgSystem, Content: fmt.Sprintf("Model: %s (%s) | Thinking: %s | %s", msg.Label, msg.ModelID, thinkLabel, effortLabel)})
		m.refreshViewport()
		return m, nil

	case modelselector.DismissMsg:
		m.focus = FocusPrompt
		m.prompt.Focus()
		return m, nil

	case agentselector.AgentSelectedMsg:
		m.focus = FocusPrompt
		m.prompt.Focus()
		m.currentAgent = msg.AgentType
		m = m.applyAgentPersona(msg)
		return m, nil

	case agentselector.DismissMsg:
		m.focus = FocusPrompt
		m.prompt.Focus()
		return m, nil

	case teamselector.TeamSelectedMsg:
		m.focus = FocusPrompt
		m.prompt.Focus()
		m = m.applyTeamContext(msg)
		return m, nil

	case teamselector.DismissMsg:
		m.focus = FocusPrompt
		m.prompt.Focus()
		return m, nil

	case permissions.ResponseMsg:
		return m.handlePermissionResponse(msg)

	case clipboardImageMsg:
		if msg.err != nil {
			// "no image" is expected when clipboard has text — only show real errors
			if msg.err.Error() != "no image" {
				m.addMessage(ChatMessage{Type: MsgError, Content: "Clipboard image: " + msg.err.Error()})
				m.refreshViewport()
			}
			return m, nil
		}
		m.prompt.AddImage("clipboard.png", msg.mediaType, msg.data)
		m.addMessage(ChatMessage{Type: MsgSystem, Content: "📎 Image pasted from clipboard"})
		m.refreshViewport()
		return m, nil

	case panelsessions.ResumeSessionMsg:
		// Close picker/panel
		if m.sessionPicker != nil {
			m.sessionPicker.Deactivate()
			m.sessionPicker = nil
		}
		m.closePanel()
		m.focus = FocusPrompt
		m.prompt.Focus()
		m.doSwitchSession(msg.SessionID)
		return m, m.resumeStreamingCmds()

	case skillspanel.InvokeSkillMsg:
		m.closePanel()
		m.addMessage(ChatMessage{Type: MsgSystem, Content: fmt.Sprintf("Running skill: %s", msg.Name)})
		m.refreshViewport()
		return m.handleSubmit(msg.Content)

	case panelsessions.DeleteSessionMsg:
		m.closePanel()
		return m, nil

	case panelconfig.InsertCommandMsg:
		m.closePanel()
		m.focus = FocusPrompt
		m.prompt.Focus()
		m.prompt.SetValue(msg.Command + " ")
		return m, nil

	case panelconfig.ConfigChangedMsg:
		m.applyConfigChange(msg.Key, msg.Value)
		return m, nil

	case memorypanel.EditorDoneMsg:
		// Memory was edited in external editor — refresh panel
		if m.activePanel != nil {
			if mp, ok := m.activePanel.(*memorypanel.Panel); ok {
				mp.Activate() // refreshes entries
			}
		}
		return m, nil

	case memorypanel.NewMemoryMsg:
		// New memory was created via temp file
		if msg.Err == nil && msg.TmpPath != "" {
			data, err := os.ReadFile(msg.TmpPath)
			os.Remove(msg.TmpPath) // cleanup temp file
			if err == nil && len(data) > 0 {
				// Parse the temp file as a memory entry and save it
				content := string(data)
				entry := parseNewMemory(content)
				if entry != nil && m.appCtx != nil && m.appCtx.Memory != nil {
					m.appCtx.Memory.Save(entry)
				}
			}
		}
		// Refresh panel
		if m.activePanel != nil {
			if mp, ok := m.activePanel.(*memorypanel.Panel); ok {
				mp.Activate()
			}
		}
		return m, nil

	case editorFinishedMsg:
		m.focus = FocusPrompt
		m.prompt.Focus()
		if msg.err != nil {
			m.addMessage(ChatMessage{Type: MsgError, Content: "Editor: " + msg.err.Error()})
			m.refreshViewport()
		} else {
			m.prompt.SetValueWithCollapse(msg.content)
		}
		return m, nil

	case filespanel.OpenFileMsg:
		// Close the panel and hand the terminal to the editor.
		if m.filesPanel != nil {
			m.filesPanel.Deactivate()
			m.filesPanel.SetFocused(false)
		}
		m.focus = FocusPrompt
		m.prompt.Blur()
		m.refreshViewport()
		var editorCmd string
		if m.appCtx != nil && m.appCtx.Config != nil {
			editorCmd = m.appCtx.Config.EditorCmd
		}
		return m, openFileInEditor(msg.Path, editorCmd)

	case fileEditorFinishedMsg:
		m.focus = FocusPrompt
		m.prompt.Focus()
		if msg.err != nil {
			m.addMessage(ChatMessage{Type: MsgError, Content: "Editor: " + msg.err.Error()})
			m.refreshViewport()
		}
		return m, nil

	case askUserEditorFinishedMsg:
		m.focus = FocusAskUser
		if msg.err != nil {
			m.addMessage(ChatMessage{Type: MsgError, Content: "Editor: " + msg.err.Error()})
			m.refreshViewport()
		} else if m.askUserDialog != nil {
			m.askUserDialog.freeText = msg.content
			m.askUserDialog.typingOther = true
			m.refreshViewport()
		}
		return m, nil

	case planEditorFinishedMsg:
		// Restore the plan approval dialog after the editor exits.
		m.focus = FocusPlanApproval
		if msg.err != nil {
			m.addMessage(ChatMessage{Type: MsgError, Content: "Editor: " + msg.err.Error()})
			m.refreshViewport()
		}
		return m, m.waitForEvent()

	case editorDoneMsg:
		defer os.Remove(msg.path)
		if msg.err != nil {
			cmd := m.toast.Show("Editor error: " + msg.err.Error())
			return m, cmd
		}
		if !msg.readOnly {
			// Read back content and set as prompt text
			data, err := os.ReadFile(msg.path)
			if err == nil {
				content := strings.TrimRight(string(data), "\n")
				m.prompt.SetValue(content)
				m.focus = FocusPrompt
			}
		}
		return m, nil

	case timerTickMsg:
		if m.streaming {
			m.refreshViewport()
			return m, tea.Tick(time.Second, func(time.Time) tea.Msg { return timerTickMsg{} })
		}
		return m, nil

	case logoTickMsg:
		if m.isWelcomeScreen() {
			m.logoFrame++
			m.refreshViewport()
			return m, tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg { return logoTickMsg{} })
		}
		// Welcome screen is gone — stop ticking; it will be restarted by Init on next launch.
		return m, nil

	case taskspanel.RefreshMsg:
		if m.activePanel != nil {
			if tp, ok := m.activePanel.(*taskspanel.Panel); ok {
				cmd := tp.HandleRefresh()
				m.refreshViewport()
				return m, cmd
			}
		}
		return m, nil

	case teampanel.RefreshMsg:
		if m.activePanel != nil {
			if ap, ok := m.activePanel.(*teampanel.Panel); ok {
				cmd := ap.HandleRefresh()
				return m, cmd
			}
		}
		return m, nil

	case agui.RefreshMsg:
		if m.activePanel != nil {
			if ap, ok := m.activePanel.(*agui.Panel); ok {
				cmd := ap.HandleRefresh()
				return m, cmd
			}
		}
		return m, nil

	case panels.ActionMsg:
		switch msg.Type {
		case "agent_message":
			// Prefill prompt with >>agentname
			if name, ok := msg.Payload.(string); ok {
				m.focus = FocusPrompt
				m.prompt.Focus()
				m.prompt.SetValue(">>" + name + " ")
			}
		case "agent_detail":
			// Open agent detail overlay
			if agentID, ok := msg.Payload.(string); ok {
				return m.openAgentDetail(agentID)
			}
		case "exit_team":
			// Close team panel and return to prompt
			m.closePanel()
		case "agui_toast":
			// Display a short notification from the AGUI panel.
			if text, ok := msg.Payload.(string); ok {
				m.addMessage(ChatMessage{Type: MsgSystem, Content: text})
				m.refreshViewport()
			}
		case "open_in_editor":
			if content, ok := msg.Payload.(string); ok {
				return m, openInEditor(content, true)
			}
		}
		return m, nil

	case engineEventMsg:
		return m.handleEngineEvent(tuiEvent(msg))

	case engineDoneMsg:
		m.streaming = false
		m.spinText = ""
		m.spinner.Stop()
		m.pendingToolCount = 0
		m.pendingPostToolText.Reset()
		m.focus = FocusPrompt
		m.prompt.Focus()
		if msg.err != nil {
			m.addMessage(ChatMessage{Type: MsgError, Content: msg.err.Error()})
		}
		// Restore model if this was a one-shot model override
		if m.pendingModelRestore != "" {
			m.model = m.pendingModelRestore
			m.apiClient.SetModel(m.pendingModelRestore)
			m.pendingModelRestore = ""
		}
		m.refreshViewport()
		// Process queued messages — batch all pending task notifications into one
		if len(m.messageQueue) > 0 {
			var notifications []string
			var others []string
			for _, qm := range m.messageQueue {
				if strings.Contains(qm, "<task-notification>") {
					notifications = append(notifications, qm)
				} else {
					others = append(others, qm)
				}
			}
			m.messageQueue = nil
			if len(notifications) > 0 {
				combined := strings.Join(notifications, "\n\n")
				m.messageQueue = others // re-queue non-notification messages
				return m.handleSubmit(combined)
			}
			next := others[0]
			m.messageQueue = others[1:]
			return m.handleSubmit(next)
		}
		// Keep listening for teammate events even when engine is idle
		return m, m.waitForEvent()

	case compactDoneMsg:
		m.streaming = false
		m.spinText = ""
		m.spinner.Stop()
		if msg.err != nil {
			m.addMessage(ChatMessage{Type: MsgError, Content: fmt.Sprintf("Compaction failed: %v", msg.err)})
		} else if msg.summary == "" {
			m.addMessage(ChatMessage{Type: MsgSystem, Content: "Nothing to compact (conversation too short)."})
		} else {
			m.engine.SetMessages(msg.compacted)
			m.engine.ReInjectCaveman()
			// Persist compacted messages to DB so they survive session resume
			if m.session != nil {
				if err := m.session.PersistCompacted(msg.compacted); err != nil {
					m.addMessage(ChatMessage{Type: MsgError, Content: fmt.Sprintf("Warning: failed to persist compacted messages: %v", err)})
				}
				_ = m.session.SaveSummary(msg.summary)
			}
			m.addMessage(ChatMessage{Type: MsgSystem, Content: fmt.Sprintf("Compacted. Summary:\n%s", msg.summary)})
		}
		m.refreshViewport()
		return m, nil

	case planCompactDoneMsg:
		m.streaming = false
		m.spinText = ""
		m.spinner.Stop()
		if msg.err != nil {
			m.addMessage(ChatMessage{Type: MsgError, Content: fmt.Sprintf("Compaction failed: %v", msg.err)})
			m.refreshViewport()
			return m, nil
		}
		if msg.summary != "" {
			m.pendingEngineMessages = msg.compacted
			m.addMessage(ChatMessage{Type: MsgSystem, Content: "Context cleared. Starting implementation..."})
		}
		m.refreshViewport()
		return m.handleSubmit(msg.submitMsg)
	}

	// Delegate to focused component
	switch m.focus {
	case FocusPrompt:
		var cmd tea.Cmd
		m.prompt, cmd = m.prompt.Update(msg)
		cmds = append(cmds, cmd)
		m.updatePaletteState()
	}

	// Update spinner only while streaming to avoid idle CPU usage (10 FPS tick)
	if m.streaming {
		var spinCmd tea.Cmd
		m.spinner, spinCmd = m.spinner.Update(msg)
		cmds = append(cmds, spinCmd)
		m.toolSpinFrame++
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) updatePaletteState() {
	if m.streaming {
		m.palette.Deactivate()
		m.filePicker.Deactivate()
		return
	}

	val := m.prompt.Value()

	// Command palette: starts with / and no space yet
	if strings.HasPrefix(val, "/") && !strings.Contains(val[1:], " ") {
		m.filePicker.Deactivate()
		query := val[1:]
		if !m.palette.IsActive() {
			m.palette.Activate(query)
		} else {
			m.palette.UpdateQuery(query)
		}
		return
	}
	m.palette.Deactivate()

	// File picker: find the last @ that isn't followed by a completed word
	atIdx := strings.LastIndex(val, "@")
	if atIdx >= 0 {
		after := val[atIdx+1:]
		if !strings.Contains(after, " ") {
			m.filePicker.SetWidth(m.width)
			if !m.filePicker.IsActive() {
				m.filePicker.Activate(after)
			} else {
				m.filePicker.UpdateQuery(after)
			}
			return
		}
	}
	m.filePicker.Deactivate()
}

// applyAgentPersona applies an agent persona to the session:
// - appends the agent's system prompt to the base system prompt
// - overrides the model if the agent specifies one
// - replaces the tool registry filtered by the agent's DisallowedTools
// If called with an empty AgentType the session is reset to the base Claudio persona.
func (m Model) applyAgentPersona(msg agentselector.AgentSelectedMsg) Model {
	// Empty AgentType means "remove agent" — restore base state
	if msg.AgentType == "" {
		m.systemPrompt = m.baseSystemPrompt
		m.model = m.baseModel
		m.apiClient.SetModel(m.baseModel)
		if m.engine != nil {
			m.engine.SetSystem(m.baseSystemPrompt)
		}
		m.addMessage(ChatMessage{Type: MsgSystem, Content: "Agent persona removed — back to default Claudio"})
		m.refreshViewport()
		return m
	}

	// Append agent system prompt on top of the base (not the already-modified one)
	base := m.baseSystemPrompt
	if msg.SystemPrompt != "" {
		base = m.baseSystemPrompt + "\n\n" + msg.SystemPrompt
	}

	// Build filtered registry from the original (not previously filtered) registry
	filtered := m.registry.Clone()
	for _, name := range msg.DisallowedTools {
		filtered.Remove(name)
	}
	registerCapabilityTools(filtered, msg.Capabilities)

	// Apply model override (resolve shortcuts like "sonnet" → full model ID)
	if msg.Model != "" {
		model := msg.Model
		if resolved, ok := m.apiClient.ResolveModelShortcut(model); ok {
			model = resolved
		}
		m.model = model
		m.apiClient.SetModel(model)
	}

	// Propagate to live engine if it already exists
	if m.engine != nil {
		m.engine.SetSystem(base)
		m.engine.SetRegistry(filtered)
	}

	// Store so future engine creation picks it up
	m.systemPrompt = base
	m.registry = filtered

	label := msg.AgentType
	if msg.DisplayName != "" {
		label = msg.DisplayName
	}
	m.addMessage(ChatMessage{Type: MsgSystem, Content: fmt.Sprintf("Agent persona: %s", label)})
	m.refreshViewport()
	return m
}

// applyTeamContext appends a team-context block to the system prompt so the
// active agent always knows which team template to use and what its roster is.
func (m Model) applyTeamContext(msg teamselector.TeamSelectedMsg) Model {
	// Inject team tools into the active registry now that a team context is active.
	if m.baseRegistry != nil {
		for _, name := range tools.TeamToolNames {
			if _, err := m.registry.Get(name); err != nil { // skip if already present
				if t, err2 := m.baseRegistry.Get(name); err2 == nil {
					m.registry.Register(t)
				}
			}
		}
		if m.engine != nil {
			m.engine.SetRegistry(m.registry)
		}
	}
	var block string
	if msg.IsEphemeral {
		block = `## Active Team
An ephemeral team is active. Use TeamCreate to name it, then SpawnTeammate to add members.`
	} else {
		// Auto-instantiate the team so SpawnTeammate works immediately without
		// requiring the model to call InstantiateTeam first.
		if m.appCtx != nil && m.appCtx.TeamManager != nil && m.appCtx.TeamRunner != nil {
			// Eagerly start session so we have a stable ID for the team name.
			if m.session != nil && m.session.Current() == nil {
				m.session.Start(m.model)
				m.syncMainWindowState()
			}
			sessionID := ""
			if m.session != nil && m.session.Current() != nil {
				sessionID = m.session.Current().ID
			}
			teamName := msg.TemplateName
			if sessionID != "" {
				sfx := sessionID
				if len(sfx) > 8 {
					sfx = sfx[:8]
				}
				teamName = msg.TemplateName + "-" + sfx
			}
			if _, err := m.appCtx.TeamManager.CreateTeam(teamName, msg.Description, sessionID, ""); err != nil {
				// Team may already exist; proceed anyway.
				_ = err
			}
			// Pre-register members so their subagent_type is persisted.
			for _, mem := range msg.Members {
				_, _ = m.appCtx.TeamManager.AddMember(teamName, mem.Name, mem.Model, "", mem.SubagentType)
			}
			m.appCtx.TeamRunner.SetActiveTeam(teamName)

			// Record in InstantiateTeamTool so engine.Close() cleans up.
			if m.registry != nil {
				if it, err := m.registry.Get("InstantiateTeam"); err == nil {
					if tool, ok := it.(*tools.InstantiateTeamTool); ok {
						tool.InstantiatedTeam = teamName
					}
				}
			}
		}

		var memberLines []string
		for _, mem := range msg.Members {
			line := fmt.Sprintf("  - %s (%s)", mem.Name, mem.SubagentType)
			if mem.Model != "" {
				line += " model=" + mem.Model
			}
			memberLines = append(memberLines, line)
		}
		roster := strings.Join(memberLines, "\n")
		desc := ""
		if msg.Description != "" {
			desc = "\nDescription: " + msg.Description
		}
		block = fmt.Sprintf(`## Active Team: %s%s
Members:
%s

The team is ready. Use SpawnTeammate to assign tasks to each member.`,
			msg.TemplateName, desc, roster)
	}

	newSystem := m.systemPrompt + "\n\n" + block
	m.systemPrompt = newSystem
	if m.engine != nil {
		m.engine.SetSystem(newSystem)
	}

	label := msg.TemplateName
	if msg.IsEphemeral {
		label = "ephemeral"
	}
	m.addMessage(ChatMessage{Type: MsgSystem, Content: fmt.Sprintf("Team context: %s", label)})
	m.refreshViewport()
	return m
}

// ApplyAgentPersonaAtStartup applies an agent persona at startup, before the engine is running.
// Unlike applyAgentPersona, it does NOT add a system message or refresh viewport (no engine yet).
// The model and system prompt are still updated so they apply when the engine starts.
func (m Model) ApplyAgentPersonaAtStartup(msg agentselector.AgentSelectedMsg) Model {
	// Empty AgentType means "remove agent" — restore base state
	if msg.AgentType == "" {
		m.systemPrompt = m.baseSystemPrompt
		m.model = m.baseModel
		m.apiClient.SetModel(m.baseModel)
		return m
	}

	// Append agent system prompt on top of the base
	base := m.baseSystemPrompt
	if msg.SystemPrompt != "" {
		base = m.baseSystemPrompt + "\n\n" + msg.SystemPrompt
	}

	// Build filtered registry from the original (not previously filtered) registry
	filtered := m.registry.Clone()
	for _, name := range msg.DisallowedTools {
		filtered.Remove(name)
	}
	registerCapabilityTools(filtered, msg.Capabilities)

	// Apply model override (resolve shortcuts like "sonnet" → full model ID)
	if msg.Model != "" {
		model := msg.Model
		if resolved, ok := m.apiClient.ResolveModelShortcut(model); ok {
			model = resolved
		}
		m.model = model
		m.apiClient.SetModel(model)
	}

	// Store so future engine creation picks it up
	m.systemPrompt = base
	m.registry = filtered
	m.currentAgent = msg.AgentType

	return m
}

// registerCapabilityTools adds capability-gated tools to the registry based on
// the active agent's declared capabilities. Called on both startup and agent switch.
// Each capability maps to a set of tools; agents without that capability never see them.
func registerCapabilityTools(registry *tools.Registry, capabilities []string) {
	if slices.Contains(capabilities, "design") {
		paths := config.GetPaths()
		registry.Register(tools.NewBundleMockupTool(paths.Designs))
		// RenderMockup + VerifyMockup added in Sprint 2+3
	}
}

// ApplyTeamContextAtStartup applies team context at startup, before the engine is running.
// Unlike applyTeamContext, it does NOT add a system message or refresh viewport (no engine yet).
// The system prompt is still updated so it applies when the engine starts.
func (m Model) ApplyTeamContextAtStartup(msg teamselector.TeamSelectedMsg, appCtx *AppContext) Model {
	// Inject team tools into the active registry now that a team context is active.
	if m.baseRegistry != nil {
		for _, name := range tools.TeamToolNames {
			if _, err := m.registry.Get(name); err != nil { // skip if already present
				if t, err2 := m.baseRegistry.Get(name); err2 == nil {
					m.registry.Register(t)
				}
			}
		}
		// No engine yet at startup — registry is picked up when the engine is created.
	}
	var block string
	if msg.IsEphemeral {
		block = `## Active Team
An ephemeral team is active. Use TeamCreate to name it, then SpawnTeammate to add members.`
	} else {
		// Auto-instantiate the team so SpawnTeammate works immediately without
		// requiring the model to call InstantiateTeam first.
		if appCtx != nil && appCtx.TeamManager != nil && appCtx.TeamRunner != nil {
			sessionID := ""
			if m.session != nil && m.session.Current() != nil {
				sessionID = m.session.Current().ID
			}
			teamName := msg.TemplateName
			if sessionID != "" {
				sfx := sessionID
				if len(sfx) > 8 {
					sfx = sfx[:8]
				}
				teamName = msg.TemplateName + "-" + sfx
			}
			if _, err := appCtx.TeamManager.CreateTeam(teamName, msg.Description, sessionID, ""); err != nil {
				// Team may already exist; proceed anyway.
				_ = err
			}
			// Pre-register members so their subagent_type is persisted.
			for _, mem := range msg.Members {
				_, _ = appCtx.TeamManager.AddMember(teamName, mem.Name, mem.Model, "", mem.SubagentType)
			}
			appCtx.TeamRunner.SetActiveTeam(teamName)

			// Record in InstantiateTeamTool so engine.Close() cleans up.
			if m.registry != nil {
				if it, err := m.registry.Get("InstantiateTeam"); err == nil {
					if tool, ok := it.(*tools.InstantiateTeamTool); ok {
						tool.InstantiatedTeam = teamName
					}
				}
			}
		}

		var memberLines []string
		for _, mem := range msg.Members {
			line := fmt.Sprintf("  - %s (%s)", mem.Name, mem.SubagentType)
			if mem.Model != "" {
				line += " model=" + mem.Model
			}
			memberLines = append(memberLines, line)
		}
		roster := strings.Join(memberLines, "\n")
		desc := ""
		if msg.Description != "" {
			desc = "\nDescription: " + msg.Description
		}
		block = fmt.Sprintf(`## Active Team: %s%s
Members:
%s

The team is ready. Use SpawnTeammate to assign tasks to each member.`,
			msg.TemplateName, desc, roster)
	}

	newSystem := m.systemPrompt + "\n\n" + block
	m.systemPrompt = newSystem
	return m
}

// ── Handlers ─────────────────────────────────────────────

func (m Model) handleSubmit(text string) (tea.Model, tea.Cmd) {
	m.palette.Deactivate()
	m.filePicker.Deactivate()

	// `:` prefix alias: treat `:command` as `/command`
	if strings.HasPrefix(text, ":") {
		text = "/" + text[1:]
	}

	// Vim-style quit commands — handled before normal command dispatch.
	switch strings.TrimSpace(text) {
	case "/q", "/x", "/wq":
		if m.streaming {
			return m, m.toast.Show("Streaming in progress — use :q! to force quit")
		}
		return m, tea.Quit
	case "/q!":
		return m, tea.Quit
	}

	if cmdName, cmdArgs, isCmd := commands.Parse(text); isCmd {
		return m.handleCommand(cmdName, cmdArgs)
	}

	// Handle >>agent messages
	if strings.HasPrefix(text, ">>") {
		return m.handleAgentMessage(text)
	}

	// If already streaming, enqueue the message for later
	if m.streaming {
		m.messageQueue = append(m.messageQueue, text)
		m.addMessage(ChatMessage{
			Type:    MsgSystem,
			Content: fmt.Sprintf("⏳ Queued: %s", truncateStr(text, 60)),
		})
		m.refreshViewport()
		return m, nil
	}

	// Collect image attachments before clearing them
	var imageBlocks []api.UserContentBlock
	for _, img := range m.prompt.Images() {
		imageBlocks = append(imageBlocks, api.NewImageBlock(img.MediaType, img.Data))
	}
	m.prompt.ClearImages()

	// Extract @file mentions — read file contents and clean the text
	cwd, _ := os.Getwd()
	fileAttachments, cleanedText := ExtractFileAttachments(text, cwd)

	// Show user message
	displayText := cleanedText
	if len(imageBlocks) > 0 {
		displayText = fmt.Sprintf("[%d image(s)] %s", len(imageBlocks), displayText)
	}
	if len(fileAttachments) > 0 {
		var names []string
		for _, att := range fileAttachments {
			name := att.DisplayPath
			if att.LineStart > 0 {
				name += fmt.Sprintf("#L%d-%d", att.LineStart, att.LineEnd)
			}
			names = append(names, name)
		}
		displayText = fmt.Sprintf("[%d file(s): %s] %s", len(fileAttachments), strings.Join(names, ", "), cleanedText)
	}
	// Lazy session creation: create session BEFORE persisting the first message
	if m.session != nil && m.session.Current() == nil {
		m.session.Start(m.model)
		m.syncMainWindowState()
		// No auto-title: session label shows the short hash until the user runs /set-name
	}

	m.addMessage(ChatMessage{Type: MsgUser, Content: displayText})
	m.refreshViewport()

	// Increment inactivity counters for all done agents — this human message
	// counts as a tick for agents that weren't explicitly messaged this turn.
	if m.appCtx != nil && m.appCtx.TeamRunner != nil && m.appCtx.Config != nil {
		m.appCtx.TeamRunner.IncrementInactivity(m.appCtx.Config.GetAgentAutoDeleteAfter())
	}

	m.streaming = true
	m.streamText.Reset()
	m.spinText = "Thinking..."
	m.spinner.Start("Thinking...")

	// Carry over conversation history from the previous engine if not already
	// populated (e.g. by session resume or plan-mode approval).
	if len(m.pendingEngineMessages) == 0 && m.engine != nil {
		m.pendingEngineMessages = m.engine.Messages()
	}

	m.approvalCh = make(chan bool, 1)
	handler := &tuiEventHandler{ch: m.eventCh, approvalCh: m.approvalCh}
	if m.engineConfig != nil {
		if m.session != nil && m.session.Current() != nil {
			m.engineConfig.SessionID = m.session.Current().ID
		}
		m.engine = query.NewEngineWithConfig(m.apiClient, m.registry, handler, *m.engineConfig)
	} else {
		m.engine = query.NewEngine(m.apiClient, m.registry, handler)
	}
	if m.engineRef != nil {
		*m.engineRef = m.engine
	}

	// Wire AskUser tool channels so questions are shown interactively.
	if t, err := m.registry.Get("AskUser"); err == nil {
		if aut, ok := t.(*tools.AskUserTool); ok {
			reqCh := make(chan tools.AskUserRequest, 1)
			respCh := make(chan tools.AskUserResponse, 1)
			aut.RequestCh = reqCh
			aut.ResponseCh = respCh
			// Forward requests to the TUI event loop.
			eventCh := m.eventCh
			go func() {
				for req := range reqCh {
					eventCh <- tuiEvent{typ: "askuser_request", askUserReq: req}
				}
			}()
		}
	}
	if m.systemPrompt != "" {
		m.engine.SetSystem(m.systemPrompt)
	}
	if m.userContext != "" {
		m.engine.SetUserContext(m.userContext)
	}
	if m.appCtx != nil && m.appCtx.Memory != nil {
		ttl := 0 // default: no TTL filtering
		if m.appCtx.Config != nil {
			ttl = m.appCtx.Config.GetMemoryIndexTTLDays()
		}
		idx := m.appCtx.Memory.BuildIndex(ttl)
		if idx != "" {
			m.engine.SetMemoryIndex("## Your Memory Index\n\n" + idx)
		}
		// Wire up the refresh function for post-compaction updates
		m.engine.SetMemoryRefreshFunc(func() string {
			ttl := 0 // default: no TTL filtering
			if m.appCtx.Config != nil {
				ttl = m.appCtx.Config.GetMemoryIndexTTLDays()
			}
			return m.appCtx.Memory.BuildIndex(ttl)
		})
	}
	if m.systemContext != "" {
		m.engine.SetSystemContext(m.systemContext)
	}
	if len(m.pendingEngineMessages) > 0 {
		m.engine.SetMessages(m.pendingEngineMessages)
		m.pendingEngineMessages = nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.cancelFunc = cancel

	// Inject sub-agent observer so agent tool events flow to TUI in real time
	ctx = tools.WithSubAgentObserver(ctx, &tuiSubAgentObserver{ch: m.eventCh})

	// Inject DB + parent session ID for sub-agent persistence
	if m.db != nil && m.session.Current() != nil {
		ctx = tools.WithSubAgentDB(ctx, m.db, m.session.Current().ID, m.model)
	}

	// Strip [Image #N: ...] references from text sent to the API — images
	// are already included as base64 content blocks.
	apiText := prompt.StripImageRefs(cleanedText)

	// Build content blocks: images + file contents + user text
	hasAttachments := len(imageBlocks) > 0 || len(fileAttachments) > 0

	go func() {
		var err error
		if hasAttachments {
			blocks := BuildContentBlocks(apiText, fileAttachments, imageBlocks)
			err = m.engine.RunWithBlocks(ctx, blocks)
		} else {
			err = m.engine.Run(ctx, apiText)
		}
		m.eventCh <- tuiEvent{typ: "done", err: err}
	}()

	timerTick := tea.Tick(time.Second, func(time.Time) tea.Msg { return timerTickMsg{} })
	return m, tea.Batch(m.spinner.Tick(), m.waitForEvent(), timerTick)
}

func (m Model) handleEngineEvent(event tuiEvent) (tea.Model, tea.Cmd) {
	switch event.typ {
	case "text_delta":
		if m.pendingToolCount > 0 {
			// Text arrived while a tool is still executing (Claude emitted text after
			// a tool_use block in the same response). Buffer it and flush once all
			// tool_end events for this turn have been received, so the MsgAssistant
			// always lands AFTER the MsgToolResult instead of between use and result.
			m.pendingPostToolText.WriteString(event.text)
		} else {
			m.streamText.WriteString(event.text)
			m.spinText = "Responding..."
			m.spinner.SetText("Responding...")
			m.updateStreamingMessage()
			m.refreshViewport()
		}

	case "thinking_delta":
		m.spinText = "Thinking deeply..."
		m.spinner.SetText("Thinking deeply...")

	case "approval_needed":
		m.finalizeStreamingMessage()
		m.permission = permissions.New(event.toolUse)
		m.permission.SetWidth(m.width)
		m.focus = FocusPermission
		m.refreshViewport()
		return m, m.waitForEvent()

	case "askuser_request":
		m.finalizeStreamingMessage()
		// Get response channel from the registered AskUserTool.
		var respCh chan tools.AskUserResponse
		if t, err := m.registry.Get("AskUser"); err == nil {
			if aut, ok := t.(*tools.AskUserTool); ok {
				respCh = aut.ResponseCh
			}
		}
		m.askUserDialog = &askUserDialogState{
			questions:     event.askUserReq.Questions,
			qIdx:          0,
			optCursor:     0,
			answers:       make(map[string]string),
			multiSelected: make(map[int]bool),
			responseCh:    respCh,
		}
		m.focus = FocusAskUser
		m.refreshViewport()
		return m, m.waitForEvent()

	case "tool_start":
		m.pendingToolCount++
		m.finalizeStreamingMessage()
		m.spinText = fmt.Sprintf("Using %s...", event.toolUse.Name)
		m.spinner.SetText(m.spinText)

		// Track tool group start index
		msgIdx := len(m.messages)
		prevType := MsgUser // sentinel
		if msgIdx > 0 {
			prevType = m.messages[msgIdx-1].Type
		}
		if prevType != MsgToolUse && prevType != MsgToolResult {
			// This starts a new tool group
			m.lastToolGroup = msgIdx
		}

		m.toolStartTimes[event.toolUse.ID] = time.Now()
		m.addMessage(ChatMessage{
			Type:         MsgToolUse,
			ToolName:     event.toolUse.Name,
			ToolInput:    formatToolSummary(event.toolUse),
			ToolInputRaw: event.toolUse.Input,
			ToolUseID:    event.toolUse.ID,
			DurationMs:   -1,
		})
		m.refreshViewport()

	case "tool_end":
		// Track plan mode state based on EnterPlanMode / ExitPlanMode tool calls.
		switch event.toolUse.Name {
		case "EnterPlanMode":
			m.planModeActive = true
			// Extract the plan file path from the result content.
			if event.result != nil {
				if idx := strings.Index(event.result.Content, "Plan file: "); idx >= 0 {
					rest := event.result.Content[idx+len("Plan file: "):]
					if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
						rest = rest[:nl]
					}
					m.planFilePath = strings.TrimSpace(rest)
				}
			}
		case "ExitPlanMode":
			// Don't flip planModeActive yet — show approval dialog first.
			m.planApprovalCursor = 0

		}

		// Compute execution duration.
		var durationMs int64 = -1
		if start, ok := m.toolStartTimes[event.toolUse.ID]; ok {
			durationMs = time.Since(start).Milliseconds()
			delete(m.toolStartTimes, event.toolUse.ID)
		}

		// Update the tool_use message with the full input and duration.
		for i := len(m.messages) - 1; i >= 0; i-- {
			if m.messages[i].Type == MsgToolUse && m.messages[i].ToolUseID == event.toolUse.ID {
				if event.toolUse.Input != nil && m.messages[i].ToolInputRaw == nil {
					m.messages[i].ToolInputRaw = event.toolUse.Input
					m.messages[i].ToolInput = formatToolSummary(event.toolUse)
				}
				m.messages[i].DurationMs = durationMs
				break
			}
		}
		if event.result != nil {
			m.addMessage(ChatMessage{
				Type:      MsgToolResult,
				Content:   event.result.Content,
				IsError:   event.result.IsError,
				ToolUseID: event.toolUse.ID,
			})
		}

		// Decrement after adding the result. When the last in-flight tool
		// completes, flush any text that was buffered during execution so the
		// MsgAssistant lands after MsgToolResult, not sandwiched between
		// MsgToolUse and MsgToolResult.
		if m.pendingToolCount > 0 {
			m.pendingToolCount--
		}
		if m.pendingToolCount == 0 && m.pendingPostToolText.Len() > 0 {
			m.streamText.WriteString(m.pendingPostToolText.String())
			m.pendingPostToolText.Reset()
			m.finalizeStreamingMessage()
		}
		m.refreshViewport()
		// Track file operations for the files panel and sidebar.
		if ops := filespanel.ExtractFileOps(event.toolUse.Name, event.toolUse.Input); len(ops) > 0 {
			m.fileOps = append(m.fileOps, ops...)
			m.filesPanel.Refresh(m.fileOps)
			if m.sidebarFiles != nil {
				m.sidebarFiles.Refresh(m.fileOps)
			}
		}

	case "ratelimit_changed":
		limits := ratelimit.Current()
		m.rateLimitWarning = ratelimit.GetWarning(limits)
		m.rateLimitError = ratelimit.GetError(limits)
		prevOverage := m.isUsingOverage
		m.isUsingOverage = limits.IsUsingOverage
		// Notify user when transitioning to overage
		if limits.IsUsingOverage && !prevOverage {
			m.addMessage(ChatMessage{Type: MsgSystem, Content: ratelimit.GetUsingOverageText(limits)})
			m.refreshViewport()
		}

	case "turn_complete":
		m.finalizeStreamingMessage()
		if m.usageTracker == nil {
			m.usageTracker = api.NewUsageTracker(m.model, 0)
		}
		m.usageTracker.Add(event.usage)
		m.totalTokens, m.totalCost = m.usageTracker.Snapshot()
		m.turns++

	case "done":
		m.pendingToolCount = 0
		m.pendingPostToolText.Reset()
		m.finalizeStreamingMessage()
		m.streaming = false
		m.spinText = ""
		m.spinner.Stop()
		if event.err != nil && event.err.Error() != "context canceled" {
			m.addMessage(ChatMessage{Type: MsgError, Content: event.err.Error()})
		}

		// Restore model if this was a one-shot model override
		if m.pendingModelRestore != "" {
			m.model = m.pendingModelRestore
			m.apiClient.SetModel(m.pendingModelRestore)
			m.pendingModelRestore = ""
		}

		// If plan mode just exited, show approval dialog instead of returning to prompt.
		if m.planModeActive {
			// Preserve conversation history so it's restored when the plan is approved.
			if m.engine != nil {
				m.pendingEngineMessages = m.engine.Messages()
			}
			m.focus = FocusPlanApproval
			m.planApprovalCursor = 0

			m.refreshViewport()
			return m, m.waitForEvent()
		}

		m.focus = FocusPrompt
		m.prompt.Focus()
		m.refreshViewport()

		// Process queued messages
		if len(m.messageQueue) > 0 {
			next := m.messageQueue[0]
			m.messageQueue = m.messageQueue[1:]
			return m.handleSubmit(next)
		}
		return m, m.waitForEvent()

	case "retry":
		// The engine is silently retrying at escalated max_tokens. Tombstone
		// any tool_start entries for these tool uses so the re-stream renders
		// fresh, complete cards rather than duplicating the partial ones.
		retryIDs := make(map[string]bool, len(event.toolUses))
		for _, tu := range event.toolUses {
			retryIDs[tu.ID] = true
			delete(m.toolStartTimes, tu.ID)
		}
		filtered := m.messages[:0]
		for _, msg := range m.messages {
			if msg.Type == MsgToolUse && retryIDs[msg.ToolUseID] {
				continue // drop the partial tool card
			}
			filtered = append(filtered, msg)
		}
		m.messages = filtered

		// Reset all streaming state so the re-stream starts clean.
		// Without this, pendingToolCount double-counts tool_start events
		// (original + re-stream), causing pendingPostToolText to never flush
		// and pre-tool streamText to duplicate on the second pass.
		m.pendingToolCount = 0
		m.pendingPostToolText.Reset()
		m.streamText.Reset()

		m.spinText = "Retrying with extended output..."
		m.spinner.SetText(m.spinText)
		m.refreshViewport()

	case "subagent_tool_start":
		// Update spinner to show sub-agent activity
		summary := formatToolSummary(event.toolUse)
		label := fmt.Sprintf("Agent → %s %s", event.toolUse.Name, summary)
		m.spinText = label
		m.spinner.SetText(label)

		m.toolStartTimes[event.toolUse.ID] = time.Now()

		// Append as a child of the most recent Agent MsgToolUse
		for i := len(m.messages) - 1; i >= 0; i-- {
			if m.messages[i].Type == MsgToolUse && m.messages[i].ToolName == "Agent" {
				m.messages[i].SubagentTools = append(m.messages[i].SubagentTools, SubagentToolCall{
					ToolName:   event.toolUse.Name,
					Summary:    summary,
					ToolUseID:  event.toolUse.ID,
					DurationMs: -1,
				})
				break
			}
		}
		m.refreshViewport()

	case "subagent_tool_end":
		// Find the most recent Agent MsgToolUse and update the matching sub-agent tool
		if event.result != nil {
			brief := resultBrief(event.result.Content)
			summary := formatToolSummary(event.toolUse)

			var durationMs int64 = -1
			if start, ok := m.toolStartTimes[event.toolUse.ID]; ok {
				durationMs = time.Since(start).Milliseconds()
				delete(m.toolStartTimes, event.toolUse.ID)
			}

			for i := len(m.messages) - 1; i >= 0; i-- {
				if m.messages[i].Type == MsgToolUse && m.messages[i].ToolName == "Agent" {
					subs := m.messages[i].SubagentTools
					for j := len(subs) - 1; j >= 0; j-- {
						// Match by ToolUseID first, fall back to name+pending
						match := subs[j].ToolUseID == event.toolUse.ID
						if !match {
							match = subs[j].ToolName == event.toolUse.Name && subs[j].Result == nil
						}
						if match {
							m.messages[i].SubagentTools[j].Result = &brief
							m.messages[i].SubagentTools[j].IsError = event.result.IsError
							m.messages[i].SubagentTools[j].DurationMs = durationMs
							if summary != "" {
								m.messages[i].SubagentTools[j].Summary = summary
							}
							break
						}
					}
					break
				}
			}
			m.refreshViewport()
		}

	case "error":
		// Finalize any in-progress streaming text so it isn't lost when
		// the "done" event adds the error message after it.
		m.finalizeStreamingMessage()
		m.refreshViewport()

	case "teammate_event":
		if event.teammateEvent != nil {
			panelCmd := m.handleTeammateEvent(*event.teammateEvent)
			m.refreshViewport()

			// Delete team-lead's inbox after consuming — notifications come via the event system
			if mb := m.appCtx.TeamRunner.GetMailbox(); mb != nil {
				mb.ReadUnread("team-lead")
				mb.ClearInbox("team-lead")
			}

			// When an agent completes or fails, notify the AI so it can act on the result
			ev := event.teammateEvent
			if ev.Type == "complete" || ev.Type == "error" {
				// Build task summary for this agent
				taskInfo := ""
				if agentTasks := tools.GlobalTaskStore.ByAssignee(ev.AgentName); len(agentTasks) > 0 {
					taskInfo = "\nAssigned tasks:\n"
					for _, t := range agentTasks {
						taskInfo += fmt.Sprintf("  #%s [%s] %s\n", t.ID, t.Status, t.Subject)
					}
				}

				// Include worktree info if agent has changes
				worktreeInfo := ""
				if ev.WorktreePath != "" {
					worktreeInfo = fmt.Sprintf("\nWorktree with changes: %s (branch: %s)\nTo use these files, copy them from the worktree to the main repo, or run: git merge %s", ev.WorktreePath, ev.WorktreeBranch, ev.WorktreeBranch)
				}

				var notification string
				if ev.Type == "complete" {
					notification = fmt.Sprintf("<task-notification>\nAgent %q in team %q completed.\nResult summary: %s%s%s\nUse the Agents panel or SendMessage to get full details if needed.\n</task-notification>", ev.AgentName, ev.TeamName, summaryFromResult(ev.Text, 15, 600), taskInfo, worktreeInfo)
				} else {
					notification = fmt.Sprintf("<task-notification>\nAgent %q in team %q failed.\nError: %s%s%s\n</task-notification>", ev.AgentName, ev.TeamName, summaryFromResult(ev.Text, 15, 600), taskInfo, worktreeInfo)
				}

				if m.streaming {
					// Engine is running — queue for next turn
					m.messageQueue = append(m.messageQueue, notification)
				} else {
					// Engine is idle — deliver notification immediately
					return m.handleSubmit(notification)
				}
			}

			if panelCmd != nil {
				return m, tea.Batch(m.spinner.Tick(), m.waitForEvent(), panelCmd)
			}
		}
	}

	return m, tea.Batch(m.spinner.Tick(), m.waitForEvent())
}

func (m Model) handleCommand(name, args string) (tea.Model, tea.Cmd) {
	// Model shortcut commands: /sonnet, /opus, /haiku, plus any configured provider shortcuts
	// Temporarily switches the model for just this interaction
	modelID, ok := m.apiClient.ResolveModelShortcut(name)
	if ok {
		if args == "" {
			m.addMessage(ChatMessage{Type: MsgSystem, Content: fmt.Sprintf("Usage: /%s <your question>", name)})
			m.refreshViewport()
			return m, nil
		}
		m.pendingModelRestore = m.model
		m.model = modelID
		m.apiClient.SetModel(modelID)
		m.addMessage(ChatMessage{Type: MsgSystem, Content: fmt.Sprintf("Using %s for this message", modelID)})
		return m.handleSubmit(args)
	}

	// /model without args → interactive selector
	if name == "model" && args == "" {
		// Build extra models from provider shortcuts
		var extraModels []modelselector.ModelOption
		for shortcut, modelID := range m.apiClient.GetModelShortcuts() {
			extraModels = append(extraModels, modelselector.ModelOption{
				Label:       shortcut,
				ID:          modelID,
				Description: fmt.Sprintf("Provider model: %s", modelID),
			})
		}
		m.modelSelector = modelselector.NewWithModels(m.apiClient.GetModel(), m.apiClient.GetThinkingMode(), m.apiClient.GetBudgetTokens(), m.apiClient.GetEffortLevel(), extraModels)
		m.modelSelector.SetWidth(m.width)
		m.focus = FocusModelSelector
		m.prompt.Blur()
		return m, nil
	}

	// :ls / :buffers → show buffer list with markers
	if name == "ls" || name == "buffers" {
		m.showBufferList()
		return m, nil
	}

	// /agent → interactive agent persona picker
	if name == "agent" {
		customDirs := agents.GetCustomDirs()
		m.agentSelector = agentselector.New(m.currentAgent, customDirs...)
		m.agentSelector.SetWidth(m.width)
		m.agentSelector.SetHeight(m.height)
		m.focus = FocusAgentSelector
		m.prompt.Blur()
		return m, nil
	}

	// /team → team template picker
	if name == "team" {
		m.teamSelector = teamselector.New(m.teamTemplatesDir)
		m.teamSelector.SetWidth(m.width)
		m.focus = FocusTeamSelector
		m.prompt.Blur()
		return m, nil
	}

	// /agui (or :AGUI) → open the two-pane agent inspector panel
	if strings.EqualFold(name, "agui") {
		m.togglePanel(PanelAgentGUI)
		return m, nil
	}

	// /compact → handle directly because closures from New() capture a stale m
	if name == "compact" {
		// Default keepLast from config, fallback to 10
		keepLast := 10
		if m.appCtx != nil && m.appCtx.Config != nil {
			keepLast = m.appCtx.Config.GetCompactKeepN()
		}
		// args is treated as an instruction (text focus hint), not a number
		instruction := strings.TrimSpace(args)
		if m.engine == nil {
			m.addMessage(ChatMessage{Type: MsgError, Content: "No active conversation to compact."})
			m.refreshViewport()
			return m, nil
		}
		msgs := m.engine.Messages()
		pinned := m.buildPinnedEngineIndices()
		// Run compaction in background — it makes a blocking API call
		m.streaming = true
		m.spinText = "Compacting..."
		m.refreshViewport()
		return m, func() tea.Msg {
			compacted, summary, err := compact.Compact(
				context.Background(), m.apiClient, msgs, keepLast, instruction, pinned,
			)
			return compactDoneMsg{compacted: compacted, summary: summary, err: err}
		}
	}

	// /memory extract → handle directly because closures from New() capture a stale m
	if name == "memory" && strings.TrimSpace(args) == "extract" {
		if m.engine == nil {
			m.addMessage(ChatMessage{Type: MsgError, Content: "No active conversation."})
			m.refreshViewport()
			return m, nil
		}
		if m.appCtx == nil || m.appCtx.Memory == nil {
			m.addMessage(ChatMessage{Type: MsgError, Content: "Memory store not available."})
			m.refreshViewport()
			return m, nil
		}
		msgs := m.engine.Messages()
		if len(msgs) == 0 {
			m.addMessage(ChatMessage{Type: MsgError, Content: "No messages in conversation."})
			m.refreshViewport()
			return m, nil
		}
		count := memory.ExtractFromMessages(m.apiClient, m.appCtx.Memory, msgs)
		if count == 0 {
			m.addMessage(ChatMessage{Type: MsgSystem, Content: "No new memories extracted from this conversation."})
		} else {
			m.addMessage(ChatMessage{Type: MsgSystem, Content: fmt.Sprintf("Extracted %d memory(ies) from this conversation.", count)})
		}
		m.refreshViewport()
		return m, nil
	}

	// /vim → toggle directly on the live model (closures capture stale copies)
	if name == "vim" {
		m.prompt.ToggleVim()
		if m.prompt.IsVimEnabled() {
			m.addMessage(ChatMessage{Type: MsgSystem, Content: "Vim mode enabled (Esc \u2192 Normal, i \u2192 Insert)"})
		} else {
			m.addMessage(ChatMessage{Type: MsgSystem, Content: "Vim mode disabled"})
		}
		m.refreshViewport()
		return m, nil
	}

	// /gain — show filter savings stats
	if name == "gain" {
		if m.appCtx == nil || m.appCtx.FilterSavings == nil {
			m.addMessage(ChatMessage{Type: MsgError, Content: "Filter savings service not available."})
			m.refreshViewport()
			return m, nil
		}
		svc := m.appCtx.FilterSavings
		ctx := context.Background()
		stats, err := svc.GetStats(ctx)
		if err != nil {
			m.addMessage(ChatMessage{Type: MsgError, Content: fmt.Sprintf("Failed to get stats: %v", err)})
			m.refreshViewport()
			return m, nil
		}
		topCmds, err := svc.GetTopCommands(ctx, 10)
		if err != nil {
			m.addMessage(ChatMessage{Type: MsgError, Content: fmt.Sprintf("Failed to get top commands: %v", err)})
			m.refreshViewport()
			return m, nil
		}
		var sb strings.Builder
		sb.WriteString("Filter Savings Summary\n")
		sb.WriteString(strings.Repeat("─", 38) + "\n")
		sb.WriteString(fmt.Sprintf("Total processed:  %s commands\n", formatInt(stats.RecordCount)))
		sb.WriteString(fmt.Sprintf("Bytes in:         %s\n", formatBytes(stats.TotalBytesIn)))
		saved := stats.TotalBytesIn - stats.TotalBytesOut
		if saved < 0 {
			saved = 0
		}
		sb.WriteString(fmt.Sprintf("Bytes saved:      %s (%.1f%%)\n", formatBytes(saved), stats.SavingsPct))
		if len(topCmds) > 0 {
			sb.WriteString("\nTop commands by savings:\n")
			for _, cs := range topCmds {
				cmdSaved := cs.BytesIn - cs.BytesOut
				if cmdSaved < 0 {
					cmdSaved = 0
				}
				sb.WriteString(fmt.Sprintf("  %-16s %s saved (%.0f%%)   %d runs\n",
					cs.Command, formatBytes(cmdSaved), cs.SavingsPct, cs.Count))
			}
		}
		m.addMessage(ChatMessage{Type: MsgSystem, Content: sb.String()})
		m.refreshViewport()
		return m, nil
	}

	// /discover — show commands without filter coverage
	if name == "discover" {
		if m.appCtx == nil || m.appCtx.FilterSavings == nil {
			m.addMessage(ChatMessage{Type: MsgError, Content: "Filter savings service not available."})
			m.refreshViewport()
			return m, nil
		}
		suggestions, err := m.appCtx.FilterSavings.Discover(context.Background(), 10)
		if err != nil {
			m.addMessage(ChatMessage{Type: MsgError, Content: fmt.Sprintf("Failed to get suggestions: %v", err)})
			m.refreshViewport()
			return m, nil
		}
		if len(suggestions) == 0 {
			m.addMessage(ChatMessage{Type: MsgSystem, Content: "No unfiltered commands found yet. Run some commands and check back!"})
			m.refreshViewport()
			return m, nil
		}
		var sb strings.Builder
		sb.WriteString("Unfiltered Command Opportunities\n")
		sb.WriteString(strings.Repeat("─", 38) + "\n")
		sb.WriteString("Commands seen without filtering applied — adding filters could save tokens:\n\n")
		for _, s := range suggestions {
			sb.WriteString(fmt.Sprintf("  %-14s avg %s/run   %d occurrences\n",
				s.Command, formatBytes(s.AvgBytesIn), s.Occurrences))
		}
		m.addMessage(ChatMessage{Type: MsgSystem, Content: sb.String()})
		m.refreshViewport()
		return m, nil
	}

	// /map <keyseq> <action> — remap a leader key binding
	if name == "map" {
		parts := strings.Fields(args)
		if len(parts) < 2 {
			m.addMessage(ChatMessage{Type: MsgError, Content: "Usage: /map <keyseq> <action-id>"})
			m.refreshViewport()
			return m, nil
		}
		keySeq := parts[0]
		actionID := keymap.ActionID(parts[1])
		if err := m.km.Set(keySeq, actionID); err != nil {
			m.addMessage(ChatMessage{Type: MsgError, Content: fmt.Sprintf("Map error: %s", err)})
			m.refreshViewport()
			return m, nil
		}
		return m, m.toast.Show(fmt.Sprintf("Mapped %s → %s", keySeq, actionID))
	}

	// /unmap <keyseq> — remove a leader key binding
	if name == "unmap" {
		keySeq := strings.TrimSpace(args)
		if keySeq == "" {
			m.addMessage(ChatMessage{Type: MsgError, Content: "Usage: /unmap <keyseq>"})
			m.refreshViewport()
			return m, nil
		}
		if err := m.km.Unset(keySeq); err != nil {
			m.addMessage(ChatMessage{Type: MsgError, Content: fmt.Sprintf("Unmap error: %s", err)})
			m.refreshViewport()
			return m, nil
		}
		return m, m.toast.Show(fmt.Sprintf("Unmapped %s", keySeq))
	}

	// /maps [group] — list all key bindings
	if name == "maps" {
		group := strings.TrimSpace(args)
		bindings := m.km.List(group)
		if len(bindings) == 0 {
			if group != "" {
				m.addMessage(ChatMessage{Type: MsgSystem, Content: fmt.Sprintf("No bindings for group: %s", group)})
			} else {
				m.addMessage(ChatMessage{Type: MsgSystem, Content: "No bindings defined."})
			}
			m.refreshViewport()
			return m, nil
		}
		var sb strings.Builder
		title := "Key Bindings"
		if group != "" {
			title = fmt.Sprintf("Key Bindings [%s]", group)
		}
		sb.WriteString(title + "\n")
		sb.WriteString(strings.Repeat("─", 50) + "\n")
		for _, b := range bindings {
			sb.WriteString(fmt.Sprintf("  %-12s  %-28s  %s\n", b.KeySeq, string(b.Action.ID), b.Action.Description))
		}
		m.addMessage(ChatMessage{Type: MsgSystem, Content: sb.String()})
		m.refreshViewport()
		return m, nil
	}


	cmd, ok := m.commands.Get(name)
	if !ok {
		m.addMessage(ChatMessage{Type: MsgError, Content: fmt.Sprintf("Unknown command: /%s. Type /help for available commands.", name)})
		m.refreshViewport()
		return m, nil
	}

	output, err := cmd.Execute(args)
	if err != nil {
		if err.Error() == "__exit__" {
			// Kill running teammates before exiting
			if m.appCtx != nil && m.appCtx.TeamRunner != nil {
				m.appCtx.TeamRunner.KillAll()
				m.appCtx.TeamRunner.WaitForAll(3 * time.Second)
			}
			return m, tea.Quit
		}
		m.addMessage(ChatMessage{Type: MsgError, Content: err.Error()})
		m.refreshViewport()
		return m, nil
	}

	if output != "" {
		// New session: clear viewport (already done in dep callback)
		if output == "__new_session__" {
			return m, nil
		}
		// Clear: wipe UI messages and engine history
		if output == "[action:clear]" {
			m.messages = nil
			m.streamText.Reset()
			m.turns = 0
			m.totalTokens = 0
			m.totalCost = 0
			m.undoStash = nil
			m.usageTracker = api.NewUsageTracker(m.model, 0)
			if m.engine != nil {
				m.engine.SetMessages(nil)
			}
			m.refreshViewport()
			return m, nil
		}
		if output == "[action:details]" {
			// Toggle expand/collapse for every tool group currently rendered.
			// If any group is collapsed, expand all; otherwise collapse all.
			anyCollapsed := false
			for i, msg := range m.messages {
				if msg.Type == MsgToolUse && !m.expandedGroups[i] {
					anyCollapsed = true
					break
				}
			}
			for i, msg := range m.messages {
				if msg.Type == MsgToolUse {
					m.expandedGroups[i] = anyCollapsed
				}
			}
			label := "collapsed"
			if anyCollapsed {
				label = "expanded"
			}
			m.addMessage(ChatMessage{Type: MsgSystem, Content: "Tool details: " + label})
			m.refreshViewport()
			return m, nil
		}
		if output == "[action:thinking]" {
			m.thinkingHidden = !m.thinkingHidden
			label := "visible"
			if m.thinkingHidden {
				label = "hidden"
			}
			m.addMessage(ChatMessage{Type: MsgSystem, Content: "Thinking blocks: " + label})
			m.refreshViewport()
			return m, nil
		}
		if output == "[action:editor]" {
			content := m.prompt.ExpandedValue()
			m.prompt.Blur()
			return m, openExternalEditor(content)
		}
		if output == "[action:undo]" {
			// Pop the trailing exchange (everything from the last user message to end)
			// into the undo stash so /redo can restore it.
			lastUser := -1
			for i := len(m.messages) - 1; i >= 0; i-- {
				if m.messages[i].Type == MsgUser {
					lastUser = i
					break
				}
			}
			if lastUser < 0 {
				m.addMessage(ChatMessage{Type: MsgSystem, Content: "Nothing to undo"})
				m.refreshViewport()
				return m, nil
			}
			stash := make([]ChatMessage, len(m.messages)-lastUser)
			copy(stash, m.messages[lastUser:])
			m.undoStash = stash
			m.messages = m.messages[:lastUser]
			if m.engine != nil {
				m.engine.SetMessages(engineMessagesFromChat(m.messages))
			}
			m.addMessage(ChatMessage{Type: MsgSystem, Content: "Undid last exchange (use /redo to restore)"})
			m.refreshViewport()
			return m, nil
		}
		if output == "[action:redo]" {
			if len(m.undoStash) == 0 {
				m.addMessage(ChatMessage{Type: MsgSystem, Content: "Nothing to redo"})
				m.refreshViewport()
				return m, nil
			}
			m.messages = append(m.messages, m.undoStash...)
			m.undoStash = nil
			if m.engine != nil {
				m.engine.SetMessages(engineMessagesFromChat(m.messages))
			}
			m.refreshViewport()
			return m, nil
		}
		// Team invocation: intercept [team:PROMPT] and send to engine with team instruction
		if strings.HasPrefix(output, "[team:") && strings.HasSuffix(output, "]") {
			userPrompt := output[6 : len(output)-1]

			// Build context about existing teams
			teamContext := ""
			if m.appCtx != nil && m.appCtx.TeamManager != nil {
				if teams := m.appCtx.TeamManager.ListTeams(); len(teams) > 0 {
					teamContext = "Existing teams:\n"
					for _, t := range teams {
						desc := t.Description
						if desc == "" {
							desc = "no description"
						}
						teamContext += fmt.Sprintf("- %s: %s (%d members)\n", t.Name, desc, len(t.Members))
					}
					teamContext += "\nReuse an existing team if appropriate, or create a new one.\n\n"
				}
			}

			teamInstruction := teamContext + `Use agent teams to accomplish this task. Follow this workflow:

1. If a suitable team already exists, reuse it. Otherwise, create a new team with TeamCreate.
2. Break the work into discrete tasks using TaskCreate — assign each task to an agent name (assigned_to field).
3. Spawn one agent per task using the Agent tool with run_in_background=true. Include the task ID in the agent's prompt so it knows which task it owns.
4. Tasks are auto-completed when agents finish — no manual status updates needed.
5. You will be notified when agents complete. Summarize results for the user.

Task:
` + userPrompt
			return m.handleSubmit(teamInstruction)
		}

		// Skill invocation: intercept [skill:NAME] and send skill content to engine
		if strings.HasPrefix(output, "[skill:") && strings.Contains(output, "]") {
			endIdx := strings.Index(output, "]")
			skillName := output[7:endIdx]
			skillArgs := output[endIdx+1:]
			if m.skills != nil {
				if skill, ok := m.skills.Get(skillName); ok {
					// Send skill content as a user message to the engine
					m.addMessage(ChatMessage{Type: MsgSystem, Content: fmt.Sprintf("Running skill: %s", skill.Name)})
					m.refreshViewport()
					content := strings.ReplaceAll(skill.Content, "$ARGUMENTS", skillArgs)
					return m.handleSubmit(content)
				}
			}
			// Skill not found — show as regular message
			m.addMessage(ChatMessage{Type: MsgError, Content: fmt.Sprintf("Skill %q not found", skillName)})
			m.refreshViewport()
			return m, nil
		}

		m.addMessage(ChatMessage{Type: MsgSystem, Content: output})
		m.refreshViewport()
	}

	return m, nil
}

// handleAskUserKey handles key events in the AskUser question dialog.
func (m Model) handleAskUserKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	d := m.askUserDialog
	if d == nil {
		return m, nil
	}
	q := d.questions[d.qIdx]
	otherIdx := len(q.Options)   // "Other (type your own...)"
	chatIdx := otherIdx + 1      // "Chat about this"

	// If user is currently typing a free-text answer, handle that first.
	if d.typingOther {
		if msg.String() == "ctrl+g" {
			return m, openAskUserEditor(d.freeText)
		}
		switch msg.Type {
		case tea.KeyEnter:
			answer := strings.TrimSpace(d.freeText)
			if answer == "" {
				answer = "Other"
			}
			d.typingOther = false
			d.freeText = ""
			d.answers[q.Label] = answer
			if d.qIdx < len(d.questions)-1 {
				d.qIdx++
				d.optCursor = 0
			} else {
				if d.responseCh != nil {
					d.responseCh <- tools.AskUserResponse{Answers: d.answers}
				}
				m.askUserDialog = nil
				m.focus = FocusPrompt
				m.refreshViewport()
				return m, m.waitForEvent()
			}
		case tea.KeyBackspace, tea.KeyDelete:
			if len(d.freeText) > 0 {
				d.freeText = d.freeText[:len(d.freeText)-1]
			}
		case tea.KeyEsc:
			d.typingOther = false
			d.freeText = ""
			d.optCursor = otherIdx
		default:
			if msg.Type == tea.KeyRunes {
				d.freeText += string(msg.Runes)
			} else if msg.Type == tea.KeySpace {
				d.freeText += " "
			}
		}
		m.refreshViewport()
		return m, nil
	}

	switch msg.String() {
	case "j", "down":
		if d.optCursor < chatIdx {
			d.optCursor++
		}
	case "k", "up":
		if d.optCursor > 0 {
			d.optCursor--
		}
	case "enter", " ":
		switch d.optCursor {
		case otherIdx:
			// Enter inline typing mode.
			d.typingOther = true
			d.freeText = ""
			m.refreshViewport()
			return m, nil
		case chatIdx:
			// "Chat about this" — dismiss dialog, send a sentinel response,
			// and return focus to the prompt so the user can type freely.
			if d.responseCh != nil {
				d.answers[q.Label] = "[user chose to chat about this instead of selecting an option]"
				d.responseCh <- tools.AskUserResponse{Answers: d.answers}
			}
			m.askUserDialog = nil
			m.focus = FocusPrompt
			m.refreshViewport()
			return m, m.waitForEvent()
		default:
			if q.MultiSelect {
				// Toggle selection; submit only on explicit enter with selections made.
				if msg.String() == " " {
					d.multiSelected[d.optCursor] = !d.multiSelected[d.optCursor]
					m.refreshViewport()
					return m, nil
				}
				// Enter = confirm selections (or pick current if none selected).
				var selected []string
				for i, opt := range q.Options {
					if d.multiSelected[i] {
						selected = append(selected, opt)
					}
				}
				if len(selected) == 0 {
					selected = []string{q.Options[d.optCursor]}
				}
				d.answers[q.Label] = strings.Join(selected, ", ")
				d.multiSelected = make(map[int]bool)
			} else {
				d.answers[q.Label] = q.Options[d.optCursor]
			}
			if d.qIdx < len(d.questions)-1 {
				d.qIdx++
				d.optCursor = 0
			} else {
				if d.responseCh != nil {
					d.responseCh <- tools.AskUserResponse{Answers: d.answers}
				}
				m.askUserDialog = nil
				m.focus = FocusPrompt
				m.refreshViewport()
				return m, m.waitForEvent()
			}
		}
	case "esc":
		// Cancel: send empty response.
		if d.responseCh != nil {
			d.responseCh <- tools.AskUserResponse{Answers: make(map[string]string)}
		}
		m.askUserDialog = nil
		m.focus = FocusPrompt
		m.refreshViewport()
		return m, m.waitForEvent()
	}
	m.refreshViewport()
	return m, nil
}

// planApprovalOptionCount returns the number of options in the plan approval dialog.
// When context usage is above 30%, an extra "clear context" option is prepended.
func (m Model) planApprovalOptionCount() int {
	if m.planContextUsedPercent() > 30 {
		return 5
	}
	return 4
}

// planContextUsedPercent returns the percentage of context window used (0-100), or 0 if unknown.
func (m Model) planContextUsedPercent() int {
	if m.engineConfig == nil || m.engineConfig.CompactState == nil {
		return 0
	}
	s := m.engineConfig.CompactState
	if s.MaxTokens <= 0 {
		return 0
	}
	return s.TotalTokens * 100 / s.MaxTokens
}

// planApprovalOffset returns the offset to apply to cursor indices.
// When the "clear context" option is shown, all subsequent options shift by 1.
func (m Model) planApprovalOffset() int {
	if m.planContextUsedPercent() > 30 {
		return 0
	}
	return -1 // no clear-context option; cursor 0 maps to "auto-accept"
}

// handlePlanApprovalKey handles key events in the plan approval dialog shown after ExitPlanMode.
func (m Model) handlePlanApprovalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	numOptions := m.planApprovalOptionCount() - 1 // max cursor index

	// Map cursor position to logical action using offset.
	// With clear-context: 0=clear+auto, 1=auto, 2=manual, 3=revise, 4=chat
	// Without:            0=auto,        1=manual, 2=revise, 3=feedback
	logicalIdx := func() int {
		off := m.planApprovalOffset()
		if off < 0 {
			return m.planApprovalCursor - off // shift up: cursor 0 → logical 1
		}
		return m.planApprovalCursor
	}

	switch msg.String() {
	case "j", "down":
		if m.planApprovalCursor < numOptions {
			m.planApprovalCursor++
		}
	case "k", "up":
		if m.planApprovalCursor > 0 {
			m.planApprovalCursor--
		}
	case "ctrl+g":
		// Open the plan file in the user's editor. tea.ExecProcess suspends
		// the TUI, hands the terminal to the editor, and resumes when done.
		// planEditorFinishedMsg then restores the approval dialog.
		if m.planFilePath != "" {
			return m, openPlanEditor(m.planFilePath)
		}
		return m, m.waitForEvent()
	case "enter":
		switch logicalIdx() {
		case 0: // Yes, clear context + auto-accept edits
			m.planModeActive = false
			if m.engineConfig != nil {
				m.engineConfig.PermissionMode = "auto"
			}
			// Run compaction in background, then submit
			msgs := m.pendingEngineMessages
			planPath := m.planFilePath
			m.focus = FocusPrompt
			m.prompt.Focus()
			m.streaming = true
			m.spinText = "Compacting context..."
			m.spinner.Start("Compacting context...")
			m.refreshViewport()
			return m, func() tea.Msg {
				compacted, summary, err := compact.Compact(
					context.Background(), m.apiClient, msgs, 2, "",
				)
				submitMsg := fmt.Sprintf("Implement the plan from %s. The planning conversation has been compacted — refer to the summary and plan file for context.", planPath)
				return planCompactDoneMsg{compacted: compacted, summary: summary, err: err, submitMsg: submitMsg}
			}
		case 1: // Yes, auto-accept edits (keep context)
			m.planModeActive = false
			if m.engineConfig != nil {
				m.engineConfig.PermissionMode = "auto"
			}
			m.focus = FocusPrompt
			m.prompt.Focus()
			m.refreshViewport()
			return m.handleSubmit("Yes, proceed with implementation. Auto-accept all file edits.")
		case 2: // Yes, manually approve edits
			m.planModeActive = false
			m.focus = FocusPrompt
			m.prompt.Focus()
			m.refreshViewport()
			return m.handleSubmit("Yes, proceed with implementation.")
		case 3: // No, let me revise
			m.planModeActive = false
			m.focus = FocusPrompt
			m.prompt.Focus()

			m.refreshViewport()
			return m, nil
		case 4: // Chat about the plan — dismiss dialog and return to prompt.
			m.planModeActive = false
			m.focus = FocusPrompt
			m.prompt.Focus()
			m.refreshViewport()
			return m, nil
		}
	case "esc":
		// Dismiss — return to prompt without sending anything.
		m.planModeActive = false
		m.focus = FocusPrompt
		m.prompt.Focus()
		m.refreshViewport()
		return m, nil
	}

	return m, m.waitForEvent()
}

// renderPlanApprovalDialog renders the plan approval overlay shown after ExitPlanMode.
// renderAskUserDialog renders the interactive AskUser question dialog overlay.
func (m Model) renderAskUserDialog(width int) string {
	d := m.askUserDialog
	if d == nil {
		return ""
	}
	q := d.questions[d.qIdx]

	boxW := width - 8
	if boxW > 72 {
		boxW = 72
	}
	if boxW < 30 {
		boxW = 30
	}

	var b strings.Builder
	b.WriteString(styles.AskUserTitle.Render("? Question"))
	progress := fmt.Sprintf(" (%d/%d)", d.qIdx+1, len(d.questions))
	b.WriteString(styles.AskUserProgress.Render(progress))
	b.WriteString("\n\n")
	b.WriteString(styles.AskUserLabel.Render(q.Label))
	b.WriteString("\n")
	if q.Description != "" {
		b.WriteString(styles.AskUserDim.Render(q.Description))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	otherIdx := len(q.Options)
	chatIdx := otherIdx + 1
	for i, opt := range q.Options {
		prefix := "  "
		if i == d.optCursor && !d.typingOther {
			prefix = "▸ "
		}
		if q.MultiSelect {
			check := "[ ]"
			if d.multiSelected[i] {
				check = "[x]"
			}
			line := prefix + check + " " + opt
			if i == d.optCursor && !d.typingOther {
				b.WriteString(styles.AskUserSelected.Render(line))
			} else {
				b.WriteString(styles.AskUserDim.Render(line))
			}
		} else {
			if i == d.optCursor && !d.typingOther {
				b.WriteString(styles.AskUserSelected.Render(prefix + opt))
			} else {
				b.WriteString(styles.AskUserDim.Render(prefix + opt))
			}
		}
		b.WriteString("\n")
	}
	// "Other" inline-typing option.
	if d.typingOther {
		inputText := d.freeText + "█"
		b.WriteString(styles.AskUserSelected.Render("▸ Other: " + inputText))
	} else if d.optCursor == otherIdx {
		b.WriteString(styles.AskUserSelected.Render("▸ Other (type your own...)"))
	} else {
		b.WriteString(styles.AskUserDim.Render("  Other (type your own...)"))
	}
	b.WriteString("\n")
	// Divider before footer option.
	b.WriteString(styles.AskUserDim.Render("─────────────────────────"))
	b.WriteString("\n")
	// "Chat about this" footer option.
	if !d.typingOther && d.optCursor == chatIdx {
		b.WriteString(styles.AskUserSelected.Render("▸ Chat about this"))
	} else {
		b.WriteString(styles.AskUserDim.Render("  Chat about this"))
	}
	b.WriteString("\n")
	b.WriteString("\n")
	var hint string
	if d.typingOther {
		hint = "type answer · enter confirm · ctrl+g $EDITOR · esc back"
	} else if q.MultiSelect {
		hint = "j/k navigate · space toggle · enter confirm"
		if d.qIdx < len(d.questions)-1 {
			hint += " · esc cancel"
		} else {
			hint += " (submit) · esc cancel"
		}
	} else {
		hint = "j/k navigate · enter select"
		if d.qIdx < len(d.questions)-1 {
			hint += " · esc cancel"
		} else {
			hint += " (submit) · esc cancel"
		}
	}
	b.WriteString(styles.PanelHint.Render(hint))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.Aqua).
		Padding(1, 2).
		Width(boxW).
		Render(b.String())

	return lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(box)
}

func (m Model) renderPlanApprovalDialog(width int) string {
	boxWidth := width - 4
	if boxWidth < 40 {
		boxWidth = 40
	}
	if boxWidth > 80 {
		boxWidth = 80
	}

	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.Primary).
		Padding(1, 2).
		Width(boxWidth)

	title := styles.PlanDialogTitle.Render("Plan ready — how would you like to proceed?")

	usedPct := m.planContextUsedPercent()
	var options []string
	if usedPct > 30 {
		options = append(options, fmt.Sprintf("Yes, clear context (%d%% used) and auto-accept edits", usedPct))
	}
	options = append(options,
		"Yes, auto-accept edits",
		"Yes, manually approve edits",
		"No, let me revise",
		"Chat about the plan",
	)

	var rows []string
	rows = append(rows, title, "")

	// Show a truncated preview of the plan file content (max 10 lines).
	if m.planFilePath != "" {
		if raw, err := os.ReadFile(m.planFilePath); err == nil && len(raw) > 0 {
			lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
			const maxPreviewLines = 10
			truncated := false
			if len(lines) > maxPreviewLines {
				lines = lines[:maxPreviewLines]
				truncated = true
			}
			for _, l := range lines {
				rows = append(rows, styles.PlanPreviewStyle.Render(l))
			}
			if truncated {
				rows = append(rows, styles.PlanPreviewStyle.Render("…"))
			}
			rows = append(rows, "")
		}
	}
	for i, opt := range options {
		cursor := "  "
		style := lipgloss.NewStyle()
		if i == m.planApprovalCursor {
			cursor = styles.PlanOptionCursor.Render("› ")
			style = styles.PlanOptionStyle
		}
		label := cursor + style.Render(opt)
		rows = append(rows, label)
	}

	rows = append(rows, "")
	hint := styles.PlanHintStyle.Render(
		"j/k navigate · enter confirm · esc dismiss",
	)
	if m.planFilePath != "" {
		planShort := m.planFilePath
		if home, err := os.UserHomeDir(); err == nil {
			planShort = strings.Replace(planShort, home, "~", 1)
		}
		hint += "\n" + styles.PlanHintStyle.Render(
			"ctrl+g edit in $EDITOR · "+planShort,
		)
	}
	rows = append(rows, hint)

	return border.Render(strings.Join(rows, "\n"))
}

func (m Model) handlePermissionResponse(resp permissions.ResponseMsg) (tea.Model, tea.Cmd) {
	m.focus = FocusPrompt

	switch resp.Decision {
	case permissions.DecisionAllow:
		m.approvalCh <- true
	case permissions.DecisionAllowAlways:
		// Persist rule BEFORE unblocking the engine so the next tool check sees it.
		rule := buildPermissionRule(resp.ToolUse)
		m.persistPermissionRule(rule)
		m.approvalCh <- true
		m.addMessage(ChatMessage{Type: MsgSystem, Content: fmt.Sprintf("Saved rule: allow %s(%s)", rule.Tool, rule.Pattern)})
	case permissions.DecisionAllowAllTool:
		// Persist rule BEFORE unblocking the engine so the next tool check sees it.
		rule := config.PermissionRule{
			Tool:     resp.ToolUse.Name,
			Pattern:  "*",
			Behavior: "allow",
		}
		m.persistPermissionRule(rule)
		m.approvalCh <- true
		m.addMessage(ChatMessage{Type: MsgSystem, Content: fmt.Sprintf("Saved rule: allow %s(*)", rule.Tool)})
	case permissions.DecisionDeny:
		m.approvalCh <- false
		m.addMessage(ChatMessage{Type: MsgSystem, Content: fmt.Sprintf("Denied %s", resp.ToolUse.Name)})
	}

	m.refreshViewport()
	return m, tea.Batch(m.spinner.Tick(), m.waitForEvent())
}

// buildPermissionRule creates a permission rule from a tool use for "Always allow".
func buildPermissionRule(tu tools.ToolUse) config.PermissionRule {
	pattern := extractRulePattern(tu)
	return config.PermissionRule{
		Tool:     tu.Name,
		Pattern:  pattern,
		Behavior: "allow",
	}
}

// extractRulePattern generates a glob pattern from a tool invocation.
// For Bash: uses the command prefix (first word + *).
// For file tools: uses the exact path.
// For web tools: uses the domain.
func extractRulePattern(tu tools.ToolUse) string {
	switch tu.Name {
	case "Bash":
		var in struct{ Command string `json:"command"` }
		if json.Unmarshal(tu.Input, &in) == nil && in.Command != "" {
			// Use first word as prefix: "go test ./..." → "go test *"
			parts := strings.SplitN(in.Command, " ", 2)
			return parts[0] + " *"
		}
	case "Read", "Write", "Edit":
		var in struct{ FilePath string `json:"file_path"` }
		if json.Unmarshal(tu.Input, &in) == nil && in.FilePath != "" {
			return in.FilePath
		}
	case "WebFetch":
		var in struct{ URL string `json:"url"` }
		if json.Unmarshal(tu.Input, &in) == nil && in.URL != "" {
			return extractDomainPattern(in.URL)
		}
	case "WebSearch":
		return "*" // allow all searches
	}
	return "*"
}

// extractDomainPattern extracts "domain:example.com" from a URL for pattern matching.
func extractDomainPattern(rawURL string) string {
	// Simple extraction: strip protocol, take host
	u := rawURL
	for _, prefix := range []string{"https://", "http://"} {
		u = strings.TrimPrefix(u, prefix)
	}
	if idx := strings.IndexByte(u, '/'); idx >= 0 {
		u = u[:idx]
	}
	if u != "" {
		return "domain:" + u
	}
	return "*"
}

// persistPermissionRule saves a permission rule to the project config (or global).
// It deduplicates: if an identical rule already exists, it is not added again.
func (m *Model) persistPermissionRule(rule config.PermissionRule) {
	if m.appCtx == nil || m.appCtx.Config == nil {
		return
	}

	// Helper: check if rule already exists in a slice.
	hasDuplicate := func(rules []config.PermissionRule) bool {
		for _, r := range rules {
			if r.Tool == rule.Tool && r.Pattern == rule.Pattern && r.Behavior == rule.Behavior {
				return true
			}
		}
		return false
	}



	// Add to live config (skip if already present).
	if !hasDuplicate(m.appCtx.Config.PermissionRules) {
		m.appCtx.Config.PermissionRules = append(m.appCtx.Config.PermissionRules, rule)
	}

	// Also add to engine config so it takes effect immediately.
	if m.engineConfig != nil && !hasDuplicate(m.engineConfig.PermissionRules) {
		m.engineConfig.PermissionRules = append(m.engineConfig.PermissionRules, rule)
	}

	// Propagate to the running engine so it applies without restart.
	if m.engine != nil {
		m.engine.SetPermissionRules(m.appCtx.Config.PermissionRules)
	}

	// Save to project config if available, else global
	cwd, _ := os.Getwd()
	projectRoot := config.FindGitRoot(cwd)
	projectSettings := filepath.Join(projectRoot, ".claudio", "settings.json")

	savePath := config.GetPaths().Settings // default: global
	if _, err := os.Stat(filepath.Join(projectRoot, ".claudio")); err == nil {
		savePath = projectSettings // prefer project if .claudio/ exists
	}

	// Load existing, append rule (if not duplicate), save back.
	data, _ := os.ReadFile(savePath)
	var existing map[string]json.RawMessage
	if json.Unmarshal(data, &existing) != nil {
		existing = make(map[string]json.RawMessage)
	}

	var rules []config.PermissionRule
	if raw, ok := existing["permissionRules"]; ok {
		json.Unmarshal(raw, &rules)
	}
	if !hasDuplicate(rules) {
		rules = append(rules, rule)
	}
	rulesJSON, _ := json.Marshal(rules)
	existing["permissionRules"] = rulesJSON

	out, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(savePath, out, 0644)
}

// ── Leader Key State Machine ────────────────────────────

// loadKeymap initialises the keymap from config, falling back to defaults.
func loadKeymap() *keymap.Keymap {
	km, err := keymap.Load()
	if err != nil {
		// Config read failed — use defaults silently.
		return keymap.Default()
	}
	return km
}

// handleLeaderKey processes the next key in a leader sequence.
// It accumulates keys in m.leaderSeq and resolves through the keymap.
// Returns (handled bool, cmd). If handled is false, the key was not consumed.
func (m *Model) handleLeaderKey(key string) (bool, tea.Cmd) {
	seq := m.leaderSeq
	m.leaderSeq = "" // reset by default

	if seq == "pending" {
		seq = ""
	}

	// Build the full key sequence accumulated so far.
	// Special case: "," + "enter" maps to ",\n" in the keymap.
	fullSeq := seq + key
	if key == "enter" {
		fullSeq = seq + "\n"
	}

	// Try to resolve an exact action.
	action, ok := m.km.Resolve(fullSeq)
	if ok {
		// If this sequence is ALSO a prefix of longer bindings, we still
		// dispatch immediately (like vim: "e" fires even if "ev" exists,
		// because the user pressed exactly "e" and nothing more).
		// However, to support "ev" properly, we need a sub-sequence mechanism.
		// Check if this is also a prefix — if so, we need to wait.
		if m.km.HasPrefix(fullSeq) {
			// Ambiguous: "e" matches but "ev" also exists.
			// Buffer and schedule timeout — on next key we'll extend or dispatch.
			m.leaderSeq = fullSeq
			m.leaderSeqGen++
			return true, whichkey.ScheduleTimeout(m.leaderSeqGen)
		}
		return true, m.dispatchAction(action)
	}

	// Not an exact match — is it a valid prefix?
	if m.km.HasPrefix(fullSeq) {
		m.leaderSeq = fullSeq
		m.leaderSeqGen++
		return true, whichkey.ScheduleTimeout(m.leaderSeqGen)
	}

	// Try dispatching the accumulated prefix if there was one.
	// E.g., user typed "e" then "x" — "ex" doesn't match, but "e" might.
	if len(seq) > 0 {
		if prevAction, prevOK := m.km.Resolve(seq); prevOK {
			// Dispatch the prefix action; the trailing key is dropped.
			return true, m.dispatchAction(prevAction)
		}
	}

	return true, nil // consumed but no match (leader mode eats the key)
}

// dispatchAction executes the logic for a resolved action.
func (m *Model) dispatchAction(action keymap.ActionID) tea.Cmd {
	switch action {
	// ── Window Management ──────────────────
	case keymap.ActionWindowCycle:
		hasPanel := m.activePanel != nil && m.activePanel.IsActive()
		switch m.focus {
		case FocusPrompt:
			m.focus = FocusViewport
			m.prompt.Blur()
			if len(m.vpSections) > 0 {
				m.vpCursor = len(m.vpSections) - 1
			} else {
				m.vpCursor = 0
			}
			m.refreshViewport()
			m.scrollToSection(m.vpCursor)
		case FocusViewport:
			if hasPanel {
				m.focus = FocusPanel
				m.activePanel.Activate()
			} else {
				m.focus = FocusPrompt
				m.vpCursor = -1
				m.prompt.Focus()
				m.refreshViewport()
			}
		case FocusPanel:
			m.focus = FocusPrompt
			m.vpCursor = -1
			m.prompt.Focus()
			m.refreshViewport()
		default:
			m.focus = FocusPrompt
			m.prompt.Focus()
		}
		return nil

	case keymap.ActionWindowFocusUp:
		m.focus = FocusViewport
		m.prompt.Blur()
		if len(m.vpSections) > 0 {
			m.vpCursor = len(m.vpSections) - 1
		} else {
			m.vpCursor = 0
		}
		m.refreshViewport()
		m.scrollToSection(m.vpCursor)
		return nil

	case keymap.ActionWindowFocusDown:
		m.focus = FocusPrompt
		m.vpCursor = -1
		m.prompt.Focus()
		m.refreshViewport()
		return nil

	case keymap.ActionWindowFocusLeft:
		if m.focus == FocusPanel {
			m.focus = FocusViewport
			m.prompt.Blur()
			if len(m.vpSections) > 0 && m.vpCursor < 0 {
				m.vpCursor = len(m.vpSections) - 1
			}
			m.refreshViewport()
		}
		return nil

	case keymap.ActionWindowFocusRight:
		hasPanel := m.activePanel != nil && m.activePanel.IsActive()
		if hasPanel {
			m.focus = FocusPanel
			m.activePanel.Activate()
		} else {
			m.openPanel(m.lastPanelID)
		}
		return nil

	case keymap.ActionWindowSplitVertical:
		m.openPanel(PanelConversation)
		m.focus = FocusPanel
		// Initialize right window to mirror the main window's session.
		// After this, bn/bp in the right window will diverge independently.
		if m.rightWindow.sessionID == "" {
			m.rightWindow.sessionID = m.mainWindow.sessionID
			m.rightWindow.title = m.mainWindow.title
			// messages stays nil → will mirror main content until diverged
		}
		return nil

	case keymap.ActionWindowClose:
		if m.activePanel != nil && m.activePanel.IsActive() {
			m.closePanel()
		}
		return nil

	// ── Buffer/Session Management ──────────
	case keymap.ActionBufferNext:
		if m.isRightWindowFocused() {
			_, cmd := m.switchRightWindowRelative(1)
			return cmd
		}
		_, cmd := m.switchSessionRelative(1)
		return cmd

	case keymap.ActionBufferPrev:
		if m.isRightWindowFocused() {
			_, cmd := m.switchRightWindowRelative(-1)
			return cmd
		}
		_, cmd := m.switchSessionRelative(-1)
		return cmd

	case keymap.ActionBufferNew:
		_, cmd := m.createNewSession()
		return cmd

	case keymap.ActionBufferClose:
		_, cmd := m.deleteCurrentSession()
		return cmd

	case keymap.ActionBufferRename:
		_, cmd := m.renameCurrentSession()
		return cmd

	case keymap.ActionBufferAlternate:
		_, cmd := m.switchToAlternateSession()
		return cmd

	case keymap.ActionBufferList:
		m.showBufferList()
		return nil

	// ── Panels ─────────────────────────────
	case keymap.ActionPanelConfig:
		m.togglePanel(PanelConfig)
		return nil

	case keymap.ActionPanelSkills:
		m.togglePanel(PanelSkills)
		return nil

	case keymap.ActionPanelMemory:
		m.togglePanel(PanelMemory)
		return nil

	case keymap.ActionPanelTasks:
		m.togglePanel(PanelTasks)
		return nil

	case keymap.ActionPanelTools:
		m.togglePanel(PanelTools)
		return nil

	case keymap.ActionPanelAnalytics:
		m.togglePanel(PanelAnalytics)
		return nil

	case keymap.ActionPanelAgents:
		m.openPanel(PanelAgents)
		if _, ok := m.activePanel.(*teampanel.Panel); ok {
			return teampanel.ScheduleRefresh()
		}
		return nil

	case keymap.ActionPanelFiles:
		if m.filesPanel != nil {
			if m.filesPanel.IsActive() {
				m.filesPanel.Deactivate()
				m.filesPanel.SetFocused(false)
				if m.focus == FocusFiles {
					m.focus = FocusPrompt
					m.prompt.Focus()
				}
			} else {
				m.filesPanel.Activate()
				m.filesPanel.SetFocused(true)
				m.focus = FocusFiles
				m.prompt.Blur()
			}
			m.refreshViewport()
		}
		return nil

	case keymap.ActionPanelSessionTree:
		m.togglePanel(PanelSessionTree)
		return nil

	case keymap.ActionPanelAgentGUI:
		m.togglePanel(PanelAgentGUI)
		return nil

	// ── Navigation ─────────────────────────
	case keymap.ActionSessionPicker, keymap.ActionSessionRecent, keymap.ActionSearch:
		_, cmd := m.openSessionPicker()
		return cmd

	case keymap.ActionCommandPalette:
		if !m.streaming {
			m.filePicker.Deactivate()
			m.prompt.SetValue("/")
			m.prompt.EnterVimInsert()
			m.focus = FocusPrompt
			m.prompt.Focus()
			m.updatePaletteState()
		}
		return nil

	// ── Editor ─────────────────────────────
	case keymap.ActionEditorEditPrompt:
		if m.streaming {
			return m.toast.Show("Cannot edit while streaming")
		}
		return openInEditor(m.prompt.Value(), false)

	case keymap.ActionEditorViewSection:
		if m.vpCursor >= 0 && m.vpCursor < len(m.vpSections) {
			section := m.vpSections[m.vpCursor]
			content := m.extractSectionText(section)
			if content == "" {
				return m.toast.Show("No content in current section")
			}
			return openInEditor(content, true)
		}
		return m.toast.Show("No section focused — use <Space>wk first")

	// ── Misc ───────────────────────────────
	case keymap.ActionTodoDock:
		if m.todoDock != nil {
			m.todoDock.ToggleExpanded()
			m.refreshViewport()
		}
		return nil
	}

	return nil
}

// togglePanel opens a panel if it's not active, or closes it if it is.
func (m *Model) togglePanel(id PanelID) {
	if m.activePanelID == id && m.activePanel != nil && m.activePanel.IsActive() {
		m.closePanel()
		m.focus = FocusPrompt
		m.prompt.Focus()
	} else {
		m.openPanel(id)
	}
	m.refreshViewport()
}

func (m *Model) openSessionPicker() (bool, tea.Cmd) {
	if m.sessionPicker != nil && m.sessionPicker.IsActive() {
		m.sessionPicker.Deactivate()
		m.sessionPicker = nil
		m.focus = FocusPrompt
		m.prompt.Focus()
	} else if m.session != nil {
		picker := panelsessions.New(m.session)
		picker.SetSize(m.width, m.height)
		picker.Activate()
		m.sessionPicker = picker
		m.focus = FocusPanel
		m.prompt.Blur()
	}
	return true, nil
}

func (m *Model) switchSessionRelative(dir int) (bool, tea.Cmd) {
	if m.session == nil {
		return true, nil
	}
	sessions, err := m.session.RecentForProject(20)
	if err != nil || len(sessions) < 2 {
		return true, nil
	}
	cur := m.session.Current()
	if cur == nil {
		return true, nil
	}
	idx := -1
	for i, s := range sessions {
		if s.ID == cur.ID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return true, nil
	}
	next := idx + dir
	if next < 0 {
		next = len(sessions) - 1
	} else if next >= len(sessions) {
		next = 0
	}
	m.doSwitchSession(sessions[next].ID)

	// Show toast
	title := sessions[next].Title
	if title == "" {
		short := sessions[next].ID
		if len(short) > 8 {
			short = short[:8]
		}
		title = short
	}
	arrow := "→"
	if dir < 0 {
		arrow = "←"
	}
	toastCmd := m.toast.Show(fmt.Sprintf(" %s %s (%d of %d) ", arrow, title, next+1, len(sessions)))
	if resumeCmd := m.resumeStreamingCmds(); resumeCmd != nil {
		return true, tea.Batch(toastCmd, resumeCmd)
	}
	return true, toastCmd
}

func (m *Model) switchToAlternateSession() (bool, tea.Cmd) {
	if m.prevSessionID == "" || m.session == nil {
		m.addMessage(ChatMessage{Type: MsgSystem, Content: "No alternate session"})
		m.refreshViewport()
		return true, nil
	}
	targetID := m.prevSessionID
	m.doSwitchSession(targetID)

	// Show toast
	cur := m.session.Current()
	if cur != nil {
		title := cur.Title
		if title == "" {
			short := cur.ID
			if len(short) > 8 {
				short = short[:8]
			}
			title = short
		}
		toastCmd := m.toast.Show(fmt.Sprintf(" ⇄ %s ", title))
		if resumeCmd := m.resumeStreamingCmds(); resumeCmd != nil {
			return true, tea.Batch(toastCmd, resumeCmd)
		}
		return true, toastCmd
	}
	return true, m.resumeStreamingCmds()
}

func (m *Model) doSwitchSession(id string) {
	if m.session == nil {
		return
	}

	// Save current session's runtime state (keep streaming in background)
	if cur := m.session.Current(); cur != nil {
		m.prevSessionID = cur.ID
		if m.streaming {
			m.saveSessionRuntime(cur.ID)
		}
	}

	// Check if we're switching to a session that has a background runtime
	if rt, ok := m.sessionRuntimes[id]; ok {
		m.restoreSessionRuntime(rt)
		delete(m.sessionRuntimes, id)
		m.session.Resume(id)
		m.syncMainWindowState()
		m.refreshViewport()
		// Note: caller must check m.streaming and issue waitForEvent()+spinner.Tick() if true
		return
	}

	resumed, err := m.session.Resume(id)
	if err != nil {
		m.addMessage(ChatMessage{Type: MsgError, Content: fmt.Sprintf("Switch failed: %v", err)})
		m.refreshViewport()
		return
	}
	// Clear current conversation and show resume context
	m.messages = nil
	m.streamText.Reset()
	m.turns = 0
	m.totalTokens = 0
	m.totalCost = 0
	m.usageTracker = api.NewUsageTracker(m.model, 0)
	m.pendingEngineMessages = nil

	title := resumed.Title
	if title == "" {
		short := resumed.ID
		if len(short) > 8 {
			short = short[:8]
		}
		title = short
	}

	// Build resume context block
	var ctx strings.Builder
	ctx.WriteString(fmt.Sprintf("Resumed: %s\n", title))
	ctx.WriteString(fmt.Sprintf("  %s · %s · %s", shortModelName(resumed.Model), timeAgo(resumed.UpdatedAt), shortenDir(resumed.ProjectDir)))
	if resumed.Summary != "" {
		ctx.WriteString(fmt.Sprintf("\n\n  %s", resumed.Summary))
	}
	// Append resume header directly — don't persist to DB
	m.messages = append(m.messages, ChatMessage{Type: MsgSystem, Content: ctx.String()})

	// Load previous messages from DB
	if storedMsgs, err := m.session.GetMessages(); err == nil && len(storedMsgs) > 0 {
		for _, msg := range storedMsgs {
			var msgType MessageType
			switch msg.Type {
			case "user":
				msgType = MsgUser
			case "assistant":
				msgType = MsgAssistant
			case "tool_use":
				msgType = MsgToolUse
			case "tool_result":
				msgType = MsgToolResult
			default:
				continue
			}
			m.messages = append(m.messages, ChatMessage{
				Type:      msgType,
				Content:   msg.Content,
				ToolName:  msg.ToolName,
				ToolUseID: msg.ToolUseID,
			})
		}

		// Restore engine conversation history so the model has full context.
		// Create the engine eagerly here so slash commands like /compact work
		// immediately after resume without needing to send a message first.
		engineMsgs := session.ReconstructEngineMessages(storedMsgs)
		if m.engine == nil {
			handler := &tuiEventHandler{ch: m.eventCh, approvalCh: m.approvalCh}
			if m.engineConfig != nil {
				if m.session != nil && m.session.Current() != nil {
					m.engineConfig.SessionID = m.session.Current().ID
				}
				m.engine = query.NewEngineWithConfig(m.apiClient, m.registry, handler, *m.engineConfig)
			} else {
				m.engine = query.NewEngine(m.apiClient, m.registry, handler)
			}
			if m.engineRef != nil {
				*m.engineRef = m.engine
			}
			if m.systemPrompt != "" {
				m.engine.SetSystem(m.systemPrompt)
			}
			if m.userContext != "" {
				m.engine.SetUserContext(m.userContext)
			}
			if m.appCtx != nil && m.appCtx.Memory != nil {
				ttl := 0 // default: no TTL filtering
				if m.appCtx.Config != nil {
					ttl = m.appCtx.Config.GetMemoryIndexTTLDays()
				}
				idx := m.appCtx.Memory.BuildIndex(ttl)
				if idx != "" {
					m.engine.SetMemoryIndex("## Your Memory Index\n\n" + idx)
				}
				// Wire up the refresh function for post-compaction updates
				m.engine.SetMemoryRefreshFunc(func() string {
					ttl := 0 // default: no TTL filtering
					if m.appCtx.Config != nil {
						ttl = m.appCtx.Config.GetMemoryIndexTTLDays()
					}
					return m.appCtx.Memory.BuildIndex(ttl)
				})
			}
			if m.systemContext != "" {
				m.engine.SetSystemContext(m.systemContext)
			}
		}
		m.engine.SetMessages(engineMsgs)

		// Restore turn count and estimate tokens from stored content
		for _, msg := range storedMsgs {
			if msg.Type == "user" {
				m.turns++
			}
			m.totalTokens += (len(msg.Content) + 3) / 4 // ~4 chars per token
		}
		// Rough cost estimate (use Sonnet pricing as baseline)
		m.totalCost = float64(m.totalTokens) * 3.0 / 1_000_000
	}

	// Inject summary into system prompt for AI continuity (guard against double-append
	// if resumeSession is called more than once for the same session).
	if resumed.Summary != "" && !m.resumeSummarySet {
		m.systemPrompt += "\n\n# Previous Session Context\n" + resumed.Summary
		m.resumeSummarySet = true
	}
	m.syncMainWindowState()
	m.refreshViewport()
}

// isRightWindowFocused returns true when focus is on the conversation mirror panel.
func (m *Model) isRightWindowFocused() bool {
	return m.focus == FocusPanel && m.activePanelID == PanelConversation
}

// rightWindowHasOwnSession returns true when the right window shows a different
// session than the main window.
func (m *Model) rightWindowHasOwnSession() bool {
	return m.rightWindow.sessionID != "" && m.rightWindow.sessionID != m.mainWindow.sessionID
}

// syncMainWindowState updates mainWindow to reflect the current active session.
func (m *Model) syncMainWindowState() {
	if m.session != nil {
		if cur := m.session.Current(); cur != nil {
			m.mainWindow.sessionID = cur.ID
			title := cur.Title
			if title == "" && len(cur.ID) > 8 {
				title = cur.ID[:8]
			} else if title == "" {
				title = cur.ID
			}
			m.mainWindow.title = title
		}
	}
}

// switchRightWindowSession loads a different session into the right (conversation
// mirror) window without affecting the main window's session or engine state.
func (m *Model) switchRightWindowSession(id string) {
	if m.session == nil || m.db == nil {
		return
	}

	// Load session metadata
	sess, err := m.db.GetSession(id)
	if err != nil || sess == nil {
		return
	}

	m.rightWindow.sessionID = sess.ID
	title := sess.Title
	if title == "" && len(sess.ID) > 8 {
		title = sess.ID[:8]
	} else if title == "" {
		title = sess.ID
	}
	m.rightWindow.title = title

	// Load messages from DB for the right window
	storedMsgs, err := m.db.GetMessages(sess.ID)
	if err != nil {
		m.rightWindow.messages = nil
		return
	}

	m.rightWindow.messages = nil
	for _, msg := range storedMsgs {
		var msgType MessageType
		switch msg.Type {
		case "user":
			msgType = MsgUser
		case "assistant":
			msgType = MsgAssistant
		case "tool_use":
			msgType = MsgToolUse
		case "tool_result":
			msgType = MsgToolResult
		default:
			continue
		}
		m.rightWindow.messages = append(m.rightWindow.messages, ChatMessage{
			Type:      msgType,
			Content:   msg.Content,
			ToolName:  msg.ToolName,
			ToolUseID: msg.ToolUseID,
		})
	}
}

// switchRightWindowRelative cycles the right window's session by dir (+1 or -1).
func (m *Model) switchRightWindowRelative(dir int) (bool, tea.Cmd) {
	if m.session == nil {
		return true, nil
	}
	sessions, err := m.session.RecentForProject(20)
	if err != nil || len(sessions) < 2 {
		return true, nil
	}

	// Find current right window session in the list
	curID := m.rightWindow.sessionID
	if curID == "" {
		// Right window not initialized — use main window's session
		if cur := m.session.Current(); cur != nil {
			curID = cur.ID
		}
	}

	idx := -1
	for i, s := range sessions {
		if s.ID == curID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return true, nil
	}
	next := idx + dir
	if next < 0 {
		next = len(sessions) - 1
	} else if next >= len(sessions) {
		next = 0
	}

	m.switchRightWindowSession(sessions[next].ID)
	m.refreshViewport()

	// Show toast
	title := sessions[next].Title
	if title == "" {
		short := sessions[next].ID
		if len(short) > 8 {
			short = short[:8]
		}
		title = short
	}
	arrow := "→"
	if dir < 0 {
		arrow = "←"
	}
	return true, m.toast.Show(fmt.Sprintf(" %s %s [right] (%d of %d) ", arrow, title, next+1, len(sessions)))
}

// showBufferList renders a :ls / :buffers style buffer list as a system message.
// Markers follow vim conventions: % = main window, # = right window, a = active, h = hidden.
func (m *Model) showBufferList() {
	if m.session == nil {
		return
	}
	sessions, err := m.session.RecentForProject(20)
	if err != nil || len(sessions) == 0 {
		m.addMessage(ChatMessage{Type: MsgSystem, Content: "No sessions found."})
		m.refreshViewport()
		return
	}

	mainID := m.mainWindow.sessionID
	rightID := m.rightWindow.sessionID
	rightActive := m.activePanelID == PanelConversation && m.activePanel != nil && m.activePanel.IsActive()

	var buf strings.Builder
	buf.WriteString("Buffer list:\n")
	for i, s := range sessions {
		// Determine markers
		marker := "  "
		flag := " "
		switch {
		case s.ID == mainID && s.ID == rightID && rightActive:
			marker = "%="
			flag = "a"
		case s.ID == mainID:
			marker = "% "
			flag = "a"
		case s.ID == rightID && rightActive:
			marker = "# "
			flag = "a"
		default:
			flag = "h"
		}

		title := s.Title
		if title == "" {
			short := s.ID
			if len(short) > 8 {
				short = short[:8]
			}
			title = short
		}

		buf.WriteString(fmt.Sprintf("  %2d %s%s  %-30s  %s\n", i+1, marker, flag, title, timeAgo(s.UpdatedAt)))
	}
	m.addMessage(ChatMessage{Type: MsgSystem, Content: buf.String()})
	m.refreshViewport()
}


// countBackgroundSessions returns the number of sessions still streaming in the background.
// Also cleans up finished runtimes.
func (m *Model) countBackgroundSessions() int {
	count := 0
	var finished []string
	for id, rt := range m.sessionRuntimes {
		rt.mu.Lock()
		if rt.Streaming {
			count++
		} else {
			finished = append(finished, id)
		}
		rt.mu.Unlock()
	}
	// Clean up finished runtimes (keep their state for when user switches back)
	// Don't delete — user may want to see the completed results
	_ = finished
	return count
}

// resumeStreamingCmds returns tea.Cmds needed if we just restored a streaming session.
func (m *Model) resumeStreamingCmds() tea.Cmd {
	if !m.streaming {
		return nil
	}
	return tea.Batch(m.spinner.Tick(), m.waitForEvent())
}

// saveSessionRuntime saves the current session's streaming state into a background runtime.
func (m *Model) saveSessionRuntime(sessionID string) {
	rt := NewSessionRuntime(sessionID)
	rt.Engine = m.engine
	rt.CancelFunc = m.cancelFunc
	rt.EventCh = m.eventCh
	rt.ApprovalCh = m.approvalCh
	rt.Messages = m.messages
	rt.StreamText = m.streamText
	rt.Streaming = m.streaming
	rt.TotalTokens = m.totalTokens
	rt.TotalCost = m.totalCost
	rt.Turns = m.turns
	rt.ExpandedGroups = m.expandedGroups
	rt.LastToolGroup = m.lastToolGroup
	rt.SpinText = m.spinText
	rt.MessageQueue = m.messageQueue
	rt.ToolStartTimes = m.toolStartTimes

	m.sessionRuntimes[sessionID] = rt
	rt.StartBackgroundDrain()

	// Reset Model state for the new session
	m.engine = nil
	m.cancelFunc = nil
	m.eventCh = make(chan tuiEvent, 64)
	m.approvalCh = nil

	// Re-wire teammate event handler to the new event channel so team
	// events are not lost when the user switches sessions.
	if m.appCtx != nil && m.appCtx.TeamRunner != nil {
		m.appCtx.TeamRunner.SetEventHandler(&tuiTeammateEventHandler{ch: m.eventCh})
	}
	m.messages = nil
	m.streamText = &strings.Builder{}
	m.streaming = false
	m.totalTokens = 0
	m.totalCost = 0
	m.usageTracker = api.NewUsageTracker(m.model, 0)
	m.turns = 0
	m.expandedGroups = make(map[int]bool)
	m.lastToolGroup = -1
	m.toolStartTimes = make(map[string]time.Time)
	m.spinText = ""
	m.spinner.Stop()
	m.messageQueue = nil
}

// restoreSessionRuntime restores a background session's state back into the Model.
func (m *Model) restoreSessionRuntime(rt *SessionRuntime) {
	rt.StopBackgroundDrain()

	// Grab the accumulated state under lock
	rt.mu.Lock()
	defer rt.mu.Unlock()

	m.engine = rt.Engine
	m.cancelFunc = rt.CancelFunc
	m.eventCh = rt.EventCh
	m.approvalCh = rt.ApprovalCh
	m.messages = rt.Messages
	m.streamText = rt.StreamText
	m.streaming = rt.Streaming
	m.totalTokens = rt.TotalTokens
	m.totalCost = rt.TotalCost
	m.turns = rt.Turns
	m.expandedGroups = rt.ExpandedGroups
	m.lastToolGroup = rt.LastToolGroup
	m.spinText = rt.SpinText
	m.messageQueue = rt.MessageQueue
	if rt.ToolStartTimes != nil {
		m.toolStartTimes = rt.ToolStartTimes
	} else {
		m.toolStartTimes = make(map[string]time.Time)
	}

	// Re-wire teammate event handler to the restored event channel
	if m.appCtx != nil && m.appCtx.TeamRunner != nil {
		m.appCtx.TeamRunner.SetEventHandler(&tuiTeammateEventHandler{ch: m.eventCh})
	}

	// Replay any teammate events that arrived while backgrounded —
	// convert them to task-notification messages so the lead can act on them.
	for _, ev := range rt.TeammateEvents {
		if ev.teammateEvent != nil {
			_ = m.handleTeammateEvent(*ev.teammateEvent)

			if ev.teammateEvent.Type == "complete" || ev.teammateEvent.Type == "error" {
				taskInfo := ""
				if agentTasks := tools.GlobalTaskStore.ByAssignee(ev.teammateEvent.AgentName); len(agentTasks) > 0 {
					taskInfo = "\nAssigned tasks:\n"
					for _, t := range agentTasks {
						taskInfo += fmt.Sprintf("  #%s [%s] %s\n", t.ID, t.Status, t.Subject)
					}
				}
				worktreeInfo := ""
				if ev.teammateEvent.WorktreePath != "" {
					worktreeInfo = fmt.Sprintf("\nWorktree with changes: %s (branch: %s)\nTo use these files, copy them from the worktree to the main repo, or run: git merge %s", ev.teammateEvent.WorktreePath, ev.teammateEvent.WorktreeBranch, ev.teammateEvent.WorktreeBranch)
				}
				var notification string
				if ev.teammateEvent.Type == "complete" {
					notification = fmt.Sprintf("<task-notification>\nAgent %q in team %q completed.\nResult summary: %s%s%s\nUse the Agents panel or SendMessage to get full details if needed.\n</task-notification>", ev.teammateEvent.AgentName, ev.teammateEvent.TeamName, summaryFromResult(ev.teammateEvent.Text, 15, 600), taskInfo, worktreeInfo)
				} else {
					notification = fmt.Sprintf("<task-notification>\nAgent %q in team %q failed.\nError: %s%s%s\n</task-notification>", ev.teammateEvent.AgentName, ev.teammateEvent.TeamName, summaryFromResult(ev.teammateEvent.Text, 15, 600), taskInfo, worktreeInfo)
				}
				m.messageQueue = append(m.messageQueue, notification)
			}

			// Delete team-lead's inbox after consuming
			if mb := m.appCtx.TeamRunner.GetMailbox(); mb != nil {
				mb.ReadUnread("team-lead")
				mb.ClearInbox("team-lead")
			}
		}
	}
	rt.TeammateEvents = nil

	if m.streaming {
		m.spinner.Start(m.spinText)
	}
}

func (m *Model) createNewSession() (bool, tea.Cmd) {
	if m.session == nil {
		return true, nil
	}
	// Save prev for alternate switching
	if cur := m.session.Current(); cur != nil {
		m.prevSessionID = cur.ID
		// If current session is streaming, save its runtime so it keeps running
		if m.streaming {
			m.saveSessionRuntime(cur.ID)
		}
	}
	if _, err := m.session.Start(m.model); err != nil {
		m.addMessage(ChatMessage{Type: MsgError, Content: fmt.Sprintf("New session failed: %v", err)})
		m.refreshViewport()
		return true, nil
	}
	// Fully reset state for the new session
	m.engine = nil
	m.cancelFunc = nil
	m.eventCh = make(chan tuiEvent, 64)
	m.approvalCh = nil
	m.messages = nil
	m.streamText = &strings.Builder{}
	m.streaming = false
	m.turns = 0
	m.totalTokens = 0
	m.totalCost = 0
	m.usageTracker = api.NewUsageTracker(m.model, 0)
	m.expandedGroups = make(map[int]bool)
	m.lastToolGroup = -1
	m.toolStartTimes = make(map[string]time.Time)
	m.spinText = ""
	m.spinner.Stop()
	m.messageQueue = nil
	m.pendingEngineMessages = nil
	m.resumeSummarySet = false
	m.syncMainWindowState()
	m.refreshViewport()
	return true, nil
}

func (m *Model) deleteCurrentSession() (bool, tea.Cmd) {
	// Don't delete the only session — create a new one first
	if m.session == nil {
		return true, nil
	}
	cur := m.session.Current()
	if cur == nil {
		return true, nil
	}
	oldTitle := cur.Title
	if oldTitle == "" {
		oldTitle = cur.ID[:8]
	}
	oldID := cur.ID
	// Cancel any streaming on the current session
	if m.streaming && m.cancelFunc != nil {
		m.cancelFunc()
	}
	// Remove background runtime if it exists
	if rt, ok := m.sessionRuntimes[oldID]; ok {
		rt.StopBackgroundDrain()
		if rt.CancelFunc != nil {
			rt.CancelFunc()
		}
		delete(m.sessionRuntimes, oldID)
	}
	// Create a new session first
	if _, err := m.session.Start(m.model); err != nil {
		return true, nil
	}
	// Delete the old one
	_ = m.session.Delete(oldID)
	// Fully reset state
	m.engine = nil
	m.cancelFunc = nil
	m.eventCh = make(chan tuiEvent, 64)
	m.approvalCh = nil
	m.messages = nil
	m.streamText = &strings.Builder{}
	m.streaming = false
	m.turns = 0
	m.totalTokens = 0
	m.totalCost = 0
	m.usageTracker = api.NewUsageTracker(m.model, 0)
	m.expandedGroups = make(map[int]bool)
	m.lastToolGroup = -1
	m.toolStartTimes = make(map[string]time.Time)
	m.spinText = ""
	m.spinner.Stop()
	m.messageQueue = nil
	m.pendingEngineMessages = nil
	m.resumeSummarySet = false
	m.addMessage(ChatMessage{Type: MsgSystem, Content: fmt.Sprintf("Deleted session: %s", oldTitle)})
	m.refreshViewport()
	return true, nil
}


func (m *Model) renameCurrentSession() (bool, tea.Cmd) {
	m.addMessage(ChatMessage{Type: MsgSystem, Content: "Use /rename <title> to rename this session"})
	m.refreshViewport()
	return true, nil
}

// handlePanelToggleByKey is kept for backward compatibility but now unused.
// Panel toggles are dispatched through dispatchAction via the keymap.

// ── Panel Management ────────────────────────────────────

func (m *Model) openPanel(id PanelID) {
	// Reuse a pooled instance if available, otherwise create and cache it.
	panel, ok := m.panelPool[id]
	if !ok {
		panel = m.createPanel(id)
		if panel == nil {
			return
		}
		m.panelPool[id] = panel
	}
	m.activePanel = panel
	m.activePanelID = id
	m.lastPanelID = id
	m.activePanel.Activate()
	m.focus = FocusPanel
	m.prompt.Blur()
	m.layout()
	m.refreshViewport()
}

// openAgentDetail opens the full-screen agent detail overlay.
func (m *Model) openAgentDetail(agentID string) (Model, tea.Cmd) {
	if m.appCtx == nil || m.appCtx.TeamRunner == nil {
		return *m, nil
	}
	state, ok := m.appCtx.TeamRunner.GetState(agentID)
	if !ok {
		return *m, nil
	}
	m.agentDetail = &agentDetailOverlay{
		state:     state,
		scroll:    0,
		toolCalls: []ToolCallEntry{},
	}
	m.prevFocus = m.focus
	m.focus = FocusAgentDetail
	m.prompt.Blur()
	return *m, nil
}

// handleTeammateEvent renders agent lifecycle events inline in the main chat.
// Returns a tea.Cmd when the agents panel needs to be opened/ticked.
func (m *Model) handleTeammateEvent(event teams.TeammateEvent) tea.Cmd {
	nameStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(event.Color))
	name := nameStyle.Render(event.AgentName)

	switch event.Type {
	case "started":
		task := event.Text
		if len(task) > 80 {
			task = task[:77] + "..."
		}
		m.addMessage(ChatMessage{
			Type:    MsgSystem,
			Content: fmt.Sprintf("◐ %s started — %s", name, task),
		})
		// Refresh both the team panel and the AGUI panel so the new agent appears immediately.
		var cmds []tea.Cmd
		cmds = append(cmds, teampanel.ScheduleRefresh())
		if m.activePanel != nil {
			if ap, ok := m.activePanel.(*agui.Panel); ok {
				ap.HandleRefresh()
				cmds = append(cmds, agui.ScheduleRefresh())
			}
		}
		if len(cmds) > 0 {
			return tea.Batch(cmds...)
		}
	case "complete":
		result := event.Text
		if result == "" {
			result = "done"
		}
		m.addMessage(ChatMessage{
			Type:    MsgSystem,
			Content: fmt.Sprintf("● %s finished — %s", name, result),
		})
		// Refresh AGUI panel so completed agents update their status immediately.
		if m.activePanel != nil {
			if ap, ok := m.activePanel.(*agui.Panel); ok {
				ap.HandleRefresh()
			}
		}
		return nil
	case "tool_start":
		// Wire tool_start into the agent detail overlay if it's currently open
		if m.agentDetail != nil && m.agentDetail.state.Identity.AgentID == event.AgentID {
			entry := ToolCallEntry{
				ToolName: event.ToolName,
				Input:    event.Input,
				Output:   "",
				Status:   "running",
				IsSkill:  event.ToolName == "Skill",
			}
			m.agentDetail.toolCalls = append(m.agentDetail.toolCalls, entry)
			// Cap at 20 entries
			if len(m.agentDetail.toolCalls) > 20 {
				m.agentDetail.toolCalls = m.agentDetail.toolCalls[len(m.agentDetail.toolCalls)-20:]
			}
		}
		// Also route to AGUI panel if it's the active panel
		if m.activePanel != nil {
			if ap, ok := m.activePanel.(*agui.Panel); ok {
				ap.HandleTeammateEvent(event)
			}
		}
	case "tool_end":
		// Wire tool_end into the agent detail overlay if it's currently open
		if m.agentDetail != nil && m.agentDetail.state.Identity.AgentID == event.AgentID {
			// Find the last running entry for this tool and update it
			for i := len(m.agentDetail.toolCalls) - 1; i >= 0; i-- {
				if m.agentDetail.toolCalls[i].ToolName == event.ToolName && m.agentDetail.toolCalls[i].Status == "running" {
					m.agentDetail.toolCalls[i].Output = event.Text
					m.agentDetail.toolCalls[i].Status = "done"
					break
				}
			}
		}
		// Also route to AGUI panel if it's the active panel
		if m.activePanel != nil {
			if ap, ok := m.activePanel.(*agui.Panel); ok {
				ap.HandleTeammateEvent(event)
			}
		}
	case "text":
		// Stream text into the AGUI panel detail view if it's open and showing this agent.
		if m.activePanel != nil {
			if ap, ok := m.activePanel.(*agui.Panel); ok {
				return ap.HandleTeammateEvent(event)
			}
		}
	case "warning":
		m.addMessage(ChatMessage{
			Type:    MsgSystem,
			Content: fmt.Sprintf("⚠ %s — %s", name, event.Text),
		})
	case "error":
		m.addMessage(ChatMessage{
			Type:    MsgError,
			Content: fmt.Sprintf("✗ %s failed — %s", name, event.Text),
		})
		// Also route error to AGUI panel.
		if m.activePanel != nil {
			if ap, ok := m.activePanel.(*agui.Panel); ok {
				ap.HandleTeammateEvent(event)
			}
		}
	}
	return nil
}

// handleAgentMessage handles >>agent message syntax.
func (m Model) handleAgentMessage(text string) (tea.Model, tea.Cmd) {
	if m.appCtx == nil || m.appCtx.TeamRunner == nil {
		m.addMessage(ChatMessage{Type: MsgError, Content: "No active team"})
		m.refreshViewport()
		return m, nil
	}

	// Parse >>agentname message
	rest := text[2:] // strip >>
	parts := strings.SplitN(rest, " ", 2)
	agentName := parts[0]
	message := ""
	if len(parts) > 1 {
		message = parts[1]
	}
	if agentName == "" {
		m.addMessage(ChatMessage{Type: MsgError, Content: "Usage: >>agentname message"})
		m.refreshViewport()
		return m, nil
	}
	if message == "" {
		m.addMessage(ChatMessage{Type: MsgError, Content: "Message cannot be empty"})
		m.refreshViewport()
		return m, nil
	}

	mailbox := m.appCtx.TeamRunner.GetMailbox()
	if mailbox == nil {
		m.addMessage(ChatMessage{Type: MsgError, Content: "No mailbox available"})
		m.refreshViewport()
		return m, nil
	}

	// Handle >>all for broadcast
	if agentName == "all" {
		// Send to all agents
		states := m.appCtx.TeamRunner.AllStates()
		for _, s := range states {
			_ = m.appCtx.TeamRunner.SendMessage(s.Identity.AgentName, teams.Message{
				Text:    message,
				Summary: "from you: " + truncateStr(message, 50),
			})
			s.AddConversation(teams.ConversationEntry{
				Time:    time.Now(),
				Type:    "message_in",
				Content: message,
			})
		}
		m.addMessage(ChatMessage{
			Type:    MsgSystem,
			Content: fmt.Sprintf("✉ you → all: %s", message),
		})
		m.refreshViewport()
		return m, nil
	}

	// Send to specific agent
	state, ok := m.appCtx.TeamRunner.GetStateByName(agentName)
	if !ok {
		m.addMessage(ChatMessage{Type: MsgError, Content: fmt.Sprintf("Agent '%s' not found", agentName)})
		m.refreshViewport()
		return m, nil
	}

	if err := m.appCtx.TeamRunner.SendMessage(agentName, teams.Message{
		Text:    message,
		Summary: "from you: " + truncateStr(message, 50),
	}); err != nil {
		m.addMessage(ChatMessage{Type: MsgError, Content: fmt.Sprintf("Failed to send message: %s", err)})
		m.refreshViewport()
		return m, nil
	}
	state.AddConversation(teams.ConversationEntry{
		Time:    time.Now(),
		Type:    "message_in",
		Content: message,
	})

	// If the agent has already completed, revive it so it picks up the new
	// message and continues the conversation.
	revivedNote := ""
	if err := m.appCtx.TeamRunner.Revive(agentName, message); err == nil {
		if st, ok := m.appCtx.TeamRunner.GetStateByName(agentName); ok && st.Status == teams.StatusWorking && st.IsIdle == false {
			revivedNote = " (revived)"
		}
	}

	m.addMessage(ChatMessage{
		Type:    MsgSystem,
		Content: fmt.Sprintf("✉ you → %s: %s%s", agentName, message, revivedNote),
	})
	m.refreshViewport()
	return m, nil
}

// handleAgentDetailKey handles keys when the agent detail overlay is focused.
func (m *Model) handleAgentDetailKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.agentDetail = nil
		m.focus = m.prevFocus
		if m.focus == FocusPrompt {
			m.prompt.Focus()
		}
		return *m, nil
	case "j", "down":
		m.agentDetail.scroll++
		return *m, nil
	case "k", "up":
		if m.agentDetail.scroll > 0 {
			m.agentDetail.scroll--
		}
		return *m, nil
	case "G":
		// Jump to bottom
		m.agentDetail.scroll = 999999
		return *m, nil
	case "g":
		// Jump to top
		m.agentDetail.scroll = 0
		return *m, nil
	case "m":
		// Message this agent
		name := m.agentDetail.state.Identity.AgentName
		m.agentDetail = nil
		m.focus = FocusPrompt
		m.prompt.Focus()
		m.prompt.SetValue(">>" + name + " ")
		return *m, nil
	}
	return *m, nil
}

// renderAgentDetail renders the full-screen WhatsApp-style conversation view.
func (m Model) renderAgentDetail(width, height int) string {
	if m.agentDetail == nil {
		return ""
	}
	state := m.agentDetail.state
	conversation := state.GetConversation()
	progress := state.GetProgress()

	var b strings.Builder

	// Header
	icon := "◐"
	statusText := "working"
	statusColor := styles.Warning
	switch state.Status {
	case teams.StatusComplete:
		icon = "●"
		statusText = "done"
		statusColor = styles.Success
	case teams.StatusFailed:
		icon = "✗"
		statusText = "failed"
		statusColor = styles.Error
	case teams.StatusShutdown:
		icon = "⊘"
		statusText = "stopped"
		statusColor = styles.Muted
	}
	dur := time.Since(state.StartedAt).Truncate(time.Second)

	nameStyle := styles.AgentDetailNameStyle.Copy().Foreground(lipgloss.Color(state.Identity.Color))
	headerLeft := fmt.Sprintf(" %s %s · %s %s · %dt",
		icon,
		nameStyle.Render(state.Identity.AgentName),
		lipgloss.NewStyle().Foreground(statusColor).Render(statusText),
		styles.AgentDetailInfoStyle.Copy().PaddingLeft(0).Render(dur.String()),
		progress.ToolCalls,
	)
	escHint := styles.AgentDetailEscHint.Render("[ESC]")
	headerRight := escHint
	padding := width - lipgloss.Width(headerLeft) - lipgloss.Width(headerRight) - 1
	if padding < 1 {
		padding = 1
	}
	b.WriteString(headerLeft + strings.Repeat(" ", padding) + headerRight + "\n")
	b.WriteString(styles.AgentDetailDimLine.Render(strings.Repeat("─", width)) + "\n")

	// Info lines: model, max_turns, prompt
	if state.Model != "" {
		b.WriteString(styles.AgentDetailInfoStyle.Render("Model: "+state.Model) + "\n")
	}
	if state.MaxTurns > 0 {
		b.WriteString(styles.AgentDetailInfoStyle.Render(fmt.Sprintf("Max turns: %d", state.MaxTurns)) + "\n")
	}

	// Task / prompt
	task := state.Prompt
	if len(task) > width-4 {
		task = task[:width-7] + "..."
	}
	b.WriteString(styles.AgentDetailTaskStyle.Render("Task: "+task) + "\n\n")

	// Conversation entries (including tool call feed)
	contentLines := make([]string, 0, len(conversation)*3)

	// Render tool calls feed if available
	if len(m.agentDetail.toolCalls) > 0 {
		contentLines = append(contentLines, styles.AgentDetailInfoStyle.Render("─ tool calls ─"))
		for _, tc := range m.agentDetail.toolCalls {
			contentLines = append(contentLines, m.renderToolCallEntry(tc, width-2))
		}
		contentLines = append(contentLines, "")
	}
	for _, entry := range conversation {
		lines := m.renderConversationEntry(entry, width-2)
		contentLines = append(contentLines, lines...)
	}

	// Working indicator at bottom
	if state.Status == teams.StatusWorking {
		contentLines = append(contentLines, "")
		contentLines = append(contentLines, styles.AgentDetailWorkingStyle.Render("◐ working..."))
	}

	// Apply scroll
	visibleH := height - 5 // header + task + hints
	if visibleH < 3 {
		visibleH = 3
	}

	// Clamp scroll
	maxScroll := len(contentLines) - visibleH
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.agentDetail.scroll > maxScroll {
		m.agentDetail.scroll = maxScroll
	}
	scroll := m.agentDetail.scroll

	end := scroll + visibleH
	if end > len(contentLines) {
		end = len(contentLines)
	}
	start := scroll
	if start > len(contentLines) {
		start = len(contentLines)
	}

	for _, line := range contentLines[start:end] {
		b.WriteString(line + "\n")
	}

	// Pad remaining height
	currentLines := 4 + (end - start) // header(2) + task(2) + content
	for i := currentLines; i < height-1; i++ {
		b.WriteString("\n")
	}

	// Hint bar
	b.WriteString(styles.AgentDetailDimLine.Render(strings.Repeat("─", width)) + "\n")
	b.WriteString(styles.AgentDetailTaskStyle.Render("j/k scroll  m message  ESC back"))

	return b.String()
}

// renderConversationEntry renders a single conversation entry.
func (m Model) renderConversationEntry(entry teams.ConversationEntry, width int) []string {
	var lines []string

	switch entry.Type {
	case "text":
		// Agent's thinking/response text
		lines = append(lines, styles.AgentDetailAgentStyle.Render("● assistant"))
		// Wrap long text
		for _, paragraph := range strings.Split(entry.Content, "\n") {
			if len(paragraph) > width-4 {
				// Simple word wrap
				for len(paragraph) > width-4 {
					cut := width - 4
					for cut > 0 && paragraph[cut] != ' ' {
						cut--
					}
					if cut == 0 {
						cut = width - 4
					}
					lines = append(lines, styles.AgentDetailTextStyle.Render(paragraph[:cut]))
					paragraph = paragraph[cut:]
				}
			}
			lines = append(lines, styles.AgentDetailTextStyle.Render(paragraph))
		}
		lines = append(lines, "")

	case "tool_start":
		box := fmt.Sprintf("┌─ %s", entry.ToolName)
		if entry.Content != "" {
			content := entry.Content
			if len(content) > width-8 {
				content = content[:width-11] + "..."
			}
			box += fmt.Sprintf("(%s)", content)
		}
		lines = append(lines, styles.AgentDetailToolStyle.Render(box))

	case "tool_end":
		content := entry.Content
		if content != "" {
			// Show truncated result
			resultLines := strings.Split(content, "\n")
			maxLines := 5
			if len(resultLines) > maxLines {
				for _, rl := range resultLines[:maxLines] {
					if len(rl) > width-8 {
						rl = rl[:width-11] + "..."
					}
					lines = append(lines, styles.AgentDetailToolDimStyle.Render("│ "+rl))
				}
				lines = append(lines, styles.AgentDetailToolDimStyle.Render(fmt.Sprintf("│ ... (%d more lines)", len(resultLines)-maxLines)))
			} else {
				for _, rl := range resultLines {
					if len(rl) > width-8 {
						rl = rl[:width-11] + "..."
					}
					lines = append(lines, styles.AgentDetailToolDimStyle.Render("│ "+rl))
				}
			}
		}
		lines = append(lines, styles.AgentDetailToolDimStyle.Render("└─"))
		lines = append(lines, "")

	case "complete":
		lines = append(lines, styles.AgentDetailDoneStyle.Render("✓ Complete"))
		if entry.Content != "" {
			for _, line := range strings.Split(entry.Content, "\n") {
				if len(line) > width-6 {
					line = line[:width-9] + "..."
				}
				lines = append(lines, styles.AgentDetailContentStyle.Render(line))
			}
		}
		lines = append(lines, "")

	case "error":
		lines = append(lines, styles.AgentDetailErrorStyle.Render("✗ Error: "+entry.Content))
		lines = append(lines, "")

	case "message_in":
		lines = append(lines, styles.AgentDetailMessageStyle.Render("┌─ ✉ from you"))
		lines = append(lines, styles.AgentDetailMessageStyle.Render("│ "+entry.Content))
		lines = append(lines, styles.AgentDetailMessageStyle.Render("└─"))
		lines = append(lines, "")
	}

	return lines
}

// renderToolCallEntry renders a single tool call entry for the tool call feed.
func (m Model) renderToolCallEntry(tc ToolCallEntry, width int) string {
	var parts []string

	// Tool name with icon/color
	toolName := tc.ToolName
	if tc.IsSkill {
		// Skill calls: use orange accent color with ★ prefix
		toolName = styles.ToolBadge.Render("★ " + toolName)
	} else {
		// Regular tool calls: use Warning color (already defined style)
		toolName = styles.AgentDetailToolStyle.Render(toolName)
	}

	// Status indicator
	var statusBadge string
	switch tc.Status {
	case "running":
		// Use spinner character for running status
		statusBadge = lipgloss.NewStyle().Foreground(styles.Warning).Render("⠋ running")
	case "done":
		statusBadge = styles.AgentDetailDoneStyle.Render("✓ done")
	case "error":
		statusBadge = styles.AgentDetailErrorStyle.Render("✗ error")
	default:
		statusBadge = lipgloss.NewStyle().Foreground(styles.Muted).Render(tc.Status)
	}

	// Build the line with tool name and status
	line := fmt.Sprintf("  %s  %s", toolName, statusBadge)

	// Add truncated input if available
	if tc.Input != "" {
		input := tc.Input
		if len(input) > 80 {
			input = input[:77] + "..."
		}
		inputLine := fmt.Sprintf("    input: %s", styles.AgentDetailContentStyle.Render(input))
		parts = append(parts, line)
		parts = append(parts, inputLine)
	} else {
		parts = append(parts, line)
	}

	// Add truncated output if available
	if tc.Output != "" {
		output := tc.Output
		if len(output) > 120 {
			output = output[:117] + "..."
		}
		outputLine := fmt.Sprintf("    output: %s", styles.AgentDetailContentStyle.Render(output))
		parts = append(parts, outputLine)
	}

	return strings.Join(parts, "\n")
}

// applyConfigChange applies a config change to the live session immediately.
func (m *Model) applyConfigChange(key, value string) {
	switch key {
	case "model":
		m.model = value
		m.apiClient.SetModel(value)
		m.addMessage(ChatMessage{Type: MsgSystem, Content: fmt.Sprintf("Model changed to %s", value)})
		m.refreshViewport()
	case "permissionMode":
		if m.engineConfig != nil {
			m.engineConfig.PermissionMode = value
		}
	case "outputStyle":
		if m.appCtx != nil && m.appCtx.Config != nil {
			m.appCtx.Config.OutputStyle = value
		}
	case "outputFilter":
		enabled := value == "true"
		if m.appCtx != nil && m.appCtx.Config != nil {
			m.appCtx.Config.OutputFilter = enabled
		}
		if bash, err := m.registry.Get("Bash"); err == nil {
			if bt, ok := bash.(*tools.BashTool); ok {
				bt.OutputFilterEnabled = enabled
			}
		}
	}
	// Other settings (autoMemoryExtract, memorySelection, compactMode, etc.)
	// are read from config at the point of use, so saving to disk is sufficient.
}

func (m *Model) closePanel() {
	if m.activePanel != nil {
		// Deactivate but keep instance in the pool so state is preserved on reopen.
		m.activePanel.Deactivate()
	}
	m.activePanel = nil
	m.activePanelID = PanelNone
	m.focus = FocusPrompt
	m.prompt.Focus()
	m.layout()
	m.refreshViewport()
}

// buildFullSystemPrompt reconstructs the system prompt with all context.
// It gathers rules, learning, output style, snippets, and plugins,
// then calls BuildSystemPrompt with the full additionalContext.
func (m *Model) buildFullSystemPrompt() string {
	var sections []string

	// Add rules
	if m.appCtx != nil && m.appCtx.Rules != nil {
		if rulesContent := m.appCtx.Rules.ForSystemPrompt(); rulesContent != "" {
			sections = append(sections, rulesContent)
		}
	}

	// Add learning/instinct
	if m.appCtx != nil && m.appCtx.Learning != nil {
		cwd, _ := os.Getwd()
		if instinctContent := m.appCtx.Learning.ForSystemPrompt(cwd); instinctContent != "" {
			sections = append(sections, instinctContent)
		}
	}

	// Add output style
	if m.appCtx != nil && m.appCtx.Config != nil && m.appCtx.Config.OutputStyle != "" {
		if styleContent := prompts.OutputStyleSection(prompts.OutputStyle(m.appCtx.Config.OutputStyle)); styleContent != "" {
			sections = append(sections, styleContent)
		}
	}

	// Add snippets
	if m.appCtx != nil && m.appCtx.Config != nil {
		if snippetSection := snippets.ForSystemPrompt(m.appCtx.Config.Snippets); snippetSection != "" {
			sections = append(sections, snippetSection)
		}
	}

	additionalCtx := strings.Join(sections, "\n\n")
	return prompts.BuildSystemPrompt(m.model, additionalCtx)
}

// createPanel instantiates the appropriate panel for the given ID.
// Returns nil if the panel cannot be created (e.g., missing dependencies).
func (m *Model) createPanel(id PanelID) panels.Panel {
	switch id {
	case PanelSessions:
		return nil // Sessions use Telescope-style overlay, not side panel
	case PanelConfig:
		if m.appCtx != nil && m.appCtx.Config != nil {
			return panelconfig.New(m.appCtx.Config)
		}
	case PanelSkills:
		if m.skills != nil {
			return skillspanel.New(m.skills)
		}
	case PanelMemory:
		if m.appCtx != nil && m.appCtx.Memory != nil {
			return memorypanel.New(m.appCtx.Memory)
		}
	case PanelAnalytics:
		if m.appCtx != nil && m.appCtx.Analytics != nil {
			return analyticspanel.New(m.appCtx.Analytics)
		}
	case PanelTasks:
		if m.appCtx != nil && m.appCtx.TaskRuntime != nil {
			return taskspanel.New(m.appCtx.TaskRuntime)
		}
	case PanelAgents:
		if m.appCtx != nil && m.appCtx.TeamManager != nil && m.appCtx.TeamRunner != nil {
			p := teampanel.New(m.appCtx.TeamManager, m.appCtx.TeamRunner)
			return p
		}
	case PanelTools:
		if m.registry != nil {
			return toolspanel.New(m.registry)
		}
	case PanelConversation:
		return conversationpanel.New()
	case PanelSessionTree:
		if m.appCtx != nil && m.appCtx.DB != nil {
			var memSvc *memory.ScopedStore
			if m.appCtx.Memory != nil {
				memSvc = m.appCtx.Memory
			}
			getCurrentID := func() string {
				if m.session != nil {
					if cur := m.session.Current(); cur != nil {
						return cur.ID
					}
				}
				return ""
			}
			return stree.New(m.appCtx.DB, memSvc, getCurrentID)
		}
	case PanelAgentGUI:
		var runner *teams.TeammateRunner
		var manager *teams.Manager
		if m.appCtx != nil {
			runner = m.appCtx.TeamRunner
			manager = m.appCtx.TeamManager
		}
		return agui.New(runner, manager)
	}
	return nil
}

// ── Message Management ───────────────────────────────────

// extractSectionText extracts plain text content from a section for viewing in editor.
func (m *Model) extractSectionText(section Section) string {
	if section.MsgIndex < 0 || section.MsgIndex >= len(m.messages) {
		return ""
	}

	msg := m.messages[section.MsgIndex]

	// For tool groups: extract all tool names and their results
	if section.IsToolGroup {
		var sb strings.Builder
		i := section.MsgIndex
		for i < len(m.messages) {
			if m.messages[i].Type == MsgToolUse {
				if sb.Len() > 0 {
					sb.WriteString("\n")
				}
				sb.WriteString("Tool: ")
				sb.WriteString(m.messages[i].ToolName)
				sb.WriteString("\n")
				if m.messages[i].ToolInput != "" {
					sb.WriteString("Input: ")
					sb.WriteString(m.messages[i].ToolInput)
					sb.WriteString("\n")
				}
				// Look for matching result
				toolID := m.messages[i].ToolUseID
				for j := i + 1; j < len(m.messages) && j-i < 50; j++ {
					if m.messages[j].Type == MsgToolResult && m.messages[j].ToolUseID == toolID {
						if m.messages[j].IsError {
							sb.WriteString("Error: ")
						} else {
							sb.WriteString("Result: ")
						}
						sb.WriteString(m.messages[j].Content)
						sb.WriteString("\n")
						break
					}
				}
				i++
			} else if m.messages[i].Type == MsgToolResult {
				i++
			} else {
				break
			}
		}
		return sb.String()
	}

	// For regular messages: return the content directly
	if msg.Content != "" {
		return msg.Content
	}

	// Fallback: try to use tool input summary if it's a tool message
	if msg.ToolInput != "" {
		return msg.ToolInput
	}

	return ""
}

func (m *Model) addMessage(msg ChatMessage) {
	m.messages = append(m.messages, msg)
	// Persist to DB
	m.persistMessage(msg)
}

func (m *Model) persistMessage(msg ChatMessage) {
	if m.session == nil || m.session.Current() == nil {
		return
	}
	// Only persist user input and assistant responses — not system/error UI messages
	var role, msgType string
	switch msg.Type {
	case MsgUser:
		role, msgType = "user", "user"
	case MsgAssistant:
		role, msgType = "assistant", "assistant"
	case MsgToolUse:
		role, msgType = "assistant", "tool_use"
	case MsgToolResult:
		role, msgType = "user", "tool_result"
	default:
		return // Don't persist system/error/thinking messages
	}
	if msg.ToolName != "" || msg.ToolUseID != "" {
		content := msg.Content
		// tool_use messages store their input in ToolInputRaw, not Content.
		if msg.Type == MsgToolUse && len(msg.ToolInputRaw) > 0 {
			content = string(msg.ToolInputRaw)
		}
		m.session.AddToolMessage(role, content, msgType, msg.ToolUseID, msg.ToolName)
	} else {
		m.session.AddMessage(role, msg.Content, msgType)
	}
}

// deleteInteraction removes the interaction (user turn + assistant/tool responses)
// that contains the message at the given ChatMessage index.
// It updates m.messages, the engine's message history, and the DB.
func (m *Model) deleteInteraction(msgIdx int) {
	if msgIdx < 0 || msgIdx >= len(m.messages) {
		return
	}

	// Find the start of this interaction: walk backwards to find the user message.
	start := msgIdx
	for start > 0 && m.messages[start].Type != MsgUser {
		start--
	}
	// If we didn't land on a user message (e.g., leading assistant messages), start from 0.
	if m.messages[start].Type != MsgUser && start == 0 {
		// Allow deleting orphan assistant responses at the beginning
	}

	// Find the end: walk forward until the next user message or end of list.
	end := start + 1
	for end < len(m.messages) && m.messages[end].Type != MsgUser {
		end++
	}

	// Remove from chat messages.
	m.messages = append(m.messages[:start], m.messages[end:]...)

	// Rebuild engine messages from the remaining chat messages.
	if m.engine != nil {
		m.engine.SetMessages(engineMessagesFromChat(m.messages))
	}

	// Update pinned indices: shift down any pins above the deleted range.
	if len(m.pinnedMsgIndices) > 0 {
		newPinned := make(map[int]bool)
		for idx, v := range m.pinnedMsgIndices {
			if idx < start {
				newPinned[idx] = v
			} else if idx >= end {
				newPinned[idx-(end-start)] = v
			}
		}
		m.pinnedMsgIndices = newPinned
	}

	// Update expanded groups: shift down indices above deleted range, remove deleted.
	if len(m.expandedGroups) > 0 {
		newExpanded := make(map[int]bool)
		for idx, v := range m.expandedGroups {
			if idx < start {
				newExpanded[idx] = v
			} else if idx >= end {
				newExpanded[idx-(end-start)] = v
			}
		}
		m.expandedGroups = newExpanded
	}

	// Fix lastToolGroup index.
	if m.lastToolGroup >= start && m.lastToolGroup < end {
		m.lastToolGroup = -1
	} else if m.lastToolGroup >= end {
		m.lastToolGroup -= end - start
	}

	// Re-persist: delete all messages for the session and re-add the remaining ones.
	if m.session != nil && m.session.Current() != nil {
		_ = m.session.DeleteAllMessages()
		for _, msg := range m.messages {
			m.persistMessage(msg)
		}
	}
}

// engineMessagesFromChat rebuilds engine-compatible []api.Message from ChatMessage slice.
// This mirrors the grouping logic of reconstructEngineMessages but works from in-memory
// ChatMessages instead of DB records.
func engineMessagesFromChat(msgs []ChatMessage) []api.Message {
	type trBlock struct {
		Type      string `json:"type"`
		ToolUseID string `json:"tool_use_id"`
		Content   string `json:"content"`
	}

	var result []api.Message
	var pendingIDs []string
	tuCounter := 0

	i := 0
	for i < len(msgs) {
		msg := msgs[i]
		switch msg.Type {
		case MsgUser:
			content, _ := json.Marshal([]api.UserContentBlock{api.NewTextBlock(msg.Content)})
			result = append(result, api.Message{Role: "user", Content: content})
			i++

		case MsgAssistant:
			var blocks []api.ContentBlock
			if msg.Content != "" {
				blocks = append(blocks, api.ContentBlock{Type: "text", Text: msg.Content})
			}
			i++
			// Consume following tool_use messages into the same assistant message.
			pendingIDs = nil
			for i < len(msgs) && msgs[i].Type == MsgToolUse {
				id := msgs[i].ToolUseID
				if id == "" {
					tuCounter++
					id = fmt.Sprintf("toolu_%04d", tuCounter)
				}
				pendingIDs = append(pendingIDs, id)
				input := json.RawMessage(msgs[i].ToolInputRaw)
				if len(input) == 0 || !json.Valid(input) {
					input = json.RawMessage("{}")
				}
				blocks = append(blocks, api.ContentBlock{
					Type:  "tool_use",
					ID:    id,
					Name:  msgs[i].ToolName,
					Input: input,
				})
				i++
			}
			if len(blocks) > 0 {
				content, _ := json.Marshal(blocks)
				result = append(result, api.Message{Role: "assistant", Content: content})
			}

		case MsgToolResult:
			// Skip orphaned tool_results with no preceding tool_use.
			if len(pendingIDs) == 0 {
				for i < len(msgs) && msgs[i].Type == MsgToolResult {
					i++
				}
				continue
			}
			var trs []trBlock
			j := 0
			for i < len(msgs) && msgs[i].Type == MsgToolResult {
				id := msgs[i].ToolUseID
				if id == "" {
					if j < len(pendingIDs) {
						id = pendingIDs[j]
					} else {
						tuCounter++
						id = fmt.Sprintf("toolu_%04d", tuCounter)
					}
				}
				trs = append(trs, trBlock{
					Type:      "tool_result",
					ToolUseID: id,
					Content:   msgs[i].Content,
				})
				i++
				j++
			}
			if len(trs) > 0 {
				content, _ := json.Marshal(trs)
				result = append(result, api.Message{Role: "user", Content: content})
			}
			pendingIDs = nil

		default:
			// Skip system/error/thinking messages — they don't map to engine messages.
			i++
		}
	}
	return session.SanitizeToolPairs(result)
}

func (m *Model) updateStreamingMessage() {
	text := m.streamText.String()
	if text == "" {
		return
	}
	if len(m.messages) > 0 && m.messages[len(m.messages)-1].Type == MsgAssistant {
		// Update in place — don't persist yet (finalize will do it)
		m.messages[len(m.messages)-1].Content = text
	} else {
		// First chunk — append without persisting (finalize will do it)
		m.messages = append(m.messages, ChatMessage{Type: MsgAssistant, Content: text})
	}
}

func (m *Model) finalizeStreamingMessage() {
	if m.streamText.Len() > 0 {
		m.updateStreamingMessage()
		// Persist the final assistant message
		if len(m.messages) > 0 && m.messages[len(m.messages)-1].Type == MsgAssistant {
			m.persistMessage(m.messages[len(m.messages)-1])
		}
		m.streamText.Reset()
	}
}

func (m *Model) refreshViewport() {
	var content string

	if len(m.messages) == 0 && !m.streaming {
		content = m.welcomeScreen()
		m.vpSections = nil
	} else {
		cursorIdx := -1
		if m.focus == FocusViewport {
			cursorIdx = m.vpCursor
		}
		msgs := m.messages
		if m.thinkingHidden {
			filtered := make([]ChatMessage, 0, len(msgs))
			for _, msg := range msgs {
				if msg.Type == MsgThinking {
					continue
				}
				filtered = append(filtered, msg)
			}
			msgs = filtered
		}
		result := renderMessages(msgs, m.viewport.Width, m.expandedGroups, cursorIdx, m.toolSpinFrame, m.thinkingExpanded)
		content = result.Content
		m.vpSections = result.Sections

		// Append inline spinner when streaming
		if m.streaming {
			spinView := m.spinner.View()
			if spinView != "" {
				content += "\n\n" + spinView
			}
		}
	}

	m.viewport.SetContent(content)
	// Sync content to the conversation mirror panel if it exists in the pool.
	// When the right window has its own session, render from rightWindow.messages
	// instead of mirroring the main viewport.
	if cp, ok := m.panelPool[PanelConversation]; ok {
		if conv, ok := cp.(*conversationpanel.Panel); ok {
			if m.rightWindowHasOwnSession() {
				rightContent := m.renderRightWindowContent(conv)
				conv.SetContent(rightContent)
			} else {
				conv.SetContent(content)
			}
		}
	}
	if m.focus != FocusViewport {
		contentLines := strings.Count(content, "\n") + 1
		if contentLines <= m.viewport.Height {
			m.viewport.GotoTop()
		} else {
			m.viewport.GotoBottom()
		}
	}

	if m.focus == FocusViewport {
		maxOffset := m.viewport.TotalLineCount() - m.viewport.Height
		if maxOffset < 0 {
			maxOffset = 0
		}
		if m.viewport.YOffset > maxOffset {
			m.viewport.GotoBottom()
		}
	}
}

// renderRightWindowContent renders the right window's session messages into
// a string suitable for the conversation panel viewport.
func (m *Model) renderRightWindowContent(conv *conversationpanel.Panel) string {
	msgs := m.rightWindow.messages
	if len(msgs) == 0 {
		return lipgloss.NewStyle().Foreground(styles.Muted).Render(
			fmt.Sprintf("  Session: %s (no messages)", m.rightWindow.title))
	}
	// Use the panel's width for rendering; pass -1 for cursor (no navigation in mirror)
	panelWidth := conv.Width()
	if panelWidth <= 0 {
		panelWidth = 60
	}
	result := renderMessages(msgs, panelWidth, nil, -1, 0, nil)
	return result.Content
}

// scrollToSection scrolls the viewport so the given section is visible.
func (m *Model) scrollToSection(idx int) {
	if idx < 0 || idx >= len(m.vpSections) {
		return
	}
	sec := m.vpSections[idx]
	vpH := m.viewport.Height

	// If the section is already fully visible, don't scroll
	yOff := m.viewport.YOffset
	if sec.LineStart >= yOff && sec.LineStart+sec.LineCount <= yOff+vpH {
		return
	}

	// Try to center the section vertically, clamping to valid range
	target := sec.LineStart - (vpH / 3)
	if target < 0 {
		target = 0
	}
	m.viewport.SetYOffset(target)
}

// sectionToolGroupIdx returns the message index if the section is a tool group, or -1.
func (m *Model) sectionToolGroupIdx(sectionIdx int) int {
	if sectionIdx < 0 || sectionIdx >= len(m.vpSections) {
		return -1
	}
	sec := m.vpSections[sectionIdx]
	if sec.IsToolGroup {
		return sec.MsgIndex
	}
	return -1
}

// logoColors is the palette used for the welcome-screen title wave animation.
// Each letter is colored based on (logoFrame + charIndex) % len(logoColors).
var logoColors = []lipgloss.Color{
	"#FF6B6B",
	"#FFD93D",
	"#6BCB77",
	"#4D96FF",
	"#9B59B6",
	"#FF8E53",
	"#00C9A7",
}

// animatedLogoWithRenderer renders "claudio" with a color-wave driven by frame,
// using the supplied lipgloss.Renderer. This is the core implementation; it is
// also called directly in tests with a color-forced renderer so that ANSI
// escape codes are emitted even outside a real terminal.
func animatedLogoWithRenderer(frame int, r *lipgloss.Renderer) string {
	const word = "claudio"
	n := len(logoColors)
	var b strings.Builder
	for i, ch := range word {
		color := logoColors[(frame+i)%n]
		b.WriteString(
			r.NewStyle().
				Foreground(color).
				Bold(true).
				Render(string(ch)),
		)
	}
	return b.String()
}

// animatedLogo renders "claudio" using the default lipgloss renderer.
func animatedLogo(frame int) string {
	return animatedLogoWithRenderer(frame, lipgloss.DefaultRenderer())
}

func (m *Model) welcomeScreen() string {
	return m.renderWelcomeScreen()
}

// renderRecentSessions builds the bordered recent sessions box for the welcome screen.
func (m *Model) renderRecentSessions(sessions []storage.Session) string {
	boxW := 52
	if m.viewport.Width < 60 {
		boxW = m.viewport.Width - 8
	}
	if boxW < 30 {
		boxW = 30
	}

	var lines []string
	for i, s := range sessions {
		stitle := s.Title
		if stitle == "" {
			short := s.ID
			if len(short) > 8 {
				short = short[:8]
			}
			stitle = short
		}

		// Time ago
		ago := timeAgo(s.UpdatedAt)

		maxTitle := boxW - len(ago) - 8
		if maxTitle < 10 {
			maxTitle = 10
		}
		if len(stitle) > maxTitle {
			stitle = stitle[:maxTitle-1] + "…"
		}

		left := styles.SessionNumStyle.Render(fmt.Sprintf("  %d  ", i+1)) + styles.SessionTitleStyle.Render(stitle)
		right := styles.SessionDateStyle.Render(ago)
		gap := boxW - lipgloss.Width(left) - lipgloss.Width(right) - 2
		if gap < 1 {
			gap = 1
		}
		lines = append(lines, left+strings.Repeat(" ", gap)+right+"  ")
	}

	lines = append(lines, "")
	lines = append(lines, styles.SessionHintStyle.Render("  [1-3] resume · <Space>. browse · type to chat"))

	content := strings.Join(lines, "\n")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.SurfaceAlt).
		Padding(0, 1).
		Width(boxW).
		Render(content)

	// Inject title into top border
	boxLines := strings.Split(box, "\n")
	if len(boxLines) > 0 {
		label := styles.SessionLabelStyle.Render(" Recent ")
		topRunes := []rune(boxLines[0])
		labelRunes := []rune(label)
		pos := 3
		if pos+len(labelRunes) < len(topRunes) {
			copy(topRunes[pos:], labelRunes)
			boxLines[0] = string(topRunes)
		}
	}

	return strings.Join(boxLines, "\n")
}

// isWelcomeScreen returns true when the welcome screen is showing (no messages, not streaming).
func (m *Model) isWelcomeScreen() bool {
	return len(m.messages) == 0 && !m.streaming
}

// buildSidebar constructs the sidebar from config (or defaults).
func (m *Model) buildSidebar() *sidebar.Sidebar {
	cfg := m.sidebarConfig()
	if !cfg.Enabled {
		return nil
	}

	blockNames := cfg.Blocks
	if len(blockNames) == 0 {
		blockNames = []string{"files", "todos", "tokens"}
	}

	var blks []sidebar.Block
	for _, name := range blockNames {
		switch name {
		case "files":
			blks = append(blks, m.sidebarFiles)
		case "todos":
			blks = append(blks, sidebarblocks.NewTodosBlock())
		case "tokens":
			blks = append(blks, sidebarblocks.NewTokensBlock(
				func() int     { return m.totalTokens },
				func() float64 { return m.totalCost },
				func() int     { return utils.MaxContextForModel(m.model) },
			))
		}
	}
	if len(blks) == 0 {
		return nil
	}
	return sidebar.New(blks...)
}

// sidebarConfig returns the effective sidebar config (from settings or defaults).
func (m *Model) sidebarConfig() config.SidebarConfig {
	if m.appCtx != nil && m.appCtx.Config != nil && m.appCtx.Config.Sidebar != nil {
		c := *m.appCtx.Config.Sidebar
		if c.Width == 0 {
			c.Width = 32
		}
		return c
	}
	// Default: enabled with standard blocks
	return config.SidebarConfig{
		Enabled: true,
		Width:   32,
		Blocks:  []string{"files", "todos", "tokens"},
	}
}

// sidebarWidth returns the pixel width the sidebar occupies (0 if disabled).
func (m *Model) sidebarWidth() int {
	if m.sidebar == nil {
		return 0
	}
	cfg := m.sidebarConfig()
	w := cfg.Width
	if w == 0 {
		w = 32
	}
	// Don't show sidebar if terminal is too narrow
	if m.width-w-1 < 40 {
		return 0
	}
	return w
}

// ── Layout & View ────────────────────────────────────────

func (m *Model) layout() {
	promptH := m.prompt.Height()
	paletteH := 0
	if m.palette.IsActive() || m.filePicker.IsActive() {
		paletteH = 10
	}

	modeLineH := 1
	helpFooterH := 1
	statusLineH := 1 // nvim-style statusline above the prompt
	const topPadding = 1
	vpHeight := m.height - promptH - paletteH - modeLineH - helpFooterH - statusLineH - 1 - topPadding
	if vpHeight < 5 {
		vpHeight = 5
	}

	// Start with full width; panels render as overlays so the viewport
	// always keeps its full width (minus sidebar).
	mw := m.width

	// If files panel is active (and no other side panel), shrink the main viewport to leave room
	hasPanel := m.activePanel != nil && m.activePanel.IsActive()
	if !hasPanel && m.filesPanel != nil && m.filesPanel.IsActive() {
		filesW := int(float64(m.width) * 0.35)
		if filesW < 20 {
			filesW = 20
		}
		if filesW > m.width-20 {
			filesW = m.width - 20
		}
		mw = m.width - filesW - 1
		if mw < 0 {
			mw = 0
		}
		if mw < 10 {
			mw = 10
		}
	}

	// OverlayDrawer panels (e.g. Sessions) shrink the viewport to make room
	if hasPanel && panelOverlayMode(m.activePanelID) == OverlayDrawer {
		drawerW := m.width * 35 / 100
		if drawerW < 30 {
			drawerW = 30
		}
		if drawerW > m.width-20 {
			drawerW = m.width - 20
		}
		mw = m.width - drawerW - 1
		if mw < 20 {
			mw = 20
		}
	}

	// Persistent sidebar shrinks main area when no panel/files panel is active
	if !hasPanel && (m.filesPanel == nil || !m.filesPanel.IsActive()) {
		if sw := m.sidebarWidth(); sw > 0 {
			mw = m.width - sw - 1
			if mw < 20 {
				mw = 20
			}
		}
	}
	m.viewport.Width = mw
	m.viewport.Height = vpHeight
	m.prompt.SetWidth(m.width) // prompt always full width
	m.permission.SetWidth(mw)
}

func (m Model) View() string {
	if m.tooSmall {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
			lipgloss.NewStyle().Foreground(styles.Error).Render("Terminal too small — please resize (min 60×20)"))
	}

	m.layout()

	// layout() already computed viewport dimensions; use them directly.
	mw := m.viewport.Width
	hasPanel := m.activePanel != nil && m.activePanel.IsActive()

	// 1. Viewport (messages + inline spinner)
	vpView := m.viewport.View()

	// Overlay model selector and other dialogs on top of viewport
	if m.focus == FocusPlanApproval {
		overlay := m.renderPlanApprovalDialog(mw)
		vpView = placeOverlay(vpView, overlay, mw, m.viewport.Height)
	}
	if m.focus == FocusAskUser && m.askUserDialog != nil {
		overlay := m.renderAskUserDialog(mw)
		vpView = placeOverlay(vpView, overlay, mw, m.viewport.Height)
	}
	if m.modelSelector.IsActive() {
		overlay := m.modelSelector.View()
		vpView = placeOverlay(vpView, overlay, mw, m.viewport.Height)
	}
	if m.agentSelector.IsActive() {
		overlay := m.agentSelector.View()
		vpView = placeOverlay(vpView, overlay, mw, m.viewport.Height)
	}
	if m.teamSelector.IsActive() {
		overlay := m.teamSelector.View()
		vpView = placeOverlay(vpView, overlay, mw, m.viewport.Height)
	}
	if m.whichKey.IsActive() {
		overlay := m.whichKey.View()
		vpView = placeOverlay(vpView, overlay, mw, m.viewport.Height)
	}
	if m.sessionPicker != nil && m.sessionPicker.IsActive() {
		m.sessionPicker.SetSize(m.width, m.viewport.Height)
		overlay := m.sessionPicker.View()
		vpView = placeOverlay(vpView, overlay, m.width, m.viewport.Height)
	}
	// Full-screen agent detail overlay replaces viewport entirely
	if m.focus == FocusAgentDetail && m.agentDetail != nil {
		vpView = m.renderAgentDetail(m.width, m.viewport.Height)
	}

	// 2. If panel is active, render as overlay on top of the viewport
	topArea := vpView
	if hasPanel && m.focus != FocusAgentDetail {
		mode := panelOverlayMode(m.activePanelID)
		switch mode {
		case OverlayCentered:
			w := m.viewport.Width * 70 / 100
			h := m.viewport.Height * 70 / 100
			if w < 40 {
				w = 40
			}
			if h < 10 {
				h = 10
			}
			if w > m.viewport.Width-2 {
				w = m.viewport.Width - 2
			}
			if h > m.viewport.Height-2 {
				h = m.viewport.Height - 2
			}
			panelView := renderPanelWithHelp(m.activePanel, w, h)
			topArea = placeOverlay(topArea, panelView, m.viewport.Width, m.viewport.Height)
		case OverlayDrawer:
			drawerW := m.width * 35 / 100
			if drawerW < 30 {
				drawerW = 30
			}
			if drawerW > m.width-20 {
				drawerW = m.width - 20
			}
			panelView := renderPanelWithHelp(m.activePanel, drawerW, m.viewport.Height)
			topArea = placeOverlayAt(topArea, panelView, 0, 0, mw, m.viewport.Height)
		case OverlayFullscreen:
			topArea = renderPanelWithHelp(m.activePanel, m.viewport.Width, m.viewport.Height)
		}
	} else if m.filesPanel != nil && m.filesPanel.IsActive() && m.focus != FocusAgentDetail {
		// layout() already sized the viewport; files panel takes the remainder
		filesW := m.width - mw - 1
		if filesW < 10 {
			filesW = 10
		}
		m.filesPanel.SetSize(filesW, m.viewport.Height)
		topArea = lipgloss.JoinHorizontal(lipgloss.Top, vpView, m.filesPanel.View())
	} else if m.sidebar != nil && m.focus != FocusAgentDetail {
		// Rebuild sidebar each frame so token/cost closures capture
		// the current (value-receiver) Model copy. Otherwise the
		// closures formed in New() reference a stale stack-local m
		// and the Usage panel always renders zeros.
		m.sidebar = m.buildSidebar()
		sw := m.sidebarWidth()
		if sw > 0 {
			m.sidebar.SetSize(sw, m.viewport.Height)
			sep := buildSeparator(m.viewport.Height)
			sidebarView := lipgloss.NewStyle().Width(sw).Height(m.viewport.Height).Render(m.sidebar.View())
			topArea = lipgloss.JoinHorizontal(lipgloss.Top, vpView, sep, sidebarView)
		}
	}

	var sections []string
	sections = append(sections, lipgloss.NewStyle().Height(1).Render("")) // top padding — prevents content from being clipped at terminal edge
	sections = append(sections, topArea)

	// 3. Command palette or file picker (full width, between viewport and prompt)
	if paletteView := m.palette.View(); paletteView != "" {
		sections = append(sections, paletteView)
	}
	if pickerView := m.filePicker.View(); pickerView != "" {
		sections = append(sections, pickerView)
	}

	// 4. Dock slot — permission dock (highest priority) or todo dock
	if m.permission.IsActive() {
		m.permission.SetWidth(mw)
		if dockView := m.permission.InlineView(); dockView != "" {
			sections = append(sections, dockView)
		}
	} else if m.todoDock != nil {
		m.todoDock.SetWidth(mw)
		if dockView := m.todoDock.View(); dockView != "" {
			sections = append(sections, dockView)
		}
	}

	// 5. Statusline (full width) — shows mode, session, model; replaced by toast when active
	sections = append(sections, m.renderStatusLine())

	// 6. Prompt (full width)
	sections = append(sections, m.prompt.View())

	// 7. Mode line (full width) — or search bar when searching
	if m.vpSearchActive {
		sections = append(sections, m.renderSearchBar())
	} else {
		sections = append(sections, m.renderModeLine())
	}

	// 8. Help footer (full width)
	sections = append(sections, m.renderHelpFooter())

	// 9. Status bar (full width)
	hint := m.statusHint()
	ctxUsed, ctxMax := m.contextBudget()
	teamSummary, unreadMail := m.teamStatus()
	displayModel := m.model
	if m.currentAgent != "" {
		displayModel = m.currentAgent
	}
	sections = append(sections, renderStatusBar(m.width, StatusBarState{
		Model:       displayModel,
		Tokens:      m.totalTokens,
		Cost:        m.totalCost,
		Turns:       m.turns,
		Streaming:   m.streaming,
		SpinText:    m.spinText,
		Hint:        hint,
		VimMode:     m.vimModeDisplay(),
		SessionName: m.sessionName(),
		PanelName:   m.panelName(),
		ContextUsed:        ctxUsed,
		ContextMax:         ctxMax,
		BackgroundSessions: m.countBackgroundSessions(),
		TeamSummary:        teamSummary,
		UnreadMailbox:      unreadMail,
		RateLimitWarning:   m.rateLimitWarning,
		RateLimitError:     m.rateLimitError,
		IsUsingOverage:     m.isUsingOverage,
	}))

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// teamStatus returns the team summary string and unread mailbox count.
func (m Model) teamStatus() (string, int) {
	if m.appCtx == nil || m.appCtx.TeamRunner == nil {
		return "", 0
	}
	teamName := m.appCtx.TeamRunner.ActiveTeamName()
	if teamName == "" {
		return "", 0
	}
	working := m.appCtx.TeamRunner.WorkingCount()
	states := m.appCtx.TeamRunner.AllStates()
	total := len(states)
	summary := fmt.Sprintf("team:%s %d/%d ◐", teamName, working, total)

	unread := 0
	if mb := m.appCtx.TeamRunner.GetMailbox(); mb != nil {
		unread = mb.UnreadCount("team-lead")
	}
	return summary, unread
}

// PermissionMode constants for display
var permissionModes = []string{"default", "auto", "plan"}

func (m *Model) cyclePermissionMode() {
	if m.engineConfig == nil {
		m.engineConfig = &query.EngineConfig{PermissionMode: "default"}
	}
	current := m.engineConfig.PermissionMode
	if current == "" {
		current = "default"
	}
	// Find current index and advance
	for i, mode := range permissionModes {
		if mode == current {
			next := permissionModes[(i+1)%len(permissionModes)]
			m.engineConfig.PermissionMode = next
			return
		}
	}
	m.engineConfig.PermissionMode = "default"
}

// renderHelpFooter renders the persistent help footer at the bottom of the screen.
func (m Model) renderHelpFooter() string {
	footerText := "[space] commands · [/] search · [q] quit"
	footerStyle := lipgloss.NewStyle().Foreground(styles.Dim)
	footer := footerStyle.Render(footerText)
	// Pad the footer to full width
	footer = lipgloss.NewStyle().Width(m.width).Render(footer)
	return footer
}

// renderModeLine renders the vim-style mode line below the prompt.
func (m Model) renderModeLine() string {
	vimMode := m.vimModeDisplay()

	var left string
	if vimMode != "" {
		left = styles.ModeLineStyle.Render("-- " + vimMode + " --")
	}

	// Mode indicator: plan mode takes precedence over permission mode label.
	permMode := "default"
	if m.engineConfig != nil && m.engineConfig.PermissionMode != "" {
		permMode = m.engineConfig.PermissionMode
	}
	var modeIndicator string
	if m.planModeActive {
		modeIndicator = styles.ModeLineArrowStyle.Render(" ▸▸ ") + styles.SearchPlanStyle.Render("plan mode")
	} else {
		modeIndicator = styles.ModeLineArrowStyle.Render(" ▸▸ ") + styles.ModeLineHintStyle.Render(permMode+" mode")
	}

	// Show queue count
	if len(m.messageQueue) > 0 {
		modeIndicator += styles.SearchQueueStyle.Render(fmt.Sprintf("  [%d queued]", len(m.messageQueue)))
	}

	right := styles.ModeLineHintStyle.Render("(shift+tab to cycle)")

	content := left + modeIndicator
	gap := m.width - lipgloss.Width(content) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}

	return " " + content + strings.Repeat(" ", gap) + right + " "
}

// renderSearchBar renders the search input line.
func (m Model) renderSearchBar() string {
	left := styles.SearchHeaderStyle.Render("/") + styles.SearchQueryStyle.Render(m.vpSearchQuery) + styles.SearchHeaderStyle.Render("▌")

	var right string
	if len(m.vpSearchMatches) > 0 {
		right = styles.ModeLineHintStyle.Render(fmt.Sprintf("[%d/%d]", m.vpSearchIdx+1, len(m.vpSearchMatches)))
	} else if m.vpSearchQuery != "" {
		right = styles.ModeLineHintStyle.Render("[no matches]")
	}

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}
	return " " + left + strings.Repeat(" ", gap) + right + " "
}

// renderStatusLine renders a 1-line nvim-style statusline above the prompt.
// When a toast is active, the toast text replaces the statusline content.
func (m Model) renderStatusLine() string {
	// Toast takes priority: show toast text in this row.
	if m.toast.IsActive() {
		toastStyle := lipgloss.NewStyle().
			Foreground(styles.Text).
			Background(styles.SurfaceAlt).
			Width(m.width)
		return toastStyle.Render(" " + m.toast.text)
	}

	// Mode pill: label + color based on focus state.
	var modeLabel string
	var pillStyle lipgloss.Style
	switch m.focus {
	case FocusViewport:
		modeLabel = "VIEWPORT"
		pillStyle = styles.StatusLinePillViewport
	case FocusPanel:
		modeLabel = "PANEL"
		pillStyle = styles.StatusLinePillPanel
	default:
		modeLabel = "PROMPT"
		pillStyle = styles.StatusLinePillPrompt
	}
	pill := pillStyle.Render("● " + modeLabel)

	// Center: session name + optional agent name.
	sessName := m.sessionName()
	if sessName == "" {
		sessName = "new session"
	}
	center := sessName
	if m.currentAgent != "" {
		center += " · " + m.currentAgent
	}
	centerStyled := styles.StatusLineCenterStyle.Render(center)

	// Right: model short name + spinner text.
	modelShort := strings.TrimPrefix(m.model, "claude-")
	right := modelShort
	if m.spinText != "" {
		right += "  " + m.spinText
	}
	rightStyled := styles.StatusLineRightStyle.Render(right)

	// Separator glyphs.
	sep := styles.StatusLineSeparator.Render(" │ ")

	left := " " + pill + sep + centerStyled
	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(rightStyled)
	sepW := lipgloss.Width(sep)

	gap := m.width - leftW - rightW - sepW - 1
	if gap < 1 {
		gap = 1
	}

	line := left + strings.Repeat(" ", gap) + sep + rightStyled + " "

	return lipgloss.NewStyle().
		Background(styles.SurfaceAlt).
		Width(m.width).
		Render(line)
}

// statusHint returns contextual keyboard hints for the status bar.
func (m Model) vimModeDisplay() string {
	if m.focus == FocusViewport {
		return "VIEWPORT"
	}
	return m.prompt.VimModeString()
}

func (m Model) sessionName() string {
	if m.session == nil {
		return ""
	}
	cur := m.session.Current()
	if cur == nil {
		return ""
	}
	if cur.Title != "" {
		return cur.Title
	}
	return cur.ID[:8]
}

// parseNewMemory parses a memory file content (with frontmatter) into an Entry.
// Returns nil if the user didn't fill in the name (template not edited).
func parseNewMemory(content string) *memory.Entry {
	entry := memory.ParseEntry(content, "")
	if entry == nil || entry.Name == "" || entry.Name == "new-memory" {
		return nil
	}
	if entry.Type == "" {
		entry.Type = "project"
	}
	return entry
}

// buildPinnedEngineIndices maps pinned ChatMessage indices to engine message indices.
// Engine messages and TUI messages don't have a 1:1 mapping (system/error msgs are TUI-only),
// so we track which user/assistant/tool messages are pinned by counting only persistable messages.
func (m *Model) buildPinnedEngineIndices() map[int]bool {
	pinned := make(map[int]bool)
	engineIdx := 0
	for _, msg := range m.messages {
		// Only user, assistant, tool_use, tool_result map to engine messages
		switch msg.Type {
		case MsgUser, MsgAssistant, MsgToolUse, MsgToolResult:
			if msg.Pinned {
				pinned[engineIdx] = true
			}
			engineIdx++
		}
	}
	return pinned
}

// updateSearchMatches recalculates which sections match the search query.
// Note: uses pointer-style mutation but is called from the value-receiver Update
// method where m is a local copy that gets returned.
func (m *Model) updateSearchMatches() {
	m.vpSearchMatches = nil
	m.vpSearchIdx = 0
	if m.vpSearchQuery == "" {
		return
	}
	query := strings.ToLower(m.vpSearchQuery)
	for i, msg := range m.messages {
		content := strings.ToLower(msg.Content + " " + msg.ToolName + " " + msg.ToolInput)
		if strings.Contains(content, query) {
			for si, sec := range m.vpSections {
				if sec.MsgIndex == i {
					m.vpSearchMatches = append(m.vpSearchMatches, si)
					break
				}
			}
		}
	}
}

// contextBudget returns (used, max) token counts from the compact state.
func (m Model) contextBudget() (int, int) {
	if m.engineConfig != nil && m.engineConfig.CompactState != nil {
		cs := m.engineConfig.CompactState
		return cs.TotalTokens, cs.MaxTokens
	}
	return 0, 0
}

func (m Model) panelName() string {
	switch m.activePanelID {
	case PanelSessions:
		return "sessions"
	case PanelConfig:
		return "config"
	case PanelSkills:
		return "skills"
	case PanelMemory:
		return "memory"
	case PanelAnalytics:
		return "analytics"
	case PanelTasks:
		return "tasks"
	case PanelTools:
		return "tools"
	default:
		return ""
	}
}

func (m Model) statusHint() string {
	if m.focus == FocusAgentDetail {
		return "j/k scroll · m message · esc back"
	}
	if m.focus == FocusPlanApproval {
		return "j/k navigate · enter confirm · ctrl+g edit plan · esc dismiss"
	}
	if m.focus == FocusPermission {
		return "y allow · n deny"
	}
	if m.focus == FocusAskUser {
		return "j/k navigate · enter select · esc cancel"
	}
	if m.focus == FocusPanel {
		return "j/k navigate · enter select · esc close"
	}
	if m.focus == FocusViewport {
		if m.vpSearchActive {
			return "type to search · enter confirm · esc cancel"
		}
		if len(m.vpSearchMatches) > 0 {
			return "j/k scroll · n/N next/prev match · / search · p pin · d delete · i/q back"
		}
		return "j/k scroll · / search · p pin · d delete · ctrl+o expand · i/q back"
	}
	if m.focus == FocusModelSelector {
		return "\u2191\u2193 select · enter confirm · esc cancel"
	}
	if m.focus == FocusAgentSelector {
		return "j/k navigate \u00B7 enter select \u00B7 esc cancel"
	}
	if m.palette.IsActive() {
		return "\u2191\u2193 navigate · tab complete · enter select · esc close"
	}
	if m.filePicker.IsActive() {
		return "\u2191\u2193 navigate · enter select · esc close"
	}
	if m.streaming {
		return "ctrl+c cancel · <Space>wk viewport"
	}
	if m.lastToolGroup >= 0 {
		return "<Space>. sessions · <Space>wk viewport · ctrl+o expand"
	}
	return "<Space>. sessions · <Space>bc new · <Space>wk viewport"
}

// placeOverlay renders an overlay string on top of a base string,
// centered horizontally and anchored to the bottom.
func placeOverlay(base, overlay string, width, height int) string {
	return lipgloss.Place(
		width, height,
		lipgloss.Center, lipgloss.Center,
		overlay,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(lipgloss.Color("#000000")),
	)
}

// ── Event Handler ────────────────────────────────────────

type tuiEventHandler struct {
	ch         chan<- tuiEvent
	approvalCh chan bool
}

func (h *tuiEventHandler) OnTextDelta(text string) {
	h.ch <- tuiEvent{typ: "text_delta", text: text}
}

func (h *tuiEventHandler) OnThinkingDelta(text string) {
	h.ch <- tuiEvent{typ: "thinking_delta", text: text}
}

func (h *tuiEventHandler) OnToolUseStart(tu tools.ToolUse) {
	h.ch <- tuiEvent{typ: "tool_start", toolUse: tu}
}

func (h *tuiEventHandler) OnToolUseEnd(tu tools.ToolUse, result *tools.Result) {
	h.ch <- tuiEvent{typ: "tool_end", toolUse: tu, result: result}
}

func (h *tuiEventHandler) OnTurnComplete(usage api.Usage) {
	h.ch <- tuiEvent{typ: "turn_complete", usage: usage}
}

func (h *tuiEventHandler) OnToolApprovalNeeded(tu tools.ToolUse) bool {
	h.ch <- tuiEvent{typ: "approval_needed", toolUse: tu}
	return <-h.approvalCh
}

func (h *tuiEventHandler) OnCostConfirmNeeded(currentCost, threshold float64) bool {
	// Auto-continue in TUI mode; budget limits are enforced via --budget flag
	return true
}

func (h *tuiEventHandler) OnError(err error) {
	h.ch <- tuiEvent{typ: "error", err: err}
}

func (h *tuiEventHandler) OnRetry(toolUses []tools.ToolUse) {
	h.ch <- tuiEvent{typ: "retry", toolUses: toolUses}
}

// tuiSubAgentObserver forwards sub-agent tool events to the TUI event channel.
type tuiSubAgentObserver struct {
	ch chan<- tuiEvent
}

func (o *tuiSubAgentObserver) OnSubAgentToolStart(desc string, tu tools.ToolUse) {
	o.ch <- tuiEvent{typ: "subagent_tool_start", text: desc, toolUse: tu}
}

func (o *tuiSubAgentObserver) OnSubAgentToolEnd(desc string, tu tools.ToolUse, result *tools.Result) {
	o.ch <- tuiEvent{typ: "subagent_tool_end", text: desc, toolUse: tu, result: result}
}

// tuiTeammateEventHandler forwards teammate events to the TUI event channel.
type tuiTeammateEventHandler struct {
	ch chan<- tuiEvent
}

func (h *tuiTeammateEventHandler) OnTeammateEvent(event teams.TeammateEvent) {
	e := event
	h.ch <- tuiEvent{typ: "teammate_event", teammateEvent: &e}
}

func (o *tuiSubAgentObserver) OnSubAgentText(_ string, _ string) {
	// Text events are not forwarded to the main TUI chat — they're captured
	// per-teammate by the teammateObserver for the agent detail view.
}

func shortModelName(model string) string {
	switch {
	case strings.Contains(model, "opus"):
		return "opus"
	case strings.Contains(model, "haiku"):
		return "haiku"
	case strings.Contains(model, "sonnet"):
		return "sonnet"
	default:
		return model
	}
}

func shortenDir(dir string) string {
	home, _ := os.UserHomeDir()
	if home != "" && strings.HasPrefix(dir, home) {
		dir = "~" + dir[len(home):]
	}
	return dir
}

func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 48*time.Hour:
		return "yesterday"
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

// formatBytes formats a byte count as a human-readable string (B, KB, MB, GB).
func formatBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// formatInt formats an int64 with comma separators.
func formatInt(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var result []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
}

func formatToolSummary(tu tools.ToolUse) string {
	switch tu.Name {
	case "Bash":
		var in struct{ Command string }
		json.Unmarshal(tu.Input, &in)
		return "$ " + in.Command
	case "Read":
		var in struct{ FilePath string }
		json.Unmarshal(tu.Input, &in)
		return in.FilePath
	case "Write":
		var in struct{ FilePath string }
		json.Unmarshal(tu.Input, &in)
		return in.FilePath
	case "Edit":
		var in struct{ FilePath string }
		json.Unmarshal(tu.Input, &in)
		return in.FilePath
	case "Glob":
		var in struct{ Pattern string }
		json.Unmarshal(tu.Input, &in)
		return in.Pattern
	case "Grep":
		var in struct{ Pattern string }
		json.Unmarshal(tu.Input, &in)
		return in.Pattern
	default:
		s := string(tu.Input)
		if len(s) > 100 {
			s = s[:100] + "..."
		}
		return s
	}
}
