package comandcenter

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	_ "modernc.org/sqlite"
)

// Storage is the ComandCenter SQLite persistence layer.
type Storage struct {
	db *sql.DB
}

// ExecRaw executes arbitrary SQL. Used by tests to seed native claudio tables.
func (s *Storage) ExecRaw(query string, args ...any) error {
	_, err := s.db.Exec(query, args...)
	return err
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
	// Use a CC-specific version table so we don't collide with claudio's
	// schema_version table when both run against the same DB file.
	if _, err := s.db.Exec(
		`CREATE TABLE IF NOT EXISTS cc_schema_version (version INTEGER NOT NULL DEFAULT 0)`,
	); err != nil {
		return fmt.Errorf("bootstrap version table: %w", err)
	}

	var version int
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM cc_schema_version`).Scan(&version); err != nil {
		return fmt.Errorf("read version: %w", err)
	}

	if version == 0 {
		version = s.detectExistingSchemaVersion()
		if version > 0 {
			if _, err := s.db.Exec(`INSERT INTO cc_schema_version (version) VALUES (?)`, version); err != nil {
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
		// 10
		`CREATE TABLE IF NOT EXISTS cc_push_subscriptions (
			id TEXT PRIMARY KEY,
			endpoint TEXT NOT NULL UNIQUE,
			p256dh TEXT NOT NULL,
			auth TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// 11
		`CREATE TABLE IF NOT EXISTS cc_vapid_keys (
			id INTEGER PRIMARY KEY CHECK (id=1),
			public_key TEXT NOT NULL,
			private_key TEXT NOT NULL
		)`,
		// 12 — idempotent retry: ensure reply_to_session exists (migration 7 may have been
		// silently skipped on DBs where schema_version was bootstrapped incorrectly).
		`ALTER TABLE cc_messages ADD COLUMN reply_to_session TEXT`,
		// 13 — idempotent retry: same for quoted_content (migration 8).
		`ALTER TABLE cc_messages ADD COLUMN quoted_content TEXT`,
		// 14 — agent type override per session.
		`ALTER TABLE cc_sessions ADD COLUMN agent_type TEXT`,
		// 15 — team template name per session.
		`ALTER TABLE cc_sessions ADD COLUMN team_template TEXT`,
		// 16 — tool_use_id links tool_use message to its result.
		`ALTER TABLE cc_messages ADD COLUMN tool_use_id TEXT`,
		// 17 — output stores tool result content.
		`ALTER TABLE cc_messages ADD COLUMN output TEXT`,
		// 18 — context token count per session (latest input tokens sent to Claude).
		`ALTER TABLE cc_sessions ADD COLUMN context_tokens INTEGER DEFAULT 0`,
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
		if _, err := s.db.Exec(`INSERT INTO cc_schema_version (version) VALUES (?)`, i+1); err != nil {
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
		INSERT INTO cc_sessions (id, name, path, model, master, status, created_at, last_active_at, agent_type, team_template)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name,
			path=excluded.path,
			model=excluded.model,
			master=excluded.master,
			status=excluded.status,
			last_active_at=excluded.last_active_at,
			agent_type=excluded.agent_type,
			team_template=excluded.team_template
	`, sess.ID, sess.Name, sess.Path, sess.Model, master, sess.Status,
		sess.CreatedAt, sess.LastActiveAt, sess.AgentType, sess.TeamTemplate)
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

// UpdateSessionConfig persists the agent type and team template overrides for a session.
// Pass empty strings to clear the overrides.
func (s *Storage) UpdateSessionConfig(id, agentType, teamTemplate string) error {
	_, err := s.db.Exec(
		`UPDATE cc_sessions SET agent_type=?, team_template=? WHERE id=?`,
		agentType, teamTemplate, id,
	)
	if err != nil {
		return fmt.Errorf("update session config: %w", err)
	}
	return nil
}

// UpdateContextTokens stores the latest context window token count for a session.
func (s *Storage) UpdateContextTokens(sessionID string, tokens int) error {
	_, err := s.db.Exec(
		`UPDATE cc_sessions SET context_tokens=?, last_active_at=? WHERE id=?`,
		tokens, time.Now(), sessionID,
	)
	if err != nil {
		return fmt.Errorf("update context tokens: %w", err)
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

// Project holds a unique project derived from session paths.
type Project struct {
	Name  string // filepath.Base of the path
	Path  string // full path
	Count int    // number of sessions in this project
}

// ListProjects returns unique projects derived from session paths, ordered by name.
func (s *Storage) ListProjects() ([]Project, error) {
	rows, err := s.db.Query(`
		SELECT path, COUNT(*) as cnt
		FROM cc_sessions
		WHERE status != 'archived' AND path != ''
		GROUP BY path
		ORDER BY path ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()
	var projects []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.Path, &p.Count); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		p.Name = filepath.Base(p.Path)
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// ListSessions returns non-archived sessions ordered by last_active_at desc.
// filter: "" = all, "active" = status='active', "inactive" = status not 'active' and not 'archived',
// "project:<path>" = sessions where path = <path>.
func (s *Storage) ListSessions(filter string) ([]Session, error) {
	const sel = `SELECT id, name, path, COALESCE(model,''), master, status, created_at, last_active_at, COALESCE(agent_type,''), COALESCE(team_template,''), COALESCE(context_tokens,0)`
	var (
		query string
		args  []any
	)
	switch {
	case filter == "active":
		query = sel + ` FROM cc_sessions WHERE status = 'active' ORDER BY last_active_at DESC`
	case filter == "inactive":
		query = sel + ` FROM cc_sessions WHERE status != 'active' AND status != 'archived' ORDER BY last_active_at DESC`
	case strings.HasPrefix(filter, "project:"):
		projectPath := strings.TrimPrefix(filter, "project:")
		query = sel + ` FROM cc_sessions WHERE status != 'archived' AND path = ? ORDER BY last_active_at DESC`
		args = append(args, projectPath)
	default:
		query = sel + ` FROM cc_sessions WHERE status != 'archived' ORDER BY last_active_at DESC`
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var sess Session
		var master int
		if err := rows.Scan(&sess.ID, &sess.Name, &sess.Path, &sess.Model,
			&master, &sess.Status, &sess.CreatedAt, &sess.LastActiveAt,
			&sess.AgentType, &sess.TeamTemplate, &sess.ContextTokens); err != nil {
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
		SELECT id, name, path, COALESCE(model,''), master, status, created_at, last_active_at, COALESCE(agent_type,''), COALESCE(team_template,''), COALESCE(context_tokens,0)
		FROM cc_sessions WHERE id=?
	`, id).Scan(&sess.ID, &sess.Name, &sess.Path, &sess.Model,
		&master, &sess.Status, &sess.CreatedAt, &sess.LastActiveAt,
		&sess.AgentType, &sess.TeamTemplate, &sess.ContextTokens)
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
		SELECT id, name, path, COALESCE(model,''), master, status, created_at, last_active_at, COALESCE(agent_type,''), COALESCE(team_template,''), COALESCE(context_tokens,0)
		FROM cc_sessions WHERE name=? ORDER BY created_at DESC LIMIT 1
	`, name).Scan(&sess.ID, &sess.Name, &sess.Path, &sess.Model,
		&master, &sess.Status, &sess.CreatedAt, &sess.LastActiveAt,
		&sess.AgentType, &sess.TeamTemplate, &sess.ContextTokens)
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
		INSERT INTO cc_messages (id, session_id, role, content, agent_name, created_at,
		                         reply_to_session, quoted_content, tool_use_id)
		VALUES (?, ?, ?, ?, ?, ?, NULLIF(?,?), NULLIF(?,?), NULLIF(?,?))
	`, msg.ID, msg.SessionID, msg.Role, msg.Content, msg.AgentName, msg.CreatedAt,
		msg.ReplyToSession, "", msg.QuotedContent, "", msg.ToolUseID, "")
	if err != nil {
		return fmt.Errorf("insert message: %w", err)
	}
	return nil
}

// UpdateMessageOutput sets the output field on a tool_use message by ToolUseID.
func (s *Storage) UpdateMessageOutput(sessionID, toolUseID, output string) error {
	_, err := s.db.Exec(`
		UPDATE cc_messages SET output=? WHERE session_id=? AND tool_use_id=?
	`, output, sessionID, toolUseID)
	if err != nil {
		return fmt.Errorf("update message output: %w", err)
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
		       COALESCE(reply_to_session,''), COALESCE(quoted_content,''),
		       COALESCE(tool_use_id,''), COALESCE(output,'')
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
			&m.AgentName, &m.CreatedAt, &m.ReplyToSession, &m.QuotedContent,
			&m.ToolUseID, &m.Output); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// GetNativeMessages reads from the native claudio messages table (written by the TUI)
// and maps rows to Message for display. Returns newest first (DESC by id), up to limit.
func (s *Storage) GetNativeMessages(sessionID string, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`
		SELECT id, session_id, role, content, COALESCE(type,'text'), COALESCE(tool_use_id,''), COALESCE(tool_name,''), created_at
		FROM messages WHERE session_id=?
		ORDER BY id DESC LIMIT ?
	`, sessionID, limit)
	if err != nil {
		return nil, fmt.Errorf("get native messages: %w", err)
	}
	defer rows.Close()

	var msgs []Message
	for rows.Next() {
		var (
			id, sid, role, content, typ, toolUseID, toolName string
			createdAt                                         time.Time
		)
		if err := rows.Scan(&id, &sid, &role, &content, &typ, &toolUseID, &toolName, &createdAt); err != nil {
			return nil, fmt.Errorf("scan native message: %w", err)
		}
		m := Message{
			ID:        id,
			SessionID: sid,
			CreatedAt: createdAt,
		}
		switch typ {
		case "tool_use":
			m.Role = "tool_use"
			m.Content = toolName + ": " + content
			m.ToolUseID = toolUseID
		case "tool_result":
			m.Role = "tool_result"
			m.Content = content
			m.ToolUseID = toolUseID
		default: // "text"
			m.Role = role
			m.Content = content
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// DeleteMessages removes all messages for a session (used by /clear command).
func (s *Storage) DeleteMessageByID(id string) error {
	_, err := s.db.Exec(`DELETE FROM cc_messages WHERE id = ?`, id)
	return err
}

func (s *Storage) DeleteMessages(sessionID string) error {
	_, err := s.db.Exec(`DELETE FROM cc_messages WHERE session_id = ?`, sessionID)
	if err != nil {
		return fmt.Errorf("delete messages: %w", err)
	}
	return nil
}

// DeleteNativeMessages deletes all messages for a session from the native claudio messages table.
func (s *Storage) DeleteNativeMessages(sessionID string) error {
	_, err := s.db.Exec(`DELETE FROM messages WHERE session_id = ?`, sessionID)
	return err
}

// InsertNativeMessage inserts a text message into the native claudio messages table.
func (s *Storage) InsertNativeMessage(sessionID, role, content string, ts time.Time) error {
	id := fmt.Sprintf("%d", ts.UnixNano())
	_, err := s.db.Exec(
		`INSERT INTO messages (id, session_id, role, content, type, tool_use_id, tool_name, created_at) VALUES (?, ?, ?, ?, 'text', '', '', ?)`,
		id, sessionID, role, content, ts,
	)
	return err
}

// GetTask returns a single task by ID from team_tasks (claudio's native table).
func (s *Storage) GetTask(id string) (Task, error) {
	var t Task
	err := s.db.QueryRow(`
		SELECT id, session_id, subject, COALESCE(description,''), status, COALESCE(assigned_to,''), created_at, updated_at
		FROM team_tasks WHERE id=?
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

// ListTasks returns all tasks for a session from team_tasks (claudio's native table).
func (s *Storage) ListTasks(sessionID string) ([]Task, error) {
	rows, err := s.db.Query(`
		SELECT id, session_id, subject, COALESCE(description,''), status, COALESCE(assigned_to,''), created_at, updated_at
		FROM team_tasks WHERE session_id=? AND status != 'deleted' ORDER BY created_at DESC
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

// SavePushSubscription inserts or replaces a push subscription.
func (s *Storage) SavePushSubscription(sub PushSubscription) error {
	_, err := s.db.Exec(`
		INSERT INTO cc_push_subscriptions (id, endpoint, p256dh, auth, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(endpoint) DO UPDATE SET
			p256dh=excluded.p256dh,
			auth=excluded.auth
	`, sub.ID, sub.Endpoint, sub.P256dh, sub.Auth, sub.CreatedAt)
	if err != nil {
		return fmt.Errorf("save push subscription: %w", err)
	}
	return nil
}

// ListPushSubscriptions returns all stored push subscriptions.
func (s *Storage) ListPushSubscriptions() ([]PushSubscription, error) {
	rows, err := s.db.Query(`
		SELECT id, endpoint, p256dh, auth, created_at FROM cc_push_subscriptions
	`)
	if err != nil {
		return nil, fmt.Errorf("list push subscriptions: %w", err)
	}
	defer rows.Close()

	var subs []PushSubscription
	for rows.Next() {
		var sub PushSubscription
		if err := rows.Scan(&sub.ID, &sub.Endpoint, &sub.P256dh, &sub.Auth, &sub.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan push subscription: %w", err)
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

// DeletePushSubscription removes a push subscription by endpoint.
func (s *Storage) DeletePushSubscription(endpoint string) error {
	_, err := s.db.Exec(`DELETE FROM cc_push_subscriptions WHERE endpoint=?`, endpoint)
	if err != nil {
		return fmt.Errorf("delete push subscription: %w", err)
	}
	return nil
}

// GetOrCreateVAPIDKeys returns stored VAPID keys, generating and storing them on first call.
func (s *Storage) GetOrCreateVAPIDKeys() (public, private string, err error) {
	err = s.db.QueryRow(`SELECT public_key, private_key FROM cc_vapid_keys WHERE id=1`).
		Scan(&public, &private)
	if err == nil {
		return public, private, nil
	}
	if err != sql.ErrNoRows {
		return "", "", fmt.Errorf("get vapid keys: %w", err)
	}

	// Generate new VAPID keys.
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return "", "", fmt.Errorf("generate vapid keys: %w", err)
	}

	_, err = s.db.Exec(`INSERT INTO cc_vapid_keys (id, public_key, private_key) VALUES (1, ?, ?)`, pub, priv)
	if err != nil {
		return "", "", fmt.Errorf("store vapid keys: %w", err)
	}
	return pub, priv, nil
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
