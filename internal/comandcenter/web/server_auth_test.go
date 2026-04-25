package web_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	cc "github.com/Abraxas-365/claudio/internal/comandcenter"
	"github.com/Abraxas-365/claudio/internal/comandcenter/web"
)

// loginAndGetCookies POSTs valid credentials and returns the auth cookie + CSRF token from response.
func loginAndGetCookies(t *testing.T, mux *http.ServeMux) *http.Cookie {
	t.Helper()
	form := url.Values{"password": {testPassword}}
	r := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Set remote addr to non-localhost so auth middleware runs.
	r.RemoteAddr = "10.0.0.1:12345"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("login want 303, got %d", w.Code)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == "auth" {
			return c
		}
	}
	t.Fatal("no auth cookie in login response")
	return nil
}

// TestSessionToken_GeneratedOnLogin verifies login sets a session token cookie (not the raw password).
func TestSessionToken_GeneratedOnLogin(t *testing.T) {
	_, mux := newTestEnv(t)
	c := loginAndGetCookies(t, mux)
	if c.Value == testPassword {
		t.Fatal("auth cookie contains raw password, expected session token")
	}
	if len(c.Value) < 64 {
		t.Fatalf("session token too short: %q (want 64 hex chars)", c.Value)
	}
	if !c.HttpOnly {
		t.Fatal("auth cookie should be HttpOnly")
	}
}

// TestSessionToken_InvalidOnWrongPassword verifies wrong password → 401, no cookie.
func TestSessionToken_InvalidOnWrongPassword(t *testing.T) {
	_, mux := newTestEnv(t)
	form := url.Values{"password": {"wrongpassword"}}
	r := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.RemoteAddr = "10.0.0.1:12345"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == "auth" {
			t.Fatal("auth cookie should not be set on failed login")
		}
	}
}

// TestSessionMiddleware_ValidToken verifies valid session cookie → 200.
func TestSessionMiddleware_ValidToken(t *testing.T) {
	_, mux := newTestEnv(t)
	c := loginAndGetCookies(t, mux)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(c)
	r.RemoteAddr = "10.0.0.1:12345"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
}

// TestSessionMiddleware_InvalidToken verifies random cookie → redirect to /login.
func TestSessionMiddleware_InvalidToken(t *testing.T) {
	_, mux := newTestEnv(t)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: "auth", Value: "deadbeefdeadbeefdeadbeef"})
	r.RemoteAddr = "10.0.0.1:12345"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("want 303 redirect, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/login" {
		t.Fatalf("want redirect to /login, got %q", loc)
	}
}

// TestSessionMiddleware_ExpiredToken verifies expired token → redirect to /login.
func TestSessionMiddleware_ExpiredToken(t *testing.T) {
	storage, err := cc.Open(":memory:")
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	if err := storage.ExecRaw(`CREATE TABLE IF NOT EXISTS team_tasks (
		id TEXT NOT NULL, session_id TEXT NOT NULL, title TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'pending',
		assigned_to TEXT NOT NULL DEFAULT '', created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY (id, session_id)
	)`); err != nil {
		t.Fatalf("create team_tasks: %v", err)
	}
	t.Cleanup(func() { storage.Close() })

	hub := cc.NewHub(storage)
	ws := web.NewWebServer(storage, hub, testPassword, "")
	mux := http.NewServeMux()
	ws.RegisterRoutes(mux)

	// Login to get a token.
	c := loginAndGetCookies(t, mux)

	// Expire the token by setting TTL to -1h via the exported test helper.
	ws.ExpireSessionForTest(c.Value, time.Now().Add(-1*time.Hour))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(c)
	r.RemoteAddr = "10.0.0.1:12345"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("want 303 redirect for expired token, got %d", w.Code)
	}
}

// TestCSRF_MissingToken verifies POST without CSRF token → 403.
func TestCSRF_MissingToken(t *testing.T) {
	_, mux := newTestEnv(t)
	c := loginAndGetCookies(t, mux)

	// Seed a session for the message endpoint.
	r := httptest.NewRequest(http.MethodPost, "/api/sessions/test-sess/message", strings.NewReader("content=hello"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(c)
	r.RemoteAddr = "10.0.0.1:12345"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403 for missing CSRF, got %d", w.Code)
	}
}

// TestCSRF_ValidToken verifies POST with valid CSRF token passes middleware.
func TestCSRF_ValidToken(t *testing.T) {
	storage, err := cc.Open(":memory:")
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	if err := storage.ExecRaw(`CREATE TABLE IF NOT EXISTS team_tasks (
		id TEXT NOT NULL, session_id TEXT NOT NULL, title TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'pending',
		assigned_to TEXT NOT NULL DEFAULT '', created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY (id, session_id)
	)`); err != nil {
		t.Fatalf("create team_tasks: %v", err)
	}
	t.Cleanup(func() { storage.Close() })

	hub := cc.NewHub(storage)
	ws := web.NewWebServer(storage, hub, testPassword, "")
	mux := http.NewServeMux()
	ws.RegisterRoutes(mux)

	// Login.
	c := loginAndGetCookies(t, mux)

	// Get CSRF token.
	csrf := ws.CSRFToken(&http.Request{Header: http.Header{"Cookie": []string{c.String()}}})
	if csrf == "" {
		t.Fatal("CSRF token empty")
	}

	// Seed a session so the handler doesn't 404 before we can check CSRF passed.
	if err := storage.UpsertSession(cc.Session{
		ID: "test-sess", Name: "TestAgent", Path: "/tmp",
		Status: "active", CreatedAt: time.Now(), LastActiveAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	// POST with CSRF token in header.
	form := url.Values{"content": {"hello"}}
	r := httptest.NewRequest(http.MethodPost, "/api/sessions/test-sess/message", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("X-CSRF-Token", csrf)
	r.AddCookie(c)
	r.RemoteAddr = "10.0.0.1:12345"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	// Should NOT be 403. Might be 4xx/5xx for other reasons (no hub connection, etc.)
	// but must not be 403 forbidden.
	if w.Code == http.StatusForbidden {
		t.Fatalf("got 403 with valid CSRF token — CSRF check incorrectly rejected")
	}
}

// TestCSP_HeaderPresent verifies CSP header is set on responses.
func TestCSP_HeaderPresent(t *testing.T) {
	_, mux := newTestEnv(t)
	c := loginAndGetCookies(t, mux)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(c)
	r.RemoteAddr = "10.0.0.1:12345"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	csp := w.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("Content-Security-Policy header missing")
	}
	if !strings.Contains(csp, "frame-ancestors") {
		t.Fatalf("CSP missing frame-ancestors: %q", csp)
	}
	if !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Fatalf("CSP missing frame-ancestors: %q", csp)
	}
}
