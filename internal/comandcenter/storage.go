package comandcenter

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Storage is the ComandCenter SQLite persistence layer.
type Storage struct {
	db *sql.DB
}

// Open creates or opens the ComandCenter SQLite database at path.
func Open(path string) (*Storage, error) {
	conn, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("comandcenter: open db: %w", err)
	}

	if _, err := conn.Exec("PRAGMA journal_mode=WAL"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("comandcenter: WAL pragma: %w", err)
	}

	if _, err := conn.Exec("PRAGMA foreign_keys=ON"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("comandcenter: foreign_keys pragma: %w", err)
	}

	s := &Storage{db: conn}
	if err := s.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("comandcenter: migration: %w", err)
	}

	return s, nil
}

// Close closes the database connection.
func (s *Storage) Close() error {
	return s.db.Close()
}

func (s *Storage) migrate() error {
	if _, err := s.db.Exec(
		`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL DEFAULT 0)`,
	); err != nil {
		return fmt.Errorf("bootstrap version table: %w", err)
	}

	var version int
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&version); err != nil {
		return fmt.Errorf("read version: %w", err)
	}

	if version == 0 {
		version = s.detectExistingSchemaVersion()
		if version > 0 {
			if _, err := s.db.Exec(`INSERT INTO schema_version (version) VALUES (?)`, version); err != nil {
				return fmt.Errorf("bootstrap version: %w", err)
			}
		}
	}

	// Append-only. Never edit existing entries.
	migrations := []string{
		// 1
		`CREATE TABLE IF NOT EXISTS cc_sessions (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			path TEXT NOT NULL,
			model TEXT,
			master INTEGER DEFAULT 0,
			status TEXT DEFAULT 'inactive',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_active_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// 2
		`CREATE TABLE IF NOT EXISTS cc_messages (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL REFERENCES cc_sessions(id),
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			agent_name TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// 3
		`CREATE TABLE IF NOT EXISTS cc_tasks (
			id TEXT PRIMARY KEY,
			session_id TEXT REFERENCES cc_sessions(id),
			title TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			assigned_to TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// 4
		`CREATE TABLE IF NOT EXISTS cc_agents (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL REFERENCES cc_sessions(id),
			name TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'idle',
			current_task_id TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// 5
		`ALTER TABLE cc_sessions ADD COLUMN last_read_at DATETIME`,
		// 6
		`CREATE TABLE IF NOT EXISTS cc_attachments (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL REFERENCES cc_sessions(id),
			message_id TEXT REFERENCES cc_messages(id),
			filename TEXT NOT NULL,
			original_name TEXT NOT NULL,
			mime_type TEXT NOT NULL DEFAULT '',
			size INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// 7
		`ALTER TABLE cc_messages ADD COLUMN reply_to_session TEXT`,
		// 8
		`ALTER TABLE cc_messages ADD COLUMN quoted_content TEXT`,
		// 9
		`ALTER TABLE cc_tasks ADD COLUMN description TEXT`,
	}

	for i, m := range migrations {
		if i < version {
			continue
		}
		if _, err := s.db.Exec(m); err != nil {
			if !isCCAlreadyExistsErr(err) {
				return fmt.Errorf("migration %d: %w\nSQL: %s", i+1, err, m)
			}
		}
		if _, err := s.db.Exec(`INSERT INTO schema_version (version) VALUES (?)`, i+1); err != nil {
			return fmt.Errorf("update version to %d: %w", i+1, err)
		}
	}

	return nil
}

func (s *Storage) detectExistingSchemaVersion() int {
	hasTable := func(table string) bool {
		var n int
		s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n)
		return n > 0
	}
	switch {
	case hasTable("cc_attachments"):
		return 6
	case hasTable("cc_agents"):
		return 4
	case hasTable("cc_tasks"):
		return 3
	case hasTable("cc_messages"):
		return 2
	case hasTable("cc_sessions"):
		return 1
	default:
		return 0
	}
}

func isCCAlreadyExistsErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "already exists") ||
		strings.Contains(msg, "duplicate column name") ||
		strings.Contains(msg, "no such table")
}

// UpsertSession inserts or replaces a session record.
func (s *Storage) UpsertSession(sess Session) error {
	master := 0
	if sess.Master {
		master = 1
	}
	_, err := s.db.Exec(`
		INSERT INTO cc_sessions (id, name, path, model, master, status, created_at, last_active_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name,
			path=excluded.path,
			model=excluded.model,
			master=excluded.master,
			status=excluded.status,
			last_active_at=excluded.last_active_at
	`, sess.ID, sess.Name, sess.Path, sess.Model, master, sess.Status,
		sess.CreatedAt, sess.LastActiveAt)
	if err != nil {
		return fmt.Errorf("upsert session: %w", err)
	}
	return nil
}

// SetSessionStatus updates the status of a session.
func (s *Storage) SetSessionStatus(id, status string) error {
	_, err := s.db.Exec(
		`UPDATE cc_sessions SET status=?, last_active_at=? WHERE id=?`,
		status, time.Now(), id,
	)
	if err != nil {
		return fmt.Errorf("set session status: %w", err)
	}
	return nil
}

// ArchiveSession sets a session's status to 'archived'.
// Archived sessions are excluded from ListSessions.
func (s *Storage) ArchiveSession(id string) error {
	_, err := s.db.Exec(
		`UPDATE cc_sessions SET status='archived', last_active_at=? WHERE id=?`,
		time.Now(), id,
	)
	if err != nil {
		return fmt.Errorf("archive session: %w", err)
	}
	return nil
}

// DeleteSession permanently removes a session and all its related records.
// Deletes messages, tasks, and agents before removing the session row.
func (s *Storage) DeleteSession(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("delete session begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	for _, q := range []string{
		`DELETE FROM cc_agents WHERE session_id=?`,
		`DELETE FROM cc_tasks WHERE session_id=?`,
		`DELETE FROM cc_attachments WHERE session_id=?`,
		`DELETE FROM cc_messages WHERE session_id=?`,
		`DELETE FROM cc_sessions WHERE id=?`,
	} {
		if _, err := tx.Exec(q, id); err != nil {
			return fmt.Errorf("delete session cleanup: %w", err)
		}
	}

	return tx.Commit()
}

// ListSessions returns non-archived sessions ordered by last_active_at desc.
// filter: "" = all, "active" = status='active', "inactive" = status not 'active' and not 'archived'.
func (s *Storage) ListSessions(filter string) ([]Session, error) {
	var query string
	switch filter {
	case "active":
		query = `SELECT id, name, path, COALESCE(model,''), master, status, created_at, last_active_at
			FROM cc_sessions WHERE status = 'active' ORDER BY last_active_at DESC`
	case "inactive":
		query = `SELECT id, name, path, COALESCE(model,''), master, status, created_at, last_active_at
			FROM cc_sessions WHERE status != 'active' AND status != 'archived' ORDER BY last_active_at DESC`
	default:
		query = `SELECT id, name, path, COALESCE(model,''), master, status, created_at, last_active_at
			FROM cc_sessions WHERE status != 'archived' ORDER BY last_active_at DESC`
	}
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var sess Session
		var master int
		if err := rows.Scan(&sess.ID, &sess.Name, &sess.Path, &sess.Model,
			&master, &sess.Status, &sess.CreatedAt, &sess.LastActiveAt); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		sess.Master = master == 1
		sessions = append(sessions, sess)
	}
	return sessions, rows.Err()
}

// GetSession returns a single session by ID. Returns error if not found.
func (s *Storage) GetSession(id string) (Session, error) {
	var sess Session
	var master int
	err := s.db.QueryRow(`
		SELECT id, name, path, COALESCE(model,''), master, status, created_at, last_active_at
		FROM cc_sessions WHERE id=?
	`, id).Scan(&sess.ID, &sess.Name, &sess.Path, &sess.Model,
		&master, &sess.Status, &sess.CreatedAt, &sess.LastActiveAt)
	if err == sql.ErrNoRows {
		return Session{}, fmt.Errorf("session %s not found", id)
	}
	if err != nil {
		return Session{}, fmt.Errorf("get session: %w", err)
	}
	sess.Master = master == 1
	return sess, nil
}

// GetSessionByName returns the most recent session with the given name, if any.
func (s *Storage) GetSessionByName(name string) (Session, bool, error) {
	var sess Session
	var master int
	err := s.db.QueryRow(`
		SELECT id, name, path, COALESCE(model,''), master, status, created_at, last_active_at
		FROM cc_sessions WHERE name=? ORDER BY created_at DESC LIMIT 1
	`, name).Scan(&sess.ID, &sess.Name, &sess.Path, &sess.Model,
		&master, &sess.Status, &sess.CreatedAt, &sess.LastActiveAt)
	if err == sql.ErrNoRows {
		return Session{}, false, nil
	}
	if err != nil {
		return Session{}, false, err
	}
	sess.Master = master == 1
	return sess, true, nil
}

// InsertMessage stores a message for a session.
func (s *Storage) InsertMessage(msg Message) error {
	_, err := s.db.Exec(`
		INSERT INTO cc_messages (id, session_id, role, content, agent_name, created_at, reply_to_session, quoted_content)
		VALUES (?, ?, ?, ?, ?, ?, NULLIF(?,?), NULLIF(?,?))
	`, msg.ID, msg.SessionID, msg.Role, msg.Content, msg.AgentName, msg.CreatedAt,
		msg.ReplyToSession, "", msg.QuotedContent, "")
	if err != nil {
		return fmt.Errorf("insert message: %w", err)
	}
	return nil
}

// ListMessages returns messages for a session, newest first, up to limit.
func (s *Storage) ListMessages(sessionID string, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`
		SELECT id, session_id, role, content, COALESCE(agent_name,''), created_at,
		       COALESCE(reply_to_session,''), COALESCE(quoted_content,'')
		FROM cc_messages WHERE session_id=?
		ORDER BY created_at DESC LIMIT ?
	`, sessionID, limit)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()

	var msgs []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content,
			&m.AgentName, &m.CreatedAt, &m.ReplyToSession, &m.QuotedContent); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// UpsertTask inserts or updates a task.
func (s *Storage) UpsertTask(task Task) error {
	_, err := s.db.Exec(`
		INSERT INTO cc_tasks (id, session_id, title, description, status, assigned_to, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			status=excluded.status,
			description=excluded.description,
			assigned_to=excluded.assigned_to,
			updated_at=excluded.updated_at
	`, task.ID, task.SessionID, task.Title, task.Description, task.Status, task.AssignedTo,
		task.CreatedAt, task.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert task: %w", err)
	}
	return nil
}

// GetTask returns a single task by ID, or an error if not found.
func (s *Storage) GetTask(id string) (Task, error) {
	var t Task
	err := s.db.QueryRow(`
		SELECT id, session_id, title, COALESCE(description,''), status, COALESCE(assigned_to,''), created_at, updated_at
		FROM cc_tasks WHERE id=?
	`, id).Scan(&t.ID, &t.SessionID, &t.Title, &t.Description, &t.Status,
		&t.AssignedTo, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return Task{}, fmt.Errorf("get task %q: %w", id, err)
	}
	return t, nil
}

// UpsertAgent inserts or updates an agent record.
func (s *Storage) UpsertAgent(agent Agent) error {
	_, err := s.db.Exec(`
		INSERT INTO cc_agents (id, session_id, name, status, current_task_id, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			status=excluded.status,
			current_task_id=excluded.current_task_id,
			updated_at=excluded.updated_at
	`, agent.ID, agent.SessionID, agent.Name, agent.Status, agent.CurrentTaskID,
		agent.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert agent: %w", err)
	}
	return nil
}

// UnreadCount returns the count of unread messages for a session.
// Messages are unread if created_at > last_read_at (or all if last_read_at is NULL).
func (s *Storage) UnreadCount(sessionID string) (int, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM cc_messages
		WHERE session_id = ? AND created_at > COALESCE(
			(SELECT last_read_at FROM cc_sessions WHERE id = ?),
			'1970-01-01'
		)
	`, sessionID, sessionID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("unread count: %w", err)
	}
	return count, nil
}

// MarkRead updates the last_read_at timestamp for a session to now.
func (s *Storage) MarkRead(sessionID string) error {
	_, err := s.db.Exec(
		`UPDATE cc_sessions SET last_read_at = CURRENT_TIMESTAMP WHERE id = ?`,
		sessionID)
	if err != nil {
		return fmt.Errorf("mark read: %w", err)
	}
	return nil
}

// ListTasks returns all tasks for a session ordered by created_at DESC.
func (s *Storage) ListTasks(sessionID string) ([]Task, error) {
	rows, err := s.db.Query(`
		SELECT id, session_id, title, COALESCE(description,''), status, COALESCE(assigned_to,''), created_at, updated_at
		FROM cc_tasks WHERE session_id=? ORDER BY created_at DESC
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.SessionID, &t.Title, &t.Description, &t.Status,
			&t.AssignedTo, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// ListAgents returns all agents for a session.
func (s *Storage) ListAgents(sessionID string) ([]Agent, error) {
	rows, err := s.db.Query(`
		SELECT id, session_id, name, status, COALESCE(current_task_id,''), updated_at
		FROM cc_agents WHERE session_id=?
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close()

	var agents []Agent
	for rows.Next() {
		var a Agent
		if err := rows.Scan(&a.ID, &a.SessionID, &a.Name, &a.Status,
			&a.CurrentTaskID, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

// InsertAttachment stores a new attachment record.
func (s *Storage) InsertAttachment(att Attachment) error {
	_, err := s.db.Exec(`
		INSERT INTO cc_attachments (id, session_id, message_id, filename, original_name, mime_type, size, created_at)
		VALUES (?, ?, NULLIF(?,?), ?, ?, ?, ?, ?)
	`, att.ID, att.SessionID, att.MessageID, "", att.Filename, att.OriginalName,
		att.MimeType, att.Size, att.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert attachment: %w", err)
	}
	return nil
}

// ListAttachments returns all attachments for a session ordered by created_at DESC.
func (s *Storage) ListAttachments(sessionID string) ([]Attachment, error) {
	rows, err := s.db.Query(`
		SELECT id, session_id, COALESCE(message_id,''), filename, original_name, mime_type, size, created_at
		FROM cc_attachments WHERE session_id=? ORDER BY created_at DESC
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list attachments: %w", err)
	}
	defer rows.Close()

	var atts []Attachment
	for rows.Next() {
		var a Attachment
		if err := rows.Scan(&a.ID, &a.SessionID, &a.MessageID, &a.Filename,
			&a.OriginalName, &a.MimeType, &a.Size, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan attachment: %w", err)
		}
		atts = append(atts, a)
	}
	return atts, rows.Err()
}

// ListMessageAttachments returns all attachments linked to a specific message.
func (s *Storage) ListMessageAttachments(messageID string) ([]Attachment, error) {
	rows, err := s.db.Query(`
		SELECT id, session_id, COALESCE(message_id,''), filename, original_name, mime_type, size, created_at
		FROM cc_attachments WHERE message_id=? ORDER BY created_at ASC
	`, messageID)
	if err != nil {
		return nil, fmt.Errorf("list message attachments: %w", err)
	}
	defer rows.Close()

	var atts []Attachment
	for rows.Next() {
		var a Attachment
		if err := rows.Scan(&a.ID, &a.SessionID, &a.MessageID, &a.Filename,
			&a.OriginalName, &a.MimeType, &a.Size, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan attachment: %w", err)
		}
		atts = append(atts, a)
	}
	return atts, rows.Err()
}
