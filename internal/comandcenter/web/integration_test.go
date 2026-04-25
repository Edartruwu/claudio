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

// newFullTestEnv is like newTestEnv but also returns the WebServer so callers can
// access CSRFToken() and ExpireSessionForTest(). All tests that need the real
// cookie-based auth flow (non-localhost) must use this helper.
func newFullTestEnv(t *testing.T) (*cc.Storage, *web.WebServer, *http.ServeMux) {
	t.Helper()
	storage, err := cc.Open(":memory:")
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	if err := storage.ExecRaw(`CREATE TABLE IF NOT EXISTS team_tasks (
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
	t.Cleanup(func() { storage.Close() })

	hub := cc.NewHub(storage)
	ws := web.NewWebServer(storage, hub, testPassword, "")
	mux := http.NewServeMux()
	ws.RegisterRoutes(mux)
	return storage, ws, mux
}

// TestFullLoginFlow_CookieToken verifies POST /login with the correct password sets
// an auth cookie whose value is a session token, not the raw password.
func TestFullLoginFlow_CookieToken(t *testing.T) {
	_, _, mux := newFullTestEnv(t)

	form := url.Values{"password": {testPassword}}
	r := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.RemoteAddr = "10.0.0.1:9999"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("want 303 after login, got %d", w.Code)
	}
	var authCookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "auth" {
			authCookie = c
			break
		}
	}
	if authCookie == nil {
		t.Fatal("auth cookie not set after login")
	}
	if authCookie.Value == testPassword {
		t.Fatal("auth cookie contains raw password — must be session token")
	}
	if len(authCookie.Value) < 64 {
		t.Fatalf("session token too short (%d chars), want ≥64 hex chars", len(authCookie.Value))
	}
}

// TestFullLoginFlow_AuthenticatedAccess verifies that GET / with a valid auth cookie
// returns 200 (not a redirect).
func TestFullLoginFlow_AuthenticatedAccess(t *testing.T) {
	_, _, mux := newFullTestEnv(t)

	c := loginAndGetCookies(t, mux)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(c)
	r.RemoteAddr = "10.0.0.1:9999"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 with valid cookie, got %d", w.Code)
	}
}

// TestFullLoginFlow_UnauthenticatedRedirect verifies that GET / without a cookie
// redirects to /login.
func TestFullLoginFlow_UnauthenticatedRedirect(t *testing.T) {
	_, _, mux := newFullTestEnv(t)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.1:9999"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("want 303 redirect, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/login" {
		t.Fatalf("want redirect to /login, got %q", loc)
	}
}

// TestFullLoginFlow_CSRFRequired verifies that a state-changing POST without a CSRF
// token returns 403, even with a valid auth cookie.
func TestFullLoginFlow_CSRFRequired(t *testing.T) {
	_, _, mux := newFullTestEnv(t)

	c := loginAndGetCookies(t, mux)

	form := url.Values{"content": {"hello"}}
	r := httptest.NewRequest(http.MethodPost, "/api/sessions/any-session/message",
		strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(c)
	r.RemoteAddr = "10.0.0.1:9999"
	// No X-CSRF-Token header.
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403 for missing CSRF, got %d", w.Code)
	}
}

// TestFullLoginFlow_CSRFValidPasses verifies that a state-changing POST with a valid
// CSRF token is NOT rejected with 403 by the middleware.
func TestFullLoginFlow_CSRFValidPasses(t *testing.T) {
	storage, ws, mux := newFullTestEnv(t)

	c := loginAndGetCookies(t, mux)

	// Retrieve CSRF token for this session.
	csrf := ws.CSRFToken(&http.Request{Header: http.Header{"Cookie": []string{c.String()}}})
	if csrf == "" {
		t.Fatal("CSRF token empty after login")
	}

	// Seed a session so the message handler can look it up.
	if err := storage.UpsertSession(cc.Session{
		ID:           "flow-sess-1",
		Name:         "FlowAgent",
		Path:         "/tmp",
		Status:       "active",
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	form := url.Values{"content": {"hello"}}
	r := httptest.NewRequest(http.MethodPost, "/api/sessions/flow-sess-1/message",
		strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("X-CSRF-Token", csrf)
	r.AddCookie(c)
	r.RemoteAddr = "10.0.0.1:9999"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	// Must not be 403 — CSRF passed. Any other status is fine (e.g. hub error).
	if w.Code == http.StatusForbidden {
		t.Fatal("valid CSRF token rejected with 403 — CSRF middleware is broken")
	}
}
