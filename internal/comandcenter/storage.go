package comandcenter

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	_ "modernc.org/sqlite"
)

// Storage is the ComandCenter SQLite persistence layer.
// It uses separate read/write DB pools to exploit SQLite WAL concurrency:
// writeDB is serialized (MaxOpenConns=1), readDB allows concurrent readers.
type Storage struct {
	writeDB *sql.DB
	readDB  *sql.DB
}

// ExecRaw executes arbitrary SQL. Used by tests to seed native claudio tables.
func (s *Storage) ExecRaw(query string, args ...any) error {
	_, err := s.writeDB.Exec(query, args...)
	return err
}

// openPool opens a single SQLite connection pool with WAL + busy_timeout + foreign_keys.
// For :memory: databases, shared cache is used so multiple pools see the same data.
func openPool(path string) (*sql.DB, error) {
	dsn := path + "?_journal_mode=WAL&_busy_timeout=5000"
	if path == ":memory:" {
		dsn = "file::memory:?mode=memory&cache=shared&_journal_mode=WAL&_busy_timeout=5000"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	for _, pragma := range []struct{ sql, label string }{
		{"PRAGMA journal_mode=WAL", "WAL"},
		{"PRAGMA busy_timeout=5000", "busy_timeout"},
		{"PRAGMA foreign_keys=ON", "foreign_keys"},
	} {
		if _, err := db.Exec(pragma.sql); err != nil {
			db.Close()
			return nil, fmt.Errorf("%s pragma: %w", pragma.label, err)
		}
	}
	return db, nil
}

// Open creates or opens the ComandCenter SQLite database at path.
// Two connection pools are opened on the same file:
//   - writeDB: serialized (MaxOpenConns=1) for INSERT/UPDATE/DELETE
//   - readDB: concurrent (unlimited conns) for SELECT queries
func Open(path string) (*Storage, error) {
	writeDB, err := openPool(path)
	if err != nil {
		return nil, fmt.Errorf("comandcenter: open write db: %w", err)
	}
	// Serialize all writes — production-standard SQLite concurrency pattern.
	writeDB.SetMaxOpenConns(1)

	readDB, err := openPool(path)
	if err != nil {
		writeDB.Close()
		return nil, fmt.Errorf("comandcenter: open read db: %w", err)
	}
	// Allow concurrent readers — WAL mode supports this.
	readDB.SetMaxOpenConns(0)
	readDB.SetMaxIdleConns(4)

	s := &Storage{writeDB: writeDB, readDB: readDB}
	if err := s.migrate(); err != nil {
		writeDB.Close()
		readDB.Close()
		return nil, fmt.Errorf("comandcenter: migration: %w", err)
	}

	return s, nil
}

// Close closes both database connection pools.
func (s *Storage) Close() error {
	err := s.writeDB.Close()
	if err2 := s.readDB.Close(); err == nil {
		err = err2
	}
	return err
}

func (s *Storage) migrate() error {
	// Use a CC-specific version table so we don't collide with claudio's
	// schema_version table when both run against the same DB file.
	if _, err := s.writeDB.Exec(
		`CREATE TABLE IF NOT EXISTS cc_schema_version (version INTEGER NOT NULL DEFAULT 0)`,
	); err != nil {
		return fmt.Errorf("bootstrap version table: %w", err)
	}

	var version int
	if err := s.writeDB.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM cc_schema_version`).Scan(&version); err != nil {
		return fmt.Errorf("read version: %w", err)
	}

	if version == 0 {
		version = s.detectExistingSchemaVersion()
		if version > 0 {
			if _, err := s.writeDB.Exec(`INSERT INTO cc_schema_version (version) VALUES (?)`, version); err != nil {
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
		// 19 — agent metrics: current tool, call count, elapsed seconds.
		`ALTER TABLE cc_agents ADD COLUMN current_tool TEXT NOT NULL DEFAULT ''`,
		// 20
		`ALTER TABLE cc_agents ADD COLUMN call_count INTEGER NOT NULL DEFAULT 0`,
		// 21
		`ALTER TABLE cc_agents ADD COLUMN elapsed_secs INTEGER NOT NULL DEFAULT 0`,
		// 22 — agent event history for reconnect replay
		`CREATE TABLE IF NOT EXISTS cc_agent_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			agent_name TEXT NOT NULL,
			status TEXT NOT NULL,
			payload TEXT NOT NULL DEFAULT '{}',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// 23
		`CREATE INDEX IF NOT EXISTS idx_cc_agent_events_session ON cc_agent_events(session_id, created_at)`,
	}

	for i, m := range migrations {
		if i < version {
			continue
		}
		if _, err := s.writeDB.Exec(m); err != nil {
			if !isCCAlreadyExistsErr(err) {
				return fmt.Errorf("migration %d: %w\nSQL: %s", i+1, err, m)
			}
		}
		if _, err := s.writeDB.Exec(`INSERT INTO cc_schema_version (version) VALUES (?)`, i+1); err != nil {
			return fmt.Errorf("update version to %d: %w", i+1, err)
		}
	}

	return nil
}

func (s *Storage) detectExistingSchemaVersion() int {
	hasTable := func(table string) bool {
		var n int
		s.writeDB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n)
		return n > 0
	}
	switch {
	case hasTable("cc_attachments"):
		return 6
	case hasTable("cc_agents"):
		return 4
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
	_, err := s.writeDB.Exec(`
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
	_, err := s.writeDB.Exec(
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
	_, err := s.writeDB.Exec(
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
	_, err := s.writeDB.Exec(
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
	_, err := s.writeDB.Exec(
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
	tx, err := s.writeDB.Begin()
	if err != nil {
		return fmt.Errorf("delete session begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	for _, q := range []string{
		`DELETE FROM cc_agents WHERE session_id=?`,
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
	rows, err := s.readDB.Query(`
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
	rows, err := s.readDB.Query(query, args...)
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
	err := s.readDB.QueryRow(`
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
	err := s.readDB.QueryRow(`
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
	_, err := s.writeDB.Exec(`
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
	_, err := s.writeDB.Exec(`
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
	rows, err := s.readDB.Query(`
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

// ListMessagesPaginated returns the most recent `limit` messages for a session,
// optionally before the message with ID `beforeID`. Returns (messages newest-first,
// hasMore bool, error). hasMore is true when older messages exist beyond the returned page.
func (s *Storage) ListMessagesPaginated(sessionID string, limit int, beforeID string) ([]Message, bool, error) {
	if limit <= 0 {
		limit = 50
	}
	// Fetch limit+1 to detect whether more messages exist.
	fetchLimit := limit + 1

	var rows *sql.Rows
	var err error
	if beforeID != "" {
		rows, err = s.readDB.Query(`
			SELECT id, session_id, role, content, COALESCE(agent_name,''), created_at,
			       COALESCE(reply_to_session,''), COALESCE(quoted_content,''),
			       COALESCE(tool_use_id,''), COALESCE(output,'')
			FROM cc_messages
			WHERE session_id=? AND created_at < (SELECT created_at FROM cc_messages WHERE id=? LIMIT 1)
			ORDER BY created_at DESC LIMIT ?
		`, sessionID, beforeID, fetchLimit)
	} else {
		rows, err = s.readDB.Query(`
			SELECT id, session_id, role, content, COALESCE(agent_name,''), created_at,
			       COALESCE(reply_to_session,''), COALESCE(quoted_content,''),
			       COALESCE(tool_use_id,''), COALESCE(output,'')
			FROM cc_messages WHERE session_id=?
			ORDER BY created_at DESC LIMIT ?
		`, sessionID, fetchLimit)
	}
	if err != nil {
		return nil, false, fmt.Errorf("list messages paginated: %w", err)
	}
	defer rows.Close()

	var msgs []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content,
			&m.AgentName, &m.CreatedAt, &m.ReplyToSession, &m.QuotedContent,
			&m.ToolUseID, &m.Output); err != nil {
			return nil, false, fmt.Errorf("scan message: %w", err)
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}

	hasMore := len(msgs) > limit
	if hasMore {
		msgs = msgs[:limit]
	}
	return msgs, hasMore, nil
}

// ListMessagesByAgent returns messages for a session filtered by agent_name, oldest first, up to limit.
func (s *Storage) ListMessagesByAgent(sessionID, agentName string, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.readDB.Query(`
		SELECT id, session_id, role, content, COALESCE(agent_name,''), created_at,
		       COALESCE(reply_to_session,''), COALESCE(quoted_content,''),
		       COALESCE(tool_use_id,''), COALESCE(output,'')
		FROM cc_messages WHERE session_id=? AND agent_name=?
		ORDER BY created_at ASC LIMIT ?
	`, sessionID, agentName, limit)
	if err != nil {
		return nil, fmt.Errorf("list messages by agent: %w", err)
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
	rows, err := s.readDB.Query(`
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
	_, err := s.writeDB.Exec(`DELETE FROM cc_messages WHERE id = ?`, id)
	return err
}

func (s *Storage) DeleteMessages(sessionID string) error {
	if _, err := s.writeDB.Exec(`DELETE FROM cc_attachments WHERE session_id = ?`, sessionID); err != nil {
		return fmt.Errorf("delete attachments: %w", err)
	}
	_, err := s.writeDB.Exec(`DELETE FROM cc_messages WHERE session_id = ?`, sessionID)
	if err != nil {
		return fmt.Errorf("delete messages: %w", err)
	}
	return nil
}

// DeleteNativeMessages deletes all messages for a session from the native claudio messages table.
func (s *Storage) DeleteNativeMessages(sessionID string) error {
	_, err := s.writeDB.Exec(`DELETE FROM messages WHERE session_id = ?`, sessionID)
	return err
}

// InsertNativeMessage inserts a text message into the native claudio messages table.
func (s *Storage) InsertNativeMessage(sessionID, role, content string, ts time.Time) error {
	id := fmt.Sprintf("%d", ts.UnixNano())
	_, err := s.writeDB.Exec(
		`INSERT INTO messages (id, session_id, role, content, type, tool_use_id, tool_name, created_at) VALUES (?, ?, ?, ?, 'text', '', '', ?)`,
		id, sessionID, role, content, ts,
	)
	return err
}

// GetTask returns a single task by ID from team_tasks, filtered by sessionID.
func (s *Storage) GetTask(id, sessionID string) (Task, error) {
	var t Task
	var blocksJSON, blockedByJSON, metadataJSON string
	err := s.readDB.QueryRow(`
		SELECT id, session_id, subject, COALESCE(description,''), status, COALESCE(assigned_to,''),
		       COALESCE(blocks,'[]'), COALESCE(blocked_by,'[]'), COALESCE(metadata,'{}'),
		       created_at, updated_at
		FROM team_tasks WHERE id=? AND session_id=?
	`, id, sessionID).Scan(&t.ID, &t.SessionID, &t.Subject, &t.Description, &t.Status,
		&t.AssignedTo, &blocksJSON, &blockedByJSON, &metadataJSON,
		&t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return Task{}, fmt.Errorf("get task %q: %w", id, err)
	}
	_ = json.Unmarshal([]byte(blocksJSON), &t.Blocks)
	_ = json.Unmarshal([]byte(blockedByJSON), &t.BlockedBy)
	_ = json.Unmarshal([]byte(metadataJSON), &t.Metadata)
	return t, nil
}

// UpsertTask inserts or updates a task record in team_tasks.
func (s *Storage) UpsertTask(task Task) error {
	blocksJSON, _ := json.Marshal(task.Blocks)
	blockedByJSON, _ := json.Marshal(task.BlockedBy)
	metadataJSON, _ := json.Marshal(task.Metadata)
	if task.Blocks == nil {
		blocksJSON = []byte("[]")
	}
	if task.BlockedBy == nil {
		blockedByJSON = []byte("[]")
	}
	if task.Metadata == nil {
		metadataJSON = []byte("{}")
	}
	_, err := s.writeDB.Exec(`
		INSERT INTO team_tasks (id, session_id, subject, description, status, assigned_to, blocks, blocked_by, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id, session_id) DO UPDATE SET
			subject=excluded.subject,
			description=excluded.description,
			status=excluded.status,
			assigned_to=excluded.assigned_to,
			blocks=excluded.blocks,
			blocked_by=excluded.blocked_by,
			metadata=excluded.metadata,
			updated_at=excluded.updated_at
	`, task.ID, task.SessionID, task.Subject, task.Description, task.Status,
		task.AssignedTo, string(blocksJSON), string(blockedByJSON), string(metadataJSON),
		task.CreatedAt, task.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert task: %w", err)
	}
	return nil
}

// UpsertAgent inserts or updates an agent record.
func (s *Storage) UpsertAgent(agent Agent) error {
	_, err := s.writeDB.Exec(`
		INSERT INTO cc_agents (id, session_id, name, status, current_task_id, current_tool, call_count, elapsed_secs, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			status=excluded.status,
			current_task_id=excluded.current_task_id,
			current_tool=excluded.current_tool,
			call_count=excluded.call_count,
			elapsed_secs=excluded.elapsed_secs,
			updated_at=excluded.updated_at
	`, agent.ID, agent.SessionID, agent.Name, agent.Status, agent.CurrentTaskID,
		agent.CurrentTool, agent.CallCount, agent.ElapsedSecs, agent.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert agent: %w", err)
	}
	return nil
}

// UnreadCount returns the count of unread messages for a session.
// Messages are unread if created_at > last_read_at (or all if last_read_at is NULL).
func (s *Storage) UnreadCount(sessionID string) (int, error) {
	var count int
	err := s.readDB.QueryRow(`
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
	_, err := s.writeDB.Exec(
		`UPDATE cc_sessions SET last_read_at = CURRENT_TIMESTAMP WHERE id = ?`,
		sessionID)
	if err != nil {
		return fmt.Errorf("mark read: %w", err)
	}
	return nil
}

// ListTasks returns all tasks for a session from team_tasks.
func (s *Storage) ListTasks(sessionID string) ([]Task, error) {
	rows, err := s.readDB.Query(`
		SELECT id, session_id, subject, COALESCE(description,''), status, COALESCE(assigned_to,''),
		       COALESCE(blocks,'[]'), COALESCE(blocked_by,'[]'), COALESCE(metadata,'{}'),
		       created_at, updated_at
		FROM team_tasks WHERE session_id=? AND status != 'deleted' ORDER BY created_at DESC
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		var blocksJSON, blockedByJSON, metadataJSON string
		if err := rows.Scan(&t.ID, &t.SessionID, &t.Subject, &t.Description, &t.Status,
			&t.AssignedTo, &blocksJSON, &blockedByJSON, &metadataJSON,
			&t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		_ = json.Unmarshal([]byte(blocksJSON), &t.Blocks)
		_ = json.Unmarshal([]byte(blockedByJSON), &t.BlockedBy)
		_ = json.Unmarshal([]byte(metadataJSON), &t.Metadata)
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// ListAgents returns all agents for a session.
func (s *Storage) ListAgents(sessionID string) ([]Agent, error) {
	rows, err := s.readDB.Query(`
		SELECT id, session_id, name, status, COALESCE(current_task_id,''),
		       COALESCE(current_tool,''), COALESCE(call_count,0), COALESCE(elapsed_secs,0), updated_at
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
			&a.CurrentTaskID, &a.CurrentTool, &a.CallCount, &a.ElapsedSecs, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

// DeleteAgentsBySession removes all agent records for a session (used by /clear).
func (s *Storage) DeleteAgentsBySession(sessionID string) error {
	_, err := s.writeDB.Exec(`DELETE FROM cc_agents WHERE session_id=?`, sessionID)
	return err
}

// DeleteAgentEventsBySession removes all agent events for a session (used by /clear).
func (s *Storage) DeleteAgentEventsBySession(sessionID string) error {
	_, err := s.writeDB.Exec(`DELETE FROM cc_agent_events WHERE session_id=?`, sessionID)
	return err
}

// InsertAgentEvent persists an agent status event for reconnect replay.
func (s *Storage) InsertAgentEvent(sessionID, agentName, status, payload string) error {
	_, err := s.writeDB.Exec(`
		INSERT INTO cc_agent_events (session_id, agent_name, status, payload, created_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, sessionID, agentName, status, payload)
	if err != nil {
		return fmt.Errorf("insert agent event: %w", err)
	}
	return nil
}

// GetLatestAgentEvents returns the last known status event per agent in a session,
// limited to events from the last 15 minutes. Used for reconnect replay.
func (s *Storage) GetLatestAgentEvents(sessionID string) ([]AgentEvent, error) {
	rows, err := s.readDB.Query(`
		SELECT ae.session_id, ae.agent_name, ae.status, ae.payload, ae.created_at
		FROM cc_agent_events ae
		INNER JOIN (
			SELECT agent_name, MAX(id) AS max_id
			FROM cc_agent_events
			WHERE session_id = ? AND created_at >= datetime('now', '-15 minutes')
			GROUP BY agent_name
		) latest ON ae.id = latest.max_id
		ORDER BY ae.created_at DESC
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get latest agent events: %w", err)
	}
	defer rows.Close()

	var events []AgentEvent
	for rows.Next() {
		var e AgentEvent
		if err := rows.Scan(&e.SessionID, &e.AgentName, &e.Status, &e.Payload, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan agent event: %w", err)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// PruneAgentEvents removes agent events older than 15 minutes.
func (s *Storage) PruneAgentEvents() error {
	_, err := s.writeDB.Exec(`DELETE FROM cc_agent_events WHERE created_at < datetime('now', '-15 minutes')`)
	return err
}

// InsertAttachment stores a new attachment record.
func (s *Storage) InsertAttachment(att Attachment) error {
	_, err := s.writeDB.Exec(`
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
	rows, err := s.readDB.Query(`
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
	if sub.ID == "" {
		sub.ID = newID()
	}
	_, err := s.writeDB.Exec(`
		INSERT INTO cc_push_subscriptions (id, endpoint, p256dh, auth, created_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(endpoint) DO UPDATE SET
			p256dh=excluded.p256dh,
			auth=excluded.auth
	`, sub.ID, sub.Endpoint, sub.P256dh, sub.Auth)
	if err != nil {
		return fmt.Errorf("save push subscription: %w", err)
	}
	return nil
}

// ListPushSubscriptions returns all stored push subscriptions.
func (s *Storage) ListPushSubscriptions() ([]PushSubscription, error) {
	rows, err := s.readDB.Query(`
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
	_, err := s.writeDB.Exec(`DELETE FROM cc_push_subscriptions WHERE endpoint=?`, endpoint)
	if err != nil {
		return fmt.Errorf("delete push subscription: %w", err)
	}
	return nil
}

// GetOrCreateVAPIDKeys returns stored VAPID keys, generating and storing them on first call.
func (s *Storage) GetOrCreateVAPIDKeys() (public, private string, err error) {
	err = s.writeDB.QueryRow(`SELECT public_key, private_key FROM cc_vapid_keys WHERE id=1`).
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

	_, err = s.writeDB.Exec(`INSERT INTO cc_vapid_keys (id, public_key, private_key) VALUES (1, ?, ?)`, pub, priv)
	if err != nil {
		return "", "", fmt.Errorf("store vapid keys: %w", err)
	}
	return pub, priv, nil
}

// ListMessageAttachments returns all attachments linked to a specific message.
func (s *Storage) ListMessageAttachments(messageID string) ([]Attachment, error) {
	rows, err := s.readDB.Query(`
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
