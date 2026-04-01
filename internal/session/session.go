package session

import (
	"fmt"
	"os"
	"time"

	"github.com/Abraxas-365/claudio/internal/storage"
)

// Session wraps the storage layer for session management.
type Session struct {
	db      *storage.DB
	current *storage.Session
}

// New creates a new session manager.
func New(db *storage.DB) *Session {
	return &Session{db: db}
}

// Start creates a new session for the current project.
func (s *Session) Start(model string) (*storage.Session, error) {
	cwd, _ := os.Getwd()
	sess, err := s.db.CreateSession(cwd, model)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}
	s.current = sess
	return sess, nil
}

// Current returns the active session.
func (s *Session) Current() *storage.Session {
	return s.current
}

// Resume loads a previous session by ID.
func (s *Session) Resume(id string) (*storage.Session, error) {
	sess, err := s.db.GetSession(id)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return nil, fmt.Errorf("session %s not found", id)
	}
	s.current = sess
	return sess, nil
}

// List returns recent sessions.
func (s *Session) List(limit int) ([]storage.Session, error) {
	return s.db.ListSessions(limit)
}

// AddMessage records a message in the current session.
func (s *Session) AddMessage(role, content, msgType string) error {
	if s.current == nil {
		return nil // No active session
	}
	return s.db.AddMessage(s.current.ID, role, content, msgType, "", "")
}

// AddToolMessage records a tool invocation message.
func (s *Session) AddToolMessage(role, content, msgType, toolUseID, toolName string) error {
	if s.current == nil {
		return nil
	}
	return s.db.AddMessage(s.current.ID, role, content, msgType, toolUseID, toolName)
}

// GetMessages retrieves all messages for the current session.
func (s *Session) GetMessages() ([]storage.MessageRecord, error) {
	if s.current == nil {
		return nil, nil
	}
	return s.db.GetMessages(s.current.ID)
}

// SetTitle updates the session title.
func (s *Session) SetTitle(title string) error {
	if s.current == nil {
		return nil
	}
	return s.db.UpdateSessionTitle(s.current.ID, title)
}

// SaveSummary stores the session summary (for compaction/end-of-session).
func (s *Session) SaveSummary(summary string) error {
	if s.current == nil {
		return nil
	}
	return s.db.UpdateSessionSummary(s.current.ID, summary)
}

// RecentForProject returns recent sessions for the current working directory.
func (s *Session) RecentForProject(limit int) ([]storage.Session, error) {
	all, err := s.db.ListSessions(limit * 3) // over-fetch, then filter
	if err != nil {
		return nil, err
	}

	cwd, _ := os.Getwd()
	var filtered []storage.Session
	for _, sess := range all {
		if sess.ProjectDir == cwd {
			filtered = append(filtered, sess)
			if len(filtered) >= limit {
				break
			}
		}
	}
	return filtered, nil
}

// LastSessionSummary returns the most recent session summary for context loading.
func (s *Session) LastSessionSummary() (string, time.Time, error) {
	sessions, err := s.RecentForProject(1)
	if err != nil || len(sessions) == 0 {
		return "", time.Time{}, err
	}
	return sessions[0].Summary, sessions[0].UpdatedAt, nil
}
