package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	cc "github.com/Abraxas-365/claudio/internal/comandcenter"
	"github.com/Abraxas-365/claudio/internal/attach"
	"golang.org/x/net/websocket"
)

// wsMaxMessageSize is the maximum allowed incoming WebSocket message size (1 MB).
const wsMaxMessageSize = 1 << 20

// wsPingInterval is how often the server sends a ping to connected UI clients.
const wsPingInterval = 30 * time.Second

// wsReadDeadline is the maximum time to wait for a message (including pong).
const wsReadDeadline = 60 * time.Second

// checkWSOrigin validates the Origin header during WebSocket upgrade.
// Allows localhost variants and the configured publicURL.
func (ws *WebServer) checkWSOrigin(cfg *websocket.Config, req *http.Request) error {
	origin := req.Header.Get("Origin")
	if origin == "" {
		// No origin header (non-browser clients) — allow
		return nil
	}

	// Always allow localhost variants
	lower := strings.ToLower(origin)
	for _, prefix := range []string{
		"http://localhost", "https://localhost",
		"http://127.0.0.1", "https://127.0.0.1",
		"http://[::1]", "https://[::1]",
	} {
		if strings.HasPrefix(lower, prefix) {
			return nil
		}
	}

	// Allow configured publicURL origin
	if ws.publicURL != "" {
		pub := strings.TrimRight(strings.ToLower(ws.publicURL), "/")
		if strings.HasPrefix(lower, pub) {
			return nil
		}
	}

	// Allow any origin matching the request's own host (e.g. Tailscale hostname)
	requestHost := req.Host
	if h, _, err := net.SplitHostPort(requestHost); err == nil {
		requestHost = h
	}
	// Strip scheme first, then port — order matters (SplitHostPort splits on "https:" otherwise)
	originHost := origin
	if idx := strings.Index(originHost, "://"); idx >= 0 {
		originHost = originHost[idx+3:]
	}
	if h, _, err := net.SplitHostPort(originHost); err == nil {
		originHost = h
	}
	if strings.EqualFold(originHost, requestHost) {
		return nil
	}

	return fmt.Errorf("websocket: origin %q not allowed", origin)
}

// handleWSUI upgrades to WebSocket and streams new messages to the browser.
func (ws *WebServer) handleWSUI(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	wsServer := websocket.Server{
		Handshake: ws.checkWSOrigin,
		Handler: websocket.Handler(func(conn *websocket.Conn) {
			// Set message size limit
			conn.MaxPayloadBytes = wsMaxMessageSize

			client := &uiClient{
				sessionID: sessionID,
				send:      make(chan []byte, 256),
			}
			ws.addClient(client)
			defer ws.removeClient(client)

			// Replay last known agent statuses so reconnecting clients see current state.
			if sessionID != "" {
				// Replay current agent states
				if agents, err := ws.storage.ListAgents(sessionID); err == nil {
					for _, a := range agents {
						payload, _ := json.Marshal(map[string]string{
							"type":   "agent_status",
							"name":   a.Name,
							"status": a.Status,
							"replay": "true",
						})
						select {
						case client.send <- payload:
						default:
						}
					}
				}
				// Replay recent terminal events with full payload (summary, report, etc.)
				// Inject "replay":"true" so client skips toast/notification for these.
				if events, err := ws.storage.GetLatestAgentEvents(sessionID); err == nil {
					for _, evt := range events {
						// Parse payload, inject replay flag, re-marshal.
						var payloadMap map[string]interface{}
						if json.Unmarshal([]byte(evt.Payload), &payloadMap) == nil {
							payloadMap["replay"] = "true"
							taggedPayload, _ := json.Marshal(payloadMap)
							env := attach.Envelope{
								Type:    attach.EventAgentStatus,
								Payload: json.RawMessage(taggedPayload),
							}
							data, _ := json.Marshal(env)
							select {
							case client.send <- data:
							default:
							}
						}
					}
				}
			}

			// Set initial read deadline
			conn.SetReadDeadline(time.Now().Add(wsReadDeadline))

			// Read loop: detect client disconnect + handle pong.
			done := make(chan struct{})
			go func() {
				defer close(done)
				for {
					var msg string
					if err := websocket.Message.Receive(conn, &msg); err != nil {
						return
					}
					// Reset read deadline on any received message
					conn.SetReadDeadline(time.Now().Add(wsReadDeadline))
					// Check for pong (app-level)
					if strings.Contains(msg, `"type":"pong"`) || strings.Contains(msg, `"type": "pong"`) {
						continue
					}
					// Other client messages ignored for now
				}
			}()

			// Ping ticker goroutine
			pingTicker := time.NewTicker(wsPingInterval)
			pingDone := make(chan struct{})
			go func() {
				defer pingTicker.Stop()
				pingMsg := `{"type":"ping"}`
				for {
					select {
					case <-pingTicker.C:
						conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
						if err := websocket.Message.Send(conn, pingMsg); err != nil {
							return
						}
						conn.SetWriteDeadline(time.Time{})
					case <-done:
						return
					case <-pingDone:
						return
					}
				}
			}()
			defer close(pingDone)

			// Write loop — on any exit, remove client so no silent drops accumulate.
			defer func() {
				ws.removeClient(client)
				conn.Close()
			}()
			for {
				select {
				case msg, ok := <-client.send:
					if !ok {
						return
					}
					conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
					if err := websocket.Message.Send(conn, string(msg)); err != nil {
						log.Printf("[ws] write error for session %s: %v", sessionID, err)
						return
					}
					conn.SetWriteDeadline(time.Time{})
				case <-done:
					return
				}
			}
		}),
	}
	wsServer.ServeHTTP(w, r)
}

// pushToSessionClients delivers payload bytes to all UI WS clients watching sessionID.
func (ws *WebServer) pushToSessionClients(sessionID string, payload []byte) {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	for client := range ws.clients {
		if client.sessionID == sessionID {
			select {
			case client.send <- payload:
			default:
				log.Printf("[ws] client send buffer full for session %s, dropping message", sessionID)
			}
		}
	}
}

// fanoutWorkers is the number of goroutines that render templates concurrently.
const fanoutWorkers = 8

// fanoutQueueSize is the buffer size for the render work queue.
const fanoutQueueSize = 512

// fanout reads UIBroadcast events and forwards them to a worker pool for
// async template rendering. The read loop never blocks on rendering.
// Exits when ws.done is closed or the UIBroadcast channel is closed.
func (ws *WebServer) fanout() {
	ch := ws.hub.UIBroadcast()
	renderQueue := make(chan cc.UIEvent, fanoutQueueSize)

	// Start worker pool.
	var wg sync.WaitGroup
	for i := 0; i < fanoutWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ev := range renderQueue {
				ws.fanoutHandleEvent(ev)
			}
		}()
	}

	// Read loop — forward events to workers, never block on rendering.
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				close(renderQueue)
				wg.Wait()
				return
			}
			select {
			case renderQueue <- ev:
			default:
				log.Printf("[ws] fanout render queue full, dropping event for session %s (type %s)", ev.SessionID, ev.Envelope.Type)
			}
		case <-ws.done:
			close(renderQueue)
			wg.Wait()
			return
		}
	}
}

// fanoutHandleEvent processes a single UIEvent with panic recovery and render timeouts.
func (ws *WebServer) fanoutHandleEvent(ev cc.UIEvent) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[fanout] recovered panic: %v", r)
		}
	}()

	// renderCtx provides a 10s deadline for template Render calls.
	renderCtx, renderCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer renderCancel()

	switch ev.Envelope.Type {
	case attach.EventMsgAssistant:
		msg := envelopeToMessage(ev)
		if msg == nil {
			return
		}
		// Skip agent status noise (⏳ agent — done, ✅ agent — done).
		if IsAgentStatusLine(msg.Content) {
			return
		}
		var buf bytes.Buffer
		if err := MessageBubble(MessageView{Message: *msg}).Render(renderCtx, &buf); err != nil {
			return
		}
		payload, err := json.Marshal(map[string]string{
			"type": "message.assistant",
			"html": buf.String(),
		})
		if err != nil {
			return
		}
		ws.pushToSessionClients(ev.SessionID, payload)

	case attach.EventMsgToolUse:
		// 1. Permanent tool-use bubble in chat history.
		msg := envelopeToMessage(ev)
		if msg == nil {
			return
		}
		var buf bytes.Buffer
		if err := MessageBubble(MessageView{Message: *msg}).Render(renderCtx, &buf); err != nil {
			return
		}
		bubblePayload, err := json.Marshal(map[string]string{
			"type": "message.tool_use",
			"html": buf.String(),
		})
		if err != nil {
			return
		}
		ws.pushToSessionClients(ev.SessionID, bubblePayload)

		// 2. Transient typing indicator with tool + agent name.
		var p attach.ToolUsePayload
		if err := ev.Envelope.UnmarshalPayload(&p); err != nil {
			return
		}
		typingPayload, err := json.Marshal(map[string]string{
			"type":      "typing",
			"tool":      p.Tool,
			"agentName": p.AgentName,
		})
		if err != nil {
			return
		}
		ws.pushToSessionClients(ev.SessionID, typingPayload)

	case attach.EventMsgToolResult:
		var p attach.ToolResultPayload
		if err := ev.Envelope.UnmarshalPayload(&p); err != nil {
			return
		}
		// Push updated bubble HTML with output filled in.
		resultPayload, err := json.Marshal(map[string]string{
			"type":      "message.tool_result",
			"toolUseID": p.ToolUseID,
			"output":    p.Output,
		})
		if err != nil {
			return
		}
		ws.pushToSessionClients(ev.SessionID, resultPayload)

	case attach.EventMsgStreamDelta:
		var p attach.StreamDeltaPayload
		if err := ev.Envelope.UnmarshalPayload(&p); err != nil {
			return
		}
		pushMsg, err := json.Marshal(map[string]interface{}{
			"type":        "message.stream_delta",
			"delta":       p.Delta,
			"accumulated": p.Accumulated,
		})
		if err != nil {
			return
		}
		ws.pushToSessionClients(ev.SessionID, pushMsg)

	case attach.EventDesignScreenshot:
		var p attach.DesignScreenshotPayload
		if err := ev.Envelope.UnmarshalPayload(&p); err != nil {
			return
		}
		ws.handleScreenshotPush(ev.SessionID, p)

	case attach.EventDesignBundleReady:
		var p attach.DesignBundlePayload
		if err := ev.Envelope.UnmarshalPayload(&p); err != nil {
			return
		}
		ws.handleBundleLinkPush(ev.SessionID, p)

	case attach.EventAgentStatus:
		var p attach.AgentStatusPayload
		if err := ev.Envelope.UnmarshalPayload(&p); err != nil {
			return
		}
		payload, err := json.Marshal(map[string]any{
			"type":         "agent_status",
			"name":         p.Name,
			"status":       p.Status,
			"summary":      p.Summary,
			"report_path":  p.ReportPath,
			"current_tool": p.CurrentTool,
			"call_count":   p.CallCount,
			"elapsed_secs": p.ElapsedSecs,
		})
		if err != nil {
			return
		}
		ws.pushToSessionClients(ev.SessionID, payload)

	case attach.EventClearHistory:
		payload, err := json.Marshal(map[string]string{
			"type": "messages.cleared",
		})
		if err != nil {
			return
		}
		ws.pushToSessionClients(ev.SessionID, payload)

	case attach.EventTaskCreated:
		payload, err := json.Marshal(map[string]string{
			"type": "task.created",
		})
		if err != nil {
			return
		}
		ws.pushToSessionClients(ev.SessionID, payload)

	case attach.EventTaskUpdated:
		payload, err := json.Marshal(map[string]string{
			"type": "task.updated",
		})
		if err != nil {
			return
		}
		ws.pushToSessionClients(ev.SessionID, payload)

	case attach.EventConfigChanged:
		var p attach.ConfigChangedPayload
		if err := ev.Envelope.UnmarshalPayload(&p); err != nil {
			return
		}
		payload, err := json.Marshal(map[string]string{
			"type":            "config.changed",
			"model":           p.Model,
			"permission_mode": p.PermissionMode,
			"output_style":    p.OutputStyle,
		})
		if err != nil {
			return
		}
		ws.pushToSessionClients(ev.SessionID, payload)

	case attach.EventAgentChanged:
		var p attach.AgentChangedPayload
		if err := ev.Envelope.UnmarshalPayload(&p); err != nil {
			return
		}
		payload, err := json.Marshal(map[string]string{
			"type":       "agent.changed",
			"agent_type": p.AgentType,
		})
		if err != nil {
			return
		}
		ws.pushToSessionClients(ev.SessionID, payload)

	case attach.EventAgentLog:
		var p attach.AgentLogPayload
		if err := ev.Envelope.UnmarshalPayload(&p); err != nil {
			return
		}
		payload, err := json.Marshal(map[string]string{
			"type":       "agent.log",
			"agent_name": p.AgentName,
			"session_id": p.SessionID,
		})
		if err != nil {
			return
		}
		ws.pushToSessionClients(ev.SessionID, payload)

	case attach.EventTeamChanged:
		var p attach.TeamChangedPayload
		if err := ev.Envelope.UnmarshalPayload(&p); err != nil {
			return
		}
		payload, err := json.Marshal(map[string]string{
			"type":          "team.changed",
			"team_template": p.TeamTemplate,
		})
		if err != nil {
			return
		}
		ws.pushToSessionClients(ev.SessionID, payload)
	}
}

// handleScreenshotPush copies a screenshot saved by the CLI into the uploads
// directory and pushes an image bubble to all browser clients watching the session.
func (ws *WebServer) handleScreenshotPush(sessionID string, p attach.DesignScreenshotPayload) {
	// 1. Copy screenshot into uploads dir so it's served at /uploads/{sessionID}/{filename}.
	destDir := filepath.Join(ws.uploadsDir, sessionID)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return
	}
	destPath := filepath.Join(destDir, p.Filename)
	if err := copyFile(p.FilePath, destPath); err != nil {
		return
	}

	fi, _ := os.Stat(destPath)
	var size int64
	if fi != nil {
		size = fi.Size()
	}

	// 2. Insert assistant message row (content empty; image attachment carries the payload).
	now := time.Now()
	msg := cc.Message{
		ID:        cc.NewID(),
		SessionID: sessionID,
		Role:      "assistant",
		Content:   "",
		CreatedAt: now,
	}
	if err := ws.storage.InsertMessage(msg); err != nil {
		return
	}

	// 3. Record attachment.
	att := cc.Attachment{
		ID:           cc.NewID(),
		SessionID:    sessionID,
		MessageID:    msg.ID,
		Filename:     p.Filename,
		OriginalName: p.Filename,
		MimeType:     "image/png",
		Size:         size,
		CreatedAt:    now,
	}
	if err := ws.storage.InsertAttachment(att); err != nil {
		return
	}

	// 4. Render message-bubble template and push to browser clients.
	rctx, rcancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer rcancel()
	var buf bytes.Buffer
	view := MessageView{Message: msg, Attachments: []cc.Attachment{att}}
	if err := MessageBubble(view).Render(rctx, &buf); err != nil {
		return
	}
	wsPayload, _ := json.Marshal(map[string]string{"type": "message.assistant", "html": buf.String()})
	ws.pushToSessionClients(sessionID, wsPayload)
}

// handleBundleLinkPush inserts an assistant message with a clickable link to
// the bundle HTML and pushes it to all browser clients watching the session.
func (ws *WebServer) handleBundleLinkPush(sessionID string, p attach.DesignBundlePayload) {
	now := time.Now()
	msgID := cc.NewID()

	// Link directly to the interactive session root (index.html), not the gallery.
	// BundleURL is .../bundle/mockup.html — strip to get the session root.
	interactiveURL := strings.TrimSuffix(p.BundleURL, "bundle/mockup.html")
	bundleURL := interactiveURL
	if ws.publicURL != "" && strings.HasPrefix(bundleURL, "/") {
		bundleURL = strings.TrimRight(ws.publicURL, "/") + bundleURL
	}

	msg := cc.Message{
		ID:        msgID,
		SessionID: sessionID,
		Role:      "bundle",
		Content:   bundleURL,
		AgentName: p.SessionName,
		CreatedAt: now,
	}
	if err := ws.storage.InsertMessage(msg); err != nil {
		return
	}

	bctx, bcancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer bcancel()
	var buf bytes.Buffer
	if err := MessageBubble(MessageView{Message: msg}).Render(bctx, &buf); err != nil {
		return
	}
	wsPayload, _ := json.Marshal(map[string]string{"type": "message.assistant", "html": buf.String()})
	ws.pushToSessionClients(sessionID, wsPayload)
}

// copyFile copies src to dst, creating dst if it does not exist.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func (ws *WebServer) addClient(c *uiClient) {
	ws.mu.Lock()
	ws.clients[c] = struct{}{}
	ws.mu.Unlock()
}

func (ws *WebServer) removeClient(c *uiClient) {
	ws.mu.Lock()
	delete(ws.clients, c)
	ws.mu.Unlock()
}

// envelopeToMessage converts a UIEvent envelope to a displayable Message-like struct.
func envelopeToMessage(ev cc.UIEvent) *cc.Message {
	now := time.Now()
	switch ev.Envelope.Type {
	case attach.EventMsgAssistant:
		var p attach.AssistantMsgPayload
		if err := ev.Envelope.UnmarshalPayload(&p); err != nil {
			return nil
		}
		return &cc.Message{
			SessionID: ev.SessionID,
			Role:      "assistant",
			Content:   p.Content,
			AgentName: p.AgentName,
			CreatedAt: now,
		}
	case attach.EventMsgToolUse:
		var p attach.ToolUsePayload
		if err := ev.Envelope.UnmarshalPayload(&p); err != nil {
			return nil
		}
		content := p.Tool
		if len(p.Input) > 0 && string(p.Input) != "null" {
			content = p.Tool + ": " + string(p.Input)
		}
		return &cc.Message{
			SessionID: ev.SessionID,
			Role:      "tool_use",
			Content:   content,
			AgentName: p.AgentName,
			ToolUseID: p.ID,
			CreatedAt: now,
		}
	}
	return nil
}
