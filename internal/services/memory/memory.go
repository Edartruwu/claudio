// Package memory provides persistent memory across sessions.
// Memories are stored as markdown files in ~/.claudio/memory/ with an
// index at MEMORY.md. This is the equivalent of Claude Code's memdir.
package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Abraxas-365/claudio/internal/storage"
)

const (
	maxIndexLines = 200
	maxIndexBytes = 25 * 1024 // 25KB

	TypeUser      = "user"
	TypeFeedback  = "feedback"
	TypeProject   = "project"
	TypeReference = "reference"
)

// Entry represents a single memory entry.
type Entry struct {
	Name        string
	Description string
	Type        string   // user, feedback, project, reference
	Scope       string   // project, global, agent (controls where Save writes)
	Facts       []string // PRIMARY: discrete one-liner facts
	Content     string   // COMPAT: populated from Facts on render, or from body for old entries
	FilePath    string
	Tags        []string
	Concepts    []string  // semantic tags, broader than Tags, auto-extracted or user-provided
	UpdatedAt   time.Time
}

// Store manages the memory directory.
type Store struct {
	dir string
	fts *storage.FTSIndex // optional; nil = no FTS write-through
}

// NewStore creates a new memory store backed by the given directory.
func NewStore(dir string) *Store {
	os.MkdirAll(dir, 0755)
	return &Store{dir: dir}
}

// Dir returns the directory backing this store.
func (s *Store) Dir() string { return s.dir }

// SetFTS attaches an FTS index to this store for write-through indexing.
func (s *Store) SetFTS(fts *storage.FTSIndex) { s.fts = fts }

// Save writes a memory entry to disk and updates the index.
// If Facts is empty but Content is set, Content is split into Facts (one fact per non-empty line).
func (s *Store) Save(entry *Entry) error {
	if entry.Name == "" {
		return fmt.Errorf("memory name required")
	}

	// If Facts is empty but Content is set, split Content into Facts
	if len(entry.Facts) == 0 && entry.Content != "" {
		for _, line := range strings.Split(entry.Content, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				entry.Facts = append(entry.Facts, line)
			}
		}
	}

	// Populate Content from Facts for compat
	entry.Content = strings.Join(entry.Facts, "\n")

	// Sanitize filename
	filename := sanitizeName(entry.Name) + ".md"
	path := filepath.Join(s.dir, filename)

	// Build content with frontmatter — facts stored as YAML list, no body
	var content strings.Builder
	content.WriteString("---\n")
	content.WriteString(fmt.Sprintf("name: %s\n", entry.Name))
	if entry.Description != "" {
		content.WriteString(fmt.Sprintf("description: %s\n", entry.Description))
	}
	content.WriteString(fmt.Sprintf("type: %s\n", entry.Type))
	if len(entry.Tags) > 0 {
		content.WriteString(fmt.Sprintf("tags: [%s]\n", strings.Join(entry.Tags, ", ")))
	}
	entry.UpdatedAt = time.Now()
	content.WriteString(fmt.Sprintf("updated_at: %s\n", entry.UpdatedAt.Format(time.RFC3339)))
	if len(entry.Facts) > 0 {
		content.WriteString("facts:\n")
		for _, fact := range entry.Facts {
			content.WriteString(fmt.Sprintf("  - %q\n", fact))
		}
	}
	if len(entry.Concepts) > 0 {
		content.WriteString("concepts:\n")
		for _, concept := range entry.Concepts {
			content.WriteString(fmt.Sprintf("  - %q\n", concept))
		}
	}
	content.WriteString("---\n")

	if err := os.WriteFile(path, []byte(content.String()), 0644); err != nil {
		return fmt.Errorf("writing memory: %w", err)
	}

	entry.FilePath = path

	// Best-effort FTS write-through (never fail the file write if FTS fails).
	// Scope must be set on the entry for FTS to work; ScopedStore sets it.
	if s.fts != nil && entry.Scope != "" {
		if info, statErr := os.Stat(path); statErr == nil {
			if ftsErr := s.fts.Upsert(
				entry.Name,
				entry.Scope,
				entry.Description,
				strings.Join(entry.Tags, " "),
				strings.Join(entry.Facts, " "),
				strings.Join(entry.Concepts, " "),
				info.ModTime().Unix(),
			); ftsErr != nil {
				fmt.Fprintf(os.Stderr, "memory fts upsert warning (%s): %v\n", entry.Name, ftsErr)
			}
		}
	}

	// Update index
	return s.updateIndex(entry)
}

// Load returns a single entry by name, or an error if not found.
func (s *Store) Load(name string) (*Entry, error) {
	entries := s.LoadAll()
	lowerName := strings.ToLower(name)

	// Exact match first
	for _, e := range entries {
		if e.Name == name {
			return e, nil
		}
	}
	// Case-insensitive match
	for _, e := range entries {
		if strings.ToLower(e.Name) == lowerName {
			return e, nil
		}
	}

	return nil, fmt.Errorf("memory %q not found", name)
}

// AppendFact loads an entry, appends a fact, and saves.
func (s *Store) AppendFact(name, fact string) error {
	entry, err := s.Load(name)
	if err != nil {
		return err
	}
	entry.Facts = append(entry.Facts, fact)
	return s.Save(entry)
}

// RemoveFact loads an entry, removes the fact at the given index, and saves.
func (s *Store) RemoveFact(name string, factIndex int) error {
	entry, err := s.Load(name)
	if err != nil {
		return err
	}
	if factIndex < 0 || factIndex >= len(entry.Facts) {
		return fmt.Errorf("fact index %d out of range (entry has %d facts)", factIndex, len(entry.Facts))
	}
	entry.Facts = append(entry.Facts[:factIndex], entry.Facts[factIndex+1:]...)
	return s.Save(entry)
}

// ReplaceFact loads an entry, replaces the fact at the given index, and saves.
func (s *Store) ReplaceFact(name string, factIndex int, newFact string) error {
	entry, err := s.Load(name)
	if err != nil {
		return err
	}
	if factIndex < 0 || factIndex >= len(entry.Facts) {
		return fmt.Errorf("fact index %d out of range (entry has %d facts)", factIndex, len(entry.Facts))
	}
	entry.Facts[factIndex] = newFact
	return s.Save(entry)
}

// Remove deletes a memory entry.
func (s *Store) Remove(name string) error {
	filename := sanitizeName(name) + ".md"
	path := filepath.Join(s.dir, filename)
	os.Remove(path)

	// Best-effort FTS delete (uses name-only delete since Store doesn't hold scope).
	if s.fts != nil {
		if ftsErr := s.fts.DeleteByName(name); ftsErr != nil {
			fmt.Fprintf(os.Stderr, "memory fts delete warning (%s): %v\n", name, ftsErr)
		}
	}

	// Rebuild index without this entry
	return s.rebuildIndex()
}

// LoadAll reads all memory entries.
func (s *Store) LoadAll() []*Entry {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil
	}

	var memories []*Entry
	for _, e := range entries {
		if e.IsDir() || e.Name() == "MEMORY.md" || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}

		path := filepath.Join(s.dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		entry := ParseEntry(string(data), path)
		if entry != nil {
			memories = append(memories, entry)
		}
	}
	return memories
}

// FindRelevant returns memories relevant to the given context string.
// Matches against name, description, facts, and tags.
func (s *Store) FindRelevant(context string) []*Entry {
	all := s.LoadAll()
	lower := strings.ToLower(context)

	var relevant []*Entry
	for _, entry := range all {
		factsStr := strings.Join(entry.Facts, " ")
		searchable := strings.ToLower(entry.Name + " " + entry.Description + " " + factsStr + " " + entry.Content + " " + strings.Join(entry.Tags, " "))
		if strings.Contains(searchable, lower) || containsAnyWord(lower, searchable) {
			relevant = append(relevant, entry)
		}
	}
	return relevant
}

// LoadIndex reads the MEMORY.md index file.
func (s *Store) LoadIndex() string {
	data, err := os.ReadFile(filepath.Join(s.dir, "MEMORY.md"))
	if err != nil {
		return ""
	}
	return string(data)
}

// BuildIndexLines returns a compact index with one line per entry.
// Format: - name [tags]: description — "fact1" | "fact2"
func (s *Store) BuildIndexLines() string {
	entries := s.LoadAll()
	if len(entries) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, e := range entries {
		sb.WriteString("- ")
		sb.WriteString(e.Name)

		if len(e.Tags) > 0 {
			sb.WriteString(" [")
			sb.WriteString(strings.Join(e.Tags, ","))
			sb.WriteString("]")
		}

		sb.WriteString(": ")
		sb.WriteString(e.Description)

		// Show first 2 facts, each truncated at 60 chars
		if len(e.Facts) > 0 {
			sb.WriteString(" — ")
			limit := len(e.Facts)
			if limit > 2 {
				limit = 2
			}
			for i := 0; i < limit; i++ {
				if i > 0 {
					sb.WriteString(" | ")
				}
				fact := e.Facts[i]
				if len(fact) > 60 {
					fact = fact[:57] + "..."
				}
				sb.WriteString(`"`)
				sb.WriteString(fact)
				sb.WriteString(`"`)
			}
		}

		sb.WriteString("\n")
	}
	return sb.String()
}

func (s *Store) updateIndex(entry *Entry) error {
	indexPath := filepath.Join(s.dir, "MEMORY.md")

	// Read existing index
	existing, _ := os.ReadFile(indexPath)
	content := string(existing)

	// Add new entry (avoid duplicates)
	relPath := entry.FilePath
	if rel, err := filepath.Rel(s.dir, entry.FilePath); err == nil {
		relPath = rel
	}

	line := fmt.Sprintf("- [%s](%s) — %s", entry.Name, relPath, entry.Description)

	if strings.Contains(content, entry.Name) {
		// Update existing line
		lines := strings.Split(content, "\n")
		for i, l := range lines {
			if strings.Contains(l, entry.Name) {
				lines[i] = line
				break
			}
		}
		content = strings.Join(lines, "\n")
	} else {
		if content == "" {
			content = "# Memory Index\n\n"
		}
		content += line + "\n"
	}

	// Enforce limits
	content = enforceIndexLimits(content)

	return os.WriteFile(indexPath, []byte(content), 0644)
}

func (s *Store) rebuildIndex() error {
	entries := s.LoadAll()

	var content strings.Builder
	content.WriteString("# Memory Index\n\n")

	for _, e := range entries {
		relPath := e.FilePath
		if rel, err := filepath.Rel(s.dir, e.FilePath); err == nil {
			relPath = rel
		}
		content.WriteString(fmt.Sprintf("- [%s](%s) — %s\n", e.Name, relPath, e.Description))
	}

	indexPath := filepath.Join(s.dir, "MEMORY.md")
	return os.WriteFile(indexPath, []byte(content.String()), 0644)
}

// ParseEntry parses a memory markdown file (with optional YAML frontmatter) into an Entry.
// Supports both new facts-based format and legacy body-content format.
func ParseEntry(content, path string) *Entry {
	lines := strings.Split(content, "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return &Entry{Content: content, FilePath: path}
	}

	endIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			endIdx = i
			break
		}
	}
	if endIdx < 0 {
		return &Entry{Content: content, FilePath: path}
	}

	entry := &Entry{FilePath: path}

	// Parse frontmatter including facts and concepts
	inFacts := false
	inConcepts := false
	for _, line := range lines[1:endIdx] {
		trimmed := strings.TrimSpace(line)

		// Handle facts list items (indented "- ..." lines)
		if inFacts {
			if strings.HasPrefix(trimmed, "- ") {
				fact := strings.TrimPrefix(trimmed, "- ")
				fact = strings.Trim(fact, `"'`)
				if fact != "" {
					entry.Facts = append(entry.Facts, fact)
				}
				continue
			}
			// Non-list line ends the facts block
			inFacts = false
		}

		// Handle concepts list items (indented "- ..." lines)
		if inConcepts {
			if strings.HasPrefix(trimmed, "- ") {
				concept := strings.TrimPrefix(trimmed, "- ")
				concept = strings.Trim(concept, `"'`)
				if concept != "" {
					entry.Concepts = append(entry.Concepts, concept)
				}
				continue
			}
			// Non-list line ends the concepts block
			inConcepts = false
		}

		if idx := strings.Index(trimmed, ":"); idx > 0 {
			key := strings.TrimSpace(trimmed[:idx])
			val := strings.TrimSpace(trimmed[idx+1:])
			val = strings.Trim(val, `"'`)
			switch key {
			case "name":
				entry.Name = val
			case "description":
				entry.Description = val
			case "type":
				entry.Type = val
			case "tags":
				// parse "[a, b, c]" format
				val = strings.Trim(val, "[]")
				for _, t := range strings.Split(val, ",") {
					t = strings.TrimSpace(t)
					if t != "" {
						entry.Tags = append(entry.Tags, t)
					}
				}
			case "updated_at":
				if t, err := time.Parse(time.RFC3339, val); err == nil {
					entry.UpdatedAt = t
				}
			case "facts":
				// Start parsing facts list on subsequent lines
				inFacts = true
			case "concepts":
				// Start parsing concepts list on subsequent lines
				inConcepts = true
			}
		}
	}

	// Backwards compat: if no facts parsed from frontmatter, use body content
	bodyContent := strings.TrimSpace(strings.Join(lines[endIdx+1:], "\n"))
	if len(entry.Facts) == 0 && bodyContent != "" {
		entry.Facts = []string{bodyContent}
	}

	// Populate Content from Facts for compat
	entry.Content = strings.Join(entry.Facts, "\n")

	if entry.Name == "" {
		entry.Name = strings.TrimSuffix(filepath.Base(path), ".md")
	}

	return entry
}

func sanitizeName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "/", "_")
	safe := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return -1
	}, name)
	if len(safe) > 50 {
		safe = safe[:50]
	}
	return safe
}

func enforceIndexLimits(content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) > maxIndexLines {
		lines = lines[:maxIndexLines]
		lines = append(lines, "\n... (index truncated at 200 lines)")
	}
	result := strings.Join(lines, "\n")
	if len(result) > maxIndexBytes {
		result = result[:maxIndexBytes] + "\n... (index truncated at 25KB)"
	}
	return result
}

func containsAnyWord(haystack, needleStr string) bool {
	words := strings.Fields(needleStr)
	for _, w := range words {
		if len(w) > 3 && strings.Contains(haystack, w) {
			return true
		}
	}
	return false
}
