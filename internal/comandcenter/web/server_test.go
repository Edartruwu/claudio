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

const testPassword = "secret123"

// newTestEnv creates a WebServer backed by an in-memory SQLite database.
func newTestEnv(t *testing.T) (*cc.Storage, *http.ServeMux) {
	t.Helper()
	storage, err := cc.Open(":memory:")
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { storage.Close() })

	hub := cc.NewHub(storage)
	ws := web.NewWebServer(storage, hub, testPassword)
	mux := http.NewServeMux()
	ws.RegisterRoutes(mux)
	return storage, mux
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
