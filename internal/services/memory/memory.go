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
	Type        string // user, feedback, project, reference
	Scope       string // project, global, agent (controls where Save writes)
	Content     string
	FilePath    string
	Tags        []string
	UpdatedAt   time.Time
}

// Store manages the memory directory.
type Store struct {
	dir string
}

// NewStore creates a new memory store backed by the given directory.
func NewStore(dir string) *Store {
	os.MkdirAll(dir, 0755)
	return &Store{dir: dir}
}

// Dir returns the directory backing this store.
func (s *Store) Dir() string { return s.dir }

// Save writes a memory entry to disk and updates the index.
func (s *Store) Save(entry *Entry) error {
	if entry.Name == "" {
		return fmt.Errorf("memory name required")
	}

	// Sanitize filename
	filename := sanitizeName(entry.Name) + ".md"
	path := filepath.Join(s.dir, filename)

	// Build content with frontmatter
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
	content.WriteString("---\n\n")
	content.WriteString(entry.Content)

	if err := os.WriteFile(path, []byte(content.String()), 0644); err != nil {
		return fmt.Errorf("writing memory: %w", err)
	}

	entry.FilePath = path

	// Update index
	return s.updateIndex(entry)
}

// Remove deletes a memory entry.
func (s *Store) Remove(name string) error {
	filename := sanitizeName(name) + ".md"
	path := filepath.Join(s.dir, filename)
	os.Remove(path)

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
// Matches against name, description, content, and tags.
func (s *Store) FindRelevant(context string) []*Entry {
	all := s.LoadAll()
	lower := strings.ToLower(context)

	var relevant []*Entry
	for _, entry := range all {
		searchable := strings.ToLower(entry.Name + " " + entry.Description + " " + entry.Content + " " + strings.Join(entry.Tags, " "))
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
	for _, line := range lines[1:endIdx] {
		line = strings.TrimSpace(line)
		if idx := strings.Index(line, ":"); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
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
			}
		}
	}

	entry.Content = strings.TrimSpace(strings.Join(lines[endIdx+1:], "\n"))
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


