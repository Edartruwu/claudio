package attachclient

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/attach"
	"golang.org/x/net/websocket"
)

// Client manages WebSocket connection to ComandCenter.
type Client struct {
	serverURL    string
	password     string
	name         string
	master       bool
	agentType    string
	teamTemplate string
	sessionID    string // CLI's internal session ID for task correlation
	conn         *websocket.Conn
	outbox       chan attach.Envelope
	onUserMsg      func(attach.UserMsgPayload)
	onInterrupt    func()
	onSetAgent     func(attach.SetAgentPayload)
	onSetTeam      func(attach.SetTeamPayload)
	onClearHistory func()
	onSetMessages  func([]api.Message)
	mu           sync.Mutex
	writeMu      sync.Mutex // protects all websocket writes (writeLoop + Close bye)
	closed       bool
	closedChan   chan struct{}
}

// New creates unconnected Client.
func New(serverURL, password, name string, master bool, agentType, teamTemplate string) *Client {
	return &Client{
		serverURL:  serverURL,
		password:   password,
		name:       name,
		master:       master,
		agentType:    agentType,
		teamTemplate: teamTemplate,
		closedChan:   make(chan struct{}),
	}
}

// SetSessionID sets the CLI session ID sent in the Hello handshake so the
// web UI can correlate team_tasks rows (written with this ID) to the session.
func (c *Client) SetSessionID(id string) {
	c.mu.Lock()
	c.sessionID = id
	c.mu.Unlock()
}

// Connect opens WebSocket to <serverURL>/ws/attach with Authorization header.
// Sends EventSessionHello immediately after connect.
// Starts goroutine to read inbound messages.
// Returns error if connection fails.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return fmt.Errorf("already connected")
	}

	wsURL := c.serverURL
	if strings.HasPrefix(wsURL, "https://") {
		wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
	} else {
		wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	}
	wsURL += "/ws/attach"

	origin := "http://localhost/"
	cfg, err := websocket.NewConfig(wsURL, origin)
	if err != nil {
		return fmt.Errorf("websocket config: %w", err)
	}
	cfg.Header = http.Header{
		"Authorization": []string{"Bearer " + c.password},
	}

	conn, err := websocket.DialConfig(cfg)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}

	c.conn = conn

	// Send hello
	cwd, _ := os.Getwd()
	hello := attach.HelloPayload{
		Name:         c.name,
		Path:         cwd,
		Master:       c.master,
		AgentType:    c.agentType,
		TeamTemplate: c.teamTemplate,
		SessionID:    c.sessionID,
	}
	if err := c.sendEnvelopeUnlocked(attach.EventSessionHello, hello); err != nil {
		conn.Close()
		c.conn = nil
		return fmt.Errorf("send hello: %w", err)
	}

	// Start outbox writer and read loop
	c.outbox = make(chan attach.Envelope, 1000)
	go c.writeLoop()
	go c.readLoop()

	return nil
}

// SendEvent marshals payload into Envelope and pushes to outbox channel.
// Returns immediately — actual WS write happens in writeLoop goroutine.
func (c *Client) SendEvent(eventType string, payload any) error {
	env, err := attach.NewEnvelope(eventType, payload)
	if err != nil {
		return err
	}

	select {
	case c.outbox <- env:
	default:
		log.Printf("[attachclient] outbox full, dropping event type=%s", eventType)
	}
	return nil
}

// writeLoop drains the outbox channel and writes envelopes to WS.
// Exits when outbox is closed (during Close()).
func (c *Client) writeLoop() {
	for envelope := range c.outbox {
		c.mu.Lock()
		conn := c.conn
		c.mu.Unlock()
		if conn == nil {
			continue
		}
		c.writeMu.Lock()
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := websocket.JSON.Send(conn, envelope); err != nil {
			log.Printf("[attachclient] write error: %v", err)
		}
		conn.SetWriteDeadline(time.Time{})
		c.writeMu.Unlock()
	}
}

// sendEnvelopeUnlocked sends envelope directly on the connection.
// Acquires writeMu to serialize with writeLoop. Safe to call without c.mu held.
func (c *Client) sendEnvelopeUnlocked(eventType string, payload any) error {
	env, err := attach.NewEnvelope(eventType, payload)
	if err != nil {
		return err
	}

	c.writeMu.Lock()
	c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	err = websocket.JSON.Send(c.conn, env)
	c.conn.SetWriteDeadline(time.Time{})
	c.writeMu.Unlock()
	return err
}

// OnUserMessage registers callback for inbound EventMsgUser messages.
func (c *Client) OnUserMessage(fn func(attach.UserMsgPayload)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onUserMsg = fn
}

// OnInterrupt registers callback invoked when the server sends EventInterrupt.
func (c *Client) OnInterrupt(fn func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onInterrupt = fn
}

// OnSetAgent registers callback invoked when the server sends EventSetAgent.
func (c *Client) OnSetAgent(fn func(attach.SetAgentPayload)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onSetAgent = fn
}

// OnClearHistory registers callback invoked when the server sends EventClearHistory.
func (c *Client) OnClearHistory(fn func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onClearHistory = fn
}

// OnSetMessages registers callback invoked when the server sends EventSetMessages.
func (c *Client) OnSetMessages(fn func([]api.Message)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onSetMessages = fn
}

// OnSetTeam registers callback invoked when the server sends EventSetTeam.
func (c *Client) OnSetTeam(fn func(attach.SetTeamPayload)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onSetTeam = fn
}

// Close sends EventSessionBye then closes connection.
func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed || c.conn == nil {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	conn := c.conn
	c.mu.Unlock()

	// Send bye directly (not via outbox — we're shutting down)
	_ = c.sendEnvelopeUnlocked(attach.EventSessionBye, nil)

	// Close outbox → writeLoop drains remaining + exits
	close(c.outbox)

	close(c.closedChan)

	// Close WS connection
	if conn != nil {
		return conn.Close()
	}

	return nil
}

// readDeadline is the maximum time to wait for a message from the server.
const readDeadline = 60 * time.Second

// readLoop reads inbound Envelopes and fires callbacks.
// Callbacks are copied under lock then invoked without lock. All registered
// callbacks (OnUserMessage, OnInterrupt, etc.) must be non-blocking — they
// send to buffered channels or perform fast in-memory ops. If a future
// callback could block, wrap its invocation in a goroutine to avoid stalling
// the read loop.
func (c *Client) readLoop() {
	// Set initial read deadline
	c.conn.SetReadDeadline(time.Now().Add(readDeadline))

	for {
		select {
		case <-c.closedChan:
			return
		default:
		}

		var env attach.Envelope
		if err := websocket.JSON.Receive(c.conn, &env); err != nil {
			log.Printf("read envelope: %v", err)
			return
		}

		// Reset read deadline after each successful receive
		c.conn.SetReadDeadline(time.Now().Add(readDeadline))

		switch env.Type {
		case attach.EventMsgUser:
			var payload attach.UserMsgPayload
			if err := env.UnmarshalPayload(&payload); err != nil {
				log.Printf("unmarshal payload: %v", err)
				continue
			}
			c.mu.Lock()
			cb := c.onUserMsg
			c.mu.Unlock()
			if cb != nil {
				cb(payload)
			}

		case attach.EventInterrupt:
			c.mu.Lock()
			cb := c.onInterrupt
			c.mu.Unlock()
			if cb != nil {
				cb()
			}

		case attach.EventSetAgent:
			var payload attach.SetAgentPayload
			if err := env.UnmarshalPayload(&payload); err != nil {
				log.Printf("unmarshal set_agent payload: %v", err)
				continue
			}
			c.mu.Lock()
			cb := c.onSetAgent
			c.mu.Unlock()
			if cb != nil {
				cb(payload)
			}

		case attach.EventSetTeam:
			var payload attach.SetTeamPayload
			if err := env.UnmarshalPayload(&payload); err != nil {
				log.Printf("unmarshal set_team payload: %v", err)
				continue
			}
			c.mu.Lock()
			cb := c.onSetTeam
			c.mu.Unlock()
			if cb != nil {
				cb(payload)
			}

		case attach.EventClearHistory:
			c.mu.Lock()
			cb := c.onClearHistory
			c.mu.Unlock()
			if cb != nil {
				cb()
			}

		case attach.EventSetMessages:
			var payload attach.SetMessagesPayload
			if err := env.UnmarshalPayload(&payload); err != nil {
				log.Printf("unmarshal set_messages payload: %v", err)
				continue
			}
			c.mu.Lock()
			cb := c.onSetMessages
			c.mu.Unlock()
			if cb != nil {
				cb(payload.Messages)
			}
		}
	}
}
