// Package web provides the browser UI for ComandCenter.
package web

import (
	"bytes"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"time"

	cc "github.com/Abraxas-365/claudio/internal/comandcenter"
	"github.com/Abraxas-365/claudio/internal/attach"
	"golang.org/x/net/websocket"
)

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

// WebServer serves the browser UI for ComandCenter.
type WebServer struct {
	storage  *cc.Storage
	hub      *cc.Hub
	password string

	mu      sync.RWMutex
	clients map[*uiClient]struct{}
}

// NewWebServer creates a WebServer.
func NewWebServer(storage *cc.Storage, hub *cc.Hub, password string) *WebServer {
	ws := &WebServer{
		storage:  storage,
		hub:      hub,
		password: password,
		clients:  make(map[*uiClient]struct{}),
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
	mux.Handle("GET /partials/sessions", ws.uiAuth(http.HandlerFunc(ws.handlePartialSessions)))
	mux.Handle("GET /partials/messages/{session_id}", ws.uiAuth(http.HandlerFunc(ws.handlePartialMessages)))
	mux.Handle("GET /ws/ui", ws.uiAuth(http.HandlerFunc(ws.handleWSUI)))
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
	Session      cc.Session
	LastMessage  string
	UnreadCount  int
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
	msgs, err := ws.storage.ListMessages(id, 100)
	if err != nil {
		msgs = nil
	}
	// ListMessages returns newest first; reverse for display.
	reversed := reverseMessages(msgs)
	chatViewTmpl.execute(w, "chat_view.html", map[string]any{
		"Session":   sess,
		"Messages":  reversed,
		"SessionID": id,
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
	messagesTmpl.execute(w, "messages-partial", reversed)
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

// fanout reads UIBroadcast events and forwards rendered HTML to interested clients.
func (ws *WebServer) fanout() {
	ch := ws.hub.UIBroadcast()
	for ev := range ch {
		if ev.Envelope.Type != attach.EventMsgAssistant && ev.Envelope.Type != attach.EventMsgToolUse {
			continue
		}

		// Render message bubble for this event.
		msg := envelopeToMessage(ev)
		if msg == nil {
			continue
		}

		var buf bytes.Buffer
		if err := bubbleTmpl.ExecuteTemplate(&buf, "message-bubble", msg); err != nil {
			continue
		}

		payload, err := json.Marshal(map[string]string{
			"type": "new_message",
			"html": buf.String(),
		})
		if err != nil {
			continue
		}

		// Send to all clients watching this session.
		ws.mu.RLock()
		for client := range ws.clients {
			if client.sessionID == ev.SessionID {
				select {
				case client.send <- payload:
				default: // drop if full
				}
			}
		}
		ws.mu.RUnlock()
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

// buildSessionRows fetches the last message for each session.
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
