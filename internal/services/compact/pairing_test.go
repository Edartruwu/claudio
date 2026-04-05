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
