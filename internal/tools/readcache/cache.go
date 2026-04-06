// Package readcache deduplicates repeated reads of the same file within a session.
// If the model reads the same path/offset/limit twice, the second call returns the
// cached result without re-reading disk (provided the file mtime hasn't changed).
package readcache

import (
	"os"
	"sync"
	"time"
)

// Key uniquely identifies a file read operation.
type Key struct {
	FilePath string
	Offset   int
	Limit    int
}

type entry struct {
	content   string
	fileModAt time.Time
}

// Cache stores recent file read results keyed by (path, offset, limit).
type Cache struct {
	mu        sync.Mutex
	entries   map[Key]entry
	maxSize   int
	order     []Key                // LRU eviction order
	writtenAt map[string]time.Time // files written this session, with the mtime at the time of the write
}

// New creates a Cache with the given maximum number of entries.
func New(maxSize int) *Cache {
	if maxSize <= 0 {
		maxSize = 256
	}
	return &Cache{
		entries:   make(map[Key]entry),
		maxSize:   maxSize,
		writtenAt: make(map[string]time.Time),
	}
}

// MarkWritten records that a file was written in this session at the given mtime.
// The Write tool uses this so subsequent Writes to the same file don't require a
// fresh Read — the model already knows what's in the file because it just wrote it.
func (c *Cache) MarkWritten(filePath string, modAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writtenAt[filePath] = modAt
}

// WrittenSince returns true if the file was written in this session and its mtime
// has not changed since.
func (c *Cache) WrittenSince(filePath string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	t, ok := c.writtenAt[filePath]
	if !ok {
		return false
	}
	info, err := os.Stat(filePath)
	if err != nil || !info.ModTime().Equal(t) {
		delete(c.writtenAt, filePath)
		return false
	}
	return true
}

// Get returns the cached content if the file hasn't changed since it was cached.
func (c *Cache) Get(key Key) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	e, ok := c.entries[key]
	if !ok {
		return "", false
	}

	// Validate: check file mtime hasn't changed
	info, err := os.Stat(key.FilePath)
	if err != nil || !info.ModTime().Equal(e.fileModAt) {
		// Stale — evict
		delete(c.entries, key)
		return "", false
	}
	return e.content, true
}

// Invalidate removes all cached entries for the given file path (all offset/limit variants).
func (c *Cache) Invalidate(filePath string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.writtenAt, filePath)
	for key := range c.entries {
		if key.FilePath == filePath {
			delete(c.entries, key)
		}
	}
	newOrder := c.order[:0]
	for _, k := range c.order {
		if k.FilePath != filePath {
			newOrder = append(newOrder, k)
		}
	}
	c.order = newOrder
}

// Put stores a read result. fileModAt should come from os.Stat on the file.
func (c *Cache) Put(key Key, content string, fileModAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.entries[key]; !exists {
		// Evict oldest entry if at capacity
		if len(c.entries) >= c.maxSize && len(c.order) > 0 {
			oldest := c.order[0]
			c.order = c.order[1:]
			delete(c.entries, oldest)
		}
		c.order = append(c.order, key)
	}

	c.entries[key] = entry{content: content, fileModAt: fileModAt}
}
