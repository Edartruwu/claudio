package readcache

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// ─── helpers ────────────────────────────────────────────────────────────────

// writeTempFile creates a temporary file with the given content and returns
// its path and the mtime reported by os.Stat. It also registers a cleanup
// function so the file is removed after the test.
func writeTempFile(t *testing.T, content string) (path string, modAt time.Time) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "readcache_test_*")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	info, err := os.Stat(f.Name())
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	return f.Name(), info.ModTime()
}

// ─── New ────────────────────────────────────────────────────────────────────

func TestNew_PositiveMaxSize(t *testing.T) {
	c := New(10)
	if c == nil {
		t.Fatal("New returned nil")
	}
	if c.maxSize != 10 {
		t.Errorf("maxSize = %d, want 10", c.maxSize)
	}
	if c.entries == nil {
		t.Error("entries map is nil")
	}
}

func TestNew_ZeroMaxSizeFallsBackTo256(t *testing.T) {
	c := New(0)
	if c.maxSize != 256 {
		t.Errorf("maxSize = %d, want 256", c.maxSize)
	}
}

func TestNew_NegativeMaxSizeFallsBackTo256(t *testing.T) {
	c := New(-99)
	if c.maxSize != 256 {
		t.Errorf("maxSize = %d, want 256", c.maxSize)
	}
}

func TestNew_MaxSizeOne(t *testing.T) {
	c := New(1)
	if c.maxSize != 1 {
		t.Errorf("maxSize = %d, want 1", c.maxSize)
	}
}

// ─── Put / Get round-trip ───────────────────────────────────────────────────

func TestPutGet_HitOnUnchangedFile(t *testing.T) {
	path, modAt := writeTempFile(t, "hello")
	c := New(8)
	key := Key{FilePath: path, Offset: 0, Limit: 100}

	c.Put(key, "hello", modAt)

	got, ok := c.Get(key)
	if !ok {
		t.Fatal("Get returned miss, want hit")
	}
	if got != "hello" {
		t.Errorf("content = %q, want %q", got, "hello")
	}
}

func TestGet_MissOnNonexistentKey(t *testing.T) {
	c := New(8)
	key := Key{FilePath: "/does/not/matter", Offset: 0, Limit: 10}
	_, ok := c.Get(key)
	if ok {
		t.Error("Get returned hit for key never stored")
	}
}

func TestGet_MissOnNonexistentFile(t *testing.T) {
	c := New(8)
	key := Key{FilePath: "/tmp/__readcache_no_such_file__", Offset: 0, Limit: 10}
	c.Put(key, "stale", time.Now())

	_, ok := c.Get(key)
	if ok {
		t.Error("Get returned hit for a file that does not exist")
	}
}

func TestGet_MissWhenFileModified(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mod.txt")
	if err := os.WriteFile(path, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	oldModAt := info.ModTime()

	c := New(8)
	key := Key{FilePath: path, Offset: 0, Limit: 100}
	c.Put(key, "v1", oldModAt)

	// Rewrite the file — force a new mtime that differs by at least 1 s so
	// filesystems with 1-second resolution still detect the change.
	newMtime := oldModAt.Add(2 * time.Second)
	if err := os.Chtimes(path, newMtime, newMtime); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	_, ok := c.Get(key)
	if ok {
		t.Error("Get returned hit after file mtime changed")
	}
	// Evicted entry should be gone from internal map too.
	if _, still := c.entries[key]; still {
		t.Error("stale entry was not removed from internal map")
	}
}

// ─── Key differentiation ───────────────────────────────────────────────────

func TestPut_DifferentKeysStoredIndependently(t *testing.T) {
	path, modAt := writeTempFile(t, "data")
	c := New(16)

	keys := []Key{
		{path, 0, 10},
		{path, 5, 10},
		{path, 0, 20},
	}
	for i, k := range keys {
		c.Put(k, fmt.Sprintf("content-%d", i), modAt)
	}
	for i, k := range keys {
		got, ok := c.Get(k)
		if !ok {
			t.Errorf("key %v: miss", k)
			continue
		}
		want := fmt.Sprintf("content-%d", i)
		if got != want {
			t.Errorf("key %v: got %q, want %q", k, got, want)
		}
	}
}

// ─── LRU eviction ──────────────────────────────────────────────────────────

func TestPut_EvictsOldestWhenFull(t *testing.T) {
	// maxSize = 2; we put 3 distinct keys — the first must be evicted.
	path, modAt := writeTempFile(t, "x")
	c := New(2)

	k1 := Key{path, 0, 1}
	k2 := Key{path, 1, 1}
	k3 := Key{path, 2, 1}

	c.Put(k1, "A", modAt)
	c.Put(k2, "B", modAt)
	c.Put(k3, "C", modAt) // k1 should be evicted

	// k1 must not be in the internal map (without triggering stat)
	if _, exists := c.entries[k1]; exists {
		t.Error("k1 was not evicted from entries map")
	}
	// k2 and k3 must still be present
	for _, k := range []Key{k2, k3} {
		if _, exists := c.entries[k]; !exists {
			t.Errorf("key %v missing from entries after eviction", k)
		}
	}
}

func TestPut_UpdateExistingKeyDoesNotGrowOrder(t *testing.T) {
	path, modAt := writeTempFile(t, "x")
	c := New(4)

	k := Key{path, 0, 10}
	c.Put(k, "v1", modAt)
	c.Put(k, "v2", modAt) // same key — should update, not append

	if len(c.order) != 1 {
		t.Errorf("order len = %d, want 1 after updating same key", len(c.order))
	}
	if c.entries[k].content != "v2" {
		t.Errorf("content = %q, want v2", c.entries[k].content)
	}
}

func TestPut_MaxSizeOneEvictsPreviousOnNew(t *testing.T) {
	path, modAt := writeTempFile(t, "x")
	c := New(1)

	k1 := Key{path, 0, 10}
	k2 := Key{path, 1, 10}

	c.Put(k1, "first", modAt)
	c.Put(k2, "second", modAt)

	if _, exists := c.entries[k1]; exists {
		t.Error("k1 should have been evicted by k2")
	}
	if _, exists := c.entries[k2]; !exists {
		t.Error("k2 should be present")
	}
}

// ─── Invalidate ─────────────────────────────────────────────────────────────

func TestInvalidate_RemovesAllEntriesForPath(t *testing.T) {
	path, modAt := writeTempFile(t, "y")
	other, otherMod := writeTempFile(t, "z")
	c := New(16)

	// Insert several entries for `path`
	for i := 0; i < 3; i++ {
		c.Put(Key{path, i, 10}, "val", modAt)
	}
	// Insert one entry for a different path
	c.Put(Key{other, 0, 10}, "other-val", otherMod)

	c.Invalidate(path)

	// All entries for `path` must be gone
	for k := range c.entries {
		if k.FilePath == path {
			t.Errorf("entry %v still present after Invalidate", k)
		}
	}
	// Entry for `other` must still be present
	if _, exists := c.entries[Key{other, 0, 10}]; !exists {
		t.Error("entry for other path was incorrectly removed")
	}
}

func TestInvalidate_UpdatesOrder(t *testing.T) {
	path, modAt := writeTempFile(t, "y")
	c := New(16)

	for i := 0; i < 3; i++ {
		c.Put(Key{path, i, 10}, "val", modAt)
	}
	c.Invalidate(path)

	for _, k := range c.order {
		if k.FilePath == path {
			t.Errorf("order still contains evicted path %s", path)
		}
	}
}

func TestInvalidate_NonexistentPathIsNoOp(t *testing.T) {
	path, modAt := writeTempFile(t, "z")
	c := New(8)
	c.Put(Key{path, 0, 10}, "content", modAt)

	// Should not panic
	c.Invalidate("/no/such/file")

	if len(c.entries) != 1 {
		t.Errorf("entries changed unexpectedly: len = %d", len(c.entries))
	}
}

func TestInvalidate_EmptyCache(t *testing.T) {
	c := New(8)
	// Must not panic
	c.Invalidate("/any/path")
}

// ─── Edge cases ─────────────────────────────────────────────────────────────

func TestPutGet_EmptyContent(t *testing.T) {
	path, modAt := writeTempFile(t, "")
	c := New(4)
	key := Key{path, 0, 0}
	c.Put(key, "", modAt)

	got, ok := c.Get(key)
	if !ok {
		t.Fatal("Get returned miss for empty content")
	}
	if got != "" {
		t.Errorf("content = %q, want empty string", got)
	}
}

func TestPutGet_ZeroOffsetZeroLimit(t *testing.T) {
	path, modAt := writeTempFile(t, "anything")
	c := New(4)
	key := Key{FilePath: path} // Offset=0, Limit=0 by zero-value
	c.Put(key, "anything", modAt)

	got, ok := c.Get(key)
	if !ok {
		t.Fatal("miss for zero-value Offset/Limit")
	}
	if got != "anything" {
		t.Errorf("got %q, want %q", got, "anything")
	}
}

func TestPutGet_LargeContent(t *testing.T) {
	large := make([]byte, 1<<20) // 1 MiB of zeros
	for i := range large {
		large[i] = byte(i % 256)
	}
	path, modAt := writeTempFile(t, string(large))
	c := New(4)
	key := Key{path, 0, len(large)}
	c.Put(key, string(large), modAt)

	got, ok := c.Get(key)
	if !ok {
		t.Fatal("miss for large content")
	}
	if got != string(large) {
		t.Error("large content mismatch")
	}
}

// ─── Key struct ─────────────────────────────────────────────────────────────

func TestKey_EqualityIsValueBased(t *testing.T) {
	k1 := Key{FilePath: "/a", Offset: 1, Limit: 2}
	k2 := Key{FilePath: "/a", Offset: 1, Limit: 2}
	if k1 != k2 {
		t.Error("identical Key structs should be equal")
	}
}

func TestKey_DifferentOffsetNotEqual(t *testing.T) {
	k1 := Key{FilePath: "/a", Offset: 1, Limit: 2}
	k2 := Key{FilePath: "/a", Offset: 2, Limit: 2}
	if k1 == k2 {
		t.Error("Keys with different Offset should not be equal")
	}
}

func TestKey_DifferentLimitNotEqual(t *testing.T) {
	k1 := Key{FilePath: "/a", Offset: 0, Limit: 10}
	k2 := Key{FilePath: "/a", Offset: 0, Limit: 20}
	if k1 == k2 {
		t.Error("Keys with different Limit should not be equal")
	}
}

// ─── Concurrency ────────────────────────────────────────────────────────────

func TestCache_ConcurrentPutGet(t *testing.T) {
	path, modAt := writeTempFile(t, "concurrent")
	c := New(64)

	const goroutines = 20
	const ops = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		g := g
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				key := Key{path, g, i}
				c.Put(key, fmt.Sprintf("g%d-i%d", g, i), modAt)
				c.Get(key)
			}
		}()
	}
	wg.Wait()
	// If we reach here without a race detector complaint the test passes.
}

func TestCache_ConcurrentInvalidate(t *testing.T) {
	path, modAt := writeTempFile(t, "inv")
	c := New(64)

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	for g := 0; g < goroutines; g++ {
		g := g
		// writers
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				c.Put(Key{path, g, i}, "v", modAt)
			}
		}()
		// invalidators
		go func() {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				c.Invalidate(path)
			}
		}()
	}
	wg.Wait()
}

// ─── Capacity boundary table tests ──────────────────────────────────────────

func TestPut_EvictionBoundary(t *testing.T) {
	path, modAt := writeTempFile(t, "boundary")

	tests := []struct {
		maxSize  int
		insertN  int
		wantSize int
	}{
		{1, 1, 1},
		{1, 3, 1},
		{3, 3, 3},
		{3, 5, 3},
		{5, 4, 4},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(fmt.Sprintf("max%d_insert%d", tc.maxSize, tc.insertN), func(t *testing.T) {
			c := New(tc.maxSize)
			for i := 0; i < tc.insertN; i++ {
				c.Put(Key{path, i, 1}, "v", modAt)
			}
			if got := len(c.entries); got != tc.wantSize {
				t.Errorf("entries len = %d, want %d", got, tc.wantSize)
			}
		})
	}
}

// ─── Stale-entry eviction is reflected in subsequent Get ────────────────────

func TestGet_EvictsStaleEntryFromMap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stale.txt")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	oldMod := info.ModTime()

	c := New(8)
	key := Key{path, 0, 100}
	c.Put(key, "old", oldMod)

	// Advance mtime so the entry becomes stale
	newMod := oldMod.Add(3 * time.Second)
	if err := os.Chtimes(path, newMod, newMod); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	_, ok := c.Get(key)
	if ok {
		t.Fatal("expected miss after mtime change")
	}
	if _, exists := c.entries[key]; exists {
		t.Error("stale entry should have been deleted from map on Get")
	}
}

// ─── Multiple files in same cache ───────────────────────────────────────────

func TestCache_MultipleFilesCoexist(t *testing.T) {
	p1, m1 := writeTempFile(t, "file1")
	p2, m2 := writeTempFile(t, "file2")

	c := New(8)
	k1 := Key{p1, 0, 5}
	k2 := Key{p2, 0, 5}

	c.Put(k1, "file1", m1)
	c.Put(k2, "file2", m2)

	got1, ok1 := c.Get(k1)
	got2, ok2 := c.Get(k2)

	if !ok1 || got1 != "file1" {
		t.Errorf("k1: ok=%v content=%q", ok1, got1)
	}
	if !ok2 || got2 != "file2" {
		t.Errorf("k2: ok=%v content=%q", ok2, got2)
	}
}

func TestInvalidate_OnlyTargetFileRemoved(t *testing.T) {
	p1, m1 := writeTempFile(t, "a")
	p2, m2 := writeTempFile(t, "b")

	c := New(16)
	for i := 0; i < 4; i++ {
		c.Put(Key{p1, i, 1}, "a", m1)
		c.Put(Key{p2, i, 1}, "b", m2)
	}

	c.Invalidate(p1)

	if len(c.entries) != 4 {
		t.Errorf("expected 4 entries for p2 after invalidating p1, got %d", len(c.entries))
	}
	for k := range c.entries {
		if k.FilePath == p1 {
			t.Errorf("entry for p1 still present: %v", k)
		}
	}
}
