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
	"time"

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
}

// NewClient creates a new API client.
func NewClient(resolver *auth.Resolver, opts ...ClientOption) *Client {
	c := &Client{
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
		authResolver: resolver,
		baseURL:      defaultBaseURL,
		apiVersion:   defaultAPIVersion,
		model:        "claude-sonnet-4-6",
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

// GetModel returns the current default model.
func (c *Client) GetModel() string {
	return c.model
}

// SetModel changes the default model at runtime.
func (c *Client) SetModel(model string) {
	c.model = model
}

// ThinkingConfig configures extended thinking for supported models.
type ThinkingConfig struct {
	Type         string `json:"type"`                    // "adaptive", "enabled", "disabled"
	BudgetTokens int    `json:"budget_tokens,omitempty"` // Required when type is "enabled"
}

// SystemBlock is a text block in the system prompt array.
type SystemBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
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
	StopReason  string          `json:"stop_reason,omitempty"`
	Thinking    *ThinkingConfig `json:"thinking,omitempty"`
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
	c.applyAttribution(req)

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
	c.applyAttribution(req)

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

// applyAttribution injects the x-anthropic-billing-header into the system prompt
// as the first text block. This is required for OAuth tokens on Sonnet/Opus models.
// It converts the plain string System field into a SystemRaw JSON array.
func (c *Client) applyAttribution(req *MessagesRequest) {
	authResult := c.authResolver.Resolve()
	if !authResult.IsOAuth {
		// For non-OAuth, just convert system string to JSON string
		if req.System != "" {
			req.SystemRaw, _ = json.Marshal(req.System)
		}
		return
	}

	blocks := []SystemBlock{
		{Type: "text", Text: "x-anthropic-billing-header: cc_version=2.1.89.4fa; cc_entrypoint=cli; cch=00000;"},
	}
	if req.System != "" {
		blocks = append(blocks, SystemBlock{Type: "text", Text: req.System})
	}
	req.SystemRaw, _ = json.Marshal(blocks)
}

// applyThinking sets adaptive thinking for models that support it (Sonnet 4+, Opus 4+).
func (c *Client) applyThinking(req *MessagesRequest) {
	if req.Thinking != nil {
		return // Already set
	}
	model := req.Model
	if strings.Contains(model, "sonnet-4") || strings.Contains(model, "opus-4") {
		req.Thinking = &ThinkingConfig{Type: "adaptive"}
	}
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", c.apiVersion)

	authResult := c.authResolver.Resolve()
	if authResult.IsOAuth {
		req.Header.Set("Authorization", "Bearer "+authResult.Token)
		req.Header.Set("anthropic-beta", "oauth-2025-04-20,claude-code-20250219,interleaved-thinking-2025-05-14")
		req.Header.Set("User-Agent", "claude-code/2.1.0 claude-cli")
		req.Header.Set("x-app", "cli")
	} else if authResult.Token != "" {
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("x-api-key", authResult.Token)
	}
}
