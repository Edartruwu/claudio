package storage

import (
	"path/filepath"
	"testing"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("openTestDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestUpdateSessionAgentType(t *testing.T) {
	db := openTestDB(t)

	sess, err := db.CreateSession("/tmp/proj", "claude-sonnet-4-6")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if err := db.UpdateSessionAgentType(sess.ID, "prab"); err != nil {
		t.Fatalf("UpdateSessionAgentType: %v", err)
	}

	got, err := db.GetSession(sess.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got == nil {
		t.Fatal("GetSession returned nil")
	}
	if got.AgentType != "prab" {
		t.Errorf("AgentType = %q, want %q", got.AgentType, "prab")
	}
}

func TestUpdateSessionTeamTemplate(t *testing.T) {
	db := openTestDB(t)

	sess, err := db.CreateSession("/tmp/proj", "claude-sonnet-4-6")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if err := db.UpdateSessionTeamTemplate(sess.ID, "backend-team"); err != nil {
		t.Fatalf("UpdateSessionTeamTemplate: %v", err)
	}

	got, err := db.GetSession(sess.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got == nil {
		t.Fatal("GetSession returned nil")
	}
	if got.TeamTemplate != "backend-team" {
		t.Errorf("TeamTemplate = %q, want %q", got.TeamTemplate, "backend-team")
	}
}

func TestUpdateSessionAgentType_NonexistentID(t *testing.T) {
	db := openTestDB(t)

	// SQLite UPDATE with no matching rows is not an error.
	if err := db.UpdateSessionAgentType("does-not-exist", "prab"); err != nil {
		t.Errorf("expected no error for nonexistent ID, got: %v", err)
	}
}
