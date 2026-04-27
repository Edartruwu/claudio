package comandcenter

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
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
	setWriteDeadline(t time.Time) error
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

func (n *netWSConn) setWriteDeadline(t time.Time) error {
	return n.c.SetWriteDeadline(t)
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
	eventQueues     map[string]chan attach.Envelope
	workerDone      map[string]chan struct{}
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
		eventQueues:  make(map[string]chan attach.Envelope),
		workerDone:   make(map[string]chan struct{}),
		uiBroadcast:  make(chan UIEvent, 1024),
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
// The RLock is held during the write to prevent a concurrent Unregister+Close
// from racing with the write on the underlying connection.
func (h *Hub) Send(sessionID string, env attach.Envelope) error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	conn, ok := h.sessions[sessionID]
	if !ok {
		return fmt.Errorf("hub: session %s not connected", sessionID)
	}
	if err := conn.setWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return fmt.Errorf("hub: session %s set write deadline: %w", sessionID, err)
	}
	err := conn.writeEnvelope(env)
	_ = conn.setWriteDeadline(time.Time{}) // clear deadline
	return err
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
		log.Printf("[push] skipping: VAPID keys not configured")
		return
	}

	subs, err := h.storage.ListPushSubscriptions()
	if err != nil {
		log.Printf("[push] error listing subscriptions: %v", err)
		return
	}
	if len(subs) == 0 {
		return
	}
	log.Printf("[push] sending to %d subscriber(s): %s", len(subs), sessionName)

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
			Subscriber:      "https://github.com/Abraxas-365/claudio",
			Urgency:         webpush.UrgencyHigh,
			TTL:             86400, // 24h — redeliver even if device was offline
		})
		if err != nil {
			log.Printf("[push] send error to %s: %v", sub.Endpoint, err)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode >= 400 {
			log.Printf("[push] HTTP %d from %s", resp.StatusCode, sub.Endpoint)
		}
		// 403 = invalid/expired subscription, 410 = gone. Auto-cleanup both.
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusGone {
			log.Printf("[push] removing stale subscription: %s", sub.Endpoint)
			_ = h.storage.DeletePushSubscription(sub.Endpoint)
		}
	}
}

// startSessionWorker spawns a goroutine that drains the per-session event
// queue, calling processEvent sequentially for each envelope. The goroutine
// exits when the channel is closed (session disconnect) and signals via
// workerDone[sessionID].
func (h *Hub) startSessionWorker(sessionID string) {
	h.mu.Lock()
	ch := make(chan attach.Envelope, 64)
	done := make(chan struct{})
	h.eventQueues[sessionID] = ch
	h.workerDone[sessionID] = done
	h.mu.Unlock()

	go func() {
		for ev := range ch {
			// Cap per-event processing to prevent worker stall on slow DB.
			eventDone := make(chan struct{})
			go func(e attach.Envelope) {
				defer close(eventDone)
				h.processEvent(sessionID, e)
			}(ev)
			select {
			case <-eventDone:
			case <-time.After(10 * time.Second):
				log.Printf("hub: session %s processEvent timeout for type=%s", sessionID, ev.Type)
			}
		}
		// Signal exit and clean up done-channel entry.
		h.mu.Lock()
		close(done)
		delete(h.workerDone, sessionID)
		h.mu.Unlock()
	}()
}

// stopSessionWorker closes and removes the per-session event queue.
// Must be called when the session disconnects.
func (h *Hub) stopSessionWorker(sessionID string) {
	h.mu.Lock()
	if ch, ok := h.eventQueues[sessionID]; ok {
		close(ch)
		delete(h.eventQueues, sessionID)
	}
	h.mu.Unlock()
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
			h.stopSessionWorker(sessionID)
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
		CliSessionID: hello.SessionID,
	}
	if err := h.storage.UpsertSession(sess); err != nil {
		return
	}

	h.Register(sessionID, conn)
	h.startSessionWorker(sessionID)
	// Register interrupt fn: sends EventInterrupt envelope to the attached Claudio process.
	h.RegisterInterrupt(sessionID, func() {
		_ = conn.setWriteDeadline(time.Now().Add(10 * time.Second))
		_ = conn.writeEnvelope(attach.Envelope{Type: attach.EventInterrupt})
		_ = conn.setWriteDeadline(time.Time{})
	})
	// Register set-agent/set-team fns: send envelopes to the attached Claudio process.
	h.RegisterSetAgentFn(sessionID, func(agentType string) {
		env, err := attach.NewEnvelope(attach.EventSetAgent, attach.SetAgentPayload{AgentType: agentType})
		if err == nil {
			_ = conn.setWriteDeadline(time.Now().Add(10 * time.Second))
			_ = conn.writeEnvelope(env)
			_ = conn.setWriteDeadline(time.Time{})
		}
	})
	h.RegisterSetTeamFn(sessionID, func(teamName string) {
		env, err := attach.NewEnvelope(attach.EventSetTeam, attach.SetTeamPayload{TeamName: teamName})
		if err == nil {
			_ = conn.setWriteDeadline(time.Now().Add(10 * time.Second))
			_ = conn.writeEnvelope(env)
			_ = conn.setWriteDeadline(time.Time{})
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
				_ = conn.setWriteDeadline(time.Now().Add(30 * time.Second))
				_ = conn.writeEnvelope(attach.Envelope{Type: "ping"})
				_ = conn.setWriteDeadline(time.Time{}) // clear deadline for subsequent writes
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

		// Send to per-session queue for sequential processing.
		// Worker goroutine drains the queue, ensuring ordered DB writes
		// (e.g. INSERT before UPDATE for tool_use → tool_result pairs).
		h.mu.RLock()
		ch := h.eventQueues[sessionID]
		h.mu.RUnlock()
		select {
		case ch <- ev:
		default:
			log.Printf("hub: session %s event queue full, dropping event type=%s", sessionID, ev.Type)
		}
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
		if err := h.storage.InsertMessage(Message{
			ID:        newID(),
			SessionID: sessionID,
			Role:      "assistant",
			Content:   p.Content,
			AgentName: p.AgentName,
			CreatedAt: now,
		}); err != nil {
			log.Printf("hub: InsertMessage (assistant) session=%s: %v", sessionID, err)
		}
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
		if err := h.storage.InsertMessage(Message{
			ID:        newID(),
			SessionID: sessionID,
			Role:      "tool_use",
			Content:   content,
			AgentName: p.AgentName,
			ToolUseID: p.ID,
			CreatedAt: now,
		}); err != nil {
			log.Printf("hub: InsertMessage (tool_use) session=%s: %v", sessionID, err)
		}
		h.broadcastAgentLog(sessionID, p.AgentName)

	case attach.EventMsgToolResult:
		var p attach.ToolResultPayload
		if err := env.UnmarshalPayload(&p); err != nil {
			return
		}
		if err := h.storage.UpdateMessageOutput(sessionID, p.ToolUseID, p.Output); err != nil {
			log.Printf("hub: UpdateMessageOutput session=%s tool_use_id=%s: %v", sessionID, p.ToolUseID, err)
		}
		h.broadcastAgentLog(sessionID, p.AgentName)

	case attach.EventAgentStatus:
		var p attach.AgentStatusPayload
		if err := env.UnmarshalPayload(&p); err != nil {
			return
		}
		agentID := fmt.Sprintf("%s:%s", sessionID, p.Name)
		if err := h.storage.UpsertAgent(Agent{
			ID:            agentID,
			SessionID:     sessionID,
			Name:          p.Name,
			Status:        p.Status,
			CurrentTaskID: p.CurrentTask,
			CurrentTool:   p.CurrentTool,
			CallCount:     p.CallCount,
			ElapsedSecs:   p.ElapsedSecs,
			UpdatedAt:     now,
		}); err != nil {
			log.Printf("hub: UpsertAgent session=%s agent=%s: %v", sessionID, p.Name, err)
		}
		// Persist event for reconnect replay.
		// Only store status transitions (not periodic heartbeats) to avoid DB bloat.
		// Heartbeats always have status "working" with no result — terminal events have
		// status done/failed/waiting or carry a non-empty result/summary.
		if p.Status != "working" || p.Result != "" || p.Summary != "" {
			payloadJSON, _ := json.Marshal(p)
			if err := h.storage.InsertAgentEvent(sessionID, p.Name, p.Status, string(payloadJSON)); err != nil {
				log.Printf("hub: InsertAgentEvent session=%s agent=%s: %v", sessionID, p.Name, err)
			}
		}
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
		// No DB write needed — team_tasks is written directly by tools.
		// Broadcast is handled by the caller.

	case attach.EventTaskUpdated:
		// No DB write needed — team_tasks is written directly by tools.
		// Broadcast is handled by the caller.

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
		if err := h.storage.UpdateContextTokens(sessionID, p.ContextTokens); err != nil {
			log.Printf("hub: UpdateContextTokens session=%s: %v", sessionID, err)
		}

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
		log.Printf("[hub] uiBroadcast full, dropping event for session %s (type %s)", sessionID, env.Type)
	}
	if env.Type == attach.EventMsgAssistant {
		var p attach.AssistantMsgPayload
		if err := env.UnmarshalPayload(&p); err == nil {
			// Skip push for short agent status lines (e.g. "⏳ agent — done").
			if isAgentStatusContent(p.Content) {
				return
			}
			go func() {
				sess, err := h.storage.GetSession(sessionID)
				if err != nil {
					return
				}
				h.sendPushNotifications(sess.Name, sessionID, stripMarkdown(p.Content))
			}()
		}
	}
}

// isAgentStatusContent returns true for short agent-status lines like "⏳ agent — done".
func isAgentStatusContent(s string) bool {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) == 0 {
		return false
	}
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if !strings.Contains(strings.ToLower(l), "done") {
			return false
		}
	}
	return len(strings.TrimSpace(s)) < 200
}

// stripMarkdown removes common markdown formatting for push notification previews.
func stripMarkdown(s string) string {
	// Remove code blocks.
	for {
		start := strings.Index(s, "```")
		if start < 0 {
			break
		}
		end := strings.Index(s[start+3:], "```")
		if end < 0 {
			s = s[:start]
			break
		}
		s = s[:start] + s[start+3+end+3:]
	}
	// Remove inline code.
	s = strings.ReplaceAll(s, "`", "")
	// Remove bold/italic markers.
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "__", "")
	s = strings.ReplaceAll(s, "*", "")
	// Remove headers.
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimLeft(l, "# ")
	}
	s = strings.Join(lines, " ")
	// Collapse whitespace.
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
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
