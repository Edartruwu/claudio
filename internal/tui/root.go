package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/cli/commands"
	"github.com/Abraxas-365/claudio/internal/query"
	"github.com/Abraxas-365/claudio/internal/tools"
	"github.com/Abraxas-365/claudio/internal/tui/components"
	"github.com/Abraxas-365/claudio/internal/tui/permissions"
	"github.com/Abraxas-365/claudio/internal/tui/prompt"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// Focus tracks which component has input focus.
type Focus int

const (
	FocusPrompt Focus = iota
	FocusPermission
)

// Model is the root Bubble Tea model for the TUI.
type Model struct {
	// Components
	viewport   viewport.Model
	prompt     prompt.Model
	spinner    components.SpinnerModel
	permission permissions.Model

	// State
	messages      []ChatMessage
	focus         Focus
	width, height int
	streaming     bool
	streamText    strings.Builder
	model         string
	totalTokens   int
	totalCost     float64

	// Engine integration
	engine       *query.Engine
	apiClient    *api.Client
	registry     *tools.Registry
	cancelFunc   context.CancelFunc
	eventCh      chan tuiEvent
	systemPrompt string
	commands     *commands.Registry
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
	engineEventMsg tuiEvent
	engineDoneMsg  struct{ err error }
)

// New creates a new TUI model.
func New(apiClient *api.Client, registry *tools.Registry, systemPrompt string) Model {
	vp := viewport.New(80, 20)
	vp.SetContent("")

	cmdRegistry := commands.NewRegistry()
	commands.RegisterCoreCommands(cmdRegistry)

	return Model{
		viewport:     vp,
		prompt:       prompt.New(),
		spinner:      components.NewSpinner(),
		focus:        FocusPrompt,
		model:        "claude-sonnet-4-6",
		apiClient:    apiClient,
		registry:     registry,
		eventCh:      make(chan tuiEvent, 64),
		systemPrompt: systemPrompt,
		commands:     cmdRegistry,
	}
}

// Init initializes the model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tea.SetWindowTitle("Claudio"),
		m.spinner.Tick(),
		m.waitForEvent(),
	)
}

// waitForEvent listens for query engine events.
func (m Model) waitForEvent() tea.Cmd {
	return func() tea.Msg {
		event, ok := <-m.eventCh
		if !ok {
			return nil
		}
		return engineEventMsg(event)
	}
}

// Update handles all messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			if m.cancelFunc != nil {
				m.cancelFunc()
			}
			return m, tea.Quit
		case "esc":
			if m.streaming && m.cancelFunc != nil {
				m.cancelFunc()
				m.streaming = false
				m.spinner.Stop()
				m.prompt.Focus()
				m.focus = FocusPrompt
			}
		}

	case prompt.SubmitMsg:
		return m.handleSubmit(msg.Text)

	case permissions.ResponseMsg:
		return m.handlePermissionResponse(msg)

	case engineEventMsg:
		return m.handleEngineEvent(tuiEvent(msg))

	case engineDoneMsg:
		m.streaming = false
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

	case FocusPermission:
		var cmd tea.Cmd
		m.permission, cmd = m.permission.Update(msg)
		cmds = append(cmds, cmd)
	}

	// Update spinner
	var spinCmd tea.Cmd
	m.spinner, spinCmd = m.spinner.Update(msg)
	cmds = append(cmds, spinCmd)

	return m, tea.Batch(cmds...)
}

func (m Model) handleSubmit(text string) (tea.Model, tea.Cmd) {
	// Check for slash commands
	if cmdName, cmdArgs, isCmd := commands.Parse(text); isCmd {
		return m.handleCommand(cmdName, cmdArgs)
	}

	// Add user message
	m.addMessage(ChatMessage{Type: MsgUser, Content: text})
	m.refreshViewport()

	// Start streaming
	m.streaming = true
	m.streamText.Reset()
	m.spinner.Start("Thinking...")
	m.prompt.Blur()

	// Create query engine with TUI event handler
	handler := &tuiEventHandler{ch: m.eventCh}
	m.engine = query.NewEngine(m.apiClient, m.registry, handler)
	if m.systemPrompt != "" {
		m.engine.SetSystem(m.systemPrompt)
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.cancelFunc = cancel

	// Run engine in background goroutine
	go func() {
		err := m.engine.Run(ctx, text)
		m.eventCh <- tuiEvent{typ: "done", err: err}
	}()

	return m, tea.Batch(m.spinner.Tick(), m.waitForEvent())
}

func (m Model) handleEngineEvent(event tuiEvent) (tea.Model, tea.Cmd) {
	switch event.typ {
	case "text_delta":
		m.streamText.WriteString(event.text)
		m.spinner.SetText("Responding...")
		// Update the last assistant message or create one
		m.updateStreamingMessage()
		m.refreshViewport()

	case "thinking_delta":
		// Show thinking indicator
		m.spinner.SetText("Thinking deeply...")

	case "tool_start":
		// Finalize any streaming text
		m.finalizeStreamingMessage()
		m.spinner.SetText(fmt.Sprintf("Using %s...", event.toolUse.Name))

		// Check if tool needs approval
		tool, err := m.registry.Get(event.toolUse.Name)
		if err == nil && tool.RequiresApproval(event.toolUse.Input) {
			// Show permission dialog
			m.permission = permissions.New(event.toolUse)
			m.permission.SetWidth(m.width)
			m.focus = FocusPermission
			m.refreshViewport()
			return m, m.waitForEvent()
		}

		m.addMessage(ChatMessage{
			Type:      MsgToolUse,
			ToolName:  event.toolUse.Name,
			ToolInput: formatToolSummary(event.toolUse),
		})
		m.refreshViewport()

	case "tool_end":
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
		// Rough cost estimate (Sonnet pricing)
		m.totalCost += float64(event.usage.InputTokens) * 3.0 / 1_000_000
		m.totalCost += float64(event.usage.OutputTokens) * 15.0 / 1_000_000

	case "done":
		m.finalizeStreamingMessage()
		m.streaming = false
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
		m.addMessage(ChatMessage{Type: MsgSystem, Content: output})
		m.refreshViewport()
	}

	return m, nil
}

func (m Model) handlePermissionResponse(resp permissions.ResponseMsg) (tea.Model, tea.Cmd) {
	m.focus = FocusPrompt

	switch resp.Decision {
	case permissions.DecisionAllow, permissions.DecisionAllowAlways:
		m.addMessage(ChatMessage{
			Type:      MsgToolUse,
			ToolName:  resp.ToolUse.Name,
			ToolInput: formatToolSummary(resp.ToolUse),
		})
	case permissions.DecisionDeny:
		m.addMessage(ChatMessage{Type: MsgSystem, Content: fmt.Sprintf("Denied %s", resp.ToolUse.Name)})
	}

	m.refreshViewport()
	return m, tea.Batch(m.spinner.Tick(), m.waitForEvent())
}

func (m *Model) addMessage(msg ChatMessage) {
	m.messages = append(m.messages, msg)
}

func (m *Model) updateStreamingMessage() {
	text := m.streamText.String()
	if text == "" {
		return
	}

	// Find or create the streaming assistant message
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
	var rendered []string
	for _, msg := range m.messages {
		rendered = append(rendered, renderMessage(msg, m.width))
	}

	content := strings.Join(rendered, "\n\n")
	m.viewport.SetContent(content)
	m.viewport.GotoBottom()
}

func (m *Model) layout() {
	footerH := 1
	promptH := 5
	spinnerH := 0
	if m.spinner.IsActive() {
		spinnerH = 1
	}
	permH := 0
	if m.permission.IsActive() {
		permH = 8
	}

	vpHeight := m.height - footerH - promptH - spinnerH - permH - 2
	if vpHeight < 5 {
		vpHeight = 5
	}

	m.viewport.Width = m.width
	m.viewport.Height = vpHeight
	m.prompt.SetWidth(m.width)
	m.permission.SetWidth(m.width)
}

// View renders the full TUI.
func (m Model) View() string {
	m.layout()

	var sections []string

	// Message viewport
	sections = append(sections, m.viewport.View())

	// Spinner (if active)
	if spinView := m.spinner.View(); spinView != "" {
		sections = append(sections, spinView)
	}

	// Permission dialog (if active)
	if m.permission.IsActive() {
		sections = append(sections, m.permission.View())
	}

	// Separator
	sections = append(sections, styles.SeparatorLine(m.width))

	// Prompt
	sections = append(sections, m.prompt.View())

	// Footer
	sections = append(sections, renderFooter(m.width, m.model, m.totalTokens, m.totalCost, ""))

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// tuiEventHandler implements query.EventHandler and sends events to the TUI channel.
type tuiEventHandler struct {
	ch chan<- tuiEvent
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

func (h *tuiEventHandler) OnError(err error) {
	h.ch <- tuiEvent{typ: "error", err: err}
}

func formatToolSummary(tu tools.ToolUse) string {
	switch tu.Name {
	case "Bash":
		var in struct{ Command string }
		json.Unmarshal(tu.Input, &in)
		return in.Command
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
