package comandcenter

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Abraxas-365/claudio/internal/attach"
	"golang.org/x/net/websocket"
)

// Server is the ComandCenter HTTP server.
type Server struct {
	password        string
	storage         *Storage
	hub             *Hub
	dataDir         string
	mux             *http.ServeMux
	vapidPublicKey  string
	vapidPrivateKey string
}

// NewServer creates a Server and registers all routes.
func NewServer(password string, storage *Storage, hub *Hub, dataDir string) *Server {
	pub, priv, err := storage.GetOrCreateVAPIDKeys()
	if err != nil {
		// Non-fatal: push won't work but server still starts.
		pub, priv = "", ""
	}

	s := &Server{
		password:        password,
		storage:         storage,
		hub:             hub,
		dataDir:         dataDir,
		mux:             http.NewServeMux(),
		vapidPublicKey:  pub,
		vapidPrivateKey: priv,
	}

	// Wire VAPID keys into hub so it can send push notifications.
	hub.SetVAPIDKeys(pub, priv)

	s.routes()
	return s
}

// Mux returns the underlying ServeMux so external packages can register
// additional routes (e.g., the browser UI) without an import cycle.
func (s *Server) Mux() *http.ServeMux {
	return s.mux
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	// No-auth routes.
	s.mux.HandleFunc("GET /health", s.handleHealth)

	// Auth-gated routes.
	s.mux.Handle("GET /ws/attach", s.auth(http.HandlerFunc(s.handleWSAttach)))
	s.mux.Handle("GET /api/sessions", s.auth(http.HandlerFunc(s.handleListSessions)))
	s.mux.Handle("POST /api/sessions", s.auth(http.HandlerFunc(s.handlePreRegisterSession)))
	s.mux.Handle("GET /api/sessions/{id}/messages", s.auth(http.HandlerFunc(s.handleListMessages)))

	// Push notification routes registered in web/routes.go

}

// auth is a middleware that checks the Authorization: Bearer <password> header.
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if !s.validToken(token) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) validToken(token string) bool {
	if token == "" || s.password == "" {
		return false
	}
	a := []byte(token)
	b := []byte(s.password)
	return subtle.ConstantTimeCompare(a, b) == 1
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(h, "Bearer ")
}

// GET /health
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GET /ws/attach — WebSocket endpoint for Claudio sessions.
func (s *Server) handleWSAttach(w http.ResponseWriter, r *http.Request) {
	wsServer := websocket.Server{
		Handshake: func(cfg *websocket.Config, req *http.Request) error {
			return nil // skip origin check; server-to-server
		},
		Handler: websocket.Handler(func(conn *websocket.Conn) {
			s.hub.HandleSession(conn)
		}),
	}
	wsServer.ServeHTTP(w, r)
}

// GET /api/sessions
func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.storage.ListSessions("")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if sessions == nil {
		sessions = []Session{}
	}
	writeJSON(w, http.StatusOK, sessions)
}

// POST /api/sessions — pre-register a session so it appears "active" before the claudio
// process has had time to connect and send EventSessionHello. SpawnSession calls this
// immediately after launching the child process to avoid the "off" flash in the web UI.
func (s *Server) handlePreRegisterSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name   string `json:"name"`
		Path   string `json:"path"`
		Master bool   `json:"master,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	// Reuse existing session ID if one exists for this name (so history is preserved).
	existing, found, err := s.storage.GetSessionByName(body.Name)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	now := time.Now()
	sess := Session{
		Name:         body.Name,
		Path:         body.Path,
		Master:       body.Master,
		Status:       "active",
		CreatedAt:    now,
		LastActiveAt: now,
	}
	if found {
		sess.ID = existing.ID
		sess.CreatedAt = existing.CreatedAt
		sess.Model = existing.Model
	} else {
		sess.ID = newID()
	}

	if err := s.storage.UpsertSession(sess); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

// GET /api/sessions/{id}/messages?limit=50
func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	limit := 50
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if n, err := strconv.Atoi(lStr); err == nil && n > 0 {
			limit = n
		}
	}

	msgs, err := s.storage.ListMessages(id, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if msgs == nil {
		msgs = []Message{}
	}
	writeJSON(w, http.StatusOK, msgs)
}

// POST /api/sessions/{id}/message
// Body: {"content": "..."}
func (s *Server) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body failed"})
		return
	}

	var req struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	payload := attach.UserMsgPayload{Content: req.Content}
	env, err := attach.NewEnvelope(attach.EventMsgUser, payload)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "envelope build failed"})
		return
	}

	if err := s.hub.Send(id, env); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("send failed: %v", err)})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

// GET /api/vapid-public-key — returns the server VAPID public key for JS subscription.
func (s *Server) handleVAPIDPublicKey(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"publicKey": s.vapidPublicKey})
}

// POST /api/push/subscribe — saves a push subscription.
// Body: {"endpoint":"...","keys":{"p256dh":"...","auth":"..."}}
func (s *Server) handlePushSubscribe(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body failed"})
		return
	}

	var req struct {
		Endpoint string `json:"endpoint"`
		Keys     struct {
			P256dh string `json:"p256dh"`
			Auth   string `json:"auth"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Endpoint == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid subscription"})
		return
	}

	sub := PushSubscription{
		ID:        newID(),
		Endpoint:  req.Endpoint,
		P256dh:    req.Keys.P256dh,
		Auth:      req.Keys.Auth,
		CreatedAt: time.Now(),
	}
	if err := s.storage.SavePushSubscription(sub); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "subscribed"})
}

// DELETE /api/push/subscribe — removes a push subscription by endpoint.
// Body: {"endpoint":"..."}
func (s *Server) handlePushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body failed"})
		return
	}

	var req struct {
		Endpoint string `json:"endpoint"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Endpoint == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	if err := s.storage.DeletePushSubscription(req.Endpoint); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "unsubscribed"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
