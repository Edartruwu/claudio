package attachclient

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

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
	conn         *websocket.Conn
	onUserMsg    func(attach.UserMsgPayload)
	onInterrupt  func()
	onSetAgent   func(attach.SetAgentPayload)
	onSetTeam    func(attach.SetTeamPayload)
	mu           sync.Mutex
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
	}
	if err := c.sendEnvelopeUnlocked(attach.EventSessionHello, hello); err != nil {
		conn.Close()
		c.conn = nil
		return fmt.Errorf("send hello: %w", err)
	}

	// Start read loop
	go c.readLoop()

	return nil
}

// SendEvent marshals payload into Envelope and writes to WS.
func (c *Client) SendEvent(eventType string, payload any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	return c.sendEnvelopeUnlocked(eventType, payload)
}

// sendEnvelopeUnlocked sends envelope without lock (caller responsible).
func (c *Client) sendEnvelopeUnlocked(eventType string, payload any) error {
	env, err := attach.NewEnvelope(eventType, payload)
	if err != nil {
		return err
	}

	return websocket.JSON.Send(c.conn, env)
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

	// Send bye
	_ = c.sendEnvelopeUnlocked(attach.EventSessionBye, nil)

	close(c.closedChan)

	// Close connection
	if conn != nil {
		return conn.Close()
	}

	return nil
}

// readLoop reads inbound Envelopes and fires callbacks.
func (c *Client) readLoop() {
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
		}
	}
}
