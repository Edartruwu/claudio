package prompts

import "sync"

// Section is a named, lazily-computed system prompt section with optional caching.
type Section struct {
	name     string
	compute  func() string
	mu       sync.Mutex
	cached   string
	computed bool
	uncached bool // if true, always recomputes (never cached)
}

var (
	sectionsMu  sync.RWMutex
	allSections []*Section
)

// NewSection registers a named cached section. Computed once, cached until ClearAllSections().
func NewSection(name string, compute func() string) *Section {
	s := &Section{name: name, compute: compute}
	sectionsMu.Lock()
	allSections = append(allSections, s)
	sectionsMu.Unlock()
	return s
}

// NewUncachedSection registers a section recomputed on every call (e.g. MCP instructions).
func NewUncachedSection(name string, compute func() string) *Section {
	s := &Section{name: name, compute: compute, uncached: true}
	sectionsMu.Lock()
	allSections = append(allSections, s)
	sectionsMu.Unlock()
	return s
}

// Get returns the section content, computing it if needed.
func (s *Section) Get() string {
	if s.uncached {
		return s.compute()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.computed {
		s.cached = s.compute()
		s.computed = true
	}
	return s.cached
}

// Clear resets the cached value so the next Get() recomputes it.
func (s *Section) Clear() {
	if s.uncached {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.computed = false
	s.cached = ""
}

// ClearAllSections resets every cached section. Call after /compact or /clear.
func ClearAllSections() {
	sectionsMu.RLock()
	defer sectionsMu.RUnlock()
	for _, s := range allSections {
		s.Clear()
	}
}
