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

// TestGetBranchMessages_EmptyBranch verifies that a branch with zero own
// messages still returns the parent messages up to (and including) the fork point.
func TestGetBranchMessages_EmptyBranch(t *testing.T) {
	db := openTestDB(t)

	parent, _ := db.CreateSession("/tmp/proj", "claude-sonnet-4-6")
	addTestMessages(t, db, parent.ID, "p1", "p2", "p3")
	parentMsgs, _ := db.GetMessages(parent.ID)

	// Fork after p2 (index 1). No messages added to branch.
	branch, _ := db.CreateBranchSession(parent.ID, parentMsgs[1].ID, "/tmp/proj", "claude-sonnet-4-6")

	msgs, err := db.GetBranchMessages(branch.ID)
	if err != nil {
		t.Fatalf("GetBranchMessages: %v", err)
	}

	// Expect only p1, p2 — p3 excluded, branch has no own msgs.
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
	if msgs[0].Content != "p1" {
		t.Errorf("msgs[0].Content = %q, want %q", msgs[0].Content, "p1")
	}
	if msgs[1].Content != "p2" {
		t.Errorf("msgs[1].Content = %q, want %q", msgs[1].Content, "p2")
	}
}

// TestCreateBranchSession_FieldsSet verifies both parent_session_id AND
// branch_from_message_id are persisted, and that the branch metadata is correct.
func TestCreateBranchSession_FieldsSet(t *testing.T) {
	db := openTestDB(t)

	parent, err := db.CreateSession("/tmp/proj", "gpt-4")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	addTestMessages(t, db, parent.ID, "hello")
	msgs, _ := db.GetMessages(parent.ID)
	forkMsgID := msgs[0].ID

	branch, err := db.CreateBranchSession(parent.ID, forkMsgID, "/tmp/proj", "gpt-4")
	if err != nil {
		t.Fatalf("CreateBranchSession: %v", err)
	}

	// Verify in-memory fields.
	if branch.ParentSessionID != parent.ID {
		t.Errorf("ParentSessionID = %q, want %q", branch.ParentSessionID, parent.ID)
	}
	if branch.BranchFromMessageID == nil {
		t.Fatal("BranchFromMessageID is nil")
	}
	if *branch.BranchFromMessageID != forkMsgID {
		t.Errorf("BranchFromMessageID = %d, want %d", *branch.BranchFromMessageID, forkMsgID)
	}
	if branch.ProjectDir != parent.ProjectDir {
		t.Errorf("ProjectDir = %q, want %q", branch.ProjectDir, parent.ProjectDir)
	}
	if branch.Model != parent.Model {
		t.Errorf("Model = %q, want %q", branch.Model, parent.Model)
	}

	// Verify round-trip from DB.
	persisted, err := db.GetSession(branch.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if persisted.ParentSessionID != parent.ID {
		t.Errorf("persisted ParentSessionID = %q, want %q", persisted.ParentSessionID, parent.ID)
	}
	if persisted.BranchFromMessageID == nil || *persisted.BranchFromMessageID != forkMsgID {
		t.Errorf("persisted BranchFromMessageID mismatch")
	}
}

// TestListSessions_StillExcludesBranches verifies that after creating multiple
// branch sessions, ListSessions returns only root-level sessions.
func TestListSessions_StillExcludesBranches(t *testing.T) {
	db := openTestDB(t)

	root1, _ := db.CreateSession("/tmp/proj", "claude-sonnet-4-6")
	root2, _ := db.CreateSession("/tmp/proj", "claude-sonnet-4-6")
	addTestMessages(t, db, root1.ID, "r1a", "r1b")
	addTestMessages(t, db, root2.ID, "r2a")

	root1Msgs, _ := db.GetMessages(root1.ID)
	root2Msgs, _ := db.GetMessages(root2.ID)

	// Create two branches from root1 and one from root2.
	db.CreateBranchSession(root1.ID, root1Msgs[0].ID, "/tmp/proj", "claude-sonnet-4-6")
	db.CreateBranchSession(root1.ID, root1Msgs[1].ID, "/tmp/proj", "claude-sonnet-4-6")
	db.CreateBranchSession(root2.ID, root2Msgs[0].ID, "/tmp/proj", "claude-sonnet-4-6")

	sessions, err := db.ListSessions(100)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}

	// Only root1 and root2 should appear.
	if len(sessions) != 2 {
		t.Fatalf("got %d sessions, want 2", len(sessions))
	}
	ids := map[string]bool{sessions[0].ID: true, sessions[1].ID: true}
	if !ids[root1.ID] {
		t.Errorf("root1 missing from ListSessions")
	}
	if !ids[root2.ID] {
		t.Errorf("root2 missing from ListSessions")
	}
}

// TestGetBranchMessages_BranchFromFirstMessage verifies that branching from
// the very first message inherits exactly one parent message.
func TestGetBranchMessages_BranchFromFirstMessage(t *testing.T) {
	db := openTestDB(t)

	parent, _ := db.CreateSession("/tmp/proj", "claude-sonnet-4-6")
	addTestMessages(t, db, parent.ID, "first", "second", "third")
	parentMsgs, _ := db.GetMessages(parent.ID)

	// Fork from first message.
	branch, _ := db.CreateBranchSession(parent.ID, parentMsgs[0].ID, "/tmp/proj", "claude-sonnet-4-6")
	addTestMessages(t, db, branch.ID, "branch-msg")

	msgs, err := db.GetBranchMessages(branch.ID)
	if err != nil {
		t.Fatalf("GetBranchMessages: %v", err)
	}

	// Expect: first (1 parent msg) + branch-msg (1 own)
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
	if msgs[0].Content != "first" {
		t.Errorf("msgs[0].Content = %q, want %q", msgs[0].Content, "first")
	}
	if msgs[1].Content != "branch-msg" {
		t.Errorf("msgs[1].Content = %q, want %q", msgs[1].Content, "branch-msg")
	}
}

// TestGetBranchMessages_BranchFromLastMessage verifies that branching from the
// last parent message inherits all parent messages.
func TestGetBranchMessages_BranchFromLastMessage(t *testing.T) {
	db := openTestDB(t)

	parent, _ := db.CreateSession("/tmp/proj", "claude-sonnet-4-6")
	addTestMessages(t, db, parent.ID, "a", "b", "c")
	parentMsgs, _ := db.GetMessages(parent.ID)

	// Fork from last parent message.
	lastMsgID := parentMsgs[len(parentMsgs)-1].ID
	branch, _ := db.CreateBranchSession(parent.ID, lastMsgID, "/tmp/proj", "claude-sonnet-4-6")
	addTestMessages(t, db, branch.ID, "branch-only")

	msgs, err := db.GetBranchMessages(branch.ID)
	if err != nil {
		t.Fatalf("GetBranchMessages: %v", err)
	}

	// Expect: a, b, c (all parent), branch-only (own)
	if len(msgs) != 4 {
		t.Fatalf("got %d messages, want 4", len(msgs))
	}
	wantContents := []string{"a", "b", "c", "branch-only"}
	for i, want := range wantContents {
		if msgs[i].Content != want {
			t.Errorf("msgs[%d].Content = %q, want %q", i, msgs[i].Content, want)
		}
	}
}

// TestGetSessionBranches_ReturnsBranchesOnly verifies GetSessionBranches
// returns only sessions that are branches (not sub-agent sessions).
func TestGetSessionBranches_ReturnsBranchesOnly(t *testing.T) {
	db := openTestDB(t)

	parent, _ := db.CreateSession("/tmp/proj", "claude-sonnet-4-6")
	addTestMessages(t, db, parent.ID, "r1")
	msgs, _ := db.GetMessages(parent.ID)

	b1, _ := db.CreateBranchSession(parent.ID, msgs[0].ID, "/tmp/proj", "claude-sonnet-4-6")
	b2, _ := db.CreateBranchSession(parent.ID, msgs[0].ID, "/tmp/proj", "claude-sonnet-4-6")
	// Sub-agent session — should NOT appear in GetSessionBranches.
	db.CreateSubSession(parent.ID, "Explore", "/tmp/proj", "claude-sonnet-4-6")

	branches, err := db.GetSessionBranches(parent.ID)
	if err != nil {
		t.Fatalf("GetSessionBranches: %v", err)
	}

	if len(branches) != 2 {
		t.Fatalf("got %d branches, want 2", len(branches))
	}
	ids := map[string]bool{branches[0].ID: true, branches[1].ID: true}
	if !ids[b1.ID] {
		t.Errorf("branch b1 missing")
	}
	if !ids[b2.ID] {
		t.Errorf("branch b2 missing")
	}
}
