package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	cc "github.com/Abraxas-365/claudio/internal/comandcenter"
)

// newAgentLogsServer creates a minimal WebServer with in-memory storage for
// testing the handleAgentLogs handler.  The fanout goroutine is started via
// the embedded done channel so Close() works normally.
func newAgentLogsServer(t *testing.T) (*WebServer, *cc.Storage) {
	t.Helper()
	storage, err := cc.Open(":memory:")
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { storage.Close() })

	hub := cc.NewHub(storage)
	ws := &WebServer{
		storage: storage,
		hub:     hub,
		clients: make(map[*uiClient]struct{}),
		done:    make(chan struct{}),
	}
	go ws.fanout()
	t.Cleanup(func() { ws.Close() })
	return ws, storage
}

// TestHandleAgentLogs_ReturnsMsgs inserts a cc_messages row directly via
// storage, then calls GET /chat/{sessionID}/agents/rafael/logs via httptest
// and asserts HTTP 200 with the message content in the body.
func TestHandleAgentLogs_ReturnsMsgs(t *testing.T) {
	ws, storage := newAgentLogsServer(t)

	const sessionID = "sess-test-logs"
	const agentName = "rafael"
	const content = "hello from rafael"

	// Seed session row (cc_messages has FK to cc_sessions).
	if err := storage.UpsertSession(cc.Session{
		ID:           sessionID,
		Name:         sessionID,
		Status:       "active",
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	}); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	// Insert message directly — bypasses hub to test handler in isolation.
	if err := storage.InsertMessage(cc.Message{
		ID:        "msg-001",
		SessionID: sessionID,
		Role:      "assistant",
		Content:   content,
		AgentName: agentName,
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("InsertMessage: %v", err)
	}

	// Build request — httptest.NewRecorder simulates 127.0.0.1 remote addr so
	// uiAuth allows it through without a password cookie.
	req := httptest.NewRequest(http.MethodGet, "/chat/"+sessionID+"/agents/"+agentName+"/logs", nil)
	req.SetPathValue("session_id", sessionID)
	req.SetPathValue("agent_name", agentName)
	req.RemoteAddr = "127.0.0.1:12345" // trusted loopback — bypasses uiAuth cookie check

	rr := httptest.NewRecorder()
	ws.handleAgentLogs(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("HTTP status: got %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	body := rr.Body.String()
	if !strings.Contains(body, content) {
		t.Errorf("response body does not contain %q\nbody:\n%s", content, body)
	}
}
