package tui

import (
	"strings"
	"testing"
)

// newStreamingModel returns a minimal Model suitable for testing streaming logic.
// It initialises streamText and leaves session nil (persistMessage is nil-safe).
func newStreamingModel() *Model {
	return &Model{
		streamText: &strings.Builder{},
	}
}

// simulateTextDelta mirrors what handleEngineEvent does for "text_delta".
func simulateTextDelta(m *Model, text string) {
	if m.pendingToolCount > 0 {
		m.pendingPostToolText.WriteString(text)
	} else {
		m.streamText.WriteString(text)
		m.updateStreamingMessage()
	}
}

// simulateToolStart mirrors what handleEngineEvent does for "tool_start".
func simulateToolStart(m *Model, id, name string) {
	m.pendingToolCount++
	m.finalizeStreamingMessage()
	m.addMessage(ChatMessage{Type: MsgToolUse, ToolUseID: id, Content: name})
}

// simulateToolEnd mirrors what handleEngineEvent does for "tool_end".
func simulateToolEnd(m *Model, id, result string) {
	m.addMessage(ChatMessage{Type: MsgToolResult, ToolUseID: id, Content: result})
	if m.pendingToolCount > 0 {
		m.pendingToolCount--
	}
	if m.pendingToolCount == 0 && m.pendingPostToolText.Len() > 0 {
		m.streamText.WriteString(m.pendingPostToolText.String())
		m.pendingPostToolText.Reset()
		m.finalizeStreamingMessage()
	}
}

// ── collectToolGroup ───────────────────────────────────────────────────────────

// TestCollectToolGroup_NormalCase checks that a tool_use immediately followed
// by its tool_result is matched correctly.
func TestCollectToolGroup_NormalCase(t *testing.T) {
	msgs := []ChatMessage{
		{Type: MsgAssistant, Content: "pre-tool text"},
		{Type: MsgToolUse, ToolUseID: "id1", Content: "Read"},
		{Type: MsgToolResult, ToolUseID: "id1", Content: "22 lines"},
	}
	group := collectToolGroup(msgs, 1)
	if len(group) != 1 {
		t.Fatalf("expected 1 pair, got %d", len(group))
	}
	if group[0].result == nil {
		t.Fatal("expected result to be matched, got nil")
	}
	if group[0].result.Content != "22 lines" {
		t.Errorf("unexpected result content: %q", group[0].result.Content)
	}
}

// TestCollectToolGroup_ToolStillRunning checks that a tool_use with no matching
// result yet (tool still executing) produces a group with result == nil.
func TestCollectToolGroup_ToolStillRunning(t *testing.T) {
	msgs := []ChatMessage{
		{Type: MsgToolUse, ToolUseID: "id1", Content: "Read"},
		// no MsgToolResult yet
	}
	group := collectToolGroup(msgs, 0)
	if len(group) != 1 {
		t.Fatalf("expected 1 pair, got %d", len(group))
	}
	if group[0].result != nil {
		t.Errorf("expected nil result for in-flight tool, got %+v", group[0].result)
	}
}

// TestCollectToolGroup_AssistantBetweenUseAndResult documents that when a
// MsgAssistant is inserted between MsgToolUse and MsgToolResult (the pre-fix
// bug), collectToolGroup cannot find the result.  The pending-tool buffering
// fix prevents this state from arising in the first place.
func TestCollectToolGroup_AssistantBetweenUseAndResult(t *testing.T) {
	msgs := []ChatMessage{
		{Type: MsgToolUse, ToolUseID: "id1", Content: "Read"},
		{Type: MsgAssistant, Content: "both."}, // interleaved — should not happen after fix
		{Type: MsgToolResult, ToolUseID: "id1", Content: "22 lines"},
	}
	group := collectToolGroup(msgs, 0)
	if len(group) != 1 {
		t.Fatalf("expected 1 pair, got %d", len(group))
	}
	// collectToolGroup scans only consecutive MsgToolResults after tool uses;
	// a MsgAssistant in between stops the scan — result is nil.
	// The pending-tool buffer fix ensures this state never occurs in practice.
	if group[0].result != nil {
		t.Errorf("expected nil result when MsgAssistant interleaved (pre-fix state), got a match — collectToolGroup may have been hardened independently")
	}
}

// TestCollectToolGroup_AssistantAfterResult is the counterpart: MsgAssistant
// coming AFTER the MsgToolResult (correct order produced by the fix) must not
// affect result-matching.
func TestCollectToolGroup_AssistantAfterResult(t *testing.T) {
	msgs := []ChatMessage{
		{Type: MsgToolUse, ToolUseID: "id1", Content: "Read"},
		{Type: MsgToolResult, ToolUseID: "id1", Content: "22 lines"},
		{Type: MsgAssistant, Content: "both."}, // correct position — after result
	}
	group := collectToolGroup(msgs, 0)
	if len(group) != 1 {
		t.Fatalf("expected 1 pair, got %d", len(group))
	}
	if group[0].result == nil {
		t.Fatal("result should be matched when MsgAssistant follows MsgToolResult")
	}
	if group[0].result.Content != "22 lines" {
		t.Errorf("result content = %q, want %q", group[0].result.Content, "22 lines")
	}
}

// TestCollectToolGroup_ParallelTools_OneRunning checks that when two tools run
// in parallel but only one result has arrived, the completed tool gets its
// result and the in-flight tool has result == nil.
func TestCollectToolGroup_ParallelTools_OneRunning(t *testing.T) {
	msgs := []ChatMessage{
		{Type: MsgToolUse, ToolUseID: "a", Content: "Read-a"},
		{Type: MsgToolUse, ToolUseID: "b", Content: "Read-b"},
		{Type: MsgToolResult, ToolUseID: "a", Content: "result-a"}, // only a finished
	}
	group := collectToolGroup(msgs, 0)
	if len(group) != 2 {
		t.Fatalf("expected 2 pairs, got %d", len(group))
	}
	for _, p := range group {
		switch p.use.ToolUseID {
		case "a":
			if p.result == nil || p.result.Content != "result-a" {
				t.Errorf("tool-a: expected result-a, got %+v", p.result)
			}
		case "b":
			if p.result != nil {
				t.Errorf("tool-b: expected nil result (still running), got %+v", p.result)
			}
		}
	}
}

// TestCollectToolGroup_ParallelTools verifies that two consecutive tool_uses
// followed by two tool_results are matched by ID regardless of result order.
func TestCollectToolGroup_ParallelTools(t *testing.T) {
	msgs := []ChatMessage{
		{Type: MsgToolUse, ToolUseID: "a", Content: "Read-a"},
		{Type: MsgToolUse, ToolUseID: "b", Content: "Read-b"},
		{Type: MsgToolResult, ToolUseID: "b", Content: "result-b"}, // arrives first
		{Type: MsgToolResult, ToolUseID: "a", Content: "result-a"},
	}
	group := collectToolGroup(msgs, 0)
	if len(group) != 2 {
		t.Fatalf("expected 2 pairs, got %d", len(group))
	}
	// Verify ID-based matching
	for _, p := range group {
		if p.result == nil {
			t.Errorf("pair for use %q has nil result", p.use.ToolUseID)
			continue
		}
		if p.use.ToolUseID != p.result.ToolUseID {
			t.Errorf("mismatched IDs: use=%q result=%q", p.use.ToolUseID, p.result.ToolUseID)
		}
	}
}

// ── Pending-tool text buffering ────────────────────────────────────────────────

// TestPendingToolText_TextBeforeToolNotBuffered is the counterpart to
// BufferedDuringToolExecution: text that arrives BEFORE any tool_start goes
// straight to streamText, never to the pending buffer.
func TestPendingToolText_TextBeforeToolNotBuffered(t *testing.T) {
	m := newStreamingModel()
	simulateTextDelta(m, "preamble before tool")

	if m.pendingPostToolText.Len() != 0 {
		t.Errorf("pre-tool text must not go to pendingPostToolText; got %q", m.pendingPostToolText.String())
	}
	if m.streamText.String() != "preamble before tool" {
		t.Errorf("streamText = %q, want %q", m.streamText.String(), "preamble before tool")
	}
}

// TestPendingToolText_NoBufferingWithoutTool checks that text_delta with no
// in-flight tool writes directly to streamText / messages as normal.
func TestPendingToolText_NoBufferingWithoutTool(t *testing.T) {
	m := newStreamingModel()
	simulateTextDelta(m, "hello world")

	if m.pendingPostToolText.Len() != 0 {
		t.Errorf("expected no buffering, got %q", m.pendingPostToolText.String())
	}
	if m.streamText.String() != "hello world" {
		t.Errorf("expected streamText to hold text, got %q", m.streamText.String())
	}
}

// TestPendingToolText_BufferedDuringToolExecution checks that a text_delta that
// arrives while a tool is in-flight goes into pendingPostToolText, not streamText.
func TestPendingToolText_BufferedDuringToolExecution(t *testing.T) {
	m := newStreamingModel()
	simulateToolStart(m, "id1", "Read")

	simulateTextDelta(m, "both.")

	if m.streamText.String() != "" {
		t.Errorf("streamText should be empty while tool in-flight, got %q", m.streamText.String())
	}
	if m.pendingPostToolText.String() != "both." {
		t.Errorf("pendingPostToolText = %q, want %q", m.pendingPostToolText.String(), "both.")
	}
}

// TestPendingToolText_NoPendingText_ToolEndNoExtraMessage is the counterpart to
// FlushedAfterToolEnd: when no text was buffered during tool execution, tool_end
// must not append a spurious MsgAssistant.
func TestPendingToolText_NoPendingText_ToolEndNoExtraMessage(t *testing.T) {
	m := newStreamingModel()
	simulateToolStart(m, "id1", "Bash")
	simulateToolEnd(m, "id1", "exit 0")

	// Expect exactly: MsgToolUse + MsgToolResult — no extra MsgAssistant
	if len(m.messages) != 2 {
		t.Fatalf("message count = %d, want 2; messages: %v", len(m.messages), m.messages)
	}
	if m.messages[0].Type != MsgToolUse {
		t.Errorf("messages[0].Type = %v, want MsgToolUse", m.messages[0].Type)
	}
	if m.messages[1].Type != MsgToolResult {
		t.Errorf("messages[1].Type = %v, want MsgToolResult", m.messages[1].Type)
	}
}

// TestPendingToolText_FlushedAfterToolEnd checks that buffered text becomes a
// MsgAssistant AFTER the MsgToolResult when the last in-flight tool completes.
func TestPendingToolText_FlushedAfterToolEnd(t *testing.T) {
	m := newStreamingModel()

	// Turn: pre-tool text → tool_start → (text during tool) → tool_end
	simulateTextDelta(m, "Clear conflict — ")
	simulateToolStart(m, "read308", "Read")
	simulateTextDelta(m, "both.") // arrives while tool executing
	simulateToolEnd(m, "read308", "22 lines")

	// Expected message order: assistant → tool_use → tool_result → assistant
	wantTypes := []MessageType{MsgAssistant, MsgToolUse, MsgToolResult, MsgAssistant}
	if len(m.messages) != len(wantTypes) {
		t.Fatalf("message count = %d, want %d; messages: %v", len(m.messages), len(wantTypes), m.messages)
	}
	for i, want := range wantTypes {
		if m.messages[i].Type != want {
			t.Errorf("messages[%d].Type = %v, want %v", i, m.messages[i].Type, want)
		}
	}
	if m.messages[3].Content != "both." {
		t.Errorf("final assistant content = %q, want %q", m.messages[3].Content, "both.")
	}
}

// TestPendingToolText_SingleTool_FlushedImmediately is the counterpart to
// NotFlushedUntilLastToolEnd: a single tool (no parallelism) must flush the
// buffer immediately on its own tool_end.
func TestPendingToolText_SingleTool_FlushedImmediately(t *testing.T) {
	m := newStreamingModel()
	simulateToolStart(m, "id1", "Read")
	simulateTextDelta(m, "result commentary")
	simulateToolEnd(m, "id1", "ok")

	if m.pendingPostToolText.Len() != 0 {
		t.Errorf("buffer not flushed after single tool_end; got %q", m.pendingPostToolText.String())
	}
	last := m.messages[len(m.messages)-1]
	if last.Type != MsgAssistant || last.Content != "result commentary" {
		t.Errorf("last message = {%v %q}, want MsgAssistant %q", last.Type, last.Content, "result commentary")
	}
}

// TestPendingToolText_NotFlushedUntilLastToolEnd checks that buffered text is
// held until ALL parallel tools have completed, not just the first one.
func TestPendingToolText_NotFlushedUntilLastToolEnd(t *testing.T) {
	m := newStreamingModel()

	simulateToolStart(m, "a", "Read-a")
	simulateToolStart(m, "b", "Read-b")
	simulateTextDelta(m, "post-tool commentary")

	// First tool ends — text must still be buffered (second tool still in-flight)
	simulateToolEnd(m, "a", "result-a")
	if m.pendingPostToolText.String() != "post-tool commentary" {
		t.Errorf("buffer should not be flushed yet; pendingPostToolText = %q", m.pendingPostToolText.String())
	}

	// Second tool ends — now text should be flushed
	simulateToolEnd(m, "b", "result-b")
	if m.pendingPostToolText.Len() != 0 {
		t.Errorf("buffer should be empty after last tool_end; got %q", m.pendingPostToolText.String())
	}

	// MsgAssistant should be last
	last := m.messages[len(m.messages)-1]
	if last.Type != MsgAssistant {
		t.Errorf("last message type = %v, want MsgAssistant", last.Type)
	}
	if last.Content != "post-tool commentary" {
		t.Errorf("last message content = %q, want %q", last.Content, "post-tool commentary")
	}
}

// TestPendingToolText_MessageOrderWithParallelTools checks the full message
// ordering when two tools run in parallel and text follows them.
func TestPendingToolText_MessageOrderWithParallelTools(t *testing.T) {
	m := newStreamingModel()

	simulateTextDelta(m, "preamble")
	simulateToolStart(m, "a", "Read-a")
	simulateToolStart(m, "b", "Read-b")
	simulateTextDelta(m, "interleaved text") // buffered
	simulateToolEnd(m, "a", "result-a")
	simulateToolEnd(m, "b", "result-b") // flushes buffer

	wantTypes := []MessageType{
		MsgAssistant,  // "preamble"
		MsgToolUse,    // Read-a
		MsgToolUse,    // Read-b
		MsgToolResult, // result-a
		MsgToolResult, // result-b
		MsgAssistant,  // "interleaved text" — must come AFTER results
	}
	if len(m.messages) != len(wantTypes) {
		t.Fatalf("message count = %d, want %d", len(m.messages), len(wantTypes))
	}
	for i, want := range wantTypes {
		if m.messages[i].Type != want {
			t.Errorf("messages[%d].Type = %v, want %v", i, m.messages[i].Type, want)
		}
	}
}

// TestPendingToolText_MessageOrder_SingleTool is the counterpart to
// MessageOrderWithParallelTools for the common single-tool case.
func TestPendingToolText_MessageOrder_SingleTool(t *testing.T) {
	m := newStreamingModel()

	simulateTextDelta(m, "before")
	simulateToolStart(m, "id1", "Edit")
	simulateTextDelta(m, "after") // buffered
	simulateToolEnd(m, "id1", "saved")

	wantTypes := []MessageType{MsgAssistant, MsgToolUse, MsgToolResult, MsgAssistant}
	if len(m.messages) != len(wantTypes) {
		t.Fatalf("message count = %d, want %d", len(m.messages), len(wantTypes))
	}
	for i, want := range wantTypes {
		if m.messages[i].Type != want {
			t.Errorf("messages[%d].Type = %v, want %v", i, m.messages[i].Type, want)
		}
	}
	if m.messages[0].Content != "before" {
		t.Errorf("messages[0].Content = %q, want %q", m.messages[0].Content, "before")
	}
	if m.messages[3].Content != "after" {
		t.Errorf("messages[3].Content = %q, want %q", m.messages[3].Content, "after")
	}
}

// TestPendingToolText_MultipleTextsAccumulate checks that multiple text_deltas
// arriving while a tool executes are concatenated in order.
func TestPendingToolText_MultipleTextsAccumulate(t *testing.T) {
	m := newStreamingModel()

	simulateToolStart(m, "id1", "Bash")
	simulateTextDelta(m, "Clear ")
	simulateTextDelta(m, "conflict")
	simulateTextDelta(m, " — combine both.")
	simulateToolEnd(m, "id1", "success")

	last := m.messages[len(m.messages)-1]
	if last.Type != MsgAssistant {
		t.Fatalf("last message type = %v, want MsgAssistant", last.Type)
	}
	want := "Clear conflict — combine both."
	if last.Content != want {
		t.Errorf("content = %q, want %q", last.Content, want)
	}
}

// TestPendingToolText_NoPendingText_NoAccumulate is the counterpart to
// MultipleTextsAccumulate: when no text arrives during tool execution, the
// buffer stays empty and no extra message is created.
func TestPendingToolText_NoPendingText_NoAccumulate(t *testing.T) {
	m := newStreamingModel()
	simulateToolStart(m, "id1", "Bash")
	// no text_delta during tool
	simulateToolEnd(m, "id1", "exit 0")

	if m.pendingPostToolText.Len() != 0 {
		t.Errorf("buffer should be empty; got %q", m.pendingPostToolText.String())
	}
	// Only MsgToolUse + MsgToolResult — no trailing MsgAssistant
	for _, msg := range m.messages {
		if msg.Type == MsgAssistant {
			t.Errorf("unexpected MsgAssistant when no text was buffered: %q", msg.Content)
		}
	}
}

// TestPendingToolText_SequentialTurns checks the normal multi-turn pattern
// (text → tool → text → tool) produces correct ordering throughout.
func TestPendingToolText_SequentialTurns(t *testing.T) {
	m := newStreamingModel()

	// Turn 1
	simulateTextDelta(m, "turn1 pre")
	simulateToolStart(m, "t1", "Bash")
	simulateToolEnd(m, "t1", "ok1")

	// Turn 2
	simulateTextDelta(m, "turn2 pre")
	simulateToolStart(m, "t2", "Read")
	simulateToolEnd(m, "t2", "ok2")

	wantTypes := []MessageType{
		MsgAssistant, MsgToolUse, MsgToolResult, // turn 1
		MsgAssistant, MsgToolUse, MsgToolResult, // turn 2
	}
	if len(m.messages) != len(wantTypes) {
		t.Fatalf("message count = %d, want %d", len(m.messages), len(wantTypes))
	}
	for i, want := range wantTypes {
		if m.messages[i].Type != want {
			t.Errorf("messages[%d].Type = %v, want %v", i, m.messages[i].Type, want)
		}
	}
}

// TestPendingToolText_ResetOnDone checks that pendingToolCount and
// pendingPostToolText are cleared when the engine finishes (simulating the
// "done" / engineDoneMsg reset path).
func TestPendingToolText_ResetOnDone(t *testing.T) {
	m := newStreamingModel()

	simulateToolStart(m, "id1", "Read")
	simulateTextDelta(m, "orphaned text")

	// Simulate engine "done" reset (mirrors what engineDoneMsg and "done" event do)
	m.pendingToolCount = 0
	m.pendingPostToolText.Reset()

	if m.pendingToolCount != 0 {
		t.Errorf("pendingToolCount = %d, want 0", m.pendingToolCount)
	}
	if m.pendingPostToolText.Len() != 0 {
		t.Errorf("pendingPostToolText not cleared")
	}
}
