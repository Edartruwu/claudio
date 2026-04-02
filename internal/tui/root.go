package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/cli/commands"
	"github.com/Abraxas-365/claudio/internal/config"
	"github.com/Abraxas-365/claudio/internal/query"
	"github.com/Abraxas-365/claudio/internal/services/compact"
	"github.com/Abraxas-365/claudio/internal/services/memory"
	"github.com/Abraxas-365/claudio/internal/services/skills"
	"github.com/Abraxas-365/claudio/internal/session"
	"github.com/Abraxas-365/claudio/internal/storage"
	"github.com/Abraxas-365/claudio/internal/tools"
	"github.com/Abraxas-365/claudio/internal/tui/commandpalette"
	"github.com/Abraxas-365/claudio/internal/tui/components"
	"github.com/Abraxas-365/claudio/internal/tui/filepicker"
	"github.com/Abraxas-365/claudio/internal/tui/modelselector"
	"github.com/Abraxas-365/claudio/internal/tui/panels"
	panelsessions "github.com/Abraxas-365/claudio/internal/tui/panels/sessions"
	"github.com/Abraxas-365/claudio/internal/tui/panels/analyticspanel"
	panelconfig "github.com/Abraxas-365/claudio/internal/tui/panels/config"
	"github.com/Abraxas-365/claudio/internal/tui/panels/memorypanel"
	"github.com/Abraxas-365/claudio/internal/tui/panels/skillspanel"
	"github.com/Abraxas-365/claudio/internal/tui/panels/taskspanel"
	"github.com/Abraxas-365/claudio/internal/tui/panels/whichkey"
	"github.com/Abraxas-365/claudio/internal/tui/permissions"
	"github.com/Abraxas-365/claudio/internal/tui/prompt"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

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
	whichKey       whichkey.Model
	sessionPicker  *panelsessions.Panel
	toast          Toast

	// Panels
	activePanel   panels.Panel
	activePanelID PanelID

	// State
	messages       []ChatMessage
	focus          Focus
	width, height  int
	streaming      bool
	streamText     *strings.Builder
	model          string
	totalTokens    int
	totalCost      float64
	turns          int
	spinText       string // current spinner status text
	expandedGroups  map[int]bool // tool group msg indices that are expanded
	lastToolGroup   int          // msg index of the last tool group start (-1 = none)
	leaderSeq       string       // leader key sequence in progress ("", "pending", "w", "b", "i", ",")
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

	// App context for panels
	appCtx *AppContext

	// Engine integration
	engine                *query.Engine
	pendingEngineMessages []api.Message
	apiClient             *api.Client
	registry     *tools.Registry
	cancelFunc   context.CancelFunc
	eventCh      chan tuiEvent
	approvalCh   chan bool
	systemPrompt string
	commands     *commands.Registry
	session      *session.Session
	skills       *skills.Registry
	engineConfig *query.EngineConfig
}

// tuiEvent wraps query engine events for the Bubble Tea message loop.
type tuiEvent struct {
	typ     string
	text    string
	toolUse tools.ToolUse
	result  *tools.Result
	usage   api.Usage
	err     error
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
	compactDoneMsg struct {
		compacted []api.Message
		summary   string
		err       error
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
		systemPrompt:   systemPrompt,
		streamText:     &strings.Builder{},
		session:        sess,
		expandedGroups:  make(map[int]bool),
		lastToolGroup:   -1,
		vpCursor:        -1,
		whichKey:        whichkey.New(),
		sessionRuntimes: make(map[string]*SessionRuntime),
	}

	// Apply options
	for _, opt := range opts {
		opt(&m)
	}

	cmdRegistry := commands.NewRegistry()
	commands.RegisterCoreCommands(cmdRegistry, &commands.CommandDeps{
		GetModel: func() string { return m.model },
		SetModel: func(model string) {
			m.model = model
			apiClient.SetModel(model)
		},
		GetThinkingLabel: func() string { return apiClient.ThinkingLabel() },
		Compact: func(keepLast int) (string, error) {
			if m.engine == nil {
				return "", fmt.Errorf("no active conversation")
			}
			msgs := m.engine.Messages()
			// Build pinned indices from ChatMessages
			pinned := m.buildPinnedEngineIndices()
			compacted, summary, err := compact.Compact(
				context.Background(), apiClient, msgs, keepLast, pinned,
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
	})
	m.commands = cmdRegistry

	// Build palette items from registered commands
	m.palette = commandpalette.New(buildPaletteItems(cmdRegistry))

	// File picker for @ mentions
	cwd, _ := os.Getwd()
	m.filePicker = filepicker.New(cwd)

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
		m.spinner.Tick(),
		m.waitForEvent(),
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
		m.palette.SetWidth(m.width)
		m.filePicker.SetWidth(m.width)
		m.modelSelector.SetWidth(mainWidth(m.width, m.activePanel))
		m.layout()
		m.refreshViewport()
		return m, nil

	case prompt.VimEscapeMsg:
		// Vim consumed Escape (Insert→Normal). Don't dismiss anything.
		return m, nil

	case ToastDismissMsg:
		m.toast.Dismiss()
		return m, nil

	case whichkey.TimeoutMsg:
		// Show which-key popup based on current leader sequence
		switch m.leaderSeq {
		case "pending":
			m.whichKey.ShowDefault()
			m.whichKey.SetWidth(m.width)
		case "w":
			m.whichKey.ShowWindow()
			m.whichKey.SetWidth(m.width)
		case "b":
			m.whichKey.Show(whichkey.SessionBindings())
			m.whichKey.SetWidth(m.width)
		case "i":
			m.whichKey.Show(whichkey.PanelBindings())
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
				m.addMessage(ChatMessage{Type: MsgSystem, Content: "Cancelled — partial response preserved"})
				m.refreshViewport()
				return m, nil
			}
			// Not streaming: quit
			if m.cancelFunc != nil {
				m.cancelFunc()
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
		case "ctrl+g":
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
			// In Insert mode, Escape always goes to Normal (standard vim)
			if m.prompt.IsVimEnabled() && !m.prompt.IsVimNormal() {
				break // fall through to prompt.Update below
			}
			// In Normal mode during streaming: do nothing (use Ctrl+C to cancel)
			// This allows navigating (Space+wk, etc.) without killing the stream
			if m.streaming {
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

		// Panel focus mode: delegate all keys to active panel
		if m.focus == FocusPanel && m.activePanel != nil {
			cmd, consumed := m.activePanel.Update(msg)
			if !consumed {
				// Panel didn't consume — check for close keys
				switch msg.String() {
				case "esc", "q":
					m.closePanel()
					return m, nil
				}
			}
			// Check if panel closed itself
			if !m.activePanel.IsActive() {
				m.closePanel()
			}
			return m, cmd
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
			case "/":
				// Enter search mode
				m.vpSearchActive = true
				m.vpSearchQuery = ""
				m.vpSearchMatches = nil
				m.vpSearchIdx = 0
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
				return m, whichkey.ScheduleTimeout()
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
				return m, whichkey.ScheduleTimeout()
			}
		}

		// Welcome screen number keys: 1-3 to resume recent sessions
		if m.focus == FocusPrompt && m.isWelcomeScreen() {
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

		// Viewport scrolling shortcuts: vim Normal mode + empty prompt
		if m.focus == FocusPrompt && m.prompt.IsVimNormal() && m.prompt.Value() == "" {
			switch msg.String() {
			case "j":
				m.viewport.ScrollDown(1)
				return m, nil
			case "k":
				m.viewport.ScrollUp(1)
				return m, nil
			case "ctrl+d":
				m.viewport.HalfPageDown()
				return m, nil
			case "ctrl+u":
				m.viewport.HalfPageUp()
				return m, nil
			case "G":
				m.viewport.GotoBottom()
				return m, nil
			case "g":
				m.viewport.GotoTop()
				return m, nil
			}
		}

		// Model selector gets priority when active
		if m.focus == FocusModelSelector {
			var cmd tea.Cmd
			m.modelSelector, cmd = m.modelSelector.Update(msg)
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
		// Regular file: insert path as text
		val := m.prompt.Value()
		atIdx := strings.LastIndex(val, "@")
		if atIdx >= 0 {
			m.prompt.SetValue(val[:atIdx] + msg.Path + " ")
		}
		return m, nil

	case modelselector.ModelSelectedMsg:
		m.focus = FocusPrompt
		m.prompt.Focus()
		m.model = msg.Label
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

	case timerTickMsg:
		if m.streaming {
			m.refreshViewport()
			return m, tea.Tick(time.Second, func(time.Time) tea.Msg { return timerTickMsg{} })
		}
		return m, nil

	case engineEventMsg:
		return m.handleEngineEvent(tuiEvent(msg))

	case engineDoneMsg:
		m.streaming = false
		m.spinText = ""
		m.spinner.Stop()
		m.focus = FocusPrompt
		m.prompt.Focus()
		if msg.err != nil {
			m.addMessage(ChatMessage{Type: MsgError, Content: msg.err.Error()})
		}
		m.refreshViewport()
		// Process queued messages
		if len(m.messageQueue) > 0 {
			next := m.messageQueue[0]
			m.messageQueue = m.messageQueue[1:]
			return m.handleSubmit(next)
		}
		return m, nil

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
			m.addMessage(ChatMessage{Type: MsgSystem, Content: fmt.Sprintf("Compacted. Summary:\n%s", msg.summary)})
		}
		m.refreshViewport()
		return m, nil
	}

	// Delegate to focused component
	switch m.focus {
	case FocusPrompt:
		var cmd tea.Cmd
		m.prompt, cmd = m.prompt.Update(msg)
		cmds = append(cmds, cmd)
		m.updatePaletteState()
	}

	// Update spinner
	var spinCmd tea.Cmd
	m.spinner, spinCmd = m.spinner.Update(msg)
	cmds = append(cmds, spinCmd)

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

// ── Handlers ─────────────────────────────────────────────

func (m Model) handleSubmit(text string) (tea.Model, tea.Cmd) {
	m.palette.Deactivate()
	m.filePicker.Deactivate()

	if cmdName, cmdArgs, isCmd := commands.Parse(text); isCmd {
		return m.handleCommand(cmdName, cmdArgs)
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
		// Auto-title from first message
		title := cleanedText
		if len(title) > 50 {
			// Truncate at word boundary
			if idx := strings.LastIndex(title[:50], " "); idx > 10 {
				title = title[:idx] + "..."
			} else {
				title = title[:47] + "..."
			}
		}
		m.session.SetTitle(title)
	}

	m.addMessage(ChatMessage{Type: MsgUser, Content: displayText})
	m.refreshViewport()

	m.streaming = true
	m.streamText.Reset()
	m.spinText = "Thinking..."
	m.spinner.Start("Thinking...")

	m.approvalCh = make(chan bool, 1)
	handler := &tuiEventHandler{ch: m.eventCh, approvalCh: m.approvalCh}
	if m.engineConfig != nil {
		m.engine = query.NewEngineWithConfig(m.apiClient, m.registry, handler, *m.engineConfig)
	} else {
		m.engine = query.NewEngine(m.apiClient, m.registry, handler)
	}
	if m.systemPrompt != "" {
		m.engine.SetSystem(m.systemPrompt)
	}
	if len(m.pendingEngineMessages) > 0 {
		m.engine.SetMessages(m.pendingEngineMessages)
		m.pendingEngineMessages = nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.cancelFunc = cancel

	// Inject sub-agent observer so agent tool events flow to TUI in real time
	ctx = tools.WithSubAgentObserver(ctx, &tuiSubAgentObserver{ch: m.eventCh})

	// Build content blocks: images + file contents + user text
	hasAttachments := len(imageBlocks) > 0 || len(fileAttachments) > 0

	go func() {
		var err error
		if hasAttachments {
			blocks := BuildContentBlocks(cleanedText, fileAttachments, imageBlocks)
			err = m.engine.RunWithBlocks(ctx, blocks)
		} else {
			err = m.engine.Run(ctx, cleanedText)
		}
		m.eventCh <- tuiEvent{typ: "done", err: err}
	}()

	timerTick := tea.Tick(time.Second, func(time.Time) tea.Msg { return timerTickMsg{} })
	return m, tea.Batch(m.spinner.Tick(), m.waitForEvent(), timerTick)
}

func (m Model) handleEngineEvent(event tuiEvent) (tea.Model, tea.Cmd) {
	switch event.typ {
	case "text_delta":
		m.streamText.WriteString(event.text)
		m.spinText = "Responding..."
		m.spinner.SetText("Responding...")
		m.updateStreamingMessage()
		m.refreshViewport()

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

	case "tool_start":
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

		m.addMessage(ChatMessage{
			Type:         MsgToolUse,
			ToolName:     event.toolUse.Name,
			ToolInput:    formatToolSummary(event.toolUse),
			ToolInputRaw: event.toolUse.Input,
		})
		m.refreshViewport()

	case "tool_end":
		// Update the tool_use message with the full input (now that streaming is done)
		if event.toolUse.Input != nil {
			for i := len(m.messages) - 1; i >= 0; i-- {
				if m.messages[i].Type == MsgToolUse && m.messages[i].ToolName == event.toolUse.Name && m.messages[i].ToolInputRaw == nil {
					m.messages[i].ToolInputRaw = event.toolUse.Input
					m.messages[i].ToolInput = formatToolSummary(event.toolUse)
					break
				}
			}
		}
		if event.result != nil {
			m.addMessage(ChatMessage{
				Type:    MsgToolResult,
				Content: event.result.Content,
				IsError: event.result.IsError,
			})
			m.refreshViewport()
		}

	case "turn_complete":
		m.finalizeStreamingMessage()
		m.totalTokens += event.usage.OutputTokens
		m.totalCost += float64(event.usage.InputTokens) * 3.0 / 1_000_000
		m.totalCost += float64(event.usage.OutputTokens) * 15.0 / 1_000_000
		m.turns++

	case "done":
		m.finalizeStreamingMessage()
		m.streaming = false
		m.spinText = ""
		m.spinner.Stop()
		m.focus = FocusPrompt
		m.prompt.Focus()
		if event.err != nil && event.err.Error() != "context canceled" {
			m.addMessage(ChatMessage{Type: MsgError, Content: event.err.Error()})
		}
		m.refreshViewport()

		// Process queued messages
		if len(m.messageQueue) > 0 {
			next := m.messageQueue[0]
			m.messageQueue = m.messageQueue[1:]
			return m.handleSubmit(next)
		}
		return m, nil

	case "subagent_tool_start":
		// Update spinner to show sub-agent activity
		summary := formatToolSummary(event.toolUse)
		label := fmt.Sprintf("Agent → %s %s", event.toolUse.Name, summary)
		m.spinText = label
		m.spinner.SetText(label)

		// Append as a child of the most recent Agent MsgToolUse
		for i := len(m.messages) - 1; i >= 0; i-- {
			if m.messages[i].Type == MsgToolUse && m.messages[i].ToolName == "Agent" {
				m.messages[i].SubagentTools = append(m.messages[i].SubagentTools, SubagentToolCall{
					ToolName: event.toolUse.Name,
					Summary:  summary,
				})
				break
			}
		}
		m.refreshViewport()

	case "subagent_tool_end":
		// Find the most recent Agent MsgToolUse and update the last matching sub-agent tool
		if event.result != nil {
			brief := resultBrief(event.result.Content)
			summary := formatToolSummary(event.toolUse)
			for i := len(m.messages) - 1; i >= 0; i-- {
				if m.messages[i].Type == MsgToolUse && m.messages[i].ToolName == "Agent" {
					subs := m.messages[i].SubagentTools
					for j := len(subs) - 1; j >= 0; j-- {
						if subs[j].ToolName == event.toolUse.Name && subs[j].Result == nil {
							m.messages[i].SubagentTools[j].Result = &brief
							m.messages[i].SubagentTools[j].IsError = event.result.IsError
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
		m.addMessage(ChatMessage{Type: MsgError, Content: event.err.Error()})
		m.refreshViewport()
	}

	return m, tea.Batch(m.spinner.Tick(), m.waitForEvent())
}

func (m Model) handleCommand(name, args string) (tea.Model, tea.Cmd) {
	// /model without args → interactive selector
	if name == "model" && args == "" {
		m.modelSelector = modelselector.New(m.apiClient.GetModel(), m.apiClient.GetThinkingMode(), m.apiClient.GetBudgetTokens(), m.apiClient.GetEffortLevel())
		m.modelSelector.SetWidth(m.width)
		m.focus = FocusModelSelector
		m.prompt.Blur()
		return m, nil
	}

	// /compact → handle directly because closures from New() capture a stale m
	if name == "compact" {
		keepLast := 10
		if args != "" {
			if _, err := fmt.Sscanf(args, "%d", &keepLast); err != nil {
				m.addMessage(ChatMessage{Type: MsgSystem, Content: "Usage: /compact [number-of-messages-to-keep]"})
				m.refreshViewport()
				return m, nil
			}
		}
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
				context.Background(), m.apiClient, msgs, keepLast, pinned,
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

	cmd, ok := m.commands.Get(name)
	if !ok {
		m.addMessage(ChatMessage{Type: MsgError, Content: fmt.Sprintf("Unknown command: /%s. Type /help for available commands.", name)})
		m.refreshViewport()
		return m, nil
	}

	output, err := cmd.Execute(args)
	if err != nil {
		if err.Error() == "__exit__" {
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
		// Skill invocation: intercept [skill:NAME] and send skill content to engine
		if strings.HasPrefix(output, "[skill:") && strings.Contains(output, "]") {
			skillName := output[7:strings.Index(output, "]")]
			if m.skills != nil {
				if skill, ok := m.skills.Get(skillName); ok {
					// Send skill content as a user message to the engine
					m.addMessage(ChatMessage{Type: MsgSystem, Content: fmt.Sprintf("Running skill: %s", skill.Name)})
					m.refreshViewport()
					return m.handleSubmit(skill.Content)
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

func (m Model) handlePermissionResponse(resp permissions.ResponseMsg) (tea.Model, tea.Cmd) {
	m.focus = FocusPrompt

	switch resp.Decision {
	case permissions.DecisionAllow:
		m.approvalCh <- true
	case permissions.DecisionAllowAlways:
		m.approvalCh <- true
		// Persist as a permission rule
		rule := buildPermissionRule(resp.ToolUse)
		m.persistPermissionRule(rule)
		m.addMessage(ChatMessage{Type: MsgSystem, Content: fmt.Sprintf("Saved rule: allow %s(%s)", rule.Tool, rule.Pattern)})
	case permissions.DecisionAllowAllTool:
		m.approvalCh <- true
		// Persist a wildcard rule for this tool
		rule := config.PermissionRule{
			Tool:     resp.ToolUse.Name,
			Pattern:  "*",
			Behavior: "allow",
		}
		m.persistPermissionRule(rule)
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
func (m *Model) persistPermissionRule(rule config.PermissionRule) {
	if m.appCtx == nil || m.appCtx.Config == nil {
		return
	}

	// Add to live config
	m.appCtx.Config.PermissionRules = append(m.appCtx.Config.PermissionRules, rule)

	// Also add to engine config so it takes effect immediately
	if m.engineConfig != nil {
		m.engineConfig.PermissionRules = append(m.engineConfig.PermissionRules, rule)
	}

	// Save to project config if available, else global
	cwd, _ := os.Getwd()
	projectRoot := config.FindGitRoot(cwd)
	projectSettings := filepath.Join(projectRoot, ".claudio", "settings.json")

	savePath := config.GetPaths().Settings // default: global
	if _, err := os.Stat(filepath.Join(projectRoot, ".claudio")); err == nil {
		savePath = projectSettings // prefer project if .claudio/ exists
	}

	// Load existing, append rule, save back
	data, _ := os.ReadFile(savePath)
	var existing map[string]json.RawMessage
	if json.Unmarshal(data, &existing) != nil {
		existing = make(map[string]json.RawMessage)
	}

	// Parse existing rules
	var rules []config.PermissionRule
	if raw, ok := existing["permissionRules"]; ok {
		json.Unmarshal(raw, &rules)
	}
	rules = append(rules, rule)
	rulesJSON, _ := json.Marshal(rules)
	existing["permissionRules"] = rulesJSON

	out, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(savePath, out, 0644)
}

// ── Leader Key State Machine ────────────────────────────

// handleLeaderKey processes the next key in a leader sequence.
// Returns (handled bool, cmd). If handled is false, the key was not consumed.
func (m *Model) handleLeaderKey(key string) (bool, tea.Cmd) {
	seq := m.leaderSeq
	m.leaderSeq = "" // reset by default

	switch seq {
	case "pending":
		// First key after Space
		switch key {
		case "w":
			m.leaderSeq = "w"
			return true, whichkey.ScheduleTimeout()
		case "b":
			m.leaderSeq = "b"
			return true, whichkey.ScheduleTimeout()
		case "i":
			m.leaderSeq = "i"
			return true, whichkey.ScheduleTimeout()
		case ",":
			m.leaderSeq = ","
			return true, nil
		case ".":
			// Session picker (like <leader>. for telescope buffers)
			return m.openSessionPicker()
		case ";":
			// Recent sessions (same as . for now)
			return m.openSessionPicker()
		case "/":
			// Search sessions (open picker with search focused)
			return m.openSessionPicker()
		}
		return true, nil // consumed but no match

	case "w":
		// Window sub-menu
		switch key {
		case "k":
			m.focus = FocusViewport
			m.prompt.Blur()
			if len(m.vpSections) > 0 {
				m.vpCursor = len(m.vpSections) - 1
			} else {
				m.vpCursor = 0
			}
			m.refreshViewport()
			m.scrollToSection(m.vpCursor)
			return true, nil
		case "j", "p":
			m.focus = FocusPrompt
			m.vpCursor = -1
			m.prompt.Focus()
			m.refreshViewport()
			return true, nil
		}
		return true, nil

	case "b":
		// Buffer/session sub-menu
		switch key {
		case "n":
			return m.switchSessionRelative(1)
		case "p":
			return m.switchSessionRelative(-1)
		case "k":
			return m.deleteCurrentSession()
		case "r":
			return m.renameCurrentSession()
		case "c":
			return m.createNewSession()
		}
		return true, nil

	case "i":
		// Info panels sub-menu
		return m.handlePanelToggleByKey(key)

	case ",":
		// Alternate session: <Space>,<CR>
		if key == "enter" {
			return m.switchToAlternateSession()
		}
		return true, nil
	}

	return false, nil
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
				Type:     msgType,
				Content:  msg.Content,
				ToolName: msg.ToolName,
			})
		}

		// Restore engine conversation history so the model has full context
		engineMsgs := reconstructEngineMessages(storedMsgs)
		if m.engine != nil {
			m.engine.SetMessages(engineMsgs)
		} else {
			m.pendingEngineMessages = engineMsgs
		}

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

	// Inject summary into system prompt for AI continuity
	if resumed.Summary != "" {
		m.systemPrompt += "\n\n# Previous Session Context\n" + resumed.Summary
	}
	m.refreshViewport()
}

// reconstructEngineMessages rebuilds a []api.Message slice from stored DB records so
// the engine has full conversation history when a session is resumed.
// Groups: "assistant" + following "tool_use" rows → one assistant message;
// consecutive "tool_result" rows → one user message, IDs paired by position.
func reconstructEngineMessages(storedMsgs []storage.MessageRecord) []api.Message {
	type trBlock struct {
		Type      string `json:"type"`
		ToolUseID string `json:"tool_use_id"`
		Content   string `json:"content"`
	}

	var result []api.Message
	var pendingIDs []string
	tuCounter := 0

	i := 0
	for i < len(storedMsgs) {
		msg := storedMsgs[i]
		switch msg.Type {
		case "user":
			content, _ := json.Marshal(msg.Content)
			result = append(result, api.Message{Role: "user", Content: content})
			pendingIDs = nil // tool_results cannot bridge past a plain user message
			i++

		case "assistant":
			var blocks []api.ContentBlock
			if msg.Content != "" {
				blocks = append(blocks, api.ContentBlock{Type: "text", Text: msg.Content})
			}
			i++
			pendingIDs = nil
			for i < len(storedMsgs) && storedMsgs[i].Type == "tool_use" {
				tuCounter++
				id := fmt.Sprintf("toolu_%04d", tuCounter)
				pendingIDs = append(pendingIDs, id)
				input := json.RawMessage(storedMsgs[i].Content)
				if len(input) == 0 || !json.Valid(input) {
					input = json.RawMessage("{}")
				}
				blocks = append(blocks, api.ContentBlock{
					Type:  "tool_use",
					ID:    id,
					Name:  storedMsgs[i].ToolName,
					Input: input,
				})
				i++
			}
			if len(blocks) > 0 {
				content, _ := json.Marshal(blocks)
				result = append(result, api.Message{Role: "assistant", Content: content})
			}

		case "tool_result":
			// Skip orphaned tool_results with no preceding tool_use — they would
			// cause a 400 from the API ("unexpected tool_use_id").
			if len(pendingIDs) == 0 {
				for i < len(storedMsgs) && storedMsgs[i].Type == "tool_result" {
					i++
				}
				continue
			}
			var trs []trBlock
			j := 0
			for i < len(storedMsgs) && storedMsgs[i].Type == "tool_result" {
				id := ""
				if j < len(pendingIDs) {
					id = pendingIDs[j]
				} else {
					tuCounter++
					id = fmt.Sprintf("toolu_%04d", tuCounter)
				}
				trs = append(trs, trBlock{
					Type:      "tool_result",
					ToolUseID: id,
					Content:   storedMsgs[i].Content,
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
			i++
		}
	}
	return sanitizeToolPairs(result)
}

// sanitizeToolPairs removes unmatched tool_use/tool_result pairs from a
// reconstructed message list. Two cases are handled:
//
//  1. An assistant message has tool_use blocks but the immediately following
//     message is not a user message whose content contains only tool_results
//     (orphaned tool_use — session ended before results were saved). The
//     tool_use blocks are stripped; any text content is kept.
//
//  2. A user message contains only tool_result blocks but the previous
//     message is not an assistant message with tool_use blocks (orphaned
//     tool_result). The entire user message is dropped.
func sanitizeToolPairs(msgs []api.Message) []api.Message {
	type contentBlock struct {
		Type string `json:"type"`
	}

	hasType := func(content json.RawMessage, typ string) bool {
		var blocks []json.RawMessage
		if json.Unmarshal(content, &blocks) != nil {
			return false
		}
		for _, b := range blocks {
			var block contentBlock
			if json.Unmarshal(b, &block) == nil && block.Type == typ {
				return true
			}
		}
		return false
	}

	stripToolUse := func(content json.RawMessage) json.RawMessage {
		var blocks []json.RawMessage
		if json.Unmarshal(content, &blocks) != nil {
			return content
		}
		var kept []json.RawMessage
		for _, b := range blocks {
			var block contentBlock
			if json.Unmarshal(b, &block) == nil && block.Type == "tool_use" {
				continue
			}
			kept = append(kept, b)
		}
		if len(kept) == 0 {
			return nil
		}
		out, _ := json.Marshal(kept)
		return out
	}

	result := make([]api.Message, 0, len(msgs))
	for i, msg := range msgs {
		if msg.Role == "assistant" && hasType(msg.Content, "tool_use") {
			// Valid only if immediately followed by a user message with tool_results
			if i+1 < len(msgs) && msgs[i+1].Role == "user" && hasType(msgs[i+1].Content, "tool_result") {
				result = append(result, msg)
			} else {
				// Strip tool_use blocks; keep any text content
				stripped := stripToolUse(msg.Content)
				if stripped != nil {
					result = append(result, api.Message{Role: "assistant", Content: stripped})
				}
			}
			continue
		}
		if msg.Role == "user" && hasType(msg.Content, "tool_result") {
			// Valid only if immediately preceded by an assistant message with tool_use
			if len(result) > 0 && result[len(result)-1].Role == "assistant" && hasType(result[len(result)-1].Content, "tool_use") {
				result = append(result, msg)
			}
			// Otherwise drop the orphaned tool_result message
			continue
		}
		result = append(result, msg)
	}
	return result
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
	rt.StreamText = *m.streamText
	rt.Streaming = m.streaming
	rt.TotalTokens = m.totalTokens
	rt.TotalCost = m.totalCost
	rt.Turns = m.turns
	rt.ExpandedGroups = m.expandedGroups
	rt.LastToolGroup = m.lastToolGroup
	rt.SpinText = m.spinText
	rt.MessageQueue = m.messageQueue

	m.sessionRuntimes[sessionID] = rt
	rt.StartBackgroundDrain()

	// Reset Model state for the new session
	m.engine = nil
	m.cancelFunc = nil
	m.eventCh = make(chan tuiEvent, 64)
	m.approvalCh = nil
	m.messages = nil
	m.streamText = &strings.Builder{}
	m.streaming = false
	m.totalTokens = 0
	m.totalCost = 0
	m.turns = 0
	m.expandedGroups = make(map[int]bool)
	m.lastToolGroup = -1
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
	m.streamText = &rt.StreamText
	m.streaming = rt.Streaming
	m.totalTokens = rt.TotalTokens
	m.totalCost = rt.TotalCost
	m.turns = rt.Turns
	m.expandedGroups = rt.ExpandedGroups
	m.lastToolGroup = rt.LastToolGroup
	m.spinText = rt.SpinText
	m.messageQueue = rt.MessageQueue

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
	}
	if _, err := m.session.Start(m.model); err != nil {
		m.addMessage(ChatMessage{Type: MsgError, Content: fmt.Sprintf("New session failed: %v", err)})
		m.refreshViewport()
		return true, nil
	}
	m.messages = nil
	m.streamText.Reset()
	m.turns = 0
	m.totalTokens = 0
	m.totalCost = 0
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
	// Create a new session first
	if _, err := m.session.Start(m.model); err != nil {
		return true, nil
	}
	// Delete the old one
	_ = m.session.Delete(cur.ID)
	m.messages = nil
	m.streamText.Reset()
	m.turns = 0
	m.totalTokens = 0
	m.totalCost = 0
	m.addMessage(ChatMessage{Type: MsgSystem, Content: fmt.Sprintf("Deleted session: %s", oldTitle)})
	m.refreshViewport()
	return true, nil
}


func (m *Model) renameCurrentSession() (bool, tea.Cmd) {
	m.addMessage(ChatMessage{Type: MsgSystem, Content: "Use /rename <title> to rename this session"})
	m.refreshViewport()
	return true, nil
}

func (m *Model) handlePanelToggleByKey(key string) (bool, tea.Cmd) {
	switch key {
	case "c":
		m.openPanel(PanelConfig)
	case "k":
		m.openPanel(PanelSkills)
	case "m":
		m.openPanel(PanelMemory)
	case "a":
		m.openPanel(PanelAnalytics)
	case "t":
		m.openPanel(PanelTasks)
	}
	return true, nil
}

// ── Panel Management ────────────────────────────────────

func (m *Model) openPanel(id PanelID) {
	panel := m.createPanel(id)
	if panel == nil {
		return
	}
	m.activePanel = panel
	m.activePanelID = id
	m.activePanel.Activate()
	m.viewport.Width = mainWidth(m.width, m.activePanel)
	m.focus = FocusPanel
	m.prompt.Blur()
	m.refreshViewport()
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
	}
	// Other settings (autoMemoryExtract, memorySelection, compactMode, etc.)
	// are read from config at the point of use, so saving to disk is sufficient.
}

func (m *Model) closePanel() {
	if m.activePanel != nil {
		m.activePanel.Deactivate()
	}
	m.activePanel = nil
	m.activePanelID = PanelNone
	m.focus = FocusPrompt
	m.prompt.Focus()
	m.viewport.Width = m.width
	m.refreshViewport()
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
	}
	return nil
}

// ── Message Management ───────────────────────────────────

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
	if msg.ToolName != "" {
		m.session.AddToolMessage(role, msg.Content, msgType, "", msg.ToolName)
	} else {
		m.session.AddMessage(role, msg.Content, msgType)
	}
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
		result := renderMessages(m.messages, m.viewport.Width, m.expandedGroups, cursorIdx)
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

	// DEBUG: log viewport state to file
	if len(m.messages) > 0 {
		contentLines := strings.Count(content, "\n") + 1
		debugInfo := fmt.Sprintf("msgs=%d contentLines=%d vpH=%d vpW=%d offset=%d\n---content---\n%s\n---end---\n",
			len(m.messages), contentLines, m.viewport.Height, m.viewport.Width, m.viewport.YOffset, content)
		os.WriteFile("/tmp/claudio-viewport-debug.txt", []byte(debugInfo), 0644)
	}

	m.viewport.SetContent(content)
	if m.focus != FocusViewport {
		contentLines := strings.Count(content, "\n") + 1
		if contentLines <= m.viewport.Height {
			m.viewport.GotoTop()
		} else {
			m.viewport.GotoBottom()
		}
	}
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

func (m *Model) welcomeScreen() string {
	title := lipgloss.NewStyle().
		Foreground(styles.Warning).
		Bold(true).
		Render("claudio")

	subtitle := lipgloss.NewStyle().
		Foreground(styles.Muted).
		Render("AI coding assistant")

	hints := lipgloss.NewStyle().
		Foreground(styles.Dim).
		Render("/help · @file · /vim · /model · <Space>. sessions")

	var parts []string
	parts = append(parts, "", title, subtitle, "", hints, "")

	// Recent sessions box — try project-scoped first, fall back to all
	if m.session != nil {
		recent, _ := m.session.RecentForProject(3)
		if len(recent) == 0 {
			// No sessions for this project — show any recent sessions
			recent, _ = m.session.Search("", 3)
		}
		if len(recent) > 0 {
			recentBox := m.renderRecentSessions(recent)
			parts = append(parts, recentBox, "")
		}
	}

	block := lipgloss.JoinVertical(lipgloss.Center, parts...)

	return lipgloss.Place(
		m.viewport.Width, m.viewport.Height,
		lipgloss.Center, lipgloss.Center,
		block,
	)
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

	numStyle := lipgloss.NewStyle().Foreground(styles.Warning).Bold(true)
	titleStyle := lipgloss.NewStyle().Foreground(styles.Text)
	dateStyle := lipgloss.NewStyle().Foreground(styles.Subtle)
	hintStyle := lipgloss.NewStyle().Foreground(styles.Dim)

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

		left := numStyle.Render(fmt.Sprintf("  %d  ", i+1)) + titleStyle.Render(stitle)
		right := dateStyle.Render(ago)
		gap := boxW - lipgloss.Width(left) - lipgloss.Width(right) - 2
		if gap < 1 {
			gap = 1
		}
		lines = append(lines, left+strings.Repeat(" ", gap)+right+"  ")
	}

	lines = append(lines, "")
	lines = append(lines, hintStyle.Render("  [1-3] resume · <Space>. browse · type to chat"))

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
		label := lipgloss.NewStyle().Foreground(styles.Dim).Render(" Recent ")
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

// ── Layout & View ────────────────────────────────────────

func (m *Model) layout() {
	statusH := 1
	promptH := m.prompt.Height()
	paletteH := 0
	if m.palette.IsActive() || m.filePicker.IsActive() {
		paletteH = 10
	}

	modeLineH := 1
	vpHeight := m.height - statusH - promptH - paletteH - modeLineH - 1
	if vpHeight < 5 {
		vpHeight = 5
	}

	mw := mainWidth(m.width, m.activePanel)
	m.viewport.Width = mw
	m.viewport.Height = vpHeight
	m.prompt.SetWidth(m.width) // prompt always full width
	m.permission.SetWidth(mw)
}

func (m Model) View() string {
	m.layout()

	mw := mainWidth(m.width, m.activePanel)
	hasPanel := m.activePanel != nil && m.activePanel.IsActive()

	// 1. Viewport (messages + inline spinner)
	vpView := m.viewport.View()

	// Overlay permission dialog or model selector on top of viewport
	if m.permission.IsActive() {
		overlay := m.permission.View()
		vpView = placeOverlay(vpView, overlay, mw, m.viewport.Height)
	}
	if m.modelSelector.IsActive() {
		overlay := m.modelSelector.View()
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
	if m.toast.IsActive() {
		overlay := m.toast.View()
		vpView = placeOverlay(vpView, overlay, mw, m.viewport.Height)
	}

	// 2. If panel is active, join viewport + panel side-by-side
	topArea := vpView
	if hasPanel {
		topArea = splitLayout(vpView, m.activePanel, m.width, m.viewport.Height)
	}

	var sections []string
	sections = append(sections, topArea)

	// 3. Command palette or file picker (full width, between viewport and prompt)
	if paletteView := m.palette.View(); paletteView != "" {
		sections = append(sections, paletteView)
	}
	if pickerView := m.filePicker.View(); pickerView != "" {
		sections = append(sections, pickerView)
	}

	// 4. Divider + Prompt (full width)
	sections = append(sections, styles.SeparatorLine(mw))
	sections = append(sections, m.prompt.View())

	// 5. Mode line (full width) — or search bar when searching
	if m.vpSearchActive {
		sections = append(sections, m.renderSearchBar())
	} else {
		sections = append(sections, m.renderModeLine())
	}

	// 6. Status bar (full width)
	hint := m.statusHint()
	ctxUsed, ctxMax := m.contextBudget()
	sections = append(sections, renderStatusBar(m.width, StatusBarState{
		Model:       m.model,
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
	}))

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
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

// renderModeLine renders the vim-style mode line below the prompt.
func (m Model) renderModeLine() string {
	vimMode := m.vimModeDisplay()

	modeStyle := lipgloss.NewStyle().Foreground(styles.Muted)
	arrowStyle := lipgloss.NewStyle().Foreground(styles.Primary)
	hintStyle := lipgloss.NewStyle().Foreground(styles.Dim)

	var left string
	if vimMode != "" {
		left = modeStyle.Render("-- " + vimMode + " --")
	}

	// Permission mode indicator
	permMode := "default"
	if m.engineConfig != nil && m.engineConfig.PermissionMode != "" {
		permMode = m.engineConfig.PermissionMode
	}
	modeIndicator := arrowStyle.Render(" ▸▸ ") +
		hintStyle.Render(permMode+" mode")

	// Show queue count
	if len(m.messageQueue) > 0 {
		queueStyle := lipgloss.NewStyle().Foreground(styles.Warning)
		modeIndicator += queueStyle.Render(fmt.Sprintf("  [%d queued]", len(m.messageQueue)))
	}

	right := hintStyle.Render("(shift+tab to cycle)")

	content := left + modeIndicator
	gap := m.width - lipgloss.Width(content) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}

	return " " + content + strings.Repeat(" ", gap) + right + " "
}

// renderSearchBar renders the search input line.
func (m Model) renderSearchBar() string {
	searchStyle := lipgloss.NewStyle().Foreground(styles.Warning).Bold(true)
	queryStyle := lipgloss.NewStyle().Foreground(styles.Text)
	hintStyle := lipgloss.NewStyle().Foreground(styles.Dim)

	left := searchStyle.Render("/") + queryStyle.Render(m.vpSearchQuery) + searchStyle.Render("▌")

	var right string
	if len(m.vpSearchMatches) > 0 {
		right = hintStyle.Render(fmt.Sprintf("[%d/%d]", m.vpSearchIdx+1, len(m.vpSearchMatches)))
	} else if m.vpSearchQuery != "" {
		right = hintStyle.Render("[no matches]")
	}

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}
	return " " + left + strings.Repeat(" ", gap) + right + " "
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
func parseNewMemory(content string) *memory.Entry {
	lines := strings.Split(content, "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return nil
	}

	endIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			endIdx = i
			break
		}
	}
	if endIdx < 0 {
		return nil
	}

	entry := &memory.Entry{}
	for _, line := range lines[1:endIdx] {
		line = strings.TrimSpace(line)
		if idx := strings.Index(line, ":"); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			val = strings.Trim(val, `"'`)
			switch key {
			case "name":
				entry.Name = val
			case "description":
				entry.Description = val
			case "type":
				entry.Type = val
			}
		}
	}

	entry.Content = strings.TrimSpace(strings.Join(lines[endIdx+1:], "\n"))
	if entry.Name == "" || entry.Name == "new-memory" {
		return nil // User didn't fill in the name
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
	default:
		return ""
	}
}

func (m Model) statusHint() string {
	if m.focus == FocusPermission {
		return "y allow · n deny"
	}
	if m.focus == FocusPanel {
		return "j/k navigate · enter select · esc close"
	}
	if m.focus == FocusViewport {
		if m.vpSearchActive {
			return "type to search · enter confirm · esc cancel"
		}
		if len(m.vpSearchMatches) > 0 {
			return "j/k scroll · n/N next/prev match · / search · p pin · i/q back"
		}
		return "j/k scroll · / search · p pin · ctrl+o expand · i/q back"
	}
	if m.focus == FocusModelSelector {
		return "\u2191\u2193 select · enter confirm · esc cancel"
	}
	if m.palette.IsActive() || m.filePicker.IsActive() {
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
