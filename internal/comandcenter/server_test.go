package comandcenter

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestServer(t *testing.T, password string) *Server {
	t.Helper()
	s := newTestStorage(t)
	hub := NewHub(s)
	return NewServer(password, s, hub, t.TempDir())
}

func TestServer_Health(t *testing.T) {
	srv := newTestServer(t, "secret")

	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("health: got %d, want 200", rec.Code)
	}
}

func TestServer_Auth_Missing(t *testing.T) {
	srv := newTestServer(t, "secret")

	req := httptest.NewRequest("GET", "/api/sessions", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("missing auth: got %d, want 401", rec.Code)
	}
}

func TestServer_Auth_Wrong(t *testing.T) {
	srv := newTestServer(t, "secret")

	req := httptest.NewRequest("GET", "/api/sessions", nil)
	req.Header.Set("Authorization", "Bearer wrong-password")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("wrong auth: got %d, want 401", rec.Code)
	}
}

func TestServer_Auth_Valid(t *testing.T) {
	srv := newTestServer(t, "secret")

	req := httptest.NewRequest("GET", "/api/sessions", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("valid auth: got %d, want 200", rec.Code)
	}
}

func TestServer_ListSessions_Empty(t *testing.T) {
	srv := newTestServer(t, "secret")

	req := httptest.NewRequest("GET", "/api/sessions", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}

	var result []Session
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty array, got %d elements", len(result))
	}
}

func TestServer_ListSessions_NonEmpty(t *testing.T) {
	s := newTestStorage(t)
	hub := NewHub(s)
	srv := NewServer("secret", s, hub, t.TempDir())

	_ = s.UpsertSession(Session{
		ID: "s1", Name: "alpha", Path: "/tmp", Status: "active",
		CreatedAt: time.Now(), LastActiveAt: time.Now(),
	})

	req := httptest.NewRequest("GET", "/api/sessions", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	var result []Session
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 session, got %d", len(result))
	}
}

func TestServer_ListMessages(t *testing.T) {
	s := newTestStorage(t)
	hub := NewHub(s)
	srv := NewServer("secret", s, hub, t.TempDir())

	_ = s.UpsertSession(Session{
		ID: "s2", Name: "b", Path: "/tmp", Status: "active",
		CreatedAt: time.Now(), LastActiveAt: time.Now(),
	})
	_ = s.InsertMessage(Message{
		ID: "m1", SessionID: "s2", Role: "assistant",
		Content: "hi", CreatedAt: time.Now(),
	})

	req := httptest.NewRequest("GET", "/api/sessions/s2/messages?limit=10", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	var msgs []Message
	if err := json.NewDecoder(rec.Body).Decode(&msgs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(msgs) != 1 {
		t.Errorf("expected 1 message, got %d", len(msgs))
	}
}



func TestServer_ContentType_JSON(t *testing.T) {
	srv := newTestServer(t, "secret")

	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type: got %q, want application/json", ct)
	}
}


