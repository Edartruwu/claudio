package attachclient

import (
	"context"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Abraxas-365/claudio/internal/attach"
	"golang.org/x/net/websocket"
)

// TestClient_Connect_SendsHello verifies connection + hello message.
func TestClient_Connect_SendsHello(t *testing.T) {
	var receivedEnv attach.Envelope
	handler := websocket.Handler(func(conn *websocket.Conn) {
		defer conn.Close()
		
		// Read hello
		if err := websocket.JSON.Receive(conn, &receivedEnv); err != nil {
			t.Logf("receive failed: %v", err)
			return
		}
	})
	
	server := httptest.NewServer(handler)
	defer server.Close()

	client := New(server.URL, "test-password", "test-session", true)
	err := client.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	// Wait for server to process
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
	handler := websocket.Handler(func(conn *websocket.Conn) {
		defer conn.Close()
		
		// Skip hello (first message)
		var hello attach.Envelope
		if err := websocket.JSON.Receive(conn, &hello); err != nil {
			t.Logf("receive hello failed: %v", err)
			return
		}
		
		// Read assistant msg (second message)
		if err := websocket.JSON.Receive(conn, &receivedEnv); err != nil {
			t.Logf("receive msg failed: %v", err)
			return
		}
	})
	
	server := httptest.NewServer(handler)
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

	handler := websocket.Handler(func(conn *websocket.Conn) {
		defer conn.Close()
		
		// Read hello
		var hello attach.Envelope
		if err := websocket.JSON.Receive(conn, &hello); err != nil {
			t.Logf("receive hello failed: %v", err)
			return
		}
		
		// Send user message
		userMsg := attach.UserMsgPayload{
			Content:     "hello from server",
			FromSession: "remote-session",
		}
		env, _ := attach.NewEnvelope(attach.EventMsgUser, userMsg)
		if err := websocket.JSON.Send(conn, env); err != nil {
			t.Logf("send failed: %v", err)
			return
		}
		
		// Keep alive
		<-time.After(100 * time.Millisecond)
	})
	
	server := httptest.NewServer(handler)
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
	handler := websocket.Handler(func(conn *websocket.Conn) {
		defer conn.Close()
		
		for {
			var env attach.Envelope
			if err := websocket.JSON.Receive(conn, &env); err != nil {
				if err == io.EOF {
					break
				}
				return
			}
			
			if env.Type == attach.EventSessionBye {
				byeReceived = true
			}
		}
	})
	
	server := httptest.NewServer(handler)
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
