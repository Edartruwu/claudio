package storage

import (
	"database/sql"
	"fmt"
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
	TeamTemplate        string // e.g. "backend-team", optional template name
	BranchFromMessageID *int64 // non-nil for user-initiated branch sessions
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
	AgentName string
	Output    string
}

// sessionColumns is the standard column list for session queries.
const sessionColumns = `id, title, project_dir, model, created_at, updated_at, summary, parent_session_id, agent_type, team_template, branch_from_message_id`

// scanSession scans a row into a Session, handling the nullable branch_from_message_id.
func scanSession(scanner interface{ Scan(...interface{}) error }) (*Session, error) {
	var s Session
	var branchFromMsgID sql.NullInt64
	err := scanner.Scan(&s.ID, &s.Title, &s.ProjectDir, &s.Model, &s.CreatedAt, &s.UpdatedAt,
		&s.Summary, &s.ParentSessionID, &s.AgentType, &s.TeamTemplate, &branchFromMsgID)
	if err != nil {
		return nil, err
	}
	if branchFromMsgID.Valid {
		v := branchFromMsgID.Int64
		s.BranchFromMessageID = &v
	}
	return &s, nil
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
		`SELECT `+sessionColumns+` FROM sessions WHERE parent_session_id = ? ORDER BY created_at ASC`,
		parentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, *s)
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

// UpdateSessionAgentType updates the agent_type of a session.
func (db *DB) UpdateSessionAgentType(id, agentType string) error {
	_, err := db.conn.Exec(
		`UPDATE sessions SET agent_type = ?, updated_at = ? WHERE id = ?`,
		agentType, time.Now(), id,
	)
	return err
}

// UpdateSessionTeamTemplate updates the team_template of a session.
func (db *DB) UpdateSessionTeamTemplate(id, teamTemplate string) error {
	_, err := db.conn.Exec(
		`UPDATE sessions SET team_template = ?, updated_at = ? WHERE id = ?`,
		teamTemplate, time.Now(), id,
	)
	return err
}

// GetSessionByTitle returns the most recent session matching title and projectDir,
// or nil if no such session exists.
func (db *DB) GetSessionByTitle(title, projectDir string) (*Session, error) {
	row := db.conn.QueryRow(
		`SELECT `+sessionColumns+` FROM sessions WHERE title = ? AND project_dir = ? ORDER BY created_at DESC LIMIT 1`,
		title, projectDir,
	)
	s, err := scanSession(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return s, err
}

// GetSession retrieves a session by ID.
func (db *DB) GetSession(id string) (*Session, error) {
	row := db.conn.QueryRow(
		`SELECT `+sessionColumns+` FROM sessions WHERE id = ?`, id,
	)
	s, err := scanSession(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return s, err
}

// ListSessions returns recent top-level sessions (no sub-agent sessions), newest first.
func (db *DB) ListSessions(limit int) ([]Session, error) {
	rows, err := db.conn.Query(
		`SELECT `+sessionColumns+` FROM sessions WHERE parent_session_id = '' ORDER BY updated_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, *s)
	}
	return sessions, rows.Err()
}

// ListSessionsByProject returns recent top-level sessions for a specific project, newest first.
func (db *DB) ListSessionsByProject(projectDir string, limit int) ([]Session, error) {
	rows, err := db.conn.Query(
		`SELECT `+sessionColumns+` FROM sessions WHERE parent_session_id = '' AND project_dir = ? ORDER BY updated_at DESC LIMIT ?`,
		projectDir, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, *s)
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
		`SELECT `+sessionColumns+` FROM sessions WHERE parent_session_id = '' AND (title LIKE ? OR project_dir LIKE ?) ORDER BY updated_at DESC LIMIT ?`,
		pattern, pattern, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, *s)
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
		`SELECT id, session_id, role, content, type, tool_use_id, tool_name, created_at,
		        COALESCE(agent_name,''), COALESCE(output,'')
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
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &m.Type, &m.ToolUseID, &m.ToolName, &m.CreatedAt, &m.AgentName, &m.Output); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

// ---------------------------------------------------------------------------
// Session branching
// ---------------------------------------------------------------------------

// CreateBranchSession creates a user branch from parentID, forking at branchFromMsgID.
// Sets both parent_session_id and branch_from_message_id.
func (db *DB) CreateBranchSession(parentID string, branchFromMsgID int64, projectDir, model string) (*Session, error) {
	s := &Session{
		ID:                  uuid.New().String(),
		Title:               "[branch]",
		ProjectDir:          projectDir,
		Model:               model,
		ParentSessionID:     parentID,
		BranchFromMessageID: &branchFromMsgID,
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
	}

	_, err := db.conn.Exec(
		`INSERT INTO sessions (id, title, project_dir, model, parent_session_id, branch_from_message_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.Title, s.ProjectDir, s.Model, s.ParentSessionID, branchFromMsgID, s.CreatedAt, s.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create branch session: %w", err)
	}
	return s, nil
}

// GetBranchMessages returns the full message chain for a branch session:
// parent messages with id <= branchFromMsgID (recursively through grandparents),
// then this session's own messages. For non-branch sessions, returns own messages only.
func (db *DB) GetBranchMessages(sessionID string) ([]MessageRecord, error) {
	type segment struct {
		sessionID string
		maxMsgID  *int64 // nil = no upper bound
	}

	// Walk parent chain: leaf → root. Each segment = one session's messages.
	// currentLimit carries the branch_from_message_id constraint downward.
	var segments []segment
	currentID := sessionID
	var currentLimit *int64

	for {
		sess, err := db.GetSession(currentID)
		if err != nil {
			return nil, fmt.Errorf("get session %s: %w", currentID, err)
		}
		if sess == nil {
			return nil, fmt.Errorf("session %s not found", currentID)
		}

		segments = append(segments, segment{sessionID: sess.ID, maxMsgID: currentLimit})

		if sess.BranchFromMessageID == nil || sess.ParentSessionID == "" {
			break
		}

		currentLimit = sess.BranchFromMessageID
		currentID = sess.ParentSessionID
	}

	// Query root-first, append in order.
	var allMessages []MessageRecord
	for i := len(segments) - 1; i >= 0; i-- {
		seg := segments[i]
		var rows *sql.Rows
		var err error
		if seg.maxMsgID != nil {
			rows, err = db.conn.Query(
				`SELECT id, session_id, role, content, type, tool_use_id, tool_name, created_at,
				        COALESCE(agent_name,''), COALESCE(output,'')
				 FROM messages WHERE session_id = ? AND id <= ? ORDER BY id ASC`,
				seg.sessionID, *seg.maxMsgID,
			)
		} else {
			rows, err = db.conn.Query(
				`SELECT id, session_id, role, content, type, tool_use_id, tool_name, created_at,
				        COALESCE(agent_name,''), COALESCE(output,'')
				 FROM messages WHERE session_id = ? ORDER BY id ASC`,
				seg.sessionID,
			)
		}
		if err != nil {
			return nil, fmt.Errorf("query messages for session %s: %w", seg.sessionID, err)
		}
		for rows.Next() {
			var m MessageRecord
			if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &m.Type, &m.ToolUseID, &m.ToolName, &m.CreatedAt, &m.AgentName, &m.Output); err != nil {
				rows.Close()
				return nil, err
			}
			allMessages = append(allMessages, m)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}

	return allMessages, nil
}

// GetSessionBranches returns direct user branch children of parentID.
func (db *DB) GetSessionBranches(parentID string) ([]*Session, error) {
	rows, err := db.conn.Query(
		`SELECT `+sessionColumns+` FROM sessions WHERE parent_session_id = ? AND branch_from_message_id IS NOT NULL ORDER BY created_at ASC`,
		parentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// SessionNode is a recursive tree node for GetSessionTree.
type SessionNode struct {
	Session  *Session
	Children []*SessionNode
}

// GetSessionTree returns a tree rooted at rootID, including all user branch descendants.
func (db *DB) GetSessionTree(rootID string) (*SessionNode, error) {
	root, err := db.GetSession(rootID)
	if err != nil {
		return nil, err
	}
	if root == nil {
		return nil, fmt.Errorf("session %s not found", rootID)
	}

	node := &SessionNode{Session: root}
	if err := db.buildTree(node); err != nil {
		return nil, err
	}
	return node, nil
}

func (db *DB) buildTree(node *SessionNode) error {
	branches, err := db.GetSessionBranches(node.Session.ID)
	if err != nil {
		return err
	}
	for _, branch := range branches {
		child := &SessionNode{Session: branch}
		if err := db.buildTree(child); err != nil {
			return err
		}
		node.Children = append(node.Children, child)
	}
	return nil
}

// GetRootSession walks the parent chain to find the root session (parent_session_id = '').
func (db *DB) GetRootSession(sessionID string) (*Session, error) {
	currentID := sessionID
	for i := 0; i < 100; i++ { // safety limit
		sess, err := db.GetSession(currentID)
		if err != nil {
			return nil, err
		}
		if sess == nil {
			return nil, fmt.Errorf("session %s not found", currentID)
		}
		if sess.ParentSessionID == "" {
			return sess, nil
		}
		currentID = sess.ParentSessionID
	}
	return nil, fmt.Errorf("parent chain too deep (>100) for session %s", sessionID)
}
