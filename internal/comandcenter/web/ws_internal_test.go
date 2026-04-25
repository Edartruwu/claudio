package web

import (
	"testing"

	cc "github.com/Abraxas-365/claudio/internal/comandcenter"
)

// TestPushToSessionClients_NilSafe verifies that pushToSessionClients with no
// registered clients does not panic.
func TestPushToSessionClients_NilSafe(t *testing.T) {
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

	// Must not panic with no clients registered.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("pushToSessionClients panicked: %v", r)
		}
	}()
	ws.pushToSessionClients("nonexistent-session", []byte(`{"type":"test"}`))
}

// TestPushToSessionClients_WrongSession verifies that pushToSessionClients does not
// deliver payload to a client watching a different session.
func TestPushToSessionClients_WrongSession(t *testing.T) {
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

	// Register a client watching "session-A".
	clientA := &uiClient{
		sessionID: "session-A",
		send:      make(chan []byte, 4),
	}
	ws.mu.Lock()
	ws.clients[clientA] = struct{}{}
	ws.mu.Unlock()

	// Push to "session-B" — clientA must NOT receive it.
	ws.pushToSessionClients("session-B", []byte(`{"type":"test"}`))

	select {
	case msg := <-clientA.send:
		t.Fatalf("clientA (session-A) unexpectedly received message for session-B: %s", msg)
	default:
		// Expected: no message delivered.
	}
}
