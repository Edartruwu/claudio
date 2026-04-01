package query

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Abraxas-365/claudio/internal/api"
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
}

// Engine orchestrates the AI conversation loop.
type Engine struct {
	client   *api.Client
	registry *tools.Registry
	handler  EventHandler
	messages []api.Message
	system   string
}

// NewEngine creates a new query engine.
func NewEngine(client *api.Client, registry *tools.Registry, handler EventHandler) *Engine {
	return &Engine{
		client:   client,
		registry: registry,
		handler:  handler,
	}
}

// SetSystem sets the system prompt.
func (e *Engine) SetSystem(prompt string) {
	e.system = prompt
}

// Run executes a single user turn: sends the message, processes the AI response,
// executes any tool calls, and loops until the AI produces a final response.
func (e *Engine) Run(ctx context.Context, userMessage string) error {
	// Append user message
	content, _ := json.Marshal(userMessage)
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

		// Append assistant message
		assistantContent, _ := json.Marshal(response.Content)
		e.messages = append(e.messages, api.Message{
			Role:    "assistant",
			Content: assistantContent,
		})

		e.handler.OnTurnComplete(response.Usage)

		// Check if we're done
		if response.StopReason == "end_turn" || len(response.ToolUses) == 0 {
			return nil
		}

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
				// Parse initial message data
				var msg struct {
					Message api.MessageResp `json:"message"`
				}
				// message_start sends full event as {"type":"message_start","message":{...}}
				// The outer event was already parsed, check if there's a message field
				if event.Delta != nil {
					json.Unmarshal(event.Delta, &msg)
				}

			case "content_block_start":
				// New content block
				var blockStart struct {
					Index        int              `json:"index"`
					ContentBlock api.ContentBlock `json:"content_block"`
				}
				if event.Delta != nil {
					json.Unmarshal(event.Delta, &blockStart)
				}
				currentBlockIdx = blockStart.Index
				for len(currentBlocks) <= currentBlockIdx {
					currentBlocks = append(currentBlocks, api.ContentBlock{})
				}
				currentBlocks[currentBlockIdx] = blockStart.ContentBlock

				if blockStart.ContentBlock.Type == "tool_use" {
					tu := tools.ToolUse{
						ID:   blockStart.ContentBlock.ID,
						Name: blockStart.ContentBlock.Name,
					}
					e.handler.OnToolUseStart(tu)
				}

			case "content_block_delta":
				if event.Delta == nil {
					continue
				}
				var delta struct {
					Type         string `json:"type"`
					Text         string `json:"text"`
					PartialJSON  string `json:"partial_json"`
					Thinking     string `json:"thinking"`
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
						StopReason string    `json:"stop_reason"`
						Usage      api.Usage `json:"usage"`
					}
					json.Unmarshal(event.Delta, &delta)
					if delta.StopReason != "" {
						response.StopReason = delta.StopReason
					}
					if delta.Usage.OutputTokens > 0 {
						response.Usage.OutputTokens = delta.Usage.OutputTokens
					}
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

		// Check approval (for now, auto-approve in non-interactive mode)
		// TODO: In TUI mode, send approval request and wait for user response
		_ = tool.RequiresApproval(tu.Input)

		result, err := tool.Execute(ctx, tu.Input)
		if err != nil {
			result = &tools.Result{
				Content: fmt.Sprintf("Tool execution error: %v", err),
				IsError: true,
			}
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

func (h *StdoutHandler) OnError(err error) {
	fmt.Printf("\n[error: %v]\n", err)
}
