package comandcenter

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Abraxas-365/claudio/internal/attach"
	"golang.org/x/net/websocket"
)

// Server is the ComandCenter HTTP server.
type Server struct {
	password string
	storage  *Storage
	hub      *Hub
	dataDir  string
	mux      *http.ServeMux
}

// NewServer creates a Server and registers all routes.
func NewServer(password string, storage *Storage, hub *Hub, dataDir string) *Server {
	s := &Server{
		password: password,
		storage:  storage,
		hub:      hub,
		dataDir:  dataDir,
		mux:      http.NewServeMux(),
	}
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
	s.mux.Handle("GET /api/sessions/{id}/messages", s.auth(http.HandlerFunc(s.handleListMessages)))
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
