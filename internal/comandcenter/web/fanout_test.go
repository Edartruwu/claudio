package web

import (
	"encoding/json"
	"testing"
	"time"

	cc "github.com/Abraxas-365/claudio/internal/comandcenter"
	"github.com/Abraxas-365/claudio/internal/attach"
)

// newFanoutServer creates a minimal WebServer with the fanout goroutine running.
// It does NOT go through NewWebServer to avoid registering routes or mixing
// with other test concerns.
func newFanoutServer(t *testing.T) (*WebServer, *cc.Hub) {
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
	}
	go ws.fanout()
	return ws, hub
}

// registerFanoutClient adds a mock UI client watching sessionID to ws.
func registerFanoutClient(ws *WebServer, sessionID string) *uiClient {
	c := &uiClient{
		sessionID: sessionID,
		send:      make(chan []byte, 8),
	}
	ws.mu.Lock()
	ws.clients[c] = struct{}{}
	ws.mu.Unlock()
	return c
}

// broadcastAgentStatus publishes an EventAgentStatus envelope to hub for sessionID.
func broadcastAgentStatus(t *testing.T, hub *cc.Hub, sessionID, name, status string) {
	t.Helper()
	env, err := attach.NewEnvelope(attach.EventAgentStatus, attach.AgentStatusPayload{
		Name:   name,
		Status: status,
	})
	if err != nil {
		t.Fatalf("NewEnvelope: %v", err)
	}
	hub.Broadcast(sessionID, env)
}

// assertAgentStatusJSON reads one payload from the client channel and checks
// that it is valid JSON containing type/name/status fields with the expected values.
func assertAgentStatusJSON(t *testing.T, c *uiClient, wantName, wantStatus string) {
	t.Helper()
	select {
	case raw := <-c.send:
		var m map[string]string
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("unmarshal JSON: %v (raw: %s)", err, raw)
		}
		if m["type"] != "agent_status" {
			t.Errorf("type: want %q, got %q", "agent_status", m["type"])
		}
		if m["name"] != wantName {
			t.Errorf("name: want %q, got %q", wantName, m["name"])
		}
		if m["status"] != wantStatus {
			t.Errorf("status: want %q, got %q", wantStatus, m["status"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: no payload pushed to client within 2s")
	}
}

// TestFanout_EventAgentStatus_PushesJSON_Complete verifies that fanout pushes
// correctly shaped JSON to the registered client when status is "complete".
func TestFanout_EventAgentStatus_PushesJSON_Complete(t *testing.T) {
	ws, hub := newFanoutServer(t)
	client := registerFanoutClient(ws, "sess-complete")
	broadcastAgentStatus(t, hub, "sess-complete", "alex", "complete")
	assertAgentStatusJSON(t, client, "alex", "complete")
}

// TestFanout_EventAgentStatus_PushesJSON_Failed verifies that fanout pushes
// correctly shaped JSON to the registered client when status is "failed".
func TestFanout_EventAgentStatus_PushesJSON_Failed(t *testing.T) {
	ws, hub := newFanoutServer(t)
	client := registerFanoutClient(ws, "sess-failed")
	broadcastAgentStatus(t, hub, "sess-failed", "alex", "failed")
	assertAgentStatusJSON(t, client, "alex", "failed")
}

// TestFanout_EventAgentStatus_PushesJSON_Working verifies that fanout pushes
// correctly shaped JSON to the registered client when status is "working".
func TestFanout_EventAgentStatus_PushesJSON_Working(t *testing.T) {
	ws, hub := newFanoutServer(t)
	client := registerFanoutClient(ws, "sess-working")
	broadcastAgentStatus(t, hub, "sess-working", "alex", "working")
	assertAgentStatusJSON(t, client, "alex", "working")
}
