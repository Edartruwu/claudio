package grepcache_test

import (
	"os"
	"testing"

	"github.com/Abraxas-365/claudio/internal/tools/grepcache"
)

func TestCache_MissOnFirstCall(t *testing.T) {
	c := grepcache.New(64)
	key := grepcache.Key{Pattern: "foo", Path: "/some/dir"}
	if _, ok := c.Get(key); ok {
		t.Fatal("expected cache miss on empty cache")
	}
}

func TestCache_HitAfterPut_Directory(t *testing.T) {
	c := grepcache.New(64)
	key := grepcache.Key{Pattern: "TODO", Path: "/src"}
	c.Put(key, "found: TODO in file.go")

	got, ok := c.Get(key)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got != "found: TODO in file.go" {
		t.Fatalf("unexpected result: %q", got)
	}
}

func TestCache_HitAfterPut_File(t *testing.T) {
	// Create a real file so the mtime check in Get passes.
	f, err := os.CreateTemp(t.TempDir(), "grepcache-*.go")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	c := grepcache.New(64)
	key := grepcache.Key{Pattern: "func", Path: f.Name()}
	c.Put(key, "func main() {}")

	got, ok := c.Get(key)
	if !ok {
		t.Fatal("expected cache hit for unchanged file")
	}
	if got != "func main() {}" {
		t.Fatalf("unexpected result: %q", got)
	}
}

func TestCache_FileModifiedInvalidatesEntry(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/file.go"
	os.WriteFile(path, []byte("original"), 0600)

	c := grepcache.New(64)
	key := grepcache.Key{Pattern: "original", Path: path}
	c.Put(key, "original match")

	// Overwrite the file (mtime advances).
	os.WriteFile(path, []byte("changed"), 0600)

	if _, ok := c.Get(key); ok {
		t.Fatal("expected cache miss after file modification")
	}
}

func TestCache_DifferentKeysMiss(t *testing.T) {
	c := grepcache.New(64)
	key1 := grepcache.Key{Pattern: "foo", Path: "/src"}
	key2 := grepcache.Key{Pattern: "bar", Path: "/src"}
	c.Put(key1, "foo result")

	if _, ok := c.Get(key2); ok {
		t.Fatal("expected miss for different pattern")
	}
}

func TestCache_FlagsArePartOfKey(t *testing.T) {
	c := grepcache.New(64)
	base := grepcache.Key{Pattern: "foo", Path: "/src"}
	withFlag := grepcache.Key{Pattern: "foo", Path: "/src", IgnoreCase: true}

	c.Put(base, "case sensitive result")

	if _, ok := c.Get(withFlag); ok {
		t.Fatal("cache hit with different IgnoreCase flag — flags must be part of cache key")
	}
}

func TestCache_LRUEviction(t *testing.T) {
	const maxSize = 3
	c := grepcache.New(maxSize)

	// Fill cache to capacity.
	for i := 0; i < maxSize; i++ {
		key := grepcache.Key{Pattern: string(rune('a' + i))}
		c.Put(key, "result")
	}

	// Add one more entry — oldest should be evicted.
	newest := grepcache.Key{Pattern: "new"}
	c.Put(newest, "newest")

	oldest := grepcache.Key{Pattern: "a"}
	if _, ok := c.Get(oldest); ok {
		t.Fatal("oldest entry should have been evicted")
	}
	if _, ok := c.Get(newest); !ok {
		t.Fatal("newest entry should be present")
	}
}
