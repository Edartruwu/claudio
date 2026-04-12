package outputfilter

import (
	"strings"
	"testing"
)

// ── MatchOutput ────
func TestMatchOutput_FirstRuleMatches(t *testing.T) {
	rules := []MatchRule{
		{Pattern: "error", Message: "found error"},
		{Pattern: "warning", Message: "found warning"},
	}
	msg, matched := MatchOutput("this is an error", rules)
	if !matched {
		t.Error("expected match")
	}
	if msg != "found error" {
		t.Errorf("expected 'found error', got %q", msg)
	}
}

func TestMatchOutput_NoMatch(t *testing.T) {
	rules := []MatchRule{
		{Pattern: "error", Message: "found error"},
		{Pattern: "warning", Message: "found warning"},
	}
	msg, matched := MatchOutput("all ok", rules)
	if matched {
		t.Error("expected no match")
	}
	if msg != "" {
		t.Errorf("expected empty message, got %q", msg)
	}
}

func TestMatchOutput_SecondRuleMatches(t *testing.T) {
	rules := []MatchRule{
		{Pattern: "error", Message: "found error"},
		{Pattern: "warning", Message: "found warning"},
	}
	msg, matched := MatchOutput("this is a warning", rules)
	if !matched {
		t.Error("expected match")
	}
	if msg != "found warning" {
		t.Errorf("expected 'found warning', got %q", msg)
	}
}

func TestMatchOutput_ShortCircuitsOnFirstMatch(t *testing.T) {
	rules := []MatchRule{
		{Pattern: "test", Message: "first match"},
		{Pattern: "test", Message: "second match"},
	}
	msg, _ := MatchOutput("test content", rules)
	if msg != "first match" {
		t.Errorf("expected short-circuit on first match, got %q", msg)
	}
}

func TestMatchOutput_UnlessGuardSkipsRule(t *testing.T) {
	rules := []MatchRule{
		{Pattern: "error", Message: "error found", Unless: "warning"},
		{Pattern: "error", Message: "error not warned"},
	}
	msg, matched := MatchOutput("error with warning", rules)
	if !matched {
		t.Error("expected match from second rule")
	}
	if msg != "error not warned" {
		t.Errorf("expected second match, got %q", msg)
	}
}

func TestMatchOutput_UnlessGuardDoesNotSkipIfNotMatched(t *testing.T) {
	rules := []MatchRule{
		{Pattern: "error", Message: "error found", Unless: "WARN"},
	}
	msg, matched := MatchOutput("error occurred in process", rules)
	if !matched {
		t.Error("expected match")
	}
	if msg != "error found" {
		t.Errorf("expected first match, got %q", msg)
	}
}

func TestMatchOutput_MalformedPatternSkipped(t *testing.T) {
	rules := []MatchRule{
		{Pattern: "[invalid(regex", Message: "bad regex"},
		{Pattern: "ok", Message: "good match"},
	}
	msg, matched := MatchOutput("ok", rules)
	if !matched {
		t.Error("expected match from second rule")
	}
	if msg != "good match" {
		t.Errorf("expected second match, got %q", msg)
	}
}

func TestMatchOutput_MalformedUnlessSkipped(t *testing.T) {
	rules := []MatchRule{
		{Pattern: "ok", Message: "good match", Unless: "[invalid(regex"},
	}
	msg, matched := MatchOutput("ok", rules)
	if !matched {
		t.Error("expected match despite malformed unless")
	}
	if msg != "good match" {
		t.Errorf("expected match, got %q", msg)
	}
}

func TestMatchOutput_MultilineBlob(t *testing.T) {
	rules := []MatchRule{
		{Pattern: "error", Message: "found error"},
	}
	input := "line1\nline2\nerror here\nline4"
	msg, matched := MatchOutput(input, rules)
	if !matched {
		t.Error("expected match across lines")
	}
	if msg != "found error" {
		t.Errorf("expected match, got %q", msg)
	}
}

// ── ApplyReplace ────
func TestApplyReplace_SingleRule(t *testing.T) {
	rules := []ReplaceRule{
		{Pattern: "old", Replacement: "new"},
	}
	got := ApplyReplace("old text\nold more", rules)
	if !strings.Contains(got, "new text") {
		t.Errorf("expected 'new text', got:\n%s", got)
	}
	if strings.Contains(got, "old") {
		t.Error("expected all 'old' replaced")
	}
}

func TestApplyReplace_ChainedRules(t *testing.T) {
	rules := []ReplaceRule{
		{Pattern: "foo", Replacement: "bar"},
		{Pattern: "bar", Replacement: "baz"},
	}
	got := ApplyReplace("foo is here", rules)
	if !strings.Contains(got, "baz") {
		t.Errorf("expected chained replacement to 'baz', got:\n%s", got)
	}
	if strings.Contains(got, "foo") {
		t.Error("expected 'foo' replaced")
	}
}

func TestApplyReplace_AppliedToAllLines(t *testing.T) {
	rules := []ReplaceRule{
		{Pattern: "\\d+", Replacement: "X"},
	}
	got := ApplyReplace("line1 123\nline2 456\nline3 789", rules)
	if strings.Count(got, "X") != 6 {
		t.Errorf("expected 6 replacements (2 per line), got:\n%s", got)
	}
}

func TestApplyReplace_MalformedRegexSkipped(t *testing.T) {
	rules := []ReplaceRule{
		{Pattern: "[invalid(regex", Replacement: "new"},
		{Pattern: "old", Replacement: "new"},
	}
	got := ApplyReplace("old text", rules)
	if !strings.Contains(got, "new text") {
		t.Errorf("expected 'new text' from second rule, got:\n%s", got)
	}
}

func TestApplyReplace_NoRules(t *testing.T) {
	got := ApplyReplace("original text", []ReplaceRule{})
	if got != "original text" {
		t.Errorf("expected unchanged, got:\n%s", got)
	}
}

func TestApplyReplace_RegexPattern(t *testing.T) {
	rules := []ReplaceRule{
		{Pattern: `\s+`, Replacement: " "},
	}
	got := ApplyReplace("too    many   spaces", rules)
	if !strings.Contains(got, "too many spaces") {
		t.Errorf("expected normalized spaces, got: %q", got)
	}
}

// ── KeepLinesMatching ────
func TestKeepLinesMatching_SinglePattern(t *testing.T) {
	patterns := []string{"error"}
	input := "line1\nerror line\nline3"
	got := KeepLinesMatching(input, patterns)
	if !strings.Contains(got, "error line") {
		t.Errorf("expected matching line kept, got:\n%s", got)
	}
	if strings.Contains(got, "line1") {
		t.Errorf("expected non-matching line dropped, got:\n%s", got)
	}
}

func TestKeepLinesMatching_MultiplePatterns(t *testing.T) {
	patterns := []string{"error", "warning"}
	input := "info line\nerror line\nwarning line"
	got := KeepLinesMatching(input, patterns)
	if !strings.Contains(got, "error line") || !strings.Contains(got, "warning line") {
		t.Errorf("expected matching lines kept, got:\n%s", got)
	}
	if strings.Contains(got, "info line") {
		t.Errorf("expected non-matching line dropped, got:\n%s", got)
	}
}

func TestKeepLinesMatching_EmptyLinesKept(t *testing.T) {
	patterns := []string{"match"}
	input := "match1\n\nmatch2\n"
	got := KeepLinesMatching(input, patterns)
	if !strings.Contains(got, "match1") || !strings.Contains(got, "match2") {
		t.Errorf("expected matches kept, got:\n%s", got)
	}
	// Check that empty lines are present (3 newlines after splits)
	lines := strings.Split(got, "\n")
	hasEmpty := false
	for _, line := range lines {
		if line == "" {
			hasEmpty = true
			break
		}
	}
	if !hasEmpty {
		t.Errorf("expected empty lines kept, got:\n%s", got)
	}
}

func TestKeepLinesMatching_NoPatterns(t *testing.T) {
	got := KeepLinesMatching("line1\nline2", []string{})
	if got != "line1\nline2" {
		t.Errorf("expected unchanged with no patterns, got:\n%s", got)
	}
}

func TestKeepLinesMatching_MalformedPatternSkipped(t *testing.T) {
	patterns := []string{"[invalid(regex", "valid"}
	input := "valid line\ninvalid"
	got := KeepLinesMatching(input, patterns)
	if !strings.Contains(got, "valid line") {
		t.Errorf("expected valid pattern applied, got:\n%s", got)
	}
}

// ── StripLinesMatching ────
func TestStripLinesMatching_SinglePattern(t *testing.T) {
	patterns := []string{"error"}
	input := "line1\nerror line\nline3"
	got := StripLinesMatching(input, patterns)
	if strings.Contains(got, "error line") {
		t.Errorf("expected matching line dropped, got:\n%s", got)
	}
	if !strings.Contains(got, "line1") {
		t.Errorf("expected non-matching line kept, got:\n%s", got)
	}
}

func TestStripLinesMatching_MultiplePatterns(t *testing.T) {
	patterns := []string{"error", "warning"}
	input := "info line\nerror line\nwarning line\ndone"
	got := StripLinesMatching(input, patterns)
	if strings.Contains(got, "error line") || strings.Contains(got, "warning line") {
		t.Errorf("expected matching lines dropped, got:\n%s", got)
	}
	if !strings.Contains(got, "info line") || !strings.Contains(got, "done") {
		t.Errorf("expected non-matching lines kept, got:\n%s", got)
	}
}

func TestStripLinesMatching_EmptyLinesNeverDropped(t *testing.T) {
	patterns := []string{"match"}
	input := "match\n\nmatch\n"
	got := StripLinesMatching(input, patterns)
	lines := strings.Split(got, "\n")
	hasEmpty := false
	for _, line := range lines {
		if line == "" {
			hasEmpty = true
			break
		}
	}
	if !hasEmpty {
		t.Errorf("expected empty lines kept, got:\n%s", got)
	}
}

func TestStripLinesMatching_NoPatterns(t *testing.T) {
	got := StripLinesMatching("line1\nline2", []string{})
	if got != "line1\nline2" {
		t.Errorf("expected unchanged with no patterns, got:\n%s", got)
	}
}

func TestStripLinesMatching_MalformedPatternSkipped(t *testing.T) {
	patterns := []string{"[invalid(regex", "remove"}
	input := "keep\nremove\nkeep"
	got := StripLinesMatching(input, patterns)
	if strings.Contains(got, "remove") {
		t.Errorf("expected 'remove' dropped, got:\n%s", got)
	}
	if !strings.Contains(got, "keep") {
		t.Errorf("expected 'keep' preserved, got:\n%s", got)
	}
}

// ── TailLines ────
func TestTailLines_LastNLines(t *testing.T) {
	input := "line1\nline2\nline3\nline4\nline5"
	got := TailLines(input, 3)
	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d: %v", len(lines), lines)
	}
	if !strings.Contains(got, "line3") || !strings.Contains(got, "line4") || !strings.Contains(got, "line5") {
		t.Errorf("expected last 3 lines, got:\n%s", got)
	}
	if strings.Contains(got, "line1") || strings.Contains(got, "line2") {
		t.Errorf("expected first 2 lines dropped, got:\n%s", got)
	}
}

func TestTailLines_ExactLineCount(t *testing.T) {
	input := "line1\nline2\nline3"
	got := TailLines(input, 3)
	if got != input {
		t.Errorf("expected unchanged when n >= line count, got:\n%s", got)
	}
}

func TestTailLines_MoreThanLineCount(t *testing.T) {
	input := "line1\nline2"
	got := TailLines(input, 10)
	if got != input {
		t.Errorf("expected unchanged when n > line count, got:\n%s", got)
	}
}

func TestTailLines_ZeroLines(t *testing.T) {
	input := "line1\nline2\nline3"
	got := TailLines(input, 0)
	// 0 lines returns empty result
	if got != "" {
		t.Errorf("expected empty for n=0, got:\n%s", got)
	}
}

func TestTailLines_NegativeLines(t *testing.T) {
	input := "line1\nline2"
	got := TailLines(input, -1)
	if got != input {
		t.Errorf("expected unchanged for negative n, got:\n%s", got)
	}
}

// ── HeadLines ────
func TestHeadLines_FirstNLines(t *testing.T) {
	input := "line1\nline2\nline3\nline4\nline5"
	got := HeadLines(input, 3)
	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d: %v", len(lines), lines)
	}
	if !strings.Contains(got, "line1") || !strings.Contains(got, "line2") || !strings.Contains(got, "line3") {
		t.Errorf("expected first 3 lines, got:\n%s", got)
	}
	if strings.Contains(got, "line4") || strings.Contains(got, "line5") {
		t.Errorf("expected last 2 lines dropped, got:\n%s", got)
	}
}

func TestHeadLines_ExactLineCount(t *testing.T) {
	input := "line1\nline2\nline3"
	got := HeadLines(input, 3)
	if got != input {
		t.Errorf("expected unchanged when n >= line count, got:\n%s", got)
	}
}

func TestHeadLines_MoreThanLineCount(t *testing.T) {
	input := "line1\nline2"
	got := HeadLines(input, 10)
	if got != input {
		t.Errorf("expected unchanged when n > line count, got:\n%s", got)
	}
}

func TestHeadLines_ZeroLines(t *testing.T) {
	input := "line1\nline2\nline3"
	got := HeadLines(input, 0)
	if got != "" {
		t.Errorf("expected empty for n=0, got:\n%s", got)
	}
}

func TestHeadLines_NegativeLines(t *testing.T) {
	input := "line1\nline2"
	got := HeadLines(input, -1)
	if got != input {
		t.Errorf("expected unchanged for negative n, got:\n%s", got)
	}
}

// ── MaxLines (alias for HeadLines) ────
func TestMaxLines_AliasForHeadLines(t *testing.T) {
	input := "line1\nline2\nline3\nline4\nline5"
	got1 := HeadLines(input, 3)
	got2 := MaxLines(input, 3)
	if got1 != got2 {
		t.Errorf("expected MaxLines to be identical to HeadLines")
	}
}

// ── TruncateLinesAt ────
func TestTruncateLinesAt_LongLines(t *testing.T) {
	input := "short\nthis is a very long line that should be truncated at 10 chars"
	got := TruncateLinesAt(input, 10)
	lines := strings.Split(got, "\n")
	if len(lines[0]) > 10 {
		t.Errorf("expected short line unchanged, got: %q", lines[0])
	}
	if len(lines[1]) > 10 {
		t.Errorf("expected long line truncated to 10 chars, got length %d: %q", len(lines[1]), lines[1])
	}
	if !strings.HasSuffix(lines[1], "...") {
		t.Errorf("expected '...' suffix on truncated line, got: %q", lines[1])
	}
}

func TestTruncateLinesAt_AllLinesBelow(t *testing.T) {
	input := "short\nok"
	got := TruncateLinesAt(input, 20)
	if got != input {
		t.Errorf("expected unchanged when all lines below limit, got:\n%s", got)
	}
}

func TestTruncateLinesAt_SmallLimit(t *testing.T) {
	input := "verylongline"
	got := TruncateLinesAt(input, 3)
	if !strings.HasSuffix(got, "...") {
		t.Errorf("expected '...' suffix, got: %q", got)
	}
	if len(got) > 3 {
		t.Errorf("expected max 3 chars for limit=3, got length %d: %q", len(got), got)
	}
}

func TestTruncateLinesAt_MultipleLines(t *testing.T) {
	input := "verylongline1\nshort\nveryverylongline2"
	got := TruncateLinesAt(input, 10)
	lines := strings.Split(got, "\n")
	for i, line := range lines {
		if len(line) > 10 {
			t.Errorf("line %d exceeds limit: length %d, got: %q", i, len(line), line)
		}
	}
}

// ── OnEmpty ────
func TestOnEmpty_WithContent(t *testing.T) {
	got := OnEmpty("some content", "fallback")
	if got != "some content" {
		t.Errorf("expected original content, got %q", got)
	}
}

func TestOnEmpty_Empty(t *testing.T) {
	got := OnEmpty("", "fallback")
	if got != "fallback" {
		t.Errorf("expected fallback, got %q", got)
	}
}

func TestOnEmpty_Whitespace(t *testing.T) {
	got := OnEmpty("  \n  \t  ", "fallback")
	if got != "fallback" {
		t.Errorf("expected fallback for whitespace-only, got %q", got)
	}
}

func TestOnEmpty_WithNewlines(t *testing.T) {
	got := OnEmpty("line1\nline2", "fallback")
	if got != "line1\nline2" {
		t.Errorf("expected original content, got %q", got)
	}
}

func TestOnEmpty_SingleSpace(t *testing.T) {
	got := OnEmpty(" ", "fallback")
	if got != "fallback" {
		t.Errorf("expected fallback for single space, got %q", got)
	}
}

// ── Integration tests ────
func TestKeepAndStripAreInverses(t *testing.T) {
	patterns := []string{"error"}
	input := "line1\nerror\nline3"

	keep := KeepLinesMatching(input, patterns)
	strip := StripLinesMatching(input, patterns)

	// Keep should have "error", Strip should not
	if !strings.Contains(keep, "error") {
		t.Errorf("expected keep to contain 'error', got:\n%s", keep)
	}
	if strings.Contains(strip, "error") {
		t.Errorf("expected strip to not contain 'error', got:\n%s", strip)
	}

	// Strip should have "line1" and "line3", Keep should not (unless pattern matches)
	if strings.Contains(keep, "line1") || strings.Contains(keep, "line3") {
		t.Errorf("expected keep to not contain non-matching lines, got:\n%s", keep)
	}
	if !strings.Contains(strip, "line1") || !strings.Contains(strip, "line3") {
		t.Errorf("expected strip to contain non-matching lines, got:\n%s", strip)
	}
}

func TestPipelineChain(t *testing.T) {
	input := "INFO: starting\nERROR: failed\nDEBUG: detailed info"

	// Strip debug, then keep only errors
	stripped := StripLinesMatching(input, []string{"DEBUG"})
	kept := KeepLinesMatching(stripped, []string{"ERROR"})

	if !strings.Contains(kept, "ERROR") {
		t.Errorf("expected ERROR in result, got:\n%s", kept)
	}
	if strings.Contains(kept, "DEBUG") || strings.Contains(kept, "INFO") {
		t.Errorf("expected only ERROR, got:\n%s", kept)
	}
}
