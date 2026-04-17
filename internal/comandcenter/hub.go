package comandcenter

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

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

// Hub manages active WebSocket connections from Claudio sessions.
type Hub struct {
	mu          sync.RWMutex
	sessions    map[string]wsConn
	storage     *Storage
	uiBroadcast chan attach.Envelope
}

// NewHub creates a Hub backed by storage.
func NewHub(storage *Storage) *Hub {
	return &Hub{
		sessions:    make(map[string]wsConn),
		storage:     storage,
		uiBroadcast: make(chan attach.Envelope, 256),
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
// for SSE broadcast to UI listeners. Callers must not close this channel.
func (h *Hub) UIBroadcast() <-chan attach.Envelope {
	return h.uiBroadcast
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

	sessionID = newID()
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
	h.broadcast(env)

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
		h.broadcast(ev)
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

// broadcast sends an envelope to UI listeners (non-blocking; drops if full).
func (h *Hub) broadcast(env attach.Envelope) {
	select {
	case h.uiBroadcast <- env:
	default:
	}
}
