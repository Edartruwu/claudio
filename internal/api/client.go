package api

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
	"time"

	"github.com/Abraxas-365/claudio/internal/auth"
	"github.com/Abraxas-365/claudio/internal/auth/refresh"
	"github.com/Abraxas-365/claudio/internal/auth/storage"
	"github.com/Abraxas-365/claudio/internal/prompts"
	"github.com/Abraxas-365/claudio/internal/ratelimit"
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
	storage      storage.SecureStorage // optional: enables 401 force-refresh
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

	// Multi-provider support
	providers      map[string]Provider // name -> provider
	modelRoutes    map[string]string   // glob pattern -> provider name
	modelShortcuts map[string]string   // shortcut name -> model ID (e.g. "llama" -> "llama-3.3-70b-versatile")
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

// WithStorage provides a SecureStorage so the client can force-refresh the
// OAuth token and retry when the API returns HTTP 401.
func WithStorage(store storage.SecureStorage) ClientOption {
	return func(c *Client) { c.storage = store }
}

// RegisterProvider adds a provider that can be used for model routing.
func (c *Client) RegisterProvider(name string, p Provider) {
	if c.providers == nil {
		c.providers = make(map[string]Provider)
	}
	c.providers[name] = p
}

// AddModelRoute maps a glob pattern (e.g. "llama-*") to a provider name.
func (c *Client) AddModelRoute(pattern, providerName string) {
	if c.modelRoutes == nil {
		c.modelRoutes = make(map[string]string)
	}
	c.modelRoutes[pattern] = providerName
}

// resolveProvider returns the provider for the given model, or nil to use the default Anthropic path.
// It checks routes against both the given model name and any shortcut alias that maps to it.
func (c *Client) resolveProvider(model string) Provider {
	for pattern, provName := range c.modelRoutes {
		matched, _ := filepath.Match(pattern, model)
		if matched {
			if p, ok := c.providers[provName]; ok {
				return p
			}
		}
	}
	// If no direct match, check if any shortcut alias that resolves to this model matches a route.
	for alias, resolved := range c.modelShortcuts {
		if resolved == model {
			for pattern, provName := range c.modelRoutes {
				matched, _ := filepath.Match(pattern, alias)
				if matched {
					if p, ok := c.providers[provName]; ok {
						return p
					}
				}
			}
		}
	}
	return nil
}

// IsExternalModel returns true if the given model routes to an external (non-Anthropic) provider.
// This is used to gate Claude-specific system prompt sections (e.g. Auto Memory instructions)
// that confuse smaller local models like Gemma.
func (c *Client) IsExternalModel(model string) bool {
	if resolved, ok := c.modelShortcuts[model]; ok {
		model = resolved
	}
	return c.resolveProvider(model) != nil
}

// NewClientFromExisting creates a shallow copy of an existing client with a different model.
// All other settings (auth, base URL, thinking mode, etc.) are inherited from the parent.
func NewClientFromExisting(parent *Client, model string) *Client {
	copy := *parent
	copy.model = model
	// Sub-agents don't need extended thinking — use disabled for speed.
	copy.thinkingMode = "disabled"
	// Inherit provider registry and routes
	copy.providers = parent.providers
	copy.modelRoutes = parent.modelRoutes
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

// ProviderModel describes a model available through a configured provider.
type ProviderModel struct {
	ModelID      string // e.g. "llama-3.3-70b-versatile"
	ProviderName string // e.g. "groq"
}

// GetProviderModels returns all model routing entries (pattern → provider).
func (c *Client) GetProviderModels() map[string]string {
	if c.modelRoutes == nil {
		return nil
	}
	result := make(map[string]string, len(c.modelRoutes))
	for k, v := range c.modelRoutes {
		result[k] = v
	}
	return result
}

// HasProvider returns true if a provider with the given name is registered.
func (c *Client) HasProvider(name string) bool {
	_, ok := c.providers[name]
	return ok
}

// GetProviderNames returns names of all registered providers.
func (c *Client) GetProviderNames() []string {
	names := make([]string, 0, len(c.providers))
	for name := range c.providers {
		names = append(names, name)
	}
	return names
}

// GetProvider returns a provider by name, or nil if not found.
func (c *Client) GetProvider(name string) Provider {
	return c.providers[name]
}

// HealthCheckResult holds the result of a provider connectivity test.
type HealthCheckResult struct {
	ProviderName string
	OK           bool
	Latency      time.Duration
	Error        string
	Model        string // model used for the test
}

// HealthCheck tests connectivity to a specific provider by sending a tiny request.
func (c *Client) HealthCheck(ctx context.Context, providerName string) HealthCheckResult {
	result := HealthCheckResult{ProviderName: providerName}

	p, ok := c.providers[providerName]
	if !ok {
		result.Error = "provider not registered"
		return result
	}

	// Find a model routed to this provider for the test.
	testModel := ""
	for pattern, pName := range c.modelRoutes {
		if pName == providerName {
			testModel = pattern
			break
		}
	}
	// If the pattern is a glob, try a shortcut instead.
	if strings.Contains(testModel, "*") || testModel == "" {
		for shortcut, modelID := range c.modelShortcuts {
			for pattern, pName := range c.modelRoutes {
				if pName == providerName {
					matched, _ := filepath.Match(pattern, shortcut)
					if matched {
						testModel = modelID
						break
					}
				}
			}
			if !strings.Contains(testModel, "*") && testModel != "" {
				break
			}
		}
	}
	if testModel == "" {
		testModel = "test"
	}
	result.Model = testModel

	// Send a minimal request.
	testContent, _ := json.Marshal([]UserContentBlock{NewTextBlock("hi")})
	req := &MessagesRequest{
		Model:     testModel,
		MaxTokens: 1,
		Messages: []Message{
			{Role: "user", Content: testContent},
		},
	}

	start := time.Now()
	_, err := p.SendMessage(ctx, c.httpClient, req)
	result.Latency = time.Since(start)

	if err != nil {
		result.Error = err.Error()
		// Some errors indicate the provider is reachable but rejected our test
		// (auth error, model not found, etc.) — that still counts as "reachable".
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "401") || strings.Contains(errStr, "403") ||
			strings.Contains(errStr, "model") || strings.Contains(errStr, "authentication") {
			result.OK = true // reachable but auth/model issue
			result.Error = "reachable but: " + result.Error
		}
	} else {
		result.OK = true
	}

	return result
}

// HealthCheckAll tests connectivity to all registered providers.
func (c *Client) HealthCheckAll(ctx context.Context) []HealthCheckResult {
	var results []HealthCheckResult
	for name := range c.providers {
		results = append(results, c.HealthCheck(ctx, name))
	}
	return results
}

// ModelLister is an optional interface that providers can implement to support
// model discovery. Type-assert a Provider to check if it supports listing.
type ModelLister interface {
	ListModels(ctx context.Context, httpClient *http.Client) ([]ModelInfo, error)
}

// ModelInfo describes an available model from a provider.
type ModelInfo struct {
	ID           string `json:"id"`
	OwnedBy      string `json:"owned_by,omitempty"`
	ProviderName string `json:"-"` // set by DiscoverModels, not part of API response
}

// DiscoverModels queries all providers that support model listing and returns
// the combined results. Providers that don't implement ModelLister are skipped.
func (c *Client) DiscoverModels(ctx context.Context) []ModelInfo {
	var all []ModelInfo
	for name, p := range c.providers {
		lister, ok := p.(ModelLister)
		if !ok {
			continue
		}
		models, err := lister.ListModels(ctx, c.httpClient)
		if err != nil {
			continue // skip providers that fail
		}
		for i := range models {
			models[i].ProviderName = name
		}
		all = append(all, models...)
	}
	return all
}

// AddModelShortcut registers a slash-command shortcut for a model.
func (c *Client) AddModelShortcut(shortcut, modelID string) {
	if c.modelShortcuts == nil {
		c.modelShortcuts = make(map[string]string)
	}
	c.modelShortcuts[shortcut] = modelID
}

// GetModelShortcuts returns all registered model shortcuts (shortcut -> model ID).
func (c *Client) GetModelShortcuts() map[string]string {
	if c.modelShortcuts == nil {
		return nil
	}
	result := make(map[string]string, len(c.modelShortcuts))
	for k, v := range c.modelShortcuts {
		result[k] = v
	}
	return result
}

// builtinModelShortcuts maps well-known short names to canonical model IDs.
var builtinModelShortcuts = map[string]string{
	"sonnet": "claude-sonnet-4-6",
	"opus":   "claude-opus-4-6",
	"haiku":  "claude-haiku-4-5-20251001",
}

// ResolveModelShortcut returns the model ID for a shortcut, or empty string if not found.
// Checks user-configured shortcuts first, then built-in shortcuts.
func (c *Client) ResolveModelShortcut(shortcut string) (string, bool) {
	if c.modelShortcuts != nil {
		if id, ok := c.modelShortcuts[shortcut]; ok {
			return id, true
		}
	}
	id, ok := builtinModelShortcuts[shortcut]
	return id, ok
}

// RenewTransport replaces the HTTP client's transport with a fresh one that
// has no stale keep-alive connections. Call this after an ECONNRESET error.
func (c *Client) RenewTransport() {
	c.httpClient = &http.Client{}
}

// IsTransientError reports whether the error is safe to retry: network-level
// connection errors (ECONNRESET, EPIPE) or HTTP-level transient failures
// (429 rate-limit, 529 overloaded, 5xx server errors).
func IsTransientError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// Network-level: stale keep-alive socket reset by server
	if strings.Contains(msg, "connection reset by peer") ||
		strings.Contains(msg, "ECONNRESET") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "EPIPE") ||
		strings.Contains(msg, "EOF") {
		return true
	}
	// HTTP-level: extract status from "API error (HTTP NNN): ..."
	if strings.Contains(msg, "API error (HTTP 429)") ||
		strings.Contains(msg, "API error (HTTP 529)") ||
		strings.Contains(msg, "overloaded_error") {
		return true
	}
	// 5xx server errors
	for _, code := range []string{"500", "502", "503", "504", "520", "524"} {
		if strings.Contains(msg, "API error (HTTP "+code+")") {
			return true
		}
	}
	return false
}

// IsConnectionResetError reports whether the error is specifically a TCP
// connection reset (ECONNRESET/EPIPE), indicating a stale keep-alive socket.
func IsConnectionResetError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "connection reset by peer") ||
		strings.Contains(msg, "ECONNRESET") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "EPIPE")
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

	// ContextManagement enables server-side context editing (Anthropic only).
	// When set, the API strips old tool results from the cached prefix.
	ContextManagement *ContextManagement `json:"context_management,omitempty"`
}

// ContextManagement configures server-side context editing strategies.
// See: https://platform.claude.com/docs/en/build-with-claude/context-editing
type ContextManagement struct {
	Edits []ContextEdit `json:"edits"`
}

// ContextEdit is a single context editing strategy.
type ContextEdit struct {
	Type             string      `json:"type"`                         // e.g. "clear_tool_uses_20250919"
	Trigger          *Trigger    `json:"trigger,omitempty"`
	Keep             *KeepConfig `json:"keep,omitempty"`               // which tool uses to protect
	ExcludeTools     []string    `json:"exclude_tools,omitempty"`      // tool names to never clear
	ClearToolInputs  bool        `json:"clear_tool_inputs,omitempty"`  // also clear tool_use input params
}

// KeepConfig specifies how many recent tool uses to preserve.
type KeepConfig struct {
	Type  string `json:"type"`            // "tool_uses"
	Value int    `json:"value,omitempty"` // number to keep
}

// Trigger defines when a context edit activates.
type Trigger struct {
	Type  string `json:"type"`            // "input_tokens" or "tool_uses"
	Value int    `json:"value,omitempty"` // threshold value
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
	Type      string          `json:"type"` // "text", "tool_use", "tool_result", "thinking"
	Text      string          `json:"text,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	Signature string          `json:"signature,omitempty"` // Required when sending thinking blocks back
	ID        string          `json:"id,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"` // Used in tool_result blocks
	Content   string          `json:"content,omitempty"`     // Used in tool_result blocks
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

// normalizeMessages ensures every message's Content is a valid JSON array
// with no broken content blocks. The Anthropic API requires content to be a
// list of content blocks, but various code paths (compaction, session restore,
// streaming) can accidentally produce a plain JSON string, null, or text
// blocks with a missing "text" field (omitempty drops empty strings). This
// function catches all those cases.
func normalizeMessages(msgs []Message) {
	for i := range msgs {
		c := msgs[i].Content
		if len(c) == 0 {
			// null / empty → the API rejects empty text blocks entirely,
			// so use a single-space placeholder instead.
			msgs[i].Content = json.RawMessage(`[{"type":"text","text":"[No message content]"}]`)
			continue
		}
		// Fast check: valid content starts with '['
		trimmed := bytes.TrimLeft(c, " \t\n\r")
		if len(trimmed) > 0 && trimmed[0] == '[' {
			// Already an array — but sanitize blocks that may have missing fields.
			msgs[i].Content = sanitizeContentBlocks(c)
			continue
		}
		// It's a JSON string, number, object, or something else — wrap it.
		var s string
		if json.Unmarshal(c, &s) == nil {
			// It was a JSON string like `"hello world"`
			msgs[i].Content, _ = json.Marshal([]UserContentBlock{NewTextBlock(s)})
		} else {
			// Some other non-array JSON (object, number, etc.) — use raw as text
			msgs[i].Content, _ = json.Marshal([]UserContentBlock{NewTextBlock(string(c))})
		}
	}
}

// sanitizeContentBlocks removes empty text blocks from a JSON content array.
// The Anthropic API rejects text blocks that are empty (missing "text" field
// due to omitempty, or "text":""). This strips them entirely so the request
// is valid. If ALL blocks would be removed, a single-space placeholder is kept.
func sanitizeContentBlocks(raw json.RawMessage) json.RawMessage {
	var blocks []json.RawMessage
	if json.Unmarshal(raw, &blocks) != nil {
		return raw
	}
	modified := false
	kept := make([]json.RawMessage, 0, len(blocks))
	for _, b := range blocks {
		var peek struct {
			Type string  `json:"type"`
			Text *string `json:"text"` // pointer distinguishes missing from empty
		}
		if json.Unmarshal(b, &peek) != nil {
			kept = append(kept, b)
			continue
		}
		if peek.Type == "text" && (peek.Text == nil || strings.TrimSpace(*peek.Text) == "") {
			modified = true
			continue // drop empty text block
		}
		kept = append(kept, b)
	}
	if !modified {
		return raw
	}
	// If all blocks were empty text, keep a single-space placeholder
	// so the message is not entirely contentless.
	if len(kept) == 0 {
		return json.RawMessage(`[{"type":"text","text":"[No message content]"}]`)
	}
	out, err := json.Marshal(kept)
	if err != nil {
		return raw
	}
	return out
}

// filterWhitespaceAssistantMessages removes assistant messages whose content
// blocks are ALL whitespace-only text (the API rejects these). When removal
// creates adjacent user messages they are merged to maintain role alternation.
// Non-final assistant messages that become entirely empty get a placeholder
// ("[No message content]") to satisfy the API's non-empty requirement.
func filterWhitespaceAssistantMessages(msgs []Message) []Message {
	// First pass: mark whitespace-only assistant messages for removal
	remove := make([]bool, len(msgs))
	for i, m := range msgs {
		if m.Role != "assistant" {
			continue
		}
		var blocks []json.RawMessage
		if json.Unmarshal(m.Content, &blocks) != nil {
			continue
		}
		allWhitespace := true
		for _, b := range blocks {
			var peek struct {
				Type string  `json:"type"`
				Text *string `json:"text"`
			}
			if json.Unmarshal(b, &peek) != nil {
				allWhitespace = false
				break
			}
			if peek.Type != "text" {
				allWhitespace = false
				break
			}
			if peek.Text != nil && strings.TrimSpace(*peek.Text) != "" {
				allWhitespace = false
				break
			}
		}
		if allWhitespace && len(blocks) > 0 {
			remove[i] = true
		}
	}

	// Second pass: rebuild slice, merging adjacent user messages
	out := make([]Message, 0, len(msgs))
	for i, m := range msgs {
		if remove[i] {
			continue
		}
		// Merge adjacent user messages
		if m.Role == "user" && len(out) > 0 && out[len(out)-1].Role == "user" {
			var prev, cur []json.RawMessage
			if json.Unmarshal(out[len(out)-1].Content, &prev) == nil &&
				json.Unmarshal(m.Content, &cur) == nil {
				merged, _ := json.Marshal(append(prev, cur...))
				out[len(out)-1].Content = merged
				continue
			}
		}
		out = append(out, m)
	}
	return out
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

	normalizeMessages(req.Messages)
	req.Messages = filterWhitespaceAssistantMessages(req.Messages)

	// Route to external provider if a match exists
	if p := c.resolveProvider(req.Model); p != nil {
		req.Stream = true
		return p.StreamMessages(ctx, c.httpClient, req)
	}

	req.Stream = true
	c.applyThinking(req)
	c.applyEffort(req)
	c.applyAttribution(req)
	c.applyContextManagement(req)
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

		if resp.StatusCode == http.StatusUnauthorized && c.storage != nil {
			resp.Body.Close()
			// Attempt a force-refresh (best-effort; ignore whether tokens were
			// actually rotated — the server told us 401 so we retry regardless).
			refresh.CheckAndRefreshIfNeeded(c.storage, true) //nolint:errcheck
			httpReq2, err2 := http.NewRequestWithContext(ctx, "POST",
				c.baseURL+"/v1/messages?beta=true", bytes.NewReader(body))
			if err2 != nil {
				errCh <- err2
				return
			}
			c.setHeaders(httpReq2)
			resp2, err2 := c.httpClient.Do(httpReq2)
			if err2 != nil {
				errCh <- fmt.Errorf("API request failed: %w", err2)
				return
			}
			resp = resp2
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			ratelimit.ExtractFromError(resp.StatusCode, resp.Header)
			bodyBytes, _ := io.ReadAll(resp.Body)
			retryAfter := resp.Header.Get("retry-after")
			extra := ""
			if retryAfter != "" {
				extra = fmt.Sprintf(" (retry after %ss)", retryAfter)
			}
			errCh <- fmt.Errorf("API error (HTTP %d): %s%s", resp.StatusCode, string(bodyBytes), extra)
			return
		}

		// Extract rate-limit headers from every successful response
		ratelimit.ExtractFromHeaders(resp.Header)

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
	if req.Model == "" {
		req.Model = c.model
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = 8192
	}

	normalizeMessages(req.Messages)

	// Route to external provider if a match exists
	if p := c.resolveProvider(req.Model); p != nil {
		req.Stream = false
		return p.SendMessage(ctx, c.httpClient, req)
	}

	req.Stream = false
	c.applyThinking(req)
	c.applyEffort(req)
	c.applyAttribution(req)
	c.applyContextManagement(req)
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

	if resp.StatusCode == http.StatusUnauthorized && c.storage != nil {
		resp.Body.Close()
		// Attempt a force-refresh (best-effort; ignore whether tokens were
		// actually rotated — the server told us 401 so we retry regardless).
		refresh.CheckAndRefreshIfNeeded(c.storage, true) //nolint:errcheck
		httpReq2, err2 := http.NewRequestWithContext(ctx, "POST",
			c.baseURL+"/v1/messages?beta=true", bytes.NewReader(body))
		if err2 != nil {
			return nil, err2
		}
		c.setHeaders(httpReq2)
		resp2, err2 := c.httpClient.Do(httpReq2)
		if err2 != nil {
			return nil, fmt.Errorf("API request failed: %w", err2)
		}
		resp = resp2
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
		// Walk backwards to find the last non-empty-text block.
		// The API rejects cache_control on empty text blocks.
		lastBlockIdx := -1
		for j := len(blocks) - 1; j >= 0; j-- {
			var peek struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if json.Unmarshal(blocks[j], &peek) == nil && peek.Type == "text" && peek.Text == "" {
				continue
			}
			lastBlockIdx = j
			break
		}
		if lastBlockIdx < 0 {
			return
		}
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
//
// When prompt caching is enabled it splits the system prompt at
// SystemPromptDynamicBoundary into two blocks:
//
//   - Static prefix  (cache_control set): identical across all sessions — the large
//     block of instructions and tool guidance that never changes between runs.
//     This block gets its own cache entry so it survives session changes.
//
//   - Dynamic suffix (no cache_control): cwd, date, CLAUDE.md content, etc.
//     This is implicitly covered by the downstream tool-definitions cache breakpoint
//     (applyMessageCacheBreakpoints), keeping the total breakpoint count at 4.
func (c *Client) applyAttribution(req *MessagesRequest) {
	authResult := c.authResolver.Resolve()

	// splitSystemBlocks splits req.System at SystemPromptDynamicBoundary and
	// returns the system blocks with appropriate cache_control settings.
	splitSystemBlocks := func(system string) []SystemBlock {
		if system == "" {
			return nil
		}
		parts := strings.SplitN(system, prompts.SystemPromptDynamicBoundary, 2)
		staticText := strings.TrimSpace(parts[0])
		var dynamicText string
		if len(parts) == 2 {
			dynamicText = strings.TrimSpace(parts[1])
		}

		if staticText == "" || !c.promptCaching {
			// No boundary or caching disabled — single block.
			block := SystemBlock{Type: "text", Text: system}
			if c.promptCaching {
				block.CacheControl = &CacheControlBlock{Type: "ephemeral"}
			}
			return []SystemBlock{block}
		}

		// Static prefix gets its own cache entry — stable across sessions.
		blocks := []SystemBlock{
			{Type: "text", Text: staticText, CacheControl: &CacheControlBlock{Type: "ephemeral"}},
		}
		// Dynamic suffix has no cache_control here; the tools cache breakpoint
		// (applied later in applyMessageCacheBreakpoints) covers it implicitly.
		if dynamicText != "" {
			blocks = append(blocks, SystemBlock{Type: "text", Text: dynamicText})
		}
		return blocks
	}

	if !authResult.IsOAuth {
		if req.System != "" {
			req.SystemRaw, _ = json.Marshal(splitSystemBlocks(req.System))
		}
		return
	}

	// OAuth: prepend billing header as the first block, then the split system blocks.
	blocks := []SystemBlock{
		{Type: "text", Text: "x-anthropic-billing-header: cc_version=2.1.89.4fa; cc_entrypoint=cli; cch=00000;"},
	}
	blocks = append(blocks, splitSystemBlocks(req.System)...)
	// If caching is off (splitSystemBlocks returns unmodified block), ensure the
	// last block is still marked so we don't regress on the OAuth path.
	if c.promptCaching && len(blocks) > 0 {
		last := &blocks[len(blocks)-1]
		if last.CacheControl == nil {
			last.CacheControl = &CacheControlBlock{Type: "ephemeral"}
		}
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

// applyContextManagement adds server-side context editing for Anthropic requests.
// This configures the API to automatically clear old tool results from the cached
// prefix when context grows large, keeping the last N tool uses intact.
// Only applies to first-party Anthropic requests (not external providers).
func (c *Client) applyContextManagement(req *MessagesRequest) {
	// Don't override if already set explicitly.
	if req.ContextManagement != nil {
		return
	}

	// Only apply for Anthropic models (external providers don't support this).
	if c.resolveProvider(req.Model) != nil {
		return
	}

	req.ContextManagement = &ContextManagement{
		Edits: []ContextEdit{
			{
				Type: "clear_tool_uses_20250919",
				Trigger: &Trigger{
					Type:      "input_tokens",
					Value: 80_000, // ~40% of 200K context — clear early to preserve cache
				},
				Keep: &KeepConfig{Type: "tool_uses", Value: 10},
				ClearToolInputs: true,
			},
		},
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
		req.Header.Set("anthropic-beta", "oauth-2025-04-20,claude-code-20250219,interleaved-thinking-2025-05-14,advanced-tool-use-2025-11-20,effort-2025-11-24,context-management-2025-06-27")
		req.Header.Set("User-Agent", "claude-code/2.1.0 claude-cli")
		req.Header.Set("x-app", "cli")
	} else if authResult.Token != "" {
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("x-api-key", authResult.Token)
		req.Header.Set("anthropic-beta", "advanced-tool-use-2025-11-20,effort-2025-11-24,context-management-2025-06-27")
	}
}
