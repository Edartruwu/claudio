package analytics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// getPricing
// ---------------------------------------------------------------------------

func TestGetPricing_ExactMatch(t *testing.T) {
	for model, want := range KnownPricing {
		got := getPricing(model)
		if got != want {
			t.Errorf("model %q: want %+v, got %+v", model, want, got)
		}
	}
}

func TestGetPricing_PrefixMatch(t *testing.T) {
	// A model string with a suffix that still starts with a known key.
	p := getPricing("claude-opus-4-5-extra-suffix")
	want := KnownPricing["claude-opus-4-5"]
	if p != want {
		t.Errorf("prefix match failed: want %+v, got %+v", want, p)
	}
}

func TestGetPricing_KeywordOpus(t *testing.T) {
	p := getPricing("my-custom-opus-model")
	if p.InputPerMTok != 15.0 || p.OutputPerMTok != 75.0 {
		t.Errorf("expected Opus pricing, got %+v", p)
	}
}

func TestGetPricing_KeywordHaiku(t *testing.T) {
	p := getPricing("super-haiku-v9")
	if p.InputPerMTok != 0.25 || p.OutputPerMTok != 1.25 {
		t.Errorf("expected Haiku pricing, got %+v", p)
	}
}

func TestGetPricing_UnknownDefaultsSonnet(t *testing.T) {
	p := getPricing("unknown-model-xyz")
	if p.InputPerMTok != 3.0 || p.OutputPerMTok != 15.0 {
		t.Errorf("expected default Sonnet pricing, got %+v", p)
	}
}

// ---------------------------------------------------------------------------
// NewTracker / basic field access
// ---------------------------------------------------------------------------

func TestNewTracker_Defaults(t *testing.T) {
	tr := NewTracker("claude-sonnet-4-5", 10.0, "")
	if tr.InputTokens() != 0 {
		t.Errorf("expected 0 input tokens, got %d", tr.InputTokens())
	}
	if tr.OutputTokens() != 0 {
		t.Errorf("expected 0 output tokens, got %d", tr.OutputTokens())
	}
	if tr.TotalTokens() != 0 {
		t.Errorf("expected 0 total tokens, got %d", tr.TotalTokens())
	}
	if tr.Cost() != 0 {
		t.Errorf("expected 0 cost, got %f", tr.Cost())
	}
	if tr.MaxBudget() != 10.0 {
		t.Errorf("expected budget 10.0, got %f", tr.MaxBudget())
	}
}

// ---------------------------------------------------------------------------
// RecordUsage / token tracking
// ---------------------------------------------------------------------------

func TestRecordUsage_AccumulatesTokens(t *testing.T) {
	tr := NewTracker("claude-sonnet-4-5", 0, "")
	tr.RecordUsage(100, 200, 0, 0)
	tr.RecordUsage(50, 75, 0, 0)

	if tr.InputTokens() != 150 {
		t.Errorf("want 150 input tokens, got %d", tr.InputTokens())
	}
	if tr.OutputTokens() != 275 {
		t.Errorf("want 275 output tokens, got %d", tr.OutputTokens())
	}
	if tr.TotalTokens() != 425 {
		t.Errorf("want 425 total tokens, got %d", tr.TotalTokens())
	}
}

func TestRecordUsage_IncreasesAPICallCount(t *testing.T) {
	tr := NewTracker("claude-sonnet-4-5", 0, "")
	tr.RecordUsage(10, 10, 0, 0)
	tr.RecordUsage(10, 10, 0, 0)

	report := tr.Report()
	if !strings.Contains(report, "API Calls:     2") {
		t.Errorf("expected 2 API calls in report, got:\n%s", report)
	}
}

// ---------------------------------------------------------------------------
// RecordToolCall
// ---------------------------------------------------------------------------

func TestRecordToolCall_Counter(t *testing.T) {
	tr := NewTracker("claude-sonnet-4-5", 0, "")
	for i := 0; i < 5; i++ {
		tr.RecordToolCall()
	}
	report := tr.Report()
	if !strings.Contains(report, "Tool Calls:    5") {
		t.Errorf("expected 5 tool calls in report, got:\n%s", report)
	}
}

// ---------------------------------------------------------------------------
// Cost calculation
// ---------------------------------------------------------------------------

func TestCost_SonnetPricing(t *testing.T) {
	tr := NewTracker("claude-sonnet-4-5", 0, "")
	// 1 million input + 1 million output
	tr.RecordUsage(1_000_000, 1_000_000, 0, 0)

	got := tr.Cost()
	// 3.0 + 15.0 = 18.0 USD
	want := 18.0
	if got != want {
		t.Errorf("want cost $%.4f, got $%.4f", want, got)
	}
}

func TestCost_OpusPricing(t *testing.T) {
	tr := NewTracker("claude-opus-4-5", 0, "")
	tr.RecordUsage(1_000_000, 1_000_000, 0, 0)

	got := tr.Cost()
	// 15.0 + 75.0 = 90.0 USD
	want := 90.0
	if got != want {
		t.Errorf("want cost $%.4f, got $%.4f", want, got)
	}
}

func TestCost_HaikuPricing(t *testing.T) {
	tr := NewTracker("claude-haiku-4-5-20251001", 0, "")
	tr.RecordUsage(1_000_000, 1_000_000, 0, 0)

	got := tr.Cost()
	// 0.25 + 1.25 = 1.50 USD
	want := 1.5
	if got != want {
		t.Errorf("want cost $%.4f, got $%.4f", want, got)
	}
}

func TestCost_FractionalTokens(t *testing.T) {
	tr := NewTracker("claude-sonnet-4-5", 0, "")
	tr.RecordUsage(500_000, 0, 0, 0)

	got := tr.Cost()
	// 500k / 1M * 3.0 = 1.50
	want := 1.5
	if got != want {
		t.Errorf("want cost $%.4f, got $%.4f", want, got)
	}
}

// ---------------------------------------------------------------------------
// Budget enforcement
// ---------------------------------------------------------------------------

func TestCheckBudget_NoLimit(t *testing.T) {
	tr := NewTracker("claude-sonnet-4-5", 0, "")
	tr.RecordUsage(1_000_000, 1_000_000, 0, 0)

	warn, exceeded := tr.CheckBudget()
	if warn != "" || exceeded {
		t.Errorf("expected no warning/exceeded, got warn=%q exceeded=%v", warn, exceeded)
	}
}

func TestCheckBudget_UnderBudget(t *testing.T) {
	tr := NewTracker("claude-sonnet-4-5", 100.0, "")
	tr.RecordUsage(100, 100, 0, 0) // tiny cost

	warn, exceeded := tr.CheckBudget()
	if warn != "" || exceeded {
		t.Errorf("expected no warning/exceeded, got warn=%q exceeded=%v", warn, exceeded)
	}
}

func TestCheckBudget_Warning_At80Percent(t *testing.T) {
	// Budget $1.00; Sonnet costs $18 per 1M+1M. Use just enough to hit ~85%.
	// We want cost ≈ $0.85 → need input such that input/1M*3.0 ≈ 0.85
	// 0.85 / 3.0 * 1M ≈ 283_334 input tokens, 0 output
	tr := NewTracker("claude-sonnet-4-5", 1.0, "")
	tr.RecordUsage(283_334, 0, 0, 0) // cost ≈ $0.850

	warn, exceeded := tr.CheckBudget()
	if exceeded {
		t.Errorf("should not be exceeded yet")
	}
	if !strings.Contains(warn, "Budget warning") {
		t.Errorf("expected budget warning, got %q", warn)
	}
}

func TestCheckBudget_Exceeded(t *testing.T) {
	tr := NewTracker("claude-sonnet-4-5", 1.0, "")
	// Cost = 18 USD, budget = 1 USD → exceeded
	tr.RecordUsage(1_000_000, 1_000_000, 0, 0)

	warn, exceeded := tr.CheckBudget()
	if !exceeded {
		t.Errorf("expected budget to be exceeded")
	}
	if !strings.Contains(warn, "Budget exceeded") {
		t.Errorf("expected 'Budget exceeded' in warning, got %q", warn)
	}
}

func TestCheckBudget_ExactlyAtBudget(t *testing.T) {
	// Budget = $3.0; Sonnet input cost: 1M tokens = $3.0 exactly
	tr := NewTracker("claude-sonnet-4-5", 3.0, "")
	tr.RecordUsage(1_000_000, 0, 0, 0)

	_, exceeded := tr.CheckBudget()
	if !exceeded {
		t.Errorf("expected exceeded when cost equals budget")
	}
}

// ---------------------------------------------------------------------------
// Report
// ---------------------------------------------------------------------------

func TestReport_ContainsExpectedFields(t *testing.T) {
	tr := NewTracker("claude-sonnet-4-5", 5.0, "")
	tr.RecordUsage(1000, 500, 0, 0)
	tr.RecordToolCall()

	report := tr.Report()

	checks := []string{
		"Session Analytics",
		"claude-sonnet-4-5",
		"Input Tokens:",
		"Output Tokens:",
		"Total Tokens:",
		"Estimated Cost:",
		"Budget:",
	}
	for _, s := range checks {
		if !strings.Contains(report, s) {
			t.Errorf("report missing %q:\n%s", s, report)
		}
	}
}

func TestReport_NoBudgetLine_WhenUnlimited(t *testing.T) {
	tr := NewTracker("claude-sonnet-4-5", 0, "")
	tr.RecordUsage(100, 100, 0, 0)

	report := tr.Report()
	if strings.Contains(report, "Budget:") {
		t.Errorf("expected no Budget line for unlimited tracker, got:\n%s", report)
	}
}

func TestReport_CorrectTokenCounts(t *testing.T) {
	tr := NewTracker("claude-sonnet-4-5", 0, "")
	tr.RecordUsage(123, 456, 0, 0)

	report := tr.Report()
	if !strings.Contains(report, "Input Tokens:  123") {
		t.Errorf("expected 'Input Tokens:  123' in report:\n%s", report)
	}
	if !strings.Contains(report, "Output Tokens: 456") {
		t.Errorf("expected 'Output Tokens: 456' in report:\n%s", report)
	}
	if !strings.Contains(report, "Total Tokens:  579") {
		t.Errorf("expected 'Total Tokens:  579' in report:\n%s", report)
	}
}

// ---------------------------------------------------------------------------
// SaveReport
// ---------------------------------------------------------------------------

func TestSaveReport_NoSaveDir_IsNoop(t *testing.T) {
	tr := NewTracker("claude-sonnet-4-5", 0, "")
	if err := tr.SaveReport("session-abc"); err != nil {
		t.Errorf("SaveReport with empty saveDir should not error, got: %v", err)
	}
}

func TestSaveReport_WritesValidJSON(t *testing.T) {
	dir := t.TempDir()
	tr := NewTracker("claude-sonnet-4-5", 10.0, dir)
	tr.RecordUsage(100, 200, 50, 30)
	tr.RecordToolCall()

	if err := tr.SaveReport("test-session-id"); err != nil {
		t.Fatalf("SaveReport failed: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 file, got %d", len(entries))
	}

	data, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	var report map[string]interface{}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, data)
	}

	checks := map[string]interface{}{
		"session_id":          "test-session-id",
		"model":               "claude-sonnet-4-5",
		"input_tokens":        float64(100),
		"output_tokens":       float64(200),
		"cache_read_tokens":   float64(50),
		"cache_create_tokens": float64(30),
		"total_tokens":        float64(380),
		"api_calls":           float64(1),
		"tool_calls":          float64(1),
		"max_budget":          float64(10.0),
	}
	for k, wantVal := range checks {
		got, ok := report[k]
		if !ok {
			t.Errorf("missing key %q in report", k)
			continue
		}
		if got != wantVal {
			t.Errorf("key %q: want %v, got %v", k, wantVal, got)
		}
	}
}

func TestSaveReport_ShortSessionID(t *testing.T) {
	dir := t.TempDir()
	tr := NewTracker("claude-sonnet-4-5", 0, dir)

	// Short session IDs (≤8 chars) should be used as-is in filename.
	if err := tr.SaveReport("abc"); err != nil {
		t.Fatalf("SaveReport failed: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 file, got %d", len(entries))
	}
	if !strings.Contains(entries[0].Name(), "abc") {
		t.Errorf("filename should contain session ID prefix 'abc', got %q", entries[0].Name())
	}
}

func TestSaveReport_LongSessionID_Truncated(t *testing.T) {
	dir := t.TempDir()
	tr := NewTracker("claude-sonnet-4-5", 0, dir)

	sessionID := "abcdefghijklmnopqrstuvwxyz"
	if err := tr.SaveReport(sessionID); err != nil {
		t.Fatalf("SaveReport failed: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 file, got %d", len(entries))
	}
	// Filename should contain the first 8 chars only.
	if !strings.Contains(entries[0].Name(), "abcdefgh") {
		t.Errorf("filename should contain 8-char prefix 'abcdefgh', got %q", entries[0].Name())
	}
	if strings.Contains(entries[0].Name(), "abcdefghi") {
		t.Errorf("filename should NOT contain more than 8 chars of session ID, got %q", entries[0].Name())
	}
}

// ---------------------------------------------------------------------------
// Cache token tracking
// ---------------------------------------------------------------------------

func TestRecordUsage_CacheTokens(t *testing.T) {
	tr := NewTracker("claude-sonnet-4-5", 0, "")
	tr.RecordUsage(1000, 500, 800, 200)
	tr.RecordUsage(1000, 500, 900, 100)

	if tr.CacheReadTokens() != 1700 {
		t.Errorf("want 1700 cache read tokens, got %d", tr.CacheReadTokens())
	}
	if tr.CacheCreateTokens() != 300 {
		t.Errorf("want 300 cache create tokens, got %d", tr.CacheCreateTokens())
	}
}

func TestCacheHitRate(t *testing.T) {
	tr := NewTracker("claude-sonnet-4-5", 0, "")
	tr.RecordUsage(200, 100, 800, 0) // 800/(200+800) = 80%

	rate := tr.CacheHitRate()
	if rate < 79.9 || rate > 80.1 {
		t.Errorf("want ~80%% cache hit rate, got %.1f%%", rate)
	}
}

func TestCacheHitRate_NoInput(t *testing.T) {
	tr := NewTracker("claude-sonnet-4-5", 0, "")
	if tr.CacheHitRate() != 0 {
		t.Errorf("expected 0 cache hit rate with no usage")
	}
}

func TestCost_WithCacheTokens(t *testing.T) {
	tr := NewTracker("claude-sonnet-4-5", 0, "")
	// 1M input + 1M output + 1M cache read + 1M cache create
	tr.RecordUsage(1_000_000, 1_000_000, 1_000_000, 1_000_000)

	got := tr.Cost()
	// input: 3.0 + output: 15.0 + cache_read: 0.3 + cache_write: 3.75 = 22.05
	want := 22.05
	if got != want {
		t.Errorf("want cost $%.4f, got $%.4f", want, got)
	}
}

// ---------------------------------------------------------------------------
// Concurrency
// ---------------------------------------------------------------------------

func TestConcurrentRecordUsage(t *testing.T) {
	tr := NewTracker("claude-sonnet-4-5", 0, "")
	const goroutines = 50
	const tokensPerGoroutine = 100

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tr.RecordUsage(tokensPerGoroutine, tokensPerGoroutine, 0, 0)
		}()
	}
	wg.Wait()

	want := goroutines * tokensPerGoroutine
	if tr.InputTokens() != want {
		t.Errorf("want %d input tokens, got %d", want, tr.InputTokens())
	}
	if tr.OutputTokens() != want {
		t.Errorf("want %d output tokens, got %d", want, tr.OutputTokens())
	}
}

func TestConcurrentRecordToolCall(t *testing.T) {
	tr := NewTracker("claude-sonnet-4-5", 0, "")
	const goroutines = 100

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tr.RecordToolCall()
		}()
	}
	wg.Wait()

	report := tr.Report()
	want := "Tool Calls:    100"
	if !strings.Contains(report, want) {
		t.Errorf("expected %q in report:\n%s", want, report)
	}
}
