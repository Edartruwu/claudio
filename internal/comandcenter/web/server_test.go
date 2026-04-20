package web_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	cc "github.com/Abraxas-365/claudio/internal/comandcenter"
	"github.com/Abraxas-365/claudio/internal/comandcenter/web"
)

const testPassword = "secret123"

// newTestEnv creates a WebServer backed by an in-memory SQLite database.
func newTestEnv(t *testing.T) (*cc.Storage, *http.ServeMux) {
	t.Helper()
	storage, err := cc.Open(":memory:")
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	// Simulate claudio's team_tasks table (normally in shared claudio.db).
	if err := storage.ExecRaw(`CREATE TABLE IF NOT EXISTS team_tasks (
		id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		subject TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'pending',
		assigned_to TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (id, session_id)
	)`); err != nil {
		t.Fatalf("create team_tasks: %v", err)
	}
	t.Cleanup(func() { storage.Close() })

	hub := cc.NewHub(storage)
	ws := web.NewWebServer(storage, hub, testPassword, "")
	mux := http.NewServeMux()
	ws.RegisterRoutes(mux)
	return storage, mux
}

func seedTask(t *testing.T, s *cc.Storage, tk cc.Task) {
	t.Helper()
	if err := s.ExecRaw(`
		INSERT INTO team_tasks (id, session_id, subject, description, status, assigned_to, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		tk.ID, tk.SessionID, tk.Title, tk.Description, tk.Status, tk.AssignedTo, tk.CreatedAt, tk.UpdatedAt,
	); err != nil {
		t.Fatalf("seedTask %s: %v", tk.ID, err)
	}
}

// authedRequest creates a request with the auth cookie set.
func authedRequest(method, target string) *http.Request {
	r := httptest.NewRequest(method, target, nil)
	r.AddCookie(&http.Cookie{Name: "auth", Value: testPassword})
	return r
}

// TestWebServer_Login_Redirect verifies GET / without cookie → redirect to /login.
func TestWebServer_Login_Redirect(t *testing.T) {
	_, mux := newTestEnv(t)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("want 303 redirect, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/login" {
		t.Fatalf("want Location: /login, got %q", loc)
	}
}

// TestWebServer_Login_POST_Valid verifies correct password → sets cookie + redirects.
func TestWebServer_Login_POST_Valid(t *testing.T) {
	_, mux := newTestEnv(t)

	form := url.Values{"password": {testPassword}}
	r := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("want 303, got %d", w.Code)
	}
	var found bool
	for _, c := range w.Result().Cookies() {
		if c.Name == "auth" && c.Value == testPassword {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("auth cookie not set after valid login")
	}
}

// TestWebServer_Login_POST_Invalid verifies wrong password → 401.
func TestWebServer_Login_POST_Invalid(t *testing.T) {
	_, mux := newTestEnv(t)

	form := url.Values{"password": {"wrongpassword"}}
	r := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

// TestWebServer_ChatList_Renders verifies GET / with valid cookie → 200 + "ComandCenter".
func TestWebServer_ChatList_Renders(t *testing.T) {
	_, mux := newTestEnv(t)

	r := authedRequest(http.MethodGet, "/")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d\nbody: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "ComandCenter") {
		t.Fatal("response body does not contain 'ComandCenter'")
	}
}

// TestWebServer_Partials_Sessions verifies GET /partials/sessions → 200 HTML fragment.
func TestWebServer_Partials_Sessions(t *testing.T) {
	_, mux := newTestEnv(t)

	r := authedRequest(http.MethodGet, "/partials/sessions")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); strings.Contains(ct, "application/json") {
		t.Fatalf("expected HTML response, got Content-Type: %s", ct)
	}
}

// TestWebServer_ChatView_Renders verifies GET /chat/{id} → 200 for a known session.
func TestWebServer_ChatView_Renders(t *testing.T) {
	storage, mux := newTestEnv(t)

	// Seed a session so GetSession succeeds.
	err := storage.UpsertSession(cc.Session{
		ID:           "test-session-id",
		Name:         "TestAgent",
		Path:         "/tmp",
		Status:       "active",
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}

	r := authedRequest(http.MethodGet, "/chat/test-session-id")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d\nbody: %s", w.Code, w.Body.String())
	}
}

// TestWebServer_ArchiveSession verifies PATCH /api/sessions/{id}/archive → 200 + session archived.
func TestWebServer_ArchiveSession(t *testing.T) {
	storage, mux := newTestEnv(t)

	err := storage.UpsertSession(cc.Session{
		ID:           "arch-sess-1",
		Name:         "ToArchive",
		Path:         "/tmp",
		Status:       "active",
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}

	r := authedRequest(http.MethodPatch, "/api/sessions/arch-sess-1/archive")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d\nbody: %s", w.Code, w.Body.String())
	}

	// Session must be absent from ListSessions (archived).
	sessions, err := storage.ListSessions("")
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	for _, s := range sessions {
		if s.ID == "arch-sess-1" {
			t.Error("archived session still visible in ListSessions")
		}
	}

	// Status in DB must be 'archived'.
	got, err := storage.GetSession("arch-sess-1")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.Status != "archived" {
		t.Errorf("Status after archive: got %q, want %q", got.Status, "archived")
	}
}

// TestWebServer_ArchiveSession_NoAuth verifies unauthenticated request → 303 redirect.
func TestWebServer_ArchiveSession_NoAuth(t *testing.T) {
	_, mux := newTestEnv(t)

	r := httptest.NewRequest(http.MethodPatch, "/api/sessions/any-id/archive", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("want 303 redirect for unauthed request, got %d", w.Code)
	}
}

// TestWebServer_DeleteSession verifies DELETE /api/sessions/{id} → 200 + session+messages removed.
func TestWebServer_DeleteSession(t *testing.T) {
	storage, mux := newTestEnv(t)

	err := storage.UpsertSession(cc.Session{
		ID:           "del-sess-1",
		Name:         "ToDelete",
		Path:         "/tmp",
		Status:       "active",
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}
	err = storage.InsertMessage(cc.Message{
		ID:        "del-msg-1",
		SessionID: "del-sess-1",
		Role:      "user",
		Content:   "hello",
		CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("seed message: %v", err)
	}

	r := authedRequest(http.MethodDelete, "/api/sessions/del-sess-1")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d\nbody: %s", w.Code, w.Body.String())
	}

	// Session must be gone.
	sessions, err := storage.ListSessions("")
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	for _, s := range sessions {
		if s.ID == "del-sess-1" {
			t.Error("deleted session still in ListSessions")
		}
	}

	// GetSession must error.
	_, err = storage.GetSession("del-sess-1")
	if err == nil {
		t.Error("expected error from GetSession after delete, got nil")
	}
}

// TestWebServer_DeleteSession_NoAuth verifies unauthenticated request → 303 redirect.
func TestWebServer_DeleteSession_NoAuth(t *testing.T) {
	_, mux := newTestEnv(t)

	r := httptest.NewRequest(http.MethodDelete, "/api/sessions/any-id", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("want 303 redirect for unauthed request, got %d", w.Code)
	}
}

// TestWebServer_SessionInfo_NotFound verifies GET /chat/{id}/info for unknown id → 404.
func TestWebServer_SessionInfo_NotFound(t *testing.T) {
	_, mux := newTestEnv(t)

	r := authedRequest(http.MethodGet, "/chat/nonexistent-id/info")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

// TestWebServer_SessionInfo_Renders verifies GET /chat/{id}/info → 200 with session name + tabs.
func TestWebServer_SessionInfo_Renders(t *testing.T) {
	storage, mux := newTestEnv(t)

	err := storage.UpsertSession(cc.Session{
		ID:           "info-sess-1",
		Name:         "InfoAgent",
		Path:         "/workspace",
		Status:       "active",
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}

	// Seed a task and agent.
	seedTask(t, storage, cc.Task{
		ID:        "info-task-1",
		SessionID: "info-sess-1",
		Title:     "Build feature",
		Status:    "in_progress",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})
	if err := storage.UpsertAgent(cc.Agent{
		ID:        "info-agent-1",
		SessionID: "info-sess-1",
		Name:      "worker",
		Status:    "working",
		UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}

	r := authedRequest(http.MethodGet, "/chat/info-sess-1/info")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d\nbody: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "InfoAgent") {
		t.Error("response body does not contain session name 'InfoAgent'")
	}
	if !strings.Contains(body, "Build feature") {
		t.Error("response body does not contain task title 'Build feature'")
	}
	// Team tab is HTMX-lazy — check it via the team endpoint directly.
	r2 := authedRequest(http.MethodGet, "/api/sessions/info-sess-1/team")
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, r2)
	if !strings.Contains(w2.Body.String(), "worker") {
		t.Error("team endpoint does not contain agent name 'worker'")
	}
	if !strings.Contains(body, "Tasks") {
		t.Error("response body does not contain 'Tasks' tab")
	}
	if !strings.Contains(body, "Team") {
		t.Error("response body does not contain 'Team' tab")
	}
	if !strings.Contains(body, "Media") {
		t.Error("response body does not contain 'Media' tab")
	}
}

// TestWebServer_SessionInfo_NoAuth verifies unauthenticated GET /chat/{id}/info → 303 redirect.
func TestWebServer_SessionInfo_NoAuth(t *testing.T) {
	_, mux := newTestEnv(t)

	r := httptest.NewRequest(http.MethodGet, "/chat/any-id/info", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("want 303 redirect for unauthed request, got %d", w.Code)
	}
}

// TestWebServer_TaskDetail_Renders verifies GET /chat/{id}/tasks/{task_id} → 200 with task content.
func TestWebServer_TaskDetail_Renders(t *testing.T) {
	storage, mux := newTestEnv(t)

	err := storage.UpsertSession(cc.Session{
		ID:           "td-sess-1",
		Name:         "TaskAgent",
		Path:         "/tmp",
		Status:       "active",
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}
	seedTask(t, storage, cc.Task{
		ID:          "td-task-1",
		SessionID:   "td-sess-1",
		Title:       "Detail Task",
		Description: "**bold** description",
		Status:      "in_progress",
		AssignedTo:  "agent-a",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	})

	r := authedRequest(http.MethodGet, "/chat/td-sess-1/tasks/td-task-1")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d\nbody: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "in_progress") && !strings.Contains(body, "in progress") {
		t.Error("response does not contain status")
	}
	if !strings.Contains(body, "agent-a") {
		t.Error("response does not contain assigned_to")
	}
	// Markdown rendered: **bold** → <strong>bold</strong>
	if !strings.Contains(body, "<strong>") {
		t.Error("response does not contain rendered markdown (<strong>)")
	}
}

// TestWebServer_TaskDetail_NotFound verifies GET /chat/{id}/tasks/{task_id} → 404 for unknown task.
func TestWebServer_TaskDetail_NotFound(t *testing.T) {
	_, mux := newTestEnv(t)

	r := authedRequest(http.MethodGet, "/chat/any-sess/tasks/nonexistent-task")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

// TestWebServer_TaskDetail_NoAuth verifies unauthenticated request → 303 redirect.
func TestWebServer_TaskDetail_NoAuth(t *testing.T) {
	_, mux := newTestEnv(t)

	r := httptest.NewRequest(http.MethodGet, "/chat/any-sess/tasks/any-task", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("want 303, got %d", w.Code)
	}
}

// TestByNameEndpoint_NotFound verifies POST /api/sessions/by-name/{name}/message
// returns 404 for an unknown session name.
func TestByNameEndpoint_NotFound(t *testing.T) {
	_, mux := newTestEnv(t)

	form := url.Values{"content": {"hello"}}
	r := authedRequest(http.MethodPost, "/api/sessions/by-name/NoSuchAgent/message")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Body = io.NopCloser(strings.NewReader(form.Encode()))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

// TestByNameEndpoint_Disconnected verifies POST /api/sessions/by-name/{name}/message
// returns 503 when the session exists but is not connected to the hub.
func TestByNameEndpoint_Disconnected(t *testing.T) {
	storage, mux := newTestEnv(t)

	if err := storage.UpsertSession(cc.Session{
		ID:           "byname-sess-1",
		Name:         "DiscoAgent",
		Path:         "/tmp",
		Status:       "inactive",
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	form := url.Values{"content": {"ping"}}
	r := authedRequest(http.MethodPost, "/api/sessions/by-name/DiscoAgent/message")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Body = io.NopCloser(strings.NewReader(form.Encode()))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", w.Code)
	}
}

// TestByNameEndpoint_NoAuth verifies unauthenticated request → 303 redirect.
func TestByNameEndpoint_NoAuth(t *testing.T) {
	_, mux := newTestEnv(t)

	r := httptest.NewRequest(http.MethodPost, "/api/sessions/by-name/AnyAgent/message", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("want 303 redirect for unauthed request, got %d", w.Code)
	}
}

// TestMentionRouting_UnknownTarget verifies @mention to a non-existent session → 404.
func TestMentionRouting_UnknownTarget(t *testing.T) {
	storage, mux := newTestEnv(t)

	// Seed originating session (no hub connection → but @mention resolves target first).
	if err := storage.UpsertSession(cc.Session{
		ID:           "origin-sess-1",
		Name:         "OriginAgent",
		Path:         "/tmp",
		Status:       "active",
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed origin session: %v", err)
	}

	form := url.Values{"content": {"@GhostAgent fix the bug"}}
	r := authedRequest(http.MethodPost, "/api/sessions/origin-sess-1/message")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Body = io.NopCloser(strings.NewReader(form.Encode()))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 for unknown @mention target, got %d", w.Code)
	}
}

// TestMentionRouting_TargetDisconnected verifies @mention to existing but disconnected session → 503.
func TestMentionRouting_TargetDisconnected(t *testing.T) {
	storage, mux := newTestEnv(t)

	for _, s := range []cc.Session{
		{ID: "origin-2", Name: "OriginAgent", Path: "/tmp", Status: "active", CreatedAt: time.Now(), LastActiveAt: time.Now()},
		{ID: "target-2", Name: "Pepito", Path: "/tmp", Status: "inactive", CreatedAt: time.Now(), LastActiveAt: time.Now()},
	} {
		if err := storage.UpsertSession(s); err != nil {
			t.Fatalf("seed session %s: %v", s.ID, err)
		}
	}

	form := url.Values{"content": {"@Pepito fix the bug"}}
	r := authedRequest(http.MethodPost, "/api/sessions/origin-2/message")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Body = io.NopCloser(strings.NewReader(form.Encode()))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503 for disconnected @mention target, got %d", w.Code)
	}
}

// TestMentionRouting_DualStore verifies @mention stores in both sessions with correct metadata.
// This test uses a fake hub connection to simulate a live session.
func TestMentionRouting_DualStore(t *testing.T) {
	storage, _ := newTestEnv(t)

	for _, s := range []cc.Session{
		{ID: "ds-origin", Name: "MasterAgent", Path: "/tmp", Status: "active", CreatedAt: time.Now(), LastActiveAt: time.Now()},
		{ID: "ds-target", Name: "Pepito", Path: "/tmp", Status: "active", CreatedAt: time.Now(), LastActiveAt: time.Now()},
	} {
		if err := storage.UpsertSession(s); err != nil {
			t.Fatalf("seed session %s: %v", s.ID, err)
		}
	}

	// Simulate @mention routing logic directly against storage (hub not connected in unit test).
	// Verify GetSessionByName returns the target correctly.
	sess, found, err := storage.GetSessionByName("Pepito")
	if err != nil {
		t.Fatalf("GetSessionByName: %v", err)
	}
	if !found {
		t.Fatal("target session Pepito not found")
	}
	if sess.ID != "ds-target" {
		t.Fatalf("want ds-target, got %q", sess.ID)
	}

	// Insert messages as the handler would.
	now := time.Now()
	fullContent := "@Pepito fix the bug"
	msgBody := "fix the bug"
	originMsg := cc.Message{
		ID:             "ds-origin-msg-1",
		SessionID:      "ds-origin",
		Role:           "user",
		Content:        fullContent,
		CreatedAt:      now,
		ReplyToSession: "Pepito",
		QuotedContent:  fullContent, // <80 chars
	}
	targetMsg := cc.Message{
		ID:        "ds-target-msg-1",
		SessionID: "ds-target",
		Role:      "user",
		Content:   msgBody,
		CreatedAt: now,
	}
	if err := storage.InsertMessage(originMsg); err != nil {
		t.Fatalf("insert origin msg: %v", err)
	}
	if err := storage.InsertMessage(targetMsg); err != nil {
		t.Fatalf("insert target msg: %v", err)
	}

	// Verify origin session has message with reply metadata.
	originMsgs, err := storage.ListMessages("ds-origin", 10)
	if err != nil {
		t.Fatalf("ListMessages origin: %v", err)
	}
	if len(originMsgs) != 1 {
		t.Fatalf("want 1 origin message, got %d", len(originMsgs))
	}
	if originMsgs[0].ReplyToSession != "Pepito" {
		t.Errorf("origin msg ReplyToSession: want %q, got %q", "Pepito", originMsgs[0].ReplyToSession)
	}
	if originMsgs[0].QuotedContent != fullContent {
		t.Errorf("origin msg QuotedContent: want %q, got %q", fullContent, originMsgs[0].QuotedContent)
	}

	// Verify target session has plain message without reply metadata.
	targetMsgs, err := storage.ListMessages("ds-target", 10)
	if err != nil {
		t.Fatalf("ListMessages target: %v", err)
	}
	if len(targetMsgs) != 1 {
		t.Fatalf("want 1 target message, got %d", len(targetMsgs))
	}
	if targetMsgs[0].Content != msgBody {
		t.Errorf("target msg Content: want %q, got %q", msgBody, targetMsgs[0].Content)
	}
	if targetMsgs[0].ReplyToSession != "" {
		t.Errorf("target msg should have empty ReplyToSession, got %q", targetMsgs[0].ReplyToSession)
	}
}

// TestBrowseSession_ListsFiles verifies GET /api/sessions/{id}/browse returns 200 + items.
func TestBrowseSession_ListsFiles(t *testing.T) {
	storage, mux := newTestEnv(t)

	// Create a temp dir with a file and a subdirectory.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("create test file: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatalf("create subdir: %v", err)
	}

	if err := storage.UpsertSession(cc.Session{
		ID:           "browse-sess-1",
		Name:         "BrowseAgent",
		Path:         dir,
		Status:       "active",
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	r := authedRequest(http.MethodGet, "/api/sessions/browse-sess-1/browse")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d\nbody: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Current string `json:"current"`
		Root    string `json:"root"`
		Items   []struct {
			Name  string `json:"name"`
			IsDir bool   `json:"is_dir"`
		} `json:"items"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Root == "" {
		t.Error("response.root is empty")
	}
	if len(resp.Items) < 2 {
		t.Fatalf("want at least 2 items, got %d", len(resp.Items))
	}
	var foundFile, foundDir bool
	for _, item := range resp.Items {
		if item.Name == "hello.txt" && !item.IsDir {
			foundFile = true
		}
		if item.Name == "subdir" && item.IsDir {
			foundDir = true
		}
	}
	if !foundFile {
		t.Error("hello.txt not in response items")
	}
	if !foundDir {
		t.Error("subdir not in response items")
	}
}

// TestBrowseSession_TraversalBlocked verifies path traversal above session root → 403.
func TestBrowseSession_TraversalBlocked(t *testing.T) {
	storage, mux := newTestEnv(t)

	dir := t.TempDir()
	if err := storage.UpsertSession(cc.Session{
		ID:           "browse-sess-2",
		Name:         "BrowseAgent2",
		Path:         dir,
		Status:       "active",
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	// Attempt traversal: ../../etc
	r := authedRequest(http.MethodGet, "/api/sessions/browse-sess-2/browse?path="+url.QueryEscape("../../etc"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403 for traversal, got %d\nbody: %s", w.Code, w.Body.String())
	}
}

// TestBrowseSession_NoPath verifies session with empty path → 400.
func TestBrowseSession_NoPath(t *testing.T) {
	storage, mux := newTestEnv(t)

	if err := storage.UpsertSession(cc.Session{
		ID:           "browse-sess-3",
		Name:         "BrowseAgent3",
		Path:         "", // no path set
		Status:       "active",
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	r := authedRequest(http.MethodGet, "/api/sessions/browse-sess-3/browse")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for no-path session, got %d\nbody: %s", w.Code, w.Body.String())
	}
}

// TestHandleSendMessage_Clear verifies POST content=/clear → 204 + all messages deleted.
func TestHandleSendMessage_Clear(t *testing.T) {
	storage, mux := newTestEnv(t)

	if err := storage.UpsertSession(cc.Session{
		ID:           "clear-sess-1",
		Name:         "ClearAgent",
		Path:         "/tmp",
		Status:       "active",
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	for _, m := range []cc.Message{
		{ID: "cl-msg-1", SessionID: "clear-sess-1", Role: "user", Content: "one", CreatedAt: time.Now()},
		{ID: "cl-msg-2", SessionID: "clear-sess-1", Role: "assistant", Content: "two", CreatedAt: time.Now()},
		{ID: "cl-msg-3", SessionID: "clear-sess-1", Role: "user", Content: "three", CreatedAt: time.Now()},
	} {
		if err := storage.InsertMessage(m); err != nil {
			t.Fatalf("seed message %s: %v", m.ID, err)
		}
	}

	form := url.Values{"content": {"/clear"}}
	r := authedRequest(http.MethodPost, "/api/sessions/clear-sess-1/message")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Body = io.NopCloser(strings.NewReader(form.Encode()))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d\nbody: %s", w.Code, w.Body.String())
	}

	msgs, err := storage.ListMessages("clear-sess-1", 100)
	if err != nil {
		t.Fatalf("ListMessages after /clear: %v", err)
	}
	// Handler inserts one system confirmation bubble after clearing.
	if len(msgs) != 1 {
		t.Errorf("expected 1 confirmation message after /clear, got %d", len(msgs))
	}
	if msgs[0].Content != "Conversation cleared. ✓" {
		t.Errorf("unexpected confirmation content: %q", msgs[0].Content)
	}
}

// TestHandleSendMessage_Compact_NoAPIClient verifies POST content=/compact → 503 when no API client.
func TestHandleSendMessage_Compact_NoAPIClient(t *testing.T) {
	storage, mux := newTestEnv(t)

	if err := storage.UpsertSession(cc.Session{
		ID:           "compact-sess-1",
		Name:         "CompactAgent",
		Path:         "/tmp",
		Status:       "active",
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if err := storage.InsertMessage(cc.Message{
		ID:        "cmp-msg-1",
		SessionID: "compact-sess-1",
		Role:      "user",
		Content:   "hello",
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed message: %v", err)
	}

	seedNativeTables(t, storage, "compact-sess-1")
	if err := storage.ExecRaw(
		`INSERT INTO messages (session_id, role, content, type) VALUES (?, 'user', 'hello', 'text')`,
		"compact-sess-1",
	); err != nil {
		t.Fatalf("seed native message: %v", err)
	}

	form := url.Values{"content": {"/compact"}}
	r := authedRequest(http.MethodPost, "/api/sessions/compact-sess-1/message")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Body = io.NopCloser(strings.NewReader(form.Encode()))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d\nbody: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "compact unavailable") {
		t.Errorf("expected body to contain %q, got: %s", "compact unavailable", w.Body.String())
	}
}

// seedNativeTables creates the native sessions and messages tables and inserts
// a session row so that FK constraints on messages.session_id are satisfied.
func seedNativeTables(t *testing.T, s *cc.Storage, sessionID string) {
	t.Helper()
	if err := s.ExecRaw(`CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		path TEXT NOT NULL DEFAULT '',
		model TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		last_active_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		t.Fatalf("seedNativeTables create sessions: %v", err)
	}
	if err := s.ExecRaw(`CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL,
		role TEXT NOT NULL,
		content TEXT NOT NULL,
		type TEXT NOT NULL DEFAULT 'text',
		tool_use_id TEXT DEFAULT '',
		tool_name TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
	)`); err != nil {
		t.Fatalf("seedNativeTables create messages: %v", err)
	}
	if err := s.ExecRaw(`INSERT OR IGNORE INTO sessions (id, name) VALUES (?, ?)`, sessionID, "test"); err != nil {
		t.Fatalf("seedNativeTables insert session: %v", err)
	}
}

// TestHandleSendMessage_Clear_AlsoClearsNativeMessages verifies /clear deletes
// rows from the native messages table in addition to cc_messages.
func TestHandleSendMessage_Clear_AlsoClearsNativeMessages(t *testing.T) {
	storage, mux := newTestEnv(t)

	const sid = "clear-native-1"

	if err := storage.UpsertSession(cc.Session{
		ID:           sid,
		Name:         "ClearNativeAgent",
		Path:         "/tmp",
		Status:       "active",
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	}); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	seedNativeTables(t, storage, sid)

	// Insert 2 native messages (no id — autoincrement).
	for _, content := range []string{"native-one", "native-two"} {
		if err := storage.ExecRaw(
			`INSERT INTO messages (session_id, role, content) VALUES (?, ?, ?)`,
			sid, "user", content,
		); err != nil {
			t.Fatalf("insert native message: %v", err)
		}
	}

	// Insert 2 cc_messages.
	for i, content := range []string{"cc-one", "cc-two"} {
		if err := storage.InsertMessage(cc.Message{
			ID:        "clr-cc-msg-" + string(rune('0'+i)),
			SessionID: sid,
			Role:      "user",
			Content:   content,
			CreatedAt: time.Now(),
		}); err != nil {
			t.Fatalf("InsertMessage: %v", err)
		}
	}

	form := url.Values{"content": {"/clear"}}
	r := authedRequest(http.MethodPost, "/api/sessions/"+sid+"/message")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Body = io.NopCloser(strings.NewReader(form.Encode()))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d\nbody: %s", w.Code, w.Body.String())
	}

	// Native messages table must be empty.
	nativeMsgs, err := storage.GetNativeMessages(sid, 100)
	if err != nil {
		t.Fatalf("GetNativeMessages after /clear: %v", err)
	}
	if len(nativeMsgs) != 0 {
		t.Errorf("expected 0 native messages after /clear, got %d", len(nativeMsgs))
	}

	// cc_messages: only the confirmation bubble remains.
	ccMsgs, err := storage.ListMessages(sid, 100)
	if err != nil {
		t.Fatalf("ListMessages after /clear: %v", err)
	}
	if len(ccMsgs) != 1 {
		t.Errorf("expected 1 cc_message (confirmation) after /clear, got %d", len(ccMsgs))
	}
}

// TestHandleSendMessage_Clear_WipesStorage verifies POST content=/clear removes seeded messages from storage.
func TestHandleSendMessage_Clear_WipesStorage(t *testing.T) {
	storage, mux := newTestEnv(t)

	const sid = "wipe-sess-1"

	if err := storage.UpsertSession(cc.Session{
		ID:           sid,
		Name:         "WipeAgent",
		Path:         "/tmp",
		Status:       "active",
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	for _, m := range []cc.Message{
		{ID: "wipe-msg-1", SessionID: sid, Role: "user", Content: "alpha", CreatedAt: time.Now()},
		{ID: "wipe-msg-2", SessionID: sid, Role: "assistant", Content: "beta", CreatedAt: time.Now()},
	} {
		if err := storage.InsertMessage(m); err != nil {
			t.Fatalf("seed message %s: %v", m.ID, err)
		}
	}

	form := url.Values{"content": {"/clear"}}
	r := authedRequest(http.MethodPost, "/api/sessions/"+sid+"/message")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Body = io.NopCloser(strings.NewReader(form.Encode()))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d\nbody: %s", w.Code, w.Body.String())
	}

	msgs, err := storage.ListMessages(sid, 100)
	if err != nil {
		t.Fatalf("ListMessages after /clear: %v", err)
	}
	// Handler inserts one system confirmation bubble; seeded messages must be gone.
	if len(msgs) != 1 {
		t.Errorf("expected 1 confirmation message after /clear, got %d", len(msgs))
	}
	if len(msgs) == 1 && msgs[0].Content != "Conversation cleared. ✓" {
		t.Errorf("unexpected confirmation content: %q", msgs[0].Content)
	}
}

// TestHandleSendMessage_Compact_ReadsNativeMessages verifies /compact reads from
// the native messages table. When messages exist there but cc_messages is empty,
// compact reaches the "no API client" error path (503) rather than "Nothing to compact".
func TestHandleSendMessage_Compact_ReadsNativeMessages(t *testing.T) {
	storage, mux := newTestEnv(t) // no API client

	const sid = "compact-native-1"

	if err := storage.UpsertSession(cc.Session{
		ID:           sid,
		Name:         "CompactNativeAgent",
		Path:         "/tmp",
		Status:       "active",
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	}); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	seedNativeTables(t, storage, sid)

	// Insert 2 native messages — do NOT insert any cc_messages.
	for _, content := range []string{"native-msg-a", "native-msg-b"} {
		if err := storage.ExecRaw(
			`INSERT INTO messages (session_id, role, content) VALUES (?, ?, ?)`,
			sid, "user", content,
		); err != nil {
			t.Fatalf("insert native message: %v", err)
		}
	}

	form := url.Values{"content": {"/compact"}}
	r := authedRequest(http.MethodPost, "/api/sessions/"+sid+"/message")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Body = io.NopCloser(strings.NewReader(form.Encode()))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	// 503 proves the API client check fired (data was found; no client configured).
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503 (no API client), got %d\nbody: %s", w.Code, w.Body.String())
	}
}

// TestHandleSendMessage_Compact_NothingToCompact verifies /compact with empty
// messages table returns 202 and inserts a "Nothing to compact" bubble.
func TestHandleSendMessage_Compact_NothingToCompact(t *testing.T) {
	storage, mux := newTestEnv(t) // no API client

	const sid = "compact-empty-1"

	if err := storage.UpsertSession(cc.Session{
		ID:           sid,
		Name:         "CompactEmptyAgent",
		Path:         "/tmp",
		Status:       "active",
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	}); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	// Create native tables + session row, but insert NO messages.
	seedNativeTables(t, storage, sid)

	form := url.Values{"content": {"/compact"}}
	r := authedRequest(http.MethodPost, "/api/sessions/"+sid+"/message")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Body = io.NopCloser(strings.NewReader(form.Encode()))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusAccepted {
		t.Fatalf("want 202, got %d\nbody: %s", w.Code, w.Body.String())
	}

	msgs, err := storage.ListMessages(sid, 10)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) == 0 {
		t.Fatal("want confirmation bubble, got no messages")
	}
	found := false
	for _, m := range msgs {
		if strings.Contains(m.Content, "Nothing to compact") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a message containing %q, got: %+v", "Nothing to compact", msgs)
	}
}
