package toolcache

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// ─── New ──────────────────────────────────────────────────────────────────────

func TestNew_CreatesDirectoryIfAbsent(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "sub", "cache")

	s, err := New(dir, 0)
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	if s == nil {
		t.Fatal("New returned nil Store")
	}

	if _, statErr := os.Stat(dir); os.IsNotExist(statErr) {
		t.Fatalf("New did not create directory %q", dir)
	}
}

func TestNew_ExistingDirectoryIsOK(t *testing.T) {
	dir := t.TempDir() // already exists

	s, err := New(dir, 1024)
	if err != nil {
		t.Fatalf("New on existing dir: unexpected error: %v", err)
	}
	if s == nil {
		t.Fatal("New returned nil Store")
	}
}

func TestNew_ZeroThresholdUsesDefault(t *testing.T) {
	dir := t.TempDir()

	s, err := New(dir, 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if s.threshold != defaultThreshold {
		t.Errorf("threshold = %d; want %d", s.threshold, defaultThreshold)
	}
}

func TestNew_NegativeThresholdUsesDefault(t *testing.T) {
	dir := t.TempDir()

	s, err := New(dir, -1)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if s.threshold != defaultThreshold {
		t.Errorf("threshold = %d; want %d", s.threshold, defaultThreshold)
	}
}

func TestNew_PositiveThresholdIsRetained(t *testing.T) {
	dir := t.TempDir()

	s, err := New(dir, 9999)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if s.threshold != 9999 {
		t.Errorf("threshold = %d; want 9999", s.threshold)
	}
}

func TestNew_IndexIsInitialised(t *testing.T) {
	dir := t.TempDir()

	s, err := New(dir, 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if s.index == nil {
		t.Error("index map is nil after New")
	}
	if len(s.index) != 0 {
		t.Errorf("index should be empty after New, got %d entries", len(s.index))
	}
}

// ─── MaybePersist ─────────────────────────────────────────────────────────────

func TestMaybePersist_BelowThreshold_ReturnsUnchanged(t *testing.T) {
	threshold := 100
	s := newTestStore(t, threshold)

	content := strings.Repeat("a", threshold) // exactly at limit → not over
	got := s.MaybePersist("id-1", content)

	if got != content {
		t.Errorf("expected content returned unchanged; got %q", got)
	}
}

func TestMaybePersist_AtThreshold_ReturnsUnchanged(t *testing.T) {
	threshold := 50
	s := newTestStore(t, threshold)

	content := strings.Repeat("x", threshold)
	got := s.MaybePersist("id-at", content)

	if got != content {
		t.Errorf("expected unchanged content at threshold; got %q", got)
	}
}

func TestMaybePersist_AboveThreshold_ReturnsSummary(t *testing.T) {
	threshold := 10
	s := newTestStore(t, threshold)

	content := strings.Repeat("b", threshold+1)
	got := s.MaybePersist("id-big", content)

	if !strings.Contains(got, "persisted to disk") {
		t.Errorf("expected placeholder; got: %q", got)
	}
}

func TestMaybePersist_AboveThreshold_FileWrittenToDisk(t *testing.T) {
	threshold := 10
	s := newTestStore(t, threshold)

	content := strings.Repeat("c", threshold+5)
	s.MaybePersist("id-file", content)

	expectedPath := filepath.Join(s.dir, "tr-id-file.txt")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("expected file %q to exist on disk", expectedPath)
	}
}

func TestMaybePersist_AboveThreshold_FileContentsMatch(t *testing.T) {
	threshold := 10
	s := newTestStore(t, threshold)

	content := strings.Repeat("d", threshold+5)
	s.MaybePersist("id-content", content)

	data, err := os.ReadFile(filepath.Join(s.dir, "tr-id-content.txt"))
	if err != nil {
		t.Fatalf("reading persisted file: %v", err)
	}
	if string(data) != content {
		t.Error("persisted file content does not match original")
	}
}

func TestMaybePersist_AboveThreshold_IndexUpdated(t *testing.T) {
	threshold := 10
	s := newTestStore(t, threshold)

	content := strings.Repeat("e", threshold+1)
	s.MaybePersist("id-index", content)

	s.mu.Lock()
	_, ok := s.index["id-index"]
	s.mu.Unlock()

	if !ok {
		t.Error("index was not updated after persisting")
	}
}

func TestMaybePersist_SummaryContainsTotalBytes(t *testing.T) {
	threshold := 10
	s := newTestStore(t, threshold)

	content := strings.Repeat("f", threshold+7)
	got := s.MaybePersist("id-bytes", content)

	expected := fmt.Sprintf("%d bytes total", len(content))
	if !strings.Contains(got, expected) {
		t.Errorf("placeholder %q missing %q", got, expected)
	}
}

func TestMaybePersist_PreviewLimitedTo2000Bytes(t *testing.T) {
	threshold := 10
	s := newTestStore(t, threshold)

	// Write much more than 2000 bytes over the threshold
	content := strings.Repeat("g", 5000)
	got := s.MaybePersist("id-preview", content)

	const previewSize = 2000
	// The placeholder line itself counts, so just check the full return is not the raw content
	// and that it contains the first 2000 bytes of content.
	if strings.Contains(got, content) {
		t.Error("raw oversized content should not appear verbatim in placeholder")
	}
	if !strings.Contains(got, content[:previewSize]) {
		t.Errorf("placeholder should contain first %d bytes of content as preview", previewSize)
	}
}

func TestMaybePersist_PreviewExactly2000WhenContentIs2001(t *testing.T) {
	threshold := 10
	s := newTestStore(t, threshold)

	content := strings.Repeat("h", 2001)
	got := s.MaybePersist("id-2001", content)

	// preview should be content[:2000]
	preview := content[:2000]
	if !strings.Contains(got, preview) {
		t.Error("placeholder should include first 2000 bytes when content is 2001 bytes")
	}
}

func TestMaybePersist_PreviewIsFullContentWhenUnder2000(t *testing.T) {
	threshold := 10
	s := newTestStore(t, threshold)

	// 15 bytes > threshold but < 2000
	content := strings.Repeat("i", 15)
	got := s.MaybePersist("id-small-over", content)

	if !strings.Contains(got, content) {
		t.Error("placeholder should contain the full content when it is < 2000 bytes")
	}
}

func TestMaybePersist_EmptyContent_ReturnedUnchanged(t *testing.T) {
	s := newTestStore(t, 0) // uses defaultThreshold

	got := s.MaybePersist("id-empty", "")
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestMaybePersist_MultipleDifferentIDs(t *testing.T) {
	threshold := 5
	s := newTestStore(t, threshold)

	ids := []string{"alpha", "beta", "gamma"}
	for _, id := range ids {
		content := strings.Repeat(id[0:1], threshold+1)
		s.MaybePersist(id, content)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range ids {
		if _, ok := s.index[id]; !ok {
			t.Errorf("index missing entry for id %q", id)
		}
	}
}

// ─── Get ──────────────────────────────────────────────────────────────────────

func TestGet_UnknownID_ReturnsFalse(t *testing.T) {
	s := newTestStore(t, 0)

	_, ok := s.Get("nonexistent-id")
	if ok {
		t.Error("Get returned true for an unknown tool-use ID")
	}
}

func TestGet_UnknownID_ReturnsEmptyString(t *testing.T) {
	s := newTestStore(t, 0)

	content, _ := s.Get("nonexistent-id")
	if content != "" {
		t.Errorf("expected empty string for unknown id, got %q", content)
	}
}

func TestGet_AfterPersist_ReturnsOriginalContent(t *testing.T) {
	threshold := 10
	s := newTestStore(t, threshold)

	original := strings.Repeat("j", threshold+10)
	s.MaybePersist("id-get", original)

	got, ok := s.Get("id-get")
	if !ok {
		t.Fatal("Get returned false after successful persist")
	}
	if got != original {
		t.Errorf("Get returned %q; want original content", got)
	}
}

func TestGet_BelowThreshold_ReturnsFalse(t *testing.T) {
	threshold := 100
	s := newTestStore(t, threshold)

	// Content is below threshold → not persisted → Get should return false
	content := strings.Repeat("k", threshold-1)
	s.MaybePersist("id-small", content)

	_, ok := s.Get("id-small")
	if ok {
		t.Error("Get returned true for content that was not persisted (below threshold)")
	}
}

func TestGet_FileDeletedExternally_ReturnsFalse(t *testing.T) {
	threshold := 5
	s := newTestStore(t, threshold)

	content := strings.Repeat("l", threshold+1)
	s.MaybePersist("id-del", content)

	// Simulate external deletion
	os.Remove(filepath.Join(s.dir, "tr-id-del.txt"))

	_, ok := s.Get("id-del")
	if ok {
		t.Error("Get should return false when the backing file has been removed")
	}
}

func TestGet_MultipleIDs_EachReturnsOwnContent(t *testing.T) {
	threshold := 5
	s := newTestStore(t, threshold)

	cases := map[string]string{
		"m1": strings.Repeat("m", threshold+1),
		"m2": strings.Repeat("n", threshold+2),
		"m3": strings.Repeat("o", threshold+3),
	}
	for id, content := range cases {
		s.MaybePersist(id, content)
	}

	for id, want := range cases {
		got, ok := s.Get(id)
		if !ok {
			t.Errorf("Get(%q) returned false", id)
			continue
		}
		if got != want {
			t.Errorf("Get(%q) = %q; want %q", id, got, want)
		}
	}
}

// ─── Cleanup ──────────────────────────────────────────────────────────────────

func TestCleanup_RemovesFilesFromDisk(t *testing.T) {
	threshold := 5
	s := newTestStore(t, threshold)

	ids := []string{"c1", "c2", "c3"}
	paths := make([]string, 0, len(ids))
	for _, id := range ids {
		s.MaybePersist(id, strings.Repeat("p", threshold+1))
		paths = append(paths, filepath.Join(s.dir, "tr-"+id+".txt"))
	}

	s.Cleanup()

	for _, p := range paths {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("file %q should have been removed by Cleanup", p)
		}
	}
}

func TestCleanup_ClearsIndex(t *testing.T) {
	threshold := 5
	s := newTestStore(t, threshold)

	for _, id := range []string{"c4", "c5"} {
		s.MaybePersist(id, strings.Repeat("q", threshold+1))
	}

	s.Cleanup()

	s.mu.Lock()
	n := len(s.index)
	s.mu.Unlock()

	if n != 0 {
		t.Errorf("index has %d entries after Cleanup; want 0", n)
	}
}

func TestCleanup_GetReturnsFalseAfterCleanup(t *testing.T) {
	threshold := 5
	s := newTestStore(t, threshold)

	s.MaybePersist("c6", strings.Repeat("r", threshold+1))
	s.Cleanup()

	_, ok := s.Get("c6")
	if ok {
		t.Error("Get should return false after Cleanup")
	}
}

func TestCleanup_IdempotentOnEmptyStore(t *testing.T) {
	s := newTestStore(t, 0)

	// Should not panic
	s.Cleanup()
	s.Cleanup()
}

func TestCleanup_AllowsReuseOfSameIDs(t *testing.T) {
	threshold := 5
	s := newTestStore(t, threshold)

	content := strings.Repeat("s", threshold+1)
	s.MaybePersist("reuse", content)
	s.Cleanup()

	// Persist again with the same ID
	s.MaybePersist("reuse", content)

	got, ok := s.Get("reuse")
	if !ok {
		t.Fatal("Get returned false after re-persisting the same ID post-Cleanup")
	}
	if got != content {
		t.Error("re-persisted content does not match original")
	}
}

// ─── Concurrency ──────────────────────────────────────────────────────────────

func TestMaybePersist_ConcurrentWrites_NoRace(t *testing.T) {
	threshold := 5
	s := newTestStore(t, threshold)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(n int) {
			defer wg.Done()
			id := fmt.Sprintf("conc-%d", n)
			content := strings.Repeat("t", threshold+n+1)
			s.MaybePersist(id, content)
		}(i)
	}
	wg.Wait()

	s.mu.Lock()
	n := len(s.index)
	s.mu.Unlock()

	if n != goroutines {
		t.Errorf("index has %d entries; want %d", n, goroutines)
	}
}

func TestGet_ConcurrentReads_NoRace(t *testing.T) {
	threshold := 5
	s := newTestStore(t, threshold)

	content := strings.Repeat("u", threshold+10)
	s.MaybePersist("shared", content)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for range goroutines {
		go func() {
			defer wg.Done()
			got, ok := s.Get("shared")
			if !ok {
				t.Errorf("concurrent Get returned false")
			}
			if got != content {
				t.Errorf("concurrent Get returned wrong content")
			}
		}()
	}
	wg.Wait()
}

func TestCleanup_ConcurrentWithPersist_NoRace(t *testing.T) {
	threshold := 5
	s := newTestStore(t, threshold)

	var wg sync.WaitGroup
	const goroutines = 20

	// Writers
	wg.Add(goroutines)
	for i := range goroutines {
		go func(n int) {
			defer wg.Done()
			id := fmt.Sprintf("race-%d", n)
			s.MaybePersist(id, strings.Repeat("v", threshold+1))
		}(i)
	}

	// Concurrent cleanups
	wg.Add(5)
	for range 5 {
		go func() {
			defer wg.Done()
			s.Cleanup()
		}()
	}

	wg.Wait()
}

// ─── Table-driven edge cases ──────────────────────────────────────────────────

func TestMaybePersist_TableDriven(t *testing.T) {
	threshold := 20

	cases := []struct {
		name        string
		id          string
		content     string
		wantInline  bool // true → returned string == content
		wantOnDisk  bool
	}{
		{
			name:       "empty string",
			id:         "td-empty",
			content:    "",
			wantInline: true,
			wantOnDisk: false,
		},
		{
			name:       "one byte",
			id:         "td-one",
			content:    "x",
			wantInline: true,
			wantOnDisk: false,
		},
		{
			name:       "exactly at threshold",
			id:         "td-at",
			content:    strings.Repeat("a", threshold),
			wantInline: true,
			wantOnDisk: false,
		},
		{
			name:       "one byte over threshold",
			id:         "td-over1",
			content:    strings.Repeat("b", threshold+1),
			wantInline: false,
			wantOnDisk: true,
		},
		{
			name:       "large content",
			id:         "td-large",
			content:    strings.Repeat("c", 10_000),
			wantInline: false,
			wantOnDisk: true,
		},
		{
			// "日" is 3 UTF-8 bytes; one rune is 3 bytes, well under threshold=20
			name:       "unicode multibyte",
			id:         "td-unicode",
			content:    "日本語", // 9 bytes — below threshold of 20
			wantInline: true,
			wantOnDisk: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Fresh store per sub-test to avoid cross-contamination
			s := newTestStore(t, threshold)

			got := s.MaybePersist(tc.id, tc.content)

			if tc.wantInline && got != tc.content {
				t.Errorf("wantInline=true but returned %q instead of original content", got)
			}
			if !tc.wantInline && got == tc.content {
				t.Error("wantInline=false but returned raw content unchanged")
			}

			filePath := filepath.Join(s.dir, "tr-"+tc.id+".txt")
			_, statErr := os.Stat(filePath)
			onDisk := !os.IsNotExist(statErr)

			if onDisk != tc.wantOnDisk {
				t.Errorf("file on disk = %v; want %v", onDisk, tc.wantOnDisk)
			}
		})
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// newTestStore creates a Store whose directory lives inside t.TempDir().
func newTestStore(t *testing.T, threshold int) *Store {
	t.Helper()
	s, err := New(t.TempDir(), threshold)
	if err != nil {
		t.Fatalf("newTestStore: %v", err)
	}
	return s
}
