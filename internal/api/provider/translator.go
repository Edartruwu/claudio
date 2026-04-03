package provider

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Abraxas-365/claudio/internal/api"
)

// --- OpenAI request types ---

type openAIRequest struct {
	Model       string            `json:"model"`
	Messages    []openAIMessage   `json:"messages"`
	Stream      bool              `json:"stream,omitempty"`
	MaxTokens   int               `json:"max_tokens,omitempty"`
	Temperature *float64          `json:"temperature,omitempty"`
	Tools       []openAITool      `json:"tools,omitempty"`
	StreamOpts  *openAIStreamOpts `json:"stream_options,omitempty"`
}

type openAIStreamOpts struct {
	IncludeUsage bool `json:"include_usage"`
}

type openAIMessage struct {
	Role       string               `json:"role"`
	Content    any                  `json:"content,omitempty"`    // string or []openAIContentPart
	ToolCalls  []openAIToolCall     `json:"tool_calls,omitempty"`
	ToolCallID string               `json:"tool_call_id,omitempty"`
	Name       string               `json:"name,omitempty"`
}

type openAIContentPart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type openAITool struct {
	Type     string         `json:"type"` // "function"
	Function openAIFunction `json:"function"`
}

type openAIFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type openAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"` // "function"
	Function openAIFunctionCall `json:"function"`
	Index    int                `json:"index,omitempty"`
}

type openAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// --- OpenAI response types (streaming) ---

type openAIStreamChunk struct {
	ID      string            `json:"id"`
	Choices []openAIChoice    `json:"choices"`
	Usage   *openAIUsage      `json:"usage,omitempty"`
}

type openAIChoice struct {
	Index        int                `json:"index"`
	Delta        openAIDelta        `json:"delta"`
	FinishReason *string            `json:"finish_reason,omitempty"`
}

type openAIDelta struct {
	Role      string           `json:"role,omitempty"`
	Content   *string          `json:"content,omitempty"`
	ToolCalls []openAIToolCall `json:"tool_calls,omitempty"`
}

type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// --- OpenAI response types (non-streaming) ---

type openAIResponse struct {
	ID      string           `json:"id"`
	Choices []openAINSChoice `json:"choices"`
	Usage   openAIUsage      `json:"usage"`
	Model   string           `json:"model"`
}

type openAINSChoice struct {
	Index        int              `json:"index"`
	Message      openAINSMessage  `json:"message"`
	FinishReason string           `json:"finish_reason"`
}

type openAINSMessage struct {
	Role      string           `json:"role"`
	Content   *string          `json:"content,omitempty"`
	ToolCalls []openAIToolCall `json:"tool_calls,omitempty"`
}

// --- Translation functions ---

// translateRequest converts an Anthropic MessagesRequest into an OpenAI ChatCompletion request body.
func translateRequest(req *api.MessagesRequest) ([]byte, error) {
	oaiReq := openAIRequest{
		Model:       req.Model,
		Stream:      req.Stream,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}

	if req.Stream {
		oaiReq.StreamOpts = &openAIStreamOpts{IncludeUsage: true}
	}

	// System prompt -> system message
	var msgs []openAIMessage
	sysText := req.System
	if sysText == "" && len(req.SystemRaw) > 0 {
		// Try to extract text from SystemRaw (could be array of blocks or a string)
		var blocks []api.SystemBlock
		if json.Unmarshal(req.SystemRaw, &blocks) == nil && len(blocks) > 0 {
			var parts []string
			for _, b := range blocks {
				if b.Text != "" {
					parts = append(parts, b.Text)
				}
			}
			sysText = strings.Join(parts, "\n")
		} else {
			// Try as plain string
			json.Unmarshal(req.SystemRaw, &sysText)
		}
	}
	if sysText != "" {
		msgs = append(msgs, openAIMessage{Role: "system", Content: sysText})
	}

	// Convert conversation messages
	for _, msg := range req.Messages {
		oaiMsgs, err := translateMessage(msg)
		if err != nil {
			return nil, fmt.Errorf("translating message: %w", err)
		}
		msgs = append(msgs, oaiMsgs...)
	}
	oaiReq.Messages = msgs

	// Convert tools
	if len(req.Tools) > 0 {
		var anthropicTools []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"input_schema"`
		}
		if err := json.Unmarshal(req.Tools, &anthropicTools); err == nil {
			for _, t := range anthropicTools {
				oaiReq.Tools = append(oaiReq.Tools, openAITool{
					Type: "function",
					Function: openAIFunction{
						Name:        t.Name,
						Description: t.Description,
						Parameters:  t.InputSchema,
					},
				})
			}
		}
	}

	return json.Marshal(oaiReq)
}

// translateMessage converts one Anthropic message into one or more OpenAI messages.
// An assistant message with tool_use blocks becomes a single message with tool_calls,
// and subsequent tool_result blocks become separate tool messages.
func translateMessage(msg api.Message) ([]openAIMessage, error) {
	var blocks []api.ContentBlock
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		// Content might be a plain string
		var text string
		if err2 := json.Unmarshal(msg.Content, &text); err2 == nil {
			return []openAIMessage{{Role: msg.Role, Content: text}}, nil
		}
		return nil, fmt.Errorf("unmarshal content: %w", err)
	}

	if msg.Role == "assistant" {
		return translateAssistantBlocks(blocks)
	}
	return translateUserBlocks(blocks)
}

func translateAssistantBlocks(blocks []api.ContentBlock) ([]openAIMessage, error) {
	var result []openAIMessage
	var textParts []string
	var toolCalls []openAIToolCall
	toolIdx := 0

	for _, block := range blocks {
		switch block.Type {
		case "text":
			textParts = append(textParts, block.Text)
		case "thinking":
			// OpenAI doesn't have thinking blocks — skip or include as text prefix
			continue
		case "tool_use":
			inputStr, _ := json.Marshal(block.Input)
			// If input is already a string, use as-is; otherwise marshal to JSON string
			var inputJSON string
			if len(block.Input) > 0 && block.Input[0] == '"' {
				json.Unmarshal(block.Input, &inputJSON)
			} else {
				inputJSON = string(inputStr)
			}
			toolCalls = append(toolCalls, openAIToolCall{
				ID:    block.ID,
				Type:  "function",
				Index: toolIdx,
				Function: openAIFunctionCall{
					Name:      block.Name,
					Arguments: inputJSON,
				},
			})
			toolIdx++
		}
	}

	// Build the assistant message
	assistMsg := openAIMessage{Role: "assistant"}
	if len(textParts) > 0 {
		combined := strings.Join(textParts, "\n")
		assistMsg.Content = combined
	}
	if len(toolCalls) > 0 {
		assistMsg.ToolCalls = toolCalls
	}
	if len(textParts) > 0 || len(toolCalls) > 0 {
		result = append(result, assistMsg)
	}

	return result, nil
}

func translateUserBlocks(blocks []api.ContentBlock) ([]openAIMessage, error) {
	var result []openAIMessage
	var textParts []string

	for _, block := range blocks {
		switch block.Type {
		case "text":
			textParts = append(textParts, block.Text)
		case "tool_result":
			// Flush accumulated text first
			if len(textParts) > 0 {
				result = append(result, openAIMessage{
					Role:    "user",
					Content: strings.Join(textParts, "\n"),
				})
				textParts = nil
			}
			// tool_result -> tool message
			content := extractToolResultContent(block)
			toolID := block.ID
			if toolID == "" {
				toolID = block.ToolUseID
			}
			result = append(result, openAIMessage{
				Role:       "tool",
				ToolCallID: toolID,
				Content:    content,
			})
		case "image":
			// Skip image blocks for now (not supported in initial translation)
			continue
		}
	}

	if len(textParts) > 0 {
		result = append(result, openAIMessage{
			Role:    "user",
			Content: strings.Join(textParts, "\n"),
		})
	}

	// If no messages were produced, return a single empty user message
	if len(result) == 0 {
		result = append(result, openAIMessage{Role: "user", Content: ""})
	}

	return result, nil
}

// extractToolResultContent gets the text content from a tool_result content block.
func extractToolResultContent(block api.ContentBlock) string {
	if block.Text != "" {
		return block.Text
	}
	if block.Content != "" {
		return block.Content
	}
	// Some tool results store content in Input as an array of content blocks
	if len(block.Input) > 0 {
		var inner []api.ContentBlock
		if json.Unmarshal(block.Input, &inner) == nil {
			var parts []string
			for _, b := range inner {
				if b.Text != "" {
					parts = append(parts, b.Text)
				}
			}
			return strings.Join(parts, "\n")
		}
		return string(block.Input)
	}
	return ""
}

// translateStreamChunk converts an OpenAI streaming chunk into Anthropic StreamEvents.
func translateStreamChunk(chunk openAIStreamChunk, state *streamState) []api.StreamEvent {
	var events []api.StreamEvent

	for _, choice := range chunk.Choices {
		// Text content delta
		if choice.Delta.Content != nil && *choice.Delta.Content != "" {
			if !state.textStarted {
				// Emit content_block_start for text
				blockJSON, _ := json.Marshal(api.ContentBlock{Type: "text"})
				events = append(events, api.StreamEvent{
					Type:         "content_block_start",
					Index:        state.blockIndex,
					ContentBlock: blockJSON,
				})
				state.textStarted = true
				state.currentBlockIndex = state.blockIndex
				state.blockIndex++
			}
			deltaJSON, _ := json.Marshal(map[string]string{
				"type": "text_delta",
				"text": *choice.Delta.Content,
			})
			events = append(events, api.StreamEvent{
				Type:  "content_block_delta",
				Index: state.currentBlockIndex,
				Delta: deltaJSON,
			})
		}

		// Tool call deltas
		for _, tc := range choice.Delta.ToolCalls {
			tcID := tc.ID
			fnName := tc.Function.Name
			fnArgs := tc.Function.Arguments

			// New tool call (has ID and name, and we haven't seen this index before).
			// Some providers (e.g. Qwen) re-send ID and name in every chunk,
			// so we must check state.toolCalls to avoid emitting duplicates.
			_, alreadySeen := state.toolCalls[tc.Index]
			if tcID != "" && fnName != "" && !alreadySeen {
				// Close previous text block if open
				if state.textStarted {
					events = append(events, api.StreamEvent{
						Type:  "content_block_stop",
						Index: state.currentBlockIndex,
					})
					state.textStarted = false
				}

				state.toolCalls[tc.Index] = &toolCallState{
					id:         tcID,
					name:       fnName,
					argsBuffer: fnArgs,
					blockIndex: state.blockIndex,
				}

				// Emit content_block_start for tool_use
				blockJSON, _ := json.Marshal(api.ContentBlock{
					Type: "tool_use",
					ID:   tcID,
					Name: fnName,
				})
				events = append(events, api.StreamEvent{
					Type:         "content_block_start",
					Index:        state.blockIndex,
					ContentBlock: blockJSON,
				})

				// Some providers (e.g. Qwen) send arguments in the same
				// chunk as the tool call ID/name.  Emit them now so the
				// engine can accumulate the input JSON.
				if fnArgs != "" {
					deltaJSON, _ := json.Marshal(map[string]string{
						"type":         "input_json_delta",
						"partial_json": fnArgs,
					})
					events = append(events, api.StreamEvent{
						Type:  "content_block_delta",
						Index: state.blockIndex,
						Delta: deltaJSON,
					})
				}

				state.currentBlockIndex = state.blockIndex
				state.blockIndex++
			} else if fnArgs != "" {
				// Argument chunk for an existing tool call
				tcs, ok := state.toolCalls[tc.Index]
				if ok {
					tcs.argsBuffer += fnArgs
					deltaJSON, _ := json.Marshal(map[string]string{
						"type":          "input_json_delta",
						"partial_json": fnArgs,
					})
					events = append(events, api.StreamEvent{
						Type:  "content_block_delta",
						Index: tcs.blockIndex,
						Delta: deltaJSON,
					})
				}
			}
		}

		// Finish reason
		if choice.FinishReason != nil {
			// Close any open block
			if state.textStarted {
				events = append(events, api.StreamEvent{
					Type:  "content_block_stop",
					Index: state.currentBlockIndex,
				})
				state.textStarted = false
			}
			// Close any open tool blocks
			for _, tcs := range state.toolCalls {
				events = append(events, api.StreamEvent{
					Type:  "content_block_stop",
					Index: tcs.blockIndex,
				})
			}

			stopReason := translateFinishReason(*choice.FinishReason)
			deltaJSON, _ := json.Marshal(map[string]string{
				"type":        "message_delta",
				"stop_reason": stopReason,
			})
			events = append(events, api.StreamEvent{
				Type:  "message_delta",
				Delta: deltaJSON,
			})
		}
	}

	// Usage info (comes with stream_options.include_usage)
	if chunk.Usage != nil {
		usage := &api.Usage{
			InputTokens:  chunk.Usage.PromptTokens,
			OutputTokens: chunk.Usage.CompletionTokens,
		}
		deltaJSON, _ := json.Marshal(map[string]string{
			"type":        "message_delta",
			"stop_reason": "",
		})
		events = append(events, api.StreamEvent{
			Type:  "message_delta",
			Delta: deltaJSON,
			Usage: usage,
		})
	}

	return events
}

// translateNonStreamingResponse converts an OpenAI response into an Anthropic MessageResp.
func translateNonStreamingResponse(resp openAIResponse) *api.MessageResp {
	var content []api.ContentBlock

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]

		if choice.Message.Content != nil && *choice.Message.Content != "" {
			content = append(content, api.ContentBlock{
				Type: "text",
				Text: *choice.Message.Content,
			})
		}

		for _, tc := range choice.Message.ToolCalls {
			content = append(content, api.ContentBlock{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Input: json.RawMessage(tc.Function.Arguments),
			})
		}
	}

	stopReason := "end_turn"
	if len(resp.Choices) > 0 {
		stopReason = translateFinishReason(resp.Choices[0].FinishReason)
	}

	return &api.MessageResp{
		ID:         resp.ID,
		Type:       "message",
		Role:       "assistant",
		Content:    content,
		Model:      resp.Model,
		StopReason: stopReason,
		Usage: api.Usage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
		},
	}
}

func translateFinishReason(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "tool_calls":
		return "tool_use"
	case "length":
		return "max_tokens"
	default:
		return "end_turn"
	}
}

// streamState tracks state across streaming chunks for proper event translation.
type streamState struct {
	blockIndex        int
	currentBlockIndex int
	textStarted       bool
	toolCalls         map[int]*toolCallState
}

type toolCallState struct {
	id         string
	name       string
	argsBuffer string
	blockIndex int
}

func newStreamState() *streamState {
	return &streamState{
		toolCalls: make(map[int]*toolCallState),
	}
}
