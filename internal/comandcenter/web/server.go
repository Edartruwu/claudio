// Package web provides the browser UI for ComandCenter.
package web

import (
	"bytes"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	cc "github.com/Abraxas-365/claudio/internal/comandcenter"
	"github.com/Abraxas-365/claudio/internal/attach"
	"golang.org/x/net/websocket"
)

// mentionRe matches "@Name message" at the start of content.
// Group 1 = session name, Group 2 = message body.
var mentionRe = regexp.MustCompile(`^@(\w[\w\s]*?)\s+(.+)$`)

//go:embed templates static
var staticFS embed.FS

// templateSet holds a parsed template set.
type templateSet struct {
	t *template.Template
}

func (ts *templateSet) execute(w http.ResponseWriter, name string, data any) {
	if err := ts.t.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
	}
}

// Templates parsed at package init. Each page has its own template.Template
// so that {{define "content"}} blocks don't collide across pages.
var (
	loginTmpl    *templateSet
	chatListTmpl *templateSet
	chatViewTmpl *templateSet
	sessionsTmpl *templateSet
	messagesTmpl *templateSet
	infoTmpl     *templateSet
	bubbleTmpl   *template.Template
)

func funcMap() template.FuncMap {
	return template.FuncMap{
		"relTime": func(t time.Time) string {
			d := time.Since(t)
			switch {
			case d < time.Minute:
				return "just now"
			case d < time.Hour:
				m := int(d.Minutes())
				if m == 1 {
					return "1 min ago"
				}
				return strings.Join([]string{itoa(m), " mins ago"}, "")
			case d < 24*time.Hour:
				h := int(d.Hours())
				if h == 1 {
					return "1 hr ago"
				}
				return strings.Join([]string{itoa(h), " hrs ago"}, "")
			default:
				return t.Format("Jan 2")
			}
		},
		"firstChar": func(s string) string {
			if len(s) == 0 {
				return "?"
			}
			r := []rune(s)
			return strings.ToUpper(string(r[0]))
		},
		"truncate": func(n int, s string) string {
			r := []rune(s)
			if len(r) <= n {
				return s
			}
			return string(r[:n]) + "…"
		},
		"avatarColor": func(s string) string {
			colors := []string{"#25D366", "#128C7E", "#075E54", "#34B7F1", "#8E44AD"}
			if len(s) == 0 {
				return colors[0]
			}
			return colors[len(s)%len(colors)]
		},
		// taskStatusDot returns a Tailwind-compatible inline style color class for a task status dot.
		"taskStatusDot": func(status string) string {
			switch status {
			case "in_progress":
				return "bg-blue-500"
			case "done":
				return "bg-green-500"
			case "blocked":
				return "bg-red-500"
			default: // pending
				return "bg-gray-500"
			}
		},
		// taskStatusBadge returns inline style classes for a task status pill badge.
		"taskStatusBadge": func(status string) string {
			switch status {
			case "in_progress":
				return "text-blue-400 bg-blue-400/10"
			case "done":
				return "text-green-400 bg-green-400/10"
			case "blocked":
				return "text-red-400 bg-red-400/10"
			default: // pending
				return "text-gray-400 bg-gray-400/10"
			}
		},
		// agentStatusDot returns a dot color class for agent status.
		"agentStatusDot": func(status string) string {
			switch status {
			case "working":
				return "bg-blue-500"
			case "idle":
				return "bg-gray-500"
			default:
				return "bg-gray-600"
			}
		},
		// agentStatusColor returns text color class for agent status label.
		"agentStatusColor": func(status string) string {
			switch status {
			case "working":
				return "text-blue-400"
			default:
				return "text-gray-400"
			}
		},
		// isImage reports whether a MIME type is an image.
		"isImage": func(mimeType string) bool {
			return strings.HasPrefix(mimeType, "image/")
		},
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 10)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	return string(buf)
}

func mustParseFS(files ...string) *templateSet {
	t, err := template.New("").Funcs(funcMap()).ParseFS(staticFS, files...)
	if err != nil {
		panic("web: parse templates " + strings.Join(files, ",") + ": " + err.Error())
	}
	return &templateSet{t: t}
}

func init() {
	loginTmpl = mustParseFS(
		"templates/layout.html",
		"templates/login.html",
	)
	chatListTmpl = mustParseFS(
		"templates/layout.html",
		"templates/chat_list.html",
		"templates/components/session_row.html",
	)
	chatViewTmpl = mustParseFS(
		"templates/layout.html",
		"templates/chat_view.html",
		"templates/components/message_bubble.html",
	)
	sessionsTmpl = mustParseFS(
		"templates/partials/sessions.html",
		"templates/components/session_row.html",
	)
	messagesTmpl = mustParseFS(
		"templates/partials/messages.html",
		"templates/components/message_bubble.html",
	)
	infoTmpl = mustParseFS(
		"templates/layout.html",
		"templates/info_panel.html",
	)
	t, err := template.New("").Funcs(funcMap()).ParseFS(staticFS, "templates/components/message_bubble.html")
	if err != nil {
		panic("web: parse bubble template: " + err.Error())
	}
	bubbleTmpl = t
}

// uiClient is a browser WebSocket connection watching a session.
type uiClient struct {
	sessionID string
	send      chan []byte
}

// MessageView wraps a cc.Message with its associated attachments for template rendering.
type MessageView struct {
	cc.Message
	Attachments []cc.Attachment
}

// WebServer serves the browser UI for ComandCenter.
type WebServer struct {
	storage    *cc.Storage
	hub        *cc.Hub
	password   string
	uploadsDir string

	mu      sync.RWMutex
	clients map[*uiClient]struct{}
}

// NewWebServer creates a WebServer. uploadsDir is the base directory for uploaded files.
func NewWebServer(storage *cc.Storage, hub *cc.Hub, password, uploadsDir string) *WebServer {
	ws := &WebServer{
		storage:  storage,
		hub:      hub,
		password: password,
		clients:    make(map[*uiClient]struct{}),
		uploadsDir: uploadsDir,
	}
	go ws.fanout()
	return ws
}

// RegisterRoutes mounts all UI routes on mux.
func (ws *WebServer) RegisterRoutes(mux *http.ServeMux) {
	// Static files — sub-FS so URL /static/foo maps to embed path static/foo.
	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic("web: static sub-FS: " + err.Error())
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticSub)))

	// No-auth routes.
	mux.HandleFunc("GET /login", ws.handleLoginGet)
	mux.HandleFunc("POST /login", ws.handleLoginPost)

	// Auth-gated routes.
	mux.Handle("GET /", ws.uiAuth(http.HandlerFunc(ws.handleChatList)))
	mux.Handle("GET /chat/{session_id}", ws.uiAuth(http.HandlerFunc(ws.handleChatView)))
	mux.Handle("GET /chat/{session_id}/info", ws.uiAuth(http.HandlerFunc(ws.handleSessionInfo)))
	mux.Handle("GET /partials/sessions", ws.uiAuth(http.HandlerFunc(ws.handlePartialSessions)))
	mux.Handle("GET /partials/messages/{session_id}", ws.uiAuth(http.HandlerFunc(ws.handlePartialMessages)))
	mux.Handle("POST /api/sessions/{session_id}/message", ws.uiAuth(http.HandlerFunc(ws.handleSendMessage)))
	mux.Handle("POST /api/sessions/by-name/{name}/message", ws.uiAuth(http.HandlerFunc(ws.handleSendMessageByName)))
	mux.Handle("GET /api/sessions/list", ws.uiAuth(http.HandlerFunc(ws.handleAPISessions)))
	mux.Handle("POST /api/sessions/{session_id}/upload", ws.uiAuth(http.HandlerFunc(ws.handleUpload)))
	mux.Handle("GET /uploads/{session_id}/{filename}", ws.uiAuth(http.HandlerFunc(ws.handleServeFile)))
	mux.Handle("GET /ws/ui", ws.uiAuth(http.HandlerFunc(ws.handleWSUI)))

	// Sessions JSON API — used by @mention autocomplete.


	// Session management API.
	mux.Handle("PATCH /api/sessions/{id}/archive", ws.uiAuth(http.HandlerFunc(ws.handleArchiveSession)))
	mux.Handle("DELETE /api/sessions/{id}", ws.uiAuth(http.HandlerFunc(ws.handleDeleteSession)))
}

// uiAuth checks the "auth" HttpOnly cookie.
func (ws *WebServer) uiAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("auth")
		if err != nil || !ws.validPassword(c.Value) {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (ws *WebServer) validPassword(token string) bool {
	if token == "" || ws.password == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(ws.password)) == 1
}

// --- Handlers ---

func (ws *WebServer) handleLoginGet(w http.ResponseWriter, r *http.Request) {
	loginTmpl.execute(w, "login.html", map[string]any{"Error": ""})
}

func (ws *WebServer) handleLoginPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	pass := r.FormValue("password")
	if !ws.validPassword(pass) {
		w.WriteHeader(http.StatusUnauthorized)
		loginTmpl.execute(w, "login.html", map[string]any{"Error": "Invalid password"})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "auth",
		Value:    pass,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

type sessionRow struct {
	Session     cc.Session
	LastMessage string
	UnreadCount int
}

// InfoPageData holds data for the session info panel.
type InfoPageData struct {
	Session   cc.Session
	Tasks     []cc.Task
	Agents    []cc.Agent
	SessionID string
	Images    []cc.Attachment // image attachments for media grid
	Docs      []cc.Attachment // non-image attachments for document list
}

func (ws *WebServer) handleChatList(w http.ResponseWriter, r *http.Request) {
	sessions, err := ws.storage.ListSessions()
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	rows := ws.buildSessionRows(sessions)
	chatListTmpl.execute(w, "chat_list.html", map[string]any{
		"Rows":      rows,
		"SessionID": "",
	})
}

func (ws *WebServer) handleChatView(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("session_id")
	sess, err := ws.storage.GetSession(id)
	if err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	// Mark as read when chat view opens.
	_ = ws.storage.MarkRead(id)
	msgs, err := ws.storage.ListMessages(id, 100)
	if err != nil {
		msgs = nil
	}
	// ListMessages returns newest first; reverse for display.
	reversed := reverseMessages(msgs)

	// Load all session attachments and group by message_id for O(1) lookup.
	allAtts, _ := ws.storage.ListAttachments(id)
	attsByMsg := make(map[string][]cc.Attachment, len(allAtts))
	for _, att := range allAtts {
		if att.MessageID != "" {
			attsByMsg[att.MessageID] = append(attsByMsg[att.MessageID], att)
		}
	}
	views := make([]MessageView, len(reversed))
	for i, m := range reversed {
		views[i] = MessageView{Message: m, Attachments: attsByMsg[m.ID]}
	}

	chatViewTmpl.execute(w, "chat_view.html", map[string]any{
		"Session":   sess,
		"Messages":  views,
		"SessionID": id,
	})
}

func (ws *WebServer) handleSessionInfo(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("session_id")
	sess, err := ws.storage.GetSession(id)
	if err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	tasks, err := ws.storage.ListTasks(id)
	if err != nil {
		tasks = nil
	}
	agents, err := ws.storage.ListAgents(id)
	if err != nil {
		agents = nil
	}
	allAtts, _ := ws.storage.ListAttachments(id)
	var images, docs []cc.Attachment
	for _, att := range allAtts {
		if strings.HasPrefix(att.MimeType, "image/") {
			images = append(images, att)
		} else {
			docs = append(docs, att)
		}
	}
	infoTmpl.execute(w, "info_panel.html", InfoPageData{
		Session:   sess,
		Tasks:     tasks,
		Agents:    agents,
		SessionID: id,
		Images:    images,
		Docs:      docs,
	})
}

func (ws *WebServer) handlePartialSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := ws.storage.ListSessions()
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	rows := ws.buildSessionRows(sessions)
	sessionsTmpl.execute(w, "sessions-partial", rows)
}

func (ws *WebServer) handlePartialMessages(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("session_id")
	msgs, err := ws.storage.ListMessages(id, 100)
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	reversed := reverseMessages(msgs)

	allAtts, _ := ws.storage.ListAttachments(id)
	attsByMsg := make(map[string][]cc.Attachment, len(allAtts))
	for _, att := range allAtts {
		if att.MessageID != "" {
			attsByMsg[att.MessageID] = append(attsByMsg[att.MessageID], att)
		}
	}
	views := make([]MessageView, len(reversed))
	for i, m := range reversed {
		views[i] = MessageView{Message: m, Attachments: attsByMsg[m.ID]}
	}
	messagesTmpl.execute(w, "messages-partial", views)
}

func (ws *WebServer) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("session_id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	content := r.FormValue("content")
	if content == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Detect @mention: "@Name message body"
	if m := mentionRe.FindStringSubmatch(content); m != nil {
		targetName := m[1]
		msgBody := m[2]
		ws.handleMentionRoute(w, sessionID, targetName, msgBody, content)
		return
	}

	// Normal (non-@mention) path — existing behavior.
	payload, _ := json.Marshal(attach.UserMsgPayload{Content: content})
	env := attach.Envelope{Type: attach.EventMsgUser, Payload: payload}
	if err := ws.hub.Send(sessionID, env); err != nil {
		http.Error(w, "session not connected", http.StatusServiceUnavailable)
		return
	}

	msg := cc.Message{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		SessionID: sessionID,
		Role:      "user",
		Content:   content,
		CreatedAt: time.Now(),
	}
	_ = ws.storage.InsertMessage(msg)

	// Push user bubble to UI WS clients immediately (no refresh needed).
	var buf bytes.Buffer
	if err := bubbleTmpl.ExecuteTemplate(&buf, "message-bubble", MessageView{Message: msg}); err == nil {
		if payload, err := json.Marshal(map[string]string{
			"type": "message.user",
			"html": buf.String(),
		}); err == nil {
			ws.pushToSessionClients(sessionID, payload)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleMentionRoute handles @Name routing: stores in origin session with reply metadata,
// routes a copy to the target session, and pushes UI events to both sessions' WS clients.
func (ws *WebServer) handleMentionRoute(w http.ResponseWriter, originID, targetName, msgBody, fullContent string) {
	// Resolve target session by name.
	targetSess, found, err := ws.storage.GetSessionByName(targetName)
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	// Route message body to target session via hub.
	payload, _ := json.Marshal(attach.UserMsgPayload{Content: msgBody})
	env := attach.Envelope{Type: attach.EventMsgUser, Payload: payload}
	if err := ws.hub.Send(targetSess.ID, env); err != nil {
		http.Error(w, "session not connected", http.StatusServiceUnavailable)
		return
	}

	now := time.Now()

	// Quoted content: first 80 runes of full message.
	quoted := fullContent
	if r := []rune(fullContent); len(r) > 80 {
		quoted = string(r[:80])
	}

	// Store in originating session with reply metadata.
	originMsg := cc.Message{
		ID:             fmt.Sprintf("%d", now.UnixNano()),
		SessionID:      originID,
		Role:           "user",
		Content:        fullContent,
		CreatedAt:      now,
		ReplyToSession: targetName,
		QuotedContent:  quoted,
	}
	_ = ws.storage.InsertMessage(originMsg)

	// Store copy in target session (plain user message, no reply fields).
	targetMsg := cc.Message{
		ID:        fmt.Sprintf("%da", now.UnixNano()),
		SessionID: targetSess.ID,
		Role:      "user",
		Content:   msgBody,
		CreatedAt: now,
	}
	_ = ws.storage.InsertMessage(targetMsg)

	// Push bubbles to both sessions' WS clients.
	ws.pushMsgBubble(originID, originMsg)
	ws.pushMsgBubble(targetSess.ID, targetMsg)

	w.WriteHeader(http.StatusNoContent)
}

// handleSendMessageByName handles POST /api/sessions/by-name/{name}/message.
// Looks up the session by name, then sends the message via hub.
func (ws *WebServer) handleSendMessageByName(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	content := r.FormValue("content")
	if content == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	sess, found, err := ws.storage.GetSessionByName(name)
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	payload, _ := json.Marshal(attach.UserMsgPayload{Content: content})
	env := attach.Envelope{Type: attach.EventMsgUser, Payload: payload}
	if err := ws.hub.Send(sess.ID, env); err != nil {
		http.Error(w, "session not connected", http.StatusServiceUnavailable)
		return
	}

	msg := cc.Message{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		SessionID: sess.ID,
		Role:      "user",
		Content:   content,
		CreatedAt: time.Now(),
	}
	_ = ws.storage.InsertMessage(msg)

	var buf bytes.Buffer
	if err := bubbleTmpl.ExecuteTemplate(&buf, "message-bubble", MessageView{Message: msg}); err == nil {
		if p, err := json.Marshal(map[string]string{
			"type": "message.user",
			"html": buf.String(),
		}); err == nil {
			ws.pushToSessionClients(sess.ID, p)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// pushMsgBubble renders and pushes a user message bubble to a session's WS clients.
func (ws *WebServer) pushMsgBubble(sessionID string, msg cc.Message) {
	var buf bytes.Buffer
	if err := bubbleTmpl.ExecuteTemplate(&buf, "message-bubble", MessageView{Message: msg}); err == nil {
		if p, err := json.Marshal(map[string]string{
			"type": "message.user",
			"html": buf.String(),
		}); err == nil {
			ws.pushToSessionClients(sessionID, p)
		}
	}
}

// handleWSUI upgrades to WebSocket and streams new messages to the browser.
func (ws *WebServer) handleWSUI(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	wsServer := websocket.Server{
		Handshake: func(cfg *websocket.Config, req *http.Request) error {
			return nil
		},
		Handler: websocket.Handler(func(conn *websocket.Conn) {
			client := &uiClient{
				sessionID: sessionID,
				send:      make(chan []byte, 64),
			}
			ws.addClient(client)
			defer ws.removeClient(client)

			// Write loop: send messages until connection closes or send closes.
			done := make(chan struct{})
			go func() {
				defer close(done)
				// Read loop: detect client disconnect.
				buf := make([]byte, 1)
				for {
					if _, err := conn.Read(buf); err != nil {
						return
					}
				}
			}()

			for {
				select {
				case msg, ok := <-client.send:
					if !ok {
						return
					}
					if _, err := conn.Write(msg); err != nil {
						return
					}
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
			default: // drop if full
			}
		}
	}
}

// fanout reads UIBroadcast events and forwards rendered HTML to interested clients.
func (ws *WebServer) fanout() {
	ch := ws.hub.UIBroadcast()
	for ev := range ch {
		switch ev.Envelope.Type {
		case attach.EventMsgAssistant:
			msg := envelopeToMessage(ev)
			if msg == nil {
				continue
			}
			var buf bytes.Buffer
			if err := bubbleTmpl.ExecuteTemplate(&buf, "message-bubble", MessageView{Message: *msg}); err != nil {
				continue
			}
			payload, err := json.Marshal(map[string]string{
				"type": "message.assistant",
				"html": buf.String(),
			})
			if err != nil {
				continue
			}
			ws.pushToSessionClients(ev.SessionID, payload)

		case attach.EventMsgToolUse:
			// 1. Permanent tool-use bubble in chat history.
			msg := envelopeToMessage(ev)
			if msg == nil {
				continue
			}
			var buf bytes.Buffer
			if err := bubbleTmpl.ExecuteTemplate(&buf, "message-bubble", MessageView{Message: *msg}); err != nil {
				continue
			}
			bubblePayload, err := json.Marshal(map[string]string{
				"type": "message.tool_use",
				"html": buf.String(),
			})
			if err != nil {
				continue
			}
			ws.pushToSessionClients(ev.SessionID, bubblePayload)

			// 2. Transient typing indicator with tool + agent name.
			var p attach.ToolUsePayload
			if err := ev.Envelope.UnmarshalPayload(&p); err != nil {
				continue
			}
			typingPayload, err := json.Marshal(map[string]string{
				"type":      "typing",
				"tool":      p.Tool,
				"agentName": p.AgentName,
			})
			if err != nil {
				continue
			}
			ws.pushToSessionClients(ev.SessionID, typingPayload)
		}
	}
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

// handleArchiveSession sets a session's status to 'archived' and returns 200 with empty body.
// htmx swaps the row with the empty response, removing it from the DOM.
func (ws *WebServer) handleArchiveSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := ws.storage.ArchiveSession(id); err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleDeleteSession permanently deletes a session + all its messages/tasks/agents.
// Returns 200 with empty body so htmx removes the row via outerHTML swap.
func (ws *WebServer) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := ws.storage.DeleteSession(id); err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleAPISessions returns all non-archived sessions as JSON.
// Used by the @mention autocomplete in the chat UI.
func (ws *WebServer) handleAPISessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := ws.storage.ListSessions()
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	if sessions == nil {
		sessions = []cc.Session{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessions)
}

// buildSessionRows fetches the last message for each session and unread count.
func (ws *WebServer) buildSessionRows(sessions []cc.Session) []sessionRow {
	rows := make([]sessionRow, 0, len(sessions))
	for _, sess := range sessions {
		row := sessionRow{Session: sess}
		msgs, err := ws.storage.ListMessages(sess.ID, 1)
		if err == nil && len(msgs) > 0 {
			content := msgs[0].Content
			r := []rune(content)
			if len(r) > 60 {
				content = string(r[:60]) + "…"
			}
			row.LastMessage = content
		}
		// Populate unread count.
		count, err := ws.storage.UnreadCount(sess.ID)
		if err == nil {
			row.UnreadCount = count
		}
		rows = append(rows, row)
	}
	return rows
}

// reverseMessages reverses a slice (DB returns newest first; UI needs oldest first).
func reverseMessages(msgs []cc.Message) []cc.Message {
	out := make([]cc.Message, len(msgs))
	for i, m := range msgs {
		out[len(msgs)-1-i] = m
	}
	return out
}

// POST /api/sessions/{session_id}/upload
// Multipart form: "file" (required), "content" (optional caption).
func (ws *WebServer) handleUpload(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("session_id")

	// Validate session.
	if _, err := ws.storage.GetSession(sessionID); err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	// Parse multipart — 32 MB limit.
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "parse multipart failed", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file field missing", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Detect MIME from first 512 bytes then reset reader.
	sniff := make([]byte, 512)
	n, _ := file.Read(sniff)
	mimeType := http.DetectContentType(sniff[:n])
	// Also honour the content-type from the part header if available.
	if ct := header.Header.Get("Content-Type"); ct != "" && ct != "application/octet-stream" {
		mimeType = ct
	}

	// Strip MIME parameters (e.g. "image/jpeg; name=...").
	if idx := strings.Index(mimeType, ";"); idx != -1 {
		mimeType = strings.TrimSpace(mimeType[:idx])
	}

	// Seek back to start by seeking on the underlying file.
	if seeker, ok := file.(io.Seeker); ok {
		seeker.Seek(0, io.SeekStart)
	}

	ext := filepath.Ext(header.Filename)
	storedName := cc.NewID() + ext

	// Ensure per-session upload directory exists.
	dir := filepath.Join(ws.uploadsDir, sessionID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}

	// Write file to disk.
	dst, err := os.Create(filepath.Join(dir, storedName))
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	size, err := io.Copy(dst, file)
	dst.Close()
	if err != nil {
		http.Error(w, "write error", http.StatusInternalServerError)
		return
	}

	caption := strings.TrimSpace(r.FormValue("content"))
	if caption == "" {
		caption = header.Filename
	}

	now := time.Now()

	// Create a user message for this upload.
	msg := cc.Message{
		ID:        cc.NewID(),
		SessionID: sessionID,
		Role:      "user",
		Content:   caption,
		CreatedAt: now,
	}
	if err := ws.storage.InsertMessage(msg); err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	// Record attachment linked to the message.
	att := cc.Attachment{
		ID:           cc.NewID(),
		SessionID:    sessionID,
		MessageID:    msg.ID,
		Filename:     storedName,
		OriginalName: header.Filename,
		MimeType:     mimeType,
		Size:         size,
		CreatedAt:    now,
	}
	if err := ws.storage.InsertAttachment(att); err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	// Push bubble to WS clients.
	var buf bytes.Buffer
	view := MessageView{Message: msg, Attachments: []cc.Attachment{att}}
	if err := bubbleTmpl.ExecuteTemplate(&buf, "message-bubble", view); err == nil {
		payload, _ := json.Marshal(map[string]string{"type": "message.user", "html": buf.String()})
		ws.pushToSessionClients(sessionID, payload)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"url":          "/uploads/" + sessionID + "/" + storedName,
		"filename":     storedName,
		"originalName": header.Filename,
		"mimeType":     mimeType,
		"size":         size,
	})
}

// GET /uploads/{session_id}/{filename}
func (ws *WebServer) handleServeFile(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("session_id")
	filename := r.PathValue("filename")

	// Sanitize: prevent path traversal.
	if strings.ContainsAny(filename, "/\\..") {
		http.Error(w, "invalid filename", http.StatusBadRequest)
		return
	}

	path := filepath.Join(ws.uploadsDir, sessionID, filename)
	http.ServeFile(w, r, path)
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
		if len(p.Input) > 0 {
			content = p.Tool + ": " + string(p.Input)
		}
		return &cc.Message{
			SessionID: ev.SessionID,
			Role:      "tool_use",
			Content:   content,
			AgentName: p.AgentName,
			CreatedAt: now,
		}
	}
	return nil
}
