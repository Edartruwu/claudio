package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/Abraxas-365/claudio/internal/api"
)

// Ollama implements the Provider interface for Ollama's native /api/chat endpoint.
//
// We can't use the OpenAI-compatible /v1/chat/completions endpoint for Ollama
// because it silently drops the `options` field — meaning num_ctx can't be set,
// so Ollama defaults to 2048 tokens regardless of the model and silently truncates
// conversation history. The native /api/chat endpoint accepts options properly.
type Ollama struct {
	name    string
	baseURL string // e.g. "http://localhost:11434" — without /v1
	numCtx  int    // num_ctx (context window). 0 leaves Ollama default (2048).
	// noToolsModels lists model name patterns (filepath.Match syntax) that don't
	// support tool calling. When matched, tools are stripped from the request.
	// e.g. ["deepseek-r1*", "gemma*"]
	noToolsModels []string
}

// NewOllama creates a new native Ollama provider. The baseURL should NOT include
// "/v1" — e.g. "http://localhost:11434" (we hit /api/chat directly).
func NewOllama(name, baseURL string) *Ollama {
	// Strip /v1 suffix if user pasted the OpenAI-compat URL
	baseURL = strings.TrimRight(baseURL, "/")
	baseURL = strings.TrimSuffix(baseURL, "/v1")
	return &Ollama{
		name:    name,
		baseURL: baseURL,
	}
}

// WithNumCtx sets the num_ctx context window passed in every request.
func (o *Ollama) WithNumCtx(n int) *Ollama {
	o.numCtx = n
	return o
}

// WithNoToolsModels sets the list of model patterns that don't support tools.
func (o *Ollama) WithNoToolsModels(patterns []string) *Ollama {
	o.noToolsModels = patterns
	return o
}

func (o *Ollama) Name() string { return o.name }

// supportsTools reports whether tools should be sent for the given model.
func (o *Ollama) supportsTools(model string) bool {
	for _, pattern := range o.noToolsModels {
		if matched, _ := filepath.Match(pattern, model); matched {
			return false
		}
	}
	return true
}

// ollamaRequest is the wire format for Ollama's /api/chat endpoint.
type ollamaRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Tools    []openAITool    `json:"tools,omitempty"`
	Options  map[string]any  `json:"options,omitempty"`
}

// ollamaStreamChunk is one line of the NDJSON stream returned by /api/chat.
type ollamaStreamChunk struct {
	Model     string         `json:"model"`
	CreatedAt string         `json:"created_at"`
	Message   ollamaMessage  `json:"message"`
	Done      bool           `json:"done"`
	DoneReason string        `json:"done_reason,omitempty"`
	// Token counts (only present in the final chunk)
	PromptEvalCount int `json:"prompt_eval_count,omitempty"`
	EvalCount       int `json:"eval_count,omitempty"`
}

type ollamaMessage struct {
	Role      string             `json:"role"`
	Content   string             `json:"content"`
	ToolCalls []ollamaToolCall   `json:"tool_calls,omitempty"`
	Thinking  string             `json:"thinking,omitempty"`
}

type ollamaToolCall struct {
	Function ollamaFunctionCall `json:"function"`
}

type ollamaFunctionCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (o *Ollama) buildRequestBody(req *api.MessagesRequest) ([]byte, error) {
	oaiReq, err := buildOpenAIRequest(req)
	if err != nil {
		return nil, err
	}

	body := ollamaRequest{
		Model:    oaiReq.Model,
		Messages: oaiReq.Messages,
		Stream:   oaiReq.Stream,
	}

	// Only include tools if the model supports them
	if o.supportsTools(req.Model) {
		body.Tools = oaiReq.Tools
	}

	// Build options. num_ctx is the critical one — without it Ollama defaults
	// to 2048 tokens which silently truncates the conversation.
	opts := map[string]any{}
	if o.numCtx > 0 {
		opts["num_ctx"] = o.numCtx
	}
	if req.MaxTokens > 0 {
		opts["num_predict"] = req.MaxTokens
	}
	if req.Temperature != nil {
		opts["temperature"] = *req.Temperature
	}
	if len(opts) > 0 {
		body.Options = opts
	}

	return json.Marshal(body)
}

func (o *Ollama) StreamMessages(ctx context.Context, httpClient *http.Client, req *api.MessagesRequest) (<-chan api.StreamEvent, <-chan error) {
	eventCh := make(chan api.StreamEvent, 64)
	errCh := make(chan error, 1)

	go func() {
		defer close(eventCh)
		defer close(errCh)

		req.Stream = true
		body, err := o.buildRequestBody(req)
		if err != nil {
			errCh <- fmt.Errorf("failed to build request: %w", err)
			return
		}

		httpReq, err := http.NewRequestWithContext(ctx, "POST",
			o.baseURL+"/api/chat", bytes.NewReader(body))
		if err != nil {
			errCh <- err
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := httpClient.Do(httpReq)
		if err != nil {
			errCh <- fmt.Errorf("API request failed: %w", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			errCh <- fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
			return
		}

		// Synthetic message_start so the engine begins assembling the response.
		// We don't know input_tokens yet — they arrive in the final chunk as
		// prompt_eval_count, so we patch the message_start at the end.
		msgStartJSON, _ := json.Marshal(api.MessageResp{
			ID:    "msg_ollama",
			Type:  "message",
			Role:  "assistant",
			Model: req.Model,
		})
		select {
		case eventCh <- api.StreamEvent{Type: "message_start", MessageField: msgStartJSON}:
		case <-ctx.Done():
			return
		}

		state := newOllamaStreamState()
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}

			var chunk ollamaStreamChunk
			if err := json.Unmarshal(line, &chunk); err != nil {
				continue
			}

			events := translateOllamaChunk(chunk, state)
			for _, event := range events {
				select {
				case eventCh <- event:
				case <-ctx.Done():
					return
				}
			}

			if chunk.Done {
				stopReason := "end_turn"
				if state.toolCallsEmitted {
					stopReason = "tool_use"
				}
				deltaPayload := map[string]any{
					"stop_reason": stopReason,
				}
				deltaJSON, _ := json.Marshal(deltaPayload)
				select {
				case eventCh <- api.StreamEvent{
					Type:  "message_delta",
					Delta: deltaJSON,
					Usage: &api.Usage{
						InputTokens:  chunk.PromptEvalCount,
						OutputTokens: chunk.EvalCount,
					},
				}:
				case <-ctx.Done():
					return
				}
				select {
				case eventCh <- api.StreamEvent{Type: "message_stop"}:
				case <-ctx.Done():
					return
				}
				break
			}
		}

		if err := scanner.Err(); err != nil {
			errCh <- fmt.Errorf("stream read error: %w", err)
		}
	}()

	return eventCh, errCh
}

func (o *Ollama) SendMessage(ctx context.Context, httpClient *http.Client, req *api.MessagesRequest) (*api.MessageResp, error) {
	req.Stream = false
	body, err := o.buildRequestBody(req)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		o.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var chunk ollamaStreamChunk
	if err := json.NewDecoder(resp.Body).Decode(&chunk); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// Build content blocks
	var blocks []api.ContentBlock
	if chunk.Message.Content != "" {
		blocks = append(blocks, api.ContentBlock{Type: "text", Text: chunk.Message.Content})
	}
	for i, tc := range chunk.Message.ToolCalls {
		blocks = append(blocks, api.ContentBlock{
			Type:  "tool_use",
			ID:    fmt.Sprintf("call_%d", i),
			Name:  tc.Function.Name,
			Input: tc.Function.Arguments,
		})
	}

	stopReason := "end_turn"
	if len(chunk.Message.ToolCalls) > 0 {
		stopReason = "tool_use"
	}

	return &api.MessageResp{
		ID:         "msg_ollama",
		Type:       "message",
		Role:       "assistant",
		Model:      req.Model,
		Content:    blocks,
		StopReason: stopReason,
		Usage: api.Usage{
			InputTokens:  chunk.PromptEvalCount,
			OutputTokens: chunk.EvalCount,
		},
	}, nil
}

// --- Streaming state and chunk translation ---

type ollamaStreamState struct {
	textStarted        bool
	textBlockIndex     int
	nextBlockIndex     int
	toolCallsEmitted   bool
	toolCallCounter    int
}

func newOllamaStreamState() *ollamaStreamState {
	return &ollamaStreamState{}
}

// translateOllamaChunk converts one /api/chat NDJSON chunk into Anthropic-style stream events.
func translateOllamaChunk(chunk ollamaStreamChunk, state *ollamaStreamState) []api.StreamEvent {
	var events []api.StreamEvent

	// Text delta
	if chunk.Message.Content != "" {
		if !state.textStarted {
			blockJSON, _ := json.Marshal(api.ContentBlock{Type: "text", Text: ""})
			events = append(events, api.StreamEvent{
				Type:         "content_block_start",
				Index:        state.nextBlockIndex,
				ContentBlock: blockJSON,
			})
			state.textBlockIndex = state.nextBlockIndex
			state.nextBlockIndex++
			state.textStarted = true
		}
		deltaJSON, _ := json.Marshal(map[string]string{
			"type": "text_delta",
			"text": chunk.Message.Content,
		})
		events = append(events, api.StreamEvent{
			Type:  "content_block_delta",
			Index: state.textBlockIndex,
			Delta: deltaJSON,
		})
	}

	// Tool calls — Ollama returns each as a complete object (no incremental streaming).
	// We close any open text block, then emit a full tool_use block per call.
	for _, tc := range chunk.Message.ToolCalls {
		if state.textStarted {
			events = append(events, api.StreamEvent{
				Type:  "content_block_stop",
				Index: state.textBlockIndex,
			})
			state.textStarted = false
		}

		callID := fmt.Sprintf("call_%d", state.toolCallCounter)
		state.toolCallCounter++
		state.toolCallsEmitted = true

		blockJSON, _ := json.Marshal(api.ContentBlock{
			Type:  "tool_use",
			ID:    callID,
			Name:  tc.Function.Name,
			Input: json.RawMessage("{}"),
		})
		events = append(events, api.StreamEvent{
			Type:         "content_block_start",
			Index:        state.nextBlockIndex,
			ContentBlock: blockJSON,
		})

		// Emit the args as a single input_json_delta
		argsStr := string(tc.Function.Arguments)
		if argsStr == "" {
			argsStr = "{}"
		}
		deltaJSON, _ := json.Marshal(map[string]string{
			"type":         "input_json_delta",
			"partial_json": argsStr,
		})
		events = append(events, api.StreamEvent{
			Type:  "content_block_delta",
			Index: state.nextBlockIndex,
			Delta: deltaJSON,
		})

		events = append(events, api.StreamEvent{
			Type:  "content_block_stop",
			Index: state.nextBlockIndex,
		})
		state.nextBlockIndex++
	}

	// Close text block when stream ends
	if chunk.Done && state.textStarted {
		events = append(events, api.StreamEvent{
			Type:  "content_block_stop",
			Index: state.textBlockIndex,
		})
		state.textStarted = false
	}

	return events
}
