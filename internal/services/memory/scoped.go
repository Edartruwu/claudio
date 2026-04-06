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
	for _, store := range s.allStores() {
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

	for _, store := range s.orderedStores() {
		for _, entry := range store.LoadAll() {
			if !seen[entry.Name] {
				seen[entry.Name] = true
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

	for _, store := range s.orderedStores() {
		for _, entry := range store.FindRelevant(context) {
			if !seen[entry.Name] {
				seen[entry.Name] = true
				result = append(result, entry)
			}
		}
	}
	return result
}

// ForSystemPrompt returns all memories formatted for the system prompt.
// Merges across scopes with deduplication, respecting the 25KB cap.
func (s *ScopedStore) ForSystemPrompt() string {
	memories := s.LoadAll()
	if len(memories) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("# Memories\n\n")
	sb.WriteString("The following memories from previous sessions may be relevant:\n\n")

	totalLen := 0
	for _, m := range memories {
		entry := fmt.Sprintf("## %s (%s)\n%s\n\n", m.Name, m.Type, m.Content)
		if totalLen+len(entry) > maxIndexBytes {
			sb.WriteString("... (additional memories truncated)\n")
			break
		}
		sb.WriteString(entry)
		totalLen += len(entry)
	}

	return sb.String()
}

// LoadIndex returns the MEMORY.md index from the primary write target.
func (s *ScopedStore) LoadIndex() string {
	target := s.writeTarget("")
	if target == nil {
		return ""
	}
	return target.LoadIndex()
}

// ProjectStore returns the project-scoped store (for direct access if needed).
func (s *ScopedStore) ProjectStore() *Store { return s.project }

// GlobalStore returns the global store (for direct access if needed).
func (s *ScopedStore) GlobalStore() *Store { return s.global }

// AgentStore returns the agent-scoped store (for direct access if needed).
func (s *ScopedStore) AgentStore() *Store { return s.agent }

// WriteTargetDir returns the directory the default Save() would write to.
// Priority matches writeTarget("") — project > global. Returns "" if no
// store is available.
func (s *ScopedStore) WriteTargetDir() string {
	target := s.writeTarget("")
	if target == nil {
		return ""
	}
	return target.Dir()
}

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

// allStores returns all non-nil stores (no priority order).
func (s *ScopedStore) allStores() []*Store {
	return s.orderedStores()
}
