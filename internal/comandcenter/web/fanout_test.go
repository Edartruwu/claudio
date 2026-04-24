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
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("unmarshal JSON: %v (raw: %s)", err, raw)
		}
		if m["type"] != "agent_status" {
			t.Errorf("type: want %q, got %v", "agent_status", m["type"])
		}
		if m["name"] != wantName {
			t.Errorf("name: want %q, got %v", wantName, m["name"])
		}
		if m["status"] != wantStatus {
			t.Errorf("status: want %q, got %v", wantStatus, m["status"])
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

// TestHub_SessionIsolation verifies that a Broadcast for session A does not deliver
// any payload to a UI client registered for session B.
func TestHub_SessionIsolation(t *testing.T) {
	ws, hub := newFanoutServer(t)

	clientA := registerFanoutClient(ws, "sess-iso-A")
	clientB := registerFanoutClient(ws, "sess-iso-B")

	// Broadcast only to session A.
	broadcastAgentStatus(t, hub, "sess-iso-A", "agent-alpha", "done")

	// Session A client should receive the event.
	assertAgentStatusJSON(t, clientA, "agent-alpha", "done")

	// Session B client must receive nothing.
	select {
	case raw := <-clientB.send:
		t.Errorf("session B received unexpected payload: %s", raw)
	case <-time.After(300 * time.Millisecond):
		// correct — nothing delivered to B
	}
}

// TestHub_ReconnectReplaysAgentEvents verifies that a persisted terminal agent event
// is replayed to a newly connecting UI client.
// This exercises the GetLatestAgentEvents → Envelope serialization pipeline used in
// handleWSUI (server.go) when a browser reconnects.
func TestHub_ReconnectReplaysAgentEvents(t *testing.T) {
	storage, err := cc.Open(":memory:")
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { storage.Close() })

	const sessionID = "sess-replay"
	const agentName = "replay-agent"
	const agentStatus = "done"

	// Persist a terminal agent event (as hub.processEvent does on EventAgentStatus).
	payloadJSON, _ := json.Marshal(attach.AgentStatusPayload{
		Name:   agentName,
		Status: agentStatus,
	})
	if err := storage.InsertAgentEvent(sessionID, agentName, agentStatus, string(payloadJSON)); err != nil {
		t.Fatalf("InsertAgentEvent: %v", err)
	}

	// Simulate the replay logic from handleWSUI (server.go lines 1200-1213).
	client := &uiClient{
		sessionID: sessionID,
		send:      make(chan []byte, 8),
	}
	events, err := storage.GetLatestAgentEvents(sessionID)
	if err != nil {
		t.Fatalf("GetLatestAgentEvents: %v", err)
	}
	for _, evt := range events {
		env := attach.Envelope{
			Type:    attach.EventAgentStatus,
			Payload: json.RawMessage(evt.Payload),
		}
		data, _ := json.Marshal(env)
		select {
		case client.send <- data:
		default:
		}
	}

	// The client's send channel must have one entry — the replayed event.
	select {
	case raw := <-client.send:
		var env attach.Envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatalf("unmarshal replayed envelope: %v", err)
		}
		if env.Type != attach.EventAgentStatus {
			t.Errorf("replayed env.Type = %q, want %q", env.Type, attach.EventAgentStatus)
		}
		// Verify the payload round-trips the agent name + status.
		var p map[string]string
		if err := json.Unmarshal(env.Payload, &p); err != nil {
			t.Fatalf("unmarshal replayed payload: %v", err)
		}
		if p["name"] != agentName {
			t.Errorf("replayed name = %q, want %q", p["name"], agentName)
		}
		if p["status"] != agentStatus {
			t.Errorf("replayed status = %q, want %q", p["status"], agentStatus)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout: no replayed event delivered to new client")
	}
}
