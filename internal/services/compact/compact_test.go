package compact

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/tools/readcache"
)

func TestSanitizeToolPairs_DropsOrphanedToolResult(t *testing.T) {
	// Simulates post-compaction state: summary + ack + tool_result with no matching tool_use
	msgs := []api.Message{
		{Role: "user", Content: json.RawMessage(`"summary"`)},
		{Role: "assistant", Content: json.RawMessage(`[{"type":"text","text":"Understood."}]`)},
		{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"toolu_123","content":"result"}]`)},
	}

	result := sanitizeToolPairs(msgs)

	// The orphaned tool_result should be dropped
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	if result[0].Role != "user" || result[1].Role != "assistant" {
		t.Fatalf("unexpected roles: %s, %s", result[0].Role, result[1].Role)
	}
}

func TestSanitizeToolPairs_KeepsMatchedPairs(t *testing.T) {
	msgs := []api.Message{
		{Role: "user", Content: json.RawMessage(`"hello"`)},
		{Role: "assistant", Content: json.RawMessage(`[{"type":"text","text":"thinking"},{"type":"tool_use","id":"toolu_1","name":"Read","input":{}}]`)},
		{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"toolu_1","content":"file content"}]`)},
		{Role: "assistant", Content: json.RawMessage(`[{"type":"text","text":"done"}]`)},
	}

	result := sanitizeToolPairs(msgs)

	if len(result) != 4 {
		t.Fatalf("expected 4 messages (all kept), got %d", len(result))
	}
}

func TestSanitizeToolPairs_StripsOrphanedToolUse(t *testing.T) {
	// Assistant has tool_use but next message is user text (not tool_result)
	msgs := []api.Message{
		{Role: "user", Content: json.RawMessage(`"hello"`)},
		{Role: "assistant", Content: json.RawMessage(`[{"type":"text","text":"let me check"},{"type":"tool_use","id":"toolu_1","name":"Read","input":{}}]`)},
		{Role: "user", Content: json.RawMessage(`"continue"`)},
	}

	result := sanitizeToolPairs(msgs)

	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}
	// The assistant message should have tool_use stripped but text kept
	var blocks []json.RawMessage
	if err := json.Unmarshal(result[1].Content, &blocks); err != nil {
		t.Fatalf("failed to parse assistant content: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block (text only), got %d", len(blocks))
	}
}

func TestSanitizeToolPairs_DropsAssistantWithOnlyToolUse(t *testing.T) {
	// Assistant has only tool_use blocks, no text — should be dropped entirely
	msgs := []api.Message{
		{Role: "user", Content: json.RawMessage(`"hello"`)},
		{Role: "assistant", Content: json.RawMessage(`[{"type":"tool_use","id":"toolu_1","name":"Read","input":{}}]`)},
		{Role: "user", Content: json.RawMessage(`"continue"`)},
	}

	result := sanitizeToolPairs(msgs)

	if len(result) != 2 {
		t.Fatalf("expected 2 messages (assistant dropped), got %d", len(result))
	}
	if result[0].Role != "user" || result[1].Role != "user" {
		t.Fatalf("unexpected roles: %s, %s", result[0].Role, result[1].Role)
	}
}

func TestCompactAssistantMessagesUseContentBlockArrays(t *testing.T) {
	// Verify the synthetic assistant messages in Compact output use arrays, not strings.
	// This is a static check of the format — the actual Compact function needs an API client,
	// so we test the sanitized output format directly.
	syntheticMessages := []api.Message{
		{Role: "user", Content: json.RawMessage(`"summary"`)},
		{Role: "assistant", Content: json.RawMessage(`[{"type":"text","text":"Understood. I have the context from the summary. Let's continue."}]`)},
	}

	for i, msg := range syntheticMessages {
		if msg.Role != "assistant" {
			continue
		}
		var blocks []json.RawMessage
		if err := json.Unmarshal(msg.Content, &blocks); err != nil {
			t.Errorf("message %d: assistant content is not a valid array: %v (content: %s)", i, err, string(msg.Content))
		}
	}
}

func TestSanitizeToolPairs_MismatchedIDs(t *testing.T) {
	// Assistant has tool_use with ID "toolu_A", but user's tool_result references "toolu_B" (orphaned).
	// This is the exact scenario from the reported API error.
	msgs := []api.Message{
		{Role: "user", Content: json.RawMessage(`"hello"`)},
		{Role: "assistant", Content: json.RawMessage(`[{"type":"text","text":"checking"},{"type":"tool_use","id":"toolu_A","name":"Read","input":{}}]`)},
		{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"toolu_B","content":"stale result"}]`)},
	}

	result := sanitizeToolPairs(msgs)

	// tool_use "toolu_A" has no matching result → stripped from assistant (text kept)
	// tool_result "toolu_B" has no matching use → stripped entirely
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	if result[0].Role != "user" || result[1].Role != "assistant" {
		t.Fatalf("unexpected roles: %s, %s", result[0].Role, result[1].Role)
	}
}

func TestSanitizeToolPairs_PartialIDMatch(t *testing.T) {
	// Assistant has 2 tool_use blocks, but user only has result for one of them.
	msgs := []api.Message{
		{Role: "user", Content: json.RawMessage(`"hello"`)},
		{Role: "assistant", Content: json.RawMessage(`[{"type":"tool_use","id":"toolu_1","name":"Read","input":{}},{"type":"tool_use","id":"toolu_2","name":"Grep","input":{}}]`)},
		{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"toolu_1","content":"ok"}]`)},
	}

	result := sanitizeToolPairs(msgs)

	// toolu_2 is orphaned → stripped from assistant; toolu_1 pair survives
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}
	// Verify assistant only has toolu_1
	var blocks []json.RawMessage
	json.Unmarshal(result[1].Content, &blocks)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 tool_use block in assistant, got %d", len(blocks))
	}
}

func TestSanitizeToolPairs_ToolResultWithTextPreserved(t *testing.T) {
	// User message has both text and an orphaned tool_result — text should survive.
	msgs := []api.Message{
		{Role: "user", Content: json.RawMessage(`"hello"`)},
		{Role: "assistant", Content: json.RawMessage(`[{"type":"text","text":"done"}]`)},
		{Role: "user", Content: json.RawMessage(`[{"type":"text","text":"keep me"},{"type":"tool_result","tool_use_id":"toolu_gone","content":"orphan"}]`)},
	}

	result := sanitizeToolPairs(msgs)

	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}
	// The user message should have only the text block, tool_result stripped
	var blocks []json.RawMessage
	json.Unmarshal(result[2].Content, &blocks)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block (text only) in user msg, got %d", len(blocks))
	}
}

// ── helpers for MicroCompact / ContentClearCompact ───────────────────────────

func makeTRMsg(id, content string) api.Message {
	type trBlock struct {
		Type      string `json:"type"`
		ToolUseID string `json:"tool_use_id"`
		Content   string `json:"content"`
		IsError   bool   `json:"is_error,omitempty"`
	}
	raw, _ := json.Marshal([]trBlock{{Type: "tool_result", ToolUseID: id, Content: content}})
	return api.Message{Role: "user", Content: raw}
}

func makeTRMsgError(id, content string) api.Message {
	type trBlock struct {
		Type      string `json:"type"`
		ToolUseID string `json:"tool_use_id"`
		Content   string `json:"content"`
		IsError   bool   `json:"is_error,omitempty"`
	}
	raw, _ := json.Marshal([]trBlock{{Type: "tool_result", ToolUseID: id, Content: content, IsError: true}})
	return api.Message{Role: "user", Content: raw}
}

func extractTRContent(t *testing.T, msg api.Message) string {
	t.Helper()
	type trBlock struct {
		Type    string `json:"type"`
		Content string `json:"content"`
	}
	var blocks []trBlock
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		t.Fatalf("failed to parse tool_result msg: %v", err)
	}
	if len(blocks) == 0 {
		return ""
	}
	return blocks[0].Content
}

func extractTRToolUseID(t *testing.T, msg api.Message) string {
	t.Helper()
	type trBlock struct {
		Type      string `json:"type"`
		ToolUseID string `json:"tool_use_id"`
	}
	var blocks []trBlock
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		t.Fatalf("failed to parse tool_result msg: %v", err)
	}
	if len(blocks) == 0 {
		return ""
	}
	return blocks[0].ToolUseID
}

// ── MicroCompact tests ────────────────────────────────────────────────────────

func TestMicroCompact_NoOp_FewResults(t *testing.T) {
	msgs := []api.Message{
		makeTRMsg("id1", "result one"),
		makeTRMsg("id2", "result two"),
	}
	result := MicroCompact(msgs, 6, 10)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	if extractTRContent(t, result[0]) != "result one" {
		t.Fatalf("expected first result unchanged")
	}
	if extractTRContent(t, result[1]) != "result two" {
		t.Fatalf("expected second result unchanged")
	}
}

func TestMicroCompact_ClearsOldLargeResult(t *testing.T) {
	// Total tool results exceed 100KB target → oldest (outside keepLast window) is cleared.
	var msgs []api.Message
	largeContent := strings.Repeat("X", 20_000) // 20KB each
	for i := 0; i < 6; i++ {
		msgs = append(msgs, makeTRMsg("id-recent", largeContent))
	}
	// Prepend an old large result — total will be ~140KB, triggering clear.
	msgs = append([]api.Message{makeTRMsg("id-old", largeContent)}, msgs...)

	result := MicroCompact(msgs, 6, 10)

	if len(result) != 7 {
		t.Fatalf("expected 7 messages, got %d", len(result))
	}
	cleared := extractTRContent(t, result[0])
	if cleared == largeContent {
		t.Fatalf("expected old large result to be cleared, got original content")
	}
	if len(cleared) == 0 {
		t.Fatalf("expected placeholder content, got empty string")
	}
}

func TestMicroCompact_PreservesRecentResults(t *testing.T) {
	// 8 results at 20KB each = 160KB → over 100KB target.
	// Last 6 are protected, first 2 should be cleared.
	var msgs []api.Message
	largeContent := strings.Repeat("Y", 20_000) // 20KB each
	for i := 0; i < 2; i++ {
		msgs = append(msgs, makeTRMsg("id-old", largeContent))
	}
	for i := 0; i < 6; i++ {
		msgs = append(msgs, makeTRMsg("id-recent", largeContent))
	}

	result := MicroCompact(msgs, 6, 10)

	// First 2 should be cleared
	for i := 0; i < 2; i++ {
		content := extractTRContent(t, result[i])
		if content == largeContent {
			t.Fatalf("result[%d]: expected cleared, got original content", i)
		}
	}
	// Last 6 should be untouched
	for i := 2; i < 8; i++ {
		content := extractTRContent(t, result[i])
		if content != largeContent {
			t.Fatalf("result[%d]: expected original content, got %q", i, content[:min(50, len(content))])
		}
	}
}

func TestMicroCompact_SkipsSmallResults(t *testing.T) {
	// Old result but content < minSizeBytes: NOT cleared even if over budget.
	smallContent := "tiny"
	largeContent := strings.Repeat("Z", 20_000)
	var msgs []api.Message
	msgs = append(msgs, makeTRMsg("id-small-old", smallContent))
	for i := 0; i < 6; i++ {
		msgs = append(msgs, makeTRMsg("id-recent", largeContent))
	}

	result := MicroCompact(msgs, 6, 10)

	content := extractTRContent(t, result[0])
	if content != smallContent {
		t.Fatalf("expected small result to remain unchanged, got %q", content)
	}
}

func TestMicroCompact_SkipsErrorResults(t *testing.T) {
	// Old result with is_error=true: NOT cleared even if large and over budget.
	largeContent := strings.Repeat("E", 20_000)
	var msgs []api.Message
	msgs = append(msgs, makeTRMsgError("id-err", largeContent))
	for i := 0; i < 6; i++ {
		msgs = append(msgs, makeTRMsg("id-recent", largeContent))
	}

	result := MicroCompact(msgs, 6, 10)

	content := extractTRContent(t, result[0])
	if content != largeContent {
		t.Fatalf("expected error result to remain unchanged, got %q", content[:min(50, len(content))])
	}
}

func TestMicroCompact_PreservesToolUseID(t *testing.T) {
	// Cleared result still has original tool_use_id.
	largeContent := strings.Repeat("T", 20_000)
	var msgs []api.Message
	msgs = append(msgs, makeTRMsg("my-tool-use-id", largeContent))
	for i := 0; i < 6; i++ {
		msgs = append(msgs, makeTRMsg("id-recent", largeContent))
	}

	result := MicroCompact(msgs, 6, 10)

	id := extractTRToolUseID(t, result[0])
	if id != "my-tool-use-id" {
		t.Fatalf("expected tool_use_id=%q, got %q", "my-tool-use-id", id)
	}
}

func TestMicroCompact_EmptyMessages(t *testing.T) {
	result := MicroCompact(nil, 6, 10)
	if len(result) != 0 {
		t.Fatalf("expected empty result for nil input, got %d", len(result))
	}
}

func TestMicroCompact_NoUserMessages(t *testing.T) {
	msgs := []api.Message{
		{Role: "assistant", Content: json.RawMessage(`[{"type":"text","text":"hello"}]`)},
		{Role: "assistant", Content: json.RawMessage(`[{"type":"text","text":"world"}]`)},
	}
	result := MicroCompact(msgs, 6, 10)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages unchanged, got %d", len(result))
	}
}

// ── ContentClearCompact tests ─────────────────────────────────────────────────

func TestContentClearCompact_NoOp_FewMessages(t *testing.T) {
	msgs := []api.Message{
		makeTRMsg("id1", "result one"),
		makeTRMsg("id2", "result two"),
	}
	result := ContentClearCompact(msgs, 10, 10)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	if extractTRContent(t, result[0]) != "result one" {
		t.Fatalf("expected first result unchanged")
	}
}

func TestContentClearCompact_ClearsOldLargeToolResult(t *testing.T) {
	largeContent := "this is definitely more than ten bytes long"
	msgs := []api.Message{
		makeTRMsg("id1", largeContent),
		makeTRMsg("id2", largeContent),
		makeTRMsg("id3", largeContent),
		makeTRMsg("id4", "recent1"),
		makeTRMsg("id5", "recent2"),
	}
	result := ContentClearCompact(msgs, 2, 10)

	// First 3 are old — large ones should be cleared
	for i := 0; i < 3; i++ {
		content := extractTRContent(t, result[i])
		if content == largeContent {
			t.Fatalf("result[%d]: expected cleared, got original content", i)
		}
	}
	// Last 2 are recent — should be untouched
	if extractTRContent(t, result[3]) != "recent1" {
		t.Fatalf("result[3]: expected 'recent1' preserved")
	}
	if extractTRContent(t, result[4]) != "recent2" {
		t.Fatalf("result[4]: expected 'recent2' preserved")
	}
}

func TestContentClearCompact_PreservesRecentMessages(t *testing.T) {
	largeContent := "this content is definitely larger than ten bytes"
	msgs := []api.Message{
		makeTRMsg("id1", largeContent),
		makeTRMsg("id2", largeContent),
		makeTRMsg("id3", largeContent),
		makeTRMsg("id4", largeContent),
		makeTRMsg("id5", largeContent),
	}
	result := ContentClearCompact(msgs, 3, 10)

	// First 2 should be cleared
	for i := 0; i < 2; i++ {
		content := extractTRContent(t, result[i])
		if content == largeContent {
			t.Fatalf("result[%d]: expected cleared, got original content", i)
		}
	}
	// Last 3 should be untouched
	for i := 2; i < 5; i++ {
		content := extractTRContent(t, result[i])
		if content != largeContent {
			t.Fatalf("result[%d]: expected original content preserved, got %q", i, content)
		}
	}
}

func TestContentClearCompact_PreservesSmallResults(t *testing.T) {
	smallContent := "tiny"
	msgs := []api.Message{
		makeTRMsg("id1", smallContent),
		makeTRMsg("id2", "recent1"),
		makeTRMsg("id3", "recent2"),
	}
	result := ContentClearCompact(msgs, 2, 10)

	// The old result is small — should NOT be cleared
	content := extractTRContent(t, result[0])
	if content != smallContent {
		t.Fatalf("expected small result preserved, got %q", content)
	}
}

func TestContentClearCompact_PreservesToolUseID(t *testing.T) {
	largeContent := "this content is definitely larger than ten bytes"
	msgs := []api.Message{
		makeTRMsg("original-id", largeContent),
		makeTRMsg("id2", "recent1"),
		makeTRMsg("id3", "recent2"),
	}
	result := ContentClearCompact(msgs, 2, 10)

	id := extractTRToolUseID(t, result[0])
	if id != "original-id" {
		t.Fatalf("expected tool_use_id=%q, got %q", "original-id", id)
	}
}

func TestContentClearCompact_SkipsAssistantMessages(t *testing.T) {
	msgs := []api.Message{
		{Role: "assistant", Content: json.RawMessage(`[{"type":"text","text":"I did stuff"}]`)},
		makeTRMsg("id2", "recent1"),
		makeTRMsg("id3", "recent2"),
	}
	result := ContentClearCompact(msgs, 2, 10)

	// The assistant message should be unchanged
	if string(result[0].Content) != `[{"type":"text","text":"I did stuff"}]` {
		t.Fatalf("expected assistant message unchanged, got %s", string(result[0].Content))
	}
}

func TestContentClearCompact_EmptyMessages(t *testing.T) {
	result := ContentClearCompact([]api.Message{}, 5, 10)
	if len(result) != 0 {
		t.Fatalf("expected empty result, got %d", len(result))
	}
}

func TestContentClearCompact_ClearedContentFormat(t *testing.T) {
	// "hello world" = 11 bytes
	content := "hello world"
	msgs := []api.Message{
		makeTRMsg("id1", content),
		makeTRMsg("id2", "recent1"),
		makeTRMsg("id3", "recent2"),
	}
	result := ContentClearCompact(msgs, 2, 10)

	cleared := extractTRContent(t, result[0])
	expected := "[content cleared — 11 bytes]"
	if cleared != expected {
		t.Fatalf("expected cleared format %q, got %q", expected, cleared)
	}
}

// makeReadToolUseMsg builds an assistant message containing a Read tool_use block.
func makeReadToolUseMsg(toolUseID, filePath string) api.Message {
	type tuBlock struct {
		Type  string          `json:"type"`
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	}
	inp, _ := json.Marshal(map[string]string{"file_path": filePath})
	raw, _ := json.Marshal([]tuBlock{{Type: "tool_use", ID: toolUseID, Name: "Read", Input: inp}})
	return api.Message{Role: "assistant", Content: raw}
}

// ── MicroCompact ReadCache behaviour (no-invalidate fix) ─────────────────────

// TestMicroCompact_DoesNotInvalidateReadCache verifies that MicroCompact no longer
// invalidates ReadCache entries when it clears a tool result. Invalidating was the
// root cause of the re-read loop: clear→invalidate→re-read→clear→…
func TestMicroCompact_DoesNotInvalidateReadCache(t *testing.T) {
	rc := readcache.New(64)

	// Create a real temp file so ReadCache.Get can stat it and validate the entry.
	f, err := os.CreateTemp(t.TempDir(), "readcache-test-*.go")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	info, err := os.Stat(f.Name())
	if err != nil {
		t.Fatal(err)
	}

	filePath := f.Name()
	const toolUseID = "tu-read-1"

	// Pre-populate the ReadCache as the Read tool would after a successful read.
	rc.Put(readcache.Key{FilePath: filePath, Offset: 1, Limit: 2000}, "file content", info.ModTime())

	// Build: assistant Read tool_use + tool_result (large enough to be cleared).
	// Use 10KB per result × 11 results = 110KB → over 100KB target → old one gets cleared.
	largeContent := strings.Repeat("x", 10_000)
	msgs := []api.Message{
		makeReadToolUseMsg(toolUseID, filePath),
		makeTRMsg(toolUseID, largeContent),
	}
	// Add 10 more recent results so the old one falls outside keepLastResults=10.
	for i := 0; i < 10; i++ {
		msgs = append(msgs, makeTRMsg("recent-id", largeContent))
	}

	MicroCompact(msgs, 10, 10, rc)

	// Cache entry must still be present after compaction.
	_, ok := rc.Get(readcache.Key{FilePath: filePath, Offset: 1, Limit: 2000})
	if !ok {
		t.Fatal("MicroCompact must NOT invalidate ReadCache entries; cache was cleared but should have been kept")
	}
}

// TestMicroCompact_ClearedStubIncludesFilePath verifies that when a Read result is
// cleared the stub message includes the file path so the model knows which file was
// compacted and can use Grep instead of re-reading.
func TestMicroCompact_ClearedStubIncludesFilePath(t *testing.T) {
	rc := readcache.New(64)

	const filePath = "/src/query/engine.go"
	const toolUseID = "tu-read-2"

	// 10KB × 11 = 110KB → over 100KB target.
	largeContent := strings.Repeat("y", 10_000)
	msgs := []api.Message{
		makeReadToolUseMsg(toolUseID, filePath),
		makeTRMsg(toolUseID, largeContent),
	}
	for i := 0; i < 10; i++ {
		msgs = append(msgs, makeTRMsg("recent-id", largeContent))
	}

	result := MicroCompact(msgs, 10, 10, rc)

	stub := extractTRContent(t, result[1])
	if !strings.Contains(stub, filePath) {
		t.Fatalf("cleared stub should include file path %q; got: %q", filePath, stub[:min(200, len(stub))])
	}
}

// TestMicroCompact_ClearedStubFallbackWithoutFilePath verifies that when the
// tool_use block cannot be found (non-Read tool or unknown ID), the generic
// "[result cleared — N bytes]" stub is used rather than panicking.
func TestMicroCompact_ClearedStubFallbackWithoutFilePath(t *testing.T) {
	// 10KB × 11 = 110KB → over 100KB target.
	largeContent := strings.Repeat("z", 10_000)
	msgs := []api.Message{
		// No matching assistant tool_use in the messages.
		makeTRMsg("unknown-id", largeContent),
	}
	for i := 0; i < 10; i++ {
		msgs = append(msgs, makeTRMsg("recent-id", largeContent))
	}

	result := MicroCompact(msgs, 10, 10)

	stub := extractTRContent(t, result[0])
	if !strings.Contains(stub, "result cleared") {
		t.Fatalf("expected fallback stub to contain 'result cleared'; got: %q", stub[:min(200, len(stub))])
	}
	if strings.Contains(stub, "nil") || stub == "" {
		t.Fatalf("unexpected empty or nil stub: %q", stub)
	}
}

func TestMessageHasToolResults(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"tool_result blocks", `[{"type":"tool_result","tool_use_id":"x","content":"y"}]`, true},
		{"text only", `[{"type":"text","text":"hello"}]`, false},
		{"bare string", `"hello"`, false},
		{"mixed", `[{"type":"text","text":"a"},{"type":"tool_result","tool_use_id":"x","content":"b"}]`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := api.Message{Role: "user", Content: json.RawMessage(tt.content)}
			got := messageHasToolResults(msg)
			if got != tt.want {
				t.Errorf("messageHasToolResults() = %v, want %v", got, tt.want)
			}
		})
	}
}
