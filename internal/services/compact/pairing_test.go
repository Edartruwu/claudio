package compact

import (
	"encoding/json"
	"testing"

	"github.com/Abraxas-365/claudio/internal/api"
)

// ── EnsureToolResultPairing tests ────────────────────────────────────────────

func makeToolUseMsg(id, name string) api.Message {
	type tuBlock struct {
		Type  string          `json:"type"`
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	}
	raw, _ := json.Marshal([]tuBlock{{Type: "tool_use", ID: id, Name: name, Input: json.RawMessage(`{}`)}})
	return api.Message{Role: "assistant", Content: raw}
}

func TestEnsureToolResultPairing_NoOpWhenPaired(t *testing.T) {
	msgs := []api.Message{
		makeToolUseMsg("tu-1", "Read"),
		makeTRMsg("tu-1", "file content"),
	}
	result := EnsureToolResultPairing(msgs)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	content := extractTRContent(t, result[1])
	if content != "file content" {
		t.Fatalf("paired result should be unchanged; got: %q", content)
	}
}

func TestEnsureToolResultPairing_SyntheticForMissingResult(t *testing.T) {
	// Assistant has tool_use but no following user message with tool_result.
	msgs := []api.Message{
		makeToolUseMsg("tu-orphan", "Bash"),
	}
	result := EnsureToolResultPairing(msgs)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages (original + synthetic user); got %d", len(result))
	}
	if result[1].Role != "user" {
		t.Fatal("second message should be a synthetic user message")
	}
	content := extractTRContent(t, result[1])
	if content == "" {
		t.Fatal("synthetic result should have content")
	}
}

func TestEnsureToolResultPairing_SyntheticForPartiallyMissing(t *testing.T) {
	// Two tool_uses but only one tool_result.
	tuBlock1 := struct {
		Type  string          `json:"type"`
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	}{Type: "tool_use", ID: "tu-a", Name: "Read", Input: json.RawMessage(`{}`)}
	tuBlock2 := struct {
		Type  string          `json:"type"`
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	}{Type: "tool_use", ID: "tu-b", Name: "Grep", Input: json.RawMessage(`{}`)}

	tuContent, _ := json.Marshal([]any{tuBlock1, tuBlock2})

	msgs := []api.Message{
		{Role: "assistant", Content: tuContent},
		makeTRMsg("tu-a", "result for a"),
	}

	result := EnsureToolResultPairing(msgs)

	// The user message should now have 2 tool_results (original + synthetic for tu-b).
	var blocks []json.RawMessage
	json.Unmarshal(result[1].Content, &blocks)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 tool_result blocks; got %d", len(blocks))
	}

	// Check the synthetic one.
	var tr struct {
		Type      string `json:"type"`
		ToolUseID string `json:"tool_use_id"`
		IsError   bool   `json:"is_error"`
	}
	json.Unmarshal(blocks[1], &tr)
	if tr.ToolUseID != "tu-b" {
		t.Fatalf("synthetic result should be for tu-b; got: %q", tr.ToolUseID)
	}
	if !tr.IsError {
		t.Fatal("synthetic result should be marked as error")
	}
}

func TestEnsureToolResultPairing_RemovesOrphanedResults(t *testing.T) {
	// Assistant has tool_use for "tu-keep" but user message also has a stale
	// orphaned "tu-stale" from a prior compaction.
	msgs := []api.Message{
		makeToolUseMsg("tu-keep", "Read"),
		func() api.Message {
			// Build a user message with both a valid and orphaned tool_result.
			type trBlock struct {
				Type      string `json:"type"`
				ToolUseID string `json:"tool_use_id"`
				Content   string `json:"content"`
			}
			raw, _ := json.Marshal([]trBlock{
				{Type: "tool_result", ToolUseID: "tu-keep", Content: "valid"},
				{Type: "tool_result", ToolUseID: "tu-stale", Content: "orphaned"},
			})
			return api.Message{Role: "user", Content: raw}
		}(),
	}
	result := EnsureToolResultPairing(msgs)

	// The orphaned tu-stale should be stripped; tu-keep should remain.
	var blocks []json.RawMessage
	json.Unmarshal(result[1].Content, &blocks)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 tool_result block after removing orphan; got %d", len(blocks))
	}
	var tr struct{ ToolUseID string `json:"tool_use_id"` }
	json.Unmarshal(blocks[0], &tr)
	if tr.ToolUseID != "tu-keep" {
		t.Fatalf("expected tu-keep; got: %q", tr.ToolUseID)
	}
}

func TestEnsureToolResultPairing_EmptyMessages(t *testing.T) {
	result := EnsureToolResultPairing(nil)
	if len(result) != 0 {
		t.Fatal("empty input should return empty result")
	}
}

func TestEnsureToolResultPairing_AllOrphanedResults(t *testing.T) {
	// Assistant has tool_use "tu-A", but user message has ONLY tool_results for
	// "tu-X" and "tu-Y" (both orphaned — no matching tool_use).
	// This is the primary bug scenario: removeOrphanedToolResults used to return
	// the original message when all blocks were orphaned, leaving invalid IDs.
	type trBlock struct {
		Type      string `json:"type"`
		ToolUseID string `json:"tool_use_id"`
		Content   string `json:"content"`
	}
	userContent, _ := json.Marshal([]trBlock{
		{Type: "tool_result", ToolUseID: "tu-X", Content: "stale X"},
		{Type: "tool_result", ToolUseID: "tu-Y", Content: "stale Y"},
	})
	msgs := []api.Message{
		makeToolUseMsg("tu-A", "Bash"),
		{Role: "user", Content: userContent},
	}

	result := EnsureToolResultPairing(msgs)

	// The user message should contain ONLY a synthetic result for tu-A.
	// The orphaned tu-X and tu-Y must be gone.
	var blocks []json.RawMessage
	json.Unmarshal(result[1].Content, &blocks)

	ids := map[string]bool{}
	for _, b := range blocks {
		var tr struct {
			Type      string `json:"type"`
			ToolUseID string `json:"tool_use_id"`
		}
		json.Unmarshal(b, &tr)
		if tr.Type == "tool_result" {
			ids[tr.ToolUseID] = true
		}
	}
	if ids["tu-X"] || ids["tu-Y"] {
		t.Fatal("orphaned tool_result IDs tu-X/tu-Y should have been stripped")
	}
	if !ids["tu-A"] {
		t.Fatal("synthetic tool_result for tu-A should be present")
	}
}

func TestEnsureToolResultPairing_OrphanedResultsPlusTextBlocks(t *testing.T) {
	// User message has text blocks + orphaned tool_results. After stripping
	// orphans, the text should survive and synthetic results should be added.
	type block struct {
		Type      string `json:"type,omitempty"`
		Text      string `json:"text,omitempty"`
		ToolUseID string `json:"tool_use_id,omitempty"`
		Content   string `json:"content,omitempty"`
	}
	userContent, _ := json.Marshal([]block{
		{Type: "text", Text: "some context"},
		{Type: "tool_result", ToolUseID: "tu-stale", Content: "orphaned"},
	})
	msgs := []api.Message{
		makeToolUseMsg("tu-real", "Read"),
		{Role: "user", Content: userContent},
	}

	result := EnsureToolResultPairing(msgs)

	var blocks []json.RawMessage
	json.Unmarshal(result[1].Content, &blocks)

	hasText := false
	hasReal := false
	hasStale := false
	for _, b := range blocks {
		var generic struct {
			Type      string `json:"type"`
			ToolUseID string `json:"tool_use_id"`
		}
		json.Unmarshal(b, &generic)
		if generic.Type == "text" {
			hasText = true
		}
		if generic.Type == "tool_result" && generic.ToolUseID == "tu-real" {
			hasReal = true
		}
		if generic.Type == "tool_result" && generic.ToolUseID == "tu-stale" {
			hasStale = true
		}
	}
	if !hasText {
		t.Fatal("text block should survive")
	}
	if !hasReal {
		t.Fatal("synthetic result for tu-real should be added")
	}
	if hasStale {
		t.Fatal("orphaned tu-stale should be stripped")
	}
}

func TestEnsureToolResultPairing_ResultFromNonAdjacentAssistant(t *testing.T) {
	// tool_result references a tool_use from an earlier (non-preceding) assistant
	// message. Only the immediately preceding assistant's IDs are valid.
	msgs := []api.Message{
		makeToolUseMsg("tu-old", "Bash"),
		makeTRMsg("tu-old", "old result"),
		makeToolUseMsg("tu-new", "Read"),
		makeTRMsg("tu-old", "stale reference to old assistant"), // wrong: references tu-old
	}

	result := EnsureToolResultPairing(msgs)

	// Last user message should have tu-new (synthetic) and NOT tu-old.
	lastUser := result[len(result)-1]
	var blocks []json.RawMessage
	json.Unmarshal(lastUser.Content, &blocks)

	ids := map[string]bool{}
	for _, b := range blocks {
		var tr struct {
			ToolUseID string `json:"tool_use_id"`
			Type      string `json:"type"`
		}
		json.Unmarshal(b, &tr)
		if tr.Type == "tool_result" {
			ids[tr.ToolUseID] = true
		}
	}
	if ids["tu-old"] {
		t.Fatal("tu-old from non-adjacent assistant should be stripped")
	}
	if !ids["tu-new"] {
		t.Fatal("tu-new should have a synthetic result")
	}
}

func TestEnsureToolResultPairing_ConsecutiveAssistantMessages(t *testing.T) {
	// Two assistant messages in a row (e.g., after partial error save + new response).
	// Each should get its own synthetic results.
	msgs := []api.Message{
		makeToolUseMsg("tu-first", "Bash"),
		makeToolUseMsg("tu-second", "Read"),
	}

	result := EnsureToolResultPairing(msgs)

	// Should be: assistant(tu-first), user(synthetic tu-first), assistant(tu-second), user(synthetic tu-second)
	if len(result) != 4 {
		t.Fatalf("expected 4 messages; got %d", len(result))
	}
	if result[1].Role != "user" {
		t.Fatal("message 1 should be synthetic user")
	}
	if result[3].Role != "user" {
		t.Fatal("message 3 should be synthetic user")
	}

	id1 := extractTRToolUseID(t, result[1])
	id2 := extractTRToolUseID(t, result[3])
	if id1 != "tu-first" {
		t.Fatalf("first synthetic should reference tu-first; got %q", id1)
	}
	if id2 != "tu-second" {
		t.Fatalf("second synthetic should reference tu-second; got %q", id2)
	}
}

func TestEnsureToolResultPairing_AssistantToolUseFollowedByUserText(t *testing.T) {
	// Assistant has tool_use, but the next user message is plain text (no tool_result).
	// Synthetic results should be added to the text message.
	msgs := []api.Message{
		makeToolUseMsg("tu-1", "Bash"),
		func() api.Message {
			content, _ := json.Marshal([]api.UserContentBlock{api.NewTextBlock("Please continue.")})
			return api.Message{Role: "user", Content: content}
		}(),
	}

	result := EnsureToolResultPairing(msgs)

	// The user message should now have both the text block and a synthetic tool_result.
	var blocks []json.RawMessage
	json.Unmarshal(result[1].Content, &blocks)

	hasText := false
	hasSynthetic := false
	for _, b := range blocks {
		var generic struct {
			Type      string `json:"type"`
			ToolUseID string `json:"tool_use_id"`
		}
		json.Unmarshal(b, &generic)
		if generic.Type == "text" {
			hasText = true
		}
		if generic.Type == "tool_result" && generic.ToolUseID == "tu-1" {
			hasSynthetic = true
		}
	}
	if !hasText {
		t.Fatal("text block should be preserved")
	}
	if !hasSynthetic {
		t.Fatal("synthetic tool_result for tu-1 should be added")
	}
}

func TestRemoveOrphanedToolResults_AllOrphaned(t *testing.T) {
	// Direct test of removeOrphanedToolResults when ALL blocks are orphaned.
	type trBlock struct {
		Type      string `json:"type"`
		ToolUseID string `json:"tool_use_id"`
		Content   string `json:"content"`
	}
	content, _ := json.Marshal([]trBlock{
		{Type: "tool_result", ToolUseID: "orphan-1", Content: "a"},
		{Type: "tool_result", ToolUseID: "orphan-2", Content: "b"},
	})
	msg := api.Message{Role: "user", Content: content}

	result := removeOrphanedToolResults(msg, []string{"valid-id"})

	var blocks []json.RawMessage
	json.Unmarshal(result.Content, &blocks)
	if len(blocks) != 0 {
		t.Fatalf("all orphaned tool_results should be stripped; got %d blocks", len(blocks))
	}
}

// ── TimeBasedMicroCompact tests ─────────────────────────────────────────────

func TestTimeBasedMicroCompact_ClearsOldResults(t *testing.T) {
	large := "this content is larger than 100 bytes for sure and should be cleared by time-based microcompact when it fires"
	var msgs []api.Message
	// 3 old results + matching tool_use blocks.
	for i := 0; i < 3; i++ {
		msgs = append(msgs, makeReadToolUseMsg("tu-old-"+string(rune('a'+i)), "/file.go"))
		msgs = append(msgs, makeTRMsg("tu-old-"+string(rune('a'+i)), large))
	}
	// 5 recent results.
	for i := 0; i < 5; i++ {
		msgs = append(msgs, makeReadToolUseMsg("tu-recent-"+string(rune('a'+i)), "/file.go"))
		msgs = append(msgs, makeTRMsg("tu-recent-"+string(rune('a'+i)), large))
	}

	result := TimeBasedMicroCompact(msgs, 5)

	// Old results should be cleared.
	for i := 0; i < 3; i++ {
		content := extractTRContent(t, result[i*2+1])
		if content == large {
			t.Fatalf("old result %d should be cleared", i)
		}
		if content != "[Old tool result content cleared]" {
			t.Fatalf("old result %d: unexpected stub: %q", i, content)
		}
	}

	// Recent results should be unchanged.
	for i := 3; i < 8; i++ {
		content := extractTRContent(t, result[i*2+1])
		if content != large {
			t.Fatalf("recent result %d should be unchanged; got: %q", i, content[:min(50, len(content))])
		}
	}
}

func TestTimeBasedMicroCompact_NoOp_FewResults(t *testing.T) {
	msgs := []api.Message{
		makeReadToolUseMsg("tu-1", "/f.go"),
		makeTRMsg("tu-1", "content"),
	}
	result := TimeBasedMicroCompact(msgs, 5)
	content := extractTRContent(t, result[1])
	if content != "content" {
		t.Fatal("few results should remain unchanged")
	}
}
