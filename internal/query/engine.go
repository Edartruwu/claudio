package query

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/config"
	"github.com/Abraxas-365/claudio/internal/hooks"
	"github.com/Abraxas-365/claudio/internal/permissions"
	"github.com/Abraxas-365/claudio/internal/security"
	"github.com/Abraxas-365/claudio/internal/services/analytics"
	"github.com/Abraxas-365/claudio/internal/services/cachetracker"
	"github.com/Abraxas-365/claudio/internal/services/compact"
	"github.com/Abraxas-365/claudio/internal/services/toolcache"
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

	// OnCostConfirmNeeded is called when session cost exceeds the confirmation threshold.
	// Returns true to continue, false to stop the session.
	OnCostConfirmNeeded(currentCost, threshold float64) bool
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
	OnTurnEnd      func(messages []api.Message) // background memory extraction callback

	// Permission pattern rules for content-based allow/deny
	PermissionRules []config.PermissionRule

	// Cost threshold for confirmation dialog (USD, 0 = disabled)
	CostConfirmThreshold float64
}

const (
	// defaultMaxTokens is used for normal requests to avoid over-reserving output
	// capacity (input capacity = context_window - max_tokens). Escalated on retry.
	defaultMaxTokens  = 8_192
	escalatedMaxTokens = 64_000
	maxContinuations  = 5
)

// Engine orchestrates the AI conversation loop.
type Engine struct {
	client          *api.Client
	registry        *tools.Registry
	handler         EventHandler
	messages        []api.Message
	system          string
	hooks           *hooks.Manager
	analytics       *analytics.Tracker
	compactState    *compact.State
	taskRuntime     *tasks.Runtime
	sessionID       string
	model           string
	permissionMode    string
	permissionRules   []config.PermissionRule
	costConfirmThresh float64
	discoveredTools   map[string]bool // tools discovered via ToolSearch
	onTurnEnd         func(messages []api.Message) // called when a turn ends (end_turn stop reason)

	// continuation tracking for diminishing returns detection
	continuationCount int
	lastOutputTokens  int

	// cache observability
	cacheTracker  *cachetracker.Tracker
	cacheExpiry   *cachetracker.ExpiryWatcher

	// tool result disk offload for oversized outputs
	toolCache *toolcache.Store
}

// NewEngine creates a new query engine (basic constructor for backwards compatibility).
func NewEngine(client *api.Client, registry *tools.Registry, handler EventHandler) *Engine {
	tc, _ := toolcache.New(os.TempDir()+"/claudio-tool-results", 0)
	return &Engine{
		client:          client,
		registry:        registry,
		handler:         handler,
		permissionMode:  "default",
		discoveredTools: make(map[string]bool),
		cacheTracker:    &cachetracker.Tracker{},
		cacheExpiry:     cachetracker.NewExpiryWatcher(5 * time.Minute),
		toolCache:       tc,
	}
}

// Close releases resources held by the engine (cleans up persisted tool result files).
func (e *Engine) Close() {
	if e.toolCache != nil {
		e.toolCache.Cleanup()
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
	e.permissionRules = cfg.PermissionRules
	e.costConfirmThresh = cfg.CostConfirmThreshold
	e.onTurnEnd = cfg.OnTurnEnd
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

// SetOnTurnEnd registers a callback that fires when a turn ends (end_turn stop reason).
// The callback receives a copy of the conversation messages.
func (e *Engine) SetOnTurnEnd(fn func(messages []api.Message)) {
	e.onTurnEnd = fn
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

	// Fire UserPromptSubmit hook
	e.fireHook(ctx, hooks.UserPromptSubmit, "", string(content))

	// Loop until end_turn (no more tool calls)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Warn once per turn if the cache has likely expired (>5 min since last call)
		if e.cacheExpiry.IsExpired() {
			e.handler.OnTextDelta("\n[Note: prompt cache likely expired — first response may be slower]\n")
			// Reset so we don't repeat the warning every loop iteration
			e.cacheExpiry.RecordCall()
		}

		// Auto-compact if approaching context limit (tiered)
		if e.compactState != nil {
			if e.compactState.ShouldForce() {
				// Full compaction at 95%
				e.fireHook(ctx, hooks.PreCompact, "", "")
				compacted, summary, err := compact.Compact(ctx, e.client, e.messages, 10)
				if err == nil && summary != "" {
					e.messages = compacted
					e.compactState.TotalTokens = 0
					e.handler.OnTextDelta("\n[Auto-compacted conversation: " + summary[:min(len(summary), 100)] + "...]\n")
					e.fireHook(ctx, hooks.PostCompact, "", "")
				}
			} else if e.compactState.ShouldPartialCompact() {
				// Partial: clear old tool results at 70%
				e.messages = compact.ContentClearCompact(e.messages, 20, 4096)
				e.handler.OnTextDelta("\n[Cleared old tool results to save context]\n")
			}
		}

		// Merge consecutive user messages before sending (reduces message count overhead)
		mergedMessages := mergeConsecutiveUserMessages(e.messages)

		// Build request with deferred tool loading
		req := &api.MessagesRequest{
			Messages:  mergedMessages,
			System:    e.buildSystemWithDeferredTools(),
			MaxTokens: defaultMaxTokens,
			Tools:     e.registry.APIDefinitionsWithDeferral(e.discoveredTools),
		}

		// Stream response
		response, err := e.streamResponse(ctx, req)
		if err != nil {
			e.handler.OnError(err)
			return err
		}

		// If we hit the output token limit, retry once with escalated max_tokens
		if response.StopReason == "max_tokens" && req.MaxTokens == defaultMaxTokens {
			req.MaxTokens = escalatedMaxTokens
			response, err = e.streamResponse(ctx, req)
			if err != nil {
				e.handler.OnError(err)
				return err
			}
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

		// Record cache observability
		e.cacheTracker.Record(response.Usage.CacheCreate, e.system, len(e.messages))
		e.cacheExpiry.RecordCall()

		// Track analytics
		if e.analytics != nil {
			e.analytics.RecordUsage(response.Usage.InputTokens, response.Usage.OutputTokens)
		}
		// Track compact state
		if e.compactState != nil {
			e.compactState.TotalTokens += response.Usage.InputTokens + response.Usage.OutputTokens
		}

		// Check cost threshold
		if e.costConfirmThresh > 0 && e.analytics != nil {
			currentCost := e.analytics.Cost()
			if currentCost >= e.costConfirmThresh {
				if !e.handler.OnCostConfirmNeeded(currentCost, e.costConfirmThresh) {
					e.handler.OnTextDelta("\n[Session stopped: cost threshold exceeded]\n")
					return nil
				}
				// Double the threshold so we don't ask again immediately
				e.costConfirmThresh *= 2
			}
		}

		// Check if we're done
		if response.StopReason == "end_turn" || (response.StopReason != "max_tokens" && len(response.ToolUses) == 0) {
			// Reset continuation tracking on clean completion
			e.continuationCount = 0
			e.lastOutputTokens = 0

			// Fire Stop hook
			e.fireHook(ctx, hooks.Stop, "", "")

			// Fire turn-end callback for background memory extraction
			if e.onTurnEnd != nil {
				msgsCopy := make([]api.Message, len(e.messages))
				copy(msgsCopy, e.messages)
				go e.onTurnEnd(msgsCopy)
			}

			return nil
		}

		// Handle max_tokens continuation with diminishing returns detection
		if response.StopReason == "max_tokens" && len(response.ToolUses) == 0 {
			e.continuationCount++
			outputTokens := response.Usage.OutputTokens

			// Diminishing returns: output tokens dropped by >50% or max continuations exceeded
			if e.continuationCount >= maxContinuations ||
				(e.lastOutputTokens > 0 && outputTokens < e.lastOutputTokens/2) {
				e.handler.OnTextDelta("\n[Stopped: diminishing returns on continuation]\n")
				e.continuationCount = 0
				e.lastOutputTokens = 0
				return nil
			}
			e.lastOutputTokens = outputTokens

			// Inject continuation prompt
			contContent, _ := json.Marshal("Please continue from where you left off.")
			e.messages = append(e.messages, api.Message{
				Role:    "user",
				Content: contContent,
			})
			continue
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

		// Microcompact: proactively clear large old tool results on every tool turn.
		// Keeps the last 6 results intact, clears anything larger than 2KB beyond that.
		e.messages = compact.MicroCompact(e.messages, 6, 2048)
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
		// Check if a rule explicitly denies this tool call
		if behavior, matched := permissions.Match(tu.Name, tu.Input, e.permissionRules); matched && behavior == "deny" {
			result := &tools.Result{
				Content: fmt.Sprintf("Tool %s was denied by permission rule", tu.Name),
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

		// Track tools discovered via ToolSearch
		if tu.Name == "ToolSearch" {
			e.trackDiscoveredTools(tu.Input)
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

		// Offload large results to disk; truncate anything still over 100KB
		content := result.Content
		if e.toolCache != nil {
			content = e.toolCache.MaybePersist(tu.ID, content)
		}
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
		// Check content-pattern permission rules first (first match wins)
		if behavior, matched := permissions.Match(tool.Name(), input, e.permissionRules); matched {
			switch behavior {
			case "allow":
				return false
			case "deny":
				return true // will show dialog but auto-deny
			case "ask":
				return true
			}
		}
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

// buildSystemWithDeferredTools returns the system prompt with a list of deferred
// tool names appended, so the model knows they exist and can fetch them via ToolSearch.
func (e *Engine) buildSystemWithDeferredTools() string {
	deferred := e.registry.DeferredToolNames()
	if len(deferred) == 0 {
		return e.system
	}

	// Only list tools that haven't been discovered yet
	var pending []string
	for _, name := range deferred {
		if !e.discoveredTools[name] {
			pending = append(pending, name)
		}
	}
	if len(pending) == 0 {
		return e.system
	}

	return e.system + "\n\n<system-reminder>\nThe following deferred tools are available via ToolSearch:\n" +
		strings.Join(pending, "\n") + "\n</system-reminder>"
}

// trackDiscoveredTools parses a ToolSearch input to mark which tools were requested,
// so their full schemas are included in subsequent API requests.
func (e *Engine) trackDiscoveredTools(input json.RawMessage) {
	var params struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return
	}

	if strings.HasPrefix(params.Query, "select:") {
		names := strings.Split(strings.TrimPrefix(params.Query, "select:"), ",")
		for _, name := range names {
			name = strings.TrimSpace(name)
			if name != "" {
				e.discoveredTools[name] = true
			}
		}
	} else {
		// For keyword searches, we mark all matching tools as discovered after execution.
		// The actual matching happens in ToolSearchTool.Execute, but we can pre-compute
		// which tools would match using the same logic.
		query := strings.ToLower(params.Query)
		keywords := strings.Fields(query)
		hints := e.registry.ToolSearchHints()
		for name, hint := range hints {
			nameL := strings.ToLower(name)
			hintL := strings.ToLower(hint)
			for _, kw := range keywords {
				if strings.Contains(nameL, kw) || strings.Contains(hintL, kw) {
					e.discoveredTools[name] = true
					break
				}
			}
		}
	}
}

// mergeConsecutiveUserMessages merges adjacent user messages into one.
// This reduces message count overhead and avoids API issues with consecutive same-role messages.
// Tool result blocks (type "tool_result") are never merged — only plain text and text-type blocks.
func mergeConsecutiveUserMessages(messages []api.Message) []api.Message {
	if len(messages) < 2 {
		return messages
	}
	result := make([]api.Message, 0, len(messages))
	for i := 0; i < len(messages); i++ {
		msg := messages[i]
		if msg.Role != "user" || i+1 >= len(messages) || messages[i+1].Role != "user" {
			result = append(result, msg)
			continue
		}
		// Check that neither message contains tool_result blocks
		if hasToolResultBlocks(msg.Content) || hasToolResultBlocks(messages[i+1].Content) {
			result = append(result, msg)
			continue
		}
		// Merge: concatenate as text blocks
		text1 := extractTextContent(msg.Content)
		text2 := extractTextContent(messages[i+1].Content)
		merged := text1 + "\n" + text2
		mergedContent, _ := json.Marshal(merged)
		result = append(result, api.Message{Role: "user", Content: mergedContent})
		i++ // skip the next message
	}
	return result
}

func hasToolResultBlocks(content json.RawMessage) bool {
	var blocks []json.RawMessage
	if json.Unmarshal(content, &blocks) != nil {
		return false
	}
	for _, b := range blocks {
		var block struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(b, &block) == nil && block.Type == "tool_result" {
			return true
		}
	}
	return false
}

func extractTextContent(content json.RawMessage) string {
	// Try plain string first
	var s string
	if json.Unmarshal(content, &s) == nil {
		return s
	}
	// Try array of blocks
	var blocks []json.RawMessage
	if json.Unmarshal(content, &blocks) != nil {
		return string(content)
	}
	var parts []string
	for _, b := range blocks {
		var block struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(b, &block) == nil && block.Type == "text" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n")
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

func (h *StdoutHandler) OnCostConfirmNeeded(currentCost, threshold float64) bool {
	fmt.Printf("\n[Cost: $%.4f exceeds threshold $%.4f — continuing in non-interactive mode]\n", currentCost, threshold)
	return true
}

func (h *StdoutHandler) OnError(err error) {
	fmt.Printf("\n[error: %v]\n", err)
}
