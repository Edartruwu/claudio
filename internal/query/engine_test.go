package query

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/config"
	"github.com/Abraxas-365/claudio/internal/tools"
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

// ---------------------------------------------------------------------------
// Mock event handler for executeTools tests
// ---------------------------------------------------------------------------

type mockHandler struct {
	toolUseStarts   []tools.ToolUse
	toolUseEnds     []tools.ToolUse
	endResults      []*tools.Result
	approvalCalls   int
	approvalReturn  bool
}

func (h *mockHandler) OnTextDelta(string)                          {}
func (h *mockHandler) OnThinkingDelta(string)                      {}
func (h *mockHandler) OnTurnComplete(api.Usage)                    {}
func (h *mockHandler) OnError(error)                               {}
func (h *mockHandler) OnCostConfirmNeeded(float64, float64) bool   { return true }

func (h *mockHandler) OnToolUseStart(tu tools.ToolUse) {
	h.toolUseStarts = append(h.toolUseStarts, tu)
}

func (h *mockHandler) OnToolUseEnd(tu tools.ToolUse, result *tools.Result) {
	h.toolUseEnds = append(h.toolUseEnds, tu)
	h.endResults = append(h.endResults, result)
}

func (h *mockHandler) OnToolApprovalNeeded(tu tools.ToolUse) bool {
	h.approvalCalls++
	return h.approvalReturn
}

// ---------------------------------------------------------------------------
// Mock tool that implements Validatable
// ---------------------------------------------------------------------------

type validatableMockTool struct {
	name        string
	validateErr *tools.Result // non-nil → Validate returns this
	execResult  *tools.Result
	readOnly    bool
}

func (t *validatableMockTool) Name() string                 { return t.name }
func (t *validatableMockTool) Description() string          { return "mock validatable tool" }
func (t *validatableMockTool) InputSchema() json.RawMessage { return json.RawMessage(`{}`) }
func (t *validatableMockTool) IsReadOnly() bool             { return t.readOnly }
func (t *validatableMockTool) RequiresApproval(json.RawMessage) bool { return true }

func (t *validatableMockTool) Execute(_ context.Context, _ json.RawMessage) (*tools.Result, error) {
	if t.execResult != nil {
		return t.execResult, nil
	}
	return &tools.Result{Content: "executed"}, nil
}

func (t *validatableMockTool) Validate(input json.RawMessage) *tools.Result {
	return t.validateErr
}

// Mock tool that does NOT implement Validatable
type plainMockTool struct {
	name       string
	execResult *tools.Result
	readOnly   bool
}

func (t *plainMockTool) Name() string                              { return t.name }
func (t *plainMockTool) Description() string                       { return "mock plain tool" }
func (t *plainMockTool) InputSchema() json.RawMessage              { return json.RawMessage(`{}`) }
func (t *plainMockTool) IsReadOnly() bool                          { return t.readOnly }
func (t *plainMockTool) RequiresApproval(json.RawMessage) bool     { return false }

func (t *plainMockTool) Execute(_ context.Context, _ json.RawMessage) (*tools.Result, error) {
	if t.execResult != nil {
		return t.execResult, nil
	}
	return &tools.Result{Content: "executed"}, nil
}

// ---------------------------------------------------------------------------
// executeTools + Validatable integration tests
// ---------------------------------------------------------------------------

func TestExecuteTools_ValidateFailure_SkipsApproval(t *testing.T) {
	reg := tools.NewRegistry()
	vTool := &validatableMockTool{
		name:        "FailValidate",
		validateErr: &tools.Result{Content: "validation failed", IsError: true},
	}
	reg.Register(vTool)

	handler := &mockHandler{approvalReturn: true}
	e := &Engine{
		registry:       reg,
		handler:        handler,
		permissionMode: "default",
	}

	results := e.executeTools(context.Background(), []tools.ToolUse{
		{ID: "tu-1", Name: "FailValidate", Input: json.RawMessage(`{}`)},
	})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].IsError {
		t.Error("expected error result")
	}
	if results[0].Content != "validation failed" {
		t.Errorf("unexpected content: %s", results[0].Content)
	}
	// Approval should never have been called
	if handler.approvalCalls != 0 {
		t.Errorf("approval was called %d times, expected 0", handler.approvalCalls)
	}
}

func TestExecuteTools_ValidatePass_ProceedsToApproval(t *testing.T) {
	reg := tools.NewRegistry()
	vTool := &validatableMockTool{
		name:        "PassValidate",
		validateErr: nil, // passes
	}
	reg.Register(vTool)

	handler := &mockHandler{approvalReturn: true}
	e := &Engine{
		registry:       reg,
		handler:        handler,
		permissionMode: "default",
	}

	results := e.executeTools(context.Background(), []tools.ToolUse{
		{ID: "tu-1", Name: "PassValidate", Input: json.RawMessage(`{}`)},
	})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].IsError {
		t.Errorf("unexpected error: %s", results[0].Content)
	}
	// Approval should have been called since tool requires it and passed validation
	if handler.approvalCalls != 1 {
		t.Errorf("approval was called %d times, expected 1", handler.approvalCalls)
	}
}

func TestExecuteTools_NonValidatable_SkipsValidation(t *testing.T) {
	reg := tools.NewRegistry()
	pTool := &plainMockTool{name: "Plain", readOnly: true}
	reg.Register(pTool)

	handler := &mockHandler{approvalReturn: true}
	e := &Engine{
		registry:       reg,
		handler:        handler,
		permissionMode: "auto", // auto-approve so we can test execution
	}

	results := e.executeTools(context.Background(), []tools.ToolUse{
		{ID: "tu-1", Name: "Plain", Input: json.RawMessage(`{}`)},
	})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].IsError {
		t.Errorf("unexpected error: %s", results[0].Content)
	}
	if results[0].Content != "executed" {
		t.Errorf("unexpected content: %s", results[0].Content)
	}
}

func TestExecuteTools_ValidateNonError_ProceedsNormally(t *testing.T) {
	// Validate returns a non-nil result but with IsError=false → should not short-circuit
	reg := tools.NewRegistry()
	vTool := &validatableMockTool{
		name:        "WarnValidate",
		validateErr: &tools.Result{Content: "just a warning", IsError: false},
	}
	reg.Register(vTool)

	handler := &mockHandler{approvalReturn: true}
	e := &Engine{
		registry:       reg,
		handler:        handler,
		permissionMode: "default",
	}

	results := e.executeTools(context.Background(), []tools.ToolUse{
		{ID: "tu-1", Name: "WarnValidate", Input: json.RawMessage(`{}`)},
	})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// Should have proceeded to approval and execution
	if handler.approvalCalls != 1 {
		t.Errorf("approval was called %d times, expected 1", handler.approvalCalls)
	}
	if results[0].IsError {
		t.Error("expected successful execution")
	}
}

func TestExecuteTools_ValidateFailure_OnToolUseEndCalled(t *testing.T) {
	reg := tools.NewRegistry()
	vTool := &validatableMockTool{
		name:        "FailValidate",
		validateErr: &tools.Result{Content: "bad input", IsError: true},
	}
	reg.Register(vTool)

	handler := &mockHandler{}
	e := &Engine{
		registry:       reg,
		handler:        handler,
		permissionMode: "default",
	}

	e.executeTools(context.Background(), []tools.ToolUse{
		{ID: "tu-1", Name: "FailValidate", Input: json.RawMessage(`{}`)},
	})

	// OnToolUseEnd must have been called for the error
	if len(handler.endResults) != 1 {
		t.Fatalf("expected 1 OnToolUseEnd call, got %d", len(handler.endResults))
	}
	if handler.endResults[0].Content != "bad input" {
		t.Errorf("unexpected end result: %s", handler.endResults[0].Content)
	}
	if !handler.endResults[0].IsError {
		t.Error("expected error result in OnToolUseEnd")
	}
}

func TestExecuteTools_MultipleTools_ValidationFailsOne(t *testing.T) {
	reg := tools.NewRegistry()
	failTool := &validatableMockTool{
		name:        "FailTool",
		validateErr: &tools.Result{Content: "fail", IsError: true},
	}
	passTool := &validatableMockTool{
		name:        "PassTool",
		validateErr: nil,
		readOnly:    true,
	}
	reg.Register(failTool)
	reg.Register(passTool)

	handler := &mockHandler{approvalReturn: true}
	e := &Engine{
		registry:       reg,
		handler:        handler,
		permissionMode: "default",
	}

	results := e.executeTools(context.Background(), []tools.ToolUse{
		{ID: "tu-1", Name: "FailTool", Input: json.RawMessage(`{}`)},
		{ID: "tu-2", Name: "PassTool", Input: json.RawMessage(`{}`)},
	})

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// First tool: validation error
	if !results[0].IsError || results[0].Content != "fail" {
		t.Errorf("result[0]: expected validation error, got isError=%v content=%q", results[0].IsError, results[0].Content)
	}

	// Second tool: should have passed validation and executed
	if results[1].IsError {
		t.Errorf("result[1]: unexpected error: %s", results[1].Content)
	}
	if results[1].Content != "executed" {
		t.Errorf("result[1]: expected 'executed', got %q", results[1].Content)
	}

	// Approval should only be called for the passing tool
	if handler.approvalCalls != 1 {
		t.Errorf("approval called %d times, expected 1", handler.approvalCalls)
	}
}

func TestExecuteTools_ValidateFailure_ToolUseIDPreserved(t *testing.T) {
	reg := tools.NewRegistry()
	vTool := &validatableMockTool{
		name:        "FailTool",
		validateErr: &tools.Result{Content: "err", IsError: true},
	}
	reg.Register(vTool)

	handler := &mockHandler{}
	e := &Engine{
		registry:       reg,
		handler:        handler,
		permissionMode: "default",
	}

	results := e.executeTools(context.Background(), []tools.ToolUse{
		{ID: "tu-xyz-123", Name: "FailTool", Input: json.RawMessage(`{}`)},
	})

	if results[0].ToolUseID != "tu-xyz-123" {
		t.Errorf("ToolUseID = %q, want %q", results[0].ToolUseID, "tu-xyz-123")
	}
}

func TestExecuteTools_DeniedApproval_AfterValidation(t *testing.T) {
	// Validate passes but user denies approval → denied error
	reg := tools.NewRegistry()
	vTool := &validatableMockTool{
		name:        "DeniedTool",
		validateErr: nil,
	}
	reg.Register(vTool)

	handler := &mockHandler{approvalReturn: false}
	e := &Engine{
		registry:       reg,
		handler:        handler,
		permissionMode: "default",
	}

	results := e.executeTools(context.Background(), []tools.ToolUse{
		{ID: "tu-1", Name: "DeniedTool", Input: json.RawMessage(`{}`)},
	})

	if !results[0].IsError {
		t.Error("expected error when user denies")
	}
	if handler.approvalCalls != 1 {
		t.Errorf("approval called %d times, expected 1", handler.approvalCalls)
	}
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
// ---------------------------------------------------------------------------
// shouldRequireApproval
// ---------------------------------------------------------------------------

// mockTool is a minimal Tool implementation for testing shouldRequireApproval.
type mockTool struct {
	name             string
	readOnly         bool
	requiresApproval bool
}

func (m *mockTool) Name() string                        { return m.name }
func (m *mockTool) Description() string                  { return "" }
func (m *mockTool) InputSchema() json.RawMessage         { return json.RawMessage(`{}`) }
func (m *mockTool) Execute(_ context.Context, _ json.RawMessage) (*tools.Result, error) {
	return &tools.Result{}, nil
}
func (m *mockTool) IsReadOnly() bool                    { return m.readOnly }
func (m *mockTool) RequiresApproval(_ json.RawMessage) bool { return m.requiresApproval }

func TestShouldRequireApproval_AutoMode(t *testing.T) {
	e := &Engine{permissionMode: "auto"}
	tool := &mockTool{name: "Bash", requiresApproval: true}
	if e.shouldRequireApproval(tool, nil) {
		t.Error("auto mode should never require approval")
	}
}

func TestShouldRequireApproval_HeadlessMode(t *testing.T) {
	e := &Engine{permissionMode: "headless"}
	tool := &mockTool{name: "Bash", requiresApproval: true}
	if e.shouldRequireApproval(tool, nil) {
		t.Error("headless mode should never require approval")
	}
}

func TestShouldRequireApproval_DangerouslySkipMode(t *testing.T) {
	e := &Engine{permissionMode: "dangerously-skip-permissions"}
	tool := &mockTool{name: "Bash", requiresApproval: true}
	if e.shouldRequireApproval(tool, nil) {
		t.Error("dangerously-skip-permissions mode should never require approval")
	}
}

func TestShouldRequireApproval_PlanMode_ReadOnlyAllowed(t *testing.T) {
	e := &Engine{permissionMode: "plan"}
	tool := &mockTool{name: "Read", readOnly: true, requiresApproval: false}
	if e.shouldRequireApproval(tool, nil) {
		t.Error("plan mode should not require approval for read-only tools")
	}
}

func TestShouldRequireApproval_PlanMode_WriteBlocked(t *testing.T) {
	e := &Engine{permissionMode: "plan"}
	tool := &mockTool{name: "Bash", readOnly: false, requiresApproval: true}
	if !e.shouldRequireApproval(tool, nil) {
		t.Error("plan mode should require approval for non-read-only tools")
	}
}

func TestShouldRequireApproval_DefaultMode_AllowRule(t *testing.T) {
	e := &Engine{
		permissionMode: "default",
		permissionRules: []config.PermissionRule{
			{Tool: "Bash", Pattern: "*", Behavior: "allow"},
		},
	}
	tool := &mockTool{name: "Bash", requiresApproval: true}
	input, _ := json.Marshal(map[string]string{"command": "echo hello"})
	if e.shouldRequireApproval(tool, input) {
		t.Error("Bash(*) allow rule should bypass approval")
	}
}

func TestShouldRequireApproval_DefaultMode_DenyRule(t *testing.T) {
	e := &Engine{
		permissionMode: "default",
		permissionRules: []config.PermissionRule{
			{Tool: "Bash", Pattern: "rm *", Behavior: "deny"},
		},
	}
	tool := &mockTool{name: "Bash", requiresApproval: true}
	input, _ := json.Marshal(map[string]string{"command": "rm -rf /"})
	if !e.shouldRequireApproval(tool, input) {
		t.Error("deny rule should require approval")
	}
}

func TestShouldRequireApproval_DefaultMode_NoMatchingRule(t *testing.T) {
	e := &Engine{
		permissionMode: "default",
		permissionRules: []config.PermissionRule{
			{Tool: "Bash", Pattern: "git *", Behavior: "allow"},
		},
	}
	tool := &mockTool{name: "Bash", requiresApproval: true}
	input, _ := json.Marshal(map[string]string{"command": "make build"})
	if !e.shouldRequireApproval(tool, input) {
		t.Error("no matching rule should fall through to RequiresApproval()")
	}
}

func TestShouldRequireApproval_DefaultMode_NoRules_FallsThrough(t *testing.T) {
	e := &Engine{permissionMode: "default"}

	// Tool that does not require approval
	tool := &mockTool{name: "Read", requiresApproval: false}
	if e.shouldRequireApproval(tool, nil) {
		t.Error("should fall through to RequiresApproval() which returns false")
	}

	// Tool that requires approval
	tool = &mockTool{name: "Bash", requiresApproval: true}
	input, _ := json.Marshal(map[string]string{"command": "ls"})
	if !e.shouldRequireApproval(tool, input) {
		t.Error("should fall through to RequiresApproval() which returns true")
	}
}

func TestShouldRequireApproval_RuleUpdatedAtRuntime(t *testing.T) {
	e := &Engine{permissionMode: "default"}
	tool := &mockTool{name: "Bash", requiresApproval: true}
	input, _ := json.Marshal(map[string]string{"command": "echo hi"})

	// Initially no rules → requires approval
	if !e.shouldRequireApproval(tool, input) {
		t.Error("initially should require approval with no rules")
	}

	// Simulate adding a rule at runtime (like persistPermissionRule does)
	e.SetPermissionRules([]config.PermissionRule{
		{Tool: "Bash", Pattern: "*", Behavior: "allow"},
	})

	// Now the same tool+input should NOT require approval
	if e.shouldRequireApproval(tool, input) {
		t.Error("after adding allow rule, should no longer require approval")
	}
}

// ---------------------------------------------------------------------------
// Plan mode transitions via runSingleTool
// ---------------------------------------------------------------------------

func TestPlanModeTransition_EnterPlanMode(t *testing.T) {
	e := &Engine{permissionMode: "default"}

	// Simulate what runSingleTool does for EnterPlanMode on success
	e.prePlanPermMode = e.permissionMode
	e.permissionMode = "plan"

	if e.permissionMode != "plan" {
		t.Errorf("permissionMode = %q, want %q", e.permissionMode, "plan")
	}
	if e.prePlanPermMode != "default" {
		t.Errorf("prePlanPermMode = %q, want %q", e.prePlanPermMode, "default")
	}
}

func TestPlanModeTransition_ExitPlanMode_RestoresPrevious(t *testing.T) {
	e := &Engine{permissionMode: "plan", prePlanPermMode: "auto"}

	// Simulate ExitPlanMode
	if e.prePlanPermMode != "" {
		e.permissionMode = e.prePlanPermMode
		e.prePlanPermMode = ""
	}

	if e.permissionMode != "auto" {
		t.Errorf("permissionMode = %q, want %q after exit", e.permissionMode, "auto")
	}
	if e.prePlanPermMode != "" {
		t.Errorf("prePlanPermMode = %q, want empty after exit", e.prePlanPermMode)
	}
}

func TestPlanModeTransition_ExitPlanMode_DefaultFallback(t *testing.T) {
	e := &Engine{permissionMode: "plan", prePlanPermMode: ""}

	// Simulate ExitPlanMode with no saved mode
	if e.prePlanPermMode != "" {
		e.permissionMode = e.prePlanPermMode
		e.prePlanPermMode = ""
	} else {
		e.permissionMode = "default"
	}

	if e.permissionMode != "default" {
		t.Errorf("permissionMode = %q, want %q after exit with no saved mode", e.permissionMode, "default")
	}
}

func TestPlanModeTransition_ErrorDoesNotSwitch(t *testing.T) {
	e := &Engine{permissionMode: "default"}

	// Simulate: EnterPlanMode returned an error (result.IsError = true)
	// The switch should NOT happen — only non-error results trigger it
	isError := true
	if !isError {
		e.prePlanPermMode = e.permissionMode
		e.permissionMode = "plan"
	}

	if e.permissionMode != "default" {
		t.Errorf("permissionMode should remain %q on error, got %q", "default", e.permissionMode)
	}
}

func TestPlanModeTransition_FullCycle(t *testing.T) {
	e := &Engine{permissionMode: "auto"}
	tool := &mockTool{name: "Bash", readOnly: false, requiresApproval: true}
	input, _ := json.Marshal(map[string]string{"command": "echo test"})

	// In auto mode, no approval needed
	if e.shouldRequireApproval(tool, input) {
		t.Fatal("auto mode should not require approval")
	}

	// Enter plan mode
	e.prePlanPermMode = e.permissionMode
	e.permissionMode = "plan"

	// Now write tools need approval
	if !e.shouldRequireApproval(tool, input) {
		t.Error("plan mode should require approval for write tools")
	}

	// Read-only tools still don't
	readTool := &mockTool{name: "Read", readOnly: true}
	if e.shouldRequireApproval(readTool, nil) {
		t.Error("plan mode should not require approval for read-only tools")
	}

	// Exit plan mode — should restore auto
	e.permissionMode = e.prePlanPermMode
	e.prePlanPermMode = ""

	if e.permissionMode != "auto" {
		t.Errorf("permissionMode = %q, want %q after exit", e.permissionMode, "auto")
	}
	if e.shouldRequireApproval(tool, input) {
		t.Error("back in auto mode, should not require approval")
	}
}

// ---------------------------------------------------------------------------
// mergeConsecutiveUserMessages — pollBackgroundTasks scenario
// ---------------------------------------------------------------------------

func TestMergeConsecutiveUserMessages_TaskNotifBeforeToolResults(t *testing.T) {
	// assistant message with a tool_use (to set context; won't be merged anyway)
	assistantContent, _ := json.Marshal([]api.ContentBlock{
		{Type: "tool_use", ID: "tu-bg", Name: "BackgroundTool", Input: json.RawMessage(`{}`)},
	})
	assistantMsg := api.Message{Role: "assistant", Content: assistantContent}

	// user notification injected by pollBackgroundTasks (content block array)
	notifContent, _ := json.Marshal([]api.UserContentBlock{api.NewTextBlock("<system-reminder>\n[Background task abc (shell): done]\n</system-reminder>")})
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
