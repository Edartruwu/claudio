package tasks

import (
	"encoding/json"
	"os"
	"time"
)

// DreamState tracks when memory consolidation should run.
type DreamState struct {
	LastDreamAt   time.Time `json:"last_dream_at"`
	SessionsSince int       `json:"sessions_since"`
}

// LoadDreamState reads the dream scheduling state from disk.
func LoadDreamState(path string) *DreamState {
	data, err := os.ReadFile(path)
	if err != nil {
		return &DreamState{}
	}
	var state DreamState
	if err := json.Unmarshal(data, &state); err != nil {
		return &DreamState{}
	}
	return &state
}

// Save persists the dream state to disk.
func (s *DreamState) Save(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// RecordSession increments the session counter.
func (s *DreamState) RecordSession() {
	s.SessionsSince++
}

// RecordDream resets the state after a dream task completes.
func (s *DreamState) RecordDream() {
	s.LastDreamAt = time.Now()
	s.SessionsSince = 0
}

// ShouldDream returns true if enough time and sessions have passed
// to justify a memory consolidation run.
// Criteria: 24+ hours since last dream AND 5+ sessions since.
func (s *DreamState) ShouldDream() bool {
	if s.LastDreamAt.IsZero() {
		// Never dreamed before — dream after 3 sessions
		return s.SessionsSince >= 3
	}
	hoursSince := time.Since(s.LastDreamAt).Hours()
	return hoursSince >= 24 && s.SessionsSince >= 5
}
