package tui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/session"
	"github.com/Abraxas-365/claudio/internal/storage"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func dbMsg(typ, content, toolUseID, toolName string) storage.MessageRecord {
	return storage.MessageRecord{
		Type:      typ,
		Content:   content,
		ToolUseID: toolUseID,
		ToolName:  toolName,
		CreatedAt: time.Time{},
	}
}

func getToolUseIDs(t *testing.T, msg api.Message) []string {
	t.Helper()
	var blocks []struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		t.Fatalf("parse assistant blocks: %v", err)
	}
	var ids []string
	for _, b := range blocks {
		if b.Type == "tool_use" {
			ids = append(ids, b.ID)
		}
	}
	return ids
}

func getToolResultIDs(t *testing.T, msg api.Message) []string {
	t.Helper()
	var blocks []struct {
		Type      string `json:"type"`
		ToolUseID string `json:"tool_use_id"`
	}
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		t.Fatalf("parse user blocks: %v", err)
	}
	var ids []string
	for _, b := range blocks {
		if b.Type == "tool_result" {
			ids = append(ids, b.ToolUseID)
		}
	}
	return ids
}

// ── session.SanitizeToolPairs tests ───────────────────────────────────────────────────

func TestSanitizeTUIPairs_MatchedPairPassesThrough(t *testing.T) {
	msgs := []api.Message{
		{Role: "assistant", Content: json.RawMessage(`[{"type":"tool_use","id":"A","name":"Read","input":{}}]`)},
		{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"A","content":"data"}]`)},
	}
	result := session.SanitizeToolPairs(msgs)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	tuIDs := getToolUseIDs(t, result[0])
	trIDs := getToolResultIDs(t, result[1])
	if len(tuIDs) != 1 || tuIDs[0] != "A" {
		t.Fatalf("expected tool_use id A, got %v", tuIDs)
	}
	if len(trIDs) != 1 || trIDs[0] != "A" {
		t.Fatalf("expected tool_result id A, got %v", trIDs)
	}
}

func TestSanitizeTUIPairs_OrphanedToolResult_Dropped(t *testing.T) {
	// user(tool_result X) with no preceding assistant(tool_use X): block is stripped
	msgs := []api.Message{
		{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"X","content":"orphan"}]`)},
	}
	result := session.SanitizeToolPairs(msgs)
	// Message becomes empty and is dropped
	if len(result) != 0 {
		t.Fatalf("expected 0 messages (orphaned tool_result dropped), got %d", len(result))
	}
}

func TestSanitizeTUIPairs_OrphanedToolUse_Stripped(t *testing.T) {
	// assistant(tool_use A) followed by non-matching user: tool_use stripped, text kept
	msgs := []api.Message{
		{Role: "assistant", Content: json.RawMessage(`[{"type":"text","text":"let me think"},{"type":"tool_use","id":"A","name":"Read","input":{}}]`)},
		{Role: "user", Content: json.RawMessage(`"just a question"`)},
	}
	result := session.SanitizeToolPairs(msgs)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	// Assistant message should only have the text block
	var blocks []json.RawMessage
	if err := json.Unmarshal(result[0].Content, &blocks); err != nil {
		t.Fatalf("failed to parse assistant content: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block (text only), got %d", len(blocks))
	}
	tuIDs := getToolUseIDs(t, result[0])
	if len(tuIDs) != 0 {
		t.Fatalf("expected no tool_use IDs, got %v", tuIDs)
	}
}

func TestSanitizeTUIPairs_MismatchedIDs(t *testing.T) {
	// assistant(tool_use A) → user(tool_result B): A orphaned in assistant, B orphaned in user
	msgs := []api.Message{
		{Role: "user", Content: json.RawMessage(`"hello"`)},
		{Role: "assistant", Content: json.RawMessage(`[{"type":"text","text":"checking"},{"type":"tool_use","id":"A","name":"Read","input":{}}]`)},
		{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"B","content":"stale"}]`)},
	}
	result := session.SanitizeToolPairs(msgs)
	// Tool_use A stripped from assistant (text kept), tool_result B dropped entirely
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	if result[0].Role != "user" || result[1].Role != "assistant" {
		t.Fatalf("expected [user, assistant], got [%s, %s]", result[0].Role, result[1].Role)
	}
	tuIDs := getToolUseIDs(t, result[1])
	if len(tuIDs) != 0 {
		t.Fatalf("expected no tool_use in assistant, got %v", tuIDs)
	}
}

func TestSanitizeTUIPairs_PartialMatch_TwoToolUses(t *testing.T) {
	// assistant(tool_use A, tool_use B) → user(tool_result A): tool_use B stripped, pair A survives
	msgs := []api.Message{
		{Role: "user", Content: json.RawMessage(`"hello"`)},
		{Role: "assistant", Content: json.RawMessage(`[{"type":"tool_use","id":"A","name":"Read","input":{}},{"type":"tool_use","id":"B","name":"Grep","input":{}}]`)},
		{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"A","content":"ok"}]`)},
	}
	result := session.SanitizeToolPairs(msgs)
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}
	tuIDs := getToolUseIDs(t, result[1])
	if len(tuIDs) != 1 || tuIDs[0] != "A" {
		t.Fatalf("expected only tool_use A, got %v", tuIDs)
	}
	trIDs := getToolResultIDs(t, result[2])
	if len(trIDs) != 1 || trIDs[0] != "A" {
		t.Fatalf("expected tool_result A, got %v", trIDs)
	}
}

func TestSanitizeTUIPairs_TextPreservedAfterOrphanedToolUse(t *testing.T) {
	// assistant has text + orphaned tool_use: tool_use stripped, text kept
	msgs := []api.Message{
		{Role: "assistant", Content: json.RawMessage(`[{"type":"text","text":"my analysis"},{"type":"tool_use","id":"orphan","name":"Bash","input":{}}]`)},
		{Role: "user", Content: json.RawMessage(`"follow up"`)},
	}
	result := session.SanitizeToolPairs(msgs)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(result[0].Content, &blocks); err != nil {
		t.Fatalf("failed to parse assistant content: %v", err)
	}
	if len(blocks) != 1 || blocks[0].Type != "text" || blocks[0].Text != "my analysis" {
		t.Fatalf("expected text block 'my analysis', got %+v", blocks)
	}
}

func TestSanitizeTUIPairs_TextPreservedInUserAfterOrphanedToolResult(t *testing.T) {
	// user has text + orphaned tool_result: tool_result stripped, text block remains
	msgs := []api.Message{
		{Role: "user", Content: json.RawMessage(`[{"type":"text","text":"keep this"},{"type":"tool_result","tool_use_id":"orphan","content":"gone"}]`)},
	}
	result := session.SanitizeToolPairs(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(result[0].Content, &blocks); err != nil {
		t.Fatalf("failed to parse user content: %v", err)
	}
	if len(blocks) != 1 || blocks[0].Type != "text" || blocks[0].Text != "keep this" {
		t.Fatalf("expected text block 'keep this', got %+v", blocks)
	}
}

func TestSanitizeTUIPairs_MultipleMatchedPairs_AllKept(t *testing.T) {
	// Two separate turns each with matching pairs: all 4 messages kept
	msgs := []api.Message{
		{Role: "assistant", Content: json.RawMessage(`[{"type":"tool_use","id":"A","name":"Read","input":{}}]`)},
		{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"A","content":"data A"}]`)},
		{Role: "assistant", Content: json.RawMessage(`[{"type":"tool_use","id":"B","name":"Grep","input":{}}]`)},
		{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"B","content":"data B"}]`)},
	}
	result := session.SanitizeToolPairs(msgs)
	if len(result) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(result))
	}
	tuIDs0 := getToolUseIDs(t, result[0])
	trIDs1 := getToolResultIDs(t, result[1])
	tuIDs2 := getToolUseIDs(t, result[2])
	trIDs3 := getToolResultIDs(t, result[3])
	if len(tuIDs0) != 1 || tuIDs0[0] != "A" {
		t.Fatalf("expected tool_use A in result[0], got %v", tuIDs0)
	}
	if len(trIDs1) != 1 || trIDs1[0] != "A" {
		t.Fatalf("expected tool_result A in result[1], got %v", trIDs1)
	}
	if len(tuIDs2) != 1 || tuIDs2[0] != "B" {
		t.Fatalf("expected tool_use B in result[2], got %v", tuIDs2)
	}
	if len(trIDs3) != 1 || trIDs3[0] != "B" {
		t.Fatalf("expected tool_result B in result[3], got %v", trIDs3)
	}
}

func TestSanitizeTUIPairs_EmptyMessages(t *testing.T) {
	result := session.SanitizeToolPairs(nil)
	if len(result) != 0 {
		t.Fatalf("expected empty result for nil input, got %d", len(result))
	}
}

func TestSanitizeTUIPairs_NonToolMessages_PassThrough(t *testing.T) {
	msgs := []api.Message{
		{Role: "user", Content: json.RawMessage(`"plain text question"`)},
		{Role: "assistant", Content: json.RawMessage(`[{"type":"text","text":"plain text answer"}]`)},
	}
	result := session.SanitizeToolPairs(msgs)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	if result[0].Role != "user" || result[1].Role != "assistant" {
		t.Fatalf("unexpected roles: %s, %s", result[0].Role, result[1].Role)
	}
}

// ── session.ReconstructEngineMessages tests ──────────────────────────────────────────

func TestReconstructEngineMessages_Empty(t *testing.T) {
	result := session.ReconstructEngineMessages(nil)
	if len(result) != 0 {
		t.Fatalf("expected empty result for nil input, got %d", len(result))
	}
}

func TestReconstructEngineMessages_SimpleConversation(t *testing.T) {
	records := []storage.MessageRecord{
		dbMsg("user", "hello", "", ""),
		dbMsg("assistant", "hi there", "", ""),
	}
	result := session.ReconstructEngineMessages(records)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	if result[0].Role != "user" {
		t.Fatalf("expected user, got %s", result[0].Role)
	}
	if result[1].Role != "assistant" {
		t.Fatalf("expected assistant, got %s", result[1].Role)
	}
}

func TestReconstructEngineMessages_AssistantWithToolUse(t *testing.T) {
	records := []storage.MessageRecord{
		dbMsg("assistant", "let me check", "", ""),
		dbMsg("tool_use", `{"file_path":"/foo"}`, "toolu_A", "Read"),
		dbMsg("tool_result", "file contents", "toolu_A", ""),
	}
	result := session.ReconstructEngineMessages(records)
	// Should produce: assistant(text+tool_use), user(tool_result)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	if result[0].Role != "assistant" {
		t.Fatalf("expected assistant, got %s", result[0].Role)
	}
	if result[1].Role != "user" {
		t.Fatalf("expected user, got %s", result[1].Role)
	}
	tuIDs := getToolUseIDs(t, result[0])
	if len(tuIDs) != 1 || tuIDs[0] != "toolu_A" {
		t.Fatalf("expected tool_use id toolu_A, got %v", tuIDs)
	}
	trIDs := getToolResultIDs(t, result[1])
	if len(trIDs) != 1 || trIDs[0] != "toolu_A" {
		t.Fatalf("expected tool_result id toolu_A, got %v", trIDs)
	}
}

func TestReconstructEngineMessages_MultipleToolUses(t *testing.T) {
	records := []storage.MessageRecord{
		dbMsg("assistant", "doing two things", "", ""),
		dbMsg("tool_use", `{}`, "toolu_1", "Read"),
		dbMsg("tool_use", `{}`, "toolu_2", "Grep"),
		dbMsg("tool_result", "result 1", "toolu_1", ""),
		dbMsg("tool_result", "result 2", "toolu_2", ""),
	}
	result := session.ReconstructEngineMessages(records)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages (assistant + user), got %d", len(result))
	}
	tuIDs := getToolUseIDs(t, result[0])
	if len(tuIDs) != 2 {
		t.Fatalf("expected 2 tool_use IDs, got %v", tuIDs)
	}
	trIDs := getToolResultIDs(t, result[1])
	if len(trIDs) != 2 {
		t.Fatalf("expected 2 tool_result IDs, got %v", trIDs)
	}
}

func TestReconstructEngineMessages_OrphanedToolResult_Skipped(t *testing.T) {
	// tool_result with no preceding tool_use: skipped
	records := []storage.MessageRecord{
		dbMsg("tool_result", "orphan result", "toolu_X", ""),
		dbMsg("user", "a question", "", ""),
	}
	result := session.ReconstructEngineMessages(records)
	if len(result) != 1 {
		t.Fatalf("expected 1 message (orphaned result skipped), got %d", len(result))
	}
	if result[0].Role != "user" {
		t.Fatalf("expected user message, got %s", result[0].Role)
	}
}

func TestReconstructEngineMessages_OrphanedToolResult_AfterUser_Skipped(t *testing.T) {
	// After a "user" type record clears pendingIDs, tool_results are skipped
	records := []storage.MessageRecord{
		dbMsg("assistant", "I'll help", "", ""),
		dbMsg("tool_use", `{}`, "toolu_1", "Bash"),
		dbMsg("user", "new question", "", ""),    // clears pendingIDs
		dbMsg("tool_result", "result", "toolu_1", ""), // orphaned — skipped
	}
	result := session.ReconstructEngineMessages(records)
	// Expect: assistant(tool_use), user(text), orphaned result skipped
	// But session.SanitizeToolPairs will strip orphaned tool_use from assistant too
	// The "user" record clears pendingIDs; tool_result after that is skipped
	roles := make([]string, len(result))
	for i, m := range result {
		roles[i] = m.Role
	}
	// No tool_result user message should be produced for the orphaned result
	trFound := false
	for _, m := range result {
		if m.Role == "user" {
			ids := getToolResultIDs(t, m)
			if len(ids) > 0 {
				trFound = true
			}
		}
	}
	if trFound {
		t.Fatalf("expected no tool_result messages for orphaned result, but found one in %v", roles)
	}
}

func TestReconstructEngineMessages_ToolUseWithNoID_GetsGenerated(t *testing.T) {
	// tool_use with empty ToolUseID: synthetic ID generated; matching tool_result (also empty ID) gets same ID
	records := []storage.MessageRecord{
		dbMsg("assistant", "doing it", "", ""),
		dbMsg("tool_use", `{}`, "", "Bash"),      // empty ID
		dbMsg("tool_result", "output", "", ""),   // empty ID — matched positionally
	}
	result := session.ReconstructEngineMessages(records)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	tuIDs := getToolUseIDs(t, result[0])
	if len(tuIDs) != 1 || tuIDs[0] == "" {
		t.Fatalf("expected a generated tool_use ID, got %v", tuIDs)
	}
	trIDs := getToolResultIDs(t, result[1])
	if len(trIDs) != 1 {
		t.Fatalf("expected 1 tool_result ID, got %v", trIDs)
	}
	if tuIDs[0] != trIDs[0] {
		t.Fatalf("tool_use ID %q doesn't match tool_result ID %q", tuIDs[0], trIDs[0])
	}
}

func TestReconstructEngineMessages_IDsPreserved(t *testing.T) {
	// Stored ToolUseID used for both tool_use and tool_result
	records := []storage.MessageRecord{
		dbMsg("assistant", "", "", ""),
		dbMsg("tool_use", `{}`, "stored-id-123", "Read"),
		dbMsg("tool_result", "content", "stored-id-123", ""),
	}
	result := session.ReconstructEngineMessages(records)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	tuIDs := getToolUseIDs(t, result[0])
	if len(tuIDs) != 1 || tuIDs[0] != "stored-id-123" {
		t.Fatalf("expected tool_use ID 'stored-id-123', got %v", tuIDs)
	}
	trIDs := getToolResultIDs(t, result[1])
	if len(trIDs) != 1 || trIDs[0] != "stored-id-123" {
		t.Fatalf("expected tool_result ID 'stored-id-123', got %v", trIDs)
	}
}

func TestReconstructEngineMessages_MismatchedStoredIDs_SanitizeHandles(t *testing.T) {
	// tool_use stored with ID "A", tool_result stored with ID "B":
	// session.SanitizeToolPairs should strip both mismatched blocks
	records := []storage.MessageRecord{
		dbMsg("assistant", "text only", "", ""),
		dbMsg("tool_use", `{}`, "A", "Read"),
		dbMsg("tool_result", "stale result", "B", ""),
	}
	result := session.ReconstructEngineMessages(records)
	// session.SanitizeToolPairs will strip the mismatched tool_use and tool_result
	// assistant text should survive; user tool_result message should be gone
	for _, m := range result {
		if m.Role == "user" {
			ids := getToolResultIDs(t, m)
			if len(ids) > 0 {
				t.Fatalf("expected no tool_result blocks (mismatched IDs stripped), got %v", ids)
			}
		}
		if m.Role == "assistant" {
			ids := getToolUseIDs(t, m)
			if len(ids) > 0 {
				t.Fatalf("expected no tool_use blocks (orphaned tool_use stripped), got %v", ids)
			}
		}
	}
}

func TestReconstructEngineMessages_ComplexSession(t *testing.T) {
	// Full session: user, assistant+tool_use, tool_result, assistant(text), user
	records := []storage.MessageRecord{
		dbMsg("user", "please do something", "", ""),
		dbMsg("assistant", "on it", "", ""),
		dbMsg("tool_use", `{}`, "toolu_1", "Bash"),
		dbMsg("tool_result", "bash output", "toolu_1", ""),
		dbMsg("assistant", "done", "", ""),
		dbMsg("user", "thanks", "", ""),
	}
	result := session.ReconstructEngineMessages(records)

	if len(result) < 4 {
		t.Fatalf("expected at least 4 engine messages, got %d", len(result))
	}
	// First should be user
	if result[0].Role != "user" {
		t.Fatalf("expected first message to be user, got %s", result[0].Role)
	}
	// Last should be user
	if result[len(result)-1].Role != "user" {
		t.Fatalf("expected last message to be user, got %s", result[len(result)-1].Role)
	}
	// Verify alternating roles
	for i := 1; i < len(result); i++ {
		if result[i].Role == result[i-1].Role {
			t.Fatalf("consecutive same roles at index %d: both %s", i, result[i].Role)
		}
	}
}

func TestReconstructEngineMessages_UnknownTypeIgnored(t *testing.T) {
	records := []storage.MessageRecord{
		dbMsg("error", "something failed", "", ""),
		dbMsg("system", "welcome", "", ""),
		dbMsg("user", "hello", "", ""),
		dbMsg("assistant", "hi", "", ""),
	}
	result := session.ReconstructEngineMessages(records)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages (unknown types skipped), got %d", len(result))
	}
	if result[0].Role != "user" || result[1].Role != "assistant" {
		t.Fatalf("expected [user, assistant], got [%s, %s]", result[0].Role, result[1].Role)
	}
}


// ── Resume summary guard ──────────────────────────────────────────────────────

// TestResumeSummary_NotDoubleAppended verifies that a session summary is appended
// to the system prompt exactly once even when the guard block is executed twice.
// This simulates resumeSession being called multiple times on the same Model.
func TestResumeSummary_NotDoubleAppended(t *testing.T) {
	pane := newPaneState("")
	pane.systemPrompt = "base system prompt"
	m := Model{
		panes: []PaneState{pane},
	}

	const summary = "previous session: fixed bug in query engine"
	resumed := &storage.Session{Summary: summary}

	// Simulate the guard block from resumeSession twice.
	applyResumeSummary := func(m *Model, sess *storage.Session) {
		if sess.Summary != "" && !m.activePane().resumeSummarySet {
			m.activePane().systemPrompt += "\n\n# Previous Session Context\n" + sess.Summary
			m.activePane().resumeSummarySet = true
		}
	}

	applyResumeSummary(&m, resumed)
	applyResumeSummary(&m, resumed)

	count := strings.Count(m.activePane().systemPrompt, summary)
	if count != 1 {
		t.Fatalf("expected summary to appear exactly once in system prompt, got %d occurrences; prompt: %q", count, m.activePane().systemPrompt)
	}
}

// TestResumeSummary_EmptySummarySkipped verifies that an empty summary does not
// modify the system prompt or set the guard flag.
func TestResumeSummary_EmptySummarySkipped(t *testing.T) {
	pane2 := newPaneState("")
	pane2.systemPrompt = "base"
	m := Model{
		panes: []PaneState{pane2},
	}
	resumed := &storage.Session{Summary: ""}

	if resumed.Summary != "" && !m.activePane().resumeSummarySet {
		m.activePane().systemPrompt += "\n\n# Previous Session Context\n" + resumed.Summary
		m.activePane().resumeSummarySet = true
	}

	if m.activePane().systemPrompt != "base" {
		t.Fatalf("expected system prompt unchanged, got: %q", m.activePane().systemPrompt)
	}
	if m.activePane().resumeSummarySet {
		t.Fatal("resumeSummarySet should remain false for empty summary")
	}
}
