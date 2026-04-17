package session

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/Abraxas-365/claudio/internal/api"
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

// FindByTitle looks up the most recent session with the given title in projectDir.
// Returns (nil, nil) when no matching session exists.
func (s *Session) FindByTitle(title, projectDir string) (*storage.Session, error) {
	return s.db.GetSessionByTitle(title, projectDir)
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

// RenameByID updates the title of any session by ID.
func (s *Session) RenameByID(id, title string) error {
	return s.db.UpdateSessionTitle(id, title)
}

// PersistCompacted replaces all DB messages for the session with the given compacted
// api.Message slice. Call this after a successful Compact() to ensure that resuming
// the session loads the compacted history, not the original uncompacted messages.
func (s *Session) PersistCompacted(messages []api.Message) error {
	if s.current == nil {
		return nil
	}
	if err := s.db.DeleteAllMessages(s.current.ID); err != nil {
		return fmt.Errorf("delete old messages: %w", err)
	}
	for _, msg := range messages {
		// Try to decode as a generic content-block array (covers text, tool_use, tool_result).
		var blocks []json.RawMessage
		if err := json.Unmarshal(msg.Content, &blocks); err != nil {
			// Fallback: plain string content
			var text string
			if json.Unmarshal(msg.Content, &text) == nil && text != "" {
				_ = s.db.AddMessage(s.current.ID, msg.Role, text, msg.Role, "", "")
			}
			continue
		}

		type minBlock struct {
			Type      string          `json:"type"`
			Text      string          `json:"text"`
			ID        string          `json:"id"`
			Name      string          `json:"name"`
			Input     json.RawMessage `json:"input"`
			ToolUseID string          `json:"tool_use_id"`
			Content   string          `json:"content"`
		}

		if msg.Role == "assistant" {
			// Collect text and tool_use blocks; write text row first, then tool_use rows.
			var textBuf string
			var toolUses []minBlock
			for _, raw := range blocks {
				var b minBlock
				if json.Unmarshal(raw, &b) != nil {
					continue
				}
				switch b.Type {
				case "text", "thinking":
					textBuf += b.Text
				case "tool_use":
					toolUses = append(toolUses, b)
				}
			}
			_ = s.db.AddMessage(s.current.ID, "assistant", textBuf, "assistant", "", "")
			for _, tu := range toolUses {
				inputStr := string(tu.Input)
				_ = s.db.AddMessage(s.current.ID, "assistant", inputStr, "tool_use", tu.ID, tu.Name)
			}
		} else {
			// user role: either plain text blocks or tool_result blocks
			var textBuf string
			hasToolResult := false
			for _, raw := range blocks {
				var b minBlock
				if json.Unmarshal(raw, &b) != nil {
					continue
				}
				switch b.Type {
				case "text":
					textBuf += b.Text
				case "tool_result":
					hasToolResult = true
					_ = s.db.AddMessage(s.current.ID, "user", b.Content, "tool_result", b.ToolUseID, "")
				}
			}
			if !hasToolResult && textBuf != "" {
				_ = s.db.AddMessage(s.current.ID, "user", textBuf, "user", "", "")
			}
		}
	}
	return nil
}

// DeleteAllMessages removes all messages from the current session.
func (s *Session) DeleteAllMessages() error {
	if s.current == nil {
		return nil
	}
	return s.db.DeleteAllMessages(s.current.ID)
}

// Delete removes a session by ID.
func (s *Session) Delete(id string) error {
	return s.db.DeleteSession(id)
}

// Search finds sessions matching the query string.
func (s *Session) Search(query string, limit int) ([]storage.Session, error) {
	if query == "" {
		return s.db.ListSessions(limit)
	}
	return s.db.SearchSessions(query, limit)
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
