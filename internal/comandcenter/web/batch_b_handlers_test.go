// Package web_test — gap-fill HTTP handler tests for Batch B UX changes.
// Covers: message pagination, settings page, DELETE /api/sessions/all stub.
package web_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	cc "github.com/Abraxas-365/claudio/internal/comandcenter"
)

// ─── Message pagination ───────────────────────────────────────────────────────

// TestHandlePartialMessages_ValidLimit_Returns200 verifies GET with ?limit=50 returns 200.
func TestHandlePartialMessages_ValidLimit_Returns200(t *testing.T) {
	storage, mux := newTestEnv(t)

	// Seed a session so the storage call doesn't short-circuit.
	if err := storage.UpsertSession(cc.Session{
		ID:           "pag-sess-1",
		Name:         "PagAgent",
		Path:         "/tmp",
		Status:       "active",
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	r := authedRequest(http.MethodGet, "/partials/messages/pag-sess-1?limit=50")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d\nbody: %s", w.Code, w.Body.String())
	}
}

// TestHandlePartialMessages_LimitParam_ReturnsHTML verifies the partial returns HTML (not JSON).
func TestHandlePartialMessages_LimitParam_ReturnsHTML(t *testing.T) {
	storage, mux := newTestEnv(t)

	if err := storage.UpsertSession(cc.Session{
		ID:           "pag-sess-html",
		Name:         "PagAgent",
		Path:         "/tmp",
		Status:       "active",
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	r := authedRequest(http.MethodGet, "/partials/messages/pag-sess-html?limit=50")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	ct := w.Header().Get("Content-Type")
	if strings.Contains(ct, "application/json") {
		t.Errorf("want HTML response, got Content-Type: %s", ct)
	}
}

// TestHandlePartialMessages_BeforeUnknownID_Returns200 verifies ?before=nonexistent-id
// returns 200 (empty list) rather than 404.
func TestHandlePartialMessages_BeforeUnknownID_Returns200(t *testing.T) {
	storage, mux := newTestEnv(t)

	if err := storage.UpsertSession(cc.Session{
		ID:           "pag-sess-before",
		Name:         "PagAgent",
		Path:         "/tmp",
		Status:       "active",
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	r := authedRequest(http.MethodGet, "/partials/messages/pag-sess-before?before=nonexistent-id-xyz&limit=50")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 for unknown before= cursor, got %d\nbody: %s", w.Code, w.Body.String())
	}
}

// TestHandlePartialMessages_LimitOverMax_Clamped verifies limit >200 is clamped to 200
// (server still returns 200, not an error).
func TestHandlePartialMessages_LimitOverMax_Clamped(t *testing.T) {
	storage, mux := newTestEnv(t)

	if err := storage.UpsertSession(cc.Session{
		ID:           "pag-sess-max",
		Name:         "PagAgent",
		Path:         "/tmp",
		Status:       "active",
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	// limit=9999 exceeds max 200 — server should cap and return 200.
	r := authedRequest(http.MethodGet, "/partials/messages/pag-sess-max?limit=9999")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 for over-max limit, got %d\nbody: %s", w.Code, w.Body.String())
	}
}

// ─── Settings page ────────────────────────────────────────────────────────────

// TestHandleSettings_Authenticated_Returns200 verifies GET /settings → 200 with HTML.
func TestHandleSettings_Authenticated_Returns200(t *testing.T) {
	_, mux := newTestEnv(t)

	r := authedRequest(http.MethodGet, "/settings")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 for GET /settings, got %d\nbody: %s", w.Code, w.Body.String())
	}
}

// TestHandleSettings_BodyContainsSettingsHeading verifies the settings page contains
// a "Settings" heading in the HTML response.
func TestHandleSettings_BodyContainsSettingsHeading(t *testing.T) {
	_, mux := newTestEnv(t)

	r := authedRequest(http.MethodGet, "/settings")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	body := w.Body.String()
	if !strings.Contains(body, "Settings") {
		t.Errorf("GET /settings: response body missing \"Settings\" heading:\n%s", body[:min2(len(body), 2000)])
	}
}

// TestHandleSettings_Unauthenticated_Redirects verifies GET /settings without auth
// → 303 redirect to /login.
func TestHandleSettings_Unauthenticated_Redirects(t *testing.T) {
	_, mux := newTestEnv(t)

	// Non-localhost address triggers full auth check.
	r := httptest.NewRequest(http.MethodGet, "/settings", nil)
	r.RemoteAddr = "10.0.0.1:12345"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("want 303 redirect for unauthenticated /settings, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/login" {
		t.Fatalf("want redirect to /login, got %q", loc)
	}
}

// ─── DELETE /api/sessions/all stub ───────────────────────────────────────────

// TestHandleDeleteAllSessions_Returns501 verifies DELETE /api/sessions/all → 501
// (stub endpoint, not yet fully implemented).
func TestHandleDeleteAllSessions_Returns501(t *testing.T) {
	_, mux := newTestEnv(t)

	r := authedRequest(http.MethodDelete, "/api/sessions/all")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNotImplemented {
		t.Fatalf("want 501 Not Implemented for DELETE /api/sessions/all, got %d\nbody: %s", w.Code, w.Body.String())
	}
}

// ─── local helpers ────────────────────────────────────────────────────────────

// min2 returns the smaller of a and b.
func min2(a, b int) int {
	if a < b {
		return a
	}
	return b
}
