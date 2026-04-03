package query

import (
	"encoding/json"
	"testing"

	"github.com/Abraxas-365/claudio/internal/api"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func makeUserMsg(text string) api.Message {
	content, _ := json.Marshal([]api.UserContentBlock{api.NewTextBlock(text)})
	return api.Message{Role: "user", Content: content}
}

func makeUserMsgRaw(content string) api.Message {
	return api.Message{Role: "user", Content: json.RawMessage(content)}
}

func makeAssistantMsg(text string) api.Message {
	blocks := []map[string]string{{"type": "text", "text": text}}
	content, _ := json.Marshal(blocks)
	return api.Message{Role: "assistant", Content: content}
}

func makeToolResultMsg(toolUseID, result string) api.Message {
	type tr struct {
		Type      string `json:"type"`
		ToolUseID string `json:"tool_use_id"`
		Content   string `json:"content"`
	}
	content, _ := json.Marshal([]tr{{Type: "tool_result", ToolUseID: toolUseID, Content: result}})
	return api.Message{Role: "user", Content: content}
}

func newTestEngine() *Engine {
	return &Engine{}
}

// ---------------------------------------------------------------------------
// mergeConsecutiveUserMessages
// ---------------------------------------------------------------------------

func TestMergeConsecutiveUserMessages_Empty(t *testing.T) {
	got := mergeConsecutiveUserMessages(nil)
	if len(got) != 0 {
		t.Errorf("expected empty result, got %d messages", len(got))
	}

	got = mergeConsecutiveUserMessages([]api.Message{})
	if len(got) != 0 {
		t.Errorf("expected empty result for empty slice, got %d messages", len(got))
	}
}

func TestMergeConsecutiveUserMessages_Single(t *testing.T) {
	msgs := []api.Message{makeUserMsg("hello")}
	got := mergeConsecutiveUserMessages(msgs)
	if len(got) != 1 {
		t.Fatalf("expected 1 message, got %d", len(got))
	}
	if string(got[0].Content) != string(msgs[0].Content) {
		t.Error("single message content was changed")
	}
}

func TestMergeConsecutiveUserMessages_TwoPlainUsers(t *testing.T) {
	msgs := []api.Message{makeUserMsg("text1"), makeUserMsg("text2")}
	got := mergeConsecutiveUserMessages(msgs)
	if len(got) != 1 {
		t.Fatalf("expected 1 merged message, got %d", len(got))
	}
	if got[0].Role != "user" {
		t.Errorf("merged message role = %q, want %q", got[0].Role, "user")
	}
}

func TestMergeConsecutiveUserMessages_AssistantSeparates(t *testing.T) {
	msgs := []api.Message{
		makeUserMsg("user1"),
		makeAssistantMsg("assistant"),
		makeUserMsg("user2"),
	}
	got := mergeConsecutiveUserMessages(msgs)
	if len(got) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(got))
	}
}

func TestMergeConsecutiveUserMessages_ToolResultInSecond_Prevents(t *testing.T) {
	msgs := []api.Message{
		makeUserMsg("text"),
		makeToolResultMsg("tool-1", "result"),
	}
	got := mergeConsecutiveUserMessages(msgs)
	if len(got) != 2 {
		t.Fatalf("expected 2 messages (no merge), got %d", len(got))
	}
}

func TestMergeConsecutiveUserMessages_ToolResultInFirst_Prevents(t *testing.T) {
	msgs := []api.Message{
		makeToolResultMsg("tool-1", "result"),
		makeUserMsg("text"),
	}
	got := mergeConsecutiveUserMessages(msgs)
	if len(got) != 2 {
		t.Fatalf("expected 2 messages (no merge), got %d", len(got))
	}
}

func TestMergeConsecutiveUserMessages_ThreeConsecutive(t *testing.T) {
	// Three consecutive text user messages: first two get merged, third stays separate.
	msgs := []api.Message{
		makeUserMsg("a"),
		makeUserMsg("b"),
		makeUserMsg("c"),
	}
	got := mergeConsecutiveUserMessages(msgs)
	// The function skips i+1 after merging, so only processes one merge per pass.
	// Result: [merged(a,b), c]
	if len(got) != 2 {
		t.Fatalf("expected 2 messages after merging first pair, got %d", len(got))
	}
}

func TestMergeConsecutiveUserMessages_PlainStringContent(t *testing.T) {
	// user message whose content is a plain JSON string (not an array)
	msg1 := makeUserMsgRaw(`"hello from string"`)
	msg2 := makeUserMsg("world")
	got := mergeConsecutiveUserMessages([]api.Message{msg1, msg2})
	// Neither has tool_result blocks, so they should merge.
	if len(got) != 1 {
		t.Fatalf("expected 1 merged message, got %d", len(got))
	}
}

func TestMergeConsecutiveUserMessages_MergedTextIsCorrect(t *testing.T) {
	msgs := []api.Message{makeUserMsg("text1"), makeUserMsg("text2")}
	got := mergeConsecutiveUserMessages(msgs)
	if len(got) != 1 {
		t.Fatalf("expected 1 message, got %d", len(got))
	}

	// Expect content to be [{"type":"text","text":"text1\ntext2"}]
	expected, _ := json.Marshal([]api.UserContentBlock{api.NewTextBlock("text1\ntext2")})
	if string(got[0].Content) != string(expected) {
		t.Errorf("merged content = %s, want %s", got[0].Content, expected)
	}
}

func TestMergeConsecutiveUserMessages_ToolResults_NotMergedWithText(t *testing.T) {
	msgs := []api.Message{
		makeUserMsg("some text"),
		makeToolResultMsg("tool-42", "tool output"),
	}
	got := mergeConsecutiveUserMessages(msgs)
	if len(got) != 2 {
		t.Fatalf("expected 2 messages (no merge because second has tool_results), got %d", len(got))
	}
}

func TestMergeConsecutiveUserMessages_MultiplePairs(t *testing.T) {
	// [user, user, assistant, user, user] → [merged1, assistant, merged2]
	msgs := []api.Message{
		makeUserMsg("u1"),
		makeUserMsg("u2"),
		makeAssistantMsg("a"),
		makeUserMsg("u3"),
		makeUserMsg("u4"),
	}
	got := mergeConsecutiveUserMessages(msgs)
	if len(got) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(got))
	}
	if got[0].Role != "user" {
		t.Errorf("got[0].Role = %q, want %q", got[0].Role, "user")
	}
	if got[1].Role != "assistant" {
		t.Errorf("got[1].Role = %q, want %q", got[1].Role, "assistant")
	}
	if got[2].Role != "user" {
		t.Errorf("got[2].Role = %q, want %q", got[2].Role, "user")
	}
}

// ---------------------------------------------------------------------------
// hasToolResultBlocks
// ---------------------------------------------------------------------------

func TestHasToolResultBlocks_WithToolResult(t *testing.T) {
	content := json.RawMessage(`[{"type":"tool_result","tool_use_id":"x","content":"y"}]`)
	if !hasToolResultBlocks(content) {
		t.Error("expected true for content with tool_result block")
	}
}

func TestHasToolResultBlocks_TextOnly(t *testing.T) {
	content := json.RawMessage(`[{"type":"text","text":"hello"}]`)
	if hasToolResultBlocks(content) {
		t.Error("expected false for content with only text blocks")
	}
}

func TestHasToolResultBlocks_PlainString(t *testing.T) {
	content := json.RawMessage(`"just a string"`)
	if hasToolResultBlocks(content) {
		t.Error("expected false for plain string content")
	}
}

func TestHasToolResultBlocks_MixedBlocks(t *testing.T) {
	content := json.RawMessage(`[{"type":"text","text":"hi"},{"type":"tool_result","tool_use_id":"x","content":"y"}]`)
	if !hasToolResultBlocks(content) {
		t.Error("expected true for content with mixed blocks including tool_result")
	}
}

func TestHasToolResultBlocks_EmptyArray(t *testing.T) {
	content := json.RawMessage(`[]`)
	if hasToolResultBlocks(content) {
		t.Error("expected false for empty array")
	}
}

func TestHasToolResultBlocks_InvalidJSON(t *testing.T) {
	content := json.RawMessage(`not valid json {{{`)
	if hasToolResultBlocks(content) {
		t.Error("expected false for invalid JSON")
	}
}

// ---------------------------------------------------------------------------
// extractTextContent
// ---------------------------------------------------------------------------

func TestExtractTextContent_PlainString(t *testing.T) {
	content := json.RawMessage(`"hello"`)
	got := extractTextContent(content)
	if got != "hello" {
		t.Errorf("extractTextContent = %q, want %q", got, "hello")
	}
}

func TestExtractTextContent_TextBlock(t *testing.T) {
	content := json.RawMessage(`[{"type":"text","text":"hello"}]`)
	got := extractTextContent(content)
	if got != "hello" {
		t.Errorf("extractTextContent = %q, want %q", got, "hello")
	}
}

func TestExtractTextContent_MultipleTextBlocks(t *testing.T) {
	content := json.RawMessage(`[{"type":"text","text":"line1"},{"type":"text","text":"line2"}]`)
	got := extractTextContent(content)
	want := "line1\nline2"
	if got != want {
		t.Errorf("extractTextContent = %q, want %q", got, want)
	}
}

func TestExtractTextContent_NonTextBlocksIgnored(t *testing.T) {
	content := json.RawMessage(`[{"type":"tool_result","tool_use_id":"x","content":"y"}]`)
	got := extractTextContent(content)
	if got != "" {
		t.Errorf("extractTextContent = %q, want empty string for non-text blocks", got)
	}
}

func TestExtractTextContent_MixedBlocks(t *testing.T) {
	content := json.RawMessage(`[{"type":"text","text":"hello"},{"type":"tool_result","tool_use_id":"x","content":"y"}]`)
	got := extractTextContent(content)
	if got != "hello" {
		t.Errorf("extractTextContent = %q, want %q", got, "hello")
	}
}

func TestExtractTextContent_InvalidJSON(t *testing.T) {
	raw := `not valid json`
	content := json.RawMessage(raw)
	got := extractTextContent(content)
	// Falls back to returning the raw string
	if got != raw {
		t.Errorf("extractTextContent = %q, want raw %q", got, raw)
	}
}

// ---------------------------------------------------------------------------
// saveAssistantMessage
// ---------------------------------------------------------------------------

func TestSaveAssistantMessage_FilterThinkingBlocks(t *testing.T) {
	e := newTestEngine()
	content := []api.ContentBlock{
		{Type: "thinking", Thinking: "internal thoughts"},
		{Type: "text", Text: "visible text"},
		{Type: "tool_use", ID: "tu-1", Name: "MyTool", Input: json.RawMessage(`{"key":"val"}`)},
	}
	e.saveAssistantMessage(content)

	if len(e.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(e.messages))
	}

	var blocks []api.ContentBlock
	if err := json.Unmarshal(e.messages[0].Content, &blocks); err != nil {
		t.Fatalf("failed to unmarshal saved content: %v", err)
	}

	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks (thinking filtered), got %d", len(blocks))
	}
	for _, b := range blocks {
		if b.Type == "thinking" {
			t.Error("thinking block should have been filtered out")
		}
	}
	if blocks[0].Type != "text" {
		t.Errorf("blocks[0].Type = %q, want %q", blocks[0].Type, "text")
	}
	if blocks[1].Type != "tool_use" {
		t.Errorf("blocks[1].Type = %q, want %q", blocks[1].Type, "tool_use")
	}
}

func TestSaveAssistantMessage_EmptyAfterFiltering(t *testing.T) {
	e := newTestEngine()
	content := []api.ContentBlock{
		{Type: "thinking", Thinking: "only thinking here"},
	}
	e.saveAssistantMessage(content)
	if len(e.messages) != 0 {
		t.Errorf("expected 0 messages when all blocks are filtered, got %d", len(e.messages))
	}
}

func TestSaveAssistantMessage_EmptyToolUseInput(t *testing.T) {
	e := newTestEngine()
	// tool_use block with nil Input (zero-value RawMessage)
	content := []api.ContentBlock{
		{Type: "tool_use", ID: "tu-2", Name: "NoInputTool"},
	}
	// Should not panic
	e.saveAssistantMessage(content)
	if len(e.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(e.messages))
	}

	var blocks []api.ContentBlock
	if err := json.Unmarshal(e.messages[0].Content, &blocks); err != nil {
		t.Fatalf("failed to unmarshal content: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	// Input should have been set to {}
	if string(blocks[0].Input) != "{}" {
		t.Errorf("Input = %q, want %q", string(blocks[0].Input), "{}")
	}
}

func TestSaveAssistantMessage_PreservesOrder(t *testing.T) {
	e := newTestEngine()
	content := []api.ContentBlock{
		{Type: "text", Text: "first"},
		{Type: "tool_use", ID: "tu-3", Name: "SomeTool", Input: json.RawMessage(`{}`)},
	}
	e.saveAssistantMessage(content)
	if len(e.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(e.messages))
	}

	var blocks []api.ContentBlock
	if err := json.Unmarshal(e.messages[0].Content, &blocks); err != nil {
		t.Fatalf("failed to unmarshal content: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if blocks[0].Type != "text" {
		t.Errorf("blocks[0].Type = %q, want %q", blocks[0].Type, "text")
	}
	if blocks[0].Text != "first" {
		t.Errorf("blocks[0].Text = %q, want %q", blocks[0].Text, "first")
	}
	if blocks[1].Type != "tool_use" {
		t.Errorf("blocks[1].Type = %q, want %q", blocks[1].Type, "tool_use")
	}
}

// ---------------------------------------------------------------------------
// mergeConsecutiveUserMessages — pollBackgroundTasks scenario
// ---------------------------------------------------------------------------

// TestMergeConsecutiveUserMessages_TaskNotifBeforeToolResults verifies that when a
// background-task notification user message appears between an assistant(tool_use) and
// the subsequent user(tool_results), the merge logic does NOT collapse the notification
// into the tool_results message, because the tool_results message contains tool_result
// blocks.  The three-message sequence [assistant, user(notif), user(tool_results)] is
// preserved as-is.
func TestMergeConsecutiveUserMessages_TaskNotifBeforeToolResults(t *testing.T) {
	// assistant message with a tool_use (to set context; won't be merged anyway)
	assistantContent, _ := json.Marshal([]api.ContentBlock{
		{Type: "tool_use", ID: "tu-bg", Name: "BackgroundTool", Input: json.RawMessage(`{}`)},
	})
	assistantMsg := api.Message{Role: "assistant", Content: assistantContent}

	// user notification injected by pollBackgroundTasks (plain string)
	notifContent, _ := json.Marshal("<system-reminder>\n[Background task abc (shell): done]\n</system-reminder>")
	notifMsg := api.Message{Role: "user", Content: notifContent}

	// user tool_results
	toolResultMsg := makeToolResultMsg("tu-bg", "output from tool")

	msgs := []api.Message{assistantMsg, notifMsg, toolResultMsg}
	got := mergeConsecutiveUserMessages(msgs)

	// notifMsg and toolResultMsg are consecutive users, but toolResultMsg has
	// tool_result blocks → they must NOT be merged.
	if len(got) != 3 {
		t.Fatalf("expected 3 messages (no merge), got %d", len(got))
	}
	if got[0].Role != "assistant" {
		t.Errorf("got[0].Role = %q, want %q", got[0].Role, "assistant")
	}
	if got[1].Role != "user" {
		t.Errorf("got[1].Role = %q, want %q", got[1].Role, "user")
	}
	if got[2].Role != "user" {
		t.Errorf("got[2].Role = %q, want %q", got[2].Role, "user")
	}
	// The tool_result message must remain unchanged so it is correctly paired
	// with the preceding assistant tool_use.
	if !hasToolResultBlocks(got[2].Content) {
		t.Error("got[2] should still contain tool_result blocks")
	}
}
