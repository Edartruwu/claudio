package memory

import (
	"fmt"
	"strings"
)

const (
	ScopeProject = "project"
	ScopeGlobal  = "global"
	ScopeAgent   = "agent"
)

// ScopedStore manages memory across multiple scopes: agent, project, and global.
// It provides a unified view of memories with priority: agent > project > global.
type ScopedStore struct {
	project *Store // project-level memories (default write target)
	global  *Store // global/user-level memories (fallback)
	agent   *Store // agent-level memories (set when running as crystallized agent)
}

// NewScopedStore creates a scoped memory store with project and global directories.
func NewScopedStore(projectDir, globalDir string) *ScopedStore {
	s := &ScopedStore{
		global: NewStore(globalDir),
	}
	if projectDir != "" {
		s.project = NewStore(projectDir)
	}
	return s
}

// SetAgentStore sets the agent-scoped memory store (used when running as a crystallized agent).
func (s *ScopedStore) SetAgentStore(dir string) {
	if dir != "" {
		s.agent = NewStore(dir)
	}
}

// Save writes a memory entry to the appropriate scope.
// If Entry.Scope is set, it writes to that scope. Otherwise defaults to project (or global if no project).
func (s *ScopedStore) Save(entry *Entry) error {
	target := s.writeTarget(entry.Scope)
	if target == nil {
		return fmt.Errorf("no memory store available for scope %q", entry.Scope)
	}
	return target.Save(entry)
}

// Remove deletes a memory entry from all scopes where it exists.
func (s *ScopedStore) Remove(name string) error {
	var lastErr error
	for _, store := range s.orderedStores() {
		if err := store.Remove(name); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// LoadAll returns all memories across all scopes, deduplicated by name.
// Priority: agent > project > global (higher priority wins on name conflict).
func (s *ScopedStore) LoadAll() []*Entry {
	seen := make(map[string]bool)
	var result []*Entry

	for _, ss := range s.orderedScopedStores() {
		for _, entry := range ss.store.LoadAll() {
			if !seen[entry.Name] {
				seen[entry.Name] = true
				entry.Scope = ss.scope
				result = append(result, entry)
			}
		}
	}
	return result
}

// FindRelevant returns memories relevant to the given context across all scopes.
func (s *ScopedStore) FindRelevant(context string) []*Entry {
	seen := make(map[string]bool)
	var result []*Entry

	for _, ss := range s.orderedScopedStores() {
		for _, entry := range ss.store.FindRelevant(context) {
			if !seen[entry.Name] {
				seen[entry.Name] = true
				entry.Scope = ss.scope
				result = append(result, entry)
			}
		}
	}
	return result
}

// LoadIndex returns the MEMORY.md index from the primary write target.
func (s *ScopedStore) LoadIndex() string {
	target := s.writeTarget("")
	if target == nil {
		return ""
	}
	return target.LoadIndex()
}

// BuildIndex returns a rich index across all scopes with scope headers.
// Format per entry: - name [tags]: description — "fact1" | "fact2"
func (s *ScopedStore) BuildIndex() string {
	var sb strings.Builder

	type scopeInfo struct {
		name  string
		store *Store
	}

	scopes := []scopeInfo{
		{"Global", s.global},
		{"Project", s.project},
		{"Agent", s.agent},
	}

	for _, scope := range scopes {
		if scope.store == nil {
			continue
		}
		lines := scope.store.BuildIndexLines()
		if lines == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("### %s Memories\n", scope.name))
		sb.WriteString(lines)
		sb.WriteString("\n")
	}

	return sb.String()
}

// Load returns a single entry by name from any scope.
func (s *ScopedStore) Load(name string) (*Entry, error) {
	for _, store := range s.orderedStores() {
		if entry, err := store.Load(name); err == nil {
			return entry, nil
		}
	}
	return nil, fmt.Errorf("memory %q not found", name)
}

// AppendFact appends a fact to an existing entry.
func (s *ScopedStore) AppendFact(name, fact string) error {
	for _, store := range s.orderedStores() {
		if _, err := store.Load(name); err == nil {
			return store.AppendFact(name, fact)
		}
	}
	return fmt.Errorf("memory %q not found", name)
}

// RemoveFact removes a fact by index from an existing entry.
func (s *ScopedStore) RemoveFact(name string, factIndex int) error {
	for _, store := range s.orderedStores() {
		if _, err := store.Load(name); err == nil {
			return store.RemoveFact(name, factIndex)
		}
	}
	return fmt.Errorf("memory %q not found", name)
}

// ReplaceFact replaces a fact by index in an existing entry.
func (s *ScopedStore) ReplaceFact(name string, factIndex int, newFact string) error {
	for _, store := range s.orderedStores() {
		if _, err := store.Load(name); err == nil {
			return store.ReplaceFact(name, factIndex, newFact)
		}
	}
	return fmt.Errorf("memory %q not found", name)
}

// ProjectStore returns the project-scoped store (for direct access if needed).
func (s *ScopedStore) ProjectStore() *Store { return s.project }

// GlobalStore returns the global store (for direct access if needed).
func (s *ScopedStore) GlobalStore() *Store { return s.global }

// AgentStore returns the agent-scoped store (for direct access if needed).
func (s *ScopedStore) AgentStore() *Store { return s.agent }

// writeTarget returns the store to write to based on the requested scope.
func (s *ScopedStore) writeTarget(scope string) *Store {
	switch scope {
	case ScopeGlobal:
		return s.global
	case ScopeAgent:
		if s.agent != nil {
			return s.agent
		}
		return s.project
	case ScopeProject:
		if s.project != nil {
			return s.project
		}
		return s.global
	default:
		// Default: project > global
		if s.project != nil {
			return s.project
		}
		return s.global
	}
}

// orderedStores returns stores in priority order (agent > project > global).
func (s *ScopedStore) orderedStores() []*Store {
	var stores []*Store
	if s.agent != nil {
		stores = append(stores, s.agent)
	}
	if s.project != nil {
		stores = append(stores, s.project)
	}
	if s.global != nil {
		stores = append(stores, s.global)
	}
	return stores
}

type scopedEntry struct {
	store *Store
	scope string
}

// orderedScopedStores returns stores paired with their scope label, in priority order.
func (s *ScopedStore) orderedScopedStores() []scopedEntry {
	var stores []scopedEntry
	if s.agent != nil {
		stores = append(stores, scopedEntry{s.agent, ScopeAgent})
	}
	if s.project != nil {
		stores = append(stores, scopedEntry{s.project, ScopeProject})
	}
	if s.global != nil {
		stores = append(stores, scopedEntry{s.global, ScopeGlobal})
	}
	return stores
}


