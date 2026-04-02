// Package toolcache offloads oversized tool results to disk instead of keeping them
// in the API message payload. This reduces tokens sent per turn for large outputs.
package toolcache

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const defaultThreshold = 50_000 // bytes

// Store manages on-disk storage of oversized tool results.
type Store struct {
	dir       string
	threshold int
	mu        sync.Mutex
	index     map[string]string // tool_use_id -> file path
}

// New creates a Store that writes to dir, persisting results larger than threshold bytes.
// Pass threshold=0 to use the default (50KB).
func New(dir string, threshold int) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("toolcache: create dir: %w", err)
	}
	if threshold <= 0 {
		threshold = defaultThreshold
	}
	return &Store{
		dir:       dir,
		threshold: threshold,
		index:     make(map[string]string),
	}, nil
}

// MaybePersist checks whether content exceeds the threshold.
// If it does, it writes the content to disk and returns a short placeholder string.
// If not, it returns the content unchanged.
func (s *Store) MaybePersist(toolUseID, content string) string {
	if len(content) <= s.threshold {
		return content
	}

	path := filepath.Join(s.dir, "tr-"+toolUseID+".txt")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		// Write failed — fall back to inline (truncated)
		return content
	}

	s.mu.Lock()
	s.index[toolUseID] = path
	s.mu.Unlock()

	return fmt.Sprintf("[tool result on disk: %s, %d bytes]", toolUseID, len(content))
}

// Get retrieves a previously persisted result. Returns ("", false) if not found.
func (s *Store) Get(toolUseID string) (string, bool) {
	s.mu.Lock()
	path, ok := s.index[toolUseID]
	s.mu.Unlock()
	if !ok {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return string(data), true
}

// Cleanup removes all persisted result files.
func (s *Store) Cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, path := range s.index {
		os.Remove(path)
	}
	s.index = make(map[string]string)
}
