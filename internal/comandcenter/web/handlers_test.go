package web_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	cc "github.com/Abraxas-365/claudio/internal/comandcenter"
	"github.com/Abraxas-365/claudio/internal/tasks"
)

// TestHandleSessionList_ReturnsHTMLFragment verifies GET /partials/sessions with HX-Request
// header returns 200 with an HTML response (not JSON).
func TestHandleSessionList_ReturnsHTMLFragment(t *testing.T) {
	_, mux := newTestEnv(t)

	r := authedRequest(http.MethodGet, "/partials/sessions")
	r.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d\nbody: %s", w.Code, w.Body.String())
	}
	ct := w.Header().Get("Content-Type")
	if strings.Contains(ct, "application/json") {
		t.Fatalf("want HTML fragment, got Content-Type: %s", ct)
	}
}

// TestHandleSessionList_ReturnsFullPage verifies GET / (chat list) returns 200 with
// a full page layout containing the app shell.
func TestHandleSessionList_ReturnsFullPage(t *testing.T) {
	_, mux := newTestEnv(t)

	r := authedRequest(http.MethodGet, "/")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d\nbody: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "ComandCenter") {
		t.Error("full page response missing 'ComandCenter' heading")
	}
	// Full page must include the HTML document skeleton.
	if !strings.Contains(body, "<html") && !strings.Contains(body, "<!DOCTYPE") {
		t.Error("full page response missing HTML document structure")
	}
}

// TestHandleDeleteSession_RemovesSession verifies DELETE /api/sessions/{id} returns 200
// and the session is gone from storage.
func TestHandleDeleteSession_RemovesSession(t *testing.T) {
	storage, mux := newTestEnv(t)

	if err := storage.UpsertSession(cc.Session{
		ID:           "hdel-sess-1",
		Name:         "ToDelete",
		Path:         "/tmp",
		Status:       "active",
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	r := authedRequest(http.MethodDelete, "/api/sessions/hdel-sess-1")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d\nbody: %s", w.Code, w.Body.String())
	}

	_, err := storage.GetSession("hdel-sess-1")
	if err == nil {
		t.Error("session still exists after delete")
	}
}

// TestHandleArchiveSession_TogglesArchive verifies PATCH /api/sessions/{id}/archive returns 200
// and the session status is updated to archived.
func TestHandleArchiveSession_TogglesArchive(t *testing.T) {
	storage, mux := newTestEnv(t)

	if err := storage.UpsertSession(cc.Session{
		ID:           "harch-sess-1",
		Name:         "ToArchive",
		Path:         "/tmp",
		Status:       "active",
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	r := authedRequest(http.MethodPatch, "/api/sessions/harch-sess-1/archive")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d\nbody: %s", w.Code, w.Body.String())
	}

	got, err := storage.GetSession("harch-sess-1")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.Status != "archived" {
		t.Errorf("want status 'archived', got %q", got.Status)
	}
}

// TestHandleSendMessage_RequiresCSRF verifies POST /api/sessions/{id}/message without
// a CSRF token returns 403.
func TestHandleSendMessage_RequiresCSRF(t *testing.T) {
	_, mux := newTestEnv(t)

	// Use cookie auth from a non-localhost addr so CSRF is enforced.
	c := loginAndGetCookies(t, mux)

	form := url.Values{"content": {"hello"}}
	r := httptest.NewRequest(http.MethodPost, "/api/sessions/any-sess/message", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(c)
	r.RemoteAddr = "10.0.0.1:9999"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403 for missing CSRF, got %d", w.Code)
	}
}

// TestHandleCronList_RendersTempl verifies GET /chat/{session_id}/crons when no CronStore
// is configured renders the CronNotConfigured component (200, HTML).
func TestHandleCronList_RendersTempl(t *testing.T) {
	storage, mux := newTestEnv(t)

	if err := storage.UpsertSession(cc.Session{
		ID:           "cron-sess-1",
		Name:         "CronAgent",
		Path:         "/tmp",
		Status:       "active",
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	r := authedRequest(http.MethodGet, "/chat/cron-sess-1/crons")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	// Should return 200 with HTML (CronNotConfigured templ component).
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d\nbody: %s", w.Code, w.Body.String())
	}
	ct := w.Header().Get("Content-Type")
	if strings.Contains(ct, "application/json") {
		t.Fatalf("cron list returned JSON, want HTML: %s", ct)
	}
}

// TestHandleAPISessions_ReturnsJSON verifies GET /api/sessions/list returns JSON array.
func TestHandleAPISessions_ReturnsJSON(t *testing.T) {
	storage, mux := newTestEnv(t)

	if err := storage.UpsertSession(cc.Session{
		ID:           "api-sess-1",
		Name:         "SessionA",
		Path:         "/tmp",
		Status:       "active",
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	r := authedRequest(http.MethodGet, "/api/sessions/list")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Fatalf("want application/json Content-Type, got %q", ct)
	}
	var sessions []map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&sessions); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
}

// TestHandlePartialMessages_ReturnsHTML verifies GET /partials/messages/{session_id}
// for a known session returns 200 with HTML.
func TestHandlePartialMessages_ReturnsHTML(t *testing.T) {
	storage, mux := newTestEnv(t)

	if err := storage.UpsertSession(cc.Session{
		ID:           "pmsg-sess-1",
		Name:         "MsgAgent",
		Path:         "/tmp",
		Status:       "active",
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	r := authedRequest(http.MethodGet, "/partials/messages/pmsg-sess-1")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d\nbody: %s", w.Code, w.Body.String())
	}
	ct := w.Header().Get("Content-Type")
	if strings.Contains(ct, "application/json") {
		t.Fatalf("expected HTML response, got Content-Type: %s", ct)
	}
}

// TestHandleSessionLookupByName_NotFound verifies GET /api/session-lookup/{name}
// for an unknown name returns 404.
func TestHandleSessionLookupByName_NotFound(t *testing.T) {
	_, mux := newTestEnv(t)

	r := authedRequest(http.MethodGet, "/api/session-lookup/doesnotexist")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

// TestHandleCronList_WithEntries verifies GET /chat/{session_id}/crons with a real
// seeded CronStore renders 200 HTML containing the cron entry data.
func TestHandleCronList_WithEntries(t *testing.T) {
	storage, ws, mux := newFullTestEnv(t)

	const sessID = "cron-entries-sess-1"
	if err := storage.UpsertSession(cc.Session{
		ID:           sessID,
		Name:         "CronEntryAgent",
		Path:         "/tmp",
		Status:       "active",
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	// Build a CronStore backed by a temp file so Add/Save works.
	cs := tasks.NewCronStore(filepath.Join(t.TempDir(), "crons.json"))
	entry, err := cs.Add("@daily", "run daily backup", "", "inline", sessID)
	if err != nil {
		t.Fatalf("add cron entry: %v", err)
	}
	ws.SetCronStore(cs)

	r := authedRequest(http.MethodGet, "/chat/"+sessID+"/crons")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d\nbody: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, "Cron store not configured") {
		t.Fatal("CronStore not attached: got 'not configured' response")
	}
	if strings.Contains(body, "No scheduled tasks") {
		t.Fatal("cron entries not found in response: got 'no scheduled tasks'")
	}
	if !strings.Contains(body, entry.ID) {
		t.Errorf("response missing cron entry ID %q\nbody: %s", entry.ID, body)
	}
	if !strings.Contains(body, "run daily backup") {
		t.Errorf("response missing cron entry prompt\nbody: %s", body)
	}
}
