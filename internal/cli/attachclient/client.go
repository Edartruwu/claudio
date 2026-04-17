package attachclient

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/Abraxas-365/claudio/internal/attach"
)

// Client manages WebSocket connection to ComandCenter.
type Client struct {
	serverURL    string
	password     string
	name         string
	master       bool
	conn         net.Conn
	onUserMsg    func(attach.UserMsgPayload)
	mu           sync.Mutex
	closed       bool
	closedChan   chan struct{}
}

// New creates unconnected Client.
func New(serverURL, password, name string, master bool) *Client {
	return &Client{
		serverURL:  serverURL,
		password:   password,
		name:       name,
		master:     master,
		closedChan: make(chan struct{}),
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

	// Parse server URL
	u, err := url.Parse(c.serverURL)
	if err != nil {
		return fmt.Errorf("parse URL: %w", err)
	}

	// Establish TCP connection
	conn, err := net.Dial("tcp", u.Host)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}

	// HTTP upgrade request
	req := fmt.Sprintf(
		"GET /ws/attach HTTP/1.1\r\n"+
			"Host: %s\r\n"+
			"Upgrade: websocket\r\n"+
			"Connection: Upgrade\r\n"+
			"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n"+
			"Sec-WebSocket-Version: 13\r\n"+
			"Authorization: Bearer %s\r\n"+
			"\r\n",
		u.Host, c.password)

	if _, err := conn.Write([]byte(req)); err != nil {
		conn.Close()
		return fmt.Errorf("send upgrade: %w", err)
	}

	// Read response headers
	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, &http.Request{Method: "GET"})
	if err != nil {
		conn.Close()
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		conn.Close()
		return fmt.Errorf("upgrade failed: status %d", resp.StatusCode)
	}

	c.conn = conn

	// Send hello
	hello := attach.HelloPayload{
		Name:   c.name,
		Path:   "",
		Master: c.master,
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

	data, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	// Simple framing: length (4 bytes) + payload
	frame := make([]byte, 4+len(data))
	frame[0] = byte((len(data) >> 24) & 0xff)
	frame[1] = byte((len(data) >> 16) & 0xff)
	frame[2] = byte((len(data) >> 8) & 0xff)
	frame[3] = byte(len(data) & 0xff)
	copy(frame[4:], data)

	if _, err := c.conn.Write(frame); err != nil {
		return fmt.Errorf("write: %w", err)
	}

	return nil
}

// OnUserMessage registers callback for inbound EventMsgUser messages.
func (c *Client) OnUserMessage(fn func(attach.UserMsgPayload)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onUserMsg = fn
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
	reader := bufio.NewReader(c.conn)

	for {
		select {
		case <-c.closedChan:
			return
		default:
		}

		// Read frame length (4 bytes)
		lenBuf := make([]byte, 4)
		if _, err := reader.Read(lenBuf); err != nil {
			if strings.Contains(err.Error(), "use of closed network connection") {
				return
			}
			log.Printf("read length: %v", err)
			return
		}

		frameLen := int(lenBuf[0])<<24 | int(lenBuf[1])<<16 | int(lenBuf[2])<<8 | int(lenBuf[3])
		if frameLen <= 0 || frameLen > 10*1024*1024 { // sanity check: 10MB max
			log.Printf("invalid frame length: %d", frameLen)
			return
		}

		// Read frame payload
		frameBuf := make([]byte, frameLen)
		if _, err := reader.Read(frameBuf); err != nil {
			log.Printf("read frame: %v", err)
			return
		}

		// Unmarshal envelope
		var env attach.Envelope
		if err := json.Unmarshal(frameBuf, &env); err != nil {
			log.Printf("unmarshal: %v", err)
			continue
		}

		// Handle EventMsgUser
		if env.Type == attach.EventMsgUser {
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
		}
	}
}
