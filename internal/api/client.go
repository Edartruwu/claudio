package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Abraxas-365/claudio/internal/auth"
)

const (
	defaultBaseURL    = "https://api.anthropic.com"
	defaultAPIVersion = "2023-06-01"
	userAgent         = "Claudio/0.1.0"
)

// Client communicates with the Anthropic Messages API.
type Client struct {
	httpClient   *http.Client
	authResolver *auth.Resolver
	baseURL      string
	apiVersion   string
	model        string

	// Thinking configuration
	thinkingMode string // "adaptive", "enabled", "disabled", "" (auto-detect)
	budgetTokens int    // Used when thinkingMode is "enabled"

	// Effort level (controls reasoning depth, separate from thinking)
	effortLevel string // "low", "medium", "high", "" (default/high)

	// Prompt caching: inject cache_control on the last system block.
	promptCaching bool
}

// NewClient creates a new API client.
func NewClient(resolver *auth.Resolver, opts ...ClientOption) *Client {
	c := &Client{
		// No timeout on the HTTP client — streaming responses (especially with
		// extended thinking on Opus) can run for 30+ minutes. Cancellation is
		// handled via the request context (ctx), which the TUI passes and can
		// cancel at any time with Ctrl+C.
		httpClient: &http.Client{},
		authResolver:  resolver,
		baseURL:       defaultBaseURL,
		apiVersion:    defaultAPIVersion,
		model:         "claude-sonnet-4-6",
		promptCaching: true, // enabled by default
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ClientOption configures the API client.
type ClientOption func(*Client)

// WithBaseURL sets a custom API base URL.
func WithBaseURL(url string) ClientOption {
	return func(c *Client) { c.baseURL = strings.TrimRight(url, "/") }
}

// WithModel sets the default model.
func WithModel(model string) ClientOption {
	return func(c *Client) { c.model = model }
}

// WithPromptCaching enables or disables prompt caching (default: true).
func WithPromptCaching(enabled bool) ClientOption {
	return func(c *Client) { c.promptCaching = enabled }
}

// NewClientFromExisting creates a shallow copy of an existing client with a different model.
// All other settings (auth, base URL, thinking mode, etc.) are inherited from the parent.
func NewClientFromExisting(parent *Client, model string) *Client {
	copy := *parent
	copy.model = model
	// Sub-agents don't need extended thinking — use disabled for speed.
	copy.thinkingMode = "disabled"
	return &copy
}

// GetModel returns the current default model.
func (c *Client) GetModel() string {
	return c.model
}

// SetModel changes the default model at runtime.
func (c *Client) SetModel(model string) {
	c.model = model
}

// GetThinkingMode returns the current thinking mode ("adaptive", "enabled", "disabled", or "" for auto).
func (c *Client) GetThinkingMode() string {
	return c.thinkingMode
}

// SetThinkingMode sets the thinking mode. Use "" for auto-detect based on model.
func (c *Client) SetThinkingMode(mode string) {
	c.thinkingMode = mode
}

// GetBudgetTokens returns the thinking budget token limit.
func (c *Client) GetBudgetTokens() int {
	return c.budgetTokens
}

// SetBudgetTokens sets the thinking budget token limit (used when mode is "enabled").
func (c *Client) SetBudgetTokens(tokens int) {
	c.budgetTokens = tokens
}

// ThinkingLabel returns a human-readable label for the current thinking configuration.
func (c *Client) ThinkingLabel() string {
	switch c.thinkingMode {
	case "disabled":
		return "Disabled"
	case "enabled":
		if c.budgetTokens > 0 {
			return fmt.Sprintf("Enabled (%dk tokens)", c.budgetTokens/1000)
		}
		return "Enabled"
	case "adaptive":
		return "Adaptive"
	default:
		return "Auto (adaptive)"
	}
}

// GetEffortLevel returns the current effort level ("low", "medium", "high", or "" for default).
func (c *Client) GetEffortLevel() string {
	return c.effortLevel
}

// SetEffortLevel sets the reasoning effort level. Use "" for default (high).
func (c *Client) SetEffortLevel(level string) {
	c.effortLevel = level
}

// EffortLabel returns a human-readable label for the current effort level.
func (c *Client) EffortLabel() string {
	switch c.effortLevel {
	case "low":
		return "Low effort"
	case "high":
		return "High effort"
	case "medium", "":
		return "Medium effort (default)"
	default:
		return "Medium effort (default)"
	}
}

// modelSupportsEffort returns true if the model supports the effort parameter.
func modelSupportsEffort(model string) bool {
	m := strings.ToLower(model)
	return strings.Contains(m, "opus-4-6") || strings.Contains(m, "sonnet-4-6") || strings.Contains(m, "opus-4") || strings.Contains(m, "sonnet-4")
}

// ThinkingConfig configures extended thinking for supported models.
type ThinkingConfig struct {
	Type         string `json:"type"`                    // "adaptive", "enabled", "disabled"
	BudgetTokens int    `json:"budget_tokens,omitempty"` // Required when type is "enabled"
}

// OutputConfig configures output behavior including effort level.
type OutputConfig struct {
	Effort string `json:"effort,omitempty"` // "low", "medium", "high"
}

// CacheControlBlock instructs the API to cache content up to this point.
type CacheControlBlock struct {
	Type string `json:"type"` // always "ephemeral"
}

// SystemBlock is a text block in the system prompt array.
type SystemBlock struct {
	Type         string             `json:"type"`
	Text         string             `json:"text"`
	CacheControl *CacheControlBlock `json:"cache_control,omitempty"`
}

// MessagesRequest is the request body for /v1/messages.
type MessagesRequest struct {
	Model       string          `json:"model"`
	MaxTokens   int             `json:"max_tokens"`
	Messages    []Message       `json:"messages"`
	System      string          `json:"-"`               // Set by callers as plain string, marshaled by custom logic
	SystemRaw   json.RawMessage `json:"system,omitempty"` // Actual JSON sent to API (string or array of blocks)
	Stream      bool            `json:"stream,omitempty"`
	Tools       json.RawMessage `json:"tools,omitempty"`
	Temperature *float64        `json:"temperature,omitempty"`
	StopReason   string          `json:"stop_reason,omitempty"`
	Thinking     *ThinkingConfig `json:"thinking,omitempty"`
	OutputConfig *OutputConfig   `json:"output_config,omitempty"`
}

// Message represents a conversation message.
type Message struct {
	Role    string          `json:"role"` // "user", "assistant"
	Content json.RawMessage `json:"content"`
}

// StreamEvent represents a single SSE event from the streaming API.
type StreamEvent struct {
	Type         string          `json:"type"`
	Delta        json.RawMessage `json:"delta,omitempty"`
	Index        int             `json:"index,omitempty"`
	MessageField json.RawMessage `json:"message,omitempty"`       // For message_start
	ContentBlock json.RawMessage `json:"content_block,omitempty"` // For content_block_start
	Usage        *Usage          `json:"usage,omitempty"`          // For message_delta
}

// ContentBlock represents a content block in the response.
type ContentBlock struct {
	Type      string          `json:"type"` // "text", "tool_use", "thinking"
	Text      string          `json:"text,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	Signature string          `json:"signature,omitempty"` // Required when sending thinking blocks back
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
}

// ImageSource describes the source of an image for the API.
type ImageSource struct {
	Type      string `json:"type"`       // "base64"
	MediaType string `json:"media_type"` // "image/png", "image/jpeg", "image/gif", "image/webp"
	Data      string `json:"data"`       // base64-encoded image data
}

// UserContentBlock is a content block in a user message (text or image).
type UserContentBlock struct {
	Type   string       `json:"type"`             // "text" or "image"
	Text   string       `json:"text,omitempty"`   // for type="text"
	Source *ImageSource `json:"source,omitempty"` // for type="image"
}

// NewTextBlock creates a text content block for a user message.
func NewTextBlock(text string) UserContentBlock {
	return UserContentBlock{Type: "text", Text: text}
}

// NewImageBlock creates an image content block for a user message.
func NewImageBlock(mediaType, base64Data string) UserContentBlock {
	return UserContentBlock{
		Type: "image",
		Source: &ImageSource{
			Type:      "base64",
			MediaType: mediaType,
			Data:      base64Data,
		},
	}
}

// Usage tracks token consumption.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	CacheRead    int `json:"cache_read_input_tokens,omitempty"`
	CacheCreate  int `json:"cache_creation_input_tokens,omitempty"`
}

// MessageResp is the full message response.
type MessageResp struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Role       string         `json:"role"`
	Content    []ContentBlock `json:"content"`
	Model      string         `json:"model"`
	StopReason string         `json:"stop_reason"`
	Usage      Usage          `json:"usage"`
}

// StreamMessages sends a streaming messages request and returns a channel of events.
// When using OAuth, it proxies through the official claude CLI to bypass third-party restrictions.
func (c *Client) StreamMessages(ctx context.Context, req *MessagesRequest) (<-chan StreamEvent, <-chan error) {
	if req.Model == "" {
		req.Model = c.model
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = 8192
	}

	req.Stream = true
	c.applyThinking(req)
	c.applyEffort(req)
	c.applyAttribution(req)
	c.applyMessageCacheBreakpoints(req)

	eventCh := make(chan StreamEvent, 64)
	errCh := make(chan error, 1)
	go func() {
		defer close(eventCh)
		defer close(errCh)

		body, err := json.Marshal(req)
		if err != nil {
			errCh <- fmt.Errorf("failed to marshal request: %w", err)
			return
		}

		httpReq, err := http.NewRequestWithContext(ctx, "POST",
			c.baseURL+"/v1/messages?beta=true", bytes.NewReader(body))
		if err != nil {
			errCh <- err
			return
		}

		c.setHeaders(httpReq)

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			errCh <- fmt.Errorf("API request failed: %w", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			retryAfter := resp.Header.Get("retry-after")
			extra := ""
			if retryAfter != "" {
				extra = fmt.Sprintf(" (retry after %ss)", retryAfter)
			}
			errCh <- fmt.Errorf("API error (HTTP %d): %s%s", resp.StatusCode, string(bodyBytes), extra)
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer

		for scanner.Scan() {
			line := scanner.Text()

			if strings.HasPrefix(line, "data: ") {
				data := line[6:]
				if data == "[DONE]" {
					return
				}

				var event StreamEvent
				if err := json.Unmarshal([]byte(data), &event); err != nil {
					continue // Skip malformed events
				}

				select {
				case eventCh <- event:
				case <-ctx.Done():
					return
				}
			}
		}

		if err := scanner.Err(); err != nil {
			errCh <- fmt.Errorf("stream read error: %w", err)
		}
	}()

	return eventCh, errCh
}

// SendMessage sends a non-streaming messages request.
func (c *Client) SendMessage(ctx context.Context, req *MessagesRequest) (*MessageResp, error) {
	req.Stream = false
	if req.Model == "" {
		req.Model = c.model
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = 8192
	}
	c.applyThinking(req)
	c.applyEffort(req)
	c.applyAttribution(req)
	c.applyMessageCacheBreakpoints(req)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/v1/messages?beta=true", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	c.setHeaders(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var msgResp MessageResp
	if err := json.NewDecoder(resp.Body).Decode(&msgResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &msgResp, nil
}

// applyMessageCacheBreakpoints adds cache_control to the last content block of
// the second-to-last message, creating a cache breakpoint that covers all stable
// conversation history. This avoids re-paying for the full history on every turn.
// It also marks the last tool definition with cache_control to cache the tool schema.
func (c *Client) applyMessageCacheBreakpoints(req *MessagesRequest) {
	if !c.promptCaching {
		return
	}

	// Cache tool definitions — mark the last non-deferred tool with a breakpoint.
	// Deferred tools (defer_loading=true) cannot have cache_control set.
	if len(req.Tools) > 0 {
		var toolDefs []json.RawMessage
		if json.Unmarshal(req.Tools, &toolDefs) == nil {
			for i := len(toolDefs) - 1; i >= 0; i-- {
				var tool map[string]json.RawMessage
				if json.Unmarshal(toolDefs[i], &tool) != nil {
					continue
				}
				var deferred bool
				if v, ok := tool["defer_loading"]; ok {
					json.Unmarshal(v, &deferred)
				}
				if deferred {
					continue
				}
				tool["cache_control"], _ = json.Marshal(CacheControlBlock{Type: "ephemeral"})
				toolDefs[i], _ = json.Marshal(tool)
				req.Tools, _ = json.Marshal(toolDefs)
				break
			}
		}
	}

	// Cache messages — the Anthropic API allows up to 4 cache_control breakpoints
	// total (system + tools already use 2). We use the remaining 2 slots on the
	// message history:
	//   • For short histories (< 10 msgs): mark only the second-to-last message.
	//   • For long histories (≥ 10 msgs): mark a midpoint AND the second-to-last,
	//     so the stable early history is cached independently from the recent tail.
	// The last message is always the current in-flight user turn — never marked.
	if len(req.Messages) < 2 {
		return
	}

	markLastBlock := func(idx int) {
		msg := req.Messages[idx]
		var blocks []json.RawMessage
		if json.Unmarshal(msg.Content, &blocks) != nil || len(blocks) == 0 {
			return
		}
		lastBlockIdx := len(blocks) - 1
		var block map[string]json.RawMessage
		if json.Unmarshal(blocks[lastBlockIdx], &block) != nil {
			return
		}
		block["cache_control"], _ = json.Marshal(CacheControlBlock{Type: "ephemeral"})
		blocks[lastBlockIdx], _ = json.Marshal(block)
		newContent, _ := json.Marshal(blocks)
		req.Messages[idx] = Message{Role: msg.Role, Content: newContent}
	}

	// For long histories place an extra breakpoint at roughly the 1/3 mark so
	// the bulk of the stable history gets its own cache slot.
	if len(req.Messages) >= 10 {
		midIdx := len(req.Messages) / 3
		if midIdx > 0 {
			markLastBlock(midIdx)
		}
	}

	markLastBlock(len(req.Messages) - 2)
}

// applyAttribution injects the x-anthropic-billing-header into the system prompt
// as the first text block. This is required for OAuth tokens on Sonnet/Opus models.
// It converts the plain string System field into a SystemRaw JSON array.
// When prompt caching is enabled, it marks the last block with cache_control so the
// API caches everything up to that point (saves re-paying for static system content).
func (c *Client) applyAttribution(req *MessagesRequest) {
	authResult := c.authResolver.Resolve()
	if !authResult.IsOAuth {
		// For non-OAuth, emit a block array so we can attach cache_control.
		if req.System != "" {
			block := SystemBlock{Type: "text", Text: req.System}
			if c.promptCaching {
				block.CacheControl = &CacheControlBlock{Type: "ephemeral"}
			}
			req.SystemRaw, _ = json.Marshal([]SystemBlock{block})
		}
		return
	}

	blocks := []SystemBlock{
		{Type: "text", Text: "x-anthropic-billing-header: cc_version=2.1.89.4fa; cc_entrypoint=cli; cch=00000;"},
	}
	if req.System != "" {
		blocks = append(blocks, SystemBlock{Type: "text", Text: req.System})
	}
	// Mark the last block for caching — the API will cache everything up to this point.
	if c.promptCaching && len(blocks) > 0 {
		blocks[len(blocks)-1].CacheControl = &CacheControlBlock{Type: "ephemeral"}
	}
	req.SystemRaw, _ = json.Marshal(blocks)
}

// applyThinking configures extended thinking based on client settings and model capability.
func (c *Client) applyThinking(req *MessagesRequest) {
	if req.Thinking != nil {
		return // Already set explicitly on the request
	}

	model := req.Model
	supportsThinking := strings.Contains(model, "sonnet-4") || strings.Contains(model, "opus-4")

	switch c.thinkingMode {
	case "disabled":
		// Explicitly disabled — don't set thinking at all
		return
	case "enabled":
		if supportsThinking {
			budget := c.budgetTokens
			if budget <= 0 {
				budget = 10000 // default budget when enabled without explicit value
			}
			req.Thinking = &ThinkingConfig{Type: "enabled", BudgetTokens: budget}
		}
	case "adaptive":
		if supportsThinking {
			req.Thinking = &ThinkingConfig{Type: "adaptive"}
		}
	default:
		// Auto-detect: use adaptive for supported models
		if supportsThinking {
			req.Thinking = &ThinkingConfig{Type: "adaptive"}
		}
	}
}

// applyEffort sets the effort level on the request for models that support it.
func (c *Client) applyEffort(req *MessagesRequest) {
	if req.OutputConfig != nil {
		return // Already set
	}
	if !modelSupportsEffort(req.Model) {
		return
	}

	// Resolve effort: explicit setting or default to medium
	effort := c.effortLevel
	if effort == "" {
		effort = "medium" // default
	}

	req.OutputConfig = &OutputConfig{Effort: effort}
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", c.apiVersion)

	authResult := c.authResolver.Resolve()
	if authResult.IsOAuth {
		req.Header.Set("Authorization", "Bearer "+authResult.Token)
		req.Header.Set("anthropic-beta", "oauth-2025-04-20,claude-code-20250219,interleaved-thinking-2025-05-14,advanced-tool-use-2025-11-20,effort-2025-11-24")
		req.Header.Set("User-Agent", "claude-code/2.1.0 claude-cli")
		req.Header.Set("x-app", "cli")
	} else if authResult.Token != "" {
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("x-api-key", authResult.Token)
		req.Header.Set("anthropic-beta", "advanced-tool-use-2025-11-20,effort-2025-11-24")
	}
}
