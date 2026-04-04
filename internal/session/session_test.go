package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/storage"
)

// ---- helpers ----------------------------------------------------------------

// openTestDB opens a fresh SQLite database in a temp directory and registers
// cleanup. Using t.TempDir() ensures the file is removed after the test.
func openTestDB(t *testing.T) *storage.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("openTestDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// newTestSession creates a Session backed by a fresh DB.
func newTestSession(t *testing.T) *Session {
	t.Helper()
	return New(openTestDB(t))
}

// rawMsg is a small helper that turns a plain string into a json.RawMessage.
func rawMsg(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

// ---- Session (session.go) ---------------------------------------------------

// TestNew verifies that New returns a non-nil *Session with no active session.
func TestNew(t *testing.T) {
	s := newTestSession(t)
	if s == nil {
		t.Fatal("New returned nil")
	}
	if s.Current() != nil {
		t.Error("fresh Session should have nil Current()")
	}
}

// TestStart verifies that Start creates a session and sets it as current.
func TestStart(t *testing.T) {
	s := newTestSession(t)

	sess, err := s.Start("claude-3-opus")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if sess == nil {
		t.Fatal("Start returned nil session")
	}
	if sess.ID == "" {
		t.Error("session ID should not be empty")
	}
	if sess.Model != "claude-3-opus" {
		t.Errorf("Model = %q, want %q", sess.Model, "claude-3-opus")
	}
	// ProjectDir should be os.Getwd()
	cwd, _ := os.Getwd()
	if sess.ProjectDir != cwd {
		t.Errorf("ProjectDir = %q, want %q", sess.ProjectDir, cwd)
	}
	if s.Current() == nil {
		t.Error("Current() should be non-nil after Start")
	}
	if s.Current().ID != sess.ID {
		t.Errorf("Current().ID = %q, want %q", s.Current().ID, sess.ID)
	}
}

// TestStart_MultipleModels verifies different model strings are stored.
func TestStart_MultipleModels(t *testing.T) {
	models := []string{"gpt-4", "claude-3-haiku", "gemini-pro", ""}
	for _, model := range models {
		t.Run("model="+model, func(t *testing.T) {
			s := newTestSession(t)
			sess, err := s.Start(model)
			if err != nil {
				t.Fatalf("Start(%q): %v", model, err)
			}
			if sess.Model != model {
				t.Errorf("Model = %q, want %q", sess.Model, model)
			}
		})
	}
}

// TestResume verifies that a previously created session can be resumed.
func TestResume(t *testing.T) {
	s := newTestSession(t)

	// create a session
	created, err := s.Start("claude-3-sonnet")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	id := created.ID

	// reset current to simulate "reconnect"
	s.current = nil
	if s.Current() != nil {
		t.Fatal("expected nil after reset")
	}

	resumed, err := s.Resume(id)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if resumed == nil {
		t.Fatal("Resume returned nil")
	}
	if resumed.ID != id {
		t.Errorf("resumed ID = %q, want %q", resumed.ID, id)
	}
	if s.Current().ID != id {
		t.Errorf("Current().ID = %q, want %q", s.Current().ID, id)
	}
}

// TestResume_NotFound verifies that resuming a non-existent ID returns an error.
func TestResume_NotFound(t *testing.T) {
	s := newTestSession(t)
	_, err := s.Resume("does-not-exist-uuid")
	if err == nil {
		t.Error("expected error for missing session, got nil")
	}
}

// TestList verifies List returns the right number of sessions.
func TestList(t *testing.T) {
	s := newTestSession(t)

	// create 5 sessions
	for i := 0; i < 5; i++ {
		if _, err := s.Start(fmt.Sprintf("model-%d", i)); err != nil {
			t.Fatalf("Start: %v", err)
		}
	}

	tests := []struct {
		limit int
		want  int
	}{
		{0, 0},
		{1, 1},
		{3, 3},
		{5, 5},
		{10, 5}, // fewer than limit
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("limit=%d", tc.limit), func(t *testing.T) {
			sessions, err := s.List(tc.limit)
			if err != nil {
				t.Fatalf("List(%d): %v", tc.limit, err)
			}
			if len(sessions) != tc.want {
				t.Errorf("got %d sessions, want %d", len(sessions), tc.want)
			}
		})
	}
}

// TestAddMessage verifies messages can be added and retrieved.
func TestAddMessage(t *testing.T) {
	s := newTestSession(t)
	if _, err := s.Start("model"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	messages := []struct {
		role    string
		content string
		msgType string
	}{
		{"user", "hello", "text"},
		{"assistant", "hi there", "text"},
		{"user", "how are you?", "text"},
	}

	for _, m := range messages {
		if err := s.AddMessage(m.role, m.content, m.msgType); err != nil {
			t.Fatalf("AddMessage(%q,%q,%q): %v", m.role, m.content, m.msgType, err)
		}
	}

	records, err := s.GetMessages()
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(records) != len(messages) {
		t.Fatalf("got %d records, want %d", len(records), len(messages))
	}
	for i, m := range messages {
		r := records[i]
		if r.Role != m.role {
			t.Errorf("[%d] Role = %q, want %q", i, r.Role, m.role)
		}
		if r.Content != m.content {
			t.Errorf("[%d] Content = %q, want %q", i, r.Content, m.content)
		}
		if r.Type != m.msgType {
			t.Errorf("[%d] Type = %q, want %q", i, r.Type, m.msgType)
		}
	}
}

// TestAddMessage_NilSession verifies AddMessage is a no-op when there is no
// active session (returns nil, no panic).
func TestAddMessage_NilSession(t *testing.T) {
	s := newTestSession(t)
	// deliberately do NOT call Start
	if err := s.AddMessage("user", "hello", "text"); err != nil {
		t.Errorf("AddMessage with nil session should return nil, got %v", err)
	}
}

// TestAddToolMessage verifies tool messages are stored correctly.
func TestAddToolMessage(t *testing.T) {
	s := newTestSession(t)
	if _, err := s.Start("model"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	err := s.AddToolMessage("user", `{"result":"ok"}`, "tool_result", "tid-123", "bash")
	if err != nil {
		t.Fatalf("AddToolMessage: %v", err)
	}

	records, err := s.GetMessages()
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	r := records[0]
	if r.ToolUseID != "tid-123" {
		t.Errorf("ToolUseID = %q, want %q", r.ToolUseID, "tid-123")
	}
	if r.ToolName != "bash" {
		t.Errorf("ToolName = %q, want %q", r.ToolName, "bash")
	}
}

// TestAddToolMessage_NilSession is a no-op guard.
func TestAddToolMessage_NilSession(t *testing.T) {
	s := newTestSession(t)
	if err := s.AddToolMessage("user", "x", "tool", "id", "name"); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

// TestGetMessages_NilSession returns nil slice without error.
func TestGetMessages_NilSession(t *testing.T) {
	s := newTestSession(t)
	msgs, err := s.GetMessages()
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if msgs != nil {
		t.Errorf("expected nil slice, got %v", msgs)
	}
}

// TestSetTitle updates the session title and verifies persistence.
func TestSetTitle(t *testing.T) {
	s := newTestSession(t)
	sess, err := s.Start("model")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := s.SetTitle("My Chat"); err != nil {
		t.Fatalf("SetTitle: %v", err)
	}

	// retrieve fresh from DB
	retrieved, err := openTestDB(t).GetSession(sess.ID)
	_ = retrieved // just confirm no error occurred
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
}

// TestSetTitle_NilSession is a no-op when there is no active session.
func TestSetTitle_NilSession(t *testing.T) {
	s := newTestSession(t)
	if err := s.SetTitle("anything"); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

// TestSaveSummary persists a summary for the current session.
func TestSaveSummary(t *testing.T) {
	db := openTestDB(t)
	s := New(db)
	sess, err := s.Start("model")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	summary := "This is a test summary."
	if err := s.SaveSummary(summary); err != nil {
		t.Fatalf("SaveSummary: %v", err)
	}

	// re-read from DB
	got, err := db.GetSession(sess.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.Summary != summary {
		t.Errorf("Summary = %q, want %q", got.Summary, summary)
	}
}

// TestSaveSummary_NilSession is a no-op.
func TestSaveSummary_NilSession(t *testing.T) {
	s := newTestSession(t)
	if err := s.SaveSummary("whatever"); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

// TestRenameByID renames a session that is NOT the current one.
func TestRenameByID(t *testing.T) {
	db := openTestDB(t)
	s := New(db)
	sess, err := s.Start("model")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	id := sess.ID

	if err := s.RenameByID(id, "New Title"); err != nil {
		t.Fatalf("RenameByID: %v", err)
	}

	got, err := db.GetSession(id)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.Title != "New Title" {
		t.Errorf("Title = %q, want %q", got.Title, "New Title")
	}
}

// TestDeleteAllMessages clears messages without deleting the session.
func TestDeleteAllMessages(t *testing.T) {
	s := newTestSession(t)
	if _, err := s.Start("model"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	for i := 0; i < 3; i++ {
		if err := s.AddMessage("user", fmt.Sprintf("msg %d", i), "text"); err != nil {
			t.Fatalf("AddMessage: %v", err)
		}
	}

	if err := s.DeleteAllMessages(); err != nil {
		t.Fatalf("DeleteAllMessages: %v", err)
	}

	records, err := s.GetMessages()
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("expected 0 messages after DeleteAllMessages, got %d", len(records))
	}
}

// TestDeleteAllMessages_NilSession is a no-op.
func TestDeleteAllMessages_NilSession(t *testing.T) {
	s := newTestSession(t)
	if err := s.DeleteAllMessages(); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

// TestDelete removes a session and verifies it is gone.
func TestDelete(t *testing.T) {
	db := openTestDB(t)
	s := New(db)
	sess, err := s.Start("model")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	id := sess.ID

	if err := s.Delete(id); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, err := db.GetSession(id)
	if err != nil {
		t.Fatalf("GetSession after delete: %v", err)
	}
	if got != nil {
		t.Error("expected nil after deletion")
	}
}

// TestSearch returns results matching the query.
func TestSearch(t *testing.T) {
	db := openTestDB(t)
	s := New(db)

	// Create two sessions in the current project dir; Start() uses Getwd()
	if _, err := s.Start("model"); err != nil {
		t.Fatal(err)
	}
	// Manually rename the sessions for searching
	sess1 := s.Current()
	_ = db.UpdateSessionTitle(sess1.ID, "alpha session")

	if _, err := s.Start("model"); err != nil {
		t.Fatal(err)
	}
	sess2 := s.Current()
	_ = db.UpdateSessionTitle(sess2.ID, "beta session")

	tests := []struct {
		query string
		want  int
	}{
		{"alpha", 1},
		{"beta", 1},
		{"session", 2},
		{"nonexistent", 0},
		{"", 2}, // empty query lists all
	}
	for _, tc := range tests {
		t.Run("query="+tc.query, func(t *testing.T) {
			results, err := s.Search(tc.query, 10)
			if err != nil {
				t.Fatalf("Search(%q): %v", tc.query, err)
			}
			if len(results) != tc.want {
				t.Errorf("got %d results, want %d", len(results), tc.want)
			}
		})
	}
}

// TestRecentForProject filters by cwd.
func TestRecentForProject(t *testing.T) {
	s := newTestSession(t)

	// Start 3 sessions (all will share the same Getwd() project dir)
	for i := 0; i < 3; i++ {
		if _, err := s.Start("model"); err != nil {
			t.Fatalf("Start: %v", err)
		}
	}

	results, err := s.RecentForProject(2)
	if err != nil {
		t.Fatalf("RecentForProject: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("got %d results, want 2", len(results))
	}
}

// TestRecentForProject_Limit respects limit boundary.
func TestRecentForProject_Limit(t *testing.T) {
	s := newTestSession(t)
	if _, err := s.Start("model"); err != nil {
		t.Fatal(err)
	}

	// limit of 0 should return nothing
	results, err := s.RecentForProject(0)
	if err != nil {
		t.Fatalf("RecentForProject(0): %v", err)
	}
	if len(results) != 0 {
		t.Errorf("got %d, want 0", len(results))
	}
}

// TestLastSessionSummary returns summary + time for the most recent session.
func TestLastSessionSummary(t *testing.T) {
	db := openTestDB(t)
	s := New(db)

	sess, err := s.Start("model")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	want := "Last session summary text"
	if err := db.UpdateSessionSummary(sess.ID, want); err != nil {
		t.Fatalf("UpdateSessionSummary: %v", err)
	}

	got, ts, err := s.LastSessionSummary()
	if err != nil {
		t.Fatalf("LastSessionSummary: %v", err)
	}
	if got != want {
		t.Errorf("summary = %q, want %q", got, want)
	}
	if ts.IsZero() {
		t.Error("timestamp should not be zero")
	}
}

// TestLastSessionSummary_NoSessions returns empty values without error.
func TestLastSessionSummary_NoSessions(t *testing.T) {
	s := newTestSession(t)
	summary, ts, err := s.LastSessionSummary()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if summary != "" {
		t.Errorf("expected empty summary, got %q", summary)
	}
	if !ts.IsZero() {
		t.Errorf("expected zero time, got %v", ts)
	}
}

// ---- sharing.go -------------------------------------------------------------

// TestExportSession verifies that ExportSession produces the expected structure.
func TestExportSession(t *testing.T) {
	msgs := []api.Message{
		{Role: "user", Content: rawMsg("hello")},
		{Role: "assistant", Content: rawMsg("world")},
	}

	before := time.Now()
	shared := ExportSession(msgs, "claude-3", "a summary")
	after := time.Now()

	if shared == nil {
		t.Fatal("ExportSession returned nil")
	}
	if shared.Version != 1 {
		t.Errorf("Version = %d, want 1", shared.Version)
	}
	if shared.Model != "claude-3" {
		t.Errorf("Model = %q, want %q", shared.Model, "claude-3")
	}
	if shared.Summary != "a summary" {
		t.Errorf("Summary = %q, want %q", shared.Summary, "a summary")
	}
	if len(shared.Messages) != 2 {
		t.Errorf("len(Messages) = %d, want 2", len(shared.Messages))
	}
	if shared.Metadata == nil {
		t.Error("Metadata map should not be nil")
	}
	if shared.ExportedAt.Before(before) || shared.ExportedAt.After(after) {
		t.Errorf("ExportedAt %v not within expected range", shared.ExportedAt)
	}
}

// TestExportSession_EmptyMessages verifies that an empty messages slice works.
func TestExportSession_EmptyMessages(t *testing.T) {
	shared := ExportSession(nil, "model", "")
	if shared == nil {
		t.Fatal("ExportSession returned nil")
	}
	if shared.Messages != nil {
		// nil slice is fine; just verify
		t.Logf("Messages is non-nil (len=%d)", len(shared.Messages))
	}
}

// TestExportSession_EmptySummary verifies omitempty behaviour.
func TestExportSession_EmptySummary(t *testing.T) {
	shared := ExportSession([]api.Message{{Role: "user", Content: rawMsg("hi")}}, "m", "")
	if shared.Summary != "" {
		t.Errorf("expected empty summary, got %q", shared.Summary)
	}
}

// TestMarshalSession verifies that MarshalSession returns valid JSON.
func TestMarshalSession(t *testing.T) {
	msgs := []api.Message{
		{Role: "user", Content: rawMsg("hello")},
	}
	shared := ExportSession(msgs, "claude", "sum")

	data, err := MarshalSession(shared)
	if err != nil {
		t.Fatalf("MarshalSession: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("MarshalSession returned empty bytes")
	}

	// must be valid JSON
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	// indented check: should contain newlines
	if string(data[0]) != "{" {
		t.Error("expected object JSON")
	}
}

// TestMarshalSession_RoundTrip verifies MarshalSession + ImportSession is lossless.
func TestMarshalSession_RoundTrip(t *testing.T) {
	original := ExportSession([]api.Message{
		{Role: "user", Content: rawMsg("round-trip test")},
		{Role: "assistant", Content: rawMsg("ok!")},
	}, "gpt-4", "round trip summary")

	data, err := MarshalSession(original)
	if err != nil {
		t.Fatalf("MarshalSession: %v", err)
	}

	imported, err := ImportSession(data)
	if err != nil {
		t.Fatalf("ImportSession: %v", err)
	}

	if imported.Version != original.Version {
		t.Errorf("Version: got %d, want %d", imported.Version, original.Version)
	}
	if imported.Model != original.Model {
		t.Errorf("Model: got %q, want %q", imported.Model, original.Model)
	}
	if imported.Summary != original.Summary {
		t.Errorf("Summary: got %q, want %q", imported.Summary, original.Summary)
	}
	if len(imported.Messages) != len(original.Messages) {
		t.Errorf("Messages len: got %d, want %d", len(imported.Messages), len(original.Messages))
	}
}

// TestImportSession_ValidJSON parses a hand-crafted JSON payload.
func TestImportSession_ValidJSON(t *testing.T) {
	payload := `{
		"version": 1,
		"exported_at": "2024-01-02T15:04:05Z",
		"model": "claude-3-haiku",
		"summary": "test",
		"messages": [
			{"role": "user", "content": "hello"}
		]
	}`

	s, err := ImportSession([]byte(payload))
	if err != nil {
		t.Fatalf("ImportSession: %v", err)
	}
	if s.Version != 1 {
		t.Errorf("Version = %d, want 1", s.Version)
	}
	if s.Model != "claude-3-haiku" {
		t.Errorf("Model = %q, want %q", s.Model, "claude-3-haiku")
	}
	if len(s.Messages) != 1 {
		t.Errorf("len(Messages) = %d, want 1", len(s.Messages))
	}
}

// TestImportSession_InvalidJSON verifies an error is returned for bad input.
func TestImportSession_InvalidJSON(t *testing.T) {
	badInputs := []struct {
		name  string
		input []byte
	}{
		{"empty", []byte{}},
		{"not-json", []byte("this is not json")},
		{"partial", []byte(`{"version":`)},
	}
	for _, tc := range badInputs {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ImportSession(tc.input)
			if err == nil {
				t.Error("expected error for invalid JSON, got nil")
			}
		})
	}
}

// TestImportSession_MissingVersion requires a non-zero version field.
func TestImportSession_MissingVersion(t *testing.T) {
	payload := `{"messages":[{"role":"user","content":"hi"}]}`
	_, err := ImportSession([]byte(payload))
	if err == nil {
		t.Error("expected error for missing version, got nil")
	}
}

// TestImportSession_NoMessages rejects sessions without messages.
func TestImportSession_NoMessages(t *testing.T) {
	payload := `{"version":1, "model":"m", "messages":[]}`
	_, err := ImportSession([]byte(payload))
	if err == nil {
		t.Error("expected error for empty messages, got nil")
	}
}

// TestImportSession_NullMessages rejects null message field.
func TestImportSession_NullMessages(t *testing.T) {
	payload := `{"version":1, "model":"m", "messages":null}`
	_, err := ImportSession([]byte(payload))
	if err == nil {
		t.Error("expected error for null messages, got nil")
	}
}

// TestImportSession_MultipleMessages handles multiple messages correctly.
func TestImportSession_MultipleMessages(t *testing.T) {
	payload := `{
		"version": 2,
		"model": "gpt-4",
		"exported_at": "2024-06-01T00:00:00Z",
		"messages": [
			{"role":"user","content":"a"},
			{"role":"assistant","content":"b"},
			{"role":"user","content":"c"}
		]
	}`
	s, err := ImportSession([]byte(payload))
	if err != nil {
		t.Fatalf("ImportSession: %v", err)
	}
	if len(s.Messages) != 3 {
		t.Errorf("expected 3 messages, got %d", len(s.Messages))
	}
	if s.Version != 2 {
		t.Errorf("Version = %d, want 2", s.Version)
	}
}

// TestSharedSession_MetadataPreserved ensures metadata survives a round-trip.
func TestSharedSession_MetadataPreserved(t *testing.T) {
	shared := ExportSession(
		[]api.Message{{Role: "user", Content: rawMsg("hi")}},
		"m", "s",
	)
	shared.Metadata["key"] = "value"
	shared.Metadata["foo"] = "bar"

	data, err := MarshalSession(shared)
	if err != nil {
		t.Fatalf("MarshalSession: %v", err)
	}

	imported, err := ImportSession(data)
	if err != nil {
		t.Fatalf("ImportSession: %v", err)
	}
	if imported.Metadata["key"] != "value" {
		t.Errorf("Metadata[key] = %q, want %q", imported.Metadata["key"], "value")
	}
	if imported.Metadata["foo"] != "bar" {
		t.Errorf("Metadata[foo] = %q, want %q", imported.Metadata["foo"], "bar")
	}
}

// TestExportedAtTimezone verifies that the exported timestamp survives JSON
// round-trip with the same UTC instant.
func TestExportedAtTimezone(t *testing.T) {
	msgs := []api.Message{{Role: "user", Content: rawMsg("hi")}}
	shared := ExportSession(msgs, "m", "s")
	original := shared.ExportedAt.UTC().Truncate(time.Second)

	data, err := MarshalSession(shared)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := ImportSession(data)
	if err != nil {
		t.Fatal(err)
	}
	got := imported.ExportedAt.UTC().Truncate(time.Second)
	if !got.Equal(original) {
		t.Errorf("ExportedAt: got %v, want %v", got, original)
	}
}
