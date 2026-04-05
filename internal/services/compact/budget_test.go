package compact

import (
	"strings"
	"testing"

	"github.com/Abraxas-365/claudio/internal/api"
)

// ── Budget enforcement tests ─────────────────────────────────────────────────

func TestEnforceToolResultBudget_NilState(t *testing.T) {
	msgs := []api.Message{makeTRMsg("id1", "content")}
	result := EnforceToolResultBudget(msgs, nil, nil)
	if len(result) != 1 {
		t.Fatal("nil state should return messages unchanged")
	}
}

func TestEnforceToolResultBudget_UnderBudget(t *testing.T) {
	state := NewReplacementState()
	msgs := []api.Message{makeTRMsg("id1", "small content")}
	result := EnforceToolResultBudget(msgs, state, nil)
	content := extractTRContent(t, result[0])
	if content != "small content" {
		t.Fatalf("under-budget content should be unchanged; got: %q", content)
	}
}

func TestEnforceToolResultBudget_OverBudget_ReplacesLargest(t *testing.T) {
	state := NewReplacementState()

	// Create content that exceeds the per-message budget.
	large := strings.Repeat("X", PerMessageBudget+1000)
	small := "tiny result"

	msgs := []api.Message{
		makeTRMsg("id-large", large),
		makeTRMsg("id-small", small),
	}

	result := EnforceToolResultBudget(msgs, state, nil)

	// The large one should have been replaced with a preview.
	largeContent := extractTRContent(t, result[0])
	if !strings.Contains(largeContent, "Tool output too large") {
		t.Fatalf("large result should be replaced with preview; got: %s", largeContent[:min(100, len(largeContent))])
	}

	// The small one should remain unchanged.
	smallContent := extractTRContent(t, result[1])
	if smallContent != small {
		t.Fatalf("small result should remain unchanged; got: %q", smallContent)
	}
}

func TestEnforceToolResultBudget_CachedReplacementReapplied(t *testing.T) {
	state := NewReplacementState()

	large := strings.Repeat("Y", PerMessageBudget+1000)
	msgs := []api.Message{makeTRMsg("id-cached", large)}

	// First call: should replace.
	EnforceToolResultBudget(msgs, state, nil)
	if _, ok := state.Replacements["id-cached"]; !ok {
		t.Fatal("expected replacement to be cached after first enforcement")
	}
	cachedReplacement := state.Replacements["id-cached"]

	// Second call with new messages (simulating the same tool_use_id reappearing).
	msgs2 := []api.Message{makeTRMsg("id-cached", "different content now")}
	EnforceToolResultBudget(msgs2, state, nil)

	// Should get the exact same cached replacement (prompt cache stability).
	content := extractTRContent(t, msgs2[0])
	if content != cachedReplacement {
		t.Fatalf("cached replacement should be re-applied byte-identically")
	}
}

func TestEnforceToolResultBudget_FrozenIDsNeverReplaced(t *testing.T) {
	state := NewReplacementState()

	// First pass: under budget, both IDs get marked as "seen" (frozen).
	small1 := "content A"
	small2 := "content B"
	msgs := []api.Message{
		makeTRMsg("id-a", small1),
		makeTRMsg("id-b", small2),
	}
	EnforceToolResultBudget(msgs, state, nil)

	if !state.SeenIDs["id-a"] || !state.SeenIDs["id-b"] {
		t.Fatal("both IDs should be marked as seen after first pass")
	}

	// Second pass with a huge new result: only the new one should be replaced,
	// not the frozen ones.
	huge := strings.Repeat("Z", PerMessageBudget+1000)
	msgs2 := []api.Message{
		makeTRMsg("id-a", small1),
		makeTRMsg("id-b", small2),
		makeTRMsg("id-new", huge),
	}
	EnforceToolResultBudget(msgs2, state, nil)

	// Frozen results should be unchanged.
	if extractTRContent(t, msgs2[0]) != small1 {
		t.Fatal("frozen id-a should not be replaced")
	}
	if extractTRContent(t, msgs2[1]) != small2 {
		t.Fatal("frozen id-b should not be replaced")
	}
	// New large result should be replaced.
	if !strings.Contains(extractTRContent(t, msgs2[2]), "Tool output too large") {
		t.Fatal("new large result should be replaced")
	}
}

func TestEnforceToolResultBudget_PreviewContainsOriginalContent(t *testing.T) {
	state := NewReplacementState()

	// Create content with a recognizable prefix.
	content := "UNIQUE_PREFIX_" + strings.Repeat("X", PerMessageBudget+1000)
	msgs := []api.Message{makeTRMsg("id-preview", content)}

	EnforceToolResultBudget(msgs, state, nil)

	preview := extractTRContent(t, msgs[0])
	if !strings.Contains(preview, "UNIQUE_PREFIX_") {
		t.Fatal("preview should contain the first ~2KB of original content")
	}
}

// ── ReplacementState tests ──────────────────────────────────────────────────

func TestNewReplacementState(t *testing.T) {
	s := NewReplacementState()
	if len(s.SeenIDs) != 0 || len(s.Replacements) != 0 {
		t.Fatal("new state should be empty")
	}
}

// ── formatCompactSummary tests ──────────────────────────────────────────────

func TestFormatCompactSummary_ExtractsSummaryBlock(t *testing.T) {
	raw := `<analysis>some analysis here</analysis>
<summary>This is the actual summary content.</summary>`
	result := formatCompactSummary(raw)
	if result != "This is the actual summary content." {
		t.Fatalf("expected summary content; got: %q", result)
	}
}

func TestFormatCompactSummary_StripsAnalysis(t *testing.T) {
	raw := `<analysis>
long analysis...
</analysis>
<summary>Summary only.</summary>`
	result := formatCompactSummary(raw)
	if strings.Contains(result, "analysis") {
		t.Fatalf("analysis content should be stripped; got: %q", result)
	}
}

// ── isAlreadyCompacted tests ─────────────────────────────────────────────────

func TestIsAlreadyCompacted(t *testing.T) {
	tests := []struct {
		content string
		want    bool
	}{
		{"[Tool output too large (68000 bytes)...]", true},
		{"[result cleared — 68000 bytes]", true},
		{"[Read result for /file.go cleared (68000 bytes)]", true},
		{"[Old tool result content cleared]", true},
		{"[content cleared — 5120 bytes]", true},
		{"[tool result persisted to disk — 68000 bytes total]", true},
		{"actual file content that is long enough to not be a stub", false},
		{"short", false},
		{"", false},
	}
	for _, tt := range tests {
		got := isAlreadyCompacted(tt.content)
		if got != tt.want {
			t.Errorf("isAlreadyCompacted(%q) = %v, want %v", tt.content[:min(40, len(tt.content))], got, tt.want)
		}
	}
}

func TestFormatCompactSummary_FallbackNoTags(t *testing.T) {
	raw := "Plain text summary without XML tags."
	result := formatCompactSummary(raw)
	if result != raw {
		t.Fatalf("fallback should return raw text; got: %q", result)
	}
}
