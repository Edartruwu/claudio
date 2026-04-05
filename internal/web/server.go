// Package web provides a browser-based UI for Claudio, using go-templ + HTMX + SSE.
package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// Config holds web server configuration.
type Config struct {
	Port     int
	Host     string // bind address, default "127.0.0.1"
	Password string
	Version  string
}

// Server is the web UI server.
type Server struct {
	config   Config
	mux      *http.ServeMux
	sessions *SessionManager
	tokens   map[string]time.Time // auth token -> expiry
	mu       sync.RWMutex
}

// New creates a new web UI server.
func New(cfg Config) *Server {
	s := &Server{
		config:   cfg,
		mux:      http.NewServeMux(),
		sessions: NewSessionManager(),
		tokens:   make(map[string]time.Time),
	}
	s.registerRoutes()
	return s
}

// Start starts the web server.
func (s *Server) Start() error {
	host := s.config.Host
	if host == "" {
		host = "127.0.0.1"
	}
	addr := fmt.Sprintf("%s:%d", host, s.config.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      s.mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // SSE needs no write timeout
		IdleTimeout:  120 * time.Second,
	}
	log.Printf("Claudio Web UI running at http://%s", addr)
	return srv.ListenAndServe()
}

func (s *Server) registerRoutes() {
	// Static files
	s.mux.HandleFunc("GET /static/", s.handleStatic)

	// Auth
	s.mux.HandleFunc("GET /login", s.handleLoginPage)
	s.mux.HandleFunc("POST /login", s.handleLogin)
	s.mux.HandleFunc("POST /logout", s.requireAuth(s.handleLogout))

	// Pages
	s.mux.HandleFunc("GET /", s.requireAuth(s.handleHome))
	s.mux.HandleFunc("GET /chat", s.requireAuth(s.handleChatPage))

	// API
	s.mux.HandleFunc("POST /api/projects/init", s.requireAuth(s.handleProjectInit))
	s.mux.HandleFunc("POST /api/chat/send", s.requireAuth(s.handleChatSend))
	s.mux.HandleFunc("GET /api/chat/stream", s.requireAuth(s.handleChatStream))
	s.mux.HandleFunc("POST /api/chat/approve", s.requireAuth(s.handleToolApprove))
	s.mux.HandleFunc("POST /api/chat/deny", s.requireAuth(s.handleToolDeny))
	s.mux.HandleFunc("GET /api/chat/status", s.requireAuth(s.handleChatStatus))
	s.mux.HandleFunc("GET /api/chat/replay", s.requireAuth(s.handleChatReplay))
	s.mux.HandleFunc("GET /api/panel/", s.requireAuth(s.handlePanel))
}

// requireAuth wraps a handler with authentication check.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("claudio_token")
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		s.mu.RLock()
		expiry, ok := s.tokens[cookie.Value]
		s.mu.RUnlock()
		if !ok || time.Now().After(expiry) {
			if ok {
				// Clean up expired token
				s.mu.Lock()
				delete(s.tokens, cookie.Value)
				s.mu.Unlock()
			}
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

// generateToken creates a secure random token.
func generateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// validatePassword does a constant-time comparison.
func (s *Server) validatePassword(input string) bool {
	return subtle.ConstantTimeCompare([]byte(input), []byte(s.config.Password)) == 1
}
