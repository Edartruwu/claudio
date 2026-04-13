package storage

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

// DB wraps the SQLite database connection.
type DB struct {
	conn *sql.DB
}

// Open creates or opens a SQLite database at the given path.
func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable WAL mode for better concurrent read performance
	if _, err := conn.Exec("PRAGMA journal_mode=WAL"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to set WAL mode: %w", err)
	}

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migration failed: %w", err)
	}

	return db, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// Conn returns the underlying sql.DB for direct queries.
func (db *DB) Conn() *sql.DB {
	return db.conn
}

func (db *DB) migrate() error {
	// Bootstrap the version table. This is always safe to run.
	if _, err := db.conn.Exec(
		`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL DEFAULT 0)`,
	); err != nil {
		return fmt.Errorf("migration error: %w", err)
	}

	var version int
	if err := db.conn.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&version); err != nil {
		return fmt.Errorf("migration error reading version: %w", err)
	}

	// First time running with the versioned system: the DB may already have
	// columns from the old run-everything approach. Detect what's present and
	// fast-forward the version so we don't re-run migrations that already
	// succeeded.
	if version == 0 {
		version = db.detectExistingSchemaVersion()
		if version > 0 {
			if _, err := db.conn.Exec(`INSERT INTO schema_version (version) VALUES (?)`, version); err != nil {
				return fmt.Errorf("migration error bootstrapping version: %w", err)
			}
		}
	}

	// Each entry is a migration applied exactly once, in order.
	// Never edit existing entries — only append new ones.
	migrations := []string{
		// 1
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL DEFAULT '',
			project_dir TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			summary TEXT DEFAULT ''
		)`,
		// 2
		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			type TEXT NOT NULL DEFAULT 'text',
			tool_use_id TEXT DEFAULT '',
			tool_name TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
		)`,
		// 3
		`CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id)`,
		// 4
		`CREATE TABLE IF NOT EXISTS events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			type TEXT NOT NULL,
			payload TEXT NOT NULL DEFAULT '{}',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
		)`,
		// 5
		`CREATE INDEX IF NOT EXISTS idx_events_session ON events(session_id)`,
		// 6
		`CREATE INDEX IF NOT EXISTS idx_events_type ON events(type)`,
		// 7 — sub-agent persistence: link sub-sessions to parent sessions
		`ALTER TABLE sessions ADD COLUMN parent_session_id TEXT NOT NULL DEFAULT ''`,
		// 8
		`ALTER TABLE sessions ADD COLUMN agent_type TEXT NOT NULL DEFAULT ''`,
		// 9
		`CREATE INDEX IF NOT EXISTS idx_sessions_parent ON sessions(parent_session_id)`,
		// 10
		`CREATE TABLE IF NOT EXISTS audit_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT DEFAULT '',
			tool TEXT NOT NULL,
			input_summary TEXT NOT NULL DEFAULT '',
			output_summary TEXT NOT NULL DEFAULT '',
			approval TEXT NOT NULL DEFAULT '',
			tokens_used INTEGER DEFAULT 0,
			duration_ms INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// 11
		`CREATE INDEX IF NOT EXISTS idx_audit_session ON audit_log(session_id)`,
		// 12
		`CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_log(created_at)`,
		// 13
		`CREATE TABLE IF NOT EXISTS instincts (
			id TEXT PRIMARY KEY,
			pattern TEXT NOT NULL,
			context TEXT NOT NULL DEFAULT '',
			confidence REAL NOT NULL DEFAULT 0.5,
			times_confirmed INTEGER DEFAULT 0,
			source_session TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// 14 — recreate team_tasks with session_id so tasks are scoped per session.
		// SQLite doesn't support ALTER TABLE to change the primary key, so we
		// create a shadow table, copy data, drop the old one, then rename.
		`CREATE TABLE IF NOT EXISTS team_tasks_v2 (
			id TEXT NOT NULL,
			session_id TEXT NOT NULL DEFAULT '',
			subject TEXT NOT NULL,
			description TEXT,
			status TEXT DEFAULT 'pending',
			assigned_to TEXT,
			created_at DATETIME,
			updated_at DATETIME,
			PRIMARY KEY (id, session_id)
		)`,
		// 15 — copy existing tasks into the new table under the empty-string session
		`INSERT OR IGNORE INTO team_tasks_v2 (id, session_id, subject, description, status, assigned_to, created_at, updated_at)
			SELECT id, '', subject, description, status, assigned_to, created_at, updated_at FROM team_tasks`,
		// 16 — drop the old table
		`DROP TABLE IF EXISTS team_tasks`,
		// 17 — rename
		`ALTER TABLE team_tasks_v2 RENAME TO team_tasks`,
		// 18 — filter savings tracking for /gain and /discover commands
		`CREATE TABLE IF NOT EXISTS filter_savings (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			command TEXT NOT NULL,
			bytes_in INTEGER NOT NULL DEFAULT 0,
			bytes_out INTEGER NOT NULL DEFAULT 0,
			saved_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// 19 — index for filter savings queries grouped by command
		`CREATE INDEX IF NOT EXISTS idx_filter_savings_command ON filter_savings(command)`,
		// 20 — add team_template column to sessions for team selection tracking
		`ALTER TABLE sessions ADD COLUMN team_template TEXT NOT NULL DEFAULT ''`,
		// 21 — FTS5 virtual table for memory search pre-filtering
		`CREATE VIRTUAL TABLE IF NOT EXISTS memory_fts USING fts5(
    name, scope, description, tags_text, facts_text, concepts_text,
    tokenize='porter ascii'
)`,
		// 22 — metadata table for startup sync (tracks last-indexed mtime per entry)
		`CREATE TABLE IF NOT EXISTS memory_fts_meta (
    name       TEXT NOT NULL,
    scope      TEXT NOT NULL,
    file_mtime INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (name, scope)
)`,
	}

	for i, m := range migrations {
		if i < version {
			continue // already applied
		}
		if _, err := db.conn.Exec(m); err != nil {
			// Treat idempotent failures as success so a partially-applied
			// schema (e.g. from a previous crash) doesn't block startup.
			if !isAlreadyExistsErr(err) {
				return fmt.Errorf("migration error: %w\nSQL: %s", err, m)
			}
		}
		if _, err := db.conn.Exec(`INSERT INTO schema_version (version) VALUES (?)`, i+1); err != nil {
			return fmt.Errorf("migration error updating version: %w", err)
		}
	}

	return nil
}

// isAlreadyExistsErr returns true for SQLite errors that are safe to ignore
// during migrations because the desired state is already present:
//   - "already exists" — table/index/column was already created
//   - "duplicate column name" — ALTER TABLE column already added
//   - "no such table" — a migration copies or drops a table that was never
//     created (e.g. migrating from an old schema that pre-dates the table).
//     If the source table doesn't exist there is nothing to migrate, so the
//     migration is effectively a no-op.
func isAlreadyExistsErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate column name") ||
		strings.Contains(msg, "already exists") ||
		strings.Contains(msg, "no such table")
}

// detectExistingSchemaVersion inspects the live schema to determine how far
// the old (unversioned) migration runner had progressed. This is called exactly
// once when the schema_version table is empty, letting us fast-forward past
// migrations that already succeeded.
func (db *DB) detectExistingSchemaVersion() int {
	hasColumn := func(table, column string) bool {
		rows, err := db.conn.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table))
		if err != nil {
			return false
		}
		defer rows.Close()
		for rows.Next() {
			var cid int
			var name, typ string
			var notNull, pk int
			var dflt interface{}
			if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
				continue
			}
			if name == column {
				return true
			}
		}
		return false
	}
	hasTable := func(table string) bool {
		var n int
		db.conn.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&n)
		return n > 0
	}

	switch {
	case hasTable("memory_fts_meta"):
		return 22
	case hasTable("memory_fts"):
		return 21
	case hasTable("filter_savings"):
		// Check if team_template column exists; if not, we're at version 19
		if !hasColumn("sessions", "team_template") {
			return 19
		}
		return 20
	case hasTable("instincts"):
		return 13
	case hasTable("audit_log"):
		// audit_log was added after the parent_session_id columns (migrations 10-12)
		return 12
	case hasColumn("sessions", "agent_type"):
		return 9
	case hasColumn("sessions", "parent_session_id"):
		return 7
	case hasTable("events"):
		return 6
	case hasTable("messages"):
		return 3
	case hasTable("sessions"):
		return 1
	default:
		return 0
	}
}
