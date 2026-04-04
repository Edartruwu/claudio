package tui

import (
	"strings"
	"testing"
)

// extractPlanFilePath replicates the exact logic in root.go's "tool_end" /
// EnterPlanMode branch so the tests stay in sync with production code.
func extractPlanFilePath(content string) string {
	const prefix = "Plan file: "
	idx := strings.Index(content, prefix)
	if idx < 0 {
		return ""
	}
	rest := content[idx+len(prefix):]
	if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
		rest = rest[:nl]
	}
	return strings.TrimSpace(rest)
}

// TestExtractPlanFilePath_HappyPath verifies that the path is correctly pulled
// from a typical EnterPlanMode result string.
func TestExtractPlanFilePath_HappyPath(t *testing.T) {
	content := `Entered plan mode. You should now focus on exploring the codebase and designing an implementation approach.

In plan mode, you should:
1. Thoroughly explore the codebase to understand existing patterns
2. Identify similar features and architectural approaches
3. Consider multiple approaches and their trade-offs
4. Use AskUser if you need to clarify the approach
5. Design a concrete implementation strategy
6. Write your plan to the Plan file: /home/user/.claudio/plans/plan-1234567890.md
7. When ready, use ExitPlanMode to present your plan for approval

Remember: DO NOT write or edit any files except the plan file. This is a read-only exploration and planning phase.`

	got := extractPlanFilePath(content)
	want := "/home/user/.claudio/plans/plan-1234567890.md"
	if got != want {
		t.Errorf("extractPlanFilePath() = %q, want %q", got, want)
	}
}

// TestExtractPlanFilePath_CapitalP_Required ensures a lowercase "plan file: "
// is NOT matched — only "Plan file: " (capital P) is recognised.
func TestExtractPlanFilePath_CapitalP_Required(t *testing.T) {
	content := "Write your plan to the plan file: /tmp/plan.md\nNext line"
	got := extractPlanFilePath(content)
	if got != "" {
		t.Errorf("expected empty string for lowercase prefix, got %q", got)
	}
}

// TestExtractPlanFilePath_StopsAtNewline verifies the parser does not bleed
// into subsequent lines of the instruction text.
func TestExtractPlanFilePath_StopsAtNewline(t *testing.T) {
	content := "Plan file: /tmp/plan-42.md\n7. When ready, use ExitPlanMode"
	got := extractPlanFilePath(content)
	want := "/tmp/plan-42.md"
	if got != want {
		t.Errorf("extractPlanFilePath() = %q, want %q (should stop at newline)", got, want)
	}
}

// TestExtractPlanFilePath_TrailingWhitespaceTrimmed ensures surrounding
// whitespace around the path is stripped.
func TestExtractPlanFilePath_TrailingWhitespaceTrimmed(t *testing.T) {
	content := "Plan file:   /tmp/plan.md   \nnext line"
	got := extractPlanFilePath(content)
	want := "/tmp/plan.md"
	if got != want {
		t.Errorf("extractPlanFilePath() = %q, want %q", got, want)
	}
}

// TestExtractPlanFilePath_MissingPrefix returns empty string when prefix is absent.
func TestExtractPlanFilePath_MissingPrefix(t *testing.T) {
	content := "No plan path here at all."
	got := extractPlanFilePath(content)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

// TestExtractPlanFilePath_EmptyContent handles empty input gracefully.
func TestExtractPlanFilePath_EmptyContent(t *testing.T) {
	got := extractPlanFilePath("")
	if got != "" {
		t.Errorf("expected empty string for empty content, got %q", got)
	}
}

// TestExtractPlanFilePath_PathWithoutTrailingNewline works when the prefix is
// at the very end of the string (no following newline).
func TestExtractPlanFilePath_PathWithoutTrailingNewline(t *testing.T) {
	content := "Plan file: /tmp/plan.md"
	got := extractPlanFilePath(content)
	want := "/tmp/plan.md"
	if got != want {
		t.Errorf("extractPlanFilePath() = %q, want %q", got, want)
	}
}

// TestExtractPlanFilePath_DoesNotContainSpaces verifies the extracted path has
// no spaces (which would indicate bleed-through from the surrounding text).
func TestExtractPlanFilePath_DoesNotContainSpaces(t *testing.T) {
	content := "Plan file: /home/user/.claudio/plans/plan-9999.md\n7. When ready, use ExitPlanMode to present"
	got := extractPlanFilePath(content)
	if strings.Contains(got, " ") {
		t.Errorf("extracted path %q contains spaces — likely bleed-through from instruction text", got)
	}
}

// TestExtractPlanFilePath_HasMdSuffix ensures the extracted path ends in .md
// as expected from the EnterPlanModeTool implementation.
func TestExtractPlanFilePath_HasMdSuffix(t *testing.T) {
	content := "Plan file: /home/user/.claudio/plans/plan-1609459200.md\nmore text"
	got := extractPlanFilePath(content)
	if !strings.HasSuffix(got, ".md") {
		t.Errorf("expected .md suffix, got %q", got)
	}
}
