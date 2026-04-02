package storage

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// Session represents a conversation session.
type Session struct {
	ID         string
	Title      string
	ProjectDir string
	Model      string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Summary    string
}

// MessageRecord represents a persisted message.
type MessageRecord struct {
	ID        int64
	SessionID string
	Role      string
	Content   string
	Type      string
	ToolUseID string
	ToolName  string
	CreatedAt time.Time
}

// CreateSession creates a new session.
func (db *DB) CreateSession(projectDir, model string) (*Session, error) {
	s := &Session{
		ID:         uuid.New().String(),
		ProjectDir: projectDir,
		Model:      model,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	_, err := db.conn.Exec(
		`INSERT INTO sessions (id, project_dir, model, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		s.ID, s.ProjectDir, s.Model, s.CreatedAt, s.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// UpdateSessionTitle updates the title of a session.
func (db *DB) UpdateSessionTitle(id, title string) error {
	_, err := db.conn.Exec(
		`UPDATE sessions SET title = ?, updated_at = ? WHERE id = ?`,
		title, time.Now(), id,
	)
	return err
}

// UpdateSessionSummary stores a session summary.
func (db *DB) UpdateSessionSummary(id, summary string) error {
	_, err := db.conn.Exec(
		`UPDATE sessions SET summary = ?, updated_at = ? WHERE id = ?`,
		summary, time.Now(), id,
	)
	return err
}

// GetSession retrieves a session by ID.
func (db *DB) GetSession(id string) (*Session, error) {
	s := &Session{}
	err := db.conn.QueryRow(
		`SELECT id, title, project_dir, model, created_at, updated_at, summary FROM sessions WHERE id = ?`,
		id,
	).Scan(&s.ID, &s.Title, &s.ProjectDir, &s.Model, &s.CreatedAt, &s.UpdatedAt, &s.Summary)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return s, err
}

// ListSessions returns recent sessions, newest first.
func (db *DB) ListSessions(limit int) ([]Session, error) {
	rows, err := db.conn.Query(
		`SELECT id, title, project_dir, model, created_at, updated_at, summary
		 FROM sessions ORDER BY updated_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.ID, &s.Title, &s.ProjectDir, &s.Model, &s.CreatedAt, &s.UpdatedAt, &s.Summary); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// DeleteSession removes a session and its messages.
func (db *DB) DeleteSession(id string) error {
	// Messages have ON DELETE CASCADE, but be explicit
	if _, err := db.conn.Exec(`DELETE FROM messages WHERE session_id = ?`, id); err != nil {
		return err
	}
	_, err := db.conn.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

// SearchSessions returns sessions matching the query string (fuzzy on title and project_dir).
func (db *DB) SearchSessions(query string, limit int) ([]Session, error) {
	pattern := "%" + query + "%"
	rows, err := db.conn.Query(
		`SELECT id, title, project_dir, model, created_at, updated_at, summary
		 FROM sessions
		 WHERE title LIKE ? OR project_dir LIKE ?
		 ORDER BY updated_at DESC LIMIT ?`,
		pattern, pattern, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.ID, &s.Title, &s.ProjectDir, &s.Model, &s.CreatedAt, &s.UpdatedAt, &s.Summary); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// AddMessage persists a message to a session.
func (db *DB) AddMessage(sessionID, role, content, msgType, toolUseID, toolName string) error {
	_, err := db.conn.Exec(
		`INSERT INTO messages (session_id, role, content, type, tool_use_id, tool_name) VALUES (?, ?, ?, ?, ?, ?)`,
		sessionID, role, content, msgType, toolUseID, toolName,
	)
	return err
}

// GetMessages retrieves all messages for a session.
func (db *DB) GetMessages(sessionID string) ([]MessageRecord, error) {
	rows, err := db.conn.Query(
		`SELECT id, session_id, role, content, type, tool_use_id, tool_name, created_at
		 FROM messages WHERE session_id = ? ORDER BY id ASC`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []MessageRecord
	for rows.Next() {
		var m MessageRecord
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &m.Type, &m.ToolUseID, &m.ToolName, &m.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}
