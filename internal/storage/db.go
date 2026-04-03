package storage

import (
	"database/sql"
	"fmt"

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
	}

	for i, m := range migrations {
		if i < version {
			continue // already applied
		}
		if _, err := db.conn.Exec(m); err != nil {
			return fmt.Errorf("migration error: %w\nSQL: %s", err, m)
		}
		if _, err := db.conn.Exec(`INSERT INTO schema_version (version) VALUES (?)`, i+1); err != nil {
			return fmt.Errorf("migration error updating version: %w", err)
		}
	}

	return nil
}
