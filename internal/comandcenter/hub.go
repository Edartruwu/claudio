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

const pingInterval = 30 * time.Second

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
	interruptFns    map[string]func()
	setAgentFns     map[string]func(agentType string)
	setTeamFns      map[string]func(teamName string)
	storage         *Storage
	uiBroadcast     chan UIEvent
	vapidPublicKey  string
	vapidPrivateKey string
}

// NewHub creates a Hub backed by storage.
func NewHub(storage *Storage) *Hub {
	return &Hub{
		sessions:     make(map[string]wsConn),
		interruptFns: make(map[string]func()),
		setAgentFns:  make(map[string]func(agentType string)),
		setTeamFns:   make(map[string]func(teamName string)),
		storage:      storage,
		uiBroadcast:  make(chan UIEvent, 256),
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

// RegisterInterrupt stores a cancel function for a session's active engine turn.
func (h *Hub) RegisterInterrupt(sessionID string, fn func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.interruptFns[sessionID] = fn
}

// UnregisterInterrupt removes the cancel function for a session.
func (h *Hub) UnregisterInterrupt(sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.interruptFns, sessionID)
}

// Interrupt calls the registered cancel function for a session.
// Returns true if a function was found and called, false otherwise.
func (h *Hub) Interrupt(sessionID string) bool {
	h.mu.RLock()
	fn, ok := h.interruptFns[sessionID]
	h.mu.RUnlock()
	if !ok {
		return false
	}
	fn()
	return true
}

// RegisterSetAgentFn stores a callback to change the active agent for a session.
func (h *Hub) RegisterSetAgentFn(sessionID string, fn func(agentType string)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.setAgentFns[sessionID] = fn
}

// RegisterSetTeamFn stores a callback to change the active team for a session.
func (h *Hub) RegisterSetTeamFn(sessionID string, fn func(teamName string)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.setTeamFns[sessionID] = fn
}

// SetAgent calls the registered set-agent callback for a session.
func (h *Hub) SetAgent(sessionID, agentType string) error {
	h.mu.RLock()
	fn, ok := h.setAgentFns[sessionID]
	h.mu.RUnlock()
	if !ok {
		return fmt.Errorf("hub: session %s not connected", sessionID)
	}
	fn(agentType)
	return nil
}

// SetTeam calls the registered set-team callback for a session.
func (h *Hub) SetTeam(sessionID, teamName string) error {
	h.mu.RLock()
	fn, ok := h.setTeamFns[sessionID]
	h.mu.RUnlock()
	if !ok {
		return fmt.Errorf("hub: session %s not connected", sessionID)
	}
	fn(teamName)
	return nil
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

	// Declarative Web Push (iOS 18.4+): embed full notification in payload.
	// On older iOS the SW push handler reads title/body/url directly.
	// On iOS 18.4+ the "web_push" key lets APNS show notification without needing the SW alive.
	type declarativeNotification struct {
		Title string `json:"title"`
		Body  string `json:"body"`
		Icon  string `json:"icon,omitempty"`
	}
	type pushPayload struct {
		Title   string                    `json:"title"`
		Body    string                    `json:"body"`
		URL     string                    `json:"url"`
		WebPush int                       `json:"web_push"` // 8030 = Declarative Web Push version
		Notification declarativeNotification `json:"notification"`
	}
	payloadBytes, err := json.Marshal(pushPayload{
		Title: sessionName,
		Body:  body,
		URL:   "/chat/" + sessionID,
		WebPush: 8030,
		Notification: declarativeNotification{
			Title: sessionName,
			Body:  body,
			Icon:  "/static/icon-192.png",
		},
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
			Urgency:         webpush.UrgencyHigh,
			TTL:             86400, // 24h — redeliver even if device was offline
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
			h.UnregisterInterrupt(sessionID)
			h.mu.Lock()
			delete(h.setAgentFns, sessionID)
			delete(h.setTeamFns, sessionID)
			h.mu.Unlock()
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
		// Populate on first connect so INSERT carries the right values.
		// On reconnect (found=true), UpsertSession does not update these columns.
		AgentType:    hello.AgentType,
		TeamTemplate: hello.TeamTemplate,
	}
	if err := h.storage.UpsertSession(sess); err != nil {
		return
	}

	h.Register(sessionID, conn)
	// Register interrupt fn: sends EventInterrupt envelope to the attached Claudio process.
	h.RegisterInterrupt(sessionID, func() {
		_ = conn.writeEnvelope(attach.Envelope{Type: attach.EventInterrupt})
	})
	// Register set-agent/set-team fns: send envelopes to the attached Claudio process.
	h.RegisterSetAgentFn(sessionID, func(agentType string) {
		env, err := attach.NewEnvelope(attach.EventSetAgent, attach.SetAgentPayload{AgentType: agentType})
		if err == nil {
			_ = conn.writeEnvelope(env)
		}
	})
	h.RegisterSetTeamFn(sessionID, func(teamName string) {
		env, err := attach.NewEnvelope(attach.EventSetTeam, attach.SetTeamPayload{TeamName: teamName})
		if err == nil {
			_ = conn.writeEnvelope(env)
		}
	})
	h.Broadcast(sessionID, env)

	// Ping ticker goroutine — keep connection alive on idle.
	pingTicker := time.NewTicker(pingInterval)
	pingDone := make(chan struct{})
	go func() {
		defer pingTicker.Stop()
		for {
			select {
			case <-pingTicker.C:
				_ = conn.writeEnvelope(attach.Envelope{Type: "ping"})
			case <-pingDone:
				return
			}
		}
	}()
	defer close(pingDone)

	// Main read loop.
	for {
		var ev attach.Envelope
		if err := conn.readEnvelope(&ev); err != nil {
			if err == io.EOF {
				break
			}
			break
		}

		// processEvent does DB writes (SQLite) — run async to avoid blocking
		// the read loop under write pressure. Broadcast is already non-blocking.
		go h.processEvent(sessionID, ev)
		h.Broadcast(sessionID, ev)
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
		h.broadcastAgentLog(sessionID, p.AgentName)

	case attach.EventMsgToolUse:
		var p attach.ToolUsePayload
		if err := env.UnmarshalPayload(&p); err != nil {
			return
		}
		content := p.Tool
		if len(p.Input) > 0 && string(p.Input) != "null" {
			content = p.Tool + ": " + string(p.Input)
		}
		_ = h.storage.InsertMessage(Message{
			ID:        newID(),
			SessionID: sessionID,
			Role:      "tool_use",
			Content:   content,
			AgentName: p.AgentName,
			ToolUseID: p.ID,
			CreatedAt: now,
		})
		h.broadcastAgentLog(sessionID, p.AgentName)

	case attach.EventMsgToolResult:
		var p attach.ToolResultPayload
		if err := env.UnmarshalPayload(&p); err != nil {
			return
		}
		_ = h.storage.UpdateMessageOutput(sessionID, p.ToolUseID, p.Output)
		h.broadcastAgentLog(sessionID, p.AgentName)

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
			CurrentTool:   p.CurrentTool,
			CallCount:     p.CallCount,
			ElapsedSecs:   p.ElapsedSecs,
			UpdatedAt:     now,
		})
		if p.Status == "done" {
			go func() {
				sess, err := h.storage.GetSession(sessionID)
				if err != nil {
					return
				}
				h.sendPushNotifications(sess.Name, sessionID, fmt.Sprintf("%s completed its task", p.Name))
			}()
		}

	case attach.EventTaskCreated:
		var p attach.TaskCreatedPayload
		if err := env.UnmarshalPayload(&p); err != nil {
			return
		}
		_ = h.storage.UpsertTask(Task{
			ID:          p.ID,
			SessionID:   sessionID,
			Title:       p.Title,
			Description: p.Description,
			Status:      p.Status,
			AssignedTo:  p.AssignedTo,
			CreatedAt:   now,
			UpdatedAt:   now,
		})

	case attach.EventTaskUpdated:
		var p attach.TaskUpdatedPayload
		if err := env.UnmarshalPayload(&p); err != nil {
			return
		}
		// Preserve existing CreatedAt by fetching prior record; fall back to now.
		existing, err := h.storage.GetTask(p.ID)
		if err != nil {
			existing = Task{ID: p.ID, SessionID: sessionID, CreatedAt: now}
		}
		if p.Title != "" {
			existing.Title = p.Title
		}
		if p.Description != "" {
			existing.Description = p.Description
		}
		if p.AssignedTo != "" {
			existing.AssignedTo = p.AssignedTo
		}
		existing.Status = p.Status
		existing.UpdatedAt = now
		_ = h.storage.UpsertTask(existing)

	case attach.EventSessionBye:
		// handled by caller (defer)

	case attach.EventDesignScreenshot:
		// handled by web server fanout; no DB storage needed here

	case attach.EventDesignBundleReady:
		// handled by web server fanout; no DB storage needed here

	case attach.EventTokenUsage:
		var p attach.TokenUsagePayload
		if err := env.UnmarshalPayload(&p); err != nil {
			return
		}
		_ = h.storage.UpdateContextTokens(sessionID, p.ContextTokens)

	case attach.EventMsgStreamDelta:
		// transient streaming delta — never persisted

	default:
		// unknown/internal events are silently dropped
	}
}

// Broadcast sends an envelope (with session context) to UI listeners (non-blocking; drops if full).
// For EventMsgAssistant it also fires push notifications so backgrounded PWA clients are notified.
func (h *Hub) Broadcast(sessionID string, env attach.Envelope) {
	select {
	case h.uiBroadcast <- UIEvent{SessionID: sessionID, Envelope: env}:
	default:
	}
	if env.Type == attach.EventMsgAssistant {
		var p attach.AssistantMsgPayload
		if err := env.UnmarshalPayload(&p); err == nil {
			go func() {
				sess, err := h.storage.GetSession(sessionID)
				if err != nil {
					return
				}
				h.sendPushNotifications(sess.Name, sessionID, p.Content)
			}()
		}
	}
}

// broadcastAgentLog emits an EventAgentLog notification so the UI knows to
// refresh the agent log drawer. No-op if agentName is empty.
func (h *Hub) broadcastAgentLog(sessionID, agentName string) {
	if agentName == "" {
		return
	}
	env, err := attach.NewEnvelope(attach.EventAgentLog, attach.AgentLogPayload{
		SessionID: sessionID,
		AgentName: agentName,
	})
	if err != nil {
		return
	}
	h.Broadcast(sessionID, env)
}
