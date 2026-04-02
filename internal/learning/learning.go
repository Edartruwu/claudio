// Package learning implements pattern extraction and instinct learning
// from sessions. Patterns discovered during work are saved and loaded
// in future sessions to avoid repeating mistakes.
package learning

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Instinct represents a learned pattern or behavior.
type Instinct struct {
	ID          string    `json:"id"`
	Pattern     string    `json:"pattern"`     // What triggers this instinct
	Response    string    `json:"response"`    // What to do when triggered
	Category    string    `json:"category"`    // "debugging", "workflow", "convention", "workaround"
	Confidence  float64   `json:"confidence"`  // 0.0 - 1.0
	Source      string    `json:"source"`      // "session", "user", "project"
	CreatedAt   time.Time `json:"created_at"`
	LastUsed    time.Time `json:"last_used"`
	UseCount    int       `json:"use_count"`
}

// Store manages learned instincts.
type Store struct {
	mu        sync.RWMutex
	instincts []*Instinct
	filePath  string
}

// NewStore creates a new learning store backed by a JSON file.
func NewStore(filePath string) *Store {
	s := &Store{filePath: filePath}
	s.load()
	return s
}

// Add adds a new instinct.
func (s *Store) Add(instinct *Instinct) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if instinct.ID == "" {
		instinct.ID = fmt.Sprintf("inst_%d", time.Now().UnixNano())
	}
	if instinct.CreatedAt.IsZero() {
		instinct.CreatedAt = time.Now()
	}
	if instinct.Confidence == 0 {
		instinct.Confidence = 0.5
	}

	// Check for duplicates
	for i, existing := range s.instincts {
		if existing.Pattern == instinct.Pattern {
			// Update existing
			s.instincts[i].Response = instinct.Response
			s.instincts[i].Confidence = min(1.0, existing.Confidence+0.1)
			s.instincts[i].UseCount++
			s.save()
			return
		}
	}

	s.instincts = append(s.instincts, instinct)
	s.save()
}

// Remove removes an instinct by ID.
func (s *Store) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, inst := range s.instincts {
		if inst.ID == id {
			s.instincts = append(s.instincts[:i], s.instincts[i+1:]...)
			s.save()
			return
		}
	}
}

// All returns all instincts.
func (s *Store) All() []*Instinct {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*Instinct, len(s.instincts))
	copy(result, s.instincts)
	return result
}

// FindRelevant returns instincts relevant to the given context.
func (s *Store) FindRelevant(context string) []*Instinct {
	s.mu.RLock()
	defer s.mu.RUnlock()

	lower := strings.ToLower(context)
	var relevant []*Instinct

	for _, inst := range s.instincts {
		if inst.Confidence < 0.3 {
			continue
		}
		pattern := strings.ToLower(inst.Pattern)
		if strings.Contains(lower, pattern) || strings.Contains(pattern, lower) {
			relevant = append(relevant, inst)
		}
	}

	return relevant
}

// ForSystemPrompt returns relevant instincts formatted for the system prompt.
func (s *Store) ForSystemPrompt(context string) string {
	relevant := s.FindRelevant(context)
	if len(relevant) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("# Learned Patterns\n\n")
	sb.WriteString("Based on previous sessions, keep these patterns in mind:\n\n")

	for _, inst := range relevant {
		sb.WriteString(fmt.Sprintf("- **%s**: %s (confidence: %.0f%%)\n",
			inst.Pattern, inst.Response, inst.Confidence*100))
	}

	return sb.String()
}

// MarkUsed updates the last-used time and use count for an instinct.
func (s *Store) MarkUsed(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, inst := range s.instincts {
		if inst.ID == id {
			inst.LastUsed = time.Now()
			inst.UseCount++
			inst.Confidence = min(1.0, inst.Confidence+0.05)
			s.save()
			return
		}
	}
}

// Decay reduces confidence of unused instincts over time.
func (s *Store) Decay() {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().AddDate(0, 0, -30) // 30 days
	changed := false

	for _, inst := range s.instincts {
		if !inst.LastUsed.IsZero() && inst.LastUsed.Before(cutoff) {
			inst.Confidence = max(0.1, inst.Confidence-0.1)
			changed = true
		}
	}

	if changed {
		s.save()
	}
}

// Prune removes instincts with very low confidence.
func (s *Store) Prune() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	var kept []*Instinct
	removed := 0
	for _, inst := range s.instincts {
		if inst.Confidence >= 0.1 {
			kept = append(kept, inst)
		} else {
			removed++
		}
	}

	if removed > 0 {
		s.instincts = kept
		s.save()
	}

	return removed
}

func (s *Store) load() {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return
	}
	json.Unmarshal(data, &s.instincts)
}

func (s *Store) save() {
	dir := filepath.Dir(s.filePath)
	os.MkdirAll(dir, 0755)

	data, err := json.MarshalIndent(s.instincts, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(s.filePath, data, 0644)
}

// SessionLog represents a session's activity for learning extraction.
type SessionLog struct {
	SessionID   string    `json:"session_id"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	Model       string    `json:"model"`
	ProjectDir  string    `json:"project_dir"`
	Summary     string    `json:"summary"`
	Decisions   []string  `json:"decisions"`
	Corrections []string  `json:"corrections"` // Things the user corrected
	Successes   []string  `json:"successes"`   // Approaches that worked
	Blockers    []string  `json:"blockers"`    // Issues encountered
}

// SaveSessionLog persists a session log for later analysis.
func SaveSessionLog(logDir string, log *SessionLog) error {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}

	filename := fmt.Sprintf("%s-%s.json",
		log.StartTime.Format("2006-01-02"),
		sanitizeFilename(log.SessionID))

	path := filepath.Join(logDir, filename)
	data, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

func sanitizeFilename(s string) string {
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, " ", "-")
	if len(s) > 50 {
		s = s[:50]
	}
	return s
}
