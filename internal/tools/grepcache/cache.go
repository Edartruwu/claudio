// Package grepcache deduplicates repeated identical Grep calls within a session.
// For single-file searches the cache validates the file's mtime before returning
// a cached result. For directory searches no mtime check is performed — file
// contents rarely change mid-session and directory mtime is not reliable enough
// to detect nested changes cheaply.
package grepcache

import (
	"os"
	"sync"
	"time"
)

// Key uniquely identifies a grep invocation by all of its inputs.
type Key struct {
	Pattern    string
	Path       string
	Glob       string
	Type       string
	OutputMode string
	Context    int
	BeforeCtx  int
	AfterCtx   int
	IgnoreCase bool
	LineNumbers bool
	HeadLimit  int
	Offset     int
	Multiline  bool
}

type entry struct {
	result    string
	fileModAt time.Time // zero for non-file paths (directories / no path)
	isFile    bool      // true when Path is a regular file
}

// Cache stores recent grep results keyed by the full invocation parameters.
type Cache struct {
	mu      sync.Mutex
	entries map[Key]entry
	maxSize int
	order   []Key
}

// New creates a Cache with the given maximum number of entries.
func New(maxSize int) *Cache {
	if maxSize <= 0 {
		maxSize = 512
	}
	return &Cache{
		entries: make(map[Key]entry),
		maxSize: maxSize,
	}
}

// Get returns the cached result if it is still valid.
// For single-file searches validity means the file mtime has not changed.
// For directory searches the cached result is always considered valid.
func (c *Cache) Get(key Key) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	e, ok := c.entries[key]
	if !ok {
		return "", false
	}

	if e.isFile {
		info, err := os.Stat(key.Path)
		if err != nil || !info.ModTime().Equal(e.fileModAt) {
			delete(c.entries, key)
			return "", false
		}
	}

	return e.result, true
}

// Put stores a grep result. If path is a regular file its current mtime is
// recorded so the entry can be invalidated when the file changes.
func (c *Cache) Put(key Key, result string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	e := entry{result: result}

	if key.Path != "" {
		if info, err := os.Stat(key.Path); err == nil && !info.IsDir() {
			e.isFile = true
			e.fileModAt = info.ModTime()
		}
	}

	if _, exists := c.entries[key]; !exists {
		if len(c.entries) >= c.maxSize && len(c.order) > 0 {
			oldest := c.order[0]
			c.order = c.order[1:]
			delete(c.entries, oldest)
		}
		c.order = append(c.order, key)
	}

	c.entries[key] = e
}
