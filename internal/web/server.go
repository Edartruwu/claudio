// Package web provides a browser-based UI for Claudio, using go-templ + HTMX + SSE.
package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Abraxas-365/claudio/internal/config"
	"github.com/Abraxas-365/claudio/internal/services/skills"
	"github.com/Abraxas-365/claudio/internal/storage"
	"github.com/Abraxas-365/claudio/internal/teams"
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
	skills   *skills.Registry
	db       *storage.DB
	teams    *teams.Manager
	tokens   map[string]time.Time // auth token -> expiry
	mu       sync.RWMutex
}

// New creates a new web UI server.
func New(cfg Config, skillsRegistry *skills.Registry) *Server {
	// Open the shared global DB so web sessions are persisted alongside CLI sessions.
	db, err := storage.Open(config.GetPaths().DB)
	if err != nil {
		log.Printf("Warning: failed to open DB for session persistence: %v", err)
	}

	paths := config.GetPaths()
	teamMgr := teams.NewManager(paths.Home+"/teams", paths.TeamTemplates)

	s := &Server{
		config:   cfg,
		mux:      http.NewServeMux(),
		sessions: NewSessionManager(db),
		skills:   skillsRegistry,
		db:       db,
		teams:    teamMgr,
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

	// Project API
	s.mux.HandleFunc("POST /api/projects/init", s.requireAuth(s.handleProjectInit))

	// Session API
	s.mux.HandleFunc("POST /api/sessions/create", s.requireAuth(s.handleSessionCreate))
	s.mux.HandleFunc("GET /api/sessions/list", s.requireAuth(s.handleSessionList))
	s.mux.HandleFunc("POST /api/sessions/delete", s.requireAuth(s.handleSessionDelete))
	s.mux.HandleFunc("POST /api/sessions/rename", s.requireAuth(s.handleSessionRename))
	s.mux.HandleFunc("GET /api/sessions/history", s.requireAuth(s.handleSessionHistory))

	// Chat API (session-aware)
	s.mux.HandleFunc("POST /api/chat/send", s.requireAuth(s.handleChatSend))
	s.mux.HandleFunc("GET /api/chat/stream", s.requireAuth(s.handleChatStream))
	s.mux.HandleFunc("POST /api/chat/approve", s.requireAuth(s.handleToolApprove))
	s.mux.HandleFunc("POST /api/chat/deny", s.requireAuth(s.handleToolDeny))
	s.mux.HandleFunc("GET /api/chat/status", s.requireAuth(s.handleChatStatus))
	s.mux.HandleFunc("GET /api/chat/replay", s.requireAuth(s.handleChatReplay))

	// Autocomplete
	s.mux.HandleFunc("GET /api/autocomplete/files", s.requireAuth(s.handleAutocompleteFiles))
	s.mux.HandleFunc("GET /api/autocomplete/commands", s.requireAuth(s.handleAutocompleteCommands))
	s.mux.HandleFunc("GET /api/autocomplete/agents", s.requireAuth(s.handleAutocompleteAgents))

	// Command execution
	s.mux.HandleFunc("POST /api/commands/execute", s.requireAuth(s.handleCommandExecute))

	// Picker API (for /agent and /team commands)
	s.mux.HandleFunc("GET /api/picker/agents", s.requireAuth(s.handlePickerAgents))
	s.mux.HandleFunc("GET /api/picker/teams", s.requireAuth(s.handlePickerTeams))
	s.mux.HandleFunc("POST /api/picker/select-agent", s.requireAuth(s.handlePickerSelectAgent))
	s.mux.HandleFunc("POST /api/picker/spawn-team", s.requireAuth(s.handlePickerSpawnTeam))

	// Panels
	s.mux.HandleFunc("GET /api/panel/", s.requireAuth(s.handlePanel))
	s.mux.HandleFunc("POST /api/panel/config/update", s.requireAuth(s.handleConfigUpdate))
	s.mux.HandleFunc("POST /api/panel/tools/toggle", s.requireAuth(s.handleToolDeferToggle))
	s.mux.HandleFunc("GET /web/agents", s.requireAuth(s.handleAgentsList))
	s.mux.HandleFunc("GET /web/agents/stream", s.requireAuth(s.handleAgentsStream))

	// Model selector
	s.mux.HandleFunc("GET /api/sessions/model", s.requireAuth(s.handleGetModel))
	s.mux.HandleFunc("POST /api/sessions/model", s.requireAuth(s.handleSetModel))
	s.mux.HandleFunc("GET /api/sessions/models", s.requireAuth(s.handleListModels))

	// Nav sidebar
	s.mux.HandleFunc("GET /api/nav/agents", s.requireAuth(s.handleNavAgents))
	s.mux.HandleFunc("GET /api/nav/teams", s.requireAuth(s.handleNavTeams))
}

// requireAuth wraps a handler with authentication check.
// API requests (/api/) get a 401 JSON response; page requests get a redirect.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		deny := func() {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"unauthorized"}`))
			} else {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
			}
		}

		cookie, err := r.Cookie("claudio_token")
		if err != nil {
			deny()
			return
		}
		s.mu.RLock()
		expiry, ok := s.tokens[cookie.Value]
		s.mu.RUnlock()
		if !ok || time.Now().After(expiry) {
			if ok {
				s.mu.Lock()
				delete(s.tokens, cookie.Value)
				s.mu.Unlock()
			}
			deny()
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
