package learning

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---- helpers ----------------------------------------------------------------

func newStoreInTemp(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "instincts.json")
	return NewStore(path), path
}

func makeInstinct(pattern, response, category string) *Instinct {
	return &Instinct{
		Pattern:  pattern,
		Response: response,
		Category: category,
	}
}

// ---- Store creation / persistence ------------------------------------------

func TestNewStore_EmptyFile(t *testing.T) {
	store, _ := newStoreInTemp(t)
	if got := store.All(); len(got) != 0 {
		t.Fatalf("expected empty store, got %d instincts", len(got))
	}
}

func TestNewStore_LoadsExistingData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "instincts.json")

	inst := &Instinct{
		ID:        "preloaded-1",
		Pattern:   "use table tests",
		Response:  "write table-driven tests",
		Category:  "convention",
		Confidence: 0.8,
		CreatedAt: time.Now(),
	}
	data, _ := json.MarshalIndent([]*Instinct{inst}, "", "  ")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	store := NewStore(path)
	all := store.All()
	if len(all) != 1 {
		t.Fatalf("expected 1 instinct, got %d", len(all))
	}
	if all[0].ID != "preloaded-1" {
		t.Errorf("expected ID 'preloaded-1', got %q", all[0].ID)
	}
}

func TestNewStore_MissingFileIsOk(t *testing.T) {
	store := NewStore("/nonexistent/path/instincts.json")
	if len(store.All()) != 0 {
		t.Error("expected empty store when file is missing")
	}
}

// ---- Add -------------------------------------------------------------------

func TestAdd_AssignsDefaults(t *testing.T) {
	store, _ := newStoreInTemp(t)

	inst := &Instinct{Pattern: "error handling", Response: "always wrap errors"}
	store.Add(inst)

	all := store.All()
	if len(all) != 1 {
		t.Fatalf("expected 1 instinct, got %d", len(all))
	}
	got := all[0]

	if got.ID == "" {
		t.Error("expected non-empty ID")
	}
	if got.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
	if got.Confidence != 0.5 {
		t.Errorf("expected default confidence 0.5, got %f", got.Confidence)
	}
}

func TestAdd_PreservesExplicitID(t *testing.T) {
	store, _ := newStoreInTemp(t)

	inst := &Instinct{ID: "my-id", Pattern: "check nil", Response: "nil guard"}
	store.Add(inst)

	if got := store.All()[0].ID; got != "my-id" {
		t.Errorf("expected ID 'my-id', got %q", got)
	}
}

func TestAdd_PreservesExplicitConfidence(t *testing.T) {
	store, _ := newStoreInTemp(t)

	inst := &Instinct{Pattern: "lint", Response: "run linter", Confidence: 0.9}
	store.Add(inst)

	if got := store.All()[0].Confidence; got != 0.9 {
		t.Errorf("expected confidence 0.9, got %f", got)
	}
}

func TestAdd_DuplicatePatternUpdatesExisting(t *testing.T) {
	store, _ := newStoreInTemp(t)

	first := &Instinct{ID: "dup-1", Pattern: "same pattern", Response: "first response", Confidence: 0.5}
	store.Add(first)

	second := &Instinct{Pattern: "same pattern", Response: "updated response"}
	store.Add(second)

	all := store.All()
	if len(all) != 1 {
		t.Fatalf("expected 1 instinct after duplicate add, got %d", len(all))
	}
	got := all[0]
	if got.Response != "updated response" {
		t.Errorf("expected response 'updated response', got %q", got.Response)
	}
	// Confidence should have increased
	if got.Confidence <= 0.5 {
		t.Errorf("expected confidence > 0.5 after duplicate add, got %f", got.Confidence)
	}
	// UseCount should have bumped
	if got.UseCount != 1 {
		t.Errorf("expected UseCount 1, got %d", got.UseCount)
	}
}

func TestAdd_ConfidenceCapAt1(t *testing.T) {
	store, _ := newStoreInTemp(t)

	inst := &Instinct{Pattern: "cap test", Response: "response", Confidence: 0.95}
	store.Add(inst)
	// Add duplicate to trigger +0.1 bump
	store.Add(&Instinct{Pattern: "cap test", Response: "response"})

	if got := store.All()[0].Confidence; got > 1.0 {
		t.Errorf("confidence exceeded 1.0: %f", got)
	}
}

func TestAdd_PersistsToDisk(t *testing.T) {
	store, path := newStoreInTemp(t)
	store.Add(makeInstinct("persist test", "it should persist", "workflow"))

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading persisted file: %v", err)
	}
	if !strings.Contains(string(data), "persist test") {
		t.Error("persisted file does not contain expected pattern")
	}
}

// ---- Remove ----------------------------------------------------------------

func TestRemove_ByID(t *testing.T) {
	store, _ := newStoreInTemp(t)

	store.Add(&Instinct{ID: "remove-me", Pattern: "old pattern", Response: "old response"})
	store.Add(&Instinct{ID: "keep-me", Pattern: "keep pattern", Response: "keep response"})

	store.Remove("remove-me")

	all := store.All()
	if len(all) != 1 {
		t.Fatalf("expected 1 instinct after remove, got %d", len(all))
	}
	if all[0].ID != "keep-me" {
		t.Errorf("wrong instinct kept: %q", all[0].ID)
	}
}

func TestRemove_NonexistentID_NoOp(t *testing.T) {
	store, _ := newStoreInTemp(t)
	store.Add(makeInstinct("pattern", "response", "workflow"))

	store.Remove("does-not-exist")

	if len(store.All()) != 1 {
		t.Error("remove of nonexistent ID should not affect store")
	}
}

// ---- All -------------------------------------------------------------------

func TestAll_ReturnsCopy(t *testing.T) {
	store, _ := newStoreInTemp(t)
	store.Add(makeInstinct("p1", "r1", "workflow"))
	store.Add(makeInstinct("p2", "r2", "debugging"))

	all1 := store.All()
	all2 := store.All()
	if len(all1) != 2 || len(all2) != 2 {
		t.Fatalf("expected 2 instincts, got %d and %d", len(all1), len(all2))
	}
	// Modifying the returned slice should not affect the store
	all1[0] = nil
	if store.All()[0] == nil {
		t.Error("All() did not return a copy — mutation affected the store")
	}
}

// ---- FindRelevant ----------------------------------------------------------

func TestFindRelevant_MatchesByContains(t *testing.T) {
	store, _ := newStoreInTemp(t)

	store.Add(&Instinct{ID: "go-errors", Pattern: "error handling", Response: "wrap errors", Confidence: 0.8})
	store.Add(&Instinct{ID: "unrelated", Pattern: "css styling", Response: "use BEM", Confidence: 0.8})

	results := store.FindRelevant("when you encounter error handling in Go")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != "go-errors" {
		t.Errorf("unexpected result: %q", results[0].ID)
	}
}

func TestFindRelevant_CaseInsensitive(t *testing.T) {
	store, _ := newStoreInTemp(t)
	store.Add(&Instinct{ID: "ci", Pattern: "Error Handling", Response: "wrap", Confidence: 0.8})

	results := store.FindRelevant("error handling")
	if len(results) != 1 {
		t.Fatalf("expected 1 result for case-insensitive match, got %d", len(results))
	}
}

func TestFindRelevant_ExcludesLowConfidence(t *testing.T) {
	store, _ := newStoreInTemp(t)
	store.Add(&Instinct{ID: "low", Pattern: "low confidence", Response: "skip me", Confidence: 0.2})

	results := store.FindRelevant("low confidence")
	if len(results) != 0 {
		t.Errorf("expected low-confidence instinct to be excluded, got %d results", len(results))
	}
}

func TestFindRelevant_IncludesExactlyAt03(t *testing.T) {
	store, _ := newStoreInTemp(t)
	store.Add(&Instinct{ID: "boundary", Pattern: "boundary test", Response: "include me", Confidence: 0.3})

	results := store.FindRelevant("boundary test")
	if len(results) != 1 {
		t.Errorf("expected instinct with confidence 0.3 to be included, got %d", len(results))
	}
}

func TestFindRelevant_PatternContainsContext(t *testing.T) {
	// The other direction: pattern contains the (short) context string
	store, _ := newStoreInTemp(t)
	store.Add(&Instinct{ID: "broad", Pattern: "always handle errors in production code", Response: "wrap", Confidence: 0.9})

	results := store.FindRelevant("errors")
	if len(results) != 1 {
		t.Fatalf("expected 1 result when pattern contains context, got %d", len(results))
	}
}

func TestFindRelevant_EmptyContext(t *testing.T) {
	store, _ := newStoreInTemp(t)
	store.Add(&Instinct{ID: "x", Pattern: "some pattern", Response: "resp", Confidence: 0.8})

	// Empty string is contained in everything — strings.Contains("some pattern", "") == true
	results := store.FindRelevant("")
	if len(results) == 0 {
		t.Error("expected empty context to match all patterns (substring property)")
	}
}

// ---- ForSystemPrompt -------------------------------------------------------

func TestForSystemPrompt_EmptyWhenNoRelevant(t *testing.T) {
	store, _ := newStoreInTemp(t)
	out := store.ForSystemPrompt("completely unrelated context xyz123")
	if out != "" {
		t.Errorf("expected empty string, got: %q", out)
	}
}

func TestForSystemPrompt_ContainsPatternAndResponse(t *testing.T) {
	store, _ := newStoreInTemp(t)
	store.Add(&Instinct{
		ID:         "sp1",
		Pattern:    "table tests",
		Response:   "use subtests with t.Run",
		Confidence: 0.75,
	})

	out := store.ForSystemPrompt("writing table tests")
	if !strings.Contains(out, "table tests") {
		t.Error("output missing pattern")
	}
	if !strings.Contains(out, "use subtests with t.Run") {
		t.Error("output missing response")
	}
	if !strings.Contains(out, "75%") {
		t.Error("output missing confidence percentage")
	}
}

func TestForSystemPrompt_Header(t *testing.T) {
	store, _ := newStoreInTemp(t)
	store.Add(&Instinct{Pattern: "x", Response: "y", Confidence: 0.9})

	out := store.ForSystemPrompt("x")
	if !strings.Contains(out, "Learned Patterns") {
		t.Error("output missing header")
	}
}

// ---- MarkUsed --------------------------------------------------------------

func TestMarkUsed_UpdatesFields(t *testing.T) {
	store, _ := newStoreInTemp(t)

	inst := &Instinct{ID: "mark-me", Pattern: "mark pattern", Response: "mark response", Confidence: 0.5}
	store.Add(inst)

	before := store.All()[0].UseCount
	beforeConf := store.All()[0].Confidence

	store.MarkUsed("mark-me")

	all := store.All()
	if all[0].UseCount != before+1 {
		t.Errorf("UseCount should have incremented: got %d", all[0].UseCount)
	}
	if all[0].Confidence <= beforeConf {
		t.Errorf("Confidence should have increased: %f -> %f", beforeConf, all[0].Confidence)
	}
	if all[0].LastUsed.IsZero() {
		t.Error("LastUsed should be set")
	}
}

func TestMarkUsed_ConfidenceCapAt1(t *testing.T) {
	store, _ := newStoreInTemp(t)
	store.Add(&Instinct{ID: "full-conf", Pattern: "full", Response: "resp", Confidence: 0.99})
	store.MarkUsed("full-conf")

	if got := store.All()[0].Confidence; got > 1.0 {
		t.Errorf("confidence exceeded 1.0: %f", got)
	}
}

func TestMarkUsed_NonexistentID_NoOp(t *testing.T) {
	store, _ := newStoreInTemp(t)
	store.Add(makeInstinct("p", "r", "workflow"))
	initial := store.All()[0].UseCount

	store.MarkUsed("ghost-id")

	if store.All()[0].UseCount != initial {
		t.Error("MarkUsed with nonexistent ID should not modify existing instincts")
	}
}

// ---- Decay -----------------------------------------------------------------

func TestDecay_ReducesConfidenceForOldInstincts(t *testing.T) {
	store, _ := newStoreInTemp(t)

	oldTime := time.Now().AddDate(0, 0, -31) // 31 days ago — past the 30-day cutoff
	inst := &Instinct{
		ID:         "old",
		Pattern:    "old pattern",
		Response:   "old response",
		Confidence: 0.8,
		CreatedAt:  oldTime,
		LastUsed:   oldTime,
	}
	store.Add(inst)
	// Directly override the stored instance's confidence and LastUsed since Add may set defaults
	all := store.instincts
	all[0].Confidence = 0.8
	all[0].LastUsed = oldTime

	store.Decay()

	if got := store.All()[0].Confidence; got >= 0.8 {
		t.Errorf("expected confidence to decrease from 0.8, got %f", got)
	}
}

func TestDecay_DoesNotDecayRecentInstincts(t *testing.T) {
	store, _ := newStoreInTemp(t)

	inst := &Instinct{
		ID:         "fresh",
		Pattern:    "fresh pattern",
		Response:   "fresh response",
		Confidence: 0.8,
		LastUsed:   time.Now(), // just used
	}
	store.Add(inst)
	store.instincts[0].Confidence = 0.8
	store.instincts[0].LastUsed = time.Now()

	store.Decay()

	if got := store.All()[0].Confidence; got != 0.8 {
		t.Errorf("expected confidence to remain 0.8 for recent instinct, got %f", got)
	}
}

func TestDecay_ConfidenceFloorAt01(t *testing.T) {
	store, _ := newStoreInTemp(t)

	oldTime := time.Now().AddDate(0, 0, -31)
	inst := &Instinct{
		ID:         "almost-dead",
		Pattern:    "almost dead",
		Response:   "barely alive",
		Confidence: 0.15,
		LastUsed:   oldTime,
	}
	store.Add(inst)
	store.instincts[0].Confidence = 0.15
	store.instincts[0].LastUsed = oldTime

	store.Decay()

	if got := store.All()[0].Confidence; got < 0.1 {
		t.Errorf("confidence dropped below floor 0.1: %f", got)
	}
}

func TestDecay_NoLastUsed_NotDecayed(t *testing.T) {
	store, _ := newStoreInTemp(t)

	inst := &Instinct{
		ID:         "never-used",
		Pattern:    "never used",
		Response:   "response",
		Confidence: 0.7,
	}
	store.Add(inst)
	store.instincts[0].Confidence = 0.7
	// LastUsed is zero value — should not decay

	store.Decay()

	if got := store.All()[0].Confidence; got != 0.7 {
		t.Errorf("expected 0.7 for never-used instinct, got %f", got)
	}
}

// ---- Prune -----------------------------------------------------------------

func TestPrune_RemovesLowConfidence(t *testing.T) {
	store, _ := newStoreInTemp(t)

	store.Add(&Instinct{ID: "dead", Pattern: "dead pattern", Response: "resp", Confidence: 0.05})
	store.instincts[0].Confidence = 0.05 // override default 0.5

	removed := store.Prune()
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}
	if len(store.All()) != 0 {
		t.Error("expected empty store after pruning low-confidence instinct")
	}
}

func TestPrune_KeepsInstinctsAtBoundary(t *testing.T) {
	store, _ := newStoreInTemp(t)

	store.Add(&Instinct{ID: "boundary", Pattern: "boundary", Response: "keep", Confidence: 0.1})
	store.instincts[0].Confidence = 0.1

	removed := store.Prune()
	if removed != 0 {
		t.Errorf("expected 0 removed at boundary 0.1, got %d", removed)
	}
	if len(store.All()) != 1 {
		t.Error("instinct at confidence 0.1 should be kept")
	}
}

func TestPrune_MixedKeepsAndRemoves(t *testing.T) {
	store, _ := newStoreInTemp(t)

	store.Add(&Instinct{ID: "keep1", Pattern: "keep1", Response: "resp", Confidence: 0.6})
	store.Add(&Instinct{ID: "keep2", Pattern: "keep2", Response: "resp", Confidence: 0.3})
	store.Add(&Instinct{ID: "drop", Pattern: "drop", Response: "resp", Confidence: 0.05})

	// Override defaults
	for _, inst := range store.instincts {
		switch inst.ID {
		case "keep1":
			inst.Confidence = 0.6
		case "keep2":
			inst.Confidence = 0.3
		case "drop":
			inst.Confidence = 0.05
		}
	}

	removed := store.Prune()
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}
	if len(store.All()) != 2 {
		t.Errorf("expected 2 kept, got %d", len(store.All()))
	}
}

func TestPrune_NothingToRemove_ReturnsZero(t *testing.T) {
	store, _ := newStoreInTemp(t)

	store.Add(makeInstinct("solid", "response", "workflow"))
	// default confidence is 0.5 — well above 0.1

	removed := store.Prune()
	if removed != 0 {
		t.Errorf("expected 0 removed, got %d", removed)
	}
}

// ---- Persistence round-trip ------------------------------------------------

func TestPersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "instincts.json")

	store1 := NewStore(path)
	store1.Add(&Instinct{
		ID:         "rt-1",
		Pattern:    "round trip",
		Response:   "data survives restart",
		Category:   "convention",
		Confidence: 0.77,
	})

	// Create a new store from the same file — simulates process restart
	store2 := NewStore(path)
	all := store2.All()
	if len(all) != 1 {
		t.Fatalf("expected 1 instinct after reload, got %d", len(all))
	}
	if all[0].Pattern != "round trip" {
		t.Errorf("unexpected pattern: %q", all[0].Pattern)
	}
	if all[0].Confidence != 0.77 {
		t.Errorf("unexpected confidence: %f", all[0].Confidence)
	}
}

// ---- Concurrency -----------------------------------------------------------

func TestConcurrentAdd(t *testing.T) {
	store, _ := newStoreInTemp(t)

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			store.Add(&Instinct{
				Pattern:  strings.Repeat("p", n+1), // unique patterns
				Response: "response",
			})
		}(i)
	}
	wg.Wait()

	all := store.All()
	if len(all) != goroutines {
		t.Errorf("expected %d instincts, got %d", goroutines, len(all))
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	store, _ := newStoreInTemp(t)
	store.Add(makeInstinct("concurrent", "pattern", "workflow"))

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = store.All()
		}()
		go func() {
			defer wg.Done()
			store.MarkUsed(store.All()[0].ID)
		}()
	}
	wg.Wait()
}

// ---- SaveSessionLog --------------------------------------------------------

func TestSaveSessionLog_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	start := time.Date(2024, 6, 15, 10, 0, 0, 0, time.UTC)

	log := &SessionLog{
		SessionID: "sess-abc",
		StartTime: start,
		Model:     "claude-opus-4-5",
		Summary:   "test session",
	}

	if err := SaveSessionLog(dir, log); err != nil {
		t.Fatalf("SaveSessionLog failed: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 file, got %d", len(entries))
	}

	name := entries[0].Name()
	if !strings.HasPrefix(name, "2024-06-15") {
		t.Errorf("filename %q should start with '2024-06-15'", name)
	}
	if !strings.HasSuffix(name, ".json") {
		t.Errorf("filename %q should end with '.json'", name)
	}
}

func TestSaveSessionLog_ContentRoundTrip(t *testing.T) {
	dir := t.TempDir()
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	orig := &SessionLog{
		SessionID:   "rt-session",
		StartTime:   start,
		Model:       "claude-3-opus",
		ProjectDir:  "/home/user/project",
		Summary:     "worked on feature X",
		Decisions:   []string{"chose approach A"},
		Corrections: []string{"user corrected B"},
		Successes:   []string{"approach C worked"},
		Blockers:    []string{"issue D"},
	}

	if err := SaveSessionLog(dir, orig); err != nil {
		t.Fatalf("SaveSessionLog: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	data, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatalf("reading saved file: %v", err)
	}

	var loaded SessionLog
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if loaded.SessionID != orig.SessionID {
		t.Errorf("SessionID mismatch: %q vs %q", loaded.SessionID, orig.SessionID)
	}
	if loaded.Summary != orig.Summary {
		t.Errorf("Summary mismatch")
	}
	if len(loaded.Decisions) != 1 || loaded.Decisions[0] != "chose approach A" {
		t.Errorf("Decisions mismatch: %v", loaded.Decisions)
	}
}

func TestSaveSessionLog_CreatesDirectoryIfMissing(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "deep", "nested", "dir")

	log := &SessionLog{
		SessionID: "mkdir-test",
		StartTime: time.Now(),
	}

	if err := SaveSessionLog(dir, log); err != nil {
		t.Fatalf("expected SaveSessionLog to create missing dirs, got: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 file in newly created dir, got %d", len(entries))
	}
}

func TestSaveSessionLog_SessionIDSanitized(t *testing.T) {
	dir := t.TempDir()
	log := &SessionLog{
		SessionID: "path/with spaces/and slashes",
		StartTime: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
	}

	if err := SaveSessionLog(dir, log); err != nil {
		t.Fatalf("SaveSessionLog: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	name := entries[0].Name()
	if strings.Contains(name, "/") {
		t.Errorf("filename contains '/': %q", name)
	}
	if strings.Contains(name, " ") {
		t.Errorf("filename contains spaces: %q", name)
	}
}

// ---- sanitizeFilename (internal helper exercised indirectly) ---------------

func TestSanitizeFilename_Slashes(t *testing.T) {
	got := sanitizeFilename("a/b/c")
	if strings.Contains(got, "/") {
		t.Errorf("sanitizeFilename did not remove slashes: %q", got)
	}
}

func TestSanitizeFilename_Spaces(t *testing.T) {
	got := sanitizeFilename("hello world")
	if strings.Contains(got, " ") {
		t.Errorf("sanitizeFilename did not remove spaces: %q", got)
	}
}

func TestSanitizeFilename_Truncation(t *testing.T) {
	long := strings.Repeat("x", 60)
	got := sanitizeFilename(long)
	if len(got) > 50 {
		t.Errorf("sanitizeFilename did not truncate to 50 chars: len=%d", len(got))
	}
}

func TestSanitizeFilename_ShortStringsUnchanged(t *testing.T) {
	in := "short"
	got := sanitizeFilename(in)
	if got != in {
		t.Errorf("sanitizeFilename changed short string: %q -> %q", in, got)
	}
}
