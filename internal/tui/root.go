package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/cli/commands"
	"github.com/Abraxas-365/claudio/internal/query"
	"github.com/Abraxas-365/claudio/internal/services/compact"
	"github.com/Abraxas-365/claudio/internal/services/skills"
	"github.com/Abraxas-365/claudio/internal/session"
	"github.com/Abraxas-365/claudio/internal/tools"
	"github.com/Abraxas-365/claudio/internal/tui/commandpalette"
	"github.com/Abraxas-365/claudio/internal/tui/components"
	"github.com/Abraxas-365/claudio/internal/tui/filepicker"
	"github.com/Abraxas-365/claudio/internal/tui/modelselector"
	"github.com/Abraxas-365/claudio/internal/tui/permissions"
	"github.com/Abraxas-365/claudio/internal/tui/prompt"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// Focus tracks which component has input focus.
type Focus int

const (
	FocusPrompt Focus = iota
	FocusViewport // vim-navigable chat viewport
	FocusPermission
	FocusModelSelector
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
	modelSelector modelselector.Model

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
	leaderPending   bool         // space leader key pressed, waiting for next key
	leaderW         bool         // space+w pressed, waiting for window direction
	vpCursor        int          // viewport section cursor (-1 = none)
	vpSections      []Section    // cached section metadata from last render
	autoEdit        bool         // auto-accept edits mode (shift+tab cycles)

	// Engine integration
	engine       *query.Engine
	apiClient    *api.Client
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
		expandedGroups: make(map[int]bool),
		lastToolGroup:  -1,
		vpCursor:       -1,
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
		Compact: func(keepLast int) (string, error) {
			if m.engine == nil {
				return "", fmt.Errorf("no active conversation")
			}
			msgs := m.engine.Messages()
			compacted, summary, err := compact.Compact(
				context.Background(), apiClient, msgs, keepLast,
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
		m.modelSelector.SetWidth(m.width)
		m.layout()
		return m, nil

	case prompt.VimEscapeMsg:
		// Vim consumed Escape (Insert→Normal). Don't dismiss anything.
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "shift+tab":
			// Cycle permission mode: default → auto → plan → default
			if !m.streaming {
				m.cyclePermissionMode()
				return m, nil
			}
		case "ctrl+c":
			if m.cancelFunc != nil {
				m.cancelFunc()
			}
			return m, tea.Quit
		case "ctrl+o":
			// Toggle expand/collapse on the last tool group
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
			// If vim is in Insert mode, let the prompt handle Escape first
			// (it will send VimEscapeMsg, handled above)
			if m.prompt.IsVimEnabled() && !m.prompt.IsVimNormal() {
				break // fall through to prompt.Update below
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
			if m.streaming && m.cancelFunc != nil {
				m.cancelFunc()
				m.streaming = false
				m.spinText = ""
				m.spinner.Stop()
				m.prompt.Focus()
				m.focus = FocusPrompt
			}
		}

		// Viewport focus mode: section-based navigation with cursor
		if m.focus == FocusViewport {
			// Leader key sequences
			if m.leaderPending {
				m.leaderPending = false
				if msg.String() == "w" {
					m.leaderW = true
					return m, nil
				}
				return m, nil
			}
			if m.leaderW {
				m.leaderW = false
				switch msg.String() {
				case "j", "p":
					m.focus = FocusPrompt
					m.vpCursor = -1
					m.prompt.Focus()
					m.refreshViewport()
					return m, nil
				case "k":
					return m, nil
				}
				return m, nil
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
			case "i", "esc", "q":
				m.focus = FocusPrompt
				m.vpCursor = -1
				m.prompt.Focus()
				m.refreshViewport()
				return m, nil
			case " ":
				m.leaderPending = true
				return m, nil
			}
			return m, nil
		}

		// Leader key: <Space> in Normal mode (works with or without text)
		if m.focus == FocusPrompt && m.prompt.IsVimNormal() {
			if m.leaderPending {
				m.leaderPending = false
				if msg.String() == "w" {
					m.leaderW = true
					return m, nil
				}
				// Not a recognized leader sequence — re-inject space as normal key
				return m, nil
			}
			if m.leaderW {
				m.leaderW = false
				switch msg.String() {
				case "k":
					// Switch to viewport (viewport is "above" prompt)
					m.focus = FocusViewport
					m.prompt.Blur()
					// Place cursor on last section
					if len(m.vpSections) > 0 {
						m.vpCursor = len(m.vpSections) - 1
					} else {
						m.vpCursor = 0
					}
					m.refreshViewport()
					m.scrollToSection(m.vpCursor)
					return m, nil
				case "j":
					// Already at prompt (prompt is "below"), stay
					return m, nil
				}
				return m, nil
			}
			if msg.String() == " " {
				m.leaderPending = true
				return m, nil
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

	case filepicker.SelectMsg:
		m.filePicker.Deactivate()
		// Check if selected file is an image
		if IsImageFile(msg.Path) {
			absPath := msg.Path
			if !filepath.IsAbs(absPath) {
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
		m.addMessage(ChatMessage{Type: MsgSystem, Content: fmt.Sprintf("Model set to: %s (%s)", msg.Label, msg.ModelID)})
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
	m.addMessage(ChatMessage{Type: MsgUser, Content: displayText})
	m.refreshViewport()

	m.streaming = true
	m.streamText.Reset()
	m.spinText = "Thinking..."
	m.spinner.Start("Thinking...")
	m.prompt.Blur()

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

	ctx, cancel := context.WithCancel(context.Background())
	m.cancelFunc = cancel

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

	return m, tea.Batch(m.spinner.Tick(), m.waitForEvent())
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
		m.totalTokens += event.usage.InputTokens + event.usage.OutputTokens
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
		return m, nil

	case "error":
		m.addMessage(ChatMessage{Type: MsgError, Content: event.err.Error()})
		m.refreshViewport()
	}

	return m, tea.Batch(m.spinner.Tick(), m.waitForEvent())
}

func (m Model) handleCommand(name, args string) (tea.Model, tea.Cmd) {
	// /model without args → interactive selector
	if name == "model" && args == "" {
		m.modelSelector = modelselector.New(m.apiClient.GetModel())
		m.modelSelector.SetWidth(m.width)
		m.focus = FocusModelSelector
		m.prompt.Blur()
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
	case permissions.DecisionAllow, permissions.DecisionAllowAlways:
		m.approvalCh <- true
	case permissions.DecisionDeny:
		m.approvalCh <- false
		m.addMessage(ChatMessage{Type: MsgSystem, Content: fmt.Sprintf("Denied %s", resp.ToolUse.Name)})
	}

	m.refreshViewport()
	return m, tea.Batch(m.spinner.Tick(), m.waitForEvent())
}

// ── Message Management ───────────────────────────────────

func (m *Model) addMessage(msg ChatMessage) {
	m.messages = append(m.messages, msg)
}

func (m *Model) updateStreamingMessage() {
	text := m.streamText.String()
	if text == "" {
		return
	}
	if len(m.messages) > 0 && m.messages[len(m.messages)-1].Type == MsgAssistant {
		m.messages[len(m.messages)-1].Content = text
	} else {
		m.messages = append(m.messages, ChatMessage{Type: MsgAssistant, Content: text})
	}
}

func (m *Model) finalizeStreamingMessage() {
	if m.streamText.Len() > 0 {
		m.updateStreamingMessage()
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
		result := renderMessages(m.messages, m.width, m.expandedGroups, cursorIdx)
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
	if m.focus != FocusViewport {
		m.viewport.GotoBottom()
	}
}

// scrollToSection scrolls the viewport so the given section is visible.
func (m *Model) scrollToSection(idx int) {
	if idx < 0 || idx >= len(m.vpSections) {
		return
	}
	sec := m.vpSections[idx]
	// Scroll so the section's start line is near the top
	m.viewport.SetYOffset(sec.LineStart)
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
		Render("/help commands  ·  @file context  ·  /vim keybindings  ·  /model switch")

	block := lipgloss.JoinVertical(lipgloss.Center,
		"",
		title,
		subtitle,
		"",
		hints,
		"",
	)

	return lipgloss.Place(
		m.width, m.viewport.Height,
		lipgloss.Center, lipgloss.Center,
		block,
	)
}

// ── Layout & View ────────────────────────────────────────

func (m *Model) layout() {
	statusH := 1
	promptH := 4
	paletteH := 0
	if m.palette.IsActive() || m.filePicker.IsActive() {
		paletteH = 10
	}

	modeLineH := 1
	vpHeight := m.height - statusH - promptH - paletteH - modeLineH - 1
	if vpHeight < 5 {
		vpHeight = 5
	}

	m.viewport.Width = m.width
	m.viewport.Height = vpHeight
	m.prompt.SetWidth(m.width)
	m.permission.SetWidth(m.width)
}

func (m Model) View() string {
	m.layout()

	var sections []string

	// 1. Viewport (messages + inline spinner)
	vpView := m.viewport.View()

	// Overlay permission dialog or model selector on top of viewport
	if m.permission.IsActive() {
		overlay := m.permission.View()
		vpView = placeOverlay(vpView, overlay, m.width, m.viewport.Height)
	}
	if m.modelSelector.IsActive() {
		overlay := m.modelSelector.View()
		vpView = placeOverlay(vpView, overlay, m.width, m.viewport.Height)
	}

	sections = append(sections, vpView)

	// 2. Command palette or file picker (between viewport and prompt)
	if paletteView := m.palette.View(); paletteView != "" {
		sections = append(sections, paletteView)
	}
	if pickerView := m.filePicker.View(); pickerView != "" {
		sections = append(sections, pickerView)
	}

	// 3. Prompt
	sections = append(sections, m.prompt.View())

	// 4. Mode line (like vim's -- INSERT -- line)
	sections = append(sections, m.renderModeLine())

	// 5. Status bar
	hint := m.statusHint()
	sections = append(sections, renderStatusBar(m.width, StatusBarState{
		Model:     m.model,
		Tokens:    m.totalTokens,
		Cost:      m.totalCost,
		Turns:     m.turns,
		Streaming: m.streaming,
		SpinText:  m.spinText,
		Hint:      hint,
		VimMode:   m.vimModeDisplay(),
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

	right := hintStyle.Render("(shift+tab to cycle)")

	content := left + modeIndicator
	gap := m.width - lipgloss.Width(content) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}

	return " " + content + strings.Repeat(" ", gap) + right + " "
}

// statusHint returns contextual keyboard hints for the status bar.
func (m Model) vimModeDisplay() string {
	if m.focus == FocusViewport {
		return "VIEWPORT"
	}
	return m.prompt.VimModeString()
}

func (m Model) statusHint() string {
	if m.focus == FocusPermission {
		return "y allow · n deny"
	}
	if m.focus == FocusViewport {
		return "j/k scroll · G/gg top/bottom · ctrl+o expand · i/q back to prompt"
	}
	if m.focus == FocusModelSelector {
		return "\u2191\u2193 select · enter confirm · esc cancel"
	}
	if m.palette.IsActive() || m.filePicker.IsActive() {
		return "\u2191\u2193 navigate · enter select · esc close"
	}
	if m.streaming {
		return "esc stop"
	}
	if m.lastToolGroup >= 0 {
		return "<Space>wk viewport · ctrl+o expand · ctrl+v image"
	}
	return "<Space>wk viewport · ctrl+v image"
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

func (h *tuiEventHandler) OnError(err error) {
	h.ch <- tuiEvent{typ: "error", err: err}
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
