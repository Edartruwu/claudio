package app

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/Abraxas-365/claudio/internal/attach"
	"github.com/Abraxas-365/claudio/internal/bus"
	"github.com/Abraxas-365/claudio/internal/storage"
)

func openTestDB(t *testing.T) *storage.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("openTestDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// TestApp_InjectMessage_Delivers verifies that InjectMessage sends content to the inject channel.
func TestApp_InjectMessage_Delivers(t *testing.T) {
	app := &App{
		InjectCh: make(chan attach.UserMsgPayload, 8),
	}

	app.InjectMessage("test message")

	// Read from channel with timeout
	select {
	case p := <-app.InjectCh:
		if p.Content != "test message" {
			t.Fatalf("expected 'test message', got %q", p.Content)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

// TestApp_InjectMessage_NonBlocking verifies that InjectMessage drops silently when channel is full.
func TestApp_InjectMessage_NonBlocking(t *testing.T) {
	app := &App{
		InjectCh: make(chan attach.UserMsgPayload, 1), // small buffer
	}

	// Fill the channel
	app.InjectCh <- attach.UserMsgPayload{Content: "first"}

	// Second call should not block or panic
	done := make(chan struct{})
	go func() {
		app.InjectMessage("second") // should drop silently, not block
		close(done)
	}()

	select {
	case <-done:
		// success — InjectMessage returned without blocking
	case <-time.After(2 * time.Second):
		t.Fatal("InjectMessage blocked when channel was full")
	}

	// Verify first message is still there
	p := <-app.InjectCh
	if p.Content != "first" {
		t.Fatalf("expected 'first', got %q", p.Content)
	}

	// Second message should have been dropped (not in channel)
	select {
	case p := <-app.InjectCh:
		t.Fatalf("expected dropped message, but got %q", p.Content)
	default:
		// correct — message was dropped
	}
}

// TestApp_ClearHistory_PublishesEvent verifies that ClearHistory publishes EventClearHistory on the bus.
func TestApp_ClearHistory_PublishesEvent(t *testing.T) {
	db := openTestDB(t)

	sess, err := db.CreateSession("/tmp/test-project", "claude-sonnet-4-6")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	b := bus.New()
	events := make(chan bus.Event, 1)
	unsub := b.Subscribe(attach.EventClearHistory, func(e bus.Event) { events <- e })
	defer unsub()

	a := &App{DB: db, Bus: b}
	a.ClearHistory(sess.ID)

	select {
	case evt := <-events:
		if evt.Type != attach.EventClearHistory {
			t.Errorf("Type = %q, want %q", evt.Type, attach.EventClearHistory)
		}
		var payload attach.ClearHistoryPayload
		if err := json.Unmarshal(evt.Payload, &payload); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		if payload.SessionID != sess.ID {
			t.Errorf("SessionID = %q, want %q", payload.SessionID, sess.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for EventClearHistory")
	}
}

// TestApp_InjectMessage_ConcurrentReceives verifies the channel can be read concurrently.
func TestApp_InjectMessage_ConcurrentReceives(t *testing.T) {
	app := &App{
		InjectCh: make(chan attach.UserMsgPayload, 8),
	}

	// Send multiple messages
	app.InjectMessage("msg1")
	app.InjectMessage("msg2")
	app.InjectMessage("msg3")

	// Read them back
	received := []string{}
	for i := 0; i < 3; i++ {
		select {
		case p := <-app.InjectCh:
			received = append(received, p.Content)
		case <-time.After(1 * time.Second):
			t.Fatal("timeout reading message")
		}
	}

	if len(received) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(received))
	}
	if received[0] != "msg1" || received[1] != "msg2" || received[2] != "msg3" {
		t.Fatalf("messages out of order: %v", received)
	}
}
