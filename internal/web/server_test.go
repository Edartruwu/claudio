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
	})
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
	form := url.Values{"project": {"/tmp"}}
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
	req := httptest.NewRequest("GET", "/api/chat/stream?project=/nonexistent", nil)
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
	_, err := s.sessions.GetOrCreate(tmpDir)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/chat/stream?project="+tmpDir, nil)
	w := httptest.NewRecorder()

	s.handleChatStream(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestToolApproval_NoSession(t *testing.T) {
	s := newTestServer(t)
	form := url.Values{"project": {"/nonexistent"}}
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

func TestSessionManager_GetOrCreate(t *testing.T) {
	sm := NewSessionManager()
	tmpDir := t.TempDir()

	sess1, err := sm.GetOrCreate(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sess1.ProjectPath != tmpDir {
		t.Errorf("expected path %s, got %s", tmpDir, sess1.ProjectPath)
	}

	// Second call should return the same session
	sess2, err := sm.GetOrCreate(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sess1 != sess2 {
		t.Error("expected same session instance")
	}
}

func TestSessionManager_GetOrCreate_InvalidPath(t *testing.T) {
	sm := NewSessionManager()
	_, err := sm.GetOrCreate("/definitely/not/a/real/path")
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

	sm.GetOrCreate(tmpDir)
	sess := sm.Get(tmpDir)
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

	sm.GetOrCreate(dir1)
	sm.GetOrCreate(dir2)

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

	sess1, _ := sm.GetOrCreate(dir1)
	sess2, _ := sm.GetOrCreate(dir2)

	if sess1 == sess2 {
		t.Error("different projects should have different sessions")
	}
}

func TestProjectSession_AddMessage(t *testing.T) {
	sm := NewSessionManager()
	tmpDir := t.TempDir()
	sess, _ := sm.GetOrCreate(tmpDir)

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
	sess, _ := sm.GetOrCreate(tmpDir)

	if h := sess.CurrentHandler(); h != nil {
		t.Error("expected nil handler before any message sent")
	}
}

func TestSSEStream_Integration(t *testing.T) {
	// Test that SSE stream correctly formats events
	s := newTestServer(t)
	tmpDir := t.TempDir()

	// Create session and manually set a handler
	sess, err := s.sessions.GetOrCreate(tmpDir)
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

	req := httptest.NewRequest("GET", "/api/chat/stream?project="+tmpDir, nil)
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

	sess, _ := s.sessions.GetOrCreate(tmpDir)
	handler := NewWebHandler()
	sess.mu.Lock()
	sess.currentHandler = handler
	sess.mu.Unlock()

	go func() {
		handler.OnTextDelta("line1\nline2\nline3")
		handler.OnTurnComplete(api.Usage{})
	}()

	req := httptest.NewRequest("GET", "/api/chat/stream?project="+tmpDir, nil)
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
	templates.ChatPage("/test/project", messages).Render(context.Background(), w)
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
	sess, err := s.sessions.GetOrCreate(tmpDir)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	sess.mu.Lock()
	sess.TotalInputTokens = 1500
	sess.TotalOutputTokens = 800
	sess.CacheReadTokens = 500
	sess.CacheCreateTokens = 200
	sess.mu.Unlock()

	req := httptest.NewRequest("GET", "/api/panel/analytics?project="+tmpDir, nil)
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
	s.sessions.GetOrCreate(tmpDir)

	req := httptest.NewRequest("GET", "/api/panel/config?project="+tmpDir, nil)
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

	sess, err := s.sessions.GetOrCreate(tmpDir)
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

	req := httptest.NewRequest("GET", "/api/chat/stream?project="+tmpDir, nil)
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
