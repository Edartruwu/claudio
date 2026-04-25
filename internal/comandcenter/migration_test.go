package comandcenter

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// openRawSQLite opens a bare SQLite connection (no migrations) for setup.
func openRawSQLite(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_journal_mode=WAL")
	if err != nil {
		t.Fatalf("openRawSQLite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// simulateClaudiaDB runs the minimal claudio schema_version bootstrap
// against a raw DB, simulating a pre-existing claudio.db at a given version.
func simulateClaudioDB(t *testing.T, db *sql.DB, atVersion int) {
	t.Helper()
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL DEFAULT 0)`); err != nil {
		t.Fatalf("simulateClaudioDB create schema_version: %v", err)
	}
	// Create the minimal claudio tables so FK constraints pass.
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL DEFAULT '',
		project_dir TEXT NOT NULL DEFAULT '',
		model TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		t.Fatalf("simulateClaudioDB create sessions: %v", err)
	}
	if atVersion > 0 {
		if _, err := db.Exec(`INSERT INTO schema_version (version) VALUES (?)`, atVersion); err != nil {
			t.Fatalf("simulateClaudioDB insert version: %v", err)
		}
	}
}

// openStorageOnDB wraps an existing *sql.DB in a Storage and runs CC migrations.
// This lets us test migration against a pre-populated DB without using the file path.
func openStorageOnDB(t *testing.T, db *sql.DB) *Storage {
	t.Helper()
	s := &Storage{writeDB: db, readDB: db}
	if err := s.migrate(); err != nil {
		t.Fatalf("openStorageOnDB migrate: %v", err)
	}
	t.Cleanup(func() { s.writeDB.Close() }) // readDB is same conn, close once
	return s
}

// TestMigration_FreshDB verifies all cc_ tables are created on a new empty DB.
func TestMigration_FreshDB(t *testing.T) {
	s := newTestStorage(t)

	tables := []string{
		"cc_sessions", "cc_messages", "cc_agents",
		"cc_attachments", "cc_push_subscriptions", "cc_vapid_keys",
		"cc_schema_version",
	}
	for _, table := range tables {
		var n int
		err := s.readDB.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&n)
		if err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if n != 1 {
			t.Errorf("table %q not created after fresh migration", table)
		}
	}
}

// TestMigration_Idempotent verifies running Open twice on the same in-memory DB
// does not fail and does not duplicate data.
func TestMigration_CC_Idempotent(t *testing.T) {
	s1 := newTestStorage(t)

	// Write data.
	sess := Session{
		ID: "idem-1", Name: "test", Path: "/tmp", Status: "active",
		CreatedAt: time.Now(), LastActiveAt: time.Now(),
	}
	if err := s1.UpsertSession(sess); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	// Run migrations again on the same underlying db — must not error.
	s2 := &Storage{writeDB: s1.writeDB, readDB: s1.readDB}
	if err := s2.migrate(); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	// Data still intact.
	sessions, err := s1.ListSessions("")
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("expected 1 session after second migration, got %d", len(sessions))
	}
}

// TestMigration_VersionTableNoCollision verifies that cc_schema_version is
// separate from schema_version (claudio's table).  When both exist, each
// tracks its own version — CC migrations don't skip because claudio's version
// is non-zero.
func TestMigration_VersionTableNoCollision(t *testing.T) {
	raw := openRawSQLite(t)
	// Simulate a claudio.db at version 22 (fully migrated).
	simulateClaudioDB(t, raw, 22)

	// Now run CC migrations on the same DB.
	s := openStorageOnDB(t, raw)

	// All cc_ tables must exist despite claudio's schema_version being 22.
	tables := []string{"cc_sessions", "cc_messages", "cc_agents", "cc_schema_version"}
	for _, table := range tables {
		var n int
		if err := raw.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&n); err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if n != 1 {
			t.Errorf("table %q missing — CC migrations skipped due to version collision", table)
		}
	}

	// CC schema_version must be 23 (all migrations applied, including cc_agent_events migration 23).
	var ccVersion int
	if err := raw.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM cc_schema_version`).Scan(&ccVersion); err != nil {
		t.Fatalf("read cc_schema_version: %v", err)
	}
	if ccVersion != 23 {
		t.Errorf("cc_schema_version: got %d, want 23", ccVersion)
	}

	// Claudio's schema_version must still be 22 — untouched.
	var claudiaVersion int
	if err := raw.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&claudiaVersion); err != nil {
		t.Fatalf("read schema_version: %v", err)
	}
	if claudiaVersion != 22 {
		t.Errorf("claudio schema_version: got %d, want 22 (CC migrations must not touch it)", claudiaVersion)
	}

	// Basic write/read must work after migration on shared DB.
	sess := Session{
		ID: "shared-1", Name: "shared", Path: "/tmp", Status: "active",
		CreatedAt: time.Now(), LastActiveAt: time.Now(),
	}
	if err := s.UpsertSession(sess); err != nil {
		t.Fatalf("UpsertSession on shared DB: %v", err)
	}
	sessions, err := s.ListSessions("")
	if err != nil {
		t.Fatalf("ListSessions on shared DB: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("expected 1 session on shared DB, got %d", len(sessions))
	}
}

// TestMigration_SharedDB_TasksFromNativeTable verifies the production scenario:
// CC opens claudio.db → ListTasks reads from team_tasks (claudio's native table)
// → single source of truth, zero lag, full description round-trip.
func TestMigration_SharedDB_TasksFromNativeTable(t *testing.T) {
	raw := openRawSQLite(t)
	simulateClaudioDB(t, raw, 22)

	// Seed team_tasks directly (as claudio's TaskCreate tool would).
	if _, err := raw.Exec(`CREATE TABLE IF NOT EXISTS team_tasks (
		id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		title TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'pending',
		assigned_to TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (id, session_id)
	)`); err != nil {
		t.Fatalf("create team_tasks: %v", err)
	}

	now := time.Now()
	_, err := raw.Exec(`
		INSERT INTO team_tasks (id, session_id, title, description, status, assigned_to, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"tt-1", "sess-native", "Implement auth middleware",
		"**Details:**\n- JWT\n- 24h expiry", "in_progress", "prab", now, now)
	if err != nil {
		t.Fatalf("seed team_tasks: %v", err)
	}

	// Open CC storage on same DB.
	s := openStorageOnDB(t, raw)

	// CC sessions table tracks the session.
	sess := Session{
		ID: "sess-native", Name: "main", Path: "/proj", Status: "active",
		CreatedAt: now, LastActiveAt: now,
	}
	if err := s.UpsertSession(sess); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	// ListTasks must read from team_tasks (single source of truth).
	tasks, err := s.ListTasks("sess-native")
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task from team_tasks, got %d", len(tasks))
	}
	if tasks[0].Title != "Implement auth middleware" {
		t.Errorf("Title: got %q, want %q", tasks[0].Title, "Implement auth middleware")
	}
	if tasks[0].Description != "**Details:**\n- JWT\n- 24h expiry" {
		t.Errorf("Description: got %q, want %q", tasks[0].Description, "**Details:**\n- JWT\n- 24h expiry")
	}

	// GetTask must also read from team_tasks.
	got, err := s.GetTask("tt-1", "sess-native")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != "in_progress" {
		t.Errorf("Status: got %q, want in_progress", got.Status)
	}
	if got.AssignedTo != "prab" {
		t.Errorf("AssignedTo: got %q, want prab", got.AssignedTo)
	}
}

// TestMigration_SharedDB_DeletedTasksFiltered verifies ListTasks excludes
// deleted tasks when reading from team_tasks.
func TestMigration_SharedDB_DeletedTasksFiltered(t *testing.T) {
	raw := openRawSQLite(t)
	simulateClaudioDB(t, raw, 22)

	if _, err := raw.Exec(`CREATE TABLE IF NOT EXISTS team_tasks (
		id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		title TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'pending',
		assigned_to TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (id, session_id)
	)`); err != nil {
		t.Fatalf("create team_tasks: %v", err)
	}

	now := time.Now()
	for _, row := range []struct{ id, status string }{
		{"t1", "pending"},
		{"t2", "deleted"},
		{"t3", "done"},
	} {
		if _, err := raw.Exec(
			`INSERT INTO team_tasks (id, session_id, title, status, created_at, updated_at)
			 VALUES (?, 'sess-del', ?, ?, ?, ?)`,
			row.id, "task "+row.id, row.status, now, now,
		); err != nil {
			t.Fatalf("seed %s: %v", row.id, err)
		}
	}

	s := openStorageOnDB(t, raw)
	sess := Session{ID: "sess-del", Name: "s", Path: "/tmp", Status: "active", CreatedAt: now, LastActiveAt: now}
	if err := s.UpsertSession(sess); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	tasks, err := s.ListTasks("sess-del")
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks (deleted excluded), got %d", len(tasks))
	}
	for _, tk := range tasks {
		if tk.Status == "deleted" {
			t.Errorf("deleted task %q leaked into ListTasks", tk.ID)
		}
	}
}

// TestMigration_CC_ToolFields verifies migrations 16+17 (tool_use_id, output).
func TestMigration_CC_ToolFields(t *testing.T) {
	s := newTestStorage(t)

	sess := Session{
		ID: "tool-sess-1", Name: "s", Path: "/tmp", Status: "active",
		CreatedAt: time.Now(), LastActiveAt: time.Now(),
	}
	if err := s.UpsertSession(sess); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	// Insert a tool_use message with tool_use_id.
	msg := Message{
		ID:        "tool-msg-1",
		SessionID: sess.ID,
		Role:      "tool_use",
		Content:   "Read: {\"file_path\": \"/tmp/foo.go\"}",
		ToolUseID: "tu-abc123",
		CreatedAt: time.Now(),
	}
	if err := s.InsertMessage(msg); err != nil {
		t.Fatalf("InsertMessage: %v", err)
	}

	// Set output via UpdateMessageOutput.
	if err := s.UpdateMessageOutput(sess.ID, "tu-abc123", "file contents here"); err != nil {
		t.Fatalf("UpdateMessageOutput: %v", err)
	}

	msgs, err := s.ListMessages(sess.ID, 10)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].ToolUseID != "tu-abc123" {
		t.Errorf("ToolUseID: got %q, want %q", msgs[0].ToolUseID, "tu-abc123")
	}
	if msgs[0].Output != "file contents here" {
		t.Errorf("Output: got %q, want %q", msgs[0].Output, "file contents here")
	}
}


