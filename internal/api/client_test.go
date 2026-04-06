package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Abraxas-365/claudio/internal/auth"
	"github.com/Abraxas-365/claudio/internal/auth/oauth"
	"github.com/Abraxas-365/claudio/internal/auth/storage"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newTestClient(t *testing.T) *Client {
	t.Helper()
	store := storage.NewPlaintextStorage(t.TempDir() + "/creds.json")
	resolver := auth.NewResolver(store)
	return NewClient(resolver, WithPromptCaching(true))
}

func newTestClientNoCaching(t *testing.T) *Client {
	t.Helper()
	store := storage.NewPlaintextStorage(t.TempDir() + "/creds.json")
	resolver := auth.NewResolver(store)
	return NewClient(resolver, WithPromptCaching(false))
}

func makeMsg(role, text string) Message {
	content, _ := json.Marshal([]map[string]string{{"type": "text", "text": text}})
	return Message{Role: role, Content: content}
}

// mockProvider is a minimal Provider implementation for routing tests.
type mockProvider struct{ name string }

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) StreamMessages(_ context.Context, _ *http.Client, _ *MessagesRequest) (<-chan StreamEvent, <-chan error) {
	ch := make(chan StreamEvent)
	e := make(chan error, 1)
	close(ch)
	close(e)
	return ch, e
}
func (m *mockProvider) SendMessage(_ context.Context, _ *http.Client, _ *MessagesRequest) (*MessageResp, error) {
	return &MessageResp{}, nil
}

// ---------------------------------------------------------------------------
// IsTransientError
// ---------------------------------------------------------------------------

func TestIsTransientError_NetworkErrors(t *testing.T) {
	cases := []string{
		"connection reset by peer",
		"ECONNRESET",
		"broken pipe",
		"EPIPE",
		"EOF",
	}
	for _, msg := range cases {
		t.Run(msg, func(t *testing.T) {
			if !IsTransientError(errors.New(msg)) {
				t.Errorf("expected IsTransientError(%q) == true", msg)
			}
		})
	}
}

func TestIsTransientError_RateLimitErrors(t *testing.T) {
	cases := []string{
		"API error (HTTP 429): too many requests",
		"API error (HTTP 529): overloaded",
		"overloaded_error: the server is busy",
	}
	for _, msg := range cases {
		t.Run(msg, func(t *testing.T) {
			if !IsTransientError(errors.New(msg)) {
				t.Errorf("expected IsTransientError(%q) == true", msg)
			}
		})
	}
}

func TestIsTransientError_ServerErrors(t *testing.T) {
	codes := []int{500, 502, 503, 504, 520, 524}
	for _, code := range codes {
		msg := fmt.Sprintf("API error (HTTP %d): server error", code)
		t.Run(msg, func(t *testing.T) {
			if !IsTransientError(errors.New(msg)) {
				t.Errorf("expected IsTransientError(%q) == true", msg)
			}
		})
	}
}

func TestIsTransientError_ClientErrors(t *testing.T) {
	cases := []string{
		"API error (HTTP 400): bad request",
		"API error (HTTP 401): unauthorized",
	}
	for _, msg := range cases {
		t.Run(msg, func(t *testing.T) {
			if IsTransientError(errors.New(msg)) {
				t.Errorf("expected IsTransientError(%q) == false", msg)
			}
		})
	}
}

func TestIsTransientError_Nil(t *testing.T) {
	if IsTransientError(nil) {
		t.Error("expected IsTransientError(nil) == false")
	}
}

// ---------------------------------------------------------------------------
// IsConnectionResetError
// ---------------------------------------------------------------------------

func TestIsConnectionResetError(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		{"connection reset by peer", true},
		{"ECONNRESET: something", true},
		{"broken pipe", true},
		{"EPIPE", true},
		{"API error (HTTP 429): rate limited", false},
		{"EOF", false},
		{"some random error", false},
	}
	for _, tc := range cases {
		t.Run(tc.msg, func(t *testing.T) {
			got := IsConnectionResetError(errors.New(tc.msg))
			if got != tc.want {
				t.Errorf("IsConnectionResetError(%q) = %v, want %v", tc.msg, got, tc.want)
			}
		})
	}
}

func TestIsConnectionResetError_Nil(t *testing.T) {
	if IsConnectionResetError(nil) {
		t.Error("expected IsConnectionResetError(nil) == false")
	}
}

// ---------------------------------------------------------------------------
// applyMessageCacheBreakpoints
// ---------------------------------------------------------------------------

func hasCacheControl(msg Message) bool {
	var blocks []json.RawMessage
	if json.Unmarshal(msg.Content, &blocks) != nil || len(blocks) == 0 {
		return false
	}
	// Check the last block only (that's where markLastBlock puts cache_control).
	var block map[string]json.RawMessage
	if json.Unmarshal(blocks[len(blocks)-1], &block) != nil {
		return false
	}
	_, ok := block["cache_control"]
	return ok
}

func TestApplyMessageCacheBreakpoints_Disabled(t *testing.T) {
	c := newTestClientNoCaching(t)
	req := &MessagesRequest{
		Model:    "claude-sonnet-4-6",
		Messages: []Message{makeMsg("user", "a"), makeMsg("assistant", "b"), makeMsg("user", "c")},
	}
	c.applyMessageCacheBreakpoints(req)
	for i, msg := range req.Messages {
		if hasCacheControl(msg) {
			t.Errorf("message[%d] should not have cache_control when caching is disabled", i)
		}
	}
}

func TestApplyMessageCacheBreakpoints_TooFewMessages(t *testing.T) {
	c := newTestClient(t)
	req := &MessagesRequest{
		Model:    "claude-sonnet-4-6",
		Messages: []Message{makeMsg("user", "only one")},
	}
	c.applyMessageCacheBreakpoints(req)
	if hasCacheControl(req.Messages[0]) {
		t.Error("single message should not get cache_control")
	}
}

func TestApplyMessageCacheBreakpoints_ShortHistory_MarksSecondToLast(t *testing.T) {
	c := newTestClient(t)
	// 3 messages: indices 0, 1, 2
	req := &MessagesRequest{
		Model: "claude-sonnet-4-6",
		Messages: []Message{
			makeMsg("user", "msg0"),
			makeMsg("assistant", "msg1"),
			makeMsg("user", "msg2"),
		},
	}
	c.applyMessageCacheBreakpoints(req)
	// messages[1] is second-to-last → must have cache_control
	if !hasCacheControl(req.Messages[1]) {
		t.Error("messages[1] (second-to-last) should have cache_control")
	}
	// messages[2] is the current user turn → must NOT have cache_control
	if hasCacheControl(req.Messages[2]) {
		t.Error("messages[2] (last) should NOT have cache_control")
	}
}

func TestApplyMessageCacheBreakpoints_LongHistory_MarksMidpoint(t *testing.T) {
	c := newTestClient(t)
	// 10 messages: indices 0..9
	msgs := make([]Message, 10)
	for i := range msgs {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		msgs[i] = makeMsg(role, fmt.Sprintf("msg%d", i))
	}
	req := &MessagesRequest{
		Model:    "claude-sonnet-4-6",
		Messages: msgs,
	}
	c.applyMessageCacheBreakpoints(req)

	// midpoint = len/3 = 3
	midIdx := 10 / 3
	if !hasCacheControl(req.Messages[midIdx]) {
		t.Errorf("messages[%d] (midpoint) should have cache_control", midIdx)
	}
	// second-to-last = index 8
	if !hasCacheControl(req.Messages[8]) {
		t.Error("messages[8] (second-to-last) should have cache_control")
	}
	// last (index 9) must NOT have cache_control
	if hasCacheControl(req.Messages[9]) {
		t.Error("messages[9] (last) should NOT have cache_control")
	}
}

func TestApplyMessageCacheBreakpoints_ToolDefinitions(t *testing.T) {
	c := newTestClient(t)

	// Build tools JSON: one normal tool and one deferred (defer_loading=true) tool.
	normalTool := map[string]interface{}{
		"name":        "MyTool",
		"description": "does stuff",
		"input_schema": map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}
	deferredTool := map[string]interface{}{
		"name":         "DeferredTool",
		"description":  "deferred",
		"defer_loading": true,
		"input_schema": map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}
	toolsJSON, _ := json.Marshal([]interface{}{normalTool, deferredTool})

	req := &MessagesRequest{
		Model: "claude-sonnet-4-6",
		Tools: toolsJSON,
		// Need at least 2 messages to not exit early
		Messages: []Message{makeMsg("user", "a"), makeMsg("assistant", "b"), makeMsg("user", "c")},
	}
	c.applyMessageCacheBreakpoints(req)

	var toolDefs []map[string]json.RawMessage
	if err := json.Unmarshal(req.Tools, &toolDefs); err != nil {
		t.Fatalf("failed to parse tools: %v", err)
	}

	// The non-deferred tool (index 0) should have cache_control.
	if _, ok := toolDefs[0]["cache_control"]; !ok {
		t.Error("normal tool should have cache_control")
	}
	// The deferred tool (index 1) should NOT have cache_control.
	if _, ok := toolDefs[1]["cache_control"]; ok {
		t.Error("deferred tool should NOT have cache_control")
	}
}

func TestApplyMessageCacheBreakpoints_PreservesMessageContent(t *testing.T) {
	c := newTestClient(t)
	req := &MessagesRequest{
		Model: "claude-sonnet-4-6",
		Messages: []Message{
			makeMsg("user", "original text"),
			makeMsg("assistant", "assistant reply"),
			makeMsg("user", "current turn"),
		},
	}
	c.applyMessageCacheBreakpoints(req)

	// messages[1] gets marked — verify its text block still has the original text.
	var blocks []map[string]json.RawMessage
	if err := json.Unmarshal(req.Messages[1].Content, &blocks); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(blocks) == 0 {
		t.Fatal("no blocks in message[1]")
	}
	var textVal string
	if err := json.Unmarshal(blocks[0]["text"], &textVal); err != nil {
		t.Fatalf("text field parse error: %v", err)
	}
	if textVal != "assistant reply" {
		t.Errorf("text content changed: got %q, want %q", textVal, "assistant reply")
	}
	var typeVal string
	if err := json.Unmarshal(blocks[0]["type"], &typeVal); err != nil {
		t.Fatalf("type field parse error: %v", err)
	}
	if typeVal != "text" {
		t.Errorf("type field changed: got %q, want %q", typeVal, "text")
	}
}

// ---------------------------------------------------------------------------
// applyThinking
// ---------------------------------------------------------------------------

func newClientWithModel(t *testing.T, model, thinkingMode string, budgetTokens int) *Client {
	t.Helper()
	c := newTestClient(t)
	c.model = model
	c.thinkingMode = thinkingMode
	c.budgetTokens = budgetTokens
	return c
}

func TestApplyThinking_AlreadySet(t *testing.T) {
	c := newClientWithModel(t, "claude-sonnet-4-6", "enabled", 5000)
	existing := &ThinkingConfig{Type: "enabled", BudgetTokens: 1234}
	req := &MessagesRequest{Model: "claude-sonnet-4-6", Thinking: existing}
	c.applyThinking(req)
	if req.Thinking != existing {
		t.Error("applyThinking should not modify a request that already has Thinking set")
	}
	if req.Thinking.BudgetTokens != 1234 {
		t.Error("BudgetTokens was changed unexpectedly")
	}
}

func TestApplyThinking_DisabledMode(t *testing.T) {
	c := newClientWithModel(t, "claude-sonnet-4-6", "disabled", 0)
	req := &MessagesRequest{Model: "claude-sonnet-4-6"}
	c.applyThinking(req)
	if req.Thinking != nil {
		t.Errorf("expected Thinking == nil for disabled mode, got %+v", req.Thinking)
	}
}

func TestApplyThinking_EnabledMode_SupportedModel(t *testing.T) {
	c := newClientWithModel(t, "claude-sonnet-4-6", "enabled", 5000)
	req := &MessagesRequest{Model: "claude-sonnet-4-6"}
	c.applyThinking(req)
	if req.Thinking == nil {
		t.Fatal("expected Thinking != nil")
	}
	if req.Thinking.Type != "enabled" {
		t.Errorf("Type = %q, want %q", req.Thinking.Type, "enabled")
	}
	if req.Thinking.BudgetTokens != 5000 {
		t.Errorf("BudgetTokens = %d, want %d", req.Thinking.BudgetTokens, 5000)
	}
}

func TestApplyThinking_EnabledMode_DefaultBudget(t *testing.T) {
	c := newClientWithModel(t, "claude-sonnet-4-6", "enabled", 0)
	req := &MessagesRequest{Model: "claude-sonnet-4-6"}
	c.applyThinking(req)
	if req.Thinking == nil {
		t.Fatal("expected Thinking != nil")
	}
	if req.Thinking.BudgetTokens != 10000 {
		t.Errorf("BudgetTokens = %d, want 10000 (default)", req.Thinking.BudgetTokens)
	}
}

func TestApplyThinking_AdaptiveMode(t *testing.T) {
	c := newClientWithModel(t, "claude-sonnet-4-6", "adaptive", 0)
	req := &MessagesRequest{Model: "claude-sonnet-4-6"}
	c.applyThinking(req)
	if req.Thinking == nil {
		t.Fatal("expected Thinking != nil")
	}
	if req.Thinking.Type != "adaptive" {
		t.Errorf("Type = %q, want %q", req.Thinking.Type, "adaptive")
	}
}

func TestApplyThinking_AutoDetect(t *testing.T) {
	// mode="" → auto-detect: supported model should get adaptive
	c := newClientWithModel(t, "claude-sonnet-4-6", "", 0)
	req := &MessagesRequest{Model: "claude-sonnet-4-6"}
	c.applyThinking(req)
	if req.Thinking == nil {
		t.Fatal("expected Thinking != nil for supported model with auto-detect")
	}
	if req.Thinking.Type != "adaptive" {
		t.Errorf("Type = %q, want %q", req.Thinking.Type, "adaptive")
	}
}

func TestApplyThinking_UnsupportedModel(t *testing.T) {
	c := newClientWithModel(t, "gpt-4", "", 0)
	req := &MessagesRequest{Model: "gpt-4"}
	c.applyThinking(req)
	if req.Thinking != nil {
		t.Errorf("expected Thinking == nil for unsupported model, got %+v", req.Thinking)
	}
}

// ---------------------------------------------------------------------------
// applyEffort
// ---------------------------------------------------------------------------

func newClientWithEffort(t *testing.T, model, effortLevel string) *Client {
	t.Helper()
	c := newTestClient(t)
	c.model = model
	c.effortLevel = effortLevel
	return c
}

func TestApplyEffort_AlreadySet(t *testing.T) {
	c := newClientWithEffort(t, "claude-sonnet-4-6", "high")
	existing := &OutputConfig{Effort: "low"}
	req := &MessagesRequest{Model: "claude-sonnet-4-6", OutputConfig: existing}
	c.applyEffort(req)
	if req.OutputConfig != existing {
		t.Error("applyEffort should not modify request that already has OutputConfig")
	}
	if req.OutputConfig.Effort != "low" {
		t.Error("Effort was changed unexpectedly")
	}
}

func TestApplyEffort_UnsupportedModel(t *testing.T) {
	c := newClientWithEffort(t, "claude-haiku-4-5-20251001", "")
	req := &MessagesRequest{Model: "claude-haiku-4-5-20251001"}
	c.applyEffort(req)
	if req.OutputConfig != nil {
		t.Errorf("expected OutputConfig == nil for unsupported model, got %+v", req.OutputConfig)
	}
}

func TestApplyEffort_SupportedModel_DefaultEffort(t *testing.T) {
	c := newClientWithEffort(t, "claude-sonnet-4-6", "")
	req := &MessagesRequest{Model: "claude-sonnet-4-6"}
	c.applyEffort(req)
	if req.OutputConfig == nil {
		t.Fatal("expected OutputConfig != nil for supported model")
	}
	if req.OutputConfig.Effort != "medium" {
		t.Errorf("Effort = %q, want %q", req.OutputConfig.Effort, "medium")
	}
}

func TestApplyEffort_SupportedModel_ExplicitHigh(t *testing.T) {
	c := newClientWithEffort(t, "claude-sonnet-4-6", "high")
	req := &MessagesRequest{Model: "claude-sonnet-4-6"}
	c.applyEffort(req)
	if req.OutputConfig == nil {
		t.Fatal("expected OutputConfig != nil")
	}
	if req.OutputConfig.Effort != "high" {
		t.Errorf("Effort = %q, want %q", req.OutputConfig.Effort, "high")
	}
}

func TestApplyEffort_SupportedModel_Low(t *testing.T) {
	c := newClientWithEffort(t, "claude-sonnet-4-6", "low")
	req := &MessagesRequest{Model: "claude-sonnet-4-6"}
	c.applyEffort(req)
	if req.OutputConfig == nil {
		t.Fatal("expected OutputConfig != nil")
	}
	if req.OutputConfig.Effort != "low" {
		t.Errorf("Effort = %q, want %q", req.OutputConfig.Effort, "low")
	}
}

// ---------------------------------------------------------------------------
// resolveProvider
// ---------------------------------------------------------------------------

func TestResolveProvider_NoProviders(t *testing.T) {
	c := newTestClient(t)
	// no providers or routes registered
	if p := c.resolveProvider("any-model"); p != nil {
		t.Error("expected nil provider when no providers are registered")
	}
}

func TestResolveProvider_GlobMatch(t *testing.T) {
	c := newTestClient(t)
	groq := &mockProvider{name: "groq"}
	c.RegisterProvider("groq", groq)
	c.AddModelRoute("llama-*", "groq")

	p := c.resolveProvider("llama-3.3-70b-versatile")
	if p == nil {
		t.Fatal("expected non-nil provider for llama-* glob match")
	}
	if p != groq {
		t.Error("resolved provider is not the registered groq provider")
	}
}

func TestResolveProvider_NoMatch(t *testing.T) {
	c := newTestClient(t)
	c.RegisterProvider("groq", &mockProvider{name: "groq"})
	c.AddModelRoute("llama-*", "groq")

	if p := c.resolveProvider("gpt-4"); p != nil {
		t.Error("expected nil provider for non-matching model")
	}
}

func TestResolveProvider_ShortcutAlias(t *testing.T) {
	c := newTestClient(t)
	groq := &mockProvider{name: "groq"}
	c.RegisterProvider("groq", groq)
	// Route maps the shortcut name directly
	c.AddModelRoute("llama", "groq")
	// Shortcut maps the alias to the full model ID
	c.AddModelShortcut("llama", "llama-3.3-70b-versatile")

	// resolveProvider is called with the full model ID after shortcut resolution
	p := c.resolveProvider("llama-3.3-70b-versatile")
	if p == nil {
		t.Fatal("expected non-nil provider via shortcut alias")
	}
	if p != groq {
		t.Error("resolved provider is not the registered groq provider")
	}
}

// ---------------------------------------------------------------------------
// modelSupportsEffort
// ---------------------------------------------------------------------------

func TestModelSupportsEffort(t *testing.T) {
	cases := []struct {
		model string
		want  bool
	}{
		{"claude-sonnet-4-6", true},
		{"claude-opus-4", true},
		{"claude-opus-4-6", true},
		{"claude-sonnet-4", true},
		{"claude-haiku-4-5-20251001", false},
		{"gpt-4", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.model, func(t *testing.T) {
			got := modelSupportsEffort(tc.model)
			if got != tc.want {
				t.Errorf("modelSupportsEffort(%q) = %v, want %v", tc.model, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GetModel / SetModel
// ---------------------------------------------------------------------------

func TestGetSetModel(t *testing.T) {
	c := newTestClient(t)
	initial := c.GetModel()
	if initial == "" {
		t.Error("expected non-empty initial model")
	}
	c.SetModel("claude-opus-4")
	if got := c.GetModel(); got != "claude-opus-4" {
		t.Errorf("GetModel() = %q, want %q", got, "claude-opus-4")
	}
}

// ---------------------------------------------------------------------------
// ThinkingLabel
// ---------------------------------------------------------------------------

func TestThinkingLabel(t *testing.T) {
	cases := []struct {
		mode         string
		budgetTokens int
		want         string
	}{
		{"disabled", 0, "Disabled"},
		{"enabled", 0, "Enabled"},
		{"enabled", 5000, "Enabled (5k tokens)"},
		{"enabled", 10000, "Enabled (10k tokens)"},
		{"adaptive", 0, "Adaptive"},
		{"", 0, "Auto (adaptive)"},
		{"unknown-value", 0, "Auto (adaptive)"},
	}
	for _, tc := range cases {
		t.Run(tc.mode+"_"+fmt.Sprint(tc.budgetTokens), func(t *testing.T) {
			c := newTestClient(t)
			c.thinkingMode = tc.mode
			c.budgetTokens = tc.budgetTokens
			got := c.ThinkingLabel()
			if got != tc.want {
				t.Errorf("ThinkingLabel() = %q, want %q", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// EffortLabel
// ---------------------------------------------------------------------------

func TestEffortLabel(t *testing.T) {
	cases := []struct {
		level string
		want  string
	}{
		{"low", "Low effort"},
		{"high", "High effort"},
		{"medium", "Medium effort (default)"},
		{"", "Medium effort (default)"},
		{"other", "Medium effort (default)"},
	}
	for _, tc := range cases {
		t.Run(tc.level, func(t *testing.T) {
			c := newTestClient(t)
			c.effortLevel = tc.level
			got := c.EffortLabel()
			if got != tc.want {
				t.Errorf("EffortLabel() = %q, want %q", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GetEffortLevel / SetEffortLevel
// ---------------------------------------------------------------------------

func TestGetSetEffortLevel(t *testing.T) {
	c := newTestClient(t)
	if got := c.GetEffortLevel(); got != "" {
		t.Errorf("initial GetEffortLevel() = %q, want %q", got, "")
	}
	c.SetEffortLevel("high")
	if got := c.GetEffortLevel(); got != "high" {
		t.Errorf("GetEffortLevel() = %q, want %q", got, "high")
	}
}

// ---------------------------------------------------------------------------
// normalizeMessages — prevents "content: Input should be a valid list" errors
// ---------------------------------------------------------------------------

func TestNormalizeMessages_AlreadyValidArray(t *testing.T) {
	msgs := []Message{makeMsg("user", "hello")}
	original := string(msgs[0].Content)
	normalizeMessages(msgs)
	if string(msgs[0].Content) != original {
		t.Errorf("valid array content was modified: got %s, want %s", msgs[0].Content, original)
	}
}

func TestNormalizeMessages_PlainString(t *testing.T) {
	// This is the main bug: json.Marshal("hello") produces `"hello"` (a JSON string)
	content, _ := json.Marshal("hello world")
	msgs := []Message{{Role: "user", Content: content}}
	normalizeMessages(msgs)

	var blocks []map[string]string
	if err := json.Unmarshal(msgs[0].Content, &blocks); err != nil {
		t.Fatalf("after normalize, content is not a valid array: %v (raw: %s)", err, msgs[0].Content)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0]["type"] != "text" || blocks[0]["text"] != "hello world" {
		t.Errorf("unexpected block: %+v", blocks[0])
	}
}

func TestNormalizeMessages_NullContent(t *testing.T) {
	msgs := []Message{{Role: "user", Content: nil}}
	normalizeMessages(msgs)

	var blocks []map[string]string
	if err := json.Unmarshal(msgs[0].Content, &blocks); err != nil {
		t.Fatalf("after normalize, content is not a valid array: %v", err)
	}
	if len(blocks) != 1 || blocks[0]["type"] != "text" {
		t.Errorf("expected single text block, got %+v", blocks)
	}
}

func TestNormalizeMessages_EmptyContent(t *testing.T) {
	msgs := []Message{{Role: "user", Content: json.RawMessage{}}}
	normalizeMessages(msgs)

	var blocks []map[string]string
	if err := json.Unmarshal(msgs[0].Content, &blocks); err != nil {
		t.Fatalf("after normalize, content is not a valid array: %v", err)
	}
}

func TestNormalizeMessages_EmptyContentHasNonEmptyText(t *testing.T) {
	// Regression: empty content must produce a non-empty text block.
	// The API rejects both missing "text" fields and empty "text":"" values.
	for _, c := range []json.RawMessage{nil, {}} {
		msgs := []Message{{Role: "user", Content: c}}
		normalizeMessages(msgs)

		var blocks []struct {
			Type string  `json:"type"`
			Text *string `json:"text"`
		}
		if err := json.Unmarshal(msgs[0].Content, &blocks); err != nil {
			t.Fatalf("content=%v: unmarshal failed: %v", c, err)
		}
		if len(blocks) != 1 {
			t.Fatalf("content=%v: expected 1 block, got %d", c, len(blocks))
		}
		if blocks[0].Text == nil {
			t.Errorf("content=%v: text field is nil (missing from JSON)", c)
		} else if strings.TrimSpace(*blocks[0].Text) == "" {
			t.Errorf("content=%v: text field is whitespace-only — API rejects this", c)
		}
	}
}

func TestSanitizeContentBlocks_RemovesEmptyText(t *testing.T) {
	// Empty text blocks (missing or empty text) should be stripped.
	input := json.RawMessage(`[{"type":"text"},{"type":"tool_use","id":"tu_1","name":"Bash","input":{}}]`)
	got := sanitizeContentBlocks(input)
	var blocks []map[string]json.RawMessage
	if err := json.Unmarshal(got, &blocks); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block after sanitize, got %d: %s", len(blocks), got)
	}

	// Also test {"type":"text","text":""} and whitespace-only
	input2 := json.RawMessage(`[{"type":"text","text":""},{"type":"text","text":"hello"}]`)
	got2 := sanitizeContentBlocks(input2)
	var blocks2 []map[string]json.RawMessage
	json.Unmarshal(got2, &blocks2)
	if len(blocks2) != 1 {
		t.Fatalf("expected 1 block, got %d: %s", len(blocks2), got2)
	}

	// Whitespace-only text blocks should also be stripped
	input3 := json.RawMessage(`[{"type":"text","text":" \t\n"},{"type":"text","text":"real content"}]`)
	got3 := sanitizeContentBlocks(input3)
	var blocks3 []map[string]json.RawMessage
	json.Unmarshal(got3, &blocks3)
	if len(blocks3) != 1 {
		t.Fatalf("expected 1 block after stripping whitespace-only, got %d: %s", len(blocks3), got3)
	}
}

func TestSanitizeContentBlocks_AllEmptyKeepsPlaceholder(t *testing.T) {
	input := json.RawMessage(`[{"type":"text","text":""}]`)
	got := sanitizeContentBlocks(input)
	var blocks []struct {
		Type string  `json:"type"`
		Text *string `json:"text"`
	}
	if err := json.Unmarshal(got, &blocks); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(blocks) != 1 || blocks[0].Text == nil || *blocks[0].Text == "" {
		t.Errorf("expected non-empty placeholder, got: %s", got)
	}
}

func TestNormalizeMessages_MixedMessages(t *testing.T) {
	// Simulate a realistic conversation with a mix of valid and invalid content
	validContent, _ := json.Marshal([]UserContentBlock{NewTextBlock("valid")})
	stringContent, _ := json.Marshal("I am a plain string")
	msgs := []Message{
		{Role: "user", Content: validContent},
		{Role: "assistant", Content: stringContent},
		{Role: "user", Content: nil},
	}
	normalizeMessages(msgs)

	for i, msg := range msgs {
		var blocks []json.RawMessage
		if err := json.Unmarshal(msg.Content, &blocks); err != nil {
			t.Errorf("message[%d] content is not a valid array after normalize: %v (raw: %s)", i, err, msg.Content)
		}
	}
}

func TestNormalizeMessages_PreservesToolUseBlocks(t *testing.T) {
	// Tool use content is already an array — must not be modified
	content := json.RawMessage(`[{"type":"tool_use","id":"tu_123","name":"bash","input":{"command":"ls"}}]`)
	msgs := []Message{{Role: "assistant", Content: content}}
	normalizeMessages(msgs)
	if string(msgs[0].Content) != string(content) {
		t.Errorf("tool_use array was modified: got %s", msgs[0].Content)
	}
}

func TestNormalizeMessages_PreservesToolResultBlocks(t *testing.T) {
	content := json.RawMessage(`[{"type":"tool_result","tool_use_id":"tu_123","content":"output"}]`)
	msgs := []Message{{Role: "user", Content: content}}
	normalizeMessages(msgs)
	if string(msgs[0].Content) != string(content) {
		t.Errorf("tool_result array was modified: got %s", msgs[0].Content)
	}
}

func TestFilterWhitespaceAssistantMessages(t *testing.T) {
	mkMsg := func(role, content string) Message {
		return Message{Role: role, Content: json.RawMessage(content)}
	}

	msgs := []Message{
		mkMsg("user", `[{"type":"text","text":"hello"}]`),
		mkMsg("assistant", `[{"type":"text","text":"  \n\t  "}]`), // whitespace-only → remove
		mkMsg("user", `[{"type":"text","text":"world"}]`),
	}

	result := filterWhitespaceAssistantMessages(msgs)

	// The whitespace-only assistant message should be removed and the two
	// user messages merged.
	if len(result) != 1 {
		t.Fatalf("expected 1 message after filtering, got %d", len(result))
	}
	if result[0].Role != "user" {
		t.Errorf("expected user message, got %s", result[0].Role)
	}
	// The merged user message should have 2 text blocks
	var blocks []json.RawMessage
	json.Unmarshal(result[0].Content, &blocks)
	if len(blocks) != 2 {
		t.Errorf("expected 2 blocks in merged user message, got %d", len(blocks))
	}
}

func TestFilterWhitespaceAssistantMessages_KeepsNonText(t *testing.T) {
	mkMsg := func(role, content string) Message {
		return Message{Role: role, Content: json.RawMessage(content)}
	}

	// Assistant message with a tool_use block should NOT be filtered
	msgs := []Message{
		mkMsg("user", `[{"type":"text","text":"hi"}]`),
		mkMsg("assistant", `[{"type":"text","text":""},{"type":"tool_use","id":"tu_1","name":"Bash","input":{}}]`),
		mkMsg("user", `[{"type":"tool_result","tool_use_id":"tu_1","content":"ok"}]`),
	}

	result := filterWhitespaceAssistantMessages(msgs)
	if len(result) != 3 {
		t.Fatalf("expected 3 messages (tool_use preserved), got %d", len(result))
	}
}

// ── Context Management tests ─────────────────────────────────────────────────

func TestApplyContextManagement_AddsForAnthropicModels(t *testing.T) {
	c := newTestClient(t)
	req := &MessagesRequest{Model: "claude-sonnet-4-6"}
	c.applyContextManagement(req)

	if req.ContextManagement == nil {
		t.Fatal("expected context_management to be set for Anthropic model")
	}
	if len(req.ContextManagement.Edits) != 1 {
		t.Fatalf("expected 1 edit, got %d", len(req.ContextManagement.Edits))
	}
	edit := req.ContextManagement.Edits[0]
	if edit.Type != "clear_tool_uses_20250919" {
		t.Fatalf("expected clear_tool_uses_20250919, got %s", edit.Type)
	}
	if edit.Keep == nil || edit.Keep.Value != 10 {
		t.Fatalf("expected keep.value=10, got %+v", edit.Keep)
	}
}

// ---------------------------------------------------------------------------
// WithStorage / 401 force-refresh retry
// ---------------------------------------------------------------------------

// streamingSSEBody returns a minimal valid SSE stream for the Anthropic API.
func streamingSSEBody() string {
	return strings.Join([]string{
		`data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-6","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":0}}}`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}`,
		`data: {"type":"content_block_stop","index":0}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":1}}`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")
}

// newClientWithHTTP creates a Client that uses a custom *http.Client (for httptest servers).
func newClientWithHTTP(t *testing.T, baseURL string, store storage.SecureStorage) *Client {
	t.Helper()
	resolver := auth.NewResolver(store)
	opts := []ClientOption{
		WithBaseURL(baseURL),
		WithPromptCaching(false),
		WithStorage(store),
	}
	c := NewClient(resolver, opts...)
	// Replace the internal httpClient to point at our test server.
	c.httpClient = &http.Client{Timeout: 5 * time.Second}
	return c
}

func freshOAuthTokens() *oauth.Tokens {
	return &oauth.Tokens{
		AccessToken:  "access-valid",
		RefreshToken: "refresh-token",
		ExpiresAt:    time.Now().Add(2 * time.Hour),
		Scopes:       []string{"user:inference"},
	}
}

// TestWithStorage_OptionSetsField verifies that WithStorage stores the
// SecureStorage on the Client.
func TestWithStorage_OptionSetsField(t *testing.T) {
	store := storage.NewPlaintextStorage(t.TempDir() + "/creds.json")
	resolver := auth.NewResolver(store)
	c := NewClient(resolver, WithStorage(store))
	if c.storage == nil {
		t.Error("expected c.storage to be set after WithStorage option")
	}
	if c.storage != store {
		t.Error("c.storage is not the store passed to WithStorage")
	}
}

// TestStreamMessages_401_NoStorage verifies that a 401 without a storage
// configured propagates as an error (no retry attempt).
func TestStreamMessages_401_NoStorage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	store := storage.NewPlaintextStorage(t.TempDir() + "/creds.json")
	resolver := auth.NewResolver(store)
	c := NewClient(resolver, WithBaseURL(srv.URL), WithPromptCaching(false))
	c.httpClient = &http.Client{Timeout: 5 * time.Second}

	req := &MessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 100,
		Messages:  []Message{makeMsg("user", "hello")},
	}
	events, errs := c.StreamMessages(context.Background(), req)
	// drain
	for range events {
	}
	err := <-errs
	if err == nil {
		t.Fatal("expected an error for 401 without storage")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 in error, got: %v", err)
	}
}

// TestStreamMessages_401_WithStorage_RefreshSucceeds verifies that when the
// API returns 401 and we have storage with valid tokens, the client refreshes
// and retries. We simulate the refresh by having the test server return 401
// on the first request and 200 on the second (the retry), and we pre-populate
// the store with tokens that won't actually need refreshing (the test OAuth
// server is not real) — in practice the retry works because after the 401 the
// client calls CheckAndRefreshIfNeeded(force=true); here we test the retry
// mechanism by having a server that returns 200 on the second attempt and
// verifying the stream completes successfully.
func TestStreamMessages_401_WithStorage_RetryReceives200(t *testing.T) {
	var requestCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := requestCount.Add(1)
		if n == 1 {
			// First request → 401
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		// Second request (retry) → success
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, streamingSSEBody())
	}))
	defer srv.Close()

	// Pre-populate the store with a valid access token so the resolver can
	// provide auth and CheckAndRefreshIfNeeded finds a token to work with.
	store := storage.NewPlaintextStorage(t.TempDir() + "/creds.json")
	if err := store.SaveTokens(freshOAuthTokens()); err != nil {
		t.Fatalf("SaveTokens: %v", err)
	}

	c := newClientWithHTTP(t, srv.URL, store)

	req := &MessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 100,
		Messages:  []Message{makeMsg("user", "hello")},
	}
	events, errs := c.StreamMessages(context.Background(), req)
	for range events {
	}
	err := <-errs
	if err != nil {
		t.Fatalf("expected no error after retry, got: %v", err)
	}
	if got := int(requestCount.Load()); got != 2 {
		t.Errorf("expected 2 HTTP requests (original + retry), got %d", got)
	}
}

// TestStreamMessages_401_WithStorage_RefreshFails verifies that when the
// server always returns 401 (even on retry), an error is propagated.
func TestStreamMessages_401_WithStorage_RefreshFails(t *testing.T) {
	var requestCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	store := storage.NewPlaintextStorage(t.TempDir() + "/creds.json")
	if err := store.SaveTokens(freshOAuthTokens()); err != nil {
		t.Fatalf("SaveTokens: %v", err)
	}

	c := newClientWithHTTP(t, srv.URL, store)

	req := &MessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 100,
		Messages:  []Message{makeMsg("user", "hello")},
	}
	events, errs := c.StreamMessages(context.Background(), req)
	for range events {
	}
	err := <-errs
	if err == nil {
		t.Fatal("expected an error when all requests return 401")
	}
}

// TestSendMessage_401_WithStorage_RetryReceives200 mirrors the streaming test
// but for SendMessage (non-streaming path).
func TestSendMessage_401_WithStorage_RetryReceives200(t *testing.T) {
	var requestCount atomic.Int32

	successBody := `{
		"id": "msg_1",
		"type": "message",
		"role": "assistant",
		"content": [{"type":"text","text":"hello"}],
		"model": "claude-sonnet-4-6",
		"stop_reason": "end_turn",
		"stop_sequence": null,
		"usage": {"input_tokens": 10, "output_tokens": 5}
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := requestCount.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, successBody)
	}))
	defer srv.Close()

	store := storage.NewPlaintextStorage(t.TempDir() + "/creds.json")
	if err := store.SaveTokens(freshOAuthTokens()); err != nil {
		t.Fatalf("SaveTokens: %v", err)
	}

	c := newClientWithHTTP(t, srv.URL, store)

	req := &MessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 100,
		Messages:  []Message{makeMsg("user", "hello")},
	}
	resp, err := c.SendMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no error after retry, got: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if got := int(requestCount.Load()); got != 2 {
		t.Errorf("expected 2 HTTP requests (original + retry), got %d", got)
	}
}

// TestSendMessage_401_NoStorage verifies 401 without storage is a hard error.
func TestSendMessage_401_NoStorage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	store := storage.NewPlaintextStorage(t.TempDir() + "/creds.json")
	resolver := auth.NewResolver(store)
	c := NewClient(resolver, WithBaseURL(srv.URL), WithPromptCaching(false))
	c.httpClient = &http.Client{Timeout: 5 * time.Second}

	req := &MessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 100,
		Messages:  []Message{makeMsg("user", "hello")},
	}
	_, err := c.SendMessage(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for 401 without storage")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 in error, got: %v", err)
	}
}

func TestApplyContextManagement_SkipsExternalProviders(t *testing.T) {
	c := newTestClient(t)
	c.RegisterProvider("groq", &mockProvider{name: "groq"})
	c.AddModelRoute("llama-*", "groq")

	req := &MessagesRequest{Model: "llama-3.3-70b"}
	c.applyContextManagement(req)

	if req.ContextManagement != nil {
		t.Fatal("context_management should NOT be set for external provider models")
	}
}

func TestApplyContextManagement_DoesNotOverrideExplicit(t *testing.T) {
	c := newTestClient(t)
	explicit := &ContextManagement{Edits: []ContextEdit{{Type: "custom"}}}
	req := &MessagesRequest{Model: "claude-sonnet-4-6", ContextManagement: explicit}
	c.applyContextManagement(req)

	if len(req.ContextManagement.Edits) != 1 || req.ContextManagement.Edits[0].Type != "custom" {
		t.Fatal("explicit context_management should not be overridden")
	}
}

func TestContextManagement_SerializesToJSON(t *testing.T) {
	req := &MessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 100,
		Messages:  []Message{makeMsg("user", "hi")},
		ContextManagement: &ContextManagement{
			Edits: []ContextEdit{
				{
					Type:            "clear_tool_uses_20250919",
					Trigger:         &Trigger{Type: "input_tokens", Value: 80000},
					Keep:            &KeepConfig{Type: "tool_uses", Value: 10},
					ClearToolInputs: true,
				},
			},
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Verify it produces valid JSON with context_management field.
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := parsed["context_management"]; !ok {
		t.Fatal("context_management field missing from serialized JSON")
	}

	// Verify the edits structure.
	var cm ContextManagement
	if err := json.Unmarshal(parsed["context_management"], &cm); err != nil {
		t.Fatalf("failed to parse context_management: %v", err)
	}
	if len(cm.Edits) != 1 || cm.Edits[0].Type != "clear_tool_uses_20250919" {
		t.Fatalf("unexpected edits: %+v", cm.Edits)
	}
	if cm.Edits[0].Trigger.Value != 80000 {
		t.Fatalf("expected threshold 80000, got %d", cm.Edits[0].Trigger.Value)
	}
}
