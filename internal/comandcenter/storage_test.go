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
	// Create team_tasks (normally created by claudio migrations).
	if _, err := s.writeDB.Exec(`CREATE TABLE IF NOT EXISTS team_tasks (
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
	t.Cleanup(func() { s.Close() })
	return s
}

// seedTask inserts a task directly into team_tasks (claudio's native table).
func seedTask(t *testing.T, s *Storage, tk Task) {
	t.Helper()
	_, err := s.writeDB.Exec(`
		INSERT INTO team_tasks (id, session_id, title, description, status, assigned_to, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		tk.ID, tk.SessionID, tk.Title, tk.Description, tk.Status, tk.AssignedTo, tk.CreatedAt, tk.UpdatedAt,
	)
	if err != nil {
		t.Fatalf("seedTask %s: %v", tk.ID, err)
	}
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
	_, err = s.writeDB.Exec(
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
		seedTask(t, s, tk)
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

	seedTask(t, s, Task{
		ID: "task-a", SessionID: "sess-a", Title: "Task A",
		Status: "pending", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	tasks, err := s.ListTasks("sess-b")
	if err != nil {
		t.Fatalf("ListTasks sess-b: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks for sess-b, got %d", len(tasks))
	}
}

func TestStorage_GetTask(t *testing.T) {
	s := newTestStorage(t)

	sess := Session{
		ID: "sess-gettask-1", Name: "s", Path: "/tmp", Status: "active",
		CreatedAt: time.Now(), LastActiveAt: time.Now(),
	}
	if err := s.UpsertSession(sess); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	task := Task{
		ID: "task-get-1", SessionID: sess.ID, Title: "Get me",
		Description: "**bold** description", Status: "pending",
		AssignedTo: "agent-x",
		CreatedAt:  time.Now(), UpdatedAt: time.Now(),
	}
	seedTask(t, s, task)

	got, err := s.GetTask("task-get-1", sess.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.ID != task.ID {
		t.Errorf("ID: got %q, want %q", got.ID, task.ID)
	}
	if got.Title != task.Title {
		t.Errorf("Title: got %q, want %q", got.Title, task.Title)
	}
	if got.Description != task.Description {
		t.Errorf("Description: got %q, want %q", got.Description, task.Description)
	}
	if got.Status != task.Status {
		t.Errorf("Status: got %q, want %q", got.Status, task.Status)
	}
	if got.AssignedTo != task.AssignedTo {
		t.Errorf("AssignedTo: got %q, want %q", got.AssignedTo, task.AssignedTo)
	}
}

func TestStorage_GetTask_NotFound(t *testing.T) {
	s := newTestStorage(t)
	_, err := s.GetTask("does-not-exist", "session-1")
	if err == nil {
		t.Error("expected error for non-existent task, got nil")
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

func TestStorage_SavePushSubscription(t *testing.T) {
	s := newTestStorage(t)

	sub := PushSubscription{
		ID:        "sub-1",
		Endpoint:  "https://push.example.com/sub1",
		P256dh:    "dGVzdC1wMjU2ZGg=",
		Auth:      "dGVzdC1hdXRo",
		CreatedAt: time.Now(),
	}
	if err := s.SavePushSubscription(sub); err != nil {
		t.Fatalf("SavePushSubscription: %v", err)
	}

	subs, err := s.ListPushSubscriptions()
	if err != nil {
		t.Fatalf("ListPushSubscriptions: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(subs))
	}
	if subs[0].Endpoint != sub.Endpoint {
		t.Errorf("Endpoint: got %q, want %q", subs[0].Endpoint, sub.Endpoint)
	}
	if subs[0].P256dh != sub.P256dh {
		t.Errorf("P256dh: got %q, want %q", subs[0].P256dh, sub.P256dh)
	}
	if subs[0].Auth != sub.Auth {
		t.Errorf("Auth: got %q, want %q", subs[0].Auth, sub.Auth)
	}
}

func TestStorage_SavePushSubscription_Upsert(t *testing.T) {
	s := newTestStorage(t)

	sub := PushSubscription{
		ID: "sub-2", Endpoint: "https://push.example.com/sub2",
		P256dh: "old-key", Auth: "old-auth", CreatedAt: time.Now(),
	}
	if err := s.SavePushSubscription(sub); err != nil {
		t.Fatalf("SavePushSubscription initial: %v", err)
	}

	// Same endpoint, updated keys.
	sub.ID = "sub-2b"
	sub.P256dh = "new-key"
	sub.Auth = "new-auth"
	if err := s.SavePushSubscription(sub); err != nil {
		t.Fatalf("SavePushSubscription upsert: %v", err)
	}

	subs, err := s.ListPushSubscriptions()
	if err != nil {
		t.Fatalf("ListPushSubscriptions: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscription after upsert, got %d", len(subs))
	}
	if subs[0].P256dh != "new-key" {
		t.Errorf("P256dh after upsert: got %q, want %q", subs[0].P256dh, "new-key")
	}
}

func TestStorage_DeletePushSubscription(t *testing.T) {
	s := newTestStorage(t)

	sub := PushSubscription{
		ID: "sub-3", Endpoint: "https://push.example.com/sub3",
		P256dh: "k", Auth: "a", CreatedAt: time.Now(),
	}
	if err := s.SavePushSubscription(sub); err != nil {
		t.Fatalf("SavePushSubscription: %v", err)
	}

	if err := s.DeletePushSubscription(sub.Endpoint); err != nil {
		t.Fatalf("DeletePushSubscription: %v", err)
	}

	subs, err := s.ListPushSubscriptions()
	if err != nil {
		t.Fatalf("ListPushSubscriptions: %v", err)
	}
	if len(subs) != 0 {
		t.Fatalf("expected 0 subscriptions after delete, got %d", len(subs))
	}
}

func TestStorage_GetOrCreateVAPIDKeys(t *testing.T) {
	s := newTestStorage(t)

	pub1, priv1, err := s.GetOrCreateVAPIDKeys()
	if err != nil {
		t.Fatalf("GetOrCreateVAPIDKeys first call: %v", err)
	}
	if pub1 == "" || priv1 == "" {
		t.Fatal("expected non-empty VAPID keys")
	}

	// Second call must return same keys.
	pub2, priv2, err := s.GetOrCreateVAPIDKeys()
	if err != nil {
		t.Fatalf("GetOrCreateVAPIDKeys second call: %v", err)
	}
	if pub2 != pub1 {
		t.Errorf("public key changed: got %q, want %q", pub2, pub1)
	}
	if priv2 != priv1 {
		t.Errorf("private key changed: got %q, want %q", priv2, priv1)
	}
}

func TestStorage_DeleteMessages(t *testing.T) {
	s := newTestStorage(t)

	sess := Session{
		ID: "sess-delmsg-1", Name: "s", Path: "/tmp", Status: "active",
		CreatedAt: time.Now(), LastActiveAt: time.Now(),
	}
	if err := s.UpsertSession(sess); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	for _, m := range []Message{
		{ID: "dm-1", SessionID: sess.ID, Role: "user", Content: "one", CreatedAt: time.Now()},
		{ID: "dm-2", SessionID: sess.ID, Role: "assistant", Content: "two", CreatedAt: time.Now()},
		{ID: "dm-3", SessionID: sess.ID, Role: "user", Content: "three", CreatedAt: time.Now()},
	} {
		if err := s.InsertMessage(m); err != nil {
			t.Fatalf("InsertMessage %s: %v", m.ID, err)
		}
	}

	if err := s.DeleteMessages(sess.ID); err != nil {
		t.Fatalf("DeleteMessages: %v", err)
	}

	msgs, err := s.ListMessages(sess.ID, 100)
	if err != nil {
		t.Fatalf("ListMessages after delete: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages after DeleteMessages, got %d", len(msgs))
	}
}

func TestStorage_ListMessages_ReplyAndQuotedFields(t *testing.T) {
	s := newTestStorage(t)

	sess := Session{
		ID: "sess-reply-1", Name: "s", Path: "/tmp", Status: "active",
		CreatedAt: time.Now(), LastActiveAt: time.Now(),
	}
	if err := s.UpsertSession(sess); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	msg := Message{
		ID:             "reply-msg-1",
		SessionID:      sess.ID,
		Role:           "user",
		Content:        "original content",
		CreatedAt:      time.Now(),
		ReplyToSession: "other-sess",
		QuotedContent:  "quoted text",
	}
	if err := s.InsertMessage(msg); err != nil {
		t.Fatalf("InsertMessage: %v", err)
	}

	msgs, err := s.ListMessages(sess.ID, 10)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	got := msgs[0]
	if got.ReplyToSession != "other-sess" {
		t.Errorf("ReplyToSession: got %q, want %q", got.ReplyToSession, "other-sess")
	}
	if got.QuotedContent != "quoted text" {
		t.Errorf("QuotedContent: got %q, want %q", got.QuotedContent, "quoted text")
	}
}

const createNativeSessionsSQL = `CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	path TEXT NOT NULL DEFAULT '',
	model TEXT NOT NULL DEFAULT '',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	last_active_at DATETIME DEFAULT CURRENT_TIMESTAMP
)`

const createNativeMessagesSQL = `CREATE TABLE IF NOT EXISTS messages (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	session_id TEXT NOT NULL,
	role TEXT NOT NULL,
	content TEXT NOT NULL,
	type TEXT NOT NULL DEFAULT 'text',
	tool_use_id TEXT DEFAULT '',
	tool_name TEXT DEFAULT '',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
)`

func TestStorage_DeleteNativeMessages(t *testing.T) {
	s := newTestStorage(t)

	const sid = "native-del-1"

	// Create native tables.
	if err := s.ExecRaw(createNativeSessionsSQL); err != nil {
		t.Fatalf("create sessions table: %v", err)
	}
	if err := s.ExecRaw(createNativeMessagesSQL); err != nil {
		t.Fatalf("create messages table: %v", err)
	}

	// Seed session row (FK requirement).
	if err := s.ExecRaw(`INSERT INTO sessions (id, name) VALUES (?, ?)`, sid, "test"); err != nil {
		t.Fatalf("insert session: %v", err)
	}

	// Insert 3 messages without specifying id (autoincrement).
	for _, content := range []string{"alpha", "beta", "gamma"} {
		if err := s.ExecRaw(
			`INSERT INTO messages (session_id, role, content) VALUES (?, ?, ?)`,
			sid, "user", content,
		); err != nil {
			t.Fatalf("insert message %q: %v", content, err)
		}
	}

	if err := s.DeleteNativeMessages(sid); err != nil {
		t.Fatalf("DeleteNativeMessages: %v", err)
	}

	msgs, err := s.GetNativeMessages(sid, 100)
	if err != nil {
		t.Fatalf("GetNativeMessages after delete: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 native messages after delete, got %d", len(msgs))
	}
}

func TestStorage_InsertNativeMessage(t *testing.T) {
	s := newTestStorage(t)

	const sid = "native-ins-1"

	// Create native tables.
	if err := s.ExecRaw(createNativeSessionsSQL); err != nil {
		t.Fatalf("create sessions table: %v", err)
	}
	if err := s.ExecRaw(createNativeMessagesSQL); err != nil {
		t.Fatalf("create messages table: %v", err)
	}

	// Seed session row.
	if err := s.ExecRaw(`INSERT INTO sessions (id, name) VALUES (?, ?)`, sid, "test"); err != nil {
		t.Fatalf("insert session: %v", err)
	}

	if err := s.InsertNativeMessage(sid, "assistant", "hello world", time.Now()); err != nil {
		t.Fatalf("InsertNativeMessage: %v", err)
	}

	msgs, err := s.GetNativeMessages(sid, 10)
	if err != nil {
		t.Fatalf("GetNativeMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 native message, got %d", len(msgs))
	}
	if msgs[0].Role != "assistant" {
		t.Errorf("Role: got %q, want %q", msgs[0].Role, "assistant")
	}
	if msgs[0].Content != "hello world" {
		t.Errorf("Content: got %q, want %q", msgs[0].Content, "hello world")
	}
}

func TestMigration_Idempotent(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	sess := Session{
		ID: "sess-mig-1", Name: "s", Path: "/tmp", Status: "active",
		CreatedAt: time.Now(), LastActiveAt: time.Now(),
	}
	if err := s.UpsertSession(sess); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	msg := Message{
		ID:             "mig-msg-1",
		SessionID:      sess.ID,
		Role:           "user",
		Content:        "migration test",
		CreatedAt:      time.Now(),
		ReplyToSession: "some-session",
		QuotedContent:  "some quoted text",
	}
	if err := s.InsertMessage(msg); err != nil {
		t.Fatalf("InsertMessage: %v", err)
	}

	msgs, err := s.ListMessages(sess.ID, 10)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	got := msgs[0]
	if got.ReplyToSession != "some-session" {
		t.Errorf("ReplyToSession: got %q, want %q", got.ReplyToSession, "some-session")
	}
	if got.QuotedContent != "some quoted text" {
		t.Errorf("QuotedContent: got %q, want %q", got.QuotedContent, "some quoted text")
	}
}

// TestStorage_InsertAndGetAgentEvents verifies that InsertAgentEvent persists an event
// and GetLatestAgentEvents retrieves it with all fields intact.
func TestStorage_InsertAndGetAgentEvents(t *testing.T) {
	s := newTestStorage(t)

	const sessionID = "sess-agent-evt"
	const agentName = "test-agent"
	const status = "done"
	const payload = `{"name":"test-agent","status":"done","result":"task complete"}`

	if err := s.InsertAgentEvent(sessionID, agentName, status, payload); err != nil {
		t.Fatalf("InsertAgentEvent: %v", err)
	}

	events, err := s.GetLatestAgentEvents(sessionID)
	if err != nil {
		t.Fatalf("GetLatestAgentEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	e := events[0]
	if e.SessionID != sessionID {
		t.Errorf("SessionID = %q, want %q", e.SessionID, sessionID)
	}
	if e.AgentName != agentName {
		t.Errorf("AgentName = %q, want %q", e.AgentName, agentName)
	}
	if e.Status != status {
		t.Errorf("Status = %q, want %q", e.Status, status)
	}
	if e.Payload != payload {
		t.Errorf("Payload = %q, want %q", e.Payload, payload)
	}
}

// TestStorage_InsertAndGetAgentEvents_CrossSession verifies that GetLatestAgentEvents
// only returns events belonging to the queried session, not events from other sessions.
func TestStorage_InsertAndGetAgentEvents_CrossSession(t *testing.T) {
	s := newTestStorage(t)

	_ = s.InsertAgentEvent("sess-X", "agent-x", "done", `{"name":"agent-x","status":"done"}`)
	_ = s.InsertAgentEvent("sess-Y", "agent-y", "done", `{"name":"agent-y","status":"done"}`)

	events, err := s.GetLatestAgentEvents("sess-X")
	if err != nil {
		t.Fatalf("GetLatestAgentEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event for sess-X, got %d", len(events))
	}
	if events[0].AgentName != "agent-x" {
		t.Errorf("AgentName = %q, want %q", events[0].AgentName, "agent-x")
	}
}

// TestStorage_GetLatestAgentEvents_LatestPerAgent verifies that when multiple events
// exist for the same agent in a session, only the most recent is returned.
func TestStorage_GetLatestAgentEvents_LatestPerAgent(t *testing.T) {
	s := newTestStorage(t)

	const sess = "sess-multi-evt"
	_ = s.InsertAgentEvent(sess, "worker", "working", `{"name":"worker","status":"working"}`)
	_ = s.InsertAgentEvent(sess, "worker", "done", `{"name":"worker","status":"done","result":"ok"}`)

	events, err := s.GetLatestAgentEvents(sess)
	if err != nil {
		t.Fatalf("GetLatestAgentEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event (latest per agent), got %d", len(events))
	}
	if events[0].Status != "done" {
		t.Errorf("Status = %q, want %q", events[0].Status, "done")
	}
}

// TestStorage_PruneAgentEvents verifies that PruneAgentEvents removes events older
// than 15 minutes and leaves recent events intact.
func TestStorage_PruneAgentEvents(t *testing.T) {
	s := newTestStorage(t)

	const sess = "sess-prune"

	// Insert a recent event (within 15 min window — default CURRENT_TIMESTAMP).
	if err := s.InsertAgentEvent(sess, "fresh-agent", "done", `{"name":"fresh-agent","status":"done"}`); err != nil {
		t.Fatalf("InsertAgentEvent recent: %v", err)
	}

	// Insert a stale event by directly writing an old timestamp.
	_, err := s.writeDB.Exec(`
		INSERT INTO cc_agent_events (session_id, agent_name, status, payload, created_at)
		VALUES (?, ?, ?, ?, datetime('now', '-20 minutes'))
	`, sess, "stale-agent", "done", `{"name":"stale-agent","status":"done"}`)
	if err != nil {
		t.Fatalf("insert stale event: %v", err)
	}

	// Confirm 2 events exist before pruning.
	before, err := s.GetLatestAgentEvents(sess)
	if err != nil {
		t.Fatalf("GetLatestAgentEvents before prune: %v", err)
	}
	// Only the recent one appears in GetLatestAgentEvents (15-min filter).
	if len(before) != 1 {
		t.Fatalf("expected 1 recent event before prune, got %d", len(before))
	}

	// Prune stale events.
	if err := s.PruneAgentEvents(); err != nil {
		t.Fatalf("PruneAgentEvents: %v", err)
	}

	// Verify stale row is deleted from the raw table.
	var count int
	if err := s.writeDB.QueryRow(`SELECT COUNT(*) FROM cc_agent_events WHERE session_id = ?`, sess).Scan(&count); err != nil {
		t.Fatalf("count after prune: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 row after prune (stale deleted), got %d", count)
	}

	// Verify the remaining row is the fresh one.
	var name string
	if err := s.writeDB.QueryRow(`SELECT agent_name FROM cc_agent_events WHERE session_id = ?`, sess).Scan(&name); err != nil {
		t.Fatalf("query remaining row: %v", err)
	}
	if name != "fresh-agent" {
		t.Errorf("remaining agent_name = %q, want %q", name, "fresh-agent")
	}
}
