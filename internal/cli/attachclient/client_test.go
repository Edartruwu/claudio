package attachclient

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Abraxas-365/claudio/internal/attach"
)

// TestClient_Connect_SendsHello verifies connection + hello message.
func TestClient_Connect_SendsHello(t *testing.T) {
	// Test server that expects WebSocket upgrade
	var receivedEnv attach.Envelope
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") != "websocket" {
			http.Error(w, "not a websocket request", http.StatusBadRequest)
			return
		}

		// Check auth header
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			http.Error(w, "missing auth", http.StatusUnauthorized)
			return
		}

		// Upgrade to raw TCP for frame reading
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "hijack not supported", http.StatusInternalServerError)
			return
		}

		conn, _, err := hj.Hijack()
		if err != nil {
			http.Error(w, "hijack failed", http.StatusInternalServerError)
			return
		}
		defer conn.Close()

		// Send upgrade response
		conn.Write([]byte("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n"))

		// Read first frame (should be hello)
		reader := bufio.NewReader(conn)
		lenBuf := make([]byte, 4)
		if _, err := reader.Read(lenBuf); err != nil {
			t.Logf("read length failed: %v", err)
			return
		}

		frameLen := int(lenBuf[0])<<24 | int(lenBuf[1])<<16 | int(lenBuf[2])<<8 | int(lenBuf[3])
		frameBuf := make([]byte, frameLen)
		if _, err := reader.Read(frameBuf); err != nil {
			t.Logf("read frame failed: %v", err)
			return
		}

		if err := json.Unmarshal(frameBuf, &receivedEnv); err != nil {
			t.Logf("unmarshal failed: %v", err)
			return
		}
	}))
	defer server.Close()

	client := New(server.URL, "test-password", "test-session", true)
	err := client.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	// Wait a tick for server goroutine
	<-time.After(10 * time.Millisecond)

	// Verify hello was sent
	if receivedEnv.Type != attach.EventSessionHello {
		t.Errorf("expected %s, got %s", attach.EventSessionHello, receivedEnv.Type)
	}

	var payload attach.HelloPayload
	if err := receivedEnv.UnmarshalPayload(&payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if payload.Name != "test-session" {
		t.Errorf("expected name=test-session, got %s", payload.Name)
	}
	if !payload.Master {
		t.Errorf("expected master=true")
	}
}

// TestClient_SendEvent_AssistantMsg verifies SendEvent works.
func TestClient_SendEvent_AssistantMsg(t *testing.T) {
	var receivedEnv attach.Envelope
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, _ := w.(http.Hijacker)
		conn, _, _ := hj.Hijack()
		defer conn.Close()

		conn.Write([]byte("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n"))

		// Skip hello (first frame)
		reader := bufio.NewReader(conn)
		lenBuf := make([]byte, 4)
		reader.Read(lenBuf)
		frameLen := int(lenBuf[0])<<24 | int(lenBuf[1])<<16 | int(lenBuf[2])<<8 | int(lenBuf[3])
		reader.Read(make([]byte, frameLen))

		// Read second frame (assistant msg)
		reader.Read(lenBuf)
		frameLen = int(lenBuf[0])<<24 | int(lenBuf[1])<<16 | int(lenBuf[2])<<8 | int(lenBuf[3])
		frameBuf := make([]byte, frameLen)
		reader.Read(frameBuf)
		json.Unmarshal(frameBuf, &receivedEnv)
	}))
	defer server.Close()

	client := New(server.URL, "test-password", "test-session", false)
	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	payload := attach.AssistantMsgPayload{
		Content:   "test response",
		AgentName: "test-agent",
	}
	if err := client.SendEvent(attach.EventMsgAssistant, payload); err != nil {
		t.Fatalf("SendEvent failed: %v", err)
	}

	<-time.After(10 * time.Millisecond)

	if receivedEnv.Type != attach.EventMsgAssistant {
		t.Errorf("expected %s, got %s", attach.EventMsgAssistant, receivedEnv.Type)
	}

	var p attach.AssistantMsgPayload
	if err := receivedEnv.UnmarshalPayload(&p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if p.Content != "test response" {
		t.Errorf("expected content=test response, got %s", p.Content)
	}
	if p.AgentName != "test-agent" {
		t.Errorf("expected agent=test-agent, got %s", p.AgentName)
	}
}

// TestClient_OnUserMessage_FiresCallback verifies callback fires.
func TestClient_OnUserMessage_FiresCallback(t *testing.T) {
	callbackFired := false
	var callbackPayload attach.UserMsgPayload

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, _ := w.(http.Hijacker)
		conn, _, _ := hj.Hijack()
		defer conn.Close()

		conn.Write([]byte("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n"))

		// Skip hello
		reader := bufio.NewReader(conn)
		lenBuf := make([]byte, 4)
		reader.Read(lenBuf)
		frameLen := int(lenBuf[0])<<24 | int(lenBuf[1])<<16 | int(lenBuf[2])<<8 | int(lenBuf[3])
		reader.Read(make([]byte, frameLen))

		// Send user message
		userMsg := attach.UserMsgPayload{
			Content:     "hello from server",
			FromSession: "remote-session",
		}
		env, _ := attach.NewEnvelope(attach.EventMsgUser, userMsg)
		data, _ := json.Marshal(env)
		frame := make([]byte, 4+len(data))
		frame[0] = byte((len(data) >> 24) & 0xff)
		frame[1] = byte((len(data) >> 16) & 0xff)
		frame[2] = byte((len(data) >> 8) & 0xff)
		frame[3] = byte(len(data) & 0xff)
		copy(frame[4:], data)
		conn.Write(frame)

		// Keep alive
		<-time.After(100 * time.Millisecond)
	}))
	defer server.Close()

	client := New(server.URL, "test-password", "test-session", false)
	client.OnUserMessage(func(p attach.UserMsgPayload) {
		callbackFired = true
		callbackPayload = p
	})

	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	<-time.After(50 * time.Millisecond)

	if !callbackFired {
		t.Error("callback did not fire")
	}
	if callbackPayload.Content != "hello from server" {
		t.Errorf("expected content=hello from server, got %s", callbackPayload.Content)
	}
	if callbackPayload.FromSession != "remote-session" {
		t.Errorf("expected from_session=remote-session, got %s", callbackPayload.FromSession)
	}
}

// TestClient_Close_SendsBye verifies Close sends bye message.
func TestClient_Close_SendsBye(t *testing.T) {
	var byeReceived bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, _ := w.(http.Hijacker)
		conn, _, _ := hj.Hijack()
		defer conn.Close()

		conn.Write([]byte("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n"))

		reader := bufio.NewReader(conn)
		for i := 0; i < 2; i++ { // hello + bye
			lenBuf := make([]byte, 4)
			if n, _ := reader.Read(lenBuf); n != 4 {
				break
			}
			frameLen := int(lenBuf[0])<<24 | int(lenBuf[1])<<16 | int(lenBuf[2])<<8 | int(lenBuf[3])
			frameBuf := make([]byte, frameLen)
			reader.Read(frameBuf)

			var env attach.Envelope
			json.Unmarshal(frameBuf, &env)
			if env.Type == attach.EventSessionBye {
				byeReceived = true
			}
		}
	}))
	defer server.Close()

	client := New(server.URL, "test-password", "test-session", false)
	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	client.Close()

	<-time.After(10 * time.Millisecond)

	if !byeReceived {
		t.Error("bye message not received")
	}
}
