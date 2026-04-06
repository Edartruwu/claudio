package web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/web/templates"
)

// newTestServer creates a Server for testing with a known password.
func newTestServer(t *testing.T) *Server {
	t.Helper()
	s := New(Config{
		Port:     0,
		Password: "testpass",
		Version:  "test-0.1",
	}, nil)
	return s
}

// authCookie returns a valid auth cookie for the test server.
func authCookie(t *testing.T, s *Server) *http.Cookie {
	t.Helper()
	token := generateToken()
	s.mu.Lock()
	s.tokens[token] = time.Now().Add(time.Hour)
	s.mu.Unlock()
	return &http.Cookie{
		Name:  "claudio_token",
		Value: token,
	}
}

func TestLoginPage_Renders(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest("GET", "/login", nil)
	w := httptest.NewRecorder()
	s.handleLoginPage(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "password") {
		t.Error("login page should contain password field")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	s := newTestServer(t)
	form := url.Values{"password": {"wrong"}}
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	s.handleLogin(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "Invalid password") {
		t.Error("expected 'Invalid password' error message")
	}
	// Should not set auth cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "claudio_token" && c.MaxAge > 0 {
			t.Error("should not set token cookie on wrong password")
		}
	}
}

func TestLogin_CorrectPassword(t *testing.T) {
	s := newTestServer(t)
	form := url.Values{"password": {"testpass"}}
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	s.handleLogin(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected redirect (303), got %d", w.Code)
	}
	var found bool
	for _, c := range w.Result().Cookies() {
		if c.Name == "claudio_token" && c.Value != "" {
			found = true
			// Verify token is stored
			s.mu.Lock()
			_, ok := s.tokens[c.Value]
			s.mu.Unlock()
			if !ok {
				t.Error("token not stored in server")
			}
		}
	}
	if !found {
		t.Error("expected claudio_token cookie to be set")
	}
}

func TestLogout_ClearsCookie(t *testing.T) {
	s := newTestServer(t)
	cookie := authCookie(t, s)

	req := httptest.NewRequest("POST", "/logout", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()

	s.handleLogout(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected redirect, got %d", w.Code)
	}
	// Token should be removed from server
	s.mu.Lock()
	_, ok := s.tokens[cookie.Value]
	s.mu.Unlock()
	if ok {
		t.Error("token should have been removed from server on logout")
	}
}

func TestAuthMiddleware_NoToken(t *testing.T) {
	s := newTestServer(t)
	called := false
	handler := s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if called {
		t.Error("handler should not be called without auth")
	}
	if w.Code != http.StatusSeeOther {
		t.Errorf("expected redirect to /login, got %d", w.Code)
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	s := newTestServer(t)
	called := false
	handler := s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "claudio_token", Value: "bogus"})
	w := httptest.NewRecorder()
	handler(w, req)

	if called {
		t.Error("handler should not be called with invalid token")
	}
}

func TestAuthMiddleware_ExpiredToken(t *testing.T) {
	s := newTestServer(t)
	token := generateToken()
	s.mu.Lock()
	s.tokens[token] = time.Now().Add(-time.Hour) // expired
	s.mu.Unlock()

	called := false
	handler := s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "claudio_token", Value: token})
	w := httptest.NewRecorder()
	handler(w, req)

	if called {
		t.Error("handler should not be called with expired token")
	}
	// Expired token should be cleaned up
	s.mu.Lock()
	_, ok := s.tokens[token]
	s.mu.Unlock()
	if ok {
		t.Error("expired token should be removed")
	}
}

func TestAuthMiddleware_ValidToken(t *testing.T) {
	s := newTestServer(t)
	cookie := authCookie(t, s)

	called := false
	handler := s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	handler(w, req)

	if !called {
		t.Error("handler should be called with valid token")
	}
}

func TestHome_RendersProjectList(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	s.handleHome(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Claudio") {
		t.Error("home page should contain 'Claudio'")
	}
}

func TestProjectInit_CreatesDirectory(t *testing.T) {
	s := newTestServer(t)
	tmpDir := t.TempDir()
	projectPath := filepath.Join(tmpDir, "new-project")

	form := url.Values{"path": {projectPath}}
	req := httptest.NewRequest("POST", "/api/projects/init", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	s.handleProjectInit(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Check .claudio dir was created
	claudioDir := filepath.Join(projectPath, ".claudio")
	if _, err := os.Stat(claudioDir); os.IsNotExist(err) {
		t.Error("expected .claudio directory to be created")
	}

	// Check settings.json was created
	settingsPath := filepath.Join(claudioDir, "settings.json")
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		t.Error("expected settings.json to be created")
	}
}

func TestProjectInit_EmptyPath(t *testing.T) {
	s := newTestServer(t)
	form := url.Values{"path": {""}}
	req := httptest.NewRequest("POST", "/api/projects/init", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	s.handleProjectInit(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestProjectInit_ExpandsTilde(t *testing.T) {
	s := newTestServer(t)
	// Use a temp dir as the "home" target — we can't easily mock os.UserHomeDir
	// but we can verify the path expansion logic works
	home, _ := os.UserHomeDir()
	projectName := fmt.Sprintf("claudio-test-%d", time.Now().UnixNano())
	tilePath := "~/" + projectName

	form := url.Values{"path": {tilePath}}
	req := httptest.NewRequest("POST", "/api/projects/init", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	s.handleProjectInit(w, req)

	expectedPath := filepath.Join(home, projectName)
	// Cleanup
	defer os.RemoveAll(expectedPath)

	if _, err := os.Stat(filepath.Join(expectedPath, ".claudio")); os.IsNotExist(err) {
		t.Errorf("expected .claudio at %s", expectedPath)
	}
}

func TestChatPage_RedirectsWithoutProject(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest("GET", "/chat", nil)
	w := httptest.NewRecorder()

	s.handleChatPage(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected redirect, got %d", w.Code)
	}
}

func TestChatPage_InvalidProject(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest("GET", "/chat?project=/nonexistent/path", nil)
	w := httptest.NewRecorder()

	s.handleChatPage(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestChatPage_ValidProject(t *testing.T) {
	s := newTestServer(t)
	tmpDir := t.TempDir()

	req := httptest.NewRequest("GET", "/chat?project="+tmpDir, nil)
	w := httptest.NewRecorder()

	s.handleChatPage(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, tmpDir) {
		t.Error("chat page should contain the project path")
	}
}

func TestChatSend_MissingParams(t *testing.T) {
	s := newTestServer(t)

	// Missing both
	req := httptest.NewRequest("POST", "/api/chat/send", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.handleChatSend(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	// Missing message
	form := url.Values{"session": {"some-id"}}
	req = httptest.NewRequest("POST", "/api/chat/send", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	s.handleChatSend(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestChatStream_NoSession(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/chat/stream?session=nonexistent", nil)
	w := httptest.NewRecorder()

	s.handleChatStream(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestChatStream_NoHandler(t *testing.T) {
	s := newTestServer(t)
	tmpDir := t.TempDir()

	// Create session but don't send a message (no handler)
	sess, err := s.sessions.GetOrCreateDefault(tmpDir)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/chat/stream?session="+sess.ID, nil)
	w := httptest.NewRecorder()

	s.handleChatStream(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestToolApproval_NoSession(t *testing.T) {
	s := newTestServer(t)
	form := url.Values{"session": {"nonexistent"}}
	req := httptest.NewRequest("POST", "/api/chat/approve", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	s.handleToolApprove(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestToolApproval_MissingProject(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest("POST", "/api/chat/approve", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	s.handleToolApprove(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestStaticFiles_CSS(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest("GET", "/static/style.css", nil)
	w := httptest.NewRecorder()

	s.handleStatic(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/css" {
		t.Errorf("expected text/css, got %s", ct)
	}
	if w.Body.Len() == 0 {
		t.Error("expected non-empty CSS response")
	}
}

func TestStaticFiles_NotFound(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest("GET", "/static/nonexistent.js", nil)
	w := httptest.NewRecorder()

	s.handleStatic(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestSessionManager_GetOrCreateDefault(t *testing.T) {
	sm := NewSessionManager()
	tmpDir := t.TempDir()

	sess1, err := sm.GetOrCreateDefault(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sess1.ProjectPath != tmpDir {
		t.Errorf("expected path %s, got %s", tmpDir, sess1.ProjectPath)
	}

	// Second call should return the same session
	sess2, err := sm.GetOrCreateDefault(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sess1 != sess2 {
		t.Error("expected same session instance")
	}
}

func TestSessionManager_GetOrCreateDefault_InvalidPath(t *testing.T) {
	sm := NewSessionManager()
	_, err := sm.GetOrCreateDefault("/definitely/not/a/real/path")
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

func TestSessionManager_Get_NotFound(t *testing.T) {
	sm := NewSessionManager()
	if sess := sm.Get("/nonexistent"); sess != nil {
		t.Error("expected nil for non-existent session")
	}
}

func TestSessionManager_Get_Found(t *testing.T) {
	sm := NewSessionManager()
	tmpDir := t.TempDir()

	created, _ := sm.GetOrCreateDefault(tmpDir)
	sess := sm.Get(created.ID)
	if sess == nil {
		t.Fatal("expected session to be found")
	}
	if sess.ProjectPath != tmpDir {
		t.Errorf("expected path %s, got %s", tmpDir, sess.ProjectPath)
	}
}

func TestSessionManager_ListProjects(t *testing.T) {
	sm := NewSessionManager()
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	sm.GetOrCreateDefault(dir1)
	sm.GetOrCreateDefault(dir2)

	paths := sm.ListProjects()
	if len(paths) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(paths))
	}
	found := make(map[string]bool)
	for _, p := range paths {
		found[p] = true
	}
	if !found[dir1] || !found[dir2] {
		t.Errorf("missing expected paths in %v", paths)
	}
}

func TestSessionManager_MultipleProjects(t *testing.T) {
	sm := NewSessionManager()
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	sess1, _ := sm.GetOrCreateDefault(dir1)
	sess2, _ := sm.GetOrCreateDefault(dir2)

	if sess1 == sess2 {
		t.Error("different projects should have different sessions")
	}
}

func TestProjectSession_AddMessage(t *testing.T) {
	sm := NewSessionManager()
	tmpDir := t.TempDir()
	sess, _ := sm.GetOrCreateDefault(tmpDir)

	sess.AddMessage("user", "hello")
	sess.AddMessage("assistant", "hi there")

	sess.mu.Lock()
	msgs := sess.Messages
	sess.mu.Unlock()

	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "hello" {
		t.Errorf("unexpected first message: %+v", msgs[0])
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != "hi there" {
		t.Errorf("unexpected second message: %+v", msgs[1])
	}
}

func TestProjectSession_CurrentHandler_Nil(t *testing.T) {
	sm := NewSessionManager()
	tmpDir := t.TempDir()
	sess, _ := sm.GetOrCreateDefault(tmpDir)

	if h := sess.CurrentHandler(); h != nil {
		t.Error("expected nil handler before any message sent")
	}
}

func TestSSEStream_Integration(t *testing.T) {
	// Test that SSE stream correctly formats events
	s := newTestServer(t)
	tmpDir := t.TempDir()

	// Create session and manually set a handler
	sess, err := s.sessions.GetOrCreateDefault(tmpDir)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	handler := NewWebHandler()
	sess.mu.Lock()
	sess.currentHandler = handler
	sess.mu.Unlock()

	// Push some events and close
	go func() {
		handler.OnTextDelta("hello world")
		handler.OnTurnComplete(api.Usage{InputTokens: 10, OutputTokens: 5})
	}()

	req := httptest.NewRequest("GET", "/api/chat/stream?session="+sess.ID, nil)
	w := httptest.NewRecorder()

	s.handleChatStream(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "event: text") {
		t.Error("expected SSE text event in response")
	}
	if !strings.Contains(body, "data: hello world") {
		t.Error("expected 'hello world' in SSE data")
	}
	if !strings.Contains(body, "event: done") {
		t.Error("expected SSE done event in response")
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %s", ct)
	}

	// Check assistant message was saved
	sess.mu.Lock()
	msgs := sess.Messages
	sess.mu.Unlock()
	found := false
	for _, m := range msgs {
		if m.Role == "assistant" && m.Content == "hello world" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected assistant message saved, got %+v", msgs)
	}
}

func TestSSEStream_MultilineData(t *testing.T) {
	// SSE spec: newlines in data must be split into multiple "data:" lines
	s := newTestServer(t)
	tmpDir := t.TempDir()

	sess, _ := s.sessions.GetOrCreateDefault(tmpDir)
	handler := NewWebHandler()
	sess.mu.Lock()
	sess.currentHandler = handler
	sess.mu.Unlock()

	go func() {
		handler.OnTextDelta("line1\nline2\nline3")
		handler.OnTurnComplete(api.Usage{})
	}()

	req := httptest.NewRequest("GET", "/api/chat/stream?session="+sess.ID, nil)
	w := httptest.NewRecorder()

	s.handleChatStream(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "data: line1\ndata: line2\ndata: line3") {
		t.Errorf("multiline data not properly formatted: %s", body)
	}
}

func TestGenerateToken(t *testing.T) {
	token1 := generateToken()
	token2 := generateToken()

	if token1 == "" {
		t.Error("token should not be empty")
	}
	if len(token1) < 32 {
		t.Errorf("token too short: %d chars", len(token1))
	}
	if token1 == token2 {
		t.Error("tokens should be unique")
	}
}

func TestValidatePassword(t *testing.T) {
	s := newTestServer(t)
	if !s.validatePassword("testpass") {
		t.Error("should accept correct password")
	}
	if s.validatePassword("wrong") {
		t.Error("should reject wrong password")
	}
	if s.validatePassword("") {
		t.Error("should reject empty password")
	}
}

// TestFullE2E_LoginAndBrowse tests the full flow: login → home page.
func TestFullE2E_LoginAndBrowse(t *testing.T) {
	s := newTestServer(t)
	ts := httptest.NewServer(s.mux)
	defer ts.Close()

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // don't follow redirects
		},
	}

	// 1. Login
	form := url.Values{"password": {"testpass"}}
	resp, err := client.PostForm(ts.URL+"/login", form)
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected redirect after login, got %d: %s", resp.StatusCode, body)
	}

	// Extract token cookie
	var tokenCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "claudio_token" {
			tokenCookie = c
		}
	}
	if tokenCookie == nil {
		t.Fatal("expected claudio_token cookie after login")
	}

	// 2. Access home page with token
	req, _ := http.NewRequest("GET", ts.URL+"/", nil)
	req.AddCookie(tokenCookie)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("home request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Projects") {
		t.Error("home page should contain 'Projects'")
	}
}

// TestFullE2E_InitProject tests initializing a project via the API.
func TestFullE2E_InitProject(t *testing.T) {
	s := newTestServer(t)
	ts := httptest.NewServer(s.mux)
	defer ts.Close()

	cookie := authCookie(t, s)
	tmpDir := filepath.Join(t.TempDir(), "my-project")

	form := url.Values{"path": {tmpDir}}
	req, _ := http.NewRequest("POST", ts.URL+"/api/projects/init", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	// Verify directory structure
	if _, err := os.Stat(filepath.Join(tmpDir, ".claudio")); os.IsNotExist(err) {
		t.Error(".claudio directory not created")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, ".claudio", "settings.json")); os.IsNotExist(err) {
		t.Error("settings.json not created")
	}
}

// --- template rendering tests ---

func TestTemplates_LoginPage(t *testing.T) {
	w := httptest.NewRecorder()
	templates.LoginPage("test error").Render(context.Background(), w)
	body := w.Body.String()
	if !strings.Contains(body, "test error") {
		t.Error("login page should render error message")
	}
}

func TestTemplates_HomePage(t *testing.T) {
	w := httptest.NewRecorder()
	projects := []templates.ProjectInfo{
		{Name: "proj1", Path: "/path/to/proj1", Initialized: true},
		{Name: "proj2", Path: "/path/to/proj2", Initialized: false},
	}
	templates.HomePage(projects, "v1.0").Render(context.Background(), w)
	body := w.Body.String()
	if !strings.Contains(body, "proj1") {
		t.Error("home page should contain project name")
	}
	if !strings.Contains(body, "/path/to/proj1") {
		t.Error("home page should contain project path")
	}
	if !strings.Contains(body, "v1.0") {
		t.Error("home page should contain version")
	}
}

func TestTemplates_ChatPage(t *testing.T) {
	w := httptest.NewRecorder()
	messages := []templates.ChatMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
	}
	sessions := []templates.SessionInfo{{ID: "test-session", Title: "Test", State: "idle", MsgCount: 2, Active: true}}
	templates.ChatPage("/test/project", "test-session", sessions, messages).Render(context.Background(), w)
	body := w.Body.String()
	if !strings.Contains(body, "/test/project") {
		t.Error("chat page should contain project path")
	}
	if !strings.Contains(body, "hello") {
		t.Error("chat page should render user message")
	}
	if !strings.Contains(body, "hi there") {
		t.Error("chat page should render assistant message")
	}
}

func TestTemplates_ProjectList_Empty(t *testing.T) {
	w := httptest.NewRecorder()
	templates.ProjectList(nil).Render(context.Background(), w)
	body := w.Body.String()
	if !strings.Contains(body, "No projects found") {
		t.Error("empty project list should show empty state")
	}
}

func TestTemplates_ErrorMessage(t *testing.T) {
	w := httptest.NewRecorder()
	templates.ErrorMessage("something failed").Render(context.Background(), w)
	body := w.Body.String()
	if !strings.Contains(body, "something failed") {
		t.Error("error message should contain the message")
	}
}

// ── Panel tests ──

func TestPanel_Analytics(t *testing.T) {
	s := newTestServer(t)
	tmpDir := t.TempDir()

	// Create session with some analytics data
	sess, err := s.sessions.GetOrCreateDefault(tmpDir)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	sess.mu.Lock()
	sess.TotalInputTokens = 1500
	sess.TotalOutputTokens = 800
	sess.CacheReadTokens = 500
	sess.CacheCreateTokens = 200
	sess.mu.Unlock()

	req := httptest.NewRequest("GET", "/api/panel/analytics?session="+sess.ID, nil)
	w := httptest.NewRecorder()
	s.handlePanel(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "1500") {
		t.Error("analytics panel should show input tokens")
	}
	if !strings.Contains(body, "800") {
		t.Error("analytics panel should show output tokens")
	}
}

func TestPanel_Config(t *testing.T) {
	s := newTestServer(t)
	tmpDir := t.TempDir()
	sess, _ := s.sessions.GetOrCreateDefault(tmpDir)

	req := httptest.NewRequest("GET", "/api/panel/config?session="+sess.ID, nil)
	w := httptest.NewRecorder()
	s.handlePanel(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "headless") {
		t.Error("config panel should show permission mode")
	}
	if !strings.Contains(body, tmpDir) {
		t.Error("config panel should show project path")
	}
}

func TestPanel_Tasks_Empty(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/panel/tasks?project=/tmp", nil)
	w := httptest.NewRecorder()
	s.handlePanel(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "No tasks") {
		t.Error("empty tasks panel should show 'No tasks'")
	}
}

func TestPanel_Unknown(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/panel/foobar?project=/tmp", nil)
	w := httptest.NewRecorder()
	s.handlePanel(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "Unknown panel") {
		t.Error("unknown panel should show error message")
	}
}

// ── Mobile / Safari iOS fixes ──

func TestChatPage_FormIsPlainNoAction(t *testing.T) {
	// The form must NOT have any action attribute — no action="javascript:void(0)",
	// no action="" — Safari navigates on submit if action is present.
	// The form must also have no onsubmit inline handler.
	w := httptest.NewRecorder()
	sessions := []templates.SessionInfo{{ID: "s1", Title: "T", State: "idle", MsgCount: 0, Active: true}}
	templates.ChatPage("/p", "s1", sessions, nil).Render(context.Background(), w)
	body := w.Body.String()
	if strings.Contains(body, `action=`) {
		t.Error("form must not have any action attribute — causes Safari to navigate")
	}
	if strings.Contains(body, `onsubmit=`) {
		t.Error("form must not use inline onsubmit — use addEventListener in JS instead")
	}
}

func TestChatPage_SendButtonIsTypeButton(t *testing.T) {
	// The send button must be type="button" to completely bypass native form
	// submission which iOS Safari handles unreliably.
	w := httptest.NewRecorder()
	sessions := []templates.SessionInfo{{ID: "s1", Title: "T", State: "idle", MsgCount: 0, Active: true}}
	templates.ChatPage("/p", "s1", sessions, nil).Render(context.Background(), w)
	body := w.Body.String()
	if !strings.Contains(body, `type="button"`) {
		t.Error("send button must be type='button' to avoid native form submission on iOS Safari")
	}
	if strings.Contains(body, `type="submit"`) {
		t.Error("no buttons should be type='submit' — causes iOS Safari form submission issues")
	}
}

func TestChatPage_SendButtonNoInlineOnclick(t *testing.T) {
	// The send button must NOT have an inline onclick — it causes
	// "sendMessage is not defined" on iOS Safari if app.js hasn't
	// finished executing yet. The click handler is added via addEventListener.
	w := httptest.NewRecorder()
	sessions := []templates.SessionInfo{{ID: "s1", Title: "T", State: "idle", MsgCount: 0, Active: true}}
	templates.ChatPage("/p", "s1", sessions, nil).Render(context.Background(), w)
	body := w.Body.String()
	if strings.Contains(body, `onclick="sendMessage()"`) {
		t.Error("send button must not have inline onclick — use addEventListener instead")
	}
}

func TestChatPage_TextareaNotRequired(t *testing.T) {
	// The textarea must NOT have the 'required' attribute — it prevents
	// programmatic form handling and shows native validation popups on iOS.
	w := httptest.NewRecorder()
	sessions := []templates.SessionInfo{{ID: "s1", Title: "T", State: "idle", MsgCount: 0, Active: true}}
	templates.ChatPage("/p", "s1", sessions, nil).Render(context.Background(), w)
	body := w.Body.String()
	if strings.Contains(body, "required") {
		t.Error("textarea must not have 'required' attribute — JS handles validation")
	}
}

func TestRequireAuth_ReturnsJSONForAPIRequests(t *testing.T) {
	// API requests that fail auth must get 401 JSON, NOT a 302 redirect.
	// A 302 redirect causes fetch() to silently follow to /login, get HTML,
	// and then .json() throws — the user sees nothing or gets "kicked out".
	s := newTestServer(t)
	apiPaths := []string{
		"/api/chat/send",
		"/api/chat/stream",
		"/api/sessions/list",
		"/api/sessions/create",
		"/api/panel/analytics",
		"/api/autocomplete/commands",
	}
	for _, path := range apiPaths {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		s.mux.ServeHTTP(w, req)
		if w.Code == http.StatusSeeOther || w.Code == http.StatusFound {
			t.Errorf("%s: API endpoint must return 401, not redirect (got %d)", path, w.Code)
		}
		if w.Code != http.StatusUnauthorized {
			// POST endpoints may return 405 for GET — that's fine, but not 302
			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s: expected 401 for unauthenticated API request, got %d", path, w.Code)
			}
		}
	}
}

func TestRequireAuth_RedirectsPageRequests(t *testing.T) {
	// Page requests (non-API) should still redirect to /login.
	s := newTestServer(t)
	for _, path := range []string{"/", "/chat"} {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		s.mux.ServeHTTP(w, req)
		if w.Code != http.StatusSeeOther {
			t.Errorf("%s: page request should redirect to login, got %d", path, w.Code)
		}
	}
}

func TestLoginCookie_SameSiteLax(t *testing.T) {
	// SameSite must be Lax, not Strict. iOS Safari has known bugs where
	// SameSite=Strict cookies are not sent with fetch() POST requests.
	s := newTestServer(t)
	form := url.Values{"password": {"testpass"}}
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	cookies := w.Result().Cookies()
	for _, c := range cookies {
		if c.Name == "claudio_token" {
			if c.SameSite == http.SameSiteStrictMode {
				t.Error("cookie must use SameSite=Lax, not Strict — Strict breaks fetch() on iOS Safari")
			}
			return
		}
	}
	t.Error("expected claudio_token cookie after login")
}

func TestStaticJS_FetchChecksResponseOk(t *testing.T) {
	// All fetch() calls must check response.ok or response.status before
	// calling .json() — otherwise a 401/redirect produces a silent failure.
	js := appJSContent
	// The JS should have a helper or inline checks for response status
	if !strings.Contains(js, "response.ok") && !strings.Contains(js, ".ok)") && !strings.Contains(js, "checkAuth") {
		t.Error("JS fetch calls must check response.ok to detect auth failures")
	}
}

func TestStaticJS_SendBtnHasClickHandler(t *testing.T) {
	// Verify send-btn has a click addEventListener in JS as primary handler
	js := appJSContent
	if !strings.Contains(js, "'send-btn'") && !strings.Contains(js, `"send-btn"`) {
		t.Error("JS must reference send-btn element")
	}
}

func TestStaticCSS_MobileSidebarTouchAction(t *testing.T) {
	// Mobile menu button and sidebar items must have touch-action: manipulation
	css := cssContent
	if !strings.Contains(css, "touch-action: manipulation") {
		t.Error("interactive elements must have touch-action: manipulation for iOS")
	}
}

func TestStaticCSS_SessionItemTouchFriendly(t *testing.T) {
	// Session items must be large enough to tap on iOS (min-height 44px)
	css := cssContent
	if !strings.Contains(css, "session-item") {
		t.Error("session-item must exist in CSS")
	}
	// The mobile media query must make buttons at least 44px
	if !strings.Contains(css, "min-height: 44px") {
		t.Error("mobile interactive elements must have min-height: 44px for touch targets")
	}
}

func TestStaticJS_NoCacheHeader(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest("GET", "/static/app.js", nil)
	w := httptest.NewRecorder()
	s.handleStatic(w, req)
	cc := w.Header().Get("Cache-Control")
	if !strings.Contains(cc, "no-cache") {
		t.Errorf("static files must have no-cache to prevent stale JS on redeploy, got: %s", cc)
	}
}

func TestStaticCSS_MobileViewport(t *testing.T) {
	css := cssContent
	// Must use dvh for proper iOS Safari viewport handling
	if !strings.Contains(css, "100dvh") {
		t.Error("CSS must use 100dvh for proper iOS Safari viewport height")
	}
	// Must have safe-area-inset handling
	if !strings.Contains(css, "safe-area-inset") {
		t.Error("CSS must handle safe-area-insets for notched devices")
	}
	// Must have touch-action on interactive elements
	if !strings.Contains(css, "touch-action") {
		t.Error("CSS must use touch-action: manipulation to eliminate iOS tap delay")
	}
}

func TestStaticCSS_iPhone15ProBreakpoint(t *testing.T) {
	// iPhone 15 Pro is 393x852 logical pixels — must be covered by mobile breakpoint
	css := cssContent
	if !strings.Contains(css, "@media (max-width: 768px)") {
		t.Error("CSS must have mobile breakpoint at 768px to cover iPhone 15 Pro (393px wide)")
	}
}

func TestStaticCSS_VisualViewportHandling(t *testing.T) {
	// When iOS keyboard opens, the visual viewport shrinks.
	// CSS or JS must handle this to keep the input bar visible.
	js := appJSContent
	if !strings.Contains(js, "visualViewport") {
		t.Error("JS must listen to visualViewport resize to handle iOS keyboard")
	}
}

func TestStaticJS_NoDoubleBackslashInRegex(t *testing.T) {
	// Go backtick strings don't process escape sequences, so \\ in the source
	// becomes literal \\ in the browser. Regex patterns in backtick strings
	// must use single \ (e.g. /\s/ not /\\s/).
	js := appJSContent
	// These patterns would be wrong — they'd match literal backslash + char
	badPatterns := []string{
		`/^\\\/`,     // should be /^\/ 
		`[^\\s]`,     // should be [^\s] (in regex context)
		`/\\s+/`,     // should be /\s+/
		`/\\*\\*`,    // should be /\*\*
		`/\\n/g`,     // should be /\n/g
	}
	for _, bad := range badPatterns {
		if strings.Contains(js, bad) {
			t.Errorf("JS contains double-backslash pattern %q which produces wrong regex in browser", bad)
		}
	}
}

func TestStaticJS_SendMessageDirect(t *testing.T) {
	// sendMessage must be callable without arguments (not dependent on form submit event)
	js := appJSContent
	if !strings.Contains(js, "window.sendMessage=function()") {
		t.Error("sendMessage must take no arguments — not depend on form submit event")
	}
}

func TestStaticJS_SendButtonOnClick(t *testing.T) {
	// The send button must have a direct click handler in JS
	js := appJSContent
	if !strings.Contains(js, "send-btn") || !strings.Contains(js, "addEventListener") {
		t.Error("send button must have addEventListener click handler in JS")
	}
}

func TestStaticJS_AutocompleteUsesTouchEvents(t *testing.T) {
	// Autocomplete popup items must handle touch events for iOS
	js := appJSContent
	if !strings.Contains(js, "touchend") {
		t.Error("autocomplete must handle touchend events for iOS Safari")
	}
}

func TestStaticCSS_SidebarOverlayInMediaQuery(t *testing.T) {
	// sidebar-overlay.visible must have display:block in mobile media query
	css := cssContent
	if !strings.Contains(css, "sidebar-overlay.visible") {
		t.Error("sidebar-overlay.visible must be styled to display:block")
	}
}

func TestStaticCSS_ChatLayoutFullHeight(t *testing.T) {
	css := cssContent
	// chat-layout must fill full height
	if !strings.Contains(css, "chat-layout") {
		t.Error("chat-layout class must exist")
	}
	// chat-main-wrap must use flex to fill remaining space
	if !strings.Contains(css, "chat-main-wrap") {
		t.Error("chat-main-wrap must exist for flex layout")
	}
}

func TestPanel_AnalyticsTemplate(t *testing.T) {
	w := httptest.NewRecorder()
	data := templates.PanelData{
		InputTokens:  5000,
		OutputTokens: 2000,
		TotalTokens:  7000,
		CacheRead:    1000,
		CacheCreate:  500,
		Cost:         "$0.12",
	}
	templates.AnalyticsPanel(data).Render(context.Background(), w)
	body := w.Body.String()
	if !strings.Contains(body, "5000") {
		t.Error("should contain input tokens")
	}
	if !strings.Contains(body, "$0.12") {
		t.Error("should contain cost")
	}
}

func TestPanel_ConfigTemplate(t *testing.T) {
	w := httptest.NewRecorder()
	data := templates.PanelData{
		Model:          "claude-opus-4-6",
		PermissionMode: "default",
		ProjectPath:    "/home/user/project",
	}
	templates.ConfigPanel(data).Render(context.Background(), w)
	body := w.Body.String()
	if !strings.Contains(body, "claude-opus-4-6") {
		t.Error("should contain model name")
	}
	if !strings.Contains(body, "/home/user/project") {
		t.Error("should contain project path")
	}
}

func TestPanel_TasksTemplate(t *testing.T) {
	w := httptest.NewRecorder()
	data := templates.PanelData{
		Tasks: []templates.TaskInfo{
			{ID: "1", Title: "Build web UI", Status: "done"},
			{ID: "2", Title: "Fix bug", Status: "running", Description: "Important fix"},
		},
	}
	templates.TasksPanel(data).Render(context.Background(), w)
	body := w.Body.String()
	if !strings.Contains(body, "Build web UI") {
		t.Error("should contain task title")
	}
	if !strings.Contains(body, "Important fix") {
		t.Error("should contain task description")
	}
}

func TestSSEStream_TracksAnalytics(t *testing.T) {
	s := newTestServer(t)
	tmpDir := t.TempDir()

	sess, err := s.sessions.GetOrCreateDefault(tmpDir)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	handler := NewWebHandler()
	sess.mu.Lock()
	sess.currentHandler = handler
	sess.mu.Unlock()

	// Push events including usage data in done
	go func() {
		handler.OnTextDelta("response")
		handler.OnTurnComplete(api.Usage{InputTokens: 100, OutputTokens: 50})
	}()

	req := httptest.NewRequest("GET", "/api/chat/stream?session="+sess.ID, nil)
	w := httptest.NewRecorder()
	s.handleChatStream(w, req)

	// Check analytics were tracked
	sess.mu.Lock()
	in := sess.TotalInputTokens
	out := sess.TotalOutputTokens
	sess.mu.Unlock()

	if in != 100 {
		t.Errorf("expected TotalInputTokens=100, got %d", in)
	}
	if out != 50 {
		t.Errorf("expected TotalOutputTokens=50, got %d", out)
	}
}
