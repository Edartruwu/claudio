package comandcenter

import (
	"encoding/json"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/Abraxas-365/claudio/internal/attach"
)

// mockConn is a test double for wsConn.
type mockConn struct {
	mu       sync.Mutex
	toRead   []attach.Envelope // envelopes served on readEnvelope calls
	idx      int
	sent     []attach.Envelope
	closed   bool
	readErr  error // returned after toRead is exhausted
}

func (m *mockConn) readEnvelope(env *attach.Envelope) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.idx >= len(m.toRead) {
		if m.readErr != nil {
			return m.readErr
		}
		return io.EOF
	}
	*env = m.toRead[m.idx]
	m.idx++
	return nil
}

func (m *mockConn) writeEnvelope(env attach.Envelope) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, env)
	return nil
}

func (m *mockConn) close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func helloEnvelope(t *testing.T, name, path string) attach.Envelope {
	t.Helper()
	env, err := attach.NewEnvelope(attach.EventSessionHello, attach.HelloPayload{
		Name: name, Path: path, Model: "claude",
	})
	if err != nil {
		t.Fatalf("build hello envelope: %v", err)
	}
	return env
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func TestHub_RegisterUnregister(t *testing.T) {
	s := newTestStorage(t)
	h := NewHub(s)

	conn := &mockConn{}
	h.Register("session-a", conn)

	if h.SessionCount() != 1 {
		t.Errorf("after Register: count=%d, want 1", h.SessionCount())
	}

	h.Unregister("session-a")
	if h.SessionCount() != 0 {
		t.Errorf("after Unregister: count=%d, want 0", h.SessionCount())
	}
}

func TestHub_Send_UnknownSession(t *testing.T) {
	s := newTestStorage(t)
	h := NewHub(s)

	env := attach.Envelope{Type: attach.EventMsgUser, Payload: mustJSON("hi")}
	err := h.Send("no-such-session", env)
	if err == nil {
		t.Error("expected error sending to unknown session, got nil")
	}
}

func TestHub_Send_KnownSession(t *testing.T) {
	s := newTestStorage(t)
	h := NewHub(s)

	conn := &mockConn{}
	h.Register("sess-known", conn)

	env := attach.Envelope{Type: attach.EventMsgUser, Payload: mustJSON("hello")}
	if err := h.Send("sess-known", env); err != nil {
		t.Fatalf("Send to known session: %v", err)
	}

	conn.mu.Lock()
	n := len(conn.sent)
	conn.mu.Unlock()
	if n != 1 {
		t.Errorf("expected 1 sent message, got %d", n)
	}
}

func TestHub_HandleConn_HelloRegisters(t *testing.T) {
	s := newTestStorage(t)
	h := NewHub(s)

	conn := &mockConn{
		toRead: []attach.Envelope{
			helloEnvelope(t, "my-session", "/tmp/proj"),
			// EOF after hello
		},
	}

	done := make(chan struct{})
	go func() {
		h.handleConn(conn)
		close(done)
	}()

	// Give the goroutine time to process hello and register.
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timeout waiting for session to appear in DB")
		case <-time.After(10 * time.Millisecond):
			sessions, err := s.ListSessions("")
			if err == nil && len(sessions) > 0 {
				if sessions[0].Name == "my-session" {
					goto done
				}
			}
		}
	}
done:
	<-done

	// After loop closes, session should be inactive.
	sessions, err := s.ListSessions("")
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) == 0 {
		t.Fatal("expected session in DB")
	}
	if sessions[0].Status != "inactive" {
		t.Errorf("status after disconnect: got %q, want inactive", sessions[0].Status)
	}
}

func TestHub_HandleConn_ProcessesAssistantMsg(t *testing.T) {
	s := newTestStorage(t)
	h := NewHub(s)

	assistantEnv, _ := attach.NewEnvelope(attach.EventMsgAssistant, attach.AssistantMsgPayload{
		Content: "I can help with that.", AgentName: "assistant",
	})

	conn := &mockConn{
		toRead: []attach.Envelope{
			helloEnvelope(t, "proc-session", "/tmp"),
			assistantEnv,
		},
	}

	h.handleConn(conn)

	sessions, err := s.ListSessions("")
	if err != nil || len(sessions) == 0 {
		t.Fatal("no session in DB")
	}
	sessionID := sessions[0].ID

	// processEvent runs asynchronously — poll until the message appears or deadline.
	var msgs []Message
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timeout: expected 1 message, got 0")
		case <-time.After(10 * time.Millisecond):
		}
		msgs, err = s.ListMessages(sessionID, 10)
		if err != nil {
			t.Fatalf("ListMessages: %v", err)
		}
		if len(msgs) >= 1 {
			break
		}
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != "assistant" {
		t.Errorf("role: got %q, want assistant", msgs[0].Role)
	}
	if msgs[0].Content != "I can help with that." {
		t.Errorf("content: got %q", msgs[0].Content)
	}
}

func TestHub_HandleConn_NonHelloFirstMsg(t *testing.T) {
	s := newTestStorage(t)
	h := NewHub(s)

	conn := &mockConn{
		toRead: []attach.Envelope{
			{Type: attach.EventMsgAssistant, Payload: mustJSON("bad first msg")},
		},
	}
	h.handleConn(conn)

	sessions, _ := s.ListSessions("")
	if len(sessions) != 0 {
		t.Errorf("expected no sessions for bad first msg, got %d", len(sessions))
	}
}
