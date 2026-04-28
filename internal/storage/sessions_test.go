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

// ---------------------------------------------------------------------------
// Session branching tests
// ---------------------------------------------------------------------------

func addTestMessages(t *testing.T, db *DB, sessionID string, contents ...string) {
	t.Helper()
	for _, c := range contents {
		if err := db.AddMessage(sessionID, "user", c, "user", "", ""); err != nil {
			t.Fatalf("AddMessage: %v", err)
		}
	}
}

func TestCreateBranchSession(t *testing.T) {
	db := openTestDB(t)

	parent, err := db.CreateSession("/tmp/proj", "claude-sonnet-4-6")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	addTestMessages(t, db, parent.ID, "msg1", "msg2", "msg3")

	msgs, _ := db.GetMessages(parent.ID)
	branchMsgID := msgs[1].ID // fork after msg2

	branch, err := db.CreateBranchSession(parent.ID, branchMsgID, "/tmp/proj", "claude-sonnet-4-6")
	if err != nil {
		t.Fatalf("CreateBranchSession: %v", err)
	}

	if branch.ParentSessionID != parent.ID {
		t.Errorf("ParentSessionID = %q, want %q", branch.ParentSessionID, parent.ID)
	}
	if branch.BranchFromMessageID == nil {
		t.Fatal("BranchFromMessageID is nil")
	}
	if *branch.BranchFromMessageID != branchMsgID {
		t.Errorf("BranchFromMessageID = %d, want %d", *branch.BranchFromMessageID, branchMsgID)
	}

	// Verify persisted
	got, err := db.GetSession(branch.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.BranchFromMessageID == nil || *got.BranchFromMessageID != branchMsgID {
		t.Errorf("persisted BranchFromMessageID mismatch")
	}
}

func TestGetBranchMessages_Simple(t *testing.T) {
	db := openTestDB(t)

	parent, _ := db.CreateSession("/tmp/proj", "claude-sonnet-4-6")
	addTestMessages(t, db, parent.ID, "p1", "p2", "p3", "p4")

	parentMsgs, _ := db.GetMessages(parent.ID)
	// Fork after p2 (index 1)
	forkID := parentMsgs[1].ID

	branch, _ := db.CreateBranchSession(parent.ID, forkID, "/tmp/proj", "claude-sonnet-4-6")
	addTestMessages(t, db, branch.ID, "b1", "b2")

	msgs, err := db.GetBranchMessages(branch.ID)
	if err != nil {
		t.Fatalf("GetBranchMessages: %v", err)
	}

	// Expect: p1, p2 (from parent, ≤ forkID), b1, b2 (own)
	if len(msgs) != 4 {
		t.Fatalf("got %d messages, want 4", len(msgs))
	}
	wantContents := []string{"p1", "p2", "b1", "b2"}
	for i, want := range wantContents {
		if msgs[i].Content != want {
			t.Errorf("msgs[%d].Content = %q, want %q", i, msgs[i].Content, want)
		}
	}
}

func TestGetBranchMessages_DeepChain(t *testing.T) {
	db := openTestDB(t)

	// root → branch1 → branch2 (depth 2)
	root, _ := db.CreateSession("/tmp/proj", "claude-sonnet-4-6")
	addTestMessages(t, db, root.ID, "r1", "r2", "r3")
	rootMsgs, _ := db.GetMessages(root.ID)

	branch1, _ := db.CreateBranchSession(root.ID, rootMsgs[1].ID, "/tmp/proj", "claude-sonnet-4-6")
	addTestMessages(t, db, branch1.ID, "b1-1", "b1-2", "b1-3")
	b1Msgs, _ := db.GetMessages(branch1.ID)

	branch2, _ := db.CreateBranchSession(branch1.ID, b1Msgs[0].ID, "/tmp/proj", "claude-sonnet-4-6")
	addTestMessages(t, db, branch2.ID, "b2-1")

	msgs, err := db.GetBranchMessages(branch2.ID)
	if err != nil {
		t.Fatalf("GetBranchMessages: %v", err)
	}

	// Expect: r1, r2 (root, ≤ rootMsgs[1]), b1-1 (branch1, ≤ b1Msgs[0]), b2-1 (own)
	wantContents := []string{"r1", "r2", "b1-1", "b2-1"}
	if len(msgs) != len(wantContents) {
		t.Fatalf("got %d messages, want %d", len(msgs), len(wantContents))
	}
	for i, want := range wantContents {
		if msgs[i].Content != want {
			t.Errorf("msgs[%d].Content = %q, want %q", i, msgs[i].Content, want)
		}
	}
}

func TestGetBranchMessages_NoBranch(t *testing.T) {
	db := openTestDB(t)

	sess, _ := db.CreateSession("/tmp/proj", "claude-sonnet-4-6")
	addTestMessages(t, db, sess.ID, "m1", "m2")

	msgs, err := db.GetBranchMessages(sess.ID)
	if err != nil {
		t.Fatalf("GetBranchMessages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
	if msgs[0].Content != "m1" || msgs[1].Content != "m2" {
		t.Errorf("unexpected content: %q, %q", msgs[0].Content, msgs[1].Content)
	}
}

func TestGetSessionTree(t *testing.T) {
	db := openTestDB(t)

	root, _ := db.CreateSession("/tmp/proj", "claude-sonnet-4-6")
	addTestMessages(t, db, root.ID, "r1", "r2", "r3")
	rootMsgs, _ := db.GetMessages(root.ID)

	child1, _ := db.CreateBranchSession(root.ID, rootMsgs[0].ID, "/tmp/proj", "claude-sonnet-4-6")
	child2, _ := db.CreateBranchSession(root.ID, rootMsgs[1].ID, "/tmp/proj", "claude-sonnet-4-6")

	addTestMessages(t, db, child1.ID, "c1-1")
	c1Msgs, _ := db.GetMessages(child1.ID)
	grandchild, _ := db.CreateBranchSession(child1.ID, c1Msgs[0].ID, "/tmp/proj", "claude-sonnet-4-6")

	tree, err := db.GetSessionTree(root.ID)
	if err != nil {
		t.Fatalf("GetSessionTree: %v", err)
	}

	if tree.Session.ID != root.ID {
		t.Errorf("root ID = %q, want %q", tree.Session.ID, root.ID)
	}
	if len(tree.Children) != 2 {
		t.Fatalf("root children = %d, want 2", len(tree.Children))
	}

	// child1 should have 1 grandchild, child2 should have 0
	var c1Node, c2Node *SessionNode
	for _, c := range tree.Children {
		if c.Session.ID == child1.ID {
			c1Node = c
		} else if c.Session.ID == child2.ID {
			c2Node = c
		}
	}
	if c1Node == nil || c2Node == nil {
		t.Fatal("missing child nodes")
	}
	if len(c1Node.Children) != 1 {
		t.Fatalf("child1 children = %d, want 1", len(c1Node.Children))
	}
	if c1Node.Children[0].Session.ID != grandchild.ID {
		t.Errorf("grandchild ID = %q, want %q", c1Node.Children[0].Session.ID, grandchild.ID)
	}
	if len(c2Node.Children) != 0 {
		t.Errorf("child2 children = %d, want 0", len(c2Node.Children))
	}
}

func TestGetRootSession(t *testing.T) {
	db := openTestDB(t)

	root, _ := db.CreateSession("/tmp/proj", "claude-sonnet-4-6")
	addTestMessages(t, db, root.ID, "r1")
	rootMsgs, _ := db.GetMessages(root.ID)

	child, _ := db.CreateBranchSession(root.ID, rootMsgs[0].ID, "/tmp/proj", "claude-sonnet-4-6")
	addTestMessages(t, db, child.ID, "c1")
	childMsgs, _ := db.GetMessages(child.ID)

	grandchild, _ := db.CreateBranchSession(child.ID, childMsgs[0].ID, "/tmp/proj", "claude-sonnet-4-6")

	got, err := db.GetRootSession(grandchild.ID)
	if err != nil {
		t.Fatalf("GetRootSession: %v", err)
	}
	if got.ID != root.ID {
		t.Errorf("root ID = %q, want %q", got.ID, root.ID)
	}

	// Root of root = itself
	got2, err := db.GetRootSession(root.ID)
	if err != nil {
		t.Fatalf("GetRootSession(root): %v", err)
	}
	if got2.ID != root.ID {
		t.Errorf("root of root = %q, want %q", got2.ID, root.ID)
	}
}

func TestListSessions_ExcludesBranches(t *testing.T) {
	db := openTestDB(t)

	root, _ := db.CreateSession("/tmp/proj", "claude-sonnet-4-6")
	addTestMessages(t, db, root.ID, "r1")
	rootMsgs, _ := db.GetMessages(root.ID)

	// Create a branch (should be excluded from ListSessions)
	db.CreateBranchSession(root.ID, rootMsgs[0].ID, "/tmp/proj", "claude-sonnet-4-6")

	// Create a sub-agent session (also excluded)
	db.CreateSubSession(root.ID, "Explore", "/tmp/proj", "claude-sonnet-4-6")

	sessions, err := db.ListSessions(100)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}

	// Only root should appear
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}
	if sessions[0].ID != root.ID {
		t.Errorf("session ID = %q, want %q", sessions[0].ID, root.ID)
	}
}
