package provider

import (
	"encoding/json"
	"testing"

	"github.com/Abraxas-365/claudio/internal/api"
)

// ---- helpers ----------------------------------------------------------------

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("mustMarshal: %v", err)
	}
	return b
}

func makeMessage(t *testing.T, role string, blocks any) api.Message {
	t.Helper()
	return api.Message{
		Role:    role,
		Content: mustMarshal(t, blocks),
	}
}

func makeStringMessage(t *testing.T, role, text string) api.Message {
	t.Helper()
	return api.Message{
		Role:    role,
		Content: mustMarshal(t, text),
	}
}

func ptrStr(s string) *string { return &s }

// ---- translateRequest -------------------------------------------------------

func TestTranslateRequest_SystemPrompt(t *testing.T) {
	req := &api.MessagesRequest{
		Model:     "test-model",
		MaxTokens: 1024,
		System:    "You are a helpful assistant.",
		Messages: []api.Message{
			makeStringMessage(t, "user", "Hello"),
		},
	}

	data, err := translateRequest(req)
	if err != nil {
		t.Fatalf("translateRequest error: %v", err)
	}

	var oai openAIRequest
	if err := json.Unmarshal(data, &oai); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(oai.Messages) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(oai.Messages))
	}

	first := oai.Messages[0]
	if first.Role != "system" {
		t.Errorf("first message role = %q, want %q", first.Role, "system")
	}
	if first.Content != "You are a helpful assistant." {
		t.Errorf("system content = %v, want 'You are a helpful assistant.'", first.Content)
	}
}

func TestTranslateRequest_SystemRaw_BlockArray(t *testing.T) {
	blocks := []api.SystemBlock{
		{Type: "text", Text: "Block one."},
		{Type: "text", Text: "Block two."},
	}
	raw, _ := json.Marshal(blocks)

	req := &api.MessagesRequest{
		Model:     "test-model",
		MaxTokens: 1024,
		SystemRaw: raw,
		Messages:  []api.Message{makeStringMessage(t, "user", "Hi")},
	}

	data, err := translateRequest(req)
	if err != nil {
		t.Fatalf("translateRequest error: %v", err)
	}

	var oai openAIRequest
	if err := json.Unmarshal(data, &oai); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(oai.Messages) == 0 || oai.Messages[0].Role != "system" {
		t.Fatalf("expected first message to be system, got %+v", oai.Messages)
	}

	wantContent := "Block one.\nBlock two."
	if oai.Messages[0].Content != wantContent {
		t.Errorf("system content = %v, want %q", oai.Messages[0].Content, wantContent)
	}
}

func TestTranslateRequest_Tools(t *testing.T) {
	type anthropicTool struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		InputSchema json.RawMessage `json:"input_schema"`
	}
	schema := json.RawMessage(`{"type":"object","properties":{"x":{"type":"integer"}}}`)
	tools := []anthropicTool{
		{Name: "my_tool", Description: "Does something", InputSchema: schema},
	}
	toolsRaw, _ := json.Marshal(tools)

	req := &api.MessagesRequest{
		Model:     "test-model",
		MaxTokens: 512,
		Tools:     toolsRaw,
		Messages:  []api.Message{makeStringMessage(t, "user", "Use the tool")},
	}

	data, err := translateRequest(req)
	if err != nil {
		t.Fatalf("translateRequest error: %v", err)
	}

	var oai openAIRequest
	if err := json.Unmarshal(data, &oai); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(oai.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(oai.Tools))
	}
	tool := oai.Tools[0]
	if tool.Type != "function" {
		t.Errorf("tool.Type = %q, want %q", tool.Type, "function")
	}
	if tool.Function.Name != "my_tool" {
		t.Errorf("tool name = %q, want %q", tool.Function.Name, "my_tool")
	}
	if tool.Function.Description != "Does something" {
		t.Errorf("tool description = %q, want %q", tool.Function.Description, "Does something")
	}
}

func TestTranslateRequest_StreamOptions(t *testing.T) {
	req := &api.MessagesRequest{
		Model:     "test-model",
		MaxTokens: 512,
		Stream:    true,
		Messages:  []api.Message{makeStringMessage(t, "user", "Ping")},
	}

	data, err := translateRequest(req)
	if err != nil {
		t.Fatalf("translateRequest error: %v", err)
	}

	var oai openAIRequest
	if err := json.Unmarshal(data, &oai); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !oai.Stream {
		t.Error("expected Stream=true")
	}
	if oai.StreamOpts == nil {
		t.Fatal("expected StreamOpts to be set")
	}
	if !oai.StreamOpts.IncludeUsage {
		t.Error("expected IncludeUsage=true")
	}
}

// ---- translateMessage -------------------------------------------------------

func TestTranslateMessage_PlainString(t *testing.T) {
	msg := makeStringMessage(t, "user", "Hello world")

	msgs, err := translateMessage(msg)
	if err != nil {
		t.Fatalf("translateMessage error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Errorf("role = %q, want %q", msgs[0].Role, "user")
	}
	if msgs[0].Content != "Hello world" {
		t.Errorf("content = %v, want %q", msgs[0].Content, "Hello world")
	}
}

// ---- translateAssistantBlocks -----------------------------------------------

func TestTranslateAssistantBlocks_TextOnly(t *testing.T) {
	blocks := []api.ContentBlock{
		{Type: "text", Text: "Hello from assistant"},
	}

	msgs, err := translateAssistantBlocks(blocks)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != "assistant" {
		t.Errorf("role = %q, want assistant", msgs[0].Role)
	}
	if msgs[0].Content != "Hello from assistant" {
		t.Errorf("content = %v", msgs[0].Content)
	}
	if len(msgs[0].ToolCalls) != 0 {
		t.Errorf("expected no tool calls")
	}
}

func TestTranslateAssistantBlocks_WithToolUse(t *testing.T) {
	input := json.RawMessage(`{"key":"value"}`)
	blocks := []api.ContentBlock{
		{Type: "tool_use", ID: "toolu_123", Name: "my_func", Input: input},
	}

	msgs, err := translateAssistantBlocks(blocks)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	msg := msgs[0]
	if msg.Role != "assistant" {
		t.Errorf("role = %q, want assistant", msg.Role)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(msg.ToolCalls))
	}
	tc := msg.ToolCalls[0]
	if tc.ID != "toolu_123" {
		t.Errorf("tool call ID = %q, want toolu_123", tc.ID)
	}
	if tc.Function.Name != "my_func" {
		t.Errorf("function name = %q, want my_func", tc.Function.Name)
	}
	if tc.Function.Arguments == "" {
		t.Error("expected non-empty arguments")
	}
	// Arguments should be the JSON-marshaled input
	if tc.Function.Arguments != `{"key":"value"}` {
		t.Errorf("arguments = %q, want %q", tc.Function.Arguments, `{"key":"value"}`)
	}
}

func TestTranslateAssistantBlocks_SkipsThinkingBlocks(t *testing.T) {
	blocks := []api.ContentBlock{
		{Type: "thinking", Thinking: "Let me think about this..."},
		{Type: "text", Text: "The answer is 42."},
	}

	msgs, err := translateAssistantBlocks(blocks)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message (thinking block skipped), got %d", len(msgs))
	}
	if msgs[0].Content != "The answer is 42." {
		t.Errorf("content = %v", msgs[0].Content)
	}
}

func TestTranslateAssistantBlocks_TextAndToolUse(t *testing.T) {
	input := json.RawMessage(`{"q":"search term"}`)
	blocks := []api.ContentBlock{
		{Type: "text", Text: "I will search for that."},
		{Type: "tool_use", ID: "toolu_456", Name: "search", Input: input},
	}

	msgs, err := translateAssistantBlocks(blocks)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 combined message, got %d", len(msgs))
	}
	msg := msgs[0]
	if msg.Content != "I will search for that." {
		t.Errorf("content = %v", msg.Content)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(msg.ToolCalls))
	}
	if msg.ToolCalls[0].ID != "toolu_456" {
		t.Errorf("tool call ID = %q", msg.ToolCalls[0].ID)
	}
}

func TestTranslateAssistantBlocks_EmptyBlocks(t *testing.T) {
	blocks := []api.ContentBlock{}

	msgs, err := translateAssistantBlocks(blocks)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages for empty blocks, got %d", len(msgs))
	}
}

// ---- translateUserBlocks ----------------------------------------------------

func TestTranslateUserBlocks_TextOnly(t *testing.T) {
	blocks := []api.ContentBlock{
		{Type: "text", Text: "Hello from user"},
	}

	msgs, err := translateUserBlocks(blocks)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Errorf("role = %q, want user", msgs[0].Role)
	}
	if msgs[0].Content != "Hello from user" {
		t.Errorf("content = %v", msgs[0].Content)
	}
}

// TestTranslateUserBlocks_ToolResult_UsesToolUseID is the critical regression
// test: a tool_result block carries its ID in ToolUseID, not ID. The resulting
// OpenAI message must use ToolUseID as the ToolCallID.
func TestTranslateUserBlocks_ToolResult_UsesToolUseID(t *testing.T) {
	blocks := []api.ContentBlock{
		{
			Type:      "tool_result",
			ToolUseID: "toolu_abc",
			// ID intentionally left empty — this is the real shape of tool_result blocks
			Content: "the result",
		},
	}

	msgs, err := translateUserBlocks(blocks)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	var toolMsg *openAIMessage
	for i := range msgs {
		if msgs[i].Role == "tool" {
			toolMsg = &msgs[i]
			break
		}
	}
	if toolMsg == nil {
		t.Fatal("no tool message produced")
	}
	if toolMsg.ToolCallID == "" {
		t.Error("ToolCallID is empty — the fix is not in effect (block.ToolUseID was not used)")
	}
	if toolMsg.ToolCallID != "toolu_abc" {
		t.Errorf("ToolCallID = %q, want %q", toolMsg.ToolCallID, "toolu_abc")
	}
}

// TestTranslateUserBlocks_ToolResult_IDTakesPrecedenceOverToolUseID verifies
// that when both ID and ToolUseID are present, ID wins.
func TestTranslateUserBlocks_ToolResult_IDTakesPrecedenceOverToolUseID(t *testing.T) {
	blocks := []api.ContentBlock{
		{
			Type:      "tool_result",
			ID:        "id_wins",
			ToolUseID: "toolu_loses",
			Content:   "result",
		},
	}

	msgs, err := translateUserBlocks(blocks)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	var toolMsg *openAIMessage
	for i := range msgs {
		if msgs[i].Role == "tool" {
			toolMsg = &msgs[i]
			break
		}
	}
	if toolMsg == nil {
		t.Fatal("no tool message produced")
	}
	if toolMsg.ToolCallID != "id_wins" {
		t.Errorf("ToolCallID = %q, want %q (ID should take precedence)", toolMsg.ToolCallID, "id_wins")
	}
}

func TestTranslateUserBlocks_MixedTextAndToolResult(t *testing.T) {
	blocks := []api.ContentBlock{
		{Type: "text", Text: "Here is the tool result:"},
		{Type: "tool_result", ToolUseID: "toolu_xyz", Content: "result data"},
	}

	msgs, err := translateUserBlocks(blocks)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	// Expect text message flushed first, then tool message
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages (text + tool), got %d", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Errorf("msgs[0].Role = %q, want user", msgs[0].Role)
	}
	if msgs[0].Content != "Here is the tool result:" {
		t.Errorf("msgs[0].Content = %v", msgs[0].Content)
	}
	if msgs[1].Role != "tool" {
		t.Errorf("msgs[1].Role = %q, want tool", msgs[1].Role)
	}
	if msgs[1].ToolCallID != "toolu_xyz" {
		t.Errorf("msgs[1].ToolCallID = %q, want toolu_xyz", msgs[1].ToolCallID)
	}
}

func TestTranslateUserBlocks_MultipleToolResults(t *testing.T) {
	blocks := []api.ContentBlock{
		{Type: "tool_result", ToolUseID: "toolu_1", Content: "result one"},
		{Type: "tool_result", ToolUseID: "toolu_2", Content: "result two"},
	}

	msgs, err := translateUserBlocks(blocks)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 tool messages, got %d", len(msgs))
	}
	if msgs[0].ToolCallID != "toolu_1" {
		t.Errorf("msgs[0].ToolCallID = %q, want toolu_1", msgs[0].ToolCallID)
	}
	if msgs[1].ToolCallID != "toolu_2" {
		t.Errorf("msgs[1].ToolCallID = %q, want toolu_2", msgs[1].ToolCallID)
	}
}

func TestTranslateUserBlocks_ToolResultEmptyContent(t *testing.T) {
	blocks := []api.ContentBlock{
		{Type: "tool_result", ToolUseID: "toolu_empty"},
	}

	msgs, err := translateUserBlocks(blocks)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	var toolMsg *openAIMessage
	for i := range msgs {
		if msgs[i].Role == "tool" {
			toolMsg = &msgs[i]
			break
		}
	}
	if toolMsg == nil {
		t.Fatal("no tool message produced")
	}
	// Content should be empty string, not nil-related panic
	if toolMsg.Content != "" {
		t.Errorf("expected empty content, got %v", toolMsg.Content)
	}
	if toolMsg.ToolCallID != "toolu_empty" {
		t.Errorf("ToolCallID = %q, want toolu_empty", toolMsg.ToolCallID)
	}
}

// ---- extractToolResultContent -----------------------------------------------

// TestExtractToolResultContent_TextField verifies that the Text field is used
// when it is present.
func TestExtractToolResultContent_TextField(t *testing.T) {
	block := api.ContentBlock{
		Type: "tool_result",
		Text: "text wins",
	}
	got := extractToolResultContent(block)
	if got != "text wins" {
		t.Errorf("got %q, want %q", got, "text wins")
	}
}

// TestExtractToolResultContent_ContentField is the critical regression test:
// a ContentBlock with Content set (and empty Text) must return the Content field.
func TestExtractToolResultContent_ContentField(t *testing.T) {
	block := api.ContentBlock{
		Type:    "tool_result",
		Content: "result",
		// Text intentionally empty
	}
	got := extractToolResultContent(block)
	if got != "result" {
		t.Errorf("got %q, want %q — Content field not used", got, "result")
	}
}

// TestExtractToolResultContent_InputFallback covers the case where Input holds a
// JSON array of content blocks (neither Text nor Content is set).
func TestExtractToolResultContent_InputFallback(t *testing.T) {
	inner := []api.ContentBlock{
		{Type: "text", Text: "part one"},
		{Type: "text", Text: "part two"},
	}
	innerJSON, _ := json.Marshal(inner)
	block := api.ContentBlock{
		Type:  "tool_result",
		Input: innerJSON,
	}
	got := extractToolResultContent(block)
	want := "part one\npart two"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractToolResultContent_EmptyBlock(t *testing.T) {
	block := api.ContentBlock{Type: "tool_result"}
	got := extractToolResultContent(block)
	if got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

// ---- translateFinishReason --------------------------------------------------

func TestTranslateFinishReason_Stop(t *testing.T) {
	if got := translateFinishReason("stop"); got != "end_turn" {
		t.Errorf("got %q, want end_turn", got)
	}
}

func TestTranslateFinishReason_ToolCalls(t *testing.T) {
	if got := translateFinishReason("tool_calls"); got != "tool_use" {
		t.Errorf("got %q, want tool_use", got)
	}
}

func TestTranslateFinishReason_Length(t *testing.T) {
	if got := translateFinishReason("length"); got != "max_tokens" {
		t.Errorf("got %q, want max_tokens", got)
	}
}

func TestTranslateFinishReason_Unknown(t *testing.T) {
	if got := translateFinishReason("some_unknown_reason"); got != "end_turn" {
		t.Errorf("got %q, want end_turn (default fallback)", got)
	}
}

// ---- translateNonStreamingResponse ------------------------------------------

func TestTranslateNonStreamingResponse_TextOnly(t *testing.T) {
	text := "Hello there!"
	resp := openAIResponse{
		ID:    "resp_001",
		Model: "gpt-4o",
		Choices: []openAINSChoice{
			{
				FinishReason: "stop",
				Message: openAINSMessage{
					Role:    "assistant",
					Content: ptrStr(text),
				},
			},
		},
		Usage: openAIUsage{PromptTokens: 10, CompletionTokens: 5},
	}

	result := translateNonStreamingResponse(resp)

	if result.ID != "resp_001" {
		t.Errorf("ID = %q, want resp_001", result.ID)
	}
	if result.Role != "assistant" {
		t.Errorf("Role = %q, want assistant", result.Role)
	}
	if result.StopReason != "end_turn" {
		t.Errorf("StopReason = %q, want end_turn", result.StopReason)
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Content))
	}
	if result.Content[0].Type != "text" {
		t.Errorf("content[0].Type = %q, want text", result.Content[0].Type)
	}
	if result.Content[0].Text != text {
		t.Errorf("content[0].Text = %q, want %q", result.Content[0].Text, text)
	}
}

func TestTranslateNonStreamingResponse_WithToolUse(t *testing.T) {
	resp := openAIResponse{
		ID:    "resp_002",
		Model: "gpt-4o",
		Choices: []openAINSChoice{
			{
				FinishReason: "tool_calls",
				Message: openAINSMessage{
					Role: "assistant",
					ToolCalls: []openAIToolCall{
						{
							ID:   "call_abc",
							Type: "function",
							Function: openAIFunctionCall{
								Name:      "my_tool",
								Arguments: `{"x":1}`,
							},
						},
					},
				},
			},
		},
		Usage: openAIUsage{PromptTokens: 20, CompletionTokens: 15},
	}

	result := translateNonStreamingResponse(resp)

	if result.StopReason != "tool_use" {
		t.Errorf("StopReason = %q, want tool_use", result.StopReason)
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block (tool_use), got %d", len(result.Content))
	}
	cb := result.Content[0]
	if cb.Type != "tool_use" {
		t.Errorf("content[0].Type = %q, want tool_use", cb.Type)
	}
	if cb.ID != "call_abc" {
		t.Errorf("content[0].ID = %q, want call_abc", cb.ID)
	}
	if cb.Name != "my_tool" {
		t.Errorf("content[0].Name = %q, want my_tool", cb.Name)
	}
	if string(cb.Input) != `{"x":1}` {
		t.Errorf("content[0].Input = %q, want {\"x\":1}", string(cb.Input))
	}
}

func TestTranslateNonStreamingResponse_Usage(t *testing.T) {
	text := "ok"
	resp := openAIResponse{
		ID:    "resp_003",
		Model: "gpt-4o",
		Choices: []openAINSChoice{
			{
				FinishReason: "stop",
				Message:      openAINSMessage{Role: "assistant", Content: ptrStr(text)},
			},
		},
		Usage: openAIUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
	}

	result := translateNonStreamingResponse(resp)

	if result.Usage.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", result.Usage.InputTokens)
	}
	if result.Usage.OutputTokens != 50 {
		t.Errorf("OutputTokens = %d, want 50", result.Usage.OutputTokens)
	}
}

func TestTranslateNonStreamingResponse_EmptyChoices(t *testing.T) {
	resp := openAIResponse{
		ID:      "resp_004",
		Model:   "gpt-4o",
		Choices: []openAINSChoice{},
		Usage:   openAIUsage{},
	}

	result := translateNonStreamingResponse(resp)

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// With no choices, stop reason defaults to "end_turn"
	if result.StopReason != "end_turn" {
		t.Errorf("StopReason = %q, want end_turn", result.StopReason)
	}
	if len(result.Content) != 0 {
		t.Errorf("expected empty content, got %d blocks", len(result.Content))
	}
}

// ---- translateStreamChunk ---------------------------------------------------

func TestTranslateStreamChunk_TextDelta(t *testing.T) {
	state := newStreamState()
	content := "Hello"
	chunk := openAIStreamChunk{
		ID: "chunk_001",
		Choices: []openAIChoice{
			{
				Index: 0,
				Delta: openAIDelta{Content: ptrStr(content)},
			},
		},
	}

	events := translateStreamChunk(chunk, state)

	// Expect content_block_start followed by content_block_delta
	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %d: %+v", len(events), events)
	}

	startEvt := events[0]
	if startEvt.Type != "content_block_start" {
		t.Errorf("events[0].Type = %q, want content_block_start", startEvt.Type)
	}

	deltaEvt := events[1]
	if deltaEvt.Type != "content_block_delta" {
		t.Errorf("events[1].Type = %q, want content_block_delta", deltaEvt.Type)
	}

	var delta map[string]string
	if err := json.Unmarshal(deltaEvt.Delta, &delta); err != nil {
		t.Fatalf("unmarshal delta: %v", err)
	}
	if delta["type"] != "text_delta" {
		t.Errorf("delta type = %q, want text_delta", delta["type"])
	}
	if delta["text"] != content {
		t.Errorf("delta text = %q, want %q", delta["text"], content)
	}

	// Second call should not emit another content_block_start (text already started)
	chunk2 := openAIStreamChunk{
		Choices: []openAIChoice{
			{Delta: openAIDelta{Content: ptrStr(" world")}},
		},
	}
	events2 := translateStreamChunk(chunk2, state)
	if len(events2) != 1 {
		t.Fatalf("expected 1 event for continued text, got %d", len(events2))
	}
	if events2[0].Type != "content_block_delta" {
		t.Errorf("events2[0].Type = %q, want content_block_delta", events2[0].Type)
	}
}

func TestTranslateStreamChunk_NewToolCall(t *testing.T) {
	state := newStreamState()
	chunk := openAIStreamChunk{
		Choices: []openAIChoice{
			{
				Delta: openAIDelta{
					ToolCalls: []openAIToolCall{
						{
							ID:    "call_001",
							Type:  "function",
							Index: 0,
							Function: openAIFunctionCall{
								Name:      "my_tool",
								Arguments: "",
							},
						},
					},
				},
			},
		},
	}

	events := translateStreamChunk(chunk, state)

	// Expect a content_block_start for tool_use
	found := false
	for _, e := range events {
		if e.Type == "content_block_start" {
			found = true
			var cb api.ContentBlock
			if err := json.Unmarshal(e.ContentBlock, &cb); err != nil {
				t.Fatalf("unmarshal ContentBlock: %v", err)
			}
			if cb.Type != "tool_use" {
				t.Errorf("content block type = %q, want tool_use", cb.Type)
			}
			if cb.ID != "call_001" {
				t.Errorf("content block ID = %q, want call_001", cb.ID)
			}
			if cb.Name != "my_tool" {
				t.Errorf("content block Name = %q, want my_tool", cb.Name)
			}
		}
	}
	if !found {
		t.Error("no content_block_start event found for new tool call")
	}
}

func TestTranslateStreamChunk_ToolCallArgsDelta(t *testing.T) {
	state := newStreamState()

	// First chunk: new tool call with ID and name
	chunkStart := openAIStreamChunk{
		Choices: []openAIChoice{
			{
				Delta: openAIDelta{
					ToolCalls: []openAIToolCall{
						{
							ID:    "call_002",
							Type:  "function",
							Index: 0,
							Function: openAIFunctionCall{
								Name:      "search",
								Arguments: "",
							},
						},
					},
				},
			},
		},
	}
	translateStreamChunk(chunkStart, state) // consume start events

	// Second chunk: argument delta
	chunkArgs := openAIStreamChunk{
		Choices: []openAIChoice{
			{
				Delta: openAIDelta{
					ToolCalls: []openAIToolCall{
						{
							Index: 0,
							Function: openAIFunctionCall{
								Arguments: `{"q":"foo"}`,
							},
						},
					},
				},
			},
		},
	}
	events := translateStreamChunk(chunkArgs, state)

	// Expect input_json_delta
	found := false
	for _, e := range events {
		if e.Type == "content_block_delta" {
			var delta map[string]string
			if err := json.Unmarshal(e.Delta, &delta); err != nil {
				t.Fatalf("unmarshal delta: %v", err)
			}
			if delta["type"] == "input_json_delta" {
				found = true
				if delta["partial_json"] != `{"q":"foo"}` {
					t.Errorf("partial_json = %q, want {\"q\":\"foo\"}", delta["partial_json"])
				}
			}
		}
	}
	if !found {
		t.Error("no input_json_delta event found")
	}
}

func TestTranslateStreamChunk_FinishReasonStop(t *testing.T) {
	state := newStreamState()

	// Simulate some prior text
	content := "done"
	priorChunk := openAIStreamChunk{
		Choices: []openAIChoice{
			{Delta: openAIDelta{Content: ptrStr(content)}},
		},
	}
	translateStreamChunk(priorChunk, state)

	// Finish chunk
	reason := "stop"
	finishChunk := openAIStreamChunk{
		Choices: []openAIChoice{
			{
				FinishReason: ptrStr(reason),
				Delta:        openAIDelta{},
			},
		},
	}
	events := translateStreamChunk(finishChunk, state)

	// Should close the text block and emit message_delta with stop_reason
	foundStop := false
	for _, e := range events {
		if e.Type == "message_delta" {
			var delta map[string]string
			if err := json.Unmarshal(e.Delta, &delta); err != nil {
				t.Fatalf("unmarshal delta: %v", err)
			}
			if delta["stop_reason"] == "end_turn" {
				foundStop = true
			}
		}
	}
	if !foundStop {
		t.Error("no message_delta with stop_reason=end_turn found")
	}
}

func TestTranslateStreamChunk_FinishReasonToolCalls(t *testing.T) {
	state := newStreamState()

	reason := "tool_calls"
	chunk := openAIStreamChunk{
		Choices: []openAIChoice{
			{
				FinishReason: ptrStr(reason),
				Delta:        openAIDelta{},
			},
		},
	}
	events := translateStreamChunk(chunk, state)

	foundToolUse := false
	for _, e := range events {
		if e.Type == "message_delta" {
			var delta map[string]string
			if err := json.Unmarshal(e.Delta, &delta); err != nil {
				t.Fatalf("unmarshal delta: %v", err)
			}
			if delta["stop_reason"] == "tool_use" {
				foundToolUse = true
			}
		}
	}
	if !foundToolUse {
		t.Error("no message_delta with stop_reason=tool_use found")
	}
}

func TestTranslateStreamChunk_Usage(t *testing.T) {
	state := newStreamState()
	chunk := openAIStreamChunk{
		Choices: []openAIChoice{},
		Usage: &openAIUsage{
			PromptTokens:     42,
			CompletionTokens: 17,
		},
	}

	events := translateStreamChunk(chunk, state)

	// Expect a message_delta event with usage attached
	foundUsage := false
	for _, e := range events {
		if e.Type == "message_delta" && e.Usage != nil {
			foundUsage = true
			if e.Usage.InputTokens != 42 {
				t.Errorf("InputTokens = %d, want 42", e.Usage.InputTokens)
			}
			if e.Usage.OutputTokens != 17 {
				t.Errorf("OutputTokens = %d, want 17", e.Usage.OutputTokens)
			}
		}
	}
	if !foundUsage {
		t.Error("no message_delta with usage found")
	}
}
