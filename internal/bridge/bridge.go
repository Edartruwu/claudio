// Package bridge provides cross-session communication for coordinating
// parallel agents working in different worktrees or sessions.
package bridge

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Message represents a cross-session message.
type Message struct {
	From      string    `json:"from"`       // sender session ID
	To        string    `json:"to"`         // target session ID or "*" for broadcast
	Type      string    `json:"type"`       // "text", "status", "result", "shutdown"
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// Bridge manages cross-session communication via Unix domain sockets.
type Bridge struct {
	mu        sync.RWMutex
	socketDir string
	sessionID string
	listener  net.Listener
	inbox     chan Message
	handlers  []func(Message)
}

// New creates a new bridge for the given session.
func New(socketDir, sessionID string) *Bridge {
	os.MkdirAll(socketDir, 0700)
	return &Bridge{
		socketDir: socketDir,
		sessionID: sessionID,
		inbox:     make(chan Message, 100),
	}
}

// Start begins listening for incoming messages.
func (b *Bridge) Start() error {
	socketPath := b.socketPath(b.sessionID)

	// Clean up stale socket
	os.Remove(socketPath)

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", socketPath, err)
	}
	b.listener = ln

	go b.acceptLoop()
	return nil
}

// Stop closes the bridge listener.
func (b *Bridge) Stop() {
	if b.listener != nil {
		b.listener.Close()
	}
	os.Remove(b.socketPath(b.sessionID))
	close(b.inbox)
}

// Send sends a message to a specific session.
func (b *Bridge) Send(targetSession string, msgType, content string) error {
	msg := Message{
		From:      b.sessionID,
		To:        targetSession,
		Type:      msgType,
		Content:   content,
		Timestamp: time.Now(),
	}

	conn, err := net.DialTimeout("unix", b.socketPath(targetSession), 3*time.Second)
	if err != nil {
		return fmt.Errorf("connect to session %s: %w", targetSession, err)
	}
	defer conn.Close()

	return json.NewEncoder(conn).Encode(msg)
}

// Broadcast sends a message to all active sessions.
func (b *Bridge) Broadcast(msgType, content string) []error {
	sessions := b.activeSessions()
	var errs []error

	for _, sess := range sessions {
		if sess == b.sessionID {
			continue
		}
		if err := b.Send(sess, msgType, content); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// OnMessage registers a handler for incoming messages.
func (b *Bridge) OnMessage(handler func(Message)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers = append(b.handlers, handler)
}

// Receive returns the next message from the inbox (blocking).
func (b *Bridge) Receive() (Message, bool) {
	msg, ok := <-b.inbox
	return msg, ok
}

// ActiveSessions returns IDs of sessions with active sockets.
func (b *Bridge) ActiveSessions() []string {
	return b.activeSessions()
}

func (b *Bridge) acceptLoop() {
	for {
		conn, err := b.listener.Accept()
		if err != nil {
			return // listener closed
		}
		go b.handleConnection(conn)
	}
}

func (b *Bridge) handleConnection(conn net.Conn) {
	defer conn.Close()

	var msg Message
	if err := json.NewDecoder(conn).Decode(&msg); err != nil {
		return
	}

	// Deliver to inbox
	select {
	case b.inbox <- msg:
	default:
		// Inbox full, drop message
	}

	// Notify handlers
	b.mu.RLock()
	handlers := make([]func(Message), len(b.handlers))
	copy(handlers, b.handlers)
	b.mu.RUnlock()

	for _, h := range handlers {
		h(msg)
	}
}

func (b *Bridge) socketPath(sessionID string) string {
	return filepath.Join(b.socketDir, sessionID+".sock")
}

func (b *Bridge) activeSessions() []string {
	entries, err := os.ReadDir(b.socketDir)
	if err != nil {
		return nil
	}

	var sessions []string
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".sock" {
			sessID := e.Name()[:len(e.Name())-5]
			// Check if socket is alive
			conn, err := net.DialTimeout("unix", filepath.Join(b.socketDir, e.Name()), 500*time.Millisecond)
			if err == nil {
				conn.Close()
				sessions = append(sessions, sessID)
			} else {
				// Stale socket, clean up
				os.Remove(filepath.Join(b.socketDir, e.Name()))
			}
		}
	}
	return sessions
}
