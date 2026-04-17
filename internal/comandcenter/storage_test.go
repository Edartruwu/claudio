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

	sessions, err := s.ListSessions()
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

	sessions, err := s.ListSessions()
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
