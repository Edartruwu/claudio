package web_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestWebSocketUpgrade_RequiresAuth verifies that GET /ws/ui from a non-localhost
// address without an auth cookie is rejected by the middleware (303 redirect to /login).
func TestWebSocketUpgrade_RequiresAuth(t *testing.T) {
	_, mux := newTestEnv(t)

	r := httptest.NewRequest(http.MethodGet, "/ws/ui", nil)
	r.Header.Set("Upgrade", "websocket")
	r.Header.Set("Connection", "Upgrade")
	// Non-localhost addr: auth middleware must enforce cookie.
	r.RemoteAddr = "10.0.0.1:9999"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	// No auth cookie → middleware redirects to /login.
	if w.Code != http.StatusSeeOther {
		t.Fatalf("want 303 redirect for unauthenticated WS request, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/login" {
		t.Fatalf("want redirect to /login, got %q", loc)
	}
}

// TestWebSocketUpgrade_LocalhostBypasses verifies that GET /ws/ui from 127.0.0.1
// bypasses the auth cookie check. Uses a real httptest.Server so the WS upgrade
// can proceed past the middleware without a Hijacker panic. The response must NOT
// be a 303 auth redirect — any other status (101, 400, etc.) means auth passed.
func TestWebSocketUpgrade_LocalhostBypasses(t *testing.T) {
	_, mux := newTestEnv(t)

	// Real server needed: websocket upgrade requires http.Hijacker which
	// httptest.ResponseRecorder does not implement.
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/ws/ui", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	// The request originates from 127.0.0.1 (httptest.Server binds localhost),
	// so auth middleware must treat it as trusted.

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Connection-level error is fine — upgrade rejected at TCP layer.
		return
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); resp.Body.Close() }()

	// Must NOT redirect to /login — that would mean auth blocked the request.
	if resp.StatusCode == http.StatusSeeOther {
		loc := resp.Header.Get("Location")
		if loc == "/login" {
			t.Fatalf("localhost WS request redirected to /login — auth bypass not working (status %d)", resp.StatusCode)
		}
	}
}
