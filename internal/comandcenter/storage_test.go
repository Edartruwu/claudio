package comandcenter

import (
	"testing"
	"time"
)

func newTestStorage(t *testing.T) *Storage {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStorage_UpsertSession(t *testing.T) {
	s := newTestStorage(t)

	sess := Session{
		ID:           "sess-1",
		Name:         "test-session",
		Path:         "/tmp/proj",
		Model:        "claude-opus",
		Master:       true,
		Status:       "active",
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	}

	if err := s.UpsertSession(sess); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	sessions, err := s.ListSessions("")
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	got := sessions[0]
	if got.ID != sess.ID {
		t.Errorf("ID: got %q, want %q", got.ID, sess.ID)
	}
	if got.Name != sess.Name {
		t.Errorf("Name: got %q, want %q", got.Name, sess.Name)
	}
	if got.Path != sess.Path {
		t.Errorf("Path: got %q, want %q", got.Path, sess.Path)
	}
	if got.Model != sess.Model {
		t.Errorf("Model: got %q, want %q", got.Model, sess.Model)
	}
	if !got.Master {
		t.Error("Master: got false, want true")
	}
	if got.Status != sess.Status {
		t.Errorf("Status: got %q, want %q", got.Status, sess.Status)
	}
}

func TestStorage_UpsertSession_Update(t *testing.T) {
	s := newTestStorage(t)

	sess := Session{
		ID: "sess-2", Name: "original", Path: "/tmp", Status: "active",
		CreatedAt: time.Now(), LastActiveAt: time.Now(),
	}
	if err := s.UpsertSession(sess); err != nil {
		t.Fatalf("UpsertSession initial: %v", err)
	}

	// Update name.
	sess.Name = "updated"
	if err := s.UpsertSession(sess); err != nil {
		t.Fatalf("UpsertSession update: %v", err)
	}

	sessions, err := s.ListSessions("")
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session after upsert, got %d", len(sessions))
	}
	if sessions[0].Name != "updated" {
		t.Errorf("Name after update: got %q, want %q", sessions[0].Name, "updated")
	}
}

func TestStorage_InsertMessage(t *testing.T) {
	s := newTestStorage(t)

	// Need a session first (FK constraint).
	sess := Session{
		ID: "sess-3", Name: "s", Path: "/tmp", Status: "active",
		CreatedAt: time.Now(), LastActiveAt: time.Now(),
	}
	if err := s.UpsertSession(sess); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	msg := Message{
		ID:        "msg-1",
		SessionID: "sess-3",
		Role:      "assistant",
		Content:   "hello world",
		AgentName: "agent-a",
		CreatedAt: time.Now(),
	}
	if err := s.InsertMessage(msg); err != nil {
		t.Fatalf("InsertMessage: %v", err)
	}

	msgs, err := s.ListMessages("sess-3", 10)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	got := msgs[0]
	if got.ID != msg.ID {
		t.Errorf("ID: got %q, want %q", got.ID, msg.ID)
	}
	if got.Role != msg.Role {
		t.Errorf("Role: got %q, want %q", got.Role, msg.Role)
	}
	if got.Content != msg.Content {
		t.Errorf("Content: got %q, want %q", got.Content, msg.Content)
	}
	if got.AgentName != msg.AgentName {
		t.Errorf("AgentName: got %q, want %q", got.AgentName, msg.AgentName)
	}
}

func TestStorage_ListMessages_Empty(t *testing.T) {
	s := newTestStorage(t)

	sess := Session{
		ID: "sess-4", Name: "s", Path: "/tmp", Status: "active",
		CreatedAt: time.Now(), LastActiveAt: time.Now(),
	}
	if err := s.UpsertSession(sess); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	msgs, err := s.ListMessages("sess-4", 10)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages, got %d", len(msgs))
	}
}

func TestStorage_SetSessionStatus(t *testing.T) {
	s := newTestStorage(t)

	sess := Session{
		ID: "sess-5", Name: "s", Path: "/tmp", Status: "active",
		CreatedAt: time.Now(), LastActiveAt: time.Now(),
	}
	if err := s.UpsertSession(sess); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	if err := s.SetSessionStatus("sess-5", "inactive"); err != nil {
		t.Fatalf("SetSessionStatus: %v", err)
	}

	got, err := s.GetSession("sess-5")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.Status != "inactive" {
		t.Errorf("Status after update: got %q, want %q", got.Status, "inactive")
	}
}

func TestStorage_GetSession_NotFound(t *testing.T) {
	s := newTestStorage(t)
	_, err := s.GetSession("does-not-exist")
	if err == nil {
		t.Error("expected error for non-existent session, got nil")
	}
}

func TestUnreadCount_ZeroWhenNoneExist(t *testing.T) {
	s := newTestStorage(t)

	sess := Session{
		ID: "sess-unread-1", Name: "s", Path: "/tmp", Status: "active",
		CreatedAt: time.Now(), LastActiveAt: time.Now(),
	}
	if err := s.UpsertSession(sess); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	count, err := s.UnreadCount("sess-unread-1")
	if err != nil {
		t.Fatalf("UnreadCount: %v", err)
	}
	if count != 0 {
		t.Errorf("UnreadCount: got %d, want 0", count)
	}
}

func TestUnreadCount_CountsMessagesAfterLastRead(t *testing.T) {
	s := newTestStorage(t)

	sess := Session{
		ID: "sess-unread-2", Name: "s", Path: "/tmp", Status: "active",
		CreatedAt: time.Now(), LastActiveAt: time.Now(),
	}
	if err := s.UpsertSession(sess); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	// Insert 3 messages at different times.
	now := time.Now()
	msg1 := Message{
		ID: "msg-1", SessionID: "sess-unread-2", Role: "user", Content: "msg1",
		CreatedAt: now.Add(-3 * time.Hour),
	}
	msg2 := Message{
		ID: "msg-2", SessionID: "sess-unread-2", Role: "assistant", Content: "msg2",
		CreatedAt: now.Add(-2 * time.Hour),
	}
	msg3 := Message{
		ID: "msg-3", SessionID: "sess-unread-2", Role: "user", Content: "msg3",
		CreatedAt: now.Add(-1 * time.Hour),
	}
	for _, m := range []Message{msg1, msg2, msg3} {
		if err := s.InsertMessage(m); err != nil {
			t.Fatalf("InsertMessage: %v", err)
		}
	}

	// No last_read_at yet; all 3 should be unread.
	count, err := s.UnreadCount("sess-unread-2")
	if err != nil {
		t.Fatalf("UnreadCount before MarkRead: %v", err)
	}
	if count != 3 {
		t.Errorf("UnreadCount before MarkRead: got %d, want 3", count)
	}

	// Mark as read at 1.5 hours ago.
	// Manually set last_read_at to between msg2 and msg3.
	_, err = s.db.Exec(
		`UPDATE cc_sessions SET last_read_at = ? WHERE id = ?`,
		now.Add(-90*time.Minute), "sess-unread-2",
	)
	if err != nil {
		t.Fatalf("manual MarkRead: %v", err)
	}

	// Only msg3 should be unread.
	count, err = s.UnreadCount("sess-unread-2")
	if err != nil {
		t.Fatalf("UnreadCount after MarkRead: %v", err)
	}
	if count != 1 {
		t.Errorf("UnreadCount after MarkRead: got %d, want 1", count)
	}
}

func TestStorage_ArchiveSession(t *testing.T) {
	s := newTestStorage(t)

	sess := Session{
		ID: "sess-archive-1", Name: "archive-me", Path: "/tmp", Status: "active",
		CreatedAt: time.Now(), LastActiveAt: time.Now(),
	}
	if err := s.UpsertSession(sess); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	// Before archive: visible in list.
	sessions, err := s.ListSessions("")
	if err != nil {
		t.Fatalf("ListSessions before archive: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session before archive, got %d", len(sessions))
	}

	if err := s.ArchiveSession(sess.ID); err != nil {
		t.Fatalf("ArchiveSession: %v", err)
	}

	// After archive: NOT in list.
	sessions, err = s.ListSessions("")
	if err != nil {
		t.Fatalf("ListSessions after archive: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions after archive, got %d", len(sessions))
	}

	// But still retrievable via GetSession with 'archived' status.
	got, err := s.GetSession(sess.ID)
	if err != nil {
		t.Fatalf("GetSession after archive: %v", err)
	}
	if got.Status != "archived" {
		t.Errorf("Status after archive: got %q, want %q", got.Status, "archived")
	}
}

func TestStorage_DeleteSession(t *testing.T) {
	s := newTestStorage(t)

	sess := Session{
		ID: "sess-delete-1", Name: "delete-me", Path: "/tmp", Status: "active",
		CreatedAt: time.Now(), LastActiveAt: time.Now(),
	}
	if err := s.UpsertSession(sess); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	// Insert some messages.
	for _, m := range []Message{
		{ID: "del-msg-0", SessionID: sess.ID, Role: "user", Content: "content 0", CreatedAt: time.Now()},
		{ID: "del-msg-1", SessionID: sess.ID, Role: "assistant", Content: "content 1", CreatedAt: time.Now()},
		{ID: "del-msg-2", SessionID: sess.ID, Role: "user", Content: "content 2", CreatedAt: time.Now()},
	} {
		if err := s.InsertMessage(m); err != nil {
			t.Fatalf("InsertMessage %s: %v", m.ID, err)
		}
	}

	// Before delete: session + messages exist.
	sessions, err := s.ListSessions("")
	if err != nil || len(sessions) != 1 {
		t.Fatalf("expected 1 session before delete, got %d (err=%v)", len(sessions), err)
	}
	msgs, err := s.ListMessages(sess.ID, 10)
	if err != nil || len(msgs) != 3 {
		t.Fatalf("expected 3 messages before delete, got %d (err=%v)", len(msgs), err)
	}

	if err := s.DeleteSession(sess.ID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	// After delete: session gone from list.
	sessions, err = s.ListSessions("")
	if err != nil {
		t.Fatalf("ListSessions after delete: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions after delete, got %d", len(sessions))
	}

	// GetSession returns error (not found).
	_, err = s.GetSession(sess.ID)
	if err == nil {
		t.Error("expected error from GetSession after delete, got nil")
	}

	// Messages also gone.
	msgs, err = s.ListMessages(sess.ID, 10)
	if err != nil {
		t.Fatalf("ListMessages after delete: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages after delete, got %d", len(msgs))
	}
}

func TestStorage_ListTasks(t *testing.T) {
	s := newTestStorage(t)

	sess := Session{
		ID: "sess-tasks-1", Name: "s", Path: "/tmp", Status: "active",
		CreatedAt: time.Now(), LastActiveAt: time.Now(),
	}
	if err := s.UpsertSession(sess); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	// Empty initially.
	tasks, err := s.ListTasks(sess.ID)
	if err != nil {
		t.Fatalf("ListTasks empty: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}

	// Insert tasks.
	now := time.Now()
	t1 := Task{
		ID: "task-1", SessionID: sess.ID, Title: "Fix bug",
		Status: "pending", AssignedTo: "agent-a",
		CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour),
	}
	t2 := Task{
		ID: "task-2", SessionID: sess.ID, Title: "Write tests",
		Status: "done", AssignedTo: "",
		CreatedAt: now.Add(-1 * time.Hour), UpdatedAt: now,
	}
	for _, tk := range []Task{t1, t2} {
		if err := s.UpsertTask(tk); err != nil {
			t.Fatalf("UpsertTask %s: %v", tk.ID, err)
		}
	}

	tasks, err = s.ListTasks(sess.ID)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
	// Newest first (task-2 created_at is later).
	if tasks[0].ID != "task-2" {
		t.Errorf("expected task-2 first, got %q", tasks[0].ID)
	}
	if tasks[0].Title != "Write tests" {
		t.Errorf("Title: got %q, want %q", tasks[0].Title, "Write tests")
	}
	if tasks[0].Status != "done" {
		t.Errorf("Status: got %q, want %q", tasks[0].Status, "done")
	}
}

func TestStorage_ListTasks_IsolatedBySession(t *testing.T) {
	s := newTestStorage(t)

	for _, id := range []string{"sess-a", "sess-b"} {
		if err := s.UpsertSession(Session{
			ID: id, Name: id, Path: "/tmp", Status: "active",
			CreatedAt: time.Now(), LastActiveAt: time.Now(),
		}); err != nil {
			t.Fatalf("UpsertSession %s: %v", id, err)
		}
	}

	if err := s.UpsertTask(Task{
		ID: "task-a", SessionID: "sess-a", Title: "Task A",
		Status: "pending", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("UpsertTask: %v", err)
	}

	tasks, err := s.ListTasks("sess-b")
	if err != nil {
		t.Fatalf("ListTasks sess-b: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks for sess-b, got %d", len(tasks))
	}
}

func TestStorage_ListAgents(t *testing.T) {
	s := newTestStorage(t)

	sess := Session{
		ID: "sess-agents-1", Name: "s", Path: "/tmp", Status: "active",
		CreatedAt: time.Now(), LastActiveAt: time.Now(),
	}
	if err := s.UpsertSession(sess); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	// Empty initially.
	agents, err := s.ListAgents(sess.ID)
	if err != nil {
		t.Fatalf("ListAgents empty: %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("expected 0 agents, got %d", len(agents))
	}

	// Insert agents.
	a1 := Agent{
		ID: "agent-1", SessionID: sess.ID, Name: "researcher",
		Status: "working", CurrentTaskID: "task-1", UpdatedAt: time.Now(),
	}
	a2 := Agent{
		ID: "agent-2", SessionID: sess.ID, Name: "coder",
		Status: "idle", CurrentTaskID: "", UpdatedAt: time.Now(),
	}
	for _, ag := range []Agent{a1, a2} {
		if err := s.UpsertAgent(ag); err != nil {
			t.Fatalf("UpsertAgent %s: %v", ag.ID, err)
		}
	}

	agents, err = s.ListAgents(sess.ID)
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(agents))
	}
	// Find agent-1 and verify fields.
	var found Agent
	for _, a := range agents {
		if a.ID == "agent-1" {
			found = a
			break
		}
	}
	if found.ID == "" {
		t.Fatal("agent-1 not found in ListAgents result")
	}
	if found.Name != "researcher" {
		t.Errorf("Name: got %q, want %q", found.Name, "researcher")
	}
	if found.Status != "working" {
		t.Errorf("Status: got %q, want %q", found.Status, "working")
	}
	if found.CurrentTaskID != "task-1" {
		t.Errorf("CurrentTaskID: got %q, want %q", found.CurrentTaskID, "task-1")
	}
}

func TestStorage_ListAgents_IsolatedBySession(t *testing.T) {
	s := newTestStorage(t)

	for _, id := range []string{"sess-c", "sess-d"} {
		if err := s.UpsertSession(Session{
			ID: id, Name: id, Path: "/tmp", Status: "active",
			CreatedAt: time.Now(), LastActiveAt: time.Now(),
		}); err != nil {
			t.Fatalf("UpsertSession %s: %v", id, err)
		}
	}

	if err := s.UpsertAgent(Agent{
		ID: "agent-x", SessionID: "sess-c", Name: "agent-x",
		Status: "idle", UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("UpsertAgent: %v", err)
	}

	agents, err := s.ListAgents("sess-d")
	if err != nil {
		t.Fatalf("ListAgents sess-d: %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("expected 0 agents for sess-d, got %d", len(agents))
	}
}

func TestStorage_ListSessions_ExcludesArchived(t *testing.T) {
	s := newTestStorage(t)

	active := Session{
		ID: "sess-active", Name: "active", Path: "/tmp", Status: "active",
		CreatedAt: time.Now(), LastActiveAt: time.Now(),
	}
	archived := Session{
		ID: "sess-arch", Name: "archived", Path: "/tmp", Status: "active",
		CreatedAt: time.Now(), LastActiveAt: time.Now(),
	}
	for _, sess := range []Session{active, archived} {
		if err := s.UpsertSession(sess); err != nil {
			t.Fatalf("UpsertSession %s: %v", sess.ID, err)
		}
	}
	if err := s.ArchiveSession(archived.ID); err != nil {
		t.Fatalf("ArchiveSession: %v", err)
	}

	sessions, err := s.ListSessions("")
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session (non-archived), got %d", len(sessions))
	}
	if sessions[0].ID != active.ID {
		t.Errorf("expected active session, got %q", sessions[0].ID)
	}
}

func TestStorage_ListSessions_Filter(t *testing.T) {
	s := newTestStorage(t)

	now := time.Now()
	for _, sess := range []Session{
		{ID: "f-active-1", Name: "a1", Path: "/tmp", Status: "active", CreatedAt: now, LastActiveAt: now},
		{ID: "f-active-2", Name: "a2", Path: "/tmp", Status: "active", CreatedAt: now, LastActiveAt: now},
		{ID: "f-inactive-1", Name: "i1", Path: "/tmp", Status: "inactive", CreatedAt: now, LastActiveAt: now},
	} {
		if err := s.UpsertSession(sess); err != nil {
			t.Fatalf("UpsertSession %s: %v", sess.ID, err)
		}
	}

	// filter="" → all 3 non-archived
	all, err := s.ListSessions("")
	if err != nil {
		t.Fatalf("ListSessions all: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("filter='': expected 3, got %d", len(all))
	}

	// filter="active" → 2
	active, err := s.ListSessions("active")
	if err != nil {
		t.Fatalf("ListSessions active: %v", err)
	}
	if len(active) != 2 {
		t.Errorf("filter='active': expected 2, got %d", len(active))
	}
	for _, sess := range active {
		if sess.Status != "active" {
			t.Errorf("filter='active': got session with status %q", sess.Status)
		}
	}

	// filter="inactive" → 1
	inactive, err := s.ListSessions("inactive")
	if err != nil {
		t.Fatalf("ListSessions inactive: %v", err)
	}
	if len(inactive) != 1 {
		t.Errorf("filter='inactive': expected 1, got %d", len(inactive))
	}
	if inactive[0].ID != "f-inactive-1" {
		t.Errorf("filter='inactive': expected f-inactive-1, got %q", inactive[0].ID)
	}
}

func TestMarkRead_ResetsUnreadCount(t *testing.T) {
	s := newTestStorage(t)

	sess := Session{
		ID: "sess-unread-3", Name: "s", Path: "/tmp", Status: "active",
		CreatedAt: time.Now(), LastActiveAt: time.Now(),
	}
	if err := s.UpsertSession(sess); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	// Insert a message.
	msg := Message{
		ID: "msg-1", SessionID: "sess-unread-3", Role: "user", Content: "hello",
		CreatedAt: time.Now(),
	}
	if err := s.InsertMessage(msg); err != nil {
		t.Fatalf("InsertMessage: %v", err)
	}

	// Should be 1 unread.
	count, err := s.UnreadCount("sess-unread-3")
	if err != nil {
		t.Fatalf("UnreadCount before MarkRead: %v", err)
	}
	if count != 1 {
		t.Errorf("UnreadCount before MarkRead: got %d, want 1", count)
	}

	// Mark as read.
	if err := s.MarkRead("sess-unread-3"); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}

	// Should now be 0 unread.
	count, err = s.UnreadCount("sess-unread-3")
	if err != nil {
		t.Fatalf("UnreadCount after MarkRead: %v", err)
	}
	if count != 0 {
		t.Errorf("UnreadCount after MarkRead: got %d, want 0", count)
	}
}
