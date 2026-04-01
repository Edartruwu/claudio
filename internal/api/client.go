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

// MessagesRequest is the request body for /v1/messages.
type MessagesRequest struct {
	Model       string          `json:"model"`
	MaxTokens   int             `json:"max_tokens"`
	Messages    []Message       `json:"messages"`
	System      string          `json:"system,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
	Tools       json.RawMessage `json:"tools,omitempty"`
	Temperature *float64        `json:"temperature,omitempty"`
	StopReason  string          `json:"stop_reason,omitempty"`
}

// Message represents a conversation message.
type Message struct {
	Role    string          `json:"role"` // "user", "assistant"
	Content json.RawMessage `json:"content"`
}

// StreamEvent represents a single SSE event from the streaming API.
type StreamEvent struct {
	Type  string          `json:"type"`
	Delta json.RawMessage `json:"delta,omitempty"`
	Index int             `json:"index,omitempty"`

	// Parsed content delta fields
	ContentBlock *ContentBlock `json:"-"`
	Usage        *Usage        `json:"-"`
	Message      *MessageResp  `json:"-"`
}

// ContentBlock represents a content block in the response.
type ContentBlock struct {
	Type  string          `json:"type"` // "text", "tool_use", "thinking"
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
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
func (c *Client) StreamMessages(ctx context.Context, req *MessagesRequest) (<-chan StreamEvent, <-chan error) {
	eventCh := make(chan StreamEvent, 64)
	errCh := make(chan error, 1)

	req.Stream = true
	if req.Model == "" {
		req.Model = c.model
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = 8192
	}

	go func() {
		defer close(eventCh)
		defer close(errCh)

		body, err := json.Marshal(req)
		if err != nil {
			errCh <- fmt.Errorf("failed to marshal request: %w", err)
			return
		}

		httpReq, err := http.NewRequestWithContext(ctx, "POST",
			c.baseURL+"/v1/messages", bytes.NewReader(body))
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
			errCh <- fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
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

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/v1/messages", bytes.NewReader(body))
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

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("anthropic-version", c.apiVersion)

	authResult := c.authResolver.Resolve()
	if authResult.IsOAuth {
		req.Header.Set("Authorization", "Bearer "+authResult.Token)
	} else if authResult.Token != "" {
		req.Header.Set("x-api-key", authResult.Token)
	}
}
