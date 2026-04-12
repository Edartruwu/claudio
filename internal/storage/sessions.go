package storage

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// Session represents a conversation session.
type Session struct {
	ID              string
	Title           string
	ProjectDir      string
	Model           string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Summary         string
	ParentSessionID string // non-empty for sub-agent sessions
	AgentType       string // e.g. "general-purpose", "Explore"
	TeamTemplate    string // e.g. "backend-team", optional template name
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

// CreateSubSession creates a session linked to a parent session (for sub-agents).
func (db *DB) CreateSubSession(parentID, agentType, projectDir, model string) (*Session, error) {
	s := &Session{
		ID:              uuid.New().String(),
		Title:           "[sub-agent] " + agentType,
		ProjectDir:      projectDir,
		Model:           model,
		ParentSessionID: parentID,
		AgentType:       agentType,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	_, err := db.conn.Exec(
		`INSERT INTO sessions (id, title, project_dir, model, parent_session_id, agent_type, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.Title, s.ProjectDir, s.Model, s.ParentSessionID, s.AgentType, s.CreatedAt, s.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// GetSubSessions returns all sub-agent sessions for a given parent session ID.
func (db *DB) GetSubSessions(parentID string) ([]Session, error) {
	rows, err := db.conn.Query(
		`SELECT id, title, project_dir, model, created_at, updated_at, summary, parent_session_id, agent_type, team_template
		 FROM sessions WHERE parent_session_id = ? ORDER BY created_at ASC`,
		parentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.ID, &s.Title, &s.ProjectDir, &s.Model, &s.CreatedAt, &s.UpdatedAt, &s.Summary, &s.ParentSessionID, &s.AgentType, &s.TeamTemplate); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
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
		`SELECT id, title, project_dir, model, created_at, updated_at, summary, parent_session_id, agent_type, team_template FROM sessions WHERE id = ?`,
		id,
	).Scan(&s.ID, &s.Title, &s.ProjectDir, &s.Model, &s.CreatedAt, &s.UpdatedAt, &s.Summary, &s.ParentSessionID, &s.AgentType, &s.TeamTemplate)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return s, err
}

// ListSessions returns recent top-level sessions (no sub-agent sessions), newest first.
func (db *DB) ListSessions(limit int) ([]Session, error) {
	rows, err := db.conn.Query(
		`SELECT id, title, project_dir, model, created_at, updated_at, summary, parent_session_id, agent_type, team_template
		 FROM sessions WHERE parent_session_id = '' ORDER BY updated_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.ID, &s.Title, &s.ProjectDir, &s.Model, &s.CreatedAt, &s.UpdatedAt, &s.Summary, &s.ParentSessionID, &s.AgentType, &s.TeamTemplate); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// ListSessionsByProject returns recent top-level sessions for a specific project, newest first.
func (db *DB) ListSessionsByProject(projectDir string, limit int) ([]Session, error) {
	rows, err := db.conn.Query(
		`SELECT id, title, project_dir, model, created_at, updated_at, summary, parent_session_id, agent_type, team_template
		 FROM sessions WHERE parent_session_id = '' AND project_dir = ?
		 ORDER BY updated_at DESC LIMIT ?`,
		projectDir, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.ID, &s.Title, &s.ProjectDir, &s.Model, &s.CreatedAt, &s.UpdatedAt, &s.Summary, &s.ParentSessionID, &s.AgentType, &s.TeamTemplate); err != nil {
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

// SearchSessions returns top-level sessions matching the query string (fuzzy on title and project_dir).
func (db *DB) SearchSessions(query string, limit int) ([]Session, error) {
	pattern := "%" + query + "%"
	rows, err := db.conn.Query(
		`SELECT id, title, project_dir, model, created_at, updated_at, summary, parent_session_id, agent_type, team_template
		 FROM sessions
		 WHERE parent_session_id = '' AND (title LIKE ? OR project_dir LIKE ?)
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
		if err := rows.Scan(&s.ID, &s.Title, &s.ProjectDir, &s.Model, &s.CreatedAt, &s.UpdatedAt, &s.Summary, &s.ParentSessionID, &s.AgentType, &s.TeamTemplate); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// AddMessage persists a message to a session and bumps the session's updated_at
// so the session list stays sorted by most-recent activity.
func (db *DB) AddMessage(sessionID, role, content, msgType, toolUseID, toolName string) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`INSERT INTO messages (session_id, role, content, type, tool_use_id, tool_name) VALUES (?, ?, ?, ?, ?, ?)`,
		sessionID, role, content, msgType, toolUseID, toolName,
	); err != nil {
		return err
	}
	// Only bump updated_at for user/assistant turns, not every tool call, to
	// avoid excessive write amplification on long tool chains.
	if msgType == "user" || msgType == "assistant" {
		if _, err := tx.Exec(
			`UPDATE sessions SET updated_at = ? WHERE id = ?`,
			time.Now(), sessionID,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteAllMessages removes all messages for a session (without deleting the session itself).
func (db *DB) DeleteAllMessages(sessionID string) error {
	_, err := db.conn.Exec(`DELETE FROM messages WHERE session_id = ?`, sessionID)
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
