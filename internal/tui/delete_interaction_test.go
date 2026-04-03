package tui

import (
	"encoding/json"
	"testing"

	"github.com/Abraxas-365/claudio/internal/api"
)

// ── helpers ──────────────────────────────────────────────────

func mkUser(text string) ChatMessage   { return ChatMessage{Type: MsgUser, Content: text} }
func mkAssist(text string) ChatMessage { return ChatMessage{Type: MsgAssistant, Content: text} }
func mkToolUse(name, id string) ChatMessage {
	return ChatMessage{Type: MsgToolUse, ToolName: name, ToolUseID: id, ToolInputRaw: json.RawMessage(`{"a":1}`)}
}
func mkToolResult(id, content string) ChatMessage {
	return ChatMessage{Type: MsgToolResult, ToolUseID: id, Content: content}
}
func mkError(text string) ChatMessage { return ChatMessage{Type: MsgError, Content: text} }
func mkSystem(text string) ChatMessage { return ChatMessage{Type: MsgSystem, Content: text} }

func msgTypes(msgs []ChatMessage) []MessageType {
	out := make([]MessageType, len(msgs))
	for i, m := range msgs {
		out[i] = m.Type
	}
	return out
}

func msgContents(msgs []ChatMessage) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		out[i] = m.Content
	}
	return out
}

func engineRoles(msgs []api.Message) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		out[i] = m.Role
	}
	return out
}

func sliceEq[T comparable](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func newTestModel(msgs []ChatMessage) *Model {
	m := &Model{
		messages:         make([]ChatMessage, len(msgs)),
		pinnedMsgIndices: make(map[int]bool),
		expandedGroups:   make(map[int]bool),
		lastToolGroup:    -1,
	}
	copy(m.messages, msgs)
	return m
}

// ── engineMessagesFromChat tests ─────────────────────────────

func TestEngineMessagesFromChat_Empty(t *testing.T) {
	result := engineMessagesFromChat(nil)
	if len(result) != 0 {
		t.Fatalf("expected 0 engine messages, got %d", len(result))
	}
}

func TestEngineMessagesFromChat_SimpleConversation(t *testing.T) {
	msgs := []ChatMessage{
		mkUser("hello"),
		mkAssist("hi there"),
	}
	result := engineMessagesFromChat(msgs)
	if len(result) != 2 {
		t.Fatalf("expected 2 engine messages, got %d", len(result))
	}
	if result[0].Role != "user" || result[1].Role != "assistant" {
		t.Fatalf("expected user,assistant roles; got %s,%s", result[0].Role, result[1].Role)
	}
}

func TestEngineMessagesFromChat_WithToolCalls(t *testing.T) {
	msgs := []ChatMessage{
		mkUser("do something"),
		mkAssist("I'll use a tool"),
		mkToolUse("Bash", "tu_001"),
		mkToolResult("tu_001", "output"),
		mkAssist("done"),
	}
	result := engineMessagesFromChat(msgs)
	// Expected: user, assistant(text+tool_use), user(tool_result), assistant(text)
	roles := engineRoles(result)
	expected := []string{"user", "assistant", "user", "assistant"}
	if !sliceEq(roles, expected) {
		t.Fatalf("expected roles %v, got %v", expected, roles)
	}
}

func TestEngineMessagesFromChat_MultipleToolCalls(t *testing.T) {
	msgs := []ChatMessage{
		mkUser("do two things"),
		mkAssist("I'll use two tools"),
		mkToolUse("Bash", "tu_001"),
		mkToolUse("Read", "tu_002"),
		mkToolResult("tu_001", "output1"),
		mkToolResult("tu_002", "output2"),
		mkAssist("done"),
	}
	result := engineMessagesFromChat(msgs)
	// assistant(text+2 tool_use), user(2 tool_result), assistant(text)
	roles := engineRoles(result)
	expected := []string{"user", "assistant", "user", "assistant"}
	if !sliceEq(roles, expected) {
		t.Fatalf("expected roles %v, got %v", expected, roles)
	}
}

func TestEngineMessagesFromChat_SkipsSystemAndError(t *testing.T) {
	msgs := []ChatMessage{
		mkSystem("welcome"),
		mkUser("hello"),
		mkError("something failed"),
		mkAssist("hi"),
	}
	result := engineMessagesFromChat(msgs)
	roles := engineRoles(result)
	expected := []string{"user", "assistant"}
	if !sliceEq(roles, expected) {
		t.Fatalf("expected roles %v, got %v", expected, roles)
	}
}

func TestEngineMessagesFromChat_OrphanedToolResults(t *testing.T) {
	// Tool results without preceding tool_use should be skipped
	msgs := []ChatMessage{
		mkUser("hello"),
		mkToolResult("tu_999", "orphan"),
		mkAssist("hi"),
	}
	result := engineMessagesFromChat(msgs)
	roles := engineRoles(result)
	expected := []string{"user", "assistant"}
	if !sliceEq(roles, expected) {
		t.Fatalf("expected roles %v, got %v", expected, roles)
	}
}

func TestEngineMessagesFromChat_ToolUseWithNoID(t *testing.T) {
	msgs := []ChatMessage{
		mkUser("do it"),
		mkAssist("ok"),
		mkToolUse("Bash", ""),       // no ID
		mkToolResult("", "output"),   // no ID
		mkAssist("done"),
	}
	result := engineMessagesFromChat(msgs)
	if len(result) != 4 {
		t.Fatalf("expected 4 engine messages, got %d", len(result))
	}
	// Verify the generated IDs are consistent (tool_use ID matches tool_result ID)
	// Parse the assistant message to get the generated tool_use ID
	var blocks []api.ContentBlock
	if err := json.Unmarshal(result[1].Content, &blocks); err != nil {
		t.Fatalf("failed to unmarshal assistant blocks: %v", err)
	}
	tuID := ""
	for _, b := range blocks {
		if b.Type == "tool_use" {
			tuID = b.ID
		}
	}
	if tuID == "" {
		t.Fatal("expected generated tool_use ID")
	}
	// Parse the tool_result user message
	type trBlock struct {
		Type      string `json:"type"`
		ToolUseID string `json:"tool_use_id"`
	}
	var trs []trBlock
	if err := json.Unmarshal(result[2].Content, &trs); err != nil {
		t.Fatalf("failed to unmarshal tool_result: %v", err)
	}
	if len(trs) != 1 || trs[0].ToolUseID != tuID {
		t.Fatalf("tool_result ID %q doesn't match tool_use ID %q", trs[0].ToolUseID, tuID)
	}
}

func TestEngineMessagesFromChat_MultiTurnConversation(t *testing.T) {
	msgs := []ChatMessage{
		mkUser("q1"),
		mkAssist("a1"),
		mkUser("q2"),
		mkAssist("a2"),
		mkUser("q3"),
		mkAssist("a3"),
	}
	result := engineMessagesFromChat(msgs)
	roles := engineRoles(result)
	expected := []string{"user", "assistant", "user", "assistant", "user", "assistant"}
	if !sliceEq(roles, expected) {
		t.Fatalf("expected roles %v, got %v", expected, roles)
	}
}

// ── deleteInteraction tests ──────────────────────────────────

func TestDeleteInteraction_SimpleConversation(t *testing.T) {
	m := newTestModel([]ChatMessage{
		mkUser("q1"),
		mkAssist("a1"),
		mkUser("q2"),
		mkAssist("a2"),
	})
	// Delete first interaction (cursor on user at index 0)
	m.deleteInteraction(0)
	if len(m.messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(m.messages))
	}
	if m.messages[0].Content != "q2" || m.messages[1].Content != "a2" {
		t.Fatalf("expected [q2, a2], got %v", msgContents(m.messages))
	}
}

func TestDeleteInteraction_CursorOnAssistant(t *testing.T) {
	m := newTestModel([]ChatMessage{
		mkUser("q1"),
		mkAssist("a1"),
		mkUser("q2"),
		mkAssist("a2"),
	})
	// Cursor on assistant message at index 1 — should still delete the whole interaction
	m.deleteInteraction(1)
	if len(m.messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(m.messages))
	}
	if m.messages[0].Content != "q2" {
		t.Fatalf("expected q2 first, got %s", m.messages[0].Content)
	}
}

func TestDeleteInteraction_MiddleInteraction(t *testing.T) {
	m := newTestModel([]ChatMessage{
		mkUser("q1"),
		mkAssist("a1"),
		mkUser("q2"),
		mkAssist("a2"),
		mkUser("q3"),
		mkAssist("a3"),
	})
	// Delete middle interaction
	m.deleteInteraction(2) // cursor on User q2
	contents := msgContents(m.messages)
	expected := []string{"q1", "a1", "q3", "a3"}
	if !sliceEq(contents, expected) {
		t.Fatalf("expected %v, got %v", expected, contents)
	}
}

func TestDeleteInteraction_LastInteraction(t *testing.T) {
	m := newTestModel([]ChatMessage{
		mkUser("q1"),
		mkAssist("a1"),
		mkUser("q2"),
		mkAssist("a2"),
	})
	// Delete last interaction
	m.deleteInteraction(3) // cursor on Asst a2
	if len(m.messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(m.messages))
	}
	if m.messages[0].Content != "q1" {
		t.Fatalf("expected q1 first, got %s", m.messages[0].Content)
	}
}

func TestDeleteInteraction_OnlyInteraction(t *testing.T) {
	m := newTestModel([]ChatMessage{
		mkUser("q1"),
		mkAssist("a1"),
	})
	m.deleteInteraction(0)
	if len(m.messages) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(m.messages))
	}
}

func TestDeleteInteraction_WithToolCalls(t *testing.T) {
	m := newTestModel([]ChatMessage{
		mkUser("q1"),
		mkAssist("thinking"),
		mkToolUse("Bash", "tu_001"),
		mkToolResult("tu_001", "output"),
		mkAssist("done"),
		mkUser("q2"),
		mkAssist("a2"),
	})
	// Delete first interaction (includes tool calls)
	m.deleteInteraction(0)
	if len(m.messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(m.messages))
	}
	if m.messages[0].Content != "q2" {
		t.Fatalf("expected q2 first, got %s", m.messages[0].Content)
	}
}

func TestDeleteInteraction_CursorOnToolUse(t *testing.T) {
	m := newTestModel([]ChatMessage{
		mkUser("q1"),
		mkAssist("thinking"),
		mkToolUse("Bash", "tu_001"),
		mkToolResult("tu_001", "output"),
		mkAssist("done"),
		mkUser("q2"),
		mkAssist("a2"),
	})
	// Cursor on tool_use at index 2 — should delete the whole first interaction
	m.deleteInteraction(2)
	if len(m.messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(m.messages))
	}
	if m.messages[0].Content != "q2" {
		t.Fatalf("expected q2 first, got %s", m.messages[0].Content)
	}
}

func TestDeleteInteraction_CursorOnToolResult(t *testing.T) {
	m := newTestModel([]ChatMessage{
		mkUser("q1"),
		mkAssist("thinking"),
		mkToolUse("Bash", "tu_001"),
		mkToolResult("tu_001", "output"),
		mkAssist("done"),
		mkUser("q2"),
		mkAssist("a2"),
	})
	// Cursor on tool_result at index 3
	m.deleteInteraction(3)
	if len(m.messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(m.messages))
	}
}

func TestDeleteInteraction_OutOfBounds(t *testing.T) {
	m := newTestModel([]ChatMessage{
		mkUser("q1"),
		mkAssist("a1"),
	})
	// Should not panic
	m.deleteInteraction(-1)
	m.deleteInteraction(5)
	if len(m.messages) != 2 {
		t.Fatalf("expected messages unchanged, got %d", len(m.messages))
	}
}

func TestDeleteInteraction_PinnedIndicesShift(t *testing.T) {
	m := newTestModel([]ChatMessage{
		mkUser("q1"),
		mkAssist("a1"),
		mkUser("q2"),
		mkAssist("a2"),
		mkUser("q3"),
		mkAssist("a3"),
	})
	m.pinnedMsgIndices[1] = true // a1 (before deleted range)
	m.pinnedMsgIndices[3] = true // a2 (in deleted range — will be removed)
	m.pinnedMsgIndices[5] = true // a3 (after deleted range — shifts to 3)

	// Delete middle interaction (indices 2,3)
	m.deleteInteraction(2)

	if m.pinnedMsgIndices[1] != true {
		t.Fatal("pin at index 1 should survive")
	}
	// Old index 5 shifts to 3; old index 3 was in the deleted range so it's gone,
	// but the shifted pin from old index 5 now occupies index 3.
	if m.pinnedMsgIndices[3] != true {
		t.Fatal("pin at old index 5 should shift to index 3")
	}
	// Index 5 should no longer exist
	if m.pinnedMsgIndices[5] {
		t.Fatal("pin at old index 5 should not remain at index 5")
	}
}

func TestDeleteInteraction_ExpandedGroupsShift(t *testing.T) {
	m := newTestModel([]ChatMessage{
		mkUser("q1"),
		mkAssist("a1"),
		mkUser("q2"),
		mkAssist("a2"),
		mkUser("q3"),
		mkAssist("a3"),
	})
	m.expandedGroups[0] = true // in deleted range
	m.expandedGroups[4] = true // after deleted range

	// Delete first interaction (indices 0,1)
	m.deleteInteraction(0)

	if m.expandedGroups[0] {
		t.Fatal("expanded group at old index 0 should be removed")
	}
	// Old index 4 should shift to 2
	if m.expandedGroups[2] != true {
		t.Fatal("expanded group at old index 4 should shift to index 2")
	}
}

func TestDeleteInteraction_LastToolGroupShift(t *testing.T) {
	m := newTestModel([]ChatMessage{
		mkUser("q1"),
		mkAssist("a1"),
		mkUser("q2"),
		mkAssist("a2"),
		mkToolUse("Bash", "tu_001"),
		mkToolResult("tu_001", "out"),
	})
	m.lastToolGroup = 4 // tool group in second interaction

	// Delete first interaction (indices 0,1)
	m.deleteInteraction(0)
	// lastToolGroup should shift from 4 to 2
	if m.lastToolGroup != 2 {
		t.Fatalf("expected lastToolGroup=2, got %d", m.lastToolGroup)
	}
}

func TestDeleteInteraction_LastToolGroupInDeletedRange(t *testing.T) {
	m := newTestModel([]ChatMessage{
		mkUser("q1"),
		mkAssist("a1"),
		mkToolUse("Bash", "tu_001"),
		mkToolResult("tu_001", "out"),
		mkUser("q2"),
		mkAssist("a2"),
	})
	m.lastToolGroup = 2 // tool group in first interaction

	// Delete first interaction (indices 0-3)
	m.deleteInteraction(0)
	if m.lastToolGroup != -1 {
		t.Fatalf("expected lastToolGroup=-1, got %d", m.lastToolGroup)
	}
}

func TestDeleteInteraction_EmptyMessages(t *testing.T) {
	m := newTestModel(nil)
	// Should not panic
	m.deleteInteraction(0)
	if len(m.messages) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(m.messages))
	}
}

func TestDeleteInteraction_OrphanAssistantAtStart(t *testing.T) {
	// Edge case: assistant message at index 0 with no preceding user message
	m := newTestModel([]ChatMessage{
		mkAssist("orphan"),
		mkUser("q1"),
		mkAssist("a1"),
	})
	// Cursor on orphan assistant at index 0 — should delete just the orphan
	m.deleteInteraction(0)
	if len(m.messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(m.messages))
	}
	if m.messages[0].Content != "q1" {
		t.Fatalf("expected q1 first, got %s", m.messages[0].Content)
	}
}

func TestDeleteInteraction_SystemMessagesInterspersed(t *testing.T) {
	m := newTestModel([]ChatMessage{
		mkSystem("welcome"),
		mkUser("q1"),
		mkAssist("a1"),
		mkError("oops"),
		mkUser("q2"),
		mkAssist("a2"),
	})
	// Delete first interaction — starts at user index 1, end walks forward past
	// asst(a1) and error(oops) to index 4 (next user). Deletes indices [1,2,3].
	m.deleteInteraction(1)
	// Remaining: [system, q2, a2]
	if len(m.messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(m.messages))
	}
	types := msgTypes(m.messages)
	expectedTypes := []MessageType{MsgSystem, MsgUser, MsgAssistant}
	if !sliceEq(types, expectedTypes) {
		t.Fatalf("expected types %v, got %v", expectedTypes, types)
	}
}

func TestDeleteInteraction_EngineMessagesValid(t *testing.T) {
	// After deleting middle interaction, engine messages must maintain valid alternation
	msgs := []ChatMessage{
		mkUser("q1"),
		mkAssist("a1"),
		mkUser("q2"),
		mkAssist("a2"),
		mkToolUse("Bash", "tu_001"),
		mkToolResult("tu_001", "out"),
		mkAssist("a2-final"),
		mkUser("q3"),
		mkAssist("a3"),
	}
	// Simulate deletion of middle interaction
	// start=2, end=7 (next user is at index 7)
	remaining := append([]ChatMessage{}, msgs[:2]...)
	remaining = append(remaining, msgs[7:]...)

	result := engineMessagesFromChat(remaining)
	roles := engineRoles(result)
	// Must be strictly alternating
	for i := 1; i < len(roles); i++ {
		if roles[i] == roles[i-1] {
			t.Fatalf("consecutive same roles at index %d: %v", i, roles)
		}
	}
	// First must be user
	if len(roles) > 0 && roles[0] != "user" {
		t.Fatalf("first engine message must be user, got %s", roles[0])
	}
}

func TestDeleteInteraction_CursorOnSystemBeforeUser(t *testing.T) {
	// Edge case: cursor on a system msg that is before the first user msg.
	// The walk-back loop won't find a user msg and lands at index 0 (the system msg).
	// The interaction should be: from system msg to next user msg.
	m := newTestModel([]ChatMessage{
		mkSystem("welcome"),
		mkUser("q1"),
		mkAssist("a1"),
	})
	m.deleteInteraction(0)
	// The system msg is at start=0, end walks forward — system is not user, so end goes
	// to index 1 (which IS user), so only the system msg gets deleted.
	if len(m.messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(m.messages))
	}
	if m.messages[0].Content != "q1" {
		t.Fatalf("expected q1 first, got %s", m.messages[0].Content)
	}
}

func TestDeleteInteraction_MultipleToolRounds(t *testing.T) {
	// Interaction with multiple tool call rounds
	m := newTestModel([]ChatMessage{
		mkUser("q1"),
		mkAssist("step1"),
		mkToolUse("Bash", "tu_001"),
		mkToolResult("tu_001", "out1"),
		mkAssist("step2"),
		mkToolUse("Read", "tu_002"),
		mkToolResult("tu_002", "out2"),
		mkAssist("final"),
		mkUser("q2"),
		mkAssist("a2"),
	})
	m.deleteInteraction(0)
	if len(m.messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(m.messages))
	}
	if m.messages[0].Content != "q2" {
		t.Fatalf("expected q2, got %s", m.messages[0].Content)
	}
}
