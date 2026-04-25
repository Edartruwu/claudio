package web_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRoutesRegistered verifies that key routes respond (not 404) after RegisterRoutes.
// This catches accidental route deletions or mis-registrations.
func TestRoutesRegistered(t *testing.T) {
	_, mux := newTestEnv(t)

	// These routes must be registered and return something other than 404.
	// Auth-gated routes redirect to /login (303) for unauthenticated requests —
	// that is also not 404, which is what we verify here.
	// Public routes (/login, /sw.js) return 200.
	routes := []struct {
		method string
		path   string
	}{
		// Public routes — no auth needed.
		{http.MethodGet, "/login"},
		{http.MethodGet, "/sw.js"},
		// Auth-gated routes — will 303 redirect to /login, not 404.
		{http.MethodGet, "/"},
		{http.MethodGet, "/partials/sessions"},
		{http.MethodGet, "/api/sessions/list"},
		{http.MethodGet, "/api/projects"},
		{http.MethodGet, "/api/agents"},
		{http.MethodGet, "/api/teams"},
		{http.MethodGet, "/api/push/vapid-public-key"},
		{http.MethodGet, "/designs"},
	}

	for _, rt := range routes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			r := httptest.NewRequest(rt.method, rt.path, nil)
			// Non-localhost so auth middleware fires and redirects — not bypasses.
			r.RemoteAddr = "10.0.0.1:9999"
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)

			if w.Code == http.StatusNotFound {
				t.Fatalf("route %s %s not registered (got 404)", rt.method, rt.path)
			}
		})
	}
}

// TestStaticRouteRegistered verifies GET /static/ serves files (not 404).
func TestStaticRouteRegistered(t *testing.T) {
	_, mux := newTestEnv(t)

	// /static/ should serve embedded files. A request for the prefix itself
	// may return 301 or 200 but must not be 404.
	r := httptest.NewRequest(http.MethodGet, "/static/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code == http.StatusNotFound {
		t.Fatal("GET /static/ returned 404 — static route not registered")
	}
}
