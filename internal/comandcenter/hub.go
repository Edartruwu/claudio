package comandcenter

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/Abraxas-365/claudio/internal/attach"
	"golang.org/x/net/websocket"
)

// wsConn abstracts a WebSocket connection for testing.
type wsConn interface {
	writeEnvelope(env attach.Envelope) error
	readEnvelope(env *attach.Envelope) error
	close() error
}

// netWSConn wraps *websocket.Conn to implement wsConn.
type netWSConn struct {
	c *websocket.Conn
}

func (n *netWSConn) writeEnvelope(env attach.Envelope) error {
	return websocket.JSON.Send(n.c, env)
}

func (n *netWSConn) readEnvelope(env *attach.Envelope) error {
	return websocket.JSON.Receive(n.c, env)
}

func (n *netWSConn) close() error {
	return n.c.Close()
}

// UIEvent wraps a broadcast event with the originating session ID.
// It is sent on the UIBroadcast channel so UI listeners know which session
// produced the event.
type UIEvent struct {
	SessionID string
	Envelope  attach.Envelope
}

// Hub manages active WebSocket connections from Claudio sessions.
type Hub struct {
	mu              sync.RWMutex
	sessions        map[string]wsConn
	storage         *Storage
	uiBroadcast     chan UIEvent
	vapidPublicKey  string
	vapidPrivateKey string
}

// NewHub creates a Hub backed by storage.
func NewHub(storage *Storage) *Hub {
	return &Hub{
		sessions:    make(map[string]wsConn),
		storage:     storage,
		uiBroadcast: make(chan UIEvent, 256),
	}
}

// Register adds a connection under sessionID.
func (h *Hub) Register(sessionID string, conn wsConn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sessions[sessionID] = conn
}

// Unregister removes a connection by sessionID.
func (h *Hub) Unregister(sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.sessions, sessionID)
}

// Send writes an envelope to the session identified by sessionID.
func (h *Hub) Send(sessionID string, env attach.Envelope) error {
	h.mu.RLock()
	conn, ok := h.sessions[sessionID]
	h.mu.RUnlock()
	if !ok {
		return fmt.Errorf("hub: session %s not connected", sessionID)
	}
	return conn.writeEnvelope(env)
}

// SessionCount returns the number of connected sessions (for testing).
func (h *Hub) SessionCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.sessions)
}

// UIBroadcast returns the channel that receives all inbound session events
// (with session context) for broadcast to UI listeners. Callers must not
// close this channel.
func (h *Hub) UIBroadcast() <-chan UIEvent {
	return h.uiBroadcast
}

// SetVAPIDKeys configures the VAPID keys used for sending push notifications.
func (h *Hub) SetVAPIDKeys(public, private string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.vapidPublicKey = public
	h.vapidPrivateKey = private
}

// sendPushNotifications sends a push notification to all subscribers for a new assistant message.
// Subscriptions that return HTTP 410 Gone are deleted (browser unsubscribed).
func (h *Hub) sendPushNotifications(sessionName, sessionID, preview string) {
	h.mu.RLock()
	pub := h.vapidPublicKey
	priv := h.vapidPrivateKey
	h.mu.RUnlock()

	if pub == "" || priv == "" {
		return
	}

	subs, err := h.storage.ListPushSubscriptions()
	if err != nil || len(subs) == 0 {
		return
	}

	// Truncate preview to 100 chars.
	body := preview
	if len(body) > 100 {
		body = body[:100]
	}

	type pushPayload struct {
		Title string `json:"title"`
		Body  string `json:"body"`
		URL   string `json:"url"`
	}
	payloadBytes, err := json.Marshal(pushPayload{
		Title: sessionName,
		Body:  body,
		URL:   "/chat/" + sessionID,
	})
	if err != nil {
		return
	}

	for _, sub := range subs {
		resp, err := webpush.SendNotification(payloadBytes, &webpush.Subscription{
			Endpoint: sub.Endpoint,
			Keys: webpush.Keys{
				P256dh: sub.P256dh,
				Auth:   sub.Auth,
			},
		}, &webpush.Options{
			VAPIDPublicKey:  pub,
			VAPIDPrivateKey: priv,
			Subscriber:      "mailto:admin@comandcenter.local",
		})
		if err != nil {
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusGone {
			_ = h.storage.DeletePushSubscription(sub.Endpoint)
		}
	}
}

// HandleSession runs the read loop for a raw *websocket.Conn.
// It wraps the connection, processes events, and cleans up on close.
func (h *Hub) HandleSession(rawConn *websocket.Conn) {
	h.handleConn(&netWSConn{c: rawConn})
}

// handleConn is the testable inner implementation.
func (h *Hub) handleConn(conn wsConn) {
	var sessionID string

	defer func() {
		conn.close()
		if sessionID != "" {
			h.Unregister(sessionID)
			_ = h.storage.SetSessionStatus(sessionID, "inactive")
		}
	}()

	// First message must be EventSessionHello.
	var env attach.Envelope
	if err := conn.readEnvelope(&env); err != nil {
		return
	}
	if env.Type != attach.EventSessionHello {
		return
	}

	var hello attach.HelloPayload
	if err := env.UnmarshalPayload(&hello); err != nil {
		return
	}

	// Reconnect to existing session by name if it exists.
	existing, found, _ := h.storage.GetSessionByName(hello.Name)
	if found {
		sessionID = existing.ID
	} else {
		sessionID = newID()
	}
	now := time.Now()
	sess := Session{
		ID:           sessionID,
		Name:         hello.Name,
		Path:         hello.Path,
		Model:        hello.Model,
		Master:       hello.Master,
		Status:       "active",
		CreatedAt:    now,
		LastActiveAt: now,
	}
	if err := h.storage.UpsertSession(sess); err != nil {
		return
	}

	h.Register(sessionID, conn)
	h.broadcast(sessionID, env)

	// Main read loop.
	for {
		var ev attach.Envelope
		if err := conn.readEnvelope(&ev); err != nil {
			if err == io.EOF {
				break
			}
			break
		}

		h.processEvent(sessionID, ev)
		h.broadcast(sessionID, ev)
	}
}

func (h *Hub) processEvent(sessionID string, env attach.Envelope) {
	now := time.Now()

	switch env.Type {
	case attach.EventMsgAssistant:
		var p attach.AssistantMsgPayload
		if err := env.UnmarshalPayload(&p); err != nil {
			return
		}
		_ = h.storage.InsertMessage(Message{
			ID:        newID(),
			SessionID: sessionID,
			Role:      "assistant",
			Content:   p.Content,
			AgentName: p.AgentName,
			CreatedAt: now,
		})
		// Notify push subscribers of new assistant message.
		go func() {
			sess, err := h.storage.GetSession(sessionID)
			if err != nil {
				return
			}
			h.sendPushNotifications(sess.Name, sessionID, p.Content)
		}()

	case attach.EventMsgToolUse:
		var p attach.ToolUsePayload
		if err := env.UnmarshalPayload(&p); err != nil {
			return
		}
		content := string(p.Input)
		if content == "" {
			content = p.Tool
		}
		_ = h.storage.InsertMessage(Message{
			ID:        newID(),
			SessionID: sessionID,
			Role:      "tool_use",
			Content:   content,
			AgentName: p.AgentName,
			CreatedAt: now,
		})

	case attach.EventTaskCreated:
		var p attach.TaskCreatedPayload
		if err := env.UnmarshalPayload(&p); err != nil {
			return
		}
		_ = h.storage.UpsertTask(Task{
			ID:         p.ID,
			SessionID:  sessionID,
			Title:      p.Title,
			Status:     p.Status,
			AssignedTo: p.AssignedTo,
			CreatedAt:  now,
			UpdatedAt:  now,
		})

	case attach.EventTaskUpdated:
		var p attach.TaskUpdatedPayload
		if err := env.UnmarshalPayload(&p); err != nil {
			return
		}
		_ = h.storage.UpsertTask(Task{
			ID:        p.ID,
			SessionID: sessionID,
			Status:    p.Status,
			UpdatedAt: now,
		})

	case attach.EventAgentStatus:
		var p attach.AgentStatusPayload
		if err := env.UnmarshalPayload(&p); err != nil {
			return
		}
		agentID := fmt.Sprintf("%s:%s", sessionID, p.Name)
		_ = h.storage.UpsertAgent(Agent{
			ID:            agentID,
			SessionID:     sessionID,
			Name:          p.Name,
			Status:        p.Status,
			CurrentTaskID: p.CurrentTask,
			UpdatedAt:     now,
		})

	case attach.EventSessionBye:
		// handled by caller (defer)

	default:
		// unknown event — store raw as user message for auditability
		raw, _ := json.Marshal(env)
		_ = h.storage.InsertMessage(Message{
			ID:        newID(),
			SessionID: sessionID,
			Role:      "user",
			Content:   string(raw),
			CreatedAt: now,
		})
	}
}

// broadcast sends an envelope (with session context) to UI listeners (non-blocking; drops if full).
func (h *Hub) broadcast(sessionID string, env attach.Envelope) {
	select {
	case h.uiBroadcast <- UIEvent{SessionID: sessionID, Envelope: env}:
	default:
	}
}
