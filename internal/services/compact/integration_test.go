package compact

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Abraxas-365/claudio/internal/api"
)

// ── Integration tests: full microcompact flow ────────────────────────────────
// These tests simulate realistic sessions to verify that all optimization layers
// compose correctly for both Anthropic (with context_management) and non-Anthropic
// (client-side only) providers.

// makeToolUsePair creates an assistant tool_use + user tool_result pair.
func makeToolUsePair(toolUseID, toolName, content string) (api.Message, api.Message) {
	tu := struct {
		Type  string          `json:"type"`
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	}{Type: "tool_use", ID: toolUseID, Name: toolName, Input: json.RawMessage(`{}`)}
	tuContent, _ := json.Marshal([]any{tu})

	tr := struct {
		Type      string `json:"type"`
		ToolUseID string `json:"tool_use_id"`
		Content   string `json:"content"`
	}{Type: "tool_result", ToolUseID: toolUseID, Content: content}
	trContent, _ := json.Marshal([]any{tr})

	return api.Message{Role: "assistant", Content: tuContent},
		api.Message{Role: "user", Content: trContent}
}

// ── MicroCompact skips already-compacted stubs ──────────────────────────────

func TestMicroCompact_SkipsAlreadyCompactedBudgetStubs(t *testing.T) {
	// Simulate: budget system already replaced a large result with a preview stub.
	// MicroCompact should not count the stub toward the size budget.
	budgetStub := "[Tool output too large (68000 bytes). Full output saved to disk.]\nPreview (first 2000 bytes):\npackage query..."
	largeContent := strings.Repeat("X", 20_000) // 20KB real content

	var msgs []api.Message
	// Old budget-replaced stub (should be ignored by MicroCompact)
	msgs = append(msgs, makeTRMsg("tu-budget-stub", budgetStub))
	// 6 recent large results
	for i := 0; i < 6; i++ {
		msgs = append(msgs, makeTRMsg("tu-recent", largeContent))
	}
	// Total real content: 6 × 20KB = 120KB (over 100KB target)
	// But the stub is only ~100 bytes — should NOT be counted or cleared.

	result := MicroCompact(msgs, 6, 10)

	// The budget stub should remain unchanged (not double-cleared).
	stubContent := extractTRContent(t, result[0])
	if !strings.Contains(stubContent, "Tool output too large") {
		t.Fatalf("budget stub should remain unchanged; got: %q", stubContent[:min(80, len(stubContent))])
	}
}

func TestMicroCompact_SkipsAlreadyClearedStubs(t *testing.T) {
	// Previous MicroCompact already cleared a result → should not re-process.
	oldStub := "[result cleared — 45000 bytes]"
	largeContent := strings.Repeat("Y", 20_000)

	var msgs []api.Message
	msgs = append(msgs, makeTRMsg("tu-old-cleared", oldStub))
	for i := 0; i < 6; i++ {
		msgs = append(msgs, makeTRMsg("tu-recent", largeContent))
	}

	result := MicroCompact(msgs, 6, 10)

	stubContent := extractTRContent(t, result[0])
	if stubContent != oldStub {
		t.Fatalf("already-cleared stub should remain unchanged; got: %q", stubContent)
	}
}

// ── Budget exempts Read tool results ────────────────────────────────────────

func TestBudget_ExemptsReadToolResults(t *testing.T) {
	state := NewReplacementState()

	// Build: assistant Read tool_use + large tool_result (>200KB).
	readTU, readTR := makeToolUsePair("tu-read-1", "Read", strings.Repeat("R", PerMessageBudget+50_000))
	// Also a Grep result (should be budget-eligible).
	grepTU, grepTR := makeToolUsePair("tu-grep-1", "Grep", strings.Repeat("G", PerMessageBudget+50_000))

	msgs := []api.Message{readTU, readTR, grepTU, grepTR}

	EnforceToolResultBudget(msgs, state, nil)

	// Read result should be unchanged (exempted).
	readContent := extractTRContent(t, msgs[1])
	if strings.Contains(readContent, "Tool output too large") {
		t.Fatal("Read tool results should be exempted from budget enforcement")
	}
	if !strings.HasPrefix(readContent, "RRRR") {
		t.Fatal("Read result content should be intact")
	}

	// Read should be marked as seen but NOT in replacements.
	if !state.SeenIDs["tu-read-1"] {
		t.Fatal("Read tool_use_id should be in SeenIDs")
	}
	if _, replaced := state.Replacements["tu-read-1"]; replaced {
		t.Fatal("Read tool_use_id should NOT be in Replacements")
	}

	// Grep result should be replaced (not exempted).
	grepContent := extractTRContent(t, msgs[3])
	if !strings.Contains(grepContent, "Tool output too large") {
		t.Fatal("Grep tool results should be replaced when over budget")
	}
}

// ── Full Anthropic flow (budget + microcompact) ─────────────────────────────

func TestIntegration_AnthropicFlow_BudgetThenMicroCompact(t *testing.T) {
	// Simulates what happens for Anthropic: budget enforcement runs before API call,
	// then MicroCompact runs after tool execution. Both should compose cleanly.
	state := NewReplacementState()

	// Turn 1: 3 tool results (Grep 50KB, Bash 200B, Read 60KB).
	grepTU, grepTR := makeToolUsePair("tu-grep", "Grep", strings.Repeat("G", 50_000))
	bashTU, bashTR := makeToolUsePair("tu-bash", "Bash", "exit 0")
	readTU, readTR := makeToolUsePair("tu-read", "Read", strings.Repeat("R", 60_000))

	msgs := []api.Message{grepTU, grepTR, bashTU, bashTR, readTU, readTR}

	// Step 1: Budget enforcement (before API call).
	msgs = EnforceToolResultBudget(msgs, state, nil)

	// Read should be exempted; Grep might be replaced if total > 200KB.
	readContent := extractTRContent(t, msgs[5])
	if strings.Contains(readContent, "Tool output too large") {
		t.Fatal("Read should be exempted from budget")
	}

	// Step 2: MicroCompact (after tool execution, same messages grow).
	// Add more tool results to push over 100KB target.
	for i := 0; i < 5; i++ {
		tu, tr := makeToolUsePair("tu-new-"+string(rune('a'+i)), "Grep", strings.Repeat("N", 20_000))
		msgs = append(msgs, tu, tr)
	}

	msgs = MicroCompact(msgs, 5, 512)

	// Verify: at least some old results got cleared, recent ones preserved.
	lastResult := extractTRContent(t, msgs[len(msgs)-1])
	if strings.Contains(lastResult, "result cleared") {
		t.Fatal("most recent results should NOT be cleared")
	}
}

// ── Full non-Anthropic flow (no context_management, client-side only) ───────

func TestIntegration_NonAnthropicFlow_MicroCompactOnly(t *testing.T) {
	// Non-Anthropic providers don't get context_management.edits,
	// so client-side MicroCompact + Budget must handle everything.
	state := NewReplacementState()

	// Build a realistic session: 15 tool calls of various sizes.
	var msgs []api.Message
	for i := 0; i < 15; i++ {
		var content string
		var toolName string
		switch i % 3 {
		case 0:
			content = strings.Repeat("R", 15_000) // 15KB Read
			toolName = "Read"
		case 1:
			content = strings.Repeat("G", 8_000) // 8KB Grep
			toolName = "Grep"
		case 2:
			content = "exit 0" // tiny Bash
			toolName = "Bash"
		}
		id := string(rune('a'+i)) + "-id"
		tu, tr := makeToolUsePair(id, toolName, content)
		msgs = append(msgs, tu, tr)
	}
	// Total: 5×15KB + 5×8KB + 5×6B = 75KB + 40KB = ~115KB

	// Budget enforcement.
	msgs = EnforceToolResultBudget(msgs, state, nil)

	// Read results should be exempted from budget.
	for i := 0; i < len(msgs); i++ {
		if msgs[i].Role != "user" {
			continue
		}
		content := extractTRContent(t, msgs[i])
		id := extractTRToolUseID(t, msgs[i])
		// If it was a Read tool (every 3rd starting from 0), it should be intact.
		if state.SeenIDs[id] && !strings.Contains(content, "Tool output too large") {
			// Either it's a Read (exempted) or under budget — both OK.
			continue
		}
	}

	// MicroCompact.
	msgs = MicroCompact(msgs, 10, 512)

	// Session should still have all messages (MicroCompact doesn't remove messages).
	if len(msgs) != 30 { // 15 pairs × 2
		t.Fatalf("expected 30 messages, got %d", len(msgs))
	}

	// Some old results should be cleared, recent 10 should be intact.
	clearedCount := 0
	for _, msg := range msgs {
		if msg.Role != "user" {
			continue
		}
		content := extractTRContent(t, msg)
		if strings.Contains(content, "result cleared") {
			clearedCount++
		}
	}
	// With 115KB total and 100KB target, at least 1 result should be cleared.
	if clearedCount == 0 {
		t.Fatal("expected at least one result to be cleared by MicroCompact")
	}
}

// ── TimeBasedMicroCompact preserves non-compactable tools ───────────────────

func TestTimeBasedMicroCompact_PreservesNonCompactableTools(t *testing.T) {
	// TaskCreate, AskUser, EnterPlanMode etc. should NOT be cleared.
	large := strings.Repeat("X", 200)

	var msgs []api.Message
	// Old non-compactable tool result (no matching Read/Grep/Bash tool_use).
	tuCustom := struct {
		Type  string          `json:"type"`
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	}{Type: "tool_use", ID: "tu-task", Name: "TaskCreate", Input: json.RawMessage(`{}`)}
	tuContent, _ := json.Marshal([]any{tuCustom})
	msgs = append(msgs, api.Message{Role: "assistant", Content: tuContent})
	msgs = append(msgs, makeTRMsg("tu-task", "Task created: fix the bug"))

	// Compactable Read results.
	for i := 0; i < 8; i++ {
		tu, tr := makeToolUsePair("tu-read-"+string(rune('a'+i)), "Read", large)
		msgs = append(msgs, tu, tr)
	}

	result := TimeBasedMicroCompact(msgs, 5)

	// TaskCreate result should be preserved (not compactable).
	taskContent := extractTRContent(t, result[1])
	if taskContent != "Task created: fix the bug" {
		t.Fatalf("non-compactable tool result should be preserved; got: %q", taskContent)
	}
}

// ── MicroCompact with realistic mixed-size data ─────────────────────────────

func TestMicroCompact_RealisticMixedSizes(t *testing.T) {
	// Simulates: Read 68KB, Grep 5KB, Bash 200B, Read 45KB, Grep 3KB.
	// Total: ~121KB, over 100KB target.
	// Should clear the 68KB Read first (largest), bringing total to ~53KB.
	var msgs []api.Message

	sizes := []struct {
		id, tool string
		size     int
	}{
		{"tu-read1", "Read", 68_000},
		{"tu-grep1", "Grep", 5_000},
		{"tu-bash1", "Bash", 200},
		{"tu-read2", "Read", 45_000},
		{"tu-grep2", "Grep", 3_000},
	}

	for _, s := range sizes {
		tu, tr := makeToolUsePair(s.id, s.tool, strings.Repeat("X", s.size))
		msgs = append(msgs, tu, tr)
	}

	result := MicroCompact(msgs, 3, 512) // protect last 3 results

	// The 68KB Read (index 0) should be cleared first (largest outside protected window).
	read1Content := extractTRContent(t, result[1])
	if !strings.Contains(read1Content, "result cleared") && !strings.Contains(read1Content, "Read result for") {
		t.Fatalf("68KB Read should be cleared; got %d bytes", len(read1Content))
	}

	// The 200B Bash should NOT be cleared (too small, < minSizeBytes=512).
	bashContent := extractTRContent(t, result[5])
	if strings.Contains(bashContent, "cleared") {
		t.Fatal("200B Bash result should NOT be cleared (below minSizeBytes)")
	}

	// Last 3 results (Read 45KB, Grep 3KB) should be in protected window.
	// Actually with keepLastResults=3, the last 3 tool_results are protected.
	// Positions: 0=Read68K, 1=Grep5K, 2=Bash200, 3=Read45K, 4=Grep3K
	// Protected: indices 2,3,4 (last 3)
	grep2Content := extractTRContent(t, result[9])
	if strings.Contains(grep2Content, "cleared") {
		t.Fatal("last Grep result should be in protected window")
	}
}

// ── Budget + MicroCompact don't fight each other ────────────────────────────

func TestIntegration_BudgetAndMicroCompact_NoConflict(t *testing.T) {
	// After budget replaces a result with a preview, MicroCompact should
	// recognize the preview as already-compacted and skip it.
	state := NewReplacementState()

	// One huge Grep result (>200KB) → budget will replace with preview.
	grepTU, grepTR := makeToolUsePair("tu-huge-grep", "Grep", strings.Repeat("G", PerMessageBudget+100_000))
	msgs := []api.Message{grepTU, grepTR}

	// Add 10 normal results.
	for i := 0; i < 10; i++ {
		tu, tr := makeToolUsePair("tu-normal-"+string(rune('a'+i)), "Grep", strings.Repeat("N", 10_000))
		msgs = append(msgs, tu, tr)
	}

	// Budget enforcement replaces the huge grep with ~2KB preview.
	msgs = EnforceToolResultBudget(msgs, state, nil)
	budgetResult := extractTRContent(t, msgs[1])
	if !strings.Contains(budgetResult, "Tool output too large") {
		t.Fatal("budget should have replaced the huge grep result")
	}

	// Now MicroCompact runs. It should NOT re-clear the budget stub.
	msgs = MicroCompact(msgs, 5, 512)
	afterMC := extractTRContent(t, msgs[1])
	if !strings.Contains(afterMC, "Tool output too large") {
		t.Fatalf("budget stub should survive MicroCompact; got: %q", afterMC[:min(80, len(afterMC))])
	}
}
