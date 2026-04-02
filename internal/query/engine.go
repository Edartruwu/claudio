package query

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/hooks"
	"github.com/Abraxas-365/claudio/internal/security"
	"github.com/Abraxas-365/claudio/internal/services/analytics"
	"github.com/Abraxas-365/claudio/internal/services/compact"
	"github.com/Abraxas-365/claudio/internal/tasks"
	"github.com/Abraxas-365/claudio/internal/tools"
)

// EventHandler receives query engine events for UI rendering.
type EventHandler interface {
	OnTextDelta(text string)
	OnThinkingDelta(text string)
	OnToolUseStart(toolUse tools.ToolUse)
	OnToolUseEnd(toolUse tools.ToolUse, result *tools.Result)
	OnTurnComplete(usage api.Usage)
	OnError(err error)

	// OnToolApprovalNeeded is called when a tool requires approval.
	// Returns true if the tool is approved, false if denied.
	OnToolApprovalNeeded(toolUse tools.ToolUse) bool
}

// EngineConfig holds all dependencies for the query engine.
type EngineConfig struct {
	Hooks          *hooks.Manager
	Analytics      *analytics.Tracker
	CompactState   *compact.State
	TaskRuntime    *tasks.Runtime
	SessionID      string
	Model          string
	PermissionMode string // "default", "auto", "headless", "plan"
}

// Engine orchestrates the AI conversation loop.
type Engine struct {
	client         *api.Client
	registry       *tools.Registry
	handler        EventHandler
	messages       []api.Message
	system         string
	hooks          *hooks.Manager
	analytics      *analytics.Tracker
	compactState   *compact.State
	taskRuntime    *tasks.Runtime
	sessionID      string
	model          string
	permissionMode string
}

// NewEngine creates a new query engine (basic constructor for backwards compatibility).
func NewEngine(client *api.Client, registry *tools.Registry, handler EventHandler) *Engine {
	return &Engine{
		client:         client,
		registry:       registry,
		handler:        handler,
		permissionMode: "default",
	}
}

// NewEngineWithConfig creates a fully-configured query engine.
func NewEngineWithConfig(client *api.Client, registry *tools.Registry, handler EventHandler, cfg EngineConfig) *Engine {
	e := NewEngine(client, registry, handler)
	e.hooks = cfg.Hooks
	e.analytics = cfg.Analytics
	e.compactState = cfg.CompactState
	e.taskRuntime = cfg.TaskRuntime
	e.sessionID = cfg.SessionID
	e.model = cfg.Model
	e.permissionMode = cfg.PermissionMode
	if e.permissionMode == "" {
		e.permissionMode = "default"
	}
	return e
}

// SetSystem sets the system prompt.
func (e *Engine) SetSystem(prompt string) {
	e.system = prompt
}

// Messages returns the current conversation messages.
func (e *Engine) Messages() []api.Message {
	return e.messages
}

// SetMessages replaces the conversation messages (used after compaction).
func (e *Engine) SetMessages(msgs []api.Message) {
	e.messages = msgs
}

// Run executes a single user turn: sends the message, processes the AI response,
// executes any tool calls, and loops until the AI produces a final response.
func (e *Engine) Run(ctx context.Context, userMessage string) error {
	return e.RunWithImages(ctx, userMessage, nil)
}

// RunWithImages executes a user turn with optional image attachments.
func (e *Engine) RunWithImages(ctx context.Context, userMessage string, images []api.UserContentBlock) error {
	// Structured content: images + text
	blocks := make([]api.UserContentBlock, 0, len(images)+1)
	blocks = append(blocks, images...)
	blocks = append(blocks, api.NewTextBlock(userMessage))
	return e.RunWithBlocks(ctx, blocks)
}

// RunWithBlocks executes a user turn with pre-built content blocks.
func (e *Engine) RunWithBlocks(ctx context.Context, blocks []api.UserContentBlock) error {
	content, _ := json.Marshal(blocks)
	e.messages = append(e.messages, api.Message{
		Role:    "user",
		Content: content,
	})

	// Loop until end_turn (no more tool calls)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Build request
		req := &api.MessagesRequest{
			Messages:  e.messages,
			System:    e.system,
			MaxTokens: 16384,
			Tools:     e.registry.APIDefinitions(),
		}

		// Stream response
		response, err := e.streamResponse(ctx, req)
		if err != nil {
			e.handler.OnError(err)
			return err
		}

		// Append assistant message — strip thinking blocks since they
		// require a signature field when sent back and are ephemeral
		var filteredContent []api.ContentBlock
		for _, block := range response.Content {
			if block.Type != "thinking" {
				filteredContent = append(filteredContent, block)
			}
		}
		assistantContent, _ := json.Marshal(filteredContent)
		e.messages = append(e.messages, api.Message{
			Role:    "assistant",
			Content: assistantContent,
		})

		e.handler.OnTurnComplete(response.Usage)

		// Track analytics
		if e.analytics != nil {
			e.analytics.RecordUsage(response.Usage.InputTokens, response.Usage.OutputTokens)
		}
		// Track compact state
		if e.compactState != nil {
			e.compactState.TotalTokens += response.Usage.InputTokens + response.Usage.OutputTokens
		}

		// Check if we're done
		if response.StopReason == "end_turn" || len(response.ToolUses) == 0 {
			// Fire Stop hook
			e.fireHook(ctx, hooks.Stop, "", "")
			return nil
		}

		// Poll background tasks and inject completed results
		e.pollBackgroundTasks()

		// Execute tools and build result message
		toolResults := e.executeTools(ctx, response.ToolUses)

		// Append tool results as a user message
		resultContent, _ := json.Marshal(toolResults)
		e.messages = append(e.messages, api.Message{
			Role:    "user",
			Content: resultContent,
		})
	}
}

// streamedResponse holds the parsed results of a streamed API response.
type streamedResponse struct {
	Content    []api.ContentBlock
	ToolUses   []tools.ToolUse
	StopReason string
	Usage      api.Usage
}

func (e *Engine) streamResponse(ctx context.Context, req *api.MessagesRequest) (*streamedResponse, error) {
	eventCh, errCh := e.client.StreamMessages(ctx, req)

	response := &streamedResponse{}

	// Track current content block being built
	var currentBlocks []api.ContentBlock
	var currentBlockIdx int = -1

	for {
		select {
		case event, ok := <-eventCh:
			if !ok {
				return response, nil
			}

			switch event.Type {
			case "message_start":
				// Parse initial message data including usage (input_tokens)
				if event.MessageField != nil {
					var msg api.MessageResp
					json.Unmarshal(event.MessageField, &msg)
					if msg.Usage.InputTokens > 0 {
						response.Usage.InputTokens = msg.Usage.InputTokens
					}
				}

			case "content_block_start":
				// New content block - index is in event.Index, block data in event.ContentBlock
				var block api.ContentBlock
				if event.ContentBlock != nil {
					json.Unmarshal(event.ContentBlock, &block)
				}
				currentBlockIdx = event.Index
				for len(currentBlocks) <= currentBlockIdx {
					currentBlocks = append(currentBlocks, api.ContentBlock{})
				}

				if block.Type == "tool_use" {
					// Clear the initial empty {} so input_json_delta can accumulate cleanly
					block.Input = nil
					tu := tools.ToolUse{
						ID:   block.ID,
						Name: block.Name,
					}
					e.handler.OnToolUseStart(tu)
				}
				currentBlocks[currentBlockIdx] = block

			case "content_block_delta":
				if event.Delta == nil {
					continue
				}
				var delta struct {
					Type         string `json:"type"`
					Text         string `json:"text"`
					PartialJSON  string `json:"partial_json"`
					Thinking     string `json:"thinking"`
					Signature    string `json:"signature"`
				}
				json.Unmarshal(event.Delta, &delta)

				switch delta.Type {
				case "text_delta":
					e.handler.OnTextDelta(delta.Text)
					if currentBlockIdx >= 0 && currentBlockIdx < len(currentBlocks) {
						currentBlocks[currentBlockIdx].Text += delta.Text
					}

				case "thinking_delta":
					e.handler.OnThinkingDelta(delta.Thinking)

				case "signature_delta":
					// Capture the thinking block's signature for API roundtrip compliance
					if currentBlockIdx >= 0 && currentBlockIdx < len(currentBlocks) {
						currentBlocks[currentBlockIdx].Signature += delta.Signature
					}
					if currentBlockIdx >= 0 && currentBlockIdx < len(currentBlocks) {
						currentBlocks[currentBlockIdx].Thinking += delta.Thinking
					}

				case "input_json_delta":
					if currentBlockIdx >= 0 && currentBlockIdx < len(currentBlocks) {
						// Accumulate partial JSON for tool input
						existing := string(currentBlocks[currentBlockIdx].Input)
						currentBlocks[currentBlockIdx].Input = json.RawMessage(existing + delta.PartialJSON)
					}
				}

			case "content_block_stop":
				if currentBlockIdx >= 0 && currentBlockIdx < len(currentBlocks) {
					block := currentBlocks[currentBlockIdx]
					if block.Type == "tool_use" {
						response.ToolUses = append(response.ToolUses, tools.ToolUse{
							ID:    block.ID,
							Name:  block.Name,
							Input: block.Input,
						})
					}
				}

			case "message_delta":
				if event.Delta != nil {
					var delta struct {
						StopReason string `json:"stop_reason"`
					}
					json.Unmarshal(event.Delta, &delta)
					if delta.StopReason != "" {
						response.StopReason = delta.StopReason
					}
				}
				if event.Usage != nil && event.Usage.OutputTokens > 0 {
					response.Usage.OutputTokens = event.Usage.OutputTokens
				}

			case "message_stop":
				response.Content = currentBlocks
				return response, nil
			}

		case err, ok := <-errCh:
			if !ok {
				continue
			}
			return nil, err

		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (e *Engine) executeTools(ctx context.Context, toolUses []tools.ToolUse) []toolResultBlock {
	var results []toolResultBlock

	for _, tu := range toolUses {
		tool, err := e.registry.Get(tu.Name)
		if err != nil {
			result := &tools.Result{
				Content: fmt.Sprintf("Unknown tool: %s", tu.Name),
				IsError: true,
			}
			e.handler.OnToolUseEnd(tu, result)
			results = append(results, toolResultBlock{
				Type:      "tool_result",
				ToolUseID: tu.ID,
				Content:   result.Content,
				IsError:   true,
			})
			continue
		}

		// 1. PreToolUse hook
		if blocked := e.fireHook(ctx, hooks.PreToolUse, tu.Name, string(tu.Input)); blocked {
			result := &tools.Result{
				Content: fmt.Sprintf("Tool %s was blocked by a PreToolUse hook", tu.Name),
				IsError: true,
			}
			e.handler.OnToolUseEnd(tu, result)
			results = append(results, toolResultBlock{
				Type:      "tool_result",
				ToolUseID: tu.ID,
				Content:   result.Content,
				IsError:   true,
			})
			continue
		}

		// 2. Permission check (mode-aware)
		if e.shouldRequireApproval(tool, tu.Input) {
			if !e.handler.OnToolApprovalNeeded(tu) {
				result := &tools.Result{
					Content: fmt.Sprintf("Tool %s was denied by user", tu.Name),
					IsError: true,
				}
				e.handler.OnToolUseEnd(tu, result)
				results = append(results, toolResultBlock{
					Type:      "tool_result",
					ToolUseID: tu.ID,
					Content:   result.Content,
					IsError:   true,
				})
				continue
			}
		}

		// 3. Execute
		if e.analytics != nil {
			e.analytics.RecordToolCall()
		}

		result, err := tool.Execute(ctx, tu.Input)
		if err != nil {
			result = &tools.Result{
				Content: fmt.Sprintf("Tool execution error: %v", err),
				IsError: true,
			}
		}

		// 4. PostToolUse hook
		if result.IsError {
			e.fireHook(ctx, hooks.PostToolUseFailure, tu.Name, result.Content)
		} else {
			e.fireHook(ctx, hooks.PostToolUse, tu.Name, result.Content)
		}

		// 5. Secret scanning on output
		if secrets := security.ScanForSecrets(result.Content); len(secrets) > 0 {
			result.Content = security.RedactSecrets(result.Content)
			result.Content += "\n\n[WARNING: Potential secrets detected and redacted in output]"
		}

		e.handler.OnToolUseEnd(tu, result)

		// Truncate very large results
		content := result.Content
		const maxContent = 100000
		if len(content) > maxContent {
			content = content[:maxContent] + "\n... (truncated)"
		}

		results = append(results, toolResultBlock{
			Type:      "tool_result",
			ToolUseID: tu.ID,
			Content:   content,
			IsError:   result.IsError,
		})
	}

	return results
}

// pollBackgroundTasks checks for completed background tasks and injects their results
// as additional context for the next turn.
func (e *Engine) pollBackgroundTasks() {
	if e.taskRuntime == nil {
		return
	}

	completed := e.taskRuntime.PollResults()
	if len(completed) == 0 {
		return
	}

	// Build a notification message with all completed task results
	var notifications []string
	for _, t := range completed {
		status := fmt.Sprintf("[Background task %s (%s): %s]", t.ID, t.Type, t.Status)
		if t.Error != "" {
			status += fmt.Sprintf(" Error: %s", t.Error)
		}

		// Read last 4KB of output as summary
		if t.OutputFile != "" {
			content, _, _ := tasks.ReadDelta(t.OutputFile, 0, 4096)
			if content != "" {
				if len(content) > 2000 {
					content = content[len(content)-2000:]
				}
				status += "\nOutput (tail):\n" + content
			}
		}
		notifications = append(notifications, status)
	}

	if len(notifications) > 0 {
		// Inject as a system reminder
		reminderText := fmt.Sprintf("<system-reminder>\n%s\n</system-reminder>",
			strings.Join(notifications, "\n\n"))

		content, _ := json.Marshal(reminderText)
		e.messages = append(e.messages, api.Message{
			Role:    "user",
			Content: content,
		})
	}

	// Evict old terminal tasks (older than 5 minutes)
	e.taskRuntime.Evict(5 * time.Minute)
}

// shouldRequireApproval checks if a tool needs user approval based on permission mode.
func (e *Engine) shouldRequireApproval(tool tools.Tool, input json.RawMessage) bool {
	switch e.permissionMode {
	case "auto", "headless", "dangerously-skip-permissions":
		return false // auto-approve everything
	case "plan":
		return !tool.IsReadOnly() // block write tools in plan mode
	default: // "default"
		return tool.RequiresApproval(input)
	}
}

// fireHook executes a hook and returns true if the action was blocked.
func (e *Engine) fireHook(ctx context.Context, event hooks.Event, toolName, toolInput string) bool {
	if e.hooks == nil {
		return false
	}

	cwd, _ := os.Getwd()
	hctx := hooks.HookContext{
		Event:     event,
		ToolName:  toolName,
		ToolInput: toolInput,
		SessionID: e.sessionID,
		Model:     e.model,
		CWD:       cwd,
	}

	_, blocked := e.hooks.Run(ctx, event, hctx)
	return blocked
}

// toolResultBlock is the format the API expects for tool results.
type toolResultBlock struct {
	Type      string `json:"type"`
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

// StdoutHandler is a simple event handler that prints to stdout.
type StdoutHandler struct {
	Verbose bool
}

func (h *StdoutHandler) OnTextDelta(text string) {
	fmt.Print(text)
}

func (h *StdoutHandler) OnThinkingDelta(text string) {
	if h.Verbose {
		fmt.Print(text)
	}
}

func (h *StdoutHandler) OnToolUseStart(tu tools.ToolUse) {
	// Format a nice summary of what tool is being called
	var inputSummary string
	switch tu.Name {
	case "Bash":
		var in struct{ Command string }
		json.Unmarshal(tu.Input, &in)
		inputSummary = in.Command
	case "Read":
		var in struct{ FilePath string }
		json.Unmarshal(tu.Input, &in)
		inputSummary = in.FilePath
	case "Write":
		var in struct{ FilePath string }
		json.Unmarshal(tu.Input, &in)
		inputSummary = in.FilePath
	case "Edit":
		var in struct{ FilePath string }
		json.Unmarshal(tu.Input, &in)
		inputSummary = in.FilePath
	case "Glob":
		var in struct{ Pattern string }
		json.Unmarshal(tu.Input, &in)
		inputSummary = in.Pattern
	case "Grep":
		var in struct{ Pattern string }
		json.Unmarshal(tu.Input, &in)
		inputSummary = in.Pattern
	default:
		inputSummary = string(tu.Input)
		if len(inputSummary) > 80 {
			inputSummary = inputSummary[:80] + "..."
		}
	}
	fmt.Printf("\n--- %s: %s ---\n", tu.Name, inputSummary)
}

func (h *StdoutHandler) OnToolUseEnd(tu tools.ToolUse, result *tools.Result) {
	// Show abbreviated result
	content := result.Content
	lines := strings.Split(content, "\n")
	if len(lines) > 20 {
		content = strings.Join(lines[:20], "\n") + fmt.Sprintf("\n... (%d more lines)", len(lines)-20)
	}
	if result.IsError {
		fmt.Printf("[ERROR] %s\n", content)
	} else if h.Verbose {
		fmt.Printf("%s\n", content)
	}
}

func (h *StdoutHandler) OnTurnComplete(usage api.Usage) {
	if h.Verbose {
		fmt.Printf("\n[tokens: in=%d out=%d]\n", usage.InputTokens, usage.OutputTokens)
	}
}

func (h *StdoutHandler) OnToolApprovalNeeded(tu tools.ToolUse) bool {
	return true // Auto-approve in non-interactive mode
}

func (h *StdoutHandler) OnError(err error) {
	fmt.Printf("\n[error: %v]\n", err)
}
