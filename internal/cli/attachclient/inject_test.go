package attachclient

import (
	"context"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Abraxas-365/claudio/internal/attach"
	"golang.org/x/net/websocket"
)

// TestOnUserMessage_InjectsToSubmitChannel verifies that when a user message
// arrives from ComandCenter, the OnUserMessage callback receives the payload
// with the correct content — validating the injection path.
func TestOnUserMessage_InjectsToSubmitChannel(t *testing.T) {
	const msgContent = "deploy to production"

	var mu sync.Mutex
	var received []string

	handler := websocket.Handler(func(conn *websocket.Conn) {
		defer conn.Close()

		// Read hello
		var hello attach.Envelope
		if err := websocket.JSON.Receive(conn, &hello); err != nil {
			return
		}

		// Send two user messages rapidly
		for _, content := range []string{msgContent, "second message"} {
			env, _ := attach.NewEnvelope(attach.EventMsgUser, attach.UserMsgPayload{
				Content: content,
			})
			if err := websocket.JSON.Send(conn, env); err != nil {
				return
			}
		}

		// Keep alive
		<-time.After(200 * time.Millisecond)
	})

	server := newTestServer(handler)
	defer server.Close()

	client := New(server.URL, "", "test", false)
	client.OnUserMessage(func(p attach.UserMsgPayload) {
		mu.Lock()
		received = append(received, p.Content)
		mu.Unlock()
	})

	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer client.Close()

	// Wait for messages to arrive
	deadline := time.After(500 * time.Millisecond)
	for {
		mu.Lock()
		n := len(received)
		mu.Unlock()
		if n >= 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for messages, got %d", n)
		case <-time.After(5 * time.Millisecond):
		}
	}

	mu.Lock()
	defer mu.Unlock()

	if received[0] != msgContent {
		t.Errorf("first msg: want %q, got %q", msgContent, received[0])
	}
	if received[1] != "second message" {
		t.Errorf("second msg: want %q, got %q", "second message", received[1])
	}
}

// TestOnUserMessage_NonBlockingWhenNoCallback verifies that receiving a user
// message without a registered callback does not block or panic.
func TestOnUserMessage_NonBlockingWhenNoCallback(t *testing.T) {
	handler := websocket.Handler(func(conn *websocket.Conn) {
		defer conn.Close()

		// Read hello
		var hello attach.Envelope
		if err := websocket.JSON.Receive(conn, &hello); err != nil {
			return
		}

		// Send user message with no callback registered
		env, _ := attach.NewEnvelope(attach.EventMsgUser, attach.UserMsgPayload{
			Content: "orphan message",
		})
		_ = websocket.JSON.Send(conn, env)

		<-time.After(100 * time.Millisecond)
	})

	server := newTestServer(handler)
	defer server.Close()

	client := New(server.URL, "", "test", false)
	// Deliberately NOT setting OnUserMessage

	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer client.Close()

	// If readLoop panics or blocks, test will hang/fail
	<-time.After(50 * time.Millisecond)
}

// TestOnUserMessage_CallbackReplaceable verifies that re-registering the
// callback replaces the previous one (supports late wiring in runInteractive).
func TestOnUserMessage_CallbackReplaceable(t *testing.T) {
	var firstCalled, secondCalled bool

	handler := websocket.Handler(func(conn *websocket.Conn) {
		defer conn.Close()

		var hello attach.Envelope
		_ = websocket.JSON.Receive(conn, &hello)

		// Wait for callback to be replaced
		<-time.After(50 * time.Millisecond)

		env, _ := attach.NewEnvelope(attach.EventMsgUser, attach.UserMsgPayload{
			Content: "after replace",
		})
		_ = websocket.JSON.Send(conn, env)

		<-time.After(100 * time.Millisecond)
	})

	server := newTestServer(handler)
	defer server.Close()

	client := New(server.URL, "", "test", false)
	client.OnUserMessage(func(p attach.UserMsgPayload) {
		firstCalled = true
	})

	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer client.Close()

	// Replace callback (simulates runInteractive wiring after PersistentPreRunE)
	client.OnUserMessage(func(p attach.UserMsgPayload) {
		secondCalled = true
	})

	<-time.After(200 * time.Millisecond)

	if firstCalled {
		t.Error("first callback should not have been called after replacement")
	}
	if !secondCalled {
		t.Error("second (replaced) callback was not called")
	}
}

func newTestServer(handler websocket.Handler) *httptest.Server {
	return httptest.NewServer(handler)
}
